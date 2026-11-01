package leaktest

import (
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1/playv1connect"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// The answer to a held roll (RN-10, RN-35): when the master rolls for a player's character and the roll waits for the
// player's question (a Bardic Inspiration die, Lucky), the player answers it, and the response of the answer is the
// answering member's own view: the combat of a player, the group check of a player. The roller's membership stays the
// one the write and its checks run as, so the roll lands as the master made it.
//
// The world is the held rolls' (a fog map, two players, a hidden NPC with a marker on its name and on its hit points),
// with halflings, a cleric who casts Guiding Bolt, and a visible goblin.

const (
	// shadowHP is the hit points of the NPC the master keeps hidden.
	shadowHP = 4321
	// groupDC and otherRollFace are the numbers the group-check row hides from the player who answers.
	groupDC       = 23
	otherRollFace = 19
	// labelBrisa is Caio's cleric, labelShadow the hidden NPC and labelGoblin the visible one.
	labelBrisa, labelShadow, labelGoblin = "Brisa", "Sombra", "Goblin"
)

// answerTable is the world and what the steps need to find in it.
type answerTable struct {
	*world
	got *answers
	// ids are the combatants by label.
	ids map[string]string
	// rows are the reads asked of everyone after each step.
	rows []read
	// brisa is Caio's cleric, pens Ana's halfling.
	brisa *charactersv1.Character
}

// newAnswerTable builds the world and begins the combat: Brisa plays first.
func newAnswerTable(t *testing.T) *answerTable {
	t.Helper()
	w := &world{stack: newStack(t), secrets: newSecrets(), pts: map[string]*mapsv1.MapPoint{}, puzzles: map[string]*playv1.Puzzle{}, notes: map[string]string{}}
	rt := &answerTable{world: w, got: newAnswers(), ids: map[string]string{}}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("building the fixture: %v", r)
		}
	}()
	ctx := t.Context()
	w.buildPeople()
	m := w.master

	img := m.upload(w.campaign, "img.png", pngOf(240, 160, 3))
	w.fogMap = must(m.maps.CreateMap(ctx, rq(&mapsv1.CreateMapRequest{CampaignId: w.campaign, Name: "Estrada", ImageId: img}))).GetMap().GetId()
	must(m.maps.SetMapRevealed(ctx, rq(&mapsv1.SetMapRevealedRequest{CampaignId: w.campaign, MapId: w.fogMap, Revealed: true})))
	must(m.maps.SetMapGrid(ctx, rq(&mapsv1.SetMapGridRequest{CampaignId: w.campaign, MapId: w.fogMap, Columns: caveColumns})))
	must(m.maps.SetMapFog(ctx, rq(&mapsv1.SetMapFogRequest{CampaignId: w.campaign, MapId: w.fogMap, FogEnabled: new(true), BaseLight: mapsv1.LightLevel_LIGHT_LEVEL_DIM.Enum()})))
	w.session = must(m.play.StartGameSession(ctx, rq(&playv1.StartGameSessionRequest{CampaignId: w.campaign}))).GetGameSession().GetId()
	must(m.play.SetCurrentMap(ctx, rq(&playv1.SetCurrentMapRequest{CampaignId: w.campaign, MapId: w.fogMap})))

	// Caio's cleric prepares Guiding Bolt; Ana's halfling is a plain character. Both are halflings (Lucky).
	rt.brisa = must(w.caio.characters.CreateCharacter(ctx, rq(&charactersv1.CreateCharacterRequest{
		CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: labelBrisa,
		Sheet: &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
			BaseScores: &rulesv1.AbilityScores{Strength: 10, Dexterity: 16, Constitution: 14, Intelligence: 10, Wisdom: 16, Charisma: 8}, RaceKey: "race:halfling",
			Classes: []*charactersv1.ClassLevel{{ClassKey: "class:cleric", Level: 3}}, WeaponKeys: []string{"equipment:mace"},
			CantripKeys: []string{"spell:sacred-flame"}, PreparedSpellKeys: []string{"spell:guiding-bolt"},
		}}},
	}))).GetCharacter()
	w.toren = rt.brisa
	w.pens = w.pc(w.ana, labelPens, "race:halfling")
	w.place(w.fogMap, w.toren.GetId(), 4, 4)
	w.place(w.fogMap, w.pens.GetId(), 10, 3)

	// The NPC the master keeps hidden: its name and its hit points are the master's.
	w.secrets.number("npc-hit-points", shadowHP)
	shadow := must(m.characters.CreateCharacter(ctx, rq(&charactersv1.CreateCharacterRequest{
		CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_MINION, Name: w.secrets.marker("shadow-name"),
		Sheet: &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Basic{Basic: &charactersv1.BasicSheet{HitPointsMax: shadowHP, ArmorClass: 12, SpeedFt: 30}}},
	}))).GetCharacter()
	e := must(m.combat.StartEncounter(ctx, rq(&playv1.StartEncounterRequest{
		CampaignId: w.campaign, IdempotencyKey: newKey(), Name: "Emboscada", Participants: []*playv1.Participant{{CharacterId: shadow.GetId()}},
	}))).GetEncounter()
	initiative := func(id string, face int32) {
		must(m.combat.SubmitInitiative(ctx, rq(&playv1.SubmitInitiativeRequest{
			CampaignId: w.campaign, EncounterId: e.GetId(), CombatantId: id, IdempotencyKey: newKey(), Roll: &playv1.SubmitInitiativeRequest_D20Face{D20Face: face},
		})))
	}
	for _, c := range e.GetCombatants() {
		rt.ids[c.GetLabel()] = c.GetId()
		switch c.GetCharacterId() {
		case shadow.GetId():
			rt.ids[labelShadow] = c.GetId()
			w.secrets.id("held-answer-shadow", c.GetId())
			initiative(c.GetId(), 5)
		case rt.brisa.GetId():
			initiative(c.GetId(), 20)
		case w.pens.GetId():
			initiative(c.GetId(), 10)
		}
	}
	hidden := false
	goblin := must(m.combat.AddMonsters(ctx, rq(&playv1.AddMonstersRequest{
		CampaignId: w.campaign, EncounterId: e.GetId(), IdempotencyKey: newKey(), CreatureKey: "monster:goblin", Count: 1, Name: w.secrets.public("goblin-name"), Hidden: &hidden,
	}))).GetCombatantIds()[0]
	rt.ids[labelGoblin] = goblin
	initiative(goblin, 3)
	w.secrets.add(&canary{needle: "monster:goblin", kind: "monster-key"})
	for label, sq := range map[string][2]int32{labelBrisa: {4, 4}, labelGoblin: {4, 3}, labelPens: {10, 3}, labelShadow: {12, 3}} {
		must(m.combat.MoveCombatant(ctx, rq(&playv1.MoveCombatantRequest{CampaignId: w.campaign, EncounterId: e.GetId(), CombatantId: rt.ids[label], IdempotencyKey: newKey(), Col: sq[0], Row: sq[1], Forced: true})))
	}
	w.encounter = must(m.combat.BeginCombat(ctx, rq(&playv1.BeginCombatRequest{CampaignId: w.campaign, EncounterId: e.GetId(), IdempotencyKey: newKey()}))).GetEncounter()
	if w.encounter.GetCurrentCombatantId() != rt.ids[labelBrisa] {
		panic("Brisa does not play first")
	}

	rt.rows = []read{
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
			procedure: playv1connect.ResourceServiceListHeldRollsProcedure, allow: members,
			req: func(w *world) proto.Message { return &playv1.ListHeldRollsRequest{CampaignId: w.campaign} },
		},
		{
			procedure: playv1connect.ContestServiceGetGroupCheckProcedure, allow: members,
			why: "the answer follows the table's rules for a player",
			req: func(w *world) proto.Message { return &playv1.GetGroupCheckRequest{CampaignId: w.campaign} },
		},
	}
	return rt
}

