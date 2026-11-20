package play

import (
	"slices"
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// targetsFight is a combat where Pensantus acts first with the Capitão Goblin next to him, "Goblin 1" revealed 90 ft
// away (out of the 60 ft of Hold Person) and "Goblin 2" not revealed: the players do not see it.
func (a *armed) targetsFight(t *testing.T) *playv1.Encounter {
	t.Helper()
	return a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: a.goblin.GetId(), Count: 2}, {CharacterId: a.capitao.GetId()}},
		npcRolls: []int{1, 1, 1}, reveal: []string{"Capitão Goblin", "Goblin 1"},
		at:      map[string][2]int32{"Pensantus": {1, 5}, "Toren": {2, 5}, "Brisa": {1, 6}, "Capitão Goblin": {2, 4}, "Goblin 1": {19, 5}, "Goblin 2": {3, 5}},
		players: map[string]int32{"Pensantus": 20, "Toren": 15, "Brisa": 10},
	})
}

// releaseOf readies the spell on Pensantus's turn in targetsFight, gives the turn on and fires the window.
func (a *armed) releaseOf(t *testing.T, spell string, slot int32) (*playv1.Encounter, *playv1.ReactionWindow) {
	t.Helper()
	e := a.targetsFight(t)
	a.mustReady(t, a.ana, a.readySpell(t, e, "Pensantus", spell, slot, "Capitão Goblin"))
	e = a.mustEndTurn(t, a.master, e)
	return e, a.mustFire(t, e, "Pensantus")
}

// wantWindowKept checks a refused release spent nothing: the window is open, the row waits, the slot of Ready, at the held
// spell's level, is the only one spent and the reaction is free.
func (a *armed) wantWindowKept(t *testing.T, level, slots int32) {
	t.Helper()
	if !a.readiedOpen(t, "Pensantus") || a.readiedStatus(t, "Pensantus") != readiedWaiting || byLabel(t, a.get(t, a.master), "Pensantus").GetReactionUsed() ||
		a.slotsUsed(t, "Pensantus", level) != slots {
		t.Errorf("after the refusal: window open %v, rows %q, reaction used %v, level-%d slots %d (want %d)", a.readiedOpen(t, "Pensantus"),
			a.readiedStatus(t, "Pensantus"), byLabel(t, a.get(t, a.master), "Pensantus").GetReactionUsed(), level, a.slotsUsed(t, "Pensantus", level), slots)
	}
}

// The release is checked as a cast, and what it refuses leaves the window open: a dart on a creature that fell, a target
// out of the spell's range and a creature the players do not see, whether the readier's player answers or the master,
// who picks the targets with that player's eyes (a hidden creature is "not found" to both).
func TestAReadiedReleaseRefusesWhatTheCastRefusesAndKeepsItsWindowOpen(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		spell  string
		slot   int32
		defeat string
		target string
		darts  int32
		master bool
		// farCol moves the target to that column of its row, on a map widened to 40 columns (the 20 of the fight reach at most 95 ft,
		// short of Magic Missile's 120 ft): column 26 is 125 ft from Pensantus. 0 leaves it where the fight put it.
		farCol int32
		check  func(t *testing.T, err error)
	}{
		{"a dart on a creature that fell", magicMissileSpell, 1, "Capitão Goblin", "Capitão Goblin", 3, false, 0, func(t *testing.T, err error) {
			t.Helper()
			wantBlockedBy(t, "a dart on a fallen goblin", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TARGET_DEFEATED)
		}},
		{"a target out of range", holdPerson, 2, "", "Goblin 1", 0, false, 0, func(t *testing.T, err error) {
			t.Helper()
			wantCode(t, "Hold Person on a target 90 ft away", err, connect.CodeFailedPrecondition)
		}},
		{"a dart on a target out of range", magicMissileSpell, 1, "", "Goblin 1", 3, false, 26, func(t *testing.T, err error) {
			t.Helper()
			wantBlockedBy(t, "a dart on a target 125 ft away", err, blockedOutOfReach)
		}},
		{"a creature the players do not see", magicMissileSpell, 1, "", "Goblin 2", 3, false, 0, func(t *testing.T, err error) {
			t.Helper()
			wantCode(t, "a hidden target", err, connect.CodeNotFound)
		}},
		{"a creature the players do not see, answered by the master", magicMissileSpell, 1, "", "Goblin 2", 3, true, 0, func(t *testing.T, err error) {
			t.Helper()
			wantCode(t, "a hidden target answered by the master", err, connect.CodeNotFound)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a := newCasters(t)
			e, w := a.releaseOf(t, tc.spell, tc.slot)
			slots := a.slotsUsed(t, "Pensantus", tc.slot)
			if tc.farCol != 0 {
				a.execSQL(t, `UPDATE encounters SET grid_columns = 40`)
				a.execSQL(t, `UPDATE combatants SET grid_col = $2 WHERE id = $1`, a.id(t, tc.target), tc.farCol)
			}
			if tc.defeat != "" {
				a.execSQL(t, `UPDATE combatants SET defeated = true WHERE id = $1`, a.id(t, tc.defeat))
			}
			u := a.ana
			if tc.master {
				u = a.master
			}
			_, err := a.answerReaction(t, u, e, w.GetId(), withTargets(darts(a, t, tc.target, tc.darts)))
			tc.check(t, err)
			a.wantWindowKept(t, tc.slot, slots)
		})
	}
}

