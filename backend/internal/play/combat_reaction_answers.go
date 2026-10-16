package play

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dice"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
	"github.com/PuraFome/meuRPG/backend/internal/rules/reaction"
)

// What each reaction does when it is used (SRD 5.1): Uncanny Dodge, Deflect Missiles,
// Cutting Words and Hellish Rebuke. Every one spends the reactor's reaction first; the
// numbers come from package reaction.

// dieRoll rolls one die of the given sides for a reaction, in the app or from the
// face typed from a physical die.
func (s *Service) dieRoll(in rollInput, sides int) (int, dice.Result, error) {
	expr := dice.Expr{Count: 1, Sides: sides}
	if in.inApp {
		r, err := dice.Roll(s.roller, expr)
		if err != nil {
			return 0, r, fmt.Errorf("roll the die: %w", err)
		}
		return r.Faces[0], r, nil
	}
	r, err := dice.Physical(expr, in.typed)
	if err != nil {
		return 0, r, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("typed must be 1 to %d", sides))
	}
	return in.typed, r, nil
}

// reactorKit reads the reactor's kit and refuses a reactor that cannot react now.
func (s *Service) reactorKit(ctx context.Context, c *combatTx, reactor playdb.Combatant) (kit, error) {
	if reactor.ReactionUsed {
		return kit{}, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_REACTION_USED, "the reaction is already used")
	}
	k, ok, err := s.kitOf(ctx, c.tx, c.session.CampaignID, reactor)
	if err != nil {
		return kit{}, err
	}
	if !ok || !k.canReact() {
		return kit{}, connect.NewError(connect.CodeInvalidArgument, errors.New("this combatant cannot react"))
	}
	return k, nil
}

// passWindow answers a window with "Deixar passar".
func (s *Service) passWindow(ctx context.Context, c *combatTx, w playdb.ReactionWindow, reactor playdb.Combatant, got *answered) error {
	t := windowTriggerOf(w)
	got.ev = actionEvent{
		Round: c.enc.Round, Secret: reactor.Hidden, Actor: reactor.ID, Target: t.Actor,
		Reaction: &reactionEvent{Kind: w.Kind, Window: w.ID, ByMaster: c.master},
	}
	got.result = &playv1.ReactionResult{Kind: reactionKindProto[reaction.Kind(w.Kind)], ByMaster: c.master}
	_, err := s.closeWindow(ctx, c, w, windowAnswered, reaction.ReasonNone, windowOutcome{ByMaster: c.master}, "")
	return err
}

// answerUncannyDodge: the damage is halved, rounded down (SRD, Rogue 5).
func (s *Service) answerUncannyDodge(ctx context.Context, c *combatTx, w playdb.ReactionWindow, reactor playdb.Combatant, use bool, got *answered) error {
	if !use {
		return s.passWindow(ctx, c, w, reactor, got)
	}
	if _, err := s.reactorKit(ctx, c, reactor); err != nil {
		return err
	}
	t := windowTriggerOf(w)
	after := clamp32(reaction.UncannyDodge(int(t.Damage)), 0, math.MaxInt32)
	if err := s.spendReaction(ctx, c, reactor); err != nil {
		return err
	}
	if _, err := s.closeWindow(ctx, c, w, windowAnswered, reaction.ReasonNone, windowOutcome{ByMaster: c.master, Used: true, Before: t.Damage, After: after}, ""); err != nil {
		return err
	}
	c.lastAnswered = reaction.UncannyDodgeKind
	got.ev = actionEvent{
		Round: c.enc.Round, Secret: reactor.Hidden, Actor: reactor.ID, Target: t.Actor, ReactionBefore: reactor.ReactionUsed,
		Reaction: &reactionEvent{Kind: w.Kind, Window: w.ID, Used: true, ByMaster: c.master, Before: t.Damage, After: after},
	}
	got.result = &playv1.ReactionResult{
		Kind: playv1.ReactionKind_REACTION_KIND_UNCANNY_DODGE, ByMaster: c.master, Used: true,
		Result: &playv1.ReactionResult_UncannyDodge{UncannyDodge: &playv1.UncannyDodgeResult{DamageBefore: t.Damage, DamageAfter: after}},
	}
	c.characterID = &reactor.CharacterID
	return nil
}

