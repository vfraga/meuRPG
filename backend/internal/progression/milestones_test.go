package progression

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	progressionv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/progression/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dbtest"
)

// The acceptance criteria of MR-016's planned milestones (question 45): the
// master writes the campaign's milestones ahead of time, marks one reached
// and picks who levels up, gives it to someone who was left out, and the
// players read only the reached ones. Each test starts its own database.

// plan adds a milestone to the campaign's list, or fails the test.
func (u *user) plan(t *testing.T, campaignID, text string) *progressionv1.Milestone {
	t.Helper()
	res, err := u.xp.AddMilestone(t.Context(), connect.NewRequest(&progressionv1.AddMilestoneRequest{CampaignId: campaignID, Text: text}))
	if err != nil {
		t.Fatalf("AddMilestone(%q) error = %v", text, err)
	}
	return res.Msg.GetMilestone()
}

// reach marks a planned milestone reached for the characters, or fails the
// test.
func (u *user) reach(t *testing.T, campaignID, milestoneID string, ids ...string) *progressionv1.MarkMilestoneReachedResponse {
	t.Helper()
	res, err := u.xp.MarkMilestoneReached(t.Context(), connect.NewRequest(&progressionv1.MarkMilestoneReachedRequest{
		CampaignId: campaignID, MilestoneId: milestoneID, CharacterIds: ids, IdempotencyKey: newKey(),
	}))
	if err != nil {
		t.Fatalf("MarkMilestoneReached() error = %v", err)
	}
	return res.Msg
}

