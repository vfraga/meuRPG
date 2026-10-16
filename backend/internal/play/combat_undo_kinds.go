package play

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// One function for each kind of event the master's Desfazer takes back (takeBack,
// combat_undo.go). Each one puts back what its event changed, with the "before"
// the event kept. A combatant that is gone (its character was deleted meanwhile)
// is skipped: there is nothing left to put back on it.

// undoer is one takeBack at work: the combat's transaction, the combatants as they
// stood when the undo began, and the vitals of the characters it restored.
type undoer struct {
	s      *Service
	c      *combatTx
	cs     []playdb.Combatant
	vitals []*playv1.CharacterVitals
}

// undoDFtPerFt is the tenths of a foot in a foot, the unit a move keeps its distances in.
const undoDFtPerFt = 10

// undoers dispatches an event kind to the function that takes it back.
var undoers = map[string]func(*undoer, context.Context, actionEvent) error{
	eventAttackRolled:      (*undoer).attackRolled,
	eventDamageRolled:      (*undoer).damageRolled,
	eventDamageApplied:     (*undoer).damageApplied,
	eventDamageDiscarded:   (*undoer).damageDiscarded,
	eventHideResolved:      (*undoer).hideResolved,
	eventCombatantMoved:    (*undoer).combatantMoved,
	eventTrapTriggered:     (*undoer).trapTriggered,
	eventActionTaken:       (*undoer).actionTaken,
	eventHitPointsAdjusted: (*undoer).hitPointsAdjusted,
	eventSpellCast:         (*undoer).spellCast,
	eventReactionUsed:      (*undoer).reactionUsed,
	eventReactionDeclined:  (*undoer).reactionDeclined,
	eventDeathSaveRolled:   (*undoer).deathSaveRolled,
	eventConditionsSet:     (*undoer).conditionsSet,
	eventWildShapeStarted:  (*undoer).wildShapeStarted,
	eventWildShapeEnded:    (*undoer).wildShapeEnded,
	eventFamiliarSight:     (*undoer).familiarSight,
}

// find is the combatant of that ID, as the undo found them.
func (u *undoer) find(id string) (playdb.Combatant, bool) {
	i := slices.IndexFunc(u.cs, func(o playdb.Combatant) bool { return o.ID == id })
	if i < 0 {
		return playdb.Combatant{}, false
	}
	return u.cs[i], true
}

// keep remembers the vitals of a character the undo restored, to tell the table.
func (u *undoer) keep(v *playv1.CharacterVitals) {
	if v != nil {
		u.vitals = append(u.vitals, v)
	}
}

func (u *undoer) setStatus(ctx context.Context, id, status string) error {
	_, err := u.c.q.SetPendingDamageStatus(ctx, playdb.SetPendingDamageStatusParams{ID: id, Status: status})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("put back the pending damage: %w", err)
	}
	return nil
}

func (u *undoer) setEconomy(ctx context.Context, who playdb.Combatant, action, bonus, reaction, dashed bool) error {
	if err := u.c.q.SetCombatantEconomy(ctx, playdb.SetCombatantEconomyParams{ID: who.ID, ActionUsed: action, BonusActionUsed: bonus, ReactionUsed: reaction, Dashed: dashed}); err != nil {
		return fmt.Errorf("put back the economy: %w", err)
	}
	return nil
}

func (u *undoer) setRun(ctx context.Context, who playdb.Combatant, run int32) error {
	if run == 0 {
		return nil
	}
	if err := u.c.q.SetCombatantRun(ctx, playdb.SetCombatantRunParams{ID: who.ID, LastMoveDft: run}); err != nil {
		return fmt.Errorf("put back the running start: %w", err)
	}
	return nil
}

func (u *undoer) setAttacks(ctx context.Context, who playdb.Combatant, n int32) error {
	if err := u.c.q.SetCombatantAttacksMade(ctx, playdb.SetCombatantAttacksMadeParams{ID: who.ID, AttacksMade: n}); err != nil {
		return fmt.Errorf("put back the attacks: %w", err)
	}
	return nil
}

func (u *undoer) setAttackState(ctx context.Context, who playdb.Combatant, key string, flurry int32) error {
	var k *string
	if key != "" {
		k = &key
	}
	if err := u.c.q.SetCombatantAttackState(ctx, playdb.SetCombatantAttackStateParams{ID: who.ID, ActionAttackKey: k, BonusAttacksLeft: flurry}); err != nil {
		return fmt.Errorf("put back the attack of the action: %w", err)
	}
	return nil
}

