package play

import (
	"context"
	"fmt"
	"slices"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// Where the zones are looked at: when a creature changes square (a walk, a jump, a push, a shove, the
// master's forced move, a placement), when a turn starts, and when the area itself moves (the caster
// walks with Spirit Guardians, Moonbeam is moved, Cloudkill drifts). A creature that comes into an
// area it was not in, or begins its turn inside, is caught (zones_trigger.go). The lines the catches
// write are held until the change's own event is written, so the log reads the move, then what it
// brought.

// zoneStep is a creature that changed square: where it stood (nil when it was not on the map) and
// the squares it went through, the last being where it stands now.
type zoneStep struct {
	who  string
	from *grid.Square
	path []grid.Square
}

// pathOf are the squares a creature goes through: the straight line for a walk, only the end for
// anything else (a jump, a push, a forced move put it there).
func pathOf(from *grid.Square, to grid.Square, line bool) []grid.Square {
	if !line || from == nil {
		return []grid.Square{to}
	}
	steps := grid.Line(*from, to)
	out := make([]grid.Square, len(steps))
	for i, st := range steps {
		out[i] = st.Square
	}
	return out
}

// enteredBy says whether the step brought the creature into the zone: one of its squares is in the
// zone and the one before it was not.
func (z *zone) enteredBy(st zoneStep) bool {
	prev := st.from
	for _, sq := range st.path {
		if z.has(sq) && (prev == nil || !z.has(*prev)) {
			return true
		}
		prev = &sq
	}
	return false
}

// currentMark is the turn that runs: the round and the first member of the group on turn.
func currentMark(c *combatTx) string {
	return turnMark(c.enc.Round, conv.OrZero(c.enc.CurrentCombatantID))
}

// zonesAfterSteps looks at the zones for creatures that changed square: the zones of a creature that
// moved follow it when they are centered on it, and the creatures that came into a zone are caught.
func (s *Service) zonesAfterSteps(ctx context.Context, c *combatTx, steps ...zoneStep) error {
	if isTheatre(c.enc) || c.enc.Status != statusActive || len(steps) == 0 {
		return nil
	}
	zs, err := listZones(ctx, c)
	if err != nil || len(zs) == 0 {
		return err
	}
	mark := currentMark(c)
	for _, st := range steps {
		if err := s.followCaster(ctx, c, zs, st.who, mark, c.enc.Round); err != nil {
			return err
		}
		for _, z := range zs {
			if !z.enteredBy(st) {
				continue
			}
			if err := s.catchByID(ctx, c, zoneCatch{z: z, why: zoneWhyEnter, mark: mark, round: c.enc.Round}, st.who); err != nil {
				return err
			}
		}
	}
	return nil
}

// catchByID catches the creature with the id, as it stands now.
func (s *Service) catchByID(ctx context.Context, c *combatTx, k zoneCatch, id string) error {
	cs, err := listCombatants(ctx, c)
	if err != nil {
		return err
	}
	who, ok := combatantByID(cs, id)
	if !ok {
		return nil
	}
	k.who = who
	return s.catchZone(ctx, c, cs, k)
}

// zoneSquares are the squares a zone covers with its origin at the square, after the walls cut it.
func zoneSquares(def zoneDef, terrain grid.Terrain, origin grid.Square) []grid.Square {
	return terrain.OpenArea(origin, def.area().FromPoint(origin), def.corners)
}

// followCaster moves the zones centered on a combatant that moved to where it stands, and catches the
// creatures the new area reaches that the old one did not.
func (s *Service) followCaster(ctx context.Context, c *combatTx, zs []*zone, ownerID, mark string, round int32) error {
	var mine []*zone
	for _, z := range owned(zs, ownerID) {
		if z.def.movement == playv1.SpellZoneMovement_SPELL_ZONE_MOVEMENT_FOLLOWS_CASTER {
			mine = append(mine, z)
		}
	}
	if len(mine) == 0 {
		return nil
	}
	cs, err := listCombatants(ctx, c)
	if err != nil {
		return err
	}
	owner, ok := combatantByID(cs, ownerID)
	if !ok || !placed(owner) {
		return nil
	}
	terrain, err := s.terrainOf(ctx, c.tx, c.session.CampaignID, c.enc)
	if err != nil {
		return fmt.Errorf("read the terrain: %w", err)
	}
	for _, z := range mine {
		if z.origin() == squareOfCombatant(owner) {
			continue
		}
		if err := s.relocate(ctx, c, z, terrain, squareOfCombatant(owner), mark, round); err != nil {
			return err
		}
	}
	return nil
}

// relocate puts a zone with its origin on the square and catches who it now covers and did not before.
func (s *Service) relocate(ctx context.Context, c *combatTx, z *zone, terrain grid.Terrain, origin grid.Square, mark string, round int32) error {
	before := z.in
	if err := z.place(ctx, c, origin, zoneSquares(z.def, terrain, origin)); err != nil {
		return err
	}
	cs, err := listCombatants(ctx, c)
	if err != nil {
		return err
	}
	for _, who := range cs {
		if !placed(who) || before[squareOfCombatant(who)] || !z.has(squareOfCombatant(who)) {
			continue
		}
		if err := s.catchByID(ctx, c, zoneCatch{z: z, why: zoneWhyEnter, mark: mark, round: round}, who.ID); err != nil {
			return err
		}
	}
	return nil
}

// zonesAtTurnStart is what the start of the turn of ids does to the zones: the ones whose time is out
// end, the ones that drift move, and each creature that starts its turn in a zone is caught.
func (s *Service) zonesAtTurnStart(ctx context.Context, c *combatTx, ids []string, round int32) error {
	if isTheatre(c.enc) || len(ids) == 0 {
		return nil
	}
	zs, err := listZones(ctx, c)
	if err != nil || len(zs) == 0 {
		return err
	}
	if zs, err = s.endZonesOutOfTime(ctx, c, zs, ids, round); err != nil {
		return err
	}
	mark := turnMark(round, ids[0])
	if err := s.driftZones(ctx, c, zs, ids, mark, round); err != nil {
		return err
	}
	return s.catchWhoStarts(ctx, c, zs, ids, mark, round)
}

// zonesAtTurnEnd is what the end of a creature's part of the turn does to the zones: it is caught by each
// zone that asks for the creatures that end their turn inside (Grease).
func (s *Service) zonesAtTurnEnd(ctx context.Context, c *combatTx, who playdb.Combatant) error {
	if isTheatre(c.enc) || c.enc.Status != statusActive || !placed(who) {
		return nil
	}
	zs, err := listZones(ctx, c)
	if err != nil {
		return err
	}
	mark := currentMark(c) + ":end"
	for _, z := range zs {
		if !z.def.onTurnEnd || !z.has(squareOfCombatant(who)) {
			continue
		}
		if err := s.catchByID(ctx, c, zoneCatch{z: z, why: zoneWhyEnd, mark: mark, round: c.enc.Round}, who.ID); err != nil {
			return err
		}
	}
	return nil
}

// endZonesOutOfTime ends the zones of the casters that start their turn at or after the round the
// zone's time is out, and returns the zones that are left.
func (s *Service) endZonesOutOfTime(ctx context.Context, c *combatTx, zs []*zone, ids []string, round int32) ([]*zone, error) {
	cs, err := listCombatants(ctx, c)
	if err != nil {
		return nil, err
	}
	var left []*zone
	for _, z := range zs {
		if z.row.EndsRound == nil || round < *z.row.EndsRound || !slices.Contains(ids, z.row.OwnerID) {
			left = append(left, z)
			continue
		}
		if err := c.q.DeleteSpellZone(ctx, z.row.ID); err != nil {
			return nil, fmt.Errorf("end the spell zone: %w", err)
		}
		if err := s.closeZoneWindows(ctx, c, z.row.ID, endDuration); err != nil {
			return nil, err
		}
		c.queueZone(eventLastingEnded, zoneEvent(z, round, ownerHidden(cs, z), "ended", endDuration))
	}
	return left, nil
}

// driftZones moves the zones that drift, owned by a combatant that starts its turn: they go on away from
// it by a fixed distance, as far as the walls let them.
func (s *Service) driftZones(ctx context.Context, c *combatTx, zs []*zone, ids []string, mark string, round int32) error {
	var drifting []*zone
	for _, id := range ids {
		for _, z := range owned(zs, id) {
			if z.def.movement == playv1.SpellZoneMovement_SPELL_ZONE_MOVEMENT_DRIFTS {
				drifting = append(drifting, z)
			}
		}
	}
	if len(drifting) == 0 {
		return nil
	}
	cs, err := listCombatants(ctx, c)
	if err != nil {
		return err
	}
	terrain, err := s.terrainOf(ctx, c.tx, c.session.CampaignID, c.enc)
	if err != nil {
		return fmt.Errorf("read the terrain: %w", err)
	}
	for _, z := range drifting {
		if err := s.drift(ctx, c, z, cs, terrain, mark, round); err != nil {
			return err
		}
	}
	return nil
}

// drift moves one zone away from its caster by the drift distance, as far as the walls let it.
func (s *Service) drift(ctx context.Context, c *combatTx, z *zone, cs []playdb.Combatant, terrain grid.Terrain, mark string, round int32) error {
	owner, ok := combatantByID(cs, z.row.OwnerID)
	if !ok || !placed(owner) {
		return nil
	}
	to := planPush(terrain, grid.OccupantMap{}, grid.Mover{Flier: true}, squareOfCombatant(owner), z.origin(), driftFt).to
	if to == z.origin() {
		return nil
	}
	if err := s.relocate(ctx, c, z, terrain, to, mark, round); err != nil {
		return err
	}
	c.queueZone(eventZoneMoved, zoneEvent(z, round, owner.Hidden, "zone_moved", ""))
	return nil
}

// catchWhoStarts catches each creature that starts its turn in a zone.
func (s *Service) catchWhoStarts(ctx context.Context, c *combatTx, zs []*zone, ids []string, mark string, round int32) error {
	for _, id := range ids {
		for _, z := range zs {
			cs, err := listCombatants(ctx, c)
			if err != nil {
				return err
			}
			who, ok := combatantByID(cs, id)
			if !ok || !placed(who) || !z.has(squareOfCombatant(who)) {
				continue
			}
			if err := s.catchZone(ctx, c, cs, zoneCatch{z: z, who: who, why: zoneWhyStart, mark: mark, round: round}); err != nil {
				return err
			}
		}
	}
	return nil
}

// zoneEvent is the event of a zone that ended or moved: the caster, hidden from a player who does not
// see it.
func zoneEvent(z *zone, round int32, hidden bool, change, reason string) actionEvent {
	return actionEvent{
		Round: round, Secret: hidden, Actor: z.row.OwnerID, Key: z.row.SpellKey,
		Lasting: &lastingEvent{Key: z.row.SpellKey, Change: change, Targets: []string{z.row.OwnerID}, Caster: z.row.OwnerID, Reason: reason},
	}
}

// ownerHidden says the caster of the zone is a combatant the players do not see.
func ownerHidden(cs []playdb.Combatant, z *zone) bool {
	o, ok := combatantByID(cs, z.row.OwnerID)
	return !ok || o.Hidden
}

// zonesAfterMove looks at the zones for what a move put on a new square: the mover, and the creature it
// dragged along.
func (s *Service) zonesAfterMove(ctx context.Context, a *moveAttempt) error {
	if !a.squareChanged {
		return nil
	}
	from := squareOfState(a.made.From)
	steps := []zoneStep{{who: a.target.ID, from: from, path: pathOf(from, a.to, a.req.walking())}}
	if a.draggedMoved != nil {
		steps = append(steps, zoneStep{who: a.draggedMoved.ID, from: a.draggedFrom, path: []grid.Square{squareOfCombatant(*a.draggedMoved)}})
	}
	return s.zonesAfterSteps(ctx, a.c, steps...)
}
