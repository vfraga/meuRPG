package rules

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/PuraFome/meuRPG/backend/internal/rules/encounter"
)

// The table's creatures. The names are invented; the base is the SRD's wolf.

const (
	packLeader = "creature:lobo-alfa@mesa"
	wolfKey    = "monster:wolf"
)

func packLeaderCreature() TableCreature {
	return TableCreature{
		TableEntry: TableEntry{Key: packLeader, NamePT: "Lobo Alfa"}, BaseKey: wolfKey,
		HitPoints: 30, ArmorClass: 15, DamageBonus: 3,
		Traits: []TableCreatureTrait{{NamePT: "Uivo do Alfa", TextPT: "Aliados a 30 pés ouvem o uivo."}},
	}
}

func creatureOverlay(cs ...TableCreature) Overlay { return Overlay{Revision: 1, Creatures: cs} }

func TestACreatureHasTheAdjustmentsInItsDerived(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	c := withOverlayOn(t, srd, creatureOverlay(packLeaderCreature()))
	base, _ := c.MonsterDerived(wolfKey)
	d, ok := c.MonsterDerived(packLeader)
	if !ok {
		t.Fatal("MonsterDerived does not know the table creature")
	}
	if d.HitPointsMax != 30 || d.ArmorClass != 15 {
		t.Errorf("HP %d AC %d, want 30 and 15", d.HitPointsMax, d.ArmorClass)
	}
	if base.HitPointsMax != 11 || base.ArmorClass != 13 {
		t.Errorf("the base wolf changed: HP %d AC %d", base.HitPointsMax, base.ArmorClass)
	}
	if len(base.Attacks) != 1 || len(d.Attacks) != 1 || d.Attacks[0].Damage != "2d4+5" || d.Attacks[0].Key != packLeader+"#bite" {
		t.Errorf("attacks = %+v, want the bite with 2d4+5", d.Attacks)
	}
	if got := d.Attacks[0].DamageDice; got.Count != 2 || got.Sides != 4 || got.Bonus != 5 {
		t.Errorf("DamageDice = %+v, want 2d4+5", got)
	}
	if d.Attacks[0].AttackBonus != base.Attacks[0].AttackBonus {
		t.Error("the attack bonus changed")
	}
	if d.SpeedWalkFt != base.SpeedWalkFt || len(d.Abilities) != len(base.Abilities) {
		t.Error("what is not adjusted differs from the base")
	}
	// The traits come after the base's, as text.
	var names []string
	for _, f := range d.Features {
		names = append(names, f.Name)
	}
	if want := append(featureNames(base), "Uivo do Alfa"); !slices.Equal(names, want) {
		t.Errorf("features = %v, want %v", names, want)
	}
	last := d.Features[len(d.Features)-1]
	if last.Key != packLeader+"#uivo-do-alfa" || last.Description[0] != "Aliados a 30 pés ouvem o uivo." || last.SourcePT != "Lobo Alfa" {
		t.Errorf("trait = %+v", last)
	}
	// The plan reads the adjusted damage too.
	ap, ok := c.MonsterActionPlan(packLeader + "#bite")
	if !ok || len(ap.Damage) != 1 || ap.Damage[0].DiceText != "2d4+5" {
		t.Errorf("plan = %+v, %v, want the bite with 2d4+5", ap, ok)
	}
	// The extra attacks of a Multiattack and the per-action count are the base's.
	if d.AttacksPerAction != base.AttacksPerAction {
		t.Errorf("AttacksPerAction = %d, want %d", d.AttacksPerAction, base.AttacksPerAction)
	}
}

func featureNames(d Derived) []string {
	var out []string
	for _, f := range d.Features {
		out = append(out, f.Name)
	}
	return out
}

