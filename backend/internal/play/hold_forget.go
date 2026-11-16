package play

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/reaction"
)

// The one way to drop a held roll. A spend of the Inspiration deletes the character's row and the hold remembers it
// (inspiration_holds.favor_spent); a hold dropped before it is answered is a roll that never happens, with no event to undo, so the
// Inspiration is lost unless it is given back here. Every delete of inspiration_holds goes through this file, before the row is gone:
// the generated Delete* queries are called nowhere else, and no path may rely on the foreign key's cascade (combatants and
// encounters delete their holds by it), so a path that deletes a combatant calls forgetHoldsOf first (archtest).

// favorGiveBackIf gives the Inspiration back when it was spent on a held roll that is dropped before it was answered: no roll event
// exists for that spend, so nothing else would put it back. A hold that was answered, or that spent nothing, gives nothing. The refund
// prompt that spend opened (promptID, which the hold remembers: nil when the spend opened none) closes as kept: the Inspiration is back, so
// there is nothing left to give.
func favorGiveBackIf(ctx context.Context, q *playdb.Queries, unsettled bool, campaignID, characterID string, promptID *string) error {
	if !unsettled || characterID == "" {
		return nil
	}
	return favorBackOfRoll(ctx, q, campaignID, characterID, deref(promptID))
}

// favorBackOfRoll gives the Inspiration a roll spent back when the roll is gone (its undo, a held roll forgotten) and closes as kept
// the refund prompt of that spend. When the master already answered that prompt "Devolver", the Inspiration was given back once: it is not
// given again (the character may have passed it away or spent it since), and the prompt keeps its answer. promptID is the prompt the spend
// opened, which the roll's event or its hold remembers (never found by a key, which two spends can share); empty when the spend opened none,
// and then there is nothing to read or to close and the give-back is unconditional.
func favorBackOfRoll(ctx context.Context, q *playdb.Queries, campaignID, characterID, promptID string) error {
	if promptID != "" {
		prompt, err := q.GetFavorRefundPrompt(ctx, playdb.GetFavorRefundPromptParams{ID: promptID, CampaignID: campaignID})
		switch {
		case errors.Is(err, pgx.ErrNoRows): // the prompt is gone: there is no answer to honor
		case err != nil:
			return fmt.Errorf("read the refund of the roll: %w", err)
		case prompt.Status == refundReturned:
			return nil
		}
	}
	if err := favorGiveBack(ctx, q, campaignID, characterID); err != nil {
		return err
	}
	return keepRefundOfRoll(ctx, q, campaignID, promptID)
}

// keepRefundOfRoll closes as kept the refund prompt of a roll that is gone: the Inspiration went back with the roll, so the master's
// question has no spend left to refund. No event is written for it: the roll's own undo is the record, and the transaction that drops the
// roll moves the session's number. Only that prompt closes, so another roll's of the same character, even under the same key, stays.
func keepRefundOfRoll(ctx context.Context, q *playdb.Queries, campaignID, promptID string) error {
	if promptID == "" {
		return nil
	}
	if _, err := q.KeepFavorRefundPrompt(ctx, playdb.KeepFavorRefundPromptParams{ID: promptID, CampaignID: campaignID}); err != nil {
		return fmt.Errorf("keep the refund question of the roll: %w", err)
	}
	return nil
}

// refundSpentFavor is the master's refund of an Inspiration a roll spent for nothing (favor_refund.go): the one other way the row is given
// back. The roller must have none now: one that has it (the spend was given back already by the roll's undo, or the master granted another)
// answers ALREADY_HAS, so a spend is never refunded twice and the one-row rule is never the silent reason.
func refundSpentFavor(ctx context.Context, q *playdb.Queries, campaignID, characterID string) error {
	if _, err := q.InsertCharacterFavor(ctx, playdb.InsertCharacterFavorParams{CharacterID: characterID, CampaignID: campaignID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return favorBlocked(playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_ALREADY_HAS, "the character has the inspiration: there is nothing to give back")
		}
		return fmt.Errorf("give the character's inspiration back: %w", err)
	}
	return nil
}

// forgetHoldsOf drops every held roll of an owner (a combatant, or a character outside a combat), giving back the Inspiration the
// unanswered ones spent. The owner's query also drops the character's holds outside any combat (combatant_id IS NULL) when its combatant
// leaves or the combat ends: those are refunded the same way.
func forgetHoldsOf(ctx context.Context, q *playdb.Queries, owner holdOwner) error {
	rows, err := q.DeleteInspirationHoldsOf(ctx, playdb.DeleteInspirationHoldsOfParams(owner))
	if err != nil {
		return fmt.Errorf("delete the held rolls: %w", err)
	}
	for _, r := range rows {
		if err := favorGiveBackIf(ctx, q, r.UnsettledFavor, r.CampaignID, r.CharacterID, r.FavorPromptID); err != nil {
			return err
		}
	}
	return nil
}

