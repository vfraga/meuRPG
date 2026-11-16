package voice

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"uuid"
)

func TestForNPCIsStableAndSpreads(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	for range 200 {
		id := uuid.New().String()
		v := ForNPC(id)
		if v != ForNPC(id) {
			t.Fatalf("ForNPC(%s) changed between two calls", id)
		}
		if v == Narrator {
			t.Errorf("an NPC got the narrator's voice %q", v)
		}
		seen[v] = true
	}
	if len(seen) != len(npcVoices) {
		t.Errorf("200 NPCs used %d of the %d voices of the list", len(seen), len(npcVoices))
	}
}

func TestFakeMakesAShortSilentWAV(t *testing.T) {
	t.Parallel()
	f := &Fake{}
	a, err := f.Speak(t.Context(), "olá", Narrator)
	if err != nil || a.ContentType != ContentType {
		t.Fatalf("Speak() = %v, %v", a.ContentType, err)
	}
	if string(a.Data[:4]) != "RIFF" || string(a.Data[8:12]) != "WAVE" || f.Calls() != 1 {
		t.Errorf("not a WAV file, or calls = %d", f.Calls())
	}
	if n := binary.LittleEndian.Uint32(a.Data[40:]); int(n) != len(a.Data)-44 {
		t.Errorf("data chunk says %d bytes, the file holds %d", n, len(a.Data)-44)
	}
	for _, b := range a.Data[44:] {
		if b != 0 {
			t.Fatal("the fake audio is not silent")
		}
	}
}

// answerOf is what the model answers: one part with the PCM in base64.
func answerOf(mime string, pcm []byte) string {
	return `{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"` + mime + `","data":"` + base64.StdEncoding.EncodeToString(pcm) + `"}}]}}]}`
}

// geminiOf is a Gemini that calls srv with srv's own client, so that no other test shares its
// connections: the zero client uses http.DefaultTransport, and every httptest.Server.Close of a parallel
// test closes the idle connections of that one, which can break a call of this test in flight. It logs
// into the returned builder, and prints the log if the test fails.
func geminiOf(t *testing.T, srv *httptest.Server, model string) (*Gemini, *strings.Builder) {
	t.Helper()
	client := srv.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	log := &strings.Builder{}
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("log of the call: %s", log.String())
		}
	})
	return &Gemini{Key: "secret-key", ModelName: model, URL: srv.URL + "/", Client: client, Logger: slog.New(slog.NewTextHandler(log, nil))}, log
}

func TestGeminiAsksForAudioWithTheVoiceAndWrapsThePCM(t *testing.T) {
	t.Parallel()
	var gotPath, gotKey, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotKey = r.URL.Path, r.Header.Get("x-goog-api-key")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		_, _ = w.Write([]byte(answerOf("audio/L16;codec=pcm;rate=16000", []byte{1, 0, 2, 0})))
	}))
	t.Cleanup(srv.Close)
	g, _ := geminiOf(t, srv, "a-model")
	g.URL = srv.URL + "/models/"
	a, err := g.Speak(t.Context(), "Bem-vindos", "Puck")
	if err != nil {
		t.Fatalf("Speak() error = %v", err)
	}
	if gotPath != "/models/a-model:generateContent" || gotKey != "secret-key" {
		t.Errorf("path %q, key %q", gotPath, gotKey)
	}
	for _, want := range []string{`"text":"Bem-vindos"`, `"responseModalities":["AUDIO"]`, `"voiceName":"Puck"`} {
		if !strings.Contains(gotBody, want) {
			t.Errorf("the body %s lacks %s", gotBody, want)
		}
	}
	if rate := binary.LittleEndian.Uint32(a.Data[24:]); rate != 16000 || len(a.Data) != 48 {
		t.Errorf("rate %d, %d bytes; want the mime type's 16000 and 44 + 4", rate, len(a.Data))
	}
}

func TestGeminiFailuresAreTypedAndNeverQuoteTheTextOrTheKey(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		status int
		body   string
		want   error
	}{
		"a server error":  {500, `{"error":{"message":"boom"}}`, ErrUnavailable},
		"not json":        {200, `<html>`, ErrUnavailable},
		"no audio":        {200, `{"candidates":[{"content":{"parts":[{"text":"hi"}]}}]}`, ErrNoAudio},
		"not base64":      {200, `{"candidates":[{"content":{"parts":[{"inlineData":{"data":"!!"}}]}}]}`, ErrNoAudio},
		"a refused text":  {200, `{"promptFeedback":{"blockReason":"SAFETY"}}`, ErrRefused},
		"no candidates":   {200, `{}`, ErrNoAudio},
		"an empty answer": {200, ``, ErrUnavailable},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			t.Cleanup(srv.Close)
			g, log := geminiOf(t, srv, "m")
			_, err := g.Speak(t.Context(), "TEXTO-SECRETO", "Kore")
			if !isSentinel(err, tc.want) {
				t.Errorf("error = %v (%T), want the sentinel %v itself", err, err, tc.want)
			}
			if strings.Contains(log.String(), "secret-key") || strings.Contains(log.String(), "TEXTO-SECRETO") {
				t.Errorf("the log holds the key or the text: %s", log.String())
			}
		})
	}
}

