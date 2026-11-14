package play

import (
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The group check (#735 part 2, 4d) goes through Service.tx: each write moves the session's number once, the hint carries it, it
// reaches the master and the players of the open scene (a player's own roll, the master and the roller), a repeat of the key moves
// nothing and tells again, and a refused call tells nothing.

const kGroupChanged = "*playv1.WatchGameSessionResponse_GroupCheckChanged_"

func (a *armed) askGroupCall(t *testing.T, key string) func() error {
	t.Helper()
	return func() error {
		_, err := a.master.contests.RequestGroupCheck(t.Context(), connect.NewRequest(&playv1.RequestGroupCheckRequest{
			CampaignId: a.campaignID, IdempotencyKey: key, SkillKey: stealthKey, Dc: 12,
		}))
		return err
	}
}

func (a *armed) rollGroupCall(t *testing.T, u *user, key, checkID string, face int) func() error {
	t.Helper()
	return func() error {
		a.h.roller.queue(face)
		_, err := u.contests.RollGroupCheck(t.Context(), connect.NewRequest(&playv1.RollGroupCheckRequest{
			CampaignId: a.campaignID, IdempotencyKey: key, GroupCheckId: checkID, Roll: rollIn(),
		}))
		return err
	}
}

func (a *armed) rollForCall(t *testing.T, key, checkID, characterID string, face int) func() error {
	t.Helper()
	return func() error {
		a.h.roller.queue(face)
		_, err := a.master.contests.RollForPlayer(t.Context(), connect.NewRequest(&playv1.RollForPlayerRequest{
			CampaignId: a.campaignID, IdempotencyKey: key, GroupCheckId: checkID, CharacterId: characterID, Roll: rollIn(),
		}))
		return err
	}
}

func (a *armed) closeGroupCall(t *testing.T, key, checkID string) func() error {
	t.Helper()
	return func() error {
		_, err := a.master.contests.CloseGroupCheck(t.Context(), connect.NewRequest(&playv1.CloseGroupCheckRequest{
			CampaignId: a.campaignID, IdempotencyKey: key, GroupCheckId: checkID,
		}))
		return err
	}
}

func TestRequestGroupCheckHints(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	g := a.stageHints(t)
	key := newKey()
	g.keyed(a.askGroupCall(t, key), a.askGroupCall(t, key), func() error {
		_, err := a.master.contests.RequestGroupCheck(t.Context(), connect.NewRequest(&playv1.RequestGroupCheckRequest{
			CampaignId: a.campaignID, IdempotencyKey: key, SkillKey: stealthKey, Dc: 13,
		}))
		return err
	})
}

func TestRequestGroupCheckLostHints(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	g := a.stageHints(t)
	g.lost(a.askGroupCall(t, newKey()))
}

func TestRequestGroupCheckInAPartialSceneTellsOnlyTheScene(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	point, _ := a.h.newScene(a.mapID, "A clareira", true, 1)
	do(t, func() error {
		_, err := a.master.play.OpenScene(t.Context(), connect.NewRequest(&playv1.OpenSceneRequest{CampaignId: a.campaignID, PointId: point, Audience: audienceOf(a.ana)}))
		return err
	})
	g := a.stageHints(t, "Caio", "Bia")
	heard := g.first(a.askGroupCall(t, newKey()), true)
	heardKinds(t, heard, map[string][]string{"master": {kGroupChanged}, "Ana": {kGroupChanged}})
}

func TestRequestGroupCheckRefusedTellsNothing(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	do(t, a.askGroupCall(t, newKey()))
	g := a.stageHints(t)
	g.refused(a.askGroupCall(t, newKey())) // a check is open already
}

func TestRequestGroupCheckWithNoOpenSessionRefusesAsBefore(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	a.master.end(t, a.master.liveSession(t, a.campaignID).GetGameSession())
	if err := a.askGroupCall(t, newKey())(); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("RequestGroupCheck(no open session) error = %v, want failed_precondition", err)
	}
}

func TestRollGroupCheckHints(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	check := a.mustRequestGroup(t, nil).GetId()
	g := a.stageHints(t, "Caio", "Bia")
	key := newKey()
	g.keyed(a.rollGroupCall(t, a.ana, key, check, 10), a.rollGroupCall(t, a.ana, key, check, 10), a.rollGroupCall(t, a.ana, key, newKey(), 11))
}

func TestRollGroupCheckLostHints(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	check := a.mustRequestGroup(t, nil).GetId()
	g := a.stageHints(t, "Caio", "Bia")
	g.lost(a.rollGroupCall(t, a.ana, newKey(), check, 10))
}

func TestRollGroupCheckRefusedTellsNothing(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	check := a.mustRequestGroup(t, nil).GetId()
	do(t, a.rollGroupCall(t, a.ana, newKey(), check, 10))
	g := a.stageHints(t)
	g.refused(a.rollGroupCall(t, a.ana, newKey(), check, 10)) // Ana answered already
}

func TestRollForPlayerHints(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	check := a.mustRequestGroup(t, nil).GetId()
	g := a.stageHints(t)
	key := newKey()
	g.keyed(a.rollForCall(t, key, check, a.toren.GetId(), 10), a.rollForCall(t, key, check, a.toren.GetId(), 10), a.rollForCall(t, key, check, newKey(), 11))
}

func TestRollForPlayerRefusedTellsNothing(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	check := a.mustRequestGroup(t, nil).GetId()
	g := a.stageHints(t)
	g.refused(a.rollForCall(t, newKey(), check, newKey(), 10)) // no such character
}

func TestCloseGroupCheckHints(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	check := a.mustRequestGroup(t, nil).GetId()
	g := a.stageHints(t)
	key := newKey()
	g.keyed(a.closeGroupCall(t, key, check), a.closeGroupCall(t, key, check), a.closeGroupCall(t, key, newKey()))
}

func TestCloseGroupCheckRefusedTellsNothing(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	g := a.stageHints(t)
	g.refused(a.closeGroupCall(t, newKey(), newKey())) // no such check
}
