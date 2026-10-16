package play

import (
	"fmt"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// Group A of the undo scenarios (see combat_undo_kinds_test.go): the master's Desfazer
// of a move, a trap's firing, the master's reveal of a hider and the events of the
// damage and the hit points. Each scenario is a branch of takeBack.

// deltaDoorOpened: a door a move opened stays open after the undo of the move, on
// purpose (RN-26, "a door opened stays opened"; the comment of lastAction in
// combat_undo.go). openDoors (combat_move.go:625) opens it through the maps module
// with the move's transaction and writes the door_opened event, and the
// eventCombatantMoved case of takeBack does not close it. The log line stays for the
// master and for each player who saw Toren, and the door's cell in the map's layers
// is the open one (1) where it was closed (2).
var deltaDoorOpened = func() undoDelta {
	line := func(state string) string {
		return `db/map_layers[.cover=\x000000000000000000000000000000000000000000000000000000000002000000000000000000000000000000004000000000004000000000000000000000000000000000000000000000000000000000000000000000000000000000000000,` +
			`.difficult_terrain=\x000000000000000000000000000000000000000000000000000000300000300000000000000000000000000000000000,` +
			`.doors=\x` + strings.Repeat("0", 177) + state + strings.Repeat("0", 206) + `,.light=<nil>,.map_id=<id>,` +
			`.walls=\xffffffffffffffff80ffff80ffff80ffff8081ff8000008000008081fc8081fcff3fc0ff3fc0ff3fc0ff3fc0ffffffff]={}`
	}
	out := undoDelta{"+ " + line("1"), "- " + line("2")}
	for _, who := range []string{"ana", "bia", "caio", "master"} {
		out = append(out, "+ "+who+"/log R1 DOOR_OPENED id=# Toren>")
	}
	return out
}()

// TestUndoKindCombatantMovedWalks: the plain walk (the square and the movement
// spent), a walk that follows another (the running start comes back as it was), the
// master's forced move of a combatant that carries a cover mark (the move cleared
// the mark) and both jumps.
func TestUndoKindCombatantMovedWalks(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.fight(t)
	w := newUndoWatch(t, c.armed)

	w.run(t, "a walk", nil, func() { c.mustMove(t, c.caio, "Toren", 9, 7) })
	c.mustMove(t, c.caio, "Toren", 9, 7)
	w.run(t, "a walk after another (the running start was set)", nil, func() { c.mustMove(t, c.caio, "Toren", 9, 9) })

	c.mark(t, "Goblin 1", playv1.CoverDegree_COVER_DEGREE_HALF)
	w.run(t, "the master's move of a combatant with a cover mark", nil, func() {
		c.mustMove(t, c.master, "Goblin 1", 18, 6)
		if got := c.who(t, c.master, "Goblin 1").GetCoverMark(); got != playv1.CoverDegree_COVER_DEGREE_NONE {
			t.Fatalf("the cover mark after the move = %v, want it cleared for the undo to have something to put back", got)
		}
	})

	w.run(t, "a long jump with a running start", nil, func() { c.mustMove(t, c.caio, "Toren", 9, 9, jumpTo(playv1.JumpKind_JUMP_KIND_LONG)) })
	w.run(t, "a high jump", nil, func() { c.mustMove(t, c.caio, "Toren", 9, 7, highJump(30)) })
}

// TestUndoKindCombatantMovedStandsUp: standing up from Prone takes the condition off
// and spends half the speed.
func TestUndoKindCombatantMovedStandsUp(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	c := a.arena(t, arenaPlan{})
	a.setConditions(t, c.refresh(t), "Toren", condProne)
	w := newUndoWatch(t, c.armed)

	w.run(t, "standing up", nil, func() {
		if _, err := a.standUp(t, a.caio, c.e, "Toren"); err != nil {
			t.Fatalf("StandUp() error = %v", err)
		}
		if c.hasCondition(t, "Toren", condProne) {
			t.Fatal("Toren is still prone after standing up")
		}
	})
}

// TestUndoKindCombatantMovedDrags: a move that drags a grappled creature along.
func TestUndoKindCombatantMovedDrags(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	c := dragFight(t, a, arenaPlan{})
	w := newUndoWatch(t, c.armed)

	w.run(t, "a drag", nil, func() {
		if _, err := c.moveTo(t, a.caio, "Toren", 3, 0); err != nil {
			t.Fatalf("a three-square drag: %v", err)
		}
		if got := c.square(t, c.hob); got != [2]int32{3, 1} {
			t.Fatalf("the Hobgoblin stands on %v after the drag, want (3,1), behind Toren", got)
		}
	})
}

// TestUndoKindCombatantMovedOffersOpportunities: a walk and a jump that leave an
// enemy's reach make offers, which the undo takes away with the move.
func TestUndoKindCombatantMovedOffersOpportunities(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.oppFight(t)
	w := newUndoWatch(t, c.armed)

	offered := func() {
		if len(c.offersOf(t, c.master)) != 1 {
			t.Fatalf("the master's offers = %v, want the move to have made one", c.offersOf(t, c.master))
		}
	}
	w.run(t, "a walk out of the reach", nil, func() { c.leaveGoblin(t); offered() })
	c.withRunningStart(t, "Toren") // set before the picture: the undo puts the run back
	w.run(t, "a jump out of the reach", nil, func() {
		if _, err := c.jumpMove(t, "Toren", 4, 6, jumpTo(playv1.JumpKind_JUMP_KIND_LONG)); err != nil {
			t.Fatalf("the jump error = %v", err)
		}
		offered()
	})
}

// TestUndoKindCombatantMovedPlaces: the master puts an unplaced combatant on the map,
// so the move comes from no square.
func TestUndoKindCombatantMovedPlaces(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.setupUnplaced(t)
	w := newUndoWatch(t, c.armed)

	w.run(t, "placing on the map", nil, func() {
		c.mustMove(t, c.master, "Goblin 1", 0, 0, func(r *playv1.MoveCombatantRequest) { r.Place = true })
	})
}

// TestUndoKindCombatantMovedThroughADoor: a move through a closed door opens it, and
// an undo of the move leaves it open (RN-26).
func TestUndoKindCombatantMovedThroughADoor(t *testing.T) {
	t.Parallel()
	f := newDoorCave(t, mapsv1.DoorState_DOOR_STATE_CLOSED)
	w := newUndoWatch(t, f.armed)

	w.run(t, "a move through a closed door", deltaDoorOpened, func() { f.mustMove(t, f.caio, "Toren", 10, 7) })
}

// TestUndoKindTrapTriggeredFromAMove: a player's move that runs into an armed pit. The
// move and the firing are two events, and one undo takes back the firing alone (the
// pending damage waits for the master, nothing landed), so the scenario takes the move
// back too, inside the action: what the picture compares is both.
func TestUndoKindTrapTriggeredFromAMove(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	r.trap(t, "Fosso Dourado", 9, 7, pit("1d6"))
	r.fight(t)
	w := newUndoWatch(t, r.armed)

	w.run(t, "a pit fired by a move", nil, func() {
		r.h.roller.queue(4)
		r.mustMove(t, r.caio, "Toren", 12, 7)
		if len(r.trapDamages(t)) != 1 {
			t.Fatalf("the trap damages = %v, want the pit to have fired", r.trapDamages(t))
		}
		r.undoLast(t) // the firing: the trap is armed again and the damage gone
	})
}

// TestUndoKindTrapTriggeredByHand: the master fires a statue on two creatures. The
// goblin fails its save, so its damage lands at once (the hit points, the defeat and the
// condition come back), and Toren, whose damage waits for the master, passes.
func TestUndoKindTrapTriggeredByHand(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	statue := r.trap(t, "Estátua de Fogo", 15, 12, manyTrapEffect())
	r.fight(t)
	w := newUndoWatch(t, r.armed)

	w.run(t, "the master fires a trap on two creatures", nil, func() {
		r.h.roller.queue(5, 3, 4, 20, 6, 6)
		if _, err := r.fireByHand(t, statue, r.id(t, "Goblin 1"), r.id(t, "Toren")); err != nil {
			t.Fatalf("FireTrap() error = %v", err)
		}
	})
}

// TestUndoKindTrapTriggeredInParts: a firing on ten creatures is written as several
// events, and one undo takes them all back (firingBefore).
func TestUndoKindTrapTriggeredInParts(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	const goblins = 10
	r.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: r.goblins.GetId(), Count: int32(goblins)}},
		npcRolls: []int{2},
		players:  map[string]int32{"Toren": 18, "Pensantus": 10, "Brisa": 1},
	})
	p := r.trap(t, "Estátua", 15, 12, manyTrapEffect())
	var ids []string
	for i := 1; i <= goblins; i++ {
		ids = append(ids, r.id(t, fmt.Sprintf("Goblin %d", i)))
	}
	w := newUndoWatch(t, r.armed)

	w.run(t, "a firing written in parts", nil, func() {
		if _, err := r.fireByHand(t, p, ids...); err != nil {
			t.Fatalf("FireTrap() error = %v", err)
		}
		if n := r.eventCount(t, eventTrapTriggered); n < 2 {
			t.Fatalf("the firing is %d event, want it in parts", n)
		}
	})
}

