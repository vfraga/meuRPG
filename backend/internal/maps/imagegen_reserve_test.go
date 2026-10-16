package maps

import (
	"errors"
	"testing"

	"connectrpc.com/connect"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	"github.com/PuraFome/meuRPG/backend/internal/maps/images/gen"
	"github.com/PuraFome/meuRPG/backend/internal/maps/mapsdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
)

// These tests call reserve (step 1 of an image request) directly, as begin does
// after it has prepared the images: every refusal reserve itself makes inside its
// transaction is reached by a state that changed after begin's own checks.

func sceneRequest(master *user, key string) newRequest {
	return newRequest{kind: kindScene, key: key, prompt: "uma cena", style: "", ratio: defaultSceneRatio, requestedBy: master.id, name: "Arte"}
}

// The month, the day and the service being on are checked again inside reserve's
// transaction, each with its typed reason and the status the client shows.
func TestMR039_ReserveChecksTheCapsAgain(t *testing.T) {
	t.Parallel()
	h := newHarness(t, func(c *Config) {
		withFake(&gen.Fake{}, 2)(c)
		c.DailyImages = 1
	})
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master)
	ctx := t.Context()

	row, status, err := h.svc.reserve(ctx, sceneRequest(master, "reserve-1"), campaign)
	if err != nil || row.Number != 1 || row.Status != statePending || row.ImageName != "Arte" || row.Kind != kindScene {
		t.Fatalf("reserve() = %+v, %v", row, err)
	}
	if status.GetUsedThisMonth() != 1 || status.GetRemaining() != 1 {
		t.Errorf("status after the reservation = %v, want used 1, remaining 1", status)
	}
	master.waitGeneration(campaign, row.ID)

	_, _, err = h.svc.reserve(ctx, sceneRequest(master, "reserve-2"), campaign)
	b := wantGenerationBlocked(t, "reserve over the daily cap", err, mapsv1.ImageGenerationBlockedReason_IMAGE_GENERATION_BLOCKED_REASON_DAILY_LIMIT_REACHED)
	if b.GetStatus().GetUsedThisMonth() != 1 || b.GetStatus().GetRemaining() != 1 {
		t.Errorf("status in the daily refusal = %v", b.GetStatus())
	}
	wantRefusal(t, "reserve over the daily cap", err, connect.CodeFailedPrecondition, "the server made all its images of the day, try again tomorrow")
}

func TestMR039_ReserveChecksTheMonthAgain(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withFake(&gen.Fake{}, 1))
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master)
	ctx := t.Context()

	row, _, err := h.svc.reserve(ctx, sceneRequest(master, "reserve-1"), campaign)
	if err != nil {
		t.Fatalf("reserve() error = %v", err)
	}
	master.waitGeneration(campaign, row.ID)
	_, _, err = h.svc.reserve(ctx, sceneRequest(master, "reserve-2"), campaign)
	b := wantGenerationBlocked(t, "reserve over the month", err, mapsv1.ImageGenerationBlockedReason_IMAGE_GENERATION_BLOCKED_REASON_LIMIT_REACHED)
	if b.GetStatus().GetRemaining() != 0 || b.GetStatus().GetUsedThisMonth() != 1 {
		t.Errorf("status in the month's refusal = %v", b.GetStatus())
	}
	wantRefusal(t, "reserve over the month", err, connect.CodeFailedPrecondition, "the campaign used its images of the month")
}

func TestMR039_ReserveChecksTheServiceIsOn(t *testing.T) {
	t.Parallel()
	h := newHarness(t) // no generator
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master)

	_, _, err := h.svc.reserve(t.Context(), sceneRequest(master, "reserve-1"), campaign)
	b := wantGenerationBlocked(t, "reserve with the service off", err, mapsv1.ImageGenerationBlockedReason_IMAGE_GENERATION_BLOCKED_REASON_OFF)
	if b.GetStatus().GetEnabled() {
		t.Errorf("status in the off refusal = %v, want not enabled", b.GetStatus())
	}
	wantRefusal(t, "reserve with the service off", err, connect.CodeFailedPrecondition, "image generation is not on in this server")
}

