package rules

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/PuraFome/meuRPG/backend/internal/rules/srd51"
)

// overlayEffectTypes is the closed menu of what the table's features may do
// (ADR-0018, section 4): the effect types of the SRD minus the ones that need
// code or the engine's own tables. There is no "handler" (the table never runs
// code, not even a handler that exists), no "spellcasting" (the engine writes it
// from the class's casting) and no "wild_shape". "ranged_attack" is the table's own:
// the SRD has no effect of that type (a feat from another book brings it).
var overlayEffectTypes = []string{
	"modifier", "proficiency", "resource", "sense", "roll_mode", "grant_action",
	"superiority_die", "extra_attack", "ranged_attack", "choice", "ability_increase", "note",
}

// overlayChoiceKinds are the choices a table feature may offer. The subclass and
// the Ability Score Improvement are the class table's, not a feature's.
var overlayChoiceKinds = []string{
	"skill", "expertise", "cantrip", "spell", "language", "tool", "feature", "feat",
}

// featureKind is where a feature belongs: a class, a subclass, a race (trait)
// or a background.
type featureKind struct {
	// prefix is the key prefix: "feature:", "trait:" or "background-feature:".
	prefix string
	// owner is the entry's key (a class, subclass, race, subrace or background),
	// and class and subclass set the Feature's own fields.
	owner, class, subclass string
	level                  int
}

// addFeature registers one table feature (or trait) in the clone: its name, its
// text and the effects, which are compiled later, once every entry exists. A
// feature that offers a choice of SRD options (a "choice" effect of kind
// "feature") also becomes what the level-up and Derive read as the options'
// offerer.
func (b *overlayBuilder) addFeature(f *TableFeature, k featureKind, path string) error {
	if err := b.checkFeatureHead(f, k.prefix, path); err != nil {
		return err
	}
	b.know(f.Key, k)
	effects, err := b.featureOptions(f, k, path)
	if err != nil {
		return err
	}
	b.n.namesEN[f.Key] = f.NamePT
	b.n.namesPT[f.Key] = f.NamePT
	switch k.prefix {
	case "feature:":
		if err := b.writeFeature(f, k, effects, path); err != nil {
			return err
		}
	case "trait:":
		b.writeTrait(f, k)
	}
	b.declareResources(k, effects)
	if len(effects) > 0 {
		b.pending = append(b.pending, pendingEffects{owner: f.Key, entry: k.owner, effects: effects, path: path, strict: b.strict[k.owner]})
	}
	return nil
}

// checkFeatureHead checks a feature's key (and claims it), its name and its text.
func (b *overlayBuilder) checkFeatureHead(f *TableFeature, prefix, path string) error {
	if err := checkTableKey(prefix, f.Key); err != nil {
		return locate(err, path)
	}
	if err := b.claim(f.Key); err != nil {
		return locate(err, path)
	}
	if err := checkEntryName(f.Key, f.NamePT); err != nil {
		return locate(err, path)
	}
	if err := checkText(f.Key, f.DescPT); err != nil {
		return locate(err, path+".desc_pt")
	}
	return nil
}

// featureOptions registers a feature's option list, when it has one, and returns
// the feature's effects with its choice of features offering those options (the
// caller's effects themselves when there is no list).
func (b *overlayBuilder) featureOptions(f *TableFeature, k featureKind, path string) ([]Effect, error) {
	if len(f.Options) == 0 {
		return f.Effects, nil
	}
	if k.prefix != "feature:" {
		return nil, ovErr(f.Key, "only a class or subclass feature has an option list").at(path+".options", ReasonEffect)
	}
	if len(f.Options) > MaxFeatureOptions {
		return nil, ovErr(f.Key, "%d options; the limit is %d per feature", len(f.Options), MaxFeatureOptions).at(path+".options", ReasonLimit)
	}
	keys, err := b.addOptions(f, k, path)
	if err != nil {
		return nil, err
	}
	effects := slices.Clone(f.Effects)
	if !offerOptions(effects, keys) {
		return nil, ovErr(f.Key, "a feature with options needs a choice of features to offer them").at(path+".effects", ReasonEffect)
	}
	return effects, nil
}

// offerOptions makes a choice of features with no `from` offer the feature's own
// options, and says whether the effects have a choice of features at all.
func offerOptions(effects []Effect, keys []string) bool {
	offers := false
	for i := range effects {
		e := &effects[i]
		if !isFeatureChoice(e) {
			continue
		}
		if len(e.From) == 0 {
			e.From = slices.Clone(keys)
		}
		offers = true
	}
	return offers
}

// isFeatureChoice says whether the effect is a choice of features (options).
func isFeatureChoice(e *Effect) bool { return e.Type == "choice" && e.Choice == "feature" }

