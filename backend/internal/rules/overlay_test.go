package rules

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// fingerprint is everything the base content shows from the outside, and the
// parts With might write by mistake: the catalog, the sorted keys of the maps
// the overlay changes, and the lists it extends on copies.
func fingerprint(t testing.TB, c *Content) string {
	t.Helper()
	x := c.c
	cat, err := json.Marshal(c.Catalog())
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	b.Write(cat)
	b.WriteString(x.version)
	for _, k := range sortedKeys(x.classes) {
		b.WriteString("|" + k + ":" + strings.Join(x.classes[k].Subclasses, ","))
	}
	for _, k := range sortedKeys(x.races) {
		b.WriteString("|" + k + ":" + strings.Join(x.races[k].Subraces, ","))
	}
	for _, k := range sortedKeys(x.spells) {
		b.WriteString("|" + k + ":" + strings.Join(x.spells[k].Classes, ","))
	}
	for _, m := range []int{
		len(x.features), len(x.traits), len(x.subclasses), len(x.subraces), len(x.backgrounds), len(x.effects),
		len(x.casting), len(x.subCasting), len(x.classLevels), len(x.subclassLevels), len(x.namesPT), len(x.namesEN), len(x.spellDetails), len(x.spellEntries),
	} {
		b.WriteString("|")
		b.WriteString(strings.Repeat("#", m%7))
		b.WriteString(string(rune('a' + m%26)))
	}
	b.WriteString(strings.Join(sortedKeys(x.spellDetails), ","))
	return b.String()
}

func TestWithAddsTheTableContent(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	c := withOverlayOn(t, srd, fullOverlay(t, srd))

	if want := srd.Version() + "+mesa.7"; c.Version() != want {
		t.Errorf("version = %q, want %q", c.Version(), want)
	}
	if c.Catalog().ContentVersion != c.Version() {
		t.Errorf("catalog version = %q", c.Catalog().ContentVersion)
	}
	cat := c.Catalog()
	if got, want := len(cat.Classes), len(srd.Catalog().Classes)+len(genKinds); got != want {
		t.Errorf("classes = %d, want %d", got, want)
	}
	// Names: a table entry has no English name, so it uses the Portuguese one.
	for _, key := range []string{"class:gen-none@mesa", "race:anao-das-brumas@mesa", "spell:raio-de-teste@mesa", "feature:gen-none-vigor@mesa", "trait:resistente@mesa"} {
		if c.NamePT(key) == "" || c.c.namesEN[key] != c.NamePT(key) {
			t.Errorf("%s: namePT %q, namesEN %q", key, c.NamePT(key), c.c.namesEN[key])
		}
	}
	// The catalog is sorted by Portuguese name with the table's entries in it.
	if i := slices.IndexFunc(cat.Classes, func(e ClassEntry) bool { return e.Key == "class:gen-none@mesa" }); i < 0 {
		t.Error("the table class is not in the catalog")
	}
	for i := 1; i < len(cat.Classes); i++ {
		if comparePT(cat.Classes[i-1].NamePT, cat.Classes[i].NamePT) > 0 {
			t.Errorf("classes not sorted: %q before %q", cat.Classes[i-1].NamePT, cat.Classes[i].NamePT)
		}
	}
	// An SRD class gains the table's subclass in its own copy of the list.
	fighter := slices.IndexFunc(cat.Classes, func(e ClassEntry) bool { return e.Key == "class:fighter" })
	if got := cat.Classes[fighter].Subclasses; !slices.Contains(got, "subclass:cavaleiro-runico@mesa") || !slices.Contains(got, "subclass:champion") {
		t.Errorf("fighter subclasses = %v", got)
	}
	if got := srd.Catalog().Classes[slices.IndexFunc(srd.Catalog().Classes, func(e ClassEntry) bool { return e.Key == "class:fighter" })].Subclasses; slices.Contains(got, "subclass:cavaleiro-runico@mesa") {
		t.Error("the SRD's fighter got the table's subclass")
	}
	// The race lists its subrace; an SRD class that a third caster joins does
	// not cast on its own.
	race := slices.IndexFunc(cat.Races, func(e RaceEntry) bool { return e.Key == "race:anao-das-brumas@mesa" })
	if got := cat.Races[race].Subraces; !slices.Equal(got, []string{"subrace:da-colina-nevoenta@mesa"}) || !slices.Equal(cat.Races[race].ChoiceBonuses, []int{2, 1}) {
		t.Errorf("race = %+v", cat.Races[race])
	}
	// The class contract: casting numbers, the ASI levels, the subclass level.
	cl := c.c.classes["class:gen-full-prepared@mesa"]
	if cl.HitDie != 8 || cl.SubclassLevel != 3 || len(c.c.classLevels[cl.Key]) != MaxLevel {
		t.Errorf("class = %+v", cl)
	}
	entry := cat.Classes[slices.IndexFunc(cat.Classes, func(e ClassEntry) bool { return e.Key == cl.Key })]
	if entry.SpellcastingAbility != WIS || !entry.PreparesSpells || entry.SpellPreparation != PreparationPrepared || entry.SpellcastingLevel != 1 {
		t.Errorf("class entry = %+v", entry)
	}
	for lvl := 1; lvl <= MaxLevel; lvl++ {
		want := slices.Contains(defaultASILevels, lvl)
		if got := isASILevel(c.c.classLevels[cl.Key][lvl-1]); got != want {
			t.Errorf("level %d: ASI = %v, want %v", lvl, got, want)
		}
	}
	// Prefix branches work for @mesa keys.
	for _, key := range []string{"class:gen-none@mesa", "subclass:cavaleiro-runico@mesa", "feature:gen-none-vigor@mesa", "trait:resistente@mesa", "spell:raio-de-teste@mesa", "background:guarda-de-farol@mesa", "race:anao-das-brumas@mesa", "subrace:da-colina-nevoenta@mesa"} {
		if !c.c.exists(key) || !c.c.effectOwnerExists(key) && !strings.HasPrefix(key, "spell:") {
			t.Errorf("%s does not resolve", key)
		}
	}
}

func TestWithDoesNotChangeTheBase(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	before := fingerprint(t, srd)
	fighter := srd.c.classes["class:fighter"]
	subs := slices.Clone(fighter.Subclasses)
	elf := srd.c.races["race:elf"]
	subraces := slices.Clone(elf.Subraces)
	fireball := srd.c.spellEntries["spell:fireball"]

	o := fullOverlay(t, srd)
	o.Subclasses = append(o.Subclasses, TableSubclass{
		Key: "subclass:duelista" + tableSuffix, NamePT: "Duelista", Class: "class:fighter",
		Levels: []TableSubclassLevel{{Level: 3, Features: []TableFeature{tf("duelista-3", "Duelo")}}},
	})
	o.Subraces = append(o.Subraces, TableSubrace{Key: "subrace:alto-elfo-do-norte" + tableSuffix, NamePT: "Elfo do Norte", Race: "race:elf"})
	c := withOverlayOn(t, srd, o)

	if after := fingerprint(t, srd); after != before {
		t.Error("With changed the base content")
	}
	if srd.c.classes["class:fighter"] != fighter || !slices.Equal(fighter.Subclasses, subs) {
		t.Error("the SRD fighter was written")
	}
	if srd.c.races["race:elf"] != elf || !slices.Equal(elf.Subraces, subraces) {
		t.Error("the SRD elf was written")
	}
	if !reflect.DeepEqual(srd.c.spellEntries["spell:fireball"], fireball) {
		t.Error("the SRD fireball entry changed")
	}
	// The copies carry the table's entries.
	if got := c.c.classes["class:fighter"].Subclasses; !slices.Contains(got, "subclass:duelista@mesa") || !slices.Contains(got, "subclass:champion") {
		t.Errorf("fighter subclasses = %v", got)
	}
	if got := c.c.races["race:elf"].Subraces; !slices.Contains(got, "subrace:alto-elfo-do-norte@mesa") {
		t.Errorf("elf subraces = %v", got)
	}
	// A table class that reuses the sorcerer's list is on every sorcerer spell,
	// in the catalog; the SRD's entry still has its own list.
	var catFireball SpellEntry
	for _, e := range c.Catalog().Spells {
		if e.Key == "spell:fireball" {
			catFireball = e
		}
	}
	if !slices.Contains(catFireball.Classes, "class:gen-full-known@mesa") || slices.Contains(fireball.Classes, "class:gen-full-known@mesa") {
		t.Errorf("fireball classes: table %v, SRD %v", catFireball.Classes, fireball.Classes)
	}
	// A spell the lists did not change shares its details with the base (the
	// bard's cantrip is on no list the table reuses); one that a table class
	// reuses has its own.
	if srd.c.spellDetails["spell:vicious-mockery"] != c.c.spellDetails["spell:vicious-mockery"] {
		t.Error("the details of an unchanged spell are parsed again")
	}
	if srd.c.spellDetails["spell:fireball"] == c.c.spellDetails["spell:fireball"] {
		t.Error("the details of a spell whose list changed are shared")
	}
}

