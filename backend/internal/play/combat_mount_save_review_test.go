package play

import (
	"strings"
	"testing"

	"connectrpc.com/connect"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// What a rider's save owes the rules of the table and of the viewer (RN-10, RN-20, RN-24), and the paths that open it that the first
// tests did not cover: a rider at 0 hit points, the table's rule on the DC, the fog, the master's note about room, a creature as rider,
// the hit rider and the trap that knock prone, and the pushes that move nobody.

// A rider at 0 hit points that is alive (a dying character) is unconscious: it auto-fails a Dexterity saving throw (SRD 5.1,
// Conditions), so nothing is asked. It is thrown at once, as a failed save throws it, with the line of the fall and no d20. A defeated
// NPC rider is skipped: the fall of the dead belongs to another rule.
func TestMountedCombat_ADyingRiderIsThrownWithNoPrompt(t *testing.T) {
	t.Parallel()
	c := riddenCave(t)
	c.downToren(t)
	c.askSave(t, "Toren", causePushed)
	if got := c.mountSaveOf(t, c.master); got != nil || len(c.openSaveWindows(t)) != 0 {
		t.Errorf("the master reads the save %v, want none: a rider at 0 hit points is thrown with no question", got)
	}
	toren, ogro := c.saveRowOf(t, "Toren"), c.saveRowOf(t, "Ogro")
	if !toren.Prone || toren.MountedOnID != "" {
		t.Errorf("rider after the fall = %+v, want prone and off the mount", toren)
	}
	if ogro.PriorInit != 0 {
		t.Errorf("mount after the fall = %+v, want its own initiative back", ogro)
	}
	wantSquare(t, c, "Toren", [2]int32{6, 7})
	for who, u := range map[string]*user{"the master": c.master, "Toren's player": c.caio, "another player": c.ana} {
		ls := c.linesOfKind(t, u, playv1.CombatLogKind_COMBAT_LOG_KIND_RIDER_THROWN)
		if len(ls) != 1 || ls[0].GetMountSave().GetD20() != 0 || ls[0].GetMountSave().GetLandedCol() != 6 {
			t.Errorf("%s reads %v, want one RIDER_THROWN line, landing on (6,7), with no d20", who, ls)
		}
	}
	if _, err := c.rollSave(t, c.caio, "Toren", saveFace(20)); err == nil {
		t.Error("RollMountSave after the fall succeeded, want it refused: nothing is asked")
	}
}

func TestMountedCombat_ADefeatedRiderIsNotAsked(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.fight(t)
	c.steed(t)
	c.mustMove(t, c.master, "Goblin 1", 6, 7)
	c.sitOn(t, "Goblin 1", "Ogro")
	c.execSQL(t, `UPDATE combatants SET defeated = true WHERE id = $1`, c.id(t, "Goblin 1"))
	c.askSave(t, "Goblin 1", causePushed)
	if len(c.openSaveWindows(t)) != 0 || c.saveRowOf(t, "Goblin 1").MountedOnID == "" {
		t.Error("a defeated rider was asked or thrown, want it left as it is")
	}
}

// The window of a rider that dies while it waits is closed by the next change.
func TestMountedCombat_TheWindowClearsWhenTheRiderIsDefeated(t *testing.T) {
	t.Parallel()
	c := riddenCave(t)
	c.askSave(t, "Toren", causePushed)
	c.execSQL(t, `UPDATE combatants SET defeated = true WHERE id = $1`, c.id(t, "Toren"))
	c.touchCombat(t)
	if n := len(c.openSaveWindows(t)); n != 0 {
		t.Errorf("open mount_save windows = %d, want 0 for a defeated rider", n)
	}
}

// The table's rule "os jogadores veem a CD dos testes de resistência" (RN-24): off, the rider's player reads no DC in the prompt, the
// result (the retry too) or the log; on, the DC is 10 for the player. The master reads it either way.
func TestMountedCombat_TheDCFollowsTheTableRule(t *testing.T) {
	t.Parallel()
	for _, shown := range []bool{true, false} {
		name := "the rule off"
		want := int32(0)
		if shown {
			name, want = "the rule on", 10
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := riddenCave(t)
			c.setRules(t, func(r *campaignsv1.TableRules) { r.ShowSaveDc = shown })
			c.askSave(t, "Toren", causePushed)
			if got := c.mountSaveOf(t, c.caio).GetDc(); got != want {
				t.Errorf("the player's prompt DC = %d, want %d", got, want)
			}
			if got := c.mountSaveOf(t, c.master).GetDc(); got != 10 {
				t.Errorf("the master's prompt DC = %d, want 10", got)
			}
			key := newKey()
			set := func(r *playv1.RollMountSaveRequest) { saveFace(15)(r); r.IdempotencyKey = key }
			if got := c.mustRollSave(t, c.caio, "Toren", set).GetResult().GetDc(); got != want {
				t.Errorf("the player's result DC = %d, want %d", got, want)
			}
			if got := c.mustRollSave(t, c.caio, "Toren", set).GetResult().GetDc(); got != want {
				t.Errorf("the retry's result DC = %d, want %d", got, want)
			}
			for who, u := range map[string]*user{"Toren's player": c.caio} {
				ls := c.linesOfKind(t, u, playv1.CombatLogKind_COMBAT_LOG_KIND_MOUNT_SAVE)
				if len(ls) != 1 || ls[0].GetMountSave().GetDc() != want || ls[0].GetMountSave().GetD20() != 15 {
					t.Errorf("%s reads %v, want the d20 15 and the DC %d", who, ls, want)
				}
			}
			if ls := c.linesOfKind(t, c.master, playv1.CombatLogKind_COMBAT_LOG_KIND_MOUNT_SAVE); len(ls) != 1 || ls[0].GetMountSave().GetDc() != 10 {
				t.Errorf("the master reads %v, want the DC 10", ls)
			}
		})
	}
}

