package maps

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1/mapsv1connect"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/maps/images"
	"github.com/PuraFome/meuRPG/backend/internal/maps/link"
	"github.com/PuraFome/meuRPG/backend/internal/platform/blob"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// TestAuthorizationMatrix calls every GalleryService method and every image
// route as each kind of caller (ADR-0011):
//
//	master      the campaign's master: everything
//	player      a player of the campaign: permission_denied for the
//	            gallery, but may fetch an image they see, here the
//	            image of a revealed map (the rest of that rule is
//	            TestRN10_PlayersOnlyFetchImagesTheyCanSee)
//	non-member  signed in, outside the campaign: not_found, and 404 for
//	            images, so nothing leaks
//	anonymous   unauthenticated (401 on the routes)
//	pending     a pending member (RN-15, MR-024): like a non-member
func TestAuthorizationMatrix(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, player, pending := h.newUser("Mestre"), h.newUser("Jogadora"), h.newUser("Pendente")
	campaign := h.newCampaign(master, player)
	h.join(master, campaign, true, pending)
	kept := master.mustUpload(campaign, "mapa.png", pngImage(t, 600, 300))
	shown := master.createMap(campaign, "Mirathel", kept.GetId())
	master.setMapRevealed(campaign, shown.GetId(), true)

	// Each row returns the call's Connect code (routes: the error body's).
	type call = func(ctx context.Context, u *user) connect.Code
	rpc := func(err error) connect.Code {
		if err == nil {
			return allowed
		}
		return connect.CodeOf(err)
	}
	route := func(t *testing.T, res httpResult, okStatus int) connect.Code {
		t.Helper()
		if res.status == okStatus {
			return allowed
		}
		var code connect.Code
		body := res.errorBody(t)
		if err := code.UnmarshalText([]byte(body.Code)); err != nil {
			t.Fatalf("error code %q: %v", body.Code, err)
		}
		if want := httpStatus(code); res.status != want {
			t.Errorf("status %d for %s, want %d", res.status, body.Code, want)
		}
		return code
	}
	rows := []struct {
		name string
		call call
		// master, player, non-member, anonymous, pending
		want [5]connect.Code
	}{
		{"ListGalleryImages", func(ctx context.Context, u *user) connect.Code {
			_, err := u.gallery.ListGalleryImages(ctx, connect.NewRequest(&mapsv1.ListGalleryImagesRequest{CampaignId: campaign}))
			return rpc(err)
		}, [5]connect.Code{allowed, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},
		{"RenameGalleryImage", func(ctx context.Context, u *user) connect.Code {
			_, err := u.gallery.RenameGalleryImage(ctx, connect.NewRequest(&mapsv1.RenameGalleryImageRequest{CampaignId: campaign, ImageId: kept.GetId(), Name: "Mapa"}))
			return rpc(err)
		}, [5]connect.Code{allowed, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},
		// Each caller deletes an image of its own, so an allowed delete does
		// not change the next caller's row.
		{"DeleteGalleryImage", func(ctx context.Context, u *user) connect.Code {
			doomed := master.mustUpload(campaign, "apagar.png", pngImage(t, 10, 10))
			_, err := u.gallery.DeleteGalleryImage(ctx, connect.NewRequest(&mapsv1.DeleteGalleryImageRequest{CampaignId: campaign, ImageId: doomed.GetId()}))
			return rpc(err)
		}, [5]connect.Code{allowed, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},
		{"POST " + UploadPath, func(_ context.Context, u *user) connect.Code {
			return route(t, u.upload(campaign, "novo.png", pngImage(t, 10, 10)), http.StatusCreated)
		}, [5]connect.Code{allowed, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},
		{"GET " + ImagesPath + "{id}", func(_ context.Context, u *user) connect.Code {
			return route(t, u.get(kept.GetUrl()), http.StatusOK)
		}, [5]connect.Code{allowed, allowed, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},
		{"GET " + ImagesPath + "{id}/thumb", func(_ context.Context, u *user) connect.Code {
			return route(t, u.get(kept.GetThumbnailUrl()), http.StatusOK)
		}, [5]connect.Code{allowed, allowed, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},
	}

	covered := map[string]bool{}
	for _, r := range rows {
		covered[r.name] = true
	}
	methods := mapsv1.File_meurpg_maps_v1_gallery_proto.Services().ByName("GalleryService").Methods()
	for i := range methods.Len() {
		if name := string(methods.Get(i).Name()); !covered[name] {
			t.Errorf("GalleryService.%s is missing from the authorization matrix", name)
		}
	}

	callers := []struct {
		name string
		user *user
	}{
		{"master", master},
		{"player", player},
		{"non-member", h.newUser("De fora")},
		{"anonymous", h.anonymous()},
		{"pending", pending},
	}
	for _, r := range rows {
		for i, caller := range callers {
			if got, want := r.call(t.Context(), caller.user), r.want[i]; got != want {
				t.Errorf("%s as %s: code %v, want %v", r.name, caller.name, got, want)
			}
		}
	}
}

// TestUploadServeRenameDelete follows an image through its whole life.
func TestUploadServeRenameDelete(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master)
	ctx := t.Context()

	res := master.upload(campaign, `C:\Users\Samuel\Mapas\Mirathel.png`, pngImage(t, 960, 480))
	if res.status != http.StatusCreated || res.header.Get("Cache-Control") != "no-store" || res.header.Get("Content-Type") != "application/json" {
		t.Fatalf("upload: status %d, headers %v, body %s; want 201, JSON, no-store", res.status, res.header, res.body)
	}
	img := res.image(t)
	if res.header.Get("Location") != img.GetUrl() || img.GetUrl() != ImagesPath+img.GetId() || img.GetThumbnailUrl() != img.GetUrl()+"/thumb" {
		t.Errorf("Location %q, url %q, thumbnail_url %q", res.header.Get("Location"), img.GetUrl(), img.GetThumbnailUrl())
	}
	if img.GetName() != "Mirathel" || img.GetContentType() != "image/png" || img.GetWidth() != 960 || img.GetHeight() != 480 || img.GetCreatedAt() == nil {
		t.Errorf("uploaded image = %v", img)
	}
	older := master.mustUpload(campaign, "segundo.jpg", jpegWithGPS(t, 20, 20))

	// Newest first, with the quota's usage.
	list := master.list(campaign)
	if len(list.GetImages()) != 2 || list.GetImages()[0].GetId() != older.GetId() || list.GetImages()[1].GetId() != img.GetId() {
		t.Errorf("gallery = %v, want the newest first", list.GetImages())
	}
	usage := list.GetUsage()
	if usage.GetImageCount() != 2 || usage.GetByteCount() != img.GetByteSize()+older.GetByteSize() ||
		usage.GetMaxImages() != DefaultMaxImages || usage.GetMaxBytes() != DefaultMaxBytes || usage.GetMaxImageBytes() != images.MaxBytes {
		t.Errorf("usage = %v", usage)
	}

	// Serving: the stored bytes, with the headers of an immutable, private
	// image for the master. (What a player may fetch, and how it is cached,
	// is TestRN10_PlayersOnlyFetchImagesTheyCanSee.)
	imageKey, thumbnailKey := blobKeys(campaign, img.GetId())
	_, storedImage := h.storedFile(imageKey)
	got := master.get(img.GetUrl())
	if got.status != http.StatusOK || !bytes.Equal(got.body, storedImage) {
		t.Fatalf("GET image as the master: status %d, %d bytes; want 200 and the stored %d bytes", got.status, len(got.body), len(storedImage))
	}
	for header, want := range map[string]string{
		"Content-Type":                 "image/png",
		"Cache-Control":                "private, max-age=31536000, immutable",
		"Vary":                         "Cookie",
		"ETag":                         `"` + img.GetId() + `"`,
		"X-Content-Type-Options":       "nosniff",
		"Content-Disposition":          "inline",
		"Content-Security-Policy":      "default-src 'none'",
		"Cross-Origin-Resource-Policy": "same-origin",
	} {
		if v := got.header.Get(header); v != want {
			t.Errorf("GET image %s = %q, want %q", header, v, want)
		}
	}
	if cl := got.header.Get("Content-Length"); cl != strconv.Itoa(len(storedImage)) {
		t.Errorf("GET image Content-Length = %q, want %d", cl, len(storedImage))
	}
	thumb := master.get(img.GetThumbnailUrl())
	_, storedThumb := h.storedFile(thumbnailKey)
	if thumb.status != http.StatusOK || !bytes.Equal(thumb.body, storedThumb) || thumb.header.Get("ETag") != `"`+img.GetId()+`.thumb"` {
		t.Errorf("GET thumbnail: status %d, ETag %q", thumb.status, thumb.header.Get("ETag"))
	}
	if again := master.get(img.GetUrl(), "If-None-Match", `"`+img.GetId()+`"`); again.status != http.StatusNotModified || len(again.body) != 0 {
		t.Errorf("GET with If-None-Match: status %d, %d bytes; want 304, empty", again.status, len(again.body))
	}
	// A stranger learns nothing, not even from an ETag.
	stranger := h.newUser("De fora")
	if res := stranger.get(img.GetUrl(), "If-None-Match", `"`+img.GetId()+`"`); res.status != http.StatusNotFound {
		t.Errorf("GET with If-None-Match as a stranger: status %d, want 404", res.status)
	}
	for _, path := range []string{ImagesPath + "not-a-uuid", ImagesPath + "6f1c7a52-3b5e-4c55-9d0b-2a51f0c1e0ff", ImagesPath + img.GetId() + "/other"} {
		if res := master.get(path); res.status != http.StatusNotFound {
			t.Errorf("GET %s: status %d, want 404", path, res.status)
		}
	}

	// Rename: the platform's name rules.
	for _, bad := range []string{"", "   ", strings.Repeat("a", 81), "duas\nlinhas"} {
		_, err := master.gallery.RenameGalleryImage(ctx, connect.NewRequest(&mapsv1.RenameGalleryImageRequest{CampaignId: campaign, ImageId: img.GetId(), Name: bad}))
		wantCode(t, "RenameGalleryImage("+bad+")", err, connect.CodeInvalidArgument)
	}
	renamed, err := master.gallery.RenameGalleryImage(ctx, connect.NewRequest(&mapsv1.RenameGalleryImageRequest{CampaignId: campaign, ImageId: img.GetId(), Name: "  Mapa de Mirathel  "}))
	if err != nil || renamed.Msg.GetImage().GetName() != "Mapa de Mirathel" {
		t.Fatalf("RenameGalleryImage() = %v, %v", renamed, err)
	}
	_, err = master.gallery.RenameGalleryImage(ctx, connect.NewRequest(&mapsv1.RenameGalleryImageRequest{CampaignId: campaign, ImageId: "6f1c7a52-3b5e-4c55-9d0b-2a51f0c1e0ff", Name: "X"}))
	wantCode(t, "RenameGalleryImage of a missing image", err, connect.CodeNotFound)
	// An image of another campaign is not found through this one.
	otherCampaign := h.newCampaign(master)
	_, err = master.gallery.RenameGalleryImage(ctx, connect.NewRequest(&mapsv1.RenameGalleryImageRequest{CampaignId: otherCampaign, ImageId: img.GetId(), Name: "X"}))
	wantCode(t, "RenameGalleryImage through another campaign", err, connect.CodeNotFound)

	// Delete: the row and both files go.
	if _, err := master.gallery.DeleteGalleryImage(ctx, connect.NewRequest(&mapsv1.DeleteGalleryImageRequest{CampaignId: campaign, ImageId: img.GetId()})); err != nil {
		t.Fatalf("DeleteGalleryImage() error = %v", err)
	}
	if got := master.list(campaign).GetImages(); len(got) != 1 || got[0].GetId() != older.GetId() {
		t.Errorf("gallery after the delete = %v", got)
	}
	for _, f := range h.storedFiles() {
		if strings.Contains(f, img.GetId()) {
			t.Errorf("file %s is still stored after the delete", f)
		}
	}
	if res := master.get(img.GetUrl()); res.status != http.StatusNotFound {
		t.Errorf("GET a deleted image: status %d, want 404", res.status)
	}
	_, err = master.gallery.DeleteGalleryImage(ctx, connect.NewRequest(&mapsv1.DeleteGalleryImageRequest{CampaignId: campaign, ImageId: img.GetId()}))
	wantCode(t, "DeleteGalleryImage again", err, connect.CodeNotFound)
}

// TestUploadRefusals: every refused upload answers with its reason and
// leaves no file and no row behind.
func TestUploadRefusals(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master)
	png := pngImage(t, 10, 10)
	field := func(name, content string) formField { return formField{name: name, content: []byte(content)} }
	file := func(name string, content []byte) formField {
		return formField{name: "file", fileName: name, content: content}
	}

	notAForm, err := http.NewRequestWithContext(t.Context(), http.MethodPost, h.server.URL+UploadPath, strings.NewReader(`{"campaign_id":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	notAForm.Header.Set("Content-Type", "application/json")

	tests := []struct {
		name   string
		req    *http.Request
		status int
		code   string
		reason string
	}{
		{"not a form", notAForm, 400, "invalid_argument", ReasonMalformedRequest},
		{"the file before campaign_id", master.uploadRequest(file("a.png", png), field("campaign_id", campaign)), 400, "invalid_argument", ReasonMalformedRequest},
		{"no file", master.uploadRequest(field("campaign_id", campaign)), 400, "invalid_argument", ReasonMalformedRequest},
		{"a text field instead of the file", master.uploadRequest(field("campaign_id", campaign), field("file", "x")), 400, "invalid_argument", ReasonUnsupportedType},
		{"a third field", master.uploadRequest(field("campaign_id", campaign), file("a.png", png), field("extra", "x")), 400, "invalid_argument", ReasonMalformedRequest},
		{"a campaign_id that is not a UUID", master.uploadRequest(field("campaign_id", "mirathel"), file("a.png", png)), 404, "not_found", ""},
		{"an SVG", master.uploadRequest(field("campaign_id", campaign), file("a.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`))), 400, "invalid_argument", ReasonUnsupportedType},
		{"a GIF", master.uploadRequest(field("campaign_id", campaign), file("a.gif", []byte("GIF89a\x01\x00\x01\x00\x00\x00\x00;"))), 400, "invalid_argument", ReasonUnsupportedType},
		{"a broken PNG", master.uploadRequest(field("campaign_id", campaign), file("a.png", png[:len(png)/2])), 400, "invalid_argument", ReasonCorrupt},
		{"a PNG claiming 50000 x 50000", master.uploadRequest(field("campaign_id", campaign), file("a.png", bombPNG())), 400, "invalid_argument", ReasonDimensions},
		{"a file over 10 MiB", master.uploadRequest(field("campaign_id", campaign), file("a.jpg", append([]byte("\xff\xd8\xff"), make([]byte, images.MaxBytes)...))), 413, "invalid_argument", ReasonTooLarge},
	}
	for _, tt := range tests {
		res := master.do(tt.req)
		body := res.errorBody(t)
		if res.status != tt.status || body.Code != tt.code || body.Reason != tt.reason || body.Message == "" {
			t.Errorf("%s: status %d, body %s; want %d %s %s", tt.name, res.status, res.body, tt.status, tt.code, tt.reason)
		}
		if res.header.Get("Cache-Control") != "no-store" || res.header.Get("Content-Type") != "application/json" {
			t.Errorf("%s: headers %v, want JSON and no-store", tt.name, res.header)
		}
	}
	if files := h.storedFiles(); len(files) != 0 {
		t.Errorf("refused uploads left files behind: %v", files)
	}
	if got := master.list(campaign).GetImages(); len(got) != 0 {
		t.Errorf("refused uploads left rows behind: %v", got)
	}
}

// bombPNG is a PNG header claiming 50000 x 50000 pixels, with no data.
func bombPNG() []byte {
	ihdr := []byte{0, 0, 0xc3, 0x50, 0, 0, 0xc3, 0x50, 8, 6, 0, 0, 0}
	c := append([]byte{0, 0, 0, 13}, "IHDR"...)
	c = append(c, ihdr...)
	c = binary.BigEndian.AppendUint32(c, crc32.ChecksumIEEE(c[4:]))
	return append([]byte("\x89PNG\r\n\x1a\n"), c...)
}

// TestUploadQuota: a campaign's gallery holds at most MaxImages images and
// MaxBytes bytes; the upload over the limit is refused with QUOTA, and its
// files are deleted.
func TestUploadQuota(t *testing.T) {
	t.Parallel()
	t.Run("images", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t, func(c *Config) { c.MaxImages = 2 })
		master := h.newUser("Mestre")
		campaign := h.newCampaign(master)
		master.mustUpload(campaign, "1.png", pngImage(t, 10, 10))
		master.mustUpload(campaign, "2.png", pngImage(t, 10, 10))
		res := master.upload(campaign, "3.png", pngImage(t, 10, 10))
		if body := res.errorBody(t); res.status != http.StatusTooManyRequests || body.Code != "resource_exhausted" || body.Reason != ReasonQuota {
			t.Errorf("third upload: status %d, body %s; want 429 resource_exhausted QUOTA", res.status, res.body)
		}
		if files := h.storedFiles(); len(files) != 4 {
			t.Errorf("stored files = %v, want the two images' four", files)
		}
		// Another campaign has its own quota.
		other := h.newCampaign(master)
		master.mustUpload(other, "1.png", pngImage(t, 10, 10))
	})
	t.Run("bytes", func(t *testing.T) {
		t.Parallel()
		first := pngImage(t, 64, 64)
		// Room for the first image, not for a second one like it.
		h := newHarness(t, func(c *Config) { c.MaxBytes = int32(len(first)) + 10 }) //nolint:gosec // G115: a PNG of a few hundred bytes
		master := h.newUser("Mestre")
		campaign := h.newCampaign(master)
		img := master.mustUpload(campaign, "1.png", first)
		// The check inside the transaction (the early one passes: the usage
		// is still under the limit).
		res := master.upload(campaign, "2.png", pngImage(t, 64, 64))
		if body := res.errorBody(t); res.status != http.StatusTooManyRequests || body.Reason != ReasonQuota {
			t.Errorf("second upload: status %d, body %s; want 429 QUOTA", res.status, res.body)
		}
		if files := h.storedFiles(); len(files) != 2 || !strings.Contains(files[0], img.GetId()) {
			t.Errorf("stored files = %v, want only the first image's two", files)
		}
	})
}

