package maps

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
	"uuid"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/protobuf/types/known/timestamppb"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/maps/images"
	"github.com/PuraFome/meuRPG/backend/internal/maps/images/gen"
	"github.com/PuraFome/meuRPG/backend/internal/maps/mapsdb"
	"github.com/PuraFome/meuRPG/backend/internal/maps/refimg"
	"github.com/PuraFome/meuRPG/backend/internal/platform/blob"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/platform/logging"
	"github.com/PuraFome/meuRPG/backend/internal/platform/names"
	"github.com/PuraFome/meuRPG/backend/internal/platform/safego"
)

// ImageGenerationService (MR-039, RN-28, ADR-0019): pictures made by an image
// model, for the master. How a request goes:
//
//  1. A short transaction reserves one of the campaign's monthly slots, by
//     inserting the request row (reserve).
//  2. A goroutine calls the model, outside any transaction (run).
//  3. The picture is stored like an upload, as a hidden gallery image, or the
//     slot is refunded.
//
// The slot is spent once the request has left the server ("sent_at") and the
// picture is in the gallery, or the master canceled after it left. Anything
// that ends without a picture gives the slot back.

// DefaultMonthlyImages is how many images a campaign may generate per month
// (a proposal; docs/operations.md has the cost behind it).
const DefaultMonthlyImages = 20

// DefaultDailyImages is how many images the whole server may generate per day
// (Brazil's day), on top of each campaign's monthly limit: with one table and
// 20 a month, a day is never close, so the cap only bites when someone is
// abusing it, and it bounds the Gemini bill (docs/operations.md).
const DefaultDailyImages = 100

const (
	// maxPromptCharacters is the longest master text, in characters. The
	// image_requests_prompt_length CHECK says the same.
	maxPromptCharacters = 500
	// maxGenerating is how many calls to the model run at once on this server.
	maxGenerating = 1
	// maxPendingRequests is how many image requests may be alive at once on this
	// server, the one calling the model and the ones waiting for its slot. Each
	// keeps its shrunk references in memory (up to about 6 MiB) until its call goes
	// out, so the wait is bounded to what the 512 MiB instance has room for.
	maxPendingRequests = 5
	// maxLongPoll is the longest wait_seconds of GetImageGeneration.
	maxLongPoll = 25
	// generationTimeout bounds a request from the goroutine's start to its end
	// (two attempts of the model's two minutes, and the storing).
	generationTimeout = 6 * time.Minute
	// staleAfter is when a request still pending is taken for lost (a restart).
	staleAfter = 10 * time.Minute
	// defaultSceneRatio is the ratio of a scene when the master picks none.
	defaultSceneRatio = "16:9"
)

// The request kinds and states as the table writes them.
const (
	kindScene = "scene"
	kindEdit  = "edit"

	statePending  = "pending"
	stateDone     = "done"
	stateRefused  = "refused"
	stateFailed   = "failed"
	stateCanceled = "canceled"
)

// The reasons a request ended without a picture, as the table writes them.
const (
	reasonNoImage      = "no_image"
	reasonRefused      = "refused"
	reasonUnavailable  = "unavailable"
	reasonGalleryFull  = "gallery_full"
	reasonImageMissing = "image_missing"
	reasonTimeout      = "timeout"
	reasonServiceOff   = "service_off"
	reasonShutdown     = "shutdown"
)

// brazil is the time zone of the month's count: Brazil has had no daylight
// saving time since 2019, so a fixed UTC-3 needs no tz database.
var brazil = time.FixedZone("BRT", -3*60*60)

// monthOf names the month t falls in ("2026-10") and says when the next one starts.
func monthOf(t time.Time) (key string, resets time.Time) {
	t = t.In(brazil)
	start := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, brazil)
	return start.Format("2006-01"), start.AddDate(0, 1, 0)
}

// dayStart is the first instant of t's day in Brazil's time, where the daily
// cap on the whole server's images starts over.
func dayStart(t time.Time) time.Time {
	t = t.In(brazil)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, brazil)
}

// stylePhrases and ratios turn the API's enums into what goes to the model.
var stylePhrases = map[mapsv1.ImageStyle]string{
	mapsv1.ImageStyle_IMAGE_STYLE_OIL_PAINTING:      "oil painting",
	mapsv1.ImageStyle_IMAGE_STYLE_WATERCOLOR:        "watercolor",
	mapsv1.ImageStyle_IMAGE_STYLE_INK:               "ink drawing",
	mapsv1.ImageStyle_IMAGE_STYLE_DIGITAL_ART:       "digital art",
	mapsv1.ImageStyle_IMAGE_STYLE_BOOK_ILLUSTRATION: "book illustration",
}

var ratioNames = map[mapsv1.ImageAspectRatio]string{
	mapsv1.ImageAspectRatio_IMAGE_ASPECT_RATIO_1_1:  "1:1",
	mapsv1.ImageAspectRatio_IMAGE_ASPECT_RATIO_3_2:  "3:2",
	mapsv1.ImageAspectRatio_IMAGE_ASPECT_RATIO_2_3:  "2:3",
	mapsv1.ImageAspectRatio_IMAGE_ASPECT_RATIO_3_4:  "3:4",
	mapsv1.ImageAspectRatio_IMAGE_ASPECT_RATIO_4_3:  "4:3",
	mapsv1.ImageAspectRatio_IMAGE_ASPECT_RATIO_4_5:  "4:5",
	mapsv1.ImageAspectRatio_IMAGE_ASPECT_RATIO_5_4:  "5:4",
	mapsv1.ImageAspectRatio_IMAGE_ASPECT_RATIO_9_16: "9:16",
	mapsv1.ImageAspectRatio_IMAGE_ASPECT_RATIO_16_9: "16:9",
	mapsv1.ImageAspectRatio_IMAGE_ASPECT_RATIO_21_9: "21:9",
}

// styleKeys are the names the table keeps for the styles ("" is none).
var styleKeys = map[mapsv1.ImageStyle]string{
	mapsv1.ImageStyle_IMAGE_STYLE_OIL_PAINTING:      "oil_painting",
	mapsv1.ImageStyle_IMAGE_STYLE_WATERCOLOR:        "watercolor",
	mapsv1.ImageStyle_IMAGE_STYLE_INK:               "ink",
	mapsv1.ImageStyle_IMAGE_STYLE_DIGITAL_ART:       "digital_art",
	mapsv1.ImageStyle_IMAGE_STYLE_BOOK_ILLUSTRATION: "book_illustration",
}

func styleFromKey(key string) mapsv1.ImageStyle {
	for style, k := range styleKeys {
		if k == key {
			return style
		}
	}
	return mapsv1.ImageStyle_IMAGE_STYLE_UNSPECIFIED
}

func ratioFromName(name string) mapsv1.ImageAspectRatio {
	for ratio, n := range ratioNames {
		if n == name {
			return ratio
		}
	}
	return mapsv1.ImageAspectRatio_IMAGE_ASPECT_RATIO_UNSPECIFIED
}

// requireGenerationMaster is the check at the top of every handler: the
// campaign's master. Everyone else, a player included, gets `not_found`, as
// for a hidden image: nothing says that this campaign generates images.
func requireGenerationMaster(ctx context.Context, campaignID string) (authz.Membership, error) {
	m, err := authz.RequireCampaignMember(ctx, campaignID)
	if err != nil {
		return authz.Membership{}, err
	}
	if m.Role != authz.RoleMaster {
		return authz.Membership{}, connect.NewError(connect.CodeNotFound, errors.New("campaign not found"))
	}
	return m, nil
}

// generationOn says whether this server can generate: a generator (the key, or
// the fake) and somewhere to keep the pictures.
func (s *Service) generationOn() bool { return s.generator != nil && s.blobs != nil }

