package play

import (
	"encoding/json"
	"math"
	"slices"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
	"github.com/PuraFome/meuRPG/backend/internal/rules/zone"
)

// Zones on the map (W7-Z): what a spell leaves on a combat's map for a while, and what
// the master puts there himself. This file keeps their state in memory (a row read, its
// squares and triggers decoded) and says what each viewer is told of one; zones_*.go
// run them.
//
// What a viewer is told (RN-10, RN-20):
//
//	                         master    player
//	a zone                   all       the ones visible_to_players says, or recognised (known_by)
//	its fields               all       shape, squares, obscurity, the spell's name; difficult
//	                                   terrain and halved speed for the ones they know
//	the caster, DC, damage,
//	duration, the excluded,
//	who is inside            yes       never
//	a creature inside a zone
//	that blocks sight        yes       only its own player sees it (the app's reading of
//	                                   "heavily obscured": nobody sees into, out of or
//	                                   through it)

// maxZones is how many zones a combat may have.
const maxZones = 30

// maxZoneSquares is the most squares a zone covers: a radius of 36 squares (180 ft, a
// Fog Cloud at the 9th level).
const maxZoneSquares = 4200

// The shapes as the table stores them (map_zones_shape_valid).
const (
	zoneSphere   = "sphere"
	zoneCube     = "cube"
	zoneCylinder = "cylinder"
	zoneWall     = "wall"
	zoneRing     = "ring"
)

// zoneState is a zone as the engine works with it: the row, with its squares and its
// triggers decoded.
type zoneState struct {
	row   playdb.MapZone
	cells []grid.Square
	trig  []zone.Trigger
}

// zoneStatesOf decodes the rows of a combat's zones. A trigger list that cannot be read
// is an empty one: the rows were written by this package.
func zoneStatesOf(rows []playdb.MapZone) []zoneState {
	out := make([]zoneState, len(rows))
	for i, r := range rows {
		out[i] = zoneStateOf(r)
	}
	return out
}

func zoneStateOf(r playdb.MapZone) zoneState {
	z := zoneState{row: r, cells: squaresOfInts(r.Cells)}
	_ = json.Unmarshal(r.Triggers, &z.trig) //nolint:errcheck // written by encodeTriggers
	return z
}

// squaresOfInts reads the squares a zone stores as col, row, col, row...
func squaresOfInts(v []int32) []grid.Square {
	out := make([]grid.Square, 0, len(v)/2)
	for i := 0; i+1 < len(v); i += 2 {
		out = append(out, grid.Square{Col: int(v[i]), Row: int(v[i+1])})
	}
	return out
}

// intsOfSquares is the zone's own way to store squares, row-major.
func intsOfSquares(sq []grid.Square) []int32 {
	sq = grid.Sorted(sq)
	out := make([]int32, 0, len(sq)*2)
	for _, s := range sq {
		out = append(out, clamp32(s.Col, 0, math.MaxInt32), clamp32(s.Row, 0, math.MaxInt32))
	}
	return out
}

func encodeTriggers(t []zone.Trigger) []byte {
	if len(t) == 0 {
		return []byte("[]")
	}
	b, err := json.Marshal(t)
	if err != nil {
		return []byte("[]") // a list of plain structs cannot fail
	}
	return b
}

// obscurity is the zone's effect on sight.
func (z zoneState) obscurity() zone.Obscurity { return zone.Obscurity(z.row.Obscurity) }

// blocksSight says nobody sees into, out of or through the zone.
func (z zoneState) blocksSight() bool { return z.obscurity().BlocksSight() }

// has says whether the square is one of the zone's.
func (z zoneState) has(sq grid.Square) bool { return slices.Contains(z.cells, sq) }

// excludes says the caster designated the creature to be unaffected.
func (z zoneState) excludes(id string) bool { return slices.Contains(z.row.ExcludedIds, id) }

// knownBy says the creature recognised a camouflaged zone.
func (z zoneState) knownBy(id string) bool { return slices.Contains(z.row.KnownBy, id) }

// trigger returns the zone's trigger of the kind.
func (z zoneState) trigger(kind zone.TriggerKind) (zone.Trigger, bool) {
	i := slices.IndexFunc(z.trig, func(t zone.Trigger) bool { return t.Kind == kind })
	if i < 0 {
		return zone.Trigger{}, false
	}
	return z.trig[i], true
}

