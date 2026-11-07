package leaktest

import (
	"slices"
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

// What a player reads of a table creature that a spell or a feature summoned (#578). The summoner's wizard casts a table
// spell and uses a trait of her table race; each brings a table creature with numbers of its own. Everyone in the
// combat reads the creature's name and key, as for any combatant; the numbers, the sheet and the entry are the
// summoner's and the master's. Caio holds none of the creatures. A hidden NPC stands next to the summoner.

const (
	spellCreatureHP, spellCreatureAC     = 6161, 37
	featureCreatureHP, featureCreatureAC = 6363, 35
)

type summonView struct {
	w                        *world
	t                        *testing.T
	spell                    string // the table spell's key
	grantedBy, featureSpell  string
	spellCreature, featureCr string
	unheld, partialSpell     string // a creature nobody holds; a spell that lists the held one, the unheld one and an SRD one
	caster                   string // the summoner's combatant
	hiddenName, hiddenChar   string
	hiddenCombatant          string
	watchers                 map[*person]*streamWatcher
}

func newSummonView(t *testing.T) *summonView {
	t.Helper()
	w := &world{stack: newStack(t), secrets: newSecrets(), pts: map[string]*mapsv1.MapPoint{}, puzzles: map[string]*playv1.Puzzle{}, notes: map[string]string{}}
	v := &summonView{w: w, t: t}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("building the fixture: %v", r)
		}
	}()
	ctx := t.Context()
	w.buildPeople()
	m := w.master
	w.imgMap = w.image("summon-map-image", 240, 160)
	w.fogMap = must(m.maps.CreateMap(ctx, rq(&mapsv1.CreateMapRequest{CampaignId: w.campaign, Name: w.secrets.public("summon-map-name"), ImageId: w.imgMap}))).GetMap().GetId()
	must(m.maps.SetMapRevealed(ctx, rq(&mapsv1.SetMapRevealedRequest{CampaignId: w.campaign, MapId: w.fogMap, Revealed: true})))
	must(m.maps.SetMapGrid(ctx, rq(&mapsv1.SetMapGridRequest{CampaignId: w.campaign, MapId: w.fogMap, Columns: 24})))
	w.session = must(m.play.StartGameSession(ctx, rq(&playv1.StartGameSessionRequest{CampaignId: w.campaign}))).GetGameSession().GetId()
	must(m.play.SetCurrentMap(ctx, rq(&playv1.SetCurrentMapRequest{CampaignId: w.campaign, MapId: w.fogMap})))

	creature := func(kind string, hp, ac int32) string {
		e := must(m.table.CreateTableEntry(ctx, rq(&rulesv1.CreateTableEntryRequest{
			CampaignId: w.campaign, Body: &rulesv1.CreateTableEntryRequest_TableCreature{TableCreature: &rulesv1.TableCreature{
				NamePt: w.secrets.marker(kind + "-name"), BaseKey: "monster:giant-crab", HitPoints: hp, ArmorClass: ac,
				Traits: []*rulesv1.TableCreatureTrait{{NamePt: w.secrets.marker(kind+"-trait-name", w.ana), TextPt: w.secrets.marker(kind+"-trait-text", w.ana)}},
			}},
		}))).GetEntry()
		w.secrets.number(kind+"-hp", int64(hp))
		w.secrets.number(kind+"-ac", int64(ac), "armor_class")
		w.secrets.allowNeedle(itoa(int64(hp)), w.ana)
		w.secrets.allowNeedle(itoa(int64(ac)), w.ana)
		return e.GetKey()
	}
	v.spellCreature = creature("spell-creature", spellCreatureHP, spellCreatureAC)
	v.featureCr = creature("feature-creature", featureCreatureHP, featureCreatureAC)
	v.unheld = creature("unheld-creature", 4141, 33)
	spellBody := func(name string, keys ...string) *rulesv1.CreateTableEntryRequest {
		return &rulesv1.CreateTableEntryRequest{CampaignId: w.campaign, Body: &rulesv1.CreateTableEntryRequest_TableSpell{TableSpell: &rulesv1.TableSpell{
			NamePt: name, Level: 1, SchoolKey: "school:conjuration",
			CastingTime: &rulesv1.TableSpellCastingTime{Unit: rulesv1.CastingTimeUnit_CASTING_TIME_UNIT_ACTION, Amount: 1},
			Range:       &rulesv1.TableSpellRange{Kind: rulesv1.SpellRangeKind_SPELL_RANGE_KIND_SELF},
			Duration:    &rulesv1.TableSpellDuration{Kind: rulesv1.SpellDurationKind_SPELL_DURATION_KIND_INSTANTANEOUS},
			Components:  &rulesv1.TableSpellComponents{Verbal: true, Somatic: true},
			ClassKeys:   []string{"class:wizard"}, DescPt: []string{w.secrets.public(name + "-text")},
			Target: &rulesv1.TableSpellTarget{Kind: rulesv1.TableSpellTargetKind_TABLE_SPELL_TARGET_KIND_SELF},
			Summon: &rulesv1.TableSummon{
				CreatureKeys: keys, Count: "2", MaxLive: 4,
				TurnMode: rulesv1.TableSummonTurnMode_TABLE_SUMMON_TURN_MODE_OWN_INITIATIVE, Replaces: rulesv1.TableSummonReplaces_TABLE_SUMMON_REPLACES_SAME_SUMMON,
			},
		}}}
	}
	v.spell = must(m.table.CreateTableEntry(ctx, rq(spellBody(w.secrets.public("summon-spell-name"), v.spellCreature)))).GetEntry().GetKey()
	// A spell whose list mixes the creature Ana holds once she casts, one nobody holds and an SRD creature.
	v.partialSpell = must(m.table.CreateTableEntry(ctx, rq(spellBody(w.secrets.public("summon-mixed-spell-name"), v.spellCreature, v.unheld, "monster:wolf")))).GetEntry().GetKey()
	race := must(m.table.CreateTableEntry(ctx, rq(&rulesv1.CreateTableEntryRequest{
		CampaignId: w.campaign, Body: &rulesv1.CreateTableEntryRequest_TableRace{TableRace: &rulesv1.TableRace{
			NamePt: w.secrets.public("summon-race-name"), Size: "Medium", SpeedFt: 30, Languages: []string{"language:common"},
			Traits: []*rulesv1.TableFeature{{
				NamePt: w.secrets.public("summon-trait-name"), DescPt: []string{w.secrets.public("summon-trait-text")},
				Effects: []*rulesv1.TableEffect{{Type: "resource", Resource: "chamados", Max: "3", Recharge: "long_rest"}},
				Summon: &rulesv1.TableSummon{
					CreatureKeys: []string{v.featureCr}, Count: "2", MaxLive: 4, ResourceKey: "chamados",
					TurnMode: rulesv1.TableSummonTurnMode_TABLE_SUMMON_TURN_MODE_OWN_INITIATIVE, Replaces: rulesv1.TableSummonReplaces_TABLE_SUMMON_REPLACES_SAME_SUMMON,
				},
			}},
		}},
	}))).GetEntry()
	known := []string{v.spell}
	pens := must(w.ana.characters.CreateCharacter(ctx, rq(&charactersv1.CreateCharacterRequest{
		CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Pensantus",
		Sheet: &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
			BaseScores: &rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 8},
			RaceKey:    race.GetKey(), Classes: []*charactersv1.ClassLevel{{ClassKey: "class:wizard", Level: 5}},
			KnownSpellKeys: known, PreparedSpellKeys: known,
		}}},
	}))).GetCharacter()
	w.pens = pens
	w.toren = w.pc(w.caio, "Toren", "race:human")
	for _, s := range must(w.ana.characters.GetSummonOptions(ctx, rq(&charactersv1.GetSummonOptionsRequest{CampaignId: w.campaign, CharacterId: pens.GetId()}))).GetSpells() {
		if s.GetGrantedBy() != "" {
			v.grantedBy, v.featureSpell = s.GetGrantedBy(), s.GetSpellKey()
		}
	}
	if v.featureSpell == "" {
		t.Fatal("the race's trait gives no summon")
	}

	hidden, hiddenName := w.npc("summon-hidden", &charactersv1.BasicSheet{HitPointsMax: 40, ArmorClass: 12, SpeedFt: 30})
	seen, _ := w.npc("summon-seen", &charactersv1.BasicSheet{HitPointsMax: 40, ArmorClass: 12, SpeedFt: 30})
	v.hiddenName, v.hiddenChar = hiddenName, hidden.GetId()
	w.allow("summon-seen-name", w.ana, w.caio)
	w.secrets.id("summon-hidden-npc", hidden.GetId())
	revealed := false
	e := must(m.combat.StartEncounter(ctx, rq(&playv1.StartEncounterRequest{
		CampaignId: w.campaign, IdempotencyKey: newKey(), Name: w.secrets.public("summon-encounter"),
		Participants: []*playv1.Participant{{CharacterId: seen.GetId(), Hidden: &revealed}, {CharacterId: hidden.GetId()}},
	}))).GetEncounter()
	for _, c := range e.GetCombatants() {
		switch c.GetCharacterId() {
		case hidden.GetId():
			v.hiddenCombatant = c.GetId()
		case pens.GetId():
			v.caster = c.GetId()
		}
		if c.GetKind() == playv1.CombatantKind_COMBATANT_KIND_PLAYER {
			must(m.combat.SubmitInitiative(ctx, rq(&playv1.SubmitInitiativeRequest{
				CampaignId: w.campaign, EncounterId: e.GetId(), CombatantId: c.GetId(), IdempotencyKey: newKey(),
				Roll: &playv1.SubmitInitiativeRequest_D20Face{D20Face: 12},
			})))
		}
	}
	w.secrets.id("summon-hidden-combatant", v.hiddenCombatant)
	for char, sq := range map[string][2]int32{pens.GetId(): {3, 5}, w.toren.GetId(): {2, 6}, seen.GetId(): {10, 5}, hidden.GetId(): {4, 5}} {
		for _, c := range e.GetCombatants() {
			if c.GetCharacterId() == char {
				must(m.combat.MoveCombatant(ctx, rq(&playv1.MoveCombatantRequest{CampaignId: w.campaign, EncounterId: e.GetId(), CombatantId: c.GetId(), IdempotencyKey: newKey(), Col: sq[0], Row: sq[1]})))
			}
		}
	}
	w.encounter = must(m.combat.BeginCombat(ctx, rq(&playv1.BeginCombatRequest{CampaignId: w.campaign, EncounterId: e.GetId(), IdempotencyKey: newKey()}))).GetEncounter()
	v.toCasterTurn()
	v.watchers = map[*person]*streamWatcher{m: w.watchStream(m), w.ana: w.watchStream(w.ana), w.caio: w.watchStream(w.caio)}
	return v
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// toNextTurn ends the summoner's turn and the others' until hers comes again.
func (v *summonView) toNextTurn() {
	v.t.Helper()
	w, ctx := v.w, v.t.Context()
	cur := must(w.master.combat.GetEncounter(ctx, rq(&playv1.GetEncounterRequest{CampaignId: w.campaign}))).GetEncounter()
	must(w.master.combat.EndTurn(ctx, rq(&playv1.EndTurnRequest{CampaignId: w.campaign, EncounterId: cur.GetId(), IdempotencyKey: newKey(), ExpectedCombatantId: cur.GetCurrentCombatantId(), ExpectedRound: cur.GetRound()})))
	v.toCasterTurn()
}

