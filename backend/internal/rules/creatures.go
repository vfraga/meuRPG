package rules

import (
	"cmp"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/PuraFome/meuRPG/backend/internal/rules/srd51"
)

// The SRD creatures (MR-037, Etapa 9): the 334 stat blocks of
// data/monsters.json, the lists the screens search, and MonsterDerived, which
// turns a stat block into the Derived that combat already reads. A creature
// is a familiar, a summoned beast, a Wild Shape form (wildshape.go) or one
// the master gives. The text of a stat block stays the SRD's English, as the
// spells' does; the names and the labels are Portuguese.

// crEighths is a challenge rating in eighths, so 1/8 is 1 and 2 is 16, and the
// ratings compare as integers. ok is false for anything but the SRD's 34
// ratings ("0", "1/8", "1/4", "1/2", "1" to "30").
func crEighths(rating string) (int, bool) {
	switch rating {
	case "0":
		return 0, true
	case "1/8":
		return 1, true
	case "1/4":
		return 2, true
	case "1/2":
		return 4, true
	}
	n, err := strconv.Atoi(rating)
	if err != nil || n < 1 || n > 30 || strconv.Itoa(n) != rating {
		return 0, false
	}
	return 8 * n, true
}

// sizeNamePT, creatureTypeNamePT, armorClassNamePT and saveSuccessNamePT are
// the Portuguese labels of the stat block's fixed words.
var (
	sizeNamePT = map[string]string{
		"Tiny": "Miúdo", "Small": "Pequeno", "Medium": "Médio", "Large": "Grande", "Huge": "Enorme", "Gargantuan": "Imenso",
	}
	creatureTypeNamePT = map[string]string{
		"aberration": "aberração", "beast": "fera", "celestial": "celestial", "construct": "construto", "dragon": "dragão",
		"elemental": "elemental", "fey": "fada", "fiend": "ínfero", "giant": "gigante", "humanoid": "humanoide",
		"monstrosity": "monstruosidade", "ooze": "gosma", "plant": "planta", "undead": "morto-vivo",
		"swarm of Tiny beasts": "enxame de feras miúdas",
	}
	armorClassNamePT = map[string]string{
		"natural": "Armadura natural", "armor": "Armadura", "dex": "Sem armadura", "spell": "Magia", "condition": "Condição",
	}
)

// creatureBeast is the type Wild Shape and Conjure Animals take.
const creatureBeast = "beast"

// CreatureEntry is a creature as a list shows it.
type CreatureEntry struct {
	// Key is "monster:wolf"; Name the SRD's English name; NamePT ours.
	Key, Name, NamePT string
	// Size is the SRD's word ("Medium") and SizeNamePT the Portuguese one.
	Size, SizeNamePT string
	// Type is the SRD's type ("beast"), TypeNamePT its Portuguese name and
	// Subtype the SRD's tag ("goblinoid"), or "".
	Type, TypeNamePT, Subtype string
	// ChallengeRating is "0", "1/8", "1/4", "1/2", "1" to "30", and XP what
	// the creature gives when defeated.
	ChallengeRating string
	XP              int
	// CanFly and CanSwim say the stat block has a fly or a swim speed.
	CanFly, CanSwim bool
	// ArmorClass and HitPoints are the stat block's armor class and average hit
	// points, the two numbers a list row shows beside the name (MR-042).
	ArmorClass, HitPoints int
}

// checkMonsters refuses a stat block that points at something that does not
// exist: a skill, a save ability, a damage type, a condition or a challenge
// rating. data/ is generated, so this catches a bad importer or a bad commit.
// The stat blocks are checked in key order and the first problem is returned.
func (c *content) checkMonsters() error {
	for _, k := range sortedKeys(c.monsters) {
		if problem := c.monsterProblem(c.monsters[k]); problem != "" {
			return fmt.Errorf("data/monsters.json: %s: %s", k, problem)
		}
	}
	return nil
}

// monsterProblem is the first thing wrong with a stat block, or "".
func (c *content) monsterProblem(m *srd51.Monster) string {
	if _, ok := crEighths(m.ChallengeRating); !ok {
		return fmt.Sprintf("%q is not a challenge rating", m.ChallengeRating)
	}
	for _, check := range [...]func(*srd51.Monster) string{
		c.skillsProblem, savesProblem, c.conditionImmunitiesProblem, c.damageModsProblem, c.armorClassProblem, c.actionsProblem,
	} {
		if problem := check(m); problem != "" {
			return problem
		}
	}
	return ""
}