// TestImagesOff: without a blob store (no BLOB_DIR), the image routes answer
// 503 and GalleryService unavailable; nothing else changes.
func TestImagesOff(t *testing.T) {
	t.Parallel()
	h := newHarness(t, func(c *Config) { c.Blobs = nil })
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master)

	for name, res := range map[string]httpResult{
		"upload": master.upload(campaign, "a.png", pngImage(t, 10, 10)),
		"image":  master.get(ImagesPath + "6f1c7a52-3b5e-4c55-9d0b-2a51f0c1e001"),
		"thumb":  master.get(ImagesPath + "6f1c7a52-3b5e-4c55-9d0b-2a51f0c1e001/thumb"),
	} {
		if body := res.errorBody(t); res.status != http.StatusServiceUnavailable || body.Code != "unavailable" {
			t.Errorf("%s: status %d, body %s; want 503 unavailable", name, res.status, res.body)
		}
	}
	_, err := master.gallery.ListGalleryImages(t.Context(), connect.NewRequest(&mapsv1.ListGalleryImagesRequest{CampaignId: campaign}))
	wantCode(t, "ListGalleryImages", err, connect.CodeUnavailable)
}

// TestCrossSiteUploadsAreRefused: the upload changes state, so another site
// must not be able to make a signed-in master's browser send one (CSRF).
// http.CrossOriginProtection, in front of every route, refuses it.
func TestCrossSiteUploadsAreRefused(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master)
	fields := []formField{{name: "campaign_id", content: []byte(campaign)}, {name: "file", fileName: "a.png", content: pngImage(t, 10, 10)}}

	crossSite := master.uploadRequest(fields...)
	crossSite.Header.Set("Sec-Fetch-Site", "cross-site")
	otherOrigin := master.uploadRequest(fields...) // an older browser, without Sec-Fetch-Site
	otherOrigin.Header.Set("Origin", "https://evil.example")
	for name, req := range map[string]*http.Request{"Sec-Fetch-Site: cross-site": crossSite, "Origin of another site": otherOrigin} {
		if res := master.do(req); res.status != http.StatusForbidden {
			t.Errorf("%s: status %d, want 403", name, res.status)
		}
	}
	if files := h.storedFiles(); len(files) != 0 {
		t.Errorf("a cross-site upload stored %v", files)
	}

	// The app's own page is same-origin, and gets through.
	sameOrigin := master.uploadRequest(fields...)
	sameOrigin.Header.Set("Sec-Fetch-Site", "same-origin")
	sameOrigin.Header.Set("Origin", h.server.URL)
	if res := master.do(sameOrigin); res.status != http.StatusCreated {
		t.Errorf("same-origin upload: status %d, body %s; want 201", res.status, res.body)
	}
}

