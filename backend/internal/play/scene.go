package play

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"time"
	"uuid"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/timestamppb"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/platform/logging"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/live"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// The RP scene during a session (MR-015, Etapa 7, D7; questions 51 to 55 of
// the progress doc, with their defaults).
//
//   - The scene is a map point of kind SCENE with the master's actions on it
//     (package maps keeps them). The master opens one in the session
//     (game_sessions.open_scene_point_id), like showing an image: a hidden
//     point can be opened, and it stays hidden on the map.
//   - Every member reads the open scene (GetOpenScene). A player gets their
//     own character's bonus on each action, from the rules engine, and never
//     another player's roll. A DC, and the pass or fail of their own rolls,
//     come only when the master turned the scene's "Mostrar a CD aos
//     jogadores" on (map_points.show_dc, question 52, RN-20); the master
//     always gets the DCs and every roll.
//   - A player rolls each action as many times as the master allowed
//     (scene_actions.max_attempts: 1 to 5, or unlimited, question 55): the
//     app rolls the d20, or takes a typed one (RN-18). The master may give
//     one character one more attempt (GrantSceneAttempt). Closing the scene
//     and opening it again starts afresh: the rolls and the grants count for
//     the opening that is the session's latest `scene_opened` event.
//   - A scene may have no actions (question 63): any SCENE point opens. The
//     master also reads the point's hooks and clues in the open scene (MR-029);
//     a player never does (RN-20).
//   - Opening a scene makes it "discovered" for the group (MR-030).
//   - Opening, closing and rolling are session events (ADR-0007): ids and
//     numbers only, never a name or the master's words. A roll also keeps
//     whether the scene showed its DC when it was made: the session summary
//     counts only those (summary.go).
//   - The NPCs "em cena" (the stage, stage.go) go with the scene: every member
//     reads them in GetOpenScene, and closing or changing the scene empties
//     the stage.

// sceneEvent is the payload of scene_opened and scene_closed.
type sceneEvent struct {
	PointID string `json:"point_id"`
	// Actions is how many actions the scene had when it opened.
	Actions int `json:"actions,omitempty"`
}

// sceneRollEvent is the payload of scene_check_rolled. Numbers and ids only:
// the action's key is a rules key, never the master's name for it, and the DC
// itself is not kept, only whether the roll reached it.
type sceneRollEvent struct {
	PointID  string `json:"point_id"`
	ActionID string `json:"action_id"`
	Key      string `json:"key"`
	D20      int32  `json:"d20"`
	Modifier int32  `json:"modifier"`
	Total    int32  `json:"total"`
	Physical bool   `json:"physical,omitempty"`
	// Passed is set only when the action had a DC.
	Passed *bool `json:"passed,omitempty"`
	// DCShown says the scene showed its DC to the players when the roll was
	// made. The session summary counts a roll only when it is true, so a
	// scene that hid its DC never gives a pass or a fail to anyone (RN-20).
	DCShown bool `json:"dc_shown,omitempty"`
}

// sceneGrantEvent is the payload of scene_attempt_granted: the action; the
// character is the event's.
type sceneGrantEvent struct {
	PointID  string `json:"point_id"`
	ActionID string `json:"action_id"`
}

// errScene is the failed_precondition of the scene calls, with the
// SceneBlocked detail that tells the app why.
func errScene(reason playv1.SceneBlockedReason, msg string) error {
	err := connect.NewError(connect.CodeFailedPrecondition, errors.New(msg))
	if detail, detailErr := connect.NewErrorDetail(&playv1.SceneBlocked{Reason: reason}); detailErr == nil {
		err.AddDetail(detail)
	}
	return err
}

func errScenePointNotFound() error {
	return connect.NewError(connect.CodeNotFound, errors.New("scene point not found"))
}

func errSceneActionNotFound() error {
	return connect.NewError(connect.CodeNotFound, errors.New("action not found"))
}

// OpenScenePoint returns the map point of the scene open in the campaign's open
// session, "" when none is open or no session is. It implements
// maps.LiveSession.
func (s *Service) OpenScenePoint(ctx context.Context, campaignID string) (string, error) {
	point, err := s.queries.GetOpenScenePoint(ctx, campaignID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read the open scene: %w", err)
	}
	return deref(point), nil
}

