package leaktest

import (
	"net/http"
	"slices"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1/charactersv1connect"
	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1/rulesv1connect"
)

// The damage lines of a table creature (damage_mods: resistances, immunities, vulnerabilities) are its stat block, and
// the stat block is read by the master and by the player whose character HOLDS the creature (they run it at the table;
// RN-23). A player who holds none (Caio) reads none of the lines, nor anything of the creature, whether the master's entry
// is live, archived or switched off in "Opções para os jogadores". The creature here is immune to necrotic and vulnerable
// to radiant, which the giant crab it is based on is neither; the SRD names those types in a hundred other places, so the
// lines are looked for by their shape (a TableCreatureDamageMod, or a Creature of the entry's key that carries them), and
// the creature itself by its key and its name, which are canaries.

const (
	damageLineImmune     = "damage-type:necrotic"
	damageLineVulnerable = "damage-type:radiant"
)

// damageLinesOf counts what a reply says of the table creatures' damage lines: the damage_mods of an entry body, and the
// immunities and vulnerabilities of a stat block of one of the keys that name the entry's two types.
func fieldOf(m protoreflect.Message, name string) protoreflect.Value {
	return m.Get(m.Descriptor().Fields().ByName(protoreflect.Name(name)))
}

func damageLinesOf(r reply, keys ...string) (mods, statBlocks int) {
	if r.msg == nil {
		return 0, 0
	}
	walk(r.msg.ProtoReflect(), "", func(_ string, _ protoreflect.Message, fd protoreflect.FieldDescriptor, v protoreflect.Value) {
		if fd.Message() == nil || fd.IsMap() {
			return
		}
		c := v.Message() // the answers are decoded dynamically: the messages are recognized by name
		switch c.Descriptor().FullName() {
		case "meurpg.rules.v1.TableCreatureDamageMod":
			mods++
		case "meurpg.rules.v1.Creature":
			if !slices.Contains(keys, fieldOf(fieldOf(c, "summary").Message(), "key").String()) {
				return
			}
			for _, name := range []string{"immunities", "vulnerabilities"} {
				l := fieldOf(c, name).List()
				for i := range l.Len() {
					types := fieldOf(l.Get(i).Message(), "types").List()
					for j := range types.Len() {
						if k := fieldOf(types.Get(j).Message(), "key").String(); k == damageLineImmune || k == damageLineVulnerable {
							statBlocks++
						}
					}
				}
			}
		}
	})
	return mods, statBlocks
}

