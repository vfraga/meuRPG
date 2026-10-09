package progression

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"
	"uuid"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/timestamppb"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	progressionv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/progression/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/platform/names"
	"github.com/PuraFome/meuRPG/backend/internal/progression/progressiondb"
)

// The planned milestones (MR-016, RN-09, RN-12, question 45): in a MILESTONES
// campaign the master writes the milestones ahead of time (planned_milestones)
// and marks one reached when the party gets there.
//
//   - Reaching one is today's MarkMilestone with the milestone's text as its
//     reason, linked to the milestone (xp_awards.milestone_id): the same
//     award, the same "pode subir de nível" tag, the same undo.
//   - "Reached" is derived: at least one of the milestone's awards is not
//     undone. Undoing its only mark makes it planned again; a "Dar a mais
//     alguém" mark is one more award on the same milestone, so undoing it
//     (it is the last, as undo only takes the last) takes the milestone off
//     those characters and leaves it reached for the others.
//   - A reached milestone is never edited, moved or removed.
//   - A player reads only the reached ones (RN-20: the master's plans stay
//     the master's). Planned-list edits have no session event: the players
//     never see the list.

// The limits of the list, as progression.proto documents them. A milestone's
// text is as long as an award's reason, because it becomes one.
const (
	maxMilestones    = 100
	maxMilestoneText = maxReasonLength
)

// errMilestoneNotFound is the answer for an ID that is not a milestone of the
// campaign, valid or not.
func errMilestoneNotFound() error {
	return connect.NewError(connect.CodeNotFound, errors.New("milestone not found"))
}

