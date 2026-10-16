package dungeon

import "slices"

// components labels the connected components of open squares (4-neighbor
// adjacency; a door is a normal open square, spec 3.6.1). Component c is
// order[starts[c]:starts[c+1]]; comp is -1 on rock.
type components struct {
	comp   []int32
	order  []int32
	starts []int // one more than the number of components
}

func (c *components) squares(id int32) []int32 { return c.order[c.starts[id]:c.starts[id+1]] }

func (g *gen) labelComponents() *components {
	n, openSq := g.w*g.h, 0
	for _, k := range g.kind {
		if k != KindRock {
			openSq++
		}
	}
	c := &components{comp: make([]int32, n), order: make([]int32, 0, openSq)}
	for i := range c.comp {
		c.comp[i] = -1
	}
	for i := range n {
		if g.kind[i] != KindRock && c.comp[i] < 0 {
			g.floodComponent(c, i)
		}
	}
	c.starts = append(c.starts, len(c.order))
	return c
}

// floodComponent labels the component of the open square i (breadth-first,
// in the order of c.order) with the next ID.
func (g *gen) floodComponent(c *components, i int) {
	id := int32(len(c.starts)) //nolint:gosec // G115: at most one component per square
	c.starts = append(c.starts, len(c.order))
	c.comp[i] = id
	c.order = append(c.order, int32(i)) //nolint:gosec // G115: a square index of a grid under 80 000
	for head := c.starts[id]; head < len(c.order); head++ {
		u := int(c.order[head])
		x, y := u%g.w, u/g.w
		for d := range 4 {
			nx, ny := x+dx[d], y+dy[d]
			if !g.inside(nx, ny) {
				continue
			}
			if v := g.idx(nx, ny); g.kind[v] != KindRock && c.comp[v] < 0 {
				c.comp[v] = id
				c.order = append(c.order, int32(v)) //nolint:gosec // G115: a square index of a grid under 80 000
			}
		}
	}
}

// link is a candidate door that would join an orphan group to the rest.
type link struct {
	room *room
	side Side
	t    threshold
	// fromMain: a door on a main-component room into the group (3.6.3 (b)).
	fromMain bool
}

// connect is spec 3.6: drop sealed corridors, link every orphan room group
// to the main component (or discard it), then the optional extra loops.
func (g *gen) connect() {
	r := stream(g.o.Seed, phaseConnect)
	c := g.labelComponents()
	main := c.comp[g.idx(g.rooms[0].centerX, g.rooms[0].centerY)]
	roomsOf := g.roomsByComponent(c)
	g.eraseSealedCorridors(c, roomsOf)
	l := &linker{g: g, r: r, c: c, main: main, roomsOf: roomsOf, mainRooms: roomsOf[main]}
	l.run(g.orphanGroups(c, main))
	g.compact()
	if g.o.ExtraLoops > 0 {
		g.extraLoops(r)
	}
}

// roomsByComponent lists the rooms of each component, in ID order.
func (g *gen) roomsByComponent(c *components) [][]*room {
	roomsOf := make([][]*room, len(c.starts)-1)
	for _, rm := range g.rooms {
		id := c.comp[g.idx(rm.centerX, rm.centerY)]
		roomsOf[id] = append(roomsOf[id], rm)
	}
	return roomsOf
}

// eraseSealedCorridors is 3.6.2: a component with no room is a sealed
// corridor tree: erase it.
func (g *gen) eraseSealedCorridors(c *components, roomsOf [][]*room) {
	for id := range int32(len(roomsOf)) { //nolint:gosec // G115: at most one component per square
		if len(roomsOf[id]) != 0 {
			continue
		}
		for _, sq := range c.squares(id) {
			g.kind[sq] = KindRock
			g.lane[sq] = false
		}
	}
}

// orphanGroups are the components with a room other than the main one, in
// order of their lowest room ID.
func (g *gen) orphanGroups(c *components, main int32) []int32 {
	var queue []int32
	seen := make([]bool, len(c.starts)-1)
	for _, rm := range g.rooms {
		id := c.comp[g.idx(rm.centerX, rm.centerY)]
		if id != main && !seen[id] {
			seen[id] = true
			queue = append(queue, id)
		}
	}
	return queue
}

