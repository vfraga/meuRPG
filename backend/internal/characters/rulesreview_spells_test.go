package characters

import (
	"fmt"
	"testing"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
)

// rulesReviewCast casts key at the slot as a level-N caster of class and returns what the app exposes.
func rulesReviewCast(t *testing.T, class, sub string, level int32, key string, slot int) link.Spell {
	t.Helper()
	h := newHarness(t)
	master := h.newUser("Mestre")
	p := h.newUser("Conjurador")
	campaign := h.newCampaign(master, "Mirathel", p)
	ch := p.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Conjurador",
		casterSheet(class, sub, level, 10, 12, 14, 18, 18, 12, nil, nil))
	tx, err := h.pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	sp, err := h.svc.CombatSpell(t.Context(), tx, campaign, ch.GetId(), key, slot)
	if err != nil {
		t.Fatalf("CombatSpell(%s, %d) error = %v", key, slot, err)
	}
	return sp
}

func rulesReviewDice(d *link.Dice) string {
	if d == nil {
		return "no damage"
	}
	return fmt.Sprintf("%dd%d %s", d.Count, d.Sides, d.DamageType)
}

// SRD: Ice Storm deals 2d8 bludgeoning AND 4d6 cold at 4th level.
func TestRulesReviewSpells_IceStormTwoTypes(t *testing.T) {
	sp := rulesReviewCast(t, "class:wizard", "subclass:evocation", 17, "spell:ice-storm", 4)
	if sp.Damage == nil || sp.Damage.Count != 2 || sp.Damage.Sides != 8 {
		t.Errorf("Ice Storm slot 4: SRD total 2d8 bludgeoning + 4d6 cold (two damage types), app gives only %s (link.Spell has a single Damage)", rulesReviewDice(sp.Damage))
		return
	}
	t.Errorf("Ice Storm slot 4: SRD total 2d8 bludgeoning + 4d6 cold, app gives only %s: the second damage type (cold) is dropped", rulesReviewDice(sp.Damage))
}

// SRD: Meteor Swarm deals 20d6 fire AND 20d6 bludgeoning.
func TestRulesReviewSpells_MeteorSwarmTwoTypes(t *testing.T) {
	sp := rulesReviewCast(t, "class:wizard", "subclass:evocation", 17, "spell:meteor-swarm", 9)
	if sp.Damage == nil || sp.Damage.Count != 40 {
		t.Errorf("Meteor Swarm slot 9: SRD total 20d6 fire + 20d6 bludgeoning (40d6), app gives only %s: the second damage type is dropped", rulesReviewDice(sp.Damage))
	}
}

// SRD: Flame Strike deals 4d6 fire AND 4d6 radiant.
func TestRulesReviewSpells_FlameStrikeTwoTypes(t *testing.T) {
	sp := rulesReviewCast(t, "class:cleric", "subclass:life", 9, "spell:flame-strike", 5)
	if sp.Damage == nil || sp.Damage.Count != 8 {
		t.Errorf("Flame Strike slot 5: SRD total 4d6 fire + 4d6 radiant (8d6), app gives only %s: the second damage type is dropped", rulesReviewDice(sp.Damage))
	}
}

// SRD: at a 6th-level slot Flame Strike still deals 4d6 fire + 4d6 radiant (+1d6 of one of them).
func TestRulesReviewSpells_FlameStrikeSlot6NoDamage(t *testing.T) {
	sp := rulesReviewCast(t, "class:cleric", "subclass:life", 11, "spell:flame-strike", 6)
	if sp.Damage == nil {
		t.Errorf("Flame Strike slot 6: SRD total 5d6 fire + 4d6 radiant (at least 8d6), app gives no damage at all (data string \"4d6 OR 5d6\" does not parse)")
	}
}

// SRD: False Life gives TEMPORARY hit points, not healing.
func TestRulesReviewSpells_FalseLifeIsNotAHeal(t *testing.T) {
	sp := rulesReviewCast(t, "class:wizard", "subclass:evocation", 5, "spell:false-life", 1)
	if sp.Heal != nil {
		t.Errorf("False Life slot 1: SRD gives 1d4+4 temporary hit points (no healing, not capped by max HP), app returns Heal %dd%d%+d, an ordinary heal capped at max HP; link.Spell has no temporary-HP field", sp.Heal.Count, sp.Heal.Sides, sp.Heal.Bonus)
	}
}

// SRD: Aid raises max AND current hit points of up to three creatures by 5; it does not heal.
func TestRulesReviewSpells_AidIsNotAHeal(t *testing.T) {
	sp := rulesReviewCast(t, "class:cleric", "subclass:life", 5, "spell:aid", 2)
	if sp.Heal != nil {
		t.Errorf("Aid slot 2: SRD raises max and current HP by 5 (no healing), app returns Heal %dd%d%+d, an ordinary heal capped at max HP; link.Spell has no max-HP field", sp.Heal.Count, sp.Heal.Sides, sp.Heal.Bonus)
	}
}

func TestRulesReviewSpells_AidThreeTargets(t *testing.T) {
	det, ok := loadRules(t).SpellDetails("spell:aid")
	if !ok {
		t.Fatal("no spell:aid")
	}
	if got := det.Target.MaxTargets(2, 2); got != 3 {
		t.Errorf("Aid MaxTargets(2,2): SRD 3 creatures, app gives %d", got)
	}
}
