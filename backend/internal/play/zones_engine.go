package play

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"

	"connectrpc.com/connect"
	"uuid"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
	"github.com/PuraFome/meuRPG/backend/internal/rules/zone"
)

// The engine of the zones (W7-Z): it makes a zone, ends it, moves it, and runs its
// triggers when a creature enters it, starts or ends its turn in it, or travels in it.
//
// Everything runs in the change's transaction (db.InTx retries it on a serialization
// failure, so each step reads before it writes and is idempotent), through the combatTx
// (c.q), and the answers a trigger asks of a creature are saving throws in windows
// (zones_save.go), not rolls the server makes behind the table's back.
//
// Where the SRD is silent the app reads it, and says so where the code applies it:
//   - a zone that moves onto a creature, or a creature pushed into one, counts as the
//     creature entering it;
//   - each of a creature's squares counts (a creature larger than a square is in a zone
//     when any is), but the map keeps one square for every creature;
//   - a straight move that crosses a zone enters it once, however many of its squares it
//     covers.

// loadZones reads the live zones of the combat into the change, once; a change that
// creates, moves or ends a zone reads them again (reloadZones).
func (c *combatTx) loadZones(ctx context.Context) error {
	if c.zonesLoaded || c.enc.ID == "" {
		return nil
	}
	rows, err := c.q.ListMapZones(ctx, c.enc.ID)
	if err != nil {
		return fmt.Errorf("list the zones: %w", err)
	}
	c.zones, c.zonesLoaded = zoneStatesOf(rows), true
	return nil
}

// reloadZones drops what the change knows of the zones and reads them again.
func (c *combatTx) reloadZones(ctx context.Context) error {
	c.zonesLoaded = false
	return c.loadZones(ctx)
}

// zoneNote is what an event keeps of a zone: ids, squares and numbers, never a name
// (docs/privacy.md); the log reads the zone's name from the zone's row.
type zoneNote struct {
	ID string `json:"id"`
	// Key is the spell that left the zone.
	Key string `json:"key,omitempty"`
	// What is "added", "moved", "ended", "dispersed", "caught" or "saved".
	What string `json:"what"`
	// Who is the creature the zone caught or asked a saving throw of.
	Who     string `json:"who,omitempty"`
	Trigger string `json:"trigger,omitempty"`
	// Moved is how many squares a moved zone walked; Squares how many a creature travelled in
	// a zone that hurts by the distance.
	Moved   int32 `json:"moved,omitempty"`
	Squares int32 `json:"squares,omitempty"`
	// The saving throw: its ability, the roll and the DC, whether it passed, and the pending
	// damage the failure (or the half) opened.
	Ability  string `json:"ability,omitempty"`
	D20      int32  `json:"d20,omitempty"`
	Modifier int32  `json:"modifier,omitempty"`
	Total    int32  `json:"total,omitempty"`
	DC       int32  `json:"dc,omitempty"`
	Saved    bool   `json:"saved,omitempty"`
	Pending  string `json:"pending,omitempty"`
	// Failed is what the failure did (zone.Fail); Reason is why a window closed; Immune says
	// the creature passed without a roll.
	Failed string `json:"failed,omitempty"`
	Reason string `json:"reason,omitempty"`
	Immune bool   `json:"immune,omitempty"`
	// Skipped says the master did not fire the trigger.
	Skipped bool `json:"skipped,omitempty"`
	// The creature's own player rolled it (not a roll of the app): the d20 is theirs.
	Physical bool `json:"physical,omitempty"`
	// Window is the saving throw the line is about.
	Window string `json:"window,omitempty"`
	// Caster is the combatant that cast the zone, for the lines that show it to the master.
	Caster string `json:"caster,omitempty"`
}

// zoneEventData is the payload field of the events of a zone (actionEvent.Zone).
type zoneEventData = zoneNote

// writeZoneEvent appends the line of a zone: a session event of the kind. The line is the
// master's alone when the zone is one the players are not told of, or a creature in it is
// one they do not see (the stamp narrows who may read it).
func (s *Service) writeZoneEvent(ctx context.Context, c *combatTx, kind string, z zoneState, note zoneNote, actor string, extra ...func(*actionEvent)) error {
	note.ID, note.Key = z.row.ID, z.row.SpellKey
	ev := actionEvent{Round: c.enc.Round, Actor: actor, Zone: &note}
	if !z.row.VisibleToPlayers && !z.hasPlayerKnowledge() {
		ev.Secret = true
	}
	for _, f := range extra {
		f(&ev)
	}
	return insertEvent(ctx, c, kind, &c.actorUserID, nil, ev)
}