func (c *content) skillsProblem(m *srd51.Monster) string {
	for s := range m.Skills {
		if _, ok := c.skills[s]; !ok {
			return fmt.Sprintf("%q is not a skill", s)
		}
	}
	return ""
}

func savesProblem(m *srd51.Monster) string {
	for a := range m.Saves {
		if _, ok := abilityIndex[Ability(a)]; !ok {
			return fmt.Sprintf("%q is not an ability", a)
		}
	}
	return ""
}

func (c *content) conditionImmunitiesProblem(m *srd51.Monster) string {
	for _, k := range m.ConditionImmunities {
		if n, ok := c.named[k]; !ok || n == nil || !strings.HasPrefix(k, "condition:") {
			return fmt.Sprintf("%q is not a condition", k)
		}
	}
	return ""
}

// isDamageTypeKey says the key is a damage type of the content.
func (c *content) isDamageTypeKey(k string) bool {
	_, ok := c.named[k]
	return ok && strings.HasPrefix(k, "damage-type:")
}

func (c *content) damageModsProblem(m *srd51.Monster) string {
	for _, list := range [][]srd51.MonsterDamageMod{m.Vulnerabilities, m.Resistances, m.Immunities} {
		for _, d := range list {
			for _, t := range d.Types {
				if !c.isDamageTypeKey(t) {
					return fmt.Sprintf("%q is not a damage type", t)
				}
			}
		}
	}
	return ""
}

// armorClassProblem checks the equipment the armor class wears and the spell and
// condition of its alternatives.
func (c *content) armorClassProblem(m *srd51.Monster) string {
	for _, e := range m.ArmorClassItems {
		if _, ok := c.equipment[e]; !ok {
			return fmt.Sprintf("%q is not equipment", e)
		}
	}
	for _, alt := range m.ArmorClassAlts {
		if _, ok := c.spells[alt.Spell]; alt.Spell != "" && !ok {
			return fmt.Sprintf("%q is not a spell", alt.Spell)
		}
		if _, ok := c.named[alt.Condition]; alt.Condition != "" && !ok {
			return fmt.Sprintf("%q is not a condition", alt.Condition)
		}
	}
	return ""
}

func (c *content) actionsProblem(m *srd51.Monster) string {
	names := map[string]bool{}
	for _, a := range m.Actions {
		names[a.Name] = true
	}
	for _, a := range m.Actions {
		if problem := multiattackProblem(&a, names); problem != "" {
			return problem
		}
		if problem := c.actionDamageProblem(&a); problem != "" {
			return problem
		}
		if a.Save != nil {
			if _, ok := abilityIndex[Ability(a.Save.Ability)]; !ok {
				return fmt.Sprintf("%s: %q is not an ability", a.Name, a.Save.Ability)
			}
		}
	}
	return ""
}

// multiattackProblem checks the routines of a Multiattack: each part is done at
// least once and, unless it is an ability or a spell, is an action of the same
// stat block (names).
func multiattackProblem(a *srd51.MonsterAction, names map[string]bool) string {
	for _, routine := range a.Multiattack {
		for _, x := range routine {
			if x.Count < 1 {
				return fmt.Sprintf("%s: %q has a count of %d", a.Name, x.Name, x.Count)
			}
			if !names[x.Name] && x.Kind != "ability" && x.Kind != "magic" {
				return fmt.Sprintf("%s: %q is not an action of the stat block", a.Name, x.Name)
			}
		}
	}
	return ""
}

func (c *content) actionDamageProblem(a *srd51.MonsterAction) string {
	for _, d := range a.Damage {
		if !c.isDamageTypeKey(d.DamageType) {
			return fmt.Sprintf("%s: %q is not a damage type", a.Name, d.DamageType)
		}
		if _, ok := ParseDice(d.Dice); !ok {
			return fmt.Sprintf("%s: %q are not dice", a.Name, d.Dice)
		}
	}
	return ""
}

