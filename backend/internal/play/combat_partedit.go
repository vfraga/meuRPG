package play

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/damagesteps"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// partRemoval is what RemoveDamagePart reads of its request, checked.
type partRemoval struct {
	m         authz.Membership
	key       string
	encID     string
	pendingID string
	partKey   string
	reason    string
}

// readPartRemovalCall checks the request in the order of the refusals: the
// master's role, the key, the combat, the pending damage, the part, the reason.
func readPartRemovalCall(ctx context.Context, msg *playv1.RemoveDamagePartRequest) (partRemoval, error) {
	m, key, encID, err := masterCall(ctx, msg.GetCampaignId(), msg.GetIdempotencyKey(), msg.GetEncounterId())
	if err != nil {
		return partRemoval{}, err
	}
	pendingID, err := parseCombatID(msg.GetPendingDamageId(), "pending damage")
	if err != nil {
		return partRemoval{}, err
	}
	partKey := msg.GetPartKey()
	if partKey == "" || len(partKey) > maxPartKeyLen {
		return partRemoval{}, connect.NewError(connect.CodeInvalidArgument, errors.New("part_key must name a part of the damage"))
	}
	reason, err := cleanReason(msg.GetReason())
	if err != nil {
		return partRemoval{}, err
	}
	return partRemoval{m: m, key: key, encID: encID, pendingID: pendingID, partKey: partKey, reason: reason}, nil
}

// maxPartKeyLen is the longest key of a part of a damage the master may name.
const maxPartKeyLen = 100

// RemoveDamagePart implements playv1connect.CombatServiceHandler: the master takes an
// extra out of a rolled damage, with a reason (PM-06b, decision 17). Sneak Attack
// without an ally near the target in a combat with no map is the usual case: the
// app cannot know, and the master can. The total drops by that part; an NPC that took
// the damage at once gets the difference back, up to its maximum. It cannot be undone:
// the damage would have to be rolled again. An extra inside a group of parts that share a damage
// type goes another way (combat_group_sum.go): the damage is taken back as an undo takes it
// back, and waits for the sum of that damage type.
func (s *Service) RemoveDamagePart(
	ctx context.Context,
	req *connect.Request[playv1.RemoveDamagePartRequest],
) (*connect.Response[playv1.RemoveDamagePartResponse], error) {
	r, err := readPartRemovalCall(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	var made actionEvent
	var run undoRun // what the take-out of a group part put back, for what is told after the commit
	res, err := s.write(ctx, combatWrite{m: r.m, key: r.key, hash: idem.Hash(req.Msg), kind: eventDamagePartRemoved, encounterID: r.encID}, func(c *combatTx) (any, error) {
		run = undoRun{}
		ev, err := s.removePartWrite(ctx, c, &run, r)
		if err != nil {
			return nil, err
		}
		made = ev
		return made, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "take a part out of a damage", err)
	}
	ev, err := resultEvent(res, made)
	if err != nil {
		return nil, s.dbError(ctx, "read the removal", err)
	}
	out, err := s.finish(ctx, r.m, res, func(ctx context.Context, d *encounterData) {
		if ev.AwaitsSum != "" { // the damage was taken back: told as an undo is
			s.publishUndoRun(ctx, r.m.CampaignID, d, ev, &run)
			return
		}
		s.publishEncounterChanged(ctx, r.m.CampaignID, d.enc)
		s.publishLogChanged(ctx, r.m.CampaignID, d.enc.ID, !ev.Secret)
	})
	if err != nil {
		return nil, err
	}
	pending, err := s.pendingFor(ctx, res, ev.Pending, combatViewer{master: true})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.RemoveDamagePartResponse{Encounter: out, PendingDamage: pending}), nil
}

