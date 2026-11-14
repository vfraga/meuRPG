package maps

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/timestamppb"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/maps/mapsdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/logging"
	"github.com/PuraFome/meuRPG/backend/internal/platform/sessiontell"
	"github.com/PuraFome/meuRPG/backend/internal/platform/uid"
)

// Traps, treasure and who knows about them (MR-035, MR-041, RN-10).
//
// A trap is hidden from a player until it was revealed to one of their
// characters (a row in map_point_reveals), revealed to everyone (the point's
// revealed_at), or triggered. Slice 9.8 writes the first kind of reveal when a
// character notices or finds the trap in play; here the master writes it
// (RevealTrap). The player of the character is told, and nobody else: not the
// master's other players, and no event or response ever names the trap to them.
//
// A treasure follows the existing reveal rule and, once the master marks it
// found (MarkTreasureFound), everyone who sees its map sees it, with what is
// inside, what it is worth and who found it. Slice 9.11 turns found treasure
// into XP and sets treasure_converted_award_id, after which its value and its
// found mark are frozen.

const (
	eventTrapRevealed    = "trap_revealed"
	eventTreasureFound   = "treasure_found"
	eventTreasureUnfound = "treasure_unfound"
)

// The session events' payloads: IDs and numbers only, never a name or a text
// (docs/privacy.md).
type trapRevealedEvent struct {
	PointID      string   `json:"point_id"`
	CharacterIDs []string `json:"character_ids,omitempty"`
	All          bool     `json:"all,omitempty"`
}

type treasureEvent struct {
	PointID      string   `json:"point_id"`
	CharacterIDs []string `json:"character_ids,omitempty"`
	ValuePO      int32    `json:"value_po"`
}

// knownTraps reads which traps the player's characters know (RN-10): by point
// ID, with the map each is on. A character that no longer lives, or whose player
// is someone else, tells the player nothing.
func (s *Service) knownTraps(ctx context.Context, campaignID, userID string) (map[string]knownTrap, error) {
	rows, err := s.queries.ListPointRevealsOfCampaign(ctx, campaignID)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	var ids []string
	for _, r := range rows {
		if !slices.Contains(ids, r.CharacterID) {
			ids = append(ids, r.CharacterID)
		}
	}
	chars, err := s.characters.MapCharacters(ctx, nil, campaignID, ids)
	if err != nil {
		return nil, err
	}
	mine := map[string]bool{}
	for _, c := range chars {
		if c.GetPlayerUserId() != "" && c.GetPlayerUserId() == userID {
			mine[c.GetId()] = true
		}
	}
	known := map[string]knownTrap{}
	for _, r := range rows {
		if mine[r.CharacterID] {
			known[r.PointID] = knownTrap{mapID: r.MapID, public: r.Public}
		}
	}
	return known, nil
}

// pointDetails are the rows attachPointDetails needs besides the points: who
// found each treasure and, for the master, who knows each trap.
type pointDetails struct {
	finders []mapsdb.MapTreasureFinder
	reveals []mapsdb.MapPointReveal
}

// readPointDetails reads a map's pointDetails through q. GetMap passes the
// queries of the read transaction that read the points, so a treasure unmarked
// or a trap changed between the reads is never shown with the wrong finders.
func readPointDetails(ctx context.Context, q *mapsdb.Queries, mapID string, master bool) (pointDetails, error) {
	var d pointDetails
	var err error
	if d.finders, err = q.ListTreasureFindersOfMap(ctx, mapID); err != nil {
		return d, fmt.Errorf("list the treasure finders: %w", err)
	}
	if master {
		if d.reveals, err = q.ListPointRevealsOfMap(ctx, mapID); err != nil {
			return d, fmt.Errorf("list the trap reveals: %w", err)
		}
	}
	return d, nil
}

// attachPointDetails fills what a point's read needs a second query for: who
// found each found treasure (everyone who sees the treasure gets it) and, for
// the master, who knows each trap. The details were read with the points.
func (s *Service) attachPointDetails(ctx context.Context, campaignID string, points []*mapsv1.MapPoint, d pointDetails) error {
	if len(d.finders) == 0 && len(d.reveals) == 0 {
		return nil
	}
	names, order, err := s.characterNames(ctx, campaignID, d.characterIDs())
	if err != nil {
		return err
	}
	for _, p := range points {
		p.TreasureFoundBy = append(p.TreasureFoundBy, foundBy(p, d.finders, order, names)...)
		p.TrapRevealedTo = append(p.TrapRevealedTo, revealedTo(p, d.reveals, names)...)
	}
	return nil
}