// TestUndoKindTrapTriggeredExtended: the master adds a creature to a firing that
// already happened. The extension never changed the trap, so the undo leaves it
// triggered and takes back only the creature it added.
func TestUndoKindTrapTriggeredExtended(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	net := r.trap(t, "Rede", 9, 7, func(s *mapsv1.TrapSpec) {
		s.Effect = &rulesv1.TrapEffect{
			Save: &rulesv1.TrapSaveEffect{
				Ability: rulesv1.Ability_ABILITY_DEXTERITY, Dc: 12, AppliesTo: rulesv1.TrapSaveApplies_TRAP_SAVE_APPLIES_CAUGHT,
				OnFail: &rulesv1.TrapOnFail{Damage: []*rulesv1.TrapDamage{{Dice: "1d6", DamageTypeKey: "damage-type:bludgeoning"}}},
				OnPass: rulesv1.TrapPassOutcome_TRAP_PASS_OUTCOME_NONE,
			},
			Targets: rulesv1.TrapTargets_TRAP_TARGETS_MANUAL,
		}
	})
	e := r.fight(t)
	r.h.roller.queue(5, 3)
	r.mustMove(t, r.caio, "Toren", 12, 7)
	var firing string
	for _, round := range r.log(t, r.master, e).GetRounds() {
		for _, en := range round.GetEntries() {
			if en.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_TRAP_TRIGGERED {
				firing = en.GetId()
			}
		}
	}
	w := newUndoWatch(t, r.armed)

	w.run(t, "adding a creature to a firing", nil, func() {
		r.h.roller.queue(5, 4)
		if _, err := r.master.play.FireTrap(t.Context(), connect.NewRequest(&playv1.FireTrapRequest{
			CampaignId: r.campaignID, MapId: r.mapID, PointId: net.GetId(), TargetIds: []string{r.id(t, "Pensantus")}, IdempotencyKey: newKey(), ExtendFiringId: firing,
		})); err != nil {
			t.Fatalf("FireTrap(extend) error = %v", err)
		}
	})
}

