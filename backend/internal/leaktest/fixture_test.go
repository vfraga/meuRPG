package leaktest

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"runtime"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/maps"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// must returns a call's message, or stops the build with the error and the line
// of the step that failed. The fixture is long: a failing step must say where.
func must[T any](res *connect.Response[T], err error) *T {
	if err != nil {
		_, file, line, _ := runtime.Caller(1)
		panic(fmt.Sprintf("%s:%d: %v", file, line, err))
	}
	return res.Msg
}

func rq[T any](m *T) *connect.Request[T] { return connect.NewRequest(m) }

// world is the hidden-data fixture: one campaign in which the master has made one
// of every kind of hidden thing, a session is open, a combat is running, a scene is
// open and puzzles are shown. Every free-text field of every hidden thing carries
// a marker, and the secret numbers and the ids are registered (see canary_test.go).
type world struct {
	*stack
	secrets *secrets

	// The people. pending waits for the master's approval; stranger is no member.
	master, ana, caio, pending, stranger *person
	campaign                             string
	// The stored spells the rules refuse (buildTableContent): the live one and the one the master retired, with their names.
	leftOutKey, retiredLeftOutKey, retiredLeftOutName string
	session                                           string

	// The characters.
	pens, toren  *charactersv1.Character // Ana's and Caio's player characters
	reserved     *charactersv1.Character // a character the master made for a player to claim: nobody's
	claimToken   string                  // its claim link's secret, shown once to the master
	pendingHero  *charactersv1.Character // the character that waits for approval
	reviewReason string                  // what the master asked her to change
	boss         *charactersv1.Character // the NPC whose numbers are secret
	hiddenNPC    *charactersv1.Character // an NPC whose token the master hid
	bandit       *charactersv1.Character // an NPC made from a bestiary creature
	merchant     *charactersv1.Character // an NPC on the stage
	casterNPC    *charactersv1.Character // an NPC with a full sheet, not on the stage, that cast a spell
	healer       *charactersv1.Character // a Life cleric NPC on the stage that healed Caio's character
	disciple     int64                   // the Disciple of Life extra hit points the healer gave Caio's character
	healCast     *playv1.OutsideCast     // that cast, as the master reads it
	hiddenCast   string                  // the id of that cast
	offstage     *charactersv1.Character // an NPC with a portrait, not on the stage
	seenNPC      *charactersv1.Character // an NPC the master left visible, in Ana's sight
	stage2       *charactersv1.Character // an NPC the stream test puts on the stage
	// The hidden NPC on the open scene's stage (its portrait is imgHiddenStage), and the hidden action of that scene.
	hiddenStageNPC *charactersv1.Character
	hiddenAction   string

	// The gallery.
	imgSceneA, imgSceneB, imgMap, imgUnshown, imgShown, imgLeft, imgPortrait, imgStage, imgHiddenStage, imgGenerated, generation string
	// The scene's conversation (MR-051, RN-36): a picture on a line for Ana alone, and the lines
	// the audio test asks for.
	imgLine string
	lines   []conversationLine
	// ids of things the probe aims at
	imgHiddenMap, imgDungeon                     string
	streamSeen                                   map[string]map[string]int // the kinds of event the stream test received
	clueUnrevealed, creature, milestoneUnreached string
	map2, map2Image, milestoneStream             string            // what the stream test changes
	notes                                        map[string]string // each player's note id, by name
	// The maps and what is on them.
	fogMap, hiddenMap, dungeonMap string
	pts                           map[string]*mapsv1.MapPoint // by role
	// The combat, as the master reads it.
	encounter *playv1.Encounter
	// Puzzles by role.
	puzzles map[string]*playv1.Puzzle
}

// secret numbers of the master's data, unlikely to turn up by chance.
const (
	bossHP       = 4321
	bossAC       = 31
	bossXP       = 8765
	treasurePO   = 7777
	trapNotice   = 27
	trapFind     = 29
	trapSave     = 26
	sceneDC      = 28
	hiddenActDC  = 30 // the DC of the scene's hidden action: no other number of the fixture is 30
	hintDC       = 25
	escapeDC     = 27 // the fixed escape DC of a grapple by a hidden NPC's attack
	groupCheckDC = 23
	caveColumns  = 24
	dungeonSeed  = 8675309
	treasureSeed = 424242
)

var caveWalls = []string{
	"########################",
	"########################",
	"################.......#",
	"################.......#",
	"################....q..#",
	"################.......#",
	"#......#########.......#",
	"...................h...#",
	"...................h...#",
	"#...::.#..######.......#",
	"#...::.#..##############",
	"######........##########",
	"######........##########",
	"######........##########",
	"######........##########",
	"########################",
}

var caveGrid = grid.Grid{Columns: caveColumns, Rows: 16}

// at is the center of a square, in basis points.
func at(col, row int) (x, y int32) {
	xBP, yBP := caveGrid.CenterOf(grid.Square{Col: col, Row: row})
	return int32(xBP), int32(yBP) //nolint:gosec // G115: at most 10000
}

