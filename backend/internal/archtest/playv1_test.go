package archtest

import (
	_ "embed"
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"testing"
)

// playv1Exceptions lists the files outside the play module and the wiring that still import playv1.
// It can only shrink.
//
//go:embed playv1_exceptions.txt
var playv1Exceptions string

// playv1Paths are the import paths of the generated play API, under the module path.
// The Connect package is checked too, and only the allowed places import it.
var playv1Paths = []string{"gen/meurpg/play/v1", "gen/meurpg/play/v1/playv1connect"}

// playv1Allowed are the places a non-test file may import playv1.
// They are the play module and the API wiring.
var playv1Allowed = []string{"internal/play/", "internal/platform/wiring/", "cmd/"}

// TestPlayV1StaysInPlayAndWiring fails for a playv1 import outside the play module and the wiring.
// The wiring is cmd/ and internal/platform/wiring.
// A non-test file that playv1_exceptions.txt lists is allowed.
// A listed file that no longer imports it fails.
// Test files may import it.
func TestPlayV1StaysInPlayAndWiring(t *testing.T) {
	root := os.DirFS(backendRoot)
	mod := modulePath(t, root) + "/"
	found := map[string]string{}
	files := 0
	for _, top := range []string{"internal", "cmd"} {
		err := walkGoFiles(root, top, func(path string) error {
			if isTestFile(path) {
				return nil
			}
			files++
			src, err := fs.ReadFile(root, path)
			if err != nil {
				return err
			}
			bad, err := importsPlayV1Outside(path, src, mod)
			if err != nil {
				return err
			}
			if bad {
				found[path] = fmt.Sprintf("%s imports playv1 outside the play module and the API wiring: pass the plain types "+
					"through the module's link package (internal/play/link) and let the API layer map to playv1", path)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("reading %s: %v", top, err)
		}
	}
	if files == 0 {
		t.Fatal("found no Go file: is backendRoot right?")
	}
	failProblems(t, compareExceptions("playv1_exceptions.txt", parseExceptions(playv1Exceptions), found))
	t.Logf("%d non-test Go files checked for playv1 imports; %d known exceptions", files, len(found))
}

// importsPlayV1Outside reports whether a file that is not in an allowed place imports a play API package.
// The module argument is the module path and a slash.
func importsPlayV1Outside(path string, src []byte, module string) (bool, error) {
	for _, ok := range playv1Allowed {
		if strings.HasPrefix(path, ok) {
			return false, nil
		}
	}
	f, err := parser.ParseFile(token.NewFileSet(), path, src, parser.ImportsOnly)
	if err != nil {
		return false, err
	}
	for _, imp := range f.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		for _, want := range playv1Paths {
			if p == module+want {
				return true, nil
			}
		}
	}
	return false, nil
}

func TestImportsPlayV1Outside(t *testing.T) {
	const module = "example.test/backend/"
	target := module + playv1Paths[0]
	imp := func(alias string) string {
		return "package x\nimport " + alias + ` "` + target + "\"\n"
	}
	cases := []struct {
		name, path, src string
		want            bool
	}{
		{"characters", "internal/characters/a.go", imp("playv1"), true},
		{"renamed", "internal/maps/a.go", imp("pb"), true},
		{"play module", "internal/play/a.go", imp("playv1"), false},
		{"cmd", "cmd/api/main.go", imp("playv1"), false},
		{"wiring", "internal/platform/wiring/w.go", imp("playv1"), false},
		{"other import", "internal/maps/a.go", "package x\nimport _ \"example.test/backend/gen/meurpg/maps/v1\"\n", false},
		{"connect package", "internal/maps/a.go", "package x\nimport _ \"" + module + playv1Paths[1] + "\"\n", true},
		{"similar path", "internal/maps/a.go", "package x\nimport _ \"" + target + "x\"\n", false},
	}
	for _, c := range cases {
		got, err := importsPlayV1Outside(c.path, []byte(c.src), module)
		if err != nil || got != c.want {
			t.Errorf("%s: got %v (err %v), want %v", c.name, got, err, c.want)
		}
	}
}
