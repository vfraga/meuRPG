package play

import (
	"slices"
	"strings"
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The session summary, "Resumo da sessão" (MR-032, RN-20; question 64). One
// session: a combat and three scenes, one of them with the DC hidden. The
// scenes are map points written straight into the tables (this harness has no
// MapService); the combat is played through the API.

// sceneSpec is an action of a scene written by newScene.
type sceneSpec struct {
	key string
	dc  *int32
}

// newScene writes a SCENE point of the map with its actions (1 attempt each
// unless the test says otherwise) and returns the point and the action IDs in
// order.
func (h *harness) newScene(mapID, name string, showDC bool, maxAttempts int, specs ...sceneSpec) (pointID string, actions []string) {
	h.t.Helper()
	pointID = newKey()
	if _, err := h.pool.Exec(h.t.Context(),
		`INSERT INTO map_points (id, map_id, kind, name, show_dc, x_bp, y_bp, created_at, updated_at) VALUES ($1, $2, 'scene', $3, $4, 100, 100, now(), now())`,
		pointID, mapID, name, showDC); err != nil {
		h.t.Fatalf("insert the scene point: %v", err)
	}
	for i, sp := range specs {
		id := newKey()
		if _, err := h.pool.Exec(h.t.Context(),
			`INSERT INTO scene_actions (id, point_id, position, key, dc, max_attempts, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, now(), now())`,
			id, pointID, i, sp.key, sp.dc, maxAttempts); err != nil {
			h.t.Fatalf("insert a scene action: %v", err)
		}
		actions = append(actions, id)
	}
	return pointID, actions
}

func (a *armed) openScene(t *testing.T, pointID string) {
	t.Helper()
	if _, err := a.master.play.OpenScene(t.Context(), connect.NewRequest(&playv1.OpenSceneRequest{CampaignId: a.campaignID, PointId: pointID})); err != nil {
		t.Fatalf("OpenScene() error = %v", err)
	}
}

// checkRoll rolls an action of the open scene in the app, with the d20 the
// roller gives.
func (a *armed) checkRoll(t *testing.T, u *user, actionID string, face int) {
	t.Helper()
	a.h.roller.queue(face)
	if _, err := u.play.RollSceneCheck(t.Context(), connect.NewRequest(&playv1.RollSceneCheckRequest{
		CampaignId: a.campaignID, ActionId: actionID, IdempotencyKey: newKey(), Roll: &playv1.RollSceneCheckRequest_RollInApp{RollInApp: true},
	})); err != nil {
		t.Fatalf("RollSceneCheck() error = %v", err)
	}
}

func (a *armed) summary(t *testing.T, u *user, sessionID string) (*playv1.SessionSummary, error) {
	t.Helper()
	res, err := u.play.GetSessionSummary(t.Context(), connect.NewRequest(&playv1.GetSessionSummaryRequest{CampaignId: a.campaignID, GameSessionId: sessionID}))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetSummary(), nil
}

func summaryCategories(sum *playv1.SessionSummary) []string {
	out := []string{}
	for _, c := range sum.GetCategories() {
		var who []string
		for _, w := range c.GetWinners() {
			who = append(who, w.GetName())
		}
		out = append(out, strings.TrimPrefix(c.GetKind().String(), "HIGHLIGHT_KIND_")+" "+itoa(c.GetValue())+": "+strings.Join(who, ", "))
	}
	return out
}

// TestMR032_SessionSummary: the canonical session. One combat: Toren's
// critical kills the Goblin (7 land of 19: a final blow), Pensantus's Raio de
// Fogo does 7 and Brisa's rapier 8 to the Capitão. Three scenes:
//
//	A carroça (DC shown)  Investigação DC 12, Atletismo DC 10, a Teste de Força with no DC
//	A ponte (DC shown)    Percepção DC 14
//	A taverna (DC hidden) Persuasão DC 10: every roll passes, and counts for nobody
//
// Pensantus passes 1 of 3 tests, Toren 2 of 3 and Brisa 2 of 2 (a check with no
// DC is not a test). The master gets every number and the table; a player, the
// winners and their own result.
func TestMR032_SessionSummary(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.start(t, plan{
		npcs: []*playv1.Participant{
			{CharacterId: a.goblin.GetId(), Hidden: new(false)},
			{CharacterId: a.capitao.GetId()}, // hidden: a new NPC starts hidden
		},
		npcRolls: []int{1, 2},
		players:  map[string]int32{"Toren": 20, "Pensantus": 15, "Brisa": 10},
		at:       map[string][2]int32{"Toren": {3, 3}, "Goblin": {4, 3}, "Pensantus": {10, 3}, "Capitão Goblin": {12, 3}, "Brisa": {11, 3}},
	})
	hit := func(attacker, key, target string, d20 int, dice ...int) {
		t.Helper()
		a.h.roller.queue(append([]int{d20}, dice...)...)
		res := a.mustAttack(t, a.master, e, attacker, key, target, inAppRoll)
		if res.GetPendingDamage() == nil {
			t.Fatalf("%s's attack on %s = %v, want a hit", attacker, target, res.GetRoll())
		}
		dmg := a.mustDamage(t, a.master, e, res.GetPendingDamage().GetId(), inAppDamage).GetPendingDamage()
		if dmg.GetStatus() == playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_ROLLED {
			if _, err := a.settle(t, a.master, e, dmg.GetId(), true); err != nil {
				t.Fatalf("ApplyPendingDamage() error = %v", err)
			}
		}
	}
	hit("Toren", battleaxe, "Goblin", 20, 8, 8) // a critical, 7 land: a final blow
	a.mustEndTurn(t, a.master, e)
	hit("Pensantus", fireBolt, "Capitão Goblin", 15, 7)
	a.mustEndTurn(t, a.master, e)
	hit("Brisa", rapier, "Capitão Goblin", 15, 5) // 8
	a.endEncounter(t, e)

	// The scenes. Pensantus's Investigação is +3 and Toren's +0, Atletismo +3
	// for Toren (Strength 16), Brisa's +0 everywhere.
	mapID := a.mapID
	cart, cartActions := a.h.newScene(mapID, "A carroça tombada", true, 0, // unlimited: a second try
		sceneSpec{"skill:investigation", new(int32(12))}, sceneSpec{"skill:athletics", new(int32(10))}, sceneSpec{"ability:str", nil})
	bridge, bridgeActions := a.h.newScene(mapID, "A ponte", true, 1, sceneSpec{"skill:perception", new(int32(14))})
	tavern, tavernActions := a.h.newScene(mapID, "A taverna", false, 1, sceneSpec{"skill:persuasion", new(int32(10))})

	a.openScene(t, cart)
	a.checkRoll(t, a.ana, cartActions[0], 18) // 21 passes
	a.checkRoll(t, a.ana, cartActions[0], 1)  // 4 misses (a second try)
	a.checkRoll(t, a.caio, cartActions[0], 5) // 5 misses
	a.checkRoll(t, a.caio, cartActions[1], 20)
	a.checkRoll(t, a.bia, cartActions[1], 15)
	a.checkRoll(t, a.bia, cartActions[2], 20) // no DC: no test
	a.openScene(t, bridge)
	a.checkRoll(t, a.bia, bridgeActions[0], 20)
	a.checkRoll(t, a.ana, bridgeActions[0], 2)
	a.checkRoll(t, a.caio, bridgeActions[0], 16)
	a.openScene(t, tavern)
	a.checkRoll(t, a.ana, tavernActions[0], 20) // the DC is hidden: none of it counts
	a.checkRoll(t, a.bia, tavernActions[0], 20)
	a.checkRoll(t, a.caio, tavernActions[0], 20)

	session := a.master.play
	listed, err := session.ListGameSessions(t.Context(), connect.NewRequest(&playv1.ListGameSessionsRequest{CampaignId: a.campaignID}))
	if err != nil || len(listed.Msg.GetGameSessions()) != 1 {
		t.Fatalf("ListGameSessions() = %v, %v", listed, err)
	}
	open := listed.Msg.GetGameSessions()[0]
	_, err = a.summary(t, a.master, open.GetId())
	wantCode(t, "the summary of an open session", err, connect.CodeFailedPrecondition)
	ended := a.master.end(t, open)

	want := []string{
		"MOST_DAMAGE 8: Brisa",
		"FINAL_BLOW 1: Toren",
		"CRITICAL_HITS 1: Toren",
		"CHECKS_PASSED 2: Toren, Brisa",
	}
	sum, err := a.summary(t, a.master, ended.GetId())
	if err != nil {
		t.Fatalf("GetSessionSummary() error = %v", err)
	}
	if got := summaryCategories(sum); !slices.Equal(got, want) {
		t.Errorf("the master's categories = %q, want %q", got, want)
	}
	if sum.GetCombats() != 1 || sum.GetScenesOpened() != 3 || sum.GetChecksPassed() != 5 || sum.GetChecksTried() != 8 {
		t.Errorf("the master's counts = %d combats, %d scenes, %d of %d tests; want 1, 3 and 5 of 8",
			sum.GetCombats(), sum.GetScenesOpened(), sum.GetChecksPassed(), sum.GetChecksTried())
	}
	if d := sum.GetDuration().AsDuration(); d != ended.GetEndedAt().AsTime().Sub(ended.GetStartedAt().AsTime()) || d <= 0 {
		t.Errorf("duration = %v, want ended_at minus started_at", d)
	}
	type row struct {
		name                                        string
		dealt, healed, taken, blows, crits, ok, try int32
	}
	var table []row
	for _, p := range sum.GetPlayers() {
		h := p.GetHighlights()
		table = append(table, row{h.GetName(), h.GetDamageDealt(), h.GetHealingDone(), h.GetDamageTaken(), h.GetFinalBlows(), h.GetCriticalHits(), p.GetChecksPassed(), p.GetChecksTried()})
	}
	wantTable := []row{{"Toren", 7, 0, 0, 1, 1, 2, 3}, {"Pensantus", 7, 0, 0, 0, 0, 1, 3}, {"Brisa", 8, 0, 0, 0, 0, 2, 2}}
	if !slices.Equal(table, wantTable) {
		t.Errorf("the master's table = %+v, want %+v", table, wantTable)
	}
	if sum.GetMine() != nil {
		t.Errorf("the master got a result of his own: %v", sum.GetMine())
	}

	// A player: the categories and the winners' numbers, their own result, and
	// nothing else: no counts, no table, no NPC.
	for who, c := range map[string]struct {
		u         *user
		dealt     int32
		ok, tried int32
	}{
		"Pensantus's player": {a.ana, 7, 1, 3},
		"Toren's player":     {a.caio, 7, 2, 3},
		"Brisa's player":     {a.bia, 8, 2, 2},
	} {
		got, err := a.summary(t, c.u, ended.GetId())
		if err != nil {
			t.Fatalf("%s: GetSessionSummary() error = %v", who, err)
		}
		if cats := summaryCategories(got); !slices.Equal(cats, want) {
			t.Errorf("%s: categories = %q, want %q", who, cats, want)
		}
		mine := got.GetMine()
		if mine.GetHighlights().GetDamageDealt() != c.dealt || mine.GetChecksPassed() != c.ok || mine.GetChecksTried() != c.tried {
			t.Errorf("%s: own result = %v, want %d damage and %d of %d tests", who, mine, c.dealt, c.ok, c.tried)
		}
		if got.GetCombats() != 0 || got.GetScenesOpened() != 0 || got.GetChecksTried() != 0 || got.GetChecksPassed() != 0 || len(got.GetPlayers()) != 0 {
			t.Errorf("%s was sent the master's numbers: %v", who, got)
		}
		text := asJSON(t, got)
		for _, banned := range []string{"Goblin", "Capitão", a.goblin.GetId(), a.capitao.GetId()} {
			if strings.Contains(text, banned) {
				t.Errorf("%s was sent %q:\n%s", who, banned, text)
			}
		}
	}

	// A session of another campaign, or a made-up one, is not found.
	_, err = a.summary(t, a.master, newKey())
	wantCode(t, "the summary of an unknown session", err, connect.CodeNotFound)
	_, err = a.summary(t, a.master, "x")
	wantCode(t, "the summary of a session that is not a UUID", err, connect.CodeNotFound)
}

// A combat still running when the master ends the session is ended with it,
// and counts; a character that died in it is still named, and its player still
// reads their own result.
func TestMR032_SummaryIncludesTheCombatActiveAtTheEndAndTheDead(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: a.goblin.GetId(), Hidden: new(false)}},
		npcRolls: []int{1},
		players:  map[string]int32{"Toren": 20, "Pensantus": 15, "Brisa": 10},
		at:       map[string][2]int32{"Toren": {3, 3}, "Goblin": {4, 3}, "Pensantus": {10, 3}, "Brisa": {11, 3}},
	})
	a.h.roller.queue(20, 8, 8)
	res := a.mustAttack(t, a.master, e, "Toren", battleaxe, "Goblin", inAppRoll)
	dmg := a.mustDamage(t, a.master, e, res.GetPendingDamage().GetId(), inAppDamage).GetPendingDamage()
	if dmg.GetStatus() == playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_ROLLED {
		if _, err := a.settle(t, a.master, e, dmg.GetId(), true); err != nil {
			t.Fatalf("ApplyPendingDamage() error = %v", err)
		}
	}
	if _, err := a.master.characters.MarkCharacterDead(t.Context(), connect.NewRequest(&charactersv1.MarkCharacterDeadRequest{CampaignId: a.campaignID, CharacterId: a.toren.GetId()})); err != nil {
		t.Fatalf("MarkCharacterDead() error = %v", err)
	}
	// The combat is NOT ended by hand.
	listed, err := a.master.play.ListGameSessions(t.Context(), connect.NewRequest(&playv1.ListGameSessionsRequest{CampaignId: a.campaignID}))
	if err != nil {
		t.Fatal(err)
	}
	ended := a.master.end(t, listed.Msg.GetGameSessions()[0])

	sum, err := a.summary(t, a.master, ended.GetId())
	if err != nil {
		t.Fatal(err)
	}
	if sum.GetCombats() != 1 {
		t.Errorf("combats = %d, want 1: the one active at the end counts", sum.GetCombats())
	}
	if len(sum.GetPlayers()) == 0 || sum.GetPlayers()[0].GetHighlights().GetName() != "Toren" || sum.GetPlayers()[0].GetHighlights().GetDamageDealt() != 7 {
		t.Errorf("the master's table = %v, want the dead Toren first, with 7 damage", sum.GetPlayers())
	}
	mine, err := a.summary(t, a.caio, ended.GetId())
	if err != nil || mine.GetMine().GetHighlights().GetDamageDealt() != 7 || mine.GetMine().GetHighlights().GetName() != "Toren" {
		t.Errorf("Toren's player's own result = %v, %v; want Toren's 7 damage", mine.GetMine(), err)
	}
}

