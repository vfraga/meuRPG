package rules

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/PuraFome/meuRPG/backend/internal/rules/srd51"
)

// The guided level-up (MR-040, RN-01, RN-12). Until the full level-up
// exists, a character that can level up may change its locked sheet, but
// only to add what the next level of one of its classes gives. Three pure
// functions say what that is:
//
//   - LevelUpOptions: what the next level of a class gives, for the screens;
//   - ApplyLevelUp: the Build that the player's choices make;
//   - CheckLevelUp: refuses every difference between the two Builds that the
//     level does not allow.
//
// Like the rest of the package they use no database, clock or randomness:
// the hit die is rolled elsewhere and passed in.

// Reasons of a LevelUpError. They are stable codes: the app maps each one to
// its own Portuguese copy, so nobody parses Message.
const (
	// LevelUpReasonClass: not exactly one class of the character, one level up,
	// or one new class at its first level, last in the list (multiclassing).
	LevelUpReasonClass = "class"
	// LevelUpReasonMulticlassPrerequisite: the new class asks for an ability the
	// character does not have at 13 (SRD 5.1, "Multiclassing", "Prerequisites").
	// ClassKey is the new class, and Ability, Minimum and Have the missing score.
	LevelUpReasonMulticlassPrerequisite = "multiclass_prerequisite"
	// LevelUpReasonMulticlassPrerequisiteCurrent: a class the character already has
	// asks for an ability the character does not have at 13, which closes every new
	// class to it. ClassKey is the class the character has.
	LevelUpReasonMulticlassPrerequisiteCurrent = "multiclass_prerequisite_current"
	// LevelUpReasonProficiencyChoice: not exactly the skill the new class's
	// multiclass proficiencies let the player choose, or one that is not on the
	// class's list or that the character already has.
	LevelUpReasonProficiencyChoice = "proficiency_choice"
	// LevelUpReasonInstrumentChoice: not exactly the musical instrument the bard's
	// multiclass proficiencies let the player choose, or one the character already
	// has.
	LevelUpReasonInstrumentChoice = "instrument_choice"
	// LevelUpReasonMaxLevel: the character is already level 20.
	LevelUpReasonMaxLevel = "max_level"
	// LevelUpReasonLocked: a field the level does not change (name, race, base
	// scores, equipment...) is different. Field says which.
	LevelUpReasonLocked = "locked_field"
	// LevelUpReasonAbilityNotDue: an ability increase at a level that has none.
	LevelUpReasonAbilityNotDue = "ability_not_due"
	// LevelUpReasonAbilityShape: the increase is not +2 in one ability or +1 in two.
	LevelUpReasonAbilityShape = "ability_shape"
	// LevelUpReasonAbilityAbove20: the increase takes an ability above 20.
	LevelUpReasonAbilityAbove20 = "ability_above_20"
	// LevelUpReasonFeat: a feat at a level that has no Ability Score Improvement, a
	// feat removed or more than one, or a feat the content does not have.
	LevelUpReasonFeat = "feat"
	// LevelUpReasonFeatPrerequisite: the character does not meet the feat's
	// prerequisite.
	LevelUpReasonFeatPrerequisite = "feat_prerequisite"
	// LevelUpReasonHitPoints: the new hit points entry is missing, outside 1 to
	// the hit die, or the earlier levels changed.
	LevelUpReasonHitPoints = "hit_points"
	// LevelUpReasonSubclass: a subclass that is not due, or is due and is missing.
	LevelUpReasonSubclass = "subclass"
	// LevelUpReasonCantrips: not exactly the new cantrips the level gives, or one
	// was removed.
	LevelUpReasonCantrips = "cantrips"
	// LevelUpReasonSpells: not exactly the new known (or spellbook) spells the
	// level gives, or one was removed.
	LevelUpReasonSpells = "spells"
	// LevelUpReasonPrepared: a prepared spell was removed, or one was added by a
	// class that does not prepare.
	LevelUpReasonPrepared = "prepared"
	// LevelUpReasonFeatureChoice: not exactly the options the new features offer.
	LevelUpReasonFeatureChoice = "feature_choice"
	// LevelUpReasonLateChoice: a choice an earlier level left open (a fighting
	// style the sheet never picked) is still open.
	LevelUpReasonLateChoice = "late_choice"
	// LevelUpReasonSkills: not exactly the skills the new level lets the player
	// choose.
	LevelUpReasonSkills = "skills"
	// LevelUpReasonExpertise: not exactly the expertise the new level gives.
	LevelUpReasonExpertise = "expertise"
	// LevelUpReasonSheetIssue: the new sheet has a problem the old one did not
	// (a spell off the class list or above the circle, too many prepared
	// spells...). Code is the Issue code and Field the Issue's field.
	LevelUpReasonSheetIssue = "sheet_issue"
)

// LevelUpError is what ApplyLevelUp, LevelUpOptions and CheckLevelUp return
// for something the level does not allow. Field names the sheet field with
// the CharacterSheet proto's field names ("full.base_scores.strength"), and
// Reason is one of the LevelUpReason* codes.
type LevelUpError struct {
	Field  string
	Reason string
	// Code is the Issue code, for LevelUpSheetIssue only.
	Code string
	// Message is English text for logs and tests, never for the app.
	Message string
	// ClassKey, Ability, Minimum and Have say which multiclass prerequisite is not
	// met (the two LevelUpReasonMulticlassPrerequisite reasons). Have is the
	// character's final score, Minimum the score the class asks for.
	ClassKey string
	Ability  Ability
	Minimum  int
	Have     int
}

func (e *LevelUpError) Error() string {
	return e.Field + ": " + e.Reason + ": " + e.Message
}

func refuse(field, reason, format string, args ...any) *LevelUpError {
	return &LevelUpError{Field: field, Reason: reason, Message: fmt.Sprintf(format, args...)}
}

// LevelUpOffer is what one class's next level gives, in data the screens
// can draw. Everything the player chooses is in the "to choose" part; the
// rest is automatic and only shown.
type LevelUpOffer struct {
	// Class is the class key and ClassNamePT its name. FromLevel and ToLevel
	// are its levels, and TotalFrom and TotalTo the character's.
	Class       string
	ClassNamePT string
	FromLevel   int
	ToLevel     int
	TotalFrom   int
	TotalTo     int
	// NewClass says the character does not have the class yet: the level is its
	// first (FromLevel 0), and Multiclass says what taking it as a later class
	// gives and asks for.
	NewClass   bool
	Multiclass *MulticlassOffer

	// HitDie is the class's die (8 for a d8) and HitPointAverage the fixed
	// gain: half the die plus one, before the Constitution modifier.
	HitDie          int
	HitPointAverage int

	// To choose.

	// AbilityScoreImprovement says this level has one: +2 in one ability or
	// +1 in two, none above 20. When the table plays with feats (an optional
	// rule), the player may take a feat from Feats in its place.
	AbilityScoreImprovement bool
	// Feats are the feats of the content with whether the character qualifies at
	// the new level, at a level with an Ability Score Improvement; empty at the
	// others. The server leaves them out when the table does not use feats.
	Feats []FeatOption
	// SubclassDue says the class picks its subclass now, from Subclasses.
	SubclassDue bool
	Subclasses  []LevelUpSubclass
	// Cantrips is how many new cantrips to add, from the class's list.
	Cantrips int
	// Spells is how many new spells to add: to the spellbook (wizard) or to
	// the spells known (bard, ranger, sorcerer, warlock). SpellsKind says
	// which (PreparationSpellbook or PreparationKnown), "" for none.
	Spells     int
	SpellsKind string
	// SpellList is the class key whose list the new spells come from, and
	// MaxSpellLevel the highest circle they may have. Both are empty/zero
	// when the class casts nothing at the new level.
	SpellList     string
	MaxSpellLevel int
	// AnyClassSpells is how many of Spells may come from any class's list, not
	// only SpellList's: the Bard's Magical Secrets (levels 10, 14 and 18).
	// They must still be of a circle the class can cast.
	AnyClassSpells int
	// Prepares says the class prepares spells. PreparedMax is the maximum
	// now, and PreparedMaxAfter the one with the new level and the
	// abilities as they are; an ability increase can raise it, so the
	// screens ask PreviewLevelUp for the final number.
	Prepares         bool
	PreparedMax      int
	PreparedMaxAfter int
	// FeatureChoices are the options the features of the new level offer
	// (a fighting style, a metamagic), for the class's own features and,
	// when the subclass is already chosen, the subclass's.
	FeatureChoices []LevelUpFeatureChoice
	// SkillChoices and ExpertiseChoices are how many new skills and
	// expertise the new level lets the player pick (the Bard's Lore
	// college, the Rogue's Expertise).
	SkillChoices     int
	ExpertiseChoices int
	// LateChoices are the choices an earlier level left open (a sheet written
	// before they were asked, or saved with one skipped): the level up asks for
	// them first and refuses without them. NewChoices are the new level's choices
	// that are not feature options (a favored enemy, a terrain, a Mystic Arcanum),
	// which FeatureChoices does not carry.
	LateChoices []ChoiceGroup
	NewChoices  []ChoiceGroup
	// CanSwapInvocation says the class is the Warlock's and the sheet has an
	// invocation to swap: one may be replaced at each level (SRD 5.1, Warlock,
	// Eldritch Invocations).
	CanSwapInvocation bool

	// Automatic.

	// ProficiencyBonusBefore and After; SlotsBefore and After are the
	// spell slots per circle (index 0 is the 1st circle); PactBefore and
	// After are the warlock's pact slots, nil for none.
	ProficiencyBonusBefore int
	ProficiencyBonusAfter  int
	SlotsBefore            [9]int
	SlotsAfter             [9]int
	PactBefore             *PactMagic
	PactAfter              *PactMagic
	// NewFeatures are the class features the level gives by name.
	NewFeatures []NamedKey
	// MasterAdds are the features of the level whose choice the guided level-up
	// does not cover (Mystic Arcanum, Spell Mastery, Signature Spells,
	// Additional Magical Secrets, favored enemies and terrains): the master
	// adds them in the sheet editor.
	MasterAdds []NamedKey
}

