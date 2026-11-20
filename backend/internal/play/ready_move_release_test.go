package play

import (
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The release of a readied move (SRD 5.1 Ready, "move up to your speed in response to a trigger"): MoveCombatant by the
// readier, or the master for it (release_readied), while its READIED window of a move is open (ready_answer.go, readiedMoveOf). What the
// budget is, what the use spends and closes, what leaves the window open, and who may release.

// readyMoveRequest is the request that has Toren ready a move.
func (a *armed) readyMoveRequest(t *testing.T, e *playv1.Encounter) *playv1.ReadyActionRequest {
	t.Helper()
	return &playv1.ReadyActionRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, "Toren"), IdempotencyKey: newKey(),
		TriggerText: readyTrigger, Kind: playv1.ReadiedKind_READIED_KIND_MOVE,
	}
}

// readyMoveFrom puts Toren on a square of an open lane (a forced move of the master, which provokes nothing), has him ready a move
// and ends his turn: Pensantus acts. walk squares are walked on his turn before he readies, if any.
func (a *armed) readyMoveFrom(t *testing.T, col, row int32, walk ...[2]int32) *playv1.Encounter {
	t.Helper()
	e := a.threeAndAGoblin(t)
	if _, err := a.frozenMove(t, a.master, "Toren", col, row, func(r *playv1.MoveCombatantRequest) { r.Forced = true }); err != nil {
		t.Fatalf("the master puts Toren on (%d,%d): %v", col, row, err)
	}
	for _, sq := range walk {
		if _, err := a.frozenMove(t, a.caio, "Toren", sq[0], sq[1]); err != nil {
			t.Fatalf("Toren's walk on his turn to %v: %v", sq, err)
		}
	}
	a.mustReady(t, a.caio, a.readyMoveRequest(t, e))
	return a.mustEndTurn(t, a.master, e)
}

// untouched says nothing of the readied move was used: Toren's reaction is free, the row waits, its window is open, and the log has no
// readied_fired line.
func (a *armed) untouched(t *testing.T, e *playv1.Encounter, what string) {
	t.Helper()
	if byLabel(t, a.get(t, a.master), "Toren").GetReactionUsed() || a.readiedStatus(t, "Toren") != readiedWaiting || !a.readiedOpen(t, "Toren") || len(a.firedLines(t, a.master, e)) != 0 {
		t.Errorf("after %s: reaction used %v, row %q, window open %v, fired lines %d; want the window as it was",
			what, byLabel(t, a.get(t, a.master), "Toren").GetReactionUsed(), a.readiedStatus(t, "Toren"), a.readiedOpen(t, "Toren"), len(a.firedLines(t, a.master, e)))
	}
}

// The budget is the speed Toren had at Ready, whole: the movement he walked on his own turn before readying is not taken
// from it (move_ft is the fresh speed). The turn's movement stays what it was.
func TestAReadiedMoveBudgetIsTheWholeSpeedAtReadyWhateverTheTurnWalked(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.readyMoveFrom(t, 1, 5, [2]int32{1, 6}) // 5 ft on his turn, then the lane of row 6 to the east
	spent := a.moveRowOf(t, "Toren").UsedDft
	if spent == 0 {
		t.Fatal("Toren's walk spent no movement")
	}
	if got := byLabel(t, a.get(t, a.master), "Toren").GetReadied().GetMoveFt(); got != 30 {
		t.Fatalf("move_ft at Ready after walking %d dft = %d, want his whole speed, 30", spent, got)
	}
	a.mustFire(t, e, "Toren")
	opts, err := a.readiedMoveOptions(t, e)
	if err != nil || opts.GetMovementLeftDft() != 300 {
		t.Fatalf("GetMoveOptions off turn = %v, %v; want 300 dft", opts.GetMovementLeftDft(), err)
	}
	_, err = a.frozenMove(t, a.caio, "Toren", 8, 6) // 35 ft
	wantBlockedBy(t, "a readied move of 35 ft", err, reasonTooFar)
	a.untouched(t, e, "the refusal")
	if _, err := a.frozenMove(t, a.caio, "Toren", 7, 6); err != nil { // 30 ft
		t.Fatalf("the readied move of 30 ft after a walk on the turn: %v", err)
	}
	if got := a.moveRowOf(t, "Toren").UsedDft; got != spent {
		t.Errorf("the turn's movement after the readied move = %d, want %d", got, spent)
	}
}

