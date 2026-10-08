package maps

import (
	"bytes"
	"strings"
	"testing"

	"connectrpc.com/connect"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	"github.com/PuraFome/meuRPG/backend/internal/maps/images/gen"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// Fix round 1 of slice 10.16 (MR-039, RN-10, RN-28): a textured map is the whole map, so the gallery
// says so for it and for every edit of it; an edit of a textured map can be used as the map's image;
// the picture has a name the players can read; and a hidden NPC's portrait is refused as an object too.

// editImage asks for an adjustment and waits for the end of the request.
func (u *user) editImage(campaign, imageID, instruction string, change ...func(*mapsv1.EditGeneratedImageRequest)) (*mapsv1.GetImageGenerationResponse, error) {
	u.h.t.Helper()
	req := &mapsv1.EditGeneratedImageRequest{CampaignId: campaign, ImageId: imageID, IdempotencyKey: nextKey(), Instruction: instruction}
	for _, c := range change {
		c(req)
	}
	res, err := u.imagegen.EditGeneratedImage(u.h.t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return u.waitGeneration(campaign, res.Msg.GetGeneration().GetId()), nil
}

func (u *user) galleryImage(campaign, id string) *mapsv1.GalleryImage {
	u.h.t.Helper()
	list, err := u.gallery.ListGalleryImages(u.h.t.Context(), connect.NewRequest(&mapsv1.ListGalleryImagesRequest{CampaignId: campaign}))
	if err != nil {
		u.h.t.Fatalf("ListGalleryImages() error = %v", err)
	}
	for _, i := range list.Msg.GetImages() {
		if i.GetId() == id {
			return i
		}
	}
	u.h.t.Fatalf("image %s is not in the gallery", id)
	return nil
}

// The gallery says "this shows the whole map" for the textured map and for every edit below it, and never for
// the scene art or the isometric view (they start from what the players see), nor for an upload.
func TestRN10_TheGalleryTellsWhichImagesShowTheWholeMap(t *testing.T) {
	t.Parallel()
	c := newCave(t, withFake(&gen.Fake{}, 20))
	m := c.master
	texture := wantDone(t, "textured map", m.mustGenerateFromMap(c.campaign, c.mapID, kindTexturedAPI, "Uma caverna úmida"))
	scene := wantDone(t, "scene art", m.mustGenerateFromMap(c.campaign, c.mapID, kindMapSceneAPI, "Uma sala"))
	iso := wantDone(t, "isometric view", m.mustGenerateFromMap(c.campaign, c.mapID, kindIsometricAPI, "Uma sala"))
	plain := wantDone(t, "scene art from a text", m.mustGenerate(c.campaign, "Uma taverna"))

	for name, tc := range map[string]struct {
		img  *mapsv1.GalleryImage
		want bool
	}{"the textured map": {texture, true}, "the scene art": {scene, false}, "the isometric view": {iso, false}, "a text's scene art": {plain, false}} {
		if tc.img.GetShowsWholeMap() != tc.want || m.galleryImage(c.campaign, tc.img.GetId()).GetShowsWholeMap() != tc.want {
			t.Errorf("%s: shows_whole_map = %v (in the gallery %v), want %v", name, tc.img.GetShowsWholeMap(), m.galleryImage(c.campaign, tc.img.GetId()).GetShowsWholeMap(), tc.want)
		}
	}
	upload := m.mustUpload(c.campaign, "mapa.png", patternImage(t, 40, 40))
	if m.galleryImage(c.campaign, upload.GetId()).GetShowsWholeMap() {
		t.Error("an upload shows the whole map")
	}

	// An edit inherits the way of the root of its chain, however deep.
	editOfTexture, err := m.editImage(c.campaign, texture.GetId(), "mais clara")
	if err != nil {
		t.Fatalf("EditGeneratedImage(texture) error = %v", err)
	}
	child := wantDone(t, "edit of the texture", editOfTexture)
	grandchild, err := m.editImage(c.campaign, child.GetId(), "com musgo")
	if err != nil {
		t.Fatalf("EditGeneratedImage(edit) error = %v", err)
	}
	deep := wantDone(t, "edit of the edit", grandchild)
	editOfScene, err := m.editImage(c.campaign, scene.GetId(), "mais escura")
	if err != nil {
		t.Fatalf("EditGeneratedImage(scene) error = %v", err)
	}
	sceneChild := wantDone(t, "edit of the scene art", editOfScene)
	for name, tc := range map[string]struct {
		img  *mapsv1.GalleryImage
		want bool
	}{"an edit of the texture": {child, true}, "an edit of that edit": {deep, true}, "an edit of the scene art": {sceneChild, false}} {
		if tc.img.GetShowsWholeMap() != tc.want || m.galleryImage(c.campaign, tc.img.GetId()).GetShowsWholeMap() != tc.want {
			t.Errorf("%s: shows_whole_map = %v, want %v", name, tc.img.GetShowsWholeMap(), tc.want)
		}
	}
	// The chain lists the flag too.
	edits, err := m.imagegen.ListImageEdits(t.Context(), connect.NewRequest(&mapsv1.ListImageEditsRequest{CampaignId: c.campaign, ImageId: deep.GetId()}))
	if err != nil {
		t.Fatalf("ListImageEdits() error = %v", err)
	}
	if len(edits.Msg.GetEdits()) != 3 {
		t.Fatalf("the chain has %d images, want 3", len(edits.Msg.GetEdits()))
	}
	for _, e := range edits.Msg.GetEdits() {
		if !e.GetImage().GetShowsWholeMap() {
			t.Errorf("chain image %s does not say it shows the whole map", e.GetImage().GetName())
		}
	}
}

// An edit of a textured map is made at the map's size and can be used as the map's image, with the
// checks of the original: the walls it started from still have to be the map's.
func TestMR039_UseAnEditOfATexturedMap(t *testing.T) {
	t.Parallel()
	c := newCave(t, withFake(&gen.Fake{}, 20))
	m := c.master
	before := m.mustLayers(c.campaign, c.mapID)
	texture := wantDone(t, "textured map", m.mustGenerateFromMap(c.campaign, c.mapID, kindTexturedAPI, "Uma caverna úmida"))
	res, err := m.editImage(c.campaign, texture.GetId(), "pedra mais clara")
	if err != nil {
		t.Fatalf("EditGeneratedImage() error = %v", err)
	}
	edit := wantDone(t, "edit of the texture", res)
	if edit.GetWidth() != 240 || edit.GetHeight() != 160 {
		t.Fatalf("the edit is %dx%d, want the cave's 240x160", edit.GetWidth(), edit.GetHeight())
	}

	used, err := m.useAsMapImage(c.campaign, edit.GetId())
	if err != nil {
		t.Fatalf("Use of an edit error = %v", err)
	}
	if used.GetMap().GetImage().GetId() != edit.GetId() {
		t.Errorf("the map's image = %s, want the edit %s", used.GetMap().GetImage().GetId(), edit.GetId())
	}
	after := m.mustLayers(c.campaign, c.mapID)
	if !bytes.Equal(after.GetWall(), before.GetWall()) {
		t.Error("the walls changed")
	}

	// A picture that is no textured map is still refused, and so is an edit when the walls changed.
	scene := wantDone(t, "scene art", m.mustGenerateFromMap(c.campaign, c.mapID, kindMapSceneAPI, "Uma sala"))
	if _, err := m.useAsMapImage(c.campaign, scene.GetId()); err == nil {
		t.Error("Use of the scene art worked")
	} else {
		wantCode(t, "Use of the scene art", err, connect.CodeNotFound)
	}
	c2 := newCave(t, withFake(&gen.Fake{}, 20))
	tx := wantDone(t, "textured map", c2.master.mustGenerateFromMap(c2.campaign, c2.mapID, kindTexturedAPI, "Uma caverna"))
	ed, err := c2.master.editImage(c2.campaign, tx.GetId(), "mais clara")
	if err != nil {
		t.Fatalf("EditGeneratedImage() error = %v", err)
	}
	editImg := wantDone(t, "edit", ed)
	c2.master.mustPaint(c2.campaign, c2.mapID, mapsv1.MapLayer_MAP_LAYER_WALL, 1, [2]int32{10, 8})
	_, err = c2.master.useAsMapImage(c2.campaign, editImg.GetId())
	wantGenerationBlocked(t, "Use of an edit after a wall was painted", err, mapsv1.ImageGenerationBlockedReason_IMAGE_GENERATION_BLOCKED_REASON_MAP_CHANGED)
}

// An edit of a textured map that was used as the map's image can be used too, whichever was made
// first: the map's image is then the picture "Usar" put there, not the original the edit started
// from. The walls (and the size and grid) are still checked.
func TestMR039_UseAnEditOfAPictureAlreadyUsed(t *testing.T) {
	t.Parallel()
	t.Run("edit made after the use", func(t *testing.T) {
		t.Parallel()
		c := newCave(t, withFake(&gen.Fake{}, 20))
		m := c.master
		t1 := wantDone(t, "textured map", m.mustGenerateFromMap(c.campaign, c.mapID, kindTexturedAPI, "Uma caverna"))
		if _, err := m.useAsMapImage(c.campaign, t1.GetId()); err != nil {
			t.Fatalf("Use of the picture error = %v", err)
		}
		t2 := wantDone(t, "edit", mustEdit(t, m, c.campaign, t1.GetId()))
		used, err := m.useAsMapImage(c.campaign, t2.GetId())
		if err != nil {
			t.Fatalf("Use of the edit after the use of its picture error = %v", err)
		}
		if used.GetMap().GetImage().GetId() != t2.GetId() {
			t.Errorf("the map's image = %s, want the edit %s", used.GetMap().GetImage().GetId(), t2.GetId())
		}
		// An edit of the edit, made over the same map image, fits as well.
		t3 := wantDone(t, "edit of the edit", mustEdit(t, m, c.campaign, t2.GetId()))
		if _, err := m.useAsMapImage(c.campaign, t3.GetId()); err != nil {
			t.Errorf("Use of the edit of the edit error = %v", err)
		}
	})
	t.Run("edit made before the use", func(t *testing.T) {
		t.Parallel()
		c := newCave(t, withFake(&gen.Fake{}, 20))
		m := c.master
		t1 := wantDone(t, "textured map", m.mustGenerateFromMap(c.campaign, c.mapID, kindTexturedAPI, "Uma caverna"))
		t2 := wantDone(t, "edit", mustEdit(t, m, c.campaign, t1.GetId()))
		if _, err := m.useAsMapImage(c.campaign, t1.GetId()); err != nil {
			t.Fatalf("Use of the picture error = %v", err)
		}
		if _, err := m.useAsMapImage(c.campaign, t2.GetId()); err != nil {
			t.Errorf("Use of the edit made before the use of its picture error = %v", err)
		}
	})
	t.Run("walls painted since still refuse it", func(t *testing.T) {
		t.Parallel()
		c := newCave(t, withFake(&gen.Fake{}, 20))
		m := c.master
		t1 := wantDone(t, "textured map", m.mustGenerateFromMap(c.campaign, c.mapID, kindTexturedAPI, "Uma caverna"))
		if _, err := m.useAsMapImage(c.campaign, t1.GetId()); err != nil {
			t.Fatalf("Use of the picture error = %v", err)
		}
		t2 := wantDone(t, "edit", mustEdit(t, m, c.campaign, t1.GetId()))
		m.mustPaint(c.campaign, c.mapID, mapsv1.MapLayer_MAP_LAYER_WALL, 1, [2]int32{10, 8})
		_, err := m.useAsMapImage(c.campaign, t2.GetId())
		wantGenerationBlocked(t, "Use of an edit after a wall was painted", err, mapsv1.ImageGenerationBlockedReason_IMAGE_GENERATION_BLOCKED_REASON_MAP_CHANGED)
	})
}

// mustEdit adjusts a generated image and returns the generation.
func mustEdit(t *testing.T, m *user, campaign, imageID string) *mapsv1.GetImageGenerationResponse {
	t.Helper()
	res, err := m.editImage(campaign, imageID, "mais clara")
	if err != nil {
		t.Fatalf("EditGeneratedImage() error = %v", err)
	}
	return res
}

// The picture is named by the master, or by a default that a player can read: never "Imagem N".
func TestMR039_TheImageHasAMeaningfulName(t *testing.T) {
	t.Parallel()
	c := newCave(t, withFake(&gen.Fake{}, 20))
	m := c.master
	// The default never takes the master's text: a player who is shown the picture reads its name (RN-10).
	text := wantDone(t, "from a text", m.mustGenerate(c.campaign, "Taverna do Corvo Branco à noite"))
	if !strings.HasPrefix(text.GetName(), "Arte da cena · ") || strings.Contains(text.GetName(), "Corvo") {
		t.Errorf("name from a text = %q, want \"Arte da cena · dd/mm\" without the text", text.GetName())
	}
	named, err := m.imagegen.GenerateSceneImage(t.Context(), connect.NewRequest(&mapsv1.GenerateSceneImageRequest{
		CampaignId: c.campaign, IdempotencyKey: nextKey(), Prompt: "Uma taverna", Name: "  A taverna do Javali ",
	}))
	if err != nil {
		t.Fatalf("GenerateSceneImage(name) error = %v", err)
	}
	if got := wantDone(t, "named", m.waitGeneration(c.campaign, named.Msg.GetGeneration().GetId())); got.GetName() != "A taverna do Javali" {
		t.Errorf("the master's name = %q", got.GetName())
	}
	for kind, want := range map[mapsv1.ImageGenerationKind]string{
		kindMapSceneAPI:  "A caverna do Vale Seco · arte da cena",
		kindIsometricAPI: "A caverna do Vale Seco · vista isométrica",
		kindTexturedAPI:  "A caverna do Vale Seco · mapa com textura",
	} {
		if got := wantDone(t, want, m.mustGenerateFromMap(c.campaign, c.mapID, kind, "Uma sala")); got.GetName() != want {
			t.Errorf("default name of %v = %q, want %q", kind, got.GetName(), want)
		}
	}
	mapNamed := wantDone(t, "map, named", m.mustGenerateFromMap(c.campaign, c.mapID, kindIsometricAPI, "Uma sala", func(r *mapsv1.GenerateMapImageRequest) { r.Name = "A cripta vista de cima" }))
	if mapNamed.GetName() != "A cripta vista de cima" {
		t.Errorf("the master's name of a map picture = %q", mapNamed.GetName())
	}
	edited, err := m.editImage(c.campaign, text.GetId(), "mais escura")
	if err != nil {
		t.Fatalf("EditGeneratedImage() error = %v", err)
	}
	if got := wantDone(t, "edit", edited); got.GetName() != text.GetName()+" (ajuste)" {
		t.Errorf("default name of an edit = %q, want %q", got.GetName(), text.GetName()+" (ajuste)")
	}
	renamed, err := m.editImage(c.campaign, text.GetId(), "mais clara", func(r *mapsv1.EditGeneratedImageRequest) { r.Name = "A taverna, clara" })
	if err != nil {
		t.Fatalf("EditGeneratedImage(name) error = %v", err)
	}
	if got := wantDone(t, "edit, named", renamed); got.GetName() != "A taverna, clara" {
		t.Errorf("the master's name of an edit = %q", got.GetName())
	}
	// A hidden map's name never names its picture either: the textured map of a hidden map (which
	// the players don't see) is named by its way and day.
	m.setMapRevealed(c.campaign, c.mapID, false)
	hidden := wantDone(t, "hidden map", m.mustGenerateFromMap(c.campaign, c.mapID, kindTexturedAPI, "Uma sala"))
	if !strings.HasPrefix(hidden.GetName(), "Mapa com textura · ") || strings.Contains(hidden.GetName(), "caverna") {
		t.Errorf("default name of a hidden map's picture = %q, want \"Mapa com textura · dd/mm\"", hidden.GetName())
	}
	m.setMapRevealed(c.campaign, c.mapID, true)
	// A name with a control character is refused, and no slot moves.
	used := m.imageStatus(c.campaign).GetUsedThisMonth()
	_, err = m.imagegen.GenerateSceneImage(t.Context(), connect.NewRequest(&mapsv1.GenerateSceneImageRequest{
		CampaignId: c.campaign, IdempotencyKey: nextKey(), Prompt: "Uma taverna", Name: "ruim\x07",
	}))
	wantCode(t, "a name with a control character", err, connect.CodeInvalidArgument)
	if now := m.imageStatus(c.campaign).GetUsedThisMonth(); now != used {
		t.Errorf("the refusal used a slot: %d -> %d", used, now)
	}
}

// The portrait of an NPC the players do not see is refused in the references too, whichever list it comes in.
func TestMR039_AHiddenNPCsPortraitIsRefusedAsAnObject(t *testing.T) {
	t.Parallel()
	c := newCave(t, withFake(&gen.Fake{}, 20))
	m := c.master
	hiddenPortrait := m.mustUpload(c.campaign, "emboscado.png", pngImage(t, 30, 40)).GetId()
	hidden := m.createNPC(c.campaign, "Emboscado Três", hiddenPortrait)
	m.placeAt(c.campaign, c.mapID, hidden.GetId(), grid.Square{Col: 8, Row: 8}) // a token starts hidden
	plain := m.mustUpload(c.campaign, "bardo.png", pngImage(t, 33, 40)).GetId()

	for _, kind := range []mapsv1.ImageGenerationKind{kindMapSceneAPI, kindIsometricAPI} {
		_, err := m.generateFromMap(c.campaign, c.mapID, kind, "Uma sala", func(r *mapsv1.GenerateMapImageRequest) { r.ObjectImageIds = []string{plain, hiddenPortrait} })
		wantInvalidField(t, "a hidden NPC's portrait as an object", err, "object_image_ids")
	}
	if used := m.imageStatus(c.campaign).GetUsedThisMonth(); used != 0 {
		t.Errorf("the refusals used %d slots", used)
	}
	wantDone(t, "an image that is no portrait, as an object", m.mustGenerateFromMap(c.campaign, c.mapID, kindMapSceneAPI, "Uma sala", func(r *mapsv1.GenerateMapImageRequest) { r.ObjectImageIds = []string{plain} }))
}