// characterIDs are the characters the details name: the finders first, then the ones a
// trap was revealed to.
func (d pointDetails) characterIDs() []string {
	var ids []string
	for _, f := range d.finders {
		ids = append(ids, f.CharacterID)
	}
	for _, r := range d.reveals {
		ids = append(ids, r.CharacterID)
	}
	return ids
}

// foundBy lists who found a treasure that is found, in the order the characters module
// lists them (players' characters first, oldest first).
func foundBy(p *mapsv1.MapPoint, finders []mapsdb.MapTreasureFinder, order []string, names map[string]string) []*mapsv1.TreasureFinder {
	if p.GetTreasureFoundAt() == nil {
		return nil
	}
	var out []*mapsv1.TreasureFinder
	for _, id := range order {
		for _, f := range finders {
			if f.PointID == p.GetId() && f.CharacterID == id {
				out = append(out, &mapsv1.TreasureFinder{CharacterId: id, CharacterName: names[id]})
			}
		}
	}
	return out
}

// revealedTo lists the characters a trap was revealed to, in the order of the rows.
func revealedTo(p *mapsv1.MapPoint, reveals []mapsdb.MapPointReveal, names map[string]string) []*mapsv1.TrapReveal {
	var out []*mapsv1.TrapReveal
	for _, r := range reveals {
		if r.PointID == p.GetId() {
			out = append(out, &mapsv1.TrapReveal{
				CharacterId: r.CharacterID, CharacterName: names[r.CharacterID], How: revealHowFromDB[r.How], At: timestamppb.New(r.At),
			})
		}
	}
	return out
}

// characterNames reads the names of living characters, and the order the
// characters module lists them in (players first, oldest first).
func (s *Service) characterNames(ctx context.Context, campaignID string, ids []string) (map[string]string, []string, error) {
	names := map[string]string{}
	if len(ids) == 0 {
		return names, nil, nil
	}
	found, err := s.characters.MapCharacters(ctx, nil, campaignID, slices.Compact(slices.Sorted(slices.Values(ids))))
	if err != nil {
		return nil, nil, err
	}
	order := make([]string, 0, len(found))
	for _, c := range found {
		names[c.GetId()] = c.GetName()
		order = append(order, c.GetId())
	}
	return names, order, nil
}

var revealHowFromDB = map[string]mapsv1.TrapRevealHow{
	"noticed":  mapsv1.TrapRevealHow_TRAP_REVEAL_HOW_NOTICED,
	"searched": mapsv1.TrapRevealHow_TRAP_REVEAL_HOW_SEARCHED,
	"master":   mapsv1.TrapRevealHow_TRAP_REVEAL_HOW_MASTER,
}

// playerCharacters checks the characters a master named: each must be a living
// player character of the campaign, and, when needUser, have a player. It
// returns their IDs without repeats, and the user of each. Anything else is
// "not found", as for a token's character.
func (s *Service) playerCharacters(ctx context.Context, campaignID string, raw []string, needUser bool) ([]string, map[string]string, error) {
	ids, err := parseCharacterIDs(raw)
	if err != nil {
		return nil, nil, err
	}
	found, err := s.characters.MapCharacters(ctx, nil, campaignID, ids)
	if err != nil {
		return nil, nil, s.dbError(ctx, "find the characters", err)
	}
	userOf := playerUsers(found, needUser)
	for _, id := range ids {
		if _, ok := userOf[id]; !ok {
			return nil, nil, errCharacterNotFound()
		}
	}
	return ids, userOf, nil
}

