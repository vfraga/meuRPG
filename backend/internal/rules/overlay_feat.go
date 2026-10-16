package rules

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/PuraFome/meuRPG/backend/internal/rules/srd51"
)

// TableFeat is a feat of the table (MR-025): a name, a text, what it asks of the
// character and its effects, from the same closed menu as a feature's, plus the
// ability_increase only a feat has. Its Key is "feat:<slug>@mesa". The effects
// belong to the feat itself (no feature of their own): a character that took the
// feat has them.
type TableFeat struct {
	TableEntry
	DescPT       []string
	Prerequisite FeatPrerequisite
	Effects      []Effect
}

// addFeat registers a feat and its effects, which are compiled later with the
// others. Every field that is wrong is reported at its own path.
func (b *overlayBuilder) addFeat(tf *TableFeat, path string) error {
	n := b.n
	key := tf.Key
	var c entryErrors
	checkTextAt(&c, key, path, ".desc_pt", tf.DescPT)
	feat := &srd51.Feat{
		Key: key, Name: tf.NamePT, Desc: slices.Clone(tf.DescPT),
		Minimums: abilityStrings(tf.Prerequisite.Minimums), AnyOf: abilityStrings(tf.Prerequisite.AnyOf),
		Proficiency: tf.Prerequisite.Proficiency, Spellcasting: tf.Prerequisite.Spellcasting, Race: tf.Prerequisite.Race, Level: tf.Prerequisite.Level,
	}
	b.checkFeatPrerequisite(&c, feat, key, path)
	increases := 0
	for i := range tf.Effects {
		if tf.Effects[i].Type == "ability_increase" {
			increases++
			if increases > 1 {
				c.at(key, path, fmt.Sprintf(".effects[%d]", i), ReasonLimit, "a feat has one ability increase")
			}
		}
	}
	// The effects are queued before the entry is judged: if it is refused, its effects'
	// problems come with the rest (compileEffectsOf).
	if len(tf.Effects) > 0 {
		b.pending = append(b.pending, pendingEffects{owner: key, entry: key, effects: tf.Effects, path: path, strict: b.strict[key]})
	}
	if err := c.err(); err != nil {
		return err
	}
	b.register(tf.TableEntry)
	n.feats[key] = feat
	return nil
}

// checkFeatPrerequisite reports what is wrong with a feat's prerequisite, each
// problem at its own field. The keys it names (a proficiency, a race or subrace)
// may be the SRD's or the table's.
func (b *overlayBuilder) checkFeatPrerequisite(c *entryErrors, f *srd51.Feat, key, path string) {
	checkFeatScores(c, key, path, ".prerequisite.minimums", f.Minimums)
	checkFeatScores(c, key, path, ".prerequisite.any_of", f.AnyOf)
	if p := f.Proficiency; p != "" {
		if _, ok := b.base.proficiencies[p]; !ok {
			c.at(key, path, ".prerequisite.proficiency_key", ReasonReference, "%q is not a proficiency of the SRD", p)
		}
	}
	if r := f.Race; r != "" && !b.isRace(r) && !b.isSubrace(r) {
		c.at(key, path, ".prerequisite.race_key", ReasonReference, "the race or subrace %q does not exist", r)
	}
	if f.Level < 0 || f.Level > MaxLevel {
		c.at(key, path, ".prerequisite.level", ReasonValue, "the level is 0 to %d", MaxLevel)
	}
}

// checkFeatScores checks the ability scores a feat asks for, in the order of
// the abilities' names; attr is the set's field.
func checkFeatScores(c *entryErrors, key, path, attr string, set map[string]int) {
	for _, a := range sortedKeys(set) {
		v := set[a]
		if _, ok := abilityIndex[Ability(a)]; !ok {
			c.at(key, path, attr, ReasonValue, "unknown ability %q", a)
		} else if v < 1 || v > MaxScore {
			c.at(key, path, attr+"."+abilityField[Ability(a)], ReasonValue, "the minimum score is 1 to %d", MaxScore)
		}
	}
}

func (b *overlayBuilder) isSubrace(key string) bool {
	_, ok := b.base.subraces[key]
	return ok || (b.entries[key] && strings.HasPrefix(key, "subrace:"))
}

// abilityStrings turns a map of abilities into the SRD data's keys, dropping the
// zeros ("not asked").
func abilityStrings(m map[Ability]int) map[string]int {
	var out map[string]int
	for a, v := range m {
		if v == 0 {
			continue
		}
		if out == nil {
			out = map[string]int{}
		}
		out[string(a)] = v
	}
	return out
}

// checkAbilityIncrease is the closed shape of an ability_increase: a count of
// different abilities the player picks from a list, and the same amount (1 or 2,
// as an Ability Score Improvement gives) added to each. It returns the field the
// first problem is at, or "".
func checkAbilityIncrease(e *Effect) (attr, msg string) {
	switch {
	case len(e.From) == 0:
		return ".from", "an ability_increase lists the abilities the player picks from"
	case e.Count < 1 || e.Count > len(e.From):
		return ".count", fmt.Sprintf("an ability_increase picks 1 to the %d abilities it lists", len(e.From))
	case e.Value != "1" && e.Value != strconv.Itoa(abilityScoreImprovementPoints):
		return ".value", "an ability_increase adds 1 or 2 to each ability"
	}
	for i, a := range e.From {
		if _, ok := abilityIndex[Ability(a)]; !ok {
			return ".from", fmt.Sprintf("unknown ability %q in an ability_increase", a)
		}
		if slices.Contains(e.From[:i], a) {
			return ".from", fmt.Sprintf("the ability %q is listed twice in an ability_increase", a)
		}
	}
	return "", ""
}
