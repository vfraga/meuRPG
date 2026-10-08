package play

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"uuid"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	maplink "github.com/PuraFome/meuRPG/backend/internal/maps/link"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/logging"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// The master firing a trap, and a token landing in one (MR-035, D5): in a combat
// that runs on the trap's map the firing is a combat change (combat_traps.go);
// otherwise it is written here, with the damage to a player's character waiting for
// the master in trap_damages. NPCs outside a combat have no hit points stored, so
// what a trap does to one is only a line of the history.

// maxTrapTargets is how many creatures one firing may catch.
const maxTrapTargets = 40

// FireTrap implements playv1connect.PlayServiceHandler.
func (s *Service) FireTrap(
	ctx context.Context,
	req *connect.Request[playv1.FireTrapRequest],
) (*connect.Response[playv1.FireTrapResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	key, err := parseKey(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	mapID, err := uuid.Parse(req.Msg.GetMapId())
	if err != nil {
		return nil, errTrapNotFound()
	}
	pointID, err := uuid.Parse(req.Msg.GetPointId())
	if err != nil {
		return nil, errTrapNotFound()
	}
	targets := req.Msg.GetTargetIds()
	if len(targets) == 0 {
		targets = nil // the creatures in the area
	}
	if len(targets) > maxTrapTargets {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("target_ids must have at most %d ids", maxTrapTargets))
	}
	for _, id := range targets {
		if _, err := uuid.Parse(id); err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("target_ids must be UUIDs"))
		}
	}
	extend := req.Msg.GetExtendFiringId()
	if extend != "" {
		if _, err := uuid.Parse(extend); err != nil {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("firing not found"))
		}
		if len(targets) == 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("extend_firing_id needs the target_ids to add"))
		}
	}
	if s.traps == nil {
		return nil, errNoTraps()
	}
	traps, err := s.traps.Traps(ctx, nil, m.CampaignID, mapID.String())
	if err != nil {
		return nil, s.dbError(ctx, "read the map's traps", err)
	}
	i := slices.IndexFunc(traps, func(t maplink.Trap) bool { return t.PointID == pointID.String() })
	if i < 0 {
		return nil, errTrapNotFound()
	}
	trap := traps[i]
	session, err := s.openSession(ctx, m.CampaignID)
	if err != nil {
		return nil, err
	}
	// A combat without a map has no traps (RN-25). A retry of a firing made before
	// it began (the key is already in the history) still gets its stored answer.
	if theatre, err := theatreRunning(ctx, s.queries, session.ID); err != nil {
		return nil, s.dbError(ctx, "find the combat", err)
	} else if theatre {
		if seen, err := keySeen(ctx, s.queries, session.ID, key); err != nil {
			return nil, s.dbError(ctx, "find the event of this idempotency key", err)
		} else if !seen {
			return nil, errNeedsAMap()
		}
	}
	enc, inCombat, err := s.runningEncounterOn(ctx, session.ID, trap.MapID)
	if err != nil {
		return nil, s.dbError(ctx, "find the combat", err)
	}
	if extend != "" {
		// The firing to extend is this trap's, from this session, in the same place (a
		// combat, or outside one), and the trap is as it left it.
		row, err := s.queries.GetSessionEventByID(ctx, playdb.GetSessionEventByIDParams{GameSessionID: session.ID, ID: extend})
		var host actionEvent
		if err == nil {
			err = json.Unmarshal(row.Payload, &host)
		}
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && (row.Kind != eventTrapTriggered || host.Trap == nil || host.Trap.PointID != trap.PointID)) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("firing not found"))
		}
		if err != nil {
			return nil, s.dbError(ctx, "read the firing", err)
		}
		if host.Trap.ExtendsID != "" || (inCombat && (row.EncounterID == nil || *row.EncounterID != enc.ID)) || (!inCombat && row.EncounterID != nil) || trap.State != "triggered" {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("that firing cannot be extended now"))
		}
	}
	if inCombat {
		return s.fireByHandInCombat(ctx, m, key, trap, enc, targets, extend)
	}
	fired, firingID, repeated, err := s.fireOutsideCombat(ctx, m.CampaignID, m.UserID, key, fireHash(trap, extend, targets), trap, targets, nil, true, extend)
	if err != nil {
		return nil, s.dbError(ctx, "fire a trap", err)
	}
	if !repeated {
		s.afterFiring(ctx, m.CampaignID, fired, playdb.Encounter{})
	} else if fired, err = s.firingWithParts(ctx, session.ID, firingID, fired); err != nil {
		return nil, s.dbError(ctx, "read the firing", err) // the answer of the first call listed every creature, whatever the events it took
	}
	names, err := s.characterLabels(ctx, m.CampaignID, fired)
	if err != nil {
		return nil, s.dbError(ctx, "read the characters' names", err)
	}
	if extend != "" {
		firingID = extend
	}
	return connect.NewResponse(&playv1.FireTrapResponse{Firing: firingProto(fired, firingID, trap.Name, trapView{master: true, label: func(id string) string { return names[id] }})}), nil
}

