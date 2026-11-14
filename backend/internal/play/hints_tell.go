package play

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/sessiontell"
	"github.com/PuraFome/meuRPG/backend/internal/play/live"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// sayFn is what an after-commit step of Service.tx sends through (hints.after): it stamps the event with the session's number.
type sayFn = func(campaignID string, ev live.Event)

// sayNow sends without a number, for the callers that tell the streams outside Service.tx (the calls of other modules after
// their own commit).
func (s *Service) sayNow(campaignID string, ev live.Event) { s.send(campaignID, ev) }

// sayWith is the say that stamps each event with seq (WatchGameSessionResponse.session_seq) and sends it; seq 0 sends it unstamped.
// It stamps a copy and never the caller's message: a publisher may send one message to several audiences, and a stream may be
// marshaling an earlier send of it.
func (s *Service) sayWith(seq int64) sayFn {
	return func(campaignID string, ev live.Event) {
		if seq > 0 && ev.Message != nil {
			ev.Message = proto.CloneOf(ev.Message)
			ev.Message.SessionSeq = seq
		}
		s.send(campaignID, ev)
	}
}

type sayKey struct{}

// withSay returns ctx carrying say, for the publishers of a combat handler's publish callback: they take only a context, and
// sayOf hands them the say that stamps with the number of the write.
func withSay(ctx context.Context, say sayFn) context.Context {
	return context.WithValue(ctx, sayKey{}, say)
}

// sayOf is the say a publisher called with ctx sends through: the one withSay put there, else sayNow (no number).
func (s *Service) sayOf(ctx context.Context) sayFn {
	if say, ok := ctx.Value(sayKey{}).(sayFn); ok {
		return say
	}
	return s.sayNow
}

// txOpen is Service.tx on the campaign's open session: the session is read first, with no lock, so tx can take its row lock as the
// first statement of the attempt, and fn is given it after the check that it is still the open one (stillOpen). It refuses with
// NO_OPEN_SESSION when the campaign has none.
func (s *Service) txOpen(ctx context.Context, campaignID string, fn func(tx pgx.Tx, h *hints, session playdb.GameSession) error) error {
	return s.txOpenOr(ctx, campaignID, errNoOpenSession, fn)
}

// txOpenOr is txOpen for a call that refuses a campaign with no open session with its own error (the puzzles' own reason).
func (s *Service) txOpenOr(ctx context.Context, campaignID string, none func() error, fn func(tx pgx.Tx, h *hints, session playdb.GameSession) error) error {
	open, err := s.queries.GetOpenGameSession(ctx, campaignID)
	if errors.Is(err, pgx.ErrNoRows) {
		return none()
	}
	if err != nil {
		return fmt.Errorf("find the open session: %w", err)
	}
	if s.afterSessionRead != nil {
		s.afterSessionRead(campaignID) // a test changes the session here
	}
	return s.tx(ctx, open.ID, func(tx pgx.Tx, h *hints) error {
		session, err := stillOpenOr(ctx, s.queries.WithTx(tx), campaignID, open.ID, none)
		if err != nil {
			return err
		}
		return fn(tx, h, session)
	})
}

