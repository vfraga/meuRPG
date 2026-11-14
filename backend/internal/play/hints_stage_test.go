package play

import (
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The stage and the table screen (#735 part 2, 4d) go through Service.tx: the session's number moves once with the write, the hints
// carry it, the players hear a change of the stage only while they are in the scene and only when they may read it, and the table
// screen is the master's alone.

const kTableScreen = "*playv1.WatchGameSessionResponse_TableScreenChanged_"

func (a *armed) stageCall(t *testing.T, character string, hidden bool) func() error {
	t.Helper()
	return func() error {
		_, err := a.master.play.PutOnStage(t.Context(), connect.NewRequest(&playv1.PutOnStageRequest{CampaignId: a.campaignID, CharacterId: character, Hidden: hidden}))
		return err
	}
}

func (a *armed) offStageCall(t *testing.T, character string) func() error {
	t.Helper()
	return func() error {
		_, err := a.master.play.TakeOffStage(t.Context(), connect.NewRequest(&playv1.TakeOffStageRequest{CampaignId: a.campaignID, CharacterId: character}))
		return err
	}
}

func (a *armed) stageHints(t *testing.T, quiet ...string) *hintRig {
	t.Helper()
	return newHintRig(t, a.h, a.campaignID, a.master, map[string]*user{"Ana": a.ana, "Caio": a.caio, "Bia": a.bia}, quiet...)
}

func TestPutOnStageHints(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	point, _ := a.h.newScene(a.mapID, "A clareira", true, 1)
	a.openScene(t, point)
	g := a.stageHints(t)
	heard := g.first(a.stageCall(t, a.goblin.GetId(), false), true)
	heardKinds(t, heard, map[string][]string{"master": {kStageChanged}, "Ana": {kStageChanged}, "Caio": {kStageChanged}, "Bia": {kStageChanged}})
}

func TestPutOnStageOfASceneForSomePlayersTellsOnlyThem(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	point, _ := a.h.newScene(a.mapID, "A clareira", true, 1)
	do(t, func() error {
		_, err := a.master.play.OpenScene(t.Context(), connect.NewRequest(&playv1.OpenSceneRequest{CampaignId: a.campaignID, PointId: point, Audience: audienceOf(a.ana)}))
		return err
	})
	g := a.stageHints(t, "Caio", "Bia")
	heard := g.first(a.stageCall(t, a.goblin.GetId(), false), true)
	heardKinds(t, heard, map[string][]string{"master": {kStageChanged}, "Ana": {kStageChanged}})
}

// An NPC put on the stage hidden is the master's alone: no player hears it, though the number moves.
func TestPutOnStageHiddenTellsTheMasterAlone(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	point, _ := a.h.newScene(a.mapID, "A clareira", true, 1)
	a.openScene(t, point)
	g := a.stageHints(t, "Ana", "Caio", "Bia")
	heard := g.first(a.stageCall(t, a.goblin.GetId(), true), true)
	heardKinds(t, heard, map[string][]string{"master": {kStageChanged}})
}

func TestTakeOffStageHints(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	point, _ := a.h.newScene(a.mapID, "A clareira", true, 1)
	a.openScene(t, point)
	do(t, a.stageCall(t, a.goblin.GetId(), false))
	g := a.stageHints(t)
	heard := g.first(a.offStageCall(t, a.goblin.GetId()), true)
	heardKinds(t, heard, map[string][]string{"master": {kStageChanged}, "Ana": {kStageChanged}, "Caio": {kStageChanged}, "Bia": {kStageChanged}})
}

func TestPutOnStageAgainMovesNothing(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	point, _ := a.h.newScene(a.mapID, "A clareira", true, 1)
	a.openScene(t, point)
	do(t, a.stageCall(t, a.goblin.GetId(), false))
	g := a.stageHints(t)
	before := g.seq()
	do(t, a.stageCall(t, a.goblin.GetId(), false))
	if got := g.seq(); got != before {
		t.Errorf("hint_seq = %d, want %d: the NPC was on the stage already", got, before)
	}
	for name, evs := range g.drain() {
		if len(evs) != 0 {
			t.Errorf("%s heard %v of a change that changed nothing", name, eventKinds(evs))
		}
	}
}

func TestPutOnStageRefusedTellsNothing(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	g := a.stageHints(t)
	g.refused(a.stageCall(t, a.goblin.GetId(), false)) // no scene is open
}

func TestPutOnStageWithNoOpenSessionRefusesAsBefore(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	a.master.end(t, a.master.liveSession(t, a.campaignID).GetGameSession())
	if err := a.stageCall(t, a.goblin.GetId(), false)(); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("PutOnStage(no open session) error = %v, want failed_precondition", err)
	}
}

func (a *armed) tableScreenCall(_ *testing.T, revision int32) func() error {
	return func() error {
		_, err := a.setScreen(a.master, revision, false, kMap)
		return err
	}
}

func TestSetTableScreenHints(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	g := a.stageHints(t, "Ana", "Caio", "Bia")
	heard := g.first(a.tableScreenCall(t, 0), true)
	heardKinds(t, heard, map[string][]string{"master": {kTableScreen}})
}

func TestSetTableScreenStaleTellsNothing(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	do(t, a.tableScreenCall(t, 0))
	g := a.stageHints(t)
	g.refused(a.tableScreenCall(t, 0)) // the revision is 1 now
}

func TestSetTableScreenWithNoOpenSessionRefusesAsBefore(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	a.master.end(t, a.master.liveSession(t, a.campaignID).GetGameSession())
	if err := a.tableScreenCall(t, 0)(); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("SetTableScreen(no open session) error = %v, want failed_precondition", err)
	}
}