// fireHash is the request hash of a trap fired by hand: the trap, the firing it extends and
// who it hits, in any order.
func fireHash(trap maplink.Trap, extend string, targetIDs []string) *string {
	return requestHash(append([]string{trap.PointID, extend}, slices.Sorted(slices.Values(targetIDs))...)...)
}

// fireByHandInCombat is FireTrap while a combat runs on the trap's map.
func (s *Service) fireByHandInCombat(ctx context.Context, m authz.Membership, key string, trap maplink.Trap, enc playdb.Encounter, targetIDs []string, extend string) (*connect.Response[playv1.FireTrapResponse], error) {
	var made actionEvent
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: fireHash(trap, extend, targetIDs), kind: eventTrapTriggered, encounterID: enc.ID}, func(c *combatTx) (any, error) {
		if err := notEnded(c.enc); err != nil {
			return nil, err
		}
		cs, err := c.q.ListCombatants(ctx, c.enc.ID)
		if err != nil {
			return nil, fmt.Errorf("list the combatants: %w", err)
		}
		var caught []playdb.Combatant
		if len(targetIDs) == 0 {
			caught = inArea(cs, trap)
		}
		for _, id := range targetIDs {
			who, err := findCombatant(cs, id, combatViewer{master: true})
			if err != nil {
				return nil, err
			}
			if !slices.ContainsFunc(caught, func(o playdb.Combatant) bool { return o.ID == who.ID }) {
				caught = append(caught, who)
			}
		}
		fired, err := s.fireInCombat(ctx, c, trap, caught, true, extend)
		if errors.Is(err, maplink.ErrTrapNotArmed) {
			return nil, errTrapNotArmed()
		}
		if err != nil {
			return nil, err
		}
		if c.enc, err = c.q.TouchEncounter(ctx, c.enc.ID); err != nil {
			return nil, fmt.Errorf("touch the encounter: %w", err)
		}
		made = actionEvent{Round: c.enc.Round, Trap: fired}
		return made, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "fire a trap", err)
	}
	ev, err := resultEvent(res, made)
	if err != nil {
		return nil, s.dbError(ctx, "read the firing", err)
	}
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.afterFiring(ctx, m.CampaignID, ev.Trap, d.enc)
	})
	if err != nil {
		return nil, err
	}
	_ = out
	firingID := extend
	if firingID == "" {
		row, err := s.queries.GetSessionEventByIdempotencyKey(ctx, playdb.GetSessionEventByIdempotencyKeyParams{GameSessionID: res.session.ID, IdempotencyKey: &key})
		if err != nil {
			return nil, s.dbError(ctx, "read the firing", err)
		}
		firingID = row.ID
		if res.repeated { // the answer of the first call listed every creature, whatever the events it took
			if ev.Trap, err = s.firingWithParts(ctx, res.session.ID, row.ID, ev.Trap); err != nil {
				return nil, s.dbError(ctx, "read the firing", err)
			}
		}
	}
	label := s.membersOf(ctx, res)
	return connect.NewResponse(&playv1.FireTrapResponse{Firing: firingProto(ev.Trap, firingID, trap.Name, trapView{master: true, label: func(id string) string {
		c, _ := label(id)
		return c.Label
	}})}), nil
}

