package campaigns

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/timestamppb"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/campaigns/campaignsdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/platform/names"
	"github.com/PuraFome/meuRPG/backend/internal/platform/rpcerr"
	"github.com/PuraFome/meuRPG/backend/internal/platform/secret"
)

// Every handler starts with one explicit check: authz.RequireSignedIn when
// any signed-in user may call it, authz.RequireCampaignMember or
// authz.RequireCampaignRole when it is about one campaign. The check's error
// is already the right Connect error, so handlers return it as is.
//
// GetCampaign is the one handler here that lets a pending member in
// (authz.RequireCampaignMemberOrPending, RN-15): they see only the
// campaign's name (campaignToProto). ListMyCampaigns lists their pending
// campaigns the same way.

// CreateCampaign implements campaignsv1connect.CampaignServiceHandler.
func (s *Service) CreateCampaign(
	ctx context.Context,
	req *connect.Request[campaignsv1.CreateCampaignRequest],
) (*connect.Response[campaignsv1.CreateCampaignResponse], error) {
	userID, err := authz.RequireSignedIn(ctx)
	if err != nil {
		return nil, err
	}
	name, err := names.Clean(req.Msg.GetName(), MaxNameLength)
	if err != nil {
		return nil, invalidArgument("name", err)
	}
	xpMode, ok := xpModeToDB[req.Msg.GetXpMode()]
	if !ok {
		return nil, invalidArgument("xp_mode", errors.New("must be enemies, gold or milestones"))
	}

	// Who may create campaigns at all (RN-30): a read through the pool, so it
	// comes before the transaction.
	allowed, err := s.canCreate(ctx, userID)
	if err != nil {
		return nil, s.dbError(ctx, "check who may create campaigns", err)
	}
	if !allowed {
		return nil, errCreationRefused(campaignsv1.CampaignCreationRefusedReason_CAMPAIGN_CREATION_REFUSED_REASON_NOT_ALLOWED, 0)
	}
	key, err := idem.Clean(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	// The key is unique for the user, and kept with a hash of the whole request: a retry
	// returns the first campaign only when it is the same request.
	scopedKey, requestHash := idem.Scope(userID, key), idem.Hash(req.Msg)

	// The campaign and its master are created together, or not at all.
	var campaign campaignsdb.Campaign
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		var replayed bool
		var err error
		campaign, replayed, err = idem.Create(ctx, scopedKey, requestHash, q.GetCampaignByCreateKey,
			func(c campaignsdb.Campaign) *string { return c.CreateHash },
			func() (campaignsdb.Campaign, error) {
				// The cap is counted only for a new campaign (a retry of one already
				// made returns it, even at the cap), in the transaction that inserts:
				// two creations at the same time cannot both slip under it
				// (SERIALIZABLE makes one retry and count again) (RN-30).
				// A negative cap is MAX_CAMPAIGNS_PER_USER=off (local and CI stacks only).
				if s.maxCampaigns > 0 {
					mastered, err := q.CountMasteredCampaigns(ctx, userID)
					if err != nil {
						return campaignsdb.Campaign{}, fmt.Errorf("count the campaigns the caller is master of: %w", err)
					}
					if int(mastered) >= s.maxCampaigns {
						return campaignsdb.Campaign{}, errCreationRefused(campaignsv1.CampaignCreationRefusedReason_CAMPAIGN_CREATION_REFUSED_REASON_LIMIT_REACHED, s.maxCampaigns)
					}
				}
				c, err := q.InsertCampaign(ctx, campaignsdb.InsertCampaignParams{
					Name:       name,
					XpMode:     xpMode,
					CreatedBy:  userID,
					CreateKey:  scopedKey,
					CreateHash: requestHash,
				})
				if err != nil && !errors.Is(err, pgx.ErrNoRows) {
					return c, fmt.Errorf("insert campaign: %w", err)
				}
				return c, err
			})
		if err != nil {
			return err
		}
		if replayed {
			return nil // the first call made the campaign and its master
		}
		_, err = q.InsertMember(ctx, campaignsdb.InsertMemberParams{
			CampaignID: campaign.ID,
			UserID:     userID,
			Role:       string(authz.RoleMaster),
			Status:     string(authz.StatusActive),
		})
		if err != nil {
			return fmt.Errorf("insert master: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "create a campaign", err)
	}
	return connect.NewResponse(&campaignsv1.CreateCampaignResponse{
		Campaign: campaignToProto(campaign, authz.RoleMaster, false),
	}), nil
}

// ListMyCampaigns implements campaignsv1connect.CampaignServiceHandler.
func (s *Service) ListMyCampaigns(
	ctx context.Context,
	_ *connect.Request[campaignsv1.ListMyCampaignsRequest],
) (*connect.Response[campaignsv1.ListMyCampaignsResponse], error) {
	userID, err := authz.RequireSignedIn(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListCampaignsOfUser(ctx, userID)
	if err != nil {
		return nil, s.dbError(ctx, "list campaigns", err)
	}
	res := &campaignsv1.ListMyCampaignsResponse{}
	for _, row := range rows {
		pending := row.Status != string(authz.StatusActive)
		res.Campaigns = append(res.Campaigns, campaignToProto(row.Campaign, authz.Role(row.Role), pending))
	}
	return connect.NewResponse(res), nil
}

// GetCampaign implements campaignsv1connect.CampaignServiceHandler.
func (s *Service) GetCampaign(
	ctx context.Context,
	req *connect.Request[campaignsv1.GetCampaignRequest],
) (*connect.Response[campaignsv1.GetCampaignResponse], error) {
	// A pending member may call it (RN-15): campaignToProto gives them only
	// the name.
	m, err := authz.RequireCampaignMemberOrPending(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	campaign, err := s.queries.GetCampaign(ctx, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "get a campaign", err)
	}
	res := campaignToProto(campaign, m.Role, m.Pending)
	if !m.Pending {
		// The caller's own dice preference (RN-18): the one list-shaped
		// call that reads it, because the campaign page needs it.
		pref, err := s.queries.GetMemberDicePreference(ctx, campaignsdb.GetMemberDicePreferenceParams{CampaignID: m.CampaignID, UserID: m.UserID})
		if err != nil {
			return nil, s.dbError(ctx, "get the dice preference", err)
		}
		res.MyDicePreference = dicePreferenceFromDB[pref]
	}
	return connect.NewResponse(&campaignsv1.GetCampaignResponse{Campaign: res}), nil
}

// ListMembers implements campaignsv1connect.CampaignServiceHandler.
func (s *Service) ListMembers(
	ctx context.Context,
	req *connect.Request[campaignsv1.ListMembersRequest],
) (*connect.Response[campaignsv1.ListMembersResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	members, err := s.queries.ListMembers(ctx, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "list members", err)
	}

	userIDs := make([]string, len(members))
	for i, member := range members {
		userIDs[i] = member.UserID
	}
	displayNames, err := s.profiles.DisplayNames(ctx, userIDs)
	if err != nil {
		return nil, s.dbError(ctx, "read display names", err)
	}

	res := &campaignsv1.ListMembersResponse{}
	for _, member := range members {
		entry := &campaignsv1.Member{
			UserId:      member.UserID,
			Role:        roleToProto(authz.Role(member.Role)),
			DisplayName: displayNames[member.UserID],
			JoinedAt:    timestamppb.New(member.JoinedAt),
		}
		// Only the master sees the players' dice choices (RN-18).
		if m.Role == authz.RoleMaster {
			entry.DicePreference = dicePreferenceFromDB[member.DicePreference]
		}
		res.Members = append(res.Members, entry)
	}
	return connect.NewResponse(res), nil
}

// ListPendingMembers implements campaignsv1connect.CampaignServiceHandler.
func (s *Service) ListPendingMembers(
	ctx context.Context,
	req *connect.Request[campaignsv1.ListPendingMembersRequest],
) (*connect.Response[campaignsv1.ListPendingMembersResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListPendingMembersWithoutCharacter(ctx, campaignsdb.ListPendingMembersWithoutCharacterParams{CampaignID: m.CampaignID, Now: s.now()})
	if err != nil {
		return nil, s.dbError(ctx, "list pending members", err)
	}

	userIDs := make([]string, len(rows))
	for i, row := range rows {
		userIDs[i] = row.UserID
	}
	displayNames, err := s.profiles.DisplayNames(ctx, userIDs)
	if err != nil {
		return nil, s.dbError(ctx, "read display names", err)
	}

	res := &campaignsv1.ListPendingMembersResponse{}
	for _, row := range rows {
		pending := &campaignsv1.PendingMember{
			UserId:      row.UserID,
			DisplayName: displayNames[row.UserID],
			JoinedAt:    timestamppb.New(row.JoinedAt),
		}
		// The query only returns rows with a deadline; the nil check keeps
		// a future change from panicking here.
		if row.PendingExpiresAt != nil {
			pending.ExpiresAt = timestamppb.New(*row.PendingExpiresAt)
		}
		res.Members = append(res.Members, pending)
	}
	return connect.NewResponse(res), nil
}

// RemovePendingMember implements campaignsv1connect.CampaignServiceHandler.
func (s *Service) RemovePendingMember(
	ctx context.Context,
	req *connect.Request[campaignsv1.RemovePendingMemberRequest],
) (*connect.Response[campaignsv1.RemovePendingMemberResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	userID, ok := parseUUID(req.Msg.GetUserId())
	if !ok {
		return nil, errMemberNotFound() // no membership could have this ID
	}

	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		// The DELETE itself says "pending and without a character", so a
		// character created at the same moment cannot be orphaned: the two
		// transactions conflict, and one of them retries (40001).
		deleted, err := q.DeletePendingMemberWithoutCharacter(ctx, campaignsdb.DeletePendingMemberWithoutCharacterParams{
			CampaignID: m.CampaignID, UserID: userID,
		})
		if err != nil {
			return fmt.Errorf("delete pending member: %w", err)
		}
		if deleted > 0 {
			return nil
		}
		// Nothing deleted: say why.
		_, err = q.GetMembership(ctx, campaignsdb.GetMembershipParams{CampaignID: m.CampaignID, UserID: userID, Now: s.now()})
		if errors.Is(err, pgx.ErrNoRows) {
			return errMemberNotFound()
		}
		if err != nil {
			return fmt.Errorf("get membership: %w", err)
		}
		return connect.NewError(connect.CodeFailedPrecondition,
			errors.New("only a pending member without a character can be removed here; reject a pending character with RejectCharacter"))
	})
	if err != nil {
		return nil, s.dbError(ctx, "remove a pending member", err)
	}
	return connect.NewResponse(&campaignsv1.RemovePendingMemberResponse{}), nil
}

// errMemberNotFound is the answer for a user ID with no membership in the
// campaign.
func errMemberNotFound() error {
	return connect.NewError(connect.CodeNotFound, errors.New("member not found"))
}

// CreateInvite implements campaignsv1connect.CampaignServiceHandler.
func (s *Service) CreateInvite(
	ctx context.Context,
	req *connect.Request[campaignsv1.CreateInviteRequest],
) (*connect.Response[campaignsv1.CreateInviteResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	maxUses, lifetime, err := inviteLimits(req.Msg)
	if err != nil {
		return nil, err
	}

	token, tokenHash := newInviteToken()
	now := s.now()
	var invite campaignsdb.CampaignInvite
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		invite, err = s.queries.WithTx(tx).InsertInvite(ctx, campaignsdb.InsertInviteParams{
			CampaignID:       m.CampaignID,
			TokenHash:        tokenHash,
			CreatedBy:        m.UserID,
			MaxUses:          maxUses,
			CreatedAt:        now,
			ExpiresAt:        now.Add(lifetime),
			RequiresApproval: req.Msg.GetRequiresApproval(),
		})
		if err != nil {
			return fmt.Errorf("insert invite: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "create an invite", err)
	}
	return connect.NewResponse(&campaignsv1.CreateInviteResponse{
		Invite: inviteToProto(invite, now),
		Token:  token,
	}), nil
}

// ListInvites implements campaignsv1connect.CampaignServiceHandler.
func (s *Service) ListInvites(
	ctx context.Context,
	req *connect.Request[campaignsv1.ListInvitesRequest],
) (*connect.Response[campaignsv1.ListInvitesResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	invites, err := s.queries.ListInvites(ctx, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "list invites", err)
	}
	now := s.now()
	res := &campaignsv1.ListInvitesResponse{}
	for _, invite := range invites {
		res.Invites = append(res.Invites, inviteToProto(invite, now))
	}
	return connect.NewResponse(res), nil
}

// RevokeInvite implements campaignsv1connect.CampaignServiceHandler.
func (s *Service) RevokeInvite(
	ctx context.Context,
	req *connect.Request[campaignsv1.RevokeInviteRequest],
) (*connect.Response[campaignsv1.RevokeInviteResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	inviteID, ok := parseUUID(req.Msg.GetInviteId())
	if !ok {
		return nil, errInviteNotFound()
	}

	now := s.now()
	var invite campaignsdb.CampaignInvite
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		invite, err = s.queries.WithTx(tx).RevokeInvite(ctx, campaignsdb.RevokeInviteParams{
			CampaignID: m.CampaignID,
			ID:         inviteID,
			Now:        &now,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return errInviteNotFound()
		}
		if err != nil {
			return fmt.Errorf("revoke invite: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "revoke an invite", err)
	}
	return connect.NewResponse(&campaignsv1.RevokeInviteResponse{
		Invite: inviteToProto(invite, now),
	}), nil
}

// AcceptInvite implements campaignsv1connect.CampaignServiceHandler. The
// rules are in acceptInvite (invites.go).
func (s *Service) AcceptInvite(
	ctx context.Context,
	req *connect.Request[campaignsv1.AcceptInviteRequest],
) (*connect.Response[campaignsv1.AcceptInviteResponse], error) {
	userID, err := authz.RequireSignedIn(ctx)
	if err != nil {
		return nil, err
	}
	if req.Msg.GetToken() == "" {
		return nil, invalidArgument("token", names.ErrEmpty)
	}
	tokenHash, ok := secret.Hash(req.Msg.GetToken())
	if !ok {
		return nil, errInviteNotFound() // no invite could have this token
	}

	joined, err := s.acceptInvite(ctx, tokenHash, userID)
	if unusable, ok := errors.AsType[*unusableInviteError](err); ok {
		return nil, errInviteUnusable(unusable.state)
	}
	switch {
	case errors.Is(err, errNoInvite):
		return nil, errInviteNotFound()
	case err != nil:
		return nil, s.dbError(ctx, "accept an invite", err)
	}
	return connect.NewResponse(&campaignsv1.AcceptInviteResponse{
		Campaign:      campaignToProto(joined.campaign, joined.role, joined.pending),
		AlreadyMember: joined.alreadyMember,
	}), nil
}

// dbError maps an error from the database to the Connect error the client gets
// (see platform/rpcerr). A row that vanished between the authorization check
// and the next query is the one case the campaigns module answers itself.
func (s *Service) dbError(ctx context.Context, action string, err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		// The campaign passed the authorization check and was deleted
		// before the next query: it is gone now.
		return connect.NewError(connect.CodeNotFound, errors.New("campaign not found"))
	}
	return rpcerr.FromDB(ctx, s.logger, "campaigns", action, err)
}

// invalidArgument is the error for a request field that breaks a rule. The
// message names the field and the rule, never the value, which could be
// personal data.
func invalidArgument(field string, err error) error {
	return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("%s %w", field, err))
}

// Conversions between the database's text values and the API's enums.
var (
	xpModeToDB = map[campaignsv1.XpMode]string{
		campaignsv1.XpMode_XP_MODE_ENEMIES:    "enemies",
		campaignsv1.XpMode_XP_MODE_GOLD:       "gold",
		campaignsv1.XpMode_XP_MODE_MILESTONES: "milestones",
	}
	xpModeFromDB = map[string]campaignsv1.XpMode{
		"enemies":    campaignsv1.XpMode_XP_MODE_ENEMIES,
		"gold":       campaignsv1.XpMode_XP_MODE_GOLD,
		"milestones": campaignsv1.XpMode_XP_MODE_MILESTONES,
	}
)

func roleToProto(role authz.Role) campaignsv1.Role {
	switch role {
	case authz.RoleMaster:
		return campaignsv1.Role_ROLE_MASTER
	case authz.RolePlayer:
		return campaignsv1.Role_ROLE_PLAYER
	default:
		return campaignsv1.Role_ROLE_UNSPECIFIED
	}
}

// campaignToProto builds the Campaign the caller sees. A pending member
// (RN-15, MR-024) is not a member yet: they see only the id, the name and
// their role, with awaiting_approval set, enough for the app to say
// "esperando a aprovação do mestre".
func campaignToProto(c campaignsdb.Campaign, myRole authz.Role, pending bool) *campaignsv1.Campaign {
	if pending {
		return &campaignsv1.Campaign{
			Id:               c.ID,
			Name:             c.Name,
			MyRole:           campaignsv1.Role_ROLE_PLAYER,
			AwaitingApproval: true,
		}
	}
	return &campaignsv1.Campaign{
		Id:        c.ID,
		Name:      c.Name,
		XpMode:    xpModeFromDB[c.XpMode],
		CreatedAt: timestamppb.New(c.CreatedAt),
		MyRole:    roleToProto(myRole),
		DiceMode:  diceModeFromDB[c.DiceMode],
	}
}
