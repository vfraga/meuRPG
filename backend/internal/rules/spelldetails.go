package rules

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/PuraFome/meuRPG/backend/internal/rules/srd51"
)

// This file turns the SRD's spell strings ("1 action", "60 feet", "Up to 1
// minute") into structured values. The web shows them in Portuguese (60 ft
// becomes "18 m"); the engine never writes that text. Whatever does not fit
// the structure is kept in a Raw field, exactly as the SRD wrote it.

// Casting time units.
const (
	CastAction      = "action"
	CastBonusAction = "bonus_action"
	CastReaction    = "reaction"
	CastMinute      = "minute"
	CastHour        = "hour"
)

// CastingTime is "1 action", "1 bonus action", "10 minutes"...
type CastingTime struct {
	Amount int
	// Unit is one of the Cast* constants, or "" when Raw did not parse.
	Unit string
	// Trigger is the English text after the comma of a reaction ("which
	// you take when..."), empty when the SRD has none. The 5e-database
	// keeps just "1 reaction", so today it is always empty.
	Trigger string
	Raw     string
}

// Economy is the part of the turn the casting time spends: an Economy* constant for an action, a bonus action or a reaction,
// and "" for a longer casting (a minute, a ritual), which spends none of it.
func (ct CastingTime) Economy() string {
	switch ct.Unit {
	case CastAction:
		return EconomyAction
	case CastBonusAction:
		return EconomyBonusAction
	case CastReaction:
		return EconomyReaction
	}
	return ""
}

// Range kinds.
const (
	RangeSelf      = "self"
	RangeTouch     = "touch"
	RangeRanged    = "ranged"
	RangeSight     = "sight"
	RangeUnlimited = "unlimited"
	RangeSpecial   = "special"
)

// SpellRange is how far a spell reaches.
type SpellRange struct {
	// Kind is one of the Range* constants.
	Kind string
	// DistanceFt is set for RangeRanged (a mile is 5280 ft).
	DistanceFt int
	Raw        string
}

// SpellComponents are the V, S and M components.
type SpellComponents struct {
	Verbal, Somatic, Material bool
	// MaterialText is the SRD's English description of the material, and
	// MaterialTextPT ours in Portuguese ("" when the spell has none, or its
	// Portuguese text is missing, see SpellDetails.TextPTMissing).
	MaterialText   string
	MaterialTextPT string
}

// Duration kinds.
const (
	DurationInstantaneous  = "instantaneous"
	DurationTimed          = "timed"
	DurationUntilDispelled = "until_dispelled"
	DurationSpecial        = "special"
)

// Duration units.
const (
	DurationRound  = "round"
	DurationMinute = "minute"
	DurationHour   = "hour"
	DurationDay    = "day"
)

// SpellDuration is how long a spell lasts.
type SpellDuration struct {
	// Kind is one of the Duration* constants.
	Kind string
	// Amount and Unit (a Duration* unit) are set for DurationTimed; UpTo
	// says "up to" (the spell may end sooner).
	Amount int
	Unit   string
	UpTo   bool
	// Concentration says the caster must keep concentrating.
	Concentration bool
	Raw           string
}

// SpellSave is the saving throw a spell asks for.
type SpellSave struct {
	Ability Ability
	// OnSuccess is "half" (half damage), "none" (no effect) or "other"
	// (something else, in the description).
	OnSuccess string
}

// SpellDamage is one damage type of a spell. The dice are the SRD's text
// by level ("8d6"); DamageAt and ParseDice turn them into numbers.
type SpellDamage struct {
	// Type is a key such as "damage-type:fire", and TypeNamePT its
	// Portuguese name.
	Type, TypeNamePT string
	// BySlotLevel is the dice by the slot level the spell is cast with,
	// ByCharacterLevel by the character's level (cantrips, at the levels
	// where it grows: 1, 5, 11, 17).
	BySlotLevel      map[int]string
	ByCharacterLevel map[int]string
}

// Spell target kinds, as in SpellTarget.Kind.
const (
	// TargetSelf: the caster only.
	TargetSelf = "self"
	// TargetCreature: one creature.
	TargetCreature = "creature"
	// TargetCreatures: several creatures, TargetCount of them at the spell's
	// own level, and PerSlotLevel more for each slot level above it.
	TargetCreatures = "creatures"
	// TargetArea: every creature in an area (Shape, SizeFt).
	TargetArea = "area"
	// TargetNone: no creature at all, a point, an object or a place (an SRD
	// spell's only: the table has none). There is nobody to pick.
	TargetNone = "none"
)

// Area shapes, as in SpellTarget.Shape.
const (
	ShapeCone     = "cone"
	ShapeCube     = "cube"
	ShapeCylinder = "cylinder"
	ShapeLine     = "line"
	ShapeSphere   = "sphere"
)

