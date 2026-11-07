package play

import (
	"context"
	"fmt"
	"math"
	"slices"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/reaction"
)

// Parry (SRD 5.1, the Reactions of the Bandit Captain, Erinyes, Gladiator, Knight, Marilith and Noble): the
// creature adds a number to its armor class against one melee attack that would hit it, if it sees the
// attacker and wields a melee weapon. The SRD says "melee attack"; the app offers melee weapon attacks only
// (its own rule: a melee spell attack is not offered). By default the server decides the Parry on its own, with no
// window (combat_parry_auto.go). When the master switched the creature to "Perguntar" (SetMonsterReactionMode), a melee
// weapon attack that hits it waits (AWAITING_REACTION) in a window of its own kind, answered by the master for the
// creature, as the Shield of an NPC is. The d20 is kept: the answer compares the stored total with the armor class
// plus the bonus.
//
// The app decides what the SRD leaves out, and says so: it cannot tell what a creature wields, so the server takes it
// that it wields a melee weapon. In "Perguntar" the window opens for every hit that is not a critical one, also when
// the bonus would not change the outcome. The bonus is not kept on the creature: it holds for that one attack.

// parryOf is the Parry of a stat block, when it has one that adds a number to the armor class.
func parryOf(st link.ReactionStats) (link.MonsterReaction, bool) {
	i := slices.IndexFunc(st.MonsterReactions, func(r link.MonsterReaction) bool { return r.Kind == rules.ReactionKindParry && r.ACBonus > 0 })
	if i < 0 {
		return link.MonsterReaction{}, false
	}
	return st.MonsterReactions[i], true
}

// parryWindows lists the window a hit opens for the Parry of its target: the target is an NPC the master asked to
// be asked for (the others parry on their own, autoParry) that can react, has the reaction in its stat block, sees
// the attacker, and the attack is a melee weapon attack. The kit is the target's, already known to be able to react.
func (s *Service) parryWindows(ctx context.Context, c *combatTx, k kit, attacker playdb.Combatant, trigger windowTrigger) ([]windowSpec, error) {
	p, ok := parryOf(k.st)
	if !ok || k.who.Kind != kindNPC || !parryAsks(k.who) {
		return nil, nil
	}
	melee, err := s.meleeWeaponUse(ctx, c, attacker, k.who, trigger.Key)
	if err != nil || !melee {
		return nil, err
	}
	cs, err := listCombatants(ctx, c)
	if err != nil {
		return nil, err
	}
	_, sees, err := s.reactorSees(ctx, c, cs, k.who, attacker)
	if err != nil || !sees {
		return nil, err
	}
	trigger.Bonus = clamp32(p.ACBonus, 0, math.MaxInt32)
	target := k.who
	return []windowSpec{{kind: reaction.ParryKind, reactor: &target, trigger: trigger}}, nil
}

// meleeWeaponUse says an attack of the attacker's sheet is used as a melee weapon attack on the target: a melee
// weapon (not a spell), and not thrown from beyond its reach.
func (s *Service) meleeWeaponUse(ctx context.Context, c *combatTx, attacker, target playdb.Combatant, key string) (bool, error) {
	sheet, err := s.sheetOf(ctx, c.tx, c.session.CampaignID, attacker)
	if connect.CodeOf(err) == connect.CodeNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	i := slices.IndexFunc(sheet.Attacks, func(a link.Attack) bool { return a.Key == key })
	if i < 0 || !sheet.Attacks[i].Melee || sheet.Attacks[i].Spell || sheet.Attacks[i].Save {
		return false, nil
	}
	dist, both := distanceFt(attacker, target)
	return !shapeOfAttack(sheet.Attacks[i]).rangedAt(dist, both && !isTheatre(c.enc)), nil
}

// answerParry answers a Parry window: "Usar" adds the bonus to the armor class against this hit, which is compared
// again with the total it was rolled with; "Deixar passar" lets the hit go on.
func (s *Service) answerParry(ctx context.Context, c *combatTx, w playdb.ReactionWindow, reactor playdb.Combatant, use bool, got *answered) error {
	if w.PendingDamageID == nil {
		return fmt.Errorf("the parry window %s holds no hit", w.ID)
	}
	p, err := c.q.GetPendingDamage(ctx, playdb.GetPendingDamageParams{EncounterID: c.enc.ID, ID: *w.PendingDamageID})
	if err != nil {
		return fmt.Errorf("find the pending damage: %w", err)
	}
	cs, err := listCombatants(ctx, c)
	if err != nil {
		return err
	}
	attacker, err := findCombatant(cs, conv.OrZero(p.AttackerID), combatViewer{master: true})
	if err != nil {
		// Removing the attacker deletes its pending damage (ON DELETE CASCADE), so a window never outlives it;
		// if it did, the answer goes on and the line names nobody.
		attacker = playdb.Combatant{}
	}
	if !use {
		return s.passParry(ctx, c, w, p, reactor, got)
	}
	return s.useParry(ctx, c, w, p, attacker, reactor, got)
}