// bulkSceneRolls copies the latest real roll of the open session count times, as a
// table could never roll them by hand.
func (a *armed) bulkSceneRolls(t *testing.T, sessionID string, count int) {
	t.Helper()
	if _, err := a.h.pool.Exec(t.Context(), `
INSERT INTO session_events (game_session_id, seq, kind, actor_user_id, character_id, payload, created_at)
SELECT e.game_session_id, (SELECT max(seq) FROM session_events WHERE game_session_id = e.game_session_id) + g, e.kind, e.actor_user_id, e.character_id, e.payload, e.created_at
FROM (SELECT * FROM session_events WHERE game_session_id = $1 AND kind = 'scene_check_rolled' ORDER BY seq DESC LIMIT 1) e, generate_series(1, $2) g`,
		sessionID, count); err != nil {
		t.Fatalf("bulk insert: %v", err)
	}
}

func TestSummaryCountsAnyNumberOfSceneRolls(t *testing.T) {
	a := newArmed(t)
	point, actions := a.h.newScene(a.mapID, "A carroça", true, 0, sceneSpec{"skill:investigation", new(int32(12))})
	a.openScene(t, point)
	a.checkRoll(t, a.ana, actions[0], 20) // passes
	for range 2 {
		a.checkRoll(t, a.ana, actions[0], 1) // fails
	}
	listed, err := a.master.play.ListGameSessions(t.Context(), connect.NewRequest(&playv1.ListGameSessionsRequest{CampaignId: a.campaignID}))
	if err != nil || len(listed.Msg.GetGameSessions()) != 1 {
		t.Fatalf("ListGameSessions() = %v, %v", listed, err)
	}
	open := listed.Msg.GetGameSessions()[0]

	// Many more rows than the roll cap lets through, as old data or a
	// bug could leave: the summary still answers, and still counts them.
	const extra = 5000
	a.bulkSceneRolls(t, open.GetId(), extra)

	ended := a.master.end(t, open)
	for who, u := range map[string]*user{"master": a.master, "player": a.ana} {
		sum, err := a.summary(t, u, ended.GetId())
		if err != nil {
			t.Errorf("%s: GetSessionSummary() code = %v, error = %v; want the summary", who, connect.CodeOf(err), err)
			continue
		}
		if who == "player" {
			// The copied roll is a failure, like the two real ones.
			if got := sum.GetMine().GetChecksTried(); got != 3+extra {
				t.Errorf("player: checks tried = %d, want %d", got, 3+extra)
			}
			if got := sum.GetMine().GetChecksPassed(); got != 1 {
				t.Errorf("player: checks passed = %d, want 1", got)
			}
		}
	}
}

