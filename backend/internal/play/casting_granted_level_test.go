package play

import (
	"fmt"
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// A granted_spell with a cast level (MR-025): the grant casts the spell at that level, with no slot, and the
// spell scales with it. The feat is the master's and its names are invented; the spells are the SRD's. These
// tests need the database (MEURPG_TEST_DATABASE_URL).

// newLevelParty is newGracePartyWith one use of a resource and a grant of spell at castLevel (0: its own level).
func newLevelParty(t *testing.T, spell string, castLevel int32, edit ...func(*rulesv1.TableEffect)) (a *armed, feat string) {
	t.Helper()
	grant := &rulesv1.TableEffect{Type: "granted_spell", Spell: spell, Ability: "cha", Uses: graceUses, CastLevel: castLevel}
	for _, e := range edit {
		e(grant)
	}
	return newGracePartyWith(t, []*rulesv1.TableEffect{{Type: "resource", Resource: graceUses, Max: "1", Recharge: "long_rest"}, grant})
}

func grantedCast(feat string) func(*playv1.CastSpellRequest) {
	return func(r *playv1.CastSpellRequest) { r.GrantedBy = feat }
}

// noSlotSpent says no spell slot of any level is used.
func noSlotSpent(t *testing.T, a *armed) {
	t.Helper()
	for _, sl := range a.vitals(t, a.pens).GetSpellSlots() {
		if sl.GetUsed() != 0 {
			t.Errorf("a slot of level %d was spent: %d", sl.GetLevel(), sl.GetUsed())
		}
	}
}

// burnFight is the fight of the cone of Mãos Flamejantes east of Pensantus: Goblin 1, Goblin 2 and the Capitão are in
// it, with nobody in the way and Toren and Brisa out of it (see TestSaveSpellRollsOnceForTheCast). Their saves are
// queued: Goblin 1 fails, Goblin 2 saves and the Capitão fails.
func (a *armed) burnFight(t *testing.T) (*playv1.Encounter, []*playv1.SpellTarget) {
	t.Helper()
	e := a.castersFight(t, 2)
	a.put(t, "Goblin 1", 7, 5)
	a.put(t, "Capitão Goblin", 8, 6)
	a.h.roller.queue(5, 18, 12)
	return e, a.at(t, "Goblin 1", "Goblin 2", "Capitão Goblin")
}

// A granted Burning Hands (3d6, one more d6 for each slot level above the 1st) rolls the dice of the grant's
// cast level, spends no slot and one use of the resource.
func TestMR025_ACombatGrantRollsTheDiceOfItsCastLevel(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		castLevel int32
		dice      int32
	}{{0, 3}, {2, 4}, {9, 11}} {
		t.Run(fmt.Sprintf("cast level %d", tc.castLevel), func(t *testing.T) {
			t.Parallel()
			a, feat := newLevelParty(t, burningHands, tc.castLevel)
			e, targets := a.burnFight(t)
			cast := a.mustCast(t, a.ana, e, "Pensantus", burningHands, nil, targets, grantedCast(feat))
			if cast.GetCast().GetSlot() != nil || len(cast.GetCast().GetPendingDamages()) != 3 {
				t.Fatalf("cast = %v, want no slot and one pending damage for each of the 3 targets", cast.GetCast())
			}
			for _, p := range cast.GetCast().GetPendingDamages() {
				if p.GetDiceCount() != tc.dice || p.GetDiceSides() != 6 {
					t.Errorf("pending damage = %dd%d, want %dd6", p.GetDiceCount(), p.GetDiceSides(), tc.dice)
				}
			}
			if used, _ := resourceUsed(a.vitals(t, a.pens), graceUses); used != 1 {
				t.Errorf("grant uses = %d, want 1", used)
			}
			noSlotSpent(t, a)
		})
	}
}

