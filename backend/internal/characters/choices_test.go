package characters

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The choices a class or a race asks (PM-05): the refusals of a save, the page that
// completes what a locked sheet left open, the level-up's catch-up and who may read the
// count of what is open (RN-10).

const unspecifiedMethod = charactersv1.AbilityMethod_ABILITY_METHOD_UNSPECIFIED

// choiceSheet is a player's sheet of a race and a class with the picks given, the
// background of an acolyte and a score array that suits every class.
func choiceSheet(race, subrace, class, subclass string, level int32, picks ...string) *charactersv1.CharacterSheet {
	cl := &charactersv1.ClassLevel{ClassKey: class, Level: level}
	if subclass != "" {
		cl.Subclass = &charactersv1.ClassLevel_SubclassKey{SubclassKey: subclass}
	}
	return &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
		BaseScores:        &rulesv1.AbilityScores{Strength: 15, Dexterity: 14, Constitution: 13, Intelligence: 10, Wisdom: 12, Charisma: 8},
		RaceKey:           race,
		SubraceKey:        subrace,
		Classes:           []*charactersv1.ClassLevel{cl},
		Background:        &charactersv1.FullSheet_BackgroundKey{BackgroundKey: "background:acolyte"},
		Languages:         testLanguages,
		FeatureChoiceKeys: picks,
	}}}
}

// choiceRefusal returns the ChoiceRefusal detail of a failed_precondition.
func choiceRefusal(t *testing.T, call string, err error) *charactersv1.ChoiceRefusal {
	t.Helper()
	wantCode(t, call, err, connect.CodeFailedPrecondition)
	ce, _ := errors.AsType[*connect.Error](err)
	for _, d := range ce.Details() {
		v, err := d.Value()
		if err != nil {
			t.Fatalf("decode error detail: %v", err)
		}
		if detail, ok := v.(*charactersv1.ChoiceRefusal); ok {
			return detail
		}
	}
	t.Fatalf("%s error %v has no ChoiceRefusal detail", call, err)
	return nil
}

// eventSpy is the play module's side of a completed choice: it records the events the
// history would get.
type eventSpy struct {
	CreatureHost
	events []string
}

func (e *eventSpy) AppendEvent(_ context.Context, _ pgx.Tx, _, kind, _ string, payload []byte, _ time.Time) (bool, error) {
	e.events = append(e.events, kind+" "+string(payload))
	return true, nil
}

// choiceTable is a campaign with a master, two players and the spy.
type choiceTable struct {
	h                 *harness
	master, ana, caio *user
	campaign          string
	spy               *eventSpy
}

func newChoiceTable(t *testing.T) *choiceTable {
	t.Helper()
	tb := &choiceTable{h: newHarness(t), spy: &eventSpy{}}
	tb.h.svc.SetCreatureHost(tb.spy)
	tb.master, tb.ana, tb.caio = tb.h.newUser("Samuel"), tb.h.newUser("Ana"), tb.h.newUser("Caio")
	tb.campaign = tb.h.newCampaign(tb.master, "Mirathel", tb.ana, tb.caio)
	return tb
}

func (tb *choiceTable) create(u *user, sheet *charactersv1.CharacterSheet) (*charactersv1.Character, error) {
	return u.createWith(tb.campaign, unspecifiedMethod, sheet)
}

// openChoice makes a character that has a choice open the way a sheet written before the
// choices were asked has it: the player makes it complete, and the master (who is not
// refused) takes a pick out.
func (tb *choiceTable) openChoice(t *testing.T, u *user, complete, open *charactersv1.CharacterSheet) *charactersv1.Character {
	t.Helper()
	c, err := tb.create(u, complete)
	if err != nil {
		t.Fatalf("create the complete sheet: %v", err)
	}
	c, err = tb.master.update(t, c, c.GetName(), open)
	if err != nil {
		t.Fatalf("the master takes a pick out: %v", err)
	}
	return c
}

