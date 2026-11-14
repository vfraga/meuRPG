package maps

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"maps"
	"slices"
	"uuid"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/encoding/protojson"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/maps/images"
	"github.com/PuraFome/meuRPG/backend/internal/maps/images/gen"
	"github.com/PuraFome/meuRPG/backend/internal/maps/link"
	"github.com/PuraFome/meuRPG/backend/internal/maps/mapsdb"
	"github.com/PuraFome/meuRPG/backend/internal/maps/refimg"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/platform/sessiontell"
	"github.com/PuraFome/meuRPG/backend/internal/platform/uid"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
	"github.com/PuraFome/meuRPG/backend/internal/rules/vision"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
)

// The pictures made from a map (MR-039, RN-28, ADR-0019). Three kinds
// share the image service of imagegen.go (reserve, run, the generator, the cap, the long poll):
//
//   - the scene art and the isometric view start from the players' view of the map:
//     what the players' characters see now (the union of their views, as the fog of
//     war works it out in 9.4, without what they remember), the creatures the
//     players see and an unrevealed secret door as wall;
//   - the textured map starts from the whole map, padded with rock to the model's
//     closest ratio; the picture it returns is cropped back to the map's rectangle
//     and scaled to the size of the map's image, so it can replace it
//     (UseGeneratedImageAsMapImage) with the grid, the layers and the players'
//     memory kept.
//
// What the reference shows is decided here, before anything is drawn, from the
// layers as the master painted them with one change: the drawing never has a door
// (an overlay), and a secret door is a wall (RN-10, RN-26). Hidden tokens, hidden
// points, traps and treasures are never in it: package refimg draws only what the
// Plan holds (floor, wall, seen, a disc per visible creature). The uploaded map
// image is not in it either, since the art itself may show a secret door or a
// hidden creature, which the server cannot know.

// The kinds of a request made from a map, as the table writes them.
const (
	kindMapScene    = "map_scene"
	kindIsometric   = "isometric"
	kindTexturedMap = "textured_map"
)

// isMapKind says whether a stored kind is one made from a map.
func isMapKind(kind string) bool {
	return kind == kindMapScene || kind == kindIsometric || kind == kindTexturedMap
}

// mapKindOf turns the API's kind into the stored one and the model's layout.
func mapKindOf(kind mapsv1.ImageGenerationKind) (stored, layout string, ok bool) {
	switch kind {
	case mapsv1.ImageGenerationKind_IMAGE_GENERATION_KIND_MAP_SCENE:
		return kindMapScene, gen.LayoutScene, true
	case mapsv1.ImageGenerationKind_IMAGE_GENERATION_KIND_ISOMETRIC:
		return kindIsometric, gen.LayoutIsometric, true
	case mapsv1.ImageGenerationKind_IMAGE_GENERATION_KIND_TEXTURED_MAP:
		return kindTexturedMap, gen.LayoutTexture, true
	}
	return "", "", false
}

// layoutOf is the model's layout of a stored map kind.
func layoutOf(stored string) string {
	switch stored {
	case kindMapScene:
		return gen.LayoutScene
	case kindIsometric:
		return gen.LayoutIsometric
	case kindTexturedMap:
		return gen.LayoutTexture
	}
	return ""
}

// maxTexturePixels is the most pixels a map's image may have for a textured map: the
// picture is scaled to the image's size, in a working copy of 4 bytes a pixel.
const maxTexturePixels = images.MaxFitPixels

// errField is `invalid_argument` with the ImageGenerationInvalidField detail: it names the
// request field that breaks a rule, never the value.
func errField(field, message string) error {
	err := connect.NewError(connect.CodeInvalidArgument, errors.New(field+" "+message))
	if detail, detailErr := connect.NewErrorDetail(&mapsv1.ImageGenerationInvalidField{Field: field}); detailErr == nil {
		err.AddDetail(detail)
	}
	return err
}

// planHash is a hash of the floor plan a drawing shows: what a wall painted, or a secret
// door revealed, changes, and a door opened or locked does not.
func planHash(solid []bool) string {
	packed := make([]byte, (len(solid)+7)/8)
	for i, v := range solid {
		if v {
			packed[i/8] |= 1 << (i % 8)
		}
	}
	sum := sha256.Sum256(packed)
	return hex.EncodeToString(sum[:16])
}

// planAt reads a map's floor plan through q (the pool or a transaction) from its
// layers, and the dungeon's record for a generated dungeon (zero for a map that is
// not one), whose rooms the textured map sends.
func (s *Service) planAt(ctx context.Context, q *mapsdb.Queries, campaignID string, row mapsdb.Map, g grid.Grid) (solid []bool, set layerSet, rec mapsdb.GeneratedDungeon, err error) {
	stored, err := q.GetMapLayers(ctx, row.ID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, layerSet{}, rec, fmt.Errorf("read the map's layers: %w", err)
	}
	set = loadLayers(stored, g)
	rec, err = q.GetGeneratedDungeon(ctx, mapsdb.GetGeneratedDungeonParams{CampaignID: campaignID, MapID: row.ID})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		rec = mapsdb.GeneratedDungeon{}
	case err != nil:
		return nil, layerSet{}, rec, fmt.Errorf("read the map's dungeon: %w", err)
	}
	return floorPlan(set, g), set, rec, nil
}

// previewSide is the longer side of the preview GetMapImageReference returns.
const previewSide = 512

// mapRequest is what a handler checked of a request made from a map.
type mapRequest struct {
	mapID  string
	layout string
	npcs   []string // character IDs of the NPCs that appear
}

// mapBasis is the map as a request saw it: UseGeneratedImageAsMapImage needs the
// map to still be this.
type mapBasis struct {
	mapID, imageID          string
	gridColumns, gridFactor int32
	width, height           int32
	planHash                string
	pad                     *refimg.Fractions // the textured map's rectangle on its canvas
}

