package play

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/live"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/progression/link"
)

// What the progression module (MR-016, Etapa 7) needs from play: a combat's
// defeated NPCs and their XP, the session's log and its live stream. Package
// progression declares the interface (progression.Combats and
// progression.SessionLog); this Service implements it, as it does the
// characters' vitals the other way round, and cmd/api connects them. The
// methods take no caller: they run after progression's own authorization
// check.

// appendableKinds are the session event kinds another module may append
// through AppendEvent: progression's XP awards and milestones, and the maps
// module's clue reveals, trap reveals and treasure found or unmarked, and the
// characters module's creatures given, dismissed or defeated outside a combat
// (MR-037). A kind that is not one of them is a bug in the caller, never a write.
var appendableKinds = []string{
	eventXPAwarded, eventXPAwardUndone, eventMilestoneMarked, eventClueRevealed,
	eventTrapRevealed, eventTrapNoticed, eventTrapDisarmed, eventTreasureFound, eventTreasureUnfound,
	eventCreatureSummoned, eventCreatureDismissed,
	eventItemGiven, eventItemTransferred, eventItemUsed, eventItemAttuned, eventItemIdentified, eventItemCharges,
}

// CampaignEncounter returns what an enemies award needs from the campaign's
// combat: its name, whether it ended and the XP its defeated NPCs give.
// `not_found` for a combat that is not the campaign's.
func (s *Service) CampaignEncounter(ctx context.Context, tx pgx.Tx, campaignID, encounterID string) (link.Encounter, error) {
	row, err := s.queriesIn(tx).GetCampaignEncounterXP(ctx, playdb.GetCampaignEncounterXPParams{CampaignID: campaignID, ID: encounterID})
	if errors.Is(err, pgx.ErrNoRows) {
		return link.Encounter{}, connect.NewError(connect.CodeNotFound, errors.New("encounter not found"))
	}
	if err != nil {
		return link.Encounter{}, fmt.Errorf("read the encounter's XP: %w", err)
	}
	return link.Encounter{Name: row.Name, Ended: row.Status == statusEnded, XP: clamp32(int(row.Xp), 0, 1_000_000)}, nil
}

// EncounterNames returns the names of those of ids that are combats of the
// campaign, by ID.
func (s *Service) EncounterNames(ctx context.Context, campaignID string, ids []string) (map[string]string, error) {
	out := make(map[string]string, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.queries.ListCampaignEncounterNames(ctx, playdb.ListCampaignEncounterNamesParams{CampaignID: campaignID, Ids: ids})
	if err != nil {
		return nil, fmt.Errorf("read the encounters' names: %w", err)
	}
	for _, row := range rows {
		out[row.ID] = row.Name
	}
	return out, nil
}

// AppendEvent appends an event to the history of the campaign's open session
// inside tx, and says whether there was one: with no open session the XP is
// still given (or the clue revealed) and nothing is written here. It locks the session's row first,
// like every other writer, so the events get their numbers in order. The
// payload carries IDs and numbers only (docs/privacy.md).
func (s *Service) AppendEvent(ctx context.Context, tx pgx.Tx, campaignID, kind, actorUserID string, payload []byte, at time.Time) (bool, error) {
	if !slices.Contains(appendableKinds, kind) {
		return false, fmt.Errorf("play: %q is not a kind another module may write", kind)
	}
	q := s.queries.WithTx(tx)
	session, err := q.GetOpenGameSessionForUpdate(ctx, campaignID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("lock the open session: %w", err)
	}
	seq, err := q.NextSessionEventSeq(ctx, session.ID)
	if err != nil {
		return false, fmt.Errorf("next event number: %w", err)
	}
	if _, err := q.InsertSessionEvent(ctx, playdb.InsertSessionEventParams{
		GameSessionID: session.ID, Seq: seq, Kind: kind, ActorUserID: &actorUserID, Payload: payload, CreatedAt: at,
	}); err != nil {
		return false, fmt.Errorf("insert session event: %w", err)
	}
	return true, nil
}

// PublishXPChanged tells every stream of the campaign that the XP changed. It
// carries no content: the app reads it again, where it is allowed (RN-20).
// Call it after the commit.
func (s *Service) PublishXPChanged(campaignID string) {
	s.hub.Publish(campaignID, live.Event{
		Audience: live.Audience{Everyone: true},
		Message:  &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_XpChanged_{XpChanged: &playv1.WatchGameSessionResponse_XpChanged{}}},
	})
}
