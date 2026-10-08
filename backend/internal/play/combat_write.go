package play

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"uuid"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/logging"
	"github.com/PuraFome/meuRPG/backend/internal/platform/tablerules"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// The kinds of session_events rows a combat writes (session_event_kinds).
// Their payloads hold IDs and numbers only, never a name (docs/privacy.md).
const (
	eventEncounterStarted    = "encounter_started"
	eventInitiativeSubmitted = "initiative_submitted"
	eventInitiativeOrderSet  = "initiative_order_set"
	eventCombatBegun         = "combat_begun"
	eventTurnEnded           = "turn_ended"
	eventCombatantMoved      = "combatant_moved"
	eventCombatantHiddenSet  = "combatant_hidden_set"
	eventCombatantsAdded     = "combatants_added"
	eventCombatantRemoved    = "combatant_removed"
	eventEncounterEnded      = "encounter_ended"
	// The actions of a turn (combat_actions.go, combat_undo.go).
	eventAttackRolled      = "attack_rolled"
	eventDamageRolled      = "damage_rolled"
	eventDamageApplied     = "damage_applied"
	eventDamageDiscarded   = "damage_discarded"
	eventActionTaken       = "action_taken"
	eventHitPointsAdjusted = "hit_points_adjusted"
	eventActionUndone      = "action_undone"
	// Spells, reactions, death saves and conditions (combat_spells.go,
	// combat_reactions.go, combat_death.go, combat_conditions.go).
	eventSpellCast        = "spell_cast"
	eventReactionUsed     = "reaction_used"
	eventReactionDeclined = "reaction_declined"
	eventDeathSaveRolled  = "death_save_rolled"
	eventDeathConfirmed   = "death_confirmed"
	eventConditionsSet    = "conditions_set"
)

// The kinds of Etapa 7: the XP awards (package progression writes them
// through AppendEvent) and the scenes. session_event_kinds lists them
// all, and TestSessionEventKindsMatchTheTable keeps the two in step.
const (
	eventXPAwarded        = "xp_awarded"
	eventXPAwardUndone    = "xp_award_undone"
	eventMilestoneMarked  = "milestone_marked"
	eventSceneOpened      = "scene_opened"
	eventSceneClosed      = "scene_closed"
	eventSceneCheckRolled = "scene_check_rolled"
)

// The kinds of Etapa 8: a clue revealed to players (package maps writes it
// through AppendEvent) and the stage changing (MR-031).
const (
	eventClueRevealed = "clue_revealed"
	eventStageChanged = "stage_changed"
)

// The kinds of the second Etapa 8 wave (migration 00083). The master gave a
// character one more attempt at a scene action (scene_attempt_granted,
// MR-015); a member of a joint turn ended their part and the turn goes on
// (turn_part_ended, MR-013). The last part to end writes turn_ended, as a turn
// always did.
const (
	eventSceneAttemptGranted = "scene_attempt_granted"
	eventTurnPartEnded       = "turn_part_ended"
)

// The kinds of Etapa 9 (migration 00090), added in one migration before the
// slices that write them, so that slices built at the same time never fight
// over the CHECK. Traps and treasure are written by package maps through
// AppendEvent, the rest by this package.
const (
	eventTrapNoticed        = "trap_noticed"
	eventTrapSearched       = "trap_searched"
	eventTrapTriggered      = "trap_triggered"
	eventTrapDisarmed       = "trap_disarmed"
	eventTrapRevealed       = "trap_revealed"
	eventTreasureFound      = "treasure_found"
	eventTreasureUnfound    = "treasure_unfound"
	eventCoverSet           = "cover_set"
	eventSideSet            = "side_set"
	eventOpportunityOffered = "opportunity_offered"
	eventCreatureSummoned   = "creature_summoned"
	eventCreatureDismissed  = "creature_dismissed"
	eventWildShapeStarted   = "wild_shape_started"
	eventWildShapeEnded     = "wild_shape_ended"
	eventFamiliarSight      = "familiar_sight"
)

// The kind of Etapa 10 (migration 00121): a move opened a closed door (MR-010,
// RN-26). Written by this package, in the move's transaction.
const eventDoorOpened = "door_opened"

