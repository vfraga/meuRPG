package play

import (
	"testing"

	"connectrpc.com/connect"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// Finding U2-2: SearchForTraps replays another user's idempotency key
func TestReview2_SearchKeyReplayAcrossUsers(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	needle := r.trap(t, "Agulha Rubra", 8, 8, func(s *mapsv1.TrapSpec) { s.NoticeDc, s.FindDc = 0, 13 })
	r.place(t, r.pens.GetId(), 8, 7) // Ana's character, next to the trap
	r.place(t, r.toren.GetId(), 3, 3) // Caio's character, far away

	key := newKey()
	req := func() *playv1.SearchForTrapsRequest {
		return &playv1.SearchForTrapsRequest{CampaignId: r.campaignID, IdempotencyKey: key,
			Skill: investigation, Roll: &playv1.SearchForTrapsRequest_D20Face{D20Face: 20}}
	}
	a, err := r.ana.play.SearchForTraps(t.Context(), connect.NewRequest(req()))
	if err != nil || len(a.Msg.GetFoundPointIds()) != 1 || a.Msg.GetFoundPointIds()[0] != needle.GetId() {
		t.Fatalf("A's search = %v, %v; want the needle", a, err)
	}
	b, err := r.caio.play.SearchForTraps(t.Context(), connect.NewRequest(req()))
	if err == nil {
		t.Errorf("B reusing A's key got no error; response = %v (found %v)", b.Msg, b.Msg.GetFoundPointIds())
		for _, id := range b.Msg.GetFoundPointIds() {
			if id == needle.GetId() {
				t.Errorf("B received trap id %s that B's character never found", id)
			}
		}
	} else {
		wantCode(t, "SearchForTraps(B, A's key)", err, connect.CodeInvalidArgument)
	}
	r.wantKnows(t, "B's player", r.caio, needle.GetId(), false)
	if n := r.eventCount(t, "trap_searched"); n != 1 {
		t.Errorf("trap_searched events = %d, want 1", n)
	}
}
