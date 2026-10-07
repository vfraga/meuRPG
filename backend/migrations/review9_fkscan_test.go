package migrations

import (
	"fmt"
	"strings"
	"testing"
)

func review9Migrated(t *testing.T) func(q string) []string {
	t.Helper()
	sdb := freshDatabase(t)
	provider, err := NewProvider(sdb)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if _, err := provider.Up(t.Context()); err != nil {
		t.Fatalf("Up: %v", err)
	}
	return func(q string) []string {
		t.Helper()
		rows, err := sdb.QueryContext(t.Context(), q)
		if err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				t.Fatalf("scan: %v", err)
			}
			out = append(out, s)
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("rows: %v", err)
		}
		return out
	}
}

// Finding U9-14: deleting a characters/users/combatants row full-scans session_events / pending_damages through FK cascades with no child index.
func TestReview9_DeleteParentDoesNotFullScanBigChild(t *testing.T) {
	t.Parallel()
	run := review9Migrated(t)
	const id = "'00000000-0000-0000-0000-000000000001'"
	for _, table := range []string{"characters", "users", "combatants"} {
		lines := run(fmt.Sprintf("SELECT info FROM [EXPLAIN DELETE FROM %s WHERE id = %s]", table, id))
		for i, l := range lines {
			if !strings.Contains(l, "fk-cascade") && !strings.Contains(l, "fk-set-null") {
				continue
			}
			// This FK action's lines, up to the next FK action.
			block := []string{l}
			for _, n := range lines[i+1:] {
				if strings.Contains(n, "• fk-") {
					break
				}
				block = append(block, n)
			}
			for j, bl := range block {
				if !strings.Contains(bl, "table: session_events@") && !strings.Contains(bl, "table: pending_damages@") {
					continue
				}
				for _, nx := range block[j+1 : min(j+3, len(block))] {
					if strings.Contains(nx, "FULL SCAN") {
						t.Errorf("DELETE FROM %s: %s then %s", table, strings.Join(strings.Fields(block[1]), " "), strings.Join(strings.Fields(bl), " ")+" / FULL SCAN")
					}
				}
			}
		}
	}
}

// Finding U9-14: every CASCADE/SET NULL FK child column on a big table should be indexed; lists those that are not.
func TestReview9_CascadeFKChildColumnsOnBigTablesAreIndexed(t *testing.T) {
	t.Parallel()
	run := review9Migrated(t)
	rows := run(`
		SELECT c.conrelid::regclass::string || '.' || a.attname || ' (' || c.conname || ')'
		FROM pg_constraint c
		JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = c.conkey[1]
		WHERE c.contype = 'f' AND c.confdeltype IN ('c','n','d')
		  AND NOT EXISTS (
		    SELECT 1 FROM pg_index i
		    WHERE i.indrelid = c.conrelid AND i.indkey[0] = c.conkey[1])
		ORDER BY 1`)
	big := []string{"session_events", "pending_damages", "combatants", "gallery", "image"}
	var bad []string
	for _, r := range rows {
		for _, b := range big {
			if strings.HasPrefix(r, b) || strings.Contains(r, "."+b) {
				bad = append(bad, r)
				break
			}
		}
	}
	t.Logf("all unindexed cascade/set-null FK columns (%d): %s", len(rows), strings.Join(rows, "; "))
	if len(bad) > 0 {
		t.Errorf("cascade FK child columns on big tables without an index: %s", strings.Join(bad, "; "))
	}
}
