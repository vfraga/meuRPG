package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// dbPingTimeout keeps /readyz fast even when the database hangs.
const dbPingTimeout = 2 * time.Second

// dbPingTTL is how long /readyz reuses the last database answer. The route
// is open to anyone and sits outside the per-IP limit (Cloud Run's probes
// share addresses), so what bounds its cost is this: at most one ping per
// window, however many requests arrive.
const dbPingTTL = 2 * time.Second

// pingCache shares one database ping among all the /readyz requests of a
// window. The ping runs under the lock, so concurrent requests wait for it
// instead of each taking a pool connection.
type pingCache struct {
	mu      sync.Mutex
	at      time.Time
	err     error
	checked bool
}

// check returns the cached answer while it is fresh, otherwise pings. The
// ping is not tied to the asking request: its answer is shared, so one
// client hanging up must not turn it into an error for the others.
func (c *pingCache) check(ctx context.Context, db Pinger, now func() time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.checked && now().Sub(c.at) < dbPingTTL {
		return c.err
	}
	pingCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), dbPingTimeout)
	defer cancel()
	c.err = db.Ping(pingCtx)
	c.at = now()
	c.checked = true
	return c.err
}

// probeResponse is the JSON body of /healthz and /readyz.
type probeResponse struct {
	Status   string `json:"status"`
	Database string `json:"database,omitempty"`
}

// handleHealthz is the liveness probe: "is the process alive?". It never
// checks dependencies. If it did, a database outage would make the platform
// restart perfectly healthy containers, which cannot fix the database.
func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeProbe(w, http.StatusOK, probeResponse{Status: "ok"})
}

// handleReadyz is the readiness probe: "can this instance serve traffic
// right now?". It answers 503 while shutting down or when the database is
// configured but unreachable.
func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	if s.draining.Load() {
		writeProbe(w, http.StatusServiceUnavailable, probeResponse{Status: "shutting_down"})
		return
	}

	if s.db == nil {
		writeProbe(w, http.StatusOK, probeResponse{Status: "ready", Database: "disabled"})
		return
	}

	if err := s.ping.check(r.Context(), s.db, s.now); err != nil {
		// The details go to the log, not to the (unauthenticated) caller.
		s.logger.WarnContext(r.Context(), "readiness: database ping failed", "error", err)
		writeProbe(w, http.StatusServiceUnavailable, probeResponse{Status: "not_ready", Database: "down"})
		return
	}

	writeProbe(w, http.StatusOK, probeResponse{Status: "ready", Database: "up"})
}

func writeProbe(w http.ResponseWriter, status int, body probeResponse) {
	w.Header().Set("Content-Type", "application/json")
	// Probes must reflect the current state, never a cached one.
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	// Nothing useful can be done if the client has gone away.
	_ = json.NewEncoder(w).Encode(body)
}
