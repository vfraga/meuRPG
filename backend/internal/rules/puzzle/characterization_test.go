package puzzle

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var updatePuzzleSweep = flag.Bool("update", false, "rewrite testdata/*.sha256 from the current code")

// checkPuzzleSweep compares the digest of what write prints with the one in
// testdata/<name>.sha256 (the full text is far too long to keep).
func checkPuzzleSweep(t *testing.T, name string, write func(w io.Writer)) {
	t.Helper()
	w := &puzzleDigest{h: sha256.New()}
	write(w)
	got := fmt.Sprintf("%s %d\n", hex.EncodeToString(w.h.Sum(nil)), w.lines)
	path := filepath.Join("testdata", name+".sha256")
	if *updatePuzzleSweep {
		if err := os.MkdirAll("testdata", 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update to create it)", err)
	}
	if got != string(want) {
		t.Errorf("the %s sweep changed: digest %q, want %q", name, strings.TrimSpace(got), strings.TrimSpace(string(want)))
	}
}

type puzzleDigest struct {
	h     hash.Hash
	lines int
}

func (d *puzzleDigest) Write(p []byte) (int, error) {
	d.lines += strings.Count(string(p), "\n")
	return d.h.Write(p)
}

// TestLightsMinimumSweepCharacterization freezes LightsMinimum, the very cells
// it picks among the solutions with as few presses (the order the kernel is
// walked decides which), for every board of 3 x 3 and seeded boards
// of the larger sizes, and LightsStart for a hundred seeds of each size.
func TestLightsMinimumSweepCharacterization(t *testing.T) {
	t.Parallel()
	const sampled = 400
	checkPuzzleSweep(t, "lights_sweep", func(w io.Writer) {
		for n := MinLights - 1; n <= MaxLights+1; n++ {
			states := []uint64{0, 1, 1<<(n*n) - 1}
			switch {
			case n == MinLights:
				states = states[:0]
				for s := range uint64(1) << (n * n) {
					states = append(states, s)
				}
			case n > MinLights:
				r := newRNG(uint64(n))
				for range sampled {
					states = append(states, r.next()&(1<<(n*n)-1))
				}
			}
			for _, s := range states {
				sol, ok, err := LightsMinimum(n, s)
				fmt.Fprintf(w, "n=%d %x %+v %v %v\n", n, s, sol, ok, err)
			}
			for seed := range uint64(100) {
				start, err := LightsStart(n, seed)
				fmt.Fprintf(w, "n=%d seed=%d start=%x %v\n", n, seed, start, err)
			}
		}
	})
}

// pillarsCases are the shapes of the pillars sweep: no links, links one way,
// links both ways, a chain, the largest, and links that leave some targets out
// of reach.
var pillarsCases = []Pillars{
	{Count: 3, Glyphs: 3},
	{Count: 3, Glyphs: 6, Links: [][]int{{1}}},
	{Count: 4, Glyphs: 4, Links: [][]int{{1, 3}, {2}, nil, {0}}},
	{Count: 5, Glyphs: 5, Links: [][]int{{1}, {2}, {3}, {4}}},
	{Count: 6, Glyphs: 6},
	{Count: 3, Glyphs: 4, Links: [][]int{{1}, {0}}},
	{Count: 4, Glyphs: 3, Links: [][]int{{1, 2, 3}, {0, 2, 3}, {0, 1, 3}, {0, 1, 2}}},
}

// TestPillarsSweepCharacterization freezes Minimum, the very path the search
// finds, for seeded states and targets of each shape, and Start for forty
// seeds of each.
func TestPillarsSweepCharacterization(t *testing.T) {
	t.Parallel()
	const pairs, seeds = 30, 40
	checkPuzzleSweep(t, "pillars_sweep", func(w io.Writer) {
		for ci, p := range pillarsCases {
			r := newRNG(uint64(ci) + 1)
			draw := func() []int {
				s := make([]int, p.Count)
				for i := range s {
					s[i] = r.intn(p.Glyphs)
				}
				return s
			}
			for range pairs {
				state, target := draw(), draw()
				moves, path, ok, err := p.Minimum(state, target)
				fmt.Fprintf(w, "p%d %v>%v moves=%d path=%v ok=%v err=%v\n", ci, state, target, moves, path, ok, err)
				moves, path, ok, err = p.Minimum(target, target)
				fmt.Fprintf(w, "p%d %v itself moves=%d path=%v ok=%v err=%v\n", ci, target, moves, path, ok, err)
			}
			target := draw()
			for seed := range uint64(seeds) {
				start, err := p.Start(target, seed)
				fmt.Fprintf(w, "p%d target=%v seed=%d start=%v err=%v\n", ci, target, seed, start, err)
			}
		}
	})
}