// giveMilestone gives a reached milestone to more characters.
func (u *user) giveMilestone(milestoneID, campaignID string, ids ...string) (*progressionv1.GiveMilestoneToResponse, error) {
	res, err := u.xp.GiveMilestoneTo(context.Background(), connect.NewRequest(&progressionv1.GiveMilestoneToRequest{
		CampaignId: campaignID, MilestoneId: milestoneID, CharacterIds: ids, IdempotencyKey: newKey(),
	}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

// milestones lists the campaign's milestones as u, or fails the test.
func (u *user) milestones(t *testing.T, campaignID string) []*progressionv1.Milestone {
	t.Helper()
	res, err := u.xp.ListMilestones(t.Context(), connect.NewRequest(&progressionv1.ListMilestonesRequest{CampaignId: campaignID}))
	if err != nil {
		t.Fatalf("ListMilestones() error = %v", err)
	}
	return res.Msg.GetMilestones()
}

// texts is the text of each milestone, in order.
func texts(ms []*progressionv1.Milestone) []string {
	var out []string
	for _, m := range ms {
		out = append(out, m.GetText())
	}
	return out
}

// TestMR016_PlannedMilestones: the master adds, edits, orders and removes the
// planned milestones one at a time; the text and the count have limits; a
// reached milestone cannot be changed.
func TestMR016_PlannedMilestones(t *testing.T) {
	t.Parallel()
	tb := newTable(t, milestones, 1)
	ctx := t.Context()
	a := tb.master.plan(t, tb.campaign, "  Chegar ao Vale Seco ")
	b := tb.master.plan(t, tb.campaign, "Derrotar o chefe")
	c := tb.master.plan(t, tb.campaign, "Fechar o portal")
	if a.GetText() != "Chegar ao Vale Seco" || a.GetReached() {
		t.Errorf("first milestone = %v, want its text trimmed and planned", a)
	}
	if got := texts(tb.master.milestones(t, tb.campaign)); !slices.Equal(got, []string{"Chegar ao Vale Seco", "Derrotar o chefe", "Fechar o portal"}) {
		t.Fatalf("list = %q, want the three in the order they were added", got)
	}

	// Edit.
	upd, err := tb.master.xp.UpdateMilestone(ctx, connect.NewRequest(&progressionv1.UpdateMilestoneRequest{CampaignId: tb.campaign, MilestoneId: b.GetId(), Text: "Derrotar a Sombra"}))
	if err != nil || upd.Msg.GetMilestone().GetText() != "Derrotar a Sombra" {
		t.Fatalf("UpdateMilestone() = %v, %v", upd, err)
	}

	// Order: the last goes up, then the first goes down; the edges change nothing.
	move := func(id string, dir progressionv1.MilestoneDirection) []string {
		t.Helper()
		res, err := tb.master.xp.MoveMilestone(ctx, connect.NewRequest(&progressionv1.MoveMilestoneRequest{CampaignId: tb.campaign, MilestoneId: id, Direction: dir}))
		if err != nil {
			t.Fatalf("MoveMilestone() error = %v", err)
		}
		return texts(res.Msg.GetMilestones())
	}
	up, down := progressionv1.MilestoneDirection_MILESTONE_DIRECTION_UP, progressionv1.MilestoneDirection_MILESTONE_DIRECTION_DOWN
	if got := move(c.GetId(), up); !slices.Equal(got, []string{"Chegar ao Vale Seco", "Fechar o portal", "Derrotar a Sombra"}) {
		t.Errorf("after the last went up: %q", got)
	}
	if got := move(a.GetId(), down); !slices.Equal(got, []string{"Fechar o portal", "Chegar ao Vale Seco", "Derrotar a Sombra"}) {
		t.Errorf("after the first went down: %q", got)
	}
	if got := move(c.GetId(), up); !slices.Equal(got, []string{"Fechar o portal", "Chegar ao Vale Seco", "Derrotar a Sombra"}) {
		t.Errorf("moving the first up changed the list: %q", got)
	}
	if got := move(b.GetId(), down); !slices.Equal(got, []string{"Fechar o portal", "Chegar ao Vale Seco", "Derrotar a Sombra"}) {
		t.Errorf("moving the last down changed the list: %q", got)
	}
	_, err = tb.master.xp.MoveMilestone(ctx, connect.NewRequest(&progressionv1.MoveMilestoneRequest{CampaignId: tb.campaign, MilestoneId: b.GetId()}))
	wantCode(t, "MoveMilestone(no direction)", err, connect.CodeInvalidArgument)

	// A reached one keeps its place and is skipped by a move.
	tb.master.reach(t, tb.campaign, a.GetId(), tb.ids(1)...)
	if got := move(b.GetId(), up); !slices.Equal(got, []string{"Derrotar a Sombra", "Chegar ao Vale Seco", "Fechar o portal"}) {
		t.Errorf("after the last went up past a reached one: %q", got)
	}
	_, err = tb.master.xp.UpdateMilestone(ctx, connect.NewRequest(&progressionv1.UpdateMilestoneRequest{CampaignId: tb.campaign, MilestoneId: a.GetId(), Text: "Outro"}))
	wantBlocked(t, "UpdateMilestone(reached)", err, progressionv1.XPBlockedReason_XP_BLOCKED_REASON_MILESTONE_ALREADY_REACHED)
	_, err = tb.master.xp.MoveMilestone(ctx, connect.NewRequest(&progressionv1.MoveMilestoneRequest{CampaignId: tb.campaign, MilestoneId: a.GetId(), Direction: up}))
	wantBlocked(t, "MoveMilestone(reached)", err, progressionv1.XPBlockedReason_XP_BLOCKED_REASON_MILESTONE_ALREADY_REACHED)
	_, err = tb.master.xp.RemoveMilestone(ctx, connect.NewRequest(&progressionv1.RemoveMilestoneRequest{CampaignId: tb.campaign, MilestoneId: a.GetId()}))
	wantBlocked(t, "RemoveMilestone(reached)", err, progressionv1.XPBlockedReason_XP_BLOCKED_REASON_MILESTONE_ALREADY_REACHED)

	// Remove.
	rem, err := tb.master.xp.RemoveMilestone(ctx, connect.NewRequest(&progressionv1.RemoveMilestoneRequest{CampaignId: tb.campaign, MilestoneId: c.GetId()}))
	if err != nil {
		t.Fatalf("RemoveMilestone() error = %v", err)
	}
	if got := texts(rem.Msg.GetMilestones()); !slices.Equal(got, []string{"Derrotar a Sombra", "Chegar ao Vale Seco"}) {
		t.Errorf("after removing: %q", got)
	}
	_, err = tb.master.xp.RemoveMilestone(ctx, connect.NewRequest(&progressionv1.RemoveMilestoneRequest{CampaignId: tb.campaign, MilestoneId: c.GetId()}))
	wantCode(t, "RemoveMilestone(again)", err, connect.CodeNotFound)
	_, err = tb.master.xp.UpdateMilestone(ctx, connect.NewRequest(&progressionv1.UpdateMilestoneRequest{CampaignId: tb.campaign, MilestoneId: "not-a-uuid", Text: "x"}))
	wantCode(t, "UpdateMilestone(not a UUID)", err, connect.CodeNotFound)

	// The text: one line, 1 to 120 characters.
	for name, text := range map[string]string{"empty": "  ", "too long": strings.Repeat("a", 121), "two lines": "um\ndois"} {
		_, err := tb.master.xp.AddMilestone(ctx, connect.NewRequest(&progressionv1.AddMilestoneRequest{CampaignId: tb.campaign, Text: text}))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("AddMilestone(%s) error = %v, want invalid_argument", name, err)
		}
		_, err = tb.master.xp.UpdateMilestone(ctx, connect.NewRequest(&progressionv1.UpdateMilestoneRequest{CampaignId: tb.campaign, MilestoneId: b.GetId(), Text: text}))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("UpdateMilestone(%s) error = %v, want invalid_argument", name, err)
		}
	}
	tb.master.plan(t, tb.campaign, strings.Repeat("a", 120)) // the longest that fits

	// The count: 100 per campaign, the reached ones included.
	for i := len(tb.master.milestones(t, tb.campaign)); i < maxMilestones; i++ {
		tb.master.plan(t, tb.campaign, "Marco")
	}
	_, err = tb.master.xp.AddMilestone(ctx, connect.NewRequest(&progressionv1.AddMilestoneRequest{CampaignId: tb.campaign, Text: "Mais um"}))
	wantCode(t, "AddMilestone(the 101st)", err, connect.CodeResourceExhausted)
}

// TestMR016_ReachingAPlannedMilestoneLetsTheChosenLevelUp: marking a planned
// milestone, in any order, makes the chosen characters "can level up" as an ad
// hoc milestone does, with the milestone's text as the reason and no XP; it
// shows when and for whom; and the session records it.
func TestMR016_ReachingAPlannedMilestoneLetsTheChosenLevelUp(t *testing.T) {
	t.Parallel()
	tb := newTable(t, milestones, 3)
	ctx := t.Context()
	if _, err := tb.master.play.StartGameSession(ctx, connect.NewRequest(&playv1.StartGameSessionRequest{CampaignId: tb.campaign})); err != nil {
		t.Fatalf("StartGameSession() error = %v", err)
	}
	first := tb.master.plan(t, tb.campaign, "Chegar ao Vale Seco")
	second := tb.master.plan(t, tb.campaign, "Derrotar a Sombra")
	tb.master.plan(t, tb.campaign, "Fechar o portal")

	// Out of order: the second, for two of the three (the third was away).
	before := time.Now().Add(-time.Minute)
	res := tb.master.reach(t, tb.campaign, second.GetId(), tb.ids(2)...)
	award := res.GetAward()
	if award.GetMode() != progressionv1.XPAwardMode_XP_AWARD_MODE_MILESTONE || award.GetReason() != "Derrotar a Sombra" || award.GetTotalXp() != 0 || len(award.GetShares()) != 2 {
		t.Fatalf("award = %v; want a milestone with the milestone's text, no XP and 2 characters", award)
	}
	ms := res.GetMilestone()
	if !ms.GetReached() || ms.GetReachedAt().AsTime().Before(before) || len(ms.GetMarks()) != 1 || ms.GetMarks()[0].GetId() != award.GetId() {
		t.Errorf("milestone = %v; want it reached, now, with the mark", ms)
	}
	if ms.GetMarks()[0].GetGivenByDisplayName() != "Samuel" || !ms.GetMarks()[0].GetCanUndo() {
		t.Errorf("mark = %v; want who gave it and can_undo for the master", ms.GetMarks()[0])
	}

	// Who can level up: the two chosen, not the third.
	for i, c := range tb.master.experience(t, tb.campaign).GetCharacters() {
		if want := i < 2; c.GetCanLevelUp() != want {
			t.Errorf("%s: can level up = %v, want %v", c.GetName(), c.GetCanLevelUp(), want)
		}
		if c.GetExperiencePoints() != 0 {
			t.Errorf("%s: XP = %d, a milestone counts none", c.GetName(), c.GetExperiencePoints())
		}
	}
	if c := tb.players[1].character(t, tb.pcs[1]); !c.GetCanLevelUp() || c.GetLevelUpReason() != charactersv1.LevelUpReason_LEVEL_UP_REASON_MILESTONE {
		t.Errorf("the chosen player's sheet = can_level_up %v (%v), want true by milestone", c.GetCanLevelUp(), c.GetLevelUpReason())
	}
	if c := tb.players[2].character(t, tb.pcs[2]); c.GetCanLevelUp() {
		t.Error("the character left out can level up")
	}

	// The list: the others are still planned, in order.
	list := tb.master.milestones(t, tb.campaign)
	if got := texts(list); !slices.Equal(got, []string{"Chegar ao Vale Seco", "Derrotar a Sombra", "Fechar o portal"}) {
		t.Fatalf("list = %q", got)
	}
	if list[0].GetReached() || !list[1].GetReached() || list[2].GetReached() {
		t.Errorf("reached = %v %v %v, want only the second", list[0].GetReached(), list[1].GetReached(), list[2].GetReached())
	}
	// The history has it like any milestone, for everybody.
	if h := tb.players[2].history(t, tb.campaign); len(h) != 1 || h[0].GetReason() != "Derrotar a Sombra" {
		t.Errorf("history = %v, want the milestone", h)
	}
	// The session recorded it, with no text.
	if n := tb.events(t, "milestone_marked"); n != 1 {
		t.Errorf("milestone_marked events = %d, want 1", n)
	}

	// A milestone is reached once.
	_, err := tb.master.xp.MarkMilestoneReached(ctx, connect.NewRequest(&progressionv1.MarkMilestoneReachedRequest{
		CampaignId: tb.campaign, MilestoneId: second.GetId(), CharacterIds: tb.ids(3)[2:], IdempotencyKey: newKey(),
	}))
	wantBlocked(t, "MarkMilestoneReached(again)", err, progressionv1.XPBlockedReason_XP_BLOCKED_REASON_MILESTONE_ALREADY_REACHED)
	// An ID that is not a milestone of the campaign.
	_, err = tb.master.xp.MarkMilestoneReached(ctx, connect.NewRequest(&progressionv1.MarkMilestoneReachedRequest{
		CampaignId: tb.campaign, MilestoneId: newKey(), CharacterIds: tb.ids(1), IdempotencyKey: newKey(),
	}))
	wantCode(t, "MarkMilestoneReached(unknown)", err, connect.CodeNotFound)
	// The characters: living player characters, 1 to 40, no repeat.
	_, err = tb.master.xp.MarkMilestoneReached(ctx, connect.NewRequest(&progressionv1.MarkMilestoneReachedRequest{
		CampaignId: tb.campaign, MilestoneId: first.GetId(), IdempotencyKey: newKey(),
	}))
	wantCode(t, "MarkMilestoneReached(nobody)", err, connect.CodeInvalidArgument)
	_, err = tb.master.xp.MarkMilestoneReached(ctx, connect.NewRequest(&progressionv1.MarkMilestoneReachedRequest{
		CampaignId: tb.campaign, MilestoneId: first.GetId(), CharacterIds: []string{newKey()}, IdempotencyKey: newKey(),
	}))
	wantBlocked(t, "MarkMilestoneReached(not a character)", err, progressionv1.XPBlockedReason_XP_BLOCKED_REASON_CHARACTER_NOT_ELIGIBLE)

	// A retry with the same key changes and answers the same.
	key := newKey()
	req := &progressionv1.MarkMilestoneReachedRequest{CampaignId: tb.campaign, MilestoneId: first.GetId(), CharacterIds: tb.ids(1), IdempotencyKey: key}
	one, err := tb.master.xp.MarkMilestoneReached(ctx, connect.NewRequest(req))
	if err != nil {
		t.Fatalf("MarkMilestoneReached() error = %v", err)
	}
	two, err := tb.master.xp.MarkMilestoneReached(ctx, connect.NewRequest(req))
	if err != nil || two.Msg.GetAward().GetId() != one.Msg.GetAward().GetId() {
		t.Fatalf("retry = %v, %v; want the same award", two, err)
	}
	if n := tb.events(t, "milestone_marked"); n != 2 {
		t.Errorf("milestone_marked events = %d after a retry, want 2", n)
	}
}

// TestMR016_GiveAReachedMilestoneToSomeoneElse: "Dar a mais alguém" is a new
// mark on the same milestone, in the history, for a character that was left
// out; it is refused for a planned milestone and for a character that has it.
func TestMR016_GiveAReachedMilestoneToSomeoneElse(t *testing.T) {
	t.Parallel()
	tb := newTable(t, milestones, 3)
	ms := tb.master.plan(t, tb.campaign, "Chegar ao Vale Seco")
	other := tb.master.plan(t, tb.campaign, "Derrotar a Sombra")

	if _, err := tb.master.giveMilestone(ms.GetId(), tb.campaign, tb.ids(3)[2]); err == nil {
		t.Fatal("GiveMilestoneTo(planned) succeeded")
	} else {
		wantBlocked(t, "GiveMilestoneTo(planned)", err, progressionv1.XPBlockedReason_XP_BLOCKED_REASON_MILESTONE_NOT_REACHED)
	}
	tb.master.reach(t, tb.campaign, ms.GetId(), tb.ids(2)...)
	if c := tb.players[2].character(t, tb.pcs[2]); c.GetCanLevelUp() {
		t.Fatal("Brisa can level up before she was given the milestone")
	}

	res, err := tb.master.giveMilestone(ms.GetId(), tb.campaign, tb.pcs[2].GetId())
	if err != nil {
		t.Fatalf("GiveMilestoneTo() error = %v", err)
	}
	award := res.GetAward()
	if award.GetReason() != "Chegar ao Vale Seco" || len(award.GetShares()) != 1 || award.GetShares()[0].GetCharacterId() != tb.pcs[2].GetId() {
		t.Errorf("award = %v; want a milestone with the same text for Brisa only", award)
	}
	if got := res.GetMilestone(); len(got.GetMarks()) != 2 || got.GetMarks()[1].GetId() != award.GetId() {
		t.Errorf("milestone = %v; want two marks, the new one last", got)
	}
	// Still one milestone, and the other is planned.
	list := tb.master.milestones(t, tb.campaign)
	if len(list) != 2 || !list[0].GetReached() || list[1].GetReached() || list[1].GetId() != other.GetId() {
		t.Errorf("list = %v, want the same two milestones", list)
	}
	if c := tb.players[2].character(t, tb.pcs[2]); !c.GetCanLevelUp() || c.GetLevelUpReason() != charactersv1.LevelUpReason_LEVEL_UP_REASON_MILESTONE {
		t.Errorf("Brisa = can_level_up %v (%v), want true by milestone", c.GetCanLevelUp(), c.GetLevelUpReason())
	}
	// The history has both marks, newest first.
	if h := tb.players[0].history(t, tb.campaign); len(h) != 2 || h[0].GetId() != award.GetId() {
		t.Errorf("history = %v, want the two marks, the new one first", h)
	}

	// Who has it already cannot be given it again; a stranger is not eligible.
	_, err = tb.master.giveMilestone(ms.GetId(), tb.campaign, tb.pcs[0].GetId())
	wantBlocked(t, "GiveMilestoneTo(has it)", err, progressionv1.XPBlockedReason_XP_BLOCKED_REASON_CHARACTER_ALREADY_MARKED)
	_, err = tb.master.giveMilestone(ms.GetId(), tb.campaign, newKey())
	wantBlocked(t, "GiveMilestoneTo(not a character)", err, progressionv1.XPBlockedReason_XP_BLOCKED_REASON_CHARACTER_NOT_ELIGIBLE)
	_, err = tb.master.giveMilestone(newKey(), tb.campaign, tb.pcs[2].GetId())
	wantCode(t, "GiveMilestoneTo(unknown)", err, connect.CodeNotFound)
}

// TestMR016_UndoOfAPlannedMilestone: the undo takes the last mark; undoing a
// milestone's only mark makes it planned again, in its place; undoing a "Dar a
// mais alguém" mark leaves the milestone reached for the others.
func TestMR016_UndoOfAPlannedMilestone(t *testing.T) {
	t.Parallel()
	tb := newTable(t, milestones, 2)
	a := tb.master.plan(t, tb.campaign, "Chegar ao Vale Seco")
	tb.master.plan(t, tb.campaign, "Derrotar a Sombra")

	tb.master.reach(t, tb.campaign, a.GetId(), tb.ids(1)...)
	if _, err := tb.master.giveMilestone(a.GetId(), tb.campaign, tb.pcs[1].GetId()); err != nil {
		t.Fatalf("GiveMilestoneTo() error = %v", err)
	}
	// The first undo takes the "Dar a mais alguém" mark only.
	if _, err := tb.master.undo(tb.campaign, newKey()); err != nil {
		t.Fatalf("UndoLastXPAward() error = %v", err)
	}
	list := tb.master.milestones(t, tb.campaign)
	if !list[0].GetReached() || len(list[0].GetMarks()) != 1 || len(list[0].GetMarks()[0].GetShares()) != 1 {
		t.Errorf("after undoing the second mark: %v; want reached, with the first mark only", list[0])
	}
	if c := tb.players[1].character(t, tb.pcs[1]); c.GetCanLevelUp() {
		t.Error("the second character can still level up after the undo")
	}
	if c := tb.players[0].character(t, tb.pcs[0]); !c.GetCanLevelUp() {
		t.Error("the first character lost the milestone")
	}
	// It can be given again to the same one.
	if _, err := tb.master.giveMilestone(a.GetId(), tb.campaign, tb.pcs[1].GetId()); err != nil {
		t.Fatalf("GiveMilestoneTo(after the undo) error = %v", err)
	}
	for range 2 {
		if _, err := tb.master.undo(tb.campaign, newKey()); err != nil {
			t.Fatalf("UndoLastXPAward() error = %v", err)
		}
	}
	// Planned again, in the same place, and it can be edited, moved, reached.
	list = tb.master.milestones(t, tb.campaign)
	if len(list) != 2 || list[0].GetId() != a.GetId() || list[0].GetReached() || len(list[0].GetMarks()) != 0 || list[0].GetReachedAt() != nil {
		t.Fatalf("list = %v; want the first planned again, in its place", list)
	}
	if c := tb.players[0].character(t, tb.pcs[0]); c.GetCanLevelUp() {
		t.Error("the first character can still level up after the last undo")
	}
	if _, err := tb.master.xp.UpdateMilestone(t.Context(), connect.NewRequest(&progressionv1.UpdateMilestoneRequest{CampaignId: tb.campaign, MilestoneId: a.GetId(), Text: "Chegar ao Vale"})); err != nil {
		t.Errorf("UpdateMilestone(planned again) error = %v", err)
	}
	tb.master.reach(t, tb.campaign, a.GetId(), tb.ids(2)...)
	// The history keeps every award, the undone ones marked.
	undone := 0
	for _, h := range tb.master.history(t, tb.campaign) {
		if h.GetUndone() {
			undone++
		}
	}
	if undone != 3 {
		t.Errorf("undone awards in the history = %d, want 3", undone)
	}
}

// TestRN20_PlayersSeeOnlyReachedMilestones: a player reads the reached
// milestones, never a planned one. Every response a player gets, as JSON, has
// the reached text and none of the planned text.
func TestRN20_PlayersSeeOnlyReachedMilestones(t *testing.T) {
	t.Parallel()
	tb := newTable(t, milestones, 2)
	ctx := t.Context()
	reached := tb.master.plan(t, tb.campaign, "Chegar ao Vale Seco")
	secrets := []string{"Revelar o traidor da guilda", "Matar a Sombra"}
	var planned []*progressionv1.Milestone
	for _, s := range secrets {
		planned = append(planned, tb.master.plan(t, tb.campaign, s))
	}
	p := tb.players[0]

	var seen []string
	record := func(name string, msg proto.Message, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("%s error = %v", name, err)
		}
		seen = append(seen, protojson.Format(msg))
	}
	readEverything := func() {
		t.Helper()
		list, err := p.xp.ListMilestones(ctx, connect.NewRequest(&progressionv1.ListMilestonesRequest{CampaignId: tb.campaign}))
		record("ListMilestones", list.Msg, err)
		hist, err := p.xp.ListXPAwards(ctx, connect.NewRequest(&progressionv1.ListXPAwardsRequest{CampaignId: tb.campaign}))
		record("ListXPAwards", hist.Msg, err)
		exp, err := p.xp.GetCampaignExperience(ctx, connect.NewRequest(&progressionv1.GetCampaignExperienceRequest{CampaignId: tb.campaign}))
		record("GetCampaignExperience", exp.Msg, err)
		char, err := p.characters.GetCharacter(ctx, connect.NewRequest(&charactersv1.GetCharacterRequest{CampaignId: tb.campaign, CharacterId: tb.pcs[0].GetId()}))
		record("GetCharacter", char.Msg, err)
	}

	// Nothing reached: the list is empty, and does not say that others exist.
	if got := p.milestones(t, tb.campaign); len(got) != 0 {
		t.Fatalf("the player's list = %v with nothing reached, want empty", got)
	}
	readEverything()

	// One reached: the player reads it, with who and when, and nothing else.
	tb.master.reach(t, tb.campaign, reached.GetId(), tb.ids(1)...)
	got := p.milestones(t, tb.campaign)
	if len(got) != 1 || got[0].GetText() != "Chegar ao Vale Seco" || !got[0].GetReached() || got[0].GetReachedAt() == nil ||
		len(got[0].GetMarks()) != 1 || got[0].GetMarks()[0].GetShares()[0].GetCharacterName() != "Pensantus" {
		t.Fatalf("the player's list = %v, want the reached milestone, when and for whom", got)
	}
	if got[0].GetMarks()[0].GetCanUndo() {
		t.Error("the player can undo a mark")
	}
	readEverything()
	reachedReads := len(seen)
	// Undone: it is planned again, and gone from the player's list.
	if _, err := tb.master.undo(tb.campaign, newKey()); err != nil {
		t.Fatalf("UndoLastXPAward() error = %v", err)
	}
	if got := p.milestones(t, tb.campaign); len(got) != 0 {
		t.Errorf("the player's list = %v after the undo, want empty", got)
	}
	readEverything()
	// Positive control: while it was reached, the player's history read its text.
	if !slices.ContainsFunc(seen[:reachedReads], func(s string) bool { return strings.Contains(s, "Chegar ao Vale Seco") }) {
		t.Error("the reached milestone's text is in none of the player's reads, so the check below proves nothing")
	}
	// Planned again, its text is the master's alone: the undone award's reason too.
	for _, s := range seen[reachedReads:] {
		if strings.Contains(s, "Chegar ao Vale Seco") {
			t.Errorf("a player's response has the text of a milestone planned again: %s", s)
		}
	}

	for _, s := range seen {
		for _, secret := range secrets {
			if strings.Contains(s, secret) {
				t.Errorf("a player's response has the planned milestone %q: %s", secret, s)
			}
		}
		for _, m := range planned {
			if strings.Contains(s, m.GetId()) {
				t.Errorf("a player's response has the planned milestone's ID %s: %s", m.GetId(), s)
			}
		}
	}
	// The master still has everything.
	if got := tb.master.milestones(t, tb.campaign); len(got) != 3 {
		t.Errorf("the master's list has %d milestones, want 3", len(got))
	}
	// And a player never writes.
	_, err := p.xp.UpdateMilestone(ctx, connect.NewRequest(&progressionv1.UpdateMilestoneRequest{CampaignId: tb.campaign, MilestoneId: planned[0].GetId(), Text: "x"}))
	wantCode(t, "UpdateMilestone(player)", err, connect.CodePermissionDenied)
}

