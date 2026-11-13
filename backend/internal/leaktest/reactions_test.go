package leaktest

import (
	"bytes"
	"strings"
	"testing"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1/playv1connect"
)

// The reaction window (PM-04, RN-10, RN-20): what an NPC could do in answer to a player's
// action is the master's alone until it happens. A player reads "Esperando o mestre" and
// nothing of the window, the reactor, the reaction or the numbers it holds.

// reactionFight puts a visible Mago (Shield, Counterspell) next to Toren, on Toren's turn,
// and returns the Mago's combatant. Toren carries a dagger.
func (w *world) reactionFight() *playv1.Combatant {
	w.t.Helper()
	return w.reactionFightNamed(w.secrets.public("mage-name"))
}

// reactionFightNamed is reactionFight with the name the Mago is given.
func (w *world) reactionFightNamed(name string) *playv1.Combatant {
	w.t.Helper()
	return w.reactionFightOf("monster:mage", name)
}

// reactionFightOf is reactionFight with the creature of the bestiary the NPC is made from, and its name.
func (w *world) reactionFightOf(creatureKey, name string) *playv1.Combatant {
	w.t.Helper()
	ctx := w.t.Context()
	m := w.master
	sheet := w.fullSheet("class:wizard", "race:human", 1)
	sheet.GetFull().WeaponKeys = []string{"equipment:dagger"}
	must(w.master.characters.UpdateCharacter(ctx, rq(&charactersv1.UpdateCharacterRequest{
		CampaignId: w.campaign, CharacterId: w.toren.GetId(), Revision: w.toren.GetRevision(), Name: w.toren.GetName(), Sheet: sheet,
	})))
	hidden := false
	w.scriptD20(ownFace)
	must(m.combat.AddMonsters(ctx, rq(&playv1.AddMonstersRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), IdempotencyKey: newKey(), CreatureKey: creatureKey, Count: 1, Name: name, Hidden: &hidden,
	})))
	e := must(m.combat.GetEncounter(ctx, rq(&playv1.GetEncounterRequest{CampaignId: w.campaign}))).GetEncounter()
	var mage *playv1.Combatant
	for _, c := range e.GetCombatants() {
		if c.GetBestiaryCreatureKey() == creatureKey {
			mage = c
		}
	}
	if mage == nil {
		w.t.Fatal("the Mago is not in the combat")
	}
	w.requireOwnTotal(mage.GetId())
	w.placeForced(m, e.GetId(), placement{w.combatant(w.toren).GetId(), 12, 14}, placement{mage.GetId(), 13, 14})
	for range 40 {
		cur := must(m.combat.GetEncounter(ctx, rq(&playv1.GetEncounterRequest{CampaignId: w.campaign}))).GetEncounter()
		if cur.GetCurrentCombatantId() == w.combatant(w.toren).GetId() {
			w.encounter = cur
			return mage
		}
		must(m.combat.EndTurn(ctx, rq(&playv1.EndTurnRequest{CampaignId: w.campaign, EncounterId: e.GetId(), IdempotencyKey: newKey(), ExpectedCombatantId: cur.GetCurrentCombatantId(), ExpectedRound: cur.GetRound()})))
	}
	w.t.Fatal("Toren's turn never came")
	return nil
}

// reactionNeedles are what the players must not read before the reaction happens: the
// reaction's names and the Mago's spells, and the id of its window.
func (w *world) reactionNeedles(windowID string) {
	w.secrets.add(&canary{needle: "Escudo Arcano", kind: "reaction-name"})
	w.secrets.add(&canary{needle: "spell:shield", kind: "reaction-spell"})
	w.secrets.add(&canary{needle: "spell:counterspell", kind: "reaction-spell"})
	w.secrets.add(&canary{needle: windowID, kind: "reaction-window-id"})
}

func (w *world) reads(p *person) []reply {
	w.t.Helper()
	enc := p.call(playv1connect.CombatServiceGetEncounterProcedure, &playv1.GetEncounterRequest{CampaignId: w.campaign})
	log := p.call(playv1connect.CombatServiceListCombatLogProcedure, &playv1.ListCombatLogRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId()})
	return []reply{enc, log}
}

