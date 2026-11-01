package play

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// The server rolls a d6 for each Recharge action a creature spent when the creature's own turn starts (SRD 5.1,
// "Recharge X-Y"). The roll is the master's alone: a line of his log, nothing for a player. These tests need the
// database (MEURPG_TEST_DATABASE_URL).

// spendByHand marks the actions of the creature as spent, as a use of each one would.
func (a *armed) spendByHand(t *testing.T, label string, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if _, err := bumpCreatureSpent(t.Context(), playdb.New(a.h.pool), a.id(t, label), k, 1); err != nil {
			t.Fatalf("spend %s: %v", k, err)
		}
	}
}

// rechargeLines are the Recharge rolls on the log as u reads it, in the log's order.
func (a *armed) rechargeLines(t *testing.T, u *user, e *playv1.Encounter) []*playv1.CombatLogRecharge {
	t.Helper()
	var out []*playv1.CombatLogRecharge
	for _, r := range a.log(t, u, e).GetRounds() {
		for _, l := range r.GetEntries() {
			if l.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_RECHARGE_ROLLED {
				out = append(out, l.GetRecharge())
			}
		}
	}
	return out
}

// A spent Recharge action rolls at the start of its creature's turn and is available again on the lowest face
// that recharges it and not on the one below: Recharge 5-6 (the hell hound's breath, a save), Recharge 6 (the
// ankheg's spray, a save) and Recharge 4-6 (the blink dog's teleport, no roll).
func TestRecharge_RollsAtTheTurnStartAndTheMinimumFaceRecharges(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		creature, action string
		min              int32
		targets          []string
	}{
		{houndKey, fireBreath, 5, []string{"Toren"}},
		{"monster:ankheg", "monster:ankheg#acid-spray", 6, []string{"Toren"}},
		{"monster:blink-dog", "monster:blink-dog#teleport", 4, nil},
	} {
		for _, below := range []bool{true, false} {
			face := tc.min
			if below {
				face--
			}
			t.Run(tc.action+"/"+strconv.Itoa(int(face)), func(t *testing.T) {
				t.Parallel()
				a, e, who := creatureFight(t, true, tc.creature, false)
				if len(tc.targets) > 0 {
					a.h.roller.queue(18)
				}
				a.mustUseAction(t, a.master, e, who, tc.action, tc.targets)
				key := spentRechargePrefix + tc.action
				if got := a.spentOf(t, a.id(t, who))[key]; got != 1 {
					t.Fatalf("after the use %s = %d, want 1", key, got)
				}
				a.h.roller.queue(int(face))
				e = a.ownTurnAgain(t, e, who)
				got := a.spentOf(t, a.id(t, who))[key]
				if recharged := !below; (got == 0) != recharged {
					t.Errorf("a %d on a Recharge %d: %s = %d, want recharged %v", face, tc.min, key, got, recharged)
				}
				if l := a.limitedOf(t, a.master, who, tc.action); l.GetAvailable() == below {
					t.Errorf("the master reads %v after a %d", l, face)
				}
				lines := a.rechargeLines(t, a.master, e)
				if len(lines) != 1 || lines[0].GetFace() != face || lines[0].GetRecharged() == below || lines[0].GetActionName() == "" {
					t.Errorf("the master's log has %v, want one roll of %d (recharged %v)", lines, face, !below)
				}
			})
		}
	}
}

// Several spent actions roll one die each, in the order of the stat block, and each keeps or clears its own key.
func TestRecharge_SeveralActionsRollInTheStatBlocksOrder(t *testing.T) {
	t.Parallel()
	a, e, who := creatureFight(t, true, "monster:adult-bronze-dragon", false)
	const base = "monster:adult-bronze-dragon#"
	a.spendByHand(t, who, spentRechargePrefix+base+"repulsion-breath", spentRechargePrefix+base+"breath-weapons", spentRechargePrefix+base+"lightning-breath")
	a.h.roller.queue(6, 2, 5) // Breath Weapons, Lightning Breath, Repulsion Breath: the stat block's order
	e = a.ownTurnAgain(t, e, who)
	got := a.spentOf(t, a.id(t, who))
	if len(got) != 1 || got[spentRechargePrefix+base+"lightning-breath"] != 1 {
		t.Errorf("after the rolls the creature has spent %v, want only the Lightning Breath", got)
	}
	var faces []int32
	for _, l := range a.rechargeLines(t, a.master, e) { // the log lists the newest first
		faces = append(faces, l.GetFace())
	}
	if !slices.Equal(faces, []int32{5, 2, 6}) {
		t.Errorf("the log's faces = %v, want the three rolls, newest first", faces)
	}
}

