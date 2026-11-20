package characters

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/sessiontell"
)

// The swap of the options of a table feature at a level up (#715): the table's subclass "Mestre das Posturas" (of the SRD
// fighter) has two features whose options the player picks, and each says at which class levels a pick may be swapped.

const (
	styleDefenseKey = "feature:fighter-fighting-style-defense"
	styleDuelingKey = "feature:fighter-fighting-style-dueling"
)

var postureNames = []string{"Postura da Garça", "Postura do Touro", "Postura do Lobo", "Postura do Corvo", "Postura do Urso", "Postura do Falcão", "Postura do Veterano"}

// posturesFeature is the feature "Posturas": seven options, three picked, the last one asking for level 20 of the fighter.
func posturesFeature(swap *rulesv1.FeatureSwapLevels) *rulesv1.TableFeature {
	f := &rulesv1.TableFeature{
		NamePt: "Posturas", DescPt: []string{"Escolha posturas."}, SwapLevels: swap,
		Effects: []*rulesv1.TableEffect{{Type: "choice", Choice: "feature", Count: 3}},
	}
	for _, n := range postureNames {
		f.Options = append(f.Options, &rulesv1.TableFeature{NamePt: n, DescPt: []string{n + "."}})
	}
	// The Heron has a resource of its own and a stance.
	f.Options[0].Effects = []*rulesv1.TableEffect{
		{Type: "resource", Resource: "garca", Max: "2", Recharge: "long_rest"},
		{Type: "state", StateName: "garca", DurationRounds: "10", EndsOn: []string{"dismiss"}, Resource: "garca", Economy: "bonus_action"},
	}
	f.Options[len(f.Options)-1].Prerequisite = &rulesv1.OptionPrerequisite{ClassLevel: &rulesv1.OptionClassLevel{ClassKey: "class:fighter", Level: 20}}
	return f
}

// dodgesFeature is the feature "Esquivas": three options, one picked, with options of its own.
func dodgesFeature(swap *rulesv1.FeatureSwapLevels) *rulesv1.TableFeature {
	f := &rulesv1.TableFeature{
		NamePt: "Esquivas", DescPt: []string{"Escolha uma esquiva."}, SwapLevels: swap,
		Effects: []*rulesv1.TableEffect{{Type: "choice", Choice: "feature", Count: 1}},
	}
	for _, n := range []string{"Esquiva curta", "Esquiva longa", "Esquiva rolada"} {
		f.Options = append(f.Options, &rulesv1.TableFeature{NamePt: n, DescPt: []string{n + "."}})
	}
	return f
}

func swapAt(levels ...int32) *rulesv1.FeatureSwapLevels {
	return &rulesv1.FeatureSwapLevels{Levels: levels}
}

func swapEvery() *rulesv1.FeatureSwapLevels { return &rulesv1.FeatureSwapLevels{EveryLevel: true} }

// optionKey is the key the server made for the option of the feature with this name.
func optionKey(t *testing.T, f *rulesv1.TableFeature, name string) string {
	t.Helper()
	for _, o := range f.GetOptions() {
		if o.GetNamePt() == name {
			return o.GetKey()
		}
	}
	t.Fatalf("the feature %q has no option %q", f.GetNamePt(), name)
	return ""
}

// swapFixture is a table with the subclass written and a fighter of that subclass on a locked sheet.
type swapFixture struct {
	*levelUpTable
	entry             *rulesv1.TableEntry
	postures, dodges  *rulesv1.TableFeature
	heron, bull, wolf string
	crow, hawk, vet   string
	short, long       string
	fighter           *charactersv1.Character
}

// newSwapFixture writes the subclass with the swap levels of its features (the dodges only when dodge is not nil) and a fighter of
// class level level with the XP for the next level: Defense, three postures and, with the dodges, the short dodge.
func newSwapFixture(t *testing.T, level, xp int32, postures, dodge *rulesv1.FeatureSwapLevels) *swapFixture {
	t.Helper()
	return newSwapFixtureWith(t, level, xp, postures, dodge)
}