// parseCharacterIDs reads the characters a master named: 1 to maxRevealCharacters of them,
// each a valid ID, without repeats.
func parseCharacterIDs(raw []string) ([]string, error) {
	if len(raw) == 0 || len(raw) > maxRevealCharacters {
		return nil, badSpec("character_ids must have 1 to %d characters", maxRevealCharacters)
	}
	ids := make([]string, 0, len(raw))
	for _, r := range raw {
		id, ok := uid.Canonical(r)
		if !ok {
			return nil, errCharacterNotFound()
		}
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// playerUsers is the user of each player character among those found (the ones of another
// kind are left out, and so are the ones with no player when needUser).
func playerUsers(found []*charactersv1.CharacterSummary, needUser bool) map[string]string {
	userOf := make(map[string]string, len(found))
	for _, c := range found {
		if c.GetKind() == charactersv1.CharacterKind_CHARACTER_KIND_PLAYER && (!needUser || c.GetPlayerUserId() != "") {
			userOf[c.GetId()] = c.GetPlayerUserId()
		}
	}
	return userOf
}

// pointCall is the start of every call on one point of a map by the master: the
// membership, the IDs, and a map of the campaign.
func (s *Service) pointCall(ctx context.Context, campaignID, mapID, pointID string) (authz.Membership, string, string, error) {
	m, err := authz.RequireCampaignRole(ctx, campaignID, authz.RoleMaster)
	if err != nil {
		return authz.Membership{}, "", "", err
	}
	mid, ok := uid.Canonical(mapID)
	if !ok {
		return authz.Membership{}, "", "", errMapNotFound()
	}
	pid, ok := uid.Canonical(pointID)
	if !ok {
		return authz.Membership{}, "", "", errPointNotFound()
	}
	return m, mid, pid, nil
}

// lockPoint reads and locks a point of a map of the campaign, which must be of
// the kind.
func (s *Service) lockPoint(ctx context.Context, q *mapsdb.Queries, campaignID, mapID, pointID string, kind mapsv1.MapPointKind) (mapsdb.Map, mapsdb.MapPoint, error) {
	mapRow, err := s.campaignMap(ctx, q, campaignID, mapID)
	if err != nil {
		return mapsdb.Map{}, mapsdb.MapPoint{}, err
	}
	p, err := findPointForUpdate(ctx, q, mapID, pointID)
	if err != nil {
		return mapsdb.Map{}, mapsdb.MapPoint{}, err
	}
	if p.Kind != kindToDB[kind] {
		return mapsdb.Map{}, mapsdb.MapPoint{}, badSpec("the point is not a %s point", kind.String()[len("MAP_POINT_KIND_"):])
	}
	return mapRow, p, nil
}

// RevealTrap implements mapsv1connect.MapServiceHandler.
func (s *Service) RevealTrap(
	ctx context.Context,
	req *connect.Request[mapsv1.RevealTrapRequest],
) (*connect.Response[mapsv1.RevealTrapResponse], error) {
	m, mapID, pointID, err := s.pointCall(ctx, req.Msg.GetCampaignId(), req.Msg.GetMapId(), req.Msg.GetPointId())
	if err != nil {
		return nil, err
	}
	all := req.Msg.GetAll()
	if all == (len(req.Msg.GetCharacterIds()) > 0) {
		return nil, badSpec("send character_ids or all, not both and not neither")
	}
	var ids []string
	var userOf map[string]string
	if !all {
		if ids, userOf, err = s.playerCharacters(ctx, m.CampaignID, req.Msg.GetCharacterIds(), true); err != nil {
			return nil, err
		}
	}
	current, err := s.currentMap(ctx, m.CampaignID)
	if err != nil {
		return nil, err
	}

	var res trapReveal
	err = s.inSession(ctx, m.CampaignID, func(tx pgx.Tx, t sessiontell.Tell) error {
		var err error
		if res, err = s.revealTrapTx(ctx, tx, m, trapRevealCall{mapID: mapID, pointID: pointID, all: all, ids: ids, userOf: userOf}); err != nil || !res.changed {
			return err // nothing changed: nothing is told and no number moves
		}
		s.announce(t, func(ctx context.Context) { s.announceTrapReveal(ctx, m.CampaignID, current, all, res) })
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "reveal a trap", err)
	}
	out, err := s.masterPoint(ctx, m, res.after)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&mapsv1.RevealTrapResponse{Point: out}), nil
}

// announceTrapReveal tells who hears of a revealed trap. Everyone who sees the map hears of a trap revealed to all. A trap revealed to
// some characters reaches only their players (and the master, who made the change), never the others.
func (s *Service) announceTrapReveal(ctx context.Context, campaignID, current string, all bool, res trapReveal) {
	seen := playersSee(res.mapRow.ID, res.mapRow.RevealedAt, current)
	s.publishPointsChanged(ctx, campaignID, res.mapRow, all && seen, res.after)
	if !all && seen && len(res.newUsers) > 0 {
		s.publishUsers(ctx, campaignID, res.newUsers, mapChangedEvent(res.mapRow.ID))
	}
}

// trapRevealCall is a RevealTrap request after its checks: the trap, and to whom it is
// revealed (everyone, or the characters of ids, each played by its user in userOf).
type trapRevealCall struct {
	mapID, pointID string
	all            bool
	ids            []string
	userOf         map[string]string
}

// trapReveal is what the RevealTrap transaction ends with: the map and the trap as they
// are now, whether anything changed, and the players who just learned of the trap.
type trapReveal struct {
	mapRow   mapsdb.Map
	after    mapsdb.MapPoint
	newUsers []string
	changed  bool
}

// revealTrapTx is the transaction of RevealTrap: lock the trap, reveal it, and record the
// reveal when it changed something.
func (s *Service) revealTrapTx(ctx context.Context, tx pgx.Tx, m authz.Membership, call trapRevealCall) (trapReveal, error) {
	q := s.queries.WithTx(tx)
	mapRow, before, err := s.lockPoint(ctx, q, m.CampaignID, call.mapID, call.pointID, mapsv1.MapPointKind_MAP_POINT_KIND_TRAP)
	if err != nil {
		return trapReveal{}, err
	}
	res := trapReveal{mapRow: mapRow, after: before}
	now := s.now()
	if call.all {
		reveal := true // as the master's "revealed" switch does
		if res.after, err = s.applyPointChange(ctx, q, m.CampaignID, before, pointChange{revealed: &reveal}); err != nil {
			return trapReveal{}, err
		}
		res.changed = before.RevealedAt == nil
	}
	gave, newUsers, err := s.revealToCharacters(ctx, q, call, now)
	if err != nil {
		return trapReveal{}, err
	}
	res.newUsers = newUsers
	if !res.changed && len(gave) == 0 {
		return res, nil // everyone it names already knew: nothing changes, nothing is recorded
	}
	res.changed = true
	payload, err := json.Marshal(trapRevealedEvent{PointID: call.pointID, CharacterIDs: gave, All: call.all})
	if err != nil {
		return trapReveal{}, fmt.Errorf("encode the event payload: %w", err)
	}
	// With no open session nothing is written here: the reveal is still made.
	if _, err := s.live.AppendEvent(ctx, tx, m.CampaignID, eventTrapRevealed, m.UserID, payload, now); err != nil {
		return trapReveal{}, fmt.Errorf("record the reveal: %w", err)
	}
	return res, nil
}

// revealToCharacters writes a reveal for each character of the call. It returns the ones
// that did not know the trap yet and their players, each once.
func (s *Service) revealToCharacters(ctx context.Context, q *mapsdb.Queries, call trapRevealCall, now time.Time) (gave, newUsers []string, err error) {
	for _, id := range call.ids {
		n, err := q.InsertPointReveal(ctx, mapsdb.InsertPointRevealParams{PointID: call.pointID, CharacterID: id, How: "master", At: now})
		if err != nil {
			return nil, nil, fmt.Errorf("reveal the trap: %w", err)
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

// mapChangedEvent is the stream event that makes a watcher read the map again.
func mapChangedEvent(mapID string) *playv1.WatchGameSessionResponse {
	return &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_MapChanged_{
		MapChanged: &playv1.WatchGameSessionResponse_MapChanged{MapId: mapID},
	}}
}

// MarkTreasureFound implements mapsv1connect.MapServiceHandler.
func (s *Service) MarkTreasureFound(
	ctx context.Context,
	req *connect.Request[mapsv1.MarkTreasureFoundRequest],
) (*connect.Response[mapsv1.MarkTreasureFoundResponse], error) {
	m, mapID, pointID, err := s.pointCall(ctx, req.Msg.GetCampaignId(), req.Msg.GetMapId(), req.Msg.GetPointId())
	if err != nil {
		return nil, err
	}
	ids, _, err := s.playerCharacters(ctx, m.CampaignID, req.Msg.GetCharacterIds(), false)
	if err != nil {
		return nil, err
	}
	current, err := s.currentMap(ctx, m.CampaignID)
	if err != nil {
		return nil, err
	}

	var res treasureChange // changed: this call is the one that found it
	err = s.inSession(ctx, m.CampaignID, func(tx pgx.Tx, t sessiontell.Tell) error {
		var err error
		if res, err = s.markTreasureFoundTx(ctx, tx, m, mapID, pointID, ids); err != nil {
			return err
		}
		// A found treasure is visible to everyone who sees the map.
		s.announce(t, func(ctx context.Context) {
			s.publishPointsChanged(ctx, m.CampaignID, res.mapRow, playersSee(mapID, res.mapRow.RevealedAt, current), res.after)
		})
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "mark a treasure found", err)
	}
	if res.changed {
		logging.Event(ctx, s.logger, "treasure.found", slog.String("map_id", mapID), slog.String("point_id", pointID), slog.Int("finders", len(ids)))
	}
	out, err := s.masterPoint(ctx, m, res.after)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&mapsv1.MarkTreasureFoundResponse{Point: out}), nil
}

// treasureChange is what the transaction of a treasure's find or unmark ends with: the map
// and the treasure as they are now, and whether this call changed it.
type treasureChange struct {
	mapRow  mapsdb.Map
	after   mapsdb.MapPoint
	changed bool
}

// markTreasureFoundTx is the transaction of MarkTreasureFound: lock the treasure, mark it found
// (in the session open now, if any), replace its finders and, when it was not found
// before, record the find.
func (s *Service) markTreasureFoundTx(ctx context.Context, tx pgx.Tx, m authz.Membership, mapID, pointID string, ids []string) (treasureChange, error) {
	var res treasureChange
	q := s.queries.WithTx(tx)
	mapRow, before, err := s.lockPoint(ctx, q, m.CampaignID, mapID, pointID, mapsv1.MapPointKind_MAP_POINT_KIND_TREASURE)
	if err != nil {
		return treasureChange{}, err
	}
	res.mapRow = mapRow
	if before.TreasureConvertedAwardID != nil {
		return treasureChange{}, errTreasureConverted()
	}
	// The session open now, if any, is the one whose summary counts it.
	session, err := s.live.OpenSessionID(ctx, tx, m.CampaignID)
	if err != nil {
		return treasureChange{}, fmt.Errorf("read the open session: %w", err)
	}
	now := s.now()
	params := mapsdb.SetTreasureFoundParams{MapID: mapID, ID: pointID, FoundAt: now, Now: now}
	if session != "" {
		params.SessionID = &session
	}
	if res.after, err = q.SetTreasureFound(ctx, params); err != nil {
		return treasureChange{}, fmt.Errorf("mark the treasure found: %w", err)
	}
	if err := replaceTreasureFinders(ctx, q, pointID, ids); err != nil {
		return treasureChange{}, err
	}
	if before.TreasureFoundAt != nil {
		return res, nil // already found: only the finders changed, and no new event
	}
	payload, err := json.Marshal(treasureEvent{PointID: pointID, CharacterIDs: ids, ValuePO: int32PtrValue(res.after.TreasureValuePo)})
	if err != nil {
		return treasureChange{}, fmt.Errorf("encode the event payload: %w", err)
	}
	if _, err := s.live.AppendEvent(ctx, tx, m.CampaignID, eventTreasureFound, m.UserID, payload, now); err != nil {
		return treasureChange{}, fmt.Errorf("record the find: %w", err)
	}
	res.changed = true
	return res, nil
}

// replaceTreasureFinders: marking a found treasure again changes who found it, and keeps when.
func replaceTreasureFinders(ctx context.Context, q *mapsdb.Queries, pointID string, ids []string) error {
	if err := q.DeleteTreasureFinders(ctx, pointID); err != nil {
		return fmt.Errorf("replace the finders: %w", err)
	}
	for _, id := range ids {
		if err := q.InsertTreasureFinder(ctx, mapsdb.InsertTreasureFinderParams{PointID: pointID, CharacterID: id}); err != nil {
			return fmt.Errorf("record a finder: %w", err)
		}
	}
	return nil
}

// UnmarkTreasureFound implements mapsv1connect.MapServiceHandler.
func (s *Service) UnmarkTreasureFound(
	ctx context.Context,
	req *connect.Request[mapsv1.UnmarkTreasureFoundRequest],
) (*connect.Response[mapsv1.UnmarkTreasureFoundResponse], error) {
	m, mapID, pointID, err := s.pointCall(ctx, req.Msg.GetCampaignId(), req.Msg.GetMapId(), req.Msg.GetPointId())
	if err != nil {
		return nil, err
	}
	current, err := s.currentMap(ctx, m.CampaignID)
	if err != nil {
		return nil, err
	}

	var res treasureChange
	err = s.inSession(ctx, m.CampaignID, func(tx pgx.Tx, t sessiontell.Tell) error {
		var err error
		if res, err = s.unmarkTreasureFoundTx(ctx, tx, m, mapID, pointID); err != nil || !res.changed {
			return err
		}
		// A found treasure was visible to everyone; so was the change.
		s.announce(t, func(ctx context.Context) {
			s.publishPointsChanged(ctx, m.CampaignID, res.mapRow, playersSee(mapID, res.mapRow.RevealedAt, current), res.after)
		})
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "unmark a treasure", err)
	}
	out, err := s.masterPoint(ctx, m, res.after)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&mapsv1.UnmarkTreasureFoundResponse{Point: out}), nil
}

// unmarkTreasureFoundTx is the transaction of UnmarkTreasureFound: a treasure that is not
// found has nothing to unmark; a found one is cleared, its finders forgotten and the
// change recorded.
func (s *Service) unmarkTreasureFoundTx(ctx context.Context, tx pgx.Tx, m authz.Membership, mapID, pointID string) (treasureChange, error) {
	var res treasureChange
	q := s.queries.WithTx(tx)
	mapRow, before, err := s.lockPoint(ctx, q, m.CampaignID, mapID, pointID, mapsv1.MapPointKind_MAP_POINT_KIND_TREASURE)
	if err != nil {
		return treasureChange{}, err
	}
	res.mapRow, res.after = mapRow, before
	if before.TreasureFoundAt == nil {
		return res, nil // not found: nothing to unmark
	}
	if before.TreasureConvertedAwardID != nil {
		return treasureChange{}, errTreasureConverted()
	}
	now := s.now()
	if res.after, err = q.ClearTreasureFound(ctx, mapsdb.ClearTreasureFoundParams{MapID: mapID, ID: pointID, Now: now}); err != nil {
		return treasureChange{}, fmt.Errorf("unmark the treasure: %w", err)
	}
	if err := q.DeleteTreasureFinders(ctx, pointID); err != nil {
		return treasureChange{}, fmt.Errorf("forget the finders: %w", err)
	}
	res.changed = true
	payload, err := json.Marshal(treasureEvent{PointID: pointID, ValuePO: int32PtrValue(res.after.TreasureValuePo)})
	if err != nil {
		return treasureChange{}, fmt.Errorf("encode the event payload: %w", err)
	}
	if _, err := s.live.AppendEvent(ctx, tx, m.CampaignID, eventTreasureUnfound, m.UserID, payload, now); err != nil {
		return treasureChange{}, fmt.Errorf("record the unmark: %w", err)
	}
	return res, nil
}

// trapKnowers returns the players whose characters know a trap that not everyone
// sees: a change to it, or its deletion, must reach them and nobody else. It
// reads inside the transaction, before a delete cascades the reveal rows away.
func (s *Service) trapKnowers(ctx context.Context, tx pgx.Tx, q *mapsdb.Queries, campaignID string, p mapsdb.MapPoint) ([]string, error) {
	if p.Kind != kindToDB[mapsv1.MapPointKind_MAP_POINT_KIND_TRAP] || everyoneSees(p) {
		return nil, nil
	}
	rows, err := q.ListPointRevealsOfPoint(ctx, p.ID)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.CharacterID)
	}
	chars, err := s.characters.MapCharacters(ctx, tx, campaignID, ids)
	if err != nil {
		return nil, fmt.Errorf("find who knows the trap: %w", err)
	}
	var users []string
	for _, c := range chars {
		if u := c.GetPlayerUserId(); u != "" && !slices.Contains(users, u) {
			users = append(users, u)
		}
	}
	return users, nil
}

// tellTrapKnowers tells those players that the map changed, after the commit,
// when the map is one they see.
func (s *Service) tellTrapKnowers(ctx context.Context, campaignID, mapID string, mapRow mapsdb.Map, currentMap string, users []string) {
	if len(users) > 0 && playersSee(mapID, mapRow.RevealedAt, currentMap) {
		s.publishUsers(ctx, campaignID, users, mapChangedEvent(mapID))
	}
}
