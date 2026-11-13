package leaktest

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1/playv1connect"
)

// Legendary Resistance on the save of a hit rider: Caio's monk (Stunning Strike, level 5, Wisdom 20: the ki save
// DC is 16) hits the adult red dragon, and the answer to the rider waits for the master whatever the dragon's
// Constitution save rolled (+13: a 1 fails it). The window, the offer, the failed roll, the use spent, the uses left
// and the word of the trait are the master's alone; the players read the wait any held action gives and, after, an
// ordinary save line (RN-20, RN-29).

// stunningStrikeFight makes Caio's Toren a monk of level 5, puts him beside the adult red dragon on his turn, has him
// hit it and returns the open Stunning Strike offer with the two combatants.
func (w *world) stunningStrikeFight() (toren, dragon *playv1.Combatant, rider string) {
	w.t.Helper()
	return w.monkStrikeFight(false)
}

// monkStrikeFight is stunningStrikeFight for a monk of the Open Hand when openHand is set.
func (w *world) monkStrikeFight(openHand bool) (toren, dragon *playv1.Combatant, rider string) {
	w.t.Helper()
	ctx := w.t.Context()
	m := w.master
	sheet := w.fullSheet("class:monk", "race:human", 5)
	sheet.GetFull().BaseScores.Dexterity = 16
	sheet.GetFull().BaseScores.Wisdom = 20
	if openHand {
		sheet.GetFull().Classes[0].Subclass = &charactersv1.ClassLevel_SubclassKey{SubclassKey: "subclass:open-hand"}
	}
	must(m.characters.UpdateCharacter(ctx, rq(&charactersv1.UpdateCharacterRequest{
		CampaignId: w.campaign, CharacterId: w.toren.GetId(), Revision: w.toren.GetRevision(), Name: w.toren.GetName(), Sheet: sheet,
	})))
	for _, c := range m.currentEncounter(w).GetCombatants() {
		if c.GetBestiaryCreatureKey() == dragonKey {
			dragon = c
		}
	}
	toren = w.combatant(w.toren)
	w.placeForced(m, w.encounter.GetId(), placement{toren.GetId(), 12, 13}, placement{dragon.GetId(), 13, 13})
	for range 40 {
		cur := must(m.combat.GetEncounter(ctx, rq(&playv1.GetEncounterRequest{CampaignId: w.campaign}))).GetEncounter()
		if cur.GetCurrentCombatantId() == toren.GetId() {
			w.encounter = cur
			break
		}
		must(m.combat.EndTurn(ctx, rq(&playv1.EndTurnRequest{CampaignId: w.campaign, EncounterId: cur.GetId(), IdempotencyKey: newKey(), ExpectedCombatantId: cur.GetCurrentCombatantId(), ExpectedRound: cur.GetRound()})))
	}
	if w.encounter.GetCurrentCombatantId() != toren.GetId() {
		w.t.Fatal("Toren's turn never came")
	}
	strike := must(w.caio.combat.RollAttack(ctx, rq(&playv1.RollAttackRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), AttackerId: toren.GetId(), AttackKey: "attack:unarmed-strike", TargetId: dragon.GetId(),
		IdempotencyKey: newKey(), Roll: &playv1.RollAttackRequest_D20Face{D20Face: 18},
	})))
	// The strike's damage is dropped: it would keep the dragon in the combat when the master takes it out.
	must(m.combat.DiscardPendingDamage(ctx, rq(&playv1.DiscardPendingDamageRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), PendingDamageId: strike.GetPendingDamage().GetId(), IdempotencyKey: newKey(),
	})))
	for _, o := range must(w.caio.combat.GetEncounter(ctx, rq(&playv1.GetEncounterRequest{CampaignId: w.campaign}))).GetEncounter().GetHitRiders() {
		if o.GetKind() == playv1.HitRiderKind_HIT_RIDER_KIND_STUNNING_STRIKE {
			rider = o.GetId()
		}
	}
	if rider == "" {
		w.t.Fatal("the strike left no Stunning Strike offer")
	}
	return toren, dragon, rider
}

// stunCall is Caio's answer to the Stunning Strike offer.
func (w *world) stunCall(rider string) reply {
	return w.caio.call(playv1connect.CombatServiceUseHitRiderProcedure, &playv1.UseHitRiderRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), RiderId: rider, Choice: playv1.HitRiderChoice_HIT_RIDER_CHOICE_STUN, IdempotencyKey: newKey(),
	})
}