// ListMilestones implements progressionv1connect.ProgressionServiceHandler.
func (s *Service) ListMilestones(
	ctx context.Context,
	req *connect.Request[progressionv1.ListMilestonesRequest],
) (*connect.Response[progressionv1.ListMilestonesResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	views, err := s.milestoneViews(ctx, m)
	if err != nil {
		return nil, s.dbError(ctx, "list milestones", err)
	}
	return connect.NewResponse(&progressionv1.ListMilestonesResponse{Milestones: views}), nil
}

// AddMilestone implements progressionv1connect.ProgressionServiceHandler.
func (s *Service) AddMilestone(
	ctx context.Context,
	req *connect.Request[progressionv1.AddMilestoneRequest],
) (*connect.Response[progressionv1.AddMilestoneResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	text, err := cleanMilestoneText(req.Msg.GetText())
	if err != nil {
		return nil, err
	}
	if err := s.requireMilestonesMode(ctx, nil, m.CampaignID); err != nil {
		return nil, err
	}
	key, err := idem.Clean(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	// The key is unique in the campaign, and kept with a hash of the whole request.
	scopedKey, requestHash := idem.Scope(m.CampaignID, key), idem.Hash(req.Msg)
	var added progressiondb.PlannedMilestone
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		if err := s.requireMilestonesMode(ctx, tx, m.CampaignID); err != nil {
			return err
		}
		// The list is locked first, so two calls with the same key take turns.
		current, err := q.ListPlannedMilestonesForUpdate(ctx, m.CampaignID)
		if err != nil {
			return fmt.Errorf("list the milestones: %w", err)
		}
		added, _, err = idem.Create(ctx, scopedKey, requestHash, q.GetPlannedMilestoneByCreateKey,
			func(p progressiondb.PlannedMilestone) *string { return p.CreateHash },
			func() (progressiondb.PlannedMilestone, error) {
				if len(current) >= maxMilestones {
					return added, connect.NewError(connect.CodeResourceExhausted, fmt.Errorf("the campaign already has %d milestones", maxMilestones))
				}
				position := int32(0)
				if n := len(current); n > 0 {
					position = current[n-1].Position + 1
				}
				p, err := q.InsertPlannedMilestone(ctx, progressiondb.InsertPlannedMilestoneParams{
					CampaignID: m.CampaignID, Position: position, Text: text, CreateKey: scopedKey, CreateHash: requestHash, Now: s.now(),
				})
				if err != nil && !errors.Is(err, pgx.ErrNoRows) {
					return p, fmt.Errorf("insert a milestone: %w", err)
				}
				return p, err
			})
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, "add a milestone", err)
	}
	views, err := s.milestoneViews(ctx, m)
	if err != nil {
		return nil, s.dbError(ctx, "read the milestones", err)
	}
	return connect.NewResponse(&progressionv1.AddMilestoneResponse{Milestone: milestoneByID(views, added.ID), Milestones: views}), nil
}

// UpdateMilestone implements progressionv1connect.ProgressionServiceHandler.
func (s *Service) UpdateMilestone(
	ctx context.Context,
	req *connect.Request[progressionv1.UpdateMilestoneRequest],
) (*connect.Response[progressionv1.UpdateMilestoneResponse], error) {
	m, id, err := s.milestoneCall(ctx, req.Msg.GetCampaignId(), req.Msg.GetMilestoneId())
	if err != nil {
		return nil, err
	}
	text, err := cleanMilestoneText(req.Msg.GetText())
	if err != nil {
		return nil, err
	}
	err = s.changePlanned(ctx, m, id, func(q *progressiondb.Queries) error {
		return q.UpdatePlannedMilestoneText(ctx, progressiondb.UpdatePlannedMilestoneTextParams{CampaignID: m.CampaignID, ID: id, Text: text, Now: s.now()})
	})
	if err != nil {
		return nil, s.dbError(ctx, "update a milestone", err)
	}
	views, err := s.milestoneViews(ctx, m)
	if err != nil {
		return nil, s.dbError(ctx, "read the milestones", err)
	}
	return connect.NewResponse(&progressionv1.UpdateMilestoneResponse{Milestone: milestoneByID(views, id), Milestones: views}), nil
}

// MoveMilestone implements progressionv1connect.ProgressionServiceHandler.
func (s *Service) MoveMilestone(
	ctx context.Context,
	req *connect.Request[progressionv1.MoveMilestoneRequest],
) (*connect.Response[progressionv1.MoveMilestoneResponse], error) {
	m, id, err := s.milestoneCall(ctx, req.Msg.GetCampaignId(), req.Msg.GetMilestoneId())
	if err != nil {
		return nil, err
	}
	var step int
	switch req.Msg.GetDirection() {
	case progressionv1.MilestoneDirection_MILESTONE_DIRECTION_UP:
		step = -1
	case progressionv1.MilestoneDirection_MILESTONE_DIRECTION_DOWN:
		step = 1
	default:
		return nil, invalidArgument("direction", errors.New("is required"))
	}
	err = s.changePlanned(ctx, m, id, func(q *progressiondb.Queries) error {
		list, err := q.ListPlannedMilestonesForUpdate(ctx, m.CampaignID)
		if err != nil {
			return fmt.Errorf("list the milestones: %w", err)
		}
		live, err := q.ListLiveMilestoneAwards(ctx, m.CampaignID)
		if err != nil {
			return fmt.Errorf("list the marks: %w", err)
		}
		reached := func(i int) bool {
			return slices.ContainsFunc(live, func(a progressiondb.XpAward) bool { return deref(a.MilestoneID) == list[i].ID })
		}
		i := slices.IndexFunc(list, func(p progressiondb.PlannedMilestone) bool { return p.ID == id })
		if i < 0 {
			return errMilestoneNotFound()
		}
		// The nearest planned one in that direction: the reached ones are
		// skipped, and keep their places.
		j := i + step
		for j >= 0 && j < len(list) && reached(j) {
			j += step
		}
		if j < 0 || j >= len(list) {
			return nil // already first or last: nothing changes
		}
		list[i], list[j] = list[j], list[i]
		for p := range list {
			want := int32(p)
			if list[p].Position == want {
				continue
			}
			if err := q.SetPlannedMilestonePosition(ctx, progressiondb.SetPlannedMilestonePositionParams{CampaignID: m.CampaignID, ID: list[p].ID, Position: want}); err != nil {
				return fmt.Errorf("set a milestone's position: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "move a milestone", err)
	}
	views, err := s.milestoneViews(ctx, m)
	if err != nil {
		return nil, s.dbError(ctx, "read the milestones", err)
	}
	return connect.NewResponse(&progressionv1.MoveMilestoneResponse{Milestones: views}), nil
}

// RemoveMilestone implements progressionv1connect.ProgressionServiceHandler.
func (s *Service) RemoveMilestone(
	ctx context.Context,
	req *connect.Request[progressionv1.RemoveMilestoneRequest],
) (*connect.Response[progressionv1.RemoveMilestoneResponse], error) {
	m, id, err := s.milestoneCall(ctx, req.Msg.GetCampaignId(), req.Msg.GetMilestoneId())
	if err != nil {
		return nil, err
	}
	err = s.changePlanned(ctx, m, id, func(q *progressiondb.Queries) error {
		// Awards are never rewritten (ADR-0007) and deleting the milestone would
		// clear their milestone_id: one that was reached before, and undone, stays.
		if had, err := q.HasMilestoneAwards(ctx, progressiondb.HasMilestoneAwardsParams{CampaignID: m.CampaignID, MilestoneID: id}); err != nil {
			return fmt.Errorf("check the milestone's history: %w", err)
		} else if had {
			return errBlocked(progressionv1.XPBlockedReason_XP_BLOCKED_REASON_MILESTONE_HAS_HISTORY, "", 0)
		}
		return q.DeletePlannedMilestone(ctx, progressiondb.DeletePlannedMilestoneParams{CampaignID: m.CampaignID, ID: id})
	})
	if err != nil {
		return nil, s.dbError(ctx, "remove a milestone", err)
	}
	views, err := s.milestoneViews(ctx, m)
	if err != nil {
		return nil, s.dbError(ctx, "read the milestones", err)
	}
	return connect.NewResponse(&progressionv1.RemoveMilestoneResponse{Milestones: views}), nil
}

// MarkMilestoneReached implements progressionv1connect.ProgressionServiceHandler.
func (s *Service) MarkMilestoneReached(
	ctx context.Context,
	req *connect.Request[progressionv1.MarkMilestoneReachedRequest],
) (*connect.Response[progressionv1.MarkMilestoneReachedResponse], error) {
	award, view, err := s.giveMilestone(ctx, req.Msg.GetCampaignId(), req.Msg.GetMilestoneId(), req.Msg.GetIdempotencyKey(), req.Msg.GetCharacterIds(), false)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&progressionv1.MarkMilestoneReachedResponse{Milestone: view, Award: award}), nil
}

// GiveMilestoneTo implements progressionv1connect.ProgressionServiceHandler.
func (s *Service) GiveMilestoneTo(
	ctx context.Context,
	req *connect.Request[progressionv1.GiveMilestoneToRequest],
) (*connect.Response[progressionv1.GiveMilestoneToResponse], error) {
	award, view, err := s.giveMilestone(ctx, req.Msg.GetCampaignId(), req.Msg.GetMilestoneId(), req.Msg.GetIdempotencyKey(), req.Msg.GetCharacterIds(), true)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&progressionv1.GiveMilestoneToResponse{Milestone: view, Award: award}), nil
}

// giveMilestone is MarkMilestoneReached and GiveMilestoneTo: the same award as
// MarkMilestone, linked to the planned milestone, with its text as the reason
// (checkMilestone, inside give's transaction, finds it and checks the
// milestone is in the state the call needs).
func (s *Service) giveMilestone(ctx context.Context, campaignID, milestoneID, key string, characterIDs []string, again bool) (*progressionv1.XPAward, *progressionv1.Milestone, error) {
	m, id, err := s.milestoneCall(ctx, campaignID, milestoneID)
	if err != nil {
		return nil, nil, err
	}
	g := grant{mode: modeMilestone, milestoneID: id, again: again}
	if g.key, g.characters, err = checkTargets(key, characterIDs); err != nil {
		return nil, nil, err
	}
	award, err := s.give(ctx, m, g)
	if err != nil {
		return nil, nil, err
	}
	views, err := s.milestoneViews(ctx, m)
	if err != nil {
		return nil, nil, s.dbError(ctx, "read the milestones", err)
	}
	view := milestoneByID(views, id)
	for _, mark := range view.GetMarks() {
		if mark.GetId() == award.ID {
			return mark, view, nil
		}
	}
	// The mark was undone since (a retry of a request whose award is undone):
	// still answer with it, as the history shows it.
	marks, err := s.awardViews(ctx, m, award)
	if err != nil {
		return nil, nil, s.dbError(ctx, "read a milestone's mark", err)
	}
	return marks[0], view, nil
}

// checkMilestone runs inside the transaction of a milestone award: it locks
// the planned milestone (so two marks of it take turns) and checks it is in
// the state the call needs. It returns the milestone's text, the award's
// reason.
func (s *Service) checkMilestone(ctx context.Context, q *progressiondb.Queries, campaignID string, g grant) (string, error) {
	ms, err := q.GetPlannedMilestoneForUpdate(ctx, progressiondb.GetPlannedMilestoneForUpdateParams{CampaignID: campaignID, ID: g.milestoneID})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", errMilestoneNotFound()
	}
	if err != nil {
		return "", fmt.Errorf("find the milestone: %w", err)
	}
	live, err := q.ListLiveMilestoneAwardsOf(ctx, progressiondb.ListLiveMilestoneAwardsOfParams{CampaignID: campaignID, MilestoneID: g.milestoneID})
	if err != nil {
		return "", fmt.Errorf("list the milestone's marks: %w", err)
	}
	switch {
	case !g.again && len(live) > 0:
		return "", errBlocked(progressionv1.XPBlockedReason_XP_BLOCKED_REASON_MILESTONE_ALREADY_REACHED, "", 0)
	case g.again && len(live) == 0:
		return "", errBlocked(progressionv1.XPBlockedReason_XP_BLOCKED_REASON_MILESTONE_NOT_REACHED, "", 0)
	case g.again:
		marked, err := q.ListMilestoneMarkedCharacters(ctx, progressiondb.ListMilestoneMarkedCharactersParams{CampaignID: campaignID, MilestoneID: g.milestoneID})
		if err != nil {
			return "", fmt.Errorf("list who has the milestone: %w", err)
		}
		for _, id := range g.characters {
			if slices.Contains(marked, id) {
				return "", errBlocked(progressionv1.XPBlockedReason_XP_BLOCKED_REASON_CHARACTER_ALREADY_MARKED, id, 0)
			}
		}
	}
	return ms.Text, nil
}

// milestoneCall is what the calls on one milestone start with: the caller is
// the campaign's master, the campaign counts milestones (MODE_NOT_ALLOWED
// otherwise), and the milestone ID is a UUID (any other is a
// milestone that does not exist).
func (s *Service) milestoneCall(ctx context.Context, campaignID, milestoneID string) (authz.Membership, string, error) {
	m, err := authz.RequireCampaignRole(ctx, campaignID, authz.RoleMaster)
	if err != nil {
		return m, "", err
	}
	if err := s.requireMilestonesMode(ctx, nil, m.CampaignID); err != nil {
		return m, "", err
	}
	id, err := uuid.Parse(milestoneID)
	if err != nil {
		return m, "", errMilestoneNotFound()
	}
	return m, id.String(), nil
}

// changePlanned runs change inside a transaction that has locked the
// milestone, once it is known to exist and to be still planned.
func (s *Service) changePlanned(ctx context.Context, m authz.Membership, id string, change func(q *progressiondb.Queries) error) error {
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		if err := s.requireMilestonesMode(ctx, tx, m.CampaignID); err != nil {
			return err
		}
		if _, err := q.GetPlannedMilestoneForUpdate(ctx, progressiondb.GetPlannedMilestoneForUpdateParams{CampaignID: m.CampaignID, ID: id}); errors.Is(err, pgx.ErrNoRows) {
			return errMilestoneNotFound()
		} else if err != nil {
			return fmt.Errorf("find the milestone: %w", err)
		}
		live, err := q.ListLiveMilestoneAwardsOf(ctx, progressiondb.ListLiveMilestoneAwardsOfParams{CampaignID: m.CampaignID, MilestoneID: id})
		if err != nil {
			return fmt.Errorf("list the milestone's marks: %w", err)
		}
		if len(live) > 0 {
			return errBlocked(progressionv1.XPBlockedReason_XP_BLOCKED_REASON_MILESTONE_ALREADY_REACHED, "", 0)
		}
		return change(q)
	})
}

