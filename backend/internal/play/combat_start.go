package play

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/protobuf/proto"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/names"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// The start of a combat, in the order its answers are given: read the request, read what
// the start needs from the database (never through the pool inside the transaction), write
// in the transaction (the open check, the mode, the map, the rows), then tell the streams.

// startCall is what StartEncounter reads from its request alone, checked.
type startCall struct {
	m       authz.Membership
	key     string
	name    string
	pointID *string
	asked   playv1.EncounterMode
	// digest identifies the request, so a retry with the same key and another request is told
	// apart from a true retry.
	digest string
}

// startPlan is a start ready to write: the request, the monsters and participants checked, and
// the battle point read.
type startPlan struct {
	startCall
	monsters startMonsters
	parts    []planned
	point    link.BattlePoint
}

// startOutcome is what the write of a start leaves for the publishers after the commit.
type startOutcome struct {
	newMap string // the map the point made current, if it changed
}

// StartEncounter implements playv1connect.CombatServiceHandler.
func (s *Service) StartEncounter(
	ctx context.Context,
	req *connect.Request[playv1.StartEncounterRequest],
) (*connect.Response[playv1.StartEncounterResponse], error) {
	call, err := readStartRequest(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	plan, err := s.planStart(ctx, call, req.Msg)
	if err != nil {
		return nil, err
	}
	var out startOutcome
	// No request hash: the start is compared by its own digest, which reads the defaults written out as the same request.
	res, err := s.write(ctx, combatWrite{m: call.m, key: call.key, kind: eventEncounterStarted}, func(c *combatTx) (any, error) {
		out = startOutcome{}
		return s.startWrite(ctx, c, plan, &out)
	})
	if err = alreadyOpenOnRace(err); err != nil {
		return nil, s.dbError(ctx, "start an encounter", err)
	}
	if res.repeated && !sameStart(res.payload, call.digest) {
		// A retry answers with the first start only when it asks for the same one.
		return nil, errKeyReused()
	}
	enc, err := s.finishStart(ctx, call.m, res, out)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.StartEncounterResponse{Encounter: enc}), nil
}

// readStartRequest reads and checks the request in the order the answers are given: who may
// call, the key, the name, the battle point, then the mode.
func readStartRequest(ctx context.Context, req *playv1.StartEncounterRequest) (startCall, error) {
	m, err := authz.RequireCampaignRole(ctx, req.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return startCall{}, err
	}
	call := startCall{m: m, asked: req.GetMode(), digest: startDigest(req)}
	if call.key, err = parseKey(req.GetIdempotencyKey()); err != nil {
		return startCall{}, err
	}
	if call.name, err = names.Clean(req.GetName(), maxEncounterName); err != nil {
		return startCall{}, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("name %w", err))
	}
	if call.pointID, err = optionalID(req.GetMapPointId(), "point not found"); err != nil {
		return startCall{}, err
	}
	if _, known := playv1.EncounterMode_name[int32(call.asked)]; !known {
		return startCall{}, connect.NewError(connect.CodeInvalidArgument, errors.New("mode must be a mode of combat"))
	}
	return call, nil
}

// planStart reads what the start needs before the transaction: the monsters, the participants,
// whether the room is enough, and the battle point.
func (s *Service) planStart(ctx context.Context, call startCall, req *playv1.StartEncounterRequest) (startPlan, error) {
	plan := startPlan{startCall: call}
	var err error
	if plan.monsters, err = s.startMonsters(ctx, call.m.CampaignID, req); err != nil {
		return startPlan{}, err
	}
	// With monsters, an empty list of participants is fine: the whole party joins.
	if plan.parts, err = s.participants(ctx, call.m.CampaignID, req.GetParticipants(), true, len(plan.monsters.batches) > 0); err != nil {
		return startPlan{}, err
	}
	// The party's own creatures (a familiar, summoned animals) join with their owners, so they take room too.
	kept, err := s.partyCreatureCount(ctx, call.m.CampaignID, plan.parts, plan.monsters)
	if err != nil {
		return startPlan{}, err
	}
	if total := countPlanned(plan.parts) + plan.monsters.count() + kept; total > maxCombatants {
		return startPlan{}, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TOO_MANY_COMBATANTS, fmt.Sprintf("a combat has at most %d combatants", maxCombatants))
	}
	if call.pointID != nil {
		if plan.point, err = s.maps.BattlePoint(ctx, nil, call.m.CampaignID, *call.pointID); err != nil {
			return startPlan{}, s.dbError(ctx, "find the battle point", err)
		}
	}
	return plan, nil
}