// riderScene is a hit rider of an attack on the adult red dragon, open and waiting for its answer: the dragon, the answer
// to the rider (a call as whoever gives it), the player who gives it (nil when the master does) and the player whose log
// holds the line of the rider.
type riderScene struct {
	dragon *playv1.Combatant
	answer func() reply
	owner  *person
	reader *person
}

// stunScene is Caio's Stunning Strike at the dragon.
func (w *world) stunScene() riderScene {
	w.t.Helper()
	_, dragon, rider := w.stunningStrikeFight()
	return riderScene{dragon: dragon, answer: func() reply { return w.stunCall(rider) }, owner: w.caio, reader: w.caio}
}

// checkWindowStream checks the stream of a master's Legendary Resistance window: the master hears it open and close (the
// positive control); the players hear neither, nor any hint of the reaction.
func (w *world) checkWindowStream(t *testing.T, got map[string][]string) {
	t.Helper()
	count := func(name string) (opened, closed int) {
		for _, ev := range got[name] {
			switch {
			case strings.HasPrefix(ev, "reaction_window_opened"):
				opened++
			case strings.HasPrefix(ev, "reaction_window_closed"):
				closed++
			}
		}
		return opened, closed
	}
	if o, c := count(w.master.name); o < 1 || c < 1 {
		t.Errorf("the master heard %d openings and %d closings, want at least one of each (%v)", o, c, got[w.master.name])
	}
	for _, p := range []*person{w.ana, w.caio} {
		if o, c := count(p.name); o != 0 || c != 0 {
			t.Errorf("%s heard %d openings and %d closings of the master's window", p.name, o, c)
		}
		for _, ev := range got[p.name] {
			if strings.Contains(ev, "reaction") {
				t.Errorf("%s heard %s: the hold sends the players no hint of its own", p.name, ev)
			}
		}
	}
}

// legendaryWindow is the master's Legendary Resistance window, or nil.
func (w *world) legendaryWindow() *playv1.ReactionWindow {
	for _, rw := range must(w.master.combat.GetEncounter(w.t.Context(), rq(&playv1.GetEncounterRequest{CampaignId: w.campaign}))).GetEncounter().GetReactionWindows() {
		if rw.GetKind() == playv1.ReactionKind_REACTION_KIND_LEGENDARY_RESISTANCE {
			return rw
		}
	}
	return nil
}

// The dragon fails the save of the Stunning Strike with a 1 and the master turns it into a success. No player reads the
// window, the offer, the failed roll, the count or the trait in the combat, the log, the answers they hold or the
// stream; Caio reads a save that passed with no number; a player's try to answer the window is refused like an
// invented window's. The master reads all of it (the positive controls).
func TestARidersLegendaryResistanceIsTheMastersAlone(t *testing.T) {
	checkRiderLeak(t, (*world).stunScene)
}

