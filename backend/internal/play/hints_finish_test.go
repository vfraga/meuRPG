package play

import (
	"context"
	"testing"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
)

// The hints a combat handler tells in finish (encounter_changed, combat_log_changed, the vitals, the forced moves) carry the number of
// the write that made the change; a retry of its key carries the number as it stands (#735).

const (
	kEncChanged = "*playv1.WatchGameSessionResponse_EncounterChanged_"
)

// finishWrite is a handler's tail: write the change, then finish with the usual publishers of a handler (the combat changed, its log, a
// player's vitals).
func (x *coreFixture) finishWrite(t *testing.T, key string) combatResult {
	t.Helper()
	res, err := x.writeThatTells(t, key)
	if err != nil {
		t.Fatalf("write() error = %v", err)
	}
	m := authz.Membership{UserID: x.master.id, CampaignID: x.campaignID, Role: authz.RoleMaster}
	svc := x.h.svc
	v, err := svc.vitals.GetVitals(t.Context(), x.campaignID, x.torenID(t))
	if err != nil {
		t.Fatalf("GetVitals() error = %v", err)
	}
	if _, err := svc.finish(t.Context(), m, res, func(ctx context.Context, d *encounterData) {
		svc.publishEncounterChanged(ctx, m.CampaignID, d.enc)
		svc.publishLogChanged(ctx, m.CampaignID, d.enc.ID, true)
		svc.publishVitals(ctx, m.CampaignID, v)
	}); err != nil {
		t.Fatalf("finish() error = %v", err)
	}
	return res
}

// wantStamped checks every event of the finish hints in heard carries seq, and that there were some.
func wantStamped(t *testing.T, who string, heard []*playv1.WatchGameSessionResponse, seq int64) {
	t.Helper()
	seen := 0
	for _, ev := range heard {
		switch k := eventKindOf(ev); k {
		case kEncChanged, kLogChanged, kVitals:
			seen++
			if ev.GetSessionSeq() != seq {
				t.Errorf("%s: %s carries number %d, want %d", who, k, ev.GetSessionSeq(), seq)
			}
		}
	}
	if seen == 0 {
		t.Errorf("%s heard none of the finish hints (heard %v)", who, eventKinds(heard))
	}
}

func eventKindOf(ev *playv1.WatchGameSessionResponse) string {
	return eventKinds([]*playv1.WatchGameSessionResponse{ev})[0]
}

// RED before finish stamped: the hints of finish went out with number 0.
func TestFinishHintsCarryTheNumberOfTheWrite(t *testing.T) {
	t.Parallel()
	x := newCoreFixture(t)
	sid := x.sessionID(t)
	master := listen(t, x.h, x.campaignID, x.master.id, true)
	caio := listen(t, x.h, x.campaignID, x.caio.id, false)
	before := x.h.hintSeq(t, sid)

	res := x.finishWrite(t, newKey())
	stored := x.h.hintSeq(t, sid)
	if stored != before+1 || res.seq != stored {
		t.Fatalf("hint_seq = %d, result number = %d; want both %d: finish does not bump again", stored, res.seq, before+1)
	}
	wantStamped(t, "master", master.heard(t, x.h), stored)
	wantStamped(t, "Toren's player", caio.heard(t, x.h), stored)
}

// A retry of the key tells the finish hints with the number the first call stored, and moves nothing.
func TestFinishHintsOfARetryCarryTheStoredNumber(t *testing.T) {
	t.Parallel()
	x := newCoreFixture(t)
	sid := x.sessionID(t)
	key := newKey()
	x.finishWrite(t, key)
	stored := x.h.hintSeq(t, sid)
	master := listen(t, x.h, x.campaignID, x.master.id, true)
	caio := listen(t, x.h, x.campaignID, x.caio.id, false)

	res := x.finishWrite(t, key)
	if !res.repeated || res.seq != stored {
		t.Fatalf("retry = repeated %v, number %d; want a repeat with %d", res.repeated, res.seq, stored)
	}
	if got := x.h.hintSeq(t, sid); got != stored {
		t.Errorf("hint_seq = %d after the retry, want %d", got, stored)
	}
	wantStamped(t, "master", master.heard(t, x.h), stored)
	wantStamped(t, "Toren's player", caio.heard(t, x.h), stored)
}
