package leaktest

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1/charactersv1connect"
	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1/rulesv1connect"
)

// A table feature that summons a table creature (#578). The class is public (every member reads that it exists, and the
// feature's name and text), but what the feature summons is not: the creature's key and the count formula are the
// master's, and the holder's. A member who holds none of the creature reads neither in the table's entries; the holder
// reads the key and the name of the creature in the options of the summon (she must pick it) and, once she holds it,
// in the entries too. Caio, who holds none, reads neither at any step. The count formula is the master's alone. The
// hidden spell the feature lowers to is no entry: no read lists it.
func TestATableFeatureThatSummonsKeepsItsCreatureAndCountFromThoseWhoHoldNone(t *testing.T) {
	const countFormula = "1 + 3 * prof()"
	w := &world{stack: newStack(t), secrets: newSecrets(), pts: map[string]*mapsv1.MapPoint{}, puzzles: map[string]*playv1.Puzzle{}, notes: map[string]string{}}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("building the fixture: %v", r)
		}
	}()
	ctx := t.Context()
	w.buildPeople()
	m := w.master
	w.session = must(m.play.StartGameSession(ctx, rq(&playv1.StartGameSessionRequest{CampaignId: w.campaign}))).GetGameSession().GetId()

	creature := must(m.table.CreateTableEntry(ctx, rq(&rulesv1.CreateTableEntryRequest{
		CampaignId: w.campaign, Body: &rulesv1.CreateTableEntryRequest_TableCreature{TableCreature: &rulesv1.TableCreature{
			NamePt: w.secrets.marker("feature-summon-creature-name", w.ana), BaseKey: "monster:giant-crab", HitPoints: 31,
		}},
	}))).GetEntry()
	creatureKey := creature.GetKey()
	w.secrets.id("feature-summon-creature-key", creatureKey, w.ana)
	w.secrets.add(&canary{needle: countFormula, kind: "feature-summon-count-formula"})
	class := must(m.table.CreateTableEntry(ctx, rq(&rulesv1.CreateTableEntryRequest{
		CampaignId: w.campaign, Body: &rulesv1.CreateTableEntryRequest_TableClass{TableClass: summonerClass(
			w.secrets.public("feature-summon-class-name"), w.secrets.public("feature-summon-feature-name"), w.secrets.public("feature-summon-feature-text"),
			&rulesv1.TableSummon{CreatureKeys: []string{creatureKey}, Count: countFormula, MaxLive: 8, ResourceKey: "chamados"},
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

	entries := read{
		procedure: rulesv1connect.TableContentServiceListTableEntriesProcedure, allow: members,
		why: "the creature's own entry is the holder's (Ana, once she holds one); the feature's summon names none the reader does not hold",
		req: func(w *world) proto.Message { return &rulesv1.ListTableEntriesRequest{CampaignId: w.campaign} },
	}
	options := read{
		procedure: charactersv1connect.CharacterServiceGetSummonOptionsProcedure, label: "Ana's character", allow: onlyAna,
		req: func(w *world) proto.Message {
			return &charactersv1.GetSummonOptionsRequest{CampaignId: w.campaign, CharacterId: w.pens.GetId()}
		},
	}
	got := newAnswers()

	t.Run("before the cast", func(t *testing.T) {
		w.runRead(t, options, got)
		w.runRead(t, entries, got)
	})
	for _, p := range []*person{w.ana, w.caio} {
		body := string(p.call(rulesv1connect.TableContentServiceListTableEntriesProcedure, &rulesv1.ListTableEntriesRequest{CampaignId: w.campaign}).body)
		if strings.Contains(body, creatureKey) || strings.Contains(body, countFormula) {
			t.Errorf("%s, who holds no creature, reads the feature's creature or count in the entries: %s", p.name, shorten([]byte(body)))
		}
		if !strings.Contains(body, class.GetKey()) {
			t.Errorf("%s does not read the class in the entries: the control is missing", p.name)
		}
	}
	masterBody := string(m.call(rulesv1connect.TableContentServiceListTableEntriesProcedure, &rulesv1.ListTableEntriesRequest{CampaignId: w.campaign}).body)
	if !strings.Contains(masterBody, creatureKey) || !strings.Contains(masterBody, countFormula) {
		t.Errorf("the master does not read the creature and the count in the entries (the positive control): %s", shorten([]byte(masterBody)))
	}
	optsRes := w.ana.call(charactersv1connect.CharacterServiceGetSummonOptionsProcedure, &charactersv1.GetSummonOptionsRequest{CampaignId: w.campaign, CharacterId: w.pens.GetId()})
	if !strings.Contains(string(optsRes.body), creatureKey) {
		t.Errorf("Ana's options do not name the creature (the positive control): %s", shorten(optsRes.body))
	}
	summons := must(w.ana.characters.GetSummonOptions(ctx, rq(&charactersv1.GetSummonOptionsRequest{CampaignId: w.campaign, CharacterId: w.pens.GetId()}))).GetSpells()
	if len(summons) != 1 || summons[0].GetGrantedBy() == "" {
		t.Fatalf("Ana's options = %v, want the feature's summon", summons)
	}
	// No read lists the hidden spell the feature lowers to: it is in no entry of the table.
	if strings.Contains(masterBody, summons[0].GetSpellKey()) {
		t.Errorf("the master's entries list the feature's hidden spell %q", summons[0].GetSpellKey())
	}

	// The cast: Ana now holds the creatures. Her entries carry the creature (it is hers), Caio's still do not.
	must(w.ana.play.CastSummon(ctx, rq(&playv1.CastSummonRequest{
		CampaignId: w.campaign, CharacterId: w.pens.GetId(), SpellKey: summons[0].GetSpellKey(), GrantedBy: summons[0].GetGrantedBy(), IdempotencyKey: newKey(),
		Summon: &playv1.SummonChoice{CreatureKeys: []string{creatureKey, creatureKey, creatureKey, creatureKey, creatureKey, creatureKey, creatureKey}},
	})))
	rows := []read{
		options, entries,
		{
			procedure: charactersv1connect.CharacterServiceListCharacterCreaturesProcedure, allow: onlyAna,
			req: func(w *world) proto.Message {
				return &charactersv1.ListCharacterCreaturesRequest{CampaignId: w.campaign, CharacterId: w.pens.GetId()}
			},
		},
		{
			procedure: rulesv1connect.ContentServiceGetCreatureProcedure, label: "the summoned creature", allow: onlyAna, why: "a table creature is not found by a player who holds none",
			req: func(w *world) proto.Message {
				return &rulesv1.GetCreatureRequest{CampaignId: w.campaign, Key: creatureKey}
			},
		},
	}
	for _, r := range rows {
		t.Run("after the cast "+r.name(), func(t *testing.T) { w.runRead(t, r, got) })
	}
	t.Run("canaries are reachable", func(t *testing.T) { checkCanariesAreReachable(t, w, got) })
}

// The spell a feature's summon lowers to belongs to the feature's entry: GetSpellDetails answers it to the master and to
// a player whose character has the entry, whether the entry is live or archived, and as an unknown spell to every other
// member. The canary is the feature's name, which the spell carries. The entry is a class, a race or a class feature of an SRD class.
func TestTheHiddenSpellOfAFeatureSummonIsReadOnlyByTheMasterAndTheHolderOfItsEntry(t *testing.T) {
	for _, owner := range []string{"class", "race", "classfeature"} {
		t.Run(owner, func(t *testing.T) { hiddenSpellOfAFeatureSummon(t, owner) })
	}
}

func hiddenSpellOfAFeatureSummon(t *testing.T, owner string) {
	t.Helper()
	w := &world{stack: newStack(t), secrets: newSecrets(), pts: map[string]*mapsv1.MapPoint{}, puzzles: map[string]*playv1.Puzzle{}, notes: map[string]string{}}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("building the fixture: %v", r)
		}
	}()
	ctx := t.Context()
	w.buildPeople()
	m := w.master
	featureName := w.secrets.marker("feature-summon-spell-feature-name-"+owner, w.ana)
	summon := &rulesv1.TableSummon{CreatureKeys: []string{"monster:wolf"}, Count: "2", MaxLive: 4, ResourceKey: "chamados"}
	var entry *rulesv1.TableEntry
	sheet := &charactersv1.FullSheet{BaseScores: &rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 8}}
	switch owner {
	case "classfeature":
		entry = must(m.table.CreateTableEntry(ctx, rq(&rulesv1.CreateTableEntryRequest{
			CampaignId: w.campaign, Body: &rulesv1.CreateTableEntryRequest_TableClassFeature{TableClassFeature: &rulesv1.TableClassFeature{
				NamePt: featureName, DescPt: []string{"O texto da habilidade."}, ClassKey: "class:fighter", Level: 1, Summon: summon,
				Effects: []*rulesv1.TableEffect{{Type: "resource", Resource: "chamados", Max: "2", Recharge: "long_rest"}},
			}},
		}))).GetEntry()
		sheet.RaceKey, sheet.Classes = "race:gnome", []*charactersv1.ClassLevel{{ClassKey: "class:fighter", Level: 1}}
	case "class":
		entry = must(m.table.CreateTableEntry(ctx, rq(&rulesv1.CreateTableEntryRequest{
			CampaignId: w.campaign, Body: &rulesv1.CreateTableEntryRequest_TableClass{TableClass: summonerClass("Guardiã da Matilha", featureName, "O texto da habilidade.", summon)},
		}))).GetEntry()
		sheet.RaceKey, sheet.Classes = "race:gnome", []*charactersv1.ClassLevel{{ClassKey: entry.GetKey(), Level: 1}}
	default:
		entry = must(m.table.CreateTableEntry(ctx, rq(&rulesv1.CreateTableEntryRequest{
			CampaignId: w.campaign, Body: &rulesv1.CreateTableEntryRequest_TableRace{TableRace: &rulesv1.TableRace{
				NamePt: "Povo da Matilha", Size: "Medium", SpeedFt: 30, Languages: []string{"language:common"},
				Traits: []*rulesv1.TableFeature{{
					NamePt: featureName, DescPt: []string{"O texto do traço."}, Summon: summon,
					Effects: []*rulesv1.TableEffect{{Type: "resource", Resource: "chamados", Max: "2", Recharge: "long_rest"}},
				}},
			}},
		}))).GetEntry()
		sheet.RaceKey, sheet.Classes = entry.GetKey(), []*charactersv1.ClassLevel{{ClassKey: "class:fighter", Level: 1}}
	}
	w.pens = must(w.ana.characters.CreateCharacter(ctx, rq(&charactersv1.CreateCharacterRequest{
		CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Pensantus",
		Sheet: &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: sheet}},
	}))).GetCharacter()
	w.toren = w.pc(w.caio, "Toren", "race:human")
	spell := must(w.ana.characters.GetSummonOptions(ctx, rq(&charactersv1.GetSummonOptionsRequest{CampaignId: w.campaign, CharacterId: w.pens.GetId()}))).GetSpells()[0].GetSpellKey()
	details := func(p *person) reply {
		return p.call(rulesv1connect.ContentServiceGetSpellDetailsProcedure, &rulesv1.GetSpellDetailsRequest{CampaignId: w.campaign, SpellKey: spell})
	}
	check := func(when string) {
		t.Helper()
		for _, p := range []*person{m, w.ana} {
			if r := details(p); !r.ok() || !strings.Contains(string(r.body), featureName) {
				t.Errorf("%s: %s does not read the hidden spell of the feature (the positive control): %d %s", when, p.name, r.status, shorten(r.body))
			}
		}
		if r := details(w.caio); r.ok() || strings.Contains(string(r.body), featureName) {
			t.Errorf("%s: Caio, whose character has not the entry, reads the feature's hidden spell: %d %s", when, r.status, shorten(r.body))
		}
	}
	check("live " + owner)
	must(m.table.ArchiveTableEntry(ctx, rq(&rulesv1.ArchiveTableEntryRequest{CampaignId: w.campaign, Key: entry.GetKey()})))
	check("archived " + owner)
	must(m.table.UnarchiveTableEntry(ctx, rq(&rulesv1.UnarchiveTableEntryRequest{CampaignId: w.campaign, Key: entry.GetKey()})))
	must(m.table.SetOptionSwitches(ctx, rq(&rulesv1.SetOptionSwitchesRequest{CampaignId: w.campaign, Switches: []*rulesv1.OptionSwitch{{Key: entry.GetKey(), Off: true}}})))
	check("off " + owner)
}

