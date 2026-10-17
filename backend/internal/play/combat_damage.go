package play

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"slices"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dice"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// The damage roll (RollDamage), in phases. rollDamage reads the request, runs the transaction as a sequence
// of phases of a damageFlow, each one of the checks or the writes the damage has always made in that order,
// and answers from what the transaction left in a damageOut. The phases keep the order of the reads, the
// dice and the writes: the scripted dice of the tests and the golden of combat_roll_damage_frozen_test.go
// read it.

// damageCall is what RollDamage reads from its request, checked.
type damageCall struct {
	key, encounterID, pendingID string
	in                          rollInput
	typedParts                  map[string]int
}

// readDamageCall reads the request, in the order the answers are given: the ids, the roll, the typed parts.
func readDamageCall(req *playv1.RollDamageRequest) (damageCall, error) {
	var call damageCall
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
	if call.in, err = readDamageRoll(req); err != nil {
		return call, err
	}
	call.typedParts, err = typedPartsOf(req.GetTypedParts())
	return call, err
}

// readDamageRoll reads how the dice come: rolled in the app, one typed sum, or the typed sums of the parts.
func readDamageRoll(req *playv1.RollDamageRequest) (rollInput, error) {
	var in rollInput
	switch roll := req.GetRoll().(type) {
	case *playv1.RollDamageRequest_RollInApp:
		if !roll.RollInApp {
			return in, connect.NewError(connect.CodeInvalidArgument, errors.New("roll_in_app must be true"))
		}
		in.inApp = true
	case *playv1.RollDamageRequest_TypedSum:
		in.typed = int(roll.TypedSum)
	default:
		if len(req.GetTypedParts()) == 0 {
			return in, connect.NewError(connect.CodeInvalidArgument, errors.New("set roll_in_app, typed_sum or typed_parts"))
		}
	}
	return in, nil
}

// damageOut is what the transaction of a damage roll leaves for its answer.
type damageOut struct {
	made actionEvent
	// vitals are the characters a heal or a death save touched.
	vitals []*playv1.CharacterVitals
	v      combatViewer
}

