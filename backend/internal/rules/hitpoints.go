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
// never falls below one hit point per level. What the effects and that floor
// add to the dice and the modifier is kept apart as HitPointsFromEffects.
func (x *deriver) hitPoints() {
	if len(x.classes) == 0 {
		return
	}
	hp := hitPointTally{con: x.mods[CON], rolled: x.b.HitPoints.Method == HitPointsRolled, dice: map[int]int{}}
	for _, oc := range x.classes {
		for l := 0; l < oc.level && hp.levels < x.d.TotalLevel; l++ {
			x.hitPointLevel(&hp, oc.class.HitDie)
		}
	}
	rolls := x.b.HitPoints.Rolls
	if hp.rolled && len(rolls) != x.d.TotalLevel-1 {
		x.issue(IssueHitPointRolls, "full.hit_points.rolls", "Há %d rolagens de pontos de vida para %d níveis depois do primeiro; os níveis sem rolagem usam a média.", len(rolls), x.d.TotalLevel-1)
	}
	// A level never gives less than 1 hit point, whatever the modifiers say: a
	// table's effect that takes hit points away stops there.
	x.d.HitPointsMax = max(x.modifiers("hp.max", hp.total), x.d.TotalLevel)
	x.d.HitPointsFromEffects = x.d.HitPointsMax - hp.total

	for die, count := range hp.dice {
		x.d.HitDice = append(x.d.HitDice, HitDice{Die: die, Count: count})
	}
	slices.SortFunc(x.d.HitDice, func(a, b HitDice) int { return cmp.Compare(b.Die, a.Die) })
}

// hitPointTally is the hit points as hitPoints adds them level by level: the
// total, the levels counted, the next roll to read and the dice by size.
type hitPointTally struct {
	con           int
	rolled        bool
	total, levels int
	roll          int
	dice          map[int]int
}

// hitPointLevel adds one character level of a class with the given hit die:
// the die's maximum at the first level, else the average or the player's roll
// (an Issue and the average when the roll does not fit the die).
func (x *deriver) hitPointLevel(hp *hitPointTally, die int) {
	hp.levels++
	hp.dice[die]++
	if hp.levels == 1 {
		hp.total += max(die+hp.con, 1)
		return
	}
	gain := die/2 + 1
	if hp.rolled {
		if rolls := x.b.HitPoints.Rolls; hp.roll < len(rolls) {
			r := rolls[hp.roll]
			if r < 1 || r > die {
				x.issue(IssueHitPointRolls, fmt.Sprintf("full.hit_points.rolls[%d]", hp.roll), "A rolagem %d não cabe num d%d; os cálculos usam a média.", r, die)
			} else {
				gain = r
			}
		}
		hp.roll++
	}
	hp.total += max(gain+hp.con, 1)
}

// heavyArmorSpeedPenaltyFt is the speed lost in armor the character is too weak for.
const heavyArmorSpeedPenaltyFt = 10

// speedAndSenses computes the walking speed and special senses.
func (x *deriver) speedAndSenses() {
	speed := 0
	if x.race != nil {
		speed = x.race.SpeedFt
	}
	speed = x.modifiers("speed.walk", speed)
	// Armor whose Strength requirement the character does not meet costs 10
	// ft of speed; dwarves ignore it.
	if a := x.armor; a != nil && a.StrMinimum > 0 && x.scores[STR] < a.StrMinimum && (x.race == nil || x.race.Key != "race:dwarf") {
		speed -= heavyArmorSpeedPenaltyFt
	}
	x.d.SpeedWalkFt = max(speed, 0)

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