// firingWithParts adds to a firing, as its event holds it, the creatures of the
// events written after it for the ones that did not fit (trapFireEvent.Part).
func (s *Service) firingWithParts(ctx context.Context, sessionID, hostID string, fired *trapFireEvent) (*trapFireEvent, error) {
	if fired == nil {
		return nil, nil
	}
	recent, err := s.queries.ListRecentSessionEvents(ctx, playdb.ListRecentSessionEventsParams{GameSessionID: sessionID, Limit: recentEvents})
	if err != nil {
		return nil, fmt.Errorf("read the latest events: %w", err)
	}
	group := fired.ExtendsID
	if group == "" {
		group = hostID
	}
	whole := *fired
	whole.Caught = slices.Clone(fired.Caught)
	at := slices.IndexFunc(recent, func(e playdb.ListRecentSessionEventsRow) bool { return e.ID == hostID })
	for i := at - 1; i >= 0; i-- { // the events after it, oldest first: its parts follow it back to back
		if recent[i].Kind != eventTrapTriggered {
			break
		}
		ev, err := readEvent(recent[i].Payload)
		if err != nil {
			return nil, fmt.Errorf("read the event of a part: %w", err)
		}
		if ev.Trap == nil || !ev.Trap.Part || ev.Trap.ExtendsID != group {
			break
		}
		whole.Caught = append(whole.Caught, ev.Trap.Caught...)
	}
	return &whole, nil
}

// afterFiring tells the streams, after the commit, what a firing changed: the trap
// is public now (everyone who sees the map), the combat and its log changed, and
// the master's trap card has new damage to apply.
func (s *Service) afterFiring(ctx context.Context, campaignID string, fired *trapFireEvent, enc playdb.Encounter) {
	if fired == nil {
		return
	}
	logging.Event(ctx, s.logger, "trap.triggered", slog.String("map_id", fired.MapID), slog.String("point_id", fired.PointID),
		slog.Int("caught", len(fired.Caught)), slog.Bool("in_combat", enc.ID != ""))
	s.traps.TrapChanged(ctx, campaignID, fired.MapID, fired.PointID)
	if enc.ID != "" {
		s.publishEncounterChanged(ctx, campaignID, enc)
		s.publishLogChanged(ctx, campaignID, enc.ID, true)
	}
	s.Publish(campaignID, false, mapChangedHint(fired.MapID)) // the master's trap card: damage waits there
}

// characterLabels returns the names of the characters a firing caught.
func (s *Service) characterLabels(ctx context.Context, campaignID string, fired *trapFireEvent) (map[string]string, error) {
	var ids []string
	for _, cc := range fired.Caught {
		ids = append(ids, cc.Target)
	}
	out := map[string]string{}
	if len(ids) == 0 {
		return out, nil
	}
	named, err := s.creatureLabels(ctx, campaignID, fired.Caught)
	if err != nil {
		return nil, err
	}
	chars, err := s.roster.SessionCharacters(ctx, nil, campaignID, ids)
	if err != nil {
		return nil, err
	}
	for _, c := range chars {
		out[c.ID] = c.Name
	}
	maps.Copy(out, named)
	return out, nil
}

