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

	// superiority_die: a die a table option adds to a roll, spent from a resource
	// (a Battle Master style maneuver). Resource is the resource it spends, Value the
	// sides of the die (an Int formula), Applies where the die goes (maneuverApplies),
	// Economy what it costs when it costs one (bonus_action or reaction; empty is
	// free) and Ability, for reduce_melee_damage, the modifier added to the die. The
	// text_pt is the rider the master reads.
	// ranged_attack uses Applies too: RangedAttackSpell or RangedAttackWeapon.
	Applies string `json:"applies,omitempty"`

	// ranged_attack (rangedattack.go): what the character's attacks at a distance get,
	// for a table feat (the SRD has none). IgnoreCover takes the AC bonus of half and
	// three-quarters cover away from the targets of those attacks; RangeMultiplier
	// ("spell" only, 2 or 3) multiplies the range of a spell that needs an attack roll;
	// NoLongRangeDisadvantage ("weapon" only) drops the disadvantage of attacking
	// beyond the normal range.
	IgnoreCover             bool `json:"ignore_cover,omitempty"`
	RangeMultiplier         int  `json:"range_multiplier,omitempty"`
	NoLongRangeDisadvantage bool `json:"no_long_range_disadvantage,omitempty"`

	// wild_shape: the beasts a druid may turn into (wildshape.go). MaxCR is
	// the highest challenge rating, such as "1/4", and NoFly and NoSwim leave
	// out the beasts with a fly or a swim speed. The druid's features of
	// levels 2, 4 and 8 each carry one; the most permissive one the
	// character has wins.
	MaxCR  string `json:"max_cr,omitempty"`
	NoFly  bool   `json:"no_fly,omitempty"`
	NoSwim bool   `json:"no_swim,omitempty"`

	// song_of_rest: the die of the bard's Song of Rest (6, 8, 10 or 12 sides),
	// which the character's short rest adds to the hit points the party regains
	// with hit dice (songofrest.go). Each tier of the feature carries one; the
	// largest the character has counts.
	Sides int `json:"sides,omitempty"`

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
		"ability_increase", "superiority_die", "song_of_rest", "ranged_attack",
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
		"language", "tool", "ability_score_improvement", "feat",
	}
	economies = []string{"action", "bonus_action", "reaction", "free", "movement"}
	// maneuverApplies is where a superiority die goes: the damage of a weapon attack
	// after a hit, an attack roll, the Athletics check of a grapple, or a reduction
	// of the damage of a melee attack that hits the character (a reaction).
	maneuverApplies = []string{ManeuverDamage, ManeuverAttack, ManeuverGrapple, ManeuverReduceMelee}
	// handlers are the Go functions an effect may name. Content (and, later,
	// the table's homebrew) can only point at these; it never brings code.
	handlers = []string{
		"monk.martial_arts",      // attacks.go: monk weapons use STR or DEX
		"wizard.arcane_recovery", // Etapa 6: recover slots on a short rest
		"wizard.sculpt_spells",   // Etapa 6: protect allies from evocations
		"monk.open_hand",         // play: the Open Hand technique rider on a Flurry of Blows hit
		"monk.stunning_strike",   // play: the Stunning Strike rider on a melee hit
	}
)

// The limits of the effect schema.
const (
	// minExtraAttacks and maxExtraAttacks bound the attacks an extra_attack makes.
	minExtraAttacks = 2
	maxExtraAttacks = 4
)

// effectCompiler checks and compiles one effect.
type effectCompiler struct {
	c *content
	// key is the feature, trait or background the effect belongs to.
	key string
	e   *Effect
}

func (x effectCompiler) fail(format string, args ...any) error {
	return fmt.Errorf("effect %s (%s): %s", x.key, x.e.Type, fmt.Sprintf(format, args...))
}

// compile compiles a formula of the effect; an empty one is no program.
func (x effectCompiler) compile(src string, kind formula.Kind) (*formula.Program, error) {
	if src == "" {
		return nil, nil
	}
	p, err := x.c.compileFormula(src, kind)
	if err != nil {
		return nil, x.fail("formula %q: %v", src, err)
	}
	return p, nil
}

// takesOnly says the effect sets nothing but the fields strip takes off a copy
// of it (compiled formulas included).
func (x effectCompiler) takesOnly(strip func(other *Effect)) bool {
	other := *x.e
	strip(&other)
	return reflect.DeepEqual(other, Effect{})
}

// effectCheckers check what each effect type needs, after the checks every
// effect goes through.
var effectCheckers = map[string]func(x effectCompiler) error{
	"ranged_attack":    effectCompiler.rangedAttack,
	"replaces":         effectCompiler.replaces,
	"modifier":         effectCompiler.modifier,
	"proficiency":      effectCompiler.proficiency,
	"roll_mode":        effectCompiler.rollMode,
	"sense":            effectCompiler.sense,
	"spellcasting":     effectCompiler.spellcasting,
	"resource":         effectCompiler.resource,
	"choice":           effectCompiler.choice,
	"ability_increase": effectCompiler.abilityIncrease,
	"grant_action":     effectCompiler.grantAction,
	"superiority_die":  effectCompiler.superiorityDie,
	"extra_attack":     effectCompiler.extraAttack,
	"wild_shape":       effectCompiler.wildShape,
	"song_of_rest":     effectCompiler.songOfRest,
	"beast_spells":     effectCompiler.beastSpells,
	"handler":          effectCompiler.handler,
	"note":             effectCompiler.note,
}

