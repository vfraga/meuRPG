package play

import (
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The refund prompts of the members of one group check: each member that spends over a hidden advantage has a prompt of their own, and what
// closes the prompt of one member (its roll's held question dropped) leaves the others' open. The session's end closes them together.

// groupRefund is a group check whose members Toren (Caio's) and Pensantus (Ana's) each spent their Inspiração over an advantage only the
// master reads. A member that holds a Bardic Inspiration die has the roll kept for the die's question; the other's is written at once.
type groupRefund struct {
	*favorFight
	torenPrompt, pensPrompt *playv1.FavorRefundPrompt
	holds                   map[string]string // the die's question of each member that holds one, by character
}

func newGroupRefund(t *testing.T, withDie ...func(*armed) *charactersv1.Character) *groupRefund {
	t.Helper()
	a := newArmed(t)
	f := &favorFight{favorParty: favorPartyOf(t, a)}
	view := a.mustRequestGroup(t, func(r *playv1.RequestGroupCheckRequest) { r.Dc = 15 })
	g := &groupRefund{favorFight: f, holds: map[string]string{}}
	for _, who := range []*charactersv1.Character{a.toren, a.pens} {
		f.give(t, who)
		f.hideOutside(t, who.GetId(), "["+checkAdvAll+"]")
	}
	for _, who := range withDie {
		giveDieByRow(t, a.h, a.campaignID, who(a).GetId())
	}
	for _, m := range []struct {
		who   *charactersv1.Character
		owner *user
	}{{a.toren, a.caio}, {a.pens, a.ana}} {
		a.h.roller.queue(4, 15)
		res, err := m.owner.contests.RollGroupCheck(t.Context(), connect.NewRequest(&playv1.RollGroupCheckRequest{
			CampaignId: a.campaignID, IdempotencyKey: newKey(), GroupCheckId: view.GetId(), Roll: rollIn(), SpendFavor: true,
		}))
		if err != nil {
			t.Fatalf("RollGroupCheck(%s) error = %v", m.who.GetName(), err)
		}
		if id := res.Msg.GetHeldRollOffer().GetHoldId(); id != "" {
			g.holds[m.who.GetId()] = id
		}
	}
	prompts := f.prompts(t, f.master)
	if len(prompts) != 2 || f.rows(t) != 0 {
		t.Fatalf("the master reads %d prompts and %d Inspirações are there, want one prompt for each member and both spent", len(prompts), f.rows(t))
	}
	for _, p := range prompts {
		if p.GetCharacterId() == a.toren.GetId() {
			g.torenPrompt = p
		} else {
			g.pensPrompt = p
		}
	}
	if g.torenPrompt == nil || g.pensPrompt == nil {
		t.Fatalf("the prompts are %v, want Toren's and Pensantus's", prompts)
	}
	return g
}

func torenOf(a *armed) *charactersv1.Character { return a.toren }
func pensOf(a *armed) *charactersv1.Character  { return a.pens }

// dieRunsOut is the game time that sweeps the questions that wait for a die that ran out.
func (g *groupRefund) dieRunsOut(t *testing.T) {
	t.Helper()
	g.advanceGameTime(t, 700) // the die had 600 seconds
}

// One member's question is dropped: its prompt closes as kept and its Inspiração is back, and the other member's prompt stays open for the master,
// who can still give that one back.
func TestDroppingOneMembersHoldKeepsOnlyItsRefundPrompt(t *testing.T) {
	t.Parallel()
	g := newGroupRefund(t, torenOf)
	if len(g.holds) != 1 || g.holds[g.toren.GetId()] == "" {
		t.Fatalf("the held questions are %v, want Toren's alone", g.holds)
	}
	g.dieRunsOut(t)
	if g.holdExists(t, g.holds[g.toren.GetId()]) || !g.has(t, g.toren.GetId()) || g.has(t, g.pens.GetId()) {
		t.Fatalf("Toren's question is there %v, Toren has the Inspiração %v, Pensantus has it %v, want his question swept and given back, hers spent",
			g.holdExists(t, g.holds[g.toren.GetId()]), g.has(t, g.toren.GetId()), g.has(t, g.pens.GetId()))
	}
	if status := g.promptStatus(t, g.torenPrompt.GetId()); status != "kept" {
		t.Errorf("Toren's prompt is %q, want kept", status)
	}
	if status := g.promptStatus(t, g.pensPrompt.GetId()); status != "open" {
		t.Errorf("Pensantus's prompt is %q, want open: her roll was not dropped", status)
	}
	if _, err := g.answerRefund(g.master, g.torenPrompt.GetId(), true, newKey()); !isBlockedBy(err, blockedPassClosed) {
		t.Errorf("a Devolver to Toren's closed prompt = %v, want PASS_CLOSED", err)
	}
	if _, err := g.answerRefund(g.master, g.pensPrompt.GetId(), true, newKey()); err != nil {
		t.Errorf("a Devolver to Pensantus's prompt error = %v, want it to work", err)
	}
	if g.rows(t) != 2 {
		t.Errorf("character_favor has %d rows, want one for each member", g.rows(t))
	}
}

// Both members' questions are dropped together (the one sweep of game time): both prompts close as kept and each member gets its own Inspiração
// back, one each.
func TestDroppingBothMembersHoldsKeepsBothRefundPrompts(t *testing.T) {
	t.Parallel()
	g := newGroupRefund(t, torenOf, pensOf)
	if len(g.holds) != 2 {
		t.Fatalf("the held questions are %v, want one for each member", g.holds)
	}
	g.dieRunsOut(t)
	for name, p := range map[string]*playv1.FavorRefundPrompt{"Toren's": g.torenPrompt, "Pensantus's": g.pensPrompt} {
		if status := g.promptStatus(t, p.GetId()); status != "kept" {
			t.Errorf("%s prompt is %q, want kept", name, status)
		}
	}
	if g.rows(t) != 2 || !g.has(t, g.toren.GetId()) || !g.has(t, g.pens.GetId()) {
		t.Errorf("character_favor has %d rows (Toren %v, Pensantus %v), want one for each member", g.rows(t), g.has(t, g.toren.GetId()), g.has(t, g.pens.GetId()))
	}
}

// The end of the session closes the prompts still open, all of them together, as kept: both Inspirações stay spent.
func TestTheEndOfASessionKeepsEveryMembersRefundPrompt(t *testing.T) {
	t.Parallel()
	g := newGroupRefund(t)
	if _, err := g.master.play.EndGameSession(t.Context(), connect.NewRequest(&playv1.EndGameSessionRequest{CampaignId: g.campaignID, GameSessionId: g.sessionID})); err != nil {
		t.Fatalf("EndGameSession() error = %v", err)
	}
	for name, p := range map[string]*playv1.FavorRefundPrompt{"Toren's": g.torenPrompt, "Pensantus's": g.pensPrompt} {
		if status := g.promptStatus(t, p.GetId()); status != "kept" {
			t.Errorf("%s prompt is %q, want kept", name, status)
		}
	}
	if g.rows(t) != 0 {
		t.Errorf("character_favor has %d rows, want 0: the Inspirações stay spent", g.rows(t))
	}
}
