package maps

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/fnv"
	"slices"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	"github.com/PuraFome/meuRPG/backend/internal/maps/link"
	"github.com/PuraFome/meuRPG/backend/internal/maps/mapsdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
	"github.com/PuraFome/meuRPG/backend/internal/rules/vision"
)

// What each player sees of a map with the fog of war on (MR-036, RN-10).
// Everything a player receives about such a map goes through here: this is
// the one place that says which squares a player sees now, which they remember,
// and so which points, tokens and layers they may be sent.
//
// The arithmetic is package rules/vision's: this file only builds its scene
// from the map (the walls and the painted light, the Luz points, the lights the
// characters carry), asks it what each viewer sees, and keeps what a player has
// seen.
//
//   - Whose eyes. A player sees through their own living character, standing on
//     its token's square, or, while a combat runs on the map, on its combatant's
//     square (CombatMaps.CombatPositions). With "Visão do grupo" on, every player
//     sees the union of every player character's view. A character with no token
//     on the map sees nothing new. Its senses (darkvision...) come from the
//     characters module (CharacterDirectory.PartyVision): the beast's in Wild
//     Shape (a wolf has no darkvision). Of a character's creatures only the
//     familiar sees for its player, and only while the player looks through its
//     eyes ("Ver pelos olhos do familiar") and it is within 30 m of the character
//     (familiarEyes), and the character's own view is then switched off (it is blind and deaf, SRD): D6's "each player sees what their own character (and its
//     creatures) sees" is the party's, and the SRD gives a familiar's senses to its
//     master only as that action, so every other creature of the character lets
//     the player see the creature, not through it. The creatures' tokens are party
//     tokens, never hidden (creaturetokens.go).
//   - What was seen stays. map_vision_memory has, per map and player, a bitmap of
//     the squares they have seen. It grows only on the writes that change what
//     someone sees (refreshVision, called after them), never on a read, and it
//     holds squares, never creatures. A read shows what is seen now and what is
//     remembered.
//   - The cache. The light of a scene (vision.Lit) does not depend on the
//     viewer and is the expensive part, so the compiled scene is kept, keyed by
//     what it depends on: the grid, the base light, the layers' revisions and the
//     light sources. The views of the viewers are kept with it, a few of them.
//     The server has one small CPU and little memory, so the cache holds a handful of
//     scenes and no more.
//
// The states a viewer gets for a square, and how they are packed, are in
// GetMapVisionResponse (maps.proto).

// The codes of a square's state in GetMapVisionResponse: the vision package's
// states, then "remembered".
const (
	stateUnseen     byte = byte(vision.Unseen)
	stateRemembered byte = 5
)

// fogInput is what the fog needs of a map's row: the grid and the settings that
// decide its scene.
type fogInput struct {
	mapID, campaignID string
	g                 grid.Grid
	base              grid.Light
	group             bool
	layersRev         int32
	lightRev          int32
	epoch             int32 // the generation of what the players remember (maps.vision_epoch)
}

// fogged says whether a map's row has the fog of war on with a grid to work on.
func fogged(r mapsdb.Map) bool { return r.FogEnabled && r.GridColumns != nil }

func baseLightOf(word string) grid.Light {
	switch word {
	case "bright":
		return grid.Bright
	case "dim":
		return grid.Dim
	}
	return grid.Dark
}

// fogInputOfRow reads the fog's input from a map's row and its image's size.
func fogInputOfRow(r mapsdb.Map, width, height int32) fogInput {
	return fogInput{
		mapID: r.ID, campaignID: r.CampaignID, g: gridOf(r.GridColumns, r.GridFactor, width, height),
		base: baseLightOf(r.BaseLight), group: r.GroupVision, layersRev: r.LayersRevision, lightRev: r.LightRevision, epoch: r.VisionEpoch,
	}
}

// fogInputOfDetails is fogInputOfRow for the row of ListMapDetails.
func fogInputOfDetails(r mapsdb.ListMapDetailsRow) fogInput {
	return fogInput{
		mapID: r.ID, campaignID: r.CampaignID, g: gridOf(r.GridColumns, r.GridFactor, r.ImageWidth, r.ImageHeight),
		base: baseLightOf(r.BaseLight), group: r.GroupVision, layersRev: r.LayersRevision, lightRev: r.LightRevision, epoch: r.VisionEpoch,
	}
}

// ---- the cache of compiled scenes ----

const (
	// litCacheSize is how many compiled scenes are kept: the maps in play at the
	// table. The cost of one at the largest grid (200 x 400) is measured in
	// TestSceneMemory and written in CONTRIBUTING ("As medidas da névoa"): the
	// compiled light and its walls, about 1.6 MB, and 80 kB for each view it keeps.
	litCacheSize = 8
	// viewsPerScene is how many viewers' views a compiled scene keeps. A table
	// has a handful of characters; when the limit is reached the views start over.
	viewsPerScene = 24
)

// litKey is what a compiled scene depends on, hashed: a scene with the same key
// is the same scene.
type litKey struct {
	mapID string
	sum   uint64
}

// litEntry is a compiled scene and the views already asked of it. It is
// compiled once, whoever asks first, and the others wait for it (single flight).
type litEntry struct {
	key  litKey
	once sync.Once
	lit  *vision.Lit
	set  layerSet // the layers the scene was compiled from
	err  error

	mu    sync.Mutex
	views map[vision.Viewer]*vision.View
}

// see is what the viewer sees, from the entry's cache when it was asked before.
func (e *litEntry) see(v vision.Viewer) *vision.View {
	e.mu.Lock()
	if cached, ok := e.views[v]; ok {
		e.mu.Unlock()
		return cached
	}
	e.mu.Unlock()
	view := e.lit.See(v)
	e.mu.Lock()
	if len(e.views) >= viewsPerScene {
		clear(e.views)
	}
	e.views[v] = view
	e.mu.Unlock()
	return view
}

