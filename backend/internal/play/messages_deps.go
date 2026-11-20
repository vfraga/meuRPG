package play

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/blob"
	"github.com/PuraFome/meuRPG/backend/internal/platform/ratelimit"
	"github.com/PuraFome/meuRPG/backend/internal/play/voice"
)

// DefaultVoiceDailyLimit is how many lines the whole server may voice per Brasília day
// (TTS_DAILY_LIMIT): a ceiling on the Gemini bill, since each line is voiced once.
const DefaultVoiceDailyLimit = 300

// DisplayNames gives the display names of accounts. The identity store implements it; play never
// reads the users table.
type DisplayNames interface {
	// DisplayNames returns the display names of the given users, keyed by user ID. A user that
	// does not exist (an erased account) is left out.
	DisplayNames(ctx context.Context, userIDs []string) (map[string]string, error)
}

// MessagesConfig configures the scene's conversation (MR-051).
type MessagesConfig struct {
	// Profiles gives the display name shown on an out-of-character line. Nil: no names.
	Profiles DisplayNames
	// Voice makes the audio of a line. Nil turns the voice off: the audio route answers
	// failed_precondition OFF and the app hides "Ouvir".
	Voice voice.Generator
	// Blobs keeps the audio next to the line. Nil turns the voice off.
	Blobs blob.Store
	// VoiceDailyLimit is the voices the whole server may make per Brasília day; zero means
	// DefaultVoiceDailyLimit.
	VoiceDailyLimit int
	// Logger receives the limiters' notices. Nil means slog.Default().
	Logger *slog.Logger
	// Send limits a user's lines, and Audio a user's requests for audio. Nil: none.
	Send, Audio *ratelimit.Limiter
}

// messageDeps is the conversation's state in the Service.
type messageDeps struct {
	profiles    DisplayNames
	voice       voice.Generator
	blobs       blob.Store
	dailyCap    int
	send, audio *ratelimit.Limiter
	sendHit     *ratelimit.Notifier
	audioHit    *ratelimit.Notifier

	typing typingBook
	// gate lets one voice be made at a time on this server, so the daily count and the call to the
	// model take turns; flights makes a second request for the same line wait for the first, and stops the work when the last reader leaves.
	gate    sync.Mutex
	flights voice.Flights
}

func newMessageDeps(cfg MessagesConfig) *messageDeps {
	d := &messageDeps{profiles: cfg.Profiles, voice: cfg.Voice, blobs: cfg.Blobs, dailyCap: cfg.VoiceDailyLimit, send: cfg.Send, audio: cfg.Audio}
	d.sendHit = ratelimit.NewNotifier(cfg.Logger, "session messages by user")
	d.audioHit = ratelimit.NewNotifier(cfg.Logger, "session audio by user")
	if d.dailyCap <= 0 {
		d.dailyCap = DefaultVoiceDailyLimit
	}
	return d
}

// voiceOn says the server can make and keep the voice of a line.
func (d *messageDeps) voiceOn() bool { return d.voice != nil && d.blobs != nil }

// typingCooldown is the shortest time between two typing hints of a campaign, and typingTTL is how
// long the app shows one.
const (
	typingCooldown = 2 * time.Second
	typingTTL      = 6 * time.Second
)

// typingBook remembers, in memory, when each campaign's master last signaled typing. The zero
// value is ready to use.
type typingBook struct {
	mu   sync.Mutex
	last map[string]time.Time
}

// take says whether a hint may go out for the campaign at now, and records it. Entries past the
// cooldown are dropped on the way.
func (b *typingBook) take(campaignID string, now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.last == nil {
		b.last = map[string]time.Time{}
	}
	for id, at := range b.last {
		if now.Sub(at) >= typingCooldown {
			delete(b.last, id)
		}
	}
	if _, waiting := b.last[campaignID]; waiting {
		return false
	}
	b.last[campaignID] = now
	return true
}

// mountVoice registers the audio route when the sessions can authenticate a plain request (the
// identity service can; a test's fake may not).
func (s *Service) mountVoice(handle func(pattern string, handler http.Handler), sessions Sessions, members authz.MembershipSource) {
	as, ok := sessions.(interface {
		AuthenticateRequest(r *http.Request) (context.Context, error)
	})
	if !ok {
		return
	}
	withAuthz := authz.Middleware(sessions, members, s.logger)
	handle("GET "+AudioPath+"{id}/audio", s.withRequestSession(as, s.limitAudio(sessions, withAuthz(http.HandlerFunc(s.handleAudio)))))
}
