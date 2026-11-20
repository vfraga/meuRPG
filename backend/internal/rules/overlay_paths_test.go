package rules

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// entryViolationRows pins the path and the reason of every field refusal of a
// table spell, race, subrace and background (MR-025).
// `field` is the path in the request's body, as the API sends it
// (TableContentViolation.field); the web copies these strings (and reasons) into
// content-violations.spec.ts: change a row here and there together. `list` and `i`
// say which entry of fullOverlay the edit is made on.
var entryViolationRows = []struct {
	name   string
	list   string
	i      int
	edit   func(o *Overlay)
	field  string
	reason string
}{
	// A spell.
	{"the circle", "spells", 0, func(o *Overlay) { o.Spells[0].Level = 12 }, "table_spell.level", ReasonValue},
	{"the school", "spells", 0, func(o *Overlay) { o.Spells[0].School = "school:poesia" }, "table_spell.school_key", ReasonReference},
	{"the range kind", "spells", 0, func(o *Overlay) { o.Spells[0].Range.Kind = "portal" }, "table_spell.range.kind", ReasonValue},
	{"the range distance", "spells", 0, func(o *Overlay) { o.Spells[0].Range.DistanceFt = 7 }, "table_spell.range.distance_ft", ReasonLimit},
	{"a self spell with a distance", "spells", 1, func(o *Overlay) {
		o.Spells[1].Range = TableRange{Kind: RangeRanged, DistanceFt: 60}
		o.Spells[1].Target = SpellTarget{Kind: TargetSelf}
	}, "table_spell.range.kind", ReasonValue},
	{"the target kind", "spells", 0, func(o *Overlay) { o.Spells[0].Target.Kind = "crowd" }, "table_spell.target.kind", ReasonValue},
	{"the area shape", "spells", 1, func(o *Overlay) { o.Spells[1].Target.Shape = "blob" }, "table_spell.target.shape", ReasonValue},
	{"the area size", "spells", 1, func(o *Overlay) { o.Spells[1].Target.SizeFt = 7 }, "table_spell.target.size_ft", ReasonLimit},
	{"the creatures count", "spells", 2, func(o *Overlay) { o.Spells[2].Target.Count = 1 }, "table_spell.target.count", ReasonLimit},
	{"the extra creatures per circle", "spells", 2, func(o *Overlay) { o.Spells[2].Target.PerSlotLevel = 11 }, "table_spell.target.per_slot_level", ReasonLimit},
	{"the casting time unit", "spells", 0, func(o *Overlay) { o.Spells[0].CastingTime = TableCastingTime{Unit: "fortnight", Amount: 1} }, "table_spell.casting_time.unit", ReasonValue},
	{"the casting time amount", "spells", 0, func(o *Overlay) { o.Spells[0].CastingTime = TableCastingTime{Unit: CastMinute, Amount: 90} }, "table_spell.casting_time.amount", ReasonLimit},
	{"a trigger on an action", "spells", 0, func(o *Overlay) { o.Spells[0].CastingTime.TriggerPT = "quando você sofre dano" }, "table_spell.casting_time.trigger_pt", ReasonValue},
	{"the duration kind", "spells", 0, func(o *Overlay) { o.Spells[0].Duration.Kind = "forever" }, "table_spell.duration.kind", ReasonValue},
	{"the duration amount", "spells", 2, func(o *Overlay) { o.Spells[2].Duration.Amount = 0 }, "table_spell.duration.amount", ReasonLimit},
	{"the duration unit", "spells", 2, func(o *Overlay) { o.Spells[2].Duration.Unit = "year" }, "table_spell.duration.unit", ReasonValue},
	{"concentration without a time", "spells", 0, func(o *Overlay) { o.Spells[0].Concentration = true }, "table_spell.concentration", ReasonValue},
	{"a cantrip ritual", "spells", 4, func(o *Overlay) { o.Spells[4].Ritual = true }, "table_spell.ritual", ReasonValue},
	{"a material text without M", "spells", 0, func(o *Overlay) { o.Spells[0].Components.MaterialPT = "uma pena" }, "table_spell.components.material_pt", ReasonValue},
	{"the attack", "spells", 0, func(o *Overlay) { o.Spells[0].Attack = "thrown" }, "table_spell.attack", ReasonValue},
	{"an attack and a save", "spells", 0, func(o *Overlay) { o.Spells[0].Save = &SpellSave{Ability: DEX, OnSuccess: "half"} }, "table_spell.save", ReasonValue},
	{"the save ability", "spells", 1, func(o *Overlay) { o.Spells[1].Save.Ability = "luck" }, "table_spell.save.ability", ReasonValue},
	{"the save on success", "spells", 1, func(o *Overlay) { o.Spells[1].Save.OnSuccess = "third" }, "table_spell.save.on_success", ReasonValue},
	{"a save for half without damage", "spells", 1, func(o *Overlay) { o.Spells[1].Damage = nil }, "table_spell.save.on_success", ReasonValue},
	{"too many damage types", "spells", 0, func(o *Overlay) {
		for range 4 {
			o.Spells[0].Damage = append(o.Spells[0].Damage, TableSpellDamage{Type: "damage-type:fire", Dice: "1d6"})
		}
	}, "table_spell.damage", ReasonLimit},
	{"the damage dice", "spells", 0, func(o *Overlay) { o.Spells[0].Damage[0].Dice = "3d6x" }, "table_spell.damage[0].dice", ReasonValue},
	{"the damage dice of the second type", "spells", 0, func(o *Overlay) {
		o.Spells[0].Damage = append(o.Spells[0].Damage, TableSpellDamage{Type: "damage-type:fire", Dice: "many"})
	}, "table_spell.damage[1].dice", ReasonValue},
	{"the damage per circle", "spells", 0, func(o *Overlay) { o.Spells[0].Damage[0].PerSlotLevel = "1d8" }, "table_spell.damage[0].per_slot_level", ReasonValue},
	{"the damage per tier", "spells", 4, func(o *Overlay) { o.Spells[4].Damage[0].PerTier = "1d6" }, "table_spell.damage[0].per_tier", ReasonValue},
	{"the damage type", "spells", 0, func(o *Overlay) { o.Spells[0].Damage[0].Type = "damage-type:love" }, "table_spell.damage[0].damage_type_key", ReasonReference},
	{"the heal dice", "spells", 3, func(o *Overlay) { o.Spells[3].Heal.Dice = "big" }, "table_spell.heal.dice", ReasonValue},
	{"the heal per circle", "spells", 3, func(o *Overlay) { o.Spells[3].Heal.PerSlotLevel = "1d6" }, "table_spell.heal.per_slot_level", ReasonValue},
	{"a cantrip that heals", "spells", 4, func(o *Overlay) { o.Spells[4].Heal = &TableSpellHeal{Dice: "1d4"} }, "table_spell.heal", ReasonValue},
	{"a description paragraph", "spells", 0, func(o *Overlay) { o.Spells[0].DescPT = []string{"ok", strings.Repeat("x", 5000)} }, "table_spell.desc_pt[1]", ReasonText},
	{"a class of the list", "spells", 0, func(o *Overlay) { o.Spells[0].Classes = []string{"class:fantasma"} }, "table_spell.class_keys[0]", ReasonReference},
	{"a class listed twice", "spells", 0, func(o *Overlay) { o.Spells[0].Classes = []string{"class:wizard", "class:wizard"} }, "table_spell.class_keys[1]", ReasonValue},
	// A race.
	{"the size", "races", 0, func(o *Overlay) { o.Races[0].Size = "Huge" }, "table_race.size", ReasonValue},
	{"the speed", "races", 0, func(o *Overlay) { o.Races[0].SpeedFt = 7 }, "table_race.speed_ft", ReasonLimit},
	{"the darkvision", "races", 0, func(o *Overlay) { o.Races[0].DarkvisionFt = 200 }, "table_race.darkvision_ft", ReasonLimit},
	{"an ability bonus", "races", 0, func(o *Overlay) { o.Races[0].AbilityBonuses[WIS] = 9 }, "table_race.ability_bonuses.wisdom", ReasonLimit},
	{"too many bonuses to place", "races", 0, func(o *Overlay) { o.Races[0].ChoiceBonuses = []int{1, 1, 1, 1, 1, 1, 1} }, "table_race.choice_bonuses", ReasonLimit},
	{"a bonus to place", "races", 0, func(o *Overlay) { o.Races[0].ChoiceBonuses = []int{2, 9} }, "table_race.choice_bonuses[1]", ReasonLimit},
	{"the languages to choose", "races", 0, func(o *Overlay) { o.Races[0].LanguageChoices = 9 }, "table_race.language_choices", ReasonLimit},
	{"a language", "races", 0, func(o *Overlay) { o.Races[0].Languages = []string{"language:common", "language:klingon"} }, "table_race.languages[1]", ReasonReference},
	{"a trait effect", "races", 0, func(o *Overlay) { o.Races[0].Traits[0].Effects[0].Roll = "luck" }, "table_race.traits[0].effects[0].roll", ReasonValue},
	// A subrace.
	{"the race of a subrace", "subraces", 0, func(o *Overlay) { o.Subraces[0].Race = "race:fantasma" }, "table_subrace.race_key", ReasonReference},
	{"a subrace bonus", "subraces", 0, func(o *Overlay) { o.Subraces[0].AbilityBonuses[DEX] = -9 }, "table_subrace.ability_bonuses.dexterity", ReasonLimit},
	// A background.
	{"one skill", "backgrounds", 0, func(o *Overlay) { o.Backgrounds[0].Skills = []string{"skill:insight"} }, "table_background.skills", ReasonValue},
	{"a skill", "backgrounds", 0, func(o *Overlay) { o.Backgrounds[0].Skills = []string{"skill:insight", "skill:cooking"} }, "table_background.skills[1]", ReasonReference},
	{"the same skill twice", "backgrounds", 0, func(o *Overlay) { o.Backgrounds[0].Skills = []string{"skill:insight", "skill:insight"} }, "table_background.skills[1]", ReasonValue},
	{"too many tools", "backgrounds", 0, func(o *Overlay) {
		o.Backgrounds[0].Tools = []string{"proficiency:thieves-tools", "proficiency:thieves-tools", "proficiency:thieves-tools", "proficiency:thieves-tools", "proficiency:thieves-tools"}
	}, "table_background.tools", ReasonLimit},
	{"a tool", "backgrounds", 0, func(o *Overlay) { o.Backgrounds[0].Tools = []string{"proficiency:light-armor"} }, "table_background.tools[0]", ReasonReference},
	{"the languages to choose of a background", "backgrounds", 0, func(o *Overlay) { o.Backgrounds[0].LanguageChoices = 9 }, "table_background.language_choices", ReasonLimit},
	{"the equipment", "backgrounds", 0, func(o *Overlay) { o.Backgrounds[0].EquipmentPT = strings.Repeat("x", 5000) }, "table_background.equipment_pt", ReasonText},
	{"a filter of a choice of tools", "backgrounds", 0, func(o *Overlay) {
		o.Backgrounds[0].Feature.Effects[0] = Effect{Type: "choice", Choice: "tool", Count: 1, Filter: &ChoiceFilter{ProficiencyKinds: []string{"armor"}}}
	}, "table_background.feature.effects[0].filter.proficiency_kinds", ReasonValue},
	{"a filter of a choice of skills", "backgrounds", 0, func(o *Overlay) {
		o.Backgrounds[0].Feature.Effects[0] = Effect{Type: "choice", Choice: "skill", Count: 1, Filter: &ChoiceFilter{ProficiencyKinds: []string{"tool"}}}
	}, "table_background.feature.effects[0].filter", ReasonValue},
	{"the options of a feature on a background", "backgrounds", 0, func(o *Overlay) {
		o.Backgrounds[0].Feature.Effects[0] = Effect{Type: "choice", Choice: "feature", Count: 1, FromFeature: "feature:fighter-fighting-style"}
	}, "table_background.feature.effects[0].from_feature", ReasonValue},
	{"too many picks of a filter", "backgrounds", 0, func(o *Overlay) {
		o.Backgrounds[0].Feature.Effects[0] = Effect{Type: "choice", Choice: "tool", Count: 99, Filter: &ChoiceFilter{ProficiencyKinds: []string{"tool"}}}
	}, "table_background.feature.effects[0].count", ReasonValue},
	{"the feature effect", "backgrounds", 0, func(o *Overlay) { o.Backgrounds[0].Feature.Effects[0].Type = "handler" }, "table_background.feature.effects[0].type", ReasonEffect},
}

