package play

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"uuid"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
	"github.com/PuraFome/meuRPG/backend/internal/rules/reaction"
)

// The spell cast (CastSpell), in phases. castSpell reads the request, runs the transaction as a sequence of
// phases of a castFlow, each one of the checks or the writes the cast makes, in that order, and
// answers from what the transaction left in a castOut. The phases keep the order of the reads, the dice and
// the writes, because the scripted dice of the tests and the golden of combat_cast_spell_golden_test.go
// depend on it.

const (
	// maxSpellKeyLen is the longest spell key a request may name.
	maxSpellKeyLen = 100
	// maxDartsPerTarget is the most darts one target of a request may be given.
	maxDartsPerTarget = 20
	// maxShownDarts is the most darts a target shows in the cast's event.
	maxShownDarts = 100
)

// castAim is a target of the request: the combatant and the darts it gets (Magic Missile).
type castAim struct {
	id    string
	darts int
}

// castCall is what CastSpell reads from its request, checked.
type castCall struct {
	key, encounterID, casterID, spellKey string
	// grantedBy is the source of the granted_spell effect that casts the spell; "" for the character's classes.
	grantedBy string
	targets   []castAim
	// in is how the dice come; rolled says the request sets them at all.
	in     rollInput
	rolled bool
}

// readCastIDs reads the key, the combat, the caster and the spell the request names.
func readCastIDs(req *playv1.CastSpellRequest) (castCall, error) {
	var call castCall
	var err error
	if call.key, call.encounterID, err = parseCall(req.GetIdempotencyKey(), req.GetEncounterId()); err != nil {
		return call, err
	}
	if call.casterID, err = parseCombatID(req.GetCasterId(), "combatant"); err != nil {
		return call, err
	}
	call.spellKey = req.GetSpellKey()
	if call.spellKey == "" || len(call.spellKey) > maxSpellKeyLen {
		return call, connect.NewError(connect.CodeInvalidArgument, errors.New("spell_key must name one of the caster's spells"))
	}
	if call.grantedBy = req.GetGrantedBy(); len(call.grantedBy) > maxSpellKeyLen {
		return call, connect.NewError(connect.CodeInvalidArgument, errors.New("granted_by must name the trait, feat or feature that grants the spell"))
	}
	return call, nil
}

// readBody reads the targets and the way the dice come, in the order the answers are given.
func (call *castCall) readBody(req *playv1.CastSpellRequest) error {
	if err := call.readTargets(req); err != nil {
		return err
	}
	var err error
	call.in, call.rolled, err = readCastRoll(req)
	return err
}

// readTargets reads the targets: at most one cast takes, each a combatant once, with the darts it gets.
func (call *castCall) readTargets(req *playv1.CastSpellRequest) error {
	if len(req.GetTargets()) > maxSpellTargets {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("targets must have at most %d entries", maxSpellTargets))
	}
	for i, t := range req.GetTargets() {
		id, err := parseCombatID(t.GetCombatantId(), "combatant")
		if err != nil {
			return err
		}
		if slices.ContainsFunc(call.targets, func(o castAim) bool { return o.id == id }) {
			return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("targets[%d] repeats a combatant", i))
		}
		if t.GetDarts() < 0 || t.GetDarts() > maxDartsPerTarget {
			return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("targets[%d].darts must be 0 to %d", i, maxDartsPerTarget))
		}
		call.targets = append(call.targets, castAim{id: id, darts: int(t.GetDarts())})
	}
	return nil
}

// readCastRoll reads how the dice come: rolled in the app, one typed face, a typed pool or the typed faces
// of a pair; rolled says any of them was set.
func readCastRoll(req *playv1.CastSpellRequest) (in rollInput, rolled bool, err error) {
	switch roll := req.GetRoll().(type) {
	case *playv1.CastSpellRequest_RollInApp:
		if !roll.RollInApp {
			return in, false, connect.NewError(connect.CodeInvalidArgument, errors.New("roll_in_app must be true"))
		}
		in.inApp, rolled = true, true
	case *playv1.CastSpellRequest_D20Face:
		in.typed, rolled = int(roll.D20Face), true
		if in.typed < 1 || in.typed > d20Sides {
			return in, rolled, connect.NewError(connect.CodeInvalidArgument, errors.New("d20_face must be 1 to 20"))
		}
	case *playv1.CastSpellRequest_PoolSum:
		in.typed, in.pool, rolled = int(roll.PoolSum), true, true
	default:
		if len(req.GetD20Faces()) > 0 {
			if in.typedFaces, err = typedD20Faces(req.GetD20Faces()); err != nil {
				return in, rolled, err
			}
			rolled = true
		}
	}
	return in, rolled, nil
}