// writeFeature puts a class or subclass feature in the clone; a choice of
// features makes it the offerer of the options it lists.
func (b *overlayBuilder) writeFeature(f *TableFeature, k featureKind, effects []Effect, path string) error {
	n := b.n
	feat := &srd51.Feature{Key: f.Key, Name: f.NamePT, Class: k.class, Subclass: k.subclass, Level: k.level, Desc: slices.Clone(f.DescPT)}
	for i := range effects {
		e := &effects[i]
		if !isFeatureChoice(e) {
			continue
		}
		if len(feat.Options) > 0 {
			return ovErr(f.Key, "a feature offers one choice of options").at(path+".effects", ReasonEffect)
		}
		feat.Options, feat.OptionsChoose = slices.Clone(e.From), e.Count
		for _, o := range e.From {
			n.offeredBy[o] = append(n.offeredBy[o], f.Key)
		}
	}
	n.features[f.Key] = feat
	return nil
}

// writeTrait puts a race's or a subrace's trait in the clone.
func (b *overlayBuilder) writeTrait(f *TableFeature, k featureKind) {
	t := &srd51.Trait{Key: f.Key, Name: f.NamePT, Desc: slices.Clone(f.DescPT)}
	if strings.HasPrefix(k.owner, "subrace:") {
		t.Subraces = []string{k.owner}
	} else {
		t.Races = []string{k.owner}
	}
	b.n.traits[f.Key] = t
}

// know records which entry (and class) a feature or option belongs to, for the
// checks that need it once every entry exists.
func (b *overlayBuilder) know(key string, k featureKind) {
	if b.featureEntry == nil {
		b.featureEntry, b.featureClass, b.optionEntry = map[string]string{}, map[string]string{}, map[string]string{}
		b.resources = map[string]map[string]bool{}
	}
	b.featureEntry[key], b.featureClass[key] = k.owner, k.class
}

// declareResources notes the names the resource effects of a feature declare, in
// its entry: a grant_action may spend one of them.
func (b *overlayBuilder) declareResources(k featureKind, effects []Effect) {
	for i := range effects {
		if effects[i].Type != "resource" || effects[i].Resource == "" {
			continue
		}
		if b.resources[k.owner] == nil {
			b.resources[k.owner] = map[string]bool{}
		}
		b.resources[k.owner][effects[i].Resource] = true
	}
}

// addOptions registers the option list of a feature as features of their own,
// each with the feature as its parent, and returns their keys. An option is in no
// level row: the choice that offers it is the only way to have it.
func (b *overlayBuilder) addOptions(f *TableFeature, k featureKind, path string) ([]string, error) {
	n := b.n
	names := map[string]bool{}
	keys := make([]string, 0, len(f.Options))
	for i := range f.Options {
		o := &f.Options[i]
		at := fmt.Sprintf("%s.options[%d]", path, i)
		if len(o.Options) > 0 {
			return nil, ovErr(o.Key, "an option does not have options of its own").at(at+".options", ReasonValue)
		}
		if err := checkTableKey("feature:", o.Key); err != nil {
			return nil, locate(err, at)
		}
		if err := b.claim(o.Key); err != nil {
			return nil, locate(err, at)
		}
		if err := checkEntryName(o.Key, o.NamePT); err != nil {
			return nil, locate(err, at)
		}
		if names[o.NamePT] {
			return nil, ovErr(o.Key, "two options of %q are named %q", f.NamePT, o.NamePT).at(at+".name_pt", ReasonName)
		}
		names[o.NamePT] = true
		if err := checkText(o.Key, o.DescPT); err != nil {
			return nil, locate(err, at+".desc_pt")
		}
		b.know(o.Key, k)
		b.optionEntry[o.Key] = k.owner
		n.namesEN[o.Key], n.namesPT[o.Key] = o.NamePT, o.NamePT
		n.features[o.Key] = &srd51.Feature{Key: o.Key, Name: o.NamePT, Class: k.class, Subclass: k.subclass, Level: k.level, Desc: slices.Clone(o.DescPT), Parent: f.Key}
		b.declareResources(k, o.Effects)
		if len(o.Effects) > 0 {
			b.pending = append(b.pending, pendingEffects{owner: o.Key, entry: k.owner, effects: o.Effects, path: at, strict: b.strict[k.owner]})
		}
		keys = append(keys, o.Key)
	}
	return keys, nil
}

// effectSite is where an effect is, for the errors it gets: the owner's key and
// the effect's path in the Overlay.
type effectSite struct{ owner, path string }

// fail is the refusal of the effect's attribute attr.
func (s effectSite) fail(attr, reason, format string, args ...any) error {
	return ovErr(s.owner, format, args...).at(s.path+attr, reason)
}

// checkEffect is the closed menu: what is not on it is refused, naming the key.
// The error carries the effect's path.
func (b *overlayBuilder) checkEffect(owner, path string, e *Effect, strict bool) error {
	site := effectSite{owner: owner, path: path}
	if err := checkEffectType(site, e); err != nil {
		return err
	}
	if name := unusedField(e); strict && name != "" {
		return site.fail("."+name, ReasonValue, "the field %s is not used by a %s effect", name, e.Type)
	}
	if len(e.Spells) > 0 {
		if err := b.checkGrantedSpells(site, e); err != nil {
			return err
		}
	}
	switch e.Type {
	case "choice":
		return b.checkChoiceEffect(site, e)
	case "ability_increase":
		if attr, msg := checkAbilityIncrease(e); attr != "" {
			return site.fail(attr, ReasonValue, "%s", msg)
		}
	case "grant_action":
		if e.Resource != "" && !b.resourceDefined(owner, e.Resource) {
			return site.fail(".resource", ReasonReference, "the action spends %q, which no resource effect of this entry or its class defines", e.Resource)
		}
	case "superiority_die":
		return b.checkSuperiorityDie(site, e)
	case "resource":
		return b.checkResourceEffect(site, e)
	}
	return nil
}

