package maps

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/maps/mapsdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/platform/sessiontell"
	"github.com/PuraFome/meuRPG/backend/internal/platform/uid"
)

// CreateMapPoint implements mapsv1connect.MapServiceHandler.
func (s *Service) CreateMapPoint(
	ctx context.Context,
	req *connect.Request[mapsv1.CreateMapPointRequest],
) (*connect.Response[mapsv1.CreateMapPointResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	in, err := s.checkNewPoint(req.Msg)
	if err != nil {
		return nil, err
	}
	current, err := s.currentMap(ctx, m.CampaignID)
	if err != nil {
		return nil, err
	}
	// The key is unique in the campaign (it shares the column of "Pôr no mapa"), and kept with a
	// hash of the whole request: a retry returns the first point only when it is the same request.
	scopedKey, requestHash, err := keyOf(m.CampaignID, req.Msg.GetIdempotencyKey(), req.Msg)
	if err != nil {
		return nil, err
	}
	created, err := s.createPointTx(ctx, m.CampaignID, in, scopedKey, requestHash, current)
	if err != nil {
		return nil, s.dbError(ctx, "create a point", err)
	}
	out, err := s.masterPoint(ctx, m, created)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&mapsv1.CreateMapPointResponse{Point: out}), nil
}

// newPoint is a CreateMapPoint request after its checks.
type newPoint struct {
	mapID, kind, name, description, hooks string
	showDC                                bool
	x, y                                  int32
	target                                *string
	spec                                  pointSpec
}

// pointText is the cleaned text of a point.
type pointText struct {
	name, description, hooks string
}

// cleanPointText cleans a new point's name, description and hooks, in that order.
func cleanPointText(name, description, hooks string) (pointText, error) {
	var out pointText
	var err error
	if out.name, err = cleanName("name", name); err != nil {
		return pointText{}, err
	}
	if out.description, err = cleanDescription(description); err != nil {
		return pointText{}, err
	}
	if out.hooks, err = cleanHooks(hooks); err != nil {
		return pointText{}, err
	}
	return out, nil
}

// checkSceneOnlyData refuses the hooks and the DC switch on a point that is not
// a scene: the hooks first.
func checkSceneOnlyData(kind, hooks string, showDC bool) error {
	if kind == kindToDB[mapsv1.MapPointKind_MAP_POINT_KIND_SCENE] {
		return nil
	}
	if hooks != "" {
		return errOnlyScenesHaveHooks()
	}
	if showDC {
		return errOnlyScenesShowDC()
	}
	return nil
}

// checkNewPoint reads and checks a CreateMapPoint request, up to the data of
// its kind, before anything is read from the database.
func (s *Service) checkNewPoint(msg *mapsv1.CreateMapPointRequest) (newPoint, error) {
	mapID, ok := uid.Canonical(msg.GetMapId())
	if !ok {
		return newPoint{}, errMapNotFound()
	}
	kind, ok := kindToDB[msg.GetKind()]
	if !ok {
		return newPoint{}, connect.NewError(connect.CodeInvalidArgument, errors.New("kind is required"))
	}
	text, err := cleanPointText(msg.GetName(), msg.GetDescription(), msg.GetHooks())
	if err != nil {
		return newPoint{}, err
	}
	if err := checkSceneOnlyData(kind, text.hooks, msg.GetShowDc()); err != nil {
		return newPoint{}, err
	}
	if err := checkPosition(msg.GetXBp(), msg.GetYBp()); err != nil {
		return newPoint{}, err
	}
	target, err := parseTarget(msg.GetTargetMapId(), kind, mapID)
	if err != nil {
		return newPoint{}, err
	}
	spec, err := s.specFor(kind, msg.GetTrap(), msg.TreasureValuePo, msg.GetLight())
	if err != nil {
		return newPoint{}, err
	}
	return newPoint{
		mapID: mapID, kind: kind, name: text.name, description: text.description, hooks: text.hooks,
		showDC: msg.GetShowDc(), x: msg.GetXBp(), y: msg.GetYBp(), target: target, spec: spec,
	}, nil
}

