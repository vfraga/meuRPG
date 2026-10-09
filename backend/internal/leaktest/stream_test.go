package leaktest

import (
	"context"
	"sort"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	progressionv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/progression/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// The live stream (PlayService.WatchGameSession) is a read that never ends: what a player
// receives is what the server pushes. The test opens it as every person, makes the master and the
// players do something of every kind that the stream announces (see streamScript), ends the
// session, and checks each event as it checks an answer. The events are hints that carry no
// content by design (RN-10): this proves that none carries anything, and that a change made
// only to what is hidden announces nothing to the players it does not concern.

// streamWatcher reads one WatchGameSession stream in the background.
type streamWatcher struct {
	p      *person
	events []*playv1.WatchGameSessionResponse
	done   chan struct{}
	err    error
}

// watchStream opens the stream as p and waits for its first event (ready), so everything
// made after it reaches the stream.
func (w *world) watchStream(p *person) *streamWatcher {
	ctx, cancel := context.WithCancel(w.t.Context())
	w.t.Cleanup(cancel)
	stream, err := p.play.WatchGameSession(ctx, connect.NewRequest(&playv1.WatchGameSessionRequest{CampaignId: w.campaign}))
	if err != nil {
		panic(err)
	}
	sw := &streamWatcher{p: p, done: make(chan struct{})}
	first := make(chan struct{})
	go func() {
		defer close(sw.done)
		defer func() { _ = stream.Close() }()
		opened := false
		for stream.Receive() {
			sw.events = append(sw.events, proto.Clone(stream.Msg()).(*playv1.WatchGameSessionResponse))
			if !opened {
				opened = true
				close(first)
			}
		}
		sw.err = stream.Err()
		if !opened {
			close(first)
		}
	}()
	select {
	case <-first:
	case <-time.After(20 * time.Second):
		panic("the stream never sent ready")
	}
	return sw
}

// streamedCases are the kinds of event a stream sends: the oneof of its message.
func streamedCases() []string {
	var out []string
	oneof := (&playv1.WatchGameSessionResponse{}).ProtoReflect().Descriptor().Oneofs().ByName("event")
	for i := range oneof.Fields().Len() {
		out = append(out, string(oneof.Fields().Get(i).Name()))
	}
	sort.Strings(out)
	return out
}

func eventCase(ev *playv1.WatchGameSessionResponse) string {
	fd := ev.ProtoReflect().WhichOneof(ev.ProtoReflect().Descriptor().Oneofs().ByName("event"))
	if fd == nil {
		return ""
	}
	return string(fd.Name())
}

// notTriggered lists the events the script does not make, and why. An event that is neither
// triggered nor here fails TestLeakMatrix/stream: a new kind of event must be exercised or
// explained.
var notTriggered = map[string]string{}

func checkStream(t *testing.T, w *world, got *answers) {
	t.Helper()
	watchers := map[*person]*streamWatcher{}
	for _, p := range []*person{w.master, w.ana, w.caio} {
		watchers[p] = w.watchStream(p)
	}
	// A pending member, a stranger and someone with no session cannot open the stream.
	for _, p := range []*person{w.pending, w.stranger, w.anonymous()} {
		stream, err := p.play.WatchGameSession(t.Context(), connect.NewRequest(&playv1.WatchGameSessionRequest{CampaignId: w.campaign}))
		if err == nil {
			// a Connect stream reports the refusal on the first Receive
			if stream.Receive() {
				t.Errorf("%s opened the live stream and got %v", p.name, stream.Msg())
			}
			err = stream.Err()
			_ = stream.Close()
		}
		if err == nil {
			t.Errorf("%s opened the live stream", p.name)
		}
	}

	step := func(name string, f func()) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("%s: %v", name, r)
			}
		}()
		f()
	}
	step("the stream script", w.streamScript)
	w.assertFogShape(t) // the changes moved two characters: the fog must still be what the canaries assume
	// The state the script left: everything is asked again, with the changes made.
	for _, r := range reads {
		if r.once {
			continue
		}
		t.Run("again after the changes: "+r.name(), func(t *testing.T) { w.runRead(t, r, got) })
	}
	step("the end of the combat", func() {
		must(w.master.combat.EndEncounter(t.Context(), rq(&playv1.EndEncounterRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), IdempotencyKey: newKey()})))
	})
	for _, r := range readsAfterTheCombat {
		t.Run("after the combat: "+r.name(), func(t *testing.T) { w.runRead(t, r, got) })
	}
	step("the end of the session", func() {
		must(w.master.play.EndGameSession(t.Context(), rq(&playv1.EndGameSessionRequest{CampaignId: w.campaign, GameSessionId: w.session})))
	})

	cases := map[string]map[string]int{}
	for p, sw := range watchers {
		select {
		case <-sw.done:
		case <-time.After(30 * time.Second):
			t.Fatalf("%s's stream did not end with the session", p.name)
		}
		cases[p.name] = map[string]int{}
		for _, ev := range sw.events {
			cases[p.name][eventCase(ev)]++
			body, err := protojson.Marshal(ev)
			if err != nil {
				t.Fatalf("marshal an event: %v", err)
			}
			r := reply{status: 200, body: body, msg: ev}
			got.keep(p, r)
			if p == w.master {
				continue
			}
			for _, f := range w.inspect(p, r, nil) {
				t.Errorf("%s: event %s: %s\n\tevent: %s", p.name, eventCase(ev), f, shorten(body))
			}
		}
	}
	for _, c := range streamedCases() {
		total := 0
		for _, n := range cases {
			total += n[c]
		}
		if total == 0 && notTriggered[c] == "" {
			t.Errorf("no stream received a %q event, and the script says nothing about it: make the script cause one, or list it in notTriggered with the reason (what each stream got: %v)", c, cases)
		}
	}
	w.streamSeen = cases
}

