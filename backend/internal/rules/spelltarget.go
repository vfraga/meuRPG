package rules

import (
	"fmt"
	"io/fs"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/PuraFome/meuRPG/backend/internal/rules/srd51"
)

// Whom a spell reaches (MR-025, MR-045, RN-23). A table spell says it in its
// own fields (SpellTarget, written by the master). An SRD spell says it in the
// 5e-database's structured area_of_effect (a shape and a size in feet) when it has
// one, and otherwise in prose, which this file reads with two patterns so a wrong
// guess has one place to fix (the master is never held to the number of targets,
// so a wrong guess never blocks a table).

var (
	// An area is a shape ("20-foot radius", "15-foot cone", "100-foot-long
	// line"), a point ("within 20 feet of a point") or "up to three creatures".
	areaRE = regexp.MustCompile(`(?i)\b\d+-foot[- ](radius|cone|cube|line|square|sphere|cylinder|long|wide)|within \d+ feet of a point|\bup to (two|three|four|five|six|seven|eight|nine|ten|twelve) (other )?(creatures|humanoids|willing creatures)|\bcreatures of your choice`)
	// A spell that gets "one additional creature" at a higher level takes one more
	// target for each level. Magic Missile's darts and a spell attack's targets are
	// not areas: the play module counts them.
	extraTargetRE = regexp.MustCompile(`(?i)additional (creature|target|humanoid)|one additional`)
)

// textArea says whether the SRD's prose makes a spell hit any number of targets:
// a spell attack takes one, a healing spell that is not mass or a prayer takes one,
// and a spell that comes out of the caster and damages (Mãos Flamejantes, Onda
// Trovejante) is an area; the rest is read from the description. It is the
// fallback for a spell the database gives no area_of_effect.
func textArea(s *srd51.Spell) bool {
	switch {
	case s.AttackType != "":
		return false
	case s.Key == "spell:magic-missile":
		return false
	case len(s.HealAtSlotLevel) > 0:
		return strings.HasPrefix(s.Key, "spell:mass-") || s.Key == "spell:prayer-of-healing"
	case s.Range == "Self" && (s.SaveAbility != "" || len(s.Damage) > 0):
		return true
	}
	return areaRE.MatchString(strings.Join(s.Desc, " "))
}

// textExtraTarget says the prose gives a spell one more target for each slot
// level above its own.
func textExtraTarget(s *srd51.Spell) bool {
	return extraTargetRE.MatchString(strings.Join(s.HigherLevel, " "))
}

// spellTargetFile is the shape of effects/spell_targets.json: the hand-written
// overrides of srdTarget, for the spells whose structured area or text says the
// wrong thing.
type spellTargetFile struct {
	Comment string                         `json:"_comment"`
	Spells  map[string]spellTargetOverride `json:"spells"`
}

type spellTargetOverride struct {
	Kind         string `json:"kind"`
	Count        int    `json:"count,omitempty"`
	PerSlotLevel int    `json:"per_slot_level,omitempty"`
	Shape        string `json:"shape,omitempty"`
	SizeFt       int    `json:"size_ft,omitempty"`
	LabelPT      string `json:"label_pt,omitempty"`
}

// loadSpellTargets reads and checks effects/spell_targets.json: every key is a
// spell of the content, the kinds are closed, and an area has a shape and a size
// in steps of 5 ft.
func (c *content) loadSpellTargets(fsys fs.FS) error {
	var f spellTargetFile
	if err := readJSON(fsys, "effects/spell_targets.json", &f); err != nil {
		return err
	}
	c.srdTargets = map[string]SpellTarget{}
	for _, key := range sortedKeys(f.Spells) {
		in := f.Spells[key]
		fail := func(format string, a ...any) error {
			return fmt.Errorf("effects/spell_targets.json: %s: %s", key, fmt.Sprintf(format, a...))
		}
		if _, ok := c.spells[key]; !ok {
			return fmt.Errorf("effects/spell_targets.json: %q is not a spell of the content", key)
		}
		t := SpellTarget{Kind: in.Kind, Count: in.Count, PerSlotLevel: in.PerSlotLevel, Shape: in.Shape, SizeFt: in.SizeFt, Label: in.LabelPT}
		switch in.Kind {
		case TargetSelf, TargetNone, TargetCreature:
			if in.Count != 0 || in.Shape != "" || in.SizeFt != 0 || (in.Kind != TargetCreature && in.PerSlotLevel != 0) {
				return fail("a %s target takes no other field", in.Kind)
			}
		case TargetCreatures:
			if in.Count < 1 || in.Count > 20 || in.PerSlotLevel < 0 || in.PerSlotLevel > 10 || in.Shape != "" || in.SizeFt != 0 {
				return fail("creatures: a count of 1 to 20 and 0 to 10 more per slot level")
			}
		case TargetArea:
			if !slices.Contains([]string{ShapeCone, ShapeCube, ShapeCylinder, ShapeLine, ShapeSphere}, in.Shape) || in.SizeFt < 5 || in.SizeFt%5 != 0 || in.Count != 0 || in.PerSlotLevel != 0 {
				return fail("an area is a cone, cube, cylinder, line or sphere of a multiple of 5 ft")
			}
		default:
			return fail("the kind %q is not creature, creatures, area, self or none", in.Kind)
		}
		if utf8.RuneCountInString(in.LabelPT) > 80 || strings.ContainsFunc(in.LabelPT, isHiddenRune) {
			return fail("label_pt is one line of at most 80 characters")
		}
		c.srdTargets[key] = t
	}
	return nil
}

