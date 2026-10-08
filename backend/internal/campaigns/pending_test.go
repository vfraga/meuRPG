package campaigns

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	"github.com/PuraFome/meuRPG/backend/internal/campaigns/campaignsdb"
)

// Pending members (RN-15, MR-024), Q24 and Q25: what happens to someone who
// accepted an invite with approval. The character side (a real pending
// character approved by an ordinary invite, the deadline cleared when the
// character is created) is in package characters' tests.

// listPending calls ListPendingMembers as u.
func (u *user) listPending(t *testing.T, campaignID string) []*campaignsv1.PendingMember {
	t.Helper()
	res, err := u.api.ListPendingMembers(t.Context(), connect.NewRequest(&campaignsv1.ListPendingMembersRequest{CampaignId: campaignID}))
	if err != nil {
		t.Fatalf("ListPendingMembers() error = %v", err)
	}
	return res.Msg.GetMembers()
}

// removePending calls RemovePendingMember as u.
func (u *user) removePending(campaignID, userID string) error {
	_, err := u.api.RemovePendingMember(context.Background(), connect.NewRequest(&campaignsv1.RemovePendingMemberRequest{CampaignId: campaignID, UserId: userID}))
	return err
}

// createManyApprovalInvite creates an invite with approval that several
// people can use (createApprovalInvite's has one use).
func (u *user) createManyApprovalInvite(t *testing.T, campaignID string) string {
	t.Helper()
	res, err := u.api.CreateInvite(t.Context(), connect.NewRequest(&campaignsv1.CreateInviteRequest{
		CampaignId: campaignID, MaxUses: 5, RequiresApproval: true,
	}))
	if err != nil {
		t.Fatalf("CreateInvite(requires_approval) error = %v", err)
	}
	return res.Msg.GetToken()
}

// pendingExpiresAt reads campaign_members.pending_expires_at directly; nil
// when it is NULL.
func (h *harness) pendingExpiresAt(campaignID, userID string) *time.Time {
	h.t.Helper()
	var at *time.Time
	if err := h.pool.QueryRow(h.t.Context(), "SELECT pending_expires_at FROM campaign_members WHERE campaign_id = $1 AND user_id = $2", campaignID, userID).Scan(&at); err != nil {
		h.t.Fatalf("read pending_expires_at: %v", err)
	}
	return at
}

// Q25: Dado um membro pendente, quando ele aceita um convite sem aprovação
// que vale agora, então ele vira jogador (um uso do convite é gasto) e o
// personagem dele, se esperava aprovação, é aprovado.
func TestQ25_PlainInvitePromotesPendingMember(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	mestre, jogadora := h.newUser("Mestre"), h.newUser("Jogadora")
	id := mestre.createCampaign(t, "Mirathel").GetId()
	_, approvalToken := mestre.createApprovalInvite(t, id)
	plain, plainToken := mestre.createInvite(t, id, 0, 0)
	jogadora.join(t, approvalToken)
	if got := h.pendingExpiresAt(id, jogadora.id); got == nil {
		t.Fatal("a new pending member has no pending_expires_at")
	}

	res, err := jogadora.accept(t, plainToken)
	if err != nil {
		t.Fatalf("AcceptInvite(plain) error = %v", err)
	}
	if c := res.GetCampaign(); res.GetAlreadyMember() || c.GetAwaitingApproval() || c.GetXpMode() == campaignsv1.XpMode_XP_MODE_UNSPECIFIED {
		t.Errorf("AcceptInvite(plain) = %v, want a join with the full campaign view and already_member false", res)
	}
	if got := h.memberStatus(id, jogadora.id); got != "active" {
		t.Errorf("status = %q, want active", got)
	}
	if got := h.pendingExpiresAt(id, jogadora.id); got != nil {
		t.Errorf("pending_expires_at = %v, want NULL once active", got)
	}
	if n := h.useCount(plain.GetId()); n != 1 {
		t.Errorf("use_count = %d, want 1: the promotion spends a use", n)
	}
	if got := h.characters.approvedUsers(); len(got) != 1 || got[0] != jogadora.id {
		t.Errorf("characters asked to approve %v, want %v", got, []string{jogadora.id})
	}
	// She is a member now: she can list the members.
	if _, err := jogadora.api.ListMembers(t.Context(), connect.NewRequest(&campaignsv1.ListMembersRequest{CampaignId: id})); err != nil {
		t.Errorf("promoted member's ListMembers() error = %v", err)
	}

	// Accepting again is the ordinary no-op of an active member: no use
	// spent, no second approval.
	again, err := jogadora.accept(t, plainToken)
	if err != nil || !again.GetAlreadyMember() {
		t.Errorf("AcceptInvite(plain) again = %v, %v; want already_member", again, err)
	}
	if n := h.useCount(plain.GetId()); n != 1 {
		t.Errorf("use_count after accepting again = %d, want 1", n)
	}
	if got := h.characters.approvedUsers(); len(got) != 1 {
		t.Errorf("characters asked to approve %v after accepting again, want once", got)
	}
}

