package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/PuraFome/meuRPG/backend/internal/platform/ratelimit"
)

type review1CountingPinger struct{ n atomic.Int64 }

func (p *review1CountingPinger) Ping(context.Context) error { p.n.Add(1); return nil }

// Finding U1-03: /readyz reaches the database (Ping) but is not covered by the
// per-IP limiter (limitedPrefixes), contradicting the "probes touch no database" comment.
func TestReview1_ReadyzIsUnlimitedDBHit(t *testing.T) {
	pinger := &review1CountingPinger{}
	policy := ratelimit.NewPolicy(1) // same wiring as cmd/api/main.go
	srv := New(Config{
		DB:    pinger,
		Limit: ratelimit.Middleware(policy.IP, false, ratelimit.NewNotifier(nil, "test")),
	})

	const flood = 5000 // far above the 600 per-IP burst and 2000 global burst
	limited := 0
	for range flood {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/readyz", nil)
		req.RemoteAddr = "203.0.113.7:1234"
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code == http.StatusTooManyRequests {
			limited++
		}
	}
	t.Logf("flood=%d limited(429)=%d db pings=%d", flood, limited, pinger.n.Load())
	if limited == 0 {
		t.Fatalf("%d GET /readyz from one IP never got 429, yet each performed a DB ping (%d pings)", flood, pinger.n.Load())
	}
}