// newSwapFixtureWith is newSwapFixture for a character that has the classes in others too, after the fighter.
func newSwapFixtureWith(t *testing.T, level, xp int32, postures, dodge *rulesv1.FeatureSwapLevels, others ...*charactersv1.ClassLevel) *swapFixture {
	t.Helper()
	f := &swapFixture{levelUpTable: newLevelUpTable(t, 900)}
	features := []*rulesv1.TableFeature{posturesFeature(postures)}
	if dodge != nil {
		features = append(features, dodgesFeature(dodge))
	}
	f.entry = f.master.addEntry(t, f.campaign, &rulesv1.TableSubclass{
		NamePt: "Mestre das Posturas", ClassKey: "class:fighter", DescPt: []string{"Um caminho de teste."},
		Levels: []*rulesv1.TableSubclassLevel{{Level: 3, Features: features}},
	})
	made := f.entry.GetTableSubclass().GetLevels()[0].GetFeatures()
	f.postures = made[0]
	if dodge != nil {
		f.dodges = made[1]
		f.short, f.long = optionKey(t, f.dodges, "Esquiva curta"), optionKey(t, f.dodges, "Esquiva longa")
	}
	f.heron, f.bull, f.wolf = optionKey(t, f.postures, "Postura da Garça"), optionKey(t, f.postures, "Postura do Touro"), optionKey(t, f.postures, "Postura do Lobo")
	f.crow, f.hawk, f.vet = optionKey(t, f.postures, "Postura do Corvo"), optionKey(t, f.postures, "Postura do Falcão"), optionKey(t, f.postures, "Postura do Veterano")
	picks := []string{styleDefenseKey, f.heron, f.bull, f.wolf}
	if dodge != nil {
		picks = append(picks, f.short)
	}
	f.fighter = f.other.create(t, f.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Ícaro", &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
		BaseScores: &rulesv1.AbilityScores{Strength: 15, Dexterity: 14, Constitution: 14, Intelligence: 10, Wisdom: 12, Charisma: 8}, RaceKey: "race:human",
		Background: &charactersv1.FullSheet_CustomBackground{CustomBackground: &charactersv1.CustomBackground{
			Name: "Batedor de torre", SkillKeys: []string{"skill:perception", "skill:survival"}, ProficiencyKeys: []string{"proficiency:thieves-tools", "language:elvish"},
			FeatureName: "Olho no horizonte", FeatureText: "Você sempre acha o ponto mais alto de um lugar.", Equipment: "Uma luneta, um rolo de corda e 10 PO.",
		}},
		Classes:              append([]*charactersv1.ClassLevel{{ClassKey: "class:fighter", Level: level, Subclass: &charactersv1.ClassLevel_SubclassKey{SubclassKey: f.entry.GetKey()}}}, others...),
		Languages:            []string{"Anão"},
		SkillProficiencyKeys: []string{"skill:athletics", "skill:history"},
		FeatureChoiceKeys:    picks,
		HitPoints:            hitPointsAverage(), ExperiencePoints: xp,
	}}})
	f.h.lockSheets(f.campaign)
	f.fighter = f.other.get(t, f.campaign, f.fighter.GetId())
	return f
}

// choices is the level the fighter goes up with no swap: the average hit points and, at an Ability Score Improvement, +2 Strength.
func (f *swapFixture) choices(asi bool) *charactersv1.LevelUpChoices {
	ch := &charactersv1.LevelUpChoices{
		ClassKey: "class:fighter", HitPoints: &charactersv1.LevelUpHitPoints{Method: charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_AVERAGE},
	}
	if asi {
		ch.AbilityIncrease = &rulesv1.AbilityScores{Strength: 2}
	}
	return ch
}

func (f *swapFixture) fighterOptions(t *testing.T) *charactersv1.LevelUpOptions {
	t.Helper()
	o, err := f.options(f.other, f.fighter)
	if err != nil {
		t.Fatalf("GetLevelUpOptions() error = %v", err)
	}
	return o
}

// swapRefusal is the refusal the preview answers for the choices, nil for none.
func (f *swapFixture) swapRefusal(t *testing.T, ch *charactersv1.LevelUpChoices) *charactersv1.LevelUpRefusal {
	t.Helper()
	res, err := f.preview(f.other, f.fighter, ch)
	if err != nil {
		t.Fatalf("PreviewLevelUp() error = %v", err)
	}
	return res.GetRefusal()
}