// checkEffectType refuses a type that is not on the menu.
func checkEffectType(site effectSite, e *Effect) error {
	if e.Type == "handler" {
		return site.fail(".type", ReasonEffect, "a handler effect is not allowed: the table's content never runs code")
	}
	if !slices.Contains(overlayEffectTypes, e.Type) {
		return site.fail(".type", ReasonEffect, "effect type %q is not on the table's menu", e.Type)
	}
	if e.Type == "ability_increase" && !strings.HasPrefix(site.owner, "feat:") {
		return site.fail(".type", ReasonEffect, "only a feat raises abilities: an ability_increase is not on the menu of a feature")
	}
	return nil
}

// checkGrantedSpells checks a spell the effect grants (a race that knows a
// cantrip, a once-a-day spell): a note with the spells, as the SRD's Infernal
// Legacy. The uses per rest are a resource effect of the same feature.
func (b *overlayBuilder) checkGrantedSpells(site effectSite, e *Effect) error {
	if e.Type != "note" {
		return site.fail(".spells", ReasonEffect, "only a note grants spells in the table's content")
	}
	for _, sp := range e.Spells {
		if !b.isSpell(sp) {
			return site.fail(".spells", ReasonReference, "the granted spell %q does not exist", sp)
		}
	}
	return nil
}

// resourceDefined says the owner can spend the resource: it is an SRD resource,
// or a resource effect of the owner's entry or of its class defines it.
func (b *overlayBuilder) resourceDefined(owner, name string) bool {
	_, srd := b.base.namesPT["resource:"+name]
	return validResourceName(name) && (srd || b.resources[b.featureEntry[owner]][name] || b.resources[b.featureClass[owner]][name])
}

// checkChoiceEffect checks a choice effect: what it asks and what it offers.
func (b *overlayBuilder) checkChoiceEffect(site effectSite, e *Effect) error {
	if err := checkChoiceCount(site, e); err != nil {
		return err
	}
	if err := checkChoiceList(site, e); err != nil {
		return err
	}
	if b.optionEntry[site.owner] != "" {
		return site.fail(".type", ReasonEffect, "an option does not ask for another choice")
	}
	for _, k := range e.From {
		if err := b.checkChoiceKey(site, e, k); err != nil {
			return err
		}
	}
	return nil
}

// checkChoiceCount checks the kind of a choice and how many it asks for.
func checkChoiceCount(site effectSite, e *Effect) error {
	if !slices.Contains(overlayChoiceKinds, e.Choice) {
		return site.fail(".choice", ReasonEffect, "choice %q is not on the table's menu", e.Choice)
	}
	if e.Count < 1 {
		return site.fail(".count", ReasonValue, "a choice needs a count of at least 1")
	}
	if e.Count > MaxChoiceCount {
		return site.fail(".count", ReasonValue, "a choice has a count of at most %d", MaxChoiceCount)
	}
	return nil
}

// checkChoiceList checks the list of options or feats a choice offers against
// the number it asks for.
func checkChoiceList(site effectSite, e *Effect) error {
	if e.Choice == "feature" && e.Count > len(e.From) {
		return site.fail(".count", ReasonValue, "a choice of features cannot ask for more than the %d options it lists", len(e.From))
	}
	if e.Choice == "feature" && len(e.From) == 0 {
		return site.fail(".from", ReasonValue, "a choice of features needs the options to choose from")
	}
	if e.Choice == "feat" && len(e.From) > 0 && e.Count > len(e.From) {
		return site.fail(".count", ReasonValue, "a choice of feats cannot ask for more than the %d feats it lists", len(e.From))
	}
	if e.Choice == "feature" && len(slices.Compact(slices.Sorted(slices.Values(e.From)))) != len(e.From) {
		return site.fail(".from", ReasonValue, "the options of a choice are listed once each")
	}
	return nil
}

// checkChoiceKey checks one key a choice offers.
func (b *overlayBuilder) checkChoiceKey(site effectSite, e *Effect, k string) error {
	if e.Choice == "feature" && b.optionEntry[k] != "" {
		// An option of a feature list: the same entry's, never another's.
		if b.optionEntry[k] != b.featureEntry[site.owner] {
			return site.fail(".from", ReasonReference, "choice option %q belongs to another entry's list", k)
		}
		return nil
	}
	if e.Choice == "feat" {
		// The feats offered are the SRD's or the table's own, and only feats.
		if !strings.HasPrefix(k, "feat:") || (!b.entries[k] && !b.base.exists(k)) {
			return site.fail(".from", ReasonReference, "choice feat %q does not exist", k)
		}
		return nil
	}
	// The options come from an SRD set: a key the SRD has, never one of the
	// table's (and never one that does not exist).
	if isTableKey(k) || !b.base.exists(k) {
		return site.fail(".from", ReasonReference, "choice option %q is not in the SRD", k)
	}
	if e.Choice == "feature" && !isOption(b.base, k) {
		return site.fail(".from", ReasonReference, "%q is not an option of an SRD feature", k)
	}
	return nil
}

