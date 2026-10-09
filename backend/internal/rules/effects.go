package rules

import (
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/PuraFome/meuRPG/backend/internal/rules/formula"
)

// Effect is one structured effect of a feature, trait or background, as
// written by hand in srd51/effects/*.json (ADR-0008, "efeitos
// estruturados"). The effect types are closed: load refuses any other.
//
// Only some fields apply to each type; load checks that the ones a type
// needs are there. Formulas (Value, When, PreparedMax, Max) are compiled
// once, when the content loads.
type Effect struct {
	// Type is one of the effect types below.
	Type string `json:"type"`

	// modifier: changes a derived number. Target is one of the targets in
	// modifierTargets (or "skill.<skill>", "save.<ability>",
	// "damage.spell.<spell>"); Mode is add, max or set; Value is an Int
	// formula.
	Target string `json:"target,omitempty"`
	Mode   string `json:"mode,omitempty"`
	Value  string `json:"value,omitempty"`

	// When is an optional Bool formula: the effect applies only while it
	// is true (Unarmored Defense: `armor() == "none"`).
	When string `json:"when,omitempty"`

	// Tags make an effect situational ("against:magic",
	// "about:magic-items"). The engine never applies a tagged effect: it
	// shows it as a Hint, and the master decides (ADR-0008).
	Tags []string `json:"tags,omitempty"`

	// proficiency: grants a proficiency. Proficiency is a skill key, "skill:*"
	// (every skill), "save.<ability>", "initiative", or a proficiency key
	// such as "proficiency:heavy-armor". Level is half, full (the default)
	// or expertise.
	Proficiency string `json:"proficiency,omitempty"`
	Level       string `json:"level,omitempty"`

	// roll_mode: Roll is advantage or disadvantage on Targets (such as
	// "save.int" or "skill.stealth"), usually with Tags.
	Roll    string   `json:"roll,omitempty"`
	Targets []string `json:"targets,omitempty"`

	// sense: Sense is darkvision, blindsight, tremorsense or truesight,
	// with RangeFt.
	Sense   string `json:"sense,omitempty"`
	RangeFt int    `json:"range_ft,omitempty"`

	// spellcasting: how a class casts. Progression is full, half or pact;
	// PreparedMax is an Int formula for classes that prepare; Spellbook
	// says the class prepares from a spellbook (the wizard) instead of
	// from its whole list.
	Ability     string `json:"ability,omitempty"`
	Progression string `json:"progression,omitempty"`
	Prepares    bool   `json:"prepares,omitempty"`
	PreparedMax string `json:"prepared_max,omitempty"`
	Spellbook   bool   `json:"spellbook,omitempty"`
	Ritual      bool   `json:"ritual,omitempty"`

	// resource: a use-limited resource. Max is an Int formula; Recharge is
	// short_rest, long_rest, dawn or none. It becomes Derived.Resources; the
	// session counts the uses (RN-02).
	// RechargeIf is an optional Bool formula: while it holds, the resource
	// recharges as RechargeThen instead of Recharge (Bardic Inspiration comes
	// back on a short rest too from bard level 5). Both or neither.
	Resource     string `json:"resource,omitempty"`
	Max          string `json:"max,omitempty"`
	Recharge     string `json:"recharge,omitempty"`
	RechargeIf   string `json:"recharge_if,omitempty"`
	RechargeThen string `json:"recharge_then,omitempty"`

	// Cap is, for a modifier on a score ("score.str"), the highest value the
	// effect may lift the score to (24 for Primal Champion); a score already
	// above it is left as it is. It also raises the ceiling over which the
	// sheet reports a score above the normal maximum.
	Cap int `json:"cap,omitempty"`

	// choice: something the player chooses. Choice is one of choiceKinds,
	// Count how many, From the keys (or a class key, for a spell list). The
	// same Count is, for extra_attack, how many attacks the Attack action
	// makes (2 at level 5); the highest of a character's effects wins, and a
	// character without one makes 1.
	Choice string   `json:"choice,omitempty"`
	Count  int      `json:"count,omitempty"`
	From   []string `json:"from,omitempty"`

	// grant_action: an action for "Sua vez" (MR-014). Economy is action,
	// bonus_action, reaction, free or movement. It becomes Derived.Actions
	// (package combat reads it); the standard actions are in
	// effects/standard_actions.json.
	Economy string `json:"economy,omitempty"`

	// wild_shape: the beasts a druid may turn into (wildshape.go). MaxCR is
	// the highest challenge rating, such as "1/4", and NoFly and NoSwim leave
	// out the beasts with a fly or a swim speed. The druid's features of
	// levels 2, 4 and 8 each carry one; the most permissive one the
	// character has wins.
	MaxCR  string `json:"max_cr,omitempty"`
	NoFly  bool   `json:"no_fly,omitempty"`
	NoSwim bool   `json:"no_swim,omitempty"`

	// replaces: the feature this one takes over from, a lower tier of the same
	// feature (Channel Divinity twice between rests replaces once). The
	// sheet lists only the newest tier; the older one's effects still apply.
	Replaces string `json:"replaces,omitempty"`

	// handler: the name of a Go function for what data cannot say, from
	// handlers.
	Handler string `json:"handler,omitempty"`

	// Spells are spells the effect grants, which then count as on the
	// character's list (Infernal Legacy's thaumaturgy).
	Spells []string `json:"spells,omitempty"`

	// TextPT is an optional short Portuguese sentence for the sheet, in
	// our own words. A note with a text becomes a Hint. In the text,
	// {value} is replaced by Value's result with its sign ("+8") and {n}
	// without it ("13"); a note may carry a Value just for that (a save
	// DC, a number of uses).
	TextPT string `json:"text_pt,omitempty"`

	// Compiled formulas, filled at load.
	value, when, preparedMax, max, rechargeIf *formula.Program
}