// Q25: the promotion needs an invite that works now and has no approval;
// nothing else moves a pending member.
func TestQ25_OnlyAWorkingPlainInvitePromotes(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	mestre := h.newUser("Mestre")
	id := mestre.createCampaign(t, "Mirathel").GetId()
	_, approvalToken := mestre.createApprovalInvite(t, id)
	other, otherToken := mestre.createApprovalInvite(t, id)
	expiring, expiringToken := mestre.createInvite(t, id, 0, time.Hour)
	revoked, revokedToken := mestre.createInvite(t, id, 0, 0)
	if _, err := mestre.api.RevokeInvite(t.Context(), connect.NewRequest(&campaignsv1.RevokeInviteRequest{CampaignId: id, InviteId: revoked.GetId()})); err != nil {
		t.Fatalf("RevokeInvite() error = %v", err)
	}

	// Another invite with approval, a revoked one: she stays pending.
	jogadora := h.newUser("Jogadora")
	jogadora.join(t, approvalToken)
	for name, token := range map[string]string{"another invite with approval": otherToken, "a revoked invite": revokedToken} {
		if _, err := jogadora.accept(t, token); err != nil {
			t.Fatalf("AcceptInvite(%s) error = %v", name, err)
		}
		if got := h.memberStatus(id, jogadora.id); got != "pending" {
			t.Errorf("status after %s = %q, want pending", name, got)
		}
	}
	if n := h.useCount(other.GetId()); n != 0 {
		t.Errorf("use_count of the other approval invite = %d, want 0", n)
	}

	// An expired invite: she stays pending too.
	h.clock.Advance(2 * time.Hour)
	if _, err := jogadora.accept(t, expiringToken); err != nil {
		t.Fatalf("AcceptInvite(expired) error = %v", err)
	}
	if got := h.memberStatus(id, jogadora.id); got != "pending" {
		t.Errorf("status after an expired invite = %q, want pending", got)
	}
	if n := h.useCount(expiring.GetId()); n != 0 {
		t.Errorf("use_count of the expired invite = %d, want 0", n)
	}
	if got := h.characters.approvedUsers(); len(got) != 0 {
		t.Errorf("characters asked to approve %v, want nobody", got)
	}
}

