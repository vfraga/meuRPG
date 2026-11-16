package play

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/live"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// The master's undo of the Inspiration (RN-43): a grant, an accepted pass, and the answer to a pass that kept it with the giver. Only
// the ones written in a combat are the combat's actions (favorInCombat): outside one nothing is undone, and the master takes the
// Inspiration back with a grant and a pass.

// favorUndoKinds are the favor events an undo takes back.
var favorUndoKinds = []string{eventFavorGranted, eventFavorPassed, eventFavorAskRefused, eventFavorPassCanceled}

// favorUndoPayload is what the undo reads of a favor event: the character granted, or the pass's giver and receiver.
type favorUndoPayload struct {
	CharacterID     string `json:"character_id"`
	FromCharacterID string `json:"from_character_id"`
	ToCharacterID   string `json:"to_character_id"`
}

// errUndoSpent refuses the undo of an Inspiration the character spent since: the target changed after the action (the reason the
// other undos use when something moved what they would put back), naming the combatant.
func errUndoSpent(combatantID string) error {
	return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_UNDO_TARGET_CHANGED,
		"the inspiration was spent after the action: nothing was undone",
		func(b *playv1.EncounterBlocked) { b.CombatantIds = []string{combatantID} })
}

// takeBackFavor puts the Inspiration back as the event found it, in the undo's transaction: one step for the whole pass (the receiver
// loses it and the giver has it again, or nothing changes). It returns the vitals of the characters it changed, for the table.
func (s *Service) takeBackFavor(ctx context.Context, c *combatTx, kind string, payload []byte, ev actionEvent) ([]*playv1.CharacterVitals, error) {
	var p favorUndoPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("read the favor event: %w", err)
	}
	var ids []string
	var err error
	switch kind {
	case eventFavorGranted:
		ids, err = s.takeBackGrant(ctx, c, p, ev)
	case eventFavorPassed:
		ids, err = s.takeBackPass(ctx, c, p, ev)
	default: // an answer that moved nothing: only its line goes
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s.favorVitals(ctx, c, ids), nil
}

// takeBackGrant takes the Inspiration the master granted from the character, and the ask it may have made to pass it on.
func (s *Service) takeBackGrant(ctx context.Context, c *combatTx, p favorUndoPayload, ev actionEvent) ([]string, error) {
	if err := s.takeFavorFrom(ctx, c, p.CharacterID, ev.Actor); err != nil {
		return nil, err
	}
	return []string{p.CharacterID}, nil
}

// takeBackPass takes the Inspiration from the receiver (and the ask it may have made to pass it on) and gives it to the giver again.
func (s *Service) takeBackPass(ctx context.Context, c *combatTx, p favorUndoPayload, ev actionEvent) ([]string, error) {
	// The giver gets it back only if it may hold one now, as the accept asked of it: a character sent to the reserve since is refused.
	if err := s.mayHold(ctx, c.tx, c.session.CampaignID, p.FromCharacterID, errCannotReceive()); err != nil {
		return nil, err
	}
	if err := s.takeFavorFrom(ctx, c, p.ToCharacterID, ev.Target); err != nil {
		return nil, err
	}
	if _, err := c.q.InsertCharacterFavor(ctx, playdb.InsertCharacterFavorParams{CharacterID: p.FromCharacterID, CampaignID: c.session.CampaignID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) { // the giver has another by now: the transaction rolls the receiver's back
			return nil, favorBlocked(playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_ALREADY_HAS, "the giver has the inspiration again")
		}
		return nil, fmt.Errorf("give the giver the inspiration back: %w", err)
	}
	return []string{p.FromCharacterID, p.ToCharacterID}, nil
}

// takeFavorFrom removes the character's row (the combatant says who the undo names when it was spent since) and closes the ask the character
// had pending to pass it on: that Inspiration is gone, so the ask would wait for the master forever. The close is the giver's cancel
// without a key: its favor_pass_canceled has no combat, so it is no line and no action to undo, and it tells nothing of its own (the
// vitals hint the undo sends already reaches the character's owner, who reads GetFavor again and finds the ask closed).
func (s *Service) takeFavorFrom(ctx context.Context, c *combatTx, characterID, combatantID string) error {
	if _, err := c.q.SpendCharacterFavor(ctx, playdb.SpendCharacterFavorParams{CharacterID: characterID, CampaignID: c.session.CampaignID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errUndoSpent(combatantID)
		}
		return fmt.Errorf("take the inspiration back: %w", err)
	}
	pass, err := c.q.GetPendingFavorPassOfGiver(ctx, characterID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read the character's pending ask: %w", err)
	}
	closed, err := c.q.CloseFavorPass(ctx, playdb.CloseFavorPassParams{Status: passCanceled, ID: pass.ID, CampaignID: pass.CampaignID})
	if err != nil {
		return fmt.Errorf("cancel the character's pending ask: %w", err)
	}
	return s.writeCanceledPassEvent(ctx, c.q, c.session.ID, nil, closed)
}

// favorVitals reads the vitals of the characters the undo changed; one that cannot be read tells nothing.
func (s *Service) favorVitals(ctx context.Context, c *combatTx, ids []string) []*playv1.CharacterVitals {
	var out []*playv1.CharacterVitals
	for _, id := range ids {
		if v, err := s.vitals.GetVitalsTx(ctx, c.tx, c.session.CampaignID, id); err == nil {
			out = append(out, v)
		}
	}
	return out
}

// publishFavorUndo tells what the undo of a favor event changed: the master, and the owners the event was told to, that the combat and its
// log changed. No other player hears of it.
func (s *Service) publishFavorUndo(ctx context.Context, campaignID string, enc playdb.Encounter, ev actionEvent) {
	s.publishEncounterChangedToMaster(ctx, campaignID, enc)
	s.send(campaignID, live.Event{Audience: live.Audience{Master: true}, Message: combatLogChangedMessage(enc.ID, true)})
	if ev.Secret {
		return
	}
	for _, u := range slices.Compact(slices.Sorted(slices.Values(ev.SeenBy))) {
		s.send(campaignID, live.Event{Audience: live.Audience{UserID: u}, Message: combatLogChangedMessage(enc.ID, false)})
	}
}
