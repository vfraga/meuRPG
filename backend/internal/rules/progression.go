package rules

import (
	"fmt"
	"io/fs"
	"slices"
	"strconv"
	"strings"
)

// advancement is effects/advancement.json: the SRD's XP tables (RN-09,
// RN-12). The master's XP awards and the "Pode subir de nível" tag read
// them through Content.
type advancement struct {
	Comment string `json:"_comment"`
	LevelXP []int  `json:"level_xp"`
	Ratings []struct {
		Rating string `json:"rating"`
		XP     int    `json:"xp"`
	} `json:"challenge_ratings"`
}

// ChallengeRating is a challenge rating ("ND") and the XP a creature of
// that rating gives when defeated.
type ChallengeRating struct {
	// Rating is "0", "1/8", "1/4", "1/2", "1" ... "30".
	Rating string
	XP     int
}

// ratingOrder is the rating of each row of the SRD table, in order. The
// file must list exactly these, so a typo cannot slip in.
var ratingOrder = func() []string {
	r := []string{"0", "1/8", "1/4", "1/2"}
	for n := 1; n <= 30; n++ {
		r = append(r, strconv.Itoa(n))
	}
	return r
}()

// loadAdvancement reads effects/advancement.json and refuses a table that
// is not the SRD's shape: 20 levels strictly increasing from 0, and the 34
// ratings in order with growing XP.
func (c *content) loadAdvancement(fsys fs.FS) error {
	const name = "effects/advancement.json"
	var f advancement
	if err := readJSON(fsys, name, &f); err != nil {
		return err
	}
	if len(f.LevelXP) != MaxLevel {
		return fmt.Errorf("%s: level_xp needs %d entries, got %d", name, MaxLevel, len(f.LevelXP))
	}
	if f.LevelXP[0] != 0 {
		return fmt.Errorf("%s: level 1 needs 0 XP", name)
	}
	for i := 1; i < len(f.LevelXP); i++ {
		if f.LevelXP[i] <= f.LevelXP[i-1] {
			return fmt.Errorf("%s: level_xp must strictly increase (level %d)", name, i+1)
		}
	}
	if len(f.Ratings) != len(ratingOrder) {
		return fmt.Errorf("%s: challenge_ratings needs %d entries, got %d", name, len(ratingOrder), len(f.Ratings))
	}
	for i, r := range f.Ratings {
		if r.Rating != ratingOrder[i] {
			return fmt.Errorf("%s: challenge_ratings[%d] must be %q, got %q", name, i, ratingOrder[i], r.Rating)
		}
		if r.XP < 0 || (i > 0 && r.XP <= f.Ratings[i-1].XP) {
			return fmt.Errorf("%s: challenge_ratings[%d] (%s): XP must be positive and increasing", name, i, r.Rating)
		}
		c.ratings = append(c.ratings, ChallengeRating{Rating: r.Rating, XP: r.XP})
	}
	c.levelXP = f.LevelXP
	return nil
}

// NextLevelXP is the XP a character needs to reach level+1, and false at
// level 20 (or for a level outside 1..20).
func (c *Content) NextLevelXP(level int) (int, bool) {
	return c.c.nextLevelXP(level)
}

func (c *content) nextLevelXP(level int) (int, bool) {
	if level < 1 || level >= len(c.levelXP) {
		return 0, false
	}
	return c.levelXP[level], true
}

// LevelForXP is the highest level (1..20) whose XP the character has
// reached. Negative XP counts as 0.
func (c *Content) LevelForXP(xp int) int {
	// levelXP[0] is 0, so the search always finds level 1 at least.
	i, found := slices.BinarySearch(c.c.levelXP, xp)
	if found {
		return i + 1
	}
	return max(i, 1)
}

// XPForChallenge is the XP of a challenge rating written as "0", "1/8",
// "1/4", "1/2" or "1" to "30". It is false for anything else. A rating 0
// creature gives 10 here; the master types 0 for one with no attack.
func (c *Content) XPForChallenge(rating string) (int, bool) {
	for _, r := range c.c.ratings {
		if r.Rating == rating {
			return r.XP, true
		}
	}
	return 0, false
}

// ChallengeRatings lists the SRD's 34 ratings with their XP, in order, for
// the editor's "ND" picker. The caller gets a copy.
func (c *Content) ChallengeRatings() []ChallengeRating {
	return slices.Clone(c.c.ratings)
}

// SplitXP is each character's share of total XP among n characters,
// rounded down like the book does (question 46): 100 among 3 is 33 each.
// It is 0 when n or total is not positive.
func SplitXP(total, n int) int {
	if total <= 0 || n <= 0 {
		return 0
	}
	return total / n
}

// Kinds of scene action. The set is closed: nothing that belongs to the
// combat (attacks, spells, combat features) can be a scene action (MR-015).
const (
	SceneSkill   = "skill"
	SceneAbility = "ability"
	SceneSave    = "save"
)

// SceneAction is a check the master put on a scene.
type SceneAction struct {
	// Key is "skill:investigation", "ability:str" or "save:wis".
	Key string
	// Name is what the master called it ("Convencer o guarda"), or empty.
	Name string
	// DC is the difficulty class, or 0 for none. Only the master sees it.
	DC int
}