// castOut is what the transaction of a cast leaves for its answer.
type castOut struct {
	made actionEvent
	// modes are the d20 rolls of the cast: their modes and circumstances.
	modes *spellModes
	// vitals are the slot spent, if it was a player's, and the characters a hit point spell changed.
	vitals []*playv1.CharacterVitals
	// asked is the question a spell attack waits on: its d20 was rolled and kept for the Bardic Inspiration
	// die or for Lucky.
	asked *outsideAsk
	v     combatViewer
	// hook is the trap hook that fired a trap on the square a teleport landed on: it tells the streams after
	// the commit.
	hook *trapHook
}

// castSpell is CastSpell for a member already authorized: the replay of a held action calls it
// with the member who made the request (combat_reaction_hold.go).
func (s *Service) castSpell(ctx context.Context, m authz.Membership, req *connect.Request[playv1.CastSpellRequest]) (*connect.Response[playv1.CastSpellResponse], error) {
	call, err := readCastIDs(req.Msg)
	if err != nil {
		return nil, err
	}
	if call.spellKey == revivifyKey { // brings the dead back: its own target, its own checks (revivify.go)
		return s.castRevivify(ctx, req)
	}
	if err := call.readBody(req.Msg); err != nil {
		return nil, err
	}
	out := &castOut{v: viewerOf(heldViewer(ctx, m))}
	res, err := s.write(ctx, combatWrite{m: m, key: call.key, hash: idem.Hash(req.Msg), kind: eventSpellCast, altKind: eventTrapTriggered, encounterID: call.encounterID}, func(c *combatTx) (any, error) {
		return newCastFlow(s, c, m, req.Msg, call, out).run(ctx)
	})
	if err != nil {
		return nil, s.dbError(ctx, "cast a spell", err)
	}
	return s.castResponse(ctx, m, res, out)
}

// castFlow is one run of the cast's transaction: what each phase reads, and the next one uses.
type castFlow struct {
	s    *Service
	c    *combatTx
	m    authz.Membership
	req  *playv1.CastSpellRequest
	call castCall
	out  *castOut

	v      combatViewer
	sees   combatViewer // who picks the targets: the caster's player when the master acts for it
	cs     []playdb.Combatant
	caster playdb.Combatant
	// targets and targs are who the cast touches: an area replaces the list, and a retry starts from the
	// request's.
	targets []castAim
	targs   []playdb.Combatant
	cast    castable
	slot    *slotRef
	// slotLevel is the level the spell is cast at: its own for a cantrip.
	slotLevel  int
	sp         link.Spell
	abilityKey string
	meta       *castMeta
	// ticks are the options of the caster's table features ticked on the cast sheet.
	ticks []tableTick
	// known is what the player knows of the map, for the cover they are told; terrain is the real one.
	known      *grid.Terrain
	terrain    grid.Terrain
	placedArea bool
	plan       areaPlan
	summon     *summoning
	// hidden says the cast shows a hidden creature: the caster or a target.
	hidden bool
	// landing is the square a teleport puts the caster on (checkTeleport); nil for any other cast.
	landing *grid.Square
	// landed is the caster on the landing square once the teleport is written.
	landed *playdb.Combatant
}

func newCastFlow(s *Service, c *combatTx, m authz.Membership, req *playv1.CastSpellRequest, call castCall, out *castOut) *castFlow {
	return &castFlow{s: s, c: c, m: m, req: req, call: call, out: out, targets: slices.Clone(call.targets)}
}

// run is the transaction: the checks, the questions a reaction or a die may hold the cast for, what the cast
// spends and what it does. It answers the event the write stores, or nil when the d20 waits for the player's
// answer.
func (f *castFlow) run(ctx context.Context) (any, error) {
	f.out.vitals, f.out.asked, f.out.hook = nil, nil, nil
	if err := f.prepare(ctx); err != nil {
		return nil, err
	}
	// A Counterspell, or the master's check of the table rule "Reações dos inimigos", can change the cast: it
	// waits for the windows and happens when they are answered (PM-04, combat_reaction_hold.go).
	if held, err := f.holdForWindow(ctx); err != nil {
		return nil, err
	} else if held {
		return f.out.made, nil
	}
	if err := f.holdForDie(ctx); err != nil || f.out.asked != nil {
		return nil, err
	}
	if err := f.spend(ctx); err != nil {
		return nil, err
	}
	if f.c.replay != nil && f.c.replay.countered {
		return f.settleCountered(ctx)
	}
	if err := f.writeCast(ctx); err != nil {
		return nil, err
	}
	return f.fireTrapOnLanding(ctx, f.out.made)
}

// prepare is the part of the cast that checks and reads: who and what, the turn, the spell and its slot, the
// Metamagic, the targets and the dice. Nothing is written.
func (f *castFlow) prepare(ctx context.Context) error {
	for _, phase := range []func(context.Context) error{
		f.findCaster, f.findTargets, f.pickSpell, f.checkEnabled, f.pickSlot, f.applyTableOptions, f.checkTargetsAlive,
		f.applyMetamagic, f.readMap, f.checkTeleport, f.placeArea, f.checkMembers, f.checkTargetList, f.planSummon,
	} {
		if err := phase(ctx); err != nil {
			return err
		}
	}
	f.aimAtCaster()
	for _, phase := range []func(context.Context) error{f.checkPool, f.readModes, f.checkSpellAttack} {
		if err := phase(ctx); err != nil {
			return err
		}
	}
	return nil
}

