package rules

import (
	"errors"
	"fmt"
	"testing"
)

const slotFeat = "feat:centelha-da-mesa" + tableSuffix

// slotFeatOverlay is a feat with a resource of 3 uses and the slot effects the test gives it.
func slotFeatOverlay(effects ...Effect) Overlay {
	return featWith(slotFeat, append([]Effect{resourceEffect("centelha", "3", RechargeLongRest)}, effects...)...)
}

func TestAnActionThatTurnsUsesIntoASlot(t *testing.T) {
	t.Parallel()
	d := deriveWithOverlay(t, slotFeatOverlay(Effect{
		Type: "grant_action", Economy: EconomyBonusAction, Resource: "centelha", Cost: "2", GivesSlot: "floor(level() / 2)",
	}))
	a := grantedActionOf(t, d, slotFeat)
	if a.Resource != "centelha" || a.Cost != 2 || a.GivesSlot != 1 || a.SpendsSlot {
		t.Fatalf("action = %+v, want 2 uses of centelha for a slot of level 1 (the wizard is level 3)", a)
	}
}

func TestAnActionThatSpendsASlotToHeal(t *testing.T) {
	t.Parallel()
	d := deriveWithOverlay(t, featWith(slotFeat, Effect{
		Type: "grant_action", Economy: EconomyAction, SpendsSlot: true, Heal: "1d8", ValuePerPoint: "1d8",
	}))
	a := grantedActionOf(t, d, slotFeat)
	if !a.SpendsSlot || a.Resource != "" || a.UnitCost() != 1 || a.ChoosesSpend() {
		t.Fatalf("action = %+v, want a spent slot, no resource", a)
	}
	for level, want := range map[int]DiceFormula{1: {Count: 1, Sides: 8}, 2: {Count: 2, Sides: 8}, 5: {Count: 5, Sides: 8}} {
		if got := a.Outcome(level).Heal; got == nil || *got != want {
			t.Errorf("Outcome(%d).Heal = %+v, want %+v", level, got, want)
		}
	}
}

func TestASlotActionOfAFeatureWithAResourceSpendsNoUse(t *testing.T) {
	t.Parallel()
	// The action of a feature that has a resource takes it by default: spending a slot must not.
	d := deriveWithOverlay(t, slotFeatOverlay(Effect{Type: "grant_action", Economy: EconomyAction, SpendsSlot: true, Heal: "1d8"}))
	if a := grantedActionOf(t, d, slotFeat); a.Resource != "" {
		t.Errorf("Resource = %q, want none: the action spends a slot", a.Resource)
	}
}

func TestASlotRecoveryTakesTheLimitsOfTheCharacter(t *testing.T) {
	t.Parallel()
	d := deriveWithOverlay(t, slotFeatOverlay(Effect{
		Type: "slot_recovery", Resource: "centelha", RecoverMaxLevel: "3", RecoverLevels: "ceil(level() / 2)",
	}))
	if len(d.TableRecoveries) != 1 {
		t.Fatalf("TableRecoveries = %+v, want one", d.TableRecoveries)
	}
	rec := d.TableRecoveries[0]
	if rec.Source != slotFeat || rec.Resource != "centelha" || rec.MaxLevel != 3 || rec.Levels != 2 {
		t.Fatalf("recovery = %+v, want centelha, up to level 3, combined level 2 (level 3 wizard)", rec)
	}
	for name, tc := range map[string]struct {
		slots  map[int]int
		levels int
		ok     bool
	}{
		"two slots of level 1":      {map[int]int{1: 2}, 2, true},
		"one slot of level 2":       {map[int]int{2: 1}, 2, true},
		"past the combined level":   {map[int]int{1: 1, 2: 1}, 3, false},
		"a slot above the highest":  {map[int]int{4: 1}, 0, false},
		"a count below one":         {map[int]int{1: 0}, 0, false},
		"a slot of level 3 is over": {map[int]int{3: 1}, 3, false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := rec.Check(tc.slots)
			if tc.ok != (err == nil) || (tc.ok && got != tc.levels) {
				t.Fatalf("Check(%v) = %d, %v; want %d, ok=%v", tc.slots, got, err, tc.levels, tc.ok)
			}
			if err != nil && !errors.Is(err, ErrSlotRecovery) {
				t.Errorf("error = %v, want ErrSlotRecovery", err)
			}
		})
	}
}

