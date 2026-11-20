package characters

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1/charactersv1connect"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// A table class whose feature at level 2 lets the player pick one skill or one language ("one of these").

func skillOrLanguageEffect() *rulesv1.TableEffect {
	return &rulesv1.TableEffect{Type: "choice", Choice: "alternatives", Count: 1, Alternatives: []*rulesv1.ChoiceAlternative{
		{Kind: "skill", From: []string{"skill:stealth", "skill:persuasion"}},
		{Kind: "language", From: []string{"language:dwarvish", "language:elvish", "language:giant"}},
	}}
}

// alternativesFlow is the class flow with a guardian that has the choice at level 2, and Ícaro at level 1 on it.
type alternativesFlow struct {
	*classFlow
	class *rulesv1.TableEntry
	owner string
	key   string
}

func newAlternativesFlow(t *testing.T) *alternativesFlow {
	t.Helper()
	f := &alternativesFlow{classFlow: newClassFlow(t)}
	cls := guardianClass(t, f.defaults, f.menu)
	cls.NamePt = "Guardião do Farol"
	cls.Levels[1].Features = []*rulesv1.TableFeature{{NamePt: "Faro do vale", DescPt: []string{"Texto de teste."}, Effects: []*rulesv1.TableEffect{skillOrLanguageEffect()}}}
	f.class = f.master.addEntry(t, f.campaign, cls)
	f.owner = f.master.entries(t, f.campaign)[f.class.GetKey()].GetTableClass().GetLevels()[1].GetFeatures()[0].GetKey()
	f.key = rules.AlternativesKey(f.owner, &rules.Effect{Count: 1, Alternatives: []rules.ChoiceAlternative{
		{Kind: "skill", From: []string{"skill:stealth", "skill:persuasion"}},
		{Kind: "language", From: []string{"language:dwarvish", "language:elvish", "language:giant"}},
	}})
	return f
}

// sheet is Ícaro of the class at the level, with the picks.
func (f *alternativesFlow) sheet(level int32, picks ...string) *charactersv1.CharacterSheet {
	s := f.icaroSheet()
	full := s.GetFull()
	full.Classes = []*charactersv1.ClassLevel{{ClassKey: f.class.GetKey(), Level: level}}
	for _, p := range picks {
		full.FeatureChoiceKeys = append(full.FeatureChoiceKeys, rules.ScopedChoice(f.key, p))
	}
	return s
}

func (f *alternativesFlow) choices(t *testing.T, sheet *charactersv1.CharacterSheet) *charactersv1.Choice {
	t.Helper()
	res, err := f.davi.api.PreviewChoices(t.Context(), connect.NewRequest(&charactersv1.PreviewChoicesRequest{
		CampaignId: f.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Sheet: sheet,
	}))
	if err != nil {
		t.Fatalf("PreviewChoices() error = %v", err)
	}
	return choiceByKey(res.Msg, f.key)
}

func optionKeysOf(ch *charactersv1.Choice) []string {
	var out []string
	for _, o := range ch.GetOptions() {
		out = append(out, o.GetKey())
	}
	slices.Sort(out)
	return out
}

func TestAChoiceOfAlternativesIsAskedAsOneChoiceOfBothKinds(t *testing.T) {
	t.Parallel()
	f := newAlternativesFlow(t)
	ch := f.choices(t, f.sheet(2))
	if ch == nil {
		t.Fatal("the choice is not asked")
	}
	if got, want := optionKeysOf(ch), []string{"language:dwarvish", "language:elvish", "language:giant", "skill:persuasion", "skill:stealth"}; !slices.Equal(got, want) || ch.GetPicks() != 1 {
		t.Errorf("options %v for %d picks, want %v for 1", got, ch.GetPicks(), want)
	}
	for _, o := range ch.GetOptions() {
		if o.GetNamePt() == "" || o.GetSummaryPt() == "" {
			t.Errorf("option %s has no name or kind: %v", o.GetKey(), o)
		}
	}
}

