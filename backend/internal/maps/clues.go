package maps

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/timestamppb"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/maps/mapsdb"
	notelink "github.com/PuraFome/meuRPG/backend/internal/notes/link"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/platform/names"
	"github.com/PuraFome/meuRPG/backend/internal/platform/uid"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
)

// The clues of an RP scene, the discoveries and what a player's notes read
// (MR-029, MR-030).
//
//   - The master prepares clues on a SCENE point, one at a time, like the
//     actions (scene_clues), and reveals each to the players it picks
//     (RevealSceneClue). A player never receives a clue that was not revealed
//     to them, and the hooks (the point's private text) never leave the
//     master's reads (RN-20).
//   - A reveal is recorded per player (scene_clue_reveals) with a copy of the
//     clue's text: removing or editing the clue afterwards changes nothing for
//     the players who have it, as what was said at the table stays said.
//   - A scene is "discovered" when its point is revealed on the map or opened
//     to the whole table in a session (scene_discoveries), and, for a scene open
//     to only some players, by each of them (scene_player_discoveries). A
//     player's note may be tagged with a scene they discovered only; package
//     notes reads both through SessionMaps.
//
// Opening a scene in the session is package play's; it reads the hooks and
// the clues through SessionMaps.ScenePoint and gives them to the master only.

const (
	// maxSceneClues is how many clues a scene point may have (the
	// master's list is short, and so is the dialog that reveals).
	maxSceneClues = 30
	// maxClueText is the longest clue, in characters (scene_clues_text_length).
	maxClueText = 500
	// maxRevealCharacters is how many characters one reveal may name: a table
	// is a handful of players, and the list must stay small.
	maxRevealCharacters = 50
	// eventClueRevealed is the session event kind of a reveal. The kind is
	// play's (session_event_kinds lists it); this package only names it.
	eventClueRevealed = "clue_revealed"
)

// clueRevealedEvent is the payload of clue_revealed: IDs only, never the
// clue's text nor a character's name (docs/privacy.md).
type clueRevealedEvent struct {
	ClueID       string   `json:"clue_id"`
	PointID      string   `json:"point_id"`
	CharacterIDs []string `json:"character_ids"`
}

// addClueCall is one AddSceneClue inside its transaction: what the call
// carries in, and the clue and whether it was a replay out.
type addClueCall struct {
	pointID     string
	text        string
	scopedKey   *string
	requestHash *string
	added       mapsdb.SceneClue
	replayed    bool
}

// AddSceneClue implements mapsv1connect.MapServiceHandler.
func (s *Service) AddSceneClue(
	ctx context.Context,
	req *connect.Request[mapsv1.AddSceneClueRequest],
) (*connect.Response[mapsv1.AddSceneClueResponse], error) {
	m, mapID, pointID, err := s.sceneCall(ctx, req.Msg.GetCampaignId(), req.Msg.GetMapId(), req.Msg.GetPointId())
	if err != nil {
		return nil, err
	}
	text, err := cleanClueText(req.Msg.GetText())
	if err != nil {
		return nil, err
	}
	// The key is unique in the campaign, and kept with a hash of the whole request: a retry
	// returns the first clue only when it is the same request.
	scopedKey, requestHash, err := keyOf(m.CampaignID, req.Msg.GetIdempotencyKey(), req.Msg)
	if err != nil {
		return nil, err
	}
	call := &addClueCall{pointID: pointID, text: text, scopedKey: scopedKey, requestHash: requestHash}
	ch, err := s.changeActions(ctx, m, mapID, pointID, func(q *mapsdb.Queries) error {
		return s.appendClue(ctx, q, call)
	})
	if err != nil {
		return nil, s.dbError(ctx, "add a scene clue", err)
	}
	if !call.replayed {
		s.cluesChanged(ctx, m.CampaignID, ch)
	}
	clues, err := s.cluesOfPoint(ctx, m.CampaignID, pointID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&mapsv1.AddSceneClueResponse{Clue: clueByID(clues, call.added.ID), Clues: clues}), nil
}

// appendClue reads the point's clues and adds the new one after them, or
// finds the one an earlier call with the same key added.
func (s *Service) appendClue(ctx context.Context, q *mapsdb.Queries, call *addClueCall) error {
	// The point is locked by now, so two calls with the same key take turns.
	current, err := q.ListSceneClues(ctx, call.pointID)
	if err != nil {
		return fmt.Errorf("list the clues: %w", err)
	}
	call.added, call.replayed, err = idem.Create(ctx, call.scopedKey, call.requestHash, q.GetSceneClueByCreateKey,
		func(c mapsdb.SceneClue) *string { return c.CreateHash },
		func() (mapsdb.SceneClue, error) { return s.insertClue(ctx, q, call, current) })
	return err
}

