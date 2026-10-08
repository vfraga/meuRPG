package gen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PuraFome/meuRPG/backend/internal/platform/config"
)

// testKey is not a key: it is only a recognizable string, to see where it ends up.
const testKey = "test-secret-value-not-a-key"

var onePixel = func() []byte {
	b, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==")
	return b
}()

// buildBody is writeBody into memory, for the tests.
func buildBody(model string, req Request) ([]byte, error) {
	var b bytes.Buffer
	err := writeBody(&b, model, req)
	return b.Bytes(), err
}

func sceneRequest() Request {
	return Request{
		Prompt: "Uma cripta úmida", Style: "oil painting", AspectRatio: "16:9",
		References: []Reference{
			{MimeType: "image/png", Data: onePixel},
			{MimeType: "image/jpeg", Data: []byte("jpeg bytes"), Character: true},
		},
	}
}

func TestBuildBody(t *testing.T) {
	t.Parallel()
	body, err := buildBody("gemini-x", sceneRequest())
	if err != nil {
		t.Fatalf("buildBody() error = %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got["model"] != "gemini-x" || got["store"] != false {
		t.Errorf("model/store = %v/%v; store must be explicitly false", got["model"], got["store"])
	}
	rf, _ := got["response_format"].(map[string]any)
	if rf["type"] != "image" || rf["aspect_ratio"] != "16:9" || rf["mime_type"] != "image/png" || rf["image_size"] != "1K" {
		t.Errorf("response_format = %v", rf)
	}
	input, _ := got["input"].([]any)
	if len(input) != 3 {
		t.Fatalf("input has %d parts, want the text and 2 references", len(input))
	}
	text, _ := input[0].(map[string]any)
	if text["type"] != "text" || !strings.Contains(text["text"].(string), "Uma cripta úmida") || !strings.Contains(text["text"].(string), "oil painting") {
		t.Errorf("text part = %v", text)
	}
	ref, _ := input[1].(map[string]any)
	if ref["type"] != "image" || ref["mime_type"] != "image/png" || ref["data"] != base64.StdEncoding.EncodeToString(onePixel) {
		t.Errorf("reference part = %v", ref)
	}
}

func TestBuildBodyEdit(t *testing.T) {
	t.Parallel()
	req := Request{AspectRatio: "1:1", Edit: &Edit{
		Previous: Image{MimeType: "image/png", Data: onePixel}, Original: "uma cripta", Adjustments: []string{"mais escura"}, Instruction: "com uma ponte",
	}}
	body, err := buildBody("m", req)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"uma cripta", "mais escura", "com uma ponte", `"store":false`, base64.StdEncoding.EncodeToString(onePixel)} {
		if !strings.Contains(string(body), want) {
			t.Errorf("the edit body lacks %q", want)
		}
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()
	tooMany := sceneRequest()
	for range MaxObjectReferences {
		tooMany.References = append(tooMany.References, Reference{Data: []byte("x")})
	}
	for name, req := range map[string]Request{
		"a ratio the model does not return": {Prompt: "x", AspectRatio: "7:3"},
		"no prompt":                         {AspectRatio: "1:1"},
		"an edit without the image":         {AspectRatio: "1:1", Edit: &Edit{Instruction: "x"}},
		"too many objects":                  tooMany,
	} {
		if err := req.Validate(); err == nil {
			t.Errorf("%s: Validate() = nil", name)
		}
	}
	if err := sceneRequest().Validate(); err != nil {
		t.Errorf("a good request: %v", err)
	}
}

func answer(status, steps string) string {
	return `{"id":"i","status":"` + status + `","steps":[` + steps + `]}`
}

func imageBlock(b64 string) string {
	return `{"type":"image","mime_type":"image/png","data":"` + b64 + `"}`
}

func step(kind string, blocks ...string) string {
	return `{"type":"` + kind + `","content":[` + strings.Join(blocks, ",") + `]}`
}

func TestParseAnswer(t *testing.T) {
	t.Parallel()
	b64 := base64.StdEncoding.EncodeToString(onePixel)
	draft := base64.StdEncoding.EncodeToString([]byte("a draft, not the picture"))
	text := `{"type":"text","text":"ok"}`
	tests := []struct {
		name    string
		status  int
		body    string
		want    []byte
		wantErr error
		refused string
		denied  bool
	}{
		{name: "the picture of a completed answer", status: 200, body: answer("completed", step("model_output", text, imageBlock(b64))), want: onePixel},
		{
			name: "thought drafts before the output are ignored", status: 200,
			body: answer("completed", step("thought", imageBlock(draft))+","+step("thought", imageBlock(draft))+","+step("model_output", imageBlock(b64))), want: onePixel,
		},
		{name: "the last image of the output wins", status: 200, body: answer("completed", step("model_output", imageBlock(draft), imageBlock(b64))), want: onePixel},
		{name: "a draft only", status: 200, body: answer("completed", step("thought", imageBlock(draft))), wantErr: ErrNoImage},
		{name: "incomplete with only a draft", status: 200, body: answer("incomplete", step("thought", imageBlock(draft))), wantErr: ErrNoImage},
		{name: "incomplete even with an output image", status: 200, body: answer("incomplete", step("model_output", imageBlock(b64))), wantErr: ErrNoImage},
		{name: "in_progress", status: 200, body: answer("in_progress", step("model_output", imageBlock(b64))), wantErr: ErrNoImage},
		{name: "the old outputs layout is not read", status: 200, body: `{"status":"completed","outputs":[` + imageBlock(b64) + `]}`, wantErr: ErrNoImage},
		{name: "text only", status: 200, body: answer("completed", step("model_output", text)), wantErr: ErrNoImage},
		{name: "no steps", status: 200, body: answer("completed", ""), wantErr: ErrNoImage},
		{name: "a safety stop in the errors", status: 200, body: `{"status":"failed","steps":[],"errors":[{"code":"SAFETY","message":"x"}]}`, refused: "safety"},
		{name: "a 400 about safety", status: 400, body: `{"error":{"code":400,"message":"The prompt was blocked because of safety","status":"INVALID_ARGUMENT"}}`, refused: "safety"},
		{name: "a 400 about something else", status: 400, body: `{"error":{"code":400,"message":"bad field","status":"INVALID_ARGUMENT"}}`, wantErr: ErrUnavailable},
		{name: "a blocked key", status: 403, body: `{"error":{"code":403,"message":"x","status":"PERMISSION_DENIED","details":[{"reason":"API_KEY_SERVICE_BLOCKED"}]}}`, denied: true},
		{name: "an invalid key", status: 401, body: `{"error":{"code":401,"message":"x","status":"UNAUTHENTICATED"}}`, denied: true},
		{name: "not JSON", status: 200, body: `<html>`, wantErr: ErrUnavailable},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			img, err := parseAnswer(tc.status, strings.NewReader(tc.body))
			switch {
			case tc.refused != "":
				re, ok := errors.AsType[*RefusedError](err)
				if !ok || re.Reason != tc.refused {
					t.Errorf("error = %v, want a refusal for %s", err, tc.refused)
				}
			case tc.denied:
				na, ok := errors.AsType[*NotAuthorizedError](err)
				if !ok || !errors.Is(err, ErrNotAuthorized) || na.Status != tc.status {
					t.Errorf("error = %v, want a key error", err)
				}
				if tc.status == 403 && na.Reason != "API_KEY_SERVICE_BLOCKED" {
					t.Errorf("reason = %q, want Google's", na.Reason)
				}
			case tc.wantErr != nil:
				if !errors.Is(err, tc.wantErr) {
					t.Errorf("error = %v, want %v", err, tc.wantErr)
				}
			default:
				if err != nil || !bytes.Equal(img.Data, tc.want) || img.MimeType != "image/png" {
					t.Errorf("image = %v, %v", img, err)
				}
			}
		})
	}
}

