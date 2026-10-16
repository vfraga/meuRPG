package play

import (
	"fmt"
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// The characterization of MoveCombatant on the cave of combat_move_test.go (see
// combat_move_frozen_test.go): a player and the master move on a map with no fog.

// caveMove is one move on the cave. Toren is on turn at (6, 7) with 30 ft (300
// dft) of movement, and his jump limits are 16 ft with a running start and 8 ft
// standing (long), 6 ft and 3 ft (high). Brisa is at (4, 7), Pensantus at (5, 8)
// and Goblin 1 at (18, 5).
type caveMove struct {
	name  string
	setup func(t *testing.T, c *cave)
	as    string // "master", "ana", "bia"; Toren's player by default
	label string // Toren by default
	col   int32
	row   int32
	edit  []func(*playv1.MoveCombatantRequest)
	// refused is the whole refusal; when nil the move works and want is checked.
	refused *refusal
	want    moveWant
}

func (m caveMove) mover(c *cave) *user {
	switch m.as {
	case "master":
		return c.master
	case "ana":
		return c.ana
	case "bia":
		return c.bia
	}
	return c.caio
}

func asForced(r *playv1.MoveCombatantRequest) { r.Forced = true }
func asPlaced(r *playv1.MoveCombatantRequest) { r.Place = true }

// runCaveMoves runs each case on its own cave and checks that a refusal changes
// nothing and that a move leaves exactly what the case says.
func runCaveMoves(t *testing.T, cases []caveMove) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := newCave(t)
			c.fight(t)
			if tc.setup != nil {
				tc.setup(t, c)
			}
			label := tc.label
			if label == "" {
				label = "Toren"
			}
			c.frozenCheck(t, tc.mover(c), label, tc.col, tc.row, tc.edit, tc.refused, tc.want)
		})
	}
}

// standAt puts the combatants on squares as the master, forced (setup, not play).
func standAt(at map[string][2]int32) func(t *testing.T, c *cave) {
	return func(t *testing.T, c *cave) {
		t.Helper()
		for label, sq := range at {
			c.mustMove(t, c.master, label, sq[0], sq[1])
		}
	}
}

// then runs the setups in order.
func then(steps ...func(t *testing.T, c *cave)) func(t *testing.T, c *cave) {
	return func(t *testing.T, c *cave) {
		t.Helper()
		for _, s := range steps {
			s(t, c)
		}
	}
}

func hiding(label string) func(t *testing.T, c *cave) {
	return func(t *testing.T, c *cave) {
		t.Helper()
		c.hide(t, label)
	}
}

// walks is a move of Toren's player during the setup.
func walks(col, row int32) func(t *testing.T, c *cave) {
	return func(t *testing.T, c *cave) {
		t.Helper()
		c.mustMove(t, c.caio, "Toren", col, row)
	}
}

// unplaced takes a combatant off the map, as an NPC added to a combat is until
// the master places it.
func unplaced(label string) func(t *testing.T, c *cave) {
	return func(t *testing.T, c *cave) {
		t.Helper()
		c.execSQL(t, `UPDATE combatants SET grid_col = NULL, grid_row = NULL WHERE id = $1`, c.id(t, label))
	}
}

func conditioned(label string, keys ...string) func(t *testing.T, c *cave) {
	return func(t *testing.T, c *cave) {
		t.Helper()
		c.setConditions(t, c.get(t, c.master), label, keys...)
	}
}

// flying gives a combatant a fly speed, the better of its speeds.
func flying(label string, ft int) func(t *testing.T, c *cave) {
	return func(t *testing.T, c *cave) {
		t.Helper()
		c.execSQL(t, `UPDATE combatants SET speed_fly_ft = $2 WHERE id = $1`, c.id(t, label), ft)
	}
}

func disengaging(t *testing.T, c *cave) {
	t.Helper()
	if _, err := c.action(t, c.caio, c.get(t, c.master), "Toren", "standard:disengage"); err != nil {
		t.Fatalf("TakeAction(disengage) error = %v", err)
	}
}

func walledAt(sq ...[2]int32) func(t *testing.T, c *cave) {
	return func(t *testing.T, c *cave) {
		t.Helper()
		for _, s := range sq {
			c.terrain.addWalls(grid.Square{Col: int(s[0]), Row: int(s[1])})
		}
	}
}

