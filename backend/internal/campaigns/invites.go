package campaigns

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
	"uuid"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/timestamppb"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/campaigns/campaignsdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/logging"
	"github.com/PuraFome/meuRPG/backend/internal/platform/secret"
)

// An invite link is https://<app>/invite#t=<token>. The token is a random
// secret (package secret) that the server returns once, to the master, and
// then only knows as a SHA-256 hash. It travels in the URL fragment, which
// browsers never send to a server, and the app posts it to AcceptInvite in
// the request body (ADR-0009, docs/privacy.md).

// newInviteToken returns a new invite token and the hash to store.
func newInviteToken() (token string, hash []byte) { return secret.New() }

// errInviteNotFound is the answer for a token or invite ID that matches no
// invite of the campaign.
func errInviteNotFound() error {
	return connect.NewError(connect.CodeNotFound, errors.New("invite not found"))
}

// inviteLimits applies the defaults and limits to a CreateInvite request.
func inviteLimits(req *campaignsv1.CreateInviteRequest) (maxUses int32, lifetime time.Duration, err error) {
	maxUses = req.GetMaxUses()
	if maxUses == 0 {
		maxUses = DefaultInviteUses
	}
	if maxUses < 1 || maxUses > MaxInviteUses {
		return 0, 0, invalidArgument("max_uses", fmt.Errorf("must be between 1 and %d", MaxInviteUses))
	}

	lifetime = DefaultInviteLifetime
	if req.GetExpiresIn() != nil {
		if err := req.GetExpiresIn().CheckValid(); err != nil {
			return 0, 0, invalidArgument("expires_in", errors.New("must be a valid duration"))
		}
		lifetime = req.GetExpiresIn().AsDuration()
	}
	if lifetime < MinInviteLifetime || lifetime > MaxInviteLifetime {
		return 0, 0, invalidArgument("expires_in", fmt.Errorf("must be between %v and %v", MinInviteLifetime, MaxInviteLifetime))
	}
	return maxUses, lifetime, nil
}

// inviteState says whether the invite works at now. When several reasons
// apply, revoked wins (the master's explicit decision), then used up (it
// warns a player that someone else may have used their link), then expired.
func inviteState(invite campaignsdb.CampaignInvite, now time.Time) campaignsv1.InviteState {
	switch {
	case invite.RevokedAt != nil:
		return campaignsv1.InviteState_INVITE_STATE_REVOKED
	case invite.UseCount >= invite.MaxUses:
		return campaignsv1.InviteState_INVITE_STATE_USED_UP
	case !now.Before(invite.ExpiresAt):
		return campaignsv1.InviteState_INVITE_STATE_EXPIRED
	default:
		return campaignsv1.InviteState_INVITE_STATE_ACTIVE
	}
}

// errNoInvite is acceptInvite's error when no invite has the token.
var errNoInvite = errors.New("no invite has this token")

// unusableInviteError is acceptInvite's error for an invite that exists but
// does not work now: expired, revoked or used up.
type unusableInviteError struct {
	state campaignsv1.InviteState
}

func (e *unusableInviteError) Error() string {
	return "the invite cannot be used: " + e.state.String()
}

// errInviteUnusable is AcceptInvite's answer for an invite that exists but
// does not work. The InviteUnusable detail tells the app which message to
// show.
func errInviteUnusable(state campaignsv1.InviteState) error {
	msg := map[campaignsv1.InviteState]string{
		campaignsv1.InviteState_INVITE_STATE_EXPIRED: "this invite has expired; ask the master for a new one",
		campaignsv1.InviteState_INVITE_STATE_REVOKED: "this invite was revoked; ask the master for a new one",
		campaignsv1.InviteState_INVITE_STATE_USED_UP: "this invite was already used; ask the master for a new one",
	}[state]
	err := connect.NewError(connect.CodeFailedPrecondition, errors.New(msg))
	if detail, detailErr := connect.NewErrorDetail(&campaignsv1.InviteUnusable{State: state}); detailErr == nil {
		err.AddDetail(detail)
	}
	return err
}