// hasPlayerKnowledge says some player's creature recognised the zone, so that a line about
// it is not a secret of the master's.
func (z zoneState) hasPlayerKnowledge() bool {
	return len(z.row.KnownBy) > 0 && !z.row.VisibleToPlayers && !z.row.Camouflaged
}

// zoneCells computes the squares a zone covers: the shape set on the point, cut by the
// walls (grid.Terrain.OpenArea), as the spell's text spreads. A spell that spreads around
// corners is the part of the shape connected to its point.
func zoneCells(spec zone.Spec, sizeFt int, terrain grid.Terrain, origin grid.Square, dir *grid.Direction) []grid.Square {
	raw := zone.Shape(spec, sizeFt, origin, dir)
	return terrain.OpenArea(origin, raw, spec.SpreadsAroundCorners)
}

// zoneBuild is what makes a zone row.
type zoneBuild struct {
	spec      zone.Spec
	hasSpec   bool
	spellKey  string
	name      string
	caster    *playdb.Combatant
	shape     string
	origin    *grid.Square
	dir       grid.Direction
	sizeFt    int
	ringR     int
	cells     []grid.Square
	obscurity zone.Obscurity
	difficult bool
	halves    bool
	camo      bool
	visible   bool
	conc      bool
	slot      int
	duration  int32
	moves     zone.MovesWith
	step      int
	casterMv  bool
	triggers  []zone.Trigger
	rules     []zone.Rule
	dc        int
	dmg       zoneDamage
	side      zone.Side
	reach     int
	anchored  bool
	excluded  []string
	knownBy   []string
}

// zoneDamage is the damage dice of a zone's triggers.
type zoneDamage struct {
	count, sides, bonus int
	kind                string
}

// insertZone writes a zone: the number of zones of the combat is checked first.
func (s *Service) insertZone(ctx context.Context, c *combatTx, b zoneBuild) (zoneState, error) {
	if err := c.loadZones(ctx); err != nil {
		return zoneState{}, err
	}
	if len(c.zones) >= maxZones {
		return zoneState{}, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TOO_MANY_ZONES, "the combat has as many zones as it may")
	}
	if len(b.cells) > maxZoneSquares {
		return zoneState{}, connect.NewError(connect.CodeInvalidArgument, errors.New("the zone covers too many squares"))
	}
	seq, err := c.q.NextMapZoneSeq(ctx, c.enc.ID)
	if err != nil {
		return zoneState{}, fmt.Errorf("number the zone: %w", err)
	}
	p := playdb.InsertMapZoneParams{
		EncounterID: c.enc.ID, Seq: seq, SpellKey: b.spellKey, Name: b.name, Shape: b.shape,
		DirDx: int16(b.dir.Dx), DirDy: int16(b.dir.Dy), //nolint:gosec // -1..1
		SizeFt: clamp32(b.sizeFt, 0, 400), RingRadius: int16(b.ringR), Cells: intsOfSquares(b.cells), //nolint:gosec // a few squares
		Obscurity: string(b.obscurity), Difficult: b.difficult, HalvesSpeed: b.halves, Camouflaged: b.camo, VisibleToPlayers: b.visible,
		Concentration: b.conc, SlotLevel: int16(b.slot), CastRound: c.enc.Round, DurationRounds: b.duration, //nolint:gosec // 1..9
		MovesWith: string(b.moves), StepSquares: int16(b.step), CasterMoves: b.casterMv, //nolint:gosec // a few squares
		Triggers: encodeTriggers(b.triggers), SaveDc: clamp32(b.dc, 0, math.MaxInt32),
		DamageCount: clamp32(b.dmg.count, 0, 100), DamageSides: clamp32(b.dmg.sides, 0, 100), DamageBonus: clamp32(b.dmg.bonus, -1000, 1000), DamageType: b.dmg.kind,
		DamageSide: string(b.side), ReachSquares: int16(b.reach), Anchored: b.anchored, //nolint:gosec // a few squares
		ExcludedIds: nonNil(b.excluded), KnownBy: nonNil(b.knownBy), Members: []string{}, CreatedAt: c.now,
	}
	for _, r := range b.rules {
		p.Rules = append(p.Rules, string(r))
	}
	p.Rules = nonNil(p.Rules)
	if b.caster != nil {
		p.CasterID = &b.caster.ID
	}
	if b.origin != nil {
		col, row := clamp32(b.origin.Col, 0, math.MaxInt32), clamp32(b.origin.Row, 0, math.MaxInt32)
		p.OriginCol, p.OriginRow = &col, &row
	}
	row, err := c.q.InsertMapZone(ctx, p)
	if err != nil {
		return zoneState{}, fmt.Errorf("put the zone: %w", err)
	}
	if err := c.reloadZones(ctx); err != nil {
		return zoneState{}, err
	}
	return zoneStateOf(row), nil
}