// The closed sets of the effect schema.
var (
	effectTypes = []string{
		"modifier", "proficiency", "roll_mode", "sense", "spellcasting",
		"resource", "choice", "grant_action", "extra_attack", "note", "handler", "wild_shape", "beast_spells", "replaces",
	}
	modifierTargets = []string{
		"ac.base", "ac", "hp.max", "speed.walk", "initiative",
		"attack.weapon.melee", "attack.weapon.ranged",
		"damage.weapon.melee", "damage.weapon.ranged",
	}
	modifierModes     = []string{"add", "max", "set"}
	proficiencyLevels = []string{"half", "full", "expertise"}
	rollModes         = []string{"advantage", "disadvantage"}
	senses            = []string{"darkvision", "blindsight", "tremorsense", "truesight"}
	progressions      = []string{"full", "half", "third", "pact"}
	recharges         = []string{"short_rest", "long_rest", "dawn", "none"}
	choiceKinds       = []string{
		"skill", "expertise", "cantrip", "spell", "subclass", "feature",
		"language", "tool", "ability_score_improvement",
	}
	economies = []string{"action", "bonus_action", "reaction", "free", "movement"}
	// handlers are the Go functions an effect may name. Content (and, later,
	// the table's homebrew) can only point at these; it never brings code.
	handlers = []string{
		"monk.martial_arts",      // attacks.go: monk weapons use STR or DEX
		"wizard.arcane_recovery", // Etapa 6: recover slots on a short rest
		"wizard.sculpt_spells",   // Etapa 6: protect allies from evocations
	}
)

