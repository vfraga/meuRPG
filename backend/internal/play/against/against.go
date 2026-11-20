// Package against holds what a saving throw is against (a kind of source: magic, poison, charm, fear) that needs no store, no
// transport and no Service: play asks it for the kinds of a spell, of an effect that lasts and of a trap.
package against

import (
	"slices"
	"strings"

	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// The racial advantages on saving throws that hold against one kind of source (Brave, Fey Ancestry, Gnome Cunning,
// Dwarven Resilience, SRD 5.1) apply when the save says what it is against (combat.SaveScene.Against). The app knows
// the source of a spell's save, of the save of an effect that lasts and of a trap's save; a save asked with no source
// (a concentration save, a save the master asks) keeps the sheet's reminder, and the master decides.

// conditionAgainst is the kind of source a condition a save is against stands for.
var conditionAgainst = map[string]string{
	"condition:frightened": combat.AgainstFrightened,
	"condition:charmed":    combat.AgainstCharmed,
	"condition:poisoned":   combat.AgainstPoison,
}

// spellImposes are the spells whose saving throw is against being charmed or frightened, which the spell data does
// not say as a condition. A spell is always against magic.
var spellImposes = map[string][]string{
	"spell:charm-person":      {combat.AgainstCharmed},
	"spell:animal-friendship": {combat.AgainstCharmed},
	"spell:dominate-beast":    {combat.AgainstCharmed},
	"spell:dominate-person":   {combat.AgainstCharmed},
	"spell:dominate-monster":  {combat.AgainstCharmed},
	"spell:geas":              {combat.AgainstCharmed},
	"spell:hypnotic-pattern":  {combat.AgainstCharmed},
	"spell:fear":              {combat.AgainstFrightened},
	"spell:phantasmal-killer": {combat.AgainstFrightened},
	"spell:weird":             {combat.AgainstFrightened},
}

const poisonDamage = "damage-type:poison"

// withAgainst adds a kind to the list once.
func withAgainst(list []string, kinds ...string) []string {
	for _, k := range kinds {
		if !slices.Contains(list, k) {
			list = append(list, k)
		}
	}
	return list
}

// conditionsAgainst are the kinds of source the conditions a save can give stand for.
func conditionsAgainst(keys []string) []string {
	var out []string
	for _, k := range keys {
		if kind, ok := conditionAgainst[k]; ok {
			out = withAgainst(out, kind)
		}
	}
	return out
}

// withConditions adds to what a save is against the conditions it can give, by their keys
// ("condition:frightened"): a table's roll mode against a condition (any of the 15) holds on these.
func WithSaveConditions(against, conditions []string) []string {
	return withAgainst(slices.Clone(against), conditions...)
}

// EffectSave is what the saving throw of an effect that lasts is against: Effect and the
// conditions the effect gives.
func EffectSave(key string, def *rules.EffectDef) []string {
	against := Effect(key, def)
	if def == nil {
		return against
	}
	return WithSaveConditions(against, def.Conditions)
}

// TrapSave is what the saving throw of a trap is against: Trap and the conditions it gives.
func TrapSave(effect *rulesv1.TrapEffect) []string {
	var conditions []string
	for _, c := range effect.GetConditions() {
		conditions = append(conditions, c.GetConditionKey())
	}
	return WithSaveConditions(Trap(effect), conditions)
}

// Spell is what the saving throw against a spell is against: magic, poison when it deals poison damage, and
// charm or fear when it is one of the spells that charm or frighten.
func Spell(sp link.Spell) []string {
	var out []string
	if !sp.Nonmagical { // a creature's breath or claws is no magic
		out = withAgainst(out, combat.AgainstMagic)
	}
	out = withAgainst(out, spellImposes[sp.Key]...)
	if slices.ContainsFunc(sp.Damages, func(d link.Dice) bool { return d.DamageType == poisonDamage }) {
		out = withAgainst(out, combat.AgainstPoison)
	}
	return out
}

// Effect is what the saving throw an effect that lasts asks is against: magic for a spell's, and what the
// effect gives (a condition) or its spell imposes.
func Effect(key string, def *rules.EffectDef) []string {
	var out []string
	if strings.HasPrefix(key, "spell:") {
		out = withAgainst(out, combat.AgainstMagic)
		out = withAgainst(out, spellImposes[key]...)
	}
	if def != nil {
		out = withAgainst(out, conditionsAgainst(def.Conditions)...)
	}
	return out
}

// Trap is what the saving throw of a trap is against: poison when it deals poison damage or gives the
// poisoned condition, charm or fear when it gives them. A trap carries no mark of being magic, so Gnome Cunning
// stays a reminder there.
func Trap(effect *rulesv1.TrapEffect) []string {
	var out []string
	for _, d := range effect.GetDamage() {
		if d.GetDamageTypeKey() == poisonDamage {
			out = withAgainst(out, combat.AgainstPoison)
		}
	}
	for _, c := range effect.GetConditions() {
		out = withAgainst(out, conditionsAgainst([]string{c.GetConditionKey()})...)
	}
	return out
}