// subject is a map read for a picture: its row, its image's size, its layers and the
// floor plan they make.
type subject struct {
	row        mapsdb.Map
	g          grid.Grid
	imgW, imgH int32
	set        layerSet
	solid      []bool // the floor plan: wall, rock and a secret door are solid
	rooms      []string
	tokens     []mapsdb.MapToken
}

// mapSubjectError is the typed reason a map cannot be the start of a picture.
type mapSubjectError struct {
	reason mapsv1.ImageGenerationBlockedReason
}

func (e mapSubjectError) Error() string { return "the map cannot be drawn: " + e.reason.String() }

// loadSubject reads a map of the campaign for a picture: `not_found` for any other
// map, mapSubjectError when it has no grid. It reads through the pool (never inside a
// transaction).
func (s *Service) loadSubject(ctx context.Context, campaignID, rawMapID string) (*subject, error) {
	mapID, ok := uid.Canonical(rawMapID)
	if !ok {
		return nil, errMapNotFound()
	}
	row, err := s.campaignMap(ctx, s.queries, campaignID, mapID)
	if err != nil {
		return nil, s.dbError(ctx, "read a map", err)
	}
	if row.GridColumns == nil {
		return nil, mapSubjectError{mapsv1.ImageGenerationBlockedReason_IMAGE_GENERATION_BLOCKED_REASON_MAP_HAS_NO_GRID}
	}
	img, err := s.queries.GetGalleryImageInCampaign(ctx, mapsdb.GetGalleryImageInCampaignParams{CampaignID: campaignID, ID: row.ImageID})
	if err != nil {
		return nil, s.dbError(ctx, "read the map's image", err)
	}
	g := gridOf(row.GridColumns, row.GridFactor, img.Width, img.Height)
	if !g.Valid() {
		return nil, mapSubjectError{mapsv1.ImageGenerationBlockedReason_IMAGE_GENERATION_BLOCKED_REASON_MAP_HAS_NO_GRID}
	}
	solid, set, rec, err := s.planAt(ctx, s.queries, campaignID, row, g)
	if err != nil {
		return nil, s.dbError(ctx, "read the map's floor plan", err)
	}
	sub := &subject{row: row, g: g, imgW: img.Width, imgH: img.Height, set: set, solid: solid}
	if len(rec.Rooms) > 0 {
		sub.rooms = roomLines(rec.Rooms)
	}
	if sub.tokens, err = s.queries.ListMapTokens(ctx, mapID); err != nil {
		return nil, s.dbError(ctx, "list the map's tokens", err)
	}
	return sub, nil
}

// floorPlan says which squares are solid: the layers' walls, and a square with a
// secret door (drawn as wall: the picture never gives it away). Every other door is
// floor. A generated dungeon needs nothing more, because its walls layer covers all
// of its rock (solidOf).
func floorPlan(set layerSet, g grid.Grid) []bool {
	return solidOf(g.Columns, g.Rows, set.walls, set.doors)
}

// roomLines is a dungeon's rooms as the text lines the textured map sends ("Sala 3:
// 7 x 5 squares"), the name and the size of each, nothing more. An unreadable list is
// no rooms.
func roomLines(raw []byte) []string {
	stored := &mapsv1.GetDungeonRoomsResponse{}
	if err := protojson.Unmarshal(raw, stored); err != nil {
		return nil
	}
	var out []string
	for _, r := range stored.GetRooms() {
		if len(out) == gen.MaxRooms {
			break
		}
		out = append(out, fmt.Sprintf("Sala %d: %d x %d squares", r.GetId(), r.GetFloor().GetWidth(), r.GetFloor().GetHeight()))
	}
	return out
}

// npcSeen is an NPC the players see.
type npcSeen struct {
	id, name string
}

// playersSeen is what the players' characters see of the map now (RN-28).
type playersSeen struct {
	// seen says which squares are in the picture, nil for the whole map (no fog).
	seen []bool
	// nobody: the fog is on and no living character of a player stands on the map.
	nobody  bool
	markers []refimg.Marker
	npcs    []npcSeen
	// npcTokens are the NPCs with a token on the map, seen or not (a hidden NPC's
	// portrait may not go as a character image).
	npcTokens []string
}

// playersSeenOf works out what the players see now: with the fog on, the union of
// the views of every player's character (the fog's own computation, with the group's
// sight, and none of what they remember); without it, the whole map. The creatures
// are the party (always, a player's character is never hidden from its party, D6) and
// the NPCs that are not hidden, on a square seen now, and none while a combat runs
// on the map (the combatants are not map tokens: as for a player's token read).
func (s *Service) playersSeenOf(ctx context.Context, campaignID string, sub *subject) (*playersSeen, error) {
	out := &playersSeen{}
	combat, err := s.combats.CombatPositions(ctx, nil, campaignID, sub.row.ID)
	if err != nil {
		return nil, s.dbError(ctx, "read the combatants' squares", err)
	}
	var view *vision.View
	isFog := fogged(sub.row)
	if isFog {
		var done bool
		if view, done, err = s.groupView(ctx, sub, out); err != nil {
			return nil, err
		} else if done {
			return out, nil
		}
	}
	ids := make([]string, 0, len(sub.tokens))
	for _, t := range sub.tokens {
		ids = append(ids, t.CharacterID)
	}
	chars, err := s.characters.MapCharacters(ctx, nil, campaignID, ids)
	if err != nil {
		return nil, s.dbError(ctx, "read the characters on the map", err)
	}
	byID := map[string]*charactersv1.CharacterSummary{}
	for _, c := range chars {
		byID[c.GetId()] = c
	}
	for _, t := range sub.tokens {
		if c, ok := byID[t.CharacterID]; ok { // not dead, nor waiting for approval
			out.addToken(sub, t, c, combat, view)
		}
	}
	return out, nil
}

