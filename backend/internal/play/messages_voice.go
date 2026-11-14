package play

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/blob"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/play/voice"
)

// audioKey is where the voice of a line is kept: under the campaign's prefix, so it goes with the
// campaign's files.
func audioKey(campaignID, messageID string) string {
	return "campaigns/" + campaignID + "/voice/" + messageID
}

// voiceError is an answer of the audio route, with the code and reason the app reads.
type voiceError struct {
	status  int
	code    connect.Code
	reason  string
	message string
}

func (e *voiceError) Error() string { return e.message }

// errNoAudio is the 404 of everything the caller may not hear, and of what does not exist: one answer
// for all, so the route says nothing about a line (RN-10, RN-36).
var errNoAudio = &voiceError{status: http.StatusNotFound, code: connect.CodeNotFound, message: "audio not found"}

var (
	errVoiceOff = &voiceError{status: http.StatusPreconditionFailed, code: connect.CodeFailedPrecondition, reason: "OFF", message: "the voice is off"}
	errVoiceCap = &voiceError{status: http.StatusTooManyRequests, code: connect.CodeResourceExhausted, reason: "DAILY_LIMIT_REACHED", message: "the day's limit of voices was reached"}
	errVoiceBad = &voiceError{status: http.StatusServiceUnavailable, code: connect.CodeUnavailable, reason: "UNAVAILABLE", message: "the voice could not be made; try again"}
	errVoiceNot = &voiceError{status: http.StatusUnprocessableEntity, code: connect.CodeFailedPrecondition, reason: "REFUSED", message: "the voice service refused this text"}
)

// writeVoiceError answers err as JSON {code, reason, message}; anything that is not a voiceError is
// logged and answered as unavailable, and never says why.
func (s *Service) writeVoiceError(w http.ResponseWriter, r *http.Request, err error) {
	ve, ok := errors.AsType[*voiceError](err)
	if !ok {
		if ce, isConnect := errors.AsType[*connect.Error](err); isConnect && ce.Code() != connect.CodeUnavailable {
			ve = &voiceError{status: connectStatus(ce.Code()), code: ce.Code(), message: ce.Message()}
		} else {
			s.logger.ErrorContext(r.Context(), "play: the audio route failed", "error", err)
			ve = errVoiceBad
		}
	}
	h := w.Header()
	h.Set("Content-Type", "application/json")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(ve.status)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": ve.code.String(), "reason": ve.reason, "message": ve.message})
}

func connectStatus(c connect.Code) int {
	switch c {
	case connect.CodeUnauthenticated:
		return http.StatusUnauthorized
	case connect.CodeResourceExhausted:
		return http.StatusTooManyRequests
	default:
		return http.StatusNotFound
	}
}