// combatWrite describes one change to a combat: who makes it, the idempotency
// key, the kind of event it becomes, and the combat it is about (empty when
// the change creates it).
type combatWrite struct {
	m   authz.Membership
	key string
	// hash is the hash of the whole request but its key (idem.Hash): a retry of the key is
	// the same request, and the same key for another one is refused.
	hash        *string
	kind        string
	encounterID string
	// altKind is the other kind the change may write instead of kind (EndTurn
	// writes turn_part_ended when the turn does not pass yet): a retry of the
	// change under the same key may find either.
	altKind string
}

// combatTx is what a change works with inside its transaction: the open
// session, locked, and the combat. characterID, when the closure sets it,
// is the character the event is about.
type combatTx struct {
	tx          pgx.Tx
	q           *playdb.Queries
	session     playdb.GameSession
	enc         playdb.Encounter
	now         time.Time
	characterID *string
	// kind is the kind of the event the change writes: the one in combatWrite,
	// unless the closure sets it to the altKind.
	kind string
	// castID is the id the pending damages of the spell being cast share.
	castID string
	// actorUserID is who makes the change, for the events a change writes besides
	// its own (the creatures it summons or dismisses).
	actorUserID string
	// svc is the service the change runs in, for what a turn starting does (ending a
	// familiar's sight); nil in the few places that make a combatTx outside write.
	svc *Service
	// told are the vitals of the characters whose Wild Shape form or familiar sight
	// ended: write tells the streams and the fog after the commit (MR-036).
	told []*playv1.CharacterVitals
	// master says the master makes the change: the turn that waits for an
	// opportunity attack's answer never stops him.
	master bool
	// sight is what the players see of the combat's map, read before the
	// transaction opened (nil without the fog of war): it filters what the change
	// tells its caller and stamps who could see the event (combat_fog.go).
	sight *fogSight
	// stamped is the last event insertEvent stamped with who could see it.
	stamped *actionEvent
	// hash is the request hash kept with the event that carries the change's key.
	hash *string
	// rules are the table's rules (RN-24), read in this transaction when it
	// opened: a critical hit and who sees the death saves follow them from the
	// next roll on, and are never kept longer than the change.
	rules tablerules.Rules
}

// combatResult is what a change leaves for the handler: the session, and
// whether the call was a retry of a change already made.
type combatResult struct {
	session playdb.GameSession
	// payload is the payload of the event a retried change wrote the first
	// time, so a handler can answer a retry with the same numbers.
	payload []byte
	// encounterID is the combat the change was about: the one named in the
	// request, or the one StartEncounter created. Empty only for a retried
	// start, whose combat the retry does not know.
	encounterID string
	repeated    bool
	// kind, characterID and event say what the change wrote, for its log line:
	// the kind of session event, the character it is about, and its payload when
	// that is an actionEvent. Empty for a retry, which wrote nothing.
	kind        string
	characterID string
	event       *actionEvent
	// sight is what the players saw of the combat's map before the change (nil without
	// the fog), and stamped the event the change wrote with who could see it: the
	// publishers use both (withFogMemo).
	sight   *fogSight
	stamped *actionEvent
}

// write runs one change to a combat, as AdjustCharacterVitals runs a vitals
// correction: one transaction locks the open session's row, checks the
// idempotency key, lets do change the rows, and appends the session event.
// do returns the event's payload, or nil when nothing changed, and then no
// event is written. Only after the commit does the handler publish.
func (s *Service) write(ctx context.Context, w combatWrite, do func(c *combatTx) (payload any, err error)) (combatResult, error) {
	// What the players see is read first, never inside the transaction: the maps
	// module asks this one where the combatants stand, and that read would wait for
	// the rows the change has already written. The sight is read with the combat's
	// revision; once the transaction holds the session lock the revision is checked
	// again, and a change that got in between makes the sight stale: it is read again
	// (at most maxSightTries times, then the change is aborted).
	for try := 1; ; try++ {
		sight, err := s.sightForWrite(ctx, w)
		if err != nil {
			return combatResult{}, s.dbError(ctx, "work out what the players see", err)
		}
		if s.afterSightRead != nil {
			s.afterSightRead(w.encounterID) // a test changes the combat here
		}
		res, err := s.writeOnce(ctx, w, sight, do)
		if errors.Is(err, errSightStale) {
			if try >= maxSightTries {
				return combatResult{}, connect.NewError(connect.CodeAborted, errors.New("the combat keeps changing: try again"))
			}
			continue
		}
		if err == nil {
			// Here, after the transaction returned, so a retried transaction
			// logs once.
			s.logCombatEvent(ctx, res)
		}
		return res, err
	}
}

