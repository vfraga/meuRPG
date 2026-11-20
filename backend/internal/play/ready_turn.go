package play

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// What happens to a readied action besides Ready: it is lost when the readier's next turn starts, it goes with the combat,
// the master's undo of the Ready takes it back, and the views show it to the master and the readier's player.

// loseReadiedAtTurnStart ends the readied actions of the combatants whose turn starts: a readied action lasts until the
// start of the readier's next turn (SRD 5.1, "Ready"). Nothing comes back: the action was spent, the reaction never was.
func loseReadiedAtTurnStart(ctx context.Context, c *combatTx, ids []string, round int32) error {
	for _, id := range ids {
		row, err := c.q.GetWaitingReadiedOf(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return fmt.Errorf("read the readied action: %w", err)
		}
		if err := loseReadied(ctx, c, row, round); err != nil {
			return err
		}
	}
	return nil
}

// loseReadiedOfDefeated ends the readied actions of the combatants that are defeated, whatever defeated them (damage, a
// death the master confirmed, exhaustion, the master's hand), and of the creatures taken out of the order (dismissed):
// their turns are skipped, so the start of the next turn, which ends a readied action, would never come. It runs in
// settleChange, after every change to a combat, so no path that writes `defeated` or `dismissed` is missed; the line is
// written before the change's own event, and the undo of that change goes past it (chainReadiedDefeat) and leaves the row
// lost: a revived or returned readier does not get it back, and its Ready cannot be undone any more (chainReadied).
// FireReadied refuses a defeated, dismissed, down or incapacitated readier all the same: this keeps none waiting, it never
// replaces that check. It takes the waiting rows and every combatant (the dismissed too) and gives back the rows still waiting.
func loseReadiedOfDefeated(ctx context.Context, c *combatTx, rows []playdb.ReadiedAction, cs []playdb.Combatant) ([]playdb.ReadiedAction, error) {
	waiting := rows[:0:0]
	for _, row := range rows {
		who, ok := combatantByID(cs, row.CombatantID)
		if !ok || (!who.Defeated && !who.Dismissed) {
			waiting = append(waiting, row)
			continue
		}
		if err := markReadiedLost(ctx, c, row, who, c.enc.Round, who.Defeated, who.Dismissed); err != nil {
			return nil, err
		}
	}
	return waiting, nil
}

// loseReadied marks the row lost and writes its line.
func loseReadied(ctx context.Context, c *combatTx, row playdb.ReadiedAction, round int32) error {
	cs, err := listCombatants(ctx, c)
	if err != nil {
		return err
	}
	who, _ := combatantByID(cs, row.CombatantID)
	return markReadiedLost(ctx, c, row, who, round, false, false)
}

// markReadiedLost marks the row lost and writes its readied_lost line; defeated and dismissed say the readier's defeat,
// or its leaving the order, ended it.
func markReadiedLost(ctx context.Context, c *combatTx, row playdb.ReadiedAction, who playdb.Combatant, round int32, defeated, dismissed bool) error {
	if err := c.q.SetReadiedActionStatus(ctx, playdb.SetReadiedActionStatusParams{ID: row.ID, Status: readiedLost}); err != nil {
		return fmt.Errorf("lose the readied action: %w", err)
	}
	ev := readiedRowEvent(row, who, round)
	ev.Readied.Defeated, ev.Readied.Dismissed = defeated, dismissed
	return insertEvent(ctx, c, eventReadiedLost, &c.actorUserID, nil, ev)
}

// chainReadiedDefeat: a readied action lost to its readier's defeat (or dismissal) is written before the event of the change that
// defeated it, and never closes the undo's chain: the undo acts on that change, and the row stays lost.
func chainReadiedDefeat(c chainEvent) chainVerdict {
	if c.row.Kind != eventReadiedLost {
		return chainGoOn
	}
	if ev, ok := c.payload(); ok && ev.Readied != nil && (ev.Readied.Defeated || ev.Readied.Dismissed || ev.Readied.Ended == readiedBroken) {
		return chainSkip
	}
	return chainGoOn
}