// publishSceneChanged tells everyone in the session that the open scene
// changed. The hint names nothing: each app reads the scene again, filtered
// for it.
func (s *Service) publishSceneChanged(campaignID string) {
	s.Publish(campaignID, true, &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_SceneChanged_{
		SceneChanged: &playv1.WatchGameSessionResponse_SceneChanged{},
	}})
}

// insertSceneEvent appends a session event inside tx, and returns its id.
func insertSceneEvent(ctx context.Context, c *combatTx, kind string, actor, key *string, payload any) (string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode the event payload: %w", err)
	}
	if ev, ok := payload.(actionEvent); ok && len(body) > eventPayloadBudget && ev.Trap != nil && len(ev.Trap.Caught) > 1 {
		return insertFiringInParts(ctx, c, kind, actor, key, ev) // a firing that catches more creatures than one event holds
	}
	seq, err := c.q.NextSessionEventSeq(ctx, c.session.ID)
	if err != nil {
		return "", fmt.Errorf("next event number: %w", err)
	}
	row, err := c.q.InsertSessionEvent(ctx, playdb.InsertSessionEventParams{
		GameSessionID: c.session.ID, Seq: seq, Kind: kind, ActorUserID: actor, CharacterID: c.characterID,
		Payload: body, IdempotencyKey: key, CreatedAt: c.now, IdempotencyHash: hashOf(c, key),
	})
	if err != nil {
		return "", fmt.Errorf("insert session event: %w", err)
	}
	return row.ID, nil
}

// OpenScene implements playv1connect.PlayServiceHandler.
func (s *Service) OpenScene(
	ctx context.Context,
	req *connect.Request[playv1.OpenSceneRequest],
) (*connect.Response[playv1.OpenSceneResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	pointID, err := uuid.Parse(req.Msg.GetPointId())
	if err != nil {
		return nil, errScenePointNotFound()
	}
	// The point must be a SCENE point of the campaign's maps, hidden or not.
	scene, err := s.maps.ScenePoint(ctx, nil, m.CampaignID, pointID.String())
	if err != nil {
		return nil, s.dbError(ctx, "find the scene point", err)
	}
	// Any SCENE point opens, even one with no actions (question 63): the
	// description, the clues and the stage are enough to run a scene.

	var session playdb.GameSession
	var changed bool // another scene (or none) is open now
	var stageCleared bool
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		var err error
		changed, stageCleared = false, false
		if session, err = q.GetOpenGameSessionForUpdate(ctx, m.CampaignID); errors.Is(err, pgx.ErrNoRows) {
			return errNoOpenSession()
		} else if err != nil {
			return fmt.Errorf("lock the open session: %w", err)
		}
		if equal(session.OpenScenePointID, new(pointID.String())) {
			return nil // already open: the rolls so far stay
		}
		point := pointID.String()
		if session, err = q.SetOpenScene(ctx, playdb.SetOpenSceneParams{ID: session.ID, OpenScenePointID: &point}); err != nil {
			return fmt.Errorf("open the scene: %w", err)
		}
		c, err := s.openTx(ctx, combatTx{tx: tx, q: q, session: session, now: s.now()})
		if err != nil {
			return err
		}
		// Opening a scene, even a hidden point's, makes it "discovered": the
		// players see its name in the open scene, so they may tag notes with
		// it (MR-030, question 61).
		if err := s.maps.DiscoverScene(ctx, tx, m.CampaignID, point, c.now); err != nil {
			return err
		}
		if _, err := insertSceneEvent(ctx, c, eventSceneOpened, &m.UserID, nil, sceneEvent{PointID: point, Actions: len(scene.Actions)}); err != nil {
			return err
		}
		// Another scene is another stage (MR-031).
		if stageCleared, err = clearStage(ctx, c, m.UserID); err != nil {
			return err
		}
		changed = true
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "open a scene", err)
	}
	if changed {
		logging.Event(ctx, s.logger, "scene.opened", slog.String("session_id", session.ID), slog.String("point_id", pointID.String()))
		s.publishSceneChanged(m.CampaignID)
	}
	if stageCleared {
		s.publishStageChanged(m.CampaignID)
	}
	info, err := s.sceneInfo(ctx, m, session, scene)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.OpenSceneResponse{Scene: info}), nil
}

