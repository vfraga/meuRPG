package characters

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/names"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// Limits on what people type, as the characters.proto comments document
// them. Lengths are in characters (Unicode code points), after trimming.
// package rules holds the limits of the rules part of a full sheet (scores,
// levels, lists of keys).
const (
	// MaxNameLength is the longest character name. The
	// characters_name_length CHECK says the same.
	MaxNameLength = 80
	// MaxMasterNotesLength is the longest master's notes. The
	// character_master_notes_length CHECK says the same.
	MaxMasterNotesLength = 20000

	// Full sheet.
	maxEquipmentItems       = 100
	maxItemNameLength       = 100
	maxItemQuantity         = 9999
	maxCoins                = 1_000_000
	maxFreeTextEntries      = 20 // languages, tool proficiencies
	maxFreeTextEntryLength  = 40
	maxExperiencePoints     = 1_000_000
	maxXPValue              = 1_000_000 // an NPC's xp_value
	maxCustomFeaturesLength = 5000

	// Basic sheet.
	maxBasicHitPoints    = 9999
	minArmorClass        = 1
	maxArmorClass        = 40
	maxSpeedFt           = 300
	minAttackBonus       = -10
	maxAttackBonus       = 30
	maxDamageLength      = 40 // the deprecated free text damage
	maxDescriptionLength = 2000
	minInitiativeBonus   = -10
	maxInitiativeBonus   = 20
	maxBasicAttacks      = 3
	legacyAttackName     = "Ataque"
	maxAttackNameLength  = 40
	minAttackRollBonus   = -10
	maxAttackRollBonus   = 20
	maxDamageDiceCount   = 20
	minDamageBonus       = -20
	maxDamageBonus       = 40
	maxRangeFt           = 600

	// Story.
	maxPersonalityLength           = 1000
	maxAppearanceFieldLength       = 40
	maxAppearanceDescriptionLength = 2000
	maxBackstoryLength             = 10000
	maxAlliesLength                = 2000
)

// fieldError is a request field that breaks a rule. Field is the path from
// the request's root, with proto field names ("sheet.full.equipment[2].name");
// the handler turns it into an invalid_argument that names the field, never
// the value.
type fieldError struct {
	field string
	err   error
}

func (e *fieldError) Error() string { return e.field + " " + e.err.Error() }

func fieldErr(field, format string, args ...any) error {
	return &fieldError{field: field, err: fmt.Errorf(format, args...)}
}

// takesFullSheet says whether a kind takes a full sheet (player, enemy,
// boss) or a basic one (minion, story NPC).
func takesFullSheet(kind string) bool {
	return kind == kindPlayer || kind == "enemy" || kind == "boss"
}

// checkSheet checks a sheet from a request and returns a copy with its text
// cleaned (trimmed), ready to store. A full sheet also goes through
// rules.Validate, with the campaign's content. It does not know the character's kind: the caller checks
// that the sheet's type matches it (checkSheetKind).
func checkSheet(content *rules.Content, sheet *charactersv1.CharacterSheet) (*charactersv1.CharacterSheet, error) {
	switch body := sheet.GetContent().(type) {
	case *charactersv1.CharacterSheet_Full:
		return checkFullSheet(content, body.Full)
	case *charactersv1.CharacterSheet_Basic:
		return checkBasicSheet(content, body.Basic)
	default:
		return nil, fieldErr("sheet", "is required: a full or a basic sheet")
	}
}

