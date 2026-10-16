package puzzle

import (
	"fmt"
	"math/bits"
)

// "Apagar as luzes" (MR-038): an N x N board of lights. Pressing a cell toggles
// it and its four neighbors (up, down, left, right; the edge has no
// neighbor). The puzzle is solved when every light is off.
//
// A board is a uint64 bitmask, bit row*N+col, because 7 x 7 = 49 bits is the
// biggest board (MinLights to MaxLights per side). A set bit is a lit light.

// The board sizes.
const (
	MinLights = 3
	MaxLights = 7
)

// maxKernel bounds the kernel enumeration in LightsMinimum. The kernel of the
// press matrix has dimension 4 for N = 4, 2 for N = 5, and 0 for the other sizes
// from 3 to 7; the bound only guards the loop.
const maxKernel = 16

func checkLights(n int) error {
	if n < MinLights || n > MaxLights {
		return sizeError("the board", n, MinLights, MaxLights)
	}
	return nil
}

// pressMasks returns, for each cell, the lights one press toggles.
func pressMasks(n int) []uint64 {
	masks := make([]uint64, n*n)
	for row := range n {
		for col := range n {
			m := uint64(1) << (row*n + col)
			if row > 0 {
				m |= 1 << ((row-1)*n + col)
			}
			if row < n-1 {
				m |= 1 << ((row+1)*n + col)
			}
			if col > 0 {
				m |= 1 << (row*n + col - 1)
			}
			if col < n-1 {
				m |= 1 << (row*n + col + 1)
			}
			masks[row*n+col] = m
		}
	}
	return masks
}

// LightsPress presses the cell of the N x N board and returns the new state and
// the lights that changed (the pressed cell and its neighbors).
func LightsPress(n int, state uint64, row, col int) (next, changed uint64, err error) {
	if err := checkLights(n); err != nil {
		return 0, 0, err
	}
	if row < 0 || row >= n || col < 0 || col >= n {
		return 0, 0, fmt.Errorf("%w: cell (%d, %d) is not on a %d x %d board", ErrMove, row, col, n, n)
	}
	changed = pressMasks(n)[row*n+col]
	return state ^ changed, changed, nil
}

// LightsSolved says whether every light is off.
func LightsSolved(state uint64) bool { return state == 0 }

// minStartPresses is how far from solved a drawn start must be, in presses, as the
// pillars' start is in turns: a board one or two presses from solved is no puzzle.
const minStartPresses = 3

// LightsStart draws a start for the board from the seed: the lights left by a
// random set of presses made on a solved board, so it always has a solution. A
// start that is already solved (the presses cancel out) or fewer than three presses
// from solved is refused and drawn again from the same generator, so the same seed
// always gives the same, never solved, start. For N = 4 and 5 the press matrix is
// singular, so some non-empty sets of presses do cancel out; for the other sizes
// none does. If sixty-four draws in a row are too easy (it does not happen in
// practice), the first start that is not solved is the answer.
func LightsStart(n int, seed uint64) (uint64, error) {
	if err := checkLights(n); err != nil {
		return 0, err
	}
	masks := pressMasks(n)
	r := newRNG(seed)
	var fallback uint64
	for range 64 {
		var state uint64
		for c := range n * n {
			if r.next()&1 == 1 {
				state ^= masks[c]
			}
		}
		if LightsSolved(state) {
			continue
		}
		if fallback == 0 {
			fallback = state
		}
		if sol, ok, err := LightsMinimum(n, state); err == nil && ok && sol.Presses >= minStartPresses {
			return state, nil
		}
	}
	if fallback != 0 {
		return fallback, nil
	}
	// Sixty-four empty draws in a row will not happen; a press in the middle is
	// never empty, so the answer is still a solvable, unsolved board.
	return masks[(n/2)*n+n/2], nil
}

// LightsSolution is the fewest presses that put out every light.
type LightsSolution struct {
	// Presses is how many.
	Presses int
	// Cells are the cells to press, one bit each (bit row*N+col).
	Cells uint64
}