// writeCast is what the cast does once it is spent: the concentration, the creatures it summons, what it
// does to each target and the event's last marks.
func (f *castFlow) writeCast(ctx context.Context) error {
	for _, phase := range []func(context.Context) error{f.concentrate, f.markHunters, f.bringSummons, f.affect, f.teleport, f.seal} {
		if err := phase(ctx); err != nil {
			return err
		}
	}
	return nil
}

// findCaster reads the combat's combatants as the caller sees them (the fog: an NPC the player does not see
// is not found) and the caster, who the caller must be able to act for and who must be able to cast now.
func (f *castFlow) findCaster(ctx context.Context) error {
	cs, err := listCombatants(ctx, f.c)
	if err != nil {
		return err
	}
	f.cs = cs
	f.v = f.c.viewer(f.m, cs)
	f.out.v = f.c.viewer(heldViewer(ctx, f.m), cs) // what the response reads is the answerer's
	if f.caster, err = findCombatant(cs, f.call.casterID, f.v); err != nil {
		return err
	}
	if err := f.v.mayAct(f.caster); err != nil {
		return err
	}
	f.sees = f.c.actorSight(f.v, cs, f.caster)
	if err := f.s.mustActNow(ctx, f.c, f.caster); err != nil {
		return err
	}
	if err := f.s.refuseInShape(ctx, f.c, f.caster); err != nil { // no spells in a beast form (MR-037)
		return err
	}
	return f.s.refuseWhileRaging(ctx, f.c, f.caster, f.v.master) // no spells in a rage (SRD)
}

// findTargets reads the targets the request lists. An area placed on the map says who it touches itself: the
// list is not read.
func (f *castFlow) findTargets(context.Context) error {
	if f.req.GetArea() != nil {
		return nil
	}
	f.targs = make([]playdb.Combatant, len(f.targets))
	for i, t := range f.targets {
		var err error
		if f.targs[i], err = findCombatant(f.cs, t.id, f.sees); err != nil {
			return err
		}
		// A dead player's character waits for the spell: a healing says it is dead.
		if f.targs[i].Defeated && f.targs[i].Kind != kindPlayer {
			return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TARGET_DEFEATED, "a target is defeated")
		}
	}
	return nil
}

// pickSpell finds the spell among the ones the caster may cast now, as its turn options list them. Quickened
// Spell makes the casting a bonus action: the action being spent does not stop it (the Metamagic checks the
// bonus action).
func (f *castFlow) pickSpell(ctx context.Context) error {
	opts, err := f.s.optionsOf(ctx, f.c.tx, f.m.CampaignID, f.caster)
	if err != nil {
		return err
	}
	if f.cast, err = castableOf(opts, f.call.spellKey, f.call.grantedBy); err != nil {
		return err
	}
	quick := slices.ContainsFunc(f.req.GetMetamagic(), func(mc *playv1.MetamagicChoice) bool { return mc.GetKey() == rules.MetamagicQuickened })
	if quick && !f.cast.enabled && f.cast.reason.GetCode() == rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_ACTION_USED {
		f.cast.enabled = true
	}
	return nil
}

// checkEnabled refuses a spell the caster may not cast now. The master has the last word on the economy only.
func (f *castFlow) checkEnabled(ctx context.Context) error {
	if f.cast.enabled {
		return nil
	}
	// A summoning spell that takes too long is refused with a reason the screen can say ("Leva 1 hora:
	// conjure fora do combate").
	if f.cast.reason.GetCode() == rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_CASTING_TIME_TOO_LONG {
		if long, err := f.s.roster.CombatSpell(ctx, f.c.tx, f.m.CampaignID, f.caster.CharacterID, f.call.spellKey, int(f.cast.level), ""); err == nil && long.Summon {
			return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_CASTING_TIME_TOO_LONG,
				"this spell takes too long to cast in a fight: cast it outside the combat")
		}
	}
	return castError(f.cast.reason, f.v.master)
}

// pickSlot checks the slot the caster chose and reads the spell as it is at that level, with the damage type
// and the ability the caster picked.
func (f *castFlow) pickSlot(ctx context.Context) error {
	var err error
	if f.slot, err = slotOf(f.req.GetSlot(), f.cast); err != nil {
		return err
	}
	f.slotLevel = int(f.cast.level)
	if f.slot != nil {
		f.slotLevel = int(f.slot.Level)
	}
	if f.sp, err = f.s.roster.GrantedCombatSpell(ctx, f.c.tx, f.m.CampaignID, f.caster.CharacterID, f.call.spellKey, f.call.grantedBy, f.slotLevel, f.req.GetDamageTypeKey()); err != nil {
		return err
	}
	if f.abilityKey, err = f.s.abilityChoiceOf(ctx, f.c, f.call.spellKey, f.req.GetAbilityKey()); err != nil {
		return err
	}
	if f.c.replay != nil { // the die a Cutting Words took off the spell attack roll (PM-04)
		f.sp.ToHit -= int(f.c.replay.reduction)
	}
	if pick := f.req.GetDamageTypeKey(); pick != "" && !slices.Contains(f.sp.DamageTypes, pick) {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("damage_type_key must be one of the damage types of a spell that lets the caster choose"))
	}
	return nil
}