// TestUndoKindHideResolved: the master's reveal of a hider puts the hiding back.
func TestUndoKindHideResolved(t *testing.T) {
	t.Parallel()
	a := newSneaks(t)
	c := a.arena(t, arenaPlan{})
	tried := c.mustHide(t, a.caio, "Toren", cunningHide, 19)
	c.mustResolveHide(t, tried.GetId(), nil)
	w := newUndoWatch(t, c.armed)

	w.run(t, "the master reveals a hider", nil, func() {
		if _, err := c.revealHider(t, a.master, "Toren"); err != nil {
			t.Fatalf("RevealHider() error = %v", err)
		}
	})
}

// TestUndoKindDamageRolledOfAnAreaSpell: one roll settles the damage of every target
// of the cast (Settled): a creature that takes it at once and a character whose damage
// waits for the master.
func TestUndoKindDamageRolledOfAnAreaSpell(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFight(t, 1)
	a.stand(t, map[string][2]int32{"Goblin": {12, 9}, "Capitão Goblin": {7, 5}}) // the cone east of Pensantus holds the Capitão and Toren only
	cast := a.mustCast(t, a.ana, e, "Pensantus", burningHands, slotOfLevel(1), a.at(t, "Capitão Goblin", "Toren"), noCastRoll)
	w := newUndoWatch(t, a)

	w.run(t, "the damage roll of an area spell on a creature and a character", nil, func() {
		a.h.roller.queue(3, 3, 3)
		a.mustDamage(t, a.ana, e, cast.GetCast().GetPendingDamages()[0].GetId(), inAppDamage)
	})
}

// TestUndoKindDamageRolledAsksAConcentrationSave: an area spell's damage roll lands at
// once on a creature that concentrates, so it opens the concentration save of that
// creature, which the undo closes first (the top of takeBack).
func TestUndoKindDamageRolledAsksAConcentrationSave(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFight(t, 1)
	a.stand(t, map[string][2]int32{"Goblin": {12, 9}, "Capitão Goblin": {7, 5}})
	a.concentrate(t, "Capitão Goblin", webSpell)
	cast := a.mustCast(t, a.ana, e, "Pensantus", burningHands, slotOfLevel(1), a.at(t, "Capitão Goblin", "Toren"), noCastRoll)
	w := newUndoWatch(t, a)
	win := closedWindowOf("concentration_save", "actor=#", "key=spell:burning-hands", "pending=#", "spell=spell:web", "taken=9", "target=#")

	w.run(t, "a damage roll that opens a concentration save", win.delta, func() {
		a.h.roller.queue(3, 3, 3)
		a.mustDamage(t, a.ana, e, cast.GetCast().GetPendingDamages()[0].GetId(), inAppDamage)
		win.learn(t, a)
	})
}