// checkSuperiorityDie checks a superiority die: the resource it spends and what
// it is added to.
func (b *overlayBuilder) checkSuperiorityDie(site effectSite, e *Effect) error {
	if !b.resourceDefined(site.owner, e.Resource) {
		return site.fail(".resource", ReasonReference, "the die spends %q, which no resource effect of this entry or its class defines", e.Resource)
	}
	if !slices.Contains(maneuverApplies, e.Applies) {
		return site.fail(".applies", ReasonValue, "applies %q is not on the table's menu", e.Applies)
	}
	return checkManeuverCost(site, e)
}

// checkManeuverCost checks the action a maneuver costs and the ability it adds,
// which only a reduction of a melee attack's damage and a grapple have.
func checkManeuverCost(site effectSite, e *Effect) error {
	reduction := e.Applies == ManeuverReduceMelee
	if reduction && e.Ability == "" {
		return site.fail(".ability", ReasonValue, "a reduction names the ability whose modifier is added")
	}
	if reduction && e.Economy != EconomyReaction {
		return site.fail(".economy", ReasonValue, "a reduction of a melee attack's damage costs a reaction")
	}
	if e.Applies == ManeuverGrapple && e.Economy != "" && e.Economy != EconomyBonusAction {
		return site.fail(".economy", ReasonValue, "a grapple maneuver costs a bonus action or nothing")
	}
	if !reduction && e.Applies != ManeuverGrapple && e.Economy != "" {
		return site.fail(".economy", ReasonValue, "a die added to an attack or to damage costs no action")
	}
	if !reduction && e.Ability != "" {
		return site.fail(".ability", ReasonValue, "only a reduction adds an ability modifier")
	}
	return nil
}

// checkResourceEffect checks the name of a resource an effect declares.
func (b *overlayBuilder) checkResourceEffect(site effectSite, e *Effect) error {
	if !validResourceName(e.Resource) {
		return site.fail(".resource", ReasonValue, "a resource name has 1 to 40 characters of a-z, 0-9 and _")
	}
	if _, taken := b.base.namesPT["resource:"+e.Resource]; taken {
		return site.fail(".resource", ReasonValue, "resource %q is an SRD resource; use another name", e.Resource)
	}
	return nil
}

// ownFields are the fields only some effect types read. Every other field of the
// menu (when, tags, text_pt) any type may have; the fields of the engine's own
// effects (the spellcasting, the wild shape, the handler) never come from the
// table.
var ownFields = map[string][]string{
	"modifier":     {"target", "mode", "value"},
	"proficiency":  {"proficiency", "level"},
	"roll_mode":    {"roll", "targets"},
	"sense":        {"sense", "range_ft"},
	"resource":     {"resource", "max", "recharge"},
	"choice":       {"choice", "count", "from"},
	"extra_attack": {"count"},
	"grant_action": {"economy", "resource"},
	"note":         {"value", "spells"},
	// A superiority die is a feature option's (or a feature's) die from a resource.
	"superiority_die": {"value", "resource", "economy", "applies", "ability"},
	// An ability_increase is a feat's alone (checkEffect).
	"ability_increase": {"value", "count", "from"},
	// A ranged_attack's switches; which of them its applies allows is checkRangedAttack's.
	"ranged_attack": {"applies", "ignore_cover", "range_multiplier", "no_long_range_disadvantage"},
}

// effectField is a field of an Effect a type may not read: its name in the
// proto message, whether it is set, and how to clear it.
type effectField struct {
	name  string
	set   func(e *Effect) bool
	clear func(e *Effect)
}