func TestParseAnswerRefusesAnOversizedAnswer(t *testing.T) {
	t.Parallel()
	big := answer("completed", step("model_output", imageBlock(strings.Repeat("A", maxResponseBytes))))
	_, err := parseAnswer(200, io.LimitReader(strings.NewReader(big), maxResponseBytes+1))
	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("error = %v, want ErrUnavailable for an oversized answer", err)
	}
}

// server answers each call with the next handler's response, and records the
// requests.
type server struct {
	*httptest.Server
	calls   atomic.Int32
	mu      sync.Mutex
	headers []http.Header
	bodies  [][]byte
}

func newServer(t *testing.T, replies ...func(w http.ResponseWriter)) *server {
	t.Helper()
	s := &server{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		n := int(s.calls.Add(1)) - 1
		s.mu.Lock()
		s.headers = append(s.headers, r.Header.Clone())
		s.bodies = append(s.bodies, body)
		s.mu.Unlock()
		replies[min(n, len(replies)-1)](w)
	}))
	t.Cleanup(s.Close)
	return s
}

func ok(w http.ResponseWriter) {
	_, _ = fmt.Fprint(w, answer("completed", step("model_output", imageBlock(base64.StdEncoding.EncodeToString(onePixel)))))
}

func status(code int) func(http.ResponseWriter) {
	return func(w http.ResponseWriter) { w.WriteHeader(code) }
}

