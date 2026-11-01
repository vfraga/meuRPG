package progression

import (
	"fmt"
	"strings"
	"testing"

	"connectrpc.com/connect"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	progressionv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/progression/v1"
)

// What AddMilestone and MoveMilestone (MR-016) do:
// each refusal with its exact words and in the order the checks run, each
// failure with its one log line, and what each call writes (the rows of
// planned_milestones, their positions, no session event). Every step is a
// line of a golden file (testdata/milestones_add.golden.json and
// testdata/milestones_move.golden.json); run with -update to write them
// again after a change of behavior that was meant.

// milestoneRowsShown is how many stored rows a step shows before it only
// counts them (the list at the limit has 100).
const milestoneRowsShown = 6

// milestoneSteps is the record of a test's steps, for its golden file.
type milestoneSteps struct {
	rig    *faultRig
	steps  map[string]any
	events int // the campaign's session events when the steps began
}

func newMilestoneSteps(t *testing.T, rig *faultRig) *milestoneSteps {
	t.Helper()
	return &milestoneSteps{rig: rig, steps: map[string]any{}, events: rig.campaignEvents(t)}
}

// summary is a list of milestones as the steps hold it: one line each, its
// ID, text, state and number of marks (the golden names the IDs at the end,
// once every label is known).
func (s *milestoneSteps) summary(list []*progressionv1.Milestone) []string {
	out := make([]string, 0, len(list))
	for _, m := range list {
		out = append(out, fmt.Sprintf("%s %q reached=%v off_list=%v has_history=%v reached_at=%v marks=%d",
			m.GetId(), m.GetText(), m.GetReached(), m.GetOffList(), m.GetHasHistory(), m.GetReachedAt() != nil, len(m.GetMarks())))
	}
	return out
}

// record writes one step: the answer (or the refusal), the stored rows and the
// log lines it left; a session event fails the test.
func (s *milestoneSteps) record(t *testing.T, name string, answer any, err error) {
	t.Helper()
	rows := s.rig.plannedRows(t)
	if len(rows) > milestoneRowsShown {
		rows = append(rows[:milestoneRowsShown:milestoneRowsShown], fmt.Sprintf("... %d rows in all, the last %s", len(rows), rows[len(rows)-1]))
	}
	step := map[string]any{"rows": rows}
	if err != nil {
		step["refusal"] = refusal(err)
	} else {
		step["answer"] = answer
	}
	if logs := s.rig.takeLogs(t); len(logs) > 0 {
		step["logs"] = logs
	}
	s.steps[fmt.Sprintf("%02d %s", len(s.steps)+1, name)] = step
	if n := s.rig.campaignEvents(t); n != s.events {
		t.Errorf("%s: the session has %d events, want %d (a planned-list edit has none)", name, n, s.events)
	}
	s.rig.wantNoPendingHistoryFault(t, name)
}

