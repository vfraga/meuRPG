package leaktest

import (
	"bytes"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1/charactersv1connect"
	progressionv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/progression/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// holdsNeither fails when the answer holds the key or the text of the option no level offers.
func (w *world) holdsNeither(t *testing.T, what, body string) {
	t.Helper()
	for _, needle := range []string{w.unofferedKey, w.unofferedLabel} {
		if strings.Contains(body, needle) {
			t.Errorf("%s holds %q: %s", what, needle, shorten([]byte(body)))
		}
	}
}

// levelUpAnswers are the two ways of sending the key of an option no level offers.
func (w *world) levelUpAnswers() map[string]*charactersv1.LevelUpChoices {
	hp := &charactersv1.LevelUpHitPoints{Method: charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_AVERAGE}
	return map[string]*charactersv1.LevelUpChoices{
		"a pick": {ClassKey: "class:wizard", HitPoints: hp, FeatureChoiceKeys: []string{w.unofferedKey}},
		"a swap": {ClassKey: "class:wizard", HitPoints: hp, SwapFeatureOptions: []*charactersv1.LevelUpPickSwap{{Group: "feature:arcane-tradition", FromKey: "feature:arcane-tradition", ToKey: w.unofferedKey}}},
	}
}

// A level-up answers a key the level does not offer with the refusal alone: no sheet of afterwards and no sentence built from
// the key's entry, for the preview and for the level-up, whether the key is a pick or the option a swap takes. The master's
// preview reads as before (the positive control).
func TestALevelUpAnswersAKeyTheLevelDoesNotOfferWithTheRefusalAlone(t *testing.T) {
	w := newWorld(t)
	ctx := t.Context()
	for name, ch := range w.levelUpAnswers() {
		preview, err := w.ana.characters.PreviewLevelUp(ctx, rq(&charactersv1.PreviewLevelUpRequest{CampaignId: w.campaign, CharacterId: w.pens.GetId(), Choices: ch}))
		if err != nil {
			t.Fatalf("%s: preview error = %v", name, err)
		}
		res := preview.Msg
		if res.GetRefusal().GetField() == "" || res.GetAfter() != nil || res.GetMulticlassSummary() != nil {
			t.Errorf("%s: the preview answers refusal %v and a sheet %v, want the refusal alone", name, res.GetRefusal(), res.GetAfter() != nil)
		}
		w.holdsNeither(t, name+": the preview", protojson.Format(res))

		_, err = w.ana.characters.LevelUpCharacter(ctx, rq(&charactersv1.LevelUpCharacterRequest{CampaignId: w.campaign, CharacterId: w.pens.GetId(), Revision: w.pens.GetRevision(), Choices: ch}))
		if err == nil || connect.CodeOf(err) != connect.CodeFailedPrecondition {
			t.Errorf("%s: the level-up answers %v, want a refusal", name, err)
			continue
		}
		answers := err.Error()
		for _, d := range connect.NewError(connect.CodeUnknown, err).Details() {
			if v, derr := d.Value(); derr == nil {
				answers += protojson.Format(v)
			}
		}
		w.holdsNeither(t, name+": the level-up", answers)
	}

	// The master reads the sheet of afterwards with the sentence of the key's entry.
	control, err := w.master.characters.PreviewLevelUp(ctx, rq(&charactersv1.PreviewLevelUpRequest{CampaignId: w.campaign, CharacterId: w.pens.GetId(), Choices: w.levelUpAnswers()["a pick"]}))
	if err != nil {
		t.Fatal(err)
	}
	if control.Msg.GetAfter() == nil || !strings.Contains(protojson.Format(control.Msg), w.unofferedLabel) {
		t.Errorf("the master's preview = %v, want the sheet of afterwards with the sentence of the entry", protojson.Format(control.Msg))
	}
}

// The sentence of a pick that no feature of the sheet offers is the same for a player whatever the pick's entry: the editor's
// preview, the choices preview and the stored sheet say "Uma opção não vale para este personagem." where the master reads the
// sentence of the entry.
func TestTheOfferedChoicesOfASheetAreToldTheSameToThePlayer(t *testing.T) {
	w := newWorld(t)
	ctx := t.Context()
	const generic = "Uma opção não vale para este personagem."
	cur := must(w.ana.characters.GetCharacter(ctx, rq(&charactersv1.GetCharacterRequest{CampaignId: w.campaign, CharacterId: w.pens.GetId()}))).GetCharacter()
	sheet := proto.CloneOf(cur.GetSheet())
	sheet.GetFull().FeatureChoiceKeys = append(sheet.GetFull().FeatureChoiceKeys, w.unofferedKey)

	for who, p := range map[string]*person{"ana": w.ana, "master": w.master} {
		wantNamed := p == w.master
		preview, err := p.characters.PreviewCharacter(ctx, rq(&charactersv1.PreviewCharacterRequest{
			CampaignId: w.campaign, Kind: cur.GetKind(), Sheet: sheet, ForPlayer: wantNamed,
		}))
		if err != nil {
			t.Fatalf("%s: the editor's preview answers %v", who, err)
		}
		w.told(t, who+": the editor's preview", protojson.Format(preview.Msg), wantNamed, generic)

		choices, err := p.characters.PreviewChoices(ctx, rq(&charactersv1.PreviewChoicesRequest{CampaignId: w.campaign, CharacterId: w.pens.GetId(), Kind: cur.GetKind(), Sheet: sheet}))
		if err != nil {
			t.Fatalf("%s: the choices preview answers %v", who, err)
		}
		if body := protojson.Format(choices.Msg); strings.Contains(body, w.unofferedLabel) {
			t.Errorf("%s: the choices preview holds the text of the entry: %s", who, shorten([]byte(body)))
		}
	}

	// The stored sheet: the master puts the key on it, and each reads the sentence of the sheet's issues.
	must(w.master.characters.UpdateCharacter(ctx, rq(&charactersv1.UpdateCharacterRequest{
		CampaignId: w.campaign, CharacterId: w.pens.GetId(), Revision: cur.GetRevision(), Name: cur.GetName(), Sheet: sheet,
	})))
	for who, p := range map[string]*person{"ana": w.ana, "master": w.master} {
		got := must(p.characters.GetCharacter(ctx, rq(&charactersv1.GetCharacterRequest{CampaignId: w.campaign, CharacterId: w.pens.GetId()})))
		w.told(t, who+": the stored sheet", protojson.Format(got.GetCharacter().GetDerived()), p == w.master, generic)
	}
}

// told checks the sentence of the issue about the option: the entry's own for the master, the generic one for a player, who
// reads nothing of the entry.
func (w *world) told(t *testing.T, what, body string, master bool, generic string) {
	t.Helper()
	if master {
		if !strings.Contains(body, w.unofferedLabel+" não vale para este personagem.") {
			t.Errorf("%s: the master reads no sentence of the entry: %s", what, shorten([]byte(body)))
		}
		return
	}
	if !strings.Contains(body, generic) {
		t.Errorf("%s: the player reads no generic sentence: %s", what, shorten([]byte(body)))
	}
	if strings.Contains(body, w.unofferedLabel) {
		t.Errorf("%s: holds the text of the entry: %s", what, shorten([]byte(body)))
	}
}

// offerKeys are the keys a level-up of the world's warlock does not offer, one for each way the table can hold a key back, and
// the keys a request with a nonexistent one is compared with.
type offerKeys struct {
	// plain are keys of options; each has its way in its name.
	plain map[string]string
	// scoped are, by the owner of the choice, the picks of its choice that no player is offered, by the same ways.
	scoped map[string]map[string]string
	// nowhere is the key of an option that does not exist.
	nowhere string
}

const (
	tomeOwner = "feature:pact-of-the-tome#cantrips"
	bookOwner = "feature:eldritch-invocation-book-of-ancient-secrets#rituals"
	bookKey   = "feature:eldritch-invocation-book-of-ancient-secrets"
	nowhere   = "feature:no-such-option-anywhere"
	noSpell   = "spell:no-such-spell-anywhere"
)

// warlockOfTheTome makes Pensantus a Pact of the Tome warlock at the end of level 4, with the experience of the next levels.
func (w *world) warlockOfTheTome() {
	ctx := w.t.Context()
	cur := must(w.master.characters.GetCharacter(ctx, rq(&charactersv1.GetCharacterRequest{CampaignId: w.campaign, CharacterId: w.pens.GetId()}))).GetCharacter()
	const inv = "feature:eldritch-invocation-"
	sheet := &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
		BaseScores: &rulesv1.AbilityScores{Strength: 15, Dexterity: 14, Constitution: 13, Intelligence: 10, Wisdom: 12, Charisma: 8}, RaceKey: "race:human",
		Classes:    []*charactersv1.ClassLevel{{ClassKey: "class:warlock", Level: 4, Subclass: &charactersv1.ClassLevel_SubclassKey{SubclassKey: "subclass:fiend"}}},
		Background: &charactersv1.FullSheet_BackgroundKey{BackgroundKey: "background:acolyte"}, Languages: []string{"Anão", "Élfico", "Gnômico"},
		FeatureChoiceKeys: []string{
			"feature:pact-of-the-tome", inv + "devils-sight", inv + "beast-speech",
			"feature:pact-of-the-tome#cantrips=spell:guidance", "feature:pact-of-the-tome#cantrips=spell:shillelagh", "feature:pact-of-the-tome#cantrips=spell:eldritch-blast",
		},
		CantripKeys:      []string{"spell:mage-hand", "spell:chill-touch", "spell:prestidigitation"},
		KnownSpellKeys:   []string{"spell:charm-person", "spell:expeditious-retreat", "spell:hellish-rebuke", "spell:protection-from-evil-and-good", "spell:darkness"},
		ExperiencePoints: 14000,
	}}}
	must(w.master.characters.UpdateCharacter(ctx, rq(&charactersv1.UpdateCharacterRequest{CampaignId: w.campaign, CharacterId: w.pens.GetId(), Revision: cur.GetRevision(), Name: cur.GetName(), Sheet: sheet})))
	// The table levels up by milestones: three more reached ones are the levels the warlock has gone up.
	for i := range 3 {
		ms := must(w.master.xp.AddMilestone(ctx, rq(&progressionv1.AddMilestoneRequest{CampaignId: w.campaign, Text: "Marco de teste " + strconv.Itoa(i)}))).GetMilestone()
		must(w.master.xp.MarkMilestoneReached(ctx, rq(&progressionv1.MarkMilestoneReachedRequest{CampaignId: w.campaign, MilestoneId: ms.GetId(), CharacterIds: []string{w.pens.GetId()}, IdempotencyKey: newKey()})))
	}
	w.pens = must(w.master.characters.GetCharacter(ctx, rq(&charactersv1.GetCharacterRequest{CampaignId: w.campaign, CharacterId: w.pens.GetId()}))).GetCharacter()
}

