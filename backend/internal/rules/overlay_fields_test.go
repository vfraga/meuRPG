package rules

import (
	"errors"
	"strconv"
	"testing"
)

// TestClassRefusalsNameTheirField: every rule a class, a subclass or an effect
// can break comes back with the exact field the editor draws (the proto's field
// names, with their own indexes) and a stable reason (field-level
// refusals). The paths here are the Overlay's; the server writes them under the
// body's name ("table_class.levels[4].slots[2]"). The web copies these paths and
// reasons into content-violations.spec.ts (CLASS_ROWS): change a row there and here together.
func TestClassRefusalsNameTheirField(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	// Classes: 0 none, 1 full-prepared, 2 full-known, 3 half-prepared, 4 pact-known.
	// Subclasses: 0 gen-none-a, 2 gen-full-prepared-b (always-prepared), 9 and 10 the third casters.
	effect := func(e Effect) func(*Overlay) {
		return func(o *Overlay) { o.Classes[1].Levels[0].Features[0].Effects = []Effect{e} }
	}
	const eff = "classes[1].levels[0].features[0].effects[0]"
	cases := []struct {
		name          string
		edit          func(o *Overlay)
		field, reason string
	}{
		// The class's basics.
		{"two equal saving throws", func(o *Overlay) { o.Classes[1].SavingThrows = []Ability{CON, CON} }, "classes[1].saving_throws[1]", ReasonValue},
		{"one saving throw", func(o *Overlay) { o.Classes[1].SavingThrows = []Ability{CON} }, "classes[1].saving_throws", ReasonValue},
		{"a saving throw that is no ability", func(o *Overlay) { o.Classes[1].SavingThrows = []Ability{CON, "luck"} }, "classes[1].saving_throws[1]", ReasonValue},
		{"more skills than the list", func(o *Overlay) { o.Classes[1].SkillChoose = 9 }, "classes[1].skill_choose", ReasonValue},
		{"a skill that is not the SRD's", func(o *Overlay) { o.Classes[1].SkillFrom[1] = "skill:nope" }, "classes[1].skill_from[1]", ReasonReference},
		{"a proficiency that is not the SRD's", func(o *Overlay) { o.Classes[1].Proficiencies[0] = "proficiency:nope" }, "classes[1].proficiencies[0]", ReasonReference},
		{"a multiclass proficiency", func(o *Overlay) {
			o.Classes[1].MulticlassProficiencies = []string{"proficiency:light-armor", "skill:arcana"}
		}, "classes[1].multiclass_proficiencies[1]", ReasonReference},
		{"multiclass skills", func(o *Overlay) { o.Classes[1].MulticlassSkillChoose = 9 }, "classes[1].multiclass_skill_choose", ReasonValue},
		{"a minimum out of range", func(o *Overlay) { o.Classes[1].Minimums = map[Ability]int{WIS: 99} }, "classes[1].minimums.wisdom", ReasonValue},
		{"an any_of out of range", func(o *Overlay) { o.Classes[1].AnyOf = map[Ability]int{STR: 13, DEX: 0} }, "classes[1].any_of.dexterity", ReasonValue},
		{"a subclass level", func(o *Overlay) { o.Classes[1].SubclassLevel = 25 }, "classes[1].subclass_level", ReasonValue},
		{"a repeated ASI level", func(o *Overlay) { o.Classes[1].ASILevels = []int{4, 8, 4} }, "classes[1].asi_levels[2]", ReasonValue},
		{"an ASI level beyond 20", func(o *Overlay) { o.Classes[1].ASILevels = []int{25, 4} }, "classes[1].asi_levels[0]", ReasonValue},
		{"too few rows", func(o *Overlay) { o.Classes[3].Levels = o.Classes[3].Levels[:5] }, "classes[3].levels", ReasonTable},
		{"a proficiency bonus", func(o *Overlay) { o.Classes[1].Levels[2].ProfBonus = 13 }, "classes[1].levels[2].prof_bonus", ReasonValue},
		// The casting.
		{"a casting kind", func(o *Overlay) { o.Classes[1].Casting.Kind = CastingThird }, "classes[1].casting.kind", ReasonCasting},
		{"no casting ability", func(o *Overlay) { o.Classes[1].Casting.Ability = "" }, "classes[1].casting.ability", ReasonCasting},
		{"a preparation", func(o *Overlay) { o.Classes[1].Casting.Preparation = "memorizado" }, "classes[1].casting.preparation", ReasonCasting},
		{"prepared_max on a known caster", func(o *Overlay) { o.Classes[2].Casting.PreparedMax = "1" }, "classes[2].casting.prepared_max", ReasonCasting},
		{"a list that is not there", func(o *Overlay) { o.Classes[1].Casting.ListFrom = "class:fantasma" }, "classes[1].casting.list_from", ReasonReference},
		{"a list of a class that does not cast", func(o *Overlay) { o.Classes[1].Casting.ListFrom = "class:fighter" }, "classes[1].casting.list_from", ReasonCasting},
		{"a start level", func(o *Overlay) { o.Classes[1].Casting.StartLevel = 25 }, "classes[1].casting.start_level", ReasonCasting},
		{"a list on a class that does not cast", func(o *Overlay) { o.Classes[0].Casting.ListFrom = "class:wizard" }, "classes[0].casting.list_from", ReasonCasting},
		{"an SRD spell on a list that is reused", func(o *Overlay) { o.Classes[1].Casting.SRDSpells = []string{"spell:aid"} }, "classes[1].casting.srd_spell_keys", ReasonCasting},
		{"an SRD spell that is not there", func(o *Overlay) {
			o.Classes[1].Casting.ListFrom, o.Classes[1].Casting.SRDSpells = "", []string{"spell:aid", "spell:fantasma"}
		}, "classes[1].casting.srd_spell_keys[1]", ReasonReference},
		{"an SRD spell twice", func(o *Overlay) {
			o.Classes[1].Casting.ListFrom, o.Classes[1].Casting.SRDSpells = "", []string{"spell:aid", "spell:aid"}
		}, "classes[1].casting.srd_spell_keys[1]", ReasonValue},
		{"an SRD spell on a class that does not cast", func(o *Overlay) { o.Classes[0].Casting.SRDSpells = []string{"spell:aid"} }, "classes[0].casting.srd_spell_keys", ReasonCasting},
		{"expanded spells without spells", func(o *Overlay) { o.Subclasses[0].SpellsExpanded = true }, "subclasses[0].spells_expanded", ReasonValue},
		{"a prepared formula", func(o *Overlay) { o.Classes[1].Casting.PreparedMax = "rand()" }, "classes[1].casting.prepared_max", ReasonFormula},
		{"a prepared formula without parentheses", func(o *Overlay) { o.Classes[1].Casting.PreparedMax = "level" }, "classes[1].casting.prepared_max", ReasonFormula},
		{"a third caster's prepared formula", func(o *Overlay) { o.Subclasses[10].Casting.PreparedMax = "rand()" }, "subclasses[10].casting.prepared_max", ReasonFormula},
		{"a third caster's prepared formula without parentheses", func(o *Overlay) { o.Subclasses[10].Casting.PreparedMax = "level" }, "subclasses[10].casting.prepared_max", ReasonFormula},
		{"a spell's class list", func(o *Overlay) { o.Spells[1].Classes = []string{"class:wizard", "class:fantasma"} }, "spells[1].class_keys[1]", ReasonReference},
		{"a class listed twice", func(o *Overlay) { o.Spells[1].Classes = []string{"class:wizard", "class:wizard"} }, "spells[1].class_keys[1]", ReasonValue},
		{"a spell's casting time", func(o *Overlay) { o.Spells[1].CastingTime = TableCastingTime{Unit: "fortnight", Amount: 1} }, "spells[1].casting_time.unit", ReasonValue},
		// The rows.
		{"cantrips known", func(o *Overlay) { o.Classes[1].Levels[3].CantripsKnown = 99 }, "classes[1].levels[3].cantrips_known", ReasonTable},
		{"spells known", func(o *Overlay) { o.Classes[2].Levels[3].SpellsKnown = 999 }, "classes[2].levels[3].spells_known", ReasonTable},
		{"a slot count", func(o *Overlay) { o.Classes[1].Levels[4].Slots[2] = 10 }, "classes[1].levels[4].slots[2]", ReasonTable},
		{"slots before casting starts", func(o *Overlay) { o.Classes[3].Levels[0].Slots[0] = 2 }, "classes[3].levels[0].slots[0]", ReasonTable},
		{"cantrips before casting starts", func(o *Overlay) { o.Classes[3].Levels[0].CantripsKnown = 1 }, "classes[3].levels[0].cantrips_known", ReasonTable},
		{"pact slots of two levels", func(o *Overlay) { o.Classes[4].Levels[5].Slots[0] = 1 }, "classes[4].levels[5].slots[2]", ReasonTable},
		{"a caster with no slots", func(o *Overlay) { o.Classes[1].Levels[6].Slots = [9]int{} }, "classes[1].levels[6].slots", ReasonTable},
		// The features.
		{"a feature without a name", func(o *Overlay) { o.Classes[1].Levels[0].Features[1].NamePT = "" }, "classes[1].levels[0].features[1].name_pt", ReasonName},
		{"the 61st feature", func(o *Overlay) {
			for n := range 60 {
				o.Classes[0].Levels[0].Features = append(o.Classes[0].Levels[0].Features, tf("extra-"+strconv.Itoa(n), "Extra"))
			}
		}, "classes[0].levels[0].features[60]", ReasonLimit},
		// A subclass.
		{"a subclass level that is not the class's", func(o *Overlay) { o.Subclasses[0].Level = 9 }, "subclasses[0].level", ReasonValue},
		{"a third caster on a caster", func(o *Overlay) {
			o.Subclasses[2].Casting = &TableCasting{Kind: CastingThird, Ability: INT, Preparation: PreparationKnown, ListFrom: "class:wizard"}
		}, "subclasses[2].casting", ReasonCasting},
		{"a third caster without a list", func(o *Overlay) { o.Subclasses[9].Casting.ListFrom = "" }, "subclasses[9].casting.list_from", ReasonCasting},
		{"a third caster starting before the subclass", func(o *Overlay) { o.Subclasses[9].Casting.StartLevel = 2 }, "subclasses[9].casting.start_level", ReasonCasting},
		{"subclass levels out of order", func(o *Overlay) {
			l := o.Subclasses[0].Levels
			l[0], l[1] = l[1], l[0]
		}, "subclasses[0].levels[1].level", ReasonTable},
		{"subclass features before the subclass", func(o *Overlay) { o.Subclasses[0].Levels[0].Level = 2 }, "subclasses[0].levels[0].features", ReasonTable},
		{"a third caster missing a row", func(o *Overlay) { o.Subclasses[9].Levels = o.Subclasses[9].Levels[:5] }, "subclasses[9].levels", ReasonTable},
		{"a subclass slot count", func(o *Overlay) { o.Subclasses[9].Levels[2].Slots[0] = 10 }, "subclasses[9].levels[2].slots[0]", ReasonTable},
		{"an always-prepared level", func(o *Overlay) { o.Subclasses[2].AlwaysPrepared[1].ClassLevel = 0 }, "subclasses[2].always_prepared[1].class_level", ReasonValue},
		{"an always-prepared spell that is not there", func(o *Overlay) { o.Subclasses[2].AlwaysPrepared[1].Spell = "spell:fantasma" }, "subclasses[2].always_prepared[1].spell_key", ReasonReference},
		{"an always-prepared cantrip", func(o *Overlay) { o.Subclasses[2].AlwaysPrepared[0].Spell = "spell:fire-bolt" }, "subclasses[2].always_prepared[0].spell_key", ReasonValue},
		{"always-prepared spells without casting", func(o *Overlay) {
			o.Subclasses[0].AlwaysPrepared = []TableAlwaysPrepared{{ClassLevel: 3, Spell: "spell:bless"}}
		}, "subclasses[0].always_prepared", ReasonCasting},
		// The effects: each field of the menu.
		{"a filter on a choice of skills", effect(Effect{Type: "choice", Choice: "skill", Count: 1, Filter: &ChoiceFilter{ProficiencyKinds: []string{"tool"}}}), eff + ".filter", ReasonValue},
		{"a filter with no kind", effect(Effect{Type: "choice", Choice: "tool", Count: 1, Filter: &ChoiceFilter{}}), eff + ".filter.proficiency_kinds", ReasonValue},
		{"a filter with a kind that is no proficiency's", effect(Effect{Type: "choice", Choice: "tool", Count: 1, Filter: &ChoiceFilter{ProficiencyKinds: []string{"armor"}}}), eff + ".filter.proficiency_kinds", ReasonValue},
		{"a filter of spells with no class list", effect(Effect{Type: "choice", Choice: "cantrip", Count: 1, Filter: &ChoiceFilter{Ritual: true}}), eff + ".filter.class_lists", ReasonValue},
		{"a filter of tools that names a class list", effect(Effect{Type: "choice", Choice: "tool", Count: 1, Filter: &ChoiceFilter{ProficiencyKinds: []string{"tool"}, ClassLists: []string{"class:wizard"}}}), eff + ".filter", ReasonValue},
		{"a filter of cantrips with a kind of proficiency", effect(Effect{Type: "choice", Choice: "cantrip", Count: 1, Filter: &ChoiceFilter{ClassLists: []string{"class:wizard"}, ProficiencyKinds: []string{"tool"}}}), eff + ".filter.proficiency_kinds", ReasonValue},
		{"a filter of cantrips with a circle", effect(Effect{Type: "choice", Choice: "cantrip", Count: 1, Filter: &ChoiceFilter{ClassLists: []string{"class:wizard"}, SpellLevels: []int{0, 1}}}), eff + ".filter.spell_levels", ReasonValue},
		{"a filter of spells with the cantrip circle", effect(Effect{Type: "choice", Choice: "spell", Count: 1, Filter: &ChoiceFilter{ClassLists: []string{"class:wizard"}, SpellLevels: []int{0}}}), eff + ".filter.spell_levels", ReasonValue},
		{"a filter of spells with a circle twice", effect(Effect{Type: "choice", Choice: "spell", Count: 1, Filter: &ChoiceFilter{ClassLists: []string{"class:wizard"}, SpellLevels: []int{2, 2}}}), eff + ".filter.spell_levels", ReasonValue},
		{"a filter of spells with a circle past the ninth", effect(Effect{Type: "choice", Choice: "spell", Count: 1, Filter: &ChoiceFilter{ClassLists: []string{"class:wizard"}, SpellLevels: []int{10}}}), eff + ".filter.spell_levels", ReasonValue},
		{"a class list that is not there", effect(Effect{Type: "choice", Choice: "spell", Count: 1, Filter: &ChoiceFilter{ClassLists: []string{"class:fantasma"}}}), eff + ".filter.class_lists", ReasonReference},
		{"a class list that is a spell", effect(Effect{Type: "choice", Choice: "spell", Count: 1, Filter: &ChoiceFilter{ClassLists: []string{"spell:fireball"}}}), eff + ".filter.class_lists", ReasonReference},
		{"a class list twice", effect(Effect{Type: "choice", Choice: "spell", Count: 1, Filter: &ChoiceFilter{ClassLists: []string{"class:wizard", "class:wizard"}}}), eff + ".filter.class_lists", ReasonValue},
		{"too many class lists", effect(Effect{Type: "choice", Choice: "spell", Count: 1, Filter: &ChoiceFilter{ClassLists: []string{
			"class:bard", "class:cleric", "class:druid", "class:paladin", "class:ranger", "class:sorcerer", "class:warlock", "class:wizard", "class:fighter",
		}}}), eff + ".filter.class_lists", ReasonLimit},
		{"a school that is not there", effect(Effect{Type: "choice", Choice: "spell", Count: 1, Filter: &ChoiceFilter{ClassLists: []string{"class:wizard"}, Schools: []string{"school:fantasma"}}}), eff + ".filter.schools", ReasonReference},
		{"a school that is a damage type", effect(Effect{Type: "choice", Choice: "spell", Count: 1, Filter: &ChoiceFilter{ClassLists: []string{"class:wizard"}, Schools: []string{"damage-type:fire"}}}), eff + ".filter.schools", ReasonReference},
		{"a filter of spells with a list", effect(Effect{Type: "choice", Choice: "spell", Count: 1, From: []string{"class:wizard"}, Filter: &ChoiceFilter{ClassLists: []string{"class:wizard"}}}), eff + ".filter", ReasonValue},
		{"a feature that is not there", effect(Effect{Type: "choice", Choice: "feature", Count: 1, FromFeature: "feature:fantasma"}), eff + ".from_feature", ReasonReference},
		{"a feature with no options", effect(Effect{Type: "choice", Choice: "feature", Count: 1, FromFeature: "feature:second-wind"}), eff + ".from_feature", ReasonReference},
		{"more picks than the feature's options", effect(Effect{Type: "choice", Choice: "feature", Count: 7, FromFeature: "feature:fighter-fighting-style"}), eff + ".count", ReasonValue},
		{"a handler", effect(Effect{Type: "handler", Handler: "monk.martial_arts"}), eff + ".type", ReasonEffect},
		{"an effect type", effect(Effect{Type: "spellcasting"}), eff + ".type", ReasonEffect},
		{"a modifier target", effect(Effect{Type: "modifier", Target: "luck", Mode: "add", Value: "1"}), eff + ".target", ReasonValue},
		{"a modifier mode", effect(Effect{Type: "modifier", Target: "ac", Mode: "double", Value: "1"}), eff + ".mode", ReasonValue},
		{"a modifier without a value", effect(Effect{Type: "modifier", Target: "ac", Mode: "add"}), eff + ".value", ReasonValue},
		{"a value formula", effect(Effect{Type: "modifier", Target: "ac", Mode: "add", Value: "prof("}), eff + ".value", ReasonFormula},
		{"a when formula", effect(Effect{Type: "modifier", Target: "ac", Mode: "add", Value: "1", When: "level() +"}), eff + ".when", ReasonFormula},
		{"a max formula", effect(Effect{Type: "resource", Resource: "surto", Max: "rand()", Recharge: "long_rest"}), eff + ".max", ReasonFormula},
		{"a proficiency", effect(Effect{Type: "proficiency", Proficiency: "skill:nope"}), eff + ".proficiency", ReasonValue},
		{"only proficient on a full proficiency", effect(Effect{Type: "proficiency", Proficiency: "skill:arcana", Level: "full", OnlyProficient: true}), eff + ".only_proficient", ReasonValue},
		{"only proficient on a saving throw", effect(Effect{Type: "proficiency", Proficiency: "save.wis", Level: "expertise", OnlyProficient: true}), eff + ".only_proficient", ReasonValue},
		{"only proficient on a modifier", effect(Effect{Type: "modifier", Target: "ac", Mode: "add", Value: "1", OnlyProficient: true}), eff + ".only_proficient", ReasonValue},
		{"a proficiency level", effect(Effect{Type: "proficiency", Proficiency: "skill:arcana", Level: "triple"}), eff + ".level", ReasonValue},
		{"a roll mode", effect(Effect{Type: "roll_mode", Roll: "luck", Targets: []string{"save.wis"}}), eff + ".roll", ReasonValue},
		{"no roll targets", effect(Effect{Type: "roll_mode", Roll: "advantage"}), eff + ".targets", ReasonValue},
		{"a roll target", effect(Effect{Type: "roll_mode", Roll: "advantage", Targets: []string{"save.nope"}}), eff + ".targets", ReasonValue},
		{"a sense", effect(Effect{Type: "sense", Sense: "x-ray", RangeFt: 30}), eff + ".sense", ReasonValue},
		{"a sense range", effect(Effect{Type: "sense", Sense: "darkvision"}), eff + ".range_ft", ReasonValue},
		{"a resource name", effect(Effect{Type: "resource", Resource: "Nome Ruim", Max: "1", Recharge: "long_rest"}), eff + ".resource", ReasonValue},
		{"a resource max", effect(Effect{Type: "resource", Resource: "surto", Recharge: "long_rest"}), eff + ".max", ReasonValue},
		{"a recharge", effect(Effect{Type: "resource", Resource: "surto", Max: "1", Recharge: "weekly"}), eff + ".recharge", ReasonValue},
		{"a choice kind", effect(Effect{Type: "choice", Choice: "subclass", Count: 1}), eff + ".choice", ReasonEffect},
		{"a choice count", effect(Effect{Type: "choice", Choice: "skill"}), eff + ".count", ReasonValue},
		{"a choice option", effect(Effect{Type: "choice", Choice: "feature", Count: 1, From: []string{"feature:fantasma"}}), eff + ".from", ReasonReference},
		{"a choice from the table", effect(Effect{Type: "choice", Choice: "feature", Count: 1, From: []string{"feature:gen-none-vigor@mesa"}}), eff + ".from", ReasonReference},
		{"an economy", effect(Effect{Type: "grant_action", Economy: "whenever"}), eff + ".economy", ReasonValue},
		{"an attack count", effect(Effect{Type: "extra_attack", Count: 9}), eff + ".count", ReasonValue},
		{"a note value without a text", effect(Effect{Type: "note", Value: "prof()"}), eff + ".text_pt", ReasonValue},
		{"an empty tag", effect(Effect{Type: "roll_mode", Roll: "advantage", Targets: []string{"save.wis"}, Tags: []string{""}}), eff + ".tags", ReasonValue},
		{"a granted spell outside a note", effect(Effect{Type: "modifier", Target: "ac", Mode: "add", Value: "1", Spells: []string{"spell:light"}}), eff + ".spells", ReasonEffect},
		{"a granted spell that is not there", effect(Effect{Type: "note", Spells: []string{"spell:fantasma"}}), eff + ".spells", ReasonReference},
		{"a field the type does not read", effect(Effect{Type: "modifier", Target: "ac", Mode: "add", Value: "1", Recharge: "long_rest"}), eff + ".recharge", ReasonValue},
		{"a count on a sense", effect(Effect{Type: "sense", Sense: "darkvision", RangeFt: 30, Count: 2}), eff + ".count", ReasonValue},
		{"an engine field", effect(Effect{Type: "note", Ability: "int"}), eff + ".ability", ReasonValue},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			o := fullOverlay(t, srd)
			tc.edit(&o)
			// Everything here is a write: the stray fields are refused.
			for _, c := range o.Classes {
				o.Strict = append(o.Strict, c.Key)
			}
			for _, s := range o.Subclasses {
				o.Strict = append(o.Strict, s.Key)
			}
			_, err := srd.With(o)
			var oe *OverlayError
			if !errors.As(err, &oe) {
				t.Fatalf("err = %v, want an OverlayError", err)
			}
			if oe.Field != tc.field || oe.Reason != tc.reason {
				t.Errorf("field %q reason %q, want %q %q (%v)", oe.Field, oe.Reason, tc.field, tc.reason, err)
			}
		})
	}
}

// TestStrayEffectFieldsAreIgnoredWhenRead: a field an effect's type does not read
// is refused only in the entry being written (Overlay.Strict); in an entry that
// comes from storage it is ignored, so a campaign never becomes unreadable.
func TestStrayEffectFieldsAreIgnoredWhenRead(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	o := fullOverlay(t, srd)
	o.Classes[1].Levels[0].Features[0].Effects = []Effect{{Type: "sense", Sense: "darkvision", RangeFt: 60, Recharge: "long_rest", Count: 2}}
	if _, err := srd.With(o); err != nil {
		t.Fatalf("a stored stray field made the content unreadable: %v", err)
	}
	o.Strict = []string{o.Classes[1].Key}
	var oe *OverlayError
	if _, err := srd.With(o); !errors.As(err, &oe) || oe.Field != "classes[1].levels[0].features[0].effects[0].recharge" {
		t.Errorf("a stray field in the entry being written: %v, want it refused at the field", err)
	}
	// The caller's effects are never written.
	if o.Classes[1].Levels[0].Features[0].Effects[0].Recharge != "long_rest" {
		t.Error("With changed the caller's effect")
	}
}
