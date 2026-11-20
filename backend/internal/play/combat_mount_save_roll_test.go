package play

import (
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// The roll of the rider's save (RollMountSave): the Dexterity saving throw against DC 10 with the save machinery the other saves use,
// the held-roll gate, the fall and the landing square, the log and the undo. What opens the window is in combat_mount_save_open_test.go.

// rollSave calls RollMountSave as u for the rider.
func (c *cave) rollSave(t *testing.T, u *user, rider string, set func(*playv1.RollMountSaveRequest)) (*playv1.RollMountSaveResponse, error) {
	t.Helper()
	req := &playv1.RollMountSaveRequest{CampaignId: c.campaignID, EncounterId: c.get(t, c.master).GetId(), RiderId: c.id(t, rider), IdempotencyKey: newKey()}
	set(req)
	res, err := u.combat.RollMountSave(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func (c *cave) mustRollSave(t *testing.T, u *user, rider string, set func(*playv1.RollMountSaveRequest)) *playv1.RollMountSaveResponse {
	t.Helper()
	res, err := c.rollSave(t, u, rider, set)
	if err != nil {
		t.Fatalf("RollMountSave(%s) error = %v", rider, err)
	}
	return res
}

func saveFace(face int32) func(*playv1.RollMountSaveRequest) {
	return func(r *playv1.RollMountSaveRequest) { r.Roll = &playv1.RollMountSaveRequest_D20Face{D20Face: face} }
}

func saveFaces(first, second int32) func(*playv1.RollMountSaveRequest) {
	return func(r *playv1.RollMountSaveRequest) {
		r.Roll = &playv1.RollMountSaveRequest_D20Face{D20Face: first}
		r.SecondD20Face = &second
	}
}

func saveInApp(r *playv1.RollMountSaveRequest) {
	r.Roll = &playv1.RollMountSaveRequest_RollInApp{RollInApp: true}
}

// rowOf is what the table keeps of a combatant for these tests.
type saveRow struct {
	Prone       bool
	Initiative  int32
	PriorInit   int
	Defeated    bool
	Hidden      bool
	MountedOnID string
}

func (c *cave) saveRowOf(t *testing.T, label string) saveRow {
	t.Helper()
	var r saveRow
	var on *string
	var conds []string
	err := c.h.pool.QueryRow(t.Context(),
		`SELECT conditions, initiative, (CASE WHEN mount_prior_init IS NULL OR mount_prior_init::text IN ('null', '{}', '[]') THEN 0 ELSE 1 END), defeated, hidden, mounted_on_id::text FROM combatants WHERE id = $1`, c.id(t, label),
	).Scan(&conds, &r.Initiative, &r.PriorInit, &r.Defeated, &r.Hidden, &on)
	if err != nil {
		t.Fatalf("read %s: %v", label, err)
	}
	for _, k := range conds {
		r.Prone = r.Prone || k == condProne
	}
	if on != nil {
		r.MountedOnID = *on
	}
	return r
}

// rogueCave is the ridden cave with a rogue Toren: Dexterity 16 with the human's +1 (+3) and the proficiency in Dexterity saving throws (+2).
func rogueCave(t *testing.T) *cave {
	t.Helper()
	c := newCaveOf(t, 3, nil, func(a *armed) *charactersv1.Character {
		return a.caio.hero(t, a.campaignID, "Toren", "class:rogue", "race:human", 2,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 15, Constitution: 14, Intelligence: 10, Wisdom: 14, Charisma: 8}, nil, nil)
	})
	c.ridden(t)
	return c
}

func riddenCave(t *testing.T) *cave {
	t.Helper()
	c := newCave(t)
	c.ridden(t)
	return c
}

// The DC is 10 and the modifier is the rider's Dexterity saving throw: the ability modifier, plus the proficiency bonus when the class
// has it; advantage and disadvantage take the d20 the usual way, and a rider that auto-fails Dexterity saves rolls nothing.
func TestMountedCombat_TheSaveIsADexteritySavingThrowAgainstDC10(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name     string
		build    func(t *testing.T) *cave
		prepare  func(t *testing.T, c *cave)
		roll     func(*playv1.RollMountSaveRequest)
		held     bool
		modifier int32
		auto     bool
		dice     int
	}{
		{name: "not proficient, 8 + 2 reaches 10", build: riddenCave, roll: saveFace(8), held: true, modifier: 2, dice: 1},
		{name: "not proficient, 7 + 2 is 9", build: riddenCave, roll: saveFace(7), modifier: 2, dice: 1},
		{name: "proficient, 5 + 5 reaches 10", build: rogueCave, roll: saveFace(5), held: true, modifier: 5, dice: 1},
		{name: "proficient, 4 + 5 is 9", build: rogueCave, roll: saveFace(4), modifier: 5, dice: 1},
		{
			name: "advantage from the Dodge action takes the better d20", build: riddenCave, roll: saveFaces(3, 15), held: true, modifier: 2, dice: 2,
			prepare: func(t *testing.T, c *cave) {
				t.Helper()
				if _, err := c.action(t, c.caio, c.get(t, c.master), "Toren", "standard:dodge"); err != nil {
					t.Fatalf("Dodge error = %v", err)
				}
			},
		},
		{
			name: "disadvantage from restrained takes the worse d20", build: riddenCave, roll: saveFaces(15, 3), modifier: 2, dice: 2,
			prepare: func(t *testing.T, c *cave) {
				t.Helper()
				if _, err := c.conditions(t, c.master, c.get(t, c.master), "Toren", []string{"condition:restrained"}, true, false); err != nil {
					t.Fatalf("restrain error = %v", err)
				}
			},
		},
		{
			name: "paralyzed fails with no d20, even with a 20", build: riddenCave, roll: saveFace(20), auto: true,
			prepare: func(t *testing.T, c *cave) {
				t.Helper()
				if _, err := c.conditions(t, c.master, c.get(t, c.master), "Toren", []string{"condition:paralyzed"}, true, false); err != nil {
					t.Fatalf("paralyze error = %v", err)
				}
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := tt.build(t)
			if tt.prepare != nil {
				tt.prepare(t, c)
			}
			c.askSave(t, "Toren", causePushed)
			if got := c.mountSaveOf(t, c.caio); got.GetAutoFail() != tt.auto {
				t.Errorf("the prompt auto_fail = %v, want %v", got.GetAutoFail(), tt.auto)
			}
			res := c.mustRollSave(t, c.master, "Toren", tt.roll) // the master reads the DC whatever the table shows
			got := res.GetResult()
			if got.GetHeld() != tt.held || got.GetAutoFail() != tt.auto || got.GetDc() != 10 {
				t.Fatalf("result = %v, want held %v, auto_fail %v and DC 10", got, tt.held, tt.auto)
			}
			if tt.auto {
				if len(got.GetSave().GetFaces()) != 0 {
					t.Errorf("an automatic failure shows the d20 %v", got.GetSave().GetFaces())
				}
				return
			}
			if got.GetSave().GetModifier() != tt.modifier || len(got.GetSave().GetFaces()) != tt.dice {
				t.Errorf("save = %v, want modifier %d and %d d20", got.GetSave(), tt.modifier, tt.dice)
			}
		})
	}
}

// A save that holds changes nothing but the log and the window: the rider stays on its mount, on its feet, and the mount keeps its
// rider's initiative. A retry of the key is the same answer and writes nothing; another key finds no window.
func TestMountedCombat_AHeldSaveOnlyWritesALogLine(t *testing.T) {
	t.Parallel()
	c := riddenCave(t)
	c.askSave(t, "Toren", causePushed)
	before := c.saveRowOf(t, "Ogro")
	key := newKey()
	set := func(r *playv1.RollMountSaveRequest) { saveFace(15)(r); r.IdempotencyKey = key }
	res := c.mustRollSave(t, c.caio, "Toren", set)
	if !res.GetResult().GetHeld() {
		t.Fatalf("result = %v, want held", res.GetResult())
	}
	if got := res.GetEncounter().GetMountSave(); got != nil {
		t.Errorf("the answer still reads the save %v", got)
	}
	if c.mountSaveOf(t, c.master) != nil || len(c.openSaveWindows(t)) != 0 {
		t.Error("the window stays open after the answer")
	}
	toren, ogro := c.saveRowOf(t, "Toren"), c.saveRowOf(t, "Ogro")
	if toren.Prone || toren.MountedOnID != c.id(t, "Ogro") || ogro != before {
		t.Errorf("rider %+v, mount %+v (was %+v): want the rider on its feet and on the mount, the mount as it was", toren, ogro, before)
	}
	if got := c.eventsOf(t, "mount_save_rolled"); got != 1 {
		t.Errorf("mount_save_rolled events = %d, want 1", got)
	}
	lines := c.linesOfKind(t, c.ana, playv1.CombatLogKind_COMBAT_LOG_KIND_MOUNT_SAVE)
	if len(lines) != 1 || !lines[0].GetMountSave().GetHeld() || lines[0].GetActorLabel() != "Toren" {
		t.Fatalf("another player's log = %v, want one MOUNT_SAVE line, held", lines)
	}
	if ms := lines[0].GetMountSave(); ms.GetD20() != 0 || ms.GetTotal() != 0 {
		t.Errorf("another player reads the d20 %d / total %d, want neither", ms.GetD20(), ms.GetTotal())
	}
	for who, u := range map[string]*user{"the master": c.master, "Toren's player": c.caio} {
		ls := c.linesOfKind(t, u, playv1.CombatLogKind_COMBAT_LOG_KIND_MOUNT_SAVE)
		if len(ls) != 1 || ls[0].GetMountSave().GetD20() != 15 || ls[0].GetMountSave().GetTotal() != 17 {
			t.Errorf("%s reads %v, want the d20 15 and the total 17", who, ls)
		}
	}
	again := c.mustRollSave(t, c.caio, "Toren", set)
	if !again.GetResult().GetHeld() || again.GetResult().GetSave().GetFaces()[0] != 15 || c.eventsOf(t, "mount_save_rolled") != 1 {
		t.Errorf("the retry = %v, events %d: want the same answer, nothing written", again.GetResult(), c.eventsOf(t, "mount_save_rolled"))
	}
	_, err := c.rollSave(t, c.caio, "Toren", saveFace(15))
	wantBlockedBy(t, "a second save", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_YOUR_TURN_TO_ANSWER)
}

// A save that fails throws the rider: Prone, on the nearest free neighbor of the mount (west of it, the north one being a wall),
// off the mount, with the mount's own initiative back, and the turn is free again.
func TestMountedCombat_AFailedSaveThrowsTheRider(t *testing.T) {
	t.Parallel()
	c := riddenCave(t)
	c.askSave(t, "Toren", causePushed)
	rider, mount := c.saveRowOf(t, "Toren"), c.saveRowOf(t, "Ogro")
	if mount.PriorInit == 0 || mount.Initiative != rider.Initiative {
		t.Fatalf("setup: the mount %+v should carry the rider's initiative %d", mount, rider.Initiative)
	}
	res := c.mustRollSave(t, c.caio, "Toren", saveFace(7))
	got := res.GetResult()
	if got.GetHeld() || got.GetLandedCol() != 6 || got.GetLandedRow() != 7 || got.GetNoRoomNextToMount() {
		t.Fatalf("result = %v, want a fall to (6,7) with room", got)
	}
	wantSquare(t, c, "Toren", [2]int32{6, 7})
	wantSquare(t, c, "Ogro", [2]int32{7, 7})
	toren, ogro := c.saveRowOf(t, "Toren"), c.saveRowOf(t, "Ogro")
	if !toren.Prone || toren.MountedOnID != "" {
		t.Errorf("rider after the fall = %+v, want prone and off the mount", toren)
	}
	if ogro.PriorInit != 0 || ogro.Initiative == toren.Initiative {
		t.Errorf("mount after the fall = %+v, want its own initiative back (the rider's is %d)", ogro, toren.Initiative)
	}
	if r := c.who(t, c.ana, "Toren"); r.GetMountedOnId() != "" || r.GetRiderId() != "" {
		t.Errorf("another player reads the pair %v, want none", r)
	}
	if c.mountSaveOf(t, c.master) != nil {
		t.Error("the window stays open after the fall")
	}
	if _, err := c.move(t, c.caio, "Toren", 5, 7); err != nil {
		t.Errorf("the rider's move after the fall error = %v, want the turn free", err)
	}
	for who, u := range map[string]*user{"the master": c.master, "another player": c.ana} {
		ls := c.linesOfKind(t, u, playv1.CombatLogKind_COMBAT_LOG_KIND_RIDER_THROWN)
		if len(ls) != 1 || ls[0].GetActorLabel() != "Toren" || ls[0].GetMountSave().GetLandedCol() != 6 || ls[0].GetMountSave().GetLandedRow() != 7 {
			t.Errorf("%s reads %v, want one RIDER_THROWN line of Toren landing on (6,7)", who, ls)
		}
	}
	if ls := c.linesOfKind(t, c.ana, playv1.CombatLogKind_COMBAT_LOG_KIND_RIDER_THROWN); len(ls) == 1 && (ls[0].GetMountSave().GetD20() != 0 || ls[0].GetMountSave().GetTotal() != 0) {
		t.Errorf("another player reads the d20 of the fall: %v", ls[0].GetMountSave())
	}
}

// The landing is the nearest free neighbor, taken in the order of distance, then row, then column: a neighbor that a creature holds
// is skipped, a hidden creature included (nobody lands on top of it, and nobody is told why). With every neighbor taken the rider
// takes the nearest free square and the line says so.
func TestMountedCombat_TheLandingSquare(t *testing.T) {
	t.Parallel()
	// Goblin 1 stands on (8,7) in every case; the wall is north of the mount.
	for _, tt := range []struct {
		name   string
		held   [][2]int32 // the squares the other creatures are put on, in the order of the blockers
		hidden bool       // the first of them is hidden
		want   [2]int32
		noRoom bool
	}{
		{name: "west is free", want: [2]int32{6, 7}},
		{name: "west is taken", held: [][2]int32{{6, 7}}, want: [2]int32{7, 8}},
		{name: "a hidden creature holds the west square", held: [][2]int32{{6, 7}}, hidden: true, want: [2]int32{7, 8}},
		{name: "every neighbor is taken", held: [][2]int32{{6, 6}, {8, 6}, {6, 7}, {6, 8}, {7, 8}, {8, 8}}, want: [2]int32{5, 7}, noRoom: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := riddenCave(t)
			blockers := []string{"Goblin 2", "Goblin 3", "Capitão Goblin", "Escudeiro", "Pensantus", "Brisa"}
			for i, sq := range tt.held {
				c.execSQL(t, `UPDATE combatants SET grid_col = $1, grid_row = $2 WHERE id = $3`, sq[0], sq[1], c.id(t, blockers[i]))
			}
			if tt.hidden {
				c.execSQL(t, `UPDATE combatants SET hidden = true WHERE id = $1`, c.id(t, blockers[0]))
			}
			c.askSave(t, "Toren", causePushed)
			got := c.mustRollSave(t, c.master, "Toren", saveFace(1)).GetResult() // the note is the master's
			if got.GetHeld() || [2]int32{got.GetLandedCol(), got.GetLandedRow()} != tt.want || got.GetNoRoomNextToMount() != tt.noRoom {
				t.Errorf("result = %v, want a fall to %v (no room %v)", got, tt.want, tt.noRoom)
			}
			wantSquare(t, c, "Toren", tt.want)
		})
	}
}

