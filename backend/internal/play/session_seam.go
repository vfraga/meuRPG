package play

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/sessiontell"
	"github.com/PuraFome/meuRPG/backend/internal/play/live"
)

var (
	_ sessiontell.Tell = (*SessionTell)(nil)
	_ sessiontell.Host = (*Service)(nil)
)

// SessionTell is what a write of another module may tell the live streams once it commits, through InSession. It offers a few hints and
// nothing else: the hub and the number stay in this package. The zero value (a write with no open session) tells nothing.
type SessionTell struct {
	s          *Service
	h          *hints
	campaignID string
	sessionID  string
	// plain collects the steps (After, a combat that changed) for a write with no open session: they run after the commit and send with no number.
	plain *hints
}

// steps is where the steps that go out after the commit are collected: the session's hints, or, for a write with no open session, the
// collector whose steps go out with no number. The zero value has none.
func (t *SessionTell) steps() *hints {
	if t.h != nil {
		return t.h
	}
	return t.plain
}

// Touch marks a write that sends no event but changes what the screens show: the session's number moves, and a stream that missed the
// change reads again at its next beat.
func (t *SessionTell) Touch() {
	if t.h != nil {
		t.h.touch()
	}
}

// CreaturesChanged tells the master and the owner's player that their creature lists changed.
func (t *SessionTell) CreaturesChanged(ownerUserID string) {
	if t.h != nil {
		t.h.tell(t.campaignID, creaturesChangedEvent(ownerUserID))
	}
}

// EncounterChanged tells the streams that a combat changed (the combat as each reader sees it, its log for the master, the vitals of
// the table and the fog), as they stand once the write committed. An empty id tells nothing.
func (t *SessionTell) EncounterChanged(encounterID string) {
	if h := t.steps(); h != nil && encounterID != "" {
		h.touch() // the hints are sent after the commit, so the write moves the number itself (with no open session there is none to move)
		t.s.tellEncounterAfter(h, t.campaignID, encounterID)
	}
}

// CombatLogChangedFull tells the master, and only the master, that the log of the combat in progress changed in a way that rewrites how
// its lines read (the table's rules decide what a line shows), so the app drops the log it holds and reads again. A player hears
// nothing kind-specific: the write's number is all their stream shows. It touches, and tells nothing when no combat is in progress.
func (t *SessionTell) CombatLogChangedFull() {
	h := t.steps()
	if h == nil {
		return
	}
	h.touch()
	h.after(func(ctx context.Context, say sayFn) {
		enc, err := t.s.queries.GetOpenEncounter(ctx, t.sessionID)
		if err != nil {
			return // no combat in progress: no log to read again
		}
		say(t.campaignID, live.Event{Audience: live.Audience{Master: true}, Message: combatLogChangedMessage(enc.ID, true)})
	})
}

// later collects fn for after the commit, for a write with no open session too (it then goes out with no number). It moves nothing by
// itself. A SessionTell with nothing to collect into (the zero value) drops it.
func (t *SessionTell) later(fn func(ctx context.Context, say sayFn)) {
	if h := t.steps(); h != nil {
		h.after(fn)
	}
}

// tellOf is the SessionTell a host method of the characters module was given by InSession. Any other Tell (a module with no play
// funnel connected) tells nothing.
func tellOf(t sessiontell.Tell) *SessionTell {
	if st, ok := t.(*SessionTell); ok && st != nil {
		return st
	}
	return &SessionTell{}
}

// XPChanged tells every stream of the campaign that the party, its levels or its experience changed. The write moves the number.
func (t *SessionTell) XPChanged() {
	t.touchSteps()
	t.later(func(_ context.Context, say sayFn) { say(t.campaignID, xpChangedEvent()) })
}

// VitalsChanged tells the master and the character's player the character's vitals as they stand once the transaction committed.
func (t *SessionTell) VitalsChanged(characterID string) {
	t.touchSteps()
	t.later(func(ctx context.Context, say sayFn) { t.s.tellVitalsNow(ctx, say, t.campaignID, characterID) })
}

// CharacterEvent tells the master and the owner that a review changed or a dead character lives again.
func (t *SessionTell) CharacterEvent(ownerUserID, characterID, event string) {
	t.touchSteps()
	t.later(func(ctx context.Context, say sayFn) {
		t.s.tellCharacterEvent(ctx, say, t.campaignID, ownerUserID, characterID, event)
	})
}

// touchSteps marks the write as one that moves the number, whether or not the campaign has an open session (with none there is no
// number, and the collector only keeps the steps).
func (t *SessionTell) touchSteps() {
	if h := t.steps(); h != nil {
		h.touch()
	}
}

