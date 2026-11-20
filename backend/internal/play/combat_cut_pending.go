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

// The line of a Cutting Words whose held roll asked its roller. A reaction window holds an attack or a spell attack, the windows are
// answered, and the replay rolls the d20; when the roller is asked about it (a halfling's Lucky, a Bardic Inspiration die) the roll
// is kept and written only when the answer comes, in a transaction of its own. The bard's answer is written first, so it cannot say
// what the roll became. Its event is marked (Asked, with the hold's group and what the group took off), the event the answer writes
// names the group (CutGroup), and the log joins them when it reads: until then the line says nothing of the roll, and once the roll
// is written it says what a roll that asked nothing says (completeAnswers). No event is ever changed.

// markAsked marks the answers of the windows of a held attack or spell attack whose replay asked its roller, so the log completes them
// when the roll is written. A held check asks the same way and is not marked: its line is not completed this way, and keeps what the answer wrote when the
// replay asked, before the roll had a total.
func markAsked(c *combatTx, h playdb.ReactionHold, windows []playdb.ReactionWindow, st *replayState) {
	if h.Kind != "attack" && h.Kind != "cast" {
		return
	}
	for _, ref := range answersOf(c, windows) {
		if ref.ev.Kind == string(reaction.CuttingWords) && !ref.ev.Ineffect { // an answer that did nothing says so whatever the roll becomes
			ref.ev.Asked, ref.ev.Group, ref.ev.Reduction = true, h.GroupID, st.reduction
		}
	}
}

// answersOf are the answers of this change that belong to the windows of a held action.
func answersOf(c *combatTx, windows []playdb.ReactionWindow) []answeredRef {
	return slices.DeleteFunc(slices.Clone(c.answered), func(ref answeredRef) bool {
		return !slices.ContainsFunc(windows, func(w playdb.ReactionWindow) bool { return w.ID == ref.window })
	})
}

// cutGroupOfAnswer is the group of the reaction hold behind the roll that the answer being run settles, "" when the roll was no
// replay of a held action. The hold of the roll is marked as a window's and keeps the key of the request the window held; the
// event that held that request, written under the key, names the group.
func (s *Service) cutGroupOfAnswer(ctx context.Context, c *combatTx) (string, error) {
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

// registerCut remembers an answer that waits for the roll of its group.
func (b *logBuilder) registerCut(entry *logEntry) {
	re := entry.ev.Reaction
	if re.Asked && re.Group != "" && re.Kind == string(reaction.CuttingWords) {
		b.byCut[re.Group] = append(b.byCut[re.Group], entry)
	}
}

// joinCut lands the event of a roll an answer settled on the lines of the Cutting Words of its group: they host it, so that the line's
// number moves with it, and read what the roll became. An undone roll never gets here, and its lines wait again.
func (b *logBuilder) joinCut(id string, final actionEvent) {
	for _, host := range b.byCut[final.CutGroup] {
		if done, ok := host.ev.Reaction.completedBy(final); ok {
			host.hosts = append(host.hosts, id)
			host.cut = &cutJoin{by: final, done: &done}
		}
	}
}

// completedBy is the answer as it reads once the roll that settled its question is known, by the rule of completeAnswers: a weapon
// attack's outcome, armor class and total, or the one roll of a spell attack's; Before is the total with the die back on it.
func (r reactionEvent) completedBy(final actionEvent) (reactionEvent, bool) {
	outcome, ac, total := final.Outcome, final.TargetAC, final.Total
	if len(final.Hits) > 0 {
		if len(final.Hits) != 1 {
			return r, false
		}
		outcome, ac, total = final.Hits[0].Outcome, final.Hits[0].TargetAC, final.Hits[0].Total
	}
	r.Outcome, r.AC, r.After = outcome, ac, total
	r.Before = total + r.Reduction
	r.Asked = false
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
