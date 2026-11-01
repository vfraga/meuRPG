package play

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dice"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
	"github.com/PuraFome/meuRPG/backend/internal/rules/reaction"
)

// The saving throw an effect asks (RN-22): Imobilizar Pessoa and Riso Histérico at the end
// of the target's turn ("o alvo pode fazer outro teste de resistência"), Teia at the
// start of it. It is a reaction window of its own kind (PM-04), answered with
// RollEffectSave: the target's player rolls in the app or types the d20, or leaves it to
// the master; the master rolls for an NPC, or skips the save (a table option the SRD does
// not give). While a window is open the turn does not move (reactionGate), and the turn
// that ended waits for it to pass (releaseHeldTurn).

// defaultEffectDC is the DC an effect asks when nobody gave one (a master's effect with
// no caster): the SRD's easy-to-hard middle is 10 to 15 and the table says the rest.
const defaultEffectDC = 10

// openEffectSave opens the saving throw of an effect on its target at a phase of the turn,
// once for the turn: a repeat finds the window open and opens none. damage says the
// window opened because the target took damage (advantage on the roll).
func (s *Service) openEffectSave(ctx context.Context, c *combatTx, group string, st playdb.CombatantState, target playdb.Combatant, phase string, damage bool) (bool, error) {
	if target.Defeated && target.Kind != kindPlayer {
		return false, nil // a defeated creature owes no saving throw
	}
	open, err := openWindowsOf(ctx, c)
	if err != nil {
		return false, err
	}
	if slices.ContainsFunc(open, func(w playdb.ReactionWindow) bool {
		t := windowTriggerOf(w)
		return reaction.Kind(w.Kind) == reaction.EffectSave && t.Effect == st.ID && t.Phase == phase && (t.OnDamage == damage || !damage)
	}) {
		return false, nil
	}
	trigger := windowTrigger{Actor: conv.OrZero(st.SourceID), Target: target.ID, Key: conv.OrZero(st.SourceKey), Effect: st.ID, Phase: phase, OnDamage: damage}
	_, err = s.openWindows(ctx, c, group, nil, []windowSpec{{kind: reaction.EffectSave, reactor: &target, trigger: trigger}})
	return err == nil, err
}

// closeEffectWindows closes the saving throws an effect row had open: it ended, or its
// caster lost the concentration.
func (s *Service) closeEffectWindows(ctx context.Context, c *combatTx, st playdb.CombatantState, reason string) error {
	open, err := openWindowsOf(ctx, c)
	if err != nil {
		return err
	}
	for _, w := range open {
		if reaction.Kind(w.Kind) != reaction.EffectSave || windowTriggerOf(w).Effect != st.ID {
			continue
		}
		if _, err := s.closeWindow(ctx, c, w, windowClosed, windowReasonOf(reason), windowOutcome{}, ""); err != nil {
			return err
		}
	}
	return nil
}

// effectDie is a die an effect adds to or takes from a roll, with the face it showed.
type effectDie struct {
	Key   string `json:"key"`
	Faces int    `json:"faces"`
	Sign  int    `json:"sign"`
	Face  int    `json:"face,omitempty"`
	// Hidden says the master keeps the effect from the players (RN-10).
	Hidden bool `json:"hidden,omitempty"`
	// Once, EffectID and CharacterID say the die is spent by the roll (Guidance, Resistance), and
	// which effect of which character goes with it; they are not kept in the events.
	Once        bool   `json:"-"`
	EffectID    string `json:"-"`
	CharacterID string `json:"-"`
	// ManeuverResource says the die is a table maneuver's, and the resource a use of which the roll spends.
	ManeuverResource string `json:"-"`
}