// applyTableOptions gives the spell what the options of the caster's table features change in its damage.
func (f *castFlow) applyTableOptions(ctx context.Context) error {
	var err error
	f.sp, f.ticks, err = f.s.applyTableSpellOptions(ctx, f.c, f.m.CampaignID, f.caster, f.req.GetSpellOptions(), f.sp, f.slotLevel)
	return err
}

// checkTargetsAlive refuses a spell aimed at the dead. A dead creature regains no hit points (SRD 5.1), so a
// healing aimed at one is refused before the slot or the action is spent.
func (f *castFlow) checkTargetsAlive(ctx context.Context) error {
	if healsHitPoints(f.sp) {
		if err := f.s.refuseTheDead(ctx, f.c, f.targs); err != nil {
			return err
		}
	}
	for _, t := range f.targs {
		if t.Defeated {
			return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TARGET_DEFEATED, "a target is defeated")
		}
	}
	return nil
}

// applyMetamagic checks the Metamagic against the spell and the points: it changes the range and the casting
// time, and Twinned Spell adds its second target.
func (f *castFlow) applyMetamagic(ctx context.Context) error {
	meta, err := f.s.prepareMetamagic(ctx, f.c, f.m, f.v, f.caster, f.req.GetMetamagic(), f.call.spellKey, f.slotLevel, f.sp, f.targs, f.cs)
	if err != nil {
		return err
	}
	f.sp, f.c.meta, f.meta = meta.apply(f.sp), meta, meta
	if meta != nil && meta.second.ID != "" {
		f.targs, f.targets = append(f.targs, meta.second), append(f.targets, castAim{id: meta.second.ID})
	}
	return nil
}

// readMap reads the map the targets are on: what the player knows of it, for the cover they are told, and the
// real one.
func (f *castFlow) readMap(ctx context.Context) error {
	known, err := f.c.sight.knownTerrain(ctx, f.c.tx, f.v)
	if err != nil {
		return err
	}
	f.known = known
	f.terrain, err = f.s.terrainOf(ctx, f.c.tx, f.m.CampaignID, f.c.enc)
	return err
}

// placeArea works out who an area spell on a map touches: the caster places it and the server says who is
// inside (planArea); without a map the caster lists them and the master judges.
func (f *castFlow) placeArea(ctx context.Context) error {
	// A spell that keeps its area on the map lays it only when the request chooses a point: a cast without one is the
	// spell as it was before the zones (its targets, its effects), and lays nothing.
	if _, ok := zoneDefOfSpell(f.sp); ok && f.req.GetOrigin() != nil && !isTheatre(f.c.enc) {
		f.sp.ZoneLaid = true
	}
	f.placedArea = placementOf(f.sp) != playv1.AreaPlacement_AREA_PLACEMENT_UNSPECIFIED && !isTheatre(f.c.enc)
	switch {
	case f.placedArea:
		return f.planPlacedArea(ctx)
	case f.req.GetArea() != nil:
		return connect.NewError(connect.CodeInvalidArgument, errors.New("this spell has no area to place on this map: list its targets"))
	}
	return nil
}

// planPlacedArea places the area the request chose and takes its creatures as the targets.
func (f *castFlow) planPlacedArea(ctx context.Context) error {
	choice, err := areaChoiceOf(f.req.GetOrigin(), f.req.GetDirection(), grid.Grid{Columns: int(f.c.enc.GridColumns), Rows: int(f.c.enc.GridRows)})
	if err != nil {
		return err
	}
	if f.sp.ZoneLaid && placementOf(f.sp) == playv1.AreaPlacement_AREA_PLACEMENT_CASTER {
		choice = areaChoice{} // centered on the caster: the point only says the cast lays the zone
	}
	if f.plan, err = planArea(areaInput{
		v: f.v, terrain: f.terrain, cs: f.cs, caster: f.caster, sp: f.sp, choice: choice,
		cover: func(origin, target playdb.Combatant) (coverPair, error) {
			return f.c.coverOf(ctx, f.v, f.terrain, f.known, origin, target, f.cs)
		},
	}); err != nil {
		return err
	}
	f.targs, f.targets = make([]playdb.Combatant, len(f.plan.targets)), make([]castAim, len(f.plan.targets))
	for i, t := range f.plan.targets {
		f.targs[i], f.targets[i] = t.who, castAim{id: t.who.ID}
	}
	return nil
}