// checkFullSheet is checkSheet for a full sheet.
func checkFullSheet(content *rules.Content, sent *charactersv1.FullSheet) (*charactersv1.CharacterSheet, error) {
	full := proto.CloneOf(sent)
	if full == nil {
		full = &charactersv1.FullSheet{}
	}
	if err := cleanFullSheet(full); err != nil {
		return nil, err
	}
	// The revision of the table's content this sheet is saved with (RN-23): the
	// server's, never the client's.
	full.ContentRevision = i32(content.TableRevision())
	// A feat's place in the improvements only means something for a feat the sheet has.
	hasFeat := make(map[string]struct{}, len(full.FeatKeys))
	for _, k := range full.FeatKeys {
		hasFeat[k] = struct{}{}
	}
	maps.DeleteFunc(full.FeatSlots, func(feat, _ string) bool { _, ok := hasFeat[feat]; return !ok })
	full.KnownIssues = issueIDs(rules.Derive(buildOf(full), content).Issues)
	full.ContentBaselines = nil // the server's: carryFlags sets it when a save keeps a flag
	if err := rules.Validate(buildOf(full), content); err != nil {
		if ve, ok := errors.AsType[*rules.ValidationError](err); ok {
			return nil, &fieldError{field: "sheet." + ve.Field, err: errors.New(ve.Message)}
		}
		return nil, err
	}
	if err := checkChallenge(content, "sheet.full", full.GetChallengeRating(), full.GetXpValue()); err != nil {
		return nil, err
	}
	return &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: full}}, nil
}

// checkBasicSheet is checkSheet for a basic sheet.
func checkBasicSheet(content *rules.Content, sent *charactersv1.BasicSheet) (*charactersv1.CharacterSheet, error) {
	basic := proto.CloneOf(sent)
	if basic == nil {
		basic = &charactersv1.BasicSheet{}
	}
	// The creature link, its ability scores and the combat-only mark are the
	// server's: only CreateNpcFromCreature and AddMonsters write them (MR-042).
	basic.MonsterKey, basic.AbilityScores, basic.CombatOnly = "", nil, false
	if err := cleanBasicSheet(basic); err != nil {
		return nil, err
	}
	if err := checkChallenge(content, "sheet.basic", basic.GetChallengeRating(), basic.GetXpValue()); err != nil {
		return nil, err
	}
	return &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Basic{Basic: basic}}, nil
}

// checkChallenge checks an NPC's challenge rating and the XP it gives (MR-016):
// the rating is empty or one of the rules' ("1/8", "5"), the XP 0 to
// maxXPValue. field is where the sheet is in the request, for the error.
func checkChallenge(content *rules.Content, field, rating string, xp int32) error {
	if rating != "" {
		if _, ok := content.XPForChallenge(rating); !ok {
			return fieldErr(field+".challenge_rating", "is not a challenge rating of the rules (0, 1/8, 1/4, 1/2, 1 to 30)")
		}
	}
	if xp < 0 || xp > maxXPValue {
		return fieldErr(field+".xp_value", "must be 0 to %d", maxXPValue)
	}
	return nil
}

// checkSheetKind checks that a checked sheet is of the type the kind takes,
// and that a player's character does not carry what only an NPC has: a
// challenge rating or the XP it gives when defeated (an error, never a silent
// drop).
func checkSheetKind(kind string, sheet *charactersv1.CharacterSheet) error {
	_, full := sheet.GetContent().(*charactersv1.CharacterSheet_Full)
	switch {
	case takesFullSheet(kind) && !full:
		return fieldErr("sheet", "must be a full sheet for this kind of character")
	case !takesFullSheet(kind) && full:
		return fieldErr("sheet", "must be a basic sheet for this kind of character")
	case kind == kindPlayer && sheet.GetFull().GetChallengeRating() != "":
		return fieldErr("sheet.full.challenge_rating", "must be empty for a player's character")
	case kind == kindPlayer && sheet.GetFull().GetXpValue() != 0:
		return fieldErr("sheet.full.xp_value", "must be 0 for a player's character")
	}
	return nil
}

// cleanFullSheet checks and trims the free text and the numbers of a full
// sheet that package rules does not look at.
func cleanFullSheet(f *charactersv1.FullSheet) error {
	if err := checkFeatListSizes(f); err != nil {
		return err
	}
	if err := cleanCustomSubclasses(f); err != nil {
		return err
	}
	if err := cleanCustomBackground(f); err != nil {
		return err
	}
	if err := checkFullSheetEnums(f); err != nil {
		return err
	}
	if err := cleanEquipment(f); err != nil {
		return err
	}
	if err := checkCoins(f.GetCoins()); err != nil {
		return err
	}
	return cleanFullSheetTexts(f)
}

