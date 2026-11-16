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

// The pass of the Inspiration (RN-43): a player asks to pass the Inspiration of their character to another player character, and
// nothing moves until the master accepts. The ask is a row of favor_passes ("pending"); the master's answer closes it, and an accept
// moves the row of character_favor in the same transaction. Each call is a transaction of Service.tx on the session the pass was asked
// in, so the asks and the answers of a table take turns.
//
// The refusals of PassFavor come in this order, so that none tells the caller what they may not read:
//
//  1. not_found: the caller is not a member; invalid_argument: a key or an id that is not a UUID.
//  2. SELF: the receiver is the giver (nothing is read for it).
//  3. NO_SESSION: the campaign has no open session (the master answers at the table). It is the campaign's own state, which every
//     member reads, and it comes before the reads of characters so that the transaction has a session to lock.
//  4. A retry of the key answers the stored pass, whatever the state is now; the key of another ask is invalid_argument.
//  5. NOT_THE_HOLDER: the giver is not a player character of the caller's, or has none. The caller's own state only.
//  6. NOT_A_PLAYER_CHARACTER: the receiver is not a player character of the campaign (the one answer for an NPC, a reserved
//     character, another game's character and an id nobody has). Whether the receiver HAS one is never told: the master reads it
//     (receiver_has) and finds it at the accept (CANNOT_RECEIVE, to the master alone).
//  7. PASS_PENDING: the giver already asked and the master has not answered.

// errFavorPassClosed is the answer to a pass that was already answered or canceled.
func errFavorPassClosed() error {
	return favorBlocked(playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_PASS_CLOSED, "the ask was already answered or canceled")
}

// errFavorNoSession is the answer to an ask made when the campaign has no open session.
func errFavorNoSession() error {
	return favorBlocked(playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_NO_SESSION, "the campaign has no open game session: the master answers the ask at the table")
}

// errFavorPassNotFound is the answer to a pass that is not the caller's to read: unknown, of another campaign, or another player's.
func errFavorPassNotFound() error {
	return connect.NewError(connect.CodeNotFound, errors.New("the ask was not found"))
}

// errNotTheHolder is the refusal of an ask whose giver is not the caller's character, or has no Inspiration.
func errNotTheHolder() error {
	return favorBlocked(playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_NOT_THE_HOLDER, "only the holder of the inspiration may pass it")
}

// favorPassCall is what a pass call reads from its request before it opens the transaction.
type favorPassCall struct {
	m         authz.Membership
	sessionID string // the session whose row Service.tx locks first
	passID    string // the pass an answer or a cancel is about
	from, to  string // the characters of an ask
	accept    bool
	// owner says only the owner of the pass's giver, now, may close it (a cancel): read under the session's lock. The master's answer needs no such check.
	owner bool
	key   string
	hash  *string
}

// readPassID reads a pass id.
func readPassID(raw string) (string, error) {
	id, ok := uid.Canonical(raw)
	if !ok {
		return "", connect.NewError(connect.CodeInvalidArgument, errors.New("pass_id must be a UUID"))
	}
	return id, nil
}

