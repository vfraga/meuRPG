package dungeon

// The door mixes of spec 2: weights out of 100 for archway, closed, barred,
// locked and secret, in that order, and the chance that a door able to carry
// the flag is trapped.
var (
	mixWeights = map[DoorMix][5]int{
		MixOpen:     {55, 41, 0, 2, 2},
		MixTypical:  {24, 50, 3, 15, 8},
		MixSecured:  {10, 44, 6, 28, 12},
		MixParanoid: {3, 30, 10, 37, 20},
	}
	mixTrapped = map[DoorMix]int{MixOpen: 0, MixTypical: 8, MixSecured: 15, MixParanoid: 25}
)

// sideCount is how many sides a room has: placeDoors gives up after that
// many sides in a row with no candidate.
const sideCount = 4

// maxSideThresholds is the most thresholds a room side has: a side of 31
// squares (the largest RoomSideMax) has 16.
const maxSideThresholds = 16

// threshold is a candidate door position on a room's ring.
type threshold struct {
	doorX, doorY     int // the door square (a wall-line square)
	beyondX, beyondY int // the first square beyond it: always a lattice node
}

// sideLen is how many thresholds a side has (2 squares apart, like nodes).
func (r *room) sideLen(side Side) int {
	if side == North || side == South {
		return (r.w + 1) / 2
	}
	return (r.h + 1) / 2
}

// thresholdAt is the p-th threshold of a side, in reading order along it
// (west to east on the north and south sides, north to south on the others).
func (r *room) thresholdAt(side Side, p int) threshold {
	switch side {
	case North:
		x := r.x + 2*p
		return threshold{x, r.y - 1, x, r.y - 2}
	case South:
		x := r.x + 2*p
		return threshold{x, r.y + r.h, x, r.y + r.h + 1}
	case West:
		y := r.y + 2*p
		return threshold{r.x - 1, y, r.x - 2, y}
	}
	y := r.y + 2*p
	return threshold{r.x + r.w, y, r.x + r.w + 1, y}
}

// thresholdOK is the part of the candidate rule (spec 3.4) shared by every
// phase that opens a door: the door square is rock and not blocked, the
// square beyond is inside the border ring and not blocked, and the door is
// at least 4 squares from every door already on that side of the room (the
// neighboring thresholds, the only places a door can be, are not doors).
// It does not look at what is beyond.
func (g *gen) thresholdOK(r *room, side Side, p int) (threshold, bool) {
	t := r.thresholdAt(side, p)
	if !g.interior(t.beyondX, t.beyondY) {
		return t, false
	}
	di, bi := g.idx(t.doorX, t.doorY), g.idx(t.beyondX, t.beyondY)
	if g.kind[di] != KindRock || g.blocked[di] || g.blocked[bi] {
		return t, false
	}
	for _, q := range [2]int{p - 1, p + 1} {
		if q < 0 || q >= r.sideLen(side) {
			continue
		}
		n := r.thresholdAt(side, q)
		if g.kind[g.idx(n.doorX, n.doorY)] == KindDoor {
			return t, false
		}
	}
	return t, true
}

// candidateOK is thresholdOK plus the rule about a room beyond: it may not
// be the same room (it never is) and a pair of rooms shares at most one
// direct door. Beyond it may be rock, a corridor or another room's floor.
func (g *gen) candidateOK(r *room, side Side, p int) (threshold, bool) {
	t, ok := g.thresholdOK(r, side, p)
	if !ok {
		return t, false
	}
	bi := g.idx(t.beyondX, t.beyondY)
	if g.kind[bi] == KindRoom {
		return t, g.directDoorOK(r, side, t, int(g.rid[bi]))
	}
	return t, true
}

// directDoorOK is the rule for a door into the room other: never the same
// room, at most one direct door per pair, and the door is also on the other
// room's ring, so its own neighbors there (the thresholds 2 squares along the
// shared wall that open into the other room) must not be doors either.
func (g *gen) directDoorOK(r *room, side Side, t threshold, other int) bool {
	if other == r.id {
		return false
	}
	if _, dup := g.directPairs[pairKey(r.id, other)]; dup {
		return false
	}
	ax, ay := 0, 2
	if side == North || side == South {
		ax, ay = 2, 0
	}
	for _, s := range [2]int{-1, 1} {
		if g.otherRoomDoorAt(t.doorX+s*ax, t.doorY+s*ay, t.beyondX+s*ax, t.beyondY+s*ay, other) {
			return false
		}
	}
	return true
}

