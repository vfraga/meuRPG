package rules

import (
	"maps"
	"slices"
	"strings"
	"testing"
)

// effectSpell is a table spell of an invented kind that lasts a minute and leaves one effect: +2 on
// attack rolls and 5 temporary hit points.
func effectSpell(mutate func(*TableSpell)) TableSpell {
	ts := TableSpell{
		TableEntry: TableEntry{Key: "spell:escudo-de-brasa" + tableSuffix, NamePT: "Escudo de brasa"},
		Level:      2, School: "school:abjuration",
		CastingTime: TableCastingTime{Amount: 1, Unit: CastAction}, Range: TableRange{Kind: RangeRanged, DistanceFt: 30},
		Duration: TableDuration{Kind: DurationTimed, Amount: 1, Unit: DurationMinute}, Components: TableComponents{Verbal: true, Somatic: true},
		Concentration: true,
		Classes:       []string{"class:wizard"}, DescPT: []string{"Texto de teste."}, Target: SpellTarget{Kind: TargetCreature},
		Effect: &TableSpellEffect{Modifiers: []EffectModifier{
			{Kind: ModifierRollBonus, Value: 2, AppliesTo: []string{RollAppliesAttack}},
			{Kind: ModifierTempHP, Value: 5},
		}},
	}
	if mutate != nil {
		mutate(&ts)
	}
	return ts
}

func withEffectSpell(t *testing.T, mutate func(*TableSpell)) (*Content, error) {
	t.Helper()
	return loadForTest(t).With(Overlay{Revision: 1, Spells: []TableSpell{effectSpell(mutate)}})
}

func TestTableSpellEffectIsRegistered(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	c, err := withEffectSpell(t, nil)
	if err != nil {
		t.Fatalf("a spell with an effect is refused: %v", err)
	}
	key := effectSpell(nil).Key
	d, ok := c.CombatSpellEffect(key)
	if !ok {
		t.Fatal("the table spell has no combat effect")
	}
	if d.Key != key || d.Kind != "spell" || !d.Concentration || d.NamePT != "Escudo de brasa" || d.Applies != "all" || d.Visibility != EffectVisibilityOwner || d.Duration != nil {
		t.Errorf("effect = %+v", d)
	}
	if want := []string{"+2 em ataques", "PV temporários: 5"}; !slices.Equal(d.TagsPT, want) {
		t.Errorf("tags = %q, want %q", d.TagsPT, want)
	}
	if _, ok := srd.CombatSpellEffect(key); ok {
		t.Error("the SRD content answers for the table's spell")
	}
	if _, ok := srd.CombatSpellEffect("spell:bless"); !ok {
		t.Error("the SRD lost Bless")
	}
	if _, ok := c.CombatSpellEffect("spell:bless"); !ok {
		t.Error("the table content lost Bless")
	}
}

func TestTableSpellEffectKeepsWhatTheEntryGave(t *testing.T) {
	t.Parallel()
	c, err := withEffectSpell(t, func(ts *TableSpell) {
		ts.Effect = &TableSpellEffect{
			Applies: "failed_save", Conditions: []string{"condition:blinded"}, PlayerLabelPT: "Cegado", Visibility: "public",
			Modifiers: []EffectModifier{{Kind: ModifierResistance, ResistanceKind: ResistanceKindVulnerable, DamageTypes: []string{"damage-type:fire"}}},
		}
		ts.Save = &SpellSave{Ability: WIS, OnSuccess: "none"}
	})
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.CombatSpellEffect(effectSpell(nil).Key)
	if d.Applies != "failed_save" || d.Visibility != "public" || d.PlayerLabelPT != "Cegado" || !slices.Equal(d.Conditions, []string{"condition:blinded"}) {
		t.Errorf("effect = %+v", d)
	}
	if want := []string{"Vulnerabilidade a fogo"}; !slices.Equal(d.TagsPT, want) {
		t.Errorf("tags = %q", d.TagsPT)
	}
}