// effectFields are the type-specific fields of TableEffect in the order of the
// proto message, then the switches of ranged_attack, then the fields of the
// engine's own effects (which no type of the menu reads).
var effectFields = []effectField{
	{"target", func(e *Effect) bool { return e.Target != "" }, func(e *Effect) { e.Target = "" }},
	{"mode", func(e *Effect) bool { return e.Mode != "" }, func(e *Effect) { e.Mode = "" }},
	{"value", func(e *Effect) bool { return e.Value != "" }, func(e *Effect) { e.Value = "" }},
	{"proficiency", func(e *Effect) bool { return e.Proficiency != "" }, func(e *Effect) { e.Proficiency = "" }},
	{"level", func(e *Effect) bool { return e.Level != "" }, func(e *Effect) { e.Level = "" }},
	{"roll", func(e *Effect) bool { return e.Roll != "" }, func(e *Effect) { e.Roll = "" }},
	{"targets", func(e *Effect) bool { return len(e.Targets) > 0 }, func(e *Effect) { e.Targets = nil }},
	{"sense", func(e *Effect) bool { return e.Sense != "" }, func(e *Effect) { e.Sense = "" }},
	{"range_ft", func(e *Effect) bool { return e.RangeFt != 0 }, func(e *Effect) { e.RangeFt = 0 }},
	{"resource", func(e *Effect) bool { return e.Resource != "" }, func(e *Effect) { e.Resource = "" }},
	{"max", func(e *Effect) bool { return e.Max != "" }, func(e *Effect) { e.Max = "" }},
	{"recharge", func(e *Effect) bool { return e.Recharge != "" }, func(e *Effect) { e.Recharge = "" }},
	{"choice", func(e *Effect) bool { return e.Choice != "" }, func(e *Effect) { e.Choice = "" }},
	{"count", func(e *Effect) bool { return e.Count != 0 }, func(e *Effect) { e.Count = 0 }},
	{"from", func(e *Effect) bool { return len(e.From) > 0 }, func(e *Effect) { e.From = nil }},
	{"economy", func(e *Effect) bool { return e.Economy != "" }, func(e *Effect) { e.Economy = "" }},
	{"ability", func(e *Effect) bool { return e.Ability != "" }, func(e *Effect) { e.Ability = "" }},
	{"applies", func(e *Effect) bool { return e.Applies != "" }, func(e *Effect) { e.Applies = "" }},
	{"ignore_cover", func(e *Effect) bool { return e.IgnoreCover }, func(e *Effect) { e.IgnoreCover = false }},
	{"range_multiplier", func(e *Effect) bool { return e.RangeMultiplier != 0 }, func(e *Effect) { e.RangeMultiplier = 0 }},
	{"no_long_range_disadvantage", func(e *Effect) bool { return e.NoLongRangeDisadvantage }, func(e *Effect) { e.NoLongRangeDisadvantage = false }},
	{"progression", func(e *Effect) bool { return e.Progression != "" }, func(e *Effect) { e.Progression = "" }},
	{"prepares", func(e *Effect) bool { return e.Prepares }, func(e *Effect) { e.Prepares = false }},
	{"prepared_max", func(e *Effect) bool { return e.PreparedMax != "" }, func(e *Effect) { e.PreparedMax = "" }},
	{"spellbook", func(e *Effect) bool { return e.Spellbook }, func(e *Effect) { e.Spellbook = false }},
	{"ritual", func(e *Effect) bool { return e.Ritual }, func(e *Effect) { e.Ritual = false }},
	{"handler", func(e *Effect) bool { return e.Handler != "" }, func(e *Effect) { e.Handler = "" }},
	{"max_cr", func(e *Effect) bool { return e.MaxCR != "" }, func(e *Effect) { e.MaxCR = "" }},
	{"no_fly", func(e *Effect) bool { return e.NoFly }, func(e *Effect) { e.NoFly = false }},
	{"no_swim", func(e *Effect) bool { return e.NoSwim }, func(e *Effect) { e.NoSwim = false }},
}

// effectFieldClear is how to clear each of effectFields, by name.
var effectFieldClear = func() map[string]func(e *Effect) {
	out := make(map[string]func(e *Effect), len(effectFields))
	for _, f := range effectFields {
		out[f.name] = f.clear
	}
	return out
}()

// unusedField is the first field of e, in the order of effectFields, that its
// type does not read (a modifier with a recharge, a sense with a count), or "".
// The editor sends only what the type shows, so a stray value is a mistake
// worth naming, not something to ignore silently.
func unusedField(e *Effect) string {
	own := ownFields[e.Type]
	for _, f := range effectFields {
		if f.set(e) && !slices.Contains(own, f.name) {
			return f.name
		}
	}
	return ""
}

// stripUnused clears every field e's type does not read, so an effect that came
// from storage with a stray one still compiles (Overlay.Strict).
func stripUnused(e *Effect) {
	for name := unusedField(e); name != ""; name = unusedField(e) {
		clearEffectField(e, name)
	}
}

// clearEffectField clears the field of effectFields named name; any other name
// changes nothing.
func clearEffectField(e *Effect, name string) {
	if clearField, ok := effectFieldClear[name]; ok {
		clearField(e)
	}
}

// effectError says which TableEffect field a compile error of the engine is
// about, so the editor can point at it: the formula that failed to compile
// (when, value or max), the target, the mode and so on. The messages are the
// engine's own (compileEffect), which the SRD load shares.
func effectError(e *Effect, msg string) (attr, reason string) {
	has := func(words ...string) bool {
		for _, w := range words {
			if strings.Contains(msg, w) {
				return true
			}
		}
		return false
	}
	if attr, ok := rangedAttackErrorAttr(e, msg); ok {
		return attr, ReasonValue
	}
	for _, r := range effectErrorRules {
		if has(r.words...) {
			return r.attr(e, has), r.reason
		}
	}
	return "", ReasonValue
}

// effectErrorRule maps the words of an engine message to the field it is
// about; the first rule whose words the message has wins.
type effectErrorRule struct {
	words  []string
	attr   func(e *Effect, has func(words ...string) bool) string
	reason string
}

// fieldAttr is an effectErrorRule's attr that always names the same field.
func fieldAttr(attr string) func(*Effect, func(...string) bool) string {
	return func(*Effect, func(...string) bool) string { return attr }
}