// SpellTarget says whom a spell reaches. A table spell has the master's own
// (RN-23); an SRD spell's is worked out from the 5e-database's structured area
// and, for the rest, from its text (spelltarget.go). The area's shape is data,
// never drawn by combat: it reads an area as "any number of targets" (MaxTargets
// 0). The engine keeps feet, and LabelPT writes the text ("Cone de 4,5 m").
type SpellTarget struct {
	// Kind is one of the Target* constants; "" only in a zero SpellTarget.
	Kind string
	// Count is how many creatures a TargetCreatures spell takes at its own
	// level, and PerSlotLevel how many more for each slot level above it.
	Count        int
	PerSlotLevel int
	// Shape (a Shape* constant) and SizeFt, in 5-foot steps, are the area of a
	// TargetArea spell: the cone's length, the cube's side, the cylinder's and
	// the sphere's radius, the line's length.
	Shape  string
	SizeFt int
	// Label, when set, is the text LabelPT writes instead of the one worked out
	// (an SRD override's: "Cilindro de 3 m de raio"). A table spell has none.
	Label string
}

// IsArea says the spell hits every creature in an area.
func (t SpellTarget) IsArea() bool { return t.Kind == TargetArea }

// CasterOnly says the spell has nobody to pick: it reaches the caster alone, or no
// creature at all.
func (t SpellTarget) CasterOnly() bool { return t.Kind == TargetSelf || t.Kind == TargetNone }

// MaxTargets is the most creatures the spell takes when cast with a slot of
// slotLevel (spellLevel is the spell's own level), or 0 for any number (see
// AnyNumber) and for a spell that only affects the caster: play's maxTargetsOf
// answers 0 for a self-only spell, because the caster is not a target the player
// picks.
func (t SpellTarget) MaxTargets(slotLevel, spellLevel int) int {
	switch t.Kind {
	case TargetCreature:
		return 1 + t.PerSlotLevel*max(slotLevel-spellLevel, 0)
	case TargetCreatures:
		if t.Count == 0 {
			return 0
		}
		return t.Count + t.PerSlotLevel*max(slotLevel-spellLevel, 0)
	}
	return 0
}

// SpellDetails is everything the SRD says about one spell, structured. The
// catalog keeps it apart from the light SpellEntry that ListContent returns,
// because the text is long: the app fetches it per spell (GetSpellDetails).
type SpellDetails struct {
	Spell       SpellEntry
	CastingTime CastingTime
	Range       SpellRange
	Components  SpellComponents
	Duration    SpellDuration
	// AttackType is "melee", "ranged" or "" (no spell attack).
	AttackType string
	// Save is nil when the spell asks for no saving throw.
	Save   *SpellSave
	Damage []SpellDamage
	// DamageChoice says what the caster picks among the Damage types: "scale"
	// (every type is dealt, and the higher-slot dice go to the chosen one),
	// "alternative" (only the chosen one is dealt) or "" (every type is dealt as
	// listed, nothing to pick). With "scale" each type's table is its dice when it
	// is the chosen one, and DamageAtChoosing says what a cast deals.
	DamageChoice string
	// HealBySlotLevel is the healing by slot level ("1d8 + MOD"), nil when
	// the spell does not heal.
	HealBySlotLevel map[int]string
	// Description and HigherLevel are the SRD's English paragraphs (the
	// table's, in Portuguese, for a table spell).
	Description []string
	HigherLevel []string
	// DescriptionPT and HigherLevelPT are our Portuguese translation of the SRD's
	// paragraphs (effects/spells_pt.json), one per English paragraph. For a table
	// spell they are the table's own text, the same as Description and HigherLevel
	// (TextPTOnly). Both are empty while TextPTMissing.
	DescriptionPT []string
	HigherLevelPT []string
	// TextPTMissing says an SRD spell has no Portuguese text yet: the English is
	// all there is, and the app shows it with a note.
	TextPTMissing bool
	// TextPTOnly says the text exists only in Portuguese (a table spell): there is
	// no English to offer.
	TextPTOnly bool
	// Target is whom the spell reaches (see SpellTarget).
	Target SpellTarget
}

// defaultLineWidthFt is the width of a line the SRD gives no other for: Lightning Bolt's 5 ft.
const defaultLineWidthFt = 5

var (
	// cornersRE is the SRD's own words for a spell whose area goes around corners
	// (Fireball: "The fire spreads around corners"). Message, which can "travel
	// freely around corners", is no area and does not match.
	cornersRE = regexp.MustCompile(`(?i)\bspreads around corners\b`)
	// widthRE is the width of a line, "10 feet wide" or "5-foot-wide".
	widthRE = regexp.MustCompile(`(?i)\b(\d+)[- ]f(?:oo|ee)t[- ]wide\b`)
)