// errAlreadyOpen is the refusal of a start in a session that has a combat.
func errAlreadyOpen() error {
	return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ENCOUNTER_ALREADY_OPEN,
		"the session already has a combat; end it first")
}

// alreadyOpenOnRace turns the unique violation of a start that lost the race to another start
// of the same session into the answer of a start in a session that has a combat.
func alreadyOpenOnRace(err error) error {
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == "23505" && pgErr.ConstraintName == "encounters_one_open_per_session" {
		return errAlreadyOpen()
	}
	return err
}

// startWrite is the transaction of a start: the open check, the mode, then the combat of the
// mode.
func (s *Service) startWrite(ctx context.Context, c *combatTx, p startPlan, out *startOutcome) (any, error) {
	if err := noOpenEncounter(ctx, c); err != nil {
		return nil, err
	}
	// The mode is chosen here, for good (RN-25): the request's, or the table's rule
	// "combate com mapa" read in this transaction.
	asked := p.asked
	if asked == playv1.EncounterMode_ENCOUNTER_MODE_UNSPECIFIED && p.pointID != nil {
		asked = playv1.EncounterMode_ENCOUNTER_MODE_GRID // a battle point leads to a map: it implies a combat on one, whatever the table's rule
	}
	mode, err := s.modeOfStart(ctx, c.tx, p.m.CampaignID, asked)
	if err != nil {
		return nil, err
	}
	if mode == modeTheatre {
		return s.startTheatre(ctx, c, p)
	}
	return s.startOnGrid(ctx, c, p, out)
}

// noOpenEncounter refuses a start in a session that already has a combat.
func noOpenEncounter(ctx context.Context, c *combatTx) error {
	_, err := c.q.GetOpenEncounter(ctx, c.session.ID)
	switch {
	case err == nil:
		return errAlreadyOpen()
	case !errors.Is(err, pgx.ErrNoRows):
		return fmt.Errorf("find the open encounter: %w", err)
	}
	return nil
}

// startTheatre starts a combat without a map: no point, no grid, nobody has a square, and the
// session's current map stays as it is.
func (s *Service) startTheatre(ctx context.Context, c *combatTx, p startPlan) (any, error) {
	if p.pointID != nil {
		return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_THEATRE_HAS_NO_MAP,
			"a combat without a map takes no battle point")
	}
	enc, err := c.q.InsertEncounter(ctx, playdb.InsertEncounterParams{
		GameSessionID: c.session.ID, Name: p.name, CreatedAt: c.now, Mode: modeTheatre,
	})
	if err != nil {
		return nil, fmt.Errorf("insert encounter: %w", err)
	}
	c.enc = enc
	added, err := s.joinStart(ctx, c, p.m, link.Grid{}, p.parts, p.monsters)
	if err != nil {
		return nil, err
	}
	return map[string]any{"encounter_id": enc.ID, "combatants": len(added), "mode": modeTheatre, "digest": p.digest}, nil
}

