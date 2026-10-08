package progression

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	progressionv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/progression/v1"
)

// The acceptance criteria of MR-016 (docs/product/stories.md): the master
// gives XP by defeated enemies, by gold or by hand, marks milestones, and
// everybody reads the history. Each test starts its own database.

const (
	enemies    = campaignsv1.XpMode_XP_MODE_ENEMIES
	gold       = campaignsv1.XpMode_XP_MODE_GOLD
	milestones = campaignsv1.XpMode_XP_MODE_MILESTONES
)

// table is a campaign with a master and n players, each with a character.
type table struct {
	h        *harness
	master   *user
	players  []*user
	pcs      []*charactersv1.Character
	campaign string
}

func newTable(t *testing.T, mode campaignsv1.XpMode, n int) *table {
	t.Helper()
	h := newHarness(t)
	tb := &table{h: h, master: h.newUser("Samuel")}
	names := []string{"Pensantus", "Toren", "Brisa", "Lia"}
	for i := range n {
		tb.players = append(tb.players, h.newUser("Jogador "+names[i]))
	}
	tb.campaign = h.newCampaign(tb.master, "Mirathel", mode, tb.players...)
	for i, p := range tb.players {
		tb.pcs = append(tb.pcs, p.pc(t, tb.campaign, names[i]))
	}
	return tb
}

func (tb *table) ids(n int) []string {
	var ids []string
	for _, c := range tb.pcs[:n] {
		ids = append(ids, c.GetId())
	}
	return ids
}

// newMap inserts a gallery image and a map with a grid in the campaign,
// straight into the tables (the maps module's own tests cover uploading), and
// returns the map's ID.
func (tb *table) newMap() string {
	tb.h.t.Helper()
	imageID, mapID := newKey(), newKey()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO gallery_images (id, campaign_id, name, content_type, width, height, byte_size, created_at)
		  VALUES ($1, $2, 'Mapa', 'image/png', 1000, 500, 100, now())`, []any{imageID, tb.campaign}},
		{`INSERT INTO maps (id, campaign_id, name, image_id, grid_columns, created_at, updated_at)
		  VALUES ($1, $2, 'Emboscada', $3, 20, now(), now())`, []any{mapID, tb.campaign, imageID}},
	} {
		if _, err := tb.h.pool.Exec(tb.h.t.Context(), q.sql, q.args...); err != nil {
			tb.h.t.Fatalf("insert map: %v", err)
		}
	}
	return mapID
}

// fight starts a session (if none is open), a combat with the party and count
// copies of the goblin, defeats the first defeated of them and, when end is
// true, ends it. It returns the combat and its combatants as the master sees
// them.
func (tb *table) fight(t *testing.T, goblin *charactersv1.Character, count, defeated int32, end bool) *playv1.Encounter {
	t.Helper()
	ctx := t.Context()
	if _, err := tb.master.play.GetLiveSession(ctx, connect.NewRequest(&playv1.GetLiveSessionRequest{CampaignId: tb.campaign})); err != nil {
		if _, err := tb.master.play.StartGameSession(ctx, connect.NewRequest(&playv1.StartGameSessionRequest{CampaignId: tb.campaign})); err != nil {
			t.Fatalf("StartGameSession() error = %v", err)
		}
		if _, err := tb.master.play.SetCurrentMap(ctx, connect.NewRequest(&playv1.SetCurrentMapRequest{CampaignId: tb.campaign, MapId: tb.newMap()})); err != nil {
			t.Fatalf("SetCurrentMap() error = %v", err)
		}
	}
	hidden := false
	res, err := tb.master.combat.StartEncounter(ctx, connect.NewRequest(&playv1.StartEncounterRequest{
		CampaignId: tb.campaign, IdempotencyKey: newKey(), Name: "Emboscada na estrada",
		Participants: []*playv1.Participant{{CharacterId: goblin.GetId(), Count: count, Hidden: &hidden}},
	}))
	if err != nil {
		t.Fatalf("StartEncounter() error = %v", err)
	}
	enc := res.Msg.GetEncounter()
	var npcs []*playv1.Combatant
	for _, c := range enc.GetCombatants() {
		if c.GetKind() == playv1.CombatantKind_COMBATANT_KIND_NPC {
			npcs = append(npcs, c)
		}
	}
	for _, c := range npcs[:defeated] {
		if _, err := tb.master.combat.AdjustCombatantHitPoints(ctx, connect.NewRequest(&playv1.AdjustCombatantHitPointsRequest{
			CampaignId: tb.campaign, EncounterId: enc.GetId(), CombatantId: c.GetId(), IdempotencyKey: newKey(),
			Change: &playv1.AdjustCombatantHitPointsRequest_HitPoints{HitPoints: 0},
		})); err != nil {
			t.Fatalf("AdjustCombatantHitPoints() error = %v", err)
		}
	}
	if end {
		out, err := tb.master.combat.EndEncounter(ctx, connect.NewRequest(&playv1.EndEncounterRequest{CampaignId: tb.campaign, EncounterId: enc.GetId(), IdempotencyKey: newKey()}))
		if err != nil {
			t.Fatalf("EndEncounter() error = %v", err)
		}
		enc = out.Msg.GetEncounter()
	}
	return enc
}

func (tb *table) enemies(encounterID string, ids ...string) (*progressionv1.AwardXPResponse, error) {
	return tb.master.award(tb.campaign, func(r *progressionv1.AwardXPRequest) {
		r.Mode, r.EncounterId, r.CharacterIds = progressionv1.XPAwardMode_XP_AWARD_MODE_ENEMIES, encounterID, ids
	})
}

// events counts the session_events rows of a kind.
func (tb *table) events(t *testing.T, kind string) int {
	t.Helper()
	var n int
	if err := tb.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM session_events WHERE kind = $1`, kind).Scan(&n); err != nil {
		t.Fatalf("count events: %v", err)
	}
	return n
}