// The roll is the rider's player's or the master's: another player is refused, and a rider that is not on the table of a player is not
// found. The master rolls for an NPC rider, with the roller's die, and reads the DC.
func TestMountedCombat_WhoRollsTheSave(t *testing.T) {
	t.Parallel()
	c := riddenCave(t)
	c.askSave(t, "Toren", causePushed)
	_, err := c.rollSave(t, c.ana, "Toren", saveFace(15))
	wantCode(t, "another player's roll", err, connect.CodePermissionDenied)
	if got := c.eventsOf(t, "mount_save_rolled"); got != 0 {
		t.Fatalf("a refused roll wrote %d events", got)
	}
	res := c.mustRollSave(t, c.master, "Toren", saveFace(12))
	if !res.GetResult().GetHeld() || res.GetResult().GetDc() != 10 {
		t.Errorf("the master's roll for Toren = %v, want held against DC 10", res.GetResult())
	}

	// An NPC rider: only the master rolls it, with the app's die.
	n := newCave(t)
	n.fight(t)
	n.steed(t)
	n.mustMove(t, n.master, "Goblin 1", 6, 7)
	n.sitOn(t, "Goblin 1", "Ogro")
	n.askSave(t, "Goblin 1", causePushed)
	_, err = n.rollSave(t, n.caio, "Goblin 1", saveFace(15))
	wantCode(t, "a player's roll for an NPC", err, connect.CodePermissionDenied)
	n.h.roller.queue(6)
	got := n.mustRollSave(t, n.master, "Goblin 1", saveInApp).GetResult()
	if got.GetSave().GetFaces()[0] != 6 || got.GetHeld() { // 6 + 2 (Dexterity 14 of the Goblin) is 8
		t.Errorf("the master's roll for the Goblin = %v, want the roller's 6 and a fall", got)
	}
	if r := n.saveRowOf(t, "Goblin 1"); !r.Prone || r.MountedOnID != "" {
		t.Errorf("the Goblin after the fall = %+v, want prone and off the mount", r)
	}
}

