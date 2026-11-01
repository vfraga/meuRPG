package play

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
	"github.com/PuraFome/meuRPG/backend/internal/rules/reaction"
)

// The concentration save (SRD 5.1, Duration): a creature that takes damage while it
// concentrates makes a Constitution saving throw, DC 10 or half the damage, and
// loses the concentration on a failure; damage from several sources is a save for
// each, and one attack or one spell is one source. At 0 hit points the concentration
// ends with no save. A save is a reaction window of its own kind: the damage that
// opened it stays applied, and the turn waits for the owner's answer (the app's die,
// a typed d20 or "Deixar o mestre rolar por mim") or the master's.

// concentrationEnd marks the event of a concentration that ended at 0 hit points, which
// the undo of the damage takes back with it.
const concentrationEnd = "concentration_end"

// damageLanded is what a damage did to its target, for what follows it.
type damageLanded struct {
	attacker string // the combatant that dealt it, empty for a trap
	key      string // the attack or spell
	pending  playdb.PendingDamage
	hit      damageHit
}

// afterDamage opens what a landed damage asks of its target: the end of a
// concentration at 0 hit points, or its saving throw. It runs in the transaction that
// applied the damage, once for each source of damage (a cast is one source).
func (s *Service) afterDamage(ctx context.Context, c *combatTx, target playdb.Combatant, d damageLanded) error {
	if d.pending.Healing || !d.hit.Applied || isCreature(target) {
		return nil
	}
	if err := s.offerCreatureRider(ctx, c, target, d); err != nil { // the rider of an NPC's attack that hit (combat_creature_rider.go)
		return err
	}
	if err := s.hellishWindow(ctx, c, target, d); err != nil {
		return err
	}
	if err := s.effectDamageSaves(ctx, c, target, d); err != nil {
		return err
	}
	if target.ConcentrationSpell == nil {
		return nil
	}
	return s.concentrationAfterDamage(ctx, c, target, d)
}

// concentrationAfterDamage is what a damage that landed on a concentrating target asks: the
// concentration ends at 0 hit points, or the save opens when the damage threatens it.
func (s *Service) concentrationAfterDamage(ctx context.Context, c *combatTx, target playdb.Combatant, d damageLanded) error {
	if after := d.hit.After; after != nil && after.HP == 0 {
		return s.endConcentrationAtZero(ctx, c, target, d.pending.ID)
	}
	if d.hit.ConcentrationDC <= 0 {
		return nil
	}
	taken, err := s.concentrationDamageTaken(ctx, c, target, d)
	if err != nil {
		return err
	}
	trigger := windowTrigger{Actor: d.attacker, Target: target.ID, Key: d.key, Spell: *target.ConcentrationSpell, Taken: taken, Pending: d.pending.ID}
	_, err = s.openWindows(ctx, c, newGroup(), nil, []windowSpec{{kind: reaction.Concentration, reactor: &target, trigger: trigger}})
	return err
}

// endConcentrationAtZero ends the concentration of a target the damage took to 0 hit points: it
// is incapacitated and loses the concentration with no save (SRD, Duration). group names the change
// that took it there (the pending damage, or the combatant the master adjusted), so that the undo of
// that change finds this event and takes it back with it; a change with no undo passes "".
func (s *Service) endConcentrationAtZero(ctx context.Context, c *combatTx, target playdb.Combatant, group string) error {
	ev := actionEvent{Round: c.enc.Round, Secret: target.Hidden, Actor: target.ID, Reaction: &reactionEvent{Kind: concentrationEnd, Group: group}}
	if err := s.stopConcentrating(ctx, c, target, &ev); err != nil {
		return err
	}
	return insertEvent(ctx, c, eventConditionsSet, &c.actorUserID, nil, ev)
}

// concentrationDamageTaken is the damage the save's DC comes from: what this damage cost the
// target, or, for the damage of a cast, the sum of what every damage of the cast cost it.
func (s *Service) concentrationDamageTaken(ctx context.Context, c *combatTx, target playdb.Combatant, d damageLanded) (int32, error) {
	taken := takenBy(d.hit.Amount, d.hit.Before, d.hit.After)
	if d.pending.CastID == nil {
		return taken, nil
	}
	all, err := c.q.ListCastPendingDamages(ctx, playdb.ListCastPendingDamagesParams{EncounterID: c.enc.ID, CastID: d.pending.CastID})
	if err != nil {
		return 0, fmt.Errorf("list the cast's pending damage: %w", err)
	}
	taken = 0
	for _, o := range all {
		if o.TargetID == target.ID && !o.Healing && o.Status == pendingApplied {
			taken += conv.OrZero(o.Taken)
		}
	}
	return taken, nil
}

