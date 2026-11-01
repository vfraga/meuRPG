package rules

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Spells that summon creatures (MR-037): the "summon" kind of
// effects/spells.json. Four SRD spells take it: Find Familiar, Animate Dead,
// Conjure Animals and the warlock's Pact of the Chain, which widens Find
// Familiar: it adds forms, and every form of the familiar then attacks with its
// reaction. Find Steed is out (mounted combat is outside the MVP). The loader checks the kind against the creatures; SummonOptions says
// what a cast can choose, and CheckSummon refuses a choice the spell does not
// allow. The play module spends the slot and creates the creatures.

// What a summoned creature may do on its own, as in the Attack field of a
// SummonForm.
const (
	// SummonAttackNone: it can't attack (a familiar).
	SummonAttackNone = "none"
	// SummonAttackReaction: it attacks only with its reaction (any familiar of a
	// warlock with the Pact of the Chain).
	SummonAttackReaction = "reaction"
	// SummonAttackFull: it acts in combat as any creature.
	SummonAttackFull = "full"
)

var summonAttacks = []string{SummonAttackNone, SummonAttackReaction, SummonAttackFull}

// Errors CheckSummon returns; test them with errors.Is.
var (
	// ErrNotSummonSpell: the spell does not summon creatures.
	ErrNotSummonSpell = errors.New("rules: not a summoning spell")
	// ErrSummonCircle: the slot's circle is below the spell's own.
	ErrSummonCircle = errors.New("rules: the slot is below the spell's circle")
	// ErrSummonOption: the option does not exist.
	ErrSummonOption = errors.New("rules: unknown summon option")
	// ErrSummonCount: the wrong number of creatures for the option and the slot.
	ErrSummonCount = errors.New("rules: wrong number of creatures")
	// ErrSummonCreature: a creature the spell does not allow here.
	ErrSummonCreature = errors.New("rules: creature not allowed")
)

// SummonForm is a creature a cast may choose, and what it may do.
type SummonForm struct {
	Key string
	// Attack is SummonAttackNone, SummonAttackReaction or SummonAttackFull.
	Attack string
}

// SummonOption is one thing a cast may summon: Count creatures, each one of
// Forms, or, when Forms is empty, any creature of Type whose challenge rating is
// MaxCR or lower (Conjure Animals: beasts). Attack is what a creature chosen
// by Type and MaxCR may do.
type SummonOption struct {
	Count  int
	Forms  []SummonForm
	Type   string
	MaxCR  string
	Attack string
}

// SummonChoices is what casting a summoning spell with a slot may choose.
type SummonChoices struct {
	Spell string
	// Circle is the slot's circle.
	Circle int
	// Ritual, Concentration and CastingTime come from the spell's data: Find
	// Familiar is a ritual that takes 1 hour, Animate Dead takes 1 minute,
	// Conjure Animals takes 1 action and needs concentration.
	Ritual        bool
	Concentration bool
	CastingTime   CastingTime
	// Options are the choices; the creatures are picked from one of them. A
	// spell with a single kind of summons (Find Familiar) has one.
	Options []SummonOption
}

// SummonPick is a cast's choice: the option's index and the creatures, one
// key per creature (a key may repeat: four wolves).
type SummonPick struct {
	Option    int
	Creatures []string
}

// SummonedCreature is a creature a cast creates, with what it may do.
type SummonedCreature struct {
	Key    string
	Attack string
}

// summonDef is a checked "summon" entry of effects/spells.json.
type summonDef struct {
	spellLevel int
	creatures  []string
	unlocks    []summonUnlock
	count      int
	perLevel   int
	attack     string
	// typ and options: Conjure Animals' bands.
	typ     string
	options []summonBand
	// multipliers are the circles at which the counts grow, ascending.
	multipliers []summonMultiplier
}

type summonUnlock struct {
	feature   string
	creatures []string
	attack    string
}

type summonBand struct {
	count int
	maxCR string
}

type summonMultiplier struct{ circle, times int }

type summonUnlockJSON struct {
	Feature   string   `json:"feature"`
	Creatures []string `json:"creatures"`
	Attack    string   `json:"attack"`
}

type summonBandJSON struct {
	Count int    `json:"count"`
	MaxCR string `json:"max_cr"`
}

// summonJSON are the summon fields of a spell entry of effects/spells.json.
type summonJSON struct {
	Creatures         []string           `json:"creatures,omitempty"`
	Unlocks           []summonUnlockJSON `json:"unlocks,omitempty"`
	Count             int                `json:"count,omitempty"`
	CountPerLevel     int                `json:"count_per_level,omitempty"`
	Attack            string             `json:"attack,omitempty"`
	Type              string             `json:"type,omitempty"`
	Options           []summonBandJSON   `json:"options,omitempty"`
	MultiplierAtLevel map[string]int     `json:"multiplier_at_level,omitempty"`
}

