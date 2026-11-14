package characters

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/timestamppb"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/identity"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/logging"
	"github.com/PuraFome/meuRPG/backend/internal/platform/ratelimit"
	"github.com/PuraFome/meuRPG/backend/internal/platform/secret"
	"github.com/PuraFome/meuRPG/backend/internal/platform/sessiontell"
	"github.com/PuraFome/meuRPG/backend/internal/platform/uid"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// Reserved characters and claim links (MR-049).
//
// A reserved character is a player character the master made for a player to take: no
// owner, and invisible to every player (RN-10) until claimed. The master sends the
// player a link, https://<app>/claim#t=<token>. The token is a random secret (package
// secret) that CreateClaimLink returns once and the database keeps only as a SHA-256;
// it travels in the URL fragment, which browsers never send to a server, and the app
// posts it in a request body (ADR-0009), exactly like an invite's.
//
// Opening the link signed out shows one page for every link: what a claim link is and
// "Entrar com Google". Signing in goes through POST /auth/login with the intent
// character_claim, which only brings the person back to the card: Complete keeps the
// token's hash for ten minutes (claim_sign_ins), and PreviewClaim and ClaimCharacter
// read it when the page sends no token. Signing in never claims: only ClaimCharacter
// does, and it is a deliberate click on the card.
//
// Every refusal of a link that does not work (never made, expired, used, revoked, its
// character gone, or a claim that lost a race with a revoke or with another claim) is
// the same error, errClaimLinkUnusable, so nobody learns which. The one specific
// refusal is RN-03: the person already has a living character in the campaign.

// ClaimIntentKind is the sign-in intent that brings a person back to a claim card.
const ClaimIntentKind = "character_claim"

// The claim link's validity, in days (CreateClaimLinkRequest.validity_days).
const (
	defaultClaimDays = 7
	maxClaimDays     = 30
	// claimSignInTTL is how long the link a person signed in with is kept: as long as
	// the login state that carried it.
	claimSignInTTL = 10 * time.Minute
)

// claimDaysAllowed are the validities the master may choose.
var claimDaysAllowed = map[int32]bool{1: true, defaultClaimDays: true, maxClaimDays: true}

// errClaimLinkUnusable is the answer for every link that cannot be used, whatever the
// reason: one code and one sentence (PreviewClaim, ClaimCharacter).
func errClaimLinkUnusable() error {
	return connect.NewError(connect.CodeNotFound, errors.New("this claim link cannot be used"))
}

// claimUsedError is the transaction's error for a revoke that lost to a claim; the
// handler turns it into the CLAIM_LINK_USED detail, with the player's name read after
// the transaction.
type claimUsedError struct{ userID *string }

func (*claimUsedError) Error() string { return "the claim link was used" }

// errBlockedUsed is RevokeClaimLink's refusal when a player used the link first.
func errBlockedUsed(characterID, playerDisplayName string) error {
	err := connect.NewError(connect.CodeFailedPrecondition, errors.New("a player already used this link and took the character"))
	if detail, detailErr := connect.NewErrorDetail(&charactersv1.CharacterBlocked{
		Reason:            charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_CLAIM_LINK_USED,
		CharacterId:       characterID,
		PlayerDisplayName: playerDisplayName,
	}); detailErr == nil {
		err.AddDetail(detail)
	}
	return err
}

// livingForClaimError is the claim's RN-03 refusal, carrying the living character's name.
type livingForClaimError struct{ campaignID, id, name string }

func (*livingForClaimError) Error() string {
	return "you already have a living character in this campaign"
}

func (e *livingForClaimError) connectError() error {
	err := connect.NewError(connect.CodeFailedPrecondition, errors.New("you already have a living character in this campaign"))
	if detail, detailErr := connect.NewErrorDetail(&charactersv1.CharacterBlocked{
		Reason:        charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_LIVING_CHARACTER_EXISTS,
		CharacterId:   e.id,
		CharacterName: e.name,
		CampaignId:    e.campaignID,
	}); detailErr == nil {
		err.AddDetail(detail)
	}
	return err
}

// claimLimits are the limits on PreviewClaim and ClaimCharacter: per person and per
// client address, tighter than the general ones, since a person has no reason to try
// more than a handful of links a minute.
type claimLimits struct {
	user, ip *ratelimit.Limiter
}

