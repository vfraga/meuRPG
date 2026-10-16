package rules

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// pool is a set of choices whose options are the same keys: the fighter's Fighting
// Style and the champion's second one, the three Metamagic features. The picks are
// shared, so they are dealt to the choices in the order the character got them.
type pool struct {
	entries []*optionEntry
	options map[string]bool
}

// assignPools deals the picks of feature and trait options to the choices that
// offer them, and fills those choices' options.
func (cb *choiceBuilder) assignPools() {
	pools := groupPools(cb.entries)

	// The entry that took each pick, over all pools, to refuse the same option taken
	// twice by name (the Defense style of the fighter and of the paladin).
	takenBy := map[string]*optionEntry{}
	poolOf := map[*optionEntry]*pool{}
	for _, p := range pools {
		for _, e := range p.entries {
			poolOf[e] = p
		}
	}
	for _, p := range pools {
		if len(p.entries) > 0 {
			maps.Copy(takenBy, cb.dealPicks(p))
		}
	}
	byName := cb.firstTakerByName(takenBy)

	for _, p := range pools {
		for i, e := range p.entries {
			cb.fillOptions(p, i, e, poolOf, byName)
		}
	}
}

// groupPools puts the choices that share an option in one pool. A choice that
// offers options of two pools bridges them into one.
func groupPools(entries []*optionEntry) []*pool {
	var pools []*pool
	for _, e := range entries {
		var into *pool
		for _, p := range pools {
			if !p.offersAnyOf(e) {
				continue
			}
			if into == nil {
				into = p
				continue
			}
			into.absorb(p) // the new choice bridges two pools
		}
		if into == nil {
			into = &pool{options: map[string]bool{}}
			pools = append(pools, into)
		}
		into.add(e)
	}
	return pools
}

// offersAnyOf says the choice offers an option of the pool.
func (p *pool) offersAnyOf(e *optionEntry) bool {
	return slices.ContainsFunc(e.options, func(o string) bool { return p.options[o] })
}

// absorb moves the entries and the options of other into p, which other leaves empty.
func (p *pool) absorb(other *pool) {
	p.entries = append(p.entries, other.entries...)
	for o := range other.options {
		p.options[o] = true
	}
	other.entries, other.options = nil, map[string]bool{}
}

func (p *pool) add(e *optionEntry) {
	p.entries = append(p.entries, e)
	for _, o := range e.options {
		p.options[o] = true
	}
}

// dealPicks gives the picks that belong to the pool to its choices, in the order
// the character got them, marks them used and returns the choice that took each.
// The picks no choice has room for overflow to the last choice.
func (cb *choiceBuilder) dealPicks(p *pool) map[string]*optionEntry {
	slices.SortStableFunc(p.entries, func(a, b *optionEntry) int {
		if a.level != b.level {
			return a.level - b.level
		}
		return a.order - b.order
	})
	var inPool []string
	for _, k := range cb.plain {
		if p.options[k] {
			inPool = append(inPool, k)
		}
	}
	assigned := map[string]*optionEntry{}
	for _, e := range p.entries {
		e.choice.Picked = takePicks(e, inPool, assigned)
	}
	last := p.entries[len(p.entries)-1]
	for _, k := range inPool {
		cb.used[k] = true
		if assigned[k] == nil {
			last.choice.Overflow = append(last.choice.Overflow, k)
		}
	}
	return assigned
}

// takePicks are the picks the choice takes from those not assigned yet, up to
// the number it chooses; it records them in assigned.
func takePicks(e *optionEntry, inPool []string, assigned map[string]*optionEntry) []string {
	var picked []string
	for _, k := range inPool {
		if len(picked) >= e.choose {
			break
		}
		if assigned[k] == nil && slices.Contains(e.options, k) {
			picked = append(picked, k)
			assigned[k] = e
		}
	}
	return picked
}

// firstTakerByName is, for each option name, the earliest choice that took an
// option of that name.
func (cb *choiceBuilder) firstTakerByName(takenBy map[string]*optionEntry) map[string]*optionEntry {
	byName := map[string]*optionEntry{}
	for k, e := range takenBy {
		name := cb.c.namePT(k)
		if prev, ok := byName[name]; !ok || e.order < prev.order {
			byName[name] = e
		}
	}
	return byName
}

