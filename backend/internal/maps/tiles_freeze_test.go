package maps

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"strconv"
	"strings"
	"testing"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// Characterization of what a player's tile carries (RN-10, MR-036): the bytes of
// every tile of every kind of viewer, over PNG and JPEG images, shrunk and not,
// are frozen as digests, written on the code before it was split into phases.
// A refactor of renderTile, blackenBlocks, decodeWorkingCopy or buildTiles must
// not move a single byte of a tile: the digests are the proof, and tiles_test.go
// (TestRN10_TileOracle) is the proof that the bytes are the right ones.

// digestOf is a short fingerprint of some bytes.
func digestOf(parts ...[]byte) string {
	h := sha256.New()
	for _, p := range parts {
		_, _ = fmt.Fprintf(h, "%d:", len(p))
		h.Write(p)
	}
	return hex.EncodeToString(h.Sum(nil))[:20]
}

// tileViewer is a way of knowing the squares of a map: a name and which squares.
type tileViewer struct {
	name  string
	known func(g grid.Grid) func(c, r int) bool
}

var tileViewers = []tileViewer{
	{"everything", func(grid.Grid) func(c, r int) bool { return func(int, int) bool { return true } }},
	{"nothing", func(grid.Grid) func(c, r int) bool { return func(int, int) bool { return false } }},
	{"one square", func(grid.Grid) func(c, r int) bool { return func(c, r int) bool { return c == 5 && r == 5 } }},
	{"left third", func(g grid.Grid) func(c, r int) bool { return func(c, _ int) bool { return c < g.Columns/3 } }},
	{"checkerboard", func(grid.Grid) func(c, r int) bool { return func(c, r int) bool { return (c+r)%2 == 0 } }},
	{"disc", func(g grid.Grid) func(c, r int) bool {
		return func(c, r int) bool {
			dc, dr := c-g.Columns/2, r-g.Rows/2
			return dc*dc+dr*dr <= 40
		}
	}},
	{"frame", func(g grid.Grid) func(c, r int) bool {
		return func(c, r int) bool { return c == 0 || r == 0 || c == g.Columns-1 || r == g.Rows-1 }
	}},
	{"two discs", func(g grid.Grid) func(c, r int) bool {
		return func(c, r int) bool {
			a, b := c-3, r-3
			d, e := c-g.Columns+4, r-g.Rows+4
			return a*a+b*b <= 9 || d*d+e*e <= 20
		}
	}},
}

// tileFixture is a stored image and its grid.
type tileFixture struct {
	name   string
	data   []byte
	jpeg   bool
	w, h   int
	g      grid.Grid
	shrunk bool
}

