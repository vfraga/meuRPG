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

// fireRequest is a FireTrap request that passed the checks that need no read: the key, the
// trap's map and point (normalized), the creatures picked (nil for the creatures in the area)
// and the firing it adds them to, if any.
type fireRequest struct {
	key     string
	mapID   string
	pointID string
	targets []string
	extend  string
}

// errFiringNotFound is the answer for a firing to extend that is not there.
func errFiringNotFound() error {
	return connect.NewError(connect.CodeNotFound, errors.New("firing not found"))
}

// parseFireRequest reads and checks what a FireTrap request says on its own.
func parseFireRequest(msg *playv1.FireTrapRequest) (fireRequest, error) {
	key, err := parseKey(msg.GetIdempotencyKey())
	if err != nil {
		return fireRequest{}, err
	}
	mapID, err := uuid.Parse(msg.GetMapId())
	if err != nil {
		return fireRequest{}, errTrapNotFound()
	}
	pointID, err := uuid.Parse(msg.GetPointId())
	if err != nil {
		return fireRequest{}, errTrapNotFound()
	}
	targets, err := parseFireTargets(msg.GetTargetIds())
	if err != nil {
		return fireRequest{}, err
	}
	extend := msg.GetExtendFiringId()
	if err := checkFireExtend(extend, targets); err != nil {
		return fireRequest{}, err
	}
	return fireRequest{key: key, mapID: mapID.String(), pointID: pointID.String(), targets: targets, extend: extend}, nil
}

// parseFireTargets checks the creatures the master picks: none means the creatures in the area.
func parseFireTargets(ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil // the creatures in the area
	}
	if len(ids) > maxTrapTargets {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("target_ids must have at most %d ids", maxTrapTargets))
	}
	for _, id := range ids {
		if _, err := uuid.Parse(id); err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("target_ids must be UUIDs"))
		}
	}
	return ids, nil
}

// checkFireExtend checks the firing to extend: it needs the creatures to add.
func checkFireExtend(extend string, targets []string) error {
	if extend == "" {
		return nil
	}
	if _, err := uuid.Parse(extend); err != nil {
		return errFiringNotFound()
	}
	if len(targets) == 0 {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("extend_firing_id needs the target_ids to add"))
	}
	return nil
}

// FireTrap implements playv1connect.PlayServiceHandler.
func (s *Service) FireTrap(
	ctx context.Context,
	req *connect.Request[playv1.FireTrapRequest],
) (*connect.Response[playv1.FireTrapResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	fr, err := parseFireRequest(req.Msg)
	if err != nil {
		return nil, err
	}
	trap, err := s.trapToFire(ctx, m.CampaignID, fr)
	if err != nil {
		return nil, err
	}
	session, err := s.openSession(ctx, m.CampaignID)
	if err != nil {
		return nil, err
	}
	if err := s.refuseFiringWithoutAMap(ctx, session.ID, fr.key); err != nil {
		return nil, err
	}
	enc, inCombat, err := s.runningEncounterOn(ctx, session.ID, trap.MapID)
	if err != nil {
		return nil, s.dbError(ctx, "find the combat", err)
	}
	if fr.extend != "" {
		if err := s.checkFiringExtendable(ctx, session.ID, fr, trap, enc, inCombat); err != nil {
			return nil, err
		}
	}
	if inCombat {
		return s.fireByHandInCombat(ctx, m, fr.key, trap, enc, fr.targets, fr.extend)
	}
	return s.fireByHandOutsideCombat(ctx, m, session.ID, fr, trap)
}

// trapToFire reads the trap the request names off its map.
func (s *Service) trapToFire(ctx context.Context, campaignID string, fr fireRequest) (maplink.Trap, error) {
	if s.traps == nil {
		return maplink.Trap{}, errNoTraps()
	}
	traps, err := s.traps.Traps(ctx, nil, campaignID, fr.mapID)
	if err != nil {
		return maplink.Trap{}, s.dbError(ctx, "read the map's traps", err)
	}
	i := slices.IndexFunc(traps, func(t maplink.Trap) bool { return t.PointID == fr.pointID })
	if i < 0 {
		return maplink.Trap{}, errTrapNotFound()
	}
	return traps[i], nil
}

// refuseFiringWithoutAMap refuses a firing while a combat with no map runs: it has no traps
// (RN-25). A retry of a firing made before it began (the key is already in the history) still
// gets its stored answer.
func (s *Service) refuseFiringWithoutAMap(ctx context.Context, sessionID, key string) error {
	theatre, err := theatreRunning(ctx, s.queries, sessionID)
	if err != nil {
		return s.dbError(ctx, "find the combat", err)
	}
	if !theatre {
		return nil
	}
	seen, err := keySeen(ctx, s.queries, sessionID, key)
	if err != nil {
		return s.dbError(ctx, "find the event of this idempotency key", err)
	}
	if !seen {
		return errNeedsAMap()
	}
	return nil
}

// checkFiringExtendable checks the firing to extend: it is this trap's, from this session, in the
// same place (a combat, or outside one), made once and not itself an extension, and the trap is
// as it left it.
func (s *Service) checkFiringExtendable(ctx context.Context, sessionID string, fr fireRequest, trap maplink.Trap, enc playdb.Encounter, inCombat bool) error {
	host, hostEncounter, err := s.firingToExtend(ctx, sessionID, fr.extend, trap.PointID)
	if err != nil {
		return err
	}
	if host.ExtendsID != "" || trap.State != "triggered" || !firedWhere(hostEncounter, enc, inCombat) {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("that firing cannot be extended now"))
	}
	return nil
}

