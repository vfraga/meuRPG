package rules

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/PuraFome/meuRPG/backend/internal/rules/srd51"
)

// protoAbility is each ability's field name in the rules.v1.AbilityScores
// proto message, used in Issue and ValidationError fields.
var protoAbility = map[Ability]string{
	STR: "strength", DEX: "dexterity", CON: "constitution",
	INT: "intelligence", WIS: "wisdom", CHA: "charisma",
}

// abbreviationPT is the Portuguese short form of each ability, as on the
// official Brazilian sheet.
var abbreviationPT = map[Ability]string{
	STR: "FOR", DEX: "DES", CON: "CON", INT: "INT", WIS: "SAB", CHA: "CAR",
}

// modifier is floor((score - 10) / 2), also for scores below 10.
func modifier(score int) int {
	d := score - 10
	if d < 0 {
		return (d - 1) / 2
	}
	return d / 2
}

// abilities computes the six scores and modifiers.
func (x *deriver) abilities() {
	raceBonus := x.raceBonuses()
	// The abilities the player picked for the race (the half-elf's two +1).
	picked := x.pickedAbilities()
	for _, a := range picked {
		raceBonus[a]++
	}
	// The abilities the player picked for a table race's bonuses.
	if x.race != nil && !x.replacesRaceBonuses() {
		for a, v := range x.c.racePicksOf(x.race.Key, scopedOf(x.b.FeatureChoices)).bonuses() {
			raceBonus[a] += v
		}
	}
	x.scores = map[Ability]int{}
	x.mods = map[Ability]int{}
	for _, a := range AllAbilities() {
		x.abilityScore(a, raceBonus[a])
	}
	x.checkRaceBonus()
	x.abilityChoiceHint(len(picked))
}

// replacesRaceBonuses says the subrace swaps the race's ability bonuses whole: the fixed ones and the
// ones the player places (a table race's sets, the half-elf's picks) are gone, the subrace's stand alone.
func (x *deriver) replacesRaceBonuses() bool {
	return x.subrace != nil && x.subrace.ReplacesAbilityBonuses
}

// raceBonuses are the fixed ability bonuses of the race and the subrace. A subrace that
// replaces them has its own only.
func (x *deriver) raceBonuses() map[Ability]int {
	raceBonus := map[Ability]int{}
	add := func(bonuses map[string]int) {
		for k, v := range bonuses {
			if a, ok := ability(k); ok {
				raceBonus[a] += v
			}
		}
	}
	if x.race != nil && !x.replacesRaceBonuses() {
		add(x.race.AbilityBonuses)
	}
	if x.subrace != nil {
		add(x.subrace.AbilityBonuses)
	}
	return raceBonus
}

// abilityScore computes one ability's score and modifier from the base score
// (10 with an Issue when it is missing or out of range), the race's bonus and
// the manual bonus (held to ±MaxManualBonus).
func (x *deriver) abilityScore(a Ability, raceBonus int) {
	field := "full.base_scores." + protoAbility[a]
	base, ok := x.b.BaseScores[a]
	if !ok || base < MinScore || base > MaxScore {
		x.issue(IssueMissing, field, "%s precisa de um valor de %d a %d; os cálculos usam 10.", x.c.namePT(string(a)), MinScore, MaxScore)
		base = 10
	}
	manual := min(max(x.b.ExtraAbilityBonuses[a], -MaxManualBonus), MaxManualBonus)
	score := base + raceBonus + manual
	x.scores[a] = score
	x.mods[a] = modifier(score)
	x.d.Abilities = append(x.d.Abilities, AbilityScore{
		Ability: a, NamePT: x.c.namePT(string(a)),
		Base: base, RaceBonus: raceBonus, ManualBonus: manual, Bonus: raceBonus + manual,
		Score: score, Modifier: x.mods[a],
	})
}

// abilityChoiceHint reminds the player of the increases the race lets them
// choose (the half-elf's +1 to two abilities), which go in the manual bonuses,
// while fewer than the race's count were picked and no manual bonus is
// positive.
func (x *deriver) abilityChoiceHint(picked int) {
	if x.race == nil || x.race.AbilityBonusChoices == nil || x.replacesRaceBonuses() {
		return
	}
	for _, v := range x.b.ExtraAbilityBonuses {
		if v > 0 {
			return
		}
	}
	ch := x.race.AbilityBonusChoices
	if picked >= ch.Choose {
		return
	}
	names := make([]string, 0, len(ch.From))
	for _, k := range ch.From {
		names = append(names, abbreviationPT[Ability(k)])
	}
	x.d.Hints = append(x.d.Hints, Hint{
		Source: x.race.Key, Mode: "note",
		TextPT: fmt.Sprintf("%s: escolha +1 em %d habilidades (%s) no passo Escolhas.", x.c.namePT(x.race.Key), ch.Choose, strings.Join(names, ", ")),
	})
}