// LevelUpSubclass is a subclass the player may pick, with the options its
// features at the new level offer.
type LevelUpSubclass struct {
	Key, NamePT    string
	FeatureChoices []LevelUpFeatureChoice
	// What this subclass adds to the level's counts: cantrips (the Land
	// Druid's bonus cantrip), skills (the College of Lore's three) and
	// expertise. The top-level counts of the offer leave them out until the
	// subclass is chosen.
	Cantrips, SkillChoices, ExpertiseChoices int
	// Spells is how many spells a third caster's subclass adds (known ones, with
	// SpellsKind PreparationKnown); the table's subclass that casts is the only one.
	Spells     int
	SpellsKind string
	// SpellList is the class whose list those spells come from, and
	// MaxSpellLevel the highest circle they may have (zero values for a subclass
	// that does not cast).
	SpellList     string
	MaxSpellLevel int
	// Prepares says a third caster's subclass prepares its spells, and
	// PreparedMaxAfter is how many it may prepare at the new level once chosen.
	Prepares         bool
	PreparedMaxAfter int
	// Archived says the table retired this subclass (see RaceEntry.Archived):
	// the screen does not offer it as a new choice.
	Archived bool
	// Off says the master switched this subclass off for the players (RN-23): its own
	// switch. The offer is for a character that has the class, so an off class does
	// not hide it.
	Off bool
}

// LevelUpFeatureChoice is "choose Choose of Options" for a feature gained
// at the new level. Subclass is the key of the subclass the feature belongs
// to, or empty for a class feature.
type LevelUpFeatureChoice struct {
	Feature  NamedKey
	Subclass string
	Choose   int
	Options  []NamedKey
	// Blocked are the options the character cannot take now (an invocation whose
	// prerequisite is unmet), with the reason; they are not in Options.
	Blocked []LevelUpBlockedOption
}

// LevelUpBlockedOption is an option that stays on the screen, with why it cannot be
// taken.
type LevelUpBlockedOption struct {
	Key, NamePT, ReasonPT string
}

// LevelUpChoices is what the player chose for one level up, in content
// keys. The lists hold only what is new, never the whole sheet.
type LevelUpChoices struct {
	// Class is the class key that gains the level.
	Class string
	// AbilityIncrease is +2 in one ability or +1 in two; empty for none. With a
	// Feat, it is the increase the feat gives (its ability_increase effect), or
	// empty when it gives none.
	AbilityIncrease map[Ability]int
	// Feat is the feat taken in place of the Ability Score Improvement, or empty.
	Feat string
	// Subclass is the subclass key, when one is due.
	Subclass string
	// The new cantrips, known (or spellbook) spells and prepared spells.
	Cantrips []string
	Spells   []string
	Prepared []string
	// FeatureChoices are the new options, SkillProficiencies the new skills
	// and Expertise the new expertise.
	FeatureChoices     []string
	SkillProficiencies []string
	Expertise          []string
	// LateChoices are the picks of the choices an earlier level left open, and
	// FeatureChoiceText the free text some picks take (Build.FeatureChoiceText).
	LateChoices       []string
	FeatureChoiceText map[string]string
	// SwapInvocation is an Eldritch Invocation the warlock gives up for another
	// from FeatureChoices (one at each level of the class).
	SwapInvocation string
	// HitPoints is the gain of this level.
	HitPoints LevelUpHitPoints
	// Instrument is the musical instrument (a proficiency key) that taking the Bard
	// as a later class lets the player pick; the sheet lists its name among the
	// tool proficiencies.
	Instrument string
}

// LevelUpHitPoints is the hit points of the new level: the fixed average,
// or a die result (rolled in the app or typed from a physical die, which
// the rules cannot tell apart).
type LevelUpHitPoints struct {
	Average bool
	// Roll is the die result, 1 to the hit die; ignored with Average.
	Roll int
}

// clone copies a Build deeply, so the level-up never changes the caller's.
func (b Build) clone() Build {
	b.BaseScores = maps.Clone(b.BaseScores)
	b.ExtraAbilityBonuses = maps.Clone(b.ExtraAbilityBonuses)
	b.Classes = slices.Clone(b.Classes)
	b.CustomBackgroundSkills = slices.Clone(b.CustomBackgroundSkills)
	b.CustomBackgroundProficiencies = slices.Clone(b.CustomBackgroundProficiencies)
	b.SkillProficiencies = slices.Clone(b.SkillProficiencies)
	b.Expertise = slices.Clone(b.Expertise)
	b.Weapons = slices.Clone(b.Weapons)
	b.Cantrips = slices.Clone(b.Cantrips)
	b.SpellsKnown = slices.Clone(b.SpellsKnown)
	b.SpellsPrepared = slices.Clone(b.SpellsPrepared)
	b.FeatureChoices = slices.Clone(b.FeatureChoices)
	b.FeatureChoiceText = maps.Clone(b.FeatureChoiceText)
	b.ToolProficiencies = slices.Clone(b.ToolProficiencies)
	b.Feats = slices.Clone(b.Feats)
	b.FeatSlots = maps.Clone(b.FeatSlots)
	b.HitPoints.Rolls = slices.Clone(b.HitPoints.Rolls)
	return b
}

func (b Build) totalLevel() int {
	n := 0
	for _, cl := range b.Classes {
		n += cl.Level
	}
	return n
}

// levelUpClass finds the class that gains the level, and refuses a class the
// content does not have, or a character at level 20. A class the character does
// not have is a new class (multiclassing): the index is past the last class, and
// the class must pass the prerequisites (see multiclassAllowed).
func levelUpClass(b Build, classKey string, c *content) (int, *LevelUpError) {
	idx := slices.IndexFunc(b.Classes, func(cl ClassLevel) bool { return cl.Class == classKey })
	if _, known := c.classes[classKey]; !known {
		return 0, refuse("full.classes", LevelUpReasonClass, "no such class")
	}
	if idx < 0 {
		if lerr := c.multiclassAllowed(b, classKey); lerr != nil {
			return 0, lerr
		}
		return len(b.Classes), nil
	}
	if b.totalLevel() >= MaxLevel || b.Classes[idx].Level >= MaxLevel {
		return 0, refuse(fmt.Sprintf("full.classes[%d].level", idx), LevelUpReasonMaxLevel, "the character is already level %d", MaxLevel)
	}
	return idx, nil
}

// LevelUpOptions says what the next level of class classKey gives a
// character built as before. A class the character does not have is a new
// class at its level 1, if the character meets the prerequisites (the
// LevelUpMulticlassPrerequisite refusals); it refuses a class the content does
// not have (LevelUpClass) and a character at level 20 (LevelUpMaxLevel).
func LevelUpOptions(before Build, classKey string, c *Content) (LevelUpOffer, error) {
	o, err := levelUpOptions(before, classKey, c.c)
	if err != nil {
		return LevelUpOffer{}, err
	}
	return o, nil
}

func levelUpOptions(b Build, classKey string, c *content) (LevelUpOffer, *LevelUpError) {
	idx, lerr := levelUpClass(b, classKey, c)
	if lerr != nil {
		return LevelUpOffer{}, lerr
	}
	var sub *srd51.Subclass
	if idx < len(b.Classes) {
		sub = c.subclasses[b.Classes[idx].Subclass]
	}
	o, _ := levelUpOptionsWith(b, idx, classKey, c, sub)
	return o, nil
}

