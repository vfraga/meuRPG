package play

import (
	"context"
	"fmt"
	"slices"

	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// The master's "Desfazer" and the zones (W7-Z): taking back a cast that left a zone takes the zone
// away, with the saving throws it was waiting for; taking back a cast or an end of concentration that
// ended zones brings them back. The lines the zones wrote with the change (put, caught, ended) are
// part of it: they go from the log with it.

// zoneEventKinds are the kinds of the lines a zone writes in the change that made or ended it.
var zoneEventKinds = []string{eventMapZoneAdded, eventMapZoneTriggered, eventMapZoneEnded, eventMapZoneMoved}

// undoZones undoes what the event did to the zones, and returns the older lines of the same change,
// which the undo takes back with it. recent are the session's latest events, newest first.
func (s *Service) undoZones(ctx context.Context, c *combatTx, recent []playdb.ListRecentSessionEventsRow, last playdb.ListRecentSessionEventsRow, ev actionEvent) ([]string, error) {
	var gone []string // the zones whose lines go
	if last.Kind == eventSpellCast && ev.Zone != nil && ev.Zone.What == "added" {
		if err := c.q.CloseZoneSaveWindowsOfZone(ctx, playdb.CloseZoneSaveWindowsOfZoneParams{ZoneID: &ev.Zone.ID, CloseReason: closeUndone, AnsweredAt: &c.now}); err != nil {
			return nil, fmt.Errorf("close the zone's saves: %w", err)
		}
		if err := c.q.DeleteMapZone(ctx, ev.Zone.ID); err != nil {
			return nil, fmt.Errorf("take the zone back: %w", err)
		}
		gone = append(gone, ev.Zone.ID)
	}
	for _, id := range ev.ZonesEnded {
		if err := c.q.UnendMapZone(ctx, id); err != nil {
			return nil, fmt.Errorf("bring the zone back: %w", err)
		}
		gone = append(gone, id)
	}
	if len(gone) == 0 {
		return nil, nil
	}
	if err := c.reloadZones(ctx); err != nil {
		return nil, err
	}
	// The older lines of the change: the ones the zones wrote just before the event's own.
	var also []string
	passed := false
	for _, e := range recent {
		if e.ID == last.ID {
			passed = true
			continue
		}
		if !passed {
			continue
		}
		if !slices.Contains(zoneEventKinds, e.Kind) {
			if e.Kind == eventCreatureSummoned || e.Kind == eventCreatureDismissed || e.Kind == eventConditionsSet {
				continue // the other parts of the same change sit among them
			}
			break
		}
		if pe, err := readEvent(e.Payload); err == nil && pe.Zone != nil && slices.Contains(gone, pe.Zone.ID) {
			also = append(also, e.ID)
		}
	}
	return also, nil
}

// closeUndone is the reason a window closes with when the cast that opened it is taken back.
const closeUndone = "undone"