// checkMembers checks the creatures Careful Spell protects and Sculpt Spells spares are among the targets:
// an area spell on a map only knows its targets once the area is placed.
func (f *castFlow) checkMembers(context.Context) error {
	if err := f.meta.membersIn(f.targs); err != nil {
		return err
	}
	var err error
	f.c.sculpted, err = sculptedOf(f.req.GetSculptedIds(), f.sp, f.slotLevel, f.caster, f.targs, f.v)
	return err
}

// checkTargetList checks who the spell may touch: a summoning spell takes none, and the others are held to
// their number, their darts and their range.
func (f *castFlow) checkTargetList(context.Context) error {
	if f.sp.Summon != (f.req.GetSummon() != nil) {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("summon is for a summoning spell, and a summoning spell needs it"))
	}
	if f.sp.Summon {
		if len(f.targs) > 0 {
			return connect.NewError(connect.CodeInvalidArgument, errors.New("a summoning spell takes no targets: the creatures appear next to the caster"))
		}
		return nil
	}
	if f.placedArea {
		return nil
	}
	dartList := make([]int, len(f.targets))
	for i, t := range f.targets {
		dartList[i] = t.darts
	}
	return f.s.checkMetaTargets(f.meta, f.v, planOn(f.v, f.terrain, f.known), f.cs, f.sp, f.caster, f.targs, dartList, dartsOf(f.sp, f.slotLevel), f.slotLevel, isTheatre(f.c.enc))
}

// planSummon works out the creatures a summoning spell brings.
func (f *castFlow) planSummon(ctx context.Context) error {
	if !f.sp.Summon {
		return nil
	}
	var err error
	f.summon, err = f.s.prepareSummon(ctx, f.c, f.m, f.v, f.caster, f.call.spellKey, f.slotLevel, f.req.GetSummon(), f.call.in, f.call.rolled, f.sp.Concentration)
	return err
}

// aimAtCaster makes the caster the target of a spell that reaches the caster alone.
func (f *castFlow) aimAtCaster() {
	if len(f.targs) == 0 && selfOnly(f.sp) {
		f.targs, f.targets = []playdb.Combatant{f.caster}, []castAim{{id: f.caster.ID}}
	}
}

// checkPool checks the pool of a spell that rolls one was set.
func (f *castFlow) checkPool(ctx context.Context) error {
	hp := f.sp.HP
	if hp == nil || (hp.Kind != rules.SpellKindHPPool && hp.Kind != rules.SpellKindTempHP) {
		return nil
	}
	return f.s.mustRollPool(ctx, f.c.tx, f.m, f.call.in, f.call.rolled)
}

// readModes reads the combat for the d20 rolls of the cast.
func (f *castFlow) readModes(ctx context.Context) error {
	var err error
	f.out.modes, err = f.s.spellModes(ctx, f.c, f.m, f.v, f.cs, f.caster, f.req)
	return err
}

// checkSpellAttack checks the dice of a spell attack: a d20 for each target, and the way the caller rolls
// (RN-18).
func (f *castFlow) checkSpellAttack(ctx context.Context) error {
	if f.sp.AttackType == "" {
		return nil
	}
	in := f.call.in
	if in.pool {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("pool_sum is for a spell that rolls a pool of dice"))
	}
	if !f.call.rolled {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("set roll_in_app or d20_face for a spell attack"))
	}
	if !in.inApp && len(raysAt(f.sp, f.targets)) != 1 {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("d20_face fits a single target: use roll_in_app for several"))
	}
	if f.v.master {
		return nil
	}
	return f.s.mustRollThisWay(ctx, f.c.tx, f.m, in)
}

// holdForWindow lets the cast wait for a reaction window: it answers true, with the event that holds it, when
// it does.
func (f *castFlow) holdForWindow(ctx context.Context) (bool, error) {
	held, ok, err := f.s.holdCast(ctx, f.c, f.m, f.req, f.cs, f.caster, f.call.spellKey, int32(f.slotLevel), f.targs, f.sp.AttackType != "") //nolint:gosec // a slot level, 0 to 9
	if err != nil {
		return false, err
	}
	if ok {
		f.out.made = held
	}
	return ok, nil
}

// holdForDie asks the player of a spell attack against one target, when the character holds a Bardic
// Inspiration die or is a halfling: the d20 is rolled now (and kept, for the die, or for the Lucky question
// of a 1), before anything is spent, and the player decides before the result is told (inspiration_held.go).
// The answer runs the cast again with the d20 already rolled. A cast that waited for a reaction window is not
// asked.
func (f *castFlow) holdForDie(ctx context.Context) error {
	var err error
	f.out.asked, err = f.s.holdSpellAttack(ctx, f.c, f.m, f.req, f.call.key, f.caster, rayCombatants(f.sp, f.targets, f.targs), f.sp, f.call.in, f.out.modes, f.strokeArmor(ctx))
	return err
}

