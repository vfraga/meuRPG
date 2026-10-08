package gen

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/PuraFome/meuRPG/backend/internal/platform/config"
)

// GeminiURL is the Gemini API's Interactions endpoint. We use it, not
// generateContent, because only it has `store`: with store=false the call is
// not kept as an interaction (the default keeps one for 55 days on the paid
// tier; it does not promise that Google keeps nothing, see docs/privacy.md).
// Docs, read 06/10/2026:
//   - https://ai.google.dev/gemini-api/docs/image-generation (request and
//     response_format, ratios, the 10 + 4 references)
//   - https://ai.google.dev/gemini-api/docs/interactions (store)
//   - https://ai.google.dev/gemini-api/docs/interactions-breaking-changes-may-2026
//     (the Api-Revision header and the steps layout)
//   - https://ai.google.dev/gemini-api/docs/pricing (the price per image)
const GeminiURL = "https://generativelanguage.googleapis.com/v1beta/interactions"

// APIRevision pins the response layout (the header `Api-Revision`): the
// `steps` schema, which became the default on 26/05/2026. Pinned, a change of
// the default cannot change what the parser reads.
const APIRevision = "2026-05-20"

// maxResponseBytes bounds what one answer may be: a 1K picture is about 2 MB
// in base64, so 8 MiB leaves room for drafts, and a broken server cannot fill
// the memory (docs/operations.md has the budget).
const maxResponseBytes = 8 << 20

// Gemini is the Generator that calls the Gemini API with a key.
type Gemini struct {
	// Key is the API key, sent only in the x-goog-api-key header. It is a
	// config.Secret: it prints as [REDACTED], and nothing here formats it.
	Key config.Secret
	// ModelName is the image model (GEMINI_IMAGE_MODEL); empty means DefaultModel.
	ModelName string
	// URL is the endpoint; empty means GeminiURL. Tests point it at httptest.
	URL string
	// Client does the calls; nil means a client of its own. Redirects are
	// always refused, so the key header never follows one.
	Client *http.Client
	// AttemptTimeout bounds one call; zero means two minutes.
	AttemptTimeout time.Duration
	// Backoff is the wait before the one retry; zero means two seconds.
	Backoff time.Duration
	// Logger gets one line per failed attempt: the status or the kind, never the text.
	Logger *slog.Logger
}

var _ Generator = (*Gemini)(nil)

// Model implements Generator.
func (g *Gemini) Model() string {
	if g.ModelName == "" {
		return DefaultModel
	}
	return g.ModelName
}

// writeBody writes the call's JSON to w: the text, then the previous picture
// (an edit), then the references, as `input`; PNG out, 1K, and store=false.
// Nothing else is in it: the Request has nothing personal to put. The images
// are base64-encoded as they are written, so the body never exists in memory
// as one piece. Interactions use snake_case.
func writeBody(w io.Writer, model string, req Request) error {
	if err := req.Validate(); err != nil {
		return err
	}
	text, err := json.Marshal(struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}{"text", req.Text()})
	if err != nil {
		return err
	}
	modelJSON, _ := json.Marshal(model)
	ratio, _ := json.Marshal(req.AspectRatio)
	var werr error
	put := func(s string) {
		if werr == nil {
			_, werr = io.WriteString(w, s)
		}
	}
	put(`{"model":` + string(modelJSON) + `,"input":[` + string(text))
	image := func(img Image) {
		mime, _ := json.Marshal(img.MimeType)
		put(`,{"type":"image","mime_type":` + string(mime) + `,"data":"`)
		if werr != nil {
			return
		}
		enc := base64.NewEncoder(base64.StdEncoding, w)
		if _, werr = enc.Write(img.Data); werr == nil {
			werr = enc.Close()
		}
		put(`"}`)
	}
	if req.Edit != nil {
		image(req.Edit.Previous)
	}
	if req.Drawing != nil {
		image(*req.Drawing)
	}
	for _, ref := range req.References {
		image(ref.Image)
	}
	put(`],"response_format":{"type":"image","mime_type":"image/png","aspect_ratio":` + string(ratio) + `,"image_size":"1K"},"store":false}`)
	return werr
}