// litCache keeps the most recently used compiled scenes. The zero value works.
type litCache struct {
	mu      sync.Mutex
	entries []*litEntry // most recently used first
}

// entry is the cache's entry for the key, made (not yet compiled) when it is
// not there: the caller compiles it inside entry.once.
func (c *litCache) entry(key litKey) *litEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, e := range c.entries {
		if e.key == key {
			copy(c.entries[1:i+1], c.entries[:i])
			c.entries[0] = e
			return e
		}
	}
	e := &litEntry{key: key, views: map[vision.Viewer]*vision.View{}}
	if len(c.entries) < litCacheSize {
		c.entries = append(c.entries, nil)
	}
	copy(c.entries[1:], c.entries)
	c.entries[0] = e
	return e
}

// drop removes an entry that failed to compile.
func (c *litCache) drop(e *litEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = slices.DeleteFunc(c.entries, func(x *litEntry) bool { return x == e })
}

// forget drops the scenes of a map (it is deleted, or its grid changed).
func (c *litCache) forget(mapID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = slices.DeleteFunc(c.entries, func(e *litEntry) bool { return e.key.mapID == mapID })
}

// ---- the scene of a map ----

// sight is a map's scene ready to say what each player sees.
type sight struct {
	g       grid.Grid
	entry   *litEntry
	members []link.PartyMember
	// stands is where each character of the party sees from, by character ID: its
	// combatant's square while a combat runs, else its token's. A character absent
	// from it is not on the map.
	stands map[string]grid.Square
	// extra are the other pairs of eyes a player has right now, by character ID:
	// the familiar they look through, at the square it stands on (MR-036).
	extra map[string][]vision.Viewer
	group bool
	// combat says a combat runs on the map: the NPC tokens are then left out of
	// a player's reads (fogcombat.go reads the combatants).
	combat bool
	// openPlan says the map's base light is "Claro": the players see the whole floor
	// plan (see playerView.terrainAll). It never touches what is in sight.
	openPlan bool
}

// partyMemo is the party's senses read once for a request that works out several
// maps (ListMaps): deriving every sheet again for each map is the costly part of it.
type partyMemo struct {
	once    sync.Once
	members []link.PartyMember
	err     error
}

type partyMemoKey struct{}

// withPartyMemo gives a request's context a memo of the party.
func withPartyMemo(ctx context.Context) context.Context {
	return context.WithValue(ctx, partyMemoKey{}, &partyMemo{})
}

func partyOf(ctx context.Context, tx pgx.Tx, s *Service, campaignID string) ([]link.PartyMember, error) {
	if memo, ok := ctx.Value(partyMemoKey{}).(*partyMemo); ok {
		memo.once.Do(func() { memo.members, memo.err = s.characters.PartyVision(ctx, tx, campaignID) })
		return memo.members, memo.err
	}
	return s.characters.PartyVision(ctx, tx, campaignID)
}

// pointCounts remembers how many points a player receives of a fog map, for the
// map's `point_count` in ListMaps: the count is worked out again only when what
// the player knows (the view's revision) or the points changed. Bounded: it starts
// over when it fills.
type pointCounts struct {
	mu sync.Mutex
	m  map[countKey]int32
}

type countKey struct {
	mapID, userID string
	revision      int32
	points        uint64
	traps         uint64 // the traps the player's characters know (map_point_reveals)
}

const maxPointCounts = 512

func (c *pointCounts) get(k countKey) (int32, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	n, ok := c.m[k]
	return n, ok
}

func (c *pointCounts) put(k countKey, n int32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil || len(c.m) >= maxPointCounts {
		c.m = map[countKey]int32{}
	}
	c.m[k] = n
}

// trapsSig hashes the traps a player knows: a reveal does not touch the point's row.
func trapsSig(known map[string]knownTrap) uint64 {
	var sum uint64
	for id, k := range known {
		h := fnv.New64a()
		_, _ = h.Write([]byte(id))
		if k.public {
			_, _ = h.Write([]byte{1})
		}
		sum ^= h.Sum64() // order does not matter
	}
	return sum
}

// pointsSig hashes what decides which points a player receives and what they say:
// each point's ID, its last change and who may see it.
func pointsSig(points []mapsdb.MapPoint) uint64 {
	h := fnv.New64a()
	for _, p := range points {
		_, _ = h.Write([]byte(p.ID))
		_, _ = h.Write([]byte(p.UpdatedAt.UTC().Format("20060102150405.000000")))
		for _, t := range []*time.Time{p.RevealedAt, p.TrapTriggeredAt, p.TreasureFoundAt} {
			if t != nil {
				_, _ = h.Write([]byte(t.UTC().Format("20060102150405.000000")))
			}
			_, _ = h.Write([]byte{0})
		}
	}
	return h.Sum64()
}

// obscured are the squares the combat's spell zones make heavily obscured (clouds) or magically dark.
type obscured struct{ clouds, dark []grid.Square }

// obscuredLayer is the squares as a layer of the grid, nil for none; squares outside the grid are left out.
func obscuredLayer(g grid.Grid, squares []grid.Square) *grid.Layer {
	if len(squares) == 0 {
		return nil
	}
	l := grid.NewLayer(g)
	for _, sq := range squares {
		if g.Contains(sq) {
			l.Set(sq.Col, sq.Row, true)
		}
	}
	return l
}

// sortedSquares is the squares of the grid in row, column order, each once, without changing the argument: what
// the scene is made of, so two lists with the same scene have the same key.
func sortedSquares(g grid.Grid, in []grid.Square) []grid.Square {
	out := slices.DeleteFunc(slices.Clone(in), func(sq grid.Square) bool { return !g.Contains(sq) })
	slices.SortFunc(out, func(a, b grid.Square) int { return cmpInts(a.Row, b.Row, a.Col, b.Col) })
	return slices.Compact(out)
}

// source is a light on the map, before it is a vision.Source.
type source = vision.Source

