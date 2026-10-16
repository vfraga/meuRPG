package rules

import (
	"fmt"
	"slices"

	"github.com/PuraFome/meuRPG/backend/internal/rules/srd51"
)

// Wild Shape (MR-037, Etapa 9). The druid's Wild Shape features carry a
// wild_shape effect (effects/druid.json): level 2 takes beasts of challenge
// rating 1/4 or lower with no fly or swim speed, level 4 up to 1/2 with no fly
// speed, level 8 up to 1. The functions here are pure: the play module keeps the
// form and the beast's hit points, and calls WildShapeDerived for the sheet.

// WildShapeLimit is what a druid's Wild Shape allows now.
type WildShapeLimit struct {
	// MaxCR is the highest challenge rating ("1/4"), and NoFly and NoSwim leave
	// out the beasts with a fly or a swim speed.
	MaxCR         string
	NoFly, NoSwim bool
}

// WildShapeLimitFor returns what the build's Wild Shape features allow, and
// false when the build has none (not a druid of level 2). The most permissive
// feature wins: the one with the highest challenge rating.
func (c *Content) WildShapeLimitFor(b Build) (WildShapeLimit, bool) {
	var best *Effect
	bestCR := -1
	for _, f := range Derive(b, c).Features {
		for _, e := range c.c.effects[f.Key] {
			if e.Type != "wild_shape" {
				continue
			}
			if v, _ := crEighths(e.MaxCR); v > bestCR {
				best, bestCR = e, v
			}
		}
	}
	if best == nil {
		return WildShapeLimit{}, false
	}
	return WildShapeLimit{MaxCR: best.MaxCR, NoFly: best.NoFly, NoSwim: best.NoSwim}, true
}

// WildShapeForms lists the beasts the build's druid may take now, sorted by
// Portuguese name. It is empty for a build without Wild Shape.
func (c *Content) WildShapeForms(b Build) []CreatureEntry {
	limit, ok := c.WildShapeLimitFor(b)
	if !ok {
		return nil
	}
	forms, _ := c.ListCreatures(CreatureFilter{Type: creatureBeast, MaxCR: limit.MaxCR, NoFly: limit.NoFly, NoSwim: limit.NoSwim})
	return forms
}

// CastsInBeastForm says whether the character keeps its spells in a beast form:
// from druid level 18 (Beast Spells) it casts druid spells in any shape, with no
// material components. d is the character's own sheet.
func (c *Content) CastsInBeastForm(d Derived) bool {
	for _, f := range d.Features {
		for _, e := range c.c.effects[f.Key] {
			if e.Type == "beast_spells" {
				return true
			}
		}
	}
	return false
}

// WildShapeAllows says whether the build's druid may take the beast now.
func (c *Content) WildShapeAllows(b Build, beast string) bool {
	return slices.ContainsFunc(c.WildShapeForms(b), func(e CreatureEntry) bool { return e.Key == beast })
}

// WildShapeDerived is the sheet of a character in beast form, by the SRD: the
// beast's armor class, hit points and Hit Dice (a pool of their own: HitPointsMax is the
// beast's), speeds, Strength, Dexterity and Constitution, attacks, Multiattack
// count and actions with a saving throw. The character keeps Intelligence,
// Wisdom and Charisma, the proficiency bonus, the features, the resources and the
// skill and saving throw proficiencies, and gains the beast's: where both have
// one, the higher bonus wins. The senses are the beast's; the character's
// darkvision comes along only if the beast has darkvision too (the larger range).
// The character can't cast spells (Spellcasting, Spells and PactMagic are empty)
// unless Beast Spells keeps them (CastsInBeastForm).
// It does not check that the form is allowed (WildShapeAllows does), only that it
// is a beast.
func (c *Content) WildShapeDerived(character Derived, beast string) (Derived, error) {
	m, ok := c.c.monsters[beast]
	if !ok || m.Type != creatureBeast {
		return Derived{}, fmt.Errorf("rules: %q is not a beast", beast)
	}
	b := c.c.monsterDerived(m)
	d := character
	d.ArmorClass, d.ArmorClassDescription = b.ArmorClass, b.ArmorClassDescription
	d.HitPointsMax, d.HitPointsFromEffects, d.HitDice = b.HitPointsMax, 0, b.HitDice
	d.SpeedWalkFt, d.SpeedFlyFt, d.SpeedSwimFt, d.SpeedClimbFt, d.SpeedBurrowFt, d.Hover =
		b.SpeedWalkFt, b.SpeedFlyFt, b.SpeedSwimFt, b.SpeedClimbFt, b.SpeedBurrowFt, b.Hover
	d.Attacks, d.AttacksPerAction, d.SaveActions = b.Attacks, b.AttacksPerAction, b.SaveActions

	// The physical scores are the beast's; the mental ones stay.
	oldMod, newMod := beastScores(&d, character, b)
	beastSaves(&d, character, m, oldMod, newMod)
	beastSkills(&d, character, m, oldMod, newMod)
	d.Initiative = character.Initiative - oldMod[DEX] + newMod[DEX]
	d.Senses = beastSenses(character.Senses, b.Senses)

	// No spells in beast form, unless Beast Spells (druid 18) keeps them; the
	// beast's traits join the features.
	hint := fmt.Sprintf("Em forma de fera (%s) você não pode conjurar magias, e falar ou usar as mãos fica limitado ao que a fera consegue.", c.c.namePT(beast))
	if c.CastsInBeastForm(character) {
		hint = fmt.Sprintf("Em forma de fera (%s) você conjura magias de druida e dispensa os materiais delas (Magias da Besta).", c.c.namePT(beast))
	} else {
		d.Spellcasting, d.Spells, d.PactMagic = nil, nil, nil
	}
	d.Features = append(slices.Clone(character.Features), b.Features...)
	d.Hints = append(slices.Clone(character.Hints), Hint{Source: "wild_shape", Mode: "note", TextPT: hint})
	return d, nil
}

