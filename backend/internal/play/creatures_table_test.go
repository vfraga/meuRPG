package play

import (
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// tableSkeletonKey is the master's creature of issue #578: an SRD skeleton with 30 hit points, armor class
// 15 and +3 on the damage of each attack. These tests need the database (MEURPG_TEST_DATABASE_URL).
const tableSkeletonKey = "creature:guarda-ossuda@mesa"

// TestMR578_ATableCreatureOfACharacterFightsWithItsAdjustments: the master gives Toren the table creature, and
// the combat takes it in with the hit points and armor class of the entry; its attack adds the entry's damage
// bonus to the skeleton's 1d6 + 2.
func TestMR578_ATableCreatureOfACharacterFightsWithItsAdjustments(t *testing.T) {
	t.Parallel()
	a := newSummoners(t)
	if _, err := a.master.table.CreateTableEntry(t.Context(), connect.NewRequest(&rulesv1.CreateTableEntryRequest{
		CampaignId: a.campaignID, Body: &rulesv1.CreateTableEntryRequest_TableCreature{TableCreature: &rulesv1.TableCreature{
			NamePt: "Guarda Ossuda", BaseKey: "monster:skeleton", HitPoints: 30, ArmorClass: 15, DamageBonus: 3,
		}},
	})); err != nil {
		t.Fatalf("CreateTableEntry(creature) error = %v", err)
	}
	given := a.give(t, a.toren, tableSkeletonKey, "Guarda")
	if given.GetHitPointsMax() != 30 || given.GetArmorClass() != 15 {
		t.Fatalf("the gift = hp %d, armor class %d; want the entry's 30 and 15", given.GetHitPointsMax(), given.GetArmorClass())
	}

	e := a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: a.goblin.GetId()}},
		npcRolls: []int{2},
		players:  map[string]int32{"Pensantus": 20, "Sálvia": 15, "Toren": 12, "Guarda": 10},
		reveal:   []string{"Goblin"},
		at:       map[string][2]int32{"Toren": {3, 3}, "Goblin": {4, 3}, "Pensantus": {10, 3}, "Sálvia": {6, 5}, "Guarda": {5, 3}},
	})
	guard := byLabel(t, e, "Guarda")
	if guard.GetHitPointsCurrent() != 30 || guard.GetHitPointsMax() != 30 || guard.GetArmorClass() != 15 {
		t.Errorf("the combatant = hp %d/%d, armor class %d; want 30/30 and 15", guard.GetHitPointsCurrent(), guard.GetHitPointsMax(), guard.GetArmorClass())
	}

	e = a.passTo(t, e, "Guarda")
	hit, err := a.attack(t, a.caio, e, "Guarda", tableSkeletonKey+"#shortsword", "Goblin", d20(15))
	if err != nil {
		t.Fatalf("RollAttack by the table creature error = %v", err)
	}
	if hit.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_HIT || hit.GetPendingDamage() == nil {
		t.Fatalf("attack = %v, want a hit with damage pending", hit)
	}
	a.mustDamage(t, a.caio, e, hit.GetPendingDamage().GetId(), typedDamage(1)) // 1d6 + 2 + 3: 1 + 5 = 6 damage
	if cur, _, _ := a.hp(t, "Goblin"); cur != 1 {
		t.Errorf("the goblin has %d hit points, want 7 - 6 = 1 (the SRD skeleton would leave 4)", cur)
	}
}

// TestMR578_ATableCreatureAddedAfterItsEditFightsWithTheNewNumbers: the master puts a table creature (armor
// class 20) in a combat, edits the entry (armor class 15, damage bonus +5) and puts it in again. The new combatant
// has the entry's current numbers; the first keeps the ones it joined with.
func TestMR578_ATableCreatureAddedAfterItsEditFightsWithTheNewNumbers(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	created, err := a.master.table.CreateTableEntry(t.Context(), connect.NewRequest(&rulesv1.CreateTableEntryRequest{
		CampaignId: a.campaignID, Body: &rulesv1.CreateTableEntryRequest_TableCreature{TableCreature: &rulesv1.TableCreature{
			NamePt: "Guarda Ossuda", BaseKey: "monster:skeleton", HitPoints: 30, ArmorClass: 20,
		}},
	}))
	if err != nil {
		t.Fatalf("CreateTableEntry(creature) error = %v", err)
	}
	entry := created.Msg.GetEntry()
	e := a.monsterSetup(t, true)
	first := a.mustAddMonsters(t, e, func(r *playv1.AddMonstersRequest) { r.CreatureKey, r.Count = tableSkeletonKey, 1 })

	if _, err := a.master.table.UpdateTableEntry(t.Context(), connect.NewRequest(&rulesv1.UpdateTableEntryRequest{
		CampaignId: a.campaignID, Key: entry.GetKey(), ExpectedRevision: entry.GetRevision(),
		Body: &rulesv1.UpdateTableEntryRequest_TableCreature{TableCreature: &rulesv1.TableCreature{
			NamePt: "Guarda Ossuda", BaseKey: "monster:skeleton", HitPoints: 30, ArmorClass: 15, DamageBonus: 5,
		}},
	})); err != nil {
		t.Fatalf("UpdateTableEntry(creature) error = %v", err)
	}
	second := a.mustAddMonsters(t, first.GetEncounter(), func(r *playv1.AddMonstersRequest) { r.CreatureKey, r.Count = tableSkeletonKey, 1 })

	// numbers reads a combatant's armor class and the damage bonus of its attack, as the master does.
	numbers := func(id string) (armorClass, damageBonus int32) {
		t.Helper()
		enc := a.get(t, a.master)
		for _, c := range enc.GetCombatants() {
			if c.GetId() != id {
				continue
			}
			opt := attackOption(a.mustOptions(t, a.master, enc, c.GetLabel()), sword)
			if opt == nil {
				t.Fatalf("the combatant %s has no attack %s", c.GetLabel(), sword)
			}
			return c.GetArmorClass(), opt.GetAttack().GetDamageDice().GetBonus()
		}
		t.Fatalf("combatant %s is not in the combat", id)
		return 0, 0
	}
	// The skeleton's shortsword is 1d6 + 2.
	if ac, bonus := numbers(second.GetCombatantIds()[0]); ac != 15 || bonus != 7 {
		t.Errorf("the combatant added after the edit = armor class %d, damage bonus %d; want the entry's 15 and 2 + 5", ac, bonus)
	}
	if ac, bonus := numbers(first.GetCombatantIds()[0]); ac != 20 || bonus != 2 {
		t.Errorf("the combatant added before the edit = armor class %d, damage bonus %d; want the 20 and 2 it joined with", ac, bonus)
	}
}
