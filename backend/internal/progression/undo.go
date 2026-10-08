package progression

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
	key, err := uuid.Parse(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, invalidArgument("idempotency_key", errors.New("must be a UUID"))
	}
	expected := ""
	if raw := req.Msg.GetExpectedAwardId(); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			return nil, invalidArgument("expected_award_id", errors.New("must be a UUID"))
		}
		expected = id.String()
	}

	var undone progressiondb.XpAward
	var repeated, logged bool
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		undone, repeated, logged = progressiondb.XpAward{}, false, false // a retry starts over
		q := s.queries.WithTx(tx)
		if done, err := q.GetXPAwardByUndoKey(ctx, progressiondb.GetXPAwardByUndoKeyParams{CampaignID: m.CampaignID, UndoKey: key.String()}); err == nil {
			undone, repeated = done, true // a retry of an undo already made
			return nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("find an undo by its key: %w", err)
		}
		last, err := q.GetLastXPAwardForUpdate(ctx, m.CampaignID)
		if errors.Is(err, pgx.ErrNoRows) {
			return errBlocked(progressionv1.XPBlockedReason_XP_BLOCKED_REASON_NOTHING_TO_UNDO, "", 0)
		}
		if err != nil {
			return fmt.Errorf("find the last award: %w", err)
		}
		if expected != "" && last.ID != expected {
			return connect.NewError(connect.CodeAborted, errors.New("another award is the last now"))
		}
		shares, err := q.ListXPSharesOfAwards(ctx, []string{last.ID})
		if err != nil {
			return fmt.Errorf("list the shares: %w", err)
		}
		now := s.now()
		ids := make([]string, 0, len(shares))
		for _, share := range shares {
			ids = append(ids, share.CharacterID)
			if last.Mode == modeMilestone || share.Xp == 0 {
				continue
			}
			if _, _, err := s.party.AddExperience(ctx, tx, m.CampaignID, share.CharacterID, -share.Xp, now); err != nil && connect.CodeOf(err) != connect.CodeNotFound {
				return fmt.Errorf("take the XP off a sheet: %w", err)
			} // not_found: the character is no longer a player character; nothing to take back
		}
		// A "Voltar à cidade" award frees its treasures: found, not converted again.
		var treasures []eventTreasure
		if last.Mode == modeGold {
			rows, err := q.ListXPAwardTreasures(ctx, []string{last.ID})
			if err != nil {
				return fmt.Errorf("list the converted treasures: %w", err)
			}
			for _, r := range rows {
				treasures = append(treasures, eventTreasure{PointID: r.PointID, ValuePO: r.ValuePo})
			}
			if len(rows) > 0 {
				if err := s.treasures.Release(ctx, tx, last.ID); err != nil {
					return err
				}
			}
		}
		if undone, err = q.MarkXPAwardUndone(ctx, progressiondb.MarkXPAwardUndoneParams{
			CampaignID: m.CampaignID, ID: last.ID, UndoneAt: &now, UndoneBy: m.UserID, UndoKey: key.String(),
		}); err != nil {
			return fmt.Errorf("mark an award undone: %w", err) // no row: undone meanwhile; the retry finds it
		}
		payload := eventPayload{AwardID: last.ID, Mode: last.Mode, TotalXP: last.TotalXp, CharacterIDs: ids, Treasures: treasures}
		if last.EncounterID != nil {
			payload.EncounterID = *last.EncounterID
		}
		payload.MilestoneID = deref(last.MilestoneID)
		logged, err = s.appendEvent(ctx, tx, m, eventXPAwardUndone, payload, now)
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, "undo an XP award", err)
	}
	if logged && !repeated {
		logging.Event(ctx, s.logger, "xp.undone", slog.String("award_id", undone.ID))
		s.log.PublishXPChanged(m.CampaignID)
	}
	views, err := s.awardViews(ctx, m, undone)
	if err != nil {
		return nil, s.dbError(ctx, "read an undone award", err)
	}
	return connect.NewResponse(&progressionv1.UndoLastXPAwardResponse{Award: views[0]}), nil
}