func (s *summonJSON) empty() bool {
	return len(s.Creatures) == 0 && len(s.Unlocks) == 0 && s.Count == 0 && s.CountPerLevel == 0 &&
		s.Attack == "" && s.Type == "" && len(s.Options) == 0 && len(s.MultiplierAtLevel) == 0
}

// minSummonMultiplier is the smallest number a multiplier_at_level may give.
const minSummonMultiplier = 2

// summonFail builds the error of a summon entry's field.
type summonFail func(string, ...any) error

// loadSummon checks a summon entry and stores it.
func (c *content) loadSummon(key string, spellLevel int, in *summonJSON, fail func(string, ...any) error) error {
	def := &summonDef{spellLevel: spellLevel, creatures: slices.Clone(in.Creatures), count: in.Count, perLevel: in.CountPerLevel, attack: in.Attack, typ: in.Type}
	if !slices.Contains(summonAttacks, in.Attack) {
		return fail("attack must be one of %v", summonAttacks)
	}
	byList := len(in.Creatures) > 0
	byBand := len(in.Options) > 0
	if byList == byBand {
		return fail("a summon takes either creatures or options")
	}
	var err error
	if byList {
		err = c.loadSummonList(def, in, fail)
	} else {
		err = loadSummonBands(def, spellLevel, in, fail)
	}
	if err != nil {
		return err
	}
	if c.summons == nil {
		c.summons = map[string]*summonDef{}
	}
	c.summons[key] = def
	return nil
}

// loadSummonList checks a summon of listed creatures and the creatures that
// features unlock: each a creature of the content, none listed twice.
func (c *content) loadSummonList(def *summonDef, in *summonJSON, fail summonFail) error {
	if in.Count < 1 || in.CountPerLevel < 0 || in.Type != "" || len(in.MultiplierAtLevel) != 0 {
		return fail("a summon of creatures takes a count of at least 1 and a count_per_level, nothing else")
	}
	seen := map[string]bool{}
	if err := c.checkSummonCreatures(in.Creatures, seen, fail); err != nil {
		return err
	}
	for _, u := range in.Unlocks {
		if _, ok := c.features[u.Feature]; !ok {
			return fail("%q is not a feature", u.Feature)
		}
		if len(u.Creatures) == 0 || !slices.Contains(summonAttacks, u.Attack) {
			return fail("an unlock needs creatures and an attack of %v", summonAttacks)
		}
		if err := c.checkSummonCreatures(u.Creatures, seen, fail); err != nil {
			return err
		}
		def.unlocks = append(def.unlocks, summonUnlock{feature: u.Feature, creatures: slices.Clone(u.Creatures), attack: u.Attack})
	}
	return nil
}

// checkSummonCreatures refuses a key that is not a creature, and one seen
// before in the entry.
func (c *content) checkSummonCreatures(keys []string, seen map[string]bool, fail summonFail) error {
	for _, k := range keys {
		if _, ok := c.monsters[k]; !ok {
			return fail("%q is not a creature", k)
		}
		if seen[k] {
			return fail("%q is listed twice", k)
		}
		seen[k] = true
	}
	return nil
}

// loadSummonBands checks a summon of a creature type: its bands of count and
// challenge rating, and the circles that multiply the counts.
func loadSummonBands(def *summonDef, spellLevel int, in *summonJSON, fail summonFail) error {
	if in.Count != 0 || in.CountPerLevel != 0 || len(in.Unlocks) != 0 {
		return fail("a summon of a creature type takes options, not a count")
	}
	if _, ok := creatureTypeNamePT[in.Type]; !ok {
		return fail("%q is not a creature type", in.Type)
	}
	for _, o := range in.Options {
		if _, ok := crEighths(o.MaxCR); !ok || o.Count < 1 {
			return fail("an option needs a count of at least 1 and a max_cr that is a challenge rating")
		}
		def.options = append(def.options, summonBand{count: o.Count, maxCR: o.MaxCR})
	}
	for circle, times := range in.MultiplierAtLevel {
		n, err := strconv.Atoi(circle)
		if err != nil || n <= spellLevel || n > MaxSlotLevel || times < minSummonMultiplier {
			return fail("multiplier_at_level maps a circle above the spell's own, up to 9, to a number of at least 2")
		}
		def.multipliers = append(def.multipliers, summonMultiplier{circle: n, times: times})
	}
	slices.SortFunc(def.multipliers, func(a, b summonMultiplier) int { return a.circle - b.circle })
	return nil
}

// times is the number the counts of the spell are multiplied by with a slot of
// the given circle (Conjure Animals: 2 at the 5th circle, 3 at the 7th, 4 at the 9th).
func (d *summonDef) times(circle int) int {
	n := 1
	for _, m := range d.multipliers {
		if circle >= m.circle {
			n = m.times
		}
	}
	return n
}

