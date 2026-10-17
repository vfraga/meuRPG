package play

import (
	"context"
	"fmt"
	"math"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// The master settling a hit on a player's character (ApplyPendingDamage and DiscardPendingDamage), in phases.
// Each call reads the request, runs the transaction as a sequence of phases of a flow, each one of the checks
// or the writes the call has always made in that order, and answers from what the transaction left in an out.
// Applying works the amount out of the roll, the target's resistances and the master's last word, and lands
// it as a character's vitals, a druid's beast form or a character at 0 hit points would take it. The phases
// keep the order of the reads and the writes: the golden of combat_actions_rest_frozen_test.go reads it.

// resolvedDamageError is the refusal of a pending damage that was settled already.
func resolvedDamageError() error {
	return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_DAMAGE_RESOLVED, "the damage was applied or discarded already")
}

// applyDamageCall is what ApplyPendingDamage reads from its request, checked.
type applyDamageCall struct {
	key, encounterID, pendingID string
	// override is the master's last word on the amount, if he sets one.
	override *int32
	// ignored are the sources of resistance he leaves out.
	ignored []string
}

// readApplyDamageCall reads the request, in the order the answers are given: the ids, the amount, the sources.
func readApplyDamageCall(req *playv1.ApplyPendingDamageRequest) (applyDamageCall, error) {
	var call applyDamageCall
	var err error
	if call.key, err = parseKey(req.GetIdempotencyKey()); err != nil {
		return call, err
	}
	if call.encounterID, err = parseCombatID(req.GetEncounterId(), "encounter"); err != nil {
		return call, err
	}
	if call.pendingID, err = parseCombatID(req.GetPendingDamageId(), "pending damage"); err != nil {
		return call, err
	}
	call.override = req.Amount
	if call.override != nil && (*call.override < 0 || *call.override > maxHitPointChange) {
		return call, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("amount must be 0 to %d", maxHitPointChange))
	}
	call.ignored = req.GetIgnoreModifiers()
	if len(call.ignored) > maxIgnoredSources {
		return call, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("ignore_modifiers must have at most %d sources", maxIgnoredSources))
	}
	return call, nil
}

// applyDamageOut is what the transaction of an apply leaves for its answer.
type applyDamageOut struct {
	made actionEvent
	// vitals are the target's, after.
	vitals *playv1.CharacterVitals
}

// ApplyPendingDamage implements playv1connect.CombatServiceHandler.
func (s *Service) ApplyPendingDamage(
	ctx context.Context,
	req *connect.Request[playv1.ApplyPendingDamageRequest],
) (*connect.Response[playv1.ApplyPendingDamageResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	call, err := readApplyDamageCall(req.Msg)
	if err != nil {
		return nil, err
	}
	out := &applyDamageOut{}
	res, err := s.write(ctx, combatWrite{m: m, key: call.key, hash: idem.Hash(req.Msg), kind: eventDamageApplied, encounterID: call.encounterID}, func(c *combatTx) (any, error) {
		return newApplyDamageFlow(s, c, call, out).run(ctx)
	})
	if err != nil {
		return nil, s.dbError(ctx, "apply a pending damage", err)
	}
	return s.applyDamageResponse(ctx, m, res, out)
}

// applyDamageResponse answers from what the transaction left: the encounter as the master sees it and the
// pending damage that landed.
func (s *Service) applyDamageResponse(ctx context.Context, m authz.Membership, res combatResult, out *applyDamageOut) (*connect.Response[playv1.ApplyPendingDamageResponse], error) {
	ev, err := resultEvent(res, out.made)
	if err != nil {
		return nil, s.dbError(ctx, "read the applied damage", err)
	}
	enc, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
		s.publishReturned(ctx, m.CampaignID, d, ev)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, !ev.Secret)
		s.publishVitals(m.CampaignID, out.vitals)
	})
	if err != nil {
		return nil, err
	}
	pending, err := s.pendingFor(ctx, res, ev.Pending, combatViewer{master: true})
	if err != nil {
		return nil, err
	}
	decorate(pending, ev, combatViewer{master: true}, s.membersOf(ctx, res))
	return connect.NewResponse(&playv1.ApplyPendingDamageResponse{Encounter: enc, PendingDamage: pending}), nil
}

