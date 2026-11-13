package leaktest

import (
	"bytes"
	"maps"
	"slices"
	"strings"
	"testing"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1/playv1connect"
)

// Legendary Resistance on the save of a spell zone: Ana's cleric of level 5 (Wisdom 16: the save DC is 14) casts Spirit
// Guardians next to the adult red dragon (Wisdom save +7), and the zone catches it at the start of its turn. The dragon has the
// trait and uses left, so the save is asked of the master like any effect save the master rolls for a creature, in a
// window of the master's alone. The window, the roll that failed, the use spent, the uses left and the word of the trait
// are the master's; the players read the wait any window gives and, after, an ordinary save line (RN-20, RN-29).

// auraFight makes Ana's Pensantus a cleric of level 5 who prepared Spirit Guardians, puts her next to the adult
// red dragon, and passes the turns until it is hers. It returns her combatant and the dragon's.
func (w *world) auraFight() (pens, dragon *playv1.Combatant) {
	w.t.Helper()
	ctx := w.t.Context()
	m := w.master
	sheet := w.fullSheet("class:cleric", "race:gnome", 5)
	sheet.GetFull().BaseScores.Wisdom = 16
	sheet.GetFull().PreparedSpellKeys = []string{"spell:spirit-guardians"}
	must(m.characters.UpdateCharacter(ctx, rq(&charactersv1.UpdateCharacterRequest{
		CampaignId: w.campaign, CharacterId: w.pens.GetId(), Revision: w.pens.GetRevision(), Name: w.pens.GetName(), Sheet: sheet,
	})))
	for _, c := range m.currentEncounter(w).GetCombatants() {
		if c.GetBestiaryCreatureKey() == dragonKey {
			dragon = c
		}
	}
	pens = w.combatant(w.pens)
	w.placeForced(m, w.encounter.GetId(), placement{pens.GetId(), 12, 13}, placement{dragon.GetId(), 13, 13})
	for range 40 {
		cur := must(m.combat.GetEncounter(ctx, rq(&playv1.GetEncounterRequest{CampaignId: w.campaign}))).GetEncounter()
		if cur.GetCurrentCombatantId() == pens.GetId() {
			w.encounter = cur
			break
		}
		must(m.combat.EndTurn(ctx, rq(&playv1.EndTurnRequest{CampaignId: w.campaign, EncounterId: cur.GetId(), IdempotencyKey: newKey(), ExpectedCombatantId: cur.GetCurrentCombatantId(), ExpectedRound: cur.GetRound()})))
	}
	if w.encounter.GetCurrentCombatantId() != pens.GetId() {
		w.t.Fatal("Pensantus's turn never came")
	}
	return pens, dragon
}

// auraCast has Ana cast Spirit Guardians around Pensantus.
func (w *world) auraCast(pens *playv1.Combatant) reply {
	w.t.Helper()
	return w.ana.call(playv1connect.CombatServiceCastSpellProcedure, &playv1.CastSpellRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), CasterId: pens.GetId(), SpellKey: "spell:spirit-guardians", Slot: &playv1.SpellSlot{Level: 3}, IdempotencyKey: newKey(),
		Area: &playv1.CastSpellRequest_Origin{Origin: &playv1.SpellOrigin{Col: 12, Row: 13}},
	})
}

// zoneWindow is the master's question of the dragon's save against the zone, or nil.
func (w *world) zoneWindow() *playv1.ReactionWindow {
	for _, rw := range must(w.master.combat.GetEncounter(w.t.Context(), rq(&playv1.GetEncounterRequest{CampaignId: w.campaign}))).GetEncounter().GetReactionWindows() {
		if rw.GetKind() == playv1.ReactionKind_REACTION_KIND_EFFECT_SAVE && rw.GetEffectSave().GetZoneId() != "" {
			return rw
		}
	}
	return nil
}

// untilAsked passes the turns, as the master, until the zone asks the dragon's save: the turn that begins waits for it.
func (w *world) untilAsked() *playv1.ReactionWindow {
	w.t.Helper()
	ctx := w.t.Context()
	for range 40 {
		if rw := w.zoneWindow(); rw != nil {
			return rw
		}
		cur := must(w.master.combat.GetEncounter(ctx, rq(&playv1.GetEncounterRequest{CampaignId: w.campaign}))).GetEncounter()
		if _, err := w.master.combat.EndTurn(ctx, rq(&playv1.EndTurnRequest{CampaignId: w.campaign, EncounterId: cur.GetId(), IdempotencyKey: newKey(), ExpectedCombatantId: cur.GetCurrentCombatantId(), ExpectedRound: cur.GetRound(), DiscardPendingDamage: true})); err != nil {
			w.t.Fatalf("EndTurn: %v; windows: %v", err, cur.GetReactionWindows())
		}
	}
	w.t.Fatal("the zone never asked the dragon's save")
	return nil
}

// zoneSave is a person's answer to the window: the d20 typed, with the choice of the trait when use is set.
func (w *world) zoneSave(p *person, window string, face int32, use bool) reply {
	return p.call(playv1connect.LastingEffectServiceRollEffectSaveProcedure, &playv1.RollEffectSaveRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), WindowId: window, IdempotencyKey: newKey(),
		Roll: &playv1.RollEffectSaveRequest_D20Face{D20Face: face}, UseLegendaryResistance: use,
	})
}

