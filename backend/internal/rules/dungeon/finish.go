package dungeon

import (
	"fmt"
	"slices"
)

// finish is spec 3.9: remove doors to nowhere, check that no room lost every
// exit, and derive the doors, exits, corridors, stairs and entrance from the
// final grid.
func (g *gen) finish() (*Dungeon, error) {
	g.removeDoorsToNowhere()
	if err := g.discardRoomsWithoutExit(); err != nil {
		return nil, err
	}
	d := g.newDungeon()
	g.deriveDoors(d)
	g.deriveRooms(d)
	g.deriveCorridors(d)
	g.linkDoorsToCorridors(d)
	g.deriveEntrance(d)
	return d, nil
}

// doorEnds are the two squares a door at square i joins. The axis follows
// from the parity: a door at (odd, even) is in a horizontal wall and is
// crossed north-south (a is north of it); at (even, odd), the other way (a is
// west of it).
func (g *gen) doorEnds(i int) (a, b int, vertical bool) {
	x, y := i%g.w, i/g.w
	if x%2 == 0 {
		return g.idx(x-1, y), g.idx(x+1, y), true
	}
	return g.idx(x, y-1), g.idx(x, y+1), false
}

// removeDoorsToNowhere is step 1: a door whose two neighbors across its wall
// are not both open is a door to nowhere (dead-end removal cut its lane): it
// becomes rock.
func (g *gen) removeDoorsToNowhere() {
	for i, k := range g.kind {
		if k != KindDoor {
			continue
		}
		a, b, _ := g.doorEnds(i)
		if g.kind[a] == KindRock || g.kind[b] == KindRock {
			g.kind[i] = KindRock
			g.doorKind[i], g.doorTrap[i] = DoorArchway, false
		}
	}
}

// discardRoomsWithoutExit is step 2: a room left with no exit is discarded
// (spec 3.9.2; with 3.6 in place and leaves-only pruning it should not
// happen, so SelfCheck fails loudly).
func (g *gen) discardRoomsWithoutExit() error {
	if len(g.rooms) <= 1 {
		return nil
	}
	for _, rm := range g.rooms {
		if g.hasExit(rm) {
			continue
		}
		if g.o.SelfCheck {
			return fmt.Errorf("%w: room %d has no exit", ErrSelfCheck, rm.id)
		}
		g.discardRoom(rm)
	}
	return nil
}

// discardRoom erases a room with no exit and its stairs; the first two
// stairs left are up and down again.
func (g *gen) discardRoom(rm *room) {
	for y := rm.y; y < rm.y+rm.h; y++ {
		for x := rm.x; x < rm.x+rm.w; x++ {
			g.kind[g.idx(x, y)], g.rid[g.idx(x, y)] = KindRock, 0
		}
	}
	rm.discarded = true
	g.roomByID[rm.id] = nil
	g.discarded = append(g.discarded, rm.id)
	g.stairs = slices.DeleteFunc(g.stairs, func(s Stair) bool { return s.InRoom == rm.id })
	for k := range min(2, len(g.stairs)) {
		g.stairs[k].Kind = StairKind(k) // the first is up, the second down
	}
}

// newDungeon starts the result from the grid: blocked squares are rock and
// the silhouette stays in Mask (step 3), then the discarded rooms and the
// stairs.
func (g *gen) newDungeon() *Dungeon {
	d := &Dungeon{
		Version: Version, Options: g.o, Width: g.w, Height: g.h,
		Kinds: g.kind, RoomIDs: g.rid, Mask: make([]bool, len(g.kind)),
		CorridorIDs: make([]uint16, len(g.kind)),
	}
	for i, b := range g.blocked {
		d.Mask[i] = !b
	}
	g.compact()
	d.DiscardedRooms = slices.Clone(g.discarded)
	slices.Sort(d.DiscardedRooms)
	d.StairsPlaced = len(g.stairs)
	d.Stairs = slices.Clone(g.stairs)
	return d
}

// deriveDoors lists the doors in row-major order of the final grid.
func (g *gen) deriveDoors(d *Dungeon) {
	for i, k := range g.kind {
		if k != KindDoor {
			continue
		}
		d.Doors = append(d.Doors, g.doorAt(i, len(d.Doors)+1))
	}
}