// TestMR016_EnemiesAwardSplitsTheDefeated: two goblins (50 XP each) defeated
// and four player characters: each gets 25 XP, the sheets show it, the history
// says who gave it, the session records it and the streams hear about it.
func TestMR016_EnemiesAwardSplitsTheDefeated(t *testing.T) {
	t.Parallel()
	tb := newTable(t, enemies, 4)
	goblin := tb.master.minion(t, tb.campaign, "Goblin", 50)
	enc := tb.fight(t, goblin, 2, 2, true)

	// The stream of a player hears of it (content-free).
	watchCtx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	stream, err := tb.players[0].play.WatchGameSession(watchCtx, connect.NewRequest(&playv1.WatchGameSessionRequest{CampaignId: tb.campaign}))
	if err != nil {
		t.Fatalf("WatchGameSession() error = %v", err)
	}
	defer func() { _ = stream.Close() }()
	if !stream.Receive() || stream.Msg().GetReady() == nil {
		t.Fatalf("the first event = %v, %v; want ready", stream.Msg(), stream.Err())
	}

	res, err := tb.enemies(enc.GetId(), tb.ids(4)...)
	if err != nil {
		t.Fatalf("AwardXP(enemies) error = %v", err)
	}
	if res.GetXpEach() != 25 || res.GetLostXp() != 0 || res.GetAward().GetTotalXp() != 100 {
		t.Errorf("award = each %d, lost %d, total %d; want 25, 0, 100", res.GetXpEach(), res.GetLostXp(), res.GetAward().GetTotalXp())
	}
	if got := res.GetAward().GetEncounterName(); got != "Emboscada na estrada" {
		t.Errorf("encounter name = %q", got)
	}
	for i, pc := range tb.pcs {
		if got := tb.master.xpOf(t, pc); got != 25 {
			t.Errorf("sheet XP of player %d = %d, want 25", i, got)
		}
	}
	// A player reads the same on their own sheet, even though it is locked.
	if got := tb.players[1].character(t, tb.pcs[1]); got.GetState() != charactersv1.CharacterState_CHARACTER_STATE_LOCKED || got.GetSheet().GetFull().GetExperiencePoints() != 25 {
		t.Errorf("a player's locked sheet = state %v, XP %d; want LOCKED, 25", got.GetState(), got.GetSheet().GetFull().GetExperiencePoints())
	}
	if n := tb.events(t, "xp_awarded"); n != 1 {
		t.Errorf("xp_awarded events = %d, want 1", n)
	}
	for stream.Receive() {
		if stream.Msg().GetXpChanged() != nil {
			return
		}
	}
	t.Fatalf("the player's stream never got xp_changed (err = %v)", stream.Err())
}

// TestMR016_MilestoneMarksWithoutXP: a milestone marks everyone it names "can
// level up", counts no XP, and the mark goes away when the sheet's level goes
// up.
func TestMR016_MilestoneMarksWithoutXP(t *testing.T) {
	t.Parallel()
	tb := newTable(t, milestones, 3)
	award := tb.master.milestone(t, tb.campaign, tb.ids(2)...) // the third did not play
	if award.GetMode() != progressionv1.XPAwardMode_XP_AWARD_MODE_MILESTONE || award.GetTotalXp() != 0 || len(award.GetShares()) != 2 {
		t.Fatalf("award = %v; want a milestone with 0 XP and 2 characters", award)
	}
	for _, sh := range award.GetShares() {
		if sh.GetXp() != 0 {
			t.Errorf("share XP = %d, want 0", sh.GetXp())
		}
	}

	exp := tb.master.experience(t, tb.campaign)
	if exp.GetXpMode() != milestones {
		t.Errorf("xp_mode = %v, want milestones", exp.GetXpMode())
	}
	for i, c := range exp.GetCharacters() {
		wantUp := i < 2
		if c.GetCanLevelUp() != wantUp || (wantUp && c.GetLevelUpReason() != charactersv1.LevelUpReason_LEVEL_UP_REASON_MILESTONE) {
			t.Errorf("%s: can level up = %v (%v), want %v by milestone", c.GetName(), c.GetCanLevelUp(), c.GetLevelUpReason(), wantUp)
		}
		if c.GetExperiencePoints() != 0 {
			t.Errorf("%s: XP = %d, a milestone counts none", c.GetName(), c.GetExperiencePoints())
		}
	}
	// The sheet says it to the master and to its player, not to the other player.
	for who, u := range map[string]*user{"master": tb.master, "owner": tb.players[0]} {
		if c := u.character(t, tb.pcs[0]); !c.GetCanLevelUp() || c.GetLevelUpReason() != charactersv1.LevelUpReason_LEVEL_UP_REASON_MILESTONE {
			t.Errorf("%s reads can_level_up = %v (%v), want true by milestone", who, c.GetCanLevelUp(), c.GetLevelUpReason())
		}
	}
	if c := tb.players[2].character(t, tb.pcs[2]); c.GetCanLevelUp() {
		t.Error("the unmarked character can level up")
	}

	// The master raises Pensantus to level 2: the mark goes, Toren's stays.
	pens := tb.master.character(t, tb.pcs[0])
	pens.GetSheet().GetFull().GetClasses()[0].Level = 2
	if _, err := tb.master.characters.UpdateCharacter(t.Context(), connect.NewRequest(&charactersv1.UpdateCharacterRequest{
		CampaignId: tb.campaign, CharacterId: pens.GetId(), Revision: pens.GetRevision(), Name: pens.GetName(), Sheet: pens.GetSheet(),
	})); err != nil {
		t.Fatalf("UpdateCharacter(level 2) error = %v", err)
	}
	if c := tb.players[0].character(t, tb.pcs[0]); c.GetCanLevelUp() {
		t.Error("Pensantus still can level up after the level went up")
	}
	if c := tb.players[1].character(t, tb.pcs[1]); !c.GetCanLevelUp() {
		t.Error("Toren lost the mark without a level up")
	}
}

