package play

import (
	"maps"
	"slices"
	"testing"
	"time"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// Wall of Fire (SRD 5.1) lays a wall of squares from a point. The geometry tests are pure; the cast tests need the
// database (MEURPG_TEST_DATABASE_URL) and use the cave of zones_test.go: Toren (5,7), Pensantus the wizard of level 9
// (12,7), Brisa (3,7), the ogre (7,7) and the goblin (20,7).

const wallOfFireKey = "spell:wall-of-fire"

func wsq(col, row int) grid.Square { return grid.Square{Col: col, Row: row} }

func setOf(squares []grid.Square) map[grid.Square]bool {
	out := map[grid.Square]bool{}
	for _, s := range squares {
		out[s] = true
	}
	return out
}

func TestWallStraightSquaresFollowTheMapsDistance(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name   string
		dir    grid.Direction
		length int
		want   int
	}{
		{"orthogonal 60 ft", grid.Direction{Dx: 1}, 60, 12},
		// The map rounds a distance down, so the diagonal wall of 60 ft is 9 squares: fewer than the 12 of a row.
		{"diagonal 60 ft", grid.Direction{Dx: 1, Dy: 1}, 60, 9},
		{"25 ft", grid.Direction{Dy: -1}, 25, 5},
		{"5 ft", grid.Direction{Dx: -1}, 5, 1},
	} {
		w := wallSpec{Dir: tt.dir, LengthFt: tt.length, Side: playv1.WallSide_WALL_SIDE_LEFT}
		got := w.squares(wsq(30, 30))
		if len(got) != tt.want {
			t.Errorf("%s: %d squares, want %d", tt.name, len(got), tt.want)
		}
		if got[0] != wsq(30, 30) {
			t.Errorf("%s: first square %v, want the point of origin", tt.name, got[0])
		}
	}
}

func TestWallRingSquares(t *testing.T) {
	t.Parallel()
	origin := wsq(10, 10)
	for _, tt := range []struct {
		diameter, want, atFt int
	}{{20, 16, 10}, {10, 8, 5}, {5, 8, 5}} {
		w := wallSpec{Ring: true, LengthFt: tt.diameter, Side: playv1.WallSide_WALL_SIDE_INNER}
		got := w.squares(origin)
		if len(got) != tt.want {
			t.Errorf("diameter %d: %d squares, want %d", tt.diameter, len(got), tt.want)
		}
		for _, s := range got {
			if grid.RangeFt(origin, s) != tt.atFt {
				t.Errorf("diameter %d: %v is %d ft out, want %d", tt.diameter, s, grid.RangeFt(origin, s), tt.atFt)
			}
		}
	}
}

func TestWallIsCutByTheTerrain(t *testing.T) {
	t.Parallel()
	g := grid.Grid{Columns: 20, Rows: 20}
	terrain := grid.Terrain{Grid: g, Walls: grid.NewLayer(g)}
	terrain.Walls.Set(13, 5, true)
	w := wallSpec{Dir: grid.Direction{Dx: 1}, LengthFt: 60, Side: playv1.WallSide_WALL_SIDE_LEFT}
	got := w.laid(terrain, wsq(10, 5))
	if want := []grid.Square{wsq(10, 5), wsq(11, 5), wsq(12, 5)}; !slices.Equal(got, want) {
		t.Errorf("laid = %v, want %v: a wall of the map ends the wall", got, want)
	}
}

func TestReadWallRefusesWhatIsMalformed(t *testing.T) {
	t.Parallel()
	east := &playv1.SpellDirection{Dx: 1}
	left, inner := playv1.WallSide_WALL_SIDE_LEFT, playv1.WallSide_WALL_SIDE_INNER
	for name, w := range map[string]*playv1.SpellWall{
		"straight without direction": {Side: left},
		"ring with direction":        {Ring: true, Direction: east, Side: inner},
		"straight with a ring side":  {Direction: east, Side: inner},
		"ring with a line side":      {Ring: true, Side: left},
		"unspecified side":           {Direction: east},
		"length not a multiple of 5": {Direction: east, Side: left, LengthFt: 22},
		"straight too long":          {Direction: east, Side: left, LengthFt: 65},
		"ring too wide":              {Ring: true, Side: inner, LengthFt: 25},
		"negative length":            {Direction: east, Side: left, LengthFt: -5},
		"not one of the eight":       {Direction: &playv1.SpellDirection{Dx: 2}, Side: left},
		"no way at all":              {Direction: &playv1.SpellDirection{}, Side: left},
	} {
		if _, err := readWall(w); connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("%s: error = %v, want invalid_argument", name, err)
		}
	}
	got, err := readWall(&playv1.SpellWall{Direction: east, Side: left})
	if err != nil || got.LengthFt != 60 {
		t.Errorf("full size: %v, %v, want 60 ft", got, err)
	}
	got, err = readWall(&playv1.SpellWall{Ring: true, Side: inner})
	if err != nil || got.LengthFt != 20 {
		t.Errorf("full ring: %v, %v, want 20 ft", got, err)
	}
}

func wallReachOf(w wallSpec, origin grid.Square) map[grid.Square]bool {
	return w.reach(origin, w.squares(origin))
}

// For a wall that runs east the left is north (rows grow downward), within 10 ft, and the wall itself hurts from both.
func TestWallReachOfAStraightWallIsTheDamagingSide(t *testing.T) {
	t.Parallel()
	origin := wsq(10, 10)
	left := wallReachOf(wallSpec{Dir: grid.Direction{Dx: 1}, LengthFt: 15, Side: playv1.WallSide_WALL_SIDE_LEFT}, origin)
	right := wallReachOf(wallSpec{Dir: grid.Direction{Dx: 1}, LengthFt: 15, Side: playv1.WallSide_WALL_SIDE_RIGHT}, origin)
	for _, s := range []grid.Square{wsq(10, 10), wsq(11, 10), wsq(12, 10)} {
		if !left[s] || !right[s] {
			t.Errorf("%v is a square of the wall: it hurts from both sides", s)
		}
	}
	for _, c := range []struct {
		s           grid.Square
		left, right bool
	}{
		{wsq(11, 9), true, false},
		{wsq(11, 8), true, false},
		{wsq(11, 7), false, false},
		{wsq(11, 11), false, true},
		{wsq(11, 12), false, true},
		{wsq(11, 13), false, false},
		{wsq(13, 9), true, false},
		{wsq(14, 10), false, false},
		{wsq(9, 10), false, false},
	} {
		if left[c.s] != c.left || right[c.s] != c.right {
			t.Errorf("%v: left %v right %v, want %v and %v", c.s, left[c.s], right[c.s], c.left, c.right)
		}
	}
	north := wallReachOf(wallSpec{Dir: grid.Direction{Dy: -1}, LengthFt: 15, Side: playv1.WallSide_WALL_SIDE_LEFT}, origin)
	if !north[wsq(9, 9)] || north[wsq(11, 9)] {
		t.Error("walking north, the left is west")
	}
}