func (u *undoer) setHP(ctx context.Context, who playdb.Combatant, hp hpState) error {
	if err := u.c.q.SetCombatantHitPoints(ctx, playdb.SetCombatantHitPointsParams{ID: who.ID, HpCurrent: &hp.HP, HpTemp: &hp.Temp, Defeated: hp.Defeated}); err != nil {
		return fmt.Errorf("put back the hit points: %w", err)
	}
	return nil
}

func (u *undoer) setDeath(ctx context.Context, who playdb.Combatant, d *deathState) error {
	if d == nil {
		return nil
	}
	if err := u.c.q.SetCombatantDeathSaves(ctx, playdb.SetCombatantDeathSavesParams{ID: who.ID, DeathSuccesses: d.Successes, DeathFailures: d.Failures, DeathSaveRolled: d.Rolled, Defeated: d.Dead}); err != nil {
		return fmt.Errorf("put back the death saves: %w", err)
	}
	return nil
}

// setConditionsBack puts the conditions the combatant had back.
func (u *undoer) setConditionsBack(ctx context.Context, id string, before []string) error {
	if err := u.c.q.SetCombatantConditions(ctx, playdb.SetCombatantConditionsParams{ID: id, Conditions: nonNil(before)}); err != nil {
		return fmt.Errorf("put back the conditions: %w", err)
	}
	return nil
}

// giveExtraActionBack gives back the extra action an effect (Velocidade) lent.
func (u *undoer) giveExtraActionBack(ctx context.Context, who playdb.Combatant) error {
	if err := u.c.q.SetCombatantExtraActionUsed(ctx, playdb.SetCombatantExtraActionUsedParams{ID: who.ID, ExtraActionUsed: false}); err != nil {
		return fmt.Errorf("give the extra action back: %w", err)
	}
	return nil
}

// putVitals puts a player's character's vitals back. The maximum may have dropped
// since (a level lost), and the vitals refuse a value above it, which would leave
// this undo stuck for good, so the values are cut to the current maximums.
func (u *undoer) putVitals(ctx context.Context, who playdb.Combatant, hp hpState) (*playv1.CharacterVitals, error) {
	now, err := u.s.vitals.GetVitalsTx(ctx, u.c.tx, u.c.session.CampaignID, who.CharacterID)
	if err != nil {
		return nil, err
	}
	cur, temp := min(hp.HP, now.GetHitPointsMax()), min(hp.Temp, maxTempHitPoints)
	_, after, err := u.s.vitalsOf(ctx, u.c, who.CharacterID, &playv1.AdjustCharacterVitalsRequest{HitPointsCurrent: &cur, HitPointsTemporary: &temp})
	if err != nil {
		return nil, err
	}
	if sameShape(after.GetWildShape(), hp.Shape) {
		return after, nil
	}
	return u.putShape(ctx, who, hp.Shape)
}

// sameShape says the Wild Shape form the vitals have is the one wanted (MR-037): the
// beast and its hit points, or the druid's own shape.
func sameShape(have *playv1.WildShapeState, want *shapeState) bool {
	if (have == nil) != (want == nil) {
		return false
	}
	return want == nil || (have.GetBeastKey() == want.Beast && have.GetHitPointsCurrent() == want.HP)
}

// putShape sets the form back: the beast and its hit points, or the druid's own
// shape, with the combatant's numbers.
func (u *undoer) putShape(ctx context.Context, who playdb.Combatant, want *shapeState) (*playv1.CharacterVitals, error) {
	beast, beastHP := "", int32(0)
	if want != nil {
		beast, beastHP = want.Beast, want.HP
	}
	restored, body, err := u.s.vitals.SetWildShape(ctx, u.c.tx, u.c.session.CampaignID, who.CharacterID, beast, beastHP)
	if err != nil {
		return nil, err
	}
	u.c.told = append(u.c.told, restored)
	return restored, applyBody(ctx, u.c, who, body)
}

// putCreatureOrVitals puts back the hit points of a combatant: an NPC or a creature
// holds its own, a player's character has them in the characters module, with the
// death saves.
func (u *undoer) putCreatureOrVitals(ctx context.Context, who playdb.Combatant, hp hpState, death *deathState) error {
	if holdsHP(who) {
		return u.setHP(ctx, who, hp)
	}
	v, err := u.putVitals(ctx, who, hp)
	if err != nil {
		return err
	}
	u.keep(v)
	return u.setDeath(ctx, who, death)
}

