package dbtest

// EXPERIMENT (exp/postgres-tests): times the kit's steps on whichever server
// the URL names. MEURPG_EXPERIMENT_TIMING=1 turns it on.

import (
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/PuraFome/meuRPG/backend/migrations"
)

func TestExperimentTimings(t *testing.T) {
	if os.Getenv("MEURPG_EXPERIMENT_TIMING") != "1" {
		t.Skip("experiment only")
	}
	raw := os.Getenv(EnvVar)
	ctx := t.Context()
	admin, err := sql.Open("pgx", raw)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	pg, err := isPostgres(ctx, admin)
	if err != nil {
		t.Fatal(err)
	}

	// 1. one fresh migrate of every migration
	name := fmt.Sprintf("exp_fresh_%d", time.Now().UnixNano())
	start := time.Now()
	mustExec(t, admin, "CREATE DATABASE "+name)
	dsn, _ := withDatabase(raw, name)
	fresh := open(t, dsn)
	if pg {
		if _, err := fresh.ExecContext(ctx, migrations.PostgresPrelude); err != nil {
			t.Fatal(err)
		}
	}
	mk := migrations.NewProvider
	if pg {
		mk = migrations.NewPostgresProvider
	}
	p, err := mk(fresh)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Up(ctx); err != nil {
		t.Fatal(err)
	}
	t.Logf("TIMING fresh migrate of every migration: %v", time.Since(start))
	_ = fresh.Close()
	start = time.Now()
	mustExec(t, admin, "DROP DATABASE "+name)
	t.Logf("TIMING drop of that database: %v", time.Since(start))

	// 2. the template (it may exist already: then this only reads it)
	start = time.Now()
	tpl, err := buildTemplate(raw)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("TIMING buildTemplate (migrate + read the copy statements; includes waiting if it exists): %v", time.Since(start))

	// 3. copies and a DELETE-reuse
	for i := range 3 {
		n := fmt.Sprintf("exp_copy_%d_%d", time.Now().UnixNano(), i)
		cdsn, _ := withDatabase(raw, n)
		start = time.Now()
		if tpl.pg {
			err = pgClone(ctx, admin, tpl, n, cdsn)
		} else {
			err = fmt.Errorf("cockroach copy is timed through NewPool below")
		}
		if err == nil {
			t.Logf("TIMING copy %d of the template (CREATE + schema + reference rows): %v", i, time.Since(start))
			start = time.Now()
			mustExec(t, admin, "DROP DATABASE "+n+" WITH (FORCE)")
			t.Logf("TIMING drop of the copy: %v", time.Since(start))
		}
	}
	start = time.Now()
	_ = NewPoolConns(t, "exp", 1)
	t.Logf("TIMING NewPoolConns, first call (a new database from the template): %v", time.Since(start))
}
