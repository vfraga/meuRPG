package play

import (
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// The sunlight of a combat (RN-20, RN-29, #704): the master's switch, and what it changes for a creature with Sunlight
// Sensitivity (the Drow). These tests need the database (MEURPG_TEST_DATABASE_URL).

const (
	drowKey = "monster:drow"
	drow    = "Drow"
	// sunSentence is what the master reads of the sunlight behind a roll.
	sunSentence = "Luz do sol (Sensibilidade à Luz do Sol)"
)

func sunSq(col, row int32) *playv1.SunlightSquare { return &playv1.SunlightSquare{Col: col, Row: row} }

// onSun is the request that turns the sunlight on, whole map.
func onSun(r *playv1.SetSunlightRequest) {
	r.On, r.Scope = true, playv1.SunlightScope_SUNLIGHT_SCOPE_MAP
}

// setSunlight calls SetSunlight as u.
func (a *armed) setSunlight(t *testing.T, u *user, e *playv1.Encounter, edit func(*playv1.SetSunlightRequest)) (*playv1.Encounter, error) {
	t.Helper()
	return a.setSunlightKey(t, u, e, newKey(), edit)
}

func (a *armed) setSunlightKey(t *testing.T, u *user, e *playv1.Encounter, key string, edit func(*playv1.SetSunlightRequest)) (*playv1.Encounter, error) {
	t.Helper()
	req := &playv1.SetSunlightRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), IdempotencyKey: key}
	edit(req)
	res, err := u.combat.SetSunlight(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetEncounter(), nil
}

func (a *armed) mustSunlight(t *testing.T, e *playv1.Encounter, edit func(*playv1.SetSunlightRequest)) *playv1.Encounter {
	t.Helper()
	out, err := a.setSunlight(t, a.master, e, edit)
	if err != nil {
		t.Fatalf("SetSunlight() error = %v", err)
	}
	return out
}

// drowFight is the Drow beside Toren (one square away) with the Drow on turn.
func drowFight(t *testing.T) (*armed, *playv1.Encounter) {
	t.Helper()
	a, e := monsterFightOn(t, newCastersGear(t, 3, "", []string{battleaxe}), drowKey, drow)
	e = a.passTo(t, e, drow)
	return a, e
}

// drowReads is the Drow's melee attack on Toren as the master reads it in the turn options: the mode and whether a
// source is the sunlight, with the sentence.
func (a *armed) drowReads(t *testing.T, e *playv1.Encounter) (playv1.RollMode, string, string) {
	t.Helper()
	o := a.mustOptions(t, a.master, e, drow)
	if len(o.GetAttackTargets()) == 0 {
		t.Fatalf("the Drow has no attack to read: %v", o)
	}
	key := o.GetAttackTargets()[0].GetAttackKey()
	tg := targetOf(o, key, "Toren")
	if tg == nil {
		t.Fatalf("Toren is not a target of the Drow's %s: %v", key, o.GetAttackTargets())
	}
	var sun string
	for _, s := range tg.GetSources() {
		if s.GetKind() == playv1.AdvantageSourceKind_ADVANTAGE_SOURCE_KIND_SUNLIGHT_ATTACK {
			sun = s.GetTextPt()
		}
	}
	return tg.GetRollMode(), sun, key
}

func (a *armed) wantDrowRolls(t *testing.T, e *playv1.Encounter, what string, want playv1.RollMode) {
	t.Helper()
	mode, sun, _ := a.drowReads(t, e)
	if mode != want {
		t.Errorf("%s: the Drow's attack mode = %v, want %v", what, mode, want)
	}
	if (want == playv1.RollMode_ROLL_MODE_DISADVANTAGE) != (sun == sunSentence) || (want != playv1.RollMode_ROLL_MODE_DISADVANTAGE && sun != "") {
		t.Errorf("%s: the sunlight source = %q, want %q only under disadvantage", what, sun, sunSentence)
	}
}