func (tb *choiceTable) preview(u *user, characterID string, sheet *charactersv1.CharacterSheet) (*charactersv1.PreviewChoicesResponse, error) {
	res, err := u.api.PreviewChoices(context.Background(), connect.NewRequest(&charactersv1.PreviewChoicesRequest{
		CampaignId: tb.campaign, CharacterId: characterID, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Sheet: sheet,
	}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func (tb *choiceTable) complete(u *user, c *charactersv1.Character, picks ...*charactersv1.ChoicePick) (*charactersv1.Character, error) {
	res, err := u.api.CompleteCharacterChoices(context.Background(), connect.NewRequest(&charactersv1.CompleteCharacterChoicesRequest{
		CampaignId: tb.campaign, CharacterId: c.GetId(), ExpectedRevision: c.GetRevision(), Picks: picks,
	}))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetCharacter(), nil
}

func pick(choiceKey string, options ...string) *charactersv1.ChoicePick {
	return &charactersv1.ChoicePick{ChoiceKey: choiceKey, OptionKeys: options}
}

func choiceByKey(res *charactersv1.PreviewChoicesResponse, key string) *charactersv1.Choice {
	for _, g := range res.GetGroups() {
		for _, ch := range g.GetChoices() {
			if ch.GetKey() == key {
				return ch
			}
		}
	}
	return nil
}

func issueKeys(r *charactersv1.ChoiceRefusal) []string {
	var keys []string
	for _, is := range r.GetIssues() {
		keys = append(keys, is.GetChoiceKey()+"|"+is.GetOptionKey())
	}
	return keys
}

// TestCreateCharacterRefusesAPlayersSheetWithAChoiceOpen: the server is the second line of
// guard behind the editor's "Criar personagem": a player's character without its Fighting
// Style is CHOICES_MISSING, with the choice's label; an NPC and the master's edit are not
// refused.
func TestCreateCharacterRefusesAPlayersSheetWithAChoiceOpen(t *testing.T) {
	t.Parallel()
	tb := newChoiceTable(t)
	_, err := tb.create(tb.ana, choiceSheet("race:human", "", "class:fighter", "", 1))
	r := choiceRefusal(t, "a fighter without a style", err)
	if r.GetReason() != charactersv1.ChoiceRefusalReason_CHOICE_REFUSAL_REASON_CHOICES_MISSING {
		t.Fatalf("reason = %v, want CHOICES_MISSING", r.GetReason())
	}
	if len(r.GetIssues()) != 1 || r.GetIssues()[0].GetChoiceKey() != "feature:fighter-fighting-style" || r.GetIssues()[0].GetLabelPt() != "Estilo de Luta (Guerreiro, nível 1)" ||
		r.GetIssues()[0].GetField() != "sheet.full.feature_choice_keys" || r.GetIssues()[0].GetRequired() != 1 || r.GetIssues()[0].GetPicked() != 0 {
		t.Errorf("issues = %v, want the Fighting Style of the fighter at level 1, with the field, 0 of 1", r.GetIssues())
	}

	c, err := tb.create(tb.ana, choiceSheet("race:human", "", "class:fighter", "", 1, "feature:fighter-fighting-style-defense"))
	if err != nil {
		t.Fatalf("a fighter with its style: %v", err)
	}
	// An NPC with the same open choice is not refused: the master may leave it open.
	if _, err := tb.master.api.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
		CampaignId: tb.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_ENEMY, Name: "Bandido", Sheet: enemySheet(),
	})); err != nil {
		t.Errorf("an NPC fighter without a style: %v", err)
	}
	// Nor the master's edit of a player's sheet that takes the pick out.
	if _, err := tb.master.update(t, c, c.GetName(), choiceSheet("race:human", "", "class:fighter", "", 1)); err != nil {
		t.Errorf("the master's edit: %v", err)
	}
}

// TestASaveRefusesAPickTheSheetCannotTake: an invocation whose prerequisite is unmet is
// PREREQUISITE_UNMET with the invocation named, and a pick no choice offers is
// CHOICE_NOT_OFFERED.
func TestASaveRefusesAPickTheSheetCannotTake(t *testing.T) {
	t.Parallel()
	tb := newChoiceTable(t)
	warlock := func(picks ...string) *charactersv1.CharacterSheet {
		s := choiceSheet("race:human", "", "class:warlock", "", 3, append([]string{"feature:pact-of-the-chain"}, picks...)...)
		s.GetFull().CantripKeys = []string{"spell:mage-hand", "spell:chill-touch"}
		return s
	}
	_, err := tb.create(tb.ana, warlock("feature:eldritch-invocation-agonizing-blast", "feature:eldritch-invocation-armor-of-shadows"))
	r := choiceRefusal(t, "Agonizing Blast without Eldritch Blast", err)
	if r.GetReason() != charactersv1.ChoiceRefusalReason_CHOICE_REFUSAL_REASON_PREREQUISITE_UNMET ||
		!slices.Equal(issueKeys(r), []string{"feature:eldritch-invocations|feature:eldritch-invocation-agonizing-blast"}) {
		t.Errorf("refusal = %v, want the unmet prerequisite of Agonizing Blast", r)
	}
	_, err = tb.create(tb.ana, warlock("feature:eldritch-invocation-armor-of-shadows", "feature:eldritch-invocation-devils-sight", "feature:fighter-fighting-style-defense"))
	r = choiceRefusal(t, "a style a warlock cannot have", err)
	if r.GetReason() != charactersv1.ChoiceRefusalReason_CHOICE_REFUSAL_REASON_CHOICE_NOT_OFFERED || len(r.GetIssues()) != 1 {
		t.Errorf("refusal = %v, want CHOICE_NOT_OFFERED", r)
	}
	// Taking the cantrip makes it a good sheet.
	good := warlock("feature:eldritch-invocation-agonizing-blast", "feature:eldritch-invocation-armor-of-shadows")
	good.GetFull().CantripKeys = append(good.GetFull().CantripKeys, "spell:eldritch-blast")
	if _, err := tb.create(tb.ana, good); err != nil {
		t.Errorf("a warlock with Eldritch Blast: %v", err)
	}
}