// linker is the state of 3.6.3: the groups in order of their lowest room ID.
// A group that cannot link yet is deferred to the back of the queue,
// unchanged; when a full pass links nothing, only the front group is
// discarded and another pass runs. Each attempt gathers its candidates again
// (the main component has grown) and shuffles them: one DRAW per attempt.
type linker struct {
	g         *gen
	r         *rng
	c         *components
	main      int32
	roomsOf   [][]*room
	mainRooms []*room
	cands     []link
	discards  int
}

// run makes passes over the queue until every group is linked or discarded.
func (l *linker) run(queue []int32) {
	for pass := 0; len(queue) > 0; pass++ {
		deferred := l.pass(queue, pass)
		if len(deferred) == len(queue) {
			// A fruitless pass: discard only the front group (lowest room ID);
			// its erased squares may open paths for the others.
			id := deferred[0]
			l.g.discardGroup(l.roomsOf[id], l.c.squares(id))
			l.discards++
			deferred = deferred[1:]
		}
		queue = deferred
		l.g.passes = pass + 1
	}
}

// pass tries each group of the queue once and returns the deferred ones.
func (l *linker) pass(queue []int32, pass int) []int32 {
	var deferred []int32
	for _, id := range queue {
		if !l.link(id) {
			deferred = append(deferred, id)
			continue
		}
		if pass > 0 && l.g.onLater != nil {
			l.g.onLater(l.discards)
		}
		l.join(id)
	}
	return deferred
}

// link gathers and shuffles the group's candidates and opens the first that
// works.
func (l *linker) link(id int32) bool {
	l.cands = l.g.gatherLinks(l.cands[:0], l.roomsOf[id], l.mainRooms, l.c, l.main, id)
	shuffle(l.r, l.cands) // DRAW: the candidate list, once per attempt
	for _, cd := range l.cands {
		if l.g.tryLink(cd, l.c, l.main) {
			return true
		}
	}
	return false
}

// join makes a linked group part of the main component.
func (l *linker) join(id int32) {
	for _, sq := range l.c.squares(id) {
		l.c.comp[sq] = l.main
	}
	l.mainRooms = append(l.mainRooms, l.roomsOf[id]...)
	slices.SortFunc(l.mainRooms, func(a, b *room) int { return a.id - b.id })
}

// gatherLinks lists the candidate links of a group (spec 3.6.3 step 1), in the
// fixed order: first every (a), then every (b); within each, by room ID, side
// (north, east, south, west) and position. (a) are the thresholds of the
// group's rooms whose far side is a main-component open square or a free
// node; (b) are the thresholds of the main component's rooms whose far side is
// an open square of the group.
func (g *gen) gatherLinks(out []link, rooms, mainRooms []*room, c *components, main, group int32) []link {
	for _, rm := range rooms {
		out = g.roomLinks(out, rm, c, main, false)
	}
	for _, rm := range mainRooms {
		out = g.roomLinks(out, rm, c, group, true)
	}
	return out
}

// roomLinks appends the candidate links of one room, by side and position.
// From a group's room (fromMain false), the far side is a free node or an
// open square of target, the main component; from a main room, an open
// square of target, the group.
func (g *gen) roomLinks(out []link, rm *room, c *components, target int32, fromMain bool) []link {
	for side := North; side <= West; side++ {
		for p := range rm.sideLen(side) {
			t, ok := g.candidateOK(rm, side, p)
			if ok && g.linkReaches(t, c, target, fromMain) {
				out = append(out, link{room: rm, side: side, t: t, fromMain: fromMain})
			}
		}
	}
	return out
}

// linkReaches reports whether the square beyond a threshold is an open
// square of the target component, or, from a group's room, a free node.
func (g *gen) linkReaches(t threshold, c *components, target int32, fromMain bool) bool {
	bi := g.idx(t.beyondX, t.beyondY)
	if g.kind[bi] != KindRock {
		return c.comp[bi] == target
	}
	return !fromMain && g.freeNode(t.beyondX, t.beyondY)
}