// statusIn reads the campaign's month inside q's transaction or pool.
func (s *Service) statusIn(ctx context.Context, q *mapsdb.Queries, campaignID string) (*mapsv1.ImageGenerationStatus, error) {
	now := s.now()
	month, resets := monthOf(now)
	used, err := q.CountImageSlots(ctx, mapsdb.CountImageSlotsParams{CampaignID: campaignID, QuotaMonth: month})
	if err != nil {
		return nil, fmt.Errorf("count the month's images: %w", err)
	}
	return &mapsv1.ImageGenerationStatus{
		Enabled:                s.generationOn(),
		MonthlyLimit:           s.monthlyImages,
		UsedThisMonth:          used,
		Remaining:              max(s.monthlyImages-used, 0),
		Month:                  month,
		ResetsAt:               timestamppb.New(resets),
		MaxPromptCharacters:    maxPromptCharacters,
		MaxObjectReferences:    gen.MaxObjectReferences,
		MaxCharacterReferences: gen.MaxCharacterReferences,
	}, nil
}

// errBlocked is the failed_precondition with the ImageGenerationBlocked detail.
func errBlocked(reason mapsv1.ImageGenerationBlockedReason, status *mapsv1.ImageGenerationStatus) error {
	message := map[mapsv1.ImageGenerationBlockedReason]string{
		mapsv1.ImageGenerationBlockedReason_IMAGE_GENERATION_BLOCKED_REASON_OFF:                 "image generation is not on in this server",
		mapsv1.ImageGenerationBlockedReason_IMAGE_GENERATION_BLOCKED_REASON_LIMIT_REACHED:       "the campaign used its images of the month",
		mapsv1.ImageGenerationBlockedReason_IMAGE_GENERATION_BLOCKED_REASON_GALLERY_FULL:        "the campaign's gallery is full",
		mapsv1.ImageGenerationBlockedReason_IMAGE_GENERATION_BLOCKED_REASON_DAILY_LIMIT_REACHED: "the server made all its images of the day, try again tomorrow",
		mapsv1.ImageGenerationBlockedReason_IMAGE_GENERATION_BLOCKED_REASON_REQUEST_TOO_LARGE:   "the references are too big to send together",
	}[reason]
	err := connect.NewError(connect.CodeFailedPrecondition, errors.New(message))
	if detail, detailErr := connect.NewErrorDetail(&mapsv1.ImageGenerationBlocked{Reason: reason, Status: status}); detailErr == nil {
		err.AddDetail(detail)
	}
	return err
}

// GetImageGenerationStatus implements mapsv1connect.ImageGenerationServiceHandler.
func (s *Service) GetImageGenerationStatus(
	ctx context.Context,
	req *connect.Request[mapsv1.GetImageGenerationStatusRequest],
) (*connect.Response[mapsv1.GetImageGenerationStatusResponse], error) {
	m, err := requireGenerationMaster(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	status, err := s.freshStatus(ctx, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "read the image generation status", err)
	}
	return connect.NewResponse(&mapsv1.GetImageGenerationStatusResponse{Status: status}), nil
}

// newRequest is what a handler checked and reserve needs to insert a row.
type newRequest struct {
	kind string
	key  string
	// hash is the hash of the whole request but its key (idem.Hash): a retry of the key has the
	// very same one, and the key reused for another request is refused.
	hash        *string
	prompt      string
	style       string
	ratio       string
	references  []string // gallery image IDs of objects
	characters  []string // gallery image IDs of characters
	source      *string  // an edit's image
	requestedBy string
	// name is the gallery image's name: the master's, or the default the server made (never "Imagem N").
	name string
	// mapReq is set for a request made from a map (the three kinds of imagegen_map.go).
	mapReq *mapRequest
	// prepared are the shrunk images the call will carry (prepare), made before
	// the slot is reserved.
	prepared prepared
}