// TestPillarsRefusalsCharacterization: Minimum and Start check the shape
// first, then the state, then the target, each with its own message.
func TestPillarsRefusalsCharacterization(t *testing.T) {
	t.Parallel()
	good := Pillars{Count: 3, Glyphs: 3}
	ok3 := []int{0, 1, 2}
	for _, c := range []struct {
		name          string
		p             Pillars
		state, target []int
		minimum       string
		start         string
	}{
		{
			"too few pillars",
			Pillars{Count: 2, Glyphs: 3},
			ok3, ok3,
			"puzzle: the size is out of range: the pillars is 2, want 3 to 6", "puzzle: the size is out of range: the pillars is 2, want 3 to 6",
		},
		{
			"too many glyphs",
			Pillars{Count: 3, Glyphs: 7},
			ok3, ok3,
			"puzzle: the size is out of range: the symbols is 7, want 3 to 6", "puzzle: the size is out of range: the symbols is 7, want 3 to 6",
		},
		{
			"a bad link",
			Pillars{Count: 3, Glyphs: 3, Links: [][]int{{0}}},
			ok3, ok3,
			"puzzle: a link is not valid: pillar 0 turns pillar 0", "puzzle: a link is not valid: pillar 0 turns pillar 0",
		},
		{
			"a short state", good,
			[]int{0, 1},
			[]int{0, 1, 3},
			"puzzle: a symbol is out of range: 2 positions for 3 pillars", "puzzle: a symbol is out of range: position 3 of 3 symbols",
		},
		{
			"a bad target", good, ok3,
			[]int{0, 1, 3},
			"puzzle: a symbol is out of range: position 3 of 3 symbols", "puzzle: a symbol is out of range: position 3 of 3 symbols",
		},
	} {
		moves, path, ok, err := c.p.Minimum(c.state, c.target)
		if err == nil || err.Error() != c.minimum || moves != 0 || path != nil || ok {
			t.Errorf("%s: Minimum = %d %v %v %v, want the refusal %q", c.name, moves, path, ok, err, c.minimum)
		}
		start, err := c.p.Start(c.target, 1)
		if err == nil || err.Error() != c.start || start != nil {
			t.Errorf("%s: Start = %v %v, want the refusal %q", c.name, start, err, c.start)
		}
	}
}

// cipherKeys are every shift from one below the range to one above it and
// keywords of each kind: too short, accented, with repeats, the alphabet
// itself, a keyword that changes nothing, the full alphabet backwards, one
// with no letter, and a shift and a keyword together.
var cipherKeys = func() []CipherKey {
	var out []CipherKey
	for s := MinShift - 2; s <= MaxShift+2; s++ {
		out = append(out, CipherKey{Shift: s})
	}
	for _, k := range []string{"", "ab", "abc", "LUA", "Lua", "ÁÉÍ óú", "banana", "ccc ddd eee", "abcdefghijklmnopqrstuvwxyz", "zyxwvutsrqponmlkjihgfedcba", "123 !?", "Ação mágica", "abcd"} {
		out = append(out, CipherKey{Keyword: k})
	}
	return append(out, CipherKey{Shift: 3, Keyword: "lua"})
}()

// TestCipherSweepCharacterization freezes cipherAlphabet through every entry
// point: the alphabet or the refusal, and Encipher, Decipher and
// ValidateCipher of messages with accents, digits, no letter and too long.
func TestCipherSweepCharacterization(t *testing.T) {
	t.Parallel()
	messages := []string{"O tesouro está sob o altar", "  42 !  ", "Ça va? Último 3", "", strings.Repeat("a", MaxCipherMessage+1)}
	checkPuzzleSweep(t, "cipher_sweep", func(w io.Writer) {
		for _, k := range cipherKeys {
			abc, err := cipherAlphabet(k)
			fmt.Fprintf(w, "%+v abc=%s err=%v\n", k, abc[:], err)
			for _, m := range messages {
				enc, encErr := Encipher(m, k)
				dec, decErr := Decipher(enc, k)
				fmt.Fprintf(w, " %q valid=%v enc=%q %v dec=%q %v\n", m, ValidateCipher(m, k), enc, encErr, dec, decErr)
			}
		}
	})
}
