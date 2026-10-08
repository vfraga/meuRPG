package play

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1/mapsv1connect"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/maps"
	"github.com/PuraFome/meuRPG/backend/internal/platform/httpserver"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// Combat movement, reach and cover (MR-034, RN-21, RN-20, Etapa 9, slice 9.6).
// These tests need the database (MEURPG_TEST_DATABASE_URL). The fixture is the
// cave "A caverna do Vale Seco" of the Etapa 9 artboards (24 x 16 squares; the
// numbers come from the artboards' cave.py): the party in the entrance cave,
// the goblins in the torch-lit guard room, rubble, crates and a column.

// caveRows is the cave: # wall, . floor, : rubble (difficult terrain), h crates
// (half cover, can be crossed), q a stone column (three-quarters cover, blocks
// movement).
var caveRows = []string{
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

// caveTerrain is the TerrainSource of these tests: the cave, plus the walls a
// test adds.
type caveTerrain struct {
	mu    sync.Mutex
	extra []grid.Square
}

func (c *caveTerrain) addWalls(sq ...grid.Square) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.extra = append(c.extra, sq...)
}

func (c *caveTerrain) Terrain(context.Context, pgx.Tx, string, string) (grid.Terrain, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	g := grid.Grid{Columns: 24, Rows: 16}
	t := grid.Terrain{Grid: g, Walls: grid.NewLayer(g), Difficult: grid.NewLayer(g), Cover: grid.NewCoverLayer(g)}
	for row, line := range caveRows {
		for col, ch := range line {
			switch ch {
			case '#':
				t.Walls.Set(col, row, true)
			case ':':
				t.Difficult.Set(col, row, true)
			case 'h':
				t.Cover.Set(col, row, grid.CoverHalf)
			case 'q':
				t.Cover.Set(col, row, grid.CoverThreeQuarters)
			}
		}
	}
	for _, sq := range c.extra {
		t.Walls.Set(sq.Col, sq.Row, true)
	}
	return t, nil
}