// tryLink opens one candidate if it works: directly onto main-component
// floor, or through the shortest path of free nodes (breadth-first, steps of
// 2, directions north, east, south, west, so ties are deterministic).
func (g *gen) tryLink(l link, c *components, main int32) bool {
	bi := g.idx(l.t.beyondX, l.t.beyondY)
	if g.kind[bi] != KindRock {
		g.epoch++ // the group joins the main component
		g.drawKind(g.openDoor(l.room, l.side, l.t, false))
		return true
	}
	path, ok := g.shortestPath(bi, c, main)
	if !ok {
		return false
	}
	g.epoch++
	for _, sq := range path {
		g.kind[sq] = KindCorridor
		c.comp[sq] = main
	}
	g.drawKind(g.openDoor(l.room, l.side, l.t, false))
	return true
}

// shortestPath finds the squares to carve (nodes and the wall-line squares
// between them, from the start node on) to reach a main-component corridor.
func (g *gen) shortestPath(start int, c *components, main int32) ([]int, bool) {
	g.ensureSearchScratch()
	if g.dead[start] == g.epoch+1 {
		return nil, false
	}
	g.curSt++
	cur := g.curSt
	g.queue = append(g.queue[:0], int32(start)) //nolint:gosec // G115: a square index of a grid under 80 000
	g.stamp[start] = cur
	g.parent[start] = -1
	for head := 0; head < len(g.queue); head++ {
		u := int(g.queue[head])
		x, y := u%g.w, u/g.w
		for d := range 4 {
			if path, ok := g.pathStep(u, x, y, d, c, main, cur); ok {
				return path, true
			}
		}
	}
	// Nothing in this free region touches the main component: remember it, until
	// the main component grows. (dead holds epoch+1 so that 0 means "unknown".)
	for _, v := range g.queue {
		g.dead[v] = g.epoch + 1
	}
	return nil, false
}

// ensureSearchScratch allocates the breadth-first scratch on first use.
func (g *gen) ensureSearchScratch() {
	if g.stamp != nil {
		return
	}
	n := g.w * g.h
	g.stamp = make([]int32, n)
	g.parent = make([]int32, n)
	g.dead = make([]int32, n)
}

// pathStep follows direction d from the node u at (x, y) across a rock
// wall-line square: a main-component corridor beyond ends the search with
// the path; a free node not yet seen joins the queue.
func (g *gen) pathStep(u, x, y, d int, c *components, main, cur int32) ([]int, bool) {
	mi := g.idx(x+dx[d], y+dy[d])
	if !g.inside(x+2*dx[d], y+2*dy[d]) || g.kind[mi] != KindRock || g.blocked[mi] {
		return nil, false
	}
	vx, vy := x+2*dx[d], y+2*dy[d]
	v := g.idx(vx, vy)
	if g.kind[v] == KindCorridor && c.comp[v] == main {
		return g.pathBack(u, mi), true
	}
	if g.freeNode(vx, vy) && g.stamp[v] != cur && g.dead[v] != g.epoch+1 {
		g.stamp[v] = cur
		g.parent[v] = int32(u)              //nolint:gosec // G115: a square index of a grid under 80 000
		g.queue = append(g.queue, int32(v)) //nolint:gosec // G115: a square index of a grid under 80 000
	}
	return nil, false
}

// pathBack is the path to carve: the last wall-line square mi, then each
// node back to the start with the wall-line square between it and its
// parent.
func (g *gen) pathBack(u, mi int) []int {
	path := []int{mi}
	for q := u; q >= 0; q = int(g.parent[q]) {
		path = append(path, q)
		if p := int(g.parent[q]); p >= 0 {
			path = append(path, (p+q)/2)
		}
	}
	return path
}