// answerDeflect: the reduction 1d10 + Dexterity + monk level (SRD, Monk 3). At 0 the monk
// catches the missile and may throw it back for 1 ki, as part of the same reaction: the
// window then asks that as its second step.
func (s *Service) answerDeflect(ctx context.Context, c *combatTx, w playdb.ReactionWindow, reactor playdb.Combatant, use bool, req *playv1.AnswerReactionRequest, got *answered) error {
	t := windowTriggerOf(w)
	if w.Step == stepSecond { // the throw back: only "Guardar a flecha" comes through here
		if use {
			return connect.NewError(connect.CodeInvalidArgument, errors.New("the missile is thrown back with RollAttack and catch_window_id"))
		}
		got.ev = actionEvent{Round: c.enc.Round, Secret: reactor.Hidden, Actor: reactor.ID, Reaction: &reactionEvent{Kind: w.Kind, Window: w.ID, Used: true, ByMaster: c.master, Caught: true, Before: t.Damage, After: 0}}
		got.result = &playv1.ReactionResult{Kind: playv1.ReactionKind_REACTION_KIND_DEFLECT_MISSILES, ByMaster: c.master, Used: true}
		_, err := s.closeWindow(ctx, c, w, windowAnswered, reaction.ReasonNone, windowOutcome{ByMaster: c.master, Used: true, Reduction: t.Reduced, Before: t.Damage, After: 0}, "")
		return err
	}
	if !use {
		return s.passWindow(ctx, c, w, reactor, got)
	}
	k, err := s.reactorKit(ctx, c, reactor)
	if err != nil {
		return err
	}
	in, err := s.reactionRoll(ctx, c, req, reaction.DeflectDie, reactor)
	if err != nil {
		return err
	}
	face, r, err := s.dieRoll(in, reaction.DeflectDie)
	if err != nil {
		return err
	}
	left, reduction := reaction.DeflectMissiles(int(t.Damage), face, k.st.DexMod, k.st.MonkLevel)
	if err := s.spendReaction(ctx, c, reactor); err != nil {
		return err
	}
	caught := left == 0
	kiLeft := k.resourceLeft(resKi)
	throwBack := caught && kiLeft > 0
	roll := diceRoll(1, reaction.DeflectDie, []int32{clamp32(face, 1, 20)}, clamp32(k.st.DexMod+k.st.MonkLevel, math.MinInt32, math.MaxInt32), clamp32(reduction, 0, math.MaxInt32), r.Physical)
	got.ev = actionEvent{
		Round: c.enc.Round, Secret: reactor.Hidden, Actor: reactor.ID, Target: t.Actor, ReactionBefore: reactor.ReactionUsed,
		Reaction: &reactionEvent{Kind: w.Kind, Window: w.ID, Used: true, ByMaster: c.master, Before: t.Damage, After: clamp32(left, 0, math.MaxInt32), Die: clamp32(face, 1, 20), DieSides: reaction.DeflectDie, Caught: caught},
	}
	got.result = &playv1.ReactionResult{
		Kind: playv1.ReactionKind_REACTION_KIND_DEFLECT_MISSILES, ByMaster: c.master, Used: true,
		Result: &playv1.ReactionResult_DeflectMissiles{DeflectMissiles: &playv1.DeflectMissilesResult{
			Reduction: roll, DamageBefore: t.Damage, DamageAfter: clamp32(left, 0, math.MaxInt32), Caught: caught, ThrowBackAvailable: throwBack,
		}},
	}
	c.lastAnswered = reaction.DeflectKind
	c.characterID = &reactor.CharacterID
	if throwBack {
		t.Caught, t.Reduced = true, clamp32(reduction, 0, math.MaxInt32)
		t.Damage = clamp32(left, 0, math.MaxInt32) // what the damage is now: 0, and Reduced what came off
		body, err := json.Marshal(t)
		if err != nil {
			return fmt.Errorf("encode the window's trigger: %w", err)
		}
		if _, err := c.q.SetReactionWindowStep(ctx, playdb.SetReactionWindowStepParams{ID: w.ID, Step: stepSecond, Trigger: body}); err != nil {
			return fmt.Errorf("ask for the throw back: %w", err)
		}
		got.result.NextWindowId = w.ID
		return nil
	}
	_, err = s.closeWindow(ctx, c, w, windowAnswered, reaction.ReasonNone, windowOutcome{ByMaster: c.master, Used: true, Reduction: clamp32(reduction, 0, math.MaxInt32), Before: t.Damage, After: clamp32(left, 0, math.MaxInt32)}, reaction.DeflectKind)
	return err
}

