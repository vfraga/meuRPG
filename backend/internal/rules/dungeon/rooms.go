package dungeon

// roomFits reports whether a room whose floor starts at the odd square
// (x, y) with sides (w, h) is valid (spec 3.3): floor and the one-square
// wall ring inside the border ring, no blocked square in either (the
// smallest reading of "must contain no blocked square": the ring is part of
// the room's reserved space), and no overlap with the floor of another room.
// Two floors that are disjoint always keep a wall line between them, because
// every floor starts and ends on an odd coordinate.
func (g *gen) roomFits(x, y, w, h int) bool {
	if x < 1 || y < 1 || x+w-1 > g.w-2 || y+h-1 > g.h-2 {
		return false
	}
	for yy := y - 1; yy <= y+h; yy++ {
		for xx := x - 1; xx <= x+w; xx++ {
			i := g.idx(xx, yy)
			if g.blocked[i] {
				return false
			}
			if yy >= y && yy < y+h && xx >= x && xx < x+w && g.kind[i] != KindRock {
				return false
			}
		}
	}
	return true
}

// aspectOK: the longest side over the shortest is within AspectLimit, in
// integers.
func (g *gen) aspectOK(w, h int) bool {
	long, short := max(w, h), min(w, h)
	return long*100 <= g.aspect*short
}

// drawSize draws a size pair (spec 3.3, 4.5): width, then height, and while
// the aspect check fails up to 4 redraws of the pair, each width then
// height. ok is false when the attempt is abandoned.
func (g *gen) drawSize(r *rng) (w, h int, ok bool) {
	k := (g.o.RoomSideMax-g.o.RoomSideMin)/2 + 1
	for range 5 {
		w = g.o.RoomSideMin + 2*r.intn(k) // DRAW: width
		h = g.o.RoomSideMin + 2*r.intn(k) // DRAW: height
		if g.aspectOK(w, h) {
			return w, h, true
		}
	}
	return 0, 0, false
}

func (g *gen) placeRooms() error {
	r := stream(g.o.Seed, phaseRooms)
	switch g.o.Placement {
	case PlacementTiled:
		g.placeTiled(r)
	default:
		g.placeSpread(r)
	}
	if len(g.rooms) == 0 {
		// One forced placement: the first node, in row-major order, where a
		// RoomSideMin room is valid. No draw.
		s := g.o.RoomSideMin
		for j := 0; j < g.ny && len(g.rooms) == 0; j++ {
			for i := range g.nx {
				if x, y := 2*i+1, 2*j+1; g.roomFits(x, y, s, s) {
					g.addRoom(x, y, s, s)
					break
				}
			}
		}
	}
	if len(g.rooms) == 0 {
		return ErrNoSpace
	}
	return nil
}

// placeSpread: random anchors, keep the ones that fit. The attempts are
// ceil(1.6 * usable_area * room_density / 100 / mean_room_area) in integers.
func (g *gen) placeSpread(r *rng) {
	usable := 0
	for _, b := range g.blocked {
		if !b {
			usable++
		}
	}
	m := (g.o.RoomSideMin + g.o.RoomSideMax) / 2
	num := 16 * usable * g.o.RoomDensity
	den := 1000 * m * m
	attempts := (num + den - 1) / den
	for a := 0; a < attempts && len(g.rooms) < roomCap; a++ {
		w, h, ok := g.drawSize(r)
		if !ok {
			continue
		}
		j := r.intn(g.ny) // DRAW: anchor row
		i := r.intn(g.nx) // DRAW: anchor column
		if x, y := 2*i+1, 2*j+1; g.roomFits(x, y, w, h) {
			g.addRoom(x, y, w, h)
		}
	}
}

// placeTiled: every node in row-major order not yet inside a room gets an
// attempt with probability min(100, room_density) percent. A size that does
// not fit shrinks, the width and then the height, in steps of 2 down to
// RoomSideMin, until it fits (the aspect limit is part of fitting: the
// invariant of spec 5.2 holds for shrunk rooms too) or the attempt is
// abandoned.
func (g *gen) placeTiled(r *rng) {
	pct := min(100, g.o.RoomDensity)
	for j := range g.ny {
		for i := range g.nx {
			if len(g.rooms) >= roomCap {
				return
			}
			g.tiledAttempt(r, 2*i+1, 2*j+1, pct)
		}
	}
}

// tiledAttempt is the attempt at the node (x, y), when it is not inside a
// room yet.
func (g *gen) tiledAttempt(r *rng, x, y, pct int) {
	if g.kind[g.idx(x, y)] != KindRock {
		return
	}
	if !r.chance(pct) { // DRAW: attempt chance (none at 100)
		return
	}
	if w, h, ok := g.drawSize(r); ok {
		g.addShrunkRoom(x, y, w, h)
	}
}

// addShrunkRoom adds the room at (x, y), shrinking the width and then the
// height in steps of 2 down to RoomSideMin until it fits; it gives up below
// that.
func (g *gen) addShrunkRoom(x, y, w, h int) {
	for {
		if g.aspectOK(w, h) && g.roomFits(x, y, w, h) {
			g.addRoom(x, y, w, h)
			return
		}
		switch {
		case w > g.o.RoomSideMin:
			w -= 2
		case h > g.o.RoomSideMin:
			h -= 2
		default:
			return
		}
	}
}
