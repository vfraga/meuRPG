package characters

import (
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// #580 S2 through the service: the master edits a table weapon a locked sheet carries. The numbers follow at once
// on the owner's read; and no "A arma mudou" banner shows, because a banner needs a new issue tied to the weapon and a
// weapon edit makes none (the only weapon issue is a key the content lacks, and an archived entry stays readable for
// the sheet that carries it). If a weapon issue is ever added, this is where its banner is asserted.
func TestAWeaponEditReachesTheSheetThatCarriesIt(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, owner := h.newUser("Samuel"), h.newUser("Dona")
	campaign := h.newCampaign(master, "Mirathel", owner)
	weapon := master.addEntry(t, campaign, testWeapon("Lâmina de teste"))

	sheet := pensantusSheet()
	sheet.GetFull().WeaponKeys = append(sheet.GetFull().WeaponKeys, weapon.GetKey())
	pc := owner.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Pensantus", sheet)
	if h.lockSheets(campaign) != 1 {
		t.Fatal("expected one sheet to lock")
	}
	damageOf := func() (string, int) {
		d := owner.get(t, campaign, pc.GetId()).GetDerived()
		for _, a := range d.GetAttacks() {
			if a.GetName() == "Lâmina de teste" {
				return a.GetDamage(), len(d.GetChangedContent())
			}
		}
		return "", len(d.GetChangedContent())
	}
	if dmg, _ := damageOf(); !strings.HasPrefix(dmg, "1d8") {
		t.Fatalf("damage before the edit = %q, want 1d8", dmg)
	}
	edited := proto.Clone(bodyMessage(weapon)).(*rulesv1.TableWeapon)
	edited.Damage = "1d12"
	if _, err := master.table.UpdateTableEntry(t.Context(), connect.NewRequest(updateReq(campaign, weapon.GetKey(), weapon.GetRevision(), edited))); err != nil {
		t.Fatalf("UpdateTableEntry() error = %v", err)
	}
	if dmg, changed := damageOf(); !strings.HasPrefix(dmg, "1d12") || changed != 0 {
		t.Errorf("after the edit: damage %q, %d changed entries; want 1d12 and no banner (no new issue)", dmg, changed)
	}
}
