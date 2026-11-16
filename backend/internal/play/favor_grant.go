package play

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/platform/uid"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// The Inspiration the master grants a player character (PHB 2014 ch. 4, outside SRD 5.1), called favor in the code because
// "inspiration" already means the Bardic Inspiration die. A character has it or not (the row of character_favor is the
// character's), so a grant to one that has it is refused.

// favorBlocked is FavorService's failed_precondition, with the FavorBlocked detail the app reads.
func favorBlocked(reason playv1.FavorBlockedReason, msg string) error {
	err := connect.NewError(connect.CodeFailedPrecondition, errors.New(msg))
	if detail, detailErr := connect.NewErrorDetail(&playv1.FavorBlocked{Reason: reason}); detailErr == nil {
		err.AddDetail(detail)
	}
	return err
}

// errNotAPlayerCharacter is the one answer for every id that is not a player character of this campaign: an NPC, a reserved
// character, one of another campaign, one that does not exist. It says nothing that tells them apart.
func errNotAPlayerCharacter() error {
	return favorBlocked(playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_NOT_A_PLAYER_CHARACTER, "the character is not a player character of this campaign")
}

// favorGrant is what GrantFavor reads from its request before it opens the transaction.
type favorGrant struct {
	m           authz.Membership
	sessionID   string // the open session the call resolved before the transaction, whose row Service.tx locks first
	characterID string
	key         string
	hash        *string
}

// GrantFavor implements playv1connect.FavorServiceHandler.
func (s *Service) GrantFavor(ctx context.Context, req *connect.Request[playv1.GrantFavorRequest]) (*connect.Response[playv1.GrantFavorResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	key, err := parseKey(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	characterID, ok := uid.Canonical(req.Msg.GetCharacterId())
	if !ok {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("character_id must be a UUID"))
	}
	open, err := s.queries.GetOpenGameSession(ctx, m.CampaignID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errNoOpenSession()
	}
	if err != nil {
		return nil, s.dbError(ctx, "find the open session", err)
	}
	g := favorGrant{m: m, sessionID: open.ID, characterID: characterID, key: key, hash: idem.Hash(req.Msg)}
	err = s.tx(ctx, open.ID, func(tx pgx.Tx, h *hints) error { return s.grantFavorIn(ctx, tx, h, g) })
	if err != nil {
		return nil, s.dbError(ctx, "grant the inspiration", err)
	}
	return connect.NewResponse(&playv1.GrantFavorResponse{}), nil
}

// grantFavorIn is the transaction of GrantFavor: the retry of a key already used, the check of who gets it, the row and the
// event. Service.tx took the session's row lock before this runs.
func (s *Service) grantFavorIn(ctx context.Context, tx pgx.Tx, h *hints, g favorGrant) error {
	q := s.queries.WithTx(tx)
	session, err := stillOpen(ctx, q, g.m.CampaignID, g.sessionID)
	if err != nil {
		return err
	}
	done, again, err := eventByKey(ctx, q, session.ID, g.key, eventFavorGranted, g.hash)
	if err != nil {
		return err
	}
	if again {
		return s.tellFavorAgain(ctx, tx, h, g.m.CampaignID, done.Payload)
	}
	receiver, err := s.favorReceiver(ctx, tx, g.m.CampaignID, g.characterID)
	if err != nil {
		return err
	}
	_, err = q.InsertCharacterFavor(ctx, playdb.InsertCharacterFavorParams{CharacterID: g.characterID, CampaignID: g.m.CampaignID, GrantedByUserID: &g.m.UserID})
	if errors.Is(err, pgx.ErrNoRows) {
		return favorBlocked(playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_ALREADY_HAS, "the character already has the inspiration")
	}
	if err != nil {
		return fmt.Errorf("insert the character's inspiration: %w", err)
	}
	event := keyedEvent{sessionID: session.ID, kind: eventFavorGranted, userID: g.m.UserID, key: g.key, hash: g.hash, characterID: &g.characterID}
	payload := any(favorGrantedEvent{CharacterID: g.characterID})
	st, inCombat, err := s.favorStage(ctx, q, session.ID, g.characterID)
	if err != nil {
		return err
	}
	if inCombat { // a grant in a combat is the combat's last action, which the master's undo takes back: its line is the master's alone
		st.SeenBy = nil
		event.encounterID = &st.EncounterID
		payload = favorGrantedCombatEvent{favorGrantedEvent{CharacterID: g.characterID}, st}
		tellFavorLog(h, g.m.CampaignID, st)
	}
	if err := s.appendKeyedEvent(ctx, q, event, payload); err != nil {
		return err
	}
	return s.tellFavor(ctx, tx, h, g.m.CampaignID, receiver)
}

// favorReceiver finds the character the master names, which must be a player character of the campaign that is not in the
// reserve. Every other id gets the same refusal.
func (s *Service) favorReceiver(ctx context.Context, tx pgx.Tx, campaignID, characterID string) (link.Character, error) {
	found, err := s.roster.SessionCharacters(ctx, tx, campaignID, []string{characterID})
	if err != nil {
		return link.Character{}, err
	}
	for _, c := range found {
		if c.ID == characterID && c.Player && !c.Reserved {
			return c, nil
		}
	}
	return link.Character{}, errNotAPlayerCharacter()
}

// tellFavor collects what a grant tells: the vitals hint of the character, to the master and the character's player (the
// vitals are theirs to read), so their screens read the character again. The write moves the session's number whatever
// the hint finds: a character whose vitals cannot be read (a dead one) tells nothing, and the number moves all the same.
func (s *Service) tellFavor(ctx context.Context, tx pgx.Tx, h *hints, campaignID string, c link.Character) error {
	h.touch()
	v, err := s.vitals.GetVitalsTx(ctx, tx, campaignID, c.ID)
	switch {
	case connect.CodeOf(err) == connect.CodeNotFound:
		return nil
	case err != nil:
		return err
	}
	h.tell(campaignID, vitalsHint(v))
	return nil
}

// tellFavorAgain is the retry of a grant already made: it tells again what the first call would have, from the stored payload,
// and moves no number.
func (s *Service) tellFavorAgain(ctx context.Context, tx pgx.Tx, h *hints, campaignID string, payload []byte) error {
	h.replay()
	var made favorGrantedEvent
	if err := json.Unmarshal(payload, &made); err != nil {
		return fmt.Errorf("read the favor_granted event: %w", err)
	}
	found, err := s.roster.SessionCharacters(ctx, tx, campaignID, []string{made.CharacterID})
	if err != nil || len(found) != 1 {
		return err // the character is gone: there is nobody to tell
	}
	return s.tellFavor(ctx, tx, h, campaignID, found[0])
}