// firingToExtend reads the event of the firing to extend: the firing and the combat it was made in
// (nil for outside one). An event that is not a firing of the trap is not found.
func (s *Service) firingToExtend(ctx context.Context, sessionID, extend, pointID string) (*trapFireEvent, *string, error) {
	row, err := s.queries.GetSessionEventByID(ctx, playdb.GetSessionEventByIDParams{GameSessionID: sessionID, ID: extend})
	var host actionEvent
	if err == nil {
		err = json.Unmarshal(row.Payload, &host)
	}
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (row.Kind != eventTrapTriggered || host.Trap == nil || host.Trap.PointID != pointID)) {
		return nil, nil, errFiringNotFound()
	}
	if err != nil {
		return nil, nil, s.dbError(ctx, "read the firing", err)
	}
	return host.Trap, row.EncounterID, nil
}

// firedWhere says the firing was made where the new one is: in the running combat, or outside one.
func firedWhere(hostEncounter *string, enc playdb.Encounter, inCombat bool) bool {
	if inCombat {
		return hostEncounter != nil && *hostEncounter == enc.ID
	}
	return hostEncounter == nil
}

// fireByHandOutsideCombat is FireTrap while no combat runs on the trap's map.
func (s *Service) fireByHandOutsideCombat(ctx context.Context, m authz.Membership, sessionID string, fr fireRequest, trap maplink.Trap) (*connect.Response[playv1.FireTrapResponse], error) {
	fired, firingID, repeated, err := s.fireOutsideCombat(ctx, m.CampaignID, m.UserID, fr.key, fireHash(trap, fr.extend, fr.targets), trap, fr.targets, nil, true, fr.extend)
	if err != nil {
		return nil, s.dbError(ctx, "fire a trap", err)
	}
	if !repeated {
		s.afterFiring(ctx, m.CampaignID, fired, playdb.Encounter{})
	} else if fired, err = s.firingWithParts(ctx, sessionID, firingID, fired); err != nil {
		return nil, s.dbError(ctx, "read the firing", err) // the answer of the first call listed every creature, whatever the events it took
	}
	names, err := s.characterLabels(ctx, m.CampaignID, fired)
	if err != nil {
		return nil, s.dbError(ctx, "read the characters' names", err)
	}
	if fr.extend != "" {
		firingID = fr.extend
	}
	return connect.NewResponse(&playv1.FireTrapResponse{Firing: firingProto(fired, firingID, trap.Name, trapView{master: true, names: s.namesFor(ctx, m.CampaignID), label: func(id string) string { return names[id] }})}), nil
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
		ev, err := s.fireByHandChange(ctx, c, trap, targetIDs, extend)
		if err != nil {
			return nil, err
		}
		made = ev
		return made, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "fire a trap", err)
	}
	ev, err := resultEvent(res, made)
	if err != nil {
		return nil, s.dbError(ctx, "read the firing", err)
	}
	if _, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.afterFiring(ctx, m.CampaignID, ev.Trap, d.enc)
	}); err != nil {
		return nil, err
	}
	firingID, fired, err := s.firingInCombatAnswer(ctx, res, key, extend, ev.Trap)
	if err != nil {
		return nil, err
	}
	label := s.membersOf(ctx, res)
	return connect.NewResponse(&playv1.FireTrapResponse{Firing: firingProto(fired, firingID, trap.Name, trapView{master: true, names: s.namesFor(ctx, res.session.CampaignID), label: func(id string) string {
		c, _ := label(id)
		return c.Label
	}})}), nil
}