// TestAPlayerSaveNeverRefusesAChoiceTheSheetAlreadyLacked: an old sheet that asks for a
// choice (made before the choices existed, or left open by the master) is saved by its
// player as it is, and the pick that comes is saved too; what the save may not do is add a
// problem the stored sheet did not have (a pick the sheet does not offer).
func TestAPlayerSaveNeverRefusesAChoiceTheSheetAlreadyLacked(t *testing.T) {
	t.Parallel()
	tb := newChoiceTable(t)
	c := tb.openChoice(t, tb.ana,
		choiceSheet("race:human", "", "class:fighter", "", 1, "feature:fighter-fighting-style-defense"),
		choiceSheet("race:human", "", "class:fighter", "", 1))
	edited := proto.CloneOf(c.GetSheet())
	edited.GetFull().Languages = []string{"Anão"}
	saved, err := tb.ana.update(t, c, c.GetName(), edited)
	if err != nil {
		t.Fatalf("the player's save with the style still open: %v", err)
	}
	// A pick the sheet does not offer is a new problem: refused.
	bad := proto.CloneOf(saved.GetSheet())
	bad.GetFull().FeatureChoiceKeys = []string{"feature:eldritch-invocation-agonizing-blast"}
	_, err = tb.ana.update(t, saved, saved.GetName(), bad)
	if r := choiceRefusal(t, "a pick the sheet does not offer", err); r.GetReason() != charactersv1.ChoiceRefusalReason_CHOICE_REFUSAL_REASON_CHOICE_NOT_OFFERED {
		t.Errorf("refusal = %v, want CHOICE_NOT_OFFERED", r)
	}
	good := proto.CloneOf(saved.GetSheet())
	good.GetFull().FeatureChoiceKeys = []string{"feature:fighter-fighting-style-archery"}
	if _, err := tb.ana.update(t, saved, saved.GetName(), good); err != nil {
		t.Errorf("the player's save with the style: %v", err)
	}
}

// TestPreviewChoicesAnswersWhatTheDraftAsks: the groups, the options and the counts of a
// draft; the numbers a pick gives come from the server; the spells outside the class list
// come with it.
func TestPreviewChoicesAnswersWhatTheDraftAsks(t *testing.T) {
	t.Parallel()
	tb := newChoiceTable(t)
	res, err := tb.preview(tb.ana, "", choiceSheet("race:dragonborn", "", "class:fighter", "", 1, "trait:draconic-ancestry-red"))
	if err != nil {
		t.Fatal(err)
	}
	// The ancestry is picked (1 of 2 with the fighting style); the Acolyte of the sheet adds two languages to pick,
	// which the two the sheet names (testLanguages) fill: 3 of 4, in the race, class and background groups.
	if res.GetDone() != 3 || res.GetTotal() != 4 || len(res.GetGroups()) != 3 {
		t.Fatalf("done %d of %d in %d groups, want 3 of 4 in 3", res.GetDone(), res.GetTotal(), len(res.GetGroups()))
	}
	ancestry := choiceByKey(res, "trait:draconic-ancestry")
	if ancestry == nil || len(ancestry.GetOptions()) != 10 || !slices.Equal(ancestry.GetPicked(), []string{"trait:draconic-ancestry-red"}) ||
		!strings.Contains(ancestry.GetResultPt(), "CD 12 com Constituição 13 e proficiência +2") && !strings.Contains(ancestry.GetResultPt(), "CD 11") {
		t.Fatalf("ancestry = %v", ancestry)
	}
	if style := choiceByKey(res, "feature:fighter-fighting-style"); style == nil || style.GetMissing() != 1 || style.GetLabelPt() != "Estilo de Luta (Guerreiro, nível 1)" {
		t.Errorf("style = %v, want it open", style)
	}

	// The Fiend's patron spells and the Bard's secrets are the spell pickers' sources.
	res, err = tb.preview(tb.ana, "", choiceSheet("race:human", "", "class:warlock", "subclass:fiend", 5))
	if err != nil {
		t.Fatal(err)
	}
	var patron []string
	for _, src := range res.GetSpellSources() {
		for _, p := range src.GetPatronSpells() {
			patron = append(patron, p.GetSpellKey())
		}
	}
	if !slices.Contains(patron, "spell:burning-hands") || !slices.Contains(patron, "spell:fireball") || slices.Contains(patron, "spell:fire-shield") {
		t.Errorf("patron spells at warlock 5 = %v, want the Fiend's up to the 3rd circle", patron)
	}
	res, err = tb.preview(tb.ana, "", choiceSheet("race:human", "", "class:bard", "subclass:lore", 10))
	if err != nil {
		t.Fatal(err)
	}
	if src := res.GetSpellSources(); len(src) != 1 || src[0].GetSecrets() != 4 || src[0].GetSecretsBeyondKnown() != 2 || src[0].GetSecretsMaxSpellLevel() != 5 {
		t.Errorf("bard sources = %v, want 4 secrets (Lore's two beyond the number known), up to the 5th circle", src)
	}
	// A sheet with nothing but the languages of its race and background to pick asks only for those, and a basic sheet
	// has no choices.
	res, err = tb.preview(tb.ana, "", choiceSheet("race:human", "", "class:rogue", "", 1))
	if err != nil || len(res.GetGroups()) != 2 {
		t.Fatalf("a human rogue 1: %v, %v; want the human's language and the Acolyte's two", res, err)
	}
	for _, g := range res.GetGroups() {
		for _, ch := range g.GetChoices() {
			if !strings.HasSuffix(ch.GetKey(), "#language") {
				t.Errorf("a human rogue 1 asks %q, want languages only", ch.GetKey())
			}
		}
	}
	_, err = tb.master.api.PreviewChoices(t.Context(), connect.NewRequest(&charactersv1.PreviewChoicesRequest{
		CampaignId: tb.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_MINION, Sheet: basicSheet(),
	}))
	wantCode(t, "PreviewChoices of a basic sheet", err, connect.CodeInvalidArgument)
	// A player may not preview an NPC kind, as with PreviewCharacter.
	_, err = tb.ana.api.PreviewChoices(t.Context(), connect.NewRequest(&charactersv1.PreviewChoicesRequest{
		CampaignId: tb.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_ENEMY, Sheet: enemySheet(),
	}))
	wantCode(t, "a player previewing an NPC", err, connect.CodePermissionDenied)
}