// compileEffect checks one effect against the schema and compiles its
// formulas. key is the feature, trait or background it belongs to.
func (c *content) compileEffect(key string, e *Effect) error {
	fail := func(format string, args ...any) error {
		return fmt.Errorf("effect %s (%s): %s", key, e.Type, fmt.Sprintf(format, args...))
	}
	if !slices.Contains(effectTypes, e.Type) {
		return fail("unknown effect type")
	}
	compile := func(src string, kind formula.Kind) (*formula.Program, error) {
		if src == "" {
			return nil, nil
		}
		p, err := c.compileFormula(src, kind)
		if err != nil {
			return nil, fail("formula %q: %v", src, err)
		}
		return p, nil
	}
	var err error
	if e.when, err = compile(e.When, formula.Bool); err != nil {
		return err
	}
	for _, tag := range e.Tags {
		if tag == "" || strings.TrimSpace(tag) != tag {
			return fail("empty tag")
		}
	}

	if e.Type != "wild_shape" && (e.MaxCR != "" || e.NoFly || e.NoSwim) {
		return fail("max_cr, no_fly and no_swim belong to wild_shape")
	}

	if e.Type != "replaces" && e.Replaces != "" {
		return fail("replaces belongs to the replaces effect")
	}

	switch e.Type {
	case "replaces":
		other := *e
		other.Type, other.Replaces = "", ""
		if !reflect.DeepEqual(other, Effect{}) {
			return fail("replaces takes the feature key only")
		}
		if _, ok := c.features[e.Replaces]; !ok {
			return fail("replaces needs the key of a feature")
		}
		if e.Replaces == key {
			return fail("a feature cannot replace itself")
		}
	case "modifier":
		if !c.validModifierTarget(e.Target) {
			return fail("unknown target %q", e.Target)
		}
		if !slices.Contains(modifierModes, e.Mode) {
			return fail("unknown mode %q", e.Mode)
		}
		if e.Value == "" {
			return fail("value is required")
		}
		if _, isScore := strings.CutPrefix(e.Target, "score."); isScore != (e.Cap != 0) || (isScore && (e.Mode != "add" || e.Cap <= MaxNormalScore || e.Cap > MaxScore)) {
			return fail("a score modifier is an add with a cap above %d, and nothing else has a cap", MaxNormalScore)
		}
		if e.value, err = compile(e.Value, formula.Int); err != nil {
			return err
		}
	case "proficiency":
		if !c.validProficiencyTarget(e.Proficiency) {
			return fail("unknown proficiency %q", e.Proficiency)
		}
		if e.Level != "" && !slices.Contains(proficiencyLevels, e.Level) {
			return fail("unknown level %q", e.Level)
		}
	case "roll_mode":
		if !slices.Contains(rollModes, e.Roll) {
			return fail("unknown roll %q", e.Roll)
		}
		if len(e.Targets) == 0 {
			return fail("targets are required")
		}
		for _, t := range e.Targets {
			if !c.validRollTarget(t) {
				return fail("unknown target %q", t)
			}
		}
	case "sense":
		if !slices.Contains(senses, e.Sense) || e.RangeFt <= 0 {
			return fail("sense needs a known sense and a range")
		}
	case "spellcasting":
		if _, ok := abilityIndex[Ability(e.Ability)]; !ok {
			return fail("unknown ability %q", e.Ability)
		}
		if !slices.Contains(progressions, e.Progression) {
			return fail("unknown progression %q", e.Progression)
		}
		if e.Prepares != (e.PreparedMax != "") || (e.Spellbook && !e.Prepares) {
			return fail("prepared_max is required exactly when prepares is true, and a spellbook prepares")
		}
		if e.preparedMax, err = compile(e.PreparedMax, formula.Int); err != nil {
			return err
		}
	case "resource":
		if e.Resource == "" || e.Max == "" || !slices.Contains(recharges, e.Recharge) {
			return fail("resource needs a name, a max and a known recharge")
		}
		if e.max, err = compile(e.Max, formula.Int); err != nil {
			return err
		}
		if (e.RechargeIf == "") != (e.RechargeThen == "") || (e.RechargeThen != "" && !slices.Contains(recharges, e.RechargeThen)) {
			return fail("recharge_if and recharge_then go together, and recharge_then is a known recharge")
		}
		if e.rechargeIf, err = compile(e.RechargeIf, formula.Bool); err != nil {
			return err
		}
	case "choice":
		if !slices.Contains(choiceKinds, e.Choice) || e.Count < 0 {
			return fail("unknown choice %q", e.Choice)
		}
		for _, k := range e.From {
			if !c.exists(k) {
				return fail("unknown key %q in from", k)
			}
		}
	case "grant_action":
		if !slices.Contains(economies, e.Economy) {
			return fail("unknown economy %q", e.Economy)
		}
	case "extra_attack":
		if e.Count < 2 || e.Count > 4 {
			return fail("extra_attack needs a count of 2 to 4 attacks")
		}
	case "wild_shape":
		other := *e
		other.Type, other.MaxCR, other.NoFly, other.NoSwim = "", "", false, false
		if !reflect.DeepEqual(other, Effect{}) {
			return fail("wild_shape takes max_cr, no_fly and no_swim only")
		}
		if _, ok := crEighths(e.MaxCR); !ok {
			return fail("wild_shape needs a max_cr that is a challenge rating, such as \"1/4\"")
		}
	case "beast_spells":
		other := *e
		other.Type = ""
		if !reflect.DeepEqual(other, Effect{}) {
			return fail("beast_spells takes no fields")
		}
	case "handler":
		if !slices.Contains(handlers, e.Handler) {
			return fail("unknown handler %q", e.Handler)
		}
	case "note":
		if e.value, err = compile(e.Value, formula.Int); err != nil {
			return err
		}
		if e.Value != "" && e.TextPT == "" {
			return fail("a value needs a text_pt to show it in")
		}
	}
	for _, s := range e.Spells {
		if _, ok := c.spells[s]; !ok {
			return fail("unknown spell %q", s)
		}
	}
	return nil
}

