package play

import (
	"math"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/zonewall"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// needsWall says the spell lays a wall: the cast must say its shape and its damaging side.
func needsWall(sp link.Spell) bool {
	d, ok := zoneDefOfSpell(sp)
	return ok && d.kind == zoneWall
}

// wallFor reads the wall of a cast or a preview on a map.
// A spell that lays a wall must have one, laid from a point of origin.
// No other spell takes one.
func wallFor(sp link.Spell, w *playv1.SpellWall, origin *playv1.SpellOrigin) (*zonewall.Spec, error) {
	switch {
	case !needsWall(sp):
		if w != nil {
			return nil, badArea("wall is for a spell that lays a wall")
		}
		return nil, nil
	case w == nil:
		return nil, errWallPlacementRequired()
	case origin == nil:
		return nil, badArea("a wall is laid from the point of origin: set origin")
	}
	spec, err := readWall(w)
	if err != nil {
		return nil, err
	}
	return &spec, nil
}

// errWallPlacementRequired is the refusal of a cast that lays a wall and comes without one.
func errWallPlacementRequired() error {
	return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_WALL_PLACEMENT_REQUIRED,
		"this spell lays a wall: set wall with its shape, size and damaging side")
}

// readWall checks a wall the request sends and fills in its full size.
// A straight wall runs in one of the eight directions and has a left or a right side.
// A ring has no direction and an inner or an outer side.
func readWall(w *playv1.SpellWall) (zonewall.Spec, error) {
	side := w.GetSide()
	spec := zonewall.Spec{Ring: w.GetRing(), Side: side}
	size, sideOK := zonewall.LineFt, side == playv1.WallSide_WALL_SIDE_LEFT || side == playv1.WallSide_WALL_SIDE_RIGHT
	if w.GetRing() {
		size, sideOK = zonewall.RingFt, side == playv1.WallSide_WALL_SIDE_INNER || side == playv1.WallSide_WALL_SIDE_OUTER
		if w.GetDirection() != nil {
			return zonewall.Spec{}, badArea("a ringed wall has no direction")
		}
	} else {
		if w.GetDirection() == nil {
			return zonewall.Spec{}, badArea("a straight wall needs its direction")
		}
		spec.Dir = grid.Direction{Dx: int(w.GetDirection().GetDx()), Dy: int(w.GetDirection().GetDy())}
		if !spec.Dir.Valid() {
			return zonewall.Spec{}, badArea("the direction must be one of the eight: dx and dy from -1 to 1, not both 0")
		}
	}
	if !sideOK {
		return zonewall.Spec{}, badArea("the side must be LEFT or RIGHT for a straight wall and INNER or OUTER for a ring")
	}
	length, err := wallLength(int(w.GetLengthFt()), size)
	spec.LengthFt = length
	return spec, err
}

// wallLength is the length a request asks for: 0 is the full size, anything else a multiple of 5 up to the size.
func wallLength(asked, size int) (int, error) {
	if asked == 0 {
		return size, nil
	}
	if asked < grid.FeetPerSquare || asked > size || asked%grid.FeetPerSquare != 0 {
		return 0, badArea("length_ft must be a multiple of 5 up to the spell's size (60 for a straight wall, 20 for a ring), or 0 for the full size")
	}
	return asked, nil
}

// burning is the squares of the damaging side the wall's own squares do not take, row by row from the top.
func (z *zone) burning() []grid.Square {
	if z.wall == nil {
		return nil
	}
	return z.wall.Burning(z.origin(), z.squares)
}

// hurtsAtEnd says whether the square is one the damaging side of the wall reaches.
func (z *zone) hurtsAtEnd(sq grid.Square) bool { return z.reach[sq] }

// wallStored are the columns of a zone row that keep the wall.
type wallStored struct {
	side     string
	dx, dy   int16
	lengthFt int32
}

// storedWall is the wall as the zone row keeps it; the zero value for a zone that is no wall.
func storedWall(w *zonewall.Spec) wallStored {
	if w == nil {
		return wallStored{}
	}
	return wallStored{
		side: zonewall.SideName(w.Side), dx: int16(w.Dir.Dx), dy: int16(w.Dir.Dy), //nolint:gosec // -1 to 1
		lengthFt: clamp32(w.LengthFt, 0, math.MaxInt32),
	}
}
