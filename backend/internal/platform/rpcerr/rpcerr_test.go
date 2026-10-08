package rpcerr_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/cockroachdb/cockroach-go/v2/crdb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/PuraFome/meuRPG/backend/internal/platform/rpcerr"
)

func TestFromDB(t *testing.T) {
	t.Parallel()
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name  string
		ctx   context.Context
		err   error
		code  connect.Code
		level string // the level of the log line, "" for none expected to be ERROR
	}{
		{"canceled", context.Background(), fmt.Errorf("query: %w", context.Canceled), connect.CodeCanceled, "DEBUG"},
		{"deadline", context.Background(), fmt.Errorf("query: %w", context.DeadlineExceeded), connect.CodeDeadlineExceeded, "WARN"},
		{"connection broken by a canceled request", canceled, io.ErrUnexpectedEOF, connect.CodeCanceled, "DEBUG"},
		{"retries exhausted", context.Background(), fmt.Errorf("tx: %w", &crdb.MaxRetriesExceededError{}), connect.CodeAborted, "WARN"},
		{"bare 40001", context.Background(), &pgconn.PgError{Code: "40001"}, connect.CodeAborted, "WARN"},
		{"statement timeout", context.Background(), &pgconn.PgError{Code: "57014"}, connect.CodeDeadlineExceeded, "WARN"},
		{"dial failure", context.Background(), &pgconn.ConnectError{}, connect.CodeUnavailable, "ERROR"},
		{"network error", context.Background(), &net.OpError{Op: "read", Err: errors.New("reset")}, connect.CodeUnavailable, "ERROR"},
		{"connection closed", context.Background(), io.EOF, connect.CodeUnavailable, "ERROR"},
		{"node shutting down", context.Background(), &pgconn.PgError{Code: "57P01"}, connect.CodeUnavailable, "ERROR"},
		{"connection exception class", context.Background(), &pgconn.PgError{Code: "08006"}, connect.CodeUnavailable, "ERROR"},
		{"unexpected no rows", context.Background(), pgx.ErrNoRows, connect.CodeInternal, "ERROR"},
		{"anything else", context.Background(), errors.New("SELECT secret FROM boom"), connect.CodeInternal, "ERROR"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var logs bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

			got := rpcerr.FromDB(tt.ctx, logger, "mod", "do it", tt.err)

			if connect.CodeOf(got) != tt.code {
				t.Fatalf("code = %v, want %v", connect.CodeOf(got), tt.code)
			}
			if !strings.Contains(logs.String(), "level="+tt.level) {
				t.Errorf("log = %q, want level %s", logs.String(), tt.level)
			}
			if !strings.Contains(logs.String(), "mod: cannot do it") {
				t.Errorf("the log line does not say what failed: %q", logs.String())
			}
			// The client never sees the driver's text.
			if strings.Contains(got.Error(), "secret") || strings.Contains(got.Error(), "SELECT") {
				t.Errorf("the client would see the cause: %v", got)
			}
		})
	}
}

func TestFromDBPassesAConnectErrorUntouchedAndSilently(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	in := connect.NewError(connect.CodeNotFound, errors.New("nothing here"))

	got := rpcerr.FromDB(t.Context(), logger, "mod", "x", fmt.Errorf("wrapped: %w", in))

	if got != in { //nolint:errorlint // identity is the point
		t.Errorf("got %v, want the same error", got)
	}
	if logs.Len() != 0 {
		t.Errorf("a chosen answer must not be logged: %q", logs.String())
	}
}

type releaseFailsTx struct{}

func (releaseFailsTx) Exec(_ context.Context, q string, _ ...interface{}) error {
	if strings.HasPrefix(q, "RELEASE") {
		return io.EOF // the connection dropped during COMMIT
	}
	return nil
}
func (releaseFailsTx) Commit(context.Context) error   { return nil }
func (releaseFailsTx) Rollback(context.Context) error { return nil }

// A commit whose outcome is unknown must not be reported as a safe-to-retry
// "unavailable": the write may have been applied and a retry would duplicate.
func TestFromDBDoesNotCallAnAmbiguousCommitRetryable(t *testing.T) {
	t.Parallel()
	err := crdb.ExecuteInTx(context.Background(), releaseFailsTx{}, func() error { return nil })
	if _, ok := errors.AsType[*crdb.AmbiguousCommitError](err); !ok {
		t.Fatalf("setup: want AmbiguousCommitError, got %T", err)
	}
	got := rpcerr.FromDB(context.Background(), slog.New(slog.DiscardHandler), "m", "save", err)
	if code := connect.CodeOf(got); code != connect.CodeUnknown {
		t.Fatalf("ambiguous commit reported as %v (%v), want unknown: clients retry unavailable and may duplicate the write", code, got)
	}
}

func TestFromDBKeepsTheCauseForTheTransactionRetry(t *testing.T) {
	t.Parallel()
	got := rpcerr.FromDB(t.Context(), slog.New(slog.DiscardHandler), "mod", "read", fmt.Errorf("read: %w", &pgconn.PgError{Code: "40001", Message: "SELECT secret"}))

	if _, ok := errors.AsType[*pgconn.PgError](got); !ok {
		t.Error("the cause is out of reach: db.InTx would not see the 40001 and would not retry")
	}
	if strings.Contains(got.Error(), "secret") {
		t.Errorf("the client would see the cause: %v", got)
	}
}
