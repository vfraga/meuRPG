package rules

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/PuraFome/meuRPG/backend/internal/rules/srd51"
)

// loadForTest loads the embedded content once per test.
func loadForTest(t testing.TB) *Content {
	t.Helper()
	c, err := LoadSRD()
	if err != nil {
		t.Fatalf("LoadSRD: %v", err)
	}
	return c
}

// TestSnapshot checks the embedded snapshot against its manifest: the
// pinned commit is recorded, every generated file still has the hash the
// importer wrote (so nobody edited data/ by hand), the effects revision
// matches the effects files, and the NOTICE carries the exact attribution.
func TestSnapshot(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	m := c.c.manifest

	if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(m.SourceCommit) {
		t.Errorf("manifest source_commit = %q, want a full commit SHA", m.SourceCommit)
	}
	if m.SourceRepo != "https://github.com/5e-bits/5e-srd-api" {
		t.Errorf("manifest source_repo = %q", m.SourceRepo)
	}
	if want := "srd51@" + m.SourceCommit[:12]; m.SnapshotVersion != want {
		t.Errorf("snapshot_version = %q, want %q", m.SnapshotVersion, want)
	}
	if !regexp.MustCompile(`^srd51@[0-9a-f]{12}\+fx\.[1-9][0-9]*$`).MatchString(c.Version()) {
		t.Errorf("Version() = %q, want srd51@<sha12>+fx.<n>", c.Version())
	}

	t.Run("every generated file matches the manifest", func(t *testing.T) {
		t.Parallel()
		files, err := fs.Glob(srd51.Files, "data/*.json")
		if err != nil {
			t.Fatal(err)
		}
		listed := map[string]string{}
		for _, o := range m.Outputs {
			listed[o.Name] = o.SHA256
		}
		for _, f := range files {
			name := path.Base(f)
			if name == "manifest.json" {
				continue
			}
			b, err := fs.ReadFile(srd51.Files, f)
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(b)
			want, ok := listed[name]
			switch {
			case !ok:
				t.Errorf("%s is not in the manifest: generate data/ with cmd/srdimport", name)
			case hex.EncodeToString(sum[:]) != want:
				t.Errorf("%s changed after cmd/srdimport wrote it: data/ is generated, never edited by hand", name)
			}
			delete(listed, name)
		}
		for name := range listed {
			t.Errorf("manifest lists %s, which is missing", name)
		}
		if len(m.Inputs) == 0 {
			t.Error("manifest has no input hashes")
		}
	})

	t.Run("effects revision matches the effects files", func(t *testing.T) {
		t.Parallel()
		var rev effectsRevision
		if err := readJSON(srd51.Files, "effects/revision.json", &rev); err != nil {
			t.Fatal(err)
		}
		got := effectsHash(t)
		if rev.SHA256 != got {
			t.Errorf("effects/ changed: content versions never change in place (ADR-0008), so raise \"revision\" in effects/revision.json to %d and set \"sha256\" to %q", rev.Revision+1, got)
		}
		if !strings.HasSuffix(c.Version(), "+fx."+strconv.Itoa(rev.Revision)) {
			t.Errorf("Version() = %q does not end in fx.%d", c.Version(), rev.Revision)
		}
	})

	t.Run("NOTICE has the attribution", func(t *testing.T) {
		t.Parallel()
		b, err := os.ReadFile("../../../NOTICE")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), srd51.Attribution) {
			t.Error("NOTICE must contain srd51.Attribution byte for byte")
		}
		if c.Catalog().Attribution != srd51.Attribution {
			t.Error("Catalog().Attribution differs from srd51.Attribution")
		}
		if !strings.Contains(string(b), m.SourceCommit) {
			t.Error("NOTICE must name the pinned 5e-srd-api commit")
		}
	})

	t.Run("the credits page shows the attribution the content carries", func(t *testing.T) {
		t.Parallel()
		page, err := os.ReadFile("../../../web/src/app/pages/credits/credits.ts")
		if err != nil {
			t.Fatal(err)
		}
		got, ok := creditsAttribution(string(page))
		if !ok {
			t.Fatal("credits.ts has no SRD_ATTRIBUTION string to compare")
		}
		if got != c.Catalog().Attribution {
			t.Errorf("credits.ts SRD_ATTRIBUTION differs from Content.attribution:\n web:    %q\n server: %q", got, c.Catalog().Attribution)
		}
		// The check can fail: a copy with one word changed is not the attribution.
		changed := strings.Replace(string(page), "Wizards of the Coast LLC", "Wizards of the Coast", 1)
		if other, _ := creditsAttribution(changed); other == c.Catalog().Attribution {
			t.Error("a changed web copy still equals the server's attribution: the comparison does not look at the text")
		}
	})
}

