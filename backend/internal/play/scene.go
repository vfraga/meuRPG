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
	// The roll had advantage or disadvantage: the other d20, which of the two was
	// first (D20 is the one that counts) and the mode.
	D20B     int32  `json:"d20_b,omitempty"`
	Counted  int32  `json:"counted,omitempty"`
	RollMode string `json:"roll_mode,omitempty"`
	// The Bardic Inspiration die the player added after the d20 (already in Total).
	BonusSides int32 `json:"bonus_sides,omitempty"`
	BonusFace  int32 `json:"bonus_face,omitempty"`
	// Lucky is the d20 the halfling's Lucky rolled again after it showed a 1 (inspiration_lucky.go):
	// D20, D20B and Counted are the dice after it, and this keeps the 1 it replaced and where, so
	// the roll still says "Sortudo: 1 → 14". Nil when nothing was rolled again.
	Lucky *luckyReroll `json:"lucky,omitempty"`
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

// errNoOpenScene is the refusal of a scene call when no scene is open (or the open
// point is no longer a scene).
func errNoOpenScene() error {
	return errScene(playv1.SceneBlockedReason_SCENE_BLOCKED_REASON_NO_OPEN_SCENE, "no scene is open")
}

// lockSceneSession locks the campaign's open game session inside the transaction of q.
func lockSceneSession(ctx context.Context, q *playdb.Queries, campaignID string) (playdb.GameSession, error) {
	session, err := q.GetOpenGameSessionForUpdate(ctx, campaignID)
	if errors.Is(err, pgx.ErrNoRows) {
		return playdb.GameSession{}, errNoOpenSession()
	}
	if err != nil {
		return playdb.GameSession{}, fmt.Errorf("lock the open session: %w", err)
	}
	return session, nil
}

// openSceneIn reads the scene the session has open, inside tx like everything else
// the caller reads: a second connection from the pool would deadlock a few rolls at once.
func (s *Service) openSceneIn(ctx context.Context, tx pgx.Tx, campaignID string, session playdb.GameSession) (link.Scene, error) {
	if session.OpenScenePointID == nil {
		return link.Scene{}, errNoOpenScene()
	}
	scene, err := s.maps.ScenePoint(ctx, tx, campaignID, *session.OpenScenePointID)
	if connect.CodeOf(err) == connect.CodeNotFound {
		return link.Scene{}, errNoOpenScene() // no longer a scene point
	}
	return scene, err
}

// sceneActionOf is the scene's action with the ID, or the not-found refusal.
func sceneActionOf(scene link.Scene, actionID string) (link.SceneAction, error) {
	i := slices.IndexFunc(scene.Actions, func(a link.SceneAction) bool { return a.ID == actionID })
	if i < 0 {
		return link.SceneAction{}, errSceneActionNotFound()
	}
	return scene.Actions[i], nil
}

// sceneSwitch is what the transaction of OpenScene or CloseScene hands over.
type sceneSwitch struct {
	session      playdb.GameSession
	changed      bool // another scene (or none) is open now
	stageCleared bool
}

// publishSceneSwitch tells the streams what a switch changed.
func (s *Service) publishSceneSwitch(campaignID string, sw sceneSwitch) {
	if sw.changed {
		s.publishSceneChanged(campaignID)
	}
	if sw.stageCleared {
		s.publishStageChanged(campaignID)
	}
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

	var sw sceneSwitch
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		sw = sceneSwitch{}
		return s.switchSceneOn(ctx, tx, m, pointID.String(), len(scene.Actions), &sw)
	})
	if err != nil {
		return nil, s.dbError(ctx, "open a scene", err)
	}
	if sw.changed {
		logging.Event(ctx, s.logger, "scene.opened", slog.String("session_id", sw.session.ID), slog.String("point_id", pointID.String()))
	}
	s.publishSceneSwitch(m.CampaignID, sw)
	info, err := s.sceneInfo(ctx, m, sw.session, scene)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.OpenSceneResponse{Scene: info}), nil
}

