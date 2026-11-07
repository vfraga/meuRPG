// Package safego keeps a panic in a goroutine the server started itself from
// taking the whole process down.
//
// net/http recovers a panic only in the goroutine that runs a handler. A
// goroutine started with `go`, or a timer's callback (time.AfterFunc), that
// panics ends the process, and with it every live session. Those goroutines
// read the database and decode data a user or a model sent.
// Go starts them with Recover deferred inside.
// A timer's callback, and a task that needs its own cleanup, defer Recover.
// The work that panicked is lost, the error is logged with its stack, and the
// server goes on.
package safego

import (
	"context"
	"log/slog"
	"runtime/debug"
)

// Recover is for `defer safego.Recover(logger, "what the goroutine does")`. It
// must be deferred directly (recover only works there). onPanic, when not nil,
// runs after the log line, to fail the job the goroutine was doing.
func Recover(logger *slog.Logger, what string, onPanic ...func()) {
	r := recover()
	if r == nil {
		return
	}
	if logger == nil {
		logger = slog.Default()
	}
	logger.LogAttrs(context.Background(), slog.LevelError, "a background goroutine panicked",
		slog.String("what", what), slog.Any("panic", r), slog.String("stack", string(debug.Stack())))
	for _, f := range onPanic {
		f()
	}
}

// Go starts a goroutine that logs a panic instead of ending the process.
// It runs fn in a new goroutine with Recover deferred directly inside.
// onPanic runs after the log line.
// What must still happen after a panic is deferred inside fn.
func Go(logger *slog.Logger, what string, fn func(), onPanic ...func()) {
	go func() {
		defer Recover(logger, what, onPanic...)
		fn()
	}()
}