// joinResult is what acceptInvite did.
type joinResult struct {
	campaign      campaignsdb.Campaign
	role          authz.Role
	alreadyMember bool
	// pending is true when the caller is a pending member (RN-15): they
	// joined through an invite that requires approval, now or before.
	pending bool
}

// acceptInvite makes userID a player of the campaign whose invite token
// hashes to tokenHash, in one transaction, or a pending member when the
// invite requires approval (RN-15, MR-024). Both ways of accepting an
// invite run it: the AcceptInvite RPC, and signing in with the invite as
// the sign-in intent (inviteIntent). The steps:
//
//  1. Find the invite by the token's hash and lock it (FOR UPDATE).
//  2. Already a member, or a pending member? Return the campaign; nothing
//     changes, and no use of the invite is spent. This makes AcceptInvite
//     idempotent, so a double click or a retry is harmless. One exception
//     (Q25): a pending member who accepts an invite that works now and does
//     NOT require approval is promoted, see promotePending.
//  3. Otherwise the invite must work: not revoked, not used up, not expired.
//  4. Spend one use and add the membership: active, or pending when the
//     invite requires approval.
//
// Two people racing for an invite's last use cannot both get in: the lock
// makes the second transaction wait for the first, and it then finds the
// invite used up (TestAcceptInviteRaceForTheLastUse). The UPDATE in step 4
// repeats the rules, and a CHECK keeps use_count <= max_uses, so even a bug
// here could not overspend an invite.
//
// It returns errNoInvite or an *unusableInviteError when the invite does
// not work; any other error is the database's.
func (s *Service) acceptInvite(ctx context.Context, tokenHash []byte, userID string) (joinResult, error) {
	now := s.now()

	var result joinResult
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)

		invite, err := q.GetInviteByTokenHashForUpdate(ctx, tokenHash)
		if errors.Is(err, pgx.ErrNoRows) {
			return errNoInvite
		}
		if err != nil {
			return fmt.Errorf("get invite: %w", err)
		}
		campaign, err := q.GetCampaign(ctx, invite.CampaignID)
		if err != nil {
			return fmt.Errorf("get campaign: %w", err)
		}

		member, err := q.GetMembership(ctx, campaignsdb.GetMembershipParams{CampaignID: invite.CampaignID, UserID: userID, Now: now})
		switch {
		case err == nil && member.Status == string(authz.StatusPending) && !invite.RequiresApproval &&
			inviteState(invite, now) == campaignsv1.InviteState_INVITE_STATE_ACTIVE:
			// An invite that does not need the master's approval was
			// accepted by someone who is waiting for it: that is approval
			// enough.
			if err := s.promotePending(ctx, q, tx, invite, userID, now); err != nil {
				return err
			}
			result = joinResult{campaign: campaign, role: authz.RolePlayer}
			return nil
		case err == nil:
			result = joinResult{
				campaign:      campaign,
				role:          authz.Role(member.Role),
				alreadyMember: true,
				pending:       member.Status == string(authz.StatusPending),
			}
			return nil
		case !errors.Is(err, pgx.ErrNoRows):
			return fmt.Errorf("get membership: %w", err)
		}

		if state := inviteState(invite, now); state != campaignsv1.InviteState_INVITE_STATE_ACTIVE {
			return &unusableInviteError{state: state}
		}
		// A pending member past the deadline is no member, but the TTL job
		// may not have deleted the row yet: it goes before the new one.
		if _, err := q.DeleteExpiredPendingMember(ctx, campaignsdb.DeleteExpiredPendingMemberParams{CampaignID: invite.CampaignID, UserID: userID, Now: now}); err != nil {
			return fmt.Errorf("delete expired pending member: %w", err)
		}
		spent, err := q.IncrementInviteUses(ctx, campaignsdb.IncrementInviteUsesParams{ID: invite.ID, Now: now})
		if err != nil {
			return fmt.Errorf("spend an invite use: %w", err)
		}
		if spent == 0 {
			// Unreachable while the row is locked and checked above; kept
			// so that a future change cannot turn into a free pass.
			return &unusableInviteError{state: campaignsv1.InviteState_INVITE_STATE_USED_UP}
		}
		// An invite with approval makes a pending member (RN-15): they may
		// only work on their character until the master approves it.
		status := authz.StatusActive
		if invite.RequiresApproval {
			status = authz.StatusPending
		}
		// A pending member has no character yet, so the 30-day deadline
		// starts now; creating the character clears it (RN-15).
		var expires *time.Time
		if invite.RequiresApproval {
			deadline := now.Add(PendingMemberLifetime)
			expires = &deadline
		}
		_, err = q.InsertMember(ctx, campaignsdb.InsertMemberParams{
			CampaignID:       invite.CampaignID,
			UserID:           userID,
			Role:             string(authz.RolePlayer),
			Status:           string(status),
			PendingExpiresAt: expires,
		})
		if err != nil {
			return fmt.Errorf("insert player: %w", err)
		}
		result = joinResult{campaign: campaign, role: authz.RolePlayer, pending: invite.RequiresApproval}
		return nil
	})
	if err == nil && !result.alreadyMember {
		// Explicit campaign_id: the sign-in intent path has no campaign in its context.
		logging.Event(ctx, s.logger, "invite.accepted", slog.String("campaign_id", result.campaign.ID),
			slog.Bool("pending_approval", result.pending))
	}
	return result, err
}

