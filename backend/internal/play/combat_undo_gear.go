package play

import (
	"context"
	"fmt"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/play/live"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// Taking a swap of gear back (the undo of a gear_swapped event, equip_event.go): the turn's economy of the combatant
// and the piece's equipped state are put back as the event kept them. The economy is the combat's, written here; the sheet
// is the characters module's, written through CombatRoster.RestoreEquipped in the undo's transaction, so an undo that the
// sheet refuses (its gear changed since) writes nothing. A swap whose combatant left the combat is never the last action:
// the removal is an event that closes the chain.

// takeBackGearSwap puts back what the swap spent and the piece it moved; the run keeps what is told after the commit.
func (s *Service) takeBackGearSwap(ctx context.Context, c *combatTx, run *undoRun, payload []byte) error {
	ev, err := readGearSwap(payload)
	if err != nil {
		return err
	}
	cs, err := listCombatants(ctx, c)
	if err != nil {
		return err
	}
	who, ok := combatantByID(cs, ev.Actor)
	if !ok || who.Dismissed {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOTHING_TO_UNDO, "the character left the combat: its swap cannot be undone")
	}
	u := &undoer{s: s, c: c, cs: cs}
	if err := u.setEconomy(ctx, who, ev.ActionBefore, ev.BonusBefore, ev.ReactionBefore, ev.DashedBefore); err != nil {
		return err
	}
	if err := c.q.SetCombatantObjectInteractionUsed(ctx, playdb.SetCombatantObjectInteractionUsedParams{ID: who.ID, ObjectInteractionUsed: ev.ObjectInteractionBefore}); err != nil {
		return fmt.Errorf("put back the object interaction: %w", err)
	}
	if err := c.q.SetCombatantExtraActionUsed(ctx, playdb.SetCombatantExtraActionUsedParams{ID: who.ID, ExtraActionUsed: ev.ExtraActionBefore}); err != nil {
		return fmt.Errorf("put back the extra action: %w", err)
	}
	run.gearOwner = conv.OrZero(who.UserID)
	run.afterGear, err = s.roster.RestoreEquipped(ctx, c.tx, c.session.CampaignID, who.CharacterID, ev.Key, ev.EquippedBefore)
	return err
}

// publishGearLog tells the master and the character's owner that the combat's log changed (the swap's line is theirs alone),
// and no other player.
func (s *Service) publishGearLog(ctx context.Context, campaignID, encounterID, ownerUserID string) {
	_ = ctx
	msg := &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_CombatLogChanged_{
		CombatLogChanged: &playv1.WatchGameSessionResponse_CombatLogChanged{EncounterId: encounterID},
	}}
	s.send(campaignID, live.Event{Audience: live.Audience{Master: true}, Message: msg})
	if ownerUserID != "" {
		s.send(campaignID, live.Event{Audience: live.Audience{UserID: ownerUserID}, Message: msg})
	}
}
