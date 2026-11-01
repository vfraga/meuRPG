package rules

import (
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strconv"

	"github.com/PuraFome/meuRPG/backend/internal/rules/encounter"
)

// The encounter builder (MR-043, RN-29). The arithmetic is
// package encounter; this file loads its table (effects/encounter_budget.json, the
// SRD 5.2.1's "XP Budget per Character") and puts the SRD's creatures in it.

type encounterBudgetFile struct {
	Source string `json:"source"`
	Levels []struct {
		Level    int `json:"level"`
		Low      int `json:"low"`
		Moderate int `json:"moderate"`
		High     int `json:"high"`
	} `json:"levels"`
}

// loadEncounterBudget reads and checks effects/encounter_budget.json: every level
// from 1 to 20, in order, each row growing from low to moderate to high, and each
// level costing more than the one before in every band.
func (c *content) loadEncounterBudget(fsys fs.FS) error {
	const name = "effects/encounter_budget.json"
	var f encounterBudgetFile
	if err := readJSON(fsys, name, &f); err != nil {
		return err
	}
	if f.Source == "" {
		return fmt.Errorf("%s: needs a source", name)
	}
	if len(f.Levels) != MaxLevel {
		return fmt.Errorf("%s: needs %d levels, got %d", name, MaxLevel, len(f.Levels))
	}
	rows := make([]encounter.Budget, 0, MaxLevel)
	for i, l := range f.Levels {
		b := encounter.Budget{Low: l.Low, Moderate: l.Moderate, High: l.High}
		switch {
		case l.Level != i+1:
			return fmt.Errorf("%s: levels[%d] is level %d, want %d (no gap, in order)", name, i, l.Level, i+1)
		case b.Low <= 0 || b.Low >= b.Moderate || b.Moderate >= b.High:
			return fmt.Errorf("%s: level %d must have 0 < low < moderate < high", name, l.Level)
		case i > 0 && (b.Low <= rows[i-1].Low || b.Moderate <= rows[i-1].Moderate || b.High <= rows[i-1].High):
			return fmt.Errorf("%s: level %d must cost more than level %d in every band", name, l.Level, l.Level-1)
		}
		rows = append(rows, b)
	}
	c.encounterBudget = rows
	return nil
}

// EncounterBudgets returns the XP budget per character for each level 1 to 20,
// index 0 being level 1 (SRD 5.2.1). The caller gets a copy.
func (c *Content) EncounterBudgets() []encounter.Budget {
	return slices.Clone(c.c.encounterBudget)
}

// crLabel writes a rating in eighths as the SRD does: "1/8", "1/4", "1/2", "1"...
func crLabel(eighths int) string {
	switch eighths {
	case 1:
		return "1/8"
	case 2:
		return "1/4"
	case 4:
		return "1/2"
	}
	return strconv.Itoa(eighths / 8)
}

// maxCREighths is the highest rating of the SRD (30), in eighths.
const maxCREighths = 30 * 8

// The errors of the encounter builder.
var (
	// ErrEncounterCreature: an entry names a creature that is not an SRD creature.
	ErrEncounterCreature = errors.New("rules: not an SRD creature")
	// ErrEncounterNoParty: the party is empty, so there is no budget to build for.
	ErrEncounterNoParty = errors.New("rules: the party is empty")
	// ErrEncounterBand: the band asked for is not low, moderate or high.
	ErrEncounterBand = errors.New("rules: not a band an encounter can be built for")
	// ErrEncounterType: the creature type is not an SRD type.
	ErrEncounterType = errors.New("rules: not a creature type")
	// ErrEncounterCount: a count below 1.
	ErrEncounterCount = errors.New("rules: a count is at least 1")
	// ErrEncounterNothingFits is encounter.ErrNoCreature: the options leave no creature.
	ErrEncounterNothingFits = encounter.ErrNoCreature
)

// EncounterLine is one kind of creature of an encounter, with its cost.
type EncounterLine struct {
	Creature CreatureEntry
	Count    int
	// Subtotal is Creature.XP times Count.
	Subtotal int
	// AboveCap says the creature's rating is above the party's cap (EncounterEvaluation.MaxCR).
	AboveCap bool
}

// EncounterEvaluation is an encounter measured against a party: what the party can
// spend, what the encounter costs, and the band it falls in.
type EncounterEvaluation struct {
	// Budget is the party's XP in each band: the sum of the members' (SRD 5.2.1).
	Budget encounter.Budget
	Lines  []EncounterLine
	// TotalXP is the sum of every line, and Creatures how many creatures there are.
	TotalXP, Creatures int
	// Band is the smallest band whose budget holds TotalXP; OverXP is how far TotalXP
	// is above the high budget when Band is above high, 0 otherwise.
	Band   encounter.Band
	OverXP int
	// LowestLevel is the party's lowest level, and MaxCR the highest rating a
	// creature may have for it ("7" for lowest level 4): the lowest level plus 3.
	LowestLevel int
	MaxCR       string
}

func encounterCreature(e CreatureEntry) encounter.Creature {
	cr, _ := crEighths(e.ChallengeRating)
	return encounter.Creature{Key: e.Key, CR: cr, XP: e.XP, Type: e.Type}
}