// tokensOn is where the map's tokens stand, in squares, with the characters that
// live (a dead character's token stays on the map and is never caught).
func (s *Service) tokensOn(ctx context.Context, campaignID, mapID string) (map[string]grid.Square, []link.Character, map[string]grid.Square, error) {
	g, err := s.maps.MapGrid(ctx, nil, campaignID, mapID)
	if err != nil {
		return nil, nil, nil, err
	}
	tokens, err := s.maps.MapTokens(ctx, nil, mapID)
	if err != nil {
		return nil, nil, nil, err
	}
	if !g.OK() {
		return nil, nil, nil, nil
	}
	gr := grid.Grid{Columns: int(g.Columns), Rows: int(g.Rows)}
	at := map[string]grid.Square{}
	creatureAt := map[string]grid.Square{} // the creatures' tokens, by creature
	ids := make([]string, 0, len(tokens))
	for _, t := range tokens {
		sq := gr.SquareOf(int(t.XBP), int(t.YBP))
		if t.CreatureID != "" {
			creatureAt[t.CreatureID] = sq
			continue
		}
		at[t.CharacterID] = sq
		ids = append(ids, t.CharacterID)
	}
	living, err := s.roster.CombatCharacters(ctx, nil, campaignID, ids)
	if err != nil {
		return nil, nil, nil, err
	}
	return at, living, creatureAt, nil
}

// creaturesInArea are the live creatures of the party whose tokens stand in the trap's
// area (MR-035, MR-037): a creature put there earlier is caught like a character.
func (s *Service) creaturesInArea(ctx context.Context, campaignID string, creatureAt map[string]grid.Square, trap maplink.Trap) ([]maplink.MapCreature, error) {
	var inArea []string
	for id, sq := range creatureAt {
		if trap.Covers(sq) {
			inArea = append(inArea, id)
		}
	}
	if len(inArea) == 0 {
		return nil, nil
	}
	party, err := s.roster.CombatParty(ctx, nil, campaignID)
	if err != nil {
		return nil, err
	}
	ownerIDs := make([]string, 0, len(party))
	for _, o := range party {
		if o.Player {
			ownerIDs = append(ownerIDs, o.ID)
		}
	}
	var found []link.Creature
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		found, err = s.roster.CharacterCreatures(ctx, tx, campaignID, ownerIDs)
		return err
	})
	if err != nil {
		return nil, err
	}
	var out []maplink.MapCreature
	for _, c := range found {
		if slices.Contains(inArea, c.ID) {
			out = append(out, maplink.MapCreature{ID: c.ID, OwnerCharacterID: c.CharacterID, OwnerUserID: c.OwnerUserID, Name: c.Name, MonsterKey: c.MonsterKey})
		}
	}
	slices.SortFunc(out, func(a, b maplink.MapCreature) int { return strings.Compare(a.ID, b.ID) })
	return out, nil
}

