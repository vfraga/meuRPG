package play

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The projector of the table screen: it copies a whitelist of fields into the Tv* messages from the reads a
// player makes, run as the TV's viewer (tvViewer). A field added to a message those reads answer never
// reaches the TV; a new TV field is a new line here, and TestTvProjectorFields makes it a decision.

const (
	tvMaxRolls    = 3 // the free rolls on the stage
	tvMaxLogLines = 6 // the lines of the combat log
)

// projectStageBody sets the content of the stage the kind asks for. When the item has gone since the
// facts were read (the shown image was taken down, the combat ended, the map is gone), the stage becomes the idle screen.
func (s *Service) projectStageBody(ctx context.Context, session playdb.GameSession, stage *playv1.TableScreenStage) error {
	tv := tvViewer(session.CampaignID)
	var err error
	switch stage.GetKind() {
	case playv1.TableScreenKind_TABLE_SCREEN_KIND_IDLE:
		stage.Body = &playv1.TableScreenStage_Idle{Idle: &playv1.TvIdle{}}
	case playv1.TableScreenKind_TABLE_SCREEN_KIND_MAP:
		err = s.projectMap(ctx, tv, session, stage)
	case playv1.TableScreenKind_TABLE_SCREEN_KIND_IMAGE:
		err = s.projectImage(ctx, tv, session, stage)
	case playv1.TableScreenKind_TABLE_SCREEN_KIND_SCENE:
		err = s.projectScene(ctx, tv, session, stage)
	case playv1.TableScreenKind_TABLE_SCREEN_KIND_COMBAT:
		err = s.projectCombat(ctx, tv, session, stage)
	}
	if err != nil {
		return err
	}
	if stage.GetBody() == nil {
		stage.Kind, stage.Body = playv1.TableScreenKind_TABLE_SCREEN_KIND_IDLE, &playv1.TableScreenStage_Idle{Idle: &playv1.TvIdle{}}
		return nil
	}
	return s.projectRolls(ctx, tv, session, stage)
}

// projectMap copies the session's current map, as a player with no character reads it. A map that is gone,
// or that no player sees, has no body: the stage falls to the idle screen.
func (s *Service) projectMap(ctx context.Context, tv authz.Membership, session playdb.GameSession, stage *playv1.TableScreenStage) error {
	if session.CurrentMapID == nil {
		return nil
	}
	m, err := s.tvMap(ctx, tv, *session.CurrentMapID)
	if err != nil || m == nil {
		return err
	}
	stage.Body = &playv1.TableScreenStage_Map{Map: m}
	return nil
}

// tvMap is the TV's form of a map: hidden_by_fog and nothing else when mapOnTV refuses it, otherwise what the
// maps module answers for the TV's viewer. Nil when the map is gone or no player sees it now: the field is
// then unset, which is not hidden_by_fog.
func (s *Service) tvMap(ctx context.Context, tv authz.Membership, mapID string) (*playv1.TvMap, error) {
	m, err := s.maps.TableScreenMap(ctx, tv.CampaignID, mapID, mapOnTV)
	if connect.CodeOf(err) == connect.CodeNotFound {
		return nil, nil
	}
	if err != nil {
		if _, ok := errors.AsType[*connect.Error](err); ok {
			return nil, err
		}
		return nil, s.dbError(ctx, "read the map for the table screen", err)
	}
	if m.Hidden {
		return &playv1.TvMap{HiddenByFog: true}, nil
	}
	return &playv1.TvMap{Name: m.Map.GetMap().GetName(), Map: m.Map, Vision: m.Vision, Revision: m.Revision}, nil
}

// projectImage copies the image shown to the whole table.
func (s *Service) projectImage(ctx context.Context, tv authz.Membership, session playdb.GameSession, stage *playv1.TableScreenStage) error {
	img, err := s.shownImageFor(ctx, tv, session)
	if err != nil || img == nil {
		return err
	}
	stage.Body = &playv1.TableScreenStage_Image{Image: &playv1.TvImage{
		Name: img.GetName(), Url: img.GetUrl(), Width: img.GetWidth(), Height: img.GetHeight(),
	}}
	return nil
}

