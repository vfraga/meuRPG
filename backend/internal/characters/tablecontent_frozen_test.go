package characters

import (
	"sort"
	"strconv"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// Characterization of the pure parts of the table content (MR-025, RN-23): what a player
// reads of one entry (forPlayer) and of the catalog (withoutArchived), what the checks of a
// pack find (checkPack, readPackJSON) and how the features get their keys (assignIn). Each
// answer is a golden file; none needs a database.

// frozenHidden is what the master hid in these tests: a class, a spell and a table race.
func frozenHidden(k string) bool {
	return k == "class:druid" || k == "spell:cure-wounds" || k == "race:oculta@mesa"
}

// frozenEffects are effects that grant a hidden and a visible spell, and choose from both.
func frozenEffects() []*rulesv1.TableEffect {
	return []*rulesv1.TableEffect{
		{Type: "note", Spells: []string{"spell:cure-wounds", "spell:fire-bolt"}},
		{Type: "choice", Choice: "spell", Count: 1, From: []string{"spell:fire-bolt", "spell:cure-wounds", "class:druid"}},
	}
}

// frozenFeatures is a feature with the effects, and one whose option has them.
func frozenFeatures() []*rulesv1.TableFeature {
	return []*rulesv1.TableFeature{
		{Key: "feature:a@mesa", NamePt: "A", Effects: frozenEffects()},
		{Key: "feature:b@mesa", NamePt: "B", Options: []*rulesv1.TableFeature{{Key: "feature:b-1@mesa", NamePt: "B1", Effects: frozenEffects()}}},
	}
}

// TestForPlayerOfEveryKindIsFrozen: the copy of each kind of entry a player reads, with every
// reference to a hidden key in it; the stored body is never changed, and with nothing hidden
// the entry goes as it is.
func TestForPlayerOfEveryKindIsFrozen(t *testing.T) {
	t.Parallel()
	bodies := []struct {
		label string
		kind  rulesv1.TableContentKind
		body  tableBody
	}{
		{"class casting from a hidden class", rulesv1.TableContentKind_TABLE_CONTENT_KIND_CLASS, &rulesv1.TableClass{
			NamePt: "Classe", Casting: &rulesv1.TableCasting{Kind: "half", ListFrom: "class:druid"},
			Levels: []*rulesv1.TableClassLevel{{Features: frozenFeatures()}, {}},
		}},
		{"class casting from a visible class", rulesv1.TableContentKind_TABLE_CONTENT_KIND_CLASS, &rulesv1.TableClass{
			NamePt: "Classe 2", Casting: &rulesv1.TableCasting{Kind: "full", ListFrom: "class:wizard"},
		}},
		{"class without casting", rulesv1.TableContentKind_TABLE_CONTENT_KIND_CLASS, &rulesv1.TableClass{NamePt: "Classe 3"}},
		{"subclass", rulesv1.TableContentKind_TABLE_CONTENT_KIND_SUBCLASS, &rulesv1.TableSubclass{
			NamePt: "Sub", ClassKey: "class:druid", Casting: &rulesv1.TableCasting{Kind: "third", ListFrom: "class:druid"},
			AlwaysPrepared: []*rulesv1.TableAlwaysPrepared{{ClassLevel: 3, SpellKey: "spell:cure-wounds"}, {ClassLevel: 5, SpellKey: "spell:fire-bolt"}},
			Levels:         []*rulesv1.TableSubclassLevel{{Level: 3, Features: frozenFeatures()}},
		}},
		{"subclass without casting", rulesv1.TableContentKind_TABLE_CONTENT_KIND_SUBCLASS, &rulesv1.TableSubclass{NamePt: "Sub 2", ClassKey: "class:fighter"}},
		{"race", rulesv1.TableContentKind_TABLE_CONTENT_KIND_RACE, &rulesv1.TableRace{NamePt: "Raça", Traits: frozenFeatures()}},
		{"subrace", rulesv1.TableContentKind_TABLE_CONTENT_KIND_SUBRACE, &rulesv1.TableSubrace{NamePt: "Sub-raça", RaceKey: "race:oculta@mesa", Traits: frozenFeatures()}},
		{"background", rulesv1.TableContentKind_TABLE_CONTENT_KIND_BACKGROUND, &rulesv1.TableBackground{NamePt: "Antecedente", Feature: frozenFeatures()[0]}},
		{"background without a feature", rulesv1.TableContentKind_TABLE_CONTENT_KIND_BACKGROUND, &rulesv1.TableBackground{NamePt: "Antecedente 2"}},
		{"spell", rulesv1.TableContentKind_TABLE_CONTENT_KIND_SPELL, &rulesv1.TableSpell{NamePt: "Magia", ClassKeys: []string{"class:druid", "class:wizard"}}},
		{"feat of a hidden race", rulesv1.TableContentKind_TABLE_CONTENT_KIND_FEAT, &rulesv1.TableFeat{
			NamePt: "Talento", Effects: frozenEffects(), Prerequisite: &rulesv1.FeatPrerequisite{RaceKey: "race:oculta@mesa", Level: 4},
		}},
		{"feat of a visible race", rulesv1.TableContentKind_TABLE_CONTENT_KIND_FEAT, &rulesv1.TableFeat{
			NamePt: "Talento 2", Prerequisite: &rulesv1.FeatPrerequisite{RaceKey: "race:elf"},
		}},
		{"feat without a prerequisite", rulesv1.TableContentKind_TABLE_CONTENT_KIND_FEAT, &rulesv1.TableFeat{NamePt: "Talento 3", Effects: frozenEffects()}},
	}
	var steps []tableContentGoldenStep
	for _, b := range bodies {
		e := entryRow{row: charactersdb.CampaignContent{ContentKey: "x:" + b.label}, kind: b.kind, body: b.body}
		before := proto.Clone(b.body)
		same := forPlayer(e, nil)
		if same.body != e.body {
			t.Errorf("%s: forPlayer with nothing hidden made a copy, want the entry as it is", b.label)
		}
		got := forPlayer(e, frozenHidden)
		if !proto.Equal(b.body, before) {
			t.Errorf("%s: forPlayer changed the stored body", b.label)
		}
		if got.row.ContentKey != e.row.ContentKey || got.kind != e.kind {
			t.Errorf("%s: forPlayer changed the row or the kind", b.label)
		}
		steps = append(steps, tableContentGoldenStep{b.label, got.body})
	}
	tableContentGolden(t, "for_player", nil, steps...)
}

// TestWithoutArchivedIsFrozen: the players' catalog of a content where every list has an
// archived, a switched-off and a kept entry, and the references to them, without and with
// the keys a player's sheet keeps.
func TestWithoutArchivedIsFrozen(t *testing.T) {
	t.Parallel()
	c := &rulesv1.Content{
		ContentVersion: "v+mesa.7",
		Races:          []*rulesv1.Race{{Key: "race:a"}, {Key: "race:arch", Archived: true}, {Key: "race:off", Off: true}},
		Subraces: []*rulesv1.Subrace{
			{Key: "subrace:a", RaceKey: "race:a"},
			{Key: "subrace:of-arch", RaceKey: "race:arch"},
			{Key: "subrace:of-off", RaceKey: "race:off"},
			{Key: "subrace:off", RaceKey: "race:a", Off: true},
		},
		Classes: []*rulesv1.CharacterClass{
			{Key: "class:a", Spellcasting: &rulesv1.ClassSpellcasting{ListClassKey: "class:arch"}},
			{Key: "class:b", Spellcasting: &rulesv1.ClassSpellcasting{ListClassKey: "class:a"}},
			{Key: "class:c"},
			{Key: "class:arch", Archived: true},
			{Key: "class:off", Off: true},
		},
		Subclasses: []*rulesv1.Subclass{
			{
				Key: "subclass:a", ClassKey: "class:a", Spellcasting: &rulesv1.SubclassSpellcasting{ListClassKey: "class:off"},
				AlwaysPrepared: []*rulesv1.SubclassAlwaysPrepared{{SpellKey: "spell:arch"}, {SpellKey: "spell:a"}},
			},
			{Key: "subclass:b", ClassKey: "class:a", Spellcasting: &rulesv1.SubclassSpellcasting{ListClassKey: "class:a"}},
			{Key: "subclass:of-arch", ClassKey: "class:arch"},
			{Key: "subclass:arch", ClassKey: "class:a", Archived: true},
		},
		Backgrounds: []*rulesv1.Background{{Key: "background:a"}, {Key: "background:off", Off: true}},
		Spells: []*rulesv1.Spell{
			{Key: "spell:a", ClassKeys: []string{"class:a", "class:arch", "class:off"}},
			{Key: "spell:arch", Archived: true},
			{Key: "spell:off", Off: true},
		},
	}
	before := proto.Clone(c)
	plain := withoutArchived(c, nil)
	kept := withoutArchived(c, map[string]bool{"race:arch": true, "class:off": true, "spell:off": true, "subclass:arch": true})
	if !proto.Equal(c, before) {
		t.Error("withoutArchived changed the master's catalog")
	}
	tableContentGolden(t, "without_archived", nil,
		tableContentGoldenStep{"nothing kept", plain},
		tableContentGoldenStep{"a sheet keeps race:arch, class:off, spell:off and subclass:arch", kept},
	)
}

// frozenPackEntry is a well formed feat of a pack.
func frozenPackEntry(slug string) *rulesv1.TableEntry {
	return &rulesv1.TableEntry{
		Key: "feat:" + slug + "@mesa", Kind: rulesv1.TableContentKind_TABLE_CONTENT_KIND_FEAT, NamePt: slug,
		Body: &rulesv1.TableEntry_TableFeat{TableFeat: &rulesv1.TableFeat{NamePt: slug}},
	}
}

// frozenPackSummary is what checkPack returns, in a form the golden file can hold.
type frozenPackSummary struct {
	Entries    []string `json:"entries"`
	Violations []string `json:"violations"`
}

// TestCheckPackIsFrozen: what checkPack keeps and says of each kind of pack, the size
// refusals (of the proto and of its JSON) first, and the per-entry checks in their order.
func TestCheckPackIsFrozen(t *testing.T) {
	t.Parallel()
	pack := func(entries ...*rulesv1.TableEntry) *rulesv1.TableContentPack {
		return &rulesv1.TableContentPack{Format: PackFormat, Version: PackVersion, Name: "Pacote", Entries: entries}
	}
	many := func(n int) *rulesv1.TableContentPack {
		p := pack()
		for i := range n {
			p.Entries = append(p.Entries, frozenPackEntry("f"+strings.Repeat("x", i%3)+string(rune('a'+i%26))+strings.Repeat("y", i/26)))
		}
		return p
	}
	cases := []struct {
		label   string
		pack    *rulesv1.TableContentPack
		unknown map[int][]*rulesv1.TableContentViolation
	}{
		{"a good pack", pack(frozenPackEntry("um"), frozenPackEntry("dois")), nil},
		{"an empty pack", pack(), nil},
		{"over the bytes as proto", func() *rulesv1.TableContentPack {
			e := frozenPackEntry("grande")
			e.GetTableFeat().DescPt = []string{strings.Repeat("a", MaxPackBytes)}
			return pack(e)
		}(), nil},
		// A control character is one byte of proto and six of JSON (\u0001).
		{"over the bytes only as JSON", func() *rulesv1.TableContentPack {
			e := frozenPackEntry("escapado")
			e.GetTableFeat().DescPt = []string{strings.Repeat("\x01", MaxPackBytes/2)}
			return pack(e)
		}(), nil},
		{"every pack field wrong", &rulesv1.TableContentPack{Format: "outro", Version: 2, Name: strings.Repeat("n", maxPackNameRunes+1)}, nil},
		{"a name of 80 characters", &rulesv1.TableContentPack{Format: PackFormat, Version: PackVersion, Name: strings.Repeat("ã", maxPackNameRunes)}, nil},
		{"a name with a carriage return", &rulesv1.TableContentPack{Format: PackFormat, Version: PackVersion, Name: "a\rb"}, nil},
		{"a name with a tab", &rulesv1.TableContentPack{Format: PackFormat, Version: PackVersion, Name: "a\tb"}, nil},
		{"too many entries, with a wrong format", func() *rulesv1.TableContentPack {
			p := many(rules.MaxOverlayEntries + 1)
			p.Format = ""
			return p
		}(), nil},
		{"the most entries", many(rules.MaxOverlayEntries), nil},
		{"each entry check", pack(
			&rulesv1.TableEntry{Key: "feat:k@mesa", NamePt: "k", Body: &rulesv1.TableEntry_TableFeat{TableFeat: &rulesv1.TableFeat{NamePt: "k"}}},
			&rulesv1.TableEntry{Key: "feat:Bad@mesa", Kind: rulesv1.TableContentKind_TABLE_CONTENT_KIND_FEAT, NamePt: "x"},
			frozenPackEntry("um"),
			frozenPackEntry("um"),
			&rulesv1.TableEntry{Key: "feat:sem-corpo@mesa", Kind: rulesv1.TableContentKind_TABLE_CONTENT_KIND_FEAT},
			&rulesv1.TableEntry{Key: "feat:outro-corpo@mesa", Kind: rulesv1.TableContentKind_TABLE_CONTENT_KIND_FEAT, Body: &rulesv1.TableEntry_TableRace{TableRace: &rulesv1.TableRace{NamePt: "r"}}},
			&rulesv1.TableEntry{Key: "feat:outro-nome@mesa", Kind: rulesv1.TableContentKind_TABLE_CONTENT_KIND_FEAT, NamePt: "a", Body: &rulesv1.TableEntry_TableFeat{TableFeat: &rulesv1.TableFeat{NamePt: "b"}}},
			&rulesv1.TableEntry{Key: "feat:sem-nome@mesa", Kind: rulesv1.TableContentKind_TABLE_CONTENT_KIND_FEAT, Body: &rulesv1.TableEntry_TableFeat{TableFeat: &rulesv1.TableFeat{NamePt: "c"}}},
			&rulesv1.TableEntry{Key: "race:raca@mesa", Kind: rulesv1.TableContentKind_TABLE_CONTENT_KIND_RACE, NamePt: "r", Body: &rulesv1.TableEntry_TableRace{TableRace: &rulesv1.TableRace{NamePt: "r"}}},
			&rulesv1.TableEntry{Key: "feat:wild-shape@mesa", Kind: rulesv1.TableContentKind_TABLE_CONTENT_KIND_FEAT, NamePt: "w", Body: &rulesv1.TableEntry_TableFeat{TableFeat: &rulesv1.TableFeat{NamePt: "w"}}},
		), map[int][]*rulesv1.TableContentViolation{
			2: {{Field: "table_feat.x", Reason: reasonUnknownField, Key: "feat:um@mesa", Message: "the field \"x\" does not exist"}},
			3: {{Field: "table_feat.y", Reason: reasonUnknownField}},
		}},
	}
	var steps []tableContentGoldenStep
	for _, tc := range cases {
		entries, violations := checkPack(tc.pack, tc.unknown)
		var s frozenPackSummary
		if len(tc.pack.GetEntries()) > 12 { // a summary of a long list: its size and its ends
			s.Entries = append(s.Entries, "count "+strconv.Itoa(len(entries)))
		} else {
			for _, e := range entries {
				s.Entries = append(s.Entries, packEntryFrozen(e))
			}
		}
		for _, v := range violations {
			s.Violations = append(s.Violations, v.GetField()+" | "+v.GetReason()+" | "+v.GetKey()+" | "+v.GetMessage())
		}
		steps = append(steps, tableContentGoldenStep{tc.label, s})
	}
	tableContentGolden(t, "check_pack", nil, steps...)
}

// packEntryFrozen is one entry checkPack kept: its index, key, kind, name and unknown fields.
func packEntryFrozen(e packEntry) string {
	var unknown []string
	for _, u := range e.unknown {
		unknown = append(unknown, u.GetField())
	}
	return strconv.Itoa(e.index) + " " + e.key + " " + e.kind.String() + " " + e.body.GetNamePt() + " unknown=" + strings.Join(unknown, ",")
}

// TestReadPackJSONIsFrozen: what readPackJSON makes of a file, the unknown fields at every
// depth (proto and JSON names alike, a null message, a list of messages), and the files it
// does not read.
//
// Finding: walkUnknown ranges over the decoded JSON object, a Go map, so the unknown fields of
// one entry (and the pack's) come out in a random order from one call to the next
// (tablecontent_pack_json.go:72); the web lists the violations in the order it gets them. The
// test sorts them.
func TestReadPackJSONIsFrozen(t *testing.T) {
	t.Parallel()
	files := []struct{ label, json string }{
		{"a good file", `{"format":"meurpg.table-content","version":1,"name":"P","entries":[{"key":"feat:um@mesa","kind":"TABLE_CONTENT_KIND_FEAT","namePt":"Um","tableFeat":{"namePt":"Um"}}]}`},
		{"unknown fields at every depth", `{"format":"meurpg.table-content","version":1,"name2":"x","zeta":1,"entries":[
			{"key":"feat:um@mesa","kind":"TABLE_CONTENT_KIND_FEAT","name_pt":"Um","extra":true,"table_feat":{"namePt":"Um","prerequisite":{"level":4,"proficiency":"x"},"effects":[{"type":"note","bogus":1},{"type":"note"},{"type":"note","nope":[1]}]}},
			{"key":"feat:dois@mesa","kind":"TABLE_CONTENT_KIND_FEAT","tableFeat":{"namePt":"Dois","prerequisite":null}}
		]}`},
		{"an unknown field in an entry past the list", `{"entries":[{"a":1},{"b":2}]}`},
		{"not JSON", `{"format":`},
		{"not an object", `[1,2]`},
		{"a field of the wrong type", `{"format":1}`},
		{"trailing data", `{"format":"meurpg.table-content"} {"x":1}`},
		{"over the bytes", `{"name":"` + strings.Repeat("a", MaxPackBytes) + `"}`},
	}
	var steps []tableContentGoldenStep
	for _, f := range files {
		pack, unknown, top := readPackJSON([]byte(f.json))
		out := map[string]any{"pack_nil": pack == nil}
		if pack != nil {
			out["entries"] = len(pack.GetEntries())
		}
		var u []string
		for idx, vs := range unknown {
			for _, v := range vs {
				u = append(u, strconv.Itoa(idx)+": "+v.GetField()+" | "+v.GetReason()+" | "+v.GetKey()+" | "+v.GetMessage())
			}
		}
		sort.Strings(u)
		var tops []string
		for _, v := range top {
			tops = append(tops, v.GetField()+" | "+v.GetReason()+" | "+v.GetMessage())
		}
		sort.Strings(tops)
		out["unknown"], out["top"] = u, tops
		steps = append(steps, tableContentGoldenStep{f.label, out})
	}
	tableContentGolden(t, "read_pack_json", nil, steps...)
}

// TestAssignInIsFrozen: the keys the features of a race get, as an editor's write and as a
// pack's: the stored keys kept, a key the entry does not have refused (or kept from a pack
// when it has the entry's stem), an old name's key reused once, and new keys made from the
// names; the options of a class's feature in their own scope.
//
// Finding: the field of a refused key of a race's, a subrace's or a background's feature is
// "table_race.features[i].key" (and "table_subrace.features", "table_background.features[0]"),
// but those messages call the list "traits" and the one feature "feature"
// (tablecontent.go:730 to 735, with assign's ".features" at tablecontent.go:825): the web finds
// no input at that path.
func TestAssignInIsFrozen(t *testing.T) {
	t.Parallel()
	const raceKey = "race:brumas@mesa"
	stem := featureStem("race", raceKey)
	old := &rulesv1.TableRace{NamePt: "Brumas", Traits: []*rulesv1.TableFeature{
		{Key: "trait:" + stem + "visao@mesa", NamePt: "Visão"},
		{Key: "trait:" + stem + "passo@mesa", NamePt: "Passo"},
	}}
	next := func() *rulesv1.TableRace {
		return &rulesv1.TableRace{NamePt: "Brumas", Traits: []*rulesv1.TableFeature{
			{NamePt: "Passo"}, // the old name: its key comes back
			{NamePt: "Passo"}, // the same name again: a new key
			{Key: "trait:" + stem + "visao@mesa", NamePt: "Outro nome"},   // a stored key stays
			{Key: "trait:" + stem + "da-mochila@mesa", NamePt: "Mochila"}, // the stem, not stored
			{Key: "trait:outra-coisa@mesa", NamePt: "Estranho"},           // another stem
			{NamePt: "Visão"}, // the old name, but its key is taken
			{NamePt: "Névoa Densa!"},
			{NamePt: ""},
		}}
	}
	cls := func() *rulesv1.TableClass {
		return &rulesv1.TableClass{NamePt: "Classe", Levels: []*rulesv1.TableClassLevel{
			{Features: []*rulesv1.TableFeature{{NamePt: "Postura", Options: []*rulesv1.TableFeature{{NamePt: "Alta"}, {NamePt: "Alta"}, {Key: "feature:x@mesa", NamePt: "Baixa"}}}}},
			{Features: []*rulesv1.TableFeature{{NamePt: "Postura"}}},
		}}
	}
	var steps []tableContentGoldenStep
	run := func(label string, body, oldBody tableBody, keys func(string, tableBody, tableBody) []*rulesv1.TableContentViolation, entryKey string) {
		violations := keys(entryKey, body, oldBody)
		var vs []string
		for _, v := range violations {
			vs = append(vs, v.GetField()+" | "+v.GetReason()+" | "+v.GetMessage())
		}
		steps = append(steps, tableContentGoldenStep{label + " (violations)", vs}, tableContentGoldenStep{label, body})
	}
	run("an editor's update", next(), old, featureKeys, raceKey)
	run("a pack's update", next(), old, importFeatureKeys, raceKey)
	run("an editor's new race", next(), nil, featureKeys, raceKey)
	run("a pack's new race", next(), nil, importFeatureKeys, raceKey)
	run("a new class with options", cls(), nil, featureKeys, "class:classe@mesa")
	keysOfClass := func(b *rulesv1.TableClass) []string {
		var out []string
		for _, l := range b.GetLevels() {
			for _, f := range l.GetFeatures() {
				out = append(out, f.GetKey())
				for _, o := range f.GetOptions() {
					out = append(out, o.GetKey())
				}
			}
		}
		return out
	}
	first := cls()
	first.Levels[0].Features[0].Options[2].Key = ""
	featureKeys("class:classe@mesa", first, nil)
	again := cls()
	again.Levels[0].Features[0].Options[2].Key = ""
	featureKeys("class:classe@mesa", again, first)
	// Finding: two options (or features of one level) with the same name, written again
	// without their keys, do not get their keys back: the old keys are found by name, and
	// collectOld keeps only the last key of a name (tablecontent.go:806 and 810), so the first
	// takes the second's key and the second gets a new one ("alta-2", "alta-3" for "alta",
	// "alta-2"). The editor sends the keys back, so only a hand-made request meets it.
	steps = append(steps,
		tableContentGoldenStep{"a class written by name", keysOfClass(first)},
		tableContentGoldenStep{"the same class written by name again", keysOfClass(again)},
	)
	tableContentGolden(t, "assign_in", nil, steps...)
}
