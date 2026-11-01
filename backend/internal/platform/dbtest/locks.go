package dbtest

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// A test that proves a request is stopped at a lock waits, on a condition and not on a clock, until the statement
// is waiting. The servers show it in different views (PostgreSQL's pg_stat_activity, CockroachDB's crdb_internal and
// SHOW CLUSTER STATEMENTS), and these helpers hide the difference.

// Querier is what a pool, a connection or a transaction of pgx offers.
type Querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func onCockroach(ctx context.Context, q Querier) (bool, error) {
	var version string
	if err := q.QueryRow(ctx, `SELECT version()`).Scan(&version); err != nil {
		return false, fmt.Errorf("ask the server its version: %w", err)
	}
	return strings.Contains(version, "CockroachDB"), nil
}

// BackendID names the server's session that q's next statement runs on: the session id on CockroachDB, the process id
// on PostgreSQL. A transaction (or one connection) keeps it for its statements. Pass it to WaitingStatements.
func BackendID(ctx context.Context, q Querier) (string, error) {
	cockroach, err := onCockroach(ctx, q)
	if err != nil {
		return "", err
	}
	query := `SELECT pg_backend_pid()::text`
	if cockroach {
		query = `SHOW session_id`
	}
	var id string
	if err := q.QueryRow(ctx, query).Scan(&id); err != nil {
		return "", fmt.Errorf("read the session id: %w", err)
	}
	return id, nil
}

// WaitingStatements counts the statements other than the observer's own whose text matches like (a SQL LIKE pattern)
// and that wait for a lock: on PostgreSQL the ones that wait on a lock now, on CockroachDB the ones that are running.
// With backends (BackendID values) it counts only the statements of those sessions.
func WaitingStatements(ctx context.Context, observer Querier, like string, backends ...string) (int, error) {
	cockroach, err := onCockroach(ctx, observer)
	if err != nil {
		return 0, err
	}
	var n int
	switch {
	case cockroach && len(backends) > 0:
		err = observer.QueryRow(ctx, `SELECT count(*) FROM [SHOW CLUSTER STATEMENTS] WHERE session_id = ANY($1) AND query LIKE $2 AND query NOT LIKE '%CLUSTER STATEMENTS%'`, backends, like).Scan(&n)
	case cockroach:
		err = observer.QueryRow(ctx, `SELECT count(*) FROM crdb_internal.cluster_queries WHERE query LIKE $1 AND query NOT LIKE '%cluster_queries%'`, like).Scan(&n)
	case len(backends) > 0:
		err = observer.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE wait_event_type = 'Lock' AND pid::text = ANY($1) AND query LIKE $2 AND pid <> pg_backend_pid()`, backends, like).Scan(&n)
	default:
		err = observer.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE wait_event_type = 'Lock' AND datname = current_database() AND query LIKE $1 AND pid <> pg_backend_pid()`, like).Scan(&n)
	}
	if err != nil {
		return 0, fmt.Errorf("look for the waiting statements: %w", err)
	}
	return n, nil
}
