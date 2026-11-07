package play

import "github.com/PuraFome/meuRPG/backend/internal/play/playdb"

// A query that deletes or updates rows and returns them ends in a SELECT over the
// changed rows (to put them in order), so sqlc gives it a row type of its own with
// the columns of the table. These turn the rows back into the table's model.

// combatantStateRow is a row type of a query on combatant_states.
type combatantStateRow interface {
	playdb.DeleteStatesOfCombatantRow | playdb.DeleteExpiredCombatantStatesRow | playdb.DeleteEndedCombatantStatesRow
}

func combatantStates[R combatantStateRow](rows []R) []playdb.CombatantState {
	out := make([]playdb.CombatantState, len(rows))
	for i, r := range rows {
		out[i] = playdb.CombatantState(r)
	}
	return out
}

// characterEffectRow is a row type of a query on character_effects.
type characterEffectRow interface {
	playdb.DeleteCharacterEffectsOfGroupRow | playdb.DeleteLongRestEffectsOfCampaignRow | playdb.AdvanceCharacterEffectsRow
}

func characterEffects[R characterEffectRow](rows []R) []playdb.CharacterEffect {
	out := make([]playdb.CharacterEffect, len(rows))
	for i, r := range rows {
		out[i] = playdb.CharacterEffect(r)
	}
	return out
}

// spellZoneRow is a row type of a query on spell_zones.
type spellZoneRow interface {
	playdb.DeleteConcentrationZonesOfRow
}

func spellZones[R spellZoneRow](rows []R) []playdb.SpellZone {
	out := make([]playdb.SpellZone, len(rows))
	for i, r := range rows {
		out[i] = playdb.SpellZone(r)
	}
	return out
}
