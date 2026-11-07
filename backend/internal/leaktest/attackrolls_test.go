package leaktest

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1/playv1connect"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// The attack rolls of a combat (RN-10, RN-20): a player's request for a better mode and the
// master's reasons for a mode or for taking an extra out of a damage are for the master and
// the attacker's player; an NPC's resistances, a creature's type, the states of a combatant the
// player does not see and the circumstances behind a mode that rest on what the player does not
// see never reach a player, though the dice, the totals and the mode itself do their work.
//
// The table is small and has a world of its own, like the area spell test: a paladin (Caio) and a
// barbarian (Ana) in the west of a fog map, a wall across it, and around them a skeleton, an
// awakened shrub (it resists piercing) and a bandit in plain sight, a wolf (Pack Tactics) and two
// NPCs the master keeps hidden. The same rows as the matrix read it (the combat, its log and the
// options of a turn), asked of everyone after each step, and the live stream is watched through
// all of it. The master's own answers are the positive controls of every marker (see
// checkCanariesAreReachable), and the checks that follow a step look at the structure of what
// each player got.

// rollsTable is the world and what the steps need to find in it.
type rollsTable struct {
	*world
	got *answers
	// ids are the combatants by label.
	ids map[string]string
	// rows are the reads asked of everyone at each step.
	rows []read
}

const (
	dagger   = "equipment:dagger"
	shortbow = "equipment:shortbow"

	// The labels of the table, other than the markers.
	labelToren, labelPens = "Toren", "Pensantus"
)

// Kinds of canary this table registers.
const (
	kindRequestReason = "roll-mode-request-reason"
	kindRemovalReason = "part-removal-reason"
	kindModeReason    = "mode-change-reason"
	kindNPCModeReason = "npc-mode-reason"
)

// newRollsTable builds the world and starts the combat: Caio's paladin plays first.
func newRollsTable(t *testing.T) *rollsTable {
	t.Helper()
	return newRollsTableWith(t, nil)
}

// newRollsTableWith is newRollsTable with a step between the heroes and the combat: what a test gives its heroes
// before the combat begins (a creature that joins it).
func newRollsTableWith(t *testing.T, afterHeroes func(rt *rollsTable)) *rollsTable {
	t.Helper()
	return newRollsTableFog(t, true, afterHeroes)
}

// newRollsTableFog is newRollsTableWith on a map with the fog of war on or off.
func newRollsTableFog(t *testing.T, fog bool, afterHeroes func(rt *rollsTable)) *rollsTable {
	t.Helper()
	return newRollsTableOf(t, false, fog, afterHeroes)
}