// fireByHandChange is the change a firing by hand makes in a combat: the creatures it catches
// are rolled and the combat is touched.
func (s *Service) fireByHandChange(ctx context.Context, c *combatTx, trap maplink.Trap, targetIDs []string, extend string) (actionEvent, error) {
	if err := notEnded(c.enc); err != nil {
		return actionEvent{}, err
	}
	caught, err := s.caughtByHand(ctx, c, trap, targetIDs)
	if err != nil {
		return actionEvent{}, err
	}
	fired, err := s.fireInCombat(ctx, c, trap, caught, true, extend)
	if errors.Is(err, maplink.ErrTrapNotArmed) {
		return actionEvent{}, errTrapNotArmed()
	}
	if err != nil {
		return actionEvent{}, err
	}
	if err = touchEncounter(ctx, c); err != nil {
		return actionEvent{}, err
	}
	return actionEvent{Round: c.enc.Round, Trap: fired}, nil
}

// caughtByHand are the combatants the master picked, or, with none picked, the ones in the area.
func (s *Service) caughtByHand(ctx context.Context, c *combatTx, trap maplink.Trap, targetIDs []string) ([]playdb.Combatant, error) {
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
	return caught, nil
}

// firingInCombatAnswer is the id of the firing a firing by hand in a combat answers with, and the
// firing: the one it extends, or the event written under the key. A retry answers with every
// creature the first call caught, whatever the events it took.
func (s *Service) firingInCombatAnswer(ctx context.Context, res combatResult, key, extend string, fired *trapFireEvent) (string, *trapFireEvent, error) {
	if extend != "" {
		return extend, fired, nil
	}
	row, err := s.queries.GetSessionEventByIdempotencyKey(ctx, playdb.GetSessionEventByIdempotencyKeyParams{GameSessionID: res.session.ID, IdempotencyKey: &key})
	if err != nil {
		return "", nil, s.dbError(ctx, "read the firing", err)
	}
	if res.repeated {
		if fired, err = s.firingWithParts(ctx, res.session.ID, row.ID, fired); err != nil {
			return "", nil, s.dbError(ctx, "read the firing", err)
		}
	}
	return row.ID, fired, nil
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

// outsideFiring is a firing outside a combat: what it asks, and what its transaction leaves.
type outsideFiring struct {
	campaignID  string
	actorUserID string
	key         string
	hash        *string
	trap        maplink.Trap
	caught      []link.Character
	creatures   []maplink.MapCreature
	manual      bool
	extend      string

	fired    *trapFireEvent
	firingID string
	repeated bool
}

// fireOutsideCombat fires the trap while no combat runs on its map, in a
// transaction of its own, and returns the firing and whether it was a retry. key
// is "" for a firing nobody retries (a token landing); targetIDs are characters
// with tokens on the map, nil for every character in the area (an empty, non-nil
// list catches none: a creature's token dropped into a trap that picks its targets by
// hand); creatures are the creature tokens that are caught as well.
func (s *Service) fireOutsideCombat(ctx context.Context, campaignID, actorUserID, key string, hash *string, trap maplink.Trap, targetIDs []string, creatures []maplink.MapCreature, manual bool, extend string) (*trapFireEvent, string, bool, error) {
	caught, creatures, err := s.outsideCaught(ctx, campaignID, trap, targetIDs, creatures)
	if err != nil {
		return nil, "", false, err
	}
	if s.afterTrapRead != nil {
		s.afterTrapRead() // a test starts a combat here
	}
	f := &outsideFiring{campaignID: campaignID, actorUserID: actorUserID, key: key, hash: hash, trap: trap, caught: caught, creatures: creatures, manual: manual, extend: extend}
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error { return s.fireOutsideTx(ctx, tx, f) })
	if errors.Is(err, maplink.ErrTrapNotArmed) {
		return nil, "", false, errTrapNotArmed()
	}
	return f.fired, f.firingID, f.repeated, err
}