// TestUndoKindDamageRolledSendsTheMoverBack: an opportunity attack whose damage drops
// its mover to 0 hit points puts it back on the square where it left the reach
// (ReturnedFrom, the 0 hit points rule).
func TestUndoKindDamageRolledSendsTheMoverBack(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.oppFight(t)
	c.passTo(t, c.get(t, c.master), "Goblin 1")
	c.moveOffering(t, "Goblin 1", 5, 9) // along a line that passes through Toren's reach
	offers := c.offersOf(t, c.caio)
	if len(offers) != 1 {
		t.Fatalf("Toren's player's offers = %v, want one", offers)
	}
	hit, err := c.offerAttack(t, c.caio, "Toren", offers[0].GetAttacks()[0].GetKey(), "Goblin 1", offers[0].GetId(), d20(15))
	if err != nil || hit.GetPendingDamage() == nil {
		t.Fatalf("Toren's opportunity attack = (%v, %v), want a hit", hit, err)
	}
	w := newUndoWatch(t, c.armed)

	w.run(t, "a damage that drops the mover to 0", nil, func() {
		if _, err := c.damage(t, c.caio, c.get(t, c.master), hit.GetPendingDamage().GetId(), typedDamage(8)); err != nil {
			t.Fatalf("RollDamage() error = %v", err)
		}
	})
}

// TestUndoKindDamageRolledSneakAttack: the roll that adds Sneak Attack marks the turn
// on the rogue (OnceBefore), and the undo gives the use back.
func TestUndoKindDamageRolledSneakAttack(t *testing.T) {
	t.Parallel()
	a, e := rollsTable(t)
	e = a.passTo(t, e, "Toren")
	a.setConditions(t, e, "Goblin", "condition:restrained")
	res := a.mustAttack(t, a.caio, a.get(t, a.caio), "Toren", rapier, "Goblin", func(r *playv1.RollAttackRequest) { r.D20Faces = []int32{5, 18} })
	w := newUndoWatch(t, a)

	w.run(t, "a damage roll with Sneak Attack", nil, func() {
		a.mustDamage(t, a.caio, a.get(t, a.caio), res.GetPendingDamage().GetId(), func(r *playv1.RollDamageRequest) {
			r.ExtrasChosen = true
			r.SelectedExtras = []*playv1.SelectedExtra{{Key: "sneak-attack"}}
			r.TypedParts = []*playv1.TypedPart{{PartKey: "weapon", Sum: 1}, {PartKey: "sneak-attack", Sum: 2}}
		})
		a.wantMark(t, "Toren", "sneak_attack_turn")
	})
}

// TestUndoKindDamageRolledColossusSlayer: the same for the Hunter's Colossus Slayer,
// which needs a target below its maximum hit points.
func TestUndoKindDamageRolledColossusSlayer(t *testing.T) {
	t.Parallel()
	a := newArmedWith(t, func(a *armed) {
		a.toren = a.caio.heroWith(t, a.campaignID, "Toren", classLevel("class:ranger", 3, "subclass:hunter"), abilities(16, 14, 14, 10, 10), []string{rapier}, nil,
			[]string{"feature:hunters-prey-colossus-slayer"})
		a.pens = a.ana.hero(t, a.campaignID, "Pensantus", "class:wizard", "race:gnome", 1,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 8}, nil, []string{fireBolt})
		a.bri = a.bia.hero(t, a.campaignID, "Brisa", "class:fighter", "race:human", 2,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 16, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 8}, []string{rapier}, nil)
	})
	e := a.threeAndAGoblin(t)
	if _, err := a.adjustHP(t, e, "Goblin", damageHP(1)); err != nil {
		t.Fatalf("AdjustCombatantHitPoints() error = %v", err)
	}
	res := a.mustAttack(t, a.caio, e, "Toren", rapier, "Goblin", d20(15))
	w := newUndoWatch(t, a)

	w.run(t, "a damage roll with Colossus Slayer", nil, func() {
		a.mustDamage(t, a.caio, e, res.GetPendingDamage().GetId(), func(r *playv1.RollDamageRequest) {
			r.ExtrasChosen = true
			r.SelectedExtras = []*playv1.SelectedExtra{{Key: "colossus-slayer"}}
			r.TypedParts = []*playv1.TypedPart{{PartKey: "weapon", Sum: 1}, {PartKey: "colossus-slayer", Sum: 2}}
		})
		a.wantMark(t, "Toren", "colossus_slayer_turn")
	})
}

// TestUndoKindDamageRolledDivineSmite: the roll that adds a Divine Smite spends the
// paladin's slot (Slot), and the undo gives it back.
func TestUndoKindDamageRolledDivineSmite(t *testing.T) {
	t.Parallel()
	a, _ := smiteTable(t)
	res := a.mustAttack(t, a.caio, a.get(t, a.caio), "Toren", battleaxe, "Esqueleto", d20(15))
	w := newUndoWatch(t, a)

	w.run(t, "a damage roll with Divine Smite", deltaPartsSelected, func() {
		a.mustDamage(t, a.caio, a.get(t, a.caio), res.GetPendingDamage().GetId(), func(r *playv1.RollDamageRequest) {
			r.ExtrasChosen = true
			r.SelectedExtras = []*playv1.SelectedExtra{{Key: "divine-smite", SlotLevel: 1}}
			r.TypedParts = []*playv1.TypedPart{{PartKey: "weapon", Sum: 1}, {PartKey: "divine-smite", Sum: 2}, {PartKey: "divine-smite-extra", Sum: 1}}
		})
	})
}

