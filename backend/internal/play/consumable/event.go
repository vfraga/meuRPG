package consumable

import (
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// What a use left behind, as the payload says it.
const (
	OutcomeHealed     = "healed"
	OutcomeResistance = "resistance"
	OutcomeTextOnly   = "text_only"
)

// Used is the payload of consumable_used. The keys round, combatant_id, target_id and secret are the ones the
// combat log reads from every event; the rest only the line of the use reads. Outside a combat the event has no combat
// and no line; in one the line names the item to the table, and its numbers are the master's, the user's owner's and the
// receiver's owner's alone. Secret is never set.
type Used struct {
	Round  int32  `json:"round"`
	Actor  string `json:"combatant_id,omitempty"`
	Secret bool   `json:"secret"`
	// Entry, Item and Name are the entry's id on the sheet, the catalog key and the name the line shows.
	Entry string `json:"use_entry,omitempty"`
	Item  string `json:"use_item"`
	Name  string `json:"use_name_pt,omitempty"`
	// Outcome is one of the Outcome* values.
	Outcome string `json:"use_outcome"`
	// Target is the character that received the effect and TargetCombatant its combatant in a running combat; both are
	// empty when the user received it.
	Target          string `json:"use_target,omitempty"`
	TargetCombatant string `json:"target_id,omitempty"`
	// Dice, Faces and Total are the healing dice as the sheet writes them, the faces rolled or typed and their total
	// with the potion's bonus; Healed the hit points regained, cut to the maximum. Physical says the faces were typed.
	Dice     string  `json:"use_dice,omitempty"`
	Faces    []int32 `json:"use_faces,omitempty"`
	Total    int32   `json:"use_total,omitempty"`
	Healed   int32   `json:"use_healed,omitempty"`
	Physical bool    `json:"use_physical,omitempty"`
	// DamageType is the key of the damage a resistance potion gave resistance to.
	DamageType string `json:"use_damage_type,omitempty"`
}

// Response is what the use answers, from the event the call wrote or found: the receiver (the user, when the event names
// none), the text of a use with no effect and the receiver's vitals.
func Response(ev Used, receiver string, text []string, vitals []*playv1.CharacterVitals) *playv1.UseConsumableResponse {
	out := &playv1.UseConsumableResponse{
		EntryId: ev.Entry, ItemKey: ev.Item, ItemNamePt: ev.Name, Text: text, DiceText: ev.Dice, Faces: ev.Faces, RollTotal: ev.Total,
		Healed: ev.Healed, DamageTypeKey: ev.DamageType, TargetCharacterId: receiver, Vitals: vitals,
	}
	out.Outcome = OutcomeOf(ev.Outcome)
	return out
}

// OutcomeOf is the outcome a payload says, as the API has it.
func OutcomeOf(outcome string) playv1.ConsumableUseOutcome {
	switch outcome {
	case OutcomeHealed:
		return playv1.ConsumableUseOutcome_CONSUMABLE_USE_OUTCOME_HEALED
	case OutcomeResistance:
		return playv1.ConsumableUseOutcome_CONSUMABLE_USE_OUTCOME_RESISTANCE
	}
	return playv1.ConsumableUseOutcome_CONSUMABLE_USE_OUTCOME_TEXT_ONLY
}

// Line is the line of a use in the combat's log: the item, the outcome, the damage type (with its name) and the receiver's label are
// the table's; the dice, the faces, the total and the hit points regained only when numbers is true (the master and the owners of
// the user and of the receiver, RN-10).
func Line(use Used, damageTypeName, targetLabel string, numbers bool) *playv1.ConsumableUseLogLine {
	line := &playv1.ConsumableUseLogLine{
		ItemNamePt: use.Name, Outcome: OutcomeOf(use.Outcome), DamageTypeKey: use.DamageType, DamageTypeNamePt: damageTypeName, TargetLabel: targetLabel,
	}
	if numbers {
		line.DiceText, line.Faces, line.RollTotal, line.Healed = use.Dice, use.Faces, use.Total, use.Healed
	}
	return line
}