// RollDamage implements playv1connect.CombatServiceHandler.
func (s *Service) RollDamage(
	ctx context.Context,
	req *connect.Request[playv1.RollDamageRequest],
) (*connect.Response[playv1.RollDamageResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	return s.rollDamage(ctx, m, req)
}

// rollDamage is RollDamage for a member already authorized: the replay of a held action calls it
// with the member who made the request (combat_reaction_hold.go).
func (s *Service) rollDamage(ctx context.Context, m authz.Membership, req *connect.Request[playv1.RollDamageRequest]) (*connect.Response[playv1.RollDamageResponse], error) {
	call, err := readDamageCall(req.Msg)
	if err != nil {
		return nil, err
	}
	out := &damageOut{v: viewerOf(m)}
	res, err := s.write(ctx, combatWrite{m: m, key: call.key, hash: idem.Hash(req.Msg), kind: eventDamageRolled, encounterID: call.encounterID}, func(c *combatTx) (any, error) {
		return newDamageFlow(s, c, m, req.Msg, call, out).run(ctx)
	})
	if err != nil {
		return nil, s.dbError(ctx, "roll damage", err)
	}
	return s.damageResponse(ctx, m, res, out)
}

// damageFlow is one run of the damage's transaction: what each phase reads, and the next one uses.
type damageFlow struct {
	s    *Service
	c    *combatTx
	m    authz.Membership
	req  *playv1.RollDamageRequest
	call damageCall
	out  *damageOut

	cs               []playdb.Combatant
	p                playdb.PendingDamage
	attacker, target playdb.Combatant
	// group are the pending damages this roll settles: the pending damage itself, or the ones of its cast
	// still to be rolled.
	group  []playdb.PendingDamage
	expr   dice.Expr
	states map[string][]playdb.CombatantState
	pr     *partsRoll
	roll   dice.Result
	// total is the damage never below 0; faces and rolledTotal are what the pending damages keep.
	total       int
	faces       []int32
	rolledTotal int32
}

func newDamageFlow(s *Service, c *combatTx, m authz.Membership, req *playv1.RollDamageRequest, call damageCall, out *damageOut) *damageFlow {
	return &damageFlow{s: s, c: c, m: m, req: req, call: call, out: out}
}

// run is the transaction: the checks, the dice, what they did to the targets. It answers the event the write
// stores, which for a roll that waits for a reaction is the event of the hold.
func (f *damageFlow) run(ctx context.Context) (any, error) {
	f.out.vitals = nil
	if err := f.prepare(ctx); err != nil {
		return nil, err
	}
	if err := f.rollDice(ctx); err != nil {
		return nil, err
	}
	held, err := f.hold(ctx)
	if err != nil {
		return nil, err
	}
	if held {
		return f.out.made, nil
	}
	f.adjustRoll()
	f.startEvent()
	if err := f.settlePartsOfRoll(ctx); err != nil {
		return nil, err
	}
	for _, g := range f.group {
		if err := f.land(ctx, g); err != nil {
			return nil, err
		}
	}
	if err = touchEncounter(ctx, f.c); err != nil {
		return nil, err
	}
	f.c.characterID = &f.attacker.CharacterID
	return f.out.made, nil
}

// prepare checks that the damage may be rolled and reads what the roll needs: the pending damage, the
// combatants, the cast's damages it settles and the states of the combat.
func (f *damageFlow) prepare(ctx context.Context) error {
	if err := notEnded(f.c.enc); err != nil {
		return err
	}
	if err := f.s.reactionGate(ctx, f.c); err != nil { // a reaction window holds the damage too (PM-04)
		return err
	}
	if err := f.findPending(ctx); err != nil {
		return err
	}
	if err := f.findParties(ctx); err != nil {
		return err
	}
	if err := f.checkStatus(); err != nil {
		return err
	}
	if err := f.openGroup(ctx); err != nil {
		return err
	}
	// The damage: the dice (doubled on a critical hit under the SRD's rule, as the pending damage kept them),
	// plus the modifier once, plus what the table's critical rule adds without rolling (the dice's maximum,
	// in "máximo mais uma rolagem"; typed_sum never includes it). A flat damage needs no dice (and so no way
	// of rolling). The extra dice of a feature (Brutal Critical) are always rolled, after the critical's own,
	// and typed_sum counts them.
	p := f.p
	f.expr = dice.Expr{Count: int(p.DiceCount) + int(p.ExtraDice), Sides: int(p.DiceSides), Modifier: int(p.DiceBonus) + int(p.CriticalMax)}
	var err error
	f.states, err = f.s.readStates(ctx, f.c.tx, f.c.enc.ID)
	return err
}

// findPending reads the pending damage.
func (f *damageFlow) findPending(ctx context.Context) error {
	p, err := f.c.q.GetPendingDamage(ctx, playdb.GetPendingDamageParams{EncounterID: f.c.enc.ID, ID: f.call.pendingID})
	if errors.Is(err, pgx.ErrNoRows) {
		return connect.NewError(connect.CodeNotFound, errors.New("pending damage not found"))
	}
	if err != nil {
		return fmt.Errorf("find the pending damage: %w", err)
	}
	f.p = p
	return nil
}

// findParties reads the combatants and finds the attacker and the target for the caller: the fog hides an NPC
// the player does not see.
func (f *damageFlow) findParties(ctx context.Context) error {
	cs, err := f.c.q.ListCombatants(ctx, f.c.enc.ID)
	if err != nil {
		return fmt.Errorf("list the combatants: %w", err)
	}
	f.cs = cs
	v := f.c.viewer(f.m, cs) // the fog: an NPC the player does not see is not found
	f.out.v = v
	if f.attacker, err = findCombatant(cs, deref(f.p.AttackerID), v); err != nil {
		return err
	}
	if err := v.mayAct(f.attacker); err != nil {
		return err
	}
	f.target, _ = findCombatant(cs, f.p.TargetID, combatViewer{master: true})
	if !v.sees(f.target) {
		return errCombatantNotFound() // a target hidden after the hit: the master resolves it
	}
	return nil
}

// checkStatus refuses a pending damage that is not waiting for its roll.
func (f *damageFlow) checkStatus() error {
	switch f.p.Status {
	case pendingAwaitingReaction:
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_REACTION_PENDING, "the hit waits for the target's reaction")
	case pendingRolled:
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_DAMAGE_ALREADY_ROLLED, "the damage was rolled already")
	case pendingApplied, pendingDiscarded:
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_DAMAGE_RESOLVED, "the damage was applied or discarded already")
	}
	return nil
}

