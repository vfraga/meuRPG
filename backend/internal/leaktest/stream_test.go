package leaktest

import (
	"context"
	"slices"
	"sort"
	"sync"
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
	mu     sync.Mutex // guards events while the stream is open
	events []*playv1.WatchGameSessionResponse
	more   chan struct{} // gets a signal when an event arrives (see settle)
	done   chan struct{}
	err    error
}

// count is how many events the stream has received so far.
func (sw *streamWatcher) count() int {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return len(sw.events)
}

// since lists the events received after the first n.
func (sw *streamWatcher) since(n int) []*playv1.WatchGameSessionResponse {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return slices.Clone(sw.events[min(n, len(sw.events)):])
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
	sw := &streamWatcher{p: p, done: make(chan struct{}), more: make(chan struct{}, 1)}
	first := make(chan struct{})
	go func() {
		defer close(sw.done)
		defer func() { _ = stream.Close() }()
		opened := false
		for stream.Receive() {
			sw.mu.Lock()
			sw.events = append(sw.events, proto.Clone(stream.Msg()).(*playv1.WatchGameSessionResponse))
			sw.mu.Unlock()
			select {
			case sw.more <- struct{}{}:
			default:
			}
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
var notTriggered = map[string]string{
	"character_revived":      "the fixture kills no character: the revive tests of package play check who hears it (the master and the owner)",
	"revivify_changed":       "the fixture has no caster of Revivify: the revivify tests of package play check who hears it (the master and the caster's player)",
	"hidden_hit_pending":     "needs a player's area spell that hits a hidden creature, which has a world of its own: TestAnAreaSpellThatHitsAHiddenCreatureNamesItToNoPlayerBeforeTheReveal (the master hears it, no player does)",
	"reaction_window_opened": "TestAnNPCsReactionWindowIsHeardByTheMasterAlone needs its own combat state and checks the events of both kinds",
	"reaction_window_closed": "same test",
	"held_rolls_changed":     "needs a player's roll that waits for a question, which has a world of its own: TestHeldRollsReachOnlyWhoMayReadThem (the master and the roller hear it, no other player does)",
}

func checkStream(t *testing.T, w *world, got *answers) {
	t.Helper()
	watchers := map[*person]*streamWatcher{}
	for _, p := range []*person{w.master, w.ana, w.caio} {
		watchers[p] = w.watchStream(p)
	}
	// The TV's stream (WatchTableScreen) is the master's tab: it carries no content at all.
	tvw := w.watchTV(w.master)
	checkTVStreamIsRefused(t, w)
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
	// A hidden NPC moves: the TV's stream hears a `changed`, and what the TV reads is the same.
	step("a hidden move on the TV", func() { checkAHiddenMoveShowsNothing(t, w, tvw, got) })
	step("the stream script", w.streamScript)
	w.assertFogShape(t) // the changes moved two characters: the fog must still be what the canaries assume
	step("the table screen after the script", func() { checkTableScreenAfterTheScript(t, w, got) })
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
	select {
	case <-tvw.done:
	case <-time.After(30 * time.Second):
		t.Fatalf("the TV's stream did not end with the session")
	}
	checkTVEvents(t, w, tvw, got)
	checkTableScreenChangedIsTheMasters(t, w, watchers)
	for _, c := range streamedCases() {
		total := 0
		for _, n := range cases {
			total += n[c]
		}
		if total == 0 && notTriggered[c] == "" {
			t.Errorf("no stream received a %q event, and the script says nothing about it: make the script cause one, or list it in notTriggered with the reason (what each stream got: %v)", c, cases)
		}
	}
	w.assertAttackHit(t, watchers)
	w.streamSeen = cases
}

// assertAttackHit checks the leak rules of the attack_hit the script caused (a hidden NPC's hit on Toren, applied by the master): only
// the character's player gets it, once, and the hidden attacker's name is not in it.
func (w *world) assertAttackHit(t *testing.T, watchers map[*person]*streamWatcher) {
	t.Helper()
	for p, sw := range watchers {
		var hits []*playv1.WatchGameSessionResponse_AttackHit
		for _, ev := range sw.events {
			if h := ev.GetAttackHit(); h != nil {
				hits = append(hits, h)
			}
		}
		if p != w.caio {
			if len(hits) != 0 {
				t.Errorf("%s's stream got an attack_hit, which is for the character's player alone: %v", p.name, hits)
			}
			continue
		}
		if len(hits) != 1 || hits[0].GetActorLabel() != "" || hits[0].GetAmount() != attackHitAmount || hits[0].GetId() == "" {
			t.Errorf("%s's stream got %v, want one attack_hit of %d with no attacker's name (the bandit is hidden)", p.name, hits, attackHitAmount)
		}
	}
}

// attackHitAmount is what the master applies in the script's hit.
const attackHitAmount = 2

// hiddenBanditHitsToren has the hidden bandit hit Toren and the master apply the damage (ApplyPendingDamage): the damage the
// target's player is told of as an attack_hit.
func (w *world) hiddenBanditHitsToren(bandit, toren *playv1.Combatant) {
	ctx := w.t.Context()
	m := w.master
	// The bandit reacts: it must not be in the group whose turn it is (a joint turn is a tie of totals).
	cur := must(m.combat.GetEncounter(ctx, rq(&playv1.GetEncounterRequest{CampaignId: w.campaign}))).GetEncounter()
	if cur.GetCurrentCombatantId() == bandit.GetId() {
		w.t.Fatalf("the bandit has the turn (round %d), so it cannot react: %s", cur.GetRound(), initiativeOrder(cur))
	}
	opts := must(m.combat.GetTurnOptions(ctx, rq(&playv1.GetTurnOptionsRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), CombatantId: bandit.GetId()})))
	key := ""
	for _, a := range opts.GetOptions().GetAttacks() {
		if a.GetAttack().GetKey() != "" && a.GetAttack().GetSaveDc() == 0 {
			key = a.GetAttack().GetKey()
			break
		}
	}
	if key == "" {
		w.t.Fatal("the bandit has no attack to make")
	}
	hit := must(m.combat.RollAttack(ctx, rq(&playv1.RollAttackRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), AttackerId: bandit.GetId(), AttackKey: key, TargetId: toren.GetId(), IdempotencyKey: newKey(),
		D20Faces: []int32{19, 18}, AsReaction: true, // Toren is down at this point of the script: the roll takes two d20
	})))
	if hit.GetPendingDamage().GetId() == "" {
		w.t.Fatalf("the bandit's attack = %v, want a hit", hit.GetRoll())
	}
	must(m.combat.RollDamage(ctx, rq(&playv1.RollDamageRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), PendingDamageId: hit.GetPendingDamage().GetId(), IdempotencyKey: newKey(), Roll: &playv1.RollDamageRequest_RollInApp{RollInApp: true}})))
	must(m.combat.ApplyPendingDamage(ctx, rq(&playv1.ApplyPendingDamageRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), PendingDamageId: hit.GetPendingDamage().GetId(), IdempotencyKey: newKey(), Amount: proto.Int32(attackHitAmount),
	})))
}