// toCasterTurn ends turns until it is the summoner's.
func (v *summonView) toCasterTurn() {
	v.t.Helper()
	w, ctx := v.w, v.t.Context()
	for range 20 {
		cur := must(w.master.combat.GetEncounter(ctx, rq(&playv1.GetEncounterRequest{CampaignId: w.campaign}))).GetEncounter()
		w.encounter = cur
		if cur.GetCurrentCombatantId() == v.caster {
			return
		}
		must(w.master.combat.EndTurn(ctx, rq(&playv1.EndTurnRequest{CampaignId: w.campaign, EncounterId: cur.GetId(), IdempotencyKey: newKey(), ExpectedCombatantId: cur.GetCurrentCombatantId(), ExpectedRound: cur.GetRound()})))
	}
	v.t.Fatal("the summoner's turn never came")
}

// cast has Ana cast a spell of her own (grantedBy empty, with a slot) or the race's trait (granted_by, no slot) in the combat,
// for the creatures named.
func (v *summonView) cast(spell, grantedBy string, slot *playv1.SpellSlot, keys ...string) *playv1.CastSpellResponse {
	v.t.Helper()
	return must(v.w.ana.combat.CastSpell(v.t.Context(), rq(&playv1.CastSpellRequest{
		CampaignId: v.w.campaign, EncounterId: v.w.encounter.GetId(), CasterId: v.caster, SpellKey: spell, GrantedBy: grantedBy, Slot: slot, IdempotencyKey: newKey(),
		Roll: &playv1.CastSpellRequest_RollInApp{RollInApp: true}, Summon: &playv1.SummonChoice{CreatureKeys: keys},
	})))
}

