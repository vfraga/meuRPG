package characters

import (
	"fmt"
	"slices"
	"testing"
	"time"

	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// storedEntry is a stored entry as the engine reads it; name is the stored name column.
func storedEntry(kind rulesv1.TableContentKind, key, name string, body tableBody) entryRow {
	featureKeys(key, body, nil) // the keys the editor gives the features
	return entryRow{row: charactersdb.CampaignContent{ContentKey: key, Kind: kindPrefix(kind), NamePt: name}, kind: kind, body: body}
}

func TestBuildTolerantLeavesOutARefusedEntryAndWhatDependsOnIt(t *testing.T) {
	t.Parallel()
	srd := loadRules(t)
	entries := []entryRow{
		storedEntry(rulesv1.TableContentKind_TABLE_CONTENT_KIND_CLASS, "class:guardiao@mesa", "", testClass("Guardião")), // no name: refused
		storedEntry(rulesv1.TableContentKind_TABLE_CONTENT_KIND_SUBCLASS, "subclass:caminho@mesa", "Caminho", testSubclass("Caminho", "class:guardiao@mesa")),
		storedEntry(rulesv1.TableContentKind_TABLE_CONTENT_KIND_RACE, "race:gnomo-teste@mesa", "Gnomo de teste", testRace("Gnomo de teste")),
	}
	c, out, err := buildTolerant(srd, entries, 3, nil)
	if err != nil {
		t.Fatalf("buildTolerant() error = %v", err)
	}
	if c.NamePT("race:gnomo-teste@mesa") != "Gnomo de teste" {
		t.Errorf("the healthy race is not in the content")
	}
	if c.NamePT("class:guardiao@mesa") != "" || c.NamePT("subclass:caminho@mesa") != "" {
		t.Errorf("the refused class or its subclass is in the content")
	}
	reasons := map[string]string{}
	for _, l := range out {
		reasons[l.Key] = l.Violations[0].GetReason()
	}
	want := map[string]string{"class:guardiao@mesa": rules.ReasonName, "subclass:caminho@mesa": rules.ReasonReference}
	if len(reasons) != len(want) || reasons["class:guardiao@mesa"] != want["class:guardiao@mesa"] || reasons["subclass:caminho@mesa"] != want["subclass:caminho@mesa"] {
		t.Errorf("left out = %v, want %v", reasons, want)
	}
	got := c.Refused()
	if len(got) != 2 || got[0].Key != "class:guardiao@mesa" || got[1].Key != "subclass:caminho@mesa" || got[0].Reason == "" {
		t.Errorf("Refused() = %v, want the class and the subclass in key order", got)
	}
	if kinds := []rulesv1.TableContentKind{out[0].Kind, out[1].Kind}; !slices.Contains(kinds, rulesv1.TableContentKind_TABLE_CONTENT_KIND_SUBCLASS) {
		t.Errorf("kinds = %v, want the subclass among them", kinds)
	}
}

func TestBuildTolerantHealthyLeavesNothingOut(t *testing.T) {
	t.Parallel()
	srd := loadRules(t)
	entries := []entryRow{storedEntry(rulesv1.TableContentKind_TABLE_CONTENT_KIND_RACE, "race:gnomo-teste@mesa", "Gnomo de teste", testRace("Gnomo de teste"))}
	c, out, err := buildTolerant(srd, entries, 1, nil)
	if err != nil || len(out) != 0 || len(c.Refused()) != 0 || c.NamePT("race:gnomo-teste@mesa") != "Gnomo de teste" {
		t.Errorf("buildTolerant() = %v, %v, %v; want the race and nothing left out", c, out, err)
	}
}

func TestBuildTolerantFailsWhenTheErrorNamesNoEntryOrAStrictOne(t *testing.T) {
	t.Parallel()
	srd := loadRules(t)
	bad := storedEntry(rulesv1.TableContentKind_TABLE_CONTENT_KIND_RACE, "race:sem-nome@mesa", "", testRace(""))
	if _, _, err := buildTolerant(srd, []entryRow{bad}, 1, nil, "race:sem-nome@mesa"); err == nil {
		t.Error("a strict entry was left out")
	}
	if _, out, err := buildTolerant(srd, []entryRow{bad}, 1, nil); err != nil || len(out) != 1 {
		t.Errorf("a non-strict one: %v, %v; want it left out", out, err)
	}
	// A violation of the content as a whole (a negative revision) names no entry.
	race := storedEntry(rulesv1.TableContentKind_TABLE_CONTENT_KIND_RACE, "race:gnomo-teste@mesa", "Gnomo de teste", testRace("Gnomo de teste"))
	if _, _, err := buildTolerant(srd, []entryRow{race}, -1, nil); err == nil {
		t.Error("a keyless violation was tolerated")
	}
}

// TestBuildTolerantLeavesOutManyRefusedEntries: 20 refused spells of one pack leave the campaign
// readable. The engine names one entry a build, so it takes 21 builds: one a spell, and the last one clean.
func TestBuildTolerantLeavesOutManyRefusedEntries(t *testing.T) {
	t.Parallel()
	srd := loadRules(t)
	const refused = 20
	entries := []entryRow{storedEntry(rulesv1.TableContentKind_TABLE_CONTENT_KIND_RACE, "race:gnomo@mesa", "Gnomo", testRace("Gnomo"))}
	for i := range refused {
		sp := testSpell(fmt.Sprintf("Raio %d", i), "class:wizard")
		sp.Level = 12
		entries = append(entries, storedEntry(rulesv1.TableContentKind_TABLE_CONTENT_KIND_SPELL, fmt.Sprintf("spell:raio%02d@mesa", i), sp.NamePt, sp))
	}
	content, out, builds, err := buildCounting(srd, entries, 1, nil, nil)
	if err != nil {
		t.Fatalf("buildCounting() error = %v, want the content without the %d spells", err, refused)
	}
	if len(out) != refused || len(content.Refused()) != refused {
		t.Errorf("left out %d, Refused() %d; want %d", len(out), len(content.Refused()), refused)
	}
	if builds != refused+1 {
		t.Errorf("builds = %d, want %d (one per refused spell and the clean one)", builds, refused+1)
	}
	if content.NamePT("race:gnomo@mesa") != "Gnomo" {
		t.Errorf("the healthy race is not in the content")
	}
}

// TestRefusedLeavesOutWhatTheMasterRetired: an archived entry the engine refuses is still out of the
// content, but it is not reported (Remover clears it), and neither is what is out only because of it.
func TestRefusedLeavesOutWhatTheMasterRetired(t *testing.T) {
	t.Parallel()
	srd := loadRules(t)
	archive := func(e entryRow) entryRow {
		now := time.Now()
		e.row.ArchivedAt = &now
		return e
	}
	class := storedEntry(rulesv1.TableContentKind_TABLE_CONTENT_KIND_CLASS, "class:guardiao@mesa", "", testClass("Guardião")) // no name: refused
	sub := storedEntry(rulesv1.TableContentKind_TABLE_CONTENT_KIND_SUBCLASS, "subclass:caminho@mesa", "Caminho", testSubclass("Caminho", "class:guardiao@mesa"))
	spell := testSpell("Raio", "class:wizard")
	spell.Level = 12
	live := storedEntry(rulesv1.TableContentKind_TABLE_CONTENT_KIND_SPELL, "spell:raio@mesa", "Raio", spell)
	race := storedEntry(rulesv1.TableContentKind_TABLE_CONTENT_KIND_RACE, "race:gnomo-teste@mesa", "Gnomo de teste", testRace("Gnomo de teste"))
	for _, tc := range []struct {
		name    string
		entries []entryRow
		want    []string
	}{
		{"nothing retired", []entryRow{class, sub, live, race}, []string{class.row.ContentKey, live.row.ContentKey, sub.row.ContentKey}},
		{"the retired class and its subclass", []entryRow{archive(class), sub, live, race}, []string{live.row.ContentKey}},
		{"a retired subclass of a class still out", []entryRow{class, archive(sub), live, race}, []string{class.row.ContentKey, live.row.ContentKey}},
		{"a retired entry alone", []entryRow{class, sub, archive(live), race}, []string{class.row.ContentKey, sub.row.ContentKey}},
		{"everything retired", []entryRow{archive(class), archive(sub), archive(live), race}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			content, out, err := buildTolerant(srd, tc.entries, 1, nil)
			if err != nil {
				t.Fatalf("buildTolerant() error = %v", err)
			}
			var got []string
			for _, r := range content.Refused() {
				got = append(got, r.Key)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("Refused() = %v, want %v", got, tc.want)
			}
			if content.NamePT(class.row.ContentKey) != "" || content.NamePT(live.row.ContentKey) != "" || content.NamePT(race.row.ContentKey) != "Gnomo de teste" {
				t.Errorf("the content holds a refused entry or lost the healthy race")
			}
			if tc.name == "nothing retired" {
				return
			}
			// What the build leaves out does not depend on the archive: only what is reported does.
			if len(out) < len(tc.want) {
				t.Errorf("left out %d, want at least the %d reported", len(out), len(tc.want))
			}
		})
	}
}