// fireOutsideCombat fires the trap while no combat runs on its map, in a
// transaction of its own, and returns the firing and whether it was a retry. key
// is "" for a firing nobody retries (a token landing); targetIDs are characters
// with tokens on the map, nil for every character in the area (an empty, non-nil
// list catches none: a creature's token dropped into a trap that picks its targets by
// hand); creatures are the creature tokens that are caught as well.
func (s *Service) fireOutsideCombat(ctx context.Context, campaignID, actorUserID, key string, hash *string, trap maplink.Trap, targetIDs []string, creatures []maplink.MapCreature, manual bool, extend string) (*trapFireEvent, string, bool, error) {
	at, living, creatureAt, err := s.tokensOn(ctx, campaignID, trap.MapID)
	if err != nil {
		return nil, "", false, err
	}
	if targetIDs == nil {
		// Everyone in the area: the creatures already standing there too, besides the one just dropped.
		inArea, err := s.creaturesInArea(ctx, campaignID, creatureAt, trap)
		if err != nil {
			return nil, "", false, err
		}
		for _, c := range inArea {
			if !slices.ContainsFunc(creatures, func(o maplink.MapCreature) bool { return o.ID == c.ID }) {
				creatures = append(creatures, c)
			}
		}
	}
	var caught []link.Character
	for _, ch := range living {
		sq, onMap := at[ch.ID]
		switch {
		case targetIDs == nil && onMap && trap.Covers(sq):
			caught = append(caught, ch)
		case slices.Contains(targetIDs, ch.ID):
			caught = append(caught, ch)
		}
	}
	for _, id := range targetIDs {
		if !slices.ContainsFunc(caught, func(ch link.Character) bool { return ch.ID == id }) {
			return nil, "", false, connect.NewError(connect.CodeInvalidArgument, errors.New("target_ids must be characters with a token on the trap's map"))
		}
	}

	if s.afterTrapRead != nil {
		s.afterTrapRead() // a test starts a combat here
	}
	var fired *trapFireEvent
	var firingID string
	var repeated bool
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		fired, firingID, repeated = nil, "", false
		session, err := q.GetOpenGameSessionForUpdate(ctx, campaignID)
		if errors.Is(err, pgx.ErrNoRows) {
			return errNoOpenSession()
		}
		if err != nil {
			return fmt.Errorf("lock the open session: %w", err)
		}
		if key != "" {
			done, err := q.GetSessionEventByIdempotencyKey(ctx, playdb.GetSessionEventByIdempotencyKeyParams{GameSessionID: session.ID, IdempotencyKey: &key})
			switch {
			case err == nil:
				if done.Kind != eventTrapTriggered || hashDiffers(done.IdempotencyHash, hash) {
					return connect.NewError(connect.CodeInvalidArgument, errors.New("idempotency_key was already used for another change"))
				}
				var ev actionEvent
				if err := json.Unmarshal(done.Payload, &ev); err != nil {
					return fmt.Errorf("decode the event of %s: %w", done.ID, err)
				}
				fired, firingID, repeated = ev.Trap, done.ID, true
				return nil
			case !errors.Is(err, pgx.ErrNoRows):
				return fmt.Errorf("find the event of this idempotency key: %w", err)
			}
		}
		// What was read before the lock may be stale: a combat that began since is the
		// one the trap fires in, and a firing outside one must not land beside it.
		enc, err := q.GetLatestEncounter(ctx, session.ID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("find the session's combat: %w", err)
		}
		switch {
		case err != nil || enc.Status == statusEnded:
		case isTheatre(enc):
			return errNeedsAMap() // a combat without a map has no traps (RN-25)
		case enc.MapID != nil && *enc.MapID == trap.MapID:
			return errCombatBegan()
		}
		c, err := s.openTx(ctx, combatTx{tx: tx, q: q, session: session, now: s.now(), actorUserID: actorUserID, hash: hash})
		if err != nil {
			return err
		}
		if fired, err = s.fireOutside(ctx, c, trap, caught, creatures, manual, extend); err != nil {
			return err
		}
		var keyPtr *string
		if key != "" {
			keyPtr = &key
		}
		firingID, err = insertSceneEvent(ctx, c, eventTrapTriggered, &actorUserID, keyPtr, actionEvent{Trap: fired})
		return err
	})
	if errors.Is(err, maplink.ErrTrapNotArmed) {
		return nil, "", false, errTrapNotArmed()
	}
	return fired, firingID, repeated, err
}

