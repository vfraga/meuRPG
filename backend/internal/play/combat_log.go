package play

import (
	"context"
	"encoding/json"
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
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/reaction"
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
	// parried are the pending damages a Parry stopped: a miss for everyone, and the master's line alone says why.
	parried []string
	// returned and returnBlocked: the opportunity attack's damage took the mover to
	// 0 hit points, and it went back to the square it left the reach at (or could
	// not, because the square was taken: the master's line says so).
	returned, returnBlocked bool
	// actorUser is who made the first event of the line (the master's hand when he acted for a player).
	actorUser *string
	// masterOnly is a line only the master gets, whatever the combatants.
	masterOnly bool
	// gear is the swap of a GEAR_SWAPPED line: its payload is its own type (equip_event.go).
	gear gearSwapEvent
	// shapeStarted: a Wild Shape line is the druid becoming the beast, not leaving it.
	shapeStarted bool
	// seq is the highest session_events.seq among the events in hosts: a later event that joins the entry raises it.
	seq int64
	// hosts are the events the entry shows: an attack's own and the ones of its
	// damage, so the entry of the last action is the one to undo.
	hosts []string
	// removed is the master's removal of an extra of the damage, partLabel the extra's
	// name on a line of the removal, rolls what the log reads of the combat's own tables.
	removed   *actionEvent
	partLabel string
	rolls     *logRolls
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

// logSnapshot is what ListCombatLog reads of the database, in one transaction.
type logSnapshot struct {
	enc    playdb.Encounter
	events []playdb.ListEncounterEventsRow
	cs     []playdb.Combatant
	recent []playdb.ListRecentSessionEventsRow
	// the reasons and the requests the events point at
	reasons  []playdb.CombatReason
	requests []playdb.RollModeRequest
}

// readLogSnapshot reads the combat's events, its combatants and the master's undo marker
// as one snapshot: a log line whose actor is missing, or an "undo" that
// points at an event the list does not have, would be a torn read.
func (s *Service) readLogSnapshot(ctx context.Context, m authz.Membership, encID string) (logSnapshot, error) {
	var snap logSnapshot
	err := db.ReadTx(ctx, s.pool, func(tx pgx.Tx) error {
		return snap.read(ctx, s.queries.WithTx(tx), m, encID)
	})
	return snap, err
}

// read fills the snapshot from the queries of the transaction it is read in.
func (snap *logSnapshot) read(ctx context.Context, q *playdb.Queries, m authz.Membership, encID string) error {
	session, err := openSessionWith(ctx, q, m.CampaignID)
	if err != nil {
		return err
	}
	if snap.enc, err = encounterInSessionWith(ctx, q, session.ID, encID); err != nil {
		return err
	}
	if snap.events, err = q.ListEncounterEvents(ctx, playdb.ListEncounterEventsParams{EncounterID: &snap.enc.ID, Limit: logEventLimit}); err != nil {
		return fmt.Errorf("list the combat's events: %w", err)
	}
	if snap.reasons, err = q.ListEncounterCombatReasons(ctx, snap.enc.ID); err != nil {
		return fmt.Errorf("list the reasons: %w", err)
	}
	if snap.requests, err = q.ListEncounterRollModeRequests(ctx, snap.enc.ID); err != nil {
		return fmt.Errorf("list the requests: %w", err)
	}
	// With the dismissed creatures: their lines survive a concentration ending.
	if snap.cs, err = q.ListCombatantsWithDismissed(ctx, snap.enc.ID); err != nil {
		return fmt.Errorf("list the combatants: %w", err)
	}
	if m.Role == authz.RoleMaster {
		if snap.recent, err = q.ListRecentSessionEvents(ctx, playdb.ListRecentSessionEventsParams{GameSessionID: session.ID, Limit: recentEvents}); err != nil {
			return fmt.Errorf("read the latest events: %w", err)
		}
	}
	return nil
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
	snap, err := s.readLogSnapshot(ctx, m, encID)
	if err != nil {
		return nil, s.dbError(ctx, "read the combat log", err)
	}
	// The log's lines are filtered by who could see them when they happened
	// (actionEvent.SeenBy), not by what the viewer sees now; only the current
	// rules about hidden combatants are worked out from the combatants as they are.
	v, err := s.logViewer(ctx, m)
	if err != nil {
		return nil, err
	}
	entries := s.logEntries(ctx, m, &snap)
	res := &playv1.ListCombatLogResponse{}
	stampSeqs(entries, snap.events, v)
	since := sinceSeqOf(req.Msg.GetSinceSeq(), v, snap.events, entries, snap.cs)
	res.Full = since == 0
	if !v.master {
		res.LastSeq = countedSeq(v, snap.events, entries, snap.cs)
	}
	if v.master {
		res.LastSeq = lastSeqOf(snap.events)
	}
	var lastID string
	if v.master {
		if last, ok := lastAction(snap.recent, snap.enc.ID); ok {
			lastID = last.ID
			res.UndoableEventId = last.ID
		}
	}

	names, byID := s.logNames(m, &snap)
	cut := logCut{since: since, lastID: lastID}
	res.Rounds, res.LastSeq = cut.rounds(ctx, entries, v, byID, names, res.LastSeq)
	return connect.NewResponse(res), nil
}

// logEntries are the entries of the snapshot's combat, oldest first, with the rolls they show. The snapshot's events are turned
// around in place.
func (s *Service) logEntries(ctx context.Context, m authz.Membership, snap *logSnapshot) []*logEntry {
	slices.Reverse(snap.events) // oldest first, as buildLog reads them
	entries := buildLog(snap.events)
	rolls := newLogRolls(snap.reasons, snap.requests, s.namesFor(ctx, m.CampaignID))
	for _, e := range entries {
		e.rolls = rolls
	}
	return entries
}

// logNames are the names and the combatants a line is worked out from.
func (s *Service) logNames(m authz.Membership, snap *logSnapshot) (*keyNames, map[string]playdb.Combatant) {
	names := &keyNames{s: s, campaignID: m.CampaignID, byCharacter: map[string]link.Sheet{}}
	byID := make(map[string]playdb.Combatant, len(snap.cs))
	for _, c := range snap.cs {
		byID[c.ID] = c
	}
	return names, byID
}

// logViews are the lines of the snapshot's combat that the viewer reads, latest first. The snapshot's events
// are turned around in place.
func (s *Service) logViews(ctx context.Context, m authz.Membership, snap *logSnapshot, v combatViewer, lastID string) []*playv1.CombatLogEntry {
	entries := s.logEntries(ctx, m, snap)
	names, byID := s.logNames(m, snap)
	var out []*playv1.CombatLogEntry
	for _, e := range slices.Backward(entries) {
		if view, ok := e.view(ctx, v, byID, names, lastID); ok {
			out = append(out, view)
		}
	}
	return out
}

// logCut is the part of the log an answer holds: the entries above since (0: all of them), and the master's undoable one.
type logCut struct {
	since  int64
	lastID string
}

// rounds turns the entries, latest first, into the rounds the viewer reads, leaving out the entries the app holds. It returns them and
// the answer's last_seq: for the master the number it was given, for a player the highest number among the entries they read.
func (c logCut) rounds(ctx context.Context, entries []*logEntry, v combatViewer, byID map[string]playdb.Combatant, names *keyNames, lastSeq int64) ([]*playv1.CombatLogRound, int64) {
	var rounds []*playv1.CombatLogRound
	for _, e := range slices.Backward(entries) {
		if v.master && c.since > 0 && e.seq <= c.since && !slices.Contains(e.hosts, c.lastID) {
			continue // the app holds this line as it is (the master's undoable line is always sent: its flag moves with the log's end)
		}
		out, ok := e.view(ctx, v, byID, names, c.lastID)
		if !ok {
			continue
		}
		if !v.master {
			lastSeq = max(lastSeq, e.seq)
			if c.since > 0 && e.seq <= c.since {
				continue
			}
		}
		if v.master { // a player's entries carry no number: the gaps where a hidden event sits would show it
			out.Seq = e.seq
		}
		rounds = appendToRound(rounds, out)
	}
	return rounds, lastSeq
}

// deltaSafeKinds are the events that only add a line to the log or join the entry they belong to (and name it in hosts): after
// the app holds the log up to a number, they change nothing the app already holds, so ListCombatLog may answer with the new and the
// changed entries alone. Any other event after the app's number (a combatant hidden or shown, an answer to a roll request, a creature
// summoned or dismissed, an undo, a kind this list does not know) can change how an entry the app holds is seen without a new
// event on that entry, so the answer is the whole log. When in doubt a kind stays off the list.
var deltaSafeKinds = map[string]bool{
	eventEncounterStarted: true, eventInitiativeSubmitted: true, eventInitiativeOrderSet: true, eventCombatBegun: true,
	eventTurnEnded: true, eventTurnPartEnded: true, eventCombatantMoved: true, eventEncounterEnded: true, eventCombatantMounted: true, eventCombatantDismounted: true,
	eventAttackRolled: true, eventDamageRolled: true, eventDamageApplied: true, eventDamageDiscarded: true,
	eventActionTaken: true, eventHitPointsAdjusted: true, eventDeathSaveRolled: true, eventDeathConfirmed: true,
	eventConditionsSet: true, eventSpellCast: true, eventReactionUsed: true, eventReactionDeclined: true,
	eventLegendarySpent: true, eventRechargeRolled: true, eventFeatureUsed: true,
}

// sinceSeqOf is the number the log is filtered by: sinceSeq, or 0 (the whole log) when an event after it may have changed an
// entry the app holds. The master hears of every such event. A player hears only of what they may see, so the answer tells them
// nothing a secret or unseen write did:
//   - an event the player does not hear of (Secret, or off the fog's list of who saw it) is ignored;
//   - an undo counts only when the action it took back is one the player may see (not Secret, seen), not when the undo itself is;
//   - a combatant toggle (shown, hidden, summoned, dismissed, removed) counts only when the combatant is in a line the player holds:
//     one at or below sinceSeq that is not Secret, not the master's alone and seen. Lines that stay secret stay invisible after a
//     reveal, so only held lines can appear or go;
//   - an add whose combatants all came in hidden (the flag is in the payload, not in Secret: addedAllHidden) is ignored; an add
//     with at least one visible combatant counts;
//   - any other kind off deltaSafeKinds counts when the player may see it.
//
// An event that cannot be read is ignored for a player (fail closed: they are told nothing from what the server cannot read) and
// counted for the master (when in doubt, the whole log).
// The publishing of the undo (publishUndoRun, players = !ev.Secret) still tells a player of an undo of a fogged action it did not see
// with a hint; the answer to their read is what this function decides.
func sinceSeqOf(sinceSeq int64, v combatViewer, events []playdb.ListEncounterEventsRow, entries []*logEntry, cs []playdb.Combatant) int64 {
	if sinceSeq <= 0 {
		return 0
	}
	for _, e := range events {
		if int64(e.Seq) > sinceSeq && changesHeldView(e, sinceSeq, v, events, entries, cs) {
			return 0
		}
	}
	return sinceSeq
}

// countedSeq is the highest number among the events a player's answer would ask for the whole log for, wherever the app's number
// stands (a toggle counts when a line they hold, below it, names the combatant). A player's last_seq reaches it, so the next read, from
// that number, does not meet the same event again and ask for the whole log once more; and it does not fall back when no new event
// came. Ignored events stay out of it, so a combatant shown again comes back as a new line.
func countedSeq(v combatViewer, events []playdb.ListEncounterEventsRow, entries []*logEntry, cs []playdb.Combatant) int64 {
	var counted int64
	for _, e := range events {
		if changesHeldView(e, int64(e.Seq)-1, v, events, entries, cs) {
			counted = max(counted, int64(e.Seq))
		}
	}
	return counted
}

// changesHeldView says whether the event, after the app's number, may change an entry the viewer holds.
func changesHeldView(e playdb.ListEncounterEventsRow, sinceSeq int64, v combatViewer, events []playdb.ListEncounterEventsRow, entries []*logEntry, cs []playdb.Combatant) bool {
	if e.Kind != eventActionUndone && deltaSafeKinds[e.Kind] {
		return false
	}
	ev, err := readEvent(e.Payload)
	if v.master {
		return true
	}
	if err != nil {
		return false
	}
	switch {
	case e.Kind == eventActionUndone:
		return undoneMaySeen(ev, v, events)
	case !ev.heardBy(v):
		return false
	case e.Kind == eventCombatantsAdded && addedAllHidden(ev, e.Payload):
		return false // every combatant added was hidden: no player has a view to change
	case toggleKinds[e.Kind]:
		return heldLineHas(ev.toggled(cs), sinceSeq, v, entries)
	}
	return true
}

// addedAllHidden says every combatant a combatants_added event put in came in hidden, and there was one at least. The flag is in
// the payload, not in Secret: AddMonsters keeps one for all the monsters of the event, AddCombatants counts the hidden_added ones beside
// the added ones. A mixed add counts (a player may see some of it), and so does an event that says nothing of hiding.
func addedAllHidden(ev actionEvent, payload []byte) bool {
	if ev.Monsters != nil {
		return ev.Monsters.Hidden
	}
	var n struct {
		Added       int `json:"added"`
		HiddenAdded int `json:"hidden_added"`
	}
	return json.Unmarshal(payload, &n) == nil && n.Added > 0 && n.HiddenAdded == n.Added
}

// toggleKinds are the events that show, hide, add or take away a combatant.
var toggleKinds = map[string]bool{
	eventCombatantHiddenSet: true, eventCreatureSummoned: true, eventCreatureDismissed: true, eventCombatantRemoved: true,
}

// undoneMaySeen says the viewer may see one of the actions an undo took back.
func undoneMaySeen(undo actionEvent, v combatViewer, events []playdb.ListEncounterEventsRow) bool {
	ids := append([]string{undo.Undone}, undo.UndoneAlso...)
	for _, e := range events {
		if !slices.Contains(ids, e.ID) {
			continue
		}
		if ev, err := readEvent(e.Payload); err == nil && ev.heardBy(v) {
			return true
		}
	}
	return false
}

// toggled are the combatants a toggle event is about: the one it names, the target, and the combatants of the creatures it made or dismissed.
func (ev actionEvent) toggled(cs []playdb.Combatant) []string {
	ids := []string{ev.Actor, ev.Target}
	for _, c := range cs {
		if c.CreatureID != nil && slices.Contains(ev.Created, *c.CreatureID) {
			ids = append(ids, c.ID)
		}
	}
	return slices.DeleteFunc(ids, func(id string) bool { return id == "" })
}

// heldLineHas says a line the viewer holds (at or below sinceSeq, not Secret, not the master's alone, seen) names one of the combatants.
func heldLineHas(ids []string, sinceSeq int64, v combatViewer, entries []*logEntry) bool {
	for _, e := range entries {
		if e.seq > sinceSeq || e.masterOnly || e.ev.Secret || !e.ev.seenByViewer(v) {
			continue
		}
		if slices.ContainsFunc(ids, func(id string) bool { return e.names(id, v) }) {
			return true
		}
	}
	return false
}

// heardBy says the viewer hears of the event: the master of all of them; a player of the ones that are not Secret, that the fog
// shows them, and that give them something to read (a firing that only adds creatures the player is not given changes no line of
// theirs).
func (ev actionEvent) heardBy(v combatViewer) bool {
	return v.master || (!ev.Secret && ev.seenByViewer(v) && !ev.Trap.addsNothingFor(v))
}

// names says the entry has the combatant as its actor, its target or one of its hits, or as a creature of a firing the viewer is given. These are the only fields the visibility rules
// read today; a rule that reads another combatant of the line (an area's caught list, a rider's target) must extend it.
func (e *logEntry) names(id string, v combatViewer) bool {
	if e.ev.Actor == id || e.ev.Target == id {
		return true
	}
	// A creature a firing caught: its being shown or hidden changes the line, but only for a viewer the line gives it to. One hidden
	// when the trap fired, or that this viewer did not see, is not in their line, and a toggle of it must tell them nothing.
	if e.ev.Trap != nil && slices.ContainsFunc(e.ev.Trap.Caught, func(cc trapCaughtEvent) bool { return cc.Target == id && !cc.hiddenFrom(v) }) {
		return true
	}
	return slices.ContainsFunc(e.ev.Hits, func(h castHit) bool { return h.Target == id })
}

// lastSeqOf is the highest event number the combat's events reach, for the master. A player's is the highest entry number among the
// entries the player reads (see ListCombatLog), so it does not move on a write the player may not see.
func lastSeqOf(events []playdb.ListEncounterEventsRow) int64 {
	var last int64
	for _, e := range events {
		last = max(last, int64(e.Seq))
	}
	return last
}

// stampSeqs sets each entry's seq: the highest number among the events it shows. A player's counts the entry's own event (the line
// is theirs or it is not shown at all: a reactor's own player gets the line of a reaction the fog or Secret hides from the table) and,
// of the events that join it, only the ones the player hears of (not Secret, seen, not an addition of creatures they are not given):
// an event that joins a line the player holds without being told to them (the damage a master applies to a hidden target of a cast
// they see, a hidden NPC added to a firing they saw) moves nothing they read, so it cannot move the number of that line, nor the
// number of their answer.
func stampSeqs(entries []*logEntry, events []playdb.ListEncounterEventsRow, v combatViewer) {
	heard := heardJoined(entries, events, v)
	seqOf := make(map[string]int64, len(events))
	for _, e := range events {
		seqOf[e.ID] = int64(e.Seq)
	}
	for _, entry := range entries {
		for i, id := range entry.hosts {
			if i == 0 || v.master || heard[id] {
				entry.seq = max(entry.seq, seqOf[id])
			}
		}
	}
}

// heardJoined are the events that join an entry (not its first) which the player hears of. Only those payloads are read, and none
// for the master, who hears of all.
func heardJoined(entries []*logEntry, events []playdb.ListEncounterEventsRow, v combatViewer) map[string]bool {
	heard := map[string]bool{}
	if v.master {
		return heard
	}
	joined := map[string]bool{}
	for _, entry := range entries {
		for _, id := range entry.hosts[min(1, len(entry.hosts)):] {
			joined[id] = true
		}
	}
	for _, e := range events {
		if joined[e.ID] {
			ev, err := readEvent(e.Payload)
			heard[e.ID] = err == nil && ev.heardBy(v)
		}
	}
	return heard
}

// logViewer is who reads the log: the member, and for a player the table's rule on the
// death saves.
func (s *Service) logViewer(ctx context.Context, m authz.Membership) (combatViewer, error) {
	v := viewerOf(m)
	if !v.master {
		rules, err := s.tableRules(ctx, nil, m.CampaignID)
		if err != nil {
			return v, s.dbError(ctx, "read the table's rules", err)
		}
		v.hideDeath, v.showSaveDC = rules.DeathSavesHidden, rules.ShowSaveDC
	}
	return v, nil
}

// appendToRound puts a line in the group of its round: the last group when it is the
// round's, else a new one.
func appendToRound(rounds []*playv1.CombatLogRound, out *playv1.CombatLogEntry) []*playv1.CombatLogRound {
	if n := len(rounds); n == 0 || rounds[n-1].Round != out.Round {
		rounds = append(rounds, &playv1.CombatLogRound{Round: out.Round})
	}
	last := rounds[len(rounds)-1]
	last.Entries = append(last.Entries, out)
	return rounds
}

// buildLog turns a combat's events, in order, into entries. An event an undo
// took back leaves no trace; the events of one attack (its roll, its damage,
// the master's apply or discard, a reaction) merge into one entry, and those of
// a spell (its cast, the damage rolls and applies of its targets) into one.
func buildLog(events []playdb.ListEncounterEventsRow) []*logEntry {
	undone := undoneEvents(events)
	b := &logBuilder{
		byPending: map[string]*logEntry{},
		byFiring:  map[string]*logEntry{},
		byMove:    map[string]*logEntry{},
		moveOf:    map[string]string{},
	}
	var out []*logEntry
	for _, e := range events {
		if undone[e.ID] || e.Kind == eventActionUndone {
			continue
		}
		if entry := b.entry(e); entry != nil {
			out = append(out, entry)
		}
	}
	return out
}

// undoneEvents are the ids of the events an undo took back.
func undoneEvents(events []playdb.ListEncounterEventsRow) map[string]bool {
	undone := map[string]bool{}
	for _, e := range events {
		if e.Kind != eventActionUndone {
			continue
		}
		markUndone(undone, e.Payload)
	}
	return undone
}

// logBuilder is what buildLog remembers while it reads the events in order: where the
// pending damages, the trap firings and the moves that made offers were written, so that
// a later event lands in the entry of the one it belongs to.
type logBuilder struct {
	byPending map[string]*logEntry
	byFiring  map[string]*logEntry
	// An opportunity offer's answer without an attack (a decline, a skip) is no
	// line, but the master's undo can take it back: it belongs to the entry of
	// the move that made the offer, so that entry is the one to undo.
	byMove map[string]*logEntry
	moveOf map[string]string // offer id -> move id
}

// logHandler fills the entry of an event of one kind and says whether the event is a line
// of its own; the events that are not may still land in the entry of another.
type logHandler func(b *logBuilder, e playdb.ListEncounterEventsRow, entry *logEntry) bool

// logHandlers are the events that make a line or join one, by event kind; the others
// (initiative, turns, reinforcements) are not lines of the log.
var logHandlers = map[string]logHandler{
	eventCombatBegun:             plainLine(playv1.CombatLogKind_COMBAT_LOG_KIND_COMBAT_BEGUN),
	eventEncounterEnded:          plainLine(playv1.CombatLogKind_COMBAT_LOG_KIND_COMBAT_ENDED),
	eventCombatantMoved:          (*logBuilder).moved,
	eventCombatantMounted:        plainLine(playv1.CombatLogKind_COMBAT_LOG_KIND_MOUNTED),
	eventCombatantDismounted:     plainLine(playv1.CombatLogKind_COMBAT_LOG_KIND_DISMOUNTED),
	eventDoorOpened:              plainLine(playv1.CombatLogKind_COMBAT_LOG_KIND_DOOR_OPENED),
	eventCombatantsAdded:         (*logBuilder).monstersAdded,
	eventLegendarySpent:          masterLine(playv1.CombatLogKind_COMBAT_LOG_KIND_LEGENDARY_SPENT),
	eventRechargeRolled:          masterLine(playv1.CombatLogKind_COMBAT_LOG_KIND_RECHARGE_ROLLED),
	eventGearSwapped:             gearSwappedLine,
	eventFeatureUsed:             featureUsedLine,
	eventCombatantHiddenSet:      (*logBuilder).hiddenSet,
	eventActionTaken:             (*logBuilder).actionTaken,
	eventHitPointsAdjusted:       masterLine(playv1.CombatLogKind_COMBAT_LOG_KIND_HIT_POINTS_ADJUSTED),
	eventDeathSaveRolled:         plainLine(playv1.CombatLogKind_COMBAT_LOG_KIND_DEATH_SAVE),
	eventDeathConfirmed:          plainLine(playv1.CombatLogKind_COMBAT_LOG_KIND_DEATH_CONFIRMED),
	eventCharacterRevived:        plainLine(playv1.CombatLogKind_COMBAT_LOG_KIND_CHARACTER_REVIVED),
	eventConditionsSet:           plainLine(playv1.CombatLogKind_COMBAT_LOG_KIND_CONDITIONS_CHANGED),
	eventCombatEffectEnded:       plainLine(playv1.CombatLogKind_COMBAT_LOG_KIND_EFFECT_ENDED),
	eventTurnPartEnded:           plainLine(playv1.CombatLogKind_COMBAT_LOG_KIND_TURN_PART_ENDED),
	eventAttackRolled:            (*logBuilder).attackRolled,
	eventSpellCast:               (*logBuilder).spellCast,
	eventTrapTriggered:           (*logBuilder).trapTriggered,
	eventRollModeAnswered:        (*logBuilder).rollModeAnswered,
	eventLastingAdded:            effectLine,
	eventLastingChanged:          effectLine,
	eventLastingEnded:            effectLine,
	eventLastingSaved:            effectLine,
	eventLastingTriggered:        effectLine,
	eventZoneTriggered:           effectLine,
	eventZoneMoved:               effectLine,
	eventLastingVisibility:       effectLine,
	eventExhaustion:              effectLine,
	eventHitRider:                (*logBuilder).hitRider,
	eventStateChanged:            plainLine(playv1.CombatLogKind_COMBAT_LOG_KIND_STATE_CHANGED),
	eventDamagePartRemoved:       (*logBuilder).partRemoved,
	eventTrapSearched:            (*logBuilder).trapSearched,
	eventContestStarted:          contestLine,
	eventContestResolved:         contestLine,
	eventContestClosed:           contestLine,
	eventShoveResolved:           contestLine,
	eventGrappleReleased:         contestLine,
	eventHideAttempted:           contestLine,
	eventHideResolved:            contestLine,
	eventHelpGiven:               contestLine,
	eventWildShapeStarted:        (*logBuilder).wildShape,
	eventWildShapeEnded:          (*logBuilder).wildShape,
	eventReactionUsed:            (*logBuilder).reactionUsed,
	eventOpportunityOffered:      (*logBuilder).opportunityOffered,
	eventReactionAnswered:        (*logBuilder).reactionAnswered,
	eventConcentrationSaveRolled: (*logBuilder).reactionAnswered,
	eventReactionDeclined:        (*logBuilder).reactionDeclined,
	eventDamageRolled:            (*logBuilder).damageEvent,
	eventDamageApplied:           (*logBuilder).damageEvent,
	eventDamageDiscarded:         (*logBuilder).damageEvent,
}

// entry reads one event: the entry it makes, or nil when it makes none.
func (b *logBuilder) entry(e playdb.ListEncounterEventsRow) *logEntry {
	ev, err := readEvent(e.Payload)
	if err != nil {
		return nil // never: this package wrote it
	}
	// The log starts when the combat begins: the setup is not in it. The master's line of
	// the monsters he put in (their rolled hit points) is the exception: it is his from the setup.
	if ev.Round == 0 && (e.Kind != eventCombatantsAdded || ev.Monsters == nil) {
		return nil
	}
	if ev.Reaction != nil && ev.Reaction.Hold {
		return nil // the event of a held action stands for nothing yet: its replay is the line
	}
	entry := &logEntry{id: e.ID, at: e.CreatedAt, ev: ev, hosts: []string{e.ID}, actorUser: e.ActorUserID}
	handle, ok := logHandlers[e.Kind]
	if !ok || !handle(b, e, entry) {
		return nil
	}
	return entry
}

// plainLine is the handler of an event that is a line of the kind, for everybody.
func plainLine(kind playv1.CombatLogKind) logHandler {
	return func(_ *logBuilder, _ playdb.ListEncounterEventsRow, entry *logEntry) bool {
		entry.kind = kind
		return true
	}
}

// masterLine is the handler of an event that is a line of the kind for the master alone.
func masterLine(kind playv1.CombatLogKind) logHandler {
	return func(_ *logBuilder, _ playdb.ListEncounterEventsRow, entry *logEntry) bool {
		entry.kind, entry.masterOnly = kind, true
		return true
	}
}

// gearSwappedLine is the handler of a gear swap: a line of the master's and of the character's owner (visibility), with the
// swap read from the event's own payload.
func gearSwappedLine(_ *logBuilder, e playdb.ListEncounterEventsRow, entry *logEntry) bool {
	gear, err := readGearSwap(e.Payload)
	if err != nil {
		return false
	}
	entry.kind, entry.masterOnly, entry.gear = playv1.CombatLogKind_COMBAT_LOG_KIND_GEAR_SWAPPED, true, gear
	return true
}

// effectLine is the handler of the events of the effects that last.
func effectLine(_ *logBuilder, e playdb.ListEncounterEventsRow, entry *logEntry) bool {
	entry.kind = effectLogKinds[e.Kind]
	return true
}

// contestLine is the handler of grapple, shove, escape, Hide and Help (combat_contests.go):
// a line when the event says one.
func contestLine(_ *logBuilder, _ playdb.ListEncounterEventsRow, entry *logEntry) bool {
	return contestLogEntry(entry, entry.ev)
}

func (b *logBuilder) moved(_ playdb.ListEncounterEventsRow, entry *logEntry) bool {
	ev := entry.ev
	if ev.Contest != nil && ev.Contest.Line == contestLineStoodUp { // "se levantou" (combat_standup.go)
		return contestLogEntry(entry, ev)
	}
	if (!ev.OnTurn && ev.SpendCost == 0) || (ev.DistanceFt == 0 && ev.DistanceDFt == 0 && ev.Jump != jumpHigh) {
		return false // placing a token is not a move of the fight
	}
	entry.kind = playv1.CombatLogKind_COMBAT_LOG_KIND_MOVED
	if ev.MoveID != "" {
		b.byMove[ev.MoveID] = entry
	}
	return true
}

func (b *logBuilder) monstersAdded(_ playdb.ListEncounterEventsRow, entry *logEntry) bool {
	if entry.ev.Monsters == nil {
		return false // reinforcements (AddCombatants) are no line
	}
	entry.kind, entry.masterOnly = playv1.CombatLogKind_COMBAT_LOG_KIND_MONSTERS_ADDED, true
	return true
}

func (b *logBuilder) hiddenSet(_ playdb.ListEncounterEventsRow, entry *logEntry) bool {
	// The master's own reveal is his alone; the one an area spell made is the
	// players' line too ("foi revelado"): the creature appears on their map.
	entry.kind, entry.masterOnly = playv1.CombatLogKind_COMBAT_LOG_KIND_REVEAL_CHANGED, !entry.ev.ByArea
	return true
}

func (b *logBuilder) actionTaken(_ playdb.ListEncounterEventsRow, entry *logEntry) bool {
	entry.kind = playv1.CombatLogKind_COMBAT_LOG_KIND_ACTION
	if entry.ev.Res != nil {
		entry.kind = playv1.CombatLogKind_COMBAT_LOG_KIND_RESOURCE
	}
	return true
}

func (b *logBuilder) attackRolled(_ playdb.ListEncounterEventsRow, entry *logEntry) bool {
	entry.kind = playv1.CombatLogKind_COMBAT_LOG_KIND_ATTACK
	if entry.ev.Pending != "" {
		entry.status = playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_AWAITING_ROLL
		b.byPending[entry.ev.Pending] = entry
	}
	return true
}

func (b *logBuilder) spellCast(_ playdb.ListEncounterEventsRow, entry *logEntry) bool {
	entry.kind = playv1.CombatLogKind_COMBAT_LOG_KIND_SPELL_CAST
	entry.pend = map[string]*damageLog{}
	for _, h := range entry.ev.Hits {
		for _, id := range append([]string{h.Pending}, h.More...) {
			if id != "" {
				entry.pend[id] = &damageLog{status: playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_AWAITING_ROLL}
				b.byPending[id] = entry
			}
		}
	}
	return true
}

// hitRider is an answered rider: the damage a creature's rider opened (a ghoul's poison) is hosted by its event, as a
// cast hosts the damage of its targets, so the roll and the apply that follow land on its line.
func (b *logBuilder) hitRider(_ playdb.ListEncounterEventsRow, entry *logEntry) bool {
	entry.kind = playv1.CombatLogKind_COMBAT_LOG_KIND_HIT_RIDER
	r := entry.ev.Rider
	if r == nil || r.Pending == "" {
		return true
	}
	entry.pend = map[string]*damageLog{}
	for _, id := range append([]string{r.Pending}, r.More...) {
		entry.pend[id] = &damageLog{status: playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_AWAITING_ROLL}
		b.byPending[id] = entry
	}
	return true
}

// trapTriggered is a trap firing, or the master adding creatures to one.
func (b *logBuilder) trapTriggered(e playdb.ListEncounterEventsRow, entry *logEntry) bool {
	ev := entry.ev
	if ev.Trap != nil && ev.Trap.ExtendsID != "" {
		// The master added creatures to a firing: they join its entry, and their damage
		// moves on with the master's apply like the rest.
		if host, ok := b.byFiring[ev.Trap.ExtendsID]; ok && host.ev.Trap != nil {
			host.ev.Trap.Caught = append(host.ev.Trap.Caught, ev.Trap.Caught...)
			host.hosts = append(host.hosts, e.ID)
			b.registerTrapDamages(host, ev.Trap.Caught)
		}
		return false
	}
	// A trap fired (MR-035): the entry holds its pending damages, which the
	// master's apply or discard moves on like a spell's.
	entry.kind = playv1.CombatLogKind_COMBAT_LOG_KIND_TRAP_TRIGGERED
	entry.pend = map[string]*damageLog{}
	b.byFiring[e.ID] = entry
	if ev.Trap != nil {
		b.registerTrapDamages(entry, ev.Trap.Caught)
	}
	return true
}

// registerTrapDamages puts the pending damages of the creatures a trap caught on the entry.
func (b *logBuilder) registerTrapDamages(host *logEntry, caught []trapCaughtEvent) {
	for _, cc := range caught {
		for _, d := range cc.Damages {
			if d.Pending == "" {
				continue
			}
			status := playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_ROLLED
			if d.Applied {
				status = playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_APPLIED
			}
			host.pend[d.Pending] = &damageLog{status: status}
			b.byPending[d.Pending] = host
		}
	}
}

func (b *logBuilder) rollModeAnswered(_ playdb.ListEncounterEventsRow, entry *logEntry) bool {
	if entry.ev.Canceled {
		return false // a request taken back is no line
	}
	entry.kind = playv1.CombatLogKind_COMBAT_LOG_KIND_ROLL_MODE_ANSWERED
	return true
}

func (b *logBuilder) partRemoved(e playdb.ListEncounterEventsRow, entry *logEntry) bool {
	// The removal belongs to the damage it took the extra out of, and is a line of its own.
	ev := entry.ev
	entry.kind = playv1.CombatLogKind_COMBAT_LOG_KIND_DAMAGE_PART_REMOVED
	host, ok := b.byPending[ev.Pending]
	if !ok {
		return true
	}
	host.hosts = append(host.hosts, e.ID)
	host.removed = &ev
	if host.dmg != nil {
		for _, r := range host.dmg.Parts {
			if r.Key == ev.Key {
				entry.partLabel = r.Label
			}
		}
		if ev.After != nil {
			host.dmg.After = ev.After
		}
	}
	return true
}

func (b *logBuilder) trapSearched(_ playdb.ListEncounterEventsRow, entry *logEntry) bool {
	// A search is the Search action: the master's line only, with the roll in
	// his history; the table sees nothing of it (RN-10).
	entry.kind, entry.masterOnly = playv1.CombatLogKind_COMBAT_LOG_KIND_ACTION, true
	entry.ev.Key = "standard:search"
	return true
}

func (b *logBuilder) wildShape(e playdb.ListEncounterEventsRow, entry *logEntry) bool {
	entry.kind = playv1.CombatLogKind_COMBAT_LOG_KIND_WILD_SHAPE
	entry.shapeStarted = e.Kind == eventWildShapeStarted
	return true
}

func (b *logBuilder) reactionUsed(e playdb.ListEncounterEventsRow, entry *logEntry) bool {
	ev := entry.ev
	entry.kind = playv1.CombatLogKind_COMBAT_LOG_KIND_REACTION
	parry := ev.Reaction != nil && ev.Reaction.Kind == string(reaction.ParryKind)
	if parry {
		entry.kind = playv1.CombatLogKind_COMBAT_LOG_KIND_REACTION_WINDOW // the master's line (reactionLiners); the players get none
	}
	if ev.Reaction != nil && ev.Reaction.Kind == string(reaction.SlowFall) {
		entry.kind = playv1.CombatLogKind_COMBAT_LOG_KIND_REACTION_WINDOW // one line for each reader (reactionLiners)
	}
	// Escudo that stopped the attack takes its damage away; the hit waits for
	// its roll otherwise, which the roll event says. A Parry does the same, without saying why to a player.
	if host, ok := b.byPending[ev.Pending]; ok {
		host.hosts = append(host.hosts, e.ID)
		switch {
		case ev.Stopped && parry:
			host.setStatus(ev.Pending, playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_DISCARDED)
			host.parried = append(host.parried, ev.Pending)
		case ev.Stopped:
			host.setStatus(ev.Pending, playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_DISCARDED)
			host.stopped = append(host.stopped, ev.Pending)
		}
	}
	for _, h := range ev.AlsoStopped {
		if host, ok := b.byPending[h.Pending]; ok {
			host.hosts = append(host.hosts, e.ID)
			host.setStatus(h.Pending, playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_DISCARDED)
			host.stopped = append(host.stopped, h.Pending)
		}
	}
	return true
}

func (b *logBuilder) opportunityOffered(_ playdb.ListEncounterEventsRow, entry *logEntry) bool {
	ev := entry.ev
	b.moveOf[ev.OfferID] = ev.MoveID
	if !ev.ByHand {
		return false // a move's offer is no line of its own: the move's is
	}
	// The master's offer in a combat without a map is a line, and its answer
	// (decline, skip, withdraw) belongs to it, so the answer can be undone.
	entry.kind = playv1.CombatLogKind_COMBAT_LOG_KIND_OPPORTUNITY_OFFERED
	b.byMove[ev.MoveID] = entry
	return true
}

func (b *logBuilder) reactionAnswered(e playdb.ListEncounterEventsRow, entry *logEntry) bool {
	// One line for each reaction used (PM-04c, 10); a "Deixar passar" and the
	// master's one-tap check are no line.
	re := entry.ev.Reaction
	if re == nil || re.Kind == string(reaction.MasterCheck) || re.Kind == string(reaction.LegendaryResistance) || (!re.Used && e.Kind == eventReactionAnswered) {
		return false
	}
	entry.kind = playv1.CombatLogKind_COMBAT_LOG_KIND_REACTION_WINDOW
	return true
}

func (b *logBuilder) reactionDeclined(e playdb.ListEncounterEventsRow, entry *logEntry) bool {
	ev := entry.ev
	if host, ok := b.byPending[ev.Pending]; ok {
		host.hosts = append(host.hosts, e.ID) // a decline is the entry's last action to undo
	}
	if host, ok := b.byMove[b.moveOf[ev.OfferID]]; ev.OfferID != "" && ok {
		host.hosts = append(host.hosts, e.ID) // an opportunity offer's answer is the move's to undo
	}
	return false
}

// damageEvent lands in the attack's or the spell's entry.
func (b *logBuilder) damageEvent(e playdb.ListEncounterEventsRow, entry *logEntry) bool {
	host, ok := b.byPending[entry.ev.Pending]
	if !ok {
		return false
	}
	host.hosts = append(host.hosts, e.ID)
	host.land(e.Kind, entry.ev)
	return false
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
		e.landRolled(ev)
	case eventDamageApplied:
		e.landApplied(ev)
	case eventDamageDiscarded:
		e.setStatus(ev.Pending, playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_DISCARDED)
		if dl, ok := e.pend[ev.Pending]; ok {
			dl.discarded = &ev
		}
	}
}