func marked(label string, cover playv1.CoverDegree) func(t *testing.T, c *cave) {
	return func(t *testing.T, c *cave) {
		t.Helper()
		c.mark(t, label, cover)
	}
}

// Walking: the cost of the line, the terrain, the walls and the creatures in the way.
func TestMoveCombatantFrozen_OnFoot(t *testing.T) {
	t.Parallel()
	runCaveMoves(t, []caveMove{
		{name: "three squares", col: 9, row: 7, want: moveWant{col: 9, row: 7, usedDft: 150, lastDft: 150, event: `{"col": 9, "combatant_id": "<Toren>", "cost_dft": 150, "cost_ft": 15, "distance_dft": 150, "distance_ft": 15, "from": {"col": 6, "cover": "none", "placed": true, "row": 7}, "on_turn": true, "round": 1, "row": 7, "spent_dft": 150}`}},
		{name: "to the limit of the speed", col: 12, row: 7, want: moveWant{col: 12, row: 7, usedDft: 300, lastDft: 300, event: `{"col": 12, "combatant_id": "<Toren>", "cost_dft": 300, "cost_ft": 30, "distance_dft": 300, "distance_ft": 30, "from": {"col": 6, "cover": "none", "placed": true, "row": 7}, "on_turn": true, "round": 1, "row": 7, "spent_dft": 300}`}},
		{name: "a diagonal of three squares costs the exact line, rounded down", col: 9, row: 10, want: moveWant{col: 9, row: 10, usedDft: 212, lastDft: 212, event: `{"col": 9, "combatant_id": "<Toren>", "cost_dft": 212, "cost_ft": 21, "distance_dft": 212, "distance_ft": 21, "from": {"col": 6, "cover": "none", "placed": true, "row": 7}, "on_turn": true, "round": 1, "row": 10, "spent_dft": 212}`}},
		// Frozen as it is: a move to the square the mover stands on spends nothing and is not a
		// log line, but still writes a combatant_moved event (with no distance).
		{name: "to where it stands", col: 6, row: 7, want: moveWant{col: 6, row: 7, usedDft: 0, lastDft: 0, event: `{"col": 6, "combatant_id": "<Toren>", "from": {"col": 6, "cover": "none", "placed": true, "row": 7}, "on_turn": true, "round": 1, "row": 7}`}},
		{
			name: "through rubble", setup: standAt(map[string][2]int32{"Toren": {3, 9}}), col: 6, row: 9,
			want: moveWant{col: 6, row: 9, usedDft: 250, lastDft: 150, event: `{"col": 6, "combatant_id": "<Toren>", "cost_dft": 250, "cost_ft": 25, "distance_dft": 150, "distance_ft": 15, "from": {"col": 3, "cover": "none", "placed": true, "row": 9}, "on_turn": true, "round": 1, "row": 9, "spent_dft": 250}`},
		},
		{
			name: "ending in rubble", setup: standAt(map[string][2]int32{"Toren": {3, 9}}), col: 5, row: 9,
			want: moveWant{col: 5, row: 9, usedDft: 200, lastDft: 100, event: `{"col": 5, "combatant_id": "<Toren>", "cost_dft": 200, "cost_ft": 20, "distance_dft": 100, "distance_ft": 10, "from": {"col": 3, "cover": "none", "placed": true, "row": 9}, "on_turn": true, "round": 1, "row": 9, "spent_dft": 200}`},
		},
		{name: "one square too far", col: 13, row: 7, refused: ptr(blockedBy("TOO_FAR", "that is beyond your movement", `"missingFt":5`, `"missingDft":50`))},
		{name: "into a wall", col: 6, row: 5, refused: ptr(blockedBy("MOVE_BLOCKED", "a wall, a column or a squeeze is in the way"))},
		{name: "onto a creature", col: 5, row: 8, refused: ptr(blockedBy("SQUARE_OCCUPIED", "another combatant is on that square"))},
		{
			name: "through an enemy", setup: standAt(map[string][2]int32{"Goblin 1": {8, 7}}), col: 10, row: 7,
			refused: ptr(blockedBy("ENEMY_IN_THE_WAY", "an enemy is in the way")),
		},
		{name: "off the grid to the east", col: 24, row: 7, refused: ptr(refuse(connect.CodeInvalidArgument, "col and row must be a square of the grid"))},
		{name: "off the grid to the south", col: 6, row: 16, refused: ptr(refuse(connect.CodeInvalidArgument, "col and row must be a square of the grid"))},
		{name: "off the grid to the west", col: -1, row: 7, refused: ptr(refuse(connect.CodeInvalidArgument, "col and row must be a square of the grid"))},
	})
}

