package rules

import (
	"fmt"
	"io/fs"
	"slices"
	"strings"
)

// Spells that read hit points (MR-014, Etapa 8): the effects/spells.json file.
// Sono, Leque Cromático, the Palavras de Poder, Estabilizar and Cura
// Completa do not roll damage: what they do depends on the hit points the
// creatures have now. Each spell takes one of four closed kinds, and the
// loader refuses anything else (ADR-0008). The numbers of the kinds are
// resolved by the pure functions of package combat (hpspells.go); the play
// module reads the real hit points and applies the result.

// The kinds of a spell that reads hit points.
const (
	// SpellKindHPPool: roll a pool (Dice) and affect the creatures in ascending
	// order of current hit points, each one that fits in what is left of it
	// (Sono, Leque Cromático).
	SpellKindHPPool = "hp_pool"
	// SpellKindHPThreshold: a creature at or below Threshold hit points suffers
	// the effect: a Condition, or death (Palavra de Poder Atordoar, Matar).
	SpellKindHPThreshold = "hp_threshold"
	// SpellKindZeroHP: it works only on a creature at 0 hit points, and makes it
	// stable (Estabilizar).
	SpellKindZeroHP = "zero_hp_target"
	// SpellKindFlatHeal: heals a fixed amount and ends the conditions in Ends
	// (Cura Completa).
	SpellKindFlatHeal = "flat_heal"
	// SpellKindTempHP: the targets gain temporary hit points, Dice plus Amount
	// (False Life). Temporary hit points are not healing: they do not count
	// against the maximum, and a target that has more keeps them (SRD 5.1).
	SpellKindTempHP = "temp_hp"
	// SpellKindMaxHP: the targets' maximum and current hit points both rise by
	// Amount (Aid). It is not healing either: the current hit points follow the
	// maximum up, and nothing is healed beyond what the maximum rose.
	SpellKindMaxHP = "max_hp"
	// SpellKindSummon: summons creatures (Convocar Familiar, Animar Mortos,
	// Conjurar Animais); see summon.go. It reads no hit points: SpellEffect
	// does not return it, and SummonOptions does.
	SpellKindSummon = "summon"
	// SpellKindIgnoresCover: the spell's saving throw gets no benefit from cover
	// (Chama Sagrada, SRD 5.1). It reads no hit points either: SpellEffect does
	// not return it, and IgnoresCover says it.
	SpellKindIgnoresCover = "ignores_cover"
	// SpellKindRevive: the spell brings a creature that died in the last minute back
	// to life with HitPoints hit points (Revivify, SRD 5.1). A minute is WindowRounds
	// rounds in a combat ("a round is about 6 seconds", SRD 5.1, "The Order of Combat").
	// It reads no hit points: SpellEffect does not return it, and Revive does.
	SpellKindRevive = "revive"
)

// ReviveSpec is what a spell of the revive kind does (Revivify).
type ReviveSpec struct {
	// HitPoints is what the creature has when it lives again.
	HitPoints int
	// WindowRounds is how many rounds of a combat the spell reaches back: a minute.
	WindowRounds int
}

// SpellEffect is what a spell that reads hit points does, at a slot level.
type SpellEffect struct {
	// Kind is one of the SpellKind constants.
	Kind string
	// Dice is the pool of an hp_pool spell at the slot level: the spell's dice
	// plus the extra dice of each level above its own. No Bonus, no modifier.
	Dice DiceFormula
	// Condition is the key ("condition:unconscious") a pool or threshold spell
	// gives to each creature it affects. Empty when it gives none.
	Condition string
	// Threshold is the most hit points a creature may have to be affected, for an
	// hp_threshold spell.
	Threshold int
	// Dies says an hp_threshold spell kills instead of giving a condition.
	Dies bool
	// Heal is the fixed healing of a flat_heal spell at the slot level, and Ends
	// the condition keys it ends.
	Heal int
	Ends []string
	// Amount is the fixed number a temp_hp spell adds to Dice and a max_hp spell
	// raises the maximum by, at the slot level.
	Amount int
}

// spellEffectFile is the shape of effects/spells.json.
type spellEffectFile struct {
	Spells map[string]spellEffectJSON `json:"spells"`
}

type spellEffectJSON struct {
	Kind         string   `json:"kind"`
	Dice         string   `json:"dice,omitempty"`
	DicePerLevel string   `json:"dice_per_level,omitempty"`
	Condition    string   `json:"condition,omitempty"`
	Threshold    int      `json:"threshold,omitempty"`
	Dies         bool     `json:"dies,omitempty"`
	Amount       int      `json:"amount,omitempty"`
	AmountPerLvl int      `json:"amount_per_level,omitempty"`
	Ends         []string `json:"ends,omitempty"`
	// The revive kind: the hit points the creature comes back with and the rounds the
	// spell reaches back. Source is the SRD passage the numbers come from; a revive
	// entry must say it.
	HitPoints    int    `json:"hit_points,omitempty"`
	WindowRounds int    `json:"window_rounds,omitempty"`
	Source       string `json:"source,omitempty"`
	// summonJSON are the fields of the "summon" kind.
	summonJSON
}