// TestUndoKindDamageRolledManeuver: a table maneuver that adds its die to the damage
// spends a use of the superiority dice (ManeuverSpent).
func TestUndoKindDamageRolledManeuver(t *testing.T) {
	t.Parallel()
	m := newManeuverTable(t, "4")
	e := m.passTo(t, m.castersFight(t, 1), "Pensantus")
	pending := m.hit(t, e)
	w := newUndoWatch(t, m.armed)

	w.run(t, "a damage roll with a maneuver", deltaManeuverParts(m.bull), func() {
		m.h.roller.queue(5, 4)
		m.mustDamage(t, m.ana, e, pending.GetId(), marking(m.heron))
	})
}

// wantMark requires the once-per-turn column of a combatant to be set: the roll the
// scenario is about must have made the mark, or its undo has nothing to put back.
func (a *armed) wantMark(t *testing.T, label, column string) {
	t.Helper()
	var set bool
	if err := a.h.pool.QueryRow(t.Context(), `SELECT `+column+` IS NOT NULL FROM combatants WHERE id = $1`, a.id(t, label)).Scan(&set); err != nil || !set {
		t.Fatalf("%s of %s is set = %v (error %v), want the roll to have marked the turn", column, label, set, err)
	}
}

// deltaPartsSelected: the roll rewrites the parts the damage offered (SetPendingDamageDetail
// in storePartsRoll, combat_parts.go) with the choice that was made, and the undo
// (ClearPendingDamageRoll, queries.sql, from takeBack's eventDamageRolled case) puts back
// the rolled columns and an empty list the roll left alone, but not a list the roll
// rewrote: the list the hit offered is kept nowhere (an event holds no text, and the
// offered parts carry labels and riders), so the damage that waits to be rolled again is
// offered with Divine Smite already chosen, at the slot level of the undone roll.
var deltaPartsSelected = undoDelta{
	"+ db/pending_damages[#].parts[1].selected=true", "+ db/pending_damages[#].parts[1].slot_level=1",
	"+ db/pending_damages[#].parts[2].selected=true", "+ db/pending_damages[#].parts[2].slot_level=1",
}

// deltaManeuverParts: the same for a table maneuver. The rewritten parts list holds only
// the golpe that was chosen, so after the undo the other golpe the character could use
// (bull, its key) is no longer offered on the damage and the chosen one is pre-selected.
func deltaManeuverParts(bull string) undoDelta {
	out := undoDelta{"+ db/pending_damages[#].parts[1].selected=true"}
	for _, f := range []string{
		"available=true", "choosable=true", "count=1", "damage_type=damage-type:slashing", "key=" + bull, "kind=extra", "label=Golpe do Touro",
		"resource=dados_de_golpe", "resource_left=4", "rider=O alvo faz um teste de Sabedoria.", "sides=8", "source=" + bull,
	} {
		out = append(out, "- db/pending_damages[#].parts[2]."+f)
	}
	return out
}

// nextTurnOf ends the turn that is on (dropping any damage left) and passes on to the
// next turn of the combatant with the label.
func (a *armed) nextTurnOf(t *testing.T, e *playv1.Encounter, label string) *playv1.Encounter {
	t.Helper()
	if _, err := a.endTurn(t, a.master, e, true); err != nil {
		t.Fatalf("EndTurn() error = %v", err)
	}
	return a.passTo(t, e, label)
}

// closedWindow is the row of a reaction window that an undo closed instead of deleting
// (CloseOpenReactionWindowsOfPending, the top of takeBack): before the action there was
// no such row, and after its undo there is a closed one that nobody reads (a closed
// window is listed nowhere). Its seq is a clock reading that differs on every run, so
// the delta learns it once the window exists, and the trigger lines are the window's.
type closedWindow struct {
	delta undoDelta
	seq   int
	kind  string
}

func closedWindowOf(kind string, trigger ...string) *closedWindow {
	w := &closedWindow{kind: kind}
	for _, l := range []string{
		"closed_reason=trigger_gone", "encounter_id=#", "group_id=#", "hold_id=<nil>", "kind=" + kind, "outcome=<nil>", "pending_damage_id=<nil>", "reactor_id=#",
		"seq=", "status=closed", "step=1",
	} {
		w.delta = append(w.delta, "+ db/reaction_windows[#]."+l)
	}
	w.seq = 8
	for _, l := range trigger {
		w.delta = append(w.delta, "+ db/reaction_windows[#].trigger."+l)
	}
	return w
}

// learn reads the window's seq, as the picture prints it (a JSON number read as float64).
func (w *closedWindow) learn(t *testing.T, a *armed) {
	t.Helper()
	var seq int64
	if err := a.h.pool.QueryRow(t.Context(), `SELECT seq FROM reaction_windows WHERE kind = $1`, w.kind).Scan(&seq); err != nil {
		t.Fatalf("read the seq of the %s window: %v", w.kind, err)
	}
	w.delta[w.seq] = fmt.Sprintf("+ db/reaction_windows[#].seq=%v", float64(seq))
}