// answerCuttingWords: the Bardic Inspiration die comes off the creature's roll (SRD,
// College of Lore). One use of Bardic Inspiration and the reaction are spent whether
// or not it does anything: a creature that cannot hear the bard or is immune to being
// charmed is unaffected, and the answer says only that it had no effect.
func (s *Service) answerCuttingWords(ctx context.Context, c *combatTx, w playdb.ReactionWindow, reactor playdb.Combatant, use bool, req *playv1.AnswerReactionRequest, got *answered) error {
	if !use {
		return s.passWindow(ctx, c, w, reactor, got)
	}
	k, err := s.reactorKit(ctx, c, reactor)
	if err != nil {
		return err
	}
	if !k.st.CuttingWords || k.resourceLeft(resBardic) == 0 {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NO_SLOT, "no Bardic Inspiration use is left")
	}
	t := windowTriggerOf(w)
	sides := reaction.CuttingWordsDie(k.st.BardLevel)
	in, err := s.reactionRoll(ctx, c, req, sides, reactor)
	if err != nil {
		return err
	}
	face, r, err := s.dieRoll(in, sides)
	if err != nil {
		return err
	}
	vit, err := s.spendReactionResource(ctx, c, reactor, resBardic)
	if err != nil {
		return err
	}
	if vit != nil {
		got.vitals = append(got.vitals, vit)
	}
	if err := s.spendReaction(ctx, c, reactor); err != nil {
		return err
	}
	effective, err := s.canBeCut(ctx, c, t.Actor)
	if err != nil {
		return err
	}
	reduction := int32(0)
	if effective {
		reduction = clamp32(face, 1, math.MaxInt32)
	}
	re := &reactionEvent{
		Kind: w.Kind, Window: w.ID, Used: true, ByMaster: c.master, Roll: t.Roll, Die: clamp32(face, 1, 20), DieSides: clamp32(sides, 1, 20), Ineffect: !effective,
	}
	got.ev = actionEvent{Round: c.enc.Round, Secret: reactor.Hidden, Actor: reactor.ID, Target: t.Actor, ReactionBefore: reactor.ReactionUsed, Reaction: re}
	c.answered = append(c.answered, answeredRef{window: w.ID, ev: re})
	if _, err := s.closeWindow(ctx, c, w, windowAnswered, reaction.ReasonNone, windowOutcome{ByMaster: c.master, Used: true, Reduction: reduction, Ineffect: !effective}, ""); err != nil {
		return err
	}
	c.lastAnswered = reaction.CuttingWords
	rk := playv1.ReactionRollKind_REACTION_ROLL_KIND_ATTACK
	switch t.Roll {
	case "damage":
		rk = playv1.ReactionRollKind_REACTION_ROLL_KIND_DAMAGE
	case "test":
		rk = playv1.ReactionRollKind_REACTION_ROLL_KIND_TEST
	}
	cw := &playv1.CuttingWordsResult{Die: diceRoll(1, int32(sides), []int32{clamp32(face, 1, 20)}, 0, clamp32(face, 1, 20), r.Physical), Effective: effective, RollKind: rk} //nolint:gosec // a die size
	got.result = &playv1.ReactionResult{Kind: playv1.ReactionKind_REACTION_KIND_CUTTING_WORDS, ByMaster: c.master, Used: true, Result: &playv1.ReactionResult_CuttingWords{CuttingWords: cw}}
	// What the roll became is known when the held action is replayed, a moment later.
	got.fill = func() {
		switch re.Outcome {
		case outcomeHit, outcomeCrit:
			cw.Outcome = playv1.AttackOutcome_ATTACK_OUTCOME_HIT
		case outcomeMiss:
			cw.Outcome = playv1.AttackOutcome_ATTACK_OUTCOME_MISS
		}
	}
	c.characterID = &reactor.CharacterID
	return nil
}