// TestMR016_PlannedMilestonesNeedAMilestonesCampaign: a campaign that counts
// XP has no milestones to plan, mark or give.
func TestMR016_PlannedMilestonesNeedAMilestonesCampaign(t *testing.T) {
	t.Parallel()
	tb := newTable(t, enemies, 1)
	ctx := t.Context()
	_, err := tb.master.xp.AddMilestone(ctx, connect.NewRequest(&progressionv1.AddMilestoneRequest{CampaignId: tb.campaign, Text: "Chegar ao Vale Seco"}))
	wantBlocked(t, "AddMilestone", err, progressionv1.XPBlockedReason_XP_BLOCKED_REASON_MODE_NOT_ALLOWED)
	_, err = tb.master.xp.MarkMilestoneReached(ctx, connect.NewRequest(&progressionv1.MarkMilestoneReachedRequest{
		CampaignId: tb.campaign, MilestoneId: newKey(), CharacterIds: tb.ids(1), IdempotencyKey: newKey(),
	}))
	wantBlocked(t, "MarkMilestoneReached", err, progressionv1.XPBlockedReason_XP_BLOCKED_REASON_MODE_NOT_ALLOWED)
	_, err = tb.master.giveMilestone(newKey(), tb.campaign, tb.ids(1)...)
	wantBlocked(t, "GiveMilestoneTo", err, progressionv1.XPBlockedReason_XP_BLOCKED_REASON_MODE_NOT_ALLOWED)
	// Every write on a milestone is refused the same way, whatever the ID.
	_, err = tb.master.xp.UpdateMilestone(ctx, connect.NewRequest(&progressionv1.UpdateMilestoneRequest{CampaignId: tb.campaign, MilestoneId: newKey(), Text: "x"}))
	wantBlocked(t, "UpdateMilestone", err, progressionv1.XPBlockedReason_XP_BLOCKED_REASON_MODE_NOT_ALLOWED)
	_, err = tb.master.xp.MoveMilestone(ctx, connect.NewRequest(&progressionv1.MoveMilestoneRequest{CampaignId: tb.campaign, MilestoneId: newKey(), Direction: progressionv1.MilestoneDirection_MILESTONE_DIRECTION_UP}))
	wantBlocked(t, "MoveMilestone", err, progressionv1.XPBlockedReason_XP_BLOCKED_REASON_MODE_NOT_ALLOWED)
	_, err = tb.master.xp.RemoveMilestone(ctx, connect.NewRequest(&progressionv1.RemoveMilestoneRequest{CampaignId: tb.campaign, MilestoneId: newKey()}))
	wantBlocked(t, "RemoveMilestone", err, progressionv1.XPBlockedReason_XP_BLOCKED_REASON_MODE_NOT_ALLOWED)
	if got := tb.master.milestones(t, tb.campaign); len(got) != 0 {
		t.Errorf("an XP campaign lists %v, want none", got)
	}
}

