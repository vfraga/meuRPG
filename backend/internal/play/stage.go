package play

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"uuid"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// The stage: the NPCs "em cena" in the open RP scene (MR-031, D7; question 62
// of the progress doc, with its default).
//
//   - The master puts NPCs on the stage while a scene is open, takes them off,
//     and marks the one that speaks. At most maxStage at once, in the order
//     they came in. The stage is kept per session, in the table stage_npcs.
//   - Closing the scene, or opening another one, empties the stage (scene.go).
//   - Every member reads the stage with the open scene (GetOpenScene). A
//     player gets an NPC's name, its portrait and whether it speaks, and
//     nothing else about it, not even its character ID (RN-20): the entry's
//     own ID is the place on the stage.
//   - A player may fetch an NPC's portrait only while the NPC is on the stage
//     (ImageOnStage, which package maps asks on every request).
//   - Each change is a session event (ADR-0007): ids only.

// maxStage is how many NPCs can be on the stage at once: the screens fit four
// portraits at the narrowest width (E8-10).
const maxStage = 4

// imagesPath is where package maps serves the gallery's images (maps.ImagesPath).
const imagesPath = "/images/"

// stageEvent is the payload of stage_changed: ids only.
type stageEvent struct {
	// Change is "put", "taken_off", "speaker" or "cleared".
	Change string `json:"change"`
	// CharacterID is the NPC the change is about; empty for "cleared" and for
	// a speaker set to nobody.
	CharacterID string `json:"character_id,omitempty"`
}

const (
	stagePut      = "put"
	stageTakenOff = "taken_off"
	stageSpeaker  = "speaker"
	stageCleared  = "cleared"
)

// publishStageChanged tells everyone in the session that the stage changed. The
// hint names no NPC: each app reads the scene again, filtered for it.
func (s *Service) publishStageChanged(campaignID string) {
	s.Publish(campaignID, true, &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_StageChanged_{
		StageChanged: &playv1.WatchGameSessionResponse_StageChanged{},
	}})
}

// stageOf builds the stage as the caller sees it: the master gets the
// character IDs too. An NPC that is no longer a living character of the
// campaign is left out.
func (s *Service) stageOf(ctx context.Context, m authz.Membership, sessionID string) ([]*playv1.StageNpc, error) {
	rows, err := s.queries.ListStage(ctx, sessionID)
	if err != nil {
		return nil, s.dbError(ctx, "list the stage", err)
	}
	chars, err := s.stageCharacters(ctx, m.CampaignID, rows)
	if err != nil {
		return nil, s.dbError(ctx, "read the NPCs on the stage", err)
	}
	master := m.Role == authz.RoleMaster
	var out []*playv1.StageNpc
	for _, r := range rows {
		c, ok := chars[r.CharacterID]
		if !ok || c.Player {
			continue
		}
		npc := &playv1.StageNpc{Id: r.ID, Name: c.Name, Speaking: r.Speaking}
		if c.PortraitImageID != "" {
			npc.PortraitUrl = imagesPath + c.PortraitImageID
		}
		if master {
			npc.CharacterId = r.CharacterID
		}
		out = append(out, npc)
	}
	return out, nil
}

// stageCharacters reads the characters on the stage, by ID.
func (s *Service) stageCharacters(ctx context.Context, campaignID string, rows []playdb.StageNpc) (map[string]link.Character, error) {
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.CharacterID)
	}
	out := make(map[string]link.Character, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	chars, err := s.roster.CombatCharacters(ctx, nil, campaignID, ids)
	if err != nil {
		return nil, err
	}
	for _, c := range chars {
		out[c.ID] = c
	}
	return out, nil
}

// ImageOnStage says whether the gallery image is the portrait of an NPC on the
// stage of the open scene of the campaign's open session. It implements
// maps.LiveSession: the image route lets a player fetch a portrait only then
// (RN-10, RN-20), so a portrait is 404 for them the moment the NPC leaves the
// stage or the scene closes.
func (s *Service) ImageOnStage(ctx context.Context, campaignID, imageID string) (bool, error) {
	session, err := s.queries.GetOpenGameSession(ctx, campaignID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && session.OpenScenePointID == nil) {
		return false, nil // no session, or no scene: no stage
	}
	if err != nil {
		return false, fmt.Errorf("find the open session: %w", err)
	}
	// A point the master turned into another kind is no open scene (GetOpenScene
	// says so too), and its stage is not on show.
	if _, err := s.maps.ScenePoint(ctx, nil, campaignID, *session.OpenScenePointID); connect.CodeOf(err) == connect.CodeNotFound {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("find the open scene: %w", err)
	}
	rows, err := s.queries.ListStage(ctx, session.ID)
	if err != nil {
		return false, fmt.Errorf("list the stage: %w", err)
	}
	chars, err := s.stageCharacters(ctx, campaignID, rows)
	if err != nil {
		return false, fmt.Errorf("read the NPCs on the stage: %w", err)
	}
	for _, c := range chars {
		if !c.Player && c.PortraitImageID == imageID {
			return true, nil
		}
	}
	return false, nil
}

