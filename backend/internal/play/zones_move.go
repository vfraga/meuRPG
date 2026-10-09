package play

import (
	"math"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// What the zones do to movement (W7-Z, SRD "Difficult terrain" and the spells' texts): the ground
// of a zone that is difficult terrain costs an extra foot for each foot, and the squares of a
// zone that halves the speed (Spirit Guardians) cost twice. Difficult terrain does not add up
// with itself: two zones over the same square cost it once. Together they make four times the
// length (the app's reading of "halved" over a cost that is already double).
//
// A player plans on the zones their creature knows: Spike Growth is camouflaged until it is
// recognised, so the preview never shows its cost; the real move runs against all of them and is
// cut short where it costs more than the plan said (the move stops, like for a wall they did not
// see).

// withZones is the terrain with the zones' ground on it, for one mover: the real move puts every
// zone, a player's plan only the ones the creature knows. A zone that does not affect the mover
// (it was left out by the caster) does not slow it.
func withZones(t grid.Terrain, zs []zoneState, mover playdb.Combatant, knownOnly bool) grid.Terrain {
	if len(zs) == 0 || !t.Grid.Valid() {
		return t
	}
	var difficult, slow *grid.Layer
	for _, z := range zs {
		if len(z.cells) == 0 || (knownOnly && !z.knownTo(mover)) {
			continue
		}
		if z.row.Difficult {
			if difficult == nil {
				difficult = copyLayer(t.Grid, t.Difficult)
			}
			for _, sq := range z.cells {
				difficult.Set(sq.Col, sq.Row, true)
			}
		}
		if z.row.HalvesSpeed && z.affects(mover) {
			if slow == nil {
				slow = grid.NewLayer(t.Grid)
			}
			for _, sq := range z.cells {
				slow.Set(sq.Col, sq.Row, true)
			}
		}
	}
	if difficult != nil {
		t.Difficult = difficult
	}
	if slow != nil {
		t.Slow = slow
	}
	return t
}

// copyLayer is a layer with the squares of another (or none).
func copyLayer(g grid.Grid, from *grid.Layer) *grid.Layer {
	out := grid.NewLayer(g)
	if from == nil {
		return out
	}
	for row := 0; row < g.Rows; row++ {
		for col := 0; col < g.Columns; col++ {
			if from.Get(col, row) {
				out.Set(col, row, true)
			}
		}
	}
	return out
}

// maxZoneCosts bounds the squares a move preview lists the zones' cost of.
const maxZoneCosts = 600

// zoneMoveOptions adds to a move preview what the zones the creature knows do to it: the cost of
// the squares in them, with the zone it comes from, and the warning of each zone the move may
// enter (its saving throw's ability, never its DC or damage). A zone the creature does not know, or
// that does not affect it, is not there.
func zoneMoveOptions(out *playv1.GetMoveOptionsResponse, zs []zoneState, who playdb.Combatant, t grid.Terrain) {
	left := movementLeftDFt(who)
	from := squareOfCombatant(who)
	radius := left/grid.DFtPerSquare + 1
	reach := map[grid.Square]bool{}
	for _, r := range out.Reachable {
		reach[grid.Square{Col: int(r.Col), Row: int(r.Row)}] = true
	}
	for _, z := range zs {
		if !z.knownTo(who) || !z.affects(who) || len(z.cells) == 0 {
			continue
		}
		warn := false
		for _, sq := range z.cells {
			if abs(sq.Col-from.Col) > radius || abs(sq.Row-from.Row) > radius {
				continue
			}
			if reach[sq] {
				warn = true
			}
			if (z.row.Difficult || z.row.HalvesSpeed) && len(out.CostPerSquare) < maxZoneCosts {
				out.CostPerSquare = append(out.CostPerSquare, &playv1.ZoneCost{
					Col: clamp32(sq.Col, 0, math.MaxInt32), Row: clamp32(sq.Row, 0, math.MaxInt32), ZoneId: z.row.ID, SpellKey: z.row.SpellKey, Name: z.row.Name,
					Difficult: z.row.Difficult, HalvesSpeed: z.row.HalvesSpeed, CostDft: squareCostDFt(z, who, t, sq),
				})
			}
		}
		if !warn {
			continue
		}
		w := &playv1.ZoneWarning{ZoneId: z.row.ID, SpellKey: z.row.SpellKey, Name: z.row.Name, HalvesSpeed: z.row.HalvesSpeed}
		for _, tr := range z.trig {
			switch {
			case tr.Save != "" && w.SaveAbility == "":
				w.SaveAbility = tr.Save
			case tr.Save == "" && tr.Damage:
				w.Damages = true
			}
		}
		out.ZoneWarnings = append(out.ZoneWarnings, w)
	}
}

// squareCostDFt is what entering a square of the zone costs, in tenths of a foot, with the zone's
// effect: the length of a square, doubled for difficult terrain, doubled again for a halved speed.
func squareCostDFt(z zoneState, who playdb.Combatant, t grid.Terrain, _ grid.Square) int32 {
	cost := grid.DFtPerSquare
	if z.row.Difficult && !moverOf(who).Flier {
		cost *= 2
	}
	if z.row.HalvesSpeed {
		cost *= 2
	}
	return clamp32(cost, 0, math.MaxInt32)
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
