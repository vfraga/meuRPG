package rules

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/PuraFome/meuRPG/backend/internal/rules/srd51"
)

// maxSpellDice bounds the dice count of a table spell's damage or healing, per
// roll and per level, so the sums stay far from anything the formulas bound.
const maxSpellDice = 30

// tableDiceSides are the dice that exist at a table, the same the dice package
// rolls (a test keeps the two lists equal): a spell's damage or healing never
// asks for a die the physical dice or the combat cannot roll.
var tableDiceSides = []int{4, 6, 8, 10, 12, 20, 100}

// plainDice parses "8d6": dice only, no bonus and no modifier, of a die that exists.
func plainDice(s string) (DiceFormula, bool) {
	f, ok := ParseDice(s)
	return f, ok && slices.Contains(tableDiceSides, f.Sides) && f.Count >= 1 && f.Count <= maxSpellDice && f.Bonus == 0 && !f.AddsModifier && !strings.ContainsAny(s, "+- ")
}

// extraDice checks an optional "extra dice" text of the same die as base: ""
// (none) or plain dice. It returns the count (0 for none) and whether it is fine.
func extraDice(s string, base DiceFormula) (int, bool) {
	if s == "" {
		return 0, true
	}
	f, ok := plainDice(s)
	if !ok || f.Sides != base.Sides {
		return 0, false
	}
	return f.Count, true
}

// addSpell registers a table spell and its structured details: the SRD's own
// strings are written from the structured fields and parsed back by the SRD's
// builder, so the table's spell is exactly as the combat code reads an SRD spell.
// Every field that is wrong is reported, each at its own path ("range.distance_ft",
// "damage[1].dice", "desc_pt[0]"...), in one error (OverlayError.More).
func (b *overlayBuilder) addSpell(ts *TableSpell, path string) error {
	key := ts.Key
	var c entryErrors
	if ts.Level < 0 || ts.Level > 9 {
		c.at(key, path, ".level", ReasonValue, "the spell level is 0 to 9")
	}
	if sch, ok := b.base.named[ts.School]; !ok || sch == nil || !strings.HasPrefix(ts.School, "school:") {
		c.at(key, path, ".school_key", ReasonReference, "school %q is not in the SRD", ts.School)
	}
	castingTime := castingTimeText(&c, key, path, ts.CastingTime)
	rng := rangeText(&c, key, path, ts.Range)
	duration := durationText(&c, key, path, ts.Duration, ts.Concentration)
	if ts.Ritual && ts.Level == 0 {
		c.at(key, path, ".ritual", ReasonValue, "a cantrip is not a ritual")
	}
	comps := []string{}
	if ts.Components.Verbal {
		comps = append(comps, "V")
	}
	if ts.Components.Somatic {
		comps = append(comps, "S")
	}
	if ts.Components.Material {
		comps = append(comps, "M")
	}
	if ts.Components.Material != (ts.Components.MaterialPT != "") {
		c.at(key, path, ".components.material_pt", ReasonValue, "the material text goes with the M component, and only with it")
	}
	checkTextAt(&c, key, path, ".desc_pt", ts.DescPT)
	checkTextAt(&c, key, path, ".higher_level_pt", ts.HigherLevelPT)
	checkTextAt(&c, key, path, ".components.material_pt", []string{ts.Components.MaterialPT})
	var classes []string
	for i, cl := range ts.Classes {
		switch {
		case !b.isClass(cl):
			c.at(key, path, fmt.Sprintf(".class_keys[%d]", i), ReasonReference, "the class %q of the spell list does not exist", cl)
		case slices.Contains(classes, cl):
			c.at(key, path, fmt.Sprintf(".class_keys[%d]", i), ReasonValue, "the class %q is listed twice", cl)
		default:
			classes = append(classes, cl)
		}
	}
	target := checkTarget(&c, key, path, ts.Target)
	if target.Kind == TargetSelf && ts.Range.Kind != RangeSelf {
		c.at(key, path, ".range.kind", ReasonValue, "a spell that only affects the caster has the range Self (Pessoal)")
	}
	if ts.Range.Kind == RangeSelf && (target.Kind == TargetCreature || target.Kind == TargetCreatures) {
		// Pessoal reaches the caster, or an area that comes out of the caster
		// (Mãos Flamejantes): a spell that picks creatures has a distance or Toque.
		c.at(key, path, ".range.kind", ReasonValue, "a spell with the range Self (Pessoal) reaches only the caster or an area; one that picks creatures has a distance or Touch (Toque)")
	}
	if target.Kind == TargetArea && ts.Attack != "" {
		c.at(key, path, ".attack", ReasonValue, "a spell attack hits one creature: it cannot have an area target")
	}

	sp := &srd51.Spell{
		Key: key, Name: ts.NamePT, Level: ts.Level, School: ts.School, Classes: classes,
		Ritual: ts.Ritual, Concentration: ts.Concentration, CastingTime: castingTime, Range: rng,
		Duration: duration, Components: comps, Material: ts.Components.MaterialPT,
		Desc: slices.Clone(ts.DescPT), HigherLevel: slices.Clone(ts.HigherLevelPT),
	}
	switch ts.Attack {
	case "":
	case "melee", "ranged":
		sp.AttackType = ts.Attack
	default:
		c.at(key, path, ".attack", ReasonValue, "the spell attack is melee or ranged")
	}
	if ts.Save != nil {
		if sp.AttackType != "" {
			c.at(key, path, ".save", ReasonValue, "a spell has an attack or a saving throw, not both")
		}
		if _, ok := abilityIndex[ts.Save.Ability]; !ok {
			c.at(key, path, ".save.ability", ReasonValue, "the saving throw is not one of the six abilities")
		}
		if ts.Save.OnSuccess != "half" && ts.Save.OnSuccess != "none" {
			c.at(key, path, ".save.on_success", ReasonValue, "on a successful save the spell does half or nothing")
		}
		sp.SaveAbility, sp.SaveSuccess = string(ts.Save.Ability), ts.Save.OnSuccess
	}
	if len(ts.Damage) > 4 {
		c.at(key, path, ".damage", ReasonLimit, "a spell has at most 4 damage types")
	}
	for i, d := range ts.Damage {
		if sd, ok := b.spellDamage(&c, key, path, i, ts.Level, d); ok {
			sp.Damage = append(sp.Damage, sd)
		}
	}
	if ts.Save != nil && ts.Save.OnSuccess == "half" && len(ts.Damage) == 0 {
		c.at(key, path, ".save.on_success", ReasonValue, "a save for half damage needs damage")
	}
	if ts.Heal != nil {
		sp.HealAtSlotLevel = spellHeal(&c, key, path, ts.Level, *ts.Heal)
	}
	if err := c.err(); err != nil {
		return err
	}

	b.register(ts.TableEntry)
	b.n.spells[key] = sp
	b.n.spellTargets[key] = target
	return nil
}