// stageChange is what one master's call to the stage does, inside the
// transaction. It returns the kind of change it made, "" when nothing changed.
type stageChange func(c *combatTx) (change, characterID string, err error)

// changeStage runs a change of the stage for the master: it locks the open
// session, applies the change, writes the stage_changed event, and answers with
// the stage. Everyone hears of it once it is committed.
func (s *Service) changeStage(ctx context.Context, m authz.Membership, what string, apply stageChange) ([]*playv1.StageNpc, error) {
	var changed bool
	var sessionID string
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		changed = false
		session, err := lockOpenSession(ctx, q, m.CampaignID)
		if err != nil {
			return err
		}
		sessionID = session.ID
		c, err := s.openTx(ctx, combatTx{tx: tx, q: q, session: session, now: s.now()})
		if err != nil {
			return err
		}
		change, characterID, err := apply(c)
		if err != nil || change == "" {
			return err
		}
		if characterID != "" {
			c.characterID = &characterID
		}
		if _, err := insertSceneEvent(ctx, c, eventStageChanged, &m.UserID, nil, stageEvent{Change: change, CharacterID: characterID}); err != nil {
			return err
		}
		changed = true
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, what, err)
	}
	if changed {
		s.publishStageChanged(m.CampaignID)
	}
	return s.stageOf(ctx, m, sessionID)
}

// parseStageNPC reads the NPC's ID from a request: an ID that is not a UUID
// names nothing.
func parseStageNPC(raw string) (string, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return "", errCharacterNotFound()
	}
	return id.String(), nil
}