// The dragon fails the save of the aura with a 1 and the master turns it into a success. No player reads the window, the
// failed roll, the count or the trait in the combat, the log, the answers they hold or the stream; Ana reads a save that
// passed with no number; a player's try to answer the window is refused like an invented window's. The master reads all of
// it (the positive controls).
func TestAZonesLegendaryResistanceIsTheMastersAlone(t *testing.T) {
	w := newWorld(t)
	for _, word := range append([]string{"REACTION_KIND_LEGENDARY_RESISTANCE", "legendaryResistance", "Usar Resistência Lendária"}, resistanceWords...) {
		w.secrets.add(&canary{needle: word, kind: "legendary-resistance-zone"})
	}
	pens, _ := w.auraFight()
	inSight := []string{"dragon-name", "seen-npc-name", "combatant-id"}

	var cast, answer, masterLog reply
	var window *playv1.ReactionWindow
	var asked, masterReads []reply
	after := map[*person][]reply{}
	var passed bool
	got := w.hiddenOnlyEvents(func() {
		if cast = w.auraCast(pens); !cast.ok() {
			t.Fatalf("Ana's cast: status %d %s", cast.status, cast.body)
		}
		window = w.untilAsked()
		asked = []reply{w.reads(w.ana)[0], w.reads(w.caio)[0]}
		for _, p := range []*person{w.ana, w.caio} {
			asks, fake := w.zoneSave(p, window.GetId(), 1, false), w.zoneSave(p, newKey(), 1, false) // use=false: the refusal comes from the window, not from the choice
			if asks.ok() || fake.ok() || asks.status != fake.status || !bytes.Equal(asks.body, fake.body) {
				t.Errorf("%s: a real window is refused with %d %s, an invented one with %d %s: they must read the same", p.name, asks.status, asks.body, fake.status, fake.body)
			}
		}
		if answer = w.zoneSave(w.master, window.GetId(), 1, true); !answer.ok() {
			t.Fatalf("the master's answer: status %d %s", answer.status, answer.body)
		}
		for _, p := range []*person{w.ana, w.caio} {
			after[p] = w.reads(p)
		}
		masterReads = w.reads(w.master)
		masterLog = w.master.call(playv1connect.CombatServiceListCombatLogProcedure, &playv1.ListCombatLogRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId()})
		for _, e := range w.logOfPerson(w.ana) {
			fx := e.GetEffect()
			passed = passed || (fx.GetSourceKey() == "spell:spirit-guardians" && fx.GetChange() == playv1.CombatLogEffectChange_COMBAT_LOG_EFFECT_CHANGE_SAVED && fx.GetD20() == 0 && !fx.GetResisted())
		}
	})
	for _, p := range []*person{w.ana, w.caio} {
		reads := slices.Concat(after[p], asked)
		if p == w.ana {
			reads = append(reads, cast)
		}
		for _, r := range reads {
			for _, f := range w.inspect(p, r, inSight) {
				t.Errorf("%s: %s", p.name, f)
			}
		}
	}
	// The stream: the master hears the window open and close; the players hear neither.
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
	}
	// The positive controls: the master reads the choice and the count; Ana reads a save that passed.
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
	if !passed {
		t.Error("Ana's log has no line of a save that passed: the control is missing")
	}
}

// zoneRun plays the aura at the dragon in a world of its own: the master types face for the dragon's save and chooses the
// trait when use is set. It returns what the players read, as the player's own text, and the master's reads.
func zoneRun(t *testing.T, face int32, use bool) heldCastRead {
	t.Helper()
	w := newWorld(t)
	pens, _ := w.auraFight()
	w.dice.ScriptFor(t, 8, 4, 4, 4) // the 3d8 of the aura lands the same in both runs: the players read the amount
	players := []*person{w.ana, w.caio}
	parts := map[string][]reply{}
	out := heldCastRead{player: map[string]string{}}
	got := w.hiddenOnlyEvents(func() {
		cast := w.auraCast(pens)
		if !cast.ok() {
			t.Fatalf("Ana's cast: status %d %s", cast.status, cast.body)
		}
		parts[w.ana.name] = append(parts[w.ana.name], cast)
		out.window = w.untilAsked()
		for _, p := range players {
			parts[p.name] = append(parts[p.name], w.reads(p)...) // while it waits
		}
		resp := w.zoneSave(w.master, out.window.GetId(), face, use)
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
		// The hints are compared by kind: how many of one kind the gates coalesced depends on the timing.
		kinds := map[string]bool{}
		for _, ev := range got[p.name] {
			kinds[strings.SplitN(ev, " ", 2)[0]] = true
		}
		text.WriteString(strings.Join(slices.Sorted(maps.Keys(kinds)), "\n"))
		out.player[p.name] = normalizeRead(text.String())
	}
	return out
}

// The dragon is asked whatever it rolls, so the players read the same thing either way: the same wait, the same log, the
// same answer to the cast and the same stream hints, whether the save passed on its own or failed and the master turned it
// into a success. Only the ids and the timestamps are normalised. The master reads the two runs differently (the positive
// controls: a use spent or nothing spent).
func TestAPassedAndAResistedZoneSaveReadTheSameToThePlayers(t *testing.T) {
	var passed, resisted heldCastRead
	t.Run("the save passes", func(t *testing.T) { passed = zoneRun(t, 20, true) })
	t.Run("the save fails and the master uses the trait", func(t *testing.T) { resisted = zoneRun(t, 1, true) })
	for _, name := range []string{"Ana", "Caio"} {
		if passed.player[name] != resisted.player[name] {
			t.Errorf("%s reads a passed save and a resisted one differently:\n--- passed\n%s\n--- resisted\n%s", name, passed.player[name], resisted.player[name])
		}
	}
	if !strings.Contains(passed.player["Caio"], "Esperando o mestre") {
		t.Error("Caio reads no wait in the run where the save passed: the control is missing")
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
