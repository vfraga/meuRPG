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
//     image upload, the sign-in form and the campaign package's parts (the
//     body clears the deadline itself when it ends), and
//     Middleware, for every unary RPC (UnaryBodyTimeout);
//   - the write of a large response on a plain route: WriteBody, for the
//     image, thumbnail and tile downloads and the app's files.
package slowclient

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"connectrpc.com/connect"
)

// UnaryBodyTimeout is how long the body of a unary RPC has to arrive. A body is
// at most its procedure's read limit (platform/reqlimit: 64 KiB by default, 4 MiB
// for the largest raised procedure), which a 1 Mbit/s connection sends in about 35 s.
const UnaryBodyTimeout = time.Minute

type controllerKey struct{}

// Middleware puts the request's http.ResponseController in its context. Connect
// does not hand the handler the ResponseWriter, and the write deadline can only
// be set through it, so the Interceptor finds it here.
func Middleware(next http.Handler) http.Handler {
	return MiddlewareWith(UnaryBodyTimeout)(next)
}

// MiddlewareWith is Middleware with another unary body deadline.
func MiddlewareWith(unaryBody time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rc := http.NewResponseController(w)
			ctx := context.WithValue(r.Context(), controllerKey{}, rc)
			if IsUnaryRPC(r) {
				// Set before the handler reads: Connect reads a unary body, up to its
				// whole limit, before any interceptor (the session's included) runs.
				if rc.SetReadDeadline(time.Now().Add(unaryBody)) == nil {
					r.Body = &deadlineBody{ReadCloser: r.Body, rc: rc}
				}
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ReadBody gives the request body d to arrive, and the body disarms the
// deadline itself: the first Read that returns io.EOF ends it. Call it before
// reading the body, in a handler that is not a stream, before any refusal that
// returns with the body unread (a missing session, a rate limit, a wrong media
// type, a form that is wrong): such a path leaves the deadline armed, with no
// call to remember. A handler that limits the body wraps it first, so that the
// deadline body stays the outermost one (see below).
//
// The deadline is what ends a client that goes on trickling after the handler
// has answered: the server has no ReadTimeout, and when a handler returns with
// the body unread, net/http discards what is left (up to 256 KB, or the whole
// of a chunked body) under whatever deadline the connection still has. Cleared,
// a trickler would hold the connection and the goroutine for as long as Cloud
// Run lets a request live. Armed, the discard ends with the deadline.
//
// At the end of the body the deadline has nothing left to bound, and it is
// harmful: net/http starts a background read there (how it notices a client
// that left), the deadline fires on that read, and the request's context is
// canceled while the handler still decodes or stores what it read. net/http
// itself clears the connection's deadline when it starts that read; what the
// body adds is the record that it ended, which keeps a later ReadBody from
// arming a deadline after that point. Only the end of the body ends it: an
// error (a body over its limit, a deadline that fired) leaves it armed.
//
// Called again on the same request while the body is still unread, ReadBody
// gives the rest of the body a new d, which a handler uses when a wait that is
// not the client's (a queue for a shared slot) must not count against it; once
// the body has ended, a second call does nothing. A request with no body, and a
// writer that cannot set deadlines (a test's recorder), are left alone.
func ReadBody(w http.ResponseWriter, r *http.Request, d time.Duration) {
	if r.Body == nil || r.Body == http.NoBody {
		return
	}
	if b, ok := r.Body.(*readBody); ok {
		b.arm(d)
		return
	}
	b := &readBody{ReadCloser: r.Body, rc: http.NewResponseController(w)}
	b.arm(d)
	r.Body = b
}

// readBody is the body ReadBody installs. It is read by one goroutine at a time
// (the handler's, or the one a store hands the reader to), and ReadBody's
// re-arm runs on the handler's, between reads.
type readBody struct {
	io.ReadCloser
	rc    *http.ResponseController
	ended bool
}

func (b *readBody) arm(d time.Duration) {
	if !b.ended {
		_ = b.rc.SetReadDeadline(time.Now().Add(d))
	}
}

func (b *readBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err == io.EOF && !b.ended {
		b.ended = true
		// Defensive: net/http has cleared the deadline in this same Read when
		// it started its background read; this keeps the rule true should a
		// server not do it.
		_ = b.rc.SetReadDeadline(time.Time{})
	}
	return n, err
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

// IsUnaryRPC reports whether r is the POST of a unary Connect call. A stream's
// request (application/connect+...) is left out: its client may stay silent
// after the one message for as long as it listens.
func IsUnaryRPC(r *http.Request) bool {
	if r.Method != http.MethodPost {
		return false
	}
	switch mediaType, _, _ := strings.Cut(r.Header.Get("Content-Type"), ";"); strings.TrimSpace(strings.ToLower(mediaType)) {
	case "application/json", "application/proto":
		return true
	}
	return false
}

// deadlineBody clears the read deadline once the body is over (or failed). A
// deadline left on the connection would reach its background read, which
// net/http uses to notice a client that left, and cancel the request's
// context while the handler still works.
type deadlineBody struct {
	io.ReadCloser
	rc *http.ResponseController
}

func (b *deadlineBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err != nil {
		_ = b.rc.SetReadDeadline(time.Time{})
	}
	return n, err
}
