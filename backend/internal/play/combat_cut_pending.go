package play

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/reaction"
)

// The line of a Cutting Words that cut a held attack or spell attack. A reaction window holds the roll, the windows are answered one
// change each, and the d20 is replayed by the change of the last answer; when the roller is asked about it (a halfling's Lucky, a Bardic
// Inspiration die) the roll is written later, by the answer. Every bard's answer is written before the roll is, so none can say what the
// roll became. Each effective answer is marked (Waiting, with the hold's group and what it took off), the event of the roll names the
// group (CutGroup), and the log joins them when it reads: until then the line says nothing of the roll, and once the roll is written it
// says what it became. No event is ever changed.

// markWaiting marks the answer of a Cutting Words that took something off a roll a window holds, when the held action is an attack or a
// spell attack. A held check or a damage roll is not marked: its line is not completed this way. An answer that did nothing says so
// whatever the roll becomes, and is not marked either.
func (s *Service) markWaiting(ctx context.Context, c *combatTx, w playdb.ReactionWindow, re *reactionEvent, reduction int32) error {
	if w.HoldID == nil || re.Ineffect {
		return nil
	}
	h, err := c.q.GetReactionHold(ctx, playdb.GetReactionHoldParams{EncounterID: c.enc.ID, ID: *w.HoldID})
	if err != nil {
		return fmt.Errorf("find the hold of a Cutting Words window: %w", err)
	}
	if h.Kind == "attack" || h.Kind == "cast" {
		re.Waiting, re.Group, re.Reduction = true, w.GroupID, reduction
	}
	return nil
}

// answersOf are the answers of this change that belong to the windows of a held action.
func answersOf(c *combatTx, windows []playdb.ReactionWindow) []answeredRef {
	return slices.DeleteFunc(slices.Clone(c.answered), func(ref answeredRef) bool {
		return !slices.ContainsFunc(windows, func(w playdb.ReactionWindow) bool { return w.ID == ref.window })
	})
}

// cutGroupOfAnswer is the group of the reaction hold behind the roll being written, "" when the roll was no replay of a held action.
// The replay that asked nothing is run by the answer that released it, which knows its hold. The roll kept for a question is settled by
// the answer to it: its hold is marked as a window's and keeps the key of the request the window held, and the event that held that
// request, written under the key, names the group.
func (s *Service) cutGroupOfAnswer(ctx context.Context, c *combatTx) (string, error) {
	if c.replay != nil {
		return c.replay.hold.GroupID, nil
	}
	reply, answering := heldReplyOf(ctx)
	if !answering {
		return "", nil
	}
	h, err := newHeldStores(c.q).byIDOf(ctx, holdRef{Campaign: c.session.CampaignID, ID: reply.holdID})
	if err != nil {
		return "", fmt.Errorf("find the held roll: %w", err)
	}
	if !h.Window {
		return "", nil
	}
	row, err := c.q.GetSessionEventByIdempotencyKey(ctx, playdb.GetSessionEventByIdempotencyKeyParams{GameSessionID: c.session.ID, IdempotencyKey: &h.Key})
	if errors.Is(err, pgx.ErrNoRows) { // no event holds the roll: nothing to complete, and the answer goes on
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("find the event that held the roll: %w", err)
	}
	held, err := readEvent(row.Payload)
	if err != nil {
		return "", fmt.Errorf("read the event that held the roll: %w", err)
	}
	if held.Reaction == nil || !held.Reaction.Hold {
		return "", nil
	}
	return held.Reaction.Group, nil
}

// cutJoin is the roll that settled the question of a Cutting Words line, and the answer as it reads once the roll is written.
type cutJoin struct {
	by   actionEvent
	done *reactionEvent
}

// cutGroup is what the log has read of one hold: the answers that wait for its roll, and the roll once it is written. The roll of a
// replay that asked nothing is written before the answer of the bard who released it, so either may come first.
type cutGroup struct {
	cuts   []*logEntry
	roll   *actionEvent
	rollID string
}

// registerCut remembers an answer that waits for the roll of its group.
func (b *logBuilder) registerCut(entry *logEntry) {
	re := entry.ev.Reaction
	if re.Waiting && re.Group != "" && re.Kind == string(reaction.CuttingWords) {
		g := b.cutGroup(re.Group)
		g.cuts = append(g.cuts, entry)
		b.completeCuts(g)
	}
}

// joinCut lands the event of the roll of a held action on the lines of the Cutting Words of its group. An undone roll never gets here,
// and its lines wait again.
func (b *logBuilder) joinCut(id string, final actionEvent) {
	g := b.cutGroup(final.CutGroup)
	g.roll, g.rollID = &final, id
	b.completeCuts(g)
}

func (b *logBuilder) cutGroup(id string) *cutGroup {
	if b.byCut[id] == nil {
		b.byCut[id] = &cutGroup{}
	}
	return b.byCut[id]
}

// completeCuts reads the roll on every line of the group: they host its event, so that the line's number moves with it, and read what
// the roll became with the whole of what the group took off.
func (b *logBuilder) completeCuts(g *cutGroup) {
	if g.roll == nil {
		return
	}
	var taken int32
	for _, host := range g.cuts {
		taken += host.ev.Reaction.Reduction
	}
	for _, host := range g.cuts {
		if done, ok := host.ev.Reaction.completedBy(*g.roll, taken); ok {
			if host.cut == nil {
				host.hosts = append(host.hosts, g.rollID)
			}
			host.cut = &cutJoin{by: *g.roll, done: &done}
		}
	}
}

// completedBy is the answer as it reads once the roll that settled its question is known, by the rule of completeAnswers: a weapon
// attack's outcome, armor class and total, or the one roll of a spell attack's; Before is the total with the die back on it.
func (r reactionEvent) completedBy(final actionEvent, taken int32) (reactionEvent, bool) {
	outcome, ac, total := final.Outcome, final.TargetAC, final.Total
	if len(final.Hits) > 0 {
		if len(final.Hits) != 1 {
			return r, false
		}
		outcome, ac, total = final.Hits[0].Outcome, final.Hits[0].TargetAC, final.Hits[0].Total
	}
	r.Outcome, r.AC, r.After = outcome, ac, total
	r.Before = total + taken
	r.Waiting = false
	return r, true
}

// reactionFor is the answer as the viewer reads it: the event's own while the roll it waits for is not written or the viewer does not hear
// of that event, else the completed one. The viewer who hears nothing of the roll (a hidden roller's, a fogged one's) reads
// no more than the others did before it.
func (e *logEntry) reactionFor(v combatViewer) *reactionEvent {
	if e.cut != nil && e.cut.by.heardBy(v) {
		return e.cut.done
	}
	return e.ev.Reaction
}