// TestTheLockedSheetsPreviewTakesOnlyThePicks: on a sheet locked for the player, the
// preview reads the stored sheet with the picks of the draft laid over it, and nothing else
// of the draft; the half-elf's new scores come with it.
func TestTheLockedSheetsPreviewTakesOnlyThePicks(t *testing.T) {
	t.Parallel()
	tb := newChoiceTable(t)
	key := rules.AbilityChoiceKey("race:half-elf")
	c := tb.openChoice(t, tb.ana,
		choiceSheet("race:half-elf", "", "class:fighter", "", 1, rules.ScopedChoice(key, "ability:dex"), rules.ScopedChoice(key, "ability:con"), "feature:fighter-fighting-style-defense"),
		choiceSheet("race:half-elf", "", "class:fighter", "", 1, "feature:fighter-fighting-style-defense"))
	tb.h.lockSheets(tb.campaign)
	c = tb.ana.get(t, tb.campaign, c.GetId())

	// A draft that also changes the class: the preview ignores that.
	draft := choiceSheet("race:human", "", "class:wizard", "", 5, rules.ScopedChoice(key, "ability:dex"), rules.ScopedChoice(key, "ability:con"), "feature:fighter-fighting-style-defense")
	res, err := tb.preview(tb.ana, c.GetId(), draft)
	if err != nil {
		t.Fatal(err)
	}
	abilities := choiceByKey(res, key)
	if abilities == nil || abilities.GetMissing() != 0 || !strings.Contains(abilities.GetResultPt(), "Destreza 14 → 15") || !strings.Contains(abilities.GetResultPt(), "Constituição 13 → 14") {
		t.Fatalf("abilities = %v, want the new scores", abilities)
	}
	if choiceByKey(res, "feature:fighter-fighting-style") == nil {
		t.Error("the preview of the locked fighter lost its class: the draft's class was read")
	}
	// The same call without a draft reads the stored sheet: the half-elf has both open.
	res, err = tb.preview(tb.ana, c.GetId(), nil)
	if err != nil || choiceByKey(res, key).GetMissing() != 2 {
		t.Errorf("stored sheet: %v, %v; want the two abilities open", res, err)
	}
	// Another player cannot read it.
	_, err = tb.preview(tb.caio, c.GetId(), nil)
	wantCode(t, "another player's preview", err, connect.CodeNotFound)
}