func TestWallReachOfARingIsInsideOrOutside(t *testing.T) {
	t.Parallel()
	origin := wsq(10, 10)
	inner := wallReachOf(wallSpec{Ring: true, LengthFt: 20, Side: playv1.WallSide_WALL_SIDE_INNER}, origin)
	outer := wallReachOf(wallSpec{Ring: true, LengthFt: 20, Side: playv1.WallSide_WALL_SIDE_OUTER}, origin)
	for _, s := range []grid.Square{wsq(12, 10), wsq(10, 8), wsq(12, 12)} {
		if !inner[s] || !outer[s] {
			t.Errorf("%v is a square of the ring: it hurts from both sides", s)
		}
	}
	for _, c := range []struct {
		s            grid.Square
		inner, outer bool
	}{
		{wsq(10, 10), true, false},
		{wsq(11, 10), true, false},
		{wsq(11, 11), true, false},
		{wsq(13, 10), false, true},
		{wsq(14, 10), false, true},
		{wsq(15, 10), false, false},
	} {
		if inner[c.s] != c.inner || outer[c.s] != c.outer {
			t.Errorf("%v: inner %v outer %v, want %v and %v", c.s, inner[c.s], outer[c.s], c.inner, c.outer)
		}
	}
}

// wallCave is the zone cave with Pensantus (wizard 9) knowing Wall of Fire.
func wallCave(t *testing.T) *cave {
	t.Helper()
	return newCaveOf(t, 9, []string{wallOfFireKey, cloudkillKey, thunderwave}, func(a *armed) *charactersv1.Character {
		return a.caio.caster(t, a.campaignID, "Toren", "class:cleric", "race:human", 7, abilities(14, 12, 14, 16, 10),
			[]string{maceKey}, []string{sacredFlame}, nil, []string{blessKey})
	})
}

// northWall is the wall from (7,8) north: the ogre at (7,7) is in it and (7,6) is a wall of the map, so it is 2 squares.
func northWall(side playv1.WallSide) func(*playv1.CastSpellRequest) {
	return func(r *playv1.CastSpellRequest) {
		r.Wall = &playv1.SpellWall{Direction: toward(0, -1), Side: side}
	}
}

func (c *cave) castWall(t *testing.T, slot int32, edit func(*playv1.CastSpellRequest)) (*playv1.CastSpellResponse, error) {
	t.Helper()
	return c.castArea(t, c.ana, "Pensantus", wallOfFireKey, slotOfLevel(slot), at(7, 8), nil, edit)
}

func (c *cave) wallFight(t *testing.T) {
	t.Helper()
	c.passTo(t, c.zoneFight(t), "Pensantus")
}

func TestWallOfFireLaysTheWallAndTheCreatureInsideSaves(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		roll int
		want int32
	}{{"failed", 5, 59 - 25}, {"passed", 20, 59 - 12}} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := wallCave(t)
			c.wallFight(t)
			c.h.roller.queue(tt.roll, 5, 5, 5, 5, 5) // the ogre's Dexterity save, then 5d8 = 25
			if _, err := c.castWall(t, 4, northWall(playv1.WallSide_WALL_SIDE_LEFT)); err != nil {
				t.Fatalf("CastSpell() error = %v", err)
			}
			z := c.oneZone(t, c.master, wallOfFireKey)
			if len(z.GetSquares()) != 2 || !zoneHas(z, 7, 8) || !zoneHas(z, 7, 7) {
				t.Errorf("squares = %v, want (7,8) and (7,7): the wall of the map ends it", z.GetSquares())
			}
			if !z.GetConcentration() || z.GetRoundsLeft() != 10 || z.GetSaveAbility() != "dex" {
				t.Errorf("zone = %v, want concentration, 10 rounds, a Dexterity save", z)
			}
			if cur, _, _ := c.hp(t, "Ogro"); cur != tt.want {
				t.Errorf("the ogre has %d hit points, want %d", cur, tt.want)
			}
			if cur, _, _ := c.hp(t, "Goblin"); cur != 7 {
				t.Errorf("the goblin has %d hit points, want 7: it is outside the wall", cur)
			}
		})
	}
}

func TestWallOfFireAtTheFifthLevelRollsSixDice(t *testing.T) {
	t.Parallel()
	c := wallCave(t)
	c.wallFight(t)
	c.h.roller.queue(5, 1, 2, 3, 4, 5, 6) // the save fails, then 6d8 = 21
	if _, err := c.castWall(t, 5, northWall(playv1.WallSide_WALL_SIDE_RIGHT)); err != nil {
		t.Fatalf("CastSpell() error = %v", err)
	}
	if cur, _, _ := c.hp(t, "Ogro"); cur != 59-21 {
		t.Errorf("the ogre has %d hit points, want 38: 6d8 = 21", cur)
	}
}