// withRequestSession finds the caller's session for a plain request, like the Connect interceptor.
func (s *Service) withRequestSession(as interface {
	AuthenticateRequest(r *http.Request) (context.Context, error)
}, next http.Handler,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, err := as.AuthenticateRequest(r)
		if err != nil {
			s.writeVoiceError(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// limitAudio applies the per-user limit of requests for audio. A caller with no session passes: the
// IP limit covered it and the handler refuses it.
func (s *Service) limitAudio(sessions Sessions, next http.Handler) http.Handler {
	if s.msgs.audio == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if userID, err := sessions.UserID(r.Context()); err == nil {
			if ok, wait := s.msgs.audio.Allow(userID); !ok {
				s.msgs.audioHit.Hit(r.Context())
				w.Header().Set("Retry-After", fmt.Sprint(int(wait/time.Second)+1))
				s.writeVoiceError(w, r, &voiceError{status: http.StatusTooManyRequests, code: connect.CodeResourceExhausted, reason: "RATE_LIMITED", message: "too many requests, please wait a moment and try again"})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// handleAudio serves GET /session-messages/{id}/audio, the voice of a line, to the line's readers only
// (RN-36). Every reason to refuse (no session, not a member, a line that does not exist or that is
// not for the caller, a line players wrote) is the same 404 as for a line that does not exist, and the
// check comes before anything is made or opened. The first reader to ask makes the voice; later
// requests, from anyone who may read the line, are served from the stored file.
func (s *Service) handleAudio(w http.ResponseWriter, r *http.Request) {
	if err := s.serveAudio(w, r); err != nil {
		s.writeVoiceError(w, r, err)
	}
}

func (s *Service) serveAudio(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	userID, err := authz.RequireSignedIn(ctx)
	if err != nil {
		return err
	}
	row, err := s.audibleMessage(ctx, r.PathValue("id"), userID)
	if err != nil {
		return err
	}
	if !s.msgs.voiceOn() {
		return errVoiceOff
	}
	obj, err := s.openVoice(ctx, row)
	if err != nil {
		return err
	}
	defer func() { _ = obj.Close() }()
	h := w.Header()
	h.Set("Content-Type", obj.ContentType)
	h.Set("Cache-Control", "private, no-cache") // asked again every time: the answer is the readers' alone
	h.Set("Vary", "Cookie")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("ETag", `"`+row.ID+`"`) // a line's voice never changes
	http.ServeContent(w, r, "", time.Time{}, obj.Content)
	return nil
}

// audibleMessage is the line if the caller may read it and it can be heard, else errNoAudio.
func (s *Service) audibleMessage(ctx context.Context, rawID, userID string) (playdb.SessionMessage, error) {
	id, err := uuid.Parse(rawID)
	if err != nil {
		return playdb.SessionMessage{}, errNoAudio
	}
	line, err := s.queries.GetSessionMessage(ctx, id.String())
	if err != nil {
		return playdb.SessionMessage{}, errNoAudio
	}
	m, err := authz.RequireCampaignMember(ctx, line.CampaignID)
	if err != nil {
		if connect.CodeOf(err) == connect.CodeUnavailable {
			return playdb.SessionMessage{}, err
		}
		return playdb.SessionMessage{}, errNoAudio
	}
	row, err := s.readableMessage(ctx, m.CampaignID, line.ID, viewer{master: m.Role == authz.RoleMaster, userID: userID})
	if err != nil || (row.SpeakerKind != speakerNarrator && row.SpeakerKind != speakerNPC) {
		return playdb.SessionMessage{}, errNoAudio
	}
	return row, nil
}

// openVoice opens the stored voice of the line, making it first when nobody asked before. One voice is
// made at a time per line: a second request waits for the first instead of paying twice.
func (s *Service) openVoice(ctx context.Context, row playdb.SessionMessage) (*blob.Object, error) {
	key := audioKey(row.CampaignID, row.ID)
	if obj, err := s.msgs.blobs.Open(ctx, key); err == nil {
		return obj, nil
	} else if !errors.Is(err, blob.ErrNotFound) {
		return nil, fmt.Errorf("open the voice: %w", err)
	}
	// The voice is made for the line, not for the request that asked: the others who wait for it
	// still want it. It has a timeout of its own and stops when the last of them leaves.
	err := s.msgs.flights.wait(ctx, row.ID, voiceTailTimeout, func(runCtx context.Context) error {
		return s.makeVoice(runCtx, row)
	})
	if err != nil {
		return nil, err
	}
	obj, err := s.msgs.blobs.Open(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("open the voice just made: %w", err)
	}
	return obj, nil
}

// makeVoice makes and stores the voice of a line. The gate lets one voice be made at a time on this
// server, so the day's count and the call to the model take turns (the same line is not made twice:
// the first thing it does is look for the file).
func (s *Service) makeVoice(ctx context.Context, row playdb.SessionMessage) error {
	s.msgs.gate.Lock()
	defer s.msgs.gate.Unlock()
	if err := ctx.Err(); err != nil {
		return err // nobody waits for it any more, or its time is up: do not start the call
	}
	key := audioKey(row.CampaignID, row.ID)
	if obj, err := s.msgs.blobs.Open(ctx, key); err == nil {
		_ = obj.Close()
		return nil // another request made it while this one waited
	}
	if err := s.checkVoiceCap(ctx); err != nil {
		return err
	}
	name := voice.Narrator
	if row.SpeakerKind == speakerNPC && row.SpeakerRef != nil {
		name = voice.ForNPC(*row.SpeakerRef)
	}
	audio, err := s.msgs.voice.Speak(ctx, row.Text, name)
	switch {
	case errors.Is(err, voice.ErrRefused):
		return errVoiceNot
	case err != nil:
		s.logger.WarnContext(ctx, "play: the voice of a line was not made", "model", s.msgs.voice.Model(), "kind", row.SpeakerKind)
		return errVoiceBad
	}
	if err := s.msgs.blobs.Put(ctx, key, audio.ContentType, bytes.NewReader(audio.Data)); err != nil {
		return fmt.Errorf("store the voice: %w", err)
	}
	return s.markVoiced(ctx, row)
}

// markVoiced records that the line has its voice, which the chat screen shows. It is a write of the session like any other: it runs in
// a transaction of its own, after the slow call and the store (the session's lock is never held across them), and moves the number
// once so every screen reads the line again. A line already marked (another maker was first) moves nothing; with no open session it
// runs plain.
func (s *Service) markVoiced(ctx context.Context, row playdb.SessionMessage) error {
	return s.txOrNone(ctx, row.CampaignID, func(tx pgx.Tx, h *hints, _ string) error {
		n, err := s.queries.WithTx(tx).MarkSessionMessageVoiced(ctx, playdb.MarkSessionMessageVoicedParams{ID: row.ID, AudioMadeAt: timeRef(s.now())})
		if err != nil {
			return fmt.Errorf("mark the line voiced: %w", err)
		}
		if n > 0 {
			h.touch()
		}
		return nil
	})
}

// checkVoiceCap refuses a new voice once the server made its day's limit (Brasília days, as the images).
func (s *Service) checkVoiceCap(ctx context.Context) error {
	n, err := s.queries.CountSessionMessagesFrom(ctx, timeRef(voiceDayStart(s.now())))
	if err != nil {
		return fmt.Errorf("count the day's voices: %w", err)
	}
	if int(n) >= s.msgs.dailyCap {
		return errVoiceCap
	}
	return nil
}

// brazil is the time zone of the day's count: Brazil has had no daylight saving time since 2019.
var brazil = time.FixedZone("BRT", -3*60*60)

// voiceDayStart is the first instant of t's day in Brazil's time, where the daily cap starts over.
func voiceDayStart(t time.Time) time.Time {
	t = t.In(brazil)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, brazil)
}

func timeRef(t time.Time) *time.Time { return &t }
