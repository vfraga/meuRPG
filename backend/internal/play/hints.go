package play

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/play/live"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// send is the one place in this package that hands an event to the live hub:
// every other file tells the streams through it (the archtest in
// internal/archtest/publish_test.go fails for a call to Publish anywhere else).
// It takes the hub's arguments and passes them on unchanged, and, like the hub,
// it takes no context and waits for nobody; it reports how many subscriptions
// received the event.
func (s *Service) send(campaignID string, ev live.Event) int {
	if s.dropSend != nil && s.dropSend(campaignID, ev) {
		return 0
	}
	return s.hub.Publish(campaignID, ev)
}

// tellMembers sends ev to the master's streams, always, and to the players'
// only when players is true. It is the body of the exported Publish (the
// maps module's seam) and the call of the play files that tell the same two
// audiences.
func (s *Service) tellMembers(campaignID string, players bool, ev *playv1.WatchGameSessionResponse) {
	s.send(campaignID, live.Event{Audience: live.Audience{Master: true, Everyone: players}, Message: ev})
}

// hints collects, during one attempt of a transaction, what the write will tell the live streams once it is saved.
// Service.tx makes it and hands it to the closure; nothing in it is sent before the commit.
type hints struct {
	events  []told
	afters  []func(ctx context.Context, say func(campaignID string, ev live.Event))
	touched bool
	// replayed says the attempt found its write already made (a key used before): see replay.
	replayed bool
	// seq is the session's hint_seq as this transaction left it: set by Service.tx after it bumped the number, 0 while it did not.
	// txOn stamps the events with it (WatchGameSessionResponse.session_seq) just before it sends them.
	seq int64
}

// told is one collected event and the campaign whose streams receive it.
type told struct {
	campaignID string
	event      live.Event
}

// tell collects ev for the streams of campaignID. ev.Message must be a message made for this call, never a shared one: tx writes the
// session's number into it (WatchGameSessionResponse.session_seq) before it sends it to several streams. Those numbers are not ordered
// from one event to the next on a stream (a write that commits later may send first), so nobody may dedupe or look for gaps on them;
// only the numbers of Ready and Heartbeat are the session's. It is sent, through send, after the transaction commits, and never if the
// transaction fails. The closure may call it as often as it likes: the session's number moves by one however many it tells.
func (h *hints) tell(campaignID string, ev live.Event) {
	h.events = append(h.events, told{campaignID: campaignID, event: ev})
}

// touch marks a write that sends no event but still changes what the screens show, so the session's number moves and a
// stream that missed the change knows it is behind.
func (h *hints) touch() { h.touched = true }

// after collects a step that runs once the transaction has committed and the events told have gone out: what the hint needs is
// the committed state (a read through the pool, which would not see this transaction's rows) or another module's own sending (the
// maps module's fog). fn gets the context of the after-commit work (the request's values, without its cancellation, bounded by
// publishTimeout) and say, which stamps an event with the session's number and sends it as send does; fn must send through say and
// never through send, so its events carry the number. Steps run in the order they were collected, after the told events, and not
// at all if the transaction fails. A step moves nothing by itself: the write touches or tells.
func (h *hints) after(fn func(ctx context.Context, say func(campaignID string, ev live.Event))) {
	h.afters = append(h.afters, fn)
}

// replay marks the attempt as the repeat of a write already made: the closure found the idempotency key stored and did not write
// again. The first call may have committed and died before it told the streams, so the closure fills the collector with the
// hints that call would have sent, worked out from the stored result, and tx sends them: a hint only says "read again", so a
// second one is harmless. A replay does not move the session's number (nothing was written), whatever it tells or touches; its
// events carry the number as it stands.
func (h *hints) replay() { h.replayed = true }

// reset empties the collector: the start of every attempt calls it, so a retried attempt keeps nothing of the one the database rolled back.
func (h *hints) reset() { *h = hints{} }