// fireOutside resolves the trap against characters, inside the transaction: the
// trap is triggered, the effect rolled, and the damage to a player's character
// waits for the master in trap_damages. An NPC has no hit points stored outside a
// combat: its damage is only in the event. Conditions are a reminder in it.
func (s *Service) fireOutside(ctx context.Context, c *combatTx, trap maplink.Trap, caught []link.Character, creatures []maplink.MapCreature, manual bool, extend string) (*trapFireEvent, error) {
	campaignID := c.session.CampaignID
	prev := trap
	ev := &trapFireEvent{PointID: trap.PointID, MapID: trap.MapID, Manual: manual, ExtendsID: extend}
	if extend == "" {
		var err error
		if prev, err = s.traps.TriggerTrap(ctx, c.tx, campaignID, trap.MapID, trap.PointID, c.now); err != nil {
			return nil, err
		}
		ev.PrevState, ev.PrevTriggeredAt = prev.State, prev.TriggeredAt
	}
	effect := prev.Spec.GetEffect()
	ability := trapSaveAbility(effect)
	// The creatures caught come after the characters: a creature has no hit points
	// stored outside a combat either, so its part is only the line, as an NPC's.
	targets := make([]trapTarget, len(caught)+len(creatures))
	for i, ch := range caught {
		targets[i].id = ch.ID
		if trapNeedsAC(effect) {
			sheet, err := s.roster.CombatSheet(ctx, c.tx, campaignID, ch.ID)
			if err != nil {
				return nil, err
			}
			targets[i].armorClass = sheet.ArmorClass
		}
		if ability != "" {
			save, err := s.roster.CombatSave(ctx, c.tx, campaignID, ch.ID, ability)
			if err != nil {
				return nil, err
			}
			targets[i].save, targets[i].saveKnown = save.Bonus, save.Known
		}
	}
	for j, cr := range creatures {
		i := len(caught) + j
		targets[i].id = cr.ID
		if trapNeedsAC(effect) {
			sheet, ok, err := s.roster.CreatureSheet(ctx, c.tx, campaignID, cr.MonsterKey, "")
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, errCombatantNotFound()
			}
			targets[i].armorClass = sheet.ArmorClass
		}
		if ability != "" {
			save, err := s.roster.CreatureSave(ctx, c.tx, campaignID, cr.MonsterKey, ability)
			if err != nil {
				return nil, err
			}
			targets[i].save, targets[i].saveKnown = save.Bonus, save.Known
		}
	}
	outcomes, err := resolveTrap(effect, targets, criticalRuleOf(c.rules), s.trapD20, s.trapDice)
	if err != nil {
		return nil, err
	}
	fireID := uuid.New().String()
	for i, o := range outcomes {
		var ch link.Character
		if i < len(caught) {
			ch = caught[i]
		} else {
			// A creature: it is a line only (no stored hit points), under its owner's
			// character, who reads it and whose player owns it.
			cr := creatures[i-len(caught)]
			ch = link.Character{ID: cr.ID}
		}
		cc := trapCaughtEvent{Target: ch.ID, Character: ch.ID, Player: ch.Player, Conditions: o.conditions, Saves: savesEventOf(o.saves)}
		if i >= len(caught) {
			cc.Character = creatures[i-len(caught)].OwnerCharacterID
		}
		for _, a := range o.attacks {
			outcome := outcomeMiss
			switch {
			case a.hit && a.critical:
				outcome = outcomeCrit
			case a.hit:
				outcome = outcomeHit
			}
			cc.Attacks = append(cc.Attacks, trapAttackEvent{D20: clampInt32(a.d20), Modifier: clampInt32(a.bonus), Total: clampInt32(a.total), Outcome: outcome, TargetAC: clampInt32(a.armorClass)})
		}
		for _, d := range o.damages {
			if d.amount <= 0 && !d.half {
				continue
			}
			de := trapDamageEvent{
				Target: ch.ID, Amount: clampInt32(d.amount), Half: d.half, Type: d.damageType,
				DiceCount: clamp32(d.count, 0, 100), DiceSides: clamp32(d.sides, 0, 100), Bonus: clamp32(d.bonus, -1000, 1000),
				Faces: faces32(d.faces), RollTotal: clampInt32(d.rollTotal), Critical: d.critical, CriticalMax: clamp32(d.criticalMax, 0, 10000), MaxRule: d.maxRule,
			}
			if ch.Player {
				row, err := c.q.InsertTrapDamage(ctx, playdb.InsertTrapDamageParams{
					GameSessionID: c.session.ID, TrapPointID: trap.PointID, FireID: fireID, CharacterID: ch.ID, Critical: d.critical,
					DiceCount: de.DiceCount, DiceSides: de.DiceSides, DiceBonus: de.Bonus, DamageType: d.damageType, Faces: de.Faces,
					RollTotal: de.RollTotal, Half: d.half, Amount: de.Amount, CreatedAt: c.now, CriticalMax: de.CriticalMax,
				})
				if err != nil {
					return nil, fmt.Errorf("open the trap's damage: %w", err)
				}
				de.Pending = row.ID
			} else {
				de.Applied = true // an NPC outside a combat has no hit points: the line says what it took
			}
			cc.Damages = append(cc.Damages, de)
		}
		ev.Caught = append(ev.Caught, cc)
	}
	return ev, nil
}