// summonerClass is a table class whose level 1 feature summons: two uses of "chamados" back on a long rest.
func summonerClass(name, feature, text string, summon *rulesv1.TableSummon) *rulesv1.TableClass {
	c := &rulesv1.TableClass{
		NamePt: name, HitDie: 8,
		SavingThrows: []rulesv1.Ability{rulesv1.Ability_ABILITY_CONSTITUTION, rulesv1.Ability_ABILITY_WISDOM},
		SkillChoose:  2, SkillFrom: []string{"skill:arcana", "skill:history", "skill:medicine", "skill:nature", "skill:survival"},
		Proficiencies: []string{"proficiency:light-armor", "proficiency:simple-weapons"}, SubclassLevel: 3,
	}
	for lvl := 1; lvl <= 20; lvl++ {
		row := &rulesv1.TableClassLevel{}
		if lvl == 1 {
			row.Features = []*rulesv1.TableFeature{{
				NamePt: feature, DescPt: []string{text}, Summon: summon,
				Effects: []*rulesv1.TableEffect{{Type: "resource", Resource: "chamados", Max: "2", Recharge: "long_rest"}},
			}}
		}
		c.Levels = append(c.Levels, row)
	}
	return c
}

// A table class feature that summons (#578): an entry of its own on an SRD class. The creature's key and the count
// formula are the master's and the holder's, as a class feature's of a table class: Ana, whose character has the
// class, reads the creature in her options and no count; Caio, who holds none, reads neither at any step. The master
// reads both (the positive control).
func TestATableClassFeatureThatSummonsKeepsItsCreatureAndCountFromThoseWhoHoldNone(t *testing.T) {
	const countFormula = "2 + 5 * prof()"
	w := &world{stack: newStack(t), secrets: newSecrets(), pts: map[string]*mapsv1.MapPoint{}, puzzles: map[string]*playv1.Puzzle{}, notes: map[string]string{}}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("building the fixture: %v", r)
		}
	}()
	ctx := t.Context()
	w.buildPeople()
	m := w.master
	w.session = must(m.play.StartGameSession(ctx, rq(&playv1.StartGameSessionRequest{CampaignId: w.campaign}))).GetGameSession().GetId()

	creatureKey := must(m.table.CreateTableEntry(ctx, rq(&rulesv1.CreateTableEntryRequest{
		CampaignId: w.campaign, Body: &rulesv1.CreateTableEntryRequest_TableCreature{TableCreature: &rulesv1.TableCreature{
			NamePt: w.secrets.marker("class-feature-summon-creature-name", w.ana), BaseKey: "monster:giant-crab", HitPoints: 31,
		}},
	}))).GetEntry().GetKey()
	w.secrets.id("class-feature-summon-creature-key", creatureKey, w.ana)
	w.secrets.add(&canary{needle: countFormula, kind: "class-feature-summon-count-formula"})
	feature := must(m.table.CreateTableEntry(ctx, rq(&rulesv1.CreateTableEntryRequest{
		CampaignId: w.campaign, Body: &rulesv1.CreateTableEntryRequest_TableClassFeature{TableClassFeature: &rulesv1.TableClassFeature{
			NamePt: w.secrets.public("class-feature-summon-feature-name"), DescPt: []string{w.secrets.public("class-feature-summon-feature-text")},
			ClassKey: "class:fighter", Level: 1,
			Effects: []*rulesv1.TableEffect{{Type: "resource", Resource: "chamados", Max: "2", Recharge: "long_rest"}},
			Summon:  &rulesv1.TableSummon{CreatureKeys: []string{creatureKey}, Count: countFormula, MaxLive: 8, ResourceKey: "chamados"},
		}},
	}))).GetEntry()
	w.pens = must(w.ana.characters.CreateCharacter(ctx, rq(&charactersv1.CreateCharacterRequest{
		CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Pensantus",
		Sheet: &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
			BaseScores: &rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 8},
			RaceKey:    "race:gnome", Classes: []*charactersv1.ClassLevel{{ClassKey: "class:fighter", Level: 1}},
		}}},
	}))).GetCharacter()
	w.toren = w.pc(w.caio, "Toren", "race:human")

	entries := read{
		procedure: rulesv1connect.TableContentServiceListTableEntriesProcedure, allow: members,
		why: "the class feature's summon names no creature the reader does not hold, and no count",
		req: func(w *world) proto.Message { return &rulesv1.ListTableEntriesRequest{CampaignId: w.campaign} },
	}
	options := read{
		procedure: charactersv1connect.CharacterServiceGetSummonOptionsProcedure, label: "Ana's character", allow: onlyAna,
		req: func(w *world) proto.Message {
			return &charactersv1.GetSummonOptionsRequest{CampaignId: w.campaign, CharacterId: w.pens.GetId()}
		},
	}
	got := newAnswers()
	t.Run("reads", func(t *testing.T) {
		w.runRead(t, options, got)
		w.runRead(t, entries, got)
	})
	listed := rulesv1connect.TableContentServiceListTableEntriesProcedure
	for _, p := range []*person{w.ana, w.caio} {
		body := string(p.call(listed, &rulesv1.ListTableEntriesRequest{CampaignId: w.campaign}).body)
		if strings.Contains(body, creatureKey) || strings.Contains(body, countFormula) {
			t.Errorf("%s, who holds no creature, reads the class feature's creature or count: %s", p.name, shorten([]byte(body)))
		}
		if !strings.Contains(body, feature.GetKey()) {
			t.Errorf("%s does not read the class feature in the entries: the control is missing", p.name)
		}
	}
	masterBody := string(m.call(listed, &rulesv1.ListTableEntriesRequest{CampaignId: w.campaign}).body)
	if !strings.Contains(masterBody, creatureKey) || !strings.Contains(masterBody, countFormula) {
		t.Errorf("the master does not read the creature and the count (the positive control): %s", shorten([]byte(masterBody)))
	}
	optsRes := w.ana.call(charactersv1connect.CharacterServiceGetSummonOptionsProcedure, &charactersv1.GetSummonOptionsRequest{CampaignId: w.campaign, CharacterId: w.pens.GetId()})
	if !strings.Contains(string(optsRes.body), creatureKey) {
		t.Errorf("Ana's options do not name the creature (the positive control): %s", shorten(optsRes.body))
	}
	t.Run("canaries are reachable", func(t *testing.T) { checkCanariesAreReachable(t, w, got) })
}