// summoned lists the creature combatants of the combat for a monster key.
func (v *summonView) summoned(key string) []*playv1.Combatant {
	var out []*playv1.Combatant
	for _, c := range must(v.w.master.combat.GetEncounter(v.t.Context(), rq(&playv1.GetEncounterRequest{CampaignId: v.w.campaign}))).GetEncounter().GetCombatants() {
		if c.GetKind() == playv1.CombatantKind_COMBATANT_KIND_CREATURE && c.GetMonsterKey() == key {
			out = append(out, c)
		}
	}
	return out
}

func (v *summonView) entries(p *person) string {
	return string(p.call(rulesv1connect.TableContentServiceListTableEntriesProcedure, &rulesv1.ListTableEntriesRequest{CampaignId: v.w.campaign}).body)
}

// combatRows are the reads of the combat and of the summoned creature, with the people each is for.
func (v *summonView) combatRows(creature *playv1.Combatant, keys ...string) []read {
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
			procedure: charactersv1connect.CharacterServiceListCharacterCreaturesProcedure, allow: onlyAna,
			req: func(w *world) proto.Message {
				return &charactersv1.ListCharacterCreaturesRequest{CampaignId: w.campaign, CharacterId: w.pens.GetId()}
			},
		},
	}
	if creature != nil {
		rows = append(rows, read{
			procedure: playv1connect.CombatServiceGetTurnOptionsProcedure, label: "the summoned creature", allow: onlyAna, why: "the creature's options are its owner's and the master's",
			req: func(w *world) proto.Message {
				return &playv1.GetTurnOptionsRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), CombatantId: creature.GetId()}
			},
		})
	}
	for _, k := range keys {
		rows = append(rows, read{
			procedure: rulesv1connect.ContentServiceGetCreatureProcedure, label: "the summoned creature " + k, allow: onlyAna, why: "a table creature is not found by a player who holds none",
			req: func(w *world) proto.Message { return &rulesv1.GetCreatureRequest{CampaignId: w.campaign, Key: k} },
		})
	}
	return rows
}

