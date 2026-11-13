package play

import (
	"encoding/json"
	"slices"
	"testing"

	"google.golang.org/protobuf/proto"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// A player reads a zone's squares only where they see now (RN-10): a square they explored and no longer see,
// or never saw, is not in the answer. These tests need no database: the roster and the sight are the small fakes
// of combat_reaction_counterspell_sight_test.go.

func zoneRowOf(t *testing.T, key string, squares []grid.Square, wall *wallSpec) playdb.SpellZone {
	t.Helper()
	raw := make([][2]int, len(squares))
	for i, sq := range squares {
		raw[i] = [2]int{sq.Col, sq.Row}
	}
	b, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	row := playdb.SpellZone{ID: "zone-1", OwnerID: "caster", SpellKey: key, OriginCol: clamp32(squares[0].Col, 0, 1000), OriginRow: clamp32(squares[0].Row, 0, 1000), Squares: b}
	if w := storedWall(wall); wall != nil {
		row.WallSide, row.WallDx, row.WallDy, row.WallLengthFt = w.side, w.dx, w.dy, w.lengthFt
	}
	return row
}

// sightData is a combat with one visible NPC caster and the zone; the viewer is a player who sees the squares.
func sightData(t *testing.T, row playdb.SpellZone) *encounterData {
	t.Helper()
	z, ok := readZone(row)
	if !ok {
		t.Fatalf("readZone(%q) is no zone", row.SpellKey)
	}
	caster := combatantAt("caster", kindNPC, 1, 1)
	return &encounterData{enc: playdb.Encounter{Status: statusActive}, cs: []playdb.Combatant{caster}, zones: []*zone{z}}
}

func playerSeeing(seen ...grid.Square) combatViewer {
	f := &sightFake{seen: map[grid.Square]bool{}}
	for _, sq := range seen {
		f.seen[sq] = true
	}
	return combatViewer{userID: "user-1", sight: &fogSight{sight: f}}
}

func squaresOf(out []*playv1.SpellOrigin) []grid.Square {
	var got []grid.Square
	for _, o := range out {
		got = append(got, grid.Square{Col: int(o.GetCol()), Row: int(o.GetRow())})
	}
	return got
}

func TestAPlayerReadsOnlyTheSquaresOfAZoneTheySeeNow(t *testing.T) {
	t.Parallel()
	all := []grid.Square{{Col: 3, Row: 3}, {Col: 4, Row: 3}, {Col: 5, Row: 3}}
	d := sightData(t, zoneRowOf(t, moonbeamKey, all, nil))

	zs := d.zonesView(playerSeeing(grid.Square{Col: 3, Row: 3}, grid.Square{Col: 4, Row: 3}), nil)
	if len(zs) != 1 {
		t.Fatalf("zones = %d, want 1: the player sees part of it", len(zs))
	}
	if got := squaresOf(zs[0].GetSquares()); len(got) != 2 || got[0] != all[0] || got[1] != all[1] {
		t.Errorf("squares = %v, want (3,3) and (4,3): (5,3) is not seen now", got)
	}

	whole := d.zonesView(playerSeeing(all...), nil)
	if len(whole) != 1 || len(whole[0].GetSquares()) != 3 {
		t.Errorf("a player who sees every square reads %v, want the whole zone", whole)
	}
}

func TestAZoneWithNoSquareSeenIsNotSent(t *testing.T) {
	t.Parallel()
	d := sightData(t, zoneRowOf(t, moonbeamKey, []grid.Square{{Col: 3, Row: 3}, {Col: 4, Row: 3}}, nil))
	if zs := d.zonesView(playerSeeing(grid.Square{Col: 9, Row: 9}), nil); len(zs) != 0 {
		t.Errorf("zones = %v, want none: the player sees no square of it now", zs)
	}
}

func TestTheMasterAndTheCastersOwnPlayerReadTheWholeZone(t *testing.T) {
	t.Parallel()
	all := []grid.Square{{Col: 3, Row: 3}, {Col: 4, Row: 3}, {Col: 5, Row: 3}}
	d := sightData(t, zoneRowOf(t, moonbeamKey, all, nil))

	if zs := d.zonesView(combatViewer{master: true}, nil); len(zs) != 1 || len(zs[0].GetSquares()) != 3 {
		t.Errorf("the master reads %v, want the whole zone: the positive control", zs)
	}
	user := "user-1"
	d.cs[0].UserID = &user
	if zs := d.zonesView(playerSeeing(), nil); len(zs) != 1 || len(zs[0].GetSquares()) != 3 {
		t.Errorf("the caster's player reads %v, want their own whole zone", zs)
	}
}

func TestAMapWithoutFogSendsTheWholeZone(t *testing.T) {
	t.Parallel()
	d := sightData(t, zoneRowOf(t, moonbeamKey, []grid.Square{{Col: 3, Row: 3}, {Col: 4, Row: 3}}, nil))
	if zs := d.zonesView(combatViewer{userID: "user-1"}, nil); len(zs) != 1 || len(zs[0].GetSquares()) != 2 {
		t.Errorf("zones = %v, want the whole zone: no fog, no hidden square", zs)
	}
}

// The burning side of a wall reaches the master whole and a player only where they see now.
func TestTheBurningSideIsReadWhereTheViewerMay(t *testing.T) {
	t.Parallel()
	w := &wallSpec{Dir: grid.Direction{Dx: 1}, LengthFt: 15, Side: playv1.WallSide_WALL_SIDE_RIGHT}
	origin := grid.Square{Col: 5, Row: 5}
	wall := w.squares(origin)
	d := sightData(t, zoneRowOf(t, wallOfFireKey, wall, w))

	master := d.zonesView(combatViewer{master: true}, nil)
	if len(master) != 1 || master[0].GetWallSide() != playv1.WallSide_WALL_SIDE_RIGHT || len(master[0].GetBurningSquares()) == 0 {
		t.Fatalf("the master reads %v, want the wall, its side and its burning squares", master)
	}
	for _, sq := range squaresOf(master[0].GetBurningSquares()) {
		if sq.Row <= origin.Row {
			t.Errorf("burning square %v is not south of the wall: the right of a wall that runs east", sq)
		}
		for _, own := range wall {
			if sq == own {
				t.Errorf("burning square %v is one of the wall's own squares", sq)
			}
		}
	}

	north := grid.Square{Col: 5, Row: 4}
	player := d.zonesView(playerSeeing(wall...), nil)
	if len(player) != 1 || len(player[0].GetBurningSquares()) != 0 {
		t.Errorf("a player who sees only the wall reads burning squares %v, want none", player)
	}
	south := master[0].GetBurningSquares()[0]
	seen := d.zonesView(playerSeeing(append(wall, grid.Square{Col: int(south.GetCol()), Row: int(south.GetRow())}, north)...), nil)
	got := squaresOf(seen[0].GetBurningSquares())
	if len(got) != 1 || got[0] != (grid.Square{Col: int(south.GetCol()), Row: int(south.GetRow())}) {
		t.Errorf("burning squares = %v, want only the one the player sees now", got)
	}
}

// A cloud or a wall blocks sight, so the fog never counts its own squares as seen. A player reads the squares of it
// that face them: the ones next to a square they see now. The squares behind stay unread.
func TestAPlayerReadsTheFaceOfAWallThatBlocksTheirSight(t *testing.T) {
	t.Parallel()
	w := &wallSpec{Dir: grid.Direction{Dx: 1}, LengthFt: 25, Side: playv1.WallSide_WALL_SIDE_RIGHT}
	wall := w.squares(grid.Square{Col: 5, Row: 5}) // (5,5) to (9,5)
	d := sightData(t, zoneRowOf(t, wallOfFireKey, wall, w))

	zs := d.zonesView(playerSeeing(grid.Square{Col: 4, Row: 5}), nil) // the player stands west of its end
	if len(zs) != 1 {
		t.Fatalf("zones = %d, want the wall: its end faces the player", len(zs))
	}
	if got := squaresOf(zs[0].GetSquares()); len(got) != 1 || got[0] != (grid.Square{Col: 5, Row: 5}) {
		t.Errorf("squares = %v, want only (5,5), the end that faces the player", got)
	}
	if zs := d.zonesView(playerSeeing(grid.Square{Col: 12, Row: 12}), nil); len(zs) != 0 {
		t.Errorf("zones = %v, want none: the player sees nothing next to the wall", zs)
	}
}

// A zone that does not block sight has no face: a square next to a seen one is not read.
func TestAZoneThatDoesNotBlockSightHasNoFace(t *testing.T) {
	t.Parallel()
	d := sightData(t, zoneRowOf(t, moonbeamKey, []grid.Square{{Col: 5, Row: 5}, {Col: 6, Row: 5}}, nil))
	if zs := d.zonesView(playerSeeing(grid.Square{Col: 4, Row: 5}), nil); len(zs) != 0 {
		t.Errorf("zones = %v, want none: (5,5) is not seen and the beam blocks no sight", zs)
	}
}

// A zone half in the player's sight reads exactly like a zone whose squares are all the ones they see: the answer
// holds nothing of the squares in the dark, not their number, not the origin that lies there (RN-10).
func TestAZoneHalfInSightReadsLikeOneWhollyInSight(t *testing.T) {
	t.Parallel()
	seen := []grid.Square{{Col: 3, Row: 3}, {Col: 4, Row: 3}}
	dark := []grid.Square{{Col: 5, Row: 3}, {Col: 6, Row: 3}, {Col: 7, Row: 3}}
	viewer := playerSeeing(seen...)
	read := func(squares []grid.Square, origin grid.Square) *playv1.SpellZone {
		row := zoneRowOf(t, moonbeamKey, squares, nil)
		row.OriginCol, row.OriginRow = clamp32(origin.Col, 0, 1000), clamp32(origin.Row, 0, 1000)
		zs := sightData(t, row).zonesView(viewer, nil)
		if len(zs) != 1 {
			t.Fatalf("zones = %d, want 1", len(zs))
		}
		return zs[0]
	}
	more := append(slices.Clone(seen), dark...)
	for _, tt := range []struct {
		name          string
		squares       []grid.Square
		origin, whole grid.Square // the origin the zone has, and the one the zone wholly in sight has
	}{
		{"origin in sight", more, seen[1], seen[1]},
		{"origin in the dark", more, dark[1], seen[0]},
		{"another half", append(slices.Clone(seen), dark[0]), dark[0], seen[0]},
	} {
		if got, want := read(tt.squares, tt.origin), read(seen, tt.whole); !proto.Equal(got, want) {
			t.Errorf("%s reads %v, want what a zone wholly in sight reads: %v", tt.name, got, want)
		}
	}
}

// The length of a wall and the side that burns are not the unseen part's to tell: a player who sees the end of a wall reads a
// wall as long as the squares they read, and no side until they see a burning square (RN-10).
func TestAWallSeenByItsEndTellsNeitherItsLengthNorItsSide(t *testing.T) {
	t.Parallel()
	w := &wallSpec{Dir: grid.Direction{Dx: 1}, LengthFt: 60, Side: playv1.WallSide_WALL_SIDE_RIGHT}
	wall := w.squares(grid.Square{Col: 5, Row: 5})
	d := sightData(t, zoneRowOf(t, wallOfFireKey, wall, w))

	zs := d.zonesView(playerSeeing(grid.Square{Col: 4, Row: 5}), nil) // sees the end: only (5,5)
	if len(zs) != 1 {
		t.Fatalf("zones = %d, want the wall", len(zs))
	}
	if got := zs[0].GetSizeFt(); got != grid.FeetPerSquare {
		t.Errorf("size_ft = %d, want %d: the length of the squares read, not the wall's 60", got, grid.FeetPerSquare)
	}
	if zs[0].GetWallSide() != playv1.WallSide_WALL_SIDE_UNSPECIFIED || len(zs[0].GetBurningSquares()) != 0 {
		t.Errorf("wall_side = %v, burning = %v, want none: no burning square is in sight", zs[0].GetWallSide(), zs[0].GetBurningSquares())
	}

	south := grid.Square{Col: 6, Row: 6} // a burning square the player sees
	seen := d.zonesView(playerSeeing(grid.Square{Col: 4, Row: 5}, south), nil)
	if len(seen[0].GetBurningSquares()) == 0 || seen[0].GetWallSide() != playv1.WallSide_WALL_SIDE_RIGHT {
		t.Errorf("a player who sees a burning square reads %v with side %v, want both", seen[0].GetBurningSquares(), seen[0].GetWallSide())
	}

	whole := d.zonesView(playerSeeing(append(slices.Clone(wall), grid.Square{Col: 4, Row: 5})...), nil)
	if got := whole[0].GetSizeFt(); got != 60 {
		t.Errorf("size_ft = %d, want 60: the player reads the whole wall", got)
	}
}