// GenerateSceneImage implements mapsv1connect.ImageGenerationServiceHandler.
func (s *Service) GenerateSceneImage(
	ctx context.Context,
	req *connect.Request[mapsv1.GenerateSceneImageRequest],
) (*connect.Response[mapsv1.GenerateSceneImageResponse], error) {
	m, err := requireGenerationMaster(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	key, err := cleanKey(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	prompt, err := cleanPrompt(req.Msg.GetPrompt(), "prompt")
	if err != nil {
		return nil, err
	}
	style, ratio, err := styleAndRatio(req.Msg.GetStyle(), req.Msg.GetAspectRatio())
	if err != nil {
		return nil, err
	}
	objects, err := cleanImageIDs(req.Msg.GetObjectImageIds(), gen.MaxObjectReferences, "object_image_ids")
	if err != nil {
		return nil, err
	}
	characters, err := cleanImageIDs(req.Msg.GetCharacterImageIds(), gen.MaxCharacterReferences, "character_image_ids")
	if err != nil {
		return nil, err
	}
	refs := append(append([]string{}, objects...), characters...)
	if hasDuplicates(refs) {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("an image is listed twice as a reference"))
	}
	name, err := imageNameOf(req.Msg.GetName(), datedName("Arte da cena", s.now()))
	if err != nil {
		return nil, err
	}
	n := newRequest{
		kind: kindScene, key: key, hash: idem.Hash(req.Msg), prompt: prompt, style: style, ratio: ratio,
		references: objects, characters: characters, requestedBy: m.UserID, name: name,
	}
	row, status, err := s.begin(ctx, n, m.CampaignID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&mapsv1.GenerateSceneImageResponse{Generation: s.generationToProto(row), Status: status}), nil
}

// EditGeneratedImage implements mapsv1connect.ImageGenerationServiceHandler.
func (s *Service) EditGeneratedImage(
	ctx context.Context,
	req *connect.Request[mapsv1.EditGeneratedImageRequest],
) (*connect.Response[mapsv1.EditGeneratedImageResponse], error) {
	m, err := requireGenerationMaster(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	key, err := cleanKey(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	instruction, err := cleanPrompt(req.Msg.GetInstruction(), "instruction")
	if err != nil {
		return nil, err
	}
	imageID, err := uuid.Parse(req.Msg.GetImageId())
	if err != nil {
		return nil, errImageNotFound()
	}
	// An empty name is the source's, with " (ajuste)": reserve fills it in, with the source in hand.
	name, err := imageNameOf(req.Msg.GetName(), "")
	if err != nil {
		return nil, err
	}
	source := imageID.String()
	row, status, err := s.begin(ctx, newRequest{
		kind: kindEdit, key: key, hash: idem.Hash(req.Msg), prompt: instruction, source: &source, requestedBy: m.UserID, name: name,
	}, m.CampaignID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&mapsv1.EditGeneratedImageResponse{Generation: s.generationToProto(row), Status: status}), nil
}

// freshStatus reads the month after giving back the slots of requests that
// were lost (expireStale).
func (s *Service) freshStatus(ctx context.Context, campaignID string) (*mapsv1.ImageGenerationStatus, error) {
	var status *mapsv1.ImageGenerationStatus
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		if err := s.expireStale(ctx, q, campaignID); err != nil {
			return err
		}
		var err error
		status, err = s.statusIn(ctx, q, campaignID)
		return err
	})
	return status, err
}

// expireStale ends the campaign's requests that lost their server: one that
// never left refunds its slot; one that left fails as timed out and the slot
// stays spent (it may have been billed).
func (s *Service) expireStale(ctx context.Context, q *mapsdb.Queries, campaignID string) error {
	now, before := s.now(), s.now().Add(-staleAfter)
	if err := q.ExpireUnsentImageRequests(ctx, mapsdb.ExpireUnsentImageRequestsParams{Now: &now, CampaignID: campaignID, Before: before}); err != nil {
		return fmt.Errorf("expire unsent requests: %w", err)
	}
	if err := q.ExpireSentImageRequests(ctx, mapsdb.ExpireSentImageRequestsParams{Now: &now, CampaignID: campaignID, Before: &before}); err != nil {
		return fmt.Errorf("expire sent requests: %w", err)
	}
	return nil
}

// prepared are the images a call carries, shrunk.
type prepared struct {
	references []gen.Reference // objects first, then characters
	previous   *gen.Image      // an edit's image
	// For a request made from a map (prepareMap): the drawing, the NPCs' portraits
	// that were added to the characters, the map as it was seen, the ratio the server
	// picked (the textured map's) and the rooms of a generated dungeon's textured map.
	drawing   *gen.Image
	npcImages []string
	basis     *mapBasis
	ratio     string
	rooms     []string
	// name is the default name of a picture made from a map (the map's, and the way).
	name string
}

// sameImageRequest says whether the request a key found is the one being retried: an image
// request from before the hash was kept has none, and is replayed as it was.
func sameImageRequest(existing mapsdb.ImageRequest, n newRequest) error {
	if existing.IdempotencyHash == nil || (n.hash != nil && *existing.IdempotencyHash == *n.hash) {
		return nil
	}
	return idem.ErrReused()
}

// begin checks what can be checked cheaply, prepares the images, and reserves
// the slot. The order spares the server: an earlier try with the same key is
// answered first, a server with generation off or a month with no slot left is
// refused before any image is read, and the images (read from the blob store
// and shrunk, one at a time in the upload slot) are prepared before the slot is
// reserved, so a request too big for the service never costs one.
func (s *Service) begin(ctx context.Context, n newRequest, campaignID string) (mapsdb.ImageRequest, *mapsv1.ImageGenerationStatus, error) {
	if existing, err := s.queries.GetImageRequestByKey(ctx, mapsdb.GetImageRequestByKeyParams{CampaignID: campaignID, IdempotencyKey: n.key}); err == nil {
		if err := sameImageRequest(existing, n); err != nil {
			return mapsdb.ImageRequest{}, nil, err
		}
		status, err := s.freshStatus(ctx, campaignID)
		if err != nil {
			return mapsdb.ImageRequest{}, nil, s.dbError(ctx, "read the image generation status", err)
		}
		return existing, status, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return mapsdb.ImageRequest{}, nil, s.dbError(ctx, "find an image request", err)
	}
	status, err := s.freshStatus(ctx, campaignID)
	if err != nil {
		return mapsdb.ImageRequest{}, nil, s.dbError(ctx, "read the image generation status", err)
	}
	switch {
	case !status.GetEnabled():
		return mapsdb.ImageRequest{}, nil, errBlocked(mapsv1.ImageGenerationBlockedReason_IMAGE_GENERATION_BLOCKED_REASON_OFF, status)
	case status.GetRemaining() <= 0:
		return mapsdb.ImageRequest{}, nil, errBlocked(mapsv1.ImageGenerationBlockedReason_IMAGE_GENERATION_BLOCKED_REASON_LIMIT_REACHED, status)
	}
	// The server's cap of the day, before any image is read and shrunk. reserve
	// checks it again in its transaction, which is the one that counts.
	if made, err := s.queries.CountImageRequestsSince(ctx, dayStart(s.now())); err != nil {
		return mapsdb.ImageRequest{}, nil, s.dbError(ctx, "count the day's generated images", err)
	} else if made >= s.dailyImages {
		return mapsdb.ImageRequest{}, nil, errBlocked(mapsv1.ImageGenerationBlockedReason_IMAGE_GENERATION_BLOCKED_REASON_DAILY_LIMIT_REACHED, status)
	}
	prep, err := s.prepare(ctx, campaignID, n, status)
	if err != nil {
		return mapsdb.ImageRequest{}, nil, err
	}
	n.prepared = prep
	// The NPCs' portraits are character references like any other: the row records them.
	n.characters = append(append([]string{}, n.characters...), prep.npcImages...)
	if prep.ratio != "" {
		n.ratio = prep.ratio
	}
	if n.name == "" {
		n.name = prep.name
	}
	stub := gen.Request{Prompt: n.prompt, References: prep.references, Edit: editStub(n, prep), Drawing: prep.drawing, Rooms: prep.rooms}
	if prep.drawing != nil {
		stub.Layout = layoutOf(n.kind)
	}
	if size := stub.BodySize(); size > gen.MaxBodyBytes {
		return mapsdb.ImageRequest{}, nil, errBlocked(mapsv1.ImageGenerationBlockedReason_IMAGE_GENERATION_BLOCKED_REASON_REQUEST_TOO_LARGE, status)
	}
	return s.reserve(ctx, n, campaignID)
}

// editStub is an edit's previous image in a Request, only to size the body.
func editStub(n newRequest, p prepared) *gen.Edit {
	if p.previous == nil {
		return nil
	}
	return &gen.Edit{Previous: *p.previous, Instruction: n.prompt}
}

// prepare reads the request's images from the blob store, as the campaign's own
// images (never a URL from the browser), and shrinks each one to a small JPEG.
// An image that is not the campaign's is `not_found`.
func (s *Service) prepare(ctx context.Context, campaignID string, n newRequest, status *mapsv1.ImageGenerationStatus) (prepared, error) {
	var p prepared
	if n.mapReq != nil {
		if err := s.prepareMap(ctx, campaignID, &n, &p, status); err != nil {
			return p, err
		}
	}
	if n.source != nil {
		src, err := s.queries.GetGalleryImageInCampaign(ctx, mapsdb.GetGalleryImageInCampaignParams{CampaignID: campaignID, ID: *n.source})
		if errors.Is(err, pgx.ErrNoRows) || err == nil && !src.Generated {
			return p, errImageNotFound()
		}
		if err != nil {
			return p, s.dbError(ctx, "find the image to edit", err)
		}
		img, err := s.shrunkImage(ctx, campaignID, *n.source)
		if err != nil {
			return p, err
		}
		p.previous = &img
	}
	for _, character := range []bool{false, true} {
		ids := n.references
		if character {
			ids = append(append([]string{}, n.characters...), p.npcImages...)
		}
		for _, id := range ids {
			img, err := s.shrunkImage(ctx, campaignID, id)
			if err != nil {
				return p, err
			}
			p.references = append(p.references, gen.Reference{Image: img, Character: character})
		}
	}
	return p, nil
}

// shrunkImage returns a gallery image as the small JPEG it travels as: its
// reference, kept next to it in the blob store (made at upload for an image
// bigger than images.ReferenceSide, so no later decode of the original). An
// image without one, an older one, is decoded once, inside the upload slot (one
// image decoded at a time on the whole server), and its reference is kept for
// next time; a small image has none to keep and is converted each time, which
// costs little.
func (s *Service) shrunkImage(ctx context.Context, campaignID, id string) (gen.Image, error) {
	if _, err := s.queries.GetGalleryImageInCampaign(ctx, mapsdb.GetGalleryImageInCampaignParams{CampaignID: campaignID, ID: id}); errors.Is(err, pgx.ErrNoRows) {
		return gen.Image{}, errImageNotFound()
	} else if err != nil {
		return gen.Image{}, s.dbError(ctx, "find a reference image", err)
	}
	refKey := referenceKey(campaignID, id)
	switch data, err := s.readBlob(ctx, refKey, images.MaxBytes); {
	case err == nil:
		return gen.Image{MimeType: images.JPEG, Data: data}, nil
	case !errors.Is(err, blob.ErrNotFound):
		return gen.Image{}, errStorage()
	}
	key, _ := blobKeys(campaignID, id)
	// The slot first, then the read: the file (at most 10 MiB) is in memory
	// only while it holds the slot, so waiting requests hold none.
	release, err := s.acquireProcessing(ctx)
	if err != nil {
		return gen.Image{}, err
	}
	data, err := s.readBlob(ctx, key, images.MaxBytes)
	if err != nil {
		release()
		s.logger.ErrorContext(ctx, "maps: cannot read an image file", "error", err)
		return gen.Image{}, errStorage()
	}
	if s.onReferenceDecode != nil {
		s.onReferenceDecode()
	}
	small, resized, err := images.Shrink(data, images.ReferenceSide, images.ReferenceQuality)
	release()
	if err != nil {
		s.logger.ErrorContext(ctx, "maps: cannot shrink a reference image", "error", err)
		return gen.Image{}, errStorage()
	}
	if resized {
		if err := s.blobs.Put(ctx, refKey, images.JPEG, bytes.NewReader(small)); err != nil {
			s.logger.WarnContext(ctx, "maps: cannot keep a reference image", "error", err)
		}
	}
	return gen.Image{MimeType: images.JPEG, Data: small}, nil
}

// readBlob reads a whole blob at once, into a buffer of its exact size (no
// growing, no second copy), refusing one over limit bytes.
func (s *Service) readBlob(ctx context.Context, key string, limit int64) ([]byte, error) {
	obj, err := s.blobs.Open(ctx, key)
	if err != nil {
		return nil, err
	}
	defer func() { _ = obj.Close() }()
	if obj.Size > limit {
		return nil, errors.New("the file is too large")
	}
	data := make([]byte, obj.Size)
	if _, err := io.ReadFull(obj.Content, data); err != nil {
		return nil, err
	}
	return data, nil
}

// errTooManyImageRequests is `resource_exhausted` for an image request made while the
// server already has as many alive as it holds in memory.
func errTooManyImageRequests() error {
	return connect.NewError(connect.CodeResourceExhausted, errors.New("too many images are being made right now; wait for one to finish"))
}

// reserve is step 1: in one short transaction it finds the request an earlier
// try with the same key made, or checks the cap and the gallery and inserts the
// row, which reserves the slot. It starts the goroutine of a new request after
// the commit. Every read goes through the transaction (the #121 rule).
func (s *Service) reserve(ctx context.Context, n newRequest, campaignID string) (mapsdb.ImageRequest, *mapsv1.ImageGenerationStatus, error) {
	var (
		row     mapsdb.ImageRequest
		status  *mapsv1.ImageGenerationStatus
		created bool
	)
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		created = false
		var (
			inherited    basisParams
			hasInherited bool
		)
		q := s.queries.WithTx(tx)
		// A retry with the same key: the first request, whatever it became.
		existing, err := q.GetImageRequestByKey(ctx, mapsdb.GetImageRequestByKeyParams{CampaignID: campaignID, IdempotencyKey: n.key})
		if err == nil {
			if err := sameImageRequest(existing, n); err != nil {
				return err
			}
			row = existing
			status, err = s.statusIn(ctx, q, campaignID)
			return err
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("find the request by its key: %w", err)
		}
		if err := s.expireStale(ctx, q, campaignID); err != nil {
			return err
		}
		if status, err = s.statusIn(ctx, q, campaignID); err != nil {
			return err
		}
		switch {
		case !status.GetEnabled():
			return errBlocked(mapsv1.ImageGenerationBlockedReason_IMAGE_GENERATION_BLOCKED_REASON_OFF, status)
		case status.GetRemaining() <= 0:
			return errBlocked(mapsv1.ImageGenerationBlockedReason_IMAGE_GENERATION_BLOCKED_REASON_LIMIT_REACHED, status)
		}
		// The server's cap of the day, counted in the transaction that inserts, so
		// two requests for the last slot cannot both get it.
		made, err := q.CountImageRequestsSince(ctx, dayStart(s.now()))
		if err != nil {
			return fmt.Errorf("count the day's generated images: %w", err)
		}
		if made >= s.dailyImages {
			return errBlocked(mapsv1.ImageGenerationBlockedReason_IMAGE_GENERATION_BLOCKED_REASON_DAILY_LIMIT_REACHED, status)
		}
		usage, err := q.GetGalleryUsage(ctx, campaignID)
		if err != nil {
			return fmt.Errorf("read the gallery usage: %w", err)
		}
		open, err := q.CountOpenImageRequests(ctx, mapsdb.CountOpenImageRequestsParams{CampaignID: campaignID, CreatedAt: s.now().Add(-staleAfter)})
		if err != nil {
			return fmt.Errorf("count the requests in flight: %w", err)
		}
		// The requests in flight will each put an image in the gallery: they count
		// against its room, so a full gallery cannot fail after the model was paid.
		if usage.ImageCount+open >= s.maxImages || usage.ByteCount >= int64(s.maxBytes) {
			return errBlocked(mapsv1.ImageGenerationBlockedReason_IMAGE_GENERATION_BLOCKED_REASON_GALLERY_FULL, status)
		}
		for _, id := range append(append([]string{}, n.references...), n.characters...) {
			if _, err := q.GetGalleryImageInCampaign(ctx, mapsdb.GetGalleryImageInCampaignParams{CampaignID: campaignID, ID: id}); errors.Is(err, pgx.ErrNoRows) {
				return errImageNotFound()
			} else if err != nil {
				return fmt.Errorf("find a reference image: %w", err)
			}
		}
		if n.kind == kindEdit {
			// The image to edit must be a generated one of the campaign, with
			// its request: the edit keeps its style and ratio.
			src, err := q.GetGalleryImageInCampaign(ctx, mapsdb.GetGalleryImageInCampaignParams{CampaignID: campaignID, ID: *n.source})
			if errors.Is(err, pgx.ErrNoRows) || err == nil && !src.Generated {
				return errImageNotFound()
			}
			if err != nil {
				return fmt.Errorf("find the image to edit: %w", err)
			}
			from, err := q.GetImageRequestForImage(ctx, mapsdb.GetImageRequestForImageParams{CampaignID: campaignID, ImageID: n.source})
			if errors.Is(err, pgx.ErrNoRows) {
				return errImageNotFound()
			}
			if err != nil {
				return fmt.Errorf("find the request of the image to edit: %w", err)
			}
			n.style, n.ratio = from.Style, from.AspectRatio
			if src.GeneratedKind == kindTexturedMap && from.MapWidth != nil && from.MapHeight != nil {
				// An edit of a textured map comes back at the map's own proportions (the original's ratio is the padded
				// canvas's, not the map's): the model ratio nearest the map image's, so little is cut and nothing is stretched.
				n.ratio = refimg.Closest(int(*from.MapWidth), int(*from.MapHeight))
			}
			if n.name == "" {
				n.name = suffixedName(src.Name, " (ajuste)")
			}
			if src.GeneratedKind == kindTexturedMap {
				// An edit of a textured map is a textured map: it keeps the map it fits (the image, the grid, the
				// size and the walls it started from), so it is made at the map's size and "Usar como imagem do
				// mapa" checks the same things (RN-10).
				inherited = basisParams{
					mapID: from.MapID, imageID: from.MapImageID, gridColumns: from.MapGridColumns, gridFactor: from.MapGridFactor,
					width: from.MapWidth, height: from.MapHeight, planHash: from.MapPlanHash,
					pad: [4]*float64{from.PadX0, from.PadY0, from.PadX1, from.PadY1},
				}
				hasInherited = true
			}
		}
		// The server holds only so many requests alive. Two requests that pass this at
		// once may both be taken: the cap is a bound on memory, not an exact count.
		if s.pending.Load() >= maxPendingRequests {
			return errTooManyImageRequests()
		}
		number, err := q.NextImageRequestNumber(ctx, campaignID)
		if err != nil {
			return fmt.Errorf("number the request: %w", err)
		}
		month, _ := monthOf(s.now())
		id := uuid.New().String()
		basis := n.prepared.basis.params()
		if hasInherited {
			basis = inherited
		}
		row, err = q.InsertImageRequest(ctx, mapsdb.InsertImageRequestParams{
			ID: id, CampaignID: campaignID, RequestedBy: &n.requestedBy, IdempotencyKey: n.key,
			Kind: n.kind, Prompt: n.prompt, Style: n.style, AspectRatio: n.ratio, Model: s.generator.Model(),
			ReferenceIds: nonNil(n.references), CharacterIds: nonNil(n.characters), SourceImageID: n.source, Number: number, QuotaMonth: month, CreatedAt: s.now(),
			MapID: basis.mapID, MapImageID: basis.imageID, MapGridColumns: basis.gridColumns, MapGridFactor: basis.gridFactor, MapWidth: basis.width, MapHeight: basis.height,
			MapPlanHash: basis.planHash, PadX0: basis.pad[0], PadY0: basis.pad[1], PadX1: basis.pad[2], PadY1: basis.pad[3],
			ImageName: imageNameFor(n), IdempotencyHash: n.hash,
		})
		if err != nil {
			return fmt.Errorf("insert the request: %w", err)
		}
		created = true
		status.UsedThisMonth++
		status.Remaining = max(status.GetRemaining()-1, 0)
		return nil
	})
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == "23505" {
		// Two tries with the same key raced: the other one made the request.
		if existing, getErr := s.queries.GetImageRequestByKey(ctx, mapsdb.GetImageRequestByKeyParams{CampaignID: campaignID, IdempotencyKey: n.key}); getErr == nil {
			if err := sameImageRequest(existing, n); err != nil {
				return mapsdb.ImageRequest{}, nil, err
			}
			status, statusErr := s.freshStatus(ctx, campaignID)
			if statusErr != nil {
				return mapsdb.ImageRequest{}, nil, s.dbError(ctx, "read the image generation status", statusErr)
			}
			return existing, status, nil
		}
	}
	if err != nil {
		return mapsdb.ImageRequest{}, nil, s.dbError(ctx, "reserve an image", err)
	}
	if created {
		logging.Event(ctx, s.logger, "image.requested", slog.String("generation_id", row.ID), slog.String("kind", n.kind))
		s.generations.Add(1)
		s.pending.Add(1) // run gives it back
		go s.run(campaignID, row.ID, n.prepared)
	}
	return row, status, nil
}

