package progression

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
	"uuid"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	progressionv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/progression/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/logging"
	"github.com/PuraFome/meuRPG/backend/internal/progression/progressiondb"
)

// UndoLastXPAward implements progressionv1connect.ProgressionServiceHandler.
//
// One transaction takes the latest award that is not undone (locked, so two
// undos take turns), subtracts each share (the XP the sheet gained) from the character's sheet (never
// below 0; a milestone has no XP to take back, its marks go with undone_at),
// marks the award undone and writes the session's event. The award row stays:
// the history is never rewritten (ADR-0007).
func (s *Service) UndoLastXPAward(
	ctx context.Context,
	req *connect.Request[progressionv1.UndoLastXPAwardRequest],
) (*connect.Response[progressionv1.UndoLastXPAwardResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	key, expected, err := checkUndo(req.Msg)
	if err != nil {
		return nil, err
	}
	var out undoneAward
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		out, err = s.undoInTx(ctx, tx, m, key, expected) // a retry starts over
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, "undo an XP award", err)
	}
	s.announceUndo(ctx, m, out)
	views, err := s.awardViews(ctx, m, out.award)
	if err != nil {
		return nil, s.dbError(ctx, "read an undone award", err)
	}
	return connect.NewResponse(&progressionv1.UndoLastXPAwardResponse{Award: views[0]}), nil
}

// undoneAward is what an undo's transaction did: the award, whether it was a
// retry of an undo already made (repeated), whether the open session logged
// it, and the maps whose treasures it freed.
type undoneAward struct {
	award     progressiondb.XpAward
	repeated  bool
	logged    bool
	freedMaps []string
}

// checkUndo checks the undo's key and the award the app expects to be the
// last one (empty for any), and returns them in canonical form.
func checkUndo(msg *progressionv1.UndoLastXPAwardRequest) (key, expected string, err error) {
	k, err := uuid.Parse(msg.GetIdempotencyKey())
	if err != nil {
		return "", "", invalidArgument("idempotency_key", errors.New("must be a UUID"))
	}
	if raw := msg.GetExpectedAwardId(); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			return "", "", invalidArgument("expected_award_id", errors.New("must be a UUID"))
		}
		expected = id.String()
	}
	return k.String(), expected, nil
}

// undoInTx is the undo's transaction: a retry of the key answers the award it
// undid; otherwise the last award comes back off the sheets and the maps, is
// marked undone, and the session gets the event.
func (s *Service) undoInTx(ctx context.Context, tx pgx.Tx, m authz.Membership, key, expected string) (undoneAward, error) {
	q := s.queries.WithTx(tx)
	done, err := q.GetXPAwardByUndoKey(ctx, progressiondb.GetXPAwardByUndoKeyParams{CampaignID: m.CampaignID, UndoKey: key})
	if err == nil {
		return undoneAward{award: done, repeated: true}, nil // a retry of an undo already made
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return undoneAward{}, fmt.Errorf("find an undo by its key: %w", err)
	}
	last, err := lastAward(ctx, q, m.CampaignID, expected)
	if err != nil {
		return undoneAward{}, err
	}
	shares, err := q.ListXPSharesOfAwards(ctx, []string{last.ID})
	if err != nil {
		return undoneAward{}, fmt.Errorf("list the shares: %w", err)
	}
	now := s.now()
	ids, err := s.takeBackShares(ctx, tx, m, last, shares, now)
	if err != nil {
		return undoneAward{}, err
	}
	treasures, freedMaps, err := s.freeTreasures(ctx, tx, q, last)
	if err != nil {
		return undoneAward{}, err
	}
	undone, err := q.MarkXPAwardUndone(ctx, progressiondb.MarkXPAwardUndoneParams{
		CampaignID: m.CampaignID, ID: last.ID, UndoneAt: &now, UndoneBy: m.UserID, UndoKey: key,
	})
	if err != nil {
		return undoneAward{}, fmt.Errorf("mark an award undone: %w", err) // no row: undone meanwhile; the retry finds it
	}
	payload := eventPayload{
		AwardID: last.ID, Mode: last.Mode, TotalXP: last.TotalXp, CharacterIDs: ids, Treasures: treasures,
		EncounterID: deref(last.EncounterID), MilestoneID: deref(last.MilestoneID),
	}
	logged, err := s.appendEvent(ctx, tx, m, eventXPAwardUndone, payload, now)
	return undoneAward{award: undone, logged: logged, freedMaps: freedMaps}, err
}

