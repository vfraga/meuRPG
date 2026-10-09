package play

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
	"github.com/PuraFome/meuRPG/backend/internal/rules/zone"
)

// A spell that leaves a zone (W7-Z): the cast places the area like any other area spell
// (combat_area.go), and then, instead of rolling a saving throw for each creature in it once,
// it leaves the zone on the map, with the triggers the spell's text gives it (package zone).
// What the zone does then is zones_engine.go's.

// maxWallSquares is the length of the longest wall of fire: 60 ft.
const maxWallSquares = 12

// ringDiameterFt is the diameter of a ringed wall of fire.
const ringDiameterFt = 20

// zoneRequest is what the cast's request says about the zone, beside the point.
type zoneRequest struct {
	excluded []string
	anchored *bool
	side     playv1.ZoneSide
	dir      *grid.Direction
	length   int
	ring     bool
}

// zoneRequestOf reads the cast request's zone fields.
func zoneRequestOf(req *playv1.CastSpellRequest) (zoneRequest, error) {
	z := zoneRequest{excluded: req.GetExcludedCombatantIds(), anchored: req.Anchored, side: req.GetDamageSide(), length: int(req.GetZoneLengthSquares()), ring: req.GetZoneRing()}
	if d := req.GetZoneDirection(); d != nil {
		dir := grid.Direction{Dx: int(d.GetDx()), Dy: int(d.GetDy())}
		if !dir.Valid() {
			return zoneRequest{}, connect.NewError(connect.CodeInvalidArgument, errors.New("zone_direction must be one of the eight: dx and dy from -1 to 1, not both 0"))
		}
		z.dir = &dir
	}
	if z.length < 0 || z.length > maxWallSquares {
		return zoneRequest{}, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("zone_length_squares must be 1 to %d", maxWallSquares))
	}
	if len(z.excluded) > maxSpellTargets {
		return zoneRequest{}, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("excluded_combatant_ids must have at most %d entries", maxSpellTargets))
	}
	return z, nil
}

// checkZoneRequest checks the zone fields of a cast against the spell: each is only for the
// spell that has it, and the ones it needs are there.
func checkZoneRequest(spec zone.Spec, isZone bool, z zoneRequest, master bool) error {
	bad := func(msg string) error { return connect.NewError(connect.CodeInvalidArgument, errors.New(msg)) }
	has := len(z.excluded) > 0 || z.anchored != nil || z.side != playv1.ZoneSide_ZONE_SIDE_UNSPECIFIED || z.dir != nil || z.length != 0 || z.ring
	if !isZone {
		if has {
			return bad("this spell leaves no zone: leave the zone fields out")
		}
		return nil
	}
	if len(z.excluded) > 0 && !spec.Excludes {
		return bad("this spell has no list of creatures to leave out")
	}
	if z.anchored != nil {
		if !spec.Anchors {
			return bad("anchored is for a spell whose zone can be anchored (Teia)")
		}
		if !master {
			return connect.NewError(connect.CodePermissionDenied, errors.New("only the master says whether the webs are anchored"))
		}
	}
	if spec.Sides {
		if z.side == playv1.ZoneSide_ZONE_SIDE_UNSPECIFIED {
			return bad("damage_side says which side of the wall burns: set it")
		}
	} else if z.side != playv1.ZoneSide_ZONE_SIDE_UNSPECIFIED || z.length != 0 || z.ring {
		return bad("damage_side, zone_length_squares and zone_ring are for a wall")
	}
	if spec.Placement == zone.PlacementPointDirection && !z.ring && z.dir == nil {
		return bad("a wall begins at origin and runs in zone_direction: set it")
	}
	if spec.Placement != zone.PlacementPointDirection && z.dir != nil {
		return bad("zone_direction is for a wall")
	}
	return nil
}

// sideOfProto is the side a request names.
func sideOfProto(s playv1.ZoneSide) zone.Side {
	switch s {
	case playv1.ZoneSide_ZONE_SIDE_A:
		return zone.SideA
	case playv1.ZoneSide_ZONE_SIDE_B:
		return zone.SideB
	}
	return zone.SideNone
}

// zoneShapeFor is the raw squares of a zone spell's area at a slot level, before the walls:
// what planArea lays out for a spell the catalog knows.
func zoneShapeFor(sp link.Spell, slotLevel int, origin grid.Square, zr zoneRequest) []grid.Square {
	spec, ok := zone.Lookup(sp.Key)
	if !ok {
		return nil
	}
	if spec.Shape == grid.ShapeWall && zr.ring {
		return grid.RingAt(origin, ringDiameterFt)
	}
	size := spec.Sized(slotLevel)
	if spec.Shape == grid.ShapeWall && zr.length > 0 {
		size = zr.length * grid.FeetPerSquare
	}
	return zone.Shape(spec, size, origin, zr.dir)
}

// zoneCast is what a cast gives the engine to leave its zone.
type zoneCast struct {
	spec      zone.Spec
	sp        link.Spell
	caster    playdb.Combatant
	slotLevel int
	plan      areaPlan
	placed    bool
	req       zoneRequest
	cs        []playdb.Combatant
	viewer    combatViewer
}