// The fog: a character the player sees on a mount the player does not see waits as any hidden reaction does (REACTION_PENDING,
// naming nobody), as it does for a mount the master hid. The master reads the save (the control).
func TestMountedCombat_AMountTheFogHidesIsNotNamed(t *testing.T) {
	t.Parallel()
	f := newFogCave(t)
	f.fight(t)
	f.stage(t)
	f.torch(t, false)
	e := f.passTo(t, f.get(t, f.master), "Pensantus")
	f.sitOn(t, "Toren", "Ogro")
	if ogro := f.who(t, f.master, "Ogro"); f.seesSquare(t, f.ana, int(ogro.GetCol()), int(ogro.GetRow())) {
		t.Fatal("the setup failed: Pensantus's player sees the Ogro")
	}
	f.askSave(t, "Toren", causePushed)
	if got := f.mountSaveOf(t, f.master); got.GetMountId() != f.id(t, "Ogro") {
		t.Fatalf("the master reads %v, want the save on the Ogro (the control)", got)
	}
	_, err := f.endTurn(t, f.ana, e, false)
	b := wantBlockedBy(t, "Pensantus's end of turn", err, reasonReactionPending)
	if len(b.GetCombatantIds()) != 0 || strings.Contains(err.Error(), "Ogro") {
		t.Errorf("the refusal %v names somebody", b)
	}
	_, err = f.endTurn(t, f.master, e, false)
	wantBlockedBy(t, "the master's end of turn", err, reasonMountSavePending)
}

// The master's note that no square next to the mount was free is the master's alone: the player who rolls, the retry, the log of
// another player and the result read nothing of it, a hidden blocker included.
func TestMountedCombat_NoRoomNextToTheMountIsTheMastersNote(t *testing.T) {
	t.Parallel()
	c := riddenCave(t)
	blockers := []string{"Goblin 2", "Goblin 3", "Capitão Goblin", "Escudeiro", "Pensantus", "Brisa"}
	for i, sq := range [][2]int32{{6, 6}, {8, 6}, {6, 7}, {6, 8}, {7, 8}, {8, 8}} {
		c.execSQL(t, `UPDATE combatants SET grid_col = $1, grid_row = $2 WHERE id = $3`, sq[0], sq[1], c.id(t, blockers[i]))
	}
	c.execSQL(t, `UPDATE combatants SET hidden = true WHERE id = $1`, c.id(t, "Goblin 2"))
	c.askSave(t, "Toren", causePushed)
	key := newKey()
	set := func(r *playv1.RollMountSaveRequest) { saveFace(1)(r); r.IdempotencyKey = key }
	if got := c.mustRollSave(t, c.caio, "Toren", set).GetResult(); got.GetNoRoomNextToMount() || got.GetLandedCol() != 5 {
		t.Errorf("the player's result = %v, want the fall to (5,7) and no note", got)
	}
	if got := c.mustRollSave(t, c.caio, "Toren", set).GetResult(); got.GetNoRoomNextToMount() {
		t.Errorf("the player's retry = %v, want no note", got)
	}
	for who, u := range map[string]*user{"Toren's player": c.caio, "another player": c.ana} {
		ls := c.linesOfKind(t, u, playv1.CombatLogKind_COMBAT_LOG_KIND_RIDER_THROWN)
		if len(ls) != 1 || ls[0].GetMountSave().GetNoRoomNextToMount() {
			t.Errorf("%s reads %v, want one RIDER_THROWN line with no note", who, ls)
		}
	}
	if ls := c.linesOfKind(t, c.master, playv1.CombatLogKind_COMBAT_LOG_KIND_RIDER_THROWN); len(ls) != 1 || !ls[0].GetMountSave().GetNoRoomNextToMount() {
		t.Errorf("the master reads %v, want the note (the control)", ls)
	}
}