// TestAddMilestoneStepByStep checks AddMilestone: who may call it, the text,
// the mode before the key, the key, each failure, the position after the last
// (gaps kept), the limit and the retry that is still answered at it.
func TestAddMilestoneStepByStep(t *testing.T) {
	t.Parallel()
	rig := newFaultRig(t, milestones, 1)
	ctx := t.Context()
	outsider, pending := rig.h.newUser("Intruso"), rig.h.newUser("Pendente")
	rig.h.joinPending(rig.master, rig.campaign, pending)
	rig.startSession(t)
	steps := newMilestoneSteps(t, rig)
	add := func(name string, u *user, campaign, text, key string) *progressionv1.AddMilestoneResponse {
		t.Helper()
		res, err := rig.client(u).AddMilestone(ctx, connect.NewRequest(&progressionv1.AddMilestoneRequest{CampaignId: campaign, Text: text, IdempotencyKey: key}))
		if err != nil {
			steps.record(t, name, nil, err)
			return nil
		}
		steps.record(t, name, map[string]any{
			"milestone": steps.summary([]*progressionv1.Milestone{res.Msg.GetMilestone()}), "milestones": steps.summary(res.Msg.GetMilestones()),
		}, nil)
		return res.Msg
	}
	remove := func(id string) {
		t.Helper()
		if _, err := rig.master.xp.RemoveMilestone(ctx, connect.NewRequest(&progressionv1.RemoveMilestoneRequest{CampaignId: rig.campaign, MilestoneId: id})); err != nil {
			t.Fatalf("RemoveMilestone() error = %v", err)
		}
		rig.takeLogs(t)
	}
	setMode := func(mode campaignsv1.XpMode) {
		t.Helper()
		if err := rig.master.setXPMode(rig.campaign, mode, false); err != nil {
			t.Fatalf("SetCampaignXpMode(%v) error = %v", mode, err)
		}
	}
	long, twoLines, badKey := strings.Repeat("a", maxMilestoneText+1), "um\ndois", strings.Repeat("k", 65)

	// Who may call it, and the request, in the order of the checks.
	add("anonymous", rig.h.anonymous(), rig.campaign, "Marco", "")
	add("outsider", outsider, rig.campaign, "Marco", "")
	add("pending member", pending, rig.campaign, "Marco", "")
	add("player", rig.players[0], rig.campaign, "Marco", "")
	add("malformed campaign", rig.master, "not-a-uuid", "Marco", "")
	add("empty text", rig.master, rig.campaign, "  ", "")
	add("text too long", rig.master, rig.campaign, long, "")
	add("two lines", rig.master, rig.campaign, twoLines, "")
	add("key too long", rig.master, rig.campaign, "Marco", badKey)
	add("key with a control character", rig.master, rig.campaign, "Marco", "a\x01b")
	add("empty text and a bad key", rig.master, rig.campaign, "", badKey)
	setMode(campaignsv1.XpMode_XP_MODE_ENEMIES)
	add("xp campaign, bad text", rig.master, rig.campaign, long, "")
	add("xp campaign, bad key", rig.master, rig.campaign, "Marco", badKey)
	add("xp campaign", rig.master, rig.campaign, "Marco", "k0")
	setMode(campaignsv1.XpMode_XP_MODE_MILESTONES)

	// Each failure: nothing is written, but by the read after the commit.
	for _, tc := range []struct {
		name string
		arm  func()
		key  string
	}{
		{"fails: the mode", func() { rig.faults.arm("xp_mode") }, "f1"},
		{"fails: the mode in the transaction", func() { rig.faults.arm("xp_mode_in_tx") }, "f2"},
		{"fails: the list", func() { rig.faults.armQuery("ListPlannedMilestonesForUpdate", 0) }, "f3"},
		{"fails: the key", func() { rig.faults.armQuery("GetPlannedMilestoneByCreateKey", 0) }, "f4"},
		{"fails: the insert", func() { rig.faults.armQuery("InsertPlannedMilestone", 0) }, "f5"},
		// The add is committed: the answer carries the written milestone and no list.
		{"fails: the read after", func() { rig.faults.armQuery("ListPlannedMilestones", 0) }, "f6"},
	} {
		tc.arm()
		add(tc.name, rig.master, rig.campaign, "Falha", tc.key)
	}
	steps.rig.label(milestoneIDOf(t, rig, "Falha"), "milestone-falha")
	remove(milestoneIDOf(t, rig, "Falha"))

	// The writes: trimmed, after the last, a key kept with the hash of the request.
	a := add("first", rig.master, rig.campaign, "  Chegar ao Vale Seco ", "k1")
	rig.label(a.GetMilestone().GetId(), "milestone-a")
	add("the same request again", rig.master, rig.campaign, "  Chegar ao Vale Seco ", "k1")
	// The key is kept with the hash of the trimmed text, so a retry that differs
	// only by the spaces around it is the same request.
	add("the same key, the text trimmed the same", rig.master, rig.campaign, "Chegar ao Vale Seco", "k1")
	add("the same key, another text", rig.master, rig.campaign, "Derrotar a Sombra", "k1")
	b := add("no key", rig.master, rig.campaign, "Derrotar a Sombra", "")
	rig.label(b.GetMilestone().GetId(), "milestone-b")
	c := add("no key again", rig.master, rig.campaign, "Derrotar a Sombra", "")
	rig.label(c.GetMilestone().GetId(), "milestone-c")
	remove(c.GetMilestone().GetId())
	rig.label(add("after the last was removed", rig.master, rig.campaign, "Fechar o portal", "k2").GetMilestone().GetId(), "milestone-d")
	remove(b.GetMilestone().GetId())
	rig.label(add("after a gap", rig.master, rig.campaign, "Voltar à cidade", "k3").GetMilestone().GetId(), "milestone-e")
	rig.label(add("the longest text", rig.master, rig.campaign, long[:maxMilestoneText], "").GetMilestone().GetId(), "milestone-long")

	// The limit, and the retry that is answered at it.
	if _, err := rig.h.pool.Exec(ctx, `
		INSERT INTO planned_milestones (campaign_id, position, text, created_at, updated_at)
		SELECT $1, g, 'Marco ' || g::TEXT, now(), now() FROM generate_series(10, $2) AS g`, rig.campaign, 10+maxMilestones-len(rig.plannedRows(t))-1); err != nil {
		t.Fatalf("fill the list: %v", err)
	}
	res, err := rig.master.xp.AddMilestone(ctx, connect.NewRequest(&progressionv1.AddMilestoneRequest{CampaignId: rig.campaign, Text: "Mais um", IdempotencyKey: "k4"}))
	steps.record(t, "over the limit", res, err)
	res, err = rig.master.xp.AddMilestone(ctx, connect.NewRequest(&progressionv1.AddMilestoneRequest{CampaignId: rig.campaign, Text: "Voltar à cidade", IdempotencyKey: "k3"}))
	if err == nil {
		steps.record(t, "a retry at the limit", map[string]any{
			"milestone": steps.summary([]*progressionv1.Milestone{res.Msg.GetMilestone()}), "count": len(res.Msg.GetMilestones()),
		}, nil)
	} else {
		steps.record(t, "a retry at the limit", nil, err)
	}
	rig.wantHistoryGolden(t, "milestones_add", steps.steps)
}