// canBeCut says Cutting Words can change the roller's roll: it can hear the bard and is
// not immune to being charmed (SRD, College of Lore).
func (s *Service) canBeCut(ctx context.Context, c *combatTx, rollerID string) (bool, error) {
	cs, err := c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return false, fmt.Errorf("list the combatants: %w", err)
	}
	i := slices.IndexFunc(cs, func(x playdb.Combatant) bool { return x.ID == rollerID })
	if i < 0 {
		return false, nil
	}
	return s.rollerCanBeCut(ctx, c, cs[i])
}

// rollerCanBeCut is canBeCut for a combatant already read.
func (s *Service) rollerCanBeCut(ctx context.Context, c *combatTx, roller playdb.Combatant) (bool, error) {
	if slices.Contains(roller.Conditions, "condition:deafened") {
		return false, nil // it cannot hear the bard (SRD, College of Lore)
	}
	k, ok, err := s.kitOf(ctx, c.tx, c.session.CampaignID, roller)
	if err != nil {
		return false, err
	}
	return !ok || !k.st.CharmImmune, nil
}

// answerHellishRebuke: the aggressor makes a Dexterity saving throw against the caster's DC
// and takes 2d10 fire, half as much on a success, and 1d10 more for each slot level above
// the 1st (SRD, Hellish Rebuke). The tiefling's Infernal Legacy casts it as a 2nd-level
// spell once a long rest. The aggressor's saving throw is the master's when it is an NPC:
// the window then asks it as its second step.
func (s *Service) answerHellishRebuke(ctx context.Context, c *combatTx, m authz.Membership, w playdb.ReactionWindow, reactor playdb.Combatant, use bool, req *playv1.AnswerReactionRequest, got *answered) error {
	t := windowTriggerOf(w)
	if w.Step == stepSecond { // the aggressor's saving throw, the master's
		if !use {
			return connect.NewError(connect.CodeInvalidArgument, errors.New("the saving throw is rolled: use roll_in_app or typed"))
		}
		if m.Role != authz.RoleMaster {
			return errWindowDenied()
		}
		return s.resolveRebuke(ctx, c, w, reactor, t, req, got)
	}
	if !use {
		return s.passWindow(ctx, c, w, reactor, got)
	}
	k, err := s.reactorKit(ctx, c, reactor)
	if err != nil {
		return err
	}
	level := int32(0)
	racial := req.GetUseRacial()
	var slot *slotRef
	if racial {
		if k.legacyLeft() == 0 {
			return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NO_USES, "the Infernal Legacy has no use left")
		}
		level = reaction.InfernalLegacyLevel
	} else {
		sl, err := pickSlot(req.GetSlot(), k.slotsFor(spellHellishRebuke))
		if err != nil {
			return err
		}
		slot, level = &sl, sl.Level
	}
	if racial {
		vit, err := s.spendReactionResource(ctx, c, reactor, resInfernalLegacy)
		if err != nil {
			return err
		}
		if vit != nil {
			got.vitals = append(got.vitals, vit)
		}
	} else {
		vit, err := s.spendReactionSlot(ctx, c, reactor, *slot)
		if err != nil {
			return err
		}
		if vit != nil {
			got.vitals = append(got.vitals, vit)
		}
	}
	if err := s.spendReaction(ctx, c, reactor); err != nil {
		return err
	}
	t.CastLevel, t.Racial, t.Dice = level, racial, int32(reaction.HellishRebukeDice(int(level))) //nolint:gosec // 2 to 10 dice
	c.lastAnswered = reaction.HellishRebukeKind
	c.characterID = &reactor.CharacterID
	cs, err := c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return fmt.Errorf("list the combatants: %w", err)
	}
	ai := slices.IndexFunc(cs, func(x playdb.Combatant) bool { return x.ID == t.Actor })
	if ai >= 0 && cs[ai].Kind != kindPlayer {
		// The aggressor is an NPC: its saving throw is the master's. The window waits.
		body, err := json.Marshal(t)
		if err != nil {
			return fmt.Errorf("encode the window's trigger: %w", err)
		}
		if _, err := c.q.SetReactionWindowStep(ctx, playdb.SetReactionWindowStepParams{ID: w.ID, Step: stepSecond, Trigger: body}); err != nil {
			return fmt.Errorf("ask for the aggressor's saving throw: %w", err)
		}
		got.ev = actionEvent{
			Round: c.enc.Round, Secret: reactor.Hidden, Actor: reactor.ID, Target: t.Actor, Slot: slot, ReactionBefore: reactor.ReactionUsed,
			Reaction: &reactionEvent{Kind: w.Kind, Window: w.ID, Used: true, ByMaster: c.master, Slot: slot, Racial: racial},
		}
		got.result = &playv1.ReactionResult{Kind: playv1.ReactionKind_REACTION_KIND_HELLISH_REBUKE, ByMaster: c.master, Used: true, NextWindowId: w.ID}
		return nil
	}
	got.ev = actionEvent{Round: c.enc.Round, Secret: reactor.Hidden, Actor: reactor.ID, Target: t.Actor, Slot: slot, ReactionBefore: reactor.ReactionUsed}
	return s.resolveRebuke(ctx, c, w, reactor, t, &playv1.AnswerReactionRequest{Roll: &playv1.AnswerReactionRequest_RollInApp{RollInApp: true}}, got)
}