func TestWithOverlaysDoNotSeeEachOther(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	mk := func(slug string) Overlay {
		return Overlay{Revision: 1, Subclasses: []TableSubclass{{
			Key: "subclass:" + slug + tableSuffix, NamePT: slug, Class: "class:fighter",
			Levels: []TableSubclassLevel{{Level: 3, Features: []TableFeature{tf(slug+"-f", "F")}}},
		}}, Spells: []TableSpell{{
			Key: "spell:" + slug + tableSuffix, NamePT: slug, Level: 1, School: "school:evocation",
			CastingTime: TableCastingTime{Unit: CastAction}, Range: TableRange{Kind: RangeSelf}, Duration: TableDuration{Kind: DurationInstantaneous},
			Components: TableComponents{Verbal: true}, Classes: []string{"class:wizard"}, Target: SpellTarget{Kind: TargetSelf},
		}}}
	}
	a, err := srd.With(mk("aaa"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := srd.With(mk("bbb"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		c         *Content
		has, lack string
	}{{a, "aaa", "bbb"}, {b, "bbb", "aaa"}} {
		sub, spell := "subclass:"+tc.has+tableSuffix, "spell:"+tc.has+tableSuffix
		if !tc.c.c.exists(sub) || !tc.c.c.exists(spell) || !slices.Contains(tc.c.c.classes["class:fighter"].Subclasses, sub) {
			t.Errorf("overlay %s lacks its own entries", tc.has)
		}
		if tc.c.c.exists("subclass:"+tc.lack+tableSuffix) || tc.c.c.exists("spell:"+tc.lack+tableSuffix) || slices.Contains(tc.c.c.classes["class:fighter"].Subclasses, "subclass:"+tc.lack+tableSuffix) {
			t.Errorf("overlay %s sees %s", tc.has, tc.lack)
		}
		if !slices.Contains(tc.c.c.spellEntries["spell:"+tc.has+tableSuffix].Classes, "class:wizard") {
			t.Error("the spell is not on the wizard's list")
		}
	}
	// Stacking a table layer on a table layer is refused.
	if _, err := a.With(mk("ccc")); err == nil {
		t.Error("With on a content that has a table layer must fail")
	}
}

// TestWithConcurrently runs With from several goroutines at once on the one
// base, with Derive reading it, under -race.
func TestWithConcurrently(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	want := fingerprint(t, srd)
	o := fullOverlay(t, srd)
	var wg sync.WaitGroup
	results := make([]*Content, 8)
	// Every goroutine is parked before any begins, so the Withs and the Derives overlap.
	var ready sync.WaitGroup
	ready.Add(2 * len(results))
	start := make(chan struct{})
	for i := range results {
		wg.Add(2)
		go func() {
			defer wg.Done()
			ready.Done()
			<-start
			c, err := srd.With(o)
			if err != nil {
				t.Error(err)
				return
			}
			results[i] = c
			Derive(pensantus(), c)
		}()
		go func() {
			defer wg.Done()
			ready.Done()
			<-start
			Derive(pensantus(), srd)
			srd.Catalog()
		}()
	}
	ready.Wait()
	close(start)
	wg.Wait()
	if fingerprint(t, srd) != want {
		t.Error("the base changed")
	}
	for i := 1; i < len(results); i++ {
		if results[i] == nil || results[0] == nil {
			continue
		}
		if fingerprint(t, results[i]) != fingerprint(t, results[0]) {
			t.Errorf("overlay %d differs from overlay 0", i)
		}
	}
}

// TestWithDoesNotWriteTheCallersEffects: compiling an effect writes its
// formulas in the Effect; With does it on copies.
func TestWithDoesNotWriteTheCallersEffects(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	o := genOverlay(t, srd)
	snapshot, _ := json.Marshal(o)
	if _, err := srd.With(o); err != nil {
		t.Fatal(err)
	}
	if again, _ := json.Marshal(o); !bytes.Equal(snapshot, again) {
		t.Error("With changed the overlay")
	}
	if e := &o.Classes[0].Levels[0].Features[0].Effects[0]; e.value != nil {
		t.Error("an effect of the caller was compiled in place")
	}
}

// TestDeriveTableCharacterGolden pins the whole Derived of a table-class
// character at levels 1, 5 and 11: a table race and subrace, a table
// background, the table's full caster that prepares, one of its subclasses with
// always-prepared spells. The sheet has no Issue at any level. Rewrite the file
// with `go test ./internal/rules -run TableCharacterGolden -update` and review
// the diff.
func TestDeriveTableCharacterGolden(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	o := fullOverlay(t, srd)
	o.Spells[4].Classes = []string{"class:gen-full-prepared@mesa"} // the table's cantrip is on the class's own list
	c := withOverlayOn(t, srd, o)
	classKey, subKey := "class:gen-full-prepared@mesa", "subclass:gen-full-prepared-b@mesa"
	b := sweepBase(t, c, classKey, subKey)
	b.Race, b.Subrace, b.Background = "race:anao-das-brumas@mesa", "subrace:da-colina-nevoenta@mesa", "background:guarda-de-farol@mesa"
	b.ExtraAbilityBonuses = map[Ability]int{WIS: 2, CON: 1}
	// Table spells on the sheet: its own-list spell and the cantrip (the list of
	// the class is the cleric's plus the spells that name it).
	b.SpellsPrepared = nil
	b = sweepUp(t, c, b, classKey, subKey, 5)
	// Two spells of the table on the sheet: the cantrip, and the cone (a prepared
	// spell of the class's own list).
	if !slices.Contains(b.Cantrips, "spell:faisca-de-teste@mesa") {
		b.Cantrips[len(b.Cantrips)-1] = "spell:faisca-de-teste@mesa"
	}
	if !slices.Contains(b.SpellsPrepared, "spell:cone-de-teste@mesa") {
		b.SpellsPrepared[len(b.SpellsPrepared)-1] = "spell:cone-de-teste@mesa"
	}

	out := map[string]Derived{}
	snap := func(name string, b Build) {
		d := Derive(b, c)
		for i := range d.Features {
			d.Features[i].Description = nil
		}
		if len(d.Issues) != 0 {
			t.Errorf("%s: issues %v", name, d.Issues)
		}
		out[name] = d
	}
	level1 := b
	level1.Classes = []ClassLevel{{Class: classKey, Level: 1}}
	level1.Cantrips, level1.SpellsPrepared, level1.FeatureChoices, level1.Expertise = nil, nil, nil, nil
	level1.SkillProficiencies = level1.SkillProficiencies[:2]
	level1 = fillSpells(c, level1, classKey, 0)
	snap("level 1", level1)
	snap("level 5", b)
	b = sweepUp(t, c, b, classKey, subKey, 11)
	snap("level 11", b)

	got, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	path := filepath.Join("testdata", "golden", "table-character.json")
	if *update {
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update to create it)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("Derive differs from %s; if the change is right, run with -update and review the diff", path)
	}
	// What the golden shows, said in words, so a wrong golden cannot hide it.
	d := out["level 11"]
	if d.RaceNamePT != "Anão das Brumas" || d.SubraceNamePT != "Da Colina Nevoenta" || d.BackgroundNamePT != "Guarda de farol" {
		t.Errorf("names: %q %q %q", d.RaceNamePT, d.SubraceNamePT, d.BackgroundNamePT)
	}
	if d.ProficiencyBonus != 4 || d.AttacksPerAction != 2 || d.SpeedWalkFt != 25 {
		t.Errorf("proficiency %d, attacks %d, speed %d", d.ProficiencyBonus, d.AttacksPerAction, d.SpeedWalkFt)
	}
	if !slices.ContainsFunc(d.Senses, func(s Sense) bool { return s.Key == "darkvision" && s.RangeFt == 60 }) {
		t.Errorf("senses = %+v", d.Senses)
	}
	for _, key := range []string{"spell:bless", "spell:fireball", "spell:cone-de-teste@mesa", "spell:faisca-de-teste@mesa"} {
		if !slices.ContainsFunc(d.Spells, func(s CharacterSpell) bool { return s.Spell.Key == key && s.Prepared }) {
			t.Errorf("%s is missing from %+v", key, d.Spells)
		}
	}
	if len(d.Attacks) == 0 || !slices.ContainsFunc(d.Attacks, func(a Attack) bool { return a.Key == "spell:faisca-de-teste@mesa" && a.Damage != "" }) {
		t.Errorf("the table's attack cantrip is not an attack of the sheet: %+v", d.Attacks)
	}
}

// TestSpellDetailsOfTableSpells: a table spell has the same SpellDetails as an
// SRD one, so the combat that resolves the SRD's resolves it, plus the target.
func TestSpellDetailsOfTableSpells(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, fullOverlay(t, loadForTest(t)))
	det := func(slug string) *SpellDetails {
		d, ok := c.SpellDetails("spell:" + slug + tableSuffix)
		if !ok {
			t.Fatalf("no details for %s", slug)
		}
		return d
	}
	roll := func(d *SpellDetails, slot, char int) string {
		var parts []string
		for _, r := range d.DamageAt(slot, char) {
			parts = append(parts, r.Raw+" "+r.Type)
		}
		return strings.Join(parts, "+")
	}

	bolt := det("raio-de-teste")
	if bolt.AttackType != "ranged" || bolt.Save != nil || bolt.Range.Kind != RangeRanged || bolt.Range.DistanceFt != 60 || bolt.CastingTime.Unit != CastAction {
		t.Errorf("bolt = %+v", bolt)
	}
	if roll(bolt, 1, 1) != "3d6 damage-type:force" || roll(bolt, 4, 1) != "6d6 damage-type:force" {
		t.Errorf("bolt damage: %s, %s", roll(bolt, 1, 1), roll(bolt, 4, 1))
	}
	if bolt.Target.Kind != TargetCreature || bolt.Target.IsArea() || bolt.Target.MaxTargets(1, 1) != 1 {
		t.Errorf("bolt target = %+v", bolt.Target)
	}

	cone := det("cone-de-teste")
	if cone.Save == nil || cone.Save.Ability != DEX || cone.Save.OnSuccess != "half" || cone.Range.Kind != RangeSelf {
		t.Errorf("cone = %+v", cone)
	}
	if !cone.Target.IsArea() || cone.Target.Shape != ShapeCone || cone.Target.SizeFt != 15 || cone.Target.MaxTargets(3, 3) != 0 {
		t.Errorf("cone target = %+v", cone.Target)
	}
	if roll(cone, 3, 5) != "6d6 damage-type:cold" || roll(cone, 5, 5) != "8d6 damage-type:cold" {
		t.Errorf("cone damage: %s, %s", roll(cone, 3, 5), roll(cone, 5, 5))
	}

	twin := det("par-de-teste")
	if got := []int{twin.Target.MaxTargets(2, 2), twin.Target.MaxTargets(3, 2), twin.Target.MaxTargets(5, 2)}; !slices.Equal(got, []int{2, 3, 5}) {
		t.Errorf("twin targets by slot = %v", got)
	}
	if !twin.Duration.Concentration || twin.Duration.Kind != DurationTimed || !twin.Duration.UpTo || twin.Duration.Amount != 1 || twin.Duration.Unit != DurationMinute {
		t.Errorf("twin duration = %+v", twin.Duration)
	}

	heal := det("cura-de-teste")
	if r, ok := heal.HealAt(1); !ok || r.Raw != "2d8 + MOD" || !r.Dice.AddsModifier {
		t.Errorf("heal at 1 = %+v %v", r, ok)
	}
	if r, _ := heal.HealAt(3); r.Raw != "4d8 + MOD" {
		t.Errorf("heal at 3 = %+v", r)
	}

	cantrip := det("faisca-de-teste")
	for lvl, want := range map[int]string{1: "1d8", 4: "1d8", 5: "2d8", 11: "3d8", 17: "4d8", 20: "4d8"} {
		if got := roll(cantrip, 0, lvl); got != want+" damage-type:lightning" {
			t.Errorf("cantrip at %d = %s, want %s", lvl, got, want)
		}
	}
	if cantrip.Spell.Level != 0 {
		t.Errorf("cantrip level = %d", cantrip.Spell.Level)
	}
	// An SRD spell's target is worked out from the database's structured area.
	srdDet, _ := c.SpellDetails("spell:fireball")
	if srdDet.Target != (SpellTarget{Kind: TargetArea, Shape: ShapeSphere, SizeFt: 20}) {
		t.Errorf("fireball target = %+v", srdDet.Target)
	}
}

// TestTableSpellsOnTheLists: a table spell names its classes, a table class may
// reuse an SRD list, and Derive, the catalog and the level-up all ask the one
// helper.
func TestTableSpellsOnTheLists(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, fullOverlay(t, loadForTest(t)))
	cone := c.c.spells["spell:cone-de-teste@mesa"]
	for class, want := range map[string]bool{
		"class:wizard": true, "class:gen-full-prepared@mesa": true, "class:cleric": false,
		"class:gen-full-known@mesa": false, // reuses the sorcerer's list; the cone is not on it
	} {
		if got := c.c.onList(cone, class); got != want {
			t.Errorf("cone on the list of %s = %v, want %v", class, got, want)
		}
	}
	fireball := c.c.spells["spell:fireball"]
	if !c.c.onList(fireball, "class:gen-full-known@mesa") || c.c.onList(fireball, "class:gen-none@mesa") {
		t.Error("the SRD list reuse is wrong")
	}
	// The wizard's cantrip choice of a table class's sheet is not an Issue, and a
	// spell off the list is one.
	classKey := "class:gen-full-prepared@mesa"
	b := sweepBase(t, c, classKey, "")
	b.Cantrips = slices.Concat(b.Cantrips[:len(b.Cantrips)-1], []string{"spell:faisca-de-teste@mesa"})
	if d := Derive(b, c); !hasIssue(d, IssueSpellNotOnList) {
		t.Error("a cantrip off the class's list should be an Issue")
	}
	// The table's cantrip is on the list of a class that names it.
	tbl := withOverlay(t, func() Overlay {
		o := fullOverlay(t, loadForTest(t))
		o.Spells[4].Classes = []string{classKey}
		return o
	}())
	b = sweepBase(t, tbl, classKey, "")
	b.Cantrips = slices.Concat(b.Cantrips[:len(b.Cantrips)-1], []string{"spell:faisca-de-teste@mesa"})
	if d := Derive(b, tbl); len(d.Issues) != 0 {
		t.Errorf("issues = %v", d.Issues)
	}
}