func TestATableSlotEffectRefusals(t *testing.T) {
	t.Parallel()
	res := resourceEffect("centelha", "3", RechargeLongRest)
	for name, tc := range map[string]struct {
		effects []Effect
		field   string
		why     string
	}{
		"gives_slot without a resource":  {[]Effect{{Type: "grant_action", Economy: EconomyAction, GivesSlot: "1"}}, ".gives_slot", ReasonValue},
		"gives_slot with a heal":         {[]Effect{res, {Type: "grant_action", Economy: EconomyAction, Resource: "centelha", GivesSlot: "1", Heal: "1d6"}}, "", ReasonValue},
		"gives_slot level 0":             {[]Effect{res, {Type: "grant_action", Economy: EconomyAction, Resource: "centelha", GivesSlot: "0"}}, ".gives_slot", ReasonValue},
		"gives_slot level 10":            {[]Effect{res, {Type: "grant_action", Economy: EconomyAction, Resource: "centelha", GivesSlot: "10"}}, ".gives_slot", ReasonValue},
		"gives_slot and spends_slot":     {[]Effect{res, {Type: "grant_action", Economy: EconomyAction, Resource: "centelha", GivesSlot: "1", SpendsSlot: true, Heal: "1d6"}}, "", ReasonValue},
		"spends_slot with a resource":    {[]Effect{res, {Type: "grant_action", Economy: EconomyAction, Resource: "centelha", SpendsSlot: true, Heal: "1d6"}}, "", ReasonValue},
		"spends_slot without a heal":     {[]Effect{{Type: "grant_action", Economy: EconomyAction, SpendsSlot: true}}, "", ReasonValue},
		"spends_slot with temp_hp":       {[]Effect{{Type: "grant_action", Economy: EconomyAction, SpendsSlot: true, Heal: "1d6", TempHP: "3"}}, "", ReasonValue},
		"gives_slot on a note":           {[]Effect{{Type: "note", GivesSlot: "1"}}, "", ReasonValue},
		"recovery without a limit":       {[]Effect{res, {Type: "slot_recovery", Resource: "centelha", RecoverMaxLevel: "3"}}, "", ReasonValue},
		"recovery of a missing resource": {[]Effect{{Type: "slot_recovery", Resource: "nada", RecoverMaxLevel: "3", RecoverLevels: "2"}}, ".resource", ReasonReference},
		"recovery above level 9":         {[]Effect{res, {Type: "slot_recovery", Resource: "centelha", RecoverMaxLevel: "10", RecoverLevels: "2"}}, ".recover_max_level", ReasonValue},
		"recovery of no levels":          {[]Effect{res, {Type: "slot_recovery", Resource: "centelha", RecoverMaxLevel: "3", RecoverLevels: "0"}}, "", ReasonValue},
		"recovery with another field":    {[]Effect{res, {Type: "slot_recovery", Resource: "centelha", RecoverMaxLevel: "3", RecoverLevels: "2", Cost: "1"}}, "", ReasonValue},
		"a second recovery in a feature": {[]Effect{res, {Type: "slot_recovery", Resource: "centelha", RecoverMaxLevel: "1", RecoverLevels: "1"}, {Type: "slot_recovery", Resource: "centelha", RecoverMaxLevel: "3", RecoverLevels: "2"}}, ".type", ReasonEffect},
		"recover_levels on a note":       {[]Effect{{Type: "note", RecoverLevels: "2"}}, "", ReasonValue},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := loadForTest(t).With(featWith(slotFeat, tc.effects...))
			if tc.field == "" {
				if _, ok := errors.AsType[*OverlayError](err); !ok {
					t.Fatalf("With() error = %v, want an OverlayError", err)
				}
				return
			}
			violationAt(t, err, fmt.Sprintf("feats[0].effects[%d]%s", len(tc.effects)-1, tc.field), tc.why)
		})
	}
}

func TestAGivenSlotLevelOutsideOneToNineGivesNoAction(t *testing.T) {
	t.Parallel()
	// The wizard is level 3: level() * 4 is 12, a level no slot has.
	d := deriveWithOverlay(t, slotFeatOverlay(Effect{
		Type: "grant_action", Economy: EconomyBonusAction, Resource: "centelha", GivesSlot: "level() * 4",
	}))
	for _, a := range d.Actions {
		if a.Source == slotFeat {
			t.Errorf("action = %+v, want none: the slot level is 12", a)
		}
	}
}
