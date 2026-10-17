package play

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
	"github.com/PuraFome/meuRPG/backend/internal/rules/reaction"
)

// Reactions (MR-014, timeline decision 5, Etapa 6, slice 6.4b). Escudo is the
// one reaction spell the combat runs: it is cast when an attack hits a player's
// character, so the hit waits (AWAITING_REACTION) until the target's player, or
// the master, answers. The opportunity attack is RollAttack's as_reaction
// (combat_actions.go). Every other reaction spell stays the table's.

// shield is the reaction spell the combat knows.
const shield = "spell:shield"

// playerAgainstEnemy says an action of a player's character is aimed at an enemy
// NPC: the actions the table rule "Reações dos inimigos: Sempre" makes the master
// answer with a tap, so that a pause says nothing about who can react (RN-10).
func playerAgainstEnemy(actor, target playdb.Combatant) bool {
	return actor.Kind == kindPlayer && target.Kind == kindNPC && target.Side != actor.Side
}

// hitWindows lists the windows a hit opens before its damage: the target's Shield
// when it can cast it (the table keeps the prompt for the hits it can stop, so not
// for a critical hit, which hits whatever the armor class), and, when the table
// plays "Sempre", the master's check of a player's hit on an enemy that nobody
// answers otherwise. total and ac are the attack's, for the master's card.
func (s *Service) hitWindows(ctx context.Context, c *combatTx, attacker, target playdb.Combatant, key string, critical, magic bool, total, ac int32) ([]windowSpec, error) {
	trigger := windowTrigger{Actor: attacker.ID, Target: target.ID, Key: key, Total: total, AC: ac, Crit: critical, Magic: magic}
	if d, ok := distanceFt(attacker, target); ok {
		trigger.Distance = d
	}
	if !critical || magic {
		k, ok, err := s.kitOf(ctx, c.tx, c.session.CampaignID, target)
		if err != nil {
			return nil, err
		}
		if ok && k.canReact() && len(k.slotsFor(spellShield)) > 0 {
			return []windowSpec{{kind: reaction.Shield, reactor: &target, trigger: trigger}}, nil
		}
	}
	if c.rules.EnemyReactionsAlways && playerAgainstEnemy(attacker, target) {
		return []windowSpec{{kind: reaction.MasterCheck, trigger: trigger}}, nil
	}
	return nil, nil
}

// openPendingWindows opens the windows of a hit on the pending damage it holds.
func (s *Service) openPendingWindows(ctx context.Context, c *combatTx, p playdb.PendingDamage, specs []windowSpec) error {
	for i := range specs {
		specs[i].pending = p.ID
		specs[i].trigger.Pending = p.ID
	}
	_, err := s.openWindows(ctx, c, newGroup(), nil, specs)
	return err
}

// openHit opens the pending damage of a hit, an attack's or a spell attack's:
// to be rolled, or, when a reaction window waits on it (Escudo, or the master's
// check), to wait for the answers first. attackTotal is kept for the new comparison.
func (s *Service) openHit(ctx context.Context, c *combatTx, attacker, target playdb.Combatant, key string, dmg link.Dice, critical bool, extraDice, savageDice, attackTotal, attackAC int) (playdb.PendingDamage, error) {
	total := clamp32(attackTotal, math.MinInt32, math.MaxInt32)
	specs, err := s.hitWindows(ctx, c, attacker, target, key, critical, false, total, clamp32(attackAC, 0, math.MaxInt32))
	if err != nil {
		return playdb.PendingDamage{}, err
	}
	status := pendingAwaitingRoll
	if len(specs) > 0 {
		status = pendingAwaitingReaction
	}
	// The table's critical rule (RN-24), read in this transaction: the dice to
	// roll, doubled or not, and the maximum that comes without rolling. A hit that
	// is not a critical one rolls its dice as they are.
	count, fixed := combat.CriticalDice(rules.DiceFormula{Count: dmg.Count, Sides: dmg.Sides}, critical, criticalRuleOf(c.rules))
	p, err := c.q.InsertPendingDamage(ctx, playdb.InsertPendingDamageParams{
		EncounterID: c.enc.ID, AttackerID: &attacker.ID, TargetID: target.ID, AttackKey: key, Status: status, Critical: critical,
		DiceCount: clamp32(count, 0, 100), CriticalMax: clamp32(fixed, 0, 10000), CriticalMaxRule: critical && c.rules.CriticalMaxPlusRoll,
		DiceSides: clamp32(dmg.Sides, 0, 100), DiceBonus: clamp32(dmg.Bonus, -1000, 1000),
		DamageType: dmg.DamageType, CreatedAt: c.now, AttackTotal: &total, AttackArmorClass: new(clamp32(attackAC, 0, math.MaxInt32)),
		ExtraDice: clamp32(extraDice, 0, maxExtraDice), SavageDice: clamp32(savageDice, 0, 1),
	})
	if err != nil {
		return playdb.PendingDamage{}, fmt.Errorf("open the pending damage: %w", err)
	}
	if err := s.openPendingWindows(ctx, c, p, specs); err != nil {
		return playdb.PendingDamage{}, err
	}
	return p, nil
}