func TestWallOfFireWithoutAWallIsRefusedAndCostsNothing(t *testing.T) {
	t.Parallel()
	c := wallCave(t)
	c.wallFight(t)
	_, err := c.castWall(t, 4, func(*playv1.CastSpellRequest) {})
	wantBlockedBy(t, "CastSpell", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_WALL_PLACEMENT_REQUIRED)
	_, err = c.castArea(t, c.ana, "Pensantus", wallOfFireKey, slotOfLevel(4), nil, nil)
	wantBlockedBy(t, "CastSpell without a point", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_WALL_PLACEMENT_REQUIRED)
	if byLabel(t, c.get(t, c.master), "Pensantus").GetActionUsed() {
		t.Error("the refused cast spent the action")
	}
	if used := usedSlots(c.vitals(t, c.pens), 4); used != 0 {
		t.Errorf("slots used = %d, want 0", used)
	}
	if zs := c.get(t, c.master).GetZones(); len(zs) != 0 {
		t.Errorf("zones = %v, want none", zs)
	}
}

func TestWallOnAnotherSpellIsRefused(t *testing.T) {
	t.Parallel()
	c := wallCave(t)
	c.wallFight(t)
	_, err := c.castArea(t, c.ana, "Pensantus", cloudkillKey, slotOfLevel(5), at(7, 8), nil, northWall(playv1.WallSide_WALL_SIDE_LEFT))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("error = %v, want invalid_argument", err)
	}
}

func TestAStraightWallWithoutADirectionIsRefused(t *testing.T) {
	t.Parallel()
	c := wallCave(t)
	c.wallFight(t)
	_, err := c.castWall(t, 4, func(r *playv1.CastSpellRequest) { r.Wall = &playv1.SpellWall{Side: playv1.WallSide_WALL_SIDE_LEFT} })
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("error = %v, want invalid_argument", err)
	}
}

func TestAWallWithoutAPointOfOriginIsRefused(t *testing.T) {
	t.Parallel()
	c := wallCave(t)
	c.wallFight(t)
	_, err := c.castArea(t, c.ana, "Pensantus", wallOfFireKey, slotOfLevel(4), nil, nil, northWall(playv1.WallSide_WALL_SIDE_LEFT))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("error = %v, want invalid_argument", err)
	}
	if zs := c.get(t, c.master).GetZones(); len(zs) != 0 {
		t.Errorf("zones = %v, want none", zs)
	}
}

func TestAWallInACombatWithoutAMapIsRefused(t *testing.T) {
	t.Parallel()
	c := wallCave(t)
	c.wallFight(t)
	c.execSQL(t, `UPDATE encounters SET status = 'active', mode = 'theatre', grid_columns = 0, grid_rows = 0, map_id = NULL, map_point_id = NULL`)
	_, err := c.castWall(t, 4, northWall(playv1.WallSide_WALL_SIDE_LEFT))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("error = %v, want invalid_argument", err)
	}
}

func TestAPlayersWallBeyondTheRangeIsRefusedAndTheMastersIsNot(t *testing.T) {
	t.Parallel()
	c := wallCave(t)
	c.wallFight(t)
	// The map is 24 squares wide, so the encounter is widened to put a point past the 120 ft of the spell.
	c.execSQL(t, `UPDATE encounters SET grid_columns = 48`)
	far := func(u *user) error {
		_, err := c.castArea(t, u, "Pensantus", wallOfFireKey, slotOfLevel(4), at(40, 7), nil, northWall(playv1.WallSide_WALL_SIDE_LEFT))
		return err
	}
	wantBlockedBy(t, "CastSpell", far(c.ana), blockedOutOfReach)
	if byLabel(t, c.get(t, c.master), "Pensantus").GetActionUsed() {
		t.Error("the refused cast spent the action")
	}
	if err := far(c.master); err != nil {
		t.Errorf("the master's cast was held to the range: %v", err)
	}
}

func TestTheMasterCanLayTheWall(t *testing.T) {
	t.Parallel()
	c := wallCave(t)
	c.wallFight(t)
	c.h.roller.queue(5, 5, 5, 5, 5, 5)
	if _, err := c.castArea(t, c.master, "Pensantus", wallOfFireKey, slotOfLevel(4), at(7, 8), nil, northWall(playv1.WallSide_WALL_SIDE_RIGHT)); err != nil {
		t.Fatalf("CastSpell() as the master error = %v", err)
	}
	if z := c.oneZone(t, c.master, wallOfFireKey); len(z.GetSquares()) != 2 {
		t.Errorf("squares = %v, want 2", z.GetSquares())
	}
}

// The answer of the cast draws the wall, and the answer of a retry of the same key draws the same one.
func TestTheAnswerAndItsRetryDrawTheWall(t *testing.T) {
	t.Parallel()
	c := wallCave(t)
	c.wallFight(t)
	c.h.roller.queue(20, 1, 1, 1, 1, 1)
	key := newKey()
	cast := func() *playv1.CastSpellResponse {
		res, err := c.castWall(t, 4, func(r *playv1.CastSpellRequest) {
			northWall(playv1.WallSide_WALL_SIDE_LEFT)(r)
			r.IdempotencyKey = key
		})
		if err != nil {
			t.Fatalf("CastSpell() error = %v", err)
		}
		return res
	}
	first, retry := cast(), cast()
	for name, res := range map[string]*playv1.CastSpellResponse{"cast": first, "retry": retry} {
		got := res.GetArea().GetSquares()
		if len(got) != 2 || got[0].GetCol() != 7 || got[1].GetCol() != 7 {
			t.Errorf("area of the %s = %v, want the squares (7,7) and (7,8)", name, got)
		}
	}
	if zs := c.get(t, c.master).GetZones(); len(zs) != 1 {
		t.Errorf("zones after the retry = %d, want 1", len(zs))
	}
}

