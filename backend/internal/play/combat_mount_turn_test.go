package play

import (
	"slices"
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The turn of a pair (RN-19, RN-20, SRD 5.1, "Mounted Combat"): the rider and its controlled mount are one joint group, the turn passes when
// both parts end, and a mount the players do not see is never in what they read of the group.

// TestMountedCombat_TheTurnPassesWhenBothPartsEnd: the pair is a joint group. The rider's player ends the rider's part and the turn stays;
// the mount's part ends by the rider's player or by the master, in either order, and then the turn passes. Another player cannot end it.
func TestMountedCombat_TheTurnPassesWhenBothPartsEnd(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		first, second string // whose part ends first and second
		by            func(c *cave) (first, second *user)
	}{
		"the rider's player ends both":                                           {"Toren", "Ogro", func(c *cave) (*user, *user) { return c.caio, c.caio }},
		"the master ends the mount's part last":                                  {"Toren", "Ogro", func(c *cave) (*user, *user) { return c.caio, c.master }},
		"the master ends the mount's part first":                                 {"Ogro", "Toren", func(c *cave) (*user, *user) { return c.master, c.caio }},
		"the rider's player ends the mount's part first, the master the rider's": {"Ogro", "Toren", func(c *cave) (*user, *user) { return c.caio, c.master }},
		"the master ends both parts, in turn":                                    {"Toren", "Ogro", func(c *cave) (*user, *user) { return c.master, c.master }},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := newCave(t)
			c.fight(t)
			c.steed(t)
			c.mustMount(t, c.caio, "Toren", "Ogro")
			if got := group(c.get(t, c.master)); got != "Toren,Ogro" {
				t.Fatalf("group = %q, want Toren,Ogro", got)
			}
			one, two := tc.by(c)
			e, err := c.endPart(t, one, c.get(t, one), tc.first)
			if err != nil {
				t.Fatalf("EndTurn(%s's part) error = %v", tc.first, err)
			}
			if got := group(e); got != "Toren,Ogro" {
				t.Errorf("after %s's part ended, the group = %q, want Toren,Ogro: the other part still runs", tc.first, got)
			}
			if !byLabel(t, e, tc.first).GetTurnPartEnded() || byLabel(t, e, tc.second).GetTurnPartEnded() {
				t.Errorf("turn_part_ended after %s's part: %s %v, %s %v, want only the first", tc.first,
					tc.first, byLabel(t, e, tc.first).GetTurnPartEnded(), tc.second, byLabel(t, e, tc.second).GetTurnPartEnded())
			}
			e = c.mustEndPart(t, two, c.get(t, two), tc.second)
			if current(e) != "Pensantus" || group(e) != "Pensantus" {
				t.Errorf("after both parts the turn is %q's (group %q), want Pensantus's", current(e), group(e))
			}
		})
	}
}

// TestMountedCombat_AnotherPlayerEndsNoPartOfThePair: Pensantus's player cannot end Toren's part nor the Ogro's, which is a mount of Toren's.
func TestMountedCombat_AnotherPlayerEndsNoPartOfThePair(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.fight(t)
	c.steed(t)
	c.mustMount(t, c.caio, "Toren", "Ogro")
	for _, label := range []string{"Toren", "Ogro"} {
		if _, err := c.endPart(t, c.ana, c.get(t, c.ana), label); connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Errorf("another player ending %s's part = %v, want permission_denied", label, err)
		}
	}
	if got := group(c.get(t, c.master)); got != "Toren,Ogro" {
		t.Errorf("group after the refused ends = %q, want Toren,Ogro", got)
	}
}

// TestMountedCombat_ADefeatedMountDoesNotHoldTheTurn: the Ogro falls to 0 while it carries Toren. The pair's order stays, and the turn passes when
// Toren's part ends: a defeated member has no part to end. When Toren gets off the Ogro goes back to its own place.
func TestMountedCombat_ADefeatedMountDoesNotHoldTheTurn(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.fight(t)
	c.steed(t)
	base := c.orderNow(t)
	c.mustMount(t, c.caio, "Toren", "Ogro")
	c.execSQL(t, `UPDATE combatants SET defeated = true, hp_current = 0 WHERE id = $1`, c.id(t, "Ogro"))
	if got := c.orderNow(t); !slices.Equal(got, orderWith(base, "Toren", "Ogro")) {
		t.Errorf("order with the mount defeated = %v, want the pair's", got)
	}
	e := c.mustEndPart(t, c.caio, c.get(t, c.caio), "Toren")
	if current(e) != "Pensantus" {
		t.Errorf("after Toren's part the turn is %q's, want Pensantus's: the defeated mount holds nothing", current(e))
	}
	c.passTo(t, e, "Toren")
	c.freeTheRide(t, "Toren")
	if _, err := c.dismountAs(t, c.caio, "Toren", 6, 7); err != nil {
		t.Fatalf("DismountCombatant() of a defeated mount error = %v", err)
	}
	if got := c.orderNow(t); !slices.Equal(got, base) {
		t.Errorf("order after getting off a defeated mount = %v, want %v", got, base)
	}
	if c.priorOf(t, "Ogro") != nil {
		t.Error("mount_prior_init of the defeated mount is still there after the dismount")
	}
}