// removePartWrite is the removal inside its transaction: the part comes out of the rolled
// damage, the damage is worked out again and kept, and the removal's event is returned.
func (s *Service) removePartWrite(ctx context.Context, c *combatTx, run *undoRun, r partRemoval) (actionEvent, error) {
	if err := notEnded(c.enc); err != nil {
		return actionEvent{}, err
	}
	p, err := rolledPending(ctx, c, r.pendingID)
	if err != nil {
		return actionEvent{}, err
	}
	cs, err := listCombatants(ctx, c)
	if err != nil {
		return actionEvent{}, err
	}
	attacker, _ := findCombatant(cs, conv.OrZero(p.AttackerID), combatViewer{master: true})
	target, _ := findCombatant(cs, p.TargetID, combatViewer{master: true})
	// The master gave a player's character the damage already: it went through the
	// vitals, and he corrects them by hand.
	if p.Status == pendingApplied && !holdsHP(target) {
		return actionEvent{}, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_DAMAGE_RESOLVED, "the damage was applied to a character already")
	}
	parts, rolls := readParts(p.Parts), readPartRolls(p.PartRolls)
	if inTypedGroup(rolls, r.partKey) {
		made, err := s.takeOutOfGroup(ctx, c, run, r, p, attacker, target, parts, rolls)
		if err != nil {
			return actionEvent{}, err
		}
		return s.keepGroupRemoval(ctx, c, made, r, attacker)
	}
	if !takeOut(parts, rolls, r.partKey) {
		return actionEvent{}, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_DAMAGE_PART_NOT_REMOVABLE, "that part cannot be taken out")
	}
	if err := giveBackOnce(ctx, c, attacker, r.partKey); err != nil {
		return actionEvent{}, err
	}
	rem, err := workOutRemoval(ctx, c, p, attacker, target, r.partKey, rolls)
	if err != nil {
		return actionEvent{}, err
	}
	made, err := keepRemoval(ctx, c, p, parts, rolls, rem, r.reason)
	if err != nil {
		return actionEvent{}, err
	}
	if err := touch(ctx, c); err != nil {
		return actionEvent{}, err
	}
	c.characterID = &attacker.CharacterID
	return made, nil
}

// removal is the damage worked out again without the part that came out.
type removal struct {
	ev     actionEvent
	total  int32 // the roll's total, before the target's modifiers
	amount int32 // the amount the pending damage keeps
	land   damagesteps.Landing
}

// workOutRemoval works the damage out again from the parts that still count, through the
// target's modifiers, and gives an NPC that took the damage at once the difference.
func workOutRemoval(ctx context.Context, c *combatTx, p playdb.PendingDamage, attacker, target playdb.Combatant, partKey string, rolls []partRoll) (removal, error) {
	byType, total := countedByType(rolls)
	// The target's modifiers are the ones the roll used: the stored groups say them.
	groups := damagesteps.Read(p.Steps)
	land := damagesteps.Settle(byType, damagesteps.Mods(groups), nil, damagesteps.Origin(groups))
	amount := clamp32(total, 0, math.MaxInt32)
	rem := removal{total: amount, amount: amount, land: land}
	rem.ev = actionEvent{
		Round: c.enc.Round, Secret: secretOf(attacker, target), Actor: attacker.ID, Target: target.ID, Pending: p.ID, Key: partKey,
		DamageType: p.DamageType, Rolled: conv.OrZero(p.Amount), Total: amount,
		PartHidden: slices.ContainsFunc(rolls, func(x partRoll) bool { return x.Key == partKey && x.Hide }),
	}
	landed := amount
	if land.Changed {
		landed = land.Amount
	}
	rem.ev.Amount = landed
	if holdsHP(target) && p.Status == pendingApplied {
		now, err := giveBackHitPoints(ctx, c, p, target, landed)
		if err != nil {
			return removal{}, err
		}
		rem.ev.Before, rem.ev.After = new(hpOf(target)), &now
		rem.amount = landed
	}
	return rem, nil
}

// keepRemoval stores the damage without the part and the master's reason, and returns the
// removal's event with the reason and the amount the damage keeps.
func keepRemoval(ctx context.Context, c *combatTx, p playdb.PendingDamage, parts []partRecord, rolls []partRoll, rem removal, reason string) (actionEvent, error) {
	var after *int32
	if rem.land.Changed {
		after = &rem.land.Amount
	}
	if err := c.q.SetPendingDamageAmount(ctx, playdb.SetPendingDamageAmountParams{
		ID: p.ID, Amount: &rem.amount, RollTotal: &rem.total, Parts: mustJSON(parts), PartRolls: mustJSON(rolls), Steps: mustJSON(rem.land.Groups), AfterSteps: after,
	}); err != nil {
		return actionEvent{}, fmt.Errorf("keep the damage without the extra: %w", err)
	}
	row, err := c.q.InsertCombatReason(ctx, playdb.InsertCombatReasonParams{EncounterID: c.enc.ID, Kind: "part_removed", Reason: reason, CreatedAt: c.now})
	if err != nil {
		return actionEvent{}, fmt.Errorf("keep the reason: %w", err)
	}
	made := rem.ev
	made.ReasonID, made.Amount = row.ID, rem.amount
	return made, nil
}

// countedByType sums the dice and numbers of the parts that count, by damage type, and
// all of them together.
func countedByType(rolls []partRoll) (map[string]int, int) {
	byType := map[string]int{}
	total := 0
	for _, r := range rolls {
		if r.Counted {
			n := max(int(r.Sum)+int(r.Flat)+int(r.Fixed), 0)
			byType[r.DamageType] += n
			total += n
		}
	}
	return byType, total
}