// resolveRebuke rolls the aggressor's saving throw and puts the fire on it.
func (s *Service) resolveRebuke(ctx context.Context, c *combatTx, w playdb.ReactionWindow, reactor playdb.Combatant, t windowTrigger, req *playv1.AnswerReactionRequest, got *answered) error {
	cs, err := c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return fmt.Errorf("list the combatants: %w", err)
	}
	ai := slices.IndexFunc(cs, func(x playdb.Combatant) bool { return x.ID == t.Actor })
	if ai < 0 {
		return connect.NewError(connect.CodeFailedPrecondition, errors.New("the aggressor is gone"))
	}
	aggressor := cs[ai]
	k, _, err := s.kitOf(ctx, c.tx, c.session.CampaignID, reactor)
	if err != nil {
		return err
	}
	dc := k.st.SaveDC
	if t.Racial {
		dc = k.st.LegacyDC
	}
	save, err := s.saveOf(ctx, c.tx, c.session.CampaignID, aggressor, "dex")
	if err != nil {
		return err
	}
	in := rollInput{inApp: true}
	if aggressor.Kind != kindPlayer {
		if in, err = s.reactionRoll(ctx, c, req, 20, aggressor); err != nil {
			return err
		}
	}
	lucky, err := s.rebukeLucky(ctx, c, aggressor)
	if err != nil {
		return err
	}
	face, rolled, err := s.d20(in, save.Bonus)
	if err != nil {
		return err
	}
	// Nobody is asked about a player's save here: a halfling's 1 is rolled again by the app (combat_lucky.go).
	roll, reroll, err := s.luckySave(d20Roll{Faces: []int{face}, Total: rolled.Total, Physical: rolled.Physical}, save.Bonus, combat.ModeNormal, lucky)
	if err != nil {
		return err
	}
	face = roll.Face()
	saved := combat.SaveSucceeded(roll.Total, dc)
	// The fire: the server rolls it in the app (2d10 and a d10 for each slot level above the 1st).
	fire, err := dice.Roll(s.roller, dice.Expr{Count: int(t.Dice), Sides: hellishDieSides})
	if err != nil {
		return fmt.Errorf("roll the fire: %w", err)
	}
	p, err := c.q.InsertPendingDamage(ctx, playdb.InsertPendingDamageParams{
		EncounterID: c.enc.ID, AttackerID: &reactor.ID, TargetID: aggressor.ID, AttackKey: spellHellishRebuke, Status: pendingAwaitingRoll,
		DiceCount: t.Dice, DiceSides: hellishDieSides, DamageType: "damage-type:fire", CreatedAt: c.now, Half: saved,
	})
	if err != nil {
		return fmt.Errorf("open the fire's damage: %w", err)
	}
	hit, vit, err := s.landRolled(ctx, c, p, aggressor, fire)
	if err != nil {
		return err
	}
	if vit != nil {
		got.vitals = append(got.vitals, vit)
	}
	re := got.ev.Reaction
	if re == nil {
		re = &reactionEvent{Kind: w.Kind, Window: w.ID, Used: true, ByMaster: c.master, Racial: t.Racial}
	}
	re.Slot, re.Racial = got.ev.Slot, t.Racial
	re.SaveD20, re.SaveBonus, re.SaveDC, re.Saved, re.Fire = clamp32(face, 1, 20), clamp32(save.Bonus, math.MinInt32, math.MaxInt32), clamp32(dc, 0, math.MaxInt32), saved, hit.Amount
	re.Level = t.CastLevel
	got.ev.Reaction = re
	got.ev.Lucky = reroll // the aggressor's save the app rolled again: the master's line says "Sortudo: 1 → 14"
	got.ev.Actor, got.ev.Target, got.ev.Round, got.ev.Secret = reactor.ID, aggressor.ID, c.enc.Round, reactor.Hidden || aggressor.Hidden
	if _, err := s.closeWindow(ctx, c, w, windowAnswered, reaction.ReasonNone, windowOutcome{ByMaster: c.master, Used: true, Slot: got.ev.Slot, Racial: t.Racial}, ""); err != nil {
		return err
	}
	got.result = &playv1.ReactionResult{
		Kind: playv1.ReactionKind_REACTION_KIND_HELLISH_REBUKE, ByMaster: c.master, Used: true,
		Result: &playv1.ReactionResult_HellishRebuke{HellishRebuke: &playv1.HellishRebukeResult{
			Save:   rebukeSaveRoll(face, re.SaveBonus, roll, reroll),
			SaveDc: re.SaveDC, Saved: saved, Damage: hit.Amount, Level: t.CastLevel, Racial: t.Racial,
		}},
	}
	return nil
}

