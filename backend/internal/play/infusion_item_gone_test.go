package play

import (
	"encoding/json"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/infusion"
)

// The whole stack, with an open session: a sheet save that takes an infused piece off the sheet ends its record in the characters
// module, and the characters module tells play, which writes an infusion_ended session event with the reason item_gone.
func TestASheetSaveRemovingAnInfusedPieceWritesAnInfusionEndedEvent(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	var record string
	if err := a.h.pool.QueryRow(t.Context(), `
		INSERT INTO character_infusions (campaign_id, artificer_character_id, holder_character_id, infusion_key, item_kind, item_key, since)
		VALUES ($1, $2, $2, $3, 'gear', $4, now()) RETURNING id::TEXT`,
		a.campaignID, a.toren.GetId(), infusionKeyOfTheTests, battleaxe).Scan(&record); err != nil {
		t.Fatalf("keep an infusion on the battleaxe: %v", err)
	}

	cur, err := a.master.characters.GetCharacter(t.Context(), connect.NewRequest(&charactersv1.GetCharacterRequest{CampaignId: a.campaignID, CharacterId: a.toren.GetId()}))
	if err != nil {
		t.Fatalf("GetCharacter() error = %v", err)
	}
	sheet := proto.CloneOf(cur.Msg.GetCharacter().GetSheet())
	sheet.GetFull().WeaponKeys = nil
	if _, err := a.master.characters.UpdateCharacter(t.Context(), connect.NewRequest(&charactersv1.UpdateCharacterRequest{
		CampaignId: a.campaignID, CharacterId: a.toren.GetId(), Revision: cur.Msg.GetCharacter().GetRevision(), Name: a.toren.GetName(), Sheet: sheet,
	})); err != nil {
		t.Fatalf("UpdateCharacter(without the battleaxe) error = %v", err)
	}

	rows := a.infusionRows(t)
	if len(rows) != 1 || rows[0].Kind != "infusion_ended" || rows[0].Character != a.toren.GetId() || rows[0].Actor != a.master.id || rows[0].Encounter != nil {
		t.Fatalf("the save wrote %+v, want one infusion_ended about Toren by the master, with no combat", rows)
	}
	var got infusion.Event
	if err := json.Unmarshal(rows[0].Payload, &got); err != nil {
		t.Fatal(err)
	}
	want := infusion.Event{
		RecordID: record, ArtificerID: a.toren.GetId(), HolderID: a.toren.GetId(), Infusion: infusionKeyOfTheTests,
		ItemKind: "gear", ItemKey: battleaxe, Ended: "item_gone",
	}
	if got != want {
		t.Errorf("the event = %+v, want %+v", got, want)
	}
}
