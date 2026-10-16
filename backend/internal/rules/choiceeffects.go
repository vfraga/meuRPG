package rules

import (
	"fmt"
	"slices"
	"strings"
)

// What the picks of the choices give the sheet (choicegroups.go says what is
// asked; this says what it does): the abilities a race lets the player raise, the
// dragonborn's breath weapon and resistance, the resistances some traits give, and
// the spells a feature grants.

// BreathWeapon is the dragonborn's Breath Weapon (SRD 5.1, Dragonborn): the dragon
// ancestry decides the damage type, the area and the saving throw.
type BreathWeapon struct {
	// AncestryKey is the trait option the player picked ("trait:draconic-ancestry-red").
	AncestryKey string
	// DamageType is a damage type key, and DamageTypePT its name.
	DamageType, DamageTypePT string
	// Dice is the damage at the character's level, such as "2d6".
	Dice string
	// Shape is BreathLine or BreathCone; SizeFt the length of the line or of the cone's
	// side, and WidthFt the width of a line (0 for a cone).
	Shape           string
	SizeFt, WidthFt int
	// SaveAbility is the ability of the saving throw, and DC its difficulty:
	// 8 + the Constitution modifier + the proficiency bonus.
	SaveAbility Ability
	DC          int
	// Text is the whole rule in one paragraph, in Portuguese, with these numbers.
	Text string
}

// ChoiceResistance is a damage type the character resists because of a choice (the
// dragonborn's ancestry), and what gives it, for the sheet. Combat applies resistances
// through Content.Resistances.
type ChoiceResistance struct {
	DamageType, DamageTypePT string
	// SourceKey is the trait that gives it.
	SourceKey string
}

// pickedAbilities are the abilities the player picked for a race that lets them
// raise some (the half-elf's two +1), in the order of the race's list.
func (x *deriver) pickedAbilities() []Ability {
	r := x.race
	if r == nil || r.AbilityBonusChoices == nil || len(x.c.raceChoice[r.Key]) > 0 {
		return nil
	}
	key := AbilityChoiceKey(r.Key)
	var out []Ability
	for _, k := range x.b.FeatureChoices {
		choiceKey, value, ok := SplitScopedChoice(k)
		if !ok || choiceKey != key || !strings.HasPrefix(value, abilityPrefix) {
			continue
		}
		a, ok := ability(strings.TrimPrefix(value, abilityPrefix))
		if ok && slices.Contains(r.AbilityBonusChoices.From, string(a)) && !slices.Contains(out, a) && len(out) < r.AbilityBonusChoices.Choose {
			out = append(out, a)
		}
	}
	return out
}

// choiceEffects adds what the picks give that no effect says: the breath weapon and
// the resistances, and the reminder of the sorcerer's Elemental Affinity.
func (x *deriver) choiceEffects() {
	c := x.c
	if c.choices == nil {
		return
	}
	owned := map[string]bool{}
	for _, f := range x.d.Features {
		owned[f.Key] = true
	}
	x.choiceResistances(owned)
	if owned["feature:elemental-affinity"] {
		x.elementalAffinityHints(owned)
	}
}

// choiceResistances adds the breath weapon and the resistance of the picked
// draconic ancestry, and the resistances of the traits that give one, each
// damage type once.
func (x *deriver) choiceResistances(owned map[string]bool) {
	c := x.c
	var resisted []string
	addResistance := func(damageType, source string) {
		if slices.Contains(resisted, damageType) {
			return
		}
		resisted = append(resisted, damageType)
		x.d.Resistances = append(x.d.Resistances, ChoiceResistance{DamageType: damageType, DamageTypePT: c.namePT(damageType), SourceKey: source})
	}
	for _, k := range x.b.FeatureChoices {
		a, ok := c.choices.ancestryByTrait[k]
		if !ok || !owned[k] {
			continue
		}
		if owned[c.choices.breathTrait] {
			x.d.BreathWeapon = x.breathWeapon(a)
			x.d.BreathWeapon.Text = c.breathWeaponText(*x.d)
		}
		addResistance(a.DamageType, a.Trait)
	}
	for _, f := range x.d.Features {
		if r, ok := c.choices.resistances[f.Key]; ok {
			addResistance(r.DamageType, r.Trait)
		}
	}
}

// elementalAffinityHints reminds a sorcerer with Elemental Affinity of what it
// does for each dragon ancestor picked.
func (x *deriver) elementalAffinityHints(owned map[string]bool) {
	c := x.c
	for _, k := range x.b.FeatureChoices {
		if a, ok := c.choices.ancestryByFeature[k]; ok && owned[k] {
			kind := c.namePT(a.DamageType)
			x.d.Hints = append(x.d.Hints, Hint{
				Source: "feature:elemental-affinity", Mode: "note",
				TextPT: fmt.Sprintf("Afinidade Elemental: soma o Carisma ao dano de %s das suas magias e, por 1 ponto de feitiçaria, dá resistência a dano %s por 1 hora.", kind, kind),
			})
		}
	}
}

