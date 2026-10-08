package rules

import (
	"cmp"
	"fmt"
	"slices"
)

// hitPoints computes the maximum hit points and the hit dice.
//
// The first character level takes the starting class's hit die maximum.
// Every later level takes the fixed average (half the die plus one) or the
// player's roll, in the order of Build.Classes. The constitution modifier is
// added at every level, and a level never adds less than 1. "hp.max"
// effects add on top (Dwarven Toughness, Draconic Resilience), and the maximum
// never falls below one hit point per level.
func (x *deriver) hitPoints() {
	if len(x.classes) == 0 {
		return
	}
	con := x.mods[CON]
	rolled := x.b.HitPoints.Method == HitPointsRolled
	rolls := x.b.HitPoints.Rolls
	hp, n, levels := 0, 0, 0
	dice := map[int]int{}
	for _, oc := range x.classes {
		die := oc.class.HitDie
		for l := 0; l < oc.level && levels < x.d.TotalLevel; l++ {
			levels++
			dice[die]++
			if levels == 1 {
				hp += max(die+con, 1)
				continue
			}
			gain := die/2 + 1
			if rolled {
				if n < len(rolls) {
					r := rolls[n]
					if r < 1 || r > die {
						x.issue(IssueHitPointRolls, fmt.Sprintf("full.hit_points.rolls[%d]", n), "A rolagem %d não cabe num d%d; os cálculos usam a média.", r, die)
					} else {
						gain = r
					}
				}
				n++
			}
			hp += max(gain+con, 1)
		}
	}
	if rolled && len(rolls) != x.d.TotalLevel-1 {
		x.issue(IssueHitPointRolls, "full.hit_points.rolls", "Há %d rolagens de pontos de vida para %d níveis depois do primeiro; os níveis sem rolagem usam a média.", len(rolls), x.d.TotalLevel-1)
	}
	// A level never gives less than 1 hit point, whatever the modifiers say: a
	// table's effect that takes hit points away stops there.
	x.d.HitPointsMax = max(x.modifiers("hp.max", hp), x.d.TotalLevel)

	for die, count := range dice {
		x.d.HitDice = append(x.d.HitDice, HitDice{Die: die, Count: count})
	}
	slices.SortFunc(x.d.HitDice, func(a, b HitDice) int { return cmp.Compare(b.Die, a.Die) })
}

// speedAndSenses computes the walking speed and special senses.
func (x *deriver) speedAndSenses() {
	speed := 0
	if x.race != nil {
		speed = x.race.SpeedFt
	}
	x.d.SpeedWalkFt = max(x.modifiers("speed.walk", speed), 0)

	best := map[string]int{}
	source := map[string]string{}
	var order []string
	for _, a := range x.active {
		e := a.effect
		if e.Type != "sense" || !x.applies(a) {
			continue
		}
		if _, seen := best[e.Sense]; !seen {
			order = append(order, e.Sense)
		}
		if e.RangeFt > best[e.Sense] {
			best[e.Sense] = e.RangeFt
			source[e.Sense] = a.owner
		}
	}
	for _, s := range order {
		x.d.Senses = append(x.d.Senses, Sense{
			Key: s, NamePT: x.c.namePT("sense:" + s), RangeFt: best[s], Source: source[s],
		})
	}
}

// languages lists the languages the race grants. Languages the player
// chooses (from the background, a subrace or a class) are free text on the
// sheet.
func (x *deriver) languages() {
	add := func(l string) {
		if _, ok := x.c.languages[l]; ok && !slices.ContainsFunc(x.d.Languages, func(n NamedKey) bool { return n.Key == l }) {
			x.d.Languages = append(x.d.Languages, NamedKey{Key: l, NamePT: x.c.namePT(l)})
		}
	}
	if x.race != nil {
		for _, l := range x.race.Languages {
			add(l)
		}
	}
	// A custom background's two picks are tools or languages, in any mix: the
	// languages join the race's (the tools are in proficiencies()).
	if x.b.Background == "" {
		for _, l := range x.b.CustomBackgroundProficiencies {
			add(l)
		}
	}
}
