package maps

// Finding U5-5: image and fog-tile downloads set no write deadline, so a client that stops reading holds the handler (and a Cloud Run slot) until the platform timeout.

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"image"
	"image/png"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// deadlineRecorder wraps the ResponseWriter and records every SetWriteDeadline,
// forwarding it to the real connection.
type deadlineRecorder struct {
	http.ResponseWriter
	mu    *sync.Mutex
	calls *[]time.Time
}

func (d deadlineRecorder) SetWriteDeadline(t time.Time) error {
	d.mu.Lock()
	*d.calls = append(*d.calls, t)
	d.mu.Unlock()
	return http.NewResponseController(d.ResponseWriter).SetWriteDeadline(t)
}

func (d deadlineRecorder) Unwrap() http.ResponseWriter { return d.ResponseWriter }

// noiseImage is an incompressible PNG of about side*side*4 bytes.
func noiseImage(t *testing.T, side int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, side, side))
	_, _ = rand.Read(img.Pix)
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 255
	}
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.NoCompression}
	if err := enc.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestReview5_ImageAndTileWriteDeadline(t *testing.T) {
	c := newCave(t)
	h := c.h

	var mu sync.Mutex
	calls := map[string]*[]time.Time{}
	inner := h.server.Config.Handler
	rec := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		l := calls[r.URL.Path]
		if l == nil {
			l = &[]time.Time{}
			calls[r.URL.Path] = l
		}
		mu.Unlock()
		inner.ServeHTTP(deadlineRecorder{ResponseWriter: w, mu: &mu, calls: l}, r)
	}))
	defer rec.Close()

	big := c.master.mustUpload(c.campaign, "grande.png", noiseImage(t, 1000))
	vision := c.ana.mustVision(c.campaign, c.mapID)
	tilePath := tileURL(vision, vision.GetTiles()[0])

	fetch := func(userID, path string) {
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, rec.URL+path, nil)
		req.Header.Set(testUserHeader, userID)
		res, err := rec.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		n := 0
		buf := make([]byte, 32<<10)
		for {
			k, err := res.Body.Read(buf)
			n += k
			if err != nil {
				break
			}
		}
		if res.StatusCode != http.StatusOK {
			t.Fatalf("GET %s: status %d", path, res.StatusCode)
		}
		t.Logf("GET %s: %d bytes", path, n)
	}
	imgPath := "/images/" + big.GetId()
	fetch(c.master.id, imgPath)
	fetch(c.master.id, imgPath+"/thumb")
	fetch(c.ana.id, tilePath)

	for _, p := range []string{imgPath, imgPath + "/thumb", strings.SplitN(tilePath, "?", 2)[0]} {
		mu.Lock()
		l := calls[p]
		var n int
		var nonZero bool
		if l != nil {
			n = len(*l)
			for _, d := range *l {
				if !d.IsZero() {
					nonZero = true
				}
			}
		}
		mu.Unlock()
		if !nonZero {
			t.Errorf("GET %s set no (non-zero) write deadline (%d SetWriteDeadline calls): a client that stops reading is never dropped", p, n)
		}
	}
}

// Supporting evidence: a client that reads only the headers of a large image
// and then stops keeps the handler blocked in its write.
func TestReview5_StalledReaderKeepsHandlerBlocked(t *testing.T) {
	c := newCave(t)
	h := c.h
	body := noiseImage(t, 1500)
	big := c.master.mustUpload(c.campaign, "grande.png", body)
	t.Logf("image size: %d bytes", len(body))

	done := make(chan struct{})
	inner := h.server.Config.Handler
	rec := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inner.ServeHTTP(w, r)
		close(done)
	}))
	defer rec.Close()

	conn, err := net.Dial("tcp", strings.TrimPrefix(rec.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }() // runs before rec.Close, so the handler can end
	if tc, ok := conn.(*net.TCPConn); ok {
		_ = tc.SetReadBuffer(4 << 10)
	}
	_, _ = conn.Write([]byte("GET /images/" + big.GetId() + " HTTP/1.1\r\nHost: x\r\n" + testUserHeader + ": " + c.master.id + "\r\n\r\n"))
	br := bufio.NewReaderSize(conn, 512)
	line, err := br.ReadString('\n')
	if err != nil || !strings.Contains(line, "200") {
		t.Fatalf("status line %q err %v", line, err)
	}
	select {
	case <-done:
		t.Fatal("handler finished before the stall")
	case <-time.After(4 * time.Second):
		t.Error("handler is still blocked writing to a client that stopped reading 4 s ago (no write deadline)")
	}
}
