package characters

import (
	"testing"
	"time"

	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/campaignpackage"
)

// A package is a file anyone may edit: the revision a weapon last became two-handed at is the server's own, never read from
// it. The staged weapon takes the revision its row is stored at (1: a package makes a new campaign's table) when it takes both
// hands, and 0 when it does not.
func TestAPackageStagesATwoHandedWeaponAtTheRevisionOfItsRowNotTheFilesOwn(t *testing.T) {
	t.Parallel()
	stage := func(w *rulesv1.TableWeapon) *rulesv1.TableWeapon {
		t.Helper()
		key := entryKey(rulesv1.TableContentKind_TABLE_CONTENT_KIND_WEAPON, w, nil)
		in := campaignpackage.NewImport(packagePartOpen(t, nil), "00000000-0000-4000-8000-0000000000c1", "u", false, nil)
		row, ok := (&packagePart{s: &Service{now: time.Now}}).stageEntry(in, packageEntryOf(rulesv1.TableContentKind_TABLE_CONTENT_KIND_WEAPON, key, w), time.Now(), map[string]bool{})
		if !ok {
			t.Fatalf("the package refuses the weapon: %v", in.Problems.List)
		}
		return row.body.(*rulesv1.TableWeapon)
	}

	both := testWeapon("Machado de Lenhador")
	twoHanded(both)
	both.TwoHandedSinceRevision = 99
	if got := stage(both).GetTwoHandedSinceRevision(); got != 1 {
		t.Errorf("a two-handed weapon staged since = %d, want 1 (the file's 99 is ignored)", got)
	}

	one := testWeapon("Espada de Mesa")
	one.TwoHandedSinceRevision = 99
	if got := stage(one).GetTwoHandedSinceRevision(); got != 0 {
		t.Errorf("a weapon that is not two-handed staged since = %d, want 0", got)
	}
}
