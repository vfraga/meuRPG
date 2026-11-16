// Package consumable holds what the play module knows about using a consumable (a potion, a scroll) that needs no
// store, no transport and no Service: the healing dice that are rolled or typed, the resistance a potion
// gives and the text a consumable with no automatic effect returns. play reads the sheet and the character, and calls here.
package consumable

import (
	"encoding/json"
	"errors"
	"fmt"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dice"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// ErrFaces is a list of typed faces that is not the potion's dice: the wrong number, or a face off its die.
var ErrFaces = errors.New("typed_faces must hold one face for each die of the potion, each a face of the die")

// Total is the potion's healing for the faces of its dice: their sum and the potion's fixed bonus, never below 0. It
// refuses a list that is not one face for each die, each from 1 to the die's sides (faces typed from real dice).
func Total(f rules.DiceFormula, faces []int32) (int32, error) {
	if len(faces) != f.Count {
		return 0, fmt.Errorf("%w (%d dice)", ErrFaces, f.Count)
	}
	sum := f.Bonus
	for _, face := range faces {
		if int(face) < 1 || int(face) > f.Sides {
			return 0, fmt.Errorf("%w (d%d)", ErrFaces, f.Sides)
		}
		sum += int(face)
	}
	return conv.Int32(max(min(sum, 1<<20), 0)), nil
}

// Resistance is the modifier a resistance potion puts on its drinker: resistance to the damage type.
func Resistance(p rules.PotionEffect) rules.EffectModifier {
	return rules.EffectModifier{Kind: rules.ModifierResistance, ResistanceKind: rules.ResistanceKindResist, DamageTypes: []string{p.DamageType}}
}

// Seconds is the time a resistance potion lasts, in the seconds game time counts (a round is six).
func Seconds(p rules.PotionEffect) int32 {
	return conv.Int32(p.Rounds * rules.SecondsPerRound)
}

// Text is what a consumable says it does: the Portuguese text when the content has it, else the SRD's English.
func Text(m rules.MagicItem) []string {
	if len(m.DescPT) > 0 {
		return m.DescPT
	}
	return m.Desc
}

// Roll is the faces of the potion's dice: rolled with r when inApp, else the faces typed from real dice, and the healing
// they make (Total). A typed list that is not the potion's dice is ErrFaces.
func Roll(r dice.Roller, f rules.DiceFormula, inApp bool, typed []int32) (faces []int32, total int32, err error) {
	faces = typed
	if inApp {
		rolled, err := dice.Roll(r, dice.Expr{Count: f.Count, Sides: f.Sides})
		if err != nil {
			return nil, 0, fmt.Errorf("roll the potion's dice: %w", err)
		}
		faces = nil
		for _, face := range rolled.Faces {
			faces = append(faces, conv.Int32(face))
		}
	}
	total, err = Total(f, faces)
	return faces, total, err
}

// ResistanceJSON is the modifiers of the record a potion's resistance is kept in outside a combat: the one resistance.
func ResistanceJSON(p rules.PotionEffect) ([]byte, error) {
	body, err := json.Marshal([]rules.EffectModifier{Resistance(p)})
	if err != nil {
		return nil, fmt.Errorf("encode the resistance: %w", err)
	}
	return body, nil
}

// FightState is how a combatant of a running combat stands for the table: down (at 0 hit points) or on its feet, the one word
// the order of the combat already gives every player.
func FightState(down bool) playv1.ConsumableTargetState {
	if down {
		return playv1.ConsumableTargetState_CONSUMABLE_TARGET_STATE_DOWN
	}
	return playv1.ConsumableTargetState_CONSUMABLE_TARGET_STATE_OK
}

// Target is a possible receiver of a gift as the list says it: the character, how far it stands and how it stands (unspecified
// when the table does not read it). A listed character may receive.
func Target(id, name string, distanceFt int32, state playv1.ConsumableTargetState) *playv1.ConsumableTarget {
	return &playv1.ConsumableTarget{CharacterId: id, Name: name, DistanceFt: distanceFt, State: state, MayReceive: true}
}

// Standing says whether a player's combatant is on a list of receivers and whether it is down, as the viewer reads it: the word the
// order of the combat gives every player. A confirmed death (gone: the combatant defeated, or its character marked dead) is on no
// list. A creature with three failed death saves that the master has not confirmed stays on a player's list, down only at 0 hit
// points as any other creature (the order shows it the same way), and is left out of the master's list; the use refuses it either
// way (TARGET_DEAD). v are the character's vitals.
func Standing(gone bool, deathFailures int32, master bool, v *playv1.CharacterVitals) (listed, down bool) {
	switch {
	case gone, master && deathFailures >= combat.DeathSaveGoal:
		return false, false
	}
	return true, v.GetHitPointsMax() > 0 && v.GetHitPointsCurrent() == 0
}
