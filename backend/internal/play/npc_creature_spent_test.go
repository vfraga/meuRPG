package play

import (
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// A persistent NPC with a stat block (a character of kind NPC that is not combat_only) keeps what it spent of its
// uses a day and its Legendary Resistance until the table's long rest, in npc_spell_use.creature_spent; a monster of
// a combat starts every fight with all of them, and the Recharge state and the legendary actions end with the combat.
// These tests need the database.

const (
	devaUse         = spentUsePrefix + healingTouch
	devaAllowed     = 3
	resistanceTotal = 3
)

// storedSpent reads the NPC's own count of creature_spent and says whether the character has a row.
func (a *armed) storedSpent(t *testing.T, characterID string) (map[string]int, bool) {
	t.Helper()
	var raw []byte
	err := a.h.pool.QueryRow(t.Context(), `SELECT creature_spent FROM npc_spell_use WHERE character_id = $1`, characterID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false
	}
	if err != nil {
		t.Fatalf("npc_spell_use.creature_spent of %s: %v", characterID, err)
	}
	return creatureSpentOf(raw), true
}

// keepAsPersistent makes the character of the combatant a persistent NPC: not combat_only, with its stat block.
func (a *armed) keepAsPersistent(t *testing.T, label string) string {
	t.Helper()
	char := a.characterOf(t, label)
	if _, err := a.h.pool.Exec(t.Context(), `UPDATE characters SET sheet = jsonb_set(sheet, '{basic,combat_only}', 'false') WHERE id = $1`, char); err != nil {
		t.Fatalf("make %s a persistent NPC: %v", label, err)
	}
	return char
}

// persistentDeva is a combat where the deva (3 a day) is a persistent NPC, on its turn.
func persistentDeva(t *testing.T) (a *armed, e *playv1.Encounter, label, char string) {
	t.Helper()
	a, e, label = creatureFight(t, true, "monster:deva", false)
	char = a.keepAsPersistent(t, label)
	return a, e, label, char
}

// nextCombatOf starts a combat of the party and the character (in setup, on no map) and returns the label it has there.
func (a *armed) nextCombatOf(t *testing.T, char string, copies int32) {
	t.Helper()
	a.start(t, plan{
		npcs: []*playv1.Participant{{CharacterId: char, Count: copies}}, npcRolls: []int{1, 2},
		players: map[string]int32{"Toren": 10, "Pensantus": 5, "Brisa": 1}, setup: true, theatre: true,
	})
}

// labelOfCharacter is the label of the first combatant of the character in the combat now.
func (a *armed) labelOfCharacter(t *testing.T, char string) string {
	t.Helper()
	for _, c := range a.get(t, a.master).GetCombatants() {
		if c.GetCharacterId() == char {
			return c.GetLabel()
		}
	}
	t.Fatalf("no combatant of %s", char)
	return ""
}

// The deva spends its Healing Touch twice; the next combat of the same NPC starts with both spent and the master
// reads one use left.
func TestAPersistentNPCKeepsItsSpentUsesADayForTheNextCombat(t *testing.T) {
	t.Parallel()
	a, e, deva, char := persistentDeva(t)
	a.mustUseAction(t, a.master, e, deva, healingTouch, nil)
	e = a.ownTurnAgain(t, e, deva)
	a.mustUseAction(t, a.master, e, deva, healingTouch, nil)
	if got, _ := a.storedSpent(t, char); got[devaUse] != 2 || len(got) != 1 {
		t.Fatalf("the NPC's count after two uses = %v, want %s 2", got, devaUse)
	}
	a.endEncounter(t, e)

	a.nextCombatOf(t, char, 1)
	next := a.labelOfCharacter(t, char)
	if got := a.spentOf(t, a.id(t, next))[devaUse]; got != 2 {
		t.Errorf("the deva starts the next combat with %d spent, want 2", got)
	}
	if l := a.limitedOf(t, a.master, next, healingTouch); l == nil || !l.GetAvailable() || l.GetUsesLeft() != devaAllowed-2 {
		t.Errorf("the master reads %v, want one use left", l)
	}
}

// The undo of a use gives it back on the combatant and on the NPC.
func TestTheUndoOfAPersistentNPCsUseLowersItsCount(t *testing.T) {
	t.Parallel()
	a, e, deva, char := persistentDeva(t)
	a.mustUseAction(t, a.master, e, deva, healingTouch, nil)
	a.undoLast(t, e)
	if got, has := a.storedSpent(t, char); got[devaUse] != 0 || has && len(got) != 0 {
		t.Errorf("the NPC's count after the undo of the only use = %v, want nothing", got)
	}
	a.mustUseAction(t, a.master, e, deva, healingTouch, nil)
	e = a.ownTurnAgain(t, e, deva)
	a.mustUseAction(t, a.master, e, deva, healingTouch, nil)
	a.undoLast(t, e)
	if got, _ := a.storedSpent(t, char); got[devaUse] != 1 {
		t.Errorf("the NPC's count after the undo of the second use = %v, want 1", got)
	}
}

// The dragon spends a use of Legendary Resistance; the next combat starts with it spent and the master reads two left.
func TestAPersistentNPCKeepsItsLegendaryResistanceForTheNextCombat(t *testing.T) {
	t.Parallel()
	a, e, w := heldDragon(t)
	char := a.keepAsPersistent(t, "Draco")
	if _, err := a.rollEffectSave(t, a.master, e, w.GetId(), resisting(effectSaveFace(1))); err != nil {
		t.Fatalf("RollEffectSave() error = %v", err)
	}
	if got, _ := a.storedSpent(t, char); got[spentLegendaryResistance] != 1 || len(got) != 1 {
		t.Fatalf("the NPC's count after the resistance = %v, want %s 1", got, spentLegendaryResistance)
	}
	a.endEncounter(t, a.get(t, a.master))

	a.nextCombatOf(t, char, 1)
	next := a.labelOfCharacter(t, char)
	if got := a.spentOf(t, a.id(t, next))[spentLegendaryResistance]; got != 1 {
		t.Errorf("the dragon starts the next combat with %d spent, want 1", got)
	}
	if l := byLabel(t, a.get(t, a.master), next).GetLegendary(); l.GetResistanceLeft() != resistanceTotal-1 {
		t.Errorf("the master reads %v, want %d uses left", l, resistanceTotal-1)
	}
}

// Only the uses a day and Legendary Resistance stay with the NPC: a spent Recharge action and the legendary actions
// of a round end with the combat, so the next one starts with every Recharge action available.
func TestAPersistentNPCDoesNotKeepRechargeOrLegendaryActions(t *testing.T) {
	t.Parallel()
	a, _, _ := dragonFight(t)
	char := a.keepAsPersistent(t, "Draco")
	id, q := a.id(t, "Draco"), playdb.New(a.h.pool)
	const recharge, use = spentRechargePrefix + "monster:adult-red-dragon#fire-breath", spentUsePrefix + "monster:adult-red-dragon#x"
	for key, n := range map[string]int{recharge: 1, spentLegendaryActions: 2, use: 1, spentLegendaryResistance: 1} {
		if _, err := bumpCreatureSpent(t.Context(), q, id, key, n); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := a.storedSpent(t, char)
	if len(got) != 2 || got[use] != 1 || got[spentLegendaryResistance] != 1 {
		t.Fatalf("the NPC's count = %v, want only %s and %s", got, use, spentLegendaryResistance)
	}
	a.endEncounter(t, a.get(t, a.master))
	a.nextCombatOf(t, char, 1)
	next := a.spentOf(t, a.id(t, a.labelOfCharacter(t, char)))
	if len(next) != 2 || next[use] != 1 || next[spentLegendaryResistance] != 1 {
		t.Errorf("the dragon starts the next combat with %v, want the use and the resistance only", next)
	}
}

// A monster (the combat_only NPC of a creature) starts every combat with all its uses, and nothing is kept for it.
func TestAMonstersUsesStartEveryCombatFresh(t *testing.T) {
	t.Parallel()
	a, e, deva := creatureFight(t, true, "monster:deva", false)
	char := a.characterOf(t, deva)
	a.mustUseAction(t, a.master, e, deva, healingTouch, nil)
	if got := a.spentOf(t, a.id(t, deva))[devaUse]; got != 1 {
		t.Fatalf("the monster spent %d, want 1", got)
	}
	if _, has := a.storedSpent(t, char); has {
		t.Error("the monster has a row of kept uses")
	}
	a.endEncounter(t, e)
	next := a.monsterSetup(t, true)
	res := a.mustAddMonsters(t, next, func(r *playv1.AddMonstersRequest) { r.CreatureKey, r.Count = "monster:deva", 1 })
	if got := a.spentOf(t, res.GetCombatantIds()[0]); len(got) != 0 {
		t.Errorf("the monster of the next combat has spent %v, want nothing", got)
	}
}

// Two copies of a persistent NPC share one count that is the highest of the two, never the sum, and each copy starts
// the next combat with it; the change of the higher copy lowers it to the other's.
func TestCopiesOfAPersistentNPCKeepTheHighestUseCountNotTheSum(t *testing.T) {
	t.Parallel()
	a, e, _, char := persistentDeva(t)
	a.endEncounter(t, e)
	a.nextCombatOf(t, char, 2)
	one, two := a.id(t, a.labelOfCharacter(t, char)), ""
	for _, c := range a.get(t, a.master).GetCombatants() {
		if c.GetCharacterId() == char && c.GetId() != one {
			two = c.GetId()
		}
	}
	q := playdb.New(a.h.pool)
	for _, step := range []struct {
		id    string
		delta int
		want  int
	}{{one, 1, 1}, {two, 1, 1}, {one, 1, 2}, {one, -1, 1}} {
		if _, err := bumpCreatureSpent(t.Context(), q, step.id, devaUse, step.delta); err != nil {
			t.Fatal(err)
		}
		if got, _ := a.storedSpent(t, char); got[devaUse] != step.want {
			t.Fatalf("the NPC's count after %+v = %v, want %d", step, got, step.want)
		}
	}
	a.endEncounter(t, a.get(t, a.master))
	a.nextCombatOf(t, char, 2)
	for _, c := range a.get(t, a.master).GetCombatants() {
		if c.GetCharacterId() == char && a.spentOf(t, c.GetId())[devaUse] != 1 {
			t.Errorf("%s starts the next combat with %v, want 1", c.GetLabel(), a.spentOf(t, c.GetId()))
		}
	}
}

// A short rest keeps the count of the uses a day; the table's long rest gives the uses and the Legendary Resistance back, on the NPC
// and on a combat in setup that copied them, and leaves the other keys of that combatant.
func TestTheLongRestGivesAPersistentNPCsUsesBack(t *testing.T) {
	t.Parallel()
	a, e, deva, char := persistentDeva(t)
	a.mustUseAction(t, a.master, e, deva, healingTouch, nil)
	if _, err := bumpCreatureSpent(t.Context(), playdb.New(a.h.pool), a.id(t, deva), spentLegendaryResistance, 1); err != nil {
		t.Fatal(err)
	}
	a.endEncounter(t, e)
	a.nextCombatOf(t, char, 1)
	next := a.id(t, a.labelOfCharacter(t, char))
	if _, err := bumpCreatureSpent(t.Context(), playdb.New(a.h.pool), next, spentLegendaryActions, 1); err != nil {
		t.Fatal(err)
	}
	a.rest(t, playv1.RestKind_REST_KIND_SHORT)
	if got, _ := a.storedSpent(t, char); got[devaUse] != 1 || got[spentLegendaryResistance] != 1 {
		t.Errorf("the NPC's count after a short rest = %v, want it kept", got)
	}
	if got := a.spentOf(t, next); got[devaUse] != 1 || got[spentLegendaryResistance] != 1 {
		t.Errorf("the combat in setup after a short rest = %v, want it kept", got)
	}
	a.rest(t, playv1.RestKind_REST_KIND_LONG)
	if _, has := a.storedSpent(t, char); has {
		t.Error("the NPC still has a row after the long rest")
	}
	if got := a.spentOf(t, next); len(got) != 1 || got[spentLegendaryActions] != 1 {
		t.Errorf("the combat in setup after the long rest = %v, want only the legendary actions it had", got)
	}
}

// The count is kept for the character: deleting it takes the row.
func TestDeletingAPersistentNPCRemovesItsKeptUses(t *testing.T) {
	t.Parallel()
	a, e, deva, char := persistentDeva(t)
	a.mustUseAction(t, a.master, e, deva, healingTouch, nil)
	if got, has := a.storedSpent(t, char); !has || got[devaUse] != 1 { // positive control
		t.Fatalf("the NPC's count before the delete = %v", got)
	}
	a.endEncounter(t, e)
	if _, err := a.h.pool.Exec(t.Context(), `DELETE FROM characters WHERE id = $1`, char); err != nil {
		t.Fatalf("delete the NPC: %v", err)
	}
	if _, has := a.storedSpent(t, char); has {
		t.Error("the NPC's row outlived the character")
	}
}

// A player's combatant keeps nothing for an NPC.
func TestAPlayersCreatureSpentLeavesNoNPCRow(t *testing.T) {
	t.Parallel()
	a, _, _, _ := persistentDeva(t)
	if _, err := bumpCreatureSpent(t.Context(), playdb.New(a.h.pool), a.id(t, "Toren"), devaUse, 1); err != nil {
		t.Fatal(err)
	}
	if _, has := a.storedSpent(t, a.characterOf(t, "Toren")); has {
		t.Error("the player has a row of kept uses")
	}
}

// The column holds 4096 bytes of text and no more, and only an object.
func TestTheKeptUsesColumnHoldsAtMost4KiB(t *testing.T) {
	t.Parallel()
	a, _, _, char := persistentDeva(t)
	ctx := t.Context()
	write := func(body string) error {
		_, err := a.h.pool.Exec(ctx, `INSERT INTO npc_spell_use (character_id, creature_spent) VALUES ($1, $2::JSONB)
			ON CONFLICT (character_id) DO UPDATE SET creature_spent = excluded.creature_spent`, char, body)
		return err
	}
	if err := write(`{"x": "` + strings.Repeat("y", creatureSpentLimit-9) + `"}`); err != nil {
		t.Errorf("4096 bytes: %v", err)
	}
	if err := write(`{"x": "` + strings.Repeat("y", creatureSpentLimit-8) + `"}`); err == nil {
		t.Error("the database accepted 4097 bytes")
	}
	if err := write(`[]`); err == nil {
		t.Error("the database accepted a value that is not an object")
	}
}

const inkSpent = spentRestPrefix + inkCloud

// persistentOctopus is a combat where the octopus (an action that recharges after a short or long rest) is a persistent NPC.
func persistentOctopus(t *testing.T) (a *armed, e *playv1.Encounter, label, char string) {
	t.Helper()
	a, e, label = creatureFight(t, true, "monster:octopus", false)
	char = a.keepAsPersistent(t, label)
	return a, e, label, char
}

// The octopus spends its Ink Cloud and is refused from then on, in this combat and in the next; a use a day of the
// same NPC stays spent through the short rest, which gives the Ink Cloud back, and the long rest gives both back.
func TestAPersistentNPCsRestActionComesBackWithAShortRest(t *testing.T) {
	t.Parallel()
	const perDay = spentUsePrefix + "monster:octopus#other"
	a, e, octopus, char := persistentOctopus(t)
	a.mustUseAction(t, a.master, e, octopus, inkCloud, nil)
	if _, err := bumpCreatureSpent(t.Context(), playdb.New(a.h.pool), a.id(t, octopus), perDay, 1); err != nil {
		t.Fatal(err)
	}
	e = a.ownTurnAgain(t, e, octopus)
	_, err := a.useAction(t, a.master, e, octopus, inkCloud, nil)
	wantBlockedBy(t, "UseCreatureAction", err, noUsesLeft)
	a.endEncounter(t, e)

	a.nextCombatOf(t, char, 1)
	next := a.labelOfCharacter(t, char)
	if l := a.limitedOf(t, a.master, next, inkCloud); l == nil || l.GetAvailable() {
		t.Fatalf("the master reads %v in the next combat, want the Ink Cloud spent", l)
	}
	a.rest(t, playv1.RestKind_REST_KIND_SHORT)
	if got, has := a.storedSpent(t, char); !has || got[perDay] != 1 || len(got) != 1 {
		t.Errorf("the NPC's count after the short rest = %v, want only %s", got, perDay)
	}
	if got := a.spentOf(t, a.id(t, next)); got[perDay] != 1 || len(got) != 1 {
		t.Errorf("the combat in setup after the short rest = %v, want only %s", got, perDay)
	}
	if l := a.limitedOf(t, a.master, next, inkCloud); l == nil || !l.GetAvailable() {
		t.Errorf("the master reads %v after the short rest, want the Ink Cloud available", l)
	}
	a.rest(t, playv1.RestKind_REST_KIND_LONG)
	if _, has := a.storedSpent(t, char); has {
		t.Error("the NPC still has a row after the long rest")
	}
	if got := a.spentOf(t, a.id(t, next)); len(got) != 0 {
		t.Errorf("the combat in setup after the long rest = %v, want nothing", got)
	}
}

// A short rest that leaves the NPC nothing spent takes its row; the long rest gives both kinds back.
func TestTheShortRestTakesTheRowOfAnNPCWithOnlyARestAction(t *testing.T) {
	t.Parallel()
	a, e, octopus, char := persistentOctopus(t)
	a.mustUseAction(t, a.master, e, octopus, inkCloud, nil)
	if got, has := a.storedSpent(t, char); !has || got[inkSpent] != 1 { // positive control
		t.Fatalf("the NPC's count before the rest = %v", got)
	}
	a.endEncounter(t, e)
	a.rest(t, playv1.RestKind_REST_KIND_SHORT)
	if got, has := a.storedSpent(t, char); has {
		t.Errorf("the NPC's count after the short rest = %v, want no row", got)
	}
}

// The undo of the use of a rest action gives it back on the combatant and on the NPC.
func TestTheUndoOfAPersistentNPCsRestActionGivesItBack(t *testing.T) {
	t.Parallel()
	a, e, octopus, char := persistentOctopus(t)
	a.mustUseAction(t, a.master, e, octopus, inkCloud, nil)
	if got, _ := a.storedSpent(t, char); got[inkSpent] != 1 {
		t.Fatalf("the NPC's count after the use = %v", got)
	}
	a.undoLast(t, e)
	if got, _ := a.storedSpent(t, char); got[inkSpent] != 0 {
		t.Errorf("the NPC's count after the undo = %v, want nothing", got)
	}
	if l := a.limitedOf(t, a.master, octopus, inkCloud); l == nil || !l.GetAvailable() || a.spentOf(t, a.id(t, octopus))[inkSpent] != 0 {
		t.Errorf("the master reads %v after the undo, want the Ink Cloud available", l)
	}
}

// A combat in setup holds the keys its NPC copied: the short rest strips the rest: keys only, the long rest the
// use:, rest: and legendary_resistance keys, and the others (a Recharge, the legendary actions) stay.
func TestTheRestsClearTheKeysOfANPCInACombatInSetup(t *testing.T) {
	t.Parallel()
	const use, recharge = spentUsePrefix + "monster:octopus#x", spentRechargePrefix + "monster:octopus#y"
	a, e, _, char := persistentOctopus(t)
	a.endEncounter(t, e)
	a.nextCombatOf(t, char, 1)
	id := a.id(t, a.labelOfCharacter(t, char))
	q := playdb.New(a.h.pool)
	for _, key := range []string{use, inkSpent, spentLegendaryResistance, recharge, spentLegendaryActions} {
		if _, err := bumpCreatureSpent(t.Context(), q, id, key, 1); err != nil {
			t.Fatal(err)
		}
	}
	a.rest(t, playv1.RestKind_REST_KIND_SHORT)
	got := a.spentOf(t, id)
	if len(got) != 4 || got[inkSpent] != 0 || got[use] != 1 || got[spentLegendaryResistance] != 1 || got[recharge] != 1 || got[spentLegendaryActions] != 1 {
		t.Errorf("the combat in setup after the short rest = %v, want everything but %s", got, inkSpent)
	}
	if _, err := bumpCreatureSpent(t.Context(), q, id, inkSpent, 1); err != nil {
		t.Fatal(err)
	}
	a.rest(t, playv1.RestKind_REST_KIND_LONG)
	got = a.spentOf(t, id)
	if len(got) != 2 || got[recharge] != 1 || got[spentLegendaryActions] != 1 {
		t.Errorf("the combat in setup after the long rest = %v, want only %s and %s", got, recharge, spentLegendaryActions)
	}
}
