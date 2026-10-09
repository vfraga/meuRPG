package rules

import "slices"

// What a monster's Spellcasting and Innate Spellcasting traits give a combat: the
// spells to cast, the slots and the damaging cantrips as attacks, in the shapes the
// turn options already read for a character (Derived.Spellcasting, Spells, SpellSlots,
// Attacks), so the spell flow that exists for an NPC with a full sheet casts a
// monster's spells too.

// MonsterCasting is a creature's spellcasting as a Derived reads it.
type MonsterCasting struct {
	// Spellcasting is one entry for each trait, the first being the one a spell uses
	// when none of them has it on its class list.
	Spellcasting []Spellcasting
	// Spells are all the spells of the creature, prepared.
	Spells []CharacterSpell
	// Slots[l-1] are the spell slots of level l.
	Slots []int
	// Level is the creature's spellcaster level, which scales its cantrips (SRD 5.1 says a
	// cantrip's damage grows with the caster's level; a creature that casts innately has
	// no level, and casts them at the 1st).
	Level int
	// Cantrips are the damaging cantrips as attacks, with the creature's spell attack
	// bonus or save DC.
	Cantrips []Attack
}

// MonsterCasting returns the spellcasting of a creature, and whether it has any.
func (c *Content) MonsterCasting(key string) (MonsterCasting, bool) {
	plan, ok := c.c.monsterPlans[key]
	if !ok || len(plan.Spellcasting) == 0 {
		return MonsterCasting{}, false
	}
	out := MonsterCasting{Slots: make([]int, 9), Level: 1}
	seen := map[string]bool{}
	for _, sc := range plan.Spellcasting {
		out.Spellcasting = append(out.Spellcasting, Spellcasting{
			Class: sc.ClassKey, ClassNamePT: c.c.namePT(sc.ClassKey), Ability: sc.Ability, SaveDC: sc.SaveDC, AttackBonus: sc.AttackBonus,
			SpellList: sc.ClassKey,
		})
		out.Level = max(out.Level, sc.CasterLevel)
		refs := slices.Clone(sc.AtWill)
		for _, l := range sc.Levels {
			refs = append(refs, l.Spells...)
			if l.Level >= 1 && l.Level <= 9 {
				out.Slots[l.Level-1] += l.Slots
			}
		}
		for _, g := range sc.PerDay {
			refs = append(refs, g.Spells...)
		}
		for _, r := range refs {
			if seen[r.Key] {
				continue
			}
			seen[r.Key] = true
			if entry, ok := c.c.spellEntries[r.Key]; ok {
				out.Spells = append(out.Spells, CharacterSpell{Spell: entry, Prepared: true})
			}
		}
	}
	for _, cs := range out.Spells {
		if a, ok := c.monsterCantrip(cs.Spell.Key, out); ok {
			out.Cantrips = append(out.Cantrips, a)
		}
	}
	return out, true
}

// monsterCantrip is a damaging cantrip as an attack of the creature: a spell attack with
// its spell attack bonus, or a saving throw with its DC, rolled at its spellcaster level.
func (c *Content) monsterCantrip(key string, mc MonsterCasting) (Attack, bool) {
	s, ok := c.c.spells[key]
	if !ok || s.Level != 0 || len(s.Damage) == 0 || len(s.Damage[0].AtCharacterLevel) == 0 {
		return Attack{}, false
	}
	sc := mc.Spellcasting[0]
	for _, o := range mc.Spellcasting {
		if slices.Contains(c.c.spellEntries[key].Classes, o.SpellList) {
			sc = o
			break
		}
	}
	dice := damageAt(s.Damage[0].AtCharacterLevel, mc.Level)
	a := Attack{
		Key: key, Name: s.Name, NamePT: c.c.namePT(key), Kind: "spell", Ability: sc.Ability,
		Damage: dice, SpellDice: dice,
		DamageType: s.Damage[0].DamageType, DamageTypeNamePT: c.c.namePT(s.Damage[0].DamageType),
		Proficient: true, RangeFt: feet(s.Range), Beams: 1,
	}
	switch {
	case s.AttackType != "":
		a.AttackBonus = sc.AttackBonus
	case s.SaveAbility != "":
		a.SaveDC = sc.SaveDC
		a.SaveAbility = Ability(s.SaveAbility)
	}
	a.DamageDice, _ = ParseDice(a.Damage)
	return a, true
}