// CloseScene implements playv1connect.PlayServiceHandler.
func (s *Service) CloseScene(
	ctx context.Context,
	req *connect.Request[playv1.CloseSceneRequest],
) (*connect.Response[playv1.CloseSceneResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	var changed, stageCleared bool
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		changed, stageCleared = false, false
		session, err := q.GetOpenGameSessionForUpdate(ctx, m.CampaignID)
		if errors.Is(err, pgx.ErrNoRows) {
			return errNoOpenSession()
		}
		if err != nil {
			return fmt.Errorf("lock the open session: %w", err)
		}
		if session.OpenScenePointID == nil {
			return nil // none open: nothing to close
		}
		if _, err := q.SetOpenScene(ctx, playdb.SetOpenSceneParams{ID: session.ID}); err != nil {
			return fmt.Errorf("close the scene: %w", err)
		}
		c, err := s.openTx(ctx, combatTx{tx: tx, q: q, session: session, now: s.now()})
		if err != nil {
			return err
		}
		if _, err := insertSceneEvent(ctx, c, eventSceneClosed, &m.UserID, nil, sceneEvent{PointID: *session.OpenScenePointID}); err != nil {
			return err
		}
		// No scene, no stage (MR-031).
		if stageCleared, err = clearStage(ctx, c, m.UserID); err != nil {
			return err
		}
		changed = true
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "close a scene", err)
	}
	if changed {
		logging.Event(ctx, s.logger, "scene.closed")
		s.publishSceneChanged(m.CampaignID)
	}
	if stageCleared {
		s.publishStageChanged(m.CampaignID)
	}
	return connect.NewResponse(&playv1.CloseSceneResponse{}), nil
}

// GetOpenScene implements playv1connect.PlayServiceHandler.
func (s *Service) GetOpenScene(
	ctx context.Context,
	req *connect.Request[playv1.GetOpenSceneRequest],
) (*connect.Response[playv1.GetOpenSceneResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	info, err := s.openSceneInfo(ctx, m)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.GetOpenSceneResponse{Scene: info}), nil
}

// openSceneInfo reads the open scene as the caller sees it: nil when none is
// open.
func (s *Service) openSceneInfo(ctx context.Context, m authz.Membership) (*playv1.OpenSceneInfo, error) {
	session, err := s.openSession(ctx, m.CampaignID)
	if err != nil {
		return nil, err
	}
	if session.OpenScenePointID == nil {
		return nil, nil
	}
	scene, err := s.maps.ScenePoint(ctx, nil, m.CampaignID, *session.OpenScenePointID)
	if connect.CodeOf(err) == connect.CodeNotFound {
		// The point stopped being a scene since it was opened (the master
		// changed its kind): no scene is open.
		return nil, nil
	}
	if err != nil {
		return nil, s.dbError(ctx, "read the open scene", err)
	}
	return s.sceneInfo(ctx, m, session, scene)
}

// myCharacter is the caller's own living character, if they have one: the
// one that rolls for them.
func (s *Service) myCharacter(ctx context.Context, tx pgx.Tx, m authz.Membership) (link.Character, bool, error) {
	if m.Role == authz.RoleMaster {
		return link.Character{}, false, nil
	}
	party, err := s.roster.CombatParty(ctx, tx, m.CampaignID)
	if err != nil {
		return link.Character{}, false, err
	}
	i := slices.IndexFunc(party, func(c link.Character) bool { return c.PlayerUserID != "" && c.PlayerUserID == m.UserID })
	if i < 0 {
		return link.Character{}, false, nil
	}
	return party[i], true, nil
}

