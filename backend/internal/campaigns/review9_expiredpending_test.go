package campaigns

import (
	"testing"

	"connectrpc.com/connect"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
)

// Finding U9-05: a pending member past pending_expires_at still counts as a member (authz GetMembership) and is still listed, until the TTL job runs.
func TestReview9_ExpiredPendingMember(t *testing.T) {
	h := newHarness(t)
	mestre := h.newUser("Mestre")
	jogadora := h.newUser("Jogadora")
	c := mestre.createCampaign(t, "Mesa")
	_, token := mestre.createApprovalInvite(t, c.GetId())
	jogadora.join(t, token)

	if got := len(mestre.listPending(t, c.GetId())); got != 1 {
		t.Fatalf("setup: pending members = %d, want 1", got)
	}
	if _, err := h.pool.Exec(t.Context(),
		"UPDATE campaign_members SET pending_expires_at = now() - interval '1 day' WHERE campaign_id = $1 AND user_id = $2",
		c.GetId(), jogadora.id); err != nil {
		t.Fatalf("setup: expire pending member: %v", err)
	}

	if got := mestre.listPending(t, c.GetId()); len(got) != 0 {
		t.Errorf("ListPendingMembers() after the deadline = %d members, want 0", len(got))
	}
	_, err := jogadora.api.GetCampaign(t.Context(), connect.NewRequest(&campaignsv1.GetCampaignRequest{CampaignId: c.GetId()}))
	wantCode(t, "GetCampaign() by expired pending member", err, connect.CodeNotFound)
}