// The answer of a readied Magic Missile is idempotent: the same key again answers the same and casts nothing more, and the
// same key with other targets is refused. The release spends the reaction and ends the held concentration, but no
// slot (it was spent at Ready), and its undo makes the row wait with the spell held, the window open and the damage taken back.
func TestAReadiedReleaseIsIdempotentAndItsUndoHoldsTheSpellAgain(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e, w := a.releaseOf(t, magicMissileSpell, 1)
	slots, key := a.slotsUsed(t, "Pensantus", 1), newKey()
	split := func(r *playv1.AnswerReactionRequest) {
		withTargets(darts(a, t, "Capitão Goblin", 3))(r)
		r.IdempotencyKey = key
	}
	first := a.mustAnswer(t, a.ana, e, w.GetId(), split)
	events := a.events(t)
	again := a.mustAnswer(t, a.ana, e, w.GetId(), split)
	if !first.GetResult().GetUsed() || !again.GetResult().GetUsed() || a.events(t) != events || len(a.castLines(t, e, magicMissileSpell)) != 1 {
		t.Errorf("replay: results %v and %v, events %d (want %d), cast lines %d (want 1)", first.GetResult(), again.GetResult(), a.events(t), events, len(a.castLines(t, e, magicMissileSpell)))
	}
	_, err := a.answerReaction(t, a.ana, e, w.GetId(), func(r *playv1.AnswerReactionRequest) {
		withTargets(darts(a, t, "Goblin 1", 3))(r)
		r.IdempotencyKey = key
	})
	wantCode(t, "the same key with other targets", err, connect.CodeInvalidArgument)
	pens := byLabel(t, a.get(t, a.master), "Pensantus")
	if !pens.GetReactionUsed() || pens.GetConcentrationSpell() != "" || a.slotsUsed(t, "Pensantus", 1) != slots || a.readiedStatus(t, "Pensantus") != readiedFired || a.pendingCount(t) != 1 {
		t.Errorf("after the release: reaction used %v, concentration %q, slots %d (want %d), rows %q, damages waiting %d (want 1)",
			pens.GetReactionUsed(), pens.GetConcentrationSpell(), a.slotsUsed(t, "Pensantus", 1), slots, a.readiedStatus(t, "Pensantus"), a.pendingCount(t))
	}

	a.mustUndo(t, e)
	pens = byLabel(t, a.get(t, a.master), "Pensantus")
	if a.readiedStatus(t, "Pensantus") != readiedWaiting || !a.readiedOpen(t, "Pensantus") || pens.GetConcentrationSpell() != magicMissileSpell ||
		pens.GetReactionUsed() || a.pendingCount(t) != 0 || a.slotsUsed(t, "Pensantus", 1) != slots {
		t.Errorf("after the undo: rows %q, window open %v, concentration %q, reaction used %v, damages waiting %d (want 0), slots %d (want %d)", a.readiedStatus(t, "Pensantus"),
			a.readiedOpen(t, "Pensantus"), pens.GetConcentrationSpell(), pens.GetReactionUsed(), a.pendingCount(t), a.slotsUsed(t, "Pensantus", 1), slots)
	}
}

