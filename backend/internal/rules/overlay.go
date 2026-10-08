package rules

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/PuraFome/meuRPG/backend/internal/platform/names"
	"github.com/PuraFome/meuRPG/backend/internal/rules/formula"
)

// The table's own content: classes, subclasses, races, subraces, backgrounds
// and spells that the master writes for one campaign (MR-025, RN-23,
// ADR-0018). Content.With adds them, as an Overlay, on top of the SRD content
// and returns a new Content; the SRD one is never changed, because it is a
// singleton that many requests read at once.
//
// The overlay uses the same loaders as the SRD: the same effect compiler
// (compileEffect), the same formula compiler, the same catalog builder. What it
// adds is checks: every key ends in "@mesa", the effects come from a closed
// menu with no handler (the table never runs code), and the class contract
// (the 20-level table, the casting kind) is checked as a whole.

const (
	// tableSuffix ends every key the table adds: "class:cavaleiro-runico@mesa".
	// The prefix before it is the SRD's, so every place that decides by prefix
	// ("class:", "feature:", "spell:"...) keeps working.
	tableSuffix = "@mesa"
	// MaxOverlayEntries is the most entries (of the six kinds, features not
	// counted) one table may have.
	MaxOverlayEntries = 300
	// MaxTableFeatures is the most features one class may have, and the most
	// one subclass may have, counted apart. The features the engine makes (the
	// Ability Score Improvements and the Spellcasting feature) do not count.
	MaxTableFeatures = 60
	// MaxSlugLength bounds the part of a key between the prefix and "@mesa".
	MaxSlugLength = 60

	maxNameRunes      = 80
	maxTextParagraphs = 50
	maxTextRunes      = 4000
	maxFeatureEffects = 20
)

// Casting kinds of a TableClass and of a TableSubclass.
const (
	// CastingNone: the class does not cast (the zero value).
	CastingNone = ""
	// CastingFull: a full caster (the Wizard's table).
	CastingFull = "full"
	// CastingHalf: a half caster (the Paladin's), which starts at level 2 unless
	// the table says otherwise.
	CastingHalf = "half"
	// CastingPact: pact magic (the Warlock's), apart from the other slots.
	CastingPact = "pact"
	// CastingThird: a third caster, which only a subclass can be (the Eldritch
	// Knight of 2014): the multiclass rule counts a third of its levels.
	CastingThird = "third"
)

// Overlay is the table's content as the engine takes it. Its shape follows the
// fields the editors fill (slice 10.1c maps the protos onto it one to one).
type Overlay struct {
	// Revision is the table's content_revision. It goes in the content
	// version: "srd51@<commit>+fx.<n>+mesa.<revision>".
	Revision int
	// Strict lists the entries the caller is writing now (their keys). A field an
	// effect's type does not read is refused in them (a write is checked); in the
	// others, which come from storage, it is ignored, so a stray field can never
	// make a campaign unreadable.
	Strict []string
	// Off are the keys the master switched off for the players: SRD keys and the
	// table's own ("Opções para os jogadores", RN-23). A key that is no class,
	// subclass, race, subrace, background or spell of this content is ignored, so
	// a stored switch can never make a campaign unreadable.
	Off         []string
	Classes     []TableClass
	Subclasses  []TableSubclass
	Races       []TableRace
	Subraces    []TableSubrace
	Backgrounds []TableBackground
	Spells      []TableSpell
}

// TableEntry is what every entry has. Key is "<kind>:<slug>@mesa" (kind is
// class, subclass, race, subrace, background or spell), the slug is 1 to 60 of
// [a-z0-9-], and the key never changes after the entry is created. The table's
// entries have no English name: NamePT is the name everywhere. Archived says
// the master retired the entry: sheets that use it keep working, but it is not
// offered as a new choice (Content.ArchivedKeys helps the server refuse it).
type TableEntry struct {
	Key      string
	NamePT   string
	Archived bool
	// Revision is the table's content revision at the entry's last change (0 for
	// an entry made outside the server, as in tests). ChangedSince compares it
	// with the revision a sheet was saved at. An archive is not a change: it does
	// not move it.
	Revision int
	// ChangedAt is when the entry last changed (a creation or an edit, never an
	// archive); the zero time for an entry made outside the server.
	ChangedAt time.Time
}

// TableFeature is a class, subclass or background feature, or a race's trait.
// Its Key is "feature:<slug>@mesa" (class and subclass), "trait:<slug>@mesa"
// (race and subrace) or "background-feature:<slug>@mesa" (background), and the
// server makes it once and never changes it, because a sheet's feature choices
// point at it. A feature with no Effects is text only, and always valid.
type TableFeature struct {
	Key     string
	NamePT  string
	DescPT  []string
	Effects []Effect
}