// Tests below need no database.

func TestImageNamesComeFromTheFileName(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"Taverna.png":                        "Taverna",
		"mapa.final.v2.jpg":                  "mapa.final.v2",
		`C:\Users\Samuel\Mapas\Mirathel.png`: "Mirathel",
		"/home/samuel/mapa.webp":             "mapa",
		"  espaços  .png":                    "espaços",
		"sem extensão":                       "sem extensão",
		".png":                               defaultImageName,
		"":                                   defaultImageName,
		"\u202eevil\u202c.png":               "evil",
		"linha\nquebrada.png":                "linhaquebrada",
		"\xff\xfe.png":                       defaultImageName,
		strings.Repeat("á", 100) + ".png":    strings.Repeat("á", 80),
	} {
		if got := imageName(in); got != want {
			t.Errorf("imageName(%q) = %q, want %q", in, got, want)
		}
	}
}

// noCharacters and noLive stand in for the characters and play modules in
// the tests that never reach them.
type noCharacters struct{}

func (noCharacters) MapCharacters(context.Context, pgx.Tx, string, []string) ([]*charactersv1.CharacterSummary, error) {
	return nil, errors.New("not in this test")
}

func (noCharacters) ClearPortraits(context.Context, pgx.Tx, string, string) (int64, error) {
	return 0, errors.New("not in this test")
}