// createPointTx inserts the point in one transaction, under the session's lock, or finds the one a retry of the same request
// created (replayed), and tells the watchers once it committed. A new point is hidden: only the master hears about it (MR-009),
// unless it is born visible to everyone (a trap created already triggered). A retry tells again what the first call did, with
// the number as it stands.
func (s *Service) createPointTx(ctx context.Context, campaignID string, in newPoint, scopedKey, requestHash *string, current string) (mapsdb.MapPoint, error) {
	var created mapsdb.MapPoint
	err := s.inSession(ctx, campaignID, func(tx pgx.Tx, t sessiontell.Tell) error {
		q := s.queries.WithTx(tx)
		mapRow, err := s.campaignMap(ctx, q, campaignID, in.mapID)
		if err != nil {
			return err
		}
		var replayed bool
		created, replayed, err = idem.Create(ctx, scopedKey, requestHash, q.GetMapPointByCreateKey,
			func(p mapsdb.MapPoint) *string { return p.CreateHash },
			func() (mapsdb.MapPoint, error) {
				return s.insertNewPoint(ctx, q, campaignID, in, scopedKey, requestHash)
			})
		if err != nil {
			return err
		}
		if replayed {
			t.Replay()
		}
		s.announce(t, func(ctx context.Context) {
			s.publishPointsChanged(ctx, campaignID, mapRow, playersSee(in.mapID, mapRow.RevealedAt, current) && everyoneSees(created), created)
		})
		return nil
	})
	return created, err
}