// outsideCaught reads who a firing outside a combat catches: the characters whose tokens stand in
// the area (or the ones named), and the creatures, which are the ones given and, for a firing that
// picks everyone in the area, the ones already standing there.
func (s *Service) outsideCaught(ctx context.Context, campaignID string, trap maplink.Trap, targetIDs []string, creatures []maplink.MapCreature) ([]link.Character, []maplink.MapCreature, error) {
	at, living, creatureAt, err := s.tokensOn(ctx, campaignID, trap.MapID)
	if err != nil {
		return nil, nil, err
	}
	if targetIDs == nil {
		// Everyone in the area: the creatures already standing there too, besides the one just dropped.
		inArea, err := s.creaturesInArea(ctx, campaignID, creatureAt, trap)
		if err != nil {
			return nil, nil, err
		}
		for _, c := range inArea {
			if !slices.ContainsFunc(creatures, func(o maplink.MapCreature) bool { return o.ID == c.ID }) {
				creatures = append(creatures, c)
			}
		}
	}
	caught := charactersCaught(living, at, trap, targetIDs)
	for _, id := range targetIDs {
		if !slices.ContainsFunc(caught, func(ch link.Character) bool { return ch.ID == id }) {
			return nil, nil, connect.NewError(connect.CodeInvalidArgument, errors.New("target_ids must be characters with a token on the trap's map"))
		}
	}
	return caught, creatures, nil
}

// charactersCaught are the living characters a trap catches: the ones named, or, with none
// named (nil), the ones whose tokens stand in the area.
func charactersCaught(living []link.Character, at map[string]grid.Square, trap maplink.Trap, targetIDs []string) []link.Character {
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
	return caught
}

// fireOutsideTx is a firing outside a combat inside its transaction: a retry answers with the
// firing it made, else the trap fires and its event is written.
func (s *Service) fireOutsideTx(ctx context.Context, tx pgx.Tx, f *outsideFiring) error {
	q := s.queries.WithTx(tx)
	f.fired, f.firingID, f.repeated = nil, "", false
	session, err := q.GetOpenGameSessionForUpdate(ctx, f.campaignID)
	if errors.Is(err, pgx.ErrNoRows) {
		return errNoOpenSession()
	}
	if err != nil {
		return fmt.Errorf("lock the open session: %w", err)
	}
	if done, err := f.replay(ctx, q, session.ID); err != nil || done {
		return err
	}
	if err := refuseFiringBesideCombat(ctx, q, session.ID, f.trap.MapID); err != nil {
		return err
	}
	c, err := s.openTx(ctx, combatTx{tx: tx, q: q, session: session, now: s.now(), actorUserID: f.actorUserID, hash: f.hash})
	if err != nil {
		return err
	}
	if f.fired, err = s.fireOutside(ctx, c, f.trap, f.caught, f.creatures, f.manual, f.extend); err != nil {
		return err
	}
	var keyPtr *string
	if f.key != "" {
		keyPtr = &f.key
	}
	f.firingID, err = insertSceneEvent(ctx, c, eventTrapTriggered, &f.actorUserID, keyPtr, actionEvent{Trap: f.fired})
	return err
}

// replay finds the firing the key made already, and answers with it: the same firing asked the same way.
func (f *outsideFiring) replay(ctx context.Context, q *playdb.Queries, sessionID string) (bool, error) {
	if f.key == "" {
		return false, nil
	}
	done, err := q.GetSessionEventByIdempotencyKey(ctx, playdb.GetSessionEventByIdempotencyKeyParams{GameSessionID: sessionID, IdempotencyKey: &f.key})
	switch {
	case err == nil:
		if done.Kind != eventTrapTriggered || hashDiffers(done.IdempotencyHash, f.hash) {
			return false, connect.NewError(connect.CodeInvalidArgument, errors.New("idempotency_key was already used for another change"))
		}
		var ev actionEvent
		if err := json.Unmarshal(done.Payload, &ev); err != nil {
			return false, fmt.Errorf("decode the event of %s: %w", done.ID, err)
		}
		f.fired, f.firingID, f.repeated = ev.Trap, done.ID, true
		return true, nil
	case !errors.Is(err, pgx.ErrNoRows):
		return false, fmt.Errorf("find the event of this idempotency key: %w", err)
	}
	return false, nil
}