// spend is the cast spending the slot and the economy at once (MR-014).
func (f *castFlow) spend(ctx context.Context) error {
	run, err := breakRun(ctx, f.c, f.caster)
	if err != nil {
		return err
	}
	f.out.made = actionEvent{
		RunBefore: run, Round: f.c.enc.Round, Actor: f.caster.ID, Key: f.call.spellKey, CastID: uuid.New().String(), Slot: f.slot,
		ActionBefore: f.caster.ActionUsed, BonusBefore: f.caster.BonusActionUsed, ReactionBefore: f.caster.ReactionUsed, DashedBefore: f.caster.Dashed,
		SpellCastBefore: f.caster.SpellCast, BonusSpellBefore: f.caster.BonusSpellCast,
	}
	for _, phase := range []func(context.Context) error{f.spendCastSlot, f.spendGrantUse, f.spendPoints, f.spendTableOptions, f.spendEconomy, f.noteSpellCast} {
		if err := phase(ctx); err != nil {
			return err
		}
	}
	return nil
}

// spendCastSlot spends the slot of the caster.
func (f *castFlow) spendCastSlot(ctx context.Context) error {
	if f.slot == nil {
		return nil
	}
	// A player's slot is spent from its vitals, an NPC's is counted on the combatant (one rule for both).
	slotVitals, err := f.s.spendSlotOf(ctx, f.c, f.caster, *f.slot)
	if err != nil {
		return err
	}
	if slotVitals != nil {
		f.out.vitals = append(f.out.vitals, slotVitals)
	}
	return nil
}

// spendGrantUse spends the use a spell cast through a limited grant costs: one of the grant's
// resource, kept in the event for the undo. A cast with a slot and an at-will grant spend none. An NPC's use is
// counted on the combatant.
func (f *castFlow) spendGrantUse(ctx context.Context) error {
	g := f.cast.grant
	if g == nil || f.slot != nil || g.GetAtWill() {
		return nil
	}
	v, err := f.s.spendResourceOf(ctx, f.c, f.caster, g.GetResource())
	if err != nil {
		return err
	}
	if v != nil {
		f.out.vitals = append(f.out.vitals, v)
	}
	f.out.made.GrantUse = g.GetResource()
	return nil
}

// spendPoints spends the sorcery points the Metamagic costs, with the slot.
func (f *castFlow) spendPoints(ctx context.Context) error {
	if f.meta == nil {
		return nil
	}
	cost := clamp32(f.meta.cost, 0, math.MaxInt32)
	pointsVitals, err := f.s.spendResource(ctx, f.c, f.caster.CharacterID, rules.SorceryPointsKey, cost)
	if err != nil {
		return err
	}
	f.out.vitals = append(f.out.vitals, pointsVitals)
	f.out.made.Res = &resourceEvent{Kind: resMetamagic, Spent: cost, Keys: f.meta.keys}
	return nil
}

// spendEconomy spends the bonus action of a bonus action spell, the action of the others.
func (f *castFlow) spendEconomy(ctx context.Context) error {
	after := f.caster
	switch f.sp.Economy {
	case rules.EconomyBonusAction:
		after.BonusActionUsed = true
	default:
		after.ActionUsed = true
	}
	if err := f.c.q.SetCombatantEconomy(ctx, playdb.SetCombatantEconomyParams{
		ID: f.caster.ID, ActionUsed: after.ActionUsed, BonusActionUsed: after.BonusActionUsed, ReactionUsed: after.ReactionUsed, Dashed: after.Dashed,
	}); err != nil {
		return fmt.Errorf("spend the economy: %w", err)
	}
	return nil
}

// noteSpellCast notes the kind of spell it was, for the bonus action spell limit: a cantrip of 1 action leaves
// the turn's spells as they were.
func (f *castFlow) noteSpellCast(ctx context.Context) error {
	sp, caster := f.sp, f.caster
	if combat.IsFreeCantrip(sp.Economy, sp.Level) {
		return nil
	}
	spellCast, bonusSpellCast := caster.SpellCast || sp.Economy != rules.EconomyBonusAction, caster.BonusSpellCast || sp.Economy == rules.EconomyBonusAction
	if err := f.c.q.SetCombatantSpellsCast(ctx, playdb.SetCombatantSpellsCastParams{ID: caster.ID, SpellCast: spellCast, BonusSpellCast: bonusSpellCast}); err != nil {
		return fmt.Errorf("note the spell cast: %w", err)
	}
	return nil
}

// settleCountered ends a cast that a Counterspell failed: the slot and the action are spent, as casting it
// expended them, and nothing else happens (SRD, Counterspell).
func (f *castFlow) settleCountered(ctx context.Context) (any, error) {
	made := &f.out.made
	made.Secret = f.caster.Hidden
	made.Reaction = &reactionEvent{Kind: string(reaction.CounterspellKind), Countered: true, Spell: f.call.spellKey, Level: int32(f.slotLevel)} //nolint:gosec // a slot level, 0 to 9
	if err := touch(ctx, f.c); err != nil {
		return nil, err
	}
	f.c.characterID = &f.caster.CharacterID
	return *made, nil
}

