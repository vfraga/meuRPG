package maps

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/maps/mapsdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/platform/names"
	"github.com/PuraFome/meuRPG/backend/internal/platform/sessiontell"
	"github.com/PuraFome/meuRPG/backend/internal/platform/uid"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
)

// The actions of an RP scene (MR-015): a skill check, an
// ability check or a saving throw on a SCENE point, each with an optional
// name and an optional DC. The master changes them one at a time, as the
// artboard shows, so there is no "save all": every call below is one change,
// and answers with the point's actions as they are now.
//
// Playing the scene (opening it in the session, rolling, the log) is package
// play's; this package keeps the actions, which belong to the map point, and
// gives play the scene through SessionMaps.ScenePoint. A player gets the
// actions of a point they see, with the DC only when the scene shows it
// (show_dc, RN-20); the open
// scene carries the actions of a hidden point too, but that is play's read.

const (
	// maxSceneActions is how many actions a scene point may have (question
	// 51's default; the master's list is short, and so is the player's screen).
	maxSceneActions = 20
	// maxActionName is the longest action name, in characters
	// (scene_actions_name_length).
	maxActionName = 60
	// maxDC is the highest difficulty class (scene_actions_dc_valid).
	maxDC = 30
	// defaultAttempts is how many times a player's character may roll a new
	// action; maxAttempts is the most the master may allow, and 0 means
	// unlimited (scene_actions_max_attempts_valid).
	defaultAttempts = 1
	maxAttempts     = 5
)

// SceneChecks tells which checks a scene may ask for. The rules module
// implements it (rules.Content.SceneCheckName), so the catalog of skills and
// abilities lives in one place, and nothing that belongs to the combat (an
// attack, a spell, a feature) can be a scene action.
type SceneChecks interface {
	// SceneCheckName is the Portuguese name of a scene check by its key
	// ("skill:investigation", "ability:str", "save:wis"), and false for any
	// other key.
	SceneCheckName(key string) (string, bool)
}

// The offset a direction moves an item in a list by.
const (
	stepUp   = -1
	stepDown = 1
)

// moveStep is the offset of a move's direction, or a refusal for no direction.
func moveStep(direction mapsv1.SceneActionDirection) (int, error) {
	switch direction {
	case mapsv1.SceneActionDirection_SCENE_ACTION_DIRECTION_UP:
		return stepUp, nil
	case mapsv1.SceneActionDirection_SCENE_ACTION_DIRECTION_DOWN:
		return stepDown, nil
	default:
		return 0, connect.NewError(connect.CodeInvalidArgument, errors.New("direction is required"))
	}
}

// newAction is a checked AddSceneAction: its fields as they are stored.
type newAction struct {
	key      string
	name     string
	dc       *int32
	attempts int32
	hidden   bool
}

// addActionCall is one AddSceneAction inside its transaction: the action to
// add and its idempotency, and the rows and whether it was a replay out.
type addActionCall struct {
	pointID     string
	action      newAction
	scopedKey   *string
	requestHash *string
	added       mapsdb.SceneAction
	actions     []mapsdb.SceneAction
	replayed    bool
}

// AddSceneAction implements mapsv1connect.MapServiceHandler.
func (s *Service) AddSceneAction(
	ctx context.Context,
	req *connect.Request[mapsv1.AddSceneActionRequest],
) (*connect.Response[mapsv1.AddSceneActionResponse], error) {
	m, mapID, pointID, err := s.sceneCall(ctx, req.Msg.GetCampaignId(), req.Msg.GetMapId(), req.Msg.GetPointId())
	if err != nil {
		return nil, err
	}
	action, err := s.cleanNewAction(req.Msg)
	if err != nil {
		return nil, err
	}
	// The key is unique in the campaign, and kept with a hash of the whole request: a retry
	// returns the first action only when it is the same request.
	scopedKey, requestHash, err := keyOf(m.CampaignID, req.Msg.GetIdempotencyKey(), req.Msg)
	if err != nil {
		return nil, err
	}
	call := &addActionCall{pointID: pointID, action: action, scopedKey: scopedKey, requestHash: requestHash}
	err = s.changeActions(ctx, m, mapID, pointID, func(q *mapsdb.Queries) error {
		return s.appendAction(ctx, q, call)
	}, func(ch changedPoint) *sceneTold {
		return &sceneTold{replayed: call.replayed, tell: func(ctx context.Context) { s.actionsChangedFor(ctx, m.CampaignID, ch, !action.hidden) }}
	})
	if err != nil {
		return nil, s.dbError(ctx, "add a scene action", err)
	}
	return connect.NewResponse(&mapsv1.AddSceneActionResponse{
		Action: s.actionToProto(call.added, true), Actions: s.actionsToProto(call.actions, true),
	}), nil
}