func TestAPreviewOfTheWallShowsTheSquaresTheCastLays(t *testing.T) {
	t.Parallel()
	c := wallCave(t)
	c.wallFight(t)
	wall := &playv1.SpellWall{Direction: toward(0, -1), Side: playv1.WallSide_WALL_SIDE_LEFT}
	res, err := c.ana.combat.PreviewSpellArea(t.Context(), connect.NewRequest(&playv1.PreviewSpellAreaRequest{
		CampaignId: c.campaignID, EncounterId: c.get(t, c.master).GetId(), CasterId: c.id(t, "Pensantus"), SpellKey: wallOfFireKey,
		Slot: slotOfLevel(4), Area: &playv1.PreviewSpellAreaRequest_Origin{Origin: at(7, 8)}, Wall: wall,
	}))
	if err != nil {
		t.Fatalf("PreviewSpellArea() error = %v", err)
	}
	if got := labelsOfTargets(res.Msg); !slices.Equal(got, []string{"Ogro"}) {
		t.Errorf("targets = %v, want the ogre", got)
	}
	c.h.roller.queue(20, 1, 1, 1, 1, 1)
	if _, err := c.castWall(t, 4, northWall(playv1.WallSide_WALL_SIDE_LEFT)); err != nil {
		t.Fatalf("CastSpell() error = %v", err)
	}
	z := c.oneZone(t, c.master, wallOfFireKey)
	if len(res.Msg.GetSquares()) != len(z.GetSquares()) {
		t.Fatalf("preview squares = %v, zone squares = %v", res.Msg.GetSquares(), z.GetSquares())
	}
	for _, s := range res.Msg.GetSquares() {
		if !zoneHas(z, s.GetCol(), s.GetRow()) {
			t.Errorf("the preview has %v, which the zone does not", s)
		}
	}
	if got, want := squaresOf(res.Msg.GetBurningSquares()), squaresOf(z.GetBurningSquares()); len(want) == 0 || !slices.Equal(got, want) {
		t.Errorf("preview burning squares = %v, want the zone's %v", got, want)
	}
	if res.Msg.GetLengthFt() != wallLineFt || z.GetWallSide() != playv1.WallSide_WALL_SIDE_LEFT {
		t.Errorf("length_ft = %d, wall_side = %v, want the full %d and LEFT", res.Msg.GetLengthFt(), z.GetWallSide(), wallLineFt)
	}
	_, err = c.ana.combat.PreviewSpellArea(t.Context(), connect.NewRequest(&playv1.PreviewSpellAreaRequest{
		CampaignId: c.campaignID, EncounterId: c.get(t, c.master).GetId(), CasterId: c.id(t, "Pensantus"), SpellKey: wallOfFireKey,
		Slot: slotOfLevel(4), Area: &playv1.PreviewSpellAreaRequest_Origin{Origin: at(7, 8)},
	}))
	wantBlockedBy(t, "PreviewSpellArea without a wall", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_WALL_PLACEMENT_REQUIRED)
}

// The ring around (19,7) holds the goblin, moved to (21,8), and is told to the viewer as a sphere of its radius.
func TestARingedWallIsLaidAroundThePoint(t *testing.T) {
	t.Parallel()
	c := wallCave(t)
	c.wallFight(t)
	c.mustMove(t, c.master, "Goblin", 21, 8)
	c.h.roller.queue(5, 5, 5, 5, 5, 5)
	_, err := c.castArea(t, c.ana, "Pensantus", wallOfFireKey, slotOfLevel(4), at(19, 7), nil, func(r *playv1.CastSpellRequest) {
		r.Wall = &playv1.SpellWall{Ring: true, Side: playv1.WallSide_WALL_SIDE_OUTER}
	})
	if err != nil {
		t.Fatalf("CastSpell() error = %v", err)
	}
	z := c.oneZone(t, c.master, wallOfFireKey)
	if len(z.GetSquares()) != 16 || zoneHas(z, 19, 7) || !zoneHas(z, 21, 7) || !zoneHas(z, 21, 8) {
		t.Errorf("squares = %v, want the 16 squares 10 ft from (19,7)", z.GetSquares())
	}
	if z.GetShape() != rulesv1.SpellAreaShape_SPELL_AREA_SHAPE_SPHERE || z.GetSizeFt() != 10 {
		t.Errorf("shape %v size %d, want a sphere of 10 ft: the ring's radius", z.GetShape(), z.GetSizeFt())
	}
	if cur, _, _ := c.hp(t, "Goblin"); cur > 0 {
		t.Errorf("the goblin has %d hit points, want it down: it stands in the ring", cur)
	}
}

func TestAStraightWallIsToldAsALineOfItsLength(t *testing.T) {
	t.Parallel()
	c := wallCave(t)
	c.wallFight(t)
	c.h.roller.queue(20, 1, 1, 1, 1, 1)
	_, err := c.castWall(t, 4, func(r *playv1.CastSpellRequest) {
		r.Wall = &playv1.SpellWall{Direction: toward(0, -1), Side: playv1.WallSide_WALL_SIDE_LEFT, LengthFt: 25}
	})
	if err != nil {
		t.Fatalf("CastSpell() error = %v", err)
	}
	if z := c.oneZone(t, c.master, wallOfFireKey); z.GetShape() != rulesv1.SpellAreaShape_SPELL_AREA_SHAPE_LINE || z.GetSizeFt() != 25 {
		t.Errorf("shape %v size %d, want a line of 25 ft", z.GetShape(), z.GetSizeFt())
	}
}

// A player's character inside the wall at the cast is asked its save, and a failure leaves the damage for the master.
func TestAPlayerInsideTheWallIsAskedTheSaveAtTheCast(t *testing.T) {
	t.Parallel()
	c := wallCave(t)
	c.wallFight(t)
	c.mustCastArea(t, c.ana, "Pensantus", wallOfFireKey, slotOfLevel(4), at(5, 8), nil, northWall(playv1.WallSide_WALL_SIDE_LEFT))
	w := c.windowOf(t, c.caio, playv1.ReactionKind_REACTION_KIND_EFFECT_SAVE)
	if w == nil {
		t.Fatal("Toren's player has no question: the wall caught his character at the cast")
	}
	if p := w.GetEffectSave(); p.GetZoneReason() != "zone_cast" || p.GetAbility() != "dex" || !p.GetHalfOnSave() {
		t.Errorf("prompt = %v, want a Dexterity zone save at the cast with half on a pass", p)
	}
	c.h.roller.queue(4, 4, 4, 4, 4) // 5d8 = 20
	if _, err := c.rollEffectSave(t, c.caio, c.get(t, c.master), w.GetId(), effectSaveFace(1)); err != nil {
		t.Fatalf("RollEffectSave() error = %v", err)
	}
	var amount int32
	if err := c.h.pool.QueryRow(t.Context(), `SELECT amount FROM pending_damages WHERE target_id = $1 AND effect_source_key = $2`, c.id(t, "Toren"), wallOfFireKey).Scan(&amount); err != nil || amount != 20 {
		t.Errorf("pending damage = %d, %v, want 20", amount, err)
	}
}