// encounterOf is the combat as p reads it.
func (rt *answerTable) encounterOf(p *person) *playv1.Encounter {
	return must(p.combat.GetEncounter(rt.t.Context(), rq(&playv1.GetEncounterRequest{CampaignId: rt.campaign}))).GetEncounter()
}

// combatantOf is the combatant as the master reads it.
func (rt *answerTable) combatantOf(label string) *playv1.Combatant {
	for _, c := range rt.encounterOf(rt.master).GetCombatants() {
		if c.GetId() == rt.ids[label] {
			return c
		}
	}
	return nil
}

// giveDie puts a d8 on the combatant, as the bard's inspiration would.
func (rt *answerTable) giveDie(label string) {
	round := rt.encounterOf(rt.master).GetRound()
	if _, err := rt.pool.Exec(rt.t.Context(), `UPDATE combatants SET inspiration_sides = 8, inspiration_expires_round = $2 WHERE id = $1`, rt.ids[label], round+50); err != nil {
		panic(err)
	}
}

// scan keeps what p was answered and looks in it for what p may not read.
func (rt *answerTable) scan(t *testing.T, p *person, what string, msg proto.Message) {
	t.Helper()
	body, err := protojson.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal %s: %v", what, err)
	}
	r := reply{status: 200, body: body, msg: msg}
	rt.got.keep(p, r)
	for _, f := range rt.inspect(p, r, nil) {
		t.Errorf("%s: %s: %s\n\tanswer: %s", p.name, what, f, shorten(body))
	}
}