// cleanNewAction checks the fields of an AddSceneAction in order: the key, the
// name, the DC and the attempts (one when not given).
func (s *Service) cleanNewAction(msg *mapsv1.AddSceneActionRequest) (newAction, error) {
	key, err := s.checkKey(msg.GetKey())
	if err != nil {
		return newAction{}, err
	}
	name, err := cleanActionName(msg.GetName())
	if err != nil {
		return newAction{}, err
	}
	dc, err := cleanDC(msg.GetDc())
	if err != nil {
		return newAction{}, err
	}
	attempts := int32(defaultAttempts)
	if msg.MaxAttempts != nil {
		if attempts, err = cleanAttempts(msg.GetMaxAttempts()); err != nil {
			return newAction{}, err
		}
	}
	return newAction{key: key, name: name, dc: dc, attempts: attempts, hidden: msg.GetHidden()}, nil
}

// appendAction reads the point's actions and adds the new one after them, or
// finds the one an earlier call with the same key added, and leaves the list
// as the caller answers it.
func (s *Service) appendAction(ctx context.Context, q *mapsdb.Queries, call *addActionCall) error {
	// The point is locked by now, so two calls with the same key take turns.
	current, err := q.ListSceneActions(ctx, call.pointID)
	if err != nil {
		return fmt.Errorf("list the actions: %w", err)
	}
	call.added, call.replayed, err = idem.Create(ctx, call.scopedKey, call.requestHash, q.GetSceneActionByCreateKey,
		func(a mapsdb.SceneAction) *string { return a.CreateHash },
		func() (mapsdb.SceneAction, error) { return s.insertAction(ctx, q, call, current) })
	if err != nil {
		return err
	}
	call.actions = current
	if !call.replayed {
		call.actions = append(call.actions, call.added)
	}
	return nil
}