// concentrate makes the spell the caster's concentration, if it asks for one. A new concentration spell ends
// the old one, and the creatures that lasted only while it did (MR-037, RN-22).
func (f *castFlow) concentrate(ctx context.Context) error {
	if !f.sp.Concentration {
		return nil
	}
	made := &f.out.made
	made.Concentrate, made.ConcBefore = true, conv.OrZero(f.caster.ConcentrationSpell)
	made.ConcEnded = made.ConcBefore
	var err error
	if made.Dismissed, err = f.s.endSummons(ctx, f.c, f.caster); err != nil {
		return err
	}
	if err := f.s.endZonesOf(ctx, f.c, f.caster, zoneEndReplaced); err != nil { // the areas it kept on the map: the line says why
		return err
	}
	if err := f.s.endConcentrationEffects(ctx, f.c, f.caster, endConcentration); err != nil { // its effects end with it (RN-22)
		return err
	}
	if err := f.c.q.SetCombatantConcentration(ctx, playdb.SetCombatantConcentrationParams{ID: f.caster.ID, ConcentrationSpell: &f.call.spellKey}); err != nil {
		return fmt.Errorf("set the concentration: %w", err)
	}
	return nil
}

// markHunters moves the Hunter's Mark: the old concentration's mark goes, and a new Hunter's Mark marks its
// target.
func (f *castFlow) markHunters(ctx context.Context) error {
	if !f.sp.Concentration {
		return nil
	}
	made := &f.out.made
	if made.ConcBefore == huntersMark {
		if err := f.s.clearHuntersMark(ctx, f.c, f.caster, made); err != nil {
			return err
		}
	}
	if f.call.spellKey == huntersMark && len(f.targs) == 1 {
		return f.s.markHuntersMark(ctx, f.c, f.caster, f.targs[0], made)
	}
	return nil
}

// bringSummons brings the creatures a summoning spell made into the combat.
func (f *castFlow) bringSummons(ctx context.Context) error {
	if f.summon == nil {
		return nil
	}
	return f.s.joinSummon(ctx, f.c, f.caster, f.call.spellKey, f.sp.Concentration, f.summon, &f.out.made)
}

// affect is what the cast does to each target.
func (f *castFlow) affect(ctx context.Context) error {
	f.c.castID = f.out.made.CastID
	f.hidden = f.caster.Hidden
	if f.sp.HP != nil {
		return f.affectHitPoints(ctx)
	}
	if def, ok := zoneDefOfSpell(f.sp); ok && f.sp.ZoneLaid { // the area stays on the map (zones_cast.go)
		return f.affectZone(ctx, def)
	}
	for _, i := range raysAt(f.sp, f.targets) { // a ray of Scorching Ray is an attack of its own
		if err := f.hitTarget(ctx, i, f.targs[i]); err != nil {
			return err
		}
	}
	if err := f.pushFailedSaves(ctx); err != nil { // the forced move of a failed save (combat_forced.go)
		return err
	}
	// What a spell that lasts leaves on the targets it took hold of (RN-22).
	return f.s.applyLastingSpell(ctx, f.c, f.cs, f.sp, f.slotLevel, f.caster, f.targs, f.abilityKey, &f.out.made)
}

// affectHitPoints is a hit point spell (a pool of dice that sleeps, or hit points that change).
func (f *castFlow) affectHitPoints(ctx context.Context) error {
	hits, changed, err := f.s.castHPSpell(ctx, f.c, f.sp, f.targs, f.call.in, &f.out.made)
	if err != nil {
		return err
	}
	f.out.made.Hits = hits
	f.out.vitals = append(f.out.vitals, changed...)
	for _, t := range f.targs {
		f.hidden = f.hidden || t.Hidden
	}
	return nil
}

// hitTarget is the spell on one target: its cover, and the attack or the save it asks.
func (f *castFlow) hitTarget(ctx context.Context, i int, t playdb.Combatant) error {
	hit := castHit{Target: t.ID, Darts: shownDarts(f.sp, f.targets[i])}
	var cp coverPair
	if f.placedArea {
		cp = f.plan.targets[i].cp // measured from the point of origin
	} else {
		var err error
		if cp, err = f.c.coverOf(ctx, f.v, f.terrain, f.known, f.caster, t, f.cs); err != nil {
			return err
		}
	}
	if err := f.s.resolveOnTarget(ctx, f.c, f.m, f.sp, f.caster, t, f.slotLevel, f.call.in, cp, &hit, f.out.modes, &f.out.made); err != nil {
		return err
	}
	f.out.made.Hits = append(f.out.made.Hits, hit)
	f.hidden = f.hidden || t.Hidden
	return nil
}