// creditsAttribution reads the SRD_ATTRIBUTION string literal of the web's
// credits page.
func creditsAttribution(src string) (string, bool) {
	m := regexp.MustCompile(`(?s)export const SRD_ATTRIBUTION =\s*'((?:[^'\\]|\\.)*)'`).FindStringSubmatch(src)
	if m == nil {
		return "", false
	}
	return strings.ReplaceAll(m[1], `\'`, `'`), true
}

// effectsHash is the sha256 of every file in effects/ but revision.json, by
// name and content, in name order.
func effectsHash(t testing.TB) string {
	t.Helper()
	files, err := fs.Glob(srd51.Files, "effects/*.json")
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(files)
	h := sha256.New()
	for _, f := range files {
		if path.Base(f) == "revision.json" {
			continue
		}
		b, err := fs.ReadFile(srd51.Files, f)
		if err != nil {
			t.Fatal(err)
		}
		h.Write([]byte(path.Base(f)))
		h.Write([]byte{0})
		h.Write(b)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// TestEffectsCoverLevels1To5 makes sure every feature a class or SRD
// subclass gives at levels 1 to 5, every racial trait and every background
// feature was reviewed: each has an effects entry, even if only a note
// ("shown as text, the master decides").
func TestEffectsCoverLevels1To5(t *testing.T) {
	t.Parallel()
	c := loadForTest(t).c
	for _, key := range sortedKeys(c.features) {
		if f := c.features[key]; f.Level <= 5 && len(c.effects[key]) == 0 {
			t.Errorf("%s (level %d) has no effects entry", key, f.Level)
		}
	}
	for _, key := range sortedKeys(c.traits) {
		if len(c.effects[key]) == 0 {
			t.Errorf("%s has no effects entry", key)
		}
	}
	for _, b := range c.backgrounds {
		if len(c.effects[b.Feature.Key]) == 0 {
			t.Errorf("%s has no effects entry", b.Feature.Key)
		}
	}
	for key := range c.classes {
		if _, ok := c.casting[key]; !ok && slices.Contains([]string{"class:bard", "class:cleric", "class:druid", "class:paladin", "class:ranger", "class:sorcerer", "class:warlock", "class:wizard"}, key) {
			t.Errorf("%s casts spells but has no spellcasting effect", key)
		}
	}
}

// TestNamesPT checks that everything the sheet and the editor name has our
// Portuguese name.
// TestNamesPTAreUnique: two spells (or creatures, items, races, classes,
// subclasses or backgrounds) never share a Portuguese name, or a list would show
// two different entries the same. Features are left out: the same feature of
// several classes (Ataque Extra) has one name on purpose.
func TestNamesPTAreUnique(t *testing.T) {
	t.Parallel()
	c := loadForTest(t).c
	seen := map[string]string{}
	for _, key := range sortedKeys(c.namesPT) {
		kind, _, _ := strings.Cut(key, ":")
		if !slices.Contains([]string{"spell", "monster", "item", "race", "subrace", "class", "subclass", "background"}, kind) {
			continue
		}
		name := kind + ":" + strings.ToLower(c.namesPT[key])
		if other, ok := seen[name]; ok {
			t.Errorf("%s and %s are both %q", other, key, c.namesPT[key])
		}
		seen[name] = key
	}
}

func TestNamesPT(t *testing.T) {
	t.Parallel()
	c := loadForTest(t).c
	var keys []string
	for _, a := range AllAbilities() {
		keys = append(keys, string(a))
	}
	for _, m := range []map[string]bool{
		keySet(c.skills), keySet(c.races), keySet(c.subraces), keySet(c.classes), keySet(c.subclasses),
		keySet(c.backgrounds), keySet(c.equipment), keySet(c.spells), keySet(c.languages), keySet(c.traits),
		keySet(c.named), keySet(c.monsters), keySet(c.magicItems),
	} {
		keys = append(keys, sortedKeys(m)...)
	}
	for _, key := range sortedKeys(c.features) {
		if c.features[key].Level <= 5 {
			keys = append(keys, key)
		}
	}
	for _, key := range sortedKeys(c.proficiencies) {
		if p := c.proficiencies[key]; p.Kind == "armor" || p.Kind == "weapon" {
			keys = append(keys, key)
		}
	}
	for _, s := range senses {
		keys = append(keys, "sense:"+s)
	}
	for _, key := range keys {
		if _, ok := c.namesPT[key]; !ok {
			t.Errorf("%s has no Portuguese name in effects/names_pt.json", key)
		}
	}
	// All 334 SRD creatures, and nothing else under "monster:".
	n := 0
	for key, name := range c.namesPT {
		if !strings.HasPrefix(key, "monster:") {
			continue
		}
		n++
		if _, ok := c.monsters[key]; !ok || strings.TrimSpace(name) == "" {
			t.Errorf("%s: a Portuguese name for a creature that does not exist, or an empty one", key)
		}
	}
	if n != 334 {
		t.Errorf("%d creature names in names_pt.json, want 334", n)
	}
}

// TestAttackNamesPT: every attack of every creature has a Portuguese name: the
// ones a character can have through a creature (Wild Shape, Conjurar Animais, the
// familiar forms, Animar Mortos, the Pacto da Corrente) and the ones the
// bestiary's "Criar NPC" copies onto an NPC's sheet (MR-042).
func TestAttackNamesPT(t *testing.T) {
	t.Parallel()
	c := loadForTest(t).c
	for _, key := range sortedKeys(c.monsters) {
		for _, a := range c.monsters[key].Actions {
			if !a.HasAttack {
				continue
			}
			if _, ok := c.namesPT["attack:"+slugOf(a.Name)]; !ok {
				t.Errorf("%s: attack %q (attack:%s) has no Portuguese name in effects/names_pt.json", key, a.Name, slugOf(a.Name))
			}
		}
	}
}

func keySet[T any](m map[string]T) map[string]bool {
	out := make(map[string]bool, len(m))
	for k := range m {
		out[k] = true
	}
	return out
}

// TestReferences checks that every key the snapshot points at exists, so
// the engine never has to guess. 5e-database is a community compilation;
// a broken reference in a newer commit shows up here, not on a sheet.
func TestReferences(t *testing.T) {
	t.Parallel()
	c := loadForTest(t).c
	check := func(from, key string) {
		t.Helper()
		if !c.exists(key) {
			t.Errorf("%s points at unknown %s", from, key)
		}
	}
	for _, r := range c.races {
		for _, k := range slices.Concat(r.Traits, r.Subraces, r.Languages) {
			check(r.Key, k)
		}
	}
	for _, s := range c.subraces {
		check(s.Key, s.Race)
		for _, k := range s.Traits {
			check(s.Key, k)
		}
	}
	for _, tr := range c.traits {
		for _, k := range slices.Concat(tr.Proficiencies, tr.ProficiencyOptions, tr.Options) {
			check(tr.Key, k)
		}
	}
	for _, cl := range c.classes {
		for _, k := range slices.Concat(cl.Proficiencies, cl.SkillChoices.From, cl.Subclasses, cl.Multiclass.Proficiencies) {
			check(cl.Key, k)
		}
		if cl.SubclassLevel < 1 {
			t.Errorf("%s has no subclass level", cl.Key)
		}
	}
	for _, rows := range c.classLevels {
		for _, row := range rows {
			for _, k := range row.Features {
				check(row.Class, k)
			}
		}
	}
	for _, rows := range c.subclassLevels {
		for _, row := range rows {
			for _, k := range row.Features {
				check(row.Subclass, k)
			}
		}
	}
	for _, s := range c.subclasses {
		check(s.Key, s.Class)
		for _, sp := range s.Spells {
			check(s.Key, sp.Spell)
		}
	}
	for _, f := range c.features {
		for _, k := range f.Options {
			check(f.Key, k)
		}
	}
	for _, b := range c.backgrounds {
		for _, k := range slices.Concat(b.Skills, b.Proficiencies) {
			check(b.Key, k)
		}
	}
	for _, s := range c.spells {
		check(s.Key, s.School)
		for _, k := range s.Classes {
			check(s.Key, k)
		}
	}
	for _, m := range c.monsters {
		for _, k := range m.ConditionImmunities {
			check(m.Key, k)
		}
		for k := range m.Skills {
			check(m.Key, k)
		}
		for _, list := range [][]srd51.MonsterDamageMod{m.Vulnerabilities, m.Resistances, m.Immunities} {
			for _, d := range list {
				for _, k := range d.Types {
					check(m.Key, k)
				}
			}
		}
		for _, a := range m.Actions {
			for _, d := range a.Damage {
				check(m.Key+" "+a.Name, d.DamageType)
			}
		}
	}
	for _, e := range c.equipment {
		if w := e.Weapon; w != nil {
			if w.DamageType != "" { // the net deals no damage
				check(e.Key, w.DamageType)
			}
			for _, k := range w.Properties {
				check(e.Key, k)
			}
		}
	}
}

// TestCatalog checks what the editor gets.
func TestCatalog(t *testing.T) {
	t.Parallel()
	cat := loadForTest(t).Catalog()
	counts := map[string][2]int{
		"races":       {len(cat.Races), 9},
		"subraces":    {len(cat.Subraces), 4},
		"classes":     {len(cat.Classes), 12},
		"subclasses":  {len(cat.Subclasses), 12},
		"backgrounds": {len(cat.Backgrounds), 1},
		"skills":      {len(cat.Skills), 18},
		"armor":       {len(cat.Armor), 12},
		"weapons":     {len(cat.Weapons), 37},
		"spells":      {len(cat.Spells), 319},
		"abilities":   {len(cat.Abilities), 6},
	}
	for name, n := range counts {
		if n[0] != n[1] {
			t.Errorf("Catalog has %d %s, want %d", n[0], name, n[1])
		}
	}
	for _, a := range cat.Armor {
		if a.Category == "shield" {
			t.Error("Catalog.Armor must not list the shield")
		}
	}
	if !slices.IsSortedFunc(cat.Spells, func(a, b SpellEntry) int { return comparePT(a.NamePT, b.NamePT) }) {
		t.Error("Catalog.Spells is not sorted by Portuguese name")
	}
	want := map[string]string{
		"class:wizard": PreparationSpellbook, "class:cleric": PreparationPrepared, "class:sorcerer": PreparationKnown,
		"class:warlock": PreparationKnown, "class:paladin": PreparationPrepared, "class:fighter": "",
	}
	for _, cl := range cat.Classes {
		if w, ok := want[cl.Key]; ok && cl.SpellPreparation != w {
			t.Errorf("%s SpellPreparation = %q, want %q", cl.Key, cl.SpellPreparation, w)
		}
		if cl.Key == "class:paladin" && cl.SpellcastingLevel != 2 {
			t.Errorf("paladin SpellcastingLevel = %d, want 2", cl.SpellcastingLevel)
		}
		if cl.Key == "class:wizard" && (cl.SubclassLevel != 2 || cl.HitDie != 6 || cl.SkillChoices != 2 || cl.NamePT != "Mago") {
			t.Errorf("wizard entry = %+v", cl)
		}
	}
	for _, s := range cat.Spells {
		if s.Key == "spell:fire-bolt" && (s.School != "school:evocation" || s.SchoolNamePT != "Evocação" || s.Level != 0) {
			t.Errorf("fire bolt entry = %+v", s)
		}
	}
	if cat.Abilities[3].AbbreviationPT != "INT" || cat.Abilities[4].AbbreviationPT != "SAB" || cat.Abilities[0].NamePT != "Força" {
		t.Errorf("abilities = %+v", cat.Abilities)
	}
}

// TestCorrectionsAreClosed: effects/corrections.json is applied over the
// snapshot, and the loader refuses a correction it does not know.
func TestCorrectionsAreClosed(t *testing.T) {
	t.Parallel()
	c := &content{classLevels: map[string][]*srd51.Level{}}
	rows := make([]*srd51.Level, MaxLevel)
	for i := range rows {
		rows[i] = &srd51.Level{ClassSpecific: []byte(`{"invocations_known":9}`)}
	}
	c.classLevels["class:warlock"] = rows
	for name, doc := range map[string]string{
		"unknown class": `{"corrections":[{"class":"class:nope","field":"invocations_known","by_level":{"1":1}}]}`,
		"unknown field": `{"corrections":[{"class":"class:warlock","field":"rage_damage","by_level":{"1":1}}]}`,
		"unknown level": `{"corrections":[{"class":"class:warlock","field":"invocations_known","by_level":{"21":1}}]}`,
	} {
		fsys := fstest.MapFS{"effects/corrections.json": {Data: []byte(doc)}}
		if err := c.applyCorrections(fsys); err == nil {
			t.Errorf("%s: the loader accepted it", name)
		}
	}
	ok := fstest.MapFS{"effects/corrections.json": {Data: []byte(`{"corrections":[{"class":"class:warlock","field":"invocations_known","by_level":{"4":2}}]}`)}}
	if err := c.applyCorrections(ok); err != nil || invocationsKnown(rows[3]) != 2 || invocationsKnown(rows[4]) != 9 {
		t.Errorf("applyCorrections = %v, level 4 %d, level 5 %d; want only level 4 changed to 2", err, invocationsKnown(rows[3]), invocationsKnown(rows[4]))
	}
}

// TestCreatureCorrectionsAreClosed: the loader refuses a creature correction it does
// not know, and accepts a good one.
func TestCreatureCorrectionsAreClosed(t *testing.T) {
	t.Parallel()
	monsters := map[string]*srd51.Monster{
		"monster:veteran": {Actions: []srd51.MonsterAction{{Name: "Multiattack", Multiattack: [][]srd51.MonsterAttackCount{{{Name: "Longsword", Count: 2, Kind: "melee"}}}}}},
		"monster:rat":     {Actions: []srd51.MonsterAction{{Name: "Bite"}}},
	}
	one := func(creature, field, value string) string {
		return `{"creature_corrections":[{"creature":"` + creature + `","field":"` + field + `","value":` + value + `,"source":"SRD"}]}`
	}
	for name, doc := range map[string]string{
		"unknown creature": one("monster:nope", "attacks_per_action", "2"),
		"unknown field":    one("monster:veteran", "challenge_rating", "2"),
		"no Multiattack":   one("monster:rat", "attacks_per_action", "2"),
		"duplicate":        `{"creature_corrections":[{"creature":"monster:veteran","field":"attacks_per_action","value":3,"source":"SRD"},{"creature":"monster:veteran","field":"attacks_per_action","value":2,"source":"SRD"}]}`,
		"value below 1":    one("monster:veteran", "attacks_per_action", "0"),
		"value above 20":   one("monster:veteran", "attacks_per_action", "21"),
		"without a source": `{"creature_corrections":[{"creature":"monster:veteran","field":"attacks_per_action","value":3}]}`,
	} {
		c := &content{classLevels: map[string][]*srd51.Level{}, monsters: monsters}
		fsys := fstest.MapFS{"effects/corrections.json": {Data: []byte(doc)}}
		if err := c.applyCorrections(fsys); err == nil {
			t.Errorf("%s: the loader accepted it", name)
		}
	}
	c := &content{classLevels: map[string][]*srd51.Level{}, monsters: monsters}
	if err := c.applyCorrections(fstest.MapFS{"effects/corrections.json": {Data: []byte(one("monster:veteran", "attacks_per_action", "3"))}}); err != nil || c.attacksPerAction["monster:veteran"] != 3 {
		t.Errorf("a good correction = %v, %v; want it applied", err, c.attacksPerAction)
	}
}

// TestStatBlockCorrectionsAreClosed: the loader refuses a stat block correction
// whose value does not fit its field, and writes a good one over the creature.
func TestStatBlockCorrectionsAreClosed(t *testing.T) {
	t.Parallel()
	newContent := func() *content {
		return &content{
			classLevels: map[string][]*srd51.Level{},
			skills:      map[string]*srd51.Skill{"skill:perception": {}},
			named:       map[string]*srd51.Named{"damage-type:cold": {}, "condition:deafened": {}},
			monsters: map[string]*srd51.Monster{"monster:ogre": {
				HitDice: "6d8", HitPoints: 22, HitPointsRoll: "6d8-5", Darkvision: 30,
				Immunities: []srd51.MonsterDamageMod{{Types: []string{"damage-type:fire"}, Note: "from nonmagical weapons"}},
			}},
		}
	}
	one := func(field, value string) string {
		return `{"creature_corrections":[{"creature":"monster:ogre","field":"` + field + `","value":` + value + `,"source":"SRD"}]}`
	}
	for name, doc := range map[string]string{
		"roll of other dice":         one("hit_points_roll", `"5d8+5"`),
		"roll that is not a roll":    one("hit_points_roll", `"many"`),
		"hit points of zero":         one("hit_points", "0"),
		"speed that is not a number": one("speed_swim", `"fast"`),
		"negative sense":             one("darkvision", "-1"),
		"unknown skill":              one("skills", `{"skill:nope":2}`),
		"unknown condition":          one("condition_immunities", `["condition:nope"]`),
		"condition listed twice":     one("condition_immunities", `["condition:deafened","condition:deafened"]`),
		"damage type as condition":   one("condition_immunities", `["damage-type:cold"]`),
		"immunity that has a note":   one("damage_immunities", `["damage-type:cold"]`),
	} {
		if err := newContent().applyCorrections(fstest.MapFS{"effects/corrections.json": {Data: []byte(doc)}}); err == nil {
			t.Errorf("%s: the loader accepted it", name)
		}
	}
	c := newContent()
	doc := `{"creature_corrections":[
		{"creature":"monster:ogre","field":"hit_points","value":33,"source":"SRD"},
		{"creature":"monster:ogre","field":"hit_points_roll","value":"6d8+6","source":"SRD"},
		{"creature":"monster:ogre","field":"darkvision","value":0,"source":"SRD"},
		{"creature":"monster:ogre","field":"blindsight","value":30,"source":"SRD"},
		{"creature":"monster:ogre","field":"skills","value":{"skill:perception":3},"source":"SRD"},
		{"creature":"monster:ogre","field":"condition_immunities","value":["condition:deafened"],"source":"SRD"}]}`
	if err := c.applyCorrections(fstest.MapFS{"effects/corrections.json": {Data: []byte(doc)}}); err != nil {
		t.Fatalf("good corrections: %v", err)
	}
	m := c.monsters["monster:ogre"]
	if m.HitPoints != 33 || m.HitPointsRoll != "6d8+6" || m.Darkvision != 0 || m.Blindsight != 30 || m.Skills["skill:perception"] != 3 || len(m.ConditionImmunities) != 1 {
		t.Errorf("corrected stat block = %+v, want every field written", m)
	}
}

// TestSpellAndSubclassCorrectionsAreClosed: the loader refuses a spell or subclass
// correction it does not know, and writes a good one over the snapshot.
func TestSpellAndSubclassCorrectionsAreClosed(t *testing.T) {
	t.Parallel()
	newContent := func() *content {
		return &content{
			classLevels: map[string][]*srd51.Level{},
			named:       map[string]*srd51.Named{"damage-type:fire": {}, "condition:deafened": {}},
			spells: map[string]*srd51.Spell{
				"spell:ray":  {Level: 2},
				"spell:ward": {Level: 3},
			},
			subclasses: map[string]*srd51.Subclass{"subclass:life": {Spells: []srd51.SubclassSpell{{Spell: "spell:ray", ClassLevel: 3}}}},
		}
	}
	spell := func(body string) string {
		return `{"spell_corrections":[{"spell":"spell:ray",` + body + `,"source":"SRD"}]}`
	}
	for name, doc := range map[string]string{
		"unknown spell":               `{"spell_corrections":[{"spell":"spell:nope","attack_type":"ranged","source":"SRD"}]}`,
		"nothing to correct":          spell(`"attack_type":""`),
		"attack that is not a type":   spell(`"attack_type":"thrown"`),
		"save that is not an ability": spell(`"save_ability":"luck","save_success":"half"`),
		"save without an outcome":     spell(`"save_ability":"dex"`),
		"outcome without a save":      spell(`"save_success":"half"`),
		"damage of a condition":       spell(`"damage":[{"damage_type":"condition:deafened","at_slot_level":{"2":"2d6"}}]`),
		"damage without slot levels":  spell(`"damage":[{"damage_type":"damage-type:fire","at_slot_level":{}}]`),
		"slot below the spell":        spell(`"damage":[{"damage_type":"damage-type:fire","at_slot_level":{"1":"2d6"}}]`),
		"slot above the ninth":        spell(`"damage":[{"damage_type":"damage-type:fire","at_slot_level":{"10":"2d6"}}]`),
		"dice that are not dice":      spell(`"damage":[{"damage_type":"damage-type:fire","at_slot_level":{"2":"lots"}}]`),
		"without a source":            `{"spell_corrections":[{"spell":"spell:ray","attack_type":"ranged"}]}`,
		"corrected twice":             `{"spell_corrections":[{"spell":"spell:ray","attack_type":"ranged","source":"SRD"},{"spell":"spell:ray","attack_type":"melee","source":"SRD"}]}`,
		"unknown subclass":            `{"subclass_corrections":[{"subclass":"subclass:nope","add_spells":[{"spell":"spell:ray","class_level":7}],"source":"SRD"}]}`,
		"subclass spell unknown":      `{"subclass_corrections":[{"subclass":"subclass:life","add_spells":[{"spell":"spell:nope","class_level":7}],"source":"SRD"}]}`,
		"subclass level out":          `{"subclass_corrections":[{"subclass":"subclass:life","add_spells":[{"spell":"spell:ward","class_level":21}],"source":"SRD"}]}`,
		"subclass spell already has":  `{"subclass_corrections":[{"subclass":"subclass:life","add_spells":[{"spell":"spell:ray","class_level":3}],"source":"SRD"}]}`,
		"subclass without a source":   `{"subclass_corrections":[{"subclass":"subclass:life","add_spells":[{"spell":"spell:ward","class_level":7}]}]}`,
	} {
		if err := newContent().applyCorrections(fstest.MapFS{"effects/corrections.json": {Data: []byte(doc)}}); err == nil {
			t.Errorf("%s: the loader accepted it", name)
		}
	}
	c := newContent()
	doc := `{"spell_corrections":[{"spell":"spell:ray","attack_type":"ranged","save_ability":"dex","save_success":"half",
		"damage":[{"damage_type":"damage-type:fire","at_slot_level":{"2":"2d6","3":"3d6"}}],"source":"SRD"}],
		"subclass_corrections":[{"subclass":"subclass:life","add_spells":[{"spell":"spell:ward","class_level":7}],"source":"SRD"}]}`
	if err := c.applyCorrections(fstest.MapFS{"effects/corrections.json": {Data: []byte(doc)}}); err != nil {
		t.Fatalf("good corrections: %v", err)
	}
	if s := c.spells["spell:ray"]; s.AttackType != "ranged" || s.SaveAbility != "dex" || s.SaveSuccess != "half" || len(s.Damage) != 1 || s.Damage[0].AtSlotLevel["3"] != "3d6" {
		t.Errorf("corrected spell = %+v, want attack, save and damage written", s)
	}
	if got := c.subclasses["subclass:life"].Spells; len(got) != 2 || got[1].Spell != "spell:ward" || got[1].ClassLevel != 7 {
		t.Errorf("corrected subclass spells = %+v, want the new spell at level 7 after the old one", got)
	}
}

// TestSpellListAndSubclassFeatureCorrectionsAreClosed: the loader refuses a spell
// list or subclass feature correction it does not know, and writes a good one over
// the snapshot.
func TestSpellListAndSubclassFeatureCorrectionsAreClosed(t *testing.T) {
	t.Parallel()
	newContent := func() *content {
		return &content{
			classLevels: map[string][]*srd51.Level{},
			classes:     map[string]*srd51.Class{"class:bard": {}, "class:cleric": {}},
			spells: map[string]*srd51.Spell{
				"spell:fire":  {Classes: []string{"class:cleric"}},
				"spell:light": {Classes: []string{"class:bard", "class:cleric"}},
			},
			subclasses: map[string]*srd51.Subclass{"subclass:land": {Class: "class:druid"}, "subclass:life": {Class: "class:cleric"}},
			features: map[string]*srd51.Feature{
				"feature:terrain":  {Subclass: "subclass:land"},
				"feature:domain":   {Subclass: "subclass:life"},
				"feature:circle-2": {Subclass: "subclass:land"},
			},
			subclassLevels: map[string]map[int]*srd51.Level{"subclass:land": {2: {Features: []string{"feature:circle-2"}}}},
		}
	}
	for name, doc := range map[string]string{
		"list of an unknown class":     `{"spell_list_corrections":[{"class":"class:nope","add":["spell:fire"],"source":"SRD"}]}`,
		"list with an unknown spell":   `{"spell_list_corrections":[{"class":"class:bard","add":["spell:nope"],"source":"SRD"}]}`,
		"list without a source":        `{"spell_list_corrections":[{"class":"class:bard","add":["spell:fire"]}]}`,
		"list that corrects nothing":   `{"spell_list_corrections":[{"class":"class:bard","source":"SRD"}]}`,
		"spell added that is on it":    `{"spell_list_corrections":[{"class":"class:bard","add":["spell:light"],"source":"SRD"}]}`,
		"spell removed that is not on": `{"spell_list_corrections":[{"class":"class:bard","remove":["spell:fire"],"source":"SRD"}]}`,
		"spell added and removed":      `{"spell_list_corrections":[{"class":"class:bard","add":["spell:fire"],"remove":["spell:fire"],"source":"SRD"}]}`,
		"list corrected twice":         `{"spell_list_corrections":[{"class":"class:bard","add":["spell:fire"],"source":"SRD"},{"class":"class:bard","remove":["spell:light"],"source":"SRD"}]}`,
		"features of an unknown class": `{"subclass_feature_corrections":[{"subclass":"subclass:nope","level":2,"add_features":["feature:terrain"],"source":"SRD"}]}`,
		"unknown feature":              `{"subclass_feature_corrections":[{"subclass":"subclass:land","level":2,"add_features":["feature:nope"],"source":"SRD"}]}`,
		"feature of another subclass":  `{"subclass_feature_corrections":[{"subclass":"subclass:land","level":2,"add_features":["feature:domain"],"source":"SRD"}]}`,
		"features at a level out":      `{"subclass_feature_corrections":[{"subclass":"subclass:land","level":21,"add_features":["feature:terrain"],"source":"SRD"}]}`,
		"feature the row has":          `{"subclass_feature_corrections":[{"subclass":"subclass:land","level":2,"add_features":["feature:circle-2"],"source":"SRD"}]}`,
		"features without a source":    `{"subclass_feature_corrections":[{"subclass":"subclass:land","level":2,"add_features":["feature:terrain"]}]}`,
		"features corrected twice":     `{"subclass_feature_corrections":[{"subclass":"subclass:land","level":3,"add_features":["feature:terrain"],"source":"SRD"},{"subclass":"subclass:land","level":3,"add_features":["feature:circle-2"],"source":"SRD"}]}`,
		"features with none to add":    `{"subclass_feature_corrections":[{"subclass":"subclass:land","level":3,"source":"SRD"}]}`,
	} {
		if err := newContent().applyCorrections(fstest.MapFS{"effects/corrections.json": {Data: []byte(doc)}}); err == nil {
			t.Errorf("%s: the loader accepted it", name)
		}
	}
	c := newContent()
	doc := `{"spell_list_corrections":[{"class":"class:bard","add":["spell:fire"],"remove":["spell:light"],"source":"SRD"}],
		"subclass_feature_corrections":[
			{"subclass":"subclass:land","level":2,"add_features":["feature:terrain"],"source":"SRD"},
			{"subclass":"subclass:land","level":3,"add_features":["feature:terrain"],"source":"SRD"}]}`
	if err := c.applyCorrections(fstest.MapFS{"effects/corrections.json": {Data: []byte(doc)}}); err != nil {
		t.Fatalf("good corrections: %v", err)
	}
	if got := c.spells["spell:fire"].Classes; !slices.Equal(got, []string{"class:cleric", "class:bard"}) {
		t.Errorf("Fire's classes = %v, want the bard added", got)
	}
	if got := c.spells["spell:light"].Classes; !slices.Equal(got, []string{"class:cleric"}) {
		t.Errorf("Light's classes = %v, want the bard removed", got)
	}
	if got := c.subclassLevels["subclass:land"][2].Features; !slices.Equal(got, []string{"feature:circle-2", "feature:terrain"}) {
		t.Errorf("land features at 2 = %v, want the terrain added after the old one", got)
	}
	if row := c.subclassLevels["subclass:land"][3]; row == nil || row.Class != "class:druid" || row.Subclass != "subclass:land" || row.Level != 3 || !slices.Equal(row.Features, []string{"feature:terrain"}) {
		t.Errorf("land row at 3 = %+v, want one made for the subclass", row)
	}
}
