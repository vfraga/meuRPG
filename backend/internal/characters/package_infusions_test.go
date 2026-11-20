package characters

import (
	"testing"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
)

// A character of a package comes with no infusions: the export reads the stored sheet, which never holds them, and the
// import drops the list from a sheet a hand-made package brings.
func TestCheckNewSheetDropsTheInfusionsOfAPackageSheet(t *testing.T) {
	t.Parallel()
	sheet := pensantusSheet()
	sheet.GetFull().Infusions = []*charactersv1.SheetInfusion{{Id: "x", InfusionKey: "option:enhanced-defense@mesa", ItemKey: "equipment:shield"}}
	checked, err := checkNewSheet(loadRules(t), authz.Membership{Role: authz.RoleMaster}, kindPlayer, sheet)
	if err != nil {
		t.Fatalf("checkNewSheet() error = %v", err)
	}
	if got := checked.GetFull().GetInfusions(); len(got) != 0 {
		t.Errorf("the checked sheet has %d infusions, want none", len(got))
	}
}