// spendSlotBack gives a character's spell slot back.
func (u *undoer) spendSlotBack(ctx context.Context, who playdb.Combatant, slot slotRef) error {
	v, err := u.s.spendSlot(ctx, u.c, who.CharacterID, slot, -1)
	if err != nil {
		return err
	}
	u.keep(v)
	return nil
}

// spendResourceBack gives back the use of a character's resource.
func (u *undoer) spendResourceBack(ctx context.Context, who playdb.Combatant, key string) error {
	v, err := u.s.spendResource(ctx, u.c, who.CharacterID, key, -1)
	if err != nil {
		return err
	}
	u.keep(v)
	return nil
}

// attackRolled: the action (or the reaction) comes back and the damage the hit
// opened goes away. A Bardic Inspiration die the roll used is held again.
func (u *undoer) attackRolled(ctx context.Context, ev actionEvent) error {
	if who, ok := u.find(ev.Actor); ok {
		if err := u.attackerBack(ctx, who, ev); err != nil {
			return err
		}
	}
	if ev.OfferID != "" { // an opportunity attack: its offer waits again
		if err := offerWaits(ctx, u.c, ev.OfferID); err != nil {
			return err
		}
	}
	if ev.Pending != "" {
		if err := u.c.q.DeletePendingDamage(ctx, ev.Pending); err != nil {
			return fmt.Errorf("delete the pending damage: %w", err)
		}
	}
	return nil
}

// attackerBack gives the attacker what the roll spent: the die, the extra action,
// the action or the reaction.
func (u *undoer) attackerBack(ctx context.Context, who playdb.Combatant, ev actionEvent) error {
	if ev.Res != nil && ev.Res.Kind == resBardicUse {
		sides, expires, from := ev.Res.Sides, ev.Res.ExpiresRound, ev.Res.FromID
		if err := u.c.q.SetCombatantInspirationDie(ctx, playdb.SetCombatantInspirationDieParams{ID: who.ID, Sides: &sides, FromID: &from, ExpiresRound: &expires}); err != nil {
			return fmt.Errorf("hold the die again: %w", err)
		}
	}
	if ev.ExtraUsed {
		if err := u.giveExtraActionBack(ctx, who); err != nil {
			return err
		}
	}
	if ev.AsReaction {
		return u.setEconomy(ctx, who, who.ActionUsed, who.BonusActionUsed, ev.ReactionBefore, who.Dashed)
	}
	bonus := who.BonusActionUsed
	if ev.AsBonus {
		bonus = ev.BonusBefore
	}
	if err := u.setEconomy(ctx, who, ev.ActionBefore, bonus, who.ReactionUsed, who.Dashed); err != nil {
		return err
	}
	return u.attackCounters(ctx, who, ev)
}

// attackCounters puts back the attacks made, the attack of the action and the
// Flurry strikes left, and the running start.
func (u *undoer) attackCounters(ctx context.Context, who playdb.Combatant, ev actionEvent) error {
	if err := u.setAttacks(ctx, who, ev.AttacksBefore); err != nil {
		return err
	}
	if err := u.setAttackState(ctx, who, ev.AttackKeyBefore, ev.FlurryBefore); err != nil {
		return err
	}
	return u.setRun(ctx, who, ev.RunBefore)
}

// damageRolled: the damage waits to be rolled again; an NPC that took it is as it
// was, and a character a heal reached has its hit points and death saves back. A
// spell's roll settled every pending damage of the cast.
func (u *undoer) damageRolled(ctx context.Context, ev actionEvent) error {
	if err := putBackMove(ctx, u.c, ev.Target, ev.ReturnedFrom); err != nil {
		return err
	}
	hits := ev.Settled
	if len(hits) == 0 {
		hits = []damageHit{{Pending: ev.Pending, Target: ev.Target, Applied: ev.Applied, Before: ev.Before, DeathBefore: ev.DeathBefore}}
	}
	for _, h := range hits {
		if err := u.hitRolledBack(ctx, h); err != nil {
			return err
		}
	}
	return u.partsSpentBack(ctx, ev)
}

// hitRolledBack makes one pending damage wait for its roll again, and puts back the
// hit points it changed.
func (u *undoer) hitRolledBack(ctx context.Context, h damageHit) error {
	if _, err := u.c.q.ClearPendingDamageRoll(ctx, h.Pending); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("put back the pending damage: %w", err)
	}
	who, ok := u.find(h.Target)
	if !ok || !h.Applied || h.Before == nil {
		return nil
	}
	return u.putCreatureOrVitals(ctx, who, *h.Before, h.DeathBefore)
}