// TestACharacterCompletesTheChoicesItsSheetLeftOpen: on a locked sheet the owner completes
// what is open; the numbers follow (a Constitution point raises the hit points), the
// revision moves, and the master's history gets a line.
func TestACharacterCompletesTheChoicesItsSheetLeftOpen(t *testing.T) {
	t.Parallel()
	tb := newChoiceTable(t)
	key := rules.AbilityChoiceKey("race:half-elf")
	c := tb.openChoice(t, tb.ana,
		choiceSheet("race:half-elf", "", "class:fighter", "", 1, rules.ScopedChoice(key, "ability:dex"), rules.ScopedChoice(key, "ability:con"), "feature:fighter-fighting-style-defense"),
		choiceSheet("race:half-elf", "", "class:fighter", "", 1, "feature:fighter-fighting-style-defense"))
	tb.h.lockSheets(tb.campaign)
	c = tb.ana.get(t, tb.campaign, c.GetId())
	if got := c.GetDerived().GetHitPointsMax(); got != 11 {
		t.Fatalf("the half-elf fighter's hit points = %d, want 10 + the Constitution modifier of 13 (+1) = 11", got)
	}

	done, err := tb.complete(tb.ana, c, pick(key, "ability:dex", "ability:con"))
	if err != nil {
		t.Fatalf("CompleteCharacterChoices: %v", err)
	}
	if done.GetRevision() != c.GetRevision()+1 || done.GetDerived().GetHitPointsMax() != 12 {
		t.Errorf("revision %d, hit points %d; want the next revision and 12 hit points (Constitution 14)", done.GetRevision(), done.GetDerived().GetHitPointsMax())
	}
	if !slices.Contains(done.GetSheet().GetFull().GetFeatureChoiceKeys(), rules.ScopedChoice(key, "ability:con")) {
		t.Errorf("stored picks = %v, want the abilities written as scoped keys", done.GetSheet().GetFull().GetFeatureChoiceKeys())
	}
	if len(tb.spy.events) != 1 || !strings.HasPrefix(tb.spy.events[0], "character_choices_completed ") || !strings.Contains(tb.spy.events[0], c.GetId()) {
		t.Errorf("history = %v, want one line for the character", tb.spy.events)
	}
	// What is made never becomes a field again.
	_, err = tb.complete(tb.ana, done, pick(key, "ability:str", "ability:wis"))
	if r := choiceRefusal(t, "completing a choice already made", err); r.GetReason() != charactersv1.ChoiceRefusalReason_CHOICE_REFUSAL_REASON_CHOICE_ALREADY_MADE {
		t.Errorf("refusal = %v, want CHOICE_ALREADY_MADE", r)
	}
	if len(tb.spy.events) != 1 {
		t.Errorf("a refused call wrote a line: %v", tb.spy.events)
	}
}

// TestCompletingAChoiceIsRefusedWhenItBreaksARule: a stale revision is aborted, a choice
// the sheet does not have is not open, an option the choice does not list or cannot take
// is refused, the Pact Boon is never swapped, someone else's character is not found, and
// nothing is written by a refusal.
func TestCompletingAChoiceIsRefusedWhenItBreaksARule(t *testing.T) {
	t.Parallel()
	tb := newChoiceTable(t)
	// A warlock 5 with the Pact of the Chain and two of its three invocations.
	complete := choiceSheet("race:human", "", "class:warlock", "", 5, "feature:pact-of-the-chain",
		"feature:eldritch-invocation-armor-of-shadows", "feature:eldritch-invocation-devils-sight", "feature:eldritch-invocation-beast-speech")
	complete.GetFull().CantripKeys = []string{"spell:mage-hand", "spell:chill-touch"}
	open := proto.CloneOf(complete)
	open.GetFull().FeatureChoiceKeys = open.GetFull().FeatureChoiceKeys[:3]
	c := tb.openChoice(t, tb.ana, complete, open)
	tb.h.lockSheets(tb.campaign)
	c = tb.ana.get(t, tb.campaign, c.GetId())
	const invocations = "feature:eldritch-invocations"
	reason := func(err error) charactersv1.ChoiceRefusalReason {
		return choiceRefusal(t, "refusal", err).GetReason()
	}

	if _, err := tb.complete(tb.ana, &charactersv1.Character{Id: c.GetId(), Revision: c.GetRevision() + 7}, pick(invocations, "feature:eldritch-invocation-mask-of-many-faces")); connect.CodeOf(err) != connect.CodeAborted {
		t.Errorf("stale revision: %v, want aborted", err)
	}
	_, err := tb.complete(tb.ana, c, pick("feature:metamagic-1", "feature:metamagic-careful-spell"))
	if got := reason(err); got != charactersv1.ChoiceRefusalReason_CHOICE_REFUSAL_REASON_CHOICE_NOT_OPEN {
		t.Errorf("a choice the warlock does not have: %v, want CHOICE_NOT_OPEN", got)
	}
	_, err = tb.complete(tb.ana, c, pick("feature:pact-boon", "feature:pact-of-the-blade"))
	if got := reason(err); got != charactersv1.ChoiceRefusalReason_CHOICE_REFUSAL_REASON_CHOICE_ALREADY_MADE {
		t.Errorf("swapping the Pact Boon: %v, want CHOICE_ALREADY_MADE", got)
	}
	_, err = tb.complete(tb.ana, c, pick(invocations, "feature:eldritch-invocation-agonizing-blast"))
	if got := reason(err); got != charactersv1.ChoiceRefusalReason_CHOICE_REFUSAL_REASON_PREREQUISITE_UNMET {
		t.Errorf("Agonizing Blast without the cantrip: %v, want PREREQUISITE_UNMET", got)
	}
	_, err = tb.complete(tb.ana, c, pick(invocations, "feature:eldritch-invocation-lifedrinker"))
	if got := reason(err); got != charactersv1.ChoiceRefusalReason_CHOICE_REFUSAL_REASON_PREREQUISITE_UNMET {
		t.Errorf("Lifedrinker at level 5: %v, want PREREQUISITE_UNMET", got)
	}
	_, err = tb.complete(tb.ana, c, pick(invocations, "feature:fighter-fighting-style-defense"))
	if got := reason(err); got != charactersv1.ChoiceRefusalReason_CHOICE_REFUSAL_REASON_CHOICE_NOT_OFFERED {
		t.Errorf("an option the choice does not list: %v, want CHOICE_NOT_OFFERED", got)
	}
	_, err = tb.complete(tb.ana, c, pick(invocations, "feature:eldritch-invocation-mask-of-many-faces", "feature:eldritch-invocation-misty-visions"))
	if got := reason(err); got != charactersv1.ChoiceRefusalReason_CHOICE_REFUSAL_REASON_CHOICE_NOT_OFFERED {
		t.Errorf("two invocations for one open slot: %v, want CHOICE_NOT_OFFERED", got)
	}
	_, err = tb.complete(tb.caio, c, pick(invocations, "feature:eldritch-invocation-mask-of-many-faces"))
	wantCode(t, "another player completing", err, connect.CodeNotFound)
	_, err = tb.complete(tb.ana, c)
	wantCode(t, "no picks", err, connect.CodeInvalidArgument)
	if len(tb.spy.events) != 0 || tb.ana.get(t, tb.campaign, c.GetId()).GetRevision() != c.GetRevision() {
		t.Errorf("a refusal wrote something: events %v", tb.spy.events)
	}

	// The one that fits works, with the picks already made left alone.
	done, err := tb.complete(tb.ana, c, pick(invocations, "feature:eldritch-invocation-armor-of-shadows", "feature:eldritch-invocation-mask-of-many-faces"))
	if err != nil {
		t.Fatalf("the open invocation: %v", err)
	}
	if got := done.GetSheet().GetFull().GetFeatureChoiceKeys(); len(got) != 4 || got[3] != "feature:eldritch-invocation-mask-of-many-faces" {
		t.Errorf("stored picks = %v, want the old ones and the new at the end", got)
	}
	// The master completes too, and a dead character cannot.
	tb.master.markDead(t, done)
	_, err = tb.complete(tb.master, tb.master.get(t, tb.campaign, c.GetId()), pick(invocations, "feature:eldritch-invocation-misty-visions"))
	if b := blocked(t, "a dead character", err); b.GetReason() != charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_CHARACTER_DEAD {
		t.Errorf("blocked = %v, want CHARACTER_DEAD", b)
	}
}