// newRollsTableOf is newRollsTableFog with the d20 rerolls switched on or off.
func newRollsTableOf(t *testing.T, d20Rerolls, fog bool, afterHeroes func(rt *rollsTable)) *rollsTable {
	t.Helper()
	w := &world{stack: newStackOf(t, d20Rerolls), secrets: newSecrets(), pts: map[string]*mapsv1.MapPoint{}, puzzles: map[string]*playv1.Puzzle{}, notes: map[string]string{}}
	rt := &rollsTable{world: w, got: newAnswers(), ids: map[string]string{}}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("building the fixture: %v", r)
		}
	}()
	ctx := t.Context()
	w.buildPeople()
	m := w.master

	// The map: a cave in two halves, the party in the west and the wall between them and the east.
	img := m.upload(w.campaign, "img.png", pngOf(240, 160, 3))
	w.fogMap = must(m.maps.CreateMap(ctx, rq(&mapsv1.CreateMapRequest{CampaignId: w.campaign, Name: "Estrada", ImageId: img}))).GetMap().GetId()
	must(m.maps.SetMapRevealed(ctx, rq(&mapsv1.SetMapRevealedRequest{CampaignId: w.campaign, MapId: w.fogMap, Revealed: true})))
	must(m.maps.SetMapGrid(ctx, rq(&mapsv1.SetMapGridRequest{CampaignId: w.campaign, MapId: w.fogMap, Columns: caveColumns})))
	var wall [][2]int32
	for row := range caveGrid.Rows {
		wall = append(wall, [2]int32{12, int32(row)})
	}
	w.paint(w.fogMap, mapsv1.MapLayer_MAP_LAYER_WALL, 1, wall)
	must(m.maps.SetMapFog(ctx, rq(&mapsv1.SetMapFogRequest{CampaignId: w.campaign, MapId: w.fogMap, FogEnabled: new(fog), BaseLight: mapsv1.LightLevel_LIGHT_LEVEL_DIM.Enum()})))
	w.session = must(m.play.StartGameSession(ctx, rq(&playv1.StartGameSessionRequest{CampaignId: w.campaign}))).GetGameSession().GetId()
	must(m.play.SetCurrentMap(ctx, rq(&playv1.SetCurrentMapRequest{CampaignId: w.campaign, MapId: w.fogMap})))

	scores := &rulesv1.AbilityScores{Strength: 16, Dexterity: 14, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 14}
	hero := func(owner *person, name string, classes []*charactersv1.ClassLevel, choices []string, weapons ...string) *charactersv1.Character {
		return must(owner.characters.CreateCharacter(ctx, rq(&charactersv1.CreateCharacterRequest{
			CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: name,
			Sheet: &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
				BaseScores: scores, RaceKey: "race:human", Classes: classes, WeaponKeys: weapons, FeatureChoiceKeys: choices,
			}}},
		}))).GetCharacter()
	}
	// A paladin who is also a rogue: Divine Smite, and Sneak Attack, the extra the master may take out.
	w.toren = hero(w.caio, labelToren, []*charactersv1.ClassLevel{{ClassKey: "class:paladin", Level: 2}, {ClassKey: "class:rogue", Level: 2}}, []string{"feature:fighting-style-dueling"}, dagger, shortbow)
	w.pens = hero(w.ana, labelPens, []*charactersv1.ClassLevel{{ClassKey: "class:barbarian", Level: 3}}, nil, dagger, shortbow)
	w.place(w.fogMap, w.toren.GetId(), 3, 5)
	w.place(w.fogMap, w.pens.GetId(), 10, 10)
	if afterHeroes != nil {
		afterHeroes(rt)
	}

	// The NPC that starts the combat is one of the two the master keeps hidden.
	hiddenA := w.secrets.marker("hid-EscondidoA")
	npcA := must(m.characters.CreateNpcFromCreature(ctx, rq(&charactersv1.CreateNpcFromCreatureRequest{
		CampaignId: w.campaign, CreatureKey: "monster:goblin", Name: hiddenA, Kind: charactersv1.CharacterKind_CHARACTER_KIND_MINION, IdempotencyKey: newKey(),
	}))).GetCharacter()
	w.secrets.add(&canary{needle: "monster:goblin", kind: "monster-key"})
	e := must(m.combat.StartEncounter(ctx, rq(&playv1.StartEncounterRequest{
		CampaignId: w.campaign, IdempotencyKey: newKey(), Name: "Emboscada", Participants: []*playv1.Participant{{CharacterId: npcA.GetId()}},
	}))).GetEncounter()
	for _, c := range e.GetCombatants() {
		rt.ids[c.GetLabel()] = c.GetId()
		switch {
		case c.GetCharacterId() == npcA.GetId():
			rt.ids["EscondidoA"] = c.GetId()
			w.secrets.id("rolls-hidden-EscondidoA", c.GetId())
		case c.GetKind() == playv1.CombatantKind_COMBATANT_KIND_CREATURE:
			// A creature of a hero rolls its group's initiative: 11, between Pensantus's and the wolf's.
			must(m.combat.SubmitInitiative(ctx, rq(&playv1.SubmitInitiativeRequest{
				CampaignId: w.campaign, EncounterId: e.GetId(), CombatantId: c.GetId(), IdempotencyKey: newKey(),
				Roll: &playv1.SubmitInitiativeRequest_D20Face{D20Face: 11},
			})))
		case c.GetKind() == playv1.CombatantKind_COMBATANT_KIND_PLAYER:
			face := int32(20)
			if c.GetCharacterId() == w.pens.GetId() {
				face = 10
			}
			must(m.combat.SubmitInitiative(ctx, rq(&playv1.SubmitInitiativeRequest{
				CampaignId: w.campaign, EncounterId: e.GetId(), CombatantId: c.GetId(), IdempotencyKey: newKey(),
				Roll: &playv1.SubmitInitiativeRequest_D20Face{D20Face: face},
			})))
		}
	}
	// The monsters: names that are markers, so a name that reaches a player is found. Those in
	// plain sight are public (the players read them), the others are the master's.
	add := func(label, key string, hidden bool, col, row int32) {
		var name string
		if hidden {
			name = w.secrets.marker("hid-" + label)
		} else {
			name = w.secrets.public("vis-" + label)
		}
		res := must(m.combat.AddMonsters(ctx, rq(&playv1.AddMonstersRequest{
			CampaignId: w.campaign, EncounterId: e.GetId(), IdempotencyKey: newKey(), CreatureKey: key, Count: 1, Name: name, Hidden: &hidden,
		})))
		id := res.GetCombatantIds()[0]
		must(m.combat.MoveCombatant(ctx, rq(&playv1.MoveCombatantRequest{CampaignId: w.campaign, EncounterId: e.GetId(), CombatantId: id, IdempotencyKey: newKey(), Col: col, Row: row, Forced: true})))
		rt.ids[label] = id
		if hidden {
			w.secrets.id("rolls-hidden-"+label, id)
		}
		w.secrets.add(&canary{needle: key, kind: "monster-key"})
	}
	add("Esqueleto", "monster:skeleton", false, 4, 5)
	add("Arbusto", "monster:awakened-shrub", false, 3, 4)
	add("Bandido", "monster:bandit", false, 2, 5)
	add("Lobo", "monster:wolf", false, 4, 6)
	add("EscondidoB", "monster:goblin", true, 10, 11)
	for label, sq := range map[string][2]int32{labelToren: {3, 5}, labelPens: {10, 10}, "EscondidoA": {2, 6}} {
		must(m.combat.MoveCombatant(ctx, rq(&playv1.MoveCombatantRequest{CampaignId: w.campaign, EncounterId: e.GetId(), CombatantId: rt.ids[label], IdempotencyKey: newKey(), Col: sq[0], Row: sq[1], Forced: true})))
	}
	// The order is the table's: Toren, the skeleton, the bandit, the two the master hides, Pensantus, the wolf, the shrub.
	for label, face := range map[string]int32{"Esqueleto": 19, "Bandido": 15, "EscondidoA": 13, "EscondidoB": 12, "Lobo": 9, "Arbusto": 7} {
		must(m.combat.SubmitInitiative(ctx, rq(&playv1.SubmitInitiativeRequest{
			CampaignId: w.campaign, EncounterId: e.GetId(), CombatantId: rt.ids[label], IdempotencyKey: newKey(),
			Roll: &playv1.SubmitInitiativeRequest_D20Face{D20Face: face},
		})))
	}
	w.encounter = must(m.combat.BeginCombat(ctx, rq(&playv1.BeginCombatRequest{CampaignId: w.campaign, EncounterId: e.GetId(), IdempotencyKey: newKey()}))).GetEncounter()
	return rt
}

