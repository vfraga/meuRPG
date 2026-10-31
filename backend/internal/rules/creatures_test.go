package rules

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/PuraFome/meuRPG/backend/internal/rules/srd51"
)

func monsterDerived(t *testing.T, c *Content, key string) Derived {
	t.Helper()
	d, ok := c.MonsterDerived(key)
	if !ok {
		t.Fatalf("MonsterDerived(%s): not a creature", key)
	}
	return d
}

// TestMonsterDerived: a stat block becomes the Derived that combat reads. The
// numbers are the ones of the designs (the wolves and the raven).
func TestMonsterDerived(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)

	tests := []struct {
		key                          string
		ac, hp, walk, fly            int
		size                         string
		attack                       string
		bonus                        int
		damage, damageType           string
		darkvision                   int
		abilityOfAttack              Ability
		reach                        int
		passive, strMod, dexMod, ini int
	}{
		{"monster:wolf", 13, 11, 40, 0, "Medium", "Bite", 4, "2d4+2", "damage-type:piercing", 0, DEX, 5, 13, 1, 2, 2},
		{"monster:dire-wolf", 14, 37, 50, 0, "Large", "Bite", 5, "2d6+3", "damage-type:piercing", 0, STR, 5, 13, 3, 2, 2},
		{"monster:raven", 12, 1, 10, 50, "Tiny", "Beak", 4, "1", "damage-type:piercing", 0, DEX, 5, 13, -4, 2, 2},
		{"monster:goblin", 15, 7, 30, 0, "Small", "Scimitar", 4, "1d6+2", "damage-type:slashing", 60, DEX, 5, 9, -1, 2, 2},
		{"monster:skeleton", 13, 13, 30, 0, "Medium", "Shortsword", 4, "1d6+2", "damage-type:piercing", 60, DEX, 5, 9, 0, 2, 2},
	}
	for _, tt := range tests {
		d := monsterDerived(t, c, tt.key)
		if d.ArmorClass != tt.ac || d.HitPointsMax != tt.hp || d.SpeedWalkFt != tt.walk || d.SpeedFlyFt != tt.fly {
			t.Errorf("%s: AC %d, HP %d, walk %d, fly %d; want %d, %d, %d, %d", tt.key, d.ArmorClass, d.HitPointsMax, d.SpeedWalkFt, d.SpeedFlyFt, tt.ac, tt.hp, tt.walk, tt.fly)
		}
		if e, _ := c.CreatureByKey(tt.key); e.Size != tt.size {
			t.Errorf("%s: size %q, want %q", tt.key, e.Size, tt.size)
		}
		if len(d.Attacks) == 0 {
			t.Fatalf("%s has no attacks", tt.key)
		}
		a := d.Attacks[0]
		if a.Name != tt.attack || a.AttackBonus != tt.bonus || a.Damage != tt.damage || a.DamageType != tt.damageType || a.Ability != tt.abilityOfAttack || a.RangeFt != tt.reach || !a.Melee {
			t.Errorf("%s: attack %+v, want %s +%d %s %s (%s)", tt.key, a, tt.attack, tt.bonus, tt.damage, tt.damageType, tt.abilityOfAttack)
		}
		if a.DamageDice.Bonus == 0 && a.DamageDice.Count == 0 {
			t.Errorf("%s: the damage dice were not read from %q", tt.key, a.Damage)
		}
		if a.Notes == "" {
			t.Errorf("%s: the rest of the action's text was lost", tt.key)
		}
		dv := 0
		for _, s := range d.Senses {
			if s.Key == "darkvision" {
				dv = s.RangeFt
			}
		}
		if dv != tt.darkvision {
			t.Errorf("%s: darkvision %d, want %d", tt.key, dv, tt.darkvision)
		}
		if d.PassivePerception != tt.passive || abilityOf(d, STR).Modifier != tt.strMod || abilityOf(d, DEX).Modifier != tt.dexMod || d.Initiative != tt.ini {
			t.Errorf("%s: passive %d, STR %+d, DEX %+d, initiative %+d", tt.key, d.PassivePerception, abilityOf(d, STR).Modifier, abilityOf(d, DEX).Modifier, d.Initiative)
		}
		if d.AttacksPerAction != 1 || len(d.SpellSlots) != 9 || len(d.Spellcasting) != 0 {
			t.Errorf("%s: attacks per action %d, slots %v, spellcasting %v", tt.key, d.AttacksPerAction, d.SpellSlots, d.Spellcasting)
		}
	}

	t.Run("the wolf's bite and its Strength save", func(t *testing.T) {
		t.Parallel()
		d := monsterDerived(t, c, "monster:wolf")
		if len(d.SaveActions) != 1 || d.SaveActions[0].Ability != STR || d.SaveActions[0].DC != 11 || d.SaveActions[0].Key != "monster:wolf#bite" {
			t.Errorf("SaveActions = %+v, want the bite's Strength save, DC 11", d.SaveActions)
		}
		if got := skillOf(d, "skill:stealth"); got.Bonus != 4 || got.Proficiency != ProficiencyFull {
			t.Errorf("stealth = %+v, want the stat block's +4", got)
		}
		if got := skillOf(d, "skill:athletics"); got.Bonus != 1 || got.Proficiency != ProficiencyNone {
			t.Errorf("athletics = %+v, want the Strength modifier", got)
		}
		if got := saveOf(d, DEX); got.Bonus != 2 || got.Proficient {
			t.Errorf("Dex save = %+v, want the modifier", got)
		}
	})
	t.Run("a stat block's saves and a Multiattack", func(t *testing.T) {
		t.Parallel()
		dragon := monsterDerived(t, c, "monster:adult-black-dragon")
		if got := saveOf(dragon, CON); got.Bonus != 10 || !got.Proficient {
			t.Errorf("dragon Con save = %+v, want the stat block's +10", got)
		}
		if dragon.AttacksPerAction != 3 || dragon.SpeedFlyFt != 80 || dragon.SpeedSwimFt != 40 {
			t.Errorf("dragon: %d attacks per action, fly %d, swim %d; want 3, 80, 40", dragon.AttacksPerAction, dragon.SpeedFlyFt, dragon.SpeedSwimFt)
		}
		var breath *SaveAction
		for i, s := range dragon.SaveActions {
			if s.Name == "Acid Breath" {
				breath = &dragon.SaveActions[i]
			}
		}
		if breath == nil || breath.Ability != DEX || breath.OnSuccess != "half" || breath.Usage != "Recharge 5-6" {
			t.Errorf("acid breath = %+v, want a Dexterity save for half, Recharge 5-6", breath)
		}
		captain := monsterDerived(t, c, "monster:bandit-captain")
		if captain.AttacksPerAction != 3 {
			t.Errorf("the bandit captain makes %d attacks, want 3 (the longest routine)", captain.AttacksPerAction)
		}
		// A choice of damage keeps the first option; the text has the rest.
		gnoll := monsterDerived(t, c, "monster:gnoll")
		for _, a := range gnoll.Attacks {
			if a.Name == "Spear" && a.Damage != "1d6+2" {
				t.Errorf("gnoll spear damage = %q, want the one-handed 1d6+2", a.Damage)
			}
		}
	})
	t.Run("a ranged attack and the traits", func(t *testing.T) {
		t.Parallel()
		d := monsterDerived(t, c, "monster:goblin")
		bow := d.Attacks[1]
		if bow.Name != "Shortbow" || bow.Melee || bow.RangeFt != 80 || bow.LongRangeFt != 320 {
			t.Errorf("shortbow = %+v, want ranged 80/320", bow)
		}
		if len(d.Features) != 1 || d.Features[0].Name != "Nimble Escape" {
			t.Errorf("features = %+v, want Nimble Escape", d.Features)
		}
	})
	if _, ok := c.MonsterDerived("monster:unicorn-of-doom"); ok {
		t.Error("an unknown creature has no stat block")
	}
}