// bodyToOverlayPath turns the API's path into the overlay's: "table_spell.range.kind"
// of spells[1] is "spells[1].range.kind".
func bodyToOverlayPath(list string, i int, field string) string {
	_, rest, _ := strings.Cut(field, ".")
	return fmt.Sprintf("%s[%d].%s", list, i, rest)
}

// The class and subclass paths and reasons are pinned by TestClassRefusalsNameTheirField (overlay_fields_test.go); the web
// copies them into content-violations.spec.ts (CLASS_ROWS): change a row there and here together.
//
// TestEntryViolationPaths: every row of the table above comes back at its exact
// path with its reason, and a saving throw never lands on a class's field.
func TestEntryViolationPaths(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	for _, tc := range entryViolationRows {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			o := fullOverlay(t, srd)
			tc.edit(&o)
			_, err := srd.With(o)
			var oe *OverlayError
			if !errors.As(err, &oe) {
				t.Fatalf("err = %v, want an OverlayError", err)
			}
			want := bodyToOverlayPath(tc.list, tc.i, tc.field)
			var found *OverlayError
			for _, v := range oe.Violations() {
				if v.Field == want {
					found = v
				}
				if strings.Contains(v.Field, "saving_throws") && strings.HasPrefix(tc.field, "table_spell") {
					t.Errorf("a spell's violation landed on a class field: %q", v.Field)
				}
			}
			if found == nil {
				t.Fatalf("no violation at %q; got %v", want, fieldsOf(oe))
			}
			if found.Reason != tc.reason {
				t.Errorf("reason %q at %q, want %q (%s)", found.Reason, want, tc.reason, found.Message)
			}
		})
	}
}