// TestMR016_AMilestoneMarkedOffTheListIsReachedToo: "Registrar um marco fora
// da lista" (MarkMilestone) shows among the reached milestones, for everybody,
// flagged off_list, and is not a planned milestone: nothing can change it.
func TestMR016_AMilestoneMarkedOffTheListIsReachedToo(t *testing.T) {
	t.Parallel()
	tb := newTable(t, milestones, 2)
	planned := tb.master.plan(t, tb.campaign, "Chegar ao Vale Seco")
	tb.master.reach(t, tb.campaign, planned.GetId(), tb.ids(1)...)
	off := tb.master.milestone(t, tb.campaign, tb.ids(2)...) // "Derrotaram o chefe"

	for who, u := range map[string]*user{"master": tb.master, "player": tb.players[1]} {
		list := u.milestones(t, tb.campaign)
		if len(list) != 2 || list[0].GetOffList() || !list[1].GetOffList() || list[1].GetText() != "Derrotaram o chefe" ||
			!list[1].GetReached() || list[1].GetId() != off.GetId() || len(list[1].GetMarks()) != 1 {
			t.Errorf("%s's list = %v, want the planned one and then the one off the list", who, list)
		}
	}
	// It is the award's ID, not a milestone's.
	_, err := tb.master.giveMilestone(off.GetId(), tb.campaign, tb.pcs[0].GetId())
	wantCode(t, "GiveMilestoneTo(off the list)", err, connect.CodeNotFound)
	// Undone, it is gone.
	if _, err := tb.master.undo(tb.campaign, newKey()); err != nil {
		t.Fatalf("UndoLastXPAward() error = %v", err)
	}
	if list := tb.master.milestones(t, tb.campaign); len(list) != 1 || list[0].GetId() != planned.GetId() {
		t.Errorf("list = %v after the undo, want only the planned one", list)
	}
}