func TestEachAdjustmentWorksAlone(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	base, _ := srd.MonsterDerived(wolfKey)
	mk := func(edit func(*TableCreature)) Derived {
		cr := TableCreature{TableEntry: TableEntry{Key: packLeader, NamePT: "Lobo Alfa"}, BaseKey: wolfKey}
		edit(&cr)
		d, ok := withOverlayOn(t, srd, creatureOverlay(cr)).MonsterDerived(packLeader)
		if !ok {
			t.Fatal("no creature")
		}
		return d
	}
	if d := mk(func(*TableCreature) {}); len(d.Attacks) != 1 || d.HitPointsMax != base.HitPointsMax || d.ArmorClass != base.ArmorClass || d.Attacks[0].Damage != "2d4+2" || len(d.Features) != len(base.Features) {
		t.Errorf("no adjustment: HP %d AC %d damage %s features %d, want the base's", d.HitPointsMax, d.ArmorClass, d.Attacks[0].Damage, len(d.Features))
	}
	if d := mk(func(c *TableCreature) { c.HitPoints = 1 }); d.HitPointsMax != 1 || d.ArmorClass != base.ArmorClass {
		t.Errorf("hit points only: HP %d AC %d", d.HitPointsMax, d.ArmorClass)
	}
	if d := mk(func(c *TableCreature) { c.ArmorClass = 40 }); d.ArmorClass != 40 || d.HitPointsMax != base.HitPointsMax {
		t.Errorf("armor class only: HP %d AC %d", d.HitPointsMax, d.ArmorClass)
	}
	if d := mk(func(c *TableCreature) { c.DamageBonus = -10 }); len(d.Attacks) != 1 || d.Attacks[0].Damage != "2d4-8" {
		t.Errorf("negative bonus: damage %s, want 2d4-8", d.Attacks[0].Damage)
	}
	// Flat damage (the raven's beak is 1) stays a number.
	cr := TableCreature{TableEntry: TableEntry{Key: packLeader, NamePT: "Corvo Grande"}, BaseKey: "monster:raven", DamageBonus: 2}
	if d, _ := withOverlayOn(t, srd, creatureOverlay(cr)).MonsterDerived(packLeader); len(d.Attacks) != 1 || d.Attacks[0].Damage != "3" {
		t.Errorf("flat damage = %s, want 3", d.Attacks[0].Damage)
	}
}

func TestACreatureResolvesWhereAnSRDCreatureDoes(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	c := withOverlayOn(t, srd, creatureOverlay(packLeaderCreature()))
	cr, ok := c.CreatureByKey(packLeader)
	if !ok || cr.NamePT != "Lobo Alfa" || cr.HitPoints != 30 || cr.ArmorClass != 15 || cr.Size != "Medium" || cr.Type != "beast" {
		t.Errorf("CreatureByKey = %+v, %v", cr.CreatureEntry, ok)
	}
	if len(cr.Traits) == 0 || cr.Traits[len(cr.Traits)-1].Name != "Uivo do Alfa" {
		t.Errorf("traits = %+v, want the table's last", cr.Traits)
	}
	if len(cr.Actions) == 0 || cr.Actions[0].Plan == nil {
		t.Error("the creature's action has no plan")
	}
	if c.c.namePT(packLeader) != "Lobo Alfa" {
		t.Errorf("namePT = %q", c.c.namePT(packLeader))
	}
	list, err := c.ListCreatures(CreatureFilter{Query: "lobo alfa"})
	if err != nil || len(list) != 1 || list[0].Key != packLeader || list[0].HitPoints != 30 {
		t.Errorf("ListCreatures = %+v, %v, want the table creature", list, err)
	}
	all, _ := c.ListCreatures(CreatureFilter{})
	plain, _ := srd.ListCreatures(CreatureFilter{})
	if len(all) != len(plain)+1 {
		t.Errorf("list has %d creatures, want %d", len(all), len(plain)+1)
	}
	if !slices.IsSortedFunc(all, func(a, b CreatureEntry) int { return comparePT(a.NamePT, b.NamePT) }) {
		t.Error("the list is not sorted by Portuguese name")
	}
	if c.HasCreature(packLeader) {
		t.Error("HasCreature = true for a table creature, want SRD creatures only")
	}
	if !c.HasCreature("monster:wolf") {
		t.Error("HasCreature(monster:wolf) = false")
	}
	if err := c.c.checkSummonCreatures([]string{packLeader}, map[string]bool{}, func(f string, _ ...any) error { return errors.New(f) }); err != nil {
		t.Errorf("a summon refuses the table creature: %v", err)
	}
	if got := c.Creatures(); len(got) != 1 || got[0].Key != packLeader || got[0].BaseKey != wolfKey || got[0].NamePT != "Lobo Alfa" {
		t.Errorf("Creatures() = %+v", got)
	}
	// Wild Shape reads the creature as a beast form.
	d, err := c.WildShapeDerived(Derived{}, packLeader)
	if err != nil || d.HitPointsMax != 30 || d.ArmorClass != 15 {
		t.Errorf("WildShapeDerived = HP %d AC %d, %v", d.HitPointsMax, d.ArmorClass, err)
	}
}