func fieldsOf(oe *OverlayError) []string {
	var out []string
	for _, v := range oe.Violations() {
		out = append(out, v.Field+" ("+v.Reason+")")
	}
	return out
}

// TestAnEntryReportsEveryViolationAtOnce: a spell, a race and a background with
// several fields wrong come back with all of them, in the order the fields are
// checked, so the editor marks every one in one answer.
func TestAnEntryReportsEveryViolationAtOnce(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	cases := []struct {
		name string
		edit func(o *Overlay)
		want []string
	}{
		{"a spell", func(o *Overlay) {
			o.Spells[0].Range.DistanceFt = 7
			o.Spells[0].Damage[0].Dice = "3d6x"
			o.Spells[0].DescPT = []string{strings.Repeat("x", 5000)}
			o.Spells[0].Components.MaterialPT = "uma pena"
		}, []string{"spells[0].range.distance_ft", "spells[0].components.material_pt", "spells[0].desc_pt[0]", "spells[0].damage[0].dice"}},
		{"a race", func(o *Overlay) {
			o.Races[0].SpeedFt = 7
			o.Races[0].AbilityBonuses[WIS] = 9
			o.Races[0].Languages = []string{"language:klingon"}
			o.Races[0].Traits[0].Effects[0].Roll = "luck"
		}, []string{"races[0].speed_ft", "races[0].ability_bonuses.wisdom", "races[0].languages[0]", "races[0].traits[0].effects[0].roll"}},
		{"a background", func(o *Overlay) {
			o.Backgrounds[0].Skills = []string{"skill:insight", "skill:cooking"}
			o.Backgrounds[0].Tools = []string{"proficiency:light-armor"}
			o.Backgrounds[0].LanguageChoices = 9
		}, []string{"backgrounds[0].skills[1]", "backgrounds[0].language_choices", "backgrounds[0].tools[0]"}},
		// A class and a subclass: three mistakes need one save, not three.
		{"a class", func(o *Overlay) {
			o.Classes[1].HitDie = 7
			o.Classes[1].SavingThrows = []Ability{CON, CON}
			o.Classes[1].SkillFrom[1] = "skill:nope"
			o.Classes[1].Levels[0].CantripsKnown = 99
			o.Classes[1].Levels[4].Slots[2] = 12
		}, []string{"classes[1].hit_die", "classes[1].saving_throws[1]", "classes[1].skill_from[1]", "classes[1].levels[0].cantrips_known", "classes[1].levels[4].slots[2]"}},
		{"a class's casting and multiclass", func(o *Overlay) {
			o.Classes[1].Casting.Ability = "luck"
			o.Classes[1].Casting.StartLevel = 40
			o.Classes[1].Minimums = map[Ability]int{WIS: 99, INT: 0}
			o.Classes[1].ASILevels = []int{4, 4, 25}
		}, []string{"classes[1].minimums.intelligence", "classes[1].minimums.wisdom", "classes[1].asi_levels[1]", "classes[1].asi_levels[2]", "classes[1].casting.ability", "classes[1].casting.start_level"}},
		{"a subclass", func(o *Overlay) {
			o.Subclasses[2].DescPT = []string{strings.Repeat("x", 5000)}
			o.Subclasses[2].AlwaysPrepared[0].ClassLevel = 25
			o.Subclasses[2].AlwaysPrepared = append(o.Subclasses[2].AlwaysPrepared, TableAlwaysPrepared{ClassLevel: 3, Spell: "spell:nope"})
		}, []string{"subclasses[2].desc_pt[0]", "subclasses[2].always_prepared[0].class_level", "subclasses[2].always_prepared[2].spell_key"}},
	}
	for _, tc := range cases {
		o := fullOverlay(t, srd)
		tc.edit(&o)
		_, err := srd.With(o)
		var oe *OverlayError
		if !errors.As(err, &oe) {
			t.Fatalf("%s: err = %v", tc.name, err)
		}
		var got []string
		for _, v := range oe.Violations() {
			got = append(got, v.Field)
		}
		if strings.Join(got, "|") != strings.Join(tc.want, "|") {
			t.Errorf("%s: fields = %v, want %v", tc.name, got, tc.want)
		}
	}
}
