package characters

import (
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

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

// Usage is clamped when read, never when written: an edit of something else while the sheet has fewer slots keeps the stored usage of the slots it lost.
func TestAdjustVitalsKeepsUsageOfSlotsTheSheetLost(t *testing.T) {
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

// The same holds for resources: a resource the sheet lacks for now, and a use above today's total, stay as stored.
func TestAdjustVitalsKeepsStoredResourceUsage(t *testing.T) {
	h := newHarness(t)
	master, player := h.newUser("Mestre"), h.newUser("Jogadora")
	campaign := h.newCampaign(master, "Mirathel", player)
	pc := player.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Pensantus", wizardAt(5))
	h.adjustVitals(campaign, pc.GetId(), &playv1.AdjustCharacterVitalsRequest{HitPointsCurrent: new(int32(1))})
	if _, err := h.pool.Exec(t.Context(),
		`UPDATE character_vitals SET resources_used = '{"gone-for-now": 2}'::JSONB WHERE character_id = $1`, pc.GetId()); err != nil {
		t.Fatalf("store usage: %v", err)
	}
	tmp := int32(3)
	h.adjustVitals(campaign, pc.GetId(), &playv1.AdjustCharacterVitalsRequest{HitPointsTemporary: &tmp})
	var got int32
	if err := h.pool.QueryRow(t.Context(),
		`SELECT COALESCE((resources_used->>'gone-for-now')::INT4, 0) FROM character_vitals WHERE character_id = $1`, pc.GetId()).Scan(&got); err != nil {
		t.Fatalf("read usage: %v", err)
	}
	if got != 2 {
		t.Errorf("stored usage of a resource the sheet lacks = %d, want 2", got)
	}
}
