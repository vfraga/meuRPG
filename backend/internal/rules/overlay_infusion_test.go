package rules

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

// The typed data of an infusion: an option of a host feature that carries an infusion rule. Every name here is
// invented; the table's own artificer class is the pack's, not the SRD's.

const (
	dmFireKey   = "damage-type:fire"
	dmColdKey   = "damage-type:cold"
	infusionPre = "subclasses[0].levels[0].features[0]"
)

// infusionOverlay is the posture overlay with the rule on its host feature and an infusion on every option.
func infusionOverlay() Overlay {
	o := postureOverlay()
	host := &o.Subclasses[0].Levels[0].Features[0]
	host.InfusionRule = &TableInfusionRule{LimitSteps: []TableInfusionLimitStep{{ClassLevel: 2, Active: 2}, {ClassLevel: 6, Active: 3}, {ClassLevel: 10, Active: 4}}}
	steps := []TableInfusionStep{{ClassLevel: 2, Bonus: 1}, {ClassLevel: 10, Bonus: 2}}
	host.Options[0].Infusion = &TableInfusion{Accepts: InfusionAcceptsWeapon, Effect: InfusionEffectItemBonus, BonusSteps: steps}
	host.Options[1].Infusion = &TableInfusion{Accepts: InfusionAcceptsArmor, Effect: InfusionEffectResistanceLabel, RequiresAttunement: true, DamageTypes: []string{dmFireKey, dmColdKey}}
	host.Options[2].Infusion = &TableInfusion{Accepts: InfusionAcceptsAnyObject, Effect: InfusionEffectNone}
	host.Options[3].Infusion = &TableInfusion{Accepts: InfusionAcceptsSpellcastingFocus, Effect: InfusionEffectSpellFocusBonus, BonusSteps: steps}
	host.Options[4].Infusion = &TableInfusion{Accepts: InfusionAcceptsThrownWeapon, Effect: InfusionEffectReturning}
	return o
}

func TestAnInfusionOptionIsKnownToTheContent(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, infusionOverlay())

	got, ok := c.Infusion(postureBull)
	if !ok {
		t.Fatalf("Infusion(%s) not found", postureBull)
	}
	if got.Accepts != InfusionAcceptsArmor || got.Effect != InfusionEffectResistanceLabel || !got.RequiresAttunement || !slices.Equal(got.DamageTypes, []string{dmFireKey, dmColdKey}) {
		t.Errorf("Infusion(bull) = %+v", got)
	}
	if bonus, _ := c.Infusion(postureHeron); len(bonus.BonusSteps) != 2 || bonus.BonusSteps[1] != (TableInfusionStep{ClassLevel: 10, Bonus: 2}) {
		t.Errorf("Infusion(heron).BonusSteps = %+v", bonus.BonusSteps)
	}
	if _, ok := c.Infusion(postureMore); ok {
		t.Errorf("a feature that is not an infusion option is one")
	}
	if _, ok := c.Infusion("feature:fantasma" + tableSuffix); ok {
		t.Errorf("an unknown key is an infusion")
	}
	// The copy is the caller's: changing it leaves the content alone.
	got.DamageTypes[0] = dmColdKey
	if again, _ := c.Infusion(postureBull); again.DamageTypes[0] != dmFireKey {
		t.Errorf("the content changed through the copy: %v", again.DamageTypes)
	}
}

func TestTheHostOfAnInfusionOptionIsItsFeature(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, infusionOverlay())
	for _, k := range []string{postureHeron, postureBull, postureWolf, postureCrow, postureBear} {
		if got := c.InfusionHost(k); got != postureFeature {
			t.Errorf("InfusionHost(%s) = %q, want %q", k, got, postureFeature)
		}
	}
	for _, k := range []string{postureFeature, postureMore, "feature:fantasma" + tableSuffix, ""} {
		if got := c.InfusionHost(k); got != "" {
			t.Errorf("InfusionHost(%q) = %q, want none", k, got)
		}
	}
}

