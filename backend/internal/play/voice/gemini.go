package voice

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuraFome/meuRPG/backend/internal/platform/config"
)

// GeminiBase is the Gemini API's endpoint of the models; the model's name and ":generateContent" follow.
// generateContent keeps no interaction on our account (docs/privacy.md). The request and answer
// follow https://ai.google.dev/gemini-api/docs/speech-generation: the audio comes as base64 16-bit
// PCM, mono, 24 kHz, in the first part of the first candidate.
const GeminiBase = "https://generativelanguage.googleapis.com/v1beta/models/"

// maxResponseBytes bounds one answer: a 2,000-character line is under three minutes of speech, about
// 8 MiB of PCM, 11 MiB in base64.
const maxResponseBytes = 16 << 20

// maxAudioBytes bounds the audio itself once decoded (base64 makes it a third bigger on the wire).
const maxAudioBytes = 10 << 20

// defaultRate is the sample rate the model answers with when its mime type names none.
const defaultRate = 24000

// minRate and maxRate bound the rate a mime type may name: anything else is the default.
const (
	minRate = 8000
	maxRate = 192000
)

// Gemini is the Generator that calls the Gemini API with a key.
type Gemini struct {
	// Key is the API key, sent only in the x-goog-api-key header. It is a config.Secret: it prints as
	// [REDACTED], and nothing here formats it.
	Key config.Secret
	// ModelName is the text-to-speech model (GEMINI_TTS_MODEL). It has no default: the model's
	// name changes, so the operator sets the one in Google's current documentation.
	ModelName string
	// URL is the endpoint up to the model's name; empty means GeminiBase. Tests point it at httptest.
	URL string
	// Client does the calls; nil means a client of its own. Redirects are always refused, so the key
	// header never follows one.
	Client *http.Client
	// Timeout bounds one call; zero means two minutes.
	Timeout time.Duration
	// Logger gets one line per failed call: the status or the kind, never the text.
	Logger *slog.Logger
}

var _ Generator = (*Gemini)(nil)

// Model implements Generator.
func (g *Gemini) Model() string { return g.ModelName }

type speakRequest struct {
	Contents         []content        `json:"contents"`
	GenerationConfig generationConfig `json:"generationConfig"`
}

type content struct {
	Parts []part `json:"parts"`
}

type part struct {
	Text string `json:"text"`
}

type generationConfig struct {
	ResponseModalities []string     `json:"responseModalities"`
	SpeechConfig       speechConfig `json:"speechConfig"`
}

type speechConfig struct {
	VoiceConfig struct {
		PrebuiltVoiceConfig struct {
			VoiceName string `json:"voiceName"`
		} `json:"prebuiltVoiceConfig"`
	} `json:"voiceConfig"`
}

// Speak implements Generator.
func (g *Gemini) Speak(ctx context.Context, text, voice string) (Audio, error) {
	if g.ModelName == "" || strings.TrimSpace(text) == "" {
		return Audio{}, ErrNoAudio
	}
	var req speakRequest
	req.Contents = []content{{Parts: []part{{Text: text}}}}
	req.GenerationConfig.ResponseModalities = []string{"AUDIO"}
	req.GenerationConfig.SpeechConfig.VoiceConfig.PrebuiltVoiceConfig.VoiceName = voice
	body, err := json.Marshal(req)
	if err != nil {
		return Audio{}, fmt.Errorf("encode the request: %w", err)
	}
	base := g.URL
	if base == "" {
		base = GeminiBase
	}
	timeout := g.Timeout
	if timeout == 0 {
		timeout = 2 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+url.PathEscape(g.ModelName)+":generateContent", bytes.NewReader(body))
	if err != nil {
		return Audio{}, fmt.Errorf("build the request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", string(g.Key))
	res, err := g.client().Do(httpReq)
	if err != nil {
		g.log(ctx, "the call failed", "cause", causeOf(err))
		return Audio{}, ErrUnavailable
	}
	defer func() { _ = res.Body.Close() }()
	return g.parse(ctx, res.StatusCode, io.LimitReader(res.Body, maxResponseBytes))
}

// causeOf is why a call failed, without the URL a *url.Error carries: the transport's own error never
// holds the text (it travels in the body) or the key (it travels in a header).
func causeOf(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) && ue.Err != nil {
		return ue.Err.Error()
	}
	return err.Error()
}

func (g *Gemini) client() *http.Client {
	if g.Client != nil {
		return g.Client
	}
	return &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func (g *Gemini) log(ctx context.Context, what string, attrs ...any) {
	if g.Logger != nil {
		g.Logger.WarnContext(ctx, "voice: the model call failed", append([]any{"what", what}, attrs...)...)
	}
}

type speakAnswer struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				InlineData *struct {
					MimeType string `json:"mimeType"`
					Data     string `json:"data"`
				} `json:"inlineData"`
			} `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	PromptFeedback struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback"`
}

var rateOf = regexp.MustCompile(`rate=(\d{4,6})`)

// parse reads an answer: the audio of the first candidate, or the reason there is none.
func (g *Gemini) parse(ctx context.Context, status int, r io.Reader) (Audio, error) {
	if status != http.StatusOK {
		g.log(ctx, "the service answered an error", "status", status)
		return Audio{}, ErrUnavailable
	}
	var ans speakAnswer
	if err := json.NewDecoder(r).Decode(&ans); err != nil {
		g.log(ctx, "the answer cannot be read")
		return Audio{}, ErrUnavailable
	}
	if ans.PromptFeedback.BlockReason != "" {
		g.log(ctx, "the text was refused", "reason", ans.PromptFeedback.BlockReason)
		return Audio{}, ErrRefused
	}
	for _, c := range ans.Candidates {
		for _, p := range c.Content.Parts {
			if p.InlineData == nil || p.InlineData.Data == "" {
				continue
			}
			return audioOf(p.InlineData.MimeType, p.InlineData.Data)
		}
	}
	g.log(ctx, "the answer holds no audio")
	return Audio{}, ErrNoAudio
}

// audioOf makes the audio of an inline part: a WAV file as it came, or raw PCM in a header.
func audioOf(mime, encoded string) (Audio, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(data) < 2 || len(data) > maxAudioBytes {
		return Audio{}, ErrNoAudio
	}
	if !isWAV(mime, data) {
		return Audio{Data: wav(data[:len(data)&^1], sampleRate(mime)), ContentType: ContentType}, nil
	}
	// The newer models answer with a WAV file: it is served as it came, never wrapped again.
	if len(data) <= wavHeaderSize {
		return Audio{}, ErrNoAudio
	}
	return Audio{Data: data, ContentType: ContentType}, nil
}

// isWAV says the answer is a WAV file already: its mime type says so, or the bytes start with a RIFF header
// (the older models answered raw 16-bit PCM, "audio/L16;codec=pcm;rate=...", which wav() wraps).
func isWAV(mime string, data []byte) bool {
	kind, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(mime)), ";")
	return kind == "audio/wav" || kind == "audio/x-wav" || kind == "audio/wave" || bytes.HasPrefix(data, []byte("RIFF"))
}

// sampleRate is the rate a mime type such as "audio/L16;codec=pcm;rate=24000" names, or the default.
func sampleRate(mime string) uint32 {
	if m := rateOf.FindStringSubmatch(mime); m != nil {
		if n, err := strconv.ParseUint(m[1], 10, 32); err == nil && n >= minRate && n <= maxRate {
			return uint32(n)
		}
	}
	return defaultRate
}
