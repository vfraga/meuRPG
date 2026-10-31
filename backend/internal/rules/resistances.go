package rules

import (
	"fmt"
	"io/fs"
	"slices"
	"strings"
)

// The Resistance* constants say when a resistance holds. "" is always (a race's);
// "rage" is while the character is raging (the barbarian's).
const (
	ResistanceAlways    = ""
	ResistanceWhileRage = "rage"
)

// The Resistance kinds: resistance halves the damage, immunity takes it to 0 and
// vulnerability doubles it (SRD 5.1, "Damage Resistance and Vulnerability"). The
// empty kind of an effect is resistance.
const (
	ResistanceKindResist     = "resist"
	ResistanceKindImmune     = "immune"
	ResistanceKindVulnerable = "vulnerable"
)

// ResistanceKinds are the kinds a resistance effect may have; "" means resist.
var ResistanceKinds = []string{"", ResistanceKindResist, ResistanceKindImmune, ResistanceKindVulnerable}

// The words a resistance effect's damage_types has besides the damage type keys:
// DamageTypeAll is every damage type and DamageTypeNonmagicalBPS the bludgeoning, piercing and slashing
// damage of attacks that are not magical (the stat blocks' "from nonmagical attacks").
// An attack the sheet or the stat block does not mark as magical counts as nonmagical.
const (
	DamageTypeAll           = "all"
	DamageTypeNonmagicalBPS = "nonmagical_bps"
)

// nonmagicalBPS are the damage types of DamageTypeNonmagicalBPS.
var nonmagicalBPS = []string{"damage-type:bludgeoning", "damage-type:piercing", "damage-type:slashing"}

// ResistanceActive says a resistance that holds while is in force: always when it has no
// condition, and while the character rages when its condition is rage. It is the one
// place that says when a resistance holds.
func ResistanceActive(while string, raging bool) bool {
	return while == ResistanceAlways || (while == ResistanceWhileRage && raging)
}

// Resistance is a damage resistance, immunity or vulnerability that a feature or a
// trait gives (SRD 5.1, "Damage Resistance and Vulnerability"): the damage types it
// covers, and when.
type Resistance struct {
	// Source is the feature or trait key, such as "trait:hellish-resistance", and
	// NamePT its Portuguese name.
	Source, NamePT string
	// DamageTypes are the damage type keys it covers, "damage-type:fire".
	DamageTypes []string
	// While is ResistanceAlways or ResistanceWhileRage.
	While string
	// Kind is a ResistanceKind* constant; empty (the data file's) is resist.
	Kind string `json:",omitempty"`
	// Nonmagical limits it to the damage of attacks that are not magical.
	Nonmagical bool `json:",omitempty"`
}

// damageResistanceFile is effects/damage_resistances.json.
type damageResistanceFile struct {
	Source      string `json:"source"`
	Resistances map[string]struct {
		DamageTypes []string `json:"damage_types"`
		While       string   `json:"while"`
	} `json:"resistances"`
}

// loadDamageResistances reads and checks effects/damage_resistances.json: every
// key is a feature or a trait of the content, every damage type exists, "while"
// is closed, and the file says where its rules come from.
func (c *content) loadDamageResistances(fsys fs.FS) error {
	var f damageResistanceFile
	if err := readJSON(fsys, "effects/damage_resistances.json", &f); err != nil {
		return err
	}
	if strings.TrimSpace(f.Source) == "" {
		return fmt.Errorf("effects/damage_resistances.json: source must say where the rules come from")
	}
	c.resistances = map[string]Resistance{}
	for _, key := range sortedKeys(f.Resistances) {
		in := f.Resistances[key]
		if msg := c.checkResistance(key, in.DamageTypes, in.While); msg != "" {
			return fmt.Errorf("effects/damage_resistances.json: %s: %s", key, msg)
		}
		c.resistances[key] = Resistance{Source: key, NamePT: c.namePT(key), DamageTypes: slices.Clone(in.DamageTypes), While: in.While}
	}
	return nil
}