func TestAWallWithNoDirectionLaysNothingInTheGeometry(t *testing.T) {
	t.Parallel()
	if got := (wallSpec{LengthFt: 60}).squares(wsq(5, 5)); len(got) != 0 {
		t.Errorf("squares = %v, want none for a wall that runs nowhere", got)
	}
}

func TestWallSideIsStoredAndReadBack(t *testing.T) {
	t.Parallel()
	c := wallCave(t)
	c.wallFight(t)
	c.h.roller.queue(20, 1, 1, 1, 1, 1)
	if _, err := c.castWall(t, 4, northWall(playv1.WallSide_WALL_SIDE_LEFT)); err != nil {
		t.Fatalf("CastSpell() error = %v", err)
	}
	zs, err := readZonesOf(t.Context(), c.h.svc.queries, c.get(t, c.master).GetId())
	if err != nil || len(zs) != 1 {
		t.Fatalf("readZonesOf = %v, %v, want one zone", zs, err)
	}
	z := zs[0]
	if z.wall == nil || z.wall.Side != playv1.WallSide_WALL_SIDE_LEFT || z.wall.Dir != (grid.Direction{Dy: -1}) || z.wall.LengthFt != 60 {
		t.Fatalf("wall read back = %+v, want left, north, 60 ft", z.wall)
	}
	// Walking north the left is west: Toren's square (5,7) is within 10 ft of the wall on that side.
	for sqr, want := range map[grid.Square]bool{wsq(7, 7): true, wsq(6, 8): true, wsq(5, 7): true, wsq(8, 7): false, wsq(9, 8): false, wsq(4, 7): false} {
		if z.hurtsAtEnd(sqr) != want {
			t.Errorf("hurtsAtEnd(%v) = %v, want %v", sqr, z.hurtsAtEnd(sqr), want)
		}
	}
}

func TestWallOfFireIsAvailableInTheTurnOptions(t *testing.T) {
	t.Parallel()
	c := wallCave(t)
	e := c.zoneFight(t)
	e = c.passTo(t, e, "Pensantus")
	opt := spellOption(c.mustOptions(t, c.ana, e, "Pensantus"), wallOfFireKey)
	if opt == nil || !opt.GetEnabled() {
		t.Fatalf("Wall of Fire option = %v, want enabled: the cast sheet sends the wall", opt)
	}
	if o := spellOption(c.mustOptions(t, c.ana, e, "Pensantus"), cloudkillKey); o == nil || !o.GetEnabled() {
		t.Errorf("Cloudkill option = %v, want enabled", o)
	}
}

// The later hits of the wall deal the damage dice with no saving throw to a creature that enters the wall or ends its
// turn inside it or within 10 ft of its damaging side. The wall is cast at (9,8) running north (squares (9,8) and
// (9,7), the map's wall ends it at (9,6)); LEFT is the west.

// wallHitsOn are the dice counts of the wall's damages waiting to be rolled for the creature.
func (c *cave) wallHitsOn(t *testing.T, label string) []int {
	t.Helper()
	return c.pendingDiceOf(t, label, wallOfFireKey)
}

// layWallWithOgreAt puts the ogre on the square, has Pensantus lay the wall and passes to the ogre's turn.
// The ogre is quick, so the master can walk it about.
func (c *cave) layWallWithOgreAt(t *testing.T, col, row int32, slot int32, edit func(*playv1.CastSpellRequest), origin *playv1.SpellOrigin, rolls ...int) *playv1.Encounter {
	t.Helper()
	e := c.zoneFight(t)
	c.passTo(t, e, "Pensantus")
	c.mustMove(t, c.master, "Ogro", col, row)
	c.execSQL(t, `UPDATE combatants SET speed_ft = 200 WHERE id = $1`, c.id(t, "Ogro"))
	if len(rolls) > 0 {
		c.h.roller.queue(rolls...) // the saves and the damage of a creature the wall is cast on
	}
	c.mustCastArea(t, c.ana, "Pensantus", wallOfFireKey, slotOfLevel(slot), origin, nil, edit)
	return c.passTo(t, c.get(t, c.master), "Ogro")
}

// layNorthWallWithOgreAt is layWallWithOgreAt for the wall from (9,8) to the north.
// ogreInWall says the ogre stands in the wall at the cast: it saves and passes, and the dice are queued for it.
func (c *cave) layNorthWallWithOgreAt(t *testing.T, col, row int32, slot int32, ogreInWall bool) *playv1.Encounter {
	t.Helper()
	var rolls []int
	if ogreInWall {
		rolls = []int{20, 1, 1, 1, 1, 1, 1}
	}
	return c.layWallWithOgreAt(t, col, row, slot, northWall(playv1.WallSide_WALL_SIDE_LEFT), at(9, 8), rolls...)
}

func TestTheWallHurtsWhoEndsItsTurnWhereItReaches(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name     string
		col, row int32
		hits     []int
	}{
		{"inside the wall", 9, 7, []int{5}},
		{"inside the wall and within 10 ft: once", 9, 8, []int{5}},
		{"10 ft from the damaging side", 7, 7, []int{5}},
		{"5 ft from the damaging side", 8, 7, []int{5}},
		{"15 ft from the damaging side", 6, 7, nil},
		{"5 ft on the other side", 10, 7, nil},
		{"10 ft on the other side", 11, 7, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := wallCave(t)
			e := c.layNorthWallWithOgreAt(t, tt.col, tt.row, 4, tt.col == 9)
			if got := c.wallHitsOn(t, "Ogro"); len(got) != 0 {
				t.Fatalf("hits before the end of the turn = %v, want none: it began its turn there", got)
			}
			c.mustEndTurn(t, c.master, e)
			if got := c.wallHitsOn(t, "Ogro"); !slices.Equal(got, tt.hits) {
				t.Errorf("hits at the end of the turn = %v, want %v", got, tt.hits)
			}
		})
	}
}