// A hit that an NPC's Shield could change waits for the master: the players read the same
// words as for a "Sempre" check, and no read, of the attack or of the combat, holds a word of the
// window, the reaction, the Mago's spells or the numbers of the trigger. The master's read has
// them (the positive control: the needles are reachable).
func TestAnNPCsReactionIsTheMastersAloneUntilItHappens(t *testing.T) {
	w := newWorld(t)
	mage := w.reactionFight()
	toren := w.combatant(w.toren)

	hit := w.caio.call(playv1connect.CombatServiceRollAttackProcedure, &playv1.RollAttackRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), AttackerId: toren.GetId(), AttackKey: "equipment:dagger", TargetId: mage.GetId(),
		IdempotencyKey: newKey(), Roll: &playv1.RollAttackRequest_D20Face{D20Face: 10},
	})
	if !hit.ok() {
		t.Fatalf("Toren's attack: status %d %s", hit.status, hit.body)
	}
	enc := must(w.master.combat.GetEncounter(t.Context(), rq(&playv1.GetEncounterRequest{CampaignId: w.campaign}))).GetEncounter()
	if len(enc.GetReactionWindows()) != 1 || enc.GetReactionWindows()[0].GetKind() != playv1.ReactionKind_REACTION_KIND_SHIELD {
		t.Fatalf("the master reads %v, want the Mago's Shield window", enc.GetReactionWindows())
	}
	window := enc.GetReactionWindows()[0]
	w.reactionNeedles(window.GetId())

	check := func(when string) {
		t.Helper()
		replies := map[*person][]reply{}
		for _, p := range []*person{w.ana, w.caio} {
			replies[p] = append(w.reads(p), hit)
			if p != w.caio {
				replies[p] = replies[p][:2] // the attack's answer is Caio's own
			}
			for _, r := range replies[p] {
				for _, f := range w.inspect(p, r, nil) {
					t.Errorf("%s %s: %s", when, p.name, f)
				}
			}
		}
	}
	check("while the Mago may react")
	// The positive control: the master's own read holds the needles the players' do not.
	masterReads := w.reads(w.master)
	found := false
	for _, r := range masterReads {
		if len(r.body) > 0 && contains(r.body, window.GetId()) {
			found = true
		}
	}
	if !found {
		t.Error("the master's reads do not hold the window: the needle is unreachable")
	}
	// A player may not answer it, and asking cannot tell a real window from an invented one.
	asked := w.caio.call(playv1connect.CombatServiceAnswerReactionProcedure, &playv1.AnswerReactionRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), WindowId: window.GetId(), Answer: playv1.ReactionChoice_REACTION_CHOICE_PASS, IdempotencyKey: newKey(),
	})
	fake := w.caio.call(playv1connect.CombatServiceAnswerReactionProcedure, &playv1.AnswerReactionRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), WindowId: newKey(), Answer: playv1.ReactionChoice_REACTION_CHOICE_PASS, IdempotencyKey: newKey(),
	})
	if asked.ok() || fake.ok() || asked.status != fake.status || !bytes.Equal(asked.body, fake.body) {
		t.Errorf("a real window is refused with %d %s, an invented one with %d %s: they must read the same", asked.status, asked.body, fake.status, fake.body)
	}
	// The master uses Shield for the Mago.
	must(w.master.combat.AnswerReaction(t.Context(), rq(&playv1.AnswerReactionRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), WindowId: window.GetId(), Answer: playv1.ReactionChoice_REACTION_CHOICE_USE,
		Slot: &playv1.SpellSlot{Level: 1}, IdempotencyKey: newKey(),
	})))
	// What happened is no secret any more, but the window and its numbers still are.
	w.secrets.allowNeedle("Escudo Arcano", w.ana, w.caio)
	w.secrets.allowNeedle("spell:shield", w.ana, w.caio)
	check("after the Mago reacted")
}

func contains(body []byte, s string) bool { return bytes.Contains(body, []byte(s)) }

