package progression

import (
	"testing"

	"connectrpc.com/connect"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	progressionv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/progression/v1"
)

// Finding U9-03: sameRequest ignores amount/character_ids/reason for MANUAL awards, so a reused idempotency key with a different request succeeds instead of failing.
func TestReview9_KeyReuseDifferentManual(t *testing.T) {
	tb := newTable(t, campaignsv1.XpMode_XP_MODE_ENEMIES, 3)
	key := newKey()
	first, err := tb.master.award(tb.campaign, func(r *progressionv1.AwardXPRequest) {
		r.Mode, r.Amount, r.CharacterIds, r.IdempotencyKey = progressionv1.XPAwardMode_XP_AWARD_MODE_MANUAL, 50, tb.ids(2), key
	})
	if err != nil {
		t.Fatalf("first award error = %v", err)
	}
	_ = first
	before := tb.master.xpOf(t, tb.pcs[2])
	_, err = tb.master.award(tb.campaign, func(r *progressionv1.AwardXPRequest) {
		r.Mode, r.Amount, r.CharacterIds, r.IdempotencyKey = progressionv1.XPAwardMode_XP_AWARD_MODE_MANUAL, 500, []string{tb.pcs[2].GetId()}, key
	})
	wantCode(t, "AwardXP(reused key, other amount and characters)", err, connect.CodeInvalidArgument)
	if got := tb.master.xpOf(t, tb.pcs[2]); got != before {
		t.Errorf("character C xp = %d, want %d", got, before)
	}
}

// Finding U9-04: AddExperience clamps at 1,000,000 but shares record the unclamped each, so undo subtracts more than was added.
func TestReview9_UndoAfterXPClamp(t *testing.T) {
	tb := newTable(t, campaignsv1.XpMode_XP_MODE_ENEMIES, 1)
	c := tb.pcs[0]
	tb.master.manual(t, tb.campaign, 999_900, c.GetId())
	if got := tb.master.xpOf(t, c); got != 999_900 {
		t.Fatalf("setup xp = %d, want 999900", got)
	}
	res, err := tb.master.award(tb.campaign, func(r *progressionv1.AwardXPRequest) {
		r.Mode, r.Amount, r.CharacterIds = progressionv1.XPAwardMode_XP_AWARD_MODE_MANUAL, 400, []string{c.GetId()}
	})
	if err != nil {
		t.Fatalf("clamped award rejected: %v", err)
	}
	if got := tb.master.xpOf(t, c); got != 1_000_000 {
		t.Fatalf("xp after clamped award = %d, want 1000000", got)
	}
	if _, err := tb.master.undo(tb.campaign, newKey()); err != nil {
		t.Fatalf("undo error = %v (award %s)", err, res.GetAward().GetId())
	}
	if got := tb.master.xpOf(t, c); got != 999_900 {
		t.Errorf("xp after undo = %d, want 999900", got)
	}
}