func featureSwap(group, from, to string) *charactersv1.LevelUpPickSwap {
	return &charactersv1.LevelUpPickSwap{Group: group, FromKey: from, ToKey: to}
}

func groupKeys(ks []*charactersv1.LevelUpNamedKey) []string {
	var out []string
	for _, k := range ks {
		out = append(out, k.GetKey())
	}
	return out
}

// A feature that allows a swap at level 5 offers it to the player at level 5, a level with no Ability Score Improvement, on a
// table that does not use the swap rule; the sheet then has the new option, and the record names the swap.
func TestAPlayerSwapsAFeatureOptionAtAListedLevelWithTheTableRuleOff(t *testing.T) {
	t.Parallel()
	f := newSwapFixture(t, 4, 6500, swapAt(5), nil)
	o := f.fighterOptions(t)
	g := groupOf(o, f.postures.GetKey())
	if o.GetAbilityScoreImprovement() || g == nil || len(o.GetSwapGroups()) != 1 {
		t.Fatalf("level 5 offers %v, want the Postures group alone", o.GetSwapGroups())
	}
	if g.GetNamePt() != "Posturas" || !slices.Equal(groupKeys(g.GetPicks()), []string{f.heron, f.bull, f.wolf}) ||
		!slices.Equal(groupKeys(g.GetPool()), []string{f.crow, optionKey(t, f.postures, "Postura do Urso"), f.hawk, f.vet}) {
		t.Fatalf("group = %v, want the three picks and the four options left", g)
	}

	ch := f.choices(false)
	ch.SwapFeatureOptions = []*charactersv1.LevelUpPickSwap{featureSwap(f.postures.GetKey(), f.heron, f.crow)}
	if r := f.swapRefusal(t, ch); r != nil {
		t.Fatalf("preview of the swap: refusal = %v", r)
	}
	up, err := f.levelUp(f.other, f.fighter, ch)
	if err != nil {
		t.Fatalf("LevelUpCharacter() error = %v", err)
	}
	picks := up.GetSheet().GetFull().GetFeatureChoiceKeys()
	if slices.Contains(picks, f.heron) || !slices.Contains(picks, f.crow) || !slices.Contains(picks, f.bull) || !slices.Contains(picks, styleDefenseKey) || len(picks) != 4 {
		t.Errorf("picks after the swap = %v, want the Crow for the Heron", picks)
	}

	// The record of the level holds the swap and names its keys; nothing else changed.
	records, err := f.levelUps(f.master, f.fighter.GetId())
	if err != nil || len(records) != 1 {
		t.Fatalf("level ups = %v, %v, want one", records, err)
	}
	rec := records[0]
	if got := rec.GetChoices().GetSwapFeatureOptions(); len(got) != 1 || got[0].GetGroup() != f.postures.GetKey() || got[0].GetFromKey() != f.heron || got[0].GetToKey() != f.crow {
		t.Errorf("record swaps = %v", got)
	}
	for _, k := range []string{f.heron, f.crow} {
		if rec.GetNamesPt()[k] == "" {
			t.Errorf("the record names %v, want the name of %s", rec.GetNamesPt(), k)
		}
	}
}

func groupOf(o *charactersv1.LevelUpOptions, group string) *charactersv1.LevelUpSwapGroup {
	for _, g := range o.GetSwapGroups() {
		if g.GetGroup() == group {
			return g
		}
	}
	return nil
}