// applyDamageFlow is one run of the apply's transaction: what each phase reads, and the next one uses.
type applyDamageFlow struct {
	s    *Service
	c    *combatTx
	call applyDamageCall
	out  *applyDamageOut

	p                playdb.PendingDamage
	attacker, target playdb.Combatant
	// amount is what lands, and applied the amount the master set when it is not the roll.
	amount  int32
	applied *int32
	// now are the target's vitals before, and down says it is at 0 hit points.
	now  *playv1.CharacterVitals
	down bool
	made actionEvent
}

func newApplyDamageFlow(s *Service, c *combatTx, call applyDamageCall, out *applyDamageOut) *applyDamageFlow {
	return &applyDamageFlow{s: s, c: c, call: call, out: out}
}

// run is the transaction: the checks, the amount, the landing and what follows it. It answers the event the
// write stores.
func (f *applyDamageFlow) run(ctx context.Context) (any, error) {
	f.out.vitals = nil
	phases := []func(context.Context) error{f.locate, f.settleAmount, f.readVitals, f.land, f.afterTaken, f.record, f.castSave, f.afterHit, f.touch}
	for _, phase := range phases {
		if err := phase(ctx); err != nil {
			return nil, err
		}
	}
	f.out.made = f.made
	return f.made, nil
}

// locate checks the combat and the hit the master settles, and finds its two combatants.
func (f *applyDamageFlow) locate(ctx context.Context) error {
	if err := notEnded(f.c.enc); err != nil {
		return err
	}
	if err := f.s.reactionGate(ctx, f.c); err != nil { // a reaction window holds the damage too (PM-04)
		return err
	}
	var err error
	if f.p, f.attacker, f.target, err = f.s.settling(ctx, f.c, f.call.pendingID); err != nil {
		return err
	}
	if err := settleable(f.p); err != nil {
		return err
	}
	if f.target.Kind != kindPlayer {
		return fmt.Errorf("pending damage %s is rolled for an NPC", f.p.ID) // an NPC takes it when rolled
	}
	return nil
}

// settleable refuses a pending damage that cannot be applied now: it waits for a reaction or for its roll,
// or it was settled already.
func settleable(p playdb.PendingDamage) error {
	switch p.Status {
	case pendingAwaitingReaction:
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_REACTION_PENDING, "the hit waits for the target's reaction")
	case pendingAwaitingRoll:
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_DAMAGE_NOT_ROLLED, "the damage was not rolled yet")
	case pendingApplied, pendingDiscarded:
		return resolvedDamageError()
	}
	return nil
}

// settleAmount works out what lands: what the target's resistance, vulnerability or immunity leaves of the
// roll, unless the master leaves a source out or sets another number. It starts the event.
func (f *applyDamageFlow) settleAmount(context.Context) error {
	rolled := num(f.p.Amount)
	groups := readStepGroups(f.p.Steps)
	expected := rolled
	if len(groups) > 0 {
		expected = reapply(rolled, groups, f.call.ignored)
	}
	amount := expected
	if f.call.override != nil {
		amount = *f.call.override
	}
	f.amount, f.applied = amount, nil
	if amount != rolled {
		f.applied = &amount
	}
	f.made = actionEvent{
		Round: f.c.enc.Round, Secret: secretOf(f.attacker, f.target), Actor: f.attacker.ID, Target: f.target.ID, Pending: f.p.ID, Key: f.p.AttackKey,
		Amount: amount, DamageType: f.p.DamageType, Overridden: f.call.override != nil && *f.call.override != expected, Rolled: rolled,
		Steps: groups, Ignored: f.call.ignored,
	}
	return nil
}