// effectDiceFor lists the dice the effects on a combatant add to a roll of the kind
// (rules.RollAppliesAttack or rules.RollAppliesSave), in the order of the effects.
func effectDiceFor(states map[string][]playdb.CombatantState, id, applies string) []effectDie {
	var out []effectDie
	for _, st := range effectsOn(states, id) {
		for _, d := range combat.EffectDice(conv.OrZero(st.SourceKey), effectModifiers(st), applies) {
			out = append(out, effectDie{Key: d.Source, Faces: d.Faces, Sign: d.Sign, Hidden: !st.PlayerVisible, Once: d.Once, EffectID: st.ID})
		}
	}
	return out
}

// rollEffectDice rolls the dice an effect adds to a roll. With the app's dice the server
// rolls them; with physical dice the player typed the faces, one for each, in the same
// order. It returns the dice with their faces and the number they add.
func (s *Service) rollEffectDice(in rollInput, dd []effectDie, typed []int32) ([]effectDie, int, error) {
	if len(dd) == 0 {
		return nil, 0, nil
	}
	if !in.inApp && len(typed) != len(dd) {
		return nil, 0, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("the roll takes %d more die(s): type their faces in extra_die_faces", len(dd)))
	}
	total := 0
	out := make([]effectDie, len(dd))
	for i, d := range dd {
		face := 0
		if in.inApp {
			r, err := dice.Roll(s.roller, dice.Expr{Count: 1, Sides: d.Faces})
			if err != nil {
				return nil, 0, fmt.Errorf("roll an effect's die: %w", err)
			}
			face = r.Faces[0]
		} else {
			face = int(typed[i])
			if face < 1 || face > d.Faces {
				return nil, 0, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("extra_die_faces[%d] must be 1 to %d", i, d.Faces))
			}
		}
		d.Face = face
		out[i] = d
		total += d.Sign * face
	}
	return out, total, nil
}

// extraDiceProto writes the dice an effect added, for who reads the roll: the effect's name,
// or "Outra fonte" with no key when the master hides the effect from the players (RN-20).
func extraDiceProto(dd []effectDie, master bool, names func(string) string) []*playv1.ExtraDie {
	out := make([]*playv1.ExtraDie, 0, len(dd))
	for _, d := range dd {
		e := &playv1.ExtraDie{SourceKey: d.Key, Faces: clamp32(d.Faces, 0, 100), Sign: clamp32(d.Sign, -1, 1), Face: clamp32(d.Face, 0, 100)}
		if names != nil {
			e.SourceNamePt = names(d.Key)
		}
		if d.Hidden && !master {
			e.SourceKey, e.SourceNamePt = "", "Outra fonte"
		}
		out = append(out, e)
	}
	return out
}

// effectSaveRollOf reads how an effect's saving throw is answered.
func effectSaveRollOf(msg *playv1.RollEffectSaveRequest) (in rollInput, delegate, skip bool, err error) {
	bad := func(text string) error { return connect.NewError(connect.CodeInvalidArgument, errors.New(text)) }
	switch roll := msg.GetRoll().(type) {
	case *playv1.RollEffectSaveRequest_RollInApp:
		if !roll.RollInApp {
			return in, false, false, bad("roll_in_app must be true")
		}
		in.inApp = true
	case *playv1.RollEffectSaveRequest_D20Face:
		in.typed = int(roll.D20Face)
		if in.typed < 1 || in.typed > 20 {
			return in, false, false, bad("d20_face must be 1 to 20")
		}
		if msg.SecondD20Face != nil {
			in.typedFaces = []int{in.typed, int(msg.GetSecondD20Face())}
		}
	case *playv1.RollEffectSaveRequest_DelegateToMaster:
		if !roll.DelegateToMaster {
			return in, false, false, bad("delegate_to_master must be true")
		}
		delegate = true
	case *playv1.RollEffectSaveRequest_Skip:
		if !roll.Skip {
			return in, false, false, bad("skip must be true")
		}
		skip = true
	default:
		return in, false, false, bad("set roll_in_app, d20_face, delegate_to_master or skip")
	}
	return in, delegate, skip, nil
}