// checkFeatListSizes applies the request size limits to the feats of a full
// sheet, before anything walks them.
func checkFeatListSizes(f *charactersv1.FullSheet) error {
	if len(f.GetFeatKeys()) > rules.MaxListLength {
		return fieldErr("sheet.full.feat_keys", "must have at most %d entries", rules.MaxListLength)
	}
	if len(f.GetFeatSlots()) > rules.MaxListLength {
		return fieldErr("sheet.full.feat_slots", "must have at most %d entries", rules.MaxListLength)
	}
	return nil
}

// cleanCustomSubclasses trims the names of the custom subclasses.
func cleanCustomSubclasses(f *charactersv1.FullSheet) error {
	for i, c := range f.GetClasses() {
		custom, ok := c.GetSubclass().(*charactersv1.ClassLevel_CustomSubclassName)
		if !ok {
			continue
		}
		name, err := names.Clean(custom.CustomSubclassName, rules.MaxCustomNameLength)
		if err != nil {
			return &fieldError{field: fmt.Sprintf("sheet.full.classes[%d].custom_subclass_name", i), err: err}
		}
		custom.CustomSubclassName = name
	}
	return nil
}

// cleanCustomBackground checks and trims the texts of a custom background.
func cleanCustomBackground(f *charactersv1.FullSheet) error {
	custom, ok := f.GetBackground().(*charactersv1.FullSheet_CustomBackground)
	if !ok {
		return nil
	}
	if custom.CustomBackground == nil {
		custom.CustomBackground = &charactersv1.CustomBackground{}
	}
	cb := custom.CustomBackground
	name, err := names.Clean(cb.GetName(), rules.MaxCustomNameLength)
	if err != nil {
		return &fieldError{field: "sheet.full.custom_background.name", err: err}
	}
	cb.Name = name
	// The rest of the "Outro" background (SRD 5.1 "Customizing a Background"):
	// one-line names and free texts, each cleaned; an empty one
	// is a draft and shows an issue, never an error.
	if cb.FeatureName != "" {
		if cb.FeatureName, err = names.Clean(cb.FeatureName, rules.MaxCustomNameLength); err != nil {
			return &fieldError{field: "sheet.full.custom_background.feature_name", err: err}
		}
	}
	if cb.FeatureText, err = names.CleanText(cb.FeatureText, rules.MaxCustomFeatureTextLength); err != nil {
		return &fieldError{field: "sheet.full.custom_background.feature_text", err: err}
	}
	if cb.Equipment, err = names.CleanText(cb.Equipment, rules.MaxCustomEquipmentLength); err != nil {
		return &fieldError{field: "sheet.full.custom_background.equipment", err: err}
	}
	return nil
}

// checkFullSheetEnums checks the hit point method, the alignment and the experience
// points.
func checkFullSheetEnums(f *charactersv1.FullSheet) error {
	if m := f.GetHitPoints().GetMethod(); charactersv1.HitPointsMethod_name[int32(m)] == "" {
		return fieldErr("sheet.full.hit_points.method", "is not a known method")
	}
	if a := f.GetAlignment(); charactersv1.Alignment_name[int32(a)] == "" {
		return fieldErr("sheet.full.alignment", "is not a known alignment")
	}
	if xp := f.GetExperiencePoints(); xp < 0 || xp > maxExperiencePoints {
		return fieldErr("sheet.full.experience_points", "must be 0 to %d", maxExperiencePoints)
	}
	return nil
}

// cleanEquipment checks and trims the equipment list.
func cleanEquipment(f *charactersv1.FullSheet) error {
	if len(f.GetEquipment()) > maxEquipmentItems {
		return fieldErr("sheet.full.equipment", "must have at most %d items", maxEquipmentItems)
	}
	for i, item := range f.GetEquipment() {
		if err := cleanItem(fmt.Sprintf("sheet.full.equipment[%d]", i), item); err != nil {
			return err
		}
	}
	return nil
}