// doorAt describes the door on square i: its axis and the rooms it joins.
func (g *gen) doorAt(i, id int) Door {
	dr := Door{ID: id, X: i % g.w, Y: i / g.w, Kind: g.doorKind[i], Trapped: g.doorTrap[i]}
	a, b, vertical := g.doorEnds(i)
	sa, sb := South, North // the side of the room at a, at b
	dr.Axis = HorizontalWall
	if vertical {
		sa, sb = East, West
		dr.Axis = VerticalWall
	}
	ra, rb := int(g.rid[a]), int(g.rid[b])
	switch {
	case ra > 0 && rb > 0:
		dr.RoomA, dr.RoomB, dr.Side = ra, rb, sa
	case ra > 0:
		dr.RoomA, dr.Side = ra, sa
	default:
		dr.RoomA, dr.Side = rb, sb
	}
	return dr
}

// deriveRooms lists the rooms and their exits (4: a direct door shows in
// both rooms).
func (g *gen) deriveRooms(d *Dungeon) {
	for _, rm := range g.rooms {
		out := Room{ID: rm.id, X: rm.x, Y: rm.y, Width: rm.w, Height: rm.h}
		for side := North; side <= West; side++ {
			out.Exits = g.sideExits(d, out.Exits, rm, side)
		}
		d.Rooms = append(d.Rooms, out)
	}
}

// sideExits appends the exits on one side of a room, in position order.
func (g *gen) sideExits(d *Dungeon, exits []Exit, rm *room, side Side) []Exit {
	for p := range rm.sideLen(side) {
		t := rm.thresholdAt(side, p)
		if g.kind[g.idx(t.doorX, t.doorY)] != KindDoor {
			continue
		}
		dr := d.Doors[doorIndex(d.Doors, t.doorX, t.doorY)]
		other := 0
		if bi := g.idx(t.beyondX, t.beyondY); g.kind[bi] == KindRoom {
			other = int(g.rid[bi])
		}
		exits = append(exits, Exit{Side: side, X: t.doorX, Y: t.doorY, DoorID: dr.ID, Kind: dr.Kind, Trapped: dr.Trapped, Other: other})
	}
	return exits
}

// linkDoorsToCorridors gives each door into a corridor that corridor's ID.
func (g *gen) linkDoorsToCorridors(d *Dungeon) {
	for i := range d.Doors {
		dr := &d.Doors[i]
		if dr.RoomB == 0 {
			bx, by := corridorEnd(d, *dr)
			dr.Corridor = int(d.CorridorIDs[by*g.w+bx])
		}
	}
}

// deriveEntrance is 1.4: the first stair, else the center of the lowest room.
func (g *gen) deriveEntrance(d *Dungeon) {
	if len(d.Stairs) == 0 {
		cx, cy := d.Rooms[0].Center()
		d.Entrance = Entrance{X: cx, Y: cy, Room: d.Rooms[0].ID}
		return
	}
	s := d.Stairs[0]
	d.Entrance = Entrance{X: s.X, Y: s.Y, Room: s.InRoom, OnStairs: true}
	if s.InRoom == 0 {
		d.Entrance.Corridor = int(d.CorridorIDs[g.idx(s.X, s.Y)])
	}
}

// corridorScan is the scratch of deriveCorridors: the squares of the run
// being labeled, the junctions it touches, and every corridor's ends back to
// back.
type corridorScan struct {
	q, touched []int
	pool       []CorridorEnd
}

// corridorScanCap is the starting room of the run and junction lists of a
// corridorScan: most runs are shorter, so they rarely grow.
const corridorScanCap = 64

// junction reports whether square i is a corridor junction (three or more
// open neighbors).
func (g *gen) junction(i int) bool { return g.kind[i] == KindCorridor && g.openDeg(i) >= 3 }

// deriveCorridors labels the corridor squares (spec 1.5 d): a maximal run of
// corridor squares that are not junctions is one corridor; a junction square
// (three or more open neighbors) is a corridor of one square. IDs follow the
// row-major order of the first square.
func (g *gen) deriveCorridors(d *Dungeon) {
	nCorr := 0
	for _, k := range g.kind {
		if k == KindCorridor {
			nCorr++
		}
	}
	// every corridor's Ends, back to back (sized from the corridor squares: a run
	// is rarely shorter than 3, and each has about one end)
	s := &corridorScan{
		q:       make([]int, 0, corridorScanCap),
		touched: make([]int, 0, corridorScanCap),
		pool:    make([]CorridorEnd, 0, nCorr/2+8),
	}
	span := make([][2]int, 0, nCorr/2+8)
	d.Corridors = make([]Corridor, 0, nCorr/2+8)
	for i0, k := range g.kind {
		if k != KindCorridor || d.CorridorIDs[i0] != 0 {
			continue
		}
		start := len(s.pool)
		d.Corridors = append(d.Corridors, g.labelCorridor(d, s, i0))
		span = append(span, [2]int{start, len(s.pool)})
	}
	attachCorridorEnds(d, s.pool, span)
}