// landRolled is the damage roll: of an attack, or the one roll of a cast's pending damages.
func (e *logEntry) landRolled(ev actionEvent) {
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
}

// landApplied is the master's apply of the damage on a character.
func (e *logEntry) landApplied(ev actionEvent) {
	e.returned, e.returnBlocked = e.returned || ev.ReturnedFrom != nil, e.returnBlocked || ev.ReturnBlocked
	if dl, ok := e.pend[ev.Pending]; ok {
		dl.applied, dl.status = &ev, playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_APPLIED
		return
	}
	e.status, e.applied = playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_APPLIED, &ev
	if e.dmg != nil {
		e.dmg.After = ev.After // the character's hit points after
	}
}

// view builds the entry the viewer gets, and false when they do not get it. It reads in
// phases: who the entry is about, whether the viewer may have it at all, the fields every
// entry has, and the fields of its kind.
func (e *logEntry) view(ctx context.Context, v combatViewer, byID map[string]playdb.Combatant, names *keyNames, lastID string) (*playv1.CombatLogEntry, bool) {
	if e.kind == playv1.CombatLogKind_COMBAT_LOG_KIND_REACTION_WINDOW {
		return e.reactionEntry(ctx, v, byID, names)
	}
	actor, target := e.parties(byID)
	visible, seen := e.visibility(v, byID, actor, target)
	if !e.viewerGets(v, actor, target, visible, seen, byID) {
		return nil, false
	}
	c := &logViewing{
		ctx: ctx, v: v, byID: byID, names: names, actor: actor, target: target,
		dice: v.master || v.owns(actor), // the dice are the master's and the owner's: the master's rolls are not the players'
	}
	out := e.commonFields(c, visible, lastID)
	if !e.fill(c, out) {
		return nil, false
	}
	if out.GetDamage().GetHook() != nil { // the event keeps the key of the feature; the line names it
		out.Damage.Hook.SourceNamePt = names.contentName(ctx, out.Damage.Hook.GetSource())
	}
	return out, true
}