// A player's creature that rides is the master's to answer: its owner is not prompted and cannot roll, and reads that the master waits.
func TestMountedCombat_ACreatureRiderIsAnsweredByTheMaster(t *testing.T) {
	t.Parallel()
	a := newSummoners(t)
	e := a.toSalvia(t, a.summonersFight(t))
	if _, err := a.summonWolves(t, a.bia, e, 11); err != nil {
		t.Fatalf("CastSpell() error = %v", err)
	}
	a.execSQL(t, `UPDATE combatants SET mounted_on_id = (SELECT id FROM combatants WHERE label = 'Lobo atroz 2'), grid_col = (SELECT grid_col FROM combatants WHERE label = 'Lobo atroz 2'), grid_row = (SELECT grid_row FROM combatants WHERE label = 'Lobo atroz 2') WHERE label = 'Lobo atroz 1'`)
	a.askSave(t, "Lobo atroz 1", causePushed)
	if got := a.mountSaveOf(t, a.master); got.GetRiderId() != a.id(t, "Lobo atroz 1") {
		t.Fatalf("the master reads %v, want the save of the creature (the control)", got)
	}
	if got := a.mountSaveOf(t, a.bia); got != nil {
		t.Errorf("the creature's owner reads the save %v, want none", got)
	}
	req := &playv1.RollMountSaveRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), RiderId: a.id(t, "Lobo atroz 1"), IdempotencyKey: newKey(), Roll: &playv1.RollMountSaveRequest_D20Face{D20Face: 15}}
	_, err := a.bia.combat.RollMountSave(t.Context(), connect.NewRequest(req))
	wantCode(t, "the owner's roll for the creature", err, connect.CodePermissionDenied)
	if w := a.get(t, a.bia).GetReactionWait(); w == nil || !strings.Contains(w.GetTitlePt(), "mestre") {
		t.Errorf("the owner reads the wait %v, want the master's", w)
	}
}

// Open Hand: the push of a mount carrying a rider and the prone of a rider both ask the save; a push that moves nobody asks nothing.
func TestMountedCombat_TheOpenHandAsksTheRidersSave(t *testing.T) {
	t.Parallel()
	t.Run("the push of the mount", func(t *testing.T) {
		t.Parallel()
		a, e, offers := riderMonkFight(t)
		a.execSQL(t, `UPDATE combatants SET mounted_on_id = (SELECT id FROM combatants WHERE label = 'Goblin'), grid_col = (SELECT grid_col FROM combatants WHERE label = 'Goblin'), grid_row = (SELECT grid_row FROM combatants WHERE label = 'Goblin') WHERE label = 'Brisa'`)
		a.h.roller.queue(1)
		if _, err := a.useRider(t, a.caio, e, offers[playv1.HitRiderKind_HIT_RIDER_KIND_OPEN_HAND], playv1.HitRiderChoice_HIT_RIDER_CHOICE_PUSH); err != nil {
			t.Fatalf("UseHitRider() error = %v", err)
		}
		if got := a.get(t, a.master).GetMountSave(); got.GetRiderId() != a.id(t, "Brisa") || got.GetCause() != causePushed {
			t.Errorf("the master reads %v, want the push save of Brisa", got)
		}
	})
	t.Run("a push that moves nobody", func(t *testing.T) {
		t.Parallel()
		a, e, offers := riderMonkFight(t)
		a.execSQL(t, `UPDATE combatants SET mounted_on_id = (SELECT id FROM combatants WHERE label = 'Goblin'), grid_col = (SELECT grid_col FROM combatants WHERE label = 'Goblin'), grid_row = (SELECT grid_row FROM combatants WHERE label = 'Goblin') WHERE label = 'Brisa'`)
		a.execSQL(t, `UPDATE combatants SET grid_col = 5, grid_row = 3 WHERE label = 'Pensantus'`) // in the way, right behind the Goblin
		a.h.roller.queue(1)
		if _, err := a.useRider(t, a.caio, e, offers[playv1.HitRiderKind_HIT_RIDER_KIND_OPEN_HAND], playv1.HitRiderChoice_HIT_RIDER_CHOICE_PUSH); err != nil {
			t.Fatalf("UseHitRider() error = %v", err)
		}
		if who := byLabel(t, a.get(t, a.master), "Goblin"); who.GetCol() != 4 {
			t.Fatalf("the Goblin stands at %d, want 4: the setup does not block the push", who.GetCol())
		}
		if got := a.get(t, a.master).GetMountSave(); got != nil {
			t.Errorf("a push that moved nobody asked %v", got)
		}
	})
	t.Run("the prone of the rider", func(t *testing.T) {
		t.Parallel()
		a, e, offers := riderMonkFight(t)
		a.execSQL(t, `UPDATE combatants p SET grid_col = g.grid_col, grid_row = g.grid_row FROM combatants g WHERE g.label = 'Goblin' AND p.label = 'Pensantus'`)
		a.execSQL(t, `UPDATE combatants SET mounted_on_id = (SELECT id FROM combatants WHERE label = 'Pensantus') WHERE label = 'Goblin'`)
		a.h.roller.queue(1)
		if _, err := a.useRider(t, a.caio, e, offers[playv1.HitRiderKind_HIT_RIDER_KIND_OPEN_HAND], playv1.HitRiderChoice_HIT_RIDER_CHOICE_PRONE); err != nil {
			t.Fatalf("UseHitRider() error = %v", err)
		}
		if got := a.get(t, a.master).GetMountSave(); got.GetRiderId() != a.id(t, "Goblin") || got.GetCause() != causeProne {
			t.Errorf("the master reads %v, want the prone save of the Goblin", got)
		}
	})
}