func hasIssue(d Derived, code string) bool {
	return slices.ContainsFunc(d.Issues, func(i Issue) bool { return i.Code == code })
}

// TestThirdCasterSlots: a third caster's table is its subclass's, and the
// multiclass rule counts a third of its levels (full: the level, half: half,
// rounded down).
func TestThirdCasterSlots(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	c := withOverlay(t, genOverlay(t, srd))
	scores := map[Ability]int{STR: 14, DEX: 14, CON: 14, INT: 16, WIS: 14, CHA: 14}
	mk := func(classes ...ClassLevel) Build {
		return Build{BaseScores: scores, Race: "race:human", Background: "background:acolyte", Languages: testLanguages, Classes: classes}
	}
	slots := func(b Build) []int { return Derive(b, c).SpellSlots }
	rowSlots := func(class string, lvl int) []int {
		s := srd.c.classLevels[class][lvl-1].Spellcasting.Slots
		return s[:]
	}
	fighter := func(lvl int) ClassLevel {
		return ClassLevel{Class: "class:fighter", Subclass: "subclass:cavaleiro-runico@mesa", Level: lvl}
	}

	// Alone: the subclass's own table (levels 3, 4 and 7 have 2, 3 and 4 first-level slots).
	for lvl, want := range map[int]int{3: 2, 4: 3, 7: 4} {
		if got := slots(mk(fighter(lvl)))[0]; got != want {
			t.Errorf("third caster alone at %d: %d first-level slots, want %d", lvl, got, want)
		}
	}
	if got := slots(mk(fighter(2))); !slices.Equal(got, make([]int, 9)) {
		t.Errorf("before it casts: %v", got)
	}
	// With a wizard 6: 3 + 6/1 -> caster level 6 + 9/3 = 9, the wizard's row 9.
	if got, want := slots(mk(fighter(9), ClassLevel{Class: "class:wizard", Level: 6})), rowSlots("class:wizard", 9); !slices.Equal(got, want) {
		t.Errorf("fighter 9 + wizard 6 = %v, want wizard 9: %v", got, want)
	}
	// Rounded down: 8/3 = 2.
	if got, want := slots(mk(fighter(8), ClassLevel{Class: "class:wizard", Level: 2})), rowSlots("class:wizard", 4); !slices.Equal(got, want) {
		t.Errorf("fighter 8 + wizard 2 = %v, want wizard 4: %v", got, want)
	}
	// With a table full caster (its own table is not the multiclass one) and a
	// half caster of the table: 5 + 4/2 + 6/3 = 9 -> the SRD's full table.
	got := slots(mk(
		ClassLevel{Class: "class:gen-full-known@mesa", Level: 5},
		ClassLevel{Class: "class:gen-half-prepared@mesa", Level: 4},
		fighter(6)))
	if want := rowSlots("class:wizard", 5+2+2); !slices.Equal(got, want) {
		t.Errorf("table full 5 + table half 4 + third 6 = %v, want %v", got, want)
	}
	// Pact magic stays apart.
	d := Derive(mk(ClassLevel{Class: "class:gen-pact-known@mesa", Level: 5}, fighter(6)), c)
	if d.PactMagic == nil || d.PactMagic.SlotLevel != 3 || d.PactMagic.Slots != 2 {
		t.Errorf("pact = %+v", d.PactMagic)
	}
	// The slots are the third caster's own table (the pact slots are apart): at
	// level 6 it has the first-level slots of caster level 2.
	if want := rowSlots("class:wizard", 2); !slices.Equal(d.SpellSlots, want) {
		t.Errorf("slots with pact magic = %v, want %v", d.SpellSlots, want)
	}
	// The third caster's numbers: save DC and ability come from its casting.
	dd := Derive(mk(fighter(7)), c)
	if len(dd.Spellcasting) != 1 || dd.Spellcasting[0].Ability != INT || dd.Spellcasting[0].CantripsKnown != 2 || dd.Spellcasting[0].SpellsKnownMax != 5 || dd.Spellcasting[0].MaxSpellLevel != 2 {
		t.Errorf("third caster at 7 = %+v", dd.Spellcasting)
	}
}

// TestAlwaysPreparedDoNotCount: a subclass's always-prepared spells are on the
// sheet, prepared, and do not count against the prepared limit.
func TestAlwaysPreparedDoNotCount(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, genOverlay(t, loadForTest(t)))
	classKey, subKey := "class:gen-full-prepared@mesa", "subclass:gen-full-prepared-b@mesa"
	b := sweepBase(t, c, classKey, subKey)
	b = sweepUp(t, c, b, classKey, subKey, 5)
	d := Derive(b, c)
	limit := d.Spellcasting[0].PreparedMax
	// Prepare the maximum from the cleric's list, plus the always-prepared ones.
	b.SpellsPrepared = pickSpells(c, classKey, 1, d.Spellcasting[0].MaxSpellLevel, nil, limit, false)
	d = Derive(b, c)
	if len(d.Issues) != 0 {
		t.Fatalf("issues at the limit: %v", d.Issues)
	}
	for _, key := range []string{"spell:bless", "spell:fireball"} {
		if !slices.ContainsFunc(d.Spells, func(s CharacterSpell) bool { return s.Spell.Key == key && s.Prepared }) {
			t.Errorf("%s is not prepared on the sheet", key)
		}
	}
	// Listing them explicitly does not count either.
	b.SpellsPrepared = append(slices.Clone(b.SpellsPrepared), "spell:bless")
	if d := Derive(b, c); hasIssue(d, IssueSpellCount) {
		t.Errorf("an always-prepared spell counted against the limit: %v", d.Issues)
	}
	// One more of the class's own does.
	b.SpellsPrepared = append(b.SpellsPrepared, pickSpells(c, classKey, 1, 2, b.SpellsPrepared, 1, false)[len(b.SpellsPrepared):]...)
	if d := Derive(b, c); !hasIssue(d, IssueSpellCount) {
		t.Error("one prepared spell over the limit should be an Issue")
	}
}