// Everyone in the combat reads who the summoned creatures are, their name and key; the numbers, the sheet and the entry
// are not Caio's. This holds for a spell's creature and for a feature's, in the combat, its log, the answer to the cast
// and the stream, and the master and the summoner read the numbers (the positive controls). The hidden NPC next to
// the summoner stays unnamed to both players, however near the creatures are placed.
func TestASummonedTableCreatureShowsAThirdPlayerItsIdentityAndNothingElse(t *testing.T) {
	v := newSummonView(t)
	w := v.w
	ctx := t.Context()
	got := newAnswers()

	// The spell, with a slot, then the feature, on the summoner's next turn.
	spellCast := v.cast(v.spell, "", &playv1.SpellSlot{Level: 1}, v.spellCreature, v.spellCreature)
	first := v.summoned(v.spellCreature)
	if len(first) != 2 {
		t.Fatalf("the spell brought %d creatures, want 2", len(first))
	}
	w.encounter = must(w.master.combat.GetEncounter(ctx, rq(&playv1.GetEncounterRequest{CampaignId: w.campaign}))).GetEncounter()
	v.toNextTurn()
	featureCast := v.cast(v.featureSpell, v.grantedBy, nil, v.featureCr, v.featureCr)
	second := v.summoned(v.featureCr)
	if len(second) != 2 {
		t.Fatalf("the feature brought %d creatures, want 2", len(second))
	}
	// Their name and key are the combat's from now on; the other markers stay out of Caio's reach.
	for _, c := range append(first, second...) {
		w.secrets.id("summoned-combatant", c.GetId(), w.ana, w.caio)
	}
	for _, kind := range []string{"spell-creature-name", "feature-creature-name"} {
		w.allow(kind, w.ana, w.caio)
	}
	w.secrets.id("spell-creature-key", v.spellCreature, w.ana, w.caio)
	w.secrets.id("feature-creature-key", v.featureCr, w.ana, w.caio)

	for _, name := range []string{"the spell's creatures", "the feature's creatures"} {
		creature := first[0]
		keys := []string{v.spellCreature}
		if strings.Contains(name, "feature") {
			creature, keys = second[0], []string{v.featureCr}
		}
		for _, r := range v.combatRows(creature, keys...) {
			t.Run(name+": "+r.name(), func(t *testing.T) { w.runRead(t, r, got) })
		}
	}
	for what, resp := range map[string]*playv1.CastSpellResponse{"the spell's cast": spellCast, "the feature's cast": featureCast} {
		body := []byte(jsonOf(resp))
		// The answer goes to the caster.
		for _, f := range w.inspect(w.ana, reply{status: 200, body: body, msg: resp}, nil) {
			t.Errorf("Ana: the answer to %s: %s", what, f)
		}
	}

	// Caio reads the creatures' identity and none of their numbers; the master and Ana read them.
	enc := must(w.caio.combat.GetEncounter(ctx, rq(&playv1.GetEncounterRequest{CampaignId: w.campaign}))).GetEncounter()
	seen := 0
	for _, c := range enc.GetCombatants() {
		if c.GetKind() != playv1.CombatantKind_COMBATANT_KIND_CREATURE {
			continue
		}
		seen++
		if (c.GetMonsterKey() != v.spellCreature && c.GetMonsterKey() != v.featureCr) || !strings.HasPrefix(c.GetMonsterNamePt(), markerPrefix) {
			t.Errorf("Caio reads a creature as %v, want its table key and name", c)
		}
		if c.HitPointsCurrent != nil || c.HitPointsMax != nil || c.ArmorClass != nil {
			t.Errorf("Caio reads the creature's numbers: %v", c)
		}
	}
	if seen != 4 {
		t.Errorf("Caio sees %d summoned creatures, want 4", seen)
	}
	masterCombat := w.master.call(playv1connect.CombatServiceGetEncounterProcedure, &playv1.GetEncounterRequest{CampaignId: w.campaign})
	mine := w.ana.call(charactersv1connect.CharacterServiceListCharacterCreaturesProcedure, &charactersv1.ListCharacterCreaturesRequest{CampaignId: w.campaign, CharacterId: w.pens.GetId()})
	for _, kind := range []string{"spell-creature-hp", "spell-creature-ac", "feature-creature-hp", "feature-creature-ac"} {
		if !holdsNumber(masterCombat, w.canaryOf(kind)) {
			t.Errorf("the master's combat does not hold %s (the positive control)", kind)
		}
		if !holdsNumber(mine, w.canaryOf(kind)) {
			t.Errorf("Ana's list of creatures does not hold %s (the positive control)", kind)
		}
	}

	// The hidden NPC stood next to the summoner all along: the summoner's view does not name it.
	for _, p := range []*person{w.ana, w.caio} {
		for _, r := range v.combatRows(nil) {
			rep := p.call(r.procedure, r.request(w, p))
			if rep.ok() && (strings.Contains(string(rep.body), v.hiddenName) || strings.Contains(string(rep.body), v.hiddenCombatant) || strings.Contains(string(rep.body), v.hiddenChar)) {
				t.Errorf("%s reads the hidden NPC in %s: %s", p.name, r.name(), shorten(rep.body))
			}
		}
	}
	if !strings.Contains(string(masterCombat.body), v.hiddenCombatant) {
		t.Error("the master's combat does not name the hidden NPC (the positive control)")
	}

	// The stream: what each player heard while the creatures were made names no number and no hidden NPC.
	w.settle(v.watchers[w.master], v.watchers[w.ana], v.watchers[w.caio])
	for p, sw := range v.watchers {
		heard := sw.since(0)
		heardReal := 0
		for _, ev := range heard {
			if ev.GetCurrentMapChanged() == nil && ev.GetHeartbeat() == nil {
				heardReal++
			}
		}
		if heardReal == 0 && (p == w.master || p == w.ana) {
			t.Errorf("%s's stream heard no event besides the marker and the heartbeat: the check proves nothing", p.name)
		}
		for _, ev := range heard {
			body := []byte(jsonOf(ev))
			r := reply{status: 200, body: body, msg: ev}
			got.keep(p, r)
			if p == w.master {
				continue
			}
			for _, f := range w.inspect(p, r, nil) {
				t.Errorf("%s: stream event %s: %s", p.name, eventCase(ev), f)
			}
		}
	}
	// The controls of what the reads above were scanning for: the master and Ana read the creatures' own text.
	for _, kind := range []string{"spell-creature-trait-text", "feature-creature-trait-text", "spell-creature-hp", "feature-creature-hp"} {
		for _, p := range []*person{w.master, w.ana} {
			if !got.holds(p, canaryByKind(w, kind)) {
				t.Errorf("%s reads %s in no answer: the control that the scan can find it is missing", p.name, kind)
			}
		}
	}
}