var effectErrorRules = []effectErrorRule{
	{[]string{"formula "}, formulaAttr, ReasonFormula},
	{[]string{"unknown target"}, func(e *Effect, _ func(...string) bool) string {
		if e.Type == "roll_mode" {
			return ".targets"
		}
		return ".target"
	}, ReasonValue},
	{[]string{"unknown mode"}, fieldAttr(".mode"), ReasonValue},
	{[]string{"value is required", "a value needs"}, func(_ *Effect, has func(...string) bool) string {
		if has("text_pt") {
			return ".text_pt"
		}
		return ".value"
	}, ReasonValue},
	{[]string{"unknown proficiency"}, fieldAttr(".proficiency"), ReasonValue},
	{[]string{"unknown level"}, fieldAttr(".level"), ReasonValue},
	{[]string{"unknown roll"}, fieldAttr(".roll"), ReasonValue},
	{[]string{"targets are required"}, fieldAttr(".targets"), ReasonValue},
	{[]string{"sense needs"}, func(e *Effect, _ func(...string) bool) string {
		if slices.Contains(senses, e.Sense) {
			return ".range_ft"
		}
		return ".sense"
	}, ReasonValue},
	{[]string{"resource needs"}, func(e *Effect, _ func(...string) bool) string {
		switch {
		case e.Resource == "":
			return ".resource"
		case e.Max == "":
			return ".max"
		}
		return ".recharge"
	}, ReasonValue},
	{[]string{"unknown choice"}, fieldAttr(".choice"), ReasonValue},
	{[]string{"in from"}, fieldAttr(".from"), ReasonReference},
	{[]string{"ability_increase"}, func(e *Effect, _ func(...string) bool) string {
		attr, _ := checkAbilityIncrease(e)
		if attr == "" {
			attr = ".value"
		}
		return attr
	}, ReasonValue},
	{[]string{"unknown economy"}, fieldAttr(".economy"), ReasonValue},
	{[]string{"extra_attack needs"}, fieldAttr(".count"), ReasonValue},
	{[]string{"empty tag"}, fieldAttr(".tags"), ReasonValue},
	{[]string{"unknown spell"}, fieldAttr(".spells"), ReasonReference},
}

// formulaAttr is the field whose formula a compile error names (".when", ".value"
// or ".max"), "" when it names none of them. has says whether the error says a word.
func formulaAttr(e *Effect, has func(words ...string) bool) string {
	for _, f := range []struct{ name, src string }{{"when", e.When}, {"value", e.Value}, {"max", e.Max}} {
		if f.src != "" && has("formula "+strconv.Quote(f.src)) {
			return "." + f.name
		}
	}
	return ""
}

func validResourceName(s string) bool {
	if len(s) < 1 || len(s) > 40 {
		return false
	}
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
			return false
		}
	}
	return true
}

// compileEffects checks and compiles every pending effect with the SRD's own
// compiler (compileEffect), once the entries they refer to exist. The overlay's
// Effect values are copied, so the caller's are never written.
func (b *overlayBuilder) compileEffects() error { return b.compileEffectsOf("") }

// compileEffectsOf compiles the pending effects of one owner (every owner when it
// is empty). Every effect that is wrong is reported, not only the first, but only
// those of one owner: the first one to fail.
func (b *overlayBuilder) compileEffectsOf(owner string) error {
	var c entryErrors
	failing := ""
	for _, p := range b.pending {
		if (owner != "" && p.entry != owner) || (failing != "" && p.entry != failing) {
			continue
		}
		out, failed := b.compilePending(&c, p)
		if failed {
			failing = p.entry
		}
		b.n.effects[p.owner] = append(b.n.effects[p.owner], out...)
	}
	return c.err()
}

// compilePending checks and compiles one owner's pending effects, on copies. It
// returns the ones that compiled, and whether any did not (those go to c).
func (b *overlayBuilder) compilePending(c *entryErrors, p pendingEffects) (out []*Effect, failed bool) {
	out = make([]*Effect, 0, len(p.effects))
	for i := range p.effects {
		e := p.effects[i]
		at := fmt.Sprintf("%s.effects[%d]", p.path, i)
		e.Tags, e.Targets, e.From, e.Spells = slices.Clone(e.Tags), slices.Clone(e.Targets), slices.Clone(e.From), slices.Clone(e.Spells)
		if !p.strict {
			stripUnused(&e)
		}
		if err := b.checkEffect(p.owner, at, &e, p.strict); err != nil {
			c.add(err)
			failed = true
			continue
		}
		if err := b.n.compileEffect(p.owner, &e); err != nil {
			attr, reason := effectError(&e, err.Error())
			c.add(ovErr(p.owner, "%v", err).at(at+attr, reason))
			failed = true
			continue
		}
		out = append(out, &e)
	}
	return out, failed
}

// compileOwn compiles an effect the engine wrote itself (the spellcasting
// effect), which is not on the table's menu.
func (b *overlayBuilder) compileOwn(owner, path string, e *Effect) error {
	if err := b.n.compileEffect(owner, e); err != nil {
		// The only thing the table writes into it is the prepared formula.
		return ovErr(owner, "%v", err).at(path+".casting.prepared_max", ReasonFormula)
	}
	b.n.effects[owner] = append(b.n.effects[owner], e)
	return nil
}

