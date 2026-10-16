package characters

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The characterization of checkSheet (with cleanFullSheet, cleanBasicSheet, checkAttack and
// checkChallenge): each refusal as the client gets it, with the field the web marks, and
// the order of the checks. A sheet with every fault is refused for the first one; fixing it
// shows the next.

// editorSheetSteps runs a cascade of checkSheet over sheet.
func editorSheetSteps(t *testing.T, sheet *charactersv1.CharacterSheet, steps []editorSheetStep) {
	t.Helper()
	content := loadRules(t)
	for i, step := range steps {
		_, err := checkSheet(content, sheet)
		wantEditorRefusal(t, "step "+strconv.Itoa(i), invalidArgument(err), step.want)
		if step.fix != nil {
			step.fix()
		}
	}
}

type editorSheetStep struct {
	want string
	fix  func()
}

// editorField is the answer for a fieldError at field.
func editorFieldRefusal(field, rule string) string {
	return `invalid_argument: ` + field + ` ` + rule + ` | InvalidField{field="` + field + `"}`
}

func TestCheckSheetEditorFullSheetRefusalsComeInOrder(t *testing.T) {
	t.Parallel()
	sheet := pensantusSheet()
	f := sheet.GetFull()
	f.Classes[0].Subclass = &charactersv1.ClassLevel_CustomSubclassName{CustomSubclassName: "a\nb"}
	cb := f.GetCustomBackground()
	cb.Name, cb.FeatureName, cb.FeatureText, cb.Equipment = " ", "a\nb", "a\x00b", strings.Repeat("x", rules.MaxCustomEquipmentLength+1)
	f.HitPoints = &charactersv1.HitPoints{Method: 7}
	f.Alignment = 99
	f.ExperiencePoints = -1
	f.Equipment = slices.Repeat([]*charactersv1.Item{{Name: "Tocha"}}, maxEquipmentItems+1)
	f.Coins = &charactersv1.Coins{Copper: -1, Platinum: maxCoins + 1}
	f.Languages = slices.Repeat([]string{"Élfico"}, maxFreeTextEntries+1)
	f.ToolProficiencies = []string{"Ferramentas de ladrão", "a\nb"}
	f.CustomFeaturesText = strings.Repeat("x", maxCustomFeaturesLength+1)
	texts := map[string]string{}
	for i := range rules.MaxChoiceTexts + 1 {
		texts["feature:x|"+strconv.Itoa(i)] = "Orcs"
	}
	f.FeatureChoiceText = texts
	f.BaseScores.Intelligence = 31
	f.ChallengeRating = "7/8"
	f.XpValue = -1
	editorSheetSteps(t, sheet, []editorSheetStep{
		{editorFieldRefusal("sheet.full.classes[0].custom_subclass_name", "must not contain the character U+000A"), func() {
			f.Classes[0].Subclass = &charactersv1.ClassLevel_CustomSubclassName{CustomSubclassName: " Escola do Farol "}
		}},
		{editorFieldRefusal("sheet.full.custom_background.name", "must not be empty"), func() { cb.Name = "Sábio" }},
		{editorFieldRefusal("sheet.full.custom_background.feature_name", "must not contain the character U+000A"), func() { cb.FeatureName = "" }},
		{editorFieldRefusal("sheet.full.custom_background.feature_text", "must not contain the character U+0000"), func() { cb.FeatureText = "" }},
		{editorFieldRefusal("sheet.full.custom_background.equipment", "must be at most 500 characters, got 501"), func() { cb.Equipment = "" }},
		{editorFieldRefusal("sheet.full.hit_points.method", "is not a known method"), func() { f.HitPoints = nil }},
		{editorFieldRefusal("sheet.full.alignment", "is not a known alignment"), func() { f.Alignment = 0 }},
		{editorFieldRefusal("sheet.full.experience_points", "must be 0 to 1000000"), func() { f.ExperiencePoints = 0 }},
		{editorFieldRefusal("sheet.full.equipment", "must have at most 100 items"), func() {
			f.Equipment = []*charactersv1.Item{nil, {Name: " "}}
		}},
		// A nil item reads as an empty one: checkSheet works on a clone, which has no nil.
		{editorFieldRefusal("sheet.full.equipment[0].name", "must not be empty"), func() { f.Equipment = f.Equipment[1:] }},
		{editorFieldRefusal("sheet.full.equipment[0].name", "must not be empty"), func() {
			f.Equipment = []*charactersv1.Item{{Name: "Tocha", Quantity: maxItemQuantity + 1}}
		}},
		{editorFieldRefusal("sheet.full.equipment[0].quantity", "must be 1 to 9999"), func() { f.Equipment = []*charactersv1.Item{{Name: "Tocha", Quantity: -1}} }},
		{editorFieldRefusal("sheet.full.equipment[0].quantity", "must be 1 to 9999"), func() { f.Equipment = []*charactersv1.Item{{Name: " Tocha "}} }},
		{editorFieldRefusal("sheet.full.coins.copper", "must be 0 to 1000000"), func() { f.Coins.Copper = 0 }},
		{editorFieldRefusal("sheet.full.coins.platinum", "must be 0 to 1000000"), func() { f.Coins.Platinum, f.Coins.Electrum = 0, -1 }},
		{editorFieldRefusal("sheet.full.coins.electrum", "must be 0 to 1000000"), func() { f.Coins.Electrum, f.Coins.Silver = 0, maxCoins+1 }},
		{editorFieldRefusal("sheet.full.coins.silver", "must be 0 to 1000000"), func() { f.Coins.Silver = 3 }},
		{editorFieldRefusal("sheet.full.languages", "must have at most 20 entries"), func() { f.Languages = []string{" Dracônico ", "a\x00"} }},
		{editorFieldRefusal("sheet.full.languages[1]", "must not contain the character U+0000"), func() { f.Languages = f.Languages[:1] }},
		{editorFieldRefusal("sheet.full.tool_proficiencies[1]", "must not contain the character U+000A"), func() {
			f.ToolProficiencies = slices.Repeat([]string{"Ferramentas"}, maxFreeTextEntries+1)
		}},
		{editorFieldRefusal("sheet.full.tool_proficiencies", "must have at most 20 entries"), func() { f.ToolProficiencies = nil }},
		{editorFieldRefusal("sheet.full.custom_features_text", "must be at most 5000 characters, got 5001"), func() { f.CustomFeaturesText = " Pesquisador. " }},
		{editorFieldRefusal("sheet.full.feature_choice_text", "must have at most 12 entries"), func() {
			f.FeatureChoiceText = map[string]string{"feature:x|1": "a\nb"}
		}},
		{editorFieldRefusal("sheet.full.feature_choice_text", "must not contain the character U+000A"), func() {
			f.FeatureChoiceText = map[string]string{"feature:x|1": "  ", "feature:x|2": strings.Repeat("y", rules.MaxChoiceTextLength+1)}
		}},
		{editorFieldRefusal("sheet.full.feature_choice_text", "must be at most 40 characters, got 41"), func() {
			f.FeatureChoiceText = map[string]string{"feature:x|1": "  "}
		}},
		{editorFieldRefusal("sheet.full.base_scores.intelligence", "must be 1 to 30"), func() { f.BaseScores.Intelligence = 16 }},
		{editorFieldRefusal("sheet.full.challenge_rating", "is not a challenge rating of the rules (0, 1/8, 1/4, 1/2, 1 to 30)"), func() { f.ChallengeRating = "1/8" }},
		{editorFieldRefusal("sheet.full.xp_value", "must be 0 to 1000000"), func() { f.XpValue = 25 }},
		{`ok`, nil},
	})
}

func TestCheckSheetEditorBasicSheetRefusalsComeInOrder(t *testing.T) {
	t.Parallel()
	sheet := basicSheet()
	b := sheet.GetBasic()
	a := b.Attacks[0]
	b.HitPointsMax, b.ArmorClass, b.SpeedFt, b.InitiativeBonus = 0, maxArmorClass+1, -1, maxInitiativeBonus+1
	b.Attacks = []*charactersv1.BasicAttack{a, a, a, a}
	b.AttackBonus, b.Size = maxAttackBonus+1, 99
	a.AttackBonus, a.DamageDiceCount, a.DamageDiceSides, a.DamageBonus = maxAttackRollBonus+1, 0, 7, minDamageBonus-1
	a.DamageType, a.RangeFt, a.Name = 0, maxRangeFt+1, "a\nb"
	b.Description = strings.Repeat("x", maxDescriptionLength+1)
	b.ChallengeRating, b.XpValue = "31", maxXPValue+1
	editorSheetSteps(t, sheet, []editorSheetStep{
		{editorFieldRefusal("sheet.basic.hit_points_max", "must be 1 to 9999"), func() { b.HitPointsMax = 7 }},
		{editorFieldRefusal("sheet.basic.armor_class", "must be 1 to 40"), func() { b.ArmorClass = 0 }},
		{editorFieldRefusal("sheet.basic.armor_class", "must be 1 to 40"), func() { b.ArmorClass = 15 }},
		{editorFieldRefusal("sheet.basic.speed_ft", "must be 0 to 300"), func() { b.SpeedFt = 30 }},
		{editorFieldRefusal("sheet.basic.initiative_bonus", "must be -10 to 20"), func() { b.InitiativeBonus = 2 }},
		{editorFieldRefusal("sheet.basic.attacks", "must have at most 3 attacks"), func() { b.Attacks = b.Attacks[:1] }},
		{editorFieldRefusal("sheet.basic.attack_bonus", "must be -10 to 30"), func() { b.AttackBonus = 4 }},
		{editorFieldRefusal("sheet.basic.size", "must be a creature size"), func() { b.Size = 0 }},
		{editorFieldRefusal("sheet.basic.attacks[0].attack_bonus", "must be -10 to 20"), func() { a.AttackBonus = 4 }},
		{editorFieldRefusal("sheet.basic.attacks[0].damage_dice_count", "must be 1 to 20"), func() { a.DamageDiceCount = 1 }},
		{editorFieldRefusal("sheet.basic.attacks[0].damage_dice_sides", "must be 4, 6, 8, 10 or 12"), func() { a.DamageDiceSides = 6 }},
		{editorFieldRefusal("sheet.basic.attacks[0].damage_bonus", "must be -20 to 40"), func() { a.DamageBonus = 2 }},
		{editorFieldRefusal("sheet.basic.attacks[0].damage_type", "is required"), func() { a.DamageType = 99 }},
		{editorFieldRefusal("sheet.basic.attacks[0].damage_type", "is required"), func() { a.DamageType = charactersv1.DamageType_DAMAGE_TYPE_SLASHING }},
		{editorFieldRefusal("sheet.basic.attacks[0].range_ft", "must be 0 to 600"), func() { a.RangeFt = 0 }},
		{editorFieldRefusal("sheet.basic.attacks[0].name", "must not contain the character U+000A"), func() { a.Name = " Cimitarra " }},
		{editorFieldRefusal("sheet.basic.description", "must be at most 2000 characters, got 2001"), func() {
			b.Description = " Um goblin nervoso. "
			b.Attacks, b.Damage = nil, strings.Repeat("d", maxDamageLength+1)
		}},
		{editorFieldRefusal("sheet.basic.damage", "must be at most 40 characters, got 41"), func() { b.Damage = " 1d6+2 " }},
		{editorFieldRefusal("sheet.basic.challenge_rating", "is not a challenge rating of the rules (0, 1/8, 1/4, 1/2, 1 to 30)"), func() { b.ChallengeRating = "" }},
		{editorFieldRefusal("sheet.basic.xp_value", "must be 0 to 1000000"), func() { b.XpValue = -1 }},
		{editorFieldRefusal("sheet.basic.xp_value", "must be 0 to 1000000"), func() { b.XpValue = 0 }},
		{`ok`, nil},
	})
}

// TestCheckSheetEditorCleansWhatItKeeps: what checkSheet returns besides a refusal: the
// trimmed texts, the server's own fields set or cleared, and the empty messages a client
// may send.
func TestCheckSheetEditorCleansWhatItKeeps(t *testing.T) {
	t.Parallel()
	content := loadRules(t)
	_, err := checkSheet(content, &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{}})
	wantEditorRefusal(t, "an empty full sheet", invalidArgument(err), editorFieldRefusal("sheet.full.base_scores.strength", "must be 1 to 30"))
	_, err = checkSheet(content, &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Basic{}})
	wantEditorRefusal(t, "an empty basic sheet", invalidArgument(err), editorFieldRefusal("sheet.basic.hit_points_max", "must be 1 to 9999"))
	nilBackground := pensantusSheet()
	nilBackground.GetFull().Background = &charactersv1.FullSheet_CustomBackground{}
	_, err = checkSheet(content, nilBackground)
	wantEditorRefusal(t, "an empty custom background", invalidArgument(err), editorFieldRefusal("sheet.full.custom_background.name", "must not be empty"))

	full := pensantusSheet()
	f := full.GetFull()
	f.Classes[0].Subclass = &charactersv1.ClassLevel_CustomSubclassName{CustomSubclassName: " Escola do Farol "}
	f.GetCustomBackground().FeatureName = " Pesquisador "
	f.FeatureChoiceText = map[string]string{"feature:x#1": "  ", "feature:x#2": " Orcs "}
	f.FeatKeys = []string{"feat:grappler"}
	f.FeatSlots = map[string]string{"feat:grappler": "class:wizard@4", "feat:alert": "class:wizard@8"}
	f.ContentBaselines = map[string]int32{"background:x": 3}
	f.ContentRevision = 9
	f.ToolProficiencies = []string{" Ferramentas de ladrão "}
	got, err := checkSheet(content, full)
	if err != nil {
		t.Fatalf("checkSheet() error = %v", err)
	}
	g := got.GetFull()
	for _, c := range []struct{ what, got, want string }{
		{"custom subclass", g.GetClasses()[0].GetCustomSubclassName(), "Escola do Farol"},
		{"feature name", g.GetCustomBackground().GetFeatureName(), "Pesquisador"},
		{"choice texts", editorField(g.ProtoReflect().Descriptor().Fields().ByName("feature_choice_text"), g.ProtoReflect().Get(g.ProtoReflect().Descriptor().Fields().ByName("feature_choice_text"))), `{feature:x#2:"Orcs"}`},
		{"feat slots", editorField(g.ProtoReflect().Descriptor().Fields().ByName("feat_slots"), g.ProtoReflect().Get(g.ProtoReflect().Descriptor().Fields().ByName("feat_slots"))), `{feat:grappler:"class:wizard@4"}`},
		{"baselines", strconv.Itoa(len(g.GetContentBaselines())), "0"},
		{"revision", strconv.Itoa(int(g.GetContentRevision())), "0"},
		{"tools", strings.Join(g.GetToolProficiencies(), ","), "Ferramentas de ladrão"},
		{"quantity 0", strconv.Itoa(int(g.GetEquipment()[1].GetQuantity())), "1"},
	} {
		if c.got != c.want {
			t.Errorf("%s = %s, want %s", c.what, c.got, c.want)
		}
	}
	if len(g.GetKnownIssues()) == 0 {
		t.Error("the known issues of a custom subclass are not recorded")
	}
	if f.GetClasses()[0].GetCustomSubclassName() != " Escola do Farol " {
		t.Error("checkSheet() changed the request's sheet")
	}

	basic := basicSheet()
	b := basic.GetBasic()
	b.MonsterKey, b.AbilityScores, b.CombatOnly = "monster:goblin", &rulesv1.AbilityScores{Strength: 8}, true
	b.AttackBonus, b.Damage = 4, "1d6"
	got, err = checkSheet(content, basic)
	if err != nil {
		t.Fatalf("checkSheet(basic) error = %v", err)
	}
	if gb := got.GetBasic(); gb.GetMonsterKey() != "" || gb.GetAbilityScores() != nil || gb.GetCombatOnly() || gb.GetAttackBonus() != 0 || gb.GetDamage() != "" {
		t.Errorf("checkSheet(basic) = %v, want the server's fields and the old attack cleared", gb)
	}
	b.Attacks, b.Damage = nil, " "
	got, err = checkSheet(content, basic)
	if err != nil || got.GetBasic().GetDamage() != "" || got.GetBasic().GetAttackBonus() != 4 {
		t.Errorf("checkSheet(basic, old attack) = %v, %v; want the blank damage emptied and the bonus kept", got.GetBasic(), err)
	}
}