func TestTheWallHurtsWithTheDiceOfTheSlot(t *testing.T) {
	t.Parallel()
	c := wallCave(t)
	e := c.layNorthWallWithOgreAt(t, 9, 7, 5, true)
	c.mustEndTurn(t, c.master, e)
	if got := c.wallHitsOn(t, "Ogro"); !slices.Equal(got, []int{6}) {
		t.Errorf("hits = %v, want one of 6 dice: the 5th level slot", got)
	}
}

// Nobody is spared: the caster's ally and the caster take it too, and it asks no save.
func TestTheWallSparesNobodyAndAsksNoSave(t *testing.T) {
	t.Parallel()
	c := wallCave(t)
	e := c.zoneFight(t)
	c.passTo(t, e, "Pensantus")
	c.mustMove(t, c.master, "Pensantus", 8, 7) // the caster, west of the wall
	c.mustCastArea(t, c.ana, "Pensantus", wallOfFireKey, slotOfLevel(4), at(9, 8), nil, northWall(playv1.WallSide_WALL_SIDE_LEFT))
	c.passTo(t, c.get(t, c.master), "Brisa")
	if got := c.wallHitsOn(t, "Pensantus"); !slices.Equal(got, []int{5}) {
		t.Errorf("hits on the caster = %v, want one: it ended its turn 5 ft from the damaging side", got)
	}
	c.mustMove(t, c.master, "Brisa", 7, 7)
	c.mustEndTurn(t, c.master, c.get(t, c.master))
	if got := c.wallHitsOn(t, "Brisa"); !slices.Equal(got, []int{5}) {
		t.Errorf("hits on Brisa = %v, want one", got)
	}
	if w := c.windowOf(t, c.bia, playv1.ReactionKind_REACTION_KIND_EFFECT_SAVE); w != nil {
		t.Errorf("Brisa is asked a save: %v, want none after the cast", w)
	}
}

func TestEnteringTheWallHurtsOnceAndEndingTheTurnThereHurtsAgain(t *testing.T) {
	t.Parallel()
	c := wallCave(t)
	e := c.layNorthWallWithOgreAt(t, 12, 7, 4, false)
	c.mustMove(t, c.master, "Ogro", 9, 7)
	if got := c.wallHitsOn(t, "Ogro"); !slices.Equal(got, []int{5}) {
		t.Fatalf("hits after walking in = %v, want one", got)
	}
	c.mustMove(t, c.master, "Ogro", 9, 8) // moving about inside the wall hurts no more
	if got := c.wallHitsOn(t, "Ogro"); len(got) != 1 {
		t.Fatalf("hits after moving inside = %v, want still one", got)
	}
	c.mustEndTurn(t, c.master, e)
	if got := c.wallHitsOn(t, "Ogro"); !slices.Equal(got, []int{5, 5}) {
		t.Errorf("hits after the end of the turn = %v, want two: one for entering and one for the turn end", got)
	}
	if got := c.zoneReasons(t, wallOfFireKey); !slices.Equal(got, []string{"zone_enter", "zone_end"}) {
		t.Errorf("reasons of the lines = %v, want zone_enter then zone_end", got)
	}
}

func TestLeavingTheWallAndEnteringAgainHurtsOnce(t *testing.T) {
	t.Parallel()
	c := wallCave(t)
	c.layNorthWallWithOgreAt(t, 12, 7, 4, false)
	for _, col := range []int32{9, 12, 9} {
		c.mustMove(t, c.master, "Ogro", col, 7)
	}
	if got := c.wallHitsOn(t, "Ogro"); !slices.Equal(got, []int{5}) {
		t.Errorf("hits = %v, want one: entering twice on a turn hurts once", got)
	}
}

func TestBeingPushedIntoTheWallHurtsAsEntering(t *testing.T) {
	t.Parallel()
	c := wallCave(t)
	e := c.layNorthWallWithOgreAt(t, 11, 7, 4, false)
	c.mustEndTurn(t, c.master, e) // the ogre is east of the wall: nothing yet
	c.nextTurnOf(t, "Pensantus")
	if got := c.wallHitsOn(t, "Ogro"); len(got) != 0 {
		t.Fatalf("hits before the push = %v, want none", got)
	}
	c.h.roller.queue(1)
	c.mustCastArea(t, c.ana, "Pensantus", thunderwave, slotOfLevel(1), nil, toward(-1, 0))
	if got := c.wallHitsOn(t, "Ogro"); !slices.Equal(got, []int{5}) {
		t.Errorf("hits after the push = %v, want one: pushed from 11 to 9, into the wall", got)
	}
}

func TestTheRingHurtsOnItsDamagingSideAtTheEndOfTheTurn(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name     string
		side     playv1.WallSide
		col, row int32
		want     int
	}{
		{"inner side, inside", playv1.WallSide_WALL_SIDE_INNER, 18, 7, 1},
		{"inner side, outside", playv1.WallSide_WALL_SIDE_INNER, 22, 7, 0},
		{"outer side, inside", playv1.WallSide_WALL_SIDE_OUTER, 18, 7, 0},
		{"outer side, outside", playv1.WallSide_WALL_SIDE_OUTER, 22, 7, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := wallCave(t)
			ring := func(r *playv1.CastSpellRequest) { r.Wall = &playv1.SpellWall{Ring: true, Side: tt.side} }
			e := c.layWallWithOgreAt(t, tt.col, tt.row, 4, ring, at(19, 7), 20, 1, 1, 1, 1, 1) // the goblin in the ring saves
			c.mustEndTurn(t, c.master, e)
			if got := c.wallHitsOn(t, "Ogro"); len(got) != tt.want {
				t.Errorf("hits = %v, want %d", got, tt.want)
			}
		})
	}
}