// lastAward finds and locks the latest award that is not undone (two undos
// take turns), and checks it is the one the app expected, when it said.
func lastAward(ctx context.Context, q *progressiondb.Queries, campaignID, expected string) (progressiondb.XpAward, error) {
	last, err := q.GetLastXPAwardForUpdate(ctx, campaignID)
	if errors.Is(err, pgx.ErrNoRows) {
		return last, errBlocked(progressionv1.XPBlockedReason_XP_BLOCKED_REASON_NOTHING_TO_UNDO, "", 0)
	}
	if err != nil {
		return last, fmt.Errorf("find the last award: %w", err)
	}
	if expected != "" && last.ID != expected {
		return last, connect.NewError(connect.CodeAborted, errors.New("another award is the last now"))
	}
	return last, nil
}

// takeBackShares subtracts each share (the XP the sheet gained) from its
// sheet, never below 0, and returns the characters in the shares' order. A
// milestone has no XP to take back: its marks go with undone_at.
func (s *Service) takeBackShares(ctx context.Context, tx pgx.Tx, m authz.Membership, last progressiondb.XpAward, shares []progressiondb.XpAwardShare, now time.Time) ([]string, error) {
	ids := make([]string, 0, len(shares))
	for _, share := range shares {
		ids = append(ids, share.CharacterID)
		if last.Mode == modeMilestone || share.Xp == 0 {
			continue
		}
		// not_found: the character is no longer a player character; nothing to take back.
		if _, _, err := s.party.AddExperience(ctx, tx, m.CampaignID, share.CharacterID, -share.Xp, now); err != nil && connect.CodeOf(err) != connect.CodeNotFound {
			return nil, fmt.Errorf("take the XP off a sheet: %w", err)
		}
	}
	return ids, nil
}

// freeTreasures frees what a "Voltar à cidade" award converted: found, not
// converted, again. It returns the treasures for the event and the maps whose
// points it freed.
func (s *Service) freeTreasures(ctx context.Context, tx pgx.Tx, q *progressiondb.Queries, last progressiondb.XpAward) ([]eventTreasure, []string, error) {
	if last.Mode != modeGold {
		return nil, nil, nil
	}
	rows, err := q.ListXPAwardTreasures(ctx, []string{last.ID})
	if err != nil {
		return nil, nil, fmt.Errorf("list the converted treasures: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil, nil
	}
	treasures := make([]eventTreasure, 0, len(rows))
	for _, r := range rows {
		treasures = append(treasures, eventTreasure{PointID: r.PointID, ValuePO: r.ValuePo})
	}
	freedMaps, err := s.treasures.Release(ctx, tx, last.ID)
	if err != nil {
		return nil, nil, err
	}
	return treasures, freedMaps, nil
}

// announceUndo runs after the commit of an undo that was not a retry: it
// tells each map whose treasures were freed, and, once the open session
// logged the undo, writes the log line and tells the streams the XP changed.
func (s *Service) announceUndo(ctx context.Context, m authz.Membership, out undoneAward) {
	if out.repeated {
		return
	}
	if len(out.freedMaps) > 0 {
		s.treasures.PublishChanged(m.CampaignID, out.freedMaps) // once per map
	}
	if out.logged {
		logging.Event(ctx, s.logger, "xp.undone", slog.String("award_id", out.award.ID))
		s.log.PublishXPChanged(m.CampaignID)
	}
}