// switchSceneOn opens the point in the locked session, unless it is the open one already
// (the rolls so far stay), and empties the stage: another scene is another stage (MR-031).
func (s *Service) switchSceneOn(ctx context.Context, tx pgx.Tx, m authz.Membership, point string, actions int, sw *sceneSwitch) error {
	q := s.queries.WithTx(tx)
	session, err := lockSceneSession(ctx, q, m.CampaignID)
	sw.session = session
	if err != nil {
		return err
	}
	if equal(session.OpenScenePointID, &point) {
		return nil // already open
	}
	if sw.session, err = q.SetOpenScene(ctx, playdb.SetOpenSceneParams{ID: session.ID, OpenScenePointID: &point}); err != nil {
		return fmt.Errorf("open the scene: %w", err)
	}
	c, err := s.openTx(ctx, combatTx{tx: tx, q: q, session: sw.session, now: s.now()})
	if err != nil {
		return err
	}
	// Opening a scene, even a hidden point's, makes it "discovered": the
	// players see its name in the open scene, so they may tag notes with
	// it (MR-030, question 61).
	if err := s.maps.DiscoverScene(ctx, tx, m.CampaignID, point, c.now); err != nil {
		return err
	}
	if _, err := insertSceneEvent(ctx, c, eventSceneOpened, &m.UserID, nil, sceneEvent{PointID: point, Actions: actions}); err != nil {
		return err
	}
	if sw.stageCleared, err = clearStage(ctx, c, m.UserID); err != nil {
		return err
	}
	sw.changed = true
	return nil
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
	var sw sceneSwitch
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		sw = sceneSwitch{}
		return s.switchSceneOff(ctx, tx, m, &sw)
	})
	if err != nil {
		return nil, s.dbError(ctx, "close a scene", err)
	}
	if sw.changed {
		logging.Event(ctx, s.logger, "scene.closed")
	}
	s.publishSceneSwitch(m.CampaignID, sw)
	return connect.NewResponse(&playv1.CloseSceneResponse{}), nil
}