// bodyReader streams writeBody through a pipe: the body of one attempt.
func bodyReader(model string, req Request) io.ReadCloser {
	pr, pw := io.Pipe()
	go func() {
		// A panic while building the body ends this attempt with an error; it
		// must not end the process (the goroutine is ours, net/http does not recover it).
		defer func() {
			if r := recover(); r != nil {
				_ = pw.CloseWithError(fmt.Errorf("gen: building the request body panicked: %v", r))
			}
		}()
		_ = pw.CloseWithError(writeBody(pw, model, req))
	}()
	return pr
}

// Generate implements Generator: one call, and one more after a 429 or a 5xx
// answer. Nothing else is retried: a timeout, a cut connection or an answer
// that cannot be read may have been billed already.
func (g *Gemini) Generate(ctx context.Context, req Request) (Image, error) {
	if err := req.Validate(); err != nil {
		return Image{}, err
	}
	backoff := g.Backoff
	if backoff == 0 {
		backoff = 2 * time.Second
	}
	var last error
	for attempt := range 2 {
		if attempt > 0 {
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return Image{}, errors.Join(ErrUnavailable, ctx.Err())
			}
		}
		img, retry, err := g.once(ctx, req)
		if err == nil || !retry {
			return img, err
		}
		last = err
		g.log(ctx, attempt, err)
	}
	return Image{}, last
}

func (g *Gemini) log(ctx context.Context, attempt int, err error) {
	logger := g.Logger
	if logger == nil {
		logger = slog.Default()
	}
	// The error carries a status or a kind, never the prompt or the key.
	logger.WarnContext(ctx, "gen: the image service failed", "attempt", attempt+1, "error", err)
}

// once makes one call. retry says whether a second try is worth it: only after
// a 429 or a 5xx response.
func (g *Gemini) once(ctx context.Context, req Request) (img Image, retry bool, err error) {
	timeout := g.AttemptTimeout
	if timeout == 0 {
		timeout = 2 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	url := g.URL
	if url == "" {
		url = GeminiURL
	}
	body := bodyReader(g.Model(), req)
	defer func() { _ = body.Close() }()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return Image{}, false, errors.Join(ErrUnavailable, errors.New("cannot build the request"))
	}
	// The only replay net/http makes of a POST is when a reused connection
	// failed before one byte of the request was written (so nothing reached
	// the server and nothing can be billed), and it needs GetBody for that.
	// Without it, an attempt that lands on a keep-alive connection that died
	// while idle fails with a network error. A fresh body is the same bytes.
	model := g.Model()
	httpReq.GetBody = func() (io.ReadCloser, error) { return bodyReader(model, req), nil }
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Api-Revision", APIRevision)
	httpReq.Header.Set("x-goog-api-key", g.Key.Reveal())
	res, err := g.client().Do(httpReq)
	if err != nil {
		// Not %w of err: only the kind goes up, and the request may have been
		// billed, so it is not retried.
		kind := "network error"
		if errors.Is(err, context.DeadlineExceeded) {
			kind = "timeout"
		}
		return Image{}, false, errors.Join(ErrUnavailable, errors.New(kind))
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode >= 500 || res.StatusCode == http.StatusTooManyRequests {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 1<<16))
		return Image{}, true, errors.Join(ErrUnavailable, fmt.Errorf("status %d", res.StatusCode))
	}
	img, err = parseAnswer(res.StatusCode, io.LimitReader(res.Body, maxResponseBytes+1))
	return img, false, err
}

// client is the HTTP client, with redirects refused.
func (g *Gemini) client() *http.Client {
	c := &http.Client{}
	if g.Client != nil {
		c = &http.Client{Transport: g.Client.Transport, Jar: g.Client.Jar, Timeout: g.Client.Timeout}
	}
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return c
}

