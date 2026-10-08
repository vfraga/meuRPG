package httpserver

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestBuild writes a minimal fake Angular build to a temp dir:
// index.html plus "hashed" assets and files copied as they are, the way `ng build` would.
func newTestBuild(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	writeFile(t, filepath.Join(dir, "index.html"), "<html>app shell</html>")
	writeFile(t, filepath.Join(dir, "main-ABCD1234.js"), "console.log('app')")
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFile(t, filepath.Join(dir, "sub", "nested-EFGH5678.js"), "console.log('nested')")
	// Copied by the build as they are, so without a hash in the name.
	writeFile(t, filepath.Join(dir, "favicon.ico"), "icon")
	if err := os.Mkdir(filepath.Join(dir, "material-symbols"), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFile(t, filepath.Join(dir, "material-symbols", "outlined.css"), "css")
	writeFile(t, filepath.Join(dir, "material-symbols", "material-symbols-outlined.woff2"), "font")

	return dir
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestNewStatic_MissingBuild(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		dir  string
	}{
		{name: "empty dir means disabled", dir: ""},
		{name: "dir does not exist", dir: "/no/such/directory"},
		{name: "dir exists but has no index.html", dir: t.TempDir()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler, ok, _ := NewStatic(tt.dir)
			if ok {
				t.Fatalf("NewStatic(%q) ok = true, want false", tt.dir)
			}
			if handler != nil {
				t.Fatalf("NewStatic(%q) handler = %v, want nil", tt.dir, handler)
			}
		})
	}
}

func TestStaticHandler_Routing(t *testing.T) {
	t.Parallel()

	dir := newTestBuild(t)
	handler, ok, _ := NewStatic(dir)
	if !ok {
		t.Fatalf("NewStatic(%q) ok = false, want true", dir)
	}

	const indexBody = "<html>app shell</html>"

	tests := []struct {
		name       string
		path       string
		wantStatus int
		wantBody   string // checked only when non-empty
		wantCache  string // checked only when wantStatus is set and not a redirect/error
	}{
		{name: "root serves index", path: "/", wantStatus: http.StatusOK, wantBody: indexBody, wantCache: "no-cache"},
		{
			name: "hashed asset is served with immutable caching", path: "/main-ABCD1234.js",
			wantStatus: http.StatusOK, wantBody: "console.log('app')", wantCache: "public, max-age=31536000, immutable",
		},
		{
			name: "nested asset is served too", path: "/sub/nested-EFGH5678.js",
			wantStatus: http.StatusOK, wantBody: "console.log('nested')", wantCache: "public, max-age=31536000, immutable",
		},
		{
			name: "an unhashed file is revalidated, not immutable", path: "/favicon.ico",
			wantStatus: http.StatusOK, wantBody: "icon", wantCache: "no-cache",
		},
		{
			name: "an unhashed stylesheet is revalidated", path: "/material-symbols/outlined.css",
			wantStatus: http.StatusOK, wantBody: "css", wantCache: "no-cache",
		},
		{
			// "-outlined." looks like a hash by length, but it is not upper-case.
			name: "an unhashed font is revalidated", path: "/material-symbols/material-symbols-outlined.woff2",
			wantStatus: http.StatusOK, wantBody: "font", wantCache: "no-cache",
		},
		{
			// http.ServeFile's own well-known behavior: a request whose
			// path ends in "/index.html" is redirected to "./" rather than
			// served directly, so it can never end up as a second,
			// separately-cacheable URL for the same page.
			name: "index.html requested directly redirects to the canonical /",
			path: "/index.html", wantStatus: http.StatusMovedPermanently,
		},
		{
			name: "unknown client route falls back to index (SPA)", path: "/campaigns/42",
			wantStatus: http.StatusOK, wantBody: indexBody, wantCache: "no-cache",
		},
		{
			name: "unknown Connect RPC path is a 404, not the app shell",
			path: "/meurpg.system.v1.SystemService/Nope", wantStatus: http.StatusNotFound,
		},
		{name: "auth path is a 404, not the app shell", path: "/auth/callback", wantStatus: http.StatusNotFound},
		{name: "image path is a 404, not the app shell", path: "/images/6f1c7a52-3b5e-4c55-9d0b-2a51f0c1e001", wantStatus: http.StatusNotFound},
		{name: "upload path is a 404, not the app shell", path: "/uploads/images", wantStatus: http.StatusNotFound},
		{name: "healthz is a 404 here (it is registered elsewhere on the mux)", path: "/healthz", wantStatus: http.StatusNotFound},
		{name: "readyz is a 404 here (it is registered elsewhere on the mux)", path: "/readyz", wantStatus: http.StatusNotFound},
		{
			// http.ServeFile itself rejects any request path containing
			// "..", regardless of the file we asked it to serve: a real
			// second layer of defense on top of path.Clean.
			name: "path traversal is rejected outright", path: "/../../../etc/passwd", wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, tt.path, nil))

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %q)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantBody != "" && rec.Body.String() != tt.wantBody {
				t.Errorf("body = %q, want %q", rec.Body.String(), tt.wantBody)
			}
			if tt.wantCache != "" {
				if got := rec.Header().Get("Cache-Control"); got != tt.wantCache {
					t.Errorf("Cache-Control = %q, want %q", got, tt.wantCache)
				}
			}
		})
	}
}

