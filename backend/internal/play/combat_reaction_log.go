package play

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/types/known/timestamppb"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/reaction"
)

// The log's line of a reaction (PM-04c, 10): one line for each reaction used, written
// on the server for each reader so that the browser never has the master's text. The
// master's line has the numbers of the NPCs (the dice, the total against the armor
// class, the damage); the players' has none, and never names a reactor they do not see
// (a hidden NPC that countered a spell is "A Bola de Fogo de Pensantus foi anulada.").

// ordinalLevel writes a spell level as the app does: "3º nível".
func ordinalLevel(level int32) string { return fmt.Sprintf("%dº nível", level) }

// reactionView builds the log entry of an answered window for the viewer, and false
// when the viewer does not get it.
func (e *logEntry) reactionEntry(ctx context.Context, v combatViewer, byID map[string]playdb.Combatant, names *keyNames) (*playv1.CombatLogEntry, bool) {
	text, named := e.reactionLine(ctx, v, byID, names)
	if text == "" {
		return nil, false
	}
	// On a fogged map the line is for who saw it happen, also when it names nobody; a reactor's own
	// player always has their line.
	if !v.master && !e.ev.seenByViewer(v) && !v.owns(byID[e.ev.Actor]) {
		return nil, false
	}
	actor := byID[e.ev.Actor]
	out := &playv1.CombatLogEntry{
		Id: e.id, Kind: e.kind, At: timestamppb.New(e.at), Round: e.ev.Round, ReactionTextPt: text,
	}
	if v.master || named {
		out.ActorId, out.ActorLabel = actor.ID, actor.Label
	}
	if v.master {
		// The players' line, if they get one: a line they do not get is marked.
		playersText, _ := e.reactionLine(ctx, combatViewer{}, byID, names)
		out.Hidden = playersText == ""
		out.Undoable = false
	}
	return out, true
}