// PassFavor implements playv1connect.FavorServiceHandler: the holder's player asks to pass the Inspiration of their character to another
// player character. It stores a pending pass and nothing moves.
func (s *Service) PassFavor(ctx context.Context, req *connect.Request[playv1.PassFavorRequest]) (*connect.Response[playv1.PassFavorResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	call, err := readAsk(m, req.Msg)
	if err != nil {
		return nil, err
	}
	openID, err := s.openSessionID(ctx, m.CampaignID)
	if err != nil {
		return nil, err
	}
	old, err := s.askOfAnotherSession(ctx, call, openID)
	if err != nil {
		return nil, err
	}
	if old != nil { // the retry of an ask made in a session that ended: it answers the stored pass
		view, err := s.passView(ctx, m, *old)
		if err != nil {
			return nil, s.dbError(ctx, "name the ask", err)
		}
		return connect.NewResponse(&playv1.PassFavorResponse{Pass: view}), nil
	}
	if openID == "" {
		return nil, errFavorNoSession()
	}
	call.sessionID = openID
	var pass playdb.FavorPass
	err = s.tx(ctx, openID, func(tx pgx.Tx, h *hints) (err error) {
		pass, err = s.askPassIn(ctx, tx, h, call)
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, "ask to pass the inspiration", err)
	}
	view, err := s.passView(ctx, m, pass)
	if err != nil {
		return nil, s.dbError(ctx, "name the ask", err)
	}
	return connect.NewResponse(&playv1.PassFavorResponse{Pass: view}), nil
}

// openSessionID is the campaign's open session, "" when it has none.
func (s *Service) openSessionID(ctx context.Context, campaignID string) (string, error) {
	open, err := s.queries.GetOpenGameSession(ctx, campaignID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", s.dbError(ctx, "find the open session", err)
	}
	return open.ID, nil
}

// askOfAnotherSession finds the ask a key made in a session other than the open one (none open: openID is ""), so that the retry of an
// ask whose session ended answers the stored pass and not NO_SESSION. It is sound because the key of an ask is unique in the campaign
// (favor_passes_ask_key_unique), not only in its session: the pass the key names is the one the first call made. The retry must be the
// asker's own and the same request (the giver and the receiver), or the key is refused as one used for another change. An ask of the open
// session is left to the transaction, which also tells the hints again.
func (s *Service) askOfAnotherSession(ctx context.Context, c favorPassCall, openID string) (*playdb.FavorPass, error) {
	pass, err := s.queries.GetFavorPassByAskKey(ctx, playdb.GetFavorPassByAskKeyParams{CampaignID: c.m.CampaignID, AskKey: c.key})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && pass.GameSessionID == openID) {
		return nil, nil
	}
	if err != nil {
		return nil, s.dbError(ctx, "find the ask of the key", err)
	}
	if pass.AskedByUserID != c.m.UserID || pass.FromCharacterID != c.from || pass.ToCharacterID != c.to {
		return nil, errKeyReused()
	}
	return &pass, nil
}

// passKeyRetry is eventByKey for a pass call: a retry is the same actor's. The key of another user's call, with the same body, is refused
// as a key used for another change (errKeyReused), the answer eventByKey gives a key of another request; it never answers the pass.
func passKeyRetry(ctx context.Context, q *playdb.Queries, c favorPassCall, kind string) (payload []byte, again bool, err error) {
	done, again, err := eventByKey(ctx, q, c.sessionID, c.key, kind, c.hash)
	if err != nil || !again {
		return nil, false, err
	}
	if done.ActorUserID == nil || *done.ActorUserID != c.m.UserID {
		return nil, false, errKeyReused()
	}
	return done.Payload, true, nil
}

// readAsk checks what PassFavor reads before any state: the key, the two ids, and the one refusal that needs no read (SELF).
func readAsk(m authz.Membership, msg *playv1.PassFavorRequest) (favorPassCall, error) {
	key, err := parseKey(msg.GetIdempotencyKey())
	if err != nil {
		return favorPassCall{}, err
	}
	from, okFrom := uid.Canonical(msg.GetFromCharacterId())
	to, okTo := uid.Canonical(msg.GetToCharacterId())
	if !okFrom || !okTo {
		return favorPassCall{}, connect.NewError(connect.CodeInvalidArgument, errors.New("from_character_id and to_character_id must be UUIDs"))
	}
	if from == to {
		return favorPassCall{}, favorBlocked(playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_SELF, "the inspiration cannot be passed to the same character")
	}
	return favorPassCall{m: m, from: from, to: to, key: key, hash: idem.Hash(msg)}, nil
}