// With also_with_slot, the cast with a slot has the slot's level and spends no use; the cast with none has the
// grant's cast level.
func TestMR025_ASlotCastOfAGrantKeepsTheSlotsLevel(t *testing.T) {
	t.Parallel()
	a, feat := newLevelParty(t, burningHands, 2, func(e *rulesv1.TableEffect) { e.AlsoWithSlot = true })
	e, targets := a.burnFight(t)
	cast := a.mustCast(t, a.ana, e, "Pensantus", burningHands, slotOfLevel(3), targets, grantedCast(feat))
	if cast.GetCast().GetSlot().GetLevel() != 3 || len(cast.GetCast().GetPendingDamages()) != 3 {
		t.Fatalf("cast = %v, want the 3rd-level slot and 3 pending damages", cast.GetCast())
	}
	if p := cast.GetCast().GetPendingDamages()[0]; p.GetDiceCount() != 5 {
		t.Errorf("pending damage = %dd%d, want 5d6 (the slot's level, not the grant's 2nd)", p.GetDiceCount(), p.GetDiceSides())
	}
	if used, _ := resourceUsed(a.vitals(t, a.pens), graceUses); used != 0 {
		t.Errorf("grant uses = %d, want 0 (a slot spends no use)", used)
	}
	if got := usedSlots(a.vitals(t, a.pens), 3); got != 1 {
		t.Errorf("3rd-level slots used = %d, want 1", got)
	}
}

// webOutcome casts a granted Web (2nd level, nothing that grows with the level) at a goblin and says what came
// of it.
func webOutcome(t *testing.T, castLevel int32) string {
	t.Helper()
	a, feat := newLevelParty(t, webSpell, castLevel)
	e := a.castersFight(t, 1)
	a.h.roller.queue(6)
	cast := a.mustCast(t, a.ana, e, "Pensantus", webSpell, nil, a.at(t, "Goblin"), grantedCast(feat)).GetCast()
	if cast.GetSlot() != nil {
		t.Errorf("web at cast level %d: slot = %v, want none", castLevel, cast.GetSlot())
	}
	if used, _ := resourceUsed(a.vitals(t, a.pens), graceUses); used != 1 {
		t.Errorf("web at cast level %d: grant uses = %d, want 1", castLevel, used)
	}
	noSlotSpent(t, a)
	var targets string
	for _, tg := range cast.GetTargets() {
		targets += fmt.Sprintf(" %v/%d", tg.GetSave().GetOutcome(), tg.GetSave().GetDc())
	}
	return fmt.Sprintf("concentrating=%v pending=%d targets:%s", cast.GetConcentrating(), len(cast.GetPendingDamages()), targets)
}

// A spell with nothing that grows with the level, cast through a grant at a higher level, is the same cast.
func TestMR025_AGrantAtAHigherLevelOfASpellThatDoesNotScaleIsTheSameCast(t *testing.T) {
	t.Parallel()
	own, higher := webOutcome(t, 0), webOutcome(t, 3)
	if own != higher {
		t.Errorf("web at cast level 3 = %q, want what it is at its own level: %q", higher, own)
	}
}

// The turn's options say the cast level of each grant: the effect's, the spell's own when it sets none, 0 for
// a cantrip.
func TestMR025_TheCombatOptionsSayTheGrantsCastLevel(t *testing.T) {
	t.Parallel()
	a, feat := newGracePartyWith(t, []*rulesv1.TableEffect{
		{Type: "granted_spell", Spell: burningHands, Ability: "cha", CastLevel: 3},
		{Type: "granted_spell", Spell: blessKey, Ability: "cha"},
		{Type: "granted_spell", Spell: lightSpell, Ability: "int"},
	})
	e := a.castersFight(t, 1)
	got := map[string]int32{}
	for _, s := range a.mustOptions(t, a.ana, e, "Pensantus").GetOptions().GetSpells() {
		if g := s.GetGrant(); g.GetSourceKey() == feat {
			got[s.GetSpell().GetKey()] = g.GetCastLevel()
		}
	}
	want := map[string]int32{burningHands: 3, blessKey: 1, lightSpell: 0}
	for k, w := range want {
		if g, ok := got[k]; !ok || g != w {
			t.Errorf("cast level of the grant of %s = %d (listed %v), want %d", k, g, ok, w)
		}
	}
}

// Outside a combat, a granted Cure Wounds (1d8, one more d8 for each slot level above the 1st) rolls the dice
// of the grant's cast level, with no slot and one use.
func TestMR025_AnOutsideGrantHealsAtItsCastLevel(t *testing.T) {
	t.Parallel()
	a, feat := newLevelParty(t, cureWounds, 2)
	a.hurt(t, a.toren, 5)
	a.h.roller.queue(6, 3)
	res := a.mustCastOut(t, a.ana, a.pens, cureWounds, nil, false, []*charactersv1.Character{a.toren}, grantedBy(feat))
	cast := res.GetCast()
	if cast.GetDiceCount() != 2 || cast.GetDiceSides() != 8 || cast.GetRollTotal() != 11 || cast.GetSlotLevel() != 0 {
		t.Errorf("cast = %dd%d total %d slot %d, want 2d8 rolled to 11 (6 + 3 and the +2 of Charisma) with no slot", cast.GetDiceCount(), cast.GetDiceSides(), cast.GetRollTotal(), cast.GetSlotLevel())
	}
	if used, _ := resourceUsed(a.vitals(t, a.pens), graceUses); used != 1 {
		t.Errorf("grant uses = %d, want 1", used)
	}
	noSlotSpent(t, a)
}