// litCompileTimeout bounds the compile of a scene that many requests share
// and none of them owns (see newSight).
const litCompileTimeout = 30 * time.Second

// newSight builds the scene of a fog map from what the caller already read: the
// map's points (the Luz ones shine) and tokens (the carried lights, and where
// the party stands). It reads the party's senses and, while a combat runs, the
// combatants' squares, and compiles the light unless the cache has it.
func (s *Service) newSight(ctx context.Context, tx pgx.Tx, in fogInput, points []mapsdb.MapPoint, tokens []mapsdb.MapToken) (*sight, error) {
	if !in.g.Valid() {
		return nil, grid.ErrBadGrid
	}
	members, err := partyOf(ctx, tx, s, in.campaignID)
	if err != nil {
		return nil, fmt.Errorf("read the party's senses: %w", err)
	}
	combat, err := s.combats.CombatPositions(ctx, tx, in.campaignID, in.mapID)
	if err != nil {
		return nil, fmt.Errorf("read the combatants' squares: %w", err)
	}
	where := standingSquare(in.g, combat, tokens)

	sources, err := s.lightSources(ctx, tx, in, points, tokens, where)
	if err != nil {
		return nil, err
	}
	sg := &sight{g: in.g, members: members, stands: map[string]grid.Square{}, group: in.group, combat: combat.Running, openPlan: in.base == grid.Bright}
	for _, m := range members {
		if sq, ok := where(m.CharacterID); ok && !m.Dead {
			sg.stands[m.CharacterID] = sq
		}
	}

	if sg.extra, err = s.familiarEyes(ctx, tx, in, members, sg.stands, combat); err != nil {
		return nil, err
	}
	if sg.entry, err = s.sceneEntry(ctx, tx, in, sources, obscured{clouds: combat.Clouds, dark: combat.MagicDark}); err != nil {
		return nil, err
	}
	return sg, nil
}

// standingSquare says where a character stands: its square in the combat, else its
// token's.
func standingSquare(g grid.Grid, combat link.CombatPositions, tokens []mapsdb.MapToken) func(string) (grid.Square, bool) {
	tokenAt := make(map[string]grid.Square, len(tokens))
	for _, t := range tokens {
		tokenAt[t.CharacterID] = g.SquareOf(int(t.XBp), int(t.YBp))
	}
	return func(id string) (grid.Square, bool) {
		if sq, ok := combat.Positions[id]; ok {
			return sq, true
		}
		sq, ok := tokenAt[id]
		return sq, ok
	}
}

// compileScene compiles the scene of the map from its stored layers and the sources,
// into the entry; the entry's err says why it could not.
func (s *Service) compileScene(ctx context.Context, tx pgx.Tx, in fogInput, sources []source, ob obscured, e *litEntry) {
	stored, err := queriesIn(s.queries, tx).GetMapLayers(ctx, in.mapID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		e.err = fmt.Errorf("read the layers: %w", err)
		return
	}
	e.set = loadLayers(stored, in.g)
	if e.lit, err = vision.Compile(vision.Scene{Grid: in.g, Walls: e.set.walls, Doors: e.set.doors, Base: in.base, Painted: e.set.light, Sources: sources, Clouds: obscuredLayer(in.g, ob.clouds), MagicDark: obscuredLayer(in.g, ob.dark)}); err != nil {
		e.err = fmt.Errorf("compile the map's light: %w", err)
	}
}

// sceneEntry is the compiled scene of the map: compiled here on the transaction when
// there is one, else the cache's, shared by everyone who asks for the same key.
func (s *Service) sceneEntry(ctx context.Context, tx pgx.Tx, in fogInput, sources []source, ob obscured) (*litEntry, error) {
	if tx != nil {
		// Inside a transaction, the scene is compiled here, on the transaction, and
		// kept out of the cache: joining the cache's single flight would make a
		// request that holds a connection wait for another that may be waiting for one.
		entry := &litEntry{key: litKeyOf(in, sources, ob), views: map[vision.Viewer]*vision.View{}}
		s.compileScene(ctx, tx, in, sources, ob, entry)
		if entry.err != nil {
			return nil, entry.err
		}
		return entry, nil
	}
	entry := s.lits.entry(litKeyOf(in, sources, ob))
	// The compiled scene is shared by everyone who asks for the same key, so
	// it must not depend on the first asker: if that request hangs up halfway,
	// the others would all receive its cancellation. It runs to the end under
	// a timeout of its own.
	entry.once.Do(func() {
		shared, cancel := context.WithTimeout(context.WithoutCancel(ctx), litCompileTimeout)
		defer cancel()
		s.compileScene(shared, tx, in, sources, ob, entry)
	})
	if entry.err != nil {
		s.lits.drop(entry)
		return nil, entry.err
	}
	return entry, nil
}

// familiarSightSquares is how far from the character the familiar may be for the
// player to see through it: 30 m, which is 20 squares of 1,5 m (SRD: 100 ft). The
// distance is the circle of RN-21: the straight line between the squares' centers.
const familiarSightSquares = 20

// familiarEyes works out, for each member who looks through a familiar, the viewer
// the familiar is: with its senses, at the square it stands on, while that is
// within 30 m of the character. The familiar stands where its combatant does while
// a combat runs on the map, else where its token is; a familiar with no square on
// this map is no viewer here, and neither is one beyond the range (the sight
// itself stays, and works again when it comes back). The character must stand on
// the map too.
func (s *Service) familiarEyes(ctx context.Context, tx pgx.Tx, in fogInput, members []link.PartyMember, stands map[string]grid.Square, combat link.CombatPositions) (map[string][]vision.Viewer, error) {
	var out map[string][]vision.Viewer
	creatures := &creatureSquares{s: s, tx: tx, in: in}
	for _, m := range members {
		eyes, ok, err := familiarViewer(ctx, creatures, stands, combat, m)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		if out == nil {
			out = map[string][]vision.Viewer{}
		}
		out[m.CharacterID] = append(out[m.CharacterID], eyes)
	}
	return out, nil
}