// txOrNone is the write of a call that needs no session but moves the screens of one that is open: it takes the open session's
// row first and tells through h, like txOpen, or, when the campaign has none, runs fn in a plain transaction whose hints are
// dropped (sessionID is "" then). A session that opens between the unlocked read and the commit is not locked by that write, and its
// screens read the change at their next Ready or read. A session that closes in between makes the write go again.
func (s *Service) txOrNone(ctx context.Context, campaignID string, fn func(tx pgx.Tx, h *hints, sessionID string) error) error {
	for try := 1; ; try++ {
		open, err := s.queries.GetOpenGameSession(ctx, campaignID)
		if errors.Is(err, pgx.ErrNoRows) {
			return s.inNoSession(ctx, campaignID, func(tx pgx.Tx, _ sessiontell.Tell) error { return fn(tx, &hints{}, "") })
		}
		if err != nil {
			return fmt.Errorf("find the open session: %w", err)
		}
		if s.afterSessionRead != nil {
			s.afterSessionRead(campaignID) // a test changes the session here
		}
		err = s.tx(ctx, open.ID, func(tx pgx.Tx, h *hints) error {
			if _, err := stillOpen(ctx, s.queries.WithTx(tx), campaignID, open.ID); err != nil {
				return errSessionMoved // the lock is held: nothing was written
			}
			return fn(tx, h, open.ID)
		})
		if !errors.Is(err, errSessionMoved) {
			return err
		}
		if try >= maxSessionTries {
			return connect.NewError(connect.CodeAborted, errors.New("the session keeps changing: try again"))
		}
	}
}

// tellVitalsAfter collects the vitals of the characters as they stand once the transaction committed, each to the master and the
// character's player (the hint of publishVitals). A character whose vitals cannot be read is left out: the hint only says "read again".
func (s *Service) tellVitalsAfter(h *hints, campaignID string, characterIDs ...string) {
	h.after(func(ctx context.Context, say sayFn) {
		for _, id := range characterIDs {
			if v, err := s.vitals.GetVitals(ctx, campaignID, id); err == nil {
				say(campaignID, vitalsHint(v))
			}
		}
	})
}

// tellEncounterAfter collects what PublishEncounterChanged tells of a combat (the combat, its log for the master, the vitals of
// the table and the fog), once the transaction committed.
func (s *Service) tellEncounterAfter(h *hints, campaignID, encounterID string) {
	h.after(func(ctx context.Context, say sayFn) { s.tellEncounterByID(ctx, say, campaignID, encounterID) })
}

// tellEncounterRowAfter collects the hint that a combat changed (publishEncounterChanged), once the transaction committed: the
// fog is read then, so the players are told what they see of the combat as it stands.
func (s *Service) tellEncounterRowAfter(h *hints, campaignID string, enc playdb.Encounter) {
	h.after(func(ctx context.Context, say sayFn) { s.tellEncounterChanged(ctx, say, campaignID, enc) })
}

// revivifyChangedEvent is the hint that the casts of Revivify outside a combat changed, for the master and the caster's player.
func revivifyChangedEvent(casterUserID string) live.Event {
	return live.Event{
		Audience: live.Audience{Master: true, UserID: casterUserID},
		Message: &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_RevivifyChanged_{
			RevivifyChanged: &playv1.WatchGameSessionResponse_RevivifyChanged{},
		}},
	}
}

// tellEffects collects what a write of the lasting effects on characters outside a combat changed: the casts, for the master and the
// players, and the vitals the write changed, each to the master and the character's player.
func tellEffects(h *hints, campaignID string, vitals []*playv1.CharacterVitals) {
	h.tell(campaignID, castsChangedEvent(false))
	for _, v := range vitals {
		if v != nil {
			h.tell(campaignID, vitalsHint(v))
		}
	}
}

// tellEffectsAgain fills h for a retry of such a write whose key was found stored, with what the first call would have told: the casts
// and, once the transaction committed, the vitals of the characters it may have changed, as they stand, each to the master and its
// player. A write that kept who it changed passes them; one that did not (no id) tells every character's. Nothing was written, so h
// only replays.
func (s *Service) tellEffectsAgain(h *hints, campaignID string, characterIDs ...string) {
	h.replay()
	h.tell(campaignID, castsChangedEvent(false))
	if len(characterIDs) > 0 {
		s.tellVitalsAfter(h, campaignID, characterIDs...)
		return
	}
	h.after(func(ctx context.Context, say sayFn) {
		if all, err := s.vitals.ListVitals(ctx, campaignID); err == nil {
			for _, v := range all {
				say(campaignID, vitalsHint(v))
			}
		}
	})
}
