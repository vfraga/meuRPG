package maps

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"connectrpc.com/connect"

	"github.com/PuraFome/meuRPG/backend/internal/maps/images"
	"github.com/PuraFome/meuRPG/backend/internal/maps/mapsdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/blob"
)

// Characterization of storeImage (every way an image is stored or refused, and
// what is left behind) and of serve (who is served which image and with which
// headers). Written on the code before it was split into phases; it must keep
// passing unchanged.

// putFailer is a blob store whose nth Put fails (n from 1), and whose Open
// fails when openErr is set.
type putFailer struct {
	blob.Store
	failPut int
	puts    int
	openErr error
}

func (f *putFailer) Put(ctx context.Context, key, contentType string, r io.Reader) error {
	f.puts++
	if f.puts == f.failPut {
		return errors.New("disk full")
	}
	return f.Store.Put(ctx, key, contentType, r)
}

func (f *putFailer) Open(ctx context.Context, key string) (*blob.Object, error) {
	if f.openErr != nil {
		return nil, f.openErr
	}
	return f.Store.Open(ctx, key)
}

var uuidPattern = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

// maskUUIDs replaces every UUID of s with a placeholder, so a golden does not
// hold the ids a run made up.
func maskUUIDs(s string) string { return uuidPattern.ReplaceAllString(s, "<uuid>") }

