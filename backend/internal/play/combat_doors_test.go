package play

import (
	"slices"
	"strings"
	"testing"

	"connectrpc.com/connect"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// Doors in a combat (MR-010, RN-26, Etapa 10, slice 10.6c). The fixture is the
// fog cave with the real maps service behind it, the doors painted through
// PaintMapCells. Toren (the player of Caio) is first in the order, at (6, 7),
// with 30 ft of movement; the row 7 corridor east of him is open floor, so a door
// painted on it is a door in a doorway. The map has no fog unless a test turns it
// on.

const doorLayer = mapsv1.MapLayer_MAP_LAYER_DOORS

var (
	reasonDoorLocked = playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_DOOR_LOCKED
	doorSquare       = &mapsv1.MapSquare{Col: 8, Row: 7}
)

// newDoorCave is the cave with the fog off, the fight started and a door at
// (8, 7) in the state given.
func newDoorCave(t *testing.T, door mapsv1.DoorState) *fogCave {
	t.Helper()
	f := newFogCave(t)
	if _, err := f.h.pool.Exec(t.Context(), `UPDATE maps SET fog_enabled = false WHERE id = $1`, f.mapID); err != nil {
		t.Fatalf("turn the fog off: %v", err)
	}
	f.fight(t)
	if door != mapsv1.DoorState_DOOR_STATE_UNSPECIFIED {
		f.paint(t, doorLayer, int32(door), doorSquare)
	}
	return f
}

// doorAt is the door at the square as the maps module keeps it (the truth).
func (f *fogCave) doorAt(t *testing.T, col, row int) grid.Door {
	t.Helper()
	terrain, err := f.msvc.Terrain(t.Context(), nil, f.campaignID, f.mapID)
	if err != nil {
		t.Fatalf("Terrain() error = %v", err)
	}
	return terrain.Doors.Get(col, row)
}

// layersRevision is the master's layers_revision of the map.
func (f *fogCave) layersRevision(t *testing.T) int32 {
	t.Helper()
	res, err := f.mapsAs(f.master).GetMapLayers(t.Context(), connect.NewRequest(&mapsv1.GetMapLayersRequest{CampaignId: f.campaignID, MapId: f.mapID}))
	if err != nil {
		t.Fatalf("GetMapLayers() error = %v", err)
	}
	return res.Msg.GetLayersRevision()
}

// doorLines are the DOOR_OPENED entries of a combat log.
func doorLines(log *playv1.ListCombatLogResponse) []*playv1.CombatLogEntry {
	var out []*playv1.CombatLogEntry
	for _, r := range log.GetRounds() {
		for _, e := range r.GetEntries() {
			if e.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_DOOR_OPENED {
				out = append(out, e)
			}
		}
	}
	return out
}

func reachable(o *playv1.GetMoveOptionsResponse, col, row int32) bool {
	return slices.ContainsFunc(o.GetReachable(), func(r *playv1.ReachableSquare) bool { return r.GetCol() == col && r.GetRow() == row })
}

// RN-26: a move whose line enters a closed door opens it, in the same
// transaction: the layer, its revision, the log line for whoever saw the mover
// and the hints.
func TestMR010_AMoveOpensAClosedDoor(t *testing.T) {
	t.Parallel()
	f := newDoorCave(t, mapsv1.DoorState_DOOR_STATE_CLOSED)
	rev := f.layersRevision(t)
	streams := f.watchAll(t)

	res, err := f.moveResponse(t, f.caio, "Toren", 10, 7)
	if err != nil {
		t.Fatalf("MoveCombatant(Toren to 10,7) error = %v", err)
	}
	if res.GetStoppedEarly() || res.GetLockedDoor() {
		t.Errorf("the move through a closed door stopped early (%v) or at a lock (%v)", res.GetStoppedEarly(), res.GetLockedDoor())
	}
	toren := f.who(t, f.caio, "Toren")
	if toren.GetCol() != 10 || toren.GetRow() != 7 || toren.GetMovementUsedDft() != 200 {
		t.Errorf("Toren is at (%d, %d) having used %d dft; want (10, 7) and 200: a door costs nothing", toren.GetCol(), toren.GetRow(), toren.GetMovementUsedDft())
	}
	if got := f.doorAt(t, 8, 7); got != grid.DoorOpen {
		t.Errorf("the door is %d after the move, want open", got)
	}
	if got := f.layersRevision(t); got != rev+1 {
		t.Errorf("layers_revision = %d, want %d", got, rev+1)
	}
	// The log, to the master and to the players who saw the mover (everyone, here).
	for name, u := range map[string]*user{"master": f.master, "Ana": f.ana, "Caio": f.caio} {
		lines := doorLines(f.log(t, u, f.get(t, u)))
		if len(lines) != 1 || lines[0].GetActorLabel() != "Toren" || lines[0].GetDoor().GetCol() != 8 || lines[0].GetDoor().GetRow() != 7 {
			t.Errorf("%s's log has the door lines %v, want one: Toren opened (8, 7)", name, lines)
		}
	}
	// The hints: the log, and the map (the layers changed). The map's hint is the
	// maps module's, at most one a second a map, so it may come a moment later.
	for name, w := range map[string]*watcher{"master": streams.master, "Ana": streams.ana, "Caio": streams.caio} {
		var logHint, mapHint bool
		for !logHint || !mapHint {
			ev := w.nextChange(t)
			logHint = logHint || ev.GetCombatLogChanged() != nil
			mapHint = mapHint || ev.GetMapChanged().GetMapId() == f.mapID
		}
		_ = name
	}

	// Walking through the open door again opens nothing.
	f.mustMove(t, f.caio, "Toren", 11, 7)
	if got := f.layersRevision(t); got != rev+1 {
		t.Errorf("layers_revision = %d after walking through an open door, want %d", got, rev+1)
	}
	if lines := doorLines(f.log(t, f.master, f.get(t, f.master))); len(lines) != 1 {
		t.Errorf("walking through an open door wrote a door line: %d in all", len(lines))
	}
}

// RN-26: a door opened stays opened. The master's undo of the move puts the mover
// back and leaves the door open, the line of the log stays, and the undo goes on
// past it.
func TestMR010_AnUndoOfTheMoveLeavesTheDoorOpen(t *testing.T) {
	t.Parallel()
	f := newDoorCave(t, mapsv1.DoorState_DOOR_STATE_CLOSED)
	f.mustMove(t, f.caio, "Toren", 10, 7)
	f.undoLast(t)
	toren := f.who(t, f.caio, "Toren")
	if toren.GetCol() != 6 || toren.GetRow() != 7 || toren.GetMovementUsedDft() != 0 {
		t.Errorf("after the undo Toren is at (%d, %d) with %d used; want (6, 7) and 0", toren.GetCol(), toren.GetRow(), toren.GetMovementUsedDft())
	}
	if got := f.doorAt(t, 8, 7); got != grid.DoorOpen {
		t.Errorf("the door is %d after the undo, want it still open: a door opened stays opened", got)
	}
	log := f.log(t, f.master, f.get(t, f.master))
	if lines := doorLines(log); len(lines) != 1 {
		t.Errorf("the master's log has %d door lines after the undo, want the one that stays", len(lines))
	}
	if log.GetUndoableEventId() != "" {
		t.Errorf("the log offers to undo %s, but the only action was undone", log.GetUndoableEventId())
	}
	// The door line never closes the chain: an action before the move is still
	// the one to undo (here, a forced move of the master's, written first).
	g := newDoorCave(t, mapsv1.DoorState_DOOR_STATE_CLOSED)
	g.mustMove(t, g.master, "Goblin 1", 17, 5)
	g.mustMove(t, g.caio, "Toren", 10, 7)
	g.undoLast(t) // the move through the door
	g.undoLast(t) // and the one before it, past the door's line
	if got := g.who(t, g.master, "Goblin 1"); got.GetCol() != 18 {
		t.Errorf("the second undo left Goblin 1 at column %d, want 18: the door line closed the undo chain", got.GetCol())
	}
}

// RN-26: a locked door stops the move at the last square before it; the mover
// pays for what it walked, and the answer says why. When the first step is the
// locked door, the move is refused with DOOR_LOCKED and nothing is spent. A player
// never knows a door is locked before trying it.
func TestMR010_ALockedDoorStopsTheMove(t *testing.T) {
	t.Parallel()
	f := newDoorCave(t, mapsv1.DoorState_DOOR_STATE_LOCKED)
	rev := f.layersRevision(t)

	// The player reads it as closed, and plans through it.
	layers, err := f.mapsAs(f.caio).GetMapLayers(t.Context(), connect.NewRequest(&mapsv1.GetMapLayersRequest{CampaignId: f.campaignID, MapId: f.mapID}))
	if err != nil {
		t.Fatalf("GetMapLayers() error = %v", err)
	}
	if d, err := grid.DecodeDoorLayer(caveGridSize, layers.Msg.GetDoors()); err != nil || d.Get(8, 7) != grid.DoorClosed {
		t.Errorf("a player reads the locked door as %v (%v), want closed", d.Get(8, 7), err)
	}
	if strings.Contains(asJSON(t, f.get(t, f.caio)), "locked") {
		t.Error("the encounter a player reads says a door is locked")
	}
	opts, err := f.options(t, f.caio, "Toren")
	if err != nil || !reachable(opts, 9, 7) {
		t.Fatalf("a player's reach through a door that looks closed: %v, %v; want (9, 7) in it", opts.GetReachable(), err)
	}
	if masterOpts, err := f.options(t, f.master, "Toren"); err != nil || reachable(masterOpts, 9, 7) {
		t.Errorf("the master's reach through the locked door: reaches (9, 7) = %v (%v), want no: the master knows", reachable(masterOpts, 9, 7), err)
	}

	res, err := f.moveResponse(t, f.caio, "Toren", 10, 7)
	if err != nil {
		t.Fatalf("MoveCombatant(Toren to 10,7) error = %v", err)
	}
	if !res.GetLockedDoor() || res.GetStoppedEarly() {
		t.Errorf("locked_door = %v, stopped_early = %v; want the lock named, not a hidden creature", res.GetLockedDoor(), res.GetStoppedEarly())
	}
	toren := f.who(t, f.caio, "Toren")
	if toren.GetCol() != 7 || toren.GetRow() != 7 || toren.GetMovementUsedDft() != 50 {
		t.Errorf("Toren is at (%d, %d) having used %d dft; want (7, 7) and 50: the real cost of the one step", toren.GetCol(), toren.GetRow(), toren.GetMovementUsedDft())
	}
	if f.doorAt(t, 8, 7) != grid.DoorLocked || f.layersRevision(t) != rev {
		t.Error("the locked door was opened or the layers' revision moved")
	}
	if lines := doorLines(f.log(t, f.master, f.get(t, f.master))); len(lines) != 0 {
		t.Errorf("a locked door that did not open wrote %d door lines", len(lines))
	}

	// Right in front of it: a refusal, nothing spent.
	_, err = f.move(t, f.caio, "Toren", 10, 7)
	b := wantEncounterBlocked(t, err, reasonDoorLocked)
	_ = b
	if got := f.who(t, f.caio, "Toren"); got.GetMovementUsedDft() != 50 || got.GetCol() != 7 {
		t.Errorf("a refused move spent movement: at column %d, %d used", got.GetCol(), got.GetMovementUsedDft())
	}
	// The master opens it (a key, a spell): painting it open, or closed, lets Toren through.
	f.paint(t, doorLayer, int32(mapsv1.DoorState_DOOR_STATE_CLOSED), doorSquare)
	f.mustMove(t, f.caio, "Toren", 10, 7)
	if f.doorAt(t, 8, 7) != grid.DoorOpen {
		t.Errorf("the door the master unlocked did not open by walking: %d", f.doorAt(t, 8, 7))
	}
}

// Barred and secret doors block movement, and a player plans on them as they
// look: bars are bars (the move is refused, as for a wall), a secret door is a
// wall, and no refusal names a secret door or a lock.
func TestMR010_BarredAndSecretDoorsBlockTheMove(t *testing.T) {
	t.Parallel()
	for name, door := range map[string]mapsv1.DoorState{"barred": mapsv1.DoorState_DOOR_STATE_BARRED, "secret": mapsv1.DoorState_DOOR_STATE_SECRET} {
		f := newDoorCave(t, door)
		_, err := f.move(t, f.caio, "Toren", 10, 7)
		wantEncounterBlocked(t, err, reasonMoveBlocked)
		if got := f.who(t, f.caio, "Toren"); got.GetCol() != 6 || got.GetMovementUsedDft() != 0 {
			t.Errorf("%s: a refused move changed Toren: column %d, %d used", name, got.GetCol(), got.GetMovementUsedDft())
		}
		if opts, err := f.options(t, f.caio, "Toren"); err != nil || reachable(opts, 9, 7) {
			t.Errorf("%s: a player reaches the square behind the door: %v (%v)", name, reachable(opts, 9, 7), err)
		}
		if opts, err := f.options(t, f.master, "Toren"); err != nil || reachable(opts, 9, 7) {
			t.Errorf("%s: the master reaches the square behind the door: %v (%v)", name, reachable(opts, 9, 7), err)
		}
		if f.doorAt(t, 8, 7) != grid.Door(door) { //nolint:gosec // G115: a DoorState is 0 to 5
			t.Errorf("%s: the door changed to %d", name, f.doorAt(t, 8, 7))
		}
	}
}

// A closed door is passable for the planner: the squares behind it are in the
// reach, of the player and of the master. (A jump cannot clear a closed door:
// package grid's tests.)
func TestMR010_ClosedDoorsInTheReach(t *testing.T) {
	t.Parallel()
	f := newDoorCave(t, mapsv1.DoorState_DOOR_STATE_CLOSED)
	for _, u := range []*user{f.caio, f.master} {
		if opts, err := f.options(t, u, "Toren"); err != nil || !reachable(opts, 8, 7) || !reachable(opts, 10, 7) {
			t.Errorf("the reach through a closed door: %v (%v), want the door and what is behind it", opts.GetReachable(), err)
		}
	}
	if f.doorAt(t, 8, 7) != grid.DoorClosed {
		t.Error("asking for the reach opened the door")
	}
}

// With the fog, a player learns of a lock only on a door their character sees: in
// the dark, the move is cut short like any other thing in the way, and nothing says
// it was a door.
func TestRN10_FogAPlayerLearnsOfALockOnlyOnADoorTheySee(t *testing.T) {
	t.Parallel()
	f := newFogCave(t)
	f.torch(t, false) // Toren sees nothing but his own square
	f.fight(t)
	f.paint(t, doorLayer, int32(mapsv1.DoorState_DOOR_STATE_LOCKED), doorSquare)

	res, err := f.moveResponse(t, f.caio, "Toren", 10, 7)
	if err != nil {
		t.Fatalf("MoveCombatant(Toren to 10,7) error = %v", err)
	}
	if res.GetLockedDoor() || !res.GetStoppedEarly() {
		t.Errorf("a locked door in the dark: locked_door = %v, stopped_early = %v; want only stopped_early", res.GetLockedDoor(), res.GetStoppedEarly())
	}
	if got := f.who(t, f.caio, "Toren"); got.GetCol() != 7 || got.GetMovementUsedDft() != 50 {
		t.Errorf("Toren is at column %d with %d used, want 7 and 50", got.GetCol(), got.GetMovementUsedDft())
	}
	// Next to it, the door is seen (it is a wall to the light, so the square he is
	// on touches it): now he knows, and the refusal names it.
	_, err = f.move(t, f.caio, "Toren", 10, 7)
	wantEncounterBlocked(t, err, reasonDoorLocked)
	f.noLeak(t, "Toren's encounter", asJSON(t, f.get(t, f.caio)), "Goblin 1")
}

// RN-26 with the fog: a door the mover opens lets sight through, and the players
// who see the corridor are told (vision_changed) and see what is behind it.
func TestMR010_FogOpeningADoorShowsWhatIsBehindIt(t *testing.T) {
	t.Parallel()
	f := newFogCave(t)
	f.fight(t)
	f.stage(t) // Goblin 2 at (8, 8), in Toren's light; the Capitão at (16, 7)
	// A closed door across the corridor behind Goblin 2: both squares of its width.
	f.paint(t, doorLayer, int32(mapsv1.DoorState_DOOR_STATE_CLOSED), &mapsv1.MapSquare{Col: 10, Row: 7}, &mapsv1.MapSquare{Col: 10, Row: 8})
	if got := npcLabels(f.get(t, f.ana)); slices.Contains(got, "Capitão Goblin") {
		t.Fatalf("Pensantus's player sees the Capitão through a closed door: %v", got)
	}
	streams := f.watchAll(t)
	f.mustMove(t, f.caio, "Toren", 9, 7) // not through the door: nothing opens
	if f.doorAt(t, 10, 7) != grid.DoorClosed {
		t.Fatal("a move that did not enter the door opened it")
	}
	if _, err := f.moveResponse(t, f.caio, "Toren", 11, 7); err != nil {
		t.Fatalf("MoveCombatant(Toren to 11,7) error = %v", err)
	}
	if f.doorAt(t, 10, 7) != grid.DoorOpen {
		t.Errorf("the door at (10, 7) is %d, want open", f.doorAt(t, 10, 7))
	}
	for vision := false; !vision; {
		vision = streams.ana.nextChange(t).GetVisionChanged() != nil
	}
	// The other half of the doorway is still shut, and Pensantus's line to the
	// Capitão runs along it: the master opens it too.
	f.paint(t, doorLayer, int32(mapsv1.DoorState_DOOR_STATE_OPEN), &mapsv1.MapSquare{Col: 10, Row: 8})
	if got := npcLabels(f.get(t, f.ana)); !slices.Contains(got, "Capitão Goblin") {
		t.Errorf("Pensantus's player does not see the Capitão after the door opened: %v", got)
	}
}

// RN-10: on a fog map the line "Toren abriu a porta" goes to the players whose
// character saw (or remembers) the door when it happened, and to the mover's
// player; the others get neither the line nor the square. Here Toren carries no
// torch: Pensantus sees along the corridor with her darkvision, Brisa sees
// nothing but her own square.
func TestRN10_FogTheDoorLineGoesOnlyToWhoKnewTheDoor(t *testing.T) {
	t.Parallel()
	f := newFogCave(t)
	f.torch(t, false)
	f.fight(t)
	f.paint(t, doorLayer, int32(mapsv1.DoorState_DOOR_STATE_CLOSED), doorSquare)
	if _, err := f.moveResponse(t, f.caio, "Toren", 10, 7); err != nil {
		t.Fatalf("MoveCombatant(Toren to 10,7) error = %v", err)
	}
	if f.doorAt(t, 8, 7) != grid.DoorOpen {
		t.Fatal("the door did not open")
	}
	for name, c := range map[string]struct {
		u    *user
		want int
	}{"the master": {f.master, 1}, "Toren's player": {f.caio, 1}, "Pensantus's player": {f.ana, 1}, "Brisa's player": {f.bia, 0}} {
		log := f.log(t, c.u, f.get(t, c.u))
		if got := len(doorLines(log)); got != c.want {
			t.Errorf("%s has %d door lines, want %d", name, got, c.want)
		}
		if c.want == 0 && strings.Contains(asJSON(t, log), `"door"`) {
			t.Errorf("%s's log carries a door: %s", name, asJSON(t, log))
		}
	}
	// The master's copy says whether it is hidden from the players, not who got it.
	if lines := doorLines(f.log(t, f.master, f.get(t, f.master))); len(lines) != 1 || lines[0].GetHidden() {
		t.Errorf("the master's door line = %v, want one that some player has", lines)
	}
}

// RN-26: the doors do what they do for everyone, the master's moves of NPCs
// included: a closed door on the line opens, a locked one stops the move before
// it, and a forced move (a teleport, a token put right) walks nothing. The master
// unlocks by painting.
func TestMR010_TheMastersMoveOpensDoorsToo(t *testing.T) {
	t.Parallel()
	f := newDoorCave(t, mapsv1.DoorState_DOOR_STATE_UNSPECIFIED)
	far := &mapsv1.MapSquare{Col: 16, Row: 7} // on Goblin 2's way west, from (20, 7)
	f.paint(t, doorLayer, int32(mapsv1.DoorState_DOOR_STATE_CLOSED), far)
	if res, err := f.moveResponse(t, f.master, "Goblin 2", 12, 7); err != nil || res.GetLockedDoor() || res.GetStoppedEarly() {
		t.Fatalf("the master's move through a closed door = %v, %v; want it to go through", res, err)
	}
	if got := f.who(t, f.master, "Goblin 2"); got.GetCol() != 12 || f.doorAt(t, 16, 7) != grid.DoorOpen {
		t.Errorf("Goblin 2 is at column %d, the door is %d; want 12 and open", got.GetCol(), f.doorAt(t, 16, 7))
	}
	if lines := doorLines(f.log(t, f.master, f.get(t, f.master))); len(lines) != 1 || lines[0].GetActorLabel() != "Goblin 2" {
		t.Errorf("the master's log has %v, want the door Goblin 2 opened", lines)
	}

	f.paint(t, doorLayer, int32(mapsv1.DoorState_DOOR_STATE_LOCKED), far)
	f.mustMove(t, f.master, "Goblin 2", 20, 7) // a forced move walks nothing
	res, err := f.moveResponse(t, f.master, "Goblin 2", 12, 7)
	if err != nil || !res.GetLockedDoor() || !res.GetStoppedEarly() {
		t.Fatalf("the master's move into a locked door = %v, %v; want locked_door and stopped_early", res, err)
	}
	if got := f.who(t, f.master, "Goblin 2"); got.GetCol() != 17 || f.doorAt(t, 16, 7) != grid.DoorLocked {
		t.Errorf("Goblin 2 is at column %d, the door %d; want 17 and still locked", got.GetCol(), f.doorAt(t, 16, 7))
	}
	// Right in front of it, a refusal; the master unlocks by painting.
	_, err = f.move(t, f.master, "Goblin 2", 12, 7)
	wantEncounterBlocked(t, err, reasonDoorLocked)
	f.paint(t, doorLayer, int32(mapsv1.DoorState_DOOR_STATE_CLOSED), far)
	if _, err := f.moveResponse(t, f.master, "Goblin 2", 12, 7); err != nil || f.who(t, f.master, "Goblin 2").GetCol() != 12 {
		t.Errorf("after unlocking, the master's move = %v", err)
	}
}

// An idempotent replay of a move that a locked door stopped says so again.
func TestMR010_AReplayKeepsTheLockedDoor(t *testing.T) {
	t.Parallel()
	f := newDoorCave(t, mapsv1.DoorState_DOOR_STATE_LOCKED)
	req := &playv1.MoveCombatantRequest{
		CampaignId: f.campaignID, EncounterId: f.get(t, f.master).GetId(), CombatantId: f.id(t, "Toren"), IdempotencyKey: newKey(), Col: 10, Row: 7,
	}
	for i := range 2 {
		res, err := f.caio.combat.MoveCombatant(t.Context(), connect.NewRequest(req))
		if err != nil || !res.Msg.GetLockedDoor() {
			t.Fatalf("attempt %d: %v, %v; want locked_door each time", i+1, res, err)
		}
	}
}

// MR-025: on a map calibrated to 2 (the cave's 24 x 16 drawn squares are 48 x 32 squares
// of 1,5 m) the combat copies the rules' grid, and a door is the whole drawn square:
// a move across two of its squares opens the 2 x 2 block, with one line in the log.
func TestMR025_ACombatOnACalibratedMapWalksThroughADoorBlock(t *testing.T) {
	t.Parallel()
	f := newFogCave(t)
	if _, err := f.h.pool.Exec(t.Context(), `UPDATE maps SET fog_enabled = false WHERE id = $1`, f.mapID); err != nil {
		t.Fatalf("turn the fog off: %v", err)
	}
	if _, err := f.mapsAs(f.master).SetMapGrid(t.Context(), connect.NewRequest(&mapsv1.SetMapGridRequest{
		CampaignId: f.campaignID, MapId: f.mapID, Columns: 24, SquareFactor: 2,
	})); err != nil {
		t.Fatalf("SetMapGrid(24, factor 2) error = %v", err)
	}
	f.fight(t)
	if e := f.get(t, f.master); e.GetGridColumns() != 48 || e.GetGridRows() != 32 {
		t.Fatalf("the combat copied a grid of %d x %d, want 48 x 32", e.GetGridColumns(), e.GetGridRows())
	}
	// The fixture stands the combatants on the squares of the 24 x 16 cave: put Toren on
	// open floor of the 48 x 32 grid first (the master moves anyone).
	f.mustMove(t, f.master, "Toren", 14, 14)
	toren := f.who(t, f.caio, "Toren")
	col, row := toren.GetCol(), toren.GetRow()
	// A door of the drawing 2 squares of the drawing east of Toren's: its block, 2 x 2 squares of the rules.
	bc := (col/2 + 2) * 2
	br := row / 2 * 2
	f.paint(t, doorLayer, int32(mapsv1.DoorState_DOOR_STATE_CLOSED), &mapsv1.MapSquare{Col: bc + 1, Row: row})
	for _, sq := range [][2]int32{{bc, br}, {bc + 1, br}, {bc, br + 1}, {bc + 1, br + 1}} {
		if got := f.doorAt(t, int(sq[0]), int(sq[1])); got != grid.DoorClosed {
			t.Fatalf("door square (%d, %d) is %d, want closed: painting one square paints the block", sq[0], sq[1], got)
		}
	}
	rev := f.layersRevision(t)
	res, err := f.moveResponse(t, f.caio, "Toren", bc+2, row)
	if err != nil || res.GetStoppedEarly() || res.GetLockedDoor() {
		t.Fatalf("the move through the door block: %v, %+v", err, res)
	}
	for _, sq := range [][2]int32{{bc, br}, {bc + 1, br}, {bc, br + 1}, {bc + 1, br + 1}} {
		if got := f.doorAt(t, int(sq[0]), int(sq[1])); got != grid.DoorOpen {
			t.Errorf("door square (%d, %d) is %d after the move, want open", sq[0], sq[1], got)
		}
	}
	if got := f.layersRevision(t); got != rev+1 {
		t.Errorf("layers_revision = %d, want %d: one bump", got, rev+1)
	}
	if lines := doorLines(f.log(t, f.master, f.get(t, f.master))); len(lines) != 1 {
		t.Errorf("the log has %d door lines, want one for the door", len(lines))
	}
}

// RN-10: on a fog map, the line "Goblin 2 abriu a porta" names an NPC, so a player
// who remembers the door but saw the NPC neither where it came from nor where it
// stands does not get it. Pensantus saw the door along the corridor, then went to
// the room below; Goblin 2 walks through it from the dark. The master's log is the
// positive control.
func TestRN10_FogTheDoorLineOfAnNPCNeedsSeeingTheNPC(t *testing.T) {
	t.Parallel()
	f := newFogCave(t)
	f.torch(t, false)
	f.fight(t)
	f.paint(t, doorLayer, int32(mapsv1.DoorState_DOOR_STATE_CLOSED), &mapsv1.MapSquare{Col: 16, Row: 7})
	f.seesSquare(t, f.ana, 15, 7) // Pensantus reads the corridor: the door stays in what she knows
	f.mustMove(t, f.master, "Pensantus", 10, 13)
	for _, col := range []int{12, 20} {
		if f.seesSquare(t, f.ana, col, 7) {
			t.Fatalf("Pensantus still sees column %d from the room", col)
		}
	}
	if _, err := f.moveResponse(t, f.master, "Goblin 2", 12, 7); err != nil {
		t.Fatalf("MoveCombatant(Goblin 2 to 12,7) error = %v", err)
	}
	if lines := doorLines(f.log(t, f.master, f.get(t, f.master))); len(lines) != 1 || lines[0].GetActorLabel() != "Goblin 2" {
		t.Fatalf("the master's log has %v, want the door Goblin 2 opened", lines)
	}
	log := f.log(t, f.ana, f.get(t, f.ana))
	if got := len(doorLines(log)); got != 0 {
		t.Errorf("Pensantus's player has %d door lines, want 0 (she saw no one open it)", got)
	}
	f.noLeak(t, "Pensantus's log", asJSON(t, log), "Goblin 2")
}
