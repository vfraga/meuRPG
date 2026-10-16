package archtest

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// backendRoot is the backend module's directory, seen from this package's
// directory (go test runs a test in its package's directory).
const backendRoot = "../.."

// linkPackage is the one package of a module that any other module may
// import: the plain types that cross the interfaces between modules
// (maps/link, notes/link, play/link, progression/link). The interfaces are
// declared by the modules and wired in cmd/api.
const linkPackage = "link"

// sharedRoots are the modules whose packages every other module may import
// directly. The value says which: false is the module's root package only,
// true is the root and every package under it.
var sharedRoots = map[string]bool{
	"authz":           false, // who may do what in a campaign
	"identity":        false, // the signed-in person (identitydb stays private)
	"campaignpackage": false, // the Part interface each module implements for the export
	"rules":           true,  // the rules engine and the SRD content, pure (rules/combat, rules/grid...)
	"platform":        true,  // configuration, database, logs, dice: the shared plumbing
}

// outsideInternal are the top-level directories of the backend module, other
// than internal/, that code under internal/ may import: the generated API
// contracts and the embedded migrations (platform/dbtest applies them).
// cmd/ is never imported: it wires the modules and depends on all of them.
var outsideInternal = []string{"gen", "migrations"}

// testHelpers are the packages of another module that a test file may import
// besides that module's root and link packages: fakes and fixtures for the
// integration harnesses.
var testHelpers = []string{
	"characters/contenttest", // the live content source the other suites build characters with
	"identity/oidctest",      // the OIDC provider for tests
	"maps/images/gen",        // the image generator, for its fake
}

// prodEdges is every module-level dependency of the production code: each
// module and the modules its non-test files import. It is exact: a new edge
// fails until it is added here on purpose, and an edge that is gone fails
// until it is removed, so this table always tells how the modules are coupled.
// An edge to a module that is not a shared root goes through its link package.
var prodEdges = map[string][]string{
	"authz":           {"platform"},
	"campaignpackage": {"authz", "platform"},
	"campaigns":       {"authz", "campaignpackage", "identity", "platform", "rules"},
	"characters":      {"authz", "campaignpackage", "identity", "maps", "platform", "play", "progression", "rules"}, // maps, play, progression: link
	"identity":        {"platform"},
	"maps":            {"authz", "campaignpackage", "notes", "platform", "play", "progression", "rules"}, // notes, play, progression: link
	"notes":           {"authz", "platform"},
	"play":            {"authz", "campaignpackage", "maps", "platform", "progression", "rules"}, // maps, progression: link
	"progression":     {"authz", "platform", "rules"},
	"rules":           {"platform"}, // rules stays a leaf: it knows no module
}

// testEdges is every module-level dependency that only test files add, on top
// of prodEdges, exact in the same way. The integration harnesses start the
// real services of the modules they talk to, and leaktest starts them all.
var testEdges = map[string][]string{
	"campaignpackage": {"campaigns", "characters", "identity", "maps", "notes", "play", "rules"},
	"characters":      {"campaigns"},
	"leaktest":        {"campaignpackage", "campaigns", "characters", "identity", "maps", "notes", "platform", "play", "progression", "rules"},
	"maps":            {"campaigns", "characters", "identity"},
	"platform":        {"system"}, // platform/nostore's test serves the system service
	"play":            {"campaigns", "characters", "identity"},
	"progression":     {"campaigns", "characters", "identity", "maps", "play"},
}

// knownCycles are the module-level cycles of the production code, each as its
// modules in order, joined by " <-> ". maps and play need each other (the map
// reads the combat's positions, the combat reads the map's terrain and traps),
// each through the other's link package, so there is no package cycle.
var knownCycles = []string{"maps <-> play"}

// importSite is one import of one file under internal/.
type importSite struct {
	file   string // the importing file, relative to backend/
	module string // the importing file's module
	target string // the imported package, relative to backend/
	test   bool   // whether the importing file is a _test.go file
}

