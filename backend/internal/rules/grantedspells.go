package rules

import (
	"cmp"
	"slices"
	"strings"

	"github.com/PuraFome/meuRPG/backend/internal/rules/formula"
)

// The granted_spell effect: a spell an entry (a race trait, a feat, a feature)
// gives the character, apart from the lists of its classes. Every name in the
// tests is invented.

const (
	// GrantedAtWill is the Uses of a granted spell with no limit; an empty Uses means the same.
	GrantedAtWill = "at_will"
	// GrantedAbilityChoice is the Ability of a grant whose casting ability the
	// player picks once, when the entry is taken.
	GrantedAbilityChoice = "choice"
	// grantedAbilityChoiceSuffix ends the choice key of that pick ("<entry>#grant-ability").
	grantedAbilityChoiceSuffix = textSeparator + "grant-ability"
)

// GrantedAbilities are the casting abilities a grant names.
var GrantedAbilities = []Ability{INT, WIS, CHA}

// MaxGrantUsesCost is the most uses one cast of a granted spell can cost on the menu.
const MaxGrantUsesCost = 20

// GrantedAbilityChoiceKey is the key of the choice of the casting ability of the
// grants an entry (a trait, a feature or a feat) makes, shared by all of them.
func GrantedAbilityChoiceKey(owner string) string { return owner + grantedAbilityChoiceSuffix }

// GrantedSpell is a spell the character has from an entry, as the sheet lists it
// and the cast flow reads it.
type GrantedSpell struct {
	Spell SpellEntry
	// Source is the key of the feature, trait or feat that grants it, and SourcePT its name.
	Source, SourcePT string
	// AtWill says there is no limit; otherwise Resource is the resource every cast spends one of.
	AtWill   bool
	Resource string
	// AlsoWithSlot, RitualOnly and CountsAsKnown are the effect's switches.
	AlsoWithSlot, RitualOnly, CountsAsKnown bool
	// NoMaterialComponents says the grant waives the material components (display only), and
	// Cost the uses of Resource every cast spends (1 or more; 0 for an at-will grant).
	NoMaterialComponents bool
	Cost                 int
	// Ability is the casting ability; empty while a grant with a choice has none picked
	// (AbilityPending), and then AttackBonus and SaveDC are 0.
	Ability        Ability
	AbilityPending bool
	AttackBonus    int
	SaveDC         int
	// FromLevel is the level the grant starts at.
	FromLevel int
	// FeatureSummon says the spell is the summon of a table feature: not a spell the character casts, so the turn's
	// spell limit does not apply to it.
	FeatureSummon bool
}

// grantedSpell checks a granted_spell effect against the content.
func (x effectCompiler) grantedSpell() error {
	e := x.e
	sp, ok := x.c.spells[e.Spell]
	if !ok {
		return x.fail("the granted spell %q is not in the content", e.Spell)
	}
	if e.Uses != "" && e.Uses != GrantedAtWill && !validResourceName(e.Uses) {
		return x.fail("uses must be %q or the name of a resource", GrantedAtWill)
	}
	if e.Ability != GrantedAbilityChoice && !slices.Contains(grantedAbilityKeys(), e.Ability) {
		return x.fail("ability must be int, wis, cha or %q", GrantedAbilityChoice)
	}
	if e.RitualOnly && !sp.Ritual {
		return x.fail("ritual_only needs a spell with the ritual tag: %q has none", e.Spell)
	}
	if e.RitualOnly && e.AlsoWithSlot {
		return x.fail("ritual_only and also_with_slot exclude each other")
	}
	if err := x.grantedCost(e); err != nil {
		return err
	}
	var err error
	e.fromLevel, err = x.compile(e.FromLevel, formula.Int)
	return err
}

// grantedCost checks the uses a cast of the grant spends: 1 to MaxGrantUsesCost, and only with a resource.
func (x *effectCompiler) grantedCost(e *Effect) error {
	switch {
	case e.UsesCost < 0:
		return x.fail("uses_cost: at least 1 use")
	case e.UsesCost > MaxGrantUsesCost:
		return x.fail("uses_cost: at most %d uses", MaxGrantUsesCost)
	case e.UsesCost != 0 && (e.Uses == "" || e.Uses == GrantedAtWill):
		return x.fail("uses_cost: a resource in uses is needed")
	}
	return nil
}

func grantedAbilityKeys() []string {
	out := make([]string, len(GrantedAbilities))
	for i, a := range GrantedAbilities {
		out[i] = string(a)
	}
	return out
}

