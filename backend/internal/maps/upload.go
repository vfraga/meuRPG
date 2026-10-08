package maps

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
	"uuid"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/maps/images"
	"github.com/PuraFome/meuRPG/backend/internal/maps/mapsdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/names"
	"github.com/PuraFome/meuRPG/backend/internal/platform/slowclient"
)

// maxUploadBody caps the whole request body: the image, plus room for the
// form's boundaries, part headers and campaign_id.
const maxUploadBody = images.MaxBytes + 64<<10

// uploadReadTimeout is how long a client has to send the whole body: 10 MiB in
// 2 minutes is a little under 1 Mbit/s, which a phone on a bad connection reaches.
const uploadReadTimeout = 2 * time.Minute

// downloadWriteTimeout is how long a client has to take a whole image,
// thumbnail or tile: the same two minutes as the upload, for the same 10 MiB
// on a slow phone. A client that stops reading is dropped after it.
const downloadWriteTimeout = 2 * time.Minute

// defaultImageName names an image whose file name leaves nothing usable.
const defaultImageName = "Imagem"

// handleUpload serves POST /uploads/images: a multipart form with
// campaign_id, then file. See the GalleryService comment in
// proto/meurpg/maps/v1/gallery.proto for the whole contract.
//
// CSRF: the route changes state and is not a Connect RPC, so the Connect
// protocol header does not protect it. http.CrossOriginProtection, around
// the whole server (package httpserver), refuses a cross-origin POST before
// it gets here, and the session cookie is SameSite=Lax, so another site's
// form never carries it.
func (s *Service) handleUpload(w http.ResponseWriter, r *http.Request) {
	img, err := s.upload(w, r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	body, err := protojson.Marshal(imageToProto(img))
	if err != nil {
		s.writeError(w, r, fmt.Errorf("encode the image: %w", err))
		return
	}
	header := w.Header()
	header.Set("Content-Type", "application/json")
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("Cache-Control", "no-store")
	header.Set("Location", imageURL(img.ID))
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write(body) //nolint:gosec // G705: JSON from protojson, sent as application/json with nosniff, never as HTML
}

// upload reads, checks and stores an uploaded image, and returns its row.
// The order matters: who is calling and the campaign come before the file,
// so a caller who may not upload never gets 10 MiB read, let alone decoded.
func (s *Service) upload(w http.ResponseWriter, r *http.Request) (mapsdb.GalleryImage, error) {
	ctx := r.Context()
	if _, err := authz.RequireSignedIn(ctx); err != nil {
		return mapsdb.GalleryImage{}, err
	}

	// A client that trickles the body must not hold the connection for the 35
	// minutes Cloud Run allows: 10 MiB in 2 minutes is under 1 Mbit/s.
	slowclient.ReadBody(w, uploadReadTimeout)
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBody)
	form, err := r.MultipartReader()
	if err != nil {
		return mapsdb.GalleryImage{}, invalid(ReasonMalformedRequest, "send a multipart/form-data body")
	}
	campaignID, err := readTextField(form, "campaign_id")
	if err != nil {
		return mapsdb.GalleryImage{}, err
	}
	m, err := authz.RequireCampaignRole(ctx, campaignID, authz.RoleMaster)
	if err != nil {
		return mapsdb.GalleryImage{}, err
	}
	// A quick look at the quota, so a full gallery refuses right away. The
	// check that counts is the one inside the insert's transaction (store).
	usage, err := s.queries.GetGalleryUsage(ctx, m.CampaignID)
	if err != nil {
		return mapsdb.GalleryImage{}, s.dbError(ctx, "read the gallery usage", err)
	}
	if usage.ImageCount >= s.maxImages || usage.ByteCount >= int64(s.maxBytes) {
		return mapsdb.GalleryImage{}, errQuota()
	}

	// The slot is taken before the file is read, not after: a body of up to
	// 10 MiB waiting for its turn would otherwise sit in memory, and a few of
	// them would not fit the instance. So at most one uploaded file is in
	// memory at a time, from the read to the end of the decode. The client's
	// time to send the file starts when its turn does.
	release, err := s.acquireProcessing(ctx)
	if err != nil {
		return mapsdb.GalleryImage{}, err
	}
	slowclient.ReadBody(w, uploadReadTimeout)
	name, data, err := readFile(form)
	if err != nil {
		release()
		return mapsdb.GalleryImage{}, err
	}
	res, err := decodeUpload(data)
	release()
	if err != nil {
		return mapsdb.GalleryImage{}, err
	}
	return s.store(ctx, m, name, res)
}

