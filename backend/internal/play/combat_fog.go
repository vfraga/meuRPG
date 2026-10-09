package play

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	maplink "github.com/PuraFome/meuRPG/backend/internal/maps/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/live"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
	"github.com/PuraFome/meuRPG/backend/internal/rules/vision"
)

// Combat on a map with the fog of war on (MR-036, RN-10, RN-20, Etapa 9, slice
// 9.7). A player sees only the NPC combatants their character sees now (with
// "Visão do grupo", the party); an NPC they do not see is, for them, a hidden
// combatant: not in the order, no name, no state, no square, its turn "Vez do
// mestre", and not found when they name it. Players' characters and their
// creatures are never hidden (the party knows where its people are). The master
// sees everyone. A map without the fog behaves as before.
//
// How it is built:
//
//   - The maps module says what each player sees (FogSource.CombatSight); this
//     file turns that into the set of combatants a viewer does not see
//     (combatViewer.unseen), and every place that asked "does the viewer see it?"
//     (combatViewer.sees) now gets the fog's answer too: the order, the turn, the
//     targets, the pending damage, the reaction prompts, the offers, the move's
//     plan, and the not_found of naming an NPC.
//   - The sight is read before a change opens its transaction (write), never inside
//     it: the maps module asks this one where the combatants stand, and a read of a
//     row the transaction already changed would wait for it.
//   - The log keeps, with each event, who could see it when it happened
//     (actionEvent.SeenBy): a line stays visible to those who saw its NPCs then, and
//     one they did not see never appears later, whatever the view is now.
//   - The live events that carry a combatant (combatant_moved, turn_changed) go to
//     each player in the form they may see, or as the content-free
//     encounter_changed hint (publishCombatantMoved, publishTurnChanged).

// FogSource is what a combat asks of the maps module's fog of war. The maps module
// implements it (maps.Service) and cmd/api connects it with SetFog. A nil source
// is a world without the fog.
type FogSource interface {
	// CombatSight returns what the players see of the map now, or nil when the map
	// has no fog of war (or is gone).
	CombatSight(ctx context.Context, campaignID, mapID string) (maplink.CombatSight, error)
	// CombatMoved tells the fog that a combatant moved on the map: what the move
	// showed is remembered and the players who see either square are told.
	CombatMoved(ctx context.Context, campaignID, mapID string, npc bool, from, to grid.Square)
}

// SetFog connects the maps module's fog of war. play and maps need each other, so
// cmd/api calls it once maps exists, before the server starts.
func (s *Service) SetFog(f FogSource) { s.fog = f }

// fogSight is the sight of one combat's map, read once and shared: the one a change
// reads before its transaction, and the one a request reads after the commit.
type fogSight struct {
	sight maplink.CombatSight
	// rev is the encounter's revision when the sight was read: a change checks it
	// again once it holds the session lock, and reads the sight again when another
	// change got in between (errSightStale), so what it stamps is what the table
	// looked like when it happened.
	rev int32
}

// fogMemo shares, inside one request, the sights its handler needs after the
// commit: the one read before the change (who could see the old squares) and, once
// asked for, the one read after it. The lines the change wrote are here too, for the
// log's hint.
type fogMemo struct {
	pre     *fogSight
	lines   []actionEvent
	mu      sync.Mutex
	post    *fogSight
	postErr error
	done    bool
}

type fogMemoKey struct{}

// withFogMemo gives a request's context the memo finish and the publishers share.
func withFogMemo(ctx context.Context, pre *fogSight, lines []actionEvent) context.Context {
	return context.WithValue(ctx, fogMemoKey{}, &fogMemo{pre: pre, lines: lines})
}

func fogMemoOf(ctx context.Context) *fogMemo {
	m, _ := ctx.Value(fogMemoKey{}).(*fogMemo)
	return m
}

// fogSightOf reads what the players see of the encounter's map: nil when there is
// no fog on it (nothing to filter), and an error when it cannot be told, which
// fails the request rather than showing a player what they may not see. Inside a
// request that shares a memo (after a change) it is read once.
func (s *Service) fogSightOf(ctx context.Context, campaignID string, enc playdb.Encounter) (*fogSight, error) {
	if s.fog == nil || enc.MapID == nil {
		return nil, nil
	}
	if memo := fogMemoOf(ctx); memo != nil {
		memo.mu.Lock()
		defer memo.mu.Unlock()
		if !memo.done {
			memo.post, memo.postErr = s.fogSightOfMap(ctx, campaignID, *enc.MapID, enc.Revision)
			memo.done = true
		}
		return memo.post, memo.postErr
	}
	return s.fogSightOfMap(ctx, campaignID, *enc.MapID, enc.Revision)
}

