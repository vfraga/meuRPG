package characters

import (
	"testing"

	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	pkgv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaignpackage/v1"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/campaignpackage"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
)

// TestMR578_ATableCreatureAndTheNpcMadeFromItTravelInACampaignPackage: the characters' part of a campaign
// package carries a table creature and an NPC made from it; the stage compiles the content first, so the NPC's
// link is kept; without the creature in the package, or with a creature whose base does not exist, the
// package is refused.
func TestMR578_ATableCreatureAndTheNpcMadeFromItTravelInACampaignPackage(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ctx := t.Context()
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master, "Mirathel")
	master.addEntry(t, campaign, tableWolf())
	if _, err := npcFromCreature(t, master, campaign, tableWolfKey, "Presa", charactersv1.CharacterKind_CHARACTER_KIND_MINION, "00000000-0000-4000-8000-0000000000b2"); err != nil {
		t.Fatalf("npcFromCreature(table creature) error = %v", err)
	}

	part := h.svc.PackagePart()
	snap := campaignpackage.NewSnapshot(campaign)
	if err := db.InTx(ctx, h.pool, func(tx pgx.Tx) error { return part.Export(ctx, tx, campaign, snap) }); err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	base := packagePartSnapshotEntries(t, snap)
	stage := func(entries []packagePartEntry) *campaignpackage.Import {
		t.Helper()
		in := campaignpackage.NewImport(packagePartOpen(t, entries), h.newCampaign(master, "Destino"), master.id, false, nil)
		if err := part.Stage(ctx, in); err != nil {
			t.Fatalf("Stage() error = %v", err)
		}
		return in
	}

	in := stage(base)
	chars, _ := in.Get(stashCharacters).(*stagedCharacters)
	if len(in.Problems.List) != 0 || chars == nil || len(chars.npcs) != 1 {
		t.Fatalf("staging the exported campaign: problems = %v, characters = %v; want the NPC and no problem", in.Problems.List, chars)
	}
	sheet := &charactersv1.CharacterSheet{}
	if err := protojson.Unmarshal(chars.npcs[0].sheet, sheet); err != nil || sheet.GetBasic().GetMonsterKey() != tableWolfKey {
		t.Errorf("the staged NPC's creature = %q, %v; want %q", sheet.GetBasic().GetMonsterKey(), err, tableWolfKey)
	}

	content := pkgv1.PackageEntryKind_PACKAGE_ENTRY_KIND_CONTENT
	// The package without the creature: the NPC's link is to a creature nobody brings.
	in = stage(packagePartWith(base, content))
	if len(in.Problems.List) == 0 {
		t.Error("an NPC made from a table creature the package does not carry was accepted")
	}
	// The creature with a base that does not exist: the content is refused.
	var broken []packagePartEntry
	for _, e := range base {
		if e.kind == content {
			pc := &pkgv1.PackageContent{}
			if err := protojson.Unmarshal(e.data, pc); err != nil {
				t.Fatal(err)
			}
			pc.GetEntry().GetTableCreature().BaseKey = "monster:nao-existe"
			e = packagePartContent(t, e.name, proto.CloneOf(pc.GetEntry()))
		}
		broken = append(broken, e)
	}
	in = stage(broken)
	if len(in.Problems.List) == 0 || in.Problems.List[0].GetKind() != pkgv1.PackageProblemKind_PACKAGE_PROBLEM_KIND_CONTENT {
		t.Errorf("a creature with a dangling base: problems = %v, want a content problem", in.Problems.List)
	}
}
