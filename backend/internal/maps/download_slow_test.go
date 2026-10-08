package maps

import (
	"bytes"
	"crypto/rand"
	"image"
	"image/png"
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

// Downloads of an image, its thumbnail and a fog tile give the client a write
// deadline, so one that stops reading is dropped instead of holding the
// handler until the platform closes the connection.
func TestImageAndTileDownloadsHaveAWriteDeadline(t *testing.T) {
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