func jpegPattern(t testing.TB, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetNRGBA(x, y, patternAt(x, y))
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func tileFixtures(t testing.TB) []tileFixture {
	t.Helper()
	return []tileFixture{
		{name: "png 400x240 10px squares", data: patternImage(t, 400, 240), w: 400, h: 240, g: grid.Grid{Columns: 40, Rows: 24}},
		{name: "jpeg 400x240 10px squares", data: jpegPattern(t, 400, 240), jpeg: true, w: 400, h: 240, g: grid.Grid{Columns: 40, Rows: 24}},
		{name: "jpeg 240x160 4px squares", data: jpegPattern(t, 240, 160), jpeg: true, w: 240, h: 160, g: grid.Grid{Columns: 60, Rows: 40}},
		{name: "png 2400x1200 shrunk", data: patternImage(t, 2400, 1200), w: 2400, h: 1200, g: grid.Grid{Columns: 48, Rows: 24}, shrunk: true},
		{name: "jpeg 2400x1200 shrunk", data: jpegPattern(t, 2400, 1200), jpeg: true, w: 2400, h: 1200, g: grid.Grid{Columns: 48, Rows: 24}, shrunk: true},
		{name: "png 50x30 finer grid than pixels", data: patternImage(t, 50, 30), w: 50, h: 30, g: grid.Grid{Columns: 80, Rows: 48}},
		{name: "jpeg 70x50 odd squares", data: jpegPattern(t, 70, 50), jpeg: true, w: 70, h: 50, g: grid.Grid{Columns: 17, Rows: 12}},
	}
}

// renderAll renders every tile of the grid for a viewer, one line a tile: its
// digest, or the refusal.
func renderAll(t *testing.T, f tileFixture, v tileViewer) string {
	t.Helper()
	wc := workingOf(t, f)
	src := tileSource{g: f.g, width: f.w, height: f.h, jpeg: f.jpeg}
	cols, rows := tileGrid(f.g)
	var lines []string
	for ty := range rows + 1 { // one past the end: outside the grid
		for tx := range cols + 1 {
			nb := newNbhd(f.g, tx, ty, src.ring(), v.known(f.g))
			b, err := renderTile(wc, f.g, tx, ty, nb, f.jpeg)
			if err != nil {
				lines = append(lines, fmt.Sprintf("%d,%d: %v", tx, ty, err))
				continue
			}
			lines = append(lines, fmt.Sprintf("%d,%d: %d bytes %s", tx, ty, len(b), digestOf(b)))
		}
	}
	return strings.Join(lines, "\n")
}

func workingOf(t *testing.T, f tileFixture) *workingCopy {
	t.Helper()
	img, sw, sh, err := decodeWorkingCopy(f.data, f.g)
	if err != nil {
		t.Fatalf("%s: decodeWorkingCopy() error = %v", f.name, err)
	}
	if shrunk := img.Bounds().Dx() != sw; shrunk != f.shrunk {
		t.Fatalf("%s: shrunk = %v, want %v", f.name, shrunk, f.shrunk)
	}
	return &workingCopy{img: img, srcW: sw, srcH: sh, cols: f.g.Columns, rows: f.g.Rows}
}

// frozenDigests are the digests recorded on the code before the refactor.
var frozenDigests = map[string]string{
	"decode|header just under the limit":                        "ccf04ea77bb3524ace1d",
	"decode|header over the limit":                              "9c16a3c9d4a6d72b59ea",
	"decode|interlaced header over the limit once doubled":      "9c16a3c9d4a6d72b59ea",
	"decode|interlaced header under the limit doubled":          "ccf04ea77bb3524ace1d",
	"decode|jpeg cut in the body":                               "ba7c8d1a5d9a1d4d62a9",
	"decode|png cut in the body":                                "8a96b238cdb29b7024c1",
	"decode|png header only":                                    "ccf04ea77bb3524ace1d",
	"decode|the same size not interlaced":                       "ccf04ea77bb3524ace1d",
	"serve|asking":                                              "39bbc382a969fd71bfdd",
	"serve|hidden map":                                          "1e011d921a6878362749",
	"serve|master as|pens":                                      "208299fbcbcc9e21e67a",
	"serve|master as|salvia":                                    "16a47c0fe5dbeca8194f",
	"serve|master as|toren":                                     "3583e9648a2b1c3ff518",
	"serve|player|ana":                                          "208299fbcbcc9e21e67a",
	"serve|player|bia":                                          "295c8b28989ad3568151",
	"serve|player|caio":                                         "3583e9648a2b1c3ff518",
	"serve|player|dani":                                         "16a47c0fe5dbeca8194f",
	"build|jpeg 2400x1200 shrunk|checkerboard|false":            "170661aff338298802fa",
	"build|jpeg 2400x1200 shrunk|checkerboard|true":             "a4688ab85c356ab70632",
	"build|jpeg 2400x1200 shrunk|disc|false":                    "b85b8f940e24a3d462c4",
	"build|jpeg 2400x1200 shrunk|disc|true":                     "a4688ab85c356ab70632",
	"build|jpeg 2400x1200 shrunk|everything|false":              "a4688ab85c356ab70632",
	"build|jpeg 2400x1200 shrunk|everything|true":               "a4688ab85c356ab70632",
	"build|jpeg 2400x1200 shrunk|frame|false":                   "0089d098d61ce017da15",
	"build|jpeg 2400x1200 shrunk|frame|true":                    "a4688ab85c356ab70632",
	"build|jpeg 2400x1200 shrunk|left third|false":              "763019ccc27c86305409",
	"build|jpeg 2400x1200 shrunk|left third|true":               "a4688ab85c356ab70632",
	"build|jpeg 2400x1200 shrunk|nothing|false":                 "b5b4980013cf27ce28d7",
	"build|jpeg 2400x1200 shrunk|nothing|true":                  "a4688ab85c356ab70632",
	"build|jpeg 2400x1200 shrunk|one square|false":              "006b161bff872ea334da",
	"build|jpeg 2400x1200 shrunk|one square|true":               "a4688ab85c356ab70632",
	"build|jpeg 2400x1200 shrunk|two discs|false":               "e2b683eebd5285377d27",
	"build|jpeg 2400x1200 shrunk|two discs|true":                "a4688ab85c356ab70632",
	"build|jpeg 240x160 4px squares|checkerboard|false":         "586f2e5b0c03cba4fcdb",
	"build|jpeg 240x160 4px squares|checkerboard|true":          "327028365b7566b98f6d",
	"build|jpeg 240x160 4px squares|disc|false":                 "68121d68b9b0af90ded0",
	"build|jpeg 240x160 4px squares|disc|true":                  "327028365b7566b98f6d",
	"build|jpeg 240x160 4px squares|everything|false":           "327028365b7566b98f6d",
	"build|jpeg 240x160 4px squares|everything|true":            "327028365b7566b98f6d",
	"build|jpeg 240x160 4px squares|frame|false":                "264181cc4a2dadd7600d",
	"build|jpeg 240x160 4px squares|frame|true":                 "327028365b7566b98f6d",
	"build|jpeg 240x160 4px squares|left third|false":           "53e718348dfa1d5159c0",
	"build|jpeg 240x160 4px squares|left third|true":            "327028365b7566b98f6d",
	"build|jpeg 240x160 4px squares|nothing|false":              "1152f629c61a3c5dbf7e",
	"build|jpeg 240x160 4px squares|nothing|true":               "327028365b7566b98f6d",
	"build|jpeg 240x160 4px squares|one square|false":           "9c59bf6cfa34e706c86c",
	"build|jpeg 240x160 4px squares|one square|true":            "327028365b7566b98f6d",
	"build|jpeg 240x160 4px squares|two discs|false":            "7ec7c63a88256930118f",
	"build|jpeg 240x160 4px squares|two discs|true":             "327028365b7566b98f6d",
	"build|jpeg 400x240 10px squares|checkerboard|false":        "6bee0493439e9afc6968",
	"build|jpeg 400x240 10px squares|checkerboard|true":         "1fa819c37f58805c02a2",
	"build|jpeg 400x240 10px squares|disc|false":                "5cd4a2993a5ee7a8fb5e",
	"build|jpeg 400x240 10px squares|disc|true":                 "1fa819c37f58805c02a2",
	"build|jpeg 400x240 10px squares|everything|false":          "1fa819c37f58805c02a2",
	"build|jpeg 400x240 10px squares|everything|true":           "1fa819c37f58805c02a2",
	"build|jpeg 400x240 10px squares|frame|false":               "5761fcfbc5e073251e7d",
	"build|jpeg 400x240 10px squares|frame|true":                "1fa819c37f58805c02a2",
	"build|jpeg 400x240 10px squares|left third|false":          "9458c8823cf6758da6bb",
	"build|jpeg 400x240 10px squares|left third|true":           "1fa819c37f58805c02a2",
	"build|jpeg 400x240 10px squares|nothing|false":             "58ff4872a5fa652f8581",
	"build|jpeg 400x240 10px squares|nothing|true":              "1fa819c37f58805c02a2",
	"build|jpeg 400x240 10px squares|one square|false":          "2ba738ea3383106fa10b",
	"build|jpeg 400x240 10px squares|one square|true":           "1fa819c37f58805c02a2",
	"build|jpeg 400x240 10px squares|two discs|false":           "536d589f1f76d5997fe1",
	"build|jpeg 400x240 10px squares|two discs|true":            "1fa819c37f58805c02a2",
	"build|jpeg 70x50 odd squares|checkerboard|false":           "cb0ea2680fc8020ca3d9",
	"build|jpeg 70x50 odd squares|checkerboard|true":            "14d69755cd42d301b967",
	"build|jpeg 70x50 odd squares|disc|false":                   "43b91bd049e4d7d1a0d4",
	"build|jpeg 70x50 odd squares|disc|true":                    "14d69755cd42d301b967",
	"build|jpeg 70x50 odd squares|everything|false":             "14d69755cd42d301b967",
	"build|jpeg 70x50 odd squares|everything|true":              "14d69755cd42d301b967",
	"build|jpeg 70x50 odd squares|frame|false":                  "7e559406d3adc92f365d",
	"build|jpeg 70x50 odd squares|frame|true":                   "14d69755cd42d301b967",
	"build|jpeg 70x50 odd squares|left third|false":             "b5cebc6a469ef18c1d28",
	"build|jpeg 70x50 odd squares|left third|true":              "14d69755cd42d301b967",
	"build|jpeg 70x50 odd squares|nothing|false":                "d5eca2fb62e2967cf581",
	"build|jpeg 70x50 odd squares|nothing|true":                 "14d69755cd42d301b967",
	"build|jpeg 70x50 odd squares|one square|false":             "02cc648e83bc64a24101",
	"build|jpeg 70x50 odd squares|one square|true":              "14d69755cd42d301b967",
	"build|jpeg 70x50 odd squares|two discs|false":              "1e7e4bba64ed14acf376",
	"build|jpeg 70x50 odd squares|two discs|true":               "14d69755cd42d301b967",
	"build|png 2400x1200 shrunk|checkerboard|false":             "170661aff338298802fa",
	"build|png 2400x1200 shrunk|checkerboard|true":              "a4688ab85c356ab70632",
	"build|png 2400x1200 shrunk|disc|false":                     "b85b8f940e24a3d462c4",
	"build|png 2400x1200 shrunk|disc|true":                      "a4688ab85c356ab70632",
	"build|png 2400x1200 shrunk|everything|false":               "a4688ab85c356ab70632",
	"build|png 2400x1200 shrunk|everything|true":                "a4688ab85c356ab70632",
	"build|png 2400x1200 shrunk|frame|false":                    "0089d098d61ce017da15",
	"build|png 2400x1200 shrunk|frame|true":                     "a4688ab85c356ab70632",
	"build|png 2400x1200 shrunk|left third|false":               "763019ccc27c86305409",
	"build|png 2400x1200 shrunk|left third|true":                "a4688ab85c356ab70632",
	"build|png 2400x1200 shrunk|nothing|false":                  "b5b4980013cf27ce28d7",
	"build|png 2400x1200 shrunk|nothing|true":                   "a4688ab85c356ab70632",
	"build|png 2400x1200 shrunk|one square|false":               "006b161bff872ea334da",
	"build|png 2400x1200 shrunk|one square|true":                "a4688ab85c356ab70632",
	"build|png 2400x1200 shrunk|two discs|false":                "e2b683eebd5285377d27",
	"build|png 2400x1200 shrunk|two discs|true":                 "a4688ab85c356ab70632",
	"build|png 400x240 10px squares|checkerboard|false":         "2024d3165bb0ac7bc708",
	"build|png 400x240 10px squares|checkerboard|true":          "47de568087f453a0d2f6",
	"build|png 400x240 10px squares|disc|false":                 "ec09912fca5838be7f7f",
	"build|png 400x240 10px squares|disc|true":                  "47de568087f453a0d2f6",
	"build|png 400x240 10px squares|everything|false":           "47de568087f453a0d2f6",
	"build|png 400x240 10px squares|everything|true":            "47de568087f453a0d2f6",
	"build|png 400x240 10px squares|frame|false":                "fe7446ffa282a2437ab7",
	"build|png 400x240 10px squares|frame|true":                 "47de568087f453a0d2f6",
	"build|png 400x240 10px squares|left third|false":           "6d24eee1d70755972f1f",
	"build|png 400x240 10px squares|left third|true":            "47de568087f453a0d2f6",
	"build|png 400x240 10px squares|nothing|false":              "58ff4872a5fa652f8581",
	"build|png 400x240 10px squares|nothing|true":               "47de568087f453a0d2f6",
	"build|png 400x240 10px squares|one square|false":           "57aea315c234c4379320",
	"build|png 400x240 10px squares|one square|true":            "47de568087f453a0d2f6",
	"build|png 400x240 10px squares|two discs|false":            "b14f8d3d6c1319cb29ff",
	"build|png 400x240 10px squares|two discs|true":             "47de568087f453a0d2f6",
	"build|png 50x30 finer grid than pixels|checkerboard|false": "2ac2a5f020ae40bf6b1e",
	"build|png 50x30 finer grid than pixels|checkerboard|true":  "13d5f923e19f5d249ab3",
	"build|png 50x30 finer grid than pixels|disc|false":         "3ee19b93eea5e55006e5",
	"build|png 50x30 finer grid than pixels|disc|true":          "13d5f923e19f5d249ab3",
	"build|png 50x30 finer grid than pixels|everything|false":   "13d5f923e19f5d249ab3",
	"build|png 50x30 finer grid than pixels|everything|true":    "13d5f923e19f5d249ab3",
	"build|png 50x30 finer grid than pixels|frame|false":        "09f46208f3d89d6cf6a4",
	"build|png 50x30 finer grid than pixels|frame|true":         "13d5f923e19f5d249ab3",
	"build|png 50x30 finer grid than pixels|left third|false":   "a36e7bdde2b6ea363d6a",
	"build|png 50x30 finer grid than pixels|left third|true":    "13d5f923e19f5d249ab3",
	"build|png 50x30 finer grid than pixels|nothing|false":      "4e53560864eb501901a7",
	"build|png 50x30 finer grid than pixels|nothing|true":       "13d5f923e19f5d249ab3",
	"build|png 50x30 finer grid than pixels|one square|false":   "bf6ed2c7ba0970717b8d",
	"build|png 50x30 finer grid than pixels|one square|true":    "13d5f923e19f5d249ab3",
	"build|png 50x30 finer grid than pixels|two discs|false":    "a8fc09a0f88916bf86ba",
	"build|png 50x30 finer grid than pixels|two discs|true":     "13d5f923e19f5d249ab3",
	"decode|a GIF":                                         "dd51ce5223fa9b4a21a1",
	"decode|empty":                                         "dd51ce5223fa9b4a21a1",
	"decode|garbage":                                       "dd51ce5223fa9b4a21a1",
	"decode|jpeg marker only":                              "0e622e7caced7ad8a581",
	"decode|jpeg no shrink":                                "3e9c2891ae4702a267dd",
	"decode|jpeg shrunk":                                   "8ef332f01f44a3916cb8",
	"decode|png 16 bits":                                   "780c4162ed56db0ba575",
	"decode|png finer than the pixels, shrunk":             "9f5082ee8e4b94a0725f",
	"decode|png gray 16 bits":                              "ce3c92ad79c2efc9f063",
	"decode|png no shrink":                                 "6be842386fb3419aace3",
	"decode|png of zero width":                             "6dde4b78a1470e56a80c",
	"decode|png shrunk":                                    "a18e94bc163535deb65d",
	"decode|png shrunk tall":                               "24123e1a8279cfa9fe9a",
	"decode|png shrunk with no columns":                    "e805bb9683b28a6477ac",
	"decode|png shrunk with no grid":                       "e805bb9683b28a6477ac",
	"decode|png shrunk with no rows":                       "e805bb9683b28a6477ac",
	"decode|png signature only":                            "0e622e7caced7ad8a581",
	"decode|png unshrunk with no grid":                     "6be842386fb3419aace3",
	"render|jpeg 2400x1200 shrunk|checkerboard":            "71a7b5d1ccb5ae758586",
	"render|jpeg 2400x1200 shrunk|disc":                    "7cd338429425793f6828",
	"render|jpeg 2400x1200 shrunk|everything":              "6833298d5b7b3d039c18",
	"render|jpeg 2400x1200 shrunk|frame":                   "3029e172caa4f2605a23",
	"render|jpeg 2400x1200 shrunk|left third":              "46b4f94da3bb8718cfe8",
	"render|jpeg 2400x1200 shrunk|nothing":                 "e0b55224457770e0c617",
	"render|jpeg 2400x1200 shrunk|one square":              "d9a6244871bbac70e705",
	"render|jpeg 2400x1200 shrunk|two discs":               "4927de677b9ff9a8c72c",
	"render|jpeg 240x160 4px squares|checkerboard":         "8b2561f5c425cbe17f1e",
	"render|jpeg 240x160 4px squares|disc":                 "40d4265f6874adf41964",
	"render|jpeg 240x160 4px squares|everything":           "757e686143083df6b733",
	"render|jpeg 240x160 4px squares|frame":                "8b2561f5c425cbe17f1e",
	"render|jpeg 240x160 4px squares|left third":           "8d998762f0aae3c7a098",
	"render|jpeg 240x160 4px squares|nothing":              "8b2561f5c425cbe17f1e",
	"render|jpeg 240x160 4px squares|one square":           "8b2561f5c425cbe17f1e",
	"render|jpeg 240x160 4px squares|two discs":            "02d1fa836692e2527074",
	"render|jpeg 400x240 10px squares|checkerboard":        "0d549ccbfbe76e2b85da",
	"render|jpeg 400x240 10px squares|disc":                "f524156d5bb31c645d13",
	"render|jpeg 400x240 10px squares|everything":          "c05423501ceb60d63d5a",
	"render|jpeg 400x240 10px squares|frame":               "0d549ccbfbe76e2b85da",
	"render|jpeg 400x240 10px squares|left third":          "f1fed84b6c3b42ea0538",
	"render|jpeg 400x240 10px squares|nothing":             "0d549ccbfbe76e2b85da",
	"render|jpeg 400x240 10px squares|one square":          "0d549ccbfbe76e2b85da",
	"render|jpeg 400x240 10px squares|two discs":           "c7785c78bf304e21d1b5",
	"render|jpeg 70x50 odd squares|checkerboard":           "31dca779b40231eb3452",
	"render|jpeg 70x50 odd squares|disc":                   "7835a336431160780b8d",
	"render|jpeg 70x50 odd squares|everything":             "bfead913ffa00b2596aa",
	"render|jpeg 70x50 odd squares|frame":                  "aea6fcfac2e76328409b",
	"render|jpeg 70x50 odd squares|left third":             "34218ccd554063e89a4a",
	"render|jpeg 70x50 odd squares|nothing":                "31dca779b40231eb3452",
	"render|jpeg 70x50 odd squares|one square":             "31dca779b40231eb3452",
	"render|jpeg 70x50 odd squares|two discs":              "e10fd9e778f7e5fee43a",
	"render|png 2400x1200 shrunk|checkerboard":             "1255beb1a3f7ba36d8f9",
	"render|png 2400x1200 shrunk|disc":                     "bad3ef95192637cb520d",
	"render|png 2400x1200 shrunk|everything":               "751982197dbe0e9aabdd",
	"render|png 2400x1200 shrunk|frame":                    "1546b624e1ddcc07a127",
	"render|png 2400x1200 shrunk|left third":               "e356586d4b31d4266882",
	"render|png 2400x1200 shrunk|nothing":                  "e0b55224457770e0c617",
	"render|png 2400x1200 shrunk|one square":               "b78dab7cb5f99b005ae7",
	"render|png 2400x1200 shrunk|two discs":                "5c8bccb66fc2de36bb58",
	"render|png 400x240 10px squares|checkerboard":         "1e9150f1e6637d94bac9",
	"render|png 400x240 10px squares|disc":                 "6749b7297eaea7ae11f2",
	"render|png 400x240 10px squares|everything":           "5b74d7db056f21e2ad73",
	"render|png 400x240 10px squares|frame":                "0c832ef1a128dbac61c7",
	"render|png 400x240 10px squares|left third":           "a6bc303225efd7245fee",
	"render|png 400x240 10px squares|nothing":              "0d549ccbfbe76e2b85da",
	"render|png 400x240 10px squares|one square":           "fb2427865d21170c012f",
	"render|png 400x240 10px squares|two discs":            "7a7deaa6f0321302bc45",
	"render|png 50x30 finer grid than pixels|checkerboard": "1f65962d9f6b2c7e587e",
	"render|png 50x30 finer grid than pixels|disc":         "2c66a153d0c2fd01ca37",
	"render|png 50x30 finer grid than pixels|everything":   "d649d163337c4467c66c",
	"render|png 50x30 finer grid than pixels|frame":        "0e926076735e57a4bab7",
	"render|png 50x30 finer grid than pixels|left third":   "b99924f3b26211f2acba",
	"render|png 50x30 finer grid than pixels|nothing":      "0e926076735e57a4bab7",
	"render|png 50x30 finer grid than pixels|one square":   "0e926076735e57a4bab7",
	"render|png 50x30 finer grid than pixels|two discs":    "ab81b0e02fd65d5c832c",
}

func checkFrozen(t *testing.T, key, got string) {
	t.Helper()
	want, ok := frozenDigests[key]
	if !ok {
		t.Errorf("RECORD\t%s\t%s\n%s", key, digestOf([]byte(got)), got)
		return
	}
	if d := digestOf([]byte(got)); d != want {
		t.Errorf("%s: digest = %s, want %s\n%s", key, d, want, got)
	}
}

func TestTilesAreByteForByteFrozen(t *testing.T) {
	t.Parallel()
	for _, f := range tileFixtures(t) {
		for _, v := range tileViewers {
			t.Run(f.name+"/"+v.name, func(t *testing.T) {
				t.Parallel()
				checkFrozen(t, "render|"+f.name+"|"+v.name, renderAll(t, f, v))
			})
		}
	}
}

// TestBuildTilesIsFrozen lists the tiles each viewer has, with their revisions,
// for a plan seen and for the whole plan ("Claro").
func TestBuildTilesIsFrozen(t *testing.T) {
	t.Parallel()
	for _, f := range tileFixtures(t) {
		for _, v := range tileViewers {
			for _, all := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/claro=%v", f.name, v.name, all), func(t *testing.T) {
					t.Parallel()
					pv := &playerView{g: f.g, codes: make([]byte, f.g.Squares()), terrainAll: all}
					known := v.known(f.g)
					for r := range f.g.Rows {
						for c := range f.g.Columns {
							if known(c, r) {
								pv.codes[r*f.g.Columns+c] = stateRemembered
							}
						}
					}
					src := tileSource{mapID: "m", campaignID: "c", imageID: "image-1", g: f.g, width: f.w, height: f.h, jpeg: f.jpeg}
					vt := buildTiles(pv, src)
					var lines []string
					for _, ti := range vt.tiles {
						lines = append(lines, fmt.Sprintf("%d,%d rev=%d nb=%s", ti.tx, ti.ty, ti.revision, digestOf([]byte(ti.nb.bits))))
					}
					lines = append(lines, fmt.Sprintf("image=%s grid=%v index=%d", vt.imageID, vt.g, len(vt.byXY)))
					for _, ti := range vt.tiles {
						if got, ok := vt.find(ti.tx, ti.ty); !ok || got.revision != ti.revision {
							t.Errorf("find(%d,%d) = %v, %v", ti.tx, ti.ty, got, ok)
						}
					}
					checkFrozen(t, fmt.Sprintf("build|%s|%s|%v", f.name, v.name, all), strings.Join(lines, "\n"))
				})
			}
		}
	}
	// A source whose size is unknown (zero) is taken as one pixel, and a grid of
	// 20 x 20 squares leaves almost every tile with no pixel to draw: none is listed.
	pv := &playerView{g: grid.Grid{Columns: 20, Rows: 20}, codes: make([]byte, 400)}
	pv.codes[0] = stateRemembered
	if vt := buildTiles(pv, tileSource{imageID: "i", g: pv.g}); len(vt.tiles) != 0 {
		t.Errorf("a source of zero size listed %d tiles, want 0", len(vt.tiles))
	}
}