func (c *content) buildCreatures() {
	for _, k := range sortedKeys(c.monsters) {
		m := c.monsters[k]
		c.monsterEntries = append(c.monsterEntries, CreatureEntry{
			Key: k, Name: m.Name, NamePT: c.namePT(k),
			Size: m.Size, SizeNamePT: sizeNamePT[m.Size],
			Type: m.Type, TypeNamePT: creatureTypeNamePT[m.Type], Subtype: m.Subtype,
			ChallengeRating: m.ChallengeRating, XP: m.XP,
			CanFly: m.Speed.Fly > 0, CanSwim: m.Speed.Swim > 0,
			ArmorClass: m.ArmorClass, HitPoints: m.HitPoints,
		})
	}
	sortPT(c.monsterEntries, func(e CreatureEntry) string { return e.NamePT })
}

// The reasons ListCreatures refuses a filter that cannot be right. They come
// inside a *CreatureFilterError, which says which field was wrong.
var (
	ErrChallengeRating = errors.New("rules: not a challenge rating")
	ErrChallengeRange  = errors.New("rules: min_cr is above max_cr")
	ErrCreatureSize    = errors.New("rules: not a creature size")
)

// CreatureFilterError is a filter ListCreatures refuses: Field is the request's
// field ("min_cr", "max_cr" or "size") and Err one of the errors above.
type CreatureFilterError struct {
	Field string
	Err   error
}

func (e *CreatureFilterError) Error() string { return fmt.Sprintf("%v (%s)", e.Err, e.Field) }
func (e *CreatureFilterError) Unwrap() error { return e.Err }

// CreatureFilter narrows the creatures a list shows. The zero value matches
// every creature.
type CreatureFilter struct {
	// Query is part of the Portuguese or the English name, ignoring case and
	// accents.
	Query string
	// Type is a creature type ("beast"); "" for any.
	Type string
	// MinCR and MaxCR are the lowest and the highest challenge rating, such as
	// "1/4"; "" for no limit. MinCR must not be above MaxCR.
	MinCR, MaxCR string
	// Size is an SRD size word ("Large"); "" for any size.
	Size string
	// NoFly and NoSwim leave out creatures with a fly or a swim speed.
	NoFly, NoSwim bool
}

