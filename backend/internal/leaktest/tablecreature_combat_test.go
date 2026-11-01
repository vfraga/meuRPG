package leaktest

import (
	"strconv"
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

// A table creature that Ana's character held when the master archived its entry fights beside her. Everyone in the
// combat reads who it is, its name and its key, as they read an SRD summon's or an NPC's. The stat block is not
// theirs: Caio, a third player, reads none of its hit points, armor class, trait, actions nor the SRD creature it
// comes from, and the creature's sheet (GetCreature) is not found for him. Ana (its owner) and the master read them.
func TestAnArchivedTableCreatureInTheCombatShowsAThirdPlayerItsIdentityAndNothingElse(t *testing.T) {
	const hp, ac = 6161, 37
	w := &world{stack: newStack(t), secrets: newSecrets(), pts: map[string]*mapsv1.MapPoint{}, puzzles: map[string]*playv1.Puzzle{}, notes: map[string]string{}}
	ctx := t.Context()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("building the fixture: %v", r)
		}
	}()
	w.buildPeople()
	m := w.master
	w.session = must(m.play.StartGameSession(ctx, rq(&playv1.StartGameSessionRequest{CampaignId: w.campaign}))).GetGameSession().GetId()
	scores := &rulesv1.AbilityScores{Strength: 16, Dexterity: 14, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 14}
	hero := func(owner *person, name, class string) *charactersv1.Character {
		return must(owner.characters.CreateCharacter(ctx, rq(&charactersv1.CreateCharacterRequest{
			CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: name,
			Sheet: &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
				BaseScores: scores, RaceKey: "race:human", Classes: []*charactersv1.ClassLevel{{ClassKey: class, Level: 3}},
			}}},
		}))).GetCharacter()
	}
	w.pens, w.toren = hero(w.ana, "Pensantus", "class:barbarian"), hero(w.caio, "Toren", "class:fighter")

	// The entry: the name and the key are the combat's to read; the rest is Ana's and the master's.
	entry := must(m.table.CreateTableEntry(ctx, rq(&rulesv1.CreateTableEntryRequest{
		CampaignId: w.campaign, Body: &rulesv1.CreateTableEntryRequest_TableCreature{TableCreature: &rulesv1.TableCreature{
			NamePt: w.secrets.marker("fight-creature-name", w.ana, w.caio), BaseKey: "monster:giant-crab", HitPoints: hp, ArmorClass: ac,
			Traits: []*rulesv1.TableCreatureTrait{{NamePt: w.secrets.marker("fight-creature-trait-name", w.ana), TextPt: w.secrets.marker("fight-creature-trait-text", w.ana)}},
		}},
	}))).GetEntry()
	key := entry.GetKey()
	w.secrets.id("fight-creature-key", key, w.ana, w.caio)
	w.giveTableCreature(w.pens, key)
	must(m.table.ArchiveTableEntry(ctx, rq(&rulesv1.ArchiveTableEntryRequest{CampaignId: w.campaign, Key: key})))
	w.secrets.number("fight-creature-hp", hp)
	w.secrets.number("fight-creature-ac", ac, "armor_class")
	w.secrets.allowNeedle(strconv.Itoa(hp), w.ana)
	w.secrets.allowNeedle(strconv.Itoa(ac), w.ana)
	w.secrets.add(&canary{needle: "monster:giant-crab", kind: "fight-creature-base-key"})
	w.secrets.add(&canary{needle: key + "#claw", kind: "fight-creature-action-key", readers: names([]*person{w.ana})})

	goblin := must(m.characters.CreateNpcFromCreature(ctx, rq(&charactersv1.CreateNpcFromCreatureRequest{
		CampaignId: w.campaign, CreatureKey: "monster:goblin", Name: w.secrets.marker("fight-goblin-name"), Kind: charactersv1.CharacterKind_CHARACTER_KIND_MINION, IdempotencyKey: newKey(),
	}))).GetCharacter()
	w.secrets.add(&canary{needle: "monster:goblin", kind: "monster-key"})
	e := must(m.combat.StartEncounter(ctx, rq(&playv1.StartEncounterRequest{
		CampaignId: w.campaign, IdempotencyKey: newKey(), Name: "Emboscada", Mode: playv1.EncounterMode_ENCOUNTER_MODE_THEATRE,
		Participants: []*playv1.Participant{{CharacterId: goblin.GetId()}},
	}))).GetEncounter()
	for _, c := range e.GetCombatants() {
		must(m.combat.SubmitInitiative(ctx, rq(&playv1.SubmitInitiativeRequest{
			CampaignId: w.campaign, EncounterId: e.GetId(), CombatantId: c.GetId(), IdempotencyKey: newKey(),
			Roll: &playv1.SubmitInitiativeRequest_D20Face{D20Face: 10},
		})))
	}
	w.encounter = must(m.combat.GetEncounter(ctx, rq(&playv1.GetEncounterRequest{CampaignId: w.campaign}))).GetEncounter()
	var creature *playv1.Combatant
	for _, c := range w.encounter.GetCombatants() {
		if c.GetKind() == playv1.CombatantKind_COMBATANT_KIND_CREATURE {
			creature = c
		}
	}
	if creature == nil {
		t.Fatalf("the creature is not in the combat: %v", w.encounter.GetCombatants())
	}

	rows := []read{
		{
			procedure: playv1connect.CombatServiceGetEncounterProcedure, allow: members,
			req: func(w *world) proto.Message { return &playv1.GetEncounterRequest{CampaignId: w.campaign} },
		},
		{
			procedure: playv1connect.CombatServiceListCombatLogProcedure, allow: members,
			req: func(w *world) proto.Message {
				return &playv1.ListCombatLogRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId()}
			},
		},
		{
			procedure: playv1connect.CombatServiceGetTurnOptionsProcedure, label: "the table creature", allow: onlyAna, why: "the creature's options are its owner's and the master's",
			req: func(w *world) proto.Message {
				return &playv1.GetTurnOptionsRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), CombatantId: creature.GetId()}
			},
		},
		{
			procedure: rulesv1connect.ContentServiceGetCreatureProcedure, label: "the archived table creature", allow: onlyAna, why: "a table creature is not found by a player who holds none, archived or not; Ana's character holds it",
			req: func(w *world) proto.Message { return &rulesv1.GetCreatureRequest{CampaignId: w.campaign, Key: key} },
		},
		{
			procedure: rulesv1connect.ContentServiceListCreaturesProcedure, allow: members, ignore: []string{"fight-creature-base-key", "monster-key"}, why: "the SRD's list names the base crab; the table creature is in the list of the master and of Ana, who holds it, and of no one else",
			req: func(w *world) proto.Message {
				return &rulesv1.ListCreaturesRequest{CampaignId: w.campaign, PageSize: 400}
			},
		},
		{
			procedure: charactersv1connect.CharacterServiceListCharacterCreaturesProcedure, allow: onlyAna,
			req: func(w *world) proto.Message {
				return &charactersv1.ListCharacterCreaturesRequest{CampaignId: w.campaign, CharacterId: w.pens.GetId()}
			},
		},
		{
			procedure: rulesv1connect.TableContentServiceListTableEntriesProcedure, allow: members, ignore: []string{"fight-creature-base-key"}, why: "the entry is a table creature: its body, base key included, is the master's and its holder's, and Caio's list leaves it out",
			req: func(w *world) proto.Message { return &rulesv1.ListTableEntriesRequest{CampaignId: w.campaign} },
		},
	}
	got := newAnswers()
	for _, r := range rows {
		t.Run(r.name(), func(t *testing.T) { w.runRead(t, r, got) })
	}

	// The identity is the combat's: Caio reads the name and the key; and no more of the creature than that.
	enc := must(w.caio.combat.GetEncounter(ctx, rq(&playv1.GetEncounterRequest{CampaignId: w.campaign}))).GetEncounter()
	for _, c := range enc.GetCombatants() {
		if c.GetKind() != playv1.CombatantKind_COMBATANT_KIND_CREATURE {
			continue
		}
		if c.GetMonsterKey() != key || !strings.HasPrefix(c.GetMonsterNamePt(), markerPrefix) {
			t.Errorf("Caio reads the creature as %v, want its table key and name", c)
		}
		if c.HitPointsCurrent != nil || c.HitPointsMax != nil || c.ArmorClass != nil {
			t.Errorf("Caio reads the creature's numbers: %v", c)
		}
	}
	t.Run("canaries are reachable", func(t *testing.T) { checkCanariesAreReachable(t, w, got) })
}