// checkRiderLeak plays the leak test of one rider scene: see TestARidersLegendaryResistanceIsTheMastersAlone.
func checkRiderLeak(t *testing.T, build func(*world) riderScene) {
	t.Helper()
	w := newWorld(t)
	for _, word := range append([]string{"REACTION_KIND_LEGENDARY_RESISTANCE", "legendaryResistance", "Usar Resistência Lendária"}, resistanceWords...) {
		w.secrets.add(&canary{needle: word, kind: "legendary-resistance-rider-window"})
	}
	scene := build(w)
	dragon := scene.dragon
	inSight := []string{"dragon-name", "seen-npc-name", "combatant-id"}
	w.scriptD20(1)

	var answer, masterLog reply
	var window *playv1.ReactionWindow
	var held, asked, masterReads []reply
	after := map[*person][]reply{}
	var line *playv1.CombatLogHitRider
	got := w.hiddenOnlyEvents(func() {
		held = append(held, scene.answer())
		if !held[0].ok() {
			t.Fatalf("the answer to the rider: status %d %s", held[0].status, held[0].body)
		}
		if window = w.legendaryWindow(); window == nil {
			t.Fatal("the master reads no Legendary Resistance window")
		}
		asked = []reply{w.reads(w.ana)[0], w.reads(w.caio)[0]}
		for _, p := range []*person{w.ana, w.caio} { // the window is the master's: a player's combat lists none, whatever its kind
			if got := p.currentEncounter(w).GetReactionWindows(); len(got) != 0 {
				t.Errorf("%s reads the windows %v: the dragon's question is the master's alone", p.name, got)
			}
		}
		for _, p := range []*person{w.ana, w.caio} {
			asks := p.call(playv1connect.CombatServiceAnswerReactionProcedure, &playv1.AnswerReactionRequest{
				CampaignId: w.campaign, EncounterId: w.encounter.GetId(), WindowId: window.GetId(), Answer: playv1.ReactionChoice_REACTION_CHOICE_USE,
				CreatureIds: []string{dragon.GetId()}, IdempotencyKey: newKey(),
			})
			fake := p.call(playv1connect.CombatServiceAnswerReactionProcedure, &playv1.AnswerReactionRequest{
				CampaignId: w.campaign, EncounterId: w.encounter.GetId(), WindowId: newKey(), Answer: playv1.ReactionChoice_REACTION_CHOICE_USE,
				CreatureIds: []string{dragon.GetId()}, IdempotencyKey: newKey(),
			})
			if asks.ok() || fake.ok() || asks.status != fake.status || !bytes.Equal(asks.body, fake.body) {
				t.Errorf("%s: a real window is refused with %d %s, an invented one with %d %s: they must read the same", p.name, asks.status, asks.body, fake.status, fake.body)
			}
		}
		answer = w.master.call(playv1connect.CombatServiceAnswerReactionProcedure, &playv1.AnswerReactionRequest{
			CampaignId: w.campaign, EncounterId: w.encounter.GetId(), WindowId: window.GetId(), Answer: playv1.ReactionChoice_REACTION_CHOICE_USE,
			CreatureIds: []string{dragon.GetId()}, IdempotencyKey: newKey(),
		})
		if !answer.ok() {
			t.Fatalf("the master's answer: status %d %s", answer.status, answer.body)
		}
		for _, p := range []*person{w.ana, w.caio} {
			after[p] = w.reads(p)
		}
		masterReads = w.reads(w.master)
		masterLog = w.master.call(playv1connect.CombatServiceListCombatLogProcedure, &playv1.ListCombatLogRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId()})
		for _, e := range w.logOfPerson(scene.reader) {
			if r := e.GetHitRider(); r != nil && r.GetResult() != playv1.HitRiderLogResult_HIT_RIDER_LOG_RESULT_DECLINED {
				line = r
			}
		}
	})
	for _, p := range []*person{w.ana, w.caio} {
		reads := slices.Concat(after[p], asked)
		if p == scene.owner { // the answer to the rider is its giver's own to read
			reads = append(reads, held...)
		}
		for _, r := range reads {
			for _, f := range w.inspect(p, r, inSight) {
				t.Errorf("%s: %s", p.name, f)
			}
		}
	}
	w.checkWindowStream(t, got)
	// The positive controls: the master reads the offer, the choice and the count; the player reads a save that passed.
	master := slices.Concat(masterReads, []reply{answer})
	for _, word := range []string{"resisted", "resistedLeft"} {
		found := false
		for _, r := range master {
			found = found || contains(r.body, word)
		}
		if !found {
			t.Errorf("the master's answers do not hold %q: the control is missing", word)
		}
	}
	if !contains(masterLog.body, "resisted") {
		t.Error("the master's log does not hold the choice: the control is missing")
	}
	if line == nil || line.GetResult() != playv1.HitRiderLogResult_HIT_RIDER_LOG_RESULT_SAVED || line.GetResisted() || line.GetNumbers() || line.GetD20() != 0 {
		t.Errorf("%s's rider line = %v, want a save that passed with no number and no trait", scene.reader.name, line)
	}
}

// riderRead is everything a player can read of a Stunning Strike at the dragon that waits for the master and then
// lands: the combat and the log while it waits and after the answer, the answer to the rider itself and the stream
// hints, as the player's own text.
func riderRun(t *testing.T, build func(*world) riderScene, face int, answer heldAnswer) heldCastRead {
	t.Helper()
	w := newWorld(t)
	ctx := t.Context()
	scene := build(w)
	dragon := scene.dragon
	w.scriptD20(face)
	players := []*person{w.ana, w.caio}
	parts := map[string][]reply{}
	out := heldCastRead{player: map[string]string{}}
	got := w.hiddenOnlyEvents(func() {
		call := scene.answer()
		if !call.ok() {
			t.Fatalf("the answer to the rider: status %d %s", call.status, call.body)
		}
		if scene.owner != nil {
			parts[scene.owner.name] = append(parts[scene.owner.name], call)
		}
		out.window = w.legendaryWindow()
		for _, p := range players {
			parts[p.name] = append(parts[p.name], w.reads(p)...) // while it waits
		}
		if out.window == nil {
			t.Error("the master reads no Legendary Resistance window")
			return
		}
		if answer == removeDragon {
			must(w.master.combat.RemoveCombatant(ctx, rq(&playv1.RemoveCombatantRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), CombatantId: dragon.GetId(), IdempotencyKey: newKey()})))
			for _, p := range players {
				parts[p.name] = append(parts[p.name], w.reads(p)...)
			}
			out.master = w.reads(w.master)
			return
		}
		req := &playv1.AnswerReactionRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), WindowId: out.window.GetId(), Answer: playv1.ReactionChoice_REACTION_CHOICE_PASS, IdempotencyKey: newKey()}
		if answer == useTrait {
			req.Answer, req.CreatureIds = playv1.ReactionChoice_REACTION_CHOICE_USE, []string{dragon.GetId()}
		}
		resp := w.master.call(playv1connect.CombatServiceAnswerReactionProcedure, req)
		if !resp.ok() {
			t.Fatalf("the master's answer: status %d %s", resp.status, resp.body)
		}
		for _, p := range players {
			parts[p.name] = append(parts[p.name], w.reads(p)...) // after the answer
		}
		out.master = slices.Concat(w.reads(w.master), []reply{resp})
	})
	for _, p := range players {
		var text strings.Builder
		for _, r := range parts[p.name] {
			text.Write(r.body)
			text.WriteString("\n")
		}
		text.WriteString(strings.Join(got[p.name], "\n"))
		out.player[p.name] = normalizeRead(text.String())
	}
	return out
}