// After runs fn once the transaction committed, after the events told; what fn sends through out carries the session's number (none, with
// no open session). It moves nothing by itself: the write touches.
func (t *SessionTell) After(fn func(ctx context.Context, out sessiontell.Out)) {
	h := t.steps()
	if h == nil {
		return
	}
	h.after(func(ctx context.Context, say sayFn) {
		fn(ctx, &sessionOut{s: t.s, campaignID: t.campaignID, say: say})
	})
}

// Replay marks the call as the repeat of a write already made: the number stays, and what the steps of After send carries it.
func (t *SessionTell) Replay() {
	if t.h != nil {
		t.h.replay()
	}
}

// sessionOut is the Out of a step of After: it sends through say, which stamps the number.
type sessionOut struct {
	s          *Service
	campaignID string
	say        sayFn
}

// Send tells ev to the audience named, one message for each stream group, each made from ev so that the number is written into a
// message no stream shares yet.
func (o *sessionOut) Send(ctx context.Context, to sessiontell.Audience, msg proto.Message) {
	ev, ok := msg.(*playv1.WatchGameSessionResponse)
	if !ok {
		return // only the stream's own message is told
	}
	mk := func() *playv1.WatchGameSessionResponse {
		m, _ := proto.Clone(ev).(*playv1.WatchGameSessionResponse)
		return m
	}
	if to.Master || to.Players {
		o.say(o.campaignID, live.Event{Audience: live.Audience{Master: to.Master, Players: to.Players}, Message: mk()})
	}
	for _, id := range to.Users {
		o.say(o.campaignID, live.Event{Audience: live.Audience{UserID: id}, Message: mk(), Coalesce: to.Coalesce})
	}
	if to.Scene {
		v, open, err := o.s.sceneViewersNow(ctx, o.campaignID)
		if err != nil {
			o.s.logger.ErrorContext(ctx, "play: cannot read the players of the scene", "error", err)
			v = viewers{} // the master alone hears it
		} else if !open {
			return
		}
		sayTo(o.say, o.campaignID, true, v.audiences(), mk)
	}
}

// InSession runs a write of another module that depends on the campaign's session, under the funnel: it takes the open session's row
// first, runs fn on that transaction (fn writes its own tables and tells through t), and, once the transaction committed, moves the
// session's number once if fn told or touched and sends what fn told, stamped with it. fn's error rolls everything back and tells
// nothing. When the campaign has no open session, fn runs in a plain transaction, t moves no number and the steps fn collected go out after the commit with none; a session that opens between the unlocked read and the commit is not locked by that write, and its screens read the change at their next Ready or read.
func (s *Service) InSession(ctx context.Context, campaignID string, fn func(tx pgx.Tx, t sessiontell.Tell) error) error {
	for try := 1; ; try++ {
		open, err := s.queries.GetOpenGameSession(ctx, campaignID)
		if errors.Is(err, pgx.ErrNoRows) {
			return s.inNoSession(ctx, campaignID, fn)
		}
		if err != nil {
			return err
		}
		if s.afterSessionRead != nil {
			s.afterSessionRead(campaignID) // a test changes the session here
		}
		err = s.tx(ctx, open.ID, func(tx pgx.Tx, h *hints) error {
			if _, err := stillOpen(ctx, s.queries.WithTx(tx), campaignID, open.ID); err != nil {
				return errSessionMoved // the lock is held: nothing was written
			}
			return fn(tx, &SessionTell{s: s, h: h, campaignID: campaignID, sessionID: open.ID})
		})
		if !errors.Is(err, errSessionMoved) {
			return err
		}
		if try >= maxSessionTries {
			return connect.NewError(connect.CodeAborted, errors.New("the session keeps changing: try again"))
		}
	}
}

// inNoSession is the write of InSession when the campaign has no open session: a plain transaction, with nothing to move; every step the write collected (After, and the hints that are steps: a combat that changed) still goes out after the commit, with no number. The hints that are events (CreaturesChanged) tell nothing: no stream is open. A session that opens between the unlocked read and the commit is not locked by it.
func (s *Service) inNoSession(ctx context.Context, campaignID string, fn func(tx pgx.Tx, t sessiontell.Tell) error) error {
	var plain hints
	if err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		plain = hints{}
		return fn(tx, &SessionTell{s: s, campaignID: campaignID, plain: &plain})
	}); err != nil {
		return err
	}
	// No session is open, so there is no number to move; what the write collected for after the commit goes out with no number.
	if len(plain.afters) > 0 {
		pctx, stop := afterCommit(ctx)
		defer stop()
		for _, step := range plain.afters {
			step(pctx, s.sayNow)
		}
	}
	return nil
}