// TestMR016_ManualAwardIsInTheHistory: who gave it, when, why and how much, for
// every member to read, newest first.
func TestMR016_ManualAwardIsInTheHistory(t *testing.T) {
	t.Parallel()
	tb := newTable(t, enemies, 2)
	before := time.Now().Add(-time.Minute)
	first := tb.master.manual(t, tb.campaign, 40, tb.ids(2)...)
	second, err := tb.master.award(tb.campaign, func(r *progressionv1.AwardXPRequest) {
		r.Mode, r.Amount, r.Reason, r.CharacterIds = progressionv1.XPAwardMode_XP_AWARD_MODE_MANUAL, 10, "Boa interpretação", tb.ids(1)
	})
	if err != nil {
		t.Fatalf("AwardXP() error = %v", err)
	}
	if first.GetXpEach() != 20 || tb.master.xpOf(t, tb.pcs[0]) != 30 || tb.master.xpOf(t, tb.pcs[1]) != 20 {
		t.Errorf("XP = %d each, sheets %d and %d; want 20, 30 and 20", first.GetXpEach(), tb.master.xpOf(t, tb.pcs[0]), tb.master.xpOf(t, tb.pcs[1]))
	}

	// A player reads it too (question 50).
	got := tb.players[1].history(t, tb.campaign)
	if len(got) != 2 || got[0].GetId() != second.GetAward().GetId() || got[1].GetId() != first.GetAward().GetId() {
		t.Fatalf("history = %v; want the two awards, newest first", got)
	}
	a := got[0]
	if a.GetGivenByDisplayName() != "Samuel" || a.GetGivenByUserId() != tb.master.id || a.GetReason() != "Boa interpretação" ||
		a.GetTotalXp() != 10 || a.GetMode() != progressionv1.XPAwardMode_XP_AWARD_MODE_MANUAL ||
		len(a.GetShares()) != 1 || a.GetShares()[0].GetCharacterName() != "Pensantus" || a.GetShares()[0].GetXp() != 10 {
		t.Errorf("entry = %v; want who, why, how much and to whom", a)
	}
	if !a.GetCreatedAt().AsTime().After(before) {
		t.Errorf("created_at = %v, want a time just now", a.GetCreatedAt().AsTime())
	}
	if a.GetCanUndo() {
		t.Error("a player can undo an award")
	}
	if !tb.master.history(t, tb.campaign)[0].GetCanUndo() || tb.master.history(t, tb.campaign)[1].GetCanUndo() {
		t.Error("only the latest award has can_undo, for the master")
	}
}

// TestGoldAwardGivesOneXPPerGoldPiece: 120 gold pieces among three characters
// is 40 XP each (RN-09).
func TestGoldAwardGivesOneXPPerGoldPiece(t *testing.T) {
	t.Parallel()
	tb := newTable(t, gold, 3)
	res, err := tb.master.award(tb.campaign, func(r *progressionv1.AwardXPRequest) {
		r.Mode, r.Gold, r.CharacterIds = progressionv1.XPAwardMode_XP_AWARD_MODE_GOLD, 120, tb.ids(3)
	})
	if err != nil {
		t.Fatalf("AwardXP(gold) error = %v", err)
	}
	if res.GetXpEach() != 40 || res.GetLostXp() != 0 || res.GetAward().GetGold() != 120 || res.GetAward().GetTotalXp() != 120 {
		t.Errorf("award = each %d, lost %d, gold %d, total %d; want 40, 0, 120, 120", res.GetXpEach(), res.GetLostXp(), res.GetAward().GetGold(), res.GetAward().GetTotalXp())
	}
	for _, pc := range tb.pcs {
		if got := tb.master.xpOf(t, pc); got != 40 {
			t.Errorf("sheet XP = %d, want 40", got)
		}
	}
}

// TestRemainderIsLostAndReported: 100 XP among three is 33 each and 1 is lost
// (question 46).
func TestRemainderIsLostAndReported(t *testing.T) {
	t.Parallel()
	tb := newTable(t, enemies, 3)
	res := tb.master.manual(t, tb.campaign, 100, tb.ids(3)...)
	if res.GetXpEach() != 33 || res.GetLostXp() != 1 || res.GetAward().GetTotalXp() != 100 {
		t.Errorf("award = each %d, lost %d, total %d; want 33, 1, 100", res.GetXpEach(), res.GetLostXp(), res.GetAward().GetTotalXp())
	}
	// Too little to give each one 1 XP is nothing to give.
	_, err := tb.master.award(tb.campaign, func(r *progressionv1.AwardXPRequest) {
		r.Mode, r.Amount, r.CharacterIds = progressionv1.XPAwardMode_XP_AWARD_MODE_MANUAL, 2, tb.ids(3)
	})
	wantBlocked(t, "AwardXP(2 among 3)", err, progressionv1.XPBlockedReason_XP_BLOCKED_REASON_NOTHING_TO_GIVE)
}

// TestAwardIsIdempotent: the same key again changes nothing and answers with
// the award it made; the key of another kind of change is refused.
func TestAwardIsIdempotent(t *testing.T) {
	t.Parallel()
	tb := newTable(t, enemies, 2)
	key := newKey()
	send := func() (*progressionv1.AwardXPResponse, error) {
		return tb.master.award(tb.campaign, func(r *progressionv1.AwardXPRequest) {
			r.Mode, r.Amount, r.CharacterIds, r.IdempotencyKey = progressionv1.XPAwardMode_XP_AWARD_MODE_MANUAL, 50, tb.ids(2), key
		})
	}
	first, err := send()
	if err != nil {
		t.Fatalf("AwardXP() error = %v", err)
	}
	again, err := send()
	if err != nil {
		t.Fatalf("AwardXP(retry) error = %v", err)
	}
	if again.GetAward().GetId() != first.GetAward().GetId() || again.GetXpEach() != 25 {
		t.Errorf("retry = %v, want the first award with 25 each", again)
	}
	if got := tb.master.xpOf(t, tb.pcs[0]); got != 25 {
		t.Errorf("sheet XP = %d after a retry, want 25", got)
	}
	if n := len(tb.master.history(t, tb.campaign)); n != 1 {
		t.Errorf("history has %d awards, want 1", n)
	}
	_, err = tb.master.award(tb.campaign, func(r *progressionv1.AwardXPRequest) {
		r.Mode, r.Gold, r.CharacterIds, r.IdempotencyKey = progressionv1.XPAwardMode_XP_AWARD_MODE_GOLD, 10, tb.ids(2), key
	})
	wantCode(t, "AwardXP(the key of another change)", err, connect.CodeInvalidArgument)
}