func TestTheDamageLinesOfATableCreatureAreItsHolderAndTheMastersAlone(t *testing.T) {
	w := &world{stack: newStack(t), secrets: newSecrets(), pts: map[string]*mapsv1.MapPoint{}, puzzles: map[string]*playv1.Puzzle{}, notes: map[string]string{}}
	ctx := t.Context()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("building the fixture: %v", r)
		}
	}()
	w.buildPeople()
	m, ana, caio := w.master, w.ana, w.caio
	scores := &rulesv1.AbilityScores{Strength: 16, Dexterity: 14, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 14}
	hero := func(owner *person, name, class string) *charactersv1.Character {
		return must(owner.characters.CreateCharacter(ctx, rq(&charactersv1.CreateCharacterRequest{
			CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: name,
			Sheet: &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
				BaseScores: scores, RaceKey: "race:human", Classes: []*charactersv1.ClassLevel{{ClassKey: class, Level: 3}},
			}}},
		}))).GetCharacter()
	}
	w.pens, w.toren = hero(ana, "Pensantus", "class:barbarian"), hero(caio, "Toren", "class:fighter")

	// Three entries with the same lines: one Ana's character holds (it is archived later), one it holds and the master
	// switches off, and one nobody holds (the master's own, hidden from every player).
	lines := []*rulesv1.TableCreatureDamageMod{
		{Kind: "immune", DamageTypes: []string{damageLineImmune}},
		{Kind: "vulnerable", DamageTypes: []string{damageLineVulnerable}},
	}
	create := func(kind string, holder *charactersv1.Character) string {
		readers := []*person{}
		if holder != nil {
			readers = append(readers, ana)
		}
		entry := must(m.table.CreateTableEntry(ctx, rq(&rulesv1.CreateTableEntryRequest{
			CampaignId: w.campaign, Body: &rulesv1.CreateTableEntryRequest_TableCreature{TableCreature: &rulesv1.TableCreature{
				NamePt: w.secrets.marker("lines-"+kind+"-name", readers...), BaseKey: "monster:giant-crab", DamageMods: lines,
			}},
		}))).GetEntry()
		key := w.secrets.id("lines-"+kind+"-key", entry.GetKey(), readers...)
		if holder != nil {
			w.giveTableCreature(holder, key)
		}
		return key
	}
	archived, off, unheld := create("archived", w.pens), create("off", w.pens), create("unheld", nil)
	keys := []string{archived, off, unheld}

	var got *answers
	rows := func(key string, holds bool) []read {
		who := onlyAna
		if !holds {
			who = masterOnlyRead
		}
		return []read{
			{
				procedure: rulesv1connect.ContentServiceGetCreatureProcedure, label: key, allow: who, why: "a table creature is not found by a player who holds none",
				req: func(w *world) proto.Message { return &rulesv1.GetCreatureRequest{CampaignId: w.campaign, Key: key} },
			},
			{
				procedure: rulesv1connect.ContentServiceListCreaturesProcedure, label: key, allow: members, ignore: []string{"monster-key"}, why: "the SRD's list names the base crab; the table creature is in the list of its holder and of the master only",
				req: func(w *world) proto.Message {
					return &rulesv1.ListCreaturesRequest{CampaignId: w.campaign, PageSize: 400}
				},
			},
			{
				procedure: charactersv1connect.CharacterServiceListCharacterCreaturesProcedure, label: key, allow: onlyAna,
				req: func(w *world) proto.Message {
					return &charactersv1.ListCharacterCreaturesRequest{CampaignId: w.campaign, CharacterId: w.pens.GetId()}
				},
			},
			{
				procedure: rulesv1connect.TableContentServiceListTableEntriesProcedure, label: key, allow: members, ignore: []string{"monster-key"}, why: "a table creature's entry, base key included, is the master's and its holder's",
				req: func(w *world) proto.Message { return &rulesv1.ListTableEntriesRequest{CampaignId: w.campaign} },
			},
		}
	}
	w.secrets.add(&canary{needle: "monster:giant-crab", kind: "monster-key"})

	// check runs every row of the state and asserts the lines: the master reads them in the entry's body and in the stat
	// block; Ana reads the stat block of what she holds; Caio reads no line anywhere.
	check := func(t *testing.T, state string, holds map[string]bool) {
		t.Helper()
		got = newAnswers()
		for _, key := range keys {
			for _, r := range rows(key, holds[key]) {
				t.Run(state+" "+r.name(), func(t *testing.T) { w.runRead(t, r, got) })
			}
		}
		read := func(p *person, proc string, req proto.Message) reply {
			r := p.call(proc, req)
			got.keep(p, r)
			return r
		}
		for _, key := range keys {
			// The master: the lines in the body of the entry and in the stat block, whatever the state.
			if mods, _ := damageLinesOf(read(m, rulesv1connect.TableContentServiceListTableEntriesProcedure, &rulesv1.ListTableEntriesRequest{CampaignId: w.campaign}), key); mods < 2*len(keys) {
				t.Errorf("%s: the master reads %d damage_mods in ListTableEntries, want %d (the positive control)", state, mods, 2*len(keys))
			}
			if _, blocks := damageLinesOf(read(m, rulesv1connect.ContentServiceGetCreatureProcedure, &rulesv1.GetCreatureRequest{CampaignId: w.campaign, Key: key}), key); blocks != 2 {
				t.Errorf("%s: the master's GetCreature of %s shows %d of the two lines, want 2", state, key, blocks)
			}
			// Ana reads the stat block's lines of what she holds, and nothing of what she does not.
			rg := read(ana, rulesv1connect.ContentServiceGetCreatureProcedure, &rulesv1.GetCreatureRequest{CampaignId: w.campaign, Key: key})
			_, blocks := damageLinesOf(rg, key)
			switch {
			case holds[key] && blocks != 2:
				t.Errorf("%s: Ana (the holder) reads %d of the two lines in GetCreature of %s, want 2", state, blocks, key)
			case !holds[key] && (rg.ok() || blocks != 0):
				t.Errorf("%s: Ana, who holds %s no more, got an answer (%d lines)", state, key, blocks)
			}
			// Caio holds none: not found, and no line anywhere he reads.
			rc := read(caio, rulesv1connect.ContentServiceGetCreatureProcedure, &rulesv1.GetCreatureRequest{CampaignId: w.campaign, Key: key})
			if rc.status != http.StatusNotFound {
				t.Errorf("%s: Caio's GetCreature of %s = HTTP %d %s, want not_found", state, key, rc.status, shorten(rc.body))
			}
		}
		for _, p := range []*person{ana, caio} {
			for _, proc := range []struct {
				name string
				call string
				req  proto.Message
			}{
				{"ListCreatures", rulesv1connect.ContentServiceListCreaturesProcedure, &rulesv1.ListCreaturesRequest{CampaignId: w.campaign, PageSize: 400}},
				{"ListTableEntries", rulesv1connect.TableContentServiceListTableEntriesProcedure, &rulesv1.ListTableEntriesRequest{CampaignId: w.campaign}},
				{"ListCharacterCreatures", charactersv1connect.CharacterServiceListCharacterCreaturesProcedure, &charactersv1.ListCharacterCreaturesRequest{CampaignId: w.campaign, CharacterId: w.pens.GetId()}},
			} {
				mods, blocks := damageLinesOf(read(p, proc.call, proc.req), keys...)
				heldKeys := 0
				for _, k := range keys {
					if holds[k] {
						heldKeys++
					}
				}
				switch {
				case p == caio && (mods != 0 || blocks != 0):
					t.Errorf("%s: Caio reads %d damage_mods and %d stat-block lines in %s, want none", state, mods, blocks, proc.name)
				case p == ana && proc.name == "ListCharacterCreatures" && (mods != 0 || blocks != 0):
					t.Errorf("%s: Ana's roster of creatures carries %d damage_mods and %d lines, want none (it is a roster; the stat block is GetCreature's)", state, mods, blocks)
				case p == ana && proc.name == "ListTableEntries" && mods != 2*heldKeys:
					// The server gives a holder the whole body of the entry she holds (forPlayer does not strip a creature's damage_mods).
					t.Errorf("%s: Ana's ListTableEntries carries %d damage_mods, want %d (two per entry she holds, and none of the entry nobody holds)", state, mods, 2*heldKeys)
				case p == ana && proc.name == "ListCreatures" && (mods != 0 || blocks != 0):
					t.Errorf("%s: the list of creatures carries %d damage_mods and %d lines, want none (rows only)", state, mods, blocks)
				}
			}
		}
		t.Run(state+" canaries are reachable", func(t *testing.T) { checkCanariesAreReachable(t, w, got) })
	}
	held := map[string]bool{archived: true, off: true, unheld: false}

	t.Run("live", func(t *testing.T) { check(t, "live", held) })

	must(m.table.ArchiveTableEntry(ctx, rq(&rulesv1.ArchiveTableEntryRequest{CampaignId: w.campaign, Key: archived})))
	t.Run("archived", func(t *testing.T) { check(t, "archived", held) })

	must(m.table.SetOptionSwitches(ctx, rq(&rulesv1.SetOptionSwitchesRequest{CampaignId: w.campaign, Switches: []*rulesv1.OptionSwitch{{Key: off, Off: true}}})))
	t.Run("off", func(t *testing.T) { check(t, "off", held) })
}