func (s *Service) fogSightOfMap(ctx context.Context, campaignID, mapID string, rev int32) (*fogSight, error) {
	sight, err := s.fog.CombatSight(ctx, campaignID, mapID)
	if err != nil {
		return nil, fmt.Errorf("read what the players see: %w", err)
	}
	if sight == nil {
		return nil, nil
	}
	return &fogSight{sight: sight, rev: rev}, nil
}

// encounterMapOf is the map and the revision of the combat with the id, for a change
// that has not opened its transaction yet; "" when the combat has none or is not the
// campaign's.
func (s *Service) encounterMapOf(ctx context.Context, campaignID, encounterID string) (mapID string, rev int32, err error) {
	var id *string
	err = s.pool.QueryRow(ctx, `
		SELECT e.map_id, e.revision FROM encounters AS e
		JOIN game_sessions AS g ON g.id = e.game_session_id
		WHERE e.id = $1 AND g.campaign_id = $2`, encounterID, campaignID).Scan(&id, &rev)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", 0, nil
	}
	if err != nil {
		return "", 0, fmt.Errorf("read the combat's map: %w", err)
	}
	return deref(id), rev, nil
}

// sightForWrite reads the sight a change needs, with the revision it was read at: nil
// for a change that creates a combat, one whose combat has no map, or a map with no
// fog. The revision is read first, so a change that lands meanwhile shows.
func (s *Service) sightForWrite(ctx context.Context, w combatWrite) (*fogSight, error) {
	if s.fog == nil || w.encounterID == "" {
		return nil, nil
	}
	mapID, rev, err := s.encounterMapOf(ctx, w.m.CampaignID, w.encounterID)
	if err != nil || mapID == "" {
		return nil, err
	}
	f, err := s.fogSightOfMap(ctx, w.m.CampaignID, mapID, rev)
	if err != nil || f == nil {
		return f, err
	}
	// The player's known terrain is read now, outside the transaction, for the plan
	// and the cover of the change.
	if _, err := f.knownTerrain(ctx, nil, viewerOf(w.m)); err != nil {
		return nil, err
	}
	return f, nil
}

// errSightStale says another change got in between the moment the sight was read and
// the moment the transaction held the session lock: write reads the sight again.
var errSightStale = errors.New("the combat changed after the sight was read")

// maxSightTries bounds how often a change reads the sight again.
const maxSightTries = 3

// seesNPC says whether the player sees the NPC combatant. An NPC with no square
// (a combat in setup, an NPC the master has not placed) is seen by no player.
func (f *fogSight) seesNPC(userID string, c playdb.Combatant) bool {
	return placed(c) && f.sight.Sees(userID, squareOfCombatant(c))
}

// unseenFor lists the NPC combatants the player does not see. Players' characters
// and creatures are never in it (D6).
func (f *fogSight) unseenFor(userID string, cs []playdb.Combatant) map[string]bool {
	out := map[string]bool{}
	for _, c := range cs {
		if c.Kind == kindNPC && !f.seesNPC(userID, c) {
			out[c.ID] = true
		}
	}
	return out
}

// viewerFor is the viewer of a read outside a change: the caller, with the
// combatants of the fog map they do not see.
func (s *Service) viewerFor(ctx context.Context, m authz.Membership, enc playdb.Encounter, cs []playdb.Combatant) (combatViewer, error) {
	v, _, err := s.viewerWith(ctx, m, enc, cs)
	return v, err
}

// viewerWith is viewerFor that also returns the sight it read (nil for the master
// and for a map without the fog), for a read that needs more of it.
func (s *Service) viewerWith(ctx context.Context, m authz.Membership, enc playdb.Encounter, cs []playdb.Combatant) (combatViewer, *fogSight, error) {
	v := viewerOf(m)
	if v.master {
		return v, nil, nil
	}
	rules, err := s.tableRules(ctx, nil, m.CampaignID)
	if err != nil {
		return v, nil, err
	}
	v.hideDeath = rules.DeathSavesHidden
	f, err := s.fogSightOf(ctx, m.CampaignID, enc)
	if err != nil {
		return v, nil, err
	}
	if f != nil {
		v.unseen, v.sight = f.unseenFor(v.userID, cs), f
	}
	// The zones that block sight hide creatures from a player (zones.go).
	if enc.ID != "" {
		rows, err := s.queries.ListMapZones(ctx, enc.ID)
		if err != nil {
			return v, nil, fmt.Errorf("list the zones: %w", err)
		}
		v = v.withZoneSight(zoneStatesOf(rows), cs)
	}
	return v, f, nil
}