// effectSaveCall is a RollEffectSave request once it is read: who asks, the keys and ids it
// names, and how the save is answered.
type effectSaveCall struct {
	m            authz.Membership
	key          string
	encID, winID string
	in           rollInput
	delegate     bool
	skip         bool
	typedExtra   []int32
}

// readEffectSaveCall reads a RollEffectSave request, refusing in the order the handler always
// did: the caller, the keys, the ids, the answer.
func readEffectSaveCall(ctx context.Context, msg *playv1.RollEffectSaveRequest) (effectSaveCall, error) {
	var call effectSaveCall
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
	if call.in, call.delegate, call.skip, err = effectSaveRollOf(msg); err != nil {
		return call, err
	}
	call.typedExtra = msg.GetExtraDieFaces()
	return call, nil
}

// effectSaveRun is what a try of the change leaves for the answer. A retry starts it over.
type effectSaveRun struct {
	made   actionEvent
	result *playv1.EffectSaveResult
	offer  *playv1.OutsideInspirationOffer // the d20 was rolled and kept for the Bardic Inspiration question
	lucky  *playv1.LuckyOffer              // or for the halfling's Lucky question, which comes first
	held   *playv1.HeldRollOffer           // the same question, in the one shape every held roll has
}

// RollEffectSave implements playv1connect.LastingEffectServiceHandler.
func (s *Service) RollEffectSave(
	ctx context.Context,
	req *connect.Request[playv1.RollEffectSaveRequest],
) (*connect.Response[playv1.RollEffectSaveResponse], error) {
	call, err := readEffectSaveCall(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	var run effectSaveRun
	res, err := s.write(ctx, combatWrite{m: call.m, key: call.key, hash: idem.Hash(req.Msg), kind: eventLastingSaved, encounterID: call.encID}, func(c *combatTx) (any, error) {
		run.result, run.offer, run.lucky, run.held = nil, nil, nil, nil
		return s.writeEffectSave(ctx, c, call, req.Msg, &run)
	})
	if err != nil {
		return nil, s.dbError(ctx, "roll an effect's saving throw", err)
	}
	return s.effectSaveAnswer(ctx, call.m, res, &run)
}

// effectSaveAnswer is the response of a save once the change is written: the question alone when
// the d20 was kept for one, else the encounter and the result (rebuilt from the event on a retry
// the change never ran for).
func (s *Service) effectSaveAnswer(ctx context.Context, m authz.Membership, res combatResult, run *effectSaveRun) (*connect.Response[playv1.RollEffectSaveResponse], error) {
	if run.offer != nil || run.lucky != nil || run.held != nil { // nothing was written: the question is the whole answer
		out, err := s.finish(ctx, m, res, nil)
		if err != nil {
			return nil, err
		}
		return connect.NewResponse(&playv1.RollEffectSaveResponse{Encounter: out, InspirationOffer: run.offer, LuckyOffer: run.lucky, HeldRollOffer: run.held}), nil
	}
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, !run.made.Secret)
	})
	if err != nil {
		return nil, err
	}
	if run.result == nil { // a retry the closure never ran for: the first answer is in the event
		showDC := heldViewerIsMaster(ctx, m)
		if !showDC {
			rules, err := s.tableRules(ctx, nil, m.CampaignID)
			if err != nil {
				return nil, s.dbError(ctx, "read the table's rules", err)
			}
			showDC = rules.ShowSaveDC
		}
		run.result = savedResultOf(res, run.made, showDC)
	}
	return connect.NewResponse(&playv1.RollEffectSaveResponse{Encounter: out, Result: run.result}), nil
}

// effectSaveStep is the save being answered, inside the change: the request, the window and who
// answers it, the effect it is for and the event it writes.
type effectSaveStep struct {
	call    effectSaveCall
	msg     *playv1.RollEffectSaveRequest
	run     *effectSaveRun
	w       playdb.ReactionWindow
	reactor playdb.Combatant
	t       windowTrigger
	st      playdb.CombatantState
	content *rules.Content
	def     *rules.EffectDef
	ability string
	dc      int32
	cs      []playdb.Combatant
	ev      actionEvent
	le      *lastingEvent
	// zone is set when the save is the one an area a spell keeps on the map asks (zones_save.go).
	zone *zoneSaveStep
}