// An NPC's window is heard by the master alone: the players' streams get the combat's
// ordinary change events and neither of the window's two (RN-10), while the master's
// stream gets both (the positive control).
func TestAnNPCsReactionWindowIsHeardByTheMasterAlone(t *testing.T) {
	w := newWorld(t)
	mage := w.reactionFight()
	toren := w.combatant(w.toren)
	got := w.hiddenOnlyEvents(func() {
		must(w.caio.combat.RollAttack(t.Context(), rq(&playv1.RollAttackRequest{
			CampaignId: w.campaign, EncounterId: w.encounter.GetId(), AttackerId: toren.GetId(), AttackKey: "equipment:dagger", TargetId: mage.GetId(),
			IdempotencyKey: newKey(), Roll: &playv1.RollAttackRequest_D20Face{D20Face: 10},
		})))
		enc := must(w.master.combat.GetEncounter(t.Context(), rq(&playv1.GetEncounterRequest{CampaignId: w.campaign}))).GetEncounter()
		must(w.master.combat.AnswerReaction(t.Context(), rq(&playv1.AnswerReactionRequest{
			CampaignId: w.campaign, EncounterId: w.encounter.GetId(), WindowId: enc.GetReactionWindows()[0].GetId(),
			Answer: playv1.ReactionChoice_REACTION_CHOICE_PASS, IdempotencyKey: newKey(),
		})))
	})
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
	if o, c := count(w.master.name); o != 1 || c != 1 {
		t.Errorf("the master heard %d openings and %d closings, want one of each (%v)", o, c, got[w.master.name])
	}
	for _, p := range []*person{w.ana, w.caio} {
		if o, c := count(p.name); o != 0 || c != 0 {
			t.Errorf("%s heard %d openings and %d closings of an NPC's window", p.name, o, c)
		}
	}
}

// A Hellish Rebuke on a bestiary NPC (it has a sheet of numbers): the tiefling's player answers the
// first question, and the second one (the aggressor's saving throw) is the master's. The player's
// window never carries the aggressor's Dexterity modifier (RN-20); the master's does (positive
// control).
func TestHellishRebukeNeverGivesThePlayerTheAggressorsModifier(t *testing.T) {
	w := newWorld(t)
	ctx := t.Context()
	m := w.master
	sheet := w.fullSheet("class:wizard", "race:tiefling", 3)
	must(m.characters.UpdateCharacter(ctx, rq(&charactersv1.UpdateCharacterRequest{
		CampaignId: w.campaign, CharacterId: w.toren.GetId(), Revision: w.toren.GetRevision(), Name: w.toren.GetName(), Sheet: sheet,
	})))
	toren, bandit := w.combatant(w.toren), w.combatant(w.bandit)
	// The aggressor is in plain sight of the tiefling (Hellish Rebuke needs to see it): its name is theirs now.
	must(m.combat.SetCombatantHidden(ctx, rq(&playv1.SetCombatantHiddenRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), CombatantId: bandit.GetId(), IdempotencyKey: newKey(), Hidden: false})))
	w.allow("bandit-name", w.ana, w.caio)
	w.secrets.allowNeedle(bandit.GetId(), w.ana, w.caio)
	w.placeForced(m, w.encounter.GetId(), placement{toren.GetId(), 12, 14}, placement{bandit.GetId(), 13, 14})
	for range 40 {
		cur := must(m.combat.GetEncounter(ctx, rq(&playv1.GetEncounterRequest{CampaignId: w.campaign}))).GetEncounter()
		if cur.GetCurrentCombatantId() == bandit.GetId() {
			w.encounter = cur
			break
		}
		must(m.combat.EndTurn(ctx, rq(&playv1.EndTurnRequest{CampaignId: w.campaign, EncounterId: cur.GetId(), IdempotencyKey: newKey(), ExpectedCombatantId: cur.GetCurrentCombatantId(), ExpectedRound: cur.GetRound()})))
	}
	opts := must(m.combat.GetTurnOptions(ctx, rq(&playv1.GetTurnOptionsRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), CombatantId: bandit.GetId()})))
	key := ""
	for _, a := range opts.GetOptions().GetAttacks() {
		if a.GetEnabled() && a.GetAttack().GetKey() != "" && a.GetAttack().GetSaveDc() == 0 {
			key = a.GetAttack().GetKey()
			break
		}
	}
	if key == "" {
		t.Fatal("the bandit has no attack to make")
	}
	hit := must(m.combat.RollAttack(ctx, rq(&playv1.RollAttackRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), AttackerId: bandit.GetId(), AttackKey: key, TargetId: toren.GetId(), IdempotencyKey: newKey(),
		Roll: &playv1.RollAttackRequest_D20Face{D20Face: 19},
	})))
	if hit.GetPendingDamage().GetId() == "" {
		t.Fatalf("the bandit's attack = %v, want a hit", hit.GetRoll())
	}
	must(m.combat.RollDamage(ctx, rq(&playv1.RollDamageRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), PendingDamageId: hit.GetPendingDamage().GetId(), IdempotencyKey: newKey(), Roll: &playv1.RollDamageRequest_RollInApp{RollInApp: true}})))
	must(m.combat.ApplyPendingDamage(ctx, rq(&playv1.ApplyPendingDamageRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), PendingDamageId: hit.GetPendingDamage().GetId(), IdempotencyKey: newKey()})))

	first := windowOfKind(w.caio, w, playv1.ReactionKind_REACTION_KIND_HELLISH_REBUKE)
	if first == nil {
		t.Fatal("the tiefling has no Hellish Rebuke window after the damage")
	}
	must(w.caio.combat.AnswerReaction(ctx, rq(&playv1.AnswerReactionRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), WindowId: first.GetId(), Answer: playv1.ReactionChoice_REACTION_CHOICE_USE, UseRacial: true, IdempotencyKey: newKey(),
	})))

	asMaster := windowOfKind(m, w, playv1.ReactionKind_REACTION_KIND_HELLISH_REBUKE)
	if asMaster == nil || !asMaster.GetSecondStep() || !asMaster.GetHellishRebukeSave().GetBonusKnown() {
		t.Fatalf("the master's second step = %v, want the aggressor's modifier (the positive control)", asMaster)
	}
	asPlayer := windowOfKind(w.caio, w, playv1.ReactionKind_REACTION_KIND_HELLISH_REBUKE)
	if asPlayer == nil {
		t.Fatal("the reactor's player lost the window")
	}
	if p := asPlayer.GetHellishRebukeSave(); p.GetSaveBonus() != 0 || p.GetBonusKnown() {
		t.Errorf("the player's second step carries the aggressor's modifier: %v", p)
	}
	for _, r := range w.reads(w.caio) {
		for _, f := range w.inspect(w.caio, r, nil) {
			t.Errorf("Caio: %s", f)
		}
	}
}