// levelUpOptionsWith is levelUpOptions as if the class had subclass sub at the
// new level: the counts of the top level then include what that subclass
// gives. CheckLevelUp calls it with the subclass the new sheet chose, so the
// options and the check read the same numbers; the options themselves call it
// with the subclass the character already has, and give each candidate
// subclass its own part (LevelUpSubclass).
func levelUpOptionsWith(b Build, idx int, classKey string, c *content, sub *srd51.Subclass) (LevelUpOffer, *LevelUpError) {
	// idx past the last class is a class the character does not have: its first level.
	newClass := idx >= len(b.Classes)
	cl := ClassLevel{Class: classKey}
	if !newClass {
		cl = b.Classes[idx]
	}
	class := c.classes[classKey]
	newLevel := cl.Level + 1

	// The level with nothing chosen yet: what follows from the level alone.
	bare := bareLevelUp(b, idx, classKey, newLevel, sub)
	dBefore, dBare := derive(b, c), derive(bare, c)

	o := LevelUpOffer{
		Class: classKey, ClassNamePT: c.namePT(classKey),
		FromLevel: cl.Level, ToLevel: newLevel, TotalFrom: b.totalLevel(), TotalTo: b.totalLevel() + 1,
		HitDie: class.HitDie, HitPointAverage: class.HitDie/2 + 1,
		ProficiencyBonusBefore: dBefore.ProficiencyBonus, ProficiencyBonusAfter: dBare.ProficiencyBonus,
		PactBefore: dBefore.PactMagic, PactAfter: dBare.PactMagic,
		NewClass: newClass,
	}
	if newClass {
		o.Multiclass = c.multiclassOffer(b, dBefore, bare, classKey)
	}
	copy(o.SlotsBefore[:], dBefore.SpellSlots)
	copy(o.SlotsAfter[:], dBare.SpellSlots)

	row := c.classLevels[classKey][newLevel-1]
	o.AbilityScoreImprovement = isASILevel(row)
	if o.AbilityScoreImprovement {
		o.Feats = featOptions(bare, c)
	}

	// Features the level adds, by name; and the ones the guided flow does not
	// cover, which the master adds in the editor.
	o.NewFeatures = newClassFeatures(dBefore, dBare, classKey)
	classFeatures, subFeatures := c.levelFeatures(classKey, sub, newLevel)
	o.addFeatureNotes(c, slices.Concat(classFeatures, subFeatures))

	// What the class gives, and the subclass when it is already chosen.
	gains := c.levelGains(classKey, sub, newLevel)
	o.FeatureChoices = c.namedChoices(b, gains.choices)
	o.SkillChoices, o.ExpertiseChoices = gains.skills, gains.expertise

	// A subclass that is due: each candidate's own part. It is read before the
	// spells below, so a third caster's part compares with counts still at zero.
	if subclassDue(cl, class, newLevel) {
		o.addSubclasses(c, b, idx, class, sub)
	}

	o.addSpells(c, dBefore, dBare, sub, gains.cantrips)
	o.addMagicalSecrets(sub, subFeatures)

	// The choices the engine asks: the ones an earlier level left open, and the new
	// level's that are not feature options.
	o.askChoices(c, b, bare, classKey)
	return o, nil
}

// bareLevelUp is b with the class one level higher (a new class added at
// newLevel) and nothing chosen; with sub as its subclass when it has none, since
// a third caster casts through its subclass and the level with the subclass
// chosen has its casting numbers.
func bareLevelUp(b Build, idx int, classKey string, newLevel int, sub *srd51.Subclass) Build {
	bare := b.clone()
	if idx >= len(b.Classes) {
		bare.Classes = append(bare.Classes, ClassLevel{Class: classKey, Level: newLevel})
	} else {
		bare.Classes[idx].Level = newLevel
	}
	if sub != nil && bare.Classes[idx].Subclass == "" {
		bare.Classes[idx].Subclass = sub.Key
	}
	return bare
}

// subclassDue says the class picks its subclass at newLevel: it has none yet, nor
// a custom one the master set.
func subclassDue(cl ClassLevel, class *srd51.Class, newLevel int) bool {
	return cl.Subclass == "" && cl.CustomSubclassName == "" && class.SubclassLevel == newLevel
}

// newClassFeatures are the features of the class that the sheet at the new level
// has and the sheet before did not, by name.
func newClassFeatures(dBefore, dBare Derived, classKey string) []NamedKey {
	had := map[string]bool{}
	for _, f := range dBefore.Features {
		had[f.Key] = true
	}
	var out []NamedKey
	for _, f := range dBare.Features {
		if !had[f.Key] && f.Source == classKey {
			out = append(out, NamedKey{Key: f.Key, NamePT: f.NamePT})
		}
	}
	return out
}

// magicalSecretsSpells is how many spells of any class one Magical Secrets
// feature of the Bard lets the player learn (SRD 5.1, Bard).
const magicalSecretsSpells = 2

// addFeatureNotes reads the features of the new level: the ones the master adds in
// the editor, and the Bard's Magical Secrets of the SRD.
func (o *LevelUpOffer) addFeatureNotes(c *content, features []string) {
	for _, fk := range features {
		if masterAdds[fk] {
			o.MasterAdds = append(o.MasterAdds, NamedKey{Key: fk, NamePT: c.namePT(fk)})
		}
		if strings.HasPrefix(fk, "feature:magical-secrets-") && !isTableKey(fk) {
			o.AnyClassSpells = magicalSecretsSpells
		}
	}
}

// addSubclasses lists the subclasses the class may pick at the new level, each
// with its own part of the level, sorted by name.
func (o *LevelUpOffer) addSubclasses(c *content, b Build, idx int, class *srd51.Class, sub *srd51.Subclass) {
	o.SubclassDue = true
	for _, key := range class.Subclasses {
		if s, ok := c.subclasses[key]; ok {
			o.Subclasses = append(o.Subclasses, o.subclassPart(c, b, idx, s, sub))
		}
	}
	slices.SortFunc(o.Subclasses, func(a, b LevelUpSubclass) int { return comparePT(a.NamePT, b.NamePT) })
}

// subclassPart is what candidate subclass s adds to the level: its feature
// choices and counts and, for a third caster when no subclass is assumed (sub
// nil), its casting numbers.
func (o *LevelUpOffer) subclassPart(c *content, b Build, idx int, s, sub *srd51.Subclass) LevelUpSubclass {
	key := s.Key
	g := c.subclassGains(s, o.ToLevel)
	ls := LevelUpSubclass{
		Key: key, NamePT: c.namePT(key), Archived: c.archived[key], Off: c.off[key], FeatureChoices: c.namedChoices(b, g.choices),
		Cantrips: g.cantrips, SkillChoices: g.skills, ExpertiseChoices: g.expertise,
	}
	if _, third := c.subCasting[key]; third && sub == nil {
		// A third caster's table starts with the subclass: what it adds to
		// the level's counts is the difference with the class alone.
		with, _ := levelUpOptionsWith(b, idx, o.Class, c, s)
		ls.Cantrips += max(with.Cantrips-o.Cantrips, 0)
		ls.Spells, ls.SpellsKind = max(with.Spells-o.Spells, 0), with.SpellsKind
		ls.SpellList, ls.MaxSpellLevel = with.SpellList, with.MaxSpellLevel
		ls.Prepares, ls.PreparedMaxAfter = with.Prepares, with.PreparedMaxAfter
	}
	return ls
}

// spellbookSpellsPerLevel is how many spells a Wizard adds to the spellbook at
// each level after the first (SRD 5.1, Wizard, "The Book").
const spellbookSpellsPerLevel = 2

// addSpells fills the spells of the level: the difference between the class's
// rows (or the spellbook's two per level), plus the cantrips the features add.
func (o *LevelUpOffer) addSpells(c *content, dBefore, dBare Derived, sub *srd51.Subclass, extraCantrips int) {
	before, after := spellcastingOf(dBefore, o.Class), spellcastingOf(dBare, o.Class)
	if after == nil {
		o.Cantrips = extraCantrips
		return
	}
	cast, _ := c.castingFor(o.Class, sub)
	o.SpellList, o.MaxSpellLevel = cast.list, after.MaxSpellLevel
	oldCantrips, oldKnown := 0, 0
	if before != nil {
		oldCantrips, oldKnown = before.CantripsKnown, before.SpellsKnownMax
		o.PreparedMax = before.PreparedMax
	}
	o.Cantrips = max(after.CantripsKnown-oldCantrips, 0) + extraCantrips
	o.Spells, o.SpellsKind = newSpellsOf(preparation(cast.effect), o.NewClass, before != nil, after.SpellsKnownMax-oldKnown)
	o.Prepares = after.PreparesSpells
	o.PreparedMaxAfter = after.PreparedMax
}

// newSpellsOf is how many spells the level adds, and to what: a Wizard taken as a
// later class starts its spellbook (SRD 5.1, Wizard, "Spellbook"), one that
// already casts adds two, and a class that knows its spells adds the growth of
// its known spells (knownGain).
func newSpellsOf(kind string, newClass, casts bool, knownGain int) (int, string) {
	switch kind {
	case PreparationSpellbook:
		switch {
		case newClass:
			return wizardStartingSpells, PreparationSpellbook
		case casts:
			return spellbookSpellsPerLevel, PreparationSpellbook
		}
	case PreparationKnown:
		return max(knownGain, 0), PreparationKnown
	}
	return 0, ""
}

// addMagicalSecrets adds the College of Lore's Additional Magical Secrets (SRD 5.1,
// Bard, Lore): two more spells of any class, outside the number the Bard knows. The
// spells of any class never exceed the spells the level gives.
func (o *LevelUpOffer) addMagicalSecrets(sub *srd51.Subclass, subFeatures []string) {
	if sub != nil && slices.Contains(subFeatures, "feature:additional-magical-secrets") {
		o.Spells += magicalSecretsSpells
		o.SpellsKind = PreparationKnown
		o.AnyClassSpells += magicalSecretsSpells
	}
	o.AnyClassSpells = min(o.AnyClassSpells, o.Spells)
}