// hasRule says the zone has the rule.
func (z zoneState) hasRule(r zone.Rule) bool { return slices.Contains(z.row.Rules, string(r)) }

// wall is the zone as a wall, to say which side of it a square is on.
func (z zoneState) wall() zone.Wall {
	w := zone.Wall{Cells: z.cells, Ring: z.row.Shape == zoneRing, Radius: int(z.row.RingRadius)}
	if z.row.OriginCol != nil && z.row.OriginRow != nil {
		w.Center = grid.Square{Col: int(*z.row.OriginCol), Row: int(*z.row.OriginRow)}
	}
	if z.row.Shape == zoneWall {
		d := grid.Direction{Dx: int(z.row.DirDx), Dy: int(z.row.DirDy)}
		w.Dir = &d
	}
	return w
}

// damageSide is the side of the wall that burns.
func (z zoneState) damageSide() zone.Side { return zone.Side(z.row.DamageSide) }

// remaining is how many rounds the zone has left at a round, 0 for a zone that lasts until
// the master ends it (see lasts).
func (z zoneState) remaining(round int32) int32 {
	if z.row.DurationRounds == 0 {
		return 0
	}
	return max(z.row.CastRound+z.row.DurationRounds-round, 0)
}

// lasts says the zone has a clock: it ends by itself.
func (z zoneState) lasts() bool { return z.row.DurationRounds > 0 }

// seenBy says whether the viewer is told of the zone: the master always; a player when the
// master's switch shows it to the players, or one of their creatures recognised it (a
// camouflaged zone), or one of them stands inside a zone of rules and so feels them (Silence
// has no visible form, and whoever is in it knows).
func (z zoneState) seenBy(v combatViewer, cs []playdb.Combatant) bool {
	if v.master || z.row.VisibleToPlayers {
		return true
	}
	for _, c := range cs {
		if !v.owns(c) {
			continue
		}
		if z.knownBy(c.ID) || (len(z.row.Rules) > 0 && placed(c) && z.has(squareOfCombatant(c))) {
			return true
		}
	}
	return false
}

// knownTo says whether the combatant knows the zone is there: it is not camouflaged, or the
// creature recognised it. What a move through it costs is told only to who knows it.
func (z zoneState) knownTo(c playdb.Combatant) bool {
	return !z.row.Camouflaged || z.knownBy(c.ID)
}

// blockingCells is the cells of every zone that blocks sight, as a lookup.
func blockingCells(zs []zoneState) func(grid.Square) bool {
	set := map[grid.Square]bool{}
	for _, z := range zs {
		if !z.blocksSight() {
			continue
		}
		for _, sq := range z.cells {
			set[sq] = true
		}
	}
	return func(sq grid.Square) bool { return set[sq] }
}

// hasBlocking says some zone blocks sight.
func hasBlocking(zs []zoneState) bool {
	return slices.ContainsFunc(zs, func(z zoneState) bool { return z.blocksSight() && len(z.cells) > 0 })
}

// hiddenByZonesFrom lists the combatants a player cannot see because of the zones that
// block sight (the app's reading of "heavily obscured": nobody sees into, out of or
// through one, and a creature inside sees only itself). A player sees a combatant when
// one of their own placed creatures does; their own are always seen. A player with no
// placed creature is not judged (the zones tell nothing of what they cannot see anyway:
// the other rules of the combat do).
func hiddenByZonesFrom(zs []zoneState, cs []playdb.Combatant, userID string) map[string]bool {
	if userID == "" || !hasBlocking(zs) {
		return nil
	}
	blocks := blockingCells(zs)
	var eyes []grid.Square
	for _, c := range cs {
		if c.UserID != nil && *c.UserID == userID && placed(c) && !c.Defeated {
			eyes = append(eyes, squareOfCombatant(c))
		}
	}
	if len(eyes) == 0 {
		return nil
	}
	out := map[string]bool{}
	for _, t := range cs {
		if (t.UserID != nil && *t.UserID == userID) || !placed(t) {
			continue
		}
		at := squareOfCombatant(t)
		if !slices.ContainsFunc(eyes, func(from grid.Square) bool { return !zone.Blocks(blocks, from, at) }) {
			out[t.ID] = true
		}
	}
	return out
}

