package maps

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"net/http"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/maps/images/gen"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dbtest"
)

// MR-039, RN-28: pictures made by an image model. The tests use the fake
// generator (package gen), so they need no key and no network.

// withFake gives the harness's maps service a fake generator and a monthly cap.
func withFake(f *gen.Fake, monthly int32) func(*Config) {
	return func(c *Config) {
		c.Generator = f
		c.MonthlyImages = monthly
	}
}

var genKeyCounter struct {
	sync.Mutex
	n int
}

// nextKey makes an idempotency key that has not been used.
func nextKey() string {
	genKeyCounter.Lock()
	defer genKeyCounter.Unlock()
	genKeyCounter.n++
	return fmt.Sprintf("test-key-%d", genKeyCounter.n)
}

func (u *user) generate(campaign, prompt string) (*mapsv1.GenerateSceneImageResponse, error) {
	u.h.t.Helper()
	res, err := u.imagegen.GenerateSceneImage(u.h.t.Context(), connect.NewRequest(&mapsv1.GenerateSceneImageRequest{
		CampaignId: campaign, IdempotencyKey: nextKey(), Prompt: prompt,
	}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

// mustGenerate asks for a picture and waits for the end of the request.
func (u *user) mustGenerate(campaign, prompt string) *mapsv1.GetImageGenerationResponse {
	u.h.t.Helper()
	res, err := u.generate(campaign, prompt)
	if err != nil {
		u.h.t.Fatalf("GenerateSceneImage() error = %v", err)
	}
	return u.waitGeneration(campaign, res.GetGeneration().GetId())
}

// waitGeneration reads the request until it is not pending.
func (u *user) waitGeneration(campaign, id string) *mapsv1.GetImageGenerationResponse {
	u.h.t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		res, err := u.imagegen.GetImageGeneration(u.h.t.Context(), connect.NewRequest(&mapsv1.GetImageGenerationRequest{CampaignId: campaign, GenerationId: id}))
		if err != nil {
			u.h.t.Fatalf("GetImageGeneration() error = %v", err)
		}
		if res.Msg.GetGeneration().GetState() != mapsv1.ImageGenerationState_IMAGE_GENERATION_STATE_PENDING {
			return res.Msg
		}
		if time.Now().After(deadline) {
			u.h.t.Fatalf("the request is still pending after 30 s: %v", res.Msg)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (u *user) imageStatus(campaign string) *mapsv1.ImageGenerationStatus {
	u.h.t.Helper()
	res, err := u.imagegen.GetImageGenerationStatus(u.h.t.Context(), connect.NewRequest(&mapsv1.GetImageGenerationStatusRequest{CampaignId: campaign}))
	if err != nil {
		u.h.t.Fatalf("GetImageGenerationStatus() error = %v", err)
	}
	return res.Msg.GetStatus()
}

// wantGenerationBlocked checks a failed_precondition with the detail.
func wantGenerationBlocked(t *testing.T, call string, err error, reason mapsv1.ImageGenerationBlockedReason) *mapsv1.ImageGenerationBlocked {
	t.Helper()
	wantCode(t, call, err, connect.CodeFailedPrecondition)
	ce, _ := errors.AsType[*connect.Error](err)
	for _, d := range ce.Details() {
		if msg, derr := d.Value(); derr == nil {
			if b, ok := msg.(*mapsv1.ImageGenerationBlocked); ok && b.GetReason() == reason {
				return b
			}
		}
	}
	t.Fatalf("%s error = %v, want an ImageGenerationBlocked detail with %v", call, err, reason)
	return nil
}

func TestMonthOf(t *testing.T) {
	t.Parallel()
	// Brazil is UTC-3: 02:00 UTC on 1 November is still 31 October there.
	key, resets := monthOf(time.Date(2026, 11, 1, 2, 0, 0, 0, time.UTC))
	if key != "2026-10" || !resets.Equal(time.Date(2026, 11, 1, 3, 0, 0, 0, time.UTC)) {
		t.Errorf("monthOf(late October in Brazil) = %s, %v", key, resets)
	}
	key, resets = monthOf(time.Date(2026, 12, 31, 12, 0, 0, 0, time.UTC))
	if key != "2026-12" || !resets.Equal(time.Date(2027, 1, 1, 3, 0, 0, 0, time.UTC)) {
		t.Errorf("monthOf(December) = %s, %v", key, resets)
	}
}

// The master generates the scene art and edits it: each result is a gallery
// image, hidden from the players, and the edit points at its parent.
func TestMR039_GenerateAndEditAScene(t *testing.T) {
	t.Parallel()
	fake := &gen.Fake{}
	h := newHarness(t, withFake(fake, 10))
	master, player := h.newUser("Mestre"), h.newUser("Jogadora")
	campaign := h.newCampaign(master, player)

	first := master.mustGenerate(campaign, "Uma cripta úmida, tochas apagadas")
	if first.GetGeneration().GetState() != mapsv1.ImageGenerationState_IMAGE_GENERATION_STATE_DONE || first.GetImage() == nil {
		t.Fatalf("first request = %v; want DONE with an image", first)
	}
	img := first.GetImage()
	if !img.GetGenerated() || img.GetParentImageId() != "" || !strings.HasPrefix(img.GetName(), "Arte da cena · ") || img.GetContentType() != "image/png" {
		t.Errorf("first image = %v", img)
	}
	if first.GetGeneration().GetImageId() != img.GetId() || first.GetGeneration().GetNumber() != 1 || !first.GetGeneration().GetSlotSpent() {
		t.Errorf("first request = %v", first.GetGeneration())
	}
	// 16:9 by default; stored by the upload pipeline (decoded and encoded again).
	file := master.get(img.GetUrl())
	cfg, err := png.DecodeConfig(bytes.NewReader(file.body))
	if file.status != http.StatusOK || err != nil || cfg.Width != 256 || cfg.Height != 144 {
		t.Fatalf("the generated file: status %d, %dx%d, %v", file.status, cfg.Width, cfg.Height, err)
	}

	edit, err := master.imagegen.EditGeneratedImage(t.Context(), connect.NewRequest(&mapsv1.EditGeneratedImageRequest{
		CampaignId: campaign, ImageId: img.GetId(), IdempotencyKey: nextKey(), Instruction: "mais escura, com uma ponte sobre o poço",
	}))
	if err != nil {
		t.Fatalf("EditGeneratedImage() error = %v", err)
	}
	if edit.Msg.GetGeneration().GetKind() != mapsv1.ImageGenerationKind_IMAGE_GENERATION_KIND_EDIT || edit.Msg.GetGeneration().GetSourceImageId() != img.GetId() {
		t.Errorf("edit request = %v", edit.Msg.GetGeneration())
	}
	second := master.waitGeneration(campaign, edit.Msg.GetGeneration().GetId())
	child := second.GetImage()
	if child == nil || child.GetParentImageId() != img.GetId() || !child.GetGenerated() || child.GetName() != img.GetName()+" (ajuste)" {
		t.Fatalf("edited image = %v", child)
	}

	// Stateless: the second call carried the previous picture, the first text
	// and the new instruction, and said store=false.
	calls := fake.Calls()
	if len(calls) != 2 {
		t.Fatalf("the model was called %d times, want 2", len(calls))
	}
	e := calls[1].Request.Edit
	if e == nil || e.Original != "Uma cripta úmida, tochas apagadas" || e.Instruction != "mais escura, com uma ponte sobre o poço" || len(e.Previous.Data) == 0 {
		t.Errorf("edit sent = %+v", e)
	}
	if !strings.Contains(string(calls[1].Body), `"store":false`) {
		t.Errorf("the body does not say store=false: %s", calls[1].Body)
	}

	// A third edit, of the second: the chain is the whole tree, oldest first.
	third, err := master.imagegen.EditGeneratedImage(t.Context(), connect.NewRequest(&mapsv1.EditGeneratedImageRequest{
		CampaignId: campaign, ImageId: child.GetId(), IdempotencyKey: nextKey(), Instruction: "com neblina",
	}))
	if err != nil {
		t.Fatal(err)
	}
	last := master.waitGeneration(campaign, third.Msg.GetGeneration().GetId()).GetImage()
	if e := fake.Calls()[2].Request.Edit; e == nil || len(e.Adjustments) != 1 || e.Adjustments[0] != "mais escura, com uma ponte sobre o poço" {
		t.Errorf("the third call's earlier adjustments = %+v", e)
	}
	for _, from := range []string{img.GetId(), child.GetId(), last.GetId()} {
		chain, err := master.imagegen.ListImageEdits(t.Context(), connect.NewRequest(&mapsv1.ListImageEditsRequest{CampaignId: campaign, ImageId: from}))
		if err != nil {
			t.Fatalf("ListImageEdits() error = %v", err)
		}
		var ids []string
		for _, e := range chain.Msg.GetEdits() {
			ids = append(ids, e.GetImage().GetId())
		}
		if want := []string{img.GetId(), child.GetId(), last.GetId()}; strings.Join(ids, ",") != strings.Join(want, ",") {
			t.Errorf("chain from %s = %v, want %v", from, ids, want)
		}
		if chain.Msg.GetEdits()[1].GetPrompt() != "mais escura, com uma ponte sobre o poço" || chain.Msg.GetEdits()[2].GetNumber() != 3 {
			t.Errorf("chain details = %v", chain.Msg.GetEdits())
		}
	}

	// An uploaded image is not a generated one: it cannot be edited.
	uploaded := master.newImage(campaign)
	_, err = master.imagegen.EditGeneratedImage(t.Context(), connect.NewRequest(&mapsv1.EditGeneratedImageRequest{CampaignId: campaign, ImageId: uploaded, IdempotencyKey: nextKey(), Instruction: "x"}))
	wantCode(t, "EditGeneratedImage of an uploaded image", err, connect.CodeNotFound)

	// The gallery lists them, labeled.
	list, err := master.gallery.ListGalleryImages(t.Context(), connect.NewRequest(&mapsv1.ListGalleryImagesRequest{CampaignId: campaign}))
	if err != nil {
		t.Fatal(err)
	}
	generated := 0
	for _, i := range list.Msg.GetImages() {
		if i.GetGenerated() {
			generated++
		}
	}
	if generated != 3 || len(list.Msg.GetImages()) != 4 {
		t.Errorf("the gallery has %d generated of %d images, want 3 of 4", generated, len(list.Msg.GetImages()))
	}
}

// RN-10: a generated image is hidden like any image, and a player never reads
// the requests. After the master shows it, the player fetches that image only.
func TestRN10_AGeneratedImageIsHiddenUntilShown(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withFake(&gen.Fake{}, 10))
	master, player := h.newUser("Mestre"), h.newUser("Jogadora")
	campaign := h.newCampaign(master, player)
	res := master.mustGenerate(campaign, "O segredo do poço")
	img := res.GetImage()

	for _, path := range []string{img.GetUrl(), img.GetThumbnailUrl()} {
		if got := player.get(path); got.status != http.StatusNotFound {
			t.Errorf("player GET %s = %d, want 404 before it is shown", path, got.status)
		}
	}
	// The player cannot read a request, and every call answers not_found.
	_, err := player.imagegen.GetImageGeneration(t.Context(), connect.NewRequest(&mapsv1.GetImageGenerationRequest{CampaignId: campaign, GenerationId: res.GetGeneration().GetId()}))
	wantCode(t, "GetImageGeneration as a player", err, connect.CodeNotFound)
	_, err = player.gallery.ListGalleryImages(t.Context(), connect.NewRequest(&mapsv1.ListGalleryImagesRequest{CampaignId: campaign}))
	wantCode(t, "ListGalleryImages as a player", err, connect.CodePermissionDenied)

	// The master shows it: the player sees the image, and the JSON they get
	// has no request, text or chain.
	master.start(campaign)
	_, err = master.play.SetShownImage(t.Context(), connect.NewRequest(&playv1.SetShownImageRequest{CampaignId: campaign, ImageId: img.GetId()}))
	if err != nil {
		t.Fatalf("SetShownImage() error = %v", err)
	}
	if got := player.get(img.GetUrl()); got.status != http.StatusOK {
		t.Errorf("player GET after showing = %d", got.status)
	}
	// An edit shown to the players too: they get the image, never the text or the chain.
	edit, err := master.imagegen.EditGeneratedImage(t.Context(), connect.NewRequest(&mapsv1.EditGeneratedImageRequest{
		CampaignId: campaign, ImageId: img.GetId(), IdempotencyKey: nextKey(), Instruction: "com uma passagem escondida",
	}))
	if err != nil {
		t.Fatal(err)
	}
	child := master.waitGeneration(campaign, edit.Msg.GetGeneration().GetId()).GetImage()
	if got := player.get(child.GetUrl()); got.status != http.StatusNotFound {
		t.Errorf("player GET of the edit before it is shown = %d, want 404", got.status)
	}
	shownEdit, err := master.play.SetShownImage(t.Context(), connect.NewRequest(&playv1.SetShownImageRequest{CampaignId: campaign, ImageId: child.GetId()}))
	if err != nil {
		t.Fatalf("SetShownImage() of the edit error = %v", err)
	}
	if got := player.get(child.GetUrl()); got.status != http.StatusOK {
		t.Errorf("player GET of the shown edit = %d", got.status)
	}
	if got := player.get(img.GetUrl()); got.status != http.StatusNotFound {
		t.Errorf("player GET of the parent after the edit is shown = %d, want 404", got.status)
	}
	live, err := player.play.GetLiveSession(t.Context(), connect.NewRequest(&playv1.GetLiveSessionRequest{CampaignId: campaign}))
	if err != nil {
		t.Fatal(err)
	}
	// The player's answers as the app reads them (the JSON encoding of the
	// messages): the shown image, and nothing of the request or the chain.
	for name, msg := range map[string]proto.Message{"GetLiveSession": live.Msg, "SetShownImage": shownEdit.Msg} {
		out, err := protojson.Marshal(msg)
		if err != nil {
			t.Fatal(err)
		}
		json := strings.ToLower(string(out))
		for _, secret := range []string{"o segredo do poço", "passagem escondida", "parent", "generated", "idempotency", res.GetGeneration().GetId(), img.GetId()} {
			if strings.Contains(json, strings.ToLower(secret)) {
				t.Errorf("a player's %s answer has %q: %s", name, secret, out)
			}
		}
		if name == "GetLiveSession" && !strings.Contains(json, child.GetId()) {
			t.Errorf("the player's live session lacks the shown edit: %s", out)
		}
	}
	// A second generated image that was not shown stays hidden.
	other := master.mustGenerate(campaign, "Outra cena").GetImage()
	if got := player.get(other.GetUrl()); got.status != http.StatusNotFound {
		t.Errorf("player GET of the other image = %d, want 404", got.status)
	}
}

// RN-28: every RPC is the master's. Everyone else gets not_found, or
// unauthenticated without a session.
func TestMR039_OnlyTheMasterGenerates(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withFake(&gen.Fake{}, 10))
	master, player, pending, outsider := h.newUser("Mestre"), h.newUser("Jogadora"), h.newUser("Pendente"), h.newUser("De fora")
	campaign := h.newCampaign(master, player)
	h.join(master, campaign, true, pending)
	made := master.mustGenerate(campaign, "Uma cena")
	img := made.GetImage().GetId()
	id := made.GetGeneration().GetId()

	calls := map[string]func(ctx context.Context, u *user) error{
		"GetImageGenerationStatus": func(ctx context.Context, u *user) error {
			_, err := u.imagegen.GetImageGenerationStatus(ctx, connect.NewRequest(&mapsv1.GetImageGenerationStatusRequest{CampaignId: campaign}))
			return err
		},
		"GenerateSceneImage": func(ctx context.Context, u *user) error {
			_, err := u.imagegen.GenerateSceneImage(ctx, connect.NewRequest(&mapsv1.GenerateSceneImageRequest{CampaignId: campaign, IdempotencyKey: nextKey(), Prompt: "x"}))
			return err
		},
		"EditGeneratedImage": func(ctx context.Context, u *user) error {
			_, err := u.imagegen.EditGeneratedImage(ctx, connect.NewRequest(&mapsv1.EditGeneratedImageRequest{CampaignId: campaign, ImageId: img, IdempotencyKey: nextKey(), Instruction: "x"}))
			return err
		},
		"GetImageGeneration": func(ctx context.Context, u *user) error {
			_, err := u.imagegen.GetImageGeneration(ctx, connect.NewRequest(&mapsv1.GetImageGenerationRequest{CampaignId: campaign, GenerationId: id}))
			return err
		},
		"CancelImageGeneration": func(ctx context.Context, u *user) error {
			_, err := u.imagegen.CancelImageGeneration(ctx, connect.NewRequest(&mapsv1.CancelImageGenerationRequest{CampaignId: campaign, GenerationId: id}))
			return err
		},
		"ListImageEdits": func(ctx context.Context, u *user) error {
			_, err := u.imagegen.ListImageEdits(ctx, connect.NewRequest(&mapsv1.ListImageEditsRequest{CampaignId: campaign, ImageId: img}))
			return err
		},
	}
	for name, call := range calls {
		if err := call(t.Context(), master); err != nil {
			t.Errorf("%s as the master: %v", name, err)
		}
		for who, u := range map[string]*user{"a player": player, "a pending member": pending, "a non-member": outsider} {
			wantCode(t, name+" as "+who, call(t.Context(), u), connect.CodeNotFound)
		}
		wantCode(t, name+" without a session", call(t.Context(), h.anonymous()), connect.CodeUnauthenticated)
	}
	// Nothing was generated for them: the master's month shows one image.
	if got := master.imageStatus(campaign).GetUsedThisMonth(); got < 1 {
		t.Errorf("used = %d", got)
	}
}

// RN-28: the monthly cap per campaign. A refusal gives the slot back; at the
// limit the request is refused with the month's numbers; another campaign has
// its own count.
func TestMR039_TheMonthlyCap(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withFake(&gen.Fake{}, 2))
	master := h.newUser("Mestre")
	campaign, other := h.newCampaign(master), h.newCampaign(master)

	st := master.imageStatus(campaign)
	if !st.GetEnabled() || st.GetMonthlyLimit() != 2 || st.GetRemaining() != 2 || st.GetUsedThisMonth() != 0 || st.GetMonth() == "" || st.GetResetsAt() == nil {
		t.Fatalf("status = %v", st)
	}
	if st.GetMaxPromptCharacters() != 500 || st.GetMaxObjectReferences() != 10 || st.GetMaxCharacterReferences() != 4 {
		t.Errorf("limits = %v", st)
	}
	// A refused request does not count.
	refused := master.mustGenerate(campaign, "x "+gen.MarkerRefuse)
	if refused.GetGeneration().GetState() != mapsv1.ImageGenerationState_IMAGE_GENERATION_STATE_REFUSED || refused.GetGeneration().GetSlotSpent() {
		t.Errorf("refused = %v", refused.GetGeneration())
	}
	if got := master.imageStatus(campaign).GetRemaining(); got != 2 {
		t.Errorf("remaining after a refusal = %d, want 2", got)
	}
	master.mustGenerate(campaign, "um")
	master.mustGenerate(campaign, "dois")
	if got := master.imageStatus(campaign); got.GetRemaining() != 0 || got.GetUsedThisMonth() != 2 {
		t.Errorf("status at the limit = %v", got)
	}
	_, err := master.generate(campaign, "três")
	b := wantGenerationBlocked(t, "GenerateSceneImage at the limit", err, mapsv1.ImageGenerationBlockedReason_IMAGE_GENERATION_BLOCKED_REASON_LIMIT_REACHED)
	if b.GetStatus().GetMonthlyLimit() != 2 || b.GetStatus().GetResetsAt() == nil {
		t.Errorf("blocked status = %v", b.GetStatus())
	}
	// An edit costs a slot too.
	first, _ := master.imagegen.ListImageEdits(t.Context(), connect.NewRequest(&mapsv1.ListImageEditsRequest{CampaignId: campaign, ImageId: mustFirstGenerated(t, master, campaign)}))
	_, err = master.imagegen.EditGeneratedImage(t.Context(), connect.NewRequest(&mapsv1.EditGeneratedImageRequest{CampaignId: campaign, ImageId: first.Msg.GetEdits()[0].GetImage().GetId(), IdempotencyKey: nextKey(), Instruction: "x"}))
	wantGenerationBlocked(t, "EditGeneratedImage at the limit", err, mapsv1.ImageGenerationBlockedReason_IMAGE_GENERATION_BLOCKED_REASON_LIMIT_REACHED)
	// The other campaign is not affected.
	master.mustGenerate(other, "outra campanha")
	if got := master.imageStatus(other).GetUsedThisMonth(); got != 1 {
		t.Errorf("the other campaign used %d, want 1", got)
	}
}

func mustFirstGenerated(t *testing.T, u *user, campaign string) string {
	t.Helper()
	list, err := u.gallery.ListGalleryImages(t.Context(), connect.NewRequest(&mapsv1.ListGalleryImagesRequest{CampaignId: campaign}))
	if err != nil || len(list.Msg.GetImages()) == 0 {
		t.Fatalf("ListGalleryImages() = %v, %v", list, err)
	}
	return list.Msg.GetImages()[0].GetId()
}

// A reply without a picture, a refusal and a service that fails all give the
// slot back and say why in Portuguese, without the model's own words.
func TestMR039_FailuresGiveTheSlotBack(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withFake(&gen.Fake{}, 5))
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master)
	for _, tc := range []struct {
		marker  string
		state   mapsv1.ImageGenerationState
		failure mapsv1.ImageGenerationFailure
		pt      string
	}{
		{gen.MarkerEmpty, mapsv1.ImageGenerationState_IMAGE_GENERATION_STATE_FAILED, mapsv1.ImageGenerationFailure_IMAGE_GENERATION_FAILURE_NO_IMAGE, "O serviço não gerou uma imagem"},
		{gen.MarkerRefuse, mapsv1.ImageGenerationState_IMAGE_GENERATION_STATE_REFUSED, mapsv1.ImageGenerationFailure_IMAGE_GENERATION_FAILURE_REFUSED, "recusou"},
	} {
		res := master.mustGenerate(campaign, "uma cena "+tc.marker)
		g := res.GetGeneration()
		if g.GetState() != tc.state || g.GetFailure() != tc.failure || !strings.Contains(g.GetReasonPt(), tc.pt) || g.GetSlotSpent() || res.GetImage() != nil {
			t.Errorf("%s: %v", tc.marker, g)
		}
		if got := res.GetStatus().GetRemaining(); got != 5 {
			t.Errorf("%s: remaining = %d, want 5", tc.marker, got)
		}
	}
	list, _ := master.gallery.ListGalleryImages(t.Context(), connect.NewRequest(&mapsv1.ListGalleryImagesRequest{CampaignId: campaign}))
	if n := len(list.Msg.GetImages()); n != 0 {
		t.Errorf("the gallery has %d images after two failures", n)
	}
}