// partsSpentBack gives back what the parts of the roll spent: the slot a Divine
// Smite used, a table maneuver's use and the once-per-turn marks of Sneak Attack
// and Colossus Slayer.
func (u *undoer) partsSpentBack(ctx context.Context, ev actionEvent) error {
	who, ok := u.find(ev.Actor)
	if !ok {
		return nil
	}
	if who.Kind == kindPlayer {
		if ev.Slot != nil {
			if err := u.spendSlotBack(ctx, who, *ev.Slot); err != nil {
				return err
			}
		}
		for _, key := range ev.ManeuverSpent {
			if err := u.spendResourceBack(ctx, who, key); err != nil {
				return err
			}
		}
	}
	if ev.OnceBefore == nil {
		return nil
	}
	if err := u.c.q.SetCombatantOncePerTurn(ctx, playdb.SetCombatantOncePerTurnParams{
		ID: who.ID, SneakAttackTurn: nilIfEmpty(ev.OnceBefore.Sneak), ColossusSlayerTurn: nilIfEmpty(ev.OnceBefore.Colossus),
	}); err != nil {
		return fmt.Errorf("put back the once-per-turn marks: %w", err)
	}
	return nil
}

// damageApplied: the character's vitals as they were, its death saves too, and the
// damage waits for the master again.
func (u *undoer) damageApplied(ctx context.Context, ev actionEvent) error {
	if err := putBackMove(ctx, u.c, ev.Target, ev.ReturnedFrom); err != nil {
		return err
	}
	who, ok := u.find(ev.Target)
	if !ok || ev.Before == nil {
		return nil
	}
	after, err := u.putVitals(ctx, who, *ev.Before)
	if err != nil {
		return err
	}
	u.keep(after)
	if ev.Relentless { // the use of Relentless Endurance comes back with the hit points
		if err := u.spendResourceBack(ctx, who, resRelentlessEndurance); err != nil {
			return err
		}
	}
	if err := u.setDeath(ctx, who, ev.DeathBefore); err != nil {
		return err
	}
	if _, err := u.c.q.ClearPendingDamageApplied(ctx, ev.Pending); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("put back the pending damage: %w", err)
	}
	return nil
}

// damageDiscarded: the damage is back in the status it was discarded from.
func (u *undoer) damageDiscarded(ctx context.Context, ev actionEvent) error {
	prev := ev.PrevStatus
	if prev != pendingAwaitingReaction && prev != pendingAwaitingRoll && prev != pendingRolled {
		prev = pendingRolled // never: only these can be discarded
	}
	return u.setStatus(ctx, ev.Pending, prev)
}

// hideResolved: the master's reveal, the hiding comes back as it was.
func (u *undoer) hideResolved(ctx context.Context, ev actionEvent) error {
	return restoreHiding(ctx, u.c, ev.Actor, ev.HidBefore)
}

// combatantMoved: the square, the movement walked, the running start and the
// master's cover mark are as they were: a move, a jump (the movement a jump spent
// comes back) and the cover a move cleared. An event written before the tenths of
// a foot has nothing to put back.
func (u *undoer) combatantMoved(ctx context.Context, ev actionEvent) error {
	who, ok := u.find(ev.Actor)
	if ev.From == nil {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOTHING_TO_UNDO, "this move cannot be undone")
	}
	if !ok {
		return nil
	}
	if err := u.moverBack(ctx, who, ev); err != nil {
		return err
	}
	// Standing up from Prone also took the condition off (combat_standup.go).
	if ev.CondSet {
		if err := u.setConditionsBack(ctx, who.ID, ev.CondBefore); err != nil {
			return err
		}
	}
	// The creature the mover dragged along goes back to where it stood (combat_drag.go).
	if ev.Dragged != "" && ev.DraggedFrom != nil && ev.DraggedFrom.Placed {
		if err := u.c.q.SetCombatantPlace(ctx, playdb.SetCombatantPlaceParams{
			ID: ev.Dragged, GridCol: &ev.DraggedFrom.Col, GridRow: &ev.DraggedFrom.Row, CoverMark: "none",
		}); err != nil {
			return fmt.Errorf("put back the dragged creature: %w", err)
		}
	}
	// The offers the move made go with it. Only the move that is the last action
	// is undone, so they are all still pending: an answer comes after it, and has
	// to be undone first (which makes the offer wait again).
	if ev.MoveID != "" {
		if err := u.c.q.DeleteOpportunityOffersOfMove(ctx, ev.MoveID); err != nil {
			return fmt.Errorf("take the opportunity offers away: %w", err)
		}
	}
	return nil
}