// grantedSpells fills Derived.GrantedSpells: the granted_spell effects that apply
// at the character's level, with the casting ability each one uses. A spell the
// effect counts as known also goes on the list of spells, without counting against
// any class's numbers.
func (x *deriver) grantedSpells() {
	for _, a := range x.active {
		if g, ok := x.grantedSpellOf(a); ok {
			x.d.GrantedSpells = append(x.d.GrantedSpells, g)
		}
	}
	slices.SortStableFunc(x.d.GrantedSpells, func(a, b GrantedSpell) int {
		return cmp.Or(cmp.Compare(a.Spell.Level, b.Spell.Level), comparePT(a.Spell.NamePT, b.Spell.NamePT))
	})
}

// grantedSpellOf builds the grant of one active effect. ok is false when the effect is not a
// granted_spell, does not apply yet, or names a spell the content does not have.
func (x *deriver) grantedSpellOf(a activeEffect) (GrantedSpell, bool) {
	e := a.effect
	if e.Type != "granted_spell" || !x.applies(a) {
		return GrantedSpell{}, false
	}
	from := 1
	if e.fromLevel != nil {
		n, err := e.fromLevel.Int(x.env)
		if err != nil {
			x.issue(IssueFormula, "", "Um efeito de %s foi ignorado: a fórmula do nível falhou.", x.c.namePT(a.owner))
			return GrantedSpell{}, false
		}
		from = n
	}
	if x.grantLevel(a.owner) < from {
		return GrantedSpell{}, false
	}
	entry, ok := x.c.spellEntries[e.Spell]
	if !ok {
		return GrantedSpell{}, false
	}
	g := GrantedSpell{
		Spell: entry, Source: a.owner, SourcePT: x.c.namePT(a.owner), AtWill: e.Uses == "" || e.Uses == GrantedAtWill,
		AlsoWithSlot: e.AlsoWithSlot, RitualOnly: e.RitualOnly, CountsAsKnown: e.CountsAsKnown, FromLevel: from,
	}
	g.NoMaterialComponents = e.NoMaterialComponents
	if !g.AtWill {
		g.Resource, g.Cost = e.Uses, max(e.UsesCost, 1)
	}
	_, g.FeatureSummon = x.c.featureSummons[e.Spell]
	x.grantAbility(&g, e)
	return g, true
}

// grantLevel is the level a grant is gated on: the level of the class a class or
// subclass feature belongs to, the character level for anything else.
func (x *deriver) grantLevel(owner string) int {
	for _, f := range x.d.Features {
		if f.Key != owner {
			continue
		}
		class := f.Source
		if sub, ok := x.c.subclasses[f.Source]; ok {
			class = sub.Class
		}
		for _, oc := range x.classes {
			if oc.key == class {
				return oc.level
			}
		}
	}
	return x.d.TotalLevel
}

// grantAbility sets the casting ability of a grant and the numbers that follow from it.
func (x *deriver) grantAbility(g *GrantedSpell, e *Effect) {
	ab := Ability(e.Ability)
	if e.Ability == GrantedAbilityChoice {
		ab = x.pickedGrantAbility(g.Source)
		if ab == "" {
			g.AbilityPending = true
			return
		}
	}
	g.Ability = ab
	g.AttackBonus = x.prof + x.mods[ab]
	g.SaveDC = 8 + x.prof + x.mods[ab]
}

// pickedGrantAbility is the ability the player picked for the grants of an entry,
// or "" when there is none (or one the grants do not allow).
func (x *deriver) pickedGrantAbility(owner string) Ability {
	key := GrantedAbilityChoiceKey(owner)
	for _, k := range x.b.FeatureChoices {
		choiceKey, value, ok := SplitScopedChoice(k)
		if !ok || choiceKey != key || !strings.HasPrefix(value, abilityPrefix) {
			continue
		}
		if a, ok := ability(strings.TrimPrefix(value, abilityPrefix)); ok && slices.Contains(GrantedAbilities, a) {
			return a
		}
	}
	return ""
}

// grantAbilityChoice adds the pick of the casting ability of the granted spells an
// entry gives with ability "choice": one pick for the entry, whatever the number of
// spells it grants.
func (cb *choiceBuilder) grantAbilityChoice(g *groupBuild, owner string) {
	c := cb.c
	if !slices.ContainsFunc(c.effects[owner], func(e *Effect) bool {
		return e.Type == "granted_spell" && e.Ability == GrantedAbilityChoice
	}) {
		return
	}
	key := GrantedAbilityChoiceKey(owner)
	ch := cb.addChoice(g, &Choice{
		Key: key, FeatureKey: owner, Kind: ChoiceKindOptions, TitlePT: "Habilidade de conjuração", Picks: 1,
		HintPT: "A habilidade que as magias concedidas por " + c.namePT(owner) + " usam para o ataque e a CD de resistência.",
	})
	for _, a := range GrantedAbilities {
		ch.Options = append(ch.Options, ChoiceOption{
			Key: abilityValue(a), Stored: ScopedChoice(key, abilityValue(a)), NamePT: c.namePT(string(a)),
		})
	}
	cb.takeScoped(ch)
}