// ---- the calls of the table ----

func (rt *rollsTable) encounterOf(p *person) *playv1.Encounter {
	return must(p.combat.GetEncounter(rt.t.Context(), rq(&playv1.GetEncounterRequest{CampaignId: rt.campaign}))).GetEncounter()
}

func (rt *rollsTable) logOf(p *person) *playv1.ListCombatLogResponse {
	return must(p.combat.ListCombatLog(rt.t.Context(), rq(&playv1.ListCombatLogRequest{CampaignId: rt.campaign, EncounterId: rt.encounter.GetId()})))
}

func (rt *rollsTable) optionsOf(p *person, label string) *playv1.GetTurnOptionsResponse {
	return must(p.combat.GetTurnOptions(rt.t.Context(), rq(&playv1.GetTurnOptionsRequest{CampaignId: rt.campaign, EncounterId: rt.encounter.GetId(), CombatantId: rt.ids[label]})))
}

// combatantOf is the combatant as p reads it.
func (rt *rollsTable) combatantOf(p *person, label string) *playv1.Combatant {
	for _, c := range rt.encounterOf(p).GetCombatants() {
		if c.GetId() == rt.ids[label] {
			return c
		}
	}
	return nil
}

// toTurn passes the turns on, as the master, until the combatant is on turn.
func (rt *rollsTable) toTurn(label string) {
	for range 40 {
		cur := rt.encounterOf(rt.master)
		if cur.GetCurrentCombatantId() == rt.ids[label] {
			return
		}
		must(rt.master.combat.EndTurn(rt.t.Context(), rq(&playv1.EndTurnRequest{
			CampaignId: rt.campaign, EncounterId: cur.GetId(), IdempotencyKey: newKey(), ExpectedCombatantId: cur.GetCurrentCombatantId(), ExpectedRound: cur.GetRound(), DiscardPendingDamage: true,
		})))
	}
	panic(label + " never came on turn")
}

func (rt *rollsTable) endTurn(p *person) *playv1.Encounter {
	cur := rt.encounterOf(rt.master)
	return must(p.combat.EndTurn(rt.t.Context(), rq(&playv1.EndTurnRequest{
		CampaignId: rt.campaign, EncounterId: cur.GetId(), IdempotencyKey: newKey(), ExpectedCombatantId: cur.GetCurrentCombatantId(), ExpectedRound: cur.GetRound(),
	}))).GetEncounter()
}

// attack rolls an attack as p with the typed dice.
func (rt *rollsTable) attack(p *person, attacker, key, target string, faces []int32, edit func(*playv1.RollAttackRequest)) (*playv1.RollAttackResponse, error) {
	req := &playv1.RollAttackRequest{
		CampaignId: rt.campaign, EncounterId: rt.encounter.GetId(), AttackerId: rt.ids[attacker], AttackKey: key, TargetId: rt.ids[target], IdempotencyKey: newKey(), D20Faces: faces,
	}
	if edit != nil {
		edit(req)
	}
	res, err := p.combat.RollAttack(rt.t.Context(), rq(req))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func (rt *rollsTable) damage(p *person, pendingID string, edit func(*playv1.RollDamageRequest)) *playv1.RollDamageResponse {
	req := &playv1.RollDamageRequest{CampaignId: rt.campaign, EncounterId: rt.encounter.GetId(), PendingDamageId: pendingID, IdempotencyKey: newKey()}
	edit(req)
	return must(p.combat.RollDamage(rt.t.Context(), rq(req)))
}

// ---- what is read, and when ----

// rollsRows are the reads that show the combat: the combat itself, its log and the options of a
// turn (as the owner, and as the master for anyone), and the sheet of the skeleton, which is the
// master's.
func (rt *rollsTable) buildRows() {
	for _, label := range []string{labelToren, labelPens, "Lobo"} {
		allow := masterOnlyRead
		switch label {
		case labelToren:
			allow = onlyCaio
		case labelPens:
			allow = onlyAna
		}
		rt.rows = append(rt.rows, read{
			procedure: playv1connect.CombatServiceGetTurnOptionsProcedure, label: label, allow: allow,
			why: "a player reads the options of their own combatant, whoever's turn it is",
			req: func(w *world) proto.Message {
				return &playv1.GetTurnOptionsRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), CombatantId: rt.ids[label]}
			},
		})
	}
	rt.rows = append(rt.rows,
		read{
			procedure: playv1connect.CombatServiceGetEncounterProcedure, allow: members,
			req: func(w *world) proto.Message { return &playv1.GetEncounterRequest{CampaignId: w.campaign} },
		},
		read{
			procedure: playv1connect.CombatServiceListCombatLogProcedure, allow: members,
			req: func(w *world) proto.Message {
				return &playv1.ListCombatLogRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId()}
			},
		},
		tvScreenLogRead, // what the TV shows of the combat after each attack: the hidden attackers are not in it
	)
}