// endZone ends a zone: the conditions it put go, the saving throws it waited for close with
// the reason zone_ended, and its row is kept, ended, for the log. It reports the creatures
// whose conditions it took off.
func (s *Service) endZone(ctx context.Context, c *combatTx, z zoneState, what string) error {
	effects, err := c.q.ListZoneEffects(ctx, z.row.ID)
	if err != nil {
		return fmt.Errorf("list what the zone put: %w", err)
	}
	if err := c.q.CloseZoneSaveWindowsOfZone(ctx, playdb.CloseZoneSaveWindowsOfZoneParams{ZoneID: &z.row.ID, CloseReason: closeZoneEnded, AnsweredAt: &c.now}); err != nil {
		return fmt.Errorf("close the zone's saves: %w", err)
	}
	if err := c.q.EndMapZone(ctx, playdb.EndMapZoneParams{ID: z.row.ID, EndedAt: &c.now}); err != nil {
		return fmt.Errorf("end the zone: %w", err)
	}
	if err := c.q.DeleteZoneFiredOfZone(ctx, z.row.ID); err != nil {
		return fmt.Errorf("clear the zone's ledger: %w", err)
	}
	cs, err := c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return fmt.Errorf("list the combatants: %w", err)
	}
	for _, e := range effects {
		if err := c.q.DeleteZoneEffect(ctx, playdb.DeleteZoneEffectParams{ZoneID: e.ZoneID, CombatantID: e.CombatantID, Condition: e.Condition}); err != nil {
			return fmt.Errorf("take the zone's condition off: %w", err)
		}
		if i := slices.IndexFunc(cs, func(o playdb.Combatant) bool { return o.ID == e.CombatantID }); i >= 0 {
			if err := s.dropZoneCondition(ctx, c, cs[i], e.Condition); err != nil {
				return err
			}
		}
	}
	if err := c.reloadZones(ctx); err != nil {
		return err
	}
	// The line: the zone ended or a wind dispersed it.
	return s.writeZoneEvent(ctx, c, eventMapZoneEnded, z, zoneNote{What: what}, "")
}

// endSpell ends the spell a zone is: the zone, and the concentration of its caster when the spell
// is the one they hold (SRD, "Concentration": the spell ends, and so does the concentration). A zone
// that is not a concentration spell, or whose caster is gone, only ends.
func (s *Service) endSpell(ctx context.Context, c *combatTx, z zoneState, what string) error {
	if z.row.Concentration && z.row.CasterID != nil {
		cs, err := c.q.ListCombatants(ctx, c.enc.ID)
		if err != nil {
			return fmt.Errorf("list the combatants: %w", err)
		}
		if i := slices.IndexFunc(cs, func(o playdb.Combatant) bool { return o.ID == *z.row.CasterID }); i >= 0 && deref(cs[i].ConcentrationSpell) == z.row.SpellKey {
			ev := actionEvent{Round: c.enc.Round, Secret: cs[i].Hidden, Actor: cs[i].ID}
			if err := s.stopConcentrating(ctx, c, cs[i], &ev); err != nil {
				return err
			}
			if err := insertEvent(ctx, c, eventConditionsSet, &c.actorUserID, nil, ev); err != nil {
				return err
			}
		}
	}
	// The concentration may have ended it already; the zone ends once.
	if err := c.reloadZones(ctx); err != nil {
		return err
	}
	if !slices.ContainsFunc(c.zones, func(o zoneState) bool { return o.row.ID == z.row.ID }) {
		return nil
	}
	return s.endZone(ctx, c, z, what)
}

// closeZoneEnded is the reason a window closes with when its zone ends.
const closeZoneEnded = "zone_ended"

// dropZoneCondition takes a condition off a creature when no other zone keeps it there.
func (s *Service) dropZoneCondition(ctx context.Context, c *combatTx, who playdb.Combatant, cond string) error {
	rest, err := c.q.ListZoneEffectsOfCombatant(ctx, who.ID)
	if err != nil {
		return fmt.Errorf("list the zones' conditions: %w", err)
	}
	if slices.ContainsFunc(rest, func(e playdb.MapZoneEffect) bool { return e.Condition == cond }) {
		return nil
	}
	now, err := c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return fmt.Errorf("list the combatants: %w", err)
	}
	i := slices.IndexFunc(now, func(o playdb.Combatant) bool { return o.ID == who.ID })
	if i < 0 || !slices.Contains(now[i].Conditions, cond) {
		return nil
	}
	left := slices.DeleteFunc(slices.Clone(now[i].Conditions), func(k string) bool { return k == cond })
	if err := c.q.SetCombatantConditions(ctx, playdb.SetCombatantConditionsParams{ID: who.ID, Conditions: nonNil(left)}); err != nil {
		return fmt.Errorf("take the condition off: %w", err)
	}
	return nil
}

