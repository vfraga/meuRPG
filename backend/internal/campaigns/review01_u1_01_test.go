package campaigns

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/PuraFome/meuRPG/backend/internal/campaigns/campaignsdb"
)

// Finding U1-01
// A pending member whose 30-day deadline (RN-15) has passed, but whose row
// the TTL job has not deleted yet, must not count as a membership and must
// not be able to clear the deadline.
func TestReview1_StalePendingMember(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	mestre, pendente := h.newUser("Mestre"), h.newUser("Pendente")
	id := mestre.createCampaign(t, "Mirathel").GetId()
	_, approvalToken := mestre.createApprovalInvite(t, id)
	pendente.join(t, approvalToken)

	// The deadline passed an hour ago; the TTL job has not run.
	if _, err := h.pool.Exec(t.Context(), "UPDATE campaign_members SET pending_expires_at = $3 WHERE campaign_id = $1 AND user_id = $2",
		id, pendente.id, h.clock.Now().Add(-time.Hour)); err != nil {
		t.Fatalf("expire the row: %v", err)
	}

	q := h.service.queries
	if m, err := q.GetMembership(t.Context(), campaignsdb.GetMembershipParams{CampaignID: id, UserID: pendente.id}); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("GetMembership of an expired pending member = %+v, %v; want no rows (authz must treat them as a stranger)", m, err)
	}

	if _, err := q.ClearPendingExpiry(t.Context(), campaignsdb.ClearPendingExpiryParams{CampaignID: id, UserID: pendente.id}); err != nil {
		t.Fatalf("ClearPendingExpiry: %v", err)
	}
	if got := h.pendingExpiresAt(id, pendente.id); got == nil {
		t.Errorf("ClearPendingExpiry removed the deadline of an already-expired pending member; want it kept")
	}
}