// TableCasting is how a class (or, for a third caster, a subclass) casts.
type TableCasting struct {
	// Kind is a Casting* constant. A class is CastingNone, CastingFull,
	// CastingHalf or CastingPact; a subclass that casts is CastingThird.
	Kind string
	// Ability is the spellcasting ability.
	Ability Ability
	// Preparation is PreparationKnown or PreparationPrepared.
	Preparation string
	// ListFrom is the class whose spell list the caster uses: an SRD class
	// ("class:wizard") or a table class that has its own list. For a class it may
	// be empty, meaning its own list (the spells that name the class); the spells
	// that name the class are then added to the ones of ListFrom. For a third
	// caster it is required.
	ListFrom string
	// PreparedMax is the formula of how many spells a class that prepares may
	// prepare. Empty is the synthesized one: the ability modifier plus the class
	// level (half of it for a half caster), at least 1.
	PreparedMax string
	// StartLevel is the class level at which casting starts. 0 is the default: 1
	// (2 for a half caster, 3 for a third caster).
	StartLevel int
	// Ritual says the caster may cast rituals.
	Ritual bool
}

// TableLevel is the casting columns of one row of a class table. Slots[0] is
// the number of 1st-level slots; for pact magic exactly one entry is not zero
// (the pact slots, all of one level).
type TableLevel struct {
	CantripsKnown int
	SpellsKnown   int
	Slots         [9]int
}

// TableClassLevel is one row of a class's 20-level table.
type TableClassLevel struct {
	// ProfBonus is the proficiency bonus at this level; 0 is the SRD's
	// (+2 at levels 1 to 4, up to +6 at 17 to 20).
	ProfBonus int
	// Features are the features the class gains at this level.
	Features []TableFeature
	TableLevel
}

// TableClass is a class: the contract of ADR-0018, section 8.
type TableClass struct {
	TableEntry
	// HitDie is 6, 8, 10 or 12.
	HitDie int
	// SavingThrows are the two abilities of the starting class.
	SavingThrows []Ability
	// SkillChoose is how many skills the player picks at level 1, from
	// SkillFrom (SRD skill keys).
	SkillChoose int
	SkillFrom   []string
	// Proficiencies are the armor, weapon and tool proficiency keys
	// ("proficiency:light-armor") of a character who starts in this class.
	Proficiencies []string
	// The multiclass prerequisites: every Minimums entry must be met, or, with
	// AnyOf set, one AnyOf entry instead. MulticlassProficiencies and
	// MulticlassSkillChoose are what taking this class as a later class gives.
	Minimums                map[Ability]int
	AnyOf                   map[Ability]int
	MulticlassProficiencies []string
	MulticlassSkillChoose   int
	// SubclassLevel is the class level at which the subclass is chosen, 1 to 20;
	// 0 is 3.
	SubclassLevel int
	// ASILevels are the levels with an Ability Score Improvement; empty is the
	// SRD's (4, 8, 12, 16 and 19). The server writes the features.
	ASILevels []int
	Casting   TableCasting
	// Levels is the class table: exactly 20 rows, level 1 first.
	Levels []TableClassLevel
}

// TableAlwaysPrepared is a spell a subclass always has prepared from a class
// level (a domain's, an oath's). It does not count against the prepared limit.
type TableAlwaysPrepared struct {
	ClassLevel int
	Spell      string
}

// TableSubclassLevel is a subclass's features at a class level and, for a third
// caster, its casting columns there.
type TableSubclassLevel struct {
	// Level is the class level, 1 to 20; rows are in ascending order.
	Level    int
	Features []TableFeature
	TableLevel
}

// TableSubclass is a subclass of an SRD class or a table class.
type TableSubclass struct {
	TableEntry
	// Class is the parent class key. It never changes after creation.
	Class string
	// Level is the class level the choice happens at. The engine keeps it as a
	// class's property (the SRD's too), so it must be the parent's SubclassLevel,
	// or 0 for "the parent's".
	Level  int
	DescPT []string
	// Levels are the features by class level.
	Levels []TableSubclassLevel
	// Casting makes the subclass a third caster (CastingThird); the parent
	// class must not cast. Its Levels must have a row, with the casting columns,
	// for every level from the start level to 20.
	Casting *TableCasting
	// AlwaysPrepared are the spells it always has prepared.
	AlwaysPrepared []TableAlwaysPrepared
}

// TableRace is a race in the 2014 shape.
type TableRace struct {
	TableEntry
	// Size is "Tiny", "Small", "Medium" or "Large".
	Size    string
	SpeedFt int
	// AbilityBonuses are the fixed bonuses; ChoiceBonuses are the ones the
	// player places on abilities of their choice, such as {2, 1} for "+2 and +1
	// to your choice" (the master's own option; each goes on a different ability).
	AbilityBonuses map[Ability]int
	ChoiceBonuses  []int
	// DarkvisionFt is 0 or the range of the darkvision sense, in feet.
	DarkvisionFt int
	// Languages are SRD language keys always known; LanguageChoices more are
	// chosen.
	Languages       []string
	LanguageChoices int
	// Traits are the race's traits ("trait:<slug>@mesa").
	Traits []TableFeature
}

// TableSubrace is a subrace of an SRD race or a table race.
type TableSubrace struct {
	TableEntry
	Race           string
	AbilityBonuses map[Ability]int
	Traits         []TableFeature
}

