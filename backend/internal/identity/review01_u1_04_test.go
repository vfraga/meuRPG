package identity

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/PuraFome/meuRPG/backend/internal/platform/secret"
)

// Finding U1-04: GET /auth/callback is not rate limited, so each request with
// state==cookie costs a takeLoginState (DB DELETE) with no per-IP limit.
func TestReview1_CallbackIsRateLimited(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	const n = 500
	limited := 0
	for range n {
		state, _ := secret.New()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "https://meurpg.test/auth/callback?state="+state+"&code=x", nil)
		req.RemoteAddr = "192.0.2.77:1234"
		req.AddCookie(&http.Cookie{Name: loginCookieName, Value: state})
		rec := httptest.NewRecorder()
		h.mux.ServeHTTP(rec, req)
		if rec.Code == http.StatusTooManyRequests {
			limited++
		}
	}
	if limited == 0 {
		t.Fatalf("%d callback requests from one IP, none got 429: every one reached the database (takeLoginState)", n)
	}
}
