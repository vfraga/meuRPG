package rules

import (
	"fmt"
	"io/fs"
	"slices"
	"strings"
)

// Trap presets (MR-035, Etapa 9, D5): the eight sample traps of the SRD 5.1
// ("Sample Traps" in the Dungeon Master's rules, 5e-database's 5e-SRD-Rules
// section "sample-traps"), in our own words, with the SRD's numbers, plus the
// SRD's tables of how severe a trap is. The file is effects/traps.json, written
// by hand: the loader checks it against a closed schema and refuses anything
// else (ADR-0008), and changing it raises the effects revision.
//
// A preset only fills the master's form; the master edits anything. The play
// module rolls and applies a trap's parts, with what package grid says about
// the area; nothing here rolls a die.

// The ways a trap is triggered.
const (
	// TrapTriggerEnter: a creature of a player enters the area (a pressure
	// plate, a trip wire, a false floor). NPCs never fire it by themselves.
	TrapTriggerEnter = "enter"
	// TrapTriggerManual: the master fires it (a chest opened, a door).
	TrapTriggerManual = "manual"
)

// Who a trap's effect reaches.
const (
	// TrapTargetsArea: every creature in the trap's area.
	TrapTargetsArea = "area"
	// TrapTargetsManual: the master picks who is caught, because the effect
	// comes from somewhere else (a statue's cone, a rolling sphere) or hits at
	// random (darts).
	TrapTargetsManual = "manual"
)

// Whom a trap's saving throw is asked of.
const (
	// TrapSaveCaught: each creature the trap caught.
	TrapSaveCaught = "caught"
	// TrapSaveHit: each creature one of the trap's attacks hit.
	TrapSaveHit = "hit"
)

// What a creature that passes the saving throw takes.
const (
	// TrapPassHalf: half the damage the failure would have done.
	TrapPassHalf = "half"
	// TrapPassNone: nothing.
	TrapPassNone = "none"
)

// The kinds of trap, as the SRD says.
const (
	TrapMechanical = "mechanical"
	TrapMagic      = "magic"
)

// TrapDamage is damage of one type. Dice may be a flat number ("1" for the
// poison needle's piercing damage).
type TrapDamage struct {
	Dice DiceFormula
	// Type is a damage type key, "damage-type:poison".
	Type string
}

// TrapCondition is a condition a trap gives to every creature it caught.
type TrapCondition struct {
	// Condition is a condition key, "condition:restrained".
	Condition string
	// DurationPT says how long it lasts, as text for the master ("1 hora"),
	// or is empty.
	DurationPT string
}

// TrapAttack is a trap that attacks: Count attacks with a bonus, each against
// one target. A hit does Damage; a saving throw that follows is asked of the
// creatures hit.
type TrapAttack struct {
	Bonus  int
	Count  int
	Damage TrapDamage
}

// TrapOnFail is what a creature that fails the saving throw takes: damage
// and/or a condition.
type TrapOnFail struct {
	Damage []TrapDamage
	// Condition is a condition key, or empty, and DurationPT how long it lasts.
	Condition  string
	DurationPT string
}

// TrapSave is the saving throw of a trap.
type TrapSave struct {
	Ability Ability
	DC      int
	// AppliesTo is TrapSaveCaught or TrapSaveHit.
	AppliesTo string
	OnFail    TrapOnFail
	// OnPass is TrapPassHalf or TrapPassNone.
	OnPass string
}