// fillOptions describes the options of the i-th choice of a pool: the ones an
// earlier choice of the pool took, or a choice of another pool took under the
// same name, are taken. It also sorts the picks by option and lists the unmet ones.
func (cb *choiceBuilder) fillOptions(p *pool, i int, e *optionEntry, poolOf map[*optionEntry]*pool, byName map[string]*optionEntry) {
	var earlier []string
	for _, before := range p.entries[:i] {
		earlier = append(earlier, before.choice.Picked...)
	}
	for _, key := range e.options {
		o := cb.optionOf(e, key)
		switch {
		case slices.Contains(earlier, key):
			o.Prerequisites = append(o.Prerequisites, takenPrerequisite("Você já escolheu esta opção."))
		case byName[cb.c.namePT(key)] != nil && poolOf[byName[cb.c.namePT(key)]] != p:
			o.Prerequisites = append(o.Prerequisites, takenPrerequisite("Você já tem esta opção por outra classe."))
		}
		o.ReasonPT = prerequisitesReason(o.Prerequisites)
		e.choice.Options = append(e.choice.Options, o)
	}
	index := map[string]int{}
	for j, o := range e.choice.Options {
		index[o.Key] = j
	}
	slices.SortFunc(e.choice.Picked, func(a, b string) int { return index[a] - index[b] })
	for _, k := range e.choice.Picked {
		if e.choice.Options[index[k]].Blocked() {
			e.choice.Unmet = append(e.choice.Unmet, k)
		}
	}
}

func takenPrerequisite(reason string) OptionPrerequisite {
	return OptionPrerequisite{Kind: PrerequisiteTaken, Met: false, ReasonPT: reason}
}

// optionOf describes one option of a feature or trait: its name and rule, what it
// asks for and, for a terrain of the Land druid, the spells it gives.
func (cb *choiceBuilder) optionOf(e *optionEntry, key string) ChoiceOption {
	c := cb.c
	o := ChoiceOption{Key: key, Stored: key, NamePT: optionLabel(c.namePT(key))}
	if f := c.features[key]; f != nil && isTableKey(key) && f.Parent != "" {
		// An option of a table's own list: its text is the rule the player reads.
		o.SummaryPT = strings.Join(f.Desc, " ")
		return o
	}
	if c.choices == nil {
		return o
	}
	if a, ok := c.choices.ancestryByTrait[key]; ok {
		o.NamePT = optionLabel(c.namePT(a.Trait))
		o.SummaryPT = fmt.Sprintf("%s · %s · teste de %s", c.namePT(a.DamageType), breathShapePT(a), c.namePT(a.Save))
		return o
	}
	if a, ok := c.choices.ancestryByFeature[key]; ok {
		o.NamePT = optionLabel(c.namePT(a.Trait))
		o.SummaryPT = c.namePT(a.DamageType)
		return o
	}
	o.SummaryPT = c.choices.summaries[key]
	if strings.HasPrefix(key, "feature:circle-of-the-land-") {
		o.CircleSpells = cb.circleSpells(key)
	}
	if e.choice.FeatureKey == invocationsFeature {
		o.Prerequisites = cb.invocationPrerequisites(key)
	}
	return o
}

// breathShapePT writes a breath weapon's area: "cone de 4,5 m" or "linha de 9 m por
// 1,5 m".
func breathShapePT(a *draconicAncestry) string {
	if a.Shape == BreathLine {
		return fmt.Sprintf("linha de %s por %s", metersPT(a.SizeFt), metersPT(a.WidthFt))
	}
	return "cone de " + metersPT(a.SizeFt)
}

// metersPT writes feet as meters the way the app does (5 ft is 1,5 m).
func metersPT(ft int) string {
	tenths := ft * 3
	if tenths%10 == 0 {
		return fmt.Sprintf("%d m", tenths/10)
	}
	return fmt.Sprintf("%d,%d m", tenths/10, tenths%10)
}