// A call that failed in a way that may have been billed (a timeout, a cut connection,
// an answer that cannot be read) keeps the month's slot; the control is a refusal, which
// surely was not billed and gives it back.
func TestMR039_AFailureThatMayHaveBeenBilledKeepsTheSlot(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withFake(&gen.Fake{}, 5))
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master)
	res := master.mustGenerate(campaign, "uma cena "+gen.MarkerRefuse)
	if res.GetGeneration().GetSlotSpent() || res.GetStatus().GetRemaining() != 5 {
		t.Fatalf("a refusal: %v, remaining %d; want the slot back", res.GetGeneration(), res.GetStatus().GetRemaining())
	}
	res = master.mustGenerate(campaign, "uma cena "+gen.MarkerError)
	g := res.GetGeneration()
	if g.GetState() != mapsv1.ImageGenerationState_IMAGE_GENERATION_STATE_FAILED || !g.GetSlotSpent() || res.GetImage() != nil {
		t.Errorf("a failed call: %v, want FAILED with the slot spent", g)
	}
	if got := res.GetStatus().GetRemaining(); got != 4 {
		t.Errorf("remaining = %d after a call that may have been billed, want 4", got)
	}
}

// A panic while a request runs (a model answer that breaks the decoder, say)
// fails that one request and gives the slot back; the process lives on.
func TestMR039_APanicInTheJobFailsOnlyThatRequest(t *testing.T) {
	t.Parallel()
	var panicked atomic.Bool
	fake := &gen.Fake{Hook: func(context.Context, gen.Request) error {
		if panicked.CompareAndSwap(false, true) {
			panic("the decoder met something unexpected")
		}
		return nil
	}}
	h := newHarness(t, withFake(fake, 5))
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master)

	res := master.mustGenerate(campaign, "uma cena que quebra")
	g := res.GetGeneration()
	if g.GetState() != mapsv1.ImageGenerationState_IMAGE_GENERATION_STATE_FAILED || g.GetSlotSpent() || res.GetStatus().GetRemaining() != 5 {
		t.Errorf("after a panic: %v, status %v", g, res.GetStatus())
	}
	// The next request runs as usual.
	if next := master.mustGenerate(campaign, "outra cena"); next.GetGeneration().GetState() != mapsv1.ImageGenerationState_IMAGE_GENERATION_STATE_DONE {
		t.Errorf("the request after the panic: %v", next.GetGeneration())
	}
}