// What does not roll: an action that is available, one with uses a day, one that waits for a rest, a defeated
// creature and an NPC made by hand. The roller gives no die for any of them.
func TestRecharge_OnlyASpentRechargeActionRolls(t *testing.T) {
	t.Parallel()
	t.Run("available, per day and rest", func(t *testing.T) {
		t.Parallel()
		a, e, who := creatureFight(t, true, houndKey, false)
		a.spendByHand(t, who, spentUsePrefix+fireBreath) // a use key of an action that recharges is no Recharge spend
		before := a.h.roller.taken()
		e = a.ownTurnAgain(t, e, who)
		if got := a.h.roller.taken() - before; got != 0 || len(a.rechargeLines(t, a.master, e)) != 0 {
			t.Errorf("an available action rolled: %d dice, lines %v", got, a.rechargeLines(t, a.master, e))
		}
		b, eb, deva := creatureFight(t, true, "monster:deva", false)
		b.mustUseAction(t, b.master, eb, deva, healingTouch, nil)
		c, ec, octopus := creatureFight(t, true, "monster:octopus", false)
		c.mustUseAction(t, c.master, ec, octopus, inkCloud, nil)
		for _, tc := range []struct {
			a    *armed
			e    *playv1.Encounter
			who  string
			key  string
			name string
		}{{b, eb, deva, spentUsePrefix + healingTouch, "per day"}, {c, ec, octopus, spentRestPrefix + inkCloud, "rest"}} {
			before := tc.a.h.roller.taken()
			ne := tc.a.ownTurnAgain(t, tc.e, tc.who)
			if got := tc.a.h.roller.taken() - before; got != 0 || tc.a.spentOf(t, tc.a.id(t, tc.who))[tc.key] != 1 || len(tc.a.rechargeLines(t, tc.a.master, ne)) != 0 {
				t.Errorf("%s action: %d dice, spent %v", tc.name, got, tc.a.spentOf(t, tc.a.id(t, tc.who)))
			}
		}
	})
	t.Run("defeated", func(t *testing.T) {
		t.Parallel()
		a, e, who := creatureFight(t, true, houndKey, false)
		a.h.roller.queue(18)
		a.mustUseAction(t, a.master, e, who, fireBreath, []string{"Toren"})
		if _, err := a.adjustHP(t, e, who, damageHP(500)); err != nil {
			t.Fatalf("defeat the hound: %v", err)
		}
		before := a.h.roller.taken()
		a.h.roller.queue(6)
		e = a.passTo(t, e, "Pensantus")
		e = a.passTo(t, e, "Toren")
		_ = e
		if a.h.roller.taken() != before || a.spentOf(t, a.id(t, who))[spentRechargePrefix+fireBreath] != 1 {
			t.Errorf("a defeated creature rolled: dice %d, spent %v", a.h.roller.taken()-before, a.spentOf(t, a.id(t, who)))
		}
	})
	t.Run("hand-made NPC", func(t *testing.T) {
		t.Parallel()
		a, e, _ := houndFight(t, true)
		a.spendByHand(t, "Goblin", spentRechargePrefix+"nothing#there")
		before := a.h.roller.taken()
		e = a.passTo(t, e, "Goblin") // the goblin's turn starts
		a.mustEndTurn(t, a.master, e)
		if a.h.roller.taken() != before || a.spentOf(t, a.id(t, "Goblin"))[spentRechargePrefix+"nothing#there"] != 1 {
			t.Errorf("a hand-made NPC rolled: dice %d, spent %v", a.h.roller.taken()-before, a.spentOf(t, a.id(t, "Goblin")))
		}
	})
}

