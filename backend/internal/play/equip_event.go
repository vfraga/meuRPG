package play

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// The swap of a piece of gear in a running combat is an event of the combat (gear_swapped): it keeps the turn's
// economy and the piece's equipped state from before, so the master's undo takes it back first (combat_undo_gear.go),
// and the log shows it to the master and the character's owner alone (RN-10).

// What a swap spent, as the event and the log line say it.
const (
	gearCostFree   = "free"         // the turn's free object interaction
	gearCostAction = "action"       // the action
	gearCostExtra  = "extra_action" // the extra action an effect lends for using an object (Haste)
)

// gearSwapEvent is the payload of a gear_swapped event, its own writer type: the keys round, combatant_id (the actor) and secret are the
// ones every combat event has (readEvent reads them from it); the others only the undo and the log read, with readGearSwap.
type gearSwapEvent struct {
	Round int32  `json:"round"`
	Actor string `json:"combatant_id"`
	// Secret: the line is the master's and the owner's, never another player's.
	Secret bool `json:"secret"`
	// Key and Kind are the piece ("weapon" or "shield"), Name its Portuguese name when the swap was made.
	Key  string `json:"key"`
	Kind string `json:"kind"`
	Name string `json:"name_pt,omitempty"`
	// EquippedBefore is whether the piece was equipped before the swap; the swap made it the other way.
	EquippedBefore bool `json:"equipped_before"`
	// The turn's economy before the swap, which the undo puts back.
	ActionBefore            bool `json:"action_before"`
	BonusBefore             bool `json:"bonus_before"`
	ReactionBefore          bool `json:"reaction_before"`
	DashedBefore            bool `json:"dashed_before"`
	ObjectInteractionBefore bool `json:"object_interaction_before"`
	ExtraActionBefore       bool `json:"extra_action_before"`
	// Cost is what the swap spent (gearCost*); empty when the master made it.
	Cost string `json:"cost,omitempty"`
}

// gearSwapOf is the event of the change, with the combatant's economy as it stands before the swap spends anything.
func gearSwapOf(who playdb.Combatant, change link.EquipChange, round int32) gearSwapEvent {
	return gearSwapEvent{
		Round: round, Actor: who.ID, Secret: true,
		Key: change.Key, Kind: string(change.Kind), EquippedBefore: !change.Equip,
		ActionBefore: who.ActionUsed, BonusBefore: who.BonusActionUsed, ReactionBefore: who.ReactionUsed, DashedBefore: who.Dashed,
		ObjectInteractionBefore: who.ObjectInteractionUsed, ExtraActionBefore: who.ExtraActionUsed,
	}
}

// writeGearSwap writes the swap in the transaction of the sheet's write, with the piece's name.
func (s *Service) writeGearSwap(ctx context.Context, c *combatTx, actorUserID, characterID string, ev gearSwapEvent) error {
	names, err := s.roster.ContentNames(ctx, c.tx, c.session.CampaignID)
	if err != nil {
		return err
	}
	if ev.Name = names(ev.Key); ev.Name == "" {
		ev.Name = ev.Key[strings.LastIndex(ev.Key, ":")+1:]
	}
	c.characterID = &characterID
	return insertEvent(ctx, c, eventGearSwapped, &actorUserID, nil, ev)
}

// readGearSwap reads a gear_swapped payload.
func readGearSwap(payload []byte) (gearSwapEvent, error) {
	var ev gearSwapEvent
	if err := json.Unmarshal(payload, &ev); err != nil {
		return gearSwapEvent{}, fmt.Errorf("read a gear swap payload: %w", err)
	}
	return ev, nil
}
