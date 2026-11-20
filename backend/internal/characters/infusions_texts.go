package characters

import (
	"errors"
	"fmt"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The kinds of piece an infusion goes on, as the proto names them and as the engine does.
var infusionAcceptsToProto = func() map[string]rulesv1.InfusionAccepts {
	out := make(map[string]rulesv1.InfusionAccepts, len(infusionAcceptsOf))
	for p, s := range infusionAcceptsOf {
		out[s] = p
	}
	return out
}()

// wrongKindPT is what a piece of the wrong kind is told, by what the infusion asks.
var wrongKindPT = map[string]string{
	rules.InfusionAcceptsThrownWeapon:      "Pede uma arma de arremesso.",
	rules.InfusionAcceptsAmmunitionWeapon:  "Pede uma arma com munição.",
	rules.InfusionAcceptsArmor:             "Pede uma armadura.",
	rules.InfusionAcceptsShield:            "Pede um escudo.",
	rules.InfusionAcceptsArmorOrShield:     "Pede uma armadura ou um escudo.",
	rules.InfusionAcceptsWeapon:            "Pede uma arma.",
	rules.InfusionAcceptsSpellcastingFocus: "Pede um foco de conjuração.",
	rules.InfusionAcceptsAnyObject:         "Pede um objeto que a ficha não lista.",
}

// kindWordPT is the word of a piece's kind.
var kindWordPT = map[string]string{rules.GearArmor: "armadura", rules.GearShield: "escudo", rules.GearWeapon: "arma"}

// objectKindPT is the kind word of the row that stands for an object the player names.
const objectKindPT = "objeto"

// alreadyMagicalPT is the sentence of a piece that is a magic item already: bonus is its +N, 0 when it has none.
func alreadyMagicalPT(bonus int) string {
	if bonus > 0 {
		return fmt.Sprintf("Já é um item mágico (+%d): a infusão pede um item comum.", bonus)
	}
	return "Já é um item mágico: a infusão pede um item comum."
}

// takenPT is the sentence of a piece that already carries an infusion, named.
func takenPT(infusionName string) string {
	return fmt.Sprintf("Já tem %s: um item guarda uma infusão só.", infusionName)
}

// maxObjectNameRunes is the longest name of an object (character_infusions.object_name).
const maxObjectNameRunes = 40

// errInfusionBlocked is InfusionService's failed_precondition for a rule of its own, with the InfusionBlocked detail.
func errInfusionBlocked(reason charactersv1.InfusionBlockedReason) error {
	msg := map[charactersv1.InfusionBlockedReason]string{
		charactersv1.InfusionBlockedReason_INFUSION_BLOCKED_REASON_ITEM_ALREADY_INFUSED: "the item already carries an infusion",
		charactersv1.InfusionBlockedReason_INFUSION_BLOCKED_REASON_ITEM_NOT_ACCEPTED:    "the item is not one this infusion goes on",
		charactersv1.InfusionBlockedReason_INFUSION_BLOCKED_REASON_ITEM_CHANGED:         "the item or the infusion changed since you chose: choose again",
		charactersv1.InfusionBlockedReason_INFUSION_BLOCKED_REASON_INFUSION_NOT_KNOWN:   "the character does not know this infusion",
	}[reason]
	err := connect.NewError(connect.CodeFailedPrecondition, errors.New(msg))
	if detail, detailErr := connect.NewErrorDetail(&charactersv1.InfusionBlocked{Reason: reason}); detailErr == nil {
		err.AddDetail(detail)
	}
	return err
}
