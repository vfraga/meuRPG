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
)

// Reactions (MR-014, timeline decision 5, Etapa 6, slice 6.4b). Escudo is the
// one reaction spell the combat runs: it is cast when an attack hits a player's
// character, so the hit waits (AWAITING_REACTION) until the target's player, or
// the master, answers. The opportunity attack is RollAttack's as_reaction
// (combat_actions.go). Every other reaction spell stays the table's.

// shield is the reaction spell the combat knows.
const shield = "spell:shield"

// shieldFor says whether a player's character can cast Escudo right now: it is
// prepared, there is a free slot, the reaction is unused and the character is
// not down. It returns the slots it can be cast with. The options come from the
// character's sheet and its vitals as they were committed.
func (s *Service) shieldFor(ctx context.Context, tx pgx.Tx, campaignID string, target playdb.Combatant) ([]*rulesv1.SlotChoice, error) {
	if target.Kind != kindPlayer || target.ReactionUsed || target.Defeated {
		return nil, nil
	}
	opts, err := s.roster.CombatTurnOptions(ctx, tx, campaignID, target.CharacterID, turnOf(target))
	if err != nil {
		return nil, err
	}
	var slots []*rulesv1.SlotChoice
	for _, sp := range opts.GetSpells() {
		if sp.GetSpell().GetKey() == shield {
			slots = sp.GetSlots()
		}
	}
	if len(slots) == 0 {
		return nil, nil
	}
	down, err := s.isDown(ctx, tx, campaignID, target)
	if err != nil || down {
		return nil, err
	}
	return slots, nil
}

// openHit opens the pending damage of a hit, an attack's or a spell attack's:
// to be rolled, or, for a hit on a player's character that can cast Escudo and
// is not a critical hit (the SRD's Escudo still works against one, but the
// table keeps the prompt for the hits it can stop), to wait for the reaction
// first. attackTotal is kept for the new comparison.
func (s *Service) openHit(ctx context.Context, c *combatTx, campaignID string, attacker, target playdb.Combatant, key string, dmg link.Dice, critical bool, attackTotal, attackAC int) (playdb.PendingDamage, error) {
	status := pendingAwaitingRoll
	if !critical {
		slots, err := s.shieldFor(ctx, c.tx, campaignID, target)
		if err != nil {
			return playdb.PendingDamage{}, err
		}
		if len(slots) > 0 {
			status = pendingAwaitingReaction
		}
	}
	total := clamp32(attackTotal, math.MinInt32, math.MaxInt32)
	// The table's critical rule (RN-24), read in this transaction: the dice to
	// roll, doubled or not, and the maximum that comes without rolling. A hit that
	// is not a critical one rolls its dice as they are.
	count, fixed := combat.CriticalDice(rules.DiceFormula{Count: dmg.Count, Sides: dmg.Sides}, critical, criticalRuleOf(c.rules))
	p, err := c.q.InsertPendingDamage(ctx, playdb.InsertPendingDamageParams{
		EncounterID: c.enc.ID, AttackerID: &attacker.ID, TargetID: target.ID, AttackKey: key, Status: status, Critical: critical,
		DiceCount: clamp32(count, 0, 100), CriticalMax: clamp32(fixed, 0, 10000), CriticalMaxRule: critical && c.rules.CriticalMaxPlusRoll,
		DiceSides: clamp32(dmg.Sides, 0, 100), DiceBonus: clamp32(dmg.Bonus, -1000, 1000),
		DamageType: dmg.DamageType, CreatedAt: c.now, AttackTotal: &total, AttackArmorClass: new(clamp32(attackAC, 0, math.MaxInt32)),
	})
	if err != nil {
		return playdb.PendingDamage{}, fmt.Errorf("open the pending damage: %w", err)
	}
	return p, nil
}

