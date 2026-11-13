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
//   - The master may keep an NPC on the stage hidden from the players
//     (stage_npcs.hidden): it counts for the limit, the master reads it marked,
//     and no player ever receives it (RN-10): not in the open scene, not its
//     portrait, not the casts and targets of the stage, not a hint when it comes
//     in or goes out. Hiding and showing it send the hint everyone gets for a
//     change of the stage, so the players read the scene again. A hidden NPC does
//     not speak: hiding it takes the floor, and giving it the floor shows it.
//   - A player may fetch an NPC's portrait only while the NPC is on the stage
//     and shown (ImageOnStage, which package maps asks on every request).
//   - Each change is a session event (ADR-0007): ids only.

// maxStage is how many NPCs can be on the stage at once: the screens fit four
// portraits at the narrowest width (E8-10).
const maxStage = 4

// imagesPath is where package maps serves the gallery's images (maps.ImagesPath).
const imagesPath = "/images/"

// stageEvent is the payload of stage_changed: ids only.
type stageEvent struct {
	// Change is "put", "taken_off", "speaker", "hidden", "shown" or "cleared".
	Change string `json:"change"`
	// CharacterID is the NPC the change is about; empty for "cleared" and for
	// a speaker set to nobody.
	CharacterID string `json:"character_id,omitempty"`
	// Hidden says a "put" put the NPC on the stage hidden from the players.
	Hidden bool `json:"hidden,omitempty"`
	// Revealed says a "speaker" gave the floor to an NPC that was hidden, which showed it.
	Revealed bool `json:"revealed,omitempty"`
}

const (
	stagePut      = "put"
	stageTakenOff = "taken_off"
	stageSpeaker  = "speaker"
	stageCleared  = "cleared"
	stageHidden   = "hidden"
	stageShown    = "shown"
)

// stageChangedEvent is the hint that the stage changed. It names no NPC: each app reads the scene
// again, filtered for it.
func stageChangedEvent() *playv1.WatchGameSessionResponse {
	return &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_StageChanged_{
		StageChanged: &playv1.WatchGameSessionResponse_StageChanged{},
	}}
}

// publishStageChanged tells the session that the stage changed: the master and the players in the scene
// when the players saw the change, the master alone when only he did (a hidden NPC put on the stage or
// taken off it).
func (s *Service) publishStageChanged(ctx context.Context, campaignID string, players bool) {
	if players {
		s.PublishToScene(ctx, campaignID, stageChangedEvent())
		return
	}
	s.tellMembers(campaignID, false, stageChangedEvent())
}