// geminiFor points a client at s with the server's own HTTP client, which has
// its own transport and so its own pool of keep-alive connections. With
// http.DefaultTransport, shared by the parallel tests, a connection that went
// idle in one test could be closed by another test's server.Close, or picked
// by a test whose server got the same port, and the call failed with a
// network error that has nothing to do with the code under test.
func geminiFor(s *server, logs io.Writer) *Gemini {
	return &Gemini{
		Key: config.Secret(testKey), URL: s.URL, Client: s.Client(), Backoff: time.Millisecond, AttemptTimeout: 2 * time.Second,
		Logger: slog.New(slog.NewTextHandler(logs, nil)),
	}
}

func TestGeminiSendsTheKeyOnlyInTheHeader(t *testing.T) {
	t.Parallel()
	s := newServer(t, ok)
	var logs bytes.Buffer
	g := geminiFor(s, &logs)
	img, err := g.Generate(t.Context(), sceneRequest())
	if err != nil || !bytes.Equal(img.Data, onePixel) {
		t.Fatalf("Generate() = %v, %v", img, err)
	}
	if got := s.headers[0].Get("x-goog-api-key"); got != testKey {
		t.Errorf("x-goog-api-key header = %q", got)
	}
	if got := s.headers[0].Get("Api-Revision"); got != APIRevision || APIRevision != "2026-05-20" {
		t.Errorf("Api-Revision header = %q", got)
	}
	if bytes.Contains(s.bodies[0], []byte(testKey)) {
		t.Error("the key is in the request body")
	}
}

func TestGeminiRetriesOnceOnA5xx(t *testing.T) {
	t.Parallel()
	s := newServer(t, status(503), ok)
	var logs bytes.Buffer
	img, err := geminiFor(s, &logs).Generate(t.Context(), sceneRequest())
	if err != nil || len(img.Data) == 0 {
		t.Fatalf("Generate() = %v, %v; want the second try to work", img, err)
	}
	if s.calls.Load() != 2 {
		t.Errorf("calls = %d, want 2", s.calls.Load())
	}
	if strings.Contains(logs.String(), testKey) || strings.Contains(logs.String(), "cripta") {
		t.Errorf("the log has the key or the text: %s", logs.String())
	}
}

func TestGeminiGivesUpAfterTheRetry(t *testing.T) {
	t.Parallel()
	s := newServer(t, status(500))
	var logs bytes.Buffer
	_, err := geminiFor(s, &logs).Generate(t.Context(), sceneRequest())
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}
	if s.calls.Load() != 2 {
		t.Errorf("calls = %d, want exactly 2 (one retry)", s.calls.Load())
	}
}

