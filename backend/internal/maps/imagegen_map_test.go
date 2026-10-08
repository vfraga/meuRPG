package maps

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/maps/images/gen"
	"github.com/PuraFome/meuRPG/backend/internal/maps/refimg"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// MR-039, RN-28, RN-10 (slice 10.8b): pictures made from a map. The tests use the
// fake generator (package gen), so they need no key and no network. The cave of
// fog_test.go is the table: four player characters, Goblin 2 in the corridor's light
// (the one NPC the party sees), Goblin 1 and the captain in the guard room (not
// seen) and a goblin the master hid.

const (
	kindMapSceneAPI  = mapsv1.ImageGenerationKind_IMAGE_GENERATION_KIND_MAP_SCENE
	kindIsometricAPI = mapsv1.ImageGenerationKind_IMAGE_GENERATION_KIND_ISOMETRIC
	kindTexturedAPI  = mapsv1.ImageGenerationKind_IMAGE_GENERATION_KIND_TEXTURED_MAP
)

func (u *user) mapReference(campaign, mapID string, kind mapsv1.ImageGenerationKind) (*mapsv1.GetMapImageReferenceResponse, error) {
	u.h.t.Helper()
	res, err := u.imagegen.GetMapImageReference(u.h.t.Context(), connect.NewRequest(&mapsv1.GetMapImageReferenceRequest{CampaignId: campaign, MapId: mapID, Kind: kind}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func (u *user) mustMapReference(campaign, mapID string, kind mapsv1.ImageGenerationKind) *mapsv1.GetMapImageReferenceResponse {
	u.h.t.Helper()
	res, err := u.mapReference(campaign, mapID, kind)
	if err != nil {
		u.h.t.Fatalf("GetMapImageReference(%v) error = %v", kind, err)
	}
	return res
}

// generateFromMap asks for a picture made from a map; edit changes the request.
func (u *user) generateFromMap(campaign, mapID string, kind mapsv1.ImageGenerationKind, prompt string, edit ...func(*mapsv1.GenerateMapImageRequest)) (*mapsv1.GenerateMapImageResponse, error) {
	u.h.t.Helper()
	req := &mapsv1.GenerateMapImageRequest{CampaignId: campaign, MapId: mapID, Kind: kind, IdempotencyKey: nextKey(), Prompt: prompt}
	for _, e := range edit {
		e(req)
	}
	res, err := u.imagegen.GenerateMapImage(u.h.t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

// mustGenerateFromMap asks for a picture made from a map and waits for the end.
func (u *user) mustGenerateFromMap(campaign, mapID string, kind mapsv1.ImageGenerationKind, prompt string, edit ...func(*mapsv1.GenerateMapImageRequest)) *mapsv1.GetImageGenerationResponse {
	u.h.t.Helper()
	res, err := u.generateFromMap(campaign, mapID, kind, prompt, edit...)
	if err != nil {
		u.h.t.Fatalf("GenerateMapImage(%v) error = %v", kind, err)
	}
	return u.waitGeneration(campaign, res.GetGeneration().GetId())
}

func (u *user) useAsMapImage(campaign, imageID string) (*mapsv1.UseGeneratedImageAsMapImageResponse, error) {
	u.h.t.Helper()
	res, err := u.imagegen.UseGeneratedImageAsMapImage(u.h.t.Context(), connect.NewRequest(&mapsv1.UseGeneratedImageAsMapImageRequest{CampaignId: campaign, ImageId: imageID}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

// wantDone checks that a request ended with a picture.
func wantDone(t *testing.T, call string, res *mapsv1.GetImageGenerationResponse) *mapsv1.GalleryImage {
	t.Helper()
	if res.GetGeneration().GetState() != mapsv1.ImageGenerationState_IMAGE_GENERATION_STATE_DONE || res.GetImage() == nil {
		t.Fatalf("%s: request = %v, want DONE with an image", call, res.GetGeneration())
	}
	return res.GetImage()
}

// vigia puts an NPC with a portrait on a square the party sees, and returns it with
// the portrait's gallery image.
func (c *cave) vigia(sq grid.Square) (*charactersv1.Character, string) {
	c.h.t.Helper()
	portrait := c.master.mustUpload(c.campaign, "vigia.png", pngImage(c.h.t, 60, 80)).GetId()
	npc := c.master.createNPC(c.campaign, "Vigia", portrait)
	c.master.placeAt(c.campaign, c.mapID, npc.GetId(), sq)
	c.master.setTokenHidden(c.campaign, c.mapID, npc.GetId(), false)
	return npc, portrait
}

// seenByParty is the union of what the four player characters see now, from
// GetMapVision "Ver como" (the master reads each character's own view): the squares in
// a state from "a wall seen" to "bright". What a character only remembers is not in it.
func (c *cave) seenByParty() []bool {
	c.h.t.Helper()
	out := make([]bool, caveGrid.Squares())
	for _, ch := range []*charactersv1.Character{c.pens, c.toren, c.brisa, c.salvia} {
		for i, code := range codes(c.h.t, c.master.mustVision(c.campaign, c.mapID, ch.GetId())) {
			if code >= 1 && code <= 4 {
				out[i] = true
			}
		}
	}
	return out
}

func count(b []bool) int {
	n := 0
	for _, v := range b {
		if v {
			n++
		}
	}
	return n
}

func decodePNG(t *testing.T, data []byte) image.Image {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode a PNG: %v", err)
	}
	return img
}

// hasBlack says whether an image has a pure black pixel: what a square nobody sees is.
func hasBlack(img image.Image) bool {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if r, g, bl, _ := img.At(x, y).RGBA(); r == 0 && g == 0 && bl == 0 {
				return true
			}
		}
	}
	return false
}

// The players' view of the cave: the union of what the party's four characters see
// now (the fog's own computation, none of what they remember), the NPC the party
// sees and none it does not (RN-28, RN-10).
func TestMR039_ThePlayersViewOfTheCave(t *testing.T) {
	t.Parallel()
	c := newCave(t, withFake(&gen.Fake{}, 20))
	ctx := t.Context()
	want := c.seenByParty()

	sub, err := c.h.svc.loadSubject(ctx, c.campaign, c.mapID)
	if err != nil {
		t.Fatal(err)
	}
	seen, err := c.h.svc.playersSeenOf(ctx, c.campaign, sub)
	if err != nil {
		t.Fatal(err)
	}
	if seen.nobody || !slices.Equal(seen.seen, want) {
		t.Errorf("the players' view has %d squares (nobody %v), want the union of the party's views, %d squares", count(seen.seen), seen.nobody, count(want))
	}
	// The union is more than any one of them sees: the corridor, the room and the cave beyond.
	for _, ch := range []*charactersv1.Character{c.pens, c.toren} {
		one := 0
		for _, code := range codes(t, c.master.mustVision(c.campaign, c.mapID, ch.GetId())) {
			if code >= 1 && code <= 4 {
				one++
			}
		}
		if one >= count(want) {
			t.Errorf("one character sees %d squares, the union %d: the union adds nothing", one, count(want))
		}
	}
	// The creatures: the four players' characters, and Goblin 2, whom the party sees.
	// Not Goblin 1 or the captain (in the guard room, out of sight) and not the goblin
	// the master hid, though it stands on a square the party sees.
	var names []string
	for _, n := range seen.npcs {
		names = append(names, n.name)
	}
	if !slices.Equal(names, []string{"Goblin 2"}) {
		t.Errorf("the NPCs the players see = %v, want only Goblin 2", names)
	}
	if len(seen.markers) != 5 {
		t.Errorf("markers = %d, want the four characters and Goblin 2", len(seen.markers))
	}
	for _, sq := range []grid.Square{sqGoblin1, sqCaptain, {Col: 20, Row: 8}} {
		if want[sq.Row*24+sq.Col] && sq != (grid.Square{Col: 20, Row: 8}) {
			t.Errorf("the square %v is seen: the fixture changed", sq)
		}
	}

	// Through the RPC: the same numbers, and a small PNG with unseen squares in black.
	res := c.master.mustMapReference(c.campaign, c.mapID, kindMapSceneAPI)
	if int(res.GetSeenSquares()) != count(want) || res.GetTotalSquares() != 24*16 || !res.GetPlayersSeeSomething() {
		t.Errorf("reference: %d of %d squares, sees something %v; want %d of 384", res.GetSeenSquares(), res.GetTotalSquares(), res.GetPlayersSeeSomething(), count(want))
	}
	img := decodePNG(t, res.GetPreview())
	if b := img.Bounds(); b.Dx() > previewSide || b.Dy() > previewSide {
		t.Errorf("the preview is %dx%d, want at most %d on a side", b.Dx(), b.Dy(), previewSide)
	}
	if !hasBlack(img) {
		t.Error("the preview has no black: the squares nobody sees are not hidden")
	}
	if len(res.GetCreatures()) != 1 || res.GetCreatures()[0].GetName() != "Goblin 2" || res.GetCreatures()[0].GetCharacterId() != c.goblin2.GetId() {
		t.Errorf("creatures = %v, want only Goblin 2", res.GetCreatures())
	}
}

// An unrevealed secret door is a wall in what the model gets; every other door is
// floor (the app draws a door over it).
func TestMR039_ASecretDoorIsAWallInThePlayersView(t *testing.T) {
	t.Parallel()
	c := newCave(t, withFake(&gen.Fake{}, 20))
	m := c.master
	m.mustPaint(c.campaign, c.mapID, doors, int32(mapsv1.DoorState_DOOR_STATE_SECRET), [2]int32{8, 7})
	m.mustPaint(c.campaign, c.mapID, doors, int32(mapsv1.DoorState_DOOR_STATE_LOCKED), [2]int32{9, 7})
	m.mustPaint(c.campaign, c.mapID, doors, int32(mapsv1.DoorState_DOOR_STATE_CLOSED), [2]int32{10, 7})
	m.mustPaint(c.campaign, c.mapID, doors, int32(mapsv1.DoorState_DOOR_STATE_OPEN), [2]int32{11, 7})
	sub, err := c.h.svc.loadSubject(t.Context(), c.campaign, c.mapID)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		col  int
		want bool
		name string
	}{{8, true, "a secret door"}, {9, false, "a locked door"}, {10, false, "a closed door"}, {11, false, "an open door"}, {12, false, "a plain floor square"}} {
		if got := sub.solid[7*24+tc.col]; got != tc.want {
			t.Errorf("%s: solid = %v, want %v", tc.name, got, tc.want)
		}
	}
	if !sub.solid[0] { // a painted wall stays one
		t.Error("a painted wall is not solid")
	}
	// The same plan goes to the model for the whole map (the textured map).
	m.mustGenerateFromMap(c.campaign, c.mapID, kindTexturedAPI, "Uma caverna úmida")
	if got := c.h.fakeCalls(t); len(got) != 1 || got[0].Request.Layout != gen.LayoutTexture {
		t.Fatalf("the model was called %d times", len(got))
	}
}

// fakeCalls reads what the harness's fake generator received.
func (h *harness) fakeCalls(t *testing.T) []gen.Call {
	t.Helper()
	f, ok := h.svc.generator.(*gen.Fake)
	if !ok {
		t.Fatalf("the generator is %T, not a fake", h.svc.generator)
	}
	return f.Calls()
}

// A map without the fog shows the players the whole map, minus what is hidden.
func TestMR039_AMapWithoutFogIsShownWhole(t *testing.T) {
	t.Parallel()
	c := newCave(t, withFake(&gen.Fake{}, 20))
	if _, err := c.master.maps.SetMapFog(t.Context(), connect.NewRequest(&mapsv1.SetMapFogRequest{CampaignId: c.campaign, MapId: c.mapID, FogEnabled: new(false)})); err != nil {
		t.Fatal(err)
	}
	sub, err := c.h.svc.loadSubject(t.Context(), c.campaign, c.mapID)
	if err != nil {
		t.Fatal(err)
	}
	seen, err := c.h.svc.playersSeenOf(t.Context(), c.campaign, sub)
	if err != nil {
		t.Fatal(err)
	}
	if seen.seen != nil || seen.nobody {
		t.Errorf("a map without the fog: seen = %v, nobody = %v, want the whole map", seen.seen != nil, seen.nobody)
	}
	var names []string
	for _, n := range seen.npcs {
		names = append(names, n.name)
	}
	slices.Sort(names)
	if !slices.Equal(names, []string{"Capitão Goblin", "Goblin 1", "Goblin 2"}) {
		t.Errorf("the NPCs without the fog = %v, want the three visible goblins and not the hidden one", names)
	}
	res := c.master.mustMapReference(c.campaign, c.mapID, kindIsometricAPI)
	if res.GetSeenSquares() != res.GetTotalSquares() || !res.GetPlayersSeeSomething() {
		t.Errorf("reference: %d of %d squares", res.GetSeenSquares(), res.GetTotalSquares())
	}
	if hasBlack(decodePNG(t, res.GetPreview())) {
		t.Error("a map without the fog has black squares")
	}
}

// While a combat runs on the map, the NPC tokens are not what the players see (the
// combatants are the combat's own), so no NPC is offered.
func TestMR039_NoNPCIsOfferedWhileACombatRuns(t *testing.T) {
	t.Parallel()
	c := newCave(t, withFake(&gen.Fake{}, 20))
	// A combat on the map: Toren and Goblin 2 fight.
	if _, err := c.master.combat.StartEncounter(t.Context(), connect.NewRequest(&playv1.StartEncounterRequest{
		CampaignId: c.campaign, IdempotencyKey: newKey(), Name: "A guarita",
		Participants: []*playv1.Participant{{CharacterId: c.toren.GetId()}, {CharacterId: c.goblin2.GetId()}},
	})); err != nil {
		t.Fatalf("StartEncounter() error = %v", err)
	}
	res := c.master.mustMapReference(c.campaign, c.mapID, kindMapSceneAPI)
	if len(res.GetCreatures()) != 0 {
		t.Errorf("creatures during a combat = %v, want none", res.GetCreatures())
	}
}

// When no living character of a player stands on the map, the players' view is
// empty: the reference says so and the scene art and the isometric view are refused
// (a refusal that costs no slot); the textured map does not need it.
func TestMR039_NobodyOnTheMap(t *testing.T) {
	t.Parallel()
	c := newCave(t, withFake(&gen.Fake{}, 20))
	m := c.master
	empty := m.createMap(c.campaign, "Salão vazio", m.newImage(c.campaign)).GetId()
	m.mustSetGrid(c.campaign, empty, 8)
	if _, err := m.maps.SetMapFog(t.Context(), connect.NewRequest(&mapsv1.SetMapFogRequest{CampaignId: c.campaign, MapId: empty, FogEnabled: new(true)})); err != nil {
		t.Fatal(err)
	}
	res := m.mustMapReference(c.campaign, empty, kindMapSceneAPI)
	if res.GetPlayersSeeSomething() || res.GetSeenSquares() != 0 {
		t.Errorf("an empty fog map: sees something %v, %d squares", res.GetPlayersSeeSomething(), res.GetSeenSquares())
	}
	for _, kind := range []mapsv1.ImageGenerationKind{kindMapSceneAPI, kindIsometricAPI} {
		_, err := m.generateFromMap(c.campaign, empty, kind, "Uma sala")
		wantGenerationBlocked(t, "GenerateMapImage with nobody on the map", err, mapsv1.ImageGenerationBlockedReason_IMAGE_GENERATION_BLOCKED_REASON_PLAYERS_SEE_NOTHING)
	}
	if got := m.imageStatus(c.campaign).GetUsedThisMonth(); got != 0 {
		t.Errorf("the refusals used %d slots", got)
	}
	m.mustGenerateFromMap(c.campaign, empty, kindTexturedAPI, "Uma sala")
	if got := m.imageStatus(c.campaign).GetUsedThisMonth(); got != 1 {
		t.Errorf("the textured map used %d slots, want 1", got)
	}
}

// What goes to Google for the scene art and the isometric view of a map: the text,
// the style, the players' view as a drawing, and the portraits of the NPCs the
// players see, never a name (RN-28, ADR-0019).
func TestMR039_TheSceneArtAndTheIsometricViewOfAMap(t *testing.T) {
	t.Parallel()
	fake := &gen.Fake{}
	c := newCave(t, withFake(fake, 20))
	m, ana := c.master, c.ana
	vigia, portrait := c.vigia(grid.Square{Col: 7, Row: 7})

	ref := m.mustMapReference(c.campaign, c.mapID, kindMapSceneAPI)
	var offered []string
	for _, cr := range ref.GetCreatures() {
		offered = append(offered, cr.GetName())
		if cr.GetCharacterId() == vigia.GetId() && cr.GetPortraitImageId() != portrait {
			t.Errorf("the portrait offered for Vigia = %q, want %q", cr.GetPortraitImageId(), portrait)
		}
	}
	slices.Sort(offered)
	if !slices.Equal(offered, []string{"Goblin 2", "Vigia"}) {
		t.Fatalf("creatures = %v, want Goblin 2 and Vigia", offered)
	}

	scene := m.mustGenerateFromMap(c.campaign, c.mapID, kindMapSceneAPI, "Uma cripta úmida, tochas apagadas", func(r *mapsv1.GenerateMapImageRequest) {
		r.Style = mapsv1.ImageStyle_IMAGE_STYLE_OIL_PAINTING
		r.NpcCharacterIds = []string{vigia.GetId()}
	})
	img := wantDone(t, "scene art", scene)
	g := scene.GetGeneration()
	if g.GetKind() != kindMapSceneAPI || g.GetMapId() != c.mapID || !slices.Equal(g.GetCharacterImageIds(), []string{portrait}) || g.GetAspectRatio() != mapsv1.ImageAspectRatio_IMAGE_ASPECT_RATIO_16_9 {
		t.Errorf("scene request = %v", g)
	}
	if !img.GetGenerated() || img.GetName() != "A caverna do Vale Seco · arte da cena" {
		t.Errorf("scene image = %v", img)
	}
	iso := m.mustGenerateFromMap(c.campaign, c.mapID, kindIsometricAPI, "A mesma cripta", func(r *mapsv1.GenerateMapImageRequest) {
		r.AspectRatio = mapsv1.ImageAspectRatio_IMAGE_ASPECT_RATIO_4_3
	})
	isoImg := wantDone(t, "isometric view", iso)

	calls := fake.Calls()
	if len(calls) != 2 {
		t.Fatalf("the model was called %d times, want 2", len(calls))
	}
	sceneCall, isoCall := calls[0], calls[1]
	if sceneCall.Request.Layout != gen.LayoutScene || isoCall.Request.Layout != gen.LayoutIsometric {
		t.Errorf("layouts = %q, %q", sceneCall.Request.Layout, isoCall.Request.Layout)
	}
	for _, call := range calls {
		d := call.Request.Drawing
		if d == nil || d.MimeType != "image/png" {
			t.Fatalf("no drawing in the request: %+v", call.Request)
		}
		drawing := decodePNG(t, d.Data)
		if b := drawing.Bounds(); b.Dx() > 1024 || b.Dy() > 1024 {
			t.Errorf("the drawing is %dx%d, want at most 1024 on a side", b.Dx(), b.Dy())
		}
		if !hasBlack(drawing) {
			t.Error("the drawing has no black: the squares nobody sees are in it")
		}
		if len(call.Request.Rooms) != 0 {
			t.Errorf("rooms went with a scene: %v", call.Request.Rooms)
		}
	}
	// The scene art: the portrait of the NPC the master chose, as a character, and
	// only that one; the text and the style as written.
	if refs := sceneCall.Request.References; len(refs) != 1 || !refs[0].Character || len(refs[0].Data) == 0 {
		t.Errorf("scene references = %+v, want one character (Vigia's portrait)", refs)
	}
	if got := sceneCall.Request.Prompt; got != "Uma cripta úmida, tochas apagadas" || sceneCall.Request.Style != "oil painting" || sceneCall.Request.AspectRatio != "16:9" {
		t.Errorf("scene text = %q, style %q, ratio %q", got, sceneCall.Request.Style, sceneCall.Request.AspectRatio)
	}
	if isoCall.Request.AspectRatio != "4:3" || len(isoCall.Request.References) != 0 || !strings.Contains(isoCall.Request.Text(), "isometric") {
		t.Errorf("isometric request = %+v", isoCall.Request)
	}
	// Nothing personal in the text, whoever is at the table, and no name of a creature.
	for _, call := range calls {
		text := inputText(t, call.Body)
		for _, secret := range []string{"Pensantus", "Toren", "Brisa", "Sálvia", "Goblin", "Vigia", "Capitão", "Ana", "Caio", "Bia", "Dani", "Mestre", "@", "example.com", c.campaign, c.mapID, vigia.GetId()} {
			if strings.Contains(text, secret) {
				t.Errorf("the text that goes to the model has %q: %s", secret, text)
			}
		}
		if !strings.Contains(string(call.Body), `"store":false`) {
			t.Errorf("the body does not say store=false")
		}
	}

	// Both are gallery images the players do not see (RN-10).
	for _, i := range []*mapsv1.GalleryImage{img, isoImg} {
		for _, path := range []string{i.GetUrl(), i.GetThumbnailUrl()} {
			if got := ana.get(path); got.status != http.StatusNotFound {
				t.Errorf("player GET %s = %d, want 404 before it is shown", path, got.status)
			}
		}
	}
}

// inputText is the text part of a request body as Gemini would get it.
func inputText(t *testing.T, body []byte) string {
	t.Helper()
	var parsed struct {
		Input []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"input"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("the body is not JSON: %v", err)
	}
	var out []string
	for _, in := range parsed.Input {
		if in.Type == "text" {
			out = append(out, in.Text)
		}
	}
	return strings.Join(out, "\n")
}

// The master may choose only NPCs the players see: any other is refused, and the
// refusal costs no slot. The portraits and the master's own character images share
// the four character places.
func TestMR039_OnlyNPCsThePlayersSeeCanAppear(t *testing.T) {
	t.Parallel()
	fake := &gen.Fake{}
	c := newCave(t, withFake(fake, 20))
	m := c.master
	vigia, portrait := c.vigia(grid.Square{Col: 7, Row: 7})

	for name, id := range map[string]string{
		"an NPC out of sight":         c.goblin1.GetId(),
		"the captain out of sight":    c.captain.GetId(),
		"an NPC the master hid":       c.hiddenGoblin.GetId(),
		"a player's character":        c.pens.GetId(),
		"a character that is no one":  "5b0d9cb3-6f5b-4a0c-8d1b-9c0d8b0f6a11",
		"something that is not an ID": "nope",
	} {
		_, err := m.generateFromMap(c.campaign, c.mapID, kindMapSceneAPI, "Uma sala", func(r *mapsv1.GenerateMapImageRequest) {
			r.NpcCharacterIds = []string{vigia.GetId(), id}
		})
		wantCode(t, "GenerateMapImage with "+name, err, connect.CodeInvalidArgument)
	}
	// An NPC listed twice, and too many characters (four images and an NPC with a portrait).
	_, err := m.generateFromMap(c.campaign, c.mapID, kindMapSceneAPI, "Uma sala", func(r *mapsv1.GenerateMapImageRequest) {
		r.NpcCharacterIds = []string{vigia.GetId(), vigia.GetId()}
	})
	wantCode(t, "an NPC twice", err, connect.CodeInvalidArgument)
	var four []string
	for i := range 4 {
		four = append(four, m.mustUpload(c.campaign, fmt.Sprintf("retrato-%d.png", i), pngImage(t, 30+i, 30)).GetId())
	}
	_, err = m.generateFromMap(c.campaign, c.mapID, kindMapSceneAPI, "Uma sala", func(r *mapsv1.GenerateMapImageRequest) {
		r.CharacterImageIds, r.NpcCharacterIds = four, []string{vigia.GetId()}
	})
	wantCode(t, "five characters", err, connect.CodeInvalidArgument)
	if used := m.imageStatus(c.campaign).GetUsedThisMonth(); used != 0 || len(fake.Calls()) != 0 {
		t.Errorf("the refusals used %d slots and made %d calls", used, len(fake.Calls()))
	}

	// The portrait is counted once when the master also picked it from the gallery.
	done := m.mustGenerateFromMap(c.campaign, c.mapID, kindMapSceneAPI, "Uma sala", func(r *mapsv1.GenerateMapImageRequest) {
		r.CharacterImageIds, r.NpcCharacterIds = []string{portrait, four[0], four[1], four[2]}, []string{vigia.GetId()}
	})
	wantDone(t, "an image chosen twice", done)
	if got := done.GetGeneration().GetCharacterImageIds(); len(got) != 4 || got[0] != portrait {
		t.Errorf("character images = %v, want the four, the portrait once", got)
	}
	// The other kinds are refused as kinds, and the scene kind for a map is not a map's.
	for _, kind := range []mapsv1.ImageGenerationKind{mapsv1.ImageGenerationKind_IMAGE_GENERATION_KIND_UNSPECIFIED, mapsv1.ImageGenerationKind_IMAGE_GENERATION_KIND_SCENE, mapsv1.ImageGenerationKind_IMAGE_GENERATION_KIND_EDIT} {
		_, err := m.generateFromMap(c.campaign, c.mapID, kind, "Uma sala")
		wantCode(t, fmt.Sprintf("GenerateMapImage kind %v", kind), err, connect.CodeInvalidArgument)
		_, err = m.mapReference(c.campaign, c.mapID, kind)
		wantCode(t, fmt.Sprintf("GetMapImageReference kind %v", kind), err, connect.CodeInvalidArgument)
	}
}

// pictureGen returns a picture the test draws from the request, and records what it got.
type pictureGen struct {
	draw  func(req gen.Request) gen.Image
	calls *[]gen.Request
}

func (g pictureGen) Generate(_ context.Context, req gen.Request) (gen.Image, error) {
	*g.calls = append(*g.calls, req)
	return g.draw(req), nil
}

func (pictureGen) Model() string { return "test-picture" }

func withGenerator(g gen.Generator, monthly int32) func(*Config) {
	return func(c *Config) {
		c.Generator = g
		c.MonthlyImages = monthly
	}
}

// encodePNG encodes an image.
func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// The textured map: the whole map's drawing, padded with rock to the model's closest
// ratio; what comes back is cropped to the map and has exactly the map image's size.
func TestMR039_TheTexturedMapIsPaddedAndCroppedBack(t *testing.T) {
	t.Parallel()
	var calls []gen.Request
	// The model answers a 640 x 360 picture (16:9) that is green where the map is
	// on the padded canvas and red everywhere else: after the crop it is all green.
	draw := func(req gen.Request) gen.Image {
		w, h := 640, 360
		// Where the map is on the drawing's canvas, as the server drew it.
		_, pad, err := refimg.RenderPadded(refimg.Plan{Cols: 20, Rows: 10, Solid: make([]bool, 200)}, 200, 100, refimg.Side)
		if err != nil || pad.Ratio != req.AspectRatio {
			t.Errorf("the pad of the drawing = %+v, %v; the request asked for %s", pad, err, req.AspectRatio)
		}
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		inMap := refimg.CropBy(pad.Fractions(), w, h)
		for y := range h {
			for x := range w {
				if (image.Point{X: x, Y: y}).In(inMap) {
					img.Set(x, y, color.RGBA{G: 200, A: 255})
				} else {
					img.Set(x, y, color.RGBA{R: 200, A: 255})
				}
			}
		}
		return gen.Image{MimeType: "image/png", Data: encodePNG(t, img)}
	}
	h := newHarness(t, withGenerator(pictureGen{draw: draw, calls: &calls}, 10))
	master, player := h.newUser("Mestre"), h.newUser("Jogadora")
	campaign := h.newCampaign(master, player)
	m := master.createMap(campaign, "Corredor", master.mustUpload(campaign, "corredor.png", patternImage(t, 200, 100)).GetId())
	master.mustSetGrid(campaign, m.GetId(), 20)

	ref := master.mustMapReference(campaign, m.GetId(), kindTexturedAPI)
	if ref.GetPaddedRatio() != mapsv1.ImageAspectRatio_IMAGE_ASPECT_RATIO_16_9 || ref.GetGridColumns() != 20 || ref.GetGridRows() != 10 {
		t.Errorf("reference: ratio %v, grid %d x %d; want 16:9 and 20 x 10", ref.GetPaddedRatio(), ref.GetGridColumns(), ref.GetGridRows())
	}
	res := master.mustGenerateFromMap(campaign, m.GetId(), kindTexturedAPI, "Um corredor de pedra", func(r *mapsv1.GenerateMapImageRequest) {
		r.AspectRatio = mapsv1.ImageAspectRatio_IMAGE_ASPECT_RATIO_21_9 // ignored: the server picks it
	})
	img := wantDone(t, "textured map", res)
	if len(calls) != 1 || calls[0].AspectRatio != "16:9" || calls[0].Layout != gen.LayoutTexture || len(calls[0].Rooms) != 0 {
		t.Fatalf("the model got %+v", calls)
	}
	if res.GetGeneration().GetAspectRatio() != mapsv1.ImageAspectRatio_IMAGE_ASPECT_RATIO_16_9 {
		t.Errorf("the request's ratio = %v, want 16:9", res.GetGeneration().GetAspectRatio())
	}
	drawing := decodePNG(t, calls[0].Drawing.Data)
	if b := drawing.Bounds(); float64(b.Dx())/float64(b.Dy()) < 1.76 || float64(b.Dx())/float64(b.Dy()) > 1.79 {
		t.Errorf("the drawing is %dx%d, want the ratio 16:9", b.Dx(), b.Dy())
	}
	if hasBlack(drawing) {
		t.Error("the whole map's drawing has black in it")
	}
	// Stored at the map image's own size, as a JPEG, and (nearly) all green: the rock
	// the padding added is gone.
	if img.GetWidth() != 200 || img.GetHeight() != 100 || img.GetContentType() != "image/jpeg" {
		t.Fatalf("the textured map is %dx%d %s, want 200x100 image/jpeg", img.GetWidth(), img.GetHeight(), img.GetContentType())
	}
	file := master.get(img.GetUrl())
	got, err := jpeg.Decode(bytes.NewReader(file.body))
	if err != nil {
		t.Fatal(err)
	}
	notGreen := 0
	for y := range 100 {
		for x := range 200 {
			r, g, _, _ := got.At(x, y).RGBA()
			if g>>8 < 120 || r>>8 > 100 {
				notGreen++
			}
		}
	}
	if notGreen > 60 { // JPEG blurs the edge a little
		t.Errorf("%d of 20000 pixels are not the map's: the crop-back is off", notGreen)
	}
	if got := player.get(img.GetUrl()); got.status != http.StatusNotFound {
		t.Errorf("player GET of the textured map = %d, want 404 before it is used", got.status)
	}
}

// The textured map of a generated dungeon sends its rooms (the name and the size of
// each, nothing else), and only that kind does; a secret door is a wall in the
// drawing of every kind.
func TestMR039_ADungeonsTexturedMapSendsItsRooms(t *testing.T) {
	t.Parallel()
	fake := &gen.Fake{}
	d := newDungeonTable(t, withFake(fake, 20))
	seed, _ := testDungeonSeed(t)
	made := d.master.createDungeon(d.campaign, "Masmorra de Mirathel", testDungeonOptions(), seed)
	mapID := made.GetMap().GetId()
	rooms, err := d.master.dungeonRooms(d.campaign, mapID)
	if err != nil {
		t.Fatal(err)
	}
	g := grid.Grid{Columns: int(rooms.GetWidth()), Rows: int(rooms.GetHeight())}
	room := rooms.GetRooms()[0]
	x, y := g.CenterOf(grid.Square{Col: int(room.GetCenterCol()), Row: int(room.GetCenterRow())})
	d.master.placeToken(d.campaign, mapID, d.pens, int32(x), int32(y)) //nolint:gosec // G115: 0 to 10000

	// The drawing: a secret door is solid, every other door is floor.
	sub, err := d.h.svc.loadSubject(t.Context(), d.campaign, mapID)
	if err != nil {
		t.Fatal(err)
	}
	secret, other := 0, 0
	for _, dr := range rooms.GetDoors() {
		solid := sub.solid[int(dr.GetY())*g.Columns+int(dr.GetX())]
		switch dr.GetKind() {
		case mapsv1.DungeonDoorKind_DUNGEON_DOOR_KIND_SECRET:
			secret++
			if !solid {
				t.Errorf("the secret door at (%d,%d) is floor in the drawing", dr.GetX(), dr.GetY())
			}
		default:
			other++
			if solid {
				t.Errorf("a door (%v) at (%d,%d) is a wall in the drawing", dr.GetKind(), dr.GetX(), dr.GetY())
			}
		}
	}
	if secret == 0 || other == 0 {
		t.Fatalf("the fixture has %d secret doors and %d others", secret, other)
	}

	d.master.mustGenerateFromMap(d.campaign, mapID, kindTexturedAPI, "Uma masmorra úmida")
	d.master.mustGenerateFromMap(d.campaign, mapID, kindIsometricAPI, "Uma masmorra úmida")
	calls := fake.Calls()
	if len(calls) != 2 {
		t.Fatalf("the model was called %d times", len(calls))
	}
	textured, isometric := calls[0].Request, calls[1].Request
	if len(textured.Rooms) != len(rooms.GetRooms()) {
		t.Fatalf("rooms sent = %d, want %d", len(textured.Rooms), len(rooms.GetRooms()))
	}
	line := regexp.MustCompile(`^Sala \d+: \d+ x \d+ squares$`)
	for i, r := range textured.Rooms {
		want := fmt.Sprintf("Sala %d: %d x %d squares", rooms.GetRooms()[i].GetId(), rooms.GetRooms()[i].GetFloor().GetWidth(), rooms.GetRooms()[i].GetFloor().GetHeight())
		if !line.MatchString(r) || r != want {
			t.Errorf("room line %d = %q, want %q", i, r, want)
		}
	}
	if len(isometric.Rooms) != 0 || strings.Contains(inputText(t, calls[1].Body), "Sala") {
		t.Errorf("the rooms went with the isometric view: %v", isometric.Rooms)
	}
	if !strings.Contains(inputText(t, calls[0].Body), "Sala 1:") {
		t.Errorf("the textured map's text lacks the rooms: %s", inputText(t, calls[0].Body))
	}
	// The isometric view starts from what the player sees, not the whole dungeon.
	ref := d.master.mustMapReference(d.campaign, mapID, kindIsometricAPI)
	if ref.GetSeenSquares() <= 0 || ref.GetSeenSquares() >= ref.GetTotalSquares() {
		t.Errorf("the isometric view shows %d of %d squares, want a part", ref.GetSeenSquares(), ref.GetTotalSquares())
	}
	if ref := d.master.mustMapReference(d.campaign, mapID, kindTexturedAPI); ref.GetRoomsListed() != int32(len(rooms.GetRooms())) { //nolint:gosec // G115: a few rooms
		t.Errorf("rooms listed = %d, want %d", ref.GetRoomsListed(), len(rooms.GetRooms()))
	}
}

// "Usar como imagem do mapa": the textured map becomes the map's image, with the
// same size, grid, calibration, painted layers, points, tokens and what the players
// remember; the old image stays in the gallery.
func TestMR039_UseTheTexturedMapAsTheMapImage(t *testing.T) {
	t.Parallel()
	c := newCave(t, withFake(&gen.Fake{}, 20))
	m, ana := c.master, c.ana
	imageBefore := c.imageID
	wallsBefore := m.mustLayers(c.campaign, c.mapID)
	memoryBefore := codes(t, ana.mustVision(c.campaign, c.mapID))
	before := m.mustGetMap(c.campaign, c.mapID)

	made := m.mustGenerateFromMap(c.campaign, c.mapID, kindTexturedAPI, "Uma caverna úmida")
	texture := wantDone(t, "textured map", made)
	if texture.GetWidth() != 240 || texture.GetHeight() != 160 {
		t.Fatalf("the textured map is %dx%d, want the cave's 240x160", texture.GetWidth(), texture.GetHeight())
	}
	// Not used yet: the map still has its own image.
	if got := m.getMapImage(c.campaign, c.mapID); got != imageBefore {
		t.Errorf("the map's image changed before the master asked: %s", got)
	}

	used, err := m.useAsMapImage(c.campaign, texture.GetId())
	if err != nil {
		t.Fatalf("UseGeneratedImageAsMapImage() error = %v", err)
	}
	after := used.GetMap()
	if after.GetImage().GetId() != texture.GetId() || after.GetRevision() <= before.GetMap().GetRevision() {
		t.Errorf("the map's image = %s (revision %d), want %s with a newer revision", after.GetImage().GetId(), after.GetRevision(), texture.GetId())
	}
	if after.GetGridColumns() != before.GetMap().GetGridColumns() || after.GetSquareFactor() != before.GetMap().GetSquareFactor() || after.GetGridRows() != before.GetMap().GetGridRows() {
		t.Errorf("the grid changed: %v", after)
	}
	// The layers, the players' memory, the points and the tokens are kept.
	wallsAfter := m.mustLayers(c.campaign, c.mapID)
	if !bytes.Equal(wallsAfter.GetWall(), wallsBefore.GetWall()) || !bytes.Equal(wallsAfter.GetDifficultTerrain(), wallsBefore.GetDifficultTerrain()) ||
		!bytes.Equal(wallsAfter.GetCover(), wallsBefore.GetCover()) || !bytes.Equal(wallsAfter.GetLight(), wallsBefore.GetLight()) {
		t.Error("the painted layers changed")
	}
	if got := codes(t, ana.mustVision(c.campaign, c.mapID)); !bytes.Equal(got, memoryBefore) {
		t.Error("what Ana sees and remembers changed")
	}
	again := m.mustGetMap(c.campaign, c.mapID)
	if len(again.GetPoints()) != len(before.GetPoints()) || len(again.GetTokens()) != len(before.GetTokens()) {
		t.Errorf("points %d -> %d, tokens %d -> %d", len(before.GetPoints()), len(again.GetPoints()), len(before.GetTokens()), len(again.GetTokens()))
	}
	// The old image is still in the gallery, and a retry changes nothing.
	if got := m.get("/images/" + imageBefore); got.status != http.StatusOK {
		t.Errorf("the old image: GET = %d, want it kept", got.status)
	}
	if res, err := m.useAsMapImage(c.campaign, texture.GetId()); err != nil || res.GetMap().GetRevision() != after.GetRevision() {
		t.Errorf("using it twice: %v, %v", res, err)
	}
	// What a player gets of a fog map still has no image (RN-10).
	for name, msg := range map[string]proto.Message{"GetMap": ana.mustGetMap(c.campaign, c.mapID), "GetMapVision": ana.mustVision(c.campaign, c.mapID)} {
		if json := asJSON(t, msg); strings.Contains(json, texture.GetId()) || strings.Contains(json, imageBefore) {
			t.Errorf("Ana's %s has a map image id: %s", name, json)
		}
	}
	if got := ana.get("/images/" + texture.GetId()); got.status != http.StatusNotFound {
		t.Errorf("Ana GET of the fog map's new image = %d, want 404", got.status)
	}
}

// The textured map is refused once the map is not what it was made from: another
// image, a new grid or another calibration. And only a textured map of a map of the
// campaign can be used.
func TestMR039_UseIsRefusedWhenTheMapChanged(t *testing.T) {
	t.Parallel()
	c := newCave(t, withFake(&gen.Fake{}, 20))
	m := c.master
	blocked := mapsv1.ImageGenerationBlockedReason_IMAGE_GENERATION_BLOCKED_REASON_MAP_CHANGED
	texture := func() string {
		return wantDone(t, "textured map", m.mustGenerateFromMap(c.campaign, c.mapID, kindTexturedAPI, "Uma caverna")).GetId()
	}

	// Another image on the map.
	first := texture()
	current := m.mustGetMap(c.campaign, c.mapID).GetMap()
	other := m.mustUpload(c.campaign, "outra.png", patternImage(t, 240, 160)).GetId()
	if _, err := m.maps.UpdateMap(t.Context(), connect.NewRequest(&mapsv1.UpdateMapRequest{CampaignId: c.campaign, MapId: c.mapID, Revision: current.GetRevision(), ImageId: new(other)})); err != nil {
		t.Fatalf("UpdateMap(image) error = %v", err)
	}
	_, err := m.useAsMapImage(c.campaign, first)
	wantGenerationBlocked(t, "Use after another image", err, blocked)
	if got := m.getMapImage(c.campaign, c.mapID); got == first {
		t.Error("the textured map was used on a map that changed")
	}

	// A new grid (another calibration of the same image: 12 columns instead of 24).
	second := texture()
	m.mustSetGrid(c.campaign, c.mapID, 12)
	_, err = m.useAsMapImage(c.campaign, second)
	wantGenerationBlocked(t, "Use after another grid", err, blocked)

	// Another calibration: the same columns, a different factor.
	third := texture()
	m.mustSetCalibration(c.campaign, c.mapID, 12, 2)
	_, err = m.useAsMapImage(c.campaign, third)
	wantGenerationBlocked(t, "Use after another calibration", err, blocked)

	// Not a textured map, an image of another campaign, and a player.
	scene := m.mustGenerate(c.campaign, "Uma cena").GetImage().GetId()
	_, err = m.useAsMapImage(c.campaign, scene)
	wantCode(t, "Use of the scene art", err, connect.CodeNotFound)
	_, err = m.useAsMapImage(c.campaign, other)
	wantCode(t, "Use of an upload", err, connect.CodeNotFound)
	_, err = c.ana.useAsMapImage(c.campaign, second)
	wantCode(t, "Use as a player", err, connect.CodeNotFound)
	_, err = m.useAsMapImage(c.campaign, "nope")
	wantCode(t, "Use of nothing", err, connect.CodeNotFound)
}

// The new RPCs are the master's, like every other of the service.
func TestMR039_TheMapRPCsAreTheMastersOnly(t *testing.T) {
	t.Parallel()
	c := newCave(t, withFake(&gen.Fake{}, 20))
	made := c.master.mustGenerateFromMap(c.campaign, c.mapID, kindTexturedAPI, "Uma caverna")
	img := made.GetImage().GetId()
	calls := map[string]func(u *user) error{
		"GetMapImageReference": func(u *user) error {
			_, err := u.mapReference(c.campaign, c.mapID, kindMapSceneAPI)
			return err
		},
		"GenerateMapImage": func(u *user) error {
			_, err := u.generateFromMap(c.campaign, c.mapID, kindMapSceneAPI, "x")
			return err
		},
		"UseGeneratedImageAsMapImage": func(u *user) error {
			_, err := u.useAsMapImage(c.campaign, img)
			return err
		},
	}
	outsider := c.h.newUser("De fora")
	for name, call := range calls {
		for who, u := range map[string]*user{"a player": c.ana, "a non-member": outsider} {
			wantCode(t, name+" as "+who, call(u), connect.CodeNotFound)
		}
		wantCode(t, name+" without a session", call(c.h.anonymous()), connect.CodeUnauthenticated)
	}
	// A map of another campaign, and a map without a grid.
	other := c.h.newCampaign(c.master)
	foreign := c.master.createMap(other, "De fora", c.master.newImage(other)).GetId()
	_, err := c.master.mapReference(c.campaign, foreign, kindMapSceneAPI)
	wantCode(t, "GetMapImageReference of another campaign's map", err, connect.CodeNotFound)
	_, err = c.master.generateFromMap(c.campaign, foreign, kindMapSceneAPI, "x")
	wantCode(t, "GenerateMapImage of another campaign's map", err, connect.CodeNotFound)
	_, err = c.master.generateFromMap(c.campaign, "nope", kindMapSceneAPI, "x")
	wantCode(t, "GenerateMapImage of nothing", err, connect.CodeNotFound)
	_, err = c.master.generateFromMap(c.campaign, c.probeMap, kindTexturedAPI, "x")
	wantGenerationBlocked(t, "GenerateMapImage of a map without a grid", err, mapsv1.ImageGenerationBlockedReason_IMAGE_GENERATION_BLOCKED_REASON_MAP_HAS_NO_GRID)
	_, err = c.master.mapReference(c.campaign, c.probeMap, kindTexturedAPI)
	wantGenerationBlocked(t, "GetMapImageReference of a map without a grid", err, mapsv1.ImageGenerationBlockedReason_IMAGE_GENERATION_BLOCKED_REASON_MAP_HAS_NO_GRID)
	// The refusals used nothing: the textured map above is the month's only slot.
	if got := c.master.imageStatus(c.campaign).GetUsedThisMonth(); got != 1 {
		t.Errorf("used = %d, want 1", got)
	}
}

// RN-10: nothing the new kinds make or read reaches a player: not the request, the
// pictures, the map's drawing or an NPC they do not see.
func TestRN10_NothingOfTheMapPicturesReachesAPlayer(t *testing.T) {
	t.Parallel()
	c := newCave(t, withFake(&gen.Fake{}, 20))
	m := c.master
	vigia, _ := c.vigia(grid.Square{Col: 7, Row: 7})
	scene := m.mustGenerateFromMap(c.campaign, c.mapID, kindMapSceneAPI, "O segredo da guarita", func(r *mapsv1.GenerateMapImageRequest) { r.NpcCharacterIds = []string{vigia.GetId()} })
	texture := m.mustGenerateFromMap(c.campaign, c.mapID, kindTexturedAPI, "O mapa inteiro")
	sceneImg, textureImg := wantDone(t, "scene", scene), wantDone(t, "texture", texture)
	if _, err := m.useAsMapImage(c.campaign, textureImg.GetId()); err != nil {
		t.Fatal(err)
	}
	for who, u := range map[string]*user{"Ana": c.ana, "Caio": c.caio} {
		for _, i := range []*mapsv1.GalleryImage{sceneImg, textureImg} {
			if got := u.get("/images/" + i.GetId()); got.status != http.StatusNotFound {
				t.Errorf("%s GET /images/%s = %d, want 404", who, i.GetId(), got.status)
			}
		}
		responses := map[string]string{
			"GetMap":       asJSON(t, u.mustGetMap(c.campaign, c.mapID)),
			"ListMaps":     asJSON(t, &mapsv1.ListMapsResponse{Maps: u.listMaps(c.campaign)}),
			"GetMapLayers": asJSON(t, u.mustLayers(c.campaign, c.mapID)),
			"GetMapVision": asJSON(t, u.mustVision(c.campaign, c.mapID)),
		}
		for name, json := range responses {
			for _, secret := range []string{
				sceneImg.GetId(), textureImg.GetId(), scene.GetGeneration().GetId(), texture.GetGeneration().GetId(), sceneImg.GetName(), textureImg.GetName(),
				"O segredo da guarita", "O mapa inteiro", "map_scene", "textured", "idempotency", "Goblin 1", c.goblin1.GetId(), "Capitão", c.captain.GetId(), c.hiddenGoblin.GetId(), "Emboscado",
			} {
				if strings.Contains(json, secret) {
					t.Errorf("%s's %s has %q: %s", who, name, secret, json)
				}
			}
		}
		// The imagegen RPCs of a player never answer, not even for the reference.
		_, err := u.mapReference(c.campaign, c.mapID, kindMapSceneAPI)
		wantCode(t, who+" GetMapImageReference", err, connect.CodeNotFound)
	}
}

// The cap and the long poll still work for the new kinds: the month's slots count
// them, a failure gives the slot back, and GetImageGeneration waits for them.
func TestMR039_TheCapAndTheLongPollForMapKinds(t *testing.T) {
	t.Parallel()
	fake, entered, release := holdingFake()
	c := newCave(t, withFake(fake, 2))
	m := c.master

	first, err := m.generateFromMap(c.campaign, c.mapID, kindMapSceneAPI, "Uma cena")
	if err != nil {
		t.Fatal(err)
	}
	id := first.GetGeneration().GetId()
	if first.GetStatus().GetUsedThisMonth() != 1 || first.GetStatus().GetRemaining() != 1 {
		t.Errorf("after the first: used %d, remaining %d", first.GetStatus().GetUsedThisMonth(), first.GetStatus().GetRemaining())
	}
	<-entered
	// A second request takes the second slot (the model is held, so it waits).
	again, err := m.imagegen.GenerateMapImage(t.Context(), connect.NewRequest(&mapsv1.GenerateMapImageRequest{
		CampaignId: c.campaign, MapId: c.mapID, Kind: kindMapSceneAPI, IdempotencyKey: nextKey(), Prompt: "Outra",
	}))
	if err != nil {
		t.Fatal(err)
	}
	// ...and now the month is full.
	_, err = m.generateFromMap(c.campaign, c.mapID, kindIsometricAPI, "Uma vista")
	blocked := wantGenerationBlocked(t, "the third request", err, mapsv1.ImageGenerationBlockedReason_IMAGE_GENERATION_BLOCKED_REASON_LIMIT_REACHED)
	if blocked.GetStatus().GetRemaining() != 0 {
		t.Errorf("remaining at the limit = %d", blocked.GetStatus().GetRemaining())
	}

	// The long poll: still pending at the deadline, then it answers at the change.
	start := time.Now()
	got, err := m.getGeneration(c.campaign, id, 1)
	if err != nil || got.GetGeneration().GetState() != mapsv1.ImageGenerationState_IMAGE_GENERATION_STATE_PENDING || time.Since(start) < 900*time.Millisecond {
		t.Fatalf("poll at the deadline = %v, %v after %v", got.GetGeneration().GetState(), err, time.Since(start))
	}
	time.AfterFunc(300*time.Millisecond, func() { close(release) })
	got, err = m.getGeneration(c.campaign, id, 25)
	if err != nil {
		t.Fatal(err)
	}
	wantDone(t, "the long poll", got)
	m.waitGeneration(c.campaign, again.Msg.GetGeneration().GetId())

	if status := m.imageStatus(c.campaign); status.GetUsedThisMonth() != 2 {
		t.Fatalf("used = %d, want the two that made pictures", status.GetUsedThisMonth())
	}
}

// A request the model refused or answered without an image gives its slot back, for a map kind too.
func TestMR039_AFailedMapRequestGivesTheSlotBack(t *testing.T) {
	t.Parallel()
	c := newCave(t, withFake(&gen.Fake{}, 3))
	m := c.master
	for _, marker := range []string{gen.MarkerRefuse, gen.MarkerEmpty} {
		res := m.mustGenerateFromMap(c.campaign, c.mapID, kindIsometricAPI, "Uma vista "+marker)
		if res.GetGeneration().GetState() == mapsv1.ImageGenerationState_IMAGE_GENERATION_STATE_DONE || res.GetGeneration().GetSlotSpent() {
			t.Errorf("%s: request = %v, want it failed with the slot back", marker, res.GetGeneration())
		}
	}
	if got := m.imageStatus(c.campaign).GetUsedThisMonth(); got != 0 {
		t.Errorf("used = %d after two refusals, want 0", got)
	}
}

// A map's request of the same key is made once; the picture is stored once.
func TestMR039_TheSameKeyMakesOneMapPicture(t *testing.T) {
	t.Parallel()
	fake := &gen.Fake{}
	c := newCave(t, withFake(fake, 5))
	key := nextKey()
	ask := func() *mapsv1.GenerateMapImageResponse {
		res, err := c.master.imagegen.GenerateMapImage(t.Context(), connect.NewRequest(&mapsv1.GenerateMapImageRequest{
			CampaignId: c.campaign, MapId: c.mapID, Kind: kindTexturedAPI, IdempotencyKey: key, Prompt: "Uma caverna",
		}))
		if err != nil {
			t.Fatal(err)
		}
		return res.Msg
	}
	a, b := ask(), ask()
	if a.GetGeneration().GetId() != b.GetGeneration().GetId() {
		t.Errorf("two tries with one key made two requests")
	}
	c.master.waitGeneration(c.campaign, a.GetGeneration().GetId())
	if len(fake.Calls()) != 1 || c.master.imageStatus(c.campaign).GetUsedThisMonth() != 1 {
		t.Errorf("calls = %d, used = %d, want 1 and 1", len(fake.Calls()), c.master.imageStatus(c.campaign).GetUsedThisMonth())
	}
}

// The textured map refuses the portrait of an NPC with a token on the map, as the scene art
// and the isometric view refuse a hidden one: the picture can become the map the players
// read, and no creature has a reason to be in it (RN-10).
func TestTexturedMapRefusesAnNpcPortrait(t *testing.T) {
	t.Parallel()
	fake := &gen.Fake{}
	c := newCave(t, withFake(fake, 20))
	m := c.master
	portrait := m.mustUpload(c.campaign, "emboscado.png", pngImage(t, 30, 40)).GetId()
	hidden := m.createNPC(c.campaign, "Emboscado Seis", portrait)
	m.placeAt(c.campaign, c.mapID, hidden.GetId(), grid.Square{Col: 8, Row: 8}) // a token starts hidden

	// Control: the isometric view refuses it.
	_, err := m.generateFromMap(c.campaign, c.mapID, kindIsometricAPI, "Uma sala", func(r *mapsv1.GenerateMapImageRequest) { r.ObjectImageIds = []string{portrait} })
	wantInvalidField(t, "isometric with a hidden NPC's portrait", err, "object_image_ids")

	_, err = m.generateFromMap(c.campaign, c.mapID, kindTexturedAPI, "Uma sala", func(r *mapsv1.GenerateMapImageRequest) { r.ObjectImageIds = []string{portrait} })
	if err == nil {
		// Accepted: wait for it and see what the model got.
		done := m.mustGenerateFromMap(c.campaign, c.mapID, kindTexturedAPI, "Uma sala", func(r *mapsv1.GenerateMapImageRequest) { r.ObjectImageIds = []string{portrait} })
		t.Errorf("textured map accepted a hidden NPC's portrait as object_image_ids (state %v)", done.GetGeneration().GetState())
		for _, call := range fake.Calls() {
			t.Logf("model call: layout %q, %d references", call.Request.Layout, len(call.Request.References))
		}
		return
	}
	wantInvalidField(t, "textured map with a hidden NPC's portrait", err, "object_image_ids")
}