// GetImageGeneration implements mapsv1connect.ImageGenerationServiceHandler.
func (s *Service) GetImageGeneration(
	ctx context.Context,
	req *connect.Request[mapsv1.GetImageGenerationRequest],
) (*connect.Response[mapsv1.GetImageGenerationResponse], error) {
	m, err := requireGenerationMaster(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(req.Msg.GetGenerationId())
	if err != nil {
		return nil, errGenerationNotFound()
	}
	wait := time.Duration(min(max(req.Msg.GetWaitSeconds(), 0), maxLongPoll)) * time.Second
	// Register before the first read, so a change in between is not missed.
	var changed <-chan struct{}
	if wait > 0 {
		var unregister func()
		changed, unregister = s.waiters.register(id.String())
		defer unregister()
	}
	res, err := s.readGeneration(ctx, m.CampaignID, id.String())
	if err != nil {
		return nil, err
	}
	if wait > 0 && res.GetGeneration().GetState() == mapsv1.ImageGenerationState_IMAGE_GENERATION_STATE_PENDING {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-changed:
		case <-timer.C:
		case <-ctx.Done():
		case <-s.baseCtx.Done(): // the server is shutting down: answer now
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if res, err = s.readGeneration(ctx, m.CampaignID, id.String()); err != nil {
			return nil, err
		}
	}
	return connect.NewResponse(res), nil
}

// readGeneration reads a request (after expiring the lost ones) with its image.
func (s *Service) readGeneration(ctx context.Context, campaignID, id string) (*mapsv1.GetImageGenerationResponse, error) {
	var (
		row    mapsdb.ImageRequest
		img    *mapsdb.GalleryImage
		status *mapsv1.ImageGenerationStatus
	)
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		img = nil
		q := s.queries.WithTx(tx)
		if err := s.expireStale(ctx, q, campaignID); err != nil {
			return err
		}
		var err error
		row, err = q.GetImageRequest(ctx, mapsdb.GetImageRequestParams{CampaignID: campaignID, ID: id})
		if errors.Is(err, pgx.ErrNoRows) {
			return errGenerationNotFound()
		}
		if err != nil {
			return fmt.Errorf("read the request: %w", err)
		}
		if row.ImageID != nil {
			g, err := q.GetGalleryImageInCampaign(ctx, mapsdb.GetGalleryImageInCampaignParams{CampaignID: campaignID, ID: *row.ImageID})
			if err != nil {
				return fmt.Errorf("read the generated image: %w", err)
			}
			img = &g
		}
		status, err = s.statusIn(ctx, q, campaignID)
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, "read an image request", err)
	}
	res := &mapsv1.GetImageGenerationResponse{Generation: s.generationToProto(row), Status: status}
	if img != nil {
		res.Image = imageToProto(*img)
	}
	return res, nil
}