// reactionPrompts lists the hits that wait for a reaction as the caller may
// see them: the master all, a player only those on their own character. A
// player never gets the attack's total or an armor class, nor who attacked
// when the attacker is hidden from them.
func (s *Service) reactionPrompts(ctx context.Context, m authz.Membership, d *encounterData, v combatViewer, names func(key string) string) ([]*playv1.ReactionPrompt, error) {
	if d.enc.Status != statusActive {
		return nil, nil
	}
	open, err := s.queries.ListOpenPendingDamages(ctx, d.enc.ID)
	if err != nil {
		return nil, s.dbError(ctx, "list the pending damage", err)
	}
	var out []*playv1.ReactionPrompt
	for _, p := range open {
		if p.Status != pendingAwaitingReaction {
			continue
		}
		i := slices.IndexFunc(d.cs, func(c playdb.Combatant) bool { return c.ID == p.TargetID })
		if i < 0 || (!v.master && !v.owns(d.cs[i])) {
			continue
		}
		slots, err := s.shieldFor(ctx, nil, m.CampaignID, d.cs[i])
		if err != nil {
			return nil, s.dbError(ctx, "work out the reaction", err)
		}
		prompt := &playv1.ReactionPrompt{PendingDamageId: p.ID, TargetId: p.TargetID, SpellKey: shield, Slots: slots, SpellNamePt: names(shield)}
		if v.master {
			prompt.AttackerId = deref(p.AttackerID)
		}
		// Who attacked, and with what, only when the viewer sees the attacker (RN-20):
		// the screen says "Capitão Goblin · Cimitarra", and a hidden attacker stays hidden.
		if j := slices.IndexFunc(d.cs, func(c playdb.Combatant) bool { return c.ID == deref(p.AttackerID) }); j >= 0 && v.sees(d.cs[j]) {
			prompt.AttackerLabel = d.cs[j].Label
			if sheet, err := s.sheetOf(ctx, nil, m.CampaignID, d.cs[j]); err == nil {
				if k := slices.IndexFunc(sheet.Attacks, func(a link.Attack) bool { return a.Key == p.AttackKey }); k >= 0 {
					prompt.AttackNamePt = sheet.Attacks[k].Name
				}
			}
		}
		out = append(out, prompt)
	}
	return out, nil
}

// reactionTarget finds the pending damage that waits for a reaction and the
// combatant it hit, inside the change's transaction, for the caller to answer:
// not_found for a pending damage the caller may not see, permission_denied for
// another player's character, NOT_AWAITING_REACTION for one that does not wait.
func (s *Service) reactionTarget(ctx context.Context, c *combatTx, v combatViewer, pendingID string) (p playdb.PendingDamage, attacker, target playdb.Combatant, err error) {
	if err = notEnded(c.enc); err != nil {
		return p, attacker, target, err
	}
	if p, err = c.q.GetPendingDamage(ctx, playdb.GetPendingDamageParams{EncounterID: c.enc.ID, ID: pendingID}); errors.Is(err, pgx.ErrNoRows) {
		return p, attacker, target, connect.NewError(connect.CodeNotFound, errors.New("pending damage not found"))
	} else if err != nil {
		return p, attacker, target, fmt.Errorf("find the pending damage: %w", err)
	}
	cs, err := c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return p, attacker, target, fmt.Errorf("list the combatants: %w", err)
	}
	if target, err = findCombatant(cs, p.TargetID, v); err != nil {
		return p, attacker, target, err
	}
	if err = v.mayAct(target); err != nil {
		return p, attacker, target, err
	}
	attacker, _ = findCombatant(cs, deref(p.AttackerID), combatViewer{master: true})
	if p.Status != pendingAwaitingReaction {
		return p, attacker, target, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_AWAITING_REACTION, "the hit does not wait for a reaction")
	}
	return p, attacker, target, nil
}