// Q24: Dado um membro pendente sem personagem, quando o mestre abre a lista,
// então vê quem é, quando entrou e quando sai sozinho; e pode removê-lo.
func TestQ24_MasterSeesAndRemovesPendingMemberWithoutCharacter(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	mestre, ativo, primeira, segunda := h.newUser("Mestre"), h.newUser("Ativo"), h.newUser("Primeira"), h.newUser("Segunda")
	id := mestre.createCampaign(t, "Mirathel").GetId()
	_, plainToken := mestre.createInvite(t, id, 0, 0)
	approvalToken := mestre.createManyApprovalInvite(t, id)
	ativo.join(t, plainToken)

	joined := h.clock.Now()
	primeira.join(t, approvalToken)
	h.clock.Advance(time.Hour)
	segunda.join(t, approvalToken)

	pending := mestre.listPending(t, id)
	if len(pending) != 2 || pending[0].GetUserId() != primeira.id || pending[1].GetUserId() != segunda.id {
		t.Fatalf("ListPendingMembers() = %v, want Primeira then Segunda, and not the active member", pending)
	}
	first := pending[0]
	// joined_at comes from the database's clock, the deadline from the
	// service's (the test's fake one), so only the name and the presence of
	// joined_at are checked here.
	if first.GetDisplayName() != "Primeira" || first.GetJoinedAt() == nil {
		t.Errorf("first pending member = %v, want her name and a joined_at", first)
	}
	if want := joined.Add(PendingMemberLifetime); !first.GetExpiresAt().AsTime().Equal(want) {
		t.Errorf("expires_at = %v, want the time she joined + 30 days = %v", first.GetExpiresAt().AsTime(), want)
	}
	// ListMembers keeps meaning "people in the campaign".
	members, err := mestre.api.ListMembers(t.Context(), connect.NewRequest(&campaignsv1.ListMembersRequest{CampaignId: id}))
	if err != nil || len(members.Msg.GetMembers()) != 2 {
		t.Errorf("ListMembers() = %v, %v; want the master and the active player only", members, err)
	}

	// Refused: an active member, and the master himself.
	for name, userID := range map[string]string{"an active member": ativo.id, "the master": mestre.id} {
		wantCode(t, "RemovePendingMember("+name+")", mestre.removePending(id, userID), connect.CodeFailedPrecondition)
		if h.memberStatus(id, userID) != "active" {
			t.Errorf("%s was removed", name)
		}
	}
	// Not found: someone who was never in, and an ID that is not a UUID.
	wantCode(t, "RemovePendingMember(stranger)", mestre.removePending(id, h.newUser("De fora").id), connect.CodeNotFound)
	wantCode(t, "RemovePendingMember(garbage)", mestre.removePending(id, "primeira"), connect.CodeNotFound)

	// Removing deletes the membership and nothing else.
	if err := mestre.removePending(id, primeira.id); err != nil {
		t.Fatalf("RemovePendingMember() error = %v", err)
	}
	if got := h.memberStatus(id, primeira.id); got != "" {
		t.Errorf("status after removal = %q, want no membership", got)
	}
	wantCode(t, "removed person's GetCampaign()", func() error {
		_, err := primeira.api.GetCampaign(t.Context(), connect.NewRequest(&campaignsv1.GetCampaignRequest{CampaignId: id}))
		return err
	}(), connect.CodeNotFound)
	mine, err := primeira.api.ListMyCampaigns(t.Context(), connect.NewRequest(&campaignsv1.ListMyCampaignsRequest{}))
	if err != nil || len(mine.Msg.GetCampaigns()) != 0 {
		t.Errorf("removed person's ListMyCampaigns() = %v, %v; want none, and no error: the account is untouched", mine, err)
	}
	wantCode(t, "RemovePendingMember() again", mestre.removePending(id, primeira.id), connect.CodeNotFound)
	if left := mestre.listPending(t, id); len(left) != 1 || left[0].GetUserId() != segunda.id {
		t.Errorf("ListPendingMembers() after the removal = %v, want only Segunda", left)
	}
	// A new invite brings her back.
	primeira.join(t, approvalToken)
	if got := h.memberStatus(id, primeira.id); got != "pending" {
		t.Errorf("status after a new invite = %q, want pending", got)
	}
}