// checkResistance says what is wrong with a resistance of the file, or "":
// its key, its damage types, then when it holds.
func (c *content) checkResistance(key string, damageTypes []string, while string) string {
	if !strings.HasPrefix(key, featureKeyPrefix) && !strings.HasPrefix(key, "trait:") || !c.exists(key) {
		return "not a feature or a trait of the content"
	}
	if len(damageTypes) == 0 {
		return "no damage type"
	}
	for _, t := range damageTypes {
		if !strings.HasPrefix(t, "damage-type:") || !c.exists(t) {
			return fmt.Sprintf("%q is not a damage type", t)
		}
	}
	if while != ResistanceAlways && while != ResistanceWhileRage {
		return fmt.Sprintf("while must be empty or %q", ResistanceWhileRage)
	}
	return ""
}

// Resistances lists the damage resistances, immunities and vulnerabilities the
// character's features and traits give, in the order of its features: the ones of
// effects/damage_resistances.json and the resistance effects of a feature (a
// table's). A resistance that holds only while raging is listed too: the play module
// applies it while the rage lasts, and two sources of one kind for one damage type
// count once there. An effect that names plain damage types and nonmagical_bps is
// listed as two entries, the second one limited to nonmagical attacks.
func (c *Content) Resistances(d Derived) []Resistance {
	var out []Resistance
	for _, f := range d.Features {
		if r, ok := c.c.resistances[f.Key]; ok {
			out = append(out, r)
		}
		for _, e := range c.c.effects[f.Key] {
			if e.Type == "resistance" {
				out = append(out, c.c.gatedResistances(d, f.Key, e)...)
			}
		}
	}
	return out
}

// gatedResistances are the entries of a resistance effect that holds while a state is on
// (gate.go): none while the state is off, and the effect's entries, now unconditional,
// while it is on. Rage is for play to judge, so that gate stays on the entries.
func (c *content) gatedResistances(d Derived, key string, e *Effect) []Resistance {
	rs := c.resistancesOf(key, e)
	if e.While == ResistanceAlways || e.While == ResistanceWhileRage {
		return rs
	}
	if !d.StateOn(e.gateKey) {
		return nil
	}
	for i := range rs {
		rs[i].While = ResistanceAlways
	}
	return rs
}

// resistancesOf are the entries of one resistance effect of a feature.
func (c *content) resistancesOf(key string, e *Effect) []Resistance {
	base := Resistance{Source: key, NamePT: c.namePT(key), While: e.While, Kind: kindOf(e)}
	if e.Except != "" {
		base.DamageTypes = c.damageTypesBut(e.Except)
		return []Resistance{base}
	}
	plain, magic := base, base
	for _, t := range e.DamageTypes {
		switch t {
		case DamageTypeAll:
			plain.DamageTypes = c.damageTypesBut("")
		case DamageTypeNonmagicalBPS:
			magic.DamageTypes, magic.Nonmagical = slices.Clone(nonmagicalBPS), true
		default:
			plain.DamageTypes = append(plain.DamageTypes, t)
		}
	}
	var out []Resistance
	for _, r := range []Resistance{plain, magic} {
		if len(r.DamageTypes) > 0 {
			out = append(out, r)
		}
	}
	return out
}

// kindOf is the kind of a resistance effect, resist when it names none.
func kindOf(e *Effect) string {
	if e.Kind == "" {
		return ResistanceKindResist
	}
	return e.Kind
}

// checkDamageTypes says what is wrong with the damage types a resistance effect covers,
// or "": the one it leaves out, or the list it names (each a damage type of the
// content or one of the closed words, listed once, "all" alone).
func (c *content) checkDamageTypes(e *Effect) string {
	if e.Except != "" {
		if !c.isLiveDamageType(e.Except) {
			return fmt.Sprintf("except %q is not a damage type", e.Except)
		}
		return ""
	}
	if len(e.DamageTypes) == 0 {
		return "damage_types is empty: name the damage types, or the one except leaves out"
	}
	for i, t := range e.DamageTypes {
		switch {
		case t != DamageTypeAll && t != DamageTypeNonmagicalBPS && !c.isLiveDamageType(t):
			return fmt.Sprintf("damage_types: %q is not a damage type", t)
		case slices.Contains(e.DamageTypes[:i], t):
			return fmt.Sprintf("damage_types: %q is listed twice", t)
		case t == DamageTypeAll && len(e.DamageTypes) > 1:
			return `damage_types: "all" cannot be mixed with other damage types`
		}
	}
	return ""
}