// labelCorridor labels the run that starts at square i0 (breadth-first) and
// collects its ends.
func (g *gen) labelCorridor(d *Dungeon, s *corridorScan, i0 int) Corridor {
	c := Corridor{ID: len(d.Corridors) + 1, FirstX: i0 % g.w, FirstY: i0 / g.w}
	id := uint16(c.ID) //nolint:gosec // G115: corridor IDs fit uint16 (at most ~20 000 nodes)
	d.CorridorIDs[i0] = id
	s.q = append(s.q[:0], i0)
	c.Junction = g.junction(i0)
	s.touched = s.touched[:0]
	for head := 0; head < len(s.q); head++ { //nolint:intrange // scanCorridorSquare appends to s.q while the loop runs
		g.scanCorridorSquare(d, s, s.q[head], c.Junction, id)
	}
	c.Length = len(s.q)
	c.Junctions = len(s.touched)
	return c
}

// scanCorridorSquare looks around one square of a run: a square with one
// open neighbor is an end (a door end or a dead end, the one neighbor
// decides); then each neighbor is a door end, a junction the run touches or
// the next square of the run.
func (g *gen) scanCorridorSquare(d *Dungeon, s *corridorScan, u int, junc bool, id uint16) {
	x, y := u%g.w, u/g.w
	deg1 := g.openDeg(u) == 1
	if !junc && deg1 {
		for dd := range 4 {
			switch g.kind[g.idx(x+dx[dd], y+dy[dd])] {
			case KindDoor:
				s.pool = append(s.pool, CorridorEnd{Type: "door", X: x + dx[dd], Y: y + dy[dd]})
			case KindCorridor:
				s.pool = append(s.pool, CorridorEnd{Type: "dead_end", X: x, Y: y})
			}
		}
	}
	for dd := range 4 {
		g.visitCorridorNeighbor(d, s, g.idx(x+dx[dd], y+dy[dd]), deg1, junc, id)
	}
}

// visitCorridorNeighbor handles the neighbor v of a run's square: a door is
// an end unless the square has one open neighbor (then it was counted
// already); a junction beside a run is touched once; a free corridor square
// joins the run. A junction's own run stops at itself.
func (g *gen) visitCorridorNeighbor(d *Dungeon, s *corridorScan, v int, deg1, junc bool, id uint16) {
	switch g.kind[v] {
	case KindDoor:
		if !deg1 {
			s.pool = append(s.pool, CorridorEnd{Type: "door", X: v % g.w, Y: v / g.w})
		}
	case KindCorridor:
		switch {
		case junc:
		case g.junction(v):
			if !slices.Contains(s.touched, v) {
				s.touched = append(s.touched, v)
				s.pool = append(s.pool, CorridorEnd{Type: "junction", X: v % g.w, Y: v / g.w})
			}
		case d.CorridorIDs[v] == 0:
			d.CorridorIDs[v] = id
			s.q = append(s.q, v)
		}
	}
}

// attachCorridorEnds gives each corridor its slice of the pool and each door
// end its door's ID.
func attachCorridorEnds(d *Dungeon, pool []CorridorEnd, span [][2]int) {
	for ci := range d.Corridors {
		sp := span[ci]
		if sp[1] > sp[0] {
			d.Corridors[ci].Ends = pool[sp[0]:sp[1]:sp[1]]
		}
		for ei := range d.Corridors[ci].Ends {
			e := &d.Corridors[ci].Ends[ei]
			if e.Type == "door" {
				e.DoorID = d.Doors[doorIndex(d.Doors, e.X, e.Y)].ID
			}
		}
	}
}

// doorIndex finds the door on a square; the doors are in row-major order.
func doorIndex(doors []Door, x, y int) int {
	i, _ := slices.BinarySearchFunc(doors, [2]int{x, y}, func(d Door, p [2]int) int {
		if d.Y != p[1] {
			return d.Y - p[1]
		}
		return d.X - p[0]
	})
	return i
}

// hasExit reports whether a door is open on the room's ring.
func (g *gen) hasExit(rm *room) bool {
	for side := North; side <= West; side++ {
		for p := 0; p < rm.sideLen(side); p++ {
			t := rm.thresholdAt(side, p)
			if g.kind[g.idx(t.doorX, t.doorY)] == KindDoor {
				return true
			}
		}
	}
	return false
}