func TestACreatureDoesNotChangeTheContentItCameFrom(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	before, _ := srd.MonsterDerived(wolfKey)
	nBefore := len(srd.c.monsterEntries)
	c := withOverlayOn(t, srd, creatureOverlay(packLeaderCreature()))
	if _, ok := srd.MonsterDerived(packLeader); ok {
		t.Error("the original content knows the table creature")
	}
	if _, ok := srd.CreatureByKey(packLeader); ok || srd.HasCreature(packLeader) || len(srd.c.monsterEntries) != nBefore || len(srd.Creatures()) != 0 {
		t.Error("the original content changed")
	}
	if _, ok := srd.MonsterActionPlan(packLeader + "#bite"); ok {
		t.Error("the original content has the plan")
	}
	after, _ := srd.MonsterDerived(wolfKey)
	if m := srd.c.monsters[wolfKey]; m.HitPoints != 11 || m.ArmorClass != 13 || m.ArmorClassType != "natural" || m.Name != "Wolf" || m.Key != wolfKey {
		t.Errorf("the SRD wolf's stat block changed: HP %d AC %d (%s) %s %s", m.HitPoints, m.ArmorClass, m.ArmorClassType, m.Name, m.Key)
	}
	if len(after.Attacks) != 1 || len(before.Attacks) != 1 {
		t.Fatal("the wolf has not one attack")
	}
	if after.HitPointsMax != before.HitPointsMax || after.ArmorClass != before.ArmorClass || after.Attacks[0].Damage != "2d4+2" || len(after.Features) != len(before.Features) {
		t.Errorf("the SRD wolf changed: %+v", after.Attacks)
	}
	viaClone, _ := c.MonsterDerived(wolfKey)
	if viaClone.Attacks[0].Damage != "2d4+2" || viaClone.HitPointsMax != 11 || len(viaClone.Features) != len(before.Features) {
		t.Error("the wolf seen through the new content changed")
	}
	if m := srd.c.monsters[wolfKey]; len(m.SpecialAbilities) != len(before.Features) || m.Actions[0].Damage[0].Dice != "2d4+2" {
		t.Error("the shared SRD stat block was written")
	}
}

func TestAnArchivedOrSwitchedOffCreatureStillResolves(t *testing.T) {
	t.Parallel()
	cr := packLeaderCreature()
	cr.Archived = true
	o := creatureOverlay(cr)
	o.Off = []string{packLeader}
	c := withOverlay(t, o)
	if d, ok := c.MonsterDerived(packLeader); !ok || d.HitPointsMax != 30 {
		t.Errorf("MonsterDerived = %d, %v", d.HitPointsMax, ok)
	}
	if got := c.Creatures(); len(got) != 1 || !got[0].Archived || !got[0].Off {
		t.Errorf("Creatures() = %+v, want it archived and off", got)
	}
	if !c.Switchable(packLeader) {
		t.Error("the table creature cannot be switched off")
	}
}