func TestTheActiveLimitOfAHostFollowsTheClassLevel(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, infusionOverlay())
	cases := []struct{ level, want int }{{0, 0}, {1, 0}, {2, 2}, {5, 2}, {6, 3}, {9, 3}, {10, 4}, {20, 4}}
	for _, tc := range cases {
		if got := c.InfusionLimit(postureFeature, tc.level); got != tc.want {
			t.Errorf("InfusionLimit(host, %d) = %d, want %d", tc.level, got, tc.want)
		}
	}
	if got := c.InfusionLimit(postureMore, 10); got != 0 {
		t.Errorf("InfusionLimit of a feature with no rule = %d, want 0", got)
	}
}

func TestInfusionDataIsRefusedAtItsField(t *testing.T) {
	t.Parallel()
	const opt0 = infusionPre + ".options[0]"
	step := func(l, b int) TableInfusionStep { return TableInfusionStep{ClassLevel: l, Bonus: b} }
	limit := func(l, a int) TableInfusionLimitStep { return TableInfusionLimitStep{ClassLevel: l, Active: a} }
	cases := []struct {
		name   string
		edit   func(o *Overlay)
		field  string
		reason string
	}{
		{"an infusion with no rule on the host", func(o *Overlay) { o.Subclasses[0].Levels[0].Features[0].InfusionRule = nil }, opt0 + ".infusion", ReasonValue},
		{"an infusion on a feature that is not an option", func(o *Overlay) {
			o.Subclasses[0].Levels[1].Features[0].Infusion = &TableInfusion{Accepts: InfusionAcceptsAnyObject, Effect: InfusionEffectNone}
		}, "subclasses[0].levels[1].features[0].infusion", ReasonValue},
		{"a rule on an option", func(o *Overlay) {
			o.Subclasses[0].Levels[0].Features[0].Options[0].InfusionRule = &TableInfusionRule{LimitSteps: []TableInfusionLimitStep{limit(2, 1)}}
		}, opt0 + ".infusion_rule", ReasonValue},
		{"a rule on a feature with no options", func(o *Overlay) {
			o.Subclasses[0].Levels[1].Features[0].InfusionRule = &TableInfusionRule{LimitSteps: []TableInfusionLimitStep{limit(2, 1)}}
		}, "subclasses[0].levels[1].features[0].infusion_rule", ReasonValue},
		{"a rule with no steps", func(o *Overlay) { o.Subclasses[0].Levels[0].Features[0].InfusionRule.LimitSteps = nil }, infusionPre + ".infusion_rule.limit_steps", ReasonValue},
		{"steps that do not ascend", func(o *Overlay) {
			o.Subclasses[0].Levels[0].Features[0].InfusionRule.LimitSteps = []TableInfusionLimitStep{limit(6, 2), limit(6, 3)}
		}, infusionPre + ".infusion_rule.limit_steps[1].class_level", ReasonValue},
		{"a step at level 0", func(o *Overlay) {
			o.Subclasses[0].Levels[0].Features[0].InfusionRule.LimitSteps = []TableInfusionLimitStep{limit(0, 2)}
		}, infusionPre + ".infusion_rule.limit_steps[0].class_level", ReasonValue},
		{"a step at level 21", func(o *Overlay) {
			o.Subclasses[0].Levels[0].Features[0].InfusionRule.LimitSteps = []TableInfusionLimitStep{limit(21, 2)}
		}, infusionPre + ".infusion_rule.limit_steps[0].class_level", ReasonValue},
		{"a step that allows none", func(o *Overlay) {
			o.Subclasses[0].Levels[0].Features[0].InfusionRule.LimitSteps = []TableInfusionLimitStep{limit(2, 0)}
		}, infusionPre + ".infusion_rule.limit_steps[0].active", ReasonValue},
		{"no accepts", func(o *Overlay) { o.Subclasses[0].Levels[0].Features[0].Options[0].Infusion.Accepts = "" }, opt0 + ".infusion.accepts", ReasonValue},
		{"no effect", func(o *Overlay) { o.Subclasses[0].Levels[0].Features[0].Options[0].Infusion.Effect = "" }, opt0 + ".infusion.effect", ReasonValue},
		{"an accepts that does not exist", func(o *Overlay) { o.Subclasses[0].Levels[0].Features[0].Options[0].Infusion.Accepts = "bolso" }, opt0 + ".infusion.accepts", ReasonValue},
		{"a bonus with no steps", func(o *Overlay) { o.Subclasses[0].Levels[0].Features[0].Options[0].Infusion.BonusSteps = nil }, opt0 + ".infusion.bonus_steps", ReasonValue},
		{"bonus steps that do not ascend", func(o *Overlay) {
			o.Subclasses[0].Levels[0].Features[0].Options[0].Infusion.BonusSteps = []TableInfusionStep{step(10, 1), step(2, 2)}
		}, opt0 + ".infusion.bonus_steps[1].class_level", ReasonValue},
		{"a bonus step at level 0", func(o *Overlay) {
			o.Subclasses[0].Levels[0].Features[0].Options[0].Infusion.BonusSteps = []TableInfusionStep{step(0, 1)}
		}, opt0 + ".infusion.bonus_steps[0].class_level", ReasonValue},
		{"a bonus step at level 21", func(o *Overlay) {
			o.Subclasses[0].Levels[0].Features[0].Options[0].Infusion.BonusSteps = []TableInfusionStep{step(21, 1)}
		}, opt0 + ".infusion.bonus_steps[0].class_level", ReasonValue},
		{"a bonus of 0", func(o *Overlay) {
			o.Subclasses[0].Levels[0].Features[0].Options[0].Infusion.BonusSteps = []TableInfusionStep{step(2, 0)}
		}, opt0 + ".infusion.bonus_steps[0].bonus", ReasonValue},
		{"a bonus of 4", func(o *Overlay) {
			o.Subclasses[0].Levels[0].Features[0].Options[0].Infusion.BonusSteps = []TableInfusionStep{step(2, 4)}
		}, opt0 + ".infusion.bonus_steps[0].bonus", ReasonValue},
		{"bonus steps on a text-only infusion", func(o *Overlay) {
			o.Subclasses[0].Levels[0].Features[0].Options[2].Infusion.BonusSteps = []TableInfusionStep{step(2, 1)}
		}, infusionPre + ".options[2].infusion.bonus_steps", ReasonValue},
		{"a resistance with no damage types", func(o *Overlay) { o.Subclasses[0].Levels[0].Features[0].Options[1].Infusion.DamageTypes = nil }, infusionPre + ".options[1].infusion.damage_types", ReasonValue},
		{"a damage type the app does not know", func(o *Overlay) {
			o.Subclasses[0].Levels[0].Features[0].Options[1].Infusion.DamageTypes = []string{"spell:light"}
		}, infusionPre + ".options[1].infusion.damage_types[0]", ReasonReference},
		{"a damage type twice", func(o *Overlay) {
			o.Subclasses[0].Levels[0].Features[0].Options[1].Infusion.DamageTypes = []string{dmFireKey, dmFireKey}
		}, infusionPre + ".options[1].infusion.damage_types[1]", ReasonValue},
		{"damage types on a bonus", func(o *Overlay) {
			o.Subclasses[0].Levels[0].Features[0].Options[0].Infusion.DamageTypes = []string{dmFireKey}
		}, opt0 + ".infusion.damage_types", ReasonValue},
		{"repeating ammunition on a weapon", func(o *Overlay) {
			o.Subclasses[0].Levels[0].Features[0].Options[2].Infusion = &TableInfusion{Accepts: InfusionAcceptsWeapon, Effect: InfusionEffectRepeatingAmmunition}
		}, infusionPre + ".options[2].infusion.accepts", ReasonValue},
		{"returning on a plain weapon", func(o *Overlay) {
			o.Subclasses[0].Levels[0].Features[0].Options[4].Infusion.Accepts = InfusionAcceptsWeapon
		}, infusionPre + ".options[4].infusion.accepts", ReasonValue},
		{"a focus bonus on a weapon", func(o *Overlay) {
			o.Subclasses[0].Levels[0].Features[0].Options[3].Infusion.Accepts = InfusionAcceptsWeapon
		}, infusionPre + ".options[3].infusion.accepts", ReasonValue},
		{"a resistance on armor or a shield", func(o *Overlay) {
			o.Subclasses[0].Levels[0].Features[0].Options[1].Infusion.Accepts = InfusionAcceptsArmorOrShield
		}, infusionPre + ".options[1].infusion.accepts", ReasonValue},
		{"a bonus on any object", func(o *Overlay) {
			o.Subclasses[0].Levels[0].Features[0].Options[0].Infusion.Accepts = InfusionAcceptsAnyObject
		}, opt0 + ".infusion.accepts", ReasonValue},
		{"a bonus on a focus", func(o *Overlay) {
			o.Subclasses[0].Levels[0].Features[0].Options[0].Infusion.Accepts = InfusionAcceptsSpellcastingFocus
		}, opt0 + ".infusion.accepts", ReasonValue},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			o := infusionOverlay()
			tc.edit(&o)
			_, err := loadForTest(t).With(o)
			oe, ok := errors.AsType[*OverlayError](err)
			if !ok {
				t.Fatalf("want *OverlayError, got %T: %v", err, err)
			}
			for _, v := range oe.Violations() {
				if v.Field == tc.field {
					if v.Reason != tc.reason {
						t.Errorf("reason at %s = %q, want %q", tc.field, v.Reason, tc.reason)
					}
					return
				}
			}
			t.Errorf("no violation at %s: %v", tc.field, err)
		})
	}
}

