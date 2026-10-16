package play

import (
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The characterization of MoveCombatant beyond the plain cave (see
// combat_move_frozen_test.go): doors, the state of the combat, the request.

// doorMove is one move on the door cave: Toren on turn at (6, 7) with the door
// of the case at (8, 7).
type doorMove struct {
	name    string
	door    mapsv1.DoorState
	setup   func(t *testing.T, c *cave)
	as      string // "master" or Toren's player
	edit    []func(*playv1.MoveCombatantRequest)
	col     int32
	refused *refusal
	want    moveWant
	// opened is the door_opened payload the move wrote, "" when it opened nothing.
	opened string
}

func TestMoveCombatantFrozen_Doors(t *testing.T) {
	t.Parallel()
	closed, locked, barred := mapsv1.DoorState_DOOR_STATE_CLOSED, mapsv1.DoorState_DOOR_STATE_LOCKED, mapsv1.DoorState_DOOR_STATE_BARRED
	nextToTheDoor := standAt(map[string][2]int32{"Toren": {7, 7}})
	doorLockedRefusal := ptr(blockedBy("DOOR_LOCKED", "the door is locked"))
	for _, tc := range []doorMove{
		{
			name: "a player through a closed door", door: closed, col: 10,
			want: moveWant{col: 10, row: 7, usedDft: 200, lastDft: 200, event: `{"col": 10, "combatant_id": "<Toren>", "cost_dft": 200, "cost_ft": 20, "distance_dft": 200, "distance_ft": 20, "from": {"col": 6, "cover": "none", "placed": true, "row": 7}, "on_turn": true, "round": 1, "row": 7, "spent_dft": 200}`}, opened: `{"col": 8, "combatant_id": "<Toren>", "from": {"col": 6, "cover": "none", "placed": true, "row": 7}, "round": 1, "row": 7}`,
		},
		{
			name: "a player stopped by a locked door", door: locked, col: 10,
			want: moveWant{col: 7, row: 7, usedDft: 50, lastDft: 50, locked: true, event: `{"col": 7, "combatant_id": "<Toren>", "cost_dft": 50, "cost_ft": 5, "distance_dft": 50, "distance_ft": 5, "from": {"col": 6, "cover": "none", "placed": true, "row": 7}, "locked_door": true, "on_turn": true, "round": 1, "row": 7, "spent_dft": 50}`},
		},
		{name: "a player in front of a locked door", door: locked, setup: nextToTheDoor, col: 10, refused: doorLockedRefusal},
		{name: "a player at a barred door", door: barred, col: 10, refused: ptr(blockedBy("MOVE_BLOCKED", "a wall, a column or a squeeze is in the way"))},
		{
			name: "the master through a closed door", door: closed, as: "master", col: 10,
			want: moveWant{col: 10, row: 7, usedDft: 0, lastDft: 0, event: `{"col": 10, "combatant_id": "<Toren>", "distance_dft": 200, "distance_ft": 20, "from": {"col": 6, "cover": "none", "placed": true, "row": 7}, "on_turn": true, "round": 1, "row": 7}`}, opened: `{"col": 8, "combatant_id": "<Toren>", "from": {"col": 6, "cover": "none", "placed": true, "row": 7}, "round": 1, "row": 7}`,
		},
		{
			name: "the master stopped by a locked door", door: locked, as: "master", col: 10,
			want: moveWant{col: 7, row: 7, usedDft: 0, lastDft: 0, stopped: true, locked: true, event: `{"col": 7, "combatant_id": "<Toren>", "distance_dft": 50, "distance_ft": 5, "from": {"col": 6, "cover": "none", "placed": true, "row": 7}, "locked_door": true, "on_turn": true, "round": 1, "row": 7, "stopped_early": true}`},
		},
		{name: "the master in front of a locked door", door: locked, setup: nextToTheDoor, as: "master", col: 10, refused: doorLockedRefusal},
		{
			name: "the master stopped by a barred door", door: barred, as: "master", col: 10,
			want: moveWant{col: 7, row: 7, usedDft: 0, lastDft: 0, stopped: true, event: `{"col": 7, "combatant_id": "<Toren>", "distance_dft": 50, "distance_ft": 5, "from": {"col": 6, "cover": "none", "placed": true, "row": 7}, "on_turn": true, "round": 1, "row": 7, "stopped_early": true}`},
		},
		{
			name: "the master, forced, ignores a locked door", door: locked, as: "master", col: 10, edit: []func(*playv1.MoveCombatantRequest){asForced},
			want: moveWant{col: 10, row: 7, usedDft: 0, lastDft: 0, event: `{"col": 10, "combatant_id": "<Toren>", "distance_dft": 200, "distance_ft": 20, "from": {"col": 6, "cover": "none", "placed": true, "row": 7}, "on_turn": true, "round": 1, "row": 7}`},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newDoorCave(t, tc.door)
			u := f.caio
			if tc.as == "master" {
				u = f.master
			}
			if tc.setup != nil {
				tc.setup(t, f.cave)
			}
			doorEvents := f.moveKindCount(t, "door_opened")
			f.frozenCheck(t, u, "Toren", tc.col, 7, tc.edit, tc.refused, tc.want)
			if got := f.moveKindCount(t, "door_opened") - doorEvents; (tc.opened == "") != (got == 0) || got > 1 {
				t.Fatalf("door_opened events written = %d, want one only when the door opened (%q)", got, tc.opened)
			}
			if tc.opened != "" {
				if got := f.lastMovePayload(t, "door_opened"); got != tc.opened {
					t.Errorf("the door_opened payload =\n  %s\nwant\n  %s", got, tc.opened)
				}
			}
		})
	}
}

// The state of the combat decides who may move at all.
func TestMoveCombatantFrozen_TheStateOfTheCombat(t *testing.T) {
	t.Parallel()
	t.Run("a combat that has not begun", func(t *testing.T) {
		t.Parallel()
		c := newCave(t)
		c.setupUnplaced(t)
		_, err := c.frozenMove(t, c.caio, "Toren", 7, 7)
		c.frozenWantRefusal(t, err, blockedBy("NOT_ACTIVE", "the combat is not running"))
		// The master sets the board up in this state: a move off turn is free.
		c.frozenCheck(t, c.master, "Toren", 7, 7, nil, nil, moveWant{col: 7, row: 7, usedDft: 0, lastDft: 0, event: `{"col": 7, "combatant_id": "<Toren>", "distance_dft": 50, "distance_ft": 5, "from": {"col": 6, "cover": "none", "placed": true, "row": 7}, "row": 7}`})
	})
	t.Run("a combat that has ended", func(t *testing.T) {
		t.Parallel()
		c := newCave(t)
		e := c.fight(t)
		if _, err := c.master.combat.EndEncounter(t.Context(), connect.NewRequest(&playv1.EndEncounterRequest{CampaignId: c.campaignID, EncounterId: e.GetId(), IdempotencyKey: newKey()})); err != nil {
			t.Fatalf("EndEncounter() error = %v", err)
		}
		for _, u := range []*user{c.caio, c.master} {
			_, err := c.frozenMove(t, u, "Toren", 7, 7)
			c.frozenWantRefusal(t, err, blockedBy("ENCOUNTER_ENDED", "the combat has ended"))
		}
	})
	t.Run("a combat without a map", func(t *testing.T) {
		t.Parallel()
		a := newArmed(t)
		e := a.theatreThree(t)
		for _, u := range []*user{a.caio, a.master} {
			_, err := u.combat.MoveCombatant(t.Context(), connect.NewRequest(&playv1.MoveCombatantRequest{
				CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, "Toren"), IdempotencyKey: newKey(), Col: 1, Row: 1,
			}))
			a.frozenWantRefusal(t, err, blockedBy("NEEDS_A_MAP", "this needs a map: a combat without one has no squares"))
		}
	})
	t.Run("a player at zero hit points", func(t *testing.T) {
		t.Parallel()
		c := newCave(t)
		c.fight(t)
		if _, err := c.master.play.AdjustCharacterVitals(t.Context(), connect.NewRequest(&playv1.AdjustCharacterVitalsRequest{
			CampaignId: c.campaignID, CharacterId: c.toren.GetId(), IdempotencyKey: newKey(), HitPointsCurrent: proto.Int32(0),
		})); err != nil {
			t.Fatalf("AdjustCharacterVitals() error = %v", err)
		}
		_, err := c.frozenMove(t, c.caio, "Toren", 7, 7)
		c.frozenWantRefusal(t, err, blockedBy("COMBATANT_DOWN", "the character is down"))
	})
	t.Run("an opportunity attack waits for its answer", func(t *testing.T) {
		t.Parallel()
		c := newCave(t)
		c.fight(t)
		standAt(map[string][2]int32{"Goblin 1": {7, 8}})(t, c)
		c.frozenCheck(t, c.caio, "Toren", 4, 6, nil, nil, moveWant{col: 4, row: 6, usedDft: 112, lastDft: 112, offered: true, event: `{"col": 4, "combatant_id": "<Toren>", "cost_dft": 112, "cost_ft": 11, "distance_dft": 112, "distance_ft": 11, "from": {"col": 6, "cover": "none", "placed": true, "row": 7}, "move_id": "<offers>", "on_turn": true, "round": 1, "row": 6, "spent_dft": 112}`})
		_, err := c.frozenMove(t, c.caio, "Toren", 3, 6)
		c.frozenWantRefusal(t, err, blockedBy("OPPORTUNITY_PENDING", "an opportunity attack waits for its answer"))
		// The master is never stopped by the wait.
		c.frozenCheck(t, c.master, "Toren", 3, 6, nil, nil, moveWant{col: 3, row: 6, usedDft: 112, lastDft: 112, event: `{"col": 3, "combatant_id": "<Toren>", "distance_dft": 50, "distance_ft": 5, "from": {"col": 4, "cover": "none", "last_dft": 112, "placed": true, "row": 6, "used_dft": 112}, "on_turn": true, "round": 1, "row": 6}`})
	})
}