// askChoices fills what the choice engine asks of the level: the choices an earlier
// level left open (b is the sheet before), the new level's that are not feature
// options (bare is the sheet at the new level), the options that cannot be taken yet
// and whether an invocation may be swapped. For a class the character does not have
// yet, bare holds its level 1, so its first-level choices are asked here.
func (o *LevelUpOffer) askChoices(c *content, b, bare Build, classKey string) {
	asked, atNewLevel := c.choiceSet(b), c.choiceSet(bare)
	o.LateChoices = pendingGroups(asked)
	o.NewChoices = newScopedGroups(asked, atNewLevel)
	for i := range o.FeatureChoices {
		o.FeatureChoices[i] = blockOptions(o.FeatureChoices[i], atNewLevel)
	}
	o.CanSwapInvocation = classKey == classWarlock && slices.ContainsFunc(b.FeatureChoices, c.isInvocation)
}

// blockOptions moves the options of a due choice that the sheet at the new level
// cannot take (an invocation whose prerequisite is unmet) from Options to Blocked.
func blockOptions(fc LevelUpFeatureChoice, atNewLevel ChoiceSet) LevelUpFeatureChoice {
	reasons := blockedReasons(fc.Feature.Key, atNewLevel)
	var takable []NamedKey
	for _, o := range fc.Options {
		if reason, blocked := reasons[o.Key]; blocked {
			fc.Blocked = append(fc.Blocked, LevelUpBlockedOption{Key: o.Key, NamePT: o.NamePT, ReasonPT: reason})
			continue
		}
		takable = append(takable, o)
	}
	fc.Options = takable
	return fc
}

// blockedReasons are the options of the choice with this key that the sheet at the
// new level cannot take, with why; only plain options (whose key is the one stored).
func blockedReasons(choiceKey string, atNewLevel ChoiceSet) map[string]string {
	reasons := map[string]string{}
	for _, g := range atNewLevel.Groups {
		for _, ch := range g.Choices {
			if ch.Key == choiceKey {
				addBlockedReasons(reasons, ch.Options)
			}
		}
	}
	return reasons
}

// addBlockedReasons adds to reasons the plain options that carry a reason.
func addBlockedReasons(reasons map[string]string, options []ChoiceOption) {
	for _, o := range options {
		if o.ReasonPT != "" && o.Key == o.Stored {
			reasons[o.Key] = o.ReasonPT
		}
	}
}

// isInvocation says whether a key is one of the Warlock's Eldritch Invocations.
func (c *content) isInvocation(key string) bool {
	inv := c.features[invocationsFeature]
	return inv != nil && slices.Contains(inv.Options, key)
}

// pendingGroups keeps, of a ChoiceSet, the choices with selections left to make.
func pendingGroups(s ChoiceSet) []ChoiceGroup {
	var out []ChoiceGroup
	for _, g := range s.Groups {
		var open []Choice
		for _, ch := range g.Choices {
			if ch.Missing() > 0 {
				open = append(open, ch)
			}
		}
		if len(open) > 0 {
			g.Choices = open
			out = append(out, g)
		}
	}
	return out
}

// newScopedGroups keeps the choices the new level adds (not in before) whose picks
// are not plain feature options: the options of LevelUpOffer.FeatureChoices, and the
// bonus cantrips of LevelUpOffer.Cantrips, are asked there.
func newScopedGroups(before, after ChoiceSet) []ChoiceGroup {
	had := map[string]bool{}
	for _, g := range before.Groups {
		for _, ch := range g.Choices {
			had[ch.Key] = true
		}
	}
	var out []ChoiceGroup
	for _, g := range after.Groups {
		var fresh []Choice
		for _, ch := range g.Choices {
			if had[ch.Key] || ch.Missing() == 0 || ch.plain || ch.viaCantrips {
				continue
			}
			fresh = append(fresh, ch)
		}
		if len(fresh) > 0 {
			g.Choices = fresh
			out = append(out, g)
		}
	}
	return out
}

// spellcastingOf finds a class's casting numbers, or nil.
func spellcastingOf(d Derived, classKey string) *Spellcasting {
	for i := range d.Spellcasting {
		if d.Spellcasting[i].Class == classKey {
			return &d.Spellcasting[i]
		}
	}
	return nil
}

// isASILevel says whether a class table row has an Ability Score
// Improvement.
// asiFeatureKey is the Ability Score Improvement feature of a class row, or "".
func asiFeatureKey(row *srd51.Level) string {
	if row == nil {
		return ""
	}
	for _, k := range row.Features {
		if strings.Contains(k, "-ability-score-improvement-") {
			return k
		}
	}
	return ""
}

// fixedIncrease is what a feat that raises every ability it lists adds to a sheet: its value
// to each, and no more than the room left under 20.
func fixedIncrease(inc *FeatIncrease, d Derived) map[Ability]int {
	out := map[Ability]int{}
	for _, s := range d.Abilities {
		if slices.Contains(inc.From, s.Ability) {
			if n := min(inc.Value, MaxNormalScore-s.Score); n > 0 {
				out[s.Ability] = n
			}
		}
	}
	return out
}

func isASILevel(row *srd51.Level) bool {
	return row != nil && slices.ContainsFunc(row.Features, func(k string) bool {
		return strings.Contains(k, "-ability-score-improvement-")
	})
}

// masterAdds are the features that ask the player for a choice the guided
// level-up cannot ask for. It lists them (LevelUpOffer.MasterAdds) and the
// master adds them in the editor (RN-12). None today: the choice engine asks
// for them all (choicegroups.go).
var masterAdds = map[string]bool{}

// invocationsFeature is the Warlock's Eldritch Invocations. How many the
// character knows comes from the class table (invocations_known), level by
// level, not from the feature, which only exists at level 2.
const invocationsFeature = "feature:eldritch-invocations"

// levelFeatures lists the keys of the features that a class, and a subclass
// when it has one, gain at a class level. Class features come first.
func (c *content) levelFeatures(classKey string, sub *srd51.Subclass, level int) (classFeatures, subclassFeatures []string) {
	if rows := c.classLevels[classKey]; level >= 1 && level <= len(rows) {
		classFeatures = rows[level-1].Features
	}
	if sub != nil {
		if row := c.subclassLevels[sub.Key][level]; row != nil {
			subclassFeatures = row.Features
		}
	}
	return classFeatures, subclassFeatures
}

// dueChoice is "choose choose of options" for a feature gained at a level.
type dueChoice struct {
	feature  string
	subclass string
	choose   int
	options  []string
}

// levelGains is what a set of features asks the player to choose.
type levelGains struct {
	choices   []dueChoice
	skills    int
	expertise int
	cantrips  int
}

func (g levelGains) plus(o levelGains) levelGains {
	return levelGains{
		choices: slices.Concat(g.choices, o.choices), skills: g.skills + o.skills,
		expertise: g.expertise + o.expertise, cantrips: g.cantrips + o.cantrips,
	}
}

// featureGains reads the choices of the features with these keys: their
// options (the feature's options_choose or, for one that does not say how
// many, the hand-written choice effect's count; a feature with neither is not
// enforced), the skills and expertise they give, and the extra cantrips.
func (c *content) featureGains(keys []string, subclass string) levelGains {
	var g levelGains
	for _, fk := range keys {
		if f := c.features[fk]; f != nil {
			c.addFeatureGains(&g, fk, f, subclass)
		}
	}
	return g
}

// addFeatureGains adds to g what feature f (key fk) asks the player to choose.
func (c *content) addFeatureGains(g *levelGains, fk string, f *srd51.Feature, subclass string) {
	g.expertise += f.ExpertiseChoices
	choose := f.OptionsChoose
	for _, e := range c.effects[fk] {
		if e.Type == "choice" {
			choose = g.addChoiceEffect(fk, e, choose)
		}
	}
	if fk != invocationsFeature && len(f.Options) > 0 && choose > 0 {
		g.choices = append(g.choices, dueChoice{feature: fk, subclass: subclass, choose: choose, options: f.Options})
	}
}

// addChoiceEffect adds a choice effect of feature fk to g, and returns how many
// of the feature's options to choose: the data's count, or the effect's when the
// data says none.
func (g *levelGains) addChoiceEffect(fk string, e *Effect, choose int) int {
	switch e.Choice {
	case "feature":
		if choose == 0 {
			choose = e.Count
		}
	case "expertise":
		// An SRD feature says its expertise in the data (ExpertiseChoices); the
		// table's say it in a choice effect.
		if isTableKey(fk) {
			g.expertise += e.Count
		}
	case "skill":
		g.skills += e.Count
	case "cantrip":
		g.cantrips += e.Count
	}
	return choose
}

// classGains is featureGains for a class level, plus the invocations the
// class table adds at that level.
func (c *content) classGains(classKey string, level int) levelGains {
	classFeatures, _ := c.levelFeatures(classKey, nil, level)
	g := c.featureGains(classFeatures, "")
	if rows := c.classLevels[classKey]; level >= 1 && level <= len(rows) {
		now, before := invocationsKnown(rows[level-1]), 0
		if level > 1 {
			before = invocationsKnown(rows[level-2])
		}
		if f := c.features[invocationsFeature]; f != nil && now > before && len(f.Options) > 0 {
			g.choices = append(g.choices, dueChoice{feature: invocationsFeature, choose: now - before, options: f.Options})
		}
	}
	return g
}

// levelGains is classGains plus, when the class has subclass sub, subclassGains.
func (c *content) levelGains(classKey string, sub *srd51.Subclass, level int) levelGains {
	gains := c.classGains(classKey, level)
	if sub != nil {
		gains = gains.plus(c.subclassGains(sub, level))
	}
	return gains
}