// CancelImageGeneration implements mapsv1connect.ImageGenerationServiceHandler.
func (s *Service) CancelImageGeneration(
	ctx context.Context,
	req *connect.Request[mapsv1.CancelImageGenerationRequest],
) (*connect.Response[mapsv1.CancelImageGenerationResponse], error) {
	m, err := requireGenerationMaster(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(req.Msg.GetGenerationId())
	if err != nil {
		return nil, errGenerationNotFound()
	}
	var (
		row    mapsdb.ImageRequest
		status *mapsv1.ImageGenerationStatus
	)
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		now := s.now()
		// Before it left: canceled, and the slot goes back. After: only the wait stops.
		n, err := q.CancelUnsentImageRequest(ctx, mapsdb.CancelUnsentImageRequestParams{CampaignID: m.CampaignID, ID: id.String(), FinishedAt: &now})
		if err != nil {
			return fmt.Errorf("cancel an unsent request: %w", err)
		}
		if n == 0 {
			if _, err := q.CancelSentImageRequest(ctx, mapsdb.CancelSentImageRequestParams{CampaignID: m.CampaignID, ID: id.String(), FinishedAt: &now}); err != nil {
				return fmt.Errorf("cancel a sent request: %w", err)
			}
		}
		row, err = q.GetImageRequest(ctx, mapsdb.GetImageRequestParams{CampaignID: m.CampaignID, ID: id.String()})
		if errors.Is(err, pgx.ErrNoRows) {
			return errGenerationNotFound()
		}
		if err != nil {
			return fmt.Errorf("read the request: %w", err)
		}
		status, err = s.statusIn(ctx, q, m.CampaignID)
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, "cancel an image request", err)
	}
	s.waiters.notify(row.ID)
	return connect.NewResponse(&mapsv1.CancelImageGenerationResponse{Generation: s.generationToProto(row), Status: status}), nil
}