func TestACreatureKeyIsChecked(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	cases := []struct {
		name   string
		cs     func() []TableCreature
		reason string
	}{
		{"a wrong prefix", func() []TableCreature {
			c := packLeaderCreature()
			c.Key = "monster:lobo@mesa"
			return []TableCreature{c}
		}, ReasonKey},
		{"no suffix", func() []TableCreature {
			c := packLeaderCreature()
			c.Key = "creature:lobo"
			return []TableCreature{c}
		}, ReasonKey},
		{"a bad slug", func() []TableCreature {
			c := packLeaderCreature()
			c.Key = "creature:Lobo Alfa@mesa"
			return []TableCreature{c}
		}, ReasonKey},
		{"a reserved word", func() []TableCreature {
			c := packLeaderCreature()
			c.Key = "creature:wild-shape-lobo@mesa"
			return []TableCreature{c}
		}, ReasonReservedKey},
		{"a repeated key", func() []TableCreature {
			a, b := packLeaderCreature(), packLeaderCreature()
			b.NamePT = "Outro"
			return []TableCreature{a, b}
		}, ReasonDuplicateKey},
		{"an empty name", func() []TableCreature {
			c := packLeaderCreature()
			c.NamePT = ""
			return []TableCreature{c}
		}, ReasonName},
	}
	for _, tc := range cases {
		_, err := srd.With(creatureOverlay(tc.cs()...))
		oe, ok := errors.AsType[*OverlayError](err)
		if !ok {
			t.Errorf("%s: err = %v, want an overlay error", tc.name, err)
			continue
		}
		if oe.Reason != tc.reason {
			t.Errorf("%s: reason %q, want %q (%v)", tc.name, oe.Reason, tc.reason, err)
		}
	}
}

func TestACreatureLimitIsTheTableLimit(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	mk := func(n int) []TableCreature {
		var out []TableCreature
		for i := range n {
			c := packLeaderCreature()
			c.Key, c.NamePT = "creature:lobo-"+itoa(i)+tableSuffix, "Lobo "+itoa(i)
			c.Traits = nil
			out = append(out, c)
		}
		return out
	}
	c, err := srd.With(creatureOverlay(mk(MaxOverlayEntries)...))
	if err != nil {
		t.Fatalf("%d creatures: %v", MaxOverlayEntries, err)
	}
	if got := len(c.Creatures()); got != MaxOverlayEntries {
		t.Errorf("Creatures() = %d", got)
	}
	_, err = srd.With(creatureOverlay(mk(MaxOverlayEntries + 1)...))
	if oe, ok := errors.AsType[*OverlayError](err); !ok || oe.Reason != ReasonLimit {
		t.Errorf("1001 creatures: err = %v, want the limit", err)
	}
}