// startOnGrid starts a combat on the session's current map, or on the map the battle point
// leads to, which then becomes the current one.
func (s *Service) startOnGrid(ctx context.Context, c *combatTx, p startPlan, out *startOutcome) (any, error) {
	mapID, grid, err := s.startMap(ctx, c, p)
	if err != nil {
		return nil, err
	}
	if err := s.showStartMap(ctx, c, p.m.CampaignID, mapID, out); err != nil {
		return nil, err
	}
	enc, err := c.q.InsertEncounter(ctx, playdb.InsertEncounterParams{
		GameSessionID: c.session.ID, MapID: &mapID, MapPointID: p.pointID, Name: p.name,
		GridColumns: grid.Columns, GridRows: grid.Rows, CreatedAt: c.now, Mode: modeGrid,
	})
	if err != nil {
		return nil, fmt.Errorf("insert encounter: %w", err)
	}
	c.enc = enc
	added, err := s.joinStart(ctx, c, p.m, grid, p.parts, p.monsters)
	if err != nil {
		return nil, err
	}
	return map[string]any{"encounter_id": enc.ID, "combatants": len(added), "map_id": mapID, "digest": p.digest}, nil
}

// startMap says which map the fight is on and reads its grid; it writes nothing.
func (s *Service) startMap(ctx context.Context, c *combatTx, p startPlan) (string, link.Grid, error) {
	mapID := deref(c.session.CurrentMapID)
	if p.point.TargetMapID != "" {
		mapID = p.point.TargetMapID
	}
	if mapID == "" {
		return "", link.Grid{}, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NO_CURRENT_MAP,
			"the session has no current map to fight on")
	}
	grid, err := s.maps.MapGrid(ctx, c.tx, p.m.CampaignID, mapID)
	if err != nil {
		return "", link.Grid{}, err
	}
	if !grid.OK() {
		return "", link.Grid{}, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_MAP_HAS_NO_GRID,
			"the map has no grid", func(b *playv1.EncounterBlocked) { b.MapId = mapID })
	}
	return mapID, grid, nil
}

// showStartMap makes the fight's map the current one when it is not: as SetCurrentMap, the
// players see the current map, so it is revealed.
func (s *Service) showStartMap(ctx context.Context, c *combatTx, campaignID, mapID string, out *startOutcome) error {
	if mapID == deref(c.session.CurrentMapID) {
		return nil
	}
	if err := s.maps.RevealMap(ctx, c.tx, campaignID, mapID, c.now); err != nil {
		return err
	}
	if _, err := c.q.SetCurrentMap(ctx, playdb.SetCurrentMapParams{ID: c.session.ID, CurrentMapID: &mapID}); err != nil {
		return fmt.Errorf("set the current map: %w", err)
	}
	out.newMap = mapID
	return nil
}

// finishStart tells the maps and the streams of a start and reads the combat back.
func (s *Service) finishStart(ctx context.Context, m authz.Membership, res combatResult, out startOutcome) (*playv1.Encounter, error) {
	if out.newMap != "" {
		s.maps.MapShown(ctx, m.CampaignID, out.newMap)
	}
	return s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		if out.newMap != "" {
			s.Publish(m.CampaignID, true, &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_CurrentMapChanged_{
				CurrentMapChanged: &playv1.WatchGameSessionResponse_CurrentMapChanged{MapId: out.newMap},
			}})
		}
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
	})
}

// startDigest identifies what a StartEncounter asks for (every field but the idempotency key), so a
// retry with the same key and another request is told apart from a true retry. The event of the
// start keeps it; the monsters, their modes, the participants, the mode, the point and the name
// are all in it.
func startDigest(req *playv1.StartEncounterRequest) string {
	c, ok := proto.Clone(req).(*playv1.StartEncounterRequest)
	if !ok {
		return "" // never: Clone returns the type it was given
	}
	c.IdempotencyKey = ""
	b, err := proto.MarshalOptions{Deterministic: true}.Marshal(c)
	if err != nil {
		return "" // never: a message of this package
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:12])
}

// sameStart says whether the stored event of a start was made by a request with this digest. An
// event written before the digest existed has none and is taken as the same.
func sameStart(payload []byte, digest string) bool {
	var ev struct {
		Digest string `json:"digest"`
	}
	if json.Unmarshal(payload, &ev) != nil || ev.Digest == "" {
		return true
	}
	return ev.Digest == digest
}