// ask asks every row of everyone, as the matrix does, and keeps the answers for the check
// that every marker was found where it may be read.
func (rt *rollsTable) ask(t *testing.T, step string) {
	t.Helper()
	rt.encounter = rt.encounterOf(rt.master)
	for _, r := range rt.rows {
		t.Run(step+": "+r.name(), func(t *testing.T) { rt.runRead(t, r, rt.got) })
	}
}

// ---- the table ----

func TestAttackRollsReachOnlyWhoMayReadThem(t *testing.T) {
	rt := newRollsTable(t)
	w, m, caio, ana := rt.world, rt.master, rt.caio, rt.ana
	ctx := t.Context()
	rt.buildRows()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("the table: %v", r)
		}
	}()

	// A creature's type is never a player's: the pseudo feature that carries it, and the words.
	for _, needle := range []string{"creature-type:", "undead", "fiend"} {
		w.secrets.add(&canary{needle: needle, kind: "creature-type"})
	}
	// The ids of the combatants a player sees are the party's.
	for _, label := range []string{labelToren, labelPens, "Esqueleto", "Arbusto", "Bandido", "Lobo"} {
		w.secrets.id("rolls-combatant", rt.ids[label], ana, caio)
	}

	watchers := map[*person]*streamWatcher{m: w.watchStream(m), ana: w.watchStream(ana), caio: w.watchStream(caio)}
	rt.toTurn(labelToren)
	rt.ask(t, "the combat begins")

	// 1. A request for a better mode: the reason is the master's and the requester's.
	reason := w.secrets.marker(kindRequestReason, caio)
	asked := must(caio.combat.RequestRollMode(ctx, rq(&playv1.RequestRollModeRequest{
		CampaignId: w.campaign, EncounterId: rt.encounter.GetId(), AttackerId: rt.ids[labelToren], AttackKey: dagger, TargetId: rt.ids["Esqueleto"],
		RollMode: playv1.RollMode_ROLL_MODE_ADVANTAGE, Reason: reason, IdempotencyKey: newKey(),
	}))).GetRequest()
	if asked.GetStatus() != playv1.RollModeRequestStatus_ROLL_MODE_REQUEST_STATUS_PENDING {
		t.Fatalf("the request = %v, want it pending", asked)
	}
	rt.ask(t, "the request waits")
	for who, p := range map[string]*person{"the master": m, "the requester": caio} {
		if got := rt.encounterOf(p).GetRollModeRequests(); len(got) != 1 || got[0].GetReason() != reason {
			t.Errorf("%s reads the requests %v, want the one with its reason", who, got)
		}
	}
	if got := rt.encounterOf(ana).GetRollModeRequests(); len(got) != 0 {
		t.Errorf("the other player reads the requests %v, want none", got)
	}
	must(m.combat.AnswerRollModeRequest(ctx, rq(&playv1.AnswerRollModeRequestRequest{
		CampaignId: w.campaign, EncounterId: rt.encounter.GetId(), RequestId: asked.GetId(), DecidedMode: playv1.RollMode_ROLL_MODE_ADVANTAGE, IdempotencyKey: newKey(),
	})))
	rt.ask(t, "the request is answered")

	// 3. The attack uses the answer, and the damage is a Divine Smite on an undead.
	hit := rt.mustAttack(caio, labelToren, dagger, "Esqueleto", []int32{4, 17}, func(r *playv1.RollAttackRequest) { r.RollModeRequestId = asked.GetId() })
	if hit.GetRoll().GetMode() != playv1.RollMode_ROLL_MODE_ADVANTAGE || hit.GetPendingDamage() == nil {
		t.Fatalf("the attack = %v, want a hit with the advantage the master gave", hit.GetRoll())
	}
	rt.ask(t, "the attack hit")
	rollSmite := func(pending string, extras ...string) *playv1.PendingDamage {
		parts := []*playv1.TypedPart{{PartKey: "weapon", Sum: 1}, {PartKey: "divine-smite", Sum: 2}, {PartKey: "divine-smite-extra", Sum: 1}}
		chosen := []*playv1.SelectedExtra{{Key: "divine-smite", SlotLevel: 1}}
		for _, key := range extras {
			chosen = append(chosen, &playv1.SelectedExtra{Key: key})
			parts = append(parts, &playv1.TypedPart{PartKey: key, Sum: 4})
		}
		return rt.damage(caio, pending, func(r *playv1.RollDamageRequest) {
			r.ExtrasChosen, r.SelectedExtras, r.TypedParts = true, chosen, parts
		}).GetPendingDamage()
	}
	onUndead := rollSmite(hit.GetPendingDamage().GetId())
	rt.ask(t, "the smite on the undead")
	rt.endTurn(caio)

	// 3. An NPC's resistance. The master rolls Toren's attack with a worse mode and says why.
	rt.toTurn(labelToren)
	changed := w.secrets.marker(kindModeReason, caio)
	graze := rt.mustAttack(m, labelToren, dagger, "Arbusto", []int32{15, 18}, func(r *playv1.RollAttackRequest) {
		r.RollMode, r.ModeReason = playv1.RollMode_ROLL_MODE_DISADVANTAGE, changed
	})
	if graze.GetPendingDamage() == nil {
		t.Fatalf("the attack on the shrub = %v, want a hit", graze.GetRoll())
	}
	rt.ask(t, "the master changed the mode")
	resisted := rt.damage(caio, graze.GetPendingDamage().GetId(), func(r *playv1.RollDamageRequest) {
		r.ExtrasChosen, r.Roll = true, &playv1.RollDamageRequest_TypedSum{TypedSum: 3}
	}).GetPendingDamage()
	rt.ask(t, "the damage a shrub resists")
	rt.checkResisted(t, "Arbusto", resisted)
	rt.endTurn(caio)

	// 4. The same smite on a target that is not undead, with Sneak Attack that the master takes out.
	rt.toTurn(labelToren)
	must(m.combat.SetCombatantConditions(ctx, rq(&playv1.SetCombatantConditionsRequest{
		CampaignId: w.campaign, EncounterId: rt.encounter.GetId(), CombatantId: rt.ids["Bandido"], IdempotencyKey: newKey(),
		Conditions: &playv1.ConditionList{Keys: []string{"condition:restrained"}},
	})))
	// the source a player sees is listed for them, in the options and in the roll
	seen := rt.targetOf(caio, labelToren, dagger, "Bandido")
	if seen.GetRollMode() != playv1.RollMode_ROLL_MODE_ADVANTAGE || !hasSource(seen.GetSources(), playv1.AdvantageSourceKind_ADVANTAGE_SOURCE_KIND_RESTRAINED_TARGET) {
		t.Errorf("Caio reads the restrained bandit as %v, want advantage with its source listed", seen)
	}
	strike := rt.mustAttack(caio, labelToren, dagger, "Bandido", []int32{5, 18}, nil)
	if strike.GetPendingDamage() == nil || !hasSource(strike.GetRoll().GetSources(), playv1.AdvantageSourceKind_ADVANTAGE_SOURCE_KIND_RESTRAINED_TARGET) {
		t.Fatalf("the attack on the bandit = %v, want a hit that lists the source", strike.GetRoll())
	}
	onHuman := rollSmite(strike.GetPendingDamage().GetId(), "sneak-attack")
	removal := w.secrets.marker(kindRemovalReason, caio)
	must(m.combat.RemoveDamagePart(ctx, rq(&playv1.RemoveDamagePartRequest{
		CampaignId: w.campaign, EncounterId: rt.encounter.GetId(), PendingDamageId: strike.GetPendingDamage().GetId(), PartKey: "sneak-attack", Reason: removal, IdempotencyKey: newKey(),
	})))
	rt.ask(t, "an extra is taken out")
	rt.checkSmite(t, onUndead, onHuman)
	rt.endTurn(caio)

	// 5. The barbarian shoots with a hidden enemy at her side: the disadvantage counts, and the
	// source is not hers to read. A visible enemy at Toren's side, on the same shot, is.
	rt.toTurn(labelPens)
	hiddenSide := rt.targetOf(ana, labelPens, shortbow, "Esqueleto")
	if hiddenSide.GetRollMode() != playv1.RollMode_ROLL_MODE_DISADVANTAGE || len(hiddenSide.GetSources()) != 0 {
		t.Errorf("Ana reads the shot as %v, want disadvantage with no source listed", hiddenSide)
	}
	if masterSide := rt.targetOf(m, labelPens, shortbow, "Esqueleto"); !hasSource(masterSide.GetSources(), playv1.AdvantageSourceKind_ADVANTAGE_SOURCE_KIND_HOSTILE_NEARBY) {
		t.Fatalf("the master reads the shot as %v, want the hidden enemy among its sources (the positive control)", masterSide)
	}
	if shot := rt.targetOf(caio, labelToren, shortbow, "Bandido"); !hasSource(shot.GetSources(), playv1.AdvantageSourceKind_ADVANTAGE_SOURCE_KIND_HOSTILE_NEARBY) {
		t.Errorf("Caio reads the shot of a man with a skeleton at his side as %v, want the visible enemy listed", shot)
	}
	rt.take(ana, labelPens, "feature:rage")
	rt.registerStates(labelPens, true)
	arrow := rt.mustAttack(ana, labelPens, shortbow, "Esqueleto", []int32{14, 3}, nil)
	if arrow.GetRoll().GetMode() != playv1.RollMode_ROLL_MODE_DISADVANTAGE || len(arrow.GetRoll().GetSources()) != 0 {
		t.Errorf("Ana's roll = %v, want disadvantage and no source listed", arrow.GetRoll())
	}
	rt.ask(t, "the barbarian shot and raged")
	rt.endTurn(ana)

	// 6. A wolf with Pack Tactics, with the only enemy at its target's side a hidden one: the
	// master reads the source, and a player reads how an NPC rolled in no way.
	for label, sq := range map[string][2]int32{"Esqueleto": {8, 13}, "Bandido": {9, 13}, "Arbusto": {10, 13}} {
		must(m.combat.MoveCombatant(ctx, rq(&playv1.MoveCombatantRequest{CampaignId: w.campaign, EncounterId: rt.encounter.GetId(), CombatantId: rt.ids[label], IdempotencyKey: newKey(), Col: sq[0], Row: sq[1], Forced: true})))
	}
	rt.toTurn("Lobo")
	if pack := rt.targetOf(m, "Lobo", "basic:0", labelToren); !hasSource(pack.GetSources(), playv1.AdvantageSourceKind_ADVANTAGE_SOURCE_KIND_PACK_TACTICS) {
		t.Fatalf("the master reads the wolf's bite as %v, want Pack Tactics among its sources (the positive control)", pack)
	}
	npcReason := w.secrets.marker(kindNPCModeReason)
	bite := rt.mustAttack(m, "Lobo", "basic:0", labelToren, []int32{12}, func(r *playv1.RollAttackRequest) {
		r.RollMode, r.ModeReason = playv1.RollMode_ROLL_MODE_NORMAL, npcReason
	})
	if bite.GetRoll().GetSuggestedMode() != playv1.RollMode_ROLL_MODE_ADVANTAGE || !hasSource(bite.GetRoll().GetSources(), playv1.AdvantageSourceKind_ADVANTAGE_SOURCE_KIND_PACK_TACTICS) {
		t.Fatalf("the wolf's roll = %v, want the advantage of Pack Tactics suggested", bite.GetRoll())
	}
	rt.ask(t, "an NPC rolled with its own mode")
	rt.checkNPCRoll(t, "Lobo")

	// 7. The states: a combatant in plain sight shows its state, and one the master hides shows
	// none, nor the combatant that caused it. The raging barbarian's player is asked whether the
	// rage ends, and everyone who sees her reads who the turn waits for.
	rt.toTurn("Bandido")
	rt.take(m, "Bandido", "standard:dodge")
	rt.registerStates("Bandido", true)
	rt.toTurn("EscondidoA")

	rt.take(m, "EscondidoA", "standard:dodge")
	rt.registerStates("EscondidoA", false)
	rt.ask(t, "states")
	for _, p := range []*person{ana, caio} {
		if st := rt.combatantOf(p, "Bandido").GetStates(); len(st) != 1 || st[0].GetKind() != playv1.CombatantStateKind_COMBATANT_STATE_KIND_DODGING {
			t.Errorf("%s reads the dodging bandit in the states %v, want the chip", p.name, st)
		}
		if rt.combatantOf(p, "EscondidoA") != nil {
			t.Errorf("%s reads the hidden combatant", p.name)
		}
	}
	rt.toTurn(labelPens)
	if pending := rt.endTurn(ana); pending.GetRagePendingCombatantId() != rt.ids[labelPens] {
		t.Fatalf("the turn waits for %q, want the barbarian %q", pending.GetRagePendingCombatantId(), rt.ids[labelPens])
	}
	rt.ask(t, "the rage question")
	for _, p := range []*person{m, ana, caio} {
		if got := rt.encounterOf(p).GetRagePendingCombatantId(); got != rt.ids[labelPens] {
			t.Errorf("%s reads that the turn waits for %q, want the barbarian", p.name, got)
		}
	}
	must(ana.combat.AnswerRageEnd(ctx, rq(&playv1.AnswerRageEndRequest{
		CampaignId: w.campaign, EncounterId: rt.encounter.GetId(), CombatantId: rt.ids[labelPens], EndRage: false, IdempotencyKey: newKey(),
	})))
	must(ana.combat.EndRage(ctx, rq(&playv1.EndRageRequest{CampaignId: w.campaign, EncounterId: rt.encounter.GetId(), CombatantId: rt.ids[labelPens], IdempotencyKey: newKey()})))
	rt.ask(t, "the rage ended")

	// 8. Where a condition comes from.
	t.Run("the sources of a condition", func(t *testing.T) {
		rt.sourced(labelPens, "EscondidoA", "condition:stunned")
		rt.sourced(labelToren, "Esqueleto", "condition:stunned")
		rt.ask(t, "the conditions have sources")
		if got := rt.combatantOf(caio, labelToren).GetConditionSources(); len(got) != 1 || got[0].GetSourceLabel() != rt.combatantOf(m, "Esqueleto").GetLabel() {
			t.Errorf("Caio reads the sources %v of the paladin's condition, want the skeleton that stunned him", got)
		}
	})

	// The end: the stream told every player of every step, and carried none of it.
	must(m.combat.EndEncounter(ctx, rq(&playv1.EndEncounterRequest{CampaignId: w.campaign, EncounterId: rt.encounter.GetId(), IdempotencyKey: newKey()})))
	must(m.play.EndGameSession(ctx, rq(&playv1.EndGameSessionRequest{CampaignId: w.campaign, GameSessionId: w.session})))
	for p, sw := range watchers {
		select {
		case <-sw.done:
		case <-time.After(30 * time.Second):
			t.Fatalf("%s's stream did not end with the session", p.name)
		}
		if len(sw.events) < 2 {
			t.Errorf("%s's stream heard %d events of all this, so it proves nothing", p.name, len(sw.events))
		}
		for _, ev := range sw.events {
			body, err := protojson.Marshal(ev)
			if err != nil {
				t.Fatalf("marshal an event: %v", err)
			}
			r := reply{status: 200, body: body, msg: ev}
			rt.got.keep(p, r)
			if p == m {
				continue
			}
			for _, f := range w.inspect(p, r, nil) {
				t.Errorf("%s: event %s: %s\n\tevent: %s", p.name, eventCase(ev), f, shorten(body))
			}
		}
	}
	t.Run("canaries are reachable", func(t *testing.T) { checkCanariesAreReachable(t, w, rt.got) })
}