// readTextField reads the form's next part, which must be the text field
// name, of at most 64 bytes.
func readTextField(form *multipart.Reader, name string) (string, error) {
	part, err := form.NextPart()
	if err != nil {
		return "", formError(err, "the form must start with "+name)
	}
	if part.FormName() != name || part.FileName() != "" {
		return "", invalid(ReasonMalformedRequest, "the form must start with "+name)
	}
	const maxLength = 64
	value, err := io.ReadAll(io.LimitReader(part, maxLength+1))
	if err != nil {
		return "", formError(err, "cannot read "+name)
	}
	if len(value) > maxLength {
		return "", invalid(ReasonMalformedRequest, name+" is too long")
	}
	return string(value), nil
}

// readFile reads the form's next part, which must be the file field, and
// checks that nothing follows it. It returns the image's first name and
// the file's bytes.
func readFile(form *multipart.Reader) (string, []byte, error) {
	part, err := form.NextPart()
	if err != nil {
		return "", nil, formError(err, "the form must have a file field after campaign_id")
	}
	if part.FormName() != "file" {
		return "", nil, invalid(ReasonMalformedRequest, "the form must have a file field after campaign_id")
	}
	name := imageName(part.FileName())
	// One byte over the limit is enough to know the file is too large.
	data, err := io.ReadAll(io.LimitReader(part, images.MaxBytes+1))
	if err != nil {
		return "", nil, formError(err, "cannot read the file")
	}
	if len(data) > images.MaxBytes {
		return "", nil, errTooLarge()
	}
	if _, err := form.NextPart(); !errors.Is(err, io.EOF) {
		return "", nil, formError(err, "the form must have only campaign_id and file")
	}
	return name, data, nil
}

// formError is the answer for a form that could not be read: 413 when the
// body went over maxUploadBody, else a malformed request.
func formError(err error, message string) error {
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return invalid(ReasonMalformedRequest, "the upload took too long")
	}
	if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
		return errTooLarge()
	}
	return invalid(ReasonMalformedRequest, message)
}