// A creature the player does not see cuts the walk and the jump short.
func TestMoveCombatantFrozen_AnUnseenCreatureInTheWay(t *testing.T) {
	t.Parallel()
	hiddenAt := func(col, row int32) func(t *testing.T, c *cave) {
		return then(standAt(map[string][2]int32{"Goblin 1": {col, row}}), hiding("Goblin 1"))
	}
	runCaveMoves(t, []caveMove{
		{
			name: "on the line", setup: hiddenAt(8, 7), col: 10, row: 7,
			want: moveWant{col: 7, row: 7, usedDft: 50, lastDft: 50, stopped: true, event: `{"col": 7, "combatant_id": "<Toren>", "cost_dft": 50, "cost_ft": 5, "distance_dft": 50, "distance_ft": 5, "from": {"col": 6, "cover": "none", "placed": true, "row": 7}, "on_turn": true, "round": 1, "row": 7, "spent_dft": 50, "stopped_early": true}`},
		},
		{
			name: "on the destination", setup: hiddenAt(8, 7), col: 8, row: 7,
			want: moveWant{col: 7, row: 7, usedDft: 50, lastDft: 50, stopped: true, event: `{"col": 7, "combatant_id": "<Toren>", "cost_dft": 50, "cost_ft": 5, "distance_dft": 50, "distance_ft": 5, "from": {"col": 6, "cover": "none", "placed": true, "row": 7}, "on_turn": true, "round": 1, "row": 7, "spent_dft": 50, "stopped_early": true}`},
		},
		{
			// Frozen as it is: the jump that is cut short at the landing spends nothing and keeps
			// the running start, but is still written as a long jump of distance 0.
			name: "holding the landing of a long jump", setup: hiddenAt(7, 7), col: 7, row: 7, edit: []func(*playv1.MoveCombatantRequest){jumpTo(playv1.JumpKind_JUMP_KIND_LONG)},
			want: moveWant{col: 6, row: 7, usedDft: 0, lastDft: 0, stopped: true, event: `{"col": 6, "combatant_id": "<Toren>", "from": {"col": 6, "cover": "none", "placed": true, "row": 7}, "jump": "long", "on_turn": true, "round": 1, "row": 7, "stopped_early": true}`},
		},
	})
}