// TestAFavoredEnemyOfTwoHumanoidRacesTakesTheirTexts: the humanoid option is picked with the
// two races written, no interpretation of the text.
func TestAFavoredEnemyOfTwoHumanoidRacesTakesTheirTexts(t *testing.T) {
	t.Parallel()
	tb := newChoiceTable(t)
	enemy := "feature:favored-enemy-1-type"
	complete := choiceSheet("race:human", "", "class:ranger", "", 1,
		rules.ScopedChoice(enemy, "creature-type:beast"), rules.ScopedChoice(enemy+"#language", rules.LanguageNone),
		rules.ScopedChoice("feature:natural-explorer-1-terrain-type", "terrain:forest"))
	open := choiceSheet("race:human", "", "class:ranger", "", 1, rules.ScopedChoice("feature:natural-explorer-1-terrain-type", "terrain:forest"))
	c := tb.openChoice(t, tb.ana, complete, open)
	tb.h.lockSheets(tb.campaign)
	c = tb.ana.get(t, tb.campaign, c.GetId())

	humanoid := &charactersv1.ChoicePick{ChoiceKey: enemy, OptionKeys: []string{"creature-type:humanoid"}, Texts: []string{"gnolls"}}
	_, err := tb.complete(tb.ana, c, humanoid, pick(enemy+"#language", "language:gnoll"))
	if r := choiceRefusal(t, "one race", err); r.GetReason() != charactersv1.ChoiceRefusalReason_CHOICE_REFUSAL_REASON_CHOICES_MISSING {
		t.Errorf("refusal = %v, want CHOICES_MISSING for the missing race", r)
	}
	humanoid.Texts = []string{"gnolls", strings.Repeat("o", 41)}
	_, err = tb.complete(tb.ana, c, humanoid)
	wantCode(t, "a text over 40 characters", err, connect.CodeInvalidArgument)
	humanoid.Texts = []string{"  gnolls ", "orcs"}
	done, err := tb.complete(tb.ana, c, humanoid, pick(enemy+"#language", rules.LanguageNone))
	if err != nil {
		t.Fatalf("two races: %v", err)
	}
	full := done.GetSheet().GetFull()
	if full.GetFeatureChoiceText()[rules.ChoiceTextKey(enemy, 1)] != "gnolls" || full.GetFeatureChoiceText()[rules.ChoiceTextKey(enemy, 2)] != "orcs" {
		t.Errorf("texts = %v, want the two races, trimmed", full.GetFeatureChoiceText())
	}
}