func (rt *rollsTable) mustAttack(p *person, attacker, key, target string, faces []int32, edit func(*playv1.RollAttackRequest)) *playv1.RollAttackResponse {
	res, err := rt.attack(p, attacker, key, target, faces, edit)
	if err != nil {
		panic(fmt.Sprintf("%s attacks %s: %v", attacker, target, err))
	}
	return res
}

// targetOf is the target of an attack in the options of a combatant, as p reads them.
func (rt *rollsTable) targetOf(p *person, attacker, attackKey, target string) *playv1.TargetInReach {
	for _, g := range rt.optionsOf(p, attacker).GetAttackTargets() {
		if g.GetAttackKey() != attackKey {
			continue
		}
		for _, tg := range g.GetTargets() {
			if tg.GetCombatantId() == rt.ids[target] {
				return tg
			}
		}
	}
	return nil
}

func hasSource(sources []*playv1.AdvantageSource, kind playv1.AdvantageSourceKind) bool {
	return slices.ContainsFunc(sources, func(s *playv1.AdvantageSource) bool { return s.GetKind() == kind })
}

// attackLine is the line of the log where the attacker hit the target, as p reads the log.
func (rt *rollsTable) attackLine(p *person, attacker, target string) *playv1.CombatLogEntry {
	for _, r := range rt.logOf(p).GetRounds() {
		for _, en := range r.GetEntries() {
			if en.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_ATTACK && en.GetActorId() == rt.ids[attacker] && en.GetTargetId() == rt.ids[target] && en.GetDamage() != nil {
				return en
			}
		}
	}
	return nil
}