// acquireProcessing waits for the one-at-a-time slot (Service.processing),
// where images are read into memory and decoded, and returns the function that
// gives it back.
func (s *Service) acquireProcessing(ctx context.Context) (release func(), err error) {
	select {
	case s.processing <- struct{}{}:
		return func() { <-s.processing }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// process checks and re-encodes an image, one at a time (Service.processing).
func (s *Service) process(ctx context.Context, data []byte) (*images.Result, error) {
	release, err := s.acquireProcessing(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	return decodeUpload(data)
}

// decodeUpload checks and re-encodes an image; the caller holds the processing
// slot.
func decodeUpload(data []byte) (*images.Result, error) {
	res, err := images.Process(data)
	switch {
	case err == nil:
		return res, nil
	case errors.Is(err, images.ErrUnsupportedType):
		return nil, invalid(ReasonUnsupportedType, "only JPEG, PNG and WebP images are accepted")
	case errors.Is(err, images.ErrTooLarge):
		return nil, errTooLarge()
	case errors.Is(err, images.ErrDimensions):
		return nil, invalid(ReasonDimensions, "the image has too many pixels")
	case errors.Is(err, images.ErrCorrupt):
		return nil, invalid(ReasonCorrupt, "the image cannot be read")
	default:
		return nil, err
	}
}

// store writes the image's files, then its row, inside a transaction that
// checks the quota. If anything fails after a file was written, the files
// are deleted again.
func (s *Service) store(ctx context.Context, m authz.Membership, name string, res *images.Result) (mapsdb.GalleryImage, error) {
	return s.storeImage(ctx, m.CampaignID, &m.UserID, name, res, false, "", nil, nil)
}

// storeImage is store for any image of the campaign: uploadedBy is who made it
// (nil when nobody is left to name), and a generated image (MR-039) says so and,
// with the way it was made (generatedKind) and, when it is an edit, names its parent (kept only if that image still exists
// inside the transaction: the master may have deleted it meanwhile). finish, when
// set, runs inside the same transaction after the insert, and an error from it
// rolls the image back and is returned as it is: the generated image and the
// request that made it are recorded together, or not at all.
func (s *Service) storeImage(ctx context.Context, campaignID string, uploadedBy *string, name string, res *images.Result, generated bool, generatedKind string, parent *string, finish func(ctx context.Context, q *mapsdb.Queries, imageID string) error) (mapsdb.GalleryImage, error) {
	m := authz.Membership{CampaignID: campaignID}
	id := uuid.New().String()
	imageKey, thumbnailKey := blobKeys(m.CampaignID, id)
	type file struct {
		key         string
		contentType string
		content     []byte
	}
	files := []file{{imageKey, res.ContentType, res.Data}, {thumbnailKey, res.ContentType, res.Thumbnail}}
	if res.Reference != nil {
		files = append(files, file{referenceKey(m.CampaignID, id), images.JPEG, res.Reference})
	}
	for _, f := range files {
		if err := s.blobs.Put(ctx, f.key, f.contentType, bytes.NewReader(f.content)); err != nil {
			s.deleteFiles(ctx, m.CampaignID, id)
			s.logger.ErrorContext(ctx, "maps: cannot store an image file", "error", err)
			return mapsdb.GalleryImage{}, errStorage()
		}
	}

	size := int32(len(res.Data)) //nolint:gosec // G115: at most images.MaxBytes
	var row mapsdb.GalleryImage
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		usage, err := q.GetGalleryUsage(ctx, m.CampaignID)
		if err != nil {
			return fmt.Errorf("read the gallery usage: %w", err)
		}
		if usage.ImageCount >= s.maxImages || usage.ByteCount+int64(size) > int64(s.maxBytes) {
			return errQuota()
		}
		parentID := parent
		if parentID != nil {
			if _, err := q.GetGalleryImageInCampaign(ctx, mapsdb.GetGalleryImageInCampaignParams{CampaignID: m.CampaignID, ID: *parentID}); errors.Is(err, pgx.ErrNoRows) {
				parentID = nil
			} else if err != nil {
				return fmt.Errorf("find the parent image: %w", err)
			}
		}
		row, err = q.InsertGalleryImage(ctx, mapsdb.InsertGalleryImageParams{
			ID:            id,
			CampaignID:    m.CampaignID,
			UploadedBy:    uploadedBy,
			Name:          name,
			ContentType:   res.ContentType,
			Width:         int32(res.Width),  //nolint:gosec // G115: at most images.MaxSide
			Height:        int32(res.Height), //nolint:gosec // G115: at most images.MaxSide
			ByteSize:      size,
			CreatedAt:     s.now(),
			Generated:     generated,
			ParentImageID: parentID,
			GeneratedKind: generatedKind,
		})
		if err != nil {
			return fmt.Errorf("insert gallery image: %w", err)
		}
		if finish != nil {
			return finish(ctx, q, id)
		}
		return nil
	})
	if err != nil {
		s.deleteFiles(ctx, m.CampaignID, id)
		if errors.Is(err, errRequestClosed) {
			return mapsdb.GalleryImage{}, err
		}
		if he, ok := errors.AsType[*httpError](err); ok {
			return mapsdb.GalleryImage{}, he
		}
		return mapsdb.GalleryImage{}, s.dbError(ctx, "save an uploaded image", err)
	}
	return row, nil
}

// errStorage is the answer when the blob store fails.
func errStorage() error {
	return &httpError{
		status: http.StatusServiceUnavailable, code: connect.CodeUnavailable,
		message: "cannot store the image right now, please try again",
	}
}

// imageName makes an image's first name from the uploaded file's name: the
// name without folders or extension, without control or invisible
// direction characters, cut to 80 characters. When nothing usable is
// left, the image is called "Imagem"; the master can rename it.
func imageName(fileName string) string {
	// Browsers send only the file's own name; a few old ones sent the path.
	if i := strings.LastIndexAny(fileName, `/\`); i >= 0 {
		fileName = fileName[i+1:]
	}
	fileName = strings.TrimSuffix(fileName, path.Ext(fileName))
	fileName = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Bidi_Control, r) {
			return -1
		}
		return r
	}, strings.ToValidUTF8(fileName, ""))
	fileName = strings.TrimSpace(fileName)
	if utf8.RuneCountInString(fileName) > maxNameLength {
		fileName = strings.TrimSpace(string([]rune(fileName)[:maxNameLength]))
	}
	if name, err := names.Clean(fileName, maxNameLength); err == nil {
		return name
	}
	return defaultImageName
}