// A feature that swaps at every level offers it at an Ability Score Improvement as well, and an earlier level's record
// is not rewritten by the next one.
func TestAFeatureSwapAtEveryLevelLeavesEarlierRecordsAlone(t *testing.T) {
	t.Parallel()
	f := newSwapFixture(t, 4, 14000, swapEvery(), nil)
	first := f.choices(false)
	first.SwapFeatureOptions = []*charactersv1.LevelUpPickSwap{featureSwap(f.postures.GetKey(), f.wolf, f.hawk)}
	up, err := f.levelUp(f.other, f.fighter, first)
	if err != nil {
		t.Fatalf("level 5: %v", err)
	}
	before, _ := f.levelUps(f.master, up.GetId())

	// Level 6 has an Ability Score Improvement and the table's rule is off: the Postures group is offered and no other.
	f.fighter = up
	o := f.fighterOptions(t)
	if !o.GetAbilityScoreImprovement() || len(o.GetSwapGroups()) != 1 || groupOf(o, f.postures.GetKey()) == nil {
		t.Fatalf("level 6 offers %v (ASI %v), want the Postures group only", o.GetSwapGroups(), o.GetAbilityScoreImprovement())
	}
	second := f.choices(true)
	second.SwapFeatureOptions = []*charactersv1.LevelUpPickSwap{featureSwap(f.postures.GetKey(), f.bull, f.crow)}
	up, err = f.levelUp(f.other, up, second)
	if err != nil {
		t.Fatalf("level 6: %v", err)
	}
	picks := up.GetSheet().GetFull().GetFeatureChoiceKeys()
	if !slices.Contains(picks, f.crow) || slices.Contains(picks, f.wolf) {
		t.Errorf("picks after two swaps = %v", picks)
	}
	records, _ := f.levelUps(f.master, up.GetId())
	if len(records) != 2 {
		t.Fatalf("level ups = %d, want 2", len(records))
	}
	if !slices.ContainsFunc(records, func(r *charactersv1.LevelUp) bool {
		return r.GetToLevel() == 5 && r.GetChoices().GetSwapFeatureOptions()[0].GetToKey() == f.hawk && r.GetNamesPt()[f.hawk] != ""
	}) || !proto.Equal(records[1], before[0]) {
		t.Errorf("the level 5 record changed: %v, was %v", records, before)
	}
}

// A swap outside the levels the feature lists, or one the group does not offer, is refused as a swap that is not allowed.
func TestAFeatureSwapThatIsNotOfferedIsRefused(t *testing.T) {
	t.Parallel()
	f := newSwapFixture(t, 4, 6500, swapAt(6), swapAt(5))
	o := f.fighterOptions(t)
	if groupOf(o, f.postures.GetKey()) != nil || groupOf(o, f.dodges.GetKey()) == nil || len(o.GetSwapGroups()) != 1 {
		t.Fatalf("level 5 offers %v, want the Dodges only", o.GetSwapGroups())
	}
	cases := []struct {
		name  string
		swaps []*charactersv1.LevelUpPickSwap
	}{
		{"a feature that does not allow a swap at this level", []*charactersv1.LevelUpPickSwap{featureSwap(f.postures.GetKey(), f.heron, f.crow)}},
		{"a feature that does not exist", []*charactersv1.LevelUpPickSwap{featureSwap("feature:nada@mesa", f.heron, f.crow)}},
		{"a pick the sheet does not have", []*charactersv1.LevelUpPickSwap{featureSwap(f.dodges.GetKey(), f.long, f.short)}},
		{"an option that is already picked", []*charactersv1.LevelUpPickSwap{featureSwap(f.dodges.GetKey(), f.short, f.short)}},
		{"an option of another feature", []*charactersv1.LevelUpPickSwap{featureSwap(f.dodges.GetKey(), f.short, f.crow)}},
		{"the same feature twice", []*charactersv1.LevelUpPickSwap{
			featureSwap(f.dodges.GetKey(), f.short, f.long), featureSwap(f.dodges.GetKey(), f.short, optionKey(t, f.dodges, "Esquiva rolada")),
		}},
	}
	for _, tc := range cases {
		ch := f.choices(false)
		ch.SwapFeatureOptions = tc.swaps
		r := f.swapRefusal(t, ch)
		if r.GetReason() != charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_SWAP_NOT_ALLOWED || r.GetField() != "choices.swap_feature_options" {
			t.Errorf("%s: preview refusal = %v", tc.name, r)
		}
		_, err := f.levelUp(f.other, f.fighter, ch)
		if r := refusal(t, "LevelUpCharacter", err); r.GetReason() != charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_SWAP_NOT_ALLOWED {
			t.Errorf("%s: level-up refusal = %v", tc.name, r)
		}
	}
	// The sheet stayed as it was.
	if again := f.other.get(t, f.campaign, f.fighter.GetId()); again.GetRevision() != f.fighter.GetRevision() {
		t.Errorf("a refused level-up changed the sheet: revision %d, was %d", again.GetRevision(), f.fighter.GetRevision())
	}
}