// seal writes the last marks of the event: the cover seen, the die used, who it is secret from, the area, the
// hidden creatures it hit and the caster who gave its place away by its noise.
func (f *castFlow) seal(ctx context.Context) error {
	made := &f.out.made
	packCoverSeen(made)
	made.Die = f.out.modes.dieUsed()
	made.Stroke = f.out.modes.strokeUsed() // the miss the caster turned into a hit: the undo gives the use back
	made.Secret = f.hidden
	if f.placedArea {
		if err := f.sealPlaced(ctx); err != nil {
			return err
		}
	}
	var err error
	// A spell with a verbal component gives a hidden caster's position away by its noise; one with only
	// somatic or material components does not (combat_hide.go).
	if f.sp.Verbal {
		if made.HidBefore, err = f.s.endHiding(ctx, f.c, f.caster); err != nil {
			return err
		}
	}
	if err := touch(ctx, f.c); err != nil {
		return err
	}
	f.c.characterID = &f.caster.CharacterID
	return nil
}

// sealPlaced marks the event of a cast with a placed area: where it landed, and what the hidden creatures it
// hit are told. The line is the players', without the hidden creatures the area hit; only a hidden caster
// makes it the master's alone.
func (f *castFlow) sealPlaced(ctx context.Context) error {
	made := &f.out.made
	made.Secret, made.Placed = f.caster.Hidden, true
	if choice := f.req.GetOrigin(); choice != nil {
		made.AreaCol, made.AreaRow = choice.GetCol(), choice.GetRow()
	}
	if dir := f.req.GetDirection(); dir != nil {
		made.Dx, made.Dy = dir.GetDx(), dir.GetDy()
	}
	return f.s.settleHidden(ctx, f.c, f.m, f.v, f.plan, f.call.spellKey, f.req.RevealHidden, made)
}

// castResponse answers the cast from what the transaction left: the question a spell attack waits on, the
// combat held by a reaction window, or the cast itself.
func (s *Service) castResponse(ctx context.Context, m authz.Membership, res combatResult, out *castOut) (*connect.Response[playv1.CastSpellResponse], error) {
	if out.asked != nil { // nothing was spent or written: the question is the whole answer
		enc, err := s.finish(ctx, m, res, nil)
		if err != nil {
			return nil, err
		}
		return connect.NewResponse(&playv1.CastSpellResponse{Encounter: enc, InspirationOffer: out.asked.Offer, LuckyOffer: out.asked.Lucky, HeldRollOffer: out.asked.Held}), nil
	}
	var err error
	if out.v, err = s.viewerAfter(ctx, m, res, out.v); err != nil { // a replay never ran the closure: the fog filter still holds
		return nil, s.dbError(ctx, "work out what the player sees", err)
	}
	ev, err := resultEvent(res, out.made)
	if err != nil {
		return nil, s.dbError(ctx, "read the spell", err)
	}
	enc, err := s.finish(ctx, m, res, s.castPublisher(m, ev, out))
	if err != nil {
		return nil, err
	}
	if ev.Reaction != nil && ev.Reaction.Hold { // a reaction window holds the cast: nothing of it happened yet
		return connect.NewResponse(&playv1.CastSpellResponse{Encounter: enc}), nil
	}
	return s.castReply(ctx, res, ev, out, enc)
}

// castPublisher tells the streams what the cast changed: the combat and its log, the vitals it moved, the
// hidden hit question and the turn a revealed creature joined.
func (s *Service) castPublisher(m authz.Membership, ev actionEvent, out *castOut) func(ctx context.Context, d *encounterData) {
	return func(ctx context.Context, d *encounterData) {
		s.publishTeleport(ctx, m.CampaignID, d, ev)
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, !ev.Secret)
		s.publishPushed(ctx, m.CampaignID, d, pushesOf(ev.Hits))
		out.hook.after(ctx, m.CampaignID, d.enc) // the trap a teleport landed on
		for _, vit := range out.vitals {
			s.publishVitals(m.CampaignID, vit)
		}
		if ev.PendingReveal != "" { // the master alone is told a question waits
			s.publishHiddenHitPending(m.CampaignID, d.enc.ID, ev.PendingReveal)
		}
		if anyInTurn(d, ev.revealedIDs()) { // a creature of the turn that appeared changes the players' copy of it
			s.publishTurnChanged(ctx, m.CampaignID, d)
		}
	}
}

// castReply is the answer of a cast that happened: the cast as the viewer reads it, the creatures it made and
// the area it landed on.
func (s *Service) castReply(ctx context.Context, res combatResult, ev actionEvent, out *castOut, enc *playv1.Encounter) (*connect.Response[playv1.CastSpellResponse], error) {
	spell, err := s.castProto(ctx, res, ev, out.v)
	if err != nil {
		return nil, err
	}
	out.modes.attach(spell)
	resp := &playv1.CastSpellResponse{Encounter: enc, Cast: spell, SummonedCombatantIds: s.combatantsOfCreatures(ctx, res, ev.Created)}
	if resp.Area, err = s.areaProto(ctx, res, ev, out.v); err != nil {
		return nil, err
	}
	if out.v.master { // what the hidden creatures the cast hit are is the master's alone (RN-10)
		for _, h := range ev.Hits {
			if h.HiddenAtCast {
				resp.HiddenHits = append(resp.HiddenHits, h.Target)
			}
		}
		resp.PendingRevealId = ev.PendingReveal
	}
	return connect.NewResponse(resp), nil
}