// groupView works out the joined view of every player's character on a fog map
// into out.seen. done is true when there is nothing more to read: nobody is on the
// map, or the characters on it see nothing (magical darkness, blind: no view).
func (s *Service) groupView(ctx context.Context, sub *subject, out *playersSeen) (view *vision.View, done bool, err error) {
	points, err := s.queries.ListMapPoints(ctx, sub.row.ID)
	if err != nil {
		return nil, false, s.dbError(ctx, "list the points", err)
	}
	sg, err := s.newSight(ctx, nil, fogInputOfRow(sub.row, sub.imgW, sub.imgH), points, sub.tokens)
	if err != nil {
		return nil, false, s.dbError(ctx, "work out what the players see", err)
	}
	sg.group = true // every player character's view, joined
	view, onMap := sg.viewOf("")
	if !onMap {
		out.nobody = true
		out.seen = make([]bool, sub.g.Squares())
		return view, true, nil
	}
	var some bool
	out.seen, some = seenSquares(sub.g, view)
	if !some {
		out.nobody = true
		return view, true, nil
	}
	return view, false, nil
}

// addToken puts a token of a living character in the players' view: a marker, and for
// an NPC its entry in the pickable list. The party sits where its combat square is
// while a combat runs; an NPC that is hidden, or any NPC while a combat runs, is left
// out. With the fog on (view is not nil), only a square seen now counts.
func (p *playersSeen) addToken(sub *subject, t mapsdb.MapToken, c *charactersv1.CharacterSummary, combat link.CombatPositions, view *vision.View) {
	at := sub.g.SquareOf(int(t.XBp), int(t.YBp))
	party := c.GetKind() == charactersv1.CharacterKind_CHARACTER_KIND_PLAYER
	if !party {
		p.npcTokens = append(p.npcTokens, t.CharacterID)
	}
	if party {
		if sq, ok := combat.Positions[t.CharacterID]; ok {
			at = sq
		}
	} else if t.Hidden || combat.Running {
		return
	}
	if view != nil && view.At(at) < vision.SeenGrey {
		return // not on a square anyone sees now
	}
	p.markers = append(p.markers, refimg.Marker{Col: at.Col, Row: at.Row, Party: party})
	if !party {
		p.npcs = append(p.npcs, npcSeen{id: t.CharacterID, name: c.GetName()})
	}
}

// seenSquares says which squares a view sees, and whether it sees any.
func seenSquares(g grid.Grid, view *vision.View) (seen []bool, some bool) {
	seen = make([]bool, g.Squares())
	for i := range seen {
		seen[i] = view.Seen(grid.Square{Col: i % g.Columns, Row: i / g.Columns})
		some = some || seen[i]
	}
	return seen, some
}

// plan is the players' view as a drawing's plan.
func (p *playersSeen) plan(sub *subject) refimg.Plan {
	return refimg.Plan{Cols: sub.g.Columns, Rows: sub.g.Rows, Solid: sub.solid, Seen: p.seen, Markers: p.markers}
}

// referenceOf draws the reference of a layout at the given side: the players'
// view (cropped to what is seen) for the scene art and the isometric view, the
// whole padded map for the textured one. It returns the PNG and what it shows.
type reference struct {
	png     []byte
	pad     refimg.Pad // the textured map's canvas
	seen    int
	seenAll bool
}

func (s *Service) referenceOf(layout string, sub *subject, players *playersSeen, side int) (*reference, error) {
	if layout == gen.LayoutTexture {
		img, pad, err := refimg.RenderPadded(refimg.Plan{Cols: sub.g.Columns, Rows: sub.g.Rows, Solid: sub.solid}, int(sub.imgW), int(sub.imgH), side)
		if err != nil {
			return nil, fmt.Errorf("draw the whole map: %w", err)
		}
		data, err := refimg.PNG(img)
		if err != nil {
			return nil, fmt.Errorf("encode the drawing: %w", err)
		}
		return &reference{png: data, pad: pad, seen: sub.g.Squares(), seenAll: true}, nil
	}
	plan, seen := players.plan(sub).Crop()
	w, h := refimg.SizeFor(plan.Cols, plan.Rows, side)
	img, err := refimg.Render(plan, w, h)
	if err != nil {
		return nil, fmt.Errorf("draw the players' view: %w", err)
	}
	data, err := refimg.PNG(img)
	if err != nil {
		return nil, fmt.Errorf("encode the drawing: %w", err)
	}
	return &reference{png: data, seen: seen, seenAll: players.seen == nil}, nil
}

// errSubjectBlocked turns a loadSubject error into the typed refusal, with the month's
// status.
func (s *Service) errSubjectBlocked(ctx context.Context, campaignID string, err error) error {
	if blocked, ok := errors.AsType[mapSubjectError](err); ok {
		status, statusErr := s.freshStatus(ctx, campaignID)
		if statusErr != nil {
			return s.dbError(ctx, "read the image generation status", statusErr)
		}
		return errBlocked(blocked.reason, status)
	}
	return err
}