// TableBackground is a background: two skills, tools and languages, and a
// feature that is a text.
type TableBackground struct {
	TableEntry
	// Skills are the two skills it gives (SRD skill keys).
	Skills []string
	// Tools are tool proficiency keys ("proficiency:thieves-tools").
	Tools           []string
	LanguageChoices int
	EquipmentPT     string
	// Feature is the background's feature, usually a note
	// ("background-feature:<slug>@mesa").
	Feature TableFeature
}

// TableSpell is a spell. Its details become the SpellDetails of the SRD's
// spells, so the combat that resolves an SRD spell resolves this one.
type TableSpell struct {
	TableEntry
	// Level is 0 (a cantrip) to 9.
	Level int
	// School is an SRD school key ("school:evocation").
	School      string
	CastingTime TableCastingTime
	Range       TableRange
	Duration    TableDuration
	Components  TableComponents
	// Concentration needs a timed duration, written "up to". Ritual needs a
	// level of 1 or more.
	Concentration bool
	Ritual        bool
	// Classes are the classes (SRD or table) whose list has the spell.
	Classes []string
	// DescPT and HigherLevelPT are the text, in Portuguese.
	DescPT        []string
	HigherLevelPT []string
	// Target is whom it reaches (required).
	Target SpellTarget
	// Attack is "melee", "ranged" or "" (no spell attack); Save is the saving
	// throw (an ability, and "half" or "none" on a pass), or nil. Not both.
	Attack string
	Save   *SpellSave
	// Damage are the damage parts; Heal the healing, or nil.
	Damage []TableSpellDamage
	Heal   *TableSpellHeal
}

// TableCastingTime is how long a table spell takes: Unit is one of CastAction,
// CastBonusAction, CastReaction, CastMinute and CastHour. An action, bonus action
// or reaction takes 1; minutes and hours 1 to 60. TriggerPT is the reaction's
// trigger, in Portuguese ("quando você sofre dano"), only for a reaction.
type TableCastingTime struct {
	Unit      string
	Amount    int
	TriggerPT string
}

// TableRange is a table spell's range: Kind is RangeSelf, RangeTouch, RangeSight,
// RangeUnlimited or RangeRanged, and DistanceFt (5 to 5280, in steps of 5) is
// for RangeRanged only.
type TableRange struct {
	Kind       string
	DistanceFt int
}

// TableDuration is a table spell's duration: Kind is DurationInstantaneous,
// DurationTimed or DurationUntilDispelled; Amount (1 to 999) and Unit (a
// Duration* unit) are for DurationTimed. UpTo writes "up to"; a concentration
// spell (TableSpell.Concentration) always is.
type TableDuration struct {
	Kind   string
	Amount int
	Unit   string
	UpTo   bool
}

// TableComponents says which components a table spell has. MaterialPT describes
// the material and goes with Material, and only with it.
type TableComponents struct {
	Verbal, Somatic, Material bool
	MaterialPT                string
}

// TableSpellDamage is one damage type of a table spell. Dice, PerSlotLevel and
// PerTier are plain dice ("8d6", "1d6"), PerSlotLevel and PerTier of the same
// die as Dice.
type TableSpellDamage struct {
	// Type is an SRD damage type key ("damage-type:fire").
	Type string
	// Dice is the damage at the spell's own level (a leveled spell) or at level
	// 1 (a cantrip).
	Dice string
	// PerSlotLevel is the extra dice for each slot level above the spell's own
	// (a leveled spell only).
	PerSlotLevel string
	// PerTier is the extra dice at each cantrip tier: levels 5, 11 and 17 (a
	// cantrip only). With a cantrip's damage always grows by tier, even if it is
	// just the one entry.
	PerTier string
}

// TableSpellHeal is a table spell's healing.
type TableSpellHeal struct {
	// Dice is the healing at the spell's own level; PerSlotLevel the extra dice
	// per slot level above it. AddsModifier adds the spellcasting modifier.
	Dice         string
	PerSlotLevel string
	AddsModifier bool
}

// Reasons of an OverlayError. They are stable codes: the server turns each into
// a field violation and the app into its own Portuguese copy; nobody parses
// Message.
const (
	// ReasonLimit: a count, size or budget is over its limit.
	ReasonLimit = "limit"
	// ReasonKey: a key is malformed or of another kind.
	ReasonKey = "bad_key"
	// ReasonDuplicateKey: the key already exists.
	ReasonDuplicateKey = "duplicate_key"
	// ReasonReservedKey: the key uses a word the SRD's engine logic reacts to
	// (magical-secrets, wild-shape, ability-score-improvement, -spellcasting).
	ReasonReservedKey = "reserved_key"
	// ReasonName: a name is empty, untrimmed, too long or has a line break.
	ReasonName = "bad_name"
	// ReasonText: a text is too long.
	ReasonText = "bad_text"
	// ReasonReference: a reference points at something that does not exist, or
	// that the SRD does not have where it must.
	ReasonReference = "dangling_reference"
	// ReasonEffect: an effect is not on the table's menu (a handler, for one).
	ReasonEffect = "forbidden_effect"
	// ReasonFormula: a formula does not compile.
	ReasonFormula = "bad_formula"
	// ReasonTable: the class table, the casting rows or the levels are wrong.
	ReasonTable = "bad_table"
	// ReasonCasting: the casting setup is wrong.
	ReasonCasting = "bad_casting"
	// ReasonValue: any other value out of its range or not allowed.
	ReasonValue = "bad_value"
	// ReasonOverlay: something about the overlay as a whole (a table layer that
	// already exists, a negative revision).
	ReasonOverlay = "overlay"
)

