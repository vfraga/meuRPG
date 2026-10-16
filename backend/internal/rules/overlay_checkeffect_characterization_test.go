package rules

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// checkEffectBuilder is an overlayBuilder in the state the second pass finds
// it: a class entry with a feature and two option lists, a resource declared in
// the entry and one in the class.
func checkEffectBuilder(srd *Content) *overlayBuilder {
	return &overlayBuilder{
		base:    srd.c,
		entries: map[string]bool{"spell:mine@mesa": true, "feat:mine@mesa": true},
		optionEntry: map[string]string{
			"feature:opt-a@mesa": "class:x@mesa", "feature:opt-b@mesa": "class:x@mesa", "feature:opt-other@mesa": "class:y@mesa",
		},
		featureEntry: map[string]string{
			"feature:owner@mesa": "class:x@mesa", "feature:opt-a@mesa": "class:x@mesa", "subclass-feature@mesa": "subclass:x@mesa",
		},
		featureClass: map[string]string{
			"feature:owner@mesa": "class:x@mesa", "subclass-feature@mesa": "class:x@mesa",
		},
		resources: map[string]map[string]bool{
			"class:x@mesa": {"mine_class": true}, "subclass:x@mesa": {"mine_sub": true},
		},
	}
}

type effectCase struct {
	owner  string
	e      Effect
	strict bool
}