// TestTableFeaturesAreDerived: the effects of a table class come out of Derive
// like the SRD's.
func TestTableFeaturesAreDerived(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, genOverlay(t, loadForTest(t)))
	classKey, subKey := "class:gen-none@mesa", "subclass:gen-none-a@mesa"
	b := sweepBase(t, c, classKey, subKey)
	b = sweepUp(t, c, b, classKey, subKey, 12)
	d := Derive(b, c)
	if len(d.Issues) != 0 {
		t.Fatalf("issues: %v", d.Issues)
	}
	has := func(key string) bool { return hasFeature(d, key) }
	for _, key := range []string{"feature:gen-none-vigor@mesa", "feature:gen-none-ataque@mesa", "feature:gen-none-marca@mesa", "feature:class-gen-none-ability-score-improvement-2@mesa", "feature:gen-none-a3@mesa"} {
		if !has(key) {
			t.Errorf("feature %s is missing", key)
		}
	}
	if has("feature:gen-none-lingua@mesa") {
		t.Error("a feature of level 14 is on a level 12 sheet")
	}
	if !has("feature:gen-none-a9@mesa") {
		t.Error("the subclass feature of level 9 is missing")
	}
	if d.AttacksPerAction != 2 {
		t.Errorf("attacks = %d", d.AttacksPerAction)
	}
	if len(d.Resources) != 1 || d.Resources[0].Key != "gen_none_surto" || d.Resources[0].Max != 4 || d.Resources[0].Recharge != RechargeShortRest {
		t.Errorf("resources = %+v", d.Resources)
	}
	if !slices.ContainsFunc(d.Actions, func(a Action) bool {
		return a.Source == "feature:gen-none-surto@mesa" && a.Economy == EconomyBonusAction && a.Resource == "gen_none_surto"
	}) {
		t.Errorf("actions = %+v", d.Actions)
	}
	if !slices.ContainsFunc(d.Senses, func(s Sense) bool { return s.Key == "darkvision" && s.Source == "feature:gen-none-olhos@mesa" }) {
		t.Errorf("senses = %+v", d.Senses)
	}
	if h, ok := hintFrom(d, "feature:gen-none-marca@mesa"); !ok || h.TextPT != "Bônus de marca +4." {
		t.Errorf("note hint = %+v", h)
	}
	if h, ok := hintFrom(d, "feature:gen-none-olhos@mesa"); !ok || h.Mode != "advantage" {
		t.Errorf("roll mode hint = %+v", h)
	}
	if skillOf(d, "skill:survival").Proficiency != ProficiencyFull {
		t.Error("the proficiency effect did not apply")
	}
	// The fighting-style option a table feature offers is applied, with the
	// feature as its parent.
	if !slices.ContainsFunc(d.Features, func(f Feature) bool {
		return f.Key == "feature:fighter-fighting-style-defense" && f.Source == "feature:gen-none-estilo@mesa"
	}) && !slices.ContainsFunc(d.Features, func(f Feature) bool {
		return f.Key == "feature:fighter-fighting-style-dueling" && f.Source == "feature:gen-none-estilo@mesa"
	}) {
		t.Errorf("no fighting style from the table feature: %+v", d.Features)
	}
	// Without the table feature, the option is not valid for the character.
	b.Classes[0].Level = 5
	b.HitPoints = HitPoints{}
	if d := Derive(b, c); !hasIssue(d, IssueUnknownKey) {
		t.Error("an option whose offering feature is not owned should be an Issue")
	}
}

// TestRaceBonusesToPlace: the master's "+2 and +1 to your choice": the sheet
// says so until the manual bonuses cover it.
func TestRaceBonusesToPlace(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, fullOverlay(t, loadForTest(t)))
	b := Build{
		BaseScores: map[Ability]int{STR: 10, DEX: 10, CON: 10, INT: 10, WIS: 10, CHA: 10},
		Race:       "race:anao-das-brumas@mesa", Subrace: "subrace:da-colina-nevoenta@mesa", Background: "background:guarda-de-farol@mesa",
		Classes: []ClassLevel{{Class: "class:fighter", Level: 1}},
	}
	b.SkillProficiencies = []string{"skill:athletics", "skill:acrobatics"}
	b.FeatureChoices = []string{"feature:fighter-fighting-style-defense"}
	b.Languages = testLanguages
	if err := Validate(b, c); err != nil {
		t.Fatal(err)
	}
	if d := Derive(b, c); !hasIssue(d, IssueRaceBonus) {
		t.Errorf("an unplaced +2/+1 should be an Issue: %v", d.Issues)
	}
	b.ExtraAbilityBonuses = map[Ability]int{STR: 1, DEX: 1} // +1 and +1 do not cover +2 and +1
	if d := Derive(b, c); !hasIssue(d, IssueRaceBonus) {
		t.Error("+1 +1 does not cover +2 +1")
	}
	b.ExtraAbilityBonuses = map[Ability]int{STR: 2, DEX: 1}
	d := Derive(b, c)
	if hasIssue(d, IssueRaceBonus) || len(d.Issues) != 0 {
		t.Errorf("placed bonuses: %v", d.Issues)
	}
	// Race (CON +1), subrace (WIS +1), the placed +2 and +1.
	got := map[Ability]int{}
	for _, s := range d.Abilities {
		got[s.Ability] = s.Score
	}
	if got[STR] != 12 || got[DEX] != 11 || got[CON] != 11 || got[WIS] != 11 {
		t.Errorf("scores = %v", got)
	}
	// The race's Common, and the three languages the sheet typed in the free field.
	if d.SpeedWalkFt != 25 || len(d.Languages) != 4 {
		t.Errorf("speed %d, languages %v", d.SpeedWalkFt, d.Languages)
	}
	if !slices.ContainsFunc(d.Senses, func(s Sense) bool {
		return s.Key == "darkvision" && s.RangeFt == 60 && s.Source == "race:anao-das-brumas@mesa"
	}) {
		t.Errorf("senses = %+v", d.Senses)
	}
	if skillOf(d, "skill:perception").Proficiency != ProficiencyFull {
		t.Error("the subrace trait's proficiency did not apply")
	}
	if !slices.ContainsFunc(d.Proficiencies, func(p Proficiency) bool { return p.Key == "proficiency:thieves-tools" }) {
		t.Errorf("proficiencies = %+v", d.Proficiencies)
	}
}

// TestArchivedEntriesStillResolve: an archived entry works everywhere on the
// sheets that have it, the catalog marks it, and the helper finds the archived
// keys of a sheet.
func TestArchivedEntriesStillResolve(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	o := fullOverlay(t, srd)
	for i := range o.Classes {
		if o.Classes[i].Key == "class:gen-full-prepared@mesa" {
			o.Classes[i].Archived = true
		}
	}
	o.Races[0].Archived = true
	o.Spells[4].Archived = true
	o.Backgrounds[0].Archived = true
	c := withOverlay(t, o)

	classKey, subKey := "class:gen-full-prepared@mesa", "subclass:gen-full-prepared-a@mesa"
	b := sweepBase(t, c, classKey, subKey)
	b.Race, b.Subrace, b.Background = "race:anao-das-brumas@mesa", "subrace:da-colina-nevoenta@mesa", "background:guarda-de-farol@mesa"
	b.ExtraAbilityBonuses = map[Ability]int{STR: 2, DEX: 1}
	b.Cantrips = slices.Concat(b.Cantrips[:len(b.Cantrips)-1], []string{"spell:faisca-de-teste@mesa"})
	if err := Validate(b, c); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	d := Derive(b, c)
	if d.RaceNamePT != "Anão das Brumas" || d.BackgroundNamePT != "Guarda de farol" || d.Classes[0].NamePT != "Classe full-prepared" {
		t.Errorf("names: %+v", d)
	}
	if got := c.NamePT("spell:faisca-de-teste@mesa"); got != "Faísca de teste" {
		t.Errorf("spell name = %q", got)
	}
	if !slices.ContainsFunc(d.Spells, func(s CharacterSpell) bool { return s.Spell.Key == "spell:faisca-de-teste@mesa" && s.Spell.Archived }) {
		t.Errorf("sheet spells = %+v", d.Spells)
	}
	if len(d.Issues) != 0 {
		// The cantrip is on no class's list in this overlay: that is an Issue of the
		// sheet, not of the archive. Everything else resolves.
		for _, is := range d.Issues {
			if is.Code != IssueSpellNotOnList {
				t.Errorf("issue %+v", is)
			}
		}
	}
	// The level-up still works for the sheet.
	if _, err := LevelUpOptions(b, classKey, c); err != nil {
		t.Errorf("LevelUpOptions: %v", err)
	}
	if got, want := c.ArchivedKeys(b), []string{"background:guarda-de-farol@mesa", classKey, "race:anao-das-brumas@mesa", "spell:faisca-de-teste@mesa"}; !slices.Equal(got, want) {
		t.Errorf("ArchivedKeys = %v, want %v", got, want)
	}
	if got := TableKeys(b); len(got) != 5 || !IsTableKey(got[0]) {
		t.Errorf("TableKeys = %v", got)
	}
	if len(srd.ArchivedKeys(b)) != 0 {
		t.Error("the SRD has no archived keys")
	}
	cat := c.Catalog()
	marked := map[string]bool{}
	for _, e := range cat.Classes {
		marked[e.Key] = e.Archived
	}
	for _, e := range cat.Races {
		marked[e.Key] = e.Archived
	}
	for _, e := range cat.Spells {
		marked[e.Key] = e.Archived
	}
	for _, e := range cat.Backgrounds {
		marked[e.Key] = e.Archived
	}
	if !marked[classKey] || !marked["race:anao-das-brumas@mesa"] || !marked["spell:faisca-de-teste@mesa"] || !marked["background:guarda-de-farol@mesa"] || marked["class:gen-none@mesa"] || marked["class:fighter"] {
		t.Errorf("archived marks: %v", marked)
	}
	if !c.Archived(classKey) || c.Archived("class:gen-none@mesa") || srd.Archived(classKey) {
		t.Error("Archived is wrong")
	}
}

