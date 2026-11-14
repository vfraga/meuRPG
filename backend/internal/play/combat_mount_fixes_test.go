package play

import (
	"slices"
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// A mount that stops sharing its rider's total in the middle of the rider's turn (the rider removed or dismissed, the mount marked
// independent) has its own place back and leaves the turn that runs: the turn then passes to the group after the rider's.

// squireOnTheOgre is the cave with the Escudeiro (NPC rider) on the controlled Ogro, in the Escudeiro's own turn.
func (c *cave) squireOnTheOgre(t *testing.T) {
	t.Helper()
	c.rolledFight(t, []int{2, 2, 2, 2, 11, 15}, map[string]int32{"Toren": 18, "Pensantus": 10, "Brisa": 1})
	c.steed(t)
	c.mustMove(t, c.master, "Escudeiro", 6, 8)
	c.passTo(t, c.get(t, c.master), "Escudeiro")
	c.mustMount(t, c.master, "Escudeiro", "Ogro")
}

// leavesTheTurn checks the Ogro is out of the turn and the group on turn is the first one after the Escudeiro's place that is not the Ogro's own
// place in the order: the turn passed on as if the Ogro had never been in it.
func (c *cave) leavesTheTurn(t *testing.T, before []string, what string) {
	t.Helper()
	e := c.get(t, c.master)
	if st := c.joinRowOf(t, "Ogro").State; st != "idle" {
		t.Errorf("%s: the Ogro's turn_state = %q, want idle: its own place is not in the Escudeiro's group", what, st)
	}
	order := labels(e)
	at := slices.Index(before, "Escudeiro")
	want := order[at%len(order)]
	if at > 0 && order[at-1] == "Ogro" {
		want = order[at]
	}
	if cur := current(e); cur != want || group(e) != want {
		t.Errorf("%s: the turn is %q's (group %q), want %q's: the group after the Escudeiro's in %v", what, cur, group(e), want, order)
	}
}

func TestMountedCombat_ARiderThatLeavesMidTurnTakesItsMountOutOfTheTurn(t *testing.T) {
	t.Parallel()
	t.Run("removed", func(t *testing.T) {
		t.Parallel()
		c := newCave(t)
		c.squireOnTheOgre(t)
		before := c.orderNow(t)
		if _, err := c.master.combat.RemoveCombatant(t.Context(), connect.NewRequest(&playv1.RemoveCombatantRequest{
			CampaignId: c.campaignID, EncounterId: c.get(t, c.master).GetId(), CombatantId: c.id(t, "Escudeiro"), IdempotencyKey: newKey(),
		})); err != nil {
			t.Fatalf("RemoveCombatant(Escudeiro) error = %v", err)
		}
		c.leavesTheTurn(t, before, "the rider removed")
	})
	t.Run("marked independent", func(t *testing.T) {
		t.Parallel()
		c := newCave(t)
		c.squireOnTheOgre(t)
		c.mustMark(t, "Ogro", "independent")
		e := c.get(t, c.master)
		if st := c.joinRowOf(t, "Ogro").State; st != "idle" {
			t.Errorf("the Ogro marked independent: turn_state = %q, want idle", st)
		}
		if group(e) != "Escudeiro" {
			t.Errorf("group = %q, want the Escudeiro alone", group(e))
		}
		if e = c.mustEndPart(t, c.master, e, "Escudeiro"); group(e) == "Ogro" || current(e) == "Ogro" {
			t.Errorf("after the Escudeiro's part the turn is %q's, want the group after the Escudeiro's, not the Ogro's", current(e))
		}
	})
}

// TestMountedCombat_AReactionFeatureOfARiddenMountIsNotLimited: a reaction is not an action, so the limit of a controlled mount that
// carries a rider does not stop one taken with TakeAction. Toren, a monk, is made the mount of Goblin 1 in the table; his Slow Fall
// still works, and a feature of his turn's economy would not.
func TestMountedCombat_AReactionFeatureOfARiddenMountIsNotLimited(t *testing.T) {
	t.Parallel()
	r, e := monkFall(t)
	r.execSQL(t, `UPDATE combatants SET mount_kind = 'controlled' WHERE id = $1`, r.id(t, "Toren"))
	r.execSQL(t, `UPDATE combatants SET mounted_on_id = $1 WHERE id = $2`, r.id(t, "Toren"), r.id(t, "Goblin 1"))
	if _, err := r.action(t, r.master, e, "Toren", "standard:search"); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("the positive control: Search by the ridden mount = %v, want it refused (failed_precondition)", err)
	}
	if _, err := r.action(t, r.caio, e, "Toren", "feature:slow-fall"); err != nil {
		t.Fatalf("TakeAction(Slow Fall) by a ridden controlled mount error = %v, want it taken: a reaction is no action", err)
	}
	if _, amount := r.fallOf(t, "Toren"); amount != 10 {
		t.Errorf("the fall damage = %d after Slow Fall, want 10", amount)
	}
}

