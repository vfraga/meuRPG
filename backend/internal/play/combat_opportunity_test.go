package play

import (
	"context"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// Opportunity attacks on the server (MR-034, RN-21, RN-20, question 69; Etapa 9,
// slice 9.6b). These tests need the database (MEURPG_TEST_DATABASE_URL). The
// fixture is the cave of combat_move_test.go: Toren is on turn at (6, 7) and
// Goblin 1 is placed by the master next to him, at (7, 8).

// opportunityRPCs are the methods of this slice; the encounter's authorization
// matrix (combat_test.go) leaves them to this file's.
var opportunityRPCs = []string{"DeclineOpportunity", "SkipOpportunity"}

var reasonOpportunityPending = playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_OPPORTUNITY_PENDING

// oppFight is the cave fight with Goblin 1 next to Toren. The master places it
// while Toren is on turn: a placement out of turn, which offers nothing.
func (c *cave) oppFight(t *testing.T) *playv1.Encounter {
	t.Helper()
	e := c.fight(t)
	c.moveOffering(t, "Goblin 1", 7, 8)
	if got := c.get(t, c.master).GetOpportunityOffers(); len(got) != 0 {
		t.Fatalf("a placement out of turn offered %v", got)
	}
	return e
}

// leaveGoblin is Toren walking out of Goblin 1's reach, as his player.
func (c *cave) leaveGoblin(t *testing.T) *playv1.MoveCombatantResponse {
	t.Helper()
	res, err := c.moveResponse(t, c.caio, "Toren", 4, 6)
	if err != nil {
		t.Fatalf("MoveCombatant(Toren to 4,6) error = %v", err)
	}
	return res
}

// jumpMove is Toren's player jumping.
func (c *cave) jumpMove(t *testing.T, label string, col, row int32, edit func(*playv1.MoveCombatantRequest)) (*playv1.MoveCombatantResponse, error) {
	t.Helper()
	req := &playv1.MoveCombatantRequest{CampaignId: c.campaignID, EncounterId: c.get(t, c.master).GetId(), CombatantId: c.id(t, label), IdempotencyKey: newKey(), Col: col, Row: row}
	edit(req)
	res, err := c.caio.combat.MoveCombatant(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

// offersOf is the offers of the combat as u reads it.
func (c *cave) offersOf(t *testing.T, u *user) []*playv1.OpportunityOffer {
	t.Helper()
	return c.get(t, u).GetOpportunityOffers()
}

func (a *armed) execSQL(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := a.h.pool.Exec(t.Context(), sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

// offerAttack calls RollAttack as u with an offer.
func (c *cave) offerAttack(t *testing.T, u *user, attacker, key, target, offerID string, roll func(*playv1.RollAttackRequest)) (*playv1.RollAttackResponse, error) {
	t.Helper()
	req := &playv1.RollAttackRequest{
		CampaignId: c.campaignID, EncounterId: c.get(t, c.master).GetId(), AttackerId: c.id(t, attacker), AttackKey: key, TargetId: c.id(t, target),
		IdempotencyKey: newKey(), OpportunityOfferId: offerID,
	}
	roll(req)
	res, err := u.combat.RollAttack(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

// decline calls DeclineOpportunity as u.
func (c *cave) decline(t *testing.T, u *user, offerID string) error {
	t.Helper()
	_, err := u.combat.DeclineOpportunity(t.Context(), connect.NewRequest(&playv1.DeclineOpportunityRequest{
		CampaignId: c.campaignID, EncounterId: c.get(t, c.master).GetId(), OpportunityOfferId: offerID, IdempotencyKey: newKey(),
	}))
	return err
}

// skipOffers passes over every offer that waits, as the master.
func (a *armed) skipOffers(t *testing.T) {
	t.Helper()
	for _, o := range a.get(t, a.master).GetOpportunityOffers() {
		if _, err := a.master.combat.SkipOpportunity(t.Context(), connect.NewRequest(&playv1.SkipOpportunityRequest{
			CampaignId: a.campaignID, EncounterId: a.get(t, a.master).GetId(), OpportunityOfferId: o.GetId(), IdempotencyKey: newKey(),
		})); err != nil {
			t.Fatalf("SkipOpportunity() error = %v", err)
		}
	}
}

// moveOffering is the master moving someone on turn, keeping the offers it makes.
func (c *cave) moveOffering(t *testing.T, label string, col, row int32) {
	t.Helper()
	if _, err := c.move(t, c.master, label, col, row); err != nil {
		t.Fatalf("MoveCombatant(%s to %d,%d) error = %v", label, col, row, err)
	}
}

// skip calls SkipOpportunity as u.
func (c *cave) skip(t *testing.T, u *user, offerID string) error {
	t.Helper()
	_, err := u.combat.SkipOpportunity(t.Context(), connect.NewRequest(&playv1.SkipOpportunityRequest{
		CampaignId: c.campaignID, EncounterId: c.get(t, c.master).GetId(), OpportunityOfferId: offerID, IdempotencyKey: newKey(),
	}))
	return err
}

// square is where a combatant stands, as the master sees it.
func (c *cave) square(t *testing.T, label string) [2]int32 {
	t.Helper()
	who := c.who(t, c.master, label)
	return [2]int32{who.GetCol(), who.GetRow()}
}

// TestRN21_LeavingAnEnemysReachOffersAnAttack: moving inside the reach offers
// nothing; leaving it lands the move at once and offers the enemy one attack,
// which the enemy's controller (the master, for an NPC) and the mover's player
// see, and nobody else does. The master's undo of the move takes the offer.
func TestRN21_LeavingAnEnemysReachOffersAnAttack(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.oppFight(t)

	res, err := c.moveResponse(t, c.caio, "Toren", 6, 8) // from beside to beside: still inside the reach
	if err != nil || res.GetProvoked() || len(res.GetEncounter().GetOpportunityOffers()) != 0 {
		t.Fatalf("a move inside the reach = (%v, %v), want no offer", res, err)
	}
	res = c.leaveGoblin(t)
	if !res.GetProvoked() {
		t.Errorf("leaving the reach: provoked = false")
	}
	if got := c.square(t, "Toren"); got != [2]int32{4, 6} {
		t.Errorf("Toren after the move stands on %v, want 4,6: the move lands at once", got)
	}

	masterOffers := c.offersOf(t, c.master)
	if len(masterOffers) != 1 {
		t.Fatalf("the master's offers = %v, want one", masterOffers)
	}
	offer := masterOffers[0]
	if offer.GetMoverId() != c.id(t, "Toren") || offer.GetReactorId() != c.id(t, "Goblin 1") || offer.GetReactorLabel() != "Goblin 1" || !offer.GetForYou() {
		t.Errorf("the master's offer = %v, want Goblin 1 on Toren, for the master", offer)
	}
	if len(offer.GetAttacks()) != 1 || offer.GetAttacks()[0].GetKey() != sword || offer.GetAttacks()[0].GetNamePt() != "Cimitarra" {
		t.Errorf("the offer's attacks = %v, want the Cimitarra (basic:0)", offer.GetAttacks())
	}
	if offer.GetLeftCol() != 6 || offer.GetLeftRow() != 8 {
		t.Errorf("the offer's left square = %d,%d, want 6,8: the last square of the line inside the reach", offer.GetLeftCol(), offer.GetLeftRow())
	}
	// Toren's player sees the offer on Toren, with the reactor he sees, and cannot answer.
	mine := c.offersOf(t, c.caio)
	if len(mine) != 1 || mine[0].GetReactorLabel() != "Goblin 1" || mine[0].GetForYou() || len(mine[0].GetAttacks()) != 0 {
		t.Errorf("Toren's player's offers = %v, want the wait, naming Goblin 1, not for him", mine)
	}
	for _, u := range []*user{c.ana, c.bia} {
		if got := c.offersOf(t, u); len(got) != 0 {
			t.Errorf("another player got offers %v, want none", got)
		}
	}

	c.undoLast(t)
	if got := c.offersOf(t, c.master); len(got) != 0 {
		t.Errorf("after the undo the offers are %v, want none", got)
	}
	if got := c.square(t, "Toren"); got != [2]int32{6, 8} {
		t.Errorf("after the undo Toren stands on %v, want 6,8", got)
	}
	wantCode(t, "answering the offer of an undone move", c.decline(t, c.master, offer.GetId()), connect.CodeAborted)
}

// TestRN21_WhoProvokesAnOpportunityAttack: a reactor that is not hostile, has no
// reaction, is incapacitated or can't see the mover doesn't offer; neither does
// a mover that took Desengajar, jumped, or was placed by the master out of turn.
func TestRN21_WhoProvokesAnOpportunityAttack(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		setup func(t *testing.T, c *cave, e *playv1.Encounter)
		move  func(t *testing.T, c *cave) (*playv1.MoveCombatantResponse, error)
		want  bool
	}{
		{name: "the baseline provokes", want: true},
		{name: "Desengajar", setup: func(t *testing.T, c *cave, e *playv1.Encounter) {
			t.Helper()
			if _, err := c.action(t, c.caio, e, "Toren", "standard:disengage"); err != nil {
				t.Fatalf("TakeAction(disengage) error = %v", err)
			}
		}},
		{name: "a reactor without its reaction", setup: func(t *testing.T, c *cave, _ *playv1.Encounter) {
			t.Helper()
			c.execSQL(t, `UPDATE combatants SET reaction_used = true WHERE id = $1`, c.id(t, "Goblin 1"))
		}},
		{name: "an incapacitated reactor", setup: func(t *testing.T, c *cave, e *playv1.Encounter) {
			t.Helper()
			if _, err := c.conditions(t, c.master, e, "Goblin 1", []string{"condition:incapacitated"}, true, false); err != nil {
				t.Fatalf("SetCombatantConditions() error = %v", err)
			}
		}},
		{name: "a reactor on the mover's side", setup: func(t *testing.T, c *cave, _ *playv1.Encounter) {
			t.Helper()
			c.side(t, "Goblin 1", playv1.CombatantSide_COMBATANT_SIDE_PARTY)
		}},
		{name: "a defeated reactor", setup: func(t *testing.T, c *cave, _ *playv1.Encounter) {
			t.Helper()
			c.execSQL(t, `UPDATE combatants SET defeated = true WHERE id = $1`, c.id(t, "Goblin 1"))
		}},
		{name: "a long jump spends movement, so it provokes", want: true, move: func(t *testing.T, c *cave) (*playv1.MoveCombatantResponse, error) {
			t.Helper()
			return c.jumpMove(t, "Toren", 5, 6, jumpTo(playv1.JumpKind_JUMP_KIND_LONG))
		}},
		{name: "a high jump moves nobody", move: func(t *testing.T, c *cave) (*playv1.MoveCombatantResponse, error) {
			t.Helper()
			return c.jumpMove(t, "Toren", 5, 6, highJump(10))
		}},
		{name: "a forced move of the master never provokes", move: func(t *testing.T, c *cave) (*playv1.MoveCombatantResponse, error) {
			t.Helper()
			res, err := c.master.combat.MoveCombatant(t.Context(), connect.NewRequest(&playv1.MoveCombatantRequest{
				CampaignId: c.campaignID, EncounterId: c.get(t, c.master).GetId(), CombatantId: c.id(t, "Toren"), IdempotencyKey: newKey(), Col: 4, Row: 6, Forced: true,
			}))
			if err != nil {
				return nil, err
			}
			return res.Msg, nil
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := newCave(t)
			e := c.oppFight(t)
			if tc.setup != nil {
				tc.setup(t, c, e)
			}
			move := tc.move
			if move == nil {
				move = func(t *testing.T, c *cave) (*playv1.MoveCombatantResponse, error) {
					t.Helper()
					return c.moveResponse(t, c.caio, "Toren", 4, 6)
				}
			}
			res, err := move(t, c)
			if err != nil {
				t.Fatalf("the move error = %v", err)
			}
			if res.GetProvoked() != tc.want || (len(c.offersOf(t, c.master)) == 1) != tc.want {
				t.Errorf("provoked = %v with offers %v, want provoked = %v", res.GetProvoked(), c.offersOf(t, c.master), tc.want)
			}
		})
	}

	t.Run("the master's placement out of turn", func(t *testing.T) {
		t.Parallel()
		c := newCave(t)
		c.oppFight(t)
		c.moveOffering(t, "Pensantus", 6, 8) // beside Goblin 1, out of turn
		c.moveOffering(t, "Pensantus", 4, 9) // and out of its reach again: still a placement
		if got := c.offersOf(t, c.master); len(got) != 0 {
			t.Errorf("the master's placements offered %v", got)
		}
	})

	t.Run("two reactors offer two attacks", func(t *testing.T) {
		t.Parallel()
		c := newCave(t)
		c.oppFight(t)
		c.mustMove(t, c.master, "Goblin 2", 6, 6)
		res, err := c.moveResponse(t, c.caio, "Toren", 3, 7)
		if err != nil || !res.GetProvoked() {
			t.Fatalf("MoveCombatant(Toren to 3,7) = (%v, %v), want provoked", res, err)
		}
		got := map[string]bool{}
		for _, o := range c.offersOf(t, c.master) {
			got[o.GetReactorLabel()] = true
		}
		if len(got) != 2 || !got["Goblin 1"] || !got["Goblin 2"] {
			t.Errorf("the offers are for %v, want Goblin 1 and Goblin 2", got)
		}
	})
}

// TestMR034_GetMoveOptionsWarnsWhichSquaresProvoke: each reachable square says
// the visible reactors it would provoke, none after Desengajar, and never a
// hidden one.
func TestMR034_GetMoveOptionsWarnsWhichSquaresProvoke(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	e := c.oppFight(t)
	gob := c.id(t, "Goblin 1")
	provokes := func(u *user) map[[2]int32][]string {
		opts, err := c.options(t, u, "Toren")
		if err != nil {
			t.Fatalf("GetMoveOptions() error = %v", err)
		}
		out := map[[2]int32][]string{}
		for _, r := range opts.GetReachable() {
			if len(r.GetProvokesReactorIds()) > 0 {
				out[[2]int32{r.GetCol(), r.GetRow()}] = r.GetProvokesReactorIds()
			}
		}
		return out
	}
	got := provokes(c.caio)
	if ids := got[[2]int32{4, 6}]; len(ids) != 1 || ids[0] != gob {
		t.Errorf("(4,6) provokes %v, want Goblin 1", ids)
	}
	if ids, ok := got[[2]int32{6, 8}]; ok {
		t.Errorf("(6,8), inside the reach, provokes %v", ids)
	}
	c.hide(t, "Goblin 1")
	if got := provokes(c.caio); len(got) != 0 {
		t.Errorf("with the goblin hidden, a player is warned about %v: a hidden reactor is never named", got)
	}
	if got := provokes(c.master); len(got[[2]int32{4, 6}]) != 1 {
		t.Errorf("the master is warned about %v, want Goblin 1 on (4,6)", got)
	}
	c.execSQL(t, `UPDATE combatants SET hidden = false WHERE id = $1`, gob)
	if _, err := c.action(t, c.caio, e, "Toren", "standard:disengage"); err != nil {
		t.Fatalf("TakeAction(disengage) error = %v", err)
	}
	if got := provokes(c.caio); len(got) != 0 {
		t.Errorf("after Desengajar the warnings are %v, want none", got)
	}
}

// TestMR034_TheAnswersToAnOffer: the reactor's controller attacks (no reach
// check, the reaction spent, the Escudo prompt still possible), declines, or the
// master skips; a second answer is aborted and the wrong person is
// permission_denied.
func TestMR034_TheAnswersToAnOffer(t *testing.T) {
	t.Parallel()
	t.Run("attack: no reach check, the reaction spent", func(t *testing.T) {
		t.Parallel()
		c := newCave(t)
		c.oppFight(t)
		c.leaveGoblin(t) // Toren is now 3 squares from Goblin 1
		offer := c.offersOf(t, c.master)[0]
		if c.who(t, c.master, "Goblin 1").GetReactionUsed() {
			t.Fatal("Goblin 1's reaction is used before the answer")
		}
		// Toren is 15 ft away: a plain reaction attack would be out of reach for a
		// player, the offer's attack asks no reach.
		res, err := c.offerAttack(t, c.master, "Goblin 1", sword, "Toren", offer.GetId(), d20(15))
		if err != nil {
			t.Fatalf("the offer's attack error = %v", err)
		}
		if res.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_HIT || res.GetPendingDamage() == nil {
			t.Errorf("the attack = %v, want a hit with its damage to roll", res)
		}
		if !c.who(t, c.master, "Goblin 1").GetReactionUsed() {
			t.Errorf("Goblin 1's reaction is not spent by its opportunity attack")
		}
		if got := c.offersOf(t, c.master); len(got) != 0 {
			t.Errorf("an answered offer still waits: %v", got)
		}
		wantCode(t, "a second answer", c.decline(t, c.master, offer.GetId()), connect.CodeAborted)
		_, err = c.offerAttack(t, c.master, "Goblin 1", sword, "Toren", offer.GetId(), d20(15))
		wantCode(t, "a second attack on the offer", err, connect.CodeAborted)

		// Taking the attack back makes the offer wait again.
		c.undoLast(t)
		if got := c.offersOf(t, c.master); len(got) != 1 || got[0].GetId() != offer.GetId() {
			t.Errorf("after undoing the attack the offers are %v, want it back", got)
		}
		if c.who(t, c.master, "Goblin 1").GetReactionUsed() {
			t.Errorf("the undo left Goblin 1's reaction spent")
		}
	})

	t.Run("a hit on a caster can still be answered with Escudo", func(t *testing.T) {
		t.Parallel()
		c := newCave(t)
		e := c.fight(t)
		c.mustMove(t, c.master, "Goblin 1", 6, 9) // beside Pensantus (5,8), out of turn
		c.mustEndTurn(t, c.caio, e)               // Pensantus's turn
		if _, err := c.moveResponse(t, c.ana, "Pensantus", 3, 7); err != nil {
			t.Fatalf("MoveCombatant(Pensantus) error = %v", err)
		}
		offer := c.offersOf(t, c.ana)
		if len(offer) != 1 || offer[0].GetForYou() {
			t.Fatalf("Pensantus's player's offers = %v, want the wait", offer)
		}
		res, err := c.offerAttack(t, c.master, "Goblin 1", sword, "Pensantus", offer[0].GetId(), d20(15))
		if err != nil {
			t.Fatalf("the offer's attack error = %v", err)
		}
		if res.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_HIT {
			t.Fatalf("the attack = %v, want a hit", res.GetRoll())
		}
		if prompts := c.get(t, c.ana).GetReactionPrompts(); len(prompts) != 1 {
			t.Errorf("Ana's reaction prompts = %v, want the Escudo prompt for the hit", prompts)
		}
	})

	t.Run("decline, skip and the wrong person", func(t *testing.T) {
		t.Parallel()
		c := newCave(t)
		c.oppFight(t)
		c.leaveGoblin(t)
		offer := c.offersOf(t, c.master)[0]
		wantCode(t, "the mover's player declining", c.decline(t, c.caio, offer.GetId()), connect.CodePermissionDenied)
		wantCode(t, "the mover's player skipping", c.skip(t, c.caio, offer.GetId()), connect.CodePermissionDenied)
		_, err := c.offerAttack(t, c.caio, "Goblin 1", sword, "Toren", offer.GetId(), d20(15))
		wantCode(t, "the mover's player attacking for the goblin", err, connect.CodePermissionDenied)
		if got := c.offersOf(t, c.master); len(got) != 1 {
			t.Fatalf("a refused answer changed the offers: %v", got)
		}
		if err := c.decline(t, c.master, offer.GetId()); err != nil {
			t.Fatalf("the master's decline error = %v", err)
		}
		if got := c.offersOf(t, c.master); len(got) != 0 {
			t.Errorf("a declined offer still waits: %v", got)
		}
		wantCode(t, "declining twice", c.decline(t, c.master, offer.GetId()), connect.CodeAborted)
		wantCode(t, "skipping a declined offer", c.skip(t, c.master, offer.GetId()), connect.CodeAborted)
		c.undoLast(t) // the decline is an action the master can take back
		if got := c.offersOf(t, c.master); len(got) != 1 {
			t.Errorf("after undoing the decline the offers are %v, want it waiting again", got)
		}
		if err := c.skip(t, c.master, offer.GetId()); err != nil {
			t.Fatalf("the master's skip error = %v", err)
		}
		if got := c.offersOf(t, c.master); len(got) != 0 {
			t.Errorf("a skipped offer still waits: %v", got)
		}
	})

	t.Run("a player reactor answers its own offer", func(t *testing.T) {
		t.Parallel()
		c := newCave(t)
		c.oppFight(t)
		c.passTo(t, c.get(t, c.master), "Goblin 1")
		c.moveOffering(t, "Goblin 1", 9, 8) // the master moves its NPC out of Toren's reach
		toren := c.id(t, "Toren")
		offers := c.offersOf(t, c.caio)
		if len(offers) != 1 || offers[0].GetReactorId() != toren || !offers[0].GetForYou() || len(offers[0].GetAttacks()) == 0 {
			t.Fatalf("Toren's player's offers = %v, want one for him with attacks", offers)
		}
		for _, u := range []*user{c.ana, c.bia} {
			if got := c.offersOf(t, u); len(got) != 0 {
				t.Errorf("another player got offers %v", got)
			}
		}
		wantCode(t, "another player declining Toren's offer", c.decline(t, c.ana, offers[0].GetId()), connect.CodePermissionDenied)
		if err := c.decline(t, c.caio, offers[0].GetId()); err != nil {
			t.Fatalf("Toren's player declining error = %v", err)
		}
		wantCode(t, "a second decline", c.decline(t, c.caio, offers[0].GetId()), connect.CodeAborted)
	})
}

// TestMR034_TheMoversTurnWaits: while an offer on the mover waits, its player's
// end of turn, actions, attacks, spells and moves are refused with
// OPPORTUNITY_PENDING; they work after the answer; the master is never stopped.
func TestMR034_TheMoversTurnWaits(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	e := c.oppFight(t)
	c.leaveGoblin(t)
	offer := c.offersOf(t, c.master)[0]

	_, endErr := c.endTurn(t, c.caio, e, false)
	wantEncounterBlocked(t, endErr, reasonOpportunityPending)
	_, err := c.action(t, c.caio, e, "Toren", "standard:dodge")
	wantEncounterBlocked(t, err, reasonOpportunityPending)
	_, err = c.attack(t, c.caio, e, "Toren", battleaxe, "Goblin 1", d20(10))
	wantEncounterBlocked(t, err, reasonOpportunityPending)
	_, err = c.move(t, c.caio, "Toren", 3, 6)
	wantEncounterBlocked(t, err, reasonOpportunityPending)

	// Other combatants' reactions are not held: Pensantus's player is not stopped by it
	// (she is off turn; her own refusal is the turn's, not the wait's).
	if _, err := c.move(t, c.ana, "Pensantus", 5, 9); err == nil || connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Errorf("Pensantus's move off turn = %v, want failed_precondition", err)
	} else if b := encounterBlocked(t, err); b.GetReason() == reasonOpportunityPending {
		t.Errorf("Pensantus's move is held by Toren's offer")
	}

	if err := c.decline(t, c.master, offer.GetId()); err != nil {
		t.Fatalf("DeclineOpportunity() error = %v", err)
	}
	if _, err := c.action(t, c.caio, e, "Toren", "standard:dodge"); err != nil {
		t.Errorf("an action after the answer error = %v", err)
	}
	if _, err := c.endTurn(t, c.caio, e, false); err != nil {
		t.Errorf("EndTurn after the answer error = %v", err)
	}

	// The master can always act, and ending the turn passes the offers over.
	c2 := newCave(t)
	e2 := c2.oppFight(t)
	c2.leaveGoblin(t)
	if _, err := c2.endTurn(t, c2.master, e2, false); err != nil {
		t.Fatalf("the master's EndTurn error = %v", err)
	}
	if got := c2.offersOf(t, c2.master); len(got) != 0 {
		t.Errorf("after the master ended the turn the offers are %v, want them passed over", got)
	}
}

// TestMR034_ZeroHitPointsSendsTheMoverBack: an opportunity attack that drops the
// mover to 0 hit points leaves it on the square where it left the reach (the
// token and the combatant), and the undo of the damage puts it where it went.
func TestMR034_ZeroHitPointsSendsTheMoverBack(t *testing.T) {
	t.Parallel()
	zeroHitPoints(t, false)
}

// TestMR034_ZeroHitPointsStaysWhereTheLeftSquareIsTaken: the same, but another
// combatant stands on the square the mover left the reach at: it stays where it
// is, and the master's log says so.
func TestMR034_ZeroHitPointsStaysWhereTheLeftSquareIsTaken(t *testing.T) {
	t.Parallel()
	zeroHitPoints(t, true)
}

func zeroHitPoints(t *testing.T, taken bool) {
	t.Helper()
	c := newCave(t)
	c.oppFight(t)
	c.passTo(t, c.get(t, c.master), "Goblin 1")
	c.moveOffering(t, "Goblin 1", 5, 9) // along a line that passes through Toren's reach
	offers := c.offersOf(t, c.caio)
	if len(offers) != 1 {
		t.Fatalf("Toren's player's offers = %v, want one", offers)
	}
	left := [2]int32{offers[0].GetLeftCol(), offers[0].GetLeftRow()}
	want := grid.LeavesReach(grid.Square{Col: 7, Row: 8}, grid.Square{Col: 5, Row: 9}, grid.Square{Col: 6, Row: 7}, 5)
	if !want.Leaves || left != [2]int32{clamp32(want.LastInReach.Col, 0, 1000), clamp32(want.LastInReach.Row, 0, 1000)} || left == [2]int32{5, 9} {
		t.Fatalf("the left square = %v, grid says %v: it must be a square inside the reach, not the destination", left, want)
	}
	if got := c.square(t, "Goblin 1"); got != [2]int32{5, 9} {
		t.Fatalf("Goblin 1 stands on %v, want 5,9: the move lands at once", got)
	}
	if taken {
		c.mustMove(t, c.master, "Goblin 2", left[0], left[1])
	}

	res, err := c.offerAttack(t, c.caio, "Toren", offers[0].GetAttacks()[0].GetKey(), "Goblin 1", offers[0].GetId(), d20(15))
	if err != nil || res.GetPendingDamage() == nil {
		t.Fatalf("Toren's opportunity attack = (%v, %v), want a hit", res, err)
	}
	if got := c.square(t, "Goblin 1"); got != [2]int32{5, 9} {
		t.Fatalf("before the damage Goblin 1 stands on %v: the rule is about the damage", got)
	}
	if _, err := c.damage(t, c.caio, c.get(t, c.master), res.GetPendingDamage().GetId(), typedDamage(8)); err != nil {
		t.Fatalf("RollDamage() error = %v", err)
	}
	if _, _, defeated := c.hp(t, "Goblin 1"); !defeated {
		t.Fatalf("Goblin 1 was not dropped to 0 hit points")
	}
	attackEntry := func(u *user) *playv1.CombatLogEntry {
		for _, r := range c.log(t, u, c.get(t, c.master)).GetRounds() {
			for _, en := range r.GetEntries() {
				if en.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_ATTACK && en.GetAsReaction() {
					return en
				}
			}
		}
		t.Fatal("no opportunity attack in the log")
		return nil
	}
	if taken {
		if got := c.square(t, "Goblin 1"); got != [2]int32{5, 9} {
			t.Errorf("with the left square taken Goblin 1 stands on %v, want it to stay on 5,9", got)
		}
		if en := attackEntry(c.master); en.GetReturnedToReach() || !en.GetReturnBlocked() {
			t.Errorf("the master's line = returned %v, blocked %v; want it to say the square was taken", en.GetReturnedToReach(), en.GetReturnBlocked())
		}
		if en := attackEntry(c.caio); en.GetReturnBlocked() {
			t.Errorf("a player's line says the return was blocked")
		}
		return
	}
	if got := c.square(t, "Goblin 1"); got != left {
		t.Errorf("Goblin 1 at 0 hit points stands on %v, want %v where it left the reach", got, left)
	}
	if en := attackEntry(c.master); !en.GetReturnedToReach() || en.GetReturnBlocked() {
		t.Errorf("the master's line = returned %v, blocked %v; want it back", en.GetReturnedToReach(), en.GetReturnBlocked())
	}
	if got := c.who(t, c.caio, "Goblin 1"); got.GetCol() != left[0] || got.GetRow() != left[1] {
		t.Errorf("the player sees Goblin 1 on %d,%d, want %v", got.GetCol(), got.GetRow(), left)
	}
	c.undoLast(t) // the damage
	if got := c.square(t, "Goblin 1"); got != [2]int32{5, 9} {
		t.Errorf("after undoing the damage Goblin 1 stands on %v, want 5,9", got)
	}
}

// TestMR034_AMissLeavesTheMoverWhereItWent: a miss, and a hit that leaves hit
// points, move nobody.
func TestMR034_AMissLeavesTheMoverWhereItWent(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.oppFight(t)
	c.leaveGoblin(t)
	offer := c.offersOf(t, c.master)[0]
	res, err := c.offerAttack(t, c.master, "Goblin 1", sword, "Toren", offer.GetId(), d20(1))
	if err != nil || res.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_MISS {
		t.Fatalf("the attack = (%v, %v), want a miss", res.GetRoll(), err)
	}
	if got := c.square(t, "Toren"); got != [2]int32{4, 6} {
		t.Errorf("Toren after a miss stands on %v, want 4,6", got)
	}
	c.undoLast(t)
	if got := c.offersOf(t, c.master); len(got) != 1 {
		t.Errorf("after undoing the missed attack the offers are %v, want it waiting again", got)
	}
}

// TestRN20_AHiddenReactorIsNeverNamedToAPlayer: a reactor the players do not see
// still provokes (it sees the mover), the mover's player is told only that the
// turn waits, and nobody else gets a thing; the answers, the move's response and
// the encounter, as the app's JSON, never carry its id or its name.
func TestRN20_AHiddenReactorIsNeverNamedToAPlayer(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.oppFight(t)
	c.hide(t, "Goblin 1")
	gob := c.id(t, "Goblin 1")

	res := c.leaveGoblin(t)
	if !res.GetProvoked() {
		t.Fatal("a hidden reactor did not provoke: it sees the mover")
	}
	all := func(u *user) string {
		raw, err := protojson.Marshal(c.get(t, u))
		if err != nil {
			t.Fatalf("protojson.Marshal() error = %v", err)
		}
		return string(raw)
	}
	mover, err := protojson.Marshal(res)
	if err != nil {
		t.Fatalf("protojson.Marshal() error = %v", err)
	}
	for who, raw := range map[string]string{"Toren's player's encounter": all(c.caio), "Ana's encounter": all(c.ana), "Toren's move response": string(mover)} {
		if strings.Contains(raw, gob) || strings.Contains(raw, "Goblin 1") {
			t.Errorf("%s names the hidden reactor: %s", who, raw)
		}
	}
	mine := c.offersOf(t, c.caio)
	if len(mine) != 1 || mine[0].GetMoverId() != c.id(t, "Toren") || mine[0].GetReactorId() != "" || mine[0].GetReactorLabel() != "" || mine[0].GetForYou() {
		t.Errorf("Toren's player's offers = %v, want one wait with no reactor", mine)
	}
	if got := c.offersOf(t, c.ana); len(got) != 0 {
		t.Errorf("Ana got offers %v", got)
	}
	// Answering a reactor the player does not see is not_found, never "not yours".
	wantCode(t, "declining for a hidden reactor", c.decline(t, c.caio, c.offersOf(t, c.master)[0].GetId()), connect.CodeNotFound)
	if got := c.offersOf(t, c.master); len(got) != 1 || got[0].GetReactorLabel() != "Goblin 1" {
		t.Errorf("the master's offers = %v, want Goblin 1 named", got)
	}
}

// TestMR034_OpportunityAuthorizationMatrix: signed out is unauthenticated, an
// outsider is not_found, a player that is not the reactor's controller is
// permission_denied, SkipOpportunity is the master's, and the master is never
// turned away by authorization.
func TestMR034_OpportunityAuthorizationMatrix(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.oppFight(t)
	c.leaveGoblin(t)
	outsider := c.h.newUser("Intruso")
	anonymous := c.h.anonymous()
	offer := c.offersOf(t, c.master)[0].GetId()
	calls := map[string]func(u *user, ctx context.Context) error{
		"DeclineOpportunity": func(u *user, _ context.Context) error { return c.decline(t, u, offer) },
		"SkipOpportunity":    func(u *user, _ context.Context) error { return c.skip(t, u, offer) },
	}
	for name, call := range calls {
		if err := call(anonymous, t.Context()); connect.CodeOf(err) != connect.CodeUnauthenticated {
			t.Errorf("%s signed out: %v, want unauthenticated", name, err)
		}
		if err := call(outsider, t.Context()); connect.CodeOf(err) != connect.CodeNotFound {
			t.Errorf("%s as an outsider: %v, want not_found", name, err)
		}
		if err := call(c.caio, t.Context()); connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Errorf("%s as a player (the mover's) on an NPC's offer: %v, want permission_denied", name, err)
		}
	}
	if err := calls["DeclineOpportunity"](c.master, t.Context()); err != nil {
		t.Errorf("DeclineOpportunity as the master: %v, want ok", err)
	}
}

// undoableEntry is the entry of the master's log that holds the last action, and
// the event the log says an undo would take back.
func (c *cave) undoableEntry(t *testing.T) (*playv1.CombatLogEntry, string) {
	t.Helper()
	log := c.log(t, c.master, c.get(t, c.master))
	for _, r := range log.GetRounds() {
		for _, en := range r.GetEntries() {
			if en.GetUndoable() {
				return en, log.GetUndoableEventId()
			}
		}
	}
	return nil, log.GetUndoableEventId()
}

// TestMR034_TheCoverOfAnOpportunityAttackIsFromWhereTheMoverLeft: the attack
// happens as the mover leaves the reach, so the cover is measured with it on the
// square it left, not on the one it landed on: a wall that stands between the
// reactor and the landing square (a door Toren walked through) is no cover.
func TestMR034_TheCoverOfAnOpportunityAttackIsFromWhereTheMoverLeft(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.oppFight(t)
	c.terrain.addWalls(grid.Square{Col: 7, Row: 7}) // on Goblin 1's line to (6, 6), not to (6, 7)
	if _, err := c.moveResponse(t, c.caio, "Toren", 6, 6); err != nil {
		t.Fatalf("MoveCombatant(Toren to 6,6) error = %v", err)
	}
	offer := c.offersOf(t, c.master)[0]
	res, err := c.offerAttack(t, c.master, "Goblin 1", sword, "Toren", offer.GetId(), d20(15))
	if err != nil {
		t.Fatalf("the offer's attack error = %v", err)
	}
	if res.GetRoll().GetCover() != playv1.CoverDegree_COVER_DEGREE_UNSPECIFIED && res.GetRoll().GetCover() != playv1.CoverDegree_COVER_DEGREE_NONE {
		t.Errorf("the attack's cover = %v, want none: Toren was beside the goblin when it struck", res.GetRoll().GetCover())
	}
}

// TestMR034_TheWaitLastsUntilTheDamageSettles: after the attack is rolled the
// turn still waits, until its damage is rolled and applied or discarded.
func TestMR034_TheWaitLastsUntilTheDamageSettles(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	e := c.oppFight(t)
	c.leaveGoblin(t)
	offer := c.offersOf(t, c.master)[0]
	res, err := c.offerAttack(t, c.master, "Goblin 1", sword, "Toren", offer.GetId(), d20(20))
	if err != nil || res.GetPendingDamage() == nil {
		t.Fatalf("the offer's attack = (%v, %v), want a hit", res, err)
	}
	pid := res.GetPendingDamage().GetId()
	refused := func(when string) {
		t.Helper()
		_, err := c.endTurn(t, c.caio, e, false)
		wantEncounterBlocked(t, err, reasonOpportunityPending)
		_, err = c.move(t, c.caio, "Toren", 3, 6)
		wantEncounterBlocked(t, err, reasonOpportunityPending)
		_, err = c.attack(t, c.caio, e, "Toren", battleaxe, "Goblin 1", d20(10))
		wantEncounterBlocked(t, err, reasonOpportunityPending)
		_ = when
	}
	refused("the damage is not rolled")
	if _, err := c.damage(t, c.master, e, pid, typedDamage(7)); err != nil {
		t.Fatalf("RollDamage() error = %v", err)
	}
	refused("the damage is rolled and waits for the master")
	if _, err := c.settle(t, c.master, e, pid, true); err != nil {
		t.Fatalf("ApplyPendingDamage() error = %v", err)
	}
	if _, err := c.endTurn(t, c.caio, e, false); err != nil {
		t.Errorf("EndTurn after the damage settled error = %v", err)
	}
}

// TestMR034_AForcedMoveIsTheMastersAndNeverProvokes: a player sending `forced`
// is refused; the master's forced move offers nothing, his normal move of the
// combatant on turn does.
func TestMR034_AForcedMoveIsTheMastersAndNeverProvokes(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.oppFight(t)
	_, err := c.caio.combat.MoveCombatant(t.Context(), connect.NewRequest(&playv1.MoveCombatantRequest{
		CampaignId: c.campaignID, EncounterId: c.get(t, c.master).GetId(), CombatantId: c.id(t, "Toren"), IdempotencyKey: newKey(), Col: 4, Row: 6, Forced: true,
	}))
	wantCode(t, "a player forcing a move", err, connect.CodePermissionDenied)
	c.moveOffering(t, "Toren", 4, 6)
	if got := c.offersOf(t, c.master); len(got) != 1 {
		t.Errorf("the master's normal move of Toren made offers %v, want one", got)
	}
}

// TestRN20_ThePlayersWarningDoesNotDependOnAnNPCsReaction: GetMoveOptions warns a
// player from what they see, so a spent reaction (which they cannot see) changes
// nothing in it; the master's list is exact.
func TestRN20_ThePlayersWarningDoesNotDependOnAnNPCsReaction(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.oppFight(t)
	warned := func(u *user) int {
		opts, err := c.options(t, u, "Toren")
		if err != nil {
			t.Fatalf("GetMoveOptions() error = %v", err)
		}
		n := 0
		for _, r := range opts.GetReachable() {
			n += len(r.GetProvokesReactorIds())
		}
		return n
	}
	before := warned(c.caio)
	if before == 0 || warned(c.master) != before {
		t.Fatalf("warnings = player %d, master %d, want the same and above 0", before, warned(c.master))
	}
	c.execSQL(t, `UPDATE combatants SET reaction_used = true WHERE id = $1`, c.id(t, "Goblin 1"))
	if got := warned(c.caio); got != before {
		t.Errorf("the player's warning changed from %d to %d when the NPC's reaction was spent: it leaks the NPC's turn", before, got)
	}
	if got := warned(c.master); got != 0 {
		t.Errorf("the master's warning = %d with the reaction spent, want 0 (exact)", got)
	}
}

// TestMR034_DecliningAndSkippingLeaveAnUndoableEntry: the log entry the master's
// undo button belongs to exists after a decline and after a skip.
func TestMR034_DecliningAndSkippingLeaveAnUndoableEntry(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.oppFight(t)
	c.leaveGoblin(t)
	offer := c.offersOf(t, c.master)[0]
	if err := c.decline(t, c.master, offer.GetId()); err != nil {
		t.Fatalf("DeclineOpportunity() error = %v", err)
	}
	if en, id := c.undoableEntry(t); en == nil || id == "" {
		t.Fatalf("after the decline: undoable entry %v, event %q; want an entry the undo belongs to", en, id)
	}
	c.undoLast(t)
	if err := c.skip(t, c.master, offer.GetId()); err != nil {
		t.Fatalf("SkipOpportunity() error = %v", err)
	}
	if en, id := c.undoableEntry(t); en == nil || id == "" {
		t.Errorf("after the skip: undoable entry %v, event %q; want an entry the undo belongs to", en, id)
	}
}

// TestMR034_OffersNobodyCanAnswerStopHoldingTheTurn: an offer whose reactor
// became incapacitated or spent its reaction is passed over, and a malformed
// offer id is invalid_argument.
func TestMR034_OffersNobodyCanAnswerStopHoldingTheTurn(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	e := c.oppFight(t)
	c.moveOffering(t, "Goblin 2", 6, 6) // a second reactor, beside Toren too
	if _, err := c.moveResponse(t, c.caio, "Toren", 3, 7); err != nil {
		t.Fatalf("MoveCombatant() error = %v", err)
	}
	if got := c.offersOf(t, c.master); len(got) != 2 {
		t.Fatalf("offers = %v, want two", got)
	}
	// Goblin 2 is incapacitated (any change prunes), its reaction spent for Goblin 1.
	if _, err := c.conditions(t, c.master, e, "Goblin 2", []string{"condition:stunned"}, true, false); err != nil {
		t.Fatalf("SetCombatantConditions() error = %v", err)
	}
	if got := c.offersOf(t, c.master); len(got) != 1 || got[0].GetReactorLabel() != "Goblin 1" {
		t.Fatalf("after Goblin 2 was stunned the offers are %v, want only Goblin 1's", got)
	}
	c.execSQL(t, `UPDATE combatants SET reaction_used = true WHERE id = $1`, c.id(t, "Goblin 1"))
	if _, err := c.conditions(t, c.master, e, "Goblin 1", []string{"condition:poisoned"}, true, false); err != nil {
		t.Fatalf("SetCombatantConditions() error = %v", err)
	}
	if got := c.offersOf(t, c.master); len(got) != 0 {
		t.Errorf("after Goblin 1 spent its reaction the offers are %v, want none", got)
	}
	if _, err := c.endTurn(t, c.caio, e, false); err != nil {
		t.Errorf("EndTurn with nothing left to answer error = %v", err)
	}
	wantCode(t, "a malformed offer id", c.decline(t, c.master, "not-a-uuid"), connect.CodeInvalidArgument)
	_, err := c.offerAttack(t, c.master, "Goblin 1", sword, "Toren", "not-a-uuid", d20(10))
	wantCode(t, "an attack with a malformed offer id", err, connect.CodeInvalidArgument)
}

// TestRN21_AReactorsReachIsItsLongestMeleeReach: the reach is the largest melee
// reach of the sheet, at least 5 ft; a thrown weapon's range is not a reach.
func TestRN21_AReactorsReachIsItsLongestMeleeReach(t *testing.T) {
	t.Parallel()
	sheet := link.Sheet{Attacks: []link.Attack{
		{Key: "bite", Melee: true},
		{Key: "constrict", Melee: true, RangeFt: 10},
		{Key: "javelin", Melee: true, RangeFt: 30, LongRangeFt: 120},
		{Key: "bow", RangeFt: 80, LongRangeFt: 320},
	}}
	if got := meleeReachOf(sheet); got != 10 {
		t.Errorf("meleeReachOf() = %d, want 10", got)
	}
	if got := meleeReachOf(link.Sheet{Attacks: []link.Attack{{Key: "bow", RangeFt: 80, LongRangeFt: 320}}}); got != 0 {
		t.Errorf("meleeReachOf() of a bow alone = %d, want 0: it cannot make an opportunity attack", got)
	}
	// A snake with 10 ft of reach is left at (2, 0): (2, 0) is 10 ft from it.
	reactor := playdb.Combatant{GridCol: new(int32(0)), GridRow: new(int32(0))}
	if got := provokedBy([]candidate{{who: reactor, reachFt: 10}}, grid.Square{Col: 2, Row: 0}, grid.Square{Col: 4, Row: 0}); len(got) != 1 {
		t.Errorf("a move from 10 ft to 20 ft of a 10 ft reach: provokers %v, want one", got)
	}
	if got := provokedBy([]candidate{{who: reactor, reachFt: 5}}, grid.Square{Col: 2, Row: 0}, grid.Square{Col: 4, Row: 0}); len(got) != 0 {
		t.Errorf("the same move with a 5 ft reach provoked: %v", got)
	}
}

// TestMR013_AJointTurnsReactorsAndCreaturesFollowTheWait: in a joint turn an
// enemy that acts in the same turn group may react (the SRD allows reactions on
// any turn).
func TestMR013_AJointTurnsReactorsAndCreaturesFollowTheWait(t *testing.T) {
	t.Parallel()
	a, e := jointFight(t, false)
	// The Capitão Goblin takes part in Brisa's turn (a mixed group is possible: the
	// master can set the turn state of a combatant that was added).
	a.execSQL(t, `UPDATE combatants SET turn_state = 'acting' WHERE id = $1`, a.id(t, "Capitão Goblin"))
	if _, err := a.bia.combat.MoveCombatant(t.Context(), connect.NewRequest(&playv1.MoveCombatantRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, "Brisa"), IdempotencyKey: newKey(), Col: 12, Row: 4,
	})); err != nil {
		t.Fatalf("Brisa's move error = %v", err)
	}
	var found bool
	for _, o := range a.get(t, a.master).GetOpportunityOffers() {
		found = found || o.GetReactorLabel() == "Capitão Goblin"
	}
	if !found {
		t.Errorf("the Capitão Goblin, in Brisa's own turn group, was not offered an attack: %v", a.get(t, a.master).GetOpportunityOffers())
	}
}

// TestMR034_TheWaitHoldsTheSamePlayersCreaturesInTheTurn: while Toren waits, his
// player's wolf in the same turn group does not act either (RN-21: "o jogador
// não age"); after the answer it does.
func TestMR034_TheWaitHoldsTheSamePlayersCreaturesInTheTurn(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.give(t, c.toren, "monster:wolf", "Presa")
	c.start(t, plan{
		npcs: []*playv1.Participant{
			{CharacterId: c.capitao.GetId()}, {CharacterId: c.goblins.GetId(), Count: 3}, {CharacterId: c.ogre.GetId()}, {CharacterId: c.squire.GetId()},
		},
		npcRolls: []int{2, 2, 2, 2, 2, 2},
		players:  map[string]int32{"Toren": 18, "Pensantus": 10, "Brisa": 1, "Presa": 15},
		reveal:   []string{"Capitão Goblin", "Goblin 1", "Goblin 2", "Goblin 3", "Ogro", "Escudeiro"},
		at: map[string][2]int32{
			"Toren": {6, 7}, "Pensantus": {5, 8}, "Brisa": {4, 7}, "Escudeiro": {3, 8}, "Presa": {8, 7},
			"Goblin 1": {7, 8}, "Goblin 2": {20, 7}, "Goblin 3": {21, 3}, "Capitão Goblin": {12, 13}, "Ogro": {22, 9},
		},
	})
	// The wolf takes part in Toren's turn (a joint turn of the character and its creature).
	c.execSQL(t, `UPDATE combatants SET turn_state = 'acting' WHERE id = $1`, c.id(t, "Presa"))
	c.leaveGoblin(t)
	_, err := c.move(t, c.caio, "Presa", 9, 7)
	wantEncounterBlocked(t, err, reasonOpportunityPending)
	if err := c.decline(t, c.master, c.offersOf(t, c.master)[0].GetId()); err != nil {
		t.Fatalf("DeclineOpportunity() error = %v", err)
	}
	if _, err := c.move(t, c.caio, "Presa", 9, 7); err != nil {
		t.Errorf("the wolf's move after the answer error = %v", err)
	}
}

// TestRN21_AProvokingMoveTellsWhoMustAnswer: the offers live in the combat, not in
// the move's hint, so a move that provokes sends encounter_changed to the master,
// the mover's player (their turn waits) and the reactors' players (their prompt),
// and to no other player: an offer of a reactor they may not see is not theirs to
// know about. Toren leaving Goblin 1's reach asks the master; Goblin 1 leaving
// Toren's asks Toren's player.
func TestRN21_AProvokingMoveTellsWhoMustAnswer(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		setup, move func(t *testing.T, c *cave)
	}{
		{"Toren leaves the goblin", func(*testing.T, *cave) {}, func(t *testing.T, c *cave) { c.leaveGoblin(t) }},
		{
			"the goblin leaves Toren",
			func(t *testing.T, c *cave) { c.passTo(t, c.get(t, c.master), "Goblin 1") },
			func(t *testing.T, c *cave) { c.moveOffering(t, "Goblin 1", 5, 9) }, // along a line through Toren's reach
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := newCave(t)
			c.oppFight(t)
			streams := map[string]*watcher{
				"the master": c.master.watch(t, c.campaignID), "Toren's player": c.caio.watch(t, c.campaignID),
				"Pensantus's player": c.ana.watch(t, c.campaignID), "Brisa's player": c.bia.watch(t, c.campaignID),
			}
			for _, w := range streams {
				w.ready(t)
			}
			brisa := c.id(t, "Brisa")
			// upTo reads every stream up to the master moving Brisa to (col, 7), which
			// everyone hears, so all that the change before it published has come by
			// then; it returns how many encounter_changed each one got.
			upTo := func(col int32) map[string]int {
				c.mustMove(t, c.master, "Brisa", col, 7)
				got := map[string]int{}
				for who, w := range streams {
					for {
						ev := w.nextChange(t)
						if m := ev.GetCombatantMoved(); m != nil && m.GetCombatantId() == brisa && m.GetCol() == col {
							break
						}
						if ev.GetEncounterChanged() != nil {
							got[who]++
						}
					}
				}
				return got
			}
			tc.setup(t, c)
			upTo(2)
			tc.move(t, c)
			if len(c.offersOf(t, c.master)) != 1 {
				t.Fatalf("the master's offers = %v, want one", c.offersOf(t, c.master))
			}
			got := upTo(1)
			for who, want := range map[string]bool{"the master": true, "Toren's player": true, "Pensantus's player": false, "Brisa's player": false} {
				if (got[who] > 0) != want {
					t.Errorf("after the provoking move %s got %d encounter_changed; want some: %v", who, got[who], want)
				}
			}
		})
	}
}