// TestCreatureArmorClass: the armor class worn, with what it wears and its
// other values in the note.
func TestCreatureArmorClass(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	for _, tt := range []struct {
		key  string
		ac   int
		note []string
	}{
		{"monster:azer", 17, []string{"escudo"}},
		{"monster:lizardfolk", 15, []string{"escudo"}},
		{"monster:mage", 12, []string{"15 com ", "Armadura"}},
		{"monster:goblin", 15, []string{"armadura de couro", "escudo"}},
		{"monster:ankheg", 14, []string{"11 se "}},
		{"monster:wolf", 13, nil},
	} {
		cr, _ := c.CreatureByKey(tt.key)
		d := monsterDerived(t, c, tt.key)
		if cr.ArmorClass != tt.ac || d.ArmorClass != tt.ac {
			t.Errorf("%s: AC %d / %d, want %d", tt.key, cr.ArmorClass, d.ArmorClass, tt.ac)
		}
		for _, w := range tt.note {
			if !strings.Contains(cr.ArmorClassNote, w) {
				t.Errorf("%s: note %q lacks %q", tt.key, cr.ArmorClassNote, w)
			}
		}
		if tt.note == nil && cr.ArmorClassNote != "" {
			t.Errorf("%s: note %q, want none", tt.key, cr.ArmorClassNote)
		}
	}
}