// spellDamage writes one damage type as the SRD's table: by slot level for a
// leveled spell, by character level (the tiers 1, 5, 11 and 17) for a cantrip.
// A problem goes to c, at damage[i] and the field it is about.
func (b *overlayBuilder) spellDamage(c *entryErrors, key, path string, i, level int, d TableSpellDamage) (srd51.SpellDamage, bool) {
	at := fmt.Sprintf(".damage[%d]", i)
	ok := true
	if dt, found := b.base.named[d.Type]; !found || dt == nil || !strings.HasPrefix(d.Type, "damage-type:") {
		c.at(key, path, at+".damage_type_key", ReasonReference, "damage type %q is not in the SRD", d.Type)
		ok = false
	}
	base, plain := plainDice(d.Dice)
	if !plain {
		c.at(key, path, at+".dice", ReasonValue, "damage dice %q are not plain dice of a d4, d6, d8, d10, d12, d20 or d100, such as 8d6", d.Dice)
		return srd51.SpellDamage{}, false
	}
	out := srd51.SpellDamage{DamageType: d.Type}
	if level == 0 {
		if d.PerSlotLevel != "" {
			c.at(key, path, at+".per_slot_level", ReasonValue, "a cantrip grows by tier, not by slot level")
			ok = false
		}
		per, fine := extraDice(d.PerTier, base)
		if !fine {
			c.at(key, path, at+".per_tier", ReasonValue, "the dice per tier %q are not plain dice of the same die as the base (%dd%d)", d.PerTier, base.Count, base.Sides)
			return out, false
		}
		out.AtCharacterLevel = map[string]string{}
		for i, lvl := range []int{1, 5, 11, 17} {
			out.AtCharacterLevel[strconv.Itoa(lvl)] = diceText(base.Count+per*i, base.Sides)
		}
		if per == 0 {
			out.AtCharacterLevel = map[string]string{"1": diceText(base.Count, base.Sides)}
		}
		return out, ok
	}
	if d.PerTier != "" {
		c.at(key, path, at+".per_tier", ReasonValue, "only a cantrip grows by tier")
		ok = false
	}
	per, fine := extraDice(d.PerSlotLevel, base)
	if !fine {
		c.at(key, path, at+".per_slot_level", ReasonValue, "the dice per slot level %q are not plain dice of the same die as the base (%dd%d)", d.PerSlotLevel, base.Count, base.Sides)
		return out, false
	}
	out.AtSlotLevel = map[string]string{}
	for slot := level; slot <= 9; slot++ {
		out.AtSlotLevel[strconv.Itoa(slot)] = diceText(base.Count+per*(slot-level), base.Sides)
	}
	return out, ok
}