func TestGeminiDoesNotRetryARefusalOrA400(t *testing.T) {
	t.Parallel()
	s := newServer(t, func(w http.ResponseWriter) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprint(w, `{"error":{"message":"blocked by safety"}}`)
	})
	var logs bytes.Buffer
	_, err := geminiFor(s, &logs).Generate(t.Context(), sceneRequest())
	if _, ok := errors.AsType[*RefusedError](err); !ok {
		t.Fatalf("error = %v, want a refusal", err)
	}
	if s.calls.Load() != 1 {
		t.Errorf("calls = %d, want 1", s.calls.Load())
	}
}

// A timeout, like a cut connection or an unreadable answer, may have been
// billed: it is never retried.
func TestGeminiDoesNotRetryATimeout(t *testing.T) {
	t.Parallel()
	// The handler counts the call as soon as it starts and then holds the
	// request until the client gives up, so the timeout always falls after
	// the call arrived, however slow the machine is. The body is read first:
	// the server only notices a closed connection once it has.
	var calls atomic.Int32
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })
	var logs bytes.Buffer
	g := &Gemini{
		Key: config.Secret(testKey), URL: srv.URL, Client: srv.Client(), Backoff: time.Millisecond,
		AttemptTimeout: time.Second, Logger: slog.New(slog.NewTextHandler(&logs, nil)),
	}
	if _, err := g.Generate(t.Context(), sceneRequest()); !errors.Is(err, ErrUnavailable) || !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("error = %v, want ErrUnavailable by timeout", err)
	}
	// A retry would come after the 1 ms backoff; this is far longer.
	time.Sleep(200 * time.Millisecond)
	if calls.Load() != 1 {
		t.Errorf("calls = %d, want 1: a timeout is not retried", calls.Load())
	}
}

func TestGeminiDoesNotRetryAnUnreadableAnswer(t *testing.T) {
	t.Parallel()
	s := newServer(t, func(w http.ResponseWriter) { _, _ = fmt.Fprint(w, "<html>") }, ok)
	var logs bytes.Buffer
	if _, err := geminiFor(s, &logs).Generate(t.Context(), sceneRequest()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("error = %v", err)
	}
	if s.calls.Load() != 1 {
		t.Errorf("calls = %d, want 1", s.calls.Load())
	}
}

func TestGeminiRetriesOnA429(t *testing.T) {
	t.Parallel()
	s := newServer(t, status(429), ok)
	var logs bytes.Buffer
	if _, err := geminiFor(s, &logs).Generate(t.Context(), sceneRequest()); err != nil || s.calls.Load() != 2 {
		t.Errorf("Generate() error = %v after %d calls, want a retry and success", err, s.calls.Load())
	}
}

// A blocked key is the operator's error: not a refusal, not retried.
func TestGeminiAKeyErrorIsNotARefusal(t *testing.T) {
	t.Parallel()
	s := newServer(t, func(w http.ResponseWriter) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = fmt.Fprint(w, `{"error":{"code":403,"message":"nope","status":"PERMISSION_DENIED","details":[{"reason":"API_KEY_SERVICE_BLOCKED"}]}}`)
	}, ok)
	var logs bytes.Buffer
	_, err := geminiFor(s, &logs).Generate(t.Context(), sceneRequest())
	if !errors.Is(err, ErrNotAuthorized) {
		t.Fatalf("error = %v, want ErrNotAuthorized", err)
	}
	if _, refused := errors.AsType[*RefusedError](err); refused || s.calls.Load() != 1 || strings.Contains(err.Error(), testKey) {
		t.Errorf("refused = %v, calls = %d, error = %v", refused, s.calls.Load(), err)
	}
}

// The key header never follows a redirect.
func TestGeminiRefusesRedirects(t *testing.T) {
	t.Parallel()
	var elsewhere atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		elsewhere.Add(1)
		if r.Header.Get("x-goog-api-key") != "" {
			t.Error("the key followed a redirect")
		}
	}))
	t.Cleanup(other.Close)
	s := newServer(t, func(w http.ResponseWriter) {
		w.Header().Set("Location", other.URL)
		w.WriteHeader(http.StatusTemporaryRedirect)
	})
	var logs bytes.Buffer
	if _, err := geminiFor(s, &logs).Generate(t.Context(), sceneRequest()); !errors.Is(err, ErrUnavailable) {
		t.Errorf("error = %v", err)
	}
	if elsewhere.Load() != 0 {
		t.Error("the redirect was followed")
	}
}

