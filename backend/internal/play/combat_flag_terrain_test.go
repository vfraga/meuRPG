package play

import (
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// The ignore_difficult_terrain flag (#572): the difficult terrain painted on the map costs nothing extra, the one a spell
// makes still does. These tests need the database. Every name is invented.

// terrainRig is the cave with Toren, a druid whose table subclass has the flag effect, who casts Spike Growth; Pensantus casts
// Web. The cost of one square entered is read from the move options, from a square just outside each kind of difficult terrain.
type terrainRig struct {
	c     *cave
	entry *rulesv1.TableEntry
}

func newTerrainRig(t *testing.T, flag string) *terrainRig {
	t.Helper()
	r := &terrainRig{}
	r.c = newCaveOf(t, 9, []string{webKey}, func(a *armed) *charactersv1.Character {
		res, err := a.master.table.CreateTableEntry(t.Context(), connect.NewRequest(&rulesv1.CreateTableEntryRequest{
			CampaignId: a.campaignID, Body: &rulesv1.CreateTableEntryRequest_TableSubclass{TableSubclass: flagSubclass("class:druid", flagEffect(flag), "")},
		}))
		if err != nil {
			t.Fatalf("CreateTableEntry() error = %v", err)
		}
		r.entry = res.Msg.GetEntry()
		return a.caio.castingHero(t, a.campaignID, "Toren", classLevel("class:druid", 7, r.entry.GetKey()), abilities(12, 12, 14, 16, 10),
			nil, []string{spikeGrowthKey}, []string{spikeGrowthKey}, "")
	})
	r.c.terrain.paintDifficult(grid.Square{Col: 10, Row: 8})
	return r
}

// costsOfOne are what Toren pays to enter a square of the map's difficult terrain, of the spikes and of the web, each from
// the square next to it that is plain floor.
func (r *terrainRig) costsOfOne(t *testing.T) (painted, spikes, web int32) {
	t.Helper()
	c := r.c
	c.zoneFight(t)
	c.mustMove(t, c.master, "Toren", 10, 7)
	c.mustCastArea(t, c.caio, "Toren", spikeGrowthKey, slotOfLevel(2), at(2, 8), nil)
	c.nextTurnOf(t, "Pensantus")
	c.mustCastArea(t, c.ana, "Pensantus", webKey, slotOfLevel(2), at(18, 8), nil)
	c.nextTurnOf(t, "Toren")
	e := c.get(t, c.master)
	spikeZone, webZone := zonesOf(e, spikeGrowthKey)[0], zonesOf(e, webKey)[0]
	spikeEdge, webEdge := int32(0), int32(100)
	for _, sq := range spikeZone.GetSquares() {
		if sq.GetRow() == 8 {
			spikeEdge = max(spikeEdge, sq.GetCol())
		}
	}
	for _, sq := range webZone.GetSquares() {
		if sq.GetRow() == 8 {
			webEdge = min(webEdge, sq.GetCol())
		}
	}
	if zoneHas(spikeZone, spikeEdge+1, 8) || zoneHas(webZone, webEdge-1, 8) || webEdge-1 <= 10 || spikeEdge+1 >= 10 {
		t.Fatalf("the zones' edges are %d and %d, want plain floor next to each and the painted square (10,8) between them", spikeEdge, webEdge)
	}
	cost := func(from, to grid.Square) int32 {
		c.mustMove(t, c.master, "Toren", clamp32(from.Col, 0, 100), clamp32(from.Row, 0, 100))
		opts, err := c.options(t, c.caio, "Toren")
		if err != nil {
			t.Fatalf("GetMoveOptions() error = %v", err)
		}
		sq := squareAt(opts, clamp32(to.Col, 0, 100), clamp32(to.Row, 0, 100))
		if sq == nil {
			t.Fatalf("Toren cannot reach %v from %v: %v", to, from, opts)
		}
		return sq.GetCostDft()
	}
	painted = cost(grid.Square{Col: 9, Row: 8}, grid.Square{Col: 10, Row: 8})
	spikes = cost(grid.Square{Col: int(spikeEdge) + 1, Row: 8}, grid.Square{Col: int(spikeEdge), Row: 8})
	web = cost(grid.Square{Col: int(webEdge) - 1, Row: 8}, grid.Square{Col: int(webEdge), Row: 8})
	return painted, spikes, web
}

func TestAStepIntoPaintedSpikeAndWebDifficultTerrainCostsDoubleWithoutTheFlag(t *testing.T) {
	t.Parallel()
	painted, spikes, web := newTerrainRig(t, rules.FlagNeverSurprised).costsOfOne(t)
	if painted != 100 || spikes != 100 || web != 100 {
		t.Errorf("the steps cost %d, %d and %d tenths of a foot (map, spikes, web), want 100 each: 5 ft doubled", painted, spikes, web)
	}
}

func TestAnIgnoreDifficultTerrainFlagFreesTheMapsDifficultTerrainButNotASpellsAreas(t *testing.T) {
	t.Parallel()
	painted, spikes, web := newTerrainRig(t, rules.FlagIgnoreDifficultTerrain).costsOfOne(t)
	if painted != 50 || spikes != 100 || web != 100 {
		t.Errorf("the steps cost %d, %d and %d tenths of a foot (map, spikes, web), want 50, 100 and 100: only the map's is ignored", painted, spikes, web)
	}
}

func TestAFlaggedLongJumpLandingIsWarnedOnASpellsAreaAndNotOnThePaintedSquare(t *testing.T) {
	t.Parallel()
	r := newTerrainRig(t, rules.FlagIgnoreDifficultTerrain)
	c := r.c
	c.zoneFight(t)
	c.mustMove(t, c.master, "Toren", 10, 7)
	c.mustCastArea(t, c.caio, "Toren", spikeGrowthKey, slotOfLevel(2), at(2, 8), nil)
	c.nextTurnOf(t, "Toren")
	e := c.get(t, c.master)
	edge := int32(0)
	for _, sq := range zonesOf(e, spikeGrowthKey)[0].GetSquares() {
		if sq.GetRow() == 8 {
			edge = max(edge, sq.GetCol())
		}
	}
	landing := func(col, row int32) bool {
		c.mustMove(t, c.caio, "Toren", col, row, jumpTo(playv1.JumpKind_JUMP_KIND_LONG))
		return lastMove(t, c.log(t, c.master, c.get(t, c.master))).GetLandingDifficult()
	}
	c.mustMove(t, c.master, "Toren", 9, 8)
	if landing(10, 8) {
		t.Errorf("landing on the painted square asks for the Acrobatics check, want none for a flagged jumper")
	}
	c.undoLast(t)
	c.mustMove(t, c.master, "Toren", edge+1, 8)
	if !landing(edge, 8) {
		t.Errorf("landing in the spikes asks for no Acrobatics check, want one: a spell's area still is difficult terrain")
	}
}

func TestACharacterWhoseSheetCannotBeReadStillMovesAndGetsMoveOptions(t *testing.T) {
	t.Parallel()
	r := newTerrainRig(t, rules.FlagIgnoreDifficultTerrain)
	c := r.c
	c.zoneFight(t)
	// Toren's character goes back to the reserve, where the combat reads no sheet of it: no flag, and nothing fails.
	if _, err := c.h.pool.Exec(t.Context(), `UPDATE characters SET reserved = true, player_user_id = NULL WHERE id = $1`, c.toren.GetId()); err != nil {
		t.Fatalf("reserve Toren's character: %v", err)
	}
	if opts, err := c.options(t, c.master, "Toren"); err != nil || len(opts.GetReachable()) == 0 {
		t.Fatalf("GetMoveOptions() = %v, %v; want squares and no error", opts, err)
	}
	c.mustMove(t, c.master, "Toren", 6, 7) // the player's own move is refused for another reason: a sheet that cannot be read is down
	if got := c.who(t, c.master, "Toren"); got.GetCol() != 6 || got.GetRow() != 7 {
		t.Errorf("Toren stands at %d,%d, want 6,7", got.GetCol(), got.GetRow())
	}
}