// ListImageEdits implements mapsv1connect.ImageGenerationServiceHandler.
func (s *Service) ListImageEdits(
	ctx context.Context,
	req *connect.Request[mapsv1.ListImageEditsRequest],
) (*connect.Response[mapsv1.ListImageEditsResponse], error) {
	m, err := requireGenerationMaster(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(req.Msg.GetImageId())
	if err != nil {
		return nil, errImageNotFound()
	}
	rows, err := s.queries.ListImageChain(ctx, mapsdb.ListImageChainParams{CampaignID: m.CampaignID, ID: id.String()})
	if err != nil {
		return nil, s.dbError(ctx, "list the edit chain", err)
	}
	if len(rows) == 0 {
		return nil, errImageNotFound()
	}
	res := &mapsv1.ListImageEditsResponse{}
	for _, r := range rows {
		res.Edits = append(res.Edits, &mapsv1.ImageEdit{
			Image: imageToProto(mapsdb.GalleryImage{
				ID: r.ID, CampaignID: r.CampaignID, UploadedBy: r.UploadedBy, Name: r.Name, ContentType: r.ContentType,
				Width: r.Width, Height: r.Height, ByteSize: r.ByteSize, CreatedAt: r.CreatedAt,
				Generated: r.Generated, ParentImageID: r.ParentImageID, GeneratedKind: r.GeneratedKind,
			}),
			Prompt: r.Prompt, Number: r.Number,
		})
	}
	return connect.NewResponse(res), nil
}

func errGenerationNotFound() error {
	return connect.NewError(connect.CodeNotFound, errors.New("image request not found"))
}

// generationToProto turns a row into the API's message.
func (s *Service) generationToProto(r mapsdb.ImageRequest) *mapsv1.ImageGeneration {
	g := &mapsv1.ImageGeneration{
		Id: r.ID, CampaignId: r.CampaignID, Prompt: r.Prompt, Number: r.Number,
		Style: styleFromKey(r.Style), AspectRatio: ratioFromName(r.AspectRatio),
		ReferenceImageIds: r.ReferenceIds, CharacterImageIds: r.CharacterIds, SlotSpent: !r.Refunded,
		CreatedAt: timestamppb.New(r.CreatedAt),
	}
	switch r.Kind {
	case kindScene:
		g.Kind = mapsv1.ImageGenerationKind_IMAGE_GENERATION_KIND_SCENE
	case kindEdit:
		g.Kind = mapsv1.ImageGenerationKind_IMAGE_GENERATION_KIND_EDIT
	case kindMapScene:
		g.Kind = mapsv1.ImageGenerationKind_IMAGE_GENERATION_KIND_MAP_SCENE
	case kindIsometric:
		g.Kind = mapsv1.ImageGenerationKind_IMAGE_GENERATION_KIND_ISOMETRIC
	case kindTexturedMap:
		g.Kind = mapsv1.ImageGenerationKind_IMAGE_GENERATION_KIND_TEXTURED_MAP
	}
	if r.MapID != nil {
		g.MapId = *r.MapID
	}
	switch r.Status {
	case statePending:
		g.State = mapsv1.ImageGenerationState_IMAGE_GENERATION_STATE_PENDING
	case stateDone:
		g.State = mapsv1.ImageGenerationState_IMAGE_GENERATION_STATE_DONE
	case stateRefused:
		g.State = mapsv1.ImageGenerationState_IMAGE_GENERATION_STATE_REFUSED
	case stateFailed:
		g.State = mapsv1.ImageGenerationState_IMAGE_GENERATION_STATE_FAILED
	case stateCanceled:
		g.State = mapsv1.ImageGenerationState_IMAGE_GENERATION_STATE_CANCELED
	}
	g.Failure, g.ReasonPt = failureOf(r.Reason)
	if r.ImageID != nil {
		g.ImageId = *r.ImageID
	}
	if r.SourceImageID != nil {
		g.SourceImageId = *r.SourceImageID
	}
	if r.FinishedAt != nil {
		g.FinishedAt = timestamppb.New(*r.FinishedAt)
	}
	return g
}

// failureOf maps a stored reason to the enum and the Portuguese sentence. The
// sentences are ours: the model's own message never reaches the master.
func failureOf(reason string) (mapsv1.ImageGenerationFailure, string) {
	switch reason {
	case reasonNoImage:
		return mapsv1.ImageGenerationFailure_IMAGE_GENERATION_FAILURE_NO_IMAGE, "O serviço não gerou uma imagem. Tente descrever a cena de outro jeito."
	case reasonRefused:
		return mapsv1.ImageGenerationFailure_IMAGE_GENERATION_FAILURE_REFUSED, "O serviço recusou este pedido. Tente descrever a cena de outro jeito."
	case reasonUnavailable:
		return mapsv1.ImageGenerationFailure_IMAGE_GENERATION_FAILURE_UNAVAILABLE, "O serviço de imagens não respondeu. Tente de novo em instantes."
	case reasonGalleryFull:
		return mapsv1.ImageGenerationFailure_IMAGE_GENERATION_FAILURE_GALLERY_FULL, "A galeria está cheia: apague uma imagem e tente de novo."
	case reasonImageMissing:
		return mapsv1.ImageGenerationFailure_IMAGE_GENERATION_FAILURE_IMAGE_MISSING, "Uma das imagens do pedido foi apagada. Escolha de novo."
	case reasonServiceOff:
		return mapsv1.ImageGenerationFailure_IMAGE_GENERATION_FAILURE_SERVICE_OFF, "O serviço de imagens não está disponível."
	case reasonShutdown:
		return mapsv1.ImageGenerationFailure_IMAGE_GENERATION_FAILURE_UNAVAILABLE, "O servidor reiniciou antes de enviar o pedido. Tente de novo."
	case reasonTimeout:
		return mapsv1.ImageGenerationFailure_IMAGE_GENERATION_FAILURE_TIMEOUT, "O pedido se perdeu. Tente de novo."
	}
	return mapsv1.ImageGenerationFailure_IMAGE_GENERATION_FAILURE_UNSPECIFIED, ""
}

// ---- the checks of what the master sent ----

// cleanKey checks an idempotency key: 1 to 64 printable characters.
func cleanKey(key string) (string, error) {
	key = strings.TrimSpace(key)
	if n := utf8.RuneCountInString(key); n < 1 || n > 64 || !utf8.ValidString(key) || strings.IndexFunc(key, unicode.IsControl) >= 0 {
		return "", connect.NewError(connect.CodeInvalidArgument, errors.New("idempotency_key must be 1 to 64 characters"))
	}
	return key, nil
}

// cleanPrompt checks the master's text: 1 to 500 characters, valid UTF-8, no
// control characters but line breaks and tabs. It stays as written; the error
// says the rule, never the text.
func cleanPrompt(text, field string) (string, error) {
	text = strings.TrimSpace(text)
	bad := !utf8.ValidString(text) || strings.IndexFunc(text, func(r rune) bool {
		return unicode.IsControl(r) && r != '\n' && r != '\t'
	}) >= 0
	if n := utf8.RuneCountInString(text); n < 1 || n > maxPromptCharacters || bad {
		return "", connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("%s must be 1 to %d characters of text", field, maxPromptCharacters))
	}
	return text, nil
}

// imageNameOf is the name the generated image gets: the master's (checked like a rename), or
// the default when there is none ("" when the caller has none and fills it in later).
func imageNameOf(given, fallback string) (string, error) {
	if strings.TrimSpace(given) == "" {
		return fallback, nil
	}
	name, err := names.Clean(given, maxNameLength)
	if err != nil {
		// The message says the rule, never the name, which is free text.
		return "", connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("name %w", err))
	}
	return name, nil
}

// imageNameFor is the name the request records for its gallery image ("" only for a request
// without one, which the picture then names "Imagem n").
func imageNameFor(n newRequest) string { return n.name }

// datedName is a default name that says what the picture is and the day it was made, and nothing
// the master wrote: the text of a request or the name of a hidden map can hold a secret, and the
// players read the name of an image they are shown (RN-10). "Arte da cena · 06/10".
func datedName(way string, now time.Time) string {
	return way + " · " + now.In(brazil).Format("02/01")
}