// reactionPrompts lists the hits that wait for a Shield as the caller may see them
// (the prompts of Encounter.reaction_prompts, kept beside the windows): the master
// all, a player only those on their own character. A player never gets the attack's
// total or an armor class, nor who attacked when the attacker is hidden from them.
func (s *Service) reactionPrompts(ctx context.Context, m authz.Membership, d *encounterData, v combatViewer, names func(key string) string) ([]*playv1.ReactionPrompt, error) {
	if d.enc.Status != statusActive {
		return nil, nil
	}
	open, err := s.queries.ListOpenReactionWindows(ctx, d.enc.ID)
	if err != nil {
		return nil, s.dbError(ctx, "list the reaction windows", err)
	}
	var out []*playv1.ReactionPrompt
	for _, w := range open {
		prompt, ok, err := s.shieldPrompt(ctx, m, d, v, names, w)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, prompt)
		}
	}
	return out, nil
}

// shieldPrompt is the prompt of one open window, when it is a Shield window the viewer may read.
func (s *Service) shieldPrompt(ctx context.Context, m authz.Membership, d *encounterData, v combatViewer, names func(key string) string, w playdb.ReactionWindow) (*playv1.ReactionPrompt, bool, error) {
	if w.Kind != string(reaction.Shield) || w.PendingDamageID == nil || w.ReactorID == nil {
		return nil, false, nil
	}
	i := slices.IndexFunc(d.cs, func(c playdb.Combatant) bool { return c.ID == *w.ReactorID })
	if i < 0 || (!v.master && !v.owns(d.cs[i])) {
		return nil, false, nil
	}
	k, ok, err := s.kitOf(ctx, nil, m.CampaignID, d.cs[i])
	if err != nil {
		return nil, false, s.dbError(ctx, "work out the reaction", err)
	}
	if !ok {
		return nil, false, nil
	}
	t := windowTriggerOf(w)
	prompt := &playv1.ReactionPrompt{PendingDamageId: *w.PendingDamageID, TargetId: *w.ReactorID, SpellKey: shield, Slots: k.slotsFor(spellShield), SpellNamePt: names(shield)}
	if v.master {
		prompt.AttackerId = t.Actor
	}
	s.nameShieldAttacker(ctx, m, d, v, t, prompt)
	return prompt, true, nil
}

// nameShieldAttacker says who attacked, and with what, only when the viewer sees the attacker
// (RN-20): the screen says "Capitão Goblin · Cimitarra", and a hidden attacker stays hidden.
func (s *Service) nameShieldAttacker(ctx context.Context, m authz.Membership, d *encounterData, v combatViewer, t windowTrigger, prompt *playv1.ReactionPrompt) {
	j := slices.IndexFunc(d.cs, func(c playdb.Combatant) bool { return c.ID == t.Actor })
	if j < 0 || !v.sees(d.cs[j]) {
		return
	}
	prompt.AttackerLabel = d.cs[j].Label
	sheet, err := s.sheetOf(ctx, nil, m.CampaignID, d.cs[j])
	if err != nil {
		return
	}
	if k := slices.IndexFunc(sheet.Attacks, func(a link.Attack) bool { return a.Key == t.Key }); k >= 0 {
		prompt.AttackNamePt = sheet.Attacks[k].Name
	}
}