// The claim limits: a person has no reason to try more than a few links a minute, and
// one table behind one address (a master and six players) tries far less than the address limit.
const (
	claimUserBurst   = 20
	claimUserEvery   = 3 * time.Second
	claimIPBurst     = 60
	claimIPEvery     = time.Second
	claimGlobalBurst = 200
	claimGlobalEvery = 250 * time.Millisecond
	claimMaxClients  = 10000
)

func newClaimLimits() claimLimits {
	global := ratelimit.Rate{Burst: claimGlobalBurst, Every: claimGlobalEvery}
	return claimLimits{
		user: ratelimit.New(ratelimit.Config{PerClient: ratelimit.Rate{Burst: claimUserBurst, Every: claimUserEvery}, Global: global, MaxClients: claimMaxClients}),
		ip:   ratelimit.New(ratelimit.Config{PerClient: ratelimit.Rate{Burst: claimIPBurst, Every: claimIPEvery}, Global: global, MaxClients: claimMaxClients}),
	}
}

// allowClaim spends one try of the person and of the caller's address, or returns the
// error to answer with.
func (s *Service) allowClaim(userID string, peer string, header http.Header) error {
	r := &http.Request{RemoteAddr: peer, Header: header}
	if ok, wait := s.claims.ip.Allow(ratelimit.ClientKey(r, s.trustedProxyHops)); !ok {
		return ratelimit.RPCError(wait)
	}
	if ok, wait := s.claims.user.Allow(userID); !ok {
		return ratelimit.RPCError(wait)
	}
	return nil
}

// CreateClaimLink implements charactersv1connect.CharacterServiceHandler.
func (s *Service) CreateClaimLink(
	ctx context.Context,
	req *connect.Request[charactersv1.CreateClaimLinkRequest],
) (*connect.Response[charactersv1.CreateClaimLinkResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	id, ok := uid.Canonical(req.Msg.GetCharacterId())
	if !ok {
		return nil, errCharacterNotFound()
	}
	days := req.Msg.GetValidityDays()
	if days == 0 {
		days = defaultClaimDays
	}
	if !claimDaysAllowed[days] {
		return nil, invalidArgument(fieldErr("validity_days", "must be 1, 7 or 30"))
	}

	token, hash := secret.New()
	now := s.now()
	expires := now.Add(time.Duration(days) * 24 * time.Hour)
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		row, err := visibleForUpdate(ctx, q, m, id)
		if err != nil {
			return err
		}
		if row.Kind != kindPlayer {
			return invalidArgument(fieldErr("character_id", "is an NPC: only a reserved player character has a claim link"))
		}
		if !row.Reserved {
			return errBlocked(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_NOT_RESERVED, row.ID)
		}
		// A new link replaces the live one: there is never more than one.
		if _, err := q.RevokeLiveClaimLinks(ctx, charactersdb.RevokeLiveClaimLinksParams{CharacterID: row.ID, Now: &now}); err != nil {
			return wrap("revoke the live claim links", err)
		}
		if _, err := q.InsertClaimLink(ctx, charactersdb.InsertClaimLinkParams{
			CampaignID: m.CampaignID, CharacterID: row.ID, TokenHash: hash, CreatedBy: m.UserID, Now: now, ExpiresAt: expires,
		}); err != nil {
			return wrap("insert a claim link", err)
		}
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "create a claim link", err)
	}
	logging.Event(ctx, s.logger, "character.claim_link_created", slog.String("character_id", id), slog.Int("validity_days", int(days)))
	return connect.NewResponse(&charactersv1.CreateClaimLinkResponse{Token: token, ExpiresAt: timestamppb.New(expires)}), nil
}

// masterAndCharacter is the first check of the master's calls on one character: the
// caller is the campaign's master, and the character's ID is a UUID (an ID that is not
// one names nothing). It returns the membership and the ID in canonical form.
func masterAndCharacter(ctx context.Context, campaignID, characterID string) (authz.Membership, string, error) {
	m, err := authz.RequireCampaignRole(ctx, campaignID, authz.RoleMaster)
	if err != nil {
		return authz.Membership{}, "", err
	}
	id, ok := uid.Canonical(characterID)
	if !ok {
		return authz.Membership{}, "", errCharacterNotFound()
	}
	return m, id, nil
}