// newMapOf is newMap for an image of width x height pixels.
func (h *harness) newMapOf(campaignID string, columns, width, height int) string {
	h.t.Helper()
	imageID, mapID := newKey(), newKey()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO gallery_images (id, campaign_id, name, content_type, width, height, byte_size, created_at)
		  VALUES ($1, $2, 'Caverna', 'image/png', $3, $4, 100, now())`, []any{imageID, campaignID, width, height}},
		{`INSERT INTO maps (id, campaign_id, name, image_id, grid_columns, created_at, updated_at)
		  VALUES ($1, $2, 'A caverna do Vale Seco', $3, $4, now(), now())`, []any{mapID, campaignID, imageID, columns}},
	} {
		if _, err := h.pool.Exec(h.t.Context(), q.sql, q.args...); err != nil {
			h.t.Fatalf("insert map: %v", err)
		}
	}
	return mapID
}

// sizedNPC creates a basic NPC of a size, with a sword at +4 (1d6+2, "basic:0").
func (u *user) sizedNPC(t *testing.T, campaignID, name string, hp, ac int32, size rulesv1.CreatureSize) *charactersv1.Character {
	t.Helper()
	sheet := &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Basic{Basic: &charactersv1.BasicSheet{
		HitPointsMax: hp, ArmorClass: ac, SpeedFt: 30, Size: size,
		Attacks: []*charactersv1.BasicAttack{{
			Name: "Cimitarra", AttackBonus: 4, DamageDiceCount: 1, DamageDiceSides: 6, DamageBonus: 2, DamageType: charactersv1.DamageType_DAMAGE_TYPE_SLASHING,
		}},
	}}}
	res, err := u.characters.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
		CampaignId: campaignID, Kind: charactersv1.CharacterKind_CHARACTER_KIND_MINION, Name: name, Sheet: sheet,
	}))
	if err != nil {
		t.Fatalf("CreateCharacter(%s) error = %v", name, err)
	}
	return res.Msg.GetCharacter()
}

// cave is the fixture: Toren (human fighter, Strength 16 with the race's bonus), Pensantus (gnome
// wizard) and Brisa (halfling cleric, Strength 8) in the entrance cave, a squire
// who is the party's ally, three Small goblins, the Capitão Goblin and an ogre.
type cave struct {
	*armed
	terrain *caveTerrain
	goblins *charactersv1.Character // Small
	ogre    *charactersv1.Character // Large
	squire  *charactersv1.Character // the master marks him "Aliado"
}

func newCave(t *testing.T) *cave {
	t.Helper()
	a := newArmedWith(t, func(a *armed) {
		scores := func(str, dex, con, intl int32) *rulesv1.AbilityScores {
			return &rulesv1.AbilityScores{Strength: str, Dexterity: dex, Constitution: con, Intelligence: intl, Wisdom: 10, Charisma: 8}
		}
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 2, scores(15, 13, 14, 10), []string{battleaxe}, nil) // Strength 16 with the human's +1
		a.pens = a.ana.caster(t, a.campaignID, "Pensantus", "class:wizard", "race:gnome", 3, scores(10, 14, 12, 16), nil, []string{fireBolt},
			[]string{burningHands, magicMissileSpell, shieldSpell}, []string{burningHands, magicMissileSpell, shieldSpell})
		a.bri = a.bia.caster(t, a.campaignID, "Brisa", "class:cleric", "race:halfling", 2,
			&rulesv1.AbilityScores{Strength: 8, Dexterity: 16, Constitution: 14, Intelligence: 10, Wisdom: 16, Charisma: 8}, []string{maceKey}, []string{sacredFlame}, nil, []string{cureWounds})
	})
	c := &cave{armed: a, terrain: &caveTerrain{}}
	a.h.svc.terrain = c.terrain
	c.goblins = a.master.sizedNPC(t, a.campaignID, "Goblin", 7, 12, rulesv1.CreatureSize_CREATURE_SIZE_SMALL)
	c.ogre = a.master.sizedNPC(t, a.campaignID, "Ogro", 59, 11, rulesv1.CreatureSize_CREATURE_SIZE_LARGE)
	c.squire = a.master.sizedNPC(t, a.campaignID, "Escudeiro", 11, 13, rulesv1.CreatureSize_CREATURE_SIZE_UNSPECIFIED)
	a.mapID = a.h.newMapOf(a.campaignID, 24, 1200, 800)
	if _, err := a.master.play.SetCurrentMap(t.Context(), connect.NewRequest(&playv1.SetCurrentMapRequest{CampaignId: a.campaignID, MapId: a.mapID})); err != nil {
		t.Fatalf("SetCurrentMap() error = %v", err)
	}
	return c
}

// fight starts the combat in the cave: Toren first, then Pensantus and Brisa,
// then the NPCs (all revealed, the squire the party's ally). The party is in the
// entrance cave, as in the artboards, and the enemies far from it.
func (c *cave) fight(t *testing.T) *playv1.Encounter {
	t.Helper()
	e := c.start(t, plan{
		npcs: []*playv1.Participant{
			{CharacterId: c.capitao.GetId()}, {CharacterId: c.goblins.GetId(), Count: 3}, {CharacterId: c.ogre.GetId()}, {CharacterId: c.squire.GetId()},
		},
		npcRolls: []int{2, 2, 2, 2, 2, 2},
		players:  map[string]int32{"Toren": 18, "Pensantus": 10, "Brisa": 1},
		reveal:   []string{"Capitão Goblin", "Goblin 1", "Goblin 2", "Goblin 3", "Ogro", "Escudeiro"},
		at: map[string][2]int32{
			"Toren": {6, 7}, "Pensantus": {5, 8}, "Brisa": {4, 7}, "Escudeiro": {3, 8},
			"Goblin 1": {18, 5}, "Goblin 2": {20, 7}, "Goblin 3": {21, 3}, "Capitão Goblin": {12, 13}, "Ogro": {22, 9},
		},
	})
	c.side(t, "Escudeiro", playv1.CombatantSide_COMBATANT_SIDE_PARTY)
	return e
}

func (c *cave) side(t *testing.T, label string, side playv1.CombatantSide) {
	t.Helper()
	if _, err := c.master.combat.SetCombatantSide(t.Context(), connect.NewRequest(&playv1.SetCombatantSideRequest{
		CampaignId: c.campaignID, EncounterId: c.get(t, c.master).GetId(), CombatantId: c.id(t, label), IdempotencyKey: newKey(), Side: side,
	})); err != nil {
		t.Fatalf("SetCombatantSide(%s) error = %v", label, err)
	}
}

func (c *cave) mark(t *testing.T, label string, cover playv1.CoverDegree) {
	t.Helper()
	if _, err := c.master.combat.SetCombatantCover(t.Context(), connect.NewRequest(&playv1.SetCombatantCoverRequest{
		CampaignId: c.campaignID, EncounterId: c.get(t, c.master).GetId(), CombatantId: c.id(t, label), IdempotencyKey: newKey(), Cover: cover,
	})); err != nil {
		t.Fatalf("SetCombatantCover(%s) error = %v", label, err)
	}
}

// move calls MoveCombatant as u.
func (c *cave) move(t *testing.T, u *user, label string, col, row int32, edit ...func(*playv1.MoveCombatantRequest)) (*playv1.Encounter, error) {
	t.Helper()
	req := &playv1.MoveCombatantRequest{
		CampaignId: c.campaignID, EncounterId: c.get(t, c.master).GetId(), CombatantId: c.id(t, label), IdempotencyKey: newKey(), Col: col, Row: row,
	}
	for _, e := range edit {
		e(req)
	}
	res, err := u.combat.MoveCombatant(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetEncounter(), nil
}

// mustMove is a move that must work. The master's moves here are setup, not
// play: the master's move of someone on turn that leaves an enemy's reach is a
// move like any, which offers the enemy an opportunity attack (slice 9.6b), so
// the setup is a forced move, which never provokes. The tests of the offers
// themselves move with moveOffering.
func (c *cave) mustMove(t *testing.T, u *user, label string, col, row int32, edit ...func(*playv1.MoveCombatantRequest)) *playv1.Encounter {
	t.Helper()
	if u == c.master {
		edit = append(edit, func(r *playv1.MoveCombatantRequest) { r.Forced = true })
	}
	e, err := c.move(t, u, label, col, row, edit...)
	if err != nil {
		t.Fatalf("MoveCombatant(%s to %d,%d) error = %v", label, col, row, err)
	}
	return e
}

func jumpTo(kind playv1.JumpKind) func(*playv1.MoveCombatantRequest) {
	return func(r *playv1.MoveCombatantRequest) { r.Jump = kind }
}

func highJump(dft int32) func(*playv1.MoveCombatantRequest) {
	return func(r *playv1.MoveCombatantRequest) { r.Jump, r.JumpHeightDft = playv1.JumpKind_JUMP_KIND_HIGH, dft }
}

// undoMove takes back the last action as the master.
func (c *cave) undoLast(t *testing.T) {
	t.Helper()
	e := c.get(t, c.master)
	last := c.log(t, c.master, e).GetUndoableEventId()
	if last == "" {
		t.Fatal("nothing to undo")
	}
	if err := c.undo(t, c.master, e, last); err != nil {
		t.Fatalf("UndoLastAction() error = %v", err)
	}
}

// who is the combatant as u sees it.
func (c *cave) who(t *testing.T, u *user, label string) *playv1.Combatant {
	t.Helper()
	return byLabel(t, c.get(t, u), label)
}

func (c *cave) options(t *testing.T, u *user, label string) (*playv1.GetMoveOptionsResponse, error) {
	t.Helper()
	res, err := u.combat.GetMoveOptions(t.Context(), connect.NewRequest(&playv1.GetMoveOptionsRequest{
		CampaignId: c.campaignID, EncounterId: c.get(t, c.master).GetId(), CombatantId: c.id(t, label),
	}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

var (
	reasonMoveBlocked = playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_MOVE_BLOCKED
	reasonEnemy       = playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ENEMY_IN_THE_WAY
	reasonCoverTotal  = playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TARGET_COVER_TOTAL
)

// TestRN21_TheCircleCostsTheExactLine: a move costs the length of the straight
// line in tenths of a foot (a diagonal step is 7,1 ft), so with 30 ft a player
// reaches (4, 4) diagonally (28,3 ft) but not (5, 5) (35,4 ft); the feet fields
// are those rounded down; the master moves anyone anywhere for free; an undo
// gives the movement back.
func TestRN21_TheCircleCostsTheExactLine(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.fight(t)

	toren := c.who(t, c.caio, "Toren")
	if toren.GetSpeedDft() != 300 || toren.GetMovementLeftDft() != 300 || toren.GetSide() != playv1.CombatantSide_COMBATANT_SIDE_PARTY ||
		toren.GetSize() != rulesv1.CreatureSize_CREATURE_SIZE_MEDIUM {
		t.Fatalf("Toren at the start = %v, want 30 ft (300 dft), the party's side, Medium", toren)
	}

	// One square straight is 5 ft; one diagonal step 7,1 ft (71 dft, 7 ft).
	c.mustMove(t, c.caio, "Toren", 9, 7)
	c.mustMove(t, c.caio, "Toren", 10, 8)
	toren = c.who(t, c.caio, "Toren")
	if toren.GetMovementUsedDft() != 221 || toren.GetMovementUsedFt() != 22 || toren.GetMovementLeftDft() != 79 || toren.GetMovementLeftFt() != 7 {
		t.Fatalf("after 3 squares and a diagonal step = used %d dft / %d ft, left %d dft / %d ft; want 221 / 22, 79 / 7",
			toren.GetMovementUsedDft(), toren.GetMovementUsedFt(), toren.GetMovementLeftDft(), toren.GetMovementLeftFt())
	}
	c.undoLast(t)
	c.undoLast(t)
	if toren = c.who(t, c.caio, "Toren"); toren.GetMovementUsedDft() != 0 || toren.GetCol() != 6 || toren.GetRow() != 7 {
		t.Fatalf("after the undos = %v, want back on (6, 7) with nothing walked", toren)
	}

	// In the guard room: (4, 4) away diagonally is 28,3 ft, (5, 5) is 35,4 ft.
	c.mustMove(t, c.master, "Toren", 17, 3)
	_, err := c.move(t, c.caio, "Toren", 22, 8)
	b := wantEncounterBlocked(t, err, reasonTooFar)
	if b.GetMissingDft() != 54 || b.GetMissingFt() != 6 {
		t.Errorf("missing = %d dft / %d ft, want 54 / 6 (35,4 ft against 30)", b.GetMissingDft(), b.GetMissingFt())
	}
	c.mustMove(t, c.caio, "Toren", 21, 7)
	if toren = c.who(t, c.caio, "Toren"); toren.GetMovementUsedDft() != 283 || toren.GetMovementUsedFt() != 28 || toren.GetMovementLeftFt() != 1 {
		t.Errorf("after (4, 4) = %v, want 283 dft used (28 ft), 1 ft left", toren)
	}

	// The master moves anyone anywhere, through walls, and nothing is spent.
	c.mustMove(t, c.master, "Pensantus", 9, 13)
	c.mustMove(t, c.master, "Pensantus", 1, 1)
	if got := c.who(t, c.master, "Pensantus"); got.GetMovementUsedDft() != 0 || got.GetCol() != 1 || got.GetRow() != 1 {
		t.Errorf("after the master's moves = %v, want (1, 1) with no feet spent", got)
	}
}

// TestRN21_WallsColumnsAndSqueezesBlockAMove: a straight move that crosses a
// wall, a three-quarters cover square or squeezes between two walls that touch
// at a corner is refused (MOVE_BLOCKED): the player moves in parts.
func TestRN21_WallsColumnsAndSqueezesBlockAMove(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.fight(t)

	_, err := c.move(t, c.caio, "Toren", 6, 5) // (6, 6) is floor, (6, 5) is rock
	wantEncounterBlocked(t, err, reasonMoveBlocked)
	// Around a corner, in parts, it goes.
	c.mustMove(t, c.caio, "Toren", 6, 6)

	c.mustMove(t, c.master, "Toren", 19, 4)
	_, err = c.move(t, c.caio, "Toren", 21, 4) // the column, three-quarters cover, is on the line
	wantEncounterBlocked(t, err, reasonMoveBlocked)
	_, err = c.move(t, c.caio, "Toren", 20, 4) // and cannot be entered
	wantEncounterBlocked(t, err, reasonMoveBlocked)
	if got := c.who(t, c.caio, "Toren"); got.GetMovementUsedDft() != 50 {
		t.Errorf("Toren walked %d dft, want only the 50 of the first part (a refused move costs nothing)", got.GetMovementUsedDft())
	}

	// A squeeze: two walls touching at a corner stop the diagonal step between them.
	c.terrain.addWalls(grid.Square{Col: 11, Row: 7}, grid.Square{Col: 10, Row: 8})
	c.mustMove(t, c.master, "Toren", 10, 7)
	_, err = c.move(t, c.caio, "Toren", 11, 8)
	wantEncounterBlocked(t, err, reasonMoveBlocked)
}

// TestRN21_DifficultTerrainAndOtherCreaturesCostMore: rubble costs 5 ft more for
// each square the line enters, once per square (an ally standing on rubble is
// +5, not +10); a square holding an ally can be passed at the same price; an
// enemy cannot be passed unless the two are two sizes apart; nobody ends in
// another creature's square; a flier ignores the rubble.
func TestRN21_DifficultTerrainAndOtherCreaturesCostMore(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.fight(t)

	// cave.py cost 3 8 6 10: 18,03 ft and three rubble squares make 33,03 ft: Toren
	// has 30, the Dash gives him 60.
	c.mustMove(t, c.master, "Escudeiro", 1, 8)
	c.mustMove(t, c.master, "Toren", 3, 8)
	_, err := c.move(t, c.caio, "Toren", 6, 10)
	if b := wantEncounterBlocked(t, err, reasonTooFar); b.GetMissingDft() != 30 || b.GetMissingFt() != 3 {
		t.Errorf("missing = %d dft / %d ft, want 30 / 3 (330 dft against 300)", b.GetMissingDft(), b.GetMissingFt())
	}
	if err := markDashed(t.Context(), c.h.svc.queries, c.id(t, "Toren")); err != nil {
		t.Fatalf("markDashed() error = %v", err)
	}
	c.mustMove(t, c.caio, "Toren", 6, 10)
	if got := c.who(t, c.caio, "Toren"); got.GetMovementUsedDft() != 330 {
		t.Errorf("through the rubble = %d dft used, want 330 (cave.py: 33,03 ft)", got.GetMovementUsedDft())
	}
	c.undoLast(t)

	// An ally on the rubble: +5 once, not +10 (and the destination's rubble +5).
	c.mustMove(t, c.master, "Pensantus", 4, 9)
	c.mustMove(t, c.caio, "Toren", 5, 10)
	if got := c.who(t, c.caio, "Toren"); got.GetMovementUsedDft() != 241 { // 141 + 50 (ally on rubble) + 50 (rubble)
		t.Errorf("through an ally on the rubble = %d dft used, want 241", got.GetMovementUsedDft())
	}
	c.undoLast(t)

	// A flier pays only the line: 18,0 ft through the same rubble.
	if _, err := c.h.pool.Exec(t.Context(), `UPDATE combatants SET speed_fly_ft = 30 WHERE id = $1`, c.id(t, "Toren")); err != nil {
		t.Fatalf("give Toren wings: %v", err)
	}
	c.mustMove(t, c.master, "Pensantus", 5, 8)
	if opts, err := c.options(t, c.caio, "Toren"); err != nil || !opts.GetFlier() {
		t.Fatalf("GetMoveOptions of a flier = %v, %v; want flier", opts, err)
	}
	c.mustMove(t, c.caio, "Toren", 6, 10)
	if got := c.who(t, c.caio, "Toren"); got.GetMovementUsedDft() != 180 || got.GetSpeedFlyFt() != 30 {
		t.Errorf("a flier through the rubble = %d dft used, want 180", got.GetMovementUsedDft())
	}
	c.undoLast(t)
	if _, err := c.h.pool.Exec(t.Context(), `UPDATE combatants SET speed_fly_ft = 0 WHERE id = $1`, c.id(t, "Toren")); err != nil {
		t.Fatalf("take Toren's wings: %v", err)
	}

	// Passing an ally costs 5 ft more; ending on one is refused.
	c.mustMove(t, c.master, "Toren", 6, 7)
	c.mustMove(t, c.master, "Pensantus", 5, 11)
	c.mustMove(t, c.caio, "Toren", 3, 7) // (4, 7) holds Brisa
	if got := c.who(t, c.caio, "Toren"); got.GetMovementUsedDft() != 200 {
		t.Errorf("past an ally = %d dft used, want 200 (3 squares and 5 ft for passing Brisa)", got.GetMovementUsedDft())
	}
	c.undoLast(t)
	_, err = c.move(t, c.caio, "Toren", 4, 7)
	wantEncounterBlocked(t, err, reasonOccupied)

	// An enemy of the same size, or one size apart, cannot be passed; the master's
	// "Aliado" can (Q69). The side is a master's call.
	c.mustMove(t, c.master, "Goblin 1", 8, 7)
	_, err = c.move(t, c.caio, "Toren", 10, 7)
	wantEncounterBlocked(t, err, reasonEnemy)
	c.side(t, "Goblin 1", playv1.CombatantSide_COMBATANT_SIDE_PARTY)
	c.mustMove(t, c.caio, "Toren", 10, 7)
	if got := c.who(t, c.caio, "Toren"); got.GetMovementUsedDft() != 250 {
		t.Errorf("past an ally goblin = %d dft used, want 250", got.GetMovementUsedDft())
	}
	c.undoLast(t)
	c.side(t, "Goblin 1", playv1.CombatantSide_COMBATANT_SIDE_ENEMY)
	_, err = c.move(t, c.caio, "Toren", 10, 7)
	wantEncounterBlocked(t, err, reasonEnemy)

	// A hidden enemy tells a player nothing: it does not block, and is not in the
	// way (RN-10).
	if _, err := c.master.combat.SetCombatantHidden(t.Context(), connect.NewRequest(&playv1.SetCombatantHiddenRequest{
		CampaignId: c.campaignID, EncounterId: c.get(t, c.master).GetId(), CombatantId: c.id(t, "Goblin 1"), IdempotencyKey: newKey(), Hidden: true,
	})); err != nil {
		t.Fatalf("SetCombatantHidden() error = %v", err)
	}
	c.mustMove(t, c.caio, "Toren", 10, 7)

	// Brisa is Small and the ogre Large: two sizes apart, she passes (at the price of
	// rubble); Toren, one size apart, would not.
	c.mustMove(t, c.master, "Ogro", 8, 8)
	c.mustMove(t, c.master, "Toren", 6, 8)
	_, err = c.move(t, c.caio, "Toren", 10, 8)
	wantEncounterBlocked(t, err, reasonEnemy)
	c.passTo(t, c.get(t, c.master), "Brisa")
	c.mustMove(t, c.master, "Brisa", 6, 8)
	c.mustMove(t, c.bia, "Brisa", 10, 8)
	if got := c.who(t, c.bia, "Brisa"); got.GetMovementUsedDft() != 250 || got.GetSize() != rulesv1.CreatureSize_CREATURE_SIZE_SMALL {
		t.Errorf("Brisa past the ogre = %d dft used, size %v; want 250 (25 ft), Small", got.GetMovementUsedDft(), got.GetSize())
	}
}

// TestRN21_GetMoveOptionsDrawsTheCircle: GetMoveOptions says where a combatant
// can go in one straight move and why it cannot go to the squares inside the
// circle that it refuses. The squares are those of cave.py (reach 6 7 30, and
// reach 16 8 30 combat).
func TestRN21_GetMoveOptionsDrawsTheCircle(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.fight(t)

	reachOf := func(opts *playv1.GetMoveOptionsResponse) map[grid.Square]int32 {
		out := map[grid.Square]int32{}
		for _, r := range opts.GetReachable() {
			out[grid.Square{Col: int(r.GetCol()), Row: int(r.GetRow())}] = r.GetCostDft()
		}
		return out
	}
	// want reads a drawing: + or * is a square that can be reached.
	want := func(drawing string) map[grid.Square]bool {
		out := map[grid.Square]bool{}
		for row, line := range strings.Split(strings.TrimSpace(drawing), "\n") {
			for col, ch := range line {
				if ch == '+' || ch == '*' {
					out[grid.Square{Col: col, Row: row}] = true
				}
			}
		}
		return out
	}
	same := func(name string, got map[grid.Square]int32, drawing string) {
		t.Helper()
		w := want(drawing)
		for sq := range w {
			if _, ok := got[sq]; !ok {
				t.Errorf("%s: %v should be reachable", name, sq)
			}
		}
		for sq := range got {
			if !w[sq] {
				t.Errorf("%s: %v should not be reachable", name, sq)
			}
		}
	}

	// cave.py reach 6 7 30, with the party around Toren.
	opts, err := c.options(t, c.caio, "Toren")
	if err != nil {
		t.Fatalf("GetMoveOptions(Toren) error = %v", err)
	}
	reach := reachOf(opts)
	same("from Toren", reach, `
########################
########################
################.......#
################.......#
################....q..#
################.......#
#.+++++#########.......#
.+++o+@++++++..........#
...o+o++++++...........#
#..+**+#++######.......#
#...:*+#.+##############
######+...+...##########
######+.......##########
######+.......##########
######........##########
########################`)
	if opts.GetMovementLeftDft() != 300 || reach[grid.Square{Col: 6, Row: 10}] != 150 {
		t.Errorf("costs = %d left, (6, 10) %d; want 300 and 150 (3 squares straight)", opts.GetMovementLeftDft(), reach[grid.Square{Col: 6, Row: 10}])
	}
	// The party's squares and the walls are refused, each for its reason.
	refused := map[grid.Square]playv1.MoveRefusal{}
	for _, r := range opts.GetRefused() {
		refused[grid.Square{Col: int(r.GetCol()), Row: int(r.GetRow())}] = r.GetReason()
	}
	if refused[grid.Square{Col: 5, Row: 8}] != playv1.MoveRefusal_MOVE_REFUSAL_OCCUPIED || refused[grid.Square{Col: 6, Row: 5}] != playv1.MoveRefusal_MOVE_REFUSAL_WALL {
		t.Errorf("refused (5, 8) = %v, (6, 5) = %v; want OCCUPIED and WALL", refused[grid.Square{Col: 5, Row: 8}], refused[grid.Square{Col: 6, Row: 5}])
	}

	// cave.py reach 16 8 30 combat: the goblins are enemies and cannot be crossed.
	c.mustMove(t, c.master, "Toren", 16, 8)
	opts, err = c.options(t, c.caio, "Toren")
	if err != nil {
		t.Fatalf("GetMoveOptions(Toren) error = %v", err)
	}
	same("from (16, 8) with the goblins", reachOf(opts), `
########################
########################
################+......#
################+++..x.#
################++..q..#
################++x+++.#
#......#########+++++..#
....o.o....+++++++++x..#
...o.o....++++++@++++++#
#...::.#..######++++++.#
#...::.#..##############
######........##########
######........##########
######........##########
######........##########
########################`)
	refused = map[grid.Square]playv1.MoveRefusal{}
	for _, r := range opts.GetRefused() {
		refused[grid.Square{Col: int(r.GetCol()), Row: int(r.GetRow())}] = r.GetReason()
	}
	for sq, reason := range map[grid.Square]playv1.MoveRefusal{
		{Col: 20, Row: 7}: playv1.MoveRefusal_MOVE_REFUSAL_OCCUPIED, // a goblin stands there
		{Col: 21, Row: 7}: playv1.MoveRefusal_MOVE_REFUSAL_ENEMY,    // behind it
		{Col: 20, Row: 4}: playv1.MoveRefusal_MOVE_REFUSAL_WALL,     // the column
	} {
		if refused[sq] != reason {
			t.Errorf("refused %v = %v, want %v", sq, refused[sq], reason)
		}
	}

	// Inside the circle but dearer than what is left: the rubble.
	c.mustMove(t, c.master, "Toren", 3, 9)
	opts, err = c.options(t, c.caio, "Toren")
	if err != nil {
		t.Fatalf("GetMoveOptions(Toren) error = %v", err)
	}
	refused = map[grid.Square]playv1.MoveRefusal{}
	for _, r := range opts.GetRefused() {
		refused[grid.Square{Col: int(r.GetCol()), Row: int(r.GetRow())}] = r.GetReason()
	}
	if len(opts.GetRefused()) == 0 {
		t.Fatalf("no refused square next to the rubble: %v", opts)
	}
	if _, ok := reachOf(opts)[grid.Square{Col: 5, Row: 10}]; !ok {
		t.Errorf("(5, 10) of (3, 9) should be reachable: 2 squares and 2 rubble squares")
	}

	// Who may ask: the combatant's own player, on their turn; the master any.
	if _, err := c.options(t, c.ana, "Toren"); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("GetMoveOptions(Toren) as Pensantus's player = %v, want permission_denied", err)
	}
	_, err = c.options(t, c.ana, "Pensantus")
	wantEncounterBlocked(t, err, reasonNotTurn)
	if _, err := c.options(t, c.master, "Pensantus"); err != nil {
		t.Errorf("GetMoveOptions(Pensantus) as the master error = %v", err)
	}
	if _, err := c.options(t, c.ana, "Escudeiro"); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("GetMoveOptions(an NPC) as a player = %v, want permission_denied", err)
	}
}

