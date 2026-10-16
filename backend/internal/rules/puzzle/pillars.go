package puzzle

import (
	"fmt"
	"slices"
)

// The turning symbols (MR-038): 3 to 6 pillars, each showing one of 3 to 6 of
// our own glyphs. A move turns a pillar one glyph up or down, wrapping around;
// when the pillar has links, the pillars it lists turn the same way with it. The
// puzzle is solved when the pillars match the target, the "mural" the master
// painted.

// The numbers of pillars and glyphs.
const (
	MinPillars = 3
	MaxPillars = 6
	MinGlyphs  = 3
	MaxGlyphs  = 6
)

// glyphs are the six glyphs of the table: ours, named in Portuguese. A puzzle
// with fewer glyphs uses the first ones.
var glyphs = []Symbol{
	{"raven", "Corvo"},
	{"wolf", "Lobo"},
	{"serpent", "Serpente"},
	{"owl", "Coruja"},
	{"deer", "Cervo"},
	{"fish", "Peixe"},
}

// Glyphs returns the first n glyphs (nil when n is out of range): the faces of a
// puzzle with n symbols, in the order a pillar turns through them.
func Glyphs(n int) []Symbol {
	if n < MinGlyphs || n > MaxGlyphs {
		return nil
	}
	return glyphs[:n]
}

// Pillars is the configuration of a turning-symbols puzzle.
type Pillars struct {
	// Count is the number of pillars and Glyphs the number of symbols each shows.
	Count, Glyphs int
	// Links[i] lists the other pillars that turn when pillar i turns. It may be
	// shorter than Count (or nil): a pillar past the end has no links.
	Links [][]int
}

// Validate checks the numbers and the links.
func (p Pillars) Validate() error {
	if p.Count < MinPillars || p.Count > MaxPillars {
		return sizeError("the pillars", p.Count, MinPillars, MaxPillars)
	}
	if p.Glyphs < MinGlyphs || p.Glyphs > MaxGlyphs {
		return sizeError("the symbols", p.Glyphs, MinGlyphs, MaxGlyphs)
	}
	if len(p.Links) > p.Count {
		return fmt.Errorf("%w: links for %d pillars, there are %d", ErrLinks, len(p.Links), p.Count)
	}
	for i, l := range p.Links {
		seen := map[int]bool{}
		for _, to := range l {
			if to < 0 || to >= p.Count || to == i || seen[to] {
				return fmt.Errorf("%w: pillar %d turns pillar %d", ErrLinks, i, to)
			}
			seen[to] = true
		}
	}
	return nil
}

// CheckState says the positions are one per pillar, each inside the glyphs.
func (p Pillars) CheckState(state []int) error {
	if len(state) != p.Count {
		return fmt.Errorf("%w: %d positions for %d pillars", ErrSymbols, len(state), p.Count)
	}
	for _, v := range state {
		if v < 0 || v >= p.Glyphs {
			return fmt.Errorf("%w: position %d of %d symbols", ErrSymbols, v, p.Glyphs)
		}
	}
	return nil
}

// turned lists the pillars one move of the pillar turns: itself and its links.
func (p Pillars) turned(pillar int) []int {
	out := []int{pillar}
	if pillar < len(p.Links) {
		out = append(out, p.Links[pillar]...)
	}
	return out
}

// Turn turns the pillar by delta (+1 or -1) and its linked pillars with it by
// the same delta, so that a turn down undoes a turn up. It returns the new
// positions and the pillars that turned; the argument is not changed.
func (p Pillars) Turn(state []int, pillar, delta int) (next, changed []int, err error) {
	if pillar < 0 || pillar >= p.Count || (delta != 1 && delta != -1) {
		return nil, nil, fmt.Errorf("%w: pillar %d by %d", ErrMove, pillar, delta)
	}
	if err := p.CheckState(state); err != nil {
		return nil, nil, err
	}
	next = slices.Clone(state)
	changed = p.turned(pillar)
	for _, c := range changed {
		next[c] = mod(next[c]+delta, p.Glyphs)
	}
	return next, changed, nil
}

// PillarStep is one move: a pillar turned by one.
type PillarStep struct {
	Pillar int
	Delta  int
}

// encode turns the positions into a number in base Glyphs (pillar 0 is the lowest
// digit), the index of a state in the search (Minimum decodes it inline).
func (p Pillars) encode(state []int) int {
	n := 0
	for i := p.Count - 1; i >= 0; i-- {
		n = n*p.Glyphs + state[i]
	}
	return n
}

