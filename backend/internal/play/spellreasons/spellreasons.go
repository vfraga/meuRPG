// Package spellreasons writes the sentence of the reason a spell line of the turn is disabled, over the proto alone.
package spellreasons

import (
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// Explain writes, on every disabled spell line of the turn, the sentence of its reason
// with the spell's name in it. NO_SLOT, REACTION_ONLY_WHEN_HIT and REACTION_ONLY keep the short words of
// the screen: NO_SLOT repeats on every spell row (the slot rows above the list are the one explanation), and
// a reaction row already says that the app asks when the trigger happens. The sentence is the same for the
// master and for a player: nothing in it is a cause a player may not read (RN-20).
func Explain(o *rulesv1.TurnOptions) {
	for _, sp := range o.GetSpells() {
		if text := spellReasonSentence(sp.GetReason().GetCode(), spellNameOf(sp.GetSpell()), sp.GetEconomy()); text != "" {
			sp.Reason.TextPt = text
		}
	}
	for _, a := range o.GetAttacks() {
		if a.GetAttack().GetKind() == rulesv1.AttackKind_ATTACK_KIND_SPELL {
			if text := spellReasonSentence(a.GetReason().GetCode(), spellNameOf(a.GetAttack()), rulesv1.ActionEconomy_ACTION_ECONOMY_UNSPECIFIED); text != "" {
				a.Reason.TextPt = text
			}
		}
	}
}

// namedByKey is what carries a content key and the names of a spell: a Spell, or the Attack of a cantrip.
type namedByKey interface {
	GetName() string
	GetNamePt() string
	GetKey() string
}

// spellNameOf is the Portuguese name of the spell, or the English one, or its key.
func spellNameOf(n namedByKey) string {
	switch {
	case n.GetNamePt() != "":
		return n.GetNamePt()
	case n.GetName() != "":
		return n.GetName()
	}
	return n.GetKey()
}

// spellReasonSentence is the sentence of a spell's reason, empty for the codes that keep the screen's words.
func spellReasonSentence(code rulesv1.DisabledReasonCode, name string, economy rulesv1.ActionEconomy) string {
	switch code {
	case rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_CASTING_TIME_TOO_LONG:
		return name + " demora demais para conjurar em um combate."
	case rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_BONUS_ACTION_SPELL_LIMIT:
		if economy == rulesv1.ActionEconomy_ACTION_ECONOMY_BONUS_ACTION {
			return "Você já conjurou outra magia neste turno: junto com " + name + ", uma magia de ação bônus, só cabe um truque de 1 ação."
		}
		return "Você já conjurou uma magia de ação bônus neste turno: " + name + " não cabe, só um truque de 1 ação."
	case rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_ARMOR_NOT_PROFICIENT:
		return "Armadura ou escudo sem proficiência: " + name + " não pode ser conjurada."
	}
	return ""
}