// RevokeClaimLink implements charactersv1connect.CharacterServiceHandler.
func (s *Service) RevokeClaimLink(
	ctx context.Context,
	req *connect.Request[charactersv1.RevokeClaimLinkRequest],
) (*connect.Response[charactersv1.RevokeClaimLinkResponse], error) {
	m, id, err := masterAndCharacter(ctx, req.Msg.GetCampaignId(), req.Msg.GetCharacterId())
	if err != nil {
		return nil, err
	}
	now := s.now()
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		return s.revokeClaimLinkTx(ctx, s.queries.WithTx(tx), m, id, now)
	})
	if used, ok := errors.AsType[*claimUsedError](err); ok {
		return nil, s.usedLinkError(ctx, id, used)
	}
	if err != nil {
		return nil, s.dbError(ctx, "revoke a claim link", err)
	}
	logging.Event(ctx, s.logger, "character.claim_link_revoked", slog.String("character_id", id))
	return connect.NewResponse(&charactersv1.RevokeClaimLinkResponse{}), nil
}

// revokeClaimLinkTx is RevokeClaimLink's transaction: it locks the character, then its
// latest link, and revokes that link if it is live.
func (s *Service) revokeClaimLinkTx(ctx context.Context, q *charactersdb.Queries, m authz.Membership, id string, now time.Time) error {
	// The character first, then its link: ClaimCharacter locks them in this order too,
	// so the two never wait for each other in a circle.
	row, err := visibleForUpdate(ctx, q, m, id)
	if err != nil {
		return err
	}
	link, err := q.GetLatestClaimLinkForUpdate(ctx, charactersdb.GetLatestClaimLinkForUpdateParams{CampaignID: m.CampaignID, CharacterID: row.ID})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return s.requireReservedOrClaimed(row)
	case err != nil:
		return wrap("read the claim link", err)
	}
	return revokeLatestLink(ctx, q, row, link, now)
}

// revokeLatestLink revokes the character's latest link, unless it was used or revoked
// already: a used one is the CLAIM_LINK_USED refusal while its player still has the
// character, and nothing to do once the character went back to the reserve.
func revokeLatestLink(ctx context.Context, q *charactersdb.Queries, row charactersdb.Character, link charactersdb.ClaimLink, now time.Time) error {
	if link.UsedAt != nil {
		if !row.Reserved {
			// A player took the character first (the claim and the revoke raced, or the
			// list was stale).
			return &claimUsedError{userID: link.UsedBy}
		}
		return nil // given back to the reserve since: no live link
	}
	if link.RevokedAt != nil {
		return nil // revoked already
	}
	if _, err := q.RevokeClaimLinkByID(ctx, charactersdb.RevokeClaimLinkByIDParams{ID: link.ID, Now: now}); err != nil {
		return wrap("revoke the claim link", err)
	}
	return nil
}

// requireReservedOrClaimed is RevokeClaimLink's answer for a character with no link at
// all: nothing to revoke, which is fine for a reserved character and not for an NPC or
// for one that has an owner it never got by a link.
func (s *Service) requireReservedOrClaimed(row charactersdb.Character) error {
	if row.Kind != kindPlayer {
		return invalidArgument(fieldErr("character_id", "is an NPC: only a reserved player character has a claim link"))
	}
	if !row.Reserved {
		return errBlocked(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_NOT_RESERVED, row.ID)
	}
	return nil
}

// usedLinkError is the CLAIM_LINK_USED refusal, with the name of the player who took
// the character. The name is read after the transaction, so the error carries the
// player's ID until then.
func (s *Service) usedLinkError(ctx context.Context, characterID string, used *claimUsedError) error {
	var name string
	if used.userID != nil {
		names, err := s.displayNames(ctx, []string{*used.userID})
		if err != nil {
			return s.dbError(ctx, "read a display name", err)
		}
		name = names[*used.userID]
	}
	return errBlockedUsed(characterID, name)
}

// ReturnCharacterToReserve implements charactersv1connect.CharacterServiceHandler.
func (s *Service) ReturnCharacterToReserve(
	ctx context.Context,
	req *connect.Request[charactersv1.ReturnCharacterToReserveRequest],
) (*connect.Response[charactersv1.ReturnCharacterToReserveResponse], error) {
	m, id, err := masterAndCharacter(ctx, req.Msg.GetCampaignId(), req.Msg.GetCharacterId())
	if err != nil {
		return nil, err
	}
	content, err := s.contentFor(ctx, nil, m.CampaignID) // before the write: a failure after the commit would make the client retry it
	if err != nil {
		return nil, s.dbError(ctx, "read rules content", err)
	}
	now := s.now()
	var row charactersdb.Character
	// Under the funnel: the session's row first, then the character's, and leaving the party moves the session's number once.
	err = s.inSession(ctx, m.CampaignID, func(tx pgx.Tx, t sessiontell.Tell) error {
		var err error
		row, err = s.returnToReserveTx(ctx, tx, m, id, now)
		if err == nil {
			t.XPChanged()
		}
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, "return a character to the reserve", err)
	}
	logging.Event(ctx, s.logger, "character.returned_to_reserve", slog.String("character_id", id))
	c, err := s.characterAfterWrite(ctx, content, row, m)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&charactersv1.ReturnCharacterToReserveResponse{Character: c}), nil
}