// A call that is not well formed.
func TestMoveCombatantFrozen_TheRequest(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.fight(t)
	stranger := c.h.newUser("Zé")
	for _, tc := range []struct {
		name string
		as   *user
		edit func(*playv1.MoveCombatantRequest)
		want refusal
	}{
		{"a key that is not a UUID", c.caio, func(r *playv1.MoveCombatantRequest) { r.IdempotencyKey = "not-a-key" }, refuse(connect.CodeInvalidArgument, "idempotency_key must be a UUID")},
		{"an encounter that is not a UUID", c.caio, func(r *playv1.MoveCombatantRequest) { r.EncounterId = "x" }, refuse(connect.CodeNotFound, "encounter not found")},
		{"a combatant that is not a UUID", c.caio, func(r *playv1.MoveCombatantRequest) { r.CombatantId = "x" }, refuse(connect.CodeNotFound, "combatant not found")},
		{"a combatant of no combat", c.caio, func(r *playv1.MoveCombatantRequest) { r.CombatantId = newKey() }, refuse(connect.CodeNotFound, "combatant not found")},
		{"a stranger to the campaign", stranger, func(*playv1.MoveCombatantRequest) {}, refuse(connect.CodeNotFound, "campaign not found")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := c.frozenMove(t, tc.as, "Toren", 7, 7, tc.edit)
			c.frozenWantRefusal(t, err, tc.want)
		})
	}
}