// logViewing is who reads an entry and who it is about, for the fillers of each kind.
type logViewing struct {
	ctx           context.Context
	v             combatViewer
	byID          map[string]playdb.Combatant
	names         *keyNames
	actor, target playdb.Combatant
	dice          bool
}

// parties are the actor and the target of the line, which the kind may swap: the
// affected one of the master's hit point change is the target.
func (e *logEntry) parties(byID map[string]playdb.Combatant) (actor, target playdb.Combatant) {
	actor, target = byID[e.ev.Actor], byID[e.ev.Target]
	switch e.kind {
	case playv1.CombatLogKind_COMBAT_LOG_KIND_HIT_POINTS_ADJUSTED:
		actor, target = playdb.Combatant{}, byID[e.ev.Actor] // the affected one is the target
	case playv1.CombatLogKind_COMBAT_LOG_KIND_REACTION:
		target = playdb.Combatant{} // who attacked is not part of the line
	case playv1.CombatLogKind_COMBAT_LOG_KIND_DEATH_CONFIRMED, playv1.CombatLogKind_COMBAT_LOG_KIND_CONDITIONS_CHANGED,
		playv1.CombatLogKind_COMBAT_LOG_KIND_CHARACTER_REVIVED:
		actor, target = playdb.Combatant{}, byID[e.ev.Actor]
	}
	return actor, target
}

