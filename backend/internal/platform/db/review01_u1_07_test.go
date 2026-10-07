package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Finding U1-07
func TestReview1_ReadTxStaysReadOnlyAfterRetry(t *testing.T) {
	pool := testPool(t)
	ctx := t.Context()
	table := fmt.Sprintf("review1_u107_%d", time.Now().UnixNano())
	if _, err := pool.Exec(ctx, "CREATE TABLE "+table+" (id INT PRIMARY KEY, n INT)"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), "DROP TABLE "+table) })

	attempts := 0
	var writeErr error
	err := ReadTx(ctx, pool, func(tx pgx.Tx) error {
		attempts++
		if attempts == 1 {
			return &pgconn.PgError{Code: "40001", Message: "forced retry"}
		}
		_, writeErr = tx.Exec(ctx, "INSERT INTO "+table+" VALUES (1, 1)")
		return writeErr
	})
	t.Logf("attempts=%d err=%v writeErr=%v", attempts, err, writeErr)
	if attempts < 2 {
		t.Fatalf("retry did not happen (attempts=%d)", attempts)
	}
	if err == nil || writeErr == nil {
		t.Errorf("write inside ReadTx on retry attempt succeeded; want read-only error")
	}
	var c int
	if e := pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&c); e != nil {
		t.Fatal(e)
	}
	if c != 0 {
		t.Errorf("rows committed by ReadTx = %d, want 0", c)
	}
}