// writeOnce is one try of write, with the sight it was given.
func (s *Service) writeOnce(ctx context.Context, w combatWrite, sight *fogSight, do func(c *combatTx) (payload any, err error)) (combatResult, error) {
	var res combatResult
	var last *actionEvent
	var ended *combatTx // the change's transaction, for what it leaves to tell
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		res, ended = combatResult{encounterID: w.encounterID, sight: sight}, nil
		last = nil
		q := s.queries.WithTx(tx)
		session, err := q.GetOpenGameSessionForUpdate(ctx, w.m.CampaignID)
		if errors.Is(err, pgx.ErrNoRows) {
			return errNoOpenSession() // MR-013: during the session
		}
		if err != nil {
			return fmt.Errorf("lock the open session: %w", err)
		}
		res.session = session

		done, err := q.GetSessionEventByIdempotencyKey(ctx, playdb.GetSessionEventByIdempotencyKeyParams{
			GameSessionID: session.ID, IdempotencyKey: &w.key,
		})
		switch {
		case err == nil:
			if done.Kind != w.kind && (w.altKind == "" || done.Kind != w.altKind) {
				return connect.NewError(connect.CodeInvalidArgument, errors.New("idempotency_key was already used for another change"))
			}
			// The key was used for another request of the same kind: a retry has the very same
			// one. An event from before the hash was kept has none, and is replayed as it was.
			if hashDiffers(done.IdempotencyHash, w.hash) {
				return connect.NewError(connect.CodeInvalidArgument, errors.New("idempotency_key was already used for another change"))
			}
			// A key is its author's: another member replaying it would be handed the
			// answer to a change that was not theirs (the vitals of someone else's
			// character, a roll).
			if done.ActorUserID == nil || *done.ActorUserID != w.m.UserID {
				return connect.NewError(connect.CodeInvalidArgument, errors.New("idempotency_key was already used for another change"))
			}
			res.repeated = true // a retry of a change already made
			res.payload = done.Payload
			return nil
		case !errors.Is(err, pgx.ErrNoRows):
			return fmt.Errorf("find the event of this idempotency key: %w", err)
		}

		c := &combatTx{tx: tx, q: q, session: session, now: s.now(), kind: w.kind, actorUserID: w.m.UserID, svc: s, master: w.m.Role == authz.RoleMaster, sight: sight, hash: w.hash}
		if c.rules, err = s.tableRules(ctx, tx, w.m.CampaignID); err != nil {
			return err
		}
		if w.encounterID != "" {
			c.enc, err = q.GetEncounterInSession(ctx, playdb.GetEncounterInSessionParams{GameSessionID: session.ID, ID: w.encounterID})
			if errors.Is(err, pgx.ErrNoRows) {
				return connect.NewError(connect.CodeNotFound, errors.New("encounter not found"))
			}
			if err != nil {
				return fmt.Errorf("find the encounter: %w", err)
			}
			if sight != nil && c.enc.Revision != sight.rev {
				return errSightStale
			}
		}
		payload, err := do(c)
		if err != nil || payload == nil {
			return err
		}
		// A change to a combatant's hit points may defeat a creature or give one back
		// (MR-037): the creatures follow, before the change's own event is written.
		if err := s.syncCreatures(ctx, c); err != nil {
			return err
		}
		// The offers nobody can answer any more stop holding the mover's turn.
		if err := s.pruneOffers(ctx, c); err != nil {
			return err
		}
		res.encounterID = c.enc.ID
		res.kind = c.kind
		res.characterID = deref(c.characterID)
		res.event = nil
		if ev, ok := payload.(actionEvent); ok {
			res.event = &ev
		}
		ended = c
		c.stamped = nil
		if err := insertEvent(ctx, c, c.kind, &w.m.UserID, &w.key, payload); err != nil {
			return err
		}
		last = c.stamped
		return nil
	})
	if err != nil {
		return combatResult{}, err
	}
	res.stamped = last
	// A turn that started ended some familiar's sight (MR-036): the player's vitals
	// and the fog's view change.
	if ended != nil && len(ended.told) > 0 {
		for _, v := range ended.told {
			s.publishVitals(w.m.CampaignID, v)
		}
		s.maps.VisionChanged(ctx, w.m.CampaignID, deref(ended.enc.MapID))
	}
	return res, nil
}