// checkResisted proves what a player reads of a damage on an NPC that resists it: the damage
// as it was rolled, in the log and in the roll, and no step; the master reads the step.
func (rt *rollsTable) checkResisted(t *testing.T, target string, rolled *playv1.PendingDamage) {
	t.Helper()
	masterLine := rt.attackLine(rt.master, labelToren, target)
	if masterLine == nil || len(masterLine.GetDamage().GetSteps()) != 1 {
		t.Fatalf("the master reads %v, want the line with the resistance as a step (the positive control)", masterLine)
	}
	step := masterLine.GetDamage().GetSteps()[0]
	if step.GetKind() != playv1.DamageStepKind_DAMAGE_STEP_KIND_RESISTANCE || step.GetAfter() >= step.GetBefore() {
		t.Fatalf("the master's step = %v, want the damage halved", step)
	}
	if rolled.GetAmount() != step.GetBefore() || len(rolled.GetSteps()) != 0 || rolled.AmountAfterSteps != nil {
		t.Errorf("the roll answers the player with amount %d, steps %v and %v after steps; want the %d that was rolled and no step", rolled.GetAmount(), rolled.GetSteps(), rolled.AmountAfterSteps, step.GetBefore())
	}
	for _, p := range []*person{rt.ana, rt.caio} {
		line := rt.attackLine(p, labelToren, target)
		if line == nil || line.GetDamage().GetAmount() != step.GetBefore() || len(line.GetDamage().GetSteps()) != 0 {
			t.Errorf("%s reads the line %v, want the %d that was rolled and no step", p.name, line, step.GetBefore())
		}
		walk(rt.encounterOf(p).ProtoReflect(), "Encounter", func(path string, _ protoreflect.Message, fd protoreflect.FieldDescriptor, _ protoreflect.Value) {
			if fd.Name() == "steps" || fd.Name() == "amount_after_steps" {
				t.Errorf("%s reads %s in the combat", p.name, path)
			}
		})
	}
}

