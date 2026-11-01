package characters

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	systemv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/system/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The acceptance criteria of MR-025 (docs/product/stories.md) for the content pack: the
// master exports the table's own entries as one file and imports it into another campaign
// (or back), all or nothing, under the same rules as the editors. Each test starts its own
// database.

// packTable is a master with two campaigns, "Mirathel" (with content) and "Outra" (empty),
// and a player in the second.
type packTable struct {
	h              *harness
	master, player *user
	from, to       string
	live           *liveSpy
}

func newPackTable(t *testing.T) *packTable {
	t.Helper()
	h := newHarness(t)
	pt := &packTable{h: h, live: &liveSpy{}}
	h.svc.SetLive(pt.live)
	pt.master, pt.player = h.newUser("Samuel"), h.newUser("Dona")
	pt.from = h.newCampaign(pt.master, "Mirathel: a Costa")
	pt.to = h.newCampaign(pt.master, "Outra", pt.player)
	return pt
}

// fill writes one entry of every kind into the campaign and returns them by kind.
func (pt *packTable) fill(t *testing.T, campaign string) map[string]*rulesv1.TableEntry {
	t.Helper()
	m := pt.master
	out := map[string]*rulesv1.TableEntry{}
	out["class"] = m.addEntry(t, campaign, testClass("Guardião do Vale"))
	out["subclass"] = m.addEntry(t, campaign, testSubclass("Caminho do Vento", "class:fighter"))
	out["race"] = m.addEntry(t, campaign, testRace("Anão das Brumas"))
	out["subrace"] = m.addEntry(t, campaign, testSubrace("Da Colina", out["race"].GetKey()))
	out["background"] = m.addEntry(t, campaign, testBackground("Guarda de farol"))
	out["spell"] = m.addEntry(t, campaign, testSpell("Raio de teste", "class:wizard"))
	out["feat"] = m.addEntry(t, campaign, testFeat("Punho de ferro"))
	return out
}

func (pt *packTable) export(t *testing.T, campaign string) *rulesv1.TableContentPack {
	t.Helper()
	res, err := pt.master.table.ExportTableContent(t.Context(), connect.NewRequest(&rulesv1.ExportTableContentRequest{CampaignId: campaign}))
	if err != nil {
		t.Fatalf("ExportTableContent() error = %v", err)
	}
	return res.Msg.GetPack()
}