// withZoneSight adds to the viewer what the zones hide from a player: an NPC they cannot
// see is unseen like one in the dark, and the square of a player's character or creature
// is not told (it stays in the order, only its place is not drawn).
func (v combatViewer) withZoneSight(zs []zoneState, cs []playdb.Combatant) combatViewer {
	if v.master {
		return v
	}
	hidden := hiddenByZonesFrom(zs, cs, v.userID)
	if len(hidden) == 0 {
		return v
	}
	unseen := make(map[string]bool, len(v.unseen)+len(hidden))
	for id := range v.unseen {
		unseen[id] = true
	}
	off := map[string]bool{}
	for _, c := range cs {
		if !hidden[c.ID] {
			continue
		}
		if c.Kind == kindNPC {
			unseen[c.ID] = true
		} else {
			off[c.ID] = true
		}
	}
	if len(unseen) > 0 {
		v.unseen = unseen
	}
	v.offMap = off
	return v
}

// canSee says the viewer sees the creature where it stands: it is not hidden from them and no
// zone that blocks sight stands in the way. A creature of the party that only a zone hides is
// still in the order (sees), but is not drawn, and a list of who was in an area leaves it out.
func (v combatViewer) canSee(c playdb.Combatant) bool { return v.sees(c) && !v.offMap[c.ID] }

// zoneProto is a zone as the viewer is told it.
func zoneProto(z zoneState, v combatViewer, cs []playdb.Combatant, round int32) *playv1.MapZone {
	r := z.row
	out := &playv1.MapZone{
		Id: r.ID, SpellKey: r.SpellKey, Shape: shapeToProto[r.Shape], Cells: slices.Clone(r.Cells),
		Obscurity: obscurityToProto[r.Obscurity],
	}
	if !v.master {
		// A scenario zone's name is told to whoever sees the zone; difficult terrain and the halved
		// speed are told for a zone the player knows, never a camouflaged one they have not recognised.
		out.Name = r.Name
		out.Difficult, out.HalvesSpeed = r.Difficult, r.HalvesSpeed
		out.VisibleToPlayers = r.VisibleToPlayers
		for _, c := range cs {
			if v.owns(c) && placed(c) && z.has(squareOfCombatant(c)) {
				out.SelfInside = true
			}
		}
		return out
	}
	out.Name = r.Name
	out.Difficult, out.HalvesSpeed = r.Difficult, r.HalvesSpeed
	out.VisibleToPlayers = r.VisibleToPlayers
	if r.OriginCol != nil && r.OriginRow != nil {
		out.Origin = &playv1.ZoneSquare{Col: *r.OriginCol, Row: *r.OriginRow}
	}
	out.DirDx, out.DirDy, out.SizeFt = int32(r.DirDx), int32(r.DirDy), r.SizeFt
	out.CasterId = deref(r.CasterID)
	out.Concentration, out.SlotLevel = r.Concentration, int32(r.SlotLevel)
	out.DurationRounds, out.RemainingRounds = r.DurationRounds, z.remaining(round)
	if r.DisperseRound != nil {
		out.DisperseRound = *r.DisperseRound
	}
	out.MovesWith = movesWithToProto[r.MovesWith]
	if spec, ok := zone.Lookup(r.SpellKey); ok {
		out.CasterMoves, out.CasterMoveFt, out.Anchors = spec.CasterMoves, clamp32(spec.MoveFt, 0, math.MaxInt32), spec.Anchors
		for _, w := range []zone.Wind{zone.WindModerate, zone.WindStrong} {
			if _, ok := spec.Dispersal[w]; ok {
				out.Winds = append(out.Winds, windToProto[w])
			}
		}
	}
	for _, t := range z.trig {
		out.Triggers = append(out.Triggers, &playv1.ZoneTrigger{
			Kind: triggerKindToProto[t.Kind], RequiresFullyInside: t.RequiresFullyInside, SaveAbility: t.Save, Damage: t.Damage,
			HalfOnSuccess: t.OnSuccess == zone.SuccessHalf, OnFail: failToProto[t.OnFail],
		})
	}
	for _, rule := range r.Rules {
		out.Rules = append(out.Rules, ruleToProto[zone.Rule(rule)])
	}
	out.SaveDc, out.DamageCount, out.DamageSides, out.DamageBonus, out.DamageType = r.SaveDc, r.DamageCount, r.DamageSides, r.DamageBonus, r.DamageType
	out.DamageSide = sideToProto[r.DamageSide]
	out.ReachSquares, out.Anchored = int32(r.ReachSquares), r.Anchored
	out.ExcludedCombatantIds, out.KnownBy, out.Members = r.ExcludedIds, r.KnownBy, r.Members
	out.Camouflaged = r.Camouflaged
	out.MasterCast = r.CasterID == nil || !isPlayerCombatant(cs, *r.CasterID)
	return out
}