// visibility says whether the players may have the entry as for who is in it (visible)
// and as for the fog of war (seen); the master gets it either way, with a mark.
func (e *logEntry) visibility(v combatViewer, byID map[string]playdb.Combatant, actor, target playdb.Combatant) (visible, seen bool) {
	visible = e.visibleToTable(v, byID, actor, target)
	// On a map with the fog of war, a line is the players' who saw its NPCs when it
	// happened, even if they have walked into the dark since; one they did not see
	// never appears later (MR-036). The master's copy says it is hidden from them.
	seen = e.ev.seenByViewer(v)
	// The master's reveal of a hider is the hider's player's line too (RN-10: no other player is told).
	if e.ev.Contest != nil && e.ev.Contest.Line == contestLineHideRevealed && v.owns(actor) {
		visible, seen = true, true
	}
	// The roll of a Recharge is the line of the master and of the player who controls the creature (RN-29), on
	// any map: the row is secret and fogged for everybody else.
	if (e.kind == playv1.CombatLogKind_COMBAT_LOG_KIND_RECHARGE_ROLLED || e.kind == playv1.CombatLogKind_COMBAT_LOG_KIND_GEAR_SWAPPED) && v.owns(actor) {
		return true, true
	}
	return visible && e.hitsVisible(byID), seen
}