// GetMapImageReference implements mapsv1connect.ImageGenerationServiceHandler.
func (s *Service) GetMapImageReference(
	ctx context.Context,
	req *connect.Request[mapsv1.GetMapImageReferenceRequest],
) (*connect.Response[mapsv1.GetMapImageReferenceResponse], error) {
	m, err := requireGenerationMaster(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	_, layout, ok := mapKindOf(req.Msg.GetKind())
	if !ok {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("kind must be MAP_SCENE, ISOMETRIC or TEXTURED_MAP"))
	}
	sub, err := s.loadSubject(ctx, m.CampaignID, req.Msg.GetMapId())
	if err != nil {
		return nil, s.errSubjectBlocked(ctx, m.CampaignID, err)
	}
	// The textured map takes no characters: no players' view, no creatures to choose.
	players := &playersSeen{}
	if layout != gen.LayoutTexture {
		if players, err = s.playersSeenOf(ctx, m.CampaignID, sub); err != nil {
			return nil, err
		}
	}
	ref, err := s.referenceOf(layout, sub, players, previewSide)
	if err != nil {
		return nil, s.dbError(ctx, "draw the reference", err)
	}
	res := &mapsv1.GetMapImageReferenceResponse{
		Preview: ref.png, PreviewContentType: images.PNG,
		PlayersSeeSomething: layout == gen.LayoutTexture || !players.nobody,
		SeenSquares:         int32(ref.seen),                                   //nolint:gosec // G115: at most 200 x 400
		TotalSquares:        int32(sub.g.Squares()),                            //nolint:gosec // G115: at most 200 x 400
		GridColumns:         int32(sub.g.Columns), GridRows: int32(sub.g.Rows), //nolint:gosec // G115: at most 200 x 400
	}
	if layout == gen.LayoutTexture {
		res.PaddedRatio = ratioFromName(ref.pad.Ratio)
		res.RoomsListed = int32(len(sub.rooms)) //nolint:gosec // G115: at most gen.MaxRooms
		res.TextureTooLarge = int64(sub.imgW)*int64(sub.imgH) > maxTexturePixels
		res.MaxTexturePixels = maxTexturePixels
	}
	ids := make([]string, 0, len(players.npcs))
	for _, n := range players.npcs {
		ids = append(ids, n.id)
	}
	portraits, err := s.characters.NpcPortraits(ctx, nil, m.CampaignID, ids)
	if err != nil {
		return nil, s.dbError(ctx, "read the portraits", err)
	}
	for _, n := range players.npcs {
		res.Creatures = append(res.Creatures, &mapsv1.MapImageCreature{CharacterId: n.id, Name: n.name, PortraitImageId: portraits[n.id]})
	}
	return connect.NewResponse(res), nil
}

// GenerateMapImage implements mapsv1connect.ImageGenerationServiceHandler.
func (s *Service) GenerateMapImage(
	ctx context.Context,
	req *connect.Request[mapsv1.GenerateMapImageRequest],
) (*connect.Response[mapsv1.GenerateMapImageResponse], error) {
	m, err := requireGenerationMaster(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	kind, layout, ok := mapKindOf(req.Msg.GetKind())
	if !ok {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("kind must be MAP_SCENE, ISOMETRIC or TEXTURED_MAP"))
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
	mapID, ok := uid.Canonical(req.Msg.GetMapId())
	if !ok {
		return nil, errMapNotFound()
	}
	objects, characters, npcs, err := cleanMapReferences(req.Msg, layout)
	if err != nil {
		return nil, err
	}
	name, err := imageNameOf(req.Msg.GetName(), "")
	if err != nil {
		return nil, err
	}
	n := newRequest{
		kind: kind, key: key, hash: idem.Hash(req.Msg), prompt: prompt, style: style, ratio: ratio, name: name,
		references: objects, characters: characters, requestedBy: m.UserID,
		mapReq: &mapRequest{mapID: mapID, layout: layout, npcs: npcs},
	}
	row, status, err := s.begin(ctx, n, m.CampaignID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&mapsv1.GenerateMapImageResponse{Generation: s.generationToProto(row), Status: status}), nil
}

// cleanMapReferences checks the images and NPCs of a request made from a map: the
// counts, the IDs, the repeats, and that a textured map takes no characters at all
// (it can become the map the players read).
func cleanMapReferences(req *mapsv1.GenerateMapImageRequest, layout string) (objects, characters, npcs []string, err error) {
	objects, err = cleanImageIDs(req.GetObjectImageIds(), gen.MaxObjectReferences, "object_image_ids")
	if err != nil {
		return nil, nil, nil, err
	}
	characters, err = cleanImageIDs(req.GetCharacterImageIds(), gen.MaxCharacterReferences, "character_image_ids")
	if err != nil {
		return nil, nil, nil, err
	}
	npcs, err = cleanNpcIDs(req.GetNpcCharacterIds())
	if err != nil {
		return nil, nil, nil, err
	}
	if hasDuplicates(append(append([]string{}, objects...), characters...)) {
		return nil, nil, nil, connect.NewError(connect.CodeInvalidArgument, errors.New("an image is listed twice as a reference"))
	}
	if layout == gen.LayoutTexture {
		if len(npcs) > 0 {
			return nil, nil, nil, errField("npc_character_ids", "must be empty for a textured map")
		}
		if len(characters) > 0 {
			return nil, nil, nil, errField("character_image_ids", "must be empty for a textured map")
		}
	}
	return objects, characters, npcs, nil
}

// cleanNpcIDs checks the NPC IDs of a request: UUIDs, no repeats, at most as many as
// there are character references.
func cleanNpcIDs(ids []string) ([]string, error) {
	if len(ids) > gen.MaxCharacterReferences {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("npc_character_ids has at most %d characters", gen.MaxCharacterReferences))
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		u, err := uuid.Parse(id)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("npc_character_ids has an ID that is not a UUID"))
		}
		out = append(out, u.String())
	}
	if hasDuplicates(out) {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("an NPC is listed twice"))
	}
	return out, nil
}

// prepareMap draws the reference of a request made from a map and works out the NPC
// portraits that go with it, into p. It is part of prepare: the slot is not
// reserved yet, so a map the players cannot see, or an NPC they do not, costs
// nothing. The map is read as it is now; the textured map's basis (the image, grid and
// size it fits) is what UseGeneratedImageAsMapImage compares against later.
func (s *Service) prepareMap(ctx context.Context, campaignID string, n *newRequest, p *prepared, status *mapsv1.ImageGenerationStatus) error {
	sub, err := s.loadSubject(ctx, campaignID, n.mapReq.mapID)
	if err != nil {
		if blocked, ok := errors.AsType[mapSubjectError](err); ok {
			return errBlocked(blocked.reason, status)
		}
		return err
	}
	players, err := s.mapPlayers(ctx, campaignID, n, p, sub, status)
	if err != nil {
		return err
	}
	if n.name == "" {
		p.name = s.defaultMapPictureName(n.kind, sub.row)
	}
	if err := s.drawMap(n.mapReq.layout, sub, players, p); err != nil {
		s.logger.ErrorContext(ctx, "maps: cannot draw the reference of a map", "error", err)
		return connect.NewError(connect.CodeInternal, errors.New("cannot draw the map right now"))
	}
	return nil
}

