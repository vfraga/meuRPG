package dungeon

// deadEnd: a corridor square with exactly one open 4-neighbor that is not
// protected (spec 3.8).
func (g *gen) deadEnd(i int) bool {
	return g.kind[i] == KindCorridor && !g.protected[i] && g.openDeg(i) == 1
}

// removeDeadEnds is spec 3.8: scan the lattice nodes in row-major order; at
// each dead end, with deadend_removal percent chance (no draw at 100; the
// phase does nothing at 0), cut the corridor back square by square until a
// junction, a door or a protected square, with one chance draw for each
// square that became a dead end; the first failed draw stops the cut. Only
// leaves go, so connectivity never changes.
func (g *gen) removeDeadEnds() {
	pct := g.o.DeadendRemoval
	if pct == 0 {
		return
	}
	r := stream(g.o.Seed, phaseDeadend)
	// left marks a node a cut loop stopped at after a failed draw: the scan must
	// not draw for it again (the cut's own loop handled it, spec 4.5).
	left := make([]bool, g.w*g.h)
	for j := range g.ny {
		for i := range g.nx {
			cur := g.idx(2*i+1, 2*j+1)
			if left[cur] || !g.deadEnd(cur) {
				continue
			}
			if g.deadEndDraw(r, cur, pct) { // DRAW: cut this dead end (none at 100)
				g.cutDeadEnd(r, left, cur, pct)
			}
		}
	}
}

// cutDeadEnd cuts the corridor back from the dead end cur, a node at a time,
// with one chance draw for each node that became a dead end.
func (g *gen) cutDeadEnd(r *rng, left []bool, cur, pct int) {
	for {
		next, more := g.cutNode(cur)
		if !more {
			return
		}
		cur = next
		if !g.deadEnd(cur) {
			return
		}
		if !g.deadEndDraw(r, cur, pct) { // DRAW: keep cutting (none at 100)
			left[cur] = true
			return
		}
	}
}

// cutNode turns the dead end cur and the wall-line square toward its one
// open neighbor into rock and returns the node beyond. It stops (more is
// false) at a door, a door to nowhere that cleanup removes (3.9).
func (g *gen) cutNode(cur int) (next int, more bool) {
	x, y := cur%g.w, cur/g.w
	d := 0
	for ; d < 4; d++ {
		if g.isOpen(g.idx(x+dx[d], y+dy[d])) {
			break
		}
	}
	mi := g.idx(x+dx[d], y+dy[d])
	g.kind[cur] = KindRock
	g.lane[cur] = false
	if g.kind[mi] == KindDoor {
		return 0, false
	}
	g.kind[mi] = KindRock
	return g.idx(x+2*dx[d], y+2*dy[d]), true
}

// deadEndDraw is the chance draw of one dead-end square (none at 100).
func (g *gen) deadEndDraw(r *rng, node, pct int) bool {
	ok := r.chance(pct)
	if g.onDeadEndDraw != nil {
		g.onDeadEndDraw(node, ok)
	}
	return ok
}