// familiarViewer is the viewer the member's familiar is, if the member looks through
// its eyes and it is in range of the character.
func familiarViewer(ctx context.Context, creatures *creatureSquares, stands map[string]grid.Square, combat link.CombatPositions, m link.PartyMember) (vision.Viewer, bool, error) {
	if m.Eyes == nil {
		return vision.Viewer{}, false, nil
	}
	mine, ok := stands[m.CharacterID]
	if !ok {
		return vision.Viewer{}, false, nil
	}
	at, ok := combat.Creatures[m.Eyes.CreatureID]
	if !ok {
		var err error
		if at, ok, err = creatures.squareOf(ctx, m.Eyes.CreatureID); err != nil || !ok {
			return vision.Viewer{}, false, err
		}
	}
	if !withinFamiliarSight(mine, at) {
		return vision.Viewer{}, false, nil
	}
	return vision.Viewer{At: at, Senses: m.Eyes.Senses}, true, nil
}

// withinFamiliarSight says whether the familiar's square is within 30 m of the
// character's: the straight line between the centers (RN-21).
func withinFamiliarSight(character, familiar grid.Square) bool {
	dc, dr := familiar.Col-character.Col, familiar.Row-character.Row
	return dc*dc+dr*dr <= familiarSightSquares*familiarSightSquares
}

// creatureSquares are the squares of a map's creature tokens, read once and only if
// a familiar needs them.
type creatureSquares struct {
	s       *Service
	tx      pgx.Tx
	in      fogInput
	squares map[string]grid.Square // nil until read
}

// squareOf is the square of a creature's token on the map, if it has one.
func (c *creatureSquares) squareOf(ctx context.Context, creatureID string) (grid.Square, bool, error) {
	if c.squares == nil {
		if err := c.load(ctx); err != nil {
			return grid.Square{}, false, err
		}
	}
	sq, ok := c.squares[creatureID]
	return sq, ok, nil
}

func (c *creatureSquares) load(ctx context.Context) error {
	c.squares = map[string]grid.Square{}
	rows, err := queriesIn(c.s.queries, c.tx).ListMapCreatureTokens(ctx, c.in.mapID)
	if err != nil {
		if c.tx != nil {
			// A failed statement aborts the transaction: carry the error up, so a
			// retryable conflict (40001) is retried, not hidden behind 25P02.
			return fmt.Errorf("read the creatures' tokens: %w", err)
		}
		c.s.logger.ErrorContext(ctx, "maps: cannot read the creatures' tokens", "error", err)
	}
	for _, t := range rows {
		c.squares[t.CreatureID] = c.in.g.SquareOf(int(t.XBp), int(t.YBp))
	}
	return nil
}

// lightSources lists the lights of a scene: the map's Luz points, and the light
// each living character's token carries, at the square the character stands on.
func (s *Service) lightSources(ctx context.Context, tx pgx.Tx, in fogInput, points []mapsdb.MapPoint, tokens []mapsdb.MapToken, where func(string) (grid.Square, bool)) ([]source, error) {
	out := pointLights(in.g, points)
	var carriers []string
	for _, t := range tokens {
		if t.CarriedLight != nil {
			carriers = append(carriers, t.CharacterID)
		}
	}
	if len(carriers) == 0 {
		return out, nil
	}
	// A dead character's token stays on the map but carries nothing.
	alive, err := s.livingCarriers(ctx, tx, in.campaignID, carriers)
	if err != nil {
		if tx != nil {
			// A failed statement aborts the transaction: carry the error up, so a
			// retryable conflict (40001) is retried, not hidden behind 25P02.
			return nil, fmt.Errorf("tell which light carriers live: %w", err)
		}
		s.logger.ErrorContext(ctx, "maps: cannot tell which light carriers live", "error", err)
		return out, nil
	}
	return append(out, s.carriedLights(tokens, alive, where)...), nil
}

// pointLights are the lights of the map's Luz points that have a radius.
func pointLights(g grid.Grid, points []mapsdb.MapPoint) []source {
	var out []source
	light := kindToDB[mapsv1.MapPointKind_MAP_POINT_KIND_LIGHT]
	for _, p := range points {
		if p.Kind == light && p.LightBrightFt != nil && p.LightDimFt != nil {
			out = append(out, source{At: g.SquareOf(int(p.XBp), int(p.YBp)), BrightFt: int(*p.LightBrightFt), DimFt: int(*p.LightDimFt)})
		}
	}
	return out
}

// livingCarriers says which of the characters are alive, by ID.
func (s *Service) livingCarriers(ctx context.Context, tx pgx.Tx, campaignID string, carriers []string) (map[string]bool, error) {
	living, err := s.characters.MapCharacters(ctx, tx, campaignID, carriers)
	if err != nil {
		return nil, err
	}
	alive := make(map[string]bool, len(living))
	for _, c := range living {
		alive[c.GetId()] = true
	}
	return alive, nil
}

// carriedLights are the lights the living characters' tokens carry, at the squares
// they stand on.
func (s *Service) carriedLights(tokens []mapsdb.MapToken, alive map[string]bool, where func(string) (grid.Square, bool)) []source {
	var out []source
	for _, t := range tokens {
		if t.CarriedLight == nil || !alive[t.CharacterID] {
			continue
		}
		preset, ok := s.rules.LightPreset(*t.CarriedLight)
		if !ok {
			continue
		}
		if sq, ok := where(t.CharacterID); ok {
			out = append(out, source{At: sq, BrightFt: preset.BrightFt, DimFt: preset.DimFt})
		}
	}
	return out
}

