package combat

import (
	"slices"
	"testing"

	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The spells a granted_spell effect gives, as options of the turn and outside a fight.
// Every name is invented; the spells are the SRD's.

func grantedSpell(key string, level int, edit ...func(*rules.GrantedSpell)) rules.GrantedSpell {
	g := rules.GrantedSpell{
		Spell:  rules.SpellEntry{Key: key, Name: key, NamePT: key, Level: level, CastingTime: rules.CastingTime{Unit: rules.CastAction, Amount: 1}},
		Source: "feat:graca@mesa", SourcePT: "Graça", AtWill: true, Ability: rules.CHA, AttackBonus: 5, SaveDC: 13,
	}
	for _, f := range edit {
		f(&g)
	}
	return g
}

func limited(g *rules.GrantedSpell) { g.AtWill, g.Resource = false, "usos_da_graca" }

func spellOptionFor(t *testing.T, o TurnOptions, key string) SpellOption {
	t.Helper()
	for _, s := range o.Spells {
		if s.Spell.Key == key {
			return s
		}
	}
	t.Fatalf("no option for %s in %d spells", key, len(o.Spells))
	return SpellOption{}
}

func TestAGrantedSpellIsCastWithNoSlot(t *testing.T) {
	t.Parallel()
	d := rules.Derived{
		GrantedSpells: []rules.GrantedSpell{grantedSpell("spell:misty-step", 2, limited), grantedSpell("spell:light", 0)},
		Resources:     []rules.Resource{{Key: "usos_da_graca", Max: 1, Recharge: rules.RechargeLongRest}},
	}
	got := Options(d, TurnState{}, Usage{})
	step := spellOptionFor(t, got, "spell:misty-step")
	if !step.Enabled || len(step.Slots) != 0 || step.Grant == nil || step.Grant.UsesLeft != 1 || step.Grant.Resource != "usos_da_graca" {
		t.Errorf("misty step = %+v, want enabled with no slots and one use left", step)
	}
	if step.Grant.Ability != rules.CHA || step.Grant.SaveDC != 13 || step.Grant.AttackBonus != 5 {
		t.Errorf("misty step grant = %+v, want the grant's own ability and numbers", step.Grant)
	}
	if light := spellOptionFor(t, got, "spell:light"); !light.Enabled || !light.Grant.AtWill {
		t.Errorf("light = %+v, want an at-will option", light)
	}

	// The use is spent: the limited grant says so, the at-will one stays.
	spent := Usage{ResourcesUsed: map[string]int{"usos_da_graca": 1}}
	again := Options(d, TurnState{}, spent)
	step = spellOptionFor(t, again, "spell:misty-step")
	if step.Enabled || step.Reason == nil || step.Reason.Code != ReasonNoUses || step.Reason.Recharge != rules.RechargeLongRest {
		t.Errorf("misty step with no use left = %+v, want NO_USES that recharges on a long rest", step)
	}
	if !spellOptionFor(t, again, "spell:light").Enabled {
		t.Error("an at-will grant is disabled")
	}
}

func TestAGrantedSpellOffersSlotsOnlyWhenItAllowsThem(t *testing.T) {
	t.Parallel()
	d := rules.Derived{
		Spellcasting: []rules.Spellcasting{{Class: "class:wizard"}},
		SpellSlots:   []int{2, 1, 0, 0, 0, 0, 0, 0, 0},
		GrantedSpells: []rules.GrantedSpell{
			grantedSpell("spell:misty-step", 2, limited, func(g *rules.GrantedSpell) { g.AlsoWithSlot = true }),
			grantedSpell("spell:hold-person", 2, limited),
		},
		Resources: []rules.Resource{{Key: "usos_da_graca", Max: 1, Recharge: rules.RechargeLongRest}},
	}
	spent := Usage{ResourcesUsed: map[string]int{"usos_da_graca": 1}}
	got := Options(d, TurnState{}, spent)
	if s := spellOptionFor(t, got, "spell:misty-step"); !s.Enabled || len(s.Slots) != 1 || s.Slots[0].Level != 2 {
		t.Errorf("misty step with a slot allowed = %+v, want enabled through the 2nd-level slot", s)
	}
	if s := spellOptionFor(t, got, "spell:hold-person"); s.Enabled || len(s.Slots) != 0 {
		t.Errorf("hold person with no slot allowed = %+v, want disabled and no slots", s)
	}
}

func TestAGrantedSpellWithoutItsAbilityOrOnlyForRitualsIsNotATurnOption(t *testing.T) {
	t.Parallel()
	d := rules.Derived{GrantedSpells: []rules.GrantedSpell{
		grantedSpell("spell:light", 0, func(g *rules.GrantedSpell) { g.Ability, g.AbilityPending, g.SaveDC = "", true, 0 }),
		grantedSpell("spell:detect-magic", 1, func(g *rules.GrantedSpell) { g.RitualOnly = true }),
	}}
	if got := Options(d, TurnState{}, Usage{}); len(got.Spells) != 0 {
		t.Errorf("spells = %+v, want none", got.Spells)
	}
}

func TestAGrantedSpellStillObeysTheTurn(t *testing.T) {
	t.Parallel()
	d := rules.Derived{GrantedSpells: []rules.GrantedSpell{grantedSpell("spell:light", 0)}}
	got := Options(d, TurnState{ActionUsed: true}, Usage{})
	if s := spellOptionFor(t, got, "spell:light"); s.Enabled || s.Reason == nil || s.Reason.Code != ReasonActionUsed {
		t.Errorf("light after the action = %+v, want ACTION_USED", s)
	}
}

func TestGrantedSpellsOutsideAFight(t *testing.T) {
	t.Parallel()
	d := rules.Derived{
		Spellcasting: []rules.Spellcasting{{Class: "class:wizard", PreparesSpells: true}},
		SpellSlots:   []int{2, 0, 0, 0, 0, 0, 0, 0, 0},
		Spells:       []rules.CharacterSpell{castSpell("spell:magic-missile", 1, false, rules.CastAction, 1)},
		GrantedSpells: []rules.GrantedSpell{
			grantedSpell("spell:magic-missile", 1, limited),
			grantedSpell("spell:misty-step", 2, limited),
			grantedSpell("spell:detect-magic", 1, func(g *rules.GrantedSpell) { g.RitualOnly = true }),
		},
		Resources: []rules.Resource{{Key: "usos_da_graca", Max: 1, Recharge: rules.RechargeLongRest}},
	}
	got := OutsideSpells(d, Usage{})
	keys := make([]string, len(got))
	for i, o := range got {
		keys[i] = o.Spell.Key
	}
	if want := []string{"spell:detect-magic", "spell:magic-missile", "spell:misty-step"}; !slices.Equal(keys, want) {
		t.Fatalf("spells = %v, want %v (a granted spell once, even when the class has it)", keys, want)
	}
	// On the class's list and granted: both ways.
	if mm := got[1]; !mm.CanCast || len(mm.Grants) != 1 || mm.Grants[0].Source != "feat:graca@mesa" {
		t.Errorf("magic missile = %+v, want the class's way and the grant", mm)
	}
	if ms := got[2]; ms.CanCast || len(ms.Grants) != 1 || !ms.Enabled() || ms.Reason != nil {
		t.Errorf("misty step = %+v, want only the grant, enabled", ms)
	}
	if dm := got[0]; !dm.Grants[0].RitualOnly || !dm.Enabled() {
		t.Errorf("detect magic = %+v, want a ritual-only grant, enabled", dm)
	}

	// No use left: the grant-only spell says why.
	spent := OutsideSpells(d, Usage{ResourcesUsed: map[string]int{"usos_da_graca": 1}})
	if ms := spent[2]; ms.Enabled() || ms.Reason == nil || ms.Reason.Code != ReasonNoUses {
		t.Errorf("misty step with no use left = %+v, want NO_USES", ms)
	}
}

// The summon of a table feature is no spell, so the turn's spell limit (SRD 5.1, Casting Time) neither stops it after a
// bonus action spell nor stops a bonus action spell after it.
func TestAFeatureSummonIsOutsideTheTurnsSpellLimit(t *testing.T) {
	t.Parallel()
	summon := grantedSpell("spell:feature-chamado@mesa", 1, limited, func(g *rules.GrantedSpell) { g.FeatureSummon = true })
	bonusSummon := grantedSpell("spell:feature-grito@mesa", 1, limited, func(g *rules.GrantedSpell) {
		g.FeatureSummon = true
		g.Spell.CastingTime.Unit = rules.CastBonusAction
	})
	plain := grantedSpell("spell:shield-of-faith", 1, limited)
	bonus := grantedSpell("spell:healing-word", 1, limited, func(g *rules.GrantedSpell) { g.Spell.CastingTime.Unit = rules.CastBonusAction })
	d := rules.Derived{
		GrantedSpells: []rules.GrantedSpell{summon, bonusSummon, plain, bonus},
		Resources:     []rules.Resource{{Key: "usos_da_graca", Max: 1, Recharge: rules.RechargeLongRest}},
	}

	afterBonusSpell := Options(d, TurnState{BonusSpellCast: true}, Usage{})
	if s := spellOptionFor(t, afterBonusSpell, summon.Spell.Key); !s.Enabled {
		t.Errorf("the feature's summon after a bonus action spell = %+v, want it enabled", s)
	}
	if s := spellOptionFor(t, afterBonusSpell, plain.Spell.Key); s.Enabled || s.Reason == nil || s.Reason.Code != ReasonBonusActionSpellLimit {
		t.Errorf("a spell after a bonus action spell (the control) = %+v, want BONUS_ACTION_SPELL_LIMIT", s)
	}
	afterSpell := Options(d, TurnState{SpellCast: true}, Usage{})
	if s := spellOptionFor(t, afterSpell, bonusSummon.Spell.Key); !s.Enabled {
		t.Errorf("a feature's summon cast with a bonus action after a spell = %+v, want it enabled", s)
	}
	if s := spellOptionFor(t, afterSpell, bonus.Spell.Key); s.Enabled || s.Reason == nil || s.Reason.Code != ReasonBonusActionSpellLimit {
		t.Errorf("a bonus action spell after a spell (the control) = %+v, want BONUS_ACTION_SPELL_LIMIT", s)
	}
}

// A grant that costs more than one use needs that many left, in a fight and outside one.
func TestAGrantedSpellNeedsItsWholeCost(t *testing.T) {
	t.Parallel()
	costly := func(g *rules.GrantedSpell) { limited(g); g.Cost, g.NoMaterialComponents = 2, true }
	d := rules.Derived{
		GrantedSpells: []rules.GrantedSpell{grantedSpell("spell:misty-step", 2, costly)},
		Resources:     []rules.Resource{{Key: "usos_da_graca", Max: 3, Recharge: rules.RechargeLongRest}},
	}
	for _, c := range []struct {
		used    int
		enabled bool
	}{{0, true}, {1, true}, {2, false}, {3, false}} {
		u := Usage{ResourcesUsed: map[string]int{"usos_da_graca": c.used}}
		step := spellOptionFor(t, Options(d, TurnState{}, u), "spell:misty-step")
		if step.Enabled != c.enabled || step.Grant.Cost != 2 || !step.Grant.NoMaterialComponents {
			t.Errorf("%d used: option = %+v, want enabled %v, cost 2, no material", c.used, step, c.enabled)
		}
		if out := OutsideSpells(d, u); out[0].Enabled() != c.enabled || out[0].Grants[0].Ready() != c.enabled {
			t.Errorf("%d used: outside = %+v, want enabled %v", c.used, out[0], c.enabled)
		}
	}
}
