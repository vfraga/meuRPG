package rules

import (
	"fmt"
	"strings"
)

// rollNamesPT and abilityTagsPT are the words of an effect's short labels.
var (
	rollNamesPT = map[string]string{
		RollAppliesAttack: "ataques", RollAppliesSave: "resistências", RollAppliesCheck: "testes",
	}
	abilityTagsPT = map[string]string{
		"str": "FOR", "dex": "DES", "con": "CON", "int": "INT", "wis": "SAB", "cha": "CAR",
	}
	damageNamesPT = map[string]string{
		"acid": "ácido", "bludgeoning": "contundente", "cold": "frio", "fire": "fogo", "force": "energia",
		"lightning": "elétrico", "necrotic": "necrótico", "piercing": "perfurante", "poison": "veneno",
		"psychic": "psíquico", "radiant": "radiante", "slashing": "cortante", "thunder": "trovejante",
	}
	resistanceVerbsPT = map[string]string{
		ResistanceKindResist: "Resistência a", ResistanceKindImmune: "Imunidade a", ResistanceKindVulnerable: "Vulnerabilidade a",
	}
	abilityLabelsPT = map[string]string{
		ModifierSaveAdvantage: "Vantagem em resistências", ModifierSaveDisadvantage: "Desvantagem em resistências",
		ModifierCheckAdvantage: "Vantagem em testes", ModifierCheckDisadvantage: "Desvantagem em testes",
	}
	// fixedTagsPT are the kinds whose label has no number or list in it.
	fixedTagsPT = map[string]string{
		ModifierNoMove: "Sem deslocamento", ModifierNoAction: "Sem ações", ModifierNoReaction: "Sem reações",
		ModifierAttackAdvantage: "Vantagem em ataques", ModifierAttackDisadvantage: "Desvantagem em ataques",
		ModifierExtraAction: "Ação extra",
	}
)

// auraAffectsPT is whom an aura reaches, as the card says it.
var auraAffectsPT = map[string]string{AuraAffectsSelf: "só quem a carrega", AuraAffectsAllies: "aliados", AuraAffectsEnemies: "inimigos"}

// EffectTagsPT are the short labels of a table spell's effect for its card ("CA +2", "Vantagem em ataques"),
// one for each modifier: a table spell has no tags_pt file to read them from.
func EffectTagsPT(mods []EffectModifier) []string {
	out := make([]string, 0, len(mods))
	for _, m := range mods {
		if tag := modifierTagPT(m); tag != "" {
			out = append(out, tag)
		}
	}
	return out
}

func modifierTagPT(m EffectModifier) string {
	if s, ok := fixedTagsPT[m.Kind]; ok {
		return s
	}
	switch m.Kind {
	case ModifierAura:
		return fmt.Sprintf("Aura de %s: %s", MetersPT(m.RadiusFt), auraAffectsPT[m.Affects])
	case ModifierRollBonus:
		return fmt.Sprintf("%+d em %s", m.Value, joinPT(m.AppliesTo, rollNamesPT))
	case ModifierRollDie:
		return fmt.Sprintf("%+dd%d em %s", m.Sign, m.Die, joinPT(m.AppliesTo, rollNamesPT))
	case ModifierACBonus:
		return fmt.Sprintf("CA %+d", m.Value)
	case ModifierBaseAC:
		return fmt.Sprintf("CA base %d", m.Value)
	case ModifierTempHP:
		return fmt.Sprintf("PV temporários: %d", m.Value)
	case ModifierTurnTempHP:
		return "PV temporários por turno"
	case ModifierResistance:
		return resistanceTagPT(m)
	case ModifierSpeedMultiplier:
		return fmt.Sprintf("Deslocamento %d%%", m.Value)
	case ModifierSpeedAdd:
		return "Deslocamento +" + MetersPT(m.Value)
	case ModifierConditionImmunity:
		return "Imunidade a condições"
	case ModifierCheckBonus:
		return fmt.Sprintf("%+d em testes de perícia", m.Value)
	}
	return abilityTagPT(m)
}

// abilityTagPT is the label of the kinds that name abilities ("Vantagem em resistências (SAB)").
func abilityTagPT(m EffectModifier) string {
	what := abilityLabelsPT[m.Kind]
	if what == "" || len(m.Abilities) == 0 {
		return what
	}
	return what + " (" + joinPT(m.Abilities, abilityTagsPT) + ")"
}

func resistanceTagPT(m EffectModifier) string {
	verb := resistanceVerbsPT[m.ResistanceKind]
	if verb == "" {
		verb = resistanceVerbsPT[ResistanceKindResist]
	}
	if len(m.DamageTypes) == 1 {
		switch m.DamageTypes[0] {
		case DamageTypeAll:
			return verb + " todo dano"
		case DamageTypeNonmagicalBPS:
			return verb + " dano de armas não mágicas"
		}
	}
	names := make([]string, len(m.DamageTypes))
	for i, t := range m.DamageTypes {
		key := strings.TrimPrefix(t, "damage-type:")
		if names[i] = damageNamesPT[key]; names[i] == "" {
			names[i] = key
		}
	}
	return verb + " " + strings.Join(names, ", ")
}

// joinPT writes the words of keys as a list: "a, b e c".
func joinPT(keys []string, words map[string]string) string {
	ws := make([]string, len(keys))
	for i, k := range keys {
		if ws[i] = words[k]; ws[i] == "" {
			ws[i] = k
		}
	}
	if len(ws) <= 1 {
		return strings.Join(ws, "")
	}
	return strings.Join(ws[:len(ws)-1], ", ") + " e " + ws[len(ws)-1]
}