// discardGroup erases a group that could not be linked: its rooms' floors,
// rings and doors (and the corridors of its component) become rock; the room
// IDs stay unused.
func (g *gen) discardGroup(rooms []*room, squares []int32) {
	g.epoch++ // erased squares are free nodes again
	for _, sq := range squares {
		g.kind[sq] = KindRock
		g.rid[sq] = 0
		g.lane[sq] = false
		g.doorKind[sq] = DoorArchway
		g.doorTrap[sq] = false
	}
	for _, rm := range rooms {
		rm.discarded = true
		g.roomByID[rm.id] = nil
		g.discarded = append(g.discarded, rm.id)
	}
}

// loopCand is a candidate of extraLoops: a wall-line square between two
// corridors (room nil), or a room's threshold onto a corridor (sq is its
// door square).
type loopCand struct {
	sq   int
	room *room
	side Side
	pos  int
}

// extraLoops is spec 3.6.4: open some of the wall-line squares that separate
// two corridors, or a corridor and a room's threshold, to give the level
// cycles. The candidates are listed in a fixed order (corridor pairs in
// row-major order, then room thresholds by room, side and position), shuffled
// once, and each is opened with probability extra_loops percent if it still
// is valid when its turn comes (a door keeps its 4-square spacing).
func (g *gen) extraLoops(r *rng) {
	cs := g.thresholdLoops(g.corridorPairLoops(nil))
	// row-major order of the square (stable: a shared wall's two thresholds keep room order)
	slices.SortStableFunc(cs, func(a, b loopCand) int { return a.sq - b.sq })
	shuffle(r, cs) // DRAW: the loop candidates, once
	for _, cd := range cs {
		if !g.loopStillValid(cd) {
			continue
		}
		if !r.chance(g.o.ExtraLoops) { // DRAW: open this one (none at 100)
			continue
		}
		g.openLoop(cd)
	}
}

// corridorPairLoops appends, in row-major order, the free rock wall-line
// squares with a corridor on both sides.
func (g *gen) corridorPairLoops(cs []loopCand) []loopCand {
	for y := 1; y < g.h-1; y++ {
		for x := 1; x < g.w-1; x++ {
			i := g.idx(x, y)
			if g.kind[i] != KindRock || g.blocked[i] {
				continue
			}
			if a, b, ok := g.midpointEnds(x, y); ok && g.kind[a] == KindCorridor && g.kind[b] == KindCorridor {
				cs = append(cs, loopCand{sq: i})
			}
		}
	}
	return cs
}

// midpointEnds are the two nodes a wall-line square (one coordinate even,
// the other odd) lies between; ok is false for any other square.
func (g *gen) midpointEnds(x, y int) (a, b int, ok bool) {
	switch {
	case x%2 == 0 && y%2 == 1:
		return g.idx(x-1, y), g.idx(x+1, y), true
	case x%2 == 1 && y%2 == 0:
		return g.idx(x, y-1), g.idx(x, y+1), true
	}
	return 0, 0, false
}

// thresholdLoops appends the room thresholds that open onto a corridor, by
// room, side and position.
func (g *gen) thresholdLoops(cs []loopCand) []loopCand {
	for _, rm := range g.rooms {
		for side := North; side <= West; side++ {
			for p := 0; p < rm.sideLen(side); p++ {
				if t, ok := g.candidateOK(rm, side, p); ok && g.kind[g.idx(t.beyondX, t.beyondY)] == KindCorridor {
					cs = append(cs, loopCand{sq: g.idx(t.doorX, t.doorY), room: rm, side: side, pos: p})
				}
			}
		}
	}
	return cs
}

// loopStillValid reports whether a candidate can still open when its turn
// comes.
func (g *gen) loopStillValid(cd loopCand) bool {
	if cd.room == nil {
		return g.kind[cd.sq] == KindRock
	}
	_, ok := g.candidateOK(cd.room, cd.side, cd.pos)
	return ok
}

// openLoop carves a corridor-pair square, or opens a threshold's door.
func (g *gen) openLoop(cd loopCand) {
	if cd.room == nil {
		g.kind[cd.sq] = KindCorridor
		return
	}
	t, _ := g.candidateOK(cd.room, cd.side, cd.pos)
	g.drawKind(g.openDoor(cd.room, cd.side, t, false))
}