// insertAction adds an action after the current ones, refusing the one over
// the limit. A conflict on the key is pgx.ErrNoRows, for idem.Create to read.
func (s *Service) insertAction(ctx context.Context, q *mapsdb.Queries, call *addActionCall, current []mapsdb.SceneAction) (mapsdb.SceneAction, error) {
	if len(current) >= maxSceneActions {
		return call.added, connect.NewError(connect.CodeResourceExhausted, fmt.Errorf("the scene already has %d actions", maxSceneActions))
	}
	position := int32(0)
	if n := len(current); n > 0 {
		position = current[n-1].Position + 1
	}
	a, err := q.InsertSceneAction(ctx, mapsdb.InsertSceneActionParams{
		PointID: call.pointID, Position: position, Key: call.action.key, Name: call.action.name, Dc: call.action.dc, MaxAttempts: call.action.attempts,
		Hidden: call.action.hidden, CreateKey: call.scopedKey, CreateHash: call.requestHash, Now: s.now(),
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return a, fmt.Errorf("insert action: %w", err)
	}
	return a, err
}

// actionEdit is a checked UpdateSceneAction: a nil field stays as it is. The
// DC is nil to remove it, so dcSet says whether the request carried one.
type actionEdit struct {
	key      *string
	name     *string
	dc       *int32
	dcSet    bool
	attempts *int32
	hidden   *bool
}

// UpdateSceneAction implements mapsv1connect.MapServiceHandler.
func (s *Service) UpdateSceneAction(
	ctx context.Context,
	req *connect.Request[mapsv1.UpdateSceneActionRequest],
) (*connect.Response[mapsv1.UpdateSceneActionResponse], error) {
	m, mapID, pointID, err := s.sceneCall(ctx, req.Msg.GetCampaignId(), req.Msg.GetMapId(), req.Msg.GetPointId())
	if err != nil {
		return nil, err
	}
	actionID, ok := uid.Canonical(req.Msg.GetActionId())
	if !ok {
		return nil, errActionNotFound()
	}
	edit, err := s.cleanActionEdit(req.Msg)
	if err != nil {
		return nil, err
	}

	var before, changed mapsdb.SceneAction
	var actions []mapsdb.SceneAction
	err = s.changeActions(ctx, m, mapID, pointID, func(q *mapsdb.Queries) error {
		var err error
		before, changed, actions, err = s.editAction(ctx, q, pointID, actionID, edit)
		return err
	}, func(ch changedPoint) *sceneTold {
		// An action that was hidden and stays hidden is the master's business alone.
		return &sceneTold{tell: func(ctx context.Context) {
			s.actionsChangedFor(ctx, m.CampaignID, ch, !before.Hidden || !changed.Hidden)
		}}
	})
	if err != nil {
		return nil, s.dbError(ctx, "update a scene action", err)
	}
	return connect.NewResponse(&mapsv1.UpdateSceneActionResponse{
		Action: s.actionToProto(changed, true), Actions: s.actionsToProto(actions, true),
	}), nil
}

// cleanOptional checks a field a request may leave out: nil stays nil.
func cleanOptional[In, Out any](in *In, clean func(In) (Out, error)) (*Out, error) {
	if in == nil {
		return nil, nil
	}
	out, err := clean(*in)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// cleanActionEdit checks the fields of an UpdateSceneAction in order: at
// least one must be there, then the key, the name, the DC and the attempts.
func (s *Service) cleanActionEdit(msg *mapsv1.UpdateSceneActionRequest) (actionEdit, error) {
	if msg.Key == nil && msg.Name == nil && msg.Dc == nil && msg.MaxAttempts == nil && msg.Hidden == nil {
		return actionEdit{}, connect.NewError(connect.CodeInvalidArgument, errors.New("nothing to change"))
	}
	var edit actionEdit
	var err error
	if edit.key, err = cleanOptional(msg.Key, s.checkKey); err != nil {
		return actionEdit{}, err
	}
	if edit.name, err = cleanOptional(msg.Name, cleanActionName); err != nil {
		return actionEdit{}, err
	}
	if edit.dcSet = msg.Dc != nil; edit.dcSet {
		if edit.dc, err = cleanDC(msg.GetDc()); err != nil { // nil removes the DC
			return actionEdit{}, err
		}
	}
	if edit.attempts, err = cleanOptional(msg.MaxAttempts, cleanAttempts); err != nil {
		return actionEdit{}, err
	}
	edit.hidden = msg.Hidden
	return edit, nil
}

// editAction locks the action, applies the edit and reads the point's
// actions as they are now. It returns the action as it was and as it is.
func (s *Service) editAction(ctx context.Context, q *mapsdb.Queries, pointID, actionID string, edit actionEdit) (before, after mapsdb.SceneAction, actions []mapsdb.SceneAction, err error) {
	a, err := q.GetSceneActionForUpdate(ctx, mapsdb.GetSceneActionForUpdateParams{PointID: pointID, ID: actionID})
	if errors.Is(err, pgx.ErrNoRows) {
		return a, a, nil, errActionNotFound()
	}
	if err != nil {
		return a, a, nil, fmt.Errorf("find action: %w", err)
	}
	params := mapsdb.UpdateSceneActionParams{PointID: pointID, ID: actionID, Key: a.Key, Name: a.Name, Dc: a.Dc, MaxAttempts: a.MaxAttempts, Hidden: a.Hidden, Now: s.now()}
	edit.applyTo(&params)
	changed, err := q.UpdateSceneAction(ctx, params)
	if err != nil {
		return a, changed, nil, fmt.Errorf("update action: %w", err)
	}
	actions, err = q.ListSceneActions(ctx, pointID)
	if err != nil {
		return a, changed, actions, fmt.Errorf("list the actions: %w", err)
	}
	return a, changed, actions, nil
}

func (e actionEdit) applyTo(params *mapsdb.UpdateSceneActionParams) {
	if e.key != nil {
		params.Key = *e.key
	}
	if e.name != nil {
		params.Name = *e.name
	}
	if e.dcSet {
		params.Dc = e.dc
	}
	if e.attempts != nil {
		params.MaxAttempts = *e.attempts
	}
	if e.hidden != nil {
		params.Hidden = *e.hidden
	}
}

// MoveSceneAction implements mapsv1connect.MapServiceHandler.
func (s *Service) MoveSceneAction(
	ctx context.Context,
	req *connect.Request[mapsv1.MoveSceneActionRequest],
) (*connect.Response[mapsv1.MoveSceneActionResponse], error) {
	m, mapID, pointID, err := s.sceneCall(ctx, req.Msg.GetCampaignId(), req.Msg.GetMapId(), req.Msg.GetPointId())
	if err != nil {
		return nil, err
	}
	actionID, ok := uid.Canonical(req.Msg.GetActionId())
	if !ok {
		return nil, errActionNotFound()
	}
	step, err := moveStep(req.Msg.GetDirection())
	if err != nil {
		return nil, err
	}

	var actions []mapsdb.SceneAction
	moved := false
	err = s.changeActions(ctx, m, mapID, pointID, func(q *mapsdb.Queries) error {
		var err error
		actions, moved, err = shiftAction(ctx, q, pointID, actionID, step)
		return err
	}, func(ch changedPoint) *sceneTold {
		if !moved {
			return nil
		}
		// Moving a hidden action changes nothing the players read.
		i := slices.IndexFunc(actions, func(a mapsdb.SceneAction) bool { return a.ID == actionID })
		return &sceneTold{tell: func(ctx context.Context) { s.actionsChangedFor(ctx, m.CampaignID, ch, i < 0 || !actions[i].Hidden) }}
	})
	if err != nil {
		return nil, s.dbError(ctx, "move a scene action", err)
	}
	return connect.NewResponse(&mapsv1.MoveSceneActionResponse{Actions: s.actionsToProto(actions, true)}), nil
}

// shiftAction swaps an action with its neighbor and renumbers the list
// from 0: positions may have gaps after a removal, and a swap of two values
// would leave equal ones. It returns the list as it is and whether anything
// changed: the first action moved up, or the last moved down, changes nothing.
func shiftAction(ctx context.Context, q *mapsdb.Queries, pointID, actionID string, step int) ([]mapsdb.SceneAction, bool, error) {
	actions, err := q.ListSceneActions(ctx, pointID)
	if err != nil {
		return actions, false, fmt.Errorf("list the actions: %w", err)
	}
	i := slices.IndexFunc(actions, func(a mapsdb.SceneAction) bool { return a.ID == actionID })
	if i < 0 {
		return actions, false, errActionNotFound()
	}
	moved, err := swapAndRenumber(actions, i, step,
		func(a *mapsdb.SceneAction) *int32 { return &a.Position },
		func(a mapsdb.SceneAction) error {
			if err := q.SetSceneActionPosition(ctx, mapsdb.SetSceneActionPositionParams{ID: a.ID, Position: a.Position}); err != nil {
				return fmt.Errorf("set an action's position: %w", err)
			}
			return nil
		})
	return actions, moved, err
}

// swapAndRenumber swaps the row at i with its neighbor step away and renumbers the list from 0:
// positions may have gaps after a removal, and a swap of two values would leave equal ones.
// store writes each row whose position changed. It says whether anything changed: the first row
// moved up, or the last moved down, changes nothing.
func swapAndRenumber[T any](rows []T, i, step int, position func(*T) *int32, store func(T) error) (bool, error) {
	j := i + step
	if j < 0 || j >= len(rows) {
		return false, nil
	}
	rows[i], rows[j] = rows[j], rows[i]
	for p := range rows {
		want := int32(p)
		if *position(&rows[p]) == want {
			continue
		}
		*position(&rows[p]) = want
		if err := store(rows[p]); err != nil {
			return false, err
		}
	}
	return true, nil
}

// RemoveSceneAction implements mapsv1connect.MapServiceHandler.
func (s *Service) RemoveSceneAction(
	ctx context.Context,
	req *connect.Request[mapsv1.RemoveSceneActionRequest],
) (*connect.Response[mapsv1.RemoveSceneActionResponse], error) {
	m, mapID, pointID, err := s.sceneCall(ctx, req.Msg.GetCampaignId(), req.Msg.GetMapId(), req.Msg.GetPointId())
	if err != nil {
		return nil, err
	}
	actionID, ok := uid.Canonical(req.Msg.GetActionId())
	if !ok {
		return nil, errActionNotFound()
	}
	var actions []mapsdb.SceneAction
	var removedHidden bool
	err = s.changeActions(ctx, m, mapID, pointID, func(q *mapsdb.Queries) error {
		var err error
		if removedHidden, actions, err = s.removeAction(ctx, q, pointID, actionID); err != nil {
			return err
		}
		return nil
	}, func(ch changedPoint) *sceneTold {
		// the players never knew a hidden action
		return &sceneTold{tell: func(ctx context.Context) { s.actionsChangedFor(ctx, m.CampaignID, ch, !removedHidden) }}
	})
	if err != nil {
		return nil, s.dbError(ctx, "remove a scene action", err)
	}
	return connect.NewResponse(&mapsv1.RemoveSceneActionResponse{Actions: s.actionsToProto(actions, true)}), nil
}

// removeAction deletes the action and reads the point's actions as they are now. It says whether
// the action was hidden from the players.
func (s *Service) removeAction(ctx context.Context, q *mapsdb.Queries, pointID, actionID string) (bool, []mapsdb.SceneAction, error) {
	a, err := q.GetSceneActionForUpdate(ctx, mapsdb.GetSceneActionForUpdateParams{PointID: pointID, ID: actionID})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil, errActionNotFound()
	}
	if err != nil {
		return false, nil, fmt.Errorf("find action: %w", err)
	}
	if _, err := q.DeleteSceneAction(ctx, mapsdb.DeleteSceneActionParams{PointID: pointID, ID: actionID}); err != nil {
		return false, nil, fmt.Errorf("delete action: %w", err)
	}
	actions, err := q.ListSceneActions(ctx, pointID)
	if err != nil {
		return false, nil, fmt.Errorf("list the actions: %w", err)
	}
	return a.Hidden, actions, nil
}

// sceneCall is the start of every scene-action handler: the master's check,
// and the IDs of the map and the point.
func (s *Service) sceneCall(ctx context.Context, campaignID, rawMapID, rawPointID string) (m authz.Membership, mapID, pointID string, err error) {
	m, err = authz.RequireCampaignRole(ctx, campaignID, authz.RoleMaster)
	if err != nil {
		return m, "", "", err
	}
	mapID, ok := uid.Canonical(rawMapID)
	if !ok {
		return m, "", "", errMapNotFound()
	}
	pointID, ok = uid.Canonical(rawPointID)
	if !ok {
		return m, "", "", errPointNotFound()
	}
	return m, mapID, pointID, nil
}

// changedPoint is what a change to a point's actions needs to tell the
// watching members: the point, its map, and the session's current map.
type changedPoint struct {
	point      mapsdb.MapPoint
	mapRow     mapsdb.Map
	currentMap string
}

// sceneTold is what a change to a scene point's actions, clues or images tells once it committed: tell runs after the commit, and
// replayed says the change was already made (a key stored), so nothing moves and the notice goes out again.
type sceneTold struct {
	tell     func(ctx context.Context)
	replayed bool
}

// changeActions runs one change to a scene point's actions in a transaction, under the session's lock:
// it checks that the map is the campaign's and the point is a SCENE point of
// it, locks the point so two changes take turns (the 20-action limit holds),
// and lets change work on the rows. Once change succeeded, then says what the change tells
// (nil: it changed nothing, so nothing moves and nobody hears).
func (s *Service) changeActions(ctx context.Context, m authz.Membership, mapID, pointID string, change func(q *mapsdb.Queries) error, then func(ch changedPoint) *sceneTold) error {
	current, err := s.currentMap(ctx, m.CampaignID)
	if err != nil {
		return err
	}
	return s.inSession(ctx, m.CampaignID, func(tx pgx.Tx, t sessiontell.Tell) error {
		out := changedPoint{currentMap: current}
		q := s.queries.WithTx(tx)
		var err error
		if out.mapRow, err = s.campaignMap(ctx, q, m.CampaignID, mapID); err != nil {
			return err
		}
		if out.point, err = findPointForUpdate(ctx, q, mapID, pointID); err != nil {
			return err
		}
		if out.point.Kind != kindToDB[mapsv1.MapPointKind_MAP_POINT_KIND_SCENE] {
			return connect.NewError(connect.CodeInvalidArgument, errors.New("only a SCENE point has actions and clues"))
		}
		if err := change(q); err != nil {
			return err
		}
		if w := then(out); w != nil {
			if w.replayed {
				t.Replay()
			}
			s.announce(t, w.tell)
		}
		return nil
	})
}

// actionsChanged tells the watching members that a scene point's actions
// changed: the master's map reads them, a player's too when the point is
// revealed on a map they see (map_changed, as any change to a point), and
// everyone in the session when the point is the open scene (scene_changed).
func (s *Service) actionsChanged(ctx context.Context, campaignID string, ch changedPoint) {
	s.publishPointsChanged(ctx, campaignID, ch.mapRow,
		playersSee(ch.mapRow.ID, ch.mapRow.RevealedAt, ch.currentMap) && ch.point.RevealedAt != nil, ch.point)
	s.publishSceneChangedIf(ctx, campaignID, ch.point.ID)
}

// actionsChangedFor is actionsChanged for a change the players may not read: when players is false
// (an action that is hidden from them, and stays so) only the master's streams hear of it.
func (s *Service) actionsChangedFor(ctx context.Context, campaignID string, ch changedPoint, players bool) {
	if players {
		s.actionsChanged(ctx, campaignID, ch)
		return
	}
	s.cluesChanged(ctx, campaignID, ch)
}

// publishSceneChangedIf tells everyone that the open scene changed, when the
// point is the scene open in the session. The hint names nothing: the app
// reads the scene again, filtered for it.
func (s *Service) publishSceneChangedIf(ctx context.Context, campaignID, pointID string) {
	open, err := s.live.OpenScenePoint(ctx, campaignID)
	if err != nil {
		s.logger.ErrorContext(ctx, "maps: read the open scene", "error", err)
		return
	}
	if open != "" && open == pointID {
		s.publishSceneChanged(ctx, campaignID)
	}
}

// publishSceneChanged tells the master and the players in the open scene that it
// changed (everyone, when it is open to the whole table).
func (s *Service) publishSceneChanged(ctx context.Context, campaignID string) {
	s.publishScene(ctx, campaignID, &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_SceneChanged_{
		SceneChanged: &playv1.WatchGameSessionResponse_SceneChanged{},
	}})
}