// TestAwardKeyReusedForAnotherRequestIsRefused: a key kept with the hash of the whole request
// refuses the same key for a request that differs in the amount, the characters, the reason or
// the gold, whatever the mode, and changes nothing; the same request, with its characters
// in another order, is still the retry.
func TestAwardKeyReusedForAnotherRequestIsRefused(t *testing.T) {
	t.Parallel()
	t.Run("manual", func(t *testing.T) {
		t.Parallel()
		tb := newTable(t, enemies, 3)
		key := newKey()
		send := func(amount int32, reason string, ids ...string) error {
			_, err := tb.master.award(tb.campaign, func(r *progressionv1.AwardXPRequest) {
				r.Mode, r.Amount, r.Reason, r.CharacterIds, r.IdempotencyKey = progressionv1.XPAwardMode_XP_AWARD_MODE_MANUAL, amount, reason, ids, key
			})
			return err
		}
		a, b, c := tb.pcs[0].GetId(), tb.pcs[1].GetId(), tb.pcs[2].GetId()
		if err := send(50, "Ponte", a, b); err != nil {
			t.Fatalf("AwardXP() error = %v", err)
		}
		if err := send(50, "Ponte", b, a); err != nil {
			t.Errorf("AwardXP(retry, characters in another order) error = %v, want the first award", err)
		}
		wantCode(t, "AwardXP(reused key, other amount)", send(500, "Ponte", a, b), connect.CodeInvalidArgument)
		wantCode(t, "AwardXP(reused key, other characters)", send(50, "Ponte", c), connect.CodeInvalidArgument)
		wantCode(t, "AwardXP(reused key, other amount and characters)", send(500, "Ponte", c), connect.CodeInvalidArgument)
		wantCode(t, "AwardXP(reused key, other reason)", send(50, "Outra", a, b), connect.CodeInvalidArgument)
		if got := tb.master.xpOf(t, tb.pcs[2]); got != 0 {
			t.Errorf("third character's XP = %d, want 0", got)
		}
		if n := len(tb.master.history(t, tb.campaign)); n != 1 {
			t.Errorf("history has %d awards, want 1", n)
		}
	})
	t.Run("gold", func(t *testing.T) {
		t.Parallel()
		tb := newTable(t, gold, 2)
		key := newKey()
		send := func(gold int32) error {
			_, err := tb.master.award(tb.campaign, func(r *progressionv1.AwardXPRequest) {
				r.Mode, r.Gold, r.CharacterIds, r.IdempotencyKey = progressionv1.XPAwardMode_XP_AWARD_MODE_GOLD, gold, tb.ids(2), key
			})
			return err
		}
		if err := send(100); err != nil {
			t.Fatalf("AwardXP() error = %v", err)
		}
		if err := send(100); err != nil {
			t.Errorf("AwardXP(retry) error = %v, want the first award", err)
		}
		wantCode(t, "AwardXP(reused key, other gold)", send(400), connect.CodeInvalidArgument)
		if got := tb.master.xpOf(t, tb.pcs[0]); got != 50 {
			t.Errorf("character's XP = %d, want 50", got)
		}
	})
}

// TestUndoTakesBackOnlyTheLastAward: the last one only, never below 0, with a
// retry that does nothing twice, and the award stays in the history as undone.
func TestUndoTakesBackOnlyTheLastAward(t *testing.T) {
	t.Parallel()
	tb := newTable(t, enemies, 2)
	_, err := tb.master.undo(tb.campaign, newKey())
	wantBlocked(t, "UndoLastXPAward(no award)", err, progressionv1.XPBlockedReason_XP_BLOCKED_REASON_NOTHING_TO_UNDO)

	first := tb.master.manual(t, tb.campaign, 100, tb.ids(2)...) // 50 each
	second := tb.master.manual(t, tb.campaign, 60, tb.pcs[0].GetId())

	// The app shows the first as the last one, but a newer award exists.
	_, err = tb.master.xp.UndoLastXPAward(t.Context(), connect.NewRequest(&progressionv1.UndoLastXPAwardRequest{
		CampaignId: tb.campaign, IdempotencyKey: newKey(), ExpectedAwardId: first.GetAward().GetId(),
	}))
	wantCode(t, "UndoLastXPAward(stale expected_award_id)", err, connect.CodeAborted)
	// A player cannot.
	_, err = tb.players[0].undo(tb.campaign, newKey())
	wantCode(t, "UndoLastXPAward(player)", err, connect.CodePermissionDenied)

	key := newKey()
	undone, err := tb.master.undo(tb.campaign, key)
	if err != nil {
		t.Fatalf("UndoLastXPAward() error = %v", err)
	}
	if undone.GetId() != second.GetAward().GetId() || !undone.GetUndone() || undone.GetUndoneByDisplayName() != "Samuel" || undone.GetUndoneAt() == nil {
		t.Errorf("undone = %v; want the second award, undone by Samuel", undone)
	}
	if got := tb.master.xpOf(t, tb.pcs[0]); got != 50 {
		t.Errorf("sheet XP = %d after the undo, want 50", got)
	}
	// A retry of the same undo does not take back the first award too.
	if _, err := tb.master.undo(tb.campaign, key); err != nil {
		t.Fatalf("UndoLastXPAward(retry) error = %v", err)
	}
	if got := tb.master.xpOf(t, tb.pcs[0]); got != 50 {
		t.Errorf("sheet XP = %d after a retry of the undo, want 50", got)
	}
	// The history keeps both; only the first can be undone now.
	h := tb.master.history(t, tb.campaign)
	if len(h) != 2 || !h[0].GetUndone() || h[1].GetUndone() || h[0].GetCanUndo() || !h[1].GetCanUndo() {
		t.Errorf("history = %v; want the second undone and only the first undoable", h)
	}

	// The master lowered Toren's XP by hand: the undo never goes below 0.
	toren := tb.master.character(t, tb.pcs[1])
	toren.GetSheet().GetFull().ExperiencePoints = 20
	if _, err := tb.master.characters.UpdateCharacter(t.Context(), connect.NewRequest(&charactersv1.UpdateCharacterRequest{
		CampaignId: tb.campaign, CharacterId: toren.GetId(), Revision: toren.GetRevision(), Name: toren.GetName(), Sheet: toren.GetSheet(),
	})); err != nil {
		t.Fatalf("UpdateCharacter() error = %v", err)
	}
	if _, err := tb.master.undo(tb.campaign, newKey()); err != nil {
		t.Fatalf("UndoLastXPAward(first) error = %v", err)
	}
	if a, b := tb.master.xpOf(t, tb.pcs[0]), tb.master.xpOf(t, tb.pcs[1]); a != 0 || b != 0 {
		t.Errorf("sheet XP = %d and %d after taking back 50, want 0 and 0", a, b)
	}
	_, err = tb.master.undo(tb.campaign, newKey())
	wantBlocked(t, "UndoLastXPAward(all undone)", err, progressionv1.XPBlockedReason_XP_BLOCKED_REASON_NOTHING_TO_UNDO)
	if n := tb.events(t, "xp_award_undone"); n != 0 {
		t.Errorf("xp_award_undone events = %d with no session open, want 0", n)
	}
}