// spellEffectDef is a checked entry: the effect at the spell's own level and
// what each level above adds.
type spellEffectDef struct {
	spellLevel int
	base       SpellEffect
	perLevel   DiceFormula // an hp_pool's extra dice
	healPerLvl int         // a flat_heal's extra healing
	amountPer  int         // a temp_hp's and a max_hp's extra amount
}

// spellEffectFields is a set of the hit point fields of an entry of
// effects/spells.json (the ones every kind allows or refuses as a group).
type spellEffectFields uint

const (
	fieldDice spellEffectFields = 1 << iota
	fieldDicePerLevel
	fieldCondition
	fieldThreshold
	fieldDies
	fieldAmount
	fieldAmountPerLevel
	fieldEnds
)

// set is the fields the entry sets to something other than zero.
func (in *spellEffectJSON) set() spellEffectFields {
	var out spellEffectFields
	for _, f := range []struct {
		on   bool
		flag spellEffectFields
	}{
		{in.Dice != "", fieldDice},
		{in.DicePerLevel != "", fieldDicePerLevel},
		{in.Condition != "", fieldCondition},
		{in.Threshold != 0, fieldThreshold},
		{in.Dies, fieldDies},
		{in.Amount != 0, fieldAmount},
		{in.AmountPerLvl != 0, fieldAmountPerLevel},
		{len(in.Ends) != 0, fieldEnds},
	} {
		if f.on {
			out |= f.flag
		}
	}
	return out
}

// outside says the entry sets a field that is not in allowed.
func (in *spellEffectJSON) outside(allowed spellEffectFields) bool { return in.set()&^allowed != 0 }

// spellEffectEntry is one entry of effects/spells.json being loaded.
type spellEffectEntry struct {
	key        string
	in         spellEffectJSON
	spellLevel int
}

func (e spellEffectEntry) fail(format string, a ...any) error {
	return fmt.Errorf("effects/spells.json: %s: %s", e.key, fmt.Sprintf(format, a...))
}

// spellEffectKinds builds the definition of each kind that reads hit points
// (the others, revive, summon and ignores_cover, are loaded apart).
var spellEffectKinds = map[string]func(e spellEffectEntry, def *spellEffectDef) error{
	SpellKindHPPool:      spellEffectEntry.hpPool,
	SpellKindHPThreshold: spellEffectEntry.hpThreshold,
	SpellKindZeroHP:      spellEffectEntry.zeroHP,
	SpellKindTempHP:      spellEffectEntry.tempHP,
	SpellKindMaxHP:       spellEffectEntry.maxHP,
	SpellKindFlatHeal:    spellEffectEntry.flatHeal,
}

// loadSpellEffects reads and checks effects/spells.json.
func (c *content) loadSpellEffects(fsys fs.FS) error {
	var f spellEffectFile
	if err := readJSON(fsys, "effects/spells.json", &f); err != nil {
		return err
	}
	c.spellEffects = map[string]spellEffectDef{}
	c.coverIgnoring = map[string]bool{}
	c.revives = map[string]ReviveSpec{}
	for _, key := range sortedKeys(f.Spells) {
		if err := c.loadSpellEffect(key, f.Spells[key]); err != nil {
			return err
		}
	}
	return nil
}

// loadSpellEffect checks one entry and stores it.
func (c *content) loadSpellEffect(key string, in spellEffectJSON) error {
	sp, ok := c.spells[key]
	if !ok {
		return fmt.Errorf("effects/spells.json: %q is not a spell of the content", key)
	}
	e := spellEffectEntry{key: key, in: in, spellLevel: sp.Level}
	if done, err := c.loadApartKind(e); done {
		return err
	}
	def := spellEffectDef{spellLevel: sp.Level, base: SpellEffect{Kind: in.Kind, Condition: in.Condition, Threshold: in.Threshold, Dies: in.Dies, Ends: in.Ends}}
	build, ok := spellEffectKinds[in.Kind]
	if !ok {
		return e.fail("unknown kind %q", in.Kind)
	}
	if err := build(e, &def); err != nil {
		return err
	}
	if err := c.checkEffectConditions(e); err != nil {
		return err
	}
	def.base.Ends = slices.Clone(in.Ends)
	c.spellEffects[key] = def
	return nil
}

// loadApartKind loads the kinds that read no hit points (revive, summon and
// ignores_cover) and refuses the fields that belong to them on any other kind.
// done says the entry is dealt with, whatever err is.
func (c *content) loadApartKind(e spellEffectEntry) (done bool, err error) {
	in := &e.in
	if in.Kind == SpellKindRevive {
		return true, c.loadRevive(e)
	}
	if in.HitPoints != 0 || in.WindowRounds != 0 {
		return true, e.fail("hit_points and window_rounds belong to the revive kind")
	}
	if in.Kind == SpellKindSummon {
		return true, c.loadSummonEntry(e)
	}
	if !in.empty() {
		return true, e.fail("the summon fields belong to the summon kind")
	}
	if in.Kind == SpellKindIgnoresCover {
		if in.set() != 0 {
			return true, e.fail("an ignores_cover spell takes nothing else")
		}
		c.coverIgnoring[e.key] = true
		return true, nil
	}
	return false, nil
}