func TestACharacterIsCreatedWithAPickThroughAnAlternative(t *testing.T) {
	t.Parallel()
	for _, pick := range []string{"skill:stealth", "language:giant"} {
		f := newAlternativesFlow(t)
		c, err := f.davi.api.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
			CampaignId: f.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Ícaro " + pick, Sheet: f.sheet(2, pick),
		}))
		if err != nil {
			t.Fatalf("CreateCharacter(%s) error = %v", pick, err)
		}
		d := c.Msg.GetCharacter().GetDerived()
		if !slices.Contains(c.Msg.GetCharacter().GetSheet().GetFull().GetFeatureChoiceKeys(), rules.ScopedChoice(f.key, pick)) {
			t.Errorf("%s: the pick is not stored", pick)
		}
		got := slices.ContainsFunc(d.GetSkills(), func(s *rulesv1.DerivedSkill) bool {
			return s.GetKey() == "skill:stealth" && s.GetProficiency() >= rulesv1.ProficiencyLevel_PROFICIENCY_LEVEL_PROFICIENT
		}) ||
			slices.ContainsFunc(d.GetLanguages(), func(l string) bool { return l == "Gigante" })
		if !got {
			t.Errorf("%s: the sheet has neither the skill nor the language", pick)
		}
	}
}

func TestAnUpdateWithAPickNoAlternativeOffersIsRefusedAsAKeyThatDoesNotExist(t *testing.T) {
	t.Parallel()
	f := newAlternativesFlow(t)
	pc := f.davi.create(t, f.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Ícaro", f.sheet(2, "skill:stealth"))
	var messages []string
	for _, bad := range []string{"skill:arcana", "language:abyssal", "skill:nao-existe"} {
		_, err := f.davi.update(t, pc, "Ícaro", f.sheet(2, bad))
		ce, ok := errors.AsType[*connect.Error](err)
		if !ok || ce.Code() != connect.CodeInvalidArgument {
			t.Fatalf("a pick of %s: error = %v, want invalid_argument", bad, err)
		}
		messages = append(messages, ce.Message())
	}
	if messages[0] != messages[2] || messages[1] != messages[2] {
		t.Errorf("the answers differ: %q", messages)
	}
}

func TestALevelUpOffersAndTakesAChoiceOfAlternatives(t *testing.T) {
	t.Parallel()
	f := newAlternativesFlow(t)
	c := f.davi.create(t, f.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Ícaro", f.sheet(1))
	res, err := f.davi.api.GetLevelUpOptions(t.Context(), connect.NewRequest(&charactersv1.GetLevelUpOptionsRequest{CampaignId: f.campaign, CharacterId: c.GetId(), ClassKey: f.class.GetKey()}))
	if err != nil {
		t.Fatalf("GetLevelUpOptions() error = %v", err)
	}
	var offered *charactersv1.Choice
	for _, g := range res.Msg.GetOptions().GetNewChoices() {
		for _, ch := range g.GetChoices() {
			if ch.GetKey() == f.key {
				offered = ch
			}
		}
	}
	if offered == nil || len(offered.GetOptions()) != 5 {
		t.Fatalf("the level-up offers %v, want the choice with its five options", offered)
	}
	up := func(picks ...string) (*charactersv1.LevelUpCharacterResponse, error) {
		ch := &charactersv1.LevelUpChoices{ClassKey: f.class.GetKey(), HitPoints: &charactersv1.LevelUpHitPoints{Method: charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_AVERAGE}}
		for _, p := range picks {
			ch.FeatureChoiceKeys = append(ch.FeatureChoiceKeys, rules.ScopedChoice(f.key, p))
		}
		// The half caster's prepared spells of level 2 are another choice: take the guardian flow's.
		r, err := f.davi.api.LevelUpCharacter(t.Context(), connect.NewRequest(&charactersv1.LevelUpCharacterRequest{CampaignId: f.campaign, CharacterId: c.GetId(), Revision: c.GetRevision(), Choices: ch}))
		if err != nil {
			return nil, err
		}
		return r.Msg, nil
	}
	if _, err := up(); err == nil {
		t.Error("the level-up goes up with the choice left open")
	}
	if _, err := up("skill:arcana"); err == nil {
		t.Error("the level-up takes a pick no alternative offers")
	}
	got, err := up("language:dwarvish")
	if err != nil {
		t.Fatalf("LevelUpCharacter() error = %v", err)
	}
	if !slices.Contains(got.GetCharacter().GetSheet().GetFull().GetFeatureChoiceKeys(), rules.ScopedChoice(f.key, "language:dwarvish")) {
		t.Errorf("the level-up lost the pick: %v", got.GetCharacter().GetSheet().GetFull().GetFeatureChoiceKeys())
	}
}

// A choice of alternatives survives the export of a pack and its import into another campaign, as the master wrote it.
func TestAChoiceOfAlternativesSurvivesAPackRoundTrip(t *testing.T) {
	t.Parallel()
	pt := newPackTable(t)
	body := testBackground("Cais do farol")
	body.Feature = &rulesv1.TableFeature{NamePt: "Rede do cais", DescPt: []string{"Texto de teste."}, Effects: []*rulesv1.TableEffect{{
		Type: "choice", Choice: "alternatives", Count: 2, Alternatives: []*rulesv1.ChoiceAlternative{
			{Kind: "skill", From: []string{"skill:stealth", "skill:persuasion"}},
			{Kind: "tool", Filter: &rulesv1.TableChoiceFilter{ProficiencyKinds: []string{"weapon", "other"}}},
		},
	}}}
	made := pt.master.addEntry(t, pt.from, body)
	file, err := storeJSON.Marshal(pt.export(t, pt.from))
	if err != nil {
		t.Fatal(err)
	}
	loaded := &rulesv1.TableContentPack{}
	if err := loadJSON.Unmarshal(file, loaded); err != nil {
		t.Fatal(err)
	}
	pt.apply(t, pt.to, loaded, "alternatives")
	in := pt.entries(t, pt.to)[made.GetKey()]
	if in == nil {
		t.Fatalf("the background %s is not in the second campaign", made.GetKey())
	}
	if !proto.Equal(bodyMessage(in), bodyMessage(made)) {
		t.Errorf("the background differs after the round trip: %v, want %v", bodyMessage(in), bodyMessage(made))
	}
	got := in.GetTableBackground().GetFeature().GetEffects()[0].GetAlternatives()
	if len(got) != 2 || got[1].GetFilter().GetProficiencyKinds()[1] != "other" {
		t.Errorf("alternatives = %v", got)
	}
}

// A choice of alternatives that mixes a list with its own is refused in an import as it is in a write: the same field and
// the same reason, in the preview and in the apply, and nothing is written.
func TestAnImportRefusesAChoiceOfAlternativesAsAWriteDoes(t *testing.T) {
	t.Parallel()
	pt := newPackTable(t)
	body := testBackground("Cais do farol")
	body.Feature = &rulesv1.TableFeature{NamePt: "Rede do cais", DescPt: []string{"Texto de teste."}, Effects: []*rulesv1.TableEffect{{
		Type: "choice", Choice: "alternatives", Count: 1, From: []string{"skill:stealth"}, Alternatives: []*rulesv1.ChoiceAlternative{
			{Kind: "skill", From: []string{"skill:stealth"}}, {Kind: "language", From: []string{"language:giant"}},
		},
	}}}
	const field = "table_background.feature.effects[0].alternatives"
	hasViolation := func(vs []*rulesv1.TableContentViolation) bool {
		return slices.ContainsFunc(vs, func(v *rulesv1.TableContentViolation) bool {
			return v.GetField() == field && v.GetReason() == "bad_value"
		})
	}
	_, err := pt.master.table.CreateTableEntry(t.Context(), connect.NewRequest(createReq(pt.to, body)))
	if !hasViolation(violationsOfErr(t, err)) {
		t.Fatalf("the write: %v %v, want a violation of reason bad_value at %s", err, violationsOfErr(t, err), field)
	}
	pack := &rulesv1.TableContentPack{Format: "meurpg.table-content", Version: 1, Entries: []*rulesv1.TableEntry{{
		Key: "background:cais-do-farol@mesa", Kind: rulesv1.TableContentKind_TABLE_CONTENT_KIND_BACKGROUND, NamePt: "Cais do farol",
		Body: &rulesv1.TableEntry_TableBackground{TableBackground: body},
	}}}
	if e := pt.preview(t, pt.to, pack).GetEntries()[0]; e.GetStatus() != rulesv1.TableImportStatus_TABLE_IMPORT_STATUS_REFUSED || !hasViolation(e.GetViolations()) {
		t.Errorf("the preview: status %v, violations %v; want it refused with the reason bad_value at %s", e.GetStatus(), e.GetViolations(), field)
	}
	_, err = pt.importPack(pt.master, pt.to, pack, rulesv1.TableImportMode_TABLE_IMPORT_MODE_APPLY, "alternatives-list")
	if !hasViolation(violationsOfErr(t, err)) {
		t.Errorf("the apply: %v, want a violation of reason bad_value at %s", err, field)
	}
	if got := pt.entries(t, pt.to); len(got) != 0 {
		t.Errorf("a refused import left %d entries", len(got))
	}
}

// The reader of a table feature gets the alternatives as the master wrote them (nothing the table hides is among them).
func TestTableEffectsOfReadsTheAlternativesOfAChoice(t *testing.T) {
	t.Parallel()
	got := tableEffectsOf([]*rulesv1.TableEffect{skillOrLanguageEffect()})
	if len(got) != 1 || len(got[0].Alternatives) != 2 || got[0].Alternatives[1].Kind != "language" || !slices.Equal(got[0].Alternatives[0].From, []string{"skill:stealth", "skill:persuasion"}) {
		t.Errorf("effect = %+v, want the two alternatives", got)
	}
}

// A new pick that repeats what another source gives the sheet is refused at the pick's own field, for the player and the
// master alike; a pick of another key of the choice is taken.
func TestANewPickOfWhatAnotherSourceGivesIsRefused(t *testing.T) {
	t.Parallel()
	f := newAlternativesFlow(t)
	repeating := func(pick string) *charactersv1.CharacterSheet {
		s := f.sheet(2, pick)
		s.GetFull().GetCustomBackground().SkillKeys = []string{"skill:stealth", "skill:survival"} // the background gives stealth
		return s
	}
	kinds := map[string]charactersv1.CharacterKind{"the player": charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "the master": charactersv1.CharacterKind_CHARACTER_KIND_ENEMY}
	for who, api := range map[string]charactersv1connect.CharacterServiceClient{"the player": f.davi.api, "the master": f.master.api} {
		kind := kinds[who]
		_, err := api.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{CampaignId: f.campaign, Kind: kind, Name: "Ícaro", Sheet: repeating("skill:stealth")}))
		ce, ok := errors.AsType[*connect.Error](err)
		if !ok || ce.Code() != connect.CodeInvalidArgument || !strings.Contains(ce.Message(), "feature_choice_keys") {
			t.Errorf("%s: the pick of the skill the background gives: error = %v, want invalid_argument at feature_choice_keys", who, err)
		}
	}
	if _, err := f.davi.api.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
		CampaignId: f.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Ícaro", Sheet: repeating("language:giant"),
	})); err != nil {
		t.Errorf("the pick of a language: error = %v", err)
	}
}