func (pt *packTable) importPack(u *user, campaign string, pack *rulesv1.TableContentPack, mode rulesv1.TableImportMode, key string) (*rulesv1.ImportTableContentResponse, error) {
	res, err := u.table.ImportTableContent(pt.h.t.Context(), connect.NewRequest(&rulesv1.ImportTableContentRequest{
		CampaignId: campaign, Pack: pack, Mode: mode, IdempotencyKey: key,
	}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func (pt *packTable) preview(t *testing.T, campaign string, pack *rulesv1.TableContentPack) *rulesv1.ImportTableContentResponse {
	t.Helper()
	res, err := pt.importPack(pt.master, campaign, pack, rulesv1.TableImportMode_TABLE_IMPORT_MODE_PREVIEW, "")
	if err != nil {
		t.Fatalf("ImportTableContent(preview) error = %v", err)
	}
	return res
}

func (pt *packTable) apply(t *testing.T, campaign string, pack *rulesv1.TableContentPack, key string) *rulesv1.ImportTableContentResponse {
	t.Helper()
	res, err := pt.importPack(pt.master, campaign, pack, rulesv1.TableImportMode_TABLE_IMPORT_MODE_APPLY, key)
	if err != nil {
		t.Fatalf("ImportTableContent(apply) error = %v", err)
	}
	return res
}

// hints is how many times the campaign's session was told its content changed.
func (pt *packTable) hints(campaign string) int {
	pt.live.mu.Lock()
	defer pt.live.mu.Unlock()
	return strings.Count(strings.Join(pt.live.content, " "), campaign)
}

func (pt *packTable) revisionOf(t *testing.T, campaign string) int32 {
	t.Helper()
	res, err := pt.master.table.ListTableEntries(t.Context(), connect.NewRequest(&rulesv1.ListTableEntriesRequest{CampaignId: campaign}))
	if err != nil {
		t.Fatalf("ListTableEntries() error = %v", err)
	}
	return res.Msg.GetTableRevision()
}

// TestMR025_AnExportedPackImportsIntoAnotherCampaignAsTheSameEntries: the round trip. The
// pack holds only content (key, kind, name and body, no revision, date, count or switch),
// goes through its file form, and the second campaign ends with the same entries, with the
// same keys and the same feature keys; importing it again changes nothing.
func TestMR025_AnExportedPackImportsIntoAnotherCampaignAsTheSameEntries(t *testing.T) {
	t.Parallel()
	pt := newPackTable(t)
	made := pt.fill(t, pt.from)

	pack := pt.export(t, pt.from)
	if pack.GetFormat() != "meurpg.table-content" || pack.GetVersion() != 1 || pack.GetName() != "Mirathel: a Costa" || len(pack.GetEntries()) != len(made) {
		t.Fatalf("pack = format %q version %d name %q with %d entries, want the campaign's seven", pack.GetFormat(), pack.GetVersion(), pack.GetName(), len(pack.GetEntries()))
	}
	for _, e := range pack.GetEntries() {
		if e.GetRevision() != 0 || e.GetCreatedAt() != nil || e.GetUpdatedAt() != nil || e.GetArchived() || e.GetOff() || e.GetCharactersUsing() != 0 || e.GetArchivedAt() != nil {
			t.Errorf("the pack carries what the server manages: %v", e)
		}
		if e.GetKey() == "" || e.GetNamePt() == "" || bodyMessage(e) == nil {
			t.Errorf("an entry of the pack lacks its key, name or body: %v", e)
		}
	}

	// Through the file: proto JSON with the proto field names, then back.
	file, err := storeJSON.Marshal(pack)
	if err != nil || !strings.Contains(string(file), `"name_pt"`) || !strings.Contains(string(file), `"meurpg.table-content"`) {
		t.Fatalf("the pack file = %s, %v; want proto JSON with the proto names", file, err)
	}
	loaded := &rulesv1.TableContentPack{}
	if err := loadJSON.Unmarshal(file, loaded); err != nil {
		t.Fatal(err)
	}

	before := pt.revisionOf(t, pt.to)
	pv := pt.preview(t, pt.to, loaded)
	if pv.GetTotals().GetNew() != 7 || pv.GetTotals().GetRefused() != 0 || pt.revisionOf(t, pt.to) != before {
		t.Fatalf("preview totals = %v, revision %d (was %d); want seven new entries and nothing written", pv.GetTotals(), pt.revisionOf(t, pt.to), before)
	}
	res := pt.apply(t, pt.to, loaded, "import-1")
	if res.GetTotals().GetNew() != 7 || res.GetTableRevision() != before+1 {
		t.Fatalf("apply totals = %v at revision %d, want seven new at one new revision (%d)", res.GetTotals(), res.GetTableRevision(), before+1)
	}
	if n := pt.hints(pt.to); n != 1 {
		t.Errorf("content_changed was sent %d times, want once for the whole import", n)
	}

	got := pt.entries(t, pt.to)
	for kind, e := range made {
		in := got[e.GetKey()]
		if in == nil {
			t.Fatalf("the %s %s is not in the second campaign", kind, e.GetKey())
		}
		if !proto.Equal(bodyMessage(in), bodyMessage(e)) {
			t.Errorf("the %s body differs after the round trip (feature keys included)", kind)
		}
		if in.GetRevision() != res.GetTableRevision() {
			t.Errorf("the %s is at revision %d, want the import's %d", kind, in.GetRevision(), res.GetTableRevision())
		}
	}
	again := pt.export(t, pt.to)
	again.Name = pack.Name
	if !proto.Equal(again, pack) {
		t.Error("exporting the second campaign does not give the first one's pack back")
	}

	// The same entries again: nothing changes, and nothing is announced.
	second := pt.apply(t, pt.to, loaded, "import-2")
	if second.GetTotals().GetUnchanged() != 7 || second.GetTableRevision() != res.GetTableRevision() || pt.revisionOf(t, pt.to) != res.GetTableRevision() || pt.hints(pt.to) != 1 {
		t.Errorf("importing the same pack again: totals %v, revision %d; want seven unchanged, the same revision, no hint", second.GetTotals(), pt.revisionOf(t, pt.to))
	}
}

func (pt *packTable) entries(t *testing.T, campaign string) map[string]*rulesv1.TableEntry {
	t.Helper()
	return pt.master.entries(t, campaign)
}

// TestMR025_ImportUpdatesAnEntryOfTheSameKeyKeepingItsFeatureKeys: the pack's entry replaces
// the campaign's (whose archive mark and switch stay), the preview says what changes, a
// feature that keeps its key keeps it and a new one gets its own.
func TestMR025_ImportUpdatesAnEntryOfTheSameKeyKeepingItsFeatureKeys(t *testing.T) {
	t.Parallel()
	pt := newPackTable(t)
	class := pt.master.addEntry(t, pt.to, testClass("Guardião do Vale"))
	pt.master.addEntry(t, pt.to, testRace("Anão das Brumas"))
	if _, err := pt.master.table.ArchiveTableEntry(t.Context(), connect.NewRequest(&rulesv1.ArchiveTableEntryRequest{CampaignId: pt.to, Key: class.GetKey()})); err != nil {
		t.Fatal(err)
	}
	oldKeys := featureKeysOf(bodyMessage(class))

	// The same class from another campaign, with another hit die, a feature renamed in text
	// only (sent without its key, as another campaign's file would have it) and a new feature.
	other := pt.master.addEntry(t, pt.from, testClass("Guardião do Vale"))
	if other.GetKey() != class.GetKey() {
		t.Fatalf("the keys differ: %s and %s", other.GetKey(), class.GetKey())
	}
	pack := &rulesv1.TableContentPack{Format: "meurpg.table-content", Version: 1, Entries: []*rulesv1.TableEntry{
		{Key: other.GetKey(), Kind: other.GetKind(), NamePt: other.GetNamePt(), Body: other.GetBody()},
		{Key: "race:anao-das-brumas@mesa", Kind: rulesv1.TableContentKind_TABLE_CONTENT_KIND_RACE, NamePt: "Anão das Brumas", Body: &rulesv1.TableEntry_TableRace{TableRace: testRace("Anão das Brumas")}},
	}}
	changed := proto.Clone(bodyMessage(other)).(*rulesv1.TableClass)
	changed.HitDie = 10
	changed.Levels[2].Features = []*rulesv1.TableFeature{testNote("novo", "Golpe novo", "Algo novo.")}
	pack.Entries[0].Body = &rulesv1.TableEntry_TableClass{TableClass: changed}

	pv := pt.preview(t, pt.to, pack)
	if pv.GetTotals().GetUpdated() != 1 || pv.GetTotals().GetUnchanged() != 1 {
		t.Fatalf("preview totals = %v, want one updated and one unchanged", pv.GetTotals())
	}
	if f := pv.GetEntries()[0].GetChangedFields(); !slices.Contains(f, "table_class.hit_die") || !slices.Contains(f, "table_class.levels") || slices.Contains(f, "table_class.skill_choose") {
		t.Errorf("changed fields = %v, want the hit die and the levels, and not what stayed", f)
	}
	res := pt.apply(t, pt.to, pack, "")
	if res.GetTotals().GetUpdated() != 1 {
		t.Fatalf("apply totals = %v", res.GetTotals())
	}
	got := pt.entries(t, pt.to)[class.GetKey()]
	if got.GetTableClass().GetHitDie() != 10 || !got.GetArchived() || got.GetRevision() != res.GetTableRevision() {
		t.Errorf("the updated class = hit die %d archived %v revision %d; want 10, still archived, the import's revision", got.GetTableClass().GetHitDie(), got.GetArchived(), got.GetRevision())
	}
	keys := featureKeysOf(got.GetTableClass())
	for _, k := range oldKeys { // every feature the class had keeps its key
		if !slices.Contains(keys, k) {
			t.Errorf("feature key %s was lost by the import (%v)", k, keys)
		}
	}
	if len(keys) != len(oldKeys)+1 || !strings.Contains(keys[len(keys)-1], "--golpe-novo@mesa") {
		t.Errorf("feature keys = %v, want the old ones and one new, made from the new feature's name", keys)
	}
}

// TestMR025_ImportIsAllOrNothing: one entry the rules refuse, among valid ones, stops the
// import. The preview lists every violation at its field; the apply refuses with the same
// violations and writes nothing, not even the valid entries.
func TestMR025_ImportIsAllOrNothing(t *testing.T) {
	t.Parallel()
	pt := newPackTable(t)
	pt.fill(t, pt.from)
	pack := pt.export(t, pt.from)
	// Break the feat (an unknown proficiency, a level over 20) and the spell (an unknown school).
	for _, e := range pack.GetEntries() {
		switch b := bodyMessage(e).(type) {
		case *rulesv1.TableFeat:
			b.Prerequisite.ProficiencyKey = "proficiency:nada"
			b.Prerequisite.Level = 21
		case *rulesv1.TableSpell:
			b.SchoolKey = "school:nada"
		}
	}
	before := pt.revisionOf(t, pt.to)

	pv := pt.preview(t, pt.to, pack)
	if pv.GetTotals().GetRefused() != 2 || pv.GetTotals().GetNew() != 5 {
		t.Fatalf("preview totals = %v, want five new and two refused", pv.GetTotals())
	}
	for _, e := range pv.GetEntries() {
		fields := map[string]bool{}
		for _, v := range e.GetViolations() {
			fields[v.GetField()] = true
			if v.GetKey() != e.GetKey() {
				t.Errorf("a violation of %s is attributed to %q", e.GetKey(), v.GetKey())
			}
		}
		switch e.GetKind() {
		case rulesv1.TableContentKind_TABLE_CONTENT_KIND_FEAT:
			if e.GetStatus() != rulesv1.TableImportStatus_TABLE_IMPORT_STATUS_REFUSED || !fields["table_feat.prerequisite.proficiency_key"] || !fields["table_feat.prerequisite.level"] {
				t.Errorf("the feat: status %v, violations at %v; want both prerequisite fields", e.GetStatus(), fields)
			}
		case rulesv1.TableContentKind_TABLE_CONTENT_KIND_SPELL:
			if e.GetStatus() != rulesv1.TableImportStatus_TABLE_IMPORT_STATUS_REFUSED || len(fields) == 0 {
				t.Errorf("the spell: status %v, violations at %v", e.GetStatus(), fields)
			}
		default:
			if e.GetStatus() != rulesv1.TableImportStatus_TABLE_IMPORT_STATUS_NEW {
				t.Errorf("%s is %v, want new", e.GetKey(), e.GetStatus())
			}
		}
	}

	_, err := pt.importPack(pt.master, pt.to, pack, rulesv1.TableImportMode_TABLE_IMPORT_MODE_APPLY, "all-or-nothing")
	vs := violationsOfErr(t, err)
	if len(vs) < 3 {
		t.Errorf("apply violations = %v, want every one of the two entries'", vs)
	}
	if got := pt.entries(t, pt.to); len(got) != 0 || pt.revisionOf(t, pt.to) != before || pt.hints(pt.to) != 0 {
		t.Errorf("a refused import left %d entries at revision %d (was %d), %d hints", len(got), pt.revisionOf(t, pt.to), before, pt.hints(pt.to))
	}
	// Fixed, the same key now works: the refused apply did not keep it.
	for _, e := range pack.GetEntries() {
		switch b := bodyMessage(e).(type) {
		case *rulesv1.TableFeat:
			b.Prerequisite.ProficiencyKey, b.Prerequisite.Level = "", 0
		case *rulesv1.TableSpell:
			b.SchoolKey = "school:evocation"
		}
	}
	if res := pt.apply(t, pt.to, pack, "all-or-nothing"); res.GetTotals().GetNew() != 7 {
		t.Errorf("the fixed pack: %v", res.GetTotals())
	}
}

// TestMR025_ImportRefusesWhatIsWrongWithThePackItself: the limits and the form of the pack are
// refused before the database is read, with the field of each.
func TestMR025_ImportRefusesWhatIsWrongWithThePackItself(t *testing.T) {
	t.Parallel()
	pt := newPackTable(t)
	good := func() *rulesv1.TableContentPack {
		return &rulesv1.TableContentPack{Format: "meurpg.table-content", Version: 1, Name: "x", Entries: []*rulesv1.TableEntry{
			{Key: "feat:um@mesa", Kind: rulesv1.TableContentKind_TABLE_CONTENT_KIND_FEAT, NamePt: "Um", Body: &rulesv1.TableEntry_TableFeat{TableFeat: &rulesv1.TableFeat{NamePt: "Um"}}},
		}}
	}
	feat := func(slug string) *rulesv1.TableEntry {
		return &rulesv1.TableEntry{Key: "feat:" + slug + "@mesa", Kind: rulesv1.TableContentKind_TABLE_CONTENT_KIND_FEAT, NamePt: slug, Body: &rulesv1.TableEntry_TableFeat{TableFeat: &rulesv1.TableFeat{NamePt: slug}}}
	}
	tests := []struct {
		name   string
		mutate func(*rulesv1.TableContentPack)
		field  string
		reason string
	}{
		{"another format", func(p *rulesv1.TableContentPack) { p.Format = "outro" }, "pack.format", "bad_value"},
		{"no format", func(p *rulesv1.TableContentPack) { p.Format = "" }, "pack.format", "bad_value"},
		{"another version", func(p *rulesv1.TableContentPack) { p.Version = 2 }, "pack.version", "bad_value"},
		{"a name over 80 characters", func(p *rulesv1.TableContentPack) { p.Name = strings.Repeat("a", 81) }, "pack.name", "bad_name"},
		{"a name with a line break", func(p *rulesv1.TableContentPack) { p.Name = "a\nb" }, "pack.name", "bad_name"},
		{"a key of an SRD entry", func(p *rulesv1.TableContentPack) { p.Entries[0].Key = "feat:grappler" }, "pack.entries[0].key", "bad_key"},
		{"a key of another form", func(p *rulesv1.TableContentPack) { p.Entries[0].Key = "feat:um@outra" }, "pack.entries[0].key", "bad_key"},
		{"a key with capitals", func(p *rulesv1.TableContentPack) { p.Entries[0].Key = "feat:Um@mesa" }, "pack.entries[0].key", "bad_key"},
		{"a key of another kind than the entry's", func(p *rulesv1.TableContentPack) { p.Entries[0].Key = "class:um@mesa" }, "pack.entries[0].key", "bad_key"},
		{"a key with a reserved word", func(p *rulesv1.TableContentPack) { p.Entries[0].Key = "feat:wild-shape@mesa" }, "pack.entries[0].key", "reserved_key"},
		{"no kind", func(p *rulesv1.TableContentPack) {
			p.Entries[0].Kind = rulesv1.TableContentKind_TABLE_CONTENT_KIND_UNSPECIFIED
		}, "pack.entries[0].kind", "bad_value"},
		{"no body", func(p *rulesv1.TableContentPack) { p.Entries[0].Body = nil }, "pack.entries[0].body", "bad_value"},
		{"a body of another kind", func(p *rulesv1.TableContentPack) {
			p.Entries[0].Body = &rulesv1.TableEntry_TableRace{TableRace: testRace("Um")}
		}, "pack.entries[0].body", "bad_value"},
		{"a name that is not the body's", func(p *rulesv1.TableContentPack) { p.Entries[0].NamePt = "Outro" }, "pack.entries[0].name_pt", "bad_value"},
		{"the same key twice", func(p *rulesv1.TableContentPack) { p.Entries = append(p.Entries, feat("um")) }, "pack.entries[1].key", "duplicate_key"},
		{"a pack over 3 MiB", func(p *rulesv1.TableContentPack) {
			big := strings.Repeat("a", 3000)
			for i := range 150 {
				e := feat("g" + strconv.Itoa(i))
				e.GetTableFeat().DescPt = []string{big, big, big, big, big, big, big, big}
				p.Entries = append(p.Entries, e)
			}
		}, "pack", "size_limit"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel() // each one is refused before the database is read
			pack := good()
			tc.mutate(pack)
			for _, mode := range []rulesv1.TableImportMode{rulesv1.TableImportMode_TABLE_IMPORT_MODE_PREVIEW, rulesv1.TableImportMode_TABLE_IMPORT_MODE_APPLY} {
				_, err := pt.importPack(pt.master, pt.to, pack, mode, "")
				found := false
				for _, v := range violationsOfErr(t, err) {
					found = found || (v.GetField() == tc.field && v.GetReason() == tc.reason)
				}
				if !found {
					t.Errorf("mode %v: error = %v, want a violation at %s (%s)", mode, err, tc.field, tc.reason)
				}
			}
		})
	}
	if got := pt.entries(t, pt.to); len(got) != 0 {
		t.Errorf("a refused pack wrote %d entries", len(got))
	}
}

// TestMR025_ImportRefuses1001EntriesAsAMessageAndAsAFile: 1001 entries are over the limit of
// `pack.entries` when the pack is a message (the request size limit, before the handler) and over
// the table's limit when it is a file (the pack is read by the server).
func TestMR025_ImportRefuses1001EntriesAsAMessageAndAsAFile(t *testing.T) {
	t.Parallel()
	pt := newPackTable(t)
	pack := &rulesv1.TableContentPack{Format: PackFormat, Version: PackVersion}
	for i := range rules.MaxOverlayEntries + 1 {
		slug := "f" + strconv.Itoa(i)
		pack.Entries = append(pack.Entries, &rulesv1.TableEntry{Key: "feat:" + slug + "@mesa", Kind: rulesv1.TableContentKind_TABLE_CONTENT_KIND_FEAT, NamePt: slug, Body: &rulesv1.TableEntry_TableFeat{TableFeat: &rulesv1.TableFeat{NamePt: slug}}})
	}
	file, err := protojson.Marshal(pack)
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []rulesv1.TableImportMode{rulesv1.TableImportMode_TABLE_IMPORT_MODE_PREVIEW, rulesv1.TableImportMode_TABLE_IMPORT_MODE_APPLY} {
		_, err := pt.importPack(pt.master, pt.to, pack, mode, "")
		ce, ok := errors.AsType[*connect.Error](err)
		if !ok || ce.Code() != connect.CodeInvalidArgument || len(ce.Details()) != 1 {
			t.Fatalf("mode %v, as a message: error = %v, want invalid_argument with the request limit detail", mode, err)
		}
		if d, derr := ce.Details()[0].Value(); derr != nil {
			t.Errorf("mode %v, as a message: detail: %v", mode, derr)
		} else if r, ok := d.(*systemv1.RequestRefused); !ok || r.GetField() != "pack.entries" || r.GetReason() != systemv1.RequestRefusedReason_REQUEST_REFUSED_REASON_LIST_TOO_LONG {
			t.Errorf("mode %v, as a message: detail = %v, want LIST_TOO_LONG at pack.entries", mode, d)
		}

		_, err = pt.master.table.ImportTableContent(t.Context(), connect.NewRequest(&rulesv1.ImportTableContentRequest{CampaignId: pt.to, PackJson: file, Mode: mode}))
		found := false
		for _, v := range violationsOfErr(t, err) {
			found = found || (v.GetField() == "pack.entries" && v.GetReason() == "limit")
		}
		if !found {
			t.Errorf("mode %v, as a file: error = %v, want a violation at pack.entries (limit)", mode, err)
		}
	}
	if got := pt.entries(t, pt.to); len(got) != 0 {
		t.Errorf("a refused pack wrote %d entries", len(got))
	}
}

// TestMR025_ImportHoldsTheLimitsOfTheTable: the 1000 entries and the 64 KiB per entry hold for
// the campaign as a whole, whatever the pack says about itself.
func TestMR025_ImportHoldsTheLimitsOfTheTable(t *testing.T) {
	t.Parallel()
	pt := newPackTable(t)
	feat := func(slug string, desc ...string) *rulesv1.TableEntry {
		return &rulesv1.TableEntry{
			Key: "feat:" + slug + "@mesa", Kind: rulesv1.TableContentKind_TABLE_CONTENT_KIND_FEAT, NamePt: slug,
			Body: &rulesv1.TableEntry_TableFeat{TableFeat: &rulesv1.TableFeat{NamePt: slug, DescPt: desc}},
		}
	}
	pack := func(entries ...*rulesv1.TableEntry) *rulesv1.TableContentPack {
		return &rulesv1.TableContentPack{Format: "meurpg.table-content", Version: 1, Entries: entries}
	}

	// An entry over 64 KiB of data: refused alone, with the size limit, in the preview too.
	para := strings.Repeat("a", 3900)
	var many []string
	for range 18 {
		many = append(many, para)
	}
	pv := pt.preview(t, pt.to, pack(feat("grande", many...), feat("pequeno")))
	big := pv.GetEntries()[0]
	if big.GetStatus() != rulesv1.TableImportStatus_TABLE_IMPORT_STATUS_REFUSED || big.GetViolations()[0].GetReason() != "size_limit" || pv.GetEntries()[1].GetStatus() != rulesv1.TableImportStatus_TABLE_IMPORT_STATUS_NEW {
		t.Errorf("entry over 64 KiB: %v; want it refused with size_limit and the other new", pv.GetEntries())
	}

	// 999 entries in the campaign, two more in the pack: 1001.
	var first []*rulesv1.TableEntry
	for i := range 999 {
		first = append(first, feat("f"+strconv.Itoa(i)))
	}
	pt.apply(t, pt.to, pack(first...), "")
	_, err := pt.importPack(pt.master, pt.to, pack(feat("a1"), feat("a2")), rulesv1.TableImportMode_TABLE_IMPORT_MODE_APPLY, "")
	vs := violationsOfErr(t, err)
	if len(vs) == 0 || vs[0].GetReason() != "limit" || vs[0].GetField() != "" {
		t.Errorf("1001 entries in the table: violations = %v, want the table's limit", vs)
	}
	if got := len(pt.entries(t, pt.to)); got != 999 {
		t.Errorf("the table has %d entries after a refused import, want 999", got)
	}
	// One more fits.
	if res := pt.apply(t, pt.to, pack(feat("a1")), ""); res.GetTotals().GetNew() != 1 {
		t.Errorf("the 1000th entry: %v", res.GetTotals())
	}
}

// TestMR025_ImportRetriedWithTheSameKeyAnswersTheSameAndWritesOnce: rule 9, the idempotency key
// of the import is scoped to the campaign, and the same key with another pack is refused.
func TestMR025_ImportRetriedWithTheSameKeyAnswersTheSameAndWritesOnce(t *testing.T) {
	t.Parallel()
	pt := newPackTable(t)
	pt.fill(t, pt.from)
	pack := pt.export(t, pt.from)

	first := pt.apply(t, pt.to, pack, "chave-do-import")
	hints := pt.hints(pt.to)
	retry := pt.apply(t, pt.to, pack, "chave-do-import")
	if !proto.Equal(first, retry) {
		t.Errorf("the retry answered %v, want what the first call answered %v", retry, first)
	}
	if pt.revisionOf(t, pt.to) != first.GetTableRevision() || pt.hints(pt.to) != hints {
		t.Errorf("the retry wrote: revision %d (was %d), %d hints (was %d)", pt.revisionOf(t, pt.to), first.GetTableRevision(), pt.hints(pt.to), hints)
	}

	other := proto.Clone(pack).(*rulesv1.TableContentPack)
	other.Entries = other.Entries[:1]
	if _, err := pt.importPack(pt.master, pt.to, other, rulesv1.TableImportMode_TABLE_IMPORT_MODE_APPLY, "chave-do-import"); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("the same key with another pack: error = %v, want invalid_argument", err)
	}
	// The key belongs to the campaign: the same string in another campaign is its own import.
	if res := pt.apply(t, pt.from, pack, "chave-do-import"); res.GetTotals().GetUnchanged() != 7 {
		t.Errorf("the same key in another campaign: %v", res.GetTotals())
	}
	// A key too long is refused.
	if _, err := pt.importPack(pt.master, pt.to, pack, rulesv1.TableImportMode_TABLE_IMPORT_MODE_APPLY, strings.Repeat("k", 65)); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("a 65-character key: error = %v, want invalid_argument", err)
	}
}