// TestMissingTableKeysNeverPanic: a sheet that references an @mesa key the
// content lacks (the table was changed, or the sheet moved) opens with Issues.
func TestMissingTableKeysNeverPanic(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	for name, c := range map[string]*Content{"srd": srd, "table": withOverlayOn(t, srd, genOverlay(t, srd))} {
		b := Build{
			BaseScores: map[Ability]int{STR: 10, DEX: 10, CON: 10, INT: 10, WIS: 10, CHA: 10},
			Race:       "race:fantasma@mesa", Subrace: "subrace:fantasma@mesa", Background: "background:fantasma@mesa",
			Classes:            []ClassLevel{{Class: "class:fantasma@mesa", Subclass: "subclass:fantasma@mesa", Level: 3}, {Class: "class:fighter", Subclass: "subclass:fantasma@mesa", Level: 3}},
			Cantrips:           []string{"spell:fantasma@mesa"},
			SpellsKnown:        []string{"spell:fantasma@mesa"},
			SpellsPrepared:     []string{"spell:fantasma@mesa"},
			FeatureChoices:     []string{"feature:fantasma@mesa", "trait:fantasma@mesa"},
			SkillProficiencies: []string{"skill:fantasma@mesa"},
		}
		d := Derive(b, c)
		if !hasIssue(d, IssueUnknownKey) {
			t.Errorf("%s: no Issue for the missing keys: %v", name, d.Issues)
		}
		if err := Validate(b, c); err == nil {
			t.Errorf("%s: Validate accepted missing keys", name)
		}
		if s := c.Summary(b); s.TotalLevel != 3 {
			t.Errorf("%s: summary = %+v", name, s)
		}
		if _, err := LevelUpOptions(b, "class:fantasma@mesa", c); err == nil {
			t.Errorf("%s: LevelUpOptions of a missing class should fail", name)
		}
		if name == "table" {
			// A class that exists, with a subclass and a table class's subclass that
			// belongs to another class.
			b2 := Build{
				BaseScores: b.BaseScores, Race: "race:human", Background: "background:acolyte", Languages: testLanguages,
				Classes: []ClassLevel{{Class: "class:gen-none@mesa", Subclass: "subclass:gen-half-prepared-a@mesa", Level: 4}},
			}
			if d := Derive(b2, c); !hasIssue(d, IssueUnknownKey) {
				t.Errorf("a subclass of another class: %v", d.Issues)
			}
		}
	}
}

// refusal is a rule of With: edit breaks a valid overlay, and With must refuse
// with an *OverlayError that has want in its message (the key or the limit).
type refusal struct {
	name string
	edit func(o *Overlay)
	want []string
}