func (noCharacters) PortraitInUse(context.Context, string, string) (bool, error) {
	return false, errors.New("not in this test")
}

func (noCharacters) NpcPortraits(context.Context, pgx.Tx, string, []string) (map[string]string, error) {
	return nil, errors.New("not in this test")
}

func (noCharacters) PartyTotalLevels(context.Context, pgx.Tx, string) ([]int, error) {
	return nil, errors.New("not in this test")
}

func (noCharacters) PartyVision(context.Context, pgx.Tx, string) ([]link.PartyMember, error) {
	return nil, errors.New("not in this test")
}

func (noCharacters) MapCreatures(context.Context, string, []string) ([]link.MapCreature, error) {
	return nil, errors.New("not in this test")
}

type noLive struct{}

func (noLive) OnScreen(context.Context, string) (string, string, error) {
	return "", "", errors.New("not in this test")
}

func (noLive) Publish(string, bool, *playv1.WatchGameSessionResponse) {}

func (noLive) ImageOnStage(context.Context, string, string) (bool, error) {
	return false, errors.New("not in this test")
}

func (noLive) ImageShown(context.Context, pgx.Tx, string, string) (bool, error) {
	return false, errors.New("not in this test")
}

func (noLive) OpenScenePoint(context.Context, string) (string, error) {
	return "", errors.New("not in this test")
}