// leaveZone makes the zone a cast leaves and runs its trigger at the cast: the creatures in the
// area when the spell is cast (Wall of Fire, Grease, Entangle) get their saving throws as windows.
func (s *Service) leaveZone(ctx context.Context, c *combatTx, in zoneCast) (zoneState, error) {
	spec, sp := in.spec, in.sp
	b := zoneBuild{
		spec: spec, hasSpec: true, spellKey: sp.Key, caster: &in.caster, shape: shapeKeyOf(spec, in.req.ring),
		sizeFt: spec.Sized(in.slotLevel), obscurity: spec.Obscurity, difficult: spec.Difficult, halves: spec.HalvesSpeed,
		camo: spec.Camouflaged, visible: spec.Visible && !spec.Camouflaged, conc: sp.Concentration, slot: in.slotLevel,
		duration: int32(spec.DurationRounds), //nolint:gosec // at most 600
		moves:    spec.Moves, step: spec.StepSquares, casterMv: spec.CasterMoves, triggers: spec.Triggers, rules: spec.Rules,
		dc: max(sp.SpellDC, sp.SaveDC), side: sideOfProto(in.req.side), reach: spec.ReachFt / grid.FeetPerSquare, anchored: true,
		excluded: in.req.excluded,
	}
	if spec.Shape == grid.ShapeWall && in.req.ring {
		b.sizeFt, b.ringR = ringDiameterFt, ringDiameterFt/grid.FeetPerSquare/2
	} else if spec.Shape == grid.ShapeWall && in.req.length > 0 {
		b.sizeFt = in.req.length * grid.FeetPerSquare
	}
	if in.req.dir != nil {
		b.dir = *in.req.dir
	}
	if spec.Anchors && in.req.anchored != nil && !*in.req.anchored {
		// A web that is not anchored collapses and the spell ends at the start of the caster's next turn.
		b.anchored, b.duration = false, 1
	}
	if in.placed {
		origin := in.plan.origin
		b.origin, b.cells = &origin, in.plan.squares
	}
	if slices.ContainsFunc(spec.Triggers, func(t zone.Trigger) bool { return t.Damage }) && len(sp.Damages) > 0 {
		d := sp.Damages[0]
		b.dmg = zoneDamage{count: d.Count, sides: d.Sides, bonus: d.Bonus, kind: d.DamageType}
	}
	if spec.Camouflaged {
		// Whoever sees the area when the spell is cast already knows it; the app takes the caster's side.
		for _, o := range in.cs {
			if !o.Defeated && (o.ID == in.caster.ID || o.Side == in.caster.Side) {
				b.knownBy = append(b.knownBy, o.ID)
			}
		}
	}
	// The caster only leaves out creatures it sees (SRD), which the viewer of a player is; the
	// master lists any.
	for _, id := range in.req.excluded {
		if _, err := findCombatant(in.cs, id, in.viewer); err != nil {
			return zoneState{}, err
		}
	}
	z, err := s.insertZone(ctx, c, b)
	if err != nil {
		return zoneState{}, err
	}
	if err := s.writeZoneEvent(ctx, c, eventMapZoneAdded, z, zoneNote{What: "added", Caster: in.caster.ID}, ""); err != nil {
		return zoneState{}, err
	}
	// The creatures in the area when it appears.
	if _, ok := z.trigger(zone.AtCast); ok {
		for _, t := range in.plan.targets {
			bonus := 0
			if in.sp.SaveAbility == "dex" || z.hasDexSave() {
				bonus = t.cp.real.bonus()
			}
			if err := s.fireZone(ctx, c, z, t.who, fireOpts{kind: zone.AtCast, coverBonus: bonus}); err != nil {
				return zoneState{}, err
			}
		}
	}
	if err := s.syncZoneRules(ctx, c); err != nil {
		return zoneState{}, err
	}
	return z, nil
}

// hasDexSave says the zone's saving throw at the cast is a Dexterity one, the only save cover helps.
func (z zoneState) hasDexSave() bool {
	tr, ok := z.trigger(zone.AtCast)
	return ok && tr.Save == "dex"
}

// shapeKeyOf is the shape a zone of the spec is stored as.
func shapeKeyOf(spec zone.Spec, ring bool) string {
	switch spec.Shape {
	case grid.ShapeSphere:
		return zoneSphere
	case grid.ShapeCylinder:
		return zoneCylinder
	case grid.ShapeCube:
		return zoneCube
	case grid.ShapeWall:
		if ring {
			return zoneRing
		}
		return zoneWall
	}
	return zoneSphere
}

// withZoneArea fills in the area a spell that leaves a zone lays out, for the placement picker
// to outline: the catalog's shape and size at the spell's own level (a Fog Cloud at a higher
// slot is bigger; the preview says the real squares). Spirit Guardians has no area in the data.
func withZoneArea(sp link.Spell) link.Spell {
	spec, ok := zone.Lookup(sp.Key)
	if !ok {
		return sp
	}
	switch spec.Shape {
	case grid.ShapeSphere:
		sp.AreaShape = string(grid.ShapeSphere)
	case grid.ShapeCylinder:
		sp.AreaShape = string(grid.ShapeCylinder)
	case grid.ShapeCube:
		sp.AreaShape = string(grid.ShapeCube)
	case grid.ShapeWall:
		sp.AreaShape, sp.AreaWidthFt = string(grid.ShapeLine), grid.FeetPerSquare
	}
	sp.AreaSizeFt = spec.SizeFt
	return sp
}

// previewZoneRequest reads the zone fields of a preview.
func previewZoneRequest(req *playv1.PreviewSpellAreaRequest) (zoneRequest, error) {
	z := zoneRequest{length: int(req.GetZoneLengthSquares()), ring: req.GetZoneRing()}
	if d := req.GetZoneDirection(); d != nil {
		dir := grid.Direction{Dx: int(d.GetDx()), Dy: int(d.GetDy())}
		if !dir.Valid() {
			return zoneRequest{}, connect.NewError(connect.CodeInvalidArgument, errors.New("zone_direction must be one of the eight: dx and dy from -1 to 1, not both 0"))
		}
		z.dir = &dir
	}
	if z.length < 0 || z.length > maxWallSquares {
		return zoneRequest{}, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("zone_length_squares must be 1 to %d", maxWallSquares))
	}
	return z, nil
}
