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

// reactionFacts are what every reaction's line shares: who the reactor and the other
// combatant are, and what the viewer may read of them.
type reactionFacts struct {
	ctx         context.Context
	e           *logEntry
	re          *reactionEvent
	v           combatViewer
	byID        map[string]playdb.Combatant
	names       *keyNames
	reactor     playdb.Combatant
	other       playdb.Combatant
	seesReactor bool
	seesOther   bool
	numbers     bool   // the dice and the damage numbers are the viewer's to read
	who         string // the reactor's label, "Alguém" when it left the combat
	by          string // " (respondido pelo mestre)" for the master's own answer to a player
}

// reactionLiner writes the line of one kind of reaction: the text, and whether it names the reactor.
type reactionLiner func(f *reactionFacts) (string, bool)

// reactionLiners are the lines by reaction kind; a kind with none (the Shield's is another
// entry) says nothing.
var reactionLiners = map[reaction.Kind]reactionLiner{
	reaction.CounterspellKind:   (*reactionFacts).counterspell,
	reaction.UncannyDodgeKind:   (*reactionFacts).uncannyDodge,
	reaction.ManeuverReduceKind: (*reactionFacts).maneuverReduce,
	reaction.DeflectKind:        (*reactionFacts).deflect,
	reaction.CuttingWords:       (*reactionFacts).cuttingWords,
	reaction.HellishRebukeKind:  (*reactionFacts).hellishRebuke,
	reaction.FeatherFall:        (*reactionFacts).featherFall,
	reaction.Concentration:      (*reactionFacts).concentration,
}

// reactionLine is the line for the viewer, and whether it names the reactor.
func (e *logEntry) reactionLine(ctx context.Context, v combatViewer, byID map[string]playdb.Combatant, names *keyNames) (string, bool) {
	re := e.ev.Reaction
	if re == nil {
		return "", false
	}
	f := e.reactionFacts(ctx, re, v, byID, names)
	if re.Kind == reactionHeldDropped {
		return f.heldDropped()
	}
	if line, ok := reactionLiners[reaction.Kind(re.Kind)]; ok {
		return line(f)
	}
	return "", false
}

func (e *logEntry) reactionFacts(ctx context.Context, re *reactionEvent, v combatViewer, byID map[string]playdb.Combatant, names *keyNames) *reactionFacts {
	reactor, other := byID[e.ev.Actor], byID[e.ev.Target]
	f := &reactionFacts{
		ctx: ctx, e: e, re: re, v: v, byID: byID, names: names, reactor: reactor, other: other,
		// The reactor is named to a player only when they see it: a combatant the master
		// hid, now or when it happened, is not (RN-10).
		seesReactor: v.master || (!reactor.Hidden && !e.ev.Secret) || v.owns(reactor),
		// The other combatant of the line (the caster, the attacker, the aggressor) the same.
		seesOther: v.master || (other.ID != "" && !other.Hidden),
		who:       reactor.Label,
	}
	if reactor.ID == "" {
		f.who = "Alguém"
	}
	if re.ByMaster && reactor.Kind == kindPlayer && (v.master || v.owns(reactor)) {
		f.by = " (respondido pelo mestre)"
	}
	f.numbers = v.master || v.owns(reactor) // the reactor's own dice are theirs, an NPC's are the master's
	if reactor.Kind != kindPlayer {
		f.numbers = v.master
	}
	return f
}

func (f *reactionFacts) spellName(key string) string {
	if n := f.names.contentName(f.ctx, key); n != "" {
		return n
	}
	return key
}

// heldDropped is the master's line of a held action that could not happen after the answers.
func (f *reactionFacts) heldDropped() (string, bool) {
	if !f.v.master {
		return "", false
	}
	what := map[string]string{"cast": "A conjuração", "attack": "O ataque", "damage": "O dano"}[f.re.Roll]
	return fmt.Sprintf("%s de %s esperava uma reação e não pôde acontecer depois das respostas (%s).", what, f.who, f.re.Why), true
}