// viewer is the viewer of a change, inside its transaction: the caller, with the
// combatants of the fog map they do not see, from the sight read before the
// transaction opened. cs are the combatants as the change reads them.
func (c *combatTx) viewer(m authz.Membership, cs []playdb.Combatant) combatViewer {
	v := viewerOf(m)
	v.hideDeath = !v.master && c.rules.DeathSavesHidden
	if !v.master && c.sight != nil {
		v.unseen, v.sight = c.sight.unseenFor(v.userID, cs), c.sight
	}
	return v.withZoneSight(c.zones, cs)
}

// ---- the move: what a player plans on ----

// knownTerrain is the terrain the player knows of the map (the squares they see now
// or remember), or nil for the master and for a map without the fog: they plan on
// the real terrain.
func (f *fogSight) knownTerrain(ctx context.Context, tx pgx.Tx, v combatViewer) (*grid.Terrain, error) {
	if f == nil || v.master || v.userID == "" {
		return nil, nil
	}
	known, err := f.sight.KnownTerrain(ctx, tx, v.userID)
	if err != nil {
		return nil, fmt.Errorf("read the terrain the player knows: %w", err)
	}
	return &known, nil
}

// planOn is the terrain a move is planned on: the player's known terrain on a fog
// map, plain floor in the dark (D1), else the real one, except that a player never
// plans on a door the way it is: a locked door looks closed and a secret one is a
// wall (RN-26). The real move runs on the real terrain and is cut short where it
// is blocked, and no refusal ever names a wall, a lock or a creature the player
// does not see. The master plans on everything.
func planOn(v combatViewer, actual grid.Terrain, known *grid.Terrain) grid.Terrain {
	switch {
	case v.master:
		return actual
	case known == nil || known.Grid != actual.Grid:
		return actual.ForPlayers()
	}
	return *known
}

// ---- who sees the mover: opportunity attacks ----

// reactorSees says whether an NPC or a creature that could make an opportunity
// attack sees the mover (D1b). Without the fog everyone does (the hidden mover is
// left out by the caller). With the fog every reactor, of any kind, sees from its
// own square with its own senses (SRD: a creature perceives with its own eyes;
// darkvision, blindsight and truesight from the sheet or stat block, plain sight when
// it gives none), in the light of the map, whatever "Visão do grupo" shows its player.
// It is a pair check, so the reach filter runs first and only the reactors a move
// leaves are asked. at is the square the mover stood on, where it was when it left.
func (f *fogSight) reactorSees(r playdb.Combatant, sheet link.Sheet, at grid.Square) bool {
	return f.sight.CanSee(squareOfCombatant(r), vision.Senses{
		DarkvisionFt: sheet.Senses.DarkvisionFt, BlindsightFt: sheet.Senses.BlindsightFt, TruesightFt: sheet.Senses.TruesightFt,
	}, at)
}

// ---- the log: who could see it when it happened ----

// stamp writes into an event who could see its NPCs when it happened: Fogged, and
// SeenBy, the players (user IDs, never a name) who saw every NPC in it, at the
// square it stood on, or, for a move, on either square of it. A line with no NPC
// in it is left as it is. It is read from the sight taken before the change, which
// is how the table looked when it happened; it is never worked out again from a
// view that has changed. A Secret event (a hidden combatant is in it) is seen by no
// player: their revision does not count it and their stream does not hear it.
func (c *combatTx) stamp(ctx context.Context, kind string, ev actionEvent) (actionEvent, error) {
	ev, err := c.stampFog(ctx, kind, ev)
	if err != nil {
		return ev, err
	}
	return c.stampZones(ctx, kind, ev)
}