// Outside a combat, a spell with nothing that grows with the level cast through a grant at a higher level is the
// same cast: Mage Armor at the 3rd level gives the same armor class as at the 1st.
func TestMR025_AnOutsideGrantOfASpellThatDoesNotScaleIsTheSameCast(t *testing.T) {
	t.Parallel()
	a, feat := newLevelParty(t, mageArmorKey, 3)
	res := a.mustCastOut(t, a.ana, a.pens, mageArmorKey, nil, false, []*charactersv1.Character{a.toren}, grantedBy(feat))
	if cast := res.GetCast(); cast.GetStatus() != playv1.OutsideCastStatus_OUTSIDE_CAST_STATUS_ACTIVE || cast.GetDurationSeconds() != 8*3600 {
		t.Fatalf("Mage Armor = %v, want active for 8 hours", cast)
	}
	// Toren has Dexterity 14 (+2): 13 + 2, whatever the level.
	var ac int32
	for _, c := range a.casts(t, a.caio).GetActive() {
		ac = c.GetTargets()[0].GetArmorClass()
	}
	if ac != 15 {
		t.Errorf("armor class given = %d, want 15", ac)
	}
	if used, _ := resourceUsed(a.vitals(t, a.pens), graceUses); used != 1 {
		t.Errorf("grant uses = %d, want 1", used)
	}
	noSlotSpent(t, a)
}

// The read models say the level each grant casts at: the sheet's list and the castable spells.
func TestMR025_TheSheetAndTheCastOptionsSayTheGrantsCastLevel(t *testing.T) {
	t.Parallel()
	a, feat := newGracePartyWith(t, []*rulesv1.TableEffect{
		{Type: "granted_spell", Spell: cureWounds, Ability: "cha", CastLevel: 2},
		{Type: "granted_spell", Spell: blessKey, Ability: "cha"},
		{Type: "granted_spell", Spell: lightSpell, Ability: "int"},
	})
	want := map[string]int32{cureWounds: 2, blessKey: 1, lightSpell: 0}
	sheet := map[string]int32{}
	for _, g := range a.pens.GetDerived().GetGrantedSpells() {
		if g.GetSourceKey() == feat {
			sheet[g.GetSpell().GetKey()] = g.GetCastLevel()
		}
	}
	res, err := a.ana.casting.GetCastOptions(t.Context(), connect.NewRequest(&playv1.GetCastOptionsRequest{CampaignId: a.campaignID, CharacterId: a.pens.GetId()}))
	if err != nil {
		t.Fatalf("GetCastOptions() error = %v", err)
	}
	cast := map[string]int32{}
	for _, s := range res.Msg.GetSpells() {
		for _, g := range s.GetGrants() {
			if g.GetSourceKey() == feat {
				cast[s.GetSpell().GetKey()] = g.GetCastLevel()
			}
		}
	}
	for k, w := range want {
		if g, ok := sheet[k]; !ok || g != w {
			t.Errorf("the sheet's cast level of %s = %d (listed %v), want %d", k, g, ok, w)
		}
		if g, ok := cast[k]; !ok || g != w {
			t.Errorf("the castable spells' cast level of %s = %d (listed %v), want %d", k, g, ok, w)
		}
	}
}

// grantEntryOf is the targets the turn's options list for the grant of the spell: the nth entry of that key, where
// n is the place of the grant among the castable spells of that key (the character's own cast of it is another).
func grantEntryOf(t *testing.T, o *playv1.GetTurnOptionsResponse, spell, feat string) *playv1.SpellTargets {
	t.Helper()
	n := 0
	for _, s := range o.GetOptions().GetSpells() {
		if !s.GetEnabled() || s.GetSpell().GetKey() != spell {
			continue
		}
		if s.GetGrant().GetSourceKey() == feat {
			var entries []*playv1.SpellTargets
			for _, st := range o.GetSpellTargets() {
				if st.GetSpellKey() == spell {
					entries = append(entries, st)
				}
			}
			if n >= len(entries) {
				t.Fatalf("no targets listed for the grant of %s (entry %d of %d)", spell, n, len(entries))
			}
			return entries[n]
		}
		n++
	}
	t.Fatalf("the options list no enabled grant of %s", spell)
	return nil
}