func (noLive) PublishToUsers(string, []string, *playv1.WatchGameSessionResponse) {}

func (noLive) PublishToUsersCoalesced(string, []string, string, *playv1.WatchGameSessionResponse) {}

func (noLive) AppendEvent(context.Context, pgx.Tx, string, string, string, []byte, time.Time) (bool, error) {
	return false, errors.New("not in this test")
}

func (noLive) OpenSessionID(context.Context, pgx.Tx, string) (string, error) {
	return "", errors.New("not in this test")
}

// noCombats stands in for the play module's combats.
type noCombats struct{}

func (noCombats) CombatRunsOnMap(context.Context, pgx.Tx, string, string) (bool, error) {
	return false, errors.New("not in this test")
}

func (noCombats) CombatPositions(context.Context, pgx.Tx, string, string) (link.CombatPositions, error) {
	return link.CombatPositions{}, errors.New("not in this test")
}

// noRules stands in for the rules module.
type noRules struct{}

func (noRules) SceneCheckName(string) (string, bool)         { return "", false }
func (noRules) NamePT(string) string                         { return "" }
func (noRules) TrapPreset(string) (rules.TrapPreset, bool)   { return rules.TrapPreset{}, false }
func (noRules) LightPreset(string) (rules.LightPreset, bool) { return rules.LightPreset{}, false }
func (noRules) GenerateTreasure(string, int, uint64) (rules.Treasure, error) {
	return rules.Treasure{}, errors.New("not in this test")
}
func (noRules) MagicItem(string) (rules.MagicItem, bool)      { return rules.MagicItem{}, false }
func (noRules) MagicItemValue(string) (rules.ItemValue, bool) { return rules.ItemValue{}, false }
func (noRules) Version() string                               { return "" }