// drawMap draws the reference of a map for the model into p, with the basis it was drawn from
// (and, for the textured map, the ratio the server picked, the rectangle on its canvas and the
// rooms). prepareMap draws when the request begins, and rebuildMap again when its task runs.
func (s *Service) drawMap(layout string, sub *subject, players *playersSeen, p *prepared) error {
	ref, err := s.referenceOf(layout, sub, players, refimg.Side)
	if err != nil {
		return err
	}
	p.drawing = &gen.Image{MimeType: images.PNG, Data: ref.png}
	p.basis = &mapBasis{
		mapID: sub.row.ID, imageID: sub.row.ImageID, gridColumns: *sub.row.GridColumns, gridFactor: sub.row.GridFactor,
		width: sub.imgW, height: sub.imgH, planHash: planHash(sub.solid),
	}
	if layout == gen.LayoutTexture {
		f := ref.pad.Fractions()
		p.basis.pad = &f
		p.ratio = ref.pad.Ratio
		p.rooms = sub.rooms
	}
	return nil
}

// errMapChanged: the map is not the one the request was made over (its image, grid or walls
// changed, or it is gone), so the drawing would not be the picture input the master agreed to.
var errMapChanged = errors.New("maps: the map changed after the image request")

// rebuildMap draws the reference of a request made from a map again, when its task runs, from
// the map as it is now, and checks it against what the request kept (the map, its image, grid,
// size, walls hash and, for the textured map, the rectangle on the canvas). A map that differs
// is errMapChanged: the request ends, nothing is sent. What the players see (the creatures
// drawn as discs) is read again too, so a task that runs late never draws what is hidden now.
func (s *Service) rebuildMap(ctx context.Context, row mapsdb.ImageRequest, p *prepared) error {
	if row.MapID == nil {
		return errMapChanged
	}
	layout := layoutOf(row.Kind)
	sub, err := s.loadSubject(ctx, row.CampaignID, *row.MapID)
	if err != nil {
		if _, blocked := errors.AsType[mapSubjectError](err); blocked || connect.CodeOf(err) == connect.CodeNotFound {
			return errMapChanged
		}
		return err
	}
	players := &playersSeen{}
	if layout != gen.LayoutTexture {
		if players, err = s.playersSeenOf(ctx, row.CampaignID, sub); err != nil {
			return err
		}
		if players.nobody {
			return errMapChanged
		}
	}
	if err := s.drawMap(layout, sub, players, p); err != nil {
		return fmt.Errorf("draw the reference of a map: %w", err)
	}
	if !basisMatches(row, p.basis) {
		return errMapChanged
	}
	return nil
}

// basisMatches says whether the basis a drawing was made from is the one the request kept.
func basisMatches(row mapsdb.ImageRequest, b *mapBasis) bool {
	same := func(stored *string, now string) bool { return stored != nil && *stored == now }
	size := func(stored *int32, now int32) bool { return stored != nil && *stored == now }
	if !same(row.MapID, b.mapID) || !same(row.MapImageID, b.imageID) || !same(row.MapPlanHash, b.planHash) ||
		!size(row.MapGridColumns, b.gridColumns) || !size(row.MapGridFactor, b.gridFactor) ||
		!size(row.MapWidth, b.width) || !size(row.MapHeight, b.height) {
		return false
	}
	if b.pad == nil {
		return true
	}
	for i, stored := range [4]*float64{row.PadX0, row.PadY0, row.PadX1, row.PadY1} {
		if stored == nil || *stored != b.pad[i] {
			return false
		}
	}
	return true
}

// mapPlayers checks what the request may say of the map's creatures and returns the
// players' view. The textured map takes no characters (GenerateMapImage refused them):
// no view is worked out for it, but its map must fit a texture and no object may be an
// NPC's portrait. The scene art and the isometric view start from the players' view.
func (s *Service) mapPlayers(ctx context.Context, campaignID string, n *newRequest, p *prepared, sub *subject, status *mapsv1.ImageGenerationStatus) (*playersSeen, error) {
	if n.mapReq.layout == gen.LayoutTexture {
		if int64(sub.imgW)*int64(sub.imgH) > maxTexturePixels {
			return nil, errBlocked(mapsv1.ImageGenerationBlockedReason_IMAGE_GENERATION_BLOCKED_REASON_MAP_IMAGE_TOO_LARGE, status)
		}
		return &playersSeen{}, s.checkTexturePortraits(ctx, campaignID, n, sub)
	}
	players, err := s.playersSeenOf(ctx, campaignID, sub)
	if err != nil {
		return nil, err
	}
	if players.nobody {
		return nil, errBlocked(mapsv1.ImageGenerationBlockedReason_IMAGE_GENERATION_BLOCKED_REASON_PLAYERS_SEE_NOTHING, status)
	}
	return players, s.checkCharacters(ctx, campaignID, n, p, players)
}

// defaultMapPictureName is the name of a picture made from a map when the master
// gave none: the map's name only when the players already see the map (a hidden
// map's name can be a secret, and they read the name of a picture they are shown,
// RN-10), else a dated one.
func (s *Service) defaultMapPictureName(kind string, row mapsdb.Map) string {
	if row.RevealedAt != nil {
		return suffixedName(row.Name, " · "+kindNamePT(kind))
	}
	return datedName(kindTitlePT(kind), s.now())
}

// kindTitlePT is the way a picture of a map was made, at the start of a default name ("Vista isométrica · 06/10").
func kindTitlePT(kind string) string {
	switch kind {
	case kindIsometric:
		return "Vista isométrica"
	case kindTexturedMap:
		return "Mapa com textura"
	default:
		return "Arte da cena"
	}
}

// kindNamePT is the way a picture of a map was made, as the default name of its gallery image says it.
func kindNamePT(kind string) string {
	switch kind {
	case kindIsometric:
		return "vista isométrica"
	case kindTexturedMap:
		return "mapa com textura"
	default:
		return "arte da cena"
	}
}