// The body is written as a stream: the same bytes as a Marshal of everything,
// with the images in base64, and an image never held twice.
func TestWriteBodyIsValidJSONAndStreams(t *testing.T) {
	t.Parallel()
	body, err := buildBody("m", sceneRequest())
	if err != nil || !json.Valid(body) {
		t.Fatalf("body valid = %v, %v", json.Valid(body), err)
	}
	r := bodyReader("m", sceneRequest())
	read, err := io.ReadAll(r)
	if err != nil || !bytes.Equal(read, body) {
		t.Errorf("the streamed body differs: %v", err)
	}
	if size := sceneRequest().BodySize(); size < len(body)-600 || size > len(body)+1200 {
		t.Errorf("BodySize() = %d, body is %d", size, len(body))
	}
}

func TestGeminiErrorsNeverCarryTheKey(t *testing.T) {
	t.Parallel()
	s := newServer(t, status(500))
	var logs bytes.Buffer
	_, err := geminiFor(s, &logs).Generate(t.Context(), sceneRequest())
	for _, shown := range []string{err.Error(), logs.String(), fmt.Sprintf("%v %+v %#v", geminiFor(s, &logs), geminiFor(s, &logs), geminiFor(s, &logs))} {
		if strings.Contains(shown, testKey) {
			t.Errorf("the key leaked into %q", shown)
		}
	}
}

func TestGeminiContextCanceled(t *testing.T) {
	t.Parallel()
	s := newServer(t, status(500))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var logs bytes.Buffer
	if _, err := geminiFor(s, &logs).Generate(ctx, sceneRequest()); !errors.Is(err, ErrUnavailable) {
		t.Errorf("error = %v", err)
	}
}