// putZoneCondition puts a condition on a creature for as long as the zone lasts (or, with
// keep, for good: a creature that fell prone stays down when the grease is gone). It
// reports whether the condition was new on the creature.
func (s *Service) putZoneCondition(ctx context.Context, c *combatTx, z zoneState, who playdb.Combatant, cond string, keep bool) error {
	now, err := c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return fmt.Errorf("list the combatants: %w", err)
	}
	i := slices.IndexFunc(now, func(o playdb.Combatant) bool { return o.ID == who.ID })
	if i < 0 {
		return nil
	}
	if !keep {
		if _, err := c.q.InsertZoneEffect(ctx, playdb.InsertZoneEffectParams{ZoneID: z.row.ID, CombatantID: who.ID, Condition: cond}); err != nil {
			return fmt.Errorf("note what the zone put: %w", err)
		}
	}
	if slices.Contains(now[i].Conditions, cond) || len(now[i].Conditions) >= maxConditions {
		return nil
	}
	list := append(slices.Clone(now[i].Conditions), cond)
	if err := c.q.SetCombatantConditions(ctx, playdb.SetCombatantConditionsParams{ID: who.ID, Conditions: list}); err != nil {
		return fmt.Errorf("put the condition on: %w", err)
	}
	return nil
}

// The conditions a zone puts, as the SRD's keys.
const (
	condRestrained = "condition:restrained"
	condProne      = "condition:prone"
	condDeafened   = "condition:deafened"
)

// endConcentrationZones ends the zones a caster's concentration holds: the spell ends with
// the concentration (SRD, "Concentration"), in the same transaction. It returns the zones it
// ended, for the event to keep: an undo of what ended the concentration brings them back.
func (s *Service) endConcentrationZones(ctx context.Context, c *combatTx, casterID string) ([]string, error) {
	if err := c.loadZones(ctx); err != nil {
		return nil, err
	}
	var ended []string
	for _, z := range slices.Clone(c.zones) {
		if z.row.Concentration && z.row.CasterID != nil && *z.row.CasterID == casterID {
			if err := s.endZone(ctx, c, z, whatEnded); err != nil {
				return nil, err
			}
			ended = append(ended, z.row.ID)
		}
	}
	return ended, nil
}

// The ways a zone's line says it ended.
const (
	whatEnded     = "ended"
	whatDispersed = "dispersed"
)

// ---- who is in a zone ----

// inside says whether the creature is in the zone: its square is one of the zone's, or,
// in a combat without a map, the master marked it.
func (z zoneState) inside(c playdb.Combatant, theatre bool) bool {
	if theatre {
		return slices.Contains(z.row.Members, c.ID)
	}
	return placed(c) && z.has(squareOfCombatant(c))
}

// footprint is the squares a creature occupies. The map keeps one square for every creature,
// whatever its size, so the footprint is that square: the rule of the zone for a larger
// creature (inside when any square is, entirely inside when all are) is zone.Overlap's.
func footprint(c playdb.Combatant) []grid.Square {
	if !placed(c) {
		return nil
	}
	return []grid.Square{squareOfCombatant(c)}
}

// fullyInside says the creature is entirely inside the zone.
func (z zoneState) fullyInside(c playdb.Combatant, theatre bool) bool {
	if theatre {
		return slices.Contains(z.row.Members, c.ID)
	}
	_, all := zone.Overlap(z.cells, footprint(c))
	return all
}

// affects says the zone may act on the creature at all: it stands, it was not left out by
// the caster, and it is not the caster of a zone that goes with it.
func (z zoneState) affects(c playdb.Combatant) bool {
	if c.Defeated || z.excludes(c.ID) {
		return false
	}
	return !(z.row.MovesWith == string(zone.MovesWithCaster) && z.row.CasterID != nil && *z.row.CasterID == c.ID)
}

// ---- triggers ----

// fireOpts says what happened to the creature.
type fireOpts struct {
	kind zone.TriggerKind
	// squares is how many squares of the zone a creature travelled (PerDistanceMoved).
	squares int
	// coverBonus is the cover a creature had against the point of origin (AtCast).
	coverBonus int
}