// sceneInfo builds the open scene as the caller sees it (RN-20, question 52):
// the master gets the DCs and every roll with its pass or fail; a player gets
// their own character's bonus and attempts left on each action and only their
// own rolls, and the DCs and the pass or fail of those rolls only when the
// scene shows its DC.
func (s *Service) sceneInfo(ctx context.Context, m authz.Membership, session playdb.GameSession, scene link.Scene) (*playv1.OpenSceneInfo, error) {
	master := m.Role == authz.RoleMaster
	sees := master || scene.ShowDC // who reads the DCs (the pass or fail follows each roll's own flag)
	info := &playv1.OpenSceneInfo{PointId: scene.PointID, Name: scene.Name, Description: scene.Description, ShowDc: scene.ShowDC}

	mine, hasMine, err := s.myCharacter(ctx, nil, m)
	if err != nil {
		return nil, s.dbError(ctx, "find the caller's character", err)
	}
	var options []link.SceneOption
	if hasMine {
		keys := make([]string, 0, len(scene.Actions))
		for _, a := range scene.Actions {
			keys = append(keys, a.Key)
		}
		if options, err = s.roster.SceneOptions(ctx, nil, m.CampaignID, mine.ID, keys); err != nil {
			return nil, s.dbError(ctx, "work out the scene's bonuses", err)
		}
	}
	// The opening this read belongs to: its rolls and its grants.
	opened, err := s.queries.GetOpenSceneEvent(ctx, session.ID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		opened.Seq = 0 // opened before events were kept: every roll counts
	case err != nil:
		return nil, s.dbError(ctx, "read when the scene opened", err)
	default:
		info.OpenedAt = timestamppb.New(opened.CreatedAt)
	}
	tally, err := tallyAttempts(ctx, s.queries, session.ID, opened.Seq)
	if err != nil {
		return nil, s.dbError(ctx, "count the scene's attempts", err)
	}

	for i, a := range scene.Actions {
		v := &playv1.SceneActionView{
			Id: a.ID, Key: a.Key, Name: a.Name, CheckName: s.roster.SceneCheckName(a.Key),
			MaxAttempts: clamp32(a.MaxAttempts, 0, 5),
		}
		if sees {
			v.Dc = clamp32(a.DC, 0, 30)
		}
		if !master && i < len(options) && options[i].Known {
			v.Bonus = new(clamp32(options[i].Bonus, math.MinInt32, math.MaxInt32))
			if options[i].HasPassive {
				v.Passive = new(clamp32(options[i].Passive, math.MinInt32, math.MaxInt32))
			}
		}
		if hasMine {
			v.AttemptsLeft = tally.left(a, mine.ID)
		}
		info.Actions = append(info.Actions, v)
	}

	if master {
		// Only the master reads the hooks and the clues (MR-029, RN-20).
		if info.Hooks, info.Clues, err = s.masterSceneNotes(ctx, m.CampaignID, scene); err != nil {
			return nil, err
		}
	}

	rolls, err := s.sceneRolls(ctx, m, scene, session.ID, opened.Seq, tally, mine.ID, hasMine)
	if err != nil {
		return nil, err
	}
	info.Rolls = rolls
	if info.Stage, err = s.stageOf(ctx, m, session.ID); err != nil {
		return nil, err
	}
	return info, nil
}

// masterSceneNotes builds the master's view of a scene's hooks and clues, with
// each clue's recipients named.
func (s *Service) masterSceneNotes(ctx context.Context, campaignID string, scene link.Scene) (string, []*mapsv1.SceneClue, error) {
	var ids []string
	for _, c := range scene.Clues {
		for _, r := range c.RevealedTo {
			if !slices.Contains(ids, r.CharacterID) {
				ids = append(ids, r.CharacterID)
			}
		}
	}
	nameOf := map[string]string{}
	if len(ids) > 0 {
		chars, err := s.roster.CombatCharacters(ctx, nil, campaignID, ids)
		if err != nil {
			return "", nil, s.dbError(ctx, "read the recipients' names", err)
		}
		for _, c := range chars {
			nameOf[c.ID] = c.Name
		}
	}
	clues := make([]*mapsv1.SceneClue, 0, len(scene.Clues))
	for _, c := range scene.Clues {
		clue := &mapsv1.SceneClue{Id: c.ID, Text: c.Text}
		for _, r := range c.RevealedTo {
			clue.RevealedTo = append(clue.RevealedTo, &mapsv1.ClueRecipient{
				CharacterId: r.CharacterID, CharacterName: nameOf[r.CharacterID], RevealedAt: timestamppb.New(r.RevealedAt),
			})
		}
		clues = append(clues, clue)
	}
	return scene.Hooks, clues, nil
}

