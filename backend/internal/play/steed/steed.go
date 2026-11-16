// Package steed holds the rules of Find Steed (SRD 5.1) that read plain values: which spells reach the steed with the caster, what the
// cast says about it, and what its Intelligence floor does to a spell that asks a score.
package steed

import (
	"slices"

	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

const (
	// TargetNote is the line of a cast that put its lasting effect on the steed too.
	TargetNote = "O corcel também é alvo da magia."
	// TableNote is the line of a cast whose effect the master applies to the steed.
	TableNote = "O corcel também é alvo da magia: o mestre aplica o efeito a ele."
	// tooSimpleScore is the Intelligence score at or under which Hideous Laughter does nothing (SRD 5.1).
	tooSimpleScore = 4
)

// SelfTargeted says the spell reaches only the caster (SRD: "a spell you cast that targets only you").
func SelfTargeted(content *rules.Content, spellKey string) bool {
	det, ok := content.SpellDetails(spellKey)
	return ok && det.Target.Kind == rules.TargetSelf
}

// NoteFor is the note of a cast with also_steed: the lasting effect reached the steed, or the table applies it.
func NoteFor(reachesSteed bool) string {
	if reachesSteed {
		return TargetNote
	}
	return TableNote
}

// TooSimple says an Intelligence score of 4 or less, which Hideous Laughter does not take hold of. A creature's own score (a steed has at
// least 6) wins when it is known (non-zero); the stat block's base score is read otherwise, and a creature with none is not too simple.
func TooSimple(score int, abilities []rules.AbilityScore) bool {
	if score == 0 {
		if i := slices.IndexFunc(abilities, func(a rules.AbilityScore) bool { return a.Ability == rules.INT }); i >= 0 {
			score = abilities[i].Base
		}
	}
	return score != 0 && score <= tooSimpleScore
}
