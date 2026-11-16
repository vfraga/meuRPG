package rules

import (
	"fmt"
	"slices"
	"strconv"

	"github.com/PuraFome/meuRPG/backend/internal/rules/srd51"
)

// Feats (MR-025): an optional rule of the game. The SRD 5.1 has one, Grappler
// (SRD 5.1, "Feats"); the table's own are added by the master (TableFeat). A
// character that takes a feat in place of an Ability Score Improvement keeps its
// key in Build.Feats, and Derive treats it like a feature: the feat's effects
// apply and the sheet lists it.
//
// What a feat asks of the character is its prerequisite (FeatPrerequisite). The
// SRD only states ability score minimums; the other conditions (a proficiency,
// spellcasting, a race, a level) are the table's.

// The kinds of FeatUnmet.
const (
	// FeatUnmetAbilityMinimum: an ability score is below the minimum, and every
	// minimum is asked (Abilities has the one that failed).
	FeatUnmetAbilityMinimum = "ability_minimum"
	// FeatUnmetAbilityAnyOf: none of the abilities reaches its minimum (Abilities
	// has all of them: one is enough).
	FeatUnmetAbilityAnyOf = "ability_any_of"
	// FeatUnmetProficiency: the character lacks the proficiency in Key.
	FeatUnmetProficiency = "proficiency"
	// FeatUnmetSpellcasting: the character cannot cast any spell.
	FeatUnmetSpellcasting = "spellcasting"
	// FeatUnmetRace: the character is not of the race or subrace in Key, or of any in Keys.
	FeatUnmetRace = "race"
	// FeatUnmetLevel: the character level is below Value.
	FeatUnmetLevel = "level"
	// FeatUnmetSize: the character's size is not the one in Key.
	FeatUnmetSize = "size"
	// FeatUnmetLacksFeature: the character has the feature in Key, which the feat asks
	// them not to have.
	FeatUnmetLacksFeature = "lacks_feature"
	// FeatUnmetClassLevel: the character has fewer levels than Value in the class in Key.
	FeatUnmetClassLevel = "class_level"
	// FeatUnmetPact: the character does not have the Pact Boon in Key.
	FeatUnmetPact = "pact"
	// FeatUnmetSpell: the character does not know the spell in Key.
	FeatUnmetSpell = "spell"
	// FeatUnmetOption: the character has not picked the option in Key.
	FeatUnmetOption = "option"
)

// FeatPrerequisite is what a feat asks of a character. Every condition that is set
// has to be met. The zero value asks nothing.
type FeatPrerequisite struct {
	// Minimums are ability scores that must all be met, and AnyOf ones of which a
	// single one is enough. A score of 0 is "not asked".
	Minimums map[Ability]int
	AnyOf    map[Ability]int
	// Proficiency is a proficiency key ("proficiency:medium-armor") the character
	// must have.
	Proficiency string
	// Spellcasting asks for the ability to cast at least one spell.
	Spellcasting bool
	// Race is a race or subrace key; the character's race or subrace must be it.
	Race string
	// Races are race or subrace keys of which the character's race or subrace must be one.
	// It is read together with Race (the table refuses a feat that sets both). A subrace
	// character is of its parent race too.
	Races []string `json:",omitempty"`
	// Size is the size the character must have ("Medium"); empty is not asked.
	Size string `json:",omitempty"`
	// LacksFeature is a feature or trait key the character must not have; empty is not asked.
	LacksFeature string `json:",omitempty"`
	// Level is the least total character level; 0 is not asked.
	Level int
	// ClassKey and ClassLevel ask for a level in one class (a table's too); both or neither.
	ClassKey   string `json:",omitempty"`
	ClassLevel int    `json:",omitempty"`
	// RequiresPact is a Pact Boon the character has, RequiresSpell a spell it knows and RequiresOptions
	// options it has picked (each one); empty asks nothing.
	RequiresPact    string   `json:",omitempty"`
	RequiresSpell   string   `json:",omitempty"`
	RequiresOptions []string `json:",omitempty"`
}

// IsZero says the prerequisite asks nothing.
func (p FeatPrerequisite) IsZero() bool {
	return len(p.Minimums) == 0 && len(p.AnyOf) == 0 && p.Proficiency == "" && !p.Spellcasting && p.Race == "" && len(p.Races) == 0 && p.Size == "" && p.LacksFeature == "" && p.Level == 0 &&
		p.ClassKey == "" && p.ClassLevel == 0 && p.RequiresPact == "" && p.RequiresSpell == "" && len(p.RequiresOptions) == 0
}