// TestUndoKindDamageAppliedRelentless: Relentless Endurance keeps the half-orc on its
// feet with 1 hit point and spends its use, which the undo gives back (Relentless).
func TestUndoKindDamageAppliedRelentless(t *testing.T) {
	t.Parallel()
	a, e := raceFight(t, "race:half-orc", "class:fighter", 1)
	a.hurt(t, a.toren, 5)
	e = a.passTo(t, e, "Goblin")
	id := goblinHitsRagna(t, a, e, 3) // 3 + 2 = 5: exactly to 0
	w := newUndoWatch(t, a)

	w.run(t, "a damage that Relentless Endurance survives", nil, func() {
		if _, err := a.settle(t, a.master, e, id, true); err != nil {
			t.Fatalf("ApplyPendingDamage() error = %v", err)
		}
		if got := usedOf(a.vitals(t, a.toren), relentless); got != 1 {
			t.Fatalf("Relentless Endurance used = %d, want the damage to have spent it", got)
		}
	})
}

// TestUndoKindDamageAppliedSendsTheMoverBack: an opportunity attack of an enemy on a
// player's character, applied by the master, drops it to 0 and takes it back to where
// it left the reach (ReturnedFrom); the undo of the application puts it where it went.
func TestUndoKindDamageAppliedSendsTheMoverBack(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.oppFight(t)
	c.hurt(t, c.toren, 3)
	c.leaveGoblin(t)
	offer := c.offersOf(t, c.master)[0]
	hit, err := c.offerAttack(t, c.master, "Goblin 1", sword, "Toren", offer.GetId(), d20(15))
	if err != nil || hit.GetPendingDamage() == nil {
		t.Fatalf("the opportunity attack = (%v, %v), want a hit", hit, err)
	}
	e := c.get(t, c.master)
	c.mustDamage(t, c.master, e, hit.GetPendingDamage().GetId(), typedDamage(4))
	w := newUndoWatch(t, c.armed)

	w.run(t, "a damage that drops the mover to 0", nil, func() {
		if _, err := c.settle(t, c.master, e, hit.GetPendingDamage().GetId(), true); err != nil {
			t.Fatalf("ApplyPendingDamage() error = %v", err)
		}
	})
}

// TestUndoKindDamageAppliedOnACharacterAtZero: a damage on a character who is down is
// a failed death save (DeathBefore): the undo takes the failure back.
func TestUndoKindDamageAppliedOnACharacterAtZero(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.threeAndAGoblin(t)
	a.correct(t, a.toren, hpIs(0))
	e = a.passTo(t, e, "Goblin")
	hit := a.mustAttack(t, a.master, e, "Goblin", sword, "Toren", d20(15))
	a.mustDamage(t, a.master, e, hit.GetPendingDamage().GetId(), typedDamage(3))
	w := newUndoWatch(t, a)

	w.run(t, "a damage on a character at 0 hit points", nil, func() {
		if _, err := a.settle(t, a.master, e, hit.GetPendingDamage().GetId(), true); err != nil {
			t.Fatalf("ApplyPendingDamage() error = %v", err)
		}
	})
}

// TestUndoKindDamageAppliedOnTemporaryHitPoints: the damage is taken from the
// temporary hit points first, and the undo puts both numbers back.
func TestUndoKindDamageAppliedOnTemporaryHitPoints(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.threeAndAGoblin(t)
	a.correct(t, a.toren, func(r *playv1.AdjustCharacterVitalsRequest) { r.HitPointsTemporary = proto.Int32(5) })
	e = a.passTo(t, e, "Goblin")
	hit := a.mustAttack(t, a.master, e, "Goblin", sword, "Toren", d20(15))
	a.mustDamage(t, a.master, e, hit.GetPendingDamage().GetId(), typedDamage(6))
	w := newUndoWatch(t, a)

	w.run(t, "a damage that eats the temporary hit points", nil, func() {
		if _, err := a.settle(t, a.master, e, hit.GetPendingDamage().GetId(), true); err != nil {
			t.Fatalf("ApplyPendingDamage() error = %v", err)
		}
	})
}

// TestUndoKindDamageAppliedAsksAConcentrationSave: the damage the master applies on a
// character who concentrates opens the owner's Constitution save, which the undo closes
// (the top of takeBack).
func TestUndoKindDamageAppliedAsksAConcentrationSave(t *testing.T) {
	t.Parallel()
	a, e, id := concentratingDamage(t, 6)
	w := newUndoWatch(t, a)
	win := closedWindowOf("concentration_save", "actor=#", "key=basic:0", "pending=#", "spell=spell:web", "taken=8", "target=#")

	w.run(t, "a damage that asks for a concentration save", win.delta, func() {
		if _, err := a.settle(t, a.master, e, id, true); err != nil {
			t.Fatalf("ApplyPendingDamage() error = %v", err)
		}
		if a.windowOf(t, a.ana, playv1.ReactionKind_REACTION_KIND_CONCENTRATION_SAVE) == nil {
			t.Fatal("the damage opened no concentration save")
		}
		win.learn(t, a)
	})
}