// TrapPreset is a trap the master can start from.
type TrapPreset struct {
	// Key is "trap:<name>"; NamePT is its Portuguese name.
	Key    string
	NamePT string
	// Kind is TrapMechanical or TrapMagic.
	Kind string
	// DescriptionPT is the preset's text, in our own words.
	DescriptionPT string
	// NoticeDC is the DC to notice it (Wisdom (Perception), passive or active),
	// and 0 when the SRD gives none (the poison needle). FindDC is the DC to find
	// it by searching (Intelligence (Investigation)); where the SRD gives only
	// one DC for spotting, it is used for both.
	NoticeDC int
	FindDC   int
	// FindDCIsOurs says the SRD gives no DC to find this trap (only to spot it),
	// so FindDC is ours: the DC to spot, reused. The DCs of the other presets
	// are the SRD's.
	FindDCIsOurs bool
	// Trigger is TrapTriggerEnter or TrapTriggerManual.
	Trigger string
	// AreaSize is the side, in squares, of the block around the trap's square
	// that fires it: 1 for the square alone, up to 4.
	AreaSize int
	// Targets is TrapTargetsArea or TrapTargetsManual.
	Targets string
	// FallFt is the depth of a pit in feet, 0 for a trap that is not a pit. The
	// fall damage in Damage is FallDice(FallFt).
	FallFt int
	// Damage lands on every creature caught, with no roll to avoid it.
	Damage []TrapDamage
	// Conditions are given to every creature caught.
	Conditions []TrapCondition
	// Attack and Save are nil when the trap has none.
	Attack *TrapAttack
	Save   *TrapSave
}

// IntRange is a closed range of whole numbers.
type IntRange struct {
	Min, Max int
}

// TrapSeverity is one row of the SRD's "Trap Save DCs and Attack Bonuses".
type TrapSeverity struct {
	// Key is "setback", "dangerous" or "deadly", NamePT "Revés", "Perigosa" or
	// "Mortal".
	Key, NamePT string
	SaveDC      IntRange
	AttackBonus IntRange
}

// TrapDamageRow is one row of the SRD's "Damage Severity by Level": the damage
// that suits characters of levels FromLevel to ToLevel.
type TrapDamageRow struct {
	FromLevel, ToLevel int
	Setback            DiceFormula
	Dangerous          DiceFormula
	Deadly             DiceFormula
}

// FallDice is the damage of a fall (SRD): 1d6 bludgeoning for every 10 feet,
// up to 20d6. A fall of less than 10 feet does none (Count 0).
func FallDice(feet int) DiceFormula {
	n := min(max(feet, 0)/10, 20)
	if n == 0 {
		return DiceFormula{}
	}
	return DiceFormula{Count: n, Sides: 6}
}

// traps is what effects/traps.json holds, once checked.
type traps struct {
	presets    []TrapPreset
	severities []TrapSeverity
	damageRows []TrapDamageRow
}

type trapsFile struct {
	Severity struct {
		Levels        []trapSeverityJSON  `json:"levels"`
		DamageByLevel []trapDamageRowJSON `json:"damage_by_level"`
	} `json:"severity"`
	Traps []trapJSON `json:"traps"`
}

type trapSeverityJSON struct {
	Key         string                 `json:"key"`
	NamePT      string                 `json:"name_pt"`
	SaveDC      struct{ Min, Max int } `json:"save_dc"`
	AttackBonus struct{ Min, Max int } `json:"attack_bonus"`
}

type trapDamageRowJSON struct {
	From      int    `json:"from"`
	To        int    `json:"to"`
	Setback   string `json:"setback"`
	Dangerous string `json:"dangerous"`
	Deadly    string `json:"deadly"`
}

type trapDamageJSON struct {
	Dice string `json:"dice"`
	Type string `json:"type"`
}

type trapConditionJSON struct {
	Condition  string `json:"condition"`
	DurationPT string `json:"duration_pt,omitempty"`
}

type trapAttackJSON struct {
	Bonus  int            `json:"bonus"`
	Count  int            `json:"count"`
	Damage trapDamageJSON `json:"damage"`
}

type trapOnFailJSON struct {
	Damage     []trapDamageJSON `json:"damage,omitempty"`
	Condition  string           `json:"condition,omitempty"`
	DurationPT string           `json:"duration_pt,omitempty"`
}

type trapSaveJSON struct {
	Ability   string         `json:"ability"`
	DC        int            `json:"dc"`
	AppliesTo string         `json:"applies_to"`
	OnFail    trapOnFailJSON `json:"on_fail"`
	OnPass    string         `json:"on_pass"`
}