// stampFog is the fog's half of stamp: who could see the NPCs in the event.
func (c *combatTx) stampFog(ctx context.Context, kind string, ev actionEvent) (actionEvent, error) {
	if c.sight == nil {
		return ev, nil
	}
	cs, err := c.q.ListCombatantsWithDismissed(ctx, c.enc.ID)
	if err != nil {
		return ev, fmt.Errorf("list the combatants: %w", err)
	}
	if kind == eventDoorOpened {
		return c.stampDoor(ctx, ev, cs)
	}
	var npcSquares [][]grid.Square // for each NPC in the event, where it could have been seen
	for _, id := range ev.combatantIDs() {
		i := slices.IndexFunc(cs, func(o playdb.Combatant) bool { return o.ID == id })
		if i < 0 || cs[i].Kind != kindNPC {
			continue
		}
		var squares []grid.Square
		if placed(cs[i]) {
			squares = append(squares, squareOfCombatant(cs[i]))
		}
		if kind == eventCombatantMoved && id == ev.Actor && ev.From != nil && ev.From.Placed {
			squares = append(squares, grid.Square{Col: int(ev.From.Col), Row: int(ev.From.Row)})
		}
		if kind == eventAttackRolled && ev.OfferID != "" && id == ev.Target {
			// An opportunity attack is made at the mover on the square it left: the line
			// is judged there, whatever the mover's square is now.
			squares = []grid.Square{{Col: int(ev.Col), Row: int(ev.Row)}}
		}
		npcSquares = append(npcSquares, squares)
	}
	if ev.Trap != nil { // the firing is public; what it did to an NPC is for the players who saw it
		t := *ev.Trap
		t.Caught = slices.Clone(t.Caught)
		for i, cc := range t.Caught {
			j := slices.IndexFunc(cs, func(o playdb.Combatant) bool { return o.ID == cc.Target })
			if j < 0 || cs[j].Kind != kindNPC {
				continue
			}
			t.Caught[i].Fogged, t.Caught[i].SeenBy = true, nil
			for _, u := range c.sight.sight.Users() {
				if c.sight.seesNPC(u, cs[j]) {
					t.Caught[i].SeenBy = append(t.Caught[i].SeenBy, u)
				}
			}
		}
		ev.Trap = &t
	}
	if ev.Secret {
		ev.Fogged, ev.SeenBy = true, nil
		return ev, nil
	}
	if len(npcSquares) == 0 {
		return ev, nil
	}
	ev.Fogged = true
	ev.SeenBy = nil
	for _, u := range c.sight.sight.Users() {
		all := true
		for _, squares := range npcSquares {
			if !slices.ContainsFunc(squares, func(sq grid.Square) bool { return c.sight.sight.Sees(u, sq) }) {
				all = false
				break
			}
		}
		if all {
			ev.SeenBy = append(ev.SeenBy, u)
		}
	}
	return ev, nil
}

// stampDoor writes into a door_opened event who may have the line, on a fog map,
// whoever opened the door (a player's character or an NPC): the players whose
// character saw or remembered the door's square when it happened, from the sight
// taken before the change (the door was still a wall to the light then, which the
// terrain the player knows has as a door), and the mover's own player. Nobody else
// gets the line: it would tell where a door is and that someone walked through it
// (RN-10). Never worked out again later.
func (c *combatTx) stampDoor(ctx context.Context, ev actionEvent, cs []playdb.Combatant) (actionEvent, error) {
	door := grid.Square{Col: int(ev.Col), Row: int(ev.Row)}
	ev.Fogged, ev.SeenBy = true, nil
	own := ""
	if i := slices.IndexFunc(cs, func(o playdb.Combatant) bool { return o.ID == ev.Actor }); i >= 0 && cs[i].UserID != nil {
		own = *cs[i].UserID
	}
	for _, u := range c.sight.sight.Users() {
		if u == own {
			ev.SeenBy = append(ev.SeenBy, u)
			continue
		}
		known, err := c.sight.knownTerrain(ctx, c.tx, combatViewer{userID: u})
		if err != nil {
			return ev, err
		}
		if known != nil && known.Doors.At(door) != grid.DoorNone {
			ev.SeenBy = append(ev.SeenBy, u)
		}
	}
	return ev, nil
}