// sceneRolls reads the rolls of the opening that began at event number
// after, newest first: all of them for the master, the caller's own
// character's for a player. The pass or fail goes to the master, and to a
// player for the rolls made while the scene showed its DC (RN-20); the attempts
// left, only to the master.
func (s *Service) sceneRolls(ctx context.Context, m authz.Membership, scene link.Scene, sessionID string, after int32, tally attemptTally, mineID string, hasMine bool) ([]*playv1.SceneRoll, error) {
	master := m.Role == authz.RoleMaster
	if !master && !hasMine {
		return nil, nil // a player with no living character has no rolls
	}
	rows, err := s.queries.ListSceneRollEvents(ctx, playdb.ListSceneRollEventsParams{GameSessionID: sessionID, Seq: after})
	if err != nil {
		return nil, s.dbError(ctx, "list the scene's rolls", err)
	}
	var ids []string
	for _, r := range rows {
		if r.CharacterID != nil && !slices.Contains(ids, *r.CharacterID) {
			ids = append(ids, *r.CharacterID)
		}
	}
	names := map[string]string{}
	if len(ids) > 0 {
		chars, err := s.roster.CombatCharacters(ctx, nil, m.CampaignID, ids)
		if err != nil {
			return nil, s.dbError(ctx, "read the rollers' names", err)
		}
		for _, c := range chars {
			names[c.ID] = c.Name
		}
	}
	var out []*playv1.SceneRoll
	for _, r := range rows {
		if !master && (r.CharacterID == nil || *r.CharacterID != mineID) {
			continue
		}
		var ev sceneRollEvent
		if err := json.Unmarshal(r.Payload, &ev); err != nil {
			return nil, s.dbError(ctx, "read a scene roll", fmt.Errorf("decode the roll of event %s: %w", r.ID, err))
		}
		// One rule for pass or fail: whether the scene showed its DC when the
		// roll was made (the same flag the session summary counts).
		roll := sceneRollToProto(r.ID, deref(r.CharacterID), names[deref(r.CharacterID)], r.CreatedAt, ev, master || ev.DCShown)
		if master {
			// An action the master removed since has no limit to read.
			if i := slices.IndexFunc(scene.Actions, func(a link.SceneAction) bool { return a.ID == ev.ActionID }); i >= 0 {
				roll.AttemptsLeft = tally.left(scene.Actions[i], deref(r.CharacterID))
			}
		}
		out = append(out, roll)
	}
	return out, nil
}

// sceneRollToProto builds a roll as the caller sees it: showPassed says
// whether the pass or fail goes to the caller (RN-20).
func sceneRollToProto(id, characterID, characterName string, at time.Time, ev sceneRollEvent, showPassed bool) *playv1.SceneRoll {
	out := &playv1.SceneRoll{
		Id: id, ActionId: ev.ActionID, CharacterId: characterID, CharacterName: characterName,
		Roll:     diceRoll(1, 20, []int32{ev.D20}, ev.Modifier, ev.Total, ev.Physical),
		RolledAt: timestamppb.New(at),
	}
	if showPassed {
		out.Passed = ev.Passed
	}
	return out
}

// attemptTally counts, for one opening of a scene, each character's rolls and
// the attempts the master granted, per action.
type attemptTally struct {
	rolled, granted map[attemptKey]int
}

type attemptKey struct{ characterID, actionID string }

// maxUnlimitedSceneRolls is how many times one character may roll one action
// whose limit is "unlimited" (0) in one opening of a scene. Every roll is a
// row of the session's log, which the readers of the scene and the summary
// go through; far more than a table rolls by hand, short of letting one
// member fill the log.
const maxUnlimitedSceneRolls = 200

// tallyAttempts reads the rolls and the grants of the opening that began at
// event number after.
func tallyAttempts(ctx context.Context, q *playdb.Queries, sessionID string, after int32) (attemptTally, error) {
	t := attemptTally{rolled: map[attemptKey]int{}, granted: map[attemptKey]int{}}
	rolls, err := q.CountSceneRolls(ctx, playdb.CountSceneRollsParams{GameSessionID: sessionID, Seq: after})
	if err != nil {
		return t, fmt.Errorf("count the scene's rolls: %w", err)
	}
	for _, r := range rolls {
		t.rolled[attemptKey{deref(r.CharacterID), r.ActionID}] += int(r.Rolls)
	}
	grants, err := q.ListSceneAttemptGrantEvents(ctx, playdb.ListSceneAttemptGrantEventsParams{GameSessionID: sessionID, Seq: after})
	if err != nil {
		return t, fmt.Errorf("list the scene's granted attempts: %w", err)
	}
	for _, g := range grants {
		var ev sceneGrantEvent
		if err := json.Unmarshal(g.Payload, &ev); err != nil {
			return t, fmt.Errorf("decode a granted attempt: %w", err)
		}
		t.granted[attemptKey{deref(g.CharacterID), ev.ActionID}]++
	}
	return t, nil
}

