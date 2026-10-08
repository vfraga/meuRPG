// Package rpcerr turns an error from the database (or from inside a
// transaction) into the Connect error a client gets, and logs the cause once.
//
// Every service used to carry its own copy of this, and each copy answered
// `unavailable: cannot reach the database` to everything it did not know: a
// request the browser canceled, a statement that ran too long, a programming
// error. Clients retry `unavailable`, and the log showed an ERROR for what was
// only a closed tab. One place now decides the code, so the mapping is the same
// in every module and is tested once.
package rpcerr

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"

	"connectrpc.com/connect"
	"github.com/cockroachdb/cockroach-go/v2/crdb"
	"github.com/jackc/pgx/v5/pgconn"
)

// SQLSTATEs that need a name. 57014 is query_canceled, which is what
// CockroachDB sends when `statement_timeout` fires.
const (
	sqlstateSerialization = "40001"
	sqlstateQueryCanceled = "57014"
	sqlstateAdminShutdown = "57P01" // the node is being shut down
	sqlstateCannotConnect = "57P03" // the node is starting up or draining
)

// FromDB maps err to the Connect error to return:
//
//   - a *connect.Error passes as it is: it is the answer the handler chose;
//   - the request was canceled (the client left) -> canceled, logged at DEBUG;
//   - the request's deadline passed, or the database canceled a statement
//     for taking too long -> deadline_exceeded, WARN;
//   - the transaction was retried on 40001 until it gave up -> aborted, WARN
//     (the client may retry);
//   - a commit whose outcome is unknown (the connection dropped during COMMIT)
//     -> unknown, ERROR: the change may have been saved, so the client must not
//     be told to retry blindly;
//   - the database cannot be reached -> unavailable, ERROR;
//   - anything else is a bug or an unexpected failure -> internal, ERROR.
//
// The client only gets a fixed sentence, never the error's text (a driver
// message can carry SQL or a host name); the cause is in the log line.
//
// The returned error keeps err as its cause (errors.As and errors.Is reach it,
// its text stays the fixed sentence). A method that runs inside another
// module's db.InTx may return it from the closure: InTx still sees a 40001 and
// retries, instead of answering `aborted` for a conflict a second attempt
// survives.
func FromDB(ctx context.Context, logger *slog.Logger, module, action string, err error) error {
	if connectErr, ok := errors.AsType[*connect.Error](err); ok {
		return connectErr
	}
	code, level, message := classify(ctx, err)
	if logger == nil {
		logger = slog.Default()
	}
	logger.Log(ctx, level, module+": cannot "+action, "error", err, "code", code.String()) //nolint:sloglint // each call site passes a fixed module and action
	return connect.NewError(code, &causedError{message: message, cause: err})
}

// causedError is the error a client gets: the fixed message, with the original
// error one Unwrap away for the code that decides on retries.
type causedError struct {
	message string
	cause   error
}

func (e *causedError) Error() string { return e.message }
func (e *causedError) Unwrap() error { return e.cause }

func classify(ctx context.Context, err error) (connect.Code, slog.Level, string) {
	// A context error wins over what it wraps: pgx wraps it in its own error
	// when the request goes away in the middle of a query.
	switch {
	case errors.Is(err, context.Canceled):
		return connect.CodeCanceled, slog.LevelDebug, "the request was canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return connect.CodeDeadlineExceeded, slog.LevelWarn, "the request took too long, please try again"
	}
	if _, ok := errors.AsType[*crdb.AmbiguousCommitError](err); ok {
		// The connection broke while committing: the change may or may not
		// have been saved. The error unwraps to the connection failure, so it
		// is tested before that (a retry could duplicate a write).
		return connect.CodeUnknown, slog.LevelError, "the change may or may not have been saved, check before trying again"
	}
	if ctx.Err() != nil && isConnectionFailure(err) {
		// The connection broke because the request was canceled underneath it.
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return connect.CodeDeadlineExceeded, slog.LevelWarn, "the request took too long, please try again"
		}
		return connect.CodeCanceled, slog.LevelDebug, "the request was canceled"
	}

	if _, ok := errors.AsType[*crdb.MaxRetriesExceededError](err); ok {
		// db.InTx retried a serialization conflict (40001) and gave up.
		return connect.CodeAborted, slog.LevelWarn, "too many changes at the same time, please try again"
	}
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		switch pgErr.Code {
		case sqlstateSerialization:
			return connect.CodeAborted, slog.LevelWarn, "too many changes at the same time, please try again"
		case sqlstateQueryCanceled:
			return connect.CodeDeadlineExceeded, slog.LevelWarn, "the request took too long, please try again"
		}
	}
	if isConnectionFailure(err) {
		return connect.CodeUnavailable, slog.LevelError, "cannot reach the database right now, please try again"
	}
	return connect.CodeInternal, slog.LevelError, "internal error"
}

// isConnectionFailure is true when the database could not be reached or the
// connection broke: a failed dial, a network error, a connection closed in the
// middle, or a node that is shutting down or starting (SQLSTATE class 08).
func isConnectionFailure(err error) bool {
	if _, ok := errors.AsType[*pgconn.ConnectError](err); ok {
		return true
	}
	if _, ok := errors.AsType[net.Error](err); ok {
		return true
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		switch {
		case len(pgErr.Code) >= 2 && pgErr.Code[:2] == "08":
			return true
		case pgErr.Code == sqlstateAdminShutdown, pgErr.Code == sqlstateCannotConnect:
			return true
		}
	}
	return false
}