// pngOf is a w x h picture with a pattern, so two uploads differ.
func pngOf(w, h int, seed uint8) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x*3) + seed, G: uint8(y*5) ^ seed, B: uint8(x+y) * 2, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// upload sends a picture to the campaign's gallery as p and returns its id.
func (p *person) upload(campaignID, fileName string, content []byte) string {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	f, _ := w.CreateFormField("campaign_id")
	_, _ = f.Write([]byte(campaignID))
	part, _ := w.CreateFormFile("file", fileName)
	_, _ = part.Write(content)
	_ = w.Close()
	req, err := http.NewRequestWithContext(p.stack.t.Context(), http.MethodPost, p.stack.server.URL+maps.UploadPath, &body)
	if err != nil {
		panic(err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	res, err := p.client.Do(req)
	if err != nil {
		panic(err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusCreated {
		panic(fmt.Sprintf("upload %s: status %d, body %s", fileName, res.StatusCode, raw))
	}
	var img mapsv1.GalleryImage
	if err := protojsonUnmarshal(raw, &img); err != nil {
		panic(err)
	}
	return img.GetId()
}

// image uploads a picture and names it in the gallery with a marker (the name is
// the master's: a player never reads it).
func (w *world) image(kind string, w1, h1 int, readers ...*person) string {
	m := w.master
	id := m.upload(w.campaign, "img.png", pngOf(w1, h1, uint8(len(w.secrets.list)))) //nolint:gosec // G115: a small count
	name := w.secrets.marker(kind+"-gallery-name", readers...)
	must(m.gallery.RenameGalleryImage(w.t.Context(), rq(&mapsv1.RenameGalleryImageRequest{CampaignId: w.campaign, ImageId: id, Name: name})))
	return id
}

func (w *world) fullSheet(class, race string, level int32) *charactersv1.CharacterSheet {
	return &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
		BaseScores: &rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 8},
		RaceKey:    race,
		Classes:    []*charactersv1.ClassLevel{{ClassKey: class, Level: level}},
	}}}
}

func (w *world) pc(owner *person, name, race string) *charactersv1.Character {
	return must(owner.characters.CreateCharacter(w.t.Context(), rq(&charactersv1.CreateCharacterRequest{
		CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: name, Sheet: w.fullSheet("class:wizard", race, 1),
	}))).GetCharacter()
}

// npc makes an NPC as the master. Its name and description are markers.
func (w *world) npc(kind string, sheet *charactersv1.BasicSheet) (*charactersv1.Character, string) {
	name := w.secrets.marker(kind + "-name")
	sheet.Description = w.secrets.marker(kind + "-description")
	ch := must(w.master.characters.CreateCharacter(w.t.Context(), rq(&charactersv1.CreateCharacterRequest{
		CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_MINION, Name: name,
		Sheet: &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Basic{Basic: sheet}},
	}))).GetCharacter()
	return ch, name
}

func (w *world) point(mapID string, kind mapsv1.MapPointKind, name, description string, col, row int, edit func(*mapsv1.CreateMapPointRequest)) *mapsv1.MapPoint {
	x, y := at(col, row)
	req := &mapsv1.CreateMapPointRequest{CampaignId: w.campaign, MapId: mapID, Kind: kind, Name: name, Description: description, XBp: x, YBp: y}
	if edit != nil {
		edit(req)
	}
	return must(w.master.maps.CreateMapPoint(w.t.Context(), rq(req))).GetPoint()
}

func (w *world) reveal(p *mapsv1.MapPoint) {
	must(w.master.maps.SetMapPointRevealed(w.t.Context(), rq(&mapsv1.SetMapPointRevealedRequest{CampaignId: w.campaign, MapId: p.GetMapId(), PointId: p.GetId(), Revealed: true})))
}

func (w *world) place(mapID, characterID string, col, row int) {
	x, y := at(col, row)
	must(w.master.maps.PlaceMapToken(w.t.Context(), rq(&mapsv1.PlaceMapTokenRequest{CampaignId: w.campaign, MapId: mapID, CharacterId: characterID, XBp: x, YBp: y})))
}

func (w *world) paint(mapID string, layer mapsv1.MapLayer, value int32, squares [][2]int32) {
	var sq []*mapsv1.MapSquare
	for _, s := range squares {
		sq = append(sq, &mapsv1.MapSquare{Col: s[0], Row: s[1]})
	}
	for len(sq) > 0 { // at most 400 squares a call
		n := min(len(sq), 400)
		must(w.master.maps.PaintMapCells(w.t.Context(), rq(&mapsv1.PaintMapCellsRequest{CampaignId: w.campaign, MapId: mapID, Layer: layer, Value: value, Squares: sq[:n]})))
		sq = sq[n:]
	}
}

// join lets people into the campaign through an invite from the master; pending
// makes them wait for approval (RN-15).
func (w *world) join(pending bool, people ...*person) {
	inv := must(w.master.campaigns.CreateInvite(w.t.Context(), rq(&campaignsv1.CreateInviteRequest{CampaignId: w.campaign, MaxUses: 20, RequiresApproval: pending})))
	for _, p := range people {
		must(p.campaigns.AcceptInvite(w.t.Context(), rq(&campaignsv1.AcceptInviteRequest{Token: inv.GetToken()})))
	}
	w.secrets.add(&canary{needle: inv.GetToken(), kind: "invite-token"})
}

// revisionOf is the revision of a character's vitals as p reads them in the live session: what the
// caller of AdjustCharacterVitals has to send as expected_revision.
func (p *person) revisionOf(w *world, characterID string) *int32 {
	w.t.Helper()
	for _, v := range must(p.play.GetLiveSession(w.t.Context(), rq(&playv1.GetLiveSessionRequest{CampaignId: w.campaign}))).GetVitals() {
		if v.GetCharacterId() == characterID {
			return proto.Int32(v.GetRevision())
		}
	}
	w.t.Fatalf("no vitals for character %s", characterID)
	return nil
}
