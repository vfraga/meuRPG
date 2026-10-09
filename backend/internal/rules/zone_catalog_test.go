package rules

import (
	"strings"
	"testing"

	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
	"github.com/PuraFome/meuRPG/backend/internal/rules/zone"
)

var saveWords = map[string]string{"dex": "dexterity", "con": "constitution", "wis": "wisdom", "str": "strength"}

// The catalog of the zones a spell leaves (package zone) is written from the SRD's text;
// the snapshot says the same numbers in its own fields, so a spell whose data moves, or
// a number typed wrong, fails here: level, concentration, the duration, the range and
// the saving throw of every spell, the shape and the size where the data has an area.
func TestZoneCatalogAgreesWithTheSpellData(t *testing.T) {
	t.Parallel()
	c, err := LoadSRD()
	if err != nil {
		t.Fatalf("LoadSRD() error = %v", err)
	}
	for _, key := range zone.Keys() {
		spec, ok := zone.Lookup(key)
		if !ok {
			t.Fatalf("zone.Lookup(%s) not found", key)
		}
		d, ok := c.SpellDetails(key)
		if !ok {
			t.Errorf("%s is not an SRD spell", key)
			continue
		}
		if d.Spell.Level != spec.Level {
			t.Errorf("%s level = %d, the catalog says %d", key, d.Spell.Level, spec.Level)
		}
		if d.Spell.Concentration != spec.Concentration {
			t.Errorf("%s concentration = %v, the catalog says %v", key, d.Spell.Concentration, spec.Concentration)
		}
		rounds := 0
		switch d.Duration.Unit {
		case DurationMinute:
			rounds = d.Duration.Amount * 10
		case DurationHour:
			rounds = d.Duration.Amount * 600
		}
		if rounds != spec.DurationRounds {
			t.Errorf("%s lasts %d rounds in the data (%s), the catalog says %d", key, rounds, d.Duration.Raw, spec.DurationRounds)
		}
		switch spec.Placement {
		case zone.PlacementCaster:
			if d.Range.Kind != RangeSelf {
				t.Errorf("%s is centered on the caster but its range is %q", key, d.Range.Raw)
			}
		default:
			if d.Range.Kind != RangeRanged || d.Range.DistanceFt == 0 {
				t.Errorf("%s is set at a point but its range is %q", key, d.Range.Raw)
			}
		}
		if d.SpreadsAroundCorners() != spec.SpreadsAroundCorners {
			t.Errorf("%s spreads around corners = %v in the text, the catalog says %v", key, d.SpreadsAroundCorners(), spec.SpreadsAroundCorners)
		}
		if d.Target.Kind == TargetArea {
			want := map[grid.Shape]string{
				grid.ShapeSphere: ShapeSphere, grid.ShapeCylinder: ShapeCylinder, grid.ShapeCube: ShapeCube, grid.ShapeWall: ShapeLine, grid.ShapeRing: ShapeLine,
			}[spec.Shape]
			if d.Target.Shape != want || d.Target.SizeFt != spec.SizeFt {
				t.Errorf("%s area = %s of %d ft, the catalog says %s of %d ft", key, d.Target.Shape, d.Target.SizeFt, spec.Shape, spec.SizeFt)
			}
		}
		text := strings.ToLower(strings.Join(d.Description, " "))
		for _, tr := range spec.Triggers {
			if tr.Save == "" {
				continue
			}
			switch {
			case d.Save != nil:
				if string(d.Save.Ability) != tr.Save {
					t.Errorf("%s trigger %s saves with %q, the data has %+v", key, tr.Kind, tr.Save, d.Save)
				}
			default:
				// Web's save is for being restrained later, so the data records none
				// (corrections.json says so): the text still asks for it.
				if !strings.Contains(text, saveWords[tr.Save]+" saving throw") {
					t.Errorf("%s trigger %s saves with %q but the text asks for no such saving throw", key, tr.Kind, tr.Save)
				}
			}
		}
	}
}

// Spirit Guardians has no area and no saving throw in the data's own rows (the
// corrections give it the Wisdom save and the damage), and its 15 feet are in the text.
func TestSpiritGuardiansIsCenteredOnTheCasterAtFifteenFeet(t *testing.T) {
	t.Parallel()
	spec, ok := zone.Lookup("spell:spirit-guardians")
	if !ok || spec.SizeFt != 15 || spec.Placement != zone.PlacementCaster || !spec.HalvesSpeed {
		t.Fatalf("spirit guardians = %+v", spec)
	}
	c, err := LoadSRD()
	if err != nil {
		t.Fatalf("LoadSRD() error = %v", err)
	}
	d, _ := c.SpellDetails("spell:spirit-guardians")
	if d.Save == nil || d.Save.Ability != "wis" || d.Save.OnSuccess != "half" {
		t.Errorf("spirit guardians save = %+v, want a Wisdom save that halves", d.Save)
	}
}