// streamScript makes everything the stream announces, short of ending the combat and the
// session. Steps by the master change what is hidden, and what is not; steps by a player make
// their own moves. Each step says what the players must (not) hear of it: nothing but a hint.
func (w *world) streamScript() {
	ctx := w.t.Context()
	m := w.master

	// -- the pending member sends her character again and the master asks again: the master hears
	// of both, as a hint with no reason; no player and no stream of anyone else does.
	must(w.pending.characters.ResubmitCharacter(ctx, rq(&charactersv1.ResubmitCharacterRequest{CampaignId: w.campaign, CharacterId: w.pendingHero.GetId(), IdempotencyKey: newKey()})))
	must(m.characters.RequestCharacterChanges(ctx, rq(&charactersv1.RequestCharacterChangesRequest{
		CampaignId: w.campaign, CharacterId: w.pendingHero.GetId(), Reason: w.reviewReason, IdempotencyKey: newKey(),
	})))

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
	w.scriptD20(ownFace)
	added := must(m.combat.AddMonsters(ctx, rq(&playv1.AddMonstersRequest{CampaignId: w.campaign, EncounterId: e.GetId(), IdempotencyKey: newKey(), CreatureKey: "monster:bandit", Count: 1, Name: w.secrets.marker("mon-stream")})))
	w.requireOwnTotal(added.GetCombatantIds()[0])
	// the hidden bandit hits Toren and the master applies the damage: Caio's stream alone hears an attack_hit, with no name
	// for the attacker (it is hidden, and its line in the log is the master's); Ana's stream and the master's hear none
	w.hiddenBanditHitsToren(bandit, w.combatant(w.toren))
	for range 3 { // the turn passes: each member gets their own copy of the hint
		cur := must(m.combat.GetEncounter(ctx, rq(&playv1.GetEncounterRequest{CampaignId: w.campaign}))).GetEncounter()
		must(m.combat.EndTurn(ctx, rq(&playv1.EndTurnRequest{CampaignId: w.campaign, EncounterId: e.GetId(), IdempotencyKey: newKey(), ExpectedCombatantId: cur.GetCurrentCombatantId(), ExpectedRound: cur.GetRound()})))
	}

	// -- the scene: the DC changes (everyone hears "changed", never the number), a check is
	// rolled, a clue goes to Caio alone, an NPC comes on the stage
	scene := w.pts["scene-open"]
	for _, a := range must(m.play.GetOpenScene(ctx, rq(&playv1.GetOpenSceneRequest{CampaignId: w.campaign}))).GetScene().GetActions() {
		must(m.maps.UpdateSceneAction(ctx, rq(&mapsv1.UpdateSceneActionRequest{CampaignId: w.campaign, MapId: scene.GetMapId(), PointId: scene.GetId(), ActionId: a.GetId(), Dc: proto.Int32(sceneDC)})))
		if a.GetHidden() {
			continue // the players cannot roll it: they do not know it (the edit above reached the master's stream alone)
		}
		must(w.ana.play.RollSceneCheck(ctx, rq(&playv1.RollSceneCheckRequest{CampaignId: w.campaign, ActionId: a.GetId(), IdempotencyKey: newKey(), Roll: &playv1.RollSceneCheckRequest_D20Face{D20Face: 12}})))
	}
	must(m.maps.RevealSceneClue(ctx, rq(&mapsv1.RevealSceneClueRequest{CampaignId: w.campaign, ClueId: w.clueUnrevealed, CharacterIds: []string{w.toren.GetId()}})))
	w.allow("clue-unrevealed", w.caio)
	must(m.play.PutOnStage(ctx, rq(&playv1.PutOnStageRequest{CampaignId: w.campaign, CharacterId: w.stage2.GetId()})))
	must(m.play.SetSpeaker(ctx, rq(&playv1.SetSpeakerRequest{CampaignId: w.campaign, CharacterId: w.stage2.GetId()})))
	w.allow("stage2-name", w.ana, w.caio)
	w.allow("stage2-description")
	w.secrets.allowNeedle(w.stage2.GetId())

	// -- the hidden stage NPC and the hidden action: shown and hidden again, the players hear a hint with no content
	// (the NPC and the action are gone from what they read again at the end); the edit of a hidden action reached nobody
	must(m.play.SetStageNpcHidden(ctx, rq(&playv1.SetStageNpcHiddenRequest{CampaignId: w.campaign, CharacterId: w.hiddenStageNPC.GetId(), Hidden: false})))
	must(m.play.SetStageNpcHidden(ctx, rq(&playv1.SetStageNpcHiddenRequest{CampaignId: w.campaign, CharacterId: w.hiddenStageNPC.GetId(), Hidden: true})))
	for _, hidden := range []bool{false, true} {
		must(m.maps.UpdateSceneAction(ctx, rq(&mapsv1.UpdateSceneActionRequest{CampaignId: w.campaign, MapId: scene.GetMapId(), PointId: scene.GetId(), ActionId: w.hiddenAction, Hidden: proto.Bool(hidden)})))
	}

	// -- a spell cast outside the combat by an NPC that is not on the stage: the hint is the master's alone
	must(m.casting.CastSpellOutsideCombat(ctx, rq(&playv1.CastSpellOutsideCombatRequest{
		CampaignId: w.campaign, CasterCharacterId: w.casterNPC.GetId(), SpellKey: "spell:mage-armor", Slot: &playv1.SpellSlot{Level: 1},
		TargetIds: []string{w.toren.GetId()}, IdempotencyKey: newKey(),
	})))

	// -- vitals, XP, the creature of Caio, the table's content
	must(m.play.AdjustCharacterVitals(ctx, rq(&playv1.AdjustCharacterVitalsRequest{CampaignId: w.campaign, CharacterId: w.toren.GetId(), IdempotencyKey: newKey(), ExpectedRevision: m.revisionOf(w, w.toren.GetId()), HitPointsCurrent: proto.Int32(3)})))
	must(m.xp.MarkMilestoneReached(ctx, rq(&progressionv1.MarkMilestoneReachedRequest{CampaignId: w.campaign, MilestoneId: w.milestoneStream, CharacterIds: []string{w.pens.GetId()}, IdempotencyKey: newKey()})))
	w.allow("milestone-stream", w.ana, w.caio)
	w.secrets.allowNeedle(w.milestoneStream, w.ana, w.caio)
	cr := must(m.characters.GiveCreature(ctx, rq(&charactersv1.GiveCreatureRequest{CampaignId: w.campaign, CharacterId: w.toren.GetId(), MonsterKey: "monster:wolf", Name: w.secrets.marker("creature-caio", w.caio)}))).GetCreature()
	w.secrets.id("creature", cr.GetId(), w.caio)
	// the table's content changes: an entry goes away and comes back
	live := w.keyOf("race-live")
	must(m.table.ArchiveTableEntry(ctx, rq(&rulesv1.ArchiveTableEntryRequest{CampaignId: w.campaign, Key: live})))
	must(m.table.UnarchiveTableEntry(ctx, rq(&rulesv1.UnarchiveTableEntryRequest{CampaignId: w.campaign, Key: live})))

	// -- the puzzles: a hint released, a wheel turned by Ana
	shown := w.puzzles["shown"]
	must(m.puzzles.ReleaseNextPuzzleHint(ctx, rq(&playv1.ReleaseNextPuzzleHintRequest{CampaignId: w.campaign, PuzzleId: shown.GetId()})))
	w.secrets.release("puzzle-hint-2", w.ana, w.caio)
	must(w.ana.puzzles.MakePuzzleMove(ctx, rq(&playv1.MakePuzzleMoveRequest{CampaignId: w.campaign, PuzzleId: shown.GetId(), IdempotencyKey: newKey(), Move: &playv1.PuzzleMove{Kind: &playv1.PuzzleMove_Lock{Lock: &playv1.LockMove{Wheel: 0, Delta: 1}}}})))

	// -- the group check: the master rolls for Ana's character, closes it and asks another; Caio rolls his
	// character's: every stream hears a hint with no content, and a player's own roll only the master and the player
	open := must(m.contests.GetGroupCheck(ctx, rq(&playv1.GetGroupCheckRequest{CampaignId: w.campaign}))).GetGroupCheck()
	must(m.contests.RollForPlayer(ctx, rq(&playv1.RollForPlayerRequest{CampaignId: w.campaign, IdempotencyKey: newKey(), GroupCheckId: open.GetId(), CharacterId: w.pens.GetId(), Roll: appRoll()})))
	must(m.contests.CloseGroupCheck(ctx, rq(&playv1.CloseGroupCheckRequest{CampaignId: w.campaign, IdempotencyKey: newKey(), GroupCheckId: open.GetId()})))
	next := must(m.contests.RequestGroupCheck(ctx, rq(&playv1.RequestGroupCheckRequest{CampaignId: w.campaign, IdempotencyKey: newKey(), SkillKey: "skill:stealth", Dc: groupCheckDC, ShowDc: true}))).GetGroupCheck()
	must(w.caio.contests.RollGroupCheck(ctx, rq(&playv1.RollGroupCheckRequest{CampaignId: w.campaign, IdempotencyKey: newKey(), GroupCheckId: next.GetId(), Roll: appRoll()})))

	// -- the nudge: the master calls Ana's character; only Ana's stream hears it, an event with no content
	must(m.play.NudgePlayer(ctx, rq(&playv1.NudgePlayerRequest{CampaignId: w.campaign, CharacterId: w.pens.GetId()})))

	// -- the conversation: a line for Ana alone reaches her stream and the master's, not Caio's (RN-36); a hint
	// that the master is typing goes to the players, with no content
	line := w.secrets.marker("line-live", w.ana)
	must(m.play.SendSessionMessage(ctx, rq(&playv1.SendSessionMessageRequest{CampaignId: w.campaign, SpeakerKind: playv1.SessionMessageSpeakerKind_SESSION_MESSAGE_SPEAKER_KIND_NARRATOR, Text: line, ReaderUserIds: []string{w.ana.id}, IdempotencyKey: newKey()})))
	must(m.play.SignalMasterTyping(ctx, rq(&playv1.SignalMasterTypingRequest{CampaignId: w.campaign})))

	// -- the free dice roller: the master rolls "Só eu" and Ana rolls for the table; every stream
	// hears Ana's roll and none hears the master's
	w.freeRollsOnTheStream()

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