// openGroup finds the pending damages this roll settles. An area spell's damage is one roll for the whole
// cast: the pending damages of the cast still to be rolled are settled by this roll.
func (f *damageFlow) openGroup(ctx context.Context) error {
	f.group = []playdb.PendingDamage{f.p}
	if f.p.CastID == nil {
		return nil
	}
	p := f.p
	all, err := f.c.q.ListCastPendingDamages(ctx, playdb.ListCastPendingDamagesParams{EncounterID: f.c.enc.ID, CastID: p.CastID})
	if err != nil {
		return fmt.Errorf("list the cast's pending damage: %w", err)
	}
	// A spell with two damage types opens one roll for each: only the ones of this damage type settle
	// together.
	f.group = slices.DeleteFunc(all, func(o playdb.PendingDamage) bool {
		return o.Status != pendingAwaitingRoll || o.DamageType != p.DamageType || o.Healing != p.Healing
	})
	// The pending damages of a cast share their creation time, so the query leaves their order to their
	// random ids: settle them in the combat's turn order.
	cs := f.cs
	slices.SortStableFunc(f.group, func(x, y playdb.PendingDamage) int {
		return cmp.Compare(combatantIndex(cs, x.TargetID), combatantIndex(cs, y.TargetID))
	})
	return nil
}

// combatantIndex is where a combatant stands in the turn order of cs, -1 when it is not there.
func combatantIndex(cs []playdb.Combatant, id string) int {
	return slices.IndexFunc(cs, func(c playdb.Combatant) bool { return c.ID == id })
}

// rollDice makes the roll: the parts of a hit with extras, or the one group of dice, then what the roll made
// when a reaction window held the damage.
func (f *damageFlow) rollDice(ctx context.Context) error {
	if err := f.rollParts(ctx); err != nil {
		return err
	}
	switch {
	case f.pr != nil:
		f.roll = f.pr.legacy()
	case f.expr.Count == 0:
		f.roll = dice.Result{Expr: f.expr, Modifier: f.expr.Modifier, Total: f.expr.Modifier}
	default:
		if err := f.rollGroup(ctx); err != nil {
			return err
		}
	}
	if f.c.replay != nil && f.c.replay.rolled != nil { // the roll made when a reaction window held the damage
		f.roll = replayedRoll(f.expr, f.c.replay.rolled)
	}
	return nil
}

// rollParts rolls the parts of a hit with parts (the weapon and its extras), each of them.
func (f *damageFlow) rollParts(ctx context.Context) error {
	if len(readParts(f.p.Parts)) == 0 {
		return nil
	}
	if !f.out.v.master {
		if err := f.s.mustRollThisWay(ctx, f.c.tx, f.m, f.call.in); err != nil {
			return err
		}
	}
	if f.c.replay != nil && f.c.replay.partRolls != nil { // the dice a reaction window held: they land as rolled
		f.pr = heldPartsRoll(f.c.replay.parts, f.c.replay.partRolls, !f.call.in.inApp)
		return nil
	}
	var err error
	f.pr, err = f.s.rollPendingParts(ctx, f.c, f.m.CampaignID, f.p, f.attacker, f.target, f.call.in, f.call.typedParts, choiceOf(f.req))
	return err
}

// rollGroup rolls the one group of dice of a damage with no parts, in the app or from the typed sum.
func (f *damageFlow) rollGroup(ctx context.Context) error {
	if !f.out.v.master {
		if err := f.s.mustRollThisWay(ctx, f.c.tx, f.m, f.call.in); err != nil {
			return err
		}
	}
	var err error
	if f.call.in.inApp {
		if f.roll, err = dice.Roll(f.s.roller, f.expr); err != nil {
			return fmt.Errorf("roll the damage: %w", err)
		}
		return nil
	}
	if f.roll, err = dice.Physical(f.expr, f.call.in.typed); err != nil {
		return connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("typed_sum must be %d to %d", f.expr.Count, f.expr.Count*f.expr.Sides))
	}
	return nil
}

// hold holds the roll for the reactions that can change the damage (Uncanny Dodge, Deflect Missiles,
// Cutting Words): it waits for the windows, and lands when they are answered (PM-04). It says whether it did.
func (f *damageFlow) hold(ctx context.Context) (bool, error) {
	held, ok, err := f.s.holdDamage(ctx, f.c, f.m, f.req, f.cs, f.p, f.attacker, f.target, f.roll, f.pr)
	if err != nil || !ok {
		return false, err
	}
	f.out.made = held
	return true, nil
}

// adjustRoll applies what the answers of the held windows did to the damage, and works out the numbers the
// pending damages keep.
func (f *damageFlow) adjustRoll() {
	if f.c.replay != nil {
		before := f.roll.Total
		f.roll.Total = adjustedDamage(f.c.replay, f.roll.Total)
		if f.pr != nil { // what the answers took off the damage comes off its types and what a player reads
			f.pr.scale(before, f.roll.Total, f.c.replay)
		}
	}
	f.total = max(f.roll.Total, 0) // damage is never below 0
	f.faces = faces32(f.roll.Faces)
	f.rolledTotal = clamp32(f.total, 0, math.MaxInt32)
}