// combatEventNames are the names of the combat's log events, where the kind
// of the session event does not already read well as `combat.<kind>`.
var combatEventNames = map[string]string{
	eventEncounterStarted: "combat.started",
	eventCombatBegun:      "combat.begun",
	eventEncounterEnded:   "combat.ended",
	eventTurnEnded:        "combat.turn_passed",
	eventAttackRolled:     "combat.attack_resolved",
	eventSpellCast:        "combat.spell_resolved",
}

// logCombatEvent writes the DEBUG `event` line of a combat change that was
// committed (not a retry, which changed nothing). One place covers every
// change that goes through write: the session event's kind is the name, and
// the line carries ids and the attack's outcome only, never a name.
func (s *Service) logCombatEvent(ctx context.Context, res combatResult) {
	if res.kind == "" || res.repeated {
		return
	}
	name, ok := combatEventNames[res.kind]
	if !ok {
		name = "combat." + res.kind
	}
	attrs := []slog.Attr{slog.String("session_id", res.session.ID), slog.String("encounter_id", res.encounterID)}
	if res.characterID != "" {
		attrs = append(attrs, slog.String("character_id", res.characterID))
	}
	if ev := res.event; ev != nil {
		if ev.Actor != "" {
			attrs = append(attrs, slog.String("combatant_id", ev.Actor))
		}
		if ev.Target != "" {
			attrs = append(attrs, slog.String("target_id", ev.Target))
		}
		if ev.Outcome != "" {
			attrs = append(attrs, slog.String("outcome", ev.Outcome))
		}
		if ev.Round > 0 {
			attrs = append(attrs, slog.Int("round", int(ev.Round)))
		}
	}
	logging.Event(ctx, s.logger, name, attrs...)
}

// eventPayloadBudget is how many bytes of payload one event may take: the table
// (session_events, migration 00024) refuses more than 4096, and an event that
// lists every creature of a scene has to stay under it with room for the fog's
// stamp. A list that does not fit is written as several events.
const eventPayloadBudget = 3500

// fitPrefix is how many of the first n items fit: the largest count from 1 to n
// for which fits is true (1 when none does: an item cannot be split further).
func fitPrefix(n int, fits func(count int) bool) int {
	for count := n; count > 1; count-- {
		if fits(count) {
			return count
		}
	}
	return min(n, 1)
}

// insertEvent appends a session event inside the change's transaction. A firing
// of a trap whose caught creatures do not fit one event is written as several
// (insertFiringInParts).
func insertEvent(ctx context.Context, c *combatTx, kind string, actor, key *string, payload any) error {
	if ev, ok := payload.(actionEvent); ok {
		var err error
		if kind != eventActionUndone { // an undo keeps the stamp of the action it takes back
			if ev, err = c.stamp(ctx, kind, ev); err != nil { // who could see it, on a fog map
				return err
			}
		}
		c.stamped = &ev
		payload = ev
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode the event payload: %w", err)
	}
	if ev, ok := payload.(actionEvent); ok && len(body) > eventPayloadBudget && ev.Trap != nil && len(ev.Trap.Caught) > 1 {
		_, err := insertFiringInParts(ctx, c, kind, actor, key, ev)
		return err
	}
	_, err = insertRaw(ctx, c, kind, actor, key, body)
	return err
}

// requestHash is the request hash of a change whose handler builds its inputs apart from the
// request message: the hash of the values that tell it from another change, in order.
func requestHash(parts ...string) *string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	h := hex.EncodeToString(sum[:])
	return &h
}

// hashDiffers says whether the hash kept with an event is not the request's: a change that
// reuses a key for another request. An event from before the hash was kept has none, and is
// replayed as it was.
func hashDiffers(stored, hash *string) bool {
	return stored != nil && (hash == nil || *stored != *hash)
}

// hashOf is the request hash to keep with an event: the change's own, on the event that
// carries its key.
func hashOf(c *combatTx, key *string) *string {
	if key == nil {
		return nil
	}
	return c.hash
}

// insertRaw writes an encoded event and returns its ID.
func insertRaw(ctx context.Context, c *combatTx, kind string, actor, key *string, body []byte) (string, error) {
	seq, err := c.q.NextSessionEventSeq(ctx, c.session.ID)
	if err != nil {
		return "", fmt.Errorf("next event number: %w", err)
	}
	// The combat log and the undo find the event by its combat.
	var encounterID *string
	if c.enc.ID != "" {
		encounterID = &c.enc.ID
	}
	row, err := c.q.InsertSessionEvent(ctx, playdb.InsertSessionEventParams{
		GameSessionID: c.session.ID, Seq: seq, Kind: kind, ActorUserID: actor, CharacterID: c.characterID,
		Payload: body, IdempotencyKey: key, CreatedAt: c.now, EncounterID: encounterID, IdempotencyHash: hashOf(c, key),
	})
	if err != nil {
		return "", fmt.Errorf("insert session event: %w", err)
	}
	return row.ID, nil
}

