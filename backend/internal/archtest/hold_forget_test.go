package archtest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// A spend of the master's Inspiration deletes the character's row, and the held roll remembers it (inspiration_holds.favor_spent).
// A hold dropped before it is answered is a roll that never happens, so the Inspiration is lost unless it is given back. There is
// one way to drop a hold: internal/play/hold_forget.go. These tests keep it the only one (docs/architecture/play/favor.md).

// holdForgetFile is the one file that may call the queries that delete holds.
const holdForgetFile = "internal/play/hold_forget.go"

// holdDeleteQueries are the only statements that DELETE FROM inspiration_holds, and the methods they generate.
var holdDeleteQueries = []string{"DeleteInspirationHold", "DeleteInspirationHoldsOf", "DeleteOrphanInspirationHolds"}

var deleteHolds = regexp.MustCompile(`(?i)\bDELETE\s+FROM\s+inspiration_holds\b`)

// TestOnlyTheForgetQueriesDeleteHolds fails for a query in a sqlc query file that deletes from inspiration_holds and is not one of the
// three that hold_forget.go wraps.
func TestOnlyTheForgetQueriesDeleteHolds(t *testing.T) {
	for _, f := range sqlcQueryFiles(t) {
		src, err := os.ReadFile(backendRoot + "/" + f)
		if err != nil {
			t.Fatal(err)
		}
		failProblems(t, holdDeletingQueries(f, string(src)))
	}
}

// holdDeletingQueries names the queries of the file that delete holds and are not allowed to.
func holdDeletingQueries(file, sql string) []string {
	var out []string
	heads := anyHeader.FindAllStringIndex(sql, -1)
	for i, h := range heads {
		end := len(sql)
		if i+1 < len(heads) {
			end = heads[i+1][0]
		}
		block := sql[h[0]:end]
		if !deleteHolds.MatchString(stripCommentsAndStrings(block)) {
			continue
		}
		name := queryHeader.FindStringSubmatch(block)[1]
		if !slices.Contains(holdDeleteQueries, name) {
			out = append(out, file+": "+name+" deletes from inspiration_holds: a hold dropped with no answer loses the Inspiration it spent. "+
				"Drop holds only through hold_forget.go (forgetHoldsOf, forgetHold, forgetOrphanHolds), which gives it back")
		}
	}
	return out
}