// abilityEffects adds the features that raise a score (Primal Champion) to the
// scores abilities computed, up to each effect's cap, and then reports a score
// above 20 that nothing explains: a positive manual bonus is where a magic item
// that raises a score goes (the SRD lets an item pass 20), and a feature's cap
// lifts the ceiling for the score it raised.
func (x *deriver) abilityEffects() {
	for i, ab := range x.d.Abilities {
		ceiling, score := MaxNormalScore, ab.Score
		for _, a := range x.active {
			e := a.effect
			if e.Type != "modifier" || e.Target != "score."+string(ab.Ability) || len(e.Tags) > 0 || !x.applies(a) {
				continue
			}
			v, ok := x.value(a)
			if !ok {
				continue
			}
			ceiling = max(ceiling, e.Cap)
			score = max(score, min(score+v, e.Cap))
		}
		if score != ab.Score {
			x.d.Abilities[i].Bonus += score - ab.Score
			x.d.Abilities[i].Score, x.d.Abilities[i].Modifier = score, modifier(score)
			x.scores[ab.Ability], x.mods[ab.Ability] = score, modifier(score)
		}
		if score > ceiling && ab.ManualBonus <= 0 {
			x.issue(IssueScoreAbove20, "full.base_scores."+protoAbility[ab.Ability], "%s passa de %d, o máximo normal de um personagem.", x.c.namePT(string(ab.Ability)), ceiling)
		}
	}
	// The carrying capacity is 15 pounds times the Strength score (SRD 5.1, "Carrying Capacity").
	x.d.CarryCapacityLb = min(max(x.modifiers(CarryCapacityTarget, 15*x.scores[STR]), 0), MaxCarryCapacityLb)
	x.damageReductions()
}

// damageReductions lists the flat reductions of weapon damage the active effects give, one entry per
// feature and kind, in the order of the effects.
func (x *deriver) damageReductions() {
	for _, kind := range []struct {
		target     string
		nonmagical bool
	}{{DamageReductionWeaponTarget, false}, {DamageReductionNonmagicalWeaponTarget, true}} {
		var seen []string
		for _, a := range x.active {
			if a.effect.Type != "modifier" || a.effect.Target != kind.target || slices.Contains(seen, a.owner) {
				continue
			}
			seen = append(seen, a.owner)
			if n := min(max(x.modifiersOf(a.owner, kind.target, 0), 0), MaxDamageReduction); n > 0 {
				x.d.DamageReductions = append(x.d.DamageReductions, DamageReduction{Source: a.owner, Amount: n, Nonmagical: kind.nonmagical})
			}
		}
	}
}

// ownedTraits are the race's and subrace's traits and the chosen trait
// options, in the order of Derived.Features.
func (x *deriver) ownedTraits() []*srd51.Trait {
	var out []*srd51.Trait
	for _, f := range x.d.Features {
		if t, ok := x.c.traits[f.Key]; ok {
			out = append(out, t)
		}
	}
	return out
}

// proficiencies gathers armor, weapon and tool proficiencies: the starting
// class's (a later class gives its smaller multiclass set), the race's, the
// background's and those granted by effects.
func (x *deriver) proficiencies() {
	for _, p := range x.grantedProficiencies() {
		if _, ok := x.c.proficiencies[p]; ok {
			x.proficient[p] = true
		}
	}
	for _, a := range x.active {
		e := a.effect
		if e.Type == "proficiency" && strings.HasPrefix(e.Proficiency, "proficiency:") && len(e.Tags) == 0 && x.applies(a) {
			if _, ok := x.c.proficiencies[e.Proficiency]; ok {
				x.proficient[e.Proficiency] = true
			}
		}
	}
	x.proficiencyPicks()
	for t := range x.dailyState().tools {
		x.proficient[t] = true
	}
	for _, p := range x.pickedTools {
		x.proficient[p.Value] = true
	}
	x.listProficiencies()
}

// grantedProficiencies are the proficiency keys the classes, the traits and the
// background give, known or not.
func (x *deriver) grantedProficiencies() []string {
	var out []string
	for i, oc := range x.classes {
		list := oc.class.Proficiencies
		if i > 0 {
			list = oc.class.Multiclass.Proficiencies
		}
		out = append(out, list...)
	}
	for _, t := range x.ownedTraits() {
		out = append(out, t.Proficiencies...)
	}
	if bg, ok := x.c.backgrounds[x.b.Background]; ok {
		out = append(out, bg.Proficiencies...)
	} else if x.b.Background == "" {
		// A custom background's tools; its languages are listed by languages().
		out = append(out, x.b.CustomBackgroundProficiencies...)
	}
	return out
}

