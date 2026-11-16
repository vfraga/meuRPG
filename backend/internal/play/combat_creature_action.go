package play

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// The master uses an action of an NPC's stat block that is no attack roll (UseCreatureAction): a breath
// weapon or another action that asks a saving throw, or one that rolls nothing. The engine rolls what the
// action rolls: each target's save against the action's DC, the damage it governs as pending damage (half
// for a target that saves, when the action says so) and the condition a failed save gives. It is the
// spell machinery with a spell made from the action (spellOfPlan), written as the event of a cast (the
// key is the action's) so that the log, the undo and the fog read it as they read one; creatureActionEvent
// says the rest. The flow is castFlow's, shorter: there is no slot, no Metamagic, no concentration and no
// reaction window.

const (
	// maxActionKeyLen is the longest action key a request may name.
	maxActionKeyLen = 200
	// actionKindSave and actionKindOther are the plan's kinds this RPC uses; an attack roll and a Multiattack
	// are made with TakeAction.
	actionKindSave  = "save"
	actionKindOther = "other"
)

// creatureCall is what UseCreatureAction reads from its request, checked.
type creatureCall struct {
	key, encounterID, actorID, actionKey string
	targets                              []string
}

// readCreatureCall reads the key, the combat, the combatant, the action and the targets.
func readCreatureCall(req *playv1.UseCreatureActionRequest) (creatureCall, error) {
	var call creatureCall
	var err error
	if call.key, call.encounterID, err = parseCall(req.GetIdempotencyKey(), req.GetEncounterId()); err != nil {
		return call, err
	}
	if call.actorID, err = parseCombatID(req.GetCombatantId(), "combatant"); err != nil {
		return call, err
	}
	if call.actionKey = req.GetActionKey(); call.actionKey == "" || len(call.actionKey) > maxActionKeyLen {
		return call, connect.NewError(connect.CodeInvalidArgument, errors.New("action_key must name an action of the creature's stat block"))
	}
	if len(req.GetTargetIds()) > maxSpellTargets {
		return call, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("target_ids must have at most %d entries", maxSpellTargets))
	}
	for i, raw := range req.GetTargetIds() {
		id, err := parseCombatID(raw, "combatant")
		if err != nil {
			return call, err
		}
		if slices.Contains(call.targets, id) {
			return call, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("target_ids[%d] repeats a combatant", i))
		}
		call.targets = append(call.targets, id)
	}
	return call, nil
}

// creatureOut is what the transaction of a creature's action leaves for its answer.
type creatureOut struct {
	made actionEvent
	v    combatViewer
}

// UseCreatureAction implements playv1connect.CombatServiceHandler.
func (s *Service) UseCreatureAction(
	ctx context.Context,
	req *connect.Request[playv1.UseCreatureActionRequest],
) (*connect.Response[playv1.UseCreatureActionResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	call, err := readCreatureCall(req.Msg)
	if err != nil {
		return nil, err
	}
	out := &creatureOut{v: viewerOf(m)}
	res, err := s.write(ctx, combatWrite{m: m, key: call.key, hash: idem.Hash(req.Msg), kind: eventSpellCast, encounterID: call.encounterID}, func(c *combatTx) (any, error) {
		return newCreatureFlow(s, c, m, req.Msg, call, out).run(ctx)
	})
	if err != nil {
		return nil, s.dbError(ctx, "use a creature's action", err)
	}
	return s.creatureResponse(ctx, m, res, out)
}

// creatureFlow is one run of the transaction: what each phase reads, and the next one uses.
type creatureFlow struct {
	s    *Service
	c    *combatTx
	m    authz.Membership
	req  *playv1.UseCreatureActionRequest
	call creatureCall
	out  *creatureOut

	v     combatViewer
	cs    []playdb.Combatant
	actor playdb.Combatant
	plan  rules.ActionPlan
	sp    link.Spell
	targs []playdb.Combatant
	modes *spellModes
	// known is what the table knows of the map; terrain is the real one.
	known      *grid.Terrain
	terrain    grid.Terrain
	placedArea bool
	area       areaPlan
	hidden     bool
}

func newCreatureFlow(s *Service, c *combatTx, m authz.Membership, req *playv1.UseCreatureActionRequest, call creatureCall, out *creatureOut) *creatureFlow {
	return &creatureFlow{s: s, c: c, m: m, req: req, call: call, out: out}
}

// run is the transaction: the checks, what the action spends and what it does. It answers the event.
func (f *creatureFlow) run(ctx context.Context) (any, error) {
	for _, phase := range []func(context.Context) error{
		f.findActor, f.pickAction, f.readMap, f.placeArea, f.findTargets, f.readModes, f.spend, f.affect, f.seal,
	} {
		if err := phase(ctx); err != nil {
			return nil, err
		}
	}
	return f.out.made, nil
}

// unavailable is the refusal of an action this RPC does not make.
func unavailable(msg string) error {
	return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_CREATURE_ACTION_UNAVAILABLE, msg)
}