// abilityField is the name of an ability in the API's AbilityScores message, where
// the violations point ("table_race.ability_bonuses.wisdom").
var abilityField = map[Ability]string{STR: "strength", DEX: "dexterity", CON: "constitution", INT: "intelligence", WIS: "wisdom", CHA: "charisma"}

// bonusMap turns a map of ability bonuses into the SRD's shape, checking the
// abilities and the size of each bonus. A bonus out of range is a violation at
// ability_bonuses.<ability>, in the abilities' order.
func bonusMap(c *entryErrors, key, path string, in map[Ability]int) map[string]int {
	out := make(map[string]int, len(in))
	for _, a := range AllAbilities() {
		v, ok := in[a]
		if !ok {
			continue
		}
		if v < -maxRaceBonus || v > maxRaceBonus {
			c.at(key, path, ".ability_bonuses."+abilityField[a], ReasonLimit, "an ability bonus is %d to %d", -maxRaceBonus, maxRaceBonus)
		}
		out[string(a)] = v
	}
	for a := range in {
		if _, ok := abilityIndex[a]; !ok {
			c.at(key, path, ".ability_bonuses", ReasonValue, "unknown ability in the ability bonuses")
		}
	}
	return out
}

// maxRaceBonus bounds a race's ability bonus, in either direction.
const maxRaceBonus = 4

// The bounds of a table race's and background's numbers.
const (
	// feetStep is the grid a speed or a darkvision range is on.
	feetStep = 5
	// maxRaceSpeedFt and maxDarkvisionFt bound a race's speed and darkvision.
	maxRaceSpeedFt  = 120
	maxDarkvisionFt = 120
	// maxEntryLanguageChoices is the most languages a race or background lets
	// the player choose.
	maxEntryLanguageChoices = 4
	// maxBackgroundTools is the most tool proficiencies a background gives.
	maxBackgroundTools = 4
)

// raceSizes are the sizes a table race may have.
var raceSizes = []string{"Tiny", "Small", "Medium", "Large"}

// addRace registers a race and its traits. Every field that is wrong is reported
// at its own path (size, speed_ft, ability_bonuses.<ability>, choice_bonuses[i],
// languages[i]...), in one error.
func (b *overlayBuilder) addRace(tr *TableRace, path string) error {
	key := tr.Key
	var c entryErrors
	checkRaceBody(&c, tr, path)
	bonuses := bonusMap(&c, key, path, tr.AbilityBonuses)
	choice := raceChoiceBonuses(&c, key, path, tr.ChoiceBonuses)
	b.checkRaceLanguages(&c, tr, path)
	race := &srd51.Race{
		Key: key, Name: tr.NamePT, SpeedFt: tr.SpeedFt, Size: tr.Size, AbilityBonuses: bonuses,
		Languages: slices.Clone(tr.Languages), LanguageChoices: tr.LanguageChoices,
	}
	race.Traits = b.addTraits(&c, key, path, tr.Traits)
	if err := c.err(); err != nil {
		return err
	}
	b.register(tr.TableEntry)
	b.n.races[key] = race
	if len(choice) > 0 {
		b.n.raceChoice[key] = choice
	}
	if tr.DarkvisionFt > 0 {
		// The darkvision is an effect of the race itself: Derive reads those too.
		b.pending = append(b.pending, pendingEffects{owner: key, entry: key, effects: []Effect{{Type: "sense", Sense: "darkvision", RangeFt: tr.DarkvisionFt}}, path: path + ".darkvision_ft"})
	}
	return nil
}

// checkRaceBody checks a race's size, speed and darkvision.
func checkRaceBody(c *entryErrors, tr *TableRace, path string) {
	key := tr.Key
	if !slices.Contains(raceSizes, tr.Size) {
		c.at(key, path, ".size", ReasonValue, "size is Tiny, Small, Medium or Large")
	}
	if tr.SpeedFt < feetStep || tr.SpeedFt > maxRaceSpeedFt || tr.SpeedFt%feetStep != 0 {
		c.at(key, path, ".speed_ft", ReasonLimit, "the speed is 5 to 120 feet, in steps of 5")
	}
	if tr.DarkvisionFt < 0 || tr.DarkvisionFt > maxDarkvisionFt || tr.DarkvisionFt%feetStep != 0 {
		c.at(key, path, ".darkvision_ft", ReasonLimit, "the darkvision is 0 or 5 to 120 feet, in steps of 5")
	}
}

// raceChoiceBonuses checks the bonuses a race lets the player place and returns
// them largest first.
func raceChoiceBonuses(c *entryErrors, key, path string, in []int) []int {
	choice := slices.Clone(in)
	if len(choice) > len(abilityIndex) {
		c.at(key, path, ".choice_bonuses", ReasonLimit, "at most %d bonuses to place", len(abilityIndex))
	}
	for i, v := range choice {
		if v < 1 || v > maxRaceBonus {
			c.at(key, path, fmt.Sprintf(".choice_bonuses[%d]", i), ReasonLimit, "a bonus to place is 1 to %d", maxRaceBonus)
		}
	}
	slices.SortFunc(choice, func(a, c int) int { return c - a })
	return choice
}

