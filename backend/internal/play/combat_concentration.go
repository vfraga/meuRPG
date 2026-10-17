package play

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
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
		return s.endConcentrationAtZero(ctx, c, target, d)
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
// is incapacitated and loses the concentration with no save (SRD, Duration).
func (s *Service) endConcentrationAtZero(ctx context.Context, c *combatTx, target playdb.Combatant, d damageLanded) error {
	ev := actionEvent{Round: c.enc.Round, Secret: target.Hidden, Actor: target.ID, Reaction: &reactionEvent{Kind: concentrationEnd, Group: d.pending.ID}}
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
			taken += num(o.Taken)
		}
	}
	return taken, nil
}

// handSaveToMaster marks a concentration save as left to the master: the owner can no longer
// roll it, and the master's card asks for it.
func (s *Service) handSaveToMaster(ctx context.Context, c *combatTx, w playdb.ReactionWindow, t windowTrigger) error {
	t.Handed = true
	body, err := json.Marshal(t)
	if err != nil {
		return fmt.Errorf("encode the window's trigger: %w", err)
	}
	if _, err := c.q.SetReactionWindowStep(ctx, playdb.SetReactionWindowStepParams{ID: w.ID, Step: w.Step, Trigger: body}); err != nil {
		return fmt.Errorf("hand the save to the master: %w", err)
	}
	if err := touchEncounter(ctx, c); err != nil {
		return err
	}
	return nil
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
}

// readConcentrationCall reads a ResolveConcentrationSave request, refusing in the order the
// handler always did: the caller, the keys, the ids, the answer.
func readConcentrationCall(ctx context.Context, msg *playv1.ResolveConcentrationSaveRequest) (concentrationCall, error) {
	var call concentrationCall
	var err error
	if call.m, err = authz.RequireCampaignMember(ctx, msg.GetCampaignId()); err != nil {
		return call, err
	}
	if call.key, err = parseKey(msg.GetIdempotencyKey()); err != nil {
		return call, err
	}
	if call.encID, err = parseCombatID(msg.GetEncounterId(), "encounter"); err != nil {
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
		run.result, run.offer, run.lucky = nil, nil, nil
		return s.writeConcentrationSave(ctx, c, call, req.Msg, &run)
	})
	if err != nil {
		return nil, s.dbError(ctx, "resolve a concentration save", err)
	}
	return s.concentrationAnswer(ctx, call.m, res, &run)
}

// concentrationAnswer is the response of a save once the change is written: the question alone
// when the d20 was kept for one, else the encounter and the result.
func (s *Service) concentrationAnswer(ctx context.Context, m authz.Membership, res combatResult, run *concentrationRun) (*connect.Response[playv1.ResolveConcentrationSaveResponse], error) {
	if run.offer != nil || run.lucky != nil { // nothing was written: the question is the whole answer
		out, err := s.finish(ctx, m, res, nil)
		if err != nil {
			return nil, err
		}
		return connect.NewResponse(&playv1.ResolveConcentrationSaveResponse{Encounter: out, InspirationOffer: run.offer, LuckyOffer: run.lucky}), nil
	}
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, !run.made.Secret)
	})
	if err != nil {
		return nil, err
	}
	if run.result != nil {
		run.result.SpellNamePt = s.namesFor(ctx, m.CampaignID)(run.result.SpellKey)
	}
	return connect.NewResponse(&playv1.ResolveConcentrationSaveResponse{Encounter: out, Result: run.result}), nil
}

// refuseSaveAnswer checks who may answer a save window, for the concentration save and the
// effect's: the master's own options (keep, skip) are refused to a player, "leave it to the
// master" to the master, and a save already handed over to anybody but the master.
func refuseSaveAnswer(t windowTrigger, master, masterOnly, handOver bool) error {
	switch {
	case masterOnly && !master:
		return errWindowDenied()
	case handOver && master:
		return connect.NewError(connect.CodeInvalidArgument, errors.New("the master rolls it: use roll_in_app or d20_face"))
	case t.Handed && !master:
		return errWindowDenied() // the roll was left to the master
	}
	return nil
}

