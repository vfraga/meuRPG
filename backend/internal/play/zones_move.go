package play

import (
	"context"
	"errors"
	"fmt"
	"math"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	"github.com/PuraFome/meuRPG/backend/internal/authz"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/live"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// Moving the area a spell keeps on the map by hand (SRD 5.1, Moonbeam): on each of its turns after
// the cast, the caster may use an action to move the beam up to 60 feet. The creatures the area now
// covers and did not before are caught as if they had come in. The master moves any zone, without the
// action and the distance (a token put right).

// MoveSpellZone implements playv1connect.CombatServiceHandler.
func (s *Service) MoveSpellZone(
	ctx context.Context,
	req *connect.Request[playv1.MoveSpellZoneRequest],
) (*connect.Response[playv1.MoveSpellZoneResponse], error) {
	m, key, encID, err := memberCall(ctx, req.Msg.GetCampaignId(), req.Msg.GetIdempotencyKey(), req.Msg.GetEncounterId())
	if err != nil {
		return nil, err
	}
	zoneID, err := parseCombatID(req.Msg.GetZoneId(), "zone")
	if err != nil {
		return nil, err
	}
	if req.Msg.GetOrigin() == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("origin must be a square of the map"))
	}
	to := grid.Square{Col: int(req.Msg.GetOrigin().GetCol()), Row: int(req.Msg.GetOrigin().GetRow())}
	var moved zoneMoved
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventZoneMoved, encounterID: encID}, func(c *combatTx) (any, error) {
		moved = zoneMoved{}
		return s.moveZoneIn(ctx, c, m, zoneID, to, &moved)
	})
	if err != nil {
		return nil, s.dbError(ctx, "move a spell zone", err)
	}
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishZoneMoved(ctx, m.CampaignID, d.enc, moved.ownerHidden)
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.MoveSpellZoneResponse{Encounter: out}), nil
}

// zoneMoved is what the transaction of MoveSpellZone leaves for the streams.
type zoneMoved struct{ ownerHidden bool }

// moveZoneIn is the transaction of MoveSpellZone: the checks, the action, the new place and what the area
// reaches there.
func (s *Service) moveZoneIn(ctx context.Context, c *combatTx, m authz.Membership, zoneID string, to grid.Square, out *zoneMoved) (any, error) {
	if err := notEnded(c.enc); err != nil {
		return nil, err
	}
	z, owner, err := s.zoneToMove(ctx, c, m, zoneID)
	if err != nil {
		return nil, err
	}
	out.ownerHidden = owner.Hidden
	terrain, err := s.terrainOf(ctx, c.tx, m.CampaignID, c.enc)
	if err != nil {
		return nil, fmt.Errorf("read the terrain: %w", err)
	}
	if !terrain.Grid.Contains(to) || terrain.Walls.Has(to) {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("origin must be a free square of the map"))
	}
	if !c.master {
		if err := s.mayMoveByAction(ctx, c, z, owner, to); err != nil {
			return nil, err
		}
	}
	if err := s.relocate(ctx, c, z, terrain, to, currentMark(c), c.enc.Round); err != nil {
		return nil, err
	}
	if err := touch(ctx, c); err != nil {
		return nil, err
	}
	c.characterID = &owner.CharacterID
	return zoneEvent(z, c.enc.Round, owner.Hidden, "zone_moved", ""), nil
}

// zoneToMove finds the zone the caller may move and its caster: a zone the caller does not read is not
// found, one they read and cannot move is refused, and so is a zone that stays where it is.
func (s *Service) zoneToMove(ctx context.Context, c *combatTx, m authz.Membership, zoneID string) (*zone, playdb.Combatant, error) {
	notFound := connect.NewError(connect.CodeNotFound, errors.New("zone not found"))
	if c.enc.Status != statusActive || isTheatre(c.enc) {
		return nil, playdb.Combatant{}, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_ACTIVE, "the combat is not running on a map")
	}
	row, err := c.q.GetSpellZone(ctx, playdb.GetSpellZoneParams{ID: zoneID, EncounterID: c.enc.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, playdb.Combatant{}, notFound
	}
	if err != nil {
		return nil, playdb.Combatant{}, fmt.Errorf("find the spell zone: %w", err)
	}
	cs, err := listCombatants(ctx, c)
	if err != nil {
		return nil, playdb.Combatant{}, err
	}
	z, ok := readZone(row)
	owner, found := combatantByID(cs, row.OwnerID)
	if !ok || !found || !c.viewer(m, cs).sees(owner) {
		return nil, playdb.Combatant{}, notFound
	}
	if err := c.viewer(m, cs).mayAct(owner); err != nil {
		return nil, playdb.Combatant{}, err
	}
	if z.def.kind == zoneWall {
		return nil, playdb.Combatant{}, connect.NewError(connect.CodeFailedPrecondition, errors.New("a wall is not moved: end it and cast the spell again"))
	}
	if z.def.movement != playv1.SpellZoneMovement_SPELL_ZONE_MOVEMENT_ACTION && !c.master {
		return nil, playdb.Combatant{}, connect.NewError(connect.CodeInvalidArgument, errors.New("this zone is not moved by an action"))
	}
	return z, owner, nil
}

// mayMoveByAction checks what the caster needs to move the area with an action: the caster's turn, the
// action still free and a square within the distance; it spends the action.
func (s *Service) mayMoveByAction(ctx context.Context, c *combatTx, z *zone, owner playdb.Combatant, to grid.Square) error {
	if err := s.mustActNow(ctx, c, owner); err != nil {
		return err
	}
	if owner.ActionUsed {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ACTION_USED, "the action of this turn is used")
	}
	if dist := grid.RangeFt(z.origin(), to); dist > moveRangeFt {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TARGET_OUT_OF_REACH, "the area moves up to 60 feet",
			func(b *playv1.EncounterBlocked) { b.MissingFt = clamp32(dist-moveRangeFt, 0, math.MaxInt32) })
	}
	if err := c.q.SetCombatantEconomy(ctx, playdb.SetCombatantEconomyParams{
		ID: owner.ID, ActionUsed: true, BonusActionUsed: owner.BonusActionUsed, ReactionUsed: owner.ReactionUsed, Dashed: owner.Dashed,
	}); err != nil {
		return fmt.Errorf("spend the action: %w", err)
	}
	return nil
}

// publishZoneMoved tells the streams a zone moved: everyone when its caster is seen, only the master
// when the caster is hidden (a hint to the players would say that something happened).
func (s *Service) publishZoneMoved(ctx context.Context, campaignID string, enc playdb.Encounter, ownerHidden bool) {
	if ownerHidden {
		s.sayOf(ctx)(campaignID, live.Event{Audience: live.Audience{Master: true}, Message: encounterChangedMessage(enc)})
		s.publishLogChanged(ctx, campaignID, enc.ID, false)
		return
	}
	s.publishEncounterChanged(ctx, campaignID, enc)
	s.publishLogChanged(ctx, campaignID, enc.ID, true)
}