// suffixedName is a name with a suffix, cut so that it still fits the name limit.
func suffixedName(name, suffix string) string {
	room := maxNameLength - utf8.RuneCountInString(suffix)
	if utf8.RuneCountInString(name) > room {
		name = strings.TrimSpace(string([]rune(name)[:room]))
	}
	return name + suffix
}

func styleAndRatio(style mapsv1.ImageStyle, ratio mapsv1.ImageAspectRatio) (styleKey, ratioName string, err error) {
	if _, ok := stylePhrases[style]; !ok && style != mapsv1.ImageStyle_IMAGE_STYLE_UNSPECIFIED {
		return "", "", connect.NewError(connect.CodeInvalidArgument, errors.New("style is not one of the styles"))
	}
	ratioName = defaultSceneRatio
	if ratio != mapsv1.ImageAspectRatio_IMAGE_ASPECT_RATIO_UNSPECIFIED {
		var ok bool
		if ratioName, ok = ratioNames[ratio]; !ok {
			return "", "", connect.NewError(connect.CodeInvalidArgument, errors.New("aspect_ratio is not one of the ratios"))
		}
	}
	return styleKeys[style], ratioName, nil
}

func cleanImageIDs(ids []string, most int, field string) ([]string, error) {
	if len(ids) > most {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("%s has at most %d images", field, most))
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		u, err := uuid.Parse(id)
		if err != nil {
			return nil, errImageNotFound()
		}
		out = append(out, u.String())
	}
	return out, nil
}

func hasDuplicates(ids []string) bool {
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			return true
		}
		seen[id] = true
	}
	return false
}

// ---- steps 2 and 3: the call and the storing ----

// CancelGenerations is the shutdown: the generation goroutines stop, a request
// still waiting for a call slot is refunded at once, a long poll answers, and a
// call already sent is cut off (it follows the expiry of a sent request: its
// slot stays spent). Call WaitForGenerations after it.
func (s *Service) CancelGenerations() { s.cancelBase() }