// A retry with the same key never generates twice.
func TestMR039_TheSameKeyGeneratesOnce(t *testing.T) {
	t.Parallel()
	fake := &gen.Fake{}
	h := newHarness(t, withFake(fake, 5))
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master)
	ask := func() *mapsv1.GenerateSceneImageResponse {
		res, err := master.imagegen.GenerateSceneImage(t.Context(), connect.NewRequest(&mapsv1.GenerateSceneImageRequest{CampaignId: campaign, IdempotencyKey: "same-key", Prompt: "uma cena"}))
		if err != nil {
			t.Fatalf("GenerateSceneImage() error = %v", err)
		}
		return res.Msg
	}
	a := ask()
	b := ask()
	if a.GetGeneration().GetId() != b.GetGeneration().GetId() {
		t.Errorf("two requests, %s and %s", a.GetGeneration().GetId(), b.GetGeneration().GetId())
	}
	master.waitGeneration(campaign, a.GetGeneration().GetId())
	c := ask() // after it finished
	if c.GetGeneration().GetId() != a.GetGeneration().GetId() || c.GetStatus().GetUsedThisMonth() != 1 {
		t.Errorf("third try = %v", c)
	}
	if n := len(fake.Calls()); n != 1 {
		t.Errorf("the model was called %d times, want 1", n)
	}
	list, _ := master.gallery.ListGalleryImages(t.Context(), connect.NewRequest(&mapsv1.ListGalleryImagesRequest{CampaignId: campaign}))
	if n := len(list.Msg.GetImages()); n != 1 {
		t.Errorf("the gallery has %d images, want 1", n)
	}
}