// rebukeLucky says the app rolls a 1 again on the aggressor's Dexterity save against Hellish
// Rebuke: a halfling player's character (luckySaver). An NPC's save is the master's, and its sheet
// is not read.
func (s *Service) rebukeLucky(ctx context.Context, c *combatTx, aggressor playdb.Combatant) (bool, error) {
	if aggressor.Kind != kindPlayer {
		return false, nil
	}
	sheet, err := s.sheetOf(ctx, c.tx, c.session.CampaignID, aggressor)
	if err != nil {
		return false, err
	}
	return luckySaver(aggressor, sheet.Traits), nil
}

// rebukeSaveRoll is the aggressor's save against Hellish Rebuke as the answer shows it, with the d20
// the app rolled again (Lucky).
func rebukeSaveRoll(face int, bonus int32, roll d20Roll, reroll *luckyReroll) *playv1.DiceRoll {
	out := diceRoll(1, 20, []int32{clamp32(face, 1, 20)}, bonus, clamp32(roll.Total, math.MinInt32, math.MaxInt32), roll.Physical)
	out.Rerolled = reroll.proto()
	return out
}

// landRolled lands the roll of a pending damage the server made itself (the fire of a
// Hellish Rebuke): the resistances of its target, the hit points or, for a player's
// character, the master's apply (RN-02), as RollDamage does for its own.
func (s *Service) landRolled(ctx context.Context, c *combatTx, p playdb.PendingDamage, target playdb.Combatant, roll dice.Result) (damageHit, *playv1.CharacterVitals, error) {
	total := max(roll.Total, 0)
	amount := clamp32(total, 0, math.MaxInt32)
	if p.Half {
		amount = clamp32(combat.HalfDamage(total), 0, math.MaxInt32)
	}
	amount, err := s.afterResistance(ctx, c, target, p.DamageType, amount)
	if err != nil {
		return damageHit{}, nil, err
	}
	hit, vit, err := s.landDamage(ctx, c, p, target, amount)
	if err != nil {
		return hit, nil, err
	}
	status := pendingRolled
	if hit.Applied {
		status = pendingApplied
	}
	rolledTotal := clamp32(total, 0, math.MaxInt32)
	if _, err := c.q.SetPendingDamageRolled(ctx, playdb.SetPendingDamageRolledParams{
		ID: p.ID, Status: status, Faces: faces32(roll.Faces), Physical: roll.Physical, Amount: &amount, ResolvedAt: resolvedAt(status, c), RollTotal: &rolledTotal,
	}); err != nil {
		return hit, nil, fmt.Errorf("save the damage roll: %w", err)
	}
	return hit, vit, s.afterDamage(ctx, c, target, damageLanded{attacker: deref(p.AttackerID), key: p.AttackKey, pending: p, hit: hit})
}