// isCondition says the key is one of the conditions of the content.
func (c *content) isCondition(k string) bool {
	if c.combatEffects != nil {
		_, ok := c.combatEffects.conditions[k]
		return ok
	}
	return strings.HasPrefix(k, "condition:") && c.exists(k)
}

// StatBlockSource is the Source of the condition immunities a creature's stat block lists.
const StatBlockSource = "stat-block"

// ConditionImmunity is a condition the character takes none of, from a feature or a
// trait (a table's condition_immunity effect).
type ConditionImmunity struct {
	// Source is the feature or trait key, NamePT its Portuguese name.
	Source, NamePT string
	// Conditions are the condition keys, "condition:poisoned".
	Conditions []string
}

// ConditionImmunities lists the conditions the character's features and traits make it
// immune to, in the order of its features.
func (c *Content) ConditionImmunities(d Derived) []ConditionImmunity {
	var out []ConditionImmunity
	for _, f := range d.Features {
		for _, e := range c.c.effects[f.Key] {
			if e.Type == "condition_immunity" {
				out = append(out, ConditionImmunity{Source: f.Key, NamePT: c.c.namePT(f.Key), Conditions: slices.Clone(e.Conditions)})
			}
		}
	}
	if len(d.StatBlockConditionImmunities) > 0 {
		out = append(out, ConditionImmunity{Source: StatBlockSource, NamePT: "ficha da criatura", Conditions: slices.Clone(d.StatBlockConditionImmunities)})
	}
	return out
}

// damageTypesBut is every damage type of the content but the one named, sorted.
func (c *content) damageTypesBut(except string) []string {
	var out []string
	for _, k := range sortedKeys(c.named) {
		if k != except && c.isLiveDamageType(k) {
			out = append(out, k)
		}
	}
	return out
}

// resistanceHint is the sheet's note of a resistance effect: the sheet lists what it
// covers (and the rage it holds in), and play applies it.
func (x *deriver) resistanceHint(a activeEffect) {
	e := a.effect
	if e.TextPT == "" {
		e = &Effect{TextPT: x.c.resistanceText(a.owner, e)}
		a = activeEffect{owner: a.owner, effect: e}
	}
	x.hint(a, nil, "note", 0)
}

// resistanceText is the sentence of a resistance effect of the entry owner, in Portuguese.
func (c *content) resistanceText(owner string, e *Effect) string {
	what := map[string]string{
		ResistanceKindResist: "Resistência a", ResistanceKindImmune: "Imunidade a", ResistanceKindVulnerable: "Vulnerabilidade a",
	}[kindOf(e)]
	var types string
	switch {
	case e.Except != "":
		types = "todos os tipos de dano, menos " + c.namePT(e.Except)
	default:
		var parts []string
		for _, t := range e.DamageTypes {
			switch t {
			case DamageTypeAll:
				parts = append(parts, "todos os tipos de dano")
			case DamageTypeNonmagicalBPS:
				parts = append(parts, "contundente, perfurante e cortante de ataques não mágicos")
			default:
				parts = append(parts, c.namePT(t))
			}
		}
		types = strings.Join(parts, ", ")
	}
	text := what + " " + types
	switch e.While {
	case ResistanceAlways:
	case ResistanceWhileRage:
		text += ", enquanto estiver em fúria"
	default:
		text += ", enquanto " + c.namePT(owner) + " estiver ativo"
	}
	return text + "."
}

// HasFeature says whether the character has a feature or a trait.
func HasFeature(d Derived, key string) bool {
	return slices.ContainsFunc(d.Features, func(f Feature) bool { return f.Key == key })
}

// LevelIn is the character's level in a class ("class:barbarian"), 0 without it.
func LevelIn(d Derived, classKey string) int {
	for _, cl := range d.Classes {
		if cl.ClassKey == classKey {
			return cl.Level
		}
	}
	return 0
}