// repeating is Ícaro whose custom background gives stealth and elvish, with the pick; the sheet has the skill and the language
// from another source whatever the choice does.
func (f *alternativesFlow) repeating(level int32, pick string) *charactersv1.CharacterSheet {
	s := f.sheet(level, pick)
	s.GetFull().GetCustomBackground().SkillKeys = []string{"skill:stealth", "skill:survival"}
	return s
}

func (f *alternativesFlow) previewChoices(t *testing.T, id string, sheet *charactersv1.CharacterSheet) error {
	t.Helper()
	_, err := f.davi.api.PreviewChoices(t.Context(), connect.NewRequest(&charactersv1.PreviewChoicesRequest{
		CampaignId: f.campaign, CharacterId: id, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Sheet: sheet,
	}))
	return err
}

func repeatedField(err error) string {
	if ce, ok := errors.AsType[*connect.Error](err); ok && ce.Code() == connect.CodeInvalidArgument && strings.Contains(ce.Message(), "feature_choice_keys") {
		return ce.Message()
	}
	return ""
}

// The choices screen and the save read the same baseline: a repeated pick that the stored sheet does not hold (none, on a
// create) is refused by the screen's preview as by the save, with the same answer; one the stored sheet holds is fine in both.
// (The unmet pick of a screen that is not refused, the locked sheet's, is the rules row TestTheChoicesAreReadAgainstTheStoredSheetAsTheSaveIs.)
func TestTheChoicesScreenAndTheSaveAgreeOnARepeatedAlternativePick(t *testing.T) {
	t.Parallel()
	f := newAlternativesFlow(t)
	pick := "skill:stealth"
	_, createErr := f.davi.api.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
		CampaignId: f.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Ícaro", Sheet: f.repeating(2, pick),
	}))
	screen := f.previewChoices(t, "", f.repeating(2, pick))
	if repeatedField(createErr) == "" || repeatedField(screen) != repeatedField(createErr) {
		t.Errorf("a create: the save answers %v, the screen %v; want the same refusal", createErr, screen)
	}

	other := f.davi.create(t, f.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Ícaro", f.sheet(2, "language:giant"))
	_, updateErr := f.davi.update(t, other, "Ícaro", f.repeating(2, pick))
	screen = f.previewChoices(t, other.GetId(), f.repeating(2, pick))
	if repeatedField(updateErr) == "" || repeatedField(screen) != repeatedField(updateErr) {
		t.Errorf("an update that adds it: the save answers %v, the screen %v; want the same refusal", updateErr, screen)
	}
}