// reactionTarget finds the Shield window of a hit that waits for a reaction, the
// pending damage and the combatant it hit, inside the change's transaction, for the
// caller to answer: not_found for a pending damage the caller may not see,
// permission_denied for another player's character, NOT_AWAITING_REACTION for one
// that does not wait.
func (s *Service) reactionTarget(ctx context.Context, c *combatTx, v combatViewer, pendingID string) (w playdb.ReactionWindow, p playdb.PendingDamage, attacker, target playdb.Combatant, err error) {
	if err = notEnded(c.enc); err != nil {
		return w, p, attacker, target, err
	}
	if p, err = c.q.GetPendingDamage(ctx, playdb.GetPendingDamageParams{EncounterID: c.enc.ID, ID: pendingID}); errors.Is(err, pgx.ErrNoRows) {
		return w, p, attacker, target, connect.NewError(connect.CodeNotFound, errors.New("pending damage not found"))
	} else if err != nil {
		return w, p, attacker, target, fmt.Errorf("find the pending damage: %w", err)
	}
	cs, err := c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return w, p, attacker, target, fmt.Errorf("list the combatants: %w", err)
	}
	if target, err = findCombatant(cs, p.TargetID, v); err != nil {
		return w, p, attacker, target, err
	}
	if err = v.mayAct(target); err != nil {
		return w, p, attacker, target, err
	}
	attacker, _ = findCombatant(cs, deref(p.AttackerID), combatViewer{master: true})
	open, err := openWindowsOf(ctx, c)
	if err != nil {
		return w, p, attacker, target, err
	}
	i := slices.IndexFunc(open, func(o playdb.ReactionWindow) bool {
		return o.Kind == string(reaction.Shield) && deref(o.PendingDamageID) == pendingID
	})
	if p.Status != pendingAwaitingReaction || i < 0 {
		return w, p, attacker, target, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_AWAITING_REACTION, "the hit does not wait for a reaction")
	}
	return open[i], p, attacker, target, nil
}

// useShield casts Escudo for the window's reactor, the target of the hit: the slot
// and the reaction are spent, the armor class goes up by 5 until its next turn and
// the hit is compared again, with the total that never leaves the server (the player
// decided without it, as at a table). A Magic Missile has no comparison: Shield takes
// all its damage away. It closes the window.
func (s *Service) useShield(ctx context.Context, c *combatTx, w playdb.ReactionWindow, p playdb.PendingDamage, attacker, target playdb.Combatant, slotIn *playv1.SpellSlot) (actionEvent, *playv1.CharacterVitals, error) {
	var none actionEvent
	slots, err := s.shieldSlots(ctx, c, target)
	if err != nil {
		return none, nil, err
	}
	slot, err := slotOf(slotIn, castable{level: 1, slots: slots})
	if err != nil {
		return none, nil, err
	}
	vitals, err := s.spendReactionSlot(ctx, c, target, *slot)
	if err != nil {
		return none, nil, err
	}
	if err := c.q.SetCombatantEconomy(ctx, playdb.SetCombatantEconomyParams{
		ID: target.ID, ActionUsed: target.ActionUsed, BonusActionUsed: target.BonusActionUsed, ReactionUsed: true, Dashed: target.Dashed,
	}); err != nil {
		return none, nil, fmt.Errorf("spend the reaction: %w", err)
	}
	if err := c.q.SetCombatantAcBonus(ctx, playdb.SetCombatantAcBonusParams{ID: target.ID, AcBonus: combat.ShieldACBonus}); err != nil {
		return none, nil, fmt.Errorf("give the armor class bonus: %w", err)
	}
	stopped, err := s.shieldStops(ctx, c, w, p, target)
	if err != nil {
		return none, nil, err
	}
	next := pendingAwaitingRoll
	if stopped {
		next = pendingDiscarded
	}
	if _, err := c.q.SetPendingDamageStatus(ctx, playdb.SetPendingDamageStatusParams{ID: p.ID, Status: next, ResolvedAt: resolvedWhen(next, c)}); err != nil {
		return none, nil, fmt.Errorf("answer the pending damage: %w", err)
	}
	// The bonus holds for every attack on the target until its next turn, so the
	// other hits already made, still waiting for the reaction or for their damage,
	// are compared again too.
	also, err := s.stopOtherHits(ctx, c, p, target)
	if err != nil {
		return none, nil, err
	}
	if _, err := s.closeWindow(ctx, c, w, windowAnswered, reaction.ReasonNone, windowOutcome{Used: true, Slot: slot, Stopped: stopped, ByMaster: c.master}, ""); err != nil {
		return none, nil, err
	}
	c.lastAnswered = reaction.Shield
	c.characterID = &target.CharacterID
	return actionEvent{
		Round: c.enc.Round, Secret: target.Hidden, AttackerHidden: attacker.Hidden, Actor: target.ID, Target: attacker.ID, Pending: p.ID, Key: shield, Slot: slot,
		Stopped: stopped, AlsoStopped: also, ReactionBefore: target.ReactionUsed, ACBonusBefore: target.AcBonus, PrevStatus: p.Status,
		Reaction: &reactionEvent{Kind: string(reaction.Shield), Window: w.ID, Used: true, Slot: slot, Stopped: stopped, ByMaster: c.master, Magic: windowTriggerOf(w).Magic},
	}, vitals, nil
}