// writeConcentrationSave is the change of a concentration save, in the transaction: the window,
// the answer (kept, or rolled), the end of the concentration on a failure, the window closed
// and the event.
func (s *Service) writeConcentrationSave(ctx context.Context, c *combatTx, call concentrationCall, msg *playv1.ResolveConcentrationSaveRequest, run *concentrationRun) (any, error) {
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
	kept := true
	if call.keep {
		ev.Reaction.Kept = true
		run.result = &playv1.ConcentrationSaveResult{Dc: dc, Kept: true, SpellKey: t.Spell}
	} else {
		var held bool
		if kept, held, err = s.rollConcentration(ctx, c, call, msg, reactor, t, dc, &ev, run); err != nil || held {
			return nil, err
		}
	}
	if !kept {
		if err := s.endConcentrationWithLine(ctx, c, reactor); err != nil {
			return nil, err
		}
	}
	if _, err := s.closeWindow(ctx, c, w, windowAnswered, reaction.ReasonNone, windowOutcome{ByMaster: master, Used: true}, ""); err != nil {
		return nil, err
	}
	if err := touchEncounter(ctx, c); err != nil {
		return nil, err
	}
	c.characterID = &reactor.CharacterID
	run.made = ev
	return ev, nil
}

// endConcentrationWithLine ends the concentration, and the spell with it, of a creature that lost
// it (a failed save, a new concentration spell, incapacity, the end of the spell): the line of the
// log says what stopped (written before the event of the change that caused it, as a cast's
// creatures are).
func (s *Service) endConcentrationWithLine(ctx context.Context, c *combatTx, caster playdb.Combatant) error {
	stop := actionEvent{Round: c.enc.Round, Secret: caster.Hidden, Actor: caster.ID}
	if err := s.stopConcentrating(ctx, c, caster, &stop); err != nil {
		return err
	}
	return insertEvent(ctx, c, eventConditionsSet, &c.actorUserID, nil, stop)
}

// concentrationStep is the d20 of a concentration save as the gate gave it, with the save's bonus
// and the effects' states the dice come from.
type concentrationStep struct {
	gate   gateResult
	bonus  int
	states map[string][]playdb.CombatantState
}

// heldConcentrationRoll checks the way of rolling and takes the save's d20 from the gate: the
// roll, or the question it waits for (the gate's Held).
func (s *Service) heldConcentrationRoll(ctx context.Context, c *combatTx, call concentrationCall, msg *playv1.ResolveConcentrationSaveRequest, reactor playdb.Combatant) (concentrationStep, error) {
	var none concentrationStep
	if call.m.Role != authz.RoleMaster {
		if err := s.mustRollThisWay(ctx, c.tx, call.m, call.in); err != nil {
			return none, err
		}
	}
	save, err := s.saveOf(ctx, c.tx, c.session.CampaignID, reactor, "con")
	if err != nil {
		return none, err
	}
	// The saving throw has a mode like any other (SRD 5.1): exhaustion 3 or more is
	// disadvantage on saving throws. Haste's advantage is on Dexterity saves only, so it
	// does not reach this Constitution save.
	states, err := s.readStates(ctx, c.tx, c.enc.ID)
	if err != nil {
		return none, err
	}
	sheet, err := s.sheetOf(ctx, c.tx, c.session.CampaignID, reactor)
	if err != nil {
		return none, err
	}
	mode := combat.Resolve(concentrationSources(creatureFacts(reactor, states, sheet.Traits)))
	// A player's character that holds a Bardic Inspiration die has the d20 kept until the player
	// answers (nothing has been written yet); the answer brings it back with the die added. So
	// has a halfling's d20 that shows a 1, for the Lucky question first.
	gate, err := s.heldGate(ctx, s.inspirationGate(c), saveSpec(call.m, heldConcentrationSave, reactor, call.key, msg, call.in, save.Bonus, mode, sheet.Traits.Lucky))
	return concentrationStep{gate: gate, bonus: save.Bonus, states: states}, err
}

