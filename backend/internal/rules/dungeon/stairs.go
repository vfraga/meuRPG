package dungeon

import "slices"

// minStairGap is the least Manhattan distance between two stairs (spec 3.7).
const minStairGap = 5

// stairSite reports whether the corridor square i is a stair site (spec
// 3.7): a dead end (exactly one open 4-neighbor) that is not beside a door
// or on a doorway lane, reached by a straight approach (the square behind it
// is a corridor), with rock on both of its other sides. It returns the
// direction of the one open neighbor.
func (g *gen) stairSite(i int) (behind int, ok bool) {
	if g.kind[i] != KindCorridor || g.lane[i] {
		return 0, false
	}
	x, y := i%g.w, i/g.w
	behind = -1
	for d := range 4 {
		nx, ny := x+dx[d], y+dy[d]
		if !g.inside(nx, ny) {
			continue
		}
		switch g.kind[g.idx(nx, ny)] {
		case KindRock:
		case KindCorridor:
			if behind >= 0 {
				return 0, false
			}
			behind = d
		default: // a door or a room floor beside it
			return 0, false
		}
	}
	return behind, behind >= 0
}

func (g *gen) farFromStairs(x, y int) bool {
	for _, s := range g.stairs {
		if abs(s.X-x)+abs(s.Y-y) < minStairGap {
			return false
		}
	}
	return true
}

func abs(a int) int {
	if a < 0 {
		return -a
	}
	return a
}

// roomSite: a floor square at a corner of a room with no door within 2
// squares (Manhattan).
func (g *gen) roomSite(x, y int) bool {
	for oy := -2; oy <= 2; oy++ {
		for ox := -2; ox <= 2; ox++ {
			if abs(ox)+abs(oy) > 2 {
				continue
			}
			if g.inside(x+ox, y+oy) && g.kind[g.idx(x+ox, y+oy)] == KindDoor {
				return false
			}
		}
	}
	return true
}

// placeStairs is spec 3.7: up to `stairs` stairways at dead ends, then room
// corners for the missing ones. The first is up, the second down, the others
// up or down by a draw (made after all the placements).
func (g *gen) placeStairs() {
	want := g.o.Stairs
	if want == 0 {
		return
	}
	r := stream(g.o.Seed, phaseStairs)
	g.placeCorridorStairs(r, want)
	if len(g.stairs) < want {
		g.placeRoomStairs(r, want)
	}
	g.drawStairKinds(r)
}

// placeCorridorStairs places stairs at the shuffled dead-end sites, each far
// enough from the others.
func (g *gen) placeCorridorStairs(r *rng, want int) {
	var sites []int
	for i := range g.kind {
		if _, ok := g.stairSite(i); ok {
			sites = append(sites, i)
		}
	}
	shuffle(r, sites) // DRAW: the dead-end sites, once
	for _, i := range sites {
		if len(g.stairs) >= want {
			break
		}
		x, y := i%g.w, i/g.w
		if !g.farFromStairs(x, y) {
			continue
		}
		behind, _ := g.stairSite(i)
		g.stairs = append(g.stairs, Stair{X: x, Y: y, Facing: Side((behind + 2) % 4)}) //nolint:gosec // G115: a direction, 0 to 3
		g.protected[i] = true
		g.protected[g.idx(x+dx[behind], y+dy[behind])] = true
	}
}

// stairCorner is a room corner where a stair may go.
type stairCorner struct {
	x, y, room int
	facing     Side
}

// roomCorners are the room sites, gathered in row-major order of the square
// (spec 4.5).
func (g *gen) roomCorners() []stairCorner {
	var cs []stairCorner
	for _, rm := range g.rooms {
		for _, c := range [4]stairCorner{
			{rm.x, rm.y, rm.id, West},
			{rm.x + rm.w - 1, rm.y, rm.id, East},
			{rm.x, rm.y + rm.h - 1, rm.id, West},
			{rm.x + rm.w - 1, rm.y + rm.h - 1, rm.id, East},
		} {
			if g.roomSite(c.x, c.y) {
				cs = append(cs, c)
			}
		}
	}
	slices.SortFunc(cs, func(a, b stairCorner) int { return a.y*g.w + a.x - (b.y*g.w + b.x) })
	return cs
}

// placeRoomStairs places the missing stairs at the shuffled room corners.
func (g *gen) placeRoomStairs(r *rng, want int) {
	cs := g.roomCorners()
	shuffle(r, cs) // DRAW: the room sites, once, only if a stair is missing
	for _, c := range cs {
		if len(g.stairs) >= want {
			break
		}
		if g.farFromStairs(c.x, c.y) {
			g.stairs = append(g.stairs, Stair{X: c.x, Y: c.y, Facing: c.facing, InRoom: c.room})
			g.protected[g.idx(c.x, c.y)] = true
		}
	}
}

// drawStairKinds makes the first stair up, the second down and draws the
// others.
func (g *gen) drawStairKinds(r *rng) {
	for k := range g.stairs {
		switch k {
		case 0:
			g.stairs[k].Kind = StairUp
		case 1:
			g.stairs[k].Kind = StairDown
		default:
			if r.intn(2) == 0 { // DRAW: up or down, for each stair after the second
				g.stairs[k].Kind = StairUp
			} else {
				g.stairs[k].Kind = StairDown
			}
		}
	}
}