// The answer's JSON, the `steps` layout of Api-Revision 2026-05-20.
type (
	interaction struct {
		Status string        `json:"status"`
		Steps  []interStep   `json:"steps"`
		Errors []interError  `json:"errors"`
		Error  *apiErrorBody `json:"error"`
	}
	interStep struct {
		Type    string       `json:"type"`
		Content []interBlock `json:"content"`
	}
	interBlock struct {
		Type     string `json:"type"`
		MimeType string `json:"mime_type"`
		Data     string `json:"data"`
	}
	interError struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	// apiErrorBody is Google's error answer: {"error": {code, message, status, details}}.
	apiErrorBody struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
		Details []struct {
			Reason string `json:"reason"`
		} `json:"details"`
	}
)

// NotAuthorizedError says what Google answered to a refused key, for the log:
// the HTTP status and Google's own reason (API_KEY_SERVICE_BLOCKED,
// PERMISSION_DENIED...). Never the key. It is also an ErrNotAuthorized.
type NotAuthorizedError struct {
	Status int
	Reason string
}

func (e *NotAuthorizedError) Error() string {
	return fmt.Sprintf("the image service did not accept the key: status %d, %s", e.Status, e.Reason)
}

// Is makes errors.Is(err, ErrNotAuthorized) true.
func (e *NotAuthorizedError) Is(target error) bool { return target == ErrNotAuthorized }

// parseAnswer reads an answer: the picture, a refusal, or no image.
//
// The picture is the last image block of the model_output steps of a
// completed interaction. A "thought" step holds the model's drafts, and an
// image there is not the result: a draft is never taken. An interaction that is
// not completed has no result.
func parseAnswer(status int, body io.Reader) (Image, error) {
	// Decoded straight from the stream: the answer is never held as bytes and
	// as a structure at once. One byte over the cap is enough to know it is too big.
	limited := &io.LimitedReader{R: body, N: maxResponseBytes + 1}
	var doc interaction
	err := json.NewDecoder(limited).Decode(&doc)
	if limited.N <= 0 {
		return Image{}, errors.Join(ErrUnavailable, errors.New("the answer is too big"))
	}
	if err != nil {
		return Image{}, errors.Join(ErrUnavailable, fmt.Errorf("cannot read the answer (status %d)", status))
	}
	if status >= 400 || doc.Error != nil {
		reason := ""
		if e := doc.Error; e != nil {
			if len(e.Details) > 0 {
				reason = e.Details[0].Reason
			}
			if reason == "" {
				reason = e.Status
			}
			if word := safetyWord(e.Message); word != "" && status == http.StatusBadRequest {
				return Image{}, &RefusedError{Reason: word}
			}
		}
		if status == http.StatusUnauthorized || status == http.StatusForbidden {
			return Image{}, &NotAuthorizedError{Status: status, Reason: reason}
		}
		return Image{}, errors.Join(ErrUnavailable, fmt.Errorf("status %d", status))
	}
	if doc.Status != "completed" {
		// Incomplete, failed, canceled: no result, even if a draft is there. A
		// safety stop says so in its errors.
		for _, e := range doc.Errors {
			if word := safetyWord(e.Code + " " + e.Message); word != "" {
				return Image{}, &RefusedError{Reason: word}
			}
		}
		return Image{}, ErrNoImage
	}
	var last *interBlock
	for i := range doc.Steps {
		if doc.Steps[i].Type != "model_output" {
			continue
		}
		for j := range doc.Steps[i].Content {
			if b := &doc.Steps[i].Content[j]; b.Type == "image" && b.Data != "" {
				last = b
			}
		}
	}
	if last == nil {
		return Image{}, ErrNoImage
	}
	data, err := base64.StdEncoding.DecodeString(last.Data)
	if err != nil || len(data) == 0 {
		return Image{}, ErrNoImage
	}
	mime := last.MimeType
	if mime == "" {
		mime = "image/png"
	}
	return Image{MimeType: mime, Data: data}, nil
}

func safetyWord(s string) string {
	s = strings.ToLower(s)
	switch {
	case strings.Contains(s, "safety"):
		return "safety"
	case strings.Contains(s, "prohibited"):
		return "prohibited"
	case strings.Contains(s, "blocked"):
		return "blocked"
	case strings.Contains(s, "recitation"), strings.Contains(s, "policy"):
		return "policy"
	}
	return ""
}
