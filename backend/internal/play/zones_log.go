package play

import (
	"math"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/zone"
)

// The line of a zone in the combat log (W7-Z): a zone put, moved, ended or dispersed; a creature
// it caught; the answer of a saving throw it asked. A creature is named only to the players who
// saw it when it happened (the stamp of the event, combat_fog.go and zones_sight.go), the caster
// and the DC of a zone of an NPC never reach a player (RN-20), and the dice are the master's and
// the roller's.

var logZoneEvents = map[string]playv1.CombatLogZoneEvent{
	"added": playv1.CombatLogZoneEvent_COMBAT_LOG_ZONE_EVENT_ADDED, "moved": playv1.CombatLogZoneEvent_COMBAT_LOG_ZONE_EVENT_MOVED,
	"ended": playv1.CombatLogZoneEvent_COMBAT_LOG_ZONE_EVENT_ENDED, "dispersed": playv1.CombatLogZoneEvent_COMBAT_LOG_ZONE_EVENT_DISPERSED,
	"caught": playv1.CombatLogZoneEvent_COMBAT_LOG_ZONE_EVENT_CAUGHT, "saved": playv1.CombatLogZoneEvent_COMBAT_LOG_ZONE_EVENT_SAVE_ANSWERED,
}

// zoneEntry is the zone line the viewer gets.
func (e *logEntry) zoneEntry(v combatViewer, byID map[string]playdb.Combatant, zones map[string]playdb.MapZone) *playv1.CombatLogZone {
	n := e.ev.Zone
	out := &playv1.CombatLogZone{ZoneId: n.ID, SpellKey: n.Key, Event: logZoneEvents[n.What], MovedSquares: n.Moved, DistanceSquares: n.Squares}
	if z, ok := zones[n.ID]; ok {
		out.Name = z.Name
	}
	if n.Who != "" {
		out.CombatantId = n.Who
	}
	out.TriggerKind = triggerKindToProto[zone.TriggerKind(n.Trigger)]
	out.SaveAbility, out.Saved = n.Ability, n.Saved
	if n.Reason != "" {
		out.CloseReason = n.Reason
	}
	// The roll is the master's and its roller's; the DC is the master's, and the party's when a player's
	// character cast the zone (RN-20).
	who := byID[n.Who]
	if v.master || v.owns(who) {
		out.D20, out.Modifier, out.Total = n.D20, n.Modifier, n.Total
		if v.master || e.zoneCastByPlayer(zones, byID) {
			if n.DC > 0 {
				dc := clamp32(int(n.DC), 0, math.MaxInt32)
				out.Dc = &dc
			}
		}
	}
	return out
}

// zoneCastByPlayer says a player's character cast the zone of the line.
func (e *logEntry) zoneCastByPlayer(zones map[string]playdb.MapZone, byID map[string]playdb.Combatant) bool {
	z, ok := zones[e.ev.Zone.ID]
	return ok && z.CasterID != nil && byID[*z.CasterID].Kind == kindPlayer
}
