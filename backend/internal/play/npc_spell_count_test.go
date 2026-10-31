package play

import (
	"strings"
	"testing"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// TestSpentOfReadsWhatAnNPCFightSpent: the count on the combatant (slot levels, "pact" and
// "res:<key>") reaches the options as the usage the rules read; a player's character is
// counted in its vitals and gives nothing here.
func TestSpentOfReadsWhatAnNPCFightSpent(t *testing.T) {
	t.Parallel()
	body := []byte(`{"1":2,"3":1,"pact":1,"res:innate_3_day_bless":2,"10":4,"x":1}`)
	got := spentOf(playdb.Combatant{Kind: kindNPC, SlotsUsed: body})
	want := link.Spent{SlotsUsed: [9]int{2, 0, 1}, PactSlotsUsed: 1, ResourcesUsed: map[string]int{"innate_3_day_bless": 2}}
	if got.SlotsUsed != want.SlotsUsed || got.PactSlotsUsed != want.PactSlotsUsed || len(got.ResourcesUsed) != 1 || got.ResourcesUsed["innate_3_day_bless"] != 2 {
		t.Errorf("spentOf(npc) = %+v, want %+v", got, want)
	}
	if p := spentOf(playdb.Combatant{Kind: kindPlayer, SlotsUsed: body}); p.SlotsUsed != [9]int{} || p.PactSlotsUsed != 0 || p.ResourcesUsed != nil {
		t.Errorf("spentOf(player) = %+v, want nothing: its vitals count", p)
	}
	if e := spentOf(playdb.Combatant{Kind: kindNPC}); e.SlotsUsed != [9]int{} || e.ResourcesUsed != nil {
		t.Errorf("spentOf(no count) = %+v, want nothing", e)
	}
}

// usedOf reads the count an NPC's fight spent, as stored.
func (a *armed) usedOf(t *testing.T, label string) string {
	t.Helper()
	var used string
	if err := a.h.pool.QueryRow(t.Context(), `SELECT slots_used::TEXT FROM combatants WHERE id = $1`, a.id(t, label)).Scan(&used); err != nil {
		t.Fatalf("slots_used of %s: %v", label, err)
	}
	return used
}

// TestAnNPCMagesCastSpendsASlotAndTheUndoGivesItBack: the Mage casts Mísseis Mágicos with a
// 1st-level slot as the master; the count on the combatant takes it, and the undo
// gives it back. Needs the database.
func TestAnNPCMagesCastSpendsASlotAndTheUndoGivesItBack(t *testing.T) {
	t.Parallel()
	a, e := mageFight(t)
	e = a.passTo(t, e, "Mago")
	a.mustCast(t, a.master, e, "Mago", magicMissileSpell, slotOfLevel(1), []*playv1.SpellTarget{darts(a, t, "Toren", 3)}, noCastRoll)
	if used := a.usedOf(t, "Mago"); !strings.Contains(used, `"1": 1`) {
		t.Fatalf("the Mage's count after the cast = %s, want one 1st-level slot", used)
	}
	a.undoLast(t, e)
	if used := a.usedOf(t, "Mago"); used != `{"1": 0}` {
		t.Errorf("the Mage's count after the undo = %s, want the level kept with no use", used)
	}
}

// freeSlotsOf reads the free slots of a level a spell is offered with.
func freeSlotsOf(res *playv1.GetTurnOptionsResponse, spell string, level int32) (free, total int32, ok bool) {
	for _, sp := range res.GetOptions().GetSpells() {
		if sp.GetSpell().GetKey() != spell {
			continue
		}
		for _, s := range sp.GetSlots() {
			if s.GetLevel() == level {
				return s.GetFree(), s.GetTotal(), true
			}
		}
	}
	return 0, 0, false
}

// TestAnNPCMagesSlotsLeftAreTheMastersAlone (RN-20, RN-29): the master's options for the Mage
// show its 1st-level slots, one less after a cast, and the options the cast sheet already
// carries need no screen change; a player asking for the same options is refused, so the
// slots, the spells and the uses left never reach the table. Needs the database.
func TestAnNPCMagesSlotsLeftAreTheMastersAlone(t *testing.T) {
	t.Parallel()
	a, e := mageFight(t)
	e = a.passTo(t, e, "Mago")
	before := a.mustOptions(t, a.master, e, "Mago")
	free, total, ok := freeSlotsOf(before, magicMissileSpell, 1) // positive control: the master reads them
	if !ok || total != 4 || free != 4 {
		t.Fatalf("the master reads %d of %d 1st-level slots (offered %v), want 4 of 4", free, total, ok)
	}
	a.mustCast(t, a.master, e, "Mago", magicMissileSpell, slotOfLevel(1), []*playv1.SpellTarget{darts(a, t, "Toren", 3)}, noCastRoll)
	after := a.mustOptions(t, a.master, a.get(t, a.master), "Mago")
	if free, _, _ := freeSlotsOf(after, magicMissileSpell, 1); free != 3 {
		t.Errorf("the master reads %d free 1st-level slots after a cast, want 3", free)
	}
	for _, player := range []*user{a.caio, a.ana} {
		res, err := a.options(t, player, a.get(t, a.master), "Mago")
		if err == nil {
			t.Errorf("a player reads the Mage's options = %v, want them refused", res)
		}
	}
}

// grantOf reads the grant a spell is offered with.
func grantOf(res *playv1.GetTurnOptionsResponse, spell string) *rulesv1.SpellGrantOption {
	for _, sp := range res.GetOptions().GetSpells() {
		if sp.GetSpell().GetKey() == spell && sp.GetGrant() != nil {
			return sp.GetGrant()
		}
	}
	return nil
}

// TestAnNPCsInnateSpellsCountTheirUses: the Couatl casts Bless 3/day with the stat block's
// numbers; the use is counted on the combatant, the master's options show one less, the undo gives
// it back, and Detect Magic at will spends nothing. Needs the database.
func TestAnNPCsInnateSpellsCountTheirUses(t *testing.T) {
	t.Parallel()
	a, e := monsterFight(t, "monster:couatl", "Couatl")
	e = a.passTo(t, e, "Couatl")
	first := grantOf(a.mustOptions(t, a.master, e, "Couatl"), "spell:bless")
	if first == nil || first.GetUsesLeft() != 3 { // positive control: the master reads the uses
		t.Fatalf("the master reads the Bless grant = %v, want 3 uses left", first)
	}
	byGrant := func(r *playv1.CastSpellRequest) { r.GrantedBy = first.GetSourceKey() }
	a.mustCast(t, a.master, e, "Couatl", "spell:bless", nil, a.at(t, "Toren"), byGrant)
	if used := a.usedOf(t, "Couatl"); used != `{"res:innate_3_day_bless": 1}` {
		t.Fatalf("the Couatl's count after Bless = %s, want one use of the resource", used)
	}
	if g := grantOf(a.mustOptions(t, a.master, a.get(t, a.master), "Couatl"), "spell:bless"); g.GetUsesLeft() != 2 {
		t.Errorf("the master reads %d uses left after Bless, want 2", g.GetUsesLeft())
	}
	a.undoLast(t, a.get(t, a.master))
	if used := a.usedOf(t, "Couatl"); used != `{"res:innate_3_day_bless": 0}` {
		t.Errorf("the Couatl's count after the undo = %s, want the use back", used)
	}
}