// The Drow's attack is at a disadvantage while the switch is on and the Drow stands in sunlight, and the master reads
// why; with the switch off, or when the Drow stands in a square the sun does not reach, it rolls as usual.
func TestSunlight_ADrowAttacksWithDisadvantageInTheSun(t *testing.T) {
	t.Parallel()
	a, e := drowFight(t)
	a.wantDrowRolls(t, e, "switch off", playv1.RollMode_ROLL_MODE_NORMAL)

	a.mustSunlight(t, e, onSun)
	a.wantDrowRolls(t, e, "the whole map in the sun", playv1.RollMode_ROLL_MODE_DISADVANTAGE)

	// The attack itself is rolled with two d20 and keeps the lower, and its sources carry the sunlight (the master's).
	_, _, key := a.drowReads(t, e)
	a.h.roller.queue(18, 2)
	res := a.mustAttack(t, a.master, e, drow, key, "Toren", inAppRoll)
	if roll := res.GetRoll(); roll.GetMode() != playv1.RollMode_ROLL_MODE_DISADVANTAGE || len(roll.GetD20().GetFaces()) != 2 {
		t.Errorf("the Drow's attack in the sun = %v, want disadvantage over two d20", roll)
	}

	a.mustSunlight(t, e, func(r *playv1.SetSunlightRequest) { r.On = false })
	a.wantDrowRolls(t, e, "switched off again", playv1.RollMode_ROLL_MODE_NORMAL)
}

// With the scope on squares only the listed squares are sunlit: the Drow's square (7, 5) counts when listed and stops
// counting when removed; the sun that is off keeps its squares so the master can switch it back.
func TestSunlight_SquaresScopeLightsOnlyTheListedSquares(t *testing.T) {
	t.Parallel()
	a, e := drowFight(t)
	a.mustSunlight(t, e, func(r *playv1.SetSunlightRequest) {
		r.On, r.Scope, r.AddSquares = true, playv1.SunlightScope_SUNLIGHT_SCOPE_SQUARES, []*playv1.SunlightSquare{sunSq(6, 5), sunSq(1, 1)}
	})
	a.wantDrowRolls(t, e, "the Drow's square is not listed", playv1.RollMode_ROLL_MODE_NORMAL)

	a.mustSunlight(t, e, func(r *playv1.SetSunlightRequest) {
		r.On, r.AddSquares = true, []*playv1.SunlightSquare{sunSq(7, 5), sunSq(7, 5)}
	})
	a.wantDrowRolls(t, e, "the Drow's square is listed", playv1.RollMode_ROLL_MODE_DISADVANTAGE)
	got := a.get(t, a.master)
	if got.GetSunlightScope() != playv1.SunlightScope_SUNLIGHT_SCOPE_SQUARES || len(got.GetSunlightSquares()) != 3 {
		t.Fatalf("the master reads scope %v and %d squares, want SQUARES and 3 (a square listed twice is kept once)", got.GetSunlightScope(), len(got.GetSunlightSquares()))
	}
	for i, want := range [][2]int32{{1, 1}, {6, 5}, {7, 5}} { // by row, then column
		if s := got.GetSunlightSquares()[i]; s.GetCol() != want[0] || s.GetRow() != want[1] {
			t.Errorf("square %d = (%d, %d), want (%d, %d)", i, s.GetCol(), s.GetRow(), want[0], want[1])
		}
	}

	a.mustSunlight(t, e, func(r *playv1.SetSunlightRequest) {
		r.On, r.RemoveSquares = true, []*playv1.SunlightSquare{sunSq(7, 5)}
	})
	a.wantDrowRolls(t, e, "the Drow's square was removed", playv1.RollMode_ROLL_MODE_NORMAL)

	a.mustSunlight(t, e, func(r *playv1.SetSunlightRequest) { r.On = false })
	got = a.get(t, a.master)
	if got.GetSunlightOn() || got.GetSunlightScope() != playv1.SunlightScope_SUNLIGHT_SCOPE_SQUARES || len(got.GetSunlightSquares()) != 2 {
		t.Errorf("after switching off the master reads on %v, scope %v, %d squares, want off, SQUARES and 2 kept", got.GetSunlightOn(), got.GetSunlightScope(), len(got.GetSunlightSquares()))
	}
}

