package play

import (
	"testing"
	"time"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// Finding U2-3: unlimited scene rolls are unbounded; past 20000 events GetSessionSummary fails forever
func TestReview2_SceneRollVolumeBreaksSummary(t *testing.T) {
	a := newArmed(t)
	point, actions := a.h.newScene(a.mapID, "A carroça", true, 0, sceneSpec{"skill:investigation", new(int32(12))})
	a.openScene(t, point)
	for range 3 {
		a.checkRoll(t, a.ana, actions[0], 10) // real calls: an unlimited action never refuses
	}
	listed, err := a.master.play.ListGameSessions(t.Context(), connect.NewRequest(&playv1.ListGameSessionsRequest{CampaignId: a.campaignID}))
	if err != nil || len(listed.Msg.GetGameSessions()) != 1 {
		t.Fatalf("ListGameSessions() = %v, %v", listed, err)
	}
	open := listed.Msg.GetGameSessions()[0]

	// Bulk copy of a real roll event, 20001 times, as 3 + 20001 rolls.
	const extra = 20001
	if _, err := a.h.pool.Exec(t.Context(), `
INSERT INTO session_events (game_session_id, seq, kind, actor_user_id, character_id, payload, created_at)
SELECT e.game_session_id, (SELECT max(seq) FROM session_events WHERE game_session_id = e.game_session_id) + g, e.kind, e.actor_user_id, e.character_id, e.payload, e.created_at
FROM (SELECT * FROM session_events WHERE game_session_id = $1 AND kind = 'scene_check_rolled' LIMIT 1) e, generate_series(1, $2) g`,
		open.GetId(), extra); err != nil {
		t.Fatalf("bulk insert: %v", err)
	}
	var n int
	if err := a.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM session_events WHERE game_session_id = $1 AND kind = 'scene_check_rolled'`, open.GetId()).Scan(&n); err != nil {
		t.Fatal(err)
	}
	t.Logf("scene_check_rolled rows: %d", n)

	a.h.roller.queue(10)
	start := time.Now()
	_, rollErr := a.ana.play.RollSceneCheck(t.Context(), connect.NewRequest(&playv1.RollSceneCheckRequest{
		CampaignId: a.campaignID, ActionId: actions[0], IdempotencyKey: newKey(), Roll: &playv1.RollSceneCheckRequest_RollInApp{RollInApp: true},
	}))
	t.Logf("one more RollSceneCheck at %d rolls: %v (err=%v)", n, time.Since(start), rollErr)
	start = time.Now()
	_, getErr := a.ana.play.GetOpenScene(t.Context(), connect.NewRequest(&playv1.GetOpenSceneRequest{CampaignId: a.campaignID}))
	t.Logf("GetOpenScene at %d rolls: %v (err=%v)", n, time.Since(start), getErr)

	ended := a.master.end(t, open)
	for who, u := range map[string]*user{"master": a.master, "player": a.ana} {
		if _, err := a.summary(t, u, ended.GetId()); err != nil {
			t.Errorf("%s: GetSessionSummary() code = %v, error = %v; want the summary", who, connect.CodeOf(err), err)
		}
	}
}