// noMembers is a MembershipSource with no members at all.
type noMembers struct{}

func (noMembers) CampaignMembership(context.Context, string, string) (authz.Role, authz.Status, error) {
	return "", "", authz.ErrNotMember
}

func lazyPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), "postgresql://nobody@127.0.0.1:1/none")
	if err != nil {
		t.Fatalf("pgxpool.New() error = %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestNewValidatesItsConfig(t *testing.T) {
	t.Parallel()
	pool := lazyPool(t)
	for name, cfg := range map[string]Config{
		"Pool":       {Characters: noCharacters{}, Live: noLive{}, Rules: noRules{}, Combats: noCombats{}},
		"Characters": {Pool: pool, Live: noLive{}, Rules: noRules{}, Combats: noCombats{}},
		"Live":       {Pool: pool, Characters: noCharacters{}, Rules: noRules{}, Combats: noCombats{}},
		"Rules":      {Pool: pool, Characters: noCharacters{}, Live: noLive{}, Combats: noCombats{}},
		"Combats":    {Pool: pool, Characters: noCharacters{}, Live: noLive{}, Rules: noRules{}},
	} {
		if _, err := New(cfg); err == nil {
			t.Errorf("New() without %s succeeded", name)
		}
	}
	s, err := New(Config{Pool: pool, Characters: noCharacters{}, Live: noLive{}, Rules: noRules{}, Combats: noCombats{}})
	if err != nil || s.maxImages != DefaultMaxImages || s.maxBytes != DefaultMaxBytes ||
		s.maxMaps != DefaultMaxMaps || s.maxPoints != DefaultMaxPointsPerMap {
		t.Errorf("New() = %+v, %v; want the default limits", s, err)
	}
}

// TestEveryMethodNeedsASession calls every method and route signed out:
// unauthenticated (401), never cacheable. Reads are POST-only.
func TestEveryMethodNeedsASession(t *testing.T) {
	t.Parallel()
	blobs, err := blob.NewFS(t.TempDir())
	if err != nil {
		t.Fatalf("blob.NewFS() error = %v", err)
	}
	t.Cleanup(func() { _ = blobs.Close() })
	svc, err := New(Config{Pool: lazyPool(t), Blobs: blobs, Characters: noCharacters{}, Live: noLive{}, Rules: noRules{}, Combats: noCombats{}, Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	mux := http.NewServeMux()
	svc.Mount(mux.Handle, testSessions, noMembers{}, connect.WithRequireConnectProtocolHeader())
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	c := mapsv1connect.NewGalleryServiceClient(server.Client(), server.URL)
	mc := mapsv1connect.NewMapServiceClient(server.Client(), server.URL)
	ctx := t.Context()
	id := "6f1c7a52-3b5e-4c55-9d0b-2a51f0c1e001"

	calls := map[string]error{}
	_, calls["ListGalleryImages"] = c.ListGalleryImages(ctx, connect.NewRequest(&mapsv1.ListGalleryImagesRequest{CampaignId: id}))
	_, calls["RenameGalleryImage"] = c.RenameGalleryImage(ctx, connect.NewRequest(&mapsv1.RenameGalleryImageRequest{CampaignId: id, ImageId: id, Name: "X"}))
	_, calls["DeleteGalleryImage"] = c.DeleteGalleryImage(ctx, connect.NewRequest(&mapsv1.DeleteGalleryImageRequest{CampaignId: id, ImageId: id}))
	methods := mapsv1.File_meurpg_maps_v1_gallery_proto.Services().ByName("GalleryService").Methods()
	if len(calls) != methods.Len() {
		t.Errorf("called %d GalleryService methods, the service has %d", len(calls), methods.Len())
	}
	mapCalls := map[string]error{}
	_, mapCalls["ListMaps"] = mc.ListMaps(ctx, connect.NewRequest(&mapsv1.ListMapsRequest{CampaignId: id}))
	_, mapCalls["GetMap"] = mc.GetMap(ctx, connect.NewRequest(&mapsv1.GetMapRequest{CampaignId: id, MapId: id}))
	_, mapCalls["CreateMap"] = mc.CreateMap(ctx, connect.NewRequest(&mapsv1.CreateMapRequest{CampaignId: id, Name: "X", ImageId: id}))
	_, mapCalls["UpdateMap"] = mc.UpdateMap(ctx, connect.NewRequest(&mapsv1.UpdateMapRequest{CampaignId: id, MapId: id, Revision: 1, Name: new("X")}))
	_, mapCalls["DeleteMap"] = mc.DeleteMap(ctx, connect.NewRequest(&mapsv1.DeleteMapRequest{CampaignId: id, MapId: id}))
	_, mapCalls["SetMapRevealed"] = mc.SetMapRevealed(ctx, connect.NewRequest(&mapsv1.SetMapRevealedRequest{CampaignId: id, MapId: id, Revealed: true}))
	_, mapCalls["SetMapGrid"] = mc.SetMapGrid(ctx, connect.NewRequest(&mapsv1.SetMapGridRequest{CampaignId: id, MapId: id, Columns: 20}))
	_, mapCalls["CreateMapPoint"] = mc.CreateMapPoint(ctx, connect.NewRequest(&mapsv1.CreateMapPointRequest{CampaignId: id, MapId: id, Kind: mapsv1.MapPointKind_MAP_POINT_KIND_SCENE, Name: "X"}))
	_, mapCalls["UpdateMapPoint"] = mc.UpdateMapPoint(ctx, connect.NewRequest(&mapsv1.UpdateMapPointRequest{CampaignId: id, MapId: id, PointId: id, Name: new("X")}))
	_, mapCalls["DeleteMapPoint"] = mc.DeleteMapPoint(ctx, connect.NewRequest(&mapsv1.DeleteMapPointRequest{CampaignId: id, MapId: id, PointId: id}))
	_, mapCalls["SetMapPointRevealed"] = mc.SetMapPointRevealed(ctx, connect.NewRequest(&mapsv1.SetMapPointRevealedRequest{CampaignId: id, MapId: id, PointId: id, Revealed: true}))
	_, mapCalls["AddSceneAction"] = mc.AddSceneAction(ctx, connect.NewRequest(&mapsv1.AddSceneActionRequest{CampaignId: id, MapId: id, PointId: id, Key: "skill:arcana"}))
	_, mapCalls["UpdateSceneAction"] = mc.UpdateSceneAction(ctx, connect.NewRequest(&mapsv1.UpdateSceneActionRequest{CampaignId: id, MapId: id, PointId: id, ActionId: id, Dc: proto.Int32(10)}))
	_, mapCalls["MoveSceneAction"] = mc.MoveSceneAction(ctx, connect.NewRequest(&mapsv1.MoveSceneActionRequest{CampaignId: id, MapId: id, PointId: id, ActionId: id, Direction: mapsv1.SceneActionDirection_SCENE_ACTION_DIRECTION_UP}))
	_, mapCalls["RemoveSceneAction"] = mc.RemoveSceneAction(ctx, connect.NewRequest(&mapsv1.RemoveSceneActionRequest{CampaignId: id, MapId: id, PointId: id, ActionId: id}))
	_, mapCalls["AddSceneClue"] = mc.AddSceneClue(ctx, connect.NewRequest(&mapsv1.AddSceneClueRequest{CampaignId: id, MapId: id, PointId: id, Text: "X"}))
	_, mapCalls["UpdateSceneClue"] = mc.UpdateSceneClue(ctx, connect.NewRequest(&mapsv1.UpdateSceneClueRequest{CampaignId: id, MapId: id, PointId: id, ClueId: id, Text: "X"}))
	_, mapCalls["MoveSceneClue"] = mc.MoveSceneClue(ctx, connect.NewRequest(&mapsv1.MoveSceneClueRequest{CampaignId: id, MapId: id, PointId: id, ClueId: id, Direction: mapsv1.SceneActionDirection_SCENE_ACTION_DIRECTION_UP}))
	_, mapCalls["RemoveSceneClue"] = mc.RemoveSceneClue(ctx, connect.NewRequest(&mapsv1.RemoveSceneClueRequest{CampaignId: id, MapId: id, PointId: id, ClueId: id}))
	_, mapCalls["RevealSceneClue"] = mc.RevealSceneClue(ctx, connect.NewRequest(&mapsv1.RevealSceneClueRequest{CampaignId: id, ClueId: id, CharacterIds: []string{id}}))
	_, mapCalls["PlaceMapToken"] = mc.PlaceMapToken(ctx, connect.NewRequest(&mapsv1.PlaceMapTokenRequest{CampaignId: id, MapId: id, CharacterId: id}))
	_, mapCalls["SetMapTokenHidden"] = mc.SetMapTokenHidden(ctx, connect.NewRequest(&mapsv1.SetMapTokenHiddenRequest{CampaignId: id, MapId: id, CharacterId: id, Hidden: true}))
	_, mapCalls["RemoveMapToken"] = mc.RemoveMapToken(ctx, connect.NewRequest(&mapsv1.RemoveMapTokenRequest{CampaignId: id, MapId: id, CharacterId: id}))
	_, mapCalls["PaintMapCells"] = mc.PaintMapCells(ctx, connect.NewRequest(&mapsv1.PaintMapCellsRequest{CampaignId: id, MapId: id, Layer: mapsv1.MapLayer_MAP_LAYER_WALL, Value: 1, Squares: []*mapsv1.MapSquare{{}}}))
	_, mapCalls["GetMapLayers"] = mc.GetMapLayers(ctx, connect.NewRequest(&mapsv1.GetMapLayersRequest{CampaignId: id, MapId: id}))
	_, mapCalls["SetMapFog"] = mc.SetMapFog(ctx, connect.NewRequest(&mapsv1.SetMapFogRequest{CampaignId: id, MapId: id, FogEnabled: new(true)}))
	_, mapCalls["RevealTrap"] = mc.RevealTrap(ctx, connect.NewRequest(&mapsv1.RevealTrapRequest{CampaignId: id, MapId: id, PointId: id, All: true}))
	_, mapCalls["GetTrapNoticers"] = mc.GetTrapNoticers(ctx, connect.NewRequest(&mapsv1.GetTrapNoticersRequest{CampaignId: id, MapId: id, PointId: id}))
	_, mapCalls["DisarmTrap"] = mc.DisarmTrap(ctx, connect.NewRequest(&mapsv1.DisarmTrapRequest{CampaignId: id, MapId: id, PointId: id}))
	_, mapCalls["MarkTreasureFound"] = mc.MarkTreasureFound(ctx, connect.NewRequest(&mapsv1.MarkTreasureFoundRequest{CampaignId: id, MapId: id, PointId: id, CharacterIds: []string{id}}))
	_, mapCalls["UnmarkTreasureFound"] = mc.UnmarkTreasureFound(ctx, connect.NewRequest(&mapsv1.UnmarkTreasureFoundRequest{CampaignId: id, MapId: id, PointId: id}))
	_, mapCalls["GetMapVision"] = mc.GetMapVision(ctx, connect.NewRequest(&mapsv1.GetMapVisionRequest{CampaignId: id, MapId: id}))
	_, mapCalls["ForgetMapVision"] = mc.ForgetMapVision(ctx, connect.NewRequest(&mapsv1.ForgetMapVisionRequest{CampaignId: id, MapId: id}))
	_, mapCalls["SetCarriedLight"] = mc.SetCarriedLight(ctx, connect.NewRequest(&mapsv1.SetCarriedLightRequest{CampaignId: id, MapId: id, CharacterId: id, LightKey: "light:torch"}))
	mapMethods := mapsv1.File_meurpg_maps_v1_maps_proto.Services().ByName("MapService").Methods()
	if len(mapCalls) != mapMethods.Len() {
		t.Errorf("called %d MapService methods, the service has %d", len(mapCalls), mapMethods.Len())
	}
	maps.Copy(calls, mapCalls)
	for name, err := range calls {
		if connect.CodeOf(err) != connect.CodeUnauthenticated {
			t.Errorf("%s signed out: error = %v, want unauthenticated", name, err)
		}
		if ce, ok := errors.AsType[*connect.Error](err); !ok || !slices.Equal(ce.Meta().Values("Cache-Control"), []string{"no-store"}) {
			t.Errorf("%s: error Cache-Control = %q, want no-store, once", name, ce.Meta().Values("Cache-Control"))
		}
	}
	for method, desc := range map[string]protoreflect.MethodDescriptor{
		"ListGalleryImages": methods.ByName("ListGalleryImages"),
		"ListMaps":          mapMethods.ByName("ListMaps"),
		"GetMap":            mapMethods.ByName("GetMap"),
		"GetMapLayers":      mapMethods.ByName("GetMapLayers"),
		"GetMapVision":      mapMethods.ByName("GetMapVision"),
	} {
		opts, _ := desc.Options().(*descriptorpb.MethodOptions)
		if opts.GetIdempotencyLevel() != descriptorpb.MethodOptions_IDEMPOTENT {
			t.Errorf("%s idempotency_level = %v, want IDEMPOTENT", method, opts.GetIdempotencyLevel())
		}
	}

	for _, r := range []struct{ method, path string }{
		{http.MethodPost, UploadPath},
		{http.MethodGet, ImagesPath + id},
		{http.MethodGet, ImagesPath + id + "/thumb"},
	} {
		req := httptest.NewRequestWithContext(ctx, r.method, r.path, strings.NewReader(""))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), `"code":"unauthenticated"`) || rec.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("%s %s signed out: status %d, body %s, Cache-Control %q; want 401 unauthenticated, no-store",
				r.method, r.path, rec.Code, rec.Body, rec.Header().Get("Cache-Control"))
		}
	}
}