// windowOfKind is the first window of the kind that the person reads in the combat.
func windowOfKind(p *person, w *world, kind playv1.ReactionKind) *playv1.ReactionWindow {
	enc := must(p.combat.GetEncounter(w.t.Context(), rq(&playv1.GetEncounterRequest{CampaignId: w.campaign}))).GetEncounter()
	for _, win := range enc.GetReactionWindows() {
		if win.GetKind() == kind {
			return win
		}
	}
	return nil
}

// A Mago that is not hidden but is outside what Ana's character sees: when the master uses its
// Escudo, the line of the log is Caio's (his character sees it) and never Ana's: neither the
// Mago's name nor the reaction reaches her (RN-10, MR-036). Caio's read is the positive control.
func TestAReactionOutsideAPlayersVisionIsNotInTheirLog(t *testing.T) {
	w := newWorld(t)
	name := w.secrets.marker("mage-seen-by-caio", w.caio)
	mage := w.reactionFightNamed(name)
	toren := w.combatant(w.toren)
	hit := w.caio.call(playv1connect.CombatServiceRollAttackProcedure, &playv1.RollAttackRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), AttackerId: toren.GetId(), AttackKey: "equipment:dagger", TargetId: mage.GetId(),
		IdempotencyKey: newKey(), Roll: &playv1.RollAttackRequest_D20Face{D20Face: 10},
	})
	if !hit.ok() {
		t.Fatalf("Toren's attack: status %d %s", hit.status, hit.body)
	}
	win := windowOfKind(w.master, w, playv1.ReactionKind_REACTION_KIND_SHIELD)
	if win == nil {
		t.Fatal("the Mago has no Shield window")
	}
	w.reactionNeedles(win.GetId())
	must(w.master.combat.AnswerReaction(t.Context(), rq(&playv1.AnswerReactionRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), WindowId: win.GetId(), Answer: playv1.ReactionChoice_REACTION_CHOICE_USE,
		Slot: &playv1.SpellSlot{Level: 1}, IdempotencyKey: newKey(),
	})))
	// What Caio sees he may read: the reaction's names; Ana's reads must hold none of them.
	w.secrets.allowNeedle("Escudo Arcano", w.caio)
	w.secrets.allowNeedle("spell:shield", w.caio)
	caio := w.reads(w.caio)[1]
	if !contains(caio.body, "Escudo Arcano") || !contains(caio.body, name) {
		t.Fatalf("Caio's log does not hold the line: %s", caio.body)
	}
	for _, r := range w.reads(w.ana) {
		for _, f := range w.inspect(w.ana, r, nil) {
			t.Errorf("Ana: %s", f)
		}
	}
}