// cleanItem checks and trims one item of the equipment, at field.
func cleanItem(field string, item *charactersv1.Item) error {
	if item == nil {
		return fieldErr(field, "must not be empty")
	}
	name, err := names.Clean(item.GetName(), maxItemNameLength)
	if err != nil {
		return &fieldError{field: field + ".name", err: err}
	}
	item.Name = name
	if item.GetQuantity() == 0 {
		item.Quantity = 1 // 0 means 1, as documented
	}
	if item.GetQuantity() < 1 || item.GetQuantity() > maxItemQuantity {
		return fieldErr(field+".quantity", "must be 1 to %d", maxItemQuantity)
	}
	return nil
}

// checkCoins checks each coin, from copper to platinum.
func checkCoins(c *charactersv1.Coins) error {
	coins := []struct {
		name  string
		value int32
	}{
		{"copper", c.GetCopper()},
		{"silver", c.GetSilver()},
		{"electrum", c.GetElectrum()},
		{"gold", c.GetGold()},
		{"platinum", c.GetPlatinum()},
	}
	for _, coin := range coins {
		if coin.value < 0 || coin.value > maxCoins {
			return fieldErr("sheet.full.coins."+coin.name, "must be 0 to %d", maxCoins)
		}
	}
	return nil
}

// cleanFullSheetTexts checks and trims the lists of free text, the custom features and
// the choices' texts.
func cleanFullSheetTexts(f *charactersv1.FullSheet) error {
	var err error
	if f.Languages, err = cleanEntries("sheet.full.languages", f.GetLanguages()); err != nil {
		return err
	}
	if f.ToolProficiencies, err = cleanEntries("sheet.full.tool_proficiencies", f.GetToolProficiencies()); err != nil {
		return err
	}
	if f.CustomFeaturesText, err = names.CleanText(f.GetCustomFeaturesText(), maxCustomFeaturesLength); err != nil {
		return &fieldError{field: "sheet.full.custom_features_text", err: err}
	}
	return cleanChoiceTexts(f)
}

// cleanChoiceTexts trims the free texts of the choices (the humanoid races of a
// favored enemy): at most rules.MaxChoiceTexts, each one line of 1 to
// rules.MaxChoiceTextLength characters. The rules never read them.
func cleanChoiceTexts(f *charactersv1.FullSheet) error {
	if len(f.GetFeatureChoiceText()) > rules.MaxChoiceTexts {
		return fieldErr("sheet.full.feature_choice_text", "must have at most %d entries", rules.MaxChoiceTexts)
	}
	for key, text := range f.GetFeatureChoiceText() {
		if strings.TrimSpace(text) == "" {
			delete(f.FeatureChoiceText, key) // not written yet
			continue
		}
		clean, err := names.Clean(text, rules.MaxChoiceTextLength)
		if err != nil {
			return &fieldError{field: "sheet.full.feature_choice_text", err: err}
		}
		f.FeatureChoiceText[key] = clean
	}
	return nil
}

// cleanEntries checks a short list of one-line free-text entries, such as
// languages.
func cleanEntries(field string, entries []string) ([]string, error) {
	if len(entries) > maxFreeTextEntries {
		return nil, fieldErr(field, "must have at most %d entries", maxFreeTextEntries)
	}
	cleaned := make([]string, len(entries))
	for i, e := range entries {
		c, err := names.Clean(e, maxFreeTextEntryLength)
		if err != nil {
			return nil, &fieldError{field: fmt.Sprintf("%s[%d]", field, i), err: err}
		}
		cleaned[i] = c
	}
	return cleaned, nil
}

// cleanBasicSheet checks and trims a basic sheet.
func cleanBasicSheet(b *charactersv1.BasicSheet) error {
	if err := checkBasicNumbers(b); err != nil {
		return err
	}
	if err := cleanBasicAttacks(b.GetAttacks()); err != nil {
		return err
	}
	return cleanBasicTexts(b)
}