// pngWith16Bits is a 16-bit RGBA PNG.
func pngWith16Bits(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA64(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetRGBA64(x, y, color.RGBA64{R: uint16(x * 257), G: uint16(y * 257), B: 0x8000, A: 0xffff})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// pngGray16 is a 16-bit gray PNG.
func pngGray16(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewGray16(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetGray16(x, y, color.Gray16{Y: uint16(x*300 + y)})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// pngHeaderOnly is a PNG signature and an IHDR chunk and nothing else: enough
// for DecodeConfig, a refusal at the body for the decode.
func pngHeaderOnly(w, h int, colorType, interlace byte) []byte {
	ihdr := binary.BigEndian.AppendUint32(nil, uint32(w)) //nolint:gosec // G115: a test size
	ihdr = binary.BigEndian.AppendUint32(ihdr, uint32(h)) //nolint:gosec // G115: a test size
	ihdr = append(ihdr, 8, colorType, 0, 0, interlace)
	out := []byte("\x89PNG\r\n\x1a\n")
	out = binary.BigEndian.AppendUint32(out, uint32(len(ihdr))) //nolint:gosec // G115: 13
	chunk := append([]byte("IHDR"), ihdr...)
	out = append(out, chunk...)
	return binary.BigEndian.AppendUint32(out, crc32.ChecksumIEEE(chunk))
}

func TestDecodeWorkingCopyIsFrozen(t *testing.T) {
	t.Parallel()
	small := grid.Grid{Columns: 8, Rows: 6}
	big := grid.Grid{Columns: 48, Rows: 24}
	bigPNG := patternImage(t, 2400, 1200)
	cases := []struct {
		name string
		data []byte
		g    grid.Grid
	}{
		{"png no shrink", patternImage(t, 40, 30), small},
		{"png shrunk", bigPNG, big},
		{"png shrunk tall", patternImage(t, 1000, 3000), grid.Grid{Columns: 10, Rows: 30}},
		{"png shrunk with no grid", bigPNG, grid.Grid{}},
		{"png shrunk with no columns", bigPNG, grid.Grid{Rows: 24}},
		{"png shrunk with no rows", bigPNG, grid.Grid{Columns: 48}},
		{"png unshrunk with no grid", patternImage(t, 40, 30), grid.Grid{}},
		{"png finer than the pixels, shrunk", patternImage(t, 2100, 100), grid.Grid{Columns: 3000, Rows: 1}},
		{"jpeg no shrink", jpegPattern(t, 100, 80), small},
		{"jpeg shrunk", jpegPattern(t, 2400, 1200), big},
		{"png 16 bits", pngWith16Bits(t, 40, 30), small},
		{"png gray 16 bits", pngGray16(t, 40, 30), small},
		{"empty", nil, small},
		{"garbage", []byte("not an image at all, not even close"), small},
		{"a GIF", []byte("GIF89a\x01\x00\x01\x00"), small},
		{"png signature only", []byte("\x89PNG\r\n\x1a\n"), small},
		{"jpeg marker only", []byte("\xff\xd8\xff"), small},
		{"png cut in the body", patternImage(t, 40, 30)[:100], small},
		{"jpeg cut in the body", jpegPattern(t, 100, 80)[:len(jpegPattern(t, 100, 80))/2], small},
		{"png header only", pngHeaderOnly(40, 30, 6, 0), small},
		{"png of zero width", pngHeaderOnly(0, 30, 6, 0), small},
		{"header just under the limit", pngHeaderOnly(7000, 7000, 6, 0), small},
		{"header over the limit", pngHeaderOnly(8000, 8000, 6, 0), small},
		{"interlaced header under the limit doubled", pngHeaderOnly(4000, 4000, 6, 1), small},
		{"interlaced header over the limit once doubled", pngHeaderOnly(5200, 5200, 6, 1), small},
		{"the same size not interlaced", pngHeaderOnly(5200, 5200, 6, 0), small},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			img, sw, sh, err := decodeWorkingCopy(c.data, c.g)
			got := fmt.Sprintf("error: %v", err)
			if err == nil {
				b := img.Bounds()
				got = fmt.Sprintf("%dx%d from %dx%d %s", b.Dx(), b.Dy(), sw, sh, digestOf(img.Pix))
			} else if img != nil || sw != 0 || sh != 0 {
				t.Errorf("a refusal also returned %v, %d, %d", img, sw, sh)
			}
			checkFrozen(t, "decode|"+c.name, got)
		})
	}
}

// tileLine renders what a tile request answered, without the parts that change
// from one run to the next (the ETag is checked against the revision).
func tileLine(r httpResult) string {
	h := r.header
	return fmt.Sprintf("status=%d type=%q cache=%q vary=%q nosniff=%q csp=%q corp=%q length=%q body=%s",
		r.status, h.Get("Content-Type"), h.Get("Cache-Control"), h.Get("Vary"), h.Get("X-Content-Type-Options"),
		h.Get("Content-Security-Policy"), h.Get("Cross-Origin-Resource-Policy"), h.Get("Content-Length"), digestOf(r.body))
}

// TestTileServeIsFrozen freezes what each viewer of the cave is served, tile by
// tile, and the answer to each way of asking wrongly, in the order the checks
// run in.
func TestTileServeIsFrozen(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	etagOf := func(tile *mapsv1.MapTile) string { return `"t` + strconv.Itoa(int(tile.GetRevision())) + `"` }

	// What each player is served: every tile of their index.
	for name, u := range map[string]*user{"ana": c.ana, "caio": c.caio, "bia": c.bia, "dani": c.dani} {
		res := u.mustVision(c.campaign, c.mapID)
		var lines []string
		for _, tile := range res.GetTiles() {
			r := u.get(tileURL(res, tile))
			if r.header.Get("ETag") != etagOf(tile) {
				t.Errorf("%s tile %d,%d: ETag = %q, want %q", name, tile.GetTx(), tile.GetTy(), r.header.Get("ETag"), etagOf(tile))
			}
			lines = append(lines, fmt.Sprintf("%d,%d %s", tile.GetTx(), tile.GetTy(), tileLine(r)))
		}
		checkFrozen(t, "serve|player|"+name, strings.Join(lines, "\n"))
	}
	// The master "Ver como" each character gets the same tiles as the player.
	for name, ch := range map[string]string{"pens": c.pens.GetId(), "toren": c.toren.GetId(), "salvia": c.salvia.GetId()} {
		res := c.master.mustVision(c.campaign, c.mapID, ch)
		var lines []string
		for _, tile := range res.GetTiles() {
			lines = append(lines, fmt.Sprintf("%d,%d %s", tile.GetTx(), tile.GetTy(), tileLine(c.master.get(tileURL(res, tile)+"&as="+ch))))
		}
		checkFrozen(t, "serve|master as|"+name, strings.Join(lines, "\n"))
	}

	// Each way of asking wrongly, one line each.
	good := c.ana.mustVision(c.campaign, c.mapID)
	base := good.GetTilesPath()
	okTile := good.GetTiles()[0]
	okPath := fmt.Sprintf("%s%d/%d", base, okTile.GetTx(), okTile.GetTy())
	outsider := c.h.newUser("Fora")
	pending := c.h.newUser("Pendente")
	c.h.join(c.master, c.campaign, true, pending)
	asAna := "?as=" + c.pens.GetId()
	etag := etagOf(okTile)
	ask := []struct {
		name string
		do   func() httpResult
	}{
		{"anonymous", func() httpResult { return c.h.anonymous().get(okPath) }},
		{"anonymous with a bad id", func() httpResult { return c.h.anonymous().get(TilesPath + "x/tiles/0/0") }},
		{"map not a UUID", func() httpResult { return c.ana.get(TilesPath + "x/tiles/0/0") }},
		{"tx not a number", func() httpResult { return c.ana.get(base + "x/0") }},
		{"ty not a number", func() httpResult { return c.ana.get(base + "0/x") }},
		{"tx negative", func() httpResult { return c.ana.get(base + "-1/0") }},
		{"ty negative", func() httpResult { return c.ana.get(base + "0/-1") }},
		{"map unknown", func() httpResult { return c.ana.get(TilesPath + newKey() + "/tiles/0/0") }},
		{"a non-member", func() httpResult { return outsider.get(okPath) }},
		{"a pending member", func() httpResult { return pending.get(okPath) }},
		{"the master with no as", func() httpResult { return c.master.get(okPath) }},
		{"a player with as", func() httpResult { return c.ana.get(okPath + asAna) }},
		{"the master as nobody", func() httpResult { return c.master.get(okPath + "?as=" + newKey()) }},
		{"the master as an NPC", func() httpResult { return c.master.get(okPath + "?as=" + c.goblin1.GetId()) }},
		{"the master as Pensantus", func() httpResult { return c.master.get(okPath + asAna) }},
		{"the master as a character with an empty value", func() httpResult { return c.master.get(okPath + "?as=") }},
		{"a tile past the grid", func() httpResult { return c.ana.get(base + "9/9") }},
		{"a tile Ana has no square of", func() httpResult { return c.ana.get(base + "1/0") }},
		{"If-None-Match equal", func() httpResult { return c.ana.get(okPath, "If-None-Match", etag) }},
		{"If-None-Match weak", func() httpResult { return c.ana.get(okPath, "If-None-Match", "W/"+etag) }},
		{"If-None-Match a list", func() httpResult { return c.ana.get(okPath, "If-None-Match", `"x", `+etag) }},
		{"If-None-Match star", func() httpResult { return c.ana.get(okPath, "If-None-Match", "*") }},
		{"If-None-Match other", func() httpResult { return c.ana.get(okPath, "If-None-Match", `"t1"`) }},
		{"If-None-Match for a tile that does not exist", func() httpResult { return c.ana.get(base+"9/9", "If-None-Match", "*") }},
		{"HEAD", func() httpResult { return c.ana.do(headRequest(t, c, okPath)) }},
	}
	var lines []string
	for _, a := range ask {
		r := a.do()
		line := tileLine(r)
		if r.status != http.StatusOK && r.status != http.StatusNotModified {
			line += " error=" + strings.TrimSpace(string(r.body))
		}
		if r.status == http.StatusNotModified && r.header.Get("ETag") != etag {
			t.Errorf("%s: a 304 carried ETag %q, want %q", a.name, r.header.Get("ETag"), etag)
		}
		lines = append(lines, a.name+": "+line)
	}
	checkFrozen(t, "serve|asking", strings.Join(lines, "\n"))

	// A map the players do not see, then one with the fog off.
	c.master.setMapRevealed(c.campaign, c.mapID, false)
	if _, err := c.master.setCurrentMap(c.campaign, c.probeMap); err != nil {
		t.Fatal(err)
	}
	hidden := []string{
		"ana: " + tileLine(c.ana.get(okPath)),
		"master as Pensantus: " + tileLine(c.master.get(okPath+asAna)),
	}
	checkFrozen(t, "serve|hidden map", strings.Join(hidden, "\n"))
}

// headRequest is a HEAD for a path of the test server.
func headRequest(t *testing.T, c *cave, path string) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodHead, c.h.server.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	return req
}
