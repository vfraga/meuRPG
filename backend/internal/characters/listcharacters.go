package characters

import (
	"context"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The phases of ListCharacters: the rows the caller may list (listParams), what the
// summaries need besides them (readCharacterList), then one summary per row.

// listParams is the rows the caller may list.
func listParams(m authz.Membership) charactersdb.ListCharactersParams {
	params := charactersdb.ListCharactersParams{CampaignID: m.CampaignID}
	if !isMaster(m) {
		params.PlayerUserID = &m.UserID // a player sees only their own
	}
	if m.Pending {
		pending := statusPending
		params.Status = &pending // and a pending member only their pending one (canSee)
	}
	return params
}

// characterList is what the summaries of a list need besides the rows.
type characterList struct {
	m            authz.Membership
	displayNames map[string]string
	content      *rules.Content
	reviews      map[string]string
	links        map[string]charactersdb.ListLatestClaimLinksRow
}

// readCharacterList reads the players' display names, the content, the reviews and,
// for the master, the claim links.
func (s *Service) readCharacterList(ctx context.Context, m authz.Membership, rows []charactersdb.ListCharactersRow) (*characterList, error) {
	var playerIDs []string
	for _, row := range rows {
		if row.PlayerUserID != nil {
			playerIDs = append(playerIDs, *row.PlayerUserID)
		}
	}
	l := &characterList{m: m}
	var err error
	if l.displayNames, err = s.displayNames(ctx, playerIDs); err != nil {
		return nil, s.dbError(ctx, "read display names", err)
	}
	if l.content, err = s.contentFor(ctx, nil, m.CampaignID); err != nil {
		return nil, s.dbError(ctx, "read rules content", err)
	}
	if l.reviews, err = s.reviewStatuses(ctx, m.CampaignID); err != nil {
		return nil, s.dbError(ctx, "list characters", err)
	}
	// Only the master lists the claim links: the reserved characters are theirs alone.
	if isMaster(m) {
		if l.links, err = s.latestClaimLinks(ctx, m.CampaignID); err != nil {
			return nil, s.dbError(ctx, "list the claim links", err)
		}
	}
	return l, nil
}

// summary is the list's line of one row. now is when the list was asked, for the claim
// links' state.
func (l *characterList) summary(row charactersdb.ListCharactersRow, now time.Time) (*charactersv1.CharacterSummary, error) {
	summary := &charactersv1.CharacterSummary{
		Id:                row.ID,
		Kind:              kindFromDB[row.Kind],
		State:             characterState(row.Status, row.SheetLockedAt),
		Name:              row.Name,
		PlayerUserId:      deref(row.PlayerUserID),
		PlayerDisplayName: l.displayNames[deref(row.PlayerUserID)],
		CreatedAt:         timestamppb.New(row.CreatedAt),
		Reserved:          row.Reserved,
	}
	if isMaster(l.m) {
		setClaimState(summary, row.Reserved, row.ClaimedAt, l.links[row.ID], now)
	}
	sheet, err := loadSheet(row.ID, row.Sheet)
	if err != nil {
		return nil, err
	}
	if id := portraitOf(sheet); row.Kind != kindPlayer && id != "" {
		summary.PortraitUrl = "/images/" + id
	}
	// The list holds the master's characters or the caller's own, so whoever reads a
	// pending one may read its review's status (never the reason).
	if row.Kind == kindPlayer && row.Status == statusPending {
		summary.ReviewStatus = reviewStatusToProto(l.reviews[row.ID])
	}
	l.describeSheet(summary, row, sheet.GetFull())
	summary.PendingChoiceCount = pendingChoiceCount(l.content, l.m, row.Kind, row.Status, row.PlayerUserID, sheet)
	return summary, nil
}

// describeSheet adds what a full sheet tells the list: its classes and race, and, for
// the master, the choices an active player's unlocked sheet leaves open.
func (l *characterList) describeSheet(summary *charactersv1.CharacterSummary, row charactersdb.ListCharactersRow, full *charactersv1.FullSheet) {
	if full == nil {
		return
	}
	labels := l.content.Summary(buildOf(full))
	summary.ClassSummary = labels.ClassSummaryPT
	summary.RaceNamePt = labels.RaceNamePT
	if isMaster(l.m) && row.Kind == kindPlayer && row.Status == statusActive && row.SheetLockedAt == nil {
		summary.OpenChoices = openChoices(rules.Derive(buildOf(full), l.content))
	}
}