// readVitals reads the target's vitals, which the landing overwrites.
func (f *applyDamageFlow) readVitals(ctx context.Context) error {
	now, err := f.s.vitals.GetVitalsTx(ctx, f.c.tx, f.c.session.CampaignID, f.target.CharacterID)
	if err != nil {
		return err
	}
	f.now, f.down = now, isDownIn(now)
	return nil
}

// land puts the amount on the target: a character at 0 hit points fails death saves, a druid in a beast form
// has the beast take it, and any other character takes it through its vitals.
func (f *applyDamageFlow) land(ctx context.Context) error {
	switch {
	case f.down:
		return f.landDown(ctx)
	case f.now.GetWildShape() != nil:
		return f.landBeast(ctx)
	}
	return f.landNormal(ctx)
}

// landDown is RN-03: a damage at 0 hit points is a death save failure (two for a critical hit), and the hit
// points stay at 0.
func (f *applyDamageFlow) landDown(ctx context.Context) error {
	before, after, added, err := f.s.failuresWhileDown(ctx, f.c, f.target, f.p.Critical, f.amount)
	if err != nil {
		return err
	}
	f.made.DeathBefore, f.made.Death, f.made.FailuresAdded, f.made.DeathHidden = before, after, added, f.c.rules.DeathSavesHidden
	f.made.Before = new(hpStateOf(f.now))
	f.made.After = f.made.Before
	return nil
}

// landBeast is a druid in a beast form: the beast takes it, and what is left over when the beast falls goes
// to the druid (MR-037, SRD).
func (f *applyDamageFlow) landBeast(ctx context.Context) error {
	before, after, err := f.s.damageBeast(ctx, f.c, f.target, f.now, f.amount, &f.made)
	if err != nil {
		return err
	}
	f.out.vitals = after
	f.made.Before, f.made.After = new(hpStateOf(before)), new(hpStateOf(after))
	f.made.ConcentrationDC = concentrationDC(f.target, f.amount)
	f.made.DeathBefore = deathOf(f.target)
	// The 0 hit points rule, as below: the damage that carried over dropped the druid to 0 on an
	// opportunity attack.
	return f.returnIfDropped(ctx)
}

// landNormal is RN-02: the damage goes through the character's vitals, temporary hit points first, never
// below 0. At 0 it is down ("Caído") and makes death saves.
func (f *applyDamageFlow) landNormal(ctx context.Context) error {
	dmg := combat.ApplyDamage(int(f.now.GetHitPointsCurrent()), int(f.now.GetHitPointsTemporary()), int(f.amount))
	if relentlessEndurance(f.now, dmg) { // the half-orc stays on its feet with 1 hit point (SRD 5.1)
		if _, err := f.s.spendResource(ctx, f.c, f.target.CharacterID, resRelentlessEndurance, 1); err != nil {
			return err
		}
		dmg.HP, dmg.FellToZero, f.made.Relentless = 1, false, true
	}
	hp, temp := clamp32(dmg.HP, 0, math.MaxInt32), clamp32(dmg.TempHP, 0, math.MaxInt32)
	before, after, err := f.s.vitalsOf(ctx, f.c, f.target.CharacterID, &playv1.AdjustCharacterVitalsRequest{HitPointsCurrent: &hp, HitPointsTemporary: &temp})
	if err != nil {
		return err
	}
	f.out.vitals = after
	f.made.Before = new(hpStateOf(before))
	f.made.After = new(hpStateOf(after))
	f.made.ConcentrationDC = concentrationDC(f.target, f.amount-clamp32(dmg.Absorbed, 0, math.MaxInt32))
	f.made.DeathBefore = deathOf(f.target) // the undo puts the turn's save back too
	// The 0 hit points rule: an opportunity attack that drops its mover to 0 takes it back to where it left
	// the reach.
	return f.returnIfDropped(ctx)
}