// concentrationRollOf reads how a concentration save is answered: the app's die, a
// physical one, "leave it to the master" or the master's "keep".
func concentrationRollOf(msg *playv1.ResolveConcentrationSaveRequest) (in rollInput, handOver, keep bool, err error) {
	bad := func(text string) error { return connect.NewError(connect.CodeInvalidArgument, errors.New(text)) }
	switch roll := msg.GetRoll().(type) {
	case *playv1.ResolveConcentrationSaveRequest_RollInApp:
		if !roll.RollInApp {
			return in, false, false, bad("roll_in_app must be true")
		}
		in.inApp = true
	case *playv1.ResolveConcentrationSaveRequest_D20Face:
		in.typed = int(roll.D20Face)
		if in.typed < 1 || in.typed > 20 {
			return in, false, false, bad("d20_face must be 1 to 20")
		}
		if msg.SecondD20Face != nil {
			in.typedFaces = []int{in.typed, int(msg.GetSecondD20Face())}
		}
	case *playv1.ResolveConcentrationSaveRequest_HandToMaster:
		if !roll.HandToMaster {
			return in, false, false, bad("hand_to_master must be true")
		}
		handOver = true
	case *playv1.ResolveConcentrationSaveRequest_Keep:
		if !roll.Keep {
			return in, false, false, bad("keep must be true")
		}
		keep = true
	default:
		return in, false, false, bad("set roll_in_app, d20_face, hand_to_master or keep")
	}
	return in, handOver, keep, nil
}

// concentrationSources are the circumstances of a Constitution saving throw to keep a
// concentration. The prompt the player reads and the roll ResolveConcentrationSave makes both
// take them from here, so the number of d20 the sheet asks for is the number the roll needs.
func concentrationSources(creature combat.Creature) []combat.Source {
	return combat.SaveMode(combat.SaveScene{Creature: creature, Ability: "con", EffectVisible: true})
}

// concentrationCall is a ResolveConcentrationSave request once it is read: who asks, the keys
// and ids it names, and how the save is answered.
type concentrationCall struct {
	m            authz.Membership
	key          string
	encID, winID string
	in           rollInput
	handOver     bool
	keep         bool
	// msg is the request, which the held roll keeps to answer again and whose extra die faces the roll reads.
	msg *playv1.ResolveConcentrationSaveRequest
}

// readConcentrationCall reads a ResolveConcentrationSave request, refusing in this order: the
// caller, the keys, the ids, the answer.
func readConcentrationCall(ctx context.Context, msg *playv1.ResolveConcentrationSaveRequest) (concentrationCall, error) {
	call := concentrationCall{msg: msg}
	var err error
	if call.m, err = heldMember(ctx, msg.GetCampaignId()); err != nil {
		return call, err
	}
	if call.key, call.encID, err = parseCall(msg.GetIdempotencyKey(), msg.GetEncounterId()); err != nil {
		return call, err
	}
	if call.winID, err = parseCombatID(msg.GetWindowId(), "window"); err != nil {
		return call, err
	}
	call.in, call.handOver, call.keep, err = concentrationRollOf(msg)
	return call, err
}

// concentrationRun is what a try of the change leaves for the answer. A retry starts it over.
type concentrationRun struct {
	made   actionEvent
	result *playv1.ConcentrationSaveResult
	offer  *playv1.OutsideInspirationOffer // the d20 was rolled and kept for the Bardic Inspiration question
	lucky  *playv1.LuckyOffer              // or for the halfling's Lucky question, which comes first
	held   *playv1.HeldRollOffer           // the same question, in the one shape every held roll has
}