// TestUndoMilestoneClearsTheMarks: undoing a milestone clears the tag.
func TestUndoMilestoneClearsTheMarks(t *testing.T) {
	t.Parallel()
	tb := newTable(t, milestones, 1)
	tb.master.milestone(t, tb.campaign, tb.ids(1)...)
	if !tb.master.experience(t, tb.campaign).GetCharacters()[0].GetCanLevelUp() {
		t.Fatal("the milestone did not mark the character")
	}
	if _, err := tb.master.undo(tb.campaign, newKey()); err != nil {
		t.Fatalf("UndoLastXPAward() error = %v", err)
	}
	if c := tb.master.experience(t, tb.campaign).GetCharacters()[0]; c.GetCanLevelUp() {
		t.Error("the character can still level up after the undo")
	}
	if c := tb.players[0].character(t, tb.pcs[0]); c.GetCanLevelUp() {
		t.Error("the sheet still says can_level_up after the undo")
	}
}

// TestSecondEnemiesAwardForTheSameEncounterIsRefused: a combat's XP is given
// once, until the award is undone.
func TestSecondEnemiesAwardForTheSameEncounterIsRefused(t *testing.T) {
	t.Parallel()
	tb := newTable(t, enemies, 2)
	goblin := tb.master.minion(t, tb.campaign, "Goblin", 50)
	open := tb.fight(t, goblin, 2, 1, false)

	_, err := tb.enemies(open.GetId(), tb.ids(2)...)
	wantBlocked(t, "AwardXP(combat not ended)", err, progressionv1.XPBlockedReason_XP_BLOCKED_REASON_ENCOUNTER_NOT_ENDED)
	_, err = tb.enemies(newKey(), tb.ids(2)...)
	wantCode(t, "AwardXP(no such combat)", err, connect.CodeNotFound)

	if _, err := tb.master.combat.EndEncounter(t.Context(), connect.NewRequest(&playv1.EndEncounterRequest{CampaignId: tb.campaign, EncounterId: open.GetId(), IdempotencyKey: newKey()})); err != nil {
		t.Fatalf("EndEncounter() error = %v", err)
	}
	res, err := tb.enemies(open.GetId(), tb.ids(2)...) // one goblin defeated: 50 XP
	if err != nil {
		t.Fatalf("AwardXP(enemies) error = %v", err)
	}
	if res.GetXpEach() != 25 {
		t.Errorf("XP each = %d, want 25 (one goblin of 50 XP defeated)", res.GetXpEach())
	}
	_, err = tb.enemies(open.GetId(), tb.ids(2)...)
	wantBlocked(t, "AwardXP(same combat again)", err, progressionv1.XPBlockedReason_XP_BLOCKED_REASON_ALREADY_AWARDED)

	if _, err := tb.master.undo(tb.campaign, newKey()); err != nil {
		t.Fatalf("UndoLastXPAward() error = %v", err)
	}
	if _, err := tb.enemies(open.GetId(), tb.ids(1)...); err != nil {
		t.Errorf("AwardXP(enemies, after the undo) error = %v, want allowed", err)
	}
}

// TestAwardRefusesAnotherCampaignsEncounter: a combat of another campaign of
// the same master is not found.
func TestAwardRefusesAnotherCampaignsEncounter(t *testing.T) {
	t.Parallel()
	tb := newTable(t, enemies, 1)
	other := newTable(t, enemies, 1)
	goblin := other.master.minion(t, other.campaign, "Goblin", 50)
	enc := other.fight(t, goblin, 1, 1, true)
	_, err := tb.enemies(enc.GetId(), tb.ids(1)...)
	wantCode(t, "AwardXP(another campaign's combat)", err, connect.CodeNotFound)
}

// TestModeMustFitTheCampaign: enemies campaigns take enemies and manual
// awards, gold campaigns gold and manual, milestones campaigns only
// milestones (RN-09).
func TestModeMustFitTheCampaign(t *testing.T) {
	t.Parallel()
	byGold := func(r *progressionv1.AwardXPRequest) {
		r.Mode, r.Gold = progressionv1.XPAwardMode_XP_AWARD_MODE_GOLD, 100
	}
	byEnemies := func(r *progressionv1.AwardXPRequest) {
		r.Mode, r.EncounterId = progressionv1.XPAwardMode_XP_AWARD_MODE_ENEMIES, newKey()
	}
	byHand := func(r *progressionv1.AwardXPRequest) {
		r.Mode, r.Amount = progressionv1.XPAwardMode_XP_AWARD_MODE_MANUAL, 100
	}
	for _, c := range []struct {
		name   string
		mode   campaignsv1.XpMode
		award  func(*progressionv1.AwardXPRequest)
		allows bool // false: refused as MODE_NOT_ALLOWED
	}{
		{"gold in an enemies campaign", enemies, byGold, false},
		{"milestone in an enemies campaign", enemies, nil, false},
		{"enemies in a gold campaign", gold, byEnemies, false},
		{"manual in a gold campaign", gold, byHand, true},
		{"manual in a milestones campaign", milestones, byHand, false},
		{"gold in a milestones campaign", milestones, byGold, false},
		{"enemies in a milestones campaign", milestones, byEnemies, false},
		{"manual in an enemies campaign", enemies, byHand, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			tb := newTable(t, c.mode, 1)
			var err error
			if c.award == nil {
				_, err = tb.master.xp.MarkMilestone(t.Context(), connect.NewRequest(&progressionv1.MarkMilestoneRequest{
					CampaignId: tb.campaign, Reason: "Marco", CharacterIds: tb.ids(1), IdempotencyKey: newKey(),
				}))
			} else {
				_, err = tb.master.award(tb.campaign, func(r *progressionv1.AwardXPRequest) {
					c.award(r)
					r.CharacterIds = tb.ids(1)
				})
			}
			if c.allows {
				if err != nil {
					t.Errorf("error = %v, want allowed", err)
				}
				return
			}
			wantBlocked(t, c.name, err, progressionv1.XPBlockedReason_XP_BLOCKED_REASON_MODE_NOT_ALLOWED)
		})
	}
}