// reactionLine is the line for the viewer, and whether it names the reactor.
func (e *logEntry) reactionLine(ctx context.Context, v combatViewer, byID map[string]playdb.Combatant, names *keyNames) (string, bool) { //nolint:gocognit,gocyclo // one flat case for each reaction; they share the visibility facts above the switch
	re := e.ev.Reaction
	if re == nil {
		return "", false
	}
	reactor, other := byID[e.ev.Actor], byID[e.ev.Target]
	// The reactor is named to a player only when they see it: a combatant the master
	// hid, now or when it happened, is not (RN-10).
	seesReactor := v.master || (!reactor.Hidden && !e.ev.Secret) || v.owns(reactor)
	// The other combatant of the line (the caster, the attacker, the aggressor) the same.
	seesOther := v.master || (other.ID != "" && !other.Hidden)
	who := reactor.Label
	if reactor.ID == "" {
		who = "Alguém"
	}
	spellName := func(key string) string {
		if n := names.contentName(ctx, key); n != "" {
			return n
		}
		return key
	}
	by := ""
	if re.ByMaster && reactor.Kind == kindPlayer && (v.master || v.owns(reactor)) {
		by = " (respondido pelo mestre)"
	}
	numbers := v.master || v.owns(reactor) // the reactor's own dice are theirs, an NPC's are the master's
	if reactor.Kind != kindPlayer {
		numbers = v.master
	}
	if re.Kind == reactionHeldDropped {
		if !v.master {
			return "", false
		}
		what := map[string]string{"cast": "A conjuração", "attack": "O ataque", "damage": "O dano"}[re.Roll]
		return fmt.Sprintf("%s de %s esperava uma reação e não pôde acontecer depois das respostas (%s).", what, who, re.Why), true
	}
	switch reaction.Kind(re.Kind) {
	case reaction.CounterspellKind:
		if !re.Used {
			return "", false
		}
		cast := spellName(re.Spell)
		caster := other.Label
		if !seesOther {
			caster = ""
		}
		owner := ""
		if caster != "" {
			owner = " de " + caster
		}
		switch {
		case re.Countered && seesReactor && v.master:
			how := "sem teste"
			if re.CheckDC > 0 {
				how = fmt.Sprintf("teste de habilidade %d contra CD %d", re.Check, re.CheckDC)
			}
			return fmt.Sprintf("%s usou Contramágica (espaço de %s): a %s%s (%s) foi anulada, %s.%s", who, slotText(re), cast, owner, ordinalLevel(re.Level), how, by), true
		case re.Countered && seesReactor:
			return fmt.Sprintf("%s usou Contramágica: a %s%s foi anulada.%s", who, cast, owner, by), true
		case re.Countered:
			return fmt.Sprintf("A %s%s foi anulada.", cast, owner), false
		case seesReactor && v.master:
			return fmt.Sprintf("%s usou Contramágica (espaço de %s): a %s%s (%s) não foi anulada: teste de habilidade %d contra CD %d.%s", who, slotText(re), cast, owner, ordinalLevel(re.Level), re.Check, re.CheckDC, by), true
		case seesReactor:
			return fmt.Sprintf("%s usou Contramágica, mas a %s%s não foi anulada.%s", who, cast, owner, by), true
		}
		return "", false // a reactor the players do not see, and the spell went on: nothing to say
	case reaction.UncannyDodgeKind:
		if !re.Used || !seesReactor {
			return "", false
		}
		if numbers && v.master {
			return fmt.Sprintf("%s usou Esquiva Sobrenatural: o dano de %s caiu de %d para %d.%s", who, other.Label, re.Before, re.After, by), true
		}
		return fmt.Sprintf("%s usou Esquiva Sobrenatural: o dano caiu pela metade.%s", who, by), true
	case reaction.ManeuverReduceKind:
		if !re.Used || !seesReactor {
			return "", false
		}
		name := re.ManeuverName
		if name == "" {
			name = "uma manobra"
		}
		if numbers && re.Die > 0 {
			return fmt.Sprintf("%s usou %s: 1d%d (%d) reduziu o dano, de %d, a %d.%s", who, name, re.DieSides, re.Die, re.Before, re.After, by), true
		}
		return fmt.Sprintf("%s usou %s: o dano caiu.%s", who, name, by), true
	case reaction.DeflectKind:
		if !re.Used || !seesReactor || re.Die == 0 && !re.Caught {
			return "", false
		}
		if numbers && re.Die > 0 {
			return fmt.Sprintf("%s usou Defletir Projéteis: 1d10 (%d) reduziu o dano, de %d, a %d%s.%s", who, re.Die, re.Before, re.After, caughtText(re.Caught), by), true
		}
		return fmt.Sprintf("%s usou Defletir Projéteis: o dano caiu%s.%s", who, caughtTextPlayers(re), by), true
	case reaction.CuttingWords:
		if !re.Used || !seesReactor {
			return "", false
		}
		target := "o ataque"
		kindText := "do ataque"
		switch re.Roll {
		case "damage":
			target, kindText = "o dano", "da jogada de dano"
		case "test":
			target, kindText = "o teste", "do teste"
		}
		actorText := ""
		if seesOther && other.Label != "" {
			actorText = " de " + other.Label
		}
		if re.Ineffect {
			if v.master {
				return fmt.Sprintf("%s usou Palavras de Interrupção: d%d (%d), sem efeito: %s não ouve o bardo ou é imune a enfeitiçar.", who, re.DieSides, re.Die, orSomeone(other.Label)), true
			}
			return fmt.Sprintf("%s usou Palavras de Interrupção: sem efeito.%s", who, by), true
		}
		if v.master {
			tail := ""
			if re.Roll == "attack" || re.Roll == "" {
				tail = fmt.Sprintf(" contra CA %d: %s", re.AC, outcomeWord(re.Outcome))
			}
			return fmt.Sprintf("%s usou Palavras de Interrupção: d%d (%d) subtraído %s%s: %d → %d%s.%s", who, re.DieSides, re.Die, kindText, actorText, re.Before, re.After, tail, by), true
		}
		verb := "diminuiu"
		if re.Roll == "attack" || re.Roll == "" {
			verb = "errou"
			if re.Outcome == outcomeHit || re.Outcome == outcomeCrit {
				verb = "ainda acertou"
			}
		}
		return fmt.Sprintf("%s usou Palavras de Interrupção: %s%s %s.%s", who, target, actorText, verb, by), true
	case reaction.HellishRebukeKind:
		if !re.Used || re.SaveDC == 0 || !seesReactor {
			return "", false
		}
		agg := ""
		if seesOther {
			agg = " contra " + other.Label
		}
		if v.master {
			result := "passou"
			if !re.Saved {
				result = "falhou"
			}
			// The d20 the app rolled again for a halfling player's save (combat_lucky.go), with the numbers.
			return fmt.Sprintf("%s usou Repreensão Infernal (%s) %s: teste de Destreza 1d20 (%d) + %d = %d contra CD %d: %s; %d de fogo.%s%s", who, rebukeCost(re), agg[1:], re.SaveD20, re.SaveBonus, re.SaveD20+re.SaveBonus, re.SaveDC, result, re.Fire, luckyNote(e.ev.Lucky), by), true
		}
		if seesOther {
			if re.Saved {
				return fmt.Sprintf("%s usou Repreensão Infernal%s: %s passou no teste de resistência e sofreu metade do fogo.%s", who, agg, other.Label, by), true
			}
			return fmt.Sprintf("%s usou Repreensão Infernal%s: %s falhou no teste de resistência e sofreu fogo.%s", who, agg, other.Label, by), true
		}
		return fmt.Sprintf("%s usou Repreensão Infernal.%s", who, by), true
	case reaction.FeatherFall:
		if !re.Used || !seesReactor {
			return "", false
		}
		return fmt.Sprintf("%s usou Queda Suave: %s.%s", who, fallText(re, byID, v), by), true
	case reaction.Concentration:
		spell := spellName(re.Spell)
		if !seesReactor {
			return "", false
		}
		lucky := luckyNote(e.ev.Lucky) // the d20 the halfling's Lucky rolled again, with the numbers
		if re.Kept {
			if numbers && re.SaveD20 > 0 {
				return fmt.Sprintf("%s manteve a concentração em %s: 1d20 (%d) + %d = %d contra CD %d.%s%s", who, spell, re.SaveD20, re.SaveBonus, re.SaveD20+re.SaveBonus, re.SaveDC, lucky, by), true
			}
			return fmt.Sprintf("%s manteve a concentração em %s.%s", who, spell, by), true
		}
		if numbers {
			return fmt.Sprintf("%s falhou no teste de concentração em %s: 1d20 (%d) + %d = %d contra CD %d.%s%s", who, spell, re.SaveD20, re.SaveBonus, re.SaveD20+re.SaveBonus, re.SaveDC, lucky, by), true
		}
		return "", false // the line that says the concentration ended is the conditions one
	}
	return "", false
}