// Darkness beats the sun: a Drow standing in a magical Darkness rolls as usual even with the whole map in sunlight,
// and the sun reaches it again when the Darkness ends.
func TestSunlight_DarknessBeatsTheSun(t *testing.T) {
	t.Parallel()
	c := obscureCave(t)
	e := c.start(t, plan{
		npcs: []*playv1.Participant{{CharacterId: c.ogre.GetId()}}, npcRolls: []int{2},
		players: map[string]int32{"Toren": 18, "Pensantus": 10, "Brisa": 5}, reveal: []string{"Ogro"},
		at:    map[string][2]int32{"Toren": {8, 7}, "Pensantus": {12, 7}, "Brisa": {3, 7}, "Ogro": {20, 9}},
		setup: true,
	})
	c.h.roller.queue(1)
	c.mustAddMonsters(t, e, func(r *playv1.AddMonstersRequest) { r.CreatureKey, r.Count = drowKey, 1 })
	for label, at := range map[string][2]int32{drow: {7, 7}} {
		if _, err := c.master.combat.MoveCombatant(t.Context(), connect.NewRequest(&playv1.MoveCombatantRequest{
			CampaignId: c.campaignID, EncounterId: e.GetId(), CombatantId: c.id(t, label), IdempotencyKey: newKey(), Col: at[0], Row: at[1],
		})); err != nil {
			t.Fatalf("MoveCombatant(%s) error = %v", label, err)
		}
	}
	if _, err := c.master.combat.SetCombatantHidden(t.Context(), connect.NewRequest(&playv1.SetCombatantHiddenRequest{
		CampaignId: c.campaignID, EncounterId: e.GetId(), CombatantId: c.id(t, drow), IdempotencyKey: newKey(),
	})); err != nil {
		t.Fatalf("SetCombatantHidden(Drow) error = %v", err)
	}
	e = c.begin(t, e)
	c.mustSunlight(t, e, onSun)
	c.passTo(t, e, drow)
	c.wantDrowRolls(t, e, "in the sun, before the Darkness", playv1.RollMode_ROLL_MODE_DISADVANTAGE)

	c.passTo(t, e, "Pensantus")
	c.mustCastArea(t, c.ana, "Pensantus", darknessKey, slotOfLevel(2), at(7, 7), nil)
	c.passTo(t, e, drow)
	c.wantDrowRolls(t, e, "inside the Darkness", playv1.RollMode_ROLL_MODE_NORMAL)

	c.passTo(t, e, "Pensantus")
	c.nextTurnOf(t, "Pensantus")
	c.mustCastArea(t, c.ana, "Pensantus", fogCloudKey, slotOfLevel(1), at(7, 7), nil) // another concentration spell: the Darkness ends
	c.passTo(t, e, drow)
	c.wantDrowRolls(t, e, "the Darkness ended", playv1.RollMode_ROLL_MODE_DISADVANTAGE)
}

// A player may not switch the sunlight, a square off the grid is refused, so are squares in a combat with no map, a
// scope of squares there, and any change after the combat ended.
func TestSunlight_Refusals(t *testing.T) {
	t.Parallel()
	a, e := drowFight(t)
	wantCode := func(what string, u *user, e *playv1.Encounter, code connect.Code, edit func(*playv1.SetSunlightRequest)) {
		t.Helper()
		_, err := a.setSunlight(t, u, e, edit)
		if connect.CodeOf(err) != code {
			t.Errorf("%s: error = %v, want %v", what, err, code)
		}
	}
	wantCode("a player", a.caio, e, connect.CodePermissionDenied, onSun)
	for what, s := range map[string]*playv1.SunlightSquare{"column too high": sunSq(gridColumns, 0), "negative row": sunSq(0, -1), "row too high": sunSq(0, 1000)} {
		wantCode("add, "+what, a.master, e, connect.CodeInvalidArgument, func(r *playv1.SetSunlightRequest) { r.On, r.AddSquares = true, []*playv1.SunlightSquare{s} })
	}
	wantCode("remove off the grid", a.master, e, connect.CodeInvalidArgument, func(r *playv1.SetSunlightRequest) { r.RemoveSquares = []*playv1.SunlightSquare{sunSq(-1, 0)} })
	if got := a.get(t, a.master); got.GetSunlightOn() || len(got.GetSunlightSquares()) != 0 {
		t.Errorf("a refused call changed the sunlight: on %v, squares %v", got.GetSunlightOn(), got.GetSunlightSquares())
	}

	// No map: a theatre combat has no squares.
	b := newArmed(t)
	te := b.start(t, plan{
		npcs: []*playv1.Participant{{CharacterId: b.goblin.GetId()}}, npcRolls: []int{3},
		players: map[string]int32{"Toren": 18, "Pensantus": 10, "Brisa": 1}, reveal: []string{"Goblin"}, theatre: true,
	})
	if _, err := b.setSunlight(t, b.master, te, func(r *playv1.SetSunlightRequest) { r.AddSquares = []*playv1.SunlightSquare{sunSq(1, 1)} }); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("squares in a theatre combat: error = %v, want invalid_argument", err)
	}
	if _, err := b.setSunlight(t, b.master, te, func(r *playv1.SetSunlightRequest) { r.On, r.Scope = true, playv1.SunlightScope_SUNLIGHT_SCOPE_SQUARES }); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("scope squares in a theatre combat: error = %v, want invalid_argument", err)
	}
	if got, err := b.setSunlight(t, b.master, te, onSun); err != nil || !got.GetSunlightOn() || got.GetSunlightScope() != playv1.SunlightScope_SUNLIGHT_SCOPE_MAP {
		t.Errorf("the whole map in a theatre combat: %v, %v, want on with scope MAP", got, err)
	}

	if _, err := a.master.combat.EndEncounter(t.Context(), connect.NewRequest(&playv1.EndEncounterRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), IdempotencyKey: newKey()})); err != nil {
		t.Fatalf("EndEncounter() error = %v", err)
	}
	_, err := a.setSunlight(t, a.master, e, onSun)
	if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Errorf("an ended combat: error = %v, want failed_precondition", err)
	}
}