// Jumps: the limits, the running start, the walls and the cost.
func TestMoveCombatantFrozen_Jumps(t *testing.T) {
	t.Parallel()
	long, high := jumpTo(playv1.JumpKind_JUMP_KIND_LONG), highJump
	onRow9 := standAt(map[string][2]int32{"Toren": {3, 9}})
	runUp := then(standAt(map[string][2]int32{"Toren": {1, 9}}), walks(3, 9))
	tooFar := func(missingDft, limitDft int, running bool) *refusal {
		fields := []string{fmt.Sprintf(`"missingFt":%d`, (missingDft+9)/10), fmt.Sprintf(`"missingDft":%d`, missingDft), fmt.Sprintf(`"jumpLimitDft":%d`, limitDft)}
		if running {
			fields = append(fields, `"jumpRunningStart":true`)
		}
		return ptr(blockedBy("TOO_FAR", "that is beyond your movement", fields...))
	}
	runCaveMoves(t, []caveMove{
		{name: "a long jump standing, too far", setup: onRow9, col: 5, row: 9, edit: []func(*playv1.MoveCombatantRequest){long}, refused: tooFar(20, 80, false)},
		{
			name: "a long jump with a running start into rubble", setup: runUp, col: 5, row: 9, edit: []func(*playv1.MoveCombatantRequest){long},
			want: moveWant{col: 5, row: 9, usedDft: 200, lastDft: 0, event: `{"col": 5, "combatant_id": "<Toren>", "cost_dft": 100, "cost_ft": 10, "distance_dft": 100, "distance_ft": 10, "from": {"col": 3, "cover": "none", "last_dft": 100, "placed": true, "row": 9, "used_dft": 100}, "jump": "long", "jump_running": true, "landing_difficult": true, "on_turn": true, "round": 1, "row": 9}`},
		},
		{
			name: "a long jump standing, one square", setup: onRow9, col: 2, row: 9, edit: []func(*playv1.MoveCombatantRequest){long},
			want: moveWant{col: 2, row: 9, usedDft: 50, lastDft: 0, event: `{"col": 2, "combatant_id": "<Toren>", "cost_dft": 50, "cost_ft": 5, "distance_dft": 50, "distance_ft": 5, "from": {"col": 3, "cover": "none", "placed": true, "row": 9}, "jump": "long", "on_turn": true, "round": 1, "row": 9}`},
		},
		{name: "a long jump to the same square", col: 6, row: 7, edit: []func(*playv1.MoveCombatantRequest){long}, refused: ptr(refuse(connect.CodeInvalidArgument, "a jump needs another square"))},
		{
			name: "a long jump over a wall", setup: then(standAt(map[string][2]int32{"Toren": {9, 7}}), walks(11, 7), walledAt([2]int32{12, 7})), col: 13, row: 7,
			edit: []func(*playv1.MoveCombatantRequest){long}, refused: ptr(blockedBy("MOVE_BLOCKED", "a wall, a column or a squeeze is in the way")),
		},
		{
			name: "a long jump onto a creature", setup: then(standAt(map[string][2]int32{"Toren": {9, 7}, "Goblin 2": {13, 7}}), walks(11, 7)), col: 13, row: 7,
			edit: []func(*playv1.MoveCombatantRequest){long}, refused: ptr(blockedBy("SQUARE_OCCUPIED", "another combatant is on that square")),
		},
		{
			name: "a long jump the movement left does not pay", setup: walks(11, 7), col: 13, row: 7,
			edit: []func(*playv1.MoveCombatantRequest){long}, refused: ptr(blockedBy("TOO_FAR", "that is beyond your movement", `"missingFt":5`, `"missingDft":50`)),
		},
		{
			name: "a high jump standing", col: 6, row: 7, edit: []func(*playv1.MoveCombatantRequest){high(30)},
			want: moveWant{col: 6, row: 7, usedDft: 30, lastDft: 0, event: `{"col": 6, "combatant_id": "<Toren>", "cost_dft": 30, "cost_ft": 3, "from": {"col": 6, "cover": "none", "placed": true, "row": 7}, "height_dft": 30, "jump": "high", "on_turn": true, "round": 1, "row": 7}`},
		},
		{name: "a high jump standing, too high", col: 6, row: 7, edit: []func(*playv1.MoveCombatantRequest){high(31)}, refused: tooFar(1, 30, false)},
		{name: "a high jump with a running start, too high", setup: walks(8, 7), col: 8, row: 7, edit: []func(*playv1.MoveCombatantRequest){high(61)}, refused: tooFar(1, 60, true)},
		{
			name: "a high jump the movement left does not pay", setup: walks(11, 7), col: 11, row: 7, edit: []func(*playv1.MoveCombatantRequest){high(60)},
			refused: ptr(blockedBy("TOO_FAR", "that is beyond your movement", `"missingFt":1`, `"missingDft":10`)),
		},
		{name: "a high jump of no height", col: 6, row: 7, edit: []func(*playv1.MoveCombatantRequest){high(0)}, refused: ptr(refuse(connect.CodeInvalidArgument, "jump_height_dft must be 1 to 1000"))},
		{name: "a high jump of too much", col: 6, row: 7, edit: []func(*playv1.MoveCombatantRequest){high(1001)}, refused: ptr(refuse(connect.CodeInvalidArgument, "jump_height_dft must be 1 to 1000"))},
		{
			name: "a kind of jump nobody knows", col: 6, row: 7, edit: []func(*playv1.MoveCombatantRequest){jumpTo(99)},
			refused: ptr(refuse(connect.CodeInvalidArgument, "jump must be a kind of jump")),
		},
		{
			name: "a jump from off the map", setup: unplaced("Goblin 1"), as: "master", label: "Goblin 1", col: 17, row: 5, edit: []func(*playv1.MoveCombatantRequest){long},
			refused: ptr(blockedBy("NOT_PLACED", "a jump starts from a square of the map")),
		},
	})
}