// A trap that knocks a rider prone asks its save.
func TestMountedCombat_ATrapThatKnocksTheRiderProneAsksItsSave(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	r.ridden(t)
	p := r.trap(t, "Fosso", 20, 12, manualTrap, func(s *mapsv1.TrapSpec) {
		s.Effect = &rulesv1.TrapEffect{Conditions: []*rulesv1.TrapCondition{{ConditionKey: "condition:prone"}}}
	})
	if _, err := r.fireRaw(t, p, func(q *playv1.FireTrapRequest) { q.TargetIds = []string{r.id(t, "Toren")} }); err != nil {
		t.Fatalf("FireTrap() error = %v", err)
	}
	wantSave(t, "after the trap", r.mountSaveOf(t, r.master), r.id(t, "Toren"), r.id(t, "Ogro"), causeProne)
}

// A push that throws a rider at 0 hit points at once leaves nothing to undo: the fall is written with the push and the rider does not
// get back on its mount, so the push is no action an undo can take back (as the push whose save was answered is not).
func TestMountedCombat_ThePushThatThrewADyingRiderCannotBeUndone(t *testing.T) {
	t.Parallel()
	c := newAreaCave(t)
	c.mountedWave(t)
	c.downToren(t)
	c.wave(t)
	if got := c.saveRowOf(t, "Toren"); !got.Prone || got.MountedOnID != "" {
		t.Fatalf("the rider after the push = %+v, want thrown", got)
	}
	if id := c.log(t, c.master, c.get(t, c.master)).GetUndoableEventId(); id != "" {
		t.Errorf("the log offers %s to undo, want nothing: the fall cannot be taken back", id)
	}
}

// The prompt carries the Dexterity saving throw bonus RollMountSave adds, so the screen can write "1d20 + 2" before the roll: it is the
// modifier of the save the same rider then rolls. A character's bonus is known.
func TestMountedCombat_ThePromptCarriesTheSaveBonus(t *testing.T) {
	t.Parallel()
	c := riddenCave(t)
	c.askSave(t, "Toren", causePushed)
	for who, u := range map[string]*user{"the rider's player": c.caio, "the master": c.master} {
		if got := c.mountSaveOf(t, u); !got.GetBonusKnown() {
			t.Errorf("%s reads bonus_known = false in %v, want a character's bonus known", who, got)
		}
	}
	asked := c.mountSaveOf(t, c.caio).GetSaveBonus()
	rolled := c.mustRollSave(t, c.caio, "Toren", saveFace(15)).GetResult().GetSave()
	if rolled.GetModifier() != asked || rolled.GetTotal() != 15+asked {
		t.Errorf("the prompt's save_bonus = %d, the roll = %v, want the roll's modifier and 15 plus it", asked, rolled)
	}
}