// The speed is read again when the window is open, and held to move_ft: a speed that fell (an effect, a condition) shrinks the
// budget, one that rose does not widen it. GetMoveOptions and MoveCombatant agree each time, and a refusal leaves the window.
func TestAReadiedMoveBudgetFollowsTheSpeedNowButNeverPastMoveFt(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.readyMoveFrom(t, 1, 5) // the lane of row 5 to the east
	a.mustFire(t, e, "Toren")
	for _, c := range []struct {
		name, set, reset string
		leftDFt          int32
		tooFar           int32 // the first column of the lane the budget does not reach
	}{
		{"a speed that doubled", `speed_ft = 60`, `speed_ft = 30`, 300, 8},
		{"a speed that halved", `effect_speed_pct = 50`, `effect_speed_pct = 0`, 150, 5},
		{"a speed of 15", `speed_ft = 15`, `speed_ft = 30`, 150, 5},
		{"grappled", `conditions = ARRAY['condition:grappled']`, `conditions = '{}'`, 0, 2},
	} {
		a.execSQL(t, `UPDATE combatants SET `+c.set+` WHERE label = 'Toren'`)
		opts, err := a.readiedMoveOptions(t, e)
		if err != nil || opts.GetMovementLeftDft() != c.leftDFt {
			t.Errorf("GetMoveOptions with %s = %v, %v; want %d dft", c.name, opts.GetMovementLeftDft(), err, c.leftDFt)
		}
		_, err = a.frozenMove(t, a.caio, "Toren", c.tooFar, 5)
		wantBlockedBy(t, "a readied move past the budget with "+c.name, err, reasonTooFar)
		a.execSQL(t, `UPDATE combatants SET `+c.reset+` WHERE label = 'Toren'`)
		a.untouched(t, e, c.name)
	}
}

// Difficult terrain costs a readied move what it costs a move on turn: the squares of rubble count double against the budget.
func TestAReadiedMoveWalksDifficultTerrainAtItsCost(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.terrain.paintDifficult(corridor(7, 8)...)
	e := c.fight(t) // Toren at (6,7), east along row 7
	c.mustReady(t, c.caio, c.readyMoveRequest(t, e))
	e = c.mustEndTurn(t, c.master, e)
	c.mustFire(t, e, "Toren")
	_, err := c.move(t, c.caio, "Toren", 11, 7) // 10 + 10 + 5 + 5 + 5 = 35 ft
	wantBlockedBy(t, "a readied move of 35 ft through two squares of rubble", err, reasonTooFar)
	c.untouched(t, e, "the refusal")
	if _, err := c.move(t, c.caio, "Toren", 10, 7); err != nil { // 10 + 10 + 5 + 5 = 30 ft
		t.Fatalf("the readied move of 30 ft through two squares of rubble: %v", err)
	}
	if got := c.moveRowOf(t, "Toren").UsedDft; got != 0 {
		t.Errorf("the turn's movement after the readied move = %d, want 0", got)
	}
}

// The use is the first move: it spends the reaction, fires the row, closes the window and writes the readied_fired line just
// before the move's own line. A partial walk is the whole use: the second call is an ordinary move off turn, and so is
// GetMoveOptions.
func TestAReadiedMoveIsOneUseEvenWhenItStopsShortOfTheBudget(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.readyMoveFrom(t, 1, 5)
	a.mustFire(t, e, "Toren")
	// The master's put of Toren on the lane, on his turn, is one moved line already.
	movedBefore := countKind(logKindsOf(a.log(t, a.master, e)), playv1.CombatLogKind_COMBAT_LOG_KIND_MOVED)
	if _, err := a.frozenMove(t, a.caio, "Toren", 3, 5); err != nil { // 10 ft of 30
		t.Fatalf("the readied move: %v", err)
	}
	toren := byLabel(t, a.get(t, a.caio), "Toren")
	if !toren.GetReactionUsed() || toren.GetReactionKind() != kindReadied || toren.GetReadied() != nil || a.squareOf(t, "Toren") != [2]int32{3, 5} {
		t.Errorf("Toren after the move: reaction %v (%v), readied %v, at %v", toren.GetReactionUsed(), toren.GetReactionKind(), toren.GetReadied(), a.squareOf(t, "Toren"))
	}
	if a.readiedStatus(t, "Toren") != readiedFired || a.readiedOpen(t, "Toren") || len(a.windows(t, a.master)) != 0 {
		t.Errorf("row %q, windows %v; want fired and none", a.readiedStatus(t, "Toren"), a.windows(t, a.master))
	}
	for _, u := range []*user{a.master, a.caio} { // the move is a line, and the readied_fired line comes first
		kinds := logKindsOf(a.log(t, u, e))
		if countKind(kinds, playv1.CombatLogKind_COMBAT_LOG_KIND_READIED_FIRED) != 1 || countKind(kinds, playv1.CombatLogKind_COMBAT_LOG_KIND_MOVED) != movedBefore+1 {
			t.Errorf("the log of %s = %v; want one readied_fired line and %d moved lines", u.id, kinds, movedBefore+1)
		}
	}
	before := a.events(t)
	_, err := a.frozenMove(t, a.caio, "Toren", 4, 5)
	wantBlockedBy(t, "a second move off turn after the use", err, blockedNotYourTurn)
	_, err = a.readiedMoveOptions(t, e)
	wantBlockedBy(t, "GetMoveOptions off turn after the use", err, blockedNotYourTurn)
	if n := a.events(t); n != before || a.squareOf(t, "Toren") != [2]int32{3, 5} {
		t.Errorf("after the second move: events %d (want %d), at %v", n, before, a.squareOf(t, "Toren"))
	}
}