// checkBasicNumbers checks the numbers of a basic sheet, its size and how many attacks
// it has.
func checkBasicNumbers(b *charactersv1.BasicSheet) error {
	switch {
	case b.GetHitPointsMax() < 1 || b.GetHitPointsMax() > maxBasicHitPoints:
		return fieldErr("sheet.basic.hit_points_max", "must be 1 to %d", maxBasicHitPoints)
	case b.GetArmorClass() < minArmorClass || b.GetArmorClass() > maxArmorClass:
		return fieldErr("sheet.basic.armor_class", "must be %d to %d", minArmorClass, maxArmorClass)
	case b.GetSpeedFt() < 0 || b.GetSpeedFt() > maxSpeedFt:
		return fieldErr("sheet.basic.speed_ft", "must be 0 to %d", maxSpeedFt)
	case b.GetInitiativeBonus() < minInitiativeBonus || b.GetInitiativeBonus() > maxInitiativeBonus:
		return fieldErr("sheet.basic.initiative_bonus", "must be %d to %d", minInitiativeBonus, maxInitiativeBonus)
	case len(b.GetAttacks()) > maxBasicAttacks:
		return fieldErr("sheet.basic.attacks", "must have at most %d attacks", maxBasicAttacks)
	case b.GetAttackBonus() < minAttackBonus || b.GetAttackBonus() > maxAttackBonus:
		return fieldErr("sheet.basic.attack_bonus", "must be %d to %d", minAttackBonus, maxAttackBonus)
	case rulesv1.CreatureSize_name[int32(b.GetSize())] == "":
		return fieldErr("sheet.basic.size", "must be a creature size")
	}
	return nil
}

// cleanBasicAttacks checks each structured attack and trims its name.
func cleanBasicAttacks(attacks []*charactersv1.BasicAttack) error {
	for i, a := range attacks {
		if err := checkAttack(a, fmt.Sprintf("sheet.basic.attacks[%d]", i)); err != nil {
			return err
		}
		name, err := names.Clean(a.GetName(), maxAttackNameLength)
		if err != nil {
			return &fieldError{field: fmt.Sprintf("sheet.basic.attacks[%d].name", i), err: err}
		}
		a.Name = name
	}
	return nil
}

// cleanBasicTexts trims the old free-text damage and the description.
func cleanBasicTexts(b *charactersv1.BasicSheet) error {
	var err error
	if len(b.GetAttacks()) > 0 {
		// The structured attacks replace the old ones: do not keep both.
		b.AttackBonus, b.Damage = 0, ""
	} else if b.Damage, err = optionalLine(b.GetDamage(), maxDamageLength); err != nil {
		return &fieldError{field: "sheet.basic.damage", err: err}
	}
	if b.Description, err = names.CleanText(b.GetDescription(), maxDescriptionLength); err != nil {
		return &fieldError{field: "sheet.basic.description", err: err}
	}
	return nil
}

// checkAttack checks the numbers and the type of one attack. path is where
// the attack is in the request, for the error, such as
// "sheet.basic.attacks[1]"; the name is cleaned by the caller.
func checkAttack(a *charactersv1.BasicAttack, path string) error {
	switch {
	case a.GetAttackBonus() < minAttackRollBonus || a.GetAttackBonus() > maxAttackRollBonus:
		return fieldErr(path+".attack_bonus", "must be %d to %d", minAttackRollBonus, maxAttackRollBonus)
	case a.GetDamageDiceCount() < 1 || a.GetDamageDiceCount() > maxDamageDiceCount:
		return fieldErr(path+".damage_dice_count", "must be 1 to %d", maxDamageDiceCount)
	case !slices.Contains([]int32{4, 6, 8, 10, 12}, a.GetDamageDiceSides()):
		return fieldErr(path+".damage_dice_sides", "must be 4, 6, 8, 10 or 12")
	case a.GetDamageBonus() < minDamageBonus || a.GetDamageBonus() > maxDamageBonus:
		return fieldErr(path+".damage_bonus", "must be %d to %d", minDamageBonus, maxDamageBonus)
	case a.GetDamageType() == charactersv1.DamageType_DAMAGE_TYPE_UNSPECIFIED ||
		charactersv1.DamageType_name[int32(a.GetDamageType())] == "":
		return fieldErr(path+".damage_type", "is required")
	case a.GetRangeFt() < 0 || a.GetRangeFt() > maxRangeFt:
		return fieldErr(path+".range_ft", "must be 0 to %d", maxRangeFt)
	}
	return nil
}