// TestMR025_OnlyTheMasterExportsAndImports: a player is refused, and a stranger gets
// not_found as for any campaign they are not in.
func TestMR025_OnlyTheMasterExportsAndImports(t *testing.T) {
	t.Parallel()
	pt := newPackTable(t)
	pack := &rulesv1.TableContentPack{Format: "meurpg.table-content", Version: 1}
	stranger := pt.h.newUser("Intruso")
	for who, want := range map[*user]connect.Code{pt.player: connect.CodePermissionDenied, stranger: connect.CodeNotFound} {
		if _, err := who.table.ExportTableContent(t.Context(), connect.NewRequest(&rulesv1.ExportTableContentRequest{CampaignId: pt.to})); connect.CodeOf(err) != want {
			t.Errorf("ExportTableContent: error = %v, want %v", err, want)
		}
		for _, mode := range []rulesv1.TableImportMode{rulesv1.TableImportMode_TABLE_IMPORT_MODE_PREVIEW, rulesv1.TableImportMode_TABLE_IMPORT_MODE_APPLY} {
			if _, err := pt.importPack(who, pt.to, pack, mode, ""); connect.CodeOf(err) != want {
				t.Errorf("ImportTableContent(%v): error = %v, want %v", mode, err, want)
			}
		}
	}
	if _, err := pt.importPack(pt.master, pt.to, pack, rulesv1.TableImportMode_TABLE_IMPORT_MODE_UNSPECIFIED, ""); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("no mode: error = %v, want invalid_argument", err)
	}
	if _, err := pt.importPack(pt.master, pt.to, nil, rulesv1.TableImportMode_TABLE_IMPORT_MODE_PREVIEW, ""); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("no pack: error = %v, want invalid_argument", err)
	}
}