type trapJSON struct {
	Key           string              `json:"key"`
	Kind          string              `json:"kind"`
	DescriptionPT string              `json:"description_pt"`
	NoticeDC      int                 `json:"notice_dc,omitempty"`
	FindDC        int                 `json:"find_dc"`
	FindDCOurs    bool                `json:"find_dc_ours,omitempty"`
	Trigger       string              `json:"trigger"`
	AreaSize      int                 `json:"area_size"`
	Targets       string              `json:"targets"`
	FallFt        int                 `json:"fall_ft,omitempty"`
	Damage        []trapDamageJSON    `json:"damage,omitempty"`
	Conditions    []trapConditionJSON `json:"conditions,omitempty"`
	Attack        *trapAttackJSON     `json:"attack,omitempty"`
	Save          *trapSaveJSON       `json:"save,omitempty"`
}

var severityOrder = []string{"setback", "dangerous", "deadly"}

// The limits of the trap schema.
const (
	// maxTrapDC is the highest DC a trap, or its saving throw, can have.
	maxTrapDC = 30
	// maxTrapArea is the longest side, in squares, of the area of a trap.
	maxTrapArea = 4
	// fallStepFt is the depth, in feet, of each 1d6 of a fall.
	fallStepFt = 10
	// maxFallFt is the deepest pit.
	maxFallFt = 200
	// maxTrapAttackBonus and maxTrapAttacks bound a trap that attacks.
	maxTrapAttackBonus = 20
	maxTrapAttacks     = 10
)

// trapLoader checks effects/traps.json against the content, in the order the
// file is read: the severity tables, then each trap.
type trapLoader struct {
	c *content
}

const trapsFileName = "effects/traps.json"

func (l trapLoader) fail(format string, a ...any) error {
	return fmt.Errorf("%s: %s", trapsFileName, fmt.Sprintf(format, a...))
}

// loadTraps reads and checks effects/traps.json.
func (c *content) loadTraps(fsys fs.FS) error {
	var f trapsFile
	if err := readJSON(fsys, trapsFileName, &f); err != nil {
		return err
	}
	l := trapLoader{c: c}
	if err := l.severityLevels(f.Severity.Levels); err != nil {
		return err
	}
	if err := l.damageRows(f.Severity.DamageByLevel); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, in := range f.Traps {
		if err := l.trap(in, seen); err != nil {
			return err
		}
	}
	// Every Portuguese name must be of a preset: a renamed or removed trap
	// leaves no stray name behind.
	for k := range c.namesPT {
		if strings.HasPrefix(k, "trap:") && !seen[k] {
			return l.fail("effects/names_pt.json names %q, which is not in traps.json", k)
		}
	}
	return nil
}

func (l trapLoader) severityLevels(levels []trapSeverityJSON) error {
	if len(levels) != len(severityOrder) {
		return l.fail("severity.levels needs %d entries, got %d", len(severityOrder), len(levels))
	}
	for i, lv := range levels {
		sev, err := l.severityLevel(i, lv)
		if err != nil {
			return err
		}
		l.c.traps.severities = append(l.c.traps.severities, sev)
	}
	return nil
}

// severityLevel checks the i-th level: in order, named, with sane ranges that
// sit above the level before.
func (l trapLoader) severityLevel(i int, lv trapSeverityJSON) (TrapSeverity, error) {
	saveDC, attack := IntRange(lv.SaveDC), IntRange(lv.AttackBonus)
	switch {
	case lv.Key != severityOrder[i]:
		return TrapSeverity{}, l.fail("severity.levels[%d] must be %q, got %q", i, severityOrder[i], lv.Key)
	case strings.TrimSpace(lv.NamePT) == "":
		return TrapSeverity{}, l.fail("severity %s has no name_pt", lv.Key)
	case saveDC.Min < 1 || saveDC.Min > saveDC.Max || attack.Min < 0 || attack.Min > attack.Max:
		return TrapSeverity{}, l.fail("severity %s has an empty or negative range", lv.Key)
	case i > 0 && l.notAbove(i, saveDC, attack):
		return TrapSeverity{}, l.fail("severity %s must be above %s", lv.Key, severityOrder[i-1])
	}
	return TrapSeverity{Key: lv.Key, NamePT: lv.NamePT, SaveDC: saveDC, AttackBonus: attack}, nil
}

