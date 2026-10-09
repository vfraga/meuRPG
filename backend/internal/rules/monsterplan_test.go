package rules

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/PuraFome/meuRPG/backend/internal/rules/srd51"
)

func planOf(t *testing.T, c *Content, creature string) *MonsterPlan {
	t.Helper()
	p, ok := c.MonsterPlan("monster:" + creature)
	if !ok {
		t.Fatalf("MonsterPlan(%s): not a creature", creature)
	}
	return p
}

func planActionOf(t *testing.T, c *Content, creature, action string) ActionPlan {
	t.Helper()
	p := planOf(t, c, creature)
	a, ok := p.Action("monster:" + creature + "#" + slugOf(action))
	if !ok {
		t.Fatalf("%s has no action %q", creature, action)
	}
	return a
}

func dmgText(parts []ActionDamage) string {
	var out []string
	for _, d := range parts {
		out = append(out, formulaText(d.Dice)+" "+strings.TrimPrefix(d.TypeKey, "damage-type:"))
	}
	return strings.Join(out, " + ")
}

// formulaText writes a formula as "2d6+3", "1d4" or a flat "1".
func formulaText(f DiceFormula) string {
	text := ""
	if f.Count > 0 {
		text = fmt.Sprintf("%dd%d", f.Count, f.Sides)
	}
	switch {
	case text == "":
		return fmt.Sprint(f.Bonus)
	case f.Bonus > 0:
		return fmt.Sprintf("%s+%d", text, f.Bonus)
	case f.Bonus < 0:
		return fmt.Sprintf("%s%d", text, f.Bonus)
	}
	return text
}