// returnIfDropped takes the mover of an opportunity attack back to where it left the reach, when the damage
// dropped it to 0 hit points.
func (f *applyDamageFlow) returnIfDropped(ctx context.Context) error {
	if f.made.Before.HP > 0 && f.made.After.HP == 0 {
		var err error
		f.made.ReturnedFrom, f.made.ReturnBlocked, err = f.s.returnToReach(ctx, f.c, f.p.ID, f.target)
		return err
	}
	return nil
}

// afterTaken runs what the damage taken does to the target's states.
func (f *applyDamageFlow) afterTaken(ctx context.Context) error {
	return f.s.afterDamageTaken(ctx, f.c, f.target, f.amount, f.made.Before, f.made.After)
}

// record marks the pending damage applied, with the amount the master set when it is not the roll.
func (f *applyDamageFlow) record(ctx context.Context) error {
	if _, err := f.c.q.SetPendingDamageApplied(ctx, playdb.SetPendingDamageAppliedParams{ID: f.p.ID, ResolvedAt: &f.c.now, AppliedAmount: f.applied}); err != nil {
		return fmt.Errorf("apply the pending damage: %w", err)
	}
	return nil
}

// castSave works out the concentration save of a spell's damage: a spell asks one concentration save of a
// target, not one per damage type.
func (f *applyDamageFlow) castSave(ctx context.Context) error {
	if f.p.CastID == nil {
		return nil
	}
	var taken int32
	if !f.down {
		taken = takenBy(f.amount, f.made.Before, f.made.After)
	}
	var err error
	f.made.ConcentrationDC, err = f.s.castConcentrationDC(ctx, f.c, f.p, f.target, taken)
	return err
}

// afterHit runs what the landed hit does to the combat: the reactions and the concentration it asks about.
func (f *applyDamageFlow) afterHit(ctx context.Context) error {
	return f.s.afterDamage(ctx, f.c, f.target, damageLanded{attacker: f.attacker.ID, key: f.p.AttackKey, pending: f.p, hit: damageHit{
		Pending: f.p.ID, Target: f.target.ID, Amount: f.amount, Applied: true, Before: f.made.Before, After: f.made.After, ConcentrationDC: f.made.ConcentrationDC,
	}})
}

// touch moves the encounter's revision and names the character the change was about.
func (f *applyDamageFlow) touch(ctx context.Context) error {
	if err := touchEncounter(ctx, f.c); err != nil {
		return err
	}
	f.c.characterID = &f.target.CharacterID
	return nil
}

// discardDamageCall is what DiscardPendingDamage reads from its request, checked.
type discardDamageCall struct {
	key, encounterID, pendingID string
}

// readDiscardDamageCall reads the request, in the order the answers are given: the ids.
func readDiscardDamageCall(req *playv1.DiscardPendingDamageRequest) (discardDamageCall, error) {
	var call discardDamageCall
	var err error
	if call.key, err = parseKey(req.GetIdempotencyKey()); err != nil {
		return call, err
	}
	if call.encounterID, err = parseCombatID(req.GetEncounterId(), "encounter"); err != nil {
		return call, err
	}
	call.pendingID, err = parseCombatID(req.GetPendingDamageId(), "pending damage")
	return call, err
}

// discardDamageOut is what the transaction of a discard leaves for its answer.
type discardDamageOut struct {
	made actionEvent
}