// moverBack puts the mover where it stood, with what it had walked and its cover mark.
func (u *undoer) moverBack(ctx context.Context, who playdb.Combatant, ev actionEvent) error {
	var col, row *int32
	if ev.From.Placed {
		col, row = &ev.From.Col, &ev.From.Row
	}
	cover := ev.From.Cover
	if cover == "" {
		cover = "none"
	}
	if err := u.c.q.SetCombatantMove(ctx, playdb.SetCombatantMoveParams{
		ID: who.ID, GridCol: col, GridRow: row, MovementUsedFt: ev.From.UsedDFt / undoDFtPerFt, MovementUsedDft: ev.From.UsedDFt, LastMoveDft: ev.From.LastDFt, CoverMark: cover,
	}); err != nil {
		return fmt.Errorf("put back the move: %w", err)
	}
	return nil
}

// trapTriggered: the trap armed again, the hit points and conditions it changed put
// back, and the pending damages it opened gone (MR-035).
func (u *undoer) trapTriggered(ctx context.Context, ev actionEvent) error {
	if ev.Trap == nil {
		return nil
	}
	return u.s.takeBackTrap(ctx, u.c, *ev.Trap, u.find)
}

// actionTaken: the economy and every state the action began come back, then what
// the resource it used spent, and the hit points of a heal.
func (u *undoer) actionTaken(ctx context.Context, ev actionEvent) error {
	who, ok := u.find(ev.Actor)
	if !ok {
		return nil
	}
	if err := u.actionStatesBack(ctx, who, ev); err != nil {
		return err
	}
	if err := u.attackCounters(ctx, who, ev); err != nil {
		return err
	}
	if ev.Res != nil {
		if err := u.takeBackResource(ctx, ev, who); err != nil {
			return err
		}
	}
	return u.actionSpentBack(ctx, who, ev)
}

// actionStatesBack gives back the economy of the action, and the states and flags it
// set: the extra action, the rage or dodge it began, the Disengage, Step of the
// Wind's doubled jump and Action Surge.
func (u *undoer) actionStatesBack(ctx context.Context, who playdb.Combatant, ev actionEvent) error {
	if err := u.setEconomy(ctx, who, ev.ActionBefore, ev.BonusBefore, ev.ReactionBefore, ev.DashedBefore); err != nil {
		return err
	}
	if ev.ExtraUsed {
		if err := u.giveExtraActionBack(ctx, who); err != nil {
			return err
		}
	}
	if ev.StateID != "" { // the state the action began: a rage, a dodge, a reckless attack
		if err := u.c.q.DeleteCombatantState(ctx, ev.StateID); err != nil {
			return fmt.Errorf("take the state off: %w", err)
		}
	}
	if err := u.c.q.SetCombatantDisengaged(ctx, playdb.SetCombatantDisengagedParams{ID: who.ID, Disengaged: ev.DisengagedBefore}); err != nil {
		return fmt.Errorf("put back the disengage: %w", err)
	}
	if ev.JumpDoubled {
		if err := u.c.q.SetCombatantJumpDoubled(ctx, playdb.SetCombatantJumpDoubledParams{ID: who.ID, JumpDoubled: ev.JumpDoubledBefore}); err != nil {
			return fmt.Errorf("put back the jump distance: %w", err)
		}
	}
	if err := u.c.q.SetCombatantActionSurged(ctx, playdb.SetCombatantActionSurgedParams{ID: who.ID, ActionSurged: ev.SurgedBefore}); err != nil {
		return fmt.Errorf("put back the action surge: %w", err)
	}
	return nil
}

// actionSpentBack gives back what the action spent: the points Slow Fall took off a
// fall, the use of a class resource, and the hit points a heal gave.
func (u *undoer) actionSpentBack(ctx context.Context, who playdb.Combatant, ev actionEvent) error {
	for _, cut := range ev.FallCuts { // the points Slow Fall took off a fall come back
		if err := u.c.q.SetPendingDamageLanding(ctx, playdb.SetPendingDamageLandingParams{ID: cut.Pending, Amount: &cut.Before}); err != nil {
			return fmt.Errorf("put back the fall damage: %w", err)
		}
	}
	if ev.Resource != "" && who.Kind == kindPlayer {
		if err := u.spendResourceBack(ctx, who, ev.Resource); err != nil {
			return err
		}
	}
	if ev.Heal && ev.Before != nil {
		return u.putCreatureOrVitals(ctx, who, *ev.Before, ev.DeathBefore)
	}
	return nil
}