// A retry of the same call writes once: the combat's revision moves one step and one event is stored.
func TestSunlight_ARetryWritesOnce(t *testing.T) {
	t.Parallel()
	a, e := drowFight(t)
	count := `SELECT count(*) FROM session_events WHERE kind = 'sunlight_set'`
	key := newKey()
	first, err := a.setSunlightKey(t, a.master, e, key, onSun)
	if err != nil {
		t.Fatalf("SetSunlight() error = %v", err)
	}
	again, err := a.setSunlightKey(t, a.master, e, key, onSun)
	if err != nil {
		t.Fatalf("SetSunlight(retry) error = %v", err)
	}
	if first.GetRevision() != again.GetRevision() || !again.GetSunlightOn() {
		t.Errorf("revision %d then %d, on %v: want the same revision and on", first.GetRevision(), again.GetRevision(), again.GetSunlightOn())
	}
	var n int
	if err := a.h.pool.QueryRow(t.Context(), count).Scan(&n); err != nil || n != 1 {
		t.Errorf("sunlight_set events = %d (%v), want 1", n, err)
	}
	if _, err := a.setSunlightKey(t, a.master, e, key, func(r *playv1.SetSunlightRequest) { r.On = false }); err == nil {
		t.Error("the same key for another request was accepted")
	}
}

// A player reads none of the sunlight: not the switch, the scope or the squares, on the combat the call answers or on
// the one he reads.
func TestSunlight_APlayerReadsNoneOfIt(t *testing.T) {
	t.Parallel()
	a, e := drowFight(t)
	a.mustSunlight(t, e, func(r *playv1.SetSunlightRequest) {
		r.On, r.Scope, r.AddSquares = true, playv1.SunlightScope_SUNLIGHT_SCOPE_SQUARES, []*playv1.SunlightSquare{sunSq(7, 5)}
	})
	if got := a.get(t, a.master); !got.GetSunlightOn() || len(got.GetSunlightSquares()) != 1 {
		t.Fatalf("the master reads on %v and %v, want on and one square", got.GetSunlightOn(), got.GetSunlightSquares())
	}
	for who, u := range map[string]*user{"Toren's player": a.caio, "Pensantus's player": a.ana, "Brisa's player": a.bia} {
		got := a.get(t, u)
		if got.GetSunlightOn() || got.GetSunlightScope() != playv1.SunlightScope_SUNLIGHT_SCOPE_UNSPECIFIED || len(got.GetSunlightSquares()) != 0 {
			t.Errorf("%s reads on %v, scope %v, squares %v, want none of them", who, got.GetSunlightOn(), got.GetSunlightScope(), got.GetSunlightSquares())
		}
	}
}