// promotePending turns userID's pending membership into an active one with
// an invite that works and needs no approval (Q25), inside the transaction
// of acceptInvite, where the invite row is already locked. It spends one use
// like any join, activates the membership (which also drops the 30-day
// deadline) and approves the character waiting for approval, if they
// created one: the same effect as the master's ApproveCharacter, through
// Characters. All of it is one transaction, so a retry after a
// serialization error (40001) starts from the unchanged state.
func (s *Service) promotePending(ctx context.Context, q *campaignsdb.Queries, tx pgx.Tx, invite campaignsdb.CampaignInvite, userID string, now time.Time) error {
	if s.characters == nil {
		return errors.New("campaigns: SetCharacters was not called")
	}
	spent, err := q.IncrementInviteUses(ctx, campaignsdb.IncrementInviteUsesParams{ID: invite.ID, Now: now})
	if err != nil {
		return fmt.Errorf("spend an invite use: %w", err)
	}
	if spent == 0 {
		// Unreachable while the row is locked and checked by the caller.
		return &unusableInviteError{state: campaignsv1.InviteState_INVITE_STATE_USED_UP}
	}
	if _, err := q.ActivatePendingMember(ctx, campaignsdb.ActivatePendingMemberParams{CampaignID: invite.CampaignID, UserID: userID}); err != nil {
		return fmt.Errorf("activate pending member: %w", err)
	}
	return s.characters.ApprovePendingCharacter(ctx, tx, invite.CampaignID, userID)
}

func inviteToProto(invite campaignsdb.CampaignInvite, now time.Time) *campaignsv1.Invite {
	res := &campaignsv1.Invite{
		Id:        invite.ID,
		MaxUses:   invite.MaxUses,
		UseCount:  invite.UseCount,
		CreatedAt: timestamppb.New(invite.CreatedAt),
		ExpiresAt: timestamppb.New(invite.ExpiresAt),
		State:     inviteState(invite, now),
		// Whoever accepts it becomes a pending member (RN-15).
		RequiresApproval: invite.RequiresApproval,
	}
	if invite.RevokedAt != nil {
		res.RevokedAt = timestamppb.New(*invite.RevokedAt)
	}
	return res
}

// parseUUID returns id in canonical form, or false if it is not a UUID.
func parseUUID(id string) (string, bool) {
	u, err := uuid.Parse(id)
	if err != nil {
		return "", false
	}
	return u.String(), true
}
