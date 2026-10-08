package ratelimit

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"connectrpc.com/connect"
)

// RetryAfterSeconds is wait in the whole seconds the Retry-After header
// wants (RFC 9110, section 10.2.3), at least 1.
func RetryAfterSeconds(wait time.Duration) int {
	return max(1, int(math.Ceil(wait.Seconds())))
}

// RPCError is the Connect error for a refused call: `resource_exhausted`,
// with a Retry-After header in its metadata. The web app reads that header
// to tell "slow down" from the other `resource_exhausted` answers (a full
// gallery, a campaign at its limit).
func RPCError(wait time.Duration) error {
	secs := RetryAfterSeconds(wait)
	err := connect.NewError(connect.CodeResourceExhausted,
		fmt.Errorf("too many requests, please wait %d seconds and try again", secs))
	err.Meta().Set("Retry-After", strconv.Itoa(secs))
	return err
}

// IsRateLimited reports whether err is an RPCError.
func IsRateLimited(err error) bool {
	connectErr, ok := errors.AsType[*connect.Error](err)
	return ok && connectErr.Code() == connect.CodeResourceExhausted && connectErr.Meta().Get("Retry-After") != ""
}

// limitedPrefixes are the routes that reach the database for a request that
// may carry no valid session. /auth/ has its own, stricter limit
// (identity/login.go); the static app and /healthz touch none, and /readyz
// shares one database ping per window (httpserver/health.go).
var limitedPrefixes = []string{"/meurpg.", "/images/", "/uploads/"}

// Middleware limits the API routes by client IP, before anything else
// reads the session: the cheapest place to stop a script that sends
// anonymous requests, or a made-up session cookie, each of which costs a
// database read. It answers 429 with Retry-After. behindCloudRun says where
// the client IP comes from (ClientKey).
func Middleware(l *Limiter, behindCloudRun bool, n *Notifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !isLimited(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			ok, wait := l.Allow(ClientKey(r, behindCloudRun))
			if ok {
				next.ServeHTTP(w, r)
				return
			}
			n.Hit(r.Context())
			writeTooMany(w, wait)
		})
	}
}

func isLimited(path string) bool {
	for _, p := range limitedPrefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// writeTooMany answers 429 in the shape each kind of client reads: the
// Connect error JSON for an RPC (a unary client reads the code from it; a
// streaming one maps the 429 itself), and the upload route's JSON (code,
// reason, message) for the rest.
func writeTooMany(w http.ResponseWriter, wait time.Duration) {
	secs := RetryAfterSeconds(wait)
	h := w.Header()
	h.Set("Retry-After", strconv.Itoa(secs))
	h.Set("Content-Type", "application/json")
	h.Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusTooManyRequests)
	body := fmt.Sprintf(`{"code":"resource_exhausted","reason":"RATE_LIMITED","message":"too many requests, please wait %d seconds and try again"}`, secs)
	_, _ = w.Write([]byte(body))
}
