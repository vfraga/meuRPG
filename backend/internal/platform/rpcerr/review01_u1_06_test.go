package rpcerr_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/cockroachdb/cockroach-go/v2/crdb"

	"github.com/PuraFome/meuRPG/backend/internal/platform/rpcerr"
)

type review1ReleaseFailsTx struct{}

func (review1ReleaseFailsTx) Exec(_ context.Context, q string, _ ...interface{}) error {
	if strings.HasPrefix(q, "RELEASE") {
		return io.EOF // the connection dropped during COMMIT
	}
	return nil
}
func (review1ReleaseFailsTx) Commit(context.Context) error   { return nil }
func (review1ReleaseFailsTx) Rollback(context.Context) error { return nil }

// Finding U1-06
// A commit whose outcome is unknown must not be reported as a safe-to-retry
// "unavailable": the write may have been applied and a retry would duplicate.
func TestReview1_AmbiguousCommit(t *testing.T) {
	t.Parallel()
	err := crdb.ExecuteInTx(context.Background(), review1ReleaseFailsTx{}, func() error { return nil })
	if _, ok := errors.AsType[*crdb.AmbiguousCommitError](err); !ok {
		t.Fatalf("setup: want AmbiguousCommitError, got %T", err)
	}
	got := rpcerr.FromDB(context.Background(), slog.New(slog.DiscardHandler), "m", "save", err)
	if code := connect.CodeOf(got); code == connect.CodeUnavailable {
		t.Fatalf("ambiguous commit reported as %v (%v): clients retry it and may duplicate the write", code, got)
	}
}