// Minimum is the fewest moves from the state to the target, and one way to make
// them, found by a breadth-first search over every state (at most 6^6 = 46 656)
// with the 2 x Count moves. ok is false when the target cannot be reached (links
// can make that so). A state that is the target takes 0 moves.
func (p Pillars) Minimum(state, target []int) (moves int, path []PillarStep, ok bool, err error) {
	if err := p.checkSearch(state, target); err != nil {
		return 0, nil, false, err
	}
	from, to := p.encode(state), p.encode(target)
	parent, step := p.search(from, to)
	if parent[to] < 0 {
		return 0, nil, false, nil
	}
	for s := to; s != from; s = int(parent[s]) {
		path = append(path, step[s])
	}
	slices.Reverse(path)
	return len(path), path, true, nil
}

// checkSearch checks the shape, then the state, then the target.
func (p Pillars) checkSearch(state, target []int) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if err := p.CheckState(state); err != nil {
		return err
	}
	return p.CheckState(target)
}

// states is how many states the pillars have: Glyphs to the power Count.
func (p Pillars) states() int {
	total := 1
	for range p.Count {
		total *= p.Glyphs
	}
	return total
}

// search is the breadth-first search from one state until it reaches the
// other or runs out. parent[s] is the state before s, step[s] the move that
// led to s; -1: not seen.
func (p Pillars) search(from, to int) (parent []int32, step []PillarStep) {
	total := p.states()
	parent = make([]int32, total)
	step = make([]PillarStep, total)
	for i := range parent {
		parent[i] = -1
	}
	parent[from] = int32(from) //nolint:gosec // from is below 6^6
	queue := []int{from}
	for len(queue) > 0 && parent[to] < 0 {
		cur := queue[0]
		queue = p.expand(queue[1:], cur, parent, step)
	}
	return parent, step
}

// expand queues every state one move from cur that was not seen yet, pillar
// by pillar, up before down.
func (p Pillars) expand(queue []int, cur int, parent []int32, step []PillarStep) []int {
	pos := p.decode(cur)
	for pillar := range p.Count {
		for _, delta := range [2]int{1, -1} {
			nextPos := pos // an array: a copy
			for _, c := range p.turned(pillar) {
				nextPos[c] = mod(nextPos[c]+delta, p.Glyphs)
			}
			n := p.encode(nextPos[:p.Count])
			if parent[n] >= 0 {
				continue
			}
			parent[n] = int32(cur) //nolint:gosec // cur is below 6^6
			step[n] = PillarStep{Pillar: pillar, Delta: delta}
			queue = append(queue, n)
		}
	}
	return queue
}

// decode is encode backwards: the positions of the state numbered n.
func (p Pillars) decode(n int) [MaxPillars]int {
	var pos [MaxPillars]int
	for i := range p.Count {
		pos[i] = n % p.Glyphs
		n /= p.Glyphs
	}
	return pos
}

// Start draws a start from the target: the target after a seeded run of moves,
// so it can always be solved. A start that is the target is drawn again, and so
// is one fewer than three moves from it; the same seed always gives the same start.
// If sixty-four draws are all too easy (not in practice), the first one that is not
// the target is the answer, so three moves is the rule, not a guarantee. The
// moves are the player's own, so the start is as far from the target as a few
// turns can take it.
func (p Pillars) Start(target []int, seed uint64) ([]int, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	if err := p.CheckState(target); err != nil {
		return nil, err
	}
	r := newRNG(seed)
	var fallback []int
	for range startDraws {
		state := p.scramble(target, r)
		if slices.Equal(state, target) {
			continue
		}
		if fallback == nil {
			fallback = state
		}
		// A start a turn or two from the mural is no puzzle: ask for three.
		if moves, _, ok, err := p.Minimum(state, target); err == nil && ok && moves >= minStartTurns {
			return state, nil
		}
	}
	if fallback != nil {
		return fallback, nil
	}
	// Every draw came back to the target (it cannot happen in practice): turn the
	// first pillar once, which is never the target.
	state, _, err := p.Turn(target, 0, 1)
	return state, err
}

// The draws of Start.
const (
	// startDraws is how many starts are drawn before the fallback is taken.
	startDraws = 64
	// minStartTurns is how far from the target a drawn start must be.
	minStartTurns = 3
	// minScramble and scrambleSpread bound the turns of one draw: 6 to 15.
	minScramble    = 6
	scrambleSpread = 10
)

// scramble is the target after a seeded run of turns, each of a random pillar
// up or down.
func (p Pillars) scramble(target []int, r *rng) []int {
	state := slices.Clone(target)
	steps := minScramble + r.intn(scrambleSpread)
	for range steps {
		pillar := r.intn(p.Count)
		delta := 1
		if r.next()&1 == 1 {
			delta = -1
		}
		state, _, _ = p.Turn(state, pillar, delta) // the arguments are valid
	}
	return state
}

// PillarsSolved says whether the pillars match the target.
func PillarsSolved(state, target []int) bool { return slices.Equal(state, target) }