// insertNewPoint counts the map's points against the limit, checks the target
// and inserts the point. It returns pgx.ErrNoRows, as idem.Create wants, when the
// key's unique index already holds a row.
func (s *Service) insertNewPoint(ctx context.Context, q *mapsdb.Queries, campaignID string, in newPoint, scopedKey, requestHash *string) (mapsdb.MapPoint, error) {
	count, err := q.CountMapPoints(ctx, in.mapID)
	if err != nil {
		return mapsdb.MapPoint{}, fmt.Errorf("count points: %w", err)
	}
	if count >= s.maxPoints {
		return mapsdb.MapPoint{}, connect.NewError(connect.CodeResourceExhausted, fmt.Errorf("the map already has %d points", s.maxPoints))
	}
	if err := checkTarget(ctx, q, campaignID, in.target); err != nil {
		return mapsdb.MapPoint{}, err
	}
	p, err := q.InsertMapPoint(ctx, mapsdb.InsertMapPointParams{
		MapID: in.mapID, Kind: in.kind, Name: in.name, Description: in.description, Hooks: in.hooks, ShowDc: in.showDC,
		XBp: in.x, YBp: in.y, TargetMapID: in.target, Now: s.now(),
		Trap: in.spec.trap, TrapState: in.spec.trapSt, TrapTriggeredAt: in.spec.triggeredAt(s.now), TreasureValuePo: in.spec.value,
		LightPreset: in.spec.light.preset, LightBrightFt: in.spec.lightBright(), LightDimFt: in.spec.lightDim(),
		CreateKey: scopedKey, CreateHash: requestHash,
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return p, fmt.Errorf("insert point: %w", err)
	}
	return p, err
}

// findPointForUpdate reads and locks a point of the map.
func findPointForUpdate(ctx context.Context, q *mapsdb.Queries, mapID, pointID string) (mapsdb.MapPoint, error) {
	p, err := q.GetMapPointForUpdate(ctx, mapsdb.GetMapPointForUpdateParams{MapID: mapID, ID: pointID})
	if errors.Is(err, pgx.ErrNoRows) {
		return p, errPointNotFound()
	}
	if err != nil {
		return p, fmt.Errorf("find point: %w", err)
	}
	return p, nil
}

// UpdateMapPoint implements mapsv1connect.MapServiceHandler.
func (s *Service) UpdateMapPoint(
	ctx context.Context,
	req *connect.Request[mapsv1.UpdateMapPointRequest],
) (*connect.Response[mapsv1.UpdateMapPointResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	mapID, ok := uid.Canonical(req.Msg.GetMapId())
	if !ok {
		return nil, errMapNotFound()
	}
	pointID, ok := uid.Canonical(req.Msg.GetPointId())
	if !ok {
		return nil, errPointNotFound()
	}
	change, err := pointChangeOf(req.Msg)
	if err != nil {
		return nil, err
	}
	current, err := s.currentMap(ctx, m.CampaignID)
	if err != nil {
		return nil, err
	}
	ch, err := s.changePointTx(ctx, m.CampaignID, mapID, pointID, change, func(ctx context.Context, ch pointChanged) {
		s.announcePointChange(ctx, m.CampaignID, mapID, current, ch)
		// The open scene shows the point's name and description, and stops being
		// one when the point changes kind.
		s.publishSceneChangedIf(ctx, m.CampaignID, pointID)
	})
	if err != nil {
		return nil, s.dbError(ctx, "update a point", err)
	}
	out, err := s.masterPoint(ctx, m, ch.after)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&mapsv1.UpdateMapPointResponse{Point: out}), nil
}

// applyPointChange checks a change against the point as it is, inside the
// transaction, and saves it.
func (s *Service) applyPointChange(ctx context.Context, q *mapsdb.Queries, campaignID string, p mapsdb.MapPoint, c pointChange) (mapsdb.MapPoint, error) {
	params := s.baseUpdateParams(p)
	c.overlay(&params)
	if err := resolvePointTarget(ctx, q, campaignID, p, c, &params); err != nil {
		return mapsdb.MapPoint{}, err
	}
	if c.revealed != nil {
		params.RevealedAt = revealedAt(p.RevealedAt, *c.revealed, params.Now)
	}
	if err := s.applySpecChange(ctx, q, p, c, &params); err != nil {
		return mapsdb.MapPoint{}, err
	}
	if err := checkChangeSceneOnlyData(params.Kind, c); err != nil {
		return mapsdb.MapPoint{}, err
	}
	if err := leaveScene(ctx, q, p, &params); err != nil {
		return mapsdb.MapPoint{}, err
	}
	if err := discoverScene(ctx, q, campaignID, p, params); err != nil {
		return mapsdb.MapPoint{}, err
	}
	out, err := q.UpdateMapPoint(ctx, params)
	if err != nil {
		return mapsdb.MapPoint{}, fmt.Errorf("update point: %w", err)
	}
	return out, nil
}

// applySpecChange works out the data of the new kinds (a trap, a treasure's
// value, a light) for the point after the change, and cleans up what a point
// that changes kind leaves behind. Each kind needs its own data, and refuses
// another's; a point keeps what it has unless the request replaces it.
func (s *Service) applySpecChange(ctx context.Context, q *mapsdb.Queries, p mapsdb.MapPoint, c pointChange, params *mapsdb.UpdateMapPointParams) error {
	if err := s.applyTrapChange(ctx, q, p, c, params); err != nil {
		return err
	}
	if err := applyTreasureChange(ctx, q, p, c, params); err != nil {
		return err
	}
	return s.applyLightChange(p, c, params)
}

// pointChange is what the master asks to change in a point. Nil fields
// stay as they are.
type pointChange struct {
	kind        *string
	name        *string
	description *string
	hooks       *string // a SCENE point's private text; "" clears it
	showDC      *bool   // a SCENE point's "Mostrar a CD aos jogadores"
	x, y        *int32
	target      *string // "" removes the target
	revealed    *bool
	trap        *mapsv1.TrapSpec
	treasure    *int32
	light       *mapsv1.LightSpec
}

// isEmptyPointChange says that an UpdateMapPoint request names nothing to change.
func isEmptyPointChange(msg *mapsv1.UpdateMapPointRequest) bool {
	return msg.Kind == nil && msg.Name == nil && msg.Description == nil && msg.Hooks == nil && msg.ShowDc == nil && msg.XBp == nil && msg.YBp == nil &&
		msg.TargetMapId == nil && msg.Revealed == nil && msg.Trap == nil && msg.TreasureValuePo == nil && msg.Light == nil
}

// pointChangeOf checks an UpdateMapPoint request and reads what it changes.
func pointChangeOf(msg *mapsv1.UpdateMapPointRequest) (pointChange, error) {
	if isEmptyPointChange(msg) {
		return pointChange{}, connect.NewError(connect.CodeInvalidArgument, errors.New("nothing to change"))
	}
	change := pointChange{
		x: msg.XBp, y: msg.YBp, target: msg.TargetMapId, revealed: msg.Revealed, showDC: msg.ShowDc,
		trap: msg.Trap, treasure: msg.TreasureValuePo, light: msg.Light,
	}
	if err := change.readKind(msg); err != nil {
		return pointChange{}, err
	}
	if err := change.readText(msg); err != nil {
		return pointChange{}, err
	}
	if err := checkPosition(msg.GetXBp(), msg.GetYBp()); err != nil {
		return pointChange{}, err // unset positions read as 0, which is valid
	}
	return change, nil
}

// readKind reads the kind a request sets, if it sets one.
func (c *pointChange) readKind(msg *mapsv1.UpdateMapPointRequest) error {
	if msg.Kind == nil {
		return nil
	}
	kind, ok := kindToDB[msg.GetKind()]
	if !ok {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("kind must not be unspecified"))
	}
	c.kind = &kind
	return nil
}