// subclassGains is featureGains for a subclass's features at a class level.
func (c *content) subclassGains(sub *srd51.Subclass, level int) levelGains {
	_, subFeatures := c.levelFeatures("", sub, level)
	return c.featureGains(subFeatures, sub.Key)
}

// invocationsKnown reads the class table's invocations_known column.
func invocationsKnown(row *srd51.Level) int {
	var cs struct {
		Known int `json:"invocations_known"`
	}
	if len(row.ClassSpecific) == 0 || json.Unmarshal(row.ClassSpecific, &cs) != nil {
		return 0
	}
	return cs.Known
}

// namedChoices is the due choices with names, leaving out the options the
// character already has.
func (c *content) namedChoices(b Build, due []dueChoice) []LevelUpFeatureChoice {
	var out []LevelUpFeatureChoice
	// An option is taken once, whichever feature offered it: the Defense style of
	// the Paladin is the Fighter's Defense (SRD 5.1, Fighter, "Fighting Style").
	taken := map[string]bool{}
	for _, k := range b.FeatureChoices {
		if f := c.features[k]; f != nil {
			taken[f.Name] = true
		}
	}
	for _, d := range due {
		fc := LevelUpFeatureChoice{Feature: NamedKey{Key: d.feature, NamePT: c.namePT(d.feature)}, Subclass: d.subclass, Choose: d.choose}
		for _, o := range d.options {
			if f := c.features[o]; f != nil && taken[f.Name] {
				continue
			}
			if !slices.Contains(b.FeatureChoices, o) {
				fc.Options = append(fc.Options, NamedKey{Key: o, NamePT: c.namePT(o)})
			}
		}
		out = append(out, fc)
	}
	return out
}

// rollDice lists the hit die of each entry of HitPoints.Rolls (character
// levels 2, 3...), in Build.Classes order.
func rollDice(b Build, c *content) []int {
	var dice []int
	first := true
	for _, cl := range b.Classes {
		class := c.classes[cl.Class]
		for range cl.Level {
			if first {
				first = false
				continue
			}
			die := 0
			if class != nil {
				die = class.HitDie
			}
			dice = append(dice, die)
		}
	}
	return dice
}

// oldRolls are the rolls of the earlier levels as the Build stands: its own
// when it rolls (a missing one is the average), the averages otherwise.
func oldRolls(b Build, c *content) []int {
	dice := rollDice(b, c)
	out := make([]int, len(dice))
	for i, die := range dice {
		out[i] = die/2 + 1
		if b.HitPoints.Method == HitPointsRolled && i < len(b.HitPoints.Rolls) {
			out[i] = b.HitPoints.Rolls[i]
		}
	}
	return out
}

// rollIndex is where the new level's roll goes in HitPoints.Rolls: after
// every earlier level of the class that gains it and of the classes before it.
func rollIndex(b Build, classIdx int) int {
	n := 0
	// A class the character does not have yet (classIdx past the last) goes after all.
	for _, cl := range b.Classes[:min(classIdx+1, len(b.Classes))] {
		n += cl.Level
	}
	return n - 1
}

// ApplyLevelUp returns the Build that the choices make from before: the
// class one level higher, or a class the character does not have at level 1
// (last in the list), and the choices added to it. It never judges the
// choices (CheckLevelUp does); it only refuses a class the content does not
// have (LevelUpClass), the prerequisites of a new class and level 20
// (LevelUpMaxLevel).
//
// The hit points follow the sheet's method. With the fixed average the Build
// stays as it is; a rolled value switches a fixed sheet to rolled, filling
// the earlier levels with their averages so nothing else changes, and an
// average on a rolled sheet adds the average as a roll.
func ApplyLevelUp(before Build, ch LevelUpChoices, c *Content) (Build, error) {
	idx, lerr := levelUpClass(before, ch.Class, c.c)
	if lerr != nil {
		return Build{}, lerr
	}
	after := before.clone()
	if idx == len(after.Classes) {
		after.Classes = append(after.Classes, ClassLevel{Class: ch.Class})
	}
	cl := &after.Classes[idx]
	cl.Level++
	if ch.Subclass != "" {
		cl.Subclass = ch.Subclass
	}
	addAbilityIncrease(&after, applyFeat(&after, before, ch, cl.Level, c.c))
	if ch.Feat != "" {
		after.Feats = append(after.Feats, ch.Feat)
	}
	applyPicks(&after, ch, c.c)
	return applyHitPoints(after, before, ch, idx, c.c), nil
}

// applyFeat records the feat taken (ch.Feat) in the slot of the class's Ability
// Score Improvement at classLevel, and returns the ability increase of the level:
// the player's, or the feat's own for a feat that raises every ability it lists
// and nothing was sent.
func applyFeat(after *Build, before Build, ch LevelUpChoices, classLevel int, c *content) map[Ability]int {
	increase := ch.AbilityIncrease
	f, ok := c.feats[ch.Feat]
	if !ok || ch.Feat == "" {
		return increase
	}
	if after.FeatSlots == nil {
		after.FeatSlots = map[string]string{}
	}
	after.FeatSlots[ch.Feat] = asiFeatureKey(c.classLevels[ch.Class][classLevel-1])
	// A feat that raises every ability it lists has nothing to choose: the increase is
	// the feat's own, stopping at 20 (SRD 5.1, Ability Score Improvement).
	if e := c.featEntry(f); len(increase) == 0 && e.Increase != nil && e.Increase.Count == len(e.Increase.From) {
		increase = fixedIncrease(e.Increase, derive(before, c))
	}
	return increase
}

// addAbilityIncrease adds an ability increase to the sheet's extra bonuses.
func addAbilityIncrease(after *Build, increase map[Ability]int) {
	for a, v := range increase {
		if after.ExtraAbilityBonuses == nil {
			after.ExtraAbilityBonuses = map[Ability]int{}
		}
		after.ExtraAbilityBonuses[a] += v
	}
}

// applyPicks adds the player's picks to the sheet: spells, options (giving up the
// swapped invocation first), their texts, skills, expertise and the instrument.
func applyPicks(after *Build, ch LevelUpChoices, c *content) {
	after.Cantrips = append(after.Cantrips, ch.Cantrips...)
	after.SpellsKnown = append(after.SpellsKnown, ch.Spells...)
	after.SpellsPrepared = append(after.SpellsPrepared, ch.Prepared...)
	if ch.SwapInvocation != "" {
		after.FeatureChoices = slices.DeleteFunc(after.FeatureChoices, func(k string) bool { return k == ch.SwapInvocation })
	}
	after.FeatureChoices = append(after.FeatureChoices, ch.LateChoices...)
	after.FeatureChoices = append(after.FeatureChoices, ch.FeatureChoices...)
	if len(ch.FeatureChoiceText) > 0 {
		if after.FeatureChoiceText == nil {
			after.FeatureChoiceText = map[string]string{}
		}
		maps.Copy(after.FeatureChoiceText, ch.FeatureChoiceText)
	}
	after.SkillProficiencies = append(after.SkillProficiencies, ch.SkillProficiencies...)
	after.Expertise = append(after.Expertise, ch.Expertise...)
	if name := c.proficiencyNamePT(ch.Instrument); ch.Instrument != "" && name != "" {
		after.ToolProficiencies = append(after.ToolProficiencies, name)
	}
}

// applyHitPoints stores the hit points of the new level (see ApplyLevelUp): the
// class at index idx gains it.
func applyHitPoints(after, before Build, ch LevelUpChoices, idx int, c *content) Build {
	die := c.classes[ch.Class].HitDie
	value := ch.HitPoints.Roll
	if ch.HitPoints.Average {
		value = die/2 + 1
	}
	if before.HitPoints.Method == HitPointsFixed && ch.HitPoints.Average {
		return after // nothing to store: the average is what Fixed means
	}
	rolls := oldRolls(before, c)
	i := rollIndex(before, idx)
	after.HitPoints = HitPoints{Method: HitPointsRolled, Rolls: slices.Insert(rolls, min(i, len(rolls)), value)}
	return after
}

// CheckLevelUp refuses any difference between before and after that one
// level of one class does not allow, with the first refusal's field and
// reason. The only allowed differences are:
//
//   - exactly one existing class gains exactly one level;
//   - the ability increase, only at an Ability Score Improvement level: +2 in
//     one ability or +1 in two, none above 20, as extra_ability_bonuses; or, in
//     its place, one feat the character qualifies for, with the increase the feat
//     gives (whether the table uses feats is the server's to check);
//   - the new level's hit points, in 1 to the hit die, with the earlier
//     levels unchanged (a fixed sheet that rolls takes their averages);
//   - exactly the new cantrips, known or spellbook spells the level gives, and
//     any new prepared spells, with none removed;
//   - the subclass when it is due, the options the new features offer, and the
//     skills and expertise the new level lets the player choose.
//
// Anything else differs is refused: name, race, base scores, equipment,
// weapons, the other classes... (RN-01: the rest stays locked). A sheet that
// ends with a problem the old one did not have (a spell off the class list,
// too many prepared spells) is refused as LevelUpSheetIssue.
func CheckLevelUp(before, after Build, c *Content) error {
	if err := checkLevelUp(before, after, c.c); err != nil {
		return err
	}
	return nil
}