// left is how many attempts the character has at the action: the limit minus
// their rolls plus the grants, never below 0. Unset for an unlimited action.
// Lowering the limit below what they used leaves them with 0 and erases
// nothing.
func (t attemptTally) left(a link.SceneAction, characterID string) *int32 {
	if a.MaxAttempts == 0 {
		return nil
	}
	k := attemptKey{characterID, a.ID}
	return new(clamp32(max(a.MaxAttempts-t.rolled[k]+t.granted[k], 0), 0, math.MaxInt32))
}

// RollSceneCheck implements playv1connect.PlayServiceHandler.
func (s *Service) RollSceneCheck(
	ctx context.Context,
	req *connect.Request[playv1.RollSceneCheckRequest],
) (*connect.Response[playv1.RollSceneCheckResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RolePlayer)
	if err != nil {
		return nil, err
	}
	key, err := parseKey(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	hash := idem.Hash(req.Msg)
	actionID, err := uuid.Parse(req.Msg.GetActionId())
	if err != nil {
		return nil, errSceneActionNotFound()
	}
	var in rollInput
	switch roll := req.Msg.GetRoll().(type) {
	case *playv1.RollSceneCheckRequest_RollInApp:
		if !roll.RollInApp {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("roll_in_app must be true"))
		}
		in.inApp = true
	case *playv1.RollSceneCheckRequest_D20Face:
		in.typed = int(roll.D20Face)
		if in.typed < 1 || in.typed > 20 {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("d20_face must be 1 to 20"))
		}
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("set roll_in_app or d20_face"))
	}

	var (
		ev       sceneRollEvent
		rollID   string
		who      link.Character
		repeated bool
		at       time.Time
		doneRow  playdb.GetSessionEventByIdempotencyKeyRow
	)
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		repeated = false
		session, err := q.GetOpenGameSessionForUpdate(ctx, m.CampaignID)
		if errors.Is(err, pgx.ErrNoRows) {
			return errNoOpenSession()
		}
		if err != nil {
			return fmt.Errorf("lock the open session: %w", err)
		}

		// A retry of a roll already made returns that roll, and changes nothing.
		doneRow, err = q.GetSessionEventByIdempotencyKey(ctx, playdb.GetSessionEventByIdempotencyKeyParams{
			GameSessionID: session.ID, IdempotencyKey: &key,
		})
		switch {
		case err == nil:
			if doneRow.Kind != eventSceneCheckRolled {
				return connect.NewError(connect.CodeInvalidArgument, errors.New("idempotency_key was already used for another change"))
			}
			if doneRow.ActorUserID == nil || *doneRow.ActorUserID != m.UserID || hashDiffers(doneRow.IdempotencyHash, hash) {
				return connect.NewError(connect.CodeInvalidArgument, errors.New("idempotency_key was already used for another change"))
			}
			repeated = true
			rollID, at = doneRow.ID, doneRow.CreatedAt
			if err := json.Unmarshal(doneRow.Payload, &ev); err != nil {
				return fmt.Errorf("decode the roll of event %s: %w", doneRow.ID, err)
			}
			return nil
		case !errors.Is(err, pgx.ErrNoRows):
			return fmt.Errorf("find the event of this idempotency key: %w", err)
		}

		if session.OpenScenePointID == nil {
			return errScene(playv1.SceneBlockedReason_SCENE_BLOCKED_REASON_NO_OPEN_SCENE, "no scene is open")
		}
		// The scene is read inside this transaction, like everything else it
		// reads: a second connection from the pool would deadlock a few rolls at
		// once. The player's own attempts are safe, the session row is locked.
		scene, err := s.maps.ScenePoint(ctx, tx, m.CampaignID, *session.OpenScenePointID)
		if connect.CodeOf(err) == connect.CodeNotFound {
			return errScene(playv1.SceneBlockedReason_SCENE_BLOCKED_REASON_NO_OPEN_SCENE, "no scene is open") // no longer a scene point
		}
		if err != nil {
			return err
		}
		i := slices.IndexFunc(scene.Actions, func(a link.SceneAction) bool { return a.ID == actionID.String() })
		if i < 0 {
			return errSceneActionNotFound()
		}
		action := scene.Actions[i]

		// The player's own living character rolls (RN-18: how, as the campaign
		// allows).
		var has bool
		if who, has, err = s.myCharacter(ctx, tx, m); err != nil {
			return err
		}
		if !has {
			return errScene(playv1.SceneBlockedReason_SCENE_BLOCKED_REASON_NO_CHARACTER, "you have no living character to roll for")
		}
		force, err := s.dice.ForcedDice(ctx, tx, m.CampaignID, m.UserID)
		if err != nil {
			return err
		}
		if force.refuses(in.inApp) {
			return errScene(playv1.SceneBlockedReason_SCENE_BLOCKED_REASON_WRONG_DICE_MODE, "this is not how the campaign has you roll your dice")
		}
		options, err := s.roster.SceneOptions(ctx, tx, m.CampaignID, who.ID, []string{action.Key})
		if err != nil {
			return err
		}
		if len(options) != 1 || !options[0].Known {
			return errScene(playv1.SceneBlockedReason_SCENE_BLOCKED_REASON_NO_CHARACTER, "your character has no numbers for this check")
		}

		// As many attempts per character per action as the master allowed while
		// the scene is open: the rolls and the grants since its opening
		// (question 55).
		opened, err := q.GetOpenSceneEvent(ctx, session.ID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("read when the scene opened: %w", err)
		}
		tally, err := tallyAttempts(ctx, q, session.ID, opened.Seq)
		if err != nil {
			return err
		}
		// An unlimited action still has a cap, so the rows a member can
		// append stay bounded; a new opening of the scene starts the count over.
		if action.MaxAttempts == 0 && tally.rolled[attemptKey{who.ID, action.ID}] >= maxUnlimitedSceneRolls {
			return errScene(playv1.SceneBlockedReason_SCENE_BLOCKED_REASON_ALREADY_ROLLED, "this action has been rolled too many times; the master can close and open the scene again")
		}
		if left := tally.left(action, who.ID); left != nil && *left == 0 {
			return errScene(playv1.SceneBlockedReason_SCENE_BLOCKED_REASON_ALREADY_ROLLED, "no attempt left at this action; the master may grant one more")
		}

		bonus := options[0].Bonus
		face, roll, err := s.d20(in, bonus)
		if err != nil {
			return err
		}
		ev = sceneRollEvent{
			PointID: scene.PointID, ActionID: action.ID, Key: action.Key,
			D20: clamp32(face, 1, 20), Modifier: clamp32(bonus, math.MinInt32, math.MaxInt32), Total: clamp32(roll.Total, math.MinInt32, math.MaxInt32),
			Physical: roll.Physical, DCShown: scene.ShowDC,
		}
		if action.DC > 0 {
			ev.Passed = new(roll.Total >= action.DC)
		}
		c, err := s.openTx(ctx, combatTx{tx: tx, q: q, session: session, now: s.now(), characterID: &who.ID, hash: hash})
		if err != nil {
			return err
		}
		at = c.now
		rollID, err = insertSceneEvent(ctx, c, eventSceneCheckRolled, &m.UserID, &key, ev)
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, "roll a scene check", err)
	}

	characterID, name := who.ID, who.Name
	if repeated {
		// Written the first time: the character and the time are the event's.
		characterID, name = deref(doneRow.CharacterID), ""
		if chars, err := s.roster.CombatCharacters(ctx, nil, m.CampaignID, []string{characterID}); err == nil && len(chars) == 1 {
			name = chars[0].Name
		}
	} else {
		logging.Event(ctx, s.logger, "scene.check_rolled",
			slog.String("point_id", ev.PointID), slog.String("action_id", ev.ActionID),
			slog.String("character_id", who.ID), slog.Bool("physical_dice", ev.Physical))
		// Only the master and the roller hear of it (RN-20).
		s.hub.Publish(m.CampaignID, live.Event{
			Audience: live.Audience{Master: true, UserID: m.UserID},
			Message: &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_SceneCheckRolled_{
				SceneCheckRolled: &playv1.WatchGameSessionResponse_SceneCheckRolled{ActionId: ev.ActionID},
			}},
		})
	}
	// The caller is a player: the pass or fail is in the answer only when the
	// scene showed its DC (RN-20).
	return connect.NewResponse(&playv1.RollSceneCheckResponse{
		Roll: sceneRollToProto(rollID, characterID, name, at, ev, ev.DCShown),
	}), nil
}