// insertFiringInParts writes a trap's firing whose creatures do not fit one event
// as several, back to back: the first carries the firing (and the idempotency key,
// so a retry finds it), and each next one adds the creatures that did not fit to
// it, as the master's own "add creatures to a firing" does (ExtendsID), marked as a
// part (trapFireEvent.Part) so that an undo takes the whole firing back at once. It
// returns the ID of the first event, the firing's.
func insertFiringInParts(ctx context.Context, c *combatTx, kind string, actor, key *string, ev actionEvent) (string, error) {
	caught := ev.Trap.Caught
	with := func(base actionEvent, trap trapFireEvent, from, to int) ([]byte, actionEvent, error) {
		trap.Caught = caught[from:to]
		base.Trap = &trap
		body, err := json.Marshal(base)
		if err != nil {
			return nil, base, fmt.Errorf("encode the event payload: %w", err)
		}
		return body, base, nil
	}
	size := func(base actionEvent, trap trapFireEvent, from int) func(count int) bool {
		return func(count int) bool {
			body, _, err := with(base, trap, from, from+count)
			return err == nil && len(body) <= eventPayloadBudget
		}
	}
	n := fitPrefix(len(caught), size(ev, *ev.Trap, 0))
	body, _, err := with(ev, *ev.Trap, 0, n)
	if err != nil {
		return "", err
	}
	hostID, err := insertRaw(ctx, c, kind, actor, key, body)
	if err != nil {
		return "", err
	}
	group := ev.Trap.ExtendsID // the firing the parts add to: this one, unless it extends another
	if group == "" {
		group = hostID
	}
	part := actionEvent{Round: ev.Round, Secret: ev.Secret, Fogged: ev.Fogged, SeenBy: ev.SeenBy, Actor: ev.Actor}
	trap := trapFireEvent{PointID: ev.Trap.PointID, MapID: ev.Trap.MapID, Manual: ev.Trap.Manual, ExtendsID: group, Part: true}
	for at := n; at < len(caught); at += n {
		n = fitPrefix(len(caught)-at, size(part, trap, at))
		body, _, err := with(part, trap, at, at+n)
		if err != nil {
			return "", err
		}
		if _, err := insertRaw(ctx, c, kind, actor, nil, body); err != nil {
			return "", err
		}
	}
	return hostID, nil
}

// publishTimeout bounds what finish reads and publishes after a commit, which
// no longer depends on the caller's patience.
const publishTimeout = 10 * time.Second

// finish builds the handler's answer after a change: it reads the combat
// the change was about (the session's latest, for a retried start), lets
// publish tell the streams, and returns the combat as the caller sees it.
//
// The change is already committed, so the reading and the telling run on a
// context that outlives the call: a client that hangs up, or a deadline that
// passes, must not leave the master and the players without the hint. A retry
// of a change already made (a repeated key) tells the streams too, with the
// hints every change gives, because the first call may have died before
// telling: a hint only says "read again", so a second one is harmless.
func (s *Service) finish(ctx context.Context, m authz.Membership, res combatResult, publish func(ctx context.Context, d *encounterData)) (*playv1.Encounter, error) {
	pctx, stop := context.WithTimeout(context.WithoutCancel(ctx), publishTimeout)
	defer stop()
	var enc playdb.Encounter
	var err error
	if res.encounterID != "" {
		enc, err = s.queries.GetEncounterInSession(pctx, playdb.GetEncounterInSessionParams{GameSessionID: res.session.ID, ID: res.encounterID})
	} else {
		enc, err = s.queries.GetLatestEncounter(pctx, res.session.ID)
	}
	if err != nil {
		return nil, s.dbError(ctx, "find the encounter", err)
	}
	d, err := loadEncounter(pctx, s.queries, enc)
	if err != nil {
		return nil, s.dbError(ctx, "read the encounter", err)
	}
	// The sight read after the commit is read once for the publishers and the answer;
	// the one read before it says who could see the old squares.
	pctx = withFogMemo(pctx, res.sight, res.stamped)
	switch {
	case publish == nil:
	case res.repeated:
		s.publishRetried(pctx, m.CampaignID, res, d)
	default:
		publish(pctx, d)
	}
	return s.viewFor(withFogMemo(ctx, res.sight, res.stamped), m, d)
}