// visibleToTable is what a player may see: nothing with a hidden combatant in it, when it
// happened or now (a combatant the master hides again takes its lines back). A combatant
// that left the combat is unknown now, and an entry about it would be anonymous: a player
// does not get it either.
func (e *logEntry) visibleToTable(v combatViewer, byID map[string]playdb.Combatant, actor, target playdb.Combatant) bool {
	_, actorKnown := byID[e.ev.Actor]
	_, targetKnown := byID[e.ev.Target]
	known := (e.ev.Actor == "" || actorKnown) && (e.ev.Target == "" || targetKnown || e.kind == playv1.CombatLogKind_COMBAT_LOG_KIND_REACTION)
	visible := !e.masterOnly && !e.ev.Secret && !actor.Hidden && !target.Hidden && known
	if e.kind == playv1.CombatLogKind_COMBAT_LOG_KIND_REACTION && (e.ev.AttackerHidden || byID[e.ev.Target].Hidden) && !v.owns(actor) {
		visible = false
	}
	return visible
}

// hitsVisible says that every target of a spell is a combatant the players may see. An
// area the server placed leaves the hidden creatures it hit out of the line, not the line.
func (e *logEntry) hitsVisible(byID map[string]playdb.Combatant) bool {
	if e.ev.Placed {
		return true
	}
	for _, h := range e.ev.Hits {
		if hit, ok := byID[h.Target]; !ok || hit.Hidden {
			return false
		}
	}
	return true
}

// viewerGets says whether the viewer gets the entry at all.
func (e *logEntry) viewerGets(v combatViewer, actor, target playdb.Combatant, visible, seen bool, byID map[string]playdb.Combatant) bool {
	if v.master {
		return true
	}
	if !visible || !seen {
		return false
	}
	if _, isEffect := effectLogKinds[kindKeyOf(e.kind)]; isEffect && e.ev.Lasting != nil && !effectLineVisible(e.ev.Lasting, v, byID) {
		return false
	}
	if e.ownersOnly(v, actor, target) {
		return false
	}
	// The offer's line is for whoever gets the offer (RN-10): the reactor's player
	// and the mover's.
	return e.kind != playv1.CombatLogKind_COMBAT_LOG_KIND_OPPORTUNITY_OFFERED || v.owns(actor) || v.owns(target)
}

// ownersOnly says the line is for the master and the combatants' own players, and the viewer is not one. An Escudo that
// ended is the combatant's: the Escudo may have answered an attack the other players do not see (a hidden attacker),
// and the line would tell them something happened. The save of a spell's hit effect tells that the effect has one: the
// attacker's player and the target's alone get it (RN-10).
func (e *logEntry) ownersOnly(v combatViewer, actor, target playdb.Combatant) bool {
	switch {
	case e.kind == playv1.CombatLogKind_COMBAT_LOG_KIND_EFFECT_ENDED && e.ev.EffectEnd != nil && e.ev.EffectEnd.Effect == effectShield:
		return !v.owns(actor)
	case e.kind == playv1.CombatLogKind_COMBAT_LOG_KIND_HIT_RIDER && e.ev.Rider != nil && e.ev.Rider.OwnersOnly:
		return !v.owns(actor) && !v.owns(target)
	}
	return false
}

// actedForPlayerKinds are the lines of what a character does on its turn: when the master's hand
// made one for a player's character, the line says so (his corrections of hit points, conditions
// and the like are the master's own tools, not an act for the character).
var actedForPlayerKinds = map[playv1.CombatLogKind]bool{
	playv1.CombatLogKind_COMBAT_LOG_KIND_ATTACK:          true,
	playv1.CombatLogKind_COMBAT_LOG_KIND_SPELL_CAST:      true,
	playv1.CombatLogKind_COMBAT_LOG_KIND_DEATH_SAVE:      true,
	playv1.CombatLogKind_COMBAT_LOG_KIND_MOVED:           true,
	playv1.CombatLogKind_COMBAT_LOG_KIND_ACTION:          true,
	playv1.CombatLogKind_COMBAT_LOG_KIND_GEAR_SWAPPED:    true,
	playv1.CombatLogKind_COMBAT_LOG_KIND_FEATURE_USED:    true,
	playv1.CombatLogKind_COMBAT_LOG_KIND_RESOURCE:        true,
	playv1.CombatLogKind_COMBAT_LOG_KIND_WILD_SHAPE:      true,
	playv1.CombatLogKind_COMBAT_LOG_KIND_TURN_PART_ENDED: true,
}

// byMasterFor says the line is the master's act for the player's character that is its actor: a
// user other than the character's player made it.
func (e *logEntry) byMasterFor(actor playdb.Combatant) bool {
	return actedForPlayerKinds[e.kind] && actor.Kind == kindPlayer && actor.UserID != nil && e.actorUser != nil && *e.actorUser != *actor.UserID
}

// commonFields are the fields every entry has, and the master's marks.
func (e *logEntry) commonFields(c *logViewing, visible bool, lastID string) *playv1.CombatLogEntry {
	out := &playv1.CombatLogEntry{
		Id: e.id, Kind: e.kind, At: timestamppb.New(e.at), Round: e.ev.Round,
		ActorId: c.actor.ID, ActorLabel: c.actor.Label, TargetId: c.target.ID, TargetLabel: c.target.Label,
		Key: shownKey(c.v, c.actor, e.ev.Key),
	}
	out.ByMaster = e.byMasterFor(c.actor)
	if c.v.master {
		out.Hidden = !visible || (e.ev.Fogged && len(e.ev.SeenBy) == 0)
		out.Undoable = slices.Contains(e.hosts, lastID)
	}
	switch e.kind {
	case playv1.CombatLogKind_COMBAT_LOG_KIND_ATTACK, playv1.CombatLogKind_COMBAT_LOG_KIND_ACTION,
		playv1.CombatLogKind_COMBAT_LOG_KIND_SPELL_CAST, playv1.CombatLogKind_COMBAT_LOG_KIND_REACTION,
		playv1.CombatLogKind_COMBAT_LOG_KIND_RESOURCE:
		out.KeyNamePt = c.names.of(c.ctx, c.actor, e.ev.Key)
	}
	return out
}

// deathSaveCount is how many successes make a character stable and how many failures make it dying.
const deathSaveCount = 3

// logFiller writes the fields of one kind of entry, and says false when the viewer has
// nothing of it to read after all.
type logFiller func(e *logEntry, c *logViewing, out *playv1.CombatLogEntry) bool