// TestMR016_AReusedKeyMustBeTheSameRequest: an idempotency key stands for one
// request. The same one again answers with the award it made; another kind
// (mark or "Dar a mais alguém"), milestone or set of characters is refused, in
// both orders, and nothing is given.
func TestMR016_AReusedKeyMustBeTheSameRequest(t *testing.T) {
	t.Parallel()
	tb := newTable(t, milestones, 3)
	ctx := t.Context()
	r := tb.master.plan(t, tb.campaign, "Chegar ao Vale Seco")
	other := tb.master.plan(t, tb.campaign, "Derrotar a Sombra")
	mark := func(milestone, key string, ids ...string) error {
		_, err := tb.master.xp.MarkMilestoneReached(ctx, connect.NewRequest(&progressionv1.MarkMilestoneReachedRequest{CampaignId: tb.campaign, MilestoneId: milestone, CharacterIds: ids, IdempotencyKey: key}))
		return err
	}
	give := func(milestone, key string, ids ...string) error {
		_, err := tb.master.xp.GiveMilestoneTo(ctx, connect.NewRequest(&progressionv1.GiveMilestoneToRequest{CampaignId: tb.campaign, MilestoneId: milestone, CharacterIds: ids, IdempotencyKey: key}))
		return err
	}
	a, b := tb.pcs[0].GetId(), tb.pcs[1].GetId()

	// Mark first, then the same key for a give (other characters, then the same ones).
	k1 := newKey()
	if err := mark(r.GetId(), k1, a); err != nil {
		t.Fatalf("MarkMilestoneReached() error = %v", err)
	}
	if err := mark(r.GetId(), k1, a); err != nil {
		t.Errorf("the same request again: %v, want the same answer", err)
	}
	wantCode(t, "GiveMilestoneTo(the mark's key, other character)", give(r.GetId(), k1, b), connect.CodeInvalidArgument)
	wantCode(t, "GiveMilestoneTo(the mark's key, same character)", give(r.GetId(), k1, a), connect.CodeInvalidArgument)
	wantCode(t, "MarkMilestoneReached(the key, other characters)", mark(r.GetId(), k1, b), connect.CodeInvalidArgument)
	wantCode(t, "MarkMilestoneReached(the key, other milestone)", mark(other.GetId(), k1, a), connect.CodeInvalidArgument)
	if c := tb.players[1].character(t, tb.pcs[1]); c.GetCanLevelUp() {
		t.Error("B was given the milestone by a reused key")
	}

	// Give first, then the same key for a mark.
	k2 := newKey()
	if err := give(r.GetId(), k2, b); err != nil {
		t.Fatalf("GiveMilestoneTo() error = %v", err)
	}
	if err := give(r.GetId(), k2, b); err != nil {
		t.Errorf("the same give again: %v, want the same answer", err)
	}
	wantCode(t, "MarkMilestoneReached(the give's key)", mark(other.GetId(), k2, b), connect.CodeInvalidArgument)
	wantCode(t, "GiveMilestoneTo(the key, other characters)", give(r.GetId(), k2, tb.pcs[2].GetId()), connect.CodeInvalidArgument)
	if n := len(tb.master.milestones(t, tb.campaign)[0].GetMarks()); n != 2 {
		t.Errorf("marks = %d, want 2 (the retries made none)", n)
	}
}