// TestMountedCombat_TheDriverDoesNotRollTheMountsReactionAttack: the master answers for the reactions of a mount the rider's player does not own.
// The driver's attack with the Ogro's opportunity offer is refused, as its damage roll is, so no pending damage is left that nobody but
// the master may roll.
func TestMountedCombat_TheDriverDoesNotRollTheMountsReactionAttack(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.ridden(t)
	c.passTo(t, c.get(t, c.master), "Goblin 1")
	c.moveOffering(t, "Goblin 1", 11, 7)
	var offerID string
	for _, o := range c.offersOf(t, c.master) {
		if o.GetReactorId() == c.id(t, "Ogro") {
			offerID = o.GetId()
		}
	}
	if offerID == "" {
		t.Fatalf("the Goblin left the Ogro's reach and the Ogro has no offer: %v", c.offersOf(t, c.master))
	}
	_, err := c.offerAttack(t, c.caio, "Ogro", "basic:0", "Goblin 1", offerID, d20(15))
	wantCode(t, "the driver's reaction attack with the mount", err, connect.CodePermissionDenied)
	var left int
	if err := c.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM pending_damages`).Scan(&left); err != nil || left != 0 {
		t.Errorf("the refused attack left %d pending damages (error %v), want none", left, err)
	}
	if _, err := c.offerAttack(t, c.master, "Ogro", "basic:0", "Goblin 1", offerID, d20(15)); err != nil {
		t.Errorf("the master's reaction attack for the mount error = %v, want it made", err)
	}
}

// TestMountedCombat_ALegendaryActionOfARiddenMountIsLimited: Draco carries Toren (a controlled mount), so its legendary text option and
// its legendary attack are actions it does not take; its legendary move is a move, which stays. A refusal spends nothing.
func TestMountedCombat_ALegendaryActionOfARiddenMountIsLimited(t *testing.T) {
	t.Parallel()
	a, e := onToren(t)
	a.execSQL(t, `UPDATE combatants SET mount_kind = 'controlled' WHERE id = $1`, a.id(t, "Draco"))
	a.execSQL(t, `UPDATE combatants SET mounted_on_id = $1 WHERE id = $2`, a.id(t, "Draco"), a.id(t, "Toren"))

	_, err := a.spendLegendary(t, a.master, e, "Draco", "detect")
	wantBlockedBy(t, "the legendary text option of a ridden mount", err, reasonMountActionLimited)
	_, err = a.legendaryAttack(t, e, "Draco", "tail-attack", dragonTail, "Toren", 20)
	wantBlockedBy(t, "the legendary attack of a ridden mount", err, reasonMountActionLimited)
	if got := a.spentOn(t, "Draco"); got != 0 {
		t.Errorf("the refused options spent %d legendary actions, want none", got)
	}

	a.execSQL(t, `UPDATE combatants SET mounted_on_id = NULL WHERE id = $1`, a.id(t, "Toren"))
	if _, err := a.spendLegendary(t, a.master, e, "Draco", "detect"); err != nil {
		t.Errorf("the positive control: Detect by Draco with nobody on it error = %v, want it spent", err)
	}
}

// TestMountedCombat_AMountThatJoinsKeepsTheReactionItSpent: the Ogro spent its reaction this round and its own turn is still to come. It
// joins Toren's part with a fresh action, bonus action and movement, and the reaction it spent stays spent, with its record.
func TestMountedCombat_AMountThatJoinsKeepsTheReactionItSpent(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	e := c.fight(t)
	c.steed(t)
	c.execSQL(t, `UPDATE combatants SET reaction_used = true, reaction_kind = 'opportunity', reaction_round = $2, action_used = true WHERE id = $1`, c.id(t, "Ogro"), e.GetRound())
	c.mustMount(t, c.caio, "Toren", "Ogro")
	var used bool
	var kind *string
	var round *int32
	var action bool
	if err := c.h.pool.QueryRow(t.Context(), `SELECT reaction_used, reaction_kind, reaction_round, action_used FROM combatants WHERE id = $1`, c.id(t, "Ogro")).Scan(&used, &kind, &round, &action); err != nil {
		t.Fatalf("read the Ogro's reaction: %v", err)
	}
	if action {
		t.Error("the Ogro's action is still spent: its part of the turn starts fresh (positive control)")
	}
	if !used || kind == nil || *kind != "opportunity" || round == nil || *round != e.GetRound() {
		t.Errorf("the Ogro's reaction after joining = used %v kind %v round %v, want it spent still, as opportunity in round %d", used, kind, round, e.GetRound())
	}
}

// TestMountedCombat_TheTieOrderKeepsAMountRightAfterItsRider: the Ogro carries the Escudeiro and Goblin 1 ties with them on the same total
// and bonus. The master orders the tie with the mount first, or without naming the mount: the mount stays right after its rider, and
// the three are ordered (the pair's tie flag is one: the mount's flag follows the rider's).
func TestMountedCombat_TheTieOrderKeepsAMountRightAfterItsRider(t *testing.T) {
	t.Parallel()
	for name, ids := range map[string][]string{
		"the mount named first":     {"Ogro", "Goblin 1", "Escudeiro"},
		"the mount not named":       {"Goblin 1", "Escudeiro"},
		"the mount named after all": {"Goblin 1", "Escudeiro", "Ogro"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := newCave(t)
			c.squireOnTheOgre(t)
			squire := c.joinRowOf(t, "Escudeiro")
			c.execSQL(t, `UPDATE combatants SET initiative = $2, initiative_bonus = $3, tie_ordered = false WHERE id = $1`, c.id(t, "Goblin 1"), *squire.Initiative, squire.Bonus)
			c.execSQL(t, `UPDATE combatants SET tie_ordered = false WHERE id = ANY($1)`, []string{c.id(t, "Escudeiro"), c.id(t, "Ogro")})

			var want []string
			for _, l := range ids {
				want = append(want, c.id(t, l))
			}
			if _, err := c.master.combat.SetInitiativeOrder(t.Context(), connect.NewRequest(&playv1.SetInitiativeOrderRequest{
				CampaignId: c.campaignID, EncounterId: c.get(t, c.master).GetId(), IdempotencyKey: newKey(), CombatantIds: want,
			})); err != nil {
				t.Fatalf("SetInitiativeOrder() error = %v", err)
			}
			order := c.orderNow(t)
			if at := slices.Index(order, "Escudeiro"); at < 0 || at+1 >= len(order) || order[at+1] != "Ogro" {
				t.Errorf("order = %v, want the Ogro right after the Escudeiro", order)
			}
			for _, l := range []string{"Escudeiro", "Ogro", "Goblin 1"} {
				if !c.joinRowOf(t, l).Tie {
					t.Errorf("%s is not tie_ordered after the master ordered the tie", l)
				}
			}
		})
	}
}

// TestMountedCombat_AMountWhoseNeighbourLeftComesBackFirstAmongItsTie: mount_prior_init.after names who stood right before the mount among
// the combatants it ties with. When that combatant is gone, the mount comes back first among them: the order stays deterministic and the
// rest keep theirs.
func TestMountedCombat_AMountWhoseNeighbourLeftComesBackFirstAmongItsTie(t *testing.T) {
	t.Parallel()
	total := func(n int32) *int32 { return &n }
	row := func(id string, i int32) playdb.Combatant {
		return playdb.Combatant{ID: id, OrderIndex: i, Initiative: total(8)}
	}
	cs := []playdb.Combatant{row("a", 0), row("b", 1), row("mount", 2)} // "gone" stood before the mount and left the combat
	order := func(after string) []string {
		var ids []string
		for _, c := range orderCombatants(placedAfter(cs, map[int]string{2: after})) {
			ids = append(ids, c.ID)
		}
		return ids
	}
	if got, want := order("a"), []string{"a", "mount", "b"}; !slices.Equal(got, want) {
		t.Errorf("order with its neighbor there = %v, want %v (the positive control)", got, want)
	}
	if got, want := order("gone"), []string{"mount", "a", "b"}; !slices.Equal(got, want) {
		t.Errorf("order with its neighbor gone = %v, want %v", got, want)
	}
	if got, want := order(""), []string{"mount", "a", "b"}; !slices.Equal(got, want) {
		t.Errorf("order of a mount that stood first = %v, want %v", got, want)
	}
}

// TestMountedCombat_TheDriverReadsNothingMasterOnlyOfTheMount: the rider's player directs the Ogro without owning it. The turn options of the
// mount carry no pending damage (the dice of the Ogro's attack are the master's), no contest state (what the Ogro would roll to escape or
// to hide), no effect notes and no jump limits. The master reads the pending damage (the positive control), and reads the mount's Hide and
// Help as disabled, as every other action it does not take.
func TestMountedCombat_TheDriverReadsNothingMasterOnlyOfTheMount(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.ridden(t)
	onTurn := c.mustOptions(t, c.master, c.get(t, c.master), "Ogro") // Toren's turn: Hide and Help would be open to a creature that takes them
	c.passTo(t, c.get(t, c.master), "Goblin 1")
	c.moveOffering(t, "Goblin 1", 11, 7)
	var offerID string
	for _, o := range c.offersOf(t, c.master) {
		if o.GetReactorId() == c.id(t, "Ogro") {
			offerID = o.GetId()
		}
	}
	if offerID == "" {
		t.Fatalf("the Ogro has no offer: %v", c.offersOf(t, c.master))
	}
	if _, err := c.offerAttack(t, c.master, "Ogro", "basic:0", "Goblin 1", offerID, d20(15)); err != nil {
		t.Fatalf("the master's reaction attack for the Ogro error = %v", err)
	}
	e := c.get(t, c.master)

	master := c.mustOptions(t, c.master, e, "Ogro")
	if len(master.GetPendingDamages()) != 1 {
		t.Fatalf("the master reads %d pending damages of the Ogro, want 1 (the positive control)", len(master.GetPendingDamages()))
	}
	if len(onTurn.GetContestState().GetHideActions()) == 0 {
		t.Error("the master reads no Hide action of the mount: the positive control failed")
	}
	for _, a := range onTurn.GetContestState().GetHideActions() {
		if a.GetEnabled() {
			t.Errorf("the master reads the mount's %s as enabled", a.GetKey())
		}
	}
	if h := onTurn.GetContestState().GetHelpAction(); h == nil || h.GetEnabled() {
		t.Errorf("the master reads the mount's Help as %v, want it listed and disabled", h)
	}

	driver := c.mustOptions(t, c.caio, e, "Ogro")
	if p := driver.GetPendingDamages(); len(p) != 0 {
		t.Errorf("the driver reads the Ogro's pending damage %v, want none: its dice are the master's", p)
	}
	if st := driver.GetContestState(); st != nil {
		t.Errorf("the driver reads the Ogro's contest state %v, want none", st)
	}
	if n := driver.GetEffectNotes(); len(n) != 0 {
		t.Errorf("the driver reads the Ogro's effect notes %v, want none", n)
	}
	if j := driver.GetOptions().GetJumps(); j != nil {
		t.Errorf("the driver reads the Ogro's jump limits %v, want none", j)
	}
}