// writeEffectSave is the change of an effect's saving throw, in the transaction: the window and
// the effect, the answer (skipped, auto-failed or rolled), the window closed, what the save does
// to the effect, and the event.
func (s *Service) writeEffectSave(ctx context.Context, c *combatTx, call effectSaveCall, msg *playv1.RollEffectSaveRequest, run *effectSaveRun) (any, error) {
	x := &effectSaveStep{call: call, msg: msg, run: run}
	if err := notEnded(c.enc); err != nil {
		return nil, err
	}
	var err error
	if x.w, x.reactor, err = s.windowToAnswer(ctx, c, call.m, call.winID); err != nil {
		return nil, err
	}
	if reaction.Kind(x.w.Kind) != reaction.EffectSave {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("this window is not an effect's saving throw: use AnswerReaction"))
	}
	x.t = windowTriggerOf(x.w)
	if err := refuseSaveAnswer(x.t, c.master, call.skip, call.delegate); err != nil {
		return nil, err
	}
	if call.delegate {
		return nil, s.handSaveToMaster(ctx, c, x.w, x.t)
	}
	if err := s.loadEffectSave(ctx, c, x); err != nil {
		return nil, err
	}
	saved, held, err := s.answerEffectSave(ctx, c, x)
	if err != nil || held {
		return nil, err
	}
	return s.settleEffectSave(ctx, c, x, saved)
}