// AbilityMinimum is an ability and the score asked of it.
type AbilityMinimum struct {
	Ability Ability
	Minimum int
}

// FeatUnmet is one thing a character lacks to take a feat.
type FeatUnmet struct {
	// Kind is a FeatUnmet* constant.
	Kind string
	// Abilities are the minimums not met (see the kinds).
	Abilities []AbilityMinimum
	// Key is the proficiency, the race, the size or the feature, and Value the level.
	// Keys are the races when the feat lists several (Key is empty then).
	Key  string
	Keys []string `json:",omitempty"`
	// NamePT is the name of the feature in Key (kind lacks_feature).
	NamePT string `json:",omitempty"`
	Value  int
}

// FeatIncrease is the feat's ability_increase effect: the player chooses Count
// different abilities from From and adds Value to each, none above 20.
type FeatIncrease struct {
	Count int
	From  []Ability
	Value int
}

// FeatEntry is a feat as the screens show it.
type FeatEntry struct {
	Key string
	// Name is the SRD's English name (the table's feats have only NamePT).
	Name, NamePT string
	// Desc is the text: the SRD's, in English, or the table's, in Portuguese.
	Desc []string
	// DescPT is the Portuguese translation of an SRD feat's text, one paragraph per English
	// one; the table's own text for a table feat. Empty while DescPTMissing; DescPTOnly says
	// the text exists only in Portuguese (a table feat).
	DescPT                    []string
	DescPTMissing, DescPTOnly bool
	Prerequisite              FeatPrerequisite
	// LacksFeatureNamePT is the Portuguese name of Prerequisite.LacksFeature.
	LacksFeatureNamePT string `json:",omitempty"`
	// Increase is the ability increase the feat gives, or nil.
	Increase *FeatIncrease
	// Table says the feat is the table's own; Archived that the table retired it
	// and Off that the master switched it off for the players. Neither is a new
	// choice for a player.
	Table, Archived, Off bool
}

// FeatOption is a feat a character may take, with whether it qualifies and what
// it lacks when it does not.
type FeatOption struct {
	FeatEntry
	Qualifies bool
	Unmet     []FeatUnmet
	// Capped are the abilities of the feat's list that cannot take its increase because
	// they would pass 20 (SRD 5.1, Ability Score Improvement): a feat that lets the player
	// choose never offers them. A feat that raises every ability it lists is still taken: the
	// increase stops at 20 and its other effects apply.
	Capped []Ability
}

const (
	// abilityScoreImprovementPoints is what an Ability Score Improvement gives in all:
	// +2 to one ability or +1 to two (SRD 5.1, "Ability Score Improvement"). It is also the
	// most an ability_increase adds to each ability it raises.
	abilityScoreImprovementPoints = 2
	// abilityCount is the number of abilities: the most an ability_increase picks.
	abilityCount = 6
)

// featEntry is the FeatEntry of a feat in the content.
func (c *content) featEntry(f *srd51.Feat) FeatEntry {
	e := FeatEntry{
		Key: f.Key, Name: f.Name, NamePT: c.namePT(f.Key), Desc: slices.Clone(f.Desc),
		Prerequisite: FeatPrerequisite{
			Minimums: abilityMap(f.Minimums), AnyOf: abilityMap(f.AnyOf), Proficiency: f.Proficiency,
			Spellcasting: f.Spellcasting, Race: f.Race, Races: slices.Clone(f.Races), Size: f.Size, LacksFeature: f.LacksFeature, Level: f.Level,
			ClassKey: f.ClassKey, ClassLevel: f.ClassLevel, RequiresPact: f.RequiresPact, RequiresSpell: f.RequiresSpell, RequiresOptions: slices.Clone(f.RequiresOptions),
		},
		Table: isTableKey(f.Key), Archived: c.archived[f.Key], Off: c.off[f.Key],
	}
	if f.LacksFeature != "" {
		e.LacksFeatureNamePT = c.namePT(f.LacksFeature)
	}
	e.DescPT, e.DescPTMissing, e.DescPTOnly = c.textPT(f.Key, f.Desc)
	for _, ef := range c.effects[f.Key] {
		if ef.Type == "ability_increase" {
			inc := &FeatIncrease{Count: ef.Count, Value: increaseValue(ef)}
			for _, a := range ef.From {
				inc.From = append(inc.From, Ability(a))
			}
			e.Increase = inc
		}
	}
	return e
}

