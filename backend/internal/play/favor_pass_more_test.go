package play

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// failingRoster is the characters module's link with a read that fails on demand.
type failingRoster struct {
	CombatRoster
	fail atomic.Bool
}

func (r *failingRoster) SessionCharacters(ctx context.Context, tx pgx.Tx, campaignID string, ids []string) ([]link.Character, error) {
	if r.fail.Load() {
		return nil, errors.New("the characters module is down")
	}
	return r.CombatRoster.SessionCharacters(ctx, tx, campaignID, ids)
}

// A read that fails at the accept is no word about the receiver: it is not CANNOT_RECEIVE, nothing changes and the pass stays pending.
func TestAcceptWithAFailingReadIsNotCannotReceive(t *testing.T) {
	t.Parallel()
	f := newFavorParty(t)
	if err := f.grant(f.toren.GetId()); err != nil {
		t.Fatalf("GrantFavor() error = %v", err)
	}
	p := f.mustAsk(t, f.toren.GetId(), f.pens.GetId())
	w := f.world(t)
	roster := &failingRoster{CombatRoster: f.h.svc.roster}
	f.h.svc.roster = roster
	roster.fail.Store(true)
	_, err := f.answer(p.GetId(), true)
	roster.fail.Store(false)
	if err == nil {
		t.Fatal("the accept with the characters module down answered nothing wrong")
	}
	if reason, ok := blockedReasonOf(err); ok {
		t.Errorf("the accept with a failing read was refused with %v, want an error that is no FavorBlocked", reason)
	}
	f.wantSame(t, "the accept that could not read", w)
	if s := f.passStatus(t, p.GetId()); s != "pending" {
		t.Errorf("stored status = %q, want the pass still pending", s)
	}
	// The same accept goes through once the read works.
	if got, err := f.answer(p.GetId(), true); err != nil || got.GetStatus() != playv1.FavorPassStatus_FAVOR_PASS_STATUS_ACCEPTED {
		t.Errorf("the accept after the read came back = %v, %v, want ACCEPTED", got, err)
	}
}

// Keys are unique in the campaign (favor_passes_ask_key_unique), so the retry of an ask whose session ended finds the pass the first call
// made and answers it; a fresh key is still NO_SESSION.
func TestPassFavorRetryAfterTheSessionEnded(t *testing.T) {
	t.Parallel()
	f := newFavorParty(t)
	if err := f.grant(f.toren.GetId()); err != nil {
		t.Fatalf("GrantFavor() error = %v", err)
	}
	key := newKey()
	first, err := f.askAs(f.caio, f.toren.GetId(), f.pens.GetId(), key)
	if err != nil {
		t.Fatalf("PassFavor() error = %v", err)
	}
	f.master.end(t, f.master.liveSession(t, f.campaignID).GetGameSession())
	w := f.world(t)
	again, err := f.askAs(f.caio, f.toren.GetId(), f.pens.GetId(), key)
	if err != nil || again.GetId() != first.GetId() || again.GetStatus() != playv1.FavorPassStatus_FAVOR_PASS_STATUS_CANCELED {
		t.Errorf("the retry after the session ended = %v, %v, want the same pass %s, canceled by the end", again, err, first.GetId())
	}
	_, err = f.askAs(f.caio, f.toren.GetId(), f.bri.GetId(), key)
	wantCode(t, "the key for another receiver after the session ended", err, connect.CodeInvalidArgument)
	_, err = f.askAs(f.ana, f.toren.GetId(), f.pens.GetId(), key)
	wantCode(t, "the key of someone else's ask after the session ended", err, connect.CodeInvalidArgument)
	_, err = f.askAs(f.caio, f.toren.GetId(), f.pens.GetId(), newKey())
	wantFavorBlocked(t, "a new ask after the session ended", err, blockedNoSession)
	f.wantSame(t, "the retries after the end", w)
}