func TestACreatureRefusalsNameTheirField(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	trait := func(n, x string) TableCreatureTrait { return TableCreatureTrait{NamePT: n, TextPT: x} }
	many := make([]TableCreatureTrait, MaxCreatureTraits+1)
	for i := range many {
		many[i] = trait("Traço "+string(rune('a'+i)), "x")
	}
	cases := []struct {
		name          string
		edit          func(c *TableCreature)
		field, reason string
	}{
		{"a base that does not exist", func(c *TableCreature) { c.BaseKey = "monster:nao-existe" }, ".base_key", ReasonReference},
		{"no base", func(c *TableCreature) { c.BaseKey = "" }, ".base_key", ReasonReference},
		{"a table creature as base", func(c *TableCreature) { c.BaseKey = "creature:outro@mesa" }, ".base_key", ReasonReference},
		{"a spell as base", func(c *TableCreature) { c.BaseKey = "spell:light" }, ".base_key", ReasonReference},
		{"negative hit points", func(c *TableCreature) { c.HitPoints = -1 }, ".hit_points", ReasonValue},
		{"too many hit points", func(c *TableCreature) { c.HitPoints = 10000 }, ".hit_points", ReasonValue},
		{"negative armor class", func(c *TableCreature) { c.ArmorClass = -1 }, ".armor_class", ReasonValue},
		{"armor class 41", func(c *TableCreature) { c.ArmorClass = 41 }, ".armor_class", ReasonValue},
		{"damage bonus -11", func(c *TableCreature) { c.DamageBonus = -11 }, ".damage_bonus", ReasonValue},
		{"damage bonus 31", func(c *TableCreature) { c.DamageBonus = 31 }, ".damage_bonus", ReasonValue},
		{"11 traits", func(c *TableCreature) { c.Traits = many }, ".traits", ReasonLimit},
		{"an empty trait name", func(c *TableCreature) { c.Traits = []TableCreatureTrait{trait("", "x")} }, ".traits[0].name_pt", ReasonName},
		{"a long trait name", func(c *TableCreature) { c.Traits = []TableCreatureTrait{trait(strings.Repeat("a", 81), "x")} }, ".traits[0].name_pt", ReasonName},
		{"a trait name on two lines", func(c *TableCreature) { c.Traits = []TableCreatureTrait{trait("a\nb", "x")} }, ".traits[0].name_pt", ReasonName},
		{"a trait name with no letter", func(c *TableCreature) { c.Traits = []TableCreatureTrait{trait("!!!", "x")} }, ".traits[0].name_pt", ReasonName},
		{"a trait named as a base trait", func(c *TableCreature) { c.Traits = []TableCreatureTrait{trait("Pack Tactics", "x")} }, ".traits[0].name_pt", ReasonName},
		{"two traits with a name", func(c *TableCreature) { c.Traits = []TableCreatureTrait{trait("Uivo", "x"), trait("uivo", "y")} }, ".traits[1].name_pt", ReasonName},
		{"an empty trait text", func(c *TableCreature) { c.Traits = []TableCreatureTrait{trait("Uivo", "")} }, ".traits[0].text_pt", ReasonText},
		{"a long trait text", func(c *TableCreature) { c.Traits = []TableCreatureTrait{trait("Uivo", strings.Repeat("a", 4001))} }, ".traits[0].text_pt", ReasonText},
	}
	for _, tc := range cases {
		cr := packLeaderCreature()
		tc.edit(&cr)
		_, err := srd.With(creatureOverlay(cr))
		oe, ok := errors.AsType[*OverlayError](err)
		if !ok {
			t.Errorf("%s: err = %v, want an overlay error", tc.name, err)
			continue
		}
		found := false
		for _, v := range oe.Violations() {
			found = found || strings.HasSuffix(v.Field, tc.field) && v.Reason == tc.reason
		}
		if !found {
			t.Errorf("%s: refused at %+v, want %s (%s)", tc.name, oe.Violations(), tc.field, tc.reason)
		}
	}
	// The edges are accepted.
	edge := packLeaderCreature()
	edge.HitPoints, edge.ArmorClass, edge.DamageBonus = MaxCreatureHitPoints, MaxCreatureArmorClass, MaxCreatureDamageBonus
	edge.Traits = make([]TableCreatureTrait, MaxCreatureTraits)
	for i := range edge.Traits {
		edge.Traits[i] = trait("Traço "+string(rune('a'+i)), strings.Repeat("ã", maxTextRunes))
	}
	if _, err := srd.With(creatureOverlay(edge)); err != nil {
		t.Errorf("the limits themselves were refused: %v", err)
	}
}

func TestATraitMayNotTakeANameTheEngineUsesOrTheBaseHas(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	cases := []struct{ base, name string }{
		{wolfKey, "Bite"},
		{wolfKey, "bite"},
		{wolfKey, "Keen Hearing and Smell"},
		{"monster:brown-bear", "Multiattack"},
		{"monster:brown-bear", "Claws"},
		{wolfKey, "Magic Weapons"},
		{wolfKey, "magic weapons"},
		{wolfKey, "Spellcasting"},
		{wolfKey, "SPELLCASTING"},
		{wolfKey, "Innate Spellcasting"},
	}
	for _, tc := range cases {
		cr := TableCreature{
			TableEntry: TableEntry{Key: packLeader, NamePT: "Lobo Alfa"}, BaseKey: tc.base,
			Traits: []TableCreatureTrait{{NamePT: tc.name, TextPT: "x"}},
		}
		_, err := srd.With(creatureOverlay(cr))
		oe, ok := errors.AsType[*OverlayError](err)
		if !ok || oe.Reason != ReasonName || !strings.HasSuffix(oe.Field, ".traits[0].name_pt") {
			t.Errorf("%s on %s: err = %v (%+v), want bad_name at .traits[0].name_pt", tc.name, tc.base, err, oe)
		}
	}
	// A name that only looks near is fine.
	cr := packLeaderCreature()
	cr.Traits = []TableCreatureTrait{{NamePT: "Mordida Alfa", TextPT: "x"}, {NamePT: "Spellcasting Lore", TextPT: "x"}}
	if _, err := srd.With(creatureOverlay(cr)); err != nil {
		t.Errorf("harmless names refused: %v", err)
	}
}