// The sunlight's event is the master's alone: no line in the log of anyone, and the undo of the action before it goes on
// past it.
func TestSunlight_NoLogLineAndTheUndoGoesPastIt(t *testing.T) {
	t.Parallel()
	a, e := monsterFightOn(t, newCastersGear(t, 3, "", []string{battleaxe}), knightKey, knight)
	hit := a.mustAttack(t, a.caio, e, "Toren", battleaxe, knight, d20(2)) // a miss: the attack is the last thing done
	if hit.GetRoll() == nil {
		t.Fatal("Toren's attack has no roll")
	}
	before := a.log(t, a.master, e)
	attack := before.GetUndoableEventId()
	if attack == "" {
		t.Fatal("the attack cannot be undone")
	}
	a.mustSunlight(t, e, onSun)
	after := a.log(t, a.master, e)
	if got := after.GetUndoableEventId(); got != attack {
		t.Errorf("the undoable event after the sunlight = %q, want the attack %q: the switch closed the undo", got, attack)
	}
	count := func(l *playv1.ListCombatLogResponse) (n int) {
		for _, r := range l.GetRounds() {
			n += len(r.GetEntries())
		}
		return n
	}
	if count(before) != count(after) {
		t.Errorf("the master's log has %d lines, was %d: the sunlight has no line", count(after), count(before))
	}
	for who, u := range map[string]*user{"Toren's player": a.caio, "Pensantus's player": a.ana} {
		if n := count(a.log(t, u, e)); n != count(a.log(t, a.master, e)) && n > count(before) {
			t.Errorf("%s reads %d lines, more than before the sunlight", who, n)
		}
	}
	if err := a.undo(t, a.master, e, attack); err != nil {
		t.Errorf("UndoLastAction(the attack) error = %v", err)
	}
}