func checkLevelUp(before, after Build, c *content) *LevelUpError {
	// One class, one level. A class added at level 1 is checked as if the
	// character had it at level 0.
	ext, tools, newClass, lerr := checkNewClass(before, after, c)
	if lerr != nil {
		return lerr
	}
	idx, lerr := levelUpClassOf(ext, after, c)
	if lerr != nil {
		return lerr
	}
	classKey := ext.Classes[idx].Class
	class := c.classes[classKey]
	newLevel := ext.Classes[idx].Level + 1

	if lerr := checkLocked(before, tools); lerr != nil {
		return lerr
	}
	dBefore, dAfter := derive(before, c), derive(after, c)
	if lerr := checkLevelUpIncrease(before, after, dBefore, dAfter, c.classLevels[classKey][newLevel-1], c); lerr != nil {
		return lerr
	}
	if lerr := checkHitPoints(ext, after, idx, class.HitDie, c); lerr != nil {
		return lerr
	}
	mc, lerr := c.checkNewClassTools(before, after, dBefore, classKey, newClass)
	if lerr != nil {
		return lerr
	}

	// The subclass, then what depends on it.
	sub, lerr := checkSubclass(ext.Classes[idx], after.Classes[idx], idx, class, c)
	if lerr != nil {
		return lerr
	}
	if lerr := checkDuplicates(duplicatesOf(before, after, newClass)); lerr != nil {
		return lerr
	}
	// The same numbers the options give: the class's, and the chosen
	// subclass's, which are not in the options' top level while it is due.
	offer, _ := levelUpOptionsWith(before, idx, classKey, c, sub)
	offOwnList, lerr := checkSpells(before, after, offer, offer.SpellList, c)
	if lerr != nil {
		return lerr
	}
	gains := c.levelGains(classKey, sub, newLevel)

	// Options, skills and expertise.
	if lerr := checkLevelUpChoices(before, after, classKey, c); lerr != nil {
		return lerr
	}
	if lerr := c.checkLevelUpSkills(before, after, mc, gains); lerr != nil {
		return lerr
	}
	return checkNewIssues(dBefore, dAfter, offOwnList)
}

// checkNewClass checks a class added at level 1, when after has one more class
// than before: one the content has, not one the character already has, and whose
// prerequisites are met. It returns the sheet before with the class at level 0
// (or before itself) and the sheet after with the tools of before, since the
// instrument of the Bard's table is checked on its own.
func checkNewClass(before, after Build, c *content) (ext, tools Build, newClass bool, lerr *LevelUpError) {
	newKey, newClass := newClassOf(before, after)
	if !newClass {
		return before, after, false, nil
	}
	field := fmt.Sprintf("full.classes[%d].class_key", len(before.Classes))
	if _, known := c.classes[newKey]; !known {
		return before, after, true, refuse(field, LevelUpReasonClass, "no such class")
	}
	if slices.ContainsFunc(before.Classes, func(cl ClassLevel) bool { return cl.Class == newKey }) {
		return before, after, true, refuse(field, LevelUpReasonClass, "the class is listed twice")
	}
	if lerr := c.multiclassAllowed(before, newKey); lerr != nil {
		return before, after, true, lerr
	}
	tools = after
	tools.ToolProficiencies = before.ToolProficiencies
	return withNewClass(before, newKey), tools, true, nil
}

// checkLevelUpIncrease checks the feat and the ability increase of the level,
// whose class table row is row.
func checkLevelUpIncrease(before, after Build, dBefore, dAfter Derived, row *srd51.Level, c *content) *LevelUpError {
	asi := isASILevel(row)
	feat, lerr := checkFeat(before, after, c, asi)
	if lerr != nil {
		return lerr
	}
	return checkAbilities(before, after, dBefore, dAfter, asi, feat)
}

// checkNewClassTools checks, for a new class, the instrument its multiclass
// proficiencies let the player pick, and returns its multiclass offer; nil for a
// class the character already has.
func (c *content) checkNewClassTools(before, after Build, dBefore Derived, classKey string, newClass bool) (*MulticlassOffer, *LevelUpError) {
	if !newClass {
		return nil, nil
	}
	bare := after.clone()
	bare.ToolProficiencies = before.ToolProficiencies
	mc := c.multiclassOffer(before, dBefore, bare, classKey)
	if lerr := c.checkMulticlassTools(mc, before.ToolProficiencies, after.ToolProficiencies); lerr != nil {
		return nil, lerr
	}
	return mc, nil
}

// checkSubclass checks the subclass of the class at index idx, bcl before and acl
// after: chosen when it is due, one of the class, and unchanged otherwise. It
// returns the subclass after, or nil.
func checkSubclass(bcl, acl ClassLevel, idx int, class *srd51.Class, c *content) (*srd51.Subclass, *LevelUpError) {
	due := subclassDue(bcl, class, bcl.Level+1)
	field := fmt.Sprintf("full.classes[%d]", idx)
	switch {
	case acl.CustomSubclassName != bcl.CustomSubclassName:
		return nil, refuse(field+".custom_subclass_name", LevelUpReasonSubclass, "a custom subclass is the master's to set")
	case due && acl.Subclass == "":
		return nil, refuse(field+".subclass_key", LevelUpReasonSubclass, "the subclass is chosen at this level")
	case due && c.subclasses[acl.Subclass] == nil, due && c.subclasses[acl.Subclass].Class != bcl.Class:
		return nil, refuse(field+".subclass_key", LevelUpReasonSubclass, "not a subclass of the class")
	case !due && acl.Subclass != bcl.Subclass:
		return nil, refuse(field+".subclass_key", LevelUpReasonSubclass, "the subclass is not chosen at this level")
	}
	return c.subclasses[acl.Subclass], nil
}

// duplicatesOf is the sheet whose lists checkDuplicates reads: after, with the
// skills of before when the class is new, since a skill picked twice, or one the
// character already has, is then the refusal of the multiclass pick
// (checkMulticlassSkills).
func duplicatesOf(before, after Build, newClass bool) Build {
	dups := after
	if newClass {
		dups.SkillProficiencies = before.SkillProficiencies
	}
	return dups
}

// checkLevelUpSkills checks the new skills (the multiclass pick of a new class, mc
// not nil) and the new expertise against what the level gives.
func (c *content) checkLevelUpSkills(before, after Build, mc *MulticlassOffer, gains levelGains) *LevelUpError {
	wantSkills, wantExpertise := gains.skills, gains.expertise
	got, removedSkills := added(before.SkillProficiencies, after.SkillProficiencies)
	switch {
	case removedSkills:
		return refuse("full.skill_proficiency_keys", LevelUpReasonSkills, "a skill was removed")
	case mc != nil:
		if lerr := c.checkMulticlassSkills(mc, got, wantSkills); lerr != nil {
			return lerr
		}
	case len(got) != wantSkills:
		return refuse("full.skill_proficiency_keys", LevelUpReasonSkills, "the new level lets the player choose %d skills", wantSkills)
	}
	if got, removed := added(before.Expertise, after.Expertise); removed || len(got) != wantExpertise {
		return refuse("full.expertise_skill_keys", LevelUpReasonExpertise, "the new level gives %d expertise", wantExpertise)
	}
	return nil
}

// checkNewIssues refuses the first problem the sheet after has and the sheet
// before did not: it must not end up with one. Magical Secrets spells
// (offOwnList) are off the class list on purpose.
func checkNewIssues(dBefore, dAfter Derived, offOwnList map[string]bool) *LevelUpError {
	had := map[string]bool{}
	for _, is := range dBefore.Issues {
		had[is.Code+"|"+is.Field+"|"+is.Message] = true
	}
	for _, is := range dAfter.Issues {
		if is.Code == IssueSpellNotOnList && offOwnList[is.Field] {
			continue
		}
		if !had[is.Code+"|"+is.Field+"|"+is.Message] {
			return &LevelUpError{Field: is.Field, Reason: LevelUpReasonSheetIssue, Code: is.Code, Message: is.Message}
		}
	}
	return nil
}

// checkLevelUpChoices checks the picks of the choice engine against the new level: the
// sheet that comes out has every choice made (the level's own and the ones an
// earlier level left open), takes nothing a choice does not offer and nothing whose
// prerequisite is unmet, and loses no pick, but for one Eldritch Invocation the
// warlock swaps (SRD 5.1, Warlock, Eldritch Invocations: "when you gain a level in
// this class, you can choose one of the invocations you know and replace it with
// another invocation that you could learn at that level").
func checkLevelUpChoices(before, after Build, classKey string, c *content) *LevelUpError {
	if lerr := checkRemovedChoices(before, after, classKey, c); lerr != nil {
		return lerr
	}
	for k, v := range before.FeatureChoiceText {
		if after.FeatureChoiceText[k] != v {
			return refuse("full.feature_choice_text", LevelUpReasonLocked, "a text already written does not change")
		}
	}
	return checkChoiceProblems(c.choiceSet(before), c.choiceSet(after))
}

// levelUpChoicesField is the sheet field of the choice engine's picks.
const levelUpChoicesField = "full.feature_choice_keys"