// askPassIn is the transaction of PassFavor: the retry of a key already used, the checks of the two characters, the row and the
// event. Service.tx took the session's row lock before this runs.
func (s *Service) askPassIn(ctx context.Context, tx pgx.Tx, h *hints, c favorPassCall) (playdb.FavorPass, error) {
	q := s.queries.WithTx(tx)
	session, err := stillOpenOr(ctx, q, c.m.CampaignID, c.sessionID, errFavorNoSession)
	if err != nil {
		return playdb.FavorPass{}, err
	}
	payload, again, err := passKeyRetry(ctx, q, c, eventFavorPassAsked)
	if err != nil {
		return playdb.FavorPass{}, err
	}
	if again {
		return s.tellPassAgain(ctx, tx, h, c.m.CampaignID, payload)
	}
	giver, err := s.favorGiver(ctx, tx, c)
	if err != nil {
		return playdb.FavorPass{}, err
	}
	if _, err := s.favorReceiver(ctx, tx, c.m.CampaignID, c.to); err != nil {
		return playdb.FavorPass{}, err
	}
	pass, err := q.InsertFavorPass(ctx, playdb.InsertFavorPassParams{
		CampaignID: c.m.CampaignID, GameSessionID: session.ID, FromCharacterID: c.from, ToCharacterID: c.to, AskedByUserID: c.m.UserID, AskKey: c.key,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return playdb.FavorPass{}, askRefusal(ctx, q, c.from)
	}
	if err != nil {
		return playdb.FavorPass{}, fmt.Errorf("insert the ask: %w", err)
	}
	if err := s.writePassEvent(ctx, q, passEvent{kind: eventFavorPassAsked, pass: pass, session: session.ID, userID: c.m.UserID, key: c.key, hash: c.hash}); err != nil {
		return playdb.FavorPass{}, err
	}
	return pass, s.tellFavor(ctx, tx, h, c.m.CampaignID, giver)
}

// askRefusal says why the row of an ask was not made: the giver has a pending ask (PASS_PENDING), or the key was used for an ask of
// another session (the key of a campaign's ask is unique) and is refused as any key used for another change.
func askRefusal(ctx context.Context, q *playdb.Queries, giver string) error {
	_, err := q.GetPendingFavorPassOfGiver(ctx, giver)
	if errors.Is(err, pgx.ErrNoRows) {
		return errKeyReused()
	}
	if err != nil {
		return fmt.Errorf("find the giver's pending ask: %w", err)
	}
	return favorBlocked(playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_PASS_PENDING, "the inspiration is already in an ask the master has not answered")
}

// favorGiver checks that the giver is a player character of the caller's (not in the reserve) that has the Inspiration.
func (s *Service) favorGiver(ctx context.Context, tx pgx.Tx, c favorPassCall) (link.Character, error) {
	found, err := s.roster.SessionCharacters(ctx, tx, c.m.CampaignID, []string{c.from})
	if err != nil {
		return link.Character{}, err
	}
	for _, ch := range found {
		if ch.ID == c.from && ch.Player && !ch.Reserved && ch.PlayerUserID == c.m.UserID {
			has, err := s.queries.WithTx(tx).HasCharacterFavor(ctx, playdb.HasCharacterFavorParams{CharacterID: c.from, CampaignID: c.m.CampaignID})
			if err != nil {
				return link.Character{}, fmt.Errorf("read the giver's inspiration: %w", err)
			}
			if has {
				return ch, nil
			}
		}
	}
	return link.Character{}, errNotTheHolder()
}

// passEvent says which session event a pass call writes.
type passEvent struct {
	kind    string
	pass    playdb.FavorPass
	session string
	userID  string
	key     string
	hash    *string
	// log says the event is a line of the running combat's log (the pass answered): it carries the combat. The ask and the cancel by the
	// end of a session never do.
	log   bool
	stage *favorInCombat
}

// writePassEvent writes the event of a pass call under its key: the ids only, never a name or a text.
func (s *Service) writePassEvent(ctx context.Context, q *playdb.Queries, e passEvent) error {
	from := e.pass.FromCharacterID
	ev := keyedEvent{sessionID: e.session, kind: e.kind, userID: e.userID, key: e.key, hash: e.hash, characterID: &from}
	ids := favorPassEvent{PassID: e.pass.ID, FromCharacterID: e.pass.FromCharacterID, ToCharacterID: e.pass.ToCharacterID}
	var payload any = ids
	switch {
	case e.log && e.stage != nil && e.kind == eventFavorPassed:
		ev.encounterID = &e.stage.EncounterID
		payload = favorPassedCombatEvent{favorPassedEvent(ids), *e.stage}
	case e.log && e.stage != nil:
		ev.encounterID = &e.stage.EncounterID
		payload = favorPassCombatEvent{ids, *e.stage}
	case e.kind == eventFavorPassed:
		payload = favorPassedEvent(ids)
	}
	return s.appendKeyedEvent(ctx, q, ev, payload)
}

// writeCanceledPassEvent writes the favor_pass_canceled of a pass that was closed by something other than the giver's call (the session's
// end, the master's undo of the Inspiration the ask would have passed): no key, no combat, so it is no line of any log and no action to undo.
func (s *Service) writeCanceledPassEvent(ctx context.Context, q *playdb.Queries, sessionID string, actor *string, pass playdb.FavorPass) error {
	body, err := json.Marshal(favorPassEvent{PassID: pass.ID, FromCharacterID: pass.FromCharacterID, ToCharacterID: pass.ToCharacterID})
	if err != nil {
		return fmt.Errorf("encode the event payload: %w", err)
	}
	seq, err := q.NextSessionEventSeq(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("next event number: %w", err)
	}
	from := pass.FromCharacterID
	if _, err := insertSessionEvent(ctx, q, playdb.InsertSessionEventParams{
		GameSessionID: sessionID, Seq: seq, Kind: eventFavorPassCanceled, ActorUserID: actor, CharacterID: &from, Payload: body, CreatedAt: s.now(),
	}); err != nil {
		return fmt.Errorf("insert session event: %w", err)
	}
	return nil
}

// stageOfPass finds the combat a pass answer is a line of: both characters must be combatants of the running combat. Only an accepted pass
// (moved) is told to the receiver's owner too: a refusal or a cancel leaves the Inspiration with the giver, and what the receiver was never
// to have is not theirs to hear of.
func (s *Service) stageOfPass(ctx context.Context, q *playdb.Queries, sessionID string, pass playdb.FavorPass, moved bool) (*favorInCombat, error) {
	st, ok, err := s.favorStage(ctx, q, sessionID, pass.FromCharacterID, pass.ToCharacterID)
	if err != nil || !ok {
		return nil, err
	}
	if !moved {
		st.SeenBy = nil
		if st.Owner != "" {
			st.SeenBy = []string{st.Owner}
		}
	}
	return &st, nil
}

// tellPassAgain is the retry of a pass call already made: it tells again what the first call would have, from the stored payload, and
// moves no number. It answers the pass as it is now.
func (s *Service) tellPassAgain(ctx context.Context, tx pgx.Tx, h *hints, campaignID string, payload []byte) (playdb.FavorPass, error) {
	h.replay()
	var made favorPassEvent // the three events carry the same ids
	if err := json.Unmarshal(payload, &made); err != nil {
		return playdb.FavorPass{}, fmt.Errorf("read the pass event: %w", err)
	}
	pass, err := s.queries.WithTx(tx).GetFavorPass(ctx, playdb.GetFavorPassParams{ID: made.PassID, CampaignID: campaignID})
	if err != nil {
		return playdb.FavorPass{}, fmt.Errorf("find the pass: %w", err)
	}
	ids := []string{pass.FromCharacterID}
	if pass.Status == passAccepted {
		ids = append(ids, pass.ToCharacterID)
	}
	found, err := s.roster.SessionCharacters(ctx, tx, campaignID, ids)
	if err != nil {
		return playdb.FavorPass{}, err
	}
	for _, c := range found {
		if err := s.tellFavor(ctx, tx, h, campaignID, c); err != nil {
			return playdb.FavorPass{}, err
		}
	}
	return pass, nil
}

// The statuses of favor_passes.
const (
	passPending  = "pending"
	passAccepted = "accepted"
	passRefused  = "refused"
	passCanceled = "canceled"
)

// AnswerFavorPass implements playv1connect.FavorServiceHandler: the master accepts or refuses a pending pass.
func (s *Service) AnswerFavorPass(ctx context.Context, req *connect.Request[playv1.AnswerFavorPassRequest]) (*connect.Response[playv1.AnswerFavorPassResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	key, err := parseKey(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	id, err := readPassID(req.Msg.GetPassId())
	if err != nil {
		return nil, err
	}
	call := favorPassCall{m: m, passID: id, accept: req.Msg.GetAccept(), key: key, hash: idem.Hash(req.Msg)}
	pass, err := s.closePass(ctx, call, s.answerPassIn)
	if err != nil {
		return nil, err
	}
	view, err := s.passView(ctx, m, pass)
	if err != nil {
		return nil, s.dbError(ctx, "name the ask", err)
	}
	return connect.NewResponse(&playv1.AnswerFavorPassResponse{Pass: view}), nil
}

// CancelFavorPass implements playv1connect.FavorServiceHandler: the giver's owner gives a pending ask up.
func (s *Service) CancelFavorPass(ctx context.Context, req *connect.Request[playv1.CancelFavorPassRequest]) (*connect.Response[playv1.CancelFavorPassResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	key, err := parseKey(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	id, err := readPassID(req.Msg.GetPassId())
	if err != nil {
		return nil, err
	}
	call := favorPassCall{m: m, passID: id, owner: true, key: key, hash: idem.Hash(req.Msg)}
	pass, err := s.closePass(ctx, call, s.cancelPassIn)
	if err != nil {
		return nil, err
	}
	view, err := s.passView(ctx, m, pass)
	if err != nil {
		return nil, s.dbError(ctx, "name the ask", err)
	}
	return connect.NewResponse(&playv1.CancelFavorPassResponse{Pass: view}), nil
}

// ownsGiver says the caller is, now, the owner of the giver of the ask: the ask follows the character, so a character that changed hands
// while it waited is canceled by its new owner (the one who reads it, favor_read.go) and not by the one who asked. A giver sent to the
// reserve has no owner. It is read in the transaction of the cancel, under the session's lock, so a character that changes hands while the
// cancel waits for the lock is the old owner's no more.
func (s *Service) ownsGiver(ctx context.Context, tx pgx.Tx, m authz.Membership, pass playdb.FavorPass) (bool, error) {
	found, err := s.roster.SessionCharacters(ctx, tx, m.CampaignID, []string{pass.FromCharacterID})
	if err != nil {
		return false, err
	}
	for _, ch := range found {
		if ch.ID == pass.FromCharacterID && ch.Player && !ch.Reserved && ch.PlayerUserID == m.UserID {
			return true, nil
		}
	}
	return false, nil
}

// closePass finds the pass the call is about and runs the transaction of the answer on the session it was asked in. A pass that is not
// the caller's to close (a cancel by anyone but the giver's owner, the master included): it is never told apart from one that does not exist.
func (s *Service) closePass(ctx context.Context, c favorPassCall, run func(context.Context, pgx.Tx, *hints, favorPassCall) (playdb.FavorPass, error)) (playdb.FavorPass, error) {
	pass, err := s.queries.GetFavorPass(ctx, playdb.GetFavorPassParams{ID: c.passID, CampaignID: c.m.CampaignID})
	if errors.Is(err, pgx.ErrNoRows) {
		return playdb.FavorPass{}, errFavorPassNotFound()
	}
	if err != nil {
		return playdb.FavorPass{}, s.dbError(ctx, "find the ask", err)
	}
	c.sessionID = pass.GameSessionID
	var out playdb.FavorPass
	err = s.tx(ctx, c.sessionID, func(tx pgx.Tx, h *hints) error {
		if c.owner { // under the lock: the character may have changed hands while the call waited for it
			owns, err := s.ownsGiver(ctx, tx, c.m, pass)
			if err != nil {
				return fmt.Errorf("find the ask's giver: %w", err)
			}
			if !owns {
				return errFavorPassNotFound()
			}
		}
		var err error
		out, err = run(ctx, tx, h, c)
		return err
	})
	if err != nil {
		return playdb.FavorPass{}, s.dbError(ctx, "answer the ask", err)
	}
	return out, nil
}

// answerPassIn is the transaction of AnswerFavorPass: the retry of a key already used, then the pass is read again under the lock
// (another answer may have closed it), then the accept or the refusal.
func (s *Service) answerPassIn(ctx context.Context, tx pgx.Tx, h *hints, c favorPassCall) (playdb.FavorPass, error) {
	q := s.queries.WithTx(tx)
	kind := eventFavorAskRefused
	if c.accept {
		kind = eventFavorPassed
	}
	payload, again, err := passKeyRetry(ctx, q, c, kind)
	if err != nil {
		return playdb.FavorPass{}, err
	}
	if again {
		return s.tellPassAgain(ctx, tx, h, c.m.CampaignID, payload)
	}
	pass, err := q.GetFavorPass(ctx, playdb.GetFavorPassParams{ID: c.passID, CampaignID: c.m.CampaignID})
	if err != nil {
		return playdb.FavorPass{}, fmt.Errorf("read the ask again: %w", err)
	}
	if pass.Status != passPending {
		return playdb.FavorPass{}, errFavorPassClosed()
	}
	if c.accept {
		return s.acceptPass(ctx, tx, h, c, pass)
	}
	return s.closeWith(ctx, tx, h, c, pass, passRefused, eventFavorAskRefused)
}

// cancelPassIn is the transaction of CancelFavorPass.
func (s *Service) cancelPassIn(ctx context.Context, tx pgx.Tx, h *hints, c favorPassCall) (playdb.FavorPass, error) {
	q := s.queries.WithTx(tx)
	payload, again, err := passKeyRetry(ctx, q, c, eventFavorPassCanceled)
	if err != nil {
		return playdb.FavorPass{}, err
	}
	if again {
		return s.tellPassAgain(ctx, tx, h, c.m.CampaignID, payload)
	}
	pass, err := q.GetFavorPass(ctx, playdb.GetFavorPassParams{ID: c.passID, CampaignID: c.m.CampaignID})
	if err != nil {
		return playdb.FavorPass{}, fmt.Errorf("read the ask again: %w", err)
	}
	if pass.Status != passPending {
		return playdb.FavorPass{}, errFavorPassClosed()
	}
	return s.closeWith(ctx, tx, h, c, pass, passCanceled, eventFavorPassCanceled)
}

// closeKeyError is the error of closing an ask: a closing key that another session of the campaign already used (the retry check reads the
// open session's events only) hits the unique index on closed_key, and is the key refused as reused.
func closeKeyError(err error) error {
	if isUniqueViolation(err) {
		return errKeyReused()
	}
	return fmt.Errorf("close the ask: %w", err)
}

// closeWith closes a pending pass with the status, writes its event under the key and tells the giver's screens (the master's and the
// giver's owner's). Nothing moves.
func (s *Service) closeWith(ctx context.Context, tx pgx.Tx, h *hints, c favorPassCall, pass playdb.FavorPass, status, kind string) (playdb.FavorPass, error) {
	q := s.queries.WithTx(tx)
	closed, err := q.CloseFavorPass(ctx, playdb.CloseFavorPassParams{Status: status, ClosedKey: &c.key, ID: pass.ID, CampaignID: pass.CampaignID})
	if errors.Is(err, pgx.ErrNoRows) {
		return playdb.FavorPass{}, errFavorPassClosed()
	}
	if err != nil {
		return playdb.FavorPass{}, closeKeyError(err)
	}
	return closed, s.writeAndTellPass(ctx, tx, h, c, closed, kind, false)
}

// writeAndTellPass writes the event of a pass answer (a line of the combat's log when the pass was answered in a running combat) and
// collects what it tells.
func (s *Service) writeAndTellPass(ctx context.Context, tx pgx.Tx, h *hints, c favorPassCall, pass playdb.FavorPass, kind string, moved bool) error {
	q := s.queries.WithTx(tx)
	stage, err := s.stageOfPass(ctx, q, c.sessionID, pass, moved)
	if err != nil {
		return err
	}
	if err := s.writePassEvent(ctx, q, passEvent{kind: kind, pass: pass, session: c.sessionID, userID: c.m.UserID, key: c.key, hash: c.hash, log: true, stage: stage}); err != nil {
		return err
	}
	if stage != nil {
		tellFavorLog(h, pass.CampaignID, *stage)
	}
	return s.tellPassOf(ctx, tx, h, pass, moved)
}

// tellPassOf collects the hints of a pass call: the giver's vitals, and the receiver's when the Inspiration moved. Each reaches the master
// and the character's own player, and nobody else.
func (s *Service) tellPassOf(ctx context.Context, tx pgx.Tx, h *hints, pass playdb.FavorPass, moved bool) error {
	ids := []string{pass.FromCharacterID}
	if moved {
		ids = append(ids, pass.ToCharacterID)
	}
	found, err := s.roster.SessionCharacters(ctx, tx, pass.CampaignID, ids)
	if err != nil {
		return err
	}
	h.touch()
	for _, c := range found {
		if err := s.tellFavor(ctx, tx, h, pass.CampaignID, c); err != nil {
			return err
		}
	}
	return nil
}

// acceptPass moves the Inspiration in the transaction: the pass is closed, the giver's row is deleted and the receiver's is inserted. A
// receiver that cannot take it (it has one already, or it left the table) rolls everything back and the master alone is told
// CANNOT_RECEIVE; the pass stays pending.
func (s *Service) acceptPass(ctx context.Context, tx pgx.Tx, h *hints, c favorPassCall, pass playdb.FavorPass) (playdb.FavorPass, error) {
	q := s.queries.WithTx(tx)
	closed, err := q.CloseFavorPass(ctx, playdb.CloseFavorPassParams{Status: passAccepted, ClosedKey: &c.key, ID: pass.ID, CampaignID: pass.CampaignID})
	if errors.Is(err, pgx.ErrNoRows) {
		return playdb.FavorPass{}, errFavorPassClosed()
	}
	if err != nil {
		return playdb.FavorPass{}, closeKeyError(err)
	}
	if err := s.mayHold(ctx, tx, closed.CampaignID, closed.ToCharacterID, errCannotReceive()); err != nil { // it may have left the table
		return playdb.FavorPass{}, err
	}
	if err := s.mayHold(ctx, tx, closed.CampaignID, closed.FromCharacterID, errNotTheHolder()); err != nil { // the giver may be back in the reserve
		return playdb.FavorPass{}, err
	}
	if _, err := q.SpendCharacterFavor(ctx, playdb.SpendCharacterFavorParams{CharacterID: closed.FromCharacterID, CampaignID: closed.CampaignID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return playdb.FavorPass{}, errNotTheHolder() // the giver has none by now (nothing else takes it while the pass is pending)
		}
		return playdb.FavorPass{}, fmt.Errorf("take the giver's inspiration: %w", err)
	}
	if _, err := q.InsertCharacterFavor(ctx, playdb.InsertCharacterFavorParams{CharacterID: closed.ToCharacterID, CampaignID: closed.CampaignID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return playdb.FavorPass{}, errCannotReceive()
		}
		return playdb.FavorPass{}, fmt.Errorf("give the receiver the inspiration: %w", err)
	}
	return closed, s.writeAndTellPass(ctx, tx, h, c, closed, eventFavorPassed, true)
}

// mayHold says the character may hold the Inspiration now: a player character of the campaign that is out of the reserve. Every move of
// the row to a character (the accept, the undo of an accept, the master's refund) asks it first. A character that may not is answered
// with notAPlayer; a failed read is returned as it is, since it says nothing of the character.
func (s *Service) mayHold(ctx context.Context, tx pgx.Tx, campaignID, characterID string, notAPlayer error) error {
	if _, err := s.favorReceiver(ctx, tx, campaignID, characterID); err != nil {
		if favorReasonOf(err) == playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_NOT_A_PLAYER_CHARACTER {
			return notAPlayer
		}
		return err
	}
	return nil
}

// favorReasonOf is the FavorBlocked reason of an error, unspecified for any other error.
func favorReasonOf(err error) playv1.FavorBlockedReason {
	if ce, ok := errors.AsType[*connect.Error](err); ok {
		for _, d := range ce.Details() {
			if v, derr := d.Value(); derr == nil {
				if b, isBlocked := v.(*playv1.FavorBlocked); isBlocked {
					return b.GetReason()
				}
			}
		}
	}
	return playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_UNSPECIFIED
}

// errCannotReceive is the master's alone: the receiver already has the Inspiration (or is no player character of the table any more).
func errCannotReceive() error {
	return favorBlocked(playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_CANNOT_RECEIVE, "the receiver cannot take the inspiration")
}

// passView is the pass as the caller reads it: the names come from the characters module (the query cannot join its table), and
// receiver_has is the master's alone.
func (s *Service) passView(ctx context.Context, m authz.Membership, pass playdb.FavorPass) (*playv1.FavorPass, error) {
	found, err := s.roster.SessionCharacters(ctx, nil, m.CampaignID, []string{pass.FromCharacterID, pass.ToCharacterID})
	if err != nil {
		return nil, err
	}
	names := map[string]string{}
	for _, c := range found {
		names[c.ID] = c.Name
	}
	return favorPassProto(pass, names, false), nil
}

// favorPassProto is the pass as the proto reads it.
func favorPassProto(p playdb.FavorPass, names map[string]string, receiverHas bool) *playv1.FavorPass {
	return &playv1.FavorPass{
		Id: p.ID, FromCharacterId: p.FromCharacterID, FromName: names[p.FromCharacterID], ToCharacterId: p.ToCharacterID, ToName: names[p.ToCharacterID],
		Status: passStatusProto(p.Status), ReceiverHas: receiverHas,
	}
}

// passStatusProto maps the stored status.
func passStatusProto(status string) playv1.FavorPassStatus {
	switch status {
	case passPending:
		return playv1.FavorPassStatus_FAVOR_PASS_STATUS_PENDING
	case passAccepted:
		return playv1.FavorPassStatus_FAVOR_PASS_STATUS_ACCEPTED
	case passRefused:
		return playv1.FavorPassStatus_FAVOR_PASS_STATUS_REFUSED
	case passCanceled:
		return playv1.FavorPassStatus_FAVOR_PASS_STATUS_CANCELED
	}
	return playv1.FavorPassStatus_FAVOR_PASS_STATUS_UNSPECIFIED
}

// cancelPassesOfSession cancels the passes still pending in the session that ends, in the transaction of the end: each closed pass
// writes favor_pass_canceled (the master's hand, no key) and tells the master and the giver's owner. The Inspiration stays with the
// giver.
func (s *Service) cancelPassesOfSession(ctx context.Context, tx pgx.Tx, m authz.Membership, sessionID string) (told []*playv1.CharacterVitals, err error) {
	q := s.queries.WithTx(tx)
	closed, err := q.CloseFavorPassesOfSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("cancel the pending asks: %w", err)
	}
	for _, pass := range closed {
		if err := s.writeCanceledPassEvent(ctx, q, sessionID, &m.UserID, playdb.FavorPass(pass)); err != nil {
			return nil, err
		}
		v, err := s.vitals.GetVitalsTx(ctx, tx, m.CampaignID, pass.FromCharacterID)
		switch {
		case connect.CodeOf(err) == connect.CodeNotFound:
		case err != nil:
			return nil, err
		default:
			told = append(told, v)
		}
	}
	return told, nil
}