func TestGeminiATransportFailureIsTheSentinelAndLogsTheCauseNotTheTextOrTheKey(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL + "/"
	srv.Close() // nothing listens at url any more: the call itself fails
	var log strings.Builder
	g := &Gemini{Key: "secret-key", ModelName: "m", URL: url, Logger: slog.New(slog.NewTextHandler(&log, nil))}
	_, err := g.Speak(t.Context(), "TEXTO-SECRETO", "Kore")
	if !isSentinel(err, ErrUnavailable) {
		t.Errorf("error = %v (%T), want the sentinel ErrUnavailable itself, as on every other path; log: %s", err, err, log.String())
	}
	if !strings.Contains(log.String(), "the call failed") || !strings.Contains(log.String(), "cause=") {
		t.Errorf("the log does not say the call failed and why: %s", log.String())
	}
	if strings.Contains(log.String(), "secret-key") || strings.Contains(log.String(), "TEXTO-SECRETO") {
		t.Errorf("the log holds the key or the text: %s", log.String())
	}
}

// isSentinel says err is the sentinel itself: it is, or wraps, want, and it wraps nothing on top.
func isSentinel(err, want error) bool {
	return errors.Is(err, want) && errors.Unwrap(err) == nil
}

func TestGeminiWithoutAModelOrTextAsksNothing(t *testing.T) {
	t.Parallel()
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	t.Cleanup(srv.Close)
	noModel, _ := geminiOf(t, srv, "")
	noText, _ := geminiOf(t, srv, "m")
	for _, g := range []*Gemini{noModel, noText} {
		text := "x"
		if g.ModelName != "" {
			text = "  "
		}
		if _, err := g.Speak(context.Background(), text, "Kore"); !errors.Is(err, ErrNoAudio) {
			t.Errorf("error = %v, want ErrNoAudio", err)
		}
	}
	if called {
		t.Error("a call without a model or text reached the service")
	}
}

func TestSampleRateIsBoundedAndFallsBackToTheDefault(t *testing.T) {
	t.Parallel()
	for mime, want := range map[string]uint32{
		"audio/L16;codec=pcm;rate=24000": 24000,
		"audio/L16;rate=16000":           16000,
		"audio/L16":                      defaultRate,
		"audio/L16;rate=0":               defaultRate,
		"audio/L16;rate=99999999999":     defaultRate,
		"audio/L16;rate=4294967295":      defaultRate,
		"audio/L16;rate=7999":            defaultRate,
		"audio/L16;rate=192001":          defaultRate,
		"audio/L16;rate=192000":          192000,
	} {
		if got := sampleRate(mime); got != want {
			t.Errorf("sampleRate(%q) = %d, want %d", mime, got, want)
		}
	}
}

// riff is a small WAV file as a model that answers with WAV makes it: its own header and its own rate.
func riff(rate uint32) []byte {
	return wav([]byte{1, 0, 2, 0, 3, 0}, rate)
}

func speakWith(t *testing.T, mime string, data []byte) (Audio, error) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(answerOf(mime, data)))
	}))
	t.Cleanup(srv.Close)
	g, _ := geminiOf(t, srv, "m")
	return g.Speak(t.Context(), "Olá", "Puck")
}

func TestGeminiPassesAWAVAnswerThroughByteForByte(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		mime string
		data []byte
	}{
		"audio/wav":               {"audio/wav", riff(24000)},
		"audio/x-wav":             {"audio/x-wav", riff(16000)},
		"a mime type with params": {"audio/wav; codecs=1", riff(24000)},
		"RIFF bytes, other mime":  {"application/octet-stream", riff(24000)},
		"RIFF bytes, no mime":     {"", riff(22050)},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			a, err := speakWith(t, tc.mime, tc.data)
			if err != nil {
				t.Fatalf("Speak() error = %v", err)
			}
			if !bytes.Equal(a.Data, tc.data) || a.ContentType != ContentType {
				t.Errorf("got %d bytes (%q), want the %d bytes of the answer untouched", len(a.Data), a.ContentType, len(tc.data))
			}
		})
	}
}

func TestGeminiStillWrapsPCMAndNeverWrapsAWAVTwice(t *testing.T) {
	t.Parallel()
	pcm := []byte{1, 0, 2, 0}
	a, err := speakWith(t, "audio/L16;codec=pcm;rate=24000", pcm)
	if err != nil {
		t.Fatalf("Speak() error = %v", err)
	}
	if !bytes.Equal(a.Data, wav(pcm, 24000)) || string(a.Data[:4]) != "RIFF" || bytes.Count(a.Data, []byte("RIFF")) != 1 {
		t.Errorf("PCM answer = % x, want one 44-byte header and the samples", a.Data)
	}
	whole, err := speakWith(t, "audio/wav", riff(24000))
	if err != nil {
		t.Fatalf("Speak() error = %v", err)
	}
	if bytes.Count(whole.Data, []byte("RIFF")) != 1 || len(whole.Data) != 44+6 {
		t.Errorf("WAV answer has %d bytes and %d RIFF headers, want 50 and one", len(whole.Data), bytes.Count(whole.Data, []byte("RIFF")))
	}
}

func TestGeminiRefusesAWAVThatIsTooSmallOrTooBig(t *testing.T) {
	t.Parallel()
	if _, err := speakWith(t, "audio/wav", []byte("RIFF")); !errors.Is(err, ErrNoAudio) {
		t.Errorf("a WAV without a header: error = %v, want ErrNoAudio", err)
	}
	big := append(riff(24000), make([]byte, maxAudioBytes)...)
	if _, err := speakWith(t, "audio/wav", big); !errors.Is(err, ErrNoAudio) {
		t.Errorf("a WAV over the limit: error = %v, want ErrNoAudio", err)
	}
}