// compileEffect checks one effect against the schema and compiles its
// formulas. key is the feature, trait or background it belongs to.
func (c *content) compileEffect(key string, e *Effect) error {
	x := effectCompiler{c: c, key: key, e: e}
	if !slices.Contains(effectTypes, e.Type) {
		return x.fail("unknown effect type")
	}
	var err error
	if e.when, err = x.compile(e.When, formula.Bool); err != nil {
		return err
	}
	for _, tag := range e.Tags {
		if tag == "" || strings.TrimSpace(tag) != tag {
			return x.fail("empty tag")
		}
	}
	if err := x.strayFields(); err != nil {
		return err
	}
	if check, ok := effectCheckers[e.Type]; ok {
		if err := check(x); err != nil {
			return err
		}
	}
	for _, s := range e.Spells {
		if _, ok := c.spells[s]; !ok {
			return x.fail("unknown spell %q", s)
		}
	}
	return nil
}

// strayFields refuses the fields of one type on an effect of another.
func (x effectCompiler) strayFields() error {
	e := x.e
	if e.Type != "wild_shape" && (e.MaxCR != "" || e.NoFly || e.NoSwim) {
		return x.fail("max_cr, no_fly and no_swim belong to wild_shape")
	}
	if e.Type != "song_of_rest" && e.Sides != 0 {
		return x.fail("sides belongs to song_of_rest")
	}
	if e.Type != "replaces" && e.Replaces != "" {
		return x.fail("replaces belongs to the replaces effect")
	}
	if e.Type != "ranged_attack" && (e.IgnoreCover || e.RangeMultiplier != 0 || e.NoLongRangeDisadvantage) {
		return x.fail("ignore_cover, range_multiplier and no_long_range_disadvantage belong to ranged_attack")
	}
	return nil
}

func (x effectCompiler) rangedAttack() error {
	if _, msg := checkRangedAttack(x.e); msg != "" {
		return x.fail("%s", msg)
	}
	return nil
}

func (x effectCompiler) replaces() error {
	if !x.takesOnly(func(o *Effect) { o.Type, o.Replaces = "", "" }) {
		return x.fail("replaces takes the feature key only")
	}
	if _, ok := x.c.features[x.e.Replaces]; !ok {
		return x.fail("replaces needs the key of a feature")
	}
	if x.e.Replaces == x.key {
		return x.fail("a feature cannot replace itself")
	}
	return nil
}

func (x effectCompiler) modifier() error {
	e := x.e
	if !x.c.validModifierTarget(e.Target) {
		return x.fail("unknown target %q", e.Target)
	}
	if !slices.Contains(modifierModes, e.Mode) {
		return x.fail("unknown mode %q", e.Mode)
	}
	if e.Value == "" {
		return x.fail("value is required")
	}
	if !scoreCapFits(e) {
		return x.fail("a score modifier is an add with a cap above %d, and nothing else has a cap", MaxNormalScore)
	}
	var err error
	e.value, err = x.compile(e.Value, formula.Int)
	return err
}

// scoreCapFits: a modifier of a score is an add with a cap between the normal
// maximum and the highest score, and no other modifier has a cap.
func scoreCapFits(e *Effect) bool {
	_, isScore := strings.CutPrefix(e.Target, "score.")
	if isScore != (e.Cap != 0) {
		return false
	}
	return !isScore || (e.Mode == "add" && e.Cap > MaxNormalScore && e.Cap <= MaxScore)
}

func (x effectCompiler) proficiency() error {
	e := x.e
	if !x.c.validProficiencyTarget(e.Proficiency) {
		return x.fail("unknown proficiency %q", e.Proficiency)
	}
	if e.Level != "" && !slices.Contains(proficiencyLevels, e.Level) {
		return x.fail("unknown level %q", e.Level)
	}
	return nil
}

func (x effectCompiler) rollMode() error {
	e := x.e
	if !slices.Contains(rollModes, e.Roll) {
		return x.fail("unknown roll %q", e.Roll)
	}
	if len(e.Targets) == 0 {
		return x.fail("targets are required")
	}
	for _, t := range e.Targets {
		if !x.c.validRollTarget(t) {
			return x.fail("unknown target %q", t)
		}
	}
	return nil
}

func (x effectCompiler) sense() error {
	if !slices.Contains(senses, x.e.Sense) || x.e.RangeFt <= 0 {
		return x.fail("sense needs a known sense and a range")
	}
	return nil
}