// shieldSlots reads the free slots Escudo can be cast with and refuses a target that cannot cast it.
func (s *Service) shieldSlots(ctx context.Context, c *combatTx, target playdb.Combatant) ([]*rulesv1.SlotChoice, error) {
	if target.ReactionUsed {
		return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_REACTION_USED, "the reaction is already used")
	}
	k, ok, err := s.kitOf(ctx, c.tx, c.session.CampaignID, target)
	if err != nil {
		return nil, err
	}
	var slots []*rulesv1.SlotChoice
	if ok && !k.down && !target.Defeated {
		slots = k.slotsFor(spellShield)
	}
	if len(slots) == 0 {
		return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NO_SLOT, "there is no free spell slot for Escudo",
			func(b *playv1.EncounterBlocked) { b.MinLevel = 1 })
	}
	return slots, nil
}

// shieldStops says Escudo stops the hit. Escudo replaces the bonus the target had: the hit is
// compared again with the armor class it was compared with (cover included), plus the Escudo's 5. A
// damage opened before the column existed falls back to the sheet's. A Magic Missile has no
// comparison: Shield takes all its damage away.
func (s *Service) shieldStops(ctx context.Context, c *combatTx, w playdb.ReactionWindow, p playdb.PendingDamage, target playdb.Combatant) (bool, error) {
	sheet, err := s.sheetOf(ctx, c.tx, c.session.CampaignID, target)
	if err != nil {
		return false, err
	}
	base := sheet.ArmorClass
	if p.AttackArmorClass != nil {
		base = int(*p.AttackArmorClass) - int(target.AcBonus)
	}
	return windowTriggerOf(w).Magic || int(num(p.AttackTotal)) < base+combat.ShieldACBonus, nil
}

// passShield lets the hit go on ("Não usar"): the window is answered and, when it was
// the last one of the hit, the damage goes on to its roll.
func (s *Service) passShield(ctx context.Context, c *combatTx, w playdb.ReactionWindow, p playdb.PendingDamage, target playdb.Combatant) (actionEvent, error) {
	if _, err := s.closeWindow(ctx, c, w, windowAnswered, reaction.ReasonNone, windowOutcome{ByMaster: c.master}, ""); err != nil {
		return actionEvent{}, err
	}
	c.characterID = &target.CharacterID
	return actionEvent{
		Round: c.enc.Round, Secret: target.Hidden, Actor: target.ID, Pending: p.ID, Key: shield, PrevStatus: p.Status,
		Reaction: &reactionEvent{Kind: string(reaction.Shield), Window: w.ID, ByMaster: c.master},
	}, nil
}

// UseReaction implements playv1connect.CombatServiceHandler.
func (s *Service) UseReaction(
	ctx context.Context,
	req *connect.Request[playv1.UseReactionRequest],
) (*connect.Response[playv1.UseReactionResponse], error) {
	m, key, encID, pendingID, err := parseReactionCall(ctx, req.Msg.GetCampaignId(), req.Msg.GetIdempotencyKey(), req.Msg.GetEncounterId(), req.Msg.GetPendingDamageId())
	if err != nil {
		return nil, err
	}
	v := viewerOf(m)

	var made actionEvent
	var vitals *playv1.CharacterVitals
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventReactionUsed, encounterID: encID}, func(c *combatTx) (any, error) {
		vitals = nil
		var err error
		if made, vitals, err = s.shieldChange(ctx, c, v, pendingID, req.Msg.GetSlot()); err != nil {
			return nil, err
		}
		return made, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "use a reaction", err)
	}
	ev, err := resultEvent(res, made)
	if err != nil {
		return nil, s.dbError(ctx, "read the reaction", err)
	}
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, !ev.Secret && !ev.AttackerHidden)
		s.publishVitals(m.CampaignID, vitals)
	})
	if err != nil {
		return nil, err
	}
	resp := &playv1.UseReactionResponse{Encounter: out, Outcome: playv1.ReactionOutcome_REACTION_OUTCOME_STILL_HIT}
	if ev.Stopped {
		resp.Outcome = playv1.ReactionOutcome_REACTION_OUTCOME_STOPPED
	}
	if v.master {
		if resp.PendingDamage, err = s.pendingFor(ctx, res, ev.Pending, v); err != nil {
			return nil, err
		}
	}
	return connect.NewResponse(resp), nil
}