// ask asks every row of everyone and keeps the answers.
func (rt *answerTable) ask(t *testing.T, step string) {
	t.Helper()
	rt.encounter = rt.encounterOf(rt.master)
	for _, r := range rt.rows {
		t.Run(step+": "+r.name(), func(t *testing.T) { rt.runRead(t, r, rt.got) })
	}
}

// watch opens the stream of the master and of the two players.
func (rt *answerTable) watch() map[*person]*streamWatcher {
	return map[*person]*streamWatcher{rt.master: rt.watchStream(rt.master), rt.ana: rt.watchStream(rt.ana), rt.caio: rt.watchStream(rt.caio)}
}

// finish ends the session and looks in the streams for what each player may not read.
func (rt *answerTable) finish(t *testing.T, watchers map[*person]*streamWatcher) {
	t.Helper()
	m := rt.master
	must(m.combat.EndEncounter(t.Context(), rq(&playv1.EndEncounterRequest{CampaignId: rt.campaign, EncounterId: rt.encounter.GetId(), IdempotencyKey: newKey()})))
	must(m.play.EndGameSession(t.Context(), rq(&playv1.EndGameSessionRequest{CampaignId: rt.campaign, GameSessionId: rt.session})))
	for p, sw := range watchers {
		select {
		case <-sw.done:
		case <-time.After(30 * time.Second):
			t.Fatalf("%s's stream did not end with the session", p.name)
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
			for _, f := range rt.inspect(p, r, nil) {
				t.Errorf("%s: event %s: %s\n\tevent: %s", p.name, eventCase(ev), f, shorten(body))
			}
		}
	}
	t.Run("canaries are reachable", func(t *testing.T) { checkCanariesAreReachable(t, rt.world, rt.got) })
}

// dieAnswerOf is the answer that uses the Bardic Inspiration die, typed from a real die.
func dieAnswerOf(face int32) *playv1.AnswerHeldRollRequest_Die {
	return &playv1.AnswerHeldRollRequest_Die{Die: &playv1.DieAnswer{Use: true, Roll: &playv1.DieAnswer_TypedFace{TypedFace: face}}}
}

// luckyAnswerOf is the answer that rolls the die that shows the 1 again, typed.
func luckyAnswerOf(face int32) *playv1.AnswerHeldRollRequest_Lucky {
	return &playv1.AnswerHeldRollRequest_Lucky{Lucky: &playv1.LuckyAnswer{Reroll: true, Roll: &playv1.LuckyAnswer_TypedFace{TypedFace: face}}}
}