func (x effectCompiler) spellcasting() error {
	e := x.e
	if _, ok := abilityIndex[Ability(e.Ability)]; !ok {
		return x.fail("unknown ability %q", e.Ability)
	}
	if !slices.Contains(progressions, e.Progression) {
		return x.fail("unknown progression %q", e.Progression)
	}
	if e.Prepares != (e.PreparedMax != "") || (e.Spellbook && !e.Prepares) {
		return x.fail("prepared_max is required exactly when prepares is true, and a spellbook prepares")
	}
	var err error
	e.preparedMax, err = x.compile(e.PreparedMax, formula.Int)
	return err
}

func (x effectCompiler) resource() error {
	e := x.e
	if e.Resource == "" || e.Max == "" || !slices.Contains(recharges, e.Recharge) {
		return x.fail("resource needs a name, a max and a known recharge")
	}
	var err error
	if e.max, err = x.compile(e.Max, formula.Int); err != nil {
		return err
	}
	if (e.RechargeIf == "") != (e.RechargeThen == "") || (e.RechargeThen != "" && !slices.Contains(recharges, e.RechargeThen)) {
		return x.fail("recharge_if and recharge_then go together, and recharge_then is a known recharge")
	}
	e.rechargeIf, err = x.compile(e.RechargeIf, formula.Bool)
	return err
}

func (x effectCompiler) choice() error {
	e := x.e
	if !slices.Contains(choiceKinds, e.Choice) || e.Count < 0 {
		return x.fail("unknown choice %q", e.Choice)
	}
	for _, k := range e.From {
		if !x.c.exists(k) {
			return x.fail("unknown key %q in from", k)
		}
	}
	return nil
}

// abilityIncrease: the player picks Count different abilities of From and adds
// Value to each; the sheet keeps the result as manual ability bonuses, so Derive
// reads nothing here. Only a feat has it (the table's menu).
func (x effectCompiler) abilityIncrease() error {
	if attr, msg := checkAbilityIncrease(x.e); attr != "" {
		return x.fail("%s", msg)
	}
	return nil
}

func (x effectCompiler) grantAction() error {
	if !slices.Contains(economies, x.e.Economy) {
		return x.fail("unknown economy %q", x.e.Economy)
	}
	return nil
}

func (x effectCompiler) superiorityDie() error {
	e := x.e
	if e.Resource == "" {
		return x.fail("a superiority die names the resource it spends")
	}
	if !slices.Contains(maneuverApplies, e.Applies) {
		return x.fail("unknown applies %q", e.Applies)
	}
	if e.Value == "" {
		return x.fail("value is required: the sides of the die")
	}
	var err error
	if e.value, err = x.compile(e.Value, formula.Int); err != nil {
		return err
	}
	return x.maneuverRider()
}

// maneuverRider checks the ability and the economy a superiority die takes,
// which depend on where the die goes.
func (x effectCompiler) maneuverRider() error {
	e := x.e
	switch e.Applies {
	case ManeuverReduceMelee:
		if _, ok := abilityIndex[Ability(e.Ability)]; !ok {
			return x.fail("reduce_melee_damage needs the ability whose modifier is added")
		}
		if e.Economy != EconomyReaction {
			return x.fail("reduce_melee_damage costs a reaction")
		}
	case ManeuverGrapple:
		if e.Economy != "" && e.Economy != EconomyBonusAction {
			return x.fail("a grapple maneuver costs a bonus action or nothing")
		}
	default:
		if e.Ability != "" || e.Economy != "" {
			return x.fail("%s takes no ability and no economy", e.Applies)
		}
	}
	return nil
}

func (x effectCompiler) extraAttack() error {
	if x.e.Count < minExtraAttacks || x.e.Count > maxExtraAttacks {
		return x.fail("extra_attack needs a count of 2 to 4 attacks")
	}
	return nil
}

func (x effectCompiler) wildShape() error {
	if !x.takesOnly(func(o *Effect) { o.Type, o.MaxCR, o.NoFly, o.NoSwim = "", "", false, false }) {
		return x.fail("wild_shape takes max_cr, no_fly and no_swim only")
	}
	if _, ok := crEighths(x.e.MaxCR); !ok {
		return x.fail("wild_shape needs a max_cr that is a challenge rating, such as \"1/4\"")
	}
	return nil
}

func (x effectCompiler) songOfRest() error {
	if !x.takesOnly(func(o *Effect) { o.Type, o.Sides = "", 0 }) {
		return x.fail("song_of_rest takes sides only")
	}
	if !slices.Contains(songOfRestDice, x.e.Sides) {
		return x.fail("song_of_rest needs sides of 6, 8, 10 or 12")
	}
	return nil
}

func (x effectCompiler) beastSpells() error {
	if !x.takesOnly(func(o *Effect) { o.Type = "" }) {
		return x.fail("beast_spells takes no fields")
	}
	return nil
}

func (x effectCompiler) handler() error {
	if !slices.Contains(handlers, x.e.Handler) {
		return x.fail("unknown handler %q", x.e.Handler)
	}
	return nil
}

func (x effectCompiler) note() error {
	e := x.e
	var err error
	if e.value, err = x.compile(e.Value, formula.Int); err != nil {
		return err
	}
	if e.Value != "" && e.TextPT == "" {
		return x.fail("a value needs a text_pt to show it in")
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
