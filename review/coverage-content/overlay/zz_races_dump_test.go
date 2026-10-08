package rules

import (
	"encoding/json"
	"os"
	"sort"
	"testing"
)

// Coverage inventory (not part of the repo): the derived sheet of a level-3
// Fighter of every race/subrace, to see what each trait changes.
func TestCoverageDumpRaces(t *testing.T) {
	c, err := LoadSRD()
	if err != nil {
		t.Fatal(err)
	}
	type combo struct{ race, sub string }
	var combos []combo
	for k, r := range c.c.races {
		combos = append(combos, combo{k, ""})
		for _, s := range r.Subraces {
			combos = append(combos, combo{k, s})
		}
	}
	sort.Slice(combos, func(i, j int) bool { return combos[i].race+combos[i].sub < combos[j].race+combos[j].sub })
	type outRow struct {
		Race, Sub    string
		Speed        int
		Senses       []string
		SkillProf    []string
		Features     []string
		Hints        []string
		Resources    []string
		Actions      []string
		HPMax        int
		HPFromFx     int
		Languages    []string
		Profs        []string
		Issues       []string
		AC           int
		Spells       []string
		Choices      []string
	}
	var rows []outRow
	for _, cb := range combos {
		b := Build{
			BaseScores: map[Ability]int{STR: 15, DEX: 14, CON: 14, INT: 10, WIS: 10, CHA: 10},
			Race:       cb.race, Subrace: cb.sub,
			Classes:    []ClassLevel{{Class: "class:fighter", Level: 3}},
			Background: "background:acolyte",
		}
		d := Derive(b, c)
		r := outRow{Race: cb.race, Sub: cb.sub, Speed: d.SpeedWalkFt, HPMax: d.HitPointsMax, HPFromFx: d.HitPointsFromEffects, AC: d.ArmorClass}
		for _, s := range d.Senses {
			r.Senses = append(r.Senses, s.Key+":"+covItoa(s.RangeFt)+" from "+s.Source)
		}
		for _, s := range d.Skills {
			if s.Proficiency != ProficiencyNone {
				r.SkillProf = append(r.SkillProf, s.Key+"/"+covItoa(int(s.Proficiency)))
			}
		}
		for _, f := range d.Features {
			r.Features = append(r.Features, f.Key)
		}
		for _, h := range d.Hints {
			r.Hints = append(r.Hints, h.Source+": "+h.TextPT)
		}
		for _, x := range d.Resources {
			r.Resources = append(r.Resources, x.Key)
		}
		for _, x := range d.Actions {
			r.Actions = append(r.Actions, x.Key)
		}
		for _, l := range d.Languages {
			r.Languages = append(r.Languages, l.Key)
		}
		for _, p := range d.Proficiencies {
			r.Profs = append(r.Profs, p.Kind+":"+p.Key)
		}
		for _, i := range d.Issues {
			r.Issues = append(r.Issues, i.Message)
		}
		for _, s := range d.Spells {
			r.Spells = append(r.Spells, s.Spell.Key)
		}
		rows = append(rows, r)
	}
	bt, _ := json.MarshalIndent(rows, "", " ")
	if err := os.WriteFile(os.Getenv("COV_OUT"), bt, 0o644); err != nil {
		t.Fatal(err)
	}
}

func covItoa(n int) string { b, _ := json.Marshal(n); return string(b) }