// checkKey checks a scene check's key against the rules' catalog.
func (s *Service) checkKey(key string) (string, error) {
	if _, ok := s.checks.SceneCheckName(key); !ok {
		return "", connect.NewError(connect.CodeInvalidArgument,
			errors.New("key must be a skill, an ability check or a saving throw (skill:investigation, ability:str, save:wis)"))
	}
	return key, nil
}

// cleanActionName checks an action's name: empty for none, else one line of
// up to 60 characters. The message says the rule, never the name.
func cleanActionName(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	name, err := names.Clean(raw, maxActionName)
	if err != nil {
		return "", connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("name %w", err))
	}
	return name, nil
}

// cleanDC checks a difficulty class: 0 for none, else 1 to 30.
func cleanDC(dc int32) (*int32, error) {
	switch {
	case dc == 0:
		return nil, nil
	case dc < 1 || dc > maxDC:
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("dc must be 1 to %d, or 0 for none", maxDC))
	}
	return &dc, nil
}

// cleanAttempts checks an action's attempts per player: 1 to 5, or 0 for
// unlimited.
func cleanAttempts(n int32) (int32, error) {
	if n < 0 || n > maxAttempts {
		return 0, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("max_attempts must be 1 to %d, or 0 for unlimited", maxAttempts))
	}
	return n, nil
}