// walkBackendGo runs check on every non-test Go file of the backend other than hold_forget.go and collects the problems.
func walkBackendGo(t *testing.T, check func(path string, src []byte) []string) []string {
	t.Helper()
	var problems []string
	root := os.DirFS(backendRoot)
	err := fs.WalkDir(root, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && path != "." && (d.Name() == "node_modules" || d.Name() == "testdata" || strings.HasPrefix(d.Name(), ".")) {
			return fs.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || path == holdForgetFile {
			return nil
		}
		src, err := fs.ReadFile(root, path)
		if err != nil {
			return err
		}
		problems = append(problems, check(path, src)...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return problems
}

// TestHoldsAreDroppedOnlyThroughHoldForget fails for a non-test Go file of the backend, other than hold_forget.go, that calls a generated
// query that deletes holds.
func TestHoldsAreDroppedOnlyThroughHoldForget(t *testing.T) {
	failProblems(t, walkBackendGo(t, callsOfHoldDeletes))
}

// callsOfHoldDeletes lists the calls in the source of a method that deletes holds.
func callsOfHoldDeletes(file string, src []byte) []string {
	return callsOf(file, src, holdDeleteQueries, "a hold dropped with no answer loses the Inspiration it spent. "+
		"Call forgetHoldsOf, forgetHold or forgetOrphanHolds from "+holdForgetFile+" instead")
}

// callsOf lists the calls in the source of any of the methods.
func callsOf(file string, src []byte, methods []string, why string) []string {
	var out []string
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, src, 0)
	if err != nil {
		return []string{file + ": " + err.Error()}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		if sel := selectorOfCall(n); sel != nil && slices.Contains(methods, sel.Sel.Name) {
			out = append(out, file+":"+strconv.Itoa(fset.Position(sel.Pos()).Line)+" calls "+sel.Sel.Name+": "+why)
		}
		return true
	})
	return out
}

// selectorOfCall is the x.Name of a call x.Name(...), nil for any other node.
func selectorOfCall(n ast.Node) *ast.SelectorExpr {
	call, ok := n.(*ast.CallExpr)
	if !ok {
		return nil
	}
	sel, _ := call.Fun.(*ast.SelectorExpr)
	return sel
}

// combatantDeleteQueries is the only statement that may DELETE FROM combatants, and the method it generates. encounters has no
// delete at all: the combat's row goes with its session, and nothing may delete it under another name.
var combatantDeleteQueries = []string{"DeleteCombatant"}

var encounterDeleteQueries []string

var (
	deleteCombatantsSQL = regexp.MustCompile(`(?i)\bDELETE\s+FROM\s+(ONLY\s+)?combatants\b`)
	deleteEncountersSQL = regexp.MustCompile(`(?i)\bDELETE\s+FROM\s+(ONLY\s+)?encounters\b`)
)

// TestOnlyNamedQueriesDeleteCombatantsOrEncounters fails for a query in a sqlc query file that deletes from combatants or encounters and is
// not on the allowlist: the foreign key of inspiration_holds cascades from both, so a delete under another name would take the held
// rolls, and the Inspiration they spent, with it.
func TestOnlyNamedQueriesDeleteCombatantsOrEncounters(t *testing.T) {
	for _, f := range sqlcQueryFiles(t) {
		src, err := os.ReadFile(backendRoot + "/" + f)
		if err != nil {
			t.Fatal(err)
		}
		failProblems(t, cascadingDeletes(f, string(src)))
	}
}

// cascadingDeletes names the queries of the file that delete combatants or encounters and are not allowed to.
func cascadingDeletes(file, sql string) []string {
	var out []string
	heads := anyHeader.FindAllStringIndex(sql, -1)
	for i, h := range heads {
		end := len(sql)
		if i+1 < len(heads) {
			end = heads[i+1][0]
		}
		block := sql[h[0]:end]
		name := queryHeader.FindStringSubmatch(block)[1]
		code := stripCommentsAndStrings(block)
		for _, c := range []struct {
			re      *regexp.Regexp
			table   string
			allowed []string
		}{{deleteCombatantsSQL, "combatants", combatantDeleteQueries}, {deleteEncountersSQL, "encounters", encounterDeleteQueries}} {
			if c.re.MatchString(code) && !slices.Contains(c.allowed, name) {
				out = append(out, file+": "+name+" deletes from "+c.table+": the foreign key would delete the held rolls and the Inspiration they spent. "+
					"Delete a combatant only through deleteCombatant in "+holdForgetFile)
			}
		}
	}
	return out
}

// TestCombatantsAreDeletedOnlyThroughHoldForget fails for a non-test Go file of the backend, other than hold_forget.go, that calls the
// generated DeleteCombatant.
func TestCombatantsAreDeletedOnlyThroughHoldForget(t *testing.T) {
	failProblems(t, walkBackendGo(t, func(path string, src []byte) []string {
		return callsOf(path, src, combatantDeleteQueries, "a combatant deleted without forgetting its holds first loses the Inspiration they spent. "+
			"Call deleteCombatant from "+holdForgetFile+" instead")
	}))
}

// TestTheCombatantDeleteForgetsFirst fails when the function of hold_forget.go that calls DeleteCombatant does not call forgetHoldsOf
// before it (the order matters: the forget reads the rows the delete would take).
func TestTheCombatantDeleteForgetsFirst(t *testing.T) {
	src, err := os.ReadFile(backendRoot + "/" + holdForgetFile)
	if err != nil {
		t.Fatal(err)
	}
	failProblems(t, deletesBeforeForgetting(holdForgetFile, src))
}

// deletesBeforeForgetting lists the functions of the source that call DeleteCombatant with no forgetHoldsOf call before it, and fails
// when no function calls it at all (the one function is the way the rest of the backend deletes combatants).
func deletesBeforeForgetting(file string, src []byte) []string {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, src, 0)
	if err != nil {
		return []string{file + ": " + err.Error()}
	}
	var out []string
	deleters := 0
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		var forgetAt, deleteAt token.Pos
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch fun := call.Fun.(type) {
			case *ast.SelectorExpr:
				if fun.Sel.Name == "DeleteCombatant" && deleteAt == 0 {
					deleteAt = call.Pos()
				}
			case *ast.Ident:
				if fun.Name == "forgetHoldsOf" && forgetAt == 0 {
					forgetAt = call.Pos()
				}
			}
			return true
		})
		if deleteAt == 0 {
			continue
		}
		deleters++
		if forgetAt == 0 || forgetAt > deleteAt {
			out = append(out, file+":"+strconv.Itoa(fset.Position(deleteAt).Line)+" "+fn.Name.Name+" calls DeleteCombatant before (or without) forgetHoldsOf: "+
				"the foreign key would delete the combatant's held rolls and the Inspiration they spent with them")
		}
	}
	if deleters != 1 {
		out = append(out, file+": "+strconv.Itoa(deleters)+" functions call DeleteCombatant, want exactly one (deleteCombatant)")
	}
	return out
}