// TestMR034_AMoveThatClearsTheCoverMarkTellsEveryone: the master's cover mark is on
// the combatant, and everyone who sees it reads it; a move takes it off, and the
// move's own hint carries only the square, so the move also sends encounter_changed.
func TestMR034_AMoveThatClearsTheCoverMarkTellsEveryone(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.fight(t)
	c.mark(t, "Goblin 2", playv1.CoverDegree_COVER_DEGREE_HALF)
	if got := c.who(t, c.ana, "Goblin 2").GetCoverMark(); got != playv1.CoverDegree_COVER_DEGREE_HALF {
		t.Fatalf("a player reads Goblin 2's mark as %v, want half", got)
	}
	w := c.ana.watch(t, c.campaignID)
	w.ready(t)
	c.mustMove(t, c.master, "Goblin 2", 21, 7)
	// The move's own combatant_moved comes first; with no encounter_changed after it
	// the test fails on the time limit.
	for got := false; !got; {
		got = w.nextChange(t).GetEncounterChanged() != nil
	}
	if got := c.who(t, c.ana, "Goblin 2").GetCoverMark(); got != playv1.CoverDegree_COVER_DEGREE_NONE && got != playv1.CoverDegree_COVER_DEGREE_UNSPECIFIED {
		t.Errorf("after the move a player reads the mark %v, want none", got)
	}
}

