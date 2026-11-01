package play

import (
	"os"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
)

// Roster reads that the characters module runs inside play's db.InTx closure
// (GetVitalsTx, CombatSheet, ...) must keep a 40001 in reach of db.InTx, so the
// transaction is retried. A plain tx read (the control) is retried and succeeds.

// injectedRetry runs read inside db.InTx with a 40001 injected on the first
// attempt only, and returns the number of attempts and the error.
func injectedRetry(t *testing.T, a *armed, read func(tx pgx.Tx) error) (int, error) {
	t.Helper()
	ctx := t.Context()
	if _, err := a.h.pool.Exec(ctx, `SET inject_retry_errors_enabled = true`); err != nil {
		t.Skipf("cannot inject retry errors: %v", err)
	}
	t.Cleanup(func() { _, _ = a.h.pool.Exec(ctx, `SET inject_retry_errors_enabled = false`) })
	attempts := 0
	err := db.InTx(ctx, a.h.pool, func(tx pgx.Tx) error {
		attempts++
		if attempts > 1 {
			if _, err := tx.Exec(ctx, `SET inject_retry_errors_enabled = false`); err != nil {
				return err
			}
		}
		return read(tx)
	})
	return attempts, err
}

func TestRosterReadInsideTxIsRetriedOn40001(t *testing.T) {
	a := newSummoners(t)
	ctx := t.Context()

	n, err := injectedRetry(t, a, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `SELECT 1`)
		return err
	})
	if err != nil || n != 2 {
		t.Fatalf("control (plain tx read): err = %v after %d attempts, want one retry and success", err, n)
	}

	n, err = injectedRetry(t, a, func(tx pgx.Tx) error {
		_, err := a.h.chars.GetVitalsTx(ctx, tx, a.campaignID, a.bri.GetId())
		return err
	})
	if err != nil || n != 2 {
		t.Errorf("GetVitalsTx inside InTx: err = %v (code %v) after %d attempt(s); want the 40001 retried (2 attempts) and no error", err, connect.CodeOf(err), n)
	}
}

// Same shape with a REAL conflict (no injection): a second connection with
// HIGH priority writes a row this tx has an intent on, aborting it; the tx's
// next statement (the roster read) gets 40001.
func TestRosterReadInsideTxIsRetriedOnRealConflict(t *testing.T) {
	a := newSummoners(t)
	ctx := t.Context()
	url := os.Getenv("MEURPG_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("no MEURPG_TEST_DATABASE_URL")
	}
	if _, err := a.h.pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS roster_retry_conflict (id INT PRIMARY KEY, v INT)`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = a.h.pool.Exec(ctx, `DROP TABLE IF EXISTS roster_retry_conflict`) })
	_, _ = a.h.pool.Exec(ctx, `UPSERT INTO roster_retry_conflict VALUES (1, 0)`)
	var dbName string
	if err := a.h.pool.QueryRow(ctx, `SELECT current_database()`).Scan(&dbName); err != nil {
		t.Fatal(err)
	}
	cfg, err := pgx.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Database = dbName
	other, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Skipf("second connection: %v", err)
	}
	defer other.Close(ctx)

	run := func(read func(tx pgx.Tx) error) (int, error) {
		attempts := 0
		err := db.InTx(ctx, a.h.pool, func(tx pgx.Tx) error {
			attempts++
			if _, err := tx.Exec(ctx, `UPDATE roster_retry_conflict SET v = v + 1 WHERE id = 1`); err != nil {
				return err
			}
			if attempts == 1 {
				otx, err := other.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := otx.Exec(ctx, `SET TRANSACTION PRIORITY HIGH`); err != nil {
					t.Fatal(err)
				}
				if _, err := otx.Exec(ctx, `UPDATE roster_retry_conflict SET v = 100 WHERE id = 1`); err != nil {
					t.Fatal(err)
				}
				if err := otx.Commit(ctx); err != nil {
					t.Fatal(err)
				}
				// the tx's heartbeat (1s) finds its record aborted, so the
				// NEXT statement (the read under test) is the one that fails.
				time.Sleep(2500 * time.Millisecond)
			}
			return read(tx)
		})
		return attempts, err
	}
	n, err := run(func(tx pgx.Tx) error { _, err := tx.Exec(ctx, `SELECT 1`); return err })
	if err != nil || n < 2 {
		t.Skipf("control did not hit a real conflict: err = %v attempts = %d", err, n)
	}
	n, err = run(func(tx pgx.Tx) error {
		_, err := a.h.chars.GetVitalsTx(ctx, tx, a.campaignID, a.bri.GetId())
		return err
	})
	if err != nil || n < 2 {
		t.Errorf("GetVitalsTx inside InTx, real conflict: err = %v (code %v) after %d attempt(s); want retried and no error", err, connect.CodeOf(err), n)
	}
}