// TestModuleBoundaries reads the imports of every Go file under internal/
// and fails when a module reaches into another one. A module is a directory
// right under internal/ (platform counts as one). The rules:
//
//   - A non-test file of module A may import, from another module B, only
//     B's link package (internal/B/link), unless B is a shared root:
//     authz, identity and campaignpackage (their root package only), rules
//     (rules and rules/*) and platform (platform/*).
//   - No file under internal/ imports cmd/; outside internal/, only gen/ and
//     migrations may be imported.
//   - The module-level edges of the non-test files are exactly prodEdges,
//     and the only cycles among them are knownCycles (maps <-> play).
//   - Test files are checked on their own list: besides the above, they may
//     import another module's root package (the integration harnesses start
//     the real services) and the packages in testHelpers, never another
//     module's other packages (its sqlc queries, say). The edges only they
//     add are exactly testEdges. Cycles through test files are allowed: they
//     do not exist when the server is built, and Go refuses a real one.
func TestModuleBoundaries(t *testing.T) {
	sites := scanImports(t)

	var violations []string
	prod := map[string]map[string]string{} // from -> to -> one importing file, for the message
	test := map[string]map[string]string{}
	for _, s := range sites {
		if v := checkSite(s); v != "" {
			violations = append(violations, v)
		}
		to, ok := targetModule(s.target)
		if !ok || to == s.module {
			continue
		}
		edges := prod
		if s.test {
			edges = test
		}
		if edges[s.module] == nil {
			edges[s.module] = map[string]string{}
		}
		if _, seen := edges[s.module][to]; !seen {
			edges[s.module][to] = s.file + " imports " + s.target
		}
	}
	// An edge that the production code already has is not a test edge.
	for from, tos := range test {
		for to := range tos {
			if _, ok := prod[from][to]; ok {
				delete(tos, to)
			}
		}
	}
	slices.Sort(violations)
	for _, v := range violations {
		t.Error(v)
	}
	compareEdges(t, "prodEdges", prod, prodEdges)
	compareEdges(t, "testEdges", test, testEdges)

	got := cyclesOf(prod)
	if !slices.Equal(got, knownCycles) {
		t.Errorf("the module-level cycles of the production code are %q, knownCycles says %q: "+
			"a new cycle means a module now needs one that needs it back (cut it with an interface "+
			"and a link package); a cycle that is gone is removed from knownCycles", got, knownCycles)
	}
	t.Logf("%d imports of the project's own packages checked", len(sites))
}

// scanImports parses the imports of every Go file under backend/internal and
// returns those of the backend's own packages.
func scanImports(t *testing.T) []importSite {
	t.Helper()
	root := os.DirFS(backendRoot)
	module := modulePath(t, root)
	fset := token.NewFileSet()
	var sites []importSite
	err := fs.WalkDir(root, "internal", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// The go tool ignores these directories too.
			if name := d.Name(); name == "testdata" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		src, err := fs.ReadFile(root, path)
		if err != nil {
			return err
		}
		f, err := parser.ParseFile(fset, path, src, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range f.Imports {
			p, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
			target, own := strings.CutPrefix(p, module+"/")
			if !own {
				continue
			}
			sites = append(sites, importSite{
				file:   path,
				module: strings.Split(path, "/")[1],
				target: target,
				test:   strings.HasSuffix(path, "_test.go"),
			})
		}
		return nil
	})
	if err != nil {
		t.Fatalf("reading the backend's source: %v", err)
	}
	if len(sites) == 0 {
		t.Fatal("found no import of the backend's own packages: is backendRoot right?")
	}
	return sites
}

// modulePath reads the module path from backend/go.mod.
func modulePath(t *testing.T, root fs.FS) string {
	t.Helper()
	mod, err := fs.ReadFile(root, "go.mod")
	if err != nil {
		t.Fatalf("reading go.mod: %v", err)
	}
	for line := range strings.Lines(string(mod)) {
		if path, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.TrimSpace(path)
		}
	}
	t.Fatal("go.mod has no module line")
	return ""
}