// TestAHeldRollsAnswerIsRenderedForWhoAnswers: the master rolls for a player's character, the roll waits for the
// player, and the answer comes back as the player's own view (nothing of the combat or of the group check the
// master hides), while the roll lands as the master made it. Each row also looks in the streams.
func TestAHeldRollsAnswerIsRenderedForWhoAnswers(t *testing.T) {
	// The master casts Guiding Bolt for Brisa, who holds a die: the cast waits for her answer.
	castHeld := func(t *testing.T, rt *answerTable) string {
		t.Helper()
		rt.giveDie(labelBrisa)
		res := must(rt.master.combat.CastSpell(t.Context(), rq(&playv1.CastSpellRequest{
			CampaignId: rt.campaign, EncounterId: rt.encounter.GetId(), CasterId: rt.ids[labelBrisa], SpellKey: "spell:guiding-bolt", Slot: &playv1.SpellSlot{Level: 1},
			Targets: []*playv1.SpellTarget{{CombatantId: rt.ids[labelGoblin]}}, IdempotencyKey: newKey(), D20Faces: []int32{4, 4},
		})))
		if res.GetCast() != nil || res.GetHeldRollOffer() == nil {
			t.Fatalf("the master's cast = %v, want the die's question and no cast", res)
		}
		return res.GetHeldRollOffer().GetHoldId()
	}
	castSettled := func(t *testing.T, rt *answerTable, cast *playv1.SpellCast) {
		t.Helper()
		if len(cast.GetTargets()) != 1 || len(cast.GetTargets()[0].GetBonusDice()) != 1 || cast.GetTargets()[0].GetAttackRoll().GetFaces()[0] != 4 {
			t.Errorf("the cast = %v, want the master's d20 of 4 with the player's d8", cast)
		}
		if !rt.combatantOf(labelBrisa).GetActionUsed() {
			t.Error("the action is not spent after the answer")
		}
	}

	// The master rolls the end-of-turn save of Hold Person for Brisa on a 1: the save waits for Lucky.
	saveHeld := func(t *testing.T, rt *answerTable) string {
		t.Helper()
		m := rt.master
		visible := true
		must(m.lasting.AddLastingEffect(t.Context(), rq(&playv1.AddLastingEffectRequest{
			CampaignId: rt.campaign, EncounterId: rt.encounter.GetId(), IdempotencyKey: newKey(), CatalogKey: "spell:hold-person",
			TargetIds: []string{rt.ids[labelBrisa]}, SaveDc: new(int32(37)), PlayerVisible: &visible,
			Duration: &playv1.EffectDurationChoice{Kind: playv1.EffectDurationKind_EFFECT_DURATION_KIND_ROUNDS, Rounds: 10, AnchorCombatantId: rt.ids[labelBrisa]},
		})))
		must(m.combat.EndTurn(t.Context(), rq(&playv1.EndTurnRequest{
			CampaignId: rt.campaign, EncounterId: rt.encounter.GetId(), IdempotencyKey: newKey(), ExpectedCombatantId: rt.ids[labelBrisa], DiscardPendingDamage: true,
		})))
		var window string
		for _, rw := range rt.encounterOf(m).GetReactionWindows() {
			if rw.GetKind() == playv1.ReactionKind_REACTION_KIND_EFFECT_SAVE {
				window = rw.GetId()
			}
		}
		if window == "" {
			t.Fatal("Brisa's save is not asked")
		}
		res := must(m.lasting.RollEffectSave(t.Context(), rq(&playv1.RollEffectSaveRequest{
			CampaignId: rt.campaign, EncounterId: rt.encounter.GetId(), WindowId: window, IdempotencyKey: newKey(), Roll: &playv1.RollEffectSaveRequest_D20Face{D20Face: 1},
		})))
		if res.GetResult() != nil || res.GetHeldRollOffer() == nil {
			t.Fatalf("the master's save = %v, want Lucky's question alone", res)
		}
		return res.GetHeldRollOffer().GetHoldId()
	}
	saveSettled := func(t *testing.T, rt *answerTable, res *playv1.RollEffectSaveResponse) {
		t.Helper()
		if res.GetResult().GetD20() != 17 || len(res.GetResult().GetRerolled()) != 1 {
			t.Errorf("the save = %v, want the 1 rolled again to 17", res.GetResult())
		}
		if res.GetResult().Dc != nil {
			t.Errorf("the save carries the DC %d at a table that does not show it", res.GetResult().GetDc())
		}
		if got := rt.combatantOf(labelBrisa).GetEffects(); len(got) != 1 {
			t.Errorf("Brisa's effects = %v, want the one the save did not end", got)
		}
	}
	answerHeld := func(rt *answerTable, hold string, answer func(*playv1.AnswerHeldRollRequest)) *playv1.AnswerHeldRollResponse {
		req := &playv1.AnswerHeldRollRequest{CampaignId: rt.campaign, HoldId: hold, IdempotencyKey: newKey()}
		answer(req)
		return must(rt.caio.resources.AnswerHeldRoll(rt.t.Context(), rq(req)))
	}
	run := func(t *testing.T, body func(t *testing.T, rt *answerTable)) {
		t.Helper()
		rt := newAnswerTable(t)
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("the table: %v", r)
			}
		}()
		watchers := rt.watch()
		body(t, rt)
		rt.ask(t, "after the answer")
		rt.finish(t, watchers)
	}

	t.Run("a spell attack with a die, AnswerHeldRoll", func(t *testing.T) {
		run(t, func(t *testing.T, rt *answerTable) {
			t.Helper()
			hold := castHeld(t, rt)
			out := answerHeld(rt, hold, func(r *playv1.AnswerHeldRollRequest) { r.Answer = dieAnswerOf(8) })
			rt.scan(t, rt.caio, "Caio's answer", out)
			castSettled(t, rt, out.GetSpellCast().GetCast())
		})
	})
	t.Run("a spell attack with a die, AnswerOutsideInspiration", func(t *testing.T) {
		run(t, func(t *testing.T, rt *answerTable) {
			t.Helper()
			hold := castHeld(t, rt)
			out := answerOutsideDie(rt, hold, newKey(), 8)
			rt.scan(t, rt.caio, "Caio's answer", out)
			castSettled(t, rt, out.GetSpellCast().GetCast())
		})
	})
	t.Run("a save that shows a 1, AnswerHeldRoll", func(t *testing.T) {
		run(t, func(t *testing.T, rt *answerTable) {
			t.Helper()
			hold := saveHeld(t, rt)
			out := answerHeld(rt, hold, func(r *playv1.AnswerHeldRollRequest) { r.Answer = luckyAnswerOf(17) })
			rt.scan(t, rt.caio, "Caio's answer", out)
			saveSettled(t, rt, out.GetEffectSave())
		})
	})
	t.Run("a save that shows a 1, AnswerOutsideLucky", func(t *testing.T) {
		run(t, func(t *testing.T, rt *answerTable) {
			t.Helper()
			hold := saveHeld(t, rt)
			out := must(rt.caio.resources.AnswerOutsideLucky(t.Context(), rq(&playv1.AnswerOutsideLuckyRequest{ //nolint:staticcheck // the row covers the deprecated RPC while it exists
				CampaignId: rt.campaign, HoldId: hold, Reroll: true, IdempotencyKey: newKey(), Roll: &playv1.AnswerOutsideLuckyRequest_TypedFace{TypedFace: 17},
			})))
			rt.scan(t, rt.caio, "Caio's answer", out)
			saveSettled(t, rt, out.GetEffectSave())
		})
	})
	t.Run("a group check that does not show its DC", func(t *testing.T) {
		run(t, func(t *testing.T, rt *answerTable) {
			t.Helper()
			m, w := rt.master, rt.world
			typed := func(face int32) *playv1.CheckRollInput {
				return &playv1.CheckRollInput{Roll: &playv1.CheckRollInput_D20Faces{D20Faces: &playv1.D20Faces{Faces: []int32{face}}}}
			}
			// The DC and Caio's roll are the master's, and Caio's roll is his own to read.
			w.secrets.number("group-check-dc", groupDC, "dc")
			w.secrets.number("group-check-other-roll", otherRollFace, "faces")
			w.secrets.allowNeedle("19", w.caio)
			view := must(m.contests.RequestGroupCheck(t.Context(), rq(&playv1.RequestGroupCheckRequest{
				CampaignId: w.campaign, IdempotencyKey: newKey(), SkillKey: "skill:stealth", Dc: groupDC, ShowDc: false,
				CharacterIds: []string{rt.brisa.GetId(), w.pens.GetId()}, Group: new(true),
			}))).GetGroupCheck()
			must(w.caio.contests.RollGroupCheck(t.Context(), rq(&playv1.RollGroupCheckRequest{
				CampaignId: w.campaign, IdempotencyKey: newKey(), GroupCheckId: view.GetId(), Roll: typed(otherRollFace),
			})))
			held := must(m.contests.RollForPlayer(t.Context(), rq(&playv1.RollForPlayerRequest{
				CampaignId: w.campaign, IdempotencyKey: newKey(), GroupCheckId: view.GetId(), CharacterId: w.pens.GetId(), Roll: typed(1),
			})))
			if held.GetGroupCheck() != nil || held.GetHeldRollOffer() == nil {
				t.Fatalf("the master's roll = %v, want Lucky's question alone", held)
			}
			req := &playv1.AnswerHeldRollRequest{CampaignId: w.campaign, HoldId: held.GetHeldRollOffer().GetHoldId(), IdempotencyKey: newKey(), Answer: luckyAnswerOf(14)}
			out := must(w.ana.resources.AnswerHeldRoll(t.Context(), rq(req)))
			rt.scan(t, w.ana, "Ana's answer", out)

			// The roll lands as the master's: Pensantus's 1 rolled again to 14, still the master's roll.
			var roll *playv1.CheckRoll
			got := must(m.contests.GetGroupCheck(t.Context(), rq(&playv1.GetGroupCheckRequest{CampaignId: w.campaign}))).GetGroupCheck()
			for _, member := range got.GetMembers() {
				if member.GetCharacterId() == w.pens.GetId() {
					roll = member.GetRoll()
				}
			}
			if roll.GetFaces()[0] != 14 || len(roll.GetRerolled()) != 1 || !roll.GetRolledByMaster() {
				t.Errorf("the roll for Pensantus = %v, want the 1 rolled again to 14, still the master's roll", roll)
			}
		})
	})
}