// hitPointsAdjusted: the master's correction of a combatant's hit points.
func (u *undoer) hitPointsAdjusted(ctx context.Context, ev actionEvent) error {
	if who, ok := u.find(ev.Actor); ok && ev.Before != nil {
		return u.setHP(ctx, who, *ev.Before)
	}
	return nil
}

// spellCast: the slot and the economy come back, the concentration is as it was,
// and the pending damages the cast opened go away.
func (u *undoer) spellCast(ctx context.Context, ev actionEvent) error {
	who, ok := u.find(ev.Actor)
	if !ok {
		return nil
	}
	steps := []func(context.Context, playdb.Combatant, actionEvent) error{
		u.castCounters, u.castDieBack, u.castEffectsBack, u.castSpentBack, u.castConcentrationBack, u.castCreaturesBack, u.castRevealBack, u.castHitsBack,
	}
	for _, step := range steps {
		if err := step(ctx, who, ev); err != nil {
			return err
		}
	}
	return nil
}

// castCounters puts back the economy, the running start and the kinds of spell cast.
func (u *undoer) castCounters(ctx context.Context, who playdb.Combatant, ev actionEvent) error {
	if err := u.setEconomy(ctx, who, ev.ActionBefore, ev.BonusBefore, ev.ReactionBefore, ev.DashedBefore); err != nil {
		return err
	}
	if err := u.setRun(ctx, who, ev.RunBefore); err != nil {
		return err
	}
	if err := u.c.q.SetCombatantSpellsCast(ctx, playdb.SetCombatantSpellsCastParams{ID: who.ID, SpellCast: ev.SpellCastBefore, BonusSpellCast: ev.BonusSpellBefore}); err != nil {
		return fmt.Errorf("put back the spells cast: %w", err)
	}
	return nil
}

// castDieBack holds again the Bardic Inspiration die the spell attack used.
func (u *undoer) castDieBack(ctx context.Context, who playdb.Combatant, ev actionEvent) error {
	if ev.Die == nil || ev.Die.Kind != resBardicUse {
		return nil
	}
	sides, expires := ev.Die.Sides, ev.Die.ExpiresRound
	var from *string
	if ev.Die.FromID != "" {
		from = &ev.Die.FromID
	}
	if err := u.c.q.SetCombatantInspirationDie(ctx, playdb.SetCombatantInspirationDieParams{ID: who.ID, Sides: &sides, FromID: from, ExpiresRound: &expires}); err != nil {
		return fmt.Errorf("hold the die again: %w", err)
	}
	return nil
}

// castEffectsBack takes away the effects the cast put on its targets, and the mark
// of Hunter's Mark.
func (u *undoer) castEffectsBack(ctx context.Context, who playdb.Combatant, ev actionEvent) error {
	if ev.Lasting != nil { // the effects the cast put on its targets
		for _, id := range ev.Lasting.Effects {
			if err := u.c.q.DeleteLastingEffect(ctx, id); err != nil {
				return fmt.Errorf("take the effects of the cast away: %w", err)
			}
		}
		if err := u.s.refreshCombatants(ctx, u.c, ev.Lasting.Targets...); err != nil {
			return err
		}
	}
	if ev.Key == huntersMark { // the mark the cast put on its target
		return u.s.clearHuntersMark(ctx, u.c, who)
	}
	return nil
}

// castSpentBack gives back the sorcery points and the slot of a player's cast.
func (u *undoer) castSpentBack(ctx context.Context, who playdb.Combatant, ev actionEvent) error {
	if ev.Res != nil && ev.Res.Kind == resMetamagic && who.Kind == kindPlayer {
		v, err := u.s.spendResource(ctx, u.c, who.CharacterID, rules.SorceryPointsKey, -ev.Res.Spent)
		if err != nil {
			return err
		}
		u.keep(v)
	}
	if ev.Slot != nil && who.Kind == kindPlayer {
		return u.spendSlotBack(ctx, who, *ev.Slot)
	}
	return nil
}

// castConcentrationBack puts back the spell the caster concentrated on before.
func (u *undoer) castConcentrationBack(ctx context.Context, who playdb.Combatant, ev actionEvent) error {
	if !ev.Concentrate {
		return nil
	}
	var before *string
	if ev.ConcBefore != "" {
		before = &ev.ConcBefore
	}
	if err := u.c.q.SetCombatantConcentration(ctx, playdb.SetCombatantConcentrationParams{ID: who.ID, ConcentrationSpell: before}); err != nil {
		return fmt.Errorf("put back the concentration: %w", err)
	}
	return nil
}