// TestMountedCombat_ARemovedMountLeavesTheRidersGroupWhole: the master removes the Ogro while Toren rides it: the link goes, the order has no
// stale place, and Toren's turn is his alone: ending his part passes it.
func TestMountedCombat_ARemovedMountLeavesTheRidersGroupWhole(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.fight(t)
	c.steed(t)
	base := c.orderNow(t)
	toren := c.joinRowOf(t, "Toren")
	c.mustMount(t, c.caio, "Toren", "Ogro")

	if _, err := c.master.combat.RemoveCombatant(t.Context(), connect.NewRequest(&playv1.RemoveCombatantRequest{
		CampaignId: c.campaignID, EncounterId: c.get(t, c.master).GetId(), CombatantId: c.id(t, "Ogro"), IdempotencyKey: newKey(),
	})); err != nil {
		t.Fatalf("RemoveCombatant(Ogro) error = %v", err)
	}
	if got, want := c.orderNow(t), slices.DeleteFunc(slices.Clone(base), func(l string) bool { return l == "Ogro" }); !slices.Equal(got, want) {
		t.Errorf("order after the removal = %v, want %v", got, want)
	}
	if got := c.joinRowOf(t, "Toren"); got.Initiative == nil || *got.Initiative != *toren.Initiative || got.Index != toren.Index {
		t.Errorf("Toren after the removal = %+v, want his place %+v", got, toren)
	}
	if on := c.rideRowOf(t, "Toren").MountedOn; on != "" {
		t.Errorf("Toren still rides %s after the removal of the mount", on)
	}
	e := c.get(t, c.master)
	if group(e) != "Toren" {
		t.Errorf("group after the removal = %q, want Toren alone", group(e))
	}
	if e = c.mustEndPart(t, c.caio, e, "Toren"); current(e) != "Pensantus" {
		t.Errorf("after Toren's part the turn is %q's, want Pensantus's", current(e))
	}
}

// TestMountedCombat_ARemovedRiderReturnsItsMountToItsPlace: the master removes the Escudeiro, an NPC rider of the Ogro, in its own turn. The Ogro
// is back at its own total and place, with no prior initiative left.
func TestMountedCombat_ARemovedRiderReturnsItsMountToItsPlace(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.rolledFight(t, []int{2, 2, 2, 2, 11, 15}, map[string]int32{"Toren": 18, "Pensantus": 10, "Brisa": 1})
	c.steed(t)
	base := c.orderNow(t)
	ogre := c.joinRowOf(t, "Ogro")
	c.mustMove(t, c.master, "Escudeiro", 6, 8)
	c.passTo(t, c.get(t, c.master), "Escudeiro")
	c.mustMount(t, c.master, "Escudeiro", "Ogro")
	if got := c.orderNow(t); !slices.Equal(got, orderWith(base, "Escudeiro", "Ogro")) {
		t.Fatalf("order with the Escudeiro on the Ogro = %v", got)
	}

	if _, err := c.master.combat.RemoveCombatant(t.Context(), connect.NewRequest(&playv1.RemoveCombatantRequest{
		CampaignId: c.campaignID, EncounterId: c.get(t, c.master).GetId(), CombatantId: c.id(t, "Escudeiro"), IdempotencyKey: newKey(),
	})); err != nil {
		t.Fatalf("RemoveCombatant(Escudeiro) error = %v", err)
	}
	back := c.joinRowOf(t, "Ogro")
	if back.Initiative == nil || *back.Initiative != *ogre.Initiative || back.Prior != "" {
		t.Errorf("the Ogro after its rider left = %+v, want its own total %d and no prior initiative", back, *ogre.Initiative)
	}
	want := slices.DeleteFunc(slices.Clone(base), func(l string) bool { return l == "Escudeiro" })
	if got := c.orderNow(t); !slices.Equal(got, want) {
		t.Errorf("order after the rider left = %v, want %v", got, want)
	}
}