// notAbove says a level's ranges do not start above the level before's.
func (l trapLoader) notAbove(i int, saveDC, attack IntRange) bool {
	before := l.c.traps.severities[i-1]
	return saveDC.Min <= before.SaveDC.Max || attack.Min <= before.AttackBonus.Max
}

// damageRows checks that the rows cover levels 1 to MaxLevel in order.
func (l trapLoader) damageRows(rows []trapDamageRowJSON) error {
	next := 1
	for _, r := range rows {
		row, err := l.damageRow(r, next)
		if err != nil {
			return err
		}
		next = r.To + 1
		l.c.traps.damageRows = append(l.c.traps.damageRows, row)
	}
	if next != MaxLevel+1 {
		return l.fail("severity.damage_by_level stops at level %d, it must reach %d", next-1, MaxLevel)
	}
	return nil
}

func (l trapLoader) damageRow(r trapDamageRowJSON, next int) (TrapDamageRow, error) {
	row := TrapDamageRow{FromLevel: r.From, ToLevel: r.To}
	if r.From != next || r.To < r.From || r.To > MaxLevel {
		return row, l.fail("severity.damage_by_level must cover levels 1 to %d in order, with no gap (at level %d)", MaxLevel, next)
	}
	for _, one := range []struct {
		text string
		into *DiceFormula
	}{{r.Setback, &row.Setback}, {r.Dangerous, &row.Dangerous}, {r.Deadly, &row.Deadly}} {
		d, ok := ParseDice(one.text)
		if !ok || d.Count < 1 || d.Bonus != 0 || d.AddsModifier {
			return row, l.fail("severity.damage_by_level %d-%d: %q is not dice such as \"2d10\"", r.From, r.To, one.text)
		}
		*one.into = d
	}
	return row, nil
}

// trap checks one trap and adds it to the presets.
func (l trapLoader) trap(in trapJSON, seen map[string]bool) error {
	key := "trap:" + in.Key
	if err := l.trapHead(in, key, seen); err != nil {
		return err
	}
	seen[key] = true
	p := TrapPreset{
		Key: key, NamePT: l.c.namesPT[key], Kind: in.Kind, DescriptionPT: in.DescriptionPT, NoticeDC: in.NoticeDC, FindDC: in.FindDC, FindDCIsOurs: in.FindDCOurs,
		Trigger: in.Trigger, AreaSize: in.AreaSize, Targets: in.Targets, FallFt: in.FallFt,
	}
	if p.NamePT == "" {
		return l.fail("%s has no Portuguese name in effects/names_pt.json", key)
	}
	var err error
	if p.Damage, err = l.damages(trapAt(key, "damage"), in.Damage); err != nil {
		return err
	}
	if err := l.pit(in, key, p.Damage); err != nil {
		return err
	}
	if p.Conditions, err = l.conditions(in, key); err != nil {
		return err
	}
	if p.Attack, err = l.attack(in, key); err != nil {
		return err
	}
	if p.Save, err = l.save(in, key); err != nil {
		return err
	}
	l.c.traps.presets = append(l.c.traps.presets, p)
	return nil
}

// trapAt names a part of a trap in an error: "trap:a damage".
func trapAt(key, part string) string { return key + " " + part }

// trapHead checks the identity of a trap, then its plain numbers and words.
func (l trapLoader) trapHead(in trapJSON, key string, seen map[string]bool) error {
	switch {
	case in.Key == "" || strings.ContainsAny(in.Key, " :") || seen[key]:
		return l.fail("trap key %q is empty, has a space or colon, or is repeated", in.Key)
	case in.Kind != TrapMechanical && in.Kind != TrapMagic:
		return l.fail("%s: kind must be mechanical or magic", key)
	case strings.TrimSpace(in.DescriptionPT) == "":
		return l.fail("%s has no description_pt", key)
	}
	return l.trapNumbers(in, key)
}