// otherRoomDoorAt reports whether (nx, ny) is a door that opens at (bx, by)
// into the room other.
func (g *gen) otherRoomDoorAt(nx, ny, bx, by, other int) bool {
	if !g.inside(nx, ny) || !g.inside(bx, by) {
		return false
	}
	n, nb := g.idx(nx, ny), g.idx(bx, by)
	return g.kind[n] == KindDoor && g.kind[nb] == KindRoom && int(g.rid[nb]) == other
}

// openDoor makes the square a door (its kind comes later) and prepares what
// is beyond: another room's floor makes a direct door; otherwise the beyond
// node becomes a corridor "lane" now, so the door always opens onto open
// floor (spec 3.4). Only the doors of 3.4 start maze
// walks (startWalk): after the maze there is nothing left to walk.
func (g *gen) openDoor(r *room, side Side, t threshold, startWalk bool) int {
	di, bi := g.idx(t.doorX, t.doorY), g.idx(t.beyondX, t.beyondY)
	g.kind[di] = KindDoor
	switch g.kind[bi] {
	case KindRoom:
		g.directPairs[pairKey(r.id, int(g.rid[bi]))] = struct{}{}
	case KindRock:
		g.kind[bi] = KindCorridor
		g.lane[bi] = true
		if startWalk {
			g.lanes = append(g.lanes, laneStart{bi, int(side)})
		}
	case KindCorridor:
		g.lane[bi] = true
		if startWalk {
			g.lanes = append(g.lanes, laneStart{bi, int(side)})
		}
	}
	return di
}

// drawKind draws the kind of the door at square di from the "door kinds"
// stream (spec 4.2), shared by every phase that makes a door, in the order the
// doors are made, so door_mix never shifts a position draw: one bounded integer in
// [0, 100) read against the cumulative weights, then, when the kind can carry
// it, the trapped chance (spec 4.5).
func (g *gen) drawKind(di int) {
	r := g.kindRNG
	wts := mixWeights[g.o.DoorMix]
	v := r.intn(100) // DRAW: door kind
	kind, acc := DoorSecret, 0
	for k := range 5 {
		acc += wts[k]
		if v < acc {
			kind = DoorKind(k)
			break
		}
	}
	g.doorKind[di] = kind
	g.doorTrap[di] = kind.canBeTrapped() && r.chance(mixTrapped[g.o.DoorMix]) // DRAW: trapped (none at 0 or 100)
}

// placeDoors is spec 3.4: per room in ID order, the count, the starting
// side, then the round-robin share-out; all the kinds at the end.
func (g *gen) placeDoors() {
	r := stream(g.o.Seed, phaseDoors)
	// A side has at most maxSideThresholds candidates, so cands never grows.
	cands := make([]int, 0, maxSideThresholds)
	var accepted []int
	for _, rm := range g.rooms {
		count := g.doorCount(r, rm)
		// DRAW: starting side, clockwise from there
		side := Side(r.intn(4)) //nolint:gosec // G115: 0 to 3
		accepted = accepted[:0]
		skips := 0
		for len(accepted) < count && skips < sideCount {
			cands = g.sideCandidates(cands[:0], rm, side)
			if len(cands) == 0 {
				skips++
			} else {
				pos := cands[r.intn(len(cands))] // DRAW: one of the valid candidates (none when there is one)
				t, _ := g.candidateOK(rm, side, pos)
				accepted = append(accepted, g.openDoor(rm, side, t, true))
				skips = 0
			}
			side = (side + 1) % 4
		}
		// DRAWS (door kinds stream): kind, then trapped, per door of this room in order of acceptance
		for _, di := range accepted {
			g.drawKind(di)
		}
	}
}

// doorCount is how many doors a room asks for: one, one more per 8 squares
// of ring, a drawn extra one, scaled by DoorDensity and never below one.
func (g *gen) doorCount(r *rng, rm *room) int {
	p := 2 * ((rm.w+1)/2 + (rm.h+1)/2)
	count := 1 + p/8 + r.intn(2) // DRAW: the count's extra door (0..1)
	return max(1, (count*g.o.DoorDensity+50)/100)
}

// sideCandidates appends the valid door positions of one side of a room.
func (g *gen) sideCandidates(cands []int, rm *room, side Side) []int {
	for pos := range rm.sideLen(side) {
		if _, ok := g.candidateOK(rm, side, pos); ok {
			cands = append(cands, pos)
		}
	}
	return cands
}
