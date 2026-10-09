package play

import (
	"context"
	"errors"
	"math"
	"slices"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
	"github.com/PuraFome/meuRPG/backend/internal/rules/zone"
)

// An area spell placed on the map (SRD 5.1, "Areas of Effect", "Spell Range",
// "Targets" and "Cover"). On a map with a grid the caster does not list who the
// spell touches: they choose where it lands (a point for a sphere or a cylinder, a
// direction for a cone, a line or a cube that come out of them) and the server
// works out who is inside, each one's cover from the point of origin, and who the
// cast hits. The shapes and the line of effect are package grid's
// (grid.Area, Terrain.OpenArea); this file puts them on a combat.
//
//   - Everyone inside is affected, the caster's allies and the caster too. A
//     hidden creature is affected like any other: the table rule decides only
//     whether the cast reveals it (combat_reveal.go).
//   - Cover is measured from the point of origin, not from the caster, and only a
//     Dexterity save counts it (SRD, "Cover"); the effect's own rules apply it
//     (resolveOnTarget). A creature with total cover from the origin is not inside,
//     except for what a spell that spreads around corners reaches: there the walls
//     on the straight line mean nothing, and the creature gets no automatic cover
//     (the master's mark still counts).
//   - A player's point that a wall hides comes into being on the near side of it
//     (SRD, "Targets"); the master's does not. A player's range is the spell's,
//     measured to the point they asked for; the master is never held to it.

// placementOf says how a spell's area is set on a grid. A sphere or a cylinder is
// centered on a point within range, or on the caster when the spell's range is
// Self; a cone, a line and a cube come out of the caster (range Self) in one of the
// eight directions. Anything else (a wall far away, a cube at a distance, a spell
// with a range of sight or Touch) stays on the caster's list: the SRD gives the
// shape a point of origin the table does not lay out here.
func placementOf(sp link.Spell) playv1.AreaPlacement {
	if spec, ok := zone.Lookup(sp.Key); ok {
		// A spell that leaves a zone is set at a point within range, or centered on the caster, or a
		// wall from a point in a direction (zones_cast.go): the cube at a distance, which no other
		// spell is placed by, too.
		switch spec.Placement {
		case zone.PlacementPoint:
			return playv1.AreaPlacement_AREA_PLACEMENT_POINT
		case zone.PlacementCaster:
			return playv1.AreaPlacement_AREA_PLACEMENT_CASTER
		case zone.PlacementPointDirection:
			return playv1.AreaPlacement_AREA_PLACEMENT_POINT_DIRECTION
		}
	}
	shape := grid.Shape(sp.AreaShape)
	switch shape {
	case grid.ShapeSphere, grid.ShapeCylinder:
		switch sp.RangeKind {
		case rules.RangeSelf:
			return playv1.AreaPlacement_AREA_PLACEMENT_CASTER
		case rules.RangeRanged:
			return playv1.AreaPlacement_AREA_PLACEMENT_POINT
		}
	case grid.ShapeCone, grid.ShapeLine, grid.ShapeCube:
		if sp.RangeKind == rules.RangeSelf {
			return playv1.AreaPlacement_AREA_PLACEMENT_DIRECTION
		}
	}
	return playv1.AreaPlacement_AREA_PLACEMENT_UNSPECIFIED
}

// areaOf is the grid shape of a spell's area.
func areaOf(sp link.Spell) grid.Area {
	return grid.Area{Shape: grid.Shape(sp.AreaShape), SizeFt: sp.AreaSizeFt, WidthFt: sp.AreaWidthFt}
}

// placedRangeFt is how far the point of origin may be from the caster: the spell's
// range for a point, 0 for the rest.
func placedRangeFt(sp link.Spell) int32 {
	if p := placementOf(sp); p != playv1.AreaPlacement_AREA_PLACEMENT_POINT && p != playv1.AreaPlacement_AREA_PLACEMENT_POINT_DIRECTION {
		return 0
	}
	return clamp32(sp.RangeFt, 0, math.MaxInt32)
}