// increaseValue is the whole number an ability_increase adds (its Value is a
// plain integer; the loader refuses anything else).
func increaseValue(e *Effect) int {
	n, _ := strconv.Atoi(e.Value)
	return n
}

// Feats lists every feat of the content, the SRD's and the table's, sorted by
// Portuguese name.
func (c *Content) Feats() []FeatEntry {
	x := c.c
	out := make([]FeatEntry, 0, len(x.feats))
	for _, k := range sortedKeys(x.feats) {
		out = append(out, x.featEntry(x.feats[k]))
	}
	slices.SortStableFunc(out, func(a, b FeatEntry) int { return comparePT(a.NamePT, b.NamePT) })
	return out
}

// Feat gives one feat of the content by key.
func (c *Content) Feat(key string) (FeatEntry, bool) {
	f, ok := c.c.feats[key]
	if !ok {
		return FeatEntry{}, false
	}
	return c.c.featEntry(f), true
}

// FeatOptions lists the feats a character may take now, each with whether it
// qualifies and, if not, what it lacks. The feats the build already has are left
// out. Retired and switched-off feats are in the list with their marks: the
// caller (the server) leaves them out for a player.
func FeatOptions(b Build, c *Content) []FeatOption { return featOptions(b, c.c) }

func featOptions(b Build, x *content) []FeatOption {
	d := derive(b, x)
	out := make([]FeatOption, 0, len(x.feats))
	for _, e := range (&Content{c: x}).Feats() {
		// A feat the sheet already has, from the list or from a choice, is not offered again.
		if slices.ContainsFunc(d.Features, func(f Feature) bool { return f.Key == e.Key }) {
			continue
		}
		if x.asksOnlyHiddenRaces(e.Prerequisite, b) {
			continue // a player's options do not hint at a feat only a hidden race can take
		}
		unmet := x.unmetPrerequisite(e, b, d)
		out = append(out, FeatOption{FeatEntry: e, Qualifies: len(unmet) == 0, Unmet: unmet, Capped: cappedAbilities(e.Increase, d)})
	}
	return out
}

// CheckFeat says what a character lacks to take the feat key, or nil when it may.
// An unknown key is an error.
func CheckFeat(b Build, key string, c *Content) ([]FeatUnmet, error) {
	e, ok := c.Feat(key)
	if !ok {
		return nil, fmt.Errorf("rules: unknown feat %q", key)
	}
	return c.c.unmetPrerequisite(e, b, derive(b, c.c)), nil
}

// lostFeats are the feats the build has and whose prerequisite the derived sheet no
// longer meets (the ability increase of a feat already taken is not asked again).
func (c *content) lostFeats(b Build, d Derived) map[string]*LostPrerequisite {
	var lost map[string]*LostPrerequisite
	keys := slices.Clone(b.Feats)
	for _, cf := range c.pickedChoiceFeats(b, ownedBy(b, d)) {
		keys = append(keys, cf.Feat)
	}
	for _, key := range keys {
		f, ok := c.feats[key]
		if !ok {
			continue
		}
		unmet := c.unmetPrerequisite(c.featEntry(f), b, d)
		if len(unmet) == 0 {
			continue
		}
		if lost == nil {
			lost = map[string]*LostPrerequisite{}
		}
		lost[key] = c.lost(key, false, unmet, b, d)
	}
	return lost
}

// cappedAbilities are the abilities of an increase's list that would pass 20 with it.
func cappedAbilities(inc *FeatIncrease, d Derived) []Ability {
	if inc == nil {
		return nil
	}
	var out []Ability
	for _, s := range d.Abilities {
		if slices.Contains(inc.From, s.Ability) && s.Score+inc.Value > MaxNormalScore {
			out = append(out, s.Ability)
		}
	}
	return out
}

// unmetPrerequisite is what the character lacks of the feat's prerequisite.
func (c *content) unmetPrerequisite(e FeatEntry, b Build, d Derived) []FeatUnmet {
	p := e.Prerequisite
	score := map[Ability]int{}
	for _, s := range d.Abilities {
		score[s.Ability] = s.Score
	}
	out := unmetMinimums(p, score)
	if u, unmet := unmetAnyOf(p, score); unmet {
		out = append(out, u)
	}
	return append(out, c.unmetOthers(p, b, d)...)
}