// TestCreatureAttacksAndCounts: a thrown weapon stays melee and carries its
// range; counts that are words keep them and give the engine a number.
func TestCreatureAttacksAndCounts(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	bugbear := monsterDerived(t, c, "monster:bugbear")
	var jav *Attack
	for i, a := range bugbear.Attacks {
		if a.Name == "Javelin" {
			jav = &bugbear.Attacks[i]
		}
	}
	if jav == nil || !jav.Melee || jav.RangeFt != 30 || jav.LongRangeFt != 120 {
		t.Errorf("bugbear javelin = %+v, want melee with range 30/120", jav)
	}
	if hydra := monsterDerived(t, c, "monster:hydra"); hydra.AttacksPerAction != 5 {
		t.Errorf("hydra: %d attacks per action, want its 5 bites", hydra.AttacksPerAction)
	}
	h, _ := c.CreatureByKey("monster:hydra")
	x := h.Actions[0].Multiattack[0][0]
	if x.Count != 5 || x.Text != "Number of Heads" {
		t.Errorf("hydra multiattack = %+v", x)
	}
	v, _ := c.CreatureByKey("monster:violet-fungus")
	if x := v.Actions[0].Multiattack[0][0]; x.Count != 1 || x.Text != "1d4" {
		t.Errorf("violet fungus multiattack = %+v", x)
	}
	// Every Multiattack routine points at a real action (or a spell), with a count.
	for _, e := range mustList(t, c, CreatureFilter{}) {
		cr, _ := c.CreatureByKey(e.Key)
		names := map[string]bool{}
		for _, a := range cr.Actions {
			names[a.Name] = true
		}
		for _, a := range cr.Actions {
			for _, r := range a.Multiattack {
				for _, x := range r {
					if x.Count < 1 || (!names[x.Name] && x.Kind != "ability" && x.Kind != "magic") {
						t.Errorf("%s: multiattack %+v", e.Key, x)
					}
				}
			}
		}
	}
	// The structured breaths and roars keep their DCs.
	brass := monsterDerived(t, c, "monster:adult-brass-dragon")
	found := map[string]int{}
	for _, s := range brass.SaveActions {
		found[s.Name] = s.DC
	}
	if found["Fire Breath"] != 18 || found["Sleep Breath"] != 18 {
		t.Errorf("brass dragon save actions = %v", found)
	}
	bul := monsterDerived(t, c, "monster:bulette")
	ok := false
	for _, s := range bul.SaveActions {
		ok = ok || (s.Name == "Deadly Leap" && s.DC == 16 && s.OnSuccess == "half")
	}
	if !ok {
		t.Errorf("bulette = %+v", bul.SaveActions)
	}
}

// TestCreatureByKey: the stat block the screen reads, with Portuguese labels
// and the SRD's English text.
func TestCreatureByKey(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	w, ok := c.CreatureByKey("monster:dire-wolf")
	if !ok || w.NamePT != "Lobo atroz" || w.SizeNamePT != "Grande" || w.TypeNamePT != "fera" || w.ChallengeRating != "1" || w.XP != 200 {
		t.Fatalf("dire wolf = %+v", w.CreatureEntry)
	}
	if w.ArmorClass != 14 || w.ArmorClassNamePT != "Armadura natural" || w.HitPoints != 37 || w.HitDice != "5d10" || w.SpeedWalkFt != 50 {
		t.Errorf("dire wolf numbers = %+v", w)
	}
	if len(w.Abilities) != 6 || w.Abilities[0].Score != 17 || w.Abilities[0].Modifier != 3 || w.Abilities[0].NamePT != "Força" {
		t.Errorf("abilities = %+v", w.Abilities)
	}
	if len(w.Skills) != 2 || w.Skills[0].NamePT != "Furtividade" && w.Skills[0].NamePT != "Percepção" {
		t.Errorf("skills = %+v", w.Skills)
	}
	if len(w.Actions) != 1 || w.Actions[0].Save == nil || w.Actions[0].Save.DC != 13 || w.Actions[0].Save.Ability != STR ||
		w.Actions[0].Damage[0].TypeNamePT == "" || len(w.Traits) != 2 {
		t.Errorf("actions = %+v, traits = %+v", w.Actions, w.Traits)
	}
	// The attacks have a Portuguese name where the content has one, in the stat block and in the derived attack.
	if w.Actions[0].NamePT != "Mordida" || w.Actions[0].Name != "Bite" {
		t.Errorf("dire wolf bite = %q / %q, want Bite / Mordida", w.Actions[0].Name, w.Actions[0].NamePT)
	}
	if got := monsterDerived(t, c, "monster:dire-wolf").Attacks[0].NamePT; got != "Mordida" {
		t.Errorf("dire wolf derived attack = %q, want Mordida", got)
	}
	sk, _ := c.CreatureByKey("monster:skeleton")
	if len(sk.Vulnerabilities) != 1 || sk.Vulnerabilities[0].Types[0].Key != "damage-type:bludgeoning" || len(sk.ConditionImmunities) != 2 ||
		len(sk.Senses) != 1 || sk.Senses[0].RangeFt != 60 || !strings.Contains(sk.ArmorClassNote, "armor scraps") {
		t.Errorf("skeleton = %+v", sk)
	}
	g, _ := c.CreatureByKey("monster:ghost")
	if !g.Hover || g.SpeedFlyFt != 40 {
		t.Errorf("a ghost hovers: fly %d, hover %v", g.SpeedFlyFt, g.Hover)
	}
}

