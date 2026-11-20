package play

import (
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// Two granted_spell effects of one spell in two table entries, at different cast levels (MR-025): each option casts at
// its own level and spends the use of its own entry. These tests need the database (MEURPG_TEST_DATABASE_URL).

const secondUses = "usos_da_segunda_graca"

// newTwoGrants is newLevelParty (a grant of Burning Hands at the 2nd level, one use of graceUses) with a second feat the
// wizard also has: Burning Hands at the 4th level, one use of its own resource.
func newTwoGrants(t *testing.T) (a *armed, first, second string) {
	t.Helper()
	a, first = newLevelParty(t, burningHands, 2)
	res, err := a.master.table.CreateTableEntry(t.Context(), connect.NewRequest(&rulesv1.CreateTableEntryRequest{
		CampaignId: a.campaignID, Body: &rulesv1.CreateTableEntryRequest_TableFeat{TableFeat: &rulesv1.TableFeat{
			NamePt: "Segunda graça de teste", DescPt: []string{"Outro dom de teste."},
			Effects: []*rulesv1.TableEffect{
				{Type: "resource", Resource: secondUses, Max: "1", Recharge: "long_rest"},
				{Type: "granted_spell", Spell: burningHands, Ability: "cha", Uses: secondUses, CastLevel: 4},
			},
		}},
	}))
	if err != nil {
		t.Fatalf("CreateTableEntry(second feat) error = %v", err)
	}
	second = res.Msg.GetEntry().GetKey()
	sheet := proto.Clone(a.pens.GetSheet()).(*charactersv1.CharacterSheet)
	sheet.GetFull().FeatKeys = append(sheet.GetFull().GetFeatKeys(), second)
	upd, err := a.master.characters.UpdateCharacter(t.Context(), connect.NewRequest(&charactersv1.UpdateCharacterRequest{
		CampaignId: a.campaignID, CharacterId: a.pens.GetId(), Revision: a.pens.GetRevision(), Name: "Pensantus", Sheet: sheet,
	}))
	if err != nil {
		t.Fatalf("UpdateCharacter(Pensantus gets the second feat) error = %v", err)
	}
	a.pens = upd.Msg.GetCharacter()
	return a, first, second
}

// In a combat, the two grants of Burning Hands cast at their own levels (4d6 at the 2nd level, 6d6 at the 4th) and each spends the use of its own resource.
func TestMR025_TwoGrantsOfOneSpellInACombatCastEachAtItsOwnLevel(t *testing.T) {
	t.Parallel()
	a, first, second := newTwoGrants(t)
	e, targets := a.burnFight(t)
	cast := a.mustCast(t, a.ana, e, "Pensantus", burningHands, nil, targets, grantedCast(second))
	if p := cast.GetCast().GetPendingDamages()[0]; p.GetDiceCount() != 6 {
		t.Errorf("the second grant rolls %dd%d, want 6d6 (the 4th level)", p.GetDiceCount(), p.GetDiceSides())
	}
	if used, _ := resourceUsed(a.vitals(t, a.pens), secondUses); used != 1 {
		t.Errorf("uses of the second grant = %d, want 1", used)
	}
	if used, _ := resourceUsed(a.vitals(t, a.pens), graceUses); used != 0 {
		t.Errorf("uses of the first grant = %d, want 0", used)
	}
	e = a.ownTurnAgain(t, e, "Pensantus")
	a.h.roller.queue(5, 18, 12)
	cast = a.mustCast(t, a.ana, e, "Pensantus", burningHands, nil, targets, grantedCast(first))
	if p := cast.GetCast().GetPendingDamages()[0]; p.GetDiceCount() != 4 {
		t.Errorf("the first grant rolls %dd%d, want 4d6 (the 2nd level)", p.GetDiceCount(), p.GetDiceSides())
	}
	if used, _ := resourceUsed(a.vitals(t, a.pens), graceUses); used != 1 {
		t.Errorf("uses of the first grant = %d, want 1", used)
	}
	noSlotSpent(t, a)
}