// ResolveConcentrationSave implements playv1connect.CombatServiceHandler.
func (s *Service) ResolveConcentrationSave(
	ctx context.Context,
	req *connect.Request[playv1.ResolveConcentrationSaveRequest],
) (*connect.Response[playv1.ResolveConcentrationSaveResponse], error) {
	call, err := readConcentrationCall(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	var run concentrationRun
	res, err := s.write(ctx, combatWrite{m: call.m, key: call.key, hash: idem.Hash(req.Msg), kind: eventConcentrationSaveRolled, encounterID: call.encID}, func(c *combatTx) (any, error) {
		run.result, run.offer, run.lucky, run.held = nil, nil, nil, nil
		return s.writeConcentrationSave(ctx, c, call, &run)
	})
	if err != nil {
		return nil, s.dbError(ctx, "resolve a concentration save", err)
	}
	return s.concentrationAnswer(ctx, call.m, res, &run)
}

// concentrationAnswer is the response of a save once the change is written: the question alone
// when the d20 was kept for one, else the encounter and the result.
func (s *Service) concentrationAnswer(ctx context.Context, m authz.Membership, res combatResult, run *concentrationRun) (*connect.Response[playv1.ResolveConcentrationSaveResponse], error) {
	if run.offer != nil || run.lucky != nil || run.held != nil { // nothing was written: the question is the whole answer
		out, err := s.finish(ctx, m, res, nil)
		if err != nil {
			return nil, err
		}
		return connect.NewResponse(&playv1.ResolveConcentrationSaveResponse{Encounter: out, InspirationOffer: run.offer, LuckyOffer: run.lucky, HeldRollOffer: run.held}), nil
	}
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, !run.made.Secret)
	})
	if err != nil {
		return nil, err
	}
	result := run.result
	if result == nil && res.repeated {
		result = concentrationResultOfRetry(res.payload)
		if err := s.hideRetryDC(ctx, m, result); err != nil {
			return nil, err
		}
	}
	if result != nil {
		result.SpellNamePt = s.namesFor(ctx, m.CampaignID)(result.SpellKey)
	}
	return connect.NewResponse(&playv1.ResolveConcentrationSaveResponse{Encounter: out, Result: result}), nil
}

// hideRetryDC zeroes the DC of a retried result for a member the table's rules do not show it to.
func (s *Service) hideRetryDC(ctx context.Context, m authz.Membership, result *playv1.ConcentrationSaveResult) error {
	if result == nil || heldViewerIsMaster(ctx, m) {
		return nil
	}
	rules, err := s.tableRules(ctx, nil, m.CampaignID)
	if err != nil {
		return s.dbError(ctx, "read the table's rules", err)
	}
	if !rules.ShowSaveDC {
		result.Dc = 0
	}
	return nil
}

// concentrationResultOfRetry is the result of a save that a retry of the same request asks for again: read back from the
// event the first time wrote, with the same numbers (nil when the event is not a save's).
func concentrationResultOfRetry(payload []byte) *playv1.ConcentrationSaveResult {
	var ev actionEvent
	if err := json.Unmarshal(payload, &ev); err != nil || ev.Reaction == nil {
		return nil
	}
	return concentrationResultOf(ev)
}

// concentrationResultOf is the result a written save's event tells: the DC, whether the concentration was kept and, when
// the d20 was rolled, the dice with the Bardic Inspiration die and the d20 Lucky rolled again.
func concentrationResultOf(ev actionEvent) *playv1.ConcentrationSaveResult {
	re := ev.Reaction
	out := &playv1.ConcentrationSaveResult{Dc: re.SaveDC, Kept: re.Kept, SpellKey: re.Spell}
	if ev.D20 == 0 { // kept with no roll
		return out
	}
	faces := []int32{ev.D20}
	if ev.D20B != 0 { // two d20, in the order they were rolled: the one that counted is Counted
		faces = []int32{ev.D20, ev.D20B}
		if ev.Counted == 1 {
			faces = []int32{ev.D20B, ev.D20}
		}
	}
	out.Save = diceRoll(clamp32(len(faces), 1, 2), 20, faces, re.SaveBonus, ev.Total, ev.Physical)
	out.Save.Rerolled = ev.rerolls()
	out.BonusDice = bonusDiceOf(ev)
	return out
}

