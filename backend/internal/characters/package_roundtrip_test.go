package characters

import (
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	pkgv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaignpackage/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/campaignpackage"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
)

// A campaign with one entry of every kind of table content, a feat with options of its own and a
// warlock who picked a table invocation exports as a package that imports without a problem: the new
// campaign has the same entries under the same keys, the same feature keys inside them, and the sheet
// keeps its pick.
func TestAPackageOfEveryKindOfTableContentImportsAsItWasExported(t *testing.T) {
	t.Parallel()
	tb := newChoiceTable(t)
	m, ctx := tb.master, t.Context()

	keys := map[string]string{}
	var kinds []rulesv1.TableContentKind
	add := func(kind rulesv1.TableContentKind, prefix string, body tableBody) {
		key := m.addEntry(t, tb.campaign, body).GetKey()
		keys[prefix] = key
		kinds = append(kinds, kind)
	}
	for _, tk := range tableKinds {
		for _, body := range packageBodies(tk.prefix, keys) {
			add(tk.kind, tk.prefix, body)
		}
	}
	add(rulesv1.TableContentKind_TABLE_CONTENT_KIND_FEAT, "feat", resilientFeat())
	option := "option:invocation:olhar-do-corvo@mesa"
	invoke := []string{"feature:eldritch-invocation-armor-of-shadows", "feature:eldritch-invocation-beast-speech"}
	warlock, err := tb.create(tb.ana, warlockSheet(5, tomePact, option, invoke[0], invoke[1]))
	if err != nil {
		t.Fatalf("a warlock with the table invocation: %v", err)
	}

	before := m.entries(t, tb.campaign)
	if len(before) != len(kinds) {
		t.Fatalf("the campaign has %d entries, want %d", len(before), len(kinds))
	}
	for _, tk := range tableKinds {
		if !slices.ContainsFunc(mapValues(before), func(e *rulesv1.TableEntry) bool { return e.GetKind() == tk.kind }) {
			t.Errorf("no entry of the kind %s", tk.prefix)
		}
	}

	part := tb.h.svc.PackagePart()
	snap := campaignpackage.NewSnapshot(tb.campaign)
	if err := db.InTx(ctx, tb.h.pool, func(tx pgx.Tx) error { return part.Export(ctx, tx, tb.campaign, snap) }); err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	exported := packagePartSnapshotEntries(t, snap)
	stage := func(files []packagePartEntry) *campaignpackage.Import {
		t.Helper()
		in := campaignpackage.NewImport(packagePartOpen(t, files), tb.h.newCampaign(m, "Destino"), m.id, false, nil)
		if err := part.Stage(ctx, in); err != nil {
			t.Fatalf("Stage() error = %v", err)
		}
		return in
	}

	in := stage(exported)
	if len(in.Problems.List) != 0 {
		t.Fatalf("the preview of the exported campaign has problems: %v", in.Problems.List)
	}
	if err := db.InTx(ctx, tb.h.pool, func(tx pgx.Tx) error { return part.Apply(ctx, tx, in) }); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	after := m.entries(t, in.CampaignID)
	if len(after) != len(before) {
		t.Errorf("the new campaign has %d entries, want %d", len(after), len(before))
	}
	for key, want := range before {
		got := after[key]
		if got == nil {
			t.Errorf("the new campaign has no entry %s", key)
			continue
		}
		wantBody, _ := bodyOf(want)
		gotBody, gotKind := bodyOf(got)
		if gotKind != want.GetKind() || got.GetNamePt() != want.GetNamePt() || !proto.Equal(gotBody, wantBody) {
			t.Errorf("the entry %s in the new campaign = %v, want %v", key, got, want)
		}
	}

	var doc []byte
	if err := tb.h.pool.QueryRow(ctx, `SELECT sheet FROM characters WHERE campaign_id = $1 AND name = $2`, in.CampaignID, warlock.GetName()).Scan(&doc); err != nil {
		t.Fatalf("read the imported warlock: %v", err)
	}
	sheet, err := loadSheet(warlock.GetId(), doc)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(sheet.GetFull().GetFeatureChoiceKeys(), option) {
		t.Errorf("the imported warlock's picks = %v, want %s among them", sheet.GetFull().GetFeatureChoiceKeys(), option)
	}

	// An option whose body names another set than its key does is refused, not carried.
	var mismatched []packagePartEntry
	for _, e := range exported {
		if e.kind == pkgv1.PackageEntryKind_PACKAGE_ENTRY_KIND_CONTENT {
			pc := &pkgv1.PackageContent{}
			if err := protojson.Unmarshal(e.data, pc); err != nil {
				t.Fatal(err)
			}
			if pc.GetEntry().GetKey() == option {
				pc.GetEntry().GetTableOption().Set = "fighting_style"
				e = packagePartContent(t, e.name, pc.GetEntry())
			}
		}
		mismatched = append(mismatched, e)
	}
	got := stage(mismatched)
	if len(got.Problems.List) != 1 || got.Problems.List[0].GetName() != "Olhar do Corvo" ||
		got.Problems.List[0].GetReason() != pkgv1.PackageProblemReason_PACKAGE_PROBLEM_REASON_INVALID {
		t.Errorf("an option whose set is not the one in its key: problems = %v, want one INVALID problem for it", got.Problems.List)
	}
}
