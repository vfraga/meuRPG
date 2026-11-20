package leaktest

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1/charactersv1connect"
	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1/playv1connect"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1/rulesv1connect"
)

// summonProbe is a table whose class has a feature that summons: Ana's character holds it, Caio's does not. The
// canaries are the feature's name and the key of the hidden spell the feature lowers to (Ana reads both).
type summonProbe struct {
	w                *world
	featureName, key string
}

func newSummonProbe(t *testing.T) *summonProbe {
	t.Helper()
	w := &world{stack: newStack(t), secrets: newSecrets(), pts: map[string]*mapsv1.MapPoint{}, puzzles: map[string]*playv1.Puzzle{}, notes: map[string]string{}}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("building the fixture: %v", r)
		}
	}()
	ctx := t.Context()
	w.buildPeople()
	w.session = must(w.master.play.StartGameSession(ctx, rq(&playv1.StartGameSessionRequest{CampaignId: w.campaign}))).GetGameSession().GetId()
	p := &summonProbe{w: w, featureName: w.secrets.marker("hidden-spell-feature-name", w.ana)}
	class := must(w.master.table.CreateTableEntry(ctx, rq(&rulesv1.CreateTableEntryRequest{
		CampaignId: w.campaign, Body: &rulesv1.CreateTableEntryRequest_TableClass{TableClass: summonerClass(
			"Guardiã da Matilha", p.featureName, "O texto da habilidade.",
			&rulesv1.TableSummon{CreatureKeys: []string{"monster:wolf"}, Count: "2", MaxLive: 4, ResourceKey: "chamados"},
		)},
	}))).GetEntry()
	w.pens = must(w.ana.characters.CreateCharacter(ctx, rq(&charactersv1.CreateCharacterRequest{
		CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Pensantus",
		Sheet: &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
			BaseScores: &rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 8},
			RaceKey:    "race:gnome", Classes: []*charactersv1.ClassLevel{{ClassKey: class.GetKey(), Level: 1}},
		}}},
	}))).GetCharacter()
	w.toren = w.pc(w.caio, "Toren", "race:human")
	spells := must(w.ana.characters.GetSummonOptions(ctx, rq(&charactersv1.GetSummonOptionsRequest{CampaignId: w.campaign, CharacterId: w.pens.GetId()}))).GetSpells()
	if len(spells) != 1 || spells[0].GetGrantedBy() == "" {
		t.Fatalf("Ana's options = %v, want the feature's summon", spells)
	}
	p.key = w.secrets.id("hidden-spell-key", spells[0].GetSpellKey(), w.ana)
	return p
}

// The hidden spell of a feature's summon is in no list a player who does not hold the feature reads, and no summon read
// of another player's character names it. Ana, who holds the feature, reads both the name and the key (the controls).
func TestTheHiddenSpellOfAFeatureSummonIsInNoReadOfThoseWhoHoldNotTheFeature(t *testing.T) {
	p := newSummonProbe(t)
	w := p.w
	got := newAnswers()
	rows := []read{
		{
			procedure: rulesv1connect.ContentServiceGetSpellDetailsProcedure, label: "the feature's hidden spell", allow: onlyAna,
			why: "the spell reads to the master and to a player whose character has the feature's entry; Caio's character has not",
			req: func(w *world) proto.Message {
				return &rulesv1.GetSpellDetailsRequest{CampaignId: w.campaign, SpellKey: p.key}
			},
		},
		{
			procedure: rulesv1connect.ContentServiceListSpellsProcedure, label: "the first circle", allow: members,
			why: "the spell catalog lists the SRD's and the table's spells; the spell of a feature's summon (of the first circle) is in no entry of it",
			req: func(w *world) proto.Message {
				return &rulesv1.ListSpellsRequest{CampaignId: w.campaign, Levels: []int32{1}, PageSize: 400}
			},
		},
		{
			procedure: rulesv1connect.ContentServiceListSpellsProcedure, label: "a search for the feature's name", allow: members,
			why: "a search by the name the hidden spell carries finds nothing for a player",
			req: func(w *world) proto.Message {
				return &rulesv1.ListSpellsRequest{CampaignId: w.campaign, Query: p.featureName}
			},
		},
		{
			procedure: charactersv1connect.CharacterServiceGetSummonOptionsProcedure, label: "the holder's character", allow: onlyAna,
			req: func(w *world) proto.Message {
				return &charactersv1.GetSummonOptionsRequest{CampaignId: w.campaign, CharacterId: w.pens.GetId()}
			},
		},
		{
			procedure: charactersv1connect.CharacterServiceGetSummonOptionsProcedure, label: "Caio's character, who does not hold the feature", allow: onlyCaio,
			req: func(w *world) proto.Message {
				return &charactersv1.GetSummonOptionsRequest{CampaignId: w.campaign, CharacterId: w.toren.GetId()}
			},
		},
	}
	for _, r := range rows {
		t.Run(r.name(), func(t *testing.T) { w.runRead(t, r, got) })
	}
	t.Run("canaries are reachable", func(t *testing.T) { checkCanariesAreReachable(t, w, got) })
}