// logFillers are the fillers by kind; a kind with none has only the common fields.
var logFillers = map[playv1.CombatLogKind]logFiller{
	playv1.CombatLogKind_COMBAT_LOG_KIND_EFFECT:              (*logEntry).fillEffect,
	playv1.CombatLogKind_COMBAT_LOG_KIND_EXHAUSTION:          (*logEntry).fillEffect,
	playv1.CombatLogKind_COMBAT_LOG_KIND_RESOURCE:            (*logEntry).fillResource,
	playv1.CombatLogKind_COMBAT_LOG_KIND_MONSTERS_ADDED:      (*logEntry).fillMonsters,
	playv1.CombatLogKind_COMBAT_LOG_KIND_LEGENDARY_SPENT:     (*logEntry).fillLegendarySpent,
	playv1.CombatLogKind_COMBAT_LOG_KIND_RECHARGE_ROLLED:     (*logEntry).fillRechargeRolled,
	playv1.CombatLogKind_COMBAT_LOG_KIND_GEAR_SWAPPED:        (*logEntry).fillGearSwapped,
	playv1.CombatLogKind_COMBAT_LOG_KIND_FEATURE_USED:        (*logEntry).fillFeatureUsed,
	playv1.CombatLogKind_COMBAT_LOG_KIND_MOVED:               (*logEntry).fillMoved,
	playv1.CombatLogKind_COMBAT_LOG_KIND_CONTEST:             (*logEntry).fillContest,
	playv1.CombatLogKind_COMBAT_LOG_KIND_HIT_RIDER:           (*logEntry).fillHitRider,
	playv1.CombatLogKind_COMBAT_LOG_KIND_DOOR_OPENED:         (*logEntry).fillDoor,
	playv1.CombatLogKind_COMBAT_LOG_KIND_REVEAL_CHANGED:      (*logEntry).fillReveal,
	playv1.CombatLogKind_COMBAT_LOG_KIND_HIT_POINTS_ADJUSTED: (*logEntry).fillHitPoints,
	playv1.CombatLogKind_COMBAT_LOG_KIND_ATTACK:              (*logEntry).fillAttack,
	playv1.CombatLogKind_COMBAT_LOG_KIND_ACTION:              (*logEntry).fillAction,
	playv1.CombatLogKind_COMBAT_LOG_KIND_ROLL_MODE_ANSWERED:  (*logEntry).fillAnswer,
	playv1.CombatLogKind_COMBAT_LOG_KIND_STATE_CHANGED:       (*logEntry).fillState,
	playv1.CombatLogKind_COMBAT_LOG_KIND_DAMAGE_PART_REMOVED: (*logEntry).fillRemoval,
	playv1.CombatLogKind_COMBAT_LOG_KIND_SPELL_CAST:          (*logEntry).fillSpell,
	playv1.CombatLogKind_COMBAT_LOG_KIND_TRAP_TRIGGERED:      (*logEntry).fillTrap,
	playv1.CombatLogKind_COMBAT_LOG_KIND_WILD_SHAPE:          (*logEntry).fillWildShape,
	playv1.CombatLogKind_COMBAT_LOG_KIND_REACTION:            (*logEntry).fillReaction,
	playv1.CombatLogKind_COMBAT_LOG_KIND_DEATH_SAVE:          (*logEntry).fillDeathSave,
	playv1.CombatLogKind_COMBAT_LOG_KIND_CONDITIONS_CHANGED:  (*logEntry).fillConditions,
	playv1.CombatLogKind_COMBAT_LOG_KIND_EFFECT_ENDED:        (*logEntry).fillEffectEnd,
}

// fill writes the fields of the entry's kind.
func (e *logEntry) fill(c *logViewing, out *playv1.CombatLogEntry) bool {
	fill, ok := logFillers[e.kind]
	return !ok || fill(e, c, out)
}

func (e *logEntry) fillEffect(c *logViewing, out *playv1.CombatLogEntry) bool {
	c.names.s.effectLogView(c.ctx, e, out, c.v, c.byID, c.names.campaignID)
	return true
}

func (e *logEntry) fillResource(c *logViewing, out *playv1.CombatLogEntry) bool {
	out.Resource = resourceLogView(e.ev, c.v, c.actor, c.target)
	if e.ev.Res != nil && e.ev.Res.Kind == resBardicGive && !c.v.master && !c.v.owns(c.actor) && !c.v.owns(c.target) {
		out.TargetId, out.TargetLabel = "", "" // who holds the die is the bard's, the holder's and the master's
	}
	return true
}

func (e *logEntry) fillMonsters(c *logViewing, out *playv1.CombatLogEntry) bool {
	for _, it := range e.ev.Monsters.Items {
		out.Monsters = append(out.Monsters, &playv1.CombatLogMonster{
			CombatantId: it.ID, Label: c.byID[it.ID].Label, HitPoints: it.HitPoints, Rolled: it.Dice != "", Dice: it.Dice, Faces: it.Faces, Modifier: it.Modifier,
		})
	}
	return true
}

// fillLegendarySpent is the master's line of a spend: the option as the stat block names it and its cost.
func (e *logEntry) fillLegendarySpent(_ *logViewing, out *playv1.CombatLogEntry) bool {
	out.LegendarySpent = &playv1.CombatLogLegendarySpent{OptionName: e.ev.SpendName, Cost: e.ev.SpendCost}
	return true
}

// fillRechargeRolled is the line of a Recharge roll, the master's and the creature's controller's: the action, the face and whether it recharged.
func (e *logEntry) fillRechargeRolled(_ *logViewing, out *playv1.CombatLogEntry) bool {
	out.Recharge = &playv1.CombatLogRecharge{ActionName: e.ev.RechargeName, Face: e.ev.RechargeFace, Recharged: e.ev.Recharged}
	return true
}

// fillGearSwapped is the line of a swap of gear, the master's and the owner's: the piece, whether it is equipped now and what it cost.
func (e *logEntry) fillGearSwapped(_ *logViewing, out *playv1.CombatLogEntry) bool {
	out.GearSwapped = &playv1.CombatLogGearSwapped{NamePt: e.gear.Name, Kind: e.gear.Kind, Equipped: !e.gear.EquippedBefore, Cost: e.gear.Cost}
	return true
}

func (e *logEntry) fillMoved(c *logViewing, out *playv1.CombatLogEntry) bool {
	out.DistanceFt, out.DistanceDft = e.ev.DistanceFt, e.ev.DistanceDFt
	out.SpentDft, out.MoveDragging, out.MoveCrawling = e.ev.SpentDFt, e.ev.Dragging, e.ev.Crawling
	if out.DistanceDft == 0 { // an event written before the tenths of a foot
		out.DistanceDft = e.ev.DistanceFt * 10
	}
	switch e.ev.Jump {
	case jumpLong:
		out.Jump, out.JumpRunningStart = playv1.JumpKind_JUMP_KIND_LONG, e.ev.JumpRunning
	case jumpHigh:
		out.Jump, out.JumpHeightDft = playv1.JumpKind_JUMP_KIND_HIGH, e.ev.HeightDFt
	}
	out.LandingDifficult = c.v.master && e.ev.LandingDifficult // the Acrobatics reminder is the master's alone
	return true
}

func (e *logEntry) fillContest(_ *logViewing, out *playv1.CombatLogEntry) bool {
	out.Contest = &playv1.CombatLogContest{Line: contestLineProto[e.ev.Contest.Line]} // never a total or a DC (RN-20)
	return true
}

func (e *logEntry) fillHitRider(c *logViewing, out *playv1.CombatLogEntry) bool {
	if r := e.ev.Rider; r != nil {
		// The target's roll is the master's (an NPC's roll never reaches a player, RN-20), or the
		// target's own player's when a player's character made the save.
		target := c.byID[e.ev.Target]
		showDC := c.v.saveDCFor(target)
		if r.ActionKey != "" { // a creature's DC is a number of its stat block: the master's alone, even when the table shows a save's DC
			showDC = c.v.master
		}
		out.HitRider = riderLogProto(r, c.v.master || c.v.owns(target), showDC)
		if c.v.master { // the roll that failed and the use that made it a success are the master's alone (RN-20, RN-29)
			out.HitRider.Resisted, out.HitRider.ResistedLeft = r.Resisted, r.ResistLeft
		}
		if r.ActionKey != "" && !c.v.master {
			creatureRiderForPlayer(out.HitRider, c.v.owns(target), r)
		}
		e.riderDamage(c, out.HitRider, r)
	}
	return true
}

// riderDamage adds the damage a creature's rider opened, as the viewer gets a cast target's.
func (e *logEntry) riderDamage(c *logViewing, out *playv1.CombatLogHitRider, r *riderEvent) {
	if dl, ok := e.pend[r.Pending]; ok {
		out.Damage = dl.view(c.v, c.actor, c.target)
	}
	for _, id := range r.More {
		if dl, ok := e.pend[id]; ok {
			out.MoreDamages = append(out.MoreDamages, dl.view(c.v, c.actor, c.target))
		}
	}
}

func (e *logEntry) fillDoor(_ *logViewing, out *playv1.CombatLogEntry) bool {
	out.Door = &playv1.CombatLogDoor{Col: e.ev.Col, Row: e.ev.Row}
	return true
}

func (e *logEntry) fillReveal(c *logViewing, out *playv1.CombatLogEntry) bool {
	out.NowHidden = e.ev.NowHidden
	out.ActorId, out.ActorLabel = "", ""
	out.TargetId, out.TargetLabel = e.ev.Actor, c.byID[e.ev.Actor].Label
	return true
}

func (e *logEntry) fillHitPoints(_ *logViewing, out *playv1.CombatLogEntry) bool {
	out.HitPointsDelta = e.ev.Delta
	if e.ev.After != nil {
		out.HitPointsAfter = &e.ev.After.HP
	}
	return true
}

func (e *logEntry) fillAttack(c *logViewing, out *playv1.CombatLogEntry) bool {
	v := c.v
	out.Outcome = outcomeToProto[e.ev.Outcome]
	out.AsReaction = e.ev.AsReaction
	out.ReturnedToReach, out.ReturnBlocked = e.returned, v.master && e.returnBlocked
	coverKey, coverSource := e.ev.coverFor(v)
	out.Cover, out.CoverSource = coverDegreeProto(coverKey), coverSourceProto(coverSource)
	if v.master && e.ev.TargetAC > 0 { // "CA 17: 15 + 2 de meia cobertura": a player never gets an armor class (RN-20)
		out.TargetArmorClass, out.CoverBonus = &e.ev.TargetAC, e.ev.CoverBonus
	}
	if c.dice {
		e.attackDice(out, v)
	}
	// The trade the attack made: its feature and numbers are the dice's audience.
	out.PowerAttack = e.ev.PowerAttack.logView(c.dice)
	if len(e.stopped) > 0 || len(e.parried) > 0 { // Escudo or a Parry stopped it: a miss, with no damage
		out.Outcome = playv1.AttackOutcome_ATTACK_OUTCOME_MISS
		out.StoppedByReaction = len(e.stopped) > 0 || v.master // a player is not told a reaction did it (RN-20)
	} else if e.ev.Pending != "" {
		out.Damage = e.damage(c.dice, v.master, v.master || v.owns(c.target), v.saveDCFor(c.target), out)
		e.dressDamage(out.Damage, v, c.dice, v.master || (c.target.Kind == kindPlayer && v.owns(c.target)), holdsHP(c.target))
	}
	return true
}