// The master moves anyone: free, with no limit, and a forced move offers nothing.
func TestMoveCombatantFrozen_TheMaster(t *testing.T) {
	t.Parallel()
	long := jumpTo(playv1.JumpKind_JUMP_KIND_LONG)
	runCaveMoves(t, []caveMove{
		{
			name: "an NPC off its turn", as: "master", label: "Goblin 1", col: 17, row: 5,
			want: moveWant{col: 17, row: 5, usedDft: 0, lastDft: 0, event: `{"col": 17, "combatant_id": "<Goblin 1>", "distance_dft": 50, "distance_ft": 5, "from": {"col": 18, "cover": "none", "placed": true, "row": 5}, "round": 1, "row": 5}`},
		},
		{
			name: "a player on turn, forced, far away", as: "master", col: 14, row: 7, edit: []func(*playv1.MoveCombatantRequest){asForced},
			want: moveWant{col: 14, row: 7, usedDft: 0, lastDft: 0, event: `{"col": 14, "combatant_id": "<Toren>", "distance_dft": 400, "distance_ft": 40, "from": {"col": 6, "cover": "none", "placed": true, "row": 7}, "on_turn": true, "round": 1, "row": 7}`},
		},
		{name: "into a wall", as: "master", label: "Goblin 1", col: 0, row: 0, refused: ptr(blockedBy("WALL_ON_SQUARE", "that square is a wall"))},
		{name: "into a wall, forced", as: "master", label: "Goblin 1", col: 0, row: 0, edit: []func(*playv1.MoveCombatantRequest){asForced}, refused: ptr(blockedBy("WALL_ON_SQUARE", "that square is a wall"))},
		{
			name: "a long jump beyond the limits", as: "master", col: 16, row: 7, edit: []func(*playv1.MoveCombatantRequest){long},
			want: moveWant{col: 16, row: 7, usedDft: 0, lastDft: 0, event: `{"col": 16, "combatant_id": "<Toren>", "distance_dft": 500, "distance_ft": 50, "from": {"col": 6, "cover": "none", "placed": true, "row": 7}, "jump": "long", "on_turn": true, "round": 1, "row": 7}`},
		},
		{
			name: "a high jump beyond the limits", as: "master", col: 6, row: 7, edit: []func(*playv1.MoveCombatantRequest){highJump(100)},
			want: moveWant{col: 6, row: 7, usedDft: 0, lastDft: 0, event: `{"col": 6, "combatant_id": "<Toren>", "from": {"col": 6, "cover": "none", "placed": true, "row": 7}, "height_dft": 100, "jump": "high", "on_turn": true, "round": 1, "row": 7}`},
		},
		{
			name: "an NPC put on the map by a move", setup: unplaced("Goblin 1"), as: "master", label: "Goblin 1", col: 17, row: 5,
			want: moveWant{col: 17, row: 5, usedDft: 0, lastDft: 0, event: `{"col": 17, "combatant_id": "<Goblin 1>", "from": {"cover": "none"}, "round": 1, "row": 5}`},
		},
		{
			name: "a placement", setup: unplaced("Goblin 1"), as: "master", label: "Goblin 1", col: 0, row: 0, edit: []func(*playv1.MoveCombatantRequest){asPlaced},
			want: moveWant{col: 12, row: 8, usedDft: 0, lastDft: 0, event: `{"col": 12, "combatant_id": "<Goblin 1>", "from": {"cover": "none"}, "round": 1, "row": 8}`},
		},
		{name: "a placement of a combatant already placed", as: "master", label: "Goblin 1", edit: []func(*playv1.MoveCombatantRequest){asPlaced}, refused: ptr(refuse(connect.CodeInvalidArgument, "the combatant is already on the map"))},
		{name: "a placement that is a jump", as: "master", label: "Goblin 1", edit: []func(*playv1.MoveCombatantRequest){asPlaced, long}, refused: ptr(refuse(connect.CodeInvalidArgument, "a placement is not a jump"))},
		{
			name: "a placement with no free square", setup: then(unplaced("Goblin 1"), func(t *testing.T, c *cave) {
				t.Helper()
				for row := range 16 {
					for col := range 24 {
						c.terrain.addWalls(grid.Square{Col: col, Row: row})
					}
				}
			}), as: "master", label: "Goblin 1", edit: []func(*playv1.MoveCombatantRequest){asPlaced},
			refused: ptr(blockedBy("NO_FREE_SQUARE", "there is no free square on the map")),
		},
	})
}