func TestStoreImageIsFrozen(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master)
	svc := h.svc
	ctx := t.Context()
	res, err := images.Process(pngImage(t, 40, 30))
	if err != nil {
		t.Fatalf("images.Process() error = %v", err)
	}
	withReference := *res
	withReference.Reference = []byte("reference bytes")
	user := master.id
	flaky := &putFailer{Store: h.blobs}
	svc.blobs = flaky

	type finisher = func(ctx context.Context, q *mapsdb.Queries, imageID string) error
	store := func(res *images.Result, generated bool, kind string, parent *string, finish finisher) (mapsdb.GalleryImage, error) {
		return svc.storeImage(ctx, campaign, &user, "Nome", res, generated, kind, parent, finish)
	}
	count := func() int {
		var n int
		if err := h.pool.QueryRow(ctx, "SELECT count(*) FROM gallery_images WHERE campaign_id = $1", campaign).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	describe := func(row mapsdb.GalleryImage, err error) string {
		if err != nil {
			code := "-"
			if ce, ok := errors.AsType[*connect.Error](err); ok {
				code = ce.Code().String() + " " + ce.Message()
			}
			if he, ok := errors.AsType[*httpError](err); ok {
				code = fmt.Sprintf("http %d %s %q", he.status, he.reason, he.message)
			}
			return fmt.Sprintf("error=%v code=%s rows=%d files=[%s]", err, code, count(), maskUUIDs(strings.Join(h.storedFiles(), ",")))
		}
		parent := "-"
		if row.ParentImageID != nil {
			parent = "set"
		}
		return fmt.Sprintf("name=%q type=%s %dx%d size=%d generated=%v kind=%q parent=%s uploadedBy=%v rows=%d files=[%s]",
			row.Name, row.ContentType, row.Width, row.Height, row.ByteSize, row.Generated, row.GeneratedKind, parent,
			row.UploadedBy != nil && *row.UploadedBy == user, count(), maskUUIDs(strings.Join(h.storedFiles(), ",")))
	}
	// wipe empties the gallery and the store so each case starts clean.
	wipe := func() {
		t.Helper()
		if _, err := h.pool.Exec(ctx, "DELETE FROM gallery_images WHERE campaign_id = $1", campaign); err != nil {
			t.Fatal(err)
		}
		for _, f := range h.storedFiles() {
			if err := h.blobs.Delete(ctx, f); err != nil {
				t.Fatal(err)
			}
		}
		flaky.puts, flaky.failPut = 0, 0
	}
	var out strings.Builder
	record := func(name string, row mapsdb.GalleryImage, err error) {
		fmt.Fprintf(&out, "== %s\n%s\n\n", name, describe(row, err))
		wipe()
	}

	row, err := store(res, false, "", nil, nil)
	record("a plain image", row, err)
	row, err = store(&withReference, false, "", nil, nil)
	record("an image with a reference", row, err)

	parent, err := store(res, false, "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	row, err = store(res, true, "scene", &parent.ID, nil)
	out.WriteString("(parent stored first)\n")
	record("a generated image whose parent is there", row, err)
	gone := newKey()
	row, err = store(res, true, "map_scene", &gone, nil)
	record("a generated image whose parent is gone", row, err)

	var gotID string
	var queried bool
	row, err = store(res, false, "", nil, func(ctx context.Context, q *mapsdb.Queries, imageID string) error {
		gotID = imageID
		_, qerr := q.GetGalleryImageInCampaign(ctx, mapsdb.GetGalleryImageInCampaignParams{CampaignID: campaign, ID: imageID})
		queried = qerr == nil
		return nil
	})
	fmt.Fprintf(&out, "(finish got the row's id: %v, read the row inside the transaction: %v)\n", gotID == row.ID, queried)
	record("a finish that succeeds", row, err)
	row, err = store(res, false, "", nil, func(context.Context, *mapsdb.Queries, string) error { return errors.New("boom") })
	record("a finish that fails", row, err)
	row, err = store(res, false, "", nil, func(context.Context, *mapsdb.Queries, string) error { return errRequestClosed })
	record("a finish that finds the request closed", row, err)
	row, err = store(res, false, "", nil, func(context.Context, *mapsdb.Queries, string) error {
		return invalid(ReasonDimensions, "too many pixels")
	})
	record("a finish that refuses with an HTTP error", row, err)
	row, err = store(res, false, "", nil, func(context.Context, *mapsdb.Queries, string) error {
		return connect.NewError(connect.CodeFailedPrecondition, errors.New("not now"))
	})
	record("a finish that refuses with a Connect error", row, err)

	for i := 1; i <= 3; i++ {
		flaky.failPut = i
		row, err = store(&withReference, false, "", nil, nil)
		record(fmt.Sprintf("the %dth file cannot be written", i), row, err)
	}

	maxImages, maxBytes := svc.maxImages, svc.maxBytes
	svc.maxImages = 1
	if _, err := store(res, false, "", nil, nil); err != nil {
		t.Fatal(err)
	}
	row, err = store(res, false, "", nil, nil)
	svc.maxImages = maxImages
	record("a gallery at its image limit", row, err)
	svc.maxBytes = int32(len(res.Data)) //nolint:gosec // G115: a test image
	row, err = store(res, false, "", nil, nil)
	record("an image that exactly fills the bytes", row, err)
	svc.maxBytes = int32(len(res.Data)) - 1 //nolint:gosec // G115: a test image
	row, err = store(res, false, "", nil, nil)
	svc.maxBytes = maxBytes
	record("an image one byte over the bytes", row, err)
	checkGolden(t, "image_store.golden.txt", []byte(out.String()))
}

func TestServeImageIsFrozen(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, ana := h.newUser("Mestre"), h.newUser("Ana")
	campaign := h.newCampaign(master, ana)
	outsider := h.newUser("Fora")
	pending := h.newUser("Pendente")
	h.join(master, campaign, true, pending)
	hidden := master.mustUpload(campaign, "escondida.png", pngImage(t, 40, 30))
	onMap := master.mustUpload(campaign, "mapa.png", pngImage(t, 60, 40))
	master.setMapRevealed(campaign, master.createMap(campaign, "Vila", onMap.GetId()).GetId(), true)
	flaky := &putFailer{Store: h.blobs}
	h.svc.blobs = flaky

	line := func(r httpResult) string {
		hd := r.header
		return fmt.Sprintf("status=%d type=%q cache=%q vary=%q etag=%q nosniff=%q disposition=%q csp=%q corp=%q length=%q range=%q body=%s",
			r.status, hd.Get("Content-Type"), hd.Get("Cache-Control"), hd.Get("Vary"), strings.ReplaceAll(strings.ReplaceAll(hd.Get("ETag"), hidden.GetId(), "<hidden>"), onMap.GetId(), "<map>"),
			hd.Get("X-Content-Type-Options"), hd.Get("Content-Disposition"), hd.Get("Content-Security-Policy"), hd.Get("Cross-Origin-Resource-Policy"),
			hd.Get("Content-Length"), hd.Get("Content-Range"), digestOf(r.body))
	}
	asks := []struct {
		name string
		do   func() httpResult
	}{
		{"master, hidden", func() httpResult { return master.get(hidden.GetUrl()) }},
		{"master, hidden thumbnail", func() httpResult { return master.get(hidden.GetThumbnailUrl()) }},
		{"master, on a map", func() httpResult { return master.get(onMap.GetUrl()) }},
		{"master, 304", func() httpResult { return master.get(hidden.GetUrl(), "If-None-Match", `"`+hidden.GetId()+`"`) }},
		{"master, thumbnail 304", func() httpResult {
			return master.get(hidden.GetThumbnailUrl(), "If-None-Match", `"`+hidden.GetId()+`.thumb"`)
		}},
		{"master, a range", func() httpResult { return master.get(hidden.GetUrl(), "Range", "bytes=0-9") }},
		{"master, HEAD", func() httpResult { return master.do(headTo(t, h, hidden.GetUrl())) }},
		{"player, hidden", func() httpResult { return ana.get(hidden.GetUrl()) }},
		{"player, hidden, 304", func() httpResult { return ana.get(hidden.GetUrl(), "If-None-Match", `"`+hidden.GetId()+`"`) }},
		{"player, on a map", func() httpResult { return ana.get(onMap.GetUrl()) }},
		{"player, on a map, thumbnail", func() httpResult { return ana.get(onMap.GetThumbnailUrl()) }},
		{"player, on a map, 304", func() httpResult { return ana.get(onMap.GetUrl(), "If-None-Match", `"`+onMap.GetId()+`"`) }},
		{"non-member", func() httpResult { return outsider.get(onMap.GetUrl()) }},
		{"pending member", func() httpResult { return pending.get(onMap.GetUrl()) }},
		{"anonymous", func() httpResult { return h.anonymous().get(onMap.GetUrl()) }},
		{"anonymous, bad id", func() httpResult { return h.anonymous().get(ImagesPath + "x") }},
		{"bad id", func() httpResult { return master.get(ImagesPath + "x") }},
		{"unknown id", func() httpResult { return master.get(ImagesPath + newKey()) }},
		{"uppercase id", func() httpResult { return master.get(ImagesPath + strings.ToUpper(hidden.GetId())) }},
		{"the store cannot open", func() httpResult {
			flaky.openErr = errors.New("io error")
			defer func() { flaky.openErr = nil }()
			return master.get(hidden.GetUrl())
		}},
		{"the file is missing", func() httpResult {
			flaky.openErr = blob.ErrNotFound
			defer func() { flaky.openErr = nil }()
			return master.get(hidden.GetUrl())
		}},
		{"the player's file is missing", func() httpResult {
			flaky.openErr = blob.ErrNotFound
			defer func() { flaky.openErr = nil }()
			return ana.get(onMap.GetUrl())
		}},
	}
	var out strings.Builder
	for _, a := range asks {
		fmt.Fprintf(&out, "%s: %s\n", a.name, line(a.do()))
	}
	checkGolden(t, "image_serve.golden.txt", []byte(out.String()))
}

// headTo is a HEAD for a path of the test server.
func headTo(t *testing.T, h *harness, path string) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodHead, h.server.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	return req
}