func TestStaticHandler_SecurityHeaders(t *testing.T) {
	t.Parallel()

	handler, ok, _ := NewStatic(newTestBuild(t))
	if !ok {
		t.Fatal("NewStatic ok = false, want true")
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))

	if got := rec.Header().Get("Content-Security-Policy"); got != cspHeader(nil) {
		t.Errorf("Content-Security-Policy = %q, want %q", got, cspHeader(nil))
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if got := rec.Header().Get("Referrer-Policy"); got != "same-origin" {
		t.Errorf("Referrer-Policy = %q, want same-origin", got)
	}
}

func TestStaticHandler_RejectsOtherMethods(t *testing.T) {
	t.Parallel()

	handler, ok, _ := NewStatic(newTestBuild(t))
	if !ok {
		t.Fatal("NewStatic ok = false, want true")
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

// TestStaticHandler_Integration wires NewStatic into a real Server, the way
// cmd/api/main.go does, to prove the mux's longest-pattern-wins routing
// really does give /healthz and /readyz priority over the "/" catch-all.
func TestStaticHandler_Integration(t *testing.T) {
	t.Parallel()

	static, ok, _ := NewStatic(newTestBuild(t))
	if !ok {
		t.Fatal("NewStatic ok = false, want true")
	}

	srv := New(Config{Addr: ":0", Logger: discardLogger()})
	srv.Handle("/", static)

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/healthz", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /healthz status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("/healthz Cache-Control = %q, want no-store (the probe's, not the static handler's)", got)
	}
}

func TestStaticHandler_FormActionOrigin(t *testing.T) {
	t.Parallel()

	// The invite page posts to /auth/login, which redirects to the provider:
	// browsers check form-action on every hop, so the provider's origin must
	// be listed, reduced to scheme and host.
	handler, ok, err := NewStatic(newTestBuild(t), WithFormActionOrigin("https://accounts.google.com/o/oauth2/v2/auth?x=1"))
	if err != nil || !ok {
		t.Fatalf("NewStatic() = ok %v, err %v; want ok, nil", ok, err)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))

	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "form-action 'self' https://accounts.google.com;") {
		t.Errorf("Content-Security-Policy = %q, want form-action 'self' https://accounts.google.com", csp)
	}
	if !strings.Contains(csp, "script-src 'self';") {
		t.Errorf("Content-Security-Policy = %q, want the rest of the policy unchanged", csp)
	}
}

func TestStaticHandler_FormActionOriginMustBeAbsolute(t *testing.T) {
	t.Parallel()

	for _, bad := range []string{"", "accounts.google.com", "/relative", "javascript:alert(1)", "ftp://example.com"} {
		if _, ok, err := NewStatic(newTestBuild(t), WithFormActionOrigin(bad)); err == nil || ok {
			t.Errorf("NewStatic(WithFormActionOrigin(%q)) = ok %v, err %v; want an error", bad, ok, err)
		}
	}
}