// castCreaturesBack: the creatures the cast made go away, and the ones it dismissed
// (the old concentration's) are back with their group's initiative (MR-037).
func (u *undoer) castCreaturesBack(ctx context.Context, _ playdb.Combatant, ev actionEvent) error {
	if len(ev.Created) > 0 {
		if err := u.s.removeCreatureCombatants(ctx, u.c, ev.Created); err != nil {
			return err
		}
	}
	if len(ev.Dismissed) > 0 {
		return u.s.rejoinCreatures(ctx, u.c, ev.Dismissed)
	}
	return nil
}

// castRevealBack: the question the cast left the master goes away, and the creatures
// it revealed are hidden again: the spell never happened (their line is the players'
// no more while they are hidden).
func (u *undoer) castRevealBack(ctx context.Context, _ playdb.Combatant, ev actionEvent) error {
	if ev.PendingReveal != "" {
		if err := u.c.q.DeleteHiddenReveal(ctx, ev.PendingReveal); err != nil {
			return fmt.Errorf("drop the hidden reveal: %w", err)
		}
	}
	for _, id := range ev.Revealed {
		if target, ok := u.find(id); ok {
			if err := u.c.q.SetCombatantHidden(ctx, playdb.SetCombatantHiddenParams{ID: target.ID, Hidden: true}); err != nil {
				return fmt.Errorf("hide the combatant again: %w", err)
			}
		}
	}
	return nil
}

// castHitsBack deletes the pending damages the cast opened and puts back what a
// spell that read hit points changed at once.
func (u *undoer) castHitsBack(ctx context.Context, _ playdb.Combatant, ev actionEvent) error {
	for _, h := range ev.Hits {
		for _, id := range append([]string{h.Pending}, h.More...) {
			if id == "" {
				continue
			}
			if err := u.c.q.DeletePendingDamage(ctx, id); err != nil {
				return fmt.Errorf("delete the pending damage: %w", err)
			}
		}
		// A spell that read hit points changed the targets at once: their hit
		// points, death saves and conditions come back.
		if target, ok := u.find(h.Target); ok && h.Fx != "" {
			if err := u.castTargetBack(ctx, target, h); err != nil {
				return err
			}
		}
	}
	return nil
}

// castTargetBack puts back a target's hit points, maximum, death saves and conditions.
func (u *undoer) castTargetBack(ctx context.Context, target playdb.Combatant, h castHit) error {
	if h.BonusBefore != nil && !holdsHP(target) { // Ajuda's bonus, before the hit points the bonus bounds
		if _, _, err := u.s.vitals.SetHitPointsMaxBonus(ctx, u.c.tx, u.c.session.CampaignID, target.CharacterID, *h.BonusBefore); err != nil {
			return err
		}
	}
	if h.Restore != nil {
		if err := u.putHitPoints(ctx, target, *h.Restore); err != nil {
			return err
		}
	}
	if h.MaxBefore != nil && holdsHP(target) { // after the hit points, which are the lower ones
		if err := u.c.q.SetCombatantHitPointsMax(ctx, playdb.SetCombatantHitPointsMaxParams{ID: target.ID, HpMax: h.MaxBefore, HpMaxBonus: num(h.BonusBefore)}); err != nil {
			return fmt.Errorf("put back the maximum hit points: %w", err)
		}
	}
	if err := u.setDeath(ctx, target, h.DeathBefore); err != nil {
		return err
	}
	if h.CondSet {
		return u.setConditionsBack(ctx, target.ID, h.CondBefore)
	}
	return nil
}

// putHitPoints puts the hit points of a combatant back: its own, or its character's.
func (u *undoer) putHitPoints(ctx context.Context, who playdb.Combatant, hp hpState) error {
	if holdsHP(who) {
		return u.setHP(ctx, who, hp)
	}
	after, err := u.putVitals(ctx, who, hp)
	if err != nil {
		return err
	}
	u.keep(after)
	return nil
}