// attackDice are the dice of an attack: the master's and the attacker's player's.
func (e *logEntry) attackDice(out *playv1.CombatLogEntry, v combatViewer) {
	out.AttackRoll = diceRoll(1, 20, []int32{e.ev.D20}, e.ev.Modifier, e.ev.Total, e.ev.Physical)
	if e.ev.D20B != 0 {
		out.AttackRoll = diceRoll(2, 20, pairOf(e.ev.D20, e.ev.D20B, e.ev.Counted), e.ev.Modifier, e.ev.Total, e.ev.Physical)
		out.AttackRoll.CountedIndex = e.ev.Counted
	}
	e.ev.mark(out.AttackRoll)
	out.ModeChange = e.modeChange(e.ev, e.ev.ByMaster)
	out.Reason = e.rolls.reasonOf(e.ev)
	out.BonusDice = bonusDiceOf(e.ev)
	out.AttackRoll.ExtraDice = extraDiceProto(e.ev.Extra, v.master, e.rolls.keyName)
	out.AttackRoll.Rerolled = strokeRerolls(e.ev.Lucky, e.ev.Stroke) // "Sortudo: 1 → 14", with the d20 it changed; "Golpe de Sorte" for a miss turned into a hit
}

func (e *logEntry) fillAction(c *logViewing, out *playv1.CombatLogEntry) bool {
	if (e.ev.Heal || e.ev.TempTried) && (c.v.master || c.v.owns(c.actor)) { // Retomar o fôlego: only the master and its own player see the numbers
		out.Damage = &playv1.CombatLogDamage{
			Status: playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_APPLIED, Healing: true, Amount: e.ev.Amount, TempHpGained: e.ev.TempGained,
		}
		if e.ev.Heal && e.ev.DiceCount > 0 { // Integridade Corporal heals with no roll
			out.Damage.Roll = diceRoll(e.ev.DiceCount, e.ev.DiceSides, e.ev.Faces, e.ev.Modifier, e.ev.Total, e.ev.Physical)
		}
	}
	if e.ev.Channel != nil && (c.v.master || c.v.owns(c.actor)) { // Canalizar Divindade: the numbers are the master's and its player's
		out.ChannelDivinity = channelProto(e.ev.Channel)
	}
	return true
}

func (e *logEntry) fillAnswer(c *logViewing, out *playv1.CombatLogEntry) bool {
	if !c.v.master && !c.v.owns(c.actor) {
		return false
	}
	out.ModeChange = e.answerChange()
	out.Reason = e.rolls.reasonOf(e.ev)
	return true
}

func (e *logEntry) fillState(_ *logViewing, out *playv1.CombatLogEntry) bool {
	out.State = &playv1.CombatLogStateChange{Kind: stateKindProto(e.ev.StateKind), Started: e.ev.StateStarted, Reason: e.ev.StateReason}
	if e.ev.StateKind == stateTable {
		out.State.Key, out.State.NamePt = e.ev.Key, e.ev.StateName
	}
	return true
}

func (e *logEntry) fillRemoval(c *logViewing, out *playv1.CombatLogEntry) bool {
	return e.removalView(c.v, c.v.owns(c.actor), out)
}

func (e *logEntry) fillSpell(c *logViewing, out *playv1.CombatLogEntry) bool {
	out.Spell = e.spellView(c.v, c.byID)
	out.CreatureAction = e.creatureActionView(c.v, c.byID)
	return true
}

// creatureActionView is what the line of a creature's action says besides the cast: which action, the
// condition a failed save gave and to whom (the ones the viewer sees; a condition with no outward sign is
// the target's own player's and the master's), how long it lasts. The text of the action is the master's.
// Nil for any other cast. The stat block's numbers are in the saves, which the viewer reads as saveView
// gives them: the DC is the master's alone.
func (e *logEntry) creatureActionView(v combatViewer, byID map[string]playdb.Combatant) *playv1.CombatLogCreatureAction {
	cr := e.ev.CreatureAct
	if cr == nil {
		return nil
	}
	out := &playv1.CombatLogCreatureAction{ActionNamePt: cr.Name, Reminder: cr.Reminder}
	if v.master {
		out.ActionKey, out.Text = e.ev.Key, cr.Text
	}
	outward := len(withoutOwnerOnly([]string{cr.ConditionKey})) > 0
	for _, h := range e.ev.Hits {
		target := byID[h.Target]
		if !h.CondSet || e.spellTargetUnseen(v, h, target) || (!outward && !v.master && !v.owns(target)) {
			continue
		}
		out.ConditionTargetIds = append(out.ConditionTargetIds, h.Target)
	}
	// The condition and how long it lasts are told with who got it: a viewer who sees no target given it
	// (every target saved, or none that the viewer sees) reads nothing of them.
	if v.master || len(out.ConditionTargetIds) > 0 {
		out.ConditionKey, out.ConditionNamePt, out.DurationText, out.RepeatSave = cr.ConditionKey, cr.ConditionName, cr.Duration, cr.RepeatSave
	}
	return out
}

func (e *logEntry) fillTrap(c *logViewing, out *playv1.CombatLogEntry) bool {
	out.Trap = e.trapEntry(c.ctx, c.v, c.byID, c.names)
	return true
}

func (e *logEntry) fillWildShape(c *logViewing, out *playv1.CombatLogEntry) bool {
	out.WildShape = &playv1.CombatLogWildShape{BeastKey: e.ev.Beast, BeastNamePt: c.names.contentName(c.ctx, e.ev.Beast), Started: e.shapeStarted}
	if !e.shapeStarted {
		out.WildShape.EndReason = wildShapeEndReasonProto[e.ev.Reason]
		if c.v.master || c.v.owns(c.actor) { // the carried damage is the druid's player's and the master's (RN-20)
			out.WildShape.CarriedDamage = e.ev.Carried
		}
	}
	return true
}

func (e *logEntry) fillReaction(c *logViewing, out *playv1.CombatLogEntry) bool {
	out.Spell = &playv1.CombatLogSpell{Slot: slotProto(e.ev.Slot, c.v, c.actor)}
	return true
}

func (e *logEntry) fillDeathSave(c *logViewing, out *playv1.CombatLogEntry) bool {
	v := c.v
	after := e.ev.Death
	if after == nil {
		after = &deathState{}
	}
	if v.hiddenFrom(e.ev.DeathHidden, c.actor) {
		// The table keeps the road to the owner and the master (RN-24): a save that
		// went on is no line for anybody else, and the one that made the character
		// stable says only that it did, a result the table sees.
		if after.Successes < deathSaveCount || e.ev.DeathOutcome != deathSuccess {
			return false
		}
		out.DeathSave = &playv1.CombatLogDeathSave{Stable: true}
		return true
	}
	out.DeathSave = &playv1.CombatLogDeathSave{
		Outcome: deathOutcomeToProto[e.ev.DeathOutcome], Successes: after.Successes, Failures: after.Failures,
		Stable: after.Successes >= deathSaveCount, Dying: v.master && after.Failures >= deathSaveCount,
	}
	if v.master || v.owns(c.actor) {
		out.DeathSave.Roll = deathRoll(e.ev)
		if h := e.ev.Hook; h != nil {
			out.DeathSave.Hook = hookProto(h, c.names.contentName(c.ctx, h.Source)) // what a hook gave is the owner's and the master's
		}
	}
	return true
}

func (e *logEntry) fillConditions(c *logViewing, out *playv1.CombatLogEntry) bool {
	out.Conditions = e.ev.Conditions
	if !c.v.master && !c.v.owns(c.actor) { // a condition with no outward sign is the owner's and the master's (RN-10)
		out.Conditions = withoutOwnerOnly(e.ev.Conditions)
		if e.ev.ConcEnded == "" && slices.Equal(out.Conditions, withoutOwnerOnly(e.ev.CondBefore)) {
			return false // nothing a player may read changed
		}
	}
	out.ConcentrationEndedKey = e.ev.ConcEnded
	return true
}

func (e *logEntry) fillEffectEnd(c *logViewing, out *playv1.CombatLogEntry) bool {
	end := e.ev.EffectEnd
	if end == nil {
		return true
	}
	out.EffectEnd = &playv1.CombatLogEffectEnd{Effect: combatEffectToProto[end.Effect]}
	if c.v.master { // the numbers are the master's and the player's own (RN-20); the player reads the vitals
		out.EffectEnd.HitPointsBefore, out.EffectEnd.HitPointsAfter = end.HP0, end.HP1
		out.EffectEnd.HitPointsMaxBefore, out.EffectEnd.HitPointsMaxAfter = end.Max0, end.Max1
	}
	return true
}

// spellView is the cast as the viewer gets it: what each target did, in the
// order they were listed. A target's save dice are the master's and the
// target's own player's; the caster's d20 the master's and the caster's.
func (e *logEntry) spellView(v combatViewer, byID map[string]playdb.Combatant) *playv1.CombatLogSpell {
	caster := byID[e.ev.Actor]
	out := &playv1.CombatLogSpell{Slot: slotProto(e.ev.Slot, v, caster), Concentrating: e.ev.Concentrate, ConcentrationEndedKey: e.ev.ConcEnded}
	out.EffectKind, out.PoolRoll, out.EffectConditionKey, out.EffectThreshold = effectHeader(e.ev, v, caster)
	out.MaterialSpent = e.ev.Material && (v.master || v.owns(caster)) // the diamonds are the caster's and the master's line
	if e.ev.Res != nil && e.ev.Res.Kind == resMetamagic {
		out.MetamagicKeys, out.SorceryPointsSpent = e.ev.Res.Keys, e.ev.Res.Spent
	}
	for _, h := range e.ev.Hits {
		if v.master && h.DeathRound != nil && e.ev.FxKind == rules.SpellKindRevive {
			out.RevivedDeathRound = h.DeathRound
		}
		target := byID[h.Target]
		if e.spellTargetUnseen(v, h, target) {
			continue
		}
		out.Targets = append(out.Targets, e.spellTargetView(v, h, caster, target))
	}
	return out
}

// spellTargetUnseen says the players' line never lists the target: a creature that was
// hidden when an area hit it (not even after the master reveals it: the line would say the
// spell hit it), nor one that is hidden now or that a combatant that left no longer names.
func (e *logEntry) spellTargetUnseen(v combatViewer, h castHit, target playdb.Combatant) bool {
	return !v.master && (h.HiddenAtCast || target.Hidden || target.ID == "" || h.unseenBy(e.ev.CoverUsers, v.userID))
}

