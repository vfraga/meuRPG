package characters

import (
	"slices"
	"testing"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

const (
	testTome       = "feature:pact-of-the-tome"
	testBook       = "feature:eldritch-invocation-book-of-ancient-secrets"
	testInvocation = "feature:eldritch-invocation-"
)

// tomeWarlock is a warlock of the Tome at level 4, with two invocations that are not the book, ready to go to level 5.
func tomeWarlock(t *testing.T, tb *levelUpTable) *charactersv1.Character {
	t.Helper()
	sheet := warlockPicksSheet(4, 3, testTome, testInvocation+"devils-sight", testInvocation+"beast-speech",
		rules.ScopedChoice(testTome+"#cantrips", "spell:guidance"),
		rules.ScopedChoice(testTome+"#cantrips", "spell:shillelagh"),
		rules.ScopedChoice(testTome+"#cantrips", "spell:eldritch-blast"))
	sheet.GetFull().KnownSpellKeys = []string{"spell:charm-person", "spell:expeditious-retreat", "spell:hellish-rebuke", "spell:protection-from-evil-and-good", "spell:darkness"}
	sheet.GetFull().ExperiencePoints = 14000
	c := tb.other.create(t, tb.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Vexa", sheet)
	tb.h.lockSheets(tb.campaign)
	return tb.other.get(t, tb.campaign, c.GetId())
}

// TestTheBookTakenWithoutItsRitualsGoesUpThroughTheLevelUpCall: the level that takes the Book of Ancient Secrets
// does not have to answer its two rituals; the next level lists them as a late choice and refuses without them.
func TestTheBookTakenWithoutItsRitualsGoesUpThroughTheLevelUpCall(t *testing.T) {
	t.Parallel()
	tb := newLevelUpTable(t, 0)
	c := tomeWarlock(t, tb)
	up, err := tb.levelUp(tb.other, c, &charactersv1.LevelUpChoices{
		ClassKey:          "class:warlock",
		KnownSpellKeys:    []string{"spell:hold-person"},
		FeatureChoiceKeys: []string{testBook},
		HitPoints:         &charactersv1.LevelUpHitPoints{Method: charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_AVERAGE},
	})
	if err != nil {
		t.Fatalf("the level that takes the book with no picks: %v", err)
	}
	if got := up.GetSheet().GetFull().GetFeatureChoiceKeys(); !slices.Contains(got, testBook) {
		t.Fatalf("the sheet after the level lacks the book: %v", got)
	}
	o, err := tb.options(tb.other, up)
	if err != nil {
		t.Fatalf("GetLevelUpOptions: %v", err)
	}
	var late []string
	for _, g := range o.GetLateChoices() {
		for _, ch := range g.GetChoices() {
			late = append(late, ch.GetKey())
		}
	}
	if !slices.Contains(late, testBook+"#rituals") {
		t.Fatalf("the next level's late choices = %v, want the book's rituals", late)
	}
	_, err = tb.levelUp(tb.other, up, &charactersv1.LevelUpChoices{
		ClassKey: "class:warlock", KnownSpellKeys: []string{"spell:fear"},
		HitPoints: &charactersv1.LevelUpHitPoints{Method: charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_AVERAGE},
	})
	wantLevelUpErr(t, "the next level with no picks", err, levelUpRefused(&charactersv1.LevelUpRefusal{
		Field: "full.feature_choice_keys", Reason: charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_LATE_CHOICE_MISSING,
	}))
}

// TestALevelTwentyWarlockWhoSwapsTheBookInAnswersItsRitualsAfterwards: the last level swaps an invocation for the
// Book of Ancient Secrets, and the two rituals are answered through CompleteCharacterChoices.
func TestALevelTwentyWarlockWhoSwapsTheBookInAnswersItsRitualsAfterwards(t *testing.T) {
	t.Parallel()
	tb := newLevelUpTable(t, 0)
	picks := []string{
		testTome, testInvocation + "devils-sight", testInvocation + "beast-speech", testInvocation + "armor-of-shadows",
		testInvocation + "fiendish-vigor", testInvocation + "mask-of-many-faces", testInvocation + "eldritch-sight",
		testInvocation + "eyes-of-the-rune-keeper", testInvocation + "gaze-of-two-minds",
		rules.ScopedChoice(testTome+"#cantrips", "spell:guidance"),
		rules.ScopedChoice(testTome+"#cantrips", "spell:shillelagh"),
		rules.ScopedChoice(testTome+"#cantrips", "spell:eldritch-blast"),
		rules.ScopedChoice("feature:mystic-arcanum-6th-level", "spell:eyebite"),
		rules.ScopedChoice("feature:mystic-arcanum-7th-level", "spell:finger-of-death"),
		rules.ScopedChoice("feature:mystic-arcanum-8th-level", "spell:dominate-monster"),
		rules.ScopedChoice("feature:mystic-arcanum-9th-level", "spell:foresight"),
	}
	sheet := warlockPicksSheet(19, 4, picks...)
	sheet.GetFull().ExperiencePoints = 355000
	c := tb.other.create(t, tb.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Vexa", sheet)
	tb.h.lockSheets(tb.campaign)
	c = tb.other.get(t, tb.campaign, c.GetId())
	up, err := tb.levelUp(tb.other, c, &charactersv1.LevelUpChoices{
		ClassKey:          "class:warlock",
		SwapInvocationKey: testInvocation + "devils-sight",
		FeatureChoiceKeys: []string{testBook},
		HitPoints:         &charactersv1.LevelUpHitPoints{Method: charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_AVERAGE},
	})
	if err != nil {
		t.Fatalf("the last level that swaps the book in: %v", err)
	}
	done, err := (&choiceTable{h: tb.h, campaign: tb.campaign}).complete(tb.other, up,
		pick(testBook+"#rituals", "spell:detect-magic", "spell:find-familiar"))
	if err != nil {
		t.Fatalf("CompleteCharacterChoices with the book's rituals: %v", err)
	}
	got := done.GetSheet().GetFull().GetFeatureChoiceKeys()
	for _, want := range []string{rules.ScopedChoice(testBook+"#rituals", "spell:detect-magic"), rules.ScopedChoice(testBook+"#rituals", "spell:find-familiar")} {
		if !slices.Contains(got, want) {
			t.Errorf("the sheet lacks the pick %s: %v", want, got)
		}
	}
}