// areaChoice is what the caster chose: a point or a direction, neither for a spell
// centered on the caster.
type areaChoice struct {
	origin *grid.Square
	dir    *grid.Direction
}

// areaChoiceOf reads the request's point or direction. It checks only what
// the numbers themselves say (a point is on the map, a direction is one of the
// eight); that it is the choice the spell takes is for areaPlan.
func areaChoiceOf(origin *playv1.SpellOrigin, dir *playv1.SpellDirection, g grid.Grid) (areaChoice, error) {
	var out areaChoice
	if origin != nil {
		sq := grid.Square{Col: int(origin.GetCol()), Row: int(origin.GetRow())}
		if !g.Contains(sq) {
			return areaChoice{}, connect.NewError(connect.CodeInvalidArgument, errors.New("the point of origin must be a square of the map"))
		}
		out.origin = &sq
	}
	if dir != nil {
		d := grid.Direction{Dx: int(dir.GetDx()), Dy: int(dir.GetDy())}
		if !d.Valid() {
			return areaChoice{}, connect.NewError(connect.CodeInvalidArgument, errors.New("the direction must be one of the eight: dx and dy from -1 to 1, not both 0"))
		}
		out.dir = &d
	}
	if out.origin != nil && out.dir != nil {
		return areaChoice{}, connect.NewError(connect.CodeInvalidArgument, errors.New("set the point of origin or the direction, not both"))
	}
	return out, nil
}

// areaTarget is a creature inside the area.
type areaTarget struct {
	who playdb.Combatant
	// cp is the cover it has against the point of origin.
	cp coverPair
}

// areaPlan is where an area landed and who is inside, every creature, the ones the
// caster cannot see too. What a viewer is told of it is for the caller to filter.
type areaPlan struct {
	// asked is the square the caster chose (the caster's own for a direction), origin
	// the one the area really has.
	asked, origin grid.Square
	squares       []grid.Square
	targets       []areaTarget
	corners       bool
}

// areaCoverFn is the cover a target has against an origin: a combatant standing on
// the point of origin. The cast reads it with its transaction (coverOf); the
// preview from what the viewer knows.
type areaCoverFn func(origin, target playdb.Combatant) (coverPair, error)

// areaInput is what the plan is made from.
type areaInput struct {
	v       combatViewer
	terrain grid.Terrain // what the area runs over: the real terrain for a cast
	cs      []playdb.Combatant
	caster  playdb.Combatant
	sp      link.Spell
	choice  areaChoice
	cover   areaCoverFn
	// slotLevel and zone are for a spell that leaves a zone: its size follows the slot, and a wall
	// takes a direction, a length or a ring (zones_cast.go).
	slotLevel int
	zone      zoneRequest
}

// rawFromPoint is the squares of the area set on a point, before the walls: the shape of a
// sphere or a cylinder, or, for a spell that leaves a zone, what its catalog lays out.
func (in areaInput) rawFromPoint(area grid.Area, origin grid.Square) []grid.Square {
	if _, ok := zone.Lookup(in.sp.Key); ok {
		return zoneShapeFor(in.sp, in.slotLevel, origin, in.zone)
	}
	return area.FromPoint(origin)
}

func badArea(msg string) error { return connect.NewError(connect.CodeInvalidArgument, errors.New(msg)) }