// checkTexturePortraits refuses, for a textured map, the portrait of any NPC with a token
// on the map as an object reference, seen or not: the picture starts from the whole map and
// shows no creature, and it can become the map the players read, so no portrait has a
// reason to be in it (RN-10).
func (s *Service) checkTexturePortraits(ctx context.Context, campaignID string, n *newRequest, sub *subject) error {
	if len(n.references) == 0 || len(sub.tokens) == 0 {
		return nil
	}
	ids := make([]string, 0, len(sub.tokens))
	for _, t := range sub.tokens {
		ids = append(ids, t.CharacterID)
	}
	portraits, err := s.characters.NpcPortraits(ctx, nil, campaignID, ids)
	if err != nil {
		return s.dbError(ctx, "read the portraits", err)
	}
	for _, img := range portraits {
		if img != "" && slices.Contains(n.references, img) {
			return errField("object_image_ids", "has the portrait of a creature on the map")
		}
	}
	return nil
}

// checkCharacters is what a scene art or isometric request says of characters: the NPCs
// the master marked must be ones the players see now (their portraits are added to p), and
// no character image may be the portrait of an NPC whose token is on this map and whom the
// players do not see (a gallery image that is no NPC's portrait stays the master's choice).
func (s *Service) checkCharacters(ctx context.Context, campaignID string, n *newRequest, p *prepared, players *playersSeen) error {
	visible := map[string]bool{}
	for _, v := range players.npcs {
		visible[v.id] = true
	}
	for _, id := range n.mapReq.npcs {
		if !visible[id] {
			return errField("npc_character_ids", "has a creature the players do not see")
		}
	}
	if err := s.checkHiddenPortraits(ctx, campaignID, n, players, visible); err != nil {
		return err
	}
	return s.addNpcPortraits(ctx, campaignID, n, p)
}

// checkHiddenPortraits refuses, as a character or as an object, the portrait of an NPC
// whose token is on the map and whom the players do not see.
func (s *Service) checkHiddenPortraits(ctx context.Context, campaignID string, n *newRequest, players *playersSeen, visible map[string]bool) error {
	var hidden []string
	for _, id := range players.npcTokens {
		if !visible[id] {
			hidden = append(hidden, id)
		}
	}
	if len(hidden) == 0 || len(n.characters) == 0 && len(n.references) == 0 {
		return nil
	}
	portraits, err := s.characters.NpcPortraits(ctx, nil, campaignID, hidden)
	if err != nil {
		return s.dbError(ctx, "read the portraits", err)
	}
	// In the order of the NPCs, so the same request always gets the same refusal.
	for _, npc := range slices.Sorted(maps.Keys(portraits)) {
		img := portraits[npc]
		if img == "" {
			continue
		}
		// The portrait is refused wherever it comes: as a character, or chosen as an object.
		if slices.Contains(n.characters, img) {
			return errField("character_image_ids", "has the portrait of a creature the players do not see")
		}
		if slices.Contains(n.references, img) {
			return errField("object_image_ids", "has the portrait of a creature the players do not see")
		}
	}
	return nil
}

// addNpcPortraits adds the portraits of the NPCs the master marked to p, as characters
// (once each, also when the master picked the image too), within the characters' cap.
func (s *Service) addNpcPortraits(ctx context.Context, campaignID string, n *newRequest, p *prepared) error {
	if len(n.mapReq.npcs) == 0 {
		return nil
	}
	portraits, err := s.characters.NpcPortraits(ctx, nil, campaignID, n.mapReq.npcs)
	if err != nil {
		return s.dbError(ctx, "read the portraits", err)
	}
	have := map[string]bool{}
	for _, id := range n.characters {
		have[id] = true
	}
	for _, id := range n.mapReq.npcs {
		if img := portraits[id]; img != "" && !have[img] {
			have[img] = true
			p.npcImages = append(p.npcImages, img)
		}
	}
	if len(n.characters)+len(p.npcImages) > gen.MaxCharacterReferences {
		return errField("npc_character_ids", fmt.Sprintf("and character_image_ids pass %d characters in all", gen.MaxCharacterReferences))
	}
	return nil
}

// cropToMap makes the textured map's stored picture of what the model returned: the
// map's rectangle of the padded canvas, scaled to the size of the map's image. It
// holds the processing slot, like any image decode.
func (s *Service) cropToMap(ctx context.Context, data []byte, row mapsdb.ImageRequest) (*images.Result, error) {
	if row.MapWidth == nil || row.MapHeight == nil || row.PadX0 == nil || row.PadY0 == nil || row.PadX1 == nil || row.PadY1 == nil {
		return nil, errors.New("a textured map has no map size or pad")
	}
	w, h := int(*row.MapWidth), int(*row.MapHeight)
	f := refimg.Fractions{*row.PadX0, *row.PadY0, *row.PadX1, *row.PadY1} // what was drawn, not worked out again
	select {
	case s.processing <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-s.processing }()
	return images.CropFit(data, func(rw, rh int) image.Rectangle { return refimg.CropBy(f, rw, rh) }, w, h)
}

// fitToMap makes an edit of a textured map the size of the map's image: the middle of what the model
// returned that has the map's proportions, scaled to the map's size (the edit has no canvas to crop: it
// starts from the map itself, and was asked for the ratio nearest the map's). It never stretches.
func (s *Service) fitToMap(ctx context.Context, data []byte, row mapsdb.ImageRequest) (*images.Result, error) {
	select {
	case s.processing <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-s.processing }()
	w, h := int(*row.MapWidth), int(*row.MapHeight)
	return images.CropFit(data, func(rw, rh int) image.Rectangle { return refimg.CenterCrop(w, h, rw, rh) }, w, h)
}