// The option taken is judged as a pick is: an option whose prerequisite the character does not meet is refused at its own field.
func TestAFeatureSwapTakesNoOptionWhosePrerequisiteIsUnmet(t *testing.T) {
	t.Parallel()
	f := newSwapFixture(t, 4, 6500, swapAt(5), nil)
	ch := f.choices(false)
	ch.SwapFeatureOptions = []*charactersv1.LevelUpPickSwap{featureSwap(f.postures.GetKey(), f.heron, f.vet)}
	r := f.swapRefusal(t, ch)
	if r.GetReason() != charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_FEATURE_CHOICE || r.GetField() != "full.feature_choice_keys" {
		t.Errorf("refusal = %v, want feature_choice at the picks", r)
	}
	if o := f.fighterOptions(t); !slices.Contains(groupKeys(groupOf(o, f.postures.GetKey()).GetPool()), f.vet) {
		t.Error("the blocked option is not in the pool: it stays listed, and the preview refuses it")
	}
}

// Two features that allow a swap at the same level each have a group of their own and take one swap each.
func TestTwoFeaturesSwapAtTheSameLevelWithTheirOwnGroups(t *testing.T) {
	t.Parallel()
	f := newSwapFixture(t, 4, 6500, swapAt(5), swapAt(5))
	o := f.fighterOptions(t)
	if len(o.GetSwapGroups()) != 2 || groupOf(o, f.postures.GetKey()) == nil || groupOf(o, f.dodges.GetKey()) == nil {
		t.Fatalf("level 5 offers %v, want a group for each feature", o.GetSwapGroups())
	}
	if pool := groupKeys(groupOf(o, f.dodges.GetKey()).GetPool()); !slices.Equal(pool, []string{f.long, optionKey(t, f.dodges, "Esquiva rolada")}) {
		t.Errorf("the dodges' pool = %v, want its own options", pool)
	}
	ch := f.choices(false)
	ch.SwapFeatureOptions = []*charactersv1.LevelUpPickSwap{
		featureSwap(f.postures.GetKey(), f.bull, f.hawk), featureSwap(f.dodges.GetKey(), f.short, f.long),
	}
	up, err := f.levelUp(f.other, f.fighter, ch)
	if err != nil {
		t.Fatalf("two swaps: %v", err)
	}
	picks := up.GetSheet().GetFull().GetFeatureChoiceKeys()
	for _, k := range []string{f.hawk, f.long, f.heron, f.wolf, styleDefenseKey} {
		if !slices.Contains(picks, k) {
			t.Errorf("%s is not among the picks %v", k, picks)
		}
	}
	for _, k := range []string{f.bull, f.short} {
		if slices.Contains(picks, k) {
			t.Errorf("%s was not given up: %v", k, picks)
		}
	}

	// The same pick given up twice is refused.
	g := newSwapFixture(t, 4, 6500, swapAt(5), swapAt(5))
	twice := g.choices(false)
	twice.SwapFeatureOptions = []*charactersv1.LevelUpPickSwap{
		featureSwap(g.postures.GetKey(), g.bull, g.hawk), featureSwap(g.dodges.GetKey(), g.bull, g.long),
	}
	if r := g.swapRefusal(t, twice); r.GetReason() != charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_SWAP_NOT_ALLOWED {
		t.Errorf("a pick given up twice: refusal = %v", r)
	}
}