// checkSmite proves that a player reads the same lines of a Divine Smite whatever the target is,
// and the master reads which die counts.
func (rt *rollsTable) checkSmite(t *testing.T, undead, human *playv1.PendingDamage) {
	t.Helper()
	lines := func(p *playv1.PendingDamage) (out []string) {
		for _, r := range p.GetPartRolls() {
			if strings.HasPrefix(r.GetPartKey(), "divine-smite") {
				out = append(out, fmt.Sprintf("%s %dd%d=%d counted=%v", r.GetPartKey(), r.GetDiceCount(), r.GetDiceSides(), r.GetSum(), r.GetCounted()))
			}
		}
		return out
	}
	if a, b := lines(undead), lines(human); len(a) != 2 || !slices.Equal(a, b) {
		t.Errorf("the player reads %v for the undead and %v for the other target; want the same two lines, every die counted", a, b)
	}
	counted := func(target string) bool {
		for _, r := range rt.logOf(rt.master).GetRounds() {
			for _, en := range r.GetEntries() {
				if en.GetTargetId() != rt.ids[target] || en.GetKind() != playv1.CombatLogKind_COMBAT_LOG_KIND_ATTACK {
					continue
				}
				for _, pr := range en.GetDamage().GetParts() {
					if pr.GetPartKey() == "divine-smite-extra" {
						return pr.GetCounted()
					}
				}
			}
		}
		panic("the master reads no extra die on " + target)
	}
	if !counted("Esqueleto") || counted("Bandido") {
		t.Errorf("the master reads the extra die as counted = %v on the skeleton and %v on the bandit; want it to count against the undead alone", counted("Esqueleto"), counted("Bandido"))
	}
}

