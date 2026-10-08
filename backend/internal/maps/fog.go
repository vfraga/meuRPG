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

// What each player sees of a map with the fog of war on (MR-036, RN-10, Etapa 9,
// D6). Everything a player receives about such a map goes through here: this is
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
	// a player's reads (the combatants are slice 9.7's).
	combat bool
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
	// where a character stands: its square in the combat, else its token's.
	tokenAt := make(map[string]grid.Square, len(tokens))
	for _, t := range tokens {
		tokenAt[t.CharacterID] = in.g.SquareOf(int(t.XBp), int(t.YBp))
	}
	where := func(id string) (grid.Square, bool) {
		if sq, ok := combat.Positions[id]; ok {
			return sq, true
		}
		sq, ok := tokenAt[id]
		return sq, ok
	}

	sources, err := s.lightSources(ctx, tx, in, points, tokens, where)
	if err != nil {
		return nil, err
	}
	sg := &sight{g: in.g, members: members, stands: map[string]grid.Square{}, group: in.group, combat: combat.Running}
	for _, m := range members {
		if sq, ok := where(m.CharacterID); ok {
			sg.stands[m.CharacterID] = sq
		}
	}

	if sg.extra, err = s.familiarEyes(ctx, tx, in, members, sg.stands, combat); err != nil {
		return nil, err
	}

	compile := func(ctx context.Context, e *litEntry) {
		stored, err := queriesIn(s.queries, tx).GetMapLayers(ctx, in.mapID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			e.err = fmt.Errorf("read the layers: %w", err)
			return
		}
		e.set = loadLayers(stored, in.g)
		if e.lit, err = vision.Compile(vision.Scene{Grid: in.g, Walls: e.set.walls, Doors: e.set.doors, Base: in.base, Painted: e.set.light, Sources: sources}); err != nil {
			e.err = fmt.Errorf("compile the map's light: %w", err)
		}
	}
	if tx != nil {
		// Inside a transaction, the scene is compiled here, on the transaction, and
		// kept out of the cache: joining the cache's single flight would make a
		// request that holds a connection wait for another that may be waiting for one.
		sg.entry = &litEntry{key: litKeyOf(in, sources), views: map[vision.Viewer]*vision.View{}}
		compile(ctx, sg.entry)
		if sg.entry.err != nil {
			return nil, sg.entry.err
		}
		return sg, nil
	}
	sg.entry = s.lits.entry(litKeyOf(in, sources))
	// The compiled scene is shared by everyone who asks for the same key, so
	// it must not depend on the first asker: if that request hangs up halfway,
	// the others would all receive its cancellation. It runs to the end under
	// a timeout of its own.
	sg.entry.once.Do(func() {
		shared, cancel := context.WithTimeout(context.WithoutCancel(ctx), litCompileTimeout)
		defer cancel()
		compile(shared, sg.entry)
	})
	if sg.entry.err != nil {
		s.lits.drop(sg.entry)
		return nil, sg.entry.err
	}
	return sg, nil
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
	var tokens map[string]grid.Square // the creatures' tokens, read once and only if needed
	for _, m := range members {
		if m.Eyes == nil {
			continue
		}
		mine, ok := stands[m.CharacterID]
		if !ok {
			continue
		}
		at, ok := combat.Creatures[m.Eyes.CreatureID]
		if !ok {
			if tokens == nil {
				tokens = map[string]grid.Square{}
				rows, err := queriesIn(s.queries, tx).ListMapCreatureTokens(ctx, in.mapID)
				if err != nil {
					if tx != nil {
						// A failed statement aborts the transaction: carry the error up, so a
						// retryable conflict (40001) is retried, not hidden behind 25P02.
						return nil, fmt.Errorf("read the creatures' tokens: %w", err)
					}
					s.logger.ErrorContext(ctx, "maps: cannot read the creatures' tokens", "error", err)
				}
				for _, t := range rows {
					tokens[t.CreatureID] = in.g.SquareOf(int(t.XBp), int(t.YBp))
				}
			}
			if at, ok = tokens[m.Eyes.CreatureID]; !ok {
				continue
			}
		}
		dc, dr := at.Col-mine.Col, at.Row-mine.Row
		if dc*dc+dr*dr > familiarSightSquares*familiarSightSquares {
			continue
		}
		if out == nil {
			out = map[string][]vision.Viewer{}
		}
		out[m.CharacterID] = append(out[m.CharacterID], vision.Viewer{At: at, Senses: m.Eyes.Senses})
	}
	return out, nil
}

// lightSources lists the lights of a scene: the map's Luz points, and the light
// each living character's token carries, at the square the character stands on.
func (s *Service) lightSources(ctx context.Context, tx pgx.Tx, in fogInput, points []mapsdb.MapPoint, tokens []mapsdb.MapToken, where func(string) (grid.Square, bool)) ([]source, error) {
	var out []source
	light := kindToDB[mapsv1.MapPointKind_MAP_POINT_KIND_LIGHT]
	for _, p := range points {
		if p.Kind == light && p.LightBrightFt != nil && p.LightDimFt != nil {
			out = append(out, source{At: in.g.SquareOf(int(p.XBp), int(p.YBp)), BrightFt: int(*p.LightBrightFt), DimFt: int(*p.LightDimFt)})
		}
	}
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
	living, err := s.characters.MapCharacters(ctx, tx, in.campaignID, carriers)
	if err != nil {
		if tx != nil {
			// A failed statement aborts the transaction: carry the error up, so a
			// retryable conflict (40001) is retried, not hidden behind 25P02.
			return nil, fmt.Errorf("tell which light carriers live: %w", err)
		}
		s.logger.ErrorContext(ctx, "maps: cannot tell which light carriers live", "error", err)
		return out, nil
	}
	alive := make(map[string]bool, len(living))
	for _, c := range living {
		alive[c.GetId()] = true
	}
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
	return out, nil
}