// streamScript makes everything the stream announces, short of ending the combat and the
// session. Steps by the master change what is hidden, and what is not; steps by a player make
// their own moves. Each step says what the players must (not) hear of it: nothing but a hint.
func (w *world) streamScript() {
	ctx := w.t.Context()
	m := w.master

	// -- hidden changes: the players hear nothing, or only a hint with no content.
	x, y := at(7, 8)
	must(m.maps.UpdateMapPoint(ctx, rq(&mapsv1.UpdateMapPointRequest{CampaignId: w.campaign, MapId: w.fogMap, PointId: w.pts["trap-hidden"].GetId(), XBp: new(x), YBp: new(y + 100)})))
	must(m.maps.SetMapPointRevealed(ctx, rq(&mapsv1.SetMapPointRevealedRequest{CampaignId: w.campaign, MapId: w.fogMap, PointId: w.pts["treasure-hidden"].GetId(), Revealed: true})))
	must(m.maps.SetMapPointRevealed(ctx, rq(&mapsv1.SetMapPointRevealedRequest{CampaignId: w.campaign, MapId: w.fogMap, PointId: w.pts["treasure-hidden"].GetId(), Revealed: false})))
	w.place(w.fogMap, w.hiddenNPC.GetId(), 21, 8) // a hidden NPC walks in the guard room
	w.place(w.fogMap, w.boss.GetId(), 22, 3)      // a revealed NPC nobody's character sees

	// -- a trap revealed to Caio's character alone, in the south room; Ana hears nothing of it
	must(m.maps.RevealTrap(ctx, rq(&mapsv1.RevealTrapRequest{CampaignId: w.campaign, MapId: w.fogMap, PointId: w.pts["trap-caio"].GetId(), CharacterIds: []string{w.toren.GetId()}})))
	w.allow("trap-caio-name", w.caio)
	w.allow("trap-caio-description", w.caio)
	w.secrets.allowNeedle(w.pts["trap-caio"].GetId(), w.caio)

	// -- Ana's character walks by the trap she can notice: her vision, her token and the trap noticed
	w.place(w.fogMap, w.pens.GetId(), 4, 9)
	w.allow("trap-noticed-name", w.ana)
	w.allow("trap-noticed-description", w.ana)
	w.secrets.allowNeedle(w.pts["trap-noticed"].GetId(), w.ana)

	// -- "Esquecer o que foi visto": every player's memory of the map goes, and each one hears a hint
	must(m.maps.ForgetMapVision(ctx, rq(&mapsv1.ForgetMapVisionRequest{CampaignId: w.campaign, MapId: w.fogMap})))

	// -- the combat: the master moves, hides and hurts what the players may not see
	e := w.encounter
	boss, bandit := w.combatant(w.boss), w.combatant(w.bandit)
	must(m.combat.MoveCombatant(ctx, rq(&playv1.MoveCombatantRequest{CampaignId: w.campaign, EncounterId: e.GetId(), CombatantId: bandit.GetId(), IdempotencyKey: newKey(), Col: 21, Row: 2, Forced: true})))
	// the goblin Ana's character sees walks: her stream hears where, Caio's hears "read it again"
	must(m.combat.MoveCombatant(ctx, rq(&playv1.MoveCombatantRequest{CampaignId: w.campaign, EncounterId: e.GetId(), CombatantId: w.combatant(w.seenNPC).GetId(), IdempotencyKey: newKey(), Col: 5, Row: 7, Forced: true})))
	// Caio's character is moved (by the master: a forced move): in a combat, a player's view follows the combatant
	must(m.combat.MoveCombatant(ctx, rq(&playv1.MoveCombatantRequest{CampaignId: w.campaign, EncounterId: e.GetId(), CombatantId: w.combatant(w.toren).GetId(), IdempotencyKey: newKey(), Col: 12, Row: 14, Forced: true})))
	must(m.combat.AdjustCombatantHitPoints(ctx, rq(&playv1.AdjustCombatantHitPointsRequest{CampaignId: w.campaign, EncounterId: e.GetId(), CombatantId: boss.GetId(), IdempotencyKey: newKey(), Change: &playv1.AdjustCombatantHitPointsRequest_Damage{Damage: 50}})))
	must(m.combat.SetCombatantHidden(ctx, rq(&playv1.SetCombatantHiddenRequest{CampaignId: w.campaign, EncounterId: e.GetId(), CombatantId: boss.GetId(), IdempotencyKey: newKey(), Hidden: true})))
	must(m.combat.AddMonsters(ctx, rq(&playv1.AddMonstersRequest{CampaignId: w.campaign, EncounterId: e.GetId(), IdempotencyKey: newKey(), CreatureKey: "monster:bandit", Count: 1, Name: w.secrets.marker("mon-stream")})))
	for range 3 { // the turn passes: each member gets their own copy of the hint
		cur := must(m.combat.GetEncounter(ctx, rq(&playv1.GetEncounterRequest{CampaignId: w.campaign}))).GetEncounter()
		must(m.combat.EndTurn(ctx, rq(&playv1.EndTurnRequest{CampaignId: w.campaign, EncounterId: e.GetId(), IdempotencyKey: newKey(), ExpectedCombatantId: cur.GetCurrentCombatantId(), ExpectedRound: cur.GetRound()})))
	}

	// -- the scene: the DC changes (everyone hears "changed", never the number), a check is
	// rolled, a clue goes to Caio alone, an NPC comes on the stage
	scene := w.pts["scene-open"]
	for _, a := range must(m.play.GetOpenScene(ctx, rq(&playv1.GetOpenSceneRequest{CampaignId: w.campaign}))).GetScene().GetActions() {
		must(m.maps.UpdateSceneAction(ctx, rq(&mapsv1.UpdateSceneActionRequest{CampaignId: w.campaign, MapId: scene.GetMapId(), PointId: scene.GetId(), ActionId: a.GetId(), Dc: proto.Int32(sceneDC)})))
		must(w.ana.play.RollSceneCheck(ctx, rq(&playv1.RollSceneCheckRequest{CampaignId: w.campaign, ActionId: a.GetId(), IdempotencyKey: newKey(), Roll: &playv1.RollSceneCheckRequest_D20Face{D20Face: 12}})))
	}
	must(m.maps.RevealSceneClue(ctx, rq(&mapsv1.RevealSceneClueRequest{CampaignId: w.campaign, ClueId: w.clueUnrevealed, CharacterIds: []string{w.toren.GetId()}})))
	w.allow("clue-unrevealed", w.caio)
	must(m.play.PutOnStage(ctx, rq(&playv1.PutOnStageRequest{CampaignId: w.campaign, CharacterId: w.stage2.GetId()})))
	must(m.play.SetSpeaker(ctx, rq(&playv1.SetSpeakerRequest{CampaignId: w.campaign, CharacterId: w.stage2.GetId()})))
	w.allow("stage2-name", w.ana, w.caio)
	w.allow("stage2-description")
	w.secrets.allowNeedle(w.stage2.GetId())

	// -- vitals, XP, the creature of Caio, the table's content
	must(m.play.AdjustCharacterVitals(ctx, rq(&playv1.AdjustCharacterVitalsRequest{CampaignId: w.campaign, CharacterId: w.toren.GetId(), IdempotencyKey: newKey(), HitPointsCurrent: proto.Int32(3)})))
	must(m.xp.MarkMilestoneReached(ctx, rq(&progressionv1.MarkMilestoneReachedRequest{CampaignId: w.campaign, MilestoneId: w.milestoneStream, CharacterIds: []string{w.pens.GetId()}, IdempotencyKey: newKey()})))
	w.allow("milestone-stream", w.ana, w.caio)
	w.secrets.allowNeedle(w.milestoneStream, w.ana, w.caio)
	cr := must(m.characters.GiveCreature(ctx, rq(&charactersv1.GiveCreatureRequest{CampaignId: w.campaign, CharacterId: w.toren.GetId(), MonsterKey: "monster:wolf", Name: w.secrets.marker("creature-caio", w.caio)}))).GetCreature()
	w.secrets.id("creature", cr.GetId(), w.caio)
	// an item the master gives Caio's character: the stream says that an inventory changed, not what
	must(m.inventory.GiveItems(ctx, rq(&charactersv1.GiveItemsRequest{CampaignId: w.campaign, CharacterId: w.toren.GetId(), IdempotencyKey: newKey(), Grants: []*charactersv1.ItemGrant{{CatalogKey: "item:cloak-of-protection", Unidentified: true, Look: w.secrets.marker("item-look-stream", w.caio)}}})))
	w.secrets.add(&canary{needle: "item:cloak-of-protection", kind: "item-identity-Caio"})
	// the table's content changes: an entry goes away and comes back
	live := w.keyOf("race-live")
	must(m.table.ArchiveTableEntry(ctx, rq(&rulesv1.ArchiveTableEntryRequest{CampaignId: w.campaign, Key: live})))
	must(m.table.UnarchiveTableEntry(ctx, rq(&rulesv1.UnarchiveTableEntryRequest{CampaignId: w.campaign, Key: live})))

	// -- the puzzles: a hint released, a wheel turned by Ana
	shown := w.puzzles["shown"]
	must(m.puzzles.ReleaseNextPuzzleHint(ctx, rq(&playv1.ReleaseNextPuzzleHintRequest{CampaignId: w.campaign, PuzzleId: shown.GetId()})))
	w.secrets.release("puzzle-hint-2", w.ana, w.caio)
	must(w.ana.puzzles.MakePuzzleMove(ctx, rq(&playv1.MakePuzzleMoveRequest{CampaignId: w.campaign, PuzzleId: shown.GetId(), IdempotencyKey: newKey(), Move: &playv1.PuzzleMove{Kind: &playv1.PuzzleMove_Lock{Lock: &playv1.LockMove{Wheel: 0, Delta: 1}}}})))

	// -- images: the master shows another image, and takes one back from the players
	must(m.play.SetShownImage(ctx, rq(&playv1.SetShownImageRequest{CampaignId: w.campaign, ImageId: w.imgLeft})))
	must(m.play.TakeBackLeftImage(ctx, rq(&playv1.TakeBackLeftImageRequest{CampaignId: w.campaign, ImageId: w.imgLeft})))

	// -- the current map goes to another and back (the other is revealed: the players read its name)
	must(m.maps.SetMapRevealed(ctx, rq(&mapsv1.SetMapRevealedRequest{CampaignId: w.campaign, MapId: w.map2, Revealed: true})))
	w.allow("map2-name", w.ana, w.caio)
	w.secrets.allowNeedle(w.map2, w.ana, w.caio)
	w.secrets.allowNeedle(w.map2Image, w.ana, w.caio)
	must(m.play.SetCurrentMap(ctx, rq(&playv1.SetCurrentMapRequest{CampaignId: w.campaign, MapId: w.map2})))
	must(m.play.SetCurrentMap(ctx, rq(&playv1.SetCurrentMapRequest{CampaignId: w.campaign, MapId: w.fogMap})))
}

var _ = campaignsv1.Role_ROLE_PLAYER