// bookLeftOpen levels the warlock up with the Book of Ancient Secrets and without its rituals, and gives the table one more
// milestone: the next level is left with the two rituals as a late choice.
func (w *world) bookLeftOpen() {
	ctx := w.t.Context()
	hp := &charactersv1.LevelUpHitPoints{Method: charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_AVERAGE}
	choices := &charactersv1.LevelUpChoices{ClassKey: "class:warlock", HitPoints: hp, KnownSpellKeys: []string{"spell:hold-person"}, FeatureChoiceKeys: []string{bookKey}}
	must(w.ana.characters.LevelUpCharacter(ctx, rq(&charactersv1.LevelUpCharacterRequest{CampaignId: w.campaign, CharacterId: w.pens.GetId(), Revision: w.pens.GetRevision(), Choices: choices})))
	ms := must(w.master.xp.AddMilestone(ctx, rq(&progressionv1.AddMilestoneRequest{CampaignId: w.campaign, Text: "Marco do livro"}))).GetMilestone()
	must(w.master.xp.MarkMilestoneReached(ctx, rq(&progressionv1.MarkMilestoneReachedRequest{CampaignId: w.campaign, MilestoneId: ms.GetId(), CharacterIds: []string{w.pens.GetId()}, IdempotencyKey: newKey()})))
	w.pens = must(w.master.characters.GetCharacter(ctx, rq(&charactersv1.GetCharacterRequest{CampaignId: w.campaign, CharacterId: w.pens.GetId()}))).GetCharacter()
}