// findActor reads the combatants and the NPC that acts: on its turn, able to act, with its action not spent.
func (f *creatureFlow) findActor(ctx context.Context) error {
	cs, err := listCombatants(ctx, f.c)
	if err != nil {
		return err
	}
	f.cs, f.v = cs, f.c.viewer(f.m, cs)
	f.out.v = f.v
	if f.actor, err = findCombatant(cs, f.call.actorID, f.v); err != nil {
		return err
	}
	if f.actor.Kind != kindNPC {
		return unavailable("only an NPC uses the actions of a stat block")
	}
	if err := f.s.mustActNow(ctx, f.c, f.actor); err != nil {
		return err
	}
	if f.actor.ActionUsed {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ACTION_USED, "the action of the turn is spent")
	}
	return nil
}

// pickAction finds the action in the stat block: one that asks a saving throw or rolls nothing.
func (f *creatureFlow) pickAction(ctx context.Context) error {
	plan, ok, err := f.s.planOfAction(ctx, f.c, f.actor, f.call.actionKey)
	if err != nil {
		return err
	}
	if !ok {
		return unavailable("the creature has no such action")
	}
	if plan.Attack != nil || (plan.Kind != actionKindSave && plan.Kind != actionKindOther) || plan.Routines != nil {
		return unavailable("this action is an attack roll or a Multiattack: make it with the attack")
	}
	if err := checkActionLimit(plan, f.actor); err != nil {
		return err
	}
	f.plan, f.sp = plan, spellOfPlan(plan)
	if f.reminder() && len(f.call.targets) > 0 {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("an action with no roll takes no targets"))
	}
	return nil
}

// reminder says the action rolls nothing: the line is the whole of it.
func (f *creatureFlow) reminder() bool { return f.plan.Save == nil }

// readMap reads the map the targets are on.
func (f *creatureFlow) readMap(ctx context.Context) error {
	known, err := f.c.sight.knownTerrain(ctx, f.c.tx, f.v)
	if err != nil {
		return err
	}
	f.known = known
	f.terrain, err = f.s.terrainOf(ctx, f.c.tx, f.m.CampaignID, f.c.enc)
	return err
}

// placeArea works out who an area on a map touches, as a spell's: the master places it and the server
// says who is inside. The creature itself is not one of them. Without a map the master names the targets.
func (f *creatureFlow) placeArea(ctx context.Context) error {
	f.placedArea = !f.reminder() && placementOf(f.sp) != playv1.AreaPlacement_AREA_PLACEMENT_UNSPECIFIED && !isTheatre(f.c.enc)
	if !f.placedArea {
		if f.req.GetArea() != nil {
			return connect.NewError(connect.CodeInvalidArgument, errors.New("this action has no area to place on this map: list its targets"))
		}
		return nil
	}
	choice, err := areaChoiceOf(f.req.GetOrigin(), f.req.GetDirection(), grid.Grid{Columns: int(f.c.enc.GridColumns), Rows: int(f.c.enc.GridRows)})
	if err != nil {
		return err
	}
	if f.area, err = planArea(areaInput{
		v: f.v, terrain: f.terrain, cs: f.cs, caster: f.actor, sp: f.sp, choice: choice,
		cover: func(origin, target playdb.Combatant) (coverPair, error) {
			return f.c.coverOf(ctx, f.v, f.terrain, f.known, origin, target, f.cs)
		},
	}); err != nil {
		return err
	}
	f.area.targets = slices.DeleteFunc(f.area.targets, func(t areaTarget) bool { return t.who.ID == f.actor.ID })
	f.targs = make([]playdb.Combatant, len(f.area.targets))
	for i, t := range f.area.targets {
		f.targs[i] = t.who
	}
	return nil
}

