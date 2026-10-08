// Package slowclient keeps a slow or stalled client from holding a connection
// and a goroutine for as long as Cloud Run lets a request live (35 minutes).
//
// The HTTP server has no global ReadTimeout or WriteTimeout, on purpose: they
// would cut the live streams short (platform/httpserver). The limits go on the
// one operation that can stall instead:
//
//   - a write on a live stream: Interceptor gives each Send a deadline, so a
//     reader that stopped reading (a laptop that went to sleep, a dead mobile
//     connection that never closed) ends the stream instead of blocking inside
//     Send until the platform closes the connection;
//   - the read of a request body that a client can trickle: ReadBody, for the
//     image upload and the sign-in form;
//   - the write of a large response on a plain route: WriteBody, for the
//     image, thumbnail and tile downloads and the app's files.
package slowclient

import (
	"context"
	"net/http"
	"time"

	"connectrpc.com/connect"
)

type controllerKey struct{}

// Middleware puts the request's http.ResponseController in its context. Connect
// does not hand the handler the ResponseWriter, and the write deadline can only
// be set through it, so the Interceptor finds it here.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), controllerKey{}, http.NewResponseController(w))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ReadBody gives the rest of the request body d to arrive. Call it before
// reading the body, in a handler that is not a stream. A writer that cannot set
// deadlines (a test's recorder) is left alone.
func ReadBody(w http.ResponseWriter, d time.Duration) {
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(d))
}

// WriteBody gives the response body d to be written: a client that stops
// reading ends the write with an error instead of holding the handler until
// the platform closes the connection. Call it before writing, in a handler
// that is not a stream, and call the function it returns when the handler is
// done: without a WriteTimeout on the server, the deadline would otherwise
// stay on a kept-alive connection for the next request. A writer that cannot
// set deadlines (a test's recorder) is left alone.
func WriteBody(w http.ResponseWriter, d time.Duration) (done func()) {
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Now().Add(d))
	return func() { _ = rc.SetWriteDeadline(time.Time{}) }
}

// Interceptor returns a Connect interceptor that gives every message a
// streaming handler sends d to be written. The deadline is set before each Send
// and cleared after it, so the quiet time between two messages (a stream waits
// for events) never counts. A Send that hits the deadline fails like one to a
// client that left, and the handler ends the stream.
func Interceptor(d time.Duration) connect.Interceptor {
	return &interceptor{d: d}
}

type interceptor struct{ d time.Duration }

func (*interceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc { return next }

func (*interceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (i *interceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		rc, ok := ctx.Value(controllerKey{}).(*http.ResponseController)
		if !ok {
			return next(ctx, conn) // no Middleware in front: nothing to set
		}
		return next(ctx, &deadlineConn{StreamingHandlerConn: conn, rc: rc, d: i.d})
	}
}

// deadlineConn bounds each Send.
type deadlineConn struct {
	connect.StreamingHandlerConn
	rc *http.ResponseController
	d  time.Duration
}

// Send implements connect.StreamingHandlerConn.
func (c *deadlineConn) Send(msg any) error {
	// An error here means the connection cannot take deadlines (a test server
	// over a pipe, say); the Send then runs as before.
	_ = c.rc.SetWriteDeadline(time.Now().Add(c.d))
	err := c.StreamingHandlerConn.Send(msg)
	if err == nil {
		_ = c.rc.SetWriteDeadline(time.Time{})
	}
	return err
}