// proficiencyKinds is the order of the kinds in Derived.Proficiencies.
var proficiencyKinds = []string{"armor", "weapon", "tool", "other"}

// proficiencyKindRank is a kind's place in proficiencyKinds; a kind not listed
// sorts with the first.
func proficiencyKindRank(kind string) int { return max(slices.Index(proficiencyKinds, kind), 0) }

// listProficiencies fills Derived.Proficiencies from x.proficient, without the
// skills and saving throws (they have their own lists), by kind and name.
func (x *deriver) listProficiencies() {
	for key := range x.proficient {
		p := x.c.proficiencies[key]
		if p.Kind == "skill" || p.Kind == "saving-throw" {
			continue
		}
		x.d.Proficiencies = append(x.d.Proficiencies, Proficiency{Kind: p.Kind, Key: key, NamePT: x.proficiencyName(key)})
	}
	// The tools the player typed in the free field (what the master adds, or a sheet from before the picks) are listed
	// too, by name.
	listedNames(x.d.Proficiencies, x.b.ToolProficiencies, func(p Proficiency) string { return p.NamePT }, func(name string) {
		x.d.Proficiencies = append(x.d.Proficiencies, Proficiency{Kind: "tool", NamePT: name})
	})
	slices.SortFunc(x.d.Proficiencies, func(a, b Proficiency) int {
		return cmp.Or(cmp.Compare(proficiencyKindRank(a.Kind), proficiencyKindRank(b.Kind)), comparePT(a.NamePT, b.NamePT))
	})
}

// proficiencyName is the Portuguese name of a proficiency, or of the one
// piece of equipment it covers.
func (x *deriver) proficiencyName(key string) string { return x.c.proficiencyNamePT(key) }

// proficiencyNamePT is the same for the content: the sheet and the effect menu
// name a tool alike.
func (c *content) proficiencyNamePT(key string) string {
	if n, ok := c.namesPT[key]; ok {
		return n
	}
	if p := c.proficiencies[key]; p != nil && len(p.Refs) == 1 {
		if n, ok := c.namesPT[p.Refs[0]]; ok {
			return n
		}
	}
	return c.namesEN[key]
}

// savingThrows: the starting class's saves plus any save proficiency from
// effects.
func (x *deriver) savingThrows() {
	proficient := map[Ability]bool{}
	if len(x.classes) > 0 {
		for _, s := range x.classes[0].class.SavingThrows {
			if a, ok := ability(s); ok {
				proficient[a] = true
			}
		}
	}
	for _, a := range x.active {
		e := a.effect
		if e.Type == "proficiency" && strings.HasPrefix(e.Proficiency, "save.") && len(e.Tags) == 0 && x.applies(a) {
			proficient[Ability(strings.TrimPrefix(e.Proficiency, "save."))] = true
		}
	}
	for _, a := range AllAbilities() {
		bonus := x.mods[a]
		if proficient[a] {
			bonus += x.prof
		}
		bonus = x.modifiers("save.all", x.modifiers("save."+string(a), bonus))
		x.d.SavingThrows = append(x.d.SavingThrows, SavingThrow{
			Ability: a, NamePT: x.c.namePT(string(a)), Proficient: proficient[a], Bonus: bonus,
		})
	}
}

// skillLevels returns each skill's proficiency level, and the skills that
// came automatically (race, background, effects) rather than from the
// player's own choice.
func (x *deriver) skillLevels() (map[string]ProficiencyLevel, map[string]bool) {
	sk := skillTally{levels: map[string]ProficiencyLevel{}, automatic: map[string]bool{}}
	for _, s := range x.automaticSkillKeys() {
		sk.raise(s, ProficiencyFull)
		sk.automatic[s] = true
	}
	for _, s := range x.b.SkillProficiencies {
		if _, ok := x.c.skills[s]; ok {
			sk.raise(s, ProficiencyFull)
		}
	}
	for s := range x.dailyState().skills {
		sk.raise(s, ProficiencyFull)
	}
	var conditional []*Effect
	for _, a := range x.active {
		e := a.effect
		if e.Type != "proficiency" || !strings.HasPrefix(e.Proficiency, "skill:") || len(e.Tags) > 0 || !x.applies(a) {
			continue
		}
		if e.OnlyProficient {
			conditional = append(conditional, e)
			continue
		}
		x.skillEffect(&sk, e)
	}
	x.onlyProficientExpertise(&sk, conditional)
	x.expertise(sk.levels)
	return sk.levels, sk.automatic
}