func TestInfusionDataOnAFeatsOptionIsRefused(t *testing.T) {
	t.Parallel()
	opt := postureOption("feature:opcao-talento"+tableSuffix, "Opção de talento")
	opt.Infusion = &TableInfusion{Accepts: InfusionAcceptsAnyObject, Effect: InfusionEffectNone}
	feat := TableFeat{
		TableEntry: TableEntry{Key: "feat:talento-de-teste" + tableSuffix, NamePT: "Talento de teste"}, DescPT: []string{"Texto."},
		Effects: []Effect{{Type: "choice", Choice: "feature", Count: 1}}, Options: []TableFeature{opt},
	}
	_, err := loadForTest(t).With(Overlay{Revision: 1, Feats: []TableFeat{feat}})
	oe, ok := errors.AsType[*OverlayError](err)
	if !ok || !hasViolation(oe, "feats[0].options[0].infusion") {
		t.Fatalf("want a violation at feats[0].options[0].infusion, got %v", err)
	}
}

// A rule on a trait's feature is refused too: a trait's options are a one-time choice of one, with no live limit.
func TestAnInfusionRuleOnATraitIsRefused(t *testing.T) {
	t.Parallel()
	trait := TableFeature{
		Key: "trait:jeito" + tableSuffix, NamePT: "Jeito", DescPT: []string{"Texto."},
		InfusionRule: &TableInfusionRule{LimitSteps: []TableInfusionLimitStep{{ClassLevel: 1, Active: 1}}},
		Options:      []TableFeature{postureOption("trait:jeito-a"+tableSuffix, "Jeito A"), postureOption("trait:jeito-b"+tableSuffix, "Jeito B")},
	}
	_, err := loadForTest(t).With(Overlay{Revision: 1, Races: []TableRace{{
		TableEntry: TableEntry{Key: "race:povo-de-teste" + tableSuffix, NamePT: "Povo de teste"}, Size: "Medium", SpeedFt: 30, Traits: []TableFeature{trait},
	}}})
	oe, ok := errors.AsType[*OverlayError](err)
	if !ok || !hasViolation(oe, "races[0].traits[0].infusion_rule") {
		t.Fatalf("want a violation at races[0].traits[0].infusion_rule, got %v", err)
	}
}