// requireMilestonesMode refuses, with MODE_NOT_ALLOWED, a campaign that
// counts XP: it has no milestones. A write asks it again inside its transaction
// (tx), so that a change of the XP mode (RN-09) and the write take turns.
func (s *Service) requireMilestonesMode(ctx context.Context, tx pgx.Tx, campaignID string) error {
	mode, err := s.campaigns.CampaignXPMode(ctx, tx, campaignID)
	if err != nil {
		return s.dbError(ctx, "read the campaign's XP mode", err)
	}
	if mode != campaignsv1.XpMode_XP_MODE_MILESTONES {
		return errBlocked(progressionv1.XPBlockedReason_XP_BLOCKED_REASON_MODE_NOT_ALLOWED, "", mode)
	}
	return nil
}

// cleanMilestoneText checks a milestone's text: one line, 1 to 120
// characters.
func cleanMilestoneText(text string) (string, error) {
	text, err := names.Clean(text, maxMilestoneText)
	if err != nil {
		return "", invalidArgument("text", err)
	}
	return text, nil
}

// milestoneViews builds the campaign's milestones as the caller may read
// them, in the master's order: all of them for the master, the reached ones
// for a player; then the ones marked off the list, oldest first. The marks are
// the awards that are not undone, as the history shows them.
func (s *Service) milestoneViews(ctx context.Context, m authz.Membership) ([]*progressionv1.Milestone, error) {
	// The planned milestones, the marks and what the marks show are one moment:
	// an undo between them would show a mark that is live next to a "Desfazer" on
	// another one.
	var rows []progressiondb.PlannedMilestone
	var live []progressiondb.XpAward
	var withHistory []string
	var data awardData
	err := db.ReadTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		var err error
		if rows, err = q.ListPlannedMilestones(ctx, m.CampaignID); err != nil {
			return fmt.Errorf("list the milestones: %w", err)
		}
		if live, err = q.ListLiveMilestoneAwards(ctx, m.CampaignID); err != nil {
			return fmt.Errorf("list the marks: %w", err)
		}
		if withHistory, err = q.ListMilestoneIDsWithAwards(ctx, m.CampaignID); err != nil {
			return fmt.Errorf("list the milestones with history: %w", err)
		}
		if len(live) > 0 {
			data, err = s.loadAwardData(ctx, q, m, live)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	var marks []*progressionv1.XPAward
	if len(live) > 0 {
		if marks, err = s.viewsOf(ctx, m, data, live); err != nil {
			return nil, err
		}
	}
	byMilestone := map[string][]*progressionv1.XPAward{}
	reachedAt := map[string]time.Time{}
	var offList []*progressionv1.Milestone // marked off the list: one milestone per award
	for i, a := range live {
		id := deref(a.MilestoneID)
		if id == "" {
			offList = append(offList, &progressionv1.Milestone{
				Id: a.ID, Text: a.Reason, Reached: true, ReachedAt: timestamppb.New(a.CreatedAt), Marks: []*progressionv1.XPAward{marks[i]}, OffList: true,
			})
			continue
		}
		byMilestone[id] = append(byMilestone[id], marks[i])
		if _, ok := reachedAt[id]; !ok {
			reachedAt[id] = a.CreatedAt // oldest first
		}
	}
	out := make([]*progressionv1.Milestone, 0, len(rows))
	for _, r := range rows {
		v := &progressionv1.Milestone{Id: r.ID, Text: r.Text, Reached: len(byMilestone[r.ID]) > 0, Marks: byMilestone[r.ID], HasHistory: slices.Contains(withHistory, r.ID)}
		if v.Reached {
			v.ReachedAt = timestamppb.New(reachedAt[r.ID])
		} else if m.Role != authz.RoleMaster {
			continue // a player never learns what is still planned
		}
		out = append(out, v)
	}
	return append(out, offList...), nil
}

// milestoneByID finds a milestone in a list of views.
func milestoneByID(views []*progressionv1.Milestone, id string) *progressionv1.Milestone {
	for _, v := range views {
		if v.GetId() == id {
			return v
		}
	}
	return nil
}