// loadEffectSave reads what the save is about: the effect, its definition, the ability and the DC
// it asks, the combatants, and starts the event and the log line.
func (s *Service) loadEffectSave(ctx context.Context, c *combatTx, x *effectSaveStep) error {
	if x.t.Zone != "" {
		return s.loadZoneSave(ctx, c, x)
	}
	st, err := c.q.GetLastingEffect(ctx, playdb.GetLastingEffectParams{ID: x.t.Effect, EncounterID: c.enc.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return errNotYourTurnToAnswer() // the window closes with its effect: never reached
	}
	if err != nil {
		return fmt.Errorf("find the effect: %w", err)
	}
	x.st = st
	if x.content, err = s.contentOf(ctx, c); err != nil {
		return err
	}
	x.def, _ = s.effectDef(x.content, conv.OrZero(st.SourceKey))
	x.ability = conv.OrZero(st.EndSaveAbility)
	if x.t.Phase == combat.PhaseStart {
		x.ability = conv.OrZero(st.StartSaveAbility)
	}
	x.dc = conv.OrZero(st.SaveDc)
	if x.dc == 0 {
		x.dc = defaultEffectDC
	}
	x.ev = actionEvent{Round: c.enc.Round, Secret: s.effectHiddenFrom(st, []playdb.Combatant{x.reactor}), Actor: x.reactor.ID, Key: conv.OrZero(st.SourceKey)}
	x.le = &lastingEvent{Key: conv.OrZero(st.SourceKey), Change: "saved", Targets: []string{x.reactor.ID}, Caster: conv.OrZero(st.SourceID), Ability: x.ability, DC: x.dc, Effects: []string{st.ID}}
	if x.cs, err = listCombatants(ctx, c); err != nil {
		return err
	}
	return nil
}

// answerEffectSave answers the save: the master's skip, or the roll. It says whether the target
// saved, and whether the d20 was kept for a question instead (nothing is written then).
func (s *Service) answerEffectSave(ctx context.Context, c *combatTx, x *effectSaveStep) (saved, held bool, err error) {
	if x.call.skip {
		x.le.Change = "skipped"
		x.run.result = &playv1.EffectSaveResult{Skipped: true}
		return false, false, nil
	}
	return s.rollEffectSaveD20(ctx, c, x)
}

// effectSaveFacts is what the roll of an effect's save is made from: the effects' states, the
// target's save and its roll mode.
type effectSaveFacts struct {
	states   map[string][]playdb.CombatantState
	sheet    link.Sheet
	creature combat.Creature
	save     link.Save
	mode     combat.RollMode
}

// readEffectSaveFacts checks the way of rolling and reads the target's save and the mode it is
// rolled with: advantage after damage asks (Riso Histérico, SRD).
func (s *Service) readEffectSaveFacts(ctx context.Context, c *combatTx, x *effectSaveStep) (effectSaveFacts, error) {
	var f effectSaveFacts
	if !c.master {
		if err := s.mustRollThisWay(ctx, c.tx, x.call.m, x.call.in); err != nil {
			return f, err
		}
	}
	var err error
	if f.states, err = s.readStates(ctx, c.tx, c.enc.ID); err != nil {
		return f, err
	}
	if f.sheet, err = s.sheetOf(ctx, c.tx, c.session.CampaignID, x.reactor); err != nil {
		return f, err
	}
	f.creature = creatureFacts(x.reactor, f.states, f.sheet.Traits)
	sources := combat.SaveMode(combat.SaveScene{Creature: f.creature, Ability: x.ability, EffectVisible: true, Against: effectSaveAgainst(conv.OrZero(x.st.SourceKey), x.def)})
	if x.t.OnDamage { // Riso Histérico: advantage on the save the damage asks (SRD)
		sources = append(sources, combat.Source{Kind: combat.SourceEffectSave, Effect: combat.ModeAdvantage})
	}
	f.mode = combat.Resolve(sources)
	f.save, err = s.saveOf(ctx, c.tx, c.session.CampaignID, x.reactor, x.ability)
	return f, err
}

// rollEffectSaveD20 rolls the save, or fails it for a target that cannot make it.
func (s *Service) rollEffectSaveD20(ctx context.Context, c *combatTx, x *effectSaveStep) (saved, held bool, err error) {
	f, err := s.readEffectSaveFacts(ctx, c, x)
	if err != nil {
		return false, false, err
	}
	x.run.result = &playv1.EffectSaveResult{Modifier: conv.Int32(f.save.Bonus)}
	if combat.AutoFailsSave(f.creature, x.ability) {
		x.run.result.AutoFail, x.le.Change = true, "failed"
		return false, false, nil
	}
	// A player's character that holds a Bardic Inspiration die has the d20 kept until the player
	// answers (nothing has been written yet); the answer brings it back with the die added. So
	// has a halfling's d20 that shows a 1, for the Lucky question first.
	spec := saveSpec(x.call.m, heldEffectSave, x.reactor, x.call.key, x.msg, x.call.in, f.save.Bonus, f.mode, f.sheet.Traits.Lucky)
	// Indomitable (held_indomitable.go): the dice an effect adds after the gate move the total, so the roller may not
	// know the result yet, as when the table hides the DC.
	extras := len(effectDiceFor(f.states, x.reactor.ID, rules.RollAppliesSave)) > 0 || x.msg.GetManeuverKey() != ""
	spec.Save = &saveRule{DC: int(x.dc), Blind: !c.rules.ShowSaveDC || extras}
	step, err := s.heldGate(ctx, s.inspirationGate(c), spec)
	if err != nil {
		return false, false, err
	}
	if step.Held != nil {
		x.run.offer, x.run.lucky, x.run.held = s.outsideOffers(ctx, c.tx, step.Held)
		return false, true, nil
	}
	own, err := s.maneuverDice(ctx, c.tx, c.session.CampaignID, x.reactor.CharacterID, x.msg.GetManeuverKey(), rules.ManeuverSave, "", x.ability)
	if err != nil {
		return false, false, err
	}
	extra, delta, err := s.rollEffectDice(x.call.in, append(effectDiceFor(f.states, x.reactor.ID, rules.RollAppliesSave), own...), x.call.typedExtra)
	if err != nil {
		return false, false, err
	}
	if err := s.spendOnceEffects(ctx, c, extra); err != nil {
		return false, false, err
	}
	if x.ev.ManeuverSpent, err = s.spendManeuverDice(ctx, c, x.reactor.CharacterID, extra); err != nil {
		return false, false, err
	}
	saved, err = s.recordEffectSaveRoll(ctx, c, x, step, extra, delta)
	return saved, false, err
}

// recordEffectSaveRoll writes what the d20 and the dice showed in the result, the log line and the
// event, and says whether the target saved.
func (s *Service) recordEffectSaveRoll(ctx context.Context, c *combatTx, x *effectSaveStep, step gateResult, extra []effectDie, delta int) (saved bool, err error) {
	d20, result := step.Roll, x.run.result
	if step.Bonus != nil {
		result.InspirationSides, result.InspirationFace = step.Bonus.Sides, step.Bonus.Face
		x.ev.Res = &resourceEvent{Kind: resBardicUse, Sides: step.Bonus.Sides, Face: step.Bonus.Face} // a retry reads the die back from here
	}
	total := d20.Total + delta
	saved = combat.SaveSucceeded(total, int(x.dc))
	for _, f := range d20.Faces {
		result.D20Faces = append(result.D20Faces, clamp32(f, 1, 20))
	}
	result.D20, result.Total, result.Physical, result.Saved = clamp32(d20.Face(), 1, 20), conv.Int32(total), d20.Physical, saved
	nm, err := s.namerFor(ctx, c.tx, c.session.CampaignID)
	if err != nil {
		return false, err
	}
	result.ExtraDice = extraDiceProto(extra, heldViewerIsMaster(ctx, x.call.m), nm)
	x.le.D20, x.le.Total = result.D20, result.Total
	x.le.Change = "failed"
	if saved {
		x.le.Change = "saved"
	}
	x.ev.D20, x.ev.Modifier, x.ev.Total, x.ev.Physical = result.D20, result.Modifier, result.Total, d20.Physical
	// The d20 Lucky rolled again, as an index into D20Faces: the log says "Sortudo: 1 → 14".
	x.ev.Lucky, x.ev.Indomitable, result.Rerolled = step.Lucky, step.Indomitable, rerollsOf(step.Lucky, step.Indomitable)
	return saved, nil
}

// settleEffectSave closes the window, does what the answer does to the effect, and writes the
// event.
func (s *Service) settleEffectSave(ctx context.Context, c *combatTx, x *effectSaveStep, saved bool) (any, error) {
	if heldViewerIsMaster(ctx, x.call.m) || c.rules.ShowSaveDC { // the answering member gets it when the table shows the DC of a save
		dc := x.dc
		x.run.result.Dc = &dc
	}
	// The window closes first: a pass ends the effect, which closes the effect's other windows.
	if _, err := s.closeWindow(ctx, c, x.w, windowAnswered, reaction.ReasonNone, windowOutcome{ByMaster: c.master, Used: true}, ""); err != nil {
		return nil, err
	}
	if err := s.resolveSave(ctx, c, x, saved); err != nil {
		return nil, err
	}
	x.ev.Lasting = x.le
	if err := touch(ctx, c); err != nil {
		return nil, err
	}
	c.characterID = &x.reactor.CharacterID
	x.run.made = x.ev
	return x.ev, nil
}

// resolveSave does what the answered save does: to the effect it is for, or to the creature a zone caught.
func (s *Service) resolveSave(ctx context.Context, c *combatTx, x *effectSaveStep, saved bool) error {
	if x.zone != nil {
		return s.settleZoneSave(ctx, c, x, saved)
	}
	return s.resolveEffectSave(ctx, c, x.cs, x.content, x.def, x.st, x.reactor, x.t.Phase, saved, x.call.skip, &x.run.result)
}

// resolveEffectSave does what an answered saving throw does to its effect: a pass ends it on
// the target (Imobilizar Pessoa, Riso Histérico), a failed save at the start of the turn adds
// what the effect says (the Teia that restrains). A skipped save does nothing.
func (s *Service) resolveEffectSave(ctx context.Context, c *combatTx, cs []playdb.Combatant, content *rules.Content, def *rules.EffectDef, st playdb.CombatantState, target playdb.Combatant, phase string, saved, skipped bool, result **playv1.EffectSaveResult) error {
	if skipped || def == nil {
		return nil
	}
	save := def.EndSave
	if phase == combat.PhaseStart {
		save = def.StartSave
	}
	if save == nil {
		return nil
	}
	r := *result
	switch {
	case saved && save.OnPass == "end":
		r.EffectEnded = true
		r.TextPt = def.NamePT + " acabou."
		if def.NamePT == "" {
			r.TextPt = "O efeito acabou."
		}
		return s.endEffectRows(ctx, c, cs, []playdb.CombatantState{st}, endSaved)
	case !saved && save.OnFail != "":
		next, ok := content.CombatEffect(save.OnFail)
		if !ok {
			return nil
		}
		var caster *playdb.Combatant
		if j := slices.IndexFunc(cs, func(o playdb.Combatant) bool { return o.ID == conv.OrZero(st.SourceID) }); j >= 0 {
			caster = &cs[j]
		}
		made, err := s.addEffects(ctx, c, cs, effectSpec{
			key: save.OnFail, sourceKind: "spell", def: next, caster: caster, group: conv.OrZero(st.GroupID), targets: []playdb.Combatant{target},
			dur: durationSpec{Kind: next.Duration.Kind}, concentration: st.Concentration, dc: st.SaveDc,
		})
		if err != nil {
			return err
		}
		return s.addedEvent(ctx, c, cs, made, save.OnFail)
	}
	return nil
}

// effectDamageSaves opens the saving throw an effect asks again when its target takes damage
// (Hideous Laughter, SRD 5.1: "each time it takes damage", with advantage). The damage the
// effect itself deals does not count.
func (s *Service) effectDamageSaves(ctx context.Context, c *combatTx, target playdb.Combatant, d damageLanded) error {
	rows, err := c.q.ListLastingEffects(ctx, c.enc.ID)
	if err != nil {
		return fmt.Errorf("list the effects: %w", err)
	}
	var content *rules.Content
	group := newGroup()
	for _, st := range rows {
		if st.CombatantID != target.ID || st.EndSaveAbility == nil || d.key == conv.OrZero(st.SourceKey) {
			continue
		}
		if content == nil {
			if content, err = s.contentOf(ctx, c); err != nil {
				return err
			}
		}
		def, ok := s.effectDef(content, conv.OrZero(st.SourceKey))
		if !ok || def.EndSave == nil || !def.EndSave.OnDamage {
			continue
		}
		if _, err := s.openEffectSave(ctx, c, group, st, target, combat.PhaseEnd, true); err != nil {
			return err
		}
	}
	return nil
}

// savedResultOf rebuilds the answer to a saving throw from the event it wrote, for a retry.
func savedResultOf(res combatResult, made actionEvent, showDC bool) *playv1.EffectSaveResult {
	ev, err := resultEvent(res, made)
	if err != nil || ev.Lasting == nil {
		return &playv1.EffectSaveResult{}
	}
	le := ev.Lasting
	out := &playv1.EffectSaveResult{D20: le.D20, Modifier: ev.Modifier, Total: le.Total, Physical: ev.Physical, Saved: le.Change == "saved", Skipped: le.Change == "skipped", EffectEnded: le.Reason == endSaved, Rerolled: ev.rerolls()}
	if ev.Res != nil && ev.Res.Kind == resBardicUse {
		out.InspirationSides, out.InspirationFace = ev.Res.Sides, ev.Res.Face
	}
	if showDC && le.DC != 0 {
		dc := le.DC
		out.Dc = &dc
	}
	return out
}