// A rider that holds a Bardic Inspiration die, or is a halfling and rolled a 1, is asked about it before anything is written: the
// held-roll gate (kind mount save). AnswerHeldRoll settles it with the die or the rerolled d20.
func TestMountedCombat_TheSaveAsksLuckyAndTheBardicDie(t *testing.T) {
	t.Parallel()
	t.Run("a Bardic Inspiration die", func(t *testing.T) {
		t.Parallel()
		c := riddenCave(t)
		c.giveCombatantDie(t, "Toren", c.get(t, c.master))
		c.askSave(t, "Toren", causePushed)
		res := c.mustRollSave(t, c.caio, "Toren", saveFace(3)) // 3 + 2 = 5: a fall without the die
		if res.GetResult() != nil || res.GetHeldRollOffer() == nil || res.GetHeldRollOffer().GetKind() != playv1.HeldRollKind_HELD_ROLL_KIND_MOUNT_SAVE {
			t.Fatalf("answer = %v, want the question of a mount save alone", res)
		}
		if len(c.openSaveWindows(t)) != 1 || c.eventsOf(t, "mount_save_rolled") != 0 {
			t.Error("the window closed or an event was written before the player answered")
		}
		c.h.roller.queue(8)
		out, err := c.answerHeldRoll(t, c.caio, res.GetHeldRollOffer().GetHoldId(), dieAnswer(true)) // the d20 3 and the modifier 2 with the die 8 make 13
		if err != nil {
			t.Fatalf("AnswerHeldRoll() error = %v", err)
		}
		got := out.GetMountSave().GetResult()
		if !got.GetHeld() || len(got.GetBonusDice()) != 1 || got.GetSave().GetTotal() != 13 {
			t.Errorf("the settled save = %v, want held with a die, total 13", got)
		}
		if r := c.saveRowOf(t, "Toren"); r.Prone || r.MountedOnID == "" {
			t.Errorf("the rider after the answer = %+v, want on the mount", r)
		}
	})
	t.Run("a halfling's 1", func(t *testing.T) {
		t.Parallel()
		c := newCave(t)
		c.fight(t)
		c.steed(t)
		c.passTo(t, c.get(t, c.master), "Brisa")
		c.sitOn(t, "Brisa", "Ogro")
		c.askSave(t, "Brisa", causePushed)
		res := c.mustRollSave(t, c.bia, "Brisa", saveFace(1))
		if res.GetResult() != nil || res.GetHeldRollOffer() == nil {
			t.Fatalf("answer = %v, want Lucky's question alone", res)
		}
		out, err := c.answerHeldRoll(t, c.bia, res.GetHeldRollOffer().GetHoldId(), luckyAnswer(17)) // 17 + 3 = 20
		if err != nil {
			t.Fatalf("AnswerHeldRoll() error = %v", err)
		}
		got := out.GetMountSave().GetResult()
		if !got.GetHeld() || len(got.GetSave().GetRerolled()) != 1 || got.GetSave().GetFaces()[0] != 17 {
			t.Errorf("the settled save = %v, want the 1 rolled again to 17, held", got)
		}
	})
}