// TestTheOpenChoicesCountIsForTheMasterAndTheOwnerOnly: how many selections a character
// lacks is shown to the master and to its player; another player never gets the number
// (not even 0), a map token carries none, and the campaign's list is the master's (RN-10).
func TestTheOpenChoicesCountIsForTheMasterAndTheOwnerOnly(t *testing.T) {
	t.Parallel()
	tb := newChoiceTable(t)
	anas := tb.openChoice(t, tb.ana,
		choiceSheet("race:human", "", "class:fighter", "", 1, "feature:fighter-fighting-style-defense"),
		choiceSheet("race:human", "", "class:fighter", "", 1))
	caios, err := tb.create(tb.caio, choiceSheet("race:human", "", "class:fighter", "", 1, "feature:fighter-fighting-style-dueling"))
	if err != nil {
		t.Fatal(err)
	}
	countOf := func(u *user, id string) *int32 {
		for _, s := range u.list(t, tb.campaign) {
			if s.GetId() == id {
				if s.PendingChoiceCount == nil {
					return nil
				}
				n := s.GetPendingChoiceCount()
				return &n
			}
		}
		t.Fatalf("a user does not list %s", id)
		return nil
	}
	if n := countOf(tb.master, anas.GetId()); n == nil || *n != 1 {
		t.Errorf("the master sees %v for Ana's character, want 1", n)
	}
	if n := countOf(tb.ana, anas.GetId()); n == nil || *n != 1 {
		t.Errorf("Ana sees %v for her character, want 1", n)
	}
	if n := countOf(tb.master, caios.GetId()); n == nil || *n != 0 {
		t.Errorf("the master sees %v for Caio's complete character, want 0", n)
	}
	// Caio's list holds only his character; Ana's never reaches him, whatever the number.
	for _, s := range tb.caio.list(t, tb.campaign) {
		if s.GetId() == anas.GetId() {
			t.Errorf("Caio lists Ana's character")
		}
	}
	// The campaign's list is the master's.
	res, err := tb.master.api.GetCampaignOpenChoices(t.Context(), connect.NewRequest(&charactersv1.GetCampaignOpenChoicesRequest{CampaignId: tb.campaign}))
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Msg.GetCharacters(); len(got) != 1 || got[0].GetCharacterId() != anas.GetId() || got[0].GetPendingCount() != 1 ||
		!slices.Equal(got[0].GetLabelsPt(), []string{"Estilo de Luta (Guerreiro, nível 1)"}) {
		t.Errorf("the master's list = %v, want only Ana's fighter with the style open", got)
	}
	for name, u := range map[string]*user{"a player": tb.caio, "the owner": tb.ana, "a stranger": tb.h.newUser("De fora")} {
		_, err := u.api.GetCampaignOpenChoices(t.Context(), connect.NewRequest(&charactersv1.GetCampaignOpenChoicesRequest{CampaignId: tb.campaign}))
		wantCode(t, name+" asking for the open choices", err, connect.CodeNotFound)
		_, absent := u.api.GetCampaignOpenChoices(t.Context(), connect.NewRequest(&charactersv1.GetCampaignOpenChoicesRequest{CampaignId: "6f1c7a52-3b5e-4c55-9d0b-2a51f0c1e001"}))
		if err.Error() != absent.Error() {
			t.Errorf("%s: %q for this campaign, %q for one that does not exist; want the same answer", name, err, absent)
		}
	}
	// A map token is a summary with no count.
	tokens, err := tb.h.svc.MapCharacters(t.Context(), nil, tb.campaign, []string{anas.GetId()})
	if err != nil || len(tokens) != 1 || tokens[0].PendingChoiceCount != nil {
		t.Errorf("map token = %v, %v; want no count", tokens, err)
	}
	// A dead character, and an NPC, are not counted.
	tb.master.markDead(t, anas)
	res, err = tb.master.api.GetCampaignOpenChoices(t.Context(), connect.NewRequest(&charactersv1.GetCampaignOpenChoicesRequest{CampaignId: tb.campaign}))
	if err != nil || len(res.Msg.GetCharacters()) != 0 {
		t.Errorf("after the death: %v, %v; want no one", res, err)
	}
}