// findTargets reads the targets the master names: living creatures other than the one that acts.
func (f *creatureFlow) findTargets(context.Context) error {
	if f.placedArea || f.reminder() {
		return nil
	}
	if len(f.call.targets) == 0 {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("target_ids must name the creatures the action affects"))
	}
	for _, id := range f.call.targets {
		t, err := findCombatant(f.cs, id, f.v)
		if err != nil {
			return err
		}
		if t.ID == f.actor.ID {
			return connect.NewError(connect.CodeInvalidArgument, errors.New("the creature is not a target of its own action"))
		}
		if t.Defeated {
			return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TARGET_DEFEATED, "a target is defeated")
		}
		f.targs = append(f.targs, t)
	}
	return nil
}

// readModes reads the combat for the d20 rolls of the saves.
func (f *creatureFlow) readModes(ctx context.Context) error {
	var err error
	f.modes, err = f.s.spellModes(ctx, f.c, f.m, f.v, f.cs, f.actor, &playv1.CastSpellRequest{}, "")
	return err
}

// spend spends the action and starts the event.
func (f *creatureFlow) spend(ctx context.Context) error {
	run, err := breakRun(ctx, f.c, f.actor)
	if err != nil {
		return err
	}
	a := f.actor
	f.out.made = actionEvent{
		RunBefore: run, Round: f.c.enc.Round, Actor: a.ID, Key: f.plan.Key, CastID: uuid.New().String(),
		ActionBefore: a.ActionUsed, BonusBefore: a.BonusActionUsed, ReactionBefore: a.ReactionUsed, DashedBefore: a.Dashed,
		SpellCastBefore: a.SpellCast, BonusSpellBefore: a.BonusSpellCast,
		CreatureAct: &creatureActionEvent{
			Name: nameOfPlan(f.plan), Reminder: f.reminder(), Text: f.plan.Text,
		},
	}
	if sv := f.plan.Save; sv != nil {
		f.out.made.CreatureAct.ConditionKey, f.out.made.CreatureAct.ConditionName = sv.ConditionKey, sv.ConditionNamePT
		f.out.made.CreatureAct.Duration, f.out.made.CreatureAct.RepeatSave = sv.Duration, sv.RepeatSave
	}
	if err := f.c.q.SetCombatantEconomy(ctx, playdb.SetCombatantEconomyParams{
		ID: a.ID, ActionUsed: true, BonusActionUsed: a.BonusActionUsed, ReactionUsed: a.ReactionUsed, Dashed: a.Dashed,
	}); err != nil {
		return fmt.Errorf("spend the action: %w", err)
	}
	f.out.made.LimitKey, err = spendActionLimit(ctx, f.c, f.plan, a)
	return err
}

// affect is what the action does to each target.
func (f *creatureFlow) affect(ctx context.Context) error {
	f.c.castID = f.out.made.CastID
	f.hidden = f.actor.Hidden
	for i, t := range f.targs {
		if err := f.hitTarget(ctx, i, t); err != nil {
			return err
		}
	}
	return nil
}

// hitTarget is the action on one target: its cover, the save and the damage it leaves, and the condition
// a failed save gives.
func (f *creatureFlow) hitTarget(ctx context.Context, i int, t playdb.Combatant) error {
	hit := castHit{Target: t.ID}
	var cp coverPair
	if f.placedArea {
		cp = f.area.targets[i].cp
	} else {
		var err error
		if cp, err = f.c.coverOf(ctx, f.v, f.terrain, f.known, f.actor, t, f.cs); err != nil {
			return err
		}
	}
	if err := f.s.resolveOnTarget(ctx, f.c, f.m, f.sp, f.actor, t, 0, rollInput{inApp: true}, cp, &hit, f.modes, &f.out.made); err != nil {
		return err
	}
	if err := f.giveCondition(ctx, t, &hit); err != nil {
		return err
	}
	f.out.made.Hits = append(f.out.made.Hits, hit)
	f.hidden = f.hidden || t.Hidden
	return nil
}

