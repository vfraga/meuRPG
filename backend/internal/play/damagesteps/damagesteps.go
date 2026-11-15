// Package damagesteps works out what a damage comes to once the target's resistance, vulnerability, immunity and flat
// reductions apply (SRD 5.1, "Damage Resistance and Vulnerability"), keeps the steps in the form the store holds them in
// (Group, JSON) so the master can leave a source out and have them worked out again, and writes each step in Portuguese
// with the hiding of an effect the master does not show. The rules are package combat's; play reads the combat for the damage
// and the modifiers and calls here.
package damagesteps

import (
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"slices"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// OtherSource is what a viewer reads in the place of a source the master hides.
const OtherSource = "Outra fonte"

// clamp32 is n limited to lo..hi, as an int32.
func clamp32(n, lo, hi int) int32 {
	return conv.Int32(min(max(n, lo), hi))
}

// Group is the damage of one type and the modifiers that covered it: what the
// steps are worked out from again when the master leaves a source out.
type Group struct {
	DamageType string `json:"damage_type"`
	// Before is the damage of the type before the modifiers.
	Before int32             `json:"before"`
	Mods   []combat.Modifier `json:"mods"`
	// From says where the damage came from: the steps are worked out with it again when
	// the master leaves a source out (a modifier can hold for a nonmagical attack only).
	From combat.Origin `json:"origin,omitzero"`
	// Allot is set on the group of the flat damage reductions, which are taken once from the whole
	// attack: the damage of each type before them (Before is the total, DamageType is empty, Mods are
	// the reductions). The groups of the types then start from what the reduction left.
	Allot map[string]int `json:"allot,omitempty"`
}

// groupSteps works out the steps of one stored group: the single reduction step of an Allot group,
// the adjustment steps of a damage type otherwise.
func groupSteps(g Group, ignored []string) []combat.Step {
	if g.Allot == nil {
		_, steps := combat.Adjust(int(g.Before), g.DamageType, g.Mods, ignored, g.From)
		return steps
	}
	if st, _, ok := combat.Reduce(g.Allot, g.Mods, ignored, g.From); ok {
		return []combat.Step{st}
	}
	return nil
}

// Proto writes the steps the modifiers take the damage through, in Portuguese.
func Proto(groups []Group, ignored []string, names func(string) string, damageTypePT func(string) string, master bool) []*playv1.DamageStep {
	var out []*playv1.DamageStep
	for _, g := range groups {
		gnames := names
		if !master { // an effect the master hides is "Outra fonte" to a player (RN-10)
			gnames = HidingNames(names, g.Mods)
		}
		for _, st := range groupSteps(g, ignored) {
			out = append(out, &playv1.DamageStep{
				Kind: stepKindProto(st.Kind), SourceKeys: shownSourceKeys(st.Sources, g.Mods, master), LabelPt: stepLabel(st, gnames, damageTypePT),
				Before: clamp32(st.Before, 0, math.MaxInt32), After: clamp32(st.After, 0, math.MaxInt32), DamageTypeKey: st.DamageType,
			})
		}
		// A source the master left out is still listed, struck.
		for _, ig := range ignored {
			for _, m := range g.Mods {
				if m.Source == ig && m.Covers(g.DamageType, g.From) {
					out = append(out, &playv1.DamageStep{
						Kind: stepKindProto(m.Kind), SourceKeys: shownSourceKeys([]string{m.Source}, g.Mods, master), LabelPt: stepLabel(combat.Step{Kind: m.Kind, Sources: []string{m.Source}, DamageType: g.DamageType, Qualifier: m.Qualifier}, gnames, damageTypePT),
						Before: g.Before, After: g.Before, DamageTypeKey: g.DamageType, Ignored: true,
					})
				}
			}
		}
	}
	return out
}

// hiddenSources are the keys of the modifiers that come from an effect the master hides.
func hiddenSources(mods []combat.Modifier) []string {
	var out []string
	for _, m := range mods {
		if m.Hidden {
			out = append(out, m.Source)
		}
	}
	return out
}

// HidingNames is names with "Outra fonte" for the sources the master hides.
func HidingNames(names func(string) string, mods []combat.Modifier) func(string) string {
	hidden := hiddenSources(mods)
	if len(hidden) == 0 {
		return names
	}
	return func(key string) string {
		if slices.Contains(hidden, key) {
			return OtherSource
		}
		return names(key)
	}
}

// shownSourceKeys are the keys of a step's sources a viewer may read: a player gets none of an effect the master hides.
func shownSourceKeys(keys []string, mods []combat.Modifier, master bool) []string {
	if master {
		return keys
	}
	hidden := hiddenSources(mods)
	return slices.DeleteFunc(slices.Clone(keys), func(k string) bool { return slices.Contains(hidden, k) })
}

func stepKindProto(kind string) playv1.DamageStepKind {
	switch kind {
	case combat.StepResistance:
		return playv1.DamageStepKind_DAMAGE_STEP_KIND_RESISTANCE
	case combat.StepVulnerability:
		return playv1.DamageStepKind_DAMAGE_STEP_KIND_VULNERABILITY
	case combat.StepImmunity:
		return playv1.DamageStepKind_DAMAGE_STEP_KIND_IMMUNITY
	case combat.StepReduction:
		return playv1.DamageStepKind_DAMAGE_STEP_KIND_REDUCTION
	}
	return playv1.DamageStepKind_DAMAGE_STEP_KIND_UNSPECIFIED
}

// stepLabel writes a step as the screen reads it: "Resistência a fogo (tiefling)",
// "Vulnerabilidade a concussão (Esqueleto)".
func stepLabel(st combat.Step, names func(string) string, damageTypePT func(string) string) string {
	var head string
	switch st.Kind {
	case combat.StepResistance:
		head = "Resistência a "
	case combat.StepVulnerability:
		head = "Vulnerabilidade a "
	case combat.StepImmunity:
		head = "Imunidade a "
	case combat.StepReduction:
		head = "Redução de dano de arma"
	}
	var sources []string
	for _, s := range st.Sources {
		sources = append(sources, sourceNamePT(s, names))
	}
	if st.Kind == combat.StepReduction { // it covers every damage type of the attack
		out := head
		if st.Qualifier == combat.QualifierNonmagical {
			out += " não mágica"
		}
		if len(sources) > 0 {
			out += " (" + joinPT(sources) + ")"
		}
		return out
	}
	out := head + damageTypePT(st.DamageType)
	switch st.Qualifier {
	case combat.QualifierNonmagical:
		out += " de ataque não mágico"
	case combat.QualifierNonmagicalUnlessSilvered:
		out += " de ataque não mágico nem prateado"
	case combat.QualifierNonmagicalUnlessAdamantine:
		out += " de ataque não mágico nem adamantino"
	}
	if len(sources) > 0 {
		out += " (" + joinPT(sources) + ")"
	}
	return out
}

// joinPT joins words with commas and "e".
func joinPT(words []string) string {
	switch len(words) {
	case 0:
		return ""
	case 1:
		return words[0]
	}
	return fmt.Sprintf("%s e %s", joinWith(words[:len(words)-1], ", "), words[len(words)-1])
}

func joinWith(words []string, sep string) string {
	out := ""
	for i, w := range words {
		if i > 0 {
			out += sep
		}
		out += w
	}
	return out
}

// sourceNamePT is the name of a source of a resistance: the race for a racial trait
// ("tiefling"), "fúria", the creature's name for a stat block.
func sourceNamePT(source string, names func(string) string) string {
	switch source {
	case "feature:rage":
		return "fúria"
	case "trait:hellish-resistance":
		return "tiefling"
	case "trait:dwarven-resilience":
		return "anão"
	}
	if len(source) > 25 && source[:25] == "trait:draconic-ancestry-" {
		return "draconato"
	}
	if n := names(source); n != "" {
		return n
	}
	return source
}

// Landing is what a damage comes to once its modifiers apply.
type Landing struct {
	// Amount is the damage that lands, Groups the damage of the types the modifiers
	// changed, and Changed says any did.
	Amount  int32
	Groups  []Group
	Changed bool
}

// Settle takes the damage through the target's modifiers. byType is the damage by type
// (one entry for a damage of one type). The flat reductions are taken once from the whole attack
// first; then each type goes through immunity, resistance and vulnerability.
func Settle(byType map[string]int, mods []combat.Modifier, ignored []string, from combat.Origin) Landing {
	var out Landing
	amounts := byType
	if _, after, ok := combat.Reduce(byType, mods, ignored, from); ok {
		out.Groups, out.Changed, amounts = append(out.Groups, reductionGroup(byType, mods, from)), true, after
	}
	others := slices.DeleteFunc(slices.Clone(mods), func(m combat.Modifier) bool { return m.Kind == combat.StepReduction })
	for _, t := range slices.Sorted(maps.Keys(amounts)) {
		final, steps := combat.Adjust(amounts[t], t, others, ignored, from)
		if len(steps) > 0 || out.Changed {
			group := Group{DamageType: t, Before: clamp32(amounts[t], 0, math.MaxInt32), From: from}
			for _, m := range others {
				if m.Covers(t, from) {
					group.Mods = append(group.Mods, combat.Modifier{Kind: m.Kind, Source: m.Source, DamageTypes: []string{t}, Qualifier: m.Qualifier, Hidden: m.Hidden})
				}
			}
			out.Groups = append(out.Groups, group)
			out.Changed = out.Changed || len(steps) > 0
		}
		out.Amount += clamp32(final, 0, math.MaxInt32)
	}
	return out
}

// reductionGroup is the stored group of the flat reductions taken once from the whole attack.
func reductionGroup(byType map[string]int, mods []combat.Modifier, from combat.Origin) Group {
	group := Group{Before: clamp32(sumOf(byType), 0, math.MaxInt32), From: from, Allot: maps.Clone(byType)}
	for _, m := range mods {
		if m.Kind == combat.StepReduction && m.Covers("", from) {
			group.Mods = append(group.Mods, m)
		}
	}
	return group
}

func sumOf(byType map[string]int) int {
	n := 0
	for _, v := range byType {
		n += max(v, 0)
	}
	return n
}

// Reapply works the Landing out again from the stored groups, leaving out the sources
// the master told the app to ignore. total is the damage before any modifier.
func Reapply(total int32, groups []Group, ignored []string) int32 {
	out := total
	var allotted map[string]int
	for _, g := range groups {
		if g.Allot == nil {
			continue
		}
		if st, after, ok := combat.Reduce(g.Allot, g.Mods, ignored, g.From); ok {
			allotted = after
			out += clamp32(st.After, 0, math.MaxInt32) - g.Before
		} else {
			allotted = g.Allot
		}
	}
	for _, g := range groups {
		if g.Allot != nil {
			continue
		}
		before := int(g.Before)
		if allotted != nil {
			before = allotted[g.DamageType]
		}
		after, _ := combat.Adjust(before, g.DamageType, g.Mods, ignored, g.From)
		out += clamp32(after, 0, math.MaxInt32) - clamp32(before, 0, math.MaxInt32)
	}
	return max(out, 0)
}

// Mods are the modifiers the stored steps were worked out with.
func Mods(groups []Group) []combat.Modifier {
	var mods []combat.Modifier
	for _, g := range groups {
		mods = append(mods, g.Mods...)
	}
	return mods
}

// Origin is where the damage of the stored steps came from: the same for every group.
func Origin(groups []Group) combat.Origin {
	if len(groups) == 0 {
		return combat.Origin{}
	}
	return groups[0].From
}

// Read reads the stored steps of a pending damage; none when there are none or they do not parse.
func Read(raw []byte) []Group {
	var out []Group
	if len(raw) == 0 || json.Unmarshal(raw, &out) != nil {
		return nil
	}
	return out
}