// "Cancelar" before the request left the server gives the slot back and
// nothing is sent; after it left, only the wait stops: the slot stays spent and
// the picture, when it comes, goes to the gallery.
func TestMR039_CancelBeforeAndAfterTheRequestLeaves(t *testing.T) {
	t.Parallel()
	entered, release := make(chan struct{}, 4), make(chan struct{})
	fake := &gen.Fake{Hook: func(ctx context.Context, _ gen.Request) error {
		entered <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
		}
		return nil
	}}
	h := newHarness(t, withFake(fake, 5))
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master)

	// Before: every call slot is taken, so the request waits on the server.
	for range maxGenerating {
		h.svc.generating <- struct{}{}
	}
	waiting, err := master.generate(campaign, "ainda não saiu")
	if err != nil {
		t.Fatal(err)
	}
	cancel, err := master.imagegen.CancelImageGeneration(t.Context(), connect.NewRequest(&mapsv1.CancelImageGenerationRequest{CampaignId: campaign, GenerationId: waiting.GetGeneration().GetId()}))
	if err != nil {
		t.Fatalf("CancelImageGeneration() error = %v", err)
	}
	if g := cancel.Msg.GetGeneration(); g.GetState() != mapsv1.ImageGenerationState_IMAGE_GENERATION_STATE_CANCELED || g.GetSlotSpent() || cancel.Msg.GetStatus().GetRemaining() != 5 {
		t.Errorf("canceled before it left = %v, status %v", g, cancel.Msg.GetStatus())
	}
	for range maxGenerating {
		<-h.svc.generating
	}
	h.svc.WaitForGenerations(t.Context()) // the goroutine finds it canceled and never calls the model
	if n := len(fake.Calls()); n != 0 {
		t.Fatalf("the model was called %d times for a canceled request", n)
	}

	// After: the model has the request and is slow.
	sent, err := master.generate(campaign, "já saiu")
	if err != nil {
		t.Fatal(err)
	}
	<-entered
	cancel, err = master.imagegen.CancelImageGeneration(t.Context(), connect.NewRequest(&mapsv1.CancelImageGenerationRequest{CampaignId: campaign, GenerationId: sent.GetGeneration().GetId()}))
	if err != nil {
		t.Fatal(err)
	}
	if g := cancel.Msg.GetGeneration(); g.GetState() != mapsv1.ImageGenerationState_IMAGE_GENERATION_STATE_CANCELED || !g.GetSlotSpent() || cancel.Msg.GetStatus().GetRemaining() != 4 {
		t.Errorf("canceled after it left = %v, status %v", g, cancel.Msg.GetStatus())
	}
	close(release)
	// The picture still arrives: the gallery has it, and the request still says CANCELED.
	deadline := time.Now().Add(30 * time.Second)
	for {
		res, err := master.imagegen.GetImageGeneration(t.Context(), connect.NewRequest(&mapsv1.GetImageGenerationRequest{CampaignId: campaign, GenerationId: sent.GetGeneration().GetId()}))
		if err != nil {
			t.Fatal(err)
		}
		if res.Msg.GetImage() != nil {
			if res.Msg.GetGeneration().GetState() != mapsv1.ImageGenerationState_IMAGE_GENERATION_STATE_CANCELED || !res.Msg.GetGeneration().GetSlotSpent() || !res.Msg.GetImage().GetGenerated() {
				t.Errorf("after the picture came = %v", res.Msg)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the picture of a request canceled after it left never reached the gallery")
		}
		time.Sleep(10 * time.Millisecond)
	}
	// Cancel on a request that already ended returns it as it is.
	again, err := master.imagegen.CancelImageGeneration(t.Context(), connect.NewRequest(&mapsv1.CancelImageGenerationRequest{CampaignId: campaign, GenerationId: sent.GetGeneration().GetId()}))
	if err != nil || again.Msg.GetGeneration().GetImageId() == "" {
		t.Errorf("cancel again = %v, %v", again, err)
	}
}

// Generation is off without a generator: a typed reason, and the rest works.
func TestMR039_GenerationOffWithoutAKey(t *testing.T) {
	t.Parallel()
	h := newHarness(t) // no generator
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master)
	if st := master.imageStatus(campaign); st.GetEnabled() {
		t.Errorf("status = %v, want enabled false", st)
	}
	_, err := master.generate(campaign, "uma cena")
	wantGenerationBlocked(t, "GenerateSceneImage without a key", err, mapsv1.ImageGenerationBlockedReason_IMAGE_GENERATION_BLOCKED_REASON_OFF)
	if master.newImage(campaign) == "" { // uploads still work
		t.Error("upload")
	}
}

