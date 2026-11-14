package progression

import (
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5/pgxpool"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	progressionv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/progression/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dbtest"
)

// The lock order of the writes that change what a live session shows (#735 part 3): the open session's row first, as play's funnel
// takes it, and then the sheets and the campaign. An award or an undo that locked a sheet and only then reached the session (at its
// event) would hold the sheet while it waits for the session, and a combat write that holds the session and reaches for the same
// sheet would each wait for what the other holds (40P01, retried only after the database's deadlock timeout).
//
// The test holds the session's row from another connection, starts the write and waits until it is stopped at that row. Whatever
// the write took before that, it holds now: the sheets and the campaign must be free to lock.

// stoppedAtTheSession runs write while the session's row is held by another connection and, once the write is waiting for it, calls
// free; then it releases the row and the write must end without an error.
func stoppedAtTheSession(t *testing.T, tb *table, write func() error, free func(t *testing.T, probe *pgxpool.Pool)) {
	t.Helper()
	ctx := t.Context()
	sessionID := tb.startSession(t)
	// The write's own connections, to tell its statements from other tests' on the same server.
	var backends []string
	var held []interface{ Release() }
	for range 2 {
		c, err := tb.h.pool.Acquire(ctx)
		if err != nil {
			t.Fatalf("Acquire() error = %v", err)
		}
		id, err := dbtest.BackendID(ctx, c)
		if err != nil {
			t.Fatalf("BackendID() error = %v", err)
		}
		backends = append(backends, id)
		held = append(held, c)
	}
	for _, c := range held {
		c.Release()
	}
	holder, err := dbtest.SideConnection(t, tb.h.pool).Begin(ctx)
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	defer func() { _ = holder.Rollback(t.Context()) }()
	if _, err := holder.Exec(ctx, `SELECT id FROM game_sessions WHERE id = $1 FOR UPDATE`, sessionID); err != nil {
		t.Fatalf("lock the session: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- write() }()
	// Any statement on game_sessions will do: on CockroachDB the write waits already at the plain read of the open session that
	// InSession makes first (a plain read of a row another transaction holds FOR UPDATE waits), before the statement that locks it. In
	// either case the write has taken nothing else, and the old order (the sheets first, the session's row after) takes the sheets
	// before its first statement on game_sessions, so the sheets are held when it is seen: the check that they are free is what fails.
	observer := dbtest.SideConnection(t, tb.h.pool)
	for deadline := time.Now().Add(30 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		n, err := dbtest.WaitingStatements(ctx, observer, `%game_sessions%`, backends...)
		if err != nil {
			t.Fatalf("look for the waiting write: %v", err)
		}
		if n > 0 {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("the write ended (%v) while the session's row was held", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("the write never waited for the session's row")
		}
	}
	free(t, dbtest.SideConnection(t, tb.h.pool))
	if err := holder.Rollback(ctx); err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}
	if err := <-done; err != nil {
		t.Errorf("the write after the session's row was released: %v", err)
	}
}

// notLocked fails the test if a row the write should not hold yet is locked: it asks for the row without waiting.
func notLocked(t *testing.T, probe *pgxpool.Pool, what, query string, id string) {
	t.Helper()
	tx, err := probe.Begin(t.Context())
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	defer func() { _ = tx.Rollback(t.Context()) }()
	if _, err := tx.Exec(t.Context(), query, id); err != nil {
		t.Errorf("%s is locked while the write waits for the session's row (the session's row comes first): %v", what, err)
	}
}

// An award takes the session's row before the sheets and the campaign.
func TestAnAwardTakesTheSessionBeforeTheSheets(t *testing.T) {
	t.Parallel()
	dbtest.PoolSize(t, 5)
	tb := newTable(t, gold, 2)
	stoppedAtTheSession(t, tb, func() error {
		_, err := tb.master.award(tb.campaign, func(r *progressionv1.AwardXPRequest) {
			r.Mode, r.Amount, r.CharacterIds = progressionv1.XPAwardMode_XP_AWARD_MODE_MANUAL, 10, tb.ids(2)
		})
		return err
	}, tb.sheetsAndCampaignAreFree)
}

// An undo takes the session's row before the sheets and the campaign.
func TestAnUndoTakesTheSessionBeforeTheSheets(t *testing.T) {
	t.Parallel()
	dbtest.PoolSize(t, 5)
	tb := newTable(t, gold, 2)
	tb.master.manual(t, tb.campaign, 10, tb.ids(2)...)
	stoppedAtTheSession(t, tb, func() error {
		_, err := tb.master.undo(tb.campaign, newKey())
		return err
	}, tb.sheetsAndCampaignAreFree)
}

// endLiveSession ends the campaign's open session, as the master.
func (u *user) endLiveSession(t *testing.T, campaignID string) {
	t.Helper()
	live, err := u.play.GetLiveSession(t.Context(), connect.NewRequest(&playv1.GetLiveSessionRequest{CampaignId: campaignID}))
	if err != nil {
		t.Fatalf("GetLiveSession() error = %v", err)
	}
	if _, err := u.play.EndGameSession(t.Context(), connect.NewRequest(&playv1.EndGameSessionRequest{CampaignId: campaignID, GameSessionId: live.Msg.GetGameSession().GetId()})); err != nil {
		t.Fatalf("EndGameSession() error = %v", err)
	}
}

// The session's number moves once for an award and once for its undo while a session is open, and never for a retry of a key.
// (An award with no session open has no number to read, so it is not asserted here.)
func TestAnAwardAndItsUndoMoveTheSessionsNumberOnce(t *testing.T) {
	t.Parallel()
	tb := newTable(t, gold, 2)
	seq := func(id string) int64 {
		t.Helper()
		var n int64
		if err := tb.h.pool.QueryRow(t.Context(), `SELECT hint_seq FROM game_sessions WHERE id = $1`, id).Scan(&n); err != nil {
			t.Fatalf("read the number: %v", err)
		}
		return n
	}
	before := func() { tb.master.manual(t, tb.campaign, 10, tb.ids(2)...) } // no session open: nothing to move
	before()
	session := tb.startSession(t)
	start := seq(session)
	key := newKey()
	give := func() {
		t.Helper()
		if _, err := tb.master.award(tb.campaign, func(r *progressionv1.AwardXPRequest) {
			r.Mode, r.Amount, r.CharacterIds, r.IdempotencyKey = progressionv1.XPAwardMode_XP_AWARD_MODE_MANUAL, 10, tb.ids(2), key
		}); err != nil {
			t.Fatalf("AwardXP() error = %v", err)
		}
	}
	give()
	if got := seq(session); got != start+1 {
		t.Errorf("the number = %d after an award, want %d", got, start+1)
	}
	give() // the retry of the key
	if got := seq(session); got != start+1 {
		t.Errorf("the number = %d after the retry of the key, want %d", got, start+1)
	}
	if _, err := tb.master.undo(tb.campaign, newKey()); err != nil {
		t.Fatalf("UndoLastXPAward() error = %v", err)
	}
	if got := seq(session); got != start+2 {
		t.Errorf("the number = %d after the undo, want %d", got, start+2)
	}
}

// sheetsAndCampaignAreFree is what the write that waits for the session's row must not hold yet.
func (tb *table) sheetsAndCampaignAreFree(t *testing.T, probe *pgxpool.Pool) {
	t.Helper()
	for _, c := range tb.pcs {
		notLocked(t, probe, "a sheet", `SELECT id FROM characters WHERE id = $1 FOR UPDATE NOWAIT`, c.GetId())
	}
	notLocked(t, probe, "the campaign", `SELECT id FROM campaigns WHERE id = $1 FOR UPDATE NOWAIT`, tb.campaign)
}
