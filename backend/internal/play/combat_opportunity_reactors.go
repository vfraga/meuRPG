package play

import (
	"context"
	"slices"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// opportunityReactors lists the combatants that could make an opportunity attack
// on the mover if it left their reach. exact is the server's own list (who is
// really offered the attack): hostile to the mover (the other side), placed, not
// defeated, awake (no incapacitating condition, not down), with the reaction
// unused, able to attack with it (a creature that may not, may not) and holding
// a melee attack, whose longest reach is the reach. They all see the mover unless
// it is hidden, and a Blinded one does not; on a map with the fog of war the
// callers ask the sight (fogSight.reactorSees) of the reactors a move leaves, after
// the reach filter, never of all of them. A reactor in the mover's own turn group
// may react too (the SRD allows reactions on any turn). Not exact is a player's
// warning in GetMoveOptions: only what the viewer sees (RN-20), so hostile, not
// defeated, visible, its visible conditions, and a 5 ft reach, never the reaction,
// the stat block or the vitals of an NPC. tx is the open transaction, or nil for a
// read.
func (s *Service) opportunityReactors(ctx context.Context, tx pgx.Tx, campaignID string, cs []playdb.Combatant, mover playdb.Combatant, v *combatViewer, draggedID string) ([]candidate, error) {
	if mover.Hidden || mover.Disengaged || !placed(mover) {
		return nil, nil
	}
	exact := v == nil || v.master
	var out []candidate
	for _, r := range cs {
		if !couldReactTo(r, mover, v, draggedID) {
			continue
		}
		cand, ok, err := s.reactorCandidate(ctx, tx, campaignID, r, exact)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, cand)
		}
	}
	return out, nil
}

// couldReactTo is the part of the filter that needs no read: r is a placed,
// living foe of the mover, not the creature the mover drags (SRD 5.1, "Moving a
// Grappled Creature": it moves with the mover, never leaves its reach and takes no
// opportunity attack), not the rider of the mover or the mount of the mover, seen by the viewer, and not incapacitated or blinded.
func couldReactTo(r, mover playdb.Combatant, v *combatViewer, draggedID string) bool {
	if r.ID == mover.ID || r.Defeated || !placed(r) || r.Side == mover.Side || r.ID == draggedID || samePair(r, mover) {
		return false
	}
	if v != nil && !v.sees(r) {
		return false
	}
	return !cannotReact(r)
}

// cannotReact says a combatant carries a condition that takes its reaction away.
func cannotReact(r playdb.Combatant) bool {
	return slices.ContainsFunc(r.Conditions, func(k string) bool { return slices.Contains(cantReact, k) })
}

// reactorCandidate turns a foe into a candidate. A player's warning (not exact)
// takes it at the default reach; the server's own list asks what it can really do.
func (s *Service) reactorCandidate(ctx context.Context, tx pgx.Tx, campaignID string, r playdb.Combatant, exact bool) (candidate, bool, error) {
	if !exact {
		return candidate{who: r, reachFt: meleeReachFt}, true, nil
	}
	if r.ReactionUsed || r.EffectNoReaction || mayAttack(r, true) != nil { // no reactions: the Open Hand technique
		return candidate{}, false, nil //nolint:nilerr // a refusal to attack means this foe is left out, not an error.
	}
	awake, err := s.awakeToReact(ctx, tx, campaignID, r)
	if err != nil || !awake {
		return candidate{}, false, err
	}
	sheet, err := s.sheetOf(ctx, tx, campaignID, r)
	if connect.CodeOf(err) == connect.CodeNotFound {
		return candidate{}, false, nil
	}
	if err != nil {
		return candidate{}, false, err
	}
	reach := meleeReachOf(sheet)
	if reach <= 0 {
		return candidate{}, false, nil
	}
	return candidate{who: r, reachFt: reach, sheet: sheet}, true, nil
}

// awakeToReact says r is neither down nor surprised (a surprised creature takes
// no reaction until its first turn ends, SRD 5.1, "Surprise").
func (s *Service) awakeToReact(ctx context.Context, tx pgx.Tx, campaignID string, r playdb.Combatant) (bool, error) {
	down, err := s.isDown(ctx, tx, campaignID, r)
	if err != nil || down {
		return false, err
	}
	surprised, err := s.surprisedReactor(ctx, tx, campaignID, r)
	if err != nil {
		return false, err
	}
	return !surprised, nil
}