// switchSceneOff closes the open scene of the locked session, if any, and empties the
// stage: no scene, no stage (MR-031).
func (s *Service) switchSceneOff(ctx context.Context, tx pgx.Tx, m authz.Membership, sw *sceneSwitch) error {
	q := s.queries.WithTx(tx)
	session, err := lockSceneSession(ctx, q, m.CampaignID)
	if err != nil {
		return err
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
	if sw.stageCleared, err = clearStage(ctx, c, m.UserID); err != nil {
		return err
	}
	sw.changed = true
	return nil
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
	v := &sceneViewing{m: m, master: master, sees: master || scene.ShowDC, scene: scene, sessionID: session.ID}
	info := &playv1.OpenSceneInfo{PointId: scene.PointID, Name: scene.Name, Description: scene.Description, ShowDc: scene.ShowDC}

	if err := s.viewingCharacter(ctx, v); err != nil {
		return nil, err
	}
	// The opening this read belongs to: its rolls and its grants.
	if err := s.viewingOpening(ctx, v, info); err != nil {
		return nil, err
	}
	var err error
	if v.tally, err = tallyAttempts(ctx, s.queries, session.ID, v.after); err != nil {
		return nil, s.dbError(ctx, "count the scene's attempts", err)
	}
	info.Actions = s.sceneActionViews(v)
	if master {
		if err := s.addMasterSceneParts(ctx, info, v); err != nil {
			return nil, err
		}
	}

	if info.Rolls, err = s.sceneRolls(ctx, v); err != nil {
		return nil, err
	}
	if info.Stage, err = s.stageOf(ctx, m, session.ID); err != nil {
		return nil, err
	}
	return info, nil
}

// sceneViewing is what the read of the open scene knows about its viewer and its opening.
type sceneViewing struct {
	m         authz.Membership
	master    bool
	sees      bool // who reads the DCs (the pass or fail follows each roll's own flag)
	scene     link.Scene
	sessionID string
	mine      link.Character // the caller's own living character, when hasMine
	hasMine   bool
	options   []link.SceneOption // the character's numbers, one per action
	after     int32              // the event number the opening began at
	tally     attemptTally
}

// viewingCharacter finds the caller's own character and what it adds to each action.
func (s *Service) viewingCharacter(ctx context.Context, v *sceneViewing) error {
	var err error
	v.mine, v.hasMine, err = s.myCharacter(ctx, nil, v.m)
	if err != nil {
		return s.dbError(ctx, "find the caller's character", err)
	}
	if !v.hasMine {
		return nil
	}
	keys := make([]string, 0, len(v.scene.Actions))
	for _, a := range v.scene.Actions {
		keys = append(keys, a.Key)
	}
	if v.options, err = s.roster.SceneOptions(ctx, nil, v.m.CampaignID, v.mine.ID, keys); err != nil {
		return s.dbError(ctx, "work out the scene's bonuses", err)
	}
	return nil
}

// viewingOpening reads when the scene was opened and the event number its rolls count from.
func (s *Service) viewingOpening(ctx context.Context, v *sceneViewing, info *playv1.OpenSceneInfo) error {
	opened, err := s.queries.GetOpenSceneEvent(ctx, v.sessionID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		v.after = 0 // opened before events were kept: every roll counts
	case err != nil:
		return s.dbError(ctx, "read when the scene opened", err)
	default:
		v.after = opened.Seq
		info.OpenedAt = timestamppb.New(opened.CreatedAt)
	}
	return nil
}

// sceneActionViews builds the scene's actions as the viewer sees them.
func (s *Service) sceneActionViews(v *sceneViewing) []*playv1.SceneActionView {
	var out []*playv1.SceneActionView
	for i, a := range v.scene.Actions {
		view := &playv1.SceneActionView{
			Id: a.ID, Key: a.Key, Name: a.Name, CheckName: s.roster.SceneCheckName(a.Key),
			MaxAttempts: clamp32(a.MaxAttempts, 0, maxSceneAttempts),
		}
		if v.sees {
			view.Dc = clamp32(a.DC, 0, maxSceneDC)
		}
		if !v.master && i < len(v.options) && v.options[i].Known {
			view.Bonus = new(clamp32(v.options[i].Bonus, math.MinInt32, math.MaxInt32))
			if v.options[i].HasPassive {
				view.Passive = new(clamp32(v.options[i].Passive, math.MinInt32, math.MaxInt32))
			}
		}
		if v.hasMine {
			view.AttemptsLeft = v.tally.left(a, v.mine.ID)
		}
		out = append(out, view)
	}
	return out
}

// addMasterSceneParts adds what only the master reads: the hooks and the clues (MR-029,
// RN-20) and the scene's images, which the master shows one at a time (MR-015).
func (s *Service) addMasterSceneParts(ctx context.Context, info *playv1.OpenSceneInfo, v *sceneViewing) error {
	var err error
	if info.Hooks, info.Clues, err = s.masterSceneNotes(ctx, v.m.CampaignID, v.scene); err != nil {
		return err
	}
	for _, img := range v.scene.Images {
		info.Images = append(info.Images, &mapsv1.SceneImage{Id: img.ID, Name: img.Name, ShowsWholeMap: img.ShowsWholeMap})
	}
	return nil
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
func (s *Service) sceneRolls(ctx context.Context, v *sceneViewing) ([]*playv1.SceneRoll, error) {
	if !v.master && !v.hasMine {
		return nil, nil // a player with no living character has no rolls
	}
	rows, err := s.queries.ListSceneRollEvents(ctx, playdb.ListSceneRollEventsParams{GameSessionID: v.sessionID, Seq: v.after})
	if err != nil {
		return nil, s.dbError(ctx, "list the scene's rolls", err)
	}
	names, err := s.rollerNames(ctx, v.m.CampaignID, rows)
	if err != nil {
		return nil, err
	}
	var out []*playv1.SceneRoll
	for _, r := range rows {
		if !v.master && (r.CharacterID == nil || *r.CharacterID != v.mine.ID) {
			continue
		}
		roll, err := s.sceneRollView(ctx, v, r, names)
		if err != nil {
			return nil, err
		}
		out = append(out, roll)
	}
	return out, nil
}

// rollerNames reads the names of the characters that made the rolls.
func (s *Service) rollerNames(ctx context.Context, campaignID string, rows []playdb.ListSceneRollEventsRow) (map[string]string, error) {
	var ids []string
	for _, r := range rows {
		if r.CharacterID != nil && !slices.Contains(ids, *r.CharacterID) {
			ids = append(ids, *r.CharacterID)
		}
	}
	names := map[string]string{}
	if len(ids) == 0 {
		return names, nil
	}
	chars, err := s.roster.CombatCharacters(ctx, nil, campaignID, ids)
	if err != nil {
		return nil, s.dbError(ctx, "read the rollers' names", err)
	}
	for _, c := range chars {
		names[c.ID] = c.Name
	}
	return names, nil
}

// sceneRollView builds one roll of the list as the viewer sees it.
func (s *Service) sceneRollView(ctx context.Context, v *sceneViewing, r playdb.ListSceneRollEventsRow, names map[string]string) (*playv1.SceneRoll, error) {
	var ev sceneRollEvent
	if err := json.Unmarshal(r.Payload, &ev); err != nil {
		return nil, s.dbError(ctx, "read a scene roll", fmt.Errorf("decode the roll of event %s: %w", r.ID, err))
	}
	// One rule for pass or fail: whether the scene showed its DC when the
	// roll was made (the same flag the session summary counts).
	roll := sceneRollToProto(r.ID, deref(r.CharacterID), names[deref(r.CharacterID)], r.CreatedAt, ev, v.master || ev.DCShown)
	if v.master {
		// An action the master removed since has no limit to read.
		if i := slices.IndexFunc(v.scene.Actions, func(a link.SceneAction) bool { return a.ID == ev.ActionID }); i >= 0 {
			roll.AttemptsLeft = v.tally.left(v.scene.Actions[i], deref(r.CharacterID))
		}
	}
	return roll, nil
}

// sceneRollToProto builds a roll as the caller sees it: showPassed says
// whether the pass or fail goes to the caller (RN-20).
func sceneRollToProto(id, characterID, characterName string, at time.Time, ev sceneRollEvent, showPassed bool) *playv1.SceneRoll {
	out := &playv1.SceneRoll{
		Id: id, ActionId: ev.ActionID, CharacterId: characterID, CharacterName: characterName,
		Roll:     treatedRoll(ev.D20, ev.Modifier, ev.Total-ev.BonusFace, ev.Physical),
		RolledAt: timestamppb.New(at),
		Mode:     modeToProto[modeOfKey(ev.RollMode)],
	}
	if ev.D20B != 0 {
		out.Roll = diceRoll(2, 20, pairOf(ev.D20, ev.D20B, ev.Counted), ev.Modifier, ev.Total, ev.Physical)
		out.Roll.CountedIndex = ev.Counted
		markTreated(out.Roll, ev.Modifier, ev.Total-ev.BonusFace)
	}
	out.Roll.Total = ev.Total            // with the Bardic Inspiration die, when the player used it
	out.Roll.Rerolled = ev.Lucky.proto() // "Sortudo: 1 → 14", with the d20 it changed
	if showPassed {
		out.Passed = ev.Passed
	}
	if ev.BonusSides != 0 {
		out.BonusDice = []*playv1.BonusDie{{SourceKey: bardicInspirationFt, Sides: ev.BonusSides, Face: ev.BonusFace, Used: true}}
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

// maxSceneAttempts and maxSceneDC are the limits of an action's attempts and DC that the
// app is told of.
const (
	maxSceneAttempts = 5
	maxSceneDC       = 30
)

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

// sceneRolled is what the transaction of a roll hands over to the answer.
type sceneRolled struct {
	shown    []*playv1.AdvantageSource // the circumstances of the roll's mode, for the roller
	ev       sceneRollEvent
	rollID   string
	who      link.Character
	repeated bool
	at       time.Time
	doneRow  playdb.GetSessionEventByIdempotencyKeyRow
	offer    *playv1.OutsideInspirationOffer // the roll waits for the answer about a Bardic Inspiration die
	lucky    *playv1.LuckyOffer              // the roll waits for the halfling's Lucky answer first
}

// sceneRollCall is a roll of a scene check being made: the request, and the transaction
// it runs in.
type sceneRollCall struct {
	s        *Service
	m        authz.Membership
	req      *playv1.RollSceneCheckRequest
	key      string
	hash     *string
	actionID string
	in       rollInput
	tx       pgx.Tx
	q        *playdb.Queries
	out      *sceneRolled
}

// sceneRollInput reads how the player rolls: in the app, or the faces they typed.
func sceneRollInput(req *playv1.RollSceneCheckRequest) (rollInput, error) {
	var in rollInput
	in.extraFaces = req.GetExtraDieFaces()
	var err error
	switch roll := req.GetRoll().(type) {
	case *playv1.RollSceneCheckRequest_RollInApp:
		if !roll.RollInApp {
			return in, connect.NewError(connect.CodeInvalidArgument, errors.New("roll_in_app must be true"))
		}
		in.inApp = true
	case *playv1.RollSceneCheckRequest_D20Face:
		in.typed = int(roll.D20Face)
		if in.typed < 1 || in.typed > d20Sides {
			return in, connect.NewError(connect.CodeInvalidArgument, errors.New("d20_face must be 1 to 20"))
		}
	default:
		in.typedFaces, err = typedD20Faces(req.GetD20Faces())
	}
	return in, err
}

// newSceneRollCall reads the request of a roll: the key, the action and the dice.
func (s *Service) newSceneRollCall(m authz.Membership, req *playv1.RollSceneCheckRequest) (*sceneRollCall, error) {
	key, err := parseKey(req.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	hash := idem.Hash(req)
	actionID, err := uuid.Parse(req.GetActionId())
	if err != nil {
		return nil, errSceneActionNotFound()
	}
	in, err := sceneRollInput(req)
	if err != nil {
		return nil, err
	}
	return &sceneRollCall{s: s, m: m, req: req, key: key, hash: hash, actionID: actionID.String(), in: in}, nil
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
	call, err := s.newSceneRollCall(m, req.Msg)
	if err != nil {
		return nil, err
	}
	var out sceneRolled
	call.out = &out
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		out.repeated, out.offer, out.lucky = false, nil, nil
		call.tx, call.q = tx, s.queries.WithTx(tx)
		return call.run(ctx)
	})
	if err != nil {
		return nil, s.dbError(ctx, "roll a scene check", err)
	}

	if out.offer != nil || out.lucky != nil {
		return connect.NewResponse(&playv1.RollSceneCheckResponse{InspirationOffer: out.offer, LuckyOffer: out.lucky}), nil
	}
	return connect.NewResponse(&playv1.RollSceneCheckResponse{Roll: s.sceneRollAnswer(ctx, m, &out)}), nil
}

// sceneRollAnswer builds the roll the caller reads, and tells the table when it is a new one.
func (s *Service) sceneRollAnswer(ctx context.Context, m authz.Membership, out *sceneRolled) *playv1.SceneRoll {
	characterID, name := out.who.ID, out.who.Name
	if out.repeated {
		// Written the first time: the character and the time are the event's.
		characterID, name = deref(out.doneRow.CharacterID), ""
		if chars, err := s.roster.CombatCharacters(ctx, nil, m.CampaignID, []string{characterID}); err == nil && len(chars) == 1 {
			name = chars[0].Name
		}
	} else {
		logging.Event(ctx, s.logger, "scene.check_rolled",
			slog.String("point_id", out.ev.PointID), slog.String("action_id", out.ev.ActionID),
			slog.String("character_id", out.who.ID), slog.Bool("physical_dice", out.ev.Physical))
		// Only the master and the roller hear of it (RN-20).
		s.hub.Publish(m.CampaignID, live.Event{
			Audience: live.Audience{Master: true, UserID: m.UserID},
			Message: &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_SceneCheckRolled_{
				SceneCheckRolled: &playv1.WatchGameSessionResponse_SceneCheckRolled{ActionId: out.ev.ActionID},
			}},
		})
	}
	// The caller is a player: the pass or fail is in the answer only when the
	// scene showed its DC (RN-20).
	roll := sceneRollToProto(out.rollID, characterID, name, out.at, out.ev, out.ev.DCShown)
	roll.Sources = out.shown
	return roll
}

// run is the transaction of a roll: a retry gets its roll back; otherwise the rules and the
// attempts allow it, the dice are rolled (or held for a question) and the roll is written.
func (c *sceneRollCall) run(ctx context.Context) error {
	session, err := lockSceneSession(ctx, c.q, c.m.CampaignID)
	if err != nil {
		return err
	}
	// A retry of a roll already made returns that roll, and changes nothing.
	if replayed, err := c.replay(ctx, session.ID); replayed || err != nil {
		return err
	}
	scene, err := c.s.openSceneIn(ctx, c.tx, c.m.CampaignID, session)
	if err != nil {
		return err
	}
	action, err := sceneActionOf(scene, c.actionID)
	if err != nil {
		return err
	}
	opt, err := c.rollerOf(ctx, action)
	if err != nil {
		return err
	}
	if err := c.checkAttempts(ctx, session.ID, action); err != nil {
		return err
	}
	cm, bonus, err := c.modeAndBonus(ctx, session.ID, action, opt.Bonus)
	if err != nil {
		return err
	}
	step, err := c.s.heldGate(ctx, c.s.outsideGate(c.tx, c.q), rollSpec{
		Kind: heldSceneCheck, Key: c.key, Request: c.req, Roller: heldRoller{CampaignID: c.m.CampaignID, CharacterID: c.out.who.ID, UserID: c.m.UserID},
		In: c.in, Modifier: bonus, Mode: cm.Mode, Reliable: opt.ReliableTalent, Lucky: opt.Lucky, MayAsk: true,
	})
	if err != nil {
		return err
	}
	if step.Held != nil {
		c.out.offer, c.out.lucky = c.s.outsideOffers(ctx, c.tx, step.Held)
		return nil
	}
	return c.write(ctx, session, scene, action, cm, bonus, step)
}

// replay answers a retry of the key with the roll it made. It says whether it did.
func (c *sceneRollCall) replay(ctx context.Context, sessionID string) (bool, error) {
	done, err := c.q.GetSessionEventByIdempotencyKey(ctx, playdb.GetSessionEventByIdempotencyKeyParams{
		GameSessionID: sessionID, IdempotencyKey: &c.key,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("find the event of this idempotency key: %w", err)
	}
	c.out.doneRow = done
	if done.Kind != eventSceneCheckRolled {
		return false, errKeyReused()
	}
	if done.ActorUserID == nil || *done.ActorUserID != c.m.UserID || hashDiffers(done.IdempotencyHash, c.hash) {
		return false, errKeyReused()
	}
	c.out.repeated = true
	c.out.rollID, c.out.at = done.ID, done.CreatedAt
	if err := json.Unmarshal(done.Payload, &c.out.ev); err != nil {
		return false, fmt.Errorf("decode the roll of event %s: %w", done.ID, err)
	}
	return true, nil
}

// rollerOf finds the player's own living character (RN-18: how it rolls, as the campaign
// allows) and its numbers for the action's check.
func (c *sceneRollCall) rollerOf(ctx context.Context, action link.SceneAction) (link.SceneOption, error) {
	who, has, err := c.s.myCharacter(ctx, c.tx, c.m)
	c.out.who = who
	if err != nil {
		return link.SceneOption{}, err
	}
	if !has {
		return link.SceneOption{}, errScene(playv1.SceneBlockedReason_SCENE_BLOCKED_REASON_NO_CHARACTER, "you have no living character to roll for")
	}
	force, err := c.s.dice.ForcedDice(ctx, c.tx, c.m.CampaignID, c.m.UserID)
	if err != nil {
		return link.SceneOption{}, err
	}
	if force.refuses(c.in.inApp) {
		return link.SceneOption{}, errScene(playv1.SceneBlockedReason_SCENE_BLOCKED_REASON_WRONG_DICE_MODE, "this is not how the campaign has you roll your dice")
	}
	options, err := c.s.roster.SceneOptions(ctx, c.tx, c.m.CampaignID, who.ID, []string{action.Key})
	if err != nil {
		return link.SceneOption{}, err
	}
	if len(options) != 1 || !options[0].Known {
		return link.SceneOption{}, errScene(playv1.SceneBlockedReason_SCENE_BLOCKED_REASON_NO_CHARACTER, "your character has no numbers for this check")
	}
	return options[0], nil
}

// checkAttempts allows as many attempts per character per action as the master allowed while
// the scene is open: the rolls and the grants since its opening (question 55).
func (c *sceneRollCall) checkAttempts(ctx context.Context, sessionID string, action link.SceneAction) error {
	opened, err := c.q.GetOpenSceneEvent(ctx, sessionID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("read when the scene opened: %w", err)
	}
	tally, err := tallyAttempts(ctx, c.q, sessionID, opened.Seq)
	if err != nil {
		return err
	}
	// An unlimited action still has a cap, so the rows a member can
	// append stay bounded; a new opening of the scene starts the count over.
	if action.MaxAttempts == 0 && tally.rolled[attemptKey{c.out.who.ID, action.ID}] >= maxUnlimitedSceneRolls {
		return errScene(playv1.SceneBlockedReason_SCENE_BLOCKED_REASON_ALREADY_ROLLED, "this action has been rolled too many times; the master can close and open the scene again")
	}
	if left := tally.left(action, c.out.who.ID); left != nil && *left == 0 {
		return errScene(playv1.SceneBlockedReason_SCENE_BLOCKED_REASON_ALREADY_ROLLED, "no attempt left at this action; the master may grant one more")
	}
	return nil
}

// modeAndBonus works out the roll's mode and what it adds. Conditions and states of the
// character in a running combat: Poisoned and Frightened are disadvantage on ability
// checks, a rage advantage on Strength. Bênção and Perdição add their die to a saving
// throw (SRD 5.1).
func (c *sceneRollCall) modeAndBonus(ctx context.Context, sessionID string, action link.SceneAction, bonus int) (checkMode, int, error) {
	ability, isCheck := checkKey(action.Key)
	cm, err := c.s.checkModeOf(ctx, c.tx, c.m.CampaignID, sessionID, c.out.who.ID, action.Key, ability, isCheck)
	if err != nil {
		return cm, 0, err
	}
	names, err := c.s.namerFor(ctx, c.tx, c.m.CampaignID)
	if err != nil {
		return cm, 0, err
	}
	if bonus, err = c.s.withEffectDice(c.in, c.in.extraFaces, &cm, bonus); err != nil {
		return cm, 0, err
	}
	c.out.shown = cm.shownCheck(names)
	return cm, bonus, nil
}

// write builds the roll's event from the settled d20 and writes it, spending the one-time
// effects and the Helps the roll used.
func (c *sceneRollCall) write(ctx context.Context, session playdb.GameSession, scene link.Scene, action link.SceneAction, cm checkMode, bonus int, step gateResult) error {
	d20 := step.Roll
	ev := sceneRollEvent{
		PointID: scene.PointID, ActionID: action.ID, Key: action.Key,
		D20: clamp32(d20.Face(), 1, d20Sides), Modifier: clamp32(bonus, math.MinInt32, math.MaxInt32), Total: clamp32(d20.Total, math.MinInt32, math.MaxInt32),
		Physical: d20.Physical, DCShown: scene.ShowDC, D20B: clamp32(d20.otherFace(), 0, d20Sides), Counted: clamp32(d20.Index, 0, 1), RollMode: modeKey(cm.Mode),
		Lucky: step.Lucky,
	}
	if step.Bonus != nil {
		ev.BonusSides, ev.BonusFace = step.Bonus.Sides, step.Bonus.Face
	}
	if action.DC > 0 {
		ev.Passed = new(d20.Total >= action.DC)
	}
	c.out.ev = ev
	tc, err := c.s.openTx(ctx, combatTx{tx: c.tx, q: c.q, session: session, now: c.s.now(), characterID: &c.out.who.ID, hash: c.hash})
	if err != nil {
		return err
	}
	c.out.at = tc.now
	if err := c.s.spendOnceEffects(ctx, tc, cm.Rolled); err != nil {
		return err
	}
	if err := c.s.spendCheckHelps(ctx, tc, cm.Helps); err != nil {
		return err
	}
	c.out.rollID, err = insertSceneEvent(ctx, tc, eventSceneCheckRolled, &c.m.UserID, &c.key, ev)
	return err
}

// grantSceneCall is a grant of one more attempt being made.
type grantSceneCall struct {
	s           *Service
	m           authz.Membership
	key         string
	actionID    string
	characterID string
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
	call := grantSceneCall{s: s, m: m, key: key, actionID: actionID.String(), characterID: characterID.String()}

	var granted bool
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		granted = false
		return call.run(ctx, tx, &granted)
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

// run is the transaction of a grant: a retry changes nothing; otherwise the action and the
// character must be the open scene's and the party's, and an action with a limit gets an event.
func (g grantSceneCall) run(ctx context.Context, tx pgx.Tx, granted *bool) error {
	q := g.s.queries.WithTx(tx)
	session, err := lockSceneSession(ctx, q, g.m.CampaignID)
	if err != nil {
		return err
	}
	// A retry of a grant already made changes nothing.
	if replayed, err := g.replay(ctx, q, session.ID); replayed || err != nil {
		return err
	}
	scene, err := g.s.openSceneIn(ctx, tx, g.m.CampaignID, session)
	if err != nil {
		return err
	}
	action, err := sceneActionOf(scene, g.actionID)
	if err != nil {
		return err
	}
	party, err := g.s.roster.CombatParty(ctx, tx, g.m.CampaignID)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(party, func(c link.Character) bool { return c.ID == g.characterID }) {
		return errSceneCharacterNotFound()
	}
	if action.MaxAttempts == 0 {
		return nil // unlimited: there is nothing to add
	}
	c, err := g.s.openTx(ctx, combatTx{tx: tx, q: q, session: session, now: g.s.now(), characterID: &g.characterID})
	if err != nil {
		return err
	}
	*granted = true
	_, err = insertSceneEvent(ctx, c, eventSceneAttemptGranted, &g.m.UserID, &g.key, sceneGrantEvent{PointID: scene.PointID, ActionID: action.ID})
	return err
}

// replay says whether the key was used by this very grant already, and refuses a key that
// was used for another change.
func (g grantSceneCall) replay(ctx context.Context, q *playdb.Queries, sessionID string) (bool, error) {
	done, err := q.GetSessionEventByIdempotencyKey(ctx, playdb.GetSessionEventByIdempotencyKeyParams{GameSessionID: sessionID, IdempotencyKey: &g.key})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("find the event of this idempotency key: %w", err)
	}
	var prev sceneGrantEvent
	if done.Kind != eventSceneAttemptGranted || json.Unmarshal(done.Payload, &prev) != nil ||
		prev.ActionID != g.actionID || deref(done.CharacterID) != g.characterID {
		return false, errKeyReused()
	}
	return true, nil
}

func errSceneCharacterNotFound() error {
	return connect.NewError(connect.CodeNotFound, errors.New("character not found"))
}