// With the cap at its last slot, requests racing for it: only one wins.
func TestMR039_TheLastSlotGoesToOneRequest(t *testing.T) {
	t.Parallel()
	const racers = 6
	dbtest.PoolSize(t, racers+2)
	fake := &gen.Fake{}
	h := newHarness(t, withFake(fake, 1))
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master)

	var wg sync.WaitGroup
	errs := make([]error, racers)
	for i := range racers {
		wg.Go(func() {
			_, errs[i] = master.generate(campaign, fmt.Sprintf("cena %d", i))
		})
	}
	wg.Wait()
	won := 0
	for _, err := range errs {
		if err == nil {
			won++
			continue
		}
		wantGenerationBlocked(t, "a racer", err, mapsv1.ImageGenerationBlockedReason_IMAGE_GENERATION_BLOCKED_REASON_LIMIT_REACHED)
	}
	if won != 1 {
		t.Errorf("%d requests won the last slot, want 1", won)
	}
	h.svc.WaitForGenerations(t.Context())
	if n := len(fake.Calls()); n != 1 {
		t.Errorf("the model was called %d times, want 1", n)
	}
}

// RN-28: what goes to Google is the master's text, the style and the gallery
// images he chose, and never a name or an e-mail of anyone.
func TestRN28_NothingPersonalGoesToTheModel(t *testing.T) {
	t.Parallel()
	fake := &gen.Fake{}
	h := newHarness(t, withFake(fake, 5))
	master, player := h.newUser("Mestre Ambrósio Quintanilha"), h.newUser("Jogadora Zuleika Albuquerque")
	campaign := h.newCampaign(master, player)
	portrait := master.newImage(campaign)
	scenery := master.newImage(campaign)

	res, err := master.imagegen.GenerateSceneImage(t.Context(), connect.NewRequest(&mapsv1.GenerateSceneImageRequest{
		CampaignId: campaign, IdempotencyKey: nextKey(), Prompt: "Uma taverna de madrugada", Style: mapsv1.ImageStyle_IMAGE_STYLE_WATERCOLOR,
		AspectRatio: mapsv1.ImageAspectRatio_IMAGE_ASPECT_RATIO_21_9, ObjectImageIds: []string{scenery}, CharacterImageIds: []string{portrait},
	}))
	if err != nil {
		t.Fatalf("GenerateSceneImage() error = %v", err)
	}
	done := master.waitGeneration(campaign, res.Msg.GetGeneration().GetId())
	if done.GetGeneration().GetState() != mapsv1.ImageGenerationState_IMAGE_GENERATION_STATE_DONE {
		t.Fatalf("request = %v", done.GetGeneration())
	}
	edit, err := master.imagegen.EditGeneratedImage(t.Context(), connect.NewRequest(&mapsv1.EditGeneratedImageRequest{CampaignId: campaign, ImageId: done.GetImage().GetId(), IdempotencyKey: nextKey(), Instruction: "ao amanhecer"}))
	if err != nil {
		t.Fatal(err)
	}
	master.waitGeneration(campaign, edit.Msg.GetGeneration().GetId())

	calls := fake.Calls()
	if len(calls) != 2 {
		t.Fatalf("%d calls, want 2", len(calls))
	}
	// The scene: the references, with the portrait counted as a character.
	r := calls[0].Request
	if r.Prompt != "Uma taverna de madrugada" || r.Style != "watercolor" || r.AspectRatio != "21:9" || len(r.References) != 2 || r.References[0].Character || !r.References[1].Character {
		t.Errorf("scene request = %+v", r)
	}
	if res := done.GetGeneration(); len(res.GetReferenceImageIds()) != 1 || len(res.GetCharacterImageIds()) != 1 || res.GetStyle() != mapsv1.ImageStyle_IMAGE_STYLE_WATERCOLOR {
		t.Errorf("stored request = %v", res)
	}
	// The edit keeps the style and the ratio.
	if e := calls[1].Request; e.Style != "watercolor" || e.AspectRatio != "21:9" || e.Edit == nil {
		t.Errorf("edit request = %+v", e)
	}
	for i, c := range calls {
		body := string(c.Body) + c.Request.Text()
		for _, secret := range []string{"Ambrósio", "Quintanilha", "Zuleika", "Albuquerque", "@", master.id, player.id, campaign} {
			if strings.Contains(body, secret) {
				t.Errorf("call %d carries %q", i, secret)
			}
		}
	}
}

// What the master may send, and what is refused: the text, the key, the
// references.
func TestMR039_TheRequestIsChecked(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withFake(&gen.Fake{}, 50))
	master := h.newUser("Mestre")
	campaign, other := h.newCampaign(master), h.newCampaign(master)
	foreign := master.newImage(other)
	mine := master.newImage(campaign)

	call := func(mod func(*mapsv1.GenerateSceneImageRequest)) error {
		req := &mapsv1.GenerateSceneImageRequest{CampaignId: campaign, IdempotencyKey: nextKey(), Prompt: "uma cena"}
		mod(req)
		_, err := master.imagegen.GenerateSceneImage(t.Context(), connect.NewRequest(req))
		return err
	}
	many := func(id string, n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = id
		}
		return out
	}
	for name, tc := range map[string]struct {
		mod  func(*mapsv1.GenerateSceneImageRequest)
		code connect.Code
	}{
		"an empty text":                   {func(r *mapsv1.GenerateSceneImageRequest) { r.Prompt = "  " }, connect.CodeInvalidArgument},
		"501 characters":                  {func(r *mapsv1.GenerateSceneImageRequest) { r.Prompt = strings.Repeat("á", 501) }, connect.CodeInvalidArgument},
		"a control character":             {func(r *mapsv1.GenerateSceneImageRequest) { r.Prompt = "a\x00b" }, connect.CodeInvalidArgument},
		"no key":                          {func(r *mapsv1.GenerateSceneImageRequest) { r.IdempotencyKey = "" }, connect.CodeInvalidArgument},
		"a key of 65 characters":          {func(r *mapsv1.GenerateSceneImageRequest) { r.IdempotencyKey = strings.Repeat("k", 65) }, connect.CodeInvalidArgument},
		"an unknown style":                {func(r *mapsv1.GenerateSceneImageRequest) { r.Style = 99 }, connect.CodeInvalidArgument},
		"an unknown ratio":                {func(r *mapsv1.GenerateSceneImageRequest) { r.AspectRatio = 99 }, connect.CodeInvalidArgument},
		"11 object references":            {func(r *mapsv1.GenerateSceneImageRequest) { r.ObjectImageIds = many(mine, 11) }, connect.CodeInvalidArgument},
		"5 character references":          {func(r *mapsv1.GenerateSceneImageRequest) { r.CharacterImageIds = many(mine, 5) }, connect.CodeInvalidArgument},
		"the same reference twice":        {func(r *mapsv1.GenerateSceneImageRequest) { r.ObjectImageIds = []string{mine, mine} }, connect.CodeInvalidArgument},
		"a reference of another campaign": {func(r *mapsv1.GenerateSceneImageRequest) { r.ObjectImageIds = []string{foreign} }, connect.CodeNotFound},
		"a reference that is no UUID":     {func(r *mapsv1.GenerateSceneImageRequest) { r.CharacterImageIds = []string{"nope"} }, connect.CodeNotFound},
	} {
		wantCode(t, name, call(tc.mod), tc.code)
	}
	if got := master.imageStatus(campaign).GetUsedThisMonth(); got != 0 {
		t.Errorf("refused requests used %d slots", got)
	}
	// The ends of the limits work: 500 characters, 10 + 4 references, every ratio.
	if err := call(func(r *mapsv1.GenerateSceneImageRequest) {
		r.Prompt = strings.Repeat("é", 500)
		r.ObjectImageIds = nil
	}); err != nil {
		t.Errorf("500 characters: %v", err)
	}
	for ratio := mapsv1.ImageAspectRatio_IMAGE_ASPECT_RATIO_1_1; ratio <= mapsv1.ImageAspectRatio_IMAGE_ASPECT_RATIO_21_9; ratio++ {
		if err := call(func(r *mapsv1.GenerateSceneImageRequest) { r.AspectRatio = ratio }); err != nil {
			t.Errorf("ratio %v: %v", ratio, err)
		}
	}
	ids := func(n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = master.newImage(campaign)
		}
		return out
	}
	if err := call(func(r *mapsv1.GenerateSceneImageRequest) { r.ObjectImageIds = ids(10); r.CharacterImageIds = ids(4) }); err != nil {
		t.Errorf("10 + 4 references: %v", err)
	}
}