// sunlight.lit, by the combatant's place and what the sunlight covers.
func TestSunlightLit(t *testing.T) {
	t.Parallel()
	at := func(c, r int32) playdb.Combatant { return playdb.Combatant{GridCol: &c, GridRow: &r} }
	squares := squareSet([]grid.Square{{Col: 2, Row: 3}, {Col: 5, Row: 5}})
	dark := map[grid.Square]bool{{Col: 5, Row: 5}: true, {Col: 9, Row: 9}: true}
	for _, tc := range []struct {
		name string
		sun  sunlight
		c    playdb.Combatant
		want bool
	}{
		{"off lights nobody", sunlight{squares: squares}, at(2, 3), false},
		{"the zero value lights nobody", sunlight{}, at(0, 0), false},
		{"map: any square", sunlight{on: true}, at(40, 40), true},
		{"map: unplaced is lit", sunlight{on: true}, playdb.Combatant{}, true},
		{"map: Darkness beats the sun", sunlight{on: true, dark: dark}, at(9, 9), false},
		{"map: a square beside the Darkness is lit", sunlight{on: true, dark: dark}, at(9, 8), true},
		{"squares: a listed square", sunlight{on: true, squaresOnly: true, squares: squares}, at(2, 3), true},
		{"squares: an unlisted square", sunlight{on: true, squaresOnly: true, squares: squares}, at(3, 2), false},
		{"squares: unplaced is not lit", sunlight{on: true, squaresOnly: true, squares: squares}, playdb.Combatant{}, false},
		{"squares: Darkness beats a listed square", sunlight{on: true, squaresOnly: true, squares: squares, dark: dark}, at(5, 5), false},
		{"squares: none listed", sunlight{on: true, squaresOnly: true}, at(2, 3), false},
	} {
		if got := tc.sun.lit(tc.c); got != tc.want {
			t.Errorf("%s: lit = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// The notes of a check a creature in the sun rolls in a contest, a held check or a passive Perception are the same pure
// rules as the attack's: the Drow (Sensitivity) is spoiled on Perception alone, and the note a player could read
// says "Outra fonte" and nothing of the sun.
func TestSunlight_AChecksNoteNamesNoSun(t *testing.T) {
	t.Parallel()
	a, e := drowFight(t)
	ctx := t.Context()
	notes := func(skill string) []rollNote {
		t.Helper()
		enc, err := a.h.svc.queries.GetEncounterByID(ctx, e.GetId())
		if err != nil {
			t.Fatalf("GetEncounterByID() error = %v", err)
		}
		cs, err := a.h.svc.queries.ListCombatants(ctx, e.GetId())
		if err != nil {
			t.Fatalf("ListCombatants() error = %v", err)
		}
		who := a.id(t, drow)
		for _, c := range cs {
			if c.ID == who {
				out, err := a.h.svc.checkSourcesRead(ctx, a.campaignID, enc, c, skill, cs)
				if err != nil {
					t.Fatalf("checkSourcesRead(%s) error = %v", skill, err)
				}
				return out
			}
		}
		t.Fatal("the Drow is not a combatant")
		return nil
	}
	if got := notes(skillPercept); len(got) != 0 {
		t.Fatalf("Perception with the sun off: notes %v, want none", got)
	}
	a.mustSunlight(t, e, onSun)
	if got := notes(skillPercept); len(got) != 1 || got[0].Kind != "other" || got[0].Label != "Outra fonte" || got[0].Adv {
		t.Errorf("Perception in the sun: notes %v, want one disadvantage note, \"Outra fonte\"", got)
	}
	if got := notes(skillAthletics); len(got) != 0 {
		t.Errorf("Athletics in the sun for a Sensitivity creature: notes %v, want none", got)
	}
}

// A combat whose map was deleted keeps its grid (migration 00043 unsets the map and the fight goes on): the master can
// still paint and erase squares, and the scope stays.
func TestSunlight_AGridWhoseMapWasDeletedStillTakesSquares(t *testing.T) {
	t.Parallel()
	a, e := drowFight(t)
	a.mustSunlight(t, e, func(r *playv1.SetSunlightRequest) {
		r.On, r.Scope, r.AddSquares = true, playv1.SunlightScope_SUNLIGHT_SCOPE_SQUARES, []*playv1.SunlightSquare{sunSq(1, 1)}
	})
	if _, err := a.h.pool.Exec(t.Context(), `UPDATE encounters SET map_id = NULL WHERE id = $1`, e.GetId()); err != nil {
		t.Fatalf("unset the map: %v", err)
	}
	got := a.mustSunlight(t, e, func(r *playv1.SetSunlightRequest) {
		r.AddSquares, r.RemoveSquares = []*playv1.SunlightSquare{sunSq(2, 2)}, []*playv1.SunlightSquare{sunSq(1, 1)}
	})
	if got.GetSunlightScope() != playv1.SunlightScope_SUNLIGHT_SCOPE_SQUARES || len(got.GetSunlightSquares()) != 1 || got.GetSunlightSquares()[0].GetCol() != 2 || got.GetSunlightSquares()[0].GetRow() != 2 {
		t.Errorf("after the map was deleted: scope %v squares %v, want squares with (2,2) alone", got.GetSunlightScope(), got.GetSunlightSquares())
	}
}

// The prompts and the trap saves that write a save's sources (the concentration and the lasting-effect prompts, a trap's
// save and attack, the trap search) read the sun as itself to the master and as "Outra fonte" to a player.
func TestSunlight_ThePromptsNameTheSunToTheMasterAlone(t *testing.T) {
	t.Parallel()
	names := func(k string) string { return k }
	sources := []combat.Source{{Kind: combat.SourceSunlightSave, Effect: combat.ModeDisadvantage, Condition: combat.TraitSunlightWeakness}}
	const sun = "Luz do sol (Fraqueza à Luz do Sol)"
	for what, shown := range map[string]func(master bool) []*playv1.AdvantageSource{
		"shownSources": func(m bool) []*playv1.AdvantageSource { return shownSources(sources, names, m) },
		"shownRecs":    func(m bool) []*playv1.AdvantageSource { return shownRecs(sourceRecs(sources), names, m) },
	} {
		if got := shown(true); len(got) != 1 || got[0].GetTextPt() != sun {
			t.Errorf("%s for the master = %v, want %q", what, got, sun)
		}
		if got := shown(false); len(got) != 1 || got[0].GetTextPt() != otherSourceTx+": desvantagem" || got[0].GetKind() == playv1.AdvantageSourceKind_ADVANTAGE_SOURCE_KIND_SUNLIGHT_SAVE {
			t.Errorf("%s for a player = %v, want %q with no sun kind", what, got, otherSourceTx+": desvantagem")
		}
	}
}
