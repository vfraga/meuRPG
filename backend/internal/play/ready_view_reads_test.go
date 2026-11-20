package play

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// afterFirstViewRead is a database that, right after the first of the two reads that say what became of a READIED window (the
// open windows and the waiting readied actions) has been read, runs a callback once: the test commits a pass there, between
// the view's reads, as a player's answer on another connection would.
type afterFirstViewRead struct {
	playdb.DBTX
	fired atomic.Bool
	fn    func()
}

func (d *afterFirstViewRead) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	rows, err := d.DBTX.Query(ctx, sql, args...)
	if err == nil && (strings.Contains(sql, "-- name: ListOpenReactionWindows :many") || strings.Contains(sql, "-- name: ListWaitingReadiedActions :many")) {
		return closedRows{Rows: rows, d: d}, nil
	}
	return rows, err
}

// closedRows runs the callback once its rows are read and their connection is back in the pool (a pool of one: the pass needs it).
type closedRows struct {
	pgx.Rows
	d *afterFirstViewRead
}

func (r closedRows) Close() {
	r.Rows.Close()
	if r.d.fired.CompareAndSwap(false, true) {
		r.d.fn()
	}
}

func (d *afterFirstViewRead) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return d.DBTX.Exec(ctx, sql, args...)
}

// The view a write answers with reads the combat through the pool, which is no snapshot: a pass of a READIED window that
// commits between the read of the open windows and the read of the waiting readied actions must never show the readier's
// action waiting with its window gone, which the readier's sheet reads as the master undoing the trigger. The windows are read
// first, so the pass shows as "window still open, readied action already ignored".
func TestAPassBetweenTheViewsReadsNeverShowsAGoneWindowWithItsReadiedActionWaiting(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.readyMove(t)
	w := a.mustFire(t, e, "Toren")
	toren := a.id(t, "Toren")
	goblin := a.id(t, "Goblin")

	hook := &afterFirstViewRead{DBTX: a.h.pool}
	hook.fn = func() {
		a.mustAnswer(t, a.master, e, w.GetId(), passAnswer)
	}
	was := a.h.svc.queries
	a.h.svc.queries = playdb.New(hook)
	t.Cleanup(func() { a.h.svc.queries = was })

	res, err := a.master.combat.SetCombatantHidden(t.Context(), connect.NewRequest(&playv1.SetCombatantHiddenRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: goblin, IdempotencyKey: newKey(), Hidden: true,
	}))
	if err != nil {
		t.Fatalf("SetCombatantHidden() error = %v", err)
	}
	if !hook.fired.Load() {
		t.Fatal("the view read neither the windows nor the readied actions: the test would prove nothing")
	}
	if got := a.readiedStatus(t, "Toren"); got != readiedIgnored {
		t.Fatalf("the pass left Toren's rows %q, want %q", got, readiedIgnored)
	}
	view := res.Msg.GetEncounter()
	open := false
	for _, rw := range view.GetReactionWindows() {
		if rw.GetKind() == kindReadied && rw.GetReactorId() == toren {
			open = true
		}
	}
	waiting := byLabel(t, view, "Toren").GetReadied() != nil
	if waiting && !open {
		t.Errorf("the view shows Toren's readied action waiting and its window gone: windows %v", view.GetReactionWindows())
	}
}