// TestMR016_AMilestoneWithHistoryIsNotRemoved: awards are never rewritten, and
// removing the milestone would clear their link, so one that was reached once
// stays, planned again after the undo.
func TestMR016_AMilestoneWithHistoryIsNotRemoved(t *testing.T) {
	t.Parallel()
	tb := newTable(t, milestones, 1)
	ctx := t.Context()
	ms := tb.master.plan(t, tb.campaign, "Chegar ao Vale Seco")
	fresh := tb.master.plan(t, tb.campaign, "Voltar à cidade")
	hasHistory := func(id string) bool {
		t.Helper()
		for _, m := range tb.master.milestones(t, tb.campaign) {
			if m.GetId() == id {
				return m.GetHasHistory()
			}
		}
		t.Fatalf("milestone %s is not in the list", id)
		return false
	}
	if hasHistory(ms.GetId()) {
		t.Error("has_history before any mark, want false")
	}
	tb.master.reach(t, tb.campaign, ms.GetId(), tb.ids(1)...)
	if !hasHistory(ms.GetId()) {
		t.Error("has_history of a reached milestone = false, want true")
	}
	if _, err := tb.master.undo(tb.campaign, newKey()); err != nil {
		t.Fatalf("UndoLastXPAward() error = %v", err)
	}
	if !hasHistory(ms.GetId()) || hasHistory(fresh.GetId()) {
		t.Errorf("has_history after the undo = %v (reached once), %v (never reached), want true, false", hasHistory(ms.GetId()), hasHistory(fresh.GetId()))
	}
	_, err := tb.master.xp.RemoveMilestone(ctx, connect.NewRequest(&progressionv1.RemoveMilestoneRequest{CampaignId: tb.campaign, MilestoneId: ms.GetId()}))
	wantBlocked(t, "RemoveMilestone(reached before)", err, progressionv1.XPBlockedReason_XP_BLOCKED_REASON_MILESTONE_HAS_HISTORY)
	var linked int
	if err := tb.h.pool.QueryRow(ctx, `SELECT count(*) FROM xp_awards WHERE milestone_id = $1`, ms.GetId()).Scan(&linked); err != nil || linked != 1 {
		t.Errorf("awards linked to the milestone = %d (%v), want 1", linked, err)
	}
	// It is planned again, and can be reached again.
	tb.master.reach(t, tb.campaign, ms.GetId(), tb.ids(1)...)
}

