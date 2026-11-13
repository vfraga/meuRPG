package play

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// An aura is a lasting effect on its bearer that reaches the creatures within a radius of the bearer, on one side
// (self, allies or enemies), where they stand when the roll or the damage happens. The row is the bearer's like
// any other (the clock, the concentration, the undo, the card), and holds the mark of the aura (the modifier of
// kind aura) with the modifiers it gives. Nothing about who is inside is ever written: withAuras answers it from the
// map each time a roll or a damage reads the effects, so a creature that moves in or out changes the next roll and
// nothing else.

// auraMark is the aura of an effect row; false for a row that is not one.
func auraMark(st playdb.CombatantState) (rules.EffectModifier, bool) {
	if !isEffect(st) {
		return rules.EffectModifier{}, false
	}
	for _, m := range storedModifiers(st) {
		if m.Kind == rules.ModifierAura {
			return m, true
		}
	}
	return rules.EffectModifier{}, false
}

// auraReaches says the creature is one the aura of the bearer reaches: on the side it names, in the radius. The
// sides are the combat's (Side), as Pack Tactics reads them. Distance is the map's, from the bearer's square;
// without a square for either (no grid, or a creature not on the map) nobody is reached but the bearer, and the
// master judges (RN-25).
func auraReaches(a rules.EffectModifier, bearer, c playdb.Combatant) bool {
	if c.Dismissed {
		return false
	}
	switch a.Affects {
	case rules.AuraAffectsSelf:
		return c.ID == bearer.ID
	case rules.AuraAffectsAllies:
		if c.ID == bearer.ID || c.Side != bearer.Side {
			return false
		}
	case rules.AuraAffectsEnemies:
		if c.Side == bearer.Side {
			return false
		}
	default:
		return false
	}
	d, ok := distanceFt(bearer, c)
	return ok && int(d) <= a.RadiusFt
}

// withAuras is the states of the combat as a roll reads them: the aura rows give, to each creature they reach, a
// copy of their modifiers (the mark taken off) as an effect on that creature, once for each source (two auras of
// the same spell reach a creature once). A combat with no aura answers the map it was given. v is who reads the
// roll: a player does not read the name of an aura whose bearer they do not see, nor of one the master keeps from
// the players or that is owner-only, on a creature that is not the bearer, which read "Outra fonte" (RN-10).
func withAuras(states map[string][]playdb.CombatantState, cs []playdb.Combatant, v *combatViewer) map[string][]playdb.CombatantState {
	type aura struct {
		row    playdb.CombatantState
		mark   rules.EffectModifier
		bearer playdb.Combatant
	}
	var auras []aura
	for _, b := range cs {
		if b.Defeated || b.Dismissed {
			continue
		}
		for _, st := range states[b.ID] {
			if m, ok := auraMark(st); ok {
				auras = append(auras, aura{row: st, mark: m, bearer: b})
			}
		}
	}
	if len(auras) == 0 {
		return states
	}
	out := make(map[string][]playdb.CombatantState, len(states))
	for id, rows := range states {
		out[id] = slices.Clone(rows)
	}
	reached := map[[2]string]bool{} // the creature and the source an aura of which already reached it
	for _, a := range auras {
		given := givenByAura(a.row)
		if len(given) == 0 {
			continue
		}
		source := a.row.ID
		if a.row.SourceKey != nil {
			source = *a.row.SourceKey
		}
		for _, c := range cs {
			if !auraReaches(a.mark, a.bearer, c) || reached[[2]string{c.ID, source}] {
				continue
			}
			reached[[2]string{c.ID, source}] = true
			view := a.row
			view.CombatantID, view.Modifiers = c.ID, given
			if c.ID != a.bearer.ID && (a.row.Audience == audienceOwner || !seenBy(v, a.bearer)) {
				view.PlayerVisible = false
			}
			out[c.ID] = append(out[c.ID], view)
		}
	}
	return out
}

// seenBy says the reader sees the bearer. A roll that knows no reader (v nil) reads as a player who does not see a
// bearer the master hides.
func seenBy(v *combatViewer, bearer playdb.Combatant) bool {
	if v == nil {
		return !bearer.Hidden
	}
	return v.sees(bearer)
}

// readStatesAt is readStates as a roll reads it: with the auras of the combat (withAuras). A combat with no aura
// reads one query, as readStates does.
func (s *Service) readStatesAt(ctx context.Context, tx pgx.Tx, encID string, v *combatViewer) (map[string][]playdb.CombatantState, error) {
	return statesAt(ctx, s.queriesIn(tx), encID, v)
}

// statesAt is readStatesAt on the queries of a change.
func statesAt(ctx context.Context, q *playdb.Queries, encID string, v *combatViewer) (map[string][]playdb.CombatantState, error) {
	rows, err := q.ListCombatantStates(ctx, encID)
	if err != nil {
		return nil, fmt.Errorf("list the states: %w", err)
	}
	states := statesOf(rows)
	if !slices.ContainsFunc(rows, isMarked) {
		return states, nil
	}
	cs, err := q.ListCombatants(ctx, encID)
	if err != nil {
		return nil, fmt.Errorf("list the combatants: %w", err)
	}
	return withAuras(states, cs, v), nil
}

func isMarked(st playdb.CombatantState) bool { _, ok := auraMark(st); return ok }

// givenByAura is what the aura gives: the modifiers of its row but the mark.
func givenByAura(st playdb.CombatantState) []byte {
	mods := slices.DeleteFunc(storedModifiers(st), func(m rules.EffectModifier) bool { return m.Kind == rules.ModifierAura })
	if len(mods) == 0 {
		return nil
	}
	body, _ := json.Marshal(mods) // plain values: they cannot fail to encode
	return body
}