// spellHeal writes the healing as the SRD's table ("1d8 + MOD" by slot level).
func spellHeal(c *entryErrors, key, path string, level int, h TableSpellHeal) map[string]string {
	if level == 0 {
		c.at(key, path, ".heal", ReasonValue, "a cantrip does not heal")
		return nil
	}
	base, ok := plainDice(h.Dice)
	if !ok {
		c.at(key, path, ".heal.dice", ReasonValue, "healing dice %q are not plain dice of a d4, d6, d8, d10, d12, d20 or d100, such as 1d8", h.Dice)
		return nil
	}
	per, fine := extraDice(h.PerSlotLevel, base)
	if !fine {
		c.at(key, path, ".heal.per_slot_level", ReasonValue, "the healing dice per slot level %q are not plain dice of the same die as the base (%dd%d)", h.PerSlotLevel, base.Count, base.Sides)
		return nil
	}
	out := map[string]string{}
	for slot := level; slot <= 9; slot++ {
		text := diceText(base.Count+per*(slot-level), base.Sides)
		if h.AddsModifier {
			text += " + MOD"
		}
		out[strconv.Itoa(slot)] = text
	}
	return out
}

func diceText(count, sides int) string { return fmt.Sprintf("%dd%d", count, sides) }

// checkTarget checks a spell's target and returns it with only the fields its
// kind uses. A problem goes to c, at target.<field>.
func checkTarget(c *entryErrors, key, path string, t SpellTarget) SpellTarget {
	if t.Label != "" {
		c.at(key, path, ".target.label", ReasonValue, "a table spell's target has no text of its own: the server writes it")
	}
	perSlot := func() {
		if t.PerSlotLevel < 0 || t.PerSlotLevel > 10 {
			c.at(key, path, ".target.per_slot_level", ReasonLimit, "0 to 10 more per slot level")
		}
	}
	switch t.Kind {
	case TargetSelf:
		if t.Count != 0 || t.PerSlotLevel != 0 || t.Shape != "" || t.SizeFt != 0 {
			c.at(key, path, ".target.kind", ReasonValue, "a %s target takes nothing else", t.Kind)
		}
	case TargetCreature:
		// One creature, and, if the master wants, one more for each circle above
		// ("uma criatura, mais uma por nível", as Hold Person).
		perSlot()
		if t.Count != 0 || t.Shape != "" || t.SizeFt != 0 {
			c.at(key, path, ".target.kind", ReasonValue, "one creature takes only 0 to 10 more per slot level")
		}
	case TargetCreatures:
		if t.Count < 2 || t.Count > 20 {
			c.at(key, path, ".target.count", ReasonLimit, "several creatures: a count of 2 to 20")
		}
		perSlot()
		if t.Shape != "" || t.SizeFt != 0 {
			c.at(key, path, ".target.kind", ReasonValue, "several creatures take no shape or size")
		}
	case TargetArea:
		if !slices.Contains([]string{ShapeCone, ShapeCube, ShapeCylinder, ShapeLine, ShapeSphere}, t.Shape) {
			c.at(key, path, ".target.shape", ReasonValue, "an area is a cone, cube, cylinder, line or sphere")
		}
		if t.SizeFt < 5 || t.SizeFt > 300 || t.SizeFt%5 != 0 {
			c.at(key, path, ".target.size_ft", ReasonLimit, "an area is 5 to 300 feet, in steps of 5")
		}
		if t.Count != 0 || t.PerSlotLevel != 0 {
			c.at(key, path, ".target.kind", ReasonValue, "an area takes no count")
		}
	default:
		c.at(key, path, ".target.kind", ReasonValue, "the target is self, creature, creatures or area")
	}
	return t
}