// TestMR025_AnImportedEntryFollowsTheRulesOfAHandMadeOne: it is on for the players by default and the
// master can switch it off, then no player reads it; a retired one is never sent to them; an
// open sheet hears of the new content; and the cache serves the new revision.
func TestMR025_AnImportedEntryFollowsTheRulesOfAHandMadeOne(t *testing.T) {
	t.Parallel()
	pt := newPackTable(t)
	const visible, hidden = "LEAKCANARY-imported-feat-1", "LEAKCANARY-imported-feat-2"
	feat := func(slug, marker string) *rulesv1.TableEntry {
		return &rulesv1.TableEntry{
			Key: "feat:" + slug + "@mesa", Kind: rulesv1.TableContentKind_TABLE_CONTENT_KIND_FEAT, NamePt: slug,
			Body: &rulesv1.TableEntry_TableFeat{TableFeat: &rulesv1.TableFeat{NamePt: slug, DescPt: []string{marker}}},
		}
	}
	pack := &rulesv1.TableContentPack{Format: "meurpg.table-content", Version: 1, Entries: []*rulesv1.TableEntry{feat("visivel", visible), feat("escondido", hidden)}}
	res := pt.apply(t, pt.to, pack, "")

	read := func() string {
		got, err := pt.player.table.ListTableEntries(t.Context(), connect.NewRequest(&rulesv1.ListTableEntriesRequest{CampaignId: pt.to}))
		if err != nil {
			t.Fatal(err)
		}
		return got.Msg.String()
	}
	if text := read(); !strings.Contains(text, visible) || !strings.Contains(text, hidden) {
		t.Fatalf("a new imported entry is on for the players by default; the player read %q", text) // the positive control
	}
	if _, err := pt.master.table.SetOptionSwitches(t.Context(), connect.NewRequest(&rulesv1.SetOptionSwitchesRequest{
		CampaignId: pt.to, Switches: []*rulesv1.OptionSwitch{{Key: "feat:escondido@mesa", Off: true}},
	})); err != nil {
		t.Fatal(err)
	}
	if text := read(); !strings.Contains(text, visible) || strings.Contains(text, hidden) || strings.Contains(text, "escondido") {
		t.Errorf("after switching one off, the player read %q", text)
	}
	if _, err := pt.master.table.ArchiveTableEntry(t.Context(), connect.NewRequest(&rulesv1.ArchiveTableEntryRequest{CampaignId: pt.to, Key: "feat:visivel@mesa"})); err != nil {
		t.Fatal(err)
	}
	if text := read(); strings.Contains(text, visible) || strings.Contains(text, hidden) {
		t.Errorf("after retiring the other, the player read %q", text)
	}
	// Importing the same pack again neither undoes the switch nor the archive.
	pt.apply(t, pt.to, pack, "")
	if text := read(); strings.Contains(text, visible) || strings.Contains(text, hidden) {
		t.Errorf("after importing again, the player read %q", text)
	}
	entries := pt.entries(t, pt.to)
	if !entries["feat:escondido@mesa"].GetOff() || !entries["feat:visivel@mesa"].GetArchived() {
		t.Error("the master lost the switch or the archive mark by importing the same pack")
	}
	if pt.hints(pt.to) < 1 || res.GetTableRevision() < 1 {
		t.Error("the import sent no content_changed")
	}
}