// planArea places the area and finds who is inside.
func planArea(in areaInput) (areaPlan, error) {
	placement := placementOf(in.sp)
	if placement == playv1.AreaPlacement_AREA_PLACEMENT_UNSPECIFIED {
		return areaPlan{}, badArea("this spell has no area to place on the map")
	}
	if !placed(in.caster) {
		return areaPlan{}, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_PLACED, "the caster must be on the map")
	}
	from := squareOfCombatant(in.caster)
	area := areaOf(in.sp)
	plan := areaPlan{asked: from, origin: from, corners: in.sp.SpreadsAroundCorners}
	var raw []grid.Square
	switch placement {
	case playv1.AreaPlacement_AREA_PLACEMENT_POINT, playv1.AreaPlacement_AREA_PLACEMENT_POINT_DIRECTION:
		if in.choice.origin == nil || in.choice.dir != nil {
			return areaPlan{}, badArea("this spell is placed at a point: set origin")
		}
		plan.asked = *in.choice.origin
		if !in.v.master {
			// The point must be within the spell's range, measured to the square they
			// chose, even when a wall hides it (SRD, "Spell Range").
			if dist := int32(grid.RangeFt(from, plan.asked)); dist > placedRangeFt(in.sp) { //nolint:gosec // a distance on a grid
				return areaPlan{}, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TARGET_OUT_OF_REACH, "the point is beyond the spell's range",
					func(b *playv1.EncounterBlocked) { b.MissingFt = dist - placedRangeFt(in.sp) })
			}
			plan.origin = in.terrain.NearSide(from, plan.asked) // a point behind a wall comes into being on the near side
		} else {
			plan.origin = plan.asked
		}
		raw = in.rawFromPoint(area, plan.origin)
	case playv1.AreaPlacement_AREA_PLACEMENT_DIRECTION:
		if in.choice.dir == nil || in.choice.origin != nil {
			return areaPlan{}, badArea("this spell comes out of the caster: set direction")
		}
		raw = area.FromCaster(from, *in.choice.dir)
	default: // centered on the caster
		if in.choice.origin != nil || in.choice.dir != nil {
			return areaPlan{}, badArea("this spell is centered on the caster: set neither origin nor direction")
		}
		raw = in.rawFromPoint(area, from)
	}
	plan.squares = in.terrain.OpenArea(plan.origin, raw, plan.corners)

	// The creature that stands for the point of origin when the cover is measured.
	// It is the caster when the area begins on the caster's square (nobody counts as
	// cover there, and the caster has none from itself); a point elsewhere has no one
	// on it, so the caster is a body like any other.
	origin := playdb.Combatant{Kind: in.caster.Kind}
	origin.GridCol, origin.GridRow = new(int32), new(int32)
	*origin.GridCol, *origin.GridRow = clamp32(plan.origin.Col, 0, math.MaxInt32), clamp32(plan.origin.Row, 0, math.MaxInt32)
	if plan.origin == from {
		origin.ID = in.caster.ID
	}
	for _, o := range in.cs {
		if o.Defeated || !placed(o) || !slices.Contains(plan.squares, squareOfCombatant(o)) {
			continue
		}
		cp, err := in.cover(origin, o)
		if err != nil {
			return areaPlan{}, err
		}
		if cp.real.total() {
			// A straight line that crosses a wall means nothing to a spell that spreads
			// around corners: the creature is inside, with no cover of the map's, and only
			// the master's mark counts (SRD does not give a cover for a corner).
			if !plan.corners || cp.real.source != playv1.CoverSource_COVER_SOURCE_MAP {
				continue
			}
			cp = markOnly(cp, o)
		}
		plan.targets = append(plan.targets, areaTarget{who: o, cp: cp})
	}
	return plan, nil
}

// markOnly is the cover pair of a creature reached around a corner: no cover of the
// map's, only the master's mark.
func markOnly(cp coverPair, target playdb.Combatant) coverPair {
	mark := coverView{}
	if m := coverToGrid[target.CoverMark]; m > grid.CoverNone {
		mark = coverView{degree: m, source: playv1.CoverSource_COVER_SOURCE_MARK}
	}
	cp.real, cp.shown, cp.restricted, cp.seenBy = mark, mark, false, nil
	return cp
}

// sideOf says which side of the table a combatant is on, for the warning that an
// area hits the caster's allies: the players' (their characters and creatures) or
// the master's.
func sideOf(c playdb.Combatant) bool { return c.Kind != kindNPC }