// ledgerKinds say which triggers share the once-a-turn ledger: a creature is hurt at most once
// for each zone and each turn by entering it for the first time on a turn or starting its turn
// in it, together.
func usesLedger(z zoneState, kind zone.TriggerKind) bool {
	switch kind {
	case zone.OnEnterFirstTimeOnATurn:
		return true
	case zone.StartOfTurn:
		_, both := z.trigger(zone.OnEnterFirstTimeOnATurn)
		return both
	}
	return false
}

// fireZone runs one trigger of a zone on a creature: the damage the zone deals with no save
// opens as a pending damage, and a saving throw opens a window the creature's player (or the
// master) answers. It does nothing when the zone has no such trigger or does not affect the
// creature.
func (s *Service) fireZone(ctx context.Context, c *combatTx, z zoneState, who playdb.Combatant, o fireOpts) error {
	tr, ok := z.trigger(o.kind)
	if !ok || !z.affects(who) {
		return nil
	}
	if c.enc.Status != statusActive {
		return nil // a zone acts on a running combat
	}
	if usesLedger(z, o.kind) {
		turnOf := deref(c.enc.CurrentCombatantID)
		if turnOf == "" {
			return nil
		}
		n, err := c.q.MarkZoneFired(ctx, playdb.MarkZoneFiredParams{ZoneID: z.row.ID, CombatantID: who.ID, Round: c.enc.Round, TurnOf: turnOf})
		if err != nil {
			return fmt.Errorf("note who the zone hurt: %w", err)
		}
		if n == 0 {
			return nil // hurt in this turn already
		}
	}
	if tr.Save == "" {
		return s.fireZoneDamage(ctx, c, z, who, tr, o)
	}
	return s.openZoneSave(ctx, c, z, who, tr, o)
}

// fireZoneDamage opens the damage a trigger with no saving throw deals (the burn of a wall,
// the spikes): one pending damage, of as many times the dice as squares travelled.
func (s *Service) fireZoneDamage(ctx context.Context, c *combatTx, z zoneState, who playdb.Combatant, tr zone.Trigger, o fireOpts) error {
	if !tr.Damage || z.row.DamageSides == 0 {
		return nil
	}
	times := 1
	if o.kind == zone.PerDistanceMoved {
		times = max(o.squares, 1)
	}
	id, err := s.openZoneDamage(ctx, c, z, who, times, false)
	if err != nil {
		return err
	}
	return s.writeZoneEvent(ctx, c, eventMapZoneTriggered, z, zoneNote{
		What: "caught", Who: who.ID, Trigger: string(o.kind), Squares: clamp32(o.squares, 0, math.MaxInt32), Pending: id,
	}, who.ID, func(ev *actionEvent) { ev.Secret = ev.Secret || who.Hidden })
}

// openZoneDamage opens the damage of a zone's dice (times of them) as a pending damage on
// the creature, attributed to the caster, who rolls it like the damage of any spell (the
// master, when the zone has no caster any more). half marks the damage of a save that halves.
func (s *Service) openZoneDamage(ctx context.Context, c *combatTx, z zoneState, who playdb.Combatant, times int, half bool) (string, error) {
	r := z.row
	if r.DamageSides == 0 {
		return "", nil
	}
	attacker := who.ID
	if r.CasterID != nil {
		attacker = *r.CasterID
	}
	key := r.SpellKey
	if key == "" {
		key = zoneScenarioKey
	}
	castID := uuid.New().String()
	p, err := c.q.InsertPendingDamage(ctx, playdb.InsertPendingDamageParams{
		EncounterID: c.enc.ID, AttackerID: &attacker, TargetID: who.ID, AttackKey: key, Status: pendingAwaitingRoll,
		DiceCount: clamp32(int(r.DamageCount)*times, 0, 100), DiceSides: r.DamageSides, DiceBonus: clamp32(int(r.DamageBonus)*times, -1000, 1000),
		DamageType: r.DamageType, CreatedAt: c.now, CastID: &castID, Half: half,
	})
	if err != nil {
		return "", fmt.Errorf("open the zone's damage: %w", err)
	}
	return p.ID, nil
}

// zoneScenarioKey is the attack key of the damage of a zone the master put.
const zoneScenarioKey = "zone:scenario"

// ---- a creature moved ----

// entered says whether a straight move from one square to another enters the zone: one of
// the squares after the start is in the zone while the one before it is not.
func entered(cells []grid.Square, from, to grid.Square) bool {
	prev := from
	for _, step := range grid.Line(from, to) {
		if slices.Contains(cells, step.Square) && !slices.Contains(cells, prev) {
			return true
		}
		prev = step.Square
	}
	return false
}