// An offer waits for a hostile reactor: when the reactor changes to the
// mover's side the offer is passed over, and no attack on its new ally is made.
func TestAnOfferIsDroppedWhenItsReactorChangesSide(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.oppFight(t)
	c.leaveGoblin(t)
	offers := c.offersOf(t, c.master)
	if len(offers) != 1 {
		t.Fatalf("offers after leaving the reach = %v, want 1", offers)
	}
	offer := offers[0]

	c.side(t, "Goblin 1", playv1.CombatantSide_COMBATANT_SIDE_PARTY)

	if got := c.offersOf(t, c.master); len(got) != 0 {
		t.Errorf("after Goblin 1 joined the party the offer on Toren still waits: %v", got)
	}
	if _, err := c.offerAttack(t, c.master, "Goblin 1", sword, "Toren", offer.GetId(), d20(15)); err == nil {
		t.Errorf("an ally made an opportunity attack on its friend Toren")
	}
}

// Positive control: a side change that keeps the reactor hostile leaves the
// offer waiting.
func TestAnOfferStaysWhileItsReactorIsStillHostile(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.oppFight(t)
	c.leaveGoblin(t)
	c.side(t, "Goblin 1", playv1.CombatantSide_COMBATANT_SIDE_ENEMY)
	if got := c.offersOf(t, c.master); len(got) != 1 {
		t.Errorf("offers after keeping the goblin an enemy = %v, want 1", got)
	}
}