// publishRetried is what a retry of a committed change tells the streams: the
// combat changed and its log did, to the players only when the first call's
// event was not one a hidden combatant is in.
func (s *Service) publishRetried(ctx context.Context, campaignID string, res combatResult, d *encounterData) {
	ev, err := readEvent(res.payload)
	s.publishEncounterChanged(ctx, campaignID, d.enc)
	s.publishLogChanged(ctx, campaignID, d.enc.ID, err == nil && !ev.Secret)
}

// changed is the usual publish: a hint that the combat changed.
func (s *Service) changed(campaignID string) func(ctx context.Context, d *encounterData) {
	return func(ctx context.Context, d *encounterData) { s.publishEncounterChanged(ctx, campaignID, d.enc) }
}

// parseKey reads an idempotency key.
func parseKey(raw string) (string, error) {
	key, err := uuid.Parse(raw)
	if err != nil {
		return "", connect.NewError(connect.CodeInvalidArgument, errors.New("idempotency_key must be a UUID"))
	}
	return key.String(), nil
}

// parseCombatID reads an ID of a combat or a combatant: not a UUID names
// nothing.
func parseCombatID(raw, what string) (string, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return "", connect.NewError(connect.CodeNotFound, errors.New(what+" not found"))
	}
	return id.String(), nil
}

func errCombatantNotFound() error {
	return connect.NewError(connect.CodeNotFound, errors.New("combatant not found"))
}

// errEncounter is CombatService's failed_precondition, with the
// EncounterBlocked detail that tells the app why. edit fills the detail's
// extra fields.
func errEncounter(reason playv1.EncounterBlockedReason, msg string, edit ...func(*playv1.EncounterBlocked)) error {
	err := connect.NewError(connect.CodeFailedPrecondition, errors.New(msg))
	blocked := &playv1.EncounterBlocked{Reason: reason}
	for _, e := range edit {
		e(blocked)
	}
	if detail, detailErr := connect.NewErrorDetail(blocked); detailErr == nil {
		err.AddDetail(detail)
	}
	return err
}

// findCombatant returns the combatant with the ID, or `not_found`; for a
// player, a hidden one is not found either (RN-10), and one that is not
// theirs is `permission_denied`.
func findCombatant(cs []playdb.Combatant, id string, v combatViewer) (playdb.Combatant, error) {
	for _, c := range cs {
		if c.ID != id {
			continue
		}
		if !v.sees(c) {
			return playdb.Combatant{}, errCombatantNotFound()
		}
		return c, nil
	}
	return playdb.Combatant{}, errCombatantNotFound()
}

// mayAct checks that the viewer may act for the combatant: the master for
// anyone, a player for their own character only.
func (v combatViewer) mayAct(c playdb.Combatant) error {
	if v.master || v.owns(c) {
		return nil
	}
	return connect.NewError(connect.CodePermissionDenied, errors.New("only the combatant's player or the master may do this"))
}

// notEnded fails with ENCOUNTER_ENDED when the combat is over.
func notEnded(e playdb.Encounter) error {
	if e.Status == statusEnded {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ENCOUNTER_ENDED, "the combat has ended")
	}
	return nil
}

// saveOrder puts the combatants in the order ordered says, writing the
// places that changed, and returns the list with them set. before is the
// list as stored.
func saveOrder(ctx context.Context, q *playdb.Queries, before, ordered []playdb.Combatant) ([]playdb.Combatant, error) {
	stored := make(map[string]playdb.Combatant, len(before))
	for _, c := range before {
		stored[c.ID] = c
	}
	out := make([]playdb.Combatant, len(ordered))
	for i, c := range ordered {
		was := stored[c.ID]
		c.OrderIndex = int32(i) // at most 40
		if was.OrderIndex != c.OrderIndex || was.TieOrdered != c.TieOrdered {
			if err := q.SetCombatantOrder(ctx, playdb.SetCombatantOrderParams{ID: c.ID, OrderIndex: c.OrderIndex, TieOrdered: c.TieOrdered}); err != nil {
				return nil, fmt.Errorf("save the turn order: %w", err)
			}
		}
		out[i] = c
	}
	return out, nil
}