// targetModule returns the module of a package under internal/.
func targetModule(target string) (string, bool) {
	parts := strings.Split(target, "/")
	if len(parts) < 2 || parts[0] != "internal" {
		return "", false
	}
	return parts[1], true
}

// checkSite returns what is wrong with one import, or "" when it is allowed.
func checkSite(s importSite) string {
	top, _, _ := strings.Cut(s.target, "/")
	if top == "cmd" {
		return fmt.Sprintf("%s imports %s: nothing under internal/ imports cmd/, which wires the modules", s.file, s.target)
	}
	if top != "internal" {
		if slices.Contains(outsideInternal, top) {
			return ""
		}
		return fmt.Sprintf("%s imports %s: outside internal/, only %v may be imported (outsideInternal)", s.file, s.target, outsideInternal)
	}
	to, _ := targetModule(s.target)
	if to == s.module {
		return ""
	}
	rest := strings.TrimPrefix(strings.TrimPrefix(s.target, "internal/"+to), "/")
	if rest == linkPackage {
		return ""
	}
	if all, shared := sharedRoots[to]; shared && (all || rest == "") {
		return ""
	}
	if s.test && (rest == "" || slices.Contains(testHelpers, to+"/"+rest)) {
		return ""
	}
	if s.test {
		return fmt.Sprintf("%s imports %s: a test may import another module's root package, its link package "+
			"or a helper in testHelpers, never its other packages", s.file, s.target)
	}
	if _, shared := sharedRoots[to]; shared {
		return fmt.Sprintf("%s imports %s: from %s, a shared root, import only its root package (internal/%s)",
			s.file, s.target, to, to)
	}
	return fmt.Sprintf("%s imports %s: from another module, import only its link package (internal/%s/link), "+
		"through an interface declared here and wired in cmd/api", s.file, s.target, to)
}

// compareEdges fails for every observed edge the table lacks and every table
// edge nothing observed has any more.
func compareEdges(t *testing.T, name string, observed map[string]map[string]string, table map[string][]string) {
	t.Helper()
	var problems []string
	for from, tos := range observed {
		for to, example := range tos {
			if !slices.Contains(table[from], to) {
				problems = append(problems, fmt.Sprintf("new module edge %s -> %s (%s): add it to %s on purpose, or remove the import",
					from, to, example, name))
			}
		}
	}
	for from, tos := range table {
		for _, to := range tos {
			if _, ok := observed[from][to]; !ok {
				problems = append(problems, fmt.Sprintf("%s lists %s -> %s, which nothing imports any more: remove it", name, from, to))
			}
		}
	}
	slices.Sort(problems)
	for _, p := range problems {
		t.Error(p)
	}
}

// cyclesOf returns the module-level cycles of a graph: each group of two or
// more modules that all reach each other, sorted and joined by " <-> ".
func cyclesOf(edges map[string]map[string]string) []string {
	var nodes []string
	for from, tos := range edges {
		nodes = append(nodes, from)
		for to := range tos {
			nodes = append(nodes, to)
		}
	}
	slices.Sort(nodes)
	nodes = slices.Compact(nodes)
	reach := map[string]map[string]bool{}
	for _, n := range nodes {
		reach[n] = reachable(edges, n)
	}
	grouped := map[string]bool{}
	var cycles []string
	for _, a := range nodes {
		if grouped[a] {
			continue
		}
		group := []string{a}
		for _, b := range nodes {
			if b != a && reach[a][b] && reach[b][a] {
				group = append(group, b)
			}
		}
		if len(group) > 1 {
			for _, m := range group {
				grouped[m] = true
			}
			cycles = append(cycles, strings.Join(group, " <-> "))
		}
	}
	return cycles
}

// reachable returns the modules reachable from one module, following edges.
func reachable(edges map[string]map[string]string, from string) map[string]bool {
	seen := map[string]bool{}
	stack := []string{from}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for to := range edges[n] {
			if !seen[to] {
				seen[to] = true
				stack = append(stack, to)
			}
		}
	}
	return seen
}