// textField is one free-text field to clean: where it is in the request,
// the value to replace, and its limit.
type textField struct {
	field string
	value *string
	max   int
}

// checkStory checks a story from a request and returns a copy with its text
// cleaned. Unset means an empty story.
func checkStory(story *charactersv1.CharacterStory) (*charactersv1.CharacterStory, error) {
	st := proto.CloneOf(story)
	if st == nil {
		return &charactersv1.CharacterStory{}, nil
	}
	texts := []textField{
		{"story.backstory", &st.Backstory, maxBackstoryLength},
		{"story.allies", &st.Allies, maxAlliesLength},
	}
	if p := st.GetPersonality(); p != nil {
		texts = append(texts,
			textField{"story.personality.traits", &p.Traits, maxPersonalityLength},
			textField{"story.personality.ideals", &p.Ideals, maxPersonalityLength},
			textField{"story.personality.bonds", &p.Bonds, maxPersonalityLength},
			textField{"story.personality.flaws", &p.Flaws, maxPersonalityLength},
		)
	}
	var lines []textField // one-line fields, which may be empty
	if a := st.GetAppearance(); a != nil {
		texts = append(texts, textField{"story.appearance.description", &a.Description, maxAppearanceDescriptionLength})
		lines = []textField{
			{"story.appearance.age", &a.Age, maxAppearanceFieldLength},
			{"story.appearance.height", &a.Height, maxAppearanceFieldLength},
			{"story.appearance.weight", &a.Weight, maxAppearanceFieldLength},
			{"story.appearance.eyes", &a.Eyes, maxAppearanceFieldLength},
			{"story.appearance.skin", &a.Skin, maxAppearanceFieldLength},
			{"story.appearance.hair", &a.Hair, maxAppearanceFieldLength},
		}
	}
	var err error
	for _, t := range texts {
		if *t.value, err = names.CleanText(*t.value, t.max); err != nil {
			return nil, &fieldError{field: t.field, err: err}
		}
	}
	for _, t := range lines {
		if *t.value, err = optionalLine(*t.value, t.max); err != nil {
			return nil, &fieldError{field: t.field, err: err}
		}
	}
	return st, nil
}

// optionalLine is names.Clean for a one-line field that may be empty.
func optionalLine(s string, maxLength int) (string, error) {
	if strings.TrimSpace(s) == "" {
		return "", nil
	}
	return names.Clean(s, maxLength)
}