// litKeyOf hashes what a compiled scene depends on. The layers' revisions stand
// for the walls and the painted light: every change to them moves one.
func litKeyOf(in fogInput, sources []source) litKey {
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
	for _, m := range sg.members {
		if !sg.group && m.UserID != userID {
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

// users lists the players with a living character, in a stable order, then the
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
	return pv
}

// ---- what a player knows of the squares ----

// playerView is what one player knows of a map's squares: the state of each
// one now, or "remembered", or unseen.
type playerView struct {
	g     grid.Grid
	codes []byte // one state code a square, row-major
	onMap bool
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
// there is theirs to receive, and so is a layer's square.
func (p *playerView) known(sq grid.Square) bool { return p.code(sq) != stateUnseen }

// seenNow says whether the player sees the square at this moment, as a place a
// creature can stand on (a wall seen because it touches a seen square is not).
func (p *playerView) seenNow(sq grid.Square) bool {
	c := p.code(sq)
	return c >= byte(vision.SeenGrey) && c <= byte(vision.SeenBright)
}

// pack is the states as GetMapVisionResponse says: four bits a square, the
// low half of the byte first.
func (p *playerView) pack() []byte {
	out := make([]byte, (len(p.codes)+1)/2)
	for i, c := range p.codes {
		out[i/2] |= c << (4 * (i % 2))
	}
	return out
}

// revision is a number of what the player knows: the packed states and, when the
// view has the layers, the walls, terrain and cover they receive. It changes when
// any of those does, so a wall painted on a remembered square is news too.
func (p *playerView) revision() int32 {
	if p.layers == nil {
		return hashOf(p.pack())
	}
	f := filterLayers(*p.layers, p)
	return hashOf(p.pack(), f.terrain.Encode(), f.walls.Encode(), f.cover.Encode(), f.doors.Encode())
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
	// fight: it is left out, and the combatants are slice 9.7's.
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
	memory, err := s.memoryOf(ctx, nil, in, userID)
	if err != nil {
		return nil, err
	}
	return sg.newView(userID, memory), nil
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
	row, err := s.queries.GetMap(ctx, mapsdb.GetMapParams{CampaignID: campaignID, ID: mapID})
	if errors.Is(err, pgx.ErrNoRows) {
		s.seen.forget(mapID)
		return nil
	}
	if err != nil {
		return fmt.Errorf("find the map: %w", err)
	}
	if !fogged(row) {
		s.seen.forget(mapID)
		return nil
	}
	// What the players have not been shown cannot be remembered: a map the master
	// prepares in private records nothing until the players see it (refreshVision
	// is called when it becomes visible).
	current, err := s.currentMap(ctx, campaignID)
	if err != nil {
		return err
	}
	if !playersSee(mapID, row.RevealedAt, current) {
		s.seen.forget(mapID)
		return nil
	}
	size, err := s.queries.GetMapGrid(ctx, mapsdb.GetMapGridParams{CampaignID: campaignID, ID: mapID})
	if err != nil {
		return fmt.Errorf("read the map's grid: %w", err)
	}
	in := fogInputOfRow(row, size.ImageWidth, size.ImageHeight)
	points, err := s.queries.ListMapPoints(ctx, mapID)
	if err != nil {
		return fmt.Errorf("list the points: %w", err)
	}
	tokens, err := s.queries.ListMapTokens(ctx, mapID)
	if err != nil {
		return fmt.Errorf("list the tokens: %w", err)
	}
	sg, err := s.newSight(ctx, nil, in, points, tokens)
	if err != nil {
		return err
	}
	stored, err := s.queries.ListMapVisionMemory(ctx, mapID)
	if err != nil {
		return fmt.Errorf("read what the players saw: %w", err)
	}
	memories := make(map[string][]byte, len(stored))
	var rememberers []string
	for _, m := range stored {
		rememberers = append(rememberers, m.UserID)
		if m.Epoch == in.epoch {
			memories[m.UserID] = m.Seen
		}
	}
	var changed []string
	for _, userID := range sg.users(rememberers...) {
		memory := grid.NewLayer(in.g)
		if seen, ok := memories[userID]; ok {
			memory = decodeMemory(in.g, seen)
		}
		now, _ := sg.viewOf(userID)
		grew := remember(in.g, memory, now)
		if grew {
			if _, err := s.rememberFor(ctx, in, userID, memory); err != nil {
				return err
			}
		}
		pv := sg.newView(userID, memory)
		rev := pv.revision()
		before, had := s.seen.swap(mapID, userID, rev)
		// A player the server has no record of (a restart, or a map it let go) is
		// told only if something is known to have changed for them.
		if (had && before != rev) || (!had && grew) || slices.ContainsFunc(watch, pv.seenNow) {
			changed = append(changed, userID)
		}
	}
	if len(changed) > 0 {
		s.live.PublishToUsersCoalesced(campaignID, changed, visionKey(mapID), visionChangedEvent(mapID))
	}
	return nil
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