// stageOf builds the stage as the caller sees it: the master gets the
// character IDs too, and every NPC, the hidden ones marked; a player gets the
// NPCs the players see and no other. An NPC that is no longer a living
// character of the campaign is left out.
func (s *Service) stageOf(ctx context.Context, m authz.Membership, sessionID string) ([]*playv1.StageNpc, error) {
	master := m.Role == authz.RoleMaster
	list := s.queries.ListShownStage
	if master {
		list = s.queries.ListStage
	}
	rows, err := list(ctx, sessionID)
	if err != nil {
		return nil, s.dbError(ctx, "list the stage", err)
	}
	chars, err := s.stageCharacters(ctx, m.CampaignID, rows)
	if err != nil {
		return nil, s.dbError(ctx, "read the NPCs on the stage", err)
	}
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
			npc.CharacterId, npc.Hidden = r.CharacterID, r.Hidden
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

// ImageOnStage says whether the gallery image is the portrait of an NPC the
// players see on the stage of the open scene of the campaign's open session. It implements
// maps.LiveSession: the image route lets a player fetch a portrait only then
// (RN-10, RN-20), so a portrait is 404 for them the moment the NPC leaves the
// stage or the scene closes. An empty userID is nobody in particular: it sees the stage only of a
// scene open to the whole table.
func (s *Service) ImageOnStage(ctx context.Context, campaignID, userID, imageID string) (bool, error) {
	return s.portraitOnStage(ctx, campaignID, imageID, func(session playdb.GameSession) (bool, error) {
		return outsideOpenScene(ctx, s.queries, session, userID)
	})
}

// ImageOnAnyStage says whether the gallery image is the portrait of an NPC shown on the stage of the
// open scene, whoever the scene is open to. It implements maps.LiveSession, for the question of whether an
// image is in use (never for what a player may fetch).
func (s *Service) ImageOnAnyStage(ctx context.Context, campaignID, imageID string) (bool, error) {
	return s.portraitOnStage(ctx, campaignID, imageID, func(playdb.GameSession) (bool, error) { return false, nil })
}

// portraitOnStage is the question of ImageOnStage; outside says whether the reader is out of a scene open to
// only some players, who sees no stage.
func (s *Service) portraitOnStage(ctx context.Context, campaignID, imageID string, outside func(playdb.GameSession) (bool, error)) (bool, error) {
	session, err := s.queries.GetOpenGameSession(ctx, campaignID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && session.OpenScenePointID == nil) {
		return false, nil // no session, or no scene: no stage
	}
	if err != nil {
		return false, fmt.Errorf("find the open session: %w", err)
	}
	if out, err := outside(session); err != nil || out {
		return false, err
	}
	// A point the master turned into another kind is no open scene (GetOpenScene
	// says so too), and its stage is not on show.
	if _, err := s.maps.ScenePoint(ctx, nil, campaignID, *session.OpenScenePointID); connect.CodeOf(err) == connect.CodeNotFound {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("find the open scene: %w", err)
	}
	rows, err := s.queries.ListShownStage(ctx, session.ID) // a hidden NPC's portrait is the master's alone
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

// stageDone is what one master's call to the stage did, inside the transaction. An empty change
// means nothing changed.
type stageDone struct {
	change      string
	characterID string
	// hidden: a put put the NPC on the stage hidden. revealed: the floor was given to a hidden NPC.
	hidden, revealed bool
	// master: only the master read the change, so the players are told nothing.
	master bool
}

// stageChange is one master's call to the stage, inside the transaction.
type stageChange func(c *combatTx) (stageDone, error)

// changeStage runs a change of the stage for the master: it locks the open
// session, applies the change, writes the stage_changed event, and answers with
// the stage. Once it is committed everyone hears of it, unless only the master
// read the change.
func (s *Service) changeStage(ctx context.Context, m authz.Membership, what string, apply stageChange) ([]*playv1.StageNpc, error) {
	var done stageDone
	var sessionID string
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		done = stageDone{}
		session, err := lockOpenSessionOrNone(ctx, q, m.CampaignID)
		if err != nil {
			return err
		}
		sessionID = session.ID
		c, err := s.openTx(ctx, combatTx{tx: tx, q: q, session: session, now: s.now()})
		if err != nil {
			return err
		}
		if done, err = apply(c); err != nil || done.change == "" {
			return err
		}
		if done.characterID != "" {
			c.characterID = &done.characterID
		}
		ev := stageEvent{Change: done.change, CharacterID: done.characterID, Hidden: done.hidden, Revealed: done.revealed}
		_, err = insertSceneEvent(ctx, c, eventStageChanged, &m.UserID, nil, ev)
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, what, err)
	}
	if done.change != "" {
		s.publishStageChanged(ctx, m.CampaignID, !done.master)
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
	stage, err := s.changeStage(ctx, m, "put an NPC on the stage", func(c *combatTx) (stageDone, error) {
		return s.putNPC(ctx, c, m.CampaignID, characterID, req.Msg.GetHidden())
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.PutOnStageResponse{Stage: stage}), nil
}

// putNPC is PutOnStage's change: it puts the NPC at the end of the stage, hidden from the players
// when the master said so, unless it is there already (nothing changes). A hidden NPC counts for the
// limit, and only the master hears of it.
func (s *Service) putNPC(ctx context.Context, c *combatTx, campaignID, characterID string, hidden bool) (stageDone, error) {
	if c.session.OpenScenePointID == nil {
		return stageDone{}, errScene(playv1.SceneBlockedReason_SCENE_BLOCKED_REASON_NO_OPEN_SCENE, "no scene is open, so there is no stage")
	}
	if err := s.checkStageNPC(ctx, c, campaignID, characterID); err != nil {
		return stageDone{}, err
	}
	rows, err := c.q.ListStage(ctx, c.session.ID)
	if err != nil {
		return stageDone{}, fmt.Errorf("list the stage: %w", err)
	}
	if slices.ContainsFunc(rows, func(r playdb.StageNpc) bool { return r.CharacterID == characterID }) {
		return stageDone{}, nil // already there: nothing changes
	}
	if len(rows) >= maxStage {
		return stageDone{}, errScene(playv1.SceneBlockedReason_SCENE_BLOCKED_REASON_STAGE_FULL, "the stage is full")
	}
	if _, err := c.q.InsertStageNPC(ctx, playdb.InsertStageNPCParams{GameSessionID: c.session.ID, CharacterID: characterID, Hidden: hidden, CreatedAt: c.now}); err != nil {
		return stageDone{}, fmt.Errorf("put the NPC on the stage: %w", err)
	}
	return stageDone{change: stagePut, characterID: characterID, hidden: hidden, master: hidden}, nil
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
	stage, err := s.changeStage(ctx, m, "take an NPC off the stage", func(c *combatTx) (stageDone, error) {
		wasHidden, err := c.q.DeleteStageNPC(ctx, playdb.DeleteStageNPCParams{GameSessionID: c.session.ID, CharacterID: characterID})
		if errors.Is(err, pgx.ErrNoRows) {
			return stageDone{}, nil // not there: nothing changes
		}
		if err != nil {
			return stageDone{}, fmt.Errorf("take the NPC off the stage: %w", err)
		}
		return stageDone{change: stageTakenOff, characterID: characterID, master: wasHidden}, nil
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.TakeOffStageResponse{Stage: stage}), nil
}

// SetStageNpcHidden implements playv1connect.PlayServiceHandler.
func (s *Service) SetStageNpcHidden(
	ctx context.Context,
	req *connect.Request[playv1.SetStageNpcHiddenRequest],
) (*connect.Response[playv1.SetStageNpcHiddenResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	characterID, err := parseStageNPC(req.Msg.GetCharacterId())
	if err != nil {
		return nil, err
	}
	stage, err := s.changeStage(ctx, m, "show or hide an NPC on the stage", func(c *combatTx) (stageDone, error) {
		return setNPCHidden(ctx, c, characterID, req.Msg.GetHidden())
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.SetStageNpcHiddenResponse{Stage: stage}), nil
}

// setNPCHidden is SetStageNpcHidden's change: the NPC on the stage is hidden from the players, or
// shown to them. Hiding the NPC that speaks takes the floor from it. Setting what it already is
// changes nothing.
func setNPCHidden(ctx context.Context, c *combatTx, characterID string, hidden bool) (stageDone, error) {
	rows, err := c.q.ListStage(ctx, c.session.ID)
	if err != nil {
		return stageDone{}, fmt.Errorf("list the stage: %w", err)
	}
	i := slices.IndexFunc(rows, func(r playdb.StageNpc) bool { return r.CharacterID == characterID })
	if i < 0 {
		return stageDone{}, errCharacterNotFound() // not on the stage
	}
	if rows[i].Hidden == hidden {
		return stageDone{}, nil
	}
	if _, err := c.q.SetStageNPCHidden(ctx, playdb.SetStageNPCHiddenParams{GameSessionID: c.session.ID, CharacterID: characterID, Hidden: hidden}); err != nil {
		return stageDone{}, fmt.Errorf("show or hide the NPC on the stage: %w", err)
	}
	change := stageShown
	if hidden {
		change = stageHidden
	}
	return stageDone{change: change, characterID: characterID}, nil
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
	stage, err := s.changeStage(ctx, m, "set the speaker", func(c *combatTx) (stageDone, error) {
		return setSpeakerOf(ctx, c, characterID)
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.SetSpeakerResponse{Stage: stage}), nil
}

// setSpeakerOf is SetSpeaker's change: the NPC speaks, or nobody when it is empty. An NPC hidden
// from the players is shown by it: the master's tap means that it speaks now.
func setSpeakerOf(ctx context.Context, c *combatTx, characterID string) (stageDone, error) {
	rows, err := c.q.ListStage(ctx, c.session.ID)
	if err != nil {
		return stageDone{}, fmt.Errorf("list the stage: %w", err)
	}
	speaking := slices.IndexFunc(rows, func(r playdb.StageNpc) bool { return r.Speaking })
	if characterID == "" {
		if speaking < 0 {
			return stageDone{}, nil // nobody speaks already
		}
		if err := c.q.ClearStageSpeakers(ctx, c.session.ID); err != nil {
			return stageDone{}, fmt.Errorf("clear the speaker: %w", err)
		}
		return stageDone{change: stageSpeaker}, nil
	}
	i := slices.IndexFunc(rows, func(r playdb.StageNpc) bool { return r.CharacterID == characterID })
	if i < 0 {
		return stageDone{}, errCharacterNotFound() // not on the stage
	}
	if i == speaking {
		return stageDone{}, nil // speaks already
	}
	// One speaks at a time: the old one stops before the new one starts.
	if err := c.q.ClearStageSpeakers(ctx, c.session.ID); err != nil {
		return stageDone{}, fmt.Errorf("clear the speaker: %w", err)
	}
	if _, err := c.q.SetStageSpeaker(ctx, playdb.SetStageSpeakerParams{GameSessionID: c.session.ID, CharacterID: characterID}); err != nil {
		return stageDone{}, fmt.Errorf("set the speaker: %w", err)
	}
	return stageDone{change: stageSpeaker, characterID: characterID, revealed: rows[i].Hidden}, nil
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
