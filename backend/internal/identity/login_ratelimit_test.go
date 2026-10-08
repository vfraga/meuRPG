package identity

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/PuraFome/meuRPG/backend/internal/platform/secret"
)

// loginFrom visits /auth/login as a client at remoteAddr, with the given
// X-Forwarded-For header lines.
func (h *harness) loginFrom(remoteAddr string, xff ...string) *httptest.ResponseRecorder {
	h.t.Helper()
	req := httptest.NewRequestWithContext(h.t.Context(), http.MethodGet, "https://meurpg.test/auth/login?return_to=/", nil)
	req.RemoteAddr = remoteAddr
	for _, v := range xff {
		req.Header.Add("X-Forwarded-For", v)
	}
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, req)
	return rec
}

func TestLoginIsRateLimitedPerClient(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	burst := loginRateLimit.PerClient.Burst

	for i := range burst {
		if rec := h.loginFrom("192.0.2.1:1111"); rec.Code != http.StatusFound {
			t.Fatalf("sign-in %d: status = %d, want 302", i+1, rec.Code)
		}
	}

	rec := h.loginFrom("192.0.2.1:2222") // another port, same client
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("sign-in over the limit: status = %d, want 429", rec.Code)
	}
	if got := rec.Header().Get("Retry-After"); got != "3" {
		t.Errorf("Retry-After = %q, want 3 (seconds)", got)
	}
	assertAuthHeaders(t, rec.Header())
	if cookieIn(rec.Result().Cookies(), loginCookieName) != nil {
		t.Error("a refused sign-in set a login cookie")
	}
	h.mem.mu.Lock()
	saved := len(h.mem.loginStates)
	h.mem.mu.Unlock()
	if saved != burst {
		t.Errorf("login states saved = %d, want %d: a refused sign-in must not write", saved, burst)
	}

	// Locally, X-Forwarded-For is ignored, so a new header does not help.
	if rec := h.loginFrom("192.0.2.1:3333", "203.0.113.50"); rec.Code != http.StatusTooManyRequests {
		t.Errorf("with a made-up X-Forwarded-For: status = %d, want 429", rec.Code)
	}
	// Other clients are not affected.
	if rec := h.loginFrom("192.0.2.2:1111"); rec.Code != http.StatusFound {
		t.Errorf("another client: status = %d, want 302", rec.Code)
	}
	// And the client can sign in again once a token comes back.
	h.clock.Advance(loginRateLimit.PerClient.Every)
	if rec := h.loginFrom("192.0.2.1:1111"); rec.Code != http.StatusFound {
		t.Errorf("after waiting: status = %d, want 302", rec.Code)
	}
}

func TestLoginRateLimitOnCloudRun(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withCloudRun())
	const frontEnd = "169.254.1.1:9999" // what RemoteAddr looks like behind Google's front end

	// The client keeps the same real address (the last entry, appended by
	// Google's front end) but makes up a new first entry every time.
	for i := range loginRateLimit.PerClient.Burst {
		spoofed := fmt.Sprintf("10.0.0.%d, 203.0.113.7", i)
		if rec := h.loginFrom(frontEnd, spoofed); rec.Code != http.StatusFound {
			t.Fatalf("sign-in %d: status = %d, want 302", i+1, rec.Code)
		}
	}
	if rec := h.loginFrom(frontEnd, "198.51.100.1, 203.0.113.7"); rec.Code != http.StatusTooManyRequests {
		t.Errorf("same client, new spoofed entry: status = %d, want 429", rec.Code)
	}
	// Another client behind the same front end has its own limit.
	if rec := h.loginFrom(frontEnd, "203.0.113.8"); rec.Code != http.StatusFound {
		t.Errorf("another client: status = %d, want 302", rec.Code)
	}
}

func TestLoginGlobalRateLimit(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	burst := loginRateLimit.Global.Burst

	// Many clients, one sign-in each: a botnet.
	for i := range burst {
		addr := fmt.Sprintf("10.0.%d.%d:1", i/256, i%256)
		if rec := h.loginFrom(addr); rec.Code != http.StatusFound {
			t.Fatalf("client %d: status = %d, want 302", i, rec.Code)
		}
	}
	rec := h.loginFrom("192.0.2.200:1")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("client over the global limit: status = %d, want 429", rec.Code)
	}
	if got := rec.Header().Get("Retry-After"); got != "1" {
		t.Errorf("Retry-After = %q, want 1", got)
	}
	h.clock.Advance(time.Second)
	if rec := h.loginFrom("192.0.2.200:1"); rec.Code != http.StatusFound {
		t.Errorf("after waiting: status = %d, want 302", rec.Code)
	}
}

func TestCallbackIsRateLimitedPerClient(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	burst := loginRateLimit.PerClient.Burst
	callback := func(remoteAddr string) int {
		state, _ := secret.New()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "https://meurpg.test/auth/callback?state="+state+"&code=x", nil)
		req.RemoteAddr = remoteAddr
		req.AddCookie(&http.Cookie{Name: loginCookieName, Value: state}) //nolint:gosec // G124: a request cookie in a test
		rec := httptest.NewRecorder()
		h.mux.ServeHTTP(rec, req)
		return rec.Code
	}

	for i := range burst {
		if code := callback("192.0.2.77:1234"); code == http.StatusTooManyRequests {
			t.Fatalf("callback %d of %d was limited", i+1, burst)
		}
	}
	if code := callback("192.0.2.77:4321"); code != http.StatusTooManyRequests {
		t.Fatalf("callback over the limit: status = %d, want 429", code)
	}
	// Positive control: another client is not affected.
	if code := callback("192.0.2.78:1234"); code == http.StatusTooManyRequests {
		t.Fatal("another client was limited")
	}
}
