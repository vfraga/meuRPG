package rules

import (
	"encoding/json"
	"errors"
	"os"
	"sort"
	"testing"
)

// Coverage inventory (not part of the repo): what the cast pipeline would read
// for every SRD spell, mirroring characters.CombatSpell.
func TestCoverageDumpSpells(t *testing.T) {
	c, err := LoadSRD()
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for k := range c.c.spellDetails {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	type row struct {
		Key           string   `json:"key"`
		Level         int      `json:"level"`
		Economy       string   `json:"economy"`
		CastUnit      string   `json:"cast_unit"`
		RangeKind     string   `json:"range_kind"`
		Concentration bool     `json:"concentration"`
		Ritual        bool     `json:"ritual"`
		DurationKind  string   `json:"duration_kind"`
		AttackType    string   `json:"attack_type"`
		SaveAbility   string   `json:"save_ability"`
		SaveSuccess   string   `json:"save_success"`
		DamageTypes   []string `json:"damage_types"`
		DamageChoice  string   `json:"damage_choice"`
		DamageParsed  bool     `json:"damage_parsed"`
		Upcast        bool     `json:"damage_upcasts"`
		HealDice      string   `json:"heal"`
		HealUpcasts   bool     `json:"heal_upcasts"`
		HP            string   `json:"hp_effect"`
		Summon        bool     `json:"summon"`
		IgnoresCover  bool     `json:"ignores_cover"`
		TargetKind    string   `json:"target_kind"`
		AnyNumber     bool     `json:"area_any_number"`
		CasterOnly    bool     `json:"caster_only"`
		CastableInFight bool   `json:"castable_in_fight"`
		Darts         bool     `json:"darts"`
	}
	var out []row
	for _, k := range keys {
		d := c.c.spellDetails[k]
		r := row{Key: k, Level: d.Spell.Level, CastUnit: d.CastingTime.Unit, RangeKind: d.Range.Kind,
			Concentration: d.Duration.Concentration, Ritual: d.Spell.Ritual, DurationKind: d.Duration.Kind,
			AttackType: d.AttackType, DamageChoice: d.DamageChoice}
		switch d.CastingTime.Unit {
		case CastAction:
			r.Economy = EconomyAction
		case CastBonusAction:
			r.Economy = EconomyBonusAction
		case CastReaction:
			r.Economy = EconomyReaction
		}
		r.CastableInFight = r.Economy == EconomyAction || r.Economy == EconomyBonusAction
		if d.Save != nil {
			r.SaveAbility, r.SaveSuccess = string(d.Save.Ability), d.Save.OnSuccess
		}
		lvl := d.Spell.Level
		rolls := d.DamageAtChoosing(lvl, 5, "")
		allParsed := len(rolls) > 0
		for _, ro := range rolls {
			if !(ro.Parsed && ro.Type != "") {
				allParsed = false
			} else {
				r.DamageTypes = append(r.DamageTypes, ro.Type)
			}
		}
		r.DamageParsed = allParsed
		if len(d.Damage) > 0 {
			r.Upcast = len(d.Damage[0].BySlotLevel) > 1
		}
		if h, ok := d.HealAt(lvl); ok && h.Parsed {
			r.HealDice = h.Raw
			r.HealUpcasts = len(d.HealBySlotLevel) > 1
		}
		if fx, ok := c.SpellEffect(k, lvl); ok {
			r.HP = string(fx.Kind)
			r.DamageParsed, r.DamageTypes, r.HealDice = false, nil, ""
		}
		_, serr := c.SummonOptions(k, lvl, Build{})
		r.Summon = !errors.Is(serr, ErrNotSummonSpell)
		r.IgnoresCover = c.IgnoresCover(k)
		r.TargetKind = d.Target.Kind
		r.AnyNumber = d.Target.AnyNumber()
		r.CasterOnly = d.Target.CasterOnly()
		r.Darts = k == "spell:magic-missile"
		out = append(out, r)
	}
	b, _ := json.MarshalIndent(out, "", " ")
	if err := os.WriteFile(os.Getenv("COV_OUT"), b, 0o644); err != nil {
		t.Fatal(err)
	}
}