// returnToReserveTx is ReturnCharacterToReserve's transaction: a living player
// character a claim brought to its player, and in no combat, loses its owner and goes
// back to the reserve, and whatever link it has left stops working.
func (s *Service) returnToReserveTx(ctx context.Context, tx pgx.Tx, m authz.Membership, id string, now time.Time) (charactersdb.Character, error) {
	q := s.queries.WithTx(tx)
	current, err := visibleForUpdate(ctx, q, m, id)
	if err != nil {
		return charactersdb.Character{}, err
	}
	if !claimedThroughLink(current) {
		return charactersdb.Character{}, errBlocked(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_NOT_CLAIMED, current.ID)
	}
	if err := s.requireOutOfCombat(ctx, tx, m.CampaignID, current.ID); err != nil {
		return charactersdb.Character{}, err
	}
	row, err := q.ReturnCharacterToReserve(ctx, charactersdb.ReturnCharacterToReserveParams{CampaignID: m.CampaignID, ID: current.ID})
	if err != nil {
		return charactersdb.Character{}, wrap("return a character to the reserve", err)
	}
	// Whatever link is left (none is live: a used link is spent) cannot work again.
	if _, err := q.RevokeLiveClaimLinks(ctx, charactersdb.RevokeLiveClaimLinksParams{CharacterID: current.ID, Now: &now}); err != nil {
		return charactersdb.Character{}, wrap("revoke the live claim links", err)
	}
	return row, nil
}

// claimedThroughLink says whether a character is one ReturnCharacterToReserve takes: a
// living player character, not reserved, that a claim link brought to its player.
func claimedThroughLink(row charactersdb.Character) bool {
	return row.Kind == kindPlayer && !row.Reserved && row.ClaimedAt != nil && row.Status == statusActive
}

// requireOutOfCombat refuses a character that is a combatant of the campaign's combat
// (MR-049), inside tx. Without the play module connected, no character is in one.
func (s *Service) requireOutOfCombat(ctx context.Context, tx pgx.Tx, campaignID, characterID string) error {
	if s.creatureHost == nil {
		return nil
	}
	inCombat, err := s.creatureHost.CharacterInCombat(ctx, tx, campaignID, characterID)
	if err != nil {
		return wrap("find the character in a combat", err)
	}
	if inCombat {
		return errBlocked(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_CHARACTER_IN_COMBAT, characterID)
	}
	return nil
}

// characterAfterWrite is the character a write answers with, read after the commit;
// a failure is the database's refusal of reading a character.
func (s *Service) characterAfterWrite(ctx context.Context, content *rules.Content, row charactersdb.Character, m authz.Membership) (*charactersv1.Character, error) {
	c, err := s.character(ctx, content, row, m)
	if err != nil {
		return nil, s.dbError(ctx, "read a character", err)
	}
	return c, nil
}

// DeleteReservedCharacter implements charactersv1connect.CharacterServiceHandler.
func (s *Service) DeleteReservedCharacter(
	ctx context.Context,
	req *connect.Request[charactersv1.DeleteReservedCharacterRequest],
) (*connect.Response[charactersv1.DeleteReservedCharacterResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	id, ok := uid.Canonical(req.Msg.GetCharacterId())
	if !ok {
		return nil, errCharacterNotFound()
	}
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		row, err := visibleForUpdate(ctx, q, m, id)
		if err != nil {
			return err
		}
		if !row.Reserved {
			return errBlocked(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_NOT_RESERVED, row.ID)
		}
		// The row's deletion takes its link, its story and the master's notes with it
		// (ON DELETE CASCADE): a link sent for it stops working in this transaction.
		n, err := q.DeleteReservedCharacter(ctx, charactersdb.DeleteReservedCharacterParams{CampaignID: m.CampaignID, ID: row.ID})
		if err != nil {
			return wrap("delete a reserved character", err)
		}
		if n == 0 {
			return errCharacterNotFound()
		}
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "delete a reserved character", err)
	}
	logging.Event(ctx, s.logger, "character.reserved_deleted", slog.String("character_id", id))
	return connect.NewResponse(&charactersv1.DeleteReservedCharacterResponse{}), nil
}