// holdBackKeys makes the keys of offerKeys: the table entries that are switched off, retired, offered by a feature that is,
// another table's, and one that is visible and not offered.
func (w *world) holdBackKeys() offerKeys {
	ctx := w.t.Context()
	m := w.master
	entry := func(campaign string, body *rulesv1.CreateTableEntryRequest) *rulesv1.TableEntry {
		body.CampaignId = campaign
		return must(m.table.CreateTableEntry(ctx, rq(body))).GetEntry()
	}
	subclass := func(campaign, name string) *rulesv1.TableEntry {
		return entry(campaign, &rulesv1.CreateTableEntryRequest{Body: &rulesv1.CreateTableEntryRequest_TableSubclass{TableSubclass: &rulesv1.TableSubclass{
			NamePt: name, ClassKey: "class:fighter",
			Levels: []*rulesv1.TableSubclassLevel{{Level: 3, Features: []*rulesv1.TableFeature{{
				NamePt: name + ", característica", Effects: []*rulesv1.TableEffect{{Type: "choice", Choice: "feature", Count: 1}},
				Options: []*rulesv1.TableFeature{{NamePt: name + ", opção"}},
			}}}},
		}}})
	}
	firstOption := func(e *rulesv1.TableEntry) string {
		return e.GetTableSubclass().GetLevels()[0].GetFeatures()[0].GetOptions()[0].GetKey()
	}
	archive := func(campaign, key string) {
		must(m.table.ArchiveTableEntry(ctx, rq(&rulesv1.ArchiveTableEntryRequest{CampaignId: campaign, Key: key})))
	}
	spell := func(campaign, name string, level int32, ritual bool) string {
		sp := &rulesv1.TableSpell{
			NamePt: name, Level: level, Ritual: ritual, SchoolKey: "school:evocation",
			CastingTime: &rulesv1.TableSpellCastingTime{Unit: rulesv1.CastingTimeUnit_CASTING_TIME_UNIT_ACTION, Amount: 1},
			Range:       &rulesv1.TableSpellRange{Kind: rulesv1.SpellRangeKind_SPELL_RANGE_KIND_RANGED, DistanceFt: 60},
			Duration:    &rulesv1.TableSpellDuration{Kind: rulesv1.SpellDurationKind_SPELL_DURATION_KIND_INSTANTANEOUS},
			Components:  &rulesv1.TableSpellComponents{Verbal: true, Somatic: true},
			ClassKeys:   []string{"class:wizard"}, DescPt: []string{"Um raio de teste."},
			Target: &rulesv1.TableSpellTarget{Kind: rulesv1.TableSpellTargetKind_TABLE_SPELL_TARGET_KIND_CREATURE},
			Attack: "ranged", Damage: []*rulesv1.TableSpellDamage{{DamageTypeKey: "damage-type:force", Dice: "3d6"}},
		}
		if ritual {
			sp.Duration = &rulesv1.TableSpellDuration{Kind: rulesv1.SpellDurationKind_SPELL_DURATION_KIND_TIMED, Amount: 10, Unit: rulesv1.SpellDurationUnit_SPELL_DURATION_UNIT_MINUTE}
		}
		return entry(campaign, &rulesv1.CreateTableEntryRequest{Body: &rulesv1.CreateTableEntryRequest_TableSpell{TableSpell: sp}}).GetKey()
	}
	liveFeature := subclass(w.campaign, "Escola das Posturas")
	feature := liveFeature.GetTableSubclass().GetLevels()[0].GetFeatures()[0].GetKey()
	option := func(name string) string {
		return entry(w.campaign, &rulesv1.CreateTableEntryRequest{Body: &rulesv1.CreateTableEntryRequest_TableOption{TableOption: &rulesv1.TableOption{
			NamePt: name, Set: "table_feature", OfferedBy: feature, DescPt: []string{name + "."},
		}}}).GetKey()
	}
	archivedOption, offOption := option("Postura aposentada"), option("Postura desligada")
	archive(w.campaign, archivedOption)
	offSubclass := subclass(w.campaign, "Escola desligada")
	other := must(m.campaigns.CreateCampaign(ctx, rq(&campaignsv1.CreateCampaignRequest{Name: "Outra mesa", XpMode: campaignsv1.XpMode_XP_MODE_MILESTONES}))).GetCampaign().GetId()
	otherSubclass := subclass(other, "Escola de outra mesa")

	archivedCantrip, archivedRitual := spell(w.campaign, "Truque aposentado", 0, false), spell(w.campaign, "Ritual aposentado", 1, true)
	archive(w.campaign, archivedCantrip)
	archive(w.campaign, archivedRitual)
	otherCantrip, otherRitual := spell(other, "Truque de outra mesa", 0, false), spell(other, "Ritual de outra mesa", 1, true)
	must(m.table.SetOptionSwitches(ctx, rq(&rulesv1.SetOptionSwitchesRequest{CampaignId: w.campaign, Switches: []*rulesv1.OptionSwitch{
		{Key: offOption, Off: true}, {Key: offSubclass.GetKey(), Off: true}, {Key: "spell:resistance", Off: true}, {Key: "spell:detect-magic", Off: true},
	}})))
	return offerKeys{
		plain: map[string]string{
			"switched off": offOption, "archived": archivedOption, "offered by a retired feature": w.unofferedKey,
			"offered by a switched-off subclass": firstOption(offSubclass), "of another table": firstOption(otherSubclass),
			"visible and not offered": firstOption(liveFeature), "of the rules and not offered": "feature:pact-of-the-chain",
		},
		scoped: map[string]map[string]string{
			tomeOwner: {"switched off": "spell:resistance", "archived": archivedCantrip, "of another table": otherCantrip, "visible and not offered": "spell:fireball"},
			bookOwner: {"switched off": "spell:detect-magic", "archived": archivedRitual, "of another table": otherRitual, "visible and not offered": "spell:magic-missile"},
		},
		nowhere: nowhere,
	}
}