// circleSpells are the spells a Land druid's terrain gives, by druid level.
func (cb *choiceBuilder) circleSpells(terrain string) []CircleSpell {
	var out []CircleSpell
	for _, oc := range cb.x.classes {
		if oc.subclass == nil {
			continue
		}
		for _, ss := range oc.subclass.Spells {
			if slices.Contains(ss.WithFeatures, terrain) {
				out = append(out, CircleSpell{Level: ss.ClassLevel, Key: ss.Spell, NamePT: cb.c.namePT(ss.Spell), Reached: oc.level >= ss.ClassLevel})
			}
		}
	}
	slices.SortStableFunc(out, func(a, b CircleSpell) int {
		if a.Level != b.Level {
			return a.Level - b.Level
		}
		return strings.Compare(foldPT(a.NamePT), foldPT(b.NamePT))
	})
	return out
}

// prerequisitesReason is the one sentence that says why an option cannot be taken.
func prerequisitesReason(ps []OptionPrerequisite) string {
	var unmet []OptionPrerequisite
	for _, p := range ps {
		if !p.Met {
			unmet = append(unmet, p)
		}
	}
	switch len(unmet) {
	case 0:
		return ""
	case 1:
		return unmet[0].ReasonPT
	}
	// Several: "Exige A e B." and what the sheet has, taken from the parts.
	var asks, has []string
	for _, p := range unmet {
		ask, status, _ := strings.Cut(strings.TrimSuffix(strings.TrimPrefix(p.ReasonPT, "Exige "), "."), ". ")
		asks = append(asks, ask)
		if status != "" {
			has = append(has, status+".")
		}
	}
	return "Exige " + strings.Join(asks, " e ") + "." + joinSentences(has)
}

func joinSentences(s []string) string {
	if len(s) == 0 {
		return ""
	}
	return " " + strings.Join(s, " ")
}

// invocationPrerequisites are what an Eldritch Invocation asks for (SRD 5.1,
// Warlock, Eldritch Invocations): a warlock level, a cantrip the sheet knows, a
// Pact Boon.
func (cb *choiceBuilder) invocationPrerequisites(key string) []OptionPrerequisite {
	c := cb.c
	p, ok := c.choices.prerequisites[key]
	if !ok {
		return nil
	}
	var out []OptionPrerequisite
	if p.Level > 0 {
		have := 0
		for _, oc := range cb.x.classes {
			if oc.key == classWarlock {
				have = oc.level
			}
		}
		pre := OptionPrerequisite{Kind: PrerequisiteLevel, Level: p.Level, Met: have >= p.Level}
		if !pre.Met {
			pre.ReasonPT = fmt.Sprintf("Exige o nível %d de %s. Você está no %d.", p.Level, c.namePT(classWarlock), have)
		}
		out = append(out, pre)
	}
	if p.Spell != "" {
		pre := OptionPrerequisite{Kind: PrerequisiteSpell, Key: p.Spell, NamePT: c.namePT(p.Spell), Met: cb.knowsCantrip(p.Spell)}
		if !pre.Met {
			pre.ReasonPT = fmt.Sprintf("Exige o truque %s, que você ainda não escolheu.", c.namePT(p.Spell))
		}
		out = append(out, pre)
	}
	if p.Feature != "" {
		boon := cb.pactBoon()
		pre := OptionPrerequisite{Kind: PrerequisiteFeature, Key: p.Feature, NamePT: c.namePT(p.Feature), Met: boon == p.Feature}
		if !pre.Met {
			status := "Você ainda não escolheu a Dádiva do Pacto."
			if boon != "" {
				status = fmt.Sprintf("Você tem o %s.", c.namePT(boon))
			}
			pre.ReasonPT = fmt.Sprintf("Exige o %s. %s", c.namePT(p.Feature), status)
		}
		out = append(out, pre)
	}
	return out
}

// pactBoon is the Pact Boon the sheet has picked, or "".
func (cb *choiceBuilder) pactBoon() string {
	for _, k := range cb.plain {
		if f := cb.c.features[k]; f != nil && f.Parent == "feature:pact-boon" {
			return k
		}
	}
	return ""
}

// knowsCantrip says whether the sheet knows a cantrip: in its cantrip list or
// granted by a pick (the Pact of the Tome's).
func (cb *choiceBuilder) knowsCantrip(key string) bool {
	return slices.Contains(cb.b.Cantrips, key) || cb.grantedCantrip(key)
}