// logKindsOf are the kinds of the lines of the log, in the order it reads them.
func logKindsOf(l *playv1.ListCombatLogResponse) []playv1.CombatLogKind {
	var out []playv1.CombatLogKind
	for _, r := range l.GetRounds() {
		for _, en := range r.GetEntries() {
			out = append(out, en.GetKind())
		}
	}
	return out
}

// countKind counts the lines of a kind.
func countKind(kinds []playv1.CombatLogKind, kind playv1.CombatLogKind) int {
	n := 0
	for _, k := range kinds {
		if k == kind {
			n++
		}
	}
	return n
}

// A window the player lets pass, or the master passes while the player is still on the map, is the end of the readied move:
// the row is ignored, the reaction stays free, no readied_lost line is written (only a spell lets go of its slot), and
// the player's move that comes after is an ordinary one off turn.
func TestAPassedReadiedMoveWindowLeavesNoMoveToMake(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		by   func(a *armed) *user
	}{
		{"the readier's player", func(a *armed) *user { return a.caio }},
		{"the master", func(a *armed) *user { return a.master }},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			a := newArmed(t)
			e := a.readyMove(t)
			w := a.mustFire(t, e, "Toren")
			if err := a.markRolling(t, a.caio, e, w.GetId()); err != nil { // the player is on the map
				t.Fatalf("MarkReactionRolling: %v", err)
			}
			res := a.mustAnswer(t, c.by(a), e, w.GetId(), passAnswer)
			if r := res.GetResult(); r.GetKind() != kindReadied || r.GetUsed() {
				t.Errorf("the pass's result = %v", r)
			}
			toren := byLabel(t, a.get(t, a.caio), "Toren")
			if toren.GetReactionUsed() || toren.GetReadied() != nil || a.readiedStatus(t, "Toren") != readiedIgnored || a.readiedOpen(t, "Toren") {
				t.Errorf("after the pass: reaction used %v, readied %v, row %q, window open %v", toren.GetReactionUsed(), toren.GetReadied(), a.readiedStatus(t, "Toren"), a.readiedOpen(t, "Toren"))
			}
			if lines := a.firedLines(t, a.caio, e); len(lines) != 1 || lines[0].GetReadiedFired().GetStatus() != playv1.ReadiedStatus_READIED_STATUS_IGNORED {
				t.Errorf("the pass's lines = %v", lines)
			}
			if lost := a.readiedLines(t, a.master, e, playv1.CombatLogKind_COMBAT_LOG_KIND_READIED_LOST); len(lost) != 0 {
				t.Errorf("a readied_lost line after the pass of a move: %v", lost)
			}
			before := a.events(t)
			_, err := a.frozenMove(t, a.caio, "Toren", 3, 5)
			wantBlockedBy(t, "Toren's move after the window passed", err, blockedNotYourTurn)
			if n := a.events(t); n != before || a.squareOf(t, "Toren") != [2]int32{3, 3} {
				t.Errorf("after the refused move: events %d (want %d), at %v", n, before, a.squareOf(t, "Toren"))
			}
			a.mustUndo(t, e) // the master's undo of the pass: the move may be made again
			a.untouched(t, e, "the undo of the pass")
			if _, err := a.frozenMove(t, a.caio, "Toren", 3, 5); err != nil {
				t.Errorf("the readied move after the undo of the pass: %v", err)
			}
		})
	}
}

// The reactor's mark of being on the map touches nothing of the move: with the mark on the window stays open and the row
// waits; the move works under the mark and closes the window with it.
func TestTheRollingMarkLeavesAReadiedMoveAsItWas(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.readyMove(t)
	w := a.mustFire(t, e, "Toren")
	rolling := func() bool { return a.windowOf(t, a.master, kindReadied).GetRolling() }
	if err := a.markRolling(t, a.caio, e, w.GetId()); err != nil || !rolling() {
		t.Fatalf("marking the window: error = %v, rolling %v", err, rolling())
	}
	a.untouched(t, e, "the mark")
	if _, err := a.frozenMove(t, a.caio, "Toren", 3, 5); err != nil {
		t.Fatalf("the readied move under the mark: %v", err)
	}
	if a.readiedStatus(t, "Toren") != readiedFired || a.readiedOpen(t, "Toren") || !byLabel(t, a.get(t, a.master), "Toren").GetReactionUsed() {
		t.Errorf("after the move under the mark: row %q, window open %v", a.readiedStatus(t, "Toren"), a.readiedOpen(t, "Toren"))
	}
	wantCode(t, "marking an answered window", a.markRolling(t, a.caio, e, w.GetId()), connect.CodeFailedPrecondition)
}