// The roll is the master's alone: his log shows the line (the positive control), a player's log, the combat a
// player reads and the stream's hints carry nothing of it, and the turn is not undoable across it.
func TestRN20_TheRechargeRollIsTheMastersAlone(t *testing.T) {
	t.Parallel()
	a, e, hound := houndFight(t, true)
	a.h.roller.queue(18)
	a.mustUseAction(t, a.master, e, hound, fireBreath, []string{"Toren"})
	w := a.caio.watch(t, a.campaignID)
	w.ready(t)
	a.h.roller.queue(5)
	e = a.ownTurnAgain(t, e, hound)
	if lines := a.rechargeLines(t, a.master, e); len(lines) != 1 || !lines[0].GetRecharged() || lines[0].GetFace() != 5 {
		t.Fatalf("the master's log has %v, want a 5 that recharged", lines)
	}
	for who, u := range map[string]*user{"Caio": a.caio, "Ana": a.ana} {
		if lines := a.rechargeLines(t, u, e); len(lines) != 0 {
			t.Errorf("%s reads %v in the log", who, lines)
		}
		if js := asJSON(t, a.log(t, u, e)) + asJSON(t, a.get(t, u)); strings.Contains(js, "echarge") || strings.Contains(js, "recharge") {
			t.Errorf("%s: the log or the combat carries the roll:\n%s", who, js)
		}
	}
	if id := a.log(t, a.master, e).GetUndoableEventId(); id != "" {
		t.Errorf("the master may undo %q across the turn start, want nothing", id)
	}
}

// The turn of a group starts every member at once: each one that spent a Recharge action rolls.
func TestRecharge_AGroupTurnRollsForEveryMember(t *testing.T) {
	t.Parallel()
	a := newCastingParty(t)
	e := a.start(t, plan{
		npcs: []*playv1.Participant{{CharacterId: a.goblin.GetId()}}, npcRolls: []int{1},
		players: map[string]int32{"Toren": 10, "Pensantus": 5, "Brisa": 1},
		reveal:  []string{"Goblin"}, setup: true, theatre: true,
	})
	a.h.roller.queue(20, 20) // both hounds roll the same initiative: one group
	res := a.mustAddMonsters(t, e, func(r *playv1.AddMonstersRequest) {
		r.CreatureKey, r.Count, r.Hidden = houndKey, 2, new(false)
	})
	e = a.begin(t, a.get(t, a.master))
	ids := res.GetCombatantIds()
	if len(e.GetTurnGroupIds()) != 2 {
		t.Fatalf("the turn group is %v, want both hounds", e.GetTurnGroupIds())
	}
	var labels []string
	for _, id := range ids {
		labels = append(labels, byLabelID(t, a.get(t, a.master), id))
		if _, err := bumpCreatureSpent(t.Context(), playdb.New(a.h.pool), id, spentRechargePrefix+fireBreath, 1); err != nil {
			t.Fatal(err)
		}
	}
	a.h.roller.queue(6, 1) // the first hound recharges, the second does not
	e = a.ownTurnAgain(t, e, labels[0])
	if a.spentOf(t, ids[0])[spentRechargePrefix+fireBreath] != 0 || a.spentOf(t, ids[1])[spentRechargePrefix+fireBreath] != 1 {
		t.Errorf("after the group's turn: %v and %v, want the first recharged and the second not", a.spentOf(t, ids[0]), a.spentOf(t, ids[1]))
	}
	if lines := a.rechargeLines(t, a.master, e); len(lines) != 2 {
		t.Errorf("the master's log has %v, want a roll for each hound", lines)
	}
}