// What a player may not ask, and who may move what.
func TestMoveCombatantFrozen_TheGates(t *testing.T) {
	t.Parallel()
	notYours := refuse(connect.CodePermissionDenied, "only the combatant's player or the master may do this")
	runCaveMoves(t, []caveMove{
		{name: "a forced move", col: 7, row: 7, edit: []func(*playv1.MoveCombatantRequest){asForced}, refused: ptr(refuse(connect.CodePermissionDenied, "only the master may force a move"))},
		{name: "a placement", col: 7, row: 7, edit: []func(*playv1.MoveCombatantRequest){asPlaced}, refused: ptr(refuse(connect.CodePermissionDenied, "only the master may place a combatant"))},
		{name: "somebody else's character", as: "ana", col: 7, row: 7, refused: &notYours},
		{name: "off the turn", as: "ana", label: "Pensantus", col: 5, row: 9, refused: ptr(blockedBy("NOT_YOUR_TURN", "it is not your turn"))},
		{name: "an NPC the master hid", setup: hiding("Goblin 1"), label: "Goblin 1", col: 17, row: 5, refused: ptr(refuse(connect.CodeNotFound, "combatant not found"))},
		{name: "an NPC of the master", label: "Goblin 1", col: 17, row: 5, refused: &notYours},
		{
			name: "off the map", setup: unplaced("Toren"), col: 7, row: 7,
			refused: ptr(blockedBy("NOT_PLACED", "you are not on the map yet; ask the master")),
		},
	})
}

// Opportunity attacks, the master's cover mark and the Disengage action.
func TestMoveCombatantFrozen_Provoking(t *testing.T) {
	t.Parallel()
	goblinNextToToren := standAt(map[string][2]int32{"Goblin 1": {7, 8}})
	runCaveMoves(t, []caveMove{
		{
			name: "a player leaving an enemy's reach", setup: goblinNextToToren, col: 4, row: 6,
			want: moveWant{col: 4, row: 6, usedDft: 112, lastDft: 112, offered: true, event: `{"col": 4, "combatant_id": "<Toren>", "cost_dft": 112, "cost_ft": 11, "distance_dft": 112, "distance_ft": 11, "from": {"col": 6, "cover": "none", "placed": true, "row": 7}, "move_id": "<offers>", "on_turn": true, "round": 1, "row": 6, "spent_dft": 112}`},
		},
		{
			name: "a player leaving it after Disengage", setup: then(goblinNextToToren, disengaging), col: 4, row: 6,
			want: moveWant{col: 4, row: 6, usedDft: 112, lastDft: 112, event: `{"col": 4, "combatant_id": "<Toren>", "cost_dft": 112, "cost_ft": 11, "distance_dft": 112, "distance_ft": 11, "from": {"col": 6, "cover": "none", "placed": true, "row": 7}, "on_turn": true, "round": 1, "row": 6, "spent_dft": 112}`},
		},
		{
			name: "the master moving a player on turn out of the reach", setup: goblinNextToToren, as: "master", col: 4, row: 6,
			want: moveWant{col: 4, row: 6, usedDft: 0, lastDft: 0, offered: true, event: `{"col": 4, "combatant_id": "<Toren>", "distance_dft": 112, "distance_ft": 11, "from": {"col": 6, "cover": "none", "placed": true, "row": 7}, "move_id": "<offers>", "on_turn": true, "round": 1, "row": 6}`},
		},
		{
			name: "the master, forced", setup: goblinNextToToren, as: "master", col: 4, row: 6, edit: []func(*playv1.MoveCombatantRequest){asForced},
			want: moveWant{col: 4, row: 6, usedDft: 0, lastDft: 0, event: `{"col": 4, "combatant_id": "<Toren>", "distance_dft": 112, "distance_ft": 11, "from": {"col": 6, "cover": "none", "placed": true, "row": 7}, "on_turn": true, "round": 1, "row": 6}`},
		},
		{
			name: "moving inside the reach", setup: goblinNextToToren, col: 7, row: 7,
			want: moveWant{col: 7, row: 7, usedDft: 50, lastDft: 50, event: `{"col": 7, "combatant_id": "<Toren>", "cost_dft": 50, "cost_ft": 5, "distance_dft": 50, "distance_ft": 5, "from": {"col": 6, "cover": "none", "placed": true, "row": 7}, "on_turn": true, "round": 1, "row": 7, "spent_dft": 50}`},
		},
		{
			name: "a move clears the master's cover mark", setup: marked("Toren", playv1.CoverDegree_COVER_DEGREE_HALF), col: 7, row: 7,
			want: moveWant{col: 7, row: 7, usedDft: 50, lastDft: 50, cover: "none", event: `{"col": 7, "combatant_id": "<Toren>", "cost_dft": 50, "cost_ft": 5, "distance_dft": 50, "distance_ft": 5, "from": {"col": 6, "cover": "half", "placed": true, "row": 7}, "on_turn": true, "round": 1, "row": 7, "spent_dft": 50}`},
		},
		{
			// The mark of a creature the players do not see clears too; only the master is told.
			name: "a hidden creature's cover mark clears", setup: then(hiding("Goblin 1"), marked("Goblin 1", playv1.CoverDegree_COVER_DEGREE_HALF)),
			as: "master", label: "Goblin 1", col: 17, row: 5,
			want: moveWant{col: 17, row: 5, usedDft: 0, lastDft: 0, cover: "none", event: `{"col": 17, "combatant_id": "<Goblin 1>", "distance_dft": 50, "distance_ft": 5, "from": {"col": 18, "cover": "half", "placed": true, "row": 5}, "round": 1, "row": 5, "secret": true}`},
		},
		{
			name: "a move that goes nowhere keeps it", setup: marked("Toren", playv1.CoverDegree_COVER_DEGREE_HALF), col: 6, row: 7,
			want: moveWant{col: 6, row: 7, usedDft: 0, lastDft: 0, cover: "half", event: `{"col": 6, "combatant_id": "<Toren>", "from": {"col": 6, "cover": "half", "placed": true, "row": 7}, "on_turn": true, "round": 1, "row": 7}`},
		},
	})
}