// startEvent makes the event of the roll, before the targets change it.
func (f *damageFlow) startEvent() {
	p := f.p
	f.out.made = actionEvent{
		Round: f.c.enc.Round, Secret: secretOf(f.attacker, f.target), Actor: f.attacker.ID, Target: p.TargetID, Pending: p.ID, Key: p.AttackKey,
		DiceCount: p.DiceCount + p.ExtraDice, ExtraDice: p.ExtraDice, SavageDice: p.SavageDice, DiceSides: p.DiceSides, Modifier: clamp32(int(p.DiceBonus)+int(p.CriticalMax), math.MinInt32, math.MaxInt32), CriticalMax: p.CriticalMax, CriticalMaxRule: p.CriticalMaxRule, Faces: f.faces,
		DamageType: p.DamageType, Critical: p.Critical, Physical: f.roll.Physical, Heal: p.Healing, Total: f.rolledTotal,
	}
}

// settlePartsOfRoll does what a roll with parts leaves besides the damage: the slot a smite spent, the
// resources, the once-per-turn marks.
func (f *damageFlow) settlePartsOfRoll(ctx context.Context) error {
	if f.pr == nil {
		return nil
	}
	vit, err := f.s.settleParts(ctx, f.c, f.attacker, f.pr, &f.out.made)
	if err != nil {
		return err
	}
	if vit != nil {
		f.out.vitals = append(f.out.vitals, vit)
	}
	return nil
}

// land puts the roll on a target of the group: the damage as the target takes it, the pending damage's new
// state, what follows the damage and the event's numbers.
func (f *damageFlow) land(ctx context.Context, g playdb.PendingDamage) error {
	tgt, _ := findCombatant(f.cs, g.TargetID, combatViewer{master: true})
	// A hidden creature in the one roll of a cast that settles several makes the roll the master's alone,
	// unless the cast lists its targets for each viewer: an area's line leaves out the hidden ones itself
	// (spellView).
	f.out.made.Secret = f.out.made.Secret || (tgt.Hidden && f.p.CastID == nil)
	amount := clamp32(f.total, 0, math.MaxInt32)
	if g.Half {
		amount = clamp32(combat.HalfDamage(f.total), 0, math.MaxInt32)
	}
	own := f.pr != nil && g.ID == f.p.ID
	// What a player who is not the target's reads is the damage as rolled, before the target's modifiers and
	// counting every die the roll made (a conditional extra included): nothing in it says what the target
	// resists or is (RN-10, RN-20).
	shown := amount
	if own {
		shown = clamp32(f.pr.shownTotal(), 0, math.MaxInt32)
	}
	steps, amount, err := f.settleSteps(ctx, g, tgt, amount, own)
	if err != nil {
		return err
	}
	hit, err := f.landHit(ctx, g, tgt, amount, shown)
	if err != nil {
		return err
	}
	if err := f.keepRoll(ctx, g, tgt, amount, steps, &hit); err != nil {
		return err
	}
	if err := f.s.afterDamage(ctx, f.c, tgt, damageLanded{attacker: f.attacker.ID, key: g.AttackKey, pending: g, hit: hit}); err != nil {
		return err
	}
	if g.ID == f.p.ID {
		if err := f.fillEvent(ctx, g, tgt, hit, steps, shown); err != nil {
			return err
		}
	}
	if f.p.CastID != nil || f.p.Healing {
		f.out.made.Settled = append(f.out.made.Settled, hit)
	}
	return nil
}

// settleSteps takes the target's resistance, vulnerability and immunity off the damage of a type, after every
// bonus and the half of a save (SRD 5.1). An NPC or a creature takes what is left at once; a player's
// character waits for the master with the same account as a preview. It answers the steps and the amount.
func (f *damageFlow) settleSteps(ctx context.Context, g playdb.PendingDamage, tgt playdb.Combatant, amount int32, own bool) (landing, int32, error) {
	var steps landing
	if g.Healing {
		return steps, amount, nil
	}
	byType := map[string]int{g.DamageType: int(amount)}
	if own {
		byType = f.pr.byType
	}
	steps, err := f.s.settleLanding(ctx, f.c, f.states, tgt, byType)
	if err != nil {
		return steps, amount, err
	}
	if holdsHP(tgt) && steps.changed {
		amount = steps.amount
	}
	return steps, amount, nil
}

