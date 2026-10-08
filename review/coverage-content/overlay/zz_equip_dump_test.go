package rules

import (
	"encoding/json"
	"os"
	"sort"
	"testing"
)

// Coverage inventory (not part of the repo): weapons and armour through Derive.
func TestCoverageDumpEquip(t *testing.T) {
	c, err := LoadSRD()
	if err != nil {
		t.Fatal(err)
	}
	var weapons, armors []string
	for k, e := range c.c.equipment {
		if e.Weapon != nil {
			weapons = append(weapons, k)
		}
		if e.Armor != nil {
			armors = append(armors, k)
		}
	}
	sort.Strings(weapons)
	sort.Strings(armors)
	base := func(race string, str int) Build {
		return Build{
			BaseScores: map[Ability]int{STR: str, DEX: 14, CON: 14, INT: 10, WIS: 10, CHA: 10},
			Race:       race, Classes: []ClassLevel{{Class: "class:fighter", Level: 3}}, Background: "background:acolyte",
		}
	}
	type wrow struct {
		Key, Props      string
		Melee           bool
		Bonus           int
		Damage, Versatile string
		Range, Long     int
		Light           bool
		Ability         string
		Notes           string
		Hints           []string
	}
	type arow struct {
		Key, Race  string
		STR        int
		AC         int
		Desc       string
		Speed      int
		Hints      []string
		Issues     []string
	}
	out := map[string]any{}
	var ws []wrow
	for _, k := range weapons {
		b := base("race:human", 15)
		b.Weapons = []string{k}
		d := Derive(b, c)
		for _, a := range d.Attacks {
			if a.Key != k {
				continue
			}
			ws = append(ws, wrow{Key: k, Props: join(c.c.equipment[k].Weapon.Properties), Melee: a.Melee, Bonus: a.AttackBonus,
				Damage: a.Damage, Versatile: a.VersatileDamage, Range: a.RangeFt, Long: a.LongRangeFt, Light: a.Light, Ability: string(a.Ability), Notes: a.Notes})
		}
		for _, h := range d.Hints {
			if h.Source == k {
				ws[len(ws)-1].Hints = append(ws[len(ws)-1].Hints, h.TextPT)
			}
		}
	}
	out["weapons"] = ws
	var as []arow
	for _, k := range armors {
		for _, cfg := range []struct {
			race string
			str  int
		}{{"race:human", 8}, {"race:human", 15}, {"race:dwarf", 8}} {
			b := base(cfg.race, cfg.str)
			if c.c.equipment[k].Armor.Category == "shield" {
				b.Shield = true
			} else {
				b.Armor = k
			}
			d := Derive(b, c)
			r := arow{Key: k, Race: cfg.race, STR: cfg.str, AC: d.ArmorClass, Desc: d.ArmorClassDescription, Speed: d.SpeedWalkFt}
			for _, h := range d.Hints {
				if h.Source == k {
					r.Hints = append(r.Hints, h.TextPT)
				}
			}
			for _, i := range d.Issues {
				r.Issues = append(r.Issues, i.Message)
			}
			as = append(as, r)
		}
	}
	out["armor"] = as
	bt, _ := json.MarshalIndent(out, "", " ")
	if err := os.WriteFile(os.Getenv("COV_OUT"), bt, 0o644); err != nil {
		t.Fatal(err)
	}
}

func join(s []string) string {
	o := ""
	for i, x := range s {
		if i > 0 {
			o += ","
		}
		o += x[len("weapon-property:"):]
	}
	return o
}