// zonesAfterMove runs the triggers a creature's move sets off: it entered a zone (every
// time, or the first time in the turn), it travelled in a zone that hurts by the distance,
// or its caster carried a zone with it. from is where it stood, nil for a creature the move
// put on the map. forced says the master moved it (a push, a teleport): it counts as
// entering all the same.
func (s *Service) zonesAfterMove(ctx context.Context, c *combatTx, cs []playdb.Combatant, who playdb.Combatant, from *grid.Square, terrain grid.Terrain) error {
	if err := c.loadZones(ctx); err != nil {
		return err
	}
	if len(c.zones) == 0 || !placed(who) {
		return nil
	}
	to := squareOfCombatant(who)
	// A zone that goes with its caster goes first: the creatures it lands on enter it.
	for _, z := range slices.Clone(c.zones) {
		if z.row.MovesWith == string(zone.MovesWithCaster) && z.row.CasterID != nil && *z.row.CasterID == who.ID {
			if err := s.moveZoneTo(ctx, c, cs, z, to, terrain, 0); err != nil {
				return err
			}
		}
	}
	if from != nil && *from != to {
		for _, z := range slices.Clone(c.zones) {
			if !z.affects(who) {
				continue
			}
			if entered(z.cells, *from, to) {
				for _, k := range []zone.TriggerKind{zone.OnEnter, zone.OnEnterFirstTimeOnATurn} {
					if err := s.fireZone(ctx, c, z, who, fireOpts{kind: k}); err != nil {
						return err
					}
				}
			}
			if n := zone.Crossed(z.cells, *from, to); n > 0 {
				if err := s.fireZone(ctx, c, z, who, fireOpts{kind: zone.PerDistanceMoved, squares: n}); err != nil {
					return err
				}
			}
		}
	}
	return s.syncZoneRules(ctx, c)
}

// moveZoneTo puts a zone on another point: its squares are worked out again, and the creatures
// that were not in it and are now enter it (the app's reading of "enters the area": a zone that
// moves onto a creature counts as the creature entering it).
func (s *Service) moveZoneTo(ctx context.Context, c *combatTx, cs []playdb.Combatant, z zoneState, origin grid.Square, terrain grid.Terrain, walked int) error {
	spec, hasSpec := zone.Lookup(z.row.SpellKey)
	var cells []grid.Square
	dir := grid.Direction{Dx: int(z.row.DirDx), Dy: int(z.row.DirDy)}
	switch {
	case hasSpec:
		cells = zoneCells(spec, int(z.row.SizeFt), terrain, origin, &dir)
	case z.row.Shape == zoneRing:
		cells = terrain.OpenArea(origin, grid.RingAt(origin, int(z.row.SizeFt)), false)
	default:
		cells = terrain.OpenArea(origin, scenarioShape(z.row.Shape, int(z.row.SizeFt), origin), false)
	}
	col, row := clamp32(origin.Col, 0, math.MaxInt32), clamp32(origin.Row, 0, math.MaxInt32)
	updated, err := c.q.SetMapZonePlace(ctx, playdb.SetMapZonePlaceParams{ID: z.row.ID, OriginCol: &col, OriginRow: &row, Cells: intsOfSquares(cells)})
	if err != nil {
		return fmt.Errorf("move the zone: %w", err)
	}
	if err := c.reloadZones(ctx); err != nil {
		return err
	}
	moved := zoneStateOf(updated)
	if err := s.writeZoneEvent(ctx, c, eventMapZoneMoved, moved, zoneNote{What: "moved", Moved: clamp32(walked, 0, math.MaxInt32)}, ""); err != nil {
		return err
	}
	for _, o := range cs {
		if !moved.affects(o) || !placed(o) {
			continue
		}
		sq := squareOfCombatant(o)
		if !slices.Contains(moved.cells, sq) || slices.Contains(z.cells, sq) {
			continue
		}
		for _, k := range []zone.TriggerKind{zone.OnEnter, zone.OnEnterFirstTimeOnATurn} {
			if err := s.fireZone(ctx, c, moved, o, fireOpts{kind: k}); err != nil {
				return err
			}
		}
	}
	return s.syncZoneRules(ctx, c)
}

// scenarioShape is the squares of a shape the master put: a sphere, a cylinder or a cube set on
// the point, before the walls.
func scenarioShape(shape string, sizeFt int, origin grid.Square) []grid.Square {
	switch shape {
	case zoneCube:
		return grid.CubeAt(origin, sizeFt)
	case zoneSphere:
		return grid.Area{Shape: grid.ShapeSphere, SizeFt: sizeFt}.FromPoint(origin)
	case zoneCylinder:
		return grid.Area{Shape: grid.ShapeCylinder, SizeFt: sizeFt}.FromPoint(origin)
	}
	return nil
}