// A summoned creature and the entries: Ana reads the creature in the table's entries while a character of hers holds
// one live, and not once it was dismissed or undone; replaced by a new casting she holds the new ones. Caio never reads
// it. This holds for a spell's creature and for a feature's. A spell that lists a creature she holds, one nobody holds and
// an SRD creature shows her the held one and the SRD one. Her options name the creatures she may pick all along, because
// she picks from them.
func TestASummonedTableCreatureLeavesTheEntriesWhenNoneIsHeld(t *testing.T) {
	v := newSummonView(t)
	w := v.w
	ctx := t.Context()
	has := func(p *person, key string) bool { return strings.Contains(v.entries(p), key) }
	opts := func() string {
		return string(w.ana.call(charactersv1connect.CharacterServiceGetSummonOptionsProcedure, &charactersv1.GetSummonOptionsRequest{CampaignId: w.campaign, CharacterId: w.pens.GetId()}).body)
	}
	live := func() int {
		return len(must(w.ana.characters.ListCharacterCreatures(ctx, rq(&charactersv1.ListCharacterCreaturesRequest{CampaignId: w.campaign, CharacterId: w.pens.GetId()}))).GetCreatures())
	}
	undo := func() {
		log := must(w.master.combat.ListCombatLog(ctx, rq(&playv1.ListCombatLogRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId()})))
		must(w.master.combat.UndoLastAction(ctx, rq(&playv1.UndoLastActionRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), ExpectedEventId: log.GetUndoableEventId(), IdempotencyKey: newKey()})))
	}
	for _, kind := range []struct {
		name, key string
		cast      func()
	}{
		{"the spell's creature", v.spellCreature, func() { v.cast(v.spell, "", &playv1.SpellSlot{Level: 1}, v.spellCreature, v.spellCreature) }},
		{"the feature's creature", v.featureCr, func() { v.cast(v.featureSpell, v.grantedBy, nil, v.featureCr, v.featureCr) }},
	} {
		key := kind.key
		if has(w.ana, key) || has(w.caio, key) || !has(w.master, key) {
			t.Fatalf("%s before a cast: Ana %v, Caio %v, the master %v read it in the entries; want false, false, true", kind.name, has(w.ana, key), has(w.caio, key), has(w.master, key))
		}
		if !strings.Contains(opts(), key) {
			t.Errorf("%s: Ana's options do not name the creature she may pick (the positive control)", kind.name)
		}

		// Held: the creature is in Ana's entries and in the mixed spell's list, beside the SRD wolf, and the unheld one is not.
		kind.cast()
		if !has(w.ana, key) || has(w.caio, key) {
			t.Errorf("%s held: Ana reads it %v, Caio %v; want true, false", kind.name, has(w.ana, key), has(w.caio, key))
		}
		if kind.key == v.spellCreature {
			if got, want := v.summonKeysOf(w.ana, v.partialSpell), []string{v.spellCreature, "monster:wolf"}; !slices.Equal(got, want) {
				t.Errorf("the mixed spell in Ana's entries lists %v, want the held creature and the SRD's wolf %v", got, want)
			}
			if got := v.summonKeysOf(w.caio, v.partialSpell); !slices.Equal(got, []string{"monster:wolf"}) {
				t.Errorf("the mixed spell in Caio's entries lists %v, want the SRD's wolf only", got)
			}
			if got := v.summonKeysOf(w.master, v.partialSpell); len(got) != 3 {
				t.Errorf("the mixed spell in the master's entries lists %v, want the three creatures (the positive control)", got)
			}
		}

		// Replaced: the second casting dismisses the first creatures and makes new ones, which Ana holds.
		v.toNextTurn()
		kind.cast()
		if !has(w.ana, key) || live() != 2 {
			t.Errorf("%s replaced: Ana reads it %v with %d creatures live; want true and 2 (the new ones only)", kind.name, has(w.ana, key), live())
		}

		// Undone: the replacing cast is taken back and the first creatures return, so Ana holds them still.
		undo()
		if !has(w.ana, key) || live() != 2 {
			t.Errorf("%s after the undo of the second cast: Ana reads it %v with %d creatures live; want true and 2 (the first ones, the positive control)", kind.name, has(w.ana, key), live())
		}

		// Dismissed: the owner sends every creature away.
		for _, c := range must(w.ana.characters.ListCharacterCreatures(ctx, rq(&charactersv1.ListCharacterCreaturesRequest{CampaignId: w.campaign, CharacterId: w.pens.GetId()}))).GetCreatures() {
			must(w.ana.characters.DismissCreature(ctx, rq(&charactersv1.DismissCreatureRequest{CampaignId: w.campaign, CreatureId: c.GetId()})))
		}
		if has(w.ana, key) || has(w.caio, key) {
			t.Errorf("%s after the dismissal: Ana reads it %v, Caio %v; want false, false", kind.name, has(w.ana, key), has(w.caio, key))
		}

		// Undone, with nothing else held: a new cast and its undo leave no creature behind.
		kind.cast()
		if !has(w.ana, key) {
			t.Fatalf("%s after a new cast: Ana does not read it", kind.name)
		}
		undo()
		if has(w.ana, key) || has(w.caio, key) {
			t.Errorf("%s after the undo of the only cast: Ana reads it %v, Caio %v; want false, false", kind.name, has(w.ana, key), has(w.caio, key))
		}
		if !strings.Contains(opts(), key) {
			t.Errorf("%s: Ana's options no longer name the creature she may pick", kind.name)
		}
		if !has(w.master, key) {
			t.Errorf("%s: the master's entries lost the creature", kind.name)
		}
		v.toNextTurn()
	}
	if !has(w.master, v.unheld) || has(w.ana, v.unheld) || has(w.caio, v.unheld) {
		t.Errorf("a creature nobody holds: the master reads it %v, Ana %v, Caio %v; want true, false, false", has(w.master, v.unheld), has(w.ana, v.unheld), has(w.caio, v.unheld))
	}
}