func TestUnlimitedSceneActionHasARollCap(t *testing.T) {
	a := newArmed(t)
	point, actions := a.h.newScene(a.mapID, "A carroça", true, 0, sceneSpec{"skill:investigation", new(int32(12))})
	a.openScene(t, point)
	a.checkRoll(t, a.ana, actions[0], 10)
	listed, err := a.master.play.ListGameSessions(t.Context(), connect.NewRequest(&playv1.ListGameSessionsRequest{CampaignId: a.campaignID}))
	if err != nil || len(listed.Msg.GetGameSessions()) != 1 {
		t.Fatalf("ListGameSessions() = %v, %v", listed, err)
	}
	// Up to the cap, an unlimited action never refuses (the control).
	a.bulkSceneRolls(t, listed.Msg.GetGameSessions()[0].GetId(), maxUnlimitedSceneRolls-2)
	a.checkRoll(t, a.ana, actions[0], 10)

	a.h.roller.queue(10)
	_, err = a.ana.play.RollSceneCheck(t.Context(), connect.NewRequest(&playv1.RollSceneCheckRequest{
		CampaignId: a.campaignID, ActionId: actions[0], IdempotencyKey: newKey(), Roll: &playv1.RollSceneCheckRequest_RollInApp{RollInApp: true},
	}))
	if err == nil {
		t.Fatalf("roll %d of an unlimited action was accepted, want it refused", maxUnlimitedSceneRolls+1)
	}
	if got := connect.CodeOf(err); got != connect.CodeFailedPrecondition {
		t.Errorf("code = %v, want %v (%v)", got, connect.CodeFailedPrecondition, err)
	}
}