// deflectThrow is the attack a monk makes with the missile it caught (SRD, Monk 3,
// Deflect Missiles): a ranged attack with proficiency, range 20/60 and the missile's
// own damage dice, as part of the reaction already spent. It returns the attack to
// roll, and the window that the throw closes.
func (s *Service) deflectThrow(ctx context.Context, c *combatTx, raw string, thrower playdb.Combatant, cs []playdb.Combatant) (link.Attack, playdb.ReactionWindow, error) {
	var none link.Attack
	id, err := parseCombatID(raw, "window")
	if err != nil {
		return none, playdb.ReactionWindow{}, err
	}
	w, err := c.q.GetReactionWindowForUpdate(ctx, playdb.GetReactionWindowForUpdateParams{EncounterID: c.enc.ID, ID: id})
	if err != nil || w.Status != windowOpen || w.Kind != string(reaction.DeflectKind) || w.Step != 2 || w.ReactorID == nil || *w.ReactorID != thrower.ID {
		return none, w, errNotYourTurnToAnswer()
	}
	t := windowTriggerOf(w)
	i := slices.IndexFunc(cs, func(x playdb.Combatant) bool { return x.ID == t.Actor })
	if i < 0 {
		return none, w, errNotYourTurnToAnswer()
	}
	sheet, err := s.sheetOf(ctx, c.tx, c.session.CampaignID, cs[i])
	if err != nil {
		return none, w, err
	}
	j := slices.IndexFunc(sheet.Attacks, func(a link.Attack) bool { return a.Key == t.Key })
	if j < 0 {
		return none, w, errNotYourTurnToAnswer()
	}
	k, ok, err := s.kitOf(ctx, c.tx, c.session.CampaignID, thrower)
	if err != nil {
		return none, w, err
	}
	if !ok || k.resourceLeft(resKi) < 1 {
		return none, w, connect.NewError(connect.CodeFailedPrecondition, errors.New("no ki left to throw the missile back"))
	}
	a := sheet.Attacks[j]
	a.Key, a.Spell, a.Save, a.Melee, a.Beams = t.Key, false, false, false, 0
	a.ToHit = k.st.Proficiency + k.st.DexMod
	a.DiceBonus = k.st.DexMod
	a.AbilityMod = k.st.DexMod
	a.RangeFt, a.LongRangeFt = 20, 60
	return a, w, nil
}