// insertClue adds a clue after the current ones, refusing the one over the
// limit. A conflict on the key is pgx.ErrNoRows, for idem.Create to read.
func (s *Service) insertClue(ctx context.Context, q *mapsdb.Queries, call *addClueCall, current []mapsdb.SceneClue) (mapsdb.SceneClue, error) {
	if len(current) >= maxSceneClues {
		return call.added, connect.NewError(connect.CodeResourceExhausted, fmt.Errorf("the scene already has %d clues", maxSceneClues))
	}
	position := int32(0)
	if n := len(current); n > 0 {
		position = current[n-1].Position + 1
	}
	c, err := q.InsertSceneClue(ctx, mapsdb.InsertSceneClueParams{
		PointID: call.pointID, Position: position, Text: call.text, CreateKey: call.scopedKey, CreateHash: call.requestHash, Now: s.now(),
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return c, fmt.Errorf("insert clue: %w", err)
	}
	return c, err
}

// UpdateSceneClue implements mapsv1connect.MapServiceHandler.
func (s *Service) UpdateSceneClue(
	ctx context.Context,
	req *connect.Request[mapsv1.UpdateSceneClueRequest],
) (*connect.Response[mapsv1.UpdateSceneClueResponse], error) {
	m, mapID, pointID, err := s.sceneCall(ctx, req.Msg.GetCampaignId(), req.Msg.GetMapId(), req.Msg.GetPointId())
	if err != nil {
		return nil, err
	}
	clueID, ok := uid.Canonical(req.Msg.GetClueId())
	if !ok {
		return nil, errClueNotFound()
	}
	text, err := cleanClueText(req.Msg.GetText())
	if err != nil {
		return nil, err
	}
	ch, err := s.changeActions(ctx, m, mapID, pointID, func(q *mapsdb.Queries) error {
		if _, err := q.GetSceneClueForUpdate(ctx, mapsdb.GetSceneClueForUpdateParams{PointID: pointID, ID: clueID}); errors.Is(err, pgx.ErrNoRows) {
			return errClueNotFound()
		} else if err != nil {
			return fmt.Errorf("find clue: %w", err)
		}
		if _, err := q.UpdateSceneClue(ctx, mapsdb.UpdateSceneClueParams{PointID: pointID, ID: clueID, Text: text, Now: s.now()}); err != nil {
			return fmt.Errorf("update clue: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "update a scene clue", err)
	}
	s.cluesChanged(ctx, m.CampaignID, ch)
	clues, err := s.cluesOfPoint(ctx, m.CampaignID, pointID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&mapsv1.UpdateSceneClueResponse{Clue: clueByID(clues, clueID), Clues: clues}), nil
}

// MoveSceneClue implements mapsv1connect.MapServiceHandler.
func (s *Service) MoveSceneClue(
	ctx context.Context,
	req *connect.Request[mapsv1.MoveSceneClueRequest],
) (*connect.Response[mapsv1.MoveSceneClueResponse], error) {
	m, mapID, pointID, err := s.sceneCall(ctx, req.Msg.GetCampaignId(), req.Msg.GetMapId(), req.Msg.GetPointId())
	if err != nil {
		return nil, err
	}
	clueID, ok := uid.Canonical(req.Msg.GetClueId())
	if !ok {
		return nil, errClueNotFound()
	}
	step, err := moveStep(req.Msg.GetDirection())
	if err != nil {
		return nil, err
	}
	moved := false
	ch, err := s.changeActions(ctx, m, mapID, pointID, func(q *mapsdb.Queries) error {
		var err error
		moved, err = shiftClue(ctx, q, pointID, clueID, step)
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, "move a scene clue", err)
	}
	if moved {
		s.cluesChanged(ctx, m.CampaignID, ch)
	}
	clues, err := s.cluesOfPoint(ctx, m.CampaignID, pointID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&mapsv1.MoveSceneClueResponse{Clues: clues}), nil
}

// shiftClue swaps a clue with its neighbor and renumbers the list from 0,
// as shiftAction does. It says whether anything changed: the first clue moved
// up, or the last moved down, changes nothing.
func shiftClue(ctx context.Context, q *mapsdb.Queries, pointID, clueID string, step int) (bool, error) {
	clues, err := q.ListSceneClues(ctx, pointID)
	if err != nil {
		return false, fmt.Errorf("list the clues: %w", err)
	}
	i := slices.IndexFunc(clues, func(c mapsdb.SceneClue) bool { return c.ID == clueID })
	if i < 0 {
		return false, errClueNotFound()
	}
	return swapAndRenumber(clues, i, step,
		func(c *mapsdb.SceneClue) *int32 { return &c.Position },
		func(c mapsdb.SceneClue) error {
			if err := q.SetSceneCluePosition(ctx, mapsdb.SetSceneCluePositionParams{ID: c.ID, Position: c.Position}); err != nil {
				return fmt.Errorf("set a clue's position: %w", err)
			}
			return nil
		})
}

// RemoveSceneClue implements mapsv1connect.MapServiceHandler. The players who
// already received the clue keep it: their reveal rows hold their own copy.
func (s *Service) RemoveSceneClue(
	ctx context.Context,
	req *connect.Request[mapsv1.RemoveSceneClueRequest],
) (*connect.Response[mapsv1.RemoveSceneClueResponse], error) {
	m, mapID, pointID, err := s.sceneCall(ctx, req.Msg.GetCampaignId(), req.Msg.GetMapId(), req.Msg.GetPointId())
	if err != nil {
		return nil, err
	}
	clueID, ok := uid.Canonical(req.Msg.GetClueId())
	if !ok {
		return nil, errClueNotFound()
	}
	ch, err := s.changeActions(ctx, m, mapID, pointID, func(q *mapsdb.Queries) error {
		n, err := q.DeleteSceneClue(ctx, mapsdb.DeleteSceneClueParams{PointID: pointID, ID: clueID})
		if err != nil {
			return fmt.Errorf("delete clue: %w", err)
		}
		if n == 0 {
			return errClueNotFound()
		}
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "remove a scene clue", err)
	}
	s.cluesChanged(ctx, m.CampaignID, ch)
	clues, err := s.cluesOfPoint(ctx, m.CampaignID, pointID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&mapsv1.RemoveSceneClueResponse{Clues: clues}), nil
}

// revealCall is one RevealSceneClue as its transaction sees it: who gets the clue (character
// to player).
type revealCall struct {
	m            authz.Membership
	clueID       string
	characterIDs []string
	userOf       map[string]string
}

// clueReveal is what the transaction of RevealSceneClue ends with: the clue, and the players
// who just got it.
type clueReveal struct {
	clue     mapsdb.SceneClue
	newUsers []string
}

// RevealSceneClue implements mapsv1connect.MapServiceHandler.
func (s *Service) RevealSceneClue(
	ctx context.Context,
	req *connect.Request[mapsv1.RevealSceneClueRequest],
) (*connect.Response[mapsv1.RevealSceneClueResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	clueID, ok := uid.Canonical(req.Msg.GetClueId())
	if !ok {
		return nil, errClueNotFound()
	}
	// The clue goes to each character's player, who owns the notes: an NPC, a dead or pending
	// character, or one with no player is "not found", as a token's.
	characterIDs, userOf, err := s.playerCharacters(ctx, m.CampaignID, req.Msg.GetCharacterIds(), true)
	if err != nil {
		return nil, err
	}
	call := revealCall{m: m, clueID: clueID, characterIDs: characterIDs, userOf: userOf}
	var res clueReveal
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		res, err = s.revealClueTx(ctx, tx, call)
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, "reveal a scene clue", err)
	}
	s.publishReveal(ctx, m.CampaignID, res)
	clues, err := s.cluesOfPoint(ctx, m.CampaignID, res.clue.PointID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&mapsv1.RevealSceneClueResponse{Clue: clueByID(clues, res.clue.ID)}), nil
}

// revealClueTx finds the clue in the campaign, gives it to the characters who
// did not have it yet and records the reveal as a session event.
func (s *Service) revealClueTx(ctx context.Context, tx pgx.Tx, call revealCall) (clueReveal, error) {
	q := s.queries.WithTx(tx)
	clue, err := q.GetSceneClueInCampaign(ctx, mapsdb.GetSceneClueInCampaignParams{CampaignID: call.m.CampaignID, ID: call.clueID})
	if errors.Is(err, pgx.ErrNoRows) {
		return clueReveal{}, errClueNotFound()
	} else if err != nil {
		return clueReveal{}, fmt.Errorf("find clue: %w", err)
	}
	now := s.now()
	gave, newUsers, err := s.insertReveals(ctx, q, call, clue, now) // the characters that did not have it yet
	if err != nil {
		return clueReveal{}, err
	}
	res := clueReveal{clue: clue, newUsers: newUsers}
	if len(gave) == 0 {
		return res, nil // everyone had it: nothing changes, nothing is recorded
	}
	payload, err := json.Marshal(clueRevealedEvent{ClueID: clue.ID, PointID: clue.PointID, CharacterIDs: gave})
	if err != nil {
		return clueReveal{}, fmt.Errorf("encode the event payload: %w", err)
	}
	// With no open session nothing is written here: the reveal is still
	// made, as an XP award is.
	if _, err := s.live.AppendEvent(ctx, tx, call.m.CampaignID, eventClueRevealed, call.m.UserID, payload, now); err != nil {
		return clueReveal{}, fmt.Errorf("record the reveal: %w", err)
	}
	return res, nil
}

// insertReveals writes one reveal per character and returns the characters
// who did not have the clue yet, noting their players in call.newUsers.
func (s *Service) insertReveals(ctx context.Context, q *mapsdb.Queries, call revealCall, clue mapsdb.SceneClue, now time.Time) (gave, newUsers []string, err error) {
	for _, id := range call.characterIDs {
		character := id
		n, err := q.InsertClueReveal(ctx, mapsdb.InsertClueRevealParams{
			CampaignID: call.m.CampaignID, ClueID: &clue.ID, PointID: &clue.PointID, UserID: call.userOf[id],
			CharacterID: &character, Text: clue.Text, Now: now,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("reveal the clue: %w", err)
		}
		if n == 1 {
			gave = append(gave, id)
			if !slices.Contains(newUsers, call.userOf[id]) {
				newUsers = append(newUsers, call.userOf[id])
			}
		}
	}
	return gave, newUsers, nil
}

// publishReveal tells the players who got the clue, and the master's open
// scene. Only the players who got it hear of it; a player who is offline reads
// it on their next ListNotes.
func (s *Service) publishReveal(ctx context.Context, campaignID string, res clueReveal) {
	if len(res.newUsers) == 0 {
		return
	}
	s.live.PublishToUsers(campaignID, res.newUsers, &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_NotesChanged_{
		NotesChanged: &playv1.WatchGameSessionResponse_NotesChanged{},
	}})
	s.publishSceneChangedToMasterIf(ctx, campaignID, res.clue.PointID)
}

// cluesChanged tells the master's watching streams that a point's clues
// changed. A player's stream hears nothing: no player reads a clue from here.
func (s *Service) cluesChanged(ctx context.Context, campaignID string, ch changedPoint) {
	s.publishMapChanged(campaignID, ch.mapRow.ID, false)
	s.publishSceneChangedToMasterIf(ctx, campaignID, ch.point.ID)
}

// publishSceneChangedToMasterIf is publishSceneChangedIf for a change only the
// master reads (the clues): only the master's streams get scene_changed.
func (s *Service) publishSceneChangedToMasterIf(ctx context.Context, campaignID, pointID string) {
	open, err := s.live.OpenScenePoint(ctx, campaignID)
	if err != nil {
		s.logger.ErrorContext(ctx, "maps: read the open scene", "error", err)
		return
	}
	if open != "" && open == pointID {
		s.publish(campaignID, false, &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_SceneChanged_{
			SceneChanged: &playv1.WatchGameSessionResponse_SceneChanged{},
		}})
	}
}

// cleanClueText checks a clue: 1 to 500 characters, line breaks allowed. The
// message says the rule, never the text.
func cleanClueText(raw string) (string, error) {
	text, err := names.CleanText(raw, maxClueText)
	if err == nil && text == "" {
		err = names.ErrEmpty
	}
	if err != nil {
		return "", connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("text %w", err))
	}
	return text, nil
}

func errClueNotFound() error {
	return connect.NewError(connect.CodeNotFound, errors.New("clue not found"))
}

// revealRef is who got a clue, from either of the reveal queries.
type revealRef struct {
	clueID      string
	characterID *string
	at          time.Time
}

// cluesOfPoint reads a point's clues as the master sees them, with who has
// each one.
func (s *Service) cluesOfPoint(ctx context.Context, campaignID, pointID string) ([]*mapsv1.SceneClue, error) {
	rows, err := s.queries.ListSceneClues(ctx, pointID)
	if err != nil {
		return nil, s.dbError(ctx, "list the clues", err)
	}
	reveals, err := s.queries.ListClueRevealsOfPoint(ctx, &pointID)
	if err != nil {
		return nil, s.dbError(ctx, "list who has each clue", err)
	}
	refs := make([]revealRef, 0, len(reveals))
	for _, r := range reveals {
		refs = append(refs, revealRef{clueID: *r.ClueID, characterID: r.CharacterID, at: r.RevealedAt})
	}
	return s.cluesToProto(ctx, campaignID, rows, refs)
}

// attachClues fills the hooks' companions, the clues of the SCENE points of a
// map, for the master. A player gets none: not even the revealed ones (those
// are in their notes), so this reads nothing for a player.
func (s *Service) attachClues(ctx context.Context, campaignID, mapID string, points []*mapsv1.MapPoint, master bool) error {
	if !master || !slices.ContainsFunc(points, func(p *mapsv1.MapPoint) bool { return p.GetKind() == mapsv1.MapPointKind_MAP_POINT_KIND_SCENE }) {
		return nil
	}
	rows, err := s.queries.ListSceneCluesOfMap(ctx, mapID)
	if err != nil {
		return err
	}
	reveals, err := s.queries.ListClueRevealsOfMap(ctx, mapID)
	if err != nil {
		return err
	}
	refs := make([]revealRef, 0, len(reveals))
	for _, r := range reveals {
		refs = append(refs, revealRef{clueID: *r.ClueID, characterID: r.CharacterID, at: r.RevealedAt})
	}
	all, err := s.cluesToProto(ctx, campaignID, rows, refs)
	if err != nil {
		return err
	}
	pointOf := make(map[string]string, len(rows))
	for _, r := range rows {
		pointOf[r.ID] = r.PointID
	}
	byPoint := map[string][]*mapsv1.SceneClue{}
	for _, c := range all {
		byPoint[pointOf[c.GetId()]] = append(byPoint[pointOf[c.GetId()]], c)
	}
	for _, p := range points {
		p.Clues = byPoint[p.GetId()]
	}
	return nil
}

// cluesToProto builds the master's clues, with each recipient's name.
func (s *Service) cluesToProto(ctx context.Context, campaignID string, rows []mapsdb.SceneClue, reveals []revealRef) ([]*mapsv1.SceneClue, error) {
	nameOf, err := s.recipientNames(ctx, campaignID, reveals)
	if err != nil {
		return nil, err
	}
	out := make([]*mapsv1.SceneClue, 0, len(rows))
	for _, row := range rows {
		out = append(out, &mapsv1.SceneClue{Id: row.ID, Text: row.Text, RevealedTo: clueRecipients(row.ID, reveals, nameOf)})
	}
	return out, nil
}

// recipientNames reads the names of the characters a list of reveals names,
// by ID. It reads nothing when no reveal names a character.
func (s *Service) recipientNames(ctx context.Context, campaignID string, reveals []revealRef) (map[string]string, error) {
	var ids []string
	for _, r := range reveals {
		if r.characterID != nil && !slices.Contains(ids, *r.characterID) {
			ids = append(ids, *r.characterID)
		}
	}
	nameOf := map[string]string{}
	if len(ids) == 0 {
		return nameOf, nil
	}
	chars, err := s.characters.MapCharacters(ctx, nil, campaignID, ids)
	if err != nil {
		return nil, s.dbError(ctx, "read the recipients' names", err)
	}
	for _, c := range chars {
		nameOf[c.GetId()] = c.GetName()
	}
	return nameOf, nil
}

// clueRecipients lists who got a clue, in the order of the reveals.
func clueRecipients(clueID string, reveals []revealRef, nameOf map[string]string) []*mapsv1.ClueRecipient {
	var out []*mapsv1.ClueRecipient
	for _, r := range reveals {
		if r.clueID != clueID || r.characterID == nil {
			continue
		}
		out = append(out, &mapsv1.ClueRecipient{
			CharacterId: *r.characterID, CharacterName: nameOf[*r.characterID], RevealedAt: timestamppb.New(r.at),
		})
	}
	return out
}

func clueByID(clues []*mapsv1.SceneClue, id string) *mapsv1.SceneClue {
	for _, c := range clues {
		if c.GetId() == id {
			return c
		}
	}
	return nil
}

// DiscoverScene records, inside tx, that the group discovered a scene: the
// master opened its point in a session (MR-030), even while the
// point is hidden on the map. A scene already discovered stays as it is. It
// implements play.MapKeeper; the caller checked that the point is a SCENE
// point of the campaign (ScenePoint).
func (sm *SessionMaps) DiscoverScene(ctx context.Context, tx pgx.Tx, campaignID, pointID string, at time.Time) error {
	err := sm.queries.WithTx(tx).UpsertSceneDiscovery(ctx, mapsdb.UpsertSceneDiscoveryParams{CampaignID: campaignID, PointID: pointID, DiscoveredAt: at})
	if err != nil {
		return fmt.Errorf("record the scene's discovery: %w", err)
	}
	return nil
}

// DiscoverSceneFor records, inside tx, that these players alone discovered a scene: the master opened it to
// them, brought them in, or they joined it, while it was open to only some players (MR-030, RN-10). The
// group's list stays as it is, so nobody else learns the scene's name from the notes. A scene they
// discovered already stays as it is. It implements play.MapKeeper; the caller checked that the point is a
// SCENE point of the campaign (ScenePoint) and that the users are players of the campaign.
func (sm *SessionMaps) DiscoverSceneFor(ctx context.Context, tx pgx.Tx, campaignID, pointID string, userIDs []string, at time.Time) error {
	q := sm.queries.WithTx(tx)
	for _, user := range userIDs {
		if err := q.UpsertScenePlayerDiscovery(ctx, mapsdb.UpsertScenePlayerDiscoveryParams{CampaignID: campaignID, PointID: pointID, UserID: user, DiscoveredAt: at}); err != nil {
			return fmt.Errorf("record the scene's discovery by a player: %w", err)
		}
	}
	return nil
}

// DiscoveredScenes returns the scenes the player may tag notes with, with their current
// names, oldest discovery first: the ones the campaign's group discovered and the ones only
// this player discovered (a scene open to some players, DiscoverSceneFor). As the player reads
// them, a scene on a map the players cannot open (not revealed, not the session's current map)
// is not listed, however it was discovered (RN-10). It implements notes.Scenes.
func (sm *SessionMaps) DiscoveredScenes(ctx context.Context, campaignID, userID string) ([]notelink.Scene, error) {
	var current *string
	if sm.svc != nil {
		id, err := sm.svc.currentMap(ctx, campaignID)
		if err != nil {
			return nil, err
		}
		if id != "" {
			current = &id
		}
	}
	rows, err := sm.queries.ListDiscoveredScenes(ctx, mapsdb.ListDiscoveredScenesParams{CampaignID: campaignID, UserID: userID, CurrentMapID: current})
	if err != nil {
		return nil, fmt.Errorf("list the discovered scenes: %w", err)
	}
	out := make([]notelink.Scene, 0, len(rows))
	for _, r := range rows {
		out = append(out, notelink.Scene{ID: r.ID, Name: r.Name})
	}
	return out, nil
}

// ReceivedClues returns the clues revealed to a player of the campaign, newest
// first, as the player got them. It implements notes.Scenes.
func (sm *SessionMaps) ReceivedClues(ctx context.Context, campaignID, userID string) ([]notelink.Clue, error) {
	rows, err := sm.queries.ListReceivedClues(ctx, mapsdb.ListReceivedCluesParams{CampaignID: campaignID, UserID: userID})
	if err != nil {
		return nil, fmt.Errorf("list the received clues: %w", err)
	}
	out := make([]notelink.Clue, 0, len(rows))
	for _, r := range rows {
		c := notelink.Clue{ID: r.ID, Text: r.Text, RevealedAt: r.RevealedAt}
		if r.PointID != nil {
			c.PointID = *r.PointID
		}
		out = append(out, c)
	}
	return out, nil
}

// sceneClues reads a scene point's clues with who has each, for the open scene
// (the master's).
func (sm *SessionMaps) sceneClues(ctx context.Context, q *mapsdb.Queries, pointID string) ([]link.SceneClue, error) {
	rows, err := q.ListSceneClues(ctx, pointID)
	if err != nil {
		return nil, fmt.Errorf("list the scene's clues: %w", err)
	}
	reveals, err := q.ListClueRevealsOfPoint(ctx, &pointID)
	if err != nil {
		return nil, fmt.Errorf("list who has each clue: %w", err)
	}
	out := make([]link.SceneClue, 0, len(rows))
	for _, row := range rows {
		clue := link.SceneClue{ID: row.ID, Text: row.Text}
		for _, r := range reveals {
			if r.ClueID != nil && *r.ClueID == row.ID && r.CharacterID != nil {
				clue.RevealedTo = append(clue.RevealedTo, link.ClueRecipient{CharacterID: *r.CharacterID, RevealedAt: r.RevealedAt})
			}
		}
		out = append(out, clue)
	}
	return out, nil
}