// TestListCreatures: the filters the screens use.
func TestListCreatures(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	all, err := c.ListCreatures(CreatureFilter{})
	if err != nil || len(all) != 334 {
		t.Fatalf("all = %d, %v; want the SRD's 334", len(all), err)
	}
	if !slices.IsSortedFunc(all, func(a, b CreatureEntry) int { return comparePT(a.NamePT, b.NamePT) }) {
		t.Error("the list is not sorted by Portuguese name")
	}
	keys := func(f CreatureFilter) []string {
		t.Helper()
		list, err := c.ListCreatures(f)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, e := range list {
			out = append(out, e.Key)
		}
		return out
	}
	if got := keys(CreatureFilter{Query: "LOBO ATR"}); !slices.Equal(got, []string{"monster:dire-wolf"}) {
		t.Errorf("a name search (Portuguese, any case) = %v", got)
	}
	if got := keys(CreatureFilter{Query: "dire wolf"}); !slices.Equal(got, []string{"monster:dire-wolf"}) {
		t.Errorf("a name search (English) = %v", got)
	}
	if got := keys(CreatureFilter{Query: "serpente venenosa", Type: "beast"}); len(got) != 2 {
		t.Errorf("the poisonous snake and the giant one = %v", got)
	}
	for _, e := range mustList(t, c, CreatureFilter{Type: "beast", MaxCR: "1/4", NoFly: true, NoSwim: true}) {
		if e.Type != "beast" || e.CanFly || e.CanSwim {
			t.Errorf("%s does not pass the filter", e.Key)
		}
		if v, _ := crEighths(e.ChallengeRating); v > 2 {
			t.Errorf("%s has CR %s, above 1/4", e.Key, e.ChallengeRating)
		}
	}
	if got := keys(CreatureFilter{Type: "beast", MaxCR: "0", NoFly: true}); !slices.Contains(got, "monster:cat") || slices.Contains(got, "monster:bat") {
		t.Errorf("CR 0 beasts without flight = %v", got)
	}
	if _, err := c.ListCreatures(CreatureFilter{MaxCR: "1/3"}); err == nil {
		t.Error("1/3 is not a challenge rating")
	}
}