func TestArchivedOrOffTableSpellHasNoEffect(t *testing.T) {
	t.Parallel()
	key := effectSpell(nil).Key
	archived, err := withEffectSpell(t, func(ts *TableSpell) { ts.Archived = true })
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := archived.CombatSpellEffect(key); ok {
		t.Error("an archived spell answers")
	}
	off, err := loadForTest(t).With(Overlay{Revision: 1, Spells: []TableSpell{effectSpell(nil)}, Off: []string{key}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := off.CombatSpellEffect(key); ok {
		t.Error("a spell switched off answers")
	}
}

func TestTableSpellEffectRefusals(t *testing.T) {
	t.Parallel()
	mods := func(m ...EffectModifier) func(*TableSpell) {
		return func(ts *TableSpell) { ts.Effect.Modifiers = m }
	}
	many := func(n int) []EffectModifier {
		out := make([]EffectModifier, n)
		for i := range out {
			out[i] = EffectModifier{Kind: ModifierTempHP, Value: 1}
		}
		return out
	}
	conds := make([]string, 9)
	for i := range conds {
		conds[i] = "condition:blinded"
	}
	cases := []struct {
		name   string
		mutate func(*TableSpell)
		field  string
	}{
		{"unknown applies", func(ts *TableSpell) { ts.Effect.Applies = "some" }, ".effect.applies"},
		{"failed_save with no save", func(ts *TableSpell) { ts.Effect.Applies = "failed_save" }, ".effect.applies"},
		{"hit with no attack", func(ts *TableSpell) { ts.Effect.Applies = "hit" }, ".effect.applies"},
		{"a save on a self spell", func(ts *TableSpell) {
			ts.Target, ts.Range = SpellTarget{Kind: TargetSelf}, TableRange{Kind: RangeSelf}
			ts.Save = &SpellSave{Ability: WIS, OnSuccess: "none"}
			ts.Effect.Applies = "failed_save"
		}, ".effect.applies"},
		{"a condition that is not SRD", func(ts *TableSpell) { ts.Effect.Conditions = []string{"condition:bored"} }, ".effect.conditions[0]"},
		{"a condition twice", func(ts *TableSpell) {
			ts.Effect.Conditions = []string{"condition:blinded", "condition:blinded"}
		}, ".effect.conditions[1]"},
		{"nine conditions", func(ts *TableSpell) { ts.Effect.Conditions = conds }, ".effect.conditions"},
		{"nine modifiers", mods(many(9)...), ".effect.modifiers"},
		{"a bad modifier value", mods(EffectModifier{Kind: ModifierRollBonus, Value: 0, AppliesTo: []string{"attack"}}), ".effect.modifiers[0].value"},
		{"a bad modifier, second", mods(EffectModifier{Kind: ModifierTempHP, Value: 1}, EffectModifier{Kind: ModifierTempHP}), ".effect.modifiers[1].value"},
		{"an unknown modifier kind", mods(EffectModifier{Kind: "fly"}), ".effect.modifiers[0].kind"},
		{"an unknown damage type", mods(EffectModifier{Kind: ModifierResistance, DamageTypes: []string{"damage-type:boredom"}}), ".effect.modifiers[0].damage_types"},
		{"a base armor class", mods(EffectModifier{Kind: ModifierBaseAC, Value: 13}), ".effect.modifiers[0].kind"},
		{"an extra action", mods(EffectModifier{Kind: ModifierExtraAction, Allowed: []string{"attack"}, MaxWeaponAttacks: 1}), ".effect.modifiers[0].kind"},
		{"turn hit points", mods(EffectModifier{Kind: ModifierTurnTempHP, Value: 3}), ".effect.modifiers[0].kind"},
		{"condition immunity", mods(EffectModifier{Kind: ModifierConditionImmunity, Conditions: []string{"condition:frightened"}}), ".effect.modifiers[0].kind"},
		{"no move", mods(EffectModifier{Kind: ModifierNoMove}), ".effect.modifiers[0].kind"},
		{"no action", mods(EffectModifier{Kind: ModifierNoAction}), ".effect.modifiers[0].kind"},
		{"no reaction", mods(EffectModifier{Kind: ModifierNoReaction}), ".effect.modifiers[0].kind"},
		{"a choice of ability", mods(EffectModifier{Kind: ModifierCheckAdvantage, Choose: true}), ".effect.modifiers[0].choose"},
		{"a once disadvantage", mods(EffectModifier{Kind: ModifierAttackDisadvantage, Once: true}), ".effect.modifiers[0].once"},
		{"a disadvantage that spares the source", mods(EffectModifier{Kind: ModifierAttackDisadvantage, ExceptSource: true}), ".effect.modifiers[0].except_source"},
		{"nothing in it", func(ts *TableSpell) { ts.Effect = &TableSpellEffect{} }, ".effect"},
		{"a long label", func(ts *TableSpell) { ts.Effect.PlayerLabelPT = strings.Repeat("a", maxPlayerLabel+1) }, ".effect.player_label_pt"},
		{"an unknown visibility", func(ts *TableSpell) { ts.Effect.Visibility = "everyone" }, ".effect.visibility"},
		{"an instantaneous spell", func(ts *TableSpell) {
			ts.Concentration, ts.Duration = false, TableDuration{Kind: DurationInstantaneous}
		}, ".effect"},
		{"an aura with no radius", func(ts *TableSpell) { auraOf(ts, 0, AuraAffectsAllies) }, ".effect.aura.radius_ft"},
		{"an aura over the radius", func(ts *TableSpell) { auraOf(ts, 125, AuraAffectsAllies) }, ".effect.aura.radius_ft"},
		{"an aura off the squares", func(ts *TableSpell) { auraOf(ts, 12, AuraAffectsAllies) }, ".effect.aura.radius_ft"},
		{"an aura with no side", func(ts *TableSpell) { auraOf(ts, 10, "") }, ".effect.aura.affects"},
		{"an aura of an unknown side", func(ts *TableSpell) { auraOf(ts, 10, "everyone") }, ".effect.aura.affects"},
		{"an aura on an instantaneous spell", func(ts *TableSpell) {
			auraOf(ts, 10, AuraAffectsAllies)
			ts.Concentration, ts.Duration = false, TableDuration{Kind: DurationInstantaneous}
		}, ".effect.aura"},
		{"an aura of a spell that is not on the caster", func(ts *TableSpell) {
			auraOf(ts, 10, AuraAffectsAllies)
			ts.Target, ts.Range = SpellTarget{Kind: TargetCreature}, TableRange{Kind: RangeRanged, DistanceFt: 30}
		}, ".effect.aura"},
		{"an aura with a condition", func(ts *TableSpell) {
			auraOf(ts, 10, AuraAffectsAllies)
			ts.Effect.Conditions = []string{"condition:frightened"}
		}, ".effect.conditions"},
		{"an aura that gives armor class", func(ts *TableSpell) {
			auraOf(ts, 10, AuraAffectsAllies)
			ts.Effect.Modifiers = []EffectModifier{{Kind: ModifierACBonus, Value: 1}}
		}, ".effect.modifiers[0].kind"},
		{"an aura that gives temporary hit points", func(ts *TableSpell) { auraOf(ts, 10, AuraAffectsAllies) }, ".effect.modifiers[1].kind"},
		{"an aura as a modifier", mods(EffectModifier{Kind: ModifierAura, RadiusFt: 10, Affects: AuraAffectsAllies}), ".effect.modifiers[0].kind"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := withEffectSpell(t, tc.mutate)
			if err == nil {
				t.Fatal("accepted")
			}
			if !slices.ContainsFunc(refusedFields(err), func(f string) bool { return strings.HasSuffix(f, tc.field) }) {
				t.Errorf("fields %q, want one ending in %q (%v)", refusedFields(err), tc.field, err)
			}
		})
	}
}

// auraOf makes the spell a self spell with an aura of the radius and side that keeps the bonus and the temporary
// hit points of effectSpell.
func auraOf(ts *TableSpell, radius int, affects string) {
	ts.Target, ts.Range = SpellTarget{Kind: TargetSelf}, TableRange{Kind: RangeSelf}
	ts.Effect.Aura = &SpellAura{RadiusFt: radius, Affects: affects}
}

// An aura is the effect's first modifier, the one the combat reads to find the bearer's reach; the other
// modifiers are what it gives, and the tags say the radius in metres and the side (the positive control of the
// refusals above: the same spell with a good radius and side is accepted).
func TestTableSpellAuraIsRegistered(t *testing.T) {
	t.Parallel()
	c, err := withEffectSpell(t, func(ts *TableSpell) {
		auraOf(ts, 15, AuraAffectsEnemies)
		ts.Effect.Modifiers = ts.Effect.Modifiers[:1] // the bonus; the temporary hit points are refused in an aura
	})
	if err != nil {
		t.Fatalf("a spell with an aura is refused: %v", err)
	}
	d, ok := c.CombatSpellEffect(effectSpell(nil).Key)
	if !ok || len(d.Modifiers) != 2 {
		t.Fatalf("effect = %+v (found %v), want the aura and the bonus", d, ok)
	}
	if a := d.Modifiers[0]; a.Kind != ModifierAura || a.RadiusFt != 15 || a.Affects != AuraAffectsEnemies {
		t.Errorf("first modifier = %+v, want the aura of 15 ft on enemies", a)
	}
	if want := []string{"Aura de 4,5 m: inimigos", "+2 em ataques"}; !slices.Equal(d.TagsPT, want) {
		t.Errorf("tags = %q, want %q", d.TagsPT, want)
	}
	for _, side := range AuraAffects {
		if _, err := withEffectSpell(t, func(ts *TableSpell) {
			auraOf(ts, 120, side)
			ts.Effect.Modifiers = ts.Effect.Modifiers[:1]
		}); err != nil {
			t.Errorf("an aura of 120 ft on %q is refused: %v", side, err)
		}
	}
}

func TestTableSpellEffectOnAFailedSaveAndOnAHitIsAccepted(t *testing.T) {
	t.Parallel()
	for name, mutate := range map[string]func(*TableSpell){
		"failed_save": func(ts *TableSpell) {
			ts.Effect.Applies, ts.Save = "failed_save", &SpellSave{Ability: CON, OnSuccess: "none"}
		},
		"hit": func(ts *TableSpell) { ts.Effect.Applies, ts.Attack = "hit", "ranged" },
	} {
		if _, err := withEffectSpell(t, mutate); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// oneMod is a spell whose effect is the single modifier.
func oneMod(m EffectModifier) func(*TableSpell) {
	return func(ts *TableSpell) { ts.Effect.Modifiers = []EffectModifier{m} }
}

func TestTableSpellEffectOldKindsThroughWith(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		ok    EffectModifier
		bad   EffectModifier
		field string
	}{
		{
			"attack_disadvantage",
			EffectModifier{Kind: ModifierAttackDisadvantage},
			EffectModifier{Kind: ModifierAttackDisadvantage, Once: true},
			".effect.modifiers[0].once",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := withEffectSpell(t, oneMod(tc.ok)); err != nil {
				t.Fatalf("refused: %v", err)
			}
			_, err := withEffectSpell(t, oneMod(tc.bad))
			if err == nil {
				t.Fatal("accepted")
			}
			if !namesField(err, tc.field) {
				t.Errorf("fields %q, want one ending in %q", refusedFields(err), tc.field)
			}
		})
	}
}

func TestTableSpellEffectDoesNotChangeTheSRD(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	before := slices.Sorted(maps.Keys(srd.c.combatEffects.spells))
	if _, err := withEffectSpell(t, nil); err != nil {
		t.Fatal(err)
	}
	// With starts from a fresh load, so compare on the one the table was built from.
	c, err := srd.With(Overlay{Revision: 1, Spells: []TableSpell{effectSpell(nil)}})
	if err != nil {
		t.Fatal(err)
	}
	after := slices.Sorted(maps.Keys(srd.c.combatEffects.spells))
	if !slices.Equal(before, after) {
		t.Errorf("the SRD spells with effects changed: %q -> %q", before, after)
	}
	if got := slices.Sorted(maps.Keys(c.c.combatEffects.spells)); !slices.Equal(before, got) {
		t.Errorf("the table content's shared SRD map changed: %q", got)
	}
}

func TestTableSpellEffectIsACopyOfTheEntry(t *testing.T) {
	t.Parallel()
	ts := effectSpell(nil)
	c, err := loadForTest(t).With(Overlay{Revision: 1, Spells: []TableSpell{ts}})
	if err != nil {
		t.Fatal(err)
	}
	ts.Effect.Modifiers[0].AppliesTo[0] = RollAppliesCheck
	ts.Effect.Modifiers[0].Value = 19
	d, _ := c.CombatSpellEffect(ts.Key)
	if d.Modifiers[0].AppliesTo[0] != RollAppliesAttack || d.Modifiers[0].Value != 2 {
		t.Errorf("the registered effect followed the caller's entry: %+v", d.Modifiers[0])
	}
}
