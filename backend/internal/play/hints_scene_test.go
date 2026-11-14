package play

import (
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The writers of the scene (OpenScene, CloseScene, SetSceneAudience, SetScenePlayersMayJoin, JoinScene, GrantSceneAttempt and
// RollSceneCheck; #735 part 2, 4d) go through Service.tx: the session's number moves once with the write, the hints carry it, only
// the people the scene concerns hear it, a repeat of a key moves nothing and tells again, and a refused call tells nothing.

const (
	kSceneChanged = "*playv1.WatchGameSessionResponse_SceneChanged_"
	kStageChanged = "*playv1.WatchGameSessionResponse_StageChanged_"
	kSceneRolled  = "*playv1.WatchGameSessionResponse_SceneCheckRolled_"
)

// sceneHints is the table of the scene, and a stream for the master and each player (Ana, Caio and Bia).
func (at *sceneTable) sceneHints(t *testing.T, quiet ...string) *hintRig {
	t.Helper()
	return newHintRig(t, at.h, at.campaignID, at.master, map[string]*user{"Ana": at.ana, "Caio": at.caio, "Bia": at.bia}, quiet...)
}

func (at *sceneTable) openCall(t *testing.T, point string, us ...*user) func() error {
	t.Helper()
	return func() error {
		req := &playv1.OpenSceneRequest{CampaignId: at.campaignID, PointId: point}
		if len(us) > 0 {
			req.Audience = audienceOf(us...)
		}
		_, err := at.master.play.OpenScene(t.Context(), connect.NewRequest(req))
		return err
	}
}

func (at *sceneTable) closeCall(t *testing.T) func() error {
	t.Helper()
	return func() error {
		_, err := at.master.play.CloseScene(t.Context(), connect.NewRequest(&playv1.CloseSceneRequest{CampaignId: at.campaignID}))
		return err
	}
}

func (at *sceneTable) audienceCall(_ *testing.T, us ...*user) func() error {
	return func() error {
		_, err := at.setAudience(at.master, audienceOf(us...))
		return err
	}
}

func (at *sceneTable) mayJoinCall(_ *testing.T, on bool) func() error {
	return func() error {
		_, err := at.setMayJoin(at.master, on)
		return err
	}
}

// do runs a call of the set-up that must work.
func do(t *testing.T, call func() error) {
	t.Helper()
	if err := call(); err != nil {
		t.Fatalf("set-up call: error = %v", err)
	}
}

// heardKinds fails unless each person heard exactly the kinds listed for them (nothing when they are not listed).
func heardKinds(t *testing.T, heard map[string][]string, want map[string][]string) {
	t.Helper()
	for name, kinds := range heard {
		if !equalKinds(kinds, want[name]) {
			t.Errorf("%s heard %v, want %v", name, kinds, want[name])
		}
	}
}

func equalKinds(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestOpenSceneHints(t *testing.T) {
	t.Parallel()
	at := newSceneTable(t)
	g := at.sceneHints(t)
	heard := g.first(at.openCall(t, at.point), true)
	heardKinds(t, heard, map[string][]string{
		"master": {kSceneChanged}, "Ana": {kSceneChanged}, "Caio": {kSceneChanged}, "Bia": {kSceneChanged},
	})
}

func TestOpenSceneToSomePlayersTellsOnlyThemHints(t *testing.T) {
	t.Parallel()
	at := newSceneTable(t)
	g := at.sceneHints(t, "Caio", "Bia")
	heard := g.first(at.openCall(t, at.point, at.ana), true)
	heardKinds(t, heard, map[string][]string{"master": {kSceneChanged}, "Ana": {kSceneChanged}})
}

func TestOpenTheOpenSceneAgainMovesNothing(t *testing.T) {
	t.Parallel()
	at := newSceneTable(t)
	at.openScene(t, at.point)
	g := at.sceneHints(t)
	before := g.seq()
	if err := at.openCall(t, at.point)(); err != nil {
		t.Fatalf("OpenScene() error = %v", err)
	}
	if got := g.seq(); got != before {
		t.Errorf("hint_seq = %d, want %d: nothing changed", got, before)
	}
	for name, evs := range g.drain() {
		if len(evs) != 0 {
			t.Errorf("%s heard %v of a scene that was open already", name, eventKinds(evs))
		}
	}
}

func TestOpenSceneRefusedTellsNothing(t *testing.T) {
	t.Parallel()
	at := newSceneTable(t)
	g := at.sceneHints(t)
	g.refused(at.openCall(t, newKey())) // no such point
}

func TestOpenSceneWithNoOpenSessionRefusesAsBefore(t *testing.T) {
	t.Parallel()
	at := newSceneTable(t)
	at.master.end(t, at.master.liveSession(t, at.campaignID).GetGameSession())
	err := at.openCall(t, at.point)()
	if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("OpenScene(no open session) error = %v, want failed_precondition", err)
	}
}

func TestCloseSceneHints(t *testing.T) {
	t.Parallel()
	at := newSceneTable(t)
	do(t, at.openCall(t, at.point, at.ana))
	g := at.sceneHints(t, "Caio", "Bia")
	heard := g.first(at.closeCall(t), true)
	heardKinds(t, heard, map[string][]string{"master": {kSceneChanged}, "Ana": {kSceneChanged}})
}

func TestCloseSceneWithNoSceneMovesNothing(t *testing.T) {
	t.Parallel()
	at := newSceneTable(t)
	g := at.sceneHints(t)
	before := g.seq()
	if err := at.closeCall(t)(); err != nil {
		t.Fatalf("CloseScene() error = %v", err)
	}
	if got := g.seq(); got != before {
		t.Errorf("hint_seq = %d, want %d", got, before)
	}
	for name, evs := range g.drain() {
		if len(evs) != 0 {
			t.Errorf("%s heard %v although no scene was open", name, eventKinds(evs))
		}
	}
}

func TestSetSceneAudienceHints(t *testing.T) {
	t.Parallel()
	at := newSceneTable(t)
	do(t, at.openCall(t, at.point, at.ana))
	g := at.sceneHints(t, "Caio")
	heard := g.first(at.audienceCall(t, at.ana, at.bia), true)
	heardKinds(t, heard, map[string][]string{"master": {kSceneChanged}, "Ana": {kSceneChanged}, "Bia": {kSceneChanged}})
}

func TestSetSceneAudienceRefusedTellsNothing(t *testing.T) {
	t.Parallel()
	at := newSceneTable(t)
	g := at.sceneHints(t)
	g.refused(at.audienceCall(t, at.ana)) // no scene is open
}

func TestSetScenePlayersMayJoinHints(t *testing.T) {
	t.Parallel()
	at := newSceneTable(t)
	do(t, at.openCall(t, at.point, at.ana))
	g := at.sceneHints(t)
	heard := g.first(at.mayJoinCall(t, true), true)
	heardKinds(t, heard, map[string][]string{"master": {kSceneChanged}, "Ana": {kSceneChanged}, "Caio": {kSceneChanged}, "Bia": {kSceneChanged}})
}

func TestSetScenePlayersMayJoinRefusedTellsNothing(t *testing.T) {
	t.Parallel()
	at := newSceneTable(t)
	at.openScene(t, at.point) // open to the whole table: there is nobody to let join
	g := at.sceneHints(t)
	g.refused(at.mayJoinCall(t, true))
}

func TestJoinSceneHints(t *testing.T) {
	t.Parallel()
	at := newSceneTable(t)
	do(t, at.openCall(t, at.point, at.ana))
	do(t, at.mayJoinCall(t, true))
	g := at.sceneHints(t, "Bia")
	heard := g.first(func() error { _, err := at.join(at.caio); return err }, true)
	heardKinds(t, heard, map[string][]string{"master": {kSceneChanged}, "Ana": {kSceneChanged}, "Caio": {kSceneChanged}})
}

func TestJoinSceneRefusedTellsNothing(t *testing.T) {
	t.Parallel()
	at := newSceneTable(t)
	do(t, at.openCall(t, at.point, at.ana)) // the switch is off
	g := at.sceneHints(t)
	g.refused(func() error { _, err := at.join(at.caio); return err })
}

func (at *sceneTable) grantCall(_ *testing.T, key, character string) func() error {
	return func() error {
		_, err := sceneGrant(at.armed, at.master, at.action, character, key)
		return err
	}
}

func (at *sceneTable) rollCall(_ *testing.T, u *user, key string, face int32) func() error {
	return func() error {
		_, err := sceneRollWith(at.armed, u, at.campaignID, at.action, key, &playv1.RollSceneCheckRequest{Roll: &playv1.RollSceneCheckRequest_D20Face{D20Face: face}})
		return err
	}
}

func TestGrantSceneAttemptHints(t *testing.T) {
	t.Parallel()
	at := newSceneTable(t)
	do(t, at.openCall(t, at.point, at.ana))
	g := at.sceneHints(t, "Caio", "Bia")
	key := newKey()
	g.keyed(at.grantCall(t, key, at.pens.GetId()), at.grantCall(t, key, at.pens.GetId()), at.grantCall(t, key, at.bri.GetId()))
}

func TestGrantSceneAttemptLostHints(t *testing.T) {
	t.Parallel()
	at := newSceneTable(t)
	do(t, at.openCall(t, at.point, at.ana))
	g := at.sceneHints(t, "Caio", "Bia")
	g.lost(at.grantCall(t, newKey(), at.pens.GetId()))
}

func TestGrantSceneAttemptRefusedTellsNothing(t *testing.T) {
	t.Parallel()
	at := newSceneTable(t)
	do(t, at.openCall(t, at.point, at.ana))
	g := at.sceneHints(t)
	g.refused(at.grantCall(t, newKey(), at.toren.GetId())) // Caio's character is not in the scene
}

func TestRollSceneCheckHints(t *testing.T) {
	t.Parallel()
	at := newSceneTable(t)
	at.openScene(t, at.point)
	g := at.sceneHints(t, "Caio", "Bia")
	key := newKey()
	g.keyed(at.rollCall(t, at.ana, key, 14), at.rollCall(t, at.ana, key, 14), at.rollCall(t, at.ana, key, 9))
}

func TestRollSceneCheckLostHints(t *testing.T) {
	t.Parallel()
	at := newSceneTable(t)
	at.openScene(t, at.point)
	g := at.sceneHints(t, "Caio", "Bia")
	g.lost(at.rollCall(t, at.ana, newKey(), 14))
}

func TestRollSceneCheckRefusedTellsNothing(t *testing.T) {
	t.Parallel()
	at := newSceneTable(t)
	do(t, at.openCall(t, at.point, at.ana))
	g := at.sceneHints(t)
	g.refused(at.rollCall(t, at.caio, newKey(), 14)) // out of the scene
}