// TestMonsterPlanReadsTheActionsAsWritten: the actions the master runs most are read as the SRD writes
// them (SRD 5.1, "Monsters": actions): the attack roll and every damage part, the saving
// throw and the damage it governs, what a failed save gives, a grapple's escape DC and the
// limit of the action.
func TestMonsterPlanReadsTheActionsAsWritten(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)

	t.Run("Ghoul's Claws paralyse on a failed save, and are fully read", func(t *testing.T) {
		t.Parallel()
		a := planActionOf(t, c, "ghoul", "Claws")
		if a.Kind != ActionKindAttack || a.Class != ActionStructured || a.Attack == nil || a.Attack.Bonus != 4 || !a.Attack.Melee || a.Attack.ReachFt != 5 {
			t.Fatalf("Claws = %+v, want a structured melee attack at +4, reach 5", a)
		}
		if got := dmgText(a.Damage); got != "2d4+2 slashing" {
			t.Errorf("damage = %q, want 2d4+2 slashing", got)
		}
		s := a.Save
		if s == nil || !s.OnHit || s.Ability != CON || s.DC != 10 || s.ConditionKey != "condition:paralyzed" || s.OnSuccess != "none" || len(s.Damage) != 0 {
			t.Errorf("save = %+v, want the rider: DC 10 Constitution on a hit, paralyzed on a failure", s)
		}
	})
	t.Run("a poison bite has the poison a failed save deals, half on a success", func(t *testing.T) {
		t.Parallel()
		a := planActionOf(t, c, "giant-spider", "Bite")
		if got := dmgText(a.Damage); got != "1d8+3 piercing" {
			t.Errorf("hit damage = %q, want 1d8+3 piercing", got)
		}
		s := a.Save
		if s == nil || !s.OnHit || s.Ability != CON || s.DC != 11 || s.OnSuccess != "half" || dmgText(s.Damage) != "2d8 poison" {
			t.Errorf("save = %+v, want DC 11 Constitution, 2d8 poison, half on a success", s)
		}
	})
	t.Run("the assassin's poison is rolled once, with the save", func(t *testing.T) {
		t.Parallel()
		a := planActionOf(t, c, "assassin", "Shortsword")
		if got := dmgText(a.Damage); got != "1d6+3 piercing" {
			t.Errorf("hit damage = %q, want 1d6+3 piercing only", got)
		}
		if a.Save == nil || dmgText(a.Save.Damage) != "7d6 poison" || a.Save.OnSuccess != "half" {
			t.Errorf("save = %+v, want 7d6 poison, half on a success", a.Save)
		}
	})
	t.Run("a dragon's breath: save, damage, half and area, and it recharges", func(t *testing.T) {
		t.Parallel()
		a := planActionOf(t, c, "adult-red-dragon", "Fire Breath")
		s := a.Save
		if a.Kind != ActionKindSave || a.Class != ActionStructured || s == nil || s.OnHit || s.Ability != DEX || s.DC != 21 || dmgText(s.Damage) != "18d6 fire" {
			t.Fatalf("Fire Breath = %+v, want a structured save: DC 21 Dexterity, 18d6 fire", a)
		}
		// The snapshot says nothing on a success; the SRD text says half (corrections.json).
		if s.OnSuccess != "half" {
			t.Errorf("OnSuccess = %q, want half", s.OnSuccess)
		}
		if a.Area == nil || a.Area.Shape != "cone" || a.Area.SizeFt != 60 {
			t.Errorf("area = %+v, want a 60-foot cone", a.Area)
		}
		if a.Usage.Kind != UsageRecharge || a.Usage.RechargeMin != 5 {
			t.Errorf("usage = %+v, want Recharge 5-6", a.Usage)
		}
	})
	t.Run("a bite that grapples", func(t *testing.T) {
		t.Parallel()
		a := planActionOf(t, c, "giant-toad", "Bite")
		if got := dmgText(a.Damage); got != "1d10+2 piercing + 1d10 poison" {
			t.Errorf("damage = %q, want both parts", got)
		}
		h := a.HitCondition
		if h == nil || h.ConditionKey != "condition:grappled" || h.EscapeDC != 13 {
			t.Errorf("hit condition = %+v, want grappled, escape DC 13", h)
		}
	})
	t.Run("a grapple only of a smaller target keeps the size", func(t *testing.T) {
		t.Parallel()
		h := planActionOf(t, c, "constrictor-snake", "Constrict").HitCondition
		if h == nil || h.EscapeDC != 14 {
			t.Fatalf("hit condition = %+v, want escape DC 14", h)
		}
		h = planActionOf(t, c, "crocodile", "Bite").HitCondition
		if h == nil || h.EscapeDC != 12 {
			t.Errorf("crocodile hit condition = %+v, want escape DC 12", h)
		}
		h = planActionOf(t, c, "chuul", "Pincer").HitCondition
		if h == nil || h.MaxSize != "Large" || h.EscapeDC != 14 {
			t.Errorf("chuul hit condition = %+v, want escape DC 14 for a Large or smaller target", h)
		}
	})
	t.Run("a thrown weapon is an attack in melee and at range", func(t *testing.T) {
		t.Parallel()
		a := planActionOf(t, c, "goblin", "Shortbow")
		if a.Attack == nil || a.Attack.Melee || a.Attack.RangeFt != 80 || a.Attack.LongRangeFt != 320 {
			t.Errorf("Shortbow = %+v, want a ranged attack 80/320", a.Attack)
		}
		a = planActionOf(t, c, "bugbear", "Javelin")
		if a.Attack == nil || !a.Attack.Melee || a.Attack.ReachFt != 5 || a.Attack.RangeFt != 30 || a.Attack.LongRangeFt != 120 {
			t.Errorf("Javelin = %+v, want melee reach 5 and range 30/120", a.Attack)
		}
	})
	t.Run("a two-handed damage stays in the text, not in the plan", func(t *testing.T) {
		t.Parallel()
		a := planActionOf(t, c, "hobgoblin", "Longsword")
		if got := dmgText(a.Damage); got != "1d8+1 slashing" || a.Class != ActionStructured {
			t.Errorf("Longsword = %q %s, want 1d8+1 slashing, structured", got, a.Class)
		}
	})
	t.Run("an action with no roll is a reminder", func(t *testing.T) {
		t.Parallel()
		a := planActionOf(t, c, "ancient-brass-dragon", "Change Shape")
		if a.Kind != ActionKindOther || a.Class != ActionText || a.Attack != nil || a.Save != nil {
			t.Errorf("Change Shape = %+v, want text only", a)
		}
	})
	t.Run("Multiattack keeps its routines, with the action of each step", func(t *testing.T) {
		t.Parallel()
		a := planActionOf(t, c, "adult-red-dragon", "Multiattack")
		if a.Kind != ActionKindMultiattack || len(a.Routines) != 1 || len(a.Routines[0]) != 3 {
			t.Fatalf("Multiattack = %+v, want one routine of three steps", a.Routines)
		}
		step := a.Routines[0][2]
		if step.Name != "Claw" || step.Count != 2 || step.ActionKey != "monster:adult-red-dragon#claw" {
			t.Errorf("step 3 = %+v, want 2 claws", step)
		}
	})
	t.Run("a creature with several routines keeps each one", func(t *testing.T) {
		t.Parallel()
		a := planActionOf(t, c, "drider", "Multiattack")
		if len(a.Routines) != 4 || len(a.Routines[2]) != 2 || a.Routines[2][0].Name != "Longsword" || a.Routines[2][0].Count != 2 || a.Routines[2][1].Name != "Bite" {
			t.Errorf("drider routines = %+v, want 3 swords, 3 bows, 2 swords and a bite, 2 bows and a bite", a.Routines)
		}
	})
	t.Run("a step that casts has no action of its own", func(t *testing.T) {
		t.Parallel()
		a := planActionOf(t, c, "glabrezu", "Multiattack")
		step := a.Routines[1][1]
		if step.Name != "Innate Spellcasting" || step.ActionKey != "" || step.Kind != "magic" {
			t.Errorf("step = %+v, want Innate Spellcasting as a magic step with no action", step)
		}
	})
}