// passParry lets the hit go on: the window is answered and the damage goes to its roll.
func (s *Service) passParry(ctx context.Context, c *combatTx, w playdb.ReactionWindow, p playdb.PendingDamage, reactor playdb.Combatant, got *answered) error {
	if _, err := s.closeWindow(ctx, c, w, windowAnswered, reaction.ReasonNone, windowOutcome{ByMaster: c.master}, ""); err != nil {
		return err
	}
	got.ev = actionEvent{
		Round: c.enc.Round, Secret: reactor.Hidden, Actor: reactor.ID, Pending: p.ID, PrevStatus: p.Status,
		Reaction: &reactionEvent{Kind: string(reaction.ParryKind), Window: w.ID, ByMaster: c.master},
	}
	got.result = &playv1.ReactionResult{Kind: playv1.ReactionKind_REACTION_KIND_PARRY, ByMaster: c.master}
	c.characterID = &reactor.CharacterID
	return nil
}

// useParry spends the reaction and compares the hit again: the total never changes (the d20 is not rolled
// again), the armor class is the one the roll was compared with plus the bonus.
func (s *Service) useParry(ctx context.Context, c *combatTx, w playdb.ReactionWindow, p playdb.PendingDamage, attacker, reactor playdb.Combatant, got *answered) error {
	k, err := s.reactorKit(ctx, c, reactor)
	if err != nil {
		return err
	}
	parry, ok := parryOf(k.st)
	if !ok {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("this combatant has no Parry"))
	}
	bonus := clamp32(parry.ACBonus, 0, math.MaxInt32)
	base, err := s.parryBaseAC(ctx, c, p, reactor)
	if err != nil {
		return err
	}
	total := conv.OrZero(p.AttackTotal)
	stopped := total < base+bonus
	if err := s.spendReaction(ctx, c, reactor, reaction.ParryKind); err != nil {
		return err
	}
	next := pendingAwaitingRoll
	if stopped {
		next = pendingDiscarded
	}
	if _, err := c.q.SetPendingDamageStatus(ctx, playdb.SetPendingDamageStatusParams{ID: p.ID, Status: next, ResolvedAt: resolvedWhen(next, c)}); err != nil {
		return fmt.Errorf("answer the pending damage: %w", err)
	}
	if _, err := s.closeWindow(ctx, c, w, windowAnswered, reaction.ReasonNone, windowOutcome{Used: true, Stopped: stopped, ByMaster: c.master}, ""); err != nil {
		return err
	}
	c.lastAnswered = reaction.ParryKind
	c.characterID = &reactor.CharacterID
	got.kind = eventReactionUsed
	got.ev = actionEvent{
		Round: c.enc.Round, Secret: reactor.Hidden, AttackerHidden: attacker.Hidden, Actor: reactor.ID, Target: attacker.ID, Pending: p.ID,
		Stopped: stopped, ReactionBefore: reactor.ReactionUsed, ACBonusBefore: reactor.AcBonus, PrevStatus: p.Status,
		Reaction: &reactionEvent{Kind: string(reaction.ParryKind), Window: w.ID, Used: true, Stopped: stopped, ByMaster: c.master, Before: total, AC: base, Bonus: bonus},
	}
	got.result = &playv1.ReactionResult{
		Kind: playv1.ReactionKind_REACTION_KIND_PARRY, ByMaster: c.master, Used: true,
		Result: &playv1.ReactionResult_Parry{Parry: &playv1.ParryResult{Stopped: stopped}},
	}
	return nil
}

// parryBaseAC is the armor class the hit was compared with (cover and an active Shield included). A damage with
// no stored armor class falls back to the sheet's.
func (s *Service) parryBaseAC(ctx context.Context, c *combatTx, p playdb.PendingDamage, target playdb.Combatant) (int32, error) {
	if p.AttackArmorClass != nil {
		return *p.AttackArmorClass, nil
	}
	sheet, err := s.sheetOf(ctx, c.tx, c.session.CampaignID, target)
	if err != nil {
		return 0, err
	}
	return clamp32(sheet.ArmorClass+int(target.AcBonus), 0, math.MaxInt32), nil
}

// parryPrompt is the Parry prompt of a window. Only the master reads it, since an NPC reactor is the master's:
// the attacker, the bonus and the armor class with it (the total and the armor class of the hit are in the trigger).
func (wv *windowView) parryPrompt(w playdb.ReactionWindow, out *playv1.ReactionWindow) {
	t := windowTriggerOf(w)
	p := &playv1.ParryPrompt{PendingDamageId: conv.OrZero(w.PendingDamageID), AcBonus: t.Bonus}
	p.AttackerLabel, p.AttackNamePt = wv.attackerOf(t)
	if wv.v.master {
		p.ArmorClassWithParry = clamp32(int(t.AC)+int(t.Bonus), 0, math.MaxInt32)
	}
	out.Prompt = &playv1.ReactionWindow_Parry{Parry: p}
}

// parry is the line of a Parry used. The master reads the numbers. A player reads a line only for a Parry the server
// used on its own that turned a hit into a miss ("X aparou o ataque."), from a reactor they see and an attacker they
// see; a Parry answered in a window, and one that changed nothing, are the master's alone.
func (f *reactionFacts) parry() (string, bool) {
	re := f.re
	if !re.Used {
		return "", false
	}
	if !f.v.master {
		if re.Auto && re.Stopped && f.seesReactor && !f.e.ev.AttackerHidden {
			return f.who + " aparou o ataque.", true
		}
		return "", false
	}
	verdict := "o ataque ainda acertou"
	if re.Stopped {
		verdict = "o ataque errou"
	}
	return fmt.Sprintf("%s aparou (+%d CA): %d contra CA %d, %s.", f.who, re.Bonus, re.Before, re.AC+re.Bonus, verdict), true
}