// ---- the turn ----

// zonesAtTurnStart runs what the start of the turn of the group ids does to the zones: the ones
// whose time has run (or a wind has dispersed) end, a zone that walks by itself walks, and each
// creature that starts its turn in a zone is hit by the zone's trigger, all before it acts.
func (s *Service) zonesAtTurnStart(ctx context.Context, c *combatTx, ids []string, round int32) error {
	if c.enc.ID == "" {
		return nil
	}
	if err := c.q.PruneZoneFired(ctx, playdb.PruneZoneFiredParams{EncounterID: c.enc.ID, Round: round}); err != nil {
		return fmt.Errorf("clear the old turns of the ledger: %w", err)
	}
	if err := c.reloadZones(ctx); err != nil {
		return err
	}
	if len(c.zones) == 0 {
		return nil
	}
	cs, err := c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return fmt.Errorf("list the combatants: %w", err)
	}
	// 1. The zones whose time has run end at the start of their caster's turn.
	for _, z := range slices.Clone(c.zones) {
		casterOnTurn := z.row.CasterID != nil && slices.Contains(ids, *z.row.CasterID)
		timeUp := z.lasts() && round >= z.row.CastRound+z.row.DurationRounds && (casterOnTurn || z.row.CasterID == nil)
		dispersed := z.row.DisperseRound != nil && round >= *z.row.DisperseRound
		switch {
		case dispersed:
			if err := s.endSpell(ctx, c, z, whatDispersed); err != nil {
				return err
			}
		case timeUp:
			if err := s.endSpell(ctx, c, z, whatEnded); err != nil {
				return err
			}
		}
	}
	// 2. A zone that walks by itself walks at the start of its caster's turn.
	if !isTheatre(c.enc) {
		terrain, err := c.svc.terrainOf(ctx, c.tx, c.session.CampaignID, c.enc)
		if err != nil {
			return err
		}
		for _, z := range slices.Clone(c.zones) {
			if z.row.MovesWith != string(zone.MovesAtTurnStart) || z.row.CasterID == nil || !slices.Contains(ids, *z.row.CasterID) {
				continue
			}
			if err := s.walkZone(ctx, c, cs, z, terrain); err != nil {
				return err
			}
		}
	}
	// 3. The creatures that start their turn in a zone.
	cs, err = c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return fmt.Errorf("list the combatants: %w", err)
	}
	theatre := isTheatre(c.enc)
	for _, id := range ids {
		i := slices.IndexFunc(cs, func(o playdb.Combatant) bool { return o.ID == id })
		if i < 0 || cs[i].Defeated {
			continue
		}
		for _, z := range slices.Clone(c.zones) {
			if !z.inside(cs[i], theatre) {
				continue
			}
			if tr, ok := z.trigger(zone.StartOfTurn); ok && (!tr.RequiresFullyInside || z.fullyInside(cs[i], theatre)) {
				if err := s.fireZone(ctx, c, z, cs[i], fireOpts{kind: zone.StartOfTurn}); err != nil {
					return err
				}
			}
		}
	}
	return s.syncZoneRules(ctx, c)
}

// walkZone moves a zone that walks away from its caster by its step (Cloudkill: 10 ft), in the
// direction away from the caster, and stops at a wall.
func (s *Service) walkZone(ctx context.Context, c *combatTx, cs []playdb.Combatant, z zoneState, terrain grid.Terrain) error {
	i := slices.IndexFunc(cs, func(o playdb.Combatant) bool { return o.ID == deref(z.row.CasterID) })
	if i < 0 || !placed(cs[i]) || z.row.OriginCol == nil || z.row.OriginRow == nil {
		return nil
	}
	origin := grid.Square{Col: int(*z.row.OriginCol), Row: int(*z.row.OriginRow)}
	d, ok := grid.Toward(squareOfCombatant(cs[i]), origin)
	if !ok {
		d = grid.Direction{Dx: 1} // the fog is on its caster: it drifts east
	}
	steps := int(z.row.StepSquares)
	next := origin
	for k := 0; k < steps; k++ {
		cand := grid.Square{Col: next.Col + d.Dx, Row: next.Row + d.Dy}
		if !terrain.Grid.Contains(cand) {
			break
		}
		next = cand
	}
	if next == origin {
		return nil
	}
	return s.moveZoneTo(ctx, c, cs, z, next, terrain, steps)
}