// The turn's targets read a grant's entry at its cast level: Magic Missile cast through a grant at the 3rd level
// lists the 5 darts of that level, and Bless at the 2nd the 4 targets of that level, with no slot to say so.
func TestMR025_TheTurnTargetsOfAGrantAreAtItsCastLevel(t *testing.T) {
	t.Parallel()
	a, feat := newGracePartyWith(t, []*rulesv1.TableEffect{
		{Type: "granted_spell", Spell: magicMissileSpell, Ability: "cha", CastLevel: 3},
		{Type: "granted_spell", Spell: blessKey, Ability: "cha", CastLevel: 2},
	})
	opts := a.mustOptions(t, a.ana, a.castersFight(t, 1), "Pensantus")
	mm := grantEntryOf(t, opts, magicMissileSpell, feat)
	if got := mm.GetDarts(); len(got) != 1 || got[0].GetSlotLevel() != 3 || got[0].GetDarts() != 5 {
		t.Errorf("darts of the granted Magic Missile = %v, want 5 darts at level 3", got)
	}
	if mm.GetMaxTargets() != 5 {
		t.Errorf("max targets of the granted Magic Missile = %d, want 5", mm.GetMaxTargets())
	}
	if bless := grantEntryOf(t, opts, blessKey, feat); bless.GetMaxTargets() != 4 {
		t.Errorf("max targets of the granted Bless = %d, want 4 (3 and one more for the 2nd level)", bless.GetMaxTargets())
	}
}

// A cast of Bless through a grant at the 2nd level takes the 4 targets of that level and refuses a 5th.
func TestMR025_AGrantCastTakesTheTargetsOfItsCastLevel(t *testing.T) {
	t.Parallel()
	a, feat := newLevelParty(t, blessKey, 2)
	e := a.castersFight(t, 1)
	if _, err := a.cast(t, a.ana, e, "Pensantus", blessKey, nil, a.at(t, "Pensantus", "Toren", "Brisa", "Capitão Goblin", "Goblin"), grantedCast(feat)); err == nil {
		t.Fatal("CastSpell(Bless, 5 targets) error = nil, want a refusal: the grant casts at the 2nd level, 4 targets")
	}
	cast := a.mustCast(t, a.ana, e, "Pensantus", blessKey, nil, a.at(t, "Pensantus", "Toren", "Brisa", "Capitão Goblin"), grantedCast(feat))
	if cast.GetCast().GetSlot() != nil || len(cast.GetCast().GetTargets()) != 4 {
		t.Errorf("cast = %v, want no slot and 4 targets", cast.GetCast())
	}
	noSlotSpent(t, a)
}

// A retried cast of a granted Fog Cloud at the 3rd level answers with the area of that level (60 ft), as the first
// answer did, and the zone it laid is that size.
func TestMR025_ARetriedGrantCastAnswersWithTheAreaOfItsCastLevel(t *testing.T) {
	t.Parallel()
	a, feat := newLevelParty(t, fogCloudKey, 3)
	e := a.castersFight(t, 1)
	key := newKey()
	first, err := a.castKey(t, a.ana, e, "Pensantus", fogCloudKey, nil, nil, grantedCast(feat), key)
	if err != nil {
		t.Fatalf("CastSpell(Fog Cloud) error = %v", err)
	}
	again, err := a.castKey(t, a.ana, e, "Pensantus", fogCloudKey, nil, nil, grantedCast(feat), key)
	if err != nil {
		t.Fatalf("CastSpell(Fog Cloud) retried error = %v", err)
	}
	if n := len(first.GetArea().GetSquares()); n == 0 || len(again.GetArea().GetSquares()) != n {
		t.Errorf("area squares = %d, retried = %d, want the same and not none", n, len(again.GetArea().GetSquares()))
	}
	zones := zonesOf(a.get(t, a.master), fogCloudKey)
	if len(zones) != 1 || zones[0].GetSizeFt() != 60 {
		t.Fatalf("zones = %v, want one of 60 ft", zones)
	}
	if n := len(zones[0].GetSquares()); len(again.GetArea().GetSquares()) != n {
		t.Errorf("retried area = %d squares, want the %d of the zone (the 3rd level, 60 ft)", len(again.GetArea().GetSquares()), n)
	}
}
