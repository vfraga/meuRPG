package play

import (
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The release of a readied spell is a cast of the spell Ready held, as Ready held it: it names no grant (MR-025). The web
// never sends granted_by on a release, but CastSpell takes it, and a readied cantrip has no slot, so a grant would spend
// one of its uses. These tests need the database (MEURPG_TEST_DATABASE_URL).

// A readied Poison Spray (a cantrip of Pensantus) released with the granted_by of a limited grant of the same cantrip is
// refused, spends no use of the grant, and leaves the readied row waiting.
func TestMR025_AReadiedCantripReleasedWithAGrantedByIsRefusedAndSpendsNoUse(t *testing.T) {
	t.Parallel()
	a, feat := newLevelParty(t, poisonSpray, 0)
	sheet := proto.Clone(a.pens.GetSheet()).(*charactersv1.CharacterSheet)
	sheet.GetFull().CantripKeys = append(sheet.GetFull().GetCantripKeys(), poisonSpray)
	upd, err := a.master.characters.UpdateCharacter(t.Context(), connect.NewRequest(&charactersv1.UpdateCharacterRequest{
		CampaignId: a.campaignID, CharacterId: a.pens.GetId(), Revision: a.pens.GetRevision(), Name: "Pensantus", Sheet: sheet,
	}))
	if err != nil {
		t.Fatalf("UpdateCharacter(Pensantus learns Poison Spray) error = %v", err)
	}
	a.pens = upd.Msg.GetCharacter()
	e := a.readyForRelease(t, poisonSpray, 0, "Capitão Goblin")
	a.mustFire(t, e, "Pensantus")
	_, err = a.cast(t, a.ana, e, "Pensantus", poisonSpray, nil, a.at(t, "Capitão Goblin"), func(r *playv1.CastSpellRequest) { grantedCast(feat)(r); noCastRoll(r) })
	wantRefusal(t, "release of a readied cantrip with granted_by", err, connect.CodeInvalidArgument, "a readied spell is released from the source it was readied with: name no grant")
	if used, _ := resourceUsed(a.vitals(t, a.pens), graceUses); used != 0 {
		t.Errorf("uses of the grant after the refused release = %d, want 0", used)
	}
	if got := a.readiedStatus(t, "Pensantus"); got != readiedWaiting {
		t.Errorf("rows after the refused release = %q, want waiting", got)
	}
}