// OverlayError is what With returns. Key is the entry or feature at fault (empty
// for the overlay as a whole). Field is the path in the Overlay the caller
// passed, with its own indexes ("classes[2].levels[4].features[0].effects[1]"),
// ending in the attribute when it is known; Reason is a stable Reason* code.
// Message is English text for logs and tests, never for the app.
type OverlayError struct {
	Key     string
	Field   string
	Reason  string
	Message string
	// More are the other things wrong with the same entry, found in the same pass
	// (a spell, a race and a background list every field they get wrong, so the
	// master sees them at once). Each has its own Field and Reason.
	More []*OverlayError
}

// Violations is the error and the ones after it, in the order they were found.
func (e *OverlayError) Violations() []*OverlayError {
	return append([]*OverlayError{e}, e.More...)
}

func (e *OverlayError) Error() string {
	if e.Key == "" {
		return "overlay: " + e.Message
	}
	return "overlay: " + e.Key + ": " + e.Message
}

// ovErr makes an error; Field and Reason are filled where it is raised (at) or,
// for the entry-level checks, from the message and the entry's path (locate).
func ovErr(key, format string, args ...any) *OverlayError {
	return &OverlayError{Key: key, Message: fmt.Sprintf(format, args...)}
}

// entryErrors collects what is wrong with one entry, so the whole list comes back
// at once instead of the first only (MR-025: the editor marks every field).
type entryErrors struct{ list []*OverlayError }

// add records err (an *OverlayError, or any other error as the entry's own).
func (c *entryErrors) add(err error) {
	if err == nil {
		return
	}
	if oe, ok := errors.AsType[*OverlayError](err); ok {
		c.list = append(c.list, oe.Violations()...)
		oe.More = nil
		return
	}
	c.list = append(c.list, &OverlayError{Message: err.Error(), Reason: ReasonValue})
}

// at records a violation at attr of the entry at path.
func (c *entryErrors) at(key, path, attr, reason, format string, args ...any) {
	c.list = append(c.list, bad(key, path, attr, reason, format, args...))
}

// err is the first violation carrying the rest, or nil when there is none.
func (c *entryErrors) err() error {
	if len(c.list) == 0 {
		return nil
	}
	first := *c.list[0]
	first.More = c.list[1:]
	return &first
}

// reason sets the reason only; the field is filled from the entry's path.
func (e *OverlayError) reason(r string) *OverlayError {
	e.Reason = r
	return e
}

// at sets the field and the reason.
func (e *OverlayError) at(field, reason string) *OverlayError {
	e.Field, e.Reason = field, reason
	return e
}

// With returns a new Content: c plus the table's entries. c is never changed
// (it is shared by every request); the maps are cloned, the entries added, and
// the indexes, the casting and the catalog rebuilt with the same loaders and
// checks as the SRD's. The version becomes "<c's version>+mesa.<revision>". It
// returns an *OverlayError when the overlay breaks a rule (see Overlay), and
// refuses a content that already has a table layer.
func (c *Content) With(o Overlay) (*Content, error) {
	b := &overlayBuilder{base: c.c}
	if err := b.build(o); err != nil {
		return nil, err
	}
	return &Content{c: b.n}, nil
}

// TableKeys are the keys of the table's layer that a Build uses, sorted: the
// race, subrace, classes, subclasses, background, spells and options with a key
// ending in "@mesa". A sheet that has any cannot move to another campaign
// (ADR-0018, section 10).
func TableKeys(b Build) []string {
	return buildKeys(b, isTableKey)
}

// ArchivedKeys are the keys the table has retired that a Build still uses,
// sorted. The server compares the ones of the new sheet with the ones of the
// previous sheet and refuses an archived key that is new (a retired entry is
// never a new choice, but old sheets keep it).
func (c *Content) ArchivedKeys(b Build) []string {
	return buildKeys(b, func(k string) bool { return c.c.archived[k] })
}

// Off says whether the master switched the key off for the players (RN-23): the
// key's own switch, whatever the kind. Always false for the plain SRD content.
func (c *Content) Off(key string) bool { return c.c.off[key] }

// Hidden says whether the players never receive the key: the table retired it,
// the master switched it off, or it is a subclass or subrace whose class or race
// is switched off. A sheet that already has a hidden key keeps it (ADR-0018,
// section 7); it is only not a choice.
func (c *Content) Hidden(key string) bool {
	if c.c.archived[key] || c.c.offOrParent(key) {
		return true
	}
	// The child of a retired class or race is not offered either.
	if s, ok := c.c.subclasses[key]; ok {
		return c.c.archived[s.Class]
	}
	if s, ok := c.c.subraces[key]; ok {
		return c.c.archived[s.Race]
	}
	return false
}

// Switchable says whether the key is one the master can switch on or off: a
// class, subclass, race, subrace, background or spell of this content, the SRD's
// or the table's.
func (c *Content) Switchable(key string) bool { return c.c.isSwitchable(key) }

