package play

import (
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// Mounting and dismounting (SRD 5.1, "Mounting and Dismounting", "Mounted Combat"). The fixture is the cave of combat_move_test.go:
// Toren (Medium, 30 ft of speed, on turn first) stands on (6,7) and the Ogro (Large, speed 30) is the steed the tests bring next to him.

var (
	reasonNotAMount     = playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_A_MOUNT
	reasonMountTooSmall = playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_MOUNT_TOO_SMALL
	reasonMountTooFar   = playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_MOUNT_TOO_FAR
	reasonMountMoveUsed = playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_MOUNT_MOVE_USED
	reasonMounted       = playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_MOUNTED
	reasonNoRoom        = playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NO_ROOM_TO_DISMOUNT
	reasonAlreadyRiding = playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ALREADY_MOUNTED
	reasonMountTaken    = playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_MOUNT_TAKEN
)

// mountCall is MountCombatant as u with a key.
func (c *cave) mountCall(t *testing.T, u *user, key, rider, mount string) (*playv1.MountCombatantResponse, error) {
	t.Helper()
	res, err := u.combat.MountCombatant(t.Context(), connect.NewRequest(&playv1.MountCombatantRequest{
		CampaignId: c.campaignID, EncounterId: c.get(t, c.master).GetId(), RiderId: c.id(t, rider), MountId: c.id(t, mount), IdempotencyKey: key,
	}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

// mountAs is a MountCombatant of a new key.
func (c *cave) mountAs(t *testing.T, u *user, rider, mount string) (*playv1.MountCombatantResponse, error) {
	t.Helper()
	return c.mountCall(t, u, newKey(), rider, mount)
}

// mustMount is a mount that must work.
func (c *cave) mustMount(t *testing.T, u *user, rider, mount string) *playv1.MountCombatantResponse {
	t.Helper()
	res, err := c.mountAs(t, u, rider, mount)
	if err != nil {
		t.Fatalf("MountCombatant(%s on %s) error = %v", rider, mount, err)
	}
	return res
}

// dismountAs is DismountCombatant as u.
func (c *cave) dismountAs(t *testing.T, u *user, rider string, col, row int32) (*playv1.DismountCombatantResponse, error) {
	t.Helper()
	res, err := u.combat.DismountCombatant(t.Context(), connect.NewRequest(&playv1.DismountCombatantRequest{
		CampaignId: c.campaignID, EncounterId: c.get(t, c.master).GetId(), RiderId: c.id(t, rider), Col: col, Row: row, IdempotencyKey: newKey(),
	}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

// steed makes the Ogro Toren's willing mount: marked, on the party's side, next to him on (7,7).
func (c *cave) steed(t *testing.T) {
	t.Helper()
	c.mustMark(t, "Ogro", "controlled")
	c.side(t, "Ogro", playv1.CombatantSide_COMBATANT_SIDE_PARTY)
	c.mustMove(t, c.master, "Ogro", 7, 7)
}

// rideRow is what the table keeps of a rider or a mount.
type rideRow struct {
	Col, Row  int32
	UsedDft   int32
	MoveUsed  bool
	MountedOn string
}

func (c *cave) rideRowOf(t *testing.T, label string) rideRow {
	t.Helper()
	var r rideRow
	var col, row *int32
	var on *string
	err := c.h.pool.QueryRow(t.Context(),
		`SELECT grid_col, grid_row, movement_used_dft, mount_move_used, mounted_on_id::text FROM combatants WHERE id = $1`, c.id(t, label),
	).Scan(&col, &row, &r.UsedDft, &r.MoveUsed, &on)
	if err != nil {
		t.Fatalf("read the ride columns of %s: %v", label, err)
	}
	if col != nil && row != nil {
		r.Col, r.Row = *col, *row
	}
	if on != nil {
		r.MountedOn = *on
	}
	return r
}

// sitOn puts the rider on the mount in the table, on the mount's square: the setup of a test that is not about mounting.
func (c *cave) sitOn(t *testing.T, rider, mount string) {
	t.Helper()
	c.execSQL(t, `UPDATE combatants SET mounted_on_id = $1, grid_col = m.grid_col, grid_row = m.grid_row FROM combatants m WHERE m.id = $1 AND combatants.id = $2`,
		c.id(t, mount), c.id(t, rider))
}

func (c *cave) eventsOf(t *testing.T, kind string) int {
	t.Helper()
	var n int
	if err := c.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM session_events WHERE kind = $1`, kind).Scan(&n); err != nil {
		t.Fatalf("count the %s events: %v", kind, err)
	}
	return n
}

func (c *cave) linesOfKind(t *testing.T, u *user, kind playv1.CombatLogKind) []*playv1.CombatLogEntry {
	t.Helper()
	var out []*playv1.CombatLogEntry
	for _, en := range c.logEntries(t, u, c.get(t, u)) {
		if en.GetKind() == kind {
			out = append(out, en)
		}
	}
	return out
}

// TestMountedCombat_MountTakesTheMountsSquareAndHalfTheSpeed: Toren gets on the Ogro: he takes its square, half of his 30 ft
// of speed goes from his movement, the pair reads as linked to the players who see both, the log says it and a replay of the key
// is the same answer with one event.
func TestMountedCombat_MountTakesTheMountsSquareAndHalfTheSpeed(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.fight(t)
	c.steed(t)
	key := newKey()
	first, err := c.mountCall(t, c.caio, key, "Toren", "Ogro")
	if err != nil {
		t.Fatalf("MountCombatant() error = %v", err)
	}
	if first.GetMovementLeftDft() != 150 {
		t.Errorf("the movement left after the mount = %d, want 150", first.GetMovementLeftDft())
	}
	again, err := c.mountCall(t, c.caio, key, "Toren", "Ogro")
	if err != nil || again.GetMovementLeftDft() != 150 {
		t.Errorf("the replay of the same key = %v, %v, want the same answer", again, err)
	}
	if n := c.eventsOf(t, "combatant_mounted"); n != 1 {
		t.Errorf("combatant_mounted events = %d, want 1 (a replay writes none)", n)
	}
	toren, ogro := c.rideRowOf(t, "Toren"), c.rideRowOf(t, "Ogro")
	if toren.Col != 7 || toren.Row != 7 || ogro.Col != 7 || ogro.Row != 7 {
		t.Errorf("Toren at (%d,%d) and the Ogro at (%d,%d), want both on (7,7)", toren.Col, toren.Row, ogro.Col, ogro.Row)
	}
	if toren.UsedDft != 150 || !toren.MoveUsed || toren.MountedOn != c.id(t, "Ogro") {
		t.Errorf("Toren's row = %+v, want 150 used, the move used and the Ogro as the mount", toren)
	}
	if ogro.UsedDft != 0 {
		t.Errorf("the mount's own movement used = %d, want 0: mounting costs the rider's", ogro.UsedDft)
	}
	for name, u := range map[string]*user{"master": c.master, "caio": c.caio, "ana": c.ana} {
		if got := c.who(t, u, "Toren").GetMountedOnId(); got != c.id(t, "Ogro") {
			t.Errorf("%s reads Toren mounted on %q, want the Ogro", name, got)
		}
		if got := c.who(t, u, "Ogro").GetRiderId(); got != c.id(t, "Toren") {
			t.Errorf("%s reads the Ogro's rider %q, want Toren", name, got)
		}
		lines := c.linesOfKind(t, u, playv1.CombatLogKind_COMBAT_LOG_KIND_MOUNTED)
		if len(lines) != 1 || lines[0].GetActorLabel() != "Toren" || lines[0].GetTargetLabel() != "Ogro" {
			t.Errorf("%s reads the mounted lines %v, want one: Toren on the Ogro", name, lines)
		}
	}
}

// TestMountedCombat_TheMountMustBeOneSizeLarger: SRD 5.1 "Mounted Combat": a mount at least one size larger than its rider,
// for the master's mount too.
func TestMountedCombat_TheMountMustBeOneSizeLarger(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.fight(t)
	c.steed(t)
	sizes := []string{"tiny", "small", "medium", "large", "huge", "gargantuan"}
	for r, rider := range sizes {
		for m, mount := range sizes {
			c.execSQL(t, `UPDATE combatants SET size = $2 WHERE id = $1`, c.id(t, "Toren"), rider)
			c.execSQL(t, `UPDATE combatants SET size = $2 WHERE id = $1`, c.id(t, "Ogro"), mount)
			_, err := c.mountAs(t, c.master, "Toren", "Ogro")
			if m > r {
				if err != nil {
					t.Errorf("a %s rider on a %s mount = %v, want it allowed", rider, mount, err)
				}
				c.execSQL(t, `UPDATE combatants SET mounted_on_id = NULL, mount_move_used = false, movement_used_dft = 0, grid_col = 6, grid_row = 7 WHERE id = $1`, c.id(t, "Toren"))
				continue
			}
			b := wantBlockedBy(t, "a "+rider+" rider on a "+mount+" mount", err, reasonMountTooSmall)
			if len(b.GetCombatantIds()) != 1 || b.GetCombatantIds()[0] != c.id(t, "Ogro") {
				t.Errorf("a %s rider on a %s mount: combatant_ids = %v, want the mount", rider, mount, b.GetCombatantIds())
			}
		}
	}
}

// TestMountedCombat_TheCostFollowsTheSurfaceAndTheSpeed: half the speed of the surface the rider stands on, the Dash adds
// movement and not speed, no speed or too little movement left refuses with TOO_FAR, and an exact fit works.
func TestMountedCombat_TheCostFollowsTheSurfaceAndTheSpeed(t *testing.T) {
	t.Parallel()
	t.Run("swimming speed in water", func(t *testing.T) {
		t.Parallel()
		c := newCave(t)
		c.terrain.paintSurface(grid.SurfaceWater, grid.Square{Col: 6, Row: 7})
		c.fight(t)
		c.steed(t)
		withSpeed("Toren", "speed_swim_ft", 40)(t, c)
		res := c.mustMount(t, c.caio, "Toren", "Ogro")
		if got := c.rideRowOf(t, "Toren").UsedDft; got != 200 || res.GetMovementLeftDft() != 100 {
			t.Errorf("the mount cost %d and left %d, want 200 and 100 (half of the 40 ft swim speed; the rider now stands on the mount's floor, where 20 ft of its 30 are spent)", got, res.GetMovementLeftDft())
		}
	})
	t.Run("no speed", func(t *testing.T) {
		t.Parallel()
		c := newCave(t)
		c.fight(t)
		c.steed(t)
		withSpeed("Toren", "speed_ft", 0)(t, c)
		_, err := c.mountAs(t, c.caio, "Toren", "Ogro")
		wantBlockedBy(t, "a mount with speed 0", err, reasonTooFar)
		if got := c.rideRowOf(t, "Toren"); got.MountedOn != "" || got.UsedDft != 0 || got.Col != 6 {
			t.Errorf("Toren's row after the refusal = %+v, want nothing changed", got)
		}
	})
	t.Run("the Dash does not double it", func(t *testing.T) {
		t.Parallel()
		c := newCave(t)
		c.fight(t)
		c.steed(t)
		c.execSQL(t, `UPDATE combatants SET dashed = true WHERE id = $1`, c.id(t, "Toren"))
		res := c.mustMount(t, c.caio, "Toren", "Ogro")
		if got := c.rideRowOf(t, "Toren").UsedDft; got != 150 || res.GetMovementLeftDft() != 450 {
			t.Errorf("after the Dash the mount cost %d and left %d, want 150 and 450", got, res.GetMovementLeftDft())
		}
	})
	t.Run("not enough left and an exact fit", func(t *testing.T) {
		t.Parallel()
		c := newCave(t)
		c.fight(t)
		c.steed(t)
		c.execSQL(t, `UPDATE combatants SET movement_used_dft = 200 WHERE id = $1`, c.id(t, "Toren"))
		_, err := c.mountAs(t, c.caio, "Toren", "Ogro")
		if b := wantBlockedBy(t, "a mount with 10 ft left", err, reasonTooFar); b.GetMissingDft() != 50 {
			t.Errorf("missing = %d dft, want 50", b.GetMissingDft())
		}
		c.execSQL(t, `UPDATE combatants SET movement_used_dft = 150 WHERE id = $1`, c.id(t, "Toren"))
		res := c.mustMount(t, c.caio, "Toren", "Ogro")
		if res.GetMovementLeftDft() != 0 || c.rideRowOf(t, "Toren").UsedDft != 300 {
			t.Errorf("an exact fit left %d with %d used, want 0 and 300", res.GetMovementLeftDft(), c.rideRowOf(t, "Toren").UsedDft)
		}
	})
}

// TestMountedCombat_OneMountOrDismountPerMove: a rider mounts or dismounts once in a move; its next turn allows it again.
func TestMountedCombat_OneMountOrDismountPerMove(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.fight(t)
	c.steed(t)
	c.mustMount(t, c.caio, "Toren", "Ogro")
	_, err := c.dismountAs(t, c.caio, "Toren", 8, 7)
	b := wantBlockedBy(t, "a dismount in the move that mounted", err, reasonMountMoveUsed)
	if len(b.GetCombatantIds()) != 1 || b.GetCombatantIds()[0] != c.id(t, "Toren") {
		t.Errorf("combatant_ids = %v, want the rider", b.GetCombatantIds())
	}
	c.nextTurnOf(t, "Toren")
	if c.rideRowOf(t, "Toren").MoveUsed {
		t.Fatalf("the mark of the used move is still set in the next turn")
	}
	res, err := c.dismountAs(t, c.caio, "Toren", 8, 7)
	if err != nil {
		t.Fatalf("the dismount of the next turn = %v", err)
	}
	if res.GetMovementLeftDft() != 150 {
		t.Errorf("the movement left after the dismount = %d, want 150 (half of 30 ft from 30 ft)", res.GetMovementLeftDft())
	}
	_, err = c.mountAs(t, c.caio, "Toren", "Ogro")
	wantBlockedBy(t, "a mount in the move that dismounted", err, reasonMountMoveUsed)
}

// TestMountedCombat_WhoMayBeMounted: a player mounts a creature of their own and the party's mounts the master marked; the master
// mounts anything, an unmarked combatant as well. A player's character is nobody's mount.
func TestMountedCombat_WhoMayBeMounted(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.fight(t)
	c.mustMove(t, c.master, "Ogro", 7, 7)
	_, err := c.mountAs(t, c.caio, "Toren", "Ogro")
	wantBlockedBy(t, "an unmarked NPC mounted by a player", err, reasonNotAMount)
	c.mustMark(t, "Ogro", "controlled")
	_, err = c.mountAs(t, c.caio, "Toren", "Ogro")
	wantBlockedBy(t, "a marked NPC of the enemy side mounted by a player", err, reasonNotAMount)
	c.side(t, "Ogro", playv1.CombatantSide_COMBATANT_SIDE_PARTY)
	_, err = c.mountAs(t, c.caio, "Toren", "Escudeiro")
	wantBlockedBy(t, "an unmarked ally", err, reasonNotAMount)
	if _, err := c.mountAs(t, c.caio, "Toren", "Ogro"); err != nil {
		t.Fatalf("a marked mount of the party's side = %v, want it allowed", err)
	}
	c.undoLast(t)

	// The master mounts an unmarked enemy and the call needs no mark, but still the size.
	c.mustMark(t, "Ogro", "")
	c.side(t, "Ogro", playv1.CombatantSide_COMBATANT_SIDE_ENEMY)
	if _, err := c.mountAs(t, c.master, "Toren", "Ogro"); err != nil {
		t.Fatalf("the master mounting an unmarked enemy = %v, want it allowed", err)
	}
	c.undoLast(t)
	c.mustMove(t, c.master, "Goblin 1", 5, 7)
	_, err = c.mountAs(t, c.master, "Toren", "Goblin 1")
	wantBlockedBy(t, "the master on a Small goblin", err, reasonMountTooSmall)
	c.mustMove(t, c.master, "Pensantus", 7, 8)
	c.execSQL(t, `UPDATE combatants SET size = 'large' WHERE id = $1`, c.id(t, "Pensantus"))
	_, err = c.mountAs(t, c.master, "Toren", "Pensantus")
	wantBlockedBy(t, "a player's character as a mount", err, reasonNotAMount)
}

// TestMountedCombat_AnOwnCreatureNeedsNoMark: a creature of the rider's own player is a willing mount without the master's mark;
// another player's creature needs it.
func TestMountedCombat_AnOwnCreatureNeedsNoMark(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.give(t, c.toren, "monster:wolf", "Presa")
	c.start(t, plan{
		npcs: []*playv1.Participant{
			{CharacterId: c.capitao.GetId()}, {CharacterId: c.goblins.GetId(), Count: 3}, {CharacterId: c.ogre.GetId()}, {CharacterId: c.squire.GetId()},
		},
		npcRolls: []int{2, 2, 2, 2, 2, 2},
		players:  map[string]int32{"Toren": 18, "Pensantus": 10, "Brisa": 1, "Presa": 15},
		reveal:   []string{"Capitão Goblin", "Goblin 1", "Goblin 2", "Goblin 3", "Ogro", "Escudeiro"},
		at: map[string][2]int32{
			"Toren": {6, 7}, "Pensantus": {5, 8}, "Brisa": {4, 7}, "Escudeiro": {3, 8}, "Presa": {7, 7},
			"Goblin 1": {18, 5}, "Goblin 2": {20, 7}, "Goblin 3": {21, 3}, "Capitão Goblin": {12, 13}, "Ogro": {22, 9},
		},
	})
	c.execSQL(t, `UPDATE combatants SET size = 'large' WHERE id = $1`, c.id(t, "Presa"))
	if _, err := c.mountAs(t, c.caio, "Toren", "Presa"); err != nil {
		t.Fatalf("Toren on his own creature = %v, want it allowed with no mark", err)
	}
	c.undoLast(t)
	c.mustMove(t, c.master, "Presa", 5, 7)
	c.passTo(t, c.get(t, c.master), "Brisa")
	_, err := c.mountAs(t, c.bia, "Brisa", "Presa")
	wantBlockedBy(t, "Brisa on Toren's unmarked creature", err, reasonNotAMount)
	c.mustMark(t, "Presa", "independent")
	if _, err := c.mountAs(t, c.bia, "Brisa", "Presa"); err != nil {
		t.Fatalf("Brisa on a creature of the party's side the master marked = %v, want it allowed", err)
	}
}

// TestMountedCombat_TheMountIsWithinFiveFeet: a diagonal neighbor counts as 5 ft, two squares do not.
func TestMountedCombat_TheMountIsWithinFiveFeet(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.fight(t)
	c.mustMark(t, "Ogro", "controlled")
	c.side(t, "Ogro", playv1.CombatantSide_COMBATANT_SIDE_PARTY)
	c.mustMove(t, c.master, "Ogro", 8, 7)
	_, err := c.mountAs(t, c.caio, "Toren", "Ogro")
	if b := wantBlockedBy(t, "a mount two squares away", err, reasonMountTooFar); len(b.GetCombatantIds()) != 1 || b.GetCombatantIds()[0] != c.id(t, "Ogro") {
		t.Errorf("combatant_ids = %v, want the mount", b.GetCombatantIds())
	}
	c.mustMove(t, c.master, "Ogro", 7, 8)
	if _, err := c.mountAs(t, c.caio, "Toren", "Ogro"); err != nil {
		t.Fatalf("a mount on the diagonal = %v, want it allowed", err)
	}
	if got := c.rideRowOf(t, "Toren"); got.Col != 7 || got.Row != 8 {
		t.Errorf("Toren on (%d,%d), want the mount's (7,8)", got.Col, got.Row)
	}
}

// TestMountedCombat_OneRiderPerMountAndOneMountPerRider: a mount that carries a rider is taken; a rider does not mount another
// mount; a mount that is itself a rider cannot carry.
func TestMountedCombat_OneRiderPerMountAndOneMountPerRider(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.fight(t)
	c.steed(t)
	c.mustMove(t, c.master, "Escudeiro", 5, 7)
	c.sitOn(t, "Escudeiro", "Ogro")
	_, err := c.mountAs(t, c.caio, "Toren", "Ogro")
	if b := wantBlockedBy(t, "a mount that carries a rider", err, reasonMountTaken); len(b.GetCombatantIds()) != 1 || b.GetCombatantIds()[0] != c.id(t, "Ogro") {
		t.Errorf("combatant_ids = %v, want the mount", b.GetCombatantIds())
	}
	c.execSQL(t, `UPDATE combatants SET mounted_on_id = NULL WHERE id = $1`, c.id(t, "Escudeiro"))
	c.sitOn(t, "Toren", "Ogro")
	_, err = c.mountAs(t, c.caio, "Toren", "Escudeiro")
	if b := wantBlockedBy(t, "a rider that mounts again", err, reasonAlreadyRiding); len(b.GetCombatantIds()) != 1 || b.GetCombatantIds()[0] != c.id(t, "Toren") {
		t.Errorf("combatant_ids = %v, want the rider", b.GetCombatantIds())
	}
}

// TestMountedCombat_DismountLandsNextToTheMount: the rider lands on a free square next to the mount (a diagonal one too), pays
// half the speed again on the next turn, leaves the mount where it is, and the log says it. A wall, a taken square, a square
// that is not next to the mount and a rider that rides nothing are refused.
func TestMountedCombat_DismountLandsNextToTheMount(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.terrain.addWalls(grid.Square{Col: 8, Row: 8})
	c.fight(t)
	c.steed(t)
	_, err := c.dismountAs(t, c.caio, "Toren", 8, 7)
	wantCode(t, "a dismount of a rider that rides nothing", err, connect.CodeInvalidArgument)
	c.mustMount(t, c.caio, "Toren", "Ogro")
	c.nextTurnOf(t, "Toren")
	c.mustMove(t, c.master, "Escudeiro", 6, 8)
	for name, sq := range map[string][2]int32{"not next to the mount": {9, 7}, "a wall": {8, 8}, "a taken square": {6, 8}, "the mount's own": {7, 7}} {
		_, err := c.dismountAs(t, c.caio, "Toren", sq[0], sq[1])
		wantBlockedBy(t, "a dismount on "+name, err, reasonNoRoom)
	}
	if got := c.rideRowOf(t, "Toren"); got.MountedOn == "" || got.UsedDft != 0 {
		t.Fatalf("Toren after the refusals = %+v, want still mounted and nothing spent", got)
	}
	res, err := c.dismountAs(t, c.caio, "Toren", 8, 7)
	if err != nil {
		t.Fatalf("the dismount on a free square = %v", err)
	}
	toren, ogro := c.rideRowOf(t, "Toren"), c.rideRowOf(t, "Ogro")
	if toren.Col != 8 || toren.Row != 7 || toren.MountedOn != "" || toren.UsedDft != 150 || res.GetMovementLeftDft() != 150 || !toren.MoveUsed {
		t.Errorf("Toren after the dismount = %+v (left %d), want (8,7), on foot, 150 used and the move used", toren, res.GetMovementLeftDft())
	}
	if ogro.Col != 7 || ogro.Row != 7 {
		t.Errorf("the mount is on (%d,%d), want it still on (7,7)", ogro.Col, ogro.Row)
	}
	if lines := c.linesOfKind(t, c.ana, playv1.CombatLogKind_COMBAT_LOG_KIND_DISMOUNTED); len(lines) != 1 || lines[0].GetActorLabel() != "Toren" || lines[0].GetTargetLabel() != "Ogro" {
		t.Errorf("Ana reads the dismounted lines %v, want one: Toren from the Ogro", lines)
	}
	if c.who(t, c.ana, "Toren").GetMountedOnId() != "" || c.who(t, c.ana, "Ogro").GetRiderId() != "" {
		t.Errorf("Ana still reads a link after the dismount")
	}
}

// TestMountedCombat_OutOfTurnRefusals: the rider mounts on its own turn, the master too; another player's rider is not the
// caller's; a rider that cannot act does not mount.
func TestMountedCombat_OutOfTurnRefusals(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.fight(t)
	c.steed(t)
	c.mustMove(t, c.master, "Pensantus", 6, 8)
	_, err := c.mountAs(t, c.ana, "Pensantus", "Ogro")
	wantBlockedBy(t, "a mount out of turn", err, reasonNotTurn)
	_, err = c.mountAs(t, c.master, "Pensantus", "Ogro")
	wantBlockedBy(t, "the master's mount out of turn", err, reasonNotTurn)
	_, err = c.mountAs(t, c.bia, "Toren", "Ogro")
	wantCode(t, "a mount of another player's character", err, connect.CodePermissionDenied)
	c.setConditions(t, c.get(t, c.master), "Toren", "condition:paralyzed")
	_, err = c.mountAs(t, c.caio, "Toren", "Ogro")
	wantBlockedBy(t, "a paralyzed rider", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_CANNOT_ACT)
}

// TestMountedCombat_ThePairIsOneOccupant: a third combatant cannot enter the pair's square, the rider's old square is free
// again, and the mount's own options never find its rider in the way.
func TestMountedCombat_ThePairIsOneOccupant(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.fight(t)
	c.steed(t)
	c.mustMount(t, c.caio, "Toren", "Ogro")
	c.nextTurnOf(t, "Pensantus") // the master may stack combatants, so a player walks the third one
	_, err := c.move(t, c.ana, "Pensantus", 7, 7)
	wantBlockedBy(t, "a combatant put on the pair's square", err, reasonOccupied)
	opts, err := c.options(t, c.ana, "Pensantus")
	if err != nil {
		t.Fatalf("GetMoveOptions(Pensantus) error = %v", err)
	}
	if r := reachableAt(opts, 7, 7); r != nil {
		t.Errorf("the pair's square is on offer to a third combatant: %v", r)
	}
	if _, err := c.move(t, c.ana, "Pensantus", 6, 7); err != nil {
		t.Errorf("a combatant put on the rider's old square = %v, want it free", err)
	}
	// The mount's own options: its square and its neighbors are not blocked by its rider, who stands on the mount's own square.
	mopts, err := c.options(t, c.master, "Ogro")
	if err != nil {
		t.Fatalf("GetMoveOptions(Ogro) error = %v", err)
	}
	if r := reachableAt(mopts, 8, 7); r == nil {
		t.Errorf("the mount cannot go to (8,7), a free square next to it")
	}
	for _, rf := range mopts.GetRefused() {
		if rf.GetCol() == 7 && rf.GetRow() == 7 {
			t.Errorf("the mount's own square is refused (%v): its rider is not in its way", rf.GetReason())
		}
	}
}

// TestMountedCombat_ReachCoverAndRangeAreTheSameAtTheSharedSquare: an attacker's distance to the rider and to the mount is one
// number, and so is the cover; the rider's body is not cover for the mount, nor the mount's for the rider.
func TestMountedCombat_ReachCoverAndRangeAreTheSameAtTheSharedSquare(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.fight(t)
	c.steed(t)
	c.mustMount(t, c.caio, "Toren", "Ogro")
	c.mustMove(t, c.master, "Goblin 1", 9, 8)
	opts, err := c.master.combat.GetTurnOptions(t.Context(), connect.NewRequest(&playv1.GetTurnOptionsRequest{CampaignId: c.campaignID, EncounterId: c.get(t, c.master).GetId(), CombatantId: c.id(t, "Goblin 1")}))
	if err != nil {
		t.Fatalf("GetTurnOptions(Goblin 1) error = %v", err)
	}
	var toren, ogro *playv1.TargetInReach
	for _, at := range opts.Msg.GetAttackTargets() {
		for _, tg := range at.GetTargets() {
			switch tg.GetCombatantId() {
			case c.id(t, "Toren"):
				toren = tg
			case c.id(t, "Ogro"):
				ogro = tg
			}
		}
	}
	if toren == nil || ogro == nil {
		t.Fatalf("the goblin's targets hold Toren %v and the Ogro %v, want both", toren, ogro)
	}
	if toren.GetDistanceFt() != ogro.GetDistanceFt() || toren.GetCover() != ogro.GetCover() || toren.GetTooFar() != ogro.GetTooFar() {
		t.Errorf("distance %d/%d, cover %v/%v, too far %v/%v for the rider and the mount, want the same",
			toren.GetDistanceFt(), ogro.GetDistanceFt(), toren.GetCover(), ogro.GetCover(), toren.GetTooFar(), ogro.GetTooFar())
	}
	if toren.GetCover() != playv1.CoverDegree_COVER_DEGREE_NONE && toren.GetCover() != playv1.CoverDegree_COVER_DEGREE_UNSPECIFIED {
		t.Errorf("the pair gives itself cover: %v", toren.GetCover())
	}
}

// TestMountedCombat_TheMountsMoveCarriesTheRider: a move of a controlled mount by the rider's player spends the mount's own
// movement and not the rider's, the rider's square follows, the rider's own move is refused whoever asks, and the undo puts both
// back.
func TestMountedCombat_TheMountsMoveCarriesTheRider(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.fight(t)
	c.steed(t)
	c.mustMount(t, c.caio, "Toren", "Ogro")
	_, err := c.move(t, c.caio, "Toren", 8, 7)
	b := wantBlockedBy(t, "the rider's own move", err, reasonMounted)
	if len(b.GetCombatantIds()) != 1 || b.GetCombatantIds()[0] != c.id(t, "Toren") {
		t.Errorf("combatant_ids = %v, want the rider", b.GetCombatantIds())
	}
	_, err = c.move(t, c.master, "Toren", 8, 7, func(r *playv1.MoveCombatantRequest) { r.Forced = true })
	wantBlockedBy(t, "the master's forced move of the rider", err, reasonMounted)
	_, err = c.move(t, c.ana, "Ogro", 8, 7)
	wantCode(t, "another player's move of the steed", err, connect.CodePermissionDenied)

	if _, err := c.move(t, c.caio, "Ogro", 10, 7); err != nil {
		t.Fatalf("the rider's player moving the controlled mount = %v", err)
	}
	toren, ogro := c.rideRowOf(t, "Toren"), c.rideRowOf(t, "Ogro")
	if toren.Col != 10 || toren.Row != 7 || ogro.Col != 10 || ogro.Row != 7 {
		t.Errorf("Toren at (%d,%d) and the Ogro at (%d,%d), want both on (10,7)", toren.Col, toren.Row, ogro.Col, ogro.Row)
	}
	if ogro.UsedDft != 150 || toren.UsedDft != 150 {
		t.Errorf("the mount spent %d and the rider %d, want 150 and 150 (the mount's three squares, the rider's mount)", ogro.UsedDft, toren.UsedDft)
	}
	if left := c.who(t, c.caio, "Ogro").GetMovementLeftDft(); left != 150 {
		t.Errorf("the mount has %d left, want 150 of its own 30 ft", left)
	}
	c.undoLast(t)
	toren, ogro = c.rideRowOf(t, "Toren"), c.rideRowOf(t, "Ogro")
	if toren.Col != 7 || ogro.Col != 7 || ogro.UsedDft != 0 || toren.UsedDft != 150 || toren.MountedOn != c.id(t, "Ogro") {
		t.Errorf("after the undo Toren = %+v and the Ogro = %+v, want both on (7,7), the mount's movement back and Toren still on it", toren, ogro)
	}
}

// TestMountedCombat_AnIndependentMountMovesOnItsOwnTurn: the master moves it on its turn, the rider is carried, and the rider's
// player does not move it.
func TestMountedCombat_AnIndependentMountMovesOnItsOwnTurn(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.fight(t)
	c.steed(t)
	c.mustMark(t, "Ogro", "independent")
	c.mustMount(t, c.caio, "Toren", "Ogro")
	_, err := c.move(t, c.caio, "Ogro", 8, 7)
	wantCode(t, "the rider's player moving an independent mount", err, connect.CodePermissionDenied)
	c.passTo(t, c.get(t, c.master), "Ogro")
	if _, err := c.move(t, c.master, "Ogro", 9, 7); err != nil {
		t.Fatalf("the master moving the independent mount on its turn = %v", err)
	}
	if toren := c.rideRowOf(t, "Toren"); toren.Col != 9 || toren.Row != 7 {
		t.Errorf("Toren at (%d,%d), want (9,7) with the mount", toren.Col, toren.Row)
	}
}

// TestMountedCombat_AnyMoveOfTheMountCarriesTheRider: the master's forced move, a push of a spell and the undo of the push carry
// the rider; a rider is never pushed on its own.
func TestMountedCombat_AnyMoveOfTheMountCarriesTheRider(t *testing.T) {
	t.Parallel()
	t.Run("the master's forced move", func(t *testing.T) {
		t.Parallel()
		c := newCave(t)
		c.fight(t)
		c.steed(t)
		c.mustMount(t, c.caio, "Toren", "Ogro")
		c.mustMove(t, c.master, "Ogro", 11, 8)
		if toren := c.rideRowOf(t, "Toren"); toren.Col != 11 || toren.Row != 8 {
			t.Errorf("Toren at (%d,%d), want (11,8) with the mount", toren.Col, toren.Row)
		}
		c.undoLast(t)
		if toren := c.rideRowOf(t, "Toren"); toren.Col != 7 || toren.Row != 7 {
			t.Errorf("after the undo Toren at (%d,%d), want (7,7) with the mount", toren.Col, toren.Row)
		}
	})
	t.Run("a push", func(t *testing.T) {
		t.Parallel()
		c := newAreaCave(t)
		c.waveFight(t, [2]int32{17, 7}, [2]int32{18, 7})
		c.mustMove(t, c.master, "Goblin 3", 18, 7)
		c.sitOn(t, "Goblin 3", "Goblin 1")
		c.wave(t)
		g1, g3 := c.square(t, "Goblin 1"), c.square(t, "Goblin 3")
		if g1 != [2]int32{20, 7} || g3 != g1 {
			t.Errorf("Goblin 1 at %v and its rider at %v, want both pushed to (20,7)", g1, g3)
		}
		c.undoLast(t)
		if g1, g3 = c.square(t, "Goblin 1"), c.square(t, "Goblin 3"); g1 != [2]int32{18, 7} || g3 != g1 {
			t.Errorf("after the undo Goblin 1 at %v and its rider at %v, want both back on (18,7)", g1, g3)
		}
	})
}

// TestMountedCombat_TheRiderDoesNotTeleportAlone: a rider's Misty Step is refused, the rider gets off first.
func TestMountedCombat_TheRiderDoesNotTeleportAlone(t *testing.T) {
	t.Parallel()
	c := newCaveWith(t, 3, []string{mistyStep})
	c.fight(t)
	c.mustMark(t, "Ogro", "controlled")
	c.side(t, "Ogro", playv1.CombatantSide_COMBATANT_SIDE_PARTY)
	c.mustMove(t, c.master, "Ogro", 5, 9)
	c.passTo(t, c.get(t, c.master), "Pensantus")
	c.mustMount(t, c.ana, "Pensantus", "Ogro")
	_, err := c.misty(t, c.ana, at(8, 9))
	wantBlockedBy(t, "a rider's Misty Step", err, reasonMounted)
}

// TestMountedCombat_UndoTakesBackAMountAndADismount: the undo puts the rider's square, movement and link back, and the move of the
// rider's turn is free again.
func TestMountedCombat_UndoTakesBackAMountAndADismount(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.fight(t)
	c.steed(t)
	c.mustMount(t, c.caio, "Toren", "Ogro")
	c.undoLast(t)
	got := c.rideRowOf(t, "Toren")
	if got.Col != 6 || got.Row != 7 || got.UsedDft != 0 || got.MountedOn != "" || got.MoveUsed {
		t.Errorf("Toren after the undo of the mount = %+v, want (6,7), nothing spent, on foot and the move free", got)
	}
	if c.who(t, c.ana, "Ogro").GetRiderId() != "" {
		t.Errorf("the mount still reads a rider after the undo")
	}
	if lines := c.linesOfKind(t, c.ana, playv1.CombatLogKind_COMBAT_LOG_KIND_MOUNTED); len(lines) != 0 {
		t.Errorf("the undone mount is still a line of the log: %v", lines)
	}

	c.mustMount(t, c.caio, "Toren", "Ogro")
	c.nextTurnOf(t, "Toren")
	if _, err := c.dismountAs(t, c.caio, "Toren", 8, 7); err != nil {
		t.Fatalf("the dismount = %v", err)
	}
	c.undoLast(t)
	got = c.rideRowOf(t, "Toren")
	if got.Col != 7 || got.Row != 7 || got.UsedDft != 0 || got.MountedOn != c.id(t, "Ogro") || got.MoveUsed {
		t.Errorf("Toren after the undo of the dismount = %+v, want on the Ogro's square, nothing spent, mounted and the move free", got)
	}
}

// TestMountedCombat_TurnOptionsListTheMountsAndTheLanding: the options say what Toren could mount with the cost and why not, and,
// mounted, the cost of getting off and the free squares.
func TestMountedCombat_TurnOptionsListTheMountsAndTheLanding(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.terrain.addWalls(grid.Square{Col: 8, Row: 8})
	c.fight(t)
	c.steed(t)
	turn := func(u *user) *playv1.GetTurnOptionsResponse {
		t.Helper()
		res, err := u.combat.GetTurnOptions(t.Context(), connect.NewRequest(&playv1.GetTurnOptionsRequest{CampaignId: c.campaignID, EncounterId: c.get(t, c.master).GetId(), CombatantId: c.id(t, "Toren")}))
		if err != nil {
			t.Fatalf("GetTurnOptions(Toren) error = %v", err)
		}
		return res.Msg
	}
	mount := turn(c.caio).GetOptions().GetMount()
	if len(mount.GetMounts()) != 1 || mount.GetMounts()[0].GetCombatantId() != c.id(t, "Ogro") || mount.GetMounts()[0].GetCostDft() != 150 || mount.GetMounts()[0].GetRefusal() != 0 {
		t.Fatalf("the mounts on offer = %v, want the Ogro at 150 with no refusal", mount.GetMounts())
	}
	c.mustMove(t, c.master, "Ogro", 9, 7)
	mount = turn(c.caio).GetOptions().GetMount()
	if got := mount.GetMounts()[0].GetRefusal(); got != 2 { // MOUNT_REFUSAL_TOO_FAR
		t.Errorf("the refusal of a mount two squares away = %v, want TOO_FAR", got)
	}
	c.mustMove(t, c.master, "Ogro", 7, 7)
	c.mustMount(t, c.caio, "Toren", "Ogro")
	mount = turn(c.caio).GetOptions().GetMount()
	if mount.GetMountedOnId() != c.id(t, "Ogro") || mount.GetDismountCostDft() != 150 || mount.GetDismountRefusal() != 4 { // MOVE_USED
		t.Errorf("the mounted options = %v, want the Ogro, 150 and the refusal MOVE_USED", mount)
	}
	c.nextTurnOf(t, "Toren")
	mount = turn(c.caio).GetOptions().GetMount()
	if mount.GetDismountRefusal() != 0 || len(mount.GetLanding()) != 5 { // 8 neighbors less the walls on (7,6), (8,6) and (8,8)
		t.Errorf("the dismount options = refusal %v and %d squares, want none and 5", mount.GetDismountRefusal(), len(mount.GetLanding()))
	}
	for _, sq := range mount.GetLanding() {
		if sq.GetCol() == 8 && sq.GetRow() == 8 {
			t.Errorf("the wall is a landing square")
		}
	}
}

// TestMountedCombat_AHiddenMountIsTheMastersLine: the line of a mount that the master hid is the master's alone, even after the
// reveal, and the call does not tell a player the mount is there.
func TestMountedCombat_AHiddenMountIsTheMastersLine(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.fight(t)
	c.steed(t)
	c.hide(t, "Ogro")
	if _, err := c.mountAs(t, c.caio, "Toren", "Ogro"); connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("a player mounting a hidden NPC = %v, want not_found: it is not there for them", err)
	}
	c.mustMount(t, c.master, "Toren", "Ogro")
	if lines := c.linesOfKind(t, c.master, playv1.CombatLogKind_COMBAT_LOG_KIND_MOUNTED); len(lines) != 1 {
		t.Fatalf("the master reads %d mounted lines, want 1", len(lines))
	}
	c.reveal(t, "Ogro")
	for name, u := range map[string]*user{"caio": c.caio, "ana": c.ana} {
		if lines := c.linesOfKind(t, u, playv1.CombatLogKind_COMBAT_LOG_KIND_MOUNTED); len(lines) != 0 {
			t.Errorf("%s reads the mounted line of a mount that was hidden: %v", name, lines)
		}
	}
}

// reveal shows a hidden NPC again.
func (c *cave) reveal(t *testing.T, label string) {
	t.Helper()
	if _, err := c.master.combat.SetCombatantHidden(t.Context(), connect.NewRequest(&playv1.SetCombatantHiddenRequest{
		CampaignId: c.campaignID, EncounterId: c.get(t, c.master).GetId(), CombatantId: c.id(t, label), IdempotencyKey: newKey(), Hidden: false,
	})); err != nil {
		t.Fatalf("SetCombatantHidden(%s, false) error = %v", label, err)
	}
}

// TestMountedCombat_AMountThatCarriesARiderDoesNotMountAnother: Toren rides the Ogro, and the Ogro on its own turn cannot get on a
// Huge mount: the carry is one level, so the chain would leave Toren behind.
func TestMountedCombat_AMountThatCarriesARiderDoesNotMountAnother(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.fight(t)
	c.steed(t)
	c.mustMark(t, "Ogro", "independent")
	c.mustMount(t, c.caio, "Toren", "Ogro")
	c.mustMove(t, c.master, "Goblin 1", 8, 7)
	c.execSQL(t, `UPDATE combatants SET size = 'huge' WHERE id = $1`, c.id(t, "Goblin 1"))
	c.passTo(t, c.get(t, c.master), "Ogro")
	_, err := c.mountAs(t, c.master, "Ogro", "Goblin 1")
	b := wantBlockedBy(t, "a mount that carries a rider getting on another", err, reasonAlreadyRiding)
	if len(b.GetCombatantIds()) != 1 || b.GetCombatantIds()[0] != c.id(t, "Ogro") {
		t.Errorf("combatant_ids = %v, want the mount that carries the rider", b.GetCombatantIds())
	}
	if on := c.rideRowOf(t, "Ogro").MountedOn; on != "" {
		t.Errorf("the Ogro rides %s, want nothing", on)
	}
}