// dropReadied deletes the readied actions of a combat that ends: none can fire any more.
func dropReadied(ctx context.Context, c *combatTx) error {
	if err := c.q.DeleteReadiedActionsOfEncounter(ctx, c.enc.ID); err != nil {
		return fmt.Errorf("drop the readied actions: %w", err)
	}
	return nil
}

// readiedBack is the undo of a Ready: the economy and the turn as TakeAction's undo puts them back, and the row goes.
func (u *undoer) readiedBack(ctx context.Context, ev actionEvent) error {
	if err := u.actionTaken(ctx, ev); err != nil {
		return err
	}
	if ev.Readied == nil {
		return nil
	}
	if err := u.readiedSpellBack(ctx, ev); err != nil {
		return err
	}
	if err := u.c.q.DeleteReadiedAction(ctx, ev.Readied.ID); err != nil {
		return fmt.Errorf("take the readied action back: %w", err)
	}
	return nil
}

// readiedView is the readied action the combatant holds, for the master and its player (the caller checks who reads).
func readiedView(rows []playdb.ReadiedAction, c playdb.Combatant, names func(key string) string) *playv1.ReadiedAction {
	for _, row := range rows {
		if row.CombatantID != c.ID || row.Status != readiedWaiting {
			continue
		}
		out := &playv1.ReadiedAction{
			Id: row.ID, TriggerText: row.TriggerText, Kind: readiedKindToProto[row.Kind], MoveFt: row.MoveFt, Round: row.Round,
			Status: readiedStatusToProto[row.Status], SlotLevel: row.SlotLevel,
		}
		if row.ActionKey != nil {
			out.ActionKey, out.ActionNamePt = *row.ActionKey, row.ActionNamePt
			if out.ActionNamePt == "" {
				out.ActionNamePt = names(*row.ActionKey)
			}
		}
		if row.TargetID != nil {
			out.TargetCombatantId = *row.TargetID
		}
		return out
	}
	return nil
}

// readiedLine is the log handler of the events of a readied action: a line of the kind for everybody who sees the
// readier; fillReadied gives the trigger and the action to the master and the readier's player alone.
func readiedLine(kind playv1.CombatLogKind) logHandler {
	return func(_ *logBuilder, _ playdb.ListEncounterEventsRow, entry *logEntry) bool {
		entry.kind = kind
		return entry.ev.Readied != nil && !entry.ev.Readied.Opened // the fire itself is told by the window, not the log
	}
}

// fillReadied writes the readied action of the line for the master and the readier's player; any other player reads the
// kind and the actor alone (RN-10), with no concentration ended.
func (e *logEntry) fillReadied(c *logViewing, out *playv1.CombatLogEntry) bool {
	if !c.v.master && !c.v.owns(c.actor) { // nor the concentration it ended: that would say the Ready was a spell
		out.Key, out.ConcentrationEndedKey = "", ""
		return true
	}
	// The concentration a readied spell ended, as a cast's line says it.
	out.ConcentrationEndedKey = e.ev.ConcEnded
	if e.ev.Readied.holdsSpell() && e.kind == playv1.CombatLogKind_COMBAT_LOG_KIND_ACTION_READIED { // the slot it spent
		out.Spell = &playv1.CombatLogSpell{Slot: slotProto(e.ev.Slot, c.v, c.actor), Concentrating: true, ConcentrationEndedKey: e.ev.ConcEnded}
	}
	r := e.ev.Readied
	name := r.Name
	if name == "" && r.ActionKey != "" {
		name = c.names.of(c.ctx, c.actor, r.ActionKey)
	}
	detail := &playv1.CombatLogReadied{
		ReadiedId: r.ID, TriggerText: r.Trigger, Kind: readiedKindToProto[r.Kind], ActionKey: r.ActionKey, ActionNamePt: name,
		Canceled: r.Canceled, Status: readiedStatusToProto[r.Ended],
	}
	switch e.kind {
	case playv1.CombatLogKind_COMBAT_LOG_KIND_READIED_FIRED:
		out.ReadiedFired = detail
	case playv1.CombatLogKind_COMBAT_LOG_KIND_READIED_LOST:
		out.ReadiedLost = detail
	default:
		out.Readied = detail
	}
	return true
}