// AnyHidden says whether the table retired or switched off anything at all, so a
// caller that only filters what a player receives can skip the work when not.
func (c *Content) AnyHidden() bool { return len(c.c.archived) > 0 || len(c.c.off) > 0 }

// OffKeys are the keys a Build uses that the master switched off for the players,
// sorted. A subclass or subrace under an off class or race counts too, but only
// when the Build lacks that class or race: a sheet that already has the off class
// judges its own subclass by the subclass's own switch, so it can still reach its
// subclass level (question 80); a new sheet cannot pick the class, so it cannot
// pick the subclass either. The server compares the ones of the new sheet with the
// ones of the previous sheet and refuses an off key that is new.
func (c *Content) OffKeys(b Build) []string {
	hasClass := map[string]bool{}
	for _, cl := range b.Classes {
		hasClass[cl.Class] = true
	}
	return buildKeys(b, func(k string) bool {
		if c.c.off[k] {
			return true
		}
		if s, ok := c.c.subclasses[k]; ok {
			return c.c.off[s.Class] && !hasClass[s.Class]
		}
		if s, ok := c.c.subraces[k]; ok {
			return c.c.off[s.Race] && b.Race != s.Race
		}
		return false
	})
}

// offOrParent says whether the key is switched off, or is the subclass of a class
// or the subrace of a race that is.
func (c *content) offOrParent(key string) bool {
	if len(c.off) == 0 {
		return false
	}
	if c.off[key] {
		return true
	}
	if s, ok := c.subclasses[key]; ok {
		return c.off[s.Class]
	}
	if s, ok := c.subraces[key]; ok {
		return c.off[s.Race]
	}
	return false
}

// isSwitchable says whether the key is a class, subclass, race, subrace,
// background or spell of the content.
func (c *content) isSwitchable(key string) bool {
	switch {
	case strings.HasPrefix(key, "class:"):
		_, ok := c.classes[key]
		return ok
	case strings.HasPrefix(key, "subclass:"):
		_, ok := c.subclasses[key]
		return ok
	case strings.HasPrefix(key, "race:"):
		_, ok := c.races[key]
		return ok
	case strings.HasPrefix(key, "subrace:"):
		_, ok := c.subraces[key]
		return ok
	case strings.HasPrefix(key, "background:"):
		_, ok := c.backgrounds[key]
		return ok
	case strings.HasPrefix(key, "spell:"):
		_, ok := c.spells[key]
		return ok
	}
	return false
}

func buildKeys(b Build, want func(string) bool) []string {
	seen := map[string]bool{}
	add := func(keys ...string) {
		for _, k := range keys {
			if k != "" && want(k) {
				seen[k] = true
			}
		}
	}
	add(b.Race, b.Subrace, b.Background)
	for _, cl := range b.Classes {
		add(cl.Class, cl.Subclass)
	}
	add(b.Cantrips...)
	add(b.SpellsKnown...)
	add(b.SpellsPrepared...)
	add(b.FeatureChoices...)
	return sortedKeys(seen)
}

// TableRevision is the content revision of the table layer this content has
// (the Overlay's Revision), or 0 for the plain SRD content.
func (c *Content) TableRevision() int { return c.c.tableRevision }

// ChangedEntry is a table entry a sheet uses that changed after the sheet was
// last saved.
type ChangedEntry struct {
	// Key is the entry's key and Field the sheet field that holds it, with the
	// CharacterSheet field names ("full.classes[0].class_key").
	Key, Field string
	// Revision is the content revision at which the entry last changed, and
	// ChangedAt when.
	Revision  int
	ChangedAt time.Time
	// Since is the revision the sheet was last known to fit the entry at.
	Since int
}

// KeyField is a content key a Build uses and the sheet field that holds it.
type KeyField struct {
	Key, Field string
}

// BuildKeys lists the content keys a Build uses with the sheet field of each,
// with the CharacterSheet field names ("full.classes[0].class_key"), in the
// sheet's order: race, subrace, classes and subclasses, background, cantrips,
// known spells, prepared spells and feature choices.
func BuildKeys(b Build) []KeyField {
	var out []KeyField
	add := func(key, field string) {
		if key != "" {
			out = append(out, KeyField{Key: key, Field: field})
		}
	}
	add(b.Race, "full.race_key")
	add(b.Subrace, "full.subrace_key")
	for i, cl := range b.Classes {
		add(cl.Class, fmt.Sprintf("full.classes[%d].class_key", i))
		add(cl.Subclass, fmt.Sprintf("full.classes[%d].subclass_key", i))
	}
	add(b.Background, "full.background_key")
	for i, k := range b.Cantrips {
		add(k, fmt.Sprintf("full.cantrip_keys[%d]", i))
	}
	for i, k := range b.SpellsKnown {
		add(k, fmt.Sprintf("full.known_spell_keys[%d]", i))
	}
	for i, k := range b.SpellsPrepared {
		add(k, fmt.Sprintf("full.prepared_spell_keys[%d]", i))
	}
	for i, k := range b.FeatureChoices {
		add(k, fmt.Sprintf("full.feature_choice_keys[%d]", i))
	}
	return out
}