func (f *reactionFacts) counterspell() (string, bool) {
	re := f.re
	if !re.Used {
		return "", false
	}
	cast := f.spellName(re.Spell)
	owner := ""
	if f.seesOther && f.other.Label != "" {
		owner = " de " + f.other.Label
	}
	who, by := f.who, f.by
	switch {
	case re.Countered && f.seesReactor && f.v.master:
		how := "sem teste"
		if re.CheckDC > 0 {
			how = fmt.Sprintf("teste de habilidade %d contra CD %d", re.Check, re.CheckDC)
		}
		return fmt.Sprintf("%s usou Contramágica (espaço de %s): a %s%s (%s) foi anulada, %s.%s", who, slotText(re), cast, owner, ordinalLevel(re.Level), how, by), true
	case re.Countered && f.seesReactor:
		return fmt.Sprintf("%s usou Contramágica: a %s%s foi anulada.%s", who, cast, owner, by), true
	case re.Countered:
		return fmt.Sprintf("A %s%s foi anulada.", cast, owner), false
	case f.seesReactor && f.v.master:
		return fmt.Sprintf("%s usou Contramágica (espaço de %s): a %s%s (%s) não foi anulada: teste de habilidade %d contra CD %d.%s", who, slotText(re), cast, owner, ordinalLevel(re.Level), re.Check, re.CheckDC, by), true
	case f.seesReactor:
		return fmt.Sprintf("%s usou Contramágica, mas a %s%s não foi anulada.%s", who, cast, owner, by), true
	}
	return "", false // a reactor the players do not see, and the spell went on: nothing to say
}

func (f *reactionFacts) uncannyDodge() (string, bool) {
	re := f.re
	if !re.Used || !f.seesReactor {
		return "", false
	}
	if f.numbers && f.v.master {
		return fmt.Sprintf("%s usou Esquiva Sobrenatural: o dano de %s caiu de %d para %d.%s", f.who, f.other.Label, re.Before, re.After, f.by), true
	}
	return fmt.Sprintf("%s usou Esquiva Sobrenatural: o dano caiu pela metade.%s", f.who, f.by), true
}

func (f *reactionFacts) maneuverReduce() (string, bool) {
	re := f.re
	if !re.Used || !f.seesReactor {
		return "", false
	}
	name := re.ManeuverName
	if name == "" {
		name = "uma manobra"
	}
	if f.numbers && re.Die > 0 {
		return fmt.Sprintf("%s usou %s: 1d%d (%d) reduziu o dano, de %d, a %d.%s", f.who, name, re.DieSides, re.Die, re.Before, re.After, f.by), true
	}
	return fmt.Sprintf("%s usou %s: o dano caiu.%s", f.who, name, f.by), true
}

func (f *reactionFacts) deflect() (string, bool) {
	re := f.re
	if !re.Used || !f.seesReactor || re.Die == 0 && !re.Caught {
		return "", false
	}
	if f.numbers && re.Die > 0 {
		return fmt.Sprintf("%s usou Defletir Projéteis: 1d10 (%d) reduziu o dano, de %d, a %d%s.%s", f.who, re.Die, re.Before, re.After, caughtText(re.Caught), f.by), true
	}
	return fmt.Sprintf("%s usou Defletir Projéteis: o dano caiu%s.%s", f.who, caughtTextPlayers(re), f.by), true
}

func (f *reactionFacts) cuttingWords() (string, bool) {
	re := f.re
	if !re.Used || !f.seesReactor {
		return "", false
	}
	target, kindText := cuttingWordsTarget(re.Roll)
	actorText := ""
	if f.seesOther && f.other.Label != "" {
		actorText = " de " + f.other.Label
	}
	if re.Ineffect {
		if f.v.master {
			return fmt.Sprintf("%s usou Palavras de Interrupção: d%d (%d), sem efeito: %s não ouve o bardo ou é imune a enfeitiçar.", f.who, re.DieSides, re.Die, orSomeone(f.other.Label)), true
		}
		return fmt.Sprintf("%s usou Palavras de Interrupção: sem efeito.%s", f.who, f.by), true
	}
	if f.v.master {
		tail := ""
		if re.Roll == "attack" || re.Roll == "" {
			tail = fmt.Sprintf(" contra CA %d: %s", re.AC, outcomeWord(re.Outcome))
		}
		return fmt.Sprintf("%s usou Palavras de Interrupção: d%d (%d) subtraído %s%s: %d → %d%s.%s", f.who, re.DieSides, re.Die, kindText, actorText, re.Before, re.After, tail, f.by), true
	}
	verb := "diminuiu"
	if re.Roll == "attack" || re.Roll == "" {
		verb = "errou"
		if re.Outcome == outcomeHit || re.Outcome == outcomeCrit {
			verb = "ainda acertou"
		}
	}
	return fmt.Sprintf("%s usou Palavras de Interrupção: %s%s %s.%s", f.who, target, actorText, verb, f.by), true
}