func slotText(re *reactionEvent) string {
	if re.Slot == nil {
		return ""
	}
	return ordinalLevel(re.Slot.Level)
}

func orSomeone(label string) string {
	if label == "" {
		return "o alvo"
	}
	return label
}

func outcomeWord(o string) string {
	if o == outcomeHit || o == outcomeCrit {
		return "acertou"
	}
	return "errou"
}

func caughtText(caught bool) string {
	if caught {
		return "; apanhou o projétil"
	}
	return ""
}

func caughtTextPlayers(re *reactionEvent) string {
	if re.Caught {
		return " a 0 e apanhou o projétil"
	}
	return ""
}

func rebukeCost(re *reactionEvent) string {
	if re.Racial {
		return fmt.Sprintf("%s, Legado Infernal", ordinalLevel(re.Level))
	}
	return fmt.Sprintf("espaço de %s", ordinalLevel(re.Level))
}

// fallText names the creatures a Feather Fall saved, the ones the reader sees.
func fallText(re *reactionEvent, byID map[string]playdb.Combatant, v combatViewer) string {
	var labels []string
	for _, id := range re.Saved2 {
		if x, ok := byID[id]; ok && (v.master || !x.Hidden) {
			labels = append(labels, x.Label)
		}
	}
	switch len(labels) {
	case 0:
		return "ninguém sofre dano de queda"
	case 1:
		return labels[0] + " não sofre dano de queda"
	}
	return joinLabels(labels) + " não sofrem dano de queda"
}

func joinLabels(l []string) string {
	if len(l) == 1 {
		return l[0]
	}
	out := ""
	for i, x := range l[:len(l)-1] {
		if i > 0 {
			out += ", "
		}
		out += x
	}
	return out + " e " + l[len(l)-1]
}