// The record of a spent reaction (which one, in which round) is the master's and the owner's, like
// reaction_used: an NPC's never reaches a player (RN-20), in the combat or in the log. The master's
// own read is the positive control.
func TestAnNPCsSpentReactionRecordIsTheMastersAlone(t *testing.T) {
	w := newWorld(t)
	mage := w.reactionFight()
	toren := w.combatant(w.toren)
	w.secrets.add(&canary{needle: `"reactionKind"`, kind: "reaction-record-kind"})
	w.secrets.add(&canary{needle: `"reactionRound"`, kind: "reaction-record-round"})

	must(w.caio.combat.RollAttack(t.Context(), rq(&playv1.RollAttackRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), AttackerId: toren.GetId(), AttackKey: "equipment:dagger", TargetId: mage.GetId(),
		IdempotencyKey: newKey(), Roll: &playv1.RollAttackRequest_D20Face{D20Face: 10},
	})))
	win := windowOfKind(w.master, w, playv1.ReactionKind_REACTION_KIND_SHIELD)
	if win == nil {
		t.Fatal("the Mago has no Shield window")
	}
	must(w.master.combat.AnswerReaction(t.Context(), rq(&playv1.AnswerReactionRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), WindowId: win.GetId(), Answer: playv1.ReactionChoice_REACTION_CHOICE_USE,
		Slot: &playv1.SpellSlot{Level: 1}, IdempotencyKey: newKey(),
	})))
	w.secrets.allowNeedle("Escudo Arcano", w.ana, w.caio)
	w.secrets.allowNeedle("spell:shield", w.ana, w.caio)

	for _, p := range []*person{w.ana, w.caio} {
		for _, r := range w.reads(p) {
			for _, f := range w.inspect(p, r, nil) {
				t.Errorf("%s: %s", p.name, f)
			}
		}
	}
	// The positive control: the master reads the record.
	enc := must(w.master.combat.GetEncounter(t.Context(), rq(&playv1.GetEncounterRequest{CampaignId: w.campaign}))).GetEncounter()
	var got *playv1.Combatant
	for _, c := range enc.GetCombatants() {
		if c.GetId() == mage.GetId() {
			got = c
		}
	}
	if got == nil || !got.GetReactionUsed() || got.GetReactionKind() != playv1.ReactionKind_REACTION_KIND_SHIELD || got.GetReactionRound() != enc.GetRound() {
		t.Fatalf("the master reads %v, want the Mago's Shield spent in round %d", got, enc.GetRound())
	}
	found := false
	for _, r := range w.reads(w.master) {
		found = found || contains(r.body, `"reactionKind"`)
	}
	if !found {
		t.Error("the master's reads do not hold the record: the canary is unreachable")
	}
}