// TestMonsterPlanBreathWeaponsFollowTheText: every breath weapon and area of the
// SRD that deals damage halves it on a success, as the text says; the snapshot's
// four exceptions are corrected.
func TestMonsterPlanSavesFollowTheText(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	checked := 0
	for _, e := range mustList(t, c, CreatureFilter{}) {
		m := c.c.monsters[e.Key]
		p := planOf(t, c, strings.TrimPrefix(e.Key, "monster:"))
		for i, a := range p.Actions {
			if a.Kind != ActionKindSave || a.Save == nil || len(a.Save.Damage) == 0 {
				continue
			}
			text := m.Actions[i].Desc
			halves := strings.Contains(text, "half as much damage on a success") || strings.Contains(text, "takes half as much damage") ||
				strings.Contains(text, "takes half the") || strings.Contains(text, "only half the damage")
			if halves != (a.Save.OnSuccess == "half") {
				t.Errorf("%s %s: OnSuccess = %q but the text %s halves", e.Key, a.Name, a.Save.OnSuccess, map[bool]string{true: "says it", false: "does not say it"}[halves])
			}
			checked++
		}
	}
	if checked < 50 {
		t.Errorf("checked %d save actions with damage; want the 60 or so of the SRD", checked)
	}
}

// TestMonsterPlanEveryCreature: the invariants of the plan over the 334 creatures, and how
// many actions fall in each class (the numbers are the ones the docs give).
func TestMonsterPlanEveryCreature(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	classes := map[string]int{}
	var multi, total int
	for _, e := range mustList(t, c, CreatureFilter{}) {
		p := planOf(t, c, strings.TrimPrefix(e.Key, "monster:"))
		m := c.c.monsters[e.Key]
		if len(p.Actions) != len(m.Actions) {
			t.Errorf("%s: %d planned actions for %d", e.Key, len(p.Actions), len(m.Actions))
		}
		seen := map[string]bool{}
		for _, a := range p.Actions {
			total++
			classes[a.Class]++
			if a.Kind == ActionKindMultiattack {
				multi++
			}
			if seen[a.Key] {
				t.Errorf("%s: key %s is used twice", e.Key, a.Key)
			}
			seen[a.Key] = true
			if (a.Kind == ActionKindAttack) != (a.Attack != nil) {
				t.Errorf("%s %s: kind %s with attack %v", e.Key, a.Name, a.Kind, a.Attack)
			}
			if a.Kind == ActionKindOther && a.Class != ActionText {
				t.Errorf("%s %s: no roll but class %s", e.Key, a.Name, a.Class)
			}
			if a.Save != nil {
				if _, ok := abilityIndex[a.Save.Ability]; !ok || a.Save.DC < 1 || !slices.Contains([]string{"none", "half", "other"}, a.Save.OnSuccess) {
					t.Errorf("%s %s: bad save %+v", e.Key, a.Name, a.Save)
				}
				if a.Save.ConditionKey != "" {
					if _, ok := c.c.named[a.Save.ConditionKey]; !ok {
						t.Errorf("%s %s: %s is not a condition", e.Key, a.Name, a.Save.ConditionKey)
					}
				}
				if a.Save.OnSuccess == "half" && len(a.Save.Damage) == 0 && a.Kind == ActionKindSave {
					// "half" with nothing to halve is the data's way to say the effect has parts the text
					// states (Whelm, Tentacle Slam are read from it); it must be a text the plan could not read.
					if a.Class == ActionStructured {
						t.Errorf("%s %s: half of nothing is structured", e.Key, a.Name)
					}
				}
			}
			for _, steps := range a.Routines {
				for _, st := range steps {
					if st.Count < 1 {
						t.Errorf("%s %s: a step of %d", e.Key, a.Name, st.Count)
					}
				}
			}
		}
		if len(m.LegendaryActions) > 0 {
			if p.Legendary == nil || p.Legendary.PerRound != 3 || len(p.Legendary.Options) != len(m.LegendaryActions) {
				t.Errorf("%s: legendary plan = %+v, want 3 a round and %d options", e.Key, p.Legendary, len(m.LegendaryActions))
			}
			for _, o := range p.Legendary.Options {
				if o.Cost < 1 || o.Cost > p.Legendary.PerRound {
					t.Errorf("%s: option %s costs %d", e.Key, o.Name, o.Cost)
				}
				if o.ActionKey != "" {
					if _, ok := p.Action(o.ActionKey); !ok {
						t.Errorf("%s: option %s makes %s, which is not an action", e.Key, o.Name, o.ActionKey)
					}
				}
			}
		} else if p.Legendary != nil {
			t.Errorf("%s: legendary plan without legendary actions", e.Key)
		}
		hasLR := slices.ContainsFunc(m.SpecialAbilities, func(a srd51.MonsterAbility) bool { return strings.HasPrefix(a.Name, "Legendary Resistance") })
		if hasLR != (p.LegendaryResistance == 3) {
			t.Errorf("%s: Legendary Resistance = %d with the trait %v", e.Key, p.LegendaryResistance, hasLR)
		}
	}
	// The classes of the 884 actions of the 334 creatures (docs/architecture.md says the same).
	if total != 884 || classes[ActionStructured] != 722 || classes[ActionPartial] != 93 || classes[ActionText] != 69 || multi != 148 {
		t.Errorf("%d actions: structured %d, partial %d, text %d, Multiattack %d; want 884: 722, 93, 69, 148 (update the docs with the numbers)",
			total, classes[ActionStructured], classes[ActionPartial], classes[ActionText], multi)
	}
}