// take is an action of a combatant, taken by its player (or the master, for an NPC).
func (rt *rollsTable) take(p *person, label, action string) {
	must(p.combat.TakeAction(rt.t.Context(), rq(&playv1.TakeActionRequest{
		CampaignId: rt.campaign, EncounterId: rt.encounter.GetId(), CombatantId: rt.ids[label], ActionKey: action, IdempotencyKey: newKey(),
	})))
}

// registerStates makes the ids of the states a combatant is in canaries: the party's players
// may read them when the party sees the combatant, nobody else may.
func (rt *rollsTable) registerStates(label string, seen bool) {
	c := rt.combatantOf(rt.master, label)
	if len(c.GetStates()) == 0 {
		panic(label + " is in no state")
	}
	for _, st := range c.GetStates() {
		if seen {
			rt.secrets.id("rolls-state", st.GetId(), rt.ana, rt.caio)
		} else {
			rt.secrets.id("rolls-state", st.GetId())
		}
	}
}

// checkNPCRoll proves that no player reads how an NPC rolled: not its mode, not what the
// master suggested, his reason, nor the circumstances behind it, while the master does.
func (rt *rollsTable) checkNPCRoll(t *testing.T, attacker string) {
	t.Helper()
	line := func(p *person) *playv1.CombatLogEntry {
		for _, r := range rt.logOf(p).GetRounds() {
			for _, en := range r.GetEntries() {
				if en.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_ATTACK && en.GetActorId() == rt.ids[attacker] {
					return en
				}
			}
		}
		return nil
	}
	if en := line(rt.master); en == nil || en.GetModeChange() == nil || en.GetReason() == "" {
		t.Fatalf("the master reads %v, want the mode change and its reason (the positive control)", en)
	}
	for _, p := range []*person{rt.ana, rt.caio} {
		if en := line(p); en == nil || en.GetModeChange() != nil || en.GetReason() != "" || en.GetAttackRoll() != nil {
			t.Errorf("%s reads %v, want the NPC's attack with no mode, no reason and no dice", p.name, en)
		}
	}
}

// sourced gives a combatant a condition that comes from another, as the service stores it.
func (rt *rollsTable) sourced(label, source, condition string) {
	ctx := rt.t.Context()
	c := rt.combatantOf(rt.master, label)
	rt.encounter = rt.encounterOf(rt.master)
	must(rt.master.combat.SetCombatantConditions(ctx, rq(&playv1.SetCombatantConditionsRequest{
		CampaignId: rt.campaign, EncounterId: rt.encounter.GetId(), CombatantId: c.GetId(), IdempotencyKey: newKey(),
		Conditions: &playv1.ConditionList{Keys: append(c.GetConditions(), condition)},
	})))
	stored := fmt.Sprintf(`[{"condition":%q,"source_id":%q,"ends_combatant_id":%q,"ends_phase":"end_of_turn"}]`, condition, rt.ids[source], rt.ids[source])
	if _, err := rt.pool.Exec(ctx, `UPDATE combatants SET condition_sources = $1::jsonb WHERE id = $2`, stored, c.GetId()); err != nil {
		panic(err)
	}
}