// TestMR034_JumpsAndTheRunningStart: Toren (Strength 16) jumps 16 ft with a
// running start (a move of 10 ft or more on foot right before) and 8 ft
// standing, and clears the rubble on the way; Brisa (Strength 8) cannot jump
// that far; a high jump is 3 + the modifier feet with a run and half standing
// and moves nobody; the log tells the master that a landing in rubble asks for
// an Acrobatics check, and the players do not hear it.
func TestMR034_JumpsAndTheRunningStart(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	e := c.fight(t)

	opts := c.mustOptions(t, c.caio, e, "Toren")
	if j := opts.GetOptions().GetJumps(); j.GetLongRunningDft() != 160 || j.GetLongStandingDft() != 80 || j.GetHighRunningDft() != 60 || j.GetHighStandingDft() != 30 || j.GetRunningStart() {
		t.Fatalf("Toren's jumps = %v, want 16 ft / 8 ft long, 6 ft / 3 ft high, no running start yet", j)
	}

	// Standing, 15 ft over the rubble is too far for 8 ft.
	c.mustMove(t, c.master, "Toren", 3, 9)
	_, err := c.move(t, c.caio, "Toren", 6, 9, jumpTo(playv1.JumpKind_JUMP_KIND_LONG))
	b := wantEncounterBlocked(t, err, reasonTooFar)
	if b.GetMissingDft() != 70 || b.GetJumpLimitDft() != 80 || b.GetJumpRunningStart() {
		t.Errorf("the standing jump = missing %d, limit %d, running %v; want 70, 80, false", b.GetMissingDft(), b.GetJumpLimitDft(), b.GetJumpRunningStart())
	}

	// A run of 10 ft first: (1, 9) to (3, 9). Then 15 ft over the pit, which costs
	// 15 ft of movement, not the 25 ft of walking through the rubble.
	c.mustMove(t, c.master, "Toren", 1, 9)
	c.mustMove(t, c.caio, "Toren", 3, 9)
	if opts = c.mustOptions(t, c.caio, e, "Toren"); !opts.GetOptions().GetJumps().GetRunningStart() {
		t.Errorf("after a run of 10 ft, running_start = false")
	}
	c.mustMove(t, c.caio, "Toren", 6, 9, jumpTo(playv1.JumpKind_JUMP_KIND_LONG))
	toren := c.who(t, c.caio, "Toren")
	if toren.GetCol() != 6 || toren.GetRow() != 9 || toren.GetMovementUsedDft() != 250 || toren.GetMovementLeftDft() != 50 {
		t.Fatalf("after the jump = %v, want on (6, 9), 250 dft used (a run of 100 and a jump of 150)", toren)
	}
	if opts = c.mustOptions(t, c.caio, e, "Toren"); opts.GetOptions().GetJumps().GetRunningStart() {
		t.Errorf("a jump is not a run: running_start should be false after it")
	}
	// The log: a long jump for everyone, no Acrobatics reminder (the landing is floor).
	if got := lastMove(t, c.log(t, c.master, e)); got.GetJump() != playv1.JumpKind_JUMP_KIND_LONG || got.GetDistanceDft() != 150 || got.GetLandingDifficult() {
		t.Errorf("the master's log of the jump = %v, want a long jump of 150 dft on floor", got)
	}
	c.undoLast(t)
	if toren = c.who(t, c.caio, "Toren"); toren.GetCol() != 3 || toren.GetMovementUsedDft() != 100 {
		t.Fatalf("after undoing the jump = %v, want back on (3, 9) with the run", toren)
	}
	// The run counts again: the undo gave it back.
	if opts = c.mustOptions(t, c.caio, e, "Toren"); !opts.GetOptions().GetJumps().GetRunningStart() {
		t.Errorf("after the undo, running_start = false, want the run back")
	}

	// Landing in the rubble: 10 ft, the pit's square (4, 9) cleared, and the master's
	// log reminds the Acrobatics check; the players' log does not.
	c.mustMove(t, c.caio, "Toren", 5, 9, jumpTo(playv1.JumpKind_JUMP_KIND_LONG))
	if got := c.who(t, c.caio, "Toren"); got.GetMovementUsedDft() != 200 {
		t.Errorf("a jump over the rubble into the rubble = %d dft, want 200 (100 run + 100 jump, no rubble surcharge)", got.GetMovementUsedDft())
	}
	if got := lastMove(t, c.log(t, c.master, e)); !got.GetLandingDifficult() {
		t.Errorf("the master's log of a landing in rubble = %v, want the Acrobatics reminder", got)
	}
	if got := lastMove(t, c.log(t, c.caio, e)); got == nil || got.GetLandingDifficult() || got.GetJump() != playv1.JumpKind_JUMP_KIND_LONG {
		t.Errorf("the player's log of a landing in rubble = %v, want the jump and no reminder", got)
	}
	c.undoLast(t)

	// A long jump cannot cross a wall or land on a creature.
	c.mustMove(t, c.master, "Toren", 6, 9)
	_, err = c.move(t, c.caio, "Toren", 8, 9, jumpTo(playv1.JumpKind_JUMP_KIND_LONG))
	wantEncounterBlocked(t, err, reasonMoveBlocked)
	c.mustMove(t, c.master, "Toren", 3, 9)
	_, err = c.move(t, c.caio, "Toren", 4, 7, jumpTo(playv1.JumpKind_JUMP_KIND_LONG))
	wantEncounterBlocked(t, err, reasonOccupied) // Brisa

	// The high jump: 6 ft with the run, nobody moves, the feet are spent.
	_, err = c.move(t, c.caio, "Toren", 3, 9, highJump(61))
	b = wantEncounterBlocked(t, err, reasonTooFar)
	if b.GetJumpLimitDft() != 60 || !b.GetJumpRunningStart() {
		t.Errorf("a high jump of 6,1 ft = limit %d running %v, want 60 and the run", b.GetJumpLimitDft(), b.GetJumpRunningStart())
	}
	c.mustMove(t, c.caio, "Toren", 3, 9, highJump(60))
	if got := c.who(t, c.caio, "Toren"); got.GetCol() != 3 || got.GetRow() != 9 || got.GetMovementUsedDft() != 160 {
		t.Errorf("after a high jump = %v, want still on (3, 9) with 160 dft used (the run and the 60)", got)
	}
	if got := lastMove(t, c.log(t, c.caio, e)); got.GetJump() != playv1.JumpKind_JUMP_KIND_HIGH || got.GetJumpHeightDft() != 60 {
		t.Errorf("the log of the high jump = %v, want a high jump of 60 dft", got)
	}
	// Standing now (a jump is no run): half, 3 ft.
	_, err = c.move(t, c.caio, "Toren", 3, 9, highJump(31))
	if b = wantEncounterBlocked(t, err, reasonTooFar); b.GetJumpLimitDft() != 30 || b.GetJumpRunningStart() {
		t.Errorf("a standing high jump = limit %d running %v, want 30 and no run", b.GetJumpLimitDft(), b.GetJumpRunningStart())
	}
	// The master jumps anyone anywhere: the limits are the players'.
	c.mustMove(t, c.master, "Toren", 6, 9, jumpTo(playv1.JumpKind_JUMP_KIND_LONG))

	// Brisa (Strength 8) cannot do what Toren did: 8 ft with a run, 4 ft standing.
	c.passTo(t, c.get(t, c.master), "Brisa")
	c.mustMove(t, c.master, "Brisa", 1, 9)
	c.mustMove(t, c.bia, "Brisa", 3, 9)
	_, err = c.move(t, c.bia, "Brisa", 6, 9, jumpTo(playv1.JumpKind_JUMP_KIND_LONG))
	b = wantEncounterBlocked(t, err, reasonTooFar)
	if b.GetMissingDft() != 70 || b.GetJumpLimitDft() != 80 || !b.GetJumpRunningStart() {
		t.Errorf("Brisa's jump = missing %d, limit %d, running %v; want 70, 80, true", b.GetMissingDft(), b.GetJumpLimitDft(), b.GetJumpRunningStart())
	}
}