// writeConcentrationSave is the change of a concentration save, in the transaction: the window,
// the answer (kept, or rolled), the end of the concentration on a failure, the window closed
// and the event.
func (s *Service) writeConcentrationSave(ctx context.Context, c *combatTx, call concentrationCall, run *concentrationRun) (any, error) {
	if err := notEnded(c.enc); err != nil {
		return nil, err
	}
	w, reactor, err := s.windowToAnswer(ctx, c, call.m, call.winID)
	if err != nil {
		return nil, err
	}
	if w.Kind != string(reaction.Concentration) {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("this window is not a concentration save: use AnswerReaction"))
	}
	t := windowTriggerOf(w)
	master := call.m.Role == authz.RoleMaster
	if err := refuseSaveAnswer(t, master, call.keep, call.handOver); err != nil {
		return nil, err
	}
	if call.handOver {
		return nil, s.handSaveToMaster(ctx, c, w, t)
	}
	dc := clamp32(reaction.ConcentrationDC(int(t.Taken)), 0, math.MaxInt32)
	ev := actionEvent{Round: c.enc.Round, Secret: reactor.Hidden, Actor: reactor.ID, Target: t.Actor, Key: t.Key, Pending: t.Pending}
	ev.Reaction = &reactionEvent{Kind: string(reaction.Concentration), Window: w.ID, ByMaster: master, Spell: t.Spell, SaveDC: dc}
	kept, err := s.concentrationOutcome(ctx, c, call, reactor, t, &ev, run)
	if err != nil {
		return nil, err
	}
	if run.offer != nil || run.lucky != nil || run.held != nil { // the d20 is kept for a question: nothing is written
		return nil, nil
	}
	if !kept {
		if err := s.loseConcentration(ctx, c, reactor); err != nil {
			return nil, err
		}
	}
	if _, err := s.closeWindow(ctx, c, w, windowAnswered, reaction.ReasonNone, windowOutcome{ByMaster: master, Used: true}, ""); err != nil {
		return nil, err
	}
	if err := touch(ctx, c); err != nil {
		return nil, err
	}
	c.characterID = &reactor.CharacterID
	run.made = ev
	return ev, nil
}

// concentrationOutcome is the save of the window: kept as it is (the master's word) or rolled. It answers
// whether the concentration is kept, and puts the result in run with the DC only for who reads it: the
// master, and the answering member when the table shows the DC of a saving throw.
func (s *Service) concentrationOutcome(ctx context.Context, c *combatTx, call concentrationCall, reactor playdb.Combatant, t windowTrigger, ev *actionEvent, run *concentrationRun) (bool, error) {
	dc := ev.Reaction.SaveDC
	kept := true
	if call.keep {
		ev.Reaction.Kept = true
		run.result = &playv1.ConcentrationSaveResult{Dc: dc, Kept: true, SpellKey: t.Spell}
	} else {
		var err error
		if kept, err = s.rollConcentration(ctx, c, call, reactor, t, dc, ev, run); err != nil {
			return false, err
		}
	}
	if run.result != nil && !heldViewerIsMaster(ctx, call.m) && !c.rules.ShowSaveDC {
		run.result.Dc = 0
	}
	return kept, nil
}

// loseConcentration ends the concentration, and the spell with it, of a creature that failed its
// save: the line of the log says what stopped (written before the save's own event, as a cast's
// creatures are).
func (s *Service) loseConcentration(ctx context.Context, c *combatTx, reactor playdb.Combatant) error {
	stop := actionEvent{Round: c.enc.Round, Secret: reactor.Hidden, Actor: reactor.ID}
	if err := s.stopConcentrating(ctx, c, reactor, &stop); err != nil {
		return err
	}
	return insertEvent(ctx, c, eventConditionsSet, &c.actorUserID, nil, stop)
}

