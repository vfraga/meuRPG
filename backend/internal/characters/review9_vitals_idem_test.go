package characters

import (
	"slices"
	"testing"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"
	"uuid"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
)

func (h *harness) adjustVitals(campaign, characterID string, req *playv1.AdjustCharacterVitalsRequest) {
	h.t.Helper()
	err := db.InTx(h.t.Context(), h.pool, func(tx pgx.Tx) error {
		_, _, err := h.svc.AdjustVitals(h.t.Context(), tx, campaign, characterID, req)
		return err
	})
	if err != nil {
		h.t.Fatalf("AdjustVitals() error = %v", err)
	}
}

// Finding U9-06: CreateCharacter's create key is unique per campaign, not per user, so player B replaying A's key and request gets A's character.
func TestReview9_CreateKeyIsNotSharedBetweenPlayers(t *testing.T) {
	h := newHarness(t)
	master, a, b := h.newUser("Mestre"), h.newUser("A"), h.newUser("B")
	campaign := h.newCampaign(master, "Mirathel", a, b)
	req := &charactersv1.CreateCharacterRequest{
		CampaignId: campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Pensantus",
		Sheet: pensantusSheet(), IdempotencyKey: uuid.New().String(),
	}
	first, err := a.api.CreateCharacter(t.Context(), connect.NewRequest(proto.Clone(req).(*charactersv1.CreateCharacterRequest)))
	if err != nil {
		t.Fatalf("A CreateCharacter() error = %v", err)
	}
	second, err := b.api.CreateCharacter(t.Context(), connect.NewRequest(proto.Clone(req).(*charactersv1.CreateCharacterRequest)))
	if err != nil {
		return // a refusal is acceptable
	}
	got := second.Msg.GetCharacter()
	if got.GetId() == first.Msg.GetCharacter().GetId() || got.GetPlayerUserId() == a.id {
		t.Errorf("B received A's character: id %q, player_user_id %q (A is %q)", got.GetId(), got.GetPlayerUserId(), a.id)
	}
}

// wizardAt is Pensantus at a level, with what only a higher level gives removed.
func wizardAt(level int32) *charactersv1.CharacterSheet {
	s := pensantusSheet()
	f := s.GetFull()
	f.Classes[0].Level = level
	if level < 2 {
		f.Classes[0].Subclass = nil
		keep := func(keys []string) []string {
			return slices.DeleteFunc(slices.Clone(keys), func(k string) bool {
				return slices.Contains([]string{"spell:scorching-ray", "spell:web"}, k)
			})
		}
		f.KnownSpellKeys, f.PreparedSpellKeys = keep(f.KnownSpellKeys), keep(f.PreparedSpellKeys)
	}
	return s
}

// Finding U9-11: AdjustVitals rewrites stored slot usage from the clamped view, so a temp-HP edit while the sheet is lower erases a used 3rd-level slot.
func TestReview9_AdjustVitalsKeepsUsageOfSlotsTheSheetLost(t *testing.T) {
	h := newHarness(t)
	master, player := h.newUser("Mestre"), h.newUser("Jogadora")
	campaign := h.newCampaign(master, "Mirathel", player)
	pc := player.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Pensantus", wizardAt(5))
	h.adjustVitals(campaign, pc.GetId(), &playv1.AdjustCharacterVitalsRequest{
		SpellSlotsUsed: []*playv1.SpellSlotsUsed{{Level: 3, Used: 1}},
	})
	pc, err := master.update(t, pc, pc.GetName(), wizardAt(1))
	if err != nil {
		t.Fatalf("lower the sheet: %v", err)
	}
	tmp := int32(5)
	h.adjustVitals(campaign, pc.GetId(), &playv1.AdjustCharacterVitalsRequest{HitPointsTemporary: &tmp})
	if _, err := master.update(t, pc, pc.GetName(), wizardAt(5)); err != nil {
		t.Fatalf("raise the sheet: %v", err)
	}
	v, err := h.svc.GetVitals(t.Context(), campaign, pc.GetId())
	if err != nil {
		t.Fatalf("GetVitals() error = %v", err)
	}
	var used int32 = -1
	for _, s := range v.GetSpellSlots() {
		if s.GetLevel() == 3 {
			used = s.GetUsed()
		}
	}
	if used != 1 {
		t.Errorf("3rd-level slots used = %d, want 1 (stored usage must survive an unrelated edit)", used)
	}
}