// litKeyOf hashes what a compiled scene depends on. The layers' revisions stand
// for the walls and the painted light: every change to them moves one.
func litKeyOf(in fogInput, sources []source, ob obscured) litKey {
	sorted := slices.Clone(sources)
	slices.SortFunc(sorted, func(a, b source) int {
		return cmpInts(a.At.Row, b.At.Row, a.At.Col, b.At.Col, a.BrightFt, b.BrightFt, a.DimFt, b.DimFt)
	})
	h := fnv.New64a()
	put := func(n int) {
		var b [8]byte
		binary.LittleEndian.PutUint64(b[:], uint64(int64(n))) //nolint:gosec // G115: a hash input
		_, _ = h.Write(b[:])
	}
	put(in.g.Columns)
	put(in.g.Rows)
	put(int(in.base))
	put(int(in.layersRev))
	put(int(in.lightRev))
	for _, src := range sorted {
		put(src.At.Col)
		put(src.At.Row)
		put(src.BrightFt)
		put(src.DimFt)
	}
	for _, list := range [][]grid.Square{ob.clouds, ob.dark} {
		squares := sortedSquares(in.g, list)
		put(len(squares)) // where one list ends and the next begins
		for _, sq := range squares {
			put(sq.Col)
			put(sq.Row)
		}
	}
	return litKey{mapID: in.mapID, sum: h.Sum64()}
}

// cmpInts compares pairs of numbers in order.
func cmpInts(n ...int) int {
	for i := 0; i+1 < len(n); i += 2 {
		if n[i] != n[i+1] {
			if n[i] < n[i+1] {
				return -1
			}
			return 1
		}
	}
	return 0
}

// viewOf is what a player sees now: their own character's view, or the union of
// every player character's with "Visão do grupo". onMap says whether any
// character is there to see: false is "Seu personagem não está neste mapa".
func (sg *sight) viewOf(userID string) (view *vision.View, onMap bool) {
	var views []*vision.View
	spectator := sg.spectates(userID)
	for _, m := range sg.members {
		if m.Dead || (!sg.group && !spectator && m.UserID != userID) {
			continue
		}
		// Looking through the familiar's eyes the character is blind to its own
		// surroundings (SRD): only the familiar's view is the player's, though the
		// other characters' views still count for the party with "Visão do grupo".
		if sq, ok := sg.stands[m.CharacterID]; ok && m.Eyes == nil {
			views = append(views, sg.entry.see(vision.Viewer{At: sq, Senses: m.Senses}))
		}
		for _, extra := range sg.extra[m.CharacterID] {
			views = append(views, sg.entry.see(extra))
		}
	}
	switch len(views) {
	case 0:
		return nil, false
	case 1:
		return views[0], true
	}
	return sg.entry.lit.Union(views...), true
}

// spectates says the player has lost every character they had: they see what the
// living party sees, as with "Visão do grupo", instead of nothing.
func (sg *sight) spectates(userID string) bool {
	dead := false
	for _, m := range sg.members {
		if m.UserID != userID {
			continue
		}
		if !m.Dead {
			return false
		}
		dead = true
	}
	return dead
}

// users lists the players with a character, in a stable order, then the
// others in extra (players who only remember: a hint about a wall painted on a
// remembered square is theirs too).
func (sg *sight) users(extra ...string) []string {
	var out []string
	for _, m := range sg.members {
		if !slices.Contains(out, m.UserID) {
			out = append(out, m.UserID)
		}
	}
	for _, u := range extra {
		if !slices.Contains(out, u) {
			out = append(out, u)
		}
	}
	return out
}

// newView is what the player knows: what they see now, plus the memory.
func (sg *sight) newView(userID string, memory *grid.Layer) *playerView {
	now, onMap := sg.viewOf(userID)
	pv := newPlayerView(sg.g, now, memory, onMap)
	pv.layers, pv.combat = &sg.entry.set, sg.combat
	pv.terrainAll = sg.openPlan
	return pv
}

// ---- what a player knows of the squares ----

// playerView is what one player knows of a map's squares: the state of each
// one now, or "remembered", or unseen.
type playerView struct {
	g grid.Grid
	// codes are the SIGHT: the state of each square for what the player's characters
	// see now or remember, row-major. Creatures, tokens and points follow these
	// (known, seenNow, pointKnown, tokenSeen) and only these.
	codes []byte
	onMap bool
	// terrainAll says the map's base light is "Claro": the player sees the whole
	// floor plan, so every square's TERRAIN (the image, the walls, the terrain, the
	// cover and the doors) is theirs, seen or not. It is read only through
	// terrainKnown and states, never by creatures or points: what is in sight is
	// still codes, and RN-10 depends on that.
	terrainAll bool
	// layers are the painted layers the view's revision also counts (nil: only the
	// states), and combat says a combat runs on the map (NPC tokens stay out).
	layers *layerSet
	combat bool
}

// newPlayerView combines what is seen now (nil for nothing) and what is
// remembered (nil for nothing).
func newPlayerView(g grid.Grid, now *vision.View, memory *grid.Layer, onMap bool) *playerView {
	pv := &playerView{g: g, codes: make([]byte, g.Squares()), onMap: onMap}
	for row := range g.Rows {
		for col := range g.Columns {
			sq := grid.Square{Col: col, Row: row}
			code := byte(now.At(sq))
			if code == stateUnseen && memory.Has(sq) {
				code = stateRemembered
			}
			pv.codes[row*g.Columns+col] = code
		}
	}
	return pv
}

// everything is the view of someone who sees every square, as the master does.
func everything(g grid.Grid) *playerView {
	pv := &playerView{g: g, codes: make([]byte, g.Squares()), onMap: true}
	for i := range pv.codes {
		pv.codes[i] = byte(vision.SeenBright)
	}
	return pv
}

func (p *playerView) code(sq grid.Square) byte {
	if !p.g.Contains(sq) {
		return stateUnseen
	}
	return p.codes[sq.Row*p.g.Columns+sq.Col]
}