// unmetMinimums are the abilities under their minimum, one entry each, in the
// order of the abilities.
func unmetMinimums(p FeatPrerequisite, score map[Ability]int) []FeatUnmet {
	var out []FeatUnmet
	for _, a := range AllAbilities() {
		if least := p.Minimums[a]; least > 0 && score[a] < least {
			out = append(out, FeatUnmet{Kind: FeatUnmetAbilityMinimum, Abilities: []AbilityMinimum{{Ability: a, Minimum: least}}})
		}
	}
	return out
}

// unmetAnyOf is the "one of these abilities" part, unmet when no ability reaches
// its minimum. A list of zeros is never met.
func unmetAnyOf(p FeatPrerequisite, score map[Ability]int) (FeatUnmet, bool) {
	if len(p.AnyOf) == 0 {
		return FeatUnmet{}, false
	}
	met := false
	var all []AbilityMinimum
	for _, a := range AllAbilities() {
		if least := p.AnyOf[a]; least > 0 {
			all = append(all, AbilityMinimum{Ability: a, Minimum: least})
			met = met || score[a] >= least
		}
	}
	return FeatUnmet{Kind: FeatUnmetAbilityAnyOf, Abilities: all}, !met
}

// unmetOthers are the proficiency, the spellcasting, the race, the size, the feature
// and the level the character lacks, in that order.
func (c *content) unmetOthers(p FeatPrerequisite, b Build, d Derived) []FeatUnmet {
	var out []FeatUnmet
	if p.Proficiency != "" && !hasProficiency(d, p.Proficiency) {
		out = append(out, FeatUnmet{Kind: FeatUnmetProficiency, Key: p.Proficiency})
	}
	if p.Spellcasting && len(d.Spellcasting) == 0 && d.PactMagic == nil && len(d.Spells) == 0 {
		out = append(out, FeatUnmet{Kind: FeatUnmetSpellcasting})
	}
	if u, unmet := unmetRace(p, b); unmet {
		out = append(out, u)
	}
	if p.Size != "" && d.Size != p.Size {
		out = append(out, FeatUnmet{Kind: FeatUnmetSize, Key: p.Size})
	}
	if p.LacksFeature != "" && ownedBy(b, d)(p.LacksFeature) {
		out = append(out, FeatUnmet{Kind: FeatUnmetLacksFeature, Key: p.LacksFeature, NamePT: c.namePT(p.LacksFeature)})
	}
	if p.Level > 0 && d.TotalLevel < p.Level {
		out = append(out, FeatUnmet{Kind: FeatUnmetLevel, Value: p.Level})
	}
	return append(out, c.unmetKnowledge(p, b, d)...)
}

// unmetKnowledge are the class level, the Pact Boon, the spell and the options a feat asks for that the
// character lacks, in that order. The sheet's own features say what it has picked: a Pact Boon and an option are
// features of the sheet.
func (c *content) unmetKnowledge(p FeatPrerequisite, b Build, d Derived) []FeatUnmet {
	var out []FeatUnmet
	if p.ClassKey != "" {
		if classLevelIn(b.Classes, p.ClassKey) < p.ClassLevel {
			out = append(out, FeatUnmet{Kind: FeatUnmetClassLevel, Key: p.ClassKey, NamePT: c.namePT(p.ClassKey), Value: p.ClassLevel})
		}
	}
	owns := ownedBy(b, d)
	if p.RequiresPact != "" && !owns(p.RequiresPact) {
		out = append(out, FeatUnmet{Kind: FeatUnmetPact, Key: p.RequiresPact, NamePT: c.namePT(p.RequiresPact)})
	}
	if k := p.RequiresSpell; k != "" && !knowsSpell(d, k) {
		out = append(out, FeatUnmet{Kind: FeatUnmetSpell, Key: k, NamePT: c.namePT(k)})
	}
	for _, k := range p.RequiresOptions {
		if !owns(k) {
			out = append(out, FeatUnmet{Kind: FeatUnmetOption, Key: k, NamePT: optionLabel(c.namePT(k))})
		}
	}
	return out
}

// classLevelIn is the character's level in one class: the sum of its entries, as a sheet may list a class twice.
// The feat and the option prerequisites read it the same way.
func classLevelIn(classes []ClassLevel, key string) int {
	have := 0
	for _, cl := range classes {
		if cl.Class == key {
			have += cl.Level
		}
	}
	return have
}