// checkRaceLanguages checks the languages a race knows and lets the player choose.
func (b *overlayBuilder) checkRaceLanguages(c *entryErrors, tr *TableRace, path string) {
	if tr.LanguageChoices < 0 || tr.LanguageChoices > maxEntryLanguageChoices {
		c.at(tr.Key, path, ".language_choices", ReasonLimit, "languages to choose is 0 to 4")
	}
	for i, l := range tr.Languages {
		if _, ok := b.base.languages[l]; !ok {
			c.at(tr.Key, path, fmt.Sprintf(".languages[%d]", i), ReasonReference, "language %q is not in the SRD", l)
		}
	}
}

// addTraits adds the traits of a race or a subrace and returns the keys of the
// ones that were added; the others' problems go to c.
func (b *overlayBuilder) addTraits(c *entryErrors, key, path string, traits []TableFeature) []string {
	var keys []string
	for i := range traits {
		t := &traits[i]
		at := fmt.Sprintf("%s.traits[%d]", path, i)
		if err := b.addFeature(t, featureKind{prefix: "trait:", owner: key}, at); err != nil {
			c.add(locate(err, at))
			continue
		}
		keys = append(keys, t.Key)
	}
	return keys
}

// addSubrace registers a subrace and puts it in its race's list, on a copy of
// the race when the race is the SRD's.
func (b *overlayBuilder) addSubrace(ts *TableSubrace, path string) error {
	key := ts.Key
	var c entryErrors
	if !b.isRace(ts.Race) {
		c.at(key, path, ".race_key", ReasonReference, "the race %q does not exist", ts.Race)
	}
	bonuses := bonusMap(&c, key, path, ts.AbilityBonuses)
	sub := &srd51.Subrace{Key: key, Name: ts.NamePT, Race: ts.Race, AbilityBonuses: bonuses}
	sub.Traits = b.addTraits(&c, key, path, ts.Traits)
	if err := c.err(); err != nil {
		return err
	}
	b.register(ts.TableEntry)
	b.n.subraces[key] = sub
	// The race lists its subraces: on a copy, because the SRD's race is shared.
	race := *b.n.races[ts.Race]
	race.Subraces = append(slices.Clone(race.Subraces), key)
	b.n.races[ts.Race] = &race
	return nil
}

// addBackground registers a background and its feature. Every field that is wrong
// is reported at its own path (skills[i], tools[i], language_choices, feature...).
func (b *overlayBuilder) addBackground(tb *TableBackground, path string) error {
	key := tb.Key
	var c entryErrors
	b.checkBackgroundSkills(&c, tb, path)
	if len(tb.Tools) > maxBackgroundTools {
		c.at(key, path, ".tools", ReasonLimit, "at most 4 tools")
	}
	if tb.LanguageChoices < 0 || tb.LanguageChoices > maxEntryLanguageChoices {
		c.at(key, path, ".language_choices", ReasonLimit, "at most 4 languages to choose")
	}
	b.checkBackgroundTools(&c, tb, path)
	checkTextAt(&c, key, path, ".equipment_pt", []string{tb.EquipmentPT})
	f := &tb.Feature
	if err := b.addFeature(f, featureKind{prefix: "background-feature:", owner: key}, path+".feature"); err != nil {
		c.add(locate(err, path+".feature"))
	}
	if err := c.err(); err != nil {
		return err
	}
	b.register(tb.TableEntry)
	b.n.backgrounds[key] = &srd51.Background{
		Key: key, Name: tb.NamePT, Skills: slices.Clone(tb.Skills), Proficiencies: slices.Clone(tb.Tools),
		LanguageChoices: tb.LanguageChoices,
		Feature:         srd51.BackgroundFeature{Key: f.Key, Name: f.NamePT, Desc: slices.Clone(f.DescPT)},
	}
	if tb.EquipmentPT != "" {
		b.n.bgEquipment[key] = tb.EquipmentPT
	}
	return nil
}

// checkBackgroundSkills checks a background's two different SRD skills.
func (b *overlayBuilder) checkBackgroundSkills(c *entryErrors, tb *TableBackground, path string) {
	key := tb.Key
	if len(tb.Skills) != CustomBackgroundSkillCount {
		c.at(key, path, ".skills", ReasonValue, "a background gives %d different skills", CustomBackgroundSkillCount)
	}
	for i, s := range tb.Skills {
		if _, ok := b.base.skills[s]; !ok {
			c.at(key, path, fmt.Sprintf(".skills[%d]", i), ReasonReference, "skill %q is not in the SRD", s)
		} else if slices.Contains(tb.Skills[:i], s) {
			c.at(key, path, fmt.Sprintf(".skills[%d]", i), ReasonValue, "a background gives %d different skills", CustomBackgroundSkillCount)
		}
	}
}

// checkBackgroundTools checks that each tool of a background is a tool (or
// other) proficiency of the SRD.
func (b *overlayBuilder) checkBackgroundTools(c *entryErrors, tb *TableBackground, path string) {
	for i, t := range tb.Tools {
		if p, ok := b.base.proficiencies[t]; !ok || (p.Kind != "tool" && p.Kind != "other") {
			c.at(tb.Key, path, fmt.Sprintf(".tools[%d]", i), ReasonReference, "%q is not a tool proficiency of the SRD", t)
		}
	}
}