// ChangedSince lists the table entries a Build uses whose last change is newer
// than savedRevision, the content revision the sheet was last saved at (RN-23,
// question 80: a change applies at once, and the owner is told). The order is
// the sheet's (BuildKeys).
func (c *Content) ChangedSince(b Build, savedRevision int, baselines map[string]int) []ChangedEntry {
	var out []ChangedEntry
	seen := map[string]bool{}
	for _, kf := range BuildKeys(b) {
		since := savedRevision
		if r, ok := baselines[kf.Key]; ok {
			since = r // an entry the sheet is still flagged for keeps its older baseline
		}
		if seen[kf.Key] || c.c.entryRevision[kf.Key] <= since {
			continue
		}
		seen[kf.Key] = true
		out = append(out, ChangedEntry{Key: kf.Key, Field: kf.Field, Revision: c.c.entryRevision[kf.Key], Since: since, ChangedAt: c.c.entryChangedAt[kf.Key]})
	}
	return out
}

// IsTableKey says whether a content key is the table's ("...@mesa").
func IsTableKey(key string) bool { return isTableKey(key) }

// overlayBuilder is the work of one With call: the base (read only) and the
// clone it fills.
type overlayBuilder struct {
	base, n *content

	// entries are the keys the overlay introduces, by entry key. used also holds
	// the features' keys. Both are checked for collisions as they are added.
	entries map[string]bool
	used    map[string]bool
	// classListFrom, classCasts and classSubLevel are what the first pass learns
	// of the table classes, so a reference to a class declared later can be
	// checked.
	classListFrom map[string]string
	classCasts    map[string]string
	classSubLevel map[string]int
	// pending are the effects to compile once every entry exists.
	pending []pendingEffects
	// strict is Overlay.Strict as a set.
	strict map[string]bool
}

type pendingEffects struct {
	owner string
	// entry is the table entry the owner belongs to (the owner itself for an entry's own effects).
	entry   string
	effects []Effect
	// strict says the entry is being written: see Overlay.Strict.
	strict bool
	// path is where the effects are in the Overlay, for errors.
	path string
}

// byKey lists the indexes of a slice of entries in the order of their keys, so
// the result of With does not depend on the order the caller listed them in
// (the order of a class's subclasses, of a race's subraces...). The indexes stay
// the caller's, so an error's Field points at the entry as it was passed.
func byKey[T any](items []T, key func(*T) string) []int {
	idx := make([]int, len(items))
	for i := range idx {
		idx[i] = i
	}
	slices.SortStableFunc(idx, func(a, b int) int { return strings.Compare(key(&items[a]), key(&items[b])) })
	return idx
}

func (b *overlayBuilder) build(o Overlay) error {
	if b.base.table {
		return ovErr("", "the content already has a table layer").at("", ReasonOverlay)
	}
	if o.Revision < 0 {
		return ovErr("", "the revision cannot be negative").at("revision", ReasonOverlay)
	}
	total := len(o.Classes) + len(o.Subclasses) + len(o.Races) + len(o.Subraces) + len(o.Backgrounds) + len(o.Spells)
	if total > MaxOverlayEntries {
		return ovErr("", "%d entries; the limit is %d per table", total, MaxOverlayEntries).at("", ReasonLimit)
	}
	if err := checkBudgets(&o); err != nil {
		return err
	}
	b.entries, b.used = map[string]bool{}, map[string]bool{}
	b.strict = map[string]bool{}
	for _, k := range o.Strict {
		b.strict[k] = true
	}
	b.classListFrom, b.classCasts, b.classSubLevel = map[string]string{}, map[string]string{}, map[string]int{}
	spells := byKey(o.Spells, func(e *TableSpell) string { return e.Key })
	races := byKey(o.Races, func(e *TableRace) string { return e.Key })
	subraces := byKey(o.Subraces, func(e *TableSubrace) string { return e.Key })
	classes := byKey(o.Classes, func(e *TableClass) string { return e.Key })
	subclasses := byKey(o.Subclasses, func(e *TableSubclass) string { return e.Key })
	backgrounds := byKey(o.Backgrounds, func(e *TableBackground) string { return e.Key })
	if err := b.claimKeys(o, classes, subclasses, races, subraces, backgrounds, spells); err != nil {
		return err
	}

	b.n = b.base.cloneForOverlay()
	n := b.n
	n.table, n.tableRevision = true, o.Revision
	n.version = b.base.version + "+mesa." + strconv.Itoa(o.Revision)
	classIndexes := make([]string, 0, len(n.classes)+len(o.Classes))
	for k := range b.base.classes {
		classIndexes = append(classIndexes, strings.TrimPrefix(k, "class:"))
	}
	for _, tc := range o.Classes {
		classIndexes = append(classIndexes, strings.TrimPrefix(tc.Key, "class:"))
	}
	slices.Sort(classIndexes)
	n.compiler = formula.NewCompiler(classIndexes)
	n.programs = map[programKey]*formula.Program{}
	defer func() { n.programs = nil }()

	// An entry that fails reports everything wrong with it: its own fields, and the
	// effects of its features and traits (which are compiled once every entry exists).
	fail := func(err error, path string) error {
		err = locate(err, path)
		var oe *OverlayError
		if errors.As(err, &oe) && oe.Key != "" {
			if extra, ok := errors.AsType[*OverlayError](b.compileEffectsOf(oe.Key)); ok {
				oe.More = append(oe.More, extra.Violations()...)
			}
		}
		return err
	}
	for _, i := range spells {
		path := fmt.Sprintf("spells[%d]", i)
		if err := b.addSpell(&o.Spells[i], path); err != nil {
			return fail(err, path)
		}
	}
	for _, i := range races {
		path := fmt.Sprintf("races[%d]", i)
		if err := b.addRace(&o.Races[i], path); err != nil {
			return fail(err, path)
		}
	}
	for _, i := range subraces {
		path := fmt.Sprintf("subraces[%d]", i)
		if err := b.addSubrace(&o.Subraces[i], path); err != nil {
			return fail(err, path)
		}
	}
	for _, i := range classes {
		path := fmt.Sprintf("classes[%d]", i)
		if err := b.addClass(&o.Classes[i], path); err != nil {
			return fail(err, path)
		}
	}
	for _, i := range subclasses {
		path := fmt.Sprintf("subclasses[%d]", i)
		if err := b.addSubclass(&o.Subclasses[i], path); err != nil {
			return fail(err, path)
		}
	}
	for _, i := range backgrounds {
		path := fmt.Sprintf("backgrounds[%d]", i)
		if err := b.addBackground(&o.Backgrounds[i], path); err != nil {
			return fail(err, path)
		}
	}
	if err := b.compileEffects(); err != nil {
		return err
	}
	for _, k := range o.Off {
		if n.isSwitchable(k) {
			n.off[k] = true
		}
	}
	for i := range o.Classes {
		if err := n.indexClassCasting(o.Classes[i].Key); err != nil {
			return ovErr(o.Classes[i].Key, "%v", err).at(fmt.Sprintf("classes[%d]", i), ReasonCasting)
		}
	}
	n.buildCatalog(b.base.spellDetails)
	return nil
}