// UseReaction implements playv1connect.CombatServiceHandler.
func (s *Service) UseReaction(
	ctx context.Context,
	req *connect.Request[playv1.UseReactionRequest],
) (*connect.Response[playv1.UseReactionResponse], error) {
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

	var made actionEvent
	var vitals *playv1.CharacterVitals
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventReactionUsed, encounterID: encID}, func(c *combatTx) (any, error) {
		vitals = nil
		p, attacker, target, err := s.reactionTarget(ctx, c, v, pendingID)
		if err != nil {
			return nil, err
		}
		if target.ReactionUsed {
			return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_REACTION_USED, "the reaction is already used")
		}
		slots, err := s.shieldFor(ctx, c.tx, m.CampaignID, target)
		if err != nil {
			return nil, err
		}
		if len(slots) == 0 {
			return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NO_SLOT, "there is no free spell slot for Escudo",
				func(b *playv1.EncounterBlocked) { b.MinLevel = 1 })
		}
		slot, err := slotOf(req.Msg.GetSlot(), castable{level: 1, slots: slots})
		if err != nil {
			return nil, err
		}
		if vitals, err = s.spendSlot(ctx, c, target.CharacterID, *slot, 1); err != nil {
			return nil, err
		}
		if err := c.q.SetCombatantEconomy(ctx, playdb.SetCombatantEconomyParams{
			ID: target.ID, ActionUsed: target.ActionUsed, BonusActionUsed: target.BonusActionUsed, ReactionUsed: true, Dashed: target.Dashed,
		}); err != nil {
			return nil, fmt.Errorf("spend the reaction: %w", err)
		}
		if err := c.q.SetCombatantAcBonus(ctx, playdb.SetCombatantAcBonusParams{ID: target.ID, AcBonus: combat.ShieldACBonus}); err != nil {
			return nil, fmt.Errorf("give the armor class bonus: %w", err)
		}

		// The attack is compared again with the new armor class, and it never
		// leaves the server: the player decided without the total (as at a table).
		sheet, err := s.sheetOf(ctx, c.tx, m.CampaignID, target)
		if err != nil {
			return nil, err
		}
		// Escudo replaces the bonus the target had: the hit is compared again with the
		// armor class it was compared with (cover included), plus the Escudo's 5. A
		// damage opened before the column existed falls back to the sheet's.
		base := sheet.ArmorClass
		if p.AttackArmorClass != nil {
			base = int(*p.AttackArmorClass) - int(target.AcBonus)
		}
		stopped := int(num(p.AttackTotal)) < base+combat.ShieldACBonus
		next := pendingAwaitingRoll
		if stopped {
			next = pendingDiscarded
		}
		if _, err := c.q.SetPendingDamageStatus(ctx, playdb.SetPendingDamageStatusParams{ID: p.ID, Status: next, ResolvedAt: resolvedWhen(next, c)}); err != nil {
			return nil, fmt.Errorf("answer the pending damage: %w", err)
		}
		// The bonus holds for every attack on the target until its next turn, so the
		// other hits already made, still waiting for the reaction or for their damage,
		// are compared again too.
		also, err := s.stopOtherHits(ctx, c, p, target)
		if err != nil {
			return nil, err
		}
		if c.enc, err = c.q.TouchEncounter(ctx, c.enc.ID); err != nil {
			return nil, fmt.Errorf("touch the encounter: %w", err)
		}
		c.characterID = &target.CharacterID
		made = actionEvent{
			Round: c.enc.Round, Secret: target.Hidden, AttackerHidden: attacker.Hidden, Actor: target.ID, Target: attacker.ID, Pending: p.ID, Key: shield, Slot: slot,
			Stopped: stopped, AlsoStopped: also, ReactionBefore: target.ReactionUsed, ACBonusBefore: target.AcBonus, PrevStatus: p.Status,
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
		p, _, target, err := s.reactionTarget(ctx, c, v, pendingID)
		if err != nil {
			return nil, err
		}
		if _, err := c.q.SetPendingDamageStatus(ctx, playdb.SetPendingDamageStatusParams{ID: p.ID, Status: pendingAwaitingRoll}); err != nil {
			return nil, fmt.Errorf("let the hit go on: %w", err)
		}
		if c.enc, err = c.q.TouchEncounter(ctx, c.enc.ID); err != nil {
			return nil, fmt.Errorf("touch the encounter: %w", err)
		}
		c.characterID = &target.CharacterID
		return actionEvent{Round: c.enc.Round, Secret: target.Hidden, Actor: target.ID, Pending: p.ID, Key: shield, PrevStatus: p.Status}, nil
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