// landHit puts the damage or the heal on the target and notes the vitals it touched.
func (f *damageFlow) landHit(ctx context.Context, g playdb.PendingDamage, tgt playdb.Combatant, amount, shown int32) (damageHit, error) {
	hit, vit, err := f.s.landDamage(ctx, f.c, g, tgt, amount)
	if err != nil {
		return hit, err
	}
	hit.Shown = shown
	if vit != nil {
		f.out.vitals = append(f.out.vitals, vit)
	}
	return hit, nil
}

// keepRoll saves the roll on the pending damage, with its detail, and notes the concentration save a cast
// owes its target.
func (f *damageFlow) keepRoll(ctx context.Context, g playdb.PendingDamage, tgt playdb.Combatant, amount int32, steps landing, hit *damageHit) error {
	c := f.c
	status := pendingRolled // a player's character waits for the master (RN-02)
	if hit.Applied {
		status = pendingApplied
	}
	if _, err := c.q.SetPendingDamageRolled(ctx, playdb.SetPendingDamageRolledParams{
		ID: g.ID, Status: status, Faces: f.faces, Physical: f.roll.Physical, Amount: &amount, ResolvedAt: resolvedAt(status, c),
		RollTotal: &f.rolledTotal,
	}); err != nil {
		return fmt.Errorf("save the damage roll: %w", err)
	}
	if err := f.s.keepRollDetail(ctx, c, g, f.p, f.pr, steps, *hit); err != nil {
		return err
	}
	if f.p.CastID == nil || !hit.Applied || g.Healing {
		return nil
	}
	// A spell asks one concentration save of a target, not one per damage type.
	var err error
	hit.ConcentrationDC, err = f.s.castConcentrationDC(ctx, c, g, tgt, takenBy(hit.Amount, hit.Before, hit.After))
	return err
}

// fillEvent puts what the roll did to its own target in the event.
func (f *damageFlow) fillEvent(ctx context.Context, g playdb.PendingDamage, tgt playdb.Combatant, hit damageHit, steps landing, shown int32) error {
	made := &f.out.made
	made.Amount, made.Applied, made.Before, made.After, made.ConcentrationDC = hit.Amount, hit.Applied, hit.Before, hit.After, hit.ConcentrationDC
	made.Steps, made.Shown = steps.groups, shown
	made.DeathBefore = hit.DeathBefore
	// The 0 hit points rule: an opportunity attack that drops its mover to 0 takes it back to where it left
	// the reach.
	if !hit.Applied || g.Healing || hit.Before == nil || hit.After == nil || hit.Before.HP <= 0 || hit.After.HP != 0 {
		return nil
	}
	var err error
	made.ReturnedFrom, made.ReturnBlocked, err = f.s.returnToReach(ctx, f.c, g.ID, tgt)
	return err
}

// damageResponse answers the roll: the combat, and the pending damages it settled as the caller may read them.
func (s *Service) damageResponse(ctx context.Context, m authz.Membership, res combatResult, out *damageOut) (*connect.Response[playv1.RollDamageResponse], error) {
	v, err := s.viewerAfter(ctx, m, res, out.v) // a replay never ran the closure: the fog filter still holds
	if err != nil {
		return nil, s.dbError(ctx, "work out what the player sees", err)
	}
	ev, err := resultEvent(res, out.made)
	if err != nil {
		return nil, s.dbError(ctx, "read the damage", err)
	}
	enc, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
		s.publishReturned(ctx, m.CampaignID, d, ev)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, !ev.Secret)
		for _, vit := range out.vitals {
			s.publishVitals(m.CampaignID, vit)
		}
	})
	if err != nil {
		return nil, err
	}
	resp := &playv1.RollDamageResponse{Encounter: enc}
	if ev.Reaction != nil && ev.Reaction.Hold { // a reaction window holds the damage: it is not rolled yet
		return connect.NewResponse(resp), nil
	}
	if resp.PendingDamage, err = s.pendingFor(ctx, res, ev.Pending, v); err != nil {
		return nil, err
	}
	decorate(resp.PendingDamage, ev, v, s.membersOf(ctx, res))
	if resp.CastPendingDamages, err = s.settledPendings(ctx, res, ev, v); err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// settledPendings are the other pending damages the roll settled, as the caller may read them.
func (s *Service) settledPendings(ctx context.Context, res combatResult, ev actionEvent, v combatViewer) ([]*playv1.PendingDamage, error) {
	var out []*playv1.PendingDamage
	for _, h := range ev.Settled {
		if h.Pending == ev.Pending {
			continue
		}
		other, err := s.pendingFor(ctx, res, h.Pending, v)
		if err != nil {
			return nil, err
		}
		if other != nil {
			decorate(other, ev, v, s.membersOf(ctx, res))
			out = append(out, other)
		}
	}
	return out, nil
}