// TestMountedCombat_AHiddenMountIsNeverInWhatPlayersReadOfTheTurn: the master seats Toren on the hidden Ogro. What Toren's player and Pensantus's
// player read is what they read without it: the group on turn, turn_part_ended, the NPC groups, the order and the order row of Toren; the
// Ogro is in none of them. The master reads the pair (the positive control), and the players read it once the Ogro is revealed.
func TestMountedCombat_AHiddenMountIsNeverInWhatPlayersReadOfTheTurn(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.fight(t)
	c.steed(t)
	c.hide(t, "Ogro")
	readers := map[string]*user{"Toren's player": c.caio, "Pensantus's player": c.ana}
	before := map[string]*playv1.Encounter{}
	for who, u := range readers {
		before[who] = c.get(t, u)
	}

	if _, err := c.mountAs(t, c.master, "Toren", "Ogro"); err != nil {
		t.Fatalf("the master mounting Toren on the hidden Ogro: %v", err)
	}

	if got := group(c.get(t, c.master)); got != "Toren,Ogro" {
		t.Fatalf("the master reads the group %q, want Toren,Ogro", got)
	}
	for who, u := range readers {
		e := c.get(t, u)
		if got := group(e); got != "Toren" {
			t.Errorf("%s reads the group %q, want Toren alone", who, got)
		}
		if got, was := labels(e), labels(before[who]); !slices.Equal(got, was) {
			t.Errorf("%s reads the order %v, want %v", who, got, was)
		}
		if n := len(e.GetNpcOnlyGroups()); n != len(before[who].GetNpcOnlyGroups()) {
			t.Errorf("%s reads %d NPC groups, want %d", who, n, len(before[who].GetNpcOnlyGroups()))
		}
		for _, cb := range e.GetCombatants() {
			was := byLabel(t, before[who], cb.GetLabel())
			if cb.GetInitiative() != was.GetInitiative() || cb.GetInitiativeBonus() != was.GetInitiativeBonus() || cb.GetInitiativeFace() != was.GetInitiativeFace() {
				t.Errorf("%s reads the order row of %s as %d/%d/%d, want %d/%d/%d", who, cb.GetLabel(),
					cb.GetInitiative(), cb.GetInitiativeBonus(), cb.GetInitiativeFace(), was.GetInitiative(), was.GetInitiativeBonus(), was.GetInitiativeFace())
			}
			if cb.GetTieUnresolved() {
				t.Errorf("%s reads %s as tied: a player never reads the flag", who, cb.GetLabel())
			}
			if cb.GetLabel() == "Ogro" || cb.GetMountedOnId() != "" {
				t.Errorf("%s reads %s carrying the hidden mount (%q)", who, cb.GetLabel(), cb.GetMountedOnId())
			}
		}
	}
	// Toren's part ends: the players read it ended and nothing of a second part; the turn still waits for the Ogro's part, which is the master's.
	e := c.mustEndPart(t, c.caio, c.get(t, c.caio), "Toren")
	if got := group(e); got != "Toren" || !byLabel(t, e, "Toren").GetTurnPartEnded() {
		t.Errorf("after Toren's part, his player reads group %q and turn_part_ended %v", got, byLabel(t, e, "Toren").GetTurnPartEnded())
	}
	for _, cb := range c.get(t, c.ana).GetCombatants() {
		if cb.GetLabel() == "Ogro" {
			t.Error("Pensantus's player reads the hidden Ogro in the combatants")
		}
	}
	if e = c.get(t, c.master); group(e) != "Toren,Ogro" || current(e) != "Ogro" || byLabel(t, e, "Ogro").GetTurnPartEnded() || !byLabel(t, e, "Toren").GetTurnPartEnded() {
		t.Errorf("the master reads group %q, current %q, Ogro's part ended %v, Toren's %v; want Toren,Ogro with the Ogro's part running",
			group(e), current(e), byLabel(t, e, "Ogro").GetTurnPartEnded(), byLabel(t, e, "Toren").GetTurnPartEnded())
	}
	// Revealed, the Ogro is in the group for the players.
	c.reveal(t, "Ogro")
	if got := group(c.get(t, c.caio)); got != "Toren,Ogro" {
		t.Errorf("Toren's player reads the group %q once the Ogro is revealed, want Toren,Ogro", got)
	}
}

// TestMountedCombat_AMountOfTwoNPCsIsAGroupForPlayersOnlyWhenSeen: the Escudeiro rides the Ogro, both NPCs: the NPC groups a player reads carry the
// pair once both are seen, and a hidden Ogro never shows the Escudeiro in a bigger group.
func TestMountedCombat_AMountOfTwoNPCsIsAGroupForPlayersOnlyWhenSeen(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.rolledFight(t, []int{2, 2, 2, 2, 11, 15}, map[string]int32{"Toren": 18, "Pensantus": 10, "Brisa": 1})
	c.steed(t)
	c.mustMove(t, c.master, "Escudeiro", 6, 8)
	c.passTo(t, c.get(t, c.master), "Escudeiro")
	c.mustMount(t, c.master, "Escudeiro", "Ogro")

	pairIn := func(e *playv1.Encounter) bool {
		for _, g := range e.GetNpcOnlyGroups() {
			if slices.Contains(g.GetCombatantIds(), c.id(t, "Escudeiro")) && slices.Contains(g.GetCombatantIds(), c.id(t, "Ogro")) {
				return true
			}
		}
		return false
	}
	if !pairIn(c.get(t, c.caio)) {
		t.Errorf("a player does not read the Escudeiro and the Ogro as an NPC group: %v", c.get(t, c.caio).GetNpcOnlyGroups())
	}
	c.hide(t, "Ogro")
	e := c.get(t, c.caio)
	if pairIn(e) {
		t.Error("a player reads an NPC group with the hidden Ogro")
	}
	for _, g := range e.GetNpcOnlyGroups() {
		if slices.Contains(g.GetCombatantIds(), c.id(t, "Escudeiro")) {
			t.Errorf("the Escudeiro is in the NPC group %v with the Ogro hidden", g.GetCombatantIds())
		}
	}
	if got := group(c.get(t, c.master)); got != "Escudeiro,Ogro" {
		t.Errorf("the master reads the group %q, want Escudeiro,Ogro", got)
	}
}