// refuseFiringBesideCombat checks, under the session's lock, that no combat runs on the trap's
// map. What was read before the lock may be stale: a combat that began since is the one the trap
// fires in, and a firing outside one must not land beside it.
func refuseFiringBesideCombat(ctx context.Context, q *playdb.Queries, sessionID, mapID string) error {
	enc, err := q.GetLatestEncounter(ctx, sessionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // no combat at all
	}
	if err != nil {
		return fmt.Errorf("find the session's combat: %w", err)
	}
	switch {
	case enc.Status == statusEnded:
	case isTheatre(enc):
		return errNeedsAMap() // a combat without a map has no traps (RN-25)
	case enc.MapID != nil && *enc.MapID == mapID:
		return errCombatBegan()
	}
	return nil
}

// fireOutside resolves the trap against characters, inside the transaction: the
// trap is triggered, the effect rolled, and the damage to a player's character
// waits for the master in trap_damages. An NPC has no hit points stored outside a
// combat: its damage is only in the event. Conditions are a reminder in it.
func (s *Service) fireOutside(ctx context.Context, c *combatTx, trap maplink.Trap, caught []link.Character, creatures []maplink.MapCreature, manual bool, extend string) (*trapFireEvent, error) {
	ev := &trapFireEvent{PointID: trap.PointID, MapID: trap.MapID, Manual: manual, ExtendsID: extend}
	prev, err := s.triggerFiredTrap(ctx, c, trap, ev)
	if err != nil {
		return nil, err
	}
	effect := prev.Spec.GetEffect()
	targets, err := s.outsideTargets(ctx, c, effect, caught, creatures)
	if err != nil {
		return nil, err
	}
	outcomes, err := resolveTrap(effect, targets, criticalRuleOf(c.rules), s.trapD20, s.trapDice, s.trapExtraDice)
	if err != nil {
		return nil, err
	}
	if err := s.spendTrapDice(ctx, c, outcomes); err != nil {
		return nil, err
	}
	fireID := uuid.New().String()
	for i, o := range outcomes {
		ch, character := outsideWho(caught, creatures, i)
		cc, err := s.caughtOutside(ctx, c, trap.PointID, fireID, ch, character, o)
		if err != nil {
			return nil, err
		}
		ev.Caught = append(ev.Caught, cc)
	}
	return ev, nil
}

// outsideTargets works out what the arithmetic needs of each creature caught outside a combat: the
// characters first, then the creatures, which have no hit points stored either, so their part is
// only the line, as an NPC's.
func (s *Service) outsideTargets(ctx context.Context, c *combatTx, effect *rulesv1.TrapEffect, caught []link.Character, creatures []maplink.MapCreature) ([]trapTarget, error) {
	ability := trapSaveAbility(effect)
	targets := make([]trapTarget, len(caught)+len(creatures))
	for i, ch := range caught {
		t, err := s.outsideCharacterTarget(ctx, c, effect, ability, ch)
		if err != nil {
			return nil, err
		}
		targets[i] = t
	}
	for j, cr := range creatures {
		t, err := s.outsideCreatureTarget(ctx, c, effect, ability, cr)
		if err != nil {
			return nil, err
		}
		targets[len(caught)+j] = t
	}
	return targets, nil
}