// ListCreatures returns the creatures that match the filter, sorted by
// Portuguese name. It refuses a MinCR or a MaxCR that is not one of the SRD's
// ratings (ErrChallengeRating), a MinCR above the MaxCR (ErrChallengeRange) and
// a Size that is not one of the SRD's (ErrCreatureSize).
func (c *Content) ListCreatures(f CreatureFilter) ([]CreatureEntry, error) {
	limit, floor := -1, -1
	if f.MaxCR != "" {
		v, ok := crEighths(f.MaxCR)
		if !ok {
			return nil, &CreatureFilterError{Field: "max_cr", Err: ErrChallengeRating}
		}
		limit = v
	}
	if f.MinCR != "" {
		v, ok := crEighths(f.MinCR)
		if !ok {
			return nil, &CreatureFilterError{Field: "min_cr", Err: ErrChallengeRating}
		}
		floor = v
	}
	if limit >= 0 && floor > limit {
		return nil, &CreatureFilterError{Field: "min_cr", Err: ErrChallengeRange}
	}
	if _, ok := sizeNamePT[f.Size]; f.Size != "" && !ok {
		return nil, &CreatureFilterError{Field: "size", Err: ErrCreatureSize}
	}
	query := foldPT(strings.TrimSpace(f.Query))
	var out []CreatureEntry
	for _, e := range c.c.monsterEntries {
		if f.Type != "" && e.Type != f.Type {
			continue
		}
		if f.Size != "" && e.Size != f.Size {
			continue
		}
		if v, _ := crEighths(e.ChallengeRating); (limit >= 0 && v > limit) || v < floor {
			continue
		}
		if (f.NoFly && e.CanFly) || (f.NoSwim && e.CanSwim) {
			continue
		}
		if query != "" && !strings.Contains(foldPT(e.NamePT), query) && !strings.Contains(foldPT(e.Name), query) {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

// Creature is a stat block with the Portuguese labels.
type Creature struct {
	CreatureEntry
	Alignment string
	// ArmorClassNamePT is what the armor class (CreatureEntry.ArmorClass) comes
	// from ("Armadura natural") and ArmorClassNote what it wears and its other
	// values, in Portuguese ("armadura de couro, escudo; 15 com Armadura
	// Arcana"), or "".
	ArmorClassNamePT string
	ArmorClassNote   string
	// HitDice is the dice ("2d8") and HitPointsRoll the dice with the bonus
	// ("2d8+2"); the average is CreatureEntry.HitPoints.
	HitDice       string
	HitPointsRoll string
	// Speeds are in feet.
	SpeedWalkFt, SpeedFlyFt, SpeedSwimFt, SpeedClimbFt, SpeedBurrowFt int
	Hover                                                             bool
	// Abilities are the six scores in AllAbilities order (Base is the score).
	Abilities []AbilityScore
	// Saves and Skills are the stat block's own bonuses, the ones it lists.
	Saves  []SavingThrow
	Skills []Skill
	// Vulnerabilities, Resistances and Immunities are damage taken double,
	// half or not at all.
	Vulnerabilities, Resistances, Immunities []CreatureDamageMod
	ConditionImmunities                      []NamedKey
	Senses                                   []Sense
	PassivePerception                        int
	// Languages is the SRD's line in Portuguese (LanguagesPT).
	Languages        string
	ProficiencyBonus int
	// Traits, Reactions and LegendaryActions are names and English text.
	Traits           []CreatureAbility
	Actions          []CreatureAction
	Reactions        []CreatureAbility
	LegendaryActions []CreatureAbility
}

// CreatureDamageMod is a vulnerability, resistance or immunity entry.
type CreatureDamageMod struct {
	// Types are the damage types it names ("damage-type:fire", "Fogo").
	Types []NamedKey
	// Note is the rest of the SRD's text, in English ("from nonmagical
	// weapons"), or the whole text when it names no damage type.
	Note string
}

// CreatureAbility is a trait, reaction or legendary action: a name and the
// SRD's text, in English. Usage is the SRD's limit ("3/day", "Recharge 5-6").
type CreatureAbility struct {
	Name, Text, Usage string
}

// CreatureAction is an action of a stat block.
type CreatureAction struct {
	Name, Text, Usage string
	// NamePT is the action's Portuguese name when the content has one (the attacks
	// a character can have through a creature: names_pt.json "attack:<slug>"); empty otherwise.
	NamePT string
	// HasAttack says it is an attack roll, with AttackBonus to the d20.
	HasAttack   bool
	AttackBonus int
	// Damage are the damage parts, in order.
	Damage []CreatureDamage
	// Save is the saving throw the action asks for, or nil.
	Save *CreatureSave
	// Multiattack is the routines of the Multiattack action: the creature
	// makes one routine's attacks. Empty for any other action.
	Multiattack [][]CreatureAttackCount
}

// CreatureDamage is one damage part ("2d4+2", "damage-type:piercing").
type CreatureDamage struct {
	Dice, TypeKey, TypeNamePT string
}

// CreatureSave is a saving throw an action asks for.
type CreatureSave struct {
	Ability Ability
	DC      int
	// OnSuccess is "none", "half" or "other".
	OnSuccess string
}

// CreatureAttackCount is one attack of a Multiattack routine: the name of the
// stat block's action, how many times, and its kind ("melee", "ranged",
// "ability" or "magic").
type CreatureAttackCount struct {
	Name  string
	Count int
	Kind  string
	// Text is the SRD's words for a count that is not a number ("Number of
	// Heads"), or "".
	Text string
}

// CreatureByKey returns one stat block, and whether the key is a creature.
func (c *Content) CreatureByKey(key string) (Creature, bool) {
	m, ok := c.c.monsters[key]
	if !ok {
		return Creature{}, false
	}
	return c.c.creature(m), true
}

func (c *content) creature(m *srd51.Monster) Creature {
	var entry CreatureEntry
	for _, e := range c.monsterEntries {
		if e.Key == m.Key {
			entry = e
			break
		}
	}
	out := Creature{
		CreatureEntry: entry, Alignment: m.Alignment,
		ArmorClassNamePT: armorClassNamePT[m.ArmorClassType], ArmorClassNote: c.armorClassNote(m),
		HitDice: m.HitDice, HitPointsRoll: m.HitPointsRoll,
		SpeedWalkFt: m.Speed.Walk, SpeedFlyFt: m.Speed.Fly, SpeedSwimFt: m.Speed.Swim,
		SpeedClimbFt: m.Speed.Climb, SpeedBurrowFt: m.Speed.Burrow, Hover: m.Speed.Hover,
		PassivePerception: m.PassivePerception, Languages: LanguagesPT(m.Languages), ProficiencyBonus: m.ProficiencyBonus,
	}
	scores := monsterScores(m)
	for _, a := range AllAbilities() {
		out.Abilities = append(out.Abilities, AbilityScore{
			Ability: a, NamePT: c.namePT(string(a)), Base: scores[a], Score: scores[a], Modifier: modifier(scores[a]),
		})
		if v, ok := m.Saves[string(a)]; ok {
			out.Saves = append(out.Saves, SavingThrow{Ability: a, NamePT: c.namePT(string(a)), Proficient: true, Bonus: v})
		}
	}
	for _, k := range sortedKeys(m.Skills) {
		out.Skills = append(out.Skills, Skill{
			Key: k, NamePT: c.namePT(k), Ability: Ability(c.skills[k].Ability), Proficiency: ProficiencyFull, Bonus: m.Skills[k],
		})
	}
	sortPT(out.Skills, func(s Skill) string { return s.NamePT })
	mods := func(list []srd51.MonsterDamageMod) []CreatureDamageMod {
		var res []CreatureDamageMod
		for _, d := range list {
			e := CreatureDamageMod{Note: d.Note}
			for _, t := range d.Types {
				e.Types = append(e.Types, NamedKey{Key: t, NamePT: c.namePT(t)})
			}
			res = append(res, e)
		}
		return res
	}
	out.Vulnerabilities, out.Resistances, out.Immunities = mods(m.Vulnerabilities), mods(m.Resistances), mods(m.Immunities)
	for _, k := range m.ConditionImmunities {
		out.ConditionImmunities = append(out.ConditionImmunities, NamedKey{Key: k, NamePT: c.namePT(k)})
	}
	out.Senses = c.monsterSenses(m)
	ability := func(list []srd51.MonsterAbility) []CreatureAbility {
		var res []CreatureAbility
		for _, a := range list {
			res = append(res, CreatureAbility{Name: a.Name, Text: a.Desc, Usage: a.Usage})
		}
		return res
	}
	out.Traits, out.Reactions, out.LegendaryActions = ability(m.SpecialAbilities), ability(m.Reactions), ability(m.LegendaryActions)
	for _, a := range m.Actions {
		act := CreatureAction{Name: a.Name, NamePT: c.namesPT["attack:"+slugOf(a.Name)], Text: a.Desc, Usage: a.Usage, HasAttack: a.HasAttack, AttackBonus: a.AttackBonus}
		for _, d := range a.Damage {
			act.Damage = append(act.Damage, CreatureDamage{Dice: d.Dice, TypeKey: d.DamageType, TypeNamePT: c.namePT(d.DamageType)})
		}
		if a.Save != nil {
			act.Save = &CreatureSave{Ability: Ability(a.Save.Ability), DC: a.Save.DC, OnSuccess: a.Save.OnSuccess}
		}
		for _, routine := range a.Multiattack {
			var r []CreatureAttackCount
			for _, x := range routine {
				r = append(r, CreatureAttackCount{Name: x.Name, Count: x.Count, Kind: x.Kind, Text: x.Text})
			}
			act.Multiattack = append(act.Multiattack, r)
		}
		out.Actions = append(out.Actions, act)
	}
	return out
}

// armorClassNote says in Portuguese what a creature wears and its other armor
// classes: the armor and shield by name ("armadura de couro, escudo"), the SRD's
// own note ("armor scraps"), and the values with a spell or a condition ("15
// com Armadura Arcana", "11 se Derrubado").
func (c *content) armorClassNote(m *srd51.Monster) string {
	var parts []string
	var worn []string
	for _, k := range m.ArmorClassItems {
		n := c.namePT(k)
		if r := []rune(n); len(r) > 0 {
			n = strings.ToLower(string(r[:1])) + string(r[1:])
		}
		worn = append(worn, n)
	}
	if len(worn) > 0 {
		parts = append(parts, strings.Join(worn, ", "))
	}
	if m.ArmorClassDesc != "" {
		parts = append(parts, m.ArmorClassDesc)
	}
	for _, a := range m.ArmorClassAlts {
		if a.Spell != "" {
			parts = append(parts, fmt.Sprintf("%d com %s", a.Value, c.namePT(a.Spell)))
		} else {
			parts = append(parts, fmt.Sprintf("%d se %s", a.Value, c.namePT(a.Condition)))
		}
	}
	return strings.Join(parts, "; ")
}

func monsterScores(m *srd51.Monster) map[Ability]int {
	return map[Ability]int{STR: m.Str, DEX: m.Dex, CON: m.Con, INT: m.Int, WIS: m.Wis, CHA: m.Cha}
}

// monsterSenses lists the stat block's special senses, in the order of the
// closed list of senses (darkvision first).
func (c *content) monsterSenses(m *srd51.Monster) []Sense {
	var out []Sense
	for _, s := range []struct {
		key string
		ft  int
	}{{"darkvision", m.Darkvision}, {"blindsight", m.Blindsight}, {"tremorsense", m.Tremorsense}, {"truesight", m.Truesight}} {
		if s.ft > 0 {
			out = append(out, Sense{Key: s.key, NamePT: c.namePT("sense:" + s.key), RangeFt: s.ft, Source: m.Key})
		}
	}
	return out
}

// MonsterDerived turns a creature's stat block into the Derived that combat
// reads, and says whether the key is a creature. Abilities and modifiers, saves
// and skills (the bonuses the stat block lists, the ability modifier for the
// rest), armor class, hit points, the speeds, the senses, the passive Perception,
// the attacks (each action with an attack bonus: the to-hit and the first damage
// part; the rest of the text in Attack.Notes), the Multiattack count as
// AttacksPerAction, and the actions with a saving throw in SaveActions.
// A creature casts no spells here.
func (c *Content) MonsterDerived(key string) (Derived, bool) {
	m, ok := c.c.monsters[key]
	if !ok {
		return Derived{}, false
	}
	return c.c.monsterDerived(m), true
}

func (c *content) monsterDerived(m *srd51.Monster) Derived {
	d := Derived{
		ContentVersion: c.version, ProficiencyBonus: m.ProficiencyBonus,
		ArmorClass: m.ArmorClass, ArmorClassDescription: armorClassNamePT[m.ArmorClassType],
		HitPointsMax: m.HitPoints,
		SpeedWalkFt:  m.Speed.Walk, SpeedFlyFt: m.Speed.Fly, SpeedSwimFt: m.Speed.Swim,
		SpeedClimbFt: m.Speed.Climb, SpeedBurrowFt: m.Speed.Burrow, Hover: m.Speed.Hover,
		Senses: c.monsterSenses(m), PassivePerception: m.PassivePerception,
		SpellSlots: make([]int, 9), AttacksPerAction: 1,
		StandardActions: slices.Clone(c.standardActions),
	}
	if note := c.armorClassNote(m); note != "" {
		d.ArmorClassDescription += " (" + note + ")"
	}
	if count, faces, ok := parseDice(m.HitDice); ok {
		d.HitDice = []HitDice{{Die: faces, Count: count}}
	}
	scores := monsterScores(m)
	mods := map[Ability]int{}
	for _, a := range AllAbilities() {
		mods[a] = modifier(scores[a])
		d.Abilities = append(d.Abilities, AbilityScore{
			Ability: a, NamePT: c.namePT(string(a)), Base: scores[a], Score: scores[a], Modifier: mods[a],
		})
		bonus, listed := m.Saves[string(a)]
		if !listed {
			bonus = mods[a]
		}
		d.SavingThrows = append(d.SavingThrows, SavingThrow{Ability: a, NamePT: c.namePT(string(a)), Proficient: listed, Bonus: bonus})
	}
	d.Initiative = mods[DEX]
	skill := map[string]int{}
	for _, key := range c.skillOrder {
		s := c.skills[key]
		bonus, listed := m.Skills[key]
		level := ProficiencyFull
		if !listed {
			bonus, level = mods[Ability(s.Ability)], ProficiencyNone
		}
		skill[key] = bonus
		d.Skills = append(d.Skills, Skill{Key: key, NamePT: c.namePT(key), Ability: Ability(s.Ability), Proficiency: level, Bonus: bonus})
	}
	slices.SortFunc(d.Skills, func(a, b Skill) int { return comparePT(a.NamePT, b.NamePT) })
	d.PassiveInvestigation = 10 + skill["skill:investigation"]
	d.PassiveInsight = 10 + skill["skill:insight"]

	byName := map[string]srd51.MonsterAction{}
	for _, a := range m.Actions {
		byName[a.Name] = a
	}
	for _, a := range m.Actions {
		actionKey := m.Key + "#" + slugOf(a.Name)
		if a.HasAttack {
			d.Attacks = append(d.Attacks, c.monsterAttack(m, a, actionKey, mods))
		}
		if a.Save != nil {
			d.SaveActions = append(d.SaveActions, SaveAction{
				Key: actionKey, Name: a.Name, Ability: Ability(a.Save.Ability), DC: a.Save.DC,
				OnSuccess: a.Save.OnSuccess, Usage: a.Usage, Text: a.Desc,
			})
		}
		for _, routine := range a.Multiattack {
			n := 0
			for _, x := range routine {
				if x.Kind == "melee" || x.Kind == "ranged" {
					n += x.Count
				}
			}
			d.AttacksPerAction = max(d.AttacksPerAction, n)
		}
	}
	// The snapshot's count, where the SRD's own text says another (corrections.json).
	if n, ok := c.attacksPerAction[m.Key]; ok {
		d.AttacksPerAction = n
	}
	for _, a := range m.SpecialAbilities {
		d.Features = append(d.Features, Feature{
			Key: m.Key + "#" + slugOf(a.Name), Name: a.Name, Source: m.Key, SourcePT: c.namePT(m.Key), Description: []string{a.Desc},
		})
	}
	return d
}

var (
	reachRe = regexp.MustCompile(`reach (\d+) ft\.`)
	rangeRe = regexp.MustCompile(`range (\d+)(?:/(\d+))? ft\.`)
	slugRe  = regexp.MustCompile(`[^a-z0-9]+`)
)

// slugOf writes an action's name as a key part: "Frightful Presence" is
// "frightful-presence".
func slugOf(name string) string {
	return strings.Trim(slugRe.ReplaceAllString(strings.ToLower(name), "-"), "-")
}

// monsterAttack reads an action with an attack bonus as an Attack: the to-hit,
// the first damage part, the reach or range from the text, and whether it is a
// melee attack. The rest of the action is in Notes.
func (c *content) monsterAttack(m *srd51.Monster, a srd51.MonsterAction, key string, mods map[Ability]int) Attack {
	melee := strings.HasPrefix(a.Desc, "Melee")
	at := Attack{
		Key: key, Name: a.Name, NamePT: cmp.Or(c.namesPT["attack:"+slugOf(a.Name)], a.Name), Kind: "weapon", AttackBonus: a.AttackBonus, Proficient: true,
		Melee: melee, Notes: a.Desc,
	}
	if strings.Contains(a.Desc, "Spell Attack") {
		at.Kind = "spell"
	}
	// The ability is the one whose modifier plus the proficiency bonus is the
	// to-hit; otherwise the usual one for the attack.
	at.Ability = STR
	if !melee {
		at.Ability = DEX
	}
	for _, ab := range []Ability{STR, DEX} {
		if mods[ab]+m.ProficiencyBonus == a.AttackBonus {
			at.Ability = ab
			break
		}
	}
	if mm := reachRe.FindStringSubmatch(a.Desc); mm != nil {
		at.RangeFt, _ = strconv.Atoi(mm[1])
	}
	// A thrown weapon ("reach 5 ft. or range 20/60 ft.") stays a melee attack and
	// carries its throwing range, as a character's thrown weapon does.
	if mm := rangeRe.FindStringSubmatch(a.Desc); mm != nil {
		at.RangeFt, _ = strconv.Atoi(mm[1])
		at.LongRangeFt, _ = strconv.Atoi(mm[2])
	}
	if len(a.Damage) > 0 {
		at.Damage = a.Damage[0].Dice
		at.DamageType = a.Damage[0].DamageType
		at.DamageTypeNamePT = c.namePT(a.Damage[0].DamageType)
		at.DamageDice, _ = ParseDice(at.Damage)
	}
	return at
}