// TestUndoKindDamageAppliedEndsTheConcentration: a damage to 0 hit points ends the
// concentration with no save, in an event written before the damage's, and the undo of
// the damage takes that one back with it.
func TestUndoKindDamageAppliedEndsTheConcentration(t *testing.T) {
	t.Parallel()
	a, e, id := concentratingDamage(t, 30)
	w := newUndoWatch(t, a)

	w.run(t, "a damage that ends the concentration at 0", nil, func() {
		if _, err := a.settle(t, a.master, e, id, true); err != nil {
			t.Fatalf("ApplyPendingDamage() error = %v", err)
		}
		if got := byLabel(t, a.get(t, a.master), "Pensantus").GetConcentrationSpell(); got != "" {
			t.Fatalf("Pensantus concentrates on %q at 0 hit points", got)
		}
	})
}

// concentratingDamage has the Capitão hit Pensantus, who concentrates on Teia, and rolls
// the damage, which waits for the master to apply it.
func concentratingDamage(t *testing.T, damage int) (*armed, *playv1.Encounter, string) {
	t.Helper()
	a := newCasters(t)
	e := a.castersFightNPCFirst(t)
	a.concentrate(t, "Pensantus", webSpell)
	hit := a.mustAttack(t, a.master, e, "Capitão Goblin", sword, "Pensantus", d20(9))
	if _, err := a.declineReaction(t, a.ana, e, hit.GetPendingDamage().GetId()); err != nil {
		t.Fatalf("DeclineReaction() error = %v", err)
	}
	a.h.roller.queue(damage)
	a.mustDamage(t, a.master, e, hit.GetPendingDamage().GetId(), inAppDamage)
	return a, e, hit.GetPendingDamage().GetId()
}

// TestUndoKindDamageAppliedAsksHellishRebuke: the damage opens the tiefling's Hellish
// Rebuke window, which the undo closes.
func TestUndoKindDamageAppliedAsksHellishRebuke(t *testing.T) {
	t.Parallel()
	a, e := reactorFight(t, tieflingFighter)
	hit := a.mustAttack(t, a.master, e, "Goblin", sword, "Reator", d20(15))
	a.mustDamage(t, a.master, e, hit.GetPendingDamage().GetId(), typedDamage(3))
	w := newUndoWatch(t, a)
	win := closedWindowOf("hellish_rebuke", "actor=#", "damage=5", "distance=5", "key=basic:0", "pending=#", "target=#")

	w.run(t, "a damage that opens Hellish Rebuke", win.delta, func() {
		if _, err := a.settle(t, a.master, e, hit.GetPendingDamage().GetId(), true); err != nil {
			t.Fatalf("ApplyPendingDamage() error = %v", err)
		}
		if a.windowOf(t, a.ana, playv1.ReactionKind_REACTION_KIND_HELLISH_REBUKE) == nil {
			t.Fatal("the damage opened no Hellish Rebuke window")
		}
		win.learn(t, a)
	})
}