// moves reports whether the write must move the session's number.
func (h *hints) moves() bool { return !h.replayed && (h.touched || len(h.events) > 0) }

// speaks reports whether the attempt has anything to send once it committed.
func (h *hints) speaks() bool { return len(h.events) > 0 || len(h.afters) > 0 }

// tx runs fn in a transaction (db.InTx, so with its retries and its rules for fn) on the session sessionID and tells the
// streams what fn collected, after the commit.
//
// The first statement of every attempt takes the session's row lock (FOR UPDATE), before fn reads anything: all the writes of
// one session take turns in that order, so they do not collide at the bump, and none can deadlock on it. fn must not lock
// the row again in another order; a closure that locks it itself (GetOpenGameSessionForUpdate) finds it already held.
// fn calls h.tell for each hint its write sends and h.touch for a write that sends none but must still move the screens.
// The collector is empty at the start of every attempt, whatever the last attempt collected.
// When fn returns nil having told or touched, tx adds 1 to game_sessions.hint_seq of sessionID in the same transaction
// (once, however many events were told), so the number is saved with the write or not at all; the session's row must exist.
// A transaction that told nothing and touched nothing takes the lock and writes nothing: the number stays as it was.
// When InTx returns nil, tx stamps each collected event with the new number and sends them, in the order they were told, then runs
// the steps of h.after. On any error, nothing is sent. A closure that finds its write already made (an idempotency key stored) calls
// h.replay and tells what the first call would have: tx sends it with the number as it stands, and does not bump.
//
// A hint that goes with no write (typing, presence, the table screen's "changed") does not go through tx: it calls send.
func (s *Service) tx(ctx context.Context, sessionID string, fn func(pgx.Tx, *hints) error) error {
	return s.txOn(ctx, s.pool, sessionID, fn)
}

// txOn is tx on any transaction starter: a test passes one whose COMMIT fails.
func (s *Service) txOn(ctx context.Context, starter db.TxStarter, sessionID string, fn func(pgx.Tx, *hints) error) error {
	var h hints
	err := db.InTx(ctx, starter, func(tx pgx.Tx) error {
		h.reset()
		q := s.queries.WithTx(tx)
		if _, err := q.LockSessionForHints(ctx, sessionID); err != nil {
			return fmt.Errorf("lock the session: %w", err)
		}
		if err := fn(tx, &h); err != nil {
			return err
		}
		if !h.moves() {
			return h.stand(ctx, q, sessionID)
		}
		seq, err := q.BumpHintSeq(ctx, sessionID)
		if err != nil {
			return fmt.Errorf("bump the session's hint number: %w", err)
		}
		h.seq = seq
		return nil
	})
	if err != nil {
		return err
	}
	s.sendTold(ctx, &h)
	return nil
}

// stand reads the number as it stands, for a replay that has something to send: it wrote nothing, so it moves nothing.
func (h *hints) stand(ctx context.Context, q *playdb.Queries, sessionID string) (err error) {
	if !h.replayed || !h.speaks() {
		return nil
	}
	if h.seq, err = q.GetHintSeq(ctx, sessionID); err != nil {
		return fmt.Errorf("read the session's hint number: %w", err)
	}
	return nil
}

// sendTold sends what the committed attempt collected: the events in the order told, then the steps of after.
func (s *Service) sendTold(ctx context.Context, h *hints) {
	say := func(campaignID string, ev live.Event) {
		if h.seq > 0 && ev.Message != nil {
			ev.Message.SessionSeq = h.seq // not shared with a stream yet: send is next
		}
		s.send(campaignID, ev)
	}
	for _, t := range h.events {
		say(t.campaignID, t.event)
	}
	if len(h.afters) > 0 {
		pctx, stop := afterCommit(ctx) // committed: a caller that hangs up must not leave the streams unheard
		defer stop()
		for _, fn := range h.afters {
			fn(pctx, say)
		}
	}
}