func TestATableCreatureIsLeftOutOfTheEncounterTools(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	c := withOverlayOn(t, srd, creatureOverlay(packLeaderCreature()))
	// Evaluate: only SRD creatures.
	if _, err := c.EvaluateEncounter([]int{3}, []encounter.Entry{{Key: packLeader, Count: 1}}); !errors.Is(err, ErrEncounterCreature) {
		t.Errorf("EvaluateEncounter err = %v, want ErrEncounterCreature", err)
	}
	if _, err := c.EvaluateEncounter([]int{3}, []encounter.Entry{{Key: wolfKey, Count: 1}}); err != nil {
		t.Errorf("the SRD wolf is refused: %v", err)
	}
	// Swaps: neither a swap for it, nor a swap to it.
	if _, err := c.EncounterSwaps(packLeader, ""); !errors.Is(err, ErrEncounterCreature) {
		t.Errorf("EncounterSwaps(table creature) err = %v, want ErrEncounterCreature", err)
	}
	swaps, err := c.EncounterSwaps(wolfKey, "")
	if err != nil {
		t.Fatal(err)
	}
	plain, _ := srd.EncounterSwaps(wolfKey, "")
	if len(swaps) != len(plain) || slices.ContainsFunc(swaps, func(e CreatureEntry) bool { return e.Key == packLeader }) {
		t.Errorf("swaps of the wolf: %d (want %d), with the table creature: %v", len(swaps), len(plain), len(swaps) != len(plain))
	}
	// Generate: the pool is the SRD's, so the answer is the one without the table creature.
	for seed := range uint32(40) {
		got, err1 := c.GenerateEncounter([]int{1, 1, 1, 1}, encounter.BandModerate, "beast", seed, 0)
		want, err2 := srd.GenerateEncounter([]int{1, 1, 1, 1}, encounter.BandModerate, "beast", seed, 0)
		if (err1 == nil) != (err2 == nil) || err1 == nil && !slices.EqualFunc(got.Lines, want.Lines, func(a, b EncounterLine) bool { return a.Creature.Key == b.Creature.Key && a.Count == b.Count }) {
			t.Fatalf("seed %d: the table creature changed the generated encounter: %v / %v", seed, err1, err2)
		}
	}
	// It stays in the creature list.
	if l, _ := c.ListCreatures(CreatureFilter{Query: "lobo alfa"}); len(l) != 1 {
		t.Error("the table creature left the creature list")
	}
}

func TestATableCreatureIsNotAnSRDSummonOrWildShapeForm(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	cr := packLeaderCreature()
	c := withOverlayOn(t, srd, creatureOverlay(cr))
	// Conjure Animals takes beasts of a rating: the wolf is one, and so would the table creature be.
	b := choiceBuild("race:human", "", ClassLevel{Class: "class:druid", Level: 9})
	opts, err := c.SummonOptions("spell:conjure-animals", 3, b)
	if err != nil || len(opts.Options) == 0 {
		t.Fatalf("SummonOptions: %+v, %v", opts, err)
	}
	var pick *SummonOption
	for i, o := range opts.Options {
		if o.Type == "beast" && len(o.Forms) == 0 && o.Count > 0 {
			pick = &opts.Options[i]
			if _, err := c.CheckSummon("spell:conjure-animals", 3, b, SummonPick{Option: i, Creatures: slices.Repeat([]string{wolfKey}, o.Count)}); err != nil {
				t.Fatalf("the SRD wolf is refused: %v", err)
			}
			_, err := c.CheckSummon("spell:conjure-animals", 3, b, SummonPick{Option: i, Creatures: slices.Repeat([]string{packLeader}, o.Count)})
			if !errors.Is(err, ErrSummonCreature) {
				t.Errorf("CheckSummon with the table creature: err = %v, want ErrSummonCreature", err)
			}
			break
		}
	}
	if pick == nil {
		t.Fatal("no beast option to test")
	}
	// Wild Shape: not offered as a form.
	druid := choiceBuild("race:human", "", ClassLevel{Class: "class:druid", Level: 4})
	if !slices.ContainsFunc(c.WildShapeForms(druid), func(e CreatureEntry) bool { return e.Key == wolfKey }) {
		t.Fatal("the SRD wolf is not a form")
	}
	if c.WildShapeAllows(druid, packLeader) || slices.ContainsFunc(c.WildShapeForms(druid), func(e CreatureEntry) bool { return e.Key == packLeader }) {
		t.Error("the table creature is a Wild Shape form")
	}
	if got, want := len(c.WildShapeForms(druid)), len(srd.WildShapeForms(druid)); got != want {
		t.Errorf("forms = %d, want %d", got, want)
	}
}