// projectScene copies the scene open to the whole table: its name and description, and the NPCs the table
// sees on the stage. It reads the point and the shown stage, not the player's OpenSceneInfo, which carries
// actions, rolls and the group check of a player's character. A scene open to only some players has no body.
func (s *Service) projectScene(ctx context.Context, tv authz.Membership, session playdb.GameSession, stage *playv1.TableScreenStage) error {
	if session.OpenScenePointID == nil || !session.OpenSceneEveryone {
		return nil
	}
	scene, err := s.maps.ScenePoint(ctx, nil, tv.CampaignID, *session.OpenScenePointID)
	if connect.CodeOf(err) == connect.CodeNotFound {
		return nil
	}
	if err != nil {
		return s.dbError(ctx, "read the open scene", err)
	}
	npcs, err := s.stageOf(ctx, tv, session.ID)
	if err != nil {
		return err
	}
	out := &playv1.TvScene{Name: scene.Name, Description: scene.Description}
	for _, n := range npcs {
		out.Stage = append(out.Stage, &playv1.TvStageNpc{Name: n.GetName(), Speaking: n.GetSpeaking()})
	}
	stage.Body = &playv1.TableScreenStage_Scene{Scene: out}
	return nil
}

// projectCombat copies the combat the table sees: the combatants the table sees, the round, whose turn it is
// and the lines every player reads, and the combat's map when it has one.
func (s *Service) projectCombat(ctx context.Context, tv authz.Membership, session playdb.GameSession, stage *playv1.TableScreenStage) error {
	var d *encounterData
	err := db.ReadTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		enc, err := q.GetOpenEncounter(ctx, session.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("find the encounter: %w", err)
		}
		if enc.Status != statusActive {
			return nil
		}
		d, err = loadEncounter(ctx, q, enc, s.d20Rerolls)
		return err
	})
	if err != nil {
		return s.dbError(ctx, "read the encounter", err)
	}
	if d == nil {
		return nil
	}
	enc, err := s.viewFor(ctx, tv, d)
	if err != nil {
		return err
	}
	out := &playv1.TvCombat{Round: enc.GetRound(), MastersTurn: enc.GetMasterTurn()}
	onTurn := map[string]bool{enc.GetCurrentCombatantId(): enc.GetCurrentCombatantId() != ""}
	for _, id := range enc.GetTurnGroupIds() {
		onTurn[id] = true
	}
	for _, c := range enc.GetCombatants() {
		out.Order = append(out.Order, &playv1.TvCombatant{
			Name: c.GetLabel(), Initial: initialOf(c.GetLabel()), State: tvStateOf(c.GetState()),
			OnTurn: onTurn[c.GetId()], IsPlayer: c.GetKind() == playv1.CombatantKind_COMBATANT_KIND_PLAYER,
		})
	}
	if out.Log, err = s.tvLog(ctx, tv, d.enc.ID); err != nil {
		return err
	}
	if d.enc.MapID != nil {
		if out.Map, err = s.tvMap(ctx, tv, *d.enc.MapID); err != nil {
			return err
		}
	}
	stage.Body = &playv1.TableScreenStage_Combat{Combat: out}
	return nil
}

// initialOf is the first letter of a name, upper case.
func initialOf(name string) string {
	r, _ := utf8.DecodeRuneInString(strings.TrimSpace(name))
	if r == utf8.RuneError {
		return ""
	}
	return string(unicode.ToUpper(r))
}

// tvStateOf is the word a player reads of a combatant (RN-20). A player character has a word only when it is
// down, stable or dead; an up one has none, as in the player's own read.
func tvStateOf(state playv1.CombatantState) playv1.TvState {
	switch state {
	case playv1.CombatantState_COMBATANT_STATE_UNHURT:
		return playv1.TvState_TV_STATE_UNHURT
	case playv1.CombatantState_COMBATANT_STATE_HURT:
		return playv1.TvState_TV_STATE_HURT
	case playv1.CombatantState_COMBATANT_STATE_BADLY_HURT:
		return playv1.TvState_TV_STATE_VERY_HURT
	case playv1.CombatantState_COMBATANT_STATE_DEFEATED, playv1.CombatantState_COMBATANT_STATE_DEAD:
		return playv1.TvState_TV_STATE_DEFEATED
	case playv1.CombatantState_COMBATANT_STATE_DOWN, playv1.CombatantState_COMBATANT_STATE_STABLE:
		return playv1.TvState_TV_STATE_FALLEN
	}
	return playv1.TvState_TV_STATE_UNSPECIFIED
}

// tvLog are the lines of the combat's log that every player reads, newest first, at most tvMaxLogLines.
// The entries are the ones the log answers to a player with no character.
// Each becomes a plain sentence; a kind with no sentence is not on the TV.
func (s *Service) tvLog(ctx context.Context, tv authz.Membership, encounterID string) ([]string, error) {
	snap, err := s.readLogSnapshot(ctx, tv, encounterID)
	if err != nil {
		return nil, s.dbError(ctx, "read the combat log", err)
	}
	v, err := s.tvLogViewer(ctx, tv)
	if err != nil {
		return nil, err
	}
	var lines []string
	for _, e := range s.logViews(ctx, tv, &snap, v, "") {
		if line, ok := tvLogLine(e); ok {
			lines = append(lines, line)
			if len(lines) == tvMaxLogLines {
				break
			}
		}
	}
	return lines, nil
}