// beastScores puts the beast's Strength, Dexterity and Constitution on d and
// returns each ability's modifier before and after.
func beastScores(d *Derived, character, beast Derived) (oldMod, newMod map[Ability]int) {
	oldMod, newMod = map[Ability]int{}, map[Ability]int{}
	d.Abilities = slices.Clone(character.Abilities)
	for i, a := range d.Abilities {
		oldMod[a.Ability] = a.Modifier
		newMod[a.Ability] = a.Modifier
		if a.Ability == STR || a.Ability == DEX || a.Ability == CON {
			d.Abilities[i] = beast.Abilities[i]
			newMod[a.Ability] = beast.Abilities[i].Modifier
		}
	}
	return oldMod, newMod
}

// beastSaves gives each saving throw the character's bonus on the new modifier,
// or the beast's listed bonus; the higher one where both are proficient.
func beastSaves(d *Derived, character Derived, m *srd51.Monster, oldMod, newMod map[Ability]int) {
	d.SavingThrows = slices.Clone(character.SavingThrows)
	for i, s := range d.SavingThrows {
		own := s.Bonus - oldMod[s.Ability] + newMod[s.Ability]
		theirs, listed := m.Saves[string(s.Ability)]
		switch {
		case s.Proficient && listed:
			d.SavingThrows[i].Bonus = max(own, theirs)
		case listed:
			d.SavingThrows[i].Bonus, d.SavingThrows[i].Proficient = theirs, true
		default:
			d.SavingThrows[i].Bonus = own
		}
	}
}

// beastSkills does the same for the skills, and sets the three passives.
func beastSkills(d *Derived, character Derived, m *srd51.Monster, oldMod, newMod map[Ability]int) {
	d.Skills = slices.Clone(character.Skills)
	bonus := map[string]int{}
	for i, s := range d.Skills {
		own := s.Bonus - oldMod[s.Ability] + newMod[s.Ability]
		theirs, listed := m.Skills[s.Key]
		switch {
		case s.Proficiency > ProficiencyNone && listed:
			d.Skills[i].Bonus = max(own, theirs)
		case listed:
			d.Skills[i].Bonus, d.Skills[i].Proficiency = theirs, ProficiencyFull
		default:
			d.Skills[i].Bonus = own
		}
		bonus[s.Key] = d.Skills[i].Bonus
	}
	d.PassivePerception = 10 + bonus["skill:perception"]
	d.PassiveInvestigation = 10 + bonus["skill:investigation"]
	d.PassiveInsight = 10 + bonus["skill:insight"]
}

// beastSenses are the beast's senses; the character's darkvision comes along
// only where the beast has darkvision too, at the larger range.
func beastSenses(own, beast []Sense) []Sense {
	out := slices.Clone(beast)
	var dark *Sense
	for i := range own {
		if own[i].Key == "darkvision" {
			dark = &own[i]
		}
	}
	if dark == nil {
		return out
	}
	for i := range out {
		if out[i].Key == "darkvision" && dark.RangeFt > out[i].RangeFt {
			out[i].RangeFt = dark.RangeFt
		}
	}
	return out
}
