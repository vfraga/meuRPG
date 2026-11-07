package archtest

import (
	"io/fs"
	"os"
	"slices"
	"strings"
	"testing"
)

// sqlcConfig is backend/sqlc.yaml, seen from this package's directory.
const sqlcConfig = backendRoot + "/sqlc.yaml"

// sqlcQueryFiles returns the query files that sqlc reads, as paths relative
// to backend/, in the order of sqlc.yaml.
// The queries: entries are read from the config, so this list cannot drift from the generated code.
func sqlcQueryFiles(t *testing.T) []string {
	t.Helper()
	cfg, err := os.ReadFile(sqlcConfig)
	if err != nil {
		t.Fatalf("reading %s: %v", sqlcConfig, err)
	}
	files := parseSQLCQueries(string(cfg))
	if len(files) == 0 {
		t.Fatalf("%s has no queries: entry", sqlcConfig)
	}
	return files
}

// parseSQLCQueries reads every "queries:" entry of a sqlc.yaml, a single path or a list.
func parseSQLCQueries(cfg string) []string {
	var files []string
	inList := false
	listIndent := 0
	for line := range strings.Lines(cfg) {
		line = strings.TrimRight(line, "\r\n")
		trimmed := strings.TrimSpace(line)
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if inList {
			if item, ok := strings.CutPrefix(trimmed, "- "); ok && indent >= listIndent {
				files = append(files, cleanYAMLScalar(item))
				continue
			}
			if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
				inList = false
			} else {
				continue
			}
		}
		key := strings.TrimPrefix(trimmed, "- ")
		if rest, ok := strings.CutPrefix(key, "queries:"); ok {
			rest = cleanYAMLScalar(rest)
			if rest == "" {
				inList, listIndent = true, indent+len(trimmed)-len(key)
			} else {
				files = append(files, rest)
			}
		}
	}
	return files
}

// cleanYAMLScalar drops a trailing comment and the quotes around a scalar.
func cleanYAMLScalar(s string) string {
	if i := strings.Index(s, " #"); i >= 0 {
		s = s[:i]
	}
	return strings.Trim(strings.TrimSpace(s), `"'`)
}

// moduleOfQueryFile is the module of a query file: the directory under internal/.
func moduleOfQueryFile(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) < 3 || parts[0] != "internal" {
		return ""
	}
	return parts[1]
}

// TestEverySQLFileIsReadBySqlc fails for a .sql file under internal/ that
// sqlc.yaml does not list, because the SQL tests would never see its queries.
func TestEverySQLFileIsReadBySqlc(t *testing.T) {
	listed := sqlcQueryFiles(t)
	var onDisk []string
	err := fs.WalkDir(os.DirFS(backendRoot), "internal", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && skipDirName(d.Name()) {
			return fs.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(path, ".sql") {
			onDisk = append(onDisk, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range onDisk {
		if !slices.Contains(listed, f) {
			t.Errorf("%s is not in the queries: of backend/sqlc.yaml: list it there so sqlc generates its code "+
				"and the SQL architecture tests read it", f)
		}
	}
	for _, f := range listed {
		if _, err := os.Stat(backendRoot + "/" + f); err != nil {
			t.Errorf("backend/sqlc.yaml lists %s, which cannot be read: %v", f, err)
		}
		if moduleOfQueryFile(f) == "" {
			t.Errorf("backend/sqlc.yaml lists %s, which is not under internal/<module>/", f)
		}
	}
	t.Logf("%d .sql files under internal/ checked against the %d queries: entries of sqlc.yaml", len(onDisk), len(listed))
}

func TestParseSQLCQueries(t *testing.T) {
	cfg := "sql:\n" +
		"  - queries: internal/a/queries.sql\n" +
		"    gen: {}\n" +
		"  - queries: \"internal/b/q.sql\"  # note\n" +
		"  - queries:\n" +
		"      - internal/c/queries.sql\n" +
		"      # comment\n" +
		"      - internal/c/more.sql\n" +
		"    gen:\n" +
		"      go: {}\n"
	want := []string{"internal/a/queries.sql", "internal/b/q.sql", "internal/c/queries.sql", "internal/c/more.sql"}
	if got := parseSQLCQueries(cfg); !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if got := moduleOfQueryFile("internal/play/combat_reserve.sql"); got != "play" {
		t.Errorf("module = %q", got)
	}
}