// Q24: the master's own campaign only: another campaign's master cannot
// list or remove.
func TestQ24_OnlyTheCampaignsMasterManagesPendingMembers(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	mestre, outro, jogadora := h.newUser("Mestre"), h.newUser("Outro mestre"), h.newUser("Jogadora")
	id := mestre.createCampaign(t, "Mirathel").GetId()
	outro.createCampaign(t, "Barovia")
	_, approvalToken := mestre.createApprovalInvite(t, id)
	jogadora.join(t, approvalToken)

	_, err := outro.api.ListPendingMembers(t.Context(), connect.NewRequest(&campaignsv1.ListPendingMembersRequest{CampaignId: id}))
	wantCode(t, "another master's ListPendingMembers()", err, connect.CodeNotFound)
	wantCode(t, "another master's RemovePendingMember()", outro.removePending(id, jogadora.id), connect.CodeNotFound)
	if got := h.memberStatus(id, jogadora.id); got != "pending" {
		t.Errorf("status = %q, want pending", got)
	}
}

// Q24: um membro pendente sem personagem sai sozinho 30 dias depois de
// entrar. The job runs in the database once a day, so this test checks what
// feeds it: the deadline is set when the pending membership is created and
// absent otherwise, and the table's TTL expression turns it into an
// expiration.
func TestQ24_PendingMembershipExpiresAfter30Days(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	mestre, ativo, pendente := h.newUser("Mestre"), h.newUser("Ativo"), h.newUser("Pendente")
	id := mestre.createCampaign(t, "Mirathel").GetId()
	_, plainToken := mestre.createInvite(t, id, 0, 0)
	_, approvalToken := mestre.createApprovalInvite(t, id)
	ativo.join(t, plainToken)
	joined := h.clock.Now()
	pendente.join(t, approvalToken)

	if got := h.pendingExpiresAt(id, pendente.id); got == nil || !got.Equal(joined.Add(30*24*time.Hour)) {
		t.Errorf("pending member's pending_expires_at = %v, want joined time + 30 days = %v", got, joined.Add(30*24*time.Hour))
	}
	for name, userID := range map[string]string{"the master": mestre.id, "an active member": ativo.id} {
		if got := h.pendingExpiresAt(id, userID); got != nil {
			t.Errorf("%s has pending_expires_at = %v, want NULL: only a pending member has a deadline", name, got)
		}
	}

	// The table's TTL is on and expires exactly the rows with a deadline.
	var options []string
	if err := h.pool.QueryRow(t.Context(), "SELECT reloptions FROM pg_class WHERE relname = 'campaign_members'").Scan(&options); err != nil {
		t.Fatalf("read the table's options: %v", err)
	}
	var ttlOn bool
	var expression string
	for _, o := range options {
		switch {
		case o == "ttl='on'":
			ttlOn = true
		case strings.HasPrefix(o, "ttl_expiration_expression="):
			// Stored as a string literal: 'pending_expires_at' or e'...'.
			v := strings.TrimPrefix(o, "ttl_expiration_expression=")
			expression = strings.Trim(strings.TrimPrefix(v, "e"), "'")
		}
	}
	if !ttlOn || expression == "" {
		t.Fatalf("campaign_members has no row-level TTL with an expiration expression: %q", options)
	}
	rows, err := h.pool.Query(t.Context(), fmt.Sprintf("SELECT user_id, (%s) FROM campaign_members WHERE campaign_id = $1", expression), id)
	if err != nil {
		t.Fatalf("evaluate %q: %v", expression, err)
	}
	defer rows.Close()
	seen := 0
	for rows.Next() {
		var userID string
		var expires *time.Time
		if err := rows.Scan(&userID, &expires); err != nil {
			t.Fatalf("scan: %v", err)
		}
		seen++
		if (userID == pendente.id) != (expires != nil) {
			t.Errorf("member %s expires at %v; only the pending member without a character should expire", userID, expires)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read rows: %v", err)
	}
	if seen != 3 {
		t.Errorf("evaluated %d rows, want 3", seen)
	}
}

// RN-15: the 30-day deadline binds as soon as it passes, not when the daily
// TTL job deletes the row. A pending member past it is a stranger to
// authorization, is not listed for the master and cannot clear the deadline.
func TestQ24_PendingMemberPastTheDeadlineIsNoMemberBeforeTheTTLJobRuns(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	mestre, pendente, ativo := h.newUser("Mestre"), h.newUser("Pendente"), h.newUser("Ativo")
	id := mestre.createCampaign(t, "Mirathel").GetId()
	_, approvalToken := mestre.createApprovalInvite(t, id)
	_, plainToken := mestre.createInvite(t, id, 0, 0)
	pendente.join(t, approvalToken)
	ativo.join(t, plainToken)
	q := h.service.queries

	// Before the deadline: a member, listed, and able to clear the deadline.
	if got := len(mestre.listPending(t, id)); got != 1 {
		t.Fatalf("setup: pending members = %d, want 1", got)
	}
	if _, err := q.GetMembership(t.Context(), campaignsdb.GetMembershipParams{CampaignID: id, UserID: pendente.id, Now: h.clock.Now()}); err != nil {
		t.Fatalf("GetMembership() before the deadline error = %v, want the membership", err)
	}

	h.clock.Advance(PendingMemberLifetime + time.Hour)

	if m, err := q.GetMembership(t.Context(), campaignsdb.GetMembershipParams{CampaignID: id, UserID: pendente.id, Now: h.clock.Now()}); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("GetMembership() of a pending member past the deadline = %+v, %v; want no rows", m, err)
	}
	if got := mestre.listPending(t, id); len(got) != 0 {
		t.Errorf("ListPendingMembers() after the deadline = %d members, want 0", len(got))
	}
	_, err := pendente.api.GetCampaign(t.Context(), connect.NewRequest(&campaignsv1.GetCampaignRequest{CampaignId: id}))
	wantCode(t, "GetCampaign() by a pending member past the deadline", err, connect.CodeNotFound)
	if _, err := q.ClearPendingExpiry(t.Context(), campaignsdb.ClearPendingExpiryParams{CampaignID: id, UserID: pendente.id, Now: h.clock.Now()}); err != nil {
		t.Fatalf("ClearPendingExpiry() error = %v", err)
	}
	if got := h.pendingExpiresAt(id, pendente.id); got == nil {
		t.Errorf("ClearPendingExpiry() removed the deadline of a pending member past it; want it kept")
	}
	// Positive control: an active member is not affected by the guard.
	if _, err := ativo.api.GetCampaign(t.Context(), connect.NewRequest(&campaignsv1.GetCampaignRequest{CampaignId: id})); err != nil {
		t.Errorf("GetCampaign() by an active member error = %v", err)
	}
}

