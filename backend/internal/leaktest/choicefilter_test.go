package leaktest

import (
	"cmp"
	"slices"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1/charactersv1connect"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1/rulesv1connect"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The pool of a choice of tools with a filter of proficiency kinds is the same for every reader: the proficiencies of
// those kinds the content has, and nothing the table wrote; the answer says nothing of the proficiencies it leaves out.
func TestAFilterOfProficiencyKindsOffersTheSamePoolToEveryone(t *testing.T) {
	p := newChoicesProbe(t)
	srd, err := rules.LoadSRD()
	if err != nil {
		t.Fatalf("LoadSRD: %v", err)
	}
	var want []string
	for _, e := range srd.Catalog().Proficiencies {
		if e.Kind == "weapon" || e.Kind == "other" {
			want = append(want, e.Key)
		}
	}
	slices.Sort(want)
	if len(want) == 0 {
		t.Fatal("the content has no weapon or other proficiency")
	}
	class := p.createClass(tableClass("Contrabandista do Cais", &rulesv1.TableFeature{
		NamePt: "Rede do cais", DescPt: []string{"Texto de teste."},
		Effects: []*rulesv1.TableEffect{{Type: "choice", Choice: "tool", Count: 1, Filter: &rulesv1.TableChoiceFilter{ProficiencyKinds: []string{"weapon", "other"}}}},
	}))

	const outside, inside = "proficiency:smiths-tools", "proficiency:glaives"
	for _, who := range p.readers() {
		t.Run(who.name, func(t *testing.T) {
			plain := p.preview(who, sheet(class, 1))
			if who.member != plain.ok() {
				t.Fatalf("HTTP %d for the plain sheet, want a read only for a member", plain.status)
			}
			if !who.member {
				return
			}
			res := choicesOf(t, plain)
			if got := optionsOf(res, "#tool"); !slices.Equal(got, want) {
				t.Errorf("the pool has %d options, want the %d proficiencies of the filter's kinds, none of the table's", len(got), len(want))
			}
			if strings.Contains(string(plain.body), "@mesa\"") && strings.Contains(strings.Join(optionsOf(res, "#tool"), " "), "@mesa") {
				t.Error("the pool holds an entry of the table")
			}
			// What the filter leaves out is named nowhere in the answer, and a pick inside it is taken.
			if strings.Contains(string(plain.body), outside) {
				t.Errorf("the answer names %s, a proficiency of a kind the filter leaves out", outside)
			}
			key := firstChoiceKey(res, "#tool")
			taken := p.preview(who, sheet(class, 1, key+"="+inside))
			if !taken.ok() || len(choicesOf(t, taken).GetNotOffered()) != 0 {
				t.Errorf("the pick of %s: HTTP %d %s; want it taken", inside, taken.status, shorten(taken.body))
			}
		})
	}
}

func firstChoiceKey(res *charactersv1.PreviewChoicesResponse, suffix string) string {
	for _, g := range res.GetGroups() {
		for _, ch := range g.GetChoices() {
			if strings.HasSuffix(ch.GetKey(), suffix) {
				return ch.GetKey()
			}
		}
	}
	return ""
}

// tides is a source of options a choice offers and the class that asks for them (from_feature) at its level 2, the
// ways the table keeps an option out of a reader's offer, and what each way hides.
type tides struct {
	p       *choicesProbe
	asking  string   // the class that asks
	ask     string   // the key of its choice
	feature string   // the source feature
	live    string   // an option that is held before the table closes anything
	out     []string // the options the table keeps out, live among them unless the way keeps it
	names   []string // the names of those options
	close   func(t *testing.T)
	// absent is the stored key of an option that does not exist, and option the key a pick of the completion names for a
	// stored one; both are the plain key (absentKey) for the options of a feature, and are set for a pick of a spell.
	absent string
	option func(stored string) string
	// archived says the table retired the entry: closed to the master too. liveStays says the live option is still on offer.
	archived, liveStays bool
}

// A way the table keeps options out: the source's class is switched off, archived, or one option is switched off.
var tideWays = []string{"the source is switched off", "the source is archived", "an option is switched off"}

func newTides(t *testing.T, way string) *tides {
	t.Helper()
	p := newChoicesProbe(t)
	w := p.w
	must(w.master.campaigns.SetCampaignXpMode(t.Context(), rq(&campaignsv1.SetCampaignXpModeRequest{CampaignId: w.campaign, XpMode: campaignsv1.XpMode_XP_MODE_ENEMIES})))
	option := func(name string) *rulesv1.TableFeature {
		return &rulesv1.TableFeature{NamePt: name, DescPt: []string{"Texto de teste."}}
	}
	feature := &rulesv1.TableFeature{
		NamePt: "Marés", DescPt: []string{"Texto de teste."},
		Effects: []*rulesv1.TableEffect{{Type: "choice", Choice: "feature", Count: 1}},
		Options: []*rulesv1.TableFeature{option("Maré Baixa"), option("Maré Alta")},
	}
	if way != tideWays[2] {
		feature.Options = append(feature.Options, option("Maré Cheia"))
	}
	source := p.createEntry(t, tableClass("Escola das Marés", feature))
	made := source.GetTableClass().GetLevels()[0].GetFeatures()[0]
	td := &tides{p: p, feature: made.GetKey()}
	for _, o := range made.GetOptions() {
		td.out = append(td.out, o.GetKey())
		td.names = append(td.names, o.GetNamePt())
	}
	td.live = td.out[0]
	td.archived, td.liveStays = way == tideWays[1], way == tideWays[2]
	switch way {
	case tideWays[0]:
		td.close = func(t *testing.T) {
			t.Helper()
			must(w.master.table.SetOptionSwitches(t.Context(), rq(&rulesv1.SetOptionSwitchesRequest{CampaignId: w.campaign, Switches: []*rulesv1.OptionSwitch{{Key: source.GetKey(), Off: true}}})))
		}
	case tideWays[1]:
		td.close = func(t *testing.T) {
			t.Helper()
			must(w.master.table.ArchiveTableEntry(t.Context(), rq(&rulesv1.ArchiveTableEntryRequest{CampaignId: w.campaign, Key: source.GetKey()})))
		}
	default:
		extra := must(w.master.table.CreateTableEntry(t.Context(), rq(&rulesv1.CreateTableEntryRequest{
			CampaignId: w.campaign, Body: &rulesv1.CreateTableEntryRequest_TableOption{TableOption: &rulesv1.TableOption{
				NamePt: "Maré Cheia", DescPt: []string{"Texto de teste."}, Set: "table_feature", OfferedBy: td.feature,
			}},
		}))).GetEntry()
		td.out, td.names = []string{extra.GetKey()}, []string{"Maré Cheia"}
		td.close = func(t *testing.T) {
			t.Helper()
			must(w.master.table.SetOptionSwitches(t.Context(), rq(&rulesv1.SetOptionSwitchesRequest{CampaignId: w.campaign, Switches: []*rulesv1.OptionSwitch{{Key: extra.GetKey(), Off: true}}})))
		}
	}
	asking := tableClass("Cantor das Marés")
	asking.SubclassLevel = 5 // the level-ups of the rows stop short of it
	asking.Levels[1].Features = []*rulesv1.TableFeature{{
		NamePt: "Eco das marés", DescPt: []string{"Texto de teste."},
		Effects: []*rulesv1.TableEffect{{Type: "choice", Choice: "feature", Count: 1, FromFeature: td.feature}},
	}}
	made2 := p.createEntry(t, asking)
	td.asking, td.ask = made2.GetKey(), made2.GetTableClass().GetLevels()[1].GetFeatures()[0].GetKey()
	return td
}

// sheetAt is a draft of a character of the asking class at the level, with the picks.
func (td *tides) sheetAt(level int32, picks ...string) *charactersv1.CharacterSheet {
	s := sheet(td.asking, level, picks...)
	s.GetFull().ExperiencePoints = 6500 // enough for the next level, in a campaign that counts XP
	return s
}

// A pick of an option the table keeps out of the offer is answered as a pick of a key the content does not have, by
// every call that takes a pick and for every reader the call lets in. The master reads what is switched off, as every
// other thing the players may not, and is answered so only for what is retired.
func TestAPickOfAnOptionTheTableKeepsOutIsAnsweredAsAKeyThatDoesNotExist(t *testing.T) {
	for _, way := range tideWays {
		t.Run(way, func(t *testing.T) {
			td := newTides(t, way)
			p, w := td.p, td.p.w
			ctx := t.Context()

			// Characters made while every option is on offer: Ana's holds the live option at level 2, and Caio's is at level 1,
			// where the choice is not asked yet. The live option is taken before the table closes anything.
			for _, who := range p.readers() {
				if r := p.preview(who, td.sheetAt(2, td.live)); who.member && (!r.ok() || len(choicesOf(t, r).GetNotOffered()) != 0) {
					t.Fatalf("%s: the pick of %s before the table closes anything: HTTP %d %s", who.name, td.live, r.status, shorten(r.body))
				}
			}
			holder := must(w.ana.characters.CreateCharacter(ctx, rq(&charactersv1.CreateCharacterRequest{
				CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Cantora", Sheet: td.sheetAt(2, td.live),
			}))).GetCharacter()
			rising := must(w.caio.characters.CreateCharacter(ctx, rq(&charactersv1.CreateCharacterRequest{
				CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Cantor", Sheet: td.sheetAt(1),
			}))).GetCharacter()
			if r := w.caio.call(charactersv1connect.CharacterServicePreviewLevelUpProcedure, td.levelUp(rising, td.live)); !r.ok() {
				t.Fatalf("the level-up with %s before the table closes anything: HTTP %d %s", td.live, r.status, shorten(r.body))
			}

			td.close(t)

			td.checkWhatReadersSee(t, holder, rising)
			td.checkDrafts(t)
			td.checkSavedSheet(t, holder)
			td.checkLevelUp(t, rising)
			td.checkHeldPick(t, holder)
			td.checkCompletion(t, holder)
		})
	}
}

const absentKey = "feature:nenhuma--opcao@mesa"

// levelUp is the choices of the level that gives the asking choice, with the picks.
func (td *tides) levelUp(c *charactersv1.Character, picks ...string) *charactersv1.PreviewLevelUpRequest {
	return &charactersv1.PreviewLevelUpRequest{CampaignId: td.p.w.campaign, CharacterId: c.GetId(), Choices: td.choices(picks...)}
}

func (td *tides) choices(picks ...string) *charactersv1.LevelUpChoices {
	return &charactersv1.LevelUpChoices{
		ClassKey: td.asking, FeatureChoiceKeys: picks,
		HitPoints: &charactersv1.LevelUpHitPoints{Method: charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_AVERAGE},
	}
}

// answers compares the answer to a pick of each of the keys with the answer to a key that does not exist, for each
// reader. A reader the table lets pick it (the master, for what is switched off) is answered yes.
func (td *tides) answers(t *testing.T, what string, keys []string, call func(who reader, key string) reply) {
	t.Helper()
	td.answersBy(t, what, false, keys, call)
}

// answersBy is answers, where a call that changes a character is not made by a reader it would let in.
func (td *tides) answersBy(t *testing.T, what string, mutates bool, keys []string, call func(who reader, key string) reply) {
	t.Helper()
	for _, who := range td.p.readers() {
		if mutates && td.mayPick(who) {
			continue
		}
		for _, key := range keys {
			missing := cmp.Or(td.absent, absentKey)
			named, absent := call(who, key), call(who, missing)
			switch {
			case td.mayPick(who):
				if !named.ok() || len(named.body) == 0 {
					t.Errorf("%s, %s: the pick of %s: HTTP %d %s; want it taken", what, who.name, key, named.status, shorten(named.body))
				}
			case !sameAnswer(named, td.optionOf(key), absent, td.optionOf(missing)):
				t.Errorf("%s, %s: the pick of %s: HTTP %d %s\nthe pick of a key that does not exist: HTTP %d %s", what, who.name, key, named.status, shorten(named.body), absent.status, shorten(absent.body))
			}
		}
	}
}

// mayPick says the call takes a pick of an option the table keeps out: the master's, for what is only switched off.
func (td *tides) mayPick(who reader) bool {
	return who.person == td.p.w.master && !td.archived
}

// checkWhatReadersSee checks the answers that show a player the options name none of the ones the table keeps out.
func (td *tides) checkWhatReadersSee(t *testing.T, holder, rising *charactersv1.Character) {
	t.Helper()
	w := td.p.w
	for _, who := range td.p.readers() {
		if !who.member || who.person == w.master {
			continue
		}
		plain := td.p.preview(who, td.sheetAt(2))
		if !plain.ok() {
			t.Fatalf("%s: the plain sheet: HTTP %d %s", who.name, plain.status, shorten(plain.body))
		}
		td.expectNone(t, who.name+"'s choices", plain.body, td.out, td.names)
	}
	up := w.caio.call(charactersv1connect.CharacterServiceGetLevelUpOptionsProcedure, &charactersv1.GetLevelUpOptionsRequest{CampaignId: w.campaign, CharacterId: rising.GetId()})
	if !up.ok() {
		t.Fatalf("GetLevelUpOptions: HTTP %d %s", up.status, shorten(up.body))
	}
	td.expectNone(t, "the level-up options", up.body, td.out, td.names)
	// The sheet that holds the live option names it, and the others stay out of its answer.
	got := w.ana.call(charactersv1connect.CharacterServiceGetCharacterProcedure, &charactersv1.GetCharacterRequest{CampaignId: w.campaign, CharacterId: holder.GetId()})
	if !got.ok() {
		t.Fatalf("GetCharacter: HTTP %d %s", got.status, shorten(got.body))
	}
	td.expectNone(t, "the character", got.body, td.fresh(), td.freshNames())
}

// fresh are the options kept out that the holder's sheet does not hold.
func (td *tides) fresh() []string {
	return slices.DeleteFunc(slices.Clone(td.out), func(k string) bool { return k == td.live })
}

func (td *tides) freshNames() []string {
	var out []string
	for i, k := range td.out {
		if k != td.live {
			out = append(out, td.names[i])
		}
	}
	return out
}

// checkDrafts checks the calls that take a sheet that does not exist yet.
func (td *tides) checkDrafts(t *testing.T) {
	t.Helper()
	td.answers(t, "PreviewChoices", td.out, func(who reader, key string) reply { return td.p.preview(who, td.sheetAt(2, key)) })
	td.answers(t, "PreviewCharacter", td.out, func(who reader, key string) reply {
		return who.person.call(charactersv1connect.CharacterServicePreviewCharacterProcedure, &charactersv1.PreviewCharacterRequest{
			CampaignId: td.p.w.campaign, Kind: who.kind, Sheet: td.sheetAt(2, key),
		})
	})
	td.answers(t, "CreateCharacter", td.out, func(who reader, key string) reply {
		return who.person.call(charactersv1connect.CharacterServiceCreateCharacterProcedure, &charactersv1.CreateCharacterRequest{
			CampaignId: td.p.w.campaign, Kind: who.kind, Name: "Marés", Sheet: td.sheetAt(2, key),
		})
	})
}

// checkSavedSheet checks the save of a sheet that takes another option than the one it holds.
func (td *tides) checkSavedSheet(t *testing.T, holder *charactersv1.Character) {
	t.Helper()
	revision := holder.GetRevision()
	td.answersBy(t, "UpdateCharacter", true, td.fresh(), func(who reader, key string) reply {
		return who.person.call(charactersv1connect.CharacterServiceUpdateCharacterProcedure, &charactersv1.UpdateCharacterRequest{
			CampaignId: td.p.w.campaign, CharacterId: holder.GetId(), Revision: revision, Name: holder.GetName(), Sheet: td.sheetAt(2, key),
		})
	})
}

// checkLevelUp checks the level that gives the choice, for the player whose character it is and for the others.
func (td *tides) checkLevelUp(t *testing.T, rising *charactersv1.Character) {
	t.Helper()
	td.answers(t, "PreviewLevelUp", td.out, func(who reader, key string) reply {
		return who.person.call(charactersv1connect.CharacterServicePreviewLevelUpProcedure, td.levelUp(rising, key))
	})
	td.answersBy(t, "LevelUpCharacter", true, td.out, func(who reader, key string) reply {
		return who.person.call(charactersv1connect.CharacterServiceLevelUpCharacterProcedure, &charactersv1.LevelUpCharacterRequest{
			CampaignId: td.p.w.campaign, CharacterId: rising.GetId(), Revision: rising.GetRevision(), Choices: td.choices(key),
		})
	})
}

// checkHeldPick checks a pick the sheet made before the table closed the option stays: the sheet saves and levels up.
func (td *tides) checkHeldPick(t *testing.T, holder *charactersv1.Character) {
	t.Helper()
	w, ctx := td.p.w, t.Context()
	saved, err := w.ana.characters.UpdateCharacter(ctx, rq(&charactersv1.UpdateCharacterRequest{
		CampaignId: w.campaign, CharacterId: holder.GetId(), Revision: holder.GetRevision(), Name: "Cantora das Marés", Sheet: td.sheetAt(2, td.live),
	}))
	if err != nil {
		t.Fatalf("the save of the sheet that holds %s: %v", td.live, err)
	}
	if !slices.Contains(saved.Msg.GetCharacter().GetSheet().GetFull().GetFeatureChoiceKeys(), td.live) {
		t.Errorf("the saved sheet lost the pick %s", td.live)
	}
	level, err := w.ana.characters.LevelUpCharacter(ctx, rq(&charactersv1.LevelUpCharacterRequest{
		CampaignId: w.campaign, CharacterId: holder.GetId(), Revision: saved.Msg.GetCharacter().GetRevision(), Choices: td.choices(),
	}))
	if err != nil {
		t.Fatalf("the level-up of the sheet that holds %s: %v", td.live, err)
	}
	if !slices.Contains(level.Msg.GetCharacter().GetSheet().GetFull().GetFeatureChoiceKeys(), td.live) {
		t.Errorf("the sheet at the next level lost the pick %s", td.live)
	}
	// A live option is taken by a sheet that does not hold it, where the table keeps none out of the offer.
	if td.liveStays {
		for _, who := range td.p.readers() {
			if r := td.p.preview(who, td.sheetAt(2, td.live)); who.member && (!r.ok() || len(choicesOf(t, r).GetNotOffered()) != 0) {
				t.Errorf("%s: the pick of the live option %s: HTTP %d %s; want it taken", who.name, td.live, r.status, shorten(r.body))
			}
		}
	}
}

// checkCompletion checks the choice a locked sheet left open: the picks of the options the table keeps out are answered
// as picks of a key that does not exist, and so is their preview.
func (td *tides) checkCompletion(t *testing.T, holder *charactersv1.Character) {
	t.Helper()
	w, ctx := td.p.w, t.Context()
	must(w.master.play.StartGameSession(ctx, rq(&playv1.StartGameSessionRequest{CampaignId: w.campaign})))
	raw := charactersv1connect.NewCharacterServiceClient(w.master.client, w.server.URL)
	current := must(w.ana.characters.GetCharacter(ctx, rq(&charactersv1.GetCharacterRequest{CampaignId: w.campaign, CharacterId: holder.GetId()}))).GetCharacter()
	open := proto.CloneOf(current.GetSheet())
	open.GetFull().FeatureChoiceKeys = nil
	current = must(raw.UpdateCharacter(ctx, rq(&charactersv1.UpdateCharacterRequest{
		CampaignId: w.campaign, CharacterId: holder.GetId(), Revision: current.GetRevision(), Name: current.GetName(), Sheet: open,
	}))).GetCharacter()
	td.answers(t, "PreviewChoices of a locked sheet", td.out, func(who reader, key string) reply {
		return who.person.call(charactersv1connect.CharacterServicePreviewChoicesProcedure, &charactersv1.PreviewChoicesRequest{
			CampaignId: w.campaign, CharacterId: holder.GetId(), Kind: who.kind, Sheet: td.sheetAt(current.GetSheet().GetFull().GetClasses()[0].GetLevel(), key),
		})
	})
	td.answersBy(t, "CompleteCharacterChoices", true, td.out, func(who reader, key string) reply {
		return who.person.call(charactersv1connect.CharacterServiceCompleteCharacterChoicesProcedure, &charactersv1.CompleteCharacterChoicesRequest{
			CampaignId: w.campaign, CharacterId: holder.GetId(), ExpectedRevision: current.GetRevision(),
			Picks: []*charactersv1.ChoicePick{{ChoiceKey: td.ask, OptionKeys: []string{td.optionOf(key)}}},
		})
	})
}

// optionOf is the key of the option a pick of the completion names for the stored key.
func (td *tides) optionOf(stored string) string {
	if td.option == nil {
		return stored
	}
	return td.option(stored)
}

// expectNone checks the answer of a call names none of the options.
func (td *tides) expectNone(t *testing.T, what string, body []byte, keys, names []string) {
	t.Helper()
	for _, k := range keys {
		if strings.Contains(string(body), k) {
			t.Errorf("%s name the option %s the table keeps out", what, k)
		}
	}
	for _, n := range names {
		if strings.Contains(string(body), n) {
			t.Errorf("%s name the option %q the table keeps out", what, n)
		}
	}
}

// The entries a player lists name no feature of a class the table keeps out: the asking choice of another class carries
// no key of it. The master reads the key.
func TestTheEntriesAPlayerListsNameNoFeatureOfAnEntryTheTableKeepsOut(t *testing.T) {
	for _, way := range tideWays[:2] {
		t.Run(way, func(t *testing.T) {
			td := newTides(t, way)
			w := td.p.w
			list := &rulesv1.ListTableEntriesRequest{CampaignId: w.campaign}
			if r := w.master.call(rulesv1connect.TableContentServiceListTableEntriesProcedure, list); !r.ok() || !strings.Contains(string(r.body), td.feature) {
				t.Fatalf("the master's entries before the table closes the class: HTTP %d, want the feature %s named", r.status, td.feature)
			}
			td.close(t)
			if r := w.master.call(rulesv1connect.TableContentServiceListTableEntriesProcedure, list); !r.ok() || !strings.Contains(string(r.body), td.feature) {
				t.Errorf("the master's entries: HTTP %d, want the feature %s named", r.status, td.feature)
			}
			for _, who := range []*person{w.ana, w.caio} {
				r := who.call(rulesv1connect.TableContentServiceListTableEntriesProcedure, list)
				if !r.ok() {
					t.Errorf("%s: HTTP %d %s", who.name, r.status, shorten(r.body))
					continue
				}
				if strings.Contains(string(r.body), td.feature) || !strings.Contains(string(r.body), td.asking) {
					t.Errorf("%s: the entries name the feature %s (or leave out the asking class %s)", who.name, td.feature, td.asking)
				}
			}
		})
	}
}