// An image that is not in the campaign's gallery, or an edit's source that is not
// a generated image with its request, is not_found, and nothing is reserved.
func TestMR039_ReserveRefusesImagesThatAreNotThere(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withFake(&gen.Fake{}, 10))
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master)
	other := h.newCampaign(master)
	ctx := t.Context()
	const notFound = "image not found"
	const ghost = "00000000-0000-4000-8000-000000000001"

	uploaded := master.newImage(campaign)
	inOther := master.newImage(other)
	source := master.mustGenerate(campaign, "a origem").GetImage()
	generated := source.GetId()
	orphan := master.mustGenerate(campaign, "sem pedido").GetImage().GetId()
	if _, err := h.pool.Exec(ctx, "DELETE FROM image_requests WHERE image_id = $1", orphan); err != nil {
		t.Fatalf("delete the request of an image: %v", err)
	}

	cases := []struct {
		name string
		mod  func(*newRequest)
	}{
		{"an object reference that does not exist", func(n *newRequest) { n.references = []string{ghost} }},
		{"a character reference of another campaign", func(n *newRequest) { n.characters = []string{inOther} }},
		{"a reference after a good one", func(n *newRequest) { n.references = []string{uploaded, ghost} }},
		{"an edit of an image that does not exist", func(n *newRequest) { n.kind, n.source = kindEdit, new(ghost) }},
		{"an edit of an uploaded image", func(n *newRequest) { n.kind, n.source = kindEdit, new(uploaded) }},
		{"an edit of an image of another campaign", func(n *newRequest) { n.kind, n.source = kindEdit, new(inOther) }},
		{"an edit of a generated image whose request is gone", func(n *newRequest) { n.kind, n.source = kindEdit, new(orphan) }},
	}
	for _, tc := range cases {
		n := sceneRequest(master, nextKey())
		tc.mod(&n)
		_, _, err := h.svc.reserve(ctx, n, campaign)
		wantRefusal(t, tc.name, err, connect.CodeNotFound, notFound)
	}
	if got := master.imageStatus(campaign).GetUsedThisMonth(); got != 1 {
		t.Errorf("used this month = %d, want 1 (the source; the orphan's request was deleted)", got)
	}

	// A good reference and a good source go through: the edit keeps its parent's style and
	// ratio, ignoring the ones asked, and is named after the source.
	n := newRequest{kind: kindEdit, key: nextKey(), prompt: "mais escura", style: "other", ratio: "1:1", source: new(generated), requestedBy: master.id, references: []string{uploaded}}
	row, _, err := h.svc.reserve(ctx, n, campaign)
	if err != nil {
		t.Fatalf("reserve() error = %v", err)
	}
	parent, err := h.svc.queries.GetImageRequestForImage(ctx, mapsdb.GetImageRequestForImageParams{CampaignID: campaign, ImageID: &generated})
	if err != nil {
		t.Fatalf("read the parent request: %v", err)
	}
	if row.Kind != kindEdit || row.Style != parent.Style || row.AspectRatio != parent.AspectRatio || row.SourceImageID == nil || *row.SourceImageID != generated || row.MapID != nil {
		t.Errorf("edit row = %+v, want the parent's style %q and ratio %q", row, parent.Style, parent.AspectRatio)
	}
	if want := source.GetName() + " (ajuste)"; row.ImageName != want {
		t.Errorf("edit name = %q, want %q", row.ImageName, want)
	}
	master.waitGeneration(campaign, row.ID)
}

// The server holds only so many requests alive: past the cap, reserve refuses
// with resource_exhausted and reserves nothing.
func TestMR039_ReserveRefusesPastThePendingCap(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withFake(&gen.Fake{}, 5))
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master)

	h.svc.pending.Store(maxPendingRequests)
	_, _, err := h.svc.reserve(t.Context(), sceneRequest(master, "reserve-1"), campaign)
	h.svc.pending.Store(0)
	wantRefusal(t, "reserve past the pending cap", err, connect.CodeResourceExhausted, "too many images are being made right now; wait for one to finish")
	if got := master.imageStatus(campaign).GetUsedThisMonth(); got != 0 {
		t.Errorf("used this month = %d, want 0", got)
	}
}

// A try with a key an earlier request used answers that request, whatever it
// became, and reserves nothing; a key reused for another request is refused.
func TestMR039_ReserveReplaysAKey(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withFake(&gen.Fake{}, 5))
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master)
	ctx := t.Context()
	hash := func(s string) *string { return &s }

	n := sceneRequest(master, "reserve-1")
	n.hash = hash("a")
	first, _, err := h.svc.reserve(ctx, n, campaign)
	if err != nil {
		t.Fatalf("reserve() error = %v", err)
	}
	master.waitGeneration(campaign, first.ID)

	again, status, err := h.svc.reserve(ctx, n, campaign)
	if err != nil || again.ID != first.ID || status.GetUsedThisMonth() != 1 {
		t.Errorf("replay = %v, %v, %v; want the first request (%s) and one used", again.ID, status, err, first.ID)
	}
	n.hash = hash("b")
	_, _, err = h.svc.reserve(ctx, n, campaign)
	wantRefusal(t, "reserve with the key reused", err, connect.CodeInvalidArgument, errReusedMessage())
}

func errReusedMessage() string {
	ce, _ := errors.AsType[*connect.Error](idem.ErrReused())
	return ce.Message()
}