func TestWithRefusals(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	good := func() Overlay { return fullOverlay(t, srd) }
	if _, err := srd.With(good()); err != nil {
		t.Fatalf("the base overlay is not valid: %v", err)
	}
	classIdx := func(o *Overlay, kind string) *TableClass {
		for i := range o.Classes {
			if o.Classes[i].Key == "class:gen-"+kind+tableSuffix {
				return &o.Classes[i]
			}
		}
		t.Fatalf("no class %s", kind)
		return nil
	}
	feature := func(o *Overlay) *TableFeature { return &classIdx(o, "none").Levels[0].Features[0] }
	cases := []refusal{
		{"key without @mesa", func(o *Overlay) { o.Classes[0].Key = "class:sem-sufixo" }, []string{"class:sem-sufixo", "@mesa"}},
		{"key with the SRD's shape", func(o *Overlay) { o.Spells[0].Key = "spell:fireball" }, []string{"spell:fireball"}},
		{"wrong prefix", func(o *Overlay) { o.Races[0].Key = "class:racas@mesa" }, []string{"class:racas@mesa", `"race:"`}},
		{"uppercase slug", func(o *Overlay) { o.Spells[0].Key = "spell:Raio@mesa" }, []string{"spell:Raio@mesa", "a-z"}},
		{"accent in slug", func(o *Overlay) { o.Spells[0].Key = "spell:raio-ágil@mesa" }, []string{"a-z"}},
		{"empty slug", func(o *Overlay) { o.Spells[0].Key = "spell:@mesa" }, []string{"spell:@mesa", "1 to 60"}},
		{"slug of 61", func(o *Overlay) { o.Spells[0].Key = "spell:" + strings.Repeat("a", 61) + "@mesa" }, []string{"1 to 60"}},
		{"colliding entries", func(o *Overlay) { o.Spells[1].Key = o.Spells[0].Key }, []string{"spell:raio-de-teste@mesa", "already exists"}},
		{"entry colliding with a feature", func(o *Overlay) {
			o.Races[0].Traits[0].Key = "trait:resistente@mesa"
			o.Subraces[0].Traits[0].Key = "trait:resistente@mesa"
		}, []string{"trait:resistente@mesa", "already exists"}},
		{"feature colliding across classes", func(o *Overlay) { classIdx(o, "full-known").Levels[0].Features[0].Key = feature(o).Key }, []string{"feature:gen-none-vigor@mesa", "already exists"}},
		{"feature key without @mesa", func(o *Overlay) { feature(o).Key = "feature:vigor" }, []string{"feature:vigor"}},
		{"feature key reserved for ASI", func(o *Overlay) { feature(o).Key = "feature:x-ability-score-improvement-9@mesa" }, []string{"reserved"}},
		{"engine key collision", func(o *Overlay) { feature(o).Key = "feature:gen-none-ability-score-improvement-1@mesa" }, []string{"reserved"}},
		{"handler effect", func(o *Overlay) {
			feature(o).Effects = append(feature(o).Effects, Effect{Type: "handler", Handler: "monk.martial_arts"})
		}, []string{"feature:gen-none-vigor@mesa", "handler"}},
		{"spellcasting effect", func(o *Overlay) {
			feature(o).Effects = append(feature(o).Effects, Effect{Type: "spellcasting", Ability: "int", Progression: "full"})
		}, []string{"feature:gen-none-vigor@mesa", "menu"}},
		{"beast spells effect", func(o *Overlay) {
			feature(o).Effects = append(feature(o).Effects, Effect{Type: "beast_spells"})
		}, []string{"menu"}},
		{"unknown effect type", func(o *Overlay) { feature(o).Effects = []Effect{{Type: "teleport"}} }, []string{"menu"}},
		{"choice from the table's own set", func(o *Overlay) {
			feature(o).Effects = []Effect{{Type: "choice", Choice: "skill", Count: 1, From: []string{"feature:gen-none-vigor@mesa"}}}
		}, []string{"not in the SRD"}},
		{"choice from a key that does not exist", func(o *Overlay) {
			feature(o).Effects = []Effect{{Type: "choice", Choice: "skill", Count: 1, From: []string{"skill:cooking"}}}
		}, []string{"not in the SRD"}},
		{"feature choice of something that is no option", func(o *Overlay) {
			feature(o).Effects = []Effect{{Type: "choice", Choice: "feature", Count: 1, From: []string{"feature:arcane-recovery"}}}
		}, []string{"not an option"}},
		{"choice of a subclass", func(o *Overlay) {
			feature(o).Effects = []Effect{{Type: "choice", Choice: "subclass", Count: 1}}
		}, []string{"menu"}},
		{"choice of nothing", func(o *Overlay) { feature(o).Effects = []Effect{{Type: "choice", Choice: "skill"}} }, []string{"count"}},
		{"SRD resource name", func(o *Overlay) {
			feature(o).Effects = []Effect{{Type: "resource", Resource: "rage", Max: "1", Recharge: "long_rest"}}
		}, []string{"SRD resource"}},
		{"bad resource name", func(o *Overlay) {
			feature(o).Effects = []Effect{{Type: "resource", Resource: "Fúria", Max: "1", Recharge: "long_rest"}}
		}, []string{"resource name"}},
		{"bad formula", func(o *Overlay) {
			feature(o).Effects = []Effect{{Type: "modifier", Target: "ac", Mode: "add", Value: "rand()"}}
		}, []string{"feature:gen-none-vigor@mesa", "formula"}},
		{"formula with a class that does not exist", func(o *Overlay) {
			feature(o).Effects = []Effect{{Type: "modifier", Target: "ac", Mode: "add", Value: `classLevel("fantasma@mesa")`}}
		}, []string{"formula"}},
		{"unknown modifier target", func(o *Overlay) {
			feature(o).Effects = []Effect{{Type: "modifier", Target: "hp.regen", Mode: "add", Value: "1"}}
		}, []string{"target"}},
		{"too many effects", func(o *Overlay) {
			feature(o).Effects = slices.Repeat([]Effect{{Type: "note"}}, 21)
		}, []string{"effects", "limit"}},
		{"dangling subclass parent", func(o *Overlay) { o.Subclasses[0].Class = "class:fantasma@mesa" }, []string{"subclass:gen-none-a@mesa", "does not exist"}},
		{"dangling spell list of a class", func(o *Overlay) { classIdx(o, "full-known").Casting.ListFrom = "class:fantasma" }, []string{"class:gen-full-known@mesa", "spell list", "does not exist"}},
		{"dangling class of a spell", func(o *Overlay) { o.Spells[0].Classes = []string{"class:fantasma@mesa"} }, []string{"spell:raio-de-teste@mesa", "does not exist"}},
		{"dangling race of a subrace", func(o *Overlay) { o.Subraces[0].Race = "race:fantasma@mesa" }, []string{"subrace:da-colina-nevoenta@mesa", "does not exist"}},
		{"dangling always-prepared spell", func(o *Overlay) {
			for i := range o.Subclasses {
				if len(o.Subclasses[i].AlwaysPrepared) > 0 {
					o.Subclasses[i].AlwaysPrepared[0].Spell = "spell:fantasma@mesa"
				}
			}
		}, []string{"spell:fantasma@mesa", "does not exist"}},
		{"a list that chains", func(o *Overlay) { classIdx(o, "full-known").Casting.ListFrom = "class:gen-full-prepared" + tableSuffix }, []string{"reuses another list"}},
		{"a class reusing its own list", func(o *Overlay) { classIdx(o, "full-known").Casting.ListFrom = "class:gen-full-known" + tableSuffix }, []string{"its own list"}},
		{"a list of a class that does not cast", func(o *Overlay) { classIdx(o, "full-known").Casting.ListFrom = "class:fighter" }, []string{"no spell list"}},
		{"a list on a class that does not cast", func(o *Overlay) { classIdx(o, "none").Casting.ListFrom = "class:wizard" }, []string{"does not cast"}},
		{"19 level rows", func(o *Overlay) { c := classIdx(o, "none"); c.Levels = c.Levels[:19] }, []string{"class:gen-none@mesa", "20 rows", "19"}},
		{"21 level rows", func(o *Overlay) { c := classIdx(o, "none"); c.Levels = append(c.Levels, TableClassLevel{}) }, []string{"20 rows", "21"}},
		{"no level rows", func(o *Overlay) { classIdx(o, "none").Levels = nil }, []string{"20 rows"}},
		{"1001 entries", func(o *Overlay) {
			for len(o.Spells)+len(o.Classes)+len(o.Subclasses)+len(o.Races)+len(o.Subraces)+len(o.Backgrounds) <= MaxOverlayEntries {
				o.Spells = append(o.Spells, o.Spells[0])
			}
		}, []string{"1001", "1000"}},
		{"61 features in a class", func(o *Overlay) {
			c := classIdx(o, "none")
			for n := range 61 {
				c.Levels[19].Features = append(c.Levels[19].Features, tf("extra-"+strings.Repeat("x", n%5)+string(rune('a'+n%26))+string(rune('a'+n/26)), "Extra"))
			}
		}, []string{"class:gen-none@mesa", "60 per class"}},
		{"61 features in a subclass", func(o *Overlay) {
			s := &o.Subclasses[0]
			for n := range 61 {
				s.Levels[1].Features = append(s.Levels[1].Features, tf("sub-extra-"+string(rune('a'+n%26))+string(rune('a'+n/26)), "Extra"))
			}
		}, []string{"subclass:gen-none-a@mesa", "60 per subclass"}},
		{"hit die", func(o *Overlay) { classIdx(o, "none").HitDie = 7 }, []string{"hit die"}},
		{"one saving throw", func(o *Overlay) { classIdx(o, "none").SavingThrows = []Ability{CON} }, []string{"two different"}},
		{"same saving throw twice", func(o *Overlay) { classIdx(o, "none").SavingThrows = []Ability{CON, CON} }, []string{"two different"}},
		{"skill outside the SRD", func(o *Overlay) { classIdx(o, "none").SkillFrom[0] = "skill:cooking" }, []string{"not in the SRD"}},
		{"choose more skills than listed", func(o *Overlay) { classIdx(o, "none").SkillChoose = 9 }, []string{"skills"}},
		{"a skill as a proficiency", func(o *Overlay) { classIdx(o, "none").Proficiencies = []string{"proficiency:skill-arcana"} }, []string{"proficiency"}},
		{"subclass level 0 is 3, 21 is not", func(o *Overlay) { classIdx(o, "none").SubclassLevel = 21 }, []string{"subclass is chosen"}},
		{"ASI level twice", func(o *Overlay) { classIdx(o, "none").ASILevels = []int{4, 4} }, []string{"Ability Score Improvement"}},
		{"ASI level 21", func(o *Overlay) { classIdx(o, "none").ASILevels = []int{21} }, []string{"Ability Score Improvement"}},
		{"proficiency bonus 13", func(o *Overlay) { classIdx(o, "none").Levels[3].ProfBonus = 13 }, []string{"proficiency bonus"}},
		{"slots in a class that does not cast", func(o *Overlay) { classIdx(o, "none").Levels[4].Slots[0] = 1 }, []string{"casting columns"}},
		{"a caster without slots", func(o *Overlay) { classIdx(o, "full-known").Levels[4].Slots = [9]int{} }, []string{"needs spell slots"}},
		{"pact slots of two levels", func(o *Overlay) { classIdx(o, "pact-known").Levels[6].Slots = [9]int{1, 1} }, []string{"pact"}},
		{"slots before casting starts", func(o *Overlay) { classIdx(o, "half-prepared").Levels[0].Slots[0] = 1 }, []string{"before casting starts"}},
		{"a class that is a third caster", func(o *Overlay) { classIdx(o, "full-known").Casting.Kind = CastingThird }, []string{"casting kind"}},
		{"casting without an ability", func(o *Overlay) { classIdx(o, "full-known").Casting.Ability = "luck" }, []string{"ability"}},
		{"casting that neither knows nor prepares", func(o *Overlay) { classIdx(o, "full-known").Casting.Preparation = PreparationSpellbook }, []string{"spells are"}},
		{"prepared formula without preparing", func(o *Overlay) { classIdx(o, "full-known").Casting.PreparedMax = "1" }, []string{"prepared_max"}},
		{"bad prepared formula", func(o *Overlay) { classIdx(o, "full-prepared").Casting.PreparedMax = "ghost()" }, []string{"feature:class-gen-full-prepared-spellcasting@mesa", "formula"}},
		{"subclass level that is not the class's", func(o *Overlay) { o.Subclasses[0].Level = 5 }, []string{"chosen at level 3"}},
		{"subclass levels out of order", func(o *Overlay) { s := &o.Subclasses[0]; s.Levels[0], s.Levels[1] = s.Levels[1], s.Levels[0] }, []string{"ascending"}},
		{"subclass feature before the subclass level", func(o *Overlay) { o.Subclasses[0].Levels[0].Level = 2 }, []string{"start at level 3"}},
		{"third caster of a class that casts", func(o *Overlay) {
			ts := thirdCaster(srd, "mago-falso", "class:wizard", "class:cleric", PreparationKnown)
			o.Subclasses = append(o.Subclasses, ts)
		}, []string{"subclass:mago-falso@mesa", "does not cast"}},
		{"third caster without a list", func(o *Overlay) {
			ts := thirdCaster(srd, "terco-sem-lista", "class:fighter", "", PreparationKnown)
			o.Subclasses = append(o.Subclasses, ts)
		}, []string{"needs the class whose spell list"}},
		{"third caster with a missing row", func(o *Overlay) {
			ts := thirdCaster(srd, "terco-curto", "class:fighter", "class:wizard", PreparationKnown)
			ts.Levels = slices.Delete(ts.Levels, 5, 6)
			o.Subclasses = append(o.Subclasses, ts)
		}, []string{"table row for every level"}},
		{"third caster starting before its subclass", func(o *Overlay) {
			ts := thirdCaster(srd, "terco-cedo", "class:fighter", "class:wizard", PreparationKnown)
			ts.Casting.StartLevel = 2
			o.Subclasses = append(o.Subclasses, ts)
		}, []string{"before the subclass"}},
		{"a subclass that is a full caster", func(o *Overlay) {
			ts := thirdCaster(srd, "terco-cheio", "class:fighter", "class:wizard", PreparationKnown)
			ts.Casting.Kind = CastingFull
			o.Subclasses = append(o.Subclasses, ts)
		}, []string{"casting kind"}},
		{"always-prepared cantrip", func(o *Overlay) {
			for i := range o.Subclasses {
				if len(o.Subclasses[i].AlwaysPrepared) > 0 {
					o.Subclasses[i].AlwaysPrepared[0].Spell = "spell:fire-bolt"
				}
			}
		}, []string{"not a cantrip"}},
		{"race size", func(o *Overlay) { o.Races[0].Size = "Huge" }, []string{"size"}},
		{"race speed", func(o *Overlay) { o.Races[0].SpeedFt = 33 }, []string{"speed"}},
		{"race bonus too big", func(o *Overlay) { o.Races[0].AbilityBonuses[STR] = 9 }, []string{"ability bonus"}},
		{"race bonus to place", func(o *Overlay) { o.Races[0].ChoiceBonuses = []int{0} }, []string{"bonus to place"}},
		{"race language outside the SRD", func(o *Overlay) { o.Races[0].Languages = []string{"language:dracônico-livre"} }, []string{"not in the SRD"}},
		{"background with one skill", func(o *Overlay) { o.Backgrounds[0].Skills = o.Backgrounds[0].Skills[:1] }, []string{"2 different skills"}},
		{"background with the same skill twice", func(o *Overlay) { o.Backgrounds[0].Skills[1] = o.Backgrounds[0].Skills[0] }, []string{"2 different skills"}},
		{"background tool that is armor", func(o *Overlay) { o.Backgrounds[0].Tools = []string{"proficiency:light-armor"} }, []string{"tool proficiency"}},
		{"spell level", func(o *Overlay) { o.Spells[0].Level = 10 }, []string{"spell level"}},
		{"spell school", func(o *Overlay) { o.Spells[0].School = "school:fantasia" }, []string{"school"}},
		{"spell target missing", func(o *Overlay) { o.Spells[0].Target = SpellTarget{} }, []string{"target"}},
		{"area without a shape", func(o *Overlay) { o.Spells[1].Target.Shape = "" }, []string{"cone, cube"}},
		{"area size", func(o *Overlay) { o.Spells[1].Target.SizeFt = 12 }, []string{"steps of 5"}},
		{"one creature with a count", func(o *Overlay) { o.Spells[0].Target.Count = 2 }, []string{"takes only 0 to 10 more"}},
		{"creatures with a count of 1", func(o *Overlay) { o.Spells[2].Target.Count = 1 }, []string{"several creatures"}},
		{"attack and save", func(o *Overlay) { o.Spells[0].Save = &SpellSave{Ability: DEX, OnSuccess: "half"} }, []string{"not both"}},
		{"attack with an area", func(o *Overlay) { o.Spells[1].Attack = "melee"; o.Spells[1].Save = nil }, []string{"area target"}},
		{"self target with a range", func(o *Overlay) { o.Spells[0].Target = SpellTarget{Kind: TargetSelf} }, []string{"range Self"}},
		{"trigger on an action", func(o *Overlay) { o.Spells[0].CastingTime.TriggerPT = "quando algo acontece" }, []string{"only a reaction"}},
		{"trigger with a line break", func(o *Overlay) {
			o.Spells[0].CastingTime = TableCastingTime{Unit: CastReaction, TriggerPT: "a\nb"}
		}, []string{"trigger"}},
		{"always-prepared on a subclass that never casts", func(o *Overlay) {
			o.Subclasses[0].AlwaysPrepared = []TableAlwaysPrepared{{ClassLevel: 3, Spell: "spell:bless"}}
		}, []string{"always-prepared", "casts"}},
		{"slug with magical-secrets", func(o *Overlay) { feature(o).Key = "feature:magical-secrets-extra@mesa" }, []string{"reserved", "magical-secrets"}},
		{"slug with wild-shape", func(o *Overlay) { feature(o).Key = "feature:wild-shape-lobo@mesa" }, []string{"reserved", "wild-shape"}},
		{"entry slug with the ASI stem", func(o *Overlay) { o.Races[0].Key = "race:ability-score-improvement@mesa" }, []string{"reserved"}},
		{"slug ending in spellcasting", func(o *Overlay) { feature(o).Key = "feature:class-x-spellcasting@mesa" }, []string{"reserved", "spellcasting"}},
		{"granted spell outside a note", func(o *Overlay) {
			feature(o).Effects = []Effect{{Type: "modifier", Target: "ac", Mode: "add", Value: "1", Spells: []string{"spell:light"}}}
		}, []string{"only a note"}},
		{"granted spell that does not exist", func(o *Overlay) {
			feature(o).Effects = []Effect{{Type: "note", Spells: []string{"spell:fantasma@mesa"}}}
		}, []string{"granted spell", "does not exist"}},
		{"61 traits in a race", func(o *Overlay) {
			for n := range 61 {
				o.Races[0].Traits = append(o.Races[0].Traits, TableFeature{Key: "trait:extra-" + string(rune('a'+n%26)) + string(rune('a'+n/26)) + tableSuffix, NamePT: "T"})
			}
		}, []string{"race:anao-das-brumas@mesa", "traits", "60"}},
		{"61 traits in a subrace", func(o *Overlay) {
			for n := range 61 {
				o.Subraces[0].Traits = append(o.Subraces[0].Traits, TableFeature{Key: "trait:sub-" + string(rune('a'+n%26)) + string(rune('a'+n/26)) + tableSuffix, NamePT: "T"})
			}
		}, []string{"traits", "60"}},
		{"6001 effects", func(o *Overlay) {
			for n := 0; len(o.Backgrounds) < MaxOverlayEffects/MaxFeatureEffects+1; n++ {
				bg := o.Backgrounds[0]
				bg.Key = "background:b" + strconv.Itoa(n) + tableSuffix
				bg.Feature = TableFeature{Key: "background-feature:b" + strconv.Itoa(n) + tableSuffix, NamePT: "F", Effects: slices.Repeat([]Effect{{Type: "note"}}, 20)}
				o.Backgrounds = append(o.Backgrounds, bg)
			}
		}, []string{"6000 effects", "limit"}},
		{"501 distinct formulas", func(o *Overlay) {
			c := classIdx(o, "none")
			for n := range 11 {
				fs := make([]TableFeature, 0, 3)
				for k := range 3 {
					var es []Effect
					for e := range 17 {
						es = append(es, Effect{Type: "modifier", Target: "ac", Mode: "add", Value: strconv.Itoa(n*1000 + k*100 + e + 1)})
					}
					fs = append(fs, tf("f"+strconv.Itoa(n)+"-"+strconv.Itoa(k), "F", es...))
				}
				c.Levels[n].Features = append(c.Levels[n].Features, fs...)
			}
		}, []string{"distinct formulas", "500"}},
		{"effect text too long", func(o *Overlay) {
			feature(o).Effects = []Effect{{Type: "note", TextPT: strings.Repeat("a", 4001)}}
		}, []string{"effect text"}},
		{"too many tags", func(o *Overlay) {
			feature(o).Effects = []Effect{{Type: "note", Tags: slices.Repeat([]string{"x"}, 21)}}
		}, []string{"at most 20"}},
		{"save on success", func(o *Overlay) { o.Spells[1].Save.OnSuccess = "other" }, []string{"half or nothing"}},
		{"half damage without damage", func(o *Overlay) { o.Spells[1].Damage = nil }, []string{"needs damage"}},
		{"damage type", func(o *Overlay) { o.Spells[0].Damage[0].Type = "damage-type:psychic-fire" }, []string{"damage type"}},
		{"damage dice with a bonus", func(o *Overlay) { o.Spells[0].Damage[0].Dice = "3d6+2" }, []string{"plain dice"}},
		{"extra dice of another die", func(o *Overlay) { o.Spells[0].Damage[0].PerSlotLevel = "1d8" }, []string{"same die"}},
		{"a cantrip per slot level", func(o *Overlay) { o.Spells[4].Damage[0].PerSlotLevel = "1d8" }, []string{"tier"}},
		{"a leveled spell per tier", func(o *Overlay) { o.Spells[0].Damage[0].PerTier = "1d6" }, []string{"only a cantrip"}},
		{"healing cantrip", func(o *Overlay) { o.Spells[4].Heal = &TableSpellHeal{Dice: "1d4"} }, []string{"cantrip does not heal"}},
		{"concentration with an instantaneous duration", func(o *Overlay) { o.Spells[0].Concentration = true }, []string{"concentration"}},
		{"a cantrip ritual", func(o *Overlay) { o.Spells[4].Ritual = true }, []string{"not a ritual"}},
		{"material without text", func(o *Overlay) { o.Spells[0].Components.Material = true }, []string{"material text"}},
		{"casting time", func(o *Overlay) { o.Spells[0].CastingTime = TableCastingTime{Amount: 2, Unit: CastAction} }, []string{"takes 1"}},
		{"range", func(o *Overlay) { o.Spells[0].Range.DistanceFt = 7 }, []string{"range"}},
		{"duration", func(o *Overlay) { o.Spells[2].Duration.Amount = 0 }, []string{"timed duration"}},
		{"empty name", func(o *Overlay) { o.Spells[0].NamePT = " " }, []string{"name"}},
		{"name with a line break", func(o *Overlay) { o.Spells[0].NamePT = "Raio\nde luz" }, []string{"one line"}},
		{"name too long", func(o *Overlay) { o.Spells[0].NamePT = strings.Repeat("a", 81) }, []string{"more than 80"}},
		{"text too long", func(o *Overlay) { o.Spells[0].DescPT = []string{strings.Repeat("a", 4001)} }, []string{"more than 4000"}},
		{"negative revision", func(o *Overlay) { o.Revision = -1 }, []string{"revision"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			o := good()
			tc.edit(&o)
			c, err := srd.With(o)
			if err == nil {
				t.Fatal("With accepted the overlay")
			}
			if _, ok := errors.AsType[*OverlayError](err); !ok {
				t.Fatalf("error = %T %v, want *OverlayError", err, err)
			}
			for _, w := range tc.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error %q does not mention %q", err, w)
				}
			}
			if c != nil {
				t.Error("With returned a content with its error")
			}
		})
	}
}