// claimKeys is the first pass: it checks the form of every entry key, that none
// repeats or exists, and learns the facts the references need.
func (b *overlayBuilder) claimKeys(o Overlay, classes, subclasses, races, subraces, backgrounds, spells []int) error {
	claim := func(prefix, path string, e TableEntry) error {
		if err := checkTableKey(prefix, e.Key); err != nil {
			return locate(err, path)
		}
		if err := b.claim(e.Key); err != nil {
			return locate(err, path)
		}
		b.entries[e.Key] = true
		return locate(checkEntryName(e.Key, e.NamePT), path)
	}
	for _, i := range classes {
		e := &o.Classes[i]
		if err := claim("class:", fmt.Sprintf("classes[%d]", i), e.TableEntry); err != nil {
			return err
		}
		b.classListFrom[e.Key] = e.Casting.ListFrom
		b.classCasts[e.Key] = e.Casting.Kind
		b.classSubLevel[e.Key] = e.SubclassLevel
	}
	for _, i := range subclasses {
		if err := claim("subclass:", fmt.Sprintf("subclasses[%d]", i), o.Subclasses[i].TableEntry); err != nil {
			return err
		}
	}
	for _, i := range races {
		if err := claim("race:", fmt.Sprintf("races[%d]", i), o.Races[i].TableEntry); err != nil {
			return err
		}
	}
	for _, i := range subraces {
		if err := claim("subrace:", fmt.Sprintf("subraces[%d]", i), o.Subraces[i].TableEntry); err != nil {
			return err
		}
	}
	for _, i := range backgrounds {
		if err := claim("background:", fmt.Sprintf("backgrounds[%d]", i), o.Backgrounds[i].TableEntry); err != nil {
			return err
		}
	}
	for _, i := range spells {
		if err := claim("spell:", fmt.Sprintf("spells[%d]", i), o.Spells[i].TableEntry); err != nil {
			return err
		}
	}
	return nil
}

// claim reserves a key: it must be new to the overlay and to the base.
func (b *overlayBuilder) claim(key string) error {
	if b.used[key] || b.base.exists(key) {
		return ovErr(key, "the key already exists")
	}
	b.used[key] = true
	return nil
}

// reservedStems are words in a slug that SRD logic reacts to by name: Magical
// Secrets (spells off the list), Wild Shape (play and the web treat the feature
// as the druid's) and the Ability Score Improvement (found by the key). A table
// key never has them, so a table feature can never be taken for the SRD's;
// the keys the engine writes (its Spellcasting and Ability Score Improvement
// features) carry the entry's kind, so two entries cannot make the same one.
var reservedStems = []string{"magical-secrets", "wild-shape", "ability-score-improvement"}