// trapNumbers checks the DCs, the trigger, the area, who is caught and the fall.
func (l trapLoader) trapNumbers(in trapJSON, key string) error {
	switch {
	case in.NoticeDC < 0 || in.NoticeDC > maxTrapDC || in.FindDC < 1 || in.FindDC > maxTrapDC:
		return l.fail("%s: the DCs must be 1 to %d (notice_dc may be left out)", key, maxTrapDC)
	case in.Trigger != TrapTriggerEnter && in.Trigger != TrapTriggerManual:
		return l.fail("%s: trigger must be enter or manual", key)
	case in.AreaSize < 1 || in.AreaSize > maxTrapArea:
		return l.fail("%s: area_size must be 1 to %d", key, maxTrapArea)
	case in.Targets != TrapTargetsArea && in.Targets != TrapTargetsManual:
		return l.fail("%s: targets must be area or manual", key)
	case in.FallFt < 0 || in.FallFt%fallStepFt != 0 || in.FallFt > maxFallFt:
		return l.fail("%s: fall_ft must be a multiple of %d, up to %d", key, fallStepFt, maxFallFt)
	}
	return nil
}

// damage checks one damage: dice (or a flat number) of a damage type.
func (l trapLoader) damage(where string, in trapDamageJSON) (TrapDamage, error) {
	d, ok := ParseDice(in.Dice)
	if !ok || d.AddsModifier || (d.Count == 0 && d.Bonus < 1) || (d.Count > 0 && d.Bonus != 0) {
		return TrapDamage{}, l.fail("%s: %q is not dice such as \"2d10\" or a flat number", where, in.Dice)
	}
	if !strings.HasPrefix(in.Type, "damage-type:") || !l.c.exists(in.Type) {
		return TrapDamage{}, l.fail("%s: %q is not a damage type", where, in.Type)
	}
	return TrapDamage{Dice: d, Type: in.Type}, nil
}

func (l trapLoader) damages(where string, in []trapDamageJSON) ([]TrapDamage, error) {
	var out []TrapDamage
	for i, d := range in {
		td, err := l.damage(fmt.Sprintf("%s[%d]", where, i), d)
		if err != nil {
			return nil, err
		}
		out = append(out, td)
	}
	return out, nil
}

func (l trapLoader) condition(where, key string) error {
	if !strings.HasPrefix(key, "condition:") || !l.c.exists(key) {
		return l.fail("%s: %q is not a condition", where, key)
	}
	return nil
}

// pit checks that a pit's first damage is the fall.
func (l trapLoader) pit(in trapJSON, key string, damage []TrapDamage) error {
	if in.FallFt <= 0 {
		return nil
	}
	want := FallDice(in.FallFt)
	if len(damage) == 0 || damage[0].Dice != want || damage[0].Type != "damage-type:bludgeoning" {
		return l.fail("%s: a pit's first damage is the fall, %dd6 bludgeoning for %d ft", key, want.Count, in.FallFt)
	}
	return nil
}

func (l trapLoader) conditions(in trapJSON, key string) ([]TrapCondition, error) {
	var out []TrapCondition
	for i, cond := range in.Conditions {
		if err := l.condition(trapAt(key, fmt.Sprintf("conditions[%d]", i)), cond.Condition); err != nil {
			return nil, err
		}
		out = append(out, TrapCondition(cond))
	}
	return out, nil
}

func (l trapLoader) attack(in trapJSON, key string) (*TrapAttack, error) {
	a := in.Attack
	if a == nil {
		return nil, nil
	}
	if a.Bonus < 0 || a.Bonus > maxTrapAttackBonus || a.Count < 1 || a.Count > maxTrapAttacks {
		return nil, l.fail("%s: an attack needs a bonus of 0 to %d and 1 to %d attacks", key, maxTrapAttackBonus, maxTrapAttacks)
	}
	d, err := l.damage(trapAt(key, "attack.damage"), a.Damage)
	if err != nil {
		return nil, err
	}
	return &TrapAttack{Bonus: a.Bonus, Count: a.Count, Damage: d}, nil
}