// reactionUsed: the slot, the reaction and the armor class bonus come back, and the
// hit waits for the reaction again.
func (u *undoer) reactionUsed(ctx context.Context, ev actionEvent) error {
	who, ok := u.find(ev.Actor)
	if !ok {
		return nil
	}
	if err := u.reactionSpentBack(ctx, who, ev); err != nil {
		return err
	}
	if err := u.setEconomy(ctx, who, who.ActionUsed, who.BonusActionUsed, ev.ReactionBefore, who.Dashed); err != nil {
		return err
	}
	if err := u.s.reopenWindowOf(ctx, u.c, ev); err != nil {
		return err
	}
	if err := u.c.q.SetCombatantAcBonus(ctx, playdb.SetCombatantAcBonusParams{ID: who.ID, AcBonus: ev.ACBonusBefore}); err != nil {
		return fmt.Errorf("put back the armor class bonus: %w", err)
	}
	return u.hitsWaitAgain(ctx, ev)
}

// reactionSpentBack gives back the slot and the maneuver uses the reaction spent.
func (u *undoer) reactionSpentBack(ctx context.Context, who playdb.Combatant, ev actionEvent) error {
	if ev.Slot != nil {
		if who.Kind == kindPlayer {
			if err := u.spendSlotBack(ctx, who, *ev.Slot); err != nil {
				return err
			}
		} else if err := u.s.giveBackNPCSlot(ctx, u.c, who, *ev.Slot); err != nil { // an NPC's slot is counted on the combatant
			return err
		}
	}
	if who.Kind != kindPlayer {
		return nil
	}
	for _, key := range ev.ManeuverSpent { // a table maneuver's use
		if err := u.spendResourceBack(ctx, who, key); err != nil {
			return err
		}
	}
	return nil
}

// hitsWaitAgain makes the hit the reaction answered, and the others it stopped, wait
// for the reaction again.
func (u *undoer) hitsWaitAgain(ctx context.Context, ev actionEvent) error {
	if _, err := u.c.q.SetPendingDamageStatus(ctx, playdb.SetPendingDamageStatusParams{ID: ev.Pending, Status: pendingAwaitingReaction}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("put back the pending damage: %w", err)
	}
	for _, h := range ev.AlsoStopped {
		if _, err := u.c.q.SetPendingDamageStatus(ctx, playdb.SetPendingDamageStatusParams{ID: h.Pending, Status: h.PrevStatus}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("put back the other hit: %w", err)
		}
	}
	return nil
}

// reactionDeclined: the offer or the window waits for its answer again.
func (u *undoer) reactionDeclined(ctx context.Context, ev actionEvent) error {
	if ev.OfferID != "" { // an opportunity offer turned down or skipped: it waits again
		return offerWaits(ctx, u.c, ev.OfferID)
	}
	if err := u.s.reopenWindowOf(ctx, u.c, ev); err != nil {
		return err
	}
	return u.setStatus(ctx, ev.Pending, pendingAwaitingReaction)
}

// deathSaveRolled: the death saves as they were, and the hit points a natural 20
// had given.
func (u *undoer) deathSaveRolled(ctx context.Context, ev actionEvent) error {
	who, ok := u.find(ev.Actor)
	if !ok {
		return nil
	}
	if ev.Before != nil { // a natural 20 brought it back with 1 hit point
		after, err := u.putVitals(ctx, who, *ev.Before)
		if err != nil {
			return err
		}
		u.keep(after)
	}
	return u.setDeath(ctx, who, ev.DeathBefore)
}

// conditionsSet: the master's conditions, the concentration it ended and the
// creatures that went with it.
func (u *undoer) conditionsSet(ctx context.Context, ev actionEvent) error {
	who, ok := u.find(ev.Actor)
	if !ok {
		return nil
	}
	if ev.CondSet {
		if err := u.setConditionsBack(ctx, who.ID, ev.CondBefore); err != nil {
			return err
		}
	}
	if ev.ConcEnded != "" {
		if err := u.c.q.SetCombatantConcentration(ctx, playdb.SetCombatantConcentrationParams{ID: who.ID, ConcentrationSpell: &ev.ConcEnded}); err != nil {
			return fmt.Errorf("put back the concentration: %w", err)
		}
	}
	if len(ev.Dismissed) > 0 {
		return u.s.rejoinCreatures(ctx, u.c, ev.Dismissed)
	}
	return nil
}

func (u *undoer) wildShapeStarted(ctx context.Context, ev actionEvent) error {
	return u.s.shapeUndo(ctx, u.c, eventWildShapeStarted, ev, u.find, u.keep)
}

func (u *undoer) wildShapeEnded(ctx context.Context, ev actionEvent) error {
	return u.s.shapeUndo(ctx, u.c, eventWildShapeEnded, ev, u.find, u.keep)
}

func (u *undoer) familiarSight(ctx context.Context, ev actionEvent) error {
	return u.s.sightUndo(ctx, u.c, ev, u.find, u.keep)
}
