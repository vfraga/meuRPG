package play

import (
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// Finding U2-1: ListTrapActivity keeps the oldest 500 trap events, so newer lines vanish.
func TestReview2_TrapActivityTruncation(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	pit := r.trap(t, "Fosso Dourado", 9, 7)
	r.place(t, r.pens.GetId(), 8, 7)
	r.place(t, r.toren.GetId(), 3, 3)

	for i := 0; i < 505; i++ {
		if _, err := r.search(t, r.ana, investigation, 1); err != nil {
			t.Fatalf("search %d: %v", i, err)
		}
	}
	if _, err := r.fireByHand(t, pit, r.pens.GetId()); err != nil {
		t.Fatalf("FireTrap() error = %v", err)
	}
	for name, u := range map[string]*user{"master": r.master, "player": r.ana} {
		res, err := u.play.ListTrapActivity(t.Context(), connect.NewRequest(&playv1.ListTrapActivityRequest{CampaignId: r.campaignID}))
		if err != nil {
			t.Fatalf("%s ListTrapActivity() error = %v", name, err)
		}
		var firings int
		for _, a := range res.Msg.GetActivity() {
			if a.GetFiring() != nil {
				firings++
			}
		}
		if firings != 1 {
			t.Errorf("%s: %d lines, %d firings; want the newest event (the firing) listed", name, len(res.Msg.GetActivity()), firings)
		}
	}
}