// isPlayerCombatant says the combatant is a player's character.
func isPlayerCombatant(cs []playdb.Combatant, id string) bool {
	i := slices.IndexFunc(cs, func(c playdb.Combatant) bool { return c.ID == id })
	return i >= 0 && cs[i].Kind == kindPlayer
}

var (
	shapeToProto = map[string]playv1.ZoneShape{
		zoneSphere: playv1.ZoneShape_ZONE_SHAPE_SPHERE, zoneCube: playv1.ZoneShape_ZONE_SHAPE_CUBE,
		zoneCylinder: playv1.ZoneShape_ZONE_SHAPE_CYLINDER, zoneWall: playv1.ZoneShape_ZONE_SHAPE_WALL, zoneRing: playv1.ZoneShape_ZONE_SHAPE_RING,
	}
	obscurityToProto = map[string]playv1.ZoneObscurity{
		string(zone.ObscureLight): playv1.ZoneObscurity_ZONE_OBSCURITY_LIGHT, string(zone.ObscureHeavy): playv1.ZoneObscurity_ZONE_OBSCURITY_HEAVY,
		string(zone.ObscureDark): playv1.ZoneObscurity_ZONE_OBSCURITY_DARK, string(zone.ObscureOpaque): playv1.ZoneObscurity_ZONE_OBSCURITY_OPAQUE,
	}
	movesWithToProto = map[string]playv1.ZoneMovesWith{
		string(zone.MovesWithCaster):  playv1.ZoneMovesWith_ZONE_MOVES_WITH_CASTER,
		string(zone.MovesAtTurnStart): playv1.ZoneMovesWith_ZONE_MOVES_WITH_SELF_AT_TURN_START,
	}
	windToProto = map[zone.Wind]playv1.ZoneWind{
		zone.WindModerate: playv1.ZoneWind_ZONE_WIND_MODERATE, zone.WindStrong: playv1.ZoneWind_ZONE_WIND_STRONG,
	}
	triggerKindToProto = map[zone.TriggerKind]playv1.ZoneTriggerKind{
		zone.AtCast: playv1.ZoneTriggerKind_ZONE_TRIGGER_KIND_AT_CAST, zone.OnEnter: playv1.ZoneTriggerKind_ZONE_TRIGGER_KIND_ON_ENTER,
		zone.OnEnterFirstTimeOnATurn: playv1.ZoneTriggerKind_ZONE_TRIGGER_KIND_ON_ENTER_FIRST_TIME_ON_A_TURN,
		zone.StartOfTurn:             playv1.ZoneTriggerKind_ZONE_TRIGGER_KIND_START_OF_TURN, zone.EndOfTurn: playv1.ZoneTriggerKind_ZONE_TRIGGER_KIND_END_OF_TURN,
		zone.EndOfTurnWithin:  playv1.ZoneTriggerKind_ZONE_TRIGGER_KIND_END_OF_TURN_WITHIN,
		zone.PerDistanceMoved: playv1.ZoneTriggerKind_ZONE_TRIGGER_KIND_PER_DISTANCE_MOVED,
	}
	failToProto = map[zone.Fail]playv1.ZoneFail{
		zone.FailRestrained: playv1.ZoneFail_ZONE_FAIL_RESTRAINED, zone.FailProne: playv1.ZoneFail_ZONE_FAIL_PRONE,
		zone.FailLoseAction: playv1.ZoneFail_ZONE_FAIL_LOSE_ACTION,
	}
	ruleToProto = map[zone.Rule]playv1.ZoneRule{
		zone.NoVerbalCast: playv1.ZoneRule_ZONE_RULE_NO_VERBAL_CAST, zone.Deaf: playv1.ZoneRule_ZONE_RULE_DEAF,
		zone.ThunderImmune: playv1.ZoneRule_ZONE_RULE_THUNDER_IMMUNE,
	}
	sideToProto = map[string]playv1.ZoneSide{
		string(zone.SideA): playv1.ZoneSide_ZONE_SIDE_A, string(zone.SideB): playv1.ZoneSide_ZONE_SIDE_B,
	}
)