// EvaluateEncounter measures the creatures against a party of these levels. It
// refuses an entry whose creature is not an SRD creature (ErrEncounterCreature) or
// whose count is below 1 (ErrEncounterCount), and a level outside 1 to 20
// (encounter.ErrLevel). An empty party is allowed: its budget is zero.
func (c *Content) EvaluateEncounter(levels []int, entries []encounter.Entry) (EncounterEvaluation, error) {
	budget, err := encounter.PartyBudget(c.c.encounterBudget, levels)
	if err != nil {
		return EncounterEvaluation{}, err
	}
	maxCR := encounter.MaxCR(levels, maxCREighths)
	ev := EncounterEvaluation{Budget: budget, LowestLevel: encounter.LowestLevel(levels), MaxCR: crLabel(maxCR)}
	byKey := make(map[string]CreatureEntry, len(entries))
	for _, e := range c.srdCreatureEntries() {
		byKey[e.Key] = e
	}
	for _, e := range entries {
		ce, ok := byKey[e.Key]
		switch {
		case !ok:
			return EncounterEvaluation{}, fmt.Errorf("%w: %q", ErrEncounterCreature, e.Key)
		case e.Count < 1:
			return EncounterEvaluation{}, ErrEncounterCount
		}
		cr, _ := crEighths(ce.ChallengeRating)
		ev.Lines = append(ev.Lines, EncounterLine{Creature: ce, Count: e.Count, Subtotal: ce.XP * e.Count, AboveCap: len(levels) > 0 && cr > maxCR})
		ev.TotalXP += ce.XP * e.Count
		ev.Creatures += e.Count
	}
	ev.Band, ev.OverXP = encounter.Classify(budget, ev.TotalXP)
	return ev, nil
}

// GenerateEncounter builds an encounter of one leader and a group for a party of
// these levels, in a band (low, moderate or high) and, if kind is not "", of one
// SRD creature type ("humanoid"). It never costs more than the band's budget and
// never has a creature above the party's rating cap; the same arguments and seed
// give the same encounter (package encounter describes the algorithm). The answer
// is measured like EvaluateEncounter's. maxTotal is the most creatures it may have (0: the generator's default, 24):
// the caller passes what is left of a combat of 40 after the party. It is below 1 when the party fills
// the combat, and then nothing fits.
// The answer is measured like EvaluateEncounter's. Errors: ErrEncounterNoParty, ErrEncounterBand,
// ErrEncounterType and ErrEncounterNothingFits (no creature has the type, or none fits
// the cap and the budget).
func (c *Content) GenerateEncounter(levels []int, band encounter.Band, kind string, seed uint32, maxTotal int) (EncounterEvaluation, error) {
	if len(levels) == 0 {
		return EncounterEvaluation{}, ErrEncounterNoParty
	}
	if band != encounter.BandLow && band != encounter.BandModerate && band != encounter.BandHigh {
		return EncounterEvaluation{}, ErrEncounterBand
	}
	if _, ok := creatureTypeNamePT[kind]; kind != "" && !ok {
		return EncounterEvaluation{}, ErrEncounterType
	}
	budget, err := encounter.PartyBudget(c.c.encounterBudget, levels)
	if err != nil {
		return EncounterEvaluation{}, err
	}
	srd := c.srdCreatureEntries()
	pool := make([]encounter.Creature, len(srd))
	for i, e := range srd {
		pool[i] = encounterCreature(e)
	}
	res, err := encounter.Generate(pool, encounter.Options{
		Budget: budget.For(band), MaxCR: encounter.MaxCR(levels, maxCREighths), Type: kind, Seed: seed, MaxTotal: maxTotal,
	})
	if err != nil {
		return EncounterEvaluation{}, err
	}
	return c.EvaluateEncounter(levels, res.Entries)
}

// EncounterSwaps lists the creatures an entry can be swapped for: the same XP, so
// the total does not change, and, when kind is not "", the same type too. The
// creature itself is left out; the list is sorted by Portuguese name. ErrEncounterCreature
// for a key that is not an SRD creature, ErrEncounterType for a type that is not an SRD type.
func (c *Content) EncounterSwaps(key, kind string) ([]CreatureEntry, error) {
	if _, ok := creatureTypeNamePT[kind]; kind != "" && !ok {
		return nil, ErrEncounterType
	}
	var cur CreatureEntry
	found := false
	srd := c.srdCreatureEntries()
	for _, e := range srd {
		if e.Key == key {
			cur, found = e, true
		}
	}
	if !found {
		return nil, fmt.Errorf("%w: %q", ErrEncounterCreature, key)
	}
	var out []CreatureEntry
	for _, e := range srd {
		if e.XP == cur.XP && e.Key != cur.Key && (kind == "" || e.Type == kind) {
			out = append(out, e)
		}
	}
	return out, nil
}

// srdCreatureEntries are the creatures the encounter tools use: the SRD's. A table creature has the
// challenge rating and the XP of its base, which its adjustments may not keep, so it is left out.
func (c *Content) srdCreatureEntries() []CreatureEntry {
	if len(c.c.creatureOf) == 0 {
		return c.c.monsterEntries
	}
	return slices.DeleteFunc(slices.Clone(c.c.monsterEntries), func(e CreatureEntry) bool { _, table := c.c.creatureOf[e.Key]; return table })
}

// HasCreature says whether the key is an SRD creature ("monster:ogre"). A table creature is not: the
// encounter tools measure with the SRD's challenge ratings.
func (c *Content) HasCreature(key string) bool {
	if _, table := c.c.creatureOf[key]; table {
		return false
	}
	_, ok := c.c.monsters[key]
	return ok
}