// levelUpAsk is one way of sending a key: the field it goes in, the choices that carry it, the keys to send and the key of the
// same kind that does not exist.
type levelUpAsk struct {
	name, field string
	choices     func(key string) *charactersv1.LevelUpChoices
	keys        map[string]string
	nowhere     string
	// offered and also are the two keys the level offers the player for a late choice (empty when it offers none for the ask).
	offered, also string
}

// A level-up answers every key it does not offer, whatever way the table holds the key back, with what it answers a key that
// does not exist: the same status, message and details, and no sheet. The key is sent as a pick, as a late pick, as the option
// a swap takes and as the pick of the choice of an option the character has or takes, to the preview and to the level-up, by the
// character's player and by those who do not reach it.
func TestALevelUpAnswersAnyKeyItDoesNotOfferAsAKeyThatDoesNotExist(t *testing.T) {
	w := newWorld(t)
	w.warlockOfTheTome()
	w.bookLeftOpen()
	keys := w.holdBackKeys()
	hp := &charactersv1.LevelUpHitPoints{Method: charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_AVERAGE}
	base := func() *charactersv1.LevelUpChoices {
		return &charactersv1.LevelUpChoices{ClassKey: "class:warlock", HitPoints: hp}
	}
	asks := []levelUpAsk{
		{name: "a pick", field: "choices.feature_choice_keys", keys: keys.plain, nowhere: keys.nowhere, choices: func(k string) *charactersv1.LevelUpChoices {
			c := base()
			c.FeatureChoiceKeys = []string{k}
			return c
		}},
		{name: "a late pick", field: "choices.late_choice_keys", offered: bookOwner + "=spell:alarm", also: bookOwner + "=spell:comprehend-languages", keys: keys.plain, nowhere: keys.nowhere, choices: func(k string) *charactersv1.LevelUpChoices {
			c := base()
			c.LateChoiceKeys = []string{k}
			return c
		}},
		{name: "the option of a swap", field: "choices.swap_feature_options", keys: keys.plain, nowhere: keys.nowhere, choices: func(k string) *charactersv1.LevelUpChoices {
			c := base()
			c.SwapFeatureOptions = []*charactersv1.LevelUpPickSwap{{Group: "feature:eldritch-invocations", FromKey: "feature:eldritch-invocation-devils-sight", ToKey: k}}
			return c
		}},
	}
	for owner, values := range keys.scoped {
		withOwner := strings.HasPrefix(owner, bookKey)
		offered, also := "", ""
		if owner == bookOwner {
			offered, also = "spell:alarm", "spell:comprehend-languages"
		}
		asks = append(asks, levelUpAsk{name: "a pick of the choice " + owner, field: "choices.feature_choice_keys", keys: values, nowhere: noSpell, choices: func(v string) *charactersv1.LevelUpChoices {
			c := base()
			c.FeatureChoiceKeys = []string{owner + "=" + v}
			if withOwner {
				c.FeatureChoiceKeys = append([]string{bookKey}, c.FeatureChoiceKeys...)
			}
			return c
		}},
			levelUpAsk{name: "a late pick of the choice " + owner, field: "choices.late_choice_keys", offered: offered, also: also, keys: values, nowhere: noSpell, choices: func(v string) *charactersv1.LevelUpChoices {
				c := base()
				c.LateChoiceKeys = []string{owner + "=" + v}
				return c
			}})
	}
	callers := []struct {
		name   string
		person *person
		// reaches says the call lets the caller past authorization to the character.
		reaches bool
	}{{"the player", w.ana, true}, {"a member who waits for approval", w.pending, false}, {"someone outside the campaign", w.stranger, false}}
	procedures := map[string]func(c *charactersv1.LevelUpChoices) proto.Message{
		charactersv1connect.CharacterServicePreviewLevelUpProcedure: func(c *charactersv1.LevelUpChoices) proto.Message {
			return &charactersv1.PreviewLevelUpRequest{CampaignId: w.campaign, CharacterId: w.pens.GetId(), Choices: c}
		},
		charactersv1connect.CharacterServiceLevelUpCharacterProcedure: func(c *charactersv1.LevelUpChoices) proto.Message {
			return &charactersv1.LevelUpCharacterRequest{CampaignId: w.campaign, CharacterId: w.pens.GetId(), Revision: w.pens.GetRevision(), Choices: c}
		},
	}
	for procedure, request := range procedures {
		for _, who := range callers {
			for _, ask := range asks {
				if ask.offered != "" && who.reaches && procedure == charactersv1connect.CharacterServicePreviewLevelUpProcedure {
					// The positive control: the world has a late choice open, and the player's call with the keys it offers is accepted.
					c := ask.choices(ask.offered)
					c.LateChoiceKeys = append(c.LateChoiceKeys, ask.choices(ask.also).LateChoiceKeys...)
					c.KnownSpellKeys = []string{"spell:fear"}
					got := who.person.call(procedure, request(c))
					if !got.ok() || strings.Contains(string(got.body), ask.field) || !strings.Contains(string(got.body), `"after"`) {
						t.Fatalf("%s, %s, %s: the keys the level offers are answered HTTP %d %s, want them accepted with a sheet of afterwards", procedure, who.name, ask.name, got.status, shorten(got.body))
					}
				}
				control := who.person.call(procedure, request(ask.choices(ask.nowhere)))
				switch {
				case who.reaches && procedure == charactersv1connect.CharacterServicePreviewLevelUpProcedure:
					if !control.ok() || !strings.Contains(string(control.body), ask.field) || strings.Contains(string(control.body), `"after"`) {
						t.Fatalf("%s, %s, %s: the answer to a key that does not exist is HTTP %d %s, want the refusal of %s alone", procedure, who.name, ask.name, control.status, shorten(control.body), ask.field)
					}
				case who.reaches:
					if control.status != http.StatusPreconditionFailed && control.status != http.StatusBadRequest || !strings.Contains(string(control.body), ask.field) {
						t.Fatalf("%s, %s, %s: the answer to a key that does not exist is HTTP %d %s, want the refusal of %s", procedure, who.name, ask.name, control.status, shorten(control.body), ask.field)
					}
				case control.ok():
					t.Fatalf("%s, %s, %s: the call went through: %s", procedure, who.name, ask.name, shorten(control.body))
				}
				for way, key := range ask.keys {
					got := who.person.call(procedure, request(ask.choices(key)))
					if got.status != control.status || !bytes.Equal(got.body, control.body) {
						t.Errorf("%s, %s, %s, a key %s: HTTP %d %s, want what a key that does not exist is told, HTTP %d %s",
							procedure, who.name, ask.name, way, got.status, shorten(got.body), control.status, shorten(control.body))
					}
				}
			}
		}
	}
}
