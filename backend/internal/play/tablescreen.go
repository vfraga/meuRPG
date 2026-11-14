package play

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/play/live"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// The table screen: the TV at the table shows what the master chose. The choice is three
// columns of the open session (tv_mirror, tv_pin, tv_choice_revision); what the TV shows now is decided
// by resolveStage from facts read for the TV, never from the master's own reads.

// errStaleTableScreen is the answer to a choice written over one that changed since it was read.
func errStaleTableScreen() error {
	return connect.NewError(connect.CodeAborted, errors.New("the table screen's choice changed since you read it: read it again and try again"))
}

// tvChoiceOf is the master's stored choice.
func tvChoiceOf(session playdb.GameSession) tvChoice {
	return tvChoice{Mirror: session.TvMirror, Pin: playv1.TableScreenKind(session.TvPin)}
}

// GetTableScreen implements playv1connect.TableScreenServiceHandler.
func (s *Service) GetTableScreen(
	ctx context.Context,
	req *connect.Request[playv1.GetTableScreenRequest],
) (*connect.Response[playv1.GetTableScreenResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	var (
		session playdb.GameSession
		facts   tvFacts
		name    string
		party   []string
	)
	err = db.ReadTx(ctx, s.pool, func(tx pgx.Tx) error { // one snapshot for the facts of the response
		var err error
		if session, err = openSessionWith(ctx, s.queriesIn(tx), m.CampaignID); err != nil {
			return err
		}
		if facts, err = s.tvFactsOf(ctx, tx, session); err != nil {
			return err
		}
		if name, err = s.campaigns.CampaignName(ctx, tx, m.CampaignID); err != nil {
			return err
		}
		party, err = s.partyNames(ctx, tx, m.CampaignID)
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, "read the table screen", err)
	}
	choice := tvChoiceOf(session)
	pick := resolveStage(choice, facts)
	stage := &playv1.TableScreenStage{Kind: pick.Kind}
	if err := s.projectStageBody(ctx, session, stage); err != nil {
		return nil, err
	}
	notice := pick.Notice
	if stage.Kind != pick.Kind && notice == playv1.TableScreenNotice_TABLE_SCREEN_NOTICE_UNSPECIFIED && !choice.Mirror {
		notice = playv1.TableScreenNotice_TABLE_SCREEN_NOTICE_PIN_ENDED // the pinned item ended since the facts were read
	}
	return connect.NewResponse(&playv1.GetTableScreenResponse{
		ChoiceRevision: session.TvChoiceRevision,
		Choice:         &playv1.TableScreenChoice{Mirror: choice.Mirror, Pin: choice.Pin},
		Stage:          stage,
		Notice:         notice,
		CampaignName:   name,
		SessionNumber:  session.SessionNumber,
		Party:          party,
		Options:        tvOptions(facts),
		Presence:       s.screens.snapshot(m.CampaignID, session.ID),
	}), nil
}

// partyNames are the names of the living player characters: nobody dead, no NPC, no character waiting
// for the master's approval (CombatParty lists only the living, active ones).
func (s *Service) partyNames(ctx context.Context, tx pgx.Tx, campaignID string) ([]string, error) {
	party, err := s.roster.CombatParty(ctx, tx, campaignID)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(party))
	for _, c := range party {
		if c.Player && !c.Reserved {
			names = append(names, c.Name)
		}
	}
	return names, nil
}

// tvFactsOf reads what resolveStage needs of the open session, inside tx.
func (s *Service) tvFactsOf(ctx context.Context, tx pgx.Tx, session playdb.GameSession) (tvFacts, error) {
	var f tvFacts
	q := s.queriesIn(tx)
	if session.ShownImageID != nil {
		v, err := shownViewersOf(ctx, q, session)
		if err != nil {
			return f, err
		}
		f.ImageToTable = v.all
	}
	enc, err := q.GetOpenEncounter(ctx, session.ID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return f, err
	}
	f.CombatActive = err == nil && enc.Status == statusActive
	if session.OpenScenePointID != nil {
		// Only whether the point is still a scene: ScenePoint also loads the master's hooks and clues, which are
		// dropped here and never reach the TV.
		_, err := s.maps.ScenePoint(ctx, tx, session.CampaignID, *session.OpenScenePointID)
		if err != nil && connect.CodeOf(err) != connect.CodeNotFound { // not found: the point stopped being a scene
			return f, err
		}
		f.SceneOpen = err == nil
		f.SceneForAll = session.OpenSceneEveryone
	}
	if session.CurrentMapID != nil {
		fog, err := s.maps.MapFog(ctx, tx, session.CampaignID, *session.CurrentMapID)
		if err != nil && connect.CodeOf(err) != connect.CodeNotFound { // not found: the map is gone
			return f, err
		}
		f.HasMap = err == nil
		f.FogOn, f.GroupVision = fog.Fog, fog.GroupVision
	}
	return f, nil
}

// SetTableScreen implements playv1connect.TableScreenServiceHandler.
//
// One transaction (Service.tx) locks the open session's row, checks the revision of the choice, writes the three
// columns and bumps the revision. It writes nothing else. After the commit the master's streams
// hear table_screen_changed; no player's stream does (RN-10).
func (s *Service) SetTableScreen(
	ctx context.Context,
	req *connect.Request[playv1.SetTableScreenRequest],
) (*connect.Response[playv1.SetTableScreenResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	want := req.Msg.GetChoice()
	if want == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("choice is required"))
	}
	pin := want.GetPin()
	if _, known := playv1.TableScreenKind_name[int32(pin)]; !known {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("pin is not a known item"))
	}
	if !want.GetMirror() && !validTvPin(pin) {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("pin must be the idle screen, the map, the image, the scene or the combat when the TV does not mirror"))
	}
	var updated playdb.GameSession
	err = s.txOpen(ctx, m.CampaignID, func(tx pgx.Tx, h *hints, session playdb.GameSession) error {
		q := s.queries.WithTx(tx)
		if session.TvChoiceRevision != req.Msg.GetChoiceRevision() {
			return errStaleTableScreen()
		}
		keep := session.TvPin // a mirror choice with no pin keeps the pin it had
		if column, ok := tvPinColumn[pin]; ok {
			keep = column
		}
		var err error
		if updated, err = q.SetTableScreenChoice(ctx, playdb.SetTableScreenChoiceParams{ID: session.ID, TvMirror: want.GetMirror(), TvPin: keep}); err != nil {
			return err
		}
		h.tell(m.CampaignID, live.Event{
			Audience: live.Audience{Master: true},
			Message: &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_TableScreenChanged_{
				TableScreenChanged: &playv1.WatchGameSessionResponse_TableScreenChanged{ChoiceRevision: updated.TvChoiceRevision},
			}},
		})
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "set the table screen", err)
	}
	choice := tvChoiceOf(updated)
	return connect.NewResponse(&playv1.SetTableScreenResponse{
		ChoiceRevision: updated.TvChoiceRevision,
		Choice:         &playv1.TableScreenChoice{Mirror: choice.Mirror, Pin: choice.Pin},
	}), nil
}