// readText reads the name, the description and the hooks a request sets, in that
// order.
func (c *pointChange) readText(msg *mapsv1.UpdateMapPointRequest) error {
	if msg.Name != nil {
		name, err := cleanName("name", msg.GetName())
		if err != nil {
			return err
		}
		c.name = &name
	}
	if msg.Description != nil {
		description, err := cleanDescription(msg.GetDescription())
		if err != nil {
			return err
		}
		c.description = &description
	}
	if msg.Hooks != nil {
		hooks, err := cleanHooks(msg.GetHooks())
		if err != nil {
			return err
		}
		c.hooks = &hooks
	}
	return nil
}

// pointChanged is a point as it was and is after a change, with the players who
// had learnt of a trap in it.
type pointChanged struct {
	mapRow        mapsdb.Map
	before, after mapsdb.MapPoint
	knowers       []string
}

// changePointTx locks the point, reads who knows of its trap and saves the
// change, in one transaction under the session's lock; tell runs once it committed.
func (s *Service) changePointTx(ctx context.Context, campaignID, mapID, pointID string, change pointChange, tell func(ctx context.Context, ch pointChanged)) (pointChanged, error) {
	var ch pointChanged
	err := s.inSession(ctx, campaignID, func(tx pgx.Tx, t sessiontell.Tell) error {
		q := s.queries.WithTx(tx)
		var err error
		if ch.mapRow, err = s.campaignMap(ctx, q, campaignID, mapID); err != nil {
			return err
		}
		if ch.before, err = findPointForUpdate(ctx, q, mapID, pointID); err != nil {
			return err
		}
		if ch.knowers, err = s.trapKnowers(ctx, tx, q, campaignID, ch.before); err != nil {
			return err
		}
		if ch.after, err = s.applyPointChange(ctx, q, campaignID, ch.before, change); err != nil {
			return err
		}
		s.announce(t, func(ctx context.Context) { tell(ctx, ch) })
		return nil
	})
	return ch, err
}

// announcePointChange tells the watchers of a changed point: the players who
// see the map and the point (before or after), and those who knew of its trap.
func (s *Service) announcePointChange(ctx context.Context, campaignID, mapID, current string, ch pointChanged) {
	s.publishPointsChanged(ctx, campaignID, ch.mapRow,
		playersSee(mapID, ch.mapRow.RevealedAt, current) && (everyoneSees(ch.before) || everyoneSees(ch.after)), ch.before, ch.after)
	s.tellTrapKnowers(ctx, campaignID, mapID, ch.mapRow, current, ch.knowers)
}

// baseUpdateParams is the update that saves the point as it is.
func (s *Service) baseUpdateParams(p mapsdb.MapPoint) mapsdb.UpdateMapPointParams {
	return mapsdb.UpdateMapPointParams{
		MapID: p.MapID, ID: p.ID, Kind: p.Kind, Name: p.Name, Description: p.Description,
		Hooks: p.Hooks, ShowDc: p.ShowDc, XBp: p.XBp, YBp: p.YBp, TargetMapID: p.TargetMapID, RevealedAt: p.RevealedAt, Now: s.now(),
		Trap: p.Trap, TrapState: p.TrapState, TrapTriggeredAt: p.TrapTriggeredAt, TreasureValuePo: p.TreasureValuePo, TreasureFoundAt: p.TreasureFoundAt,
		TreasureSessionID: p.TreasureSessionID, LightPreset: p.LightPreset, LightBrightFt: p.LightBrightFt, LightDimFt: p.LightDimFt,
	}
}

