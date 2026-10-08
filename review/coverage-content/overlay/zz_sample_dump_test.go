package rules

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// Coverage inventory (not part of the repo): what the content side of a cast gives for a sample of spells,
// with the same calls characters.CombatSpell makes.
func TestCoverageSampleSpells(t *testing.T) {
	c, err := LoadSRD()
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	sample := []struct {
		key  string
		slot int
	}{{"spell:fireball", 3}, {"spell:fireball", 5}, {"spell:cure-wounds", 1}, {"spell:cure-wounds", 3}, {"spell:hold-person", 2}, {"spell:web", 2},
		{"spell:sleep", 1}, {"spell:sleep", 3}, {"spell:guiding-bolt", 1}, {"spell:guiding-bolt", 3}, {"spell:eldritch-blast", 0}, {"spell:fire-bolt", 0},
		{"spell:hellish-rebuke", 1}, {"spell:spirit-guardians", 3}, {"spell:bless", 1}, {"spell:haste", 3}, {"spell:flaming-sphere", 2},
		{"spell:vampiric-touch", 3}, {"spell:chromatic-orb", 1}, {"spell:mass-heal", 9}, {"spell:acid-arrow", 2}, {"spell:thunderwave", 1}, {"spell:sacred-flame", 0}}
	for _, s := range sample {
		d, ok := c.SpellDetails(s.key)
		if !ok {
			fmt.Fprintf(&b, "%-26s (not in the SRD content)\n", s.key)
			continue
		}
		fx, hasFx := c.SpellEffect(s.key, s.slot)
		_, serr := c.SummonOptions(s.key, d.Spell.Level, Build{})
		var dmg []string
		for _, r := range d.DamageAtChoosing(s.slot, 5, "") {
			dmg = append(dmg, fmt.Sprintf("%s %s parsed=%v", r.Raw, r.Type, r.Parsed))
		}
		heal, hasHeal := d.HealAt(s.slot)
		save := "-"
		if d.Save != nil {
			save = string(d.Save.Ability) + "/" + d.Save.OnSuccess
		}
		fmt.Fprintf(&b, "%-26s slot=%d attack=%q save=%s damage=%v heal=%v(%s) hpEffect=%v(%s) summon=%v target=%s/%d conc=%v time=%s\n", s.key, s.slot,
			d.AttackType, save, dmg, hasHeal, heal.Raw, hasFx, fx.Kind, serr == nil, d.Target.Kind, d.Target.MaxTargets(s.slot, d.Spell.Level), d.Duration.Concentration, d.CastingTime.Raw)
	}
	if err := os.WriteFile(os.Getenv("COV_OUT"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}
