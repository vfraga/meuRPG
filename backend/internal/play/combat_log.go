package play

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/timestamppb"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// The combat log, "Registro do combate" (D10, MR-012; the history screen
// ADR-0007 deferred). It is built on every read from the combat's
// session_events, never stored on its own, so the history and the log cannot
// disagree. Each entry is structured (kind, who, the roll, the outcome, the
// damage): the app writes the Portuguese sentence.
//
// What each viewer gets (RN-10, RN-20):
//
//	                    master                    player
//	an entry            all                       only those without a hidden combatant
//	                                              (when it happened, and now)
//	the dice            all                       their own character's only
//	hit points after    yes                       never
//	"hidden" mark       set on the ones the       never set
//	                    players do not get
//	undo                the last action's entry   never
//
// A combatant the master reveals later does not bring its old entries with
// it: each entry remembers, from the event, whether a hidden combatant was in
// it, and a player never gets that one.

// logEventLimit is how many of a combat's latest events the log reads. A
// variable only so a test can make a combat longer than it.
var logEventLimit int32 = 5000

// logEntry is an entry while it is being built: the event's numbers and who
// may see it.
type logEntry struct {
	id     string
	kind   playv1.CombatLogKind
	at     time.Time
	ev     actionEvent
	status playv1.PendingDamageStatus // the damage of an attack, once it hit
	dmg    *actionEvent               // the damage roll, once it was rolled
	// applied is the master's apply of an attack's damage on a character.
	applied *actionEvent
	// pend is, for a spell, what became of each pending damage it opened, and
	// stopped the pending damages Escudo stopped (a spell attack or an attack).
	pend    map[string]*damageLog
	stopped []string
	// returned and returnBlocked: the opportunity attack's damage took the mover to
	// 0 hit points, and it went back to the square it left the reach at (or could
	// not, because the square was taken: the master's line says so).
	returned, returnBlocked bool
	// masterOnly is a line only the master gets, whatever the combatants.
	masterOnly bool
	// shapeStarted: a Wild Shape line is the druid becoming the beast, not leaving it.
	shapeStarted bool
	// hosts are the events the entry shows: an attack's own and the ones of its
	// damage, so the entry of the last action is the one to undo.
	hosts []string
}

// damageLog is what became of one pending damage of a spell: where it is, the
// roll that settled it (shared by the whole cast), what it did to its target,
// and the master's apply.
type damageLog struct {
	status  playv1.PendingDamageStatus
	roll    *actionEvent
	hit     damageHit
	applied *actionEvent
	// discarded is the master's discard, which carries the concentration reminder
	// when it settled the cast's last damage for the target.
	discarded *actionEvent
}