// zonesAtTurnEnd runs what the end of a creature's turn does: a zone that hurts whoever ends
// its turn in it (Grease) or within reach of its damaging side (Wall of Fire).
func (s *Service) zonesAtTurnEnd(ctx context.Context, c *combatTx, who playdb.Combatant) error {
	if err := c.loadZones(ctx); err != nil {
		return err
	}
	theatre := isTheatre(c.enc)
	for _, z := range slices.Clone(c.zones) {
		if z.inside(who, theatre) {
			if err := s.fireZone(ctx, c, z, who, fireOpts{kind: zone.EndOfTurn}); err != nil {
				return err
			}
		}
		if _, ok := z.trigger(zone.EndOfTurnWithin); ok && !theatre && placed(who) {
			if z.wall().Within(squareOfCombatant(who), z.damageSide(), int(z.row.ReachSquares)*grid.FeetPerSquare) {
				if err := s.fireZone(ctx, c, z, who, fireOpts{kind: zone.EndOfTurnWithin}); err != nil {
					return err
				}
			}
		} else if ok && theatre && z.inside(who, theatre) {
			if err := s.fireZone(ctx, c, z, who, fireOpts{kind: zone.EndOfTurnWithin}); err != nil {
				return err
			}
		}
	}
	return nil
}

// ---- the rules of a zone that ask no roll ----

// syncZoneRules keeps the conditions the rules of the zones give: a creature entirely inside a
// zone of silence is deafened, and the condition goes when it leaves. Only what a zone put is
// taken off; a condition the master set by hand stays.
func (s *Service) syncZoneRules(ctx context.Context, c *combatTx) error {
	if err := c.loadZones(ctx); err != nil {
		return err
	}
	deaf := slices.ContainsFunc(c.zones, func(z zoneState) bool { return z.hasRule(zone.Deaf) })
	have := false
	for _, z := range c.zones {
		effs, err := c.q.ListZoneEffects(ctx, z.row.ID)
		if err != nil {
			return fmt.Errorf("list what the zone put: %w", err)
		}
		if slices.ContainsFunc(effs, func(e playdb.MapZoneEffect) bool { return e.Condition == condDeafened }) {
			have = true
		}
	}
	if !deaf && !have {
		return nil
	}
	cs, err := c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return fmt.Errorf("list the combatants: %w", err)
	}
	theatre := isTheatre(c.enc)
	for _, z := range c.zones {
		if !z.hasRule(zone.Deaf) {
			continue
		}
		effs, err := c.q.ListZoneEffects(ctx, z.row.ID)
		if err != nil {
			return fmt.Errorf("list what the zone put: %w", err)
		}
		for _, o := range cs {
			held := slices.ContainsFunc(effs, func(e playdb.MapZoneEffect) bool { return e.CombatantID == o.ID && e.Condition == condDeafened })
			switch in := !o.Defeated && z.fullyInside(o, theatre); {
			case in && !held:
				if err := s.putZoneCondition(ctx, c, z, o, condDeafened, false); err != nil {
					return err
				}
			case !in && held:
				if err := c.q.DeleteZoneEffect(ctx, playdb.DeleteZoneEffectParams{ZoneID: z.row.ID, CombatantID: o.ID, Condition: condDeafened}); err != nil {
					return fmt.Errorf("take the zone's condition off: %w", err)
				}
				if err := s.dropZoneCondition(ctx, c, o, condDeafened); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// silencedAt says the creature cannot cast a spell with a verbal component: it is entirely
// inside a zone of silence.
func silencedAt(zs []zoneState, c playdb.Combatant, theatre bool) bool {
	return slices.ContainsFunc(zs, func(z zoneState) bool { return z.hasRule(zone.NoVerbalCast) && z.fullyInside(c, theatre) })
}

// thunderImmune says the creature is immune to thunder damage: entirely inside a zone of
// silence (SRD, Silence).
func thunderImmune(zs []zoneState, c playdb.Combatant, theatre bool) bool {
	return slices.ContainsFunc(zs, func(z zoneState) bool { return z.hasRule(zone.ThunderImmune) && z.fullyInside(c, theatre) })
}

// The kinds of session events a zone writes (session_event_kinds, migration 00199).
const (
	eventMapZoneAdded     = "map_zone_added"
	eventMapZoneMoved     = "map_zone_moved"
	eventMapZoneEnded     = "map_zone_ended"
	eventMapZoneTriggered = "map_zone_triggered"
	// eventMapZoneChanged is the event of a change of the master's that the log does not show: who is
	// inside, who recognised a zone, whether the players see it, a wind told to disperse it.
	eventMapZoneChanged   = "map_zone_changed"
	eventZoneSaveAnswered = "zone_save_answered"
)