// rollConcentration checks the way of rolling, takes the save's d20 from the gate and writes what it
// showed in the event. It answers whether the concentration is kept, unless the gate kept the d20 for a
// question instead (the offers are in run, and nothing is written).
func (s *Service) rollConcentration(
	ctx context.Context, c *combatTx, call concentrationCall,
	reactor playdb.Combatant, t windowTrigger, dc int32, ev *actionEvent, run *concentrationRun,
) (bool, error) {
	if call.m.Role != authz.RoleMaster {
		if err := s.mustRollThisWay(ctx, c.tx, call.m, call.in); err != nil {
			return false, err
		}
	}
	save, err := s.saveOf(ctx, c.tx, c.session.CampaignID, reactor, "con")
	if err != nil {
		return false, err
	}
	// The saving throw has a mode like any other (SRD 5.1): exhaustion 3 or more is
	// disadvantage on saving throws. Haste's advantage is on Dexterity saves only, so it
	// does not reach this Constitution save.
	states, err := s.readStates(ctx, c.tx, c.enc.ID)
	if err != nil {
		return false, err
	}
	sheet, err := s.sheetOf(ctx, c.tx, c.session.CampaignID, reactor)
	if err != nil {
		return false, err
	}
	mode := combat.Resolve(concentrationSources(creatureFacts(reactor, states, sheet.Traits)))
	// A player's character that holds a Bardic Inspiration die has the d20 kept until the player
	// answers (nothing has been written yet); the answer brings it back with the die added. So
	// has a halfling's d20 that shows a 1, for the Lucky question first.
	spec := saveSpec(call.m, heldConcentrationSave, reactor, call.key, call.msg, call.in, save.Bonus, mode, sheet.Traits.Lucky)
	// Indomitable (held_indomitable.go): the dice an effect adds after the gate move the total, so the roller may not
	// know the result yet, as when the table hides the DC.
	extras := len(effectDiceFor(states, reactor.ID, rules.RollAppliesSave)) > 0 || call.msg.GetManeuverKey() != ""
	spec.Save = &saveRule{DC: int(dc), Blind: !c.rules.ShowSaveDC || extras}
	gate, err := s.heldGate(ctx, s.inspirationGate(c), spec)
	if err != nil {
		return false, err
	}
	if gate.Held != nil {
		run.offer, run.lucky, run.held = s.outsideOffers(ctx, c.tx, gate.Held)
		return false, nil
	}
	// Bênção and Perdição add their die to the saving throw (SRD 5.1).
	own, err := s.maneuverDice(ctx, c.tx, c.session.CampaignID, reactor.CharacterID, call.msg.GetManeuverKey(), rules.ManeuverSave, "", "con")
	if err != nil {
		return false, err
	}
	extra, delta, err := s.rollEffectDice(call.in, append(effectDiceFor(states, reactor.ID, rules.RollAppliesSave), own...), call.msg.GetExtraDieFaces())
	if err != nil {
		return false, err
	}
	if err := s.spendOnceEffects(ctx, c, extra); err != nil {
		return false, err
	}
	if ev.ManeuverSpent, err = s.spendManeuverDice(ctx, c, reactor.CharacterID, extra); err != nil {
		return false, err
	}
	return recordConcentrationRoll(ev, run, gate, save.Bonus, delta, dc, t.Spell), nil
}

// recordConcentrationRoll writes what the save showed in the event and the result, and answers
// whether the concentration is kept: the d20 and the dice the effects added reach the DC.
func recordConcentrationRoll(ev *actionEvent, run *concentrationRun, gate gateResult, bonus, delta int, dc int32, spell string) bool {
	roll := gate.Roll
	bonusFace := 0
	if gate.Bonus != nil && gate.Indomitable == nil {
		bonusFace = int(gate.Bonus.Face) // the die the player added after rolling (a reroll leaves it out of the total)
	}
	total := roll.Total + delta
	kept := total >= int(dc)
	re := ev.Reaction
	re.SaveD20, re.SaveBonus, re.Saved, re.Kept = clamp32(roll.Face(), 1, 20), conv.Int32(bonus+delta+bonusFace), kept, kept
	ev.D20, ev.Modifier, ev.Total, ev.Physical = re.SaveD20, re.SaveBonus, conv.Int32(total), roll.Physical
	ev.D20B, ev.Counted = clamp32(roll.otherFace(), 0, 20), clamp32(roll.Index, 0, 1)
	ev.Lucky, ev.Indomitable = gate.Lucky, gate.Indomitable // the d20 rolled again: the log says "Sortudo: 1 → 14"
	run.result = &playv1.ConcentrationSaveResult{
		Save: diceRoll(clamp32(len(roll.Faces), 1, 2), 20, faces32(roll.Faces), re.SaveBonus, ev.Total, roll.Physical), Dc: dc, Kept: kept, SpellKey: spell,
	}
	run.result.Save.Rerolled = rerollsOf(gate.Lucky, gate.Indomitable)
	if gate.Bonus != nil {
		run.result.BonusDice = []*playv1.BonusDie{gate.Bonus}
		ev.Res = &resourceEvent{Kind: resBardicUse, Sides: gate.Bonus.Sides, Face: gate.Bonus.Face} // a retry reads the die back from here
	}
	return kept
}