// parseReactionCall reads the caller's membership and the ids of a Shield answer, in the order the
// handlers always checked them.
func parseReactionCall(ctx context.Context, campaignID, keyRaw, encounterRaw, pendingRaw string) (m authz.Membership, key string, encID, pendingID string, err error) {
	if m, key, encID, err = contestCaller(ctx, campaignID, keyRaw, encounterRaw); err != nil {
		return m, key, encID, "", err
	}
	if pendingID, err = parseCombatID(pendingRaw, "pending damage"); err != nil {
		return m, key, encID, "", err
	}
	return m, key, encID, pendingID, nil
}

// shieldChange is the change of a Shield used: the window of the hit is found, must be the first of
// its group to answer, and Escudo is cast for it.
func (s *Service) shieldChange(ctx context.Context, c *combatTx, v combatViewer, pendingID string, slot *playv1.SpellSlot) (actionEvent, *playv1.CharacterVitals, error) {
	w, p, attacker, target, err := s.reactionTarget(ctx, c, v, pendingID)
	if err != nil {
		return actionEvent{}, nil, err
	}
	if err := s.mustBeFirstOfGroup(ctx, c, w); err != nil {
		return actionEvent{}, nil, err
	}
	made, vitals, err := s.useShield(ctx, c, w, p, attacker, target, slot)
	if err != nil {
		return made, vitals, err
	}
	if err = touchEncounter(ctx, c); err != nil {
		return made, vitals, err
	}
	made.Round = c.enc.Round
	return made, vitals, nil
}

// stopOtherHits discards the hits on the target, other than answered, that the
// Escudo's armor class stops: each is compared again with the armor class it was
// compared with (cover included) plus the Escudo's 5, as the answered one is. It
// returns what it discarded, for the undo.
func (s *Service) stopOtherHits(ctx context.Context, c *combatTx, answered playdb.PendingDamage, target playdb.Combatant) ([]stoppedHit, error) {
	open, err := c.q.ListOpenPendingDamages(ctx, c.enc.ID)
	if err != nil {
		return nil, fmt.Errorf("list the pending damage: %w", err)
	}
	var out []stoppedHit
	for _, o := range open {
		if o.ID == answered.ID || o.TargetID != target.ID || o.AttackTotal == nil || o.AttackArmorClass == nil ||
			(o.Status != pendingAwaitingReaction && o.Status != pendingAwaitingRoll) {
			continue
		}
		if int(*o.AttackTotal) >= int(*o.AttackArmorClass)-int(target.AcBonus)+combat.ShieldACBonus {
			continue
		}
		if _, err := c.q.SetPendingDamageStatus(ctx, playdb.SetPendingDamageStatusParams{ID: o.ID, Status: pendingDiscarded, ResolvedAt: &c.now}); err != nil {
			return nil, fmt.Errorf("stop the other hit: %w", err)
		}
		out = append(out, stoppedHit{Pending: o.ID, PrevStatus: o.Status})
	}
	return out, nil
}

// resolvedWhen is when a pending damage was settled by a reaction: now for one
// that was stopped, not yet for one that goes on.
func resolvedWhen(status string, c *combatTx) *time.Time {
	if status == pendingDiscarded {
		return &c.now
	}
	return nil
}

// DeclineReaction implements playv1connect.CombatServiceHandler.
func (s *Service) DeclineReaction(
	ctx context.Context,
	req *connect.Request[playv1.DeclineReactionRequest],
) (*connect.Response[playv1.DeclineReactionResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	key, err := parseKey(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	encID, err := parseCombatID(req.Msg.GetEncounterId(), "encounter")
	if err != nil {
		return nil, err
	}
	pendingID, err := parseCombatID(req.Msg.GetPendingDamageId(), "pending damage")
	if err != nil {
		return nil, err
	}
	v := viewerOf(m)

	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventReactionDeclined, encounterID: encID}, func(c *combatTx) (any, error) {
		w, p, _, target, err := s.reactionTarget(ctx, c, v, pendingID)
		if err != nil {
			return nil, err
		}
		if err := s.mustBeFirstOfGroup(ctx, c, w); err != nil {
			return nil, err
		}
		ev, err := s.passShield(ctx, c, w, p, target)
		if err != nil {
			return nil, err
		}
		if err = touchEncounter(ctx, c); err != nil {
			return nil, err
		}
		ev.Round = c.enc.Round
		return ev, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "decline a reaction", err)
	}
	out, err := s.finish(ctx, m, res, s.changed(m.CampaignID))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.DeclineReactionResponse{Encounter: out}), nil
}
