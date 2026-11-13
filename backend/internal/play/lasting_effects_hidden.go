package play

import (
	"slices"

	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// What the master hides from the players reaches them through no channel (RN-10), the sources
// of a roll included: a condition or a bonus that comes from an effect the master turned
// invisible to the players reads "Outra fonte".

// hiddenConditionsOf are the conditions a combatant carries only because of effects the master
// hides from the players.
func hiddenConditionsOf(states map[string][]playdb.CombatantState, c playdb.Combatant) []string {
	var hidden, shown []string
	for _, st := range effectsOn(states, c.ID) {
		if st.PlayerVisible {
			shown = append(shown, st.ConditionKeys...)
		} else {
			hidden = append(hidden, st.ConditionKeys...)
		}
	}
	return slices.DeleteFunc(hidden, func(k string) bool { return slices.Contains(shown, k) || slices.Contains(manualConditions(c), k) })
}

// manualConditions are the conditions the master set by hand.
func manualConditions(c playdb.Combatant) []string {
	return slices.DeleteFunc(slices.Clone(c.Conditions), func(k string) bool { return slices.Contains(c.EffectConditions, k) })
}

// hasHiddenEffect says a combatant carries an effect of the key that the master hides.
func hasHiddenEffect(states map[string][]playdb.CombatantState, id string) bool {
	return slices.ContainsFunc(effectsOn(states, id), func(st playdb.CombatantState) bool { return !st.PlayerVisible })
}

// hasUnreadEffect says a combatant carries an effect the viewer may not read: one the master hides, or one only
// the creature's player reads when the viewer is not that player.
func hasUnreadEffect(states map[string][]playdb.CombatantState, c playdb.Combatant, v combatViewer) bool {
	return slices.ContainsFunc(effectsOn(states, c.ID), func(st playdb.CombatantState) bool {
		return !st.PlayerVisible || (st.Audience == audienceOwner && !v.owns(c))
	})
}

// hiddenFrom says which sources of a roll between two combatants a viewer may not read the
// origin of. The master reads everything.
func (f modeFacts) hiddenFrom(v combatViewer, a, b playdb.Combatant) func(combat.Source) bool {
	if v.master {
		return nil
	}
	// Even the player whose character it is on reads "Outra fonte" on the roll: what the effect is
	// stays the master's (the card of their own character is another matter).
	var conds []string
	anyHidden := false
	for _, c := range []playdb.Combatant{a, b} {
		conds = append(conds, hiddenConditionsOf(f.states, c)...)
		anyHidden = anyHidden || hasUnreadEffect(f.states, c, v)
	}
	return func(src combat.Source) bool {
		if isSunlight(src) {
			return true // the sun and its trait are the master's (RN-20, RN-29)
		}
		switch src.Kind {
		case combat.SourceOutlinedTarget, combat.SourceEffectSave, combat.SourceEffectCheck, combat.SourceEffectAttack:
			return anyHidden
		}
		return src.Condition != "" && slices.Contains(conds, src.Condition)
	}
}