// SummonSpells lists the keys of the spells that summon creatures, by their own
// circle and then their key: the character sheet offers the ones its build can
// cast (SummonOptions says what each may bring).
func (c *Content) SummonSpells() []string {
	keys := make([]string, 0, len(c.c.summons))
	for k := range c.c.summons {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(a, b string) int {
		if d := c.c.summons[a].spellLevel - c.c.summons[b].spellLevel; d != 0 {
			return d
		}
		return strings.Compare(a, b)
	})
	return keys
}

// SummonOptions says what casting a summoning spell with a slot of the given
// circle may choose, for the build (the Pact of the Chain adds the imp, the
// pseudodragon, the quasit and the sprite to Find Familiar, and lets any form of
// the familiar attack with its reaction). It refuses a spell
// that does not summon, and a slot below the spell's circle.
func (c *Content) SummonOptions(spell string, circle int, b Build) (SummonChoices, error) {
	def, ok := c.c.summons[spell]
	if !ok {
		return SummonChoices{}, fmt.Errorf("%w: %s", ErrNotSummonSpell, spell)
	}
	if circle < def.spellLevel || circle > MaxSlotLevel {
		return SummonChoices{}, fmt.Errorf("%w: %s at circle %d", ErrSummonCircle, spell, circle)
	}
	sp := c.c.spells[spell]
	out := SummonChoices{
		Spell: spell, Circle: circle, Ritual: sp.Ritual, Concentration: sp.Concentration,
		CastingTime: parseCastingTime(sp.CastingTime),
	}
	if len(def.options) > 0 {
		for _, o := range def.options {
			out.Options = append(out.Options, SummonOption{Count: o.count * def.times(circle), Type: def.typ, MaxCR: o.maxCR, Attack: def.attack})
		}
		return out, nil
	}
	opt := SummonOption{Count: def.count + def.perLevel*(circle-def.spellLevel)}
	for _, k := range def.creatures {
		opt.Forms = append(opt.Forms, SummonForm{Key: k, Attack: def.attack})
	}
	if len(def.unlocks) > 0 {
		opt.Forms = c.unlockedForms(def, opt.Forms, b)
	}
	out.Options = []SummonOption{opt}
	return out, nil
}

// unlockedForms adds the forms the build's features unlock, each of which
// changes what every form may do.
func (c *Content) unlockedForms(def *summonDef, forms []SummonForm, b Build) []SummonForm {
	owned := map[string]bool{}
	for _, f := range Derive(b, c).Features {
		owned[f.Key] = true
	}
	for _, u := range def.unlocks {
		if !owned[u.feature] {
			continue
		}
		for _, k := range u.creatures {
			forms = append(forms, SummonForm{Key: k})
		}
		// The feature changes the familiar whatever its form: the SRD lets
		// a Pact of the Chain warlock's familiar attack with its reaction.
		for i := range forms {
			forms[i].Attack = u.attack
		}
	}
	return forms
}

// CheckSummon refuses a choice the spell does not allow with this slot and
// build, with ErrSummonOption, ErrSummonCount or ErrSummonCreature, and returns
// the creatures with what each may do.
func (c *Content) CheckSummon(spell string, circle int, b Build, pick SummonPick) ([]SummonedCreature, error) {
	choices, err := c.SummonOptions(spell, circle, b)
	if err != nil {
		return nil, err
	}
	if pick.Option < 0 || pick.Option >= len(choices.Options) {
		return nil, fmt.Errorf("%w: %d", ErrSummonOption, pick.Option)
	}
	opt := choices.Options[pick.Option]
	if len(pick.Creatures) != opt.Count {
		return nil, fmt.Errorf("%w: %d chosen, the option takes %d", ErrSummonCount, len(pick.Creatures), opt.Count)
	}
	limit, _ := crEighths(opt.MaxCR)
	out := make([]SummonedCreature, 0, len(pick.Creatures))
	for _, k := range pick.Creatures {
		if len(opt.Forms) > 0 {
			i := slices.IndexFunc(opt.Forms, func(f SummonForm) bool { return f.Key == k })
			if i < 0 {
				return nil, fmt.Errorf("%w: %s", ErrSummonCreature, k)
			}
			out = append(out, SummonedCreature{Key: k, Attack: opt.Forms[i].Attack})
			continue
		}
		m, ok := c.c.monsters[k]
		if _, table := c.c.creatureOf[k]; !ok || table || m.Type != opt.Type {
			return nil, fmt.Errorf("%w: %s", ErrSummonCreature, k)
		}
		if v, _ := crEighths(m.ChallengeRating); v > limit {
			return nil, fmt.Errorf("%w: %s has a challenge rating above %s", ErrSummonCreature, k, opt.MaxCR)
		}
		out = append(out, SummonedCreature{Key: k, Attack: opt.Attack})
	}
	return out, nil
}