// onFogMap plays the cave's fog fight with a hell hound added to it, whose breath is spent when spent is true,
// until the hound's turn starts. It returns how much each player's revision went up and what the master's log
// and the stored rows say of the Recharge roll.
func rechargeOnFogMap(t *testing.T, spent bool) (rise map[string]int32, lines int, rows []string) {
	t.Helper()
	f := newFogCave(t)
	e := f.fight(t)
	f.stage(t)
	res := f.mustAddMonsters(t, e, func(r *playv1.AddMonstersRequest) {
		r.CreatureKey, r.Count, r.Hidden = houndKey, 1, new(false)
	})
	hound := byLabelID(t, f.get(t, f.master), res.GetCombatantIds()[0])
	if spent {
		f.spendByHand(t, hound, spentRechargePrefix+fireBreath)
	}
	players := map[string]*user{"Toren's player": f.caio, "Pensantus's player": f.ana, "Brisa's player": f.bia}
	before := map[string]int32{}
	for name, u := range players {
		before[name] = f.get(t, u).GetRevision()
	}
	f.h.roller.queue(6)
	e = f.passTo(t, f.get(t, f.master), hound)
	rise = map[string]int32{}
	for name, u := range players {
		rise[name] = f.get(t, u).GetRevision() - before[name]
	}
	rowsOf, err := f.h.pool.Query(t.Context(), `SELECT coalesce(actor_user_id::text, '') || '|' || coalesce(character_id::text, '') FROM session_events WHERE kind = $1`, eventRechargeRolled)
	if err != nil {
		t.Fatalf("read the stored rows: %v", err)
	}
	defer rowsOf.Close()
	for rowsOf.Next() {
		var s string
		if err := rowsOf.Scan(&s); err != nil {
			t.Fatal(err)
		}
		rows = append(rows, s)
	}
	return rise, len(f.rechargeLines(t, f.master, e)), rows
}

// On a map with the fog a Recharge roll counts for no player: the revision a player reads goes up by the same
// amount whether the creature rolled or not, the player's log has no line, and the stored row names neither the
// user who ended the turn before nor a character. The master's log has the line (the positive control).
func TestRN20_TheRechargeRollIsNotCountedOnAFogMap(t *testing.T) {
	t.Parallel()
	control, controlLines, controlRows := rechargeOnFogMap(t, false)
	rolled, rolledLines, rolledRows := rechargeOnFogMap(t, true)
	if controlLines != 0 || len(controlRows) != 0 {
		t.Fatalf("the control rolled: %d lines, rows %v", controlLines, controlRows)
	}
	if rolledLines != 1 || len(rolledRows) != 1 {
		t.Fatalf("the master reads %d lines and %d rows of the roll, want one of each", rolledLines, len(rolledRows))
	}
	for name, n := range control {
		if rolled[name] != n {
			t.Errorf("%s: the revision went up by %d with the roll and by %d without it, want the same", name, rolled[name], n)
		}
	}
	if rolledRows[0] != "|" {
		t.Errorf("the stored row carries %q as user and character, want neither", rolledRows[0])
	}
}

// Without the fog the revision a player reads is the combat's own: it goes up by the same amount across a turn
// start whether the creature rolled a Recharge or not.
func TestRN20_TheRechargeRollIsNotCountedWithoutTheFog(t *testing.T) {
	t.Parallel()
	rise := func(spent bool) int32 {
		a, e, hound := houndFight(t, true)
		if spent {
			a.spendByHand(t, hound, spentRechargePrefix+fireBreath)
		}
		before := a.get(t, a.caio).GetRevision()
		a.h.roller.queue(6)
		a.ownTurnAgain(t, e, hound)
		if lines := len(a.rechargeLines(t, a.master, a.get(t, a.master))); lines != map[bool]int{true: 1, false: 0}[spent] {
			t.Fatalf("spent %v: the master reads %d roll lines", spent, lines)
		}
		return a.get(t, a.caio).GetRevision() - before
	}
	if with, without := rise(true), rise(false); with != without {
		t.Errorf("the revision went up by %d with a roll and by %d without, want the same", with, without)
	}
}

// The roll at a turn start cannot be undone: it is the latest event, it is no undoable kind, and the use before
// it stays spent.
func TestRecharge_TheRollAtTheTurnStartCannotBeUndone(t *testing.T) {
	t.Parallel()
	a, e, hound := houndFight(t, true)
	a.h.roller.queue(18)
	a.mustUseAction(t, a.master, e, hound, fireBreath, []string{"Toren"})
	a.h.roller.queue(6) // the Recharge roll recharges the breath
	e = a.ownTurnAgain(t, e, hound)
	if len(a.rechargeLines(t, a.master, e)) != 1 {
		t.Fatalf("the master reads no roll line: %v", a.rechargeLines(t, a.master, e))
	}
	events := a.events(t)
	wantBlockedBy(t, "UndoLastAction after the roll", a.undo(t, a.master, e, newKey()), blockedNothingToUndo)
	if a.events(t) != events {
		t.Errorf("the refused undo wrote events: %d (was %d)", a.events(t), events)
	}
}