// lastMove is the newest MOVED entry of a log, or nil.
func lastMove(t *testing.T, log *playv1.ListCombatLogResponse) *playv1.CombatLogEntry {
	t.Helper()
	for _, r := range log.GetRounds() {
		for _, en := range r.GetEntries() { // newest first
			if en.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_MOVED {
				return en
			}
		}
	}
	return nil
}

// aim puts the attacker and the target on squares, as the master.
func (c *cave) aim(t *testing.T, at map[string][2]int32) {
	t.Helper()
	for label, sq := range at {
		c.mustMove(t, c.master, label, sq[0], sq[1])
	}
}

// targetOf is the entry of a target list.
func listed(list []*playv1.TargetInReach, label string, c *cave, t *testing.T) *playv1.TargetInReach {
	t.Helper()
	id := c.id(t, label)
	for _, tg := range list {
		if tg.GetCombatantId() == id {
			return tg
		}
	}
	return nil
}

// TestMR034_CoverRaisesTheArmorClassOfTheTarget: for a weapon attack, a spell
// attack and a Dexterity save the server adds the cover between the attacker's
// and the target's squares: the crates (half, +2), the column (three-quarters,
// +5), a creature in between (half), the master's mark; the larger applies,
// never the sum; a wall or the master's "total" mark takes the target out of
// reach; a move clears the mark and the undo brings it back. The master's log
// has the armor class with the cover; a player never gets one (RN-20).
func TestMR034_CoverRaisesTheArmorClassOfTheTarget(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	e := c.fight(t)
	e = c.passTo(t, e, "Pensantus")
	bolt := func(target string, face int32) (*playv1.RollAttackResponse, error) {
		return c.attack(t, c.ana, c.get(t, c.ana), "Pensantus", fireBolt, target, d20(face))
	}
	toHit := c.mustOptions(t, c.ana, e, "Pensantus").GetOptions().GetAttacks()[0].GetAttack().GetAttackBonus()
	if toHit == 0 {
		t.Fatal("Fire Bolt has no attack bonus")
	}

	// Goblin 2 (AC 12) behind the crates: half cover, 14. A total of 12 would hit
	// a bare goblin and misses this one.
	c.aim(t, map[string][2]int32{"Pensantus": {17, 7}, "Goblin 2": {20, 7}})
	list := listed(c.mustOptions(t, c.ana, c.get(t, c.ana), "Pensantus").GetAttackTargets()[0].GetTargets(), "Goblin 2", c, t)
	if list.GetCover() != playv1.CoverDegree_COVER_DEGREE_HALF || list.GetCoverSource() != playv1.CoverSource_COVER_SOURCE_MAP || list.GetTooFar() {
		t.Errorf("Goblin 2 in the target list = %v, want half cover from the map", list)
	}
	hit, err := bolt("Goblin 2", 12-toHit)
	if err != nil {
		t.Fatalf("RollAttack() error = %v", err)
	}
	if hit.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_MISS || hit.GetRoll().GetCover() != playv1.CoverDegree_COVER_DEGREE_HALF {
		t.Errorf("the attack at 12 against half cover = %v, want a miss behind the crates", hit.GetRoll())
	}
	if hit.GetRoll().TargetArmorClass != nil {
		t.Errorf("a player's roll has an armor class: %v (RN-20)", hit.GetRoll())
	}
	masterEntry := func() *playv1.CombatLogEntry {
		t.Helper()
		for _, r := range c.log(t, c.master, c.get(t, c.master)).GetRounds() {
			for _, en := range r.GetEntries() {
				if en.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_ATTACK {
					return en
				}
			}
		}
		t.Fatal("no attack in the master's log")
		return nil
	}
	if en := masterEntry(); en.GetTargetArmorClass() != 14 || en.GetCoverBonus() != 2 || en.GetCover() != playv1.CoverDegree_COVER_DEGREE_HALF {
		t.Errorf("the master's log = AC %d, cover bonus %d, %v; want CA 14: 12 + 2 of half cover", en.GetTargetArmorClass(), en.GetCoverBonus(), en.GetCover())
	}
	c.undoLast(t)

	// The Capitão (AC 18) behind the column: three-quarters, 23. A total of 21 hits
	// a bare Capitão and misses this one.
	c.aim(t, map[string][2]int32{"Pensantus": {18, 4}, "Capitão Goblin": {21, 4}})
	hit, err = bolt("Capitão Goblin", 21-toHit)
	if err != nil || hit.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_MISS || hit.GetRoll().GetCover() != playv1.CoverDegree_COVER_DEGREE_THREE_QUARTERS {
		t.Errorf("the attack at 21 against three-quarters cover = %v, %v; want a miss", hit.GetRoll(), err)
	}
	if en := masterEntry(); en.GetTargetArmorClass() != 23 || en.GetCoverBonus() != 5 {
		t.Errorf("the master's log = AC %d, cover bonus %d, want 23 = 18 + 5", en.GetTargetArmorClass(), en.GetCoverBonus())
	}
	c.undoLast(t)

	// A wall on the line is total cover: not a target, with the reason; the master
	// still may.
	c.aim(t, map[string][2]int32{"Pensantus": {8, 10}, "Goblin 1": {13, 11}})
	tg := listed(c.mustOptions(t, c.ana, c.get(t, c.ana), "Pensantus").GetAttackTargets()[0].GetTargets(), "Goblin 1", c, t)
	if tg.GetCover() != playv1.CoverDegree_COVER_DEGREE_TOTAL || tg.GetCoverSource() != playv1.CoverSource_COVER_SOURCE_MAP {
		t.Errorf("a target behind a wall = %v, want total cover from the map", tg)
	}
	_, err = bolt("Goblin 1", 15)
	wantEncounterBlocked(t, err, reasonCoverTotal)
	if _, err := c.attack(t, c.master, c.get(t, c.master), "Pensantus", fireBolt, "Goblin 1", d20(15)); err != nil {
		t.Errorf("the master's attack through a wall = %v, want it allowed (he has the last word)", err)
	}
	c.undoLast(t)

	// Another creature in between is half cover (Toren at (17, 6) between
	// Pensantus and Goblin 3).
	c.aim(t, map[string][2]int32{"Pensantus": {16, 6}, "Toren": {17, 6}, "Goblin 3": {19, 6}})
	tg = listed(c.mustOptions(t, c.ana, c.get(t, c.ana), "Pensantus").GetAttackTargets()[0].GetTargets(), "Goblin 3", c, t)
	if tg.GetCover() != playv1.CoverDegree_COVER_DEGREE_HALF || tg.GetCoverSource() != playv1.CoverSource_COVER_SOURCE_MAP {
		t.Errorf("a target behind an ally = %v, want half cover", tg)
	}

	// The master's mark: the larger of the two applies, never the sum.
	c.mark(t, "Goblin 3", playv1.CoverDegree_COVER_DEGREE_HALF)
	hit, err = bolt("Goblin 3", 13-toHit)
	if err != nil || en0(masterEntry()) != 14 || hit.GetRoll().GetCover() != playv1.CoverDegree_COVER_DEGREE_HALF {
		t.Errorf("half from the map and half marked: AC %d, %v, %v; want 14 (+2, not +4)", en0(masterEntry()), hit.GetRoll(), err)
	}
	c.undoLast(t)
	c.mark(t, "Goblin 3", playv1.CoverDegree_COVER_DEGREE_THREE_QUARTERS)
	if got := c.who(t, c.ana, "Goblin 3"); got.GetCoverMark() != playv1.CoverDegree_COVER_DEGREE_THREE_QUARTERS {
		t.Errorf("the mark everyone sees = %v, want three-quarters", got.GetCoverMark())
	}
	hit, err = bolt("Goblin 3", 20-toHit) // 20 against 12 + 5 = 17: a hit
	if err != nil || hit.GetRoll().GetCoverSource() != playv1.CoverSource_COVER_SOURCE_MARK || hit.GetRoll().GetCover() != playv1.CoverDegree_COVER_DEGREE_THREE_QUARTERS {
		t.Errorf("the attack against a marked target = %v, %v; want three-quarters from the mark", hit.GetRoll(), err)
	}
	if en := masterEntry(); en.GetTargetArmorClass() != 17 || en.GetCoverBonus() != 5 {
		t.Errorf("the master's log = AC %d + %d, want 17 = 12 + 5", en.GetTargetArmorClass(), en.GetCoverBonus())
	}
	c.undoLast(t)
	// A "total" mark takes the target out of reach for a player.
	c.mark(t, "Goblin 3", playv1.CoverDegree_COVER_DEGREE_TOTAL)
	_, err = bolt("Goblin 3", 15)
	wantEncounterBlocked(t, err, reasonCoverTotal)
	// It clears when the combatant moves, and the undo of the move brings it back.
	c.mustMove(t, c.master, "Goblin 3", 20, 6)
	if got := c.who(t, c.master, "Goblin 3"); got.GetCoverMark() != playv1.CoverDegree_COVER_DEGREE_NONE {
		t.Errorf("after a move the mark = %v, want none", got.GetCoverMark())
	}
	c.undoLast(t)
	if got := c.who(t, c.master, "Goblin 3"); got.GetCoverMark() != playv1.CoverDegree_COVER_DEGREE_TOTAL || got.GetCol() != 19 {
		t.Errorf("after undoing the move = %v, want the total mark and (19, 6) back", got)
	}
	c.mark(t, "Goblin 3", playv1.CoverDegree_COVER_DEGREE_NONE)

	// A Dexterity save gets the cover too: Mãos Flamejantes at Goblin 2 behind the
	// crates. DC 14 (8 + 2 + 4: the gnome has Intelligence 18); the goblin rolls 12 +
	// 2 of cover = 14 and saves, as 12 alone would not.
	c.aim(t, map[string][2]int32{"Pensantus": {17, 7}, "Goblin 2": {20, 7}, "Toren": {6, 7}})
	c.h.roller.queue(12)
	cast := c.mustCast(t, c.ana, c.get(t, c.ana), "Pensantus", burningHands, slotOfLevel(1), c.at(t, "Goblin 2"), noCastRoll)
	res := cast.GetCast().GetTargets()[0]
	if res.GetSave().GetOutcome() != playv1.SaveOutcome_SAVE_OUTCOME_SAVED || res.GetCover() != playv1.CoverDegree_COVER_DEGREE_HALF {
		t.Errorf("the save behind the crates = %v, cover %v; want it saved with half cover", res.GetSave(), res.GetCover())
	}
	masterCast := false
	for _, r := range c.log(t, c.master, c.get(t, c.master)).GetRounds() {
		for _, en := range r.GetEntries() {
			if en.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_SPELL_CAST && len(en.GetSpell().GetTargets()) > 0 {
				masterCast = en.GetSpell().GetTargets()[0].GetCover() == playv1.CoverDegree_COVER_DEGREE_HALF
			}
		}
	}
	if !masterCast {
		t.Errorf("the log of the cast has no half cover for Goblin 2")
	}

	// What a player's browser gets: no armor class, whatever the cover (RN-20).
	for _, text := range []string{
		asJSON(t, hit), asJSON(t, cast), asJSON(t, c.get(t, c.ana)), asJSON(t, c.log(t, c.ana, e)),
		asJSON(t, c.mustOptions(t, c.ana, c.get(t, c.ana), "Pensantus")), asJSON(t, c.log(t, c.caio, e)),
	} {
		for _, banned := range []string{"armor", "Armor", "coverBonus"} {
			if strings.Contains(text, banned) {
				t.Errorf("a player's message has %q: %s", banned, text)
			}
		}
	}
}