// giveCondition gives the condition of the action to a target that failed its save, unless the target
// is immune to it or already has it. The duration is a note of the line: nothing counts the time.
func (f *creatureFlow) giveCondition(ctx context.Context, t playdb.Combatant, hit *castHit) error {
	sv := f.plan.Save
	if sv == nil || sv.ConditionKey == "" || hit.Save == nil || hit.Save.Saved {
		return nil
	}
	return f.s.giveCondition(ctx, f.c, t, sv.ConditionKey, hit)
}

// seal writes the last marks of the event: the cover seen, who it is secret from, the area, the hidden
// creatures it hit and the creature that gave its place away.
func (f *creatureFlow) seal(ctx context.Context) error {
	made := &f.out.made
	packCoverSeen(made)
	made.Secret = f.hidden
	if f.placedArea {
		if err := f.sealPlaced(ctx); err != nil {
			return err
		}
	}
	var err error
	if made.HidBefore, err = f.s.endHiding(ctx, f.c, f.actor); err != nil { // acting gives a hidden creature away
		return err
	}
	if err := touch(ctx, f.c); err != nil {
		return err
	}
	f.c.characterID = &f.actor.CharacterID
	return nil
}

// sealPlaced marks the event of an area the server placed: where it landed, and what the hidden
// creatures it hit are told (the line is the players', without them).
func (f *creatureFlow) sealPlaced(ctx context.Context) error {
	made := &f.out.made
	made.Secret, made.Placed = f.actor.Hidden, true
	if choice := f.req.GetOrigin(); choice != nil {
		made.AreaCol, made.AreaRow = choice.GetCol(), choice.GetRow()
	}
	if dir := f.req.GetDirection(); dir != nil {
		made.Dx, made.Dy = dir.GetDx(), dir.GetDy()
	}
	return f.s.settleHidden(ctx, f.c, f.m, f.v, f.area, f.plan.Key, nil, made)
}

// creatureResponse answers from what the transaction left.
func (s *Service) creatureResponse(ctx context.Context, m authz.Membership, res combatResult, out *creatureOut) (*connect.Response[playv1.UseCreatureActionResponse], error) {
	ev, err := resultEvent(res, out.made)
	if err != nil {
		return nil, s.dbError(ctx, "read the action", err)
	}
	enc, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, !ev.Secret)
		s.publishPushed(ctx, m.CampaignID, d, pushesOf(ev.Hits))
		if ev.PendingReveal != "" {
			s.publishHiddenHitPending(m.CampaignID, d.enc.ID, ev.PendingReveal)
		}
		if anyInTurn(d, ev.revealedIDs()) {
			s.publishTurnChanged(ctx, m.CampaignID, d)
		}
	})
	if err != nil {
		return nil, err
	}
	cast, err := s.castProto(ctx, res, ev, out.v)
	if err != nil {
		return nil, err
	}
	resp := &playv1.UseCreatureActionResponse{Encounter: enc, Result: creatureResult(ev, cast)}
	if resp.Area, err = s.areaProto(ctx, res, ev, out.v); err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// creatureResult is the answer's account of the action, from the cast the event tells.
func creatureResult(ev actionEvent, cast *playv1.SpellCast) *playv1.CreatureActionResult {
	cr := ev.CreatureAct
	out := &playv1.CreatureActionResult{CastId: ev.CastID, ActionKey: ev.Key, PendingDamages: cast.GetPendingDamages()}
	if cr != nil {
		out.Reminder, out.ConditionKey, out.ConditionNamePt = cr.Reminder, cr.ConditionKey, cr.ConditionName
		out.DurationText, out.RepeatSave = cr.Duration, cr.RepeatSave
	}
	for _, t := range cast.GetTargets() {
		given := slices.ContainsFunc(ev.Hits, func(h castHit) bool { return h.Target == t.GetCombatantId() && h.CondSet })
		out.Targets = append(out.Targets, &playv1.CreatureActionTarget{
			CombatantId: t.GetCombatantId(), Save: t.GetSave(), PendingDamageId: t.GetPendingDamageId(),
			MorePendingDamageIds: t.GetMorePendingDamageIds(), ConditionApplied: given,
		})
	}
	return out
}