// spellTargetView is what one target of the cast did, as the viewer gets it.
func (e *logEntry) spellTargetView(v combatViewer, h castHit, caster, target playdb.Combatant) *playv1.CombatLogSpellTarget {
	t := &playv1.CombatLogSpellTarget{
		TargetId: target.ID, TargetLabel: target.Label, Darts: h.Darts, Outcome: outcomeToProto[h.Outcome],
		AttackRoll: attackRollView(h, v, caster), Save: creatureSaveView(e.ev, h, v, caster, target),
		Effect: effectView(h, v, target),
	}
	coverKey, coverSource := h.coverFor(v, e.ev.CoverUsers)
	t.Cover, t.CoverSource = coverDegreeProto(coverKey), coverSourceProto(coverSource)
	t.Hidden = v.master && h.HiddenAtCast
	t.PushedFt, t.PushNoMap, t.PushStopped = pushedFt(h.Pushed), pushedNoMap(h.Pushed), pushedStopped(h.Pushed)
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
	return t
}

// view is a spell target's damage or heal as the viewer gets it: the amount that
// landed, the dice to the master and the caster's player, what the master
// overruled to him alone.
func (d *damageLog) view(v combatViewer, caster, target playdb.Combatant) *playv1.CombatLogDamage {
	out := &playv1.CombatLogDamage{Status: d.status}
	if d.discarded != nil && d.discarded.ConcentrationDC > 0 && v.saveDCFor(target) {
		dc := d.discarded.ConcentrationDC
		out.ConcentrationDc = &dc
	}
	if d.roll == nil {
		return out // the cast opened it and it is not rolled yet
	}
	d.viewAmount(out, v, target)
	r := d.roll
	if v.master || v.owns(caster) { // the caster's dice, as for an attack's damage
		out.Roll = diceRoll(r.DiceCount, r.DiceSides, r.Faces, r.Modifier, r.Total, r.Physical)
		out.Roll.CountedDice = countedProto(r.CountedDice) // with the faces: the roller's and the master's
	}
	if d.applied != nil { // the master's apply of a character's damage
		d.viewApplied(out, v, target)
		return out
	}
	d.viewLanded(out, v, target)
	return out
}

// viewAmount sets the amount the damage or heal came to, the type and the critical rule:
// a player who is not the target's reads it as rolled.
func (d *damageLog) viewAmount(out *playv1.CombatLogDamage, v combatViewer, target playdb.Combatant) {
	r := d.roll
	out.Amount, out.DamageTypeKey, out.DamageTypePt = d.hit.Amount, r.DamageType, damageTypePT[r.DamageType]
	if !v.master && !v.owns(target) && d.hit.Shown > 0 { // as rolled, before the target's modifiers (RN-20)
		out.Amount = d.hit.Shown
	}
	out.Half, out.Healing = d.hit.Half, r.Heal
	out.CriticalRule, out.CriticalMax = pendingCriticalRule(r.Critical, r.CriticalMaxRule), r.CriticalMax
	if r.Heal && !v.master && !v.owns(target) {
		// A heal capped at the maximum would tell how many hit points the target
		// lacked: everyone but the master and the target's player gets the roll (RN-20).
		out.Amount = r.Total
	}
}

// viewApplied adds the master's apply of a character's damage to the view.
func (d *damageLog) viewApplied(out *playv1.CombatLogDamage, v combatViewer, target playdb.Combatant) {
	ap := d.applied
	if v.master || v.owns(target) { // the apply is after the target's modifiers and the master's override: another player keeps the amount as rolled (RN-20)
		out.Amount = ap.Amount
	}
	out.RelentlessEndurance = ap.Relentless
	if v.master || v.owns(target) {
		out.Hook = hookProto(ap.Hook, "") // what a drop_to_zero hook gave is the target's player's and the master's
	}
	if !v.hiddenFrom(ap.DeathHidden, target) { // a hit at 0 is a death save failure (RN-24)
		out.DeathFailuresAdded = ap.FailuresAdded
	}
	if ap.Overridden && v.master {
		rolled := ap.Rolled
		out.RolledAmount = &rolled
	}
	if dc := ap.ConcentrationDC; dc > 0 && v.saveDCFor(target) {
		out.ConcentrationDc = &dc
	}
	if after := ap.After; after != nil {
		out.TargetDown = !after.Defeated && after.HP == 0
	}
}

// viewLanded adds what the damage did to the target when it landed by itself (an NPC's).
func (d *damageLog) viewLanded(out *playv1.CombatLogDamage, v combatViewer, target playdb.Combatant) {
	after := d.hit.After
	if after == nil || d.status != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_APPLIED {
		return
	}
	out.TargetDefeated = after.Defeated
	out.TargetDown = !after.Defeated && after.HP == 0
	if dc := d.hit.ConcentrationDC; dc > 0 && v.saveDCFor(target) {
		out.ConcentrationDc = &dc
	}
}

// damage is the damage of an attack as the viewer gets it. The target's hit
// points after it are the master's alone, set on the entry; the concentration
// DC is the master's and, when the table shows the DC of a saving throw, the target's own player's
// (showDC).
func (e *logEntry) damage(dice, master, targetsOwn, showDC bool, out *playv1.CombatLogEntry) *playv1.CombatLogDamage {
	d := &playv1.CombatLogDamage{Status: e.status}
	if e.dmg == nil {
		return d // the attack hit and its damage is not rolled yet
	}
	d.Amount, d.DamageTypeKey, d.DamageTypePt = e.dmg.Amount, e.dmg.DamageType, damageTypePT[e.dmg.DamageType]
	d.CriticalRule, d.CriticalMax = pendingCriticalRule(e.dmg.Critical, e.dmg.CriticalMaxRule), e.dmg.CriticalMax
	if dice {
		d.ExtraDiceCount, d.ExtraDiceNamePt = e.dmg.ExtraDice, extraDiceName(e.dmg.ExtraDice, e.dmg.SavageDice)
		d.Roll = diceRoll(e.dmg.DiceCount, e.dmg.DiceSides, e.dmg.Faces, e.dmg.Modifier, e.dmg.Amount, e.dmg.Physical)
		d.Roll.CountedDice = countedProto(e.dmg.CountedDice)
	}
	if e.applied != nil { // the master's apply of a character's damage
		e.damageApplied(d, master, targetsOwn, showDC)
	}
	if after := e.dmg.After; after != nil && e.status == playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_APPLIED {
		d.TargetDefeated = after.Defeated
		d.TargetDown = !after.Defeated && after.HP == 0
		if master {
			out.HitPointsAfter = &after.HP
		}
	}
	if e.applied == nil && e.dmg.ConcentrationDC > 0 && showDC && e.status == playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_APPLIED {
		dc := e.dmg.ConcentrationDC
		d.ConcentrationDc = &dc
	}
	return d
}

// damageApplied adds the master's apply of a character's damage to an attack's damage.
func (e *logEntry) damageApplied(d *playv1.CombatLogDamage, master, targetsOwn, showDC bool) {
	ap := e.applied
	d.Amount = ap.Amount
	d.RelentlessEndurance = ap.Relentless
	if master || targetsOwn {
		d.Hook = hookProto(ap.Hook, "")
	}
	if !ap.DeathHidden || targetsOwn { // a hit at 0 is a death save failure, which the table may keep to the owner and the master (RN-24)
		d.DeathFailuresAdded = ap.FailuresAdded
	}
	if ap.Overridden && master {
		rolled := ap.Rolled
		d.RolledAmount = &rolled
	}
	if dc := ap.ConcentrationDC; dc > 0 && showDC {
		d.ConcentrationDc = &dc
	}
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
		showDC: v.showSaveDC,
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
			return e.ev.Trap.Caught[i].hiddenFrom(v)
		},
		label: func(id string) string { return byID[id].Label },
		names: func(key string) string { return names.contentName(ctx, key) },
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
	traps       map[string]string       // the names of the traps that fired, by point
	content     func(key string) string // the names of the content, from the campaign's content
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
	if name, found := sheetActionName(sheet, key); found {
		return name
	}
	if strings.HasPrefix(key, "spell:") && !isCreature(c) {
		if sp, err := n.s.roster.CombatSpell(ctx, nil, n.campaignID, c.CharacterID, key, 0, ""); err == nil {
			return sp.Name
		}
	}
	return ""
}

// choiceText is the label of the choice a feature offered, from the sheet of the combatant that used it ("" when the sheet no
// longer has it).
func (n *keyNames) choiceText(ctx context.Context, c playdb.Combatant, key, choice string) string {
	if n.of(ctx, c, key) == "" || choice == "" { // reads the sheet into byCharacter
		return ""
	}
	if fa, ok := featureOfKey(n.byCharacter[sheetKey(c)], key); ok && fa.Use != nil {
		for _, ch := range fa.Use.Choices {
			if ch.Key == choice {
				return ch.TextPT
			}
		}
	}
	return ""
}

// sheetActionName is the name of the key among the attacks, the actions and the feature
// actions of a sheet, in that order; found says the sheet has the key.
func sheetActionName(sheet link.Sheet, key string) (name string, found bool) {
	for _, a := range sheet.Attacks {
		if a.Key == key {
			return a.Name, true
		}
	}
	for _, a := range sheet.Actions {
		if a.Key == key {
			return a.Name, true
		}
	}
	for _, a := range sheet.FeatureActions {
		if a.Key == key {
			return a.Name, true
		}
	}
	return "", false
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
	endedByLeaving:   playv1.WildShapeEndReason_WILD_SHAPE_END_REASON_LEFT,
	endedByDamage:    playv1.WildShapeEndReason_WILD_SHAPE_END_REASON_DAMAGE,
	endedByMaster:    playv1.WildShapeEndReason_WILD_SHAPE_END_REASON_MASTER,
	endedAtZero:      playv1.WildShapeEndReason_WILD_SHAPE_END_REASON_ZERO_HP,
	endedAsleep:      playv1.WildShapeEndReason_WILD_SHAPE_END_REASON_UNCONSCIOUS,
	endedExpired:     playv1.WildShapeEndReason_WILD_SHAPE_END_REASON_EXPIRED,
	endedMasterEnded: playv1.WildShapeEndReason_WILD_SHAPE_END_REASON_MASTER_ENDED,
}

// withoutOwnerOnly is the conditions a player who does not own the creature may read.
func withoutOwnerOnly(keys []string) []string {
	var out []string
	for _, k := range keys {
		if !slices.Contains(ownerOnlyConditions, k) {
			out = append(out, k)
		}
	}
	return out
}