// The checks above fail on a violation: each one is shown on a source that has it, and passes on one that has not.
func TestTheHoldForgetChecksSeeAViolation(t *testing.T) {
	if got := holdDeletingQueries("q.sql", "-- name: DropHolds :exec\nDELETE FROM inspiration_holds WHERE id = $1;\n"); len(got) != 1 {
		t.Errorf("a query that deletes holds = %v, want one problem", got)
	}
	if got := holdDeletingQueries("q.sql", "-- name: DeleteInspirationHold :many\nWITH c AS (DELETE FROM inspiration_holds WHERE id = $1 RETURNING id)\nSELECT id FROM c ORDER BY id;\n"); len(got) != 0 {
		t.Errorf("the wrapped query = %v, want none", got)
	}
	if got := callsOfHoldDeletes("x.go", []byte("package x\nfunc f() { q.DeleteOrphanInspirationHolds(ctx, id) }\n")); len(got) != 1 {
		t.Errorf("a call of a hold delete = %v, want one problem", got)
	}
	if got := callsOf("x.go", []byte("package x\nfunc f() { q.DeleteCombatant(ctx, id) }\n"), combatantDeleteQueries, "why"); len(got) != 1 {
		t.Errorf("a combatant deleted outside hold_forget.go = %v, want one problem", got)
	}
	if got := cascadingDeletes("q.sql", "-- name: DropCombatants :exec\nDELETE FROM combatants WHERE encounter_id = $1;\n"); len(got) != 1 {
		t.Errorf("a query that deletes combatants under another name = %v, want one problem", got)
	}
	if got := cascadingDeletes("q.sql", "-- name: DropEncounter :exec\nWITH x AS (DELETE FROM encounters WHERE id = $1 RETURNING id) SELECT id FROM x;\n"); len(got) != 1 {
		t.Errorf("a query that deletes encounters = %v, want one problem", got)
	}
	if got := cascadingDeletes("q.sql", "-- name: DeleteCombatant :exec\nDELETE FROM combatants WHERE id = $1;\n-- name: DeleteBattleEncounter :execrows\nDELETE FROM battle_encounters WHERE id = $1;\n"); len(got) != 0 {
		t.Errorf("the named delete and another table = %v, want none", got)
	}
	if got := deletesBeforeForgetting("x.go", []byte("package x\nfunc f() { q.DeleteCombatant(ctx, id) }\n")); len(got) != 1 {
		t.Errorf("a delete with no forget = %v, want one problem", got)
	}
	if got := deletesBeforeForgetting("x.go", []byte("package x\nfunc f() { q.DeleteCombatant(ctx, id); forgetHoldsOf(ctx, q, o) }\n")); len(got) != 1 {
		t.Errorf("a forget after the delete = %v, want one problem", got)
	}
	if got := deletesBeforeForgetting("x.go", []byte("package x\nfunc f() { forgetHoldsOf(ctx, q, o); q.DeleteCombatant(ctx, id) }\nfunc g() { forgetHoldsOf(ctx, q, o); q.DeleteCombatant(ctx, id) }\n")); len(got) != 1 {
		t.Errorf("two functions that delete = %v, want one problem (exactly one function)", got)
	}
	if got := deletesBeforeForgetting("x.go", []byte("package x\nfunc f() { forgetHoldsOf(ctx, q, o); q.DeleteCombatant(ctx, id) }\n")); len(got) != 0 {
		t.Errorf("a combatant deleted after the forget = %v, want none", got)
	}
}

// favorRowFiles are the only files that put a spent Inspiration back: the master's undo of a roll (favor_spend.go, and the undo of a zone's catch in zones_save.go) and the refund path
// (hold_forget.go). The row is one per character and never stacks, so every give-back goes through favorGiveBack and its one-row rule.
var favorRowFiles = []string{"internal/play/favor_spend.go", "internal/play/hold_forget.go", "internal/play/zones_save.go"}