// A key typed on a sheet reads the same whether it is the hidden spell of a feature's summon or one the content does not
// have: the probe of the sheet (the preview) tells Caio nothing about the feature, in the cantrips, the known and the
// prepared spells and a pick. The control is the answer to a key that does not exist, which names the unknown key.
func TestTheHiddenSpellOfAFeatureSummonCannotBeProbedWithASheet(t *testing.T) {
	p := newSummonProbe(t)
	w := p.w
	const unknown = "spell:nao-existe@mesa"
	warlock := func(edit func(f *charactersv1.FullSheet, key string)) func(key string) proto.Message {
		return func(key string) proto.Message {
			f := &charactersv1.FullSheet{
				BaseScores: &rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 8},
				RaceKey:    "race:human", Classes: []*charactersv1.ClassLevel{{ClassKey: "class:warlock", Level: 3}},
				FeatureChoiceKeys: []string{"feature:pact-of-the-tome"},
			}
			edit(f, key)
			return &charactersv1.PreviewCharacterRequest{
				CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER,
				Sheet: &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: f}},
			}
		}
	}
	for _, tc := range []struct {
		name  string
		build func(key string) proto.Message
	}{
		{"cantrips", warlock(func(f *charactersv1.FullSheet, k string) { f.CantripKeys = []string{k} })},
		{"known spells", warlock(func(f *charactersv1.FullSheet, k string) { f.KnownSpellKeys = []string{k} })},
		{"prepared spells", warlock(func(f *charactersv1.FullSheet, k string) { f.PreparedSpellKeys = []string{k} })},
		{"picks", warlock(func(f *charactersv1.FullSheet, k string) {
			f.FeatureChoiceKeys = append(f.FeatureChoiceKeys, "feature:pact-of-the-tome#cantrips="+k)
		})},
		{"book rituals picks", warlock(func(f *charactersv1.FullSheet, k string) {
			book := "feature:eldritch-invocation-book-of-ancient-secrets"
			f.FeatureChoiceKeys = append(f.FeatureChoiceKeys, book, book+"#rituals="+k)
		})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			proc := charactersv1connect.CharacterServicePreviewCharacterProcedure
			hidden, missing := w.caio.call(proc, tc.build(p.key)), w.caio.call(proc, tc.build(unknown))
			if missing.status != 400 || !strings.Contains(string(missing.body), "not a known key") {
				t.Fatalf("a key that does not exist: HTTP %d %s; want the answer that names the unknown key (the control)", missing.status, shorten(missing.body))
			}
			for _, f := range w.inspect(w.caio, hidden, nil) {
				t.Errorf("Caio's probe of the hidden spell: %s\n\tanswer: %s", f, shorten(hidden.body))
			}
			norm := func(r reply, key string) string { return strings.ReplaceAll(string(r.body), key, "KEY") }
			if hidden.status != missing.status || norm(hidden, p.key) != norm(missing, unknown) {
				t.Errorf("the hidden spell reads differently from an unknown key:\n\thidden:  %s\n\tunknown: %s", shorten(hidden.body), shorten(missing.body))
			}
		})
	}
}

// CastSummon as a ritual skips the price of the cast, which is where a spell the character cannot cast is found; the
// feature's key must read as an unknown spell there too, to a player whose character has not the feature.
func TestTheHiddenSpellOfAFeatureSummonCannotBeProbedWithARitual(t *testing.T) {
	p := newSummonProbe(t)
	w := p.w
	const unknown = "spell:nao-existe@mesa"
	cast := func(key string) reply {
		return w.caio.call(playv1connect.PlayServiceCastSummonProcedure, &playv1.CastSummonRequest{
			CampaignId: w.campaign, CharacterId: w.toren.GetId(), SpellKey: key, Ritual: true, IdempotencyKey: newKey(),
			Summon: &playv1.SummonChoice{CreatureKeys: []string{"monster:wolf"}},
		})
	}
	hidden, missing := cast(p.key), cast(unknown)
	if missing.status != 404 {
		t.Fatalf("a key that does not exist: HTTP %d %s; want not_found (the control)", missing.status, shorten(missing.body))
	}
	for _, f := range w.inspect(w.caio, hidden, nil) {
		t.Errorf("Caio's ritual probe of the hidden spell: %s\n\tanswer: %s", f, shorten(hidden.body))
	}
	norm := func(r reply, key string) string { return strings.ReplaceAll(string(r.body), key, "KEY") }
	if hidden.status != missing.status || norm(hidden, p.key) != norm(missing, unknown) {
		t.Errorf("the ritual probe of the hidden spell reads differently from one of an unknown key:\n\thidden:  HTTP %d %s\n\tunknown: HTTP %d %s", hidden.status, shorten(hidden.body), missing.status, shorten(missing.body))
	}
}

// A ritual the master switched off never reaches a player in the Book of Ancient Secrets' choice: Caio's preview of
// a warlock's choices offers it before the switch (the control) and names it nowhere after.
func TestASwitchedOffRitualIsNotOfferedInTheBookChoice(t *testing.T) {
	p := newSummonProbe(t)
	w := p.w
	const off = "spell:detect-magic"
	book := "feature:eldritch-invocation-book-of-ancient-secrets"
	preview := func(who *person) reply {
		return who.call(charactersv1connect.CharacterServicePreviewChoicesProcedure, &charactersv1.PreviewChoicesRequest{
			CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER,
			Sheet: &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
				BaseScores: &rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 8},
				RaceKey:    "race:human", Classes: []*charactersv1.ClassLevel{{ClassKey: "class:warlock", Level: 5}},
				FeatureChoiceKeys: []string{"feature:pact-of-the-tome", book, "feature:eldritch-invocation-devils-sight"},
			}}},
		})
	}
	if r := preview(w.caio); !r.ok() || !strings.Contains(string(r.body), off) {
		t.Fatalf("before the switch Caio's choices lack %s (the control): HTTP %d %s", off, r.status, shorten(r.body))
	}
	must(w.master.table.SetOptionSwitches(t.Context(), rq(&rulesv1.SetOptionSwitchesRequest{CampaignId: w.campaign, Switches: []*rulesv1.OptionSwitch{{Key: off, Off: true}}})))
	if r := preview(w.caio); !r.ok() || strings.Contains(string(r.body), off) {
		t.Errorf("after the switch Caio's choices name the spell: HTTP %d %s", r.status, shorten(r.body))
	}
}