// checkRemovedChoices refuses a pick of before that after lost, but for one
// Eldritch Invocation a warlock swaps.
func checkRemovedChoices(before, after Build, classKey string, c *content) *LevelUpError {
	var gone []string
	for _, k := range before.FeatureChoices {
		if !slices.Contains(after.FeatureChoices, k) {
			gone = append(gone, k)
		}
	}
	if len(gone) > 1 || (len(gone) == 1 && (classKey != classWarlock || !c.isInvocation(gone[0]))) {
		return refuse(levelUpChoicesField, LevelUpReasonFeatureChoice, "an option was removed")
	}
	return nil
}

// checkChoiceProblems refuses the first problem of the sheet after's choices that
// the sheet before did not have: a choice left open (a late one, which an earlier
// level left open, or a new one), an unmet prerequisite or an option not offered.
func checkChoiceProblems(setBefore, setAfter ChoiceSet) *LevelUpError {
	had := map[string]bool{}
	for _, p := range setBefore.ChoiceProblems(false) {
		had[p.Code+"|"+p.ChoiceKey+"|"+p.OptionKey] = true
	}
	late := map[string]bool{}
	for _, p := range setBefore.Pending() {
		late[p.ChoiceKey] = true
	}
	for _, p := range setAfter.ChoiceProblems(true) {
		if lerr := choiceProblemRefusal(p, late[p.ChoiceKey], had[p.Code+"|"+p.ChoiceKey+"|"+p.OptionKey]); lerr != nil {
			return lerr
		}
	}
	return nil
}

// choiceProblemRefusal is the refusal of one problem of the new sheet's choices,
// or nil for one the sheet already had. late says the choice was left open by an
// earlier level.
func choiceProblemRefusal(p ChoiceProblem, late, had bool) *LevelUpError {
	switch {
	case p.Code == ChoiceProblemMissing && late:
		return refuse(levelUpChoicesField, LevelUpReasonLateChoice, "%s was left open by an earlier level and is still open", p.ChoiceKey)
	case p.Code == ChoiceProblemMissing:
		return refuse(levelUpChoicesField, LevelUpReasonFeatureChoice, "%s needs %d options, got %d", p.ChoiceKey, p.Required, p.Picked)
	case had:
		// A problem the sheet already had is not the level's.
		return nil
	case p.Code == ChoiceProblemPrerequisite:
		return refuse(levelUpChoicesField, LevelUpReasonFeatureChoice, "%s does not meet its prerequisite", p.OptionKey)
	default:
		return refuse(levelUpChoicesField, LevelUpReasonFeatureChoice, "an option the new features do not offer")
	}
}

// levelUpClassOf finds the class that gained one level, and refuses any
// other change to the classes: another class, a class added or removed.
func levelUpClassOf(before, after Build, c *content) (int, *LevelUpError) {
	const field = "full.classes"
	if len(before.Classes) != len(after.Classes) {
		return 0, refuse(field, LevelUpReasonClass, "a class was added or removed")
	}
	idx := -1
	for i := range before.Classes {
		b, a := before.Classes[i], after.Classes[i]
		switch {
		case a.Class != b.Class:
			return 0, refuse(fmt.Sprintf("%s[%d].class_key", field, i), LevelUpReasonClass, "a class changed")
		case a.Level == b.Level:
		case a.Level == b.Level+1 && idx < 0:
			idx = i
		default:
			return 0, refuse(fmt.Sprintf("%s[%d].level", field, i), LevelUpReasonClass, "exactly one class gains exactly one level")
		}
	}
	if idx < 0 {
		return 0, refuse(field, LevelUpReasonClass, "no class gained a level")
	}
	if _, lerr := levelUpClass(before, before.Classes[idx].Class, c); lerr != nil {
		return 0, lerr
	}
	return idx, nil
}

// checkLocked refuses a difference in anything the level does not touch.
func checkLocked(before, after Build) *LevelUpError {
	locked := func(field string) *LevelUpError {
		return refuse(field, LevelUpReasonLocked, "the sheet is locked: only the master changes this")
	}
	for _, a := range AllAbilities() {
		if before.BaseScores[a] != after.BaseScores[a] {
			return locked("full.base_scores." + protoAbility[a])
		}
	}
	switch {
	case before.Race != after.Race:
		return locked("full.race_key")
	case before.Subrace != after.Subrace:
		return locked("full.subrace_key")
	case before.Background != after.Background:
		return locked("full.background_key")
	case before.CustomBackgroundName != after.CustomBackgroundName,
		!slices.Equal(before.CustomBackgroundSkills, after.CustomBackgroundSkills),
		!slices.Equal(before.CustomBackgroundProficiencies, after.CustomBackgroundProficiencies),
		before.CustomBackgroundFeatureName != after.CustomBackgroundFeatureName,
		before.CustomBackgroundFeature != after.CustomBackgroundFeature,
		before.CustomBackgroundEquipment != after.CustomBackgroundEquipment:
		return locked("full.custom_background")
	case before.Armor != after.Armor:
		return locked("full.armor_key")
	case before.Shield != after.Shield:
		return locked("full.shield")
	case !slices.Equal(before.Weapons, after.Weapons):
		return locked("full.weapon_keys")
	case !slices.Equal(before.ToolProficiencies, after.ToolProficiencies):
		return locked("full.tool_proficiencies")
	}
	return nil
}

// added returns the entries of after that before does not have, and whether
// after dropped any entry of before.
func added(before, after []string) (fresh []string, removed bool) {
	for _, k := range before {
		if !slices.Contains(after, k) {
			removed = true
		}
	}
	for _, k := range after {
		if !slices.Contains(before, k) {
			fresh = append(fresh, k)
		}
	}
	return fresh, removed
}

// checkFeat checks the feat of the new level: at most one, only at an Ability Score
// Improvement level, one the content has, and one the character qualifies for
// with the level gained and before the feat's own ability increase. It returns the
// feat taken, or nil.
func checkFeat(before, after Build, c *content, asi bool) (*FeatEntry, *LevelUpError) {
	const field = "full.feat_keys"
	fresh, removed := added(before.Feats, after.Feats)
	switch {
	case removed:
		return nil, refuse(field, LevelUpReasonFeat, "a feat was removed")
	case len(fresh) == 0:
		return nil, nil
	case len(fresh) > 1:
		return nil, refuse(field, LevelUpReasonFeat, "one feat in place of one Ability Score Improvement")
	case !asi:
		return nil, refuse(field, LevelUpReasonFeat, "this level has no Ability Score Improvement")
	}
	f, ok := c.feats[fresh[0]]
	if !ok {
		return nil, refuse(field, LevelUpReasonFeat, "not a feat of the content")
	}
	// A feat the sheet already has, picked through a choice, is not taken twice.
	if slices.ContainsFunc(derive(before, c).Features, func(ft Feature) bool { return ft.Key == fresh[0] }) {
		return nil, refuse(field, LevelUpReasonFeat, "the character already has this feat")
	}
	entry := c.featEntry(f)
	// The prerequisite is judged on the sheet with the level gained and without the
	// feat and what it raises, as the guided level-up offered it.
	pre := after.clone()
	pre.Feats, pre.ExtraAbilityBonuses = slices.Clone(before.Feats), maps.Clone(before.ExtraAbilityBonuses)
	if unmet := c.unmetPrerequisite(entry, pre, derive(pre, c)); len(unmet) > 0 {
		return nil, refuse(field, LevelUpReasonFeatPrerequisite, "the character does not meet the prerequisite of the feat (%s)", unmet[0].Kind)
	}
	return &entry, nil
}

// checkAbilities checks the ability increase: only at an ASI level, +2 in one
// ability or +1 in two, and none above 20. With a feat taken in its place the
// increase is the feat's own: its ability_increase effect's count of different
// abilities from its list with its value each, and nothing when it has none.
func checkAbilities(before, after Build, dBefore, dAfter Derived, asi bool, feat *FeatEntry) *LevelUpError {
	const field = "full.extra_ability_bonuses"
	raised, total, lerr := abilityIncrease(before, after)
	if lerr != nil {
		return lerr
	}
	if total == 0 && (feat == nil || feat.Increase == nil) {
		return nil
	}
	if !asi {
		return refuse(field, LevelUpReasonAbilityNotDue, "this level has no Ability Score Improvement")
	}
	switch {
	case feat != nil && feat.Increase == nil:
		return refuse(field, LevelUpReasonAbilityShape, "the feat gives no ability increase")
	case feat != nil:
		return checkFeatIncrease(feat.Increase, before, after, dBefore)
	case total != abilityScoreImprovementPoints:
		return refuse(field, LevelUpReasonAbilityShape, "+2 in one ability or +1 in two")
	}
	return checkRaisedTo20(raised, dAfter)
}

// maxAbilityStep is the most one ability goes up in one level.
const maxAbilityStep = 2

// abilityIncrease reads the increase from before to after: the abilities raised
// and the points in all. It refuses an ability lowered or raised by more than 2,
// and an ability the rules do not know.
func abilityIncrease(before, after Build) (raised []Ability, total int, lerr *LevelUpError) {
	const field = "full.extra_ability_bonuses"
	for _, a := range AllAbilities() {
		d := after.ExtraAbilityBonuses[a] - before.ExtraAbilityBonuses[a]
		switch {
		case d < 0 || d > maxAbilityStep:
			return nil, 0, refuse(field+"."+protoAbility[a], LevelUpReasonAbilityShape, "an ability goes up by 1 or 2 at most")
		case d > 0:
			raised = append(raised, a)
			total += d
		}
	}
	for a := range after.ExtraAbilityBonuses {
		if _, ok := abilityIndex[a]; !ok {
			return nil, 0, refuse(field, LevelUpReasonAbilityShape, "an unknown ability")
		}
	}
	return raised, total, nil
}