// coverCounts says the spell's saving throw is one cover helps with: a Dexterity
// save (SRD 5.1, "Cover": +2 or +5 to AC and Dexterity saving throws), and not one of
// the spells that ignore it (Chama Sagrada).
func coverCounts(sp link.Spell) bool { return sp.SaveAbility == "dex" && !sp.IgnoresCover }

// previewTarget is a creature of the plan as the preview tells it.
func previewTarget(t areaTarget, caster playdb.Combatant, origin grid.Square, counts bool, v combatViewer) *playv1.AreaTarget {
	out := &playv1.AreaTarget{
		CombatantId: t.who.ID, Label: t.who.Label, State: stateOf(t.who),
		Ally: sideOf(t.who) == sideOf(caster), Self: t.who.ID == caster.ID,
		DistanceFt: clamp32(grid.RangeFt(origin, squareOfCombatant(t.who)), 0, math.MaxInt32),
		Hidden:     v.master && t.who.Hidden,
	}
	if counts {
		shown := t.cp.shown
		out.Cover, out.CoverSource = coverDegreeProto(shown.key()), shown.source
		if shown.degree == grid.CoverNone {
			out.CoverSource = playv1.CoverSource_COVER_SOURCE_UNSPECIFIED
		}
	}
	return out
}

// squaresProto lists squares for the API.
func squaresProto(sq []grid.Square) []*playv1.SpellOrigin {
	out := make([]*playv1.SpellOrigin, len(sq))
	for i, s := range sq {
		out[i] = originProto(s)
	}
	return out
}

func originProto(s grid.Square) *playv1.SpellOrigin {
	return &playv1.SpellOrigin{Col: clamp32(s.Col, 0, math.MaxInt32), Row: clamp32(s.Row, 0, math.MaxInt32)}
}

// PreviewSpellArea implements playv1connect.CombatServiceHandler.
func (s *Service) PreviewSpellArea(
	ctx context.Context,
	req *connect.Request[playv1.PreviewSpellAreaRequest],
) (*connect.Response[playv1.PreviewSpellAreaResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	encID, err := parseCombatID(req.Msg.GetEncounterId(), "encounter")
	if err != nil {
		return nil, err
	}
	casterID, err := parseCombatID(req.Msg.GetCasterId(), "combatant")
	if err != nil {
		return nil, err
	}
	spellKey := req.Msg.GetSpellKey()
	if spellKey == "" || len(spellKey) > 100 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("spell_key must name one of the caster's spells"))
	}
	_, d, err := s.readEncounter(ctx, m.CampaignID, encID)
	if err != nil {
		return nil, err
	}
	enc := d.enc
	if enc.Status != statusActive {
		return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_ACTIVE, "the combat is not running")
	}
	v, sight, err := s.viewerWith(ctx, m, enc, d.cs)
	if err != nil {
		return nil, s.dbError(ctx, "work out what the player sees", err)
	}
	caster, err := findCombatant(d.cs, casterID, v)
	if err != nil {
		return nil, err
	}
	if err := v.mayAct(caster); err != nil {
		return nil, err
	}
	if isTheatre(enc) {
		return nil, badArea("a combat without a map has no area to place: the master decides who is inside")
	}
	opts, err := s.optionsOf(ctx, nil, m.CampaignID, caster)
	if err != nil {
		return nil, s.dbError(ctx, "work out the turn options", err)
	}
	cast, err := castableOf(opts, spellKey)
	if err != nil {
		return nil, err
	}
	slot, err := slotOf(req.Msg.GetSlot(), cast)
	if err != nil {
		return nil, err
	}
	slotLevel := int(cast.level)
	if slot != nil {
		slotLevel = int(slot.Level)
	}
	sp, err := s.roster.CombatSpell(ctx, nil, m.CampaignID, caster.CharacterID, spellKey, slotLevel, "")
	if err != nil {
		return nil, s.dbError(ctx, "read the spell", err)
	}
	choice, err := areaChoiceOf(req.Msg.GetOrigin(), req.Msg.GetDirection(), grid.Grid{Columns: int(enc.GridColumns), Rows: int(enc.GridRows)})
	if err != nil {
		return nil, err
	}
	terrain, err := s.terrainOf(ctx, nil, m.CampaignID, enc)
	if err != nil {
		return nil, s.dbError(ctx, "read the terrain", err)
	}
	known, err := sight.knownTerrain(ctx, nil, v)
	if err != nil {
		return nil, s.dbError(ctx, "read the terrain the player knows", err)
	}
	// The player is shown the map as they know it: a wall they have not seen is no
	// edge of an area they are told about (RN-10).
	terrain = planOn(v, terrain, known)
	zr, err := previewZoneRequest(req.Msg)
	if err != nil {
		return nil, err
	}
	plan, err := planArea(areaInput{
		v: v, terrain: terrain, cs: d.cs, caster: caster, sp: sp, choice: choice, slotLevel: slotLevel, zone: zr,
		cover: func(origin, target playdb.Combatant) (coverPair, error) {
			cv := coverAgainst(terrain, origin, target, coverPool(d.cs, v))
			return coverPair{real: cv, shown: cv}, nil
		},
	})
	if err != nil {
		return nil, err
	}
	counts := coverCounts(sp)
	out := &playv1.PreviewSpellAreaResponse{
		Origin: originProto(plan.origin), Squares: squaresProto(plan.squares), CoverCounts: counts,
		RangeFt: placedRangeFt(sp), Moved: plan.origin != plan.asked,
	}
	for _, t := range plan.targets {
		if v.canSee(t.who) { // a hidden creature, or one in the dark, is not named or counted
			out.Targets = append(out.Targets, previewTarget(t, caster, plan.origin, counts, v))
		}
	}
	return connect.NewResponse(out), nil
}