// The conditions that change what a foot costs.
func TestMoveCombatantFrozen_Conditions(t *testing.T) {
	t.Parallel()
	runCaveMoves(t, []caveMove{
		{
			// Crawling: every foot costs one more. The run adds the length, not the cost.
			name: "prone crawls at twice the cost", setup: conditioned("Toren", "condition:prone"), col: 8, row: 7,
			want: moveWant{col: 8, row: 7, usedDft: 200, lastDft: 100, event: `{"col": 8, "combatant_id": "<Toren>", "cost_dft": 100, "cost_ft": 10, "crawling": true, "distance_dft": 100, "distance_ft": 10, "from": {"col": 6, "cover": "none", "placed": true, "row": 7}, "on_turn": true, "round": 1, "row": 7, "spent_dft": 200}`},
		},
		{
			name: "prone, a forced move is no crawl", setup: conditioned("Toren", "condition:prone"), as: "master", col: 8, row: 7, edit: []func(*playv1.MoveCombatantRequest){asForced},
			want: moveWant{col: 8, row: 7, usedDft: 0, lastDft: 0, event: `{"col": 8, "combatant_id": "<Toren>", "distance_dft": 100, "distance_ft": 10, "from": {"col": 6, "cover": "none", "placed": true, "row": 7}, "on_turn": true, "round": 1, "row": 7}`},
		},
		{
			// A flier's walk is no run: it leaves no running start behind.
			name: "a flier's move adds no running start", setup: flying("Toren", 60), col: 8, row: 7,
			want: moveWant{col: 8, row: 7, usedDft: 100, lastDft: 0, event: `{"col": 8, "combatant_id": "<Toren>", "cost_dft": 100, "cost_ft": 10, "distance_dft": 100, "distance_ft": 10, "from": {"col": 6, "cover": "none", "placed": true, "row": 7}, "on_turn": true, "round": 1, "row": 7, "spent_dft": 100}`},
		},
		{
			name: "a prone flier does not crawl", setup: then(flying("Toren", 60), conditioned("Toren", "condition:prone")), col: 8, row: 7,
			want: moveWant{col: 8, row: 7, usedDft: 100, lastDft: 0, event: `{"col": 8, "combatant_id": "<Toren>", "cost_dft": 100, "cost_ft": 10, "distance_dft": 100, "distance_ft": 10, "from": {"col": 6, "cover": "none", "placed": true, "row": 7}, "on_turn": true, "round": 1, "row": 7, "spent_dft": 100}`},
		},
		{
			name: "grappled has no speed", setup: conditioned("Toren", "condition:grappled"), col: 7, row: 7,
			refused: ptr(blockedBy("TOO_FAR", "that is beyond your movement", `"missingFt":5`, `"missingDft":50`)),
		},
	})
}
