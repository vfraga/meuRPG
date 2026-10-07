package characters

import (
	"testing"
	"uuid"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
)

// Finding U7-8 (review/unit-07-characters.md): the create key is unique per campaign and the request
// hash leaves out the caller, so player B sending A's key with a byte-identical request gets A's
// character as a "replay" instead of a character of their own.
func TestReview7_CreateCharacterReplayIsPerUser(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, a, b := h.newUser("Mestre"), h.newUser("Jogadora A"), h.newUser("Jogador B")
	campaign := h.newCampaign(master, "Mirathel", a, b)

	key := uuid.New().String()
	req := &charactersv1.CreateCharacterRequest{
		CampaignId: campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Pensantus", Sheet: pensantusSheet(), IdempotencyKey: key,
	}
	resA, err := a.api.CreateCharacter(t.Context(), connect.NewRequest(req))
	if err != nil {
		t.Fatalf("A CreateCharacter() error = %v", err)
	}
	charA := resA.Msg.GetCharacter()

	resB, err := b.api.CreateCharacter(t.Context(), connect.NewRequest(req))
	if err != nil {
		t.Fatalf("B CreateCharacter() error = %v", err)
	}
	charB := resB.Msg.GetCharacter()
	if charB.GetId() == charA.GetId() {
		t.Errorf("B got A's character %q as a replay; want a new character of B", charA.GetId())
	}
	if charB.GetPlayerUserId() != b.id {
		t.Errorf("B's character player_user_id = %q, want B (%q); A is %q", charB.GetPlayerUserId(), b.id, a.id)
	}
}
