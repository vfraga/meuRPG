package play

import (
	"context"
	"fmt"
	"slices"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
	"github.com/PuraFome/meuRPG/backend/internal/rules/zone"
)

// What the zones that block sight do to what the players are told (W7-Z, RN-10): the creatures in
// one, and the ones they hide from a player, are named only to those who see them, on the map,
// in the log, in the waits and in the move events. zones.go says who sees whom; this file stamps
// the lines of the log and keeps the live events from telling where a creature is.

// playerUsers lists the players (user IDs) that play a creature of the combat, in a stable order.
func playerUsers(cs []playdb.Combatant) []string {
	var out []string
	for _, c := range cs {
		if c.UserID != nil && *c.UserID != "" && !slices.Contains(out, *c.UserID) {
			out = append(out, *c.UserID)
		}
	}
	slices.Sort(out)
	return out
}

// zonesHide says some zone of the combat blocks sight, so that a move is nobody's news.
func (s *Service) zonesHide(ctx context.Context, e playdb.Encounter) bool {
	if e.ID == "" {
		return false
	}
	rows, err := s.queries.ListMapZones(ctx, e.ID)
	if err != nil {
		return true // when it cannot be told, the safe answer is the one that tells nothing
	}
	return hasBlocking(zoneStatesOf(rows))
}

// stampZones narrows who may read the line of an event to the players who saw what it is about
// when it happened: a creature in a zone that blocks sight is seen only by its own player and by
// the ones whose creatures have a clear line to it. The master's lines (Secret) are left alone.
func (c *combatTx) stampZones(ctx context.Context, kind string, ev actionEvent) (actionEvent, error) {
	if kind == eventDoorOpened || ev.Secret || c.enc.ID == "" {
		return ev, nil
	}
	if err := c.loadZones(ctx); err != nil {
		return ev, err
	}
	if !hasBlocking(c.zones) {
		return ev, nil
	}
	cs, err := c.q.ListCombatantsWithDismissed(ctx, c.enc.ID)
	if err != nil {
		return ev, err
	}
	blocks := blockingCells(c.zones)
	users := playerUsers(cs)
	seen := slices.Clone(users)
	if ev.Fogged {
		seen = slices.Clone(ev.SeenBy)
	}
	changed := false
	for _, u := range users {
		if !slices.Contains(seen, u) {
			continue
		}
		var eyes []grid.Square
		for _, o := range cs {
			if o.UserID != nil && *o.UserID == u && placed(o) && !o.Defeated {
				eyes = append(eyes, squareOfCombatant(o))
			}
		}
		if len(eyes) == 0 {
			continue
		}
		for _, id := range ev.combatantIDs() {
			i := slices.IndexFunc(cs, func(o playdb.Combatant) bool { return o.ID == id })
			if i < 0 || !placed(cs[i]) || (cs[i].UserID != nil && *cs[i].UserID == u) {
				continue
			}
			squares := []grid.Square{squareOfCombatant(cs[i])}
			if kind == eventCombatantMoved && id == ev.Actor && ev.From != nil && ev.From.Placed {
				squares = append(squares, grid.Square{Col: int(ev.From.Col), Row: int(ev.From.Row)})
			}
			sees := slices.ContainsFunc(squares, func(sq grid.Square) bool {
				return slices.ContainsFunc(eyes, func(from grid.Square) bool { return !zone.Blocks(blocks, from, sq) })
			})
			if !sees {
				seen = slices.DeleteFunc(seen, func(x string) bool { return x == u })
				changed = true
				break
			}
		}
	}
	if changed {
		ev.Fogged, ev.SeenBy = true, seen
	}
	return ev, nil
}

// viewerNow is the viewer of a read after a change, from the combat as it stands.
func (s *Service) viewerNow(ctx context.Context, m authz.Membership, res combatResult) (combatViewer, error) {
	enc, err := s.queries.GetEncounterInSession(ctx, playdb.GetEncounterInSessionParams{GameSessionID: res.session.ID, ID: res.encounterID})
	if err != nil {
		return combatViewer{}, fmt.Errorf("find the encounter: %w", err)
	}
	cs, err := s.queries.ListCombatants(ctx, enc.ID)
	if err != nil {
		return combatViewer{}, fmt.Errorf("list the combatants: %w", err)
	}
	return s.viewerFor(ctx, m, enc, cs)
}

// castZoneProto is the zone a cast left, as the viewer is told it: the caster's player is told of
// their own zone whatever its form (they know where they put it), everyone else of the ones they see.
func (s *Service) castZoneProto(ctx context.Context, m authz.Membership, res combatResult, ev actionEvent, v combatViewer) (*playv1.MapZone, error) {
	if ev.Zone == nil || ev.Zone.What != "added" {
		return nil, nil
	}
	enc, err := s.queries.GetEncounterInSession(ctx, playdb.GetEncounterInSessionParams{GameSessionID: res.session.ID, ID: res.encounterID})
	if err != nil {
		return nil, s.dbError(ctx, "find the encounter", err)
	}
	cs, err := s.queries.ListCombatants(ctx, enc.ID)
	if err != nil {
		return nil, s.dbError(ctx, "list the combatants", err)
	}
	rows, err := s.queries.ListMapZones(ctx, enc.ID)
	if err != nil {
		return nil, s.dbError(ctx, "list the zones", err)
	}
	for _, z := range zoneStatesOf(rows) {
		if z.row.ID != ev.Zone.ID {
			continue
		}
		casterPlayer := z.row.CasterID != nil && slices.ContainsFunc(cs, func(c playdb.Combatant) bool { return c.ID == *z.row.CasterID && v.owns(c) })
		if !z.seenBy(v, cs) && !casterPlayer {
			return nil, nil
		}
		return zoneProto(z, v, cs, enc.Round), nil
	}
	return nil, nil // the zone ended meanwhile
}

// markSilenced marks the spells the creature cannot cast now because it is entirely inside a zone
// of silence: the ones with a verbal component are not available, with the reason SILENCED
// (SRD, Silence: "casting a spell that includes a verbal component is impossible there").
func (s *Service) markSilenced(ctx context.Context, campaignID string, d *encounterData, who playdb.Combatant, opts *rulesv1.TurnOptions) error {
	if !silencedAt(zoneStatesOf(d.zones), who, isTheatre(d.enc)) {
		return nil
	}
	reason := func() *rulesv1.DisabledReason {
		return &rulesv1.DisabledReason{Code: rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_SILENCED}
	}
	for _, sp := range opts.GetSpells() {
		if !sp.GetEnabled() {
			continue
		}
		spell, err := s.roster.CombatSpell(ctx, nil, campaignID, who.CharacterID, sp.GetSpell().GetKey(), int(sp.GetSpell().GetLevel()), "")
		if err != nil {
			return err
		}
		if spell.Verbal {
			sp.Enabled, sp.Reason = false, reason()
		}
	}
	for _, a := range opts.GetAttacks() {
		if !a.GetEnabled() || a.GetAttack().GetKind() != rulesv1.AttackKind_ATTACK_KIND_SPELL {
			continue
		}
		spell, err := s.roster.CombatSpell(ctx, nil, campaignID, who.CharacterID, a.GetAttack().GetKey(), 0, "")
		if err != nil {
			return err
		}
		if spell.Verbal {
			a.Enabled, a.Reason = false, reason()
		}
	}
	return nil
}
