package play

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"connectrpc.com/connect"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The RP scene calls (scene.go), frozen before the refactor of their long
// functions: OpenScene, CloseScene, GetOpenScene, RollSceneCheck and
// GrantSceneAttempt. The scenes are written straight into the tables by
// newScene (summary_test.go); the held-roll paths (Bardic Inspiration, Lucky)
// have their own tests and are not repeated here.

// wantSceneRefusal checks a scene call's error: the code, the exact message and, for a
// failed_precondition, the SceneBlocked reason of its detail (UNSPECIFIED: none).
func wantSceneRefusal(t *testing.T, call string, err error, code connect.Code, reason playv1.SceneBlockedReason, msg string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s error = nil, want %v %q", call, code, msg)
	}
	ce, ok := errors.AsType[*connect.Error](err)
	if !ok {
		t.Fatalf("%s error = %T %v, want a connect error", call, err, err)
	}
	if ce.Code() != code || ce.Message() != msg {
		t.Errorf("%s error = %v %q, want %v %q", call, ce.Code(), ce.Message(), code, msg)
	}
	got := playv1.SceneBlockedReason_SCENE_BLOCKED_REASON_UNSPECIFIED
	for _, d := range ce.Details() {
		if v, derr := d.Value(); derr == nil {
			if b, ok := v.(*playv1.SceneBlocked); ok {
				got = b.GetReason()
			}
		}
	}
	if got != reason {
		t.Errorf("%s SceneBlocked reason = %v, want %v", call, got, reason)
	}
}

// wantSceneNoSession checks the NO_OPEN_SESSION refusal of the play service.
func wantSceneNoSession(t *testing.T, call string, err error) {
	t.Helper()
	wantBlocked(t, call, err, playv1.GameSessionBlockedReason_GAME_SESSION_BLOCKED_REASON_NO_OPEN_SESSION)
	if ce, _ := errors.AsType[*connect.Error](err); ce.Message() != "the campaign has no open game session" {
		t.Errorf("%s message = %q, want the no-open-session text", call, ce.Message())
	}
}

const (
	sceneNoScene = playv1.SceneBlockedReason_SCENE_BLOCKED_REASON_NO_OPEN_SCENE
	sceneNoChar  = playv1.SceneBlockedReason_SCENE_BLOCKED_REASON_NO_CHARACTER
	sceneAlready = playv1.SceneBlockedReason_SCENE_BLOCKED_REASON_ALREADY_ROLLED
	sceneWrongDi = playv1.SceneBlockedReason_SCENE_BLOCKED_REASON_WRONG_DICE_MODE
	sceneNone    = playv1.SceneBlockedReason_SCENE_BLOCKED_REASON_UNSPECIFIED
)

// sceneKinds is the scene and stage events of the campaign's latest session, in order.
func sceneKinds(t *testing.T, a *armed) string {
	t.Helper()
	rows, err := a.h.pool.Query(t.Context(), `SELECT e.kind FROM session_events e JOIN game_sessions g ON g.id = e.game_session_id
		WHERE g.campaign_id = $1 AND (e.kind LIKE 'scene\_%' OR e.kind LIKE 'stage\_%') ORDER BY g.started_at, e.seq`, a.campaignID)
	if err != nil {
		t.Fatalf("read the events: %v", err)
	}
	defer rows.Close()
	var kinds []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			t.Fatalf("scan: %v", err)
		}
		kinds = append(kinds, k)
	}
	return strings.Join(kinds, ",")
}

// sceneHints drains what the streams got up to a marker, as short words.
func sceneHints(t *testing.T, a *armed, ws ...*watcher) []string {
	t.Helper()
	a.master.markCurrentMap(t, a.campaignID, a.mapID, ws...)
	out := make([]string, len(ws))
	for i, w := range ws {
		var words []string
		for _, ev := range w.beforeMarker(t) {
			switch {
			case ev.GetSceneChanged() != nil:
				words = append(words, "scene_changed")
			case ev.GetSceneCheckRolled() != nil:
				words = append(words, "scene_check_rolled")
			case ev.GetStageChanged() != nil:
				words = append(words, "stage_changed")
			default:
				words = append(words, fmt.Sprintf("other:%T", ev.GetEvent()))
			}
		}
		out[i] = strings.Join(words, ",")
	}
	return out
}