// TestTheLevelUpAsksForTheChoicesAnEarlierLevelLeftOpen: through the API, the offer lists
// the late choices, the level is refused without them (LATE_CHOICE_MISSING) and goes
// through with them.
func TestTheLevelUpAsksForTheChoicesAnEarlierLevelLeftOpen(t *testing.T) {
	t.Parallel()
	tb := newLevelUpTable(t, 6500, 8, 8, 8, 8)
	// Replace Pensantus by a player's fighter with the style open, at the XP of level 5.
	fighter := choiceSheet("race:human", "", "class:fighter", "", 4, "feature:fighter-fighting-style-defense")
	fighter.GetFull().ExperiencePoints = 6500
	complete, err := tb.other.createWith(tb.campaign, unspecifiedMethod, fighter)
	if err != nil {
		t.Fatal(err)
	}
	open := proto.CloneOf(complete.GetSheet())
	open.GetFull().FeatureChoiceKeys = nil
	pc, err := tb.master.update(t, complete, complete.GetName(), open)
	if err != nil {
		t.Fatal(err)
	}
	pc = tb.other.get(t, tb.campaign, pc.GetId())

	res, err := tb.other.api.GetLevelUpOptions(t.Context(), connect.NewRequest(&charactersv1.GetLevelUpOptionsRequest{CampaignId: tb.campaign, CharacterId: pc.GetId()}))
	if err != nil {
		t.Fatal(err)
	}
	late := res.Msg.GetOptions().GetLateChoices()
	if len(late) != 1 || late[0].GetChoices()[0].GetKey() != "feature:fighter-fighting-style" || late[0].GetLevel() != 1 {
		t.Fatalf("late choices = %v, want the style of level 1", late)
	}
	hp := &charactersv1.LevelUpHitPoints{Method: charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_AVERAGE}
	ch := &charactersv1.LevelUpChoices{ClassKey: "class:fighter", HitPoints: hp}
	_, err = tb.levelUp(tb.other, pc, ch)
	var refusal *charactersv1.LevelUpRefusal
	if ce, ok := errors.AsType[*connect.Error](err); ok {
		for _, d := range ce.Details() {
			if v, derr := d.Value(); derr == nil {
				refusal, _ = v.(*charactersv1.LevelUpRefusal)
			}
		}
	}
	if connect.CodeOf(err) != connect.CodeFailedPrecondition || refusal.GetReason() != charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_LATE_CHOICE_MISSING ||
		refusal.GetField() != "full.feature_choice_keys" {
		t.Fatalf("the level without the late style: %v, want LATE_CHOICE_MISSING on the choices field", err)
	}
	ch.LateChoiceKeys = []string{"feature:fighter-fighting-style-archery"}
	done, err := tb.levelUp(tb.other, pc, ch)
	if err != nil {
		t.Fatalf("the level with the late style: %v", err)
	}
	if got := done.GetSheet().GetFull().GetFeatureChoiceKeys(); !slices.Equal(got, []string{"feature:fighter-fighting-style-archery"}) || done.GetDerived().GetTotalLevel() != 5 {
		t.Errorf("after the level: picks %v, level %d", got, done.GetDerived().GetTotalLevel())
	}
}

// A wizard draft of a class changed after two skills were ticked from the old class's list: the preview answers the lists of the
// draft's sources, not a refusal of the picks that no longer fit (the save is what refuses them), so the pickers show the new
// class's list. The refusals of what a player may not have stay (a key the content does not have).
func TestPreviewChoicesAnswersTheListsOfADraftWhoseOldPicksNoLongerFit(t *testing.T) {
	t.Parallel()
	tb := newChoiceTable(t)
	draft := func(class string, skills []string, picks ...string) *charactersv1.CharacterSheet {
		sheet := choiceSheet("race:human", "", class, "", 1, picks...)
		sheet.GetFull().SkillProficiencyKeys = skills
		return sheet
	}
	ticked := []string{"skill:athletics", "skill:acrobatics"}
	check := func(t *testing.T, res *charactersv1.PreviewChoicesResponse, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("the preview of a draft with picks that no longer fit: %v", err)
		}
		keys := res.GetSkillsAllowed().GetKeys()
		if !slices.Contains(keys, "skill:arcana") || slices.Contains(keys, "skill:athletics") || slices.Contains(keys, "skill:acrobatics") {
			t.Errorf("skills allowed = %v, want the wizard's list without the picked athletics and acrobatics", keys)
		}
	}
	t.Run("a new draft", func(t *testing.T) {
		t.Parallel()
		res, err := tb.preview(tb.ana, "", draft("class:wizard", ticked))
		check(t, res, err)
	})
	t.Run("a character of the player", func(t *testing.T) {
		t.Parallel()
		c := tb.ana.create(t, tb.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Mutante", draft("class:fighter", []string{"skill:perception", "skill:survival"}, "feature:fighter-fighting-style-defense"))
		res, err := tb.preview(tb.ana, c.GetId(), draft("class:wizard", ticked))
		check(t, res, err)
		if res.GetDone() != 3 || res.GetTotal() != 3 {
			t.Errorf("done %d of %d, want 3 of 3 for the wizard draft", res.GetDone(), res.GetTotal())
		}
		_, err = tb.ana.update(t, c, "Mutante", draft("class:wizard", ticked))
		wantUnfit(t, err, "sheet.full.skill_proficiency_keys[0]", "Atletismo")
	})
}