func TestFake(t *testing.T) {
	t.Parallel()
	f := &Fake{SlowDelay: 10 * time.Millisecond}
	req := Request{Prompt: "uma cripta", AspectRatio: "16:9"}
	a, err := f.Generate(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := f.Generate(t.Context(), req)
	if !bytes.Equal(a.Data, b.Data) {
		t.Error("the same request gave different pictures")
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(a.Data))
	if err != nil || cfg.Width != 256 || cfg.Height != 144 {
		t.Errorf("16:9 picture = %dx%d, %v; want 256x144", cfg.Width, cfg.Height, err)
	}
	for ratio, want := range map[string][2]int{"1:1": {256, 256}, "9:16": {144, 256}, "21:9": {256, 109}, "4:5": {204, 256}} {
		img, err := f.Generate(t.Context(), Request{Prompt: "x", AspectRatio: ratio})
		if err != nil {
			t.Fatal(err)
		}
		c, _ := png.DecodeConfig(bytes.NewReader(img.Data))
		if [2]int{c.Width, c.Height} != want {
			t.Errorf("%s = %dx%d, want %v", ratio, c.Width, c.Height, want)
		}
	}
	if _, err := f.Generate(t.Context(), Request{Prompt: "x " + MarkerRefuse, AspectRatio: "1:1"}); err == nil {
		t.Error("no refusal")
	} else if _, ok := errors.AsType[*RefusedError](err); !ok {
		t.Errorf("refusal error = %v", err)
	}
	if _, err := f.Generate(t.Context(), Request{Prompt: MarkerEmpty, AspectRatio: "1:1"}); !errors.Is(err, ErrNoImage) {
		t.Errorf("empty = %v", err)
	}
	if _, err := f.Generate(t.Context(), Request{Prompt: MarkerError, AspectRatio: "1:1"}); !errors.Is(err, ErrUnavailable) {
		t.Errorf("error = %v", err)
	}
	start := time.Now()
	if _, err := f.Generate(t.Context(), Request{Prompt: MarkerSlow, AspectRatio: "1:1"}); err != nil || time.Since(start) < 10*time.Millisecond {
		t.Errorf("slow: %v after %v", err, time.Since(start))
	}
	if _, err := f.Generate(t.Context(), Request{AspectRatio: "7:3", Prompt: "x"}); err == nil {
		t.Error("a bad ratio was accepted")
	}
	calls := f.Calls()
	if len(calls) == 0 || !strings.Contains(string(calls[0].Body), `"store":false`) {
		t.Errorf("the fake did not record the body Gemini would send: %d calls", len(calls))
	}
}

func mapRequest(layout string) Request {
	r := sceneRequest()
	r.Drawing = &Image{MimeType: "image/png", Data: onePixel}
	r.Layout = layout
	return r
}

// A request made from a map carries its drawing as the first image (after an edit's, if
// any), the layout tells the model how to read it, and the rooms go only with the
// textured map (MR-039).
func TestMapRequests(t *testing.T) {
	t.Parallel()
	for layout, phrase := range map[string]string{LayoutScene: "Paint a scene", LayoutIsometric: "isometric view", LayoutTexture: "textured top-down battle map"} {
		r := mapRequest(layout)
		if err := r.Validate(); err != nil {
			t.Errorf("%s: Validate() = %v", layout, err)
		}
		if text := r.Text(); !strings.Contains(text, phrase) || !strings.Contains(text, r.Prompt) {
			t.Errorf("%s: text = %q, want the framing %q and the master's words", layout, text, phrase)
		}
		body, err := buildBody("m", r)
		if err != nil {
			t.Fatal(err)
		}
		var parsed struct {
			Input []struct {
				Type string `json:"type"`
				Data string `json:"data"`
			} `json:"input"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			t.Fatal(err)
		}
		// The text, then the drawing, then the references.
		if len(parsed.Input) != 2+len(r.References) || parsed.Input[1].Type != "image" || parsed.Input[1].Data != base64.StdEncoding.EncodeToString(onePixel) {
			t.Errorf("%s: body input = %+v, want the text and then the drawing", layout, parsed.Input)
		}
		if r.BodySize() < len(onePixel) {
			t.Errorf("%s: BodySize() = %d does not count the drawing", layout, r.BodySize())
		}
	}
	// The rooms: only with the texture, one line each.
	r := mapRequest(LayoutTexture)
	r.Rooms = []string{"Sala 1: 5 x 4 squares", "Sala 2: 3 x 3 squares"}
	if err := r.Validate(); err != nil {
		t.Errorf("rooms with the texture: %v", err)
	}
	if text := r.Text(); !strings.Contains(text, "- Sala 1: 5 x 4 squares") || !strings.Contains(text, "- Sala 2: 3 x 3 squares") {
		t.Errorf("the texture's text lacks the rooms: %q", text)
	}
	for _, layout := range []string{LayoutScene, LayoutIsometric} {
		r := mapRequest(layout)
		r.Rooms = []string{"Sala 1: 5 x 4 squares"}
		if err := r.Validate(); err == nil {
			t.Errorf("rooms were accepted with %s", layout)
		}
	}
	tooMany := mapRequest(LayoutTexture)
	for range MaxRooms + 1 {
		tooMany.Rooms = append(tooMany.Rooms, "Sala 1: 5 x 4 squares")
	}
	if err := tooMany.Validate(); err == nil {
		t.Error("more than MaxRooms rooms were accepted")
	}
	// A drawing needs a layout and the other way round, and an edit has neither.
	noLayout := mapRequest("")
	noDrawing := mapRequest(LayoutScene)
	noDrawing.Drawing = nil
	unknown := mapRequest("panorama")
	edit := mapRequest(LayoutScene)
	edit.Edit = &Edit{Previous: Image{MimeType: "image/png", Data: onePixel}, Instruction: "mais escura"}
	for name, r := range map[string]Request{"a drawing with no layout": noLayout, "a layout with no drawing": noDrawing, "an unknown layout": unknown, "an edit with a drawing": edit} {
		if err := r.Validate(); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}