func sceneRollWith(a *armed, u *user, campaignID, action, key string, roll *playv1.RollSceneCheckRequest) (*playv1.RollSceneCheckResponse, error) {
	roll.CampaignId, roll.ActionId, roll.IdempotencyKey = campaignID, action, key
	res, err := u.play.RollSceneCheck(a.h.t.Context(), connect.NewRequest(roll))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

// sceneInApp rolls an action in the app with the d20 the roller gives.
func sceneInApp(a *armed, u *user, action, key string, face int) (*playv1.RollSceneCheckResponse, error) {
	a.h.roller.queue(face)
	return sceneRollWith(a, u, a.campaignID, action, key, &playv1.RollSceneCheckRequest{Roll: &playv1.RollSceneCheckRequest_RollInApp{RollInApp: true}})
}

func sceneGrant(a *armed, u *user, action, character, key string) (*playv1.GrantSceneAttemptResponse, error) {
	res, err := u.play.GrantSceneAttempt(a.h.t.Context(), connect.NewRequest(&playv1.GrantSceneAttemptRequest{
		CampaignId: a.campaignID, ActionId: action, CharacterId: character, IdempotencyKey: key,
	}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

// sceneRender prints what a viewer reads of the open scene, one line per fact, with the actions named
// by their label.
func sceneRender(info *playv1.OpenSceneInfo, labels map[string]string) string {
	if info == nil {
		return "<no scene>"
	}
	i32 := func(p *int32) string {
		if p == nil {
			return "-"
		}
		return fmt.Sprint(*p)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "scene %q desc=%q show_dc=%v opened=%v hooks=%q\n", info.GetName(), info.GetDescription(), info.GetShowDc(), info.GetOpenedAt() != nil, info.GetHooks())
	for _, v := range info.GetActions() {
		fmt.Fprintf(&b, "action %s key=%s name=%q check=%q max=%d dc=%d bonus=%s passive=%s left=%s\n",
			labels[v.GetId()], v.GetKey(), v.GetName(), v.GetCheckName(), v.GetMaxAttempts(), v.GetDc(), i32(v.Bonus), i32(v.Passive), i32(v.AttemptsLeft))
	}
	for _, c := range info.GetClues() {
		var who []string
		for _, r := range c.GetRevealedTo() {
			who = append(who, r.GetCharacterName())
		}
		fmt.Fprintf(&b, "clue %q to=%v\n", c.GetText(), who)
	}
	for _, img := range info.GetImages() {
		fmt.Fprintf(&b, "image %q whole=%v\n", img.GetName(), img.GetShowsWholeMap())
	}
	for _, r := range info.GetRolls() {
		fmt.Fprintf(&b, "roll %s by=%q faces=%v mod=%d total=%d passed=%s left=%s physical=%v mode=%v\n",
			labels[r.GetActionId()], r.GetCharacterName(), r.GetRoll().GetFaces(), r.GetRoll().GetModifier(), r.GetRoll().GetTotal(),
			sceneBool(r.Passed), i32(r.AttemptsLeft), r.GetRoll().GetPhysical(), r.GetMode())
	}
	fmt.Fprintf(&b, "stage=%d", len(info.GetStage()))
	return b.String()
}

func sceneBool(p *bool) string {
	if p == nil {
		return "-"
	}
	return fmt.Sprint(*p)
}

func sceneRead(t *testing.T, a *armed, u *user, labels map[string]string) string {
	t.Helper()
	res, err := u.play.GetOpenScene(t.Context(), connect.NewRequest(&playv1.GetOpenSceneRequest{CampaignId: a.campaignID}))
	if err != nil {
		return "error: " + err.Error()
	}
	return sceneRender(res.Msg.GetScene(), labels)
}

func sceneLabels(actions []string) map[string]string {
	out := map[string]string{}
	for i, id := range actions {
		out[id] = fmt.Sprintf("A%d", i+1)
	}
	return out
}

func wantSceneText(t *testing.T, what, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s:\n got:\n%s\nwant:\n%s", what, got, want)
	}
}

// sceneLoneCampaign is a campaign with a map and a scene point but no game session.
func sceneLoneCampaign(t *testing.T, a *armed) (campaignID, pointID string) {
	t.Helper()
	campaignID = a.h.newCampaign(a.master, "Sem sessão", a.caio)
	mapID := a.h.newMap(campaignID, gridColumns)
	pointID, _ = a.h.newScene(mapID, "Sem sessão", true, 1)
	return campaignID, pointID
}

func TestSceneOpenCloseRefusals(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	ctx := t.Context()
	point, _ := a.h.newScene(a.mapID, "A carroça", true, 1, sceneSpec{"skill:investigation", new(int32(12))})
	open := func(u *user, campaignID, pointID string) error {
		_, err := u.play.OpenScene(ctx, connect.NewRequest(&playv1.OpenSceneRequest{CampaignId: campaignID, PointId: pointID}))
		return err
	}
	closeScene := func(u *user, campaignID string) error {
		_, err := u.play.CloseScene(ctx, connect.NewRequest(&playv1.CloseSceneRequest{CampaignId: campaignID}))
		return err
	}
	outsider := a.h.newUser("Fora")

	// Who may call: the campaign's master alone; a stranger does not even learn the campaign exists.
	wantSceneRefusal(t, "player OpenScene", open(a.caio, a.campaignID, point), connect.CodePermissionDenied, sceneNone, "only the campaign's master can do this")
	wantSceneRefusal(t, "outsider OpenScene", open(outsider, a.campaignID, point), connect.CodeNotFound, sceneNone, "campaign not found")
	wantSceneRefusal(t, "player CloseScene", closeScene(a.caio, a.campaignID), connect.CodePermissionDenied, sceneNone, "only the campaign's master can do this")
	wantSceneRefusal(t, "outsider CloseScene", closeScene(outsider, a.campaignID), connect.CodeNotFound, sceneNone, "campaign not found")

	// The point is looked up before the session.
	// Finding: a malformed ID answers "scene point not found" (scene.go errScenePointNotFound), an
	// unknown or non-scene one maps' "point not found": two texts for one situation. Frozen.
	wantSceneRefusal(t, "OpenScene(not a uuid)", open(a.master, a.campaignID, "nope"), connect.CodeNotFound, sceneNone, "scene point not found")
	wantSceneRefusal(t, "OpenScene(unknown point)", open(a.master, a.campaignID, newKey()), connect.CodeNotFound, sceneNone, "point not found")
	battle := newKey()
	if _, err := a.h.pool.Exec(ctx, `INSERT INTO map_points (id, map_id, kind, name, x_bp, y_bp, created_at, updated_at) VALUES ($1, $2, 'battle', 'Emboscada', 10, 10, now(), now())`, battle, a.mapID); err != nil {
		t.Fatalf("insert a battle point: %v", err)
	}
	wantSceneRefusal(t, "OpenScene(a battle point)", open(a.master, a.campaignID, battle), connect.CodeNotFound, sceneNone, "point not found")

	// No game session open.
	lone, lonePoint := sceneLoneCampaign(t, a)
	wantSceneNoSession(t, "OpenScene", open(a.master, lone, lonePoint))
	wantSceneNoSession(t, "CloseScene", closeScene(a.master, lone))
	// The read answers the same.
	_, err := a.master.play.GetOpenScene(ctx, connect.NewRequest(&playv1.GetOpenSceneRequest{CampaignId: lone}))
	wantSceneNoSession(t, "GetOpenScene", err)
	// A point of another campaign is not found from this one.
	wantSceneRefusal(t, "OpenScene(other campaign's point)", open(a.master, a.campaignID, lonePoint), connect.CodeNotFound, sceneNone, "point not found")
}

func TestSceneOpenAndCloseEventsAndHints(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	ctx := t.Context()
	p1, _ := a.h.newScene(a.mapID, "A carroça", true, 1, sceneSpec{"skill:investigation", new(int32(12))})
	p2, _ := a.h.newScene(a.mapID, "A ponte", false, 1)
	mw, pw := a.master.watch(t, a.campaignID), a.caio.watch(t, a.campaignID)
	mw.ready(t)
	pw.ready(t)

	// Closing with none open changes nothing.
	if _, err := a.master.play.CloseScene(ctx, connect.NewRequest(&playv1.CloseSceneRequest{CampaignId: a.campaignID})); err != nil {
		t.Fatalf("CloseScene(none open) error = %v", err)
	}
	if got := sceneKinds(t, a); got != "" {
		t.Errorf("events after closing nothing = %q, want none", got)
	}
	if got := sceneHints(t, a, mw, pw); got[0] != "" || got[1] != "" {
		t.Errorf("hints after closing nothing = %q, want none", got)
	}

	a.openScene(t, p1)
	if got := sceneKinds(t, a); got != "scene_opened" {
		t.Errorf("events = %q, want scene_opened", got)
	}
	// Both the master and the player hear that the scene changed.
	if got := sceneHints(t, a, mw, pw); got[0] != "scene_changed" || got[1] != "scene_changed" {
		t.Errorf("hints after opening = %q, want scene_changed for each", got)
	}
	// Opening the open scene again: the answer is the scene, nothing is written or published.
	a.openScene(t, p1)
	if got := sceneKinds(t, a); got != "scene_opened" {
		t.Errorf("events after opening it again = %q, want one scene_opened", got)
	}
	if got := sceneHints(t, a, mw, pw); got[0] != "" || got[1] != "" {
		t.Errorf("hints after opening it again = %q, want none", got)
	}

	// Another scene replaces it and clears the stage (only then does the stage hint go).
	a.onStage(t, a.goblin)
	a.openScene(t, p2)
	if got := sceneKinds(t, a); got != "scene_opened,scene_opened,stage_changed" {
		t.Errorf("events after the second scene = %q", got)
	}
	if got := sceneHints(t, a, mw, pw); got[0] != "scene_changed,stage_changed" || got[1] != "scene_changed,stage_changed" {
		t.Errorf("hints after the second scene = %q", got)
	}
	var onStage int
	if err := a.h.pool.QueryRow(ctx, `SELECT count(*) FROM stage_npcs`).Scan(&onStage); err != nil || onStage != 0 {
		t.Errorf("NPCs on the stage = %d, %v; want 0", onStage, err)
	}

	// Opening one with an empty stage publishes no stage hint.
	a.openScene(t, p1)
	if got := sceneHints(t, a, mw, pw); got[0] != "scene_changed" {
		t.Errorf("hints after a third scene = %q, want scene_changed alone", got)
	}

	// Closing: scene_closed, the scene is gone for everyone, the stage is emptied.
	a.onStage(t, a.goblin)
	if _, err := a.master.play.CloseScene(ctx, connect.NewRequest(&playv1.CloseSceneRequest{CampaignId: a.campaignID})); err != nil {
		t.Fatalf("CloseScene() error = %v", err)
	}
	if got := sceneKinds(t, a); got != "scene_opened,scene_opened,stage_changed,scene_opened,scene_closed,stage_changed" {
		t.Errorf("events after closing = %q", got)
	}
	if got := sceneHints(t, a, mw, pw); got[0] != "scene_changed,stage_changed" {
		t.Errorf("hints after closing = %q", got)
	}
	for _, u := range []*user{a.master, a.caio} {
		if got := sceneRead(t, a, u, nil); got != "<no scene>" {
			t.Errorf("read after closing = %q, want no scene", got)
		}
	}
	// Closed twice: nothing more.
	if _, err := a.master.play.CloseScene(ctx, connect.NewRequest(&playv1.CloseSceneRequest{CampaignId: a.campaignID})); err != nil {
		t.Fatalf("CloseScene() again error = %v", err)
	}
	if got := sceneHints(t, a, mw, pw); got[0] != "" {
		t.Errorf("hints after closing twice = %q, want none", got)
	}
}

// A point that stopped being a scene since it was opened reads as no scene, and the calls that need
// one say so.
func TestSceneThePointStoppedBeingAScene(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	ctx := t.Context()
	point, actions := a.h.newScene(a.mapID, "A carroça", true, 2, sceneSpec{"skill:investigation", new(int32(12))})
	a.openScene(t, point)
	if _, err := a.h.pool.Exec(ctx, `UPDATE map_points SET kind = 'battle' WHERE id = $1`, point); err != nil {
		t.Fatalf("change the kind: %v", err)
	}
	for _, u := range []*user{a.master, a.caio} {
		if got := sceneRead(t, a, u, nil); got != "<no scene>" {
			t.Errorf("read = %q, want no scene", got)
		}
	}
	_, err := sceneInApp(a, a.caio, actions[0], newKey(), 10)
	wantSceneRefusal(t, "RollSceneCheck", err, connect.CodeFailedPrecondition, sceneNoScene, "no scene is open")
	_, err = sceneGrant(a, a.master, actions[0], a.toren.GetId(), newKey())
	wantSceneRefusal(t, "GrantSceneAttempt", err, connect.CodeFailedPrecondition, sceneNoScene, "no scene is open")
	// Closing still works: the session's pointer is cleared.
	if _, err := a.master.play.CloseScene(ctx, connect.NewRequest(&playv1.CloseSceneRequest{CampaignId: a.campaignID})); err != nil {
		t.Fatalf("CloseScene() error = %v", err)
	}
	if got := sceneKinds(t, a); got != "scene_opened,scene_closed" {
		t.Errorf("events = %q", got)
	}
}

func TestSceneRollRefusals(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	ctx := t.Context()
	point, actions := a.h.newScene(a.mapID, "A carroça", true, 1,
		sceneSpec{"skill:investigation", new(int32(12))}, sceneSpec{"skill:nonsense", nil})
	outsider := a.h.newUser("Fora")
	idle := a.h.newUser("Sem ficha")
	a.h.join(a.master, a.campaignID, idle)
	good := &playv1.RollSceneCheckRequest{Roll: &playv1.RollSceneCheckRequest_RollInApp{RollInApp: true}}
	roll := func(u *user, campaignID, action, key string, r *playv1.RollSceneCheckRequest) error {
		_, err := sceneRollWith(a, u, campaignID, action, key, &playv1.RollSceneCheckRequest{Roll: r.Roll, D20Faces: r.D20Faces})
		return err
	}

	// Who may call.
	wantSceneRefusal(t, "master", roll(a.master, a.campaignID, actions[0], newKey(), good), connect.CodePermissionDenied, sceneNone, "only the campaign's player can do this")
	wantSceneRefusal(t, "outsider", roll(outsider, a.campaignID, actions[0], newKey(), good), connect.CodeNotFound, sceneNone, "campaign not found")

	// The request is read before anything is looked up, in this order: key, action, the roll.
	wantSceneRefusal(t, "bad key", roll(a.caio, a.campaignID, "nope", "nope", good), connect.CodeInvalidArgument, sceneNone, "idempotency_key must be a UUID")
	wantSceneRefusal(t, "bad action", roll(a.caio, a.campaignID, "nope", newKey(), &playv1.RollSceneCheckRequest{}), connect.CodeNotFound, sceneNone, "action not found")
	wantSceneRefusal(t, "roll_in_app false", roll(a.caio, a.campaignID, actions[0], newKey(), &playv1.RollSceneCheckRequest{Roll: &playv1.RollSceneCheckRequest_RollInApp{}}), connect.CodeInvalidArgument, sceneNone, "roll_in_app must be true")
	for _, face := range []int32{0, 21, -3} {
		wantSceneRefusal(t, fmt.Sprintf("d20_face %d", face), roll(a.caio, a.campaignID, actions[0], newKey(), &playv1.RollSceneCheckRequest{Roll: &playv1.RollSceneCheckRequest_D20Face{D20Face: face}}), connect.CodeInvalidArgument, sceneNone, "d20_face must be 1 to 20")
	}
	wantSceneRefusal(t, "no roll", roll(a.caio, a.campaignID, actions[0], newKey(), &playv1.RollSceneCheckRequest{}), connect.CodeInvalidArgument, sceneNone, "set roll_in_app, d20_face or d20_faces")
	wantSceneRefusal(t, "three faces", roll(a.caio, a.campaignID, actions[0], newKey(), &playv1.RollSceneCheckRequest{D20Faces: []int32{1, 2, 3}}), connect.CodeInvalidArgument, sceneNone, "set roll_in_app, d20_face or d20_faces")
	wantSceneRefusal(t, "face out of range in d20_faces", roll(a.caio, a.campaignID, actions[0], newKey(), &playv1.RollSceneCheckRequest{D20Faces: []int32{5, 21}}), connect.CodeInvalidArgument, sceneNone, "d20_faces must be 1 to 20 each")

	// The request is valid; the session, the scene, the action, the character.
	lone, _ := sceneLoneCampaign(t, a)
	wantSceneNoSession(t, "no session", roll(a.caio, lone, actions[0], newKey(), good))
	wantSceneRefusal(t, "no scene open", roll(a.caio, a.campaignID, actions[0], newKey(), good), connect.CodeFailedPrecondition, sceneNoScene, "no scene is open")
	a.openScene(t, point)
	wantSceneRefusal(t, "action of another scene", roll(a.caio, a.campaignID, newKey(), newKey(), good), connect.CodeNotFound, sceneNone, "action not found")
	wantSceneRefusal(t, "a member with no character", roll(idle, a.campaignID, actions[0], newKey(), good), connect.CodeFailedPrecondition, sceneNoChar, "you have no living character to roll for")

	// The campaign's dice mode: a typed roll when the app must roll, and the other way round.
	a.forceDice(t, campaignsv1.DiceMode_DICE_MODE_APP)
	wantSceneRefusal(t, "typed when the app rolls", roll(a.caio, a.campaignID, actions[0], newKey(), &playv1.RollSceneCheckRequest{Roll: &playv1.RollSceneCheckRequest_D20Face{D20Face: 12}}), connect.CodeFailedPrecondition, sceneWrongDi, "this is not how the campaign has you roll your dice")
	a.forceDice(t, campaignsv1.DiceMode_DICE_MODE_PHYSICAL)
	wantSceneRefusal(t, "in-app when the dice are physical", roll(a.caio, a.campaignID, actions[0], newKey(), good), connect.CodeFailedPrecondition, sceneWrongDi, "this is not how the campaign has you roll your dice")
	a.forceDice(t, campaignsv1.DiceMode_DICE_MODE_PLAYERS_CHOOSE)

	// A check the character has no numbers for.
	wantSceneRefusal(t, "unknown check", roll(a.caio, a.campaignID, actions[1], newKey(), good), connect.CodeFailedPrecondition, sceneNoChar, "your character has no numbers for this check")

	// One attempt: the second is refused, until the master grants another.
	if _, err := sceneInApp(a, a.caio, actions[0], newKey(), 10); err != nil {
		t.Fatalf("first roll error = %v", err)
	}
	_, err := sceneInApp(a, a.caio, actions[0], newKey(), 10)
	wantSceneRefusal(t, "second roll", err, connect.CodeFailedPrecondition, sceneAlready, "no attempt left at this action; the master may grant one more")
	if _, err := sceneInApp(a, a.ana, actions[0], newKey(), 10); err != nil {
		t.Errorf("another character's first roll error = %v, want it allowed", err)
	}
	if _, err := sceneGrant(a, a.master, actions[0], a.toren.GetId(), newKey()); err != nil {
		t.Fatalf("GrantSceneAttempt() error = %v", err)
	}
	if _, err := sceneInApp(a, a.caio, actions[0], newKey(), 10); err != nil {
		t.Errorf("roll after the grant error = %v, want it allowed", err)
	}
	_, err = sceneInApp(a, a.caio, actions[0], newKey(), 10)
	wantSceneRefusal(t, "roll after the grant is used", err, connect.CodeFailedPrecondition, sceneAlready, "no attempt left at this action; the master may grant one more")

	// A new opening of the scene starts the count over.
	if _, err := a.master.play.CloseScene(ctx, connect.NewRequest(&playv1.CloseSceneRequest{CampaignId: a.campaignID})); err != nil {
		t.Fatalf("CloseScene() error = %v", err)
	}
	a.openScene(t, point)
	if _, err := sceneInApp(a, a.caio, actions[0], newKey(), 10); err != nil {
		t.Errorf("roll in a new opening error = %v, want it allowed", err)
	}
}

func TestSceneRollRetryAndAnswer(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	point, actions := a.h.newScene(a.mapID, "A carroça", true, 3,
		sceneSpec{"skill:investigation", new(int32(12))}, sceneSpec{"skill:athletics", nil})
	a.openScene(t, point)
	mw, pw, ow := a.master.watch(t, a.campaignID), a.ana.watch(t, a.campaignID), a.caio.watch(t, a.campaignID)
	mw.ready(t)
	pw.ready(t)
	ow.ready(t)

	key := newKey()
	first, err := sceneInApp(a, a.ana, actions[0], key, 14)
	if err != nil {
		t.Fatalf("RollSceneCheck() error = %v", err)
	}
	r := first.GetRoll()
	if r.GetActionId() != actions[0] || r.GetCharacterId() != a.pens.GetId() || r.GetCharacterName() != "Pensantus" || r.GetRoll().GetFaces()[0] != 14 ||
		r.GetRoll().GetTotal() != 14+r.GetRoll().GetModifier() || r.GetPassed() != (r.GetRoll().GetTotal() >= 12) || r.Passed == nil || r.GetRoll().GetPhysical() || len(r.GetSources()) != 0 {
		t.Errorf("the roll = %v", r)
	}
	if r.GetRoll().GetModifier() != 4 || r.GetRoll().GetTotal() != 18 || r.GetMode() != playv1.RollMode_ROLL_MODE_NORMAL || !r.GetPassed() {
		t.Errorf("the roll = %v, want 14 + 4 = 18, normal, a pass against DC 12", r)
	}
	// Only the master and the roller hear of it.
	if got := sceneHints(t, a, mw, pw, ow); got[0] != "scene_check_rolled" || got[1] != "scene_check_rolled" || got[2] != "" {
		t.Errorf("hints = %q, want scene_check_rolled for the master and the roller alone", got)
	}

	// The same request again: the same roll (same id, same time), nothing rolled; the master and the roller hear of it again.
	a.h.roller.queue(3)
	again, err := sceneRollWith(a, a.ana, a.campaignID, actions[0], key, &playv1.RollSceneCheckRequest{Roll: &playv1.RollSceneCheckRequest_RollInApp{RollInApp: true}})
	if err != nil {
		t.Fatalf("the retry error = %v", err)
	}
	if again.GetRoll().GetId() != r.GetId() || again.GetRoll().GetRoll().GetFaces()[0] != 14 || again.GetRoll().GetCharacterName() != "Pensantus" ||
		!again.GetRoll().GetRolledAt().AsTime().Equal(r.GetRolledAt().AsTime()) {
		t.Errorf("the retry = %v, want the first roll %v", again, first)
	}
	if got := sceneHints(t, a, mw, pw); got[0] != "scene_check_rolled" || got[1] != "scene_check_rolled" {
		t.Errorf("hints after the retry = %q, want the first call's again", got)
	}
	if n := a.countSceneRolls(t); n != 1 {
		t.Errorf("%d rolls after the retry, want 1", n)
	}
	if left, _ := a.h.roller.Roll(20); left != 3 {
		t.Errorf("the retry used the roller (next = %d, want the queued 3)", left)
	}

	// The key is the roller's own, and bound to what was asked.
	a.h.roller.queue(5)
	_, err = sceneRollWith(a, a.caio, a.campaignID, actions[0], key, &playv1.RollSceneCheckRequest{Roll: &playv1.RollSceneCheckRequest_RollInApp{RollInApp: true}})
	wantSceneRefusal(t, "another player's retry", err, connect.CodeInvalidArgument, sceneNone, "idempotency_key was already used for another change")
	_, err = sceneRollWith(a, a.ana, a.campaignID, actions[1], key, &playv1.RollSceneCheckRequest{Roll: &playv1.RollSceneCheckRequest_RollInApp{RollInApp: true}})
	wantSceneRefusal(t, "same key, another action", err, connect.CodeInvalidArgument, sceneNone, "idempotency_key was already used for another change")
	_, err = sceneRollWith(a, a.ana, a.campaignID, actions[0], key, &playv1.RollSceneCheckRequest{Roll: &playv1.RollSceneCheckRequest_D20Face{D20Face: 9}})
	wantSceneRefusal(t, "same key, another roll", err, connect.CodeInvalidArgument, sceneNone, "idempotency_key was already used for another change")
	// A key spent on another kind of change.
	_, err = sceneRollWith(a, a.ana, a.campaignID, actions[0], key, &playv1.RollSceneCheckRequest{Roll: &playv1.RollSceneCheckRequest_RollInApp{RollInApp: true}, ExtraDieFaces: []int32{4}})
	wantSceneRefusal(t, "same key, an extra die", err, connect.CodeInvalidArgument, sceneNone, "idempotency_key was already used for another change")
	grantKey := newKey()
	if _, err := sceneGrant(a, a.master, actions[0], a.toren.GetId(), grantKey); err != nil {
		t.Fatalf("GrantSceneAttempt() error = %v", err)
	}
	_, err = sceneRollWith(a, a.ana, a.campaignID, actions[0], grantKey, &playv1.RollSceneCheckRequest{Roll: &playv1.RollSceneCheckRequest_RollInApp{RollInApp: true}})
	wantSceneRefusal(t, "a grant's key", err, connect.CodeInvalidArgument, sceneNone, "idempotency_key was already used for another change")

	// Typed dice: one face.
	typed, err := a.rollSceneTyped(t, a.caio, actions[1], newKey(), 17)
	if err != nil {
		t.Fatalf("typed roll error = %v", err)
	}
	if tr := typed.GetRoll(); tr.GetRoll().GetFaces()[0] != 17 || tr.Passed != nil || !tr.GetRoll().GetPhysical() || tr.GetCharacterName() != "Toren" {
		t.Errorf("the typed roll = %v, want 17, physical, no pass or fail (no DC)", tr)
	}
	if tr := typed.GetRoll(); tr.GetRoll().GetModifier() != 3 || tr.GetRoll().GetTotal() != 20 {
		t.Errorf("the typed roll = %v, want 17 + 3 = 20", tr)
	}
	// d20_faces with one face rolls like d20_face.
	multi, err := sceneRollWith(a, a.bia, a.campaignID, actions[1], newKey(), &playv1.RollSceneCheckRequest{D20Faces: []int32{8}})
	if err != nil || multi.GetRoll().GetRoll().GetFaces()[0] != 8 || !multi.GetRoll().GetRoll().GetPhysical() {
		t.Errorf("d20_faces [8] = %v, %v; want a physical 8", multi, err)
	}
	// Two faces with no advantage: refused by the roll (the mode needs one).
	_, err = sceneRollWith(a, a.bia, a.campaignID, actions[1], newKey(), &playv1.RollSceneCheckRequest{D20Faces: []int32{8, 9}})
	wantSceneRefusal(t, "two faces without advantage", err, connect.CodeInvalidArgument, sceneNone, "the roll takes 1 d20: type 1 face(s) in d20_faces")
}

// What each viewer reads of the open scene (RN-20): the master the DCs, every roll with its pass or
// fail and the attempts left, the hooks, the clues with their recipients and the images; a player
// their own bonus, passive and attempts left, their own rolls, the DCs and the pass or fail only when
// the scene shows its DC; a member with no character and a stranger.
func TestSceneReadByEachViewer(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	ctx := t.Context()
	point, actions := a.h.newScene(a.mapID, "A carroça", true, 2,
		sceneSpec{"skill:investigation", new(int32(12))}, sceneSpec{"skill:athletics", new(int32(10))}, sceneSpec{"ability:str", nil})
	hidden, hiddenActions := a.h.newScene(a.mapID, "A taverna", false, 0, sceneSpec{"skill:persuasion", new(int32(10))})
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`UPDATE map_points SET description = 'Uma carroça tombada.', hooks = 'O cocheiro mente.' WHERE id = $1`, []any{point}},
		{`UPDATE scene_actions SET name = 'Revistar' WHERE id = $1`, []any{actions[0]}},
	} {
		if _, err := a.h.pool.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatalf("prepare the scene: %v", err)
		}
	}
	clue1, clue2 := newKey(), newKey()
	image := newKey()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO scene_clues (id, point_id, position, text, created_at, updated_at) VALUES ($1, $2, 0, 'Marcas de bota.', now(), now())`, []any{clue1, point}},
		{`INSERT INTO scene_clues (id, point_id, position, text, created_at, updated_at) VALUES ($1, $2, 1, 'Uma moeda.', now(), now())`, []any{clue2, point}},
		{`INSERT INTO scene_clue_reveals (campaign_id, clue_id, point_id, user_id, character_id, text, revealed_at) VALUES ($1, $2, $3, $4, $5, 'Marcas de bota.', now())`, []any{a.campaignID, clue1, point, a.caio.id, a.toren.GetId()}},
		{`INSERT INTO gallery_images (id, campaign_id, name, content_type, width, height, byte_size, created_at) VALUES ($1, $2, 'Carroça.png', 'image/png', 100, 100, 100, now())`, []any{image, a.campaignID}},
		{`INSERT INTO map_point_images (point_id, image_id, position, created_at) VALUES ($1, $2, 0, now())`, []any{point, image}},
	} {
		if _, err := a.h.pool.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatalf("prepare the scene: %v", err)
		}
	}
	outsider := a.h.newUser("Fora")
	idle := a.h.newUser("Sem ficha")
	a.h.join(a.master, a.campaignID, idle)
	labels := sceneLabels(actions)
	for id, l := range sceneLabels(hiddenActions) {
		labels[id] = "H" + l[1:]
	}
	viewers := func() map[string]string {
		out := map[string]string{}
		for who, u := range map[string]*user{"master": a.master, "toren": a.caio, "pensantus": a.ana, "brisa": a.bia, "idle": idle, "outsider": outsider} {
			out[who] = sceneRead(t, a, u, labels)
		}
		return out
	}
	// Nothing open: every member reads no scene, a stranger is turned away.
	got := viewers()
	for who, text := range got {
		want := "<no scene>"
		if who == "outsider" {
			want = "error: not_found: campaign not found"
		}
		wantSceneText(t, "before opening, "+who, text, want)
	}

	a.openScene(t, point)
	a.checkRoll(t, a.caio, actions[0], 15)
	a.checkRoll(t, a.caio, actions[0], 3)
	a.checkRoll(t, a.ana, actions[1], 20)
	a.checkRoll(t, a.ana, actions[2], 4)
	a.checkRoll(t, a.bia, actions[1], 2)
	if _, err := sceneGrant(a, a.master, actions[1], a.pens.GetId(), newKey()); err != nil {
		t.Fatalf("GrantSceneAttempt() error = %v", err)
	}
	got = viewers()
	const shownMaster = `scene "A carroça" desc="Uma carroça tombada." show_dc=true opened=true hooks="O cocheiro mente."
action A1 key=skill:investigation name="Revistar" check="Investigação" max=2 dc=12 bonus=- passive=- left=-
action A2 key=skill:athletics name="" check="Atletismo" max=2 dc=10 bonus=- passive=- left=-
action A3 key=ability:str name="" check="Teste de Força" max=2 dc=0 bonus=- passive=- left=-
clue "Marcas de bota." to=[Toren]
clue "Uma moeda." to=[]
image "Carroça.png" whole=false
roll A2 by="Brisa" faces=[2] mod=0 total=2 passed=false left=1 physical=false mode=ROLL_MODE_NORMAL
roll A3 by="Pensantus" faces=[4] mod=0 total=4 passed=- left=1 physical=false mode=ROLL_MODE_NORMAL
roll A2 by="Pensantus" faces=[20] mod=0 total=20 passed=true left=2 physical=false mode=ROLL_MODE_NORMAL
roll A1 by="Toren" faces=[3] mod=0 total=3 passed=false left=0 physical=false mode=ROLL_MODE_NORMAL
roll A1 by="Toren" faces=[15] mod=0 total=15 passed=true left=0 physical=false mode=ROLL_MODE_NORMAL
stage=0`
	const shownToren = `scene "A carroça" desc="Uma carroça tombada." show_dc=true opened=true hooks=""
action A1 key=skill:investigation name="Revistar" check="Investigação" max=2 dc=12 bonus=0 passive=10 left=0
action A2 key=skill:athletics name="" check="Atletismo" max=2 dc=10 bonus=3 passive=- left=2
action A3 key=ability:str name="" check="Teste de Força" max=2 dc=0 bonus=3 passive=- left=2
roll A1 by="Toren" faces=[3] mod=0 total=3 passed=false left=- physical=false mode=ROLL_MODE_NORMAL
roll A1 by="Toren" faces=[15] mod=0 total=15 passed=true left=- physical=false mode=ROLL_MODE_NORMAL
stage=0`
	const shownPensantus = `scene "A carroça" desc="Uma carroça tombada." show_dc=true opened=true hooks=""
action A1 key=skill:investigation name="Revistar" check="Investigação" max=2 dc=12 bonus=4 passive=14 left=2
action A2 key=skill:athletics name="" check="Atletismo" max=2 dc=10 bonus=0 passive=- left=2
action A3 key=ability:str name="" check="Teste de Força" max=2 dc=0 bonus=0 passive=- left=1
roll A3 by="Pensantus" faces=[4] mod=0 total=4 passed=- left=- physical=false mode=ROLL_MODE_NORMAL
roll A2 by="Pensantus" faces=[20] mod=0 total=20 passed=true left=- physical=false mode=ROLL_MODE_NORMAL
stage=0`
	const shownBrisa = `scene "A carroça" desc="Uma carroça tombada." show_dc=true opened=true hooks=""
action A1 key=skill:investigation name="Revistar" check="Investigação" max=2 dc=12 bonus=0 passive=10 left=2
action A2 key=skill:athletics name="" check="Atletismo" max=2 dc=10 bonus=0 passive=- left=1
action A3 key=ability:str name="" check="Teste de Força" max=2 dc=0 bonus=0 passive=- left=2
roll A2 by="Brisa" faces=[2] mod=0 total=2 passed=false left=- physical=false mode=ROLL_MODE_NORMAL
stage=0`
	const shownIdle = `scene "A carroça" desc="Uma carroça tombada." show_dc=true opened=true hooks=""
action A1 key=skill:investigation name="Revistar" check="Investigação" max=2 dc=12 bonus=- passive=- left=-
action A2 key=skill:athletics name="" check="Atletismo" max=2 dc=10 bonus=- passive=- left=-
action A3 key=ability:str name="" check="Teste de Força" max=2 dc=0 bonus=- passive=- left=-
stage=0`
	wantSceneText(t, "master", got["master"], shownMaster)
	wantSceneText(t, "Toren", got["toren"], shownToren)
	wantSceneText(t, "Pensantus", got["pensantus"], shownPensantus)
	wantSceneText(t, "Brisa", got["brisa"], shownBrisa)
	wantSceneText(t, "a member with no character", got["idle"], shownIdle)
	wantSceneText(t, "a stranger", got["outsider"], "error: not_found: campaign not found")

	// The scene that hides its DC: a player gets no DC and no pass or fail, the master both.
	a.openScene(t, hidden)
	a.checkRoll(t, a.caio, hiddenActions[0], 18)
	wantSceneText(t, "hidden DC, master", sceneRead(t, a, a.master, labels), `scene "A taverna" desc="" show_dc=false opened=true hooks=""
action H1 key=skill:persuasion name="" check="Persuasão" max=0 dc=10 bonus=- passive=- left=-
roll H1 by="Toren" faces=[18] mod=-1 total=17 passed=true left=- physical=false mode=ROLL_MODE_NORMAL
stage=0`)
	const hiddenToren = `scene "A taverna" desc="" show_dc=false opened=true hooks=""
action H1 key=skill:persuasion name="" check="Persuasão" max=0 dc=0 bonus=-1 passive=- left=-
roll H1 by="Toren" faces=[18] mod=-1 total=17 passed=- left=- physical=false mode=ROLL_MODE_NORMAL
stage=0`
	wantSceneText(t, "hidden DC, Toren", sceneRead(t, a, a.caio, labels), hiddenToren)
	// Turning the DC on later shows the DC, but not the pass or fail of the earlier rolls.
	if _, err := a.h.pool.Exec(ctx, `UPDATE map_points SET show_dc = true WHERE id = $1`, hidden); err != nil {
		t.Fatalf("show the DC: %v", err)
	}
	wantSceneText(t, "DC turned on later, Toren", sceneRead(t, a, a.caio, labels), `scene "A taverna" desc="" show_dc=true opened=true hooks=""
action H1 key=skill:persuasion name="" check="Persuasão" max=0 dc=10 bonus=-1 passive=- left=-
roll H1 by="Toren" faces=[18] mod=-1 total=17 passed=- left=- physical=false mode=ROLL_MODE_NORMAL
stage=0`)
}

func TestSceneGrantAttemptRefusalsAndEffects(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	ctx := t.Context()
	point, actions := a.h.newScene(a.mapID, "A carroça", true, 1, sceneSpec{"skill:investigation", new(int32(12))})
	_, unlimited := a.h.newScene(a.mapID, "A ponte", true, 0, sceneSpec{"skill:perception", nil})
	outsider := a.h.newUser("Fora")
	grant := func(u *user, campaignID, action, character, key string) error {
		_, err := u.play.GrantSceneAttempt(ctx, connect.NewRequest(&playv1.GrantSceneAttemptRequest{CampaignId: campaignID, ActionId: action, CharacterId: character, IdempotencyKey: key}))
		return err
	}
	toren := a.toren.GetId()
	wantSceneRefusal(t, "player", grant(a.caio, a.campaignID, actions[0], toren, newKey()), connect.CodePermissionDenied, sceneNone, "only the campaign's master can do this")
	wantSceneRefusal(t, "outsider", grant(outsider, a.campaignID, actions[0], toren, newKey()), connect.CodeNotFound, sceneNone, "campaign not found")
	// Parsed in this order: key, action, character.
	wantSceneRefusal(t, "bad key", grant(a.master, a.campaignID, "nope", "nope", "nope"), connect.CodeInvalidArgument, sceneNone, "idempotency_key must be a UUID")
	wantSceneRefusal(t, "bad action", grant(a.master, a.campaignID, "nope", "nope", newKey()), connect.CodeNotFound, sceneNone, "action not found")
	wantSceneRefusal(t, "bad character", grant(a.master, a.campaignID, actions[0], "nope", newKey()), connect.CodeNotFound, sceneNone, "character not found")

	lone, _ := sceneLoneCampaign(t, a)
	wantSceneNoSession(t, "no session", grant(a.master, lone, actions[0], toren, newKey()))
	wantSceneRefusal(t, "no scene open", grant(a.master, a.campaignID, actions[0], toren, newKey()), connect.CodeFailedPrecondition, sceneNoScene, "no scene is open")
	a.openScene(t, point)
	wantSceneRefusal(t, "action of another scene", grant(a.master, a.campaignID, unlimited[0], toren, newKey()), connect.CodeNotFound, sceneNone, "action not found")
	// The character must be in the party; an NPC and an unknown ID are not.
	wantSceneRefusal(t, "an NPC", grant(a.master, a.campaignID, actions[0], a.goblin.GetId(), newKey()), connect.CodeNotFound, sceneNone, "character not found")
	wantSceneRefusal(t, "unknown character", grant(a.master, a.campaignID, actions[0], newKey(), newKey()), connect.CodeNotFound, sceneNone, "character not found")

	mw, pw := a.master.watch(t, a.campaignID), a.caio.watch(t, a.campaignID)
	mw.ready(t)
	pw.ready(t)
	labels := sceneLabels(actions)
	a.checkRoll(t, a.caio, actions[0], 10)
	before := sceneRead(t, a, a.caio, labels)
	_ = sceneHints(t, a, mw, pw)

	key := newKey()
	res, err := sceneGrant(a, a.master, actions[0], toren, key)
	if err != nil {
		t.Fatalf("GrantSceneAttempt() error = %v", err)
	}
	wantSceneText(t, "Toren before the grant", before, `scene "A carroça" desc="" show_dc=true opened=true hooks=""
action A1 key=skill:investigation name="" check="Investigação" max=1 dc=12 bonus=0 passive=10 left=0
roll A1 by="Toren" faces=[10] mod=0 total=10 passed=false left=- physical=false mode=ROLL_MODE_NORMAL
stage=0`)
	// The answer is the scene as the master reads it, with the attempts left on the roll.
	wantSceneText(t, "the master's answer", sceneRender(res.GetScene(), labels), `scene "A carroça" desc="" show_dc=true opened=true hooks=""
action A1 key=skill:investigation name="" check="Investigação" max=1 dc=12 bonus=- passive=- left=-
roll A1 by="Toren" faces=[10] mod=0 total=10 passed=false left=1 physical=false mode=ROLL_MODE_NORMAL
stage=0`)
	if got := sceneKinds(t, a); got != "scene_opened,scene_check_rolled,scene_attempt_granted" {
		t.Errorf("events = %q", got)
	}
	if got := sceneHints(t, a, mw, pw); got[0] != "scene_changed" || got[1] != "scene_changed" {
		t.Errorf("hints after the grant = %q, want scene_changed for each", got)
	}
	wantSceneText(t, "Toren after the grant", sceneRead(t, a, a.caio, labels), `scene "A carroça" desc="" show_dc=true opened=true hooks=""
action A1 key=skill:investigation name="" check="Investigação" max=1 dc=12 bonus=0 passive=10 left=1
roll A1 by="Toren" faces=[10] mod=0 total=10 passed=false left=- physical=false mode=ROLL_MODE_NORMAL
stage=0`)

	// The same grant again: the answer is the scene, nothing is written, and the hint goes again.
	if _, err := sceneGrant(a, a.master, actions[0], toren, key); err != nil {
		t.Fatalf("the retry error = %v", err)
	}
	if got := sceneKinds(t, a); got != "scene_opened,scene_check_rolled,scene_attempt_granted" {
		t.Errorf("events after the retry = %q", got)
	}
	if got := sceneHints(t, a, mw, pw); got[0] != "scene_changed" || got[1] != "scene_changed" {
		t.Errorf("hints after the retry = %q, want the first call's again", got)
	}
	// The key is bound to what was granted.
	wantSceneRefusal(t, "same key, another character", grant(a.master, a.campaignID, actions[0], a.pens.GetId(), key), connect.CodeInvalidArgument, sceneNone, "idempotency_key was already used for another change")
	rollKey := newKey()
	if _, err := sceneInApp(a, a.caio, actions[0], rollKey, 10); err != nil {
		t.Fatalf("the granted roll error = %v", err)
	}
	wantSceneRefusal(t, "a roll's key", grant(a.master, a.campaignID, actions[0], toren, rollKey), connect.CodeInvalidArgument, sceneNone, "idempotency_key was already used for another change")

	// A second grant is another event; it adds up.
	if _, err := sceneGrant(a, a.master, actions[0], toren, newKey()); err != nil {
		t.Fatalf("a second grant error = %v", err)
	}
	wantSceneText(t, "Toren after two grants and a second roll", sceneRead(t, a, a.caio, labels), `scene "A carroça" desc="" show_dc=true opened=true hooks=""
action A1 key=skill:investigation name="" check="Investigação" max=1 dc=12 bonus=0 passive=10 left=1
roll A1 by="Toren" faces=[10] mod=0 total=10 passed=false left=- physical=false mode=ROLL_MODE_NORMAL
roll A1 by="Toren" faces=[10] mod=0 total=10 passed=false left=- physical=false mode=ROLL_MODE_NORMAL
stage=0`)
	// A new opening starts afresh: the grants no longer count.
	if _, err := a.master.play.CloseScene(ctx, connect.NewRequest(&playv1.CloseSceneRequest{CampaignId: a.campaignID})); err != nil {
		t.Fatalf("CloseScene() error = %v", err)
	}
	a.openScene(t, point)
	wantSceneText(t, "Toren in a new opening", sceneRead(t, a, a.caio, labels), `scene "A carroça" desc="" show_dc=true opened=true hooks=""
action A1 key=skill:investigation name="" check="Investigação" max=1 dc=12 bonus=0 passive=10 left=1
stage=0`)
}

// An unlimited action has nothing to grant: the call answers with the scene and writes and tells nothing.
func TestSceneGrantAttemptOnAnUnlimitedAction(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	point, actions := a.h.newScene(a.mapID, "A ponte", true, 0, sceneSpec{"skill:perception", new(int32(14))})
	a.openScene(t, point)
	mw := a.master.watch(t, a.campaignID)
	mw.ready(t)
	res, err := sceneGrant(a, a.master, actions[0], a.toren.GetId(), newKey())
	if err != nil {
		t.Fatalf("GrantSceneAttempt() error = %v", err)
	}
	if res.GetScene() == nil || len(res.GetScene().GetActions()) != 1 {
		t.Errorf("answer = %v, want the scene", res)
	}
	if got := sceneKinds(t, a); got != "scene_opened" {
		t.Errorf("events = %q, want only scene_opened", got)
	}
	if got := sceneHints(t, a, mw); got[0] != "" {
		t.Errorf("hints = %q, want none", got)
	}
}

// A Poisoned character has disadvantage on ability checks: the roll keeps both d20 and the one that
// counts, in the answer and in the reads of the master and of the roller; the circumstance is named
// to the roller only in the answer.
func TestSceneRollWithDisadvantage(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	ctx := t.Context()
	if _, err := a.h.pool.Exec(ctx, `INSERT INTO character_effects (campaign_id, character_id, group_id, source_key, source_kind, condition_keys, duration_kind, player_visible)
		VALUES ($1, $2, gen_random_uuid(), 'effect:scene-test', 'master', ARRAY['condition:poisoned'], 'until_dismissed', true)`, a.campaignID, a.toren.GetId()); err != nil {
		t.Fatalf("poison Toren: %v", err)
	}
	point, actions := a.h.newScene(a.mapID, "A ponte", true, 0, sceneSpec{"skill:perception", new(int32(10))})
	a.openScene(t, point)
	labels := sceneLabels(actions)

	a.h.roller.queue(15, 6)
	res, err := sceneRollWith(a, a.caio, a.campaignID, actions[0], newKey(), &playv1.RollSceneCheckRequest{Roll: &playv1.RollSceneCheckRequest_RollInApp{RollInApp: true}})
	if err != nil {
		t.Fatalf("RollSceneCheck() error = %v", err)
	}
	r := res.GetRoll()
	if r.GetMode() != playv1.RollMode_ROLL_MODE_DISADVANTAGE || r.GetRoll().GetDiceCount() != 2 || r.GetRoll().GetFaces()[0] != 15 || r.GetRoll().GetFaces()[1] != 6 ||
		r.GetRoll().GetCountedIndex() != 1 || r.GetRoll().GetTotal() != 6+r.GetRoll().GetModifier() || len(r.GetSources()) != 1 || r.Passed == nil || r.GetPassed() {
		t.Errorf("the roll = %v, want 15 and 6 with disadvantage, the 6 counted, one source, a fail", r)
	}
	// Typed: two faces are needed, and the lower counts.
	typed, err := sceneRollWith(a, a.caio, a.campaignID, actions[0], newKey(), &playv1.RollSceneCheckRequest{D20Faces: []int32{19, 12}})
	if err != nil {
		t.Fatalf("typed RollSceneCheck() error = %v", err)
	}
	if tr := typed.GetRoll(); tr.GetRoll().GetCountedIndex() != 1 || tr.GetRoll().GetTotal() != 12+tr.GetRoll().GetModifier() || !tr.GetRoll().GetPhysical() || !tr.GetPassed() {
		t.Errorf("the typed roll = %v, want 19 and 12, the 12 counted, a pass", tr)
	}
	_, err = sceneRollWith(a, a.caio, a.campaignID, actions[0], newKey(), &playv1.RollSceneCheckRequest{D20Faces: []int32{19}})
	wantSceneRefusal(t, "one face with disadvantage", err, connect.CodeInvalidArgument, sceneNone, "the roll takes 2 d20: type 2 face(s) in d20_faces")

	want := `roll A1 by="Toren" faces=[19 12] mod=0 total=12 passed=true left=%s physical=true mode=ROLL_MODE_DISADVANTAGE
roll A1 by="Toren" faces=[15 6] mod=0 total=6 passed=false left=%s physical=false mode=ROLL_MODE_DISADVANTAGE`
	for who, u := range map[string]*user{"master": a.master, "toren": a.caio} {
		read := sceneRead(t, a, u, labels)
		lines := strings.Split(read, "\n")
		got := strings.Join(lines[len(lines)-3:len(lines)-1], "\n")
		if wantRolls := fmt.Sprintf(want, "-", "-"); got != wantRolls {
			t.Errorf("%s reads:\n%s\nwant:\n%s", who, got, wantRolls)
		}
	}
}

// The master reads a roll of an action that was removed since with no attempts left to show, and
// the players' reads go on; the open scene point is the one maps asks about.
func TestSceneRemovedActionAndOpenScenePoint(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	ctx := t.Context()
	point, actions := a.h.newScene(a.mapID, "A ponte", true, 2, sceneSpec{"skill:perception", new(int32(10))}, sceneSpec{"skill:athletics", nil})
	if got, err := a.h.svc.OpenScenePoint(ctx, a.campaignID); err != nil || got != "" {
		t.Errorf("OpenScenePoint() with none open = %q, %v; want empty", got, err)
	}
	lone, _ := sceneLoneCampaign(t, a)
	if got, err := a.h.svc.OpenScenePoint(ctx, lone); err != nil || got != "" {
		t.Errorf("OpenScenePoint() with no session = %q, %v; want empty", got, err)
	}
	a.openScene(t, point)
	if got, err := a.h.svc.OpenScenePoint(ctx, a.campaignID); err != nil || got != point {
		t.Errorf("OpenScenePoint() = %q, %v; want %q", got, err, point)
	}
	a.checkRoll(t, a.caio, actions[0], 10)
	a.checkRoll(t, a.caio, actions[1], 11)
	if _, err := a.h.pool.Exec(ctx, `DELETE FROM scene_actions WHERE id = $1`, actions[0]); err != nil {
		t.Fatalf("remove the action: %v", err)
	}
	labels := sceneLabels(actions)
	wantSceneText(t, "master", sceneRead(t, a, a.master, labels), `scene "A ponte" desc="" show_dc=true opened=true hooks=""
action A2 key=skill:athletics name="" check="Atletismo" max=2 dc=0 bonus=- passive=- left=-
roll A2 by="Toren" faces=[11] mod=3 total=14 passed=- left=1 physical=false mode=ROLL_MODE_NORMAL
roll A1 by="Toren" faces=[10] mod=0 total=10 passed=true left=- physical=false mode=ROLL_MODE_NORMAL
stage=0`)
	wantSceneText(t, "Toren", sceneRead(t, a, a.caio, labels), `scene "A ponte" desc="" show_dc=true opened=true hooks=""
action A2 key=skill:athletics name="" check="Atletismo" max=2 dc=0 bonus=3 passive=- left=1
roll A2 by="Toren" faces=[11] mod=3 total=14 passed=- left=- physical=false mode=ROLL_MODE_NORMAL
roll A1 by="Toren" faces=[10] mod=0 total=10 passed=true left=- physical=false mode=ROLL_MODE_NORMAL
stage=0`)
}

// A scene opened before the events were kept has no scene_opened event: the read has no opening
// time, and every roll of the session counts for it.
func TestSceneOpenedBeforeEventsWereKept(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	ctx := t.Context()
	point, actions := a.h.newScene(a.mapID, "A ponte", true, 1, sceneSpec{"skill:athletics", nil})
	if _, err := a.h.pool.Exec(ctx, `UPDATE game_sessions SET open_scene_point_id = $2 WHERE campaign_id = $1 AND ended_at IS NULL`, a.campaignID, point); err != nil {
		t.Fatalf("open the scene without an event: %v", err)
	}
	labels := sceneLabels(actions)
	a.checkRoll(t, a.caio, actions[0], 11)
	_, err := sceneInApp(a, a.caio, actions[0], newKey(), 11)
	wantSceneRefusal(t, "second roll", err, connect.CodeFailedPrecondition, sceneAlready, "no attempt left at this action; the master may grant one more")
	wantSceneText(t, "master", sceneRead(t, a, a.master, labels), `scene "A ponte" desc="" show_dc=true opened=false hooks=""
action A1 key=skill:athletics name="" check="Atletismo" max=1 dc=0 bonus=- passive=- left=-
roll A1 by="Toren" faces=[11] mod=3 total=14 passed=- left=0 physical=false mode=ROLL_MODE_NORMAL
stage=0`)
}