// combatantIDs lists the combatants an event is about: who did it, who it was done
// to and every target of a spell or of a damage that settled several.
func (ev actionEvent) combatantIDs() []string {
	var ids []string
	add := func(id string) {
		if id != "" && !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	add(ev.Actor)
	add(ev.Target)
	for _, h := range ev.Hits {
		if !h.HiddenAtCast { // the players' line never lists it: who sees it is no question of the line
			add(h.Target)
		}
	}
	for _, h := range ev.Settled {
		add(h.Target)
	}
	return ids
}

// seenByViewer says whether the viewer may have the event's line as far as the fog
// goes: an event written before the fog was on, or with no NPC in it, is not
// stamped and follows the old rules; a stamped one only goes to those who saw it.
func (ev actionEvent) seenByViewer(v combatViewer) bool {
	return v.master || !ev.Fogged || slices.Contains(ev.SeenBy, v.userID)
}

// ---- publishing ----

// The messages of the combat's live events, built in one place so that the master's,
// the players' and the fog's copies say the same thing.

func encounterChangedMessage(e playdb.Encounter) *playv1.WatchGameSessionResponse {
	return &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_EncounterChanged_{
		EncounterChanged: &playv1.WatchGameSessionResponse_EncounterChanged{EncounterId: e.ID, Revision: e.Revision, Mode: modeProto(e.Mode)},
	}}
}

func turnChangedMessage(e playdb.Encounter, turn turnView) *playv1.WatchGameSessionResponse {
	return &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_TurnChanged_{
		TurnChanged: &playv1.WatchGameSessionResponse_TurnChanged{
			EncounterId: e.ID, Round: e.Round, CurrentCombatantId: turn.currentID, MasterTurn: turn.masterTurn,
		},
	}}
}

func combatantMovedMessage(e playdb.Encounter, c playdb.Combatant) *playv1.WatchGameSessionResponse {
	return &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_CombatantMoved_{
		CombatantMoved: &playv1.WatchGameSessionResponse_CombatantMoved{
			EncounterId: e.ID, CombatantId: c.ID, Col: *c.GridCol, Row: *c.GridRow,
		},
	}}
}

// publishTurnChangedToPlayers tells the players who is on turn. Without the fog it is
// one event for all of them; with it, each player gets the turn as they see it (an
// NPC they do not see is "Vez do mestre"). When what they see cannot be read they
// are told nothing here, and read the combat again on the encounter_changed that
// every change of the turn also publishes.
func (s *Service) publishTurnChangedToPlayers(ctx context.Context, campaignID string, d *encounterData) {
	f, err := s.fogSightOf(ctx, campaignID, d.enc)
	zs := zoneStatesOf(d.zones)
	switch {
	case err != nil:
		s.logger.ErrorContext(ctx, "play: cannot work out what the players see in a combat", "error", err)
	case f == nil && !hasBlocking(zs):
		s.hub.Publish(campaignID, live.Event{Audience: live.Audience{Players: true}, Message: turnChangedMessage(d.enc, d.turnFor(combatViewer{}))})
	case f == nil:
		// The zones that block sight hide NPCs from some players and not from others.
		for _, u := range playerUsers(d.cs) {
			turn := d.turnFor(combatViewer{userID: u}.withZoneSight(zs, d.cs))
			s.hub.Publish(campaignID, live.Event{Audience: live.Audience{UserID: u}, Message: turnChangedMessage(d.enc, turn)})
		}
	default:
		for _, u := range f.sight.Users() {
			turn := d.turnFor(combatViewer{userID: u, unseen: f.unseenFor(u, d.cs)}.withZoneSight(zs, d.cs))
			s.hub.Publish(campaignID, live.Event{Audience: live.Audience{UserID: u}, Message: turnChangedMessage(d.enc, turn)})
		}
	}
}