func errActionNotFound() error {
	return connect.NewError(connect.CodeNotFound, errors.New("action not found"))
}

// actionToProto builds a SceneAction. The DC goes to the master, and to a
// player only when the scene shows it (RN-20): withDC says whether the viewer
// gets it.
func (s *Service) actionToProto(a mapsdb.SceneAction, withDC bool) *mapsv1.SceneAction {
	checkName, _ := s.checks.SceneCheckName(a.Key)
	out := &mapsv1.SceneAction{Id: a.ID, Key: a.Key, Name: a.Name, CheckName: checkName, MaxAttempts: a.MaxAttempts, Hidden: a.Hidden}
	if withDC && a.Dc != nil {
		out.Dc = *a.Dc
	}
	return out
}

func (s *Service) actionsToProto(rows []mapsdb.SceneAction, withDC bool) []*mapsv1.SceneAction {
	out := make([]*mapsv1.SceneAction, 0, len(rows))
	for _, a := range rows {
		out = append(out, s.actionToProto(a, withDC))
	}
	return out
}

// attachActions fills the scene_actions of the SCENE points of a map, as the
// viewer sees them: a player gets a DC only from a scene that shows it
// (show_dc), and never an action the master hid (RN-10).
func (s *Service) attachActions(ctx context.Context, mapID string, points []*mapsv1.MapPoint, master bool) error {
	if !slices.ContainsFunc(points, func(p *mapsv1.MapPoint) bool { return p.GetKind() == mapsv1.MapPointKind_MAP_POINT_KIND_SCENE }) {
		return nil
	}
	rows, err := s.queries.ListSceneActionsOfMap(ctx, mapID)
	if err != nil {
		return err
	}
	byPoint := map[string][]mapsdb.SceneAction{}
	for _, r := range rows {
		if r.Hidden && !master {
			continue
		}
		byPoint[r.PointID] = append(byPoint[r.PointID], r)
	}
	for _, p := range points {
		p.SceneActions = s.actionsToProto(byPoint[p.GetId()], master || p.GetShowDc())
	}
	return nil
}