// logLines is how many lines of the combat's log the master reads.
func (rt *answerTable) logLines() int {
	n := 0
	for _, r := range must(rt.master.combat.ListCombatLog(rt.t.Context(), rq(&playv1.ListCombatLogRequest{CampaignId: rt.campaign, EncounterId: rt.encounter.GetId()}))).GetRounds() {
		n += len(r.GetEntries())
	}
	return n
}

// TestAHeldRollsAnswerGivenAgainIsRenderedForWhoAnswers: the master's cast for a player's character, answered by the
// player and settled, comes again with the same answer; the second response is the player's view like the first, and
// the master's write stays as the first answer made it.
func TestAHeldRollsAnswerGivenAgainIsRenderedForWhoAnswers(t *testing.T) {
	run := func(t *testing.T, ask func(rt *answerTable, hold, key string) (proto.Message, *playv1.CastSpellResponse)) {
		t.Helper()
		rt := newAnswerTable(t)
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("the table: %v", r)
			}
		}()
		watchers := rt.watch()
		rt.giveDie(labelBrisa)
		res := must(rt.master.combat.CastSpell(t.Context(), rq(&playv1.CastSpellRequest{
			CampaignId: rt.campaign, EncounterId: rt.encounter.GetId(), CasterId: rt.ids[labelBrisa], SpellKey: "spell:guiding-bolt", Slot: &playv1.SpellSlot{Level: 1},
			Targets: []*playv1.SpellTarget{{CombatantId: rt.ids[labelGoblin]}}, IdempotencyKey: newKey(), D20Faces: []int32{4, 4},
		})))
		if res.GetCast() != nil || res.GetHeldRollOffer() == nil {
			t.Fatalf("the master's cast = %v, want the die's question and no cast", res)
		}
		hold, key := res.GetHeldRollOffer().GetHoldId(), newKey()

		whole, first := ask(rt, hold, key)
		rt.scan(t, rt.caio, "Caio's first answer", whole)
		rt.ask(t, "after the first answer")
		// The master hides the target between the two answers: the answer again is still rendered for who answers.
		must(rt.master.combat.SetCombatantHidden(t.Context(), rq(&playv1.SetCombatantHiddenRequest{
			CampaignId: rt.campaign, EncounterId: rt.encounter.GetId(), CombatantId: rt.ids[labelGoblin], IdempotencyKey: newKey(), Hidden: true,
		})))
		lines := rt.logLines()
		whole, again := ask(rt, hold, key)
		rt.scan(t, rt.caio, "Caio's answer again", whole)
		if again.GetCast() == nil || strings.Contains(protojson.Format(whole), rt.ids[labelGoblin]) || again.GetPendingRevealId() != "" || len(again.GetHiddenHits()) != 0 {
			t.Errorf("the answer again = %v, want the player's view of the combat after the master hid the target", whole)
		}
		targets1 := first.GetCast().GetTargets()
		if len(targets1) != 1 || len(targets1[0].GetBonusDice()) != 1 || targets1[0].GetAttackRoll().GetFaces()[0] != 4 {
			t.Errorf("the first cast = %v, want the master's d20 of 4 with the player's d8", first.GetCast())
		}
		if !rt.combatantOf(labelBrisa).GetActionUsed() || rt.logLines() != lines {
			t.Errorf("the log has %d lines after the answer again, want the %d it had; action used = %v", rt.logLines(), lines, rt.combatantOf(labelBrisa).GetActionUsed())
		}
		rt.ask(t, "after the answer again")
		rt.finish(t, watchers)
	}
	t.Run("AnswerHeldRoll", func(t *testing.T) {
		run(t, func(rt *answerTable, hold, key string) (proto.Message, *playv1.CastSpellResponse) {
			out := must(rt.caio.resources.AnswerHeldRoll(rt.t.Context(), rq(&playv1.AnswerHeldRollRequest{CampaignId: rt.campaign, HoldId: hold, IdempotencyKey: key, Answer: dieAnswerOf(8)})))
			cast := out.GetSpellCast()
			return out, cast
		})
	})
	t.Run("AnswerOutsideInspiration", func(t *testing.T) {
		run(t, func(rt *answerTable, hold, key string) (proto.Message, *playv1.CastSpellResponse) {
			out := answerOutsideDie(rt, hold, key, 8)
			cast := out.GetSpellCast()
			return out, cast
		})
	})
}

// answerOutsideDie is Caio's answer to the Bardic Inspiration question through the outside-answer RPC, with the die
// typed from a real die.
func answerOutsideDie(rt *answerTable, hold, key string, face int32) *playv1.AnswerOutsideInspirationResponse {
	return must(rt.caio.resources.AnswerOutsideInspiration(rt.t.Context(), rq(&playv1.AnswerOutsideInspirationRequest{ //nolint:staticcheck // the rows cover the deprecated RPC while it exists
		CampaignId: rt.campaign, HoldId: hold, Use: true, IdempotencyKey: key, Roll: &playv1.AnswerOutsideInspirationRequest_TypedFace{TypedFace: face},
	})))
}