func castingTimeText(c *entryErrors, key, path string, ct TableCastingTime) string {
	if ct.TriggerPT != "" && ct.Unit != CastReaction {
		c.at(key, path, ".casting_time.trigger_pt", ReasonValue, "only a reaction has a trigger in its casting time")
	}
	switch ct.Unit {
	case CastAction, CastBonusAction, CastReaction:
		if ct.Amount != 1 && ct.Amount != 0 {
			c.at(key, path, ".casting_time.amount", ReasonLimit, "an action, bonus action or reaction takes 1")
		}
		text := "1 " + strings.ReplaceAll(ct.Unit, "_", " ")
		if ct.TriggerPT != "" && ct.Unit == CastReaction {
			if utf8.RuneCountInString(ct.TriggerPT) > 200 || strings.ContainsFunc(ct.TriggerPT, isHiddenRune) || strings.TrimSpace(ct.TriggerPT) != ct.TriggerPT {
				c.at(key, path, ".casting_time.trigger_pt", ReasonText, "the casting time trigger is one line of at most 200 characters")
			}
			// parseCastingTime reads what follows the first comma as the trigger.
			text += ", " + ct.TriggerPT
		}
		return text
	case CastMinute, CastHour:
		if ct.Amount < 1 || ct.Amount > 60 {
			c.at(key, path, ".casting_time.amount", ReasonLimit, "casting time is 1 to 60 %ss", ct.Unit)
			return ""
		}
		return plural(ct.Amount, ct.Unit)
	}
	c.at(key, path, ".casting_time.unit", ReasonValue, "the casting time is an action, bonus action, reaction, minutes or hours")
	return ""
}

func rangeText(c *entryErrors, key, path string, r TableRange) string {
	switch r.Kind {
	case RangeSelf:
		return "Self"
	case RangeTouch:
		return "Touch"
	case RangeSight:
		return "Sight"
	case RangeUnlimited:
		return "Unlimited"
	case RangeRanged:
		if r.DistanceFt < 5 || r.DistanceFt > 5280 || r.DistanceFt%5 != 0 {
			c.at(key, path, ".range.distance_ft", ReasonLimit, "the range is 5 to 5280 feet, in steps of 5")
			return ""
		}
		return strconv.Itoa(r.DistanceFt) + " feet"
	}
	c.at(key, path, ".range.kind", ReasonValue, "the range is self, touch, sight, unlimited or a distance")
	return ""
}

func durationText(c *entryErrors, key, path string, d TableDuration, concentration bool) string {
	switch d.Kind {
	case DurationInstantaneous:
		if concentration {
			c.at(key, path, ".concentration", ReasonValue, "concentration needs a timed duration")
		}
		return "Instantaneous"
	case DurationUntilDispelled:
		if concentration {
			c.at(key, path, ".concentration", ReasonValue, "concentration needs a timed duration")
		}
		return "Until dispelled"
	case DurationTimed:
		if d.Amount < 1 || d.Amount > 999 {
			c.at(key, path, ".duration.amount", ReasonLimit, "a timed duration is 1 to 999")
		}
		if !slices.Contains([]string{DurationRound, DurationMinute, DurationHour, DurationDay}, d.Unit) {
			c.at(key, path, ".duration.unit", ReasonValue, "a timed duration is in rounds, minutes, hours or days")
			return ""
		}
		text := plural(d.Amount, d.Unit)
		if d.UpTo || concentration {
			// A concentration spell always lasts "up to" its duration.
			text = "Up to " + text
		}
		return text
	}
	c.at(key, path, ".duration.kind", ReasonValue, "the duration is instantaneous, timed or until dispelled")
	return ""
}

// checkTextAt bounds a text made of paragraphs: too many is a limit at attr, a
// paragraph that is too long is bad_text at attr[i] (at attr itself for the
// material text, which is one string).
func checkTextAt(c *entryErrors, key, path, attr string, paragraphs []string) {
	if len(paragraphs) > maxTextParagraphs {
		c.at(key, path, attr, ReasonLimit, "the text has more than %d paragraphs", maxTextParagraphs)
	}
	for i, p := range paragraphs {
		if utf8.RuneCountInString(p) > maxTextRunes {
			at := attr
			if attr != ".components.material_pt" && attr != ".equipment_pt" {
				at = fmt.Sprintf("%s[%d]", attr, i)
			}
			c.at(key, path, at, ReasonText, "a paragraph has more than %d characters", maxTextRunes)
		}
	}
}

func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return strconv.Itoa(n) + " " + unit + "s"
}