// A key is its asker's: another player who sends the same body under it is refused as for a key used for another change, and is not
// answered the pass (the grant's retry checks no actor because only the master grants; here the answer would show a player another's ask).
func TestPassFavorKeyOfAnotherActor(t *testing.T) {
	t.Parallel()
	f := newFavorParty(t)
	if err := f.grant(f.toren.GetId()); err != nil {
		t.Fatalf("GrantFavor() error = %v", err)
	}
	key := newKey()
	if _, err := f.askAs(f.caio, f.toren.GetId(), f.pens.GetId(), key); err != nil {
		t.Fatalf("PassFavor() error = %v", err)
	}
	w := f.world(t)
	_, err := f.askAs(f.ana, f.toren.GetId(), f.pens.GetId(), key)
	wantCode(t, "Ana with Caio's key and body", err, connect.CodeInvalidArgument)
	_, err = f.askAs(f.master, f.toren.GetId(), f.pens.GetId(), key)
	wantCode(t, "the master with Caio's key and body", err, connect.CodeInvalidArgument)
	f.wantSame(t, "the keys of another actor", w)
}

// The sentences of an Inspiração that the replay of a held roll could not spend because its pass waits: the master and the roller read them,
// another player reads nothing.
func TestFavorLostPendingTexts(t *testing.T) {
	t.Parallel()
	owner, other := "owner", "other"
	actor := playdb.Combatant{Label: "Toren", UserID: &owner}
	ev := actionEvent{FavorLost: favorLostPending}
	const line = "A Inspiração de Toren está num pedido de passagem que o mestre não respondeu: a rolagem foi feita sem ela."
	for who, c := range map[string]struct {
		v    combatViewer
		want string
	}{
		"the master":     {combatViewer{master: true}, line},
		"the roller":     {combatViewer{userID: owner}, line},
		"another player": {combatViewer{userID: other}, ""},
	} {
		if got := favorLine(ev, c.v, actor); got != c.want {
			t.Errorf("favorLine(pending) for %s = %q, want %q", who, got, c.want)
		}
	}
	if got, want := favorLostNote(favorLostPending), "Inspiração não usada: está num pedido de passagem sem resposta"; got != want {
		t.Errorf("favorLostNote(pending) = %q, want %q", got, want)
	}
	if got, want := favorLostNote(favorLostGone), "Inspiração não usada: não estava mais com você"; got != want {
		t.Errorf("favorLostNote(gone) = %q, want %q", got, want)
	}
	if got, want := favorLostNote(favorLostNothing), "Inspiração guardada: não mudaria mais a rolagem"; got != want {
		t.Errorf("favorLostNote(adds_nothing) = %q, want %q", got, want)
	}
}

// A real window replay that finds the pass pending: the check is kept (the hold is released and rolled), without the Inspiração, nothing
// is refunded or spent, the pass stays pending, and the roll's note says why.
func TestFavorWithAPendingPassAtTheReplayRollsTheCombatCheckWithoutIt(t *testing.T) {
	t.Parallel()
	for _, p := range checkEnvs {
		if p.name != "a contest start" && p.name != "a contest answer" && p.name != "a hide" {
			continue
		}
		t.Run(p.name, func(t *testing.T) {
			t.Parallel()
			e := p.mk(t)
			e.setRules(t, enemyReactionsAlways)
			e.give(t, e.who)
			held, err := e.send(t, newKey(), true)
			if err != nil || !held.heldByWindow {
				t.Fatalf("the spend on a check a window holds = %+v, %v, want it held", held, err)
			}
			to := e.pens.GetId()
			if to == e.who.GetId() {
				to = e.toren.GetId()
			}
			ask := e.mustAsk(t, e.who.GetId(), to) // asked while the window waits
			w := e.windowHeld(t)
			e.h.roller.queue(12)
			e.mustAnswer(t, e.master, e.fight.refresh(t), w.GetId(), passAnswer)
			e.wantRolledWithoutIt(t, p.name, "Inspiração não usada: está num pedido de passagem sem resposta", 1)
			if !e.favorHolds(t) || e.has(t, to) {
				t.Errorf("after the replay the giver has it = %v and the receiver has it = %v, want it still with the giver, nothing refunded or moved", e.favorHolds(t), e.has(t, to))
			}
			if s := e.passStatus(t, ask.GetId()); s != "pending" {
				t.Errorf("the pass is %q after the replay, want it pending", s)
			}
		})
	}
}