// publishMovedToPlayers tells the players that a combatant moved. A player's
// character or creature, and any combatant on a map without the fog, go to every
// player as before; an NPC of a fog map goes only to the players who see its new
// square. A player who saw the square it left (in the sight read before the change)
// and not the new one gets the content-free encounter_changed, to read the combat
// again and find it gone; the others hear nothing. from is where it stood.
func (s *Service) publishMovedToPlayers(ctx context.Context, campaignID string, e playdb.Encounter, c playdb.Combatant, from *grid.Square) {
	all := live.Event{Audience: live.Audience{Players: true}, Message: combatantMovedMessage(e, c)}
	if s.zonesHide(ctx, e) {
		// A zone that blocks sight hides the squares of some creatures from some players: nobody is told
		// where a creature went, they read the combat again and find what they see (zones_sight.go).
		s.hub.Publish(campaignID, live.Event{Audience: live.Audience{Players: true}, Message: encounterChangedMessage(playdb.Encounter{ID: e.ID, Mode: e.Mode})})
		return
	}
	if c.Kind != kindNPC {
		s.hub.Publish(campaignID, all)
		return
	}
	f, err := s.fogSightOf(ctx, campaignID, e)
	switch {
	case err != nil:
		s.logger.ErrorContext(ctx, "play: cannot work out what the players see in a combat", "error", err)
		s.hub.Publish(campaignID, live.Event{Audience: live.Audience{Players: true}, Message: encounterChangedMessage(playdb.Encounter{ID: e.ID, Mode: e.Mode})})
	case f == nil:
		s.hub.Publish(campaignID, all)
	default:
		var pre *fogSight
		if memo := fogMemoOf(ctx); memo != nil {
			pre = memo.pre
		}
		for _, u := range f.sight.Users() {
			switch {
			case f.seesNPC(u, c):
				s.hub.Publish(campaignID, live.Event{Audience: live.Audience{UserID: u}, Message: combatantMovedMessage(e, c)})
			case pre != nil && from != nil && pre.sight.Sees(u, *from):
				s.hub.Publish(campaignID, live.Event{Audience: live.Audience{UserID: u}, Message: encounterChangedMessage(playdb.Encounter{ID: e.ID, Mode: e.Mode})})
			}
		}
	}
}

// positionChanged is the hook after a combat move that landed (and after the undo
// of one): the fog remembers what it showed and tells the players who see the
// squares. from and to are the squares the combatant left and reached; a combatant
// that had none stands where it was put. A hidden combatant's move is nobody's
// news: telling the players who see its squares would tell them it is there (RN-10).
func (s *Service) positionChanged(ctx context.Context, campaignID string, e playdb.Encounter, c playdb.Combatant, from *grid.Square) {
	if s.fog == nil || e.MapID == nil || !placed(c) || c.Hidden {
		return
	}
	to := squareOfCombatant(c)
	if from == nil {
		from = &to
	}
	s.fog.CombatMoved(ctx, campaignID, *e.MapID, c.Kind == kindNPC, *from, to)
}

// squareOfState is the square a move event's From says the combatant stood on, nil
// when it had none.
func squareOfState(st *moveState) *grid.Square {
	if st == nil || !st.Placed {
		return nil
	}
	return &grid.Square{Col: int(st.Col), Row: int(st.Row)}
}