func TestAMultiattackFollowsTheNewKeyAndTheDamageBonus(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	// The brown bear's count is corrected (corrections.json); the troll's is the stat block's routine.
	for _, base := range []string{"monster:brown-bear", "monster:troll"} {
		_, corrected := srd.c.attacksPerAction[base]
		if (base == "monster:brown-bear") != corrected {
			t.Fatalf("%s: corrected = %v", base, corrected)
		}
		bd, _ := srd.MonsterDerived(base)
		if bd.AttacksPerAction < 2 {
			t.Fatalf("%s: AttacksPerAction = %d, want 2 or more", base, bd.AttacksPerAction)
		}
		key := "creature:multi@mesa"
		c := withOverlayOn(t, srd, creatureOverlay(TableCreature{TableEntry: TableEntry{Key: key, NamePT: "Multi"}, BaseKey: base, DamageBonus: 4}))
		d, ok := c.MonsterDerived(key)
		if !ok || d.AttacksPerAction != bd.AttacksPerAction {
			t.Errorf("%s: AttacksPerAction = %d, want %d", base, d.AttacksPerAction, bd.AttacksPerAction)
		}
		if len(d.Attacks) != len(bd.Attacks) || len(d.Attacks) < 2 {
			t.Fatalf("%s: %d attacks, want %d (2 or more)", base, len(d.Attacks), len(bd.Attacks))
		}
		for i, a := range d.Attacks {
			want, _ := ParseDice(bd.Attacks[i].Damage)
			got, _ := ParseDice(a.Damage)
			if got.Count != want.Count || got.Sides != want.Sides || got.Bonus != want.Bonus+4 || !strings.HasPrefix(a.Key, key+"#") {
				t.Errorf("%s: attack %d = %s %s, want the base's %s plus 4", base, i, a.Key, a.Damage, bd.Attacks[i].Damage)
			}
		}
		plan, ok := c.MonsterPlan(key)
		if !ok || plan.Key != key {
			t.Fatalf("%s: no plan under the new key", base)
		}
		steps := 0
		for _, ap := range plan.Actions {
			if !strings.HasPrefix(ap.Key, key+"#") {
				t.Errorf("%s: action key %s", base, ap.Key)
			}
			for _, r := range ap.Routines {
				for _, st := range r {
					steps++
					if st.ActionKey != "" {
						if !strings.HasPrefix(st.ActionKey, key+"#") {
							t.Errorf("%s: routine step key %s", base, st.ActionKey)
						} else if _, ok := plan.Action(st.ActionKey); !ok {
							t.Errorf("%s: routine step %s is no action of the plan", base, st.ActionKey)
						}
					}
				}
			}
		}
		if steps == 0 {
			t.Errorf("%s: no Multiattack routine in the plan", base)
		}
	}
}