// With the table's rule on, the swap of a feature goes with the rule's own swap of a pick at the same level.
func TestAFeatureSwapGoesWithThePickSwapOfTheTableRule(t *testing.T) {
	t.Parallel()
	f := newSwapFixture(t, 5, 14000, swapEvery(), nil)
	setRules(t, f.master, f.campaign, func(r *campaignsv1.TableRules) { r.PickSwapAllowed = true })
	o := f.fighterOptions(t)
	if groupOf(o, "fighting_style") == nil || groupOf(o, f.postures.GetKey()) == nil {
		t.Fatalf("level 6 offers %v, want the fighting style and the Postures", o.GetSwapGroups())
	}
	ch := f.choices(true)
	ch.SwapPick = featureSwap("fighting_style", styleDefenseKey, styleDuelingKey)
	ch.SwapFeatureOptions = []*charactersv1.LevelUpPickSwap{featureSwap(f.postures.GetKey(), f.wolf, f.crow)}
	up, err := f.levelUp(f.other, f.fighter, ch)
	if err != nil {
		t.Fatalf("both swaps: %v", err)
	}
	picks := up.GetSheet().GetFull().GetFeatureChoiceKeys()
	for _, k := range []string{styleDuelingKey, f.crow, f.heron, f.bull} {
		if !slices.Contains(picks, k) {
			t.Errorf("%s is not among the picks %v", k, picks)
		}
	}
	if slices.Contains(picks, styleDefenseKey) || slices.Contains(picks, f.wolf) {
		t.Errorf("a given-up pick is still there: %v", picks)
	}
}

// Another class gaining the level offers no swap of the fighter's features.
func TestAnotherClassGainingTheLevelOffersNoFeatureSwapOfTheFightersFeatures(t *testing.T) {
	t.Parallel()
	f := newSwapFixture(t, 4, 6500, swapEvery(), nil)
	if o := f.fighterOptions(t); groupOf(o, f.postures.GetKey()) == nil {
		t.Fatal("the fighter's own level offers no group")
	}
	f.withScores(t, f.fighter, &rulesv1.AbilityScores{Strength: 15, Dexterity: 14, Constitution: 14, Intelligence: 13, Wisdom: 12, Charisma: 8})
	f.fighter = f.other.get(t, f.campaign, f.fighter.GetId())
	o, err := f.optionsFor(f.other, f.fighter, "class:wizard")
	if err != nil || !o.GetIsNewClass() || len(o.GetSwapGroups()) != 0 {
		t.Errorf("a new class offers %v (new class %v), %v; want no swap groups", o.GetSwapGroups(), o.GetIsNewClass(), err)
	}
}

// The key lengths of the swaps are bounded.
func TestFeatureSwapKeysAreBounded(t *testing.T) {
	t.Parallel()
	f := newSwapFixture(t, 4, 6500, swapAt(5), nil)
	ch := f.choices(false)
	ch.SwapFeatureOptions = []*charactersv1.LevelUpPickSwap{featureSwap(f.postures.GetKey(), strings.Repeat("x", 500), f.crow)}
	_, err := f.preview(f.other, f.fighter, ch)
	wantCode(t, "PreviewLevelUp", err, connect.CodeInvalidArgument)
}

// optionSwapSpy is the module that hears the swaps: it records the calls and answers err.
type optionSwapSpy struct {
	mu    sync.Mutex
	calls []optionSwapCall
	err   error
}

type optionSwapCall struct {
	campaign, character string
	swaps               []OptionSwap
	inTx                bool
}

func (s *optionSwapSpy) OptionsGivenUp(_ context.Context, tx pgx.Tx, campaignID, characterID string, swaps []OptionSwap, _ sessiontell.Tell) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, optionSwapCall{campaign: campaignID, character: characterID, swaps: slices.Clone(swaps), inTx: tx != nil})
	return s.err
}