func en0(en *playv1.CombatLogEntry) int32 { return en.GetTargetArmorClass() }

// TestMR034_ReachIsCountedInSquaresBetweenCentresRoundedDown: a diagonal
// neighbor is 5 ft away, so a melee attack works on all eight sides; 2 squares
// diagonally is 10 ft (not 14,1).
func TestMR034_ReachIsCountedInSquaresBetweenCentresRoundedDown(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	e := c.fight(t)
	c.aim(t, map[string][2]int32{"Toren": {17, 3}, "Goblin 1": {18, 4}, "Goblin 2": {19, 5}, "Goblin 3": {17, 2}})
	opts := c.mustOptions(t, c.caio, e, "Toren")
	var axe *playv1.AttackTargets
	for _, at := range opts.GetAttackTargets() {
		if at.GetAttackKey() == battleaxe {
			axe = at
		}
	}
	if axe == nil {
		t.Fatal("Toren has no battleaxe targets")
	}
	for label, want := range map[string]struct {
		ft      int32
		tooFar  bool
		comment string
	}{
		"Goblin 1": {5, false, "a diagonal neighbor is 5 ft"}, "Goblin 3": {5, false, "the square next to it is 5 ft"},
		"Goblin 2": {10, true, "two squares diagonally is 10 ft, beyond a 5 ft reach"},
	} {
		tg := listed(axe.GetTargets(), label, c, t)
		if tg == nil || tg.GetDistanceFt() != want.ft || tg.GetTooFar() != want.tooFar {
			t.Errorf("%s: %v, want %d ft, too_far %v (%s)", label, tg, want.ft, want.tooFar, want.comment)
		}
	}
	// Toren cannot swing two squares diagonally, but can at the diagonal neighbor.
	_, err := c.attack(t, c.caio, e, "Toren", battleaxe, "Goblin 2", d20(12))
	if b := wantEncounterBlocked(t, err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TARGET_OUT_OF_REACH); b.GetMissingFt() != 5 {
		t.Errorf("missing_ft = %d, want 5", b.GetMissingFt())
	}
	if _, err := c.attack(t, c.caio, e, "Toren", battleaxe, "Goblin 1", d20(12)); err != nil {
		t.Errorf("a melee attack on a diagonal neighbor = %v, want it allowed", err)
	}
}