func TestTheDamageBonusReadsEveryShapeOfAction(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		dice, want string
		n          int
	}{
		{"2d4+2", "2d4+5", 3},
		{"1d6", "1d6+1", 1},
		{"1d6", "1d6-2", -2},
		{"2d6-1", "2d6", 1},
		{"5", "8", 3},
		{"1", "0", -10},
		{"0", "0", -1},
		{"1d8 + MOD", "1d8 + MOD", 5},
		{"4d6 OR 5d6", "4d6 OR 5d6", 2},
	} {
		if got := addDiceBonus(tc.dice, tc.n); got != tc.want {
			t.Errorf("addDiceBonus(%q, %d) = %q, want %q", tc.dice, tc.n, got, tc.want)
		}
	}
	srd := loadForTest(t)
	// Fixtures: a creature with an attack of several damage parts, one with a save-only action that
	// deals damage, and one with a Multiattack action.
	var multi, save, routine string
	for _, k := range sortedKeys(srd.c.monsters) {
		for _, a := range srd.c.monsters[k].Actions {
			switch {
			case multi == "" && a.HasAttack && len(a.Damage) > 1:
				multi = k
			case save == "" && !a.HasAttack && a.Save != nil && len(a.Damage) > 0:
				save = k
			case routine == "" && len(a.Multiattack) > 0:
				routine = k
			}
		}
	}
	if multi == "" || save == "" || routine == "" {
		t.Fatalf("fixtures: multi %q save %q multiattack %q", multi, save, routine)
	}
	const key = "creature:x@mesa"
	with := func(base string, bonus int) *Content {
		return withOverlayOn(t, srd, creatureOverlay(TableCreature{TableEntry: TableEntry{Key: key, NamePT: "X"}, BaseKey: base, DamageBonus: bonus}))
	}
	// Several damage parts: only the first gets the bonus (it is added once per hit).
	c := with(multi, 2)
	checked := 0
	for _, a := range srd.c.monsters[multi].Actions {
		if !a.HasAttack || len(a.Damage) < 2 {
			continue
		}
		ap, ok := srd.MonsterActionPlan(multi + "#" + slugOf(a.Name))
		cp, ok2 := c.MonsterActionPlan(key + "#" + slugOf(a.Name))
		if !ok || !ok2 || len(cp.Damage) != len(ap.Damage) {
			t.Fatalf("plans of %s", a.Name)
		}
		for i := range ap.Damage {
			wantBonus := ap.Damage[i].Dice.Bonus
			if i == 0 {
				wantBonus += 2
			}
			if cp.Damage[i].Dice.Bonus != wantBonus || cp.Damage[i].TypeKey != ap.Damage[i].TypeKey {
				t.Errorf("%s part %d = %+v, want bonus %d", a.Name, i, cp.Damage[i], wantBonus)
			}
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("no multi-part attack checked")
	}
	// A save-only action and the Multiattack action are untouched.
	for _, k := range []string{save, routine} {
		cm := with(k, 7).c.monsters[key]
		for i, a := range srd.c.monsters[k].Actions {
			if a.HasAttack {
				continue
			}
			if !slices.Equal(cm.Actions[i].Damage, a.Damage) || cm.Actions[i].Desc != a.Desc {
				t.Errorf("%s: action %q changed without an attack roll", k, a.Name)
			}
		}
	}
}

func TestAFlatNegativeBonusNeverGoesBelowZero(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	key := "creature:corvo@mesa"
	c := withOverlayOn(t, srd, creatureOverlay(TableCreature{TableEntry: TableEntry{Key: key, NamePT: "Corvo"}, BaseKey: "monster:raven", DamageBonus: -10}))
	d, _ := c.MonsterDerived(key)
	if len(d.Attacks) != 1 || d.Attacks[0].Damage != "0" || d.Attacks[0].DamageDice.Bonus != 0 {
		t.Errorf("raven with -10: %+v, want damage 0", d.Attacks)
	}
}

func TestAnArmorClassOverrideDropsTheBaseKindOfArmor(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	cr := packLeaderCreature()
	c := withOverlayOn(t, srd, creatureOverlay(cr))
	d, _ := c.MonsterDerived(packLeader)
	if strings.Contains(d.ArmorClassDescription, "natural") || strings.Contains(d.ArmorClassDescription, "Armadura natural") {
		t.Errorf("description = %q, want no natural armor", d.ArmorClassDescription)
	}
	cr.ArmorClass = 0
	d, _ = withOverlayOn(t, srd, creatureOverlay(cr)).MonsterDerived(packLeader)
	if d.ArmorClassDescription != "Armadura natural" {
		t.Errorf("without an override: %q, want the base's", d.ArmorClassDescription)
	}
}