// TestMonsterSpellcastingIsRead: the Spellcasting and Innate Spellcasting traits of the SRD in
// the two layouts, with the slots, the uses and the DC the stat block states.
func TestMonsterSpellcastingIsRead(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)

	mage := planOf(t, c, "mage").Spellcasting
	if len(mage) != 1 {
		t.Fatalf("mage has %d spellcasting traits, want 1", len(mage))
	}
	sc := mage[0]
	if sc.Innate || sc.CasterLevel != 9 || sc.Ability != INT || sc.SaveDC != 14 || sc.AttackBonus != 6 || sc.ClassKey != "class:wizard" {
		t.Errorf("mage = %+v, want a 9th-level wizard: Intelligence, DC 14, +6", sc)
	}
	slots := map[int]int{}
	for _, l := range sc.Levels {
		slots[l.Level] = l.Slots
	}
	if want := map[int]int{1: 4, 2: 3, 3: 3, 4: 3, 5: 1}; !equalMaps(slots, want) {
		t.Errorf("mage slots = %v, want %v", slots, want)
	}
	cantrips := planSpellKeys(sc.AtWill)
	if want := []string{"spell:fire-bolt", "spell:light", "spell:mage-hand", "spell:prestidigitation"}; !slices.Equal(cantrips, want) {
		t.Errorf("mage cantrips = %v, want %v", cantrips, want)
	}

	lich := planOf(t, c, "lich").Spellcasting[0]
	if lich.CasterLevel != 18 || lich.SaveDC != 20 || lich.AttackBonus != 12 || len(lich.Levels) != 9 || lich.Levels[8].Level != 9 || lich.Levels[8].Slots != 1 || lich.Levels[8].Spells[0].Key != "spell:power-word-kill" {
		t.Errorf("lich = %+v, want an 18th-level caster with a 9th-level slot for power word kill", lich)
	}

	djinni := planOf(t, c, "djinni").Spellcasting[0]
	if !djinni.Innate || djinni.SaveDC != 17 || djinni.AttackBonus != 9 || len(djinni.AtWill) != 3 || len(djinni.PerDay) != 2 || djinni.PerDay[0].Uses != 3 || !djinni.PerDay[0].Each || djinni.PerDay[1].Uses != 1 {
		t.Errorf("djinni = %+v, want innate: DC 17, +9, 3 at will, 3/day each and 1/day each", djinni)
	}
	efreeti := planOf(t, c, "efreeti").Spellcasting[0]
	if len(efreeti.PerDay) != 2 || efreeti.PerDay[0].Uses != 3 || efreeti.PerDay[0].Each {
		t.Errorf("efreeti = %+v, want 3/day shared by enlarge/reduce and tongues", efreeti.PerDay)
	}
	lamia := planOf(t, c, "lamia").Spellcasting[0]
	if len(lamia.AtWill) != 2 || lamia.AtWill[0].Note != "any humanoid form" || len(lamia.PerDay) != 2 || lamia.PerDay[1].Spells[0].Key != "spell:geas" {
		t.Errorf("lamia = %+v, want the one-line layout read", lamia)
	}
	mephit := planOf(t, c, "dust-mephit").Spellcasting[0]
	if len(mephit.PerDay) != 1 || mephit.PerDay[0].Uses != 1 || mephit.PerDay[0].Spells[0].Key != "spell:sleep" || mephit.SaveDC != 10 {
		t.Errorf("dust mephit = %+v, want sleep 1/day, DC 10", mephit)
	}
	archmage := planOf(t, c, "archmage").Spellcasting[0]
	if i := slices.IndexFunc(archmage.Levels[0].Spells, func(r SpellRef) bool { return r.Key == "spell:mage-armor" }); i < 0 || archmage.Levels[0].Spells[i].Note == "" {
		t.Errorf("archmage mage armor = %+v, want the note that it is cast before the fight", archmage.Levels[0])
	}
	if !slices.ContainsFunc(archmage.AtWill, func(r SpellRef) bool { return r.Key == "spell:invisibility" }) {
		t.Errorf("archmage at will = %v, want invisibility, from the sentence before the list", planSpellKeys(archmage.AtWill))
	}

	// Every spellcasting trait of the SRD is read, and every spell is an SRD spell.
	count := 0
	for _, e := range mustList(t, c, CreatureFilter{}) {
		for _, sc := range planOf(t, c, strings.TrimPrefix(e.Key, "monster:")).Spellcasting {
			count++
			if sc.SaveDC < 8 || sc.Ability == "" {
				t.Errorf("%s: spellcasting %+v has no DC or ability", e.Key, sc)
			}
			if !sc.Innate && (sc.CasterLevel < 1 || len(sc.Levels) == 0) {
				t.Errorf("%s: a slot caster without level or slots: %+v", e.Key, sc)
			}
		}
	}
	if count != 36 {
		t.Errorf("%d spellcasting traits read, want the 36 of the SRD", count)
	}
}

