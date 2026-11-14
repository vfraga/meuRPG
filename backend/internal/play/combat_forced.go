package play

import (
	"context"
	"fmt"
	"math"
	"slices"

	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// Forced movement (SRD 5.1, Thunderwave and the Open Hand technique): a creature that fails
// the save is pushed some feet in a straight line away from the source, in whole squares of
// the combat map. It stops in the last free square before a wall, a column, a shut or locked
// door, the edge of the map or another creature. The push costs the creature no movement,
// provokes no opportunity attack and does no damage of its own; it clears the master's cover
// mark like any change of square. A combat in the theatre of the mind has no map: nothing
// moves and the log line says so.

// forcedRider is what a spell or a feature makes of a failed save beside its damage. Adding a
// pusher is adding a row to forcedRiders.
type forcedRider struct {
	// PushFeet is how far the creature is pushed away from the source.
	PushFeet int
	// MaxSize is the largest creature that is pushed; 0 means any size (the SRD text of the
	// two riders below names none).
	MaxSize grid.Size
}

// The keys of the riders that push.
const (
	spellThunderwave   = "spell:thunderwave"
	featureOpenHandKey = "feature:open-hand-technique"
)

// The distances the SRD text gives.
const (
	thunderwavePushFt = 10
	openHandPushFt    = 15
)

var forcedRiders = map[string]forcedRider{
	spellThunderwave:   {PushFeet: thunderwavePushFt},
	featureOpenHandKey: {PushFeet: openHandPushFt},
}

// forcedMove is the record a push leaves in the event of the cast or of the rider: where the
// creature stood (the undo puts it back) and where it ended.
type forcedMove struct {
	Target string     `json:"target_id"`
	From   *moveState `json:"from"`
	Col    int32      `json:"col"`
	Row    int32      `json:"row"`
	// DistanceFt is how far it really went, in feet, and Short says a wall or a creature
	// stopped it before the whole push.
	DistanceFt int32 `json:"distance_ft"`
	Short      bool  `json:"short,omitempty"`
	// NoMap says the combat has no map to push on (theatre of the mind): nothing moved and
	// the log line says so.
	NoMap bool `json:"no_map,omitempty"`
}

// pushedFt is the feet a push moved, for the log line.
func pushedFt(m *forcedMove) int32 {
	if m == nil {
		return 0
	}
	return m.DistanceFt
}

// pushedStopped says a wall, a door, the edge of the grid or a creature stopped the push before its
// whole distance.
func pushedStopped(m *forcedMove) bool { return m != nil && m.Short }

// pushedNoMap says the push was left to the table: there is no map.
func pushedNoMap(m *forcedMove) bool { return m != nil && m.NoMap }

// forcedPlan is where a push would end.
type forcedPlan struct {
	to    grid.Square
	short bool
}

// planPush works out the push of a creature standing on from, away from source, by feet: the
// squares of the straight line (Euclidean, 5 ft a square) whose distance from the start is
// within the feet, up to the first one the creature cannot enter. occ holds the other
// creatures; any of them is in the way, whatever its size or side.
func planPush(t grid.Terrain, occ grid.OccupantMap, mover grid.Mover, source, from grid.Square, feet int) forcedPlan {
	plan := forcedPlan{to: from}
	line := pushLine(source, from, feet)
	for i, step := range line {
		if grid.LengthDFt(from, step.Square) > feet*10 {
			break
		}
		mv := t.Move(plan.to, step.Square, occ, mover)
		if mv.Blocked || len(mv.Opens) > 0 {
			plan.short = true
			return plan
		}
		plan.to = step.Square
		if i == len(line)-1 {
			return plan
		}
	}
	return plan
}

// pushLine is the straight line from the target's square outwards, away from the source,
// as far as the whole squares of the push reach.
func pushLine(source, from grid.Square, feet int) []grid.Step {
	dc, dr := from.Col-source.Col, from.Row-source.Row
	norm := math.Hypot(float64(dc), float64(dr))
	if norm == 0 {
		return nil
	}
	n := float64(feet / grid.FeetPerSquare)
	end := grid.Square{Col: from.Col + int(math.Round(float64(dc)/norm*n)), Row: from.Row + int(math.Round(float64(dr)/norm*n))}
	return grid.Line(from, end)
}

// pushBlockers are the creatures that stand in the way of a push: every other placed
// creature that is not defeated, as large as the pushed one and hostile, so that none can be
// passed or shared.
func pushBlockers(cs []playdb.Combatant, target playdb.Combatant) grid.OccupantMap {
	size := moverOf(target).Size
	out := grid.OccupantMap{}
	for _, o := range cs {
		if o.ID != target.ID && !samePair(o, target) && !o.Defeated && placed(o) {
			out[squareOfCombatant(o)] = grid.Occupant{Size: size, Hostile: true}
		}
	}
	return out
}

// pusher is a push at work: the combat's transaction, the terrain and the combatants as they
// stand now (a push moves one, so the next one sees the new square).
type pusher struct {
	s       *Service
	c       *combatTx
	cs      []playdb.Combatant
	terrain grid.Terrain
	rider   forcedRider
	source  playdb.Combatant
	// noMap is a combat with no map: push answers with the note and moves nobody.
	noMap bool
	// moved are the squares the pushes went through, for the areas spells keep on the map.
	moved []zoneStep
}

// pusherFor sets up the push of a rider key, or nil when the key pushes nobody. A combat with
// no map (or a source with no square) gets a pusher that moves nobody and says so.
func (s *Service) pusherFor(c *combatTx, cs []playdb.Combatant, terrain grid.Terrain, key string, source playdb.Combatant) *pusher {
	rider, ok := forcedRiders[key]
	if !ok {
		return nil
	}
	return &pusher{s: s, c: c, cs: slices.Clone(cs), terrain: terrain, rider: rider, source: source, noMap: isTheatre(c.enc) || !placed(source)}
}

// push moves the target and writes its square. It returns nil when the creature is too large
// for the rider or has no square; a push with nowhere to go answers with no distance and
// `Short`.
func (p *pusher) push(ctx context.Context, target playdb.Combatant) (*forcedMove, error) {
	if p.noMap {
		return &forcedMove{Target: target.ID, NoMap: true}, nil
	}
	target, ok := combatantByID(p.cs, target.ID)
	if !ok || !placed(target) || target.Defeated || target.MountedOnID != nil || (p.rider.MaxSize > 0 && moverOf(target).Size > p.rider.MaxSize) {
		return nil, nil
	}
	from := squareOfCombatant(target)
	plan := planPush(p.terrain, pushBlockers(p.cs, target), moverOf(target), squareOfCombatant(p.source), from, p.rider.PushFeet)
	if plan.to == from {
		if !plan.short {
			return nil, nil
		}
		// A wall or a creature leaves nowhere to go: the save failed, nothing moves, and the log says the push was stopped.
		col, row := clamp32(from.Col, 0, math.MaxInt32), clamp32(from.Row, 0, math.MaxInt32)
		return &forcedMove{Target: target.ID, From: moveStateIn(p.cs, target), Col: col, Row: row, Short: true}, nil
	}
	col, row := clamp32(plan.to.Col, 0, math.MaxInt32), clamp32(plan.to.Row, 0, math.MaxInt32)
	if err := p.c.q.SetCombatantPlace(ctx, playdb.SetCombatantPlaceParams{ID: target.ID, GridCol: &col, GridRow: &row, CoverMark: "none"}); err != nil {
		return nil, fmt.Errorf("push the creature: %w", err)
	}
	if err := carryRider(ctx, p.c.q, target.ID); err != nil {
		return nil, err
	}
	moved := forcedMove{
		Target: target.ID, From: moveStateIn(p.cs, target), Col: col, Row: row,
		DistanceFt: clamp32(grid.LengthDFt(from, plan.to)/10, 0, math.MaxInt32), Short: plan.short,
	}
	p.remember(target.ID, col, row)
	if rider := riderOf(p.cs, target.ID); rider != nil {
		p.remember(rider.ID, col, row)
	}
	p.moved = append(p.moved, zoneStep{who: target.ID, from: &from, path: []grid.Square{plan.to}, travels: true})
	return &moved, nil
}

// enterZones looks at the areas spells keep on the map for the creatures the pushes moved.
func (p *pusher) enterZones(ctx context.Context) error {
	return p.s.zonesAfterSteps(ctx, p.c, p.moved...)
}

// remember puts the new square in the copy of the combatants.
func (p *pusher) remember(id string, col, row int32) {
	for i := range p.cs {
		if p.cs[i].ID == id {
			p.cs[i].GridCol, p.cs[i].GridRow, p.cs[i].CoverMark = &col, &row, "none"
		}
	}
}

// pushFailedSaves pushes the creatures that failed the save of a cast, the farthest from the
// caster first so the near ones find the squares behind them free, and records each push in
// its hit.
func (f *castFlow) pushFailedSaves(ctx context.Context) error {
	p := f.s.pusherFor(f.c, f.cs, f.terrain, f.sp.Key, f.caster)
	if p == nil {
		return nil
	}
	hits := f.out.made.Hits
	var order []int
	for i, h := range hits {
		if h.Save != nil && !h.Save.Saved && h.Target != f.caster.ID {
			order = append(order, i)
		}
	}
	at := squareOfCombatant(f.caster)
	farther := func(i int) int {
		t, _ := combatantByID(f.cs, hits[i].Target)
		if !placed(t) {
			return 0
		}
		return grid.LengthDFt(at, squareOfCombatant(t))
	}
	slices.SortStableFunc(order, func(a, b int) int { return farther(b) - farther(a) })
	for _, i := range order {
		t, ok := combatantByID(f.cs, hits[i].Target)
		if !ok {
			continue
		}
		moved, err := p.push(ctx, t)
		if err != nil {
			return err
		}
		hits[i].Pushed = moved
	}
	return p.enterZones(ctx)
}

// pushByRider pushes the target of a hit rider (the Open Hand technique) away from the monk.
func (s *Service) pushByRider(ctx context.Context, c *combatTx, cs []playdb.Combatant, attacker, target playdb.Combatant, re *riderEvent) error {
	terrain, err := s.terrainOf(ctx, c.tx, c.session.CampaignID, c.enc)
	if err != nil {
		return err
	}
	p := s.pusherFor(c, cs, terrain, featureOpenHandKey, attacker)
	if p == nil {
		return nil
	}
	if re.Pushed, err = p.push(ctx, target); err != nil {
		return err
	}
	return p.enterZones(ctx)
}

// putPushedBack puts a creature back on the square a push took it from: the undo.
func putPushedBack(ctx context.Context, c *combatTx, m *forcedMove) error {
	if m == nil || m.From == nil || !m.From.Placed {
		return nil
	}
	if err := c.q.SetCombatantPlace(ctx, playdb.SetCombatantPlaceParams{ID: m.Target, GridCol: &m.From.Col, GridRow: &m.From.Row, CoverMark: coverOrNone(m.From.Cover)}); err != nil {
		return fmt.Errorf("put the pushed creature back: %w", err)
	}
	return carryRiderBack(ctx, c.q, m.Target, m.From)
}

// publishPushed tells the streams and the fog that the creatures moved, after the commit.
func (s *Service) publishPushed(ctx context.Context, campaignID string, d *encounterData, moves []*forcedMove) {
	for _, m := range moves {
		if m == nil || m.NoMap || m.DistanceFt == 0 {
			continue
		}
		if who, ok := combatantByID(d.cs, m.Target); ok {
			s.publishCombatantMoved(ctx, campaignID, d.enc, who, squareOfState(m.From))
			s.positionChanged(ctx, campaignID, d.enc, who, squareOfState(m.From))
			s.publishRideAlong(ctx, campaignID, d, who, squareOfState(m.From))
		}
	}
}

// pushesOf lists the pushes of a cast's hits.
func pushesOf(hits []castHit) []*forcedMove {
	var out []*forcedMove
	for _, h := range hits {
		if h.Pushed != nil {
			out = append(out, h.Pushed)
		}
	}
	return out
}