// SceneOption is a SceneAction with one character's numbers on it.
type SceneOption struct {
	Action SceneAction
	// Kind is SceneSkill, SceneAbility or SceneSave.
	Kind string
	// NamePT is the check's name: "Investigação", "Teste de Força",
	// "Teste de resistência de Sabedoria".
	NamePT string
	// Bonus is what the character adds to the d20.
	Bonus int
	// Passive is the sheet's passive value for Percepção, Investigação and
	// Intuição (10 + the bonus, plus whatever the sheet adds), and
	// HasPassive says whether there is one.
	Passive    int
	HasPassive bool
	// ReliableTalent says a d20 of this check counts as at least 10: the
	// character has Reliable Talent and the check adds the proficiency bonus
	// (a skill the sheet is proficient in, expertise included).
	ReliableTalent bool
}

// SceneError is an action whose key is not a skill, ability or save the
// sheet knows. Index is the action's place in the list.
type SceneError struct {
	Index int
	Key   string
}

func (e *SceneError) Error() string {
	return fmt.Sprintf("scene action %d: unknown check %q", e.Index, e.Key)
}

// IsSkill says whether key is one of the SRD's 18 skills ("skill:arcana").
func (c *Content) IsSkill(key string) bool {
	if !strings.HasPrefix(key, SceneSkill+":") {
		return false
	}
	_, ok := c.SceneCheckName(key)
	return ok
}

// SceneCheckName is the Portuguese name of a scene check by its key
// ("skill:investigation" is "Investigação", "ability:str" is "Teste de
// Força", "save:wis" is "Teste de resistência de Sabedoria"), and false for any
// other key: the server uses it to refuse an action that is not a skill,
// an ability check or a saving throw (MR-015), and to name the actions
// for the master, who has no character's numbers on the screen.
func (c *Content) SceneCheckName(key string) (string, bool) {
	kind, rest, _ := strings.Cut(key, ":")
	cat := c.c.catalog
	switch kind {
	case SceneSkill:
		for _, s := range cat.Skills {
			if s.Key == key {
				return s.NamePT, true
			}
		}
	case SceneAbility, SceneSave:
		for _, a := range cat.Abilities {
			if string(a.Ability) == rest {
				if kind == SceneAbility {
					return "Teste de " + a.NamePT, true
				}
				return "Teste de resistência de " + a.NamePT, true
			}
		}
	}
	return "", false
}

// SceneOptions is what a player sees of a scene: each action with their own
// bonus, from their derived sheet. It stops at the first unknown key with a
// *SceneError (the server checks keys when the master saves, so that is a
// stale scene, never a crash).
func SceneOptions(d Derived, actions []SceneAction) ([]SceneOption, error) {
	out := make([]SceneOption, 0, len(actions))
	for i, a := range actions {
		kind, key, _ := strings.Cut(a.Key, ":")
		o := SceneOption{Action: a, Kind: kind}
		ok := false
		switch kind {
		case SceneSkill:
			ok = sceneSkillOption(d, &o)
		case SceneAbility:
			ok = sceneAbilityOption(d, key, &o)
		case SceneSave:
			ok = sceneSaveOption(d, key, &o)
		}
		if !ok {
			return nil, &SceneError{Index: i, Key: a.Key}
		}
		out = append(out, o)
	}
	return out, nil
}

// sceneSkillOption fills a skill check with the skill's name and bonus, Reliable
// Talent and the passive score of the three skills that have one; false for a
// skill the sheet does not list.
func sceneSkillOption(d Derived, o *SceneOption) bool {
	ok := false
	for _, s := range d.Skills {
		if s.Key == o.Action.Key {
			o.NamePT, o.Bonus, ok = s.NamePT, s.Bonus, true
			o.ReliableTalent = d.ReliableTalent && (s.Proficiency == ProficiencyFull || s.Proficiency == ProficiencyExpertise)
			if passive, has := scenePassive(d, s.Key); has {
				o.Passive, o.HasPassive = passive, true
			}
		}
	}
	return ok
}

// scenePassive is the passive score of a skill, for Perception, Investigation and
// Insight.
func scenePassive(d Derived, skillKey string) (int, bool) {
	switch strings.TrimPrefix(skillKey, SceneSkill+":") {
	case "perception":
		return d.PassivePerception, true
	case "investigation":
		return d.PassiveInvestigation, true
	case "insight":
		return d.PassiveInsight, true
	}
	return 0, false
}

// sceneAbilityOption fills a raw ability check; false for an unknown ability.
func sceneAbilityOption(d Derived, key string, o *SceneOption) bool {
	ok := false
	for _, s := range d.Abilities {
		if string(s.Ability) == key {
			o.NamePT, o.Bonus, ok = "Teste de "+s.NamePT, s.Modifier, true
			// A raw ability check includes no proficiency: Jack of All Trades adds half
			// the bonus, rounded down (SRD 5.1, Bard).
			if HasFeature(d, "feature:jack-of-all-trades") {
				o.Bonus += d.ProficiencyBonus / 2
			}
		}
	}
	return ok
}

// sceneSaveOption fills a saving throw; false for an unknown ability.
func sceneSaveOption(d Derived, key string, o *SceneOption) bool {
	ok := false
	for _, s := range d.SavingThrows {
		if string(s.Ability) == key {
			o.NamePT, o.Bonus, ok = "Teste de resistência de "+s.NamePT, s.Bonus, true
		}
	}
	return ok
}