// rollConcentration rolls the save of a concentration and writes what it showed in the event. It
// answers whether the concentration is kept, and whether the d20 was kept for a question instead
// (the offers are in run, and nothing is written).
func (s *Service) rollConcentration(
	ctx context.Context, c *combatTx, call concentrationCall, msg *playv1.ResolveConcentrationSaveRequest,
	reactor playdb.Combatant, t windowTrigger, dc int32, ev *actionEvent, run *concentrationRun,
) (kept, held bool, err error) {
	step, err := s.heldConcentrationRoll(ctx, c, call, msg, reactor)
	if err != nil {
		return false, false, err
	}
	if step.gate.Held != nil {
		run.offer, run.lucky = s.outsideOffers(ctx, c.tx, step.gate.Held)
		return false, true, nil
	}
	// Bênção and Perdição add their die to the saving throw (SRD 5.1).
	extra, delta, err := s.rollEffectDice(call.in, effectDiceFor(step.states, reactor.ID, rules.RollAppliesSave), msg.GetExtraDieFaces())
	if err != nil {
		return false, false, err
	}
	if err := s.spendOnceEffects(ctx, c, extra); err != nil {
		return false, false, err
	}
	return recordConcentrationRoll(ev, run, step, delta, dc, t.Spell), false, nil
}

// recordConcentrationRoll writes what the save showed in the event and the result, and answers
// whether the concentration is kept: the d20 and the dice the effects added reach the DC.
func recordConcentrationRoll(ev *actionEvent, run *concentrationRun, step concentrationStep, delta int, dc int32, spell string) bool {
	roll := step.gate.Roll
	bonusFace := 0
	if step.gate.Bonus != nil {
		bonusFace = int(step.gate.Bonus.Face) // the die the player added after rolling
	}
	total := roll.Total + delta
	kept := total >= int(dc)
	re := ev.Reaction
	re.SaveD20, re.SaveBonus, re.Saved, re.Kept = clamp32(roll.Face(), 1, 20), clamp32(step.bonus+delta+bonusFace, math.MinInt32, math.MaxInt32), kept, kept
	ev.D20, ev.Modifier, ev.Total, ev.Physical = re.SaveD20, re.SaveBonus, clamp32(total, math.MinInt32, math.MaxInt32), roll.Physical
	ev.D20B, ev.Counted = clamp32(roll.otherFace(), 0, 20), clamp32(roll.Index, 0, 1)
	ev.Lucky = step.gate.Lucky // the d20 Lucky rolled again: the log says "Sortudo: 1 → 14"
	run.result = &playv1.ConcentrationSaveResult{
		Save: diceRoll(clamp32(len(roll.Faces), 1, 2), 20, faces32(roll.Faces), re.SaveBonus, ev.Total, roll.Physical), Dc: dc, Kept: kept, SpellKey: spell,
	}
	run.result.Save.Rerolled = step.gate.Lucky.proto()
	if step.gate.Bonus != nil {
		run.result.BonusDice = []*playv1.BonusDie{step.gate.Bonus}
	}
	return kept
}

// windowToAnswer finds the window an answer is for, inside the change's transaction,
// and checks the caller may answer it and that it is its turn: the master answers any
// window; a player only the ones of their own character, and any other window id, real
// or not, is refused the same way (RN-10). A window that is gone or already answered
// is "not your turn to answer".
func (s *Service) windowToAnswer(ctx context.Context, c *combatTx, m authz.Membership, id string) (playdb.ReactionWindow, playdb.Combatant, error) {
	var none playdb.ReactionWindow
	master := m.Role == authz.RoleMaster
	w, err := c.q.GetReactionWindowForUpdate(ctx, playdb.GetReactionWindowForUpdateParams{EncounterID: c.enc.ID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		if master {
			return none, playdb.Combatant{}, connect.NewError(connect.CodeNotFound, errors.New("reaction window not found"))
		}
		return none, playdb.Combatant{}, errWindowDenied()
	}
	if err != nil {
		return none, playdb.Combatant{}, fmt.Errorf("find the reaction window: %w", err)
	}
	var reactor playdb.Combatant
	if w.ReactorID != nil {
		cs, err := c.q.ListCombatants(ctx, c.enc.ID)
		if err != nil {
			return none, reactor, fmt.Errorf("list the combatants: %w", err)
		}
		if i := slices.IndexFunc(cs, func(x playdb.Combatant) bool { return x.ID == *w.ReactorID }); i >= 0 {
			reactor = cs[i]
		}
	}
	if !master && (w.ReactorID == nil || reactor.ID == "" || reactor.UserID == nil || *reactor.UserID != m.UserID || isCreature(reactor)) {
		return none, reactor, errWindowDenied()
	}
	if w.Status != windowOpen {
		return none, reactor, errNotYourTurnToAnswer()
	}
	if err := s.mustBeFirstOfGroup(ctx, c, w); err != nil {
		return none, reactor, err
	}
	return w, reactor, nil
}