// The module wired to hear the swaps is called in the level-up's transaction with every swap of the level, only for a level
// that swaps; its error aborts the level.
func TestTheModuleThatHearsTheSwapsIsToldInTheLevelUpTransaction(t *testing.T) {
	t.Parallel()
	f := newSwapFixture(t, 4, 14000, swapEvery(), swapEvery())
	spy := &optionSwapSpy{}
	f.h.svc.SetOptionSwaps(spy)

	// A level with no swap, and a preview of one, call nobody.
	swapped := f.choices(false)
	swapped.SwapFeatureOptions = []*charactersv1.LevelUpPickSwap{
		featureSwap(f.postures.GetKey(), f.bull, f.hawk), featureSwap(f.dodges.GetKey(), f.short, f.long),
	}
	if r := f.swapRefusal(t, swapped); r != nil {
		t.Fatalf("preview: refusal = %v", r)
	}
	plain, err := f.levelUp(f.other, f.fighter, f.choices(false))
	if err != nil {
		t.Fatalf("level 5 with no swap: %v", err)
	}
	if len(spy.calls) != 0 {
		t.Fatalf("called %d times for a preview and a level with no swap, want 0", len(spy.calls))
	}

	// A swap that the module refuses aborts the level: the sheet, the record and the revision stay.
	spy.err = errors.New("the module is down")
	if _, err := f.levelUp(f.other, plain, swapped); err == nil {
		t.Fatal("the level-up went through though the module refused")
	}
	if len(spy.calls) != 1 {
		t.Fatalf("calls = %d for the level the module refused, want 1", len(spy.calls))
	}
	if again := f.other.get(t, f.campaign, plain.GetId()); again.GetRevision() != plain.GetRevision() || again.GetSheet().GetFull().GetClasses()[0].GetLevel() != 5 {
		t.Errorf("the aborted level changed the sheet: revision %d (was %d), level %d", again.GetRevision(), plain.GetRevision(), again.GetSheet().GetFull().GetClasses()[0].GetLevel())
	}
	if records, _ := f.levelUps(f.master, plain.GetId()); len(records) != 1 {
		t.Errorf("level ups = %d after the aborted level, want the one before", len(records))
	}

	// With the module well, it hears both swaps once.
	spy.err = nil
	spy.calls = nil
	if _, err := f.levelUp(f.other, plain, swapped); err != nil {
		t.Fatalf("level 6 with two swaps: %v", err)
	}
	if len(spy.calls) != 1 {
		t.Fatalf("calls = %d for the level that went through, want 1", len(spy.calls))
	}
	got := spy.calls[0]
	want := []OptionSwap{
		{FeatureKey: f.postures.GetKey(), GivenUp: f.bull, Taken: f.hawk, ClassKey: "class:fighter", ToLevel: 6},
		{FeatureKey: f.dodges.GetKey(), GivenUp: f.short, Taken: f.long, ClassKey: "class:fighter", ToLevel: 6},
	}
	if got.campaign != f.campaign || got.character != f.fighter.GetId() || !got.inTx || !slices.Equal(got.swaps, want) {
		t.Errorf("heard %+v, want both swaps of the fighter in the transaction", got)
	}
}

