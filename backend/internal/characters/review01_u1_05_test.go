package characters

import (
	"testing"
	"uuid"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
)

// Finding U1-05: the create key is scoped to the campaign, not the caller, so member B
// reusing member A's key with an identical body gets A's character back.
func TestReview1_CreateKeyIsPerCaller(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, a, b := h.newUser("Mestre"), h.newUser("A"), h.newUser("B")
	campaign := h.newCampaign(master, "Mirathel", a, b)

	key := uuid.New().String()
	req := &charactersv1.CreateCharacterRequest{
		CampaignId: campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Pensantus", Sheet: pensantusSheet(), IdempotencyKey: key,
	}
	resA, err := a.api.CreateCharacter(t.Context(), connect.NewRequest(req))
	if err != nil {
		t.Fatalf("A CreateCharacter() error = %v", err)
	}
	resB, err := b.api.CreateCharacter(t.Context(), connect.NewRequest(req))
	if err != nil {
		t.Fatalf("B CreateCharacter() error = %v", err)
	}
	got := resB.Msg.GetCharacter()
	if got.GetId() == resA.Msg.GetCharacter().GetId() {
		t.Errorf("B got A's character %q (player_user_id %q): B's create was dropped", got.GetId(), got.GetPlayerUserId())
	}
	if got.GetPlayerUserId() != b.id {
		t.Errorf("B's character player_user_id = %q, want B's %q", got.GetPlayerUserId(), b.id)
	}
}