// WaitForGenerations waits for the requests in flight, up to ctx: the shutdown
// (and a test's cleanup) uses it, so no goroutine writes after the pool closes.
func (s *Service) WaitForGenerations(ctx context.Context) {
	done := make(chan struct{})
	go func() { s.generations.Wait(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

// generationWaiters wakes the long polls of one instance: a request's goroutine
// (or a cancel) calls notify, and every GetImageGeneration waiting on it reads
// the row again. One instance only: Cloud Run runs max-instances 1
// (docs/operations.md); with more, the poll still ends at its deadline.
type generationWaiters struct {
	mu sync.Mutex
	m  map[string]map[chan struct{}]struct{}
}

// register returns a channel closed by the next notify of id, and a function
// that forgets it.
func (w *generationWaiters) register(id string) (<-chan struct{}, func()) {
	ch := make(chan struct{})
	w.mu.Lock()
	if w.m == nil {
		w.m = map[string]map[chan struct{}]struct{}{}
	}
	if w.m[id] == nil {
		w.m[id] = map[chan struct{}]struct{}{}
	}
	w.m[id][ch] = struct{}{}
	w.mu.Unlock()
	return ch, func() {
		w.mu.Lock()
		delete(w.m[id], ch)
		if len(w.m[id]) == 0 {
			delete(w.m, id)
		}
		w.mu.Unlock()
	}
}

func (w *generationWaiters) notify(id string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for ch := range w.m[id] {
		close(ch)
		delete(w.m[id], ch)
	}
}

// errRequestClosed: the request can no longer take an image (it ended another
// way, or already has one). The gallery insert that asked is rolled back.
var errRequestClosed = errors.New("maps: the image request is closed")

// run is a request's goroutine: it waits for a call slot, marks the request
// sent, calls the model outside any transaction, stores the picture together
// with the request's end, and records how it ended. It never uses the caller's
// context (the master may close the page): it derives from the service's.
func (s *Service) run(campaignID, id string, prep prepared) {
	defer s.generations.Done()
	defer s.pending.Add(-1)
	defer s.waiters.notify(id)
	ctx, cancel := context.WithTimeout(s.baseCtx, generationTimeout)
	defer cancel()
	// This goroutine decodes and draws what a model sent back, and a panic in a
	// goroutine the server started itself would take the whole process down (the
	// net/http recover only covers a handler's own goroutine). So a panic fails
	// this one request: logged with its stack, recorded as failed. Declared last,
	// so it runs first, before the waiters are woken and the slot is given back.
	defer safego.Recover(s.logger, "image request", func() { s.finishFailed(campaignID, id, stateFailed, reasonUnavailable) })
	select {
	case s.generating <- struct{}{}:
		defer func() { <-s.generating }()
		if s.baseCtx.Err() != nil { // a slot freed by the shutdown itself
			s.finishFailed(campaignID, id, stateFailed, reasonShutdown)
			return
		}
	case <-ctx.Done():
		// Never sent: the slot goes back at once.
		s.finishFailed(campaignID, id, stateFailed, endReason(s.baseCtx))
		return
	}

	row, err := s.queries.GetImageRequest(ctx, mapsdb.GetImageRequestParams{CampaignID: campaignID, ID: id})
	if err != nil {
		s.logger.ErrorContext(ctx, "maps: cannot read an image request to run it", "error", err)
		s.finishFailed(campaignID, id, stateFailed, reasonUnavailable)
		return
	}
	if row.Status != statePending {
		return // canceled before it started
	}
	request, reason, err := s.modelRequest(ctx, row, prep)
	if err != nil {
		s.logger.ErrorContext(ctx, "maps: cannot build an image request", "error", err, "reason", reason)
		s.finishFailed(campaignID, id, stateFailed, reason)
		return
	}
	if s.baseCtx.Err() != nil {
		s.finishFailed(campaignID, id, stateFailed, reasonShutdown)
		return
	}
	// The request leaves now. A Cancel that came first wins: nothing is sent.
	sent, err := s.queries.MarkImageRequestSent(ctx, mapsdb.MarkImageRequestSentParams{CampaignID: campaignID, ID: id, SentAt: new(s.now())})
	if err != nil {
		s.logger.ErrorContext(ctx, "maps: cannot mark an image request as sent", "error", err)
		s.finishFailed(campaignID, id, stateFailed, reasonUnavailable)
		return
	}
	if sent == 0 {
		return
	}

	picture, err := s.generator.Generate(ctx, request)
	if err != nil {
		if s.baseCtx.Err() != nil {
			// Cut off by the shutdown: the call may have been billed, so the slot
			// is not given back. The request expires as a sent one (expireStale).
			return
		}
		state, why := stateFailed, reasonUnavailable
		certainlyFree := false // the slot goes back only when the call surely was not billed
		switch refused := (*gen.RefusedError)(nil); {
		case errors.As(err, &refused):
			state, why, certainlyFree = stateRefused, reasonRefused, true
		case errors.Is(err, gen.ErrNoImage):
			why, certainlyFree = reasonNoImage, true
		case errors.Is(err, gen.ErrNotAuthorized):
			// The operator's problem: the key was refused. Say it loudly, once.
			why, certainlyFree = reasonServiceOff, true
			s.logger.ErrorContext(ctx, "maps: the image service refused the API key", "error", err)
		}
		// The error says a status or a kind, never the text (package gen).
		s.logger.WarnContext(ctx, "maps: an image request ended without a picture", "reason", why, "error", err)
		if certainlyFree {
			s.finishFailed(campaignID, id, state, why)
		} else {
			// A timeout, a cut connection or an answer that cannot be read may have been billed.
			s.finishSpent(campaignID, id, why)
		}
		return
	}
	// The picture is here: finish it on a context of its own, even in a shutdown.
	fin, cancelFin := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancelFin()
	var res *images.Result
	kind := s.generatedKindOf(fin, row)
	switch {
	case row.Kind == kindTexturedMap:
		// The textured map is the map's rectangle of the padded canvas, in the size of the map's image.
		res, err = s.cropToMap(fin, picture.Data, row)
	case row.Kind == kindEdit && kind == kindTexturedMap && row.MapWidth != nil && row.MapHeight != nil:
		// An edit of a textured map keeps the map's size, so it can be used as the map's image too.
		res, err = s.fitToMap(fin, picture.Data, row)
	default:
		res, err = s.process(fin, picture.Data)
	}
	if err != nil {
		s.logger.WarnContext(fin, "maps: the generated picture cannot be used", "error", err)
		s.finishSpent(campaignID, id, reasonNoImage)
		return
	}
	var parent *string
	if row.Kind == kindEdit {
		parent = row.SourceImageID
	}
	name := row.ImageName
	if name == "" {
		name = "Imagem " + strconv.Itoa(int(row.Number))
	}
	// The gallery row and the request's end go in one transaction, allowed once.
	stored, err := s.storeImage(fin, campaignID, row.RequestedBy, name, res, true, kind, parent,
		func(ctx context.Context, q *mapsdb.Queries, imageID string) error {
			n, err := q.FinishImageRequestDone(ctx, mapsdb.FinishImageRequestDoneParams{CampaignID: campaignID, ID: id, ImageID: &imageID, Now: new(s.now())})
			if err != nil {
				return fmt.Errorf("finish the request: %w", err)
			}
			if n == 0 {
				return errRequestClosed
			}
			return nil
		})
	if err != nil {
		why := reasonUnavailable
		if he, ok := errors.AsType[*httpError](err); ok && he.reason == ReasonQuota {
			why = reasonGalleryFull
		}
		if errors.Is(err, errRequestClosed) {
			s.logger.WarnContext(fin, "maps: a generated picture arrived for a request that is closed; it was dropped")
			return
		}
		s.logger.ErrorContext(fin, "maps: cannot store a generated image", "error", err, "reason", why)
		s.finishSpent(campaignID, id, why)
		return
	}
	s.logger.InfoContext(fin, "maps: an image was generated", "campaign", campaignID, "request", id, "image", stored.ID)
	// The background context has no request fields, so the campaign is explicit.
	logging.Event(fin, s.logger, "image.generated", slog.String("campaign_id", campaignID), slog.String("generation_id", id), slog.String("image_id", stored.ID), slog.String("kind", kind))
}

// generatedKindOf is the way a request's picture was made, as the gallery records it: the
// request's own kind, and for an edit the way of the image it adjusts (a chain keeps the way
// of its root). An edit that carries a map basis is a textured map's even if its source is gone.
func (s *Service) generatedKindOf(ctx context.Context, row mapsdb.ImageRequest) string {
	if row.Kind != kindEdit {
		return row.Kind
	}
	if row.SourceImageID != nil {
		if src, err := s.queries.GetGalleryImageInCampaign(ctx, mapsdb.GetGalleryImageInCampaignParams{CampaignID: row.CampaignID, ID: *row.SourceImageID}); err == nil {
			return src.GeneratedKind
		}
	}
	if row.MapID != nil {
		return kindTexturedMap
	}
	return ""
}

// endReason says why a request that never left stops: the shutdown, or its
// own timeout.
func endReason(base context.Context) string {
	if base.Err() != nil {
		return reasonShutdown
	}
	return reasonTimeout
}

// finishFailed ends a request that made no picture: the slot goes back.
func (s *Service) finishFailed(campaignID, id, state, reason string) {
	s.finish(campaignID, id, state, reason, true)
}

// finishSpent ends a request whose picture the model returned but the server could
// not store: the call was made, so the slot stays spent.
func (s *Service) finishSpent(campaignID, id, reason string) {
	s.finish(campaignID, id, stateFailed, reason, false)
}

func (s *Service) finish(campaignID, id, state, reason string, refund bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var err error
	if refund {
		err = s.queries.FinishImageRequestFailed(ctx, mapsdb.FinishImageRequestFailedParams{
			Now: new(s.now()), Status: state, Reason: reason, CampaignID: campaignID, ID: id,
		})
	} else {
		err = s.queries.FinishImageRequestSpent(ctx, mapsdb.FinishImageRequestSpentParams{
			Now: new(s.now()), Status: state, Reason: reason, CampaignID: campaignID, ID: id,
		})
	}
	if err != nil {
		s.logger.ErrorContext(ctx, "maps: cannot record a failed image request", "error", err)
		return
	}
	// Never the prompt: the ids, and the closed reason.
	name := "image.failed"
	if state == stateRefused {
		name = "image.refused"
	}
	logging.Event(ctx, s.logger, name, slog.String("campaign_id", campaignID), slog.String("generation_id", id), slog.String("reason", reason))
}

// modelRequest builds what goes to the model from the stored request and the
// images prepared before the slot was reserved (shrunk, from the campaign's own
// gallery, ADR-0012). The text is the master's; the only thing added is the
// style's name. Nothing else goes: not a name, an e-mail or a sheet.
func (s *Service) modelRequest(ctx context.Context, row mapsdb.ImageRequest, prep prepared) (gen.Request, string, error) {
	req := gen.Request{AspectRatio: row.AspectRatio, Style: stylePhrases[styleFromKey(row.Style)], References: prep.references}
	if row.Kind == kindEdit {
		edit, err := s.editOf(ctx, row, prep)
		if err != nil {
			return gen.Request{}, reasonImageMissing, err
		}
		req.Edit = edit
	} else {
		req.Prompt = row.Prompt
	}
	if isMapKind(row.Kind) {
		if prep.drawing == nil {
			return gen.Request{}, reasonImageMissing, errors.New("a request made from a map has no drawing")
		}
		req.Drawing, req.Layout, req.Rooms = prep.drawing, layoutOf(row.Kind), prep.rooms
	}
	return req, "", nil
}

// editOf collects an edit's inputs: the previous image, the text that made the
// chain's first image, the adjustments since, and the new instruction.
func (s *Service) editOf(ctx context.Context, row mapsdb.ImageRequest, prep prepared) (*gen.Edit, error) {
	if row.SourceImageID == nil || prep.previous == nil {
		return nil, errors.New("an edit has no source image")
	}
	chain, err := s.queries.ListImageChain(ctx, mapsdb.ListImageChainParams{CampaignID: row.CampaignID, ID: *row.SourceImageID})
	if err != nil {
		return nil, fmt.Errorf("read the edit chain: %w", err)
	}
	byID := map[string]mapsdb.ListImageChainRow{}
	for _, c := range chain {
		byID[c.ID] = c
	}
	// The path from the source up to the chain's root, then turned around.
	var path []mapsdb.ListImageChainRow
	for cur, ok := byID[*row.SourceImageID]; ok; {
		path = append(path, cur)
		if cur.ParentImageID == nil {
			break
		}
		cur, ok = byID[*cur.ParentImageID]
	}
	edit := &gen.Edit{Previous: *prep.previous, Instruction: row.Prompt}
	for i, p := range slices.Backward(path) {
		if i == len(path)-1 {
			edit.Original = p.Prompt
		} else {
			edit.Adjustments = append(edit.Adjustments, p.Prompt)
		}
	}
	return edit, nil
}

// nonNil is ids, or an empty list: the columns are NOT NULL, and a nil slice is NULL.
func nonNil(ids []string) []string {
	if ids == nil {
		return []string{}
	}
	return ids
}