// SpreadsAroundCorners says the spell's area reaches what a straight line from its
// origin does not, around a corner (SRD 5.1, Fireball and the other spheres that
// say it: its text says "spreads around corners"). Its area is then the part of the
// shape connected to the origin, not what the origin sees.
func (d *SpellDetails) SpreadsAroundCorners() bool {
	return d.Target.Kind == TargetArea && cornersRE.MatchString(strings.Join(d.Description, " "))
}

// AreaWidthFt is the width of a line area in feet: the one the spell's text gives
// ("10 feet wide" for Gust of Wind), and 5 ft, the SRD's width of Lightning Bolt and
// the rest, when it gives none. 0 for any other shape.
func (d *SpellDetails) AreaWidthFt() int {
	if d.Target.Kind != TargetArea || d.Target.Shape != ShapeLine {
		return 0
	}
	if m := widthRE.FindStringSubmatch(strings.Join(d.Description, " ")); m != nil {
		if w, err := strconv.Atoi(m[1]); err == nil && w >= defaultLineWidthFt && w%5 == 0 {
			return w
		}
	}
	return defaultLineWidthFt
}

// SpellDetails returns the details of a spell key, and whether it exists.
// The result is shared and must not be modified.
func (c *Content) SpellDetails(key string) (*SpellDetails, bool) {
	d, ok := c.c.spellDetails[key]
	return d, ok
}

// DamageRoll is one damage (or healing) roll of a spell at a level.
type DamageRoll struct {
	// Type is the damage type key; empty for healing.
	Type string
	// Raw is the SRD's dice text; Dice is it parsed, and Parsed false when
	// the text is not a plain formula ("4d6 OR 5d6").
	Raw    string
	Dice   DiceFormula
	Parsed bool
}

// DamageAt lists the damage rolls of the spell cast with a slot of
// slotLevel by a character of characterLevel. A level the table does not
// list uses the highest entry below it.
func (d *SpellDetails) DamageAt(slotLevel, characterLevel int) []DamageRoll {
	var out []DamageRoll
	for _, dm := range d.Damage {
		table, level := dm.BySlotLevel, slotLevel
		if len(dm.ByCharacterLevel) > 0 {
			table, level = dm.ByCharacterLevel, characterLevel
		}
		if raw, ok := atLevel(table, level); ok {
			out = append(out, newRoll(dm.Type, raw))
		}
	}
	return out
}

// DamageTypeChoices are the damage types the caster picks among, in the order
// the spell lists them; nil for a spell that has no choice (DamageChoice "").
func (d *SpellDetails) DamageTypeChoices() []string {
	if d.DamageChoice == "" {
		return nil
	}
	out := make([]string, 0, len(d.Damage))
	for _, dm := range d.Damage {
		out = append(out, dm.Type)
	}
	return out
}

// DamageAtChoosing lists the damage rolls of a cast in which the caster picked
// the damage type pick (one of DamageTypeChoices; "" picks the first). A spell
// with no choice answers as DamageAt. For "alternative" only the picked type is
// dealt. For "scale" every type is dealt, the picked one with its table at the
// slot level and the others with their dice at the spell's own level.
func (d *SpellDetails) DamageAtChoosing(slotLevel, characterLevel int, pick string) []DamageRoll {
	if d.DamageChoice == "" {
		return d.DamageAt(slotLevel, characterLevel)
	}
	if pick == "" {
		pick = d.Damage[0].Type
	}
	var out []DamageRoll
	for _, dm := range d.Damage {
		level := d.Spell.Level
		if dm.Type == pick {
			level = slotLevel
		} else if d.DamageChoice == "alternative" {
			continue
		}
		if raw, ok := atLevel(dm.BySlotLevel, level); ok {
			out = append(out, newRoll(dm.Type, raw))
		}
	}
	return out
}

// HealAt is the healing roll at a slot level, and whether the spell heals.
func (d *SpellDetails) HealAt(slotLevel int) (DamageRoll, bool) {
	raw, ok := atLevel(d.HealBySlotLevel, slotLevel)
	if !ok {
		return DamageRoll{}, false
	}
	return newRoll("", raw), true
}

func newRoll(damageType, raw string) DamageRoll {
	f, ok := ParseDice(raw)
	return DamageRoll{Type: damageType, Raw: raw, Dice: f, Parsed: ok}
}

// atLevel picks the entry for level, or the highest one below it.
func atLevel(table map[int]string, level int) (string, bool) {
	best, found := 0, false
	for l := range table {
		if l <= level && (!found || l > best) {
			best, found = l, true
		}
	}
	if !found {
		return "", false
	}
	return table[best], true
}