// GrantSceneAttempt implements playv1connect.PlayServiceHandler.
func (s *Service) GrantSceneAttempt(
	ctx context.Context,
	req *connect.Request[playv1.GrantSceneAttemptRequest],
) (*connect.Response[playv1.GrantSceneAttemptResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	key, err := parseKey(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	actionID, err := uuid.Parse(req.Msg.GetActionId())
	if err != nil {
		return nil, errSceneActionNotFound()
	}
	characterID, err := uuid.Parse(req.Msg.GetCharacterId())
	if err != nil {
		return nil, errSceneCharacterNotFound()
	}

	var granted bool
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		granted = false
		session, err := q.GetOpenGameSessionForUpdate(ctx, m.CampaignID)
		if errors.Is(err, pgx.ErrNoRows) {
			return errNoOpenSession()
		}
		if err != nil {
			return fmt.Errorf("lock the open session: %w", err)
		}

		// A retry of a grant already made changes nothing.
		done, err := q.GetSessionEventByIdempotencyKey(ctx, playdb.GetSessionEventByIdempotencyKeyParams{GameSessionID: session.ID, IdempotencyKey: &key})
		switch {
		case err == nil:
			var prev sceneGrantEvent
			if done.Kind != eventSceneAttemptGranted || json.Unmarshal(done.Payload, &prev) != nil ||
				prev.ActionID != actionID.String() || deref(done.CharacterID) != characterID.String() {
				return connect.NewError(connect.CodeInvalidArgument, errors.New("idempotency_key was already used for another change"))
			}
			return nil
		case !errors.Is(err, pgx.ErrNoRows):
			return fmt.Errorf("find the event of this idempotency key: %w", err)
		}

		if session.OpenScenePointID == nil {
			return errScene(playv1.SceneBlockedReason_SCENE_BLOCKED_REASON_NO_OPEN_SCENE, "no scene is open")
		}
		scene, err := s.maps.ScenePoint(ctx, tx, m.CampaignID, *session.OpenScenePointID)
		if connect.CodeOf(err) == connect.CodeNotFound {
			return errScene(playv1.SceneBlockedReason_SCENE_BLOCKED_REASON_NO_OPEN_SCENE, "no scene is open") // no longer a scene point
		}
		if err != nil {
			return err
		}
		i := slices.IndexFunc(scene.Actions, func(a link.SceneAction) bool { return a.ID == actionID.String() })
		if i < 0 {
			return errSceneActionNotFound()
		}
		party, err := s.roster.CombatParty(ctx, tx, m.CampaignID)
		if err != nil {
			return err
		}
		if !slices.ContainsFunc(party, func(c link.Character) bool { return c.ID == characterID.String() }) {
			return errSceneCharacterNotFound()
		}
		if scene.Actions[i].MaxAttempts == 0 {
			return nil // unlimited: there is nothing to add
		}
		c, err := s.openTx(ctx, combatTx{tx: tx, q: q, session: session, now: s.now(), characterID: new(characterID.String())})
		if err != nil {
			return err
		}
		granted = true
		_, err = insertSceneEvent(ctx, c, eventSceneAttemptGranted, &m.UserID, &key, sceneGrantEvent{PointID: scene.PointID, ActionID: scene.Actions[i].ID})
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, "grant a scene attempt", err)
	}
	if granted {
		// The hint names nothing: the player reads the scene again.
		s.publishSceneChanged(m.CampaignID)
	}
	info, err := s.openSceneInfo(ctx, m)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.GrantSceneAttemptResponse{Scene: info}), nil
}

func errSceneCharacterNotFound() error {
	return connect.NewError(connect.CodeNotFound, errors.New("character not found"))
}