// onlyProficientExpertise doubles the skills the only_proficient effects name
// where the character is proficient from any other source: the levels are read
// before any of them applies, so they never lift each other.
func (x *deriver) onlyProficientExpertise(sk *skillTally, effects []*Effect) {
	proficient := map[string]bool{}
	for skill, lvl := range sk.levels {
		proficient[skill] = lvl >= ProficiencyFull
	}
	for _, e := range effects {
		if e.Proficiency == "skill:*" {
			for _, s := range x.c.skillOrder {
				if proficient[s] {
					sk.raise(s, ProficiencyExpertise)
				}
			}
		} else if proficient[e.Proficiency] {
			sk.raise(e.Proficiency, ProficiencyExpertise)
		}
	}
}

// skillTally is the skills' proficiency levels as skillLevels gathers them, and
// the skills that came automatically.
type skillTally struct {
	levels    map[string]ProficiencyLevel
	automatic map[string]bool
}

// raise lifts a skill to a level, never lowering it.
func (sk *skillTally) raise(skill string, to ProficiencyLevel) {
	if sk.levels[skill] < to {
		sk.levels[skill] = to
	}
}

// automaticSkillKeys are the skills the background (an SRD or table one, or the
// custom one's known skills) and the traits give, in that order.
func (x *deriver) automaticSkillKeys() []string {
	var out []string
	if bg, ok := x.c.backgrounds[x.b.Background]; ok {
		out = append(out, bg.Skills...)
	}
	if x.b.Background == "" {
		for _, s := range x.b.CustomBackgroundSkills {
			if _, ok := x.c.skills[s]; ok {
				out = append(out, s)
			}
		}
	}
	for _, t := range x.ownedTraits() {
		for _, p := range t.Proficiencies {
			if skill, ok := strings.CutPrefix(p, "proficiency:skill-"); ok {
				out = append(out, "skill:"+skill)
			}
		}
	}
	return out
}

// skillEffect applies a skill proficiency effect: "skill:*" raises every skill
// (Jack of All Trades), and a full or better proficiency in one skill counts as
// automatic.
func (x *deriver) skillEffect(sk *skillTally, e *Effect) {
	lvl := proficiencyLevelOf(e.Level)
	if e.Proficiency == "skill:*" {
		for _, s := range x.c.skillOrder {
			sk.raise(s, lvl)
		}
		return
	}
	sk.raise(e.Proficiency, lvl)
	if lvl >= ProficiencyFull {
		sk.automatic[e.Proficiency] = true
	}
}

// expertise sets the expertise skills, with an Issue for an unknown skill (left
// out) and for one without proficiency (set all the same).
func (x *deriver) expertise(levels map[string]ProficiencyLevel) {
	for i, s := range x.b.Expertise {
		if _, ok := x.c.skills[s]; !ok {
			x.issue(IssueUnknownKey, fmt.Sprintf("full.expertise_skill_keys[%d]", i), "A perícia escolhida não existe.")
			continue
		}
		if levels[s] < ProficiencyFull {
			x.issue(IssueExpertise, fmt.Sprintf("full.expertise_skill_keys[%d]", i), "Especialização em %s pede proficiência nessa perícia.", x.c.namePT(s))
		}
		levels[s] = ProficiencyExpertise
	}
}

func proficiencyLevelOf(s string) ProficiencyLevel {
	switch s {
	case "half":
		return ProficiencyHalf
	case "expertise":
		return ProficiencyExpertise
	default:
		return ProficiencyFull
	}
}

// profBonus is the part of the proficiency bonus a level adds.
func (x *deriver) profBonus(l ProficiencyLevel) int {
	switch l {
	case ProficiencyHalf:
		return x.prof / 2
	case ProficiencyFull:
		return x.prof
	case ProficiencyExpertise:
		return 2 * x.prof
	}
	return 0
}