// A move to the square the readier stands on travels nothing: the player's is refused as any walk to the same square is, and
// the window stays open with the reaction free.
func TestAReadiedMoveToTheSameSquareIsRefusedAndKeepsTheWindow(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.readyMove(t)
	a.mustFire(t, e, "Toren")
	before := a.events(t)
	_, err := a.frozenMove(t, a.caio, "Toren", 3, 3)
	wantRefusal(t, "a readied move to the same square", err, connect.CodeInvalidArgument, "a move needs another square")
	if n := a.events(t); n != before {
		t.Errorf("events after the refusal = %d, want %d", n, before)
	}
	a.untouched(t, e, "the refusal")
}

// forced is the master's "Movimento forçado".
func forced(r *playv1.MoveCombatantRequest) { r.Forced = true }

// The master's drag of a token while a readied move's window is open is no release (only a walk with release_readied is): his
// forced move or jump of the readier, his placing of a token and his walk or forced move of any other token are as they always
// are, spend no reaction and leave the row waiting with its window open.
func TestTheMastersOtherDragsDoNotReleaseAReadiedMove(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.readyMove(t)
	a.mustFire(t, e, "Toren")
	if _, err := a.frozenMove(t, a.master, "Toren", 5, 5, forced); err != nil {
		t.Fatalf("the master's forced move of the readier: %v", err)
	}
	if got := a.squareOf(t, "Toren"); got != [2]int32{5, 5} {
		t.Errorf("Toren after the forced move is at %v, want (5,5)", got)
	}
	a.untouched(t, e, "the forced move of the readier")
	if _, err := a.frozenMove(t, a.master, "Toren", 5, 6, func(r *playv1.MoveCombatantRequest) { r.Jump = playv1.JumpKind_JUMP_KIND_LONG }); err != nil {
		t.Fatalf("the master's long jump of the readier: %v", err)
	}
	a.untouched(t, e, "the master's jump of the readier")
	_, err := a.frozenMove(t, a.master, "Toren", 6, 6, func(r *playv1.MoveCombatantRequest) { r.Place = true })
	wantRefusal(t, "the master's placing of a token that has a square", err, connect.CodeInvalidArgument, "the combatant is already on the map")
	a.untouched(t, e, "the refused placing of the readier")

	if _, err := a.frozenMove(t, a.master, "Goblin", 6, 3, forced); err != nil {
		t.Fatalf("the master's forced move of another token: %v", err)
	}
	if _, err := a.frozenMove(t, a.master, "Goblin", 7, 3); err != nil {
		t.Fatalf("the master's walk of another token: %v", err)
	}
	a.execSQL(t, `UPDATE combatants SET grid_col = NULL, grid_row = NULL WHERE label = 'Brisa'`)
	if _, err := a.frozenMove(t, a.master, "Brisa", 0, 0, func(r *playv1.MoveCombatantRequest) { r.Place = true }); err != nil {
		t.Fatalf("the master's placing of another token: %v", err)
	}
	a.untouched(t, e, "the master's drags of other tokens")

	if _, err := a.frozenMove(t, a.caio, "Toren", 5, 9); err != nil { // from where the master left him, (5,6): 15 ft
		t.Fatalf("the readied move after the master's drags: %v", err)
	}
	if a.readiedStatus(t, "Toren") != readiedFired || !byLabel(t, a.get(t, a.master), "Toren").GetReactionUsed() {
		t.Errorf("after the readier's move: row %q", a.readiedStatus(t, "Toren"))
	}
}

// Who may release: the readier's player and the master (with release_readied). Another player is refused as for any token that is not theirs
// (permission_denied, before the window is looked at), and nothing is spent.
func TestOnlyTheReadiersPlayerOrTheMasterReleasesAReadiedMove(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.readyMove(t)
	a.mustFire(t, e, "Toren")
	before := a.events(t)
	for _, u := range []*user{a.ana, a.bia} {
		_, err := a.frozenMove(t, u, "Toren", 3, 5)
		wantRefusal(t, u.id+"'s move of Toren in his readied window", err, connect.CodePermissionDenied, "only the combatant's player or the master may do this")
	}
	if n := a.events(t); n != before || a.squareOf(t, "Toren") != [2]int32{3, 3} {
		t.Errorf("after the refusals: events %d (want %d), at %v", n, before, a.squareOf(t, "Toren"))
	}
	a.untouched(t, e, "the refusals")
}