// claimTarget is the live link and the reserved character a token opens.
type claimTarget struct {
	link      charactersdb.ClaimLink
	character charactersdb.Character
}

// claimHash is the hash of the link a call is about: the token in the request, or the
// one the person signed in with when the request has none. Anything that is not a
// token, or no pending link, is the generic refusal.
func (s *Service) claimHash(ctx context.Context, q *charactersdb.Queries, userID, token string, now time.Time) ([]byte, error) {
	if token == "" {
		hash, err := q.GetClaimSignIn(ctx, charactersdb.GetClaimSignInParams{UserID: userID, Now: now})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, errClaimLinkUnusable()
			}
			return nil, wrap("read the link the person signed in with", err)
		}
		return hash, nil
	}
	hash, ok := secret.Hash(token)
	if !ok {
		return nil, errClaimLinkUnusable()
	}
	return hash, nil
}

// linkWorks says whether a claim link can be used at now.
func linkWorks(link charactersdb.ClaimLink, now time.Time) bool {
	return link.UsedAt == nil && link.RevokedAt == nil && now.Before(link.ExpiresAt)
}

// PreviewClaim implements charactersv1connect.CharacterServiceHandler.
func (s *Service) PreviewClaim(
	ctx context.Context,
	req *connect.Request[charactersv1.PreviewClaimRequest],
) (*connect.Response[charactersv1.PreviewClaimResponse], error) {
	userID, err := authz.RequireSignedIn(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.allowClaim(userID, req.Peer().Addr, req.Header()); err != nil {
		return nil, err
	}
	now := s.now()
	hash, err := s.claimHash(ctx, s.queries, userID, req.Msg.GetToken(), now)
	if err != nil {
		return nil, s.claimError(ctx, "read a claim link", err)
	}
	target, err := s.targetOf(ctx, s.queries, hash, now)
	if err != nil {
		return nil, s.claimError(ctx, "read a claim link", err)
	}
	campaignName, master, err := s.members.CampaignForClaim(ctx, nil, target.link.CampaignID, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errClaimLinkUnusable()
	}
	if err != nil {
		return nil, s.dbError(ctx, "read the campaign of a claim link", err)
	}
	if master {
		return connect.NewResponse(&charactersv1.PreviewClaimResponse{OwnLink: true}), nil
	}
	content, err := s.contentFor(ctx, nil, target.link.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "read rules content", err)
	}
	sheet, err := loadSheet(target.character.ID, target.character.Sheet)
	if err != nil {
		return nil, s.dbError(ctx, "read a sheet", err)
	}
	card := &charactersv1.ClaimCard{CharacterName: target.character.Name, CampaignName: campaignName}
	if full := sheet.GetFull(); full != nil {
		labels := content.Summary(buildOf(full))
		card.ClassSummary, card.RaceNamePt = labels.ClassSummaryPT, labels.RaceNamePT
	}
	names, err := s.displayNames(ctx, []string{target.link.CreatedBy})
	if err != nil {
		return nil, s.dbError(ctx, "read a display name", err)
	}
	card.SentByDisplayName = names[target.link.CreatedBy]
	return connect.NewResponse(&charactersv1.PreviewClaimResponse{Card: card}), nil
}

// targetOf finds the working link a hash opens and its reserved character, or returns
// the generic refusal. q reads through the pool or a transaction.
func (s *Service) targetOf(ctx context.Context, q *charactersdb.Queries, hash []byte, now time.Time) (claimTarget, error) {
	link, err := q.GetClaimLinkByTokenHash(ctx, hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return claimTarget{}, errClaimLinkUnusable()
	}
	if err != nil {
		return claimTarget{}, wrap("read a claim link", err)
	}
	if !linkWorks(link, now) {
		return claimTarget{}, errClaimLinkUnusable()
	}
	row, err := q.GetCharacter(ctx, charactersdb.GetCharacterParams{CampaignID: link.CampaignID, ID: link.CharacterID})
	if errors.Is(err, pgx.ErrNoRows) {
		return claimTarget{}, errClaimLinkUnusable()
	}
	if err != nil {
		return claimTarget{}, wrap("read a reserved character", err)
	}
	if !row.Reserved || row.Kind != kindPlayer {
		return claimTarget{}, errClaimLinkUnusable()
	}
	return claimTarget{link: link, character: row}, nil
}