func TestAnInfusionsBonusFollowsItsSteps(t *testing.T) {
	t.Parallel()
	in := TableInfusion{BonusSteps: []TableInfusionStep{{ClassLevel: 2, Bonus: 1}, {ClassLevel: 10, Bonus: 2}}}
	cases := []struct{ level, want int }{{1, 0}, {2, 1}, {9, 1}, {10, 2}, {20, 2}}
	for _, tc := range cases {
		if got := in.BonusAt(tc.level); got != tc.want {
			t.Errorf("BonusAt(%d) = %d, want %d", tc.level, got, tc.want)
		}
	}
	if got := (TableInfusion{}).BonusAt(20); got != 0 {
		t.Errorf("BonusAt with no steps = %d, want 0", got)
	}
}

// An option of the set "table_feature" is an infusion of its feature too, when that feature has the rule.
func TestATableOptionOfAHostWithARuleCanBeAnInfusion(t *testing.T) {
	t.Parallel()
	o := infusionOverlay()
	fox := postureFoxOption()
	fox.Prerequisite = TableOptionPrerequisite{}
	fox.Infusion = &TableInfusion{Accepts: InfusionAcceptsShield, Effect: InfusionEffectItemBonus, BonusSteps: []TableInfusionStep{{ClassLevel: 2, Bonus: 1}}}
	o.Options = []TableOption{fox}
	c := withOverlay(t, o)
	if in, ok := c.Infusion(postureOwn); !ok || in.Accepts != InfusionAcceptsShield {
		t.Errorf("Infusion(table option) = %+v, %v", in, ok)
	}
	if got := c.InfusionHost(postureOwn); got != postureFeature {
		t.Errorf("InfusionHost(table option) = %q, want %q", got, postureFeature)
	}
}