func (c *content) loadRevive(e spellEffectEntry) error {
	in := &e.in
	if in.HitPoints < 1 || in.WindowRounds < 1 || in.Source == "" || in.set() != 0 {
		return e.fail("a revive takes hit_points, window_rounds and its source only")
	}
	c.revives[e.key] = ReviveSpec{HitPoints: in.HitPoints, WindowRounds: in.WindowRounds}
	return nil
}

func (c *content) loadSummonEntry(e spellEffectEntry) error {
	if e.in.set() != 0 {
		return e.fail("a summon takes the summon fields only")
	}
	return c.loadSummon(e.key, e.spellLevel, &e.in.summonJSON, e.fail)
}

func (e spellEffectEntry) hpPool(def *spellEffectDef) error {
	in := &e.in
	dice, ok1 := plainDice(in.Dice)
	per, ok2 := plainDice(in.DicePerLevel)
	if !ok1 || !ok2 || per.Sides != dice.Sides {
		return e.fail("an hp_pool needs dice and dice_per_level of the same die that exists at a table, as in \"5d8\" and \"2d8\"")
	}
	if in.Condition == "" || in.outside(fieldDice|fieldDicePerLevel|fieldCondition) {
		return e.fail("an hp_pool takes dice, dice_per_level and a condition only")
	}
	def.base.Dice, def.perLevel = dice, per
	return nil
}

func (e spellEffectEntry) hpThreshold(*spellEffectDef) error {
	in := &e.in
	if in.Threshold < 1 || (in.Dies == (in.Condition != "")) || in.outside(fieldThreshold|fieldDies|fieldCondition) {
		return e.fail("an hp_threshold takes a threshold and either a condition or dies")
	}
	return nil
}

func (e spellEffectEntry) zeroHP(*spellEffectDef) error {
	if e.in.set() != 0 {
		return e.fail("a zero_hp_target takes nothing else")
	}
	return nil
}

func (e spellEffectEntry) tempHP(def *spellEffectDef) error {
	in := &e.in
	dice, ok := plainDice(in.Dice)
	if !ok || in.Amount < 0 || in.AmountPerLvl < 1 || in.outside(fieldDice|fieldAmount|fieldAmountPerLevel) {
		return e.fail("a temp_hp takes dice, amount and amount_per_level only")
	}
	def.base.Dice, def.base.Amount, def.amountPer = dice, in.Amount, in.AmountPerLvl
	return nil
}

func (e spellEffectEntry) maxHP(def *spellEffectDef) error {
	in := &e.in
	if in.Amount < 1 || in.AmountPerLvl < 0 || in.outside(fieldAmount|fieldAmountPerLevel) {
		return e.fail("a max_hp takes amount and amount_per_level only")
	}
	def.base.Amount, def.amountPer = in.Amount, in.AmountPerLvl
	return nil
}

func (e spellEffectEntry) flatHeal(def *spellEffectDef) error {
	in := &e.in
	if in.Amount < 1 || in.AmountPerLvl < 0 || len(in.Ends) == 0 || in.outside(fieldAmount|fieldAmountPerLevel|fieldEnds) {
		return e.fail("a flat_heal takes an amount, amount_per_level and the conditions it ends")
	}
	def.base.Heal, def.healPerLvl = in.Amount, in.AmountPerLvl
	return nil
}

// checkEffectConditions: the condition the entry gives and the ones it ends
// are conditions of the content, the condition first.
func (c *content) checkEffectConditions(e spellEffectEntry) error {
	for _, k := range append([]string{e.in.Condition}, e.in.Ends...) {
		if k == "" {
			continue
		}
		if _, ok := c.named[k]; !ok || !strings.HasPrefix(k, "condition:") {
			return e.fail("%q is not a condition", k)
		}
	}
	return nil
}

// IgnoresCover says the spell's saving throw gets no benefit from cover (the
// SRD's Chama Sagrada), from the "ignores_cover" kind of effects/spells.json.
func (c *Content) IgnoresCover(key string) bool { return c.c.coverIgnoring[key] }

// Revive says what a spell of the revive kind does (Revivify), and false for any other spell.
func (c *Content) Revive(key string) (ReviveSpec, bool) {
	r, ok := c.c.revives[key]
	return r, ok
}

// SpellEffect returns what a spell that reads hit points does when cast with a
// slot of slotLevel (the spell's own level for a smaller or zero slotLevel),
// and false for any other spell.
func (c *Content) SpellEffect(key string, slotLevel int) (SpellEffect, bool) {
	def, ok := c.c.spellEffects[key]
	if !ok {
		return SpellEffect{}, false
	}
	out := def.base
	out.Ends = slices.Clone(def.base.Ends)
	extra := max(slotLevel-def.spellLevel, 0)
	out.Dice.Count += def.perLevel.Count * extra
	out.Heal += def.healPerLvl * extra
	out.Amount += def.amountPer * extra
	return out, true
}
