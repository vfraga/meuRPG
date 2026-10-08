package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// fakePinger is a Pinger whose answer the test controls.
type fakePinger struct{ err error }

func (f fakePinger) Ping(context.Context) error { return f.err }

// discardLogger keeps test output clean.
func discardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

func TestProbes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		db         Pinger
		draining   bool
		path       string
		wantStatus int
		wantBody   probeResponse
	}{
		{
			name:       "healthz is always ok",
			db:         fakePinger{err: errors.New("connection refused")},
			path:       "/healthz",
			wantStatus: http.StatusOK,
			wantBody:   probeResponse{Status: "ok"},
		},
		{
			name:       "healthz stays ok while draining",
			draining:   true,
			path:       "/healthz",
			wantStatus: http.StatusOK,
			wantBody:   probeResponse{Status: "ok"},
		},
		{
			name:       "readyz without a database",
			db:         nil,
			path:       "/readyz",
			wantStatus: http.StatusOK,
			wantBody:   probeResponse{Status: "ready", Database: "disabled"},
		},
		{
			name:       "readyz with a healthy database",
			db:         fakePinger{},
			path:       "/readyz",
			wantStatus: http.StatusOK,
			wantBody:   probeResponse{Status: "ready", Database: "up"},
		},
		{
			name:       "readyz with an unreachable database",
			db:         fakePinger{err: errors.New("connection refused")},
			path:       "/readyz",
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   probeResponse{Status: "not_ready", Database: "down"},
		},
		{
			name:       "readyz while shutting down",
			db:         fakePinger{},
			draining:   true,
			path:       "/readyz",
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   probeResponse{Status: "shutting_down"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := New(Config{Addr: ":0", Logger: discardLogger(), DB: tt.db})
			srv.draining.Store(tt.draining)

			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, tt.path, nil))

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if got := rec.Header().Get("Cache-Control"); got != "no-store" {
				t.Errorf("Cache-Control = %q, want no-store", got)
			}
			var body probeResponse
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if body != tt.wantBody {
				t.Errorf("body = %+v, want %+v", body, tt.wantBody)
			}
		})
	}
}

func TestProbesRejectOtherMethods(t *testing.T) {
	t.Parallel()

	srv := New(Config{Addr: ":0", Logger: discardLogger()})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/healthz", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /healthz status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

// countingPinger counts the pings that reach the database; it fails them
// when its context is already done, like a real connection would.
type countingPinger struct{ n atomic.Int64 }

func (p *countingPinger) Ping(ctx context.Context) error { p.n.Add(1); return ctx.Err() }

func TestReadyzSharesOneDatabasePingPerWindow(t *testing.T) {
	t.Parallel()
	pinger := &countingPinger{}
	srv := New(Config{DB: pinger, Logger: discardLogger()})
	clock := time.Now()
	srv.now = func() time.Time { return clock }

	const flood = 5000
	for range flood {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/readyz", nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("/readyz = %d, want 200", rec.Code)
		}
	}
	if got := pinger.n.Load(); got != 1 {
		t.Fatalf("%d requests in one window made %d database pings, want 1", flood, got)
	}

	// Positive control: once the window has passed, the database is asked again.
	clock = clock.Add(dbPingTTL + time.Millisecond)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/readyz", nil)
	srv.Handler().ServeHTTP(httptest.NewRecorder(), req)
	if got := pinger.n.Load(); got != 2 {
		t.Fatalf("after the window: %d pings, want 2", got)
	}
}

func TestReadyzAnswerDoesNotDependOnTheAskingRequest(t *testing.T) {
	t.Parallel()
	srv := New(Config{DB: &countingPinger{}, Logger: discardLogger()})
	gone, cancel := context.WithCancel(t.Context())
	cancel()
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequestWithContext(gone, http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/readyz for a hung-up request = %d, want 200 (the shared ping must not use its context)", rec.Code)
	}
}