// milestoneIDOf is the ID of the planned milestone with the text.
func milestoneIDOf(t *testing.T, rig *faultRig, text string) string {
	t.Helper()
	var id string
	if err := rig.h.pool.QueryRow(t.Context(), `SELECT id FROM planned_milestones WHERE campaign_id = $1 AND text = $2`, rig.campaign, text).Scan(&id); err != nil {
		t.Fatalf("find the milestone %q: %v", text, err)
	}
	return id
}

// TestMoveMilestoneStepByStep checks MoveMilestone: who may call it, the
// milestone and the direction in the order of the checks, each failure, the
// edges that change nothing (gaps kept), the reached ones skipped, and the
// positions renumbered from 0 when something moves.
func TestMoveMilestoneStepByStep(t *testing.T) {
	t.Parallel()
	rig := newFaultRig(t, milestones, 1)
	ctx := t.Context()
	outsider, pending := rig.h.newUser("Intruso"), rig.h.newUser("Pendente")
	rig.h.joinPending(rig.master, rig.campaign, pending)
	rig.startSession(t)
	ids := map[string]string{}
	for _, name := range []string{"A", "B", "C", "D", "E"} {
		ids[name] = rig.master.plan(t, rig.campaign, "Marco "+name).GetId()
		rig.label(ids[name], "milestone-"+strings.ToLower(name))
	}
	if _, err := rig.master.xp.RemoveMilestone(ctx, connect.NewRequest(&progressionv1.RemoveMilestoneRequest{CampaignId: rig.campaign, MilestoneId: ids["B"]})); err != nil {
		t.Fatalf("RemoveMilestone() error = %v", err)
	}
	rig.takeLogs(t)
	steps := newMilestoneSteps(t, rig)
	up, down := progressionv1.MilestoneDirection_MILESTONE_DIRECTION_UP, progressionv1.MilestoneDirection_MILESTONE_DIRECTION_DOWN
	moveKeyed := func(name string, u *user, campaign, id string, dir progressionv1.MilestoneDirection, key string) {
		t.Helper()
		res, err := rig.client(u).MoveMilestone(ctx, connect.NewRequest(&progressionv1.MoveMilestoneRequest{CampaignId: campaign, MilestoneId: id, Direction: dir, IdempotencyKey: key}))
		if err != nil {
			steps.record(t, name, nil, err)
			return
		}
		steps.record(t, name, steps.summary(res.Msg.GetMilestones()), nil)
	}
	move := func(name string, u *user, campaign, id string, dir progressionv1.MilestoneDirection) {
		t.Helper()
		moveKeyed(name, u, campaign, id, dir, "")
	}
	unknown := newKey()
	rig.label(unknown, "unknown")

	// Who may call it, and the request, in the order of the checks.
	move("anonymous", rig.h.anonymous(), rig.campaign, ids["A"], up)
	move("outsider", outsider, rig.campaign, ids["A"], up)
	move("pending member", pending, rig.campaign, ids["A"], up)
	move("player", rig.players[0], rig.campaign, ids["A"], up)
	move("malformed campaign", rig.master, "not-a-uuid", ids["A"], up)
	move("malformed milestone, no direction", rig.master, rig.campaign, "not-a-uuid", 0)
	move("unknown milestone, no direction", rig.master, rig.campaign, unknown, 0)
	move("a direction that does not exist", rig.master, rig.campaign, ids["A"], 7)
	move("unknown milestone", rig.master, rig.campaign, unknown, up)
	if err := rig.master.setXPMode(rig.campaign, campaignsv1.XpMode_XP_MODE_ENEMIES, false); err != nil {
		t.Fatalf("SetCampaignXpMode(enemies) error = %v", err)
	}
	move("xp campaign, malformed milestone, no direction", rig.master, rig.campaign, "not-a-uuid", 0)
	if err := rig.master.setXPMode(rig.campaign, campaignsv1.XpMode_XP_MODE_MILESTONES, false); err != nil {
		t.Fatalf("SetCampaignXpMode(milestones) error = %v", err)
	}

	// Each failure of a move that would swap C and D: nothing is written, but by the read after the commit.
	for _, tc := range []struct {
		name string
		arm  func()
	}{
		{"fails: the mode", func() { rig.faults.arm("xp_mode") }},
		{"fails: the mode in the transaction", func() { rig.faults.arm("xp_mode_in_tx") }},
		{"fails: the milestone", func() { rig.faults.armQuery("GetPlannedMilestoneForUpdate", 0) }},
		{"fails: the milestone's marks", func() { rig.faults.armQuery("ListLiveMilestoneAwardsOf", 0) }},
		{"fails: the list", func() { rig.faults.armQuery("ListPlannedMilestonesForUpdate", 0) }},
		{"fails: the marks", func() { rig.faults.armQuery("ListLiveMilestoneAwards", 0) }},
		{"fails: the second position", func() { rig.faults.armQuery("SetPlannedMilestonePosition", 1) }},
	} {
		tc.arm()
		move(tc.name, rig.master, rig.campaign, ids["D"], up)
	}

	// The edges change nothing, and keep the gap B left.
	move("the first up", rig.master, rig.campaign, ids["A"], up)
	move("the last down", rig.master, rig.campaign, ids["E"], down)
	// A move renumbers the whole list from 0.
	move("D up", rig.master, rig.campaign, ids["D"], up)
	// A reached one keeps its place: the move goes past it.
	rig.master.reach(t, rig.campaign, ids["C"], rig.pcs[0].GetId())
	steps.events = rig.campaignEvents(t)
	rig.takeLogs(t)
	move("E up, past the reached C", rig.master, rig.campaign, ids["E"], up)
	move("the reached C", rig.master, rig.campaign, ids["C"], down)
	rig.master.reach(t, rig.campaign, ids["A"], rig.pcs[0].GetId())
	off := rig.master.milestone(t, rig.campaign, rig.pcs[0].GetId())
	rig.label(off.GetId(), "off-list")
	steps.events = rig.campaignEvents(t)
	rig.takeLogs(t)
	move("E up, only reached ones above", rig.master, rig.campaign, ids["E"], up)
	move("D up, past the reached C, with one off the list", rig.master, rig.campaign, ids["D"], up)
	// The move is committed: the answer has no list, and the app reads it again.
	rig.faults.armQuery("ListPlannedMilestones", 0)
	move("fails: the read after", rig.master, rig.campaign, ids["D"], down)

	// A move with a key (MoveMilestone's idempotency): two more planned milestones give the same
	// request somewhere to go twice. The retry replays the first answer and moves nothing again;
	// the key with another direction is refused; a retry whose read after fails is still answered
	// by the committed path (an empty list).
	for _, name := range []string{"F", "G"} {
		ids[name] = rig.master.plan(t, rig.campaign, "Marco "+name).GetId()
		rig.label(ids[name], "milestone-"+strings.ToLower(name))
	}
	rig.takeLogs(t)
	key := newKey()
	moveKeyed("key: F up", rig.master, rig.campaign, ids["F"], up, key)
	moveKeyed("key again, the same move: replayed", rig.master, rig.campaign, ids["F"], up, key)
	moveKeyed("key again, another direction", rig.master, rig.campaign, ids["F"], down, key)
	moveKeyed("key again, another milestone", rig.master, rig.campaign, ids["G"], up, key)
	rig.faults.armQuery("ListPlannedMilestones", 0)
	moveKeyed("key again, the read after fails", rig.master, rig.campaign, ids["F"], up, key)
	rig.wantHistoryGolden(t, "milestones_move", steps.steps)
}