func checkEffectCases() map[string]effectCase {
	const owner = "feature:owner@mesa"
	c := map[string]effectCase{}
	add := func(name string, e Effect) { c[name] = effectCase{owner: owner, e: e} }
	addStrict := func(name string, e Effect) { c[name] = effectCase{owner: owner, e: e, strict: true} }
	addAs := func(name, o string, e Effect) { c[name] = effectCase{owner: o, e: e} }

	// The menu.
	add("ok: note", Effect{Type: "note", TextPT: "x"})
	add("ok: modifier", Effect{Type: "modifier", Target: "ac", Mode: "add", Value: "1"})
	add("bad: handler", Effect{Type: "handler", Handler: "monk.martial_arts"})
	add("bad: spellcasting", Effect{Type: "spellcasting"})
	add("bad: no type", Effect{})
	add("bad: ability increase on a feature", Effect{Type: "ability_increase"})
	addAs("ok: ability increase on a feat", "feat:mine@mesa", Effect{Type: "ability_increase", From: abilities{STR}.asStrings(), Count: 1, Value: "1"})
	addAs("bad: ability increase without a list", "feat:mine@mesa", Effect{Type: "ability_increase"})
	addAs("bad: ability increase count", "feat:mine@mesa", Effect{Type: "ability_increase", From: []string{"str"}, Count: 2, Value: "1"})
	addAs("bad: ability increase value", "feat:mine@mesa", Effect{Type: "ability_increase", From: []string{"str"}, Count: 1, Value: "3"})

	// Stray fields.
	addStrict("bad: strict, stray recharge on a modifier", Effect{Type: "modifier", Target: "ac", Mode: "add", Value: "1", Recharge: "long_rest"})
	addStrict("bad: strict, stray count on a note", Effect{Type: "note", Count: 2})
	addStrict("bad: strict, stray ability", Effect{Type: "note", Ability: "str"})
	addStrict("bad: strict, stray applies", Effect{Type: "note", Applies: "damage"})
	addStrict("bad: strict, stray progression", Effect{Type: "note", Progression: "full"})
	addStrict("bad: strict, first stray field wins", Effect{Type: "note", Recharge: "long_rest", Target: "ac"})
	addStrict("ok: strict, own fields", Effect{Type: "resource", Resource: "mine_res", Max: "1", Recharge: "long_rest"})
	add("ok: lax, stray field", Effect{Type: "note", Count: 2})

	// Granted spells.
	add("ok: note with spells", Effect{Type: "note", Spells: []string{"spell:light", "spell:mine@mesa"}})
	add("bad: spells on a modifier", Effect{Type: "modifier", Spells: []string{"spell:light"}})
	add("bad: spell unknown", Effect{Type: "note", Spells: []string{"spell:light", "spell:nope"}})
	add("bad: spell of another kind", Effect{Type: "note", Spells: []string{"feat:grappler"}})

	// The choice effect.
	add("ok: choice of skills", Effect{Type: "choice", Choice: "skill", Count: 1, From: []string{"skill:arcana"}})
	add("bad: choice kind", Effect{Type: "choice", Choice: "subclass", Count: 1})
	add("bad: choice kind empty", Effect{Type: "choice", Count: 1})
	add("bad: choice count 0", Effect{Type: "choice", Choice: "skill"})
	add("bad: choice count above the limit", Effect{Type: "choice", Choice: "skill", Count: MaxChoiceCount + 1})
	add("bad: feature choice asks more than listed", Effect{Type: "choice", Choice: "feature", Count: 2, From: []string{"feature:opt-a@mesa"}})
	add("bad: feature choice lists nothing", Effect{Type: "choice", Choice: "feature", Count: 1})
	add("bad: feat choice asks more than listed", Effect{Type: "choice", Choice: "feat", Count: 2, From: []string{"feat:grappler"}})
	add("bad: feature choice lists twice", Effect{Type: "choice", Choice: "feature", Count: 1, From: []string{"feature:opt-a@mesa", "feature:opt-a@mesa"}})
	addAs("bad: an option asks for a choice", "feature:opt-a@mesa", Effect{Type: "choice", Choice: "skill", Count: 1})
	add("ok: feature choice of the entry's options", Effect{Type: "choice", Choice: "feature", Count: 2, From: []string{"feature:opt-a@mesa", "feature:opt-b@mesa"}})
	add("bad: feature choice of another entry's option", Effect{Type: "choice", Choice: "feature", Count: 1, From: []string{"feature:opt-other@mesa"}})
	add("ok: feature choice of an SRD option", Effect{Type: "choice", Choice: "feature", Count: 1, From: []string{"feature:fighter-fighting-style-defense"}})
	add("bad: feature choice of an SRD feature that is no option", Effect{Type: "choice", Choice: "feature", Count: 1, From: []string{"feature:second-wind"}})
	add("bad: feature choice of a key that does not exist", Effect{Type: "choice", Choice: "feature", Count: 1, From: []string{"feature:nope"}})
	add("bad: feature choice of a table key", Effect{Type: "choice", Choice: "feature", Count: 1, From: []string{"feature:owner@mesa"}})
	add("ok: feat choice of the SRD's and the table's", Effect{Type: "choice", Choice: "feat", Count: 2, From: []string{"feat:grappler", "feat:mine@mesa"}})
	add("bad: feat choice of a non-feat", Effect{Type: "choice", Choice: "feat", Count: 1, From: []string{"skill:arcana"}})
	add("bad: feat choice of a feat that does not exist", Effect{Type: "choice", Choice: "feat", Count: 1, From: []string{"feat:nope"}})
	add("bad: skill choice of a table key", Effect{Type: "choice", Choice: "skill", Count: 1, From: []string{"skill:mine@mesa"}})
	add("bad: skill choice of a key that does not exist", Effect{Type: "choice", Choice: "skill", Count: 1, From: []string{"skill:cooking"}})
	add("bad: second key is the bad one", Effect{Type: "choice", Choice: "skill", Count: 2, From: []string{"skill:arcana", "skill:cooking"}})

	// grant_action.
	add("ok: grant_action without a resource", Effect{Type: "grant_action", Economy: "action"})
	add("ok: grant_action of the entry's resource", Effect{Type: "grant_action", Resource: "mine_class"})
	add("ok: grant_action of an SRD resource", Effect{Type: "grant_action", Resource: "rage"})
	add("bad: grant_action of a resource nobody defines", Effect{Type: "grant_action", Resource: "ghost"})
	add("bad: grant_action of a bad resource name", Effect{Type: "grant_action", Resource: "Bad Name"})
	addAs("ok: grant_action of the subclass resource", "subclass-feature@mesa", Effect{Type: "grant_action", Resource: "mine_sub"})
	addAs("ok: grant_action of the class resource from a subclass", "subclass-feature@mesa", Effect{Type: "grant_action", Resource: "mine_class"})
	addAs("bad: grant_action of a resource of another entry", "subclass-feature@mesa", Effect{Type: "grant_action", Resource: "mine_other"})

	// superiority_die.
	sup := func(edit func(e *Effect)) Effect {
		e := Effect{Type: "superiority_die", Resource: "mine_class", Value: "1d8", Applies: ManeuverDamage}
		edit(&e)
		return e
	}
	add("ok: superiority die on damage", sup(func(_ *Effect) {}))
	add("ok: superiority die on an attack", sup(func(e *Effect) { e.Applies = ManeuverAttack }))
	add("ok: superiority die of an SRD resource", sup(func(e *Effect) { e.Resource = "rage" }))
	add("ok: reduction", sup(func(e *Effect) { e.Applies, e.Ability, e.Economy = ManeuverReduceMelee, "dex", EconomyReaction }))
	add("ok: grapple with a bonus action", sup(func(e *Effect) { e.Applies, e.Economy = ManeuverGrapple, EconomyBonusAction }))
	add("ok: grapple with nothing", sup(func(e *Effect) { e.Applies = ManeuverGrapple }))
	add("bad: superiority die of an unknown resource", sup(func(e *Effect) { e.Resource = "ghost" }))
	add("bad: superiority die without a resource", sup(func(e *Effect) { e.Resource = "" }))
	add("bad: superiority die applies", sup(func(e *Effect) { e.Applies = "heal" }))
	add("bad: superiority die applies empty", sup(func(e *Effect) { e.Applies = "" }))
	add("bad: reduction without an ability", sup(func(e *Effect) { e.Applies, e.Economy = ManeuverReduceMelee, EconomyReaction }))
	add("bad: reduction without a reaction", sup(func(e *Effect) { e.Applies, e.Ability = ManeuverReduceMelee, "dex" }))
	add("bad: reduction with a bonus action", sup(func(e *Effect) { e.Applies, e.Ability, e.Economy = ManeuverReduceMelee, "dex", EconomyBonusAction }))
	add("bad: grapple with an action", sup(func(e *Effect) { e.Applies, e.Economy = ManeuverGrapple, "action" }))
	add("bad: damage die with an economy", sup(func(e *Effect) { e.Economy = EconomyBonusAction }))
	add("bad: damage die with an ability", sup(func(e *Effect) { e.Ability = "dex" }))
	add("bad: grapple with an ability", sup(func(e *Effect) { e.Applies, e.Ability = ManeuverGrapple, "dex" }))
	add("bad: attack die with an economy and an ability", sup(func(e *Effect) { e.Applies, e.Economy, e.Ability = ManeuverAttack, EconomyReaction, "dex" }))

	// resource.
	add("ok: resource", Effect{Type: "resource", Resource: "mine_res", Max: "1", Recharge: "long_rest"})
	add("bad: resource name empty", Effect{Type: "resource", Max: "1", Recharge: "long_rest"})
	add("bad: resource name capitals", Effect{Type: "resource", Resource: "Fúria", Max: "1"})
	add("bad: resource name too long", Effect{Type: "resource", Resource: strings.Repeat("a", 41)})
	add("bad: resource of the SRD", Effect{Type: "resource", Resource: "rage", Max: "1"})

	// Other menu types that pass without a case of their own.
	add("ok: proficiency", Effect{Type: "proficiency", Proficiency: "proficiency:light-armor"})
	add("ok: sense", Effect{Type: "sense", Sense: "darkvision", RangeFt: 60})
	add("ok: extra attack", Effect{Type: "extra_attack", Count: 1})
	add("ok: ranged attack", Effect{Type: "ranged_attack", Applies: "weapon"})
	add("order: type before stray", Effect{Type: "teleport", Count: 2})
	addStrict("order: stray before spells", Effect{Type: "modifier", Count: 1, Spells: []string{"spell:light"}})
	add("order: spells before choice", Effect{Type: "choice", Choice: "skill", Spells: []string{"spell:light"}})
	add("order: choice kind before count", Effect{Type: "choice", Choice: "nope"})
	add("order: count before list", Effect{Type: "choice", Choice: "feature", Count: 0})
	add("order: more than listed before empty list", Effect{Type: "choice", Choice: "feature", Count: 1})
	return c
}