// LightsMinimum finds the fewest presses that solve the board, over GF(2): a
// press is a bit, the board a vector, and "press the cells x" gives the lights
// A x, with A the (symmetric) matrix whose column c is the press mask of cell c.
// Gaussian elimination solves A x = state. When A is singular (N = 4 and 5)
// there are several solutions, one for each element of the kernel added to a
// particular one, and the kernel is small enough (at most 2^4 elements for the
// sizes allowed) to look at all of them for the one with fewest presses. ok is
// false when the board has no solution at all.
func LightsMinimum(n int, state uint64) (sol LightsSolution, ok bool, err error) {
	if err := checkLights(n); err != nil {
		return LightsSolution{}, false, err
	}
	m := n * n
	rows := lightsEquations(n, state)
	pivotOf, rank := eliminate(rows, m)
	if inconsistent(rows, rank, m) {
		return LightsSolution{}, false, nil // 0 = 1: no solution
	}
	particular, kernel := solutionSpace(rows, pivotOf, m)
	if len(kernel) > maxKernel {
		return LightsSolution{}, false, fmt.Errorf("puzzle: a kernel of %d vectors is more than expected", len(kernel))
	}
	best := fewestPresses(particular, kernel)
	return LightsSolution{Presses: bits.OnesCount64(best), Cells: best}, true, nil
}

// lightsEquations is the system to solve, one row per light: bit c is set when
// pressing c toggles the light (the matrix is symmetric, so that is bit r of
// masks[c] and bit c of masks[r]), and bit m (N x N) is the right-hand side,
// the light's own state.
func lightsEquations(n int, state uint64) []uint64 {
	m := n * n
	masks := pressMasks(n)
	rows := make([]uint64, m)
	for r := range m {
		rows[r] = masks[r] | (state>>r&1)<<m
	}
	return rows
}

// eliminate runs Gauss-Jordan elimination over GF(2) on the m x m system in
// place. pivotOf[col] is the row whose leading column is col, or -1 for a
// free column; rank is how many pivot rows there are, the first ones.
func eliminate(rows []uint64, m int) (pivotOf []int, rank int) {
	pivotOf = make([]int, m)
	for i := range pivotOf {
		pivotOf[i] = -1
	}
	for col := range m {
		sel := pivotRow(rows, rank, col)
		if sel < 0 {
			continue // a free column
		}
		rows[rank], rows[sel] = rows[sel], rows[rank]
		for r := range rows {
			if r != rank && rows[r]>>col&1 == 1 {
				rows[r] ^= rows[rank]
			}
		}
		pivotOf[col] = rank
		rank++
	}
	return pivotOf, rank
}

// pivotRow is the first row from `from` on with the column set, or -1.
func pivotRow(rows []uint64, from, col int) int {
	for r := from; r < len(rows); r++ {
		if rows[r]>>col&1 == 1 {
			return r
		}
	}
	return -1
}

// inconsistent says whether a row past the pivots reads 0 = 1.
func inconsistent(rows []uint64, rank, m int) bool {
	for r := rank; r < len(rows); r++ {
		if rows[r]>>m&1 == 1 {
			return true
		}
	}
	return false
}

// solutionSpace is a particular solution (the free columns are 0, each pivot
// column takes the right-hand side of its row) and a basis of the kernel, one
// vector per free column, in column order.
func solutionSpace(rows []uint64, pivotOf []int, m int) (particular uint64, kernel []uint64) {
	for col := range m {
		if pivotOf[col] >= 0 {
			particular |= (rows[pivotOf[col]] >> m & 1) << col
			continue
		}
		kernel = append(kernel, kernelVector(rows, pivotOf, m, col))
	}
	return particular, kernel
}

// kernelVector presses the free column, and every pivot column whose row has
// the free column in it.
func kernelVector(rows []uint64, pivotOf []int, m, free int) uint64 {
	v := uint64(1) << free
	for p := range m {
		if pivotOf[p] >= 0 && rows[pivotOf[p]]>>free&1 == 1 {
			v |= 1 << p
		}
	}
	return v
}

// fewestPresses is the solution with the fewest presses among the particular
// one plus each combination of the kernel, the first one found on a tie.
func fewestPresses(particular uint64, kernel []uint64) uint64 {
	best := particular
	for combo := 1; combo < 1<<len(kernel); combo++ {
		x := particular
		for i, v := range kernel {
			if combo>>i&1 == 1 {
				x ^= v
			}
		}
		if bits.OnesCount64(x) < bits.OnesCount64(best) {
			best = x
		}
	}
	return best
}

// LightsToBools lists the board row by row, true for a lit light.
func LightsToBools(n int, state uint64) []bool {
	out := make([]bool, n*n)
	for i := range out {
		out[i] = state>>i&1 == 1
	}
	return out
}

// LightsFromBools is the inverse of LightsToBools; the slice must have N x N cells.
func LightsFromBools(n int, lit []bool) (uint64, error) {
	if err := checkLights(n); err != nil {
		return 0, err
	}
	if len(lit) != n*n {
		return 0, fmt.Errorf("%w: %d lights for a %d x %d board", ErrMove, len(lit), n, n)
	}
	var state uint64
	for i, on := range lit {
		if on {
			state |= 1 << i
		}
	}
	return state, nil
}