// targets are for the use of a readied spell: a pass that carries them is refused and not taken for a plain pass, and the
// window can still be answered.
func TestTargetsOnAPassOfAReadiedWindowAreRefused(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e, w := a.releaseOf(t, magicMissileSpell, 1)
	_, err := a.answerReaction(t, a.ana, e, w.GetId(), func(r *playv1.AnswerReactionRequest) {
		passAnswer(r)
		r.Targets = []*playv1.SpellTarget{darts(a, t, "Capitão Goblin", 3)}
	})
	wantCode(t, "a pass with targets", err, connect.CodeInvalidArgument)
	a.wantWindowKept(t, 1, a.slotsUsed(t, "Pensantus", 1))
	a.mustAnswer(t, a.ana, e, w.GetId(), passAnswer)
}

// The darts of a release that Counterspell holds are the ones the answer named: Pensantus lets the counter pass and the
// replay casts the spell with them. The Counterspell window takes no targets: they are for a readied spell.
func TestACounterspellOnAReadiedReleaseWithDartsCastsThemWhenItPasses(t *testing.T) {
	t.Parallel()
	a, e := counterFight(t)
	a.mustReady(t, a.master, a.readySpell(t, e, "Mago Sombrio", magicMissileSpell, 1, "Toren"))
	e = a.mustEndTurn(t, a.master, e)
	w := a.mustFire(t, e, "Mago Sombrio")
	held := a.mustAnswer(t, a.master, e, w.GetId(), withTargets(darts(a, t, "Toren", 2), darts(a, t, "Brisa", 1)))
	cw := a.windowOf(t, a.ana, playv1.ReactionKind_REACTION_KIND_COUNTERSPELL)
	if held.GetResult().GetUsed() || cw == nil || a.readiedStatus(t, "Mago Sombrio") != readiedWaiting {
		t.Fatalf("the release = %v, Counterspell window %v, rows %q; want it held for Counterspell", held.GetResult(), cw, a.readiedStatus(t, "Mago Sombrio"))
	}
	_, err := a.answerReaction(t, a.ana, e, cw.GetId(), func(r *playv1.AnswerReactionRequest) {
		passAnswer(r)
		r.Targets = []*playv1.SpellTarget{darts(a, t, "Toren", 3)}
	})
	wantCode(t, "Counterspell's window with targets", err, connect.CodeInvalidArgument)
	_, err = a.answerReaction(t, a.ana, e, cw.GetId(), func(r *playv1.AnswerReactionRequest) {
		useAnswer(3)(r)
		r.Targets = []*playv1.SpellTarget{darts(a, t, "Toren", 3)}
	})
	wantCode(t, "a use of Counterspell's window with targets", err, connect.CodeInvalidArgument)
	if a.windowOf(t, a.ana, playv1.ReactionKind_REACTION_KIND_COUNTERSPELL) == nil || a.slotsUsed(t, "Pensantus", 3) != 0 {
		t.Errorf("after the refused use: Counterspell window open %v, 3rd-level slots used %d (want it open and 0)",
			a.windowOf(t, a.ana, playv1.ReactionKind_REACTION_KIND_COUNTERSPELL) != nil, a.slotsUsed(t, "Pensantus", 3))
	}
	a.mustAnswer(t, a.ana, e, cw.GetId(), passAnswer)
	got := map[string]int32{}
	for _, tg := range a.castLine(t, e, magicMissileSpell).GetSpell().GetTargets() {
		got[tg.GetTargetId()] += tg.GetDarts()
	}
	if got[a.id(t, "Toren")] != 2 || got[a.id(t, "Brisa")] != 1 || a.readiedStatus(t, "Mago Sombrio") != readiedFired {
		t.Errorf("darts by target = %v, rows %q; want 2 on Toren and 1 on Brisa, fired", got, a.readiedStatus(t, "Mago Sombrio"))
	}
}