// insertWallOf puts a second wall on the map, like the first one but the zone of another caster.
func (c *cave) insertWallOf(t *testing.T, owner string) {
	t.Helper()
	e := c.get(t, c.master)
	_, err := playdb.New(c.h.pool).InsertSpellZone(t.Context(), playdb.InsertSpellZoneParams{
		EncounterID: e.GetId(), OwnerID: c.id(t, owner), SpellKey: wallOfFireKey, CastID: newKey(), SlotLevel: 4,
		OriginCol: 9, OriginRow: 8, Squares: squaresToJSON([]grid.Square{{Col: 9, Row: 8}, {Col: 9, Row: 7}}),
		SaveAbility: "dex", SaveDc: 15, DamageDice: "5d8", DamageType: "damage-type:fire", StartedRound: e.GetRound(), CreatedAt: time.Now(),
		WallSide: "left", WallDx: 0, WallDy: -1, WallLengthFt: 60,
	})
	if err != nil {
		t.Fatalf("InsertSpellZone() error = %v", err)
	}
}

func TestTwoWallsHurtOneCreatureTwiceAtTheEndOfTheTurn(t *testing.T) {
	t.Parallel()
	c := wallCave(t)
	e := c.layNorthWallWithOgreAt(t, 8, 7, 4, false)
	c.insertWallOf(t, "Brisa")
	c.mustEndTurn(t, c.master, e)
	if got := c.wallHitsOn(t, "Ogro"); !slices.Equal(got, []int{5, 5}) {
		t.Errorf("hits = %v, want two: one for each wall", got)
	}
}

// The cast and the end of the turn mark the caster apart, so both ask.
func TestTheCasterInItsOwnWallSavesAtTheCastAndIsHurtAtTheEndOfTheTurn(t *testing.T) {
	t.Parallel()
	c := wallCave(t)
	e := c.zoneFight(t)
	c.passTo(t, e, "Pensantus")
	c.mustMove(t, c.master, "Pensantus", 9, 7)
	c.mustCastArea(t, c.ana, "Pensantus", wallOfFireKey, slotOfLevel(4), at(9, 8), nil, northWall(playv1.WallSide_WALL_SIDE_LEFT))
	w := c.windowOf(t, c.ana, playv1.ReactionKind_REACTION_KIND_EFFECT_SAVE)
	if w == nil {
		t.Fatal("the caster standing in her own wall is not asked the save at the cast")
	}
	c.h.roller.queue(4, 4, 4, 4, 4)
	if _, err := c.rollEffectSave(t, c.ana, c.get(t, c.master), w.GetId(), effectSaveFace(1)); err != nil {
		t.Fatalf("RollEffectSave() error = %v", err)
	}
	if got := c.wallHitsOn(t, "Pensantus"); len(got) != 0 {
		t.Fatalf("hits before the end of the turn = %v, want none", got)
	}
	if _, err := c.endTurn(t, c.master, c.get(t, c.master), true); err != nil {
		t.Fatalf("EndTurn() error = %v", err)
	}
	if got := c.wallHitsOn(t, "Pensantus"); !slices.Equal(got, []int{5}) {
		t.Errorf("hits at the end of the turn = %v, want one", got)
	}
}

func TestADefeatedCreatureIsNotHurtByTheWall(t *testing.T) {
	t.Parallel()
	c := wallCave(t)
	c.layNorthWallWithOgreAt(t, 12, 7, 4, false)
	c.execSQL(t, `UPDATE combatants SET defeated = true WHERE id = $1`, c.id(t, "Ogro"))
	c.mustMove(t, c.master, "Ogro", 9, 7)
	if got := c.wallHitsOn(t, "Ogro"); len(got) != 0 {
		t.Errorf("hits = %v, want none for a creature that is out of the fight", got)
	}
}

// eastWall is the wall from (18,3) to the south: the east room holds all of it, and nobody stands near its right side.
func eastWall(r *playv1.CastSpellRequest) {
	r.Wall = &playv1.SpellWall{Direction: toward(0, 1), Side: playv1.WallSide_WALL_SIDE_RIGHT}
}

func (c *cave) castEastWall(t *testing.T) *playv1.SpellZone {
	t.Helper()
	c.passTo(t, c.zoneFight(t), "Pensantus")
	c.mustCastArea(t, c.ana, "Pensantus", wallOfFireKey, slotOfLevel(4), at(18, 3), nil, eastWall)
	return c.oneZone(t, c.master, wallOfFireKey)
}

// wantCloudsOf fails unless the cloud squares of the map are exactly the squares of the zone.
func wantCloudsOf(t *testing.T, c *cave, z *playv1.SpellZone) {
	t.Helper()
	var want []grid.Square
	for _, s := range z.GetSquares() {
		want = append(want, wsq(int(s.GetCol()), int(s.GetRow())))
	}
	clouds, dark := c.combatPositions(t)
	if len(want) == 0 || !maps.Equal(setOf(clouds), setOf(want)) {
		t.Errorf("cloud squares = %v, want the %d squares of the wall %v", clouds, len(want), want)
	}
	if len(dark) != 0 {
		t.Errorf("magic dark squares = %v, want none: the wall is no darkness", dark)
	}
}

// The wall's squares block sight like a Fog Cloud's, for a straight wall and for a ring.
func TestTheWallsSquaresAreCloudSquaresOfTheFog(t *testing.T) {
	t.Parallel()
	t.Run("straight", func(t *testing.T) {
		t.Parallel()
		c := wallCave(t)
		wantCloudsOf(t, c, c.castEastWall(t))
	})
	t.Run("ring", func(t *testing.T) {
		t.Parallel()
		c := wallCave(t)
		c.wallFight(t)
		c.mustCastArea(t, c.ana, "Pensantus", wallOfFireKey, slotOfLevel(4), at(19, 7), nil, func(r *playv1.CastSpellRequest) {
			r.Wall = &playv1.SpellWall{Ring: true, Side: playv1.WallSide_WALL_SIDE_INNER}
		})
		z := c.oneZone(t, c.master, wallOfFireKey)
		if len(z.GetSquares()) != 16 {
			t.Fatalf("ring squares = %d, want 16", len(z.GetSquares()))
		}
		wantCloudsOf(t, c, z)
	})
}