// DiscardPendingDamage implements playv1connect.CombatServiceHandler.
func (s *Service) DiscardPendingDamage(
	ctx context.Context,
	req *connect.Request[playv1.DiscardPendingDamageRequest],
) (*connect.Response[playv1.DiscardPendingDamageResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	call, err := readDiscardDamageCall(req.Msg)
	if err != nil {
		return nil, err
	}
	out := &discardDamageOut{}
	res, err := s.write(ctx, combatWrite{m: m, key: call.key, hash: idem.Hash(req.Msg), kind: eventDamageDiscarded, encounterID: call.encounterID}, func(c *combatTx) (any, error) {
		return newDiscardDamageFlow(s, c, call, out).run(ctx)
	})
	if err != nil {
		return nil, s.dbError(ctx, "discard a pending damage", err)
	}
	return s.discardDamageResponse(ctx, m, res, out)
}

// discardDamageResponse answers from what the transaction left: the encounter as the master sees it and the
// pending damage that was dropped.
func (s *Service) discardDamageResponse(ctx context.Context, m authz.Membership, res combatResult, out *discardDamageOut) (*connect.Response[playv1.DiscardPendingDamageResponse], error) {
	ev, err := resultEvent(res, out.made)
	if err != nil {
		return nil, s.dbError(ctx, "read the discarded damage", err)
	}
	enc, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, !ev.Secret)
	})
	if err != nil {
		return nil, err
	}
	pending, err := s.pendingFor(ctx, res, ev.Pending, combatViewer{master: true})
	if err != nil {
		return nil, err
	}
	decorate(pending, ev, combatViewer{master: true}, s.membersOf(ctx, res))
	return connect.NewResponse(&playv1.DiscardPendingDamageResponse{Encounter: enc, PendingDamage: pending}), nil
}

// discardDamageFlow is one run of the discard's transaction: what each phase reads, and the next one uses.
type discardDamageFlow struct {
	s    *Service
	c    *combatTx
	call discardDamageCall
	out  *discardDamageOut

	p                playdb.PendingDamage
	attacker, target playdb.Combatant
	// windows are the reaction windows that waited on the hit, closed with it.
	windows []string
	made    actionEvent
}

func newDiscardDamageFlow(s *Service, c *combatTx, call discardDamageCall, out *discardDamageOut) *discardDamageFlow {
	return &discardDamageFlow{s: s, c: c, call: call, out: out}
}

// run is the transaction: the checks, the drop and its event. It answers the event the write stores.
func (f *discardDamageFlow) run(ctx context.Context) (any, error) {
	phases := []func(context.Context) error{f.locate, f.drop, f.touch, f.describe, f.castSave}
	for _, phase := range phases {
		if err := phase(ctx); err != nil {
			return nil, err
		}
	}
	f.out.made = f.made
	return f.made, nil
}

// locate checks the combat and the hit the master discards, and finds its two combatants.
func (f *discardDamageFlow) locate(ctx context.Context) error {
	if err := notEnded(f.c.enc); err != nil {
		return err
	}
	var err error
	if f.p, f.attacker, f.target, err = f.s.settling(ctx, f.c, f.call.pendingID); err != nil {
		return err
	}
	if f.p.Status == pendingApplied || f.p.Status == pendingDiscarded {
		return resolvedDamageError()
	}
	return nil
}

// drop discards the pending damage and the windows that waited on it.
func (f *discardDamageFlow) drop(ctx context.Context) error {
	var err error
	f.windows, err = discardHit(ctx, f.c, f.p)
	return err
}

// touch moves the encounter's revision and names the character the change was about.
func (f *discardDamageFlow) touch(ctx context.Context) error {
	if err := touchEncounter(ctx, f.c); err != nil {
		return err
	}
	f.c.characterID = &f.attacker.CharacterID
	return nil
}

// describe starts the event of the discard.
func (f *discardDamageFlow) describe(context.Context) error {
	f.made = actionEvent{
		Round: f.c.enc.Round, Secret: secretOf(f.attacker, f.target), Actor: f.attacker.ID, Target: f.target.ID, Pending: f.p.ID, Key: f.p.AttackKey,
		Amount: num(f.p.Amount), PrevStatus: f.p.Status, ClosedWindows: f.windows,
	}
	return nil
}

// castSave works out the concentration save a spell's damage still owes: the discard may settle the cast's
// last damage for the target, and what landed before it owes the save.
func (f *discardDamageFlow) castSave(ctx context.Context) error {
	if f.p.CastID == nil || f.p.Healing {
		return nil
	}
	var err error
	f.made.ConcentrationDC, err = f.s.castConcentrationDC(ctx, f.c, f.p, f.target, 0)
	return err
}