// programKey identifies a compiled formula: its text and its kind.
type programKey struct {
	src  string
	kind formula.Kind
}

// compileFormula compiles one formula. While a table layer is being added
// (With), programs memoizes by text: a program is immutable, and a table repeats
// the same few formulas ("prof()", "1") in many features, so each is compiled
// once. The memo is dropped when With is done; the SRD load does not use it.
func (c *content) compileFormula(src string, kind formula.Kind) (*formula.Program, error) {
	if c.programs == nil {
		return c.compiler.Compile(src, kind)
	}
	key := programKey{src, kind}
	if p, ok := c.programs[key]; ok {
		return p, nil
	}
	p, err := c.compiler.Compile(src, kind)
	if err == nil {
		c.programs[key] = p
	}
	return p, err
}

func (c *content) validModifierTarget(t string) bool {
	if slices.Contains(modifierTargets, t) {
		return true
	}
	if skill, ok := strings.CutPrefix(t, "skill."); ok {
		_, found := c.skills["skill:"+skill]
		return found
	}
	if ab, ok := strings.CutPrefix(t, "save."); ok {
		_, found := abilityIndex[Ability(ab)] // save.all adds to every saving throw
		return found || ab == "all"
	}
	if ab, ok := strings.CutPrefix(t, "score."); ok {
		_, found := abilityIndex[Ability(ab)]
		return found
	}
	if spell, ok := strings.CutPrefix(t, "damage.spell."); ok {
		_, found := c.spells["spell:"+spell]
		return found
	}
	return false
}

func (c *content) validProficiencyTarget(t string) bool {
	switch {
	case t == "skill:*", t == "initiative":
		return true
	case strings.HasPrefix(t, "skill:"):
		_, ok := c.skills[t]
		return ok
	case strings.HasPrefix(t, "save."):
		_, ok := abilityIndex[Ability(strings.TrimPrefix(t, "save."))]
		return ok
	case strings.HasPrefix(t, "proficiency:"):
		_, ok := c.proficiencies[t]
		return ok
	}
	return false
}

func (c *content) validRollTarget(t string) bool {
	switch t {
	case "attack", "death_save", "initiative", "check.all", "save.all":
		return true
	}
	if ab, ok := strings.CutPrefix(t, "check."); ok {
		_, found := abilityIndex[Ability(ab)]
		return found
	}
	return c.validModifierTarget(t) && (strings.HasPrefix(t, "skill.") || strings.HasPrefix(t, "save."))
}