// heldTargetsOf are the ids of the creatures the options of the combatant list for the held spell, as u reads them.
func (a *armed) heldTargetsOf(t *testing.T, u *user, e *playv1.Encounter, label, spell string) []string {
	t.Helper()
	var ids []string
	for _, st := range a.mustOptions(t, u, e, label).GetSpellTargets() {
		if st.GetSpellKey() != spell {
			continue
		}
		for _, tg := range st.GetTargets() {
			ids = append(ids, tg.GetCombatantId())
		}
	}
	slices.Sort(ids)
	return ids
}

// The release picks only what the readier's player could pick, whoever answers (it refuses a hidden creature as not found to
// both), so the list the master reads for the held spell holds exactly what the release accepts: the readier's player's.
func TestTheMastersListOfAHeldSpellHoldsWhatTheReleaseAccepts(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.targetsFight(t)
	a.mustReady(t, a.ana, a.readySpell(t, e, "Pensantus", holdPerson, 2, "Capitão Goblin"))
	e = a.mustEndTurn(t, a.master, e)
	hidden := a.id(t, "Goblin 2")
	if byLabel(t, a.get(t, a.master), "Goblin 2").GetId() != hidden {
		t.Fatal("the master's combat does not list the hidden goblin: the canary is unreachable")
	}
	player, master := a.heldTargetsOf(t, a.ana, e, "Pensantus", holdPerson), a.heldTargetsOf(t, a.master, e, "Pensantus", holdPerson)
	if len(player) == 0 || !slices.Equal(player, master) || slices.Contains(master, hidden) {
		t.Errorf("held Hold Person's targets: the player's %v, the master's %v; want the same creatures and not the hidden goblin %s", player, master, hidden)
	}
}

// The master answers for the readier's player and releases the held spell with targets that player could pick: it resolves as
// the player's answer does. The darts are dealt (a damage waits for each target), the row is fired, the window closed and the
// reaction of the readier spent, not the master's.
func TestTheMasterReleasesAHeldSpellWithTheTargetsThePlayerCouldPick(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e, w := a.releaseOf(t, magicMissileSpell, 1)
	slots := a.slotsUsed(t, "Pensantus", 1)
	res := a.mustAnswer(t, a.master, e, w.GetId(), withTargets(darts(a, t, "Capitão Goblin", 2), darts(a, t, "Goblin 1", 1)))
	if !res.GetResult().GetUsed() || !res.GetResult().GetByMaster() {
		t.Fatalf("the master's release = %v, want used by the master", res.GetResult())
	}
	got := map[string]int32{}
	for _, tg := range a.castLine(t, e, magicMissileSpell).GetSpell().GetTargets() {
		got[tg.GetTargetId()] += tg.GetDarts()
	}
	pens := byLabel(t, a.get(t, a.master), "Pensantus")
	if got[a.id(t, "Capitão Goblin")] != 2 || got[a.id(t, "Goblin 1")] != 1 || a.pendingCount(t) != 2 || a.readiedStatus(t, "Pensantus") != readiedFired ||
		a.readiedOpen(t, "Pensantus") || !pens.GetReactionUsed() || a.slotsUsed(t, "Pensantus", 1) != slots {
		t.Errorf("after the master's release: darts %v (want 2 on the Capitão, 1 on Goblin 1), damages waiting %d (want 2), rows %q, window open %v, reaction used %v, slots %d (want %d)",
			got, a.pendingCount(t), a.readiedStatus(t, "Pensantus"), a.readiedOpen(t, "Pensantus"), pens.GetReactionUsed(), a.slotsUsed(t, "Pensantus", 1), slots)
	}
}
