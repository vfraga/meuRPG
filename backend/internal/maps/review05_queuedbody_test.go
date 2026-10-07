package maps

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type countingReader struct {
	r    io.Reader
	read *atomic.Int64
}

func (c countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.read.Add(int64(n))
	return n, err
}

// Finding U5-1: upload() reads the whole 10 MiB body into memory before waiting for the processing slot, so queued uploads each hold a full body.
func TestReview5_QueuedUploadsHoldBodies(t *testing.T) {
	h := newHarness(t)
	master := h.newUser("Master")
	campaign := h.newCampaign(master)

	const k = 6
	const fileSize = 9 << 20
	content := append([]byte("\xff\xd8\xff"), make([]byte, fileSize)...)

	// Occupy the one-at-a-time slot, as a running image would.
	h.svc.processing <- struct{}{}
	released := false
	release := func() {
		if !released {
			released = true
			<-h.svc.processing
		}
	}
	defer release()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	var consumed atomic.Int64
	var total int64
	var wg sync.WaitGroup
	for range k {
		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		_ = w.WriteField("campaign_id", campaign)
		part, _ := w.CreateFormFile("file", "a.jpg")
		_, _ = part.Write(content)
		_ = w.Close()
		total += int64(buf.Len())
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.server.URL+UploadPath, countingReader{&buf, &consumed})
		if err != nil {
			t.Fatal(err)
		}
		req.ContentLength = int64(buf.Len())
		req.Header.Set("Content-Type", w.FormDataContentType())
		req.Header.Set(testUserHeader, master.id)
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := h.server.Client().Do(req)
			if err == nil {
				_, _ = io.Copy(io.Discard, res.Body)
				_ = res.Body.Close()
			}
		}()
	}

	// Correct behaviour: while the slot is busy, the waiting requests do not
	// all hold a full body. Give them time to (wrongly) read everything.
	deadline := time.Now().Add(10 * time.Second)
	for consumed.Load() < total && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	got := consumed.Load()
	cancel()
	release()
	wg.Wait()
	if got >= total-total/10 {
		t.Errorf("with the processing slot busy, the server read %d of %d bytes of %d queued uploads (about %d bodies of %d MiB held in memory); want at most about one body", got, total, k, got/fileSize, fileSize>>20)
	}
}