// A dragon with a use left holds the Stunning Strike whatever it rolled, and the players read the same thing either way:
// the same wait, the same log, the same answer to the rider and the same stream hints, whether the save passed on its own
// or failed and the master turned it into a success. Only the ids and the timestamps are normalised. The master reads the
// two runs differently (the positive controls: a failed roll to offer and a use spent, or nothing to offer).
func TestAPassedAndAResistedRiderSaveReadTheSameToThePlayers(t *testing.T) {
	checkPassedAndResistedRider(t, (*world).stunScene)
}

// checkPassedAndResistedRider plays the two runs of one rider scene: see TestAPassedAndAResistedRiderSaveReadTheSameToThePlayers.
func checkPassedAndResistedRider(t *testing.T, build func(*world) riderScene) {
	t.Helper()
	var passed, resisted heldCastRead
	t.Run("the save passes", func(t *testing.T) { passed = riderRun(t, build, 20, passWindow) })
	t.Run("the save fails and the master uses the trait", func(t *testing.T) { resisted = riderRun(t, build, 1, useTrait) })
	for _, name := range []string{"Ana", "Caio"} {
		if passed.player[name] != resisted.player[name] {
			a, b := passed.player[name], resisted.player[name]
			at := 0
			for at < len(a) && at < len(b) && a[at] == b[at] {
				at++
			}
			from := max(at-200, 0)
			t.Errorf("%s reads a passed save and a resisted one differently, first at byte %d:\n--- passed\n…%s\n--- resisted\n…%s",
				name, at, a[from:min(at+300, len(a))], b[from:min(at+300, len(b))])
		}
	}
	if !strings.Contains(passed.player["Caio"], "Esperando o mestre") {
		t.Error("Caio reads no wait in the run where the save passed: the control is missing")
	}
	if n := len(passed.window.GetLegendaryResistance().GetOffers()); n != 0 {
		t.Errorf("the master's window of the passed save offers %d saves, want none", n)
	}
	if n := len(resisted.window.GetLegendaryResistance().GetOffers()); n != 1 {
		t.Errorf("the master's window of the failed save offers %d saves, want one", n)
	}
	for name, run := range map[string]heldCastRead{"passed": passed, "resisted": resisted} {
		spent := false
		for _, r := range run.master {
			spent = spent || contains(r.body, `"resisted":true`)
		}
		if spent != (name == "resisted") {
			t.Errorf("the master's reads of the %s run show a use spent = %v", name, spent)
		}
	}
}

// The master takes the dragon out of the combat while the rider waits: the window and the wait end whatever the dragon
// rolled, so the players read the same thing after the removal for a passed save and for a failed one.
func TestARemovedDragonEndsThePassedAndTheFailedRiderWaitAlike(t *testing.T) {
	var passed, failed heldCastRead
	t.Run("the save passes", func(t *testing.T) { passed = riderRun(t, (*world).stunScene, 20, removeDragon) })
	t.Run("the save fails", func(t *testing.T) { failed = riderRun(t, (*world).stunScene, 1, removeDragon) })
	for _, name := range []string{"Ana", "Caio"} {
		if passed.player[name] != failed.player[name] {
			t.Errorf("%s reads the removal after a passed save and after a failed one differently:\n--- passed\n%s\n--- failed\n%s", name, passed.player[name], failed.player[name])
		}
	}
	if !strings.Contains(failed.player["Caio"], "Esperando o mestre") {
		t.Error("Caio reads no wait before the removal: the control is missing")
	}
}