// TestMR016_TwoMarksAtOnceReachItOnce: two calls to mark the same milestone,
// with different keys, race: exactly one reaches it, the other is refused.
func TestMR016_TwoMarksAtOnceReachItOnce(t *testing.T) {
	t.Parallel()
	dbtest.PoolSize(t, 4) // the racers must overlap: one connection would run them one by one
	tb := newTable(t, milestones, 2)
	ms := tb.master.plan(t, tb.campaign, "Chegar ao Vale Seco")
	errs := make([]error, 2)
	var wg sync.WaitGroup
	start := dbtest.NewBarrier(len(errs))
	for i := range errs {
		wg.Go(func() {
			start.Wait()
			_, errs[i] = tb.master.xp.MarkMilestoneReached(context.Background(), connect.NewRequest(&progressionv1.MarkMilestoneReachedRequest{
				CampaignId: tb.campaign, MilestoneId: ms.GetId(), CharacterIds: []string{tb.pcs[i].GetId()}, IdempotencyKey: newKey(),
			}))
		})
	}
	wg.Wait()
	ok, refused := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case connect.CodeOf(err) == connect.CodeFailedPrecondition:
			wantBlocked(t, "the losing mark", err, progressionv1.XPBlockedReason_XP_BLOCKED_REASON_MILESTONE_ALREADY_REACHED)
			refused++
		default:
			t.Errorf("unexpected error %v", err)
		}
	}
	if ok != 1 || refused != 1 {
		t.Fatalf("ok = %d, refused = %d; want 1 and 1", ok, refused)
	}
	if n := len(tb.master.milestones(t, tb.campaign)[0].GetMarks()); n != 1 {
		t.Errorf("marks = %d, want 1", n)
	}
}