// ListCombatLog implements playv1connect.CombatServiceHandler.
func (s *Service) ListCombatLog(
	ctx context.Context,
	req *connect.Request[playv1.ListCombatLogRequest],
) (*connect.Response[playv1.ListCombatLogResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	encID, err := parseCombatID(req.Msg.GetEncounterId(), "encounter")
	if err != nil {
		return nil, err
	}
	// The combat's events, its combatants and the master's undo marker are one
	// snapshot (audit D-03): a log line whose actor is missing, or an "undo" that
	// points at an event the list does not have, would be a torn read.
	var (
		enc    playdb.Encounter
		events []playdb.ListEncounterEventsRow
		cs     []playdb.Combatant
		recent []playdb.ListRecentSessionEventsRow
		zones  []playdb.MapZone
	)
	err = db.ReadTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		session, err := openSessionWith(ctx, q, m.CampaignID)
		if err != nil {
			return err
		}
		if enc, err = encounterInSessionWith(ctx, q, session.ID, encID); err != nil {
			return err
		}
		if events, err = q.ListEncounterEvents(ctx, playdb.ListEncounterEventsParams{EncounterID: &enc.ID, Limit: logEventLimit}); err != nil {
			return fmt.Errorf("list the combat's events: %w", err)
		}
		if zones, err = q.ListAllMapZones(ctx, enc.ID); err != nil {
			return fmt.Errorf("list the zones: %w", err)
		}
		// With the dismissed creatures: their lines survive a concentration ending.
		if cs, err = q.ListCombatantsWithDismissed(ctx, enc.ID); err != nil {
			return fmt.Errorf("list the combatants: %w", err)
		}
		if m.Role == authz.RoleMaster {
			if recent, err = q.ListRecentSessionEvents(ctx, playdb.ListRecentSessionEventsParams{GameSessionID: session.ID, Limit: recentEvents}); err != nil {
				return fmt.Errorf("read the latest events: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "read the combat log", err)
	}
	slices.Reverse(events) // oldest first, as buildLog reads them
	// The log's lines are filtered by who could see them when they happened
	// (actionEvent.SeenBy), not by what the viewer sees now; only the current
	// rules about hidden combatants are worked out from the combatants as they are.
	v := viewerOf(m)
	if !v.master {
		rules, err := s.tableRules(ctx, nil, m.CampaignID)
		if err != nil {
			return nil, s.dbError(ctx, "read the table's rules", err)
		}
		v.hideDeath = rules.DeathSavesHidden
	}
	entries := buildLog(events)

	res := &playv1.ListCombatLogResponse{}
	var lastID string
	if v.master {
		if last, ok := lastAction(recent, enc.ID); ok {
			lastID = last.ID
			res.UndoableEventId = last.ID
		}
	}

	names := &keyNames{s: s, campaignID: m.CampaignID, byCharacter: map[string]link.Sheet{}, zones: map[string]playdb.MapZone{}}
	for _, z := range zones {
		names.zones[z.ID] = z
	}
	byID := make(map[string]playdb.Combatant, len(cs))
	for _, c := range cs {
		byID[c.ID] = c
	}
	// Latest first, in groups by round.
	for _, e := range slices.Backward(entries) {
		out, ok := e.view(ctx, v, byID, names, lastID)
		if !ok {
			continue
		}
		if n := len(res.Rounds); n == 0 || res.Rounds[n-1].Round != out.Round {
			res.Rounds = append(res.Rounds, &playv1.CombatLogRound{Round: out.Round})
		}
		last := res.Rounds[len(res.Rounds)-1]
		last.Entries = append(last.Entries, out)
	}
	return connect.NewResponse(res), nil
}

// buildLog turns a combat's events, in order, into entries. An event an undo
// took back leaves no trace; the events of one attack (its roll, its damage,
// the master's apply or discard, a reaction) merge into one entry, and those of
// a spell (its cast, the damage rolls and applies of its targets) into one.
func buildLog(events []playdb.ListEncounterEventsRow) []*logEntry {
	undone := map[string]bool{}
	for _, e := range events {
		if e.Kind != eventActionUndone {
			continue
		}
		if ev, err := readEvent(e.Payload); err == nil {
			undone[ev.Undone] = true
			for _, id := range ev.UndoneAlso {
				undone[id] = true
			}
		}
	}
	var out []*logEntry
	byPending := map[string]*logEntry{}
	byFiring := map[string]*logEntry{}
	// An opportunity offer's answer without an attack (a decline, a skip) is no
	// line, but the master's undo can take it back: it belongs to the entry of
	// the move that made the offer, so that entry is the one to undo.
	byMove := map[string]*logEntry{}
	moveOf := map[string]string{} // offer id -> move id
	for _, e := range events {
		if undone[e.ID] || e.Kind == eventActionUndone {
			continue
		}
		ev, err := readEvent(e.Payload)
		if err != nil {
			continue // never: this package wrote it
		}
		// The log starts when the combat begins: the setup is not in it. The master's line of
		// the monsters he put in (their rolled hit points) is the exception: it is his from the setup.
		if ev.Round == 0 && (e.Kind != eventCombatantsAdded || ev.Monsters == nil) {
			continue
		}
		entry := &logEntry{id: e.ID, at: e.CreatedAt, ev: ev, hosts: []string{e.ID}}
		switch e.Kind {
		case eventCombatBegun:
			entry.kind = playv1.CombatLogKind_COMBAT_LOG_KIND_COMBAT_BEGUN
		case eventEncounterEnded:
			entry.kind = playv1.CombatLogKind_COMBAT_LOG_KIND_COMBAT_ENDED
		case eventCombatantMoved:
			if !ev.OnTurn || (ev.DistanceFt == 0 && ev.DistanceDFt == 0 && ev.Jump != jumpHigh) {
				continue // placing a token is not a move of the fight
			}
			entry.kind = playv1.CombatLogKind_COMBAT_LOG_KIND_MOVED
			if ev.MoveID != "" {
				byMove[ev.MoveID] = entry
			}
		case eventDoorOpened:
			entry.kind = playv1.CombatLogKind_COMBAT_LOG_KIND_DOOR_OPENED
		case eventCombatantsAdded:
			if ev.Monsters == nil {
				continue // reinforcements (AddCombatants) are no line
			}
			entry.kind, entry.masterOnly = playv1.CombatLogKind_COMBAT_LOG_KIND_MONSTERS_ADDED, true
		case eventCombatantHiddenSet:
			// The master's own reveal is his alone; the one an area spell made is the
			// players' line too ("foi revelado"): the creature appears on their map.
			entry.kind, entry.masterOnly = playv1.CombatLogKind_COMBAT_LOG_KIND_REVEAL_CHANGED, !ev.ByArea
		case eventActionTaken:
			entry.kind = playv1.CombatLogKind_COMBAT_LOG_KIND_ACTION
		case eventHitPointsAdjusted:
			entry.kind, entry.masterOnly = playv1.CombatLogKind_COMBAT_LOG_KIND_HIT_POINTS_ADJUSTED, true
		case eventDeathSaveRolled:
			entry.kind = playv1.CombatLogKind_COMBAT_LOG_KIND_DEATH_SAVE
		case eventDeathConfirmed:
			entry.kind = playv1.CombatLogKind_COMBAT_LOG_KIND_DEATH_CONFIRMED
		case eventConditionsSet:
			entry.kind = playv1.CombatLogKind_COMBAT_LOG_KIND_CONDITIONS_CHANGED
		case eventTurnPartEnded:
			entry.kind = playv1.CombatLogKind_COMBAT_LOG_KIND_TURN_PART_ENDED
		case eventAttackRolled:
			entry.kind = playv1.CombatLogKind_COMBAT_LOG_KIND_ATTACK
			if ev.Pending != "" {
				entry.status = playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_AWAITING_ROLL
				byPending[ev.Pending] = entry
			}
		case eventSpellCast:
			entry.kind = playv1.CombatLogKind_COMBAT_LOG_KIND_SPELL_CAST
			entry.pend = map[string]*damageLog{}
			for _, h := range ev.Hits {
				for _, id := range append([]string{h.Pending}, h.More...) {
					if id != "" {
						entry.pend[id] = &damageLog{status: playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_AWAITING_ROLL}
						byPending[id] = entry
					}
				}
			}
		case eventTrapTriggered:
			if ev.Trap != nil && ev.Trap.ExtendsID != "" {
				// The master added creatures to a firing: they join its entry, and their damage
				// moves on with the master's apply like the rest.
				if host, ok := byFiring[ev.Trap.ExtendsID]; ok && host.ev.Trap != nil {
					host.ev.Trap.Caught = append(host.ev.Trap.Caught, ev.Trap.Caught...)
					host.hosts = append(host.hosts, e.ID)
					for _, cc := range ev.Trap.Caught {
						for _, d := range cc.Damages {
							if d.Pending == "" {
								continue
							}
							status := playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_ROLLED
							if d.Applied {
								status = playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_APPLIED
							}
							host.pend[d.Pending] = &damageLog{status: status}
							byPending[d.Pending] = host
						}
					}
				}
				continue
			}
			// A trap fired (MR-035): the entry holds its pending damages, which the
			// master's apply or discard moves on like a spell's.
			entry.kind = playv1.CombatLogKind_COMBAT_LOG_KIND_TRAP_TRIGGERED
			entry.pend = map[string]*damageLog{}
			byFiring[e.ID] = entry
			if ev.Trap != nil {
				for _, cc := range ev.Trap.Caught {
					for _, d := range cc.Damages {
						if d.Pending == "" {
							continue
						}
						status := playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_ROLLED
						if d.Applied {
							status = playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_APPLIED
						}
						entry.pend[d.Pending] = &damageLog{status: status}
						byPending[d.Pending] = entry
					}
				}
			}
		case eventMapZoneAdded, eventMapZoneMoved, eventMapZoneEnded, eventMapZoneTriggered, eventZoneSaveAnswered:
			// What a zone did (zones_log.go): the master's own edits (map_zone_changed) are no line.
			if ev.Zone == nil {
				continue
			}
			entry.kind = playv1.CombatLogKind_COMBAT_LOG_KIND_ZONE
		case eventTrapSearched:
			// A search is the Search action: the master's line only, with the roll in
			// his history; the table sees nothing of it (RN-10).
			entry.kind, entry.masterOnly = playv1.CombatLogKind_COMBAT_LOG_KIND_ACTION, true
			entry.ev.Key = "standard:search"
		case eventWildShapeStarted, eventWildShapeEnded:
			entry.kind = playv1.CombatLogKind_COMBAT_LOG_KIND_WILD_SHAPE
			entry.shapeStarted = e.Kind == eventWildShapeStarted
		case eventReactionUsed:
			entry.kind = playv1.CombatLogKind_COMBAT_LOG_KIND_REACTION
			// Escudo that stopped the attack takes its damage away; the hit waits for
			// its roll otherwise, which the roll event says.
			if host, ok := byPending[ev.Pending]; ok {
				host.hosts = append(host.hosts, e.ID)
				if ev.Stopped {
					host.setStatus(ev.Pending, playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_DISCARDED)
					host.stopped = append(host.stopped, ev.Pending)
				}
			}
			for _, h := range ev.AlsoStopped {
				if host, ok := byPending[h.Pending]; ok {
					host.hosts = append(host.hosts, e.ID)
					host.setStatus(h.Pending, playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_DISCARDED)
					host.stopped = append(host.stopped, h.Pending)
				}
			}
		case eventOpportunityOffered:
			moveOf[ev.OfferID] = ev.MoveID
			if !ev.ByHand {
				continue // a move's offer is no line of its own: the move's is
			}
			// The master's offer in a combat without a map is a line, and its answer
			// (decline, skip, withdraw) belongs to it, so the answer can be undone.
			entry.kind = playv1.CombatLogKind_COMBAT_LOG_KIND_OPPORTUNITY_OFFERED
			byMove[ev.MoveID] = entry
		case eventReactionDeclined:
			if host, ok := byPending[ev.Pending]; ok {
				host.hosts = append(host.hosts, e.ID) // a decline is the entry's last action to undo
			}
			if host, ok := byMove[moveOf[ev.OfferID]]; ev.OfferID != "" && ok {
				host.hosts = append(host.hosts, e.ID) // an opportunity offer's answer is the move's to undo
			}
			continue
		case eventDamageRolled, eventDamageApplied, eventDamageDiscarded:
			// The damage lands in the attack's or the spell's entry.
			host, ok := byPending[ev.Pending]
			if !ok {
				continue
			}
			host.hosts = append(host.hosts, e.ID)
			host.land(e.Kind, ev)
			continue
		default:
			continue // initiative, turns, reinforcements: not lines of the log
		}
		out = append(out, entry)
	}
	return out
}

// setStatus records where a pending damage of the entry is.
func (e *logEntry) setStatus(pending string, status playv1.PendingDamageStatus) {
	if dl, ok := e.pend[pending]; ok {
		dl.status = status
		return
	}
	e.status = status
}

// land puts a damage event on the entry of the attack or the spell it belongs
// to: the roll, the master's apply, or his discard.
func (e *logEntry) land(kind string, ev actionEvent) {
	switch kind {
	case eventDamageRolled:
		e.returned, e.returnBlocked = e.returned || ev.ReturnedFrom != nil, e.returnBlocked || ev.ReturnBlocked
		hits := ev.Settled
		if len(hits) == 0 {
			hits = []damageHit{{Pending: ev.Pending, Target: ev.Target, Amount: ev.Amount, Applied: ev.Applied, After: ev.After, ConcentrationDC: ev.ConcentrationDC}}
		}
		for _, h := range hits {
			status := playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_ROLLED
			if h.Applied {
				status = playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_APPLIED
			}
			if dl, ok := e.pend[h.Pending]; ok {
				dl.roll, dl.hit, dl.status = &ev, h, status
				continue
			}
			e.dmg, e.status = &ev, status
			e.dmg.After = h.After // the target's hit points after, for an NPC that took it at once
		}
	case eventDamageApplied:
		e.returned, e.returnBlocked = e.returned || ev.ReturnedFrom != nil, e.returnBlocked || ev.ReturnBlocked
		if dl, ok := e.pend[ev.Pending]; ok {
			dl.applied, dl.status = &ev, playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_APPLIED
			return
		}
		e.status, e.applied = playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_APPLIED, &ev
		if e.dmg != nil {
			e.dmg.After = ev.After // the character's hit points after
		}
	case eventDamageDiscarded:
		e.setStatus(ev.Pending, playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_DISCARDED)
		if dl, ok := e.pend[ev.Pending]; ok {
			dl.discarded = &ev
		}
	}
}

// view builds the entry the viewer gets, and false when they do not get it.
func (e *logEntry) view(ctx context.Context, v combatViewer, byID map[string]playdb.Combatant, names *keyNames, lastID string) (*playv1.CombatLogEntry, bool) {
	actor, target := byID[e.ev.Actor], byID[e.ev.Target]
	switch e.kind {
	case playv1.CombatLogKind_COMBAT_LOG_KIND_HIT_POINTS_ADJUSTED:
		actor, target = playdb.Combatant{}, byID[e.ev.Actor] // the affected one is the target
	case playv1.CombatLogKind_COMBAT_LOG_KIND_REACTION:
		target = playdb.Combatant{} // who attacked is not part of the line
	case playv1.CombatLogKind_COMBAT_LOG_KIND_DEATH_CONFIRMED, playv1.CombatLogKind_COMBAT_LOG_KIND_CONDITIONS_CHANGED:
		actor, target = playdb.Combatant{}, byID[e.ev.Actor]
	case playv1.CombatLogKind_COMBAT_LOG_KIND_ZONE:
		actor, target = playdb.Combatant{}, byID[e.ev.Actor] // the creature the zone caught is the line's target
	}
	// What a player may see: nothing with a hidden combatant in it, when it
	// happened or now (a combatant the master hides again takes its lines back).
	// A combatant that left the combat is unknown now, and an entry about it
	// would be anonymous: a player does not get it either.
	_, actorKnown := byID[e.ev.Actor]
	_, targetKnown := byID[e.ev.Target]
	known := (e.ev.Actor == "" || actorKnown) && (e.ev.Target == "" || targetKnown || e.kind == playv1.CombatLogKind_COMBAT_LOG_KIND_REACTION)
	visible := !e.masterOnly && !e.ev.Secret && !actor.Hidden && !target.Hidden && known
	if e.kind == playv1.CombatLogKind_COMBAT_LOG_KIND_REACTION && (e.ev.AttackerHidden || byID[e.ev.Target].Hidden) && !v.owns(actor) {
		visible = false
	}
	// On a map with the fog of war, a line is the players' who saw its NPCs when it
	// happened, even if they have walked into the dark since; one they did not see
	// never appears later (MR-036). The master's copy says it is hidden from them.
	seen := e.ev.seenByViewer(v)
	for _, h := range e.ev.Hits { // every target of a spell
		hit, ok := byID[h.Target]
		if e.ev.Placed { // an area the server placed: the hidden creatures it hit are left out of the line, not the line
			continue
		}
		visible = visible && ok && !hit.Hidden
	}
	if !v.master && (!visible || !seen) {
		return nil, false
	}
	// The offer's line is for whoever gets the offer (RN-10): the reactor's player
	// and the mover's.
	if !v.master && e.kind == playv1.CombatLogKind_COMBAT_LOG_KIND_OPPORTUNITY_OFFERED && !v.owns(actor) && !v.owns(target) {
		return nil, false
	}

	// The dice are the master's and the owner's: the master's rolls are not the
	// players'.
	dice := v.master || v.owns(actor)
	out := &playv1.CombatLogEntry{
		Id: e.id, Kind: e.kind, At: timestamppb.New(e.at), Round: e.ev.Round,
		ActorId: actor.ID, ActorLabel: actor.Label, TargetId: target.ID, TargetLabel: target.Label,
		Key: e.ev.Key,
	}
	if v.master {
		out.Hidden = !visible || (e.ev.Fogged && len(e.ev.SeenBy) == 0)
		out.Undoable = slices.Contains(e.hosts, lastID)
	}
	switch e.kind {
	case playv1.CombatLogKind_COMBAT_LOG_KIND_ATTACK, playv1.CombatLogKind_COMBAT_LOG_KIND_ACTION,
		playv1.CombatLogKind_COMBAT_LOG_KIND_SPELL_CAST, playv1.CombatLogKind_COMBAT_LOG_KIND_REACTION:
		out.KeyNamePt = names.of(ctx, actor, e.ev.Key)
	}
	switch e.kind {
	case playv1.CombatLogKind_COMBAT_LOG_KIND_MONSTERS_ADDED:
		for _, it := range e.ev.Monsters.Items {
			out.Monsters = append(out.Monsters, &playv1.CombatLogMonster{
				CombatantId: it.ID, Label: byID[it.ID].Label, HitPoints: it.HitPoints, Rolled: it.Dice != "", Dice: it.Dice, Faces: it.Faces, Modifier: it.Modifier,
			})
		}
	case playv1.CombatLogKind_COMBAT_LOG_KIND_MOVED:
		out.DistanceFt, out.DistanceDft = e.ev.DistanceFt, e.ev.DistanceDFt
		if out.DistanceDft == 0 { // an event written before the tenths of a foot
			out.DistanceDft = e.ev.DistanceFt * 10
		}
		switch e.ev.Jump {
		case jumpLong:
			out.Jump = playv1.JumpKind_JUMP_KIND_LONG
		case jumpHigh:
			out.Jump, out.JumpHeightDft = playv1.JumpKind_JUMP_KIND_HIGH, e.ev.HeightDFt
		}
		out.LandingDifficult = v.master && e.ev.LandingDifficult // the Acrobatics reminder is the master's alone
	case playv1.CombatLogKind_COMBAT_LOG_KIND_DOOR_OPENED:
		out.Door = &playv1.CombatLogDoor{Col: e.ev.Col, Row: e.ev.Row}
	case playv1.CombatLogKind_COMBAT_LOG_KIND_REVEAL_CHANGED:
		out.NowHidden = e.ev.NowHidden
		out.ActorId, out.ActorLabel = "", ""
		out.TargetId, out.TargetLabel = e.ev.Actor, byID[e.ev.Actor].Label
	case playv1.CombatLogKind_COMBAT_LOG_KIND_HIT_POINTS_ADJUSTED:
		out.HitPointsDelta = e.ev.Delta
		if e.ev.After != nil {
			out.HitPointsAfter = &e.ev.After.HP
		}
	case playv1.CombatLogKind_COMBAT_LOG_KIND_ATTACK:
		out.Outcome = outcomeToProto[e.ev.Outcome]
		out.AsReaction = e.ev.AsReaction
		out.ReturnedToReach, out.ReturnBlocked = e.returned, v.master && e.returnBlocked
		coverKey, coverSource := e.ev.coverFor(v)
		out.Cover, out.CoverSource = coverDegreeProto(coverKey), coverSourceProto(coverSource)
		if v.master && e.ev.TargetAC > 0 { // "CA 17: 15 + 2 de meia cobertura": a player never gets an armor class (RN-20)
			out.TargetArmorClass, out.CoverBonus = &e.ev.TargetAC, e.ev.CoverBonus
		}
		if dice {
			out.AttackRoll = diceRoll(1, 20, []int32{e.ev.D20}, e.ev.Modifier, e.ev.Total, e.ev.Physical)
		}
		if len(e.stopped) > 0 { // Escudo stopped it: a miss, with no damage
			out.Outcome, out.StoppedByReaction = playv1.AttackOutcome_ATTACK_OUTCOME_MISS, true
		} else if e.ev.Pending != "" {
			out.Damage = e.damage(dice, v.master, v.master || v.owns(target), out)
		}
	case playv1.CombatLogKind_COMBAT_LOG_KIND_ACTION:
		if e.ev.Heal { // Retomar o fôlego: only the master and its own player see the numbers
			if v.master || v.owns(actor) {
				out.Damage = &playv1.CombatLogDamage{
					Status: playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_APPLIED, Healing: true, Amount: e.ev.Amount,
					Roll: diceRoll(e.ev.DiceCount, e.ev.DiceSides, e.ev.Faces, e.ev.Modifier, e.ev.Total, e.ev.Physical),
				}
			}
		}
	case playv1.CombatLogKind_COMBAT_LOG_KIND_SPELL_CAST:
		out.Spell = e.spellView(v, byID)
	case playv1.CombatLogKind_COMBAT_LOG_KIND_ZONE:
		out.Zone = e.zoneEntry(v, byID, names.zones)
	case playv1.CombatLogKind_COMBAT_LOG_KIND_TRAP_TRIGGERED:
		out.Trap = e.trapEntry(ctx, v, byID, names)
	case playv1.CombatLogKind_COMBAT_LOG_KIND_WILD_SHAPE:
		out.WildShape = &playv1.CombatLogWildShape{BeastKey: e.ev.Beast, BeastNamePt: names.contentName(ctx, e.ev.Beast), Started: e.shapeStarted}
		if !e.shapeStarted {
			out.WildShape.EndReason = wildShapeEndReasonProto[e.ev.Reason]
			if v.master || v.owns(actor) { // the carried damage is the druid's player's and the master's (RN-20)
				out.WildShape.CarriedDamage = e.ev.Carried
			}
		}
	case playv1.CombatLogKind_COMBAT_LOG_KIND_REACTION:
		out.Spell = &playv1.CombatLogSpell{Slot: slotProto(e.ev.Slot)}
	case playv1.CombatLogKind_COMBAT_LOG_KIND_DEATH_SAVE:
		after := e.ev.Death
		if after == nil {
			after = &deathState{}
		}
		if v.hiddenFrom(e.ev.DeathHidden, actor) {
			// The table keeps the road to the owner and the master (RN-24): a save that
			// went on is no line for anybody else, and the one that made the character
			// stable says only that it did, a result the table sees.
			if after.Successes < 3 || e.ev.DeathOutcome != deathSuccess {
				return nil, false
			}
			out.DeathSave = &playv1.CombatLogDeathSave{Stable: true}
			break
		}
		out.DeathSave = &playv1.CombatLogDeathSave{
			Outcome: deathOutcomeToProto[e.ev.DeathOutcome], Successes: after.Successes, Failures: after.Failures,
			Stable: after.Successes >= 3, Dying: v.master && after.Failures >= 3,
		}
		if v.master || v.owns(actor) {
			out.DeathSave.Roll = diceRoll(1, 20, []int32{e.ev.D20}, 0, e.ev.D20, e.ev.Physical)
		}
	case playv1.CombatLogKind_COMBAT_LOG_KIND_CONDITIONS_CHANGED:
		out.Conditions = e.ev.Conditions
		out.ConcentrationEndedKey = e.ev.ConcEnded
	}
	return out, true
}

// spellView is the cast as the viewer gets it: what each target did, in the
// order they were listed. A target's save dice are the master's and the
// target's own player's; the caster's d20 the master's and the caster's.
func (e *logEntry) spellView(v combatViewer, byID map[string]playdb.Combatant) *playv1.CombatLogSpell {
	caster := byID[e.ev.Actor]
	out := &playv1.CombatLogSpell{Slot: slotProto(e.ev.Slot), Concentrating: e.ev.Concentrate, ConcentrationEndedKey: e.ev.ConcEnded}
	out.EffectKind, out.PoolRoll, out.EffectConditionKey, out.EffectThreshold = effectHeader(e.ev, v, caster)
	for _, h := range e.ev.Hits {
		target := byID[h.Target]
		// The players' line never lists a creature that was hidden when an area hit it
		// (not even after the master reveals it: the line would say the spell hit it),
		// nor one that is hidden now or that a combatant that left no longer names.
		if !v.master && (h.HiddenAtCast || target.Hidden || target.ID == "") {
			continue
		}
		t := &playv1.CombatLogSpellTarget{
			TargetId: target.ID, TargetLabel: target.Label, Darts: h.Darts, Outcome: outcomeToProto[h.Outcome],
			AttackRoll: attackRollView(h, v, caster), Save: saveView(h.Save, v, caster, target),
			Effect: effectView(h, v, target),
		}
		coverKey, coverSource := h.coverFor(v, e.ev.CoverUsers)
		t.Cover, t.CoverSource = coverDegreeProto(coverKey), coverSourceProto(coverSource)
		t.Hidden = v.master && h.HiddenAtCast
		if v.master && h.TargetAC > 0 {
			t.TargetArmorClass, t.CoverBonus = &h.TargetAC, h.CoverBonus
		}
		if dl, ok := e.pend[h.Pending]; ok {
			if slices.Contains(e.stopped, h.Pending) { // Escudo stopped the spell attack
				t.Outcome = playv1.AttackOutcome_ATTACK_OUTCOME_MISS
			} else {
				t.Damage = dl.view(v, caster, target)
			}
		}
		for _, id := range h.More { // the other damage types of the spell
			if dl, ok := e.pend[id]; ok {
				t.MoreDamages = append(t.MoreDamages, dl.view(v, caster, target))
			}
		}
		out.Targets = append(out.Targets, t)
	}
	return out
}

// view is a spell target's damage or heal as the viewer gets it: the amount that
// landed, the dice to the master and the caster's player, what the master
// overruled to him alone.
func (d *damageLog) view(v combatViewer, caster, target playdb.Combatant) *playv1.CombatLogDamage {
	out := &playv1.CombatLogDamage{Status: d.status}
	if d.discarded != nil && d.discarded.ConcentrationDC > 0 && (v.master || v.owns(target)) {
		dc := d.discarded.ConcentrationDC
		out.ConcentrationDc = &dc
	}
	if d.roll == nil {
		return out // the cast opened it and it is not rolled yet
	}
	r := d.roll
	out.Amount, out.DamageTypeKey, out.DamageTypePt = d.hit.Amount, r.DamageType, damageTypePT[r.DamageType]
	out.Half, out.Healing = d.hit.Half, r.Heal
	out.CriticalRule, out.CriticalMax = pendingCriticalRule(r.Critical, r.CriticalMaxRule), r.CriticalMax
	if r.Heal && !v.master && !v.owns(target) {
		// A heal capped at the maximum would tell how many hit points the target
		// lacked: everyone but the master and the target's player gets the roll (RN-20).
		out.Amount = r.Total
	}
	if v.master || v.owns(caster) { // the caster's dice, as for an attack's damage
		out.Roll = diceRoll(r.DiceCount, r.DiceSides, r.Faces, r.Modifier, r.Total, r.Physical)
	}
	if d.applied != nil { // the master's apply of a character's damage
		out.Amount = d.applied.Amount
		if !v.hiddenFrom(d.applied.DeathHidden, target) { // a hit at 0 is a death save failure (RN-24)
			out.DeathFailuresAdded = d.applied.FailuresAdded
		}
		if d.applied.Overridden && v.master {
			rolled := d.applied.Rolled
			out.RolledAmount = &rolled
		}
		if dc := d.applied.ConcentrationDC; dc > 0 && (v.master || v.owns(target)) {
			out.ConcentrationDc = &dc
		}
		if after := d.applied.After; after != nil {
			out.TargetDown = !after.Defeated && after.HP == 0
		}
		return out
	}
	if after := d.hit.After; after != nil && d.status == playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_APPLIED {
		out.TargetDefeated = after.Defeated
		out.TargetDown = !after.Defeated && after.HP == 0
		if dc := d.hit.ConcentrationDC; dc > 0 && (v.master || v.owns(target)) {
			out.ConcentrationDc = &dc
		}
	}
	return out
}

// damage is the damage of an attack as the viewer gets it. The target's hit
// points after it are the master's alone, set on the entry; the concentration
// DC is the master's and the target's own player's (targetsOwn).
func (e *logEntry) damage(dice, master, targetsOwn bool, out *playv1.CombatLogEntry) *playv1.CombatLogDamage {
	d := &playv1.CombatLogDamage{Status: e.status}
	if e.dmg == nil {
		return d // the attack hit and its damage is not rolled yet
	}
	d.Amount, d.DamageTypeKey, d.DamageTypePt = e.dmg.Amount, e.dmg.DamageType, damageTypePT[e.dmg.DamageType]
	d.CriticalRule, d.CriticalMax = pendingCriticalRule(e.dmg.Critical, e.dmg.CriticalMaxRule), e.dmg.CriticalMax
	if dice {
		d.Roll = diceRoll(e.dmg.DiceCount, e.dmg.DiceSides, e.dmg.Faces, e.dmg.Modifier, e.dmg.Amount, e.dmg.Physical)
	}
	if ap := e.applied; ap != nil { // the master's apply of a character's damage
		d.Amount = ap.Amount
		if !ap.DeathHidden || targetsOwn { // a hit at 0 is a death save failure, which the table may keep to the owner and the master (RN-24)
			d.DeathFailuresAdded = ap.FailuresAdded
		}
		if ap.Overridden && master {
			rolled := ap.Rolled
			d.RolledAmount = &rolled
		}
		if dc := ap.ConcentrationDC; dc > 0 && targetsOwn {
			d.ConcentrationDc = &dc
		}
	}
	if after := e.dmg.After; after != nil && e.status == playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_APPLIED {
		d.TargetDefeated = after.Defeated
		d.TargetDown = !after.Defeated && after.HP == 0
		if master {
			out.HitPointsAfter = &after.HP
		}
	}
	if e.applied == nil && e.dmg.ConcentrationDC > 0 && targetsOwn && e.status == playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_APPLIED {
		dc := e.dmg.ConcentrationDC
		d.ConcentrationDc = &dc
	}
	return d
}

// trapEntry is the firing as the viewer gets it: everyone gets the trap (it is
// public now) and what it did, the master the DCs and armor classes too, and the
// dice are the master's and the target's own player's.
func (e *logEntry) trapEntry(ctx context.Context, v combatViewer, byID map[string]playdb.Combatant, names *keyNames) *playv1.TrapFiring {
	if e.ev.Trap == nil {
		return nil
	}
	tv := trapView{
		master: v.master,
		owns:   func(id string) bool { return v.owns(byID[id]) },
		hidden: func(id string) bool {
			if byID[id].Hidden {
				return true
			}
			i := slices.IndexFunc(e.ev.Trap.Caught, func(cc trapCaughtEvent) bool { return cc.Target == id })
			if i < 0 {
				return false
			}
			// Hidden when it fired stays hidden; on a fog map an NPC the firing caught is
			// the line of the players who saw it then.
			cc := e.ev.Trap.Caught[i]
			return cc.Hidden || (cc.Fogged && !slices.Contains(cc.SeenBy, v.userID))
		},
		label: func(id string) string { return byID[id].Label },
		status: func(pendingID string) (playv1.PendingDamageStatus, bool) {
			if dl, ok := e.pend[pendingID]; ok {
				return dl.status, true
			}
			return 0, false
		},
		amount: func(pendingID string) (int32, bool) {
			if dl, ok := e.pend[pendingID]; ok && dl.applied != nil {
				return dl.applied.Amount, true
			}
			return 0, false
		},
	}
	return firingProto(e.ev.Trap, e.id, names.trapName(ctx, e.ev.Trap.PointID), tv)
}

// trapName is the name of a trap that fired, "" when it was deleted since.
func (n *keyNames) trapName(ctx context.Context, pointID string) string {
	if n.s.traps == nil {
		return ""
	}
	if name, ok := n.traps[pointID]; ok {
		return name
	}
	found, err := n.s.traps.TrapNames(ctx, n.campaignID, []string{pointID})
	if err != nil {
		n.s.logger.WarnContext(ctx, "play: cannot read a trap's name for the combat log")
	}
	if n.traps == nil {
		n.traps = map[string]string{}
	}
	n.traps[pointID] = found[pointID]
	return found[pointID]
}

// keyNames finds the Portuguese names of the attacks and actions an entry
// mentions, from the actor's sheet, read once for each character.
type keyNames struct {
	s           *Service
	campaignID  string
	byCharacter map[string]link.Sheet
	traps       map[string]string         // the names of the traps that fired, by point
	content     func(key string) string   // the names of the content, from the campaign's content
	zones       map[string]playdb.MapZone // the combat's zones, the ended ones too, by id (zones_log.go)
}

// of returns the name of the key on the combatant's sheet, or "" when the
// combatant left the combat or the sheet no longer has it.
func (n *keyNames) of(ctx context.Context, c playdb.Combatant, key string) string {
	if c.CharacterID == "" || key == "" {
		return ""
	}
	sheet, ok := n.byCharacter[sheetKey(c)]
	if !ok {
		var err error
		if sheet, err = n.s.sheetOf(ctx, nil, n.campaignID, c); err != nil {
			n.s.logger.WarnContext(ctx, "play: cannot read a sheet for the combat log") // no names or IDs in logs
		}
		n.byCharacter[sheetKey(c)] = sheet
	}
	for _, a := range sheet.Attacks {
		if a.Key == key {
			return a.Name
		}
	}
	for _, a := range sheet.Actions {
		if a.Key == key {
			return a.Name
		}
	}
	for _, a := range sheet.FeatureActions {
		if a.Key == key {
			return a.Name
		}
	}
	if strings.HasPrefix(key, "spell:") && !isCreature(c) {
		if sp, err := n.s.roster.CombatSpell(ctx, nil, n.campaignID, c.CharacterID, key, 0, ""); err == nil {
			return sp.Name
		}
	}
	return ""
}

// contentName is the name of a content key in the campaign's content, "" when it
// cannot be read (the log line still shows the key).
func (n *keyNames) contentName(ctx context.Context, key string) string {
	if n.content == nil { // read once for the whole log
		n.content = n.s.namesFor(ctx, n.campaignID)
	}
	return n.content(key)
}

// wildShapeEndReasonProto reads the reason a wild_shape_ended event gives.
var wildShapeEndReasonProto = map[string]playv1.WildShapeEndReason{
	endedByLeaving: playv1.WildShapeEndReason_WILD_SHAPE_END_REASON_LEFT,
	endedByDamage:  playv1.WildShapeEndReason_WILD_SHAPE_END_REASON_DAMAGE,
	endedByMaster:  playv1.WildShapeEndReason_WILD_SHAPE_END_REASON_MASTER,
	endedAtZero:    playv1.WildShapeEndReason_WILD_SHAPE_END_REASON_ZERO_HP,
	endedAsleep:    playv1.WildShapeEndReason_WILD_SHAPE_END_REASON_UNCONSCIOUS,
}