// claimError turns an error of the claim's reads into what the client gets: the
// generic refusal as it is, anything else through dbError.
func (s *Service) claimError(ctx context.Context, action string, err error) error {
	if _, ok := errors.AsType[*connect.Error](err); ok {
		return err
	}
	return s.dbError(ctx, action, err)
}

// ClaimCharacter implements charactersv1connect.CharacterServiceHandler.
func (s *Service) ClaimCharacter(
	ctx context.Context,
	req *connect.Request[charactersv1.ClaimCharacterRequest],
) (*connect.Response[charactersv1.ClaimCharacterResponse], error) {
	userID, err := authz.RequireSignedIn(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.allowClaim(userID, req.Peer().Addr, req.Header()); err != nil {
		return nil, err
	}
	now := s.now()
	var res *charactersv1.ClaimCharacterResponse
	var linkCampaign string // the link's campaign, for an RN-03 refusal that comes from the unique index
	campaignID, err := s.claimCampaign(ctx, userID, req.Msg.GetToken(), now)
	if err != nil {
		return nil, s.claimError(ctx, "read a claim link", err)
	}
	// Under the funnel, on the link's campaign: the session's row first, then the character's, and joining the party moves the
	// session's number once. A token that opens no link has no campaign: its claim is refused in a plain transaction.
	claim := func(tx pgx.Tx, t sessiontell.Tell) error {
		claimed, err := s.claimTx(ctx, tx, userID, req.Msg.GetToken(), now, &linkCampaign)
		if err == nil {
			res = claimed
			t.XPChanged()
		}
		return err
	}
	if campaignID == "" {
		err = sessiontell.Direct{Pool: s.pool}.InSession(ctx, "", claim) // the claim is refused: nothing to tell, no session to lock
	} else {
		err = s.inSession(ctx, campaignID, claim)
	}
	if err != nil {
		return nil, s.claimFailure(ctx, err, userID, linkCampaign)
	}
	logging.Event(ctx, s.logger, "character.claimed", slog.String("campaign_id", res.GetCampaignId()), slog.String("character_id", res.GetCharacterId()))
	return connect.NewResponse(res), nil
}

// claimTx is ClaimCharacter's transaction: it locks the link's character and the link,
// checks that the caller may take it, and makes the caller its player. It sets
// *linkCampaign as soon as the link is known to work.
func (s *Service) claimTx(ctx context.Context, tx pgx.Tx, userID, token string, now time.Time, linkCampaign *string) (*charactersv1.ClaimCharacterResponse, error) {
	q := s.queries.WithTx(tx)
	target, err := s.lockClaimTarget(ctx, q, userID, token, now)
	if err != nil {
		return nil, err
	}
	link, row := target.link, target.character
	*linkCampaign = link.CampaignID
	if err := s.checkClaimant(ctx, tx, q, target, userID); err != nil {
		return nil, err
	}
	campaignName, err := s.members.JoinAsPlayer(ctx, tx, link.CampaignID, userID)
	if err != nil {
		return nil, wrap("join the campaign", err)
	}
	if err := spendClaim(ctx, q, target, userID, now); err != nil {
		return nil, err
	}
	return &charactersv1.ClaimCharacterResponse{
		CampaignId: link.CampaignID, CampaignName: campaignName, CharacterId: row.ID, CharacterName: row.Name,
	}, nil
}

// lockClaimTarget finds the link a claim is about and its character, locks both, and
// returns them while the link works and the character is still a reserved player
// character; anything else is the generic refusal.
func (s *Service) lockClaimTarget(ctx context.Context, q *charactersdb.Queries, userID, token string, now time.Time) (claimTarget, error) {
	hash, err := s.claimHash(ctx, q, userID, token, now)
	if err != nil {
		return claimTarget{}, err
	}
	// Find the character without a lock, then lock it, then the link, as RevokeClaimLink
	// does: a claim and a revoke wait for each other and the second sees the first.
	first, err := q.GetClaimLinkByTokenHash(ctx, hash)
	if err != nil {
		return claimTarget{}, claimReadError("read a claim link", err)
	}
	row, err := q.GetCharacterForUpdate(ctx, charactersdb.GetCharacterForUpdateParams{CampaignID: first.CampaignID, ID: first.CharacterID})
	if err != nil {
		return claimTarget{}, claimReadError("read a reserved character", err)
	}
	link, err := q.GetClaimLinkByTokenHashForUpdate(ctx, hash)
	if err != nil {
		return claimTarget{}, claimReadError("lock a claim link", err)
	}
	if !linkWorks(link, now) || !row.Reserved || row.Kind != kindPlayer {
		return claimTarget{}, errClaimLinkUnusable()
	}
	return claimTarget{link: link, character: row}, nil
}

// claimReadError is the claim's answer to a read that failed: no row is the generic
// refusal, anything else the database's error.
func claimReadError(what string, err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return errClaimLinkUnusable()
	}
	return wrap(what, err)
}