// favorInsertFiles are the only files that insert the character's row of Inspiration (character_favor): the grant, the pass, the undo of
// either and the refund paths. Every other path goes through favorGiveBack, which never stacks a second one.
var favorInsertFiles = []string{
	"internal/play/favor_grant.go", "internal/play/favor_pass.go", "internal/play/combat_undo_favor.go", "internal/play/favor_spend.go", "internal/play/hold_forget.go",
}

// TestFavorGiveBackIsCalledOnlyFromTheRefundPaths fails for a non-test Go file that calls favorGiveBack or favorGiveBackIf outside the files
// that own the refund of a spent Inspiration.
func TestFavorGiveBackIsCalledOnlyFromTheRefundPaths(t *testing.T) {
	failProblems(t, favorFileProblems(t, favorRowFiles, func(path string, src []byte) []string {
		return plainCallsOf(path, src, []string{"favorGiveBack", "favorGiveBackIf"})
	}))
}

// TestInsertCharacterFavorIsCalledOnlyFromTheFavorFiles fails for a non-test Go file that inserts the row of the Inspiration with the raw
// query outside the files that own it.
func TestInsertCharacterFavorIsCalledOnlyFromTheFavorFiles(t *testing.T) {
	failProblems(t, favorFileProblems(t, favorInsertFiles, callsOfRawFavorInsert))
}

// callsOfRawFavorInsert lists the calls of InsertCharacterFavor in the source.
func callsOfRawFavorInsert(path string, src []byte) []string {
	return callsOf(path, src, []string{"InsertCharacterFavor"}, "the character's Inspiration is inserted only by the grant, the pass, the undo of either and the refund paths "+
		"(favor_grant.go, favor_pass.go, combat_undo_favor.go, favor_spend.go, hold_forget.go): use favorGiveBack for a give-back")
}

// TestFavorRowChecksSeeAViolation shows that the two checks above fail for a call outside their files.
func TestFavorRowChecksSeeAViolation(t *testing.T) {
	if got := callsOfRawFavorInsert("x.go", []byte("package x\nfunc f() { q.InsertCharacterFavor(ctx, p) }\n")); len(got) != 1 {
		t.Errorf("a raw insert of the Inspiration = %v, want one problem", got)
	}
	if got := callsOfRawFavorInsert("x.go", []byte("package x\nfunc f() { q.SpendCharacterFavor(ctx, p) }\n")); len(got) != 0 {
		t.Errorf("a spend = %v, want none", got)
	}
	if got := plainCallsOf("x.go", []byte("package x\nfunc f() { favorGiveBackIf(ctx, q, true, a, b, c) }\n"), []string{"favorGiveBack", "favorGiveBackIf"}); len(got) != 1 {
		t.Errorf("a give-back outside its files = %v, want one problem", got)
	}
	problems := favorFileProblems(t, []string{"internal/play/none.go"}, func(path string, _ []byte) []string { return []string{path} })
	if len(problems) == 0 {
		t.Error("the walk of the play package found no file outside the allowed ones")
	}
}

// favorFileProblems runs check on every non-test Go file of the play package that is not one of the allowed files.
func favorFileProblems(t *testing.T, allowed []string, check func(path string, src []byte) []string) []string {
	t.Helper()
	root := os.DirFS(backendRoot)
	var problems []string
	err := fs.WalkDir(root, "internal/play", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || slices.Contains(allowed, path) || strings.Contains(path, "/playdb/") {
			return nil
		}
		src, err := fs.ReadFile(root, path)
		if err != nil {
			return err
		}
		problems = append(problems, check(path, src)...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return problems
}

// plainCallsOf lists the calls of the package-level functions by name (f(...), which callsOf does not read: it reads x.f(...)).
func plainCallsOf(file string, src []byte, funcs []string) []string {
	var out []string
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, src, 0)
	if err != nil {
		return []string{file + ": " + err.Error()}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if id, ok := call.Fun.(*ast.Ident); ok && slices.Contains(funcs, id.Name) {
				out = append(out, file+":"+strconv.Itoa(fset.Position(id.Pos()).Line)+" calls "+id.Name+": an Inspiration spent is given back only through hold_forget.go (refundSpentFavor, forgetHoldsOf) or the undo of its roll")
			}
		}
		return true
	})
	return out
}