// cuttingWordsTarget names what the die came off: the attack (also when the roll is not
// said), the damage or the test.
func cuttingWordsTarget(roll string) (target, kindText string) {
	switch roll {
	case "damage":
		return "o dano", "da jogada de dano"
	case "test":
		return "o teste", "do teste"
	}
	return "o ataque", "do ataque"
}

func (f *reactionFacts) hellishRebuke() (string, bool) {
	re := f.re
	if !re.Used || re.SaveDC == 0 || !f.seesReactor {
		return "", false
	}
	if f.v.master {
		return f.hellishRebukeMaster(), true
	}
	if !f.seesOther {
		return fmt.Sprintf("%s usou Repreensão Infernal.%s", f.who, f.by), true
	}
	agg := " contra " + f.other.Label
	if re.Saved {
		return fmt.Sprintf("%s usou Repreensão Infernal%s: %s passou no teste de resistência e sofreu metade do fogo.%s", f.who, agg, f.other.Label, f.by), true
	}
	return fmt.Sprintf("%s usou Repreensão Infernal%s: %s falhou no teste de resistência e sofreu fogo.%s", f.who, agg, f.other.Label, f.by), true
}

// hellishRebukeMaster is the master's line, with the aggressor's save.
func (f *reactionFacts) hellishRebukeMaster() string {
	re := f.re
	agg := ""
	if f.seesOther {
		agg = " contra " + f.other.Label
	}
	result := "passou"
	if !re.Saved {
		result = "falhou"
	}
	// The d20 the app rolled again for a halfling player's save (combat_lucky.go), with the numbers.
	return fmt.Sprintf("%s usou Repreensão Infernal (%s) %s: teste de Destreza 1d20 (%d) + %d = %d contra CD %d: %s; %d de fogo.%s%s", f.who, rebukeCost(re), agg[1:], re.SaveD20, re.SaveBonus, re.SaveD20+re.SaveBonus, re.SaveDC, result, re.Fire, luckyNote(f.e.ev.Lucky), f.by)
}

func (f *reactionFacts) featherFall() (string, bool) {
	if !f.re.Used || !f.seesReactor {
		return "", false
	}
	return fmt.Sprintf("%s usou Queda Suave: %s.%s", f.who, fallText(f.re, f.byID, f.v), f.by), true
}

func (f *reactionFacts) concentration() (string, bool) {
	re := f.re
	spell := f.spellName(re.Spell)
	if !f.seesReactor {
		return "", false
	}
	lucky := luckyNote(f.e.ev.Lucky) // the d20 the halfling's Lucky rolled again, with the numbers
	if re.Kept {
		if f.numbers && re.SaveD20 > 0 {
			return fmt.Sprintf("%s manteve a concentração em %s: 1d20 (%d) + %d = %d contra CD %d.%s%s", f.who, spell, re.SaveD20, re.SaveBonus, re.SaveD20+re.SaveBonus, re.SaveDC, lucky, f.by), true
		}
		return fmt.Sprintf("%s manteve a concentração em %s.%s", f.who, spell, f.by), true
	}
	if f.numbers {
		return fmt.Sprintf("%s falhou no teste de concentração em %s: 1d20 (%d) + %d = %d contra CD %d.%s%s", f.who, spell, re.SaveD20, re.SaveBonus, re.SaveD20+re.SaveBonus, re.SaveDC, lucky, f.by), true
	}
	return "", false // the line that says the concentration ended is the conditions one
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