// deleteCombatant is the one function that deletes a combatant's row (archtest keeps the generated DeleteCombatant here). The foreign key
// would delete the combatant's held rolls and the Inspiration they spent with them, so they are forgotten first, with their refund.
func deleteCombatant(ctx context.Context, q *playdb.Queries, who playdb.Combatant) error {
	if err := forgetHoldsOf(ctx, q, holdOwnerOfCombatant(who)); err != nil {
		return fmt.Errorf("forget the held rolls: %w", err)
	}
	if err := q.DeleteCombatant(ctx, who.ID); err != nil {
		return fmt.Errorf("delete the combatant: %w", err)
	}
	return nil
}

// forgetHold drops one held roll (the gate's: a question gone stale), giving back the Inspiration it spent when it was not answered.
func forgetHold(ctx context.Context, q *playdb.Queries, id string) error {
	rows, err := q.DeleteInspirationHold(ctx, id)
	if err != nil {
		return fmt.Errorf("delete the held roll: %w", err)
	}
	for _, r := range rows {
		if err := favorGiveBackIf(ctx, q, r.UnsettledFavor, r.CampaignID, r.CharacterID, r.FavorPromptID); err != nil {
			return err
		}
	}
	return nil
}

// forgetHoldsOfWindow drops the questions kept for one window that closed by itself, giving back the Inspiration the unanswered ones
// spent. A hold has no column that names its window: the request it stores does (window_id), and a reactor can have two windows of one
// kind open, so only the holds whose request names this window go. A kind that keeps no question forgets nothing.
func forgetHoldsOfWindow(ctx context.Context, q *playdb.Queries, encounterID string, w playdb.ReactionWindow) error {
	kind, ok := heldKindOfWindow(reaction.Kind(w.Kind))
	if !ok || w.ReactorID == nil {
		return nil
	}
	holds, err := q.ListOpenInspirationHolds(ctx, playdb.ListOpenInspirationHoldsParams{EncounterID: &encounterID, CombatantID: w.ReactorID})
	if err != nil {
		return fmt.Errorf("list the held rolls of a combatant: %w", err)
	}
	for _, h := range holds {
		if h.Kind == string(kind) && windowOfHold(kind, h.Request) == w.ID {
			if err := forgetHold(ctx, q, h.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

// windowOfHold is the window the request of a held save answers; empty when the request says none.
func windowOfHold(kind heldKind, request []byte) string {
	switch kind {
	case heldConcentrationSave:
		var req playv1.ResolveConcentrationSaveRequest
		if proto.Unmarshal(request, &req) == nil {
			return req.GetWindowId()
		}
	case heldEffectSave:
		var req playv1.RollEffectSaveRequest
		if proto.Unmarshal(request, &req) == nil {
			return req.GetWindowId()
		}
	}
	return ""
}

// forgetHoldsOfKind drops the questions of one kind a combatant keeps open in a combat (the one a window kept for its roller, when the
// window closes by itself), giving back the Inspiration the unanswered ones spent.
func forgetHoldsOfKind(ctx context.Context, q *playdb.Queries, encounterID string, combatantID *string, kind string) error {
	if combatantID == nil {
		return nil
	}
	holds, err := q.ListOpenInspirationHolds(ctx, playdb.ListOpenInspirationHoldsParams{EncounterID: &encounterID, CombatantID: combatantID})
	if err != nil {
		return fmt.Errorf("list the held rolls of a combatant: %w", err)
	}
	for _, h := range holds {
		if h.Kind != kind {
			continue
		}
		if err := forgetHold(ctx, q, h.ID); err != nil {
			return err
		}
	}
	return nil
}

// forgetStaleHoldsOf drops the held rolls of a roller that went stale (heldRoll.stale), giving back what the unanswered ones spent. The
// roller is read where it is (its combatant and the round, or the die its character holds); a hold that still waits stays.
func (s *Service) forgetStaleHoldsOf(ctx context.Context, env gateEnv, r heldRoller) error {
	st, err := s.rollerStateOf(ctx, env, r)
	if err != nil {
		return err
	}
	stores := env.stores()
	open, err := stores.openOf(ctx, r.in(st))
	if err != nil {
		return err
	}
	for _, h := range open {
		if h.stale(st) {
			if err := stores.forget(ctx, h); err != nil {
				return fmt.Errorf("forget the old held roll: %w", err)
			}
		}
	}
	return nil
}

// forgetOrphanHolds drops the held rolls of the campaign that wait for a Bardic Inspiration die that ran out, giving back the
// Inspiration they spent.
func forgetOrphanHolds(ctx context.Context, q *playdb.Queries, campaignID string) error {
	rows, err := q.DeleteOrphanInspirationHolds(ctx, campaignID)
	if err != nil {
		return fmt.Errorf("forget the rolls that waited for a die that ran out: %w", err)
	}
	for _, r := range rows {
		if err := favorGiveBackIf(ctx, q, r.UnsettledFavor, r.CampaignID, r.CharacterID, r.FavorPromptID); err != nil {
			return err
		}
	}
	return nil
}