// tvLogViewer is who reads the log for the TV: a player, who sees the log as every player does, but with the keys
// whole. A player's view blanks the key of an NPC's stat block action, and RN-23 is decided on the key.
func (s *Service) tvLogViewer(ctx context.Context, tv authz.Membership) (combatViewer, error) {
	v, err := s.logViewer(ctx, tv)
	v.wholeKeys = true
	return v, err
}

// tvLogLine is the sentence of one entry a player reads. A name of table content (RN-23) is not told: the
// master may have hidden the entry, so the sentence says only that it was used.
func tvLogLine(e *playv1.CombatLogEntry) (string, bool) {
	actor, target := e.GetActorLabel(), e.GetTargetLabel()
	what := ""
	// A key with "@mesa" anywhere is table content (a spell, or an action of a table feature), which the master may
	// have hidden. The name of a stat-block action of an NPC is the creature's own, as a player reads it.
	if e.GetKeyNamePt() != "" && !rules.IsTableKeyAnywhere(e.GetKey()) {
		what = e.GetKeyNamePt()
	}
	switch e.GetKind() {
	case playv1.CombatLogKind_COMBAT_LOG_KIND_COMBAT_BEGUN:
		return "O combate começou", true
	case playv1.CombatLogKind_COMBAT_LOG_KIND_COMBAT_ENDED:
		return "O combate terminou", true
	case playv1.CombatLogKind_COMBAT_LOG_KIND_DOOR_OPENED:
		return "Uma porta foi aberta", true
	}
	if actor == "" {
		return "", false
	}
	switch e.GetKind() {
	case playv1.CombatLogKind_COMBAT_LOG_KIND_MOVED:
		return actor + " se moveu", true
	case playv1.CombatLogKind_COMBAT_LOG_KIND_ATTACK:
		return tvAttackLine(e, actor, target, what)
	case playv1.CombatLogKind_COMBAT_LOG_KIND_SPELL_CAST:
		if what == "" {
			return actor + " conjurou uma magia", true
		}
		return actor + " conjurou " + what, true
	case playv1.CombatLogKind_COMBAT_LOG_KIND_ACTION:
		if what == "" {
			return actor + " agiu", true
		}
		return actor + " usou " + what, true
	}
	return "", false
}

// tvAttackLine is "Toren atacou Goblin: acertou".
func tvAttackLine(e *playv1.CombatLogEntry, actor, target, what string) (string, bool) {
	if target == "" {
		return "", false
	}
	line := actor + " atacou " + target
	if what != "" {
		line += " com " + what
	}
	switch e.GetOutcome() {
	case playv1.AttackOutcome_ATTACK_OUTCOME_HIT:
		return line + ": acertou", true
	case playv1.AttackOutcome_ATTACK_OUTCOME_CRITICAL_HIT:
		return line + ": acerto crítico", true
	case playv1.AttackOutcome_ATTACK_OUTCOME_MISS:
		return line + ": errou", true
	}
	return line, true
}

// projectRolls sets the free rolls for the whole table of the stage: newest first, at most tvMaxRolls. A
// roll only its roller reads ("Só eu") is not on the list the TV reads from: the SQL leaves it out for any
// reader that is not the master, and the shaping checks it again (freeRollViews).
func (s *Service) projectRolls(ctx context.Context, tv authz.Membership, session playdb.GameSession, stage *playv1.TableScreenStage) error {
	switch stage.GetBody().(type) {
	case *playv1.TableScreenStage_Scene, *playv1.TableScreenStage_Combat, *playv1.TableScreenStage_Map:
	default:
		return nil
	}
	rows, err := s.queries.ListFreeRollEvents(ctx, playdb.ListFreeRollEventsParams{GameSessionID: session.ID, OnlyTable: true, Limit: tvMaxRolls})
	if err != nil {
		return s.dbError(ctx, "list the free rolls", err)
	}
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		if r.ActorUserID != nil {
			ids = append(ids, *r.ActorUserID)
		}
	}
	rolls, err := freeRollViews(rows, s.freeRollNames(ctx, ids), tvReader(tv))
	if err != nil {
		return err
	}
	for _, r := range rolls {
		if len(stage.Rolls) == tvMaxRolls {
			break
		}
		roll := &playv1.TvRoll{Who: r.GetRollerName(), Expression: r.GetFormula(), Total: r.GetTotal()}
		for _, d := range r.GetDice() {
			if !d.GetDiscarded() {
				roll.Faces = append(roll.Faces, d.GetFace())
			}
		}
		stage.Rolls = append(stage.Rolls, roll)
	}
	return nil
}