// checkClaimant refuses the caller who may not take the character: the campaign's
// master (their own link), and a player who has a living character in the campaign
// already (RN-03).
func (s *Service) checkClaimant(ctx context.Context, tx pgx.Tx, q *charactersdb.Queries, target claimTarget, userID string) error {
	campaignID := target.link.CampaignID
	_, master, err := s.members.CampaignForClaim(ctx, tx, campaignID, userID)
	if err != nil {
		return wrap("read the campaign of a claim link", err)
	}
	if master {
		return errBlocked(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_CLAIM_OWN_LINK, target.character.ID)
	}
	// RN-03: one living character per player per campaign. A dead one does not count.
	living, err := q.GetLivingPlayerCharacterID(ctx, charactersdb.GetLivingPlayerCharacterIDParams{CampaignID: campaignID, PlayerUserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return wrap("find the living character", err)
	}
	existing, err := q.GetCharacterNameAndOwner(ctx, charactersdb.GetCharacterNameAndOwnerParams{CampaignID: campaignID, ID: living})
	if err != nil {
		return wrap("read the living character", err)
	}
	return &livingForClaimError{campaignID: campaignID, id: living, name: existing.Name}
}

// spendClaim makes the caller the character's player, spends the link, and forgets the
// link the caller signed in with.
func spendClaim(ctx context.Context, q *charactersdb.Queries, target claimTarget, userID string, now time.Time) error {
	link := target.link
	if _, err := q.ClaimReservedCharacter(ctx, charactersdb.ClaimReservedCharacterParams{CampaignID: link.CampaignID, ID: target.character.ID, PlayerUserID: userID, Now: now}); err != nil {
		return claimReadError("claim a character", err)
	}
	spent, err := q.MarkClaimLinkUsed(ctx, charactersdb.MarkClaimLinkUsedParams{ID: link.ID, UsedBy: userID, Now: now})
	if err != nil {
		return wrap("spend a claim link", err)
	}
	if spent == 0 {
		return errClaimLinkUnusable() // unreachable under the row lock; kept so a change cannot make a free pass
	}
	if err := q.DeleteClaimSignIn(ctx, userID); err != nil {
		return wrap("delete the link signed in with", err)
	}
	return nil
}

// claimFailure turns the error of ClaimCharacter's transaction into what the client
// gets: RN-03 with the living character, also when the unique index found it, and
// otherwise as claimError does.
func (s *Service) claimFailure(ctx context.Context, err error, userID, linkCampaign string) error {
	if living, ok := errors.AsType[*livingForClaimError](err); ok {
		return living.connectError()
	}
	if db.IsUniqueViolation(err, "characters_one_living_player_character") {
		// Another claim or another character of this person won the race: answer as the
		// check above would have.
		living, lookupErr := s.queries.GetLivingPlayerCharacterID(ctx, charactersdb.GetLivingPlayerCharacterIDParams{CampaignID: linkCampaign, PlayerUserID: userID})
		if lookupErr != nil {
			living = ""
		}
		return (&livingForClaimError{campaignID: linkCampaign, id: living}).connectError()
	}
	return s.claimError(ctx, "claim a character", err)
}

// claimCampaign is the campaign of the link a token (or the sign-in) opens, read before the transaction so that it can start on the campaign's
// session; "" for a token that opens no link (the claim then answers the generic refusal). A link's campaign never changes.
func (s *Service) claimCampaign(ctx context.Context, userID, token string, now time.Time) (string, error) {
	hash, err := s.claimHash(ctx, s.queries, userID, token, now)
	if _, refused := errors.AsType[*connect.Error](err); refused {
		return "", nil // the generic refusal: the transaction answers it
	}
	if err != nil {
		return "", err
	}
	link, err := s.queries.GetClaimLinkByTokenHash(ctx, hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", wrap("read a claim link", err)
	}
	return link.CampaignID, nil
}

// ClaimIntent returns the handler for ClaimIntentKind, for identity.Config.Intents.
func (s *Service) ClaimIntent() identity.IntentHandler { return claimIntent{s: s} }

// claimIntent implements identity.IntentHandler for claim links: it only brings the
// person back to the claim page. It never claims.
type claimIntent struct{ s *Service }

// Prepare implements identity.IntentHandler: it keeps only the token's hash. A payload
// that cannot be a token is refused before the person is sent to the provider, which
// says nothing about whether the link works: every well-formed token goes through.
func (claimIntent) Prepare(payload string) ([]byte, error) {
	hash, ok := secret.Hash(payload)
	if !ok {
		return nil, errors.New("the payload is not a claim token")
	}
	return hash, nil
}

// claimReturnPath is where signing in with a claim link ends: the claim page, which
// then asks PreviewClaim with no token.
const claimReturnPath = "/claim"

// Complete implements identity.IntentHandler: it keeps the link's hash for the person for
// ten minutes and sends them to the claim page. It does not look the link up and does
// not claim: a link that does not work is found out by PreviewClaim, which shows the
// same page for every reason. The returned error never includes the hash.
func (i claimIntent) Complete(ctx context.Context, who identity.SignedIn, tokenHash []byte) (string, error) {
	userID := who.UserID()
	if userID == "" {
		return "", errors.New("character claim: no signed-in user")
	}
	return i.complete(ctx, userID, tokenHash)
}

// complete is Complete once the user is known.
func (i claimIntent) complete(ctx context.Context, userID string, tokenHash []byte) (string, error) {
	if len(tokenHash) != secret.Size {
		return claimReturnPath, errors.New("character claim: the stored data is not a token hash")
	}
	expires := i.s.now().Add(claimSignInTTL)
	err := db.InTx(ctx, i.s.pool, func(tx pgx.Tx) error {
		return i.s.queries.WithTx(tx).UpsertClaimSignIn(ctx, charactersdb.UpsertClaimSignInParams{UserID: userID, TokenHash: tokenHash, ExpiresAt: expires})
	})
	if err != nil {
		return claimReturnPath, fmt.Errorf("character claim: keep the link: %w", err)
	}
	return claimReturnPath, nil
}

// latestClaimLinks reads the latest claim link of each character of the campaign that
// has one, by character ID.
func (s *Service) latestClaimLinks(ctx context.Context, campaignID string) (map[string]charactersdb.ListLatestClaimLinksRow, error) {
	rows, err := s.queries.ListLatestClaimLinks(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]charactersdb.ListLatestClaimLinksRow, len(rows))
	for _, r := range rows {
		out[r.CharacterID] = r
	}
	return out, nil
}