// TestOnlyLivingPlayerCharactersGetXP: a dead character, an NPC, a pending
// member's character and another campaign's character are refused, and the
// request is checked: no repeat, 1 to 40 characters, a reason.
func TestOnlyLivingPlayerCharactersGetXP(t *testing.T) {
	t.Parallel()
	tb := newTable(t, enemies, 2)
	npc := tb.master.minion(t, tb.campaign, "Goblin", 50)
	other := newTable(t, enemies, 1)
	pending := tb.h.newUser("Pendente")
	tb.h.joinPending(tb.master, tb.campaign, pending)
	pendingPC := pending.pc(t, tb.campaign, "Esperando")

	if _, err := tb.master.characters.MarkCharacterDead(t.Context(), connect.NewRequest(&charactersv1.MarkCharacterDeadRequest{CampaignId: tb.campaign, CharacterId: tb.pcs[1].GetId()})); err != nil {
		t.Fatalf("MarkCharacterDead() error = %v", err)
	}
	for name, id := range map[string]string{
		"a dead character": tb.pcs[1].GetId(), "an NPC": npc.GetId(), "a pending member's character": pendingPC.GetId(),
		"another campaign's character": other.pcs[0].GetId(), "a character that does not exist": newKey(),
	} {
		_, err := tb.master.award(tb.campaign, func(r *progressionv1.AwardXPRequest) {
			r.Mode, r.Amount, r.CharacterIds = progressionv1.XPAwardMode_XP_AWARD_MODE_MANUAL, 100, []string{tb.pcs[0].GetId(), id}
		})
		wantBlocked(t, "AwardXP("+name+")", err, progressionv1.XPBlockedReason_XP_BLOCKED_REASON_CHARACTER_NOT_ELIGIBLE)
	}
	if got := tb.master.xpOf(t, tb.pcs[0]); got != 0 {
		t.Errorf("sheet XP = %d after refused awards, want 0 (nothing is half done)", got)
	}

	for name, edit := range map[string]func(*progressionv1.AwardXPRequest){
		"no characters":    func(r *progressionv1.AwardXPRequest) { r.CharacterIds = nil },
		"a repeat":         func(r *progressionv1.AwardXPRequest) { r.CharacterIds = []string{tb.pcs[0].GetId(), tb.pcs[0].GetId()} },
		"not a UUID":       func(r *progressionv1.AwardXPRequest) { r.CharacterIds = []string{"pensantus"} },
		"no reason":        func(r *progressionv1.AwardXPRequest) { r.Reason = "  " },
		"a long reason":    func(r *progressionv1.AwardXPRequest) { r.Reason = strings.Repeat("a", 121) },
		"no amount":        func(r *progressionv1.AwardXPRequest) { r.Amount = 0 },
		"a huge amount":    func(r *progressionv1.AwardXPRequest) { r.Amount = 1_000_001 },
		"gold for manual":  func(r *progressionv1.AwardXPRequest) { r.Gold = 5 },
		"a bad key":        func(r *progressionv1.AwardXPRequest) { r.IdempotencyKey = "1" },
		"no mode":          func(r *progressionv1.AwardXPRequest) { r.Mode = progressionv1.XPAwardMode_XP_AWARD_MODE_UNSPECIFIED },
		"a milestone mode": func(r *progressionv1.AwardXPRequest) { r.Mode = progressionv1.XPAwardMode_XP_AWARD_MODE_MILESTONE },
	} {
		_, err := tb.master.award(tb.campaign, func(r *progressionv1.AwardXPRequest) {
			r.Mode, r.Amount, r.CharacterIds = progressionv1.XPAwardMode_XP_AWARD_MODE_MANUAL, 100, tb.ids(1)
			edit(r)
		})
		wantCode(t, "AwardXP("+name+")", err, connect.CodeInvalidArgument)
	}
}

// TestPlayersNeverWrite: a player calls none of the three writes.
func TestPlayersNeverWrite(t *testing.T) {
	t.Parallel()
	tb := newTable(t, enemies, 1)
	p := tb.players[0]
	_, err := p.award(tb.campaign, func(r *progressionv1.AwardXPRequest) {
		r.Mode, r.Amount, r.CharacterIds = progressionv1.XPAwardMode_XP_AWARD_MODE_MANUAL, 100, tb.ids(1)
	})
	wantCode(t, "AwardXP", err, connect.CodePermissionDenied)
	_, err = p.xp.MarkMilestone(t.Context(), connect.NewRequest(&progressionv1.MarkMilestoneRequest{CampaignId: tb.campaign, Reason: "x", CharacterIds: tb.ids(1), IdempotencyKey: newKey()}))
	wantCode(t, "MarkMilestone", err, connect.CodePermissionDenied)
	_, err = p.undo(tb.campaign, newKey())
	wantCode(t, "UndoLastXPAward", err, connect.CodePermissionDenied)
	if got := tb.master.xpOf(t, tb.pcs[0]); got != 0 || len(tb.master.history(t, tb.campaign)) != 0 {
		t.Errorf("a player's calls changed XP to %d or the history", got)
	}
}

// TestXPCanLevelUpInAnXPCampaign: the sheet's XP reaching the next level's
// (300 for level 2) is "Pode subir de nível" (RN-12), for the master and the
// owner; the XP is not what a milestones campaign counts.
func TestXPCanLevelUpInAnXPCampaign(t *testing.T) {
	t.Parallel()
	tb := newTable(t, enemies, 2)
	tb.master.manual(t, tb.campaign, 299, tb.pcs[0].GetId())
	tb.master.manual(t, tb.campaign, 600, tb.pcs[1].GetId())
	exp := tb.players[1].experience(t, tb.campaign) // a player reads everyone's (question 50)
	pens, toren := exp.GetCharacters()[0], exp.GetCharacters()[1]
	if pens.GetCanLevelUp() || pens.GetNextLevelXp() != 300 || pens.GetExperiencePoints() != 299 || pens.GetLevel() != 1 {
		t.Errorf("Pensantus = %v; want 299 of 300 at level 1, no level up", pens)
	}
	if !toren.GetCanLevelUp() || toren.GetLevelUpReason() != charactersv1.LevelUpReason_LEVEL_UP_REASON_XP {
		t.Errorf("Toren = %v; want can level up by XP", toren)
	}
	if c := tb.players[1].character(t, tb.pcs[1]); !c.GetCanLevelUp() || c.GetLevelUpReason() != charactersv1.LevelUpReason_LEVEL_UP_REASON_XP {
		t.Errorf("Toren's sheet = can level up %v (%v), want XP", c.GetCanLevelUp(), c.GetLevelUpReason())
	}
	if c := tb.players[0].character(t, tb.pcs[0]); c.GetCanLevelUp() {
		t.Error("Pensantus's sheet says can level up at 299 XP")
	}
	// The other player's sheet is not theirs to read at all.
	_, err := tb.players[0].characters.GetCharacter(t.Context(), connect.NewRequest(&charactersv1.GetCharacterRequest{CampaignId: tb.campaign, CharacterId: tb.pcs[1].GetId()}))
	wantCode(t, "GetCharacter(another player's)", err, connect.CodeNotFound)
}