// checkRaisedTo20 refuses a raised ability whose score after passes 20.
func checkRaisedTo20(raised []Ability, dAfter Derived) *LevelUpError {
	for _, a := range raised {
		for _, s := range dAfter.Abilities {
			if s.Ability == a && s.Score > MaxNormalScore {
				return refuse("full.extra_ability_bonuses."+protoAbility[a], LevelUpReasonAbilityAbove20, "an ability score cannot pass 20")
			}
		}
	}
	return nil
}

// checkHitPoints checks the hit points of the new level: the new entry is in
// 1 to the die, and the earlier levels are as they were.
func checkHitPoints(before, after Build, classIdx, die int, c *content) *LevelUpError {
	const field = "full.hit_points"
	if before.HitPoints.Method == HitPointsFixed && after.HitPoints.Method == HitPointsFixed {
		if !slices.Equal(before.HitPoints.Rolls, after.HitPoints.Rolls) {
			return refuse(field+".rolls", LevelUpReasonHitPoints, "the rolls of a fixed sheet do not change")
		}
		return nil
	}
	if after.HitPoints.Method != HitPointsRolled {
		return refuse(field+".method", LevelUpReasonHitPoints, "a sheet that rolls its hit points keeps rolling")
	}
	old := oldRolls(before, c)
	i := rollIndex(before, classIdx)
	rolls := after.HitPoints.Rolls
	if len(rolls) != len(old)+1 || i > len(old) {
		return refuse(field+".rolls", LevelUpReasonHitPoints, "one new roll for the new level")
	}
	if got := slices.Delete(slices.Clone(rolls), i, i+1); !slices.Equal(got, old) {
		return refuse(field+".rolls", LevelUpReasonHitPoints, "the earlier levels do not change")
	}
	if rolls[i] < 1 || rolls[i] > die {
		return refuse(fmt.Sprintf("%s.rolls[%d]", field, i), LevelUpReasonHitPoints, "the roll is 1 to %d", die)
	}
	return nil
}

// checkSpells checks the new cantrips, known or spellbook spells, and
// prepared spells against what the level gives. It also returns the sheet
// fields of the new known spells that are off the class's list, which the
// Bard's Magical Secrets allow (up to offer.AnyClassSpells of them).
func checkSpells(before, after Build, offer LevelUpOffer, list string, c *content) (map[string]bool, *LevelUpError) {
	newKnown, lerr := checkSpellCounts(before, after, offer)
	if lerr != nil {
		return nil, lerr
	}
	offList := map[string]bool{}
	patron := patronOf(after, offer.Class, c)
	for i, key := range after.SpellsKnown {
		if slices.Contains(newKnown, key) && c.offList(key, list, patron) {
			offList[fmt.Sprintf("full.known_spell_keys[%d]", i)] = true
		}
	}
	if len(offList) > offer.AnyClassSpells {
		return nil, refuse("full.known_spell_keys", LevelUpReasonSpells, "only %d of the new spells may come from another class's list", offer.AnyClassSpells)
	}
	return offList, nil
}

// checkSpellCounts checks the counts of the new cantrips, known (or spellbook)
// spells and prepared spells, with none removed, and returns the new known ones.
func checkSpellCounts(before, after Build, offer LevelUpOffer) ([]string, *LevelUpError) {
	newCantrips, removed := added(before.Cantrips, after.Cantrips)
	if removed || len(newCantrips) != offer.Cantrips {
		return nil, refuse("full.cantrip_keys", LevelUpReasonCantrips, "the level gives %d new cantrips, none removed", offer.Cantrips)
	}
	newKnown, removed := added(before.SpellsKnown, after.SpellsKnown)
	if removed || len(newKnown) != offer.Spells {
		return nil, refuse("full.known_spell_keys", LevelUpReasonSpells, "the level gives %d new spells, none removed", offer.Spells)
	}
	newPrepared, removed := added(before.SpellsPrepared, after.SpellsPrepared)
	if removed || (len(newPrepared) > 0 && !offer.Prepares) {
		return nil, refuse("full.prepared_spell_keys", LevelUpReasonPrepared, "prepared spells are only added by a class that prepares, none removed")
	}
	return newKnown, nil
}

// patronOf is the subclass of the class classKey whose expanded list (The Fiend)
// is part of the warlock's list for the sheet, or "".
func patronOf(b Build, classKey string, c *content) string {
	patron := ""
	for _, cl := range b.Classes {
		if sub := c.subclasses[cl.Subclass]; cl.Class == classKey && sub != nil && sub.ExpandedList {
			patron = sub.Key
		}
	}
	return patron
}

// offList says a spell the content has is on neither the list nor the patron's
// expanded list.
func (c *content) offList(key, list, patron string) bool {
	s := c.spells[key]
	return s != nil && !c.onList(s, list) && (patron == "" || !slices.Contains(s.Subclasses, patron))
}

// checkDuplicates refuses a key twice in any list of the new sheet.
func checkDuplicates(b Build) *LevelUpError {
	for _, l := range []struct {
		field, reason string
		keys          []string
	}{
		{"full.cantrip_keys", LevelUpReasonCantrips, b.Cantrips},
		{"full.known_spell_keys", LevelUpReasonSpells, b.SpellsKnown},
		{"full.prepared_spell_keys", LevelUpReasonPrepared, b.SpellsPrepared},
		{"full.feature_choice_keys", LevelUpReasonFeatureChoice, b.FeatureChoices},
		{"full.feat_keys", LevelUpReasonFeat, b.Feats},
		{"full.skill_proficiency_keys", LevelUpReasonSkills, b.SkillProficiencies},
		{"full.expertise_skill_keys", LevelUpReasonExpertise, b.Expertise},
	} {
		seen := map[string]bool{}
		for i, k := range l.keys {
			if seen[k] {
				return refuse(fmt.Sprintf("%s[%d]", l.field, i), l.reason, "the same entry twice")
			}
			seen[k] = true
		}
	}
	return nil
}

// checkFeatIncrease checks the increase a feat gave. A feat that raises every ability it lists
// raises each by its value, no more than the room under 20 (nothing on an ability already at
// 20). A feat that lets the player choose raises as many different abilities of its list as it
// asks, or as many as still have room when fewer do, each by its value and none above 20
// (SRD 5.1, Ability Score Improvement: an ability score cannot be raised above 20 by it).
func checkFeatIncrease(inc *FeatIncrease, before, after Build, dBefore Derived) *LevelUpError {
	const field = "full.extra_ability_bonuses"
	score := map[Ability]int{}
	for _, s := range dBefore.Abilities {
		score[s.Ability] = s.Score
	}
	delta := func(a Ability) int { return after.ExtraAbilityBonuses[a] - before.ExtraAbilityBonuses[a] }
	for _, a := range AllAbilities() {
		if delta(a) != 0 && !slices.Contains(inc.From, a) {
			return refuse(field+"."+protoAbility[a], LevelUpReasonAbilityShape, "the feat raises only the abilities it lists")
		}
	}
	if inc.Count == len(inc.From) {
		return checkFixedFeatIncrease(inc, score, delta)
	}
	return checkChosenFeatIncrease(inc, score, delta)
}

// checkFixedFeatIncrease checks the increase of a feat that raises every ability
// it lists: each by its value, no more than the room under 20.
func checkFixedFeatIncrease(inc *FeatIncrease, score map[Ability]int, delta func(Ability) int) *LevelUpError {
	for _, a := range inc.From {
		if want := max(min(inc.Value, MaxNormalScore-score[a]), 0); delta(a) != want {
			return refuse("full.extra_ability_bonuses."+protoAbility[a], LevelUpReasonAbilityShape, "the feat raises %s by %d, to 20 at most", protoAbility[a], want)
		}
	}
	return nil
}

// checkChosenFeatIncrease checks the increase of a feat that lets the player
// choose: as many different abilities of its list as it asks (or as still have
// room), each by its value and none above 20.
func checkChosenFeatIncrease(inc *FeatIncrease, score map[Ability]int, delta func(Ability) int) *LevelUpError {
	const field = "full.extra_ability_bonuses"
	room, picked := 0, 0
	for _, a := range inc.From {
		if score[a]+inc.Value <= MaxNormalScore {
			room++
		}
		switch d := delta(a); {
		case d == 0:
		case d != inc.Value:
			return refuse(field+"."+protoAbility[a], LevelUpReasonAbilityShape, "the feat raises each ability it picks by %d", inc.Value)
		case score[a]+d > MaxNormalScore:
			return refuse(field+"."+protoAbility[a], LevelUpReasonAbilityAbove20, "an ability score cannot pass 20")
		default:
			picked++
		}
	}
	if picked != min(inc.Count, room) {
		return refuse(field, LevelUpReasonAbilityShape, "the feat raises %d different abilities by %d each", min(inc.Count, room), inc.Value)
	}
	return nil
}