// giveBackHitPoints gives an NPC the hit points a removed extra took: its hit points
// before the damage, with what is left of the damage taken.
func giveBackHitPoints(ctx context.Context, c *combatTx, p playdb.PendingDamage, target playdb.Combatant, landed int32) (hpState, error) {
	var before hpState
	if err := jsonUnmarshal(p.LandedBefore, &before); err != nil {
		return hpState{}, fmt.Errorf("read the hit points before the damage: %w", err)
	}
	dmg := combat.ApplyDamage(int(before.HP), int(before.Temp), int(landed))
	now := hpState{HP: clamp32(dmg.HP, 0, math.MaxInt32), Temp: clamp32(dmg.TempHP, 0, math.MaxInt32), Defeated: dmg.HP == 0}
	if err := c.q.SetCombatantHitPoints(ctx, playdb.SetCombatantHitPointsParams{ID: target.ID, HpCurrent: &now.HP, HpTemp: &now.Temp, Defeated: now.Defeated}); err != nil {
		return hpState{}, fmt.Errorf("give the hit points back: %w", err)
	}
	return now, nil
}

// keepGroupRemoval stores the master's reason for the take-out of a group part and finishes the removal's transaction.
func (s *Service) keepGroupRemoval(ctx context.Context, c *combatTx, made actionEvent, r partRemoval, attacker playdb.Combatant) (actionEvent, error) {
	row, err := c.q.InsertCombatReason(ctx, playdb.InsertCombatReasonParams{EncounterID: c.enc.ID, Kind: "part_removed", Reason: r.reason, CreatedAt: c.now})
	if err != nil {
		return actionEvent{}, fmt.Errorf("keep the reason: %w", err)
	}
	made.ReasonID = row.ID
	if err := touch(ctx, c); err != nil {
		return actionEvent{}, err
	}
	c.characterID = &attacker.CharacterID
	return made, nil
}

// inTypedGroup says the roll was entered with a damage-type sum that covers the part: the sum sits on one part of the
// group, so a part of it cannot come out alone.
func inTypedGroup(rolls []partRoll, key string) bool {
	return slices.ContainsFunc(rolls, func(x partRoll) bool { return x.Key == key && x.Group != "" })
}

// takeOut marks an extra as removed, and says whether it could be. Only an extra that
// counts comes out; the weapon, an automatic line, a smite (whose slot is spent) and a part a typed
// damage-type sum covers stay.
func takeOut(parts []partRecord, rolls []partRoll, key string) bool {
	pi := slices.IndexFunc(parts, func(x partRecord) bool { return x.Key == key })
	ri := slices.IndexFunc(rolls, func(x partRoll) bool { return x.Key == key })
	if pi < 0 || ri < 0 || rolls[ri].Group != "" || parts[pi].Kind != partExtra || key == combat.ExtraDivineSmite || key == combat.ExtraDivineSmiteExtra || !rolls[ri].Counted {
		return false
	}
	parts[pi].Removed, rolls[ri].Counted = true, false
	return true
}

// giveBackOnce gives back the use of a once-per-turn extra the master took out: the
// damage never had it, so the turn may still add it (Sneak Attack, Colossus Slayer).
func giveBackOnce(ctx context.Context, c *combatTx, attacker playdb.Combatant, key string) error {
	var feature string
	switch key {
	case combat.ExtraSneakAttack:
		feature = featureSneakAttack
	case combat.ExtraColossusSlayer:
		feature = featureColossusSlayer
	default:
		return nil
	}
	now := stateNowOf(c.enc)
	state := readFeatureState(attacker.FeatureState)
	if !state.used(feature, now) {
		return nil
	}
	state.del(feature)
	if err := saveFeatureState(ctx, c, attacker.ID, state, now); err != nil {
		return fmt.Errorf("give back the once-per-turn damage: %w", err)
	}
	return nil
}

// rolledPending finds a pending damage of the combat that was rolled.
func rolledPending(ctx context.Context, c *combatTx, id string) (playdb.PendingDamage, error) {
	p, err := c.q.GetPendingDamage(ctx, playdb.GetPendingDamageParams{EncounterID: c.enc.ID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return p, connect.NewError(connect.CodeNotFound, errors.New("pending damage not found"))
	}
	if err != nil {
		return p, fmt.Errorf("find the pending damage: %w", err)
	}
	if p.Status != pendingRolled && p.Status != pendingApplied {
		return p, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_DAMAGE_NOT_ROLLED, "the damage was not rolled")
	}
	return p, nil
}