// areaProto is where a cast's area landed, for the viewer: the master's is the real
// one, a player's the one drawn on the map they know. The cast event keeps only the
// choice, so a retry's answer is drawn again from it. nil for a cast that is not a
// placed area.
func (s *Service) areaProto(ctx context.Context, res combatResult, ev actionEvent, v combatViewer) (*playv1.SpellArea, error) {
	if !ev.Placed || ev.Zone != nil { // a zone's squares are the zone's (CastSpellResponse.zone)
		return nil, nil
	}
	enc, err := s.queries.GetEncounterInSession(ctx, playdb.GetEncounterInSessionParams{GameSessionID: res.session.ID, ID: res.encounterID})
	if err != nil {
		return nil, s.dbError(ctx, "find the encounter", err)
	}
	cs, err := s.queries.ListCombatants(ctx, enc.ID)
	if err != nil {
		return nil, s.dbError(ctx, "list the combatants", err)
	}
	i := slices.IndexFunc(cs, func(c playdb.Combatant) bool { return c.ID == ev.Actor })
	if i < 0 || !placed(cs[i]) {
		return nil, nil // the caster left the combat meanwhile
	}
	caster := cs[i]
	sp, err := s.roster.CombatSpell(ctx, nil, res.session.CampaignID, caster.CharacterID, ev.Key, slotLevelOf(ev), "")
	if err != nil {
		return nil, s.dbError(ctx, "read the spell", err)
	}
	terrain, err := s.terrainOf(ctx, nil, res.session.CampaignID, enc)
	if err != nil {
		return nil, s.dbError(ctx, "read the terrain", err)
	}
	if !v.master {
		known, err := s.fogKnown(ctx, res.session.CampaignID, enc, v, cs)
		if err != nil {
			return nil, err
		}
		terrain = planOn(v, terrain, known)
	}
	var choice areaChoice
	switch placementOf(sp) {
	case playv1.AreaPlacement_AREA_PLACEMENT_DIRECTION:
		d := grid.Direction{Dx: int(ev.Dx), Dy: int(ev.Dy)}
		choice.dir = &d
	case playv1.AreaPlacement_AREA_PLACEMENT_POINT:
		sq := grid.Square{Col: int(ev.AreaCol), Row: int(ev.AreaRow)}
		choice.origin = &sq
	}
	plan, err := planArea(areaInput{
		v: v, terrain: terrain, cs: nil, caster: caster, sp: sp, choice: choice,
		cover: func(_, _ playdb.Combatant) (coverPair, error) { return coverPair{}, nil },
	})
	if err != nil {
		return nil, nil //nolint:nilerr // the map changed since: the cast keeps its answer, only without a drawing
	}
	return &playv1.SpellArea{Origin: originProto(plan.origin), Squares: squaresProto(plan.squares)}, nil
}