// knowsSpell says whether the sheet knows a spell: its cantrips, known, prepared and always-prepared spells, and a
// granted spell that counts as known (a race's or a feat's). A spell cast from an item does not count. The feat and the
// option prerequisites read it the same way.
func knowsSpell(d Derived, key string) bool {
	return slices.ContainsFunc(d.Spells, func(s CharacterSpell) bool { return s.Spell.Key == key })
}

// prerequisiteRaces are the races and subraces the prerequisite asks for: Race and Races
// together, each once.
func prerequisiteRaces(p FeatPrerequisite) []string {
	var out []string
	if p.Race != "" {
		out = append(out, p.Race)
	}
	for _, r := range p.Races {
		if !slices.Contains(out, r) {
			out = append(out, r)
		}
	}
	return out
}

// unmetRace is the race part, unmet when the character's race (a subrace's parent
// included) and its subrace are none of the races asked.
func unmetRace(p FeatPrerequisite, b Build) (FeatUnmet, bool) {
	asked := prerequisiteRaces(p)
	if len(asked) == 0 || slices.Contains(asked, b.Race) || slices.Contains(asked, b.Subrace) {
		return FeatUnmet{}, false
	}
	if len(asked) == 1 {
		return FeatUnmet{Kind: FeatUnmetRace, Key: asked[0]}, true
	}
	return FeatUnmet{Kind: FeatUnmetRace, Keys: asked}, true
}

// hasProficiency says whether the sheet has the proficiency key, or one that
// covers it: "All armor" (the Fighter's) is proficiency in light, medium and heavy
// armor.
func hasProficiency(d Derived, key string) bool {
	have := func(k string) bool {
		return slices.ContainsFunc(d.Proficiencies, func(pr Proficiency) bool { return pr.Key == k })
	}
	switch key {
	case "proficiency:light-armor", "proficiency:medium-armor", "proficiency:heavy-armor":
		return have(key) || have("proficiency:all-armor")
	}
	return have(key)
}

// checkFeatShape checks a feat's own data: the abilities of its prerequisite are
// real, the keys it names exist and the numbers are in range. owner is the feat's
// key, for the messages; it returns the first problem and the field it is at.
func (c *content) checkFeatShape(f *srd51.Feat) (field, msg string) {
	if field, msg = checkPrerequisiteScores("minimums", f.Minimums); field != "" {
		return field, msg
	}
	if field, msg = checkPrerequisiteScores("any_of", f.AnyOf); field != "" {
		return field, msg
	}
	if field, msg = c.checkFeatKeys(f); field != "" {
		return field, msg
	}
	if f.Level < 0 || f.Level > MaxLevel {
		return "level", fmt.Sprintf("the level is 0 to %d", MaxLevel)
	}
	return "", ""
}

// checkPrerequisiteScores checks one set of minimum scores of a prerequisite, named
// name: real abilities, each 1 to MaxScore.
func checkPrerequisiteScores(name string, set map[string]int) (field, msg string) {
	for k, v := range set {
		if _, ok := abilityIndex[Ability(k)]; !ok {
			return name, fmt.Sprintf("unknown ability %q", k)
		}
		if v < 1 || v > MaxScore {
			return name, fmt.Sprintf("the minimum score of %s is 1 to %d", k, MaxScore)
		}
	}
	return "", ""
}

// checkFeatKeys checks the proficiency, the races (or subraces), the size and the feature a
// prerequisite names exist.
func (c *content) checkFeatKeys(f *srd51.Feat) (field, msg string) {
	if f.Proficiency != "" {
		if _, ok := c.proficiencies[f.Proficiency]; !ok {
			return "proficiency", fmt.Sprintf("unknown proficiency %q", f.Proficiency)
		}
	}
	if r, ok := c.unknownRace(append([]string{f.Race}, f.Races...)); !ok {
		return "race", fmt.Sprintf("unknown race or subrace %q", r)
	}
	if f.Size != "" && !slices.Contains(raceSizes, f.Size) {
		return "size", fmt.Sprintf("unknown size %q", f.Size)
	}
	if k := f.LacksFeature; k != "" && !c.isFeatureKey(k) {
		return "lacks_feature", fmt.Sprintf("unknown feature %q", k)
	}
	return "", ""
}

// unknownRace gives the first key (an empty one is skipped) that is neither a race nor a
// subrace of the content, and false; true when all exist.
func (c *content) unknownRace(keys []string) (string, bool) {
	for _, r := range keys {
		_, race := c.races[r]
		_, subrace := c.subraces[r]
		if r != "" && !race && !subrace {
			return r, false
		}
	}
	return "", true
}