// overlay puts the plain fields a request sets over the update.
func (c *pointChange) overlay(params *mapsdb.UpdateMapPointParams) {
	if c.kind != nil {
		params.Kind = *c.kind
	}
	if c.name != nil {
		params.Name = *c.name
	}
	if c.description != nil {
		params.Description = *c.description
	}
	if c.hooks != nil {
		params.Hooks = *c.hooks
	}
	if c.showDC != nil {
		params.ShowDc = *c.showDC
	}
	if c.x != nil {
		params.XBp = *c.x
	}
	if c.y != nil {
		params.YBp = *c.y
	}
}

// resolvePointTarget works out the point's target after the change, and checks
// that it is a map of the campaign.
func resolvePointTarget(ctx context.Context, q *mapsdb.Queries, campaignID string, p mapsdb.MapPoint, c pointChange, params *mapsdb.UpdateMapPointParams) error {
	if c.target != nil {
		target, err := parseTarget(*c.target, params.Kind, p.MapID)
		if err != nil {
			return err
		}
		params.TargetMapID = target
	}
	if !leads(params.Kind) {
		params.TargetMapID = nil // only a submap or a battle leads somewhere
	}
	return checkTarget(ctx, q, campaignID, params.TargetMapID)
}

// checkChangeSceneOnlyData refuses the hooks and the DC switch a request sets
// on a point that ends up not being a scene.
func checkChangeSceneOnlyData(kind string, c pointChange) error {
	var hooks string
	if c.hooks != nil {
		hooks = *c.hooks
	}
	return checkSceneOnlyData(kind, hooks, c.showDC != nil && *c.showDC)
}

// leaveScene cleans up what a scene leaves behind when the point changes kind.
func leaveScene(ctx context.Context, q *mapsdb.Queries, p mapsdb.MapPoint, params *mapsdb.UpdateMapPointParams) error {
	scene := kindToDB[mapsv1.MapPointKind_MAP_POINT_KIND_SCENE]
	if p.Kind != scene || params.Kind == p.Kind {
		return nil
	}
	// Only a scene has actions, clues, hooks and the DC switch (MR-015,
	// MR-029). A scene
	// still open in a session then reads as closed: it is not a scene
	// point anymore. What players already received stays in their notes.
	if err := q.DeleteSceneActionsOfPoint(ctx, p.ID); err != nil {
		return fmt.Errorf("delete the scene's actions: %w", err)
	}
	if err := q.DeleteSceneCluesOfPoint(ctx, p.ID); err != nil {
		return fmt.Errorf("delete the scene's clues: %w", err)
	}
	if err := q.DeletePointImages(ctx, p.ID); err != nil {
		return fmt.Errorf("delete the scene's images: %w", err)
	}
	params.Hooks = ""
	params.ShowDc = false
	return nil
}

// discoverScene records a scene that the change reveals for the first time.
func discoverScene(ctx context.Context, q *mapsdb.Queries, campaignID string, p mapsdb.MapPoint, params mapsdb.UpdateMapPointParams) error {
	scene := kindToDB[mapsv1.MapPointKind_MAP_POINT_KIND_SCENE]
	if params.Kind != scene || params.RevealedAt == nil || (p.RevealedAt != nil && p.Kind == scene) {
		return nil
	}
	// Revealing a scene makes it "discovered" (MR-030): the
	// group may tag notes with it from now on, even if it is hidden again.
	if err := q.UpsertSceneDiscovery(ctx, mapsdb.UpsertSceneDiscoveryParams{CampaignID: campaignID, PointID: p.ID, DiscoveredAt: params.Now}); err != nil {
		return fmt.Errorf("record the scene's discovery: %w", err)
	}
	return nil
}

