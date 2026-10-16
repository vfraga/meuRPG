package characters

import (
	"fmt"
	"slices"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The master's correction of the vitals (AdjustVitals), one value of the request at a
// time: each function checks the value req sets against its maximum and puts it on v,
// or does nothing when req leaves it alone. Errors are fieldErrors with the request's
// field names.

// vitalsChange checks and applies one value of a correction.
type vitalsChange func(v *playv1.CharacterVitals, req *playv1.AdjustCharacterVitalsRequest) error

// vitalsChanges are the values of a correction in the order they are checked: the web
// shows the first refusal under its field, so the order is part of the answer.
var vitalsChanges = []vitalsChange{
	changeWildShapeHitPoints,
	changeHitPoints,
	changeTemporaryHitPoints,
	changeSpellSlotsUsed,
	changePactSlotsUsed,
	changeHitDiceUsed,
	changeHitDiceUsedByDie,
	changeResourcesUsed,
}

// setsAnyVital says whether req sets at least one value to change.
func setsAnyVital(req *playv1.AdjustCharacterVitalsRequest) bool {
	return req.HitPointsCurrent != nil || req.HitPointsTemporary != nil || len(req.GetSpellSlotsUsed()) > 0 ||
		req.PactSlotsUsed != nil || req.HitDiceUsed != nil || len(req.GetHitDiceUsedByDie()) > 0 ||
		len(req.GetResourcesUsed()) > 0 || req.WildShapeHitPointsCurrent != nil
}

// changeWildShapeHitPoints sets the beast's hit points; at 0 the beast is gone and the
// form ends.
func changeWildShapeHitPoints(v *playv1.CharacterVitals, req *playv1.AdjustCharacterVitalsRequest) error {
	if req.WildShapeHitPointsCurrent == nil {
		return nil
	}
	w := v.GetWildShape()
	if w == nil {
		return fieldErr("wild_shape_hit_points_current", "must be unset: the character is not in a beast form")
	}
	if err := inRange("wild_shape_hit_points_current", req.GetWildShapeHitPointsCurrent(), w.GetHitPointsMax()); err != nil {
		return err
	}
	if req.GetWildShapeHitPointsCurrent() == 0 {
		v.WildShape = nil
	} else {
		w.HitPointsCurrent = req.GetWildShapeHitPointsCurrent()
	}
	return nil
}

// changeHitPoints sets the current hit points, up to the maximum.
func changeHitPoints(v *playv1.CharacterVitals, req *playv1.AdjustCharacterVitalsRequest) error {
	if req.HitPointsCurrent == nil {
		return nil
	}
	if err := inRange("hit_points_current", req.GetHitPointsCurrent(), v.GetHitPointsMax()); err != nil {
		return err
	}
	v.HitPointsCurrent = req.GetHitPointsCurrent()
	return nil
}

// changeTemporaryHitPoints sets the temporary hit points, up to MaxTemporaryHitPoints.
func changeTemporaryHitPoints(v *playv1.CharacterVitals, req *playv1.AdjustCharacterVitalsRequest) error {
	if req.HitPointsTemporary == nil {
		return nil
	}
	if err := inRange("hit_points_temporary", req.GetHitPointsTemporary(), MaxTemporaryHitPoints); err != nil {
		return err
	}
	v.HitPointsTemporary = req.GetHitPointsTemporary()
	return nil
}

// changeSpellSlotsUsed sets the slots used of each level req names, once each, of a
// level the character has slots of.
func changeSpellSlotsUsed(v *playv1.CharacterVitals, req *playv1.AdjustCharacterVitalsRequest) error {
	seen := map[int32]bool{}
	for i, change := range req.GetSpellSlotsUsed() {
		field := fmt.Sprintf("spell_slots_used[%d]", i)
		level := change.GetLevel()
		if seen[level] {
			return fieldErr(field+".level", "repeats level %d", level)
		}
		seen[level] = true
		slot := slotOfLevel(v, level)
		if slot == nil {
			return fieldErr(field+".level", "must be a spell level the character has slots of")
		}
		if err := inRange(field+".used", change.GetUsed(), slot.GetTotal()); err != nil {
			return err
		}
		slot.Used = change.GetUsed()
	}
	return nil
}

// changePactSlotsUsed sets the pact magic slots used, for a character that has them.
func changePactSlotsUsed(v *playv1.CharacterVitals, req *playv1.AdjustCharacterVitalsRequest) error {
	if req.PactSlotsUsed == nil {
		return nil
	}
	if v.GetPactSlots() == nil {
		return fieldErr("pact_slots_used", "must be unset: the character has no pact magic slots")
	}
	if err := inRange("pact_slots_used", req.GetPactSlotsUsed(), v.GetPactSlots().GetTotal()); err != nil {
		return err
	}
	v.PactSlots.Used = req.GetPactSlotsUsed()
	return nil
}

// changeHitDiceUsed sets the hit dice spent as a count, never together with the
// sizes. A count with no sizes spends the largest dice first.
func changeHitDiceUsed(v *playv1.CharacterVitals, req *playv1.AdjustCharacterVitalsRequest) error {
	if req.HitDiceUsed == nil {
		return nil
	}
	if len(req.GetHitDiceUsedByDie()) > 0 {
		return fieldErr("hit_dice_used", "must not be set together with hit_dice_used_by_die")
	}
	if err := inRange("hit_dice_used", req.GetHitDiceUsed(), v.GetHitDiceTotal()); err != nil {
		return err
	}
	setHitDiceUsed(v, rules.SplitHitDiceUsed(hitDiceOf(v), int(req.GetHitDiceUsed())))
	return nil
}

// changeHitDiceUsedByDie sets the hit dice spent of each size req names, of a size the
// character has; 0 spends none of it. The sizes are a map, so which bad size is named
// first, when there are two, is Go's map order.
func changeHitDiceUsedByDie(v *playv1.CharacterVitals, req *playv1.AdjustCharacterVitalsRequest) error {
	if len(req.GetHitDiceUsedByDie()) == 0 {
		return nil
	}
	next := hitDiceFromProto(v.GetHitDiceUsedByDie())
	for die, n := range req.GetHitDiceUsedByDie() {
		field := fmt.Sprintf("hit_dice_used_by_die[%d]", die)
		i := slices.IndexFunc(v.GetHitDice(), func(hd *rulesv1.HitDice) bool { return hd.GetFaces() == die })
		if i < 0 {
			return fieldErr(field, "must be a die size the character has")
		}
		if err := inRange(field, n, v.GetHitDice()[i].GetCount()); err != nil {
			return err
		}
		if n == 0 {
			delete(next, int(die))
		} else {
			next[int(die)] = int(n)
		}
	}
	setHitDiceUsed(v, next)
	return nil
}

// changeResourcesUsed sets the uses of each resource req names, once each, of a
// resource the character has.
func changeResourcesUsed(v *playv1.CharacterVitals, req *playv1.AdjustCharacterVitalsRequest) error {
	seen := map[string]bool{}
	for i, change := range req.GetResourcesUsed() {
		field := fmt.Sprintf("resources_used[%d]", i)
		if seen[change.GetKey()] {
			return fieldErr(field+".key", "repeats a resource")
		}
		seen[change.GetKey()] = true
		at := slices.IndexFunc(v.GetResources(), func(r *playv1.ResourceUsage) bool { return r.GetKey() == change.GetKey() })
		if at < 0 {
			return fieldErr(field+".key", "must be a resource the character has")
		}
		if err := inRange(field+".used", change.GetUsed(), v.GetResources()[at].GetTotal()); err != nil {
			return err
		}
		v.Resources[at].Used = change.GetUsed()
	}
	return nil
}