// TestHistoryPages: the history comes newest first, a page at a time.
func TestHistoryPages(t *testing.T) {
	t.Parallel()
	tb := newTable(t, enemies, 1)
	var ids []string
	for i := range 5 {
		ids = append(ids, tb.master.manual(t, tb.campaign, int32(10+i), tb.ids(1)...).GetAward().GetId())
	}
	slices.Reverse(ids)
	var got []string
	token := ""
	for pages := 0; ; pages++ {
		res, err := tb.players[0].xp.ListXPAwards(t.Context(), connect.NewRequest(&progressionv1.ListXPAwardsRequest{CampaignId: tb.campaign, PageSize: 2, PageToken: token}))
		if err != nil {
			t.Fatalf("ListXPAwards(page %d) error = %v", pages, err)
		}
		for _, a := range res.Msg.GetAwards() {
			got = append(got, a.GetId())
		}
		if token = res.Msg.GetNextPageToken(); token == "" {
			break
		}
		if pages > 5 {
			t.Fatal("the history never ends")
		}
	}
	if !slices.Equal(got, ids) {
		t.Errorf("pages = %v, want %v", got, ids)
	}
	_, err := tb.players[0].xp.ListXPAwards(t.Context(), connect.NewRequest(&progressionv1.ListXPAwardsRequest{CampaignId: tb.campaign, PageToken: "nope"}))
	wantCode(t, "ListXPAwards(bad token)", err, connect.CodeInvalidArgument)
	_, err = tb.players[0].xp.ListXPAwards(t.Context(), connect.NewRequest(&progressionv1.ListXPAwardsRequest{CampaignId: tb.campaign, PageSize: 51}))
	wantCode(t, "ListXPAwards(page_size 51)", err, connect.CodeInvalidArgument)
}

// TestSessionEventsHaveNoNamesNorReasons: the event of an award is IDs and
// numbers only (docs/privacy.md).
func TestSessionEventsHaveNoNamesNorReasons(t *testing.T) {
	t.Parallel()
	tb := newTable(t, milestones, 1)
	if _, err := tb.master.play.StartGameSession(t.Context(), connect.NewRequest(&playv1.StartGameSessionRequest{CampaignId: tb.campaign})); err != nil {
		t.Fatalf("StartGameSession() error = %v", err)
	}
	tb.master.milestone(t, tb.campaign, tb.ids(1)...)
	if _, err := tb.master.undo(tb.campaign, newKey()); err != nil {
		t.Fatalf("UndoLastXPAward() error = %v", err)
	}
	if tb.events(t, "milestone_marked") != 1 || tb.events(t, "xp_award_undone") != 1 {
		t.Fatalf("events: milestone_marked %d, xp_award_undone %d; want 1 and 1", tb.events(t, "milestone_marked"), tb.events(t, "xp_award_undone"))
	}
	rows, err := tb.h.pool.Query(t.Context(), `SELECT payload FROM session_events WHERE kind IN ('milestone_marked', 'xp_award_undone')`)
	if err != nil {
		t.Fatalf("read the payloads: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			t.Fatalf("scan: %v", err)
		}
		var decoded map[string]any
		if err := json.Unmarshal(payload, &decoded); err != nil {
			t.Fatalf("payload %s: %v", payload, err)
		}
		if strings.Contains(string(payload), "Derrotaram") || strings.Contains(string(payload), "Pensantus") {
			t.Errorf("payload %s has a reason or a name", payload)
		}
	}
}

// TestRN20_PlayersNeverGetAnNPCsXP: the XP an NPC gives is the master's. A
// player's view of the combat has no xp_value, though the goblins are revealed
// and defeated; the master's has it.
func TestRN20_PlayersNeverGetAnNPCsXP(t *testing.T) {
	t.Parallel()
	tb := newTable(t, enemies, 1)
	goblin := tb.master.minion(t, tb.campaign, "Goblin", 50)
	enc := tb.fight(t, goblin, 2, 2, true)

	total := int32(0)
	for _, c := range enc.GetCombatants() {
		total += c.GetXpValue()
	}
	if total != 100 {
		t.Errorf("the master's combatants give %d XP, want 100", total)
	}
	res, err := tb.players[0].combat.GetEncounter(t.Context(), connect.NewRequest(&playv1.GetEncounterRequest{CampaignId: tb.campaign}))
	if err != nil {
		t.Fatalf("GetEncounter(player) error = %v", err)
	}
	seen := 0
	for _, c := range res.Msg.GetEncounter().GetCombatants() {
		seen++
		if c.GetXpValue() != 0 {
			t.Errorf("a player got xp_value %d for %s", c.GetXpValue(), c.GetLabel())
		}
	}
	if seen < 3 {
		t.Fatalf("the player sees %d combatants, want the revealed goblins too", seen)
	}
	if s := protojson.Format(res.Msg); strings.Contains(s, "xpValue") || strings.Contains(s, "challengeRating") {
		t.Errorf("the player's encounter has an XP or a challenge rating: %s", s)
	}
	// The NPC's sheet, where the rating is, is not the player's to read.
	_, err = tb.players[0].characters.GetCharacter(t.Context(), connect.NewRequest(&charactersv1.GetCharacterRequest{CampaignId: tb.campaign, CharacterId: goblin.GetId()}))
	wantCode(t, "GetCharacter(an NPC, as a player)", err, connect.CodeNotFound)
	list, err := tb.players[0].characters.ListCharacters(t.Context(), connect.NewRequest(&charactersv1.ListCharactersRequest{CampaignId: tb.campaign}))
	if err != nil {
		t.Fatalf("ListCharacters(player) error = %v", err)
	}
	if s := protojson.Format(list.Msg); strings.Contains(s, "xpValue") || strings.Contains(s, "challengeRating") || strings.Contains(s, "Goblin") {
		t.Errorf("the player's list has an NPC or its XP: %s", s)
	}
}