// TestGeneratedKeysCannotCollide: the keys the engine writes for a class and for
// a third caster's subclass of the same slug carry the kind, so both are valid
// together; and the reserved stems keep a table feature from taking an SRD
// feature's name or an engine key.
func TestGeneratedKeysCannotCollide(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	tc := genClass(srd, genKinds[4]) // pact
	tc.Key = "class:runa" + tableSuffix
	for i := range tc.Levels {
		tc.Levels[i].Features = nil
	}
	third := thirdCaster(srd, "runa", "class:fighter", "class:wizard", PreparationKnown)
	third.Class = "class:fighter"
	c := withOverlayOn(t, srd, Overlay{Revision: 1, Classes: []TableClass{tc}, Subclasses: []TableSubclass{third}})
	for _, key := range []string{"feature:class-runa-spellcasting@mesa", "feature:subclass-runa-spellcasting@mesa", "feature:class-runa-ability-score-improvement-1@mesa"} {
		if !c.c.exists(key) {
			t.Errorf("%s is not there", key)
		}
	}
	// A class whose slug is "runa" is not offered an ASI before level 4 or a
	// half caster at 1 to 2 an ASI: no feature of a table key looks like one.
	for _, k := range sortedKeys(c.c.features) {
		if isTableKey(k) && strings.Contains(k, "-ability-score-improvement-") && !strings.HasPrefix(k, "feature:class-") {
			t.Errorf("%s looks like an ASI but is not the engine's", k)
		}
	}
}

// TestOverlayDoesNotDependOnOrder: the entries listed in any order give the same
// content: the same Derived for a sheet, the same catalog.
func TestOverlayDoesNotDependOnOrder(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	o := fullOverlay(t, srd)
	o.Subclasses = append(o.Subclasses, TableSubclass{
		Key: "subclass:duelista" + tableSuffix, NamePT: "Duelista", Class: "class:fighter",
		Levels: []TableSubclassLevel{{Level: 3, Features: []TableFeature{tf("duelista-3", "Duelo")}}},
	}, TableSubclass{
		Key: "subclass:arqueiro" + tableSuffix, NamePT: "Arqueiro", Class: "class:fighter",
		Levels: []TableSubclassLevel{{Level: 3, Features: []TableFeature{tf("arqueiro-3", "Arco")}}},
	})
	o.Subraces = append(o.Subraces, TableSubrace{Key: "subrace:alto-elfo-do-norte" + tableSuffix, NamePT: "Elfo do Norte", Race: "race:elf"})

	reverse := func(o Overlay) Overlay {
		slices.Reverse(o.Classes)
		slices.Reverse(o.Subclasses)
		slices.Reverse(o.Races)
		slices.Reverse(o.Subraces)
		slices.Reverse(o.Backgrounds)
		slices.Reverse(o.Spells)
		return o
	}
	rotate := func(o Overlay) Overlay {
		o.Spells = append(slices.Clone(o.Spells[2:]), o.Spells[:2]...)
		o.Subclasses = append(slices.Clone(o.Subclasses[3:]), o.Subclasses[:3]...)
		o.Classes = append(slices.Clone(o.Classes[1:]), o.Classes[:1]...)
		return o
	}
	derived := func(o Overlay) (string, string) {
		c := withOverlayOn(t, srd, o)
		classKey, subKey := "class:gen-full-prepared@mesa", "subclass:gen-full-prepared-b@mesa"
		b := sweepBase(t, c, classKey, subKey)
		b = sweepUp(t, c, b, classKey, subKey, 8)
		d := Derive(b, c)
		out, _ := json.Marshal(d)
		cat, _ := json.Marshal(c.Catalog())
		return string(out), string(cat) + strings.Join(c.c.classes["class:fighter"].Subclasses, ",") + strings.Join(c.c.races["race:elf"].Subraces, ",")
	}
	want, wantCat := derived(fullOverlayWith(o))
	for name, perm := range map[string]func(Overlay) Overlay{"reversed": reverse, "rotated": rotate} {
		got, gotCat := derived(perm(fullOverlayWith(o)))
		if got != want || gotCat != wantCat {
			t.Errorf("%s: the content depends on the order of the entries", name)
		}
	}
}