// DeleteMapPoint implements mapsv1connect.MapServiceHandler.
func (s *Service) DeleteMapPoint(
	ctx context.Context,
	req *connect.Request[mapsv1.DeleteMapPointRequest],
) (*connect.Response[mapsv1.DeleteMapPointResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	mapID, ok := uid.Canonical(req.Msg.GetMapId())
	if !ok {
		return nil, errMapNotFound()
	}
	pointID, ok := uid.Canonical(req.Msg.GetPointId())
	if !ok {
		return nil, errPointNotFound()
	}
	current, err := s.currentMap(ctx, m.CampaignID)
	if err != nil {
		return nil, err
	}
	openScene, err := s.live.OpenScenePoint(ctx, m.CampaignID) // the foreign key closes it with the point
	if err != nil {
		return nil, s.dbError(ctx, "read the open scene", err)
	}
	_, err = s.deletePointTx(ctx, m.CampaignID, mapID, pointID, func(ctx context.Context, gone pointDeleted) {
		s.publishPointsChanged(ctx, m.CampaignID, gone.mapRow, playersSee(mapID, gone.mapRow.RevealedAt, current) && everyoneSees(gone.deleted), gone.deleted)
		s.tellTrapKnowers(ctx, m.CampaignID, mapID, gone.mapRow, current, gone.knowers)
		if openScene == pointID {
			s.publishSceneChanged(ctx, m.CampaignID)
		}
	})
	if err != nil {
		return nil, s.dbError(ctx, "delete a point", err)
	}
	return connect.NewResponse(&mapsv1.DeleteMapPointResponse{}), nil
}

// SetMapPointRevealed implements mapsv1connect.MapServiceHandler.
func (s *Service) SetMapPointRevealed(
	ctx context.Context,
	req *connect.Request[mapsv1.SetMapPointRevealedRequest],
) (*connect.Response[mapsv1.SetMapPointRevealedResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	mapID, ok := uid.Canonical(req.Msg.GetMapId())
	if !ok {
		return nil, errMapNotFound()
	}
	pointID, ok := uid.Canonical(req.Msg.GetPointId())
	if !ok {
		return nil, errPointNotFound()
	}
	current, err := s.currentMap(ctx, m.CampaignID)
	if err != nil {
		return nil, err
	}
	revealed := req.Msg.GetRevealed()
	ch, err := s.changePointTx(ctx, m.CampaignID, mapID, pointID, pointChange{revealed: &revealed}, func(ctx context.Context, ch pointChanged) {
		s.announcePointChange(ctx, m.CampaignID, mapID, current, ch)
	})
	if err != nil {
		return nil, s.dbError(ctx, "reveal or hide a point", err)
	}
	out, err := s.masterPoint(ctx, m, ch.after)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&mapsv1.SetMapPointRevealedResponse{Point: out}), nil
}

// pointDeleted is what a deletion leaves to announce.
type pointDeleted struct {
	mapRow  mapsdb.Map
	deleted mapsdb.MapPoint
	knowers []string
}

// checkDeletable refuses to delete a treasure that was found or turned into XP:
// it is part of the session's record, unmarked before it goes, never lost
// silently.
func checkDeletable(p mapsdb.MapPoint) error {
	if p.TreasureConvertedAwardID != nil {
		return errTreasureConverted()
	}
	if p.TreasureFoundAt != nil {
		return errTreasureFound()
	}
	return nil
}

// deletePointTx deletes the point in one transaction, under the session's lock; tell runs once it committed.
func (s *Service) deletePointTx(ctx context.Context, campaignID, mapID, pointID string, tell func(ctx context.Context, gone pointDeleted)) (pointDeleted, error) {
	var out pointDeleted
	err := s.inSession(ctx, campaignID, func(tx pgx.Tx, t sessiontell.Tell) error {
		q := s.queries.WithTx(tx)
		var err error
		if out.mapRow, err = s.campaignMap(ctx, q, campaignID, mapID); err != nil {
			return err
		}
		point, err := findPointForUpdate(ctx, q, mapID, pointID)
		if err != nil {
			return err
		}
		if err := checkDeletable(point); err != nil {
			return err
		}
		// Read before the delete cascades the reveal rows away.
		if out.knowers, err = s.trapKnowers(ctx, tx, q, campaignID, point); err != nil {
			return err
		}
		out.deleted, err = q.DeleteMapPoint(ctx, mapsdb.DeleteMapPointParams{MapID: mapID, ID: pointID})
		if errors.Is(err, pgx.ErrNoRows) {
			return errPointNotFound()
		}
		if err != nil {
			return fmt.Errorf("delete point: %w", err)
		}
		s.announce(t, func(ctx context.Context) { tell(ctx, out) })
		return nil
	})
	return out, err
}