// UseGeneratedImageAsMapImage implements mapsv1connect.ImageGenerationServiceHandler.
func (s *Service) UseGeneratedImageAsMapImage(
	ctx context.Context,
	req *connect.Request[mapsv1.UseGeneratedImageAsMapImageRequest],
) (*connect.Response[mapsv1.UseGeneratedImageAsMapImageResponse], error) {
	m, err := requireGenerationMaster(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	imageID, ok := uid.Canonical(req.Msg.GetImageId())
	if !ok {
		return nil, errImageNotFound()
	}
	request, err := s.texturedMapRequest(ctx, m.CampaignID, imageID)
	if err != nil {
		return nil, err
	}
	call := useCall{m: m, request: request, imageID: imageID, mapID: *request.MapID}
	current, err := s.currentMap(ctx, m.CampaignID)
	if err != nil {
		return nil, err
	}
	call.status, err = s.freshStatus(ctx, m.CampaignID) // for the refusal's detail, read outside the transaction
	if err != nil {
		return nil, s.dbError(ctx, "read the image generation status", err)
	}
	if call.imageCopy, call.fogSeen, err = s.imageCopyFor(ctx, m.CampaignID, call.mapID, imageID); err != nil {
		return nil, s.dbError(ctx, "copy the textured map", err)
	}
	var out useOutcome
	err = s.inSession(ctx, m.CampaignID, func(tx pgx.Tx, t sessiontell.Tell) error {
		var err error
		if out, err = s.useGeneratedImageTx(ctx, s.queries.WithTx(tx), &call); err != nil || out.alreadyUsed {
			return err // already the map's image: nothing changed, nothing is told and no number moves
		}
		if out.retried {
			t.Replay() // the first call may have died before it told: the notice goes out again, with the number as it stands
		}
		s.announce(t, func(ctx context.Context) {
			s.tiles.forget(call.mapID) // the fog's tiles are cut from the image
			s.publishMapChanged(ctx, m.CampaignID, call.mapID, playersSee(call.mapID, out.updated.RevealedAt, current))
		})
		return nil
	})
	s.discardFogCopy(ctx, m.CampaignID, call.imageCopy, err != nil || !out.copied)
	if err != nil {
		return nil, s.dbError(ctx, "use a generated image as the map's image", err)
	}
	masterMap, err := s.masterMap(ctx, m, call.mapID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&mapsv1.UseGeneratedImageAsMapImageResponse{Map: masterMap}), nil
}

// useCall is what UseGeneratedImageAsMapImage read before its transaction.
type useCall struct {
	m       authz.Membership
	request mapsdb.ImageRequest
	imageID string
	mapID   string
	status  *mapsv1.ImageGenerationStatus
	// imageCopy is the copy of the picture the map gets when the picture is used
	// another way too, or nil.
	imageCopy *fogCopy
	fogSeen   bool // the fog the copy was prepared for (imageCopyFor)
}

// useOutcome is what one attempt of the transaction decided.
type useOutcome struct {
	updated     mapsdb.Map
	copied      bool
	alreadyUsed bool
	retried     bool // the image was already the map's: the retry tells again and moves no number
}

// errMapChanged is the refusal for a map that is no longer the one the picture was drawn for.
func (c *useCall) errMapChanged() error {
	return errBlocked(mapsv1.ImageGenerationBlockedReason_IMAGE_GENERATION_BLOCKED_REASON_MAP_CHANGED, c.status)
}

// texturedMapRequest finds the request that made a gallery image and checks the
// image is a textured map, or an edit of one (it kept the map's basis): the
// gallery says which. Anything else, or a map that is gone, is `not_found`.
func (s *Service) texturedMapRequest(ctx context.Context, campaignID, imageID string) (mapsdb.ImageRequest, error) {
	request, err := s.queries.GetImageRequestForImage(ctx, mapsdb.GetImageRequestForImageParams{CampaignID: campaignID, ImageID: &imageID})
	if errors.Is(err, pgx.ErrNoRows) {
		return mapsdb.ImageRequest{}, errImageNotFound()
	}
	if err != nil {
		return mapsdb.ImageRequest{}, s.dbError(ctx, "find the request of a generated image", err)
	}
	picture, err := s.queries.GetGalleryImageInCampaign(ctx, mapsdb.GetGalleryImageInCampaignParams{CampaignID: campaignID, ID: imageID})
	if errors.Is(err, pgx.ErrNoRows) {
		return mapsdb.ImageRequest{}, errImageNotFound()
	}
	if err != nil {
		return mapsdb.ImageRequest{}, s.dbError(ctx, "read the generated image", err)
	}
	if picture.GeneratedKind != kindTexturedMap || request.MapID == nil {
		return mapsdb.ImageRequest{}, errImageNotFound() // not a textured map, or its map is gone
	}
	return request, nil
}

// useGeneratedImageTx is one attempt of the transaction: it locks the map, answers a retry,
// checks the map is still the one the picture was drawn for, and swaps the image.
func (s *Service) useGeneratedImageTx(ctx context.Context, q *mapsdb.Queries, c *useCall) (useOutcome, error) {
	campaignID := c.m.CampaignID
	locked, err := q.GetMapForUpdate(ctx, mapsdb.GetMapForUpdateParams{CampaignID: campaignID, ID: c.mapID})
	if errors.Is(err, pgx.ErrNoRows) {
		return useOutcome{}, errImageNotFound()
	}
	if err != nil {
		return useOutcome{}, fmt.Errorf("find the map: %w", err)
	}
	if locked.ImageID == c.imageID {
		return useOutcome{updated: locked, retried: true}, nil // already its image: a retry changes nothing
	}
	// A Use that already happened (maybe with a copy, for a fog map): the same answer
	// while the image it set is still the map's, with no new copy and no write.
	if again, err := q.GetImageRequest(ctx, mapsdb.GetImageRequestParams{CampaignID: campaignID, ID: c.request.ID}); err != nil {
		return useOutcome{}, fmt.Errorf("read the request again: %w", err)
	} else if again.UsedMapImageID != nil && *again.UsedMapImageID == locked.ImageID {
		return useOutcome{updated: locked, alreadyUsed: true}, nil
	}
	if err := checkUseGrid(ctx, q, c, locked); err != nil {
		return useOutcome{}, err
	}
	old, fresh, err := s.checkUseSize(ctx, q, c, locked)
	if err != nil {
		return useOutcome{}, err
	}
	// What the drawing showed: a wall painted, or a secret door revealed, since then.
	solid, _, _, err := s.planAt(ctx, q, campaignID, locked, gridOf(locked.GridColumns, locked.GridFactor, old.Width, old.Height))
	if err != nil {
		return useOutcome{}, err
	}
	if c.request.MapPlanHash == nil || planHash(solid) != *c.request.MapPlanHash {
		return useOutcome{}, c.errMapChanged()
	}
	if c.fogSeen != locked.FogEnabled {
		return useOutcome{}, c.errMapChanged() // the fog changed after the copy was prepared, and the revision did not
	}
	return s.swapMapImage(ctx, q, c, locked, old, fresh)
}

// checkUseGrid refuses a map whose image is not the one the request was made
// over, or whose grid is not the request's.
func checkUseGrid(ctx context.Context, q *mapsdb.Queries, c *useCall, locked mapsdb.Map) error {
	request := c.request
	fits, err := mapImageFitsRequest(ctx, q, c.m.CampaignID, request, locked.ImageID)
	if err != nil {
		return err
	}
	if !fits || locked.GridColumns == nil ||
		request.MapGridColumns == nil || *locked.GridColumns != *request.MapGridColumns ||
		request.MapGridFactor == nil || locked.GridFactor != *request.MapGridFactor {
		return c.errMapChanged()
	}
	return nil
}

// checkUseSize reads the map's current image and the textured one, and refuses
// a size that is not the request's. It returns both.
func (s *Service) checkUseSize(ctx context.Context, q *mapsdb.Queries, c *useCall, locked mapsdb.Map) (old, fresh mapsdb.GalleryImage, err error) {
	request := c.request
	old, err = q.GetGalleryImageInCampaign(ctx, mapsdb.GetGalleryImageInCampaignParams{CampaignID: c.m.CampaignID, ID: locked.ImageID})
	if err != nil {
		return old, fresh, fmt.Errorf("read the map's image: %w", err)
	}
	fresh, err = q.GetGalleryImageInCampaign(ctx, mapsdb.GetGalleryImageInCampaignParams{CampaignID: c.m.CampaignID, ID: c.imageID})
	if errors.Is(err, pgx.ErrNoRows) {
		return old, fresh, errImageNotFound()
	}
	if err != nil {
		return old, fresh, fmt.Errorf("read the textured map: %w", err)
	}
	if request.MapWidth == nil || request.MapHeight == nil || old.Width != *request.MapWidth || old.Height != *request.MapHeight ||
		fresh.Width != old.Width || fresh.Height != old.Height {
		return old, fresh, c.errMapChanged()
	}
	return old, fresh, nil
}

// swapMapImage puts the textured map on the map (a copy of it, when there is
// one) and marks the request used.
func (s *Service) swapMapImage(ctx context.Context, q *mapsdb.Queries, c *useCall, locked mapsdb.Map, old, fresh mapsdb.GalleryImage) (useOutcome, error) {
	var out useOutcome
	var err error
	if c.imageCopy != nil {
		if fresh, err = c.imageCopy.insertRow(ctx, q, s, c.m.CampaignID, &c.m.UserID); err != nil {
			return useOutcome{}, err
		}
		out.copied = true
	}
	if out.updated, err = s.replaceMapImageKeepingLayers(ctx, q, locked, old, fresh); err != nil {
		return useOutcome{}, err
	}
	err = q.MarkImageRequestUsed(ctx, mapsdb.MarkImageRequestUsedParams{CampaignID: c.m.CampaignID, ID: c.request.ID, UsedMapImageID: &fresh.ID})
	return out, err
}

// maxEditChain bounds the walk up the edits of an edit: far more than a master makes.
const maxEditChain = 64

// mapImageFitsRequest says whether the map's current image (imageID) is the one
// the request was made over: the map's image when the request was made, or a
// picture that "Usar" put on the map from the request itself or from an earlier
// edit of the same chain (an edit keeps the basis of the picture it adjusts, and
// after that picture is used the map's image is no longer the original). The grid,
// size and walls checks that follow keep the rest of the basis.
func mapImageFitsRequest(ctx context.Context, q *mapsdb.Queries, campaignID string, request mapsdb.ImageRequest, imageID string) (bool, error) {
	if request.MapImageID == nil {
		return false, nil
	}
	if imageID == *request.MapImageID {
		return true, nil
	}
	for cur, steps := request, 0; cur.SourceImageID != nil && steps < maxEditChain; steps++ {
		from, err := q.GetImageRequestForImage(ctx, mapsdb.GetImageRequestForImageParams{CampaignID: campaignID, ImageID: cur.SourceImageID})
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("read the request of an edited image: %w", err)
		}
		if from.MapImageID == nil || *from.MapImageID != *request.MapImageID {
			return false, nil // not made over the same map image
		}
		if from.UsedMapImageID != nil && *from.UsedMapImageID == imageID {
			return true, nil
		}
		cur = from
	}
	return false, nil
}

// basisParams are a basis's columns of the request's row; a request that is not
// made from a map has none (NULL).
type basisParams struct {
	mapID, imageID                         *string
	gridColumns, gridFactor, width, height *int32
	planHash                               *string
	pad                                    [4]*float64
}

func (b *mapBasis) params() basisParams {
	if b == nil {
		return basisParams{}
	}
	out := basisParams{mapID: &b.mapID, imageID: &b.imageID, gridColumns: &b.gridColumns, gridFactor: &b.gridFactor, width: &b.width, height: &b.height, planHash: &b.planHash}
	if b.pad != nil {
		for i := range out.pad {
			out.pad[i] = &b.pad[i]
		}
	}
	return out
}