// PutOnStage implements playv1connect.PlayServiceHandler.
func (s *Service) PutOnStage(
	ctx context.Context,
	req *connect.Request[playv1.PutOnStageRequest],
) (*connect.Response[playv1.PutOnStageResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	characterID, err := parseStageNPC(req.Msg.GetCharacterId())
	if err != nil {
		return nil, err
	}
	stage, err := s.changeStage(ctx, m, "put an NPC on the stage", func(c *combatTx) (string, string, error) {
		return s.putNPC(ctx, c, m.CampaignID, characterID)
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.PutOnStageResponse{Stage: stage}), nil
}

// putNPC is PutOnStage's change: it puts the NPC at the end of the stage, unless it is
// there already (nothing changes).
func (s *Service) putNPC(ctx context.Context, c *combatTx, campaignID, characterID string) (string, string, error) {
	if c.session.OpenScenePointID == nil {
		return "", "", errScene(playv1.SceneBlockedReason_SCENE_BLOCKED_REASON_NO_OPEN_SCENE, "no scene is open, so there is no stage")
	}
	if err := s.checkStageNPC(ctx, c, campaignID, characterID); err != nil {
		return "", "", err
	}
	rows, err := c.q.ListStage(ctx, c.session.ID)
	if err != nil {
		return "", "", fmt.Errorf("list the stage: %w", err)
	}
	if slices.ContainsFunc(rows, func(r playdb.StageNpc) bool { return r.CharacterID == characterID }) {
		return "", "", nil // already there: nothing changes
	}
	if len(rows) >= maxStage {
		return "", "", errScene(playv1.SceneBlockedReason_SCENE_BLOCKED_REASON_STAGE_FULL, "the stage is full")
	}
	if _, err := c.q.InsertStageNPC(ctx, playdb.InsertStageNPCParams{GameSessionID: c.session.ID, CharacterID: characterID, CreatedAt: c.now}); err != nil {
		return "", "", fmt.Errorf("put the NPC on the stage: %w", err)
	}
	return stagePut, characterID, nil
}

// checkStageNPC checks that the character is a living NPC of the campaign the master
// may put on the stage: a player's character is not one, nor the NPC of a monster.
func (s *Service) checkStageNPC(ctx context.Context, c *combatTx, campaignID, characterID string) error {
	chars, err := s.roster.CombatCharacters(ctx, c.tx, campaignID, []string{characterID})
	if err != nil {
		return err
	}
	if len(chars) != 1 || chars[0].Player {
		return errCharacterNotFound()
	}
	if chars[0].CombatOnly {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("a monster's NPC is not a character the master can put on the stage"))
	}
	return nil
}

// TakeOffStage implements playv1connect.PlayServiceHandler.
func (s *Service) TakeOffStage(
	ctx context.Context,
	req *connect.Request[playv1.TakeOffStageRequest],
) (*connect.Response[playv1.TakeOffStageResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	characterID, err := parseStageNPC(req.Msg.GetCharacterId())
	if err != nil {
		return nil, err
	}
	stage, err := s.changeStage(ctx, m, "take an NPC off the stage", func(c *combatTx) (string, string, error) {
		n, err := c.q.DeleteStageNPC(ctx, playdb.DeleteStageNPCParams{GameSessionID: c.session.ID, CharacterID: characterID})
		if err != nil {
			return "", "", fmt.Errorf("take the NPC off the stage: %w", err)
		}
		if n == 0 {
			return "", "", nil // not there: nothing changes
		}
		return stageTakenOff, characterID, nil
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.TakeOffStageResponse{Stage: stage}), nil
}

// SetSpeaker implements playv1connect.PlayServiceHandler.
func (s *Service) SetSpeaker(
	ctx context.Context,
	req *connect.Request[playv1.SetSpeakerRequest],
) (*connect.Response[playv1.SetSpeakerResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	var characterID string // empty: nobody speaks
	if req.Msg.GetCharacterId() != "" {
		if characterID, err = parseStageNPC(req.Msg.GetCharacterId()); err != nil {
			return nil, err
		}
	}
	stage, err := s.changeStage(ctx, m, "set the speaker", func(c *combatTx) (string, string, error) {
		return setSpeakerOf(ctx, c, characterID)
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.SetSpeakerResponse{Stage: stage}), nil
}

// setSpeakerOf is SetSpeaker's change: the NPC speaks, or nobody when it is empty.
func setSpeakerOf(ctx context.Context, c *combatTx, characterID string) (string, string, error) {
	rows, err := c.q.ListStage(ctx, c.session.ID)
	if err != nil {
		return "", "", fmt.Errorf("list the stage: %w", err)
	}
	speaking := slices.IndexFunc(rows, func(r playdb.StageNpc) bool { return r.Speaking })
	if characterID == "" {
		if speaking < 0 {
			return "", "", nil // nobody speaks already
		}
		if err := c.q.ClearStageSpeakers(ctx, c.session.ID); err != nil {
			return "", "", fmt.Errorf("clear the speaker: %w", err)
		}
		return stageSpeaker, "", nil
	}
	i := slices.IndexFunc(rows, func(r playdb.StageNpc) bool { return r.CharacterID == characterID })
	if i < 0 {
		return "", "", errCharacterNotFound() // not on the stage
	}
	if i == speaking {
		return "", "", nil // speaks already
	}
	// One speaks at a time: the old one stops before the new one starts.
	if err := c.q.ClearStageSpeakers(ctx, c.session.ID); err != nil {
		return "", "", fmt.Errorf("clear the speaker: %w", err)
	}
	if _, err := c.q.SetStageSpeaker(ctx, playdb.SetStageSpeakerParams{GameSessionID: c.session.ID, CharacterID: characterID}); err != nil {
		return "", "", fmt.Errorf("set the speaker: %w", err)
	}
	return stageSpeaker, characterID, nil
}

// clearStage empties the session's stage inside tx, and writes the event when
// somebody was on it. It reports whether the stage changed, so the caller
// publishes the hint after the commit. The scene closing or changing calls it.
func clearStage(ctx context.Context, c *combatTx, actor string) (bool, error) {
	n, err := c.q.ClearStage(ctx, c.session.ID)
	if err != nil {
		return false, fmt.Errorf("clear the stage: %w", err)
	}
	if n == 0 {
		return false, nil
	}
	if _, err := insertSceneEvent(ctx, c, eventStageChanged, &actor, nil, stageEvent{Change: stageCleared}); err != nil {
		return false, err
	}
	return true, nil
}