// skills computes the 18 skills, the three passives and initiative.
func (x *deriver) skills() {
	levels, automatic := x.skillLevels()
	x.skillLevel, x.automaticSkills = levels, automatic
	bonus := map[string]int{}
	for _, key := range x.c.skillOrder {
		s := x.c.skills[key]
		a := Ability(s.Ability)
		b := x.mods[a] + x.profBonus(levels[key])
		b = x.modifiers("skill."+strings.TrimPrefix(key, "skill:"), b)
		bonus[key] = b
		x.d.Skills = append(x.d.Skills, Skill{
			Key: key, NamePT: x.c.namePT(key), Ability: a, Proficiency: levels[key], Bonus: b, UntilRest: x.dailyState().skills[key],
		})
	}
	slices.SortFunc(x.d.Skills, func(a, b Skill) int { return comparePT(a.NamePT, b.NamePT) })
	x.d.PassivePerception = max(x.modifiers(PassivePerceptionTarget, 10+bonus["skill:perception"]), 0)
	x.d.PassiveInvestigation = max(x.modifiers(PassiveInvestigationTarget, 10+bonus["skill:investigation"]), 0)
	x.d.PassiveInsight = max(x.modifiers(PassiveInsightTarget, 10+bonus["skill:insight"]), 0)

	// Initiative is a Dexterity check: Jack of All Trades adds half the
	// proficiency bonus to it, through an "initiative" proficiency effect.
	best := ProficiencyNone
	for _, a := range x.active {
		e := a.effect
		if e.Type == "proficiency" && e.Proficiency == "initiative" && len(e.Tags) == 0 && x.applies(a) {
			best = max(best, proficiencyLevelOf(e.Level))
		}
	}
	x.d.Initiative = x.modifiers("initiative", x.mods[DEX]+x.profBonus(best))
}

// comparePT orders Portuguese names ignoring accents and case, so
// "Atletismo" comes before "Él" and "Íntimo" sorts with the I's.
func comparePT(a, b string) int {
	return cmp.Or(cmp.Compare(foldPT(a), foldPT(b)), cmp.Compare(a, b))
}

var accentFolder = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a",
	"é", "e", "ê", "e", "è", "e", "í", "i", "ì", "i", "î", "i",
	"ó", "o", "ô", "o", "õ", "o", "ò", "o", "ö", "o",
	"ú", "u", "ù", "u", "û", "u", "ü", "u", "ç", "c",
)

func foldPT(s string) string {
	return accentFolder.Replace(strings.ToLower(s))
}

// checkRaceBonus checks the bonuses a table race lets the player place ("+2 and +1 to your choice", or one of several
// sets): the player picks them in "Escolhas" (racebonuspicks.go), and an Issue says when the picks do not fill a set and
// the manual bonuses, where a sheet placed them before they were picks, cover none. A level's Ability Score Improvement
// only adds to the manual bonuses, so it never takes the check back.
func (x *deriver) checkRaceBonus() {
	if x.race == nil || x.replacesRaceBonuses() {
		return
	}
	sets := x.c.raceBonusSets(x.race.Key)
	if len(sets) == 0 {
		return
	}
	if x.c.racePicksOf(x.race.Key, scopedOf(x.b.FeatureChoices)).complete(sets) || x.c.manualCoversSet(x.race.Key, x.b.ExtraAbilityBonuses) >= 0 {
		return
	}
	options := make([]string, len(sets))
	for i, set := range sets {
		options[i] = bonusSetWords(set)
	}
	text := "escolha em quais habilidades vão os bônus %s, no passo Escolhas."
	if len(sets) > 1 {
		text = "escolha um dos grupos (%s) e em quais habilidades vão os bônus, no passo Escolhas."
	}
	x.issue(IssueRaceBonus, choiceKeysField, "%s: "+text, x.c.namePT(x.race.Key), strings.Join(options, " ou "))
}

// coversBonuses says the placed bonuses (largest first) cover the wanted ones (largest first).
func coversBonuses(placed, want []int) bool {
	if len(placed) < len(want) {
		return false
	}
	for i, v := range want {
		if placed[i] < v {
			return false
		}
	}
	return true
}

// raceBonusSets are the sets of ability bonuses a table race lets the player place, largest first: its
// alternative sets, or its single set of choice_bonuses; nil when it has none.
func (c *content) raceBonusSets(raceKey string) [][]int {
	if sets := c.raceSets[raceKey]; len(sets) > 0 {
		return sets
	}
	if one := c.raceChoice[raceKey]; len(one) > 0 {
		return [][]int{one}
	}
	return nil
}

// racePlacedPoints is the most points the player can place on abilities for a table race's sets, 0 when the
// subrace swaps the race's bonuses whole.
func (c *content) racePlacedPoints(raceKey, subraceKey string) int {
	if s := c.subraces[subraceKey]; s != nil && s.Race == raceKey && s.ReplacesAbilityBonuses {
		return 0
	}
	n := 0
	for _, set := range c.raceBonusSets(raceKey) {
		n = max(n, sumOf(set)) // the player picks one set: the largest is the most it can place
	}
	return n
}