// known says whether the player sees the square now or remembers it: a point
// there is theirs to receive. It is SIGHT. The terrain (image, layers) uses
// terrainKnown, and creatures and points must never use that one.
func (p *playerView) known(sq grid.Square) bool { return p.code(sq) != stateUnseen }

// terrainKnown says whether the square's terrain is the player's: the image, the
// walls, the difficult terrain, the cover and the doors. With the base light
// "Claro" every square of the grid is; otherwise it is known.
func (p *playerView) terrainKnown(sq grid.Square) bool {
	return p.g.Contains(sq) && (p.terrainAll || p.known(sq))
}

// seenNow says whether the player sees the square at this moment, as a place a
// creature can stand on (a wall seen because it touches a seen square is not).
func (p *playerView) seenNow(sq grid.Square) bool {
	c := p.code(sq)
	return c >= byte(vision.SeenGrey) && c <= byte(vision.SeenBright)
}

// pack is the states as GetMapVisionResponse says: four bits a square, the
// low half of the byte first.
//
// They are the terrain's states, what the app draws as fog: with the base light
// "Claro" a square not seen now (unseen or remembered) is packed as seen, so the app
// shows the whole plan. The sight itself (codes) is untouched.
func (p *playerView) pack() []byte {
	out := make([]byte, (len(p.codes)+1)/2)
	for i, c := range p.codes {
		if p.terrainAll && (c == stateUnseen || c == stateRemembered) {
			c = byte(vision.SeenBright)
		}
		out[i/2] |= c << (4 * (i % 2))
	}
	return out
}

// revision is a number of what the player knows: the packed states and, when the
// view has the layers, the walls, terrain and cover they receive. It changes when
// any of those does, so a wall painted on a remembered square is news too.
func (p *playerView) revision() int32 {
	// The sight (codes), not only the packed states: on a "Claro" map the states say
	// "seen" everywhere, yet a change of what is in sight moves creatures and points
	// and must still tell the player (vision_changed).
	if p.layers == nil {
		return hashOf(p.codes, p.pack())
	}
	f := filterLayers(*p.layers, p)
	return layersHash([][]byte{p.codes, p.pack(), f.terrain.Encode(), f.walls.Encode(), f.cover.Encode(), f.doors.Encode()}, nilIfBlank(f.noPass.Encode()))
}

// hashOf is a 31-bit hash of bytes, for the revisions the app compares.
func hashOf(parts ...[]byte) int32 {
	h := fnv.New32a()
	for _, part := range parts {
		_, _ = h.Write(part)
		_, _ = h.Write([]byte{0xff}) // a boundary, so [a][b] differs from [ab]
	}
	return int32(h.Sum32() >> 1)
}

// layersHash is hashOf the parts, and the can't-pass layer when something is painted on it: a map
// with none has the revision it had before the layer existed, so a number a player holds stays
// right.
func layersHash(parts [][]byte, noPass []byte) int32 {
	if len(noPass) > 0 {
		parts = append(parts, noPass)
	}
	return hashOf(parts...)
}

// pointSquares are the squares a point covers: its own, or, for a trap, the block
// of area_size squares around it (the point's square is the block's middle, the
// first of the two middle ones for an even side).
func (s *Service) pointSquares(g grid.Grid, p mapsdb.MapPoint) []grid.Square {
	at := g.SquareOf(int(p.XBp), int(p.YBp))
	if p.Kind != kindToDB[mapsv1.MapPointKind_MAP_POINT_KIND_TRAP] {
		return []grid.Square{at}
	}
	size := int(s.trapOf(p).GetAreaSize())
	if size <= 1 {
		return []grid.Square{at}
	}
	var out []grid.Square
	for row := at.Row - (size-1)/2; row < at.Row-(size-1)/2+size; row++ {
		for col := at.Col - (size-1)/2; col < at.Col-(size-1)/2+size; col++ {
			if sq := (grid.Square{Col: col, Row: row}); g.Contains(sq) {
				out = append(out, sq)
			}
		}
	}
	return out
}

// pointKnown says whether any square of the point is seen now or remembered.
func (s *Service) pointKnown(pv *playerView, p mapsdb.MapPoint) bool {
	return slices.ContainsFunc(s.pointSquares(pv.g, p), pv.known)
}

// pointSeenNow says whether any square of the point is seen now.
func (s *Service) pointSeenNow(pv *playerView, p mapsdb.MapPoint) bool {
	return slices.ContainsFunc(s.pointSquares(pv.g, p), pv.seenNow)
}

// tokenSeen says whether the viewer receives the token on a fog map: a player
// character's always (the party knows where its people are), an NPC's only on a
// square seen now, and not at all while a combat runs on the map.
func tokenSeen(pv *playerView, t mapsdb.MapToken, player bool) bool {
	// While a combat runs, the token of an NPC is the square it had before the
	// fight: it is left out, and fogcombat.go reads the combatants.
	return player || (!pv.combat && pv.seenNow(pv.g.SquareOf(int(t.XBp), int(t.YBp))))
}

// ---- what was seen ----