// A content pack carries the swap levels of a feature, and a table that imports it can swap the options, those of the
// features and the table options a feature offers.
func TestTheSwapLevelsOfAFeatureTravelInAContentPack(t *testing.T) {
	t.Parallel()
	pt := newPackTable(t)
	pt.h.svc.SetLevelUps(xpLevelUps{})
	entry := pt.master.addEntry(t, pt.from, &rulesv1.TableSubclass{
		NamePt: "Mestre das Posturas", ClassKey: "class:fighter", DescPt: []string{"Um caminho de teste."},
		Levels: []*rulesv1.TableSubclassLevel{{Level: 3, Features: []*rulesv1.TableFeature{posturesFeature(swapEvery())}}},
	})
	feature := entry.GetTableSubclass().GetLevels()[0].GetFeatures()[0]
	golpe := pt.master.addEntry(t, pt.from, &rulesv1.TableOption{
		NamePt: "Golpe do Corvo", Set: "table_feature", OfferedBy: feature.GetKey(), DescPt: []string{"Um golpe."},
	})

	pack := pt.export(t, pt.from)
	file, err := storeJSON.Marshal(pack)
	if err != nil || !strings.Contains(string(file), "swap_levels") || !strings.Contains(string(file), "every_level") {
		t.Fatalf("the pack file = %v, %v, want the swap levels in it", string(file), err)
	}
	var back rulesv1.TableContentPack
	if err := (protojson.UnmarshalOptions{DiscardUnknown: false}).Unmarshal(file, &back); err != nil {
		t.Fatalf("the pack does not read back: %v", err)
	}
	pt.apply(t, pt.to, &back, "")
	imported := pt.master.entries(t, pt.to)[entry.GetKey()]
	if imported == nil || !imported.GetTableSubclass().GetLevels()[0].GetFeatures()[0].GetSwapLevels().GetEveryLevel() {
		t.Fatalf("the imported subclass = %v, want the swap levels kept", imported)
	}

	// A fighter of the imported subclass swaps for the imported table option.
	heron, bull, wolf := optionKey(t, feature, "Postura da Garça"), optionKey(t, feature, "Postura do Touro"), optionKey(t, feature, "Postura do Lobo")
	pc := pt.player.create(t, pt.to, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Ícaro", &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
		BaseScores: &rulesv1.AbilityScores{Strength: 15, Dexterity: 14, Constitution: 14, Intelligence: 10, Wisdom: 12, Charisma: 8}, RaceKey: "race:human",
		Background: &charactersv1.FullSheet_CustomBackground{CustomBackground: &charactersv1.CustomBackground{
			Name: "Batedor de torre", SkillKeys: []string{"skill:perception", "skill:survival"}, ProficiencyKeys: []string{"proficiency:thieves-tools", "language:elvish"},
			FeatureName: "Olho no horizonte", FeatureText: "Você sempre acha o ponto mais alto de um lugar.", Equipment: "Uma luneta, um rolo de corda e 10 PO.",
		}},
		Classes:              []*charactersv1.ClassLevel{{ClassKey: "class:fighter", Level: 4, Subclass: &charactersv1.ClassLevel_SubclassKey{SubclassKey: entry.GetKey()}}},
		Languages:            []string{"Anão"},
		SkillProficiencyKeys: []string{"skill:athletics", "skill:history"},
		FeatureChoiceKeys:    []string{styleDefenseKey, heron, bull, wolf},
		HitPoints:            hitPointsAverage(), ExperiencePoints: 6500,
	}}})
	pt.h.lockSheets(pt.to)
	pc = pt.player.get(t, pt.to, pc.GetId())

	res, err := pt.player.api.GetLevelUpOptions(t.Context(), connect.NewRequest(&charactersv1.GetLevelUpOptionsRequest{CampaignId: pt.to, CharacterId: pc.GetId()}))
	if err != nil {
		t.Fatalf("GetLevelUpOptions() error = %v", err)
	}
	g := groupOf(res.Msg.GetOptions(), feature.GetKey())
	if g == nil || !slices.Contains(groupKeys(g.GetPool()), golpe.GetKey()) {
		t.Fatalf("the imported group = %v, want the table option %s in its pool", g, golpe.GetKey())
	}
	up, err := pt.player.api.LevelUpCharacter(t.Context(), connect.NewRequest(&charactersv1.LevelUpCharacterRequest{
		CampaignId: pt.to, CharacterId: pc.GetId(), Revision: pc.GetRevision(), Choices: &charactersv1.LevelUpChoices{
			ClassKey: "class:fighter", HitPoints: &charactersv1.LevelUpHitPoints{Method: charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_AVERAGE},
			SwapFeatureOptions: []*charactersv1.LevelUpPickSwap{featureSwap(feature.GetKey(), heron, golpe.GetKey())},
		},
	}))
	if err != nil {
		t.Fatalf("a swap for the imported option: %v", err)
	}
	if picks := up.Msg.GetCharacter().GetSheet().GetFull().GetFeatureChoiceKeys(); !slices.Contains(picks, golpe.GetKey()) || slices.Contains(picks, heron) {
		t.Errorf("picks after the swap = %v, want the table option for the Heron", picks)
	}
}

// What the protocol says of the new field: the message is the one the pack and the API carry.
func TestTheSwapLevelsAreAFieldOfTheFeatureMessage(t *testing.T) {
	t.Parallel()
	f := posturesFeature(&rulesv1.FeatureSwapLevels{Levels: []int32{3, 5}})
	raw, err := protojson.Marshal(f)
	if err != nil || !strings.Contains(string(raw), `"swapLevels"`) {
		t.Fatalf("json = %s, %v, want the field", raw, err)
	}
	got := tableFeatureOf(f)
	if !slices.Equal(got.SwapLevels.Levels, []int{3, 5}) || got.SwapLevels.EveryLevel {
		t.Errorf("tableFeatureOf = %+v, want the levels 3 and 5", got.SwapLevels)
	}
	if got := tableFeatureOf(posturesFeature(nil)); !got.SwapLevels.IsZero() {
		t.Errorf("a feature with no swap levels reads %+v", got.SwapLevels)
	}
}