// srdTarget says whom an SRD spell reaches: the hand-written override when there
// is one (effects/spell_targets.json), then the structured area of the database,
// then the prose. The kinds are the table's (SpellTarget), with differences: "creatures" with a
// Count of 0 is "as many as the caster picks" (the text says how many; the master
// is never held to it), "creature" may take PerSlotLevel more for each circle
// above (Hold Person), and "none" is no creature at all (a point, an object, a
// place).
func (c *content) srdTarget(s *srd51.Spell) SpellTarget {
	if t, ok := c.srdTargets[s.Key]; ok {
		return t
	}
	extra := 0
	if textExtraTarget(s) {
		extra = 1
	}
	switch {
	case s.AreaType != "":
		return SpellTarget{Kind: TargetArea, Shape: s.AreaType, SizeFt: s.AreaSizeFt}
	case s.Key == "spell:magic-missile", s.Key == "spell:scorching-ray":
		// A dart or a ray each: three at the spell's level and one more for each
		// circle above, as many targets as there are darts or rays.
		return SpellTarget{Kind: TargetCreatures, Count: 3, PerSlotLevel: 1}
	case s.AttackType != "":
		return SpellTarget{Kind: TargetCreature}
	case textArea(s):
		return SpellTarget{Kind: TargetCreatures, PerSlotLevel: extra}
	case s.Range == "Self":
		return SpellTarget{Kind: TargetSelf}
	}
	return SpellTarget{Kind: TargetCreature, PerSlotLevel: extra}
}

// AnyNumber says the spell takes any number of targets: an area, or "creatures"
// without a count (the text says how many and the caster picks).
func (t SpellTarget) AnyNumber() bool {
	return t.Kind == TargetArea || (t.Kind == TargetCreatures && t.Count == 0)
}

// LabelPT is the target as the "Magias" page and the spell editor's preview read
// it: "Uma criatura", "Várias criaturas", "Só quem conjura" or an area, in meters
// ("Cone de 4,5 m"). Empty for a spell without a target (none today: every spell
// of the content has one).
func (t SpellTarget) LabelPT() string {
	if t.Label != "" {
		return t.Label
	}
	switch t.Kind {
	case TargetSelf:
		return "Só quem conjura"
	case TargetNone:
		return "Nenhuma criatura"
	case TargetCreature:
		return "Uma criatura"
	case TargetCreatures:
		return "Várias criaturas"
	case TargetArea:
		shape := map[string]string{ShapeCone: "Cone", ShapeCube: "Cubo", ShapeCylinder: "Cilindro", ShapeLine: "Linha", ShapeSphere: "Esfera"}[t.Shape]
		if shape == "" {
			return "Área"
		}
		return shape + " de " + MetersPT(t.SizeFt)
	}
	return ""
}

// MetersPT writes a distance in feet in the table's units (5 ft = 1,5 m, so the
// meters are the feet times 0,3): "4,5 m", "18 m", and kilometers from 1 000 m
// ("1,6 km"). It is the text of the app's formatMeters and formatRangeFt.
func MetersPT(feet int) string {
	tenths := feet * 3 // tenths of a meter
	if tenths >= 10000 {
		km := (tenths + 500) / 1000 // tenths of a kilometer, rounded
		return decimalPT(km) + " km"
	}
	return decimalPT(tenths) + " m"
}

// decimalPT writes tenths as a number with a comma: 45 is "4,5", 180 is "18".
func decimalPT(tenths int) string {
	if tenths%10 == 0 {
		return strconv.Itoa(tenths / 10)
	}
	return fmt.Sprintf("%d,%d", tenths/10, tenths%10)
}