// summonKeysOf are the creature keys the summon of a spell lists in the entries p reads.
func (v *summonView) summonKeysOf(p *person, spell string) []string {
	v.t.Helper()
	for _, e := range must(p.table.ListTableEntries(v.t.Context(), rq(&rulesv1.ListTableEntriesRequest{CampaignId: v.w.campaign}))).GetEntries() {
		if e.GetKey() == spell {
			return e.GetTableSpell().GetSummon().GetCreatureKeys()
		}
	}
	v.t.Fatalf("%s does not read the spell %s in the entries", p.name, spell)
	return nil
}

// canaryByKind is the first canary of the kind, a number or not.
func canaryByKind(w *world, kind string) *canary {
	w.t.Helper()
	for _, c := range w.secrets.list {
		if c.kind == kind {
			return c
		}
	}
	w.t.Fatalf("no canary of the kind %q is registered", kind)
	return nil
}

// An archived creature and a switched-off one that a spell lists are not offered to the player as a new choice: GetSummonOptions
// names neither their key nor their name, while the live creature and the SRD's wolf stay (the holder's positive control) and the
// master, who sees everything, still reads both in the table's entries (the master's positive control).
func TestAnArchivedOrSwitchedOffCreatureOfASpellIsNotOfferedToThePlayer(t *testing.T) {
	v := newSummonView(t)
	w := v.w
	ctx := t.Context()
	m := w.master
	creature := func(kind string) (key, name string) {
		name = w.secrets.marker(kind + "-name")
		e := must(m.table.CreateTableEntry(ctx, rq(&rulesv1.CreateTableEntryRequest{
			CampaignId: w.campaign, Body: &rulesv1.CreateTableEntryRequest_TableCreature{TableCreature: &rulesv1.TableCreature{
				NamePt: name, BaseKey: "monster:giant-crab", HitPoints: 20, ArmorClass: 12,
			}},
		}))).GetEntry()
		return e.GetKey(), name
	}
	live, liveName := creature("offer-live")
	archived, archivedName := creature("offer-archived")
	off, offName := creature("offer-off")
	// The summoner's own spell lists the three creatures and the SRD's wolf from now on.
	var listing *rulesv1.TableEntry
	for _, e := range must(m.table.ListTableEntries(ctx, rq(&rulesv1.ListTableEntriesRequest{CampaignId: w.campaign}))).GetEntries() {
		if e.GetKey() == v.spell {
			listing = e
		}
	}
	body := proto.Clone(listing.GetTableSpell()).(*rulesv1.TableSpell)
	body.Summon.CreatureKeys = []string{live, archived, off, "monster:wolf"}
	must(m.table.UpdateTableEntry(ctx, rq(&rulesv1.UpdateTableEntryRequest{
		CampaignId: w.campaign, Key: v.spell, ExpectedRevision: listing.GetRevision(),
		Body: &rulesv1.UpdateTableEntryRequest_TableSpell{TableSpell: body},
	})))
	mage := w.pens
	options := func(p *person) string {
		return string(p.call(charactersv1connect.CharacterServiceGetSummonOptionsProcedure, &charactersv1.GetSummonOptionsRequest{CampaignId: w.campaign, CharacterId: mage.GetId()}).body)
	}
	namesAll := func(body string, keys, names []string) bool {
		for _, s := range append(slices.Clone(keys), names...) {
			if !strings.Contains(body, s) {
				return false
			}
		}
		return true
	}

	// Live: she is offered all of them, which proves the read can name the creatures at all.
	if all := options(w.ana); !namesAll(all, []string{live, archived, off}, []string{liveName, archivedName, offName}) {
		t.Fatalf("Ana's options do not offer the live creatures (the positive control): %s", shorten([]byte(all)))
	}

	must(m.table.ArchiveTableEntry(ctx, rq(&rulesv1.ArchiveTableEntryRequest{CampaignId: w.campaign, Key: archived})))
	must(m.table.SetOptionSwitches(ctx, rq(&rulesv1.SetOptionSwitchesRequest{CampaignId: w.campaign, Switches: []*rulesv1.OptionSwitch{{Key: off, Off: true}}})))

	got := options(w.ana)
	for _, hidden := range []string{archived, archivedName, off, offName} {
		if strings.Contains(got, hidden) {
			t.Errorf("Ana's options still name %q, a creature the master archived or switched off: %s", hidden, shorten([]byte(got)))
		}
	}
	if !namesAll(got, []string{live, "monster:wolf"}, []string{liveName}) {
		t.Errorf("Ana's options lost the live creature or the SRD's wolf (the holder's positive control): %s", shorten([]byte(got)))
	}
	// The master reads both creatures in the table's entries, archived or off.
	entries := v.entries(m)
	for _, key := range []string{archived, off} {
		if !strings.Contains(entries, key) {
			t.Errorf("the master's entries lost %s (the positive control)", key)
		}
	}
	// Nobody else reads them in the entries either: Ana holds none of the three.
	for _, key := range []string{archived, off} {
		if strings.Contains(v.entries(w.ana), key) || strings.Contains(v.entries(w.caio), key) {
			t.Errorf("a player reads %s in the table's entries, a creature nobody of theirs holds", key)
		}
	}
}