// TestListCreaturesBestiary: the filters the bestiary (MR-042) adds to the basic
// ones, and what a row carries.
func TestListCreaturesBestiary(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	keys := func(f CreatureFilter) []string {
		var out []string
		for _, e := range mustList(t, c, f) {
			out = append(out, e.Key)
		}
		return out
	}

	// A row: the English name, the armor class and the average hit points.
	for _, e := range mustList(t, c, CreatureFilter{Query: "dire wolf"}) {
		if e.Name != "Dire Wolf" || e.NamePT != "Lobo atroz" || e.ArmorClass != 14 || e.HitPoints != 37 {
			t.Errorf("dire wolf row = %+v", e)
		}
	}
	if got := mustList(t, c, CreatureFilter{Query: "wolf"}); len(got) == 0 || got[0].ArmorClass == 0 || got[0].HitPoints == 0 {
		t.Errorf("rows without numbers: %+v", got)
	}

	// "lobo" finds the Portuguese names, and "Wolf spider" through "wolf".
	lobo := keys(CreatureFilter{Query: "lobo"})
	for _, k := range []string{"monster:wolf", "monster:giant-wolf-spider", "monster:dire-wolf", "monster:winter-wolf"} {
		if !slices.Contains(lobo, k) {
			t.Errorf("\"lobo\" does not find %s: %v", k, lobo)
		}
	}
	if wolf := keys(CreatureFilter{Query: "wolf"}); !slices.Contains(wolf, "monster:giant-wolf-spider") || len(wolf) <= len(lobo)-1 {
		t.Errorf("\"wolf\" = %v", wolf)
	}

	// The sizes.
	for _, size := range []string{"Tiny", "Small", "Medium", "Large", "Huge", "Gargantuan"} {
		list := mustList(t, c, CreatureFilter{Size: size})
		if len(list) == 0 {
			t.Errorf("no %s creature", size)
		}
		for _, e := range list {
			if e.Size != size {
				t.Errorf("%s is %s, not %s", e.Key, e.Size, size)
			}
		}
	}
	if got := keys(CreatureFilter{Size: "Large", Query: "lobo"}); !slices.Contains(got, "monster:dire-wolf") || slices.Contains(got, "monster:wolf") {
		t.Errorf("Large \"lobo\" = %v", got)
	}

	// The range, both ends included.
	between := mustList(t, c, CreatureFilter{MinCR: "1/4", MaxCR: "1"})
	seen := map[string]bool{}
	for _, e := range between {
		seen[e.ChallengeRating] = true
		if v, _ := crEighths(e.ChallengeRating); v < 2 || v > 8 {
			t.Errorf("%s has CR %s, outside 1/4 to 1", e.Key, e.ChallengeRating)
		}
	}
	if !seen["1/4"] || !seen["1"] || !seen["1/2"] {
		t.Errorf("the range leaves out an end: %v", seen)
	}
	exact := mustList(t, c, CreatureFilter{MinCR: "2", MaxCR: "2"})
	if len(exact) == 0 {
		t.Error("min = max is the one rating")
	}
	for _, e := range exact {
		if e.ChallengeRating != "2" {
			t.Errorf("%s has CR %s", e.Key, e.ChallengeRating)
		}
	}
	if low := mustList(t, c, CreatureFilter{MinCR: "20"}); len(low) == 0 || len(low) > 40 {
		t.Errorf("CR 20 and up = %d creatures", len(low))
	}
	// Only a MinCR is a floor, only a MaxCR a ceiling, and the zero value both open.
	if n := len(mustList(t, c, CreatureFilter{MinCR: "0"})); n != 334 {
		t.Errorf("MinCR 0 = %d, want all 334", n)
	}

	// A filter that cannot be right.
	if _, err := c.ListCreatures(CreatureFilter{MinCR: "3", MaxCR: "2"}); !errors.Is(err, ErrChallengeRange) {
		t.Errorf("min 3 above max 2: %v", err)
	}
	if _, err := c.ListCreatures(CreatureFilter{MinCR: "1/3"}); !errors.Is(err, ErrChallengeRating) {
		t.Errorf("min 1/3: %v", err)
	}
	if _, err := c.ListCreatures(CreatureFilter{MaxCR: "31"}); !errors.Is(err, ErrChallengeRating) {
		t.Errorf("max 31: %v", err)
	}
	for f, field := range map[string]CreatureFilter{"min_cr": {MinCR: "1/3"}, "max_cr": {MaxCR: "99"}, "size": {Size: "Colossal"}} {
		_, err := c.ListCreatures(field)
		if fe, ok := errors.AsType[*CreatureFilterError](err); !ok || fe.Field != f {
			t.Errorf("a bad %s: error %v does not name the field", f, err)
		}
	}
	if _, err := c.ListCreatures(CreatureFilter{Size: "Colossal"}); !errors.Is(err, ErrCreatureSize) {
		t.Errorf("a size the SRD does not have: %v", err)
	}
}

func mustList(t *testing.T, c *Content, f CreatureFilter) []CreatureEntry {
	t.Helper()
	list, err := c.ListCreatures(f)
	if err != nil {
		t.Fatal(err)
	}
	return list
}