// ScenePoint implements play.MapKeeper (through SessionMaps): the scene of a
// SCENE point of the campaign, hidden or not, with its actions and their DCs, its hooks and its clues (the master's: package play gives them to the master only),
// or a `not_found` Connect error for any other point. Package play decides
// what each member sees of it.
func (sm *SessionMaps) ScenePoint(ctx context.Context, tx pgx.Tx, campaignID, pointID string) (link.Scene, error) {
	q := queriesIn(sm.queries, tx)
	p, err := q.GetScenePoint(ctx, mapsdb.GetScenePointParams{CampaignID: campaignID, ID: pointID})
	if errors.Is(err, pgx.ErrNoRows) {
		return link.Scene{}, errPointNotFound()
	}
	if err != nil {
		return link.Scene{}, fmt.Errorf("find the scene point: %w", err)
	}
	rows, err := q.ListSceneActions(ctx, p.ID)
	if err != nil {
		return link.Scene{}, fmt.Errorf("list the scene's actions: %w", err)
	}
	out := link.Scene{PointID: p.ID, Name: p.Name, Description: p.Description, Hooks: p.Hooks, ShowDC: p.ShowDc}
	if out.Clues, err = sm.sceneClues(ctx, q, p.ID); err != nil {
		return link.Scene{}, err
	}
	if out.Images, err = sceneImages(ctx, q, p.ID); err != nil {
		return link.Scene{}, err
	}
	for _, a := range rows {
		act := link.SceneAction{ID: a.ID, Key: a.Key, Name: a.Name, MaxAttempts: int(a.MaxAttempts), Hidden: a.Hidden}
		if a.Dc != nil {
			act.DC = int(*a.Dc)
		}
		out.Actions = append(out.Actions, act)
	}
	return out, nil
}