func TestInfusionDataOnATableOptionIsRefusedAtItsField(t *testing.T) {
	t.Parallel()
	good := TableInfusion{Accepts: InfusionAcceptsAnyObject, Effect: InfusionEffectNone}
	cases := []struct {
		name  string
		edit  func(o *Overlay)
		field string
	}{
		{"a host with no rule", func(o *Overlay) {
			*o = postureOverlay()
			fox := postureFoxOption()
			fox.Infusion = &good
			o.Options = []TableOption{fox}
		}, "options[0].infusion"},
		{"a bad infusion", func(o *Overlay) {
			bad := good
			bad.Effect = ""
			o.Options[0].Infusion = &bad
		}, "options[0].infusion.effect"},
		{"an option of another set", func(o *Overlay) {
			o.Options[0].Set, o.Options[0].OfferedBy = "invocation", ""
			o.Options[0].Key = "option:invocation:postura-da-raposa" + tableSuffix
		}, "options[0].infusion"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			o := infusionOverlay()
			fox := postureFoxOption()
			fox.Infusion = &good
			o.Options = []TableOption{fox}
			tc.edit(&o)
			_, err := loadForTest(t).With(o)
			oe, ok := errors.AsType[*OverlayError](err)
			if !ok || !hasViolation(oe, tc.field) {
				t.Fatalf("want a violation at %s, got %v", tc.field, err)
			}
		})
	}
}

// A kind or an effect that is not on the list is refused as unknown, and an empty one as missing.
func TestAnUnknownInfusionKindIsRefusedAsUnknownNotAsRequired(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, field, want string
		edit              func(in *TableInfusion)
	}{
		{"an unknown accepts", "accepts", "is not a kind of piece", func(in *TableInfusion) { in.Accepts = "bolso" }},
		{"no accepts", "accepts", "is required", func(in *TableInfusion) { in.Accepts = "" }},
		{"an unknown effect", "effect", "is not an effect", func(in *TableInfusion) { in.Effect = "brilho" }},
		{"no effect", "effect", "is required", func(in *TableInfusion) { in.Effect = "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			o := infusionOverlay()
			tc.edit(o.Subclasses[0].Levels[0].Features[0].Options[2].Infusion)
			_, err := loadForTest(t).With(o)
			oe, ok := errors.AsType[*OverlayError](err)
			if !ok {
				t.Fatalf("want *OverlayError, got %v", err)
			}
			field := infusionPre + ".options[2].infusion." + tc.field
			for _, v := range oe.Violations() {
				if v.Field == field {
					if !strings.Contains(v.Message, tc.want) {
						t.Errorf("message at %s = %q, want it to contain %q", field, v.Message, tc.want)
					}
					return
				}
			}
			t.Errorf("no violation at %s: %v", field, err)
		})
	}
}