// A full gallery refuses before the model is called.
func TestMR039_AFullGalleryRefuses(t *testing.T) {
	t.Parallel()
	fake := &gen.Fake{}
	h := newHarness(t, withFake(fake, 5), func(c *Config) { c.MaxImages = 1 })
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master)
	master.newImage(campaign)
	_, err := master.generate(campaign, "uma cena")
	wantGenerationBlocked(t, "GenerateSceneImage with a full gallery", err, mapsv1.ImageGenerationBlockedReason_IMAGE_GENERATION_BLOCKED_REASON_GALLERY_FULL)
	if len(fake.Calls()) != 0 {
		t.Error("the model was called")
	}
}

// holdingFake is a fake whose calls wait until release is closed (or the test
// ends), saying so on entered: a request "in flight" at the model.
func holdingFake() (f *gen.Fake, entered chan struct{}, release chan struct{}) {
	entered, release = make(chan struct{}, 16), make(chan struct{})
	f = &gen.Fake{Hook: func(ctx context.Context, _ gen.Request) error {
		entered <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
		}
		return nil
	}}
	return f, entered, release
}

func (u *user) getGeneration(campaign, id string, wait int32) (*mapsv1.GetImageGenerationResponse, error) {
	u.h.t.Helper()
	res, err := u.imagegen.GetImageGeneration(u.h.t.Context(), connect.NewRequest(&mapsv1.GetImageGenerationRequest{CampaignId: campaign, GenerationId: id, WaitSeconds: wait}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

// A sent request that is still pending ten minutes later times out with the slot
// spent (it may have been billed). If its picture still arrives, it is stored
// once, with its request, and the slot is counted once.
func TestMR039_APictureThatArrivesAfterTheExpiryIsStoredOnce(t *testing.T) {
	t.Parallel()
	fake, entered, release := holdingFake()
	h := newHarness(t, withFake(fake, 5))
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master)
	res, err := master.generate(campaign, "uma cena")
	if err != nil {
		t.Fatal(err)
	}
	id := res.GetGeneration().GetId()
	<-entered
	if _, err := h.pool.Exec(t.Context(), `UPDATE image_requests SET sent_at = sent_at - INTERVAL '1 hour' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	got, err := master.getGeneration(campaign, id, 0)
	if err != nil {
		t.Fatal(err)
	}
	g := got.GetGeneration()
	if g.GetState() != mapsv1.ImageGenerationState_IMAGE_GENERATION_STATE_FAILED || g.GetFailure() != mapsv1.ImageGenerationFailure_IMAGE_GENERATION_FAILURE_TIMEOUT || !g.GetSlotSpent() || got.GetStatus().GetRemaining() != 4 {
		t.Fatalf("expired sent request = %v, status %v; want failed, timed out, slot spent", g, got.GetStatus())
	}
	close(release)
	// The picture arrives late: the request becomes DONE with its image.
	deadline := time.Now().Add(30 * time.Second)
	for {
		got, err = master.getGeneration(campaign, id, 0)
		if err != nil {
			t.Fatal(err)
		}
		if got.GetImage() != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the late picture never reached the gallery")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if g := got.GetGeneration(); g.GetState() != mapsv1.ImageGenerationState_IMAGE_GENERATION_STATE_DONE || !g.GetSlotSpent() || g.GetFailure() != mapsv1.ImageGenerationFailure_IMAGE_GENERATION_FAILURE_UNSPECIFIED || got.GetStatus().GetUsedThisMonth() != 1 {
		t.Errorf("after the late picture = %v, status %v", g, got.GetStatus())
	}
	h.svc.WaitForGenerations(t.Context())
	list, _ := master.gallery.ListGalleryImages(t.Context(), connect.NewRequest(&mapsv1.ListGalleryImagesRequest{CampaignId: campaign}))
	if n := len(list.Msg.GetImages()); n != 1 {
		t.Errorf("the gallery has %d images, want 1", n)
	}
}

// A request that never left and is pending ten minutes later gives its slot
// back, and the status says so.
func TestMR039_AnUnsentStaleRequestRefunds(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withFake(&gen.Fake{}, 5))
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master)
	for range maxGenerating {
		h.svc.generating <- struct{}{} // no call slot: the request waits on the server
	}
	res, err := master.generate(campaign, "uma cena")
	if err != nil {
		t.Fatal(err)
	}
	if got := master.imageStatus(campaign).GetRemaining(); got != 4 {
		t.Fatalf("remaining = %d, want 4 with a request waiting", got)
	}
	if _, err := h.pool.Exec(t.Context(), `UPDATE image_requests SET created_at = created_at - INTERVAL '1 hour' WHERE id = $1`, res.GetGeneration().GetId()); err != nil {
		t.Fatal(err)
	}
	// The status alone expires it, so "Restam" is right.
	if got := master.imageStatus(campaign).GetRemaining(); got != 5 {
		t.Errorf("remaining after the expiry = %d, want 5", got)
	}
	got, _ := master.getGeneration(campaign, res.GetGeneration().GetId(), 0)
	if g := got.GetGeneration(); g.GetState() != mapsv1.ImageGenerationState_IMAGE_GENERATION_STATE_FAILED || g.GetSlotSpent() {
		t.Errorf("expired unsent request = %v", g)
	}
	for range maxGenerating {
		<-h.svc.generating
	}
}

// The long poll answers at the state change, and at its deadline.
func TestMR039_TheLongPoll(t *testing.T) {
	t.Parallel()
	fake, entered, release := holdingFake()
	h := newHarness(t, withFake(fake, 5))
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master)
	res, err := master.generate(campaign, "uma cena")
	if err != nil {
		t.Fatal(err)
	}
	id := res.GetGeneration().GetId()
	<-entered

	// At the deadline: still pending, after about the wait.
	start := time.Now()
	got, err := master.getGeneration(campaign, id, 1)
	if err != nil || got.GetGeneration().GetState() != mapsv1.ImageGenerationState_IMAGE_GENERATION_STATE_PENDING {
		t.Fatalf("poll at the deadline = %v, %v", got, err)
	}
	if d := time.Since(start); d < 900*time.Millisecond || d > 5*time.Second {
		t.Errorf("the poll took %v, want about 1 s", d)
	}
	// Without a wait it answers at once.
	start = time.Now()
	if _, err := master.getGeneration(campaign, id, 0); err != nil || time.Since(start) > time.Second {
		t.Errorf("no wait took %v, %v", time.Since(start), err)
	}
	// At the state change: a long wait ends as soon as the picture is stored.
	time.AfterFunc(300*time.Millisecond, func() { close(release) })
	start = time.Now()
	got, err = master.getGeneration(campaign, id, 25)
	if err != nil {
		t.Fatal(err)
	}
	if got.GetGeneration().GetState() != mapsv1.ImageGenerationState_IMAGE_GENERATION_STATE_DONE || got.GetImage() == nil {
		t.Errorf("poll at the change = %v", got.GetGeneration())
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Errorf("the poll took %v: it did not return at the state change", d)
	}
	// Anything over 25 is 25, and a bad request id is not_found.
	_, err = master.getGeneration(campaign, "nope", 30)
	wantCode(t, "a long poll of an unknown request", err, connect.CodeNotFound)
}

// At shutdown: the request still waiting for a call slot refunds at once, the
// one in flight is cut off with its slot spent, and a long poll answers.
func TestMR039_Shutdown(t *testing.T) {
	t.Parallel()
	fake, entered, _ := holdingFake()
	h := newHarness(t, withFake(fake, 5))
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master)
	for range maxGenerating - 1 {
		h.svc.generating <- struct{}{} // three call slots taken: one request flies, the next waits
	}
	inFlight, err := master.generate(campaign, "no ar")
	if err != nil {
		t.Fatal(err)
	}
	<-entered
	queued, err := master.generate(campaign, "na fila")
	if err != nil {
		t.Fatal(err)
	}
	polled := make(chan *mapsv1.GetImageGenerationResponse, 1)
	go func() {
		got, _ := master.getGeneration(campaign, queued.GetGeneration().GetId(), 25)
		polled <- got
	}()
	time.Sleep(200 * time.Millisecond)

	start := time.Now()
	h.svc.CancelGenerations()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	h.svc.WaitForGenerations(ctx)
	if time.Since(start) > 1500*time.Millisecond {
		t.Errorf("the shutdown took %v, want about a second at most", time.Since(start))
	}
	<-polled // the long poll answered at the shutdown, maybe before the refund was written
	got, err := master.getGeneration(campaign, queued.GetGeneration().GetId(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if got.GetGeneration().GetState() != mapsv1.ImageGenerationState_IMAGE_GENERATION_STATE_FAILED || got.GetGeneration().GetSlotSpent() {
		t.Errorf("the queued request = %v, want failed with the slot back", got.GetGeneration())
	}
	flying, err := master.getGeneration(campaign, inFlight.GetGeneration().GetId(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if g := flying.GetGeneration(); g.GetState() != mapsv1.ImageGenerationState_IMAGE_GENERATION_STATE_PENDING || !g.GetSlotSpent() {
		t.Errorf("the request in flight = %v, want pending with the slot spent (it follows the expiry)", g)
	}
	if st := flying.GetStatus(); st.GetRemaining() != 4 {
		t.Errorf("remaining = %d, want 4 (only the sent request is spent)", st.GetRemaining())
	}
	for range maxGenerating - 1 {
		<-h.svc.generating
	}
}

// The master deletes the image being edited while the edit is in the air: the
// new image is kept, without a parent, and the slot stays spent.
func TestMR039_AParentDeletedDuringAnEdit(t *testing.T) {
	t.Parallel()
	var hold atomic.Bool
	entered, release := make(chan struct{}, 4), make(chan struct{})
	h := newHarness(t, withFake(&gen.Fake{Hook: func(ctx context.Context, _ gen.Request) error {
		if hold.Load() {
			entered <- struct{}{}
			select {
			case <-release:
			case <-ctx.Done():
			}
		}
		return nil
	}}, 5))
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master)
	parent := master.mustGenerate(campaign, "uma cena").GetImage()
	hold.Store(true)
	edit, err := master.imagegen.EditGeneratedImage(t.Context(), connect.NewRequest(&mapsv1.EditGeneratedImageRequest{CampaignId: campaign, ImageId: parent.GetId(), IdempotencyKey: nextKey(), Instruction: "mais escura"}))
	if err != nil {
		t.Fatal(err)
	}
	<-entered
	if _, err := master.gallery.DeleteGalleryImage(t.Context(), connect.NewRequest(&mapsv1.DeleteGalleryImageRequest{CampaignId: campaign, ImageId: parent.GetId()})); err != nil {
		t.Fatalf("DeleteGalleryImage() error = %v", err)
	}
	close(release)
	done := master.waitGeneration(campaign, edit.Msg.GetGeneration().GetId())
	if done.GetGeneration().GetState() != mapsv1.ImageGenerationState_IMAGE_GENERATION_STATE_DONE || done.GetImage() == nil || done.GetImage().GetParentImageId() != "" || !done.GetGeneration().GetSlotSpent() {
		t.Errorf("the edit of a deleted parent = %v, image %v", done.GetGeneration(), done.GetImage())
	}
	if done.GetStatus().GetUsedThisMonth() != 2 {
		t.Errorf("used = %d, want 2", done.GetStatus().GetUsedThisMonth())
	}
}

// The service refusing our key is the operator's problem: a typed failure that is
// not a refusal, with the slot back.
func TestMR039_ARefusedKeyIsNotARefusal(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withFake(&gen.Fake{Hook: func(context.Context, gen.Request) error {
		return &gen.NotAuthorizedError{Status: 403, Reason: "API_KEY_SERVICE_BLOCKED"}
	}}, 5))
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master)
	res := master.mustGenerate(campaign, "uma cena")
	g := res.GetGeneration()
	if g.GetState() != mapsv1.ImageGenerationState_IMAGE_GENERATION_STATE_FAILED || g.GetFailure() != mapsv1.ImageGenerationFailure_IMAGE_GENERATION_FAILURE_SERVICE_OFF ||
		g.GetReasonPt() != "O serviço de imagens não está disponível." || g.GetSlotSpent() || res.GetStatus().GetRemaining() != 5 {
		t.Errorf("request = %v, status %v", g, res.GetStatus())
	}
}

// Requests in flight count against the gallery's room, so a full gallery cannot
// fail after the model was paid.
func TestMR039_RequestsInFlightCountAgainstTheGallery(t *testing.T) {
	t.Parallel()
	fake, entered, release := holdingFake()
	h := newHarness(t, withFake(fake, 5), func(c *Config) { c.MaxImages = 2 })
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master)
	master.newImage(campaign)
	if _, err := master.generate(campaign, "a primeira"); err != nil {
		t.Fatal(err)
	}
	<-entered
	_, err := master.generate(campaign, "a segunda")
	wantGenerationBlocked(t, "GenerateSceneImage with the last room taken by a request in flight", err, mapsv1.ImageGenerationBlockedReason_IMAGE_GENERATION_BLOCKED_REASON_GALLERY_FULL)
	close(release)
}

// The references are shrunk, and a request too big even then is refused before
// a slot is reserved.
func TestMR039_TheReferencesAreShrunkAndTheRequestIsCapped(t *testing.T) {
	t.Parallel()
	fake := &gen.Fake{}
	h := newHarness(t, withFake(fake, 5))
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master)
	big := master.mustUpload(campaign, "grande.png", noisePNG(t, 1500, 1000)).GetId()
	res, err := master.imagegen.GenerateSceneImage(t.Context(), connect.NewRequest(&mapsv1.GenerateSceneImageRequest{CampaignId: campaign, IdempotencyKey: nextKey(), Prompt: "x", ObjectImageIds: []string{big}}))
	if err != nil {
		t.Fatal(err)
	}
	master.waitGeneration(campaign, res.Msg.GetGeneration().GetId())
	sent := fake.Calls()[0].Request.References[0]
	cfg, _, err := image.DecodeConfig(bytes.NewReader(sent.Data))
	if err != nil || sent.MimeType != "image/jpeg" || max(cfg.Width, cfg.Height) != 1024 {
		t.Errorf("the reference sent = %s %dx%d (%v), want a JPEG of 1024 on the long side", sent.MimeType, cfg.Width, cfg.Height, err)
	}

	// Many incompressible references: over 8 MiB even shrunk.
	var ids []string
	for range 9 {
		ids = append(ids, master.mustUpload(campaign, "ruido.png", noisePNG(t, 1100, 1100)).GetId())
	}
	before := master.imageStatus(campaign).GetUsedThisMonth()
	_, err = master.imagegen.GenerateSceneImage(t.Context(), connect.NewRequest(&mapsv1.GenerateSceneImageRequest{CampaignId: campaign, IdempotencyKey: nextKey(), Prompt: "x", ObjectImageIds: ids}))
	wantGenerationBlocked(t, "GenerateSceneImage with too many big references", err, mapsv1.ImageGenerationBlockedReason_IMAGE_GENERATION_BLOCKED_REASON_REQUEST_TOO_LARGE)
	if got := master.imageStatus(campaign).GetUsedThisMonth(); got != before {
		t.Errorf("a refused request used a slot: %d -> %d", before, got)
	}
}

// A request keeps its shrunk references in memory until its call goes out, so the
// server holds only so many alive at once: the one calling the model and the ones
// waiting for its slot. Another is refused without spending the month's slot, and the
// room comes back as the requests end.
func TestMR039_OnlySoManyRequestsWaitForTheModel(t *testing.T) {
	t.Parallel()
	fake, entered, release := holdingFake()
	h := newHarness(t, withFake(fake, 30))
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master)
	ask := func() (string, error) {
		res, err := master.generate(campaign, "x")
		if err != nil {
			return "", err
		}
		return res.GetGeneration().GetId(), nil
	}
	var ids []string
	for range maxPendingRequests {
		id, err := ask()
		if err != nil {
			t.Fatalf("a request among the first %d: %v", maxPendingRequests, err)
		}
		ids = append(ids, id)
	}
	<-entered
	used := master.imageStatus(campaign).GetUsedThisMonth()
	_, err := ask()
	if connect.CodeOf(err) != connect.CodeResourceExhausted {
		t.Fatalf("a request over %d alive = %v, want resource_exhausted", maxPendingRequests, err)
	}
	if got := master.imageStatus(campaign).GetUsedThisMonth(); got != used {
		t.Errorf("a refused request used a slot: %d -> %d", used, got)
	}
	close(release)
	for _, id := range ids {
		master.waitGeneration(campaign, id)
	}
	if _, err := ask(); err != nil {
		t.Errorf("a request after the others ended: %v", err)
	}
}

func noisePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	if _, err := rand.Read(img.Pix); err != nil {
		t.Fatal(err)
	}
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 255
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// Two calls with the same key at the same time make one request and one call to
// the model, and get the same answer.
func TestMR039_TheSameKeyAtTheSameTime(t *testing.T) {
	t.Parallel()
	const racers = 6
	dbtest.PoolSize(t, racers+2)
	fake := &gen.Fake{}
	h := newHarness(t, withFake(fake, 5))
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master)

	var wg sync.WaitGroup
	ids := make([]string, racers)
	errs := make([]error, racers)
	for i := range racers {
		wg.Go(func() {
			res, err := master.imagegen.GenerateSceneImage(t.Context(), connect.NewRequest(&mapsv1.GenerateSceneImageRequest{CampaignId: campaign, IdempotencyKey: "one-key", Prompt: "uma cena"}))
			errs[i] = err
			if err == nil {
				ids[i] = res.Msg.GetGeneration().GetId()
			}
		})
	}
	wg.Wait()
	for i := range racers {
		if errs[i] != nil || ids[i] != ids[0] {
			t.Errorf("racer %d: id %q, error %v; want %q", i, ids[i], errs[i], ids[0])
		}
	}
	master.waitGeneration(campaign, ids[0])
	if n := len(fake.Calls()); n != 1 {
		t.Errorf("the model was called %d times, want 1", n)
	}
	if got := master.imageStatus(campaign).GetUsedThisMonth(); got != 1 {
		t.Errorf("used = %d, want 1", got)
	}
}

// The 1024 px reference of a big image is made at upload and kept next to it, so
// the original is not decoded again for references; an older image gets its
// reference the first time it is used; deleting the image deletes it.
func TestMR039_TheReferenceIsKeptNextToTheImage(t *testing.T) {
	t.Parallel()
	fake := &gen.Fake{}
	h := newHarness(t, withFake(fake, 10))
	var decodes atomic.Int32
	h.svc.onReferenceDecode = func() { decodes.Add(1) }
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master)
	big := master.mustUpload(campaign, "grande.png", pngImage(t, 2000, 1000))
	small := master.mustUpload(campaign, "pequena.png", pngImage(t, 300, 200))
	refKey := referenceKey(campaign, big.GetId())
	if _, data := h.storedFile(refKey); len(data) == 0 {
		t.Fatal("the upload of a big image made no reference")
	}
	if slices.Contains(h.storedFiles(), referenceKey(campaign, small.GetId())) {
		t.Error("a small image got a reference file")
	}
	ask := func(ids ...string) {
		t.Helper()
		res, err := master.imagegen.GenerateSceneImage(t.Context(), connect.NewRequest(&mapsv1.GenerateSceneImageRequest{CampaignId: campaign, IdempotencyKey: nextKey(), Prompt: "x", ObjectImageIds: ids}))
		if err != nil {
			t.Fatal(err)
		}
		master.waitGeneration(campaign, res.Msg.GetGeneration().GetId())
	}
	ask(big.GetId())
	ask(big.GetId())
	if decodes.Load() != 0 {
		t.Errorf("the original was decoded %d times for references, want 0 (the upload made the reference)", decodes.Load())
	}
	sent := fake.Calls()[0].Request.References[0]
	if cfg, _, err := image.DecodeConfig(bytes.NewReader(sent.Data)); err != nil || max(cfg.Width, cfg.Height) != 1024 || sent.MimeType != "image/jpeg" {
		t.Errorf("the reference sent = %s %v (%v)", sent.MimeType, cfg, err)
	}

	// An older image, without a reference file: made on first use, once.
	if err := h.blobs.Delete(t.Context(), refKey); err != nil {
		t.Fatal(err)
	}
	ask(big.GetId())
	ask(big.GetId())
	ask(big.GetId())
	if decodes.Load() != 1 {
		t.Errorf("the original was decoded %d times, want 1 for three uses", decodes.Load())
	}
	if _, data := h.storedFile(refKey); len(data) == 0 {
		t.Error("the reference was not kept after its first use")
	}

	// Deleting the image deletes its reference.
	if _, err := master.gallery.DeleteGalleryImage(t.Context(), connect.NewRequest(&mapsv1.DeleteGalleryImageRequest{CampaignId: campaign, ImageId: big.GetId()})); err != nil {
		t.Fatal(err)
	}
	for _, f := range h.storedFiles() {
		if strings.Contains(f, big.GetId()) {
			t.Errorf("%s is still stored after the delete", f)
		}
	}
}

// A picture the model returned (the call was billed) that cannot
// be stored because the gallery filled up while the call was in the air must
// keep its slot spent; refunding it lets paid pictures escape the monthly and
// daily caps.
func TestAGalleryFilledMidCallDoesNotRefundAPaidPicture(t *testing.T) {
	t.Parallel()
	fake, entered, release := holdingFake()
	h := newHarness(t, withFake(fake, 5), func(c *Config) { c.MaxImages = 3 })
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master)
	master.newImage(campaign)
	res, err := master.generate(campaign, "uma cena")
	if err != nil {
		t.Fatalf("reserve with room for the picture: %v", err)
	}
	id := res.GetGeneration().GetId()
	<-entered
	// Uploads ignore the open request: fill the gallery meanwhile.
	master.newImage(campaign)
	master.newImage(campaign)
	close(release)
	got := master.waitGeneration(campaign, id)
	if len(fake.Calls()) != 1 {
		t.Fatalf("model calls = %d, want 1", len(fake.Calls()))
	}
	g := got.GetGeneration()
	t.Logf("state=%v failure=%v slotSpent=%v used=%d", g.GetState(), g.GetFailure(), g.GetSlotSpent(), master.imageStatus(campaign).GetUsedThisMonth())
	if g.GetState() != mapsv1.ImageGenerationState_IMAGE_GENERATION_STATE_FAILED {
		t.Fatalf("state = %v, want failed (gallery full)", g.GetState())
	}
	if !g.GetSlotSpent() {
		t.Errorf("a paid picture that could not be stored gave its slot back (SlotSpent=false)")
	}
	if used := master.imageStatus(campaign).GetUsedThisMonth(); used != 1 {
		t.Errorf("UsedThisMonth = %d, want 1 (the model was called and billed)", used)
	}
}

// A generation key is kept with the hash of its request: the same key and request is the first
// generation again, the same key for another prompt is refused instead of answering with the
// first one's image.
func TestAGenerationKeyReusedForAnotherRequestIsRefused(t *testing.T) {
	t.Parallel()
	fake := &gen.Fake{}
	h := newHarness(t, withFake(fake, 5))
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master)
	generate := func(prompt string) (*connect.Response[mapsv1.GenerateSceneImageResponse], error) {
		return master.imagegen.GenerateSceneImage(t.Context(), connect.NewRequest(&mapsv1.GenerateSceneImageRequest{CampaignId: campaign, IdempotencyKey: "k-reuse", Prompt: prompt}))
	}
	first, err := generate("uma taverna")
	if err != nil {
		t.Fatal(err)
	}
	master.waitGeneration(campaign, first.Msg.GetGeneration().GetId())
	again, err := generate("uma taverna")
	if err != nil || again.Msg.GetGeneration().GetId() != first.Msg.GetGeneration().GetId() {
		t.Errorf("same key, same prompt = %v, %v; want the first generation", again.Msg.GetGeneration().GetId(), err)
	}
	_, err = generate("um dragão furioso")
	wantCode(t, "same key, another prompt", err, connect.CodeInvalidArgument)
}