// A held mount save writes only the held roll, yet the table's combat changed (the rider's d20 is kept and asked about): every
// screen is told, so the master reads the question at once instead of on the next change.
func TestMountedCombat_AHeldMountSavePublishesTheEncounterChange(t *testing.T) {
	t.Parallel()
	c := riddenCave(t)
	c.giveCombatantDie(t, "Toren", c.get(t, c.master))
	c.askSave(t, "Toren", causePushed)
	masterStream := c.master.watch(t, c.campaignID)
	masterStream.ready(t)
	res := c.mustRollSave(t, c.caio, "Toren", saveFace(3))
	if res.GetHeldRollOffer() == nil {
		t.Fatalf("answer = %v, want the question of a mount save", res)
	}
	var changed int
	for _, ev := range c.streamHeard(t, masterStream) {
		if ev.GetEncounterChanged() != nil {
			changed++
		}
	}
	if changed != 1 {
		t.Errorf("the master heard %d encounter_changed after the held question, want 1", changed)
	}
}

// A question kept for the rider (the Bardic die, Lucky) does not outlive the window it was asked for: when the window closes by itself,
// the held roll goes with it and asks nothing of the rider any more.
func TestMountedCombat_TheHeldQuestionGoesWithItsWindow(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name  string
		clear func(t *testing.T, c *cave)
	}{
		{"the mount is removed", func(t *testing.T, c *cave) { t.Helper(); c.removeFromCombat(t, "Ogro") }},
		{"the rider gets off", func(t *testing.T, c *cave) {
			t.Helper()
			c.execSQL(t, `UPDATE combatants SET mounted_on_id = NULL WHERE id = $1`, c.id(t, "Toren"))
			c.touchCombat(t)
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := riddenCave(t)
			c.giveCombatantDie(t, "Toren", c.get(t, c.master))
			c.askSave(t, "Toren", causePushed)
			res := c.mustRollSave(t, c.caio, "Toren", saveFace(3))
			if res.GetHeldRollOffer() == nil {
				t.Fatalf("answer = %v, want the question of a mount save", res)
			}
			tt.clear(t, c)
			if n := len(c.openSaveWindows(t)); n != 0 {
				t.Fatalf("open mount_save windows = %d, want 0: the setup does not close the window", n)
			}
			var held int
			if err := c.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM inspiration_holds WHERE kind = 'mount_save'`).Scan(&held); err != nil {
				t.Fatalf("count the held rolls: %v", err)
			}
			if held != 0 {
				t.Errorf("held mount saves = %d after the window closed, want 0", held)
			}
		})
	}
}