// TestMR034_SidesAndTheMastersMarks: SetCombatantSide and SetCombatantCover are
// the master's, write the session events side_set and cover_set, and leave a
// player's character alone.
func TestMR034_SidesAndTheMastersMarks(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	e := c.fight(t)
	if got := c.who(t, c.caio, "Escudeiro").GetSide(); got != playv1.CombatantSide_COMBATANT_SIDE_PARTY {
		t.Errorf("the squire's side = %v, want the party's (Aliado)", got)
	}
	if got := c.who(t, c.caio, "Goblin 1").GetSide(); got != playv1.CombatantSide_COMBATANT_SIDE_ENEMY {
		t.Errorf("a goblin's side = %v, want the enemy's", got)
	}
	call := func(u *user, label string, side playv1.CombatantSide) error {
		_, err := u.combat.SetCombatantSide(t.Context(), connect.NewRequest(&playv1.SetCombatantSideRequest{
			CampaignId: c.campaignID, EncounterId: e.GetId(), CombatantId: c.id(t, label), IdempotencyKey: newKey(), Side: side,
		}))
		return err
	}
	if err := call(c.caio, "Goblin 1", playv1.CombatantSide_COMBATANT_SIDE_PARTY); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("a player marking a side = %v, want permission_denied", err)
	}
	if err := call(c.master, "Toren", playv1.CombatantSide_COMBATANT_SIDE_ENEMY); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("a player's character as an enemy = %v, want invalid_argument", err)
	}
	if err := call(c.master, "Goblin 1", playv1.CombatantSide_COMBATANT_SIDE_UNSPECIFIED); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("an unspecified side = %v, want invalid_argument", err)
	}
	if _, err := c.master.combat.SetCombatantCover(t.Context(), connect.NewRequest(&playv1.SetCombatantCoverRequest{
		CampaignId: c.campaignID, EncounterId: e.GetId(), CombatantId: c.id(t, "Goblin 1"), IdempotencyKey: newKey(),
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("an unspecified cover = %v, want invalid_argument", err)
	}
	if _, err := c.caio.combat.SetCombatantCover(t.Context(), connect.NewRequest(&playv1.SetCombatantCoverRequest{
		CampaignId: c.campaignID, EncounterId: e.GetId(), CombatantId: c.id(t, "Goblin 1"), IdempotencyKey: newKey(), Cover: playv1.CoverDegree_COVER_DEGREE_HALF,
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("a player marking cover = %v, want permission_denied", err)
	}
	c.mark(t, "Goblin 1", playv1.CoverDegree_COVER_DEGREE_HALF)
	var sides, covers int
	rows, err := c.h.pool.Query(t.Context(), `SELECT kind FROM session_events WHERE kind IN ('side_set', 'cover_set')`)
	if err != nil {
		t.Fatalf("read the events: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var kind string
		if err := rows.Scan(&kind); err != nil {
			t.Fatalf("Scan() error = %v", err)
		}
		switch kind {
		case "side_set":
			sides++
		case "cover_set":
			covers++
		}
	}
	if sides != 1 || covers != 1 { // the squire, and the goblin's mark
		t.Errorf("events = %d side_set, %d cover_set; want 1 and 1", sides, covers)
	}
}

// TestRN21_DisengageSetsAFlagForTheTurn: taking Desengajar marks the combatant
// for the rest of its turn (slice 9.6b reads it); the undo takes it back and the
// next turn starts without it.
func TestRN21_DisengageSetsAFlagForTheTurn(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	e := c.fight(t)
	if _, err := c.action(t, c.caio, e, "Toren", "standard:disengage"); err != nil {
		t.Fatalf("TakeAction(disengage) error = %v", err)
	}
	for who, u := range map[string]*user{"Toren's player": c.caio, "the master": c.master} {
		if !c.who(t, u, "Toren").GetDisengaged() {
			t.Errorf("Toren after Desengajar for %s: disengaged = false", who)
		}
	}
	if c.who(t, c.ana, "Toren").GetDisengaged() {
		t.Errorf("another player got Toren's disengaged flag, which is turn detail")
	}
	c.undoLast(t)
	if c.who(t, c.caio, "Toren").GetDisengaged() {
		t.Errorf("after the undo, disengaged = true")
	}
	if _, err := c.action(t, c.caio, e, "Toren", "standard:disengage"); err != nil {
		t.Fatalf("TakeAction(disengage) error = %v", err)
	}
	c.mustEndTurn(t, c.caio, e)
	c.passTo(t, c.get(t, c.master), "Toren") // the next round
	if c.who(t, c.caio, "Toren").GetDisengaged() {
		t.Errorf("in the next round, disengaged = true, want it reset at the start of the turn")
	}
}

// TestRN21_TheFeetFieldsAreTheTenthsRoundedDown: the shipped web reads feet, so
// the feet fields are the tenths rounded down.
func TestRN21_TheFeetFieldsAreTheTenthsRoundedDown(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	e := c.fight(t)
	c.mustMove(t, c.caio, "Toren", 7, 8) // one diagonal step
	got := c.who(t, c.caio, "Toren")
	if got.GetMovementUsedFt() != 7 || got.GetMovementUsedDft() != 71 || got.GetMovementLeftFt() != 22 || got.GetMovementLeftDft() != 229 {
		t.Errorf("after one diagonal step = %d ft / %d dft used, %d ft / %d dft left; want 7 / 71, 22 / 229", got.GetMovementUsedFt(), got.GetMovementUsedDft(), got.GetMovementLeftFt(), got.GetMovementLeftDft())
	}
	if m := c.mustOptions(t, c.caio, e, "Toren").GetOptions().GetEconomy().GetMovement(); m.GetLeftDft() != 229 || m.GetLeftFt() != 22 || m.GetSpeedDft() != 300 || m.GetUsedFt() != 7 {
		t.Errorf("the turn options' movement = %v, want 229 dft (22 ft) left of 300", m)
	}
}

// moveResponse is move with the whole response.
func (c *cave) moveResponse(t *testing.T, u *user, label string, col, row int32) (*playv1.MoveCombatantResponse, error) {
	t.Helper()
	res, err := u.combat.MoveCombatant(t.Context(), connect.NewRequest(&playv1.MoveCombatantRequest{
		CampaignId: c.campaignID, EncounterId: c.get(t, c.master).GetId(), CombatantId: c.id(t, label), IdempotencyKey: newKey(), Col: col, Row: row,
	}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func (c *cave) hide(t *testing.T, label string) {
	t.Helper()
	if _, err := c.master.combat.SetCombatantHidden(t.Context(), connect.NewRequest(&playv1.SetCombatantHiddenRequest{
		CampaignId: c.campaignID, EncounterId: c.get(t, c.master).GetId(), CombatantId: c.id(t, label), IdempotencyKey: newKey(), Hidden: true,
	})); err != nil {
		t.Fatalf("SetCombatantHidden(%s) error = %v", label, err)
	}
}

// TestRN20_AHiddenCreatureOnTheLineIsNoCoverForAPlayer: a creature the players do
// not see, standing between a player's character and the target, gives no cover
// (it would leak as "meia cobertura, do mapa"); when the master attacks, every
// creature counts.
func TestRN20_AHiddenCreatureOnTheLineIsNoCoverForAPlayer(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	e := c.fight(t)
	e = c.passTo(t, e, "Pensantus")
	c.aim(t, map[string][2]int32{"Pensantus": {16, 6}, "Capitão Goblin": {17, 6}, "Goblin 3": {19, 6}})
	c.hide(t, "Capitão Goblin")
	opts := c.mustOptions(t, c.ana, c.get(t, c.ana), "Pensantus")
	tg := listed(opts.GetAttackTargets()[0].GetTargets(), "Goblin 3", c, t)
	if tg.GetCover() != playv1.CoverDegree_COVER_DEGREE_NONE || tg.GetCoverSource() != playv1.CoverSource_COVER_SOURCE_UNSPECIFIED {
		t.Errorf("a target behind a hidden creature = %v, want no cover for the player", tg)
	}
	hit, err := c.attack(t, c.ana, c.get(t, c.ana), "Pensantus", fireBolt, "Goblin 3", d20(10))
	if err != nil {
		t.Fatalf("RollAttack() error = %v", err)
	}
	for _, text := range []string{asJSON(t, tg), asJSON(t, hit), asJSON(t, c.log(t, c.ana, e))} {
		if strings.Contains(text, "HALF") || strings.Contains(text, "COVER_SOURCE") {
			t.Errorf("a player's message tells about cover from a hidden creature: %s", text)
		}
	}
	// The master's goblin, shooting back along the same line, does count it.
	master := c.mustOptions(t, c.master, c.get(t, c.master), "Goblin 3")
	var at *playv1.TargetInReach
	for _, list := range master.GetAttackTargets() {
		if got := listed(list.GetTargets(), "Pensantus", c, t); got != nil {
			at = got
		}
	}
	if at == nil || at.GetCover() != playv1.CoverDegree_COVER_DEGREE_HALF {
		t.Errorf("Pensantus behind the hidden Capitão, for the goblin = %v, want half cover", at)
	}
}

// TestMR014_EscudoJudgesTheHitByTheArmorClassCoverRaised: Pensantus (AC 12) behind
// the crates has 14; a hit of 18 is a hit, and with Escudo (19) it misses. The
// bare sheet's 12 + 5 = 17 would have let it through.
func TestMR014_EscudoJudgesTheHitByTheArmorClassCoverRaised(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	e := c.fight(t)
	c.aim(t, map[string][2]int32{"Pensantus": {17, 7}, "Goblin 1": {20, 7}})
	e = c.passTo(t, e, "Goblin 1")
	hit := c.mustAttack(t, c.master, e, "Goblin 1", sword, "Pensantus", d20(14)) // 14 + 4 = 18 against 14
	pending := hit.GetPendingDamage()
	if hit.GetRoll().GetTargetArmorClass() != 14 || pending.GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_AWAITING_REACTION {
		t.Fatalf("the hit = AC %d, %v; want 14 and a hit that waits for the reaction", hit.GetRoll().GetTargetArmorClass(), pending)
	}
	res, err := c.useReaction(t, c.ana, e, pending.GetId(), slotOfLevel(1))
	if err != nil {
		t.Fatalf("UseReaction() error = %v", err)
	}
	if res.GetOutcome() != playv1.ReactionOutcome_REACTION_OUTCOME_STOPPED {
		t.Errorf("Escudo against an 18 on AC 14 (19 with it) = %v, want the attack stopped", res.GetOutcome())
	}
}

// TestMR034_SacredFlameGetsNoCover: the SRD's Chama Sagrada gives its target no
// benefit from cover for the save, said in the spell data (ignores_cover); a wall
// still stops the targeting.
func TestMR034_SacredFlameGetsNoCover(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	e := c.fight(t)
	e = c.passTo(t, e, "Brisa")
	c.aim(t, map[string][2]int32{"Brisa": {17, 7}, "Goblin 2": {20, 7}})
	// DC 13 (8 + 2 + 3); the goblin rolls 11: with the crates' +2 it would save.
	c.h.roller.queue(11)
	cast := c.mustCast(t, c.bia, e, "Brisa", sacredFlame, nil, c.at(t, "Goblin 2"), noCastRoll)
	res := cast.GetCast().GetTargets()[0]
	if res.GetSave().GetOutcome() != playv1.SaveOutcome_SAVE_OUTCOME_FAILED || res.GetCover() != playv1.CoverDegree_COVER_DEGREE_NONE {
		t.Errorf("Chama Sagrada behind the crates = %v, cover %v; want a failed save and no cover counted", res.GetSave(), res.GetCover())
	}
	c.undoLast(t)
	c.aim(t, map[string][2]int32{"Brisa": {8, 10}, "Goblin 1": {13, 11}})
	_, err := c.cast(t, c.bia, c.get(t, c.bia), "Brisa", sacredFlame, nil, c.at(t, "Goblin 1"), noCastRoll)
	wantEncounterBlocked(t, err, reasonCoverTotal)
}

// TestMR034_TheJumpLimitsShownAreTheOnesEnforced: GetTurnOptions shows what the
// combatant has on it, which MoveCombatant enforces (a combat started before the
// columns had zeros for both).
func TestMR034_TheJumpLimitsShownAreTheOnesEnforced(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	e := c.fight(t)
	if _, err := c.h.pool.Exec(t.Context(), `UPDATE combatants SET jump_long_dft = 0, jump_high_dft = 0 WHERE id = $1`, c.id(t, "Toren")); err != nil {
		t.Fatalf("zero the jumps: %v", err)
	}
	j := c.mustOptions(t, c.caio, e, "Toren").GetOptions().GetJumps()
	if j.GetLongRunningDft() != 0 || j.GetHighRunningDft() != 0 {
		t.Errorf("the jumps shown = %v, want the combatant's own (none)", j)
	}
	c.mustMove(t, c.master, "Toren", 3, 9)
	_, err := c.move(t, c.caio, "Toren", 4, 9, jumpTo(playv1.JumpKind_JUMP_KIND_LONG))
	wantEncounterBlocked(t, err, reasonTooFar)
}

// TestMR034_AMoveBeforeTheUndoKnewMovesCannotBeUndone: a move whose event has no
// "from" is not offered to the undo, and asking for it is refused.
func TestMR034_AMoveBeforeTheUndoKnewMovesCannotBeUndone(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	e := c.fight(t)
	c.mustMove(t, c.caio, "Toren", 7, 7)
	var id string
	if err := c.h.pool.QueryRow(t.Context(), `SELECT id FROM session_events WHERE kind = 'combatant_moved' ORDER BY seq DESC LIMIT 1`).Scan(&id); err != nil {
		t.Fatalf("find the move: %v", err)
	}
	if _, err := c.h.pool.Exec(t.Context(), `UPDATE session_events SET payload = payload - 'from' WHERE id = $1`, id); err != nil {
		t.Fatalf("strip the from: %v", err)
	}
	if got := c.log(t, c.master, e).GetUndoableEventId(); got != "" {
		t.Errorf("undoable event = %q, want none for a move with nothing to put back", got)
	}
	err := c.undo(t, c.master, e, id)
	wantEncounterBlocked(t, err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOTHING_TO_UNDO)
}

// TestRN10_ACreatureThePlayerDoesNotSeeCutsTheMoveShort: the player plans on what
// they see; the real move stops before a hidden creature, charges the real cost up
// to there and says only that it was cut short (D1).
func TestRN10_ACreatureThePlayerDoesNotSeeCutsTheMoveShort(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.fight(t)
	c.mustMove(t, c.master, "Goblin 1", 8, 7)
	c.hide(t, "Goblin 1")
	res, err := c.moveResponse(t, c.caio, "Toren", 10, 7)
	if err != nil {
		t.Fatalf("MoveCombatant() error = %v", err)
	}
	toren := byLabel(t, res.GetEncounter(), "Toren")
	if !res.GetStoppedEarly() || toren.GetCol() != 7 || toren.GetMovementUsedDft() != 50 {
		t.Errorf("after walking into a hidden goblin = stopped %v at (%d, %d), %d dft; want cut short on (7, 7) with 50 dft", res.GetStoppedEarly(), toren.GetCol(), toren.GetRow(), toren.GetMovementUsedDft())
	}
	if text := asJSON(t, res); strings.Contains(text, "Goblin 1") || strings.Contains(text, c.id(t, "Goblin 1")) {
		t.Errorf("the player's answer names the hidden goblin: %s", text)
	}
	// Onto its very square: stops before it too.
	c.mustMove(t, c.master, "Toren", 6, 7)
	res, err = c.moveResponse(t, c.caio, "Toren", 8, 7)
	if err != nil || !res.GetStoppedEarly() || byLabel(t, res.GetEncounter(), "Toren").GetCol() != 7 {
		t.Errorf("onto the hidden goblin's square = %v, %v; want cut short on (7, 7)", res, err)
	}
}

// TestMR034_TheRunningStartAddsUpOnFootAndAnythingBreaksIt: two legs of 5 ft are
// the 10 ft before a jump; an attack in between breaks it (and its undo gives it
// back).
func TestMR034_TheRunningStartAddsUpOnFootAndAnythingBreaksIt(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	e := c.fight(t)
	run := func() bool { return c.mustOptions(t, c.caio, e, "Toren").GetOptions().GetJumps().GetRunningStart() }
	c.aim(t, map[string][2]int32{"Toren": {1, 9}, "Goblin 3": {2, 8}})
	c.mustMove(t, c.caio, "Toren", 2, 9)
	if run() {
		t.Errorf("after 5 ft on foot, running_start = true")
	}
	c.mustMove(t, c.caio, "Toren", 3, 9)
	if !run() {
		t.Errorf("after two legs of 5 ft, running_start = false, want the 10 ft added up")
	}
	if _, err := c.attack(t, c.caio, e, "Toren", battleaxe, "Goblin 3", d20(12)); err != nil {
		t.Fatalf("RollAttack() error = %v", err)
	}
	if run() {
		t.Errorf("after an attack, running_start = true, want it broken")
	}
	c.undoLast(t)
	if !run() {
		t.Errorf("after undoing the attack, running_start = false, want it back")
	}
}

// TestMR034_NothingMovedKeepsTheMarkAndTheRun: a high jump, a master's move to the
// square the combatant stands on, and a move of no length change neither the
// master's cover mark nor (but for the high jump, a jump) the running start.
func TestMR034_NothingMovedKeepsTheMarkAndTheRun(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	e := c.fight(t)
	c.aim(t, map[string][2]int32{"Toren": {1, 9}})
	c.mustMove(t, c.caio, "Toren", 3, 9)
	c.mark(t, "Toren", playv1.CoverDegree_COVER_DEGREE_HALF)
	c.mustMove(t, c.master, "Toren", 3, 9) // where he stands
	c.mustMove(t, c.caio, "Toren", 3, 9)   // no length
	if got := c.who(t, c.caio, "Toren"); got.GetCoverMark() != playv1.CoverDegree_COVER_DEGREE_HALF {
		t.Errorf("after moves that moved nobody the mark = %v, want it kept", got.GetCoverMark())
	}
	if !c.mustOptions(t, c.caio, e, "Toren").GetOptions().GetJumps().GetRunningStart() {
		t.Errorf("a move of no length broke the running start")
	}
	c.mustMove(t, c.caio, "Toren", 3, 9, highJump(30))
	if got := c.who(t, c.caio, "Toren"); got.GetCoverMark() != playv1.CoverDegree_COVER_DEGREE_HALF {
		t.Errorf("after a high jump the mark = %v, want it kept (nobody changed square)", got.GetCoverMark())
	}
	c.mustMove(t, c.caio, "Toren", 4, 9)
	if got := c.who(t, c.caio, "Toren"); got.GetCoverMark() != playv1.CoverDegree_COVER_DEGREE_NONE {
		t.Errorf("after a step the mark = %v, want it cleared", got.GetCoverMark())
	}
}

// TestMR034_TheMapsLayersDecideTheMove: through the real maps service, a wall
// painted with PaintMapCells blocks a straight move and a square of difficult
// terrain costs 5 ft more.
func TestMR034_TheMapsLayersDecideTheMove(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	content, err := testRules()
	if err != nil {
		t.Fatalf("rules.LoadSRD() error = %v", err)
	}
	msvc, err := maps.New(maps.Config{Pool: c.h.pool, Characters: c.h.chars, Live: c.h.svc, Rules: content, Combats: c.h.svc, Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("maps.New() error = %v", err)
	}
	c.h.svc.SetTerrain(msvc)
	srv := httpserver.New(httpserver.Config{Logger: slog.New(slog.DiscardHandler)})
	msvc.Mount(srv.Handle, mapsSessions{testSessions}, c.h.camps, connect.WithRequireConnectProtocolHeader())
	server := httptest.NewServer(srv.Handler())
	t.Cleanup(server.Close)
	paint := mapsv1connect.NewMapServiceClient(&http.Client{Transport: userTransport{userID: c.master.id, next: server.Client().Transport}}, server.URL)
	put := func(layer mapsv1.MapLayer, col, row int32) {
		t.Helper()
		if _, err := paint.PaintMapCells(t.Context(), connect.NewRequest(&mapsv1.PaintMapCellsRequest{
			CampaignId: c.campaignID, MapId: c.mapID, Layer: layer, Value: 1, Squares: []*mapsv1.MapSquare{{Col: col, Row: row}},
		})); err != nil {
			t.Fatalf("PaintMapCells(%v) error = %v", layer, err)
		}
	}

	c.fight(t)
	put(mapsv1.MapLayer_MAP_LAYER_WALL, 8, 7)
	put(mapsv1.MapLayer_MAP_LAYER_DIFFICULT_TERRAIN, 6, 8)
	_, err = c.move(t, c.caio, "Toren", 10, 7)
	wantEncounterBlocked(t, err, reasonMoveBlocked)
	c.mustMove(t, c.caio, "Toren", 6, 9) // (6, 8) is rubble: 10 ft of line and 5 ft more
	if got := c.who(t, c.caio, "Toren"); got.GetMovementUsedDft() != 150 {
		t.Errorf("through painted rubble = %d dft used, want 150", got.GetMovementUsedDft())
	}
}

// mapsSessions lets the test sessions serve the maps module, whose image routes
// authenticate plain requests too (none is made here).
type mapsSessions struct{ Sessions }

func (mapsSessions) AuthenticateRequest(r *http.Request) (context.Context, error) {
	return withTestUser(r.Context(), r.Header), nil
}

// A character the master marked dead in the middle of a combat is out of the fight: the
// master's moves of an NPC and the move options still work.
func TestADeadCharacterDoesNotBreakNPCMoves(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.fight(t)

	if _, err := c.master.characters.MarkCharacterDead(t.Context(), connect.NewRequest(&charactersv1.MarkCharacterDeadRequest{
		CampaignId: c.campaignID, CharacterId: c.pens.GetId(),
	})); err != nil {
		t.Fatalf("MarkCharacterDead() error = %v", err)
	}

	// The NPC is on turn: only then does a move offer opportunity attacks.
	c.passTo(t, c.get(t, c.master), "Goblin 1")

	// A forced move still works (no reactors are computed).
	if _, err := c.move(t, c.master, "Goblin 1", 17, 5, func(r *playv1.MoveCombatantRequest) { r.Forced = true }); err != nil {
		t.Errorf("forced MoveCombatant(Goblin 1) error = %v, want nil", err)
	}
	if _, err := c.options(t, c.master, "Goblin 1"); err != nil {
		t.Errorf("GetMoveOptions(Goblin 1) error = %v, want nil", err)
	}
	if _, err := c.move(t, c.master, "Goblin 1", 16, 5); err != nil {
		t.Errorf("MoveCombatant(Goblin 1, not forced) error = %v, want nil", err)
	}
}

// A player's character at 0 hit points does not move on their turn (RN-03).
func TestADownPlayerCannotMove(t *testing.T) {
	c := newCave(t)
	c.fight(t) // Toren is first on turn
	if _, err := c.master.play.AdjustCharacterVitals(t.Context(), connect.NewRequest(&playv1.AdjustCharacterVitalsRequest{
		CampaignId: c.campaignID, CharacterId: c.toren.GetId(), IdempotencyKey: newKey(), HitPointsCurrent: proto.Int32(0),
	})); err != nil {
		t.Fatalf("AdjustCharacterVitals() error = %v", err)
	}
	_, err := c.move(t, c.caio, "Toren", 7, 7)
	wantBlockedBy(t, "a down player's MoveCombatant", err, blockedDown)
}