// buildOf turns a full sheet into the rules.Build that package rules reads.
// It does not check anything: rules.Validate does, on writes, and
// rules.Derive reports what it cannot use as issues, on reads.
func buildOf(f *charactersv1.FullSheet) rules.Build {
	b := rules.Build{
		BaseScores:          abilityMap(f.GetBaseScores()),
		Race:                f.GetRaceKey(),
		Subrace:             f.GetSubraceKey(),
		Size:                f.GetSize(),
		SkillProficiencies:  f.GetSkillProficiencyKeys(),
		Expertise:           f.GetExpertiseSkillKeys(),
		ExtraAbilityBonuses: abilityMap(f.GetExtraAbilityBonuses()),
		Armor:               f.GetArmorKey(),
		Shield:              f.GetShield(),
		Weapons:             f.GetWeaponKeys(),
		Cantrips:            f.GetCantripKeys(),
		SpellsKnown:         f.GetKnownSpellKeys(),
		SpellsPrepared:      f.GetPreparedSpellKeys(),
		FeatureChoices:      f.GetFeatureChoiceKeys(),
		FeatureChoiceText:   f.GetFeatureChoiceText(),
		ToolProficiencies:   f.GetToolProficiencies(),
		Languages:           f.GetLanguages(),
		Feats:               f.GetFeatKeys(),
		FeatSlots:           f.GetFeatSlots(),
	}
	for _, c := range f.GetClasses() {
		b.Classes = append(b.Classes, rules.ClassLevel{
			Class:              c.GetClassKey(),
			Subclass:           c.GetSubclassKey(),
			CustomSubclassName: c.GetCustomSubclassName(),
			Level:              int(c.GetLevel()),
		})
	}
	switch bg := f.GetBackground().(type) {
	case *charactersv1.FullSheet_BackgroundKey:
		b.Background = bg.BackgroundKey
	case *charactersv1.FullSheet_CustomBackground:
		b.CustomBackgroundName = bg.CustomBackground.GetName()
		b.CustomBackgroundSkills = bg.CustomBackground.GetSkillKeys()
		b.CustomBackgroundProficiencies = bg.CustomBackground.GetProficiencyKeys()
		b.CustomBackgroundFeatureName = bg.CustomBackground.GetFeatureName()
		b.CustomBackgroundFeature = bg.CustomBackground.GetFeatureText()
		b.CustomBackgroundEquipment = bg.CustomBackground.GetEquipment()
	}
	if f.GetHitPoints().GetMethod() == charactersv1.HitPointsMethod_HIT_POINTS_METHOD_ROLLED {
		b.HitPoints.Method = rules.HitPointsRolled
	} // UNSPECIFIED and AVERAGE are rules.HitPointsFixed, the zero value
	for _, r := range f.GetHitPoints().GetRolls() {
		b.HitPoints.Rolls = append(b.HitPoints.Rolls, int(r))
	}
	return b
}

// abilityMap turns AbilityScores into the map package rules reads, with all
// six abilities (a missing message reads as zeros, which rules.Validate
// refuses for base scores).
func abilityMap(s *rulesv1.AbilityScores) map[rules.Ability]int {
	return map[rules.Ability]int{
		rules.STR: int(s.GetStrength()),
		rules.DEX: int(s.GetDexterity()),
		rules.CON: int(s.GetConstitution()),
		rules.INT: int(s.GetIntelligence()),
		rules.WIS: int(s.GetWisdom()),
		rules.CHA: int(s.GetCharisma()),
	}
}

// The sheet and the story are stored as protojson with the proto field
// names, and read back ignoring fields this version does not know, so a
// sheet written by a newer server still opens.
var (
	storeJSON = protojson.MarshalOptions{UseProtoNames: true}
	loadJSON  = protojson.UnmarshalOptions{DiscardUnknown: true}
)

// errCorruptDocument means a stored sheet or story cannot be read back: a
// bug, never a user's mistake.
var errCorruptDocument = errors.New("a stored character document cannot be read")

// loadSheet reads a stored sheet. The error never carries the document,
// which holds what the player typed.
func loadSheet(characterID string, doc []byte) (*charactersv1.CharacterSheet, error) {
	sheet := &charactersv1.CharacterSheet{}
	if err := loadJSON.Unmarshal(doc, sheet); err != nil {
		return nil, fmt.Errorf("%w: the sheet of character %s", errCorruptDocument, characterID)
	}
	if basic := sheet.GetBasic(); basic != nil {
		upgradeLegacyAttack(basic)
	}
	return sheet, nil
}

// loadStory reads a stored story, like loadSheet.
func loadStory(characterID string, doc []byte) (*charactersv1.CharacterStory, error) {
	story := &charactersv1.CharacterStory{}
	if err := loadJSON.Unmarshal(doc, story); err != nil {
		return nil, fmt.Errorf("%w: the story of character %s", errCorruptDocument, characterID)
	}
	return story, nil
}