// The record of a player's spent reaction (which one, in which round) is the owner's and the master's. A
// joint-turn partner reads reaction_used and nothing more, and a player outside the joint turn reads none of
// the three. The owner's and the master's reads are the positive controls.
func TestAPlayersSpentReactionRecordIsTheOwnersAndTheMastersAlone(t *testing.T) {
	w := newWorld(t)
	ctx := t.Context()
	m := w.master
	sheet := w.fullSheet("class:wizard", "race:tiefling", 3)
	must(m.characters.UpdateCharacter(ctx, rq(&charactersv1.UpdateCharacterRequest{
		CampaignId: w.campaign, CharacterId: w.toren.GetId(), Revision: w.toren.GetRevision(), Name: w.toren.GetName(), Sheet: sheet,
	})))
	toren, bandit := w.combatant(w.toren), w.combatant(w.bandit)
	must(m.combat.SetCombatantHidden(ctx, rq(&playv1.SetCombatantHiddenRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), CombatantId: bandit.GetId(), IdempotencyKey: newKey(), Hidden: false})))
	w.allow("bandit-name", w.ana, w.caio)
	w.secrets.allowNeedle(bandit.GetId(), w.ana, w.caio)
	w.placeForced(m, w.encounter.GetId(), placement{toren.GetId(), 12, 14}, placement{bandit.GetId(), 13, 14})
	for range 40 {
		cur := must(m.combat.GetEncounter(ctx, rq(&playv1.GetEncounterRequest{CampaignId: w.campaign}))).GetEncounter()
		if cur.GetCurrentCombatantId() == toren.GetId() {
			w.encounter = cur
			break
		}
		must(m.combat.EndTurn(ctx, rq(&playv1.EndTurnRequest{CampaignId: w.campaign, EncounterId: cur.GetId(), IdempotencyKey: newKey(), ExpectedCombatantId: cur.GetCurrentCombatantId(), ExpectedRound: cur.GetRound()})))
	}
	if len(w.encounter.GetTurnGroupIds()) < 2 {
		t.Fatalf("the turn group = %v, want Toren and Pensantus in one joint turn", w.encounter.GetTurnGroupIds())
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
		t.Fatal("the bandit has no attack to make")
	}
	hit := must(m.combat.RollAttack(ctx, rq(&playv1.RollAttackRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), AttackerId: bandit.GetId(), AttackKey: key, TargetId: toren.GetId(), IdempotencyKey: newKey(),
		AsReaction: true, Roll: &playv1.RollAttackRequest_D20Face{D20Face: 19},
	})))
	if hit.GetPendingDamage().GetId() == "" {
		t.Fatalf("the bandit's attack = %v, want a hit", hit.GetRoll())
	}
	must(m.combat.RollDamage(ctx, rq(&playv1.RollDamageRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), PendingDamageId: hit.GetPendingDamage().GetId(), IdempotencyKey: newKey(), Roll: &playv1.RollDamageRequest_RollInApp{RollInApp: true}})))
	must(m.combat.ApplyPendingDamage(ctx, rq(&playv1.ApplyPendingDamageRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), PendingDamageId: hit.GetPendingDamage().GetId(), IdempotencyKey: newKey()})))
	first := windowOfKind(w.caio, w, playv1.ReactionKind_REACTION_KIND_HELLISH_REBUKE)
	if first == nil {
		t.Fatal("the tiefling has no Hellish Rebuke window after the damage")
	}
	must(w.caio.combat.AnswerReaction(ctx, rq(&playv1.AnswerReactionRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), WindowId: first.GetId(), Answer: playv1.ReactionChoice_REACTION_CHOICE_USE, UseRacial: true, IdempotencyKey: newKey(),
	})))
	w.secrets.add(&canary{needle: `"reactionKind"`, kind: "reaction-record-kind"})
	w.secrets.add(&canary{needle: `"reactionRound"`, kind: "reaction-record-round"})

	read := func(p *person) *playv1.Combatant {
		t.Helper()
		e := must(p.combat.GetEncounter(ctx, rq(&playv1.GetEncounterRequest{CampaignId: w.campaign}))).GetEncounter()
		return w.combatantOf(e, w.toren.GetId())
	}
	// The owner and the master read the record.
	for _, p := range []*person{w.caio, m} {
		c := read(p)
		if !c.GetReactionUsed() || c.GetReactionKind() != playv1.ReactionKind_REACTION_KIND_HELLISH_REBUKE || c.GetReactionRound() < 1 {
			t.Errorf("%s reads %v/%v/round %d, want the Hellish Rebuke spent", p.name, c.GetReactionUsed(), c.GetReactionKind(), c.GetReactionRound())
		}
	}
	// The joint-turn partner reads that the reaction is used and nothing more.
	if c := read(w.ana); !c.GetReactionUsed() || c.GetReactionKind() != playv1.ReactionKind_REACTION_KIND_UNSPECIFIED || c.GetReactionRound() != 0 {
		t.Errorf("Ana (in the joint turn) reads %v/%v/round %d, want reaction_used and no record", c.GetReactionUsed(), c.GetReactionKind(), c.GetReactionRound())
	}
	for _, p := range []*person{w.ana, w.pending} {
		for _, r := range w.reads(p) {
			for _, f := range w.inspect(p, r, nil) {
				t.Errorf("%s: %s", p.name, f)
			}
		}
	}
}