// outsideCharacterTarget is a character caught outside a combat: its armor class when the trap
// attacks, and its saving throw when it asks one.
func (s *Service) outsideCharacterTarget(ctx context.Context, c *combatTx, effect *rulesv1.TrapEffect, ability string, ch link.Character) (trapTarget, error) {
	campaignID := c.session.CampaignID
	t := trapTarget{id: ch.ID}
	if trapNeedsAC(effect) {
		sheet, err := s.roster.CombatSheet(ctx, c.tx, campaignID, ch.ID)
		if err != nil {
			return trapTarget{}, err
		}
		if t.armorClass, err = s.armorWithSpells(ctx, c.q, campaignID, ch.ID, sheet.ArmorClass); err != nil {
			return trapTarget{}, err
		}
	}
	if ability == "" {
		return t, nil
	}
	save, err := s.roster.CombatSave(ctx, c.tx, campaignID, ch.ID, ability)
	if err != nil {
		return trapTarget{}, err
	}
	t.save, t.saveKnown = save.Bonus, save.Known
	if t.saveDice, err = s.outsideSaveDice(ctx, c.tx, ch.ID); err != nil {
		return trapTarget{}, err
	}
	if t.lucky, err = s.outsideLucky(ctx, c.tx, campaignID, ch); err != nil {
		return trapTarget{}, err
	}
	return t, nil
}

// outsideCreatureTarget is a creature caught outside a combat: its armor class when the trap
// attacks, and its saving throw when it asks one.
func (s *Service) outsideCreatureTarget(ctx context.Context, c *combatTx, effect *rulesv1.TrapEffect, ability string, cr maplink.MapCreature) (trapTarget, error) {
	campaignID := c.session.CampaignID
	t := trapTarget{id: cr.ID}
	if trapNeedsAC(effect) {
		sheet, ok, err := s.roster.CreatureSheet(ctx, c.tx, campaignID, cr.MonsterKey, "")
		if err != nil {
			return trapTarget{}, err
		}
		if !ok {
			return trapTarget{}, errCombatantNotFound()
		}
		t.armorClass = sheet.ArmorClass
	}
	if ability != "" {
		save, err := s.roster.CreatureSave(ctx, c.tx, campaignID, cr.MonsterKey, ability)
		if err != nil {
			return trapTarget{}, err
		}
		t.save, t.saveKnown = save.Bonus, save.Known
	}
	return t, nil
}

// outsideWho is the creature at place i of the outcomes, a character or, after the characters, a
// creature, and the character its line is under. A creature is a line only (no stored hit
// points), under its owner's character, who reads it and whose player owns it.
func outsideWho(caught []link.Character, creatures []maplink.MapCreature, i int) (link.Character, string) {
	if i < len(caught) {
		return caught[i], caught[i].ID
	}
	cr := creatures[i-len(caught)]
	return link.Character{ID: cr.ID}, cr.OwnerCharacterID
}

// caughtOutside is what the trap did to one creature outside a combat.
func (s *Service) caughtOutside(ctx context.Context, c *combatTx, pointID, fireID string, ch link.Character, character string, o trapOutcome) (trapCaughtEvent, error) {
	cc := trapCaughtEvent{Target: ch.ID, Character: character, Player: ch.Player, Conditions: o.conditions, Saves: savesEventOf(o.saves)}
	for _, a := range o.attacks {
		cc.Attacks = append(cc.Attacks, trapAttackEventOf(a))
	}
	for _, d := range o.damages {
		if d.amount <= 0 && !d.half {
			continue
		}
		de, err := s.damageOutside(ctx, c, pointID, fireID, ch, d)
		if err != nil {
			return trapCaughtEvent{}, err
		}
		cc.Damages = append(cc.Damages, de)
	}
	return cc, nil
}

// damageOutside records one damage part for a creature outside a combat: a player's character's
// waits for the master as a trap damage; an NPC's, or a creature's, is only the line, applied.
func (s *Service) damageOutside(ctx context.Context, c *combatTx, pointID, fireID string, ch link.Character, d trapDamageRoll) (trapDamageEvent, error) {
	de := trapDamageEventOf(d)
	de.damageHit = damageHit{Target: ch.ID, Amount: clampInt32(d.amount), Half: d.half}
	if !ch.Player {
		de.Applied = true // an NPC outside a combat has no hit points: the line says what it took
		return de, nil
	}
	row, err := c.q.InsertTrapDamage(ctx, playdb.InsertTrapDamageParams{
		GameSessionID: c.session.ID, TrapPointID: pointID, FireID: fireID, CharacterID: ch.ID, Critical: d.critical,
		DiceCount: de.DiceCount, DiceSides: de.DiceSides, DiceBonus: de.Bonus, DamageType: d.damageType, Faces: de.Faces,
		RollTotal: de.RollTotal, Half: d.half, Amount: de.Amount, CreatedAt: c.now, CriticalMax: de.CriticalMax,
	})
	if err != nil {
		return trapDamageEvent{}, fmt.Errorf("open the trap's damage: %w", err)
	}
	de.Pending = row.ID
	return de, nil
}