// breathWeapon computes the breath weapon of an ancestry at the character's level.
func (x *deriver) breathWeapon(a *draconicAncestry) *BreathWeapon {
	c := x.c
	dice := ""
	for _, row := range c.choices.breathDice {
		if x.d.TotalLevel >= row.Level {
			dice = row.Dice
		}
	}
	return &BreathWeapon{
		AncestryKey: a.Trait, DamageType: a.DamageType, DamageTypePT: c.namePT(a.DamageType), Dice: dice,
		Shape: a.Shape, SizeFt: a.SizeFt, WidthFt: a.WidthFt,
		SaveAbility: Ability(a.Save), DC: 8 + x.mods[CON] + x.prof,
	}
}

// BreathWeaponText says the breath weapon in one paragraph, with the numbers of
// the sheet it was computed for.
func (c *Content) BreathWeaponText(d Derived) string { return c.c.breathWeaponText(d) }

func (c *content) breathWeaponText(d Derived) string {
	bw := d.BreathWeapon
	if bw == nil || c.choices == nil || c.choices.ancestryByTrait[bw.AncestryKey] == nil {
		return ""
	}
	save := c.namePT(string(bw.SaveAbility))
	conScore := 0
	for _, ab := range d.Abilities {
		if ab.Ability == CON {
			conScore = ab.Score
		}
	}
	var growth []string
	for _, row := range c.choices.breathDice {
		if row.Level > d.TotalLevel {
			growth = append(growth, fmt.Sprintf("%s no nível %d", row.Dice, row.Level))
		}
	}
	text := fmt.Sprintf("Arma de Sopro: %s de %s num %s; teste de resistência de %s, CD 8 + Constituição + proficiência (CD %d com Constituição %d e proficiência %s); metade do dano em caso de sucesso; 1 uso por descanso curto ou longo.",
		bw.Dice, bw.DamageTypePT, breathAreaPT(bw), save, bw.DC, conScore, signed(d.ProficiencyBonus))
	if len(growth) > 0 {
		text += " O dano vira " + joinList(growth) + "."
	}
	return text + fmt.Sprintf(" Resistência a dano de %s.", bw.DamageTypePT)
}

func breathAreaPT(bw *BreathWeapon) string {
	if bw.Shape == BreathLine {
		return fmt.Sprintf("linha de %s por %s", metersPT(bw.SizeFt), metersPT(bw.WidthFt))
	}
	return "cone de " + metersPT(bw.SizeFt)
}

// joinList joins "a", "b" and "c" as "a, b e c".
func joinList(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	}
	return strings.Join(items[:len(items)-1], ", ") + " e " + items[len(items)-1]
}

// results writes what the picks give into the choices that show it: the breath
// weapon under the dragonborn's ancestry, the sorcerer's draconic text and the
// half-elf's new scores.
func (cb *choiceBuilder) results(set *ChoiceSet) {
	c := cb.c
	if c.choices == nil {
		return
	}
	for gi := range set.Groups {
		for ci := range set.Groups[gi].Choices {
			ch := &set.Groups[gi].Choices[ci]
			switch {
			case ch.FeatureKey == "trait:draconic-ancestry" && len(ch.Picked) > 0:
				d := cb.derive()
				ch.ResultPT = c.breathWeaponText(*d)
			case ch.FeatureKey == "feature:dragon-ancestor" && len(ch.Picked) > 0:
				if a := c.choices.ancestryByFeature[ch.Picked[0]]; a != nil {
					kind := c.namePT(a.DamageType)
					ch.ResultPT = fmt.Sprintf("Você fala, lê e escreve Dracônico, e dobra a proficiência em testes de Carisma ao lidar com dragões. No nível 6, a Afinidade Elemental soma o Carisma ao dano de %s e dá resistência a dano %s por 1 hora (1 ponto de feitiçaria).", kind, kind)
				}
			case ch.Kind == ChoiceKindAbilities && len(ch.Picked) > 0:
				ch.ResultPT = cb.abilityResult(ch)
			}
		}
	}
}

// abilityResult says the scores the half-elf's picks changed: "Destreza 16 → 17 e
// Sabedoria 14 → 15".
func (cb *choiceBuilder) abilityResult(ch *Choice) string {
	d := cb.derive()
	var parts []string
	for _, k := range ch.Picked {
		a := Ability(strings.TrimPrefix(k, abilityPrefix))
		for _, ab := range d.Abilities {
			if ab.Ability == a {
				parts = append(parts, fmt.Sprintf("%s %d → %d", cb.c.namePT(string(a)), ab.Score-1, ab.Score))
			}
		}
	}
	return joinList(parts) + "."
}
