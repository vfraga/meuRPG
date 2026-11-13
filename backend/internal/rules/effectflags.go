package rules

import (
	"slices"
	"strconv"
	"strings"
)

// The closed flags of a flag effect: a rule of the game that a table feature switches on for its owner. The play reads
// them through link.Traits.HasFlag only.
const (
	// FlagNeverSurprised says the character cannot be surprised: no one marks it surprised, and a mark made before the
	// flag came counts for nothing.
	FlagNeverSurprised = "never_surprised"
	// FlagIgnoreHostileNearby says a ranged weapon attack made with a hostile creature within 5 feet gets no
	// disadvantage for it (a spell attack still does).
	FlagIgnoreHostileNearby = "ignore_hostile_nearby"
	// FlagIgnoreDifficultTerrain says difficult terrain painted on the map costs the character no extra movement. The
	// difficult terrain a spell makes still does.
	FlagIgnoreDifficultTerrain = "ignore_difficult_terrain"
)

// effectFlags are the flags a flag effect may name, in the order of the menu.
var effectFlags = []string{FlagNeverSurprised, FlagIgnoreHostileNearby, FlagIgnoreDifficultTerrain}

// effectFlagNames are the Portuguese names and hints of the flags.
var effectFlagNames = map[string][2]string{
	FlagNeverSurprised:         {"Nunca é surpreendido", "O personagem não pode ser surpreendido: o mestre não consegue marcá-lo como surpreso."},
	FlagIgnoreHostileNearby:    {"Ignora hostil por perto", "Um ataque à distância com arma não sofre desvantagem por haver um inimigo a até 1,5 m. Ataques de magia continuam sofrendo."},
	FlagIgnoreDifficultTerrain: {"Ignora terreno difícil do mapa", "O terreno difícil pintado no mapa não custa movimento extra. O terreno difícil criado por magias (Crescer Espinhos, Teia) continua custando."},
}

// effectFlagValues are the flags of the menu, with their names and hints.
func effectFlagValues() []MenuValue {
	out := make([]MenuValue, 0, len(effectFlags))
	for _, k := range effectFlags {
		out = append(out, MenuValue{Key: k, NamePT: effectFlagNames[k][0], HintPT: effectFlagNames[k][1]})
	}
	return out
}

// flagShape says what is wrong with a flag effect's own fields (the field and why), "" when right: a known flag, and
// no tags (a tagged effect is a hint the engine never applies).
func flagShape(e *Effect) (field, msg string) {
	switch {
	case e.Flag == "":
		return "flag", "flag is required: " + strings.Join(effectFlags, " or ")
	case !slices.Contains(effectFlags, e.Flag):
		return "flag", "flag " + strconv.Quote(e.Flag) + " is not on the menu: " + strings.Join(effectFlags, " or ")
	case len(e.Tags) > 0:
		return "tags", "a flag takes no tags"
	}
	return "", ""
}

// flag checks a flag effect: the flag, and no field but when and text_pt.
func (x effectCompiler) flag() error {
	if field, msg := flagShape(x.e); field != "" {
		return x.fail("%s", msg)
	}
	if !x.takesOnly(func(o *Effect) { o.Type, o.Flag, o.When, o.TextPT = "", "", "", ""; o.when = nil }) {
		return x.fail("a flag takes no fields but flag, when and text_pt")
	}
	return nil
}

// checkFlag checks a flag effect of the table's content at its own fields.
func checkFlag(site effectSite, e *Effect) error {
	if field, msg := flagShape(e); field != "" {
		return site.fail("."+field, ReasonValue, "%s", msg)
	}
	return nil
}

// strayFlag refuses the flag field on an effect that is no flag.
func (x effectCompiler) strayFlag() error {
	if x.e.Type != "flag" && x.e.Flag != "" {
		return x.fail("flag belongs to the flag effect")
	}
	return nil
}

// flagEffects fills Derived.Flags: the flags of the effects that hold, sorted and without repeats.
func (x *deriver) flagEffects() {
	for _, a := range x.active {
		if a.effect.Type == "flag" && x.applies(a) && !slices.Contains(x.d.Flags, a.effect.Flag) {
			x.d.Flags = append(x.d.Flags, a.effect.Flag)
		}
	}
	slices.Sort(x.d.Flags)
}