func planSpellKeys(refs []SpellRef) []string {
	var out []string
	for _, r := range refs {
		out = append(out, r.Key)
	}
	return out
}

func equalMaps(a, b map[int]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// TestMonsterLegendaryActionsAreRead: the options with their cost and the action they make
// (SRD 5.1, "Legendary Creatures").
func TestMonsterLegendaryActionsAreRead(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	p := planOf(t, c, "adult-red-dragon")
	if p.LegendaryResistance != 3 {
		t.Errorf("Legendary Resistance = %d, want 3 a day", p.LegendaryResistance)
	}
	byName := map[string]LegendaryOption{}
	for _, o := range p.Legendary.Options {
		byName[o.Name] = o
	}
	if o := byName["Tail Attack"]; o.Cost != 1 || o.ActionKey != "monster:adult-red-dragon#tail" {
		t.Errorf("Tail Attack = %+v, want cost 1 making the Tail action", o)
	}
	wing := byName["Wing Attack"]
	if wing.Cost != 2 || wing.Action == nil || wing.Action.Save == nil || wing.Action.Save.DC != 22 || wing.Action.Save.Ability != DEX ||
		dmgText(wing.Action.Save.Damage) != "2d6+8 bludgeoning" || wing.Action.Save.ConditionKey != "condition:prone" || wing.Action.Save.OnSuccess != "none" {
		t.Errorf("Wing Attack = %+v, want cost 2 with its own save (DC 22 Dexterity, 2d6+8, prone)", wing)
	}
	if byName["Detect"].Cost != 1 || byName["Detect"].Action != nil || byName["Detect"].ActionKey != "" {
		t.Errorf("Detect = %+v, want a reminder of cost 1", byName["Detect"])
	}
	lich := planOf(t, c, "lich").Legendary
	var disrupt LegendaryOption
	for _, o := range lich.Options {
		if o.Name == "Disrupt Life" {
			disrupt = o
		}
	}
	if disrupt.Cost != 3 || disrupt.Action == nil || disrupt.Action.Save.OnSuccess != "half" || dmgText(disrupt.Action.Save.Damage) != "6d6 necrotic" {
		t.Errorf("Disrupt Life = %+v, want cost 3: 6d6 necrotic, half", disrupt)
	}
}

// TestActionCorrectionsAreClosed: the loader refuses a correction of an action, of the legendary
// actions or of an innate spell that does not fit, and writes a good one.
func TestActionCorrectionsAreClosed(t *testing.T) {
	t.Parallel()
	newContent := func() *content {
		return &content{
			classLevels: map[string][]*srd51.Level{},
			monsters: map[string]*srd51.Monster{
				"monster:drake": {
					Actions:          []srd51.MonsterAction{{Name: "Breath", Save: &srd51.MonsterSave{Ability: "dex", DC: 12, OnSuccess: "none"}}, {Name: "Bite"}},
					LegendaryActions: []srd51.MonsterAbility{{Name: "Detect"}},
					SpecialAbilities: []srd51.MonsterAbility{{Name: "Innate Spellcasting"}},
				},
				"monster:rat": {Actions: []srd51.MonsterAction{{Name: "Bite"}}},
			},
		}
	}
	one := func(creature, action, field, value string) string {
		a := ""
		if action != "" {
			a = `"action":"` + action + `",`
		}
		return `{"creature_corrections":[{"creature":"` + creature + `",` + a + `"field":"` + field + `","value":` + value + `,"source":"SRD"}]}`
	}
	for name, doc := range map[string]string{
		"an action the stat block lacks":       one("monster:drake", "Roar", "action_save_success", `"half"`),
		"an action without a save":             one("monster:drake", "Bite", "action_save_success", `"half"`),
		"a save outcome that is not one":       one("monster:drake", "Breath", "action_save_success", `"all"`),
		"a save outcome without an action":     one("monster:drake", "", "action_save_success", `"half"`),
		"an action for another field":          one("monster:drake", "Breath", "legendary_actions", "3"),
		"legendary actions of a lone creature": one("monster:rat", "", "legendary_actions", "3"),
		"no legendary actions a round":         one("monster:drake", "", "legendary_actions", "0"),
		"too many legendary actions":           one("monster:drake", "", "legendary_actions", "6"),
		"innate uses of a creature without":    one("monster:rat", "", "innate_spell_uses", "1"),
		"no innate uses":                       one("monster:drake", "", "innate_spell_uses", "0"),
		"the same save corrected twice":        `{"creature_corrections":[{"creature":"monster:drake","action":"Breath","field":"action_save_success","value":"half","source":"SRD"},{"creature":"monster:drake","action":"Breath","field":"action_save_success","value":"none","source":"SRD"}]}`,
	} {
		if err := newContent().applyCorrections(fstest.MapFS{"effects/corrections.json": {Data: []byte(doc)}}); err == nil {
			t.Errorf("%s: the loader accepted it", name)
		}
	}
	c := newContent()
	doc := `{"creature_corrections":[` +
		`{"creature":"monster:drake","action":"Breath","field":"action_save_success","value":"half","source":"SRD"},` +
		`{"creature":"monster:drake","field":"legendary_actions","value":3,"source":"SRD"},` +
		`{"creature":"monster:drake","field":"innate_spell_uses","value":2,"source":"SRD"}]}`
	if err := c.applyCorrections(fstest.MapFS{"effects/corrections.json": {Data: []byte(doc)}}); err != nil {
		t.Fatal(err)
	}
	if got := c.monsters["monster:drake"].Actions[0].Save.OnSuccess; got != "half" || c.legendaryPerRound["monster:drake"] != 3 || c.innateUses["monster:drake"] != 2 {
		t.Errorf("good corrections: on success %q, legendary %v, innate %v; want half, 3, 2", got, c.legendaryPerRound, c.innateUses)
	}
}
