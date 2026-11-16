package play

import (
	"strings"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// The lines about the Inspiration in the combat's log (RN-43): the pass the master accepted and the Inspiration he gave (FAVOR_PASSED; the
// grant is his alone: the line only gives him the undo of his last action, no player reads it), a pass he refused or the
// giver gave up (FAVOR_PASS_ANSWERED, which also reads the master's undo of an accepted pass to the receiver), and the Inspiration he gave
// back (FAVOR_REFUNDED). Each reader gets a sentence built for them: the master reads the names, the giver (or the roller) their own part,
// the receiver only that they have it now. Another player reads nothing; the audience is stamped in the event (favorInCombat), so a
// hidden combatant in it keeps the line the master's alone.

// The variants of a favor line.
const (
	favorLineGranted  = "granted"
	favorLineMoved    = "passed"
	favorLineRefused  = "refused"
	favorLineCanceled = "canceled"
	favorLineUndone   = "undone"
	favorLineRefunded = "refunded"
)

// favorLogLine is the handler of the events that are a line about the Inspiration.
func favorLogLine(kind playv1.CombatLogKind, variant string) logHandler {
	return func(_ *logBuilder, _ playdb.ListEncounterEventsRow, entry *logEntry) bool {
		entry.kind, entry.favor = kind, variant
		return true
	}
}

// favorUndoneLine is the line the master's undo of an accepted pass leaves for the receiver; the undo's event is its host.
func favorUndoneLine(e playdb.ListEncounterEventsRow) *logEntry {
	ev, err := readEvent(e.Payload)
	if err != nil || ev.UndoneKind != eventFavorPassed || ev.Round == 0 {
		return nil
	}
	return &logEntry{
		id: e.ID, at: e.CreatedAt, ev: ev, hosts: []string{e.ID}, actorUser: e.ActorUserID,
		kind: playv1.CombatLogKind_COMBAT_LOG_KIND_FAVOR_PASS_ANSWERED, favor: favorLineUndone,
	}
}

// fillFavor writes the sentence this reader may read, and nothing for a reader the line says nothing to.
func (e *logEntry) fillFavor(c *logViewing, out *playv1.CombatLogEntry) bool {
	text := favorText(e.favor, c.v.master, c.v.owns(c.actor), c.v.owns(c.target), c.actor.Label, c.target.Label)
	if text == "" {
		return false
	}
	if e.favor == favorLineUndone { // the sentence names nobody, and neither do the fields
		out.ActorId, out.ActorLabel = "", ""
	}
	out.Favor = &playv1.CombatLogFavor{TextPt: text}
	return true
}

// The line variants are named favorLineMoved, not ...Passed: gosec G101 reads a key named ...Pass... as a credential.
var favorMasterWords = map[string]string{
	favorLineGranted:  "O mestre deu a Inspiração a {a}.",
	favorLineMoved:    "{a} passou a Inspiração para {t}.",
	favorLineRefused:  "O mestre recusou o pedido de {a}: a Inspiração continua com {a}.",
	favorLineCanceled: "{a} desistiu de passar a Inspiração para {t}.",
	favorLineRefunded: "O mestre devolveu a Inspiração de {a}.",
}

var favorGiverWords = map[string]string{
	favorLineMoved:    "O mestre aceitou: a sua Inspiração foi para {t}.",
	favorLineRefused:  "O mestre recusou: a Inspiração continua com você.",
	favorLineCanceled: "Você desistiu de passar a Inspiração.",
	favorLineRefunded: "O mestre devolveu a sua Inspiração.",
}

// favorText is the sentence of a variant for a reader: the master, the owner of the actor (the giver, the roller) or of the target (the
// receiver). The actor's text wins for a player who is both.
func favorText(variant string, master, ownsActor, ownsTarget bool, actor, target string) string {
	names := strings.NewReplacer("{a}", actor, "{t}", target)
	switch {
	case master:
		return names.Replace(favorMasterWords[variant])
	case ownsActor:
		return names.Replace(favorGiverWords[variant])
	case ownsTarget && variant == favorLineMoved:
		return actor + " passou a Inspiração para você."
	case ownsTarget && variant == favorLineUndone:
		return "O mestre desfez a passagem: a Inspiração não está mais com você."
	}
	return ""
}