// checkTableKey checks "<prefix><slug>@mesa": the slug is 1 to 60 characters of
// [a-z0-9-].
func checkTableKey(prefix, key string) error {
	rest, ok := strings.CutPrefix(key, prefix)
	if !ok {
		return ovErr(key, "the key must start with %q", prefix)
	}
	slug, ok := strings.CutSuffix(rest, tableSuffix)
	if !ok {
		return ovErr(key, "the key must end in %q", tableSuffix)
	}
	if len(slug) < 1 || len(slug) > MaxSlugLength {
		return ovErr(key, "the slug must have 1 to %d characters", MaxSlugLength)
	}
	for _, r := range slug {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return ovErr(key, "the slug may only have a-z, 0-9 and -")
		}
	}
	for _, stem := range reservedStems {
		if strings.Contains(slug, stem) {
			return ovErr(key, "the slug cannot have %q: the engine reads it in SRD keys, so it is reserved", stem).reason(ReasonReservedKey)
		}
	}
	if strings.HasSuffix(slug, "-spellcasting") || slug == "spellcasting" {
		return ovErr(key, "the slug cannot end in \"spellcasting\": it is reserved for the Spellcasting feature the engine writes").reason(ReasonReservedKey)
	}
	return nil
}

// slugOfKey is the part of a key between its prefix and "@mesa".
func slugOfKey(key string) string {
	_, rest, _ := strings.Cut(key, ":")
	return strings.TrimSuffix(rest, tableSuffix)
}

// checkEntryName checks a Portuguese name: not empty, trimmed, one line, short.
func checkEntryName(key, name string) error {
	switch {
	case name == "" || strings.TrimSpace(name) != name:
		return ovErr(key, "the name must not be empty or start or end with spaces").reason(ReasonName)
	case utf8.RuneCountInString(name) > maxNameRunes:
		return ovErr(key, "the name has more than %d characters", maxNameRunes).reason(ReasonName)
	case strings.ContainsFunc(name, isHiddenRune):
		return ovErr(key, "the name must be one line, without control or invisible characters").reason(ReasonName)
	}
	return nil
}

// isHiddenRune says whether a one-line text must refuse r: a control character,
// a line or paragraph separator, or an invisible one (text direction, zero width).
func isHiddenRune(r rune) bool {
	return unicode.IsControl(r) || names.IsHidden(r)
}

// checkText bounds a text made of paragraphs.
func checkText(key string, paragraphs []string) error {
	if len(paragraphs) > maxTextParagraphs {
		return ovErr(key, "the text has more than %d paragraphs", maxTextParagraphs).reason(ReasonLimit)
	}
	for _, p := range paragraphs {
		if utf8.RuneCountInString(p) > maxTextRunes {
			return ovErr(key, "a paragraph has more than %d characters", maxTextRunes).reason(ReasonText)
		}
	}
	return nil
}

// isClass, isRace and isSpell say whether a key is one of the base's or one the
// overlay adds.
func (b *overlayBuilder) isClass(key string) bool {
	_, ok := b.base.classes[key]
	return ok || (b.entries[key] && strings.HasPrefix(key, "class:"))
}

func (b *overlayBuilder) isRace(key string) bool {
	_, ok := b.base.races[key]
	return ok || (b.entries[key] && strings.HasPrefix(key, "race:"))
}

func (b *overlayBuilder) isSpell(key string) bool {
	_, ok := b.base.spells[key]
	return ok || (b.entries[key] && strings.HasPrefix(key, "spell:"))
}

// register puts a name and the archived mark of an entry in the clone.
func (b *overlayBuilder) register(e TableEntry) {
	b.n.namesEN[e.Key] = e.NamePT
	b.n.namesPT[e.Key] = e.NamePT
	if e.Archived {
		b.n.archived[e.Key] = true
	}
	if e.Revision > 0 {
		b.n.entryRevision[e.Key] = e.Revision
		b.n.entryChangedAt[e.Key] = e.ChangedAt
	}
}

// cloneForOverlay is a content whose maps can be added to without touching c:
// the maps With changes are cloned, and the rest are shared (they are only
// read). The values in the maps are shared too, so an entry of c is never
// written: a change to one (a class gaining a subclass) writes a copy.
func (c *content) cloneForOverlay() *content {
	n := *c
	n.races = maps.Clone(c.races)
	n.subraces = maps.Clone(c.subraces)
	n.traits = maps.Clone(c.traits)
	n.classes = maps.Clone(c.classes)
	n.subclasses = maps.Clone(c.subclasses)
	n.features = maps.Clone(c.features)
	n.backgrounds = maps.Clone(c.backgrounds)
	n.spells = maps.Clone(c.spells)
	n.classLevels = maps.Clone(c.classLevels)
	n.subclassLevels = maps.Clone(c.subclassLevels)
	n.effects = maps.Clone(c.effects)
	n.casting = maps.Clone(c.casting)
	n.subCasting = maps.Clone(c.subCasting)
	n.namesPT = maps.Clone(c.namesPT)
	n.namesEN = maps.Clone(c.namesEN)
	n.listFrom = map[string]string{}
	n.offeredBy = map[string][]string{}
	n.archived = map[string]bool{}
	n.off = map[string]bool{}
	n.entryRevision = map[string]int{}
	n.entryChangedAt = map[string]time.Time{}
	n.spellTargets = map[string]SpellTarget{}
	n.raceChoice = map[string][]int{}
	n.bgEquipment = map[string]string{}
	n.catalog = Catalog{}
	n.spellEntries = nil
	n.spellDetails = nil
	return &n
}