// A pick the stored sheet holds stays when the sheet then has the same thing from another source.
func TestAHeldAlternativePickStaysOnTheScreenAndInTheSave(t *testing.T) {
	t.Parallel()
	f := newAlternativesFlow(t)
	held := f.davi.create(t, f.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Ícaro", f.sheet(2, "skill:stealth"))
	if err := f.previewChoices(t, held.GetId(), f.repeating(2, "skill:stealth")); err != nil {
		t.Errorf("the screen: %v", err)
	}
	if _, err := f.davi.update(t, held, "Ícaro", f.repeating(2, "skill:stealth")); err != nil {
		t.Errorf("the update of the held pick: %v", err)
	}
}

// The preview of a character and the level-up plan refuse a new repeated pick as the update does, and a sheet a package brings
// keeps its picks: the import accepts the repeated pick, as it does a pick no list takes.
func TestARepeatedAlternativePickIsRefusedOnPreviewAndLevelUpAndKeptOnImport(t *testing.T) {
	t.Parallel()
	f := newAlternativesFlow(t)
	// A character at level 1 whose background gives stealth and elvish: the choice of level 2 is not asked yet.
	level1 := f.sheet(1)
	level1.GetFull().GetCustomBackground().SkillKeys = []string{"skill:stealth", "skill:survival"}
	c := f.davi.create(t, f.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Ícaro", level1)

	_, err := f.davi.api.PreviewCharacter(t.Context(), connect.NewRequest(&charactersv1.PreviewCharacterRequest{
		CampaignId: f.campaign, CharacterId: c.GetId(), Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Sheet: f.repeating(2, "skill:stealth"),
	}))
	if repeatedField(err) == "" {
		t.Errorf("the preview of the character takes the repeated pick: %v", err)
	}

	up := func(pick string) error {
		ch := &charactersv1.LevelUpChoices{
			ClassKey: f.class.GetKey(), HitPoints: &charactersv1.LevelUpHitPoints{Method: charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_AVERAGE},
			FeatureChoiceKeys: []string{rules.ScopedChoice(f.key, pick)},
		}
		_, err := f.davi.api.PreviewLevelUp(t.Context(), connect.NewRequest(&charactersv1.PreviewLevelUpRequest{CampaignId: f.campaign, CharacterId: c.GetId(), Choices: ch}))
		return err
	}
	if err := up("skill:stealth"); repeatedField(err) == "" {
		t.Errorf("the level-up plan takes the repeated pick: %v", err)
	}
	if err := up("language:elvish"); repeatedField(err) == "" {
		t.Errorf("the level-up plan takes the language the background gives: %v", err)
	}
	if err := up("language:giant"); err != nil {
		t.Errorf("the level-up plan refuses a pick that is new: %v", err)
	}

	content, err := f.h.svc.contentFor(t.Context(), nil, f.campaign)
	if err != nil {
		t.Fatal(err)
	}
	master := authz.Membership{CampaignID: f.campaign, Role: authz.RoleMaster}
	if _, err := checkNewSheet(content, master, kindPlayer, proto.CloneOf(f.repeating(2, "skill:stealth"))); err == nil {
		t.Error("a sheet made now with the repeated pick is accepted")
	}
	if _, err := checkImportedSheet(content, master, kindPlayer, proto.CloneOf(f.repeating(2, "skill:stealth"))); err != nil {
		t.Errorf("the imported sheet is refused: %v", err)
	}
}

// The picks page of a locked sheet reads the draft's picks against the stored sheet: a new pick that repeats another source is
// refused there as in the save, and a pick the stored sheet holds is fine.
func TestTheChoicesOfALockedSheetRefuseANewRepeatedPickAsTheSaveDoes(t *testing.T) {
	t.Parallel()
	f := newAlternativesFlow(t)
	stored := f.repeating(2, "language:giant") // the background gives stealth; the held pick is another key
	c := f.davi.create(t, f.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Ícaro", stored)
	f.h.lockSheets(f.campaign)
	read := func(pick string) error {
		_, err := f.davi.api.PreviewChoices(t.Context(), connect.NewRequest(&charactersv1.PreviewChoicesRequest{
			CampaignId: f.campaign, CharacterId: c.GetId(), Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Sheet: f.sheet(2, pick),
		}))
		return err
	}
	if repeatedField(read("skill:stealth")) == "" {
		t.Error("the page of a locked sheet takes a new repeated pick")
	}
	if err := read("language:giant"); err != nil {
		t.Errorf("the held pick: %v", err)
	}
}