// deltaShieldLost: the master's discard of a hit that waits for the target's Escudo closes
// the Shield window (settleReactions finds its trigger gone, triggerGone in
// combat_reaction_window.go:296, the pending being discarded), and the undo sets the
// pending damage back to awaiting_reaction without reopening the window (the
// eventDamageDiscarded case of takeBack, combat_undo.go:605, has no reopenWindowOf as the
// reaction cases have). With no open window the next settleReactions releases the hit:
// it ends up awaiting_roll, and the reactor's prompt, the table's "Esperando a reação" and
// the master's window are gone. Escudo can no longer be cast on this hit.
func deltaShieldLost() undoDelta {
	out := undoDelta{
		"+ db/pending_damages[#].status=awaiting_roll", "- db/pending_damages[#].status=awaiting_reaction",
		"+ db/reaction_windows[#].closed_reason=trigger_gone", "+ db/reaction_windows[#].outcome={}", "+ db/reaction_windows[#].status=closed",
		"- db/reaction_windows[#].closed_reason=<nil>", "- db/reaction_windows[#].outcome=<nil>", "- db/reaction_windows[#].status=open",
	}
	prompt := []string{
		"attackNamePt=Cimitarra", "attackerLabel=Capitão Goblin", "pendingDamageId=#", "slots[0].free=4", "slots[0].level=1", "slots[1].free=2", "slots[1].level=2",
		"spellKey=spell:shield", "spellNamePt=Escudo Arcano", "targetId=#",
	}
	window := []string{
		"forYou=true", "groupId=#", "id=#", "kind=REACTION_KIND_SHIELD", "reactorId=#", "reactorIsPlayer=true", "reactorLabel=Pensantus",
		"shield.attackNamePt=Cimitarra", "shield.attackerLabel=Capitão Goblin", "shield.pendingDamageId=#", "shield.slots[0].free=4", "shield.slots[0].level=1",
		"shield.slots[1].free=2", "shield.slots[1].level=2", "shield.spellNamePt=Escudo Arcano", "status=REACTION_WINDOW_STATUS_OPEN",
	}
	for _, p := range prompt {
		out = append(out, "- ana/combat.reactionPrompts[0]."+p, "- master/combat.reactionPrompts[0]."+p)
	}
	out = append(out, "- master/combat.reactionPrompts[0].attackerId=#")
	for _, p := range window {
		out = append(out, "- ana/combat.reactionWindows[0]."+p, "- master/combat.reactionWindows[0]."+p)
	}
	for _, p := range []string{
		"answerNow=true", "trigger.actionKey=basic:0", "trigger.actionNamePt=Cimitarra", "trigger.actorId=#", "trigger.actorLabel=Capitão Goblin",
		"trigger.armorClassWithShield=17", "trigger.attackTotal=13", "trigger.distanceFt=5", "trigger.targetArmorClass=12", "trigger.targetId=#", "trigger.targetLabel=Pensantus",
	} {
		out = append(out, "- master/combat.reactionWindows[0]."+p)
	}
	out = append(out, "- master/combat.reactionWait.titlePt=Esperando a reação de Pensantus")
	for _, who := range []string{"bia", "caio"} {
		out = append(out, "- "+who+"/combat.reactionWait.detailPt=O turno continua quando responder.", "- "+who+"/combat.reactionWait.titlePt=Esperando a reação de Pensantus")
	}
	return out
}

// TestUndoKindDamageDiscarded: the master discards a damage in each status it can be
// in: waiting for the target's reaction, waiting for the roll and rolled.
func TestUndoKindDamageDiscarded(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFightNPCFirst(t)
	w := newUndoWatch(t, a)
	discard := func(id string) func() {
		return func() {
			if _, err := a.settle(t, a.master, e, id, false); err != nil {
				t.Fatalf("DiscardPendingDamage() error = %v", err)
			}
		}
	}

	// Pensantus can cast Escudo: the hit waits for her reaction.
	waiting := a.mustAttack(t, a.master, e, "Capitão Goblin", sword, "Pensantus", d20(9)).GetPendingDamage()
	w.run(t, "discarding a damage that waits for a reaction", deltaShieldLost(), discard(waiting.GetId()))
	e = a.nextTurnOf(t, e, "Capitão Goblin")

	// Toren has no reaction: the hit waits for the roll.
	unrolled := a.mustAttack(t, a.master, e, "Capitão Goblin", sword, "Toren", d20(19)).GetPendingDamage()
	w.run(t, "discarding a damage that waits for the roll", nil, discard(unrolled.GetId()))
	e = a.nextTurnOf(t, e, "Capitão Goblin")

	rolled := a.mustAttack(t, a.master, e, "Capitão Goblin", sword, "Toren", d20(19)).GetPendingDamage()
	a.h.roller.queue(3)
	a.mustDamage(t, a.master, e, rolled.GetId(), inAppDamage)
	w.run(t, "discarding a rolled damage", nil, discard(rolled.GetId()))
}

// TestUndoKindHitPointsAdjustedOnACreature: the master's damage, heal and temporary hit
// points on a player's creature (the wolf), whose hit points the combat keeps itself.
// A player's character is refused by AdjustCombatantHitPoints (its hit points are in
// its vitals, which have their own correction), so that is the only player-side
// combatant this event reaches.
func TestUndoKindHitPointsAdjustedOnACreature(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	e := c.wolfFight(t)
	w := newUndoWatch(t, c.armed)

	w.run(t, "damage on a player's creature", nil, func() {
		if _, err := c.adjustHP(t, e, "Presa", damageHP(3)); err != nil {
			t.Fatalf("AdjustCombatantHitPoints() error = %v", err)
		}
	})
	if _, err := c.adjustHP(t, e, "Presa", damageHP(3)); err != nil {
		t.Fatalf("AdjustCombatantHitPoints() error = %v", err)
	}
	w.run(t, "a heal on a player's creature", nil, func() {
		if _, err := c.adjustHP(t, e, "Presa", healHP(2)); err != nil {
			t.Fatalf("AdjustCombatantHitPoints() error = %v", err)
		}
	})
	w.run(t, "temporary hit points on a player's creature", nil, func() {
		if _, err := c.adjustHP(t, e, "Presa", func(r *playv1.AdjustCombatantHitPointsRequest) { r.HitPointsTemporary = proto.Int32(4) }); err != nil {
			t.Fatalf("AdjustCombatantHitPoints() error = %v", err)
		}
	})
}