// asStrings writes abilities as the strings an effect's From holds.
type abilities []Ability

func (a abilities) asStrings() []string {
	out := make([]string, len(a))
	for i, x := range a {
		out[i] = string(x)
	}
	return out
}

func violationText(err error) string {
	if err == nil {
		return "accepted"
	}
	oe, ok := errors.AsType[*OverlayError](err)
	if !ok {
		return "not an OverlayError: " + err.Error()
	}
	var parts []string
	for _, v := range oe.Violations() {
		parts = append(parts, fmt.Sprintf("%s [field=%s reason=%s]", v.Error(), v.Field, v.Reason))
	}
	return strings.Join(parts, " || ")
}

// TestCheckEffectCharacterization freezes every refusal of checkEffect: its
// message, field and reason, and the order of the checks.
func TestCheckEffectCharacterization(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	b := checkEffectBuilder(srd)
	got := map[string]string{}
	for name, tc := range checkEffectCases() {
		e := tc.e
		err := b.checkEffect(tc.owner, "classes[0].levels[0].features[0].effects[0]", &e, tc.strict)
		got[name] = violationText(err)
		if strings.HasPrefix(name, "ok:") != (err == nil) {
			t.Errorf("%s: %v", name, err)
		}
	}
	goldenText(t, "checkeffect-refusals", got)
}