// fullOverlayWith copies the slices of an overlay, so a permutation does not
// reorder the original.
func fullOverlayWith(o Overlay) Overlay {
	o.Classes, o.Subclasses, o.Races = slices.Clone(o.Classes), slices.Clone(o.Subclasses), slices.Clone(o.Races)
	o.Subraces, o.Backgrounds, o.Spells = slices.Clone(o.Subraces), slices.Clone(o.Backgrounds), slices.Clone(o.Spells)
	return o
}

// TestGrantedSpellOnATableRace: a race that knows a cantrip (a note that grants
// it), and a spell once a day (the grant plus a resource, as the SRD's Infernal
// Legacy has no handler either). The granted spells are on the character's list
// and count against no class number, so a Fighter with a granted cantrip has no
// Issue.
func TestGrantedSpellOnATableRace(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	race, _, _ := tableMisc()
	race.Traits = append(race.Traits, TableFeature{
		Key: "trait:luz-interior" + tableSuffix, NamePT: "Luz interior", DescPT: []string{"Conhece o truque Luz e uma magia por dia."},
		Effects: []Effect{
			{Type: "note", Spells: []string{"spell:light", "spell:burning-hands"}},
			{Type: "resource", Resource: "luz_interior", Max: "1", Recharge: "long_rest"},
		},
	})
	race.ChoiceBonuses = nil
	c := withOverlayOn(t, srd, Overlay{Revision: 1, Races: []TableRace{race}})
	b := Build{
		BaseScores: map[Ability]int{STR: 15, DEX: 14, CON: 13, INT: 12, WIS: 10, CHA: 8},
		Race:       race.Key, Background: "background:acolyte", Languages: testLanguages,
		Classes:            []ClassLevel{{Class: "class:fighter", Level: 1}},
		SkillProficiencies: []string{"skill:athletics", "skill:acrobatics"},
		FeatureChoices:     []string{"feature:fighter-fighting-style-defense"},
		Cantrips:           []string{"spell:light"},
		SpellsPrepared:     []string{"spell:burning-hands"},
	}
	if err := Validate(b, c); err != nil {
		t.Fatal(err)
	}
	d := Derive(b, c)
	if len(d.Issues) != 0 {
		t.Errorf("issues = %v", d.Issues)
	}
	if !slices.ContainsFunc(d.Resources, func(r Resource) bool { return r.Key == "luz_interior" && r.Recharge == RechargeLongRest }) {
		t.Errorf("resources = %+v", d.Resources)
	}
	// The same sheet without the race's grant has the Issues (the spells are on
	// no list), so the grant is what makes them valid.
	b.Race = "race:human"
	if d := Derive(b, c); !hasIssue(d, IssueSpellNotOnList) {
		t.Error("without the grant the spells are off the list")
	}
}

// TestOverlayErrorFields: an error points at the entry as the caller passed it
// (its own index, even when With sorts the entries) and carries a stable reason.
func TestOverlayErrorFields(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	cases := []struct {
		name          string
		edit          func(o *Overlay)
		field, reason string
	}{
		{"a formula deep in a class", func(o *Overlay) {
			o.Classes[2].Levels[4].Features = []TableFeature{tf("x", "X", Effect{Type: "note"}, Effect{Type: "modifier", Target: "ac", Mode: "add", Value: "rand()"})}
		}, "classes[2].levels[4].features[0].effects[1].value", ReasonFormula},
		{"a handler in a trait", func(o *Overlay) {
			o.Races[0].Traits[0].Effects = []Effect{{Type: "handler", Handler: "monk.martial_arts"}}
		}, "races[0].traits[0].effects[0].type", ReasonEffect},
		{"a key", func(o *Overlay) { o.Spells[3].Key = "spell:sem-sufixo" }, "spells[3].key", ReasonKey},
		{"a reserved key", func(o *Overlay) { o.Spells[3].Key = "spell:wild-shape-x@mesa" }, "spells[3].key", ReasonReservedKey},
		{"a duplicate", func(o *Overlay) { o.Spells[1].Key = o.Spells[0].Key }, "spells[1].key", ReasonDuplicateKey},
		{"a hit die", func(o *Overlay) { o.Classes[1].HitDie = 7 }, "classes[1].hit_die", ReasonValue},
		{"rows", func(o *Overlay) { o.Classes[3].Levels = o.Classes[3].Levels[:5] }, "classes[3].levels", ReasonTable},
		{"a dangling parent", func(o *Overlay) { o.Subclasses[2].Class = "class:fantasma@mesa" }, "subclasses[2].class_key", ReasonReference},
		{"a trait budget", func(o *Overlay) {
			for n := range 61 {
				o.Races[0].Traits = append(o.Races[0].Traits, TableFeature{Key: "trait:t" + strconv.Itoa(n) + tableSuffix, NamePT: "T"})
			}
		}, "races[0].traits", ReasonLimit},
		{"an area attack", func(o *Overlay) { o.Spells[1].Attack = "ranged"; o.Spells[1].Save = nil }, "spells[1].attack", ReasonValue},
	}
	for _, tc := range cases {
		o := fullOverlay(t, srd)
		tc.edit(&o)
		// The caller's order is not the sorted one: the index must still be the caller's.
		_, err := srd.With(o)
		var oe *OverlayError
		if !errors.As(err, &oe) {
			t.Errorf("%s: err = %v", tc.name, err)
			continue
		}
		if oe.Field != tc.field || oe.Reason != tc.reason {
			t.Errorf("%s: field %q reason %q, want %q %q (%v)", tc.name, oe.Field, oe.Reason, tc.field, tc.reason, err)
		}
	}
}

// TestSubclassEntryCasting: the catalog says how a third caster's subclass
// casts; a subclass that does not cast has no Casting.
func TestSubclassEntryCasting(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	c := withOverlayOn(t, srd, genOverlay(t, srd))
	var third, plain SubclassEntry
	for _, e := range c.Catalog().Subclasses {
		switch e.Key {
		case "subclass:cavaleiro-runico@mesa":
			third = e
		case "subclass:champion":
			plain = e
		}
	}
	sc := third.Casting
	if sc == nil || sc.Kind != CastingThird || sc.Ability != INT || sc.Preparation != PreparationKnown || sc.SpellList != "class:wizard" || sc.StartLevel != 3 {
		t.Fatalf("casting = %+v", sc)
	}
	if len(sc.MaxSpellLevelByLevel) != MaxLevel || sc.MaxSpellLevelByLevel[1] != 0 || sc.MaxSpellLevelByLevel[2] != 1 || sc.MaxSpellLevelByLevel[19] != 4 {
		t.Errorf("max spell level by level = %v", sc.MaxSpellLevelByLevel)
	}
	if plain.Casting != nil {
		t.Errorf("the champion casts: %+v", plain.Casting)
	}
	// The caster the sheet derives names the list.
	b := sweepBase(t, c, "class:fighter", "subclass:cavaleiro-runico@mesa")
	b = sweepUp(t, c, b, "class:fighter", "subclass:cavaleiro-runico@mesa", 4)
	if d := Derive(b, c); len(d.Spellcasting) != 1 || d.Spellcasting[0].SpellList != "class:wizard" || d.Spellcasting[0].Class != "class:fighter" {
		t.Errorf("spellcasting = %+v", d.Spellcasting)
	}
}

// TestArchivedSubclassInTheLevelUpOffer: the offer marks a retired subclass, so
// the screen does not offer it as a new choice.
func TestArchivedSubclassInTheLevelUpOffer(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	o := genOverlay(t, srd)
	for i := range o.Subclasses {
		if o.Subclasses[i].Key == "subclass:cavaleiro-runico@mesa" {
			o.Subclasses[i].Archived = true
		}
	}
	c := withOverlayOn(t, srd, o)
	b := sweepBase(t, c, "class:fighter", "subclass:champion")
	b = sweepUp(t, c, b, "class:fighter", "subclass:champion", 2)
	offer, err := LevelUpOptions(b, "class:fighter", c)
	if err != nil || !offer.SubclassDue {
		t.Fatalf("offer %+v, err %v", offer, err)
	}
	archived := map[string]bool{}
	for _, s := range offer.Subclasses {
		archived[s.Key] = s.Archived
	}
	if !archived["subclass:cavaleiro-runico@mesa"] || archived["subclass:champion"] || archived["subclass:trapaceiro-mistico@mesa"] {
		t.Errorf("archived marks = %v", archived)
	}
}

// TestReactionSpellKeepsItsTrigger: a table reaction spell has the trigger text
// in its details, and an action spell has none.
func TestReactionSpellKeepsItsTrigger(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	o := fullOverlay(t, srd)
	o.Spells[0].CastingTime = TableCastingTime{Unit: CastReaction, TriggerPT: "quando você sofre dano, mesmo de longe"}
	c := withOverlayOn(t, srd, o)
	d, _ := c.SpellDetails("spell:raio-de-teste@mesa")
	if d.CastingTime.Unit != CastReaction || d.CastingTime.Trigger != "quando você sofre dano, mesmo de longe" {
		t.Errorf("casting time = %+v", d.CastingTime)
	}
	if e := c.c.spellEntries["spell:raio-de-teste@mesa"]; e.CastingTime.Unit != CastReaction {
		t.Errorf("entry casting time = %+v", e.CastingTime)
	}
	if self := (SpellTarget{Kind: TargetSelf}); self.MaxTargets(1, 1) != 0 {
		t.Error("a self spell takes 0 targets, as play's maxTargetsOf says")
	}
}