// TestRefusedLeavesOutAnUnreadableRowOnlyWhileItIsLive: a row that cannot be read is reported
// unless it is archived.
func TestRefusedLeavesOutAnUnreadableRowOnlyWhileItIsLive(t *testing.T) {
	t.Parallel()
	srd := loadRules(t)
	now := time.Now()
	rows := []charactersdb.CampaignContent{
		{ContentKey: "race:vivo@mesa", Kind: "race", Data: []byte("{")},
		{ContentKey: "race:velho@mesa", Kind: "race", Data: []byte("{"), ArchivedAt: &now},
	}
	content, err := buildFromRows(srd, rows, 1, nil)
	if err != nil {
		t.Fatalf("buildFromRows() error = %v", err)
	}
	got := content.Refused()
	if len(got) != 1 || got[0].Key != "race:vivo@mesa" {
		t.Errorf("Refused() = %v, want only the live unreadable row", got)
	}
}

// TestUnfinishedKeepsALiveChildWithAViolationOfItsOwn: a child out only because its parent is retired goes with it, and a
// child that is also wrong in a field of its own stays, because the master still has to fix it.
func TestUnfinishedKeepsALiveChildWithAViolationOfItsOwn(t *testing.T) {
	t.Parallel()
	v := func(reason string) []*rulesv1.TableContentViolation {
		return []*rulesv1.TableContentViolation{{Reason: reason}}
	}
	out := []leftOut{
		{Key: "class:a@mesa", Archived: true, Violations: v("bad_name")},
		{Key: "subclass:only@mesa", Parent: "class:a@mesa", Violations: v("dangling_reference")},
		{Key: "classfeature:deep@mesa", Parent: "subclass:only@mesa", Violations: v("dangling_reference")},
		{Key: "subclass:own@mesa", Parent: "class:a@mesa", Violations: v("bad_formula")},
	}
	var got []string
	for _, l := range unfinished(out) {
		got = append(got, l.Key)
	}
	if want := []string{"subclass:own@mesa"}; !slices.Equal(got, want) {
		t.Errorf("unfinished() = %v, want %v", got, want)
	}
}