// setClaimState fills the master's view of a character's claim: reserved and still
// waiting for a player, or claimed through a link (claimedAt). link is the character's
// latest link, the zero value for none. A character that was never reserved is left
// as it is, with no claim state.
func setClaimState(summary *charactersv1.CharacterSummary, reserved bool, claimedAt *time.Time, link charactersdb.ListLatestClaimLinksRow, now time.Time) {
	switch {
	case !reserved && claimedAt != nil:
		summary.ClaimState = charactersv1.ClaimState_CLAIM_STATE_USED
		summary.ClaimedByDisplayName = summary.PlayerDisplayName
	case !reserved:
		// An ordinary character: no claim.
	case link.CharacterID == "" || link.UsedAt != nil:
		// No link, or one that was used before the character went back to the reserve.
		summary.ClaimState = charactersv1.ClaimState_CLAIM_STATE_NONE
	case link.RevokedAt != nil:
		summary.ClaimState = charactersv1.ClaimState_CLAIM_STATE_REVOKED
	case now.Before(link.ExpiresAt):
		summary.ClaimState = charactersv1.ClaimState_CLAIM_STATE_SENT
		summary.ClaimExpiresAt = timestamppb.New(link.ExpiresAt)
	default:
		summary.ClaimState = charactersv1.ClaimState_CLAIM_STATE_EXPIRED
		summary.ClaimExpiresAt = timestamppb.New(link.ExpiresAt)
	}
}