// visibleRevision is the revision a player of a fog map reads: how many events of the
// combat they could see (an event with no NPC in it, one written before the fog, or
// one stamped with them in seen_by). It never goes down, and it does not go up for
// what happens out of their sight, so it cannot be used to count moves in the dark.
func (s *Service) visibleRevision(ctx context.Context, encounterID, userID string) (int32, error) {
	var n int64
	err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM session_events
		WHERE encounter_id = $1 AND (payload->'fogged' IS NULL OR payload->'seen_by' @> to_jsonb($2::STRING))`,
		encounterID, userID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count the events a player could see: %w", err)
	}
	return clamp32(int(n), 0, 1<<30), nil
}

// ---- cover: what a player is told ----

// coverPair is the cover of a target against an attacker, twice: the real one, which
// sets the armor class and the saving throw (and goes only to the master as numbers),
// and the one the player is told, worked out on the terrain they know and the
// creatures they see. On a map without the fog they are the same.
type coverPair struct {
	real, shown coverView
	// restricted says the map gave the real cover and only seenBy (user IDs) would be
	// told the same: the event keeps both so a line is read for each player as they
	// could know it (castHit.CoverRestricted, actionEvent.CoverRestricted).
	restricted bool
	seenBy     []string
}

// hiddenOut is the combatants as the cover counts them for the real roll: all for the
// master, and for a player's attack the ones the master did not hide (a hidden body
// would leak as "cover do mapa").
func hiddenOut(cs []playdb.Combatant, v combatViewer) []playdb.Combatant {
	if v.master {
		return cs
	}
	return slices.DeleteFunc(slices.Clone(cs), func(o playdb.Combatant) bool { return o.Hidden })
}

// coverOf works out the cover for a change inside its transaction. known is the
// terrain the caller's player knows (nil for the master and without the fog).
func (c *combatTx) coverOf(ctx context.Context, v combatViewer, terrain grid.Terrain, known *grid.Terrain, attacker, target playdb.Combatant, cs []playdb.Combatant) (coverPair, error) {
	cp := coverPair{real: coverAgainst(terrain, attacker, target, hiddenOut(cs, v))}
	cp.shown = cp.real
	if c.sight == nil {
		return cp, nil
	}
	if !v.master {
		cp.shown = coverAgainst(planOn(v, terrain, known), attacker, target, coverPool(cs, v))
	}
	if cp.real.degree == grid.CoverNone || cp.real.source != playv1.CoverSource_COVER_SOURCE_MAP {
		return cp, nil
	}
	cp.restricted = true
	for _, u := range c.sight.sight.Users() {
		kt, err := c.sight.knownTerrain(ctx, c.tx, combatViewer{userID: u})
		if err != nil {
			return cp, err
		}
		unseen := c.sight.unseenFor(u, cs)
		pool := slices.DeleteFunc(slices.Clone(cs), func(o playdb.Combatant) bool { return o.Hidden || unseen[o.ID] })
		if got := coverAgainst(planOn(combatViewer{userID: u}, terrain, kt), attacker, target, pool); got.degree == cp.real.degree && got.source == cp.real.source {
			cp.seenBy = append(cp.seenBy, u)
		}
	}
	return cp, nil
}

// coverFor is the cover an event tells the viewer: the master and a map without the
// fog get the real one; a player gets a restricted cover only if they are in
// seenBy, and none otherwise.
func coverFor(v combatViewer, key, source string, restricted bool, seenBy []string) (string, string) {
	if v.master || !restricted || slices.Contains(seenBy, v.userID) {
		return key, source
	}
	return "", ""
}

func (ev actionEvent) coverFor(v combatViewer) (string, string) {
	return coverFor(v, ev.Cover, ev.CoverSource, ev.CoverRestricted, ev.CoverSeenBy)
}

// coverFor is the cover the cast told the viewer for this target; users is the cast's
// CoverUsers.
func (h castHit) coverFor(v combatViewer, users []string) (string, string) {
	seenBy := h.CoverSeenBy
	for i, u := range users {
		if h.CoverSeenMask&(1<<i) != 0 {
			seenBy = append(slices.Clone(seenBy), u)
		}
	}
	return coverFor(v, h.Cover, h.CoverSource, h.CoverRestricted, seenBy)
}

// packCoverSeen replaces, in a cast's hits, the list of users who may read the cover of
// each target by a bit of the event's own list of users: the same few ids would
// otherwise be written again for every target, and ten targets of a table of players
// pass what one event can hold. A hit stays with its list when the table is too big for
// the bits.
func packCoverSeen(ev *actionEvent) {
	for i := range ev.Hits {
		h := &ev.Hits[i]
		var mask uint64
		for _, u := range h.CoverSeenBy {
			j := slices.Index(ev.CoverUsers, u)
			if j < 0 {
				if len(ev.CoverUsers) >= maxCoverUsers {
					mask = 0
					break
				}
				ev.CoverUsers = append(ev.CoverUsers, u)
				j = len(ev.CoverUsers) - 1
			}
			mask |= 1 << j
		}
		if mask != 0 || len(h.CoverSeenBy) == 0 {
			h.CoverSeenMask, h.CoverSeenBy = mask, nil
		}
	}
}

// maxCoverUsers is how many players a cast's hits can name with the bits of a mask.
const maxCoverUsers = 64

// viewerAfter is the viewer to build a change's answer with. A replay (the same
// idempotency key) never ran the change's closure, so its viewer knows nothing of the
// fog; this one is read again from the combat as it stands.
func (s *Service) viewerAfter(ctx context.Context, m authz.Membership, res combatResult, v combatViewer) (combatViewer, error) {
	if !res.repeated || v.master || res.encounterID == "" {
		return v, nil
	}
	enc, err := s.queries.GetEncounterInSession(ctx, playdb.GetEncounterInSessionParams{GameSessionID: res.session.ID, ID: res.encounterID})
	if err != nil {
		return v, fmt.Errorf("find the encounter: %w", err)
	}
	cs, err := s.queries.ListCombatants(ctx, enc.ID)
	if err != nil {
		return v, fmt.Errorf("list the combatants: %w", err)
	}
	return s.viewerFor(ctx, m, enc, cs)
}