// outsideLucky says the character a trap caught outside a combat is a halfling player's
// character: the app rolls a 1 on its saving throw again by itself (combat_lucky.go). An NPC
// never is, and its sheet is not read.
func (s *Service) outsideLucky(ctx context.Context, tx pgx.Tx, campaignID string, ch link.Character) (bool, error) {
	if !ch.Player {
		return false, nil
	}
	sheet, err := s.roster.CombatSheet(ctx, tx, campaignID, ch.ID)
	if err != nil {
		return false, err
	}
	return sheet.Traits.Lucky, nil
}

// TokenDropped implements maps.TrapFirer: the master dropped a player's
// character's token on a square, and the armed "Ao entrar na área" traps whose area
// holds it fire (during a session, while no combat runs on the map; in a combat the
// character's moves are the combat's). A failure is the maps module's to log.
func (s *Service) TokenDropped(ctx context.Context, campaignID, mapID, characterID, actorUserID string, at grid.Square) error {
	traps, err := s.trapsDroppedOn(ctx, campaignID, mapID, at)
	if err != nil {
		return err
	}
	for _, t := range traps {
		targets := []string{characterID}
		if t.Spec.GetEffect().GetTargets() != rulesv1.TrapTargets_TRAP_TARGETS_MANUAL {
			targets = nil // everyone standing in the area, the one that landed included
		}
		if err := s.fireDropped(ctx, campaignID, actorUserID, t, targets, nil); err != nil {
			return err
		}
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
	traps, err := s.trapsDroppedOn(ctx, campaignID, mapID, at)
	if err != nil {
		return err
	}
	for _, t := range traps {
		var targets []string // everyone standing in the area
		if t.Spec.GetEffect().GetTargets() == rulesv1.TrapTargets_TRAP_TARGETS_MANUAL {
			targets = []string{} // only the creature
		}
		if err := s.fireDropped(ctx, campaignID, actorUserID, t, targets, []maplink.MapCreature{creature}); err != nil {
			return err
		}
	}
	return nil
}

// trapsDroppedOn are the armed "Ao entrar na área" traps whose area holds the square a token or a
// creature was dropped on, when they may fire: only during a session (the vitals and the history
// need one), and not while a combat runs on the map, where the moves are the combat's, nor in a
// combat without a map, which has no traps (RN-25).
func (s *Service) trapsDroppedOn(ctx context.Context, campaignID, mapID string, at grid.Square) ([]maplink.Trap, error) {
	if s.traps == nil {
		return nil, nil
	}
	session, err := s.queries.GetOpenGameSession(ctx, campaignID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find the open session: %w", err)
	}
	if theatre, err := theatreRunning(ctx, s.queries, session.ID); err != nil || theatre {
		return nil, err
	}
	if _, inCombat, err := s.runningEncounterOn(ctx, session.ID, mapID); err != nil || inCombat {
		return nil, err
	}
	traps, err := s.traps.Traps(ctx, nil, campaignID, mapID)
	if err != nil {
		return nil, err
	}
	var hit []maplink.Trap
	for _, t := range traps {
		if t.Armed() && t.OnEnter && t.Covers(at) {
			hit = append(hit, t)
		}
	}
	return hit, nil
}

// fireDropped fires a trap a token or a creature was dropped in, outside a combat, and tells the
// streams. A trap fired or disarmed since it was read does not fire twice.
func (s *Service) fireDropped(ctx context.Context, campaignID, actorUserID string, t maplink.Trap, targets []string, creatures []maplink.MapCreature) error {
	fired, _, _, err := s.fireOutsideCombat(ctx, campaignID, actorUserID, "", nil, t, targets, creatures, false, "")
	if err != nil && connect.CodeOf(err) == connect.CodeFailedPrecondition {
		return nil
	}
	if err != nil {
		return err
	}
	s.afterFiring(ctx, campaignID, fired, playdb.Encounter{})
	return nil
}
