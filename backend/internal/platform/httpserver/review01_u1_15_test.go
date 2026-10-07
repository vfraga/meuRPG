package httpserver

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// Finding U1-15: unhashed static files (favicon.ico from web/public) must not
// be served with the one-year immutable Cache-Control.
func TestReview1_UnhashedStaticNotImmutable(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "index.html"), "<html></html>")
	writeFile(t, filepath.Join(dir, "favicon.ico"), "icon")
	writeFile(t, filepath.Join(dir, "main-ABC123.js"), "js")
	h, ok, err := NewStatic(dir)
	if err != nil || !ok {
		t.Fatalf("NewStatic: ok=%v err=%v", ok, err)
	}
	get := func(p string) string {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		return rec.Header().Get("Cache-Control")
	}
	if cc := get("/main-ABC123.js"); !strings.Contains(cc, "immutable") {
		t.Errorf("hashed file should be immutable, got %q", cc)
	}
	if cc := get("/favicon.ico"); strings.Contains(cc, "immutable") {
		t.Errorf("unhashed favicon.ico must not be immutable, got %q", cc)
	}
}