// save checks the saving throw of a trap: who rolls it, against what, and
// what the failure and the pass do.
func (l trapLoader) save(in trapJSON, key string) (*TrapSave, error) {
	s := in.Save
	if s == nil {
		return nil, nil
	}
	ab, err := l.saveRoll(in, key)
	if err != nil {
		return nil, err
	}
	fl, err := l.saveFailure(s, key)
	if err != nil {
		return nil, err
	}
	if s.OnPass == TrapPassHalf && len(fl) == 0 {
		return nil, l.fail("%s: half damage on a pass needs damage on a fail", key)
	}
	return &TrapSave{
		Ability: ab, DC: s.DC, AppliesTo: s.AppliesTo, OnPass: s.OnPass,
		OnFail: TrapOnFail{Damage: fl, Condition: s.OnFail.Condition, DurationPT: s.OnFail.DurationPT},
	}, nil
}

// saveRoll checks the ability, the DC, who rolls and what a pass is.
func (l trapLoader) saveRoll(in trapJSON, key string) (Ability, error) {
	s := in.Save
	ab, ok := ability(s.Ability)
	if !ok {
		return "", l.fail("%s: a saving throw needs an ability (str, dex, con, int, wis or cha), got %q", key, s.Ability)
	}
	if s.DC < 1 || s.DC > maxTrapDC {
		return "", l.fail("%s: the saving throw's DC must be 1 to %d", key, maxTrapDC)
	}
	if s.AppliesTo != TrapSaveCaught && s.AppliesTo != TrapSaveHit {
		return "", l.fail("%s: save.applies_to must be caught or hit", key)
	}
	if s.AppliesTo == TrapSaveHit && in.Attack == nil {
		return "", l.fail("%s: a save that applies to the creatures hit needs an attack", key)
	}
	if s.OnPass != TrapPassHalf && s.OnPass != TrapPassNone {
		return "", l.fail("%s: save.on_pass must be half or none", key)
	}
	return ab, nil
}

// saveFailure checks what a failed save does: damage and/or a condition.
func (l trapLoader) saveFailure(s *trapSaveJSON, key string) ([]TrapDamage, error) {
	fl, err := l.damages(trapAt(key, "save.on_fail.damage"), s.OnFail.Damage)
	if err != nil {
		return nil, err
	}
	if len(fl) == 0 && s.OnFail.Condition == "" {
		return nil, l.fail("%s: a failed save must do damage or give a condition", key)
	}
	if s.OnFail.Condition != "" {
		if err := l.condition(trapAt(key, "save.on_fail"), s.OnFail.Condition); err != nil {
			return nil, err
		}
	}
	if s.OnFail.Condition == "" && s.OnFail.DurationPT != "" {
		return nil, l.fail("%s: save.on_fail has a duration but no condition", key)
	}
	return fl, nil
}

// clone is a copy of the preset that shares nothing with it.
func (p TrapPreset) clone() TrapPreset {
	p.Damage = slices.Clone(p.Damage)
	p.Conditions = slices.Clone(p.Conditions)
	if p.Attack != nil {
		a := *p.Attack
		p.Attack = &a
	}
	if p.Save != nil {
		s := *p.Save
		s.OnFail.Damage = slices.Clone(s.OnFail.Damage)
		p.Save = &s
	}
	return p
}

// TrapPresets returns the SRD's sample traps, in the order of
// effects/traps.json, for the master's form. The caller gets a deep copy.
func (c *Content) TrapPresets() []TrapPreset {
	out := make([]TrapPreset, len(c.c.traps.presets))
	for i, p := range c.c.traps.presets {
		out[i] = p.clone()
	}
	return out
}

// TrapPreset returns a deep copy of the preset with a key such as
// "trap:poison-needle".
func (c *Content) TrapPreset(key string) (TrapPreset, bool) {
	for _, p := range c.c.traps.presets {
		if p.Key == key {
			return p.clone(), true
		}
	}
	return TrapPreset{}, false
}

// TrapSeverities returns the SRD's three levels of trap severity (Revés,
// Perigosa, Mortal) with the save DCs and attack bonuses that suit them: the
// hint the master's form shows.
func (c *Content) TrapSeverities() []TrapSeverity {
	return slices.Clone(c.c.traps.severities)
}

// TrapDamageByLevel returns the SRD's damage that suits each severity at four
// ranges of character levels.
func (c *Content) TrapDamageByLevel() []TrapDamageRow {
	return slices.Clone(c.c.traps.damageRows)
}