// buildSpellDetails structures one SRD spell.
func (c *content) buildSpellDetails(s *srd51.Spell, entry SpellEntry) *SpellDetails {
	d := &SpellDetails{
		Spell:        entry,
		CastingTime:  parseCastingTime(s.CastingTime),
		Range:        parseRange(s.Range),
		Components:   SpellComponents{MaterialText: s.Material},
		Duration:     parseDuration(s.Duration, s.Concentration),
		AttackType:   s.AttackType,
		DamageChoice: s.DamageChoice,
		Description:  s.Desc,
		HigherLevel:  s.HigherLevel,
	}
	switch pt, ok := c.spellTextsPT[s.Key]; {
	case ok:
		d.DescriptionPT, d.HigherLevelPT, d.Components.MaterialTextPT = pt.Description, pt.HigherLevel, pt.Material
	case strings.HasSuffix(s.Key, "@mesa"):
		d.DescriptionPT, d.HigherLevelPT, d.Components.MaterialTextPT = s.Desc, s.HigherLevel, s.Material
		d.TextPTOnly = true
	default:
		d.TextPTMissing = true
	}
	for _, comp := range s.Components {
		switch comp {
		case "V":
			d.Components.Verbal = true
		case "S":
			d.Components.Somatic = true
		case "M":
			d.Components.Material = true
		}
	}
	if s.SaveAbility != "" {
		d.Save = &SpellSave{Ability: Ability(s.SaveAbility), OnSuccess: s.SaveSuccess}
	}
	for _, dm := range s.Damage {
		d.Damage = append(d.Damage, SpellDamage{
			Type: dm.DamageType, TypeNamePT: c.namePT(dm.DamageType),
			BySlotLevel: levelTable(dm.AtSlotLevel), ByCharacterLevel: levelTable(dm.AtCharacterLevel),
		})
	}
	d.HealBySlotLevel = levelTable(s.HealAtSlotLevel)
	return d
}

// levelTable turns the SRD's {"2": "4d4"} into {2: "4d4"}.
func levelTable(in map[string]string) map[int]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[int]string, len(in))
	for k, v := range in {
		if l, err := strconv.Atoi(k); err == nil {
			out[l] = v
		}
	}
	return out
}

var castingUnits = map[string]string{
	"action": CastAction, "bonus action": CastBonusAction, "reaction": CastReaction,
	"minute": CastMinute, "minutes": CastMinute, "hour": CastHour, "hours": CastHour,
}

func parseCastingTime(raw string) CastingTime {
	ct := CastingTime{Raw: raw}
	main, trigger, _ := strings.Cut(raw, ",")
	ct.Trigger = strings.TrimSpace(trigger)
	n, unit, _ := strings.Cut(strings.TrimSpace(main), " ")
	amount, err := strconv.Atoi(n)
	if u, ok := castingUnits[unit]; ok && err == nil {
		ct.Amount, ct.Unit = amount, u
	}
	return ct
}

func parseRange(raw string) SpellRange {
	r := SpellRange{Raw: raw}
	switch strings.ToLower(raw) {
	case "self":
		r.Kind = RangeSelf
	case "touch":
		r.Kind = RangeTouch
	case "sight":
		r.Kind = RangeSight
	case "unlimited":
		r.Kind = RangeUnlimited
	case "special":
		r.Kind = RangeSpecial
	default:
		n, unit, _ := strings.Cut(raw, " ")
		amount, err := strconv.Atoi(n)
		switch {
		case err != nil:
			r.Kind = RangeSpecial
		case unit == "feet":
			r.Kind, r.DistanceFt = RangeRanged, amount
		case unit == "mile" || unit == "miles":
			r.Kind, r.DistanceFt = RangeRanged, amount*5280
		default:
			r.Kind = RangeSpecial
		}
	}
	return r
}

var durationUnits = map[string]string{
	"round": DurationRound, "rounds": DurationRound, "minute": DurationMinute, "minutes": DurationMinute,
	"hour": DurationHour, "hours": DurationHour, "day": DurationDay, "days": DurationDay,
}

func parseDuration(raw string, concentration bool) SpellDuration {
	d := SpellDuration{Raw: raw, Concentration: concentration}
	text := raw
	if rest, ok := strings.CutPrefix(text, "Up to "); ok {
		d.UpTo, text = true, rest
	}
	switch text {
	case "Instantaneous":
		d.Kind = DurationInstantaneous
		return d
	case "Until dispelled":
		d.Kind = DurationUntilDispelled
		return d
	}
	n, unit, _ := strings.Cut(text, " ")
	amount, err := strconv.Atoi(n)
	if u, ok := durationUnits[unit]; ok && err == nil {
		d.Kind, d.Amount, d.Unit = DurationTimed, amount, u
		return d
	}
	d.Kind, d.UpTo = DurationSpecial, false
	return d
}