// TokenDropped implements maps.TrapFirer: the master dropped a player's
// character's token on a square, and the armed "Ao entrar na área" traps whose area
// holds it fire (during a session, while no combat runs on the map; in a combat the
// character's moves are the combat's). A failure is the maps module's to log.
func (s *Service) TokenDropped(ctx context.Context, campaignID, mapID, characterID, actorUserID string, at grid.Square) error {
	if s.traps == nil {
		return nil
	}
	session, err := s.queries.GetOpenGameSession(ctx, campaignID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // only during a session: the vitals and the history need one
	}
	if err != nil {
		return fmt.Errorf("find the open session: %w", err)
	}
	if theatre, err := theatreRunning(ctx, s.queries, session.ID); err != nil || theatre {
		return err // a combat without a map has no traps: nothing fires (RN-25)
	}
	if _, inCombat, err := s.runningEncounterOn(ctx, session.ID, mapID); err != nil || inCombat {
		return err
	}
	traps, err := s.traps.Traps(ctx, nil, campaignID, mapID)
	if err != nil {
		return err
	}
	for _, t := range traps {
		if !t.Armed() || !t.OnEnter || !t.Covers(at) {
			continue
		}
		targets := []string{characterID}
		if t.Spec.GetEffect().GetTargets() != rulesv1.TrapTargets_TRAP_TARGETS_MANUAL {
			targets = nil // everyone standing in the area, the one that landed included
		}
		fired, _, _, err := s.fireOutsideCombat(ctx, campaignID, actorUserID, "", nil, t, targets, nil, false, "")
		if err != nil && connect.CodeOf(err) == connect.CodeFailedPrecondition {
			continue // fired or disarmed since it was read: it does not fire twice
		}
		if err != nil {
			return err
		}
		s.afterFiring(ctx, campaignID, fired, playdb.Encounter{})
	}
	return nil
}

// CreatureDropped implements maps.TrapFirer: the master dropped a creature of a
// player's character on a square (9.10 left this to the traps slice), and the armed
// "Ao entrar na área" traps whose area holds it fire, as for a character's token: the
// characters standing in the area are caught with it, or only the creature when the
// trap picks its targets by hand. The creature's part is a line only: no hit points
// are stored for it outside a combat (as for an NPC), so nothing waits for the master.
func (s *Service) CreatureDropped(ctx context.Context, campaignID, mapID string, creature maplink.MapCreature, actorUserID string, at grid.Square) error {
	if s.traps == nil {
		return nil
	}
	session, err := s.queries.GetOpenGameSession(ctx, campaignID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("find the open session: %w", err)
	}
	if theatre, err := theatreRunning(ctx, s.queries, session.ID); err != nil || theatre {
		return err // a combat without a map has no traps: nothing fires (RN-25)
	}
	if _, inCombat, err := s.runningEncounterOn(ctx, session.ID, mapID); err != nil || inCombat {
		return err // in a combat the creature's moves are the combat's
	}
	traps, err := s.traps.Traps(ctx, nil, campaignID, mapID)
	if err != nil {
		return err
	}
	for _, t := range traps {
		if !t.Armed() || !t.OnEnter || !t.Covers(at) {
			continue
		}
		var targets []string // everyone standing in the area
		if t.Spec.GetEffect().GetTargets() == rulesv1.TrapTargets_TRAP_TARGETS_MANUAL {
			targets = []string{} // only the creature
		}
		fired, _, _, err := s.fireOutsideCombat(ctx, campaignID, actorUserID, "", nil, t, targets, []maplink.MapCreature{creature}, false, "")
		if err != nil && connect.CodeOf(err) == connect.CodeFailedPrecondition {
			continue
		}
		if err != nil {
			return err
		}
		s.afterFiring(ctx, campaignID, fired, playdb.Encounter{})
	}
	return nil
}