// The wall hurts but does not stop: a creature walks through it.
func TestTheWallDoesNotBlockMovement(t *testing.T) {
	t.Parallel()
	c := wallCave(t)
	c.wallFight(t)
	c.mustCastArea(t, c.ana, "Pensantus", wallOfFireKey, slotOfLevel(4), at(9, 8), nil, northWall(playv1.WallSide_WALL_SIDE_LEFT))
	c.execSQL(t, `UPDATE combatants SET speed_ft = 200 WHERE id = $1`, c.id(t, "Ogro"))
	c.h.roller.queue(1, 1, 1, 1, 1, 1) // the ogre's save and the damage of the square it crosses
	c.mustMove(t, c.master, "Ogro", 11, 7)
	if o := byLabel(t, c.get(t, c.master), "Ogro"); o.GetCol() != 11 || o.GetRow() != 7 {
		t.Errorf("the ogre is at (%d,%d), want (11,7): the wall stopped it", o.GetCol(), o.GetRow())
	}
}

// A hidden caster's wall hides what a hidden caster's Fog Cloud hides: the squares are the fog's and the zone is the master's.
func TestAHiddenCastersWallBlocksSightLikeAnyCloud(t *testing.T) {
	t.Parallel()
	c := newCaveWith(t, 5, []string{thunderwave})
	c.druidFightOf(t, 9, []string{wallOfFireKey})
	c.mustCastArea(t, c.master, "Druida", wallOfFireKey, slotOfLevel(4), at(18, 3), nil, eastWall)
	z := c.oneZone(t, c.master, wallOfFireKey)
	if z.GetVisibility() != playv1.SpellZoneVisibility_SPELL_ZONE_VISIBILITY_MASTER_ONLY {
		t.Errorf("visibility = %v, want MASTER_ONLY: the caster is hidden", z.GetVisibility())
	}
	wantCloudsOf(t, c, z)
}

// Whatever ends the wall, the fog hears of it and the wall's squares stop blocking sight.
func TestTheFogHearsOfAWallThatEnds(t *testing.T) {
	t.Parallel()
	ends := []struct {
		name string
		end  func(t *testing.T, c *cave, z *playv1.SpellZone)
	}{
		{"the caster loses the concentration", func(t *testing.T, c *cave, _ *playv1.SpellZone) {
			t.Helper()
			c.correct(t, c.pens, hpIs(0))
		}},
		{"the time passes", func(t *testing.T, c *cave, _ *playv1.SpellZone) {
			t.Helper()
			for range 10 {
				c.nextTurnOf(t, "Pensantus")
			}
		}},
		{"the master ends it", func(t *testing.T, c *cave, z *playv1.SpellZone) {
			t.Helper()
			if _, err := c.master.combat.EndSpellZone(t.Context(), connect.NewRequest(&playv1.EndSpellZoneRequest{
				CampaignId: c.campaignID, EncounterId: c.get(t, c.master).GetId(), ZoneId: z.GetId(), IdempotencyKey: newKey(),
			})); err != nil {
				t.Fatalf("EndSpellZone() error = %v", err)
			}
		}},
		{"the master undoes the cast", func(t *testing.T, c *cave, _ *playv1.SpellZone) {
			t.Helper()
			c.undoLast(t)
		}},
	}
	for _, tc := range ends {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := wallCave(t)
			z := c.castEastWall(t)
			if clouds, _ := c.combatPositions(t); len(clouds) == 0 {
				t.Fatal("the cast put no cloud squares on the map")
			}
			before := c.h.vision.n.Load()
			tc.end(t, c, z)
			if got := c.h.vision.n.Load(); got <= before {
				t.Errorf("fog refreshes after the wall ended = %d, before %d: the fog was not told", got, before)
			}
			if clouds, _ := c.combatPositions(t); len(clouds) != 0 {
				t.Errorf("cloud squares after = %d, want none", len(clouds))
			}
		})
	}
}

// The cast itself tells the fog.
func TestTheFogHearsOfTheWallCast(t *testing.T) {
	t.Parallel()
	c := wallCave(t)
	c.wallFight(t)
	before := c.h.vision.n.Load()
	c.mustCastArea(t, c.ana, "Pensantus", wallOfFireKey, slotOfLevel(4), at(18, 3), nil, eastWall)
	if got := c.h.vision.n.Load(); got <= before {
		t.Errorf("fog refreshes after the cast = %d, before %d: the fog was not told", got, before)
	}
}

// A wall is not moved: the master's move is refused and the wall keeps its squares.
func TestTheMasterCannotMoveAWall(t *testing.T) {
	t.Parallel()
	c := wallCave(t)
	z := c.castEastWall(t)
	_, err := c.moveZone(t, c.master, z.GetId(), 12, 7)
	if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("MoveSpellZone(wall) error = %v, want failed_precondition", err)
	}
	if now := c.oneZone(t, c.master, wallOfFireKey); len(now.GetSquares()) != len(z.GetSquares()) || len(now.GetSquares()) == 0 {
		t.Errorf("the wall has %d squares after the refused move, want its %d", len(now.GetSquares()), len(z.GetSquares()))
	}
	wantCloudsOf(t, c, z)
}

// A spell that lays a wall is placed on a point, whatever the shape and the range the spell's own data give it: the
// turn options say so, so the cast sheet asks "Onde fica" before the cast (the cast itself sets ZoneLaid).
func TestAWallSpellIsPlacedOnAPointInTheTurnOptions(t *testing.T) {
	t.Parallel()
	sp := link.Spell{Key: wallOfFireKey, AreaShape: string(grid.ShapeLine), AreaSizeFt: wallLineFt, RangeKind: rules.RangeRanged}
	if got := placementFor(sp, false); got != playv1.AreaPlacement_AREA_PLACEMENT_POINT {
		t.Errorf("placementFor(Wall of Fire) = %v, want POINT", got)
	}
	if got, want := areaShapeProto(sp, false), rulesv1.SpellAreaShape_SPELL_AREA_SHAPE_LINE; got != want {
		t.Errorf("areaShapeProto = %v, want %v", got, want)
	}
	if got := areaSizeFor(sp, false); got != wallLineFt {
		t.Errorf("areaSizeFor = %d, want %d", got, wallLineFt)
	}
	if got := placementFor(sp, true); got != playv1.AreaPlacement_AREA_PLACEMENT_UNSPECIFIED {
		t.Errorf("placementFor(Wall of Fire) in a combat with no map = %v, want UNSPECIFIED", got)
	}
}