// A pending member past the deadline can join again with a new invite, even
// though the TTL job has not deleted the stale row: the new membership has a
// new deadline and the invite spends one use.
func TestQ24_PendingMemberPastTheDeadlineJoinsAgainWithANewInvite(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	mestre, pendente := h.newUser("Mestre"), h.newUser("Pendente")
	id := mestre.createCampaign(t, "Mirathel").GetId()
	_, firstToken := mestre.createApprovalInvite(t, id)
	pendente.join(t, firstToken)

	h.clock.Advance(PendingMemberLifetime + time.Hour)
	invite, secondToken := mestre.createApprovalInvite(t, id)
	res, err := pendente.accept(t, secondToken)
	if err != nil {
		t.Fatalf("AcceptInvite() after the deadline error = %v", err)
	}
	if res.GetAlreadyMember() {
		t.Errorf("AcceptInvite() after the deadline = already_member, want a new join")
	}
	if n := h.useCount(invite.GetId()); n != 1 {
		t.Errorf("use_count = %d, want 1", n)
	}
	want := h.clock.Now().Add(PendingMemberLifetime)
	if got := h.pendingExpiresAt(id, pendente.id); got == nil || !got.Equal(want) {
		t.Errorf("pending_expires_at = %v, want a new deadline %v", got, want)
	}
}