// memoryOf is what a player remembers of a map: nothing when there is no row,
// when it is of an older epoch (a clear came after it) or when it does not fit
// the grid.
func (s *Service) memoryOf(ctx context.Context, tx pgx.Tx, in fogInput, userID string) (*grid.Layer, error) {
	row, err := queriesIn(s.queries, tx).GetMapVisionMemory(ctx, mapsdb.GetMapVisionMemoryParams{MapID: in.mapID, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && row.Epoch != in.epoch) {
		return grid.NewLayer(in.g), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read what the player saw: %w", err)
	}
	return decodeMemory(in.g, row.Seen), nil
}

func decodeMemory(g grid.Grid, seen []byte) *grid.Layer {
	l, err := grid.DecodeLayer(g, seen)
	if err != nil {
		return grid.NewLayer(g) // bytes of another grid: forgotten
	}
	return l
}

// remember adds the squares the view sees to the memory, and says whether any
// was new. The memory only grows.
func remember(g grid.Grid, memory *grid.Layer, now *vision.View) bool {
	grew := false
	if now == nil {
		return false
	}
	for row := range g.Rows {
		for col := range g.Columns {
			if sq := (grid.Square{Col: col, Row: row}); now.Seen(sq) && !memory.Has(sq) {
				memory.Set(col, row, true)
				grew = true
			}
		}
	}
	return grew
}

// playerViewOf is what the player knows of a fog map: what their characters see
// now and what they remember. It is the one read every filtered response uses.
func (s *Service) playerViewOf(ctx context.Context, in fogInput, points []mapsdb.MapPoint, tokens []mapsdb.MapToken, userID string) (*playerView, error) {
	sg, err := s.newSight(ctx, nil, in, points, tokens)
	if err != nil {
		return nil, err
	}
	var memory *grid.Layer
	if userID == tableScreenUser {
		memory, err = s.groupMemoryOf(ctx, in, sg.members)
	} else {
		memory, err = s.memoryOf(ctx, nil, in, userID)
	}
	if err != nil {
		return nil, err
	}
	return sg.newView(userID, memory), nil
}

// tableScreenUser is the user id of the table screen's viewer: the whole table and nobody in particular. No
// account has it, so the view it gets is the group's, never a player's own.
const tableScreenUser = ""

// groupMemoryOf is what the table screen remembers of a map: the squares any member of the party remembers,
// in the map's current epoch. Without "Visão do grupo" the table is no group and remembers nothing, as it
// sees nothing.
func (s *Service) groupMemoryOf(ctx context.Context, in fogInput, members []link.PartyMember) (*grid.Layer, error) {
	memory := grid.NewLayer(in.g)
	if !in.group {
		return memory, nil
	}
	stored, err := s.queries.ListMapVisionMemory(ctx, in.mapID)
	if err != nil {
		return nil, fmt.Errorf("read what the players saw: %w", err)
	}
	party := make(map[string]bool, len(members))
	for _, m := range members {
		party[m.UserID] = true
	}
	for _, row := range stored {
		if row.Epoch != in.epoch || !party[row.UserID] {
			continue
		}
		addSquares(memory, decodeMemory(in.g, row.Seen))
	}
	return memory, nil
}

// addSquares sets in dst every square set in src (both of one grid).
func addSquares(dst, src *grid.Layer) {
	g := dst.Grid()
	for row := range g.Rows {
		for col := range g.Columns {
			if src.Get(col, row) {
				dst.Set(col, row, true)
			}
		}
	}
}

// visionState remembers, per map and player, the revision of what the player
// knew when they were last told (or last read it), so a write tells only the
// players whose view or memory changed. It lives in memory, for the last
// maxTrackedMaps maps used, the oldest leaving first. After a restart, or for a
// map it forgot, a player is told only when something they see or remember is
// known to have changed (updateVision).
type visionState struct {
	mu    sync.Mutex
	last  map[string]map[string]int32 // map ID -> user ID -> revision
	order []string                    // map IDs, least recently used first
}

// maxTrackedMaps bounds the table.
const maxTrackedMaps = 64

// swap records the revision and returns the one before, and whether there was one.
func (v *visionState) swap(mapID, userID string, rev int32) (before int32, had bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.last == nil {
		v.last = map[string]map[string]int32{}
	}
	users := v.last[mapID]
	if users == nil {
		users = map[string]int32{}
		v.last[mapID] = users
	} else {
		v.order = slices.DeleteFunc(v.order, func(id string) bool { return id == mapID })
	}
	v.order = append(v.order, mapID)
	for len(v.order) > maxTrackedMaps {
		delete(v.last, v.order[0])
		v.order = v.order[1:]
	}
	before, had = users[userID]
	users[userID] = rev
	return before, had
}

func (v *visionState) forget(mapID string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	delete(v.last, mapID)
	v.order = slices.DeleteFunc(v.order, func(id string) bool { return id == mapID })
}

// visionLocks serialize the refreshes of one map (a refresh reads a player's memory,
// adds to it and writes it back): a striped set, so two maps rarely wait for
// each other and nothing grows with the number of maps.
type visionLocks [32]sync.Mutex

func (l *visionLocks) of(mapID string) *sync.Mutex {
	h := fnv.New32a()
	_, _ = h.Write([]byte(mapID))
	return &l[h.Sum32()%uint32(len(l))]
}

// refreshVision is called after a write that changes what someone sees (a token
// placed or moved, a carried light, a wall or light painted, a Luz point, the
// fog's settings, a combat move): it works out again what each player of the map
// sees, adds it to their memory, and tells with `vision_changed` the players
// whose view or memory changed, and the players who see one of the squares in
// watch now (where an NPC token was or is: its move is news to those who see it,
// and to nobody else). It does nothing for a map without fog. Slices 9.6 and 9.7
// call it after a combat move (CombatMoved).
//
// It runs after the write's commit and never fails the write: an error is logged
// (it holds no personal data), and the app reads the map again after any
// reconnection.
func (s *Service) refreshVision(ctx context.Context, campaignID, mapID string, watch ...grid.Square) {
	ctx = context.WithoutCancel(ctx)
	// What the players' tiles depend on is about to change, and has when this
	// returns: drop their kept tiles at both ends.
	s.tiles.views.drop(mapID)
	defer s.tiles.views.drop(mapID)
	if err := s.updateVision(ctx, campaignID, mapID, watch); err != nil {
		s.logger.ErrorContext(ctx, "maps: cannot update what the players see", "error", err)
	}
}

// CombatMoved is the hook for a combat move (slices 9.6 and 9.7): the play module
// calls it, after the commit that moved a combatant on a map, so that the players
// who see the old or the new square are told, and what the move showed is
// remembered. from and to are the combatant's squares; npc says whether it is an
// NPC (a player character's move is public, and its view change is what is sent).
func (s *Service) CombatMoved(ctx context.Context, campaignID, mapID string, npc bool, from, to grid.Square) {
	if npc {
		s.refreshVision(ctx, campaignID, mapID, from, to)
		return
	}
	s.refreshVision(ctx, campaignID, mapID)
}

func (s *Service) updateVision(ctx context.Context, campaignID, mapID string, watch []grid.Square) error {
	lock := s.visionLocks.of(mapID)
	lock.Lock()
	defer lock.Unlock()
	row, ok, err := s.mapToRemember(ctx, campaignID, mapID)
	if err != nil || !ok {
		return err
	}
	in, sg, err := s.visionSight(ctx, row)
	if err != nil {
		return err
	}
	memories, rememberers, err := s.storedMemories(ctx, in)
	if err != nil {
		return err
	}
	var changed []string
	for _, userID := range sg.users(rememberers...) {
		seen, had := memories[userID]
		told, err := s.refreshUser(ctx, in, sg, userID, seen, had, watch)
		if err != nil {
			return err
		}
		if told {
			changed = append(changed, userID)
		}
	}
	if len(changed) > 0 {
		s.live.PublishToUsersCoalesced(campaignID, changed, visionKey(mapID), visionChangedEvent(mapID))
	}
	return nil
}

// mapToRemember is the row of the map whose vision is worked out again; ok is false
// (and the server forgets what it kept of the map) when there is nothing to work out:
// no such map, no fog, or a map the players do not see.
func (s *Service) mapToRemember(ctx context.Context, campaignID, mapID string) (row mapsdb.Map, ok bool, err error) {
	row, err = s.queries.GetMap(ctx, mapsdb.GetMapParams{CampaignID: campaignID, ID: mapID})
	if errors.Is(err, pgx.ErrNoRows) {
		s.seen.forget(mapID)
		return row, false, nil
	}
	if err != nil {
		return row, false, fmt.Errorf("find the map: %w", err)
	}
	if !fogged(row) {
		s.seen.forget(mapID)
		return row, false, nil
	}
	// What the players have not been shown cannot be remembered: a map the master
	// prepares in private records nothing until the players see it (refreshVision
	// is called when it becomes visible).
	current, err := s.currentMap(ctx, campaignID)
	if err != nil {
		return row, false, err
	}
	if !playersSee(mapID, row.RevealedAt, current) {
		s.seen.forget(mapID)
		return row, false, nil
	}
	return row, true, nil
}

// visionSight reads what the scene of the fog map is made of and builds it.
func (s *Service) visionSight(ctx context.Context, row mapsdb.Map) (fogInput, *sight, error) {
	size, err := s.queries.GetMapGrid(ctx, mapsdb.GetMapGridParams{CampaignID: row.CampaignID, ID: row.ID})
	if err != nil {
		return fogInput{}, nil, fmt.Errorf("read the map's grid: %w", err)
	}
	in := fogInputOfRow(row, size.ImageWidth, size.ImageHeight)
	points, err := s.queries.ListMapPoints(ctx, row.ID)
	if err != nil {
		return in, nil, fmt.Errorf("list the points: %w", err)
	}
	tokens, err := s.queries.ListMapTokens(ctx, row.ID)
	if err != nil {
		return in, nil, fmt.Errorf("list the tokens: %w", err)
	}
	sg, err := s.newSight(ctx, nil, in, points, tokens)
	return in, sg, err
}

// storedMemories reads what the players saw of the map: the bitmaps of the map's
// current epoch by user, and every user who has a memory, of any epoch.
func (s *Service) storedMemories(ctx context.Context, in fogInput) (map[string][]byte, []string, error) {
	stored, err := s.queries.ListMapVisionMemory(ctx, in.mapID)
	if err != nil {
		return nil, nil, fmt.Errorf("read what the players saw: %w", err)
	}
	memories := make(map[string][]byte, len(stored))
	var rememberers []string
	for _, m := range stored {
		rememberers = append(rememberers, m.UserID)
		if m.Epoch == in.epoch {
			memories[m.UserID] = m.Seen
		}
	}
	return memories, rememberers, nil
}

// refreshUser adds what the user sees now to their memory (seen, when they had one in
// this epoch) and says whether they are to be told that their view changed.
func (s *Service) refreshUser(ctx context.Context, in fogInput, sg *sight, userID string, seen []byte, had bool, watch []grid.Square) (bool, error) {
	memory := grid.NewLayer(in.g)
	if had {
		memory = decodeMemory(in.g, seen)
	}
	now, _ := sg.viewOf(userID)
	grew := remember(in.g, memory, now)
	if grew {
		if _, err := s.rememberFor(ctx, in, userID, memory); err != nil {
			return false, err
		}
	}
	pv := sg.newView(userID, memory)
	rev := pv.revision()
	before, known := s.seen.swap(in.mapID, userID, rev)
	// A player the server has no record of (a restart, or a map it let go) is
	// told only if something is known to have changed for them.
	return (known && before != rev) || (!known && grew) || slices.ContainsFunc(watch, pv.seenNow), nil
}

// rememberFor writes a player's memory of the map, built for the epoch in: it
// writes only while the map is still in that epoch, decided inside the
// statement, so a clear that came meanwhile (a new grid or image, "Esquecer o que
// foi visto") is never undone. It says whether the memory was written.
func (s *Service) rememberFor(ctx context.Context, in fogInput, userID string, memory *grid.Layer) (bool, error) {
	var wrote int64
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) (err error) {
		wrote, err = s.queries.WithTx(tx).UpsertMapVisionMemory(ctx, mapsdb.UpsertMapVisionMemoryParams{
			MapID: in.mapID, UserID: userID, Seen: memory.Encode(), Epoch: in.epoch, UpdatedAt: s.now(),
		})
		return err
	})
	if err != nil {
		return false, fmt.Errorf("remember what the player saw: %w", err)
	}
	return wrote > 0, nil
}
