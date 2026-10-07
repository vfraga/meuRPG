package play

import (
	"testing"

	"connectrpc.com/connect"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// Finding U4-6: SearchForTraps answers a replayed idempotency key with the stored
// event after checking only its kind, never its actor. Another player who learns the
// key gets the first player's d20, total and FoundPointIds instead of the
// "idempotency_key was already used for another change" refusal of the other handlers.
func TestReview4_SearchForTrapsReplayByAnotherPlayer(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	needle := r.trap(t, "Agulha Rubra", 8, 8, func(s *mapsv1.TrapSpec) { s.NoticeDc, s.FindDc = 0, 13 })
	r.place(t, r.pens.GetId(), 8, 7)
	r.place(t, r.toren.GetId(), 3, 3)

	key := newKey()
	search := func(u *user, face int32) (*playv1.SearchForTrapsResponse, error) {
		res, err := u.play.SearchForTraps(t.Context(), connect.NewRequest(&playv1.SearchForTrapsRequest{
			CampaignId: r.campaignID, IdempotencyKey: key,
			Skill: playv1.TrapSearchSkill_TRAP_SEARCH_SKILL_INVESTIGATION,
			Roll:  &playv1.SearchForTrapsRequest_D20Face{D20Face: face},
		}))
		if err != nil {
			return nil, err
		}
		return res.Msg, nil
	}

	first, err := search(r.ana, 17)
	if err != nil || len(first.GetFoundPointIds()) != 1 || first.GetFoundPointIds()[0] != needle.GetId() {
		t.Fatalf("player A's search = %v, %v; want the needle", first, err)
	}

	got, err := search(r.caio, 3) // player B, other character, same key
	if err == nil {
		t.Errorf("player B replaying A's key got a success (d20 %v, total %d, found %v); want invalid_argument",
			got.GetRoll().GetFaces(), got.GetRoll().GetTotal(), got.GetFoundPointIds())
		return
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("error code = %v, want invalid_argument", connect.CodeOf(err))
	}
}