// TestCreaturesAreTheSRD: 334 creatures, with no creature of another book.
func TestCreaturesAreTheSRD(t *testing.T) {
	t.Parallel()
	c := loadForTest(t).c
	if len(c.monsters) != 334 {
		t.Errorf("%d creatures, want the SRD 5.1's 334", len(c.monsters))
	}
	for _, key := range []string{"monster:wolf", "monster:raven", "monster:imp", "monster:pseudodragon", "monster:quasit", "monster:sprite", "monster:skeleton", "monster:zombie"} {
		if _, ok := c.monsters[key]; !ok {
			t.Errorf("%s is missing", key)
		}
	}
}

// TestCRs reads the SRD's ratings in eighths.
func TestCRs(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]int{"0": 0, "1/8": 1, "1/4": 2, "1/2": 4, "1": 8, "2": 16, "30": 240} {
		if got, ok := crEighths(in); !ok || got != want {
			t.Errorf("crEighths(%q) = %d, %v; want %d", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "1/3", "31", "-1", "01", "0.5", "x"} {
		if _, ok := crEighths(in); ok {
			t.Errorf("crEighths(%q) accepted", in)
		}
	}
}

// TestLoadMonstersRefuses: a stat block that points at nothing never loads.
func TestLoadMonstersRefuses(t *testing.T) {
	t.Parallel()
	base := func() *srd51.Monster {
		return &srd51.Monster{
			Key: "monster:x", ChallengeRating: "1", Saves: map[string]int{"dex": 2}, Skills: map[string]int{"skill:stealth": 4},
			Immunities: []srd51.MonsterDamageMod{{Types: []string{"damage-type:fire"}}}, ConditionImmunities: []string{"condition:poisoned"},
			Actions: []srd51.MonsterAction{{Name: "Bite", Damage: []srd51.MonsterDamage{{Dice: "1d6", DamageType: "damage-type:fire"}}}},
		}
	}
	check := func(mutate func(*srd51.Monster)) error {
		m := base()
		mutate(m)
		c := &content{
			monsters: map[string]*srd51.Monster{m.Key: m},
			skills:   map[string]*srd51.Skill{"skill:stealth": {}},
			named:    map[string]*srd51.Named{"damage-type:fire": {}, "condition:poisoned": {}},
		}
		return c.checkMonsters()
	}
	if err := check(func(*srd51.Monster) {}); err != nil {
		t.Fatalf("a good stat block: %v", err)
	}
	bad := map[string]func(*srd51.Monster){
		"a bad challenge rating": func(m *srd51.Monster) { m.ChallengeRating = "1/3" },
		"an unknown skill":       func(m *srd51.Monster) { m.Skills["skill:nope"] = 1 },
		"an unknown save":        func(m *srd51.Monster) { m.Saves["luck"] = 1 },
		"an unknown condition":   func(m *srd51.Monster) { m.ConditionImmunities = []string{"condition:bored"} },
		"an unknown damage type": func(m *srd51.Monster) { m.Immunities[0].Types[0] = "damage-type:fun" },
		"bad damage dice":        func(m *srd51.Monster) { m.Actions[0].Damage[0].Dice = "lots" },
		"an unknown action type": func(m *srd51.Monster) { m.Actions[0].Damage[0].DamageType = "damage-type:fun" },
		"a count of 0": func(m *srd51.Monster) {
			m.Actions = append(m.Actions, srd51.MonsterAction{Name: "Multiattack", Multiattack: [][]srd51.MonsterAttackCount{{{Name: "Bite", Count: 0, Kind: "melee"}}}})
		},
		"a routine of an action that is not there": func(m *srd51.Monster) {
			m.Actions = append(m.Actions, srd51.MonsterAction{Name: "Multiattack", Multiattack: [][]srd51.MonsterAttackCount{{{Name: "Tail", Count: 1, Kind: "melee"}}}})
		},
		"armor that is not equipment": func(m *srd51.Monster) { m.ArmorClassItems = []string{"equipment:nope"} },
	}
	for name, mutate := range bad {
		if check(mutate) == nil {
			t.Errorf("%s: the loader accepted it", name)
		}
	}
}

func creatureOf(t *testing.T, c *Content, key string) Creature {
	t.Helper()
	cr, ok := c.CreatureByKey(key)
	if !ok {
		t.Fatalf("%s: creature not found", key)
	}
	return cr
}

func senseRange(cr Creature, key string) int {
	for _, s := range cr.Senses {
		if s.Key == key {
			return s.RangeFt
		}
	}
	return 0
}

// TestCreatureStatBlocksFollowTheSRD: armor class, speeds and senses the
// snapshot had wrong are the SRD stat block's.
func TestCreatureStatBlocksFollowTheSRD(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	if cr := creatureOf(t, c, "monster:basilisk"); cr.ArmorClass != 15 {
		t.Errorf("Basilisk armor class = %d, want 15", cr.ArmorClass)
	}
	if cr := creatureOf(t, c, "monster:crocodile"); cr.SpeedWalkFt != 20 || cr.SpeedSwimFt != 30 {
		t.Errorf("Crocodile speed = walk %d, swim %d, want 20 and 30", cr.SpeedWalkFt, cr.SpeedSwimFt)
	}
	if cr := creatureOf(t, c, "monster:giant-wasp"); cr.SpeedWalkFt != 10 || cr.SpeedFlyFt != 50 || cr.SpeedSwimFt != 0 {
		t.Errorf("Giant Wasp speed = walk %d, fly %d, swim %d, want 10, 50 and no swim", cr.SpeedWalkFt, cr.SpeedFlyFt, cr.SpeedSwimFt)
	}
	if cr := creatureOf(t, c, "monster:adult-brass-dragon"); cr.SpeedWalkFt != 40 || cr.SpeedBurrowFt != 30 || cr.SpeedFlyFt != 80 {
		t.Errorf("Adult Brass Dragon speed = walk %d, burrow %d, fly %d, want 40, 30 and 80", cr.SpeedWalkFt, cr.SpeedBurrowFt, cr.SpeedFlyFt)
	}
	shark := creatureOf(t, c, "monster:hunter-shark")
	if got := senseRange(shark, "blindsight"); got != 30 {
		t.Errorf("Hunter Shark blindsight = %d ft, want 30", got)
	}
	if got := senseRange(shark, "darkvision"); got != 0 {
		t.Errorf("Hunter Shark darkvision = %d ft, want none", got)
	}
}

// TestCultFanaticHitPoints: Cult Fanatic has 33 hit points, 6d8 + 6 (Constitution
// 12 adds 1 per die), and every creature's roll is its dice plus dice times its
// Constitution modifier, with the average as hit points.
func TestCultFanaticHitPoints(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	if cr := creatureOf(t, c, "monster:cult-fanatic"); cr.HitPoints != 33 || cr.HitPointsRoll != "6d8+6" {
		t.Errorf("Cult Fanatic hit points = %d (%s), want 33 (6d8+6)", cr.HitPoints, cr.HitPointsRoll)
	}
	list, err := c.ListCreatures(CreatureFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) < 300 {
		t.Fatalf("only %d creatures listed", len(list))
	}
	for _, e := range list {
		cr := creatureOf(t, c, e.Key)
		var n, sides, bonus int
		var sign string
		switch k, _ := fmt.Sscanf(cr.HitPointsRoll, "%dd%d%1s%d", &n, &sides, &sign, &bonus); k {
		case 2:
			sign, bonus = "+", 0
		case 4:
		default:
			t.Errorf("%s: unreadable hit point roll %q", e.Key, cr.HitPointsRoll)
			continue
		}
		if sign == "-" {
			bonus = -bonus
		}
		con := 10
		for _, a := range cr.Abilities {
			if a.Ability == CON {
				con = a.Base
			}
		}
		mod := (con - 10) / 2
		if con < 10 && (con-10)%2 != 0 {
			mod = (con - 11) / 2
		}
		if want := n*(sides+1)/2 + bonus; cr.HitPoints != want || bonus != n*mod {
			t.Errorf("%s: hit points %d, roll %q, Constitution %d; want the average %d and a bonus of %d", e.Key, cr.HitPoints, cr.HitPointsRoll, con, want, n*mod)
		}
	}
}

// TestCreatureImmunitiesFollowTheSRD: the condition immunities the snapshot lost
// (deafened) and the Ice Devil's cold immunity.
func TestCreatureImmunitiesFollowTheSRD(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	want := map[string][]string{
		"monster:black-pudding":     {"blinded", "charmed", "deafened", "exhaustion", "frightened", "prone"},
		"monster:flying-sword":      {"blinded", "charmed", "deafened", "frightened", "paralyzed", "petrified", "poisoned"},
		"monster:rug-of-smothering": {"blinded", "charmed", "deafened", "frightened", "paralyzed", "petrified", "poisoned"},
		"monster:ochre-jelly":       {"blinded", "charmed", "deafened", "exhaustion", "frightened", "prone"},
		"monster:shambling-mound":   {"blinded", "deafened", "exhaustion"},
		"monster:shrieker":          {"blinded", "deafened", "frightened"},
		"monster:violet-fungus":     {"blinded", "deafened", "frightened"},
	}
	for key, conditions := range want {
		var got []string
		for _, ci := range creatureOf(t, c, key).ConditionImmunities {
			got = append(got, strings.TrimPrefix(ci.Key, "condition:"))
		}
		slices.Sort(got)
		if !slices.Equal(got, conditions) {
			t.Errorf("%s condition immunities = %v, want %v", key, got, conditions)
		}
	}
	var got []string
	for _, m := range creatureOf(t, c, "monster:ice-devil").Immunities {
		for _, ty := range m.Types {
			got = append(got, ty.Key)
		}
	}
	slices.Sort(got)
	if want := []string{"damage-type:cold", "damage-type:fire", "damage-type:poison"}; !slices.Equal(got, want) {
		t.Errorf("Ice Devil damage immunities = %v, want %v", got, want)
	}
}

// TestCreaturePerceptionFollowsTheSRD: the skills the snapshot left out are
// listed, and every creature's passive Perception is 10 plus its Perception
// bonus (the listed skill, or else the Wisdom modifier).
func TestCreaturePerceptionFollowsTheSRD(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	for _, tc := range []struct {
		key, skill string
		bonus      int
	}{
		{"monster:black-bear", "skill:perception", 3},
		{"monster:half-red-dragon-veteran", "skill:athletics", 5},
		{"monster:half-red-dragon-veteran", "skill:perception", 2},
		{"monster:swarm-of-ravens", "skill:perception", 5},
	} {
		got, listed := 0, false
		for _, s := range creatureOf(t, c, tc.key).Skills {
			if s.Key == tc.skill {
				got, listed = s.Bonus, true
			}
		}
		if !listed || got != tc.bonus {
			t.Errorf("%s: %s = %+d (listed %v), want %+d", tc.key, tc.skill, got, listed, tc.bonus)
		}
	}
	if cr := creatureOf(t, c, "monster:blink-dog"); cr.PassivePerception != 13 {
		t.Errorf("Blink Dog passive Perception = %d, want 13", cr.PassivePerception)
	}
	list, err := c.ListCreatures(CreatureFilter{})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range list {
		// The SRD's Spider says passive Perception 12 with Wisdom 10 and no
		// Perception skill; the other transcription says 10, so it is left out.
		if e.Key == "monster:spider" {
			continue
		}
		cr := creatureOf(t, c, e.Key)
		bonus, listed := 0, false
		for _, s := range cr.Skills {
			if s.Key == "skill:perception" {
				bonus, listed = s.Bonus, true
			}
		}
		if !listed {
			for _, a := range cr.Abilities {
				if a.Ability == WIS {
					bonus = a.Modifier
				}
			}
		}
		if want := 10 + bonus; cr.PassivePerception != want {
			t.Errorf("%s: passive Perception %d, want %d (Perception %+d, skill listed %v)", e.Key, cr.PassivePerception, want, bonus, listed)
		}
	}
}

// A creature's attack that says "reach 10 ft. or range 20/60 ft." keeps both: the reach it has in melee and
// the range it throws at.
func TestAMonsterAttackKeepsItsReachAndItsThrowingRange(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	m := &srd51.Monster{ProficiencyBonus: 2}
	for _, tc := range []struct {
		desc                string
		reach, rng, longRng int
	}{
		{"Melee or Ranged Weapon Attack: +4 to hit, reach 10 ft. or range 20/60 ft., one target.", 10, 20, 60},
		{"Melee or Ranged Weapon Attack: +4 to hit, reach 5 ft. or range 20/60 ft., one target.", 0, 20, 60},
		{"Melee Weapon Attack: +4 to hit, reach 10 ft., one target.", 10, 10, 0},
		{"Melee Weapon Attack: +4 to hit, reach 5 ft., one target.", 0, 5, 0},
	} {
		a := c.c.monsterAttack(m, srd51.MonsterAction{Name: "Lança", Desc: tc.desc, HasAttack: true, AttackBonus: 4}, "attack:test", map[Ability]int{})
		if a.ReachFt != tc.reach || a.RangeFt != tc.rng || a.LongRangeFt != tc.longRng {
			t.Errorf("%q: reach %d, range %d/%d, want %d, %d/%d", tc.desc, a.ReachFt, a.RangeFt, a.LongRangeFt, tc.reach, tc.rng, tc.longRng)
		}
	}
}