// slotLevelOf is the level a cast was made at, for reading the spell again: the
// slot's, 0 for a cantrip.
func slotLevelOf(ev actionEvent) int {
	if ev.Slot == nil {
		return 0
	}
	return int(ev.Slot.Level)
}

// fogKnown is the terrain a player knows of a combat's map on a fog map, nil without
// the fog and for the master (planOn then uses what the player may be told).
func (s *Service) fogKnown(ctx context.Context, campaignID string, enc playdb.Encounter, v combatViewer, cs []playdb.Combatant) (*grid.Terrain, error) {
	_, sight, err := s.viewerWith(ctx, authz.Membership{CampaignID: campaignID, UserID: v.userID, Role: authz.RolePlayer}, enc, cs)
	if err != nil {
		return nil, s.dbError(ctx, "work out what the player sees", err)
	}
	known, err := sight.knownTerrain(ctx, nil, v)
	if err != nil {
		return nil, s.dbError(ctx, "read the terrain the player knows", err)
	}
	return known, nil
}

// placementFor is the placement a combat's mode gives a spell: without a grid the
// caster lists who the spell touches, whatever its shape.
func placementFor(sp link.Spell, theatre bool) playv1.AreaPlacement {
	if theatre {
		return playv1.AreaPlacement_AREA_PLACEMENT_UNSPECIFIED
	}
	return placementOf(sp)
}

func areaShapeProto(sp link.Spell, theatre bool) rulesv1.SpellAreaShape {
	if placementFor(sp, theatre) == playv1.AreaPlacement_AREA_PLACEMENT_UNSPECIFIED {
		return rulesv1.SpellAreaShape_SPELL_AREA_SHAPE_UNSPECIFIED
	}
	sp = withZoneArea(sp)
	switch grid.Shape(sp.AreaShape) {
	case grid.ShapeCone:
		return rulesv1.SpellAreaShape_SPELL_AREA_SHAPE_CONE
	case grid.ShapeCube:
		return rulesv1.SpellAreaShape_SPELL_AREA_SHAPE_CUBE
	case grid.ShapeCylinder:
		return rulesv1.SpellAreaShape_SPELL_AREA_SHAPE_CYLINDER
	case grid.ShapeLine:
		return rulesv1.SpellAreaShape_SPELL_AREA_SHAPE_LINE
	case grid.ShapeSphere:
		return rulesv1.SpellAreaShape_SPELL_AREA_SHAPE_SPHERE
	}
	return rulesv1.SpellAreaShape_SPELL_AREA_SHAPE_UNSPECIFIED
}

func areaSizeFor(sp link.Spell, theatre bool) int32 {
	if placementFor(sp, theatre) == playv1.AreaPlacement_AREA_PLACEMENT_UNSPECIFIED {
		return 0
	}
	return clamp32(withZoneArea(sp).AreaSizeFt, 0, math.MaxInt32)
}

func areaWidthFor(sp link.Spell, theatre bool) int32 {
	sp = withZoneArea(sp)
	if placementFor(sp, theatre) == playv1.AreaPlacement_AREA_PLACEMENT_UNSPECIFIED || grid.Shape(sp.AreaShape) != grid.ShapeLine {
		return 0
	}
	return clamp32(sp.AreaWidthFt, 0, math.MaxInt32)
}

func placedRangeFor(sp link.Spell, theatre bool) int32 {
	if theatre {
		return 0
	}
	return placedRangeFt(sp)
}
