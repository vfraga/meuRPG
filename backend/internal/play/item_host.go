package play

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/live"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// The session event kinds of the inventory (migration 00196): package characters writes
// them through AppendEvent, and ItemEvents reads them back for the master's item log.
// session_event_kinds lists them, and TestSessionEventKindsMatchTheTable keeps the two
// in step.
const (
	eventItemGiven       = "item_given"
	eventItemTransferred = "item_transferred"
	eventItemUsed        = "item_used"
	eventItemAttuned     = "item_attuned"
	eventItemIdentified  = "item_identified"
	eventItemCharges     = "item_charges"
)

// CampaignInCombat says whether the campaign has a combat that is not ended, inside tx
// (characters.CreatureHost): attuning to an item and changing body armor wait for a
// moment outside one (SRD 5.1 "Attunement" takes a short rest).
func (s *Service) CampaignInCombat(ctx context.Context, tx pgx.Tx, campaignID string) (bool, error) {
	q := s.queries.WithTx(tx)
	session, err := q.GetOpenGameSession(ctx, campaignID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("find the open session: %w", err)
	}
	if _, err := q.GetOpenEncounter(ctx, session.ID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("find the open encounter: %w", err)
	}
	return true, nil
}

// PublishInventoryChanged tells the master and the character's player that the
// inventory changed (characters.CreatureHost). It carries only the character: the item
// and what happened to it never travel on the stream (RN-10). Call it after the commit.
func (s *Service) PublishInventoryChanged(campaignID, characterID, ownerUserID string) {
	s.hub.Publish(campaignID, live.Event{
		Audience: live.Audience{Master: true, UserID: ownerUserID},
		Message: &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_InventoryChanged_{
			InventoryChanged: &playv1.WatchGameSessionResponse_InventoryChanged{CharacterId: characterID},
		}},
		Coalesce: "inventory:" + characterID,
	})
}

// ItemEvents returns the inventory's history in the campaign's open session, newest
// first (characters.CreatureHost). No open session: none.
func (s *Service) ItemEvents(ctx context.Context, campaignID string, limit int) ([]link.ItemEvent, error) {
	q := s.queries
	session, err := q.GetOpenGameSession(ctx, campaignID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find the open session: %w", err)
	}
	rows, err := q.ListItemEventsOfSession(ctx, playdb.ListItemEventsOfSessionParams{GameSessionID: session.ID, Kinds: link.ItemEventKinds, RowLimit: int32(limit)})
	if err != nil {
		return nil, fmt.Errorf("list the item events: %w", err)
	}
	out := make([]link.ItemEvent, 0, len(rows))
	for _, r := range rows {
		out = append(out, link.ItemEvent{Kind: r.Kind, ActorUserID: deref(r.ActorUserID), Payload: r.Payload, At: r.CreatedAt})
	}
	return out, nil
}