// TestMR025_ImportRefusesAFieldThePackDoesNotKnow: a pack written with a misspelled field
// ("proficiency" for "proficiency_key") is read strictly. The entry is refused with the path of
// the field, in the preview too, and nothing is written; the field is never silently dropped.
func TestMR025_ImportRefusesAFieldThePackDoesNotKnow(t *testing.T) {
	t.Parallel()
	pt := newPackTable(t)
	file := func(feat string) []byte {
		return []byte(`{"format": "meurpg.table-content", "version": 1, "name": "x", "entries": [
			{"key": "feat:certo@mesa", "kind": "TABLE_CONTENT_KIND_FEAT", "name_pt": "Certo", "table_feat": {"name_pt": "Certo", "prerequisite": {"proficiencyKey": "proficiency:medium-armor", "level": 4}}},
			{"key": "feat:errado@mesa", "kind": "TABLE_CONTENT_KIND_FEAT", "name_pt": "Errado", "table_feat": ` + feat + `}]}`)
	}
	send := func(raw []byte, mode rulesv1.TableImportMode) (*rulesv1.ImportTableContentResponse, error) {
		out, err := pt.master.table.ImportTableContent(pt.h.t.Context(), connect.NewRequest(&rulesv1.ImportTableContentRequest{CampaignId: pt.to, PackJson: raw, Mode: mode}))
		if err != nil {
			return nil, err
		}
		return out.Msg, nil
	}

	// Both spellings of a known field are read (the proto name and the JSON name).
	ok, err := send(file(`{"name_pt": "Errado", "prerequisite": {"proficiency_key": "proficiency:light-armor"}}`), rulesv1.TableImportMode_TABLE_IMPORT_MODE_PREVIEW)
	if err != nil || ok.GetTotals().GetNew() != 2 || ok.GetTotals().GetRefused() != 0 {
		t.Fatalf("a correct file: %v, %v; want two new entries", ok.GetTotals(), err)
	}

	typo := file(`{"name_pt": "Errado", "prerequisite": {"proficiency": "proficiency:light-armor", "levl": 4}, "efects": []}`)
	res, err := send(typo, rulesv1.TableImportMode_TABLE_IMPORT_MODE_PREVIEW)
	if err != nil {
		t.Fatal(err)
	}
	if res.GetTotals().GetNew() != 1 || res.GetTotals().GetRefused() != 1 {
		t.Fatalf("totals = %v, want the right entry new and the misspelled one refused", res.GetTotals())
	}
	bad := res.GetEntries()[1]
	fields := map[string]bool{}
	for _, v := range bad.GetViolations() {
		if v.GetReason() != "unknown_field" || v.GetKey() != "feat:errado@mesa" {
			t.Errorf("violation = %v, want unknown_field of the entry", v)
		}
		fields[v.GetField()] = true
	}
	for _, want := range []string{"table_feat.prerequisite.proficiency", "table_feat.prerequisite.levl", "table_feat.efects"} {
		if !fields[want] {
			t.Errorf("no violation at %s (have %v)", want, fields)
		}
	}
	if _, err := send(typo, rulesv1.TableImportMode_TABLE_IMPORT_MODE_APPLY); len(violationsOfErr(t, err)) != 3 {
		t.Errorf("apply error = %v, want the three unknown fields", err)
	}
	if got := pt.entries(t, pt.to); len(got) != 0 {
		t.Errorf("a refused import wrote %d entries", len(got))
	}

	// A field outside any entry is a violation of the pack; garbage and both forms are refused.
	_, err = send([]byte(`{"format": "meurpg.table-content", "version": 1, "nome": "x"}`), rulesv1.TableImportMode_TABLE_IMPORT_MODE_PREVIEW)
	if vs := violationsOfErr(t, err); len(vs) != 1 || vs[0].GetField() != "pack.nome" || vs[0].GetReason() != "unknown_field" {
		t.Errorf("a top-level typo: %v", vs)
	}
	if _, err := send([]byte(`not json`), rulesv1.TableImportMode_TABLE_IMPORT_MODE_PREVIEW); len(violationsOfErr(t, err)) != 1 {
		t.Errorf("garbage: error = %v", err)
	}
	_, err = pt.master.table.ImportTableContent(t.Context(), connect.NewRequest(&rulesv1.ImportTableContentRequest{
		CampaignId: pt.to, PackJson: file(`{}`), Pack: &rulesv1.TableContentPack{}, Mode: rulesv1.TableImportMode_TABLE_IMPORT_MODE_PREVIEW,
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("pack and pack_json together: error = %v, want invalid_argument", err)
	}

	// An exported pack, as the file the app saves, goes back in whole.
	pt.fill(t, pt.from)
	file2, err := storeJSON.Marshal(pt.export(t, pt.from))
	if err != nil {
		t.Fatal(err)
	}
	if res, err := send(file2, rulesv1.TableImportMode_TABLE_IMPORT_MODE_APPLY); err != nil || res.GetTotals().GetNew() != 7 {
		t.Errorf("the exported file: %v, %v; want seven new", res.GetTotals(), err)
	}
}

// TestMR025_ImportPreviewReportsEveryViolationOfAnEntry: an entry with several things wrong
// (a prerequisite, an effect, a duplicate name) lists all of them, and so does every other
// refused entry of the pack.
func TestMR025_ImportPreviewReportsEveryViolationOfAnEntry(t *testing.T) {
	t.Parallel()
	pt := newPackTable(t)
	pt.master.addEntry(t, pt.to, &rulesv1.TableFeat{NamePt: "Repetido", DescPt: []string{"x"}})
	feat := func(slug, name string, f *rulesv1.TableFeat) *rulesv1.TableEntry {
		f.NamePt = name
		return &rulesv1.TableEntry{Key: "feat:" + slug + "@mesa", Kind: rulesv1.TableContentKind_TABLE_CONTENT_KIND_FEAT, NamePt: name, Body: &rulesv1.TableEntry_TableFeat{TableFeat: f}}
	}
	worse := &rulesv1.TableFeat{
		Prerequisite: &rulesv1.FeatPrerequisite{ProficiencyKey: "proficiency:nada", Level: 21, RaceKey: "race:nada"},
		Effects: []*rulesv1.TableEffect{
			{Type: "modifier", Target: "ac", Mode: "add", Value: "1 +"},
			{Type: "ability_increase", Count: 3, From: []string{"str"}, Value: "1"},
		},
	}
	pack := &rulesv1.TableContentPack{Format: "meurpg.table-content", Version: 1, Entries: []*rulesv1.TableEntry{
		feat("a", "Repetido", worse),
		feat("b", "Outro", &rulesv1.TableFeat{Prerequisite: &rulesv1.FeatPrerequisite{Level: 30}}),
	}}
	res := pt.preview(t, pt.to, pack)
	fields := func(e *rulesv1.TableImportEntry) []string {
		var out []string
		for _, v := range e.GetViolations() {
			out = append(out, v.GetField())
		}
		return out
	}
	got := fields(res.GetEntries()[0])
	for _, want := range []string{
		"table_feat.name_pt", "table_feat.prerequisite.proficiency_key", "table_feat.prerequisite.level", "table_feat.prerequisite.race_key",
		"table_feat.effects[0].value", "table_feat.effects[1].count",
	} {
		if !slices.Contains(got, want) {
			t.Errorf("the first entry's violations lack %s (have %v)", want, got)
		}
	}
	if got := fields(res.GetEntries()[1]); !slices.Contains(got, "table_feat.prerequisite.level") {
		t.Errorf("the second entry's violations = %v", got)
	}
}

// TestMR025_APackWithOptionsPreviewsAndImports: a pack carries options of two SRD sets under
// their own keys ("option:<set>:<slug>@mesa"), and both the preview and the import accept them.
func TestMR025_APackWithOptionsPreviewsAndImports(t *testing.T) {
	t.Parallel()
	pt := newPackTable(t)
	guard := &rulesv1.TableOption{
		NamePt: "Guarda Dupla", DescPt: []string{"Escudo e arma na mão."}, Set: "fighting_style",
		Effects: []*rulesv1.TableEffect{{Type: "modifier", Target: "ac", Mode: "add", Value: "1", When: "shield()"}},
	}
	pack := &rulesv1.TableContentPack{Format: "meurpg.table-content", Version: 1, Entries: []*rulesv1.TableEntry{
		{Key: "option:invocation:olhar-do-corvo@mesa", Kind: rulesv1.TableContentKind_TABLE_CONTENT_KIND_OPTION, NamePt: "Olhar do Corvo", Body: &rulesv1.TableEntry_TableOption{TableOption: crowGaze()}},
		{Key: "option:fighting_style:guarda-dupla@mesa", Kind: rulesv1.TableContentKind_TABLE_CONTENT_KIND_OPTION, NamePt: "Guarda Dupla", Body: &rulesv1.TableEntry_TableOption{TableOption: guard}},
	}}
	pv := pt.preview(t, pt.to, pack)
	if pv.GetTotals().GetNew() != 2 || pv.GetTotals().GetRefused() != 0 {
		t.Fatalf("preview totals = %v, entries %v; want two new entries and no refusal", pv.GetTotals(), pv.GetEntries())
	}
	res := pt.apply(t, pt.to, pack, "options-1")
	if res.GetTotals().GetNew() != 2 {
		t.Fatalf("import totals = %v, want two new entries", res.GetTotals())
	}
	got := pt.entries(t, pt.to)
	for _, key := range []string{"option:invocation:olhar-do-corvo@mesa", "option:fighting_style:guarda-dupla@mesa"} {
		if got[key] == nil {
			t.Errorf("the campaign has no %s after the import; it has %d entries", key, len(got))
		}
	}
}
