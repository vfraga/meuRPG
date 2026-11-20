// Package effecttext holds the words the card of an effect on a character says, for what needs no store, no transport and no
// Service: the effect's own title, the whole length it was given and the time it has left.
package effecttext

import (
	"cmp"
	"fmt"
	"strings"

	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The kinds of source a stored effect has, as the effect's row spells them, that the total length reads.
const (
	sourceItem  = "item"
	sourceSpell = "spell"
)

// EffectTitle is the effect's own name: the potion's ("Resistência a fogo") when the key is a potion with one, else the source's.
func EffectTitle(content *rules.Content, key, sourceName string) string {
	return cmp.Or(content.PotionTitlePT(key), sourceName)
}

// TotalDuration is the whole length an effect was given, as a sentence part ("por 1 hora"), and "" when the row does not say it.
// The row keeps only the seconds that are left, so the total is told only for the two writers that give the catalog's length and
// no other: a potion drunk (source kind "item": the potion's seconds) and a spell cast by a character (source kind "spell" with a
// caster: the spell's seconds). An effect the master put on, whose length the master may have chosen, an effect that is not timed
// and any other source have no total.
func TotalDuration(content *rules.Content, key, sourceKind string, hasCaster, timed bool) string {
	if !timed {
		return ""
	}
	if sourceKind == sourceItem {
		return content.PotionLengthPT(key)
	}
	seconds := 0
	if sourceKind == sourceSpell && hasCaster {
		seconds, _ = content.SpellEffectSeconds(key)
	}
	if seconds <= 0 {
		return ""
	}
	return "por " + rules.DurationPT(seconds)
}

// GameTime writes a game time the way the card reads it outside a combat: "dura 1 minuto",
// "dura 54 segundos", "dura 8 horas" (a round is 6 seconds; the app has no wall clock).
func GameTime(seconds *int32, concentration bool) string {
	if seconds == nil {
		if concentration {
			return "Dura enquanto quem a conjurou se concentrar"
		}
		return "Dura até o mestre encerrar"
	}
	left := int(*seconds)
	var parts []string
	unit := func(n int, one, many string) {
		if n == 0 {
			return
		}
		if n == 1 {
			parts = append(parts, "1 "+one)
			return
		}
		parts = append(parts, fmt.Sprintf("%d %s", n, many))
	}
	unit(left/3600, "hora", "horas")
	unit(left%3600/60, "minuto", "minutos")
	unit(left%60, "segundo", "segundos")
	if len(parts) == 0 {
		return "Dura menos de um segundo"
	}
	return "Dura " + strings.Join(parts, " e ")
}