// TestLevelUpReason: the rule of "Pode subir de nível" (RN-12), as a table.
func TestLevelUpReason(t *testing.T) {
	t.Parallel()
	const (
		none = charactersv1.LevelUpReason_LEVEL_UP_REASON_UNSPECIFIED
		byXP = charactersv1.LevelUpReason_LEVEL_UP_REASON_XP
		mark = charactersv1.LevelUpReason_LEVEL_UP_REASON_MILESTONE
	)
	for _, c := range []struct {
		name                       string
		mode                       campaignsv1.XpMode
		level, xp, next, markLevel int32
		want                       charactersv1.LevelUpReason
	}{
		{"299 of 300", enemies, 1, 299, 300, 0, none},
		{"300 of 300", enemies, 1, 300, 300, 0, byXP},
		{"more than enough, by gold", gold, 2, 1000, 900, 0, byXP},
		{"level 20 never", enemies, 20, 400000, 0, 0, none},
		{"marked at the same level", milestones, 3, 0, 2700, 3, mark},
		{"marked before the level went up", milestones, 4, 0, 6500, 3, none},
		{"not marked", milestones, 3, 0, 2700, 0, none},
		{"XP means nothing in a milestones campaign", milestones, 1, 5000, 300, 0, none},
		{"a mark means nothing in an XP campaign", enemies, 1, 0, 300, 1, none},
		{"marked at level 20", milestones, 20, 0, 0, 20, none},
	} {
		if got := levelUpReason(c.mode, c.level, c.xp, c.next, c.markLevel); got != c.want {
			t.Errorf("%s: levelUpReason = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestMR040_CanLevelUpClearsAfterwards: the guided level-up (MR-040) takes the
// character up on a locked sheet, and "Pode subir de nível" goes away by
// itself: in an XP campaign because the XP no longer reaches the next level,
// in a milestones campaign because the level passed the mark's.
func TestMR040_CanLevelUpClearsAfterwards(t *testing.T) {
	t.Parallel()
	for _, mode := range []struct {
		name string
		mode campaignsv1.XpMode
		give func(t *testing.T, tb *table)
		why  charactersv1.LevelUpReason
	}{
		{"XP", enemies, func(t *testing.T, tb *table) { tb.master.manual(t, tb.campaign, 300, tb.ids(1)...) }, charactersv1.LevelUpReason_LEVEL_UP_REASON_XP},
		{"milestones", milestones, func(t *testing.T, tb *table) { tb.master.milestone(t, tb.campaign, tb.ids(1)...) }, charactersv1.LevelUpReason_LEVEL_UP_REASON_MILESTONE},
	} {
		t.Run(mode.name, func(t *testing.T) {
			t.Parallel()
			tb := newTable(t, mode.mode, 1)
			player, pc := tb.players[0], tb.pcs[0]
			if _, err := tb.master.play.StartGameSession(t.Context(), connect.NewRequest(&playv1.StartGameSessionRequest{CampaignId: tb.campaign})); err != nil {
				t.Fatalf("StartGameSession() error = %v", err)
			}
			mode.give(t, tb)
			c := player.character(t, pc)
			if c.GetState() != charactersv1.CharacterState_CHARACTER_STATE_LOCKED || !c.GetCanLevelUp() || c.GetLevelUpReason() != mode.why {
				t.Fatalf("before: state %v, can level up %v (%v), want a locked sheet that can level up (%v)", c.GetState(), c.GetCanLevelUp(), c.GetLevelUpReason(), mode.why)
			}
			// Wizard 1 to 2: the subclass, two spells for the book, and prepared ones.
			res, err := player.characters.LevelUpCharacter(t.Context(), connect.NewRequest(&charactersv1.LevelUpCharacterRequest{
				CampaignId: tb.campaign, CharacterId: pc.GetId(), Revision: c.GetRevision(),
				Choices: &charactersv1.LevelUpChoices{
					ClassKey: "class:wizard", SubclassKey: "subclass:evocation",
					KnownSpellKeys:    []string{"spell:magic-missile", "spell:shield"},
					PreparedSpellKeys: []string{"spell:magic-missile", "spell:shield"},
					HitPoints:         &charactersv1.LevelUpHitPoints{Method: charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_AVERAGE},
				},
			}))
			if err != nil {
				t.Fatalf("LevelUpCharacter() error = %v", err)
			}
			after := res.Msg.GetCharacter()
			if after.GetDerived().GetTotalLevel() != 2 || after.GetCanLevelUp() || after.GetLevelUpReason() != charactersv1.LevelUpReason_LEVEL_UP_REASON_UNSPECIFIED {
				t.Errorf("after: level %d, can level up %v (%v); want level 2 and no tag", after.GetDerived().GetTotalLevel(), after.GetCanLevelUp(), after.GetLevelUpReason())
			}
			// The master's tag is gone too, and so is the party's list.
			if got := tb.master.character(t, pc); got.GetCanLevelUp() {
				t.Error("the master still sees the tag")
			}
			if exp := tb.master.experience(t, tb.campaign).GetCharacters()[0]; exp.GetCanLevelUp() || exp.GetLevel() != 2 {
				t.Errorf("the campaign's list = level %d, can level up %v", exp.GetLevel(), exp.GetCanLevelUp())
			}
		})
	}
}

// TestUndoGivesBackWhatTheSheetGained: a sheet holds at most 1,000,000 XP, so an
// award past it gains less than its share; the share records the gain and the
// undo takes back exactly that.
func TestUndoGivesBackWhatTheSheetGained(t *testing.T) {
	t.Parallel()
	tb := newTable(t, enemies, 1)
	pc := tb.pcs[0]

	ch := tb.master.character(t, pc)
	ch.GetSheet().GetFull().ExperiencePoints = 999_950
	if _, err := tb.master.characters.UpdateCharacter(t.Context(), connect.NewRequest(&charactersv1.UpdateCharacterRequest{
		CampaignId: tb.campaign, CharacterId: ch.GetId(), Revision: ch.GetRevision(), Name: ch.GetName(), Sheet: ch.GetSheet(),
	})); err != nil {
		t.Fatalf("UpdateCharacter() error = %v", err)
	}

	tb.master.manual(t, tb.campaign, 100, pc.GetId())
	if got := tb.master.xpOf(t, pc); got != 1_000_000 {
		t.Fatalf("sheet XP = %d after the award, want 1000000 (clamped)", got)
	}
	if shares := tb.master.history(t, tb.campaign)[0].GetShares(); len(shares) != 1 || shares[0].GetXp() != 50 {
		t.Errorf("shares = %v, want one of 50 XP (what the sheet gained)", shares)
	}

	if _, err := tb.master.undo(tb.campaign, newKey()); err != nil {
		t.Fatalf("UndoLastXPAward() error = %v", err)
	}
	if got := tb.master.xpOf(t, pc); got != 999_950 {
		t.Errorf("sheet XP = %d after the undo, want 999950 (the XP before the award)", got)
	}

	// Positive control: an award that fits is taken back whole.
	tb.master.manual(t, tb.campaign, 20, pc.GetId())
	if _, err := tb.master.undo(tb.campaign, newKey()); err != nil {
		t.Fatalf("UndoLastXPAward() error = %v", err)
	}
	if got := tb.master.xpOf(t, pc); got != 999_950 {
		t.Errorf("sheet XP = %d after the second undo, want 999950", got)
	}
}