// TestMR016_TwoAddsAtTheLimit: with 99 milestones, two adds at once end at
// exactly 100 and the other is refused.
func TestMR016_TwoAddsAtTheLimit(t *testing.T) {
	t.Parallel()
	dbtest.PoolSize(t, 4) // the racers must overlap: one connection would run them one by one
	tb := newTable(t, milestones, 1)
	for range maxMilestones - 1 {
		tb.master.plan(t, tb.campaign, "Marco")
	}
	errs := make([]error, 2)
	var wg sync.WaitGroup
	start := dbtest.NewBarrier(len(errs))
	for i := range errs {
		wg.Go(func() {
			start.Wait()
			_, errs[i] = tb.master.xp.AddMilestone(context.Background(), connect.NewRequest(&progressionv1.AddMilestoneRequest{CampaignId: tb.campaign, Text: "Último"}))
		})
	}
	wg.Wait()
	ok := 0
	for _, err := range errs {
		if err == nil {
			ok++
		} else if connect.CodeOf(err) != connect.CodeResourceExhausted {
			t.Errorf("unexpected error %v", err)
		}
	}
	if got := len(tb.master.milestones(t, tb.campaign)); ok != 1 || got != maxMilestones {
		t.Fatalf("ok = %d, list = %d; want 1 and %d", ok, got, maxMilestones)
	}
}

// TestMR016_UndoPutsAMilestoneBackInItsPlace: moves made while a milestone was
// reached skip it, and the undo returns it to the place it kept.
func TestMR016_UndoPutsAMilestoneBackInItsPlace(t *testing.T) {
	t.Parallel()
	tb := newTable(t, milestones, 1)
	ctx := t.Context()
	a := tb.master.plan(t, tb.campaign, "A")
	b := tb.master.plan(t, tb.campaign, "B")
	tb.master.plan(t, tb.campaign, "C")
	tb.master.reach(t, tb.campaign, b.GetId(), tb.ids(1)...)
	// A goes down: it skips B and swaps with C.
	if _, err := tb.master.xp.MoveMilestone(ctx, connect.NewRequest(&progressionv1.MoveMilestoneRequest{CampaignId: tb.campaign, MilestoneId: a.GetId(), Direction: progressionv1.MilestoneDirection_MILESTONE_DIRECTION_DOWN})); err != nil {
		t.Fatalf("MoveMilestone() error = %v", err)
	}
	if _, err := tb.master.undo(tb.campaign, newKey()); err != nil {
		t.Fatalf("UndoLastXPAward() error = %v", err)
	}
	if got := texts(tb.master.milestones(t, tb.campaign)); !slices.Equal(got, []string{"C", "B", "A"}) {
		t.Errorf("list = %q, want C, B, A (B kept the middle)", got)
	}
}

// TestMR016_UndoAfterGiveWithAStaleExpectation: the screen showed the first
// mark as the last award; a "Dar a mais alguém" came after, so the undo
// changes nothing and fails with aborted.
func TestMR016_UndoAfterGiveWithAStaleExpectation(t *testing.T) {
	t.Parallel()
	tb := newTable(t, milestones, 2)
	ms := tb.master.plan(t, tb.campaign, "Chegar ao Vale Seco")
	first := tb.master.reach(t, tb.campaign, ms.GetId(), tb.pcs[0].GetId()).GetAward()
	if _, err := tb.master.giveMilestone(ms.GetId(), tb.campaign, tb.pcs[1].GetId()); err != nil {
		t.Fatalf("GiveMilestoneTo() error = %v", err)
	}
	_, err := tb.master.xp.UndoLastXPAward(t.Context(), connect.NewRequest(&progressionv1.UndoLastXPAwardRequest{CampaignId: tb.campaign, IdempotencyKey: newKey(), ExpectedAwardId: first.GetId()}))
	wantCode(t, "UndoLastXPAward(stale)", err, connect.CodeAborted)
	if n := len(tb.master.milestones(t, tb.campaign)[0].GetMarks()); n != 2 {
		t.Errorf("marks = %d, want 2 (nothing was undone)", n)
	}
}
