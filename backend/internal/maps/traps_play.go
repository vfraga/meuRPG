package maps

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/maps/link"
	"github.com/PuraFome/meuRPG/backend/internal/maps/mapsdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
	"github.com/PuraFome/meuRPG/backend/internal/rules/vision"
)

// Traps in play (MR-035): who notices a trap by passing
// near it, who finds it by searching, and the state a firing or a disarm leaves
// it in. The play module runs the game around it (the moves, the dice, the
// damage) and asks this one, through play.TrapBook, for everything that is a
// trap's or a map's: where the area is, what a character sees of it, who knows
// it.
//
// RN-10 is the rule that matters: a trap no character of a player knows (no row
// in map_point_reveals, not revealed to all, not fired) is never sent to that
// player. Noticing and searching are decided here, on the server, from what the
// character sees, and each one reveals the trap to that character alone. The
// answers say nothing a player could use to tell a trap from no trap.

// The session events this file writes (the others are in traps.go and play's).
const (
	eventTrapNoticed  = "trap_noticed"
	eventTrapDisarmed = "trap_disarmed"
)

// noticeRangeDFt is how near a trap's area a character must stand to notice it
// by passing: 3 m, which is 10 ft, in tenths of a foot (our radius: the SRD says
// only "in passing"). It is the distance between the squares' centers.
const noticeRangeDFt = 100

// noticeKey is one trap that one player's character noticed.
type noticeKey struct{ point, user string }

type trapNoticedEvent struct {
	PointID      string   `json:"point_id"`
	CharacterIDs []string `json:"character_ids"`
}

type trapDisarmedEvent struct {
	PointID string `json:"point_id"`
}

// trapScene is a map as the traps in play read it: the grid, the traps, who
// stands where and what each one sees.
type trapScene struct {
	row     mapsdb.Map
	g       grid.Grid
	points  []mapsdb.MapPoint // the traps, oldest first
	specs   map[string]*mapsv1.TrapSpec
	reveals map[string][]string // trap point ID -> the characters that know it
	members map[string]link.PartyMember
	order   []string // the party's character IDs, in the characters module's order
	sg      *sight   // nil when the map has no fog: everything is seen in bright light
}

// loadTrapScene reads what noticing needs. A map with no grid has no traps in
// play: a trap's area is squares (g is invalid and points empty).
func (s *Service) loadTrapScene(ctx context.Context, tx pgx.Tx, campaignID, mapID string, withEyes bool) (*trapScene, error) {
	q := queriesIn(s.queries, tx) // inside the caller's transaction: a change that commits meanwhile makes it retry, and no second connection is taken
	row, err := s.campaignMap(ctx, q, campaignID, mapID)
	if err != nil {
		return nil, err
	}
	size, err := q.GetMapGrid(ctx, mapsdb.GetMapGridParams{CampaignID: campaignID, ID: mapID})
	if err != nil {
		return nil, fmt.Errorf("read the map's grid: %w", err)
	}
	ts := &trapScene{
		row: row, g: gridOf(size.GridColumns, size.GridFactor, size.ImageWidth, size.ImageHeight),
		specs: map[string]*mapsv1.TrapSpec{}, reveals: map[string][]string{}, members: map[string]link.PartyMember{},
	}
	if !ts.g.Valid() {
		return ts, nil
	}
	all, err := s.loadTraps(ctx, q, ts, mapID)
	if err != nil {
		return nil, err
	}
	if !withEyes {
		return ts, nil // who sees what is not asked: the party's sheets are not derived
	}
	if err := s.loadEyes(ctx, tx, q, ts, campaignID, mapID, size, all); err != nil {
		return nil, err
	}
	return ts, nil
}

// loadTraps reads the map's points into the scene: the traps with their specs, and who
// knows each one. It returns every point of the map, for the fog.
func (s *Service) loadTraps(ctx context.Context, q *mapsdb.Queries, ts *trapScene, mapID string) ([]mapsdb.MapPoint, error) {
	all, err := q.ListMapPoints(ctx, mapID)
	if err != nil {
		return nil, fmt.Errorf("list the points: %w", err)
	}
	trap := kindToDB[mapsv1.MapPointKind_MAP_POINT_KIND_TRAP]
	for _, p := range all {
		if p.Kind == trap {
			ts.points = append(ts.points, p)
			ts.specs[p.ID] = s.trapOf(p)
		}
	}
	revealed, err := q.ListPointRevealCharacters(ctx, mapID)
	if err != nil {
		return nil, fmt.Errorf("list who knows the traps: %w", err)
	}
	for _, r := range revealed {
		ts.reveals[r.PointID] = append(ts.reveals[r.PointID], r.CharacterID)
	}
	return all, nil
}

// loadEyes reads the living party into the scene and, on a map with fog, what it sees.
func (s *Service) loadEyes(ctx context.Context, tx pgx.Tx, q *mapsdb.Queries, ts *trapScene, campaignID, mapID string, size mapsdb.GetMapGridRow, all []mapsdb.MapPoint) error {
	party, err := partyOf(ctx, tx, s, campaignID)
	if err != nil {
		return fmt.Errorf("read the party: %w", err)
	}
	for _, m := range party {
		if m.Dead {
			continue // a dead character notices nothing
		}
		ts.members[m.CharacterID] = m
		ts.order = append(ts.order, m.CharacterID)
	}
	if !fogged(ts.row) {
		return nil
	}
	tokens, err := q.ListMapTokens(ctx, mapID)
	if err != nil {
		return fmt.Errorf("list the tokens: %w", err)
	}
	ts.sg, err = s.newSight(ctx, tx, fogInputOfRow(ts.row, size.ImageWidth, size.ImageHeight), all, tokens)
	return err
}

// stateAt is how the viewer sees a square: with the fog on, from the compiled
// light and walls; without it, every square of the grid in bright light.
func (ts *trapScene) stateAt(v vision.Viewer, sq grid.Square) vision.State {
	if ts.sg != nil {
		return ts.sg.entry.see(v).At(sq)
	}
	if ts.g.Contains(sq) {
		return vision.SeenBright
	}
	return vision.Unseen
}

// seesSquare says a state is a square seen now (a wall seen by its neighbor is not).
func seesSquare(st vision.State) bool { return st >= vision.SeenGrey && st <= vision.SeenBright }

// sighting is what an observer makes of a trap's area.
type sighting struct {
	inRange bool // some square of the area is within 3 m
	sees    bool // some square of the area is seen now
	// eligible says a square of the area is both: the observer can notice or find it.
	eligible bool
	// penalty is the passive-check penalty at the best such square: 0 or -5.
	penalty int
	// seenPenalty is the same at the best square seen at all, near or not: what the
	// master's card shows for a character that is out of range.
	seenPenalty int
	// obscured says the best eligible square is lightly obscured to the observer: an
	// active Perception check on it has disadvantage (SRD).
	obscured bool
}

// penaltyAt is the passive-check penalty at a square the viewer sees: dim light, or
// darkness seen through darkvision, takes 5 (vision.PassivePenalty); a square within
// the viewer's blindsight is not lightly obscured to it.
func (ts *trapScene) penaltyAt(v vision.Viewer, sq grid.Square) int {
	if bs := v.Senses.BlindsightFt; bs > 0 && grid.LengthDFt(v.At, sq) <= bs*10 {
		return 0
	}
	return vision.PassivePenalty(ts.stateAt(v, sq))
}

// nearbyObscured says some square within 3 m that the viewer sees is lightly obscured
// to it. It looks at squares, never at traps, so it tells nothing about them: it is
// what decides whether a Perception search on a real die needs a second one.
func (ts *trapScene) nearbyObscured(v vision.Viewer) bool {
	for row := v.At.Row - 2; row <= v.At.Row+2; row++ {
		for col := v.At.Col - 2; col <= v.At.Col+2; col++ {
			sq := grid.Square{Col: col, Row: row}
			if ts.g.Contains(sq) && grid.LengthDFt(v.At, sq) <= noticeRangeDFt && seesSquare(ts.stateAt(v, sq)) && ts.penaltyAt(v, sq) < 0 {
				return true
			}
		}
	}
	return false
}

// sightOf is what the viewer makes of the squares of an area.
func (ts *trapScene) sightOf(v vision.Viewer, area []grid.Square) sighting {
	var out sighting
	for _, sq := range area {
		out.addSquare(ts, v, sq)
	}
	return out
}

// addSquare adds what the viewer makes of one square of an area: whether it is near, seen,
// and the best light penalty among the squares seen (and among the ones seen and near).
func (out *sighting) addSquare(ts *trapScene, v vision.Viewer, sq grid.Square) {
	near := grid.LengthDFt(v.At, sq) <= noticeRangeDFt
	seen := seesSquare(ts.stateAt(v, sq))
	out.inRange = out.inRange || near
	if !seen {
		return
	}
	pen := ts.penaltyAt(v, sq)
	if !out.sees || pen > out.seenPenalty {
		out.seenPenalty = pen
	}
	out.sees = true
	if !near {
		return
	}
	if !out.eligible || pen > out.penalty {
		out.penalty, out.obscured = pen, pen < 0
	}
	out.eligible = true
}

// eyesOf is what an observer notices with, and the user who plays the
// character that learns of it (ok is false for a character with no player).
func (ts *trapScene) eyesOf(ob link.Observer) (eyes link.Eyes, userID string, ok bool) {
	m, ok := ts.members[ob.CharacterID]
	if !ok {
		return link.Eyes{}, "", false
	}
	if ob.Creature != nil {
		return *ob.Creature, m.UserID, true
	}
	return link.Eyes{Passive: m.PassivePerception, Senses: m.Senses}, m.UserID, true
}

// knows says the character knows the trap: revealed to it, or to everyone.
func (ts *trapScene) knows(p mapsdb.MapPoint, characterID string) bool {
	return everyoneSees(p) || slices.Contains(ts.reveals[p.ID], characterID)
}

// armed says the trap can still fire.
func armed(p mapsdb.MapPoint) bool { return p.TrapState != nil && *p.TrapState == "armed" }

// Notice is the passive notice after a move (MR-035, D5): each observer that
// ends a move within 3 m of an armed trap's area, sees a square of it, and has a
// passive Perception, less the light penalty there, that reaches the trap's DC to
// notice it, learns of the trap (map_point_reveals, how "noticed"), and only its
// player hears of it (`map_changed`, per user). A trap with no DC to notice is
// never noticed this way. The play module calls it after a combat move commits
// and PlaceMapToken calls it after a token lands. It implements play.TrapBook;
// it takes no caller, and failing leaves the move made.
func (s *Service) Notice(ctx context.Context, campaignID, mapID string, who []link.Observer) error {
	if len(who) == 0 {
		return nil
	}
	ts, err := s.loadTrapScene(ctx, nil, campaignID, mapID, true)
	if err != nil || len(ts.points) == 0 {
		return err
	}
	finds := s.noticeFinds(ts, who)
	if len(finds) == 0 {
		return nil
	}
	told, noticed, err := s.recordNotices(ctx, campaignID, finds)
	if err != nil {
		return fmt.Errorf("notice a trap: %w", err)
	}
	s.tellUsers(ctx, campaignID, mapID, ts.row, told)
	s.tellNoticers(ctx, campaignID, mapID, ts.row, noticed)
	// The master's trap card and log have news: a hint with no content.
	s.publish(campaignID, false, mapChangedEvent(mapID))
	return nil
}

// noticeFind is one trap that one character, played by user, notices.
type noticeFind struct{ point, character, user string }

// noticeFinds works out, from what each observer sees, the traps each one notices: in
// the order of the observers and then of the traps, once for each pair.
func (s *Service) noticeFinds(ts *trapScene, who []link.Observer) []noticeFind {
	var finds []noticeFind
	for _, ob := range who {
		eyes, user, ok := ts.eyesOf(ob)
		if !ok {
			continue
		}
		viewer := vision.Viewer{At: ob.At, Senses: eyes.Senses}
		for _, p := range ts.points {
			if slices.ContainsFunc(finds, func(f noticeFind) bool { return f.point == p.ID && f.character == ob.CharacterID }) {
				continue
			}
			if s.noticesTrap(ts, p, ob.CharacterID, eyes, viewer) {
				finds = append(finds, noticeFind{point: p.ID, character: ob.CharacterID, user: user})
			}
		}
	}
	return finds
}

// noticesTrap says the character notices the trap by passing: the trap is armed, has a DC
// to notice, is not known to the character yet, and the character sees a square of its
// area within 3 m with a passive Perception, less the light there, that reaches the DC.
func (s *Service) noticesTrap(ts *trapScene, p mapsdb.MapPoint, characterID string, eyes link.Eyes, viewer vision.Viewer) bool {
	spec := ts.specs[p.ID]
	if !armed(p) || spec == nil || spec.GetNoticeDc() == 0 || ts.knows(p, characterID) {
		return false
	}
	sg := ts.sightOf(viewer, s.pointSquares(ts.g, p))
	return sg.eligible && eyes.Passive+sg.penalty >= int(spec.GetNoticeDc())
}

// recordNotices writes the reveals of the finds in one transaction, and one event for each
// trap and player (the actor is the one whose character noticed). It returns the players
// told, and the (trap, player) pairs that are new, in order.
func (s *Service) recordNotices(ctx context.Context, campaignID string, finds []noticeFind) ([]string, []noticeKey, error) {
	var told []string
	var noticed []noticeKey
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		told, noticed = nil, nil
		now := s.now()
		var order []noticeKey
		var byKey map[noticeKey][]string
		var err error
		if told, order, byKey, err = s.insertNotices(ctx, s.queries.WithTx(tx), finds, now); err != nil {
			return err
		}
		noticed, err = s.appendNotices(ctx, tx, campaignID, order, byKey, now)
		return err
	})
	return told, noticed, err
}

// insertNotices writes a "noticed" reveal for each find, and groups the new ones by trap
// and player. It returns the players told, the keys in order and the characters of each.
func (s *Service) insertNotices(ctx context.Context, q *mapsdb.Queries, finds []noticeFind, now time.Time) ([]string, []noticeKey, map[noticeKey][]string, error) {
	var told []string
	var order []noticeKey
	byKey := map[noticeKey][]string{}
	for _, f := range finds {
		n, err := q.InsertPointReveal(ctx, mapsdb.InsertPointRevealParams{PointID: f.point, CharacterID: f.character, How: "noticed", At: now})
		if err != nil {
			return nil, nil, nil, fmt.Errorf("reveal the trap: %w", err)
		}
		if n == 0 {
			continue
		}
		k := noticeKey{f.point, f.user}
		if _, seen := byKey[k]; !seen {
			order = append(order, k)
		}
		byKey[k] = append(byKey[k], f.character)
		if !slices.Contains(told, f.user) {
			told = append(told, f.user)
		}
	}
	return told, order, byKey, nil
}

// appendNotices writes the session event of each (trap, player) pair, in order.
func (s *Service) appendNotices(ctx context.Context, tx pgx.Tx, campaignID string, order []noticeKey, byKey map[noticeKey][]string, now time.Time) ([]noticeKey, error) {
	var noticed []noticeKey
	for _, k := range order {
		payload, err := json.Marshal(trapNoticedEvent{PointID: k.point, CharacterIDs: byKey[k]})
		if err != nil {
			return nil, fmt.Errorf("encode the event payload: %w", err)
		}
		if _, err := s.live.AppendEvent(ctx, tx, campaignID, eventTrapNoticed, k.user, payload, now); err != nil {
			return nil, fmt.Errorf("record the notice: %w", err)
		}
		noticed = append(noticed, k)
	}
	return noticed, nil
}

// tellUsers tells those players that the map changed (a trap they now know), when
// the map is one the players see. Nobody else hears of it.
func (s *Service) tellUsers(ctx context.Context, campaignID, mapID string, row mapsdb.Map, users []string) {
	if len(users) == 0 {
		return
	}
	current, err := s.currentMap(ctx, campaignID)
	if err != nil {
		s.logger.ErrorContext(ctx, "maps: cannot tell the players about a trap", "error", err)
		return
	}
	if playersSee(mapID, row.RevealedAt, current) {
		s.live.PublishToUsers(campaignID, users, mapChangedEvent(mapID))
	}
}

// tellNoticers sends each player who just noticed a trap the `trap_noticed` hint
// ("Você notou uma armadilha."), under the same rule as tellUsers: only when
// the map is one the players see, and only to that player (RN-10). The hint names the
// map and the trap, which that player now knows; nobody else gets it.
func (s *Service) tellNoticers(ctx context.Context, campaignID, mapID string, row mapsdb.Map, noticed []noticeKey) {
	if len(noticed) == 0 {
		return
	}
	current, err := s.currentMap(ctx, campaignID)
	if err != nil {
		s.logger.ErrorContext(ctx, "maps: cannot tell the players about a trap", "error", err)
		return
	}
	if !playersSee(mapID, row.RevealedAt, current) {
		return
	}
	for _, n := range noticed {
		s.live.PublishToUsers(campaignID, []string{n.user}, &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_TrapNoticed_{
			TrapNoticed: &playv1.WatchGameSessionResponse_TrapNoticed{MapId: mapID, PointId: n.point},
		}})
	}
}

// SearchTraps is "Procurar armadilhas" (MR-035) as the maps
// module sees it: the observer's roll total, with the skill ("perception",
// "investigation" or the key of another skill, "skill:arcana"), against every armed
// trap within 3 m whose square it sees and that its character does not know.
// Perception meets the trap's DC to notice it (a trap with none is never found by
// it), Investigation its DC to find it, and another skill the DC to find, only for
// the traps whose master listed it (TrapSpec.also_find_skill_keys): for the others it
// sees nothing, which reads exactly as a search that fell short. A
// pass reveals the trap to the observer's character, inside tx, and nothing else
// is said: who found what is the play module's event. It implements play.TrapBook.
func (s *Service) SearchTraps(ctx context.Context, tx pgx.Tx, campaignID, mapID string, who link.Observer, skill string, totals []int, at time.Time) (link.SearchResult, error) {
	ts, err := s.loadTrapScene(ctx, tx, campaignID, mapID, true)
	if err != nil {
		return link.SearchResult{}, err
	}
	// No return for a map without traps: the refusal below depends on the light around the
	// searcher alone, so it comes the same on a map that has traps and on one that has none.
	eyes, user, ok := ts.eyesOf(who)
	if !ok {
		return link.SearchResult{}, nil
	}
	viewer := vision.Viewer{At: who.At, Senses: eyes.Senses}
	// A Perception search has disadvantage on what is lightly obscured to the searcher
	// (SRD): two rolls, the lower counts there and the first elsewhere. With a real die
	// the second one is asked for whenever some square nearby is lightly obscured.
	if skill == "perception" && len(totals) < 2 && ts.nearbyObscured(viewer) {
		return link.SearchResult{}, link.ErrSearchNeedsTwoDice
	}
	q := s.queries.WithTx(tx)
	search := trapSearch{who: who, viewer: viewer, skill: skill, totals: totals, at: at}
	var out link.SearchResult
	for _, p := range ts.points {
		found, err := s.searchTrap(ctx, q, ts, p, search)
		if err != nil {
			return link.SearchResult{}, err
		}
		if found {
			out.Found = append(out.Found, p.ID)
		}
	}
	if len(out.Found) > 0 {
		out.Users = []string{user}
	}
	return out, nil
}

// trapSearch is one character's search: who, from where, with which skill and rolls.
type trapSearch struct {
	who    link.Observer
	viewer vision.Viewer
	skill  string
	totals []int
	at     time.Time
}

// searchDC is the DC the skill meets on the trap, and whether the skill searches this trap at
// all: Perception meets the DC to notice, Investigation the DC to find, and another skill
// the DC to find, only on the traps whose master listed it.
func searchDC(skill string, spec *mapsv1.TrapSpec) (int, bool) {
	switch {
	case skill == "perception":
		return int(spec.GetNoticeDc()), true
	case skill != "investigation" && !slices.Contains(spec.GetAlsoFindSkillKeys(), skill):
		return 0, false // another skill finds only the traps whose master allowed it, against the DC to find
	}
	return int(spec.GetFindDc()), true
}

// searchTrap tries the search on one trap, and says whether it revealed the trap to the
// searcher's character just now: it must be armed, unknown to the character, within 3 m and
// seen, and the total must reach the DC.
func (s *Service) searchTrap(ctx context.Context, q *mapsdb.Queries, ts *trapScene, p mapsdb.MapPoint, c trapSearch) (bool, error) {
	spec := ts.specs[p.ID]
	if !armed(p) || spec == nil || ts.knows(p, c.who.CharacterID) {
		return false, nil
	}
	dc, ok := searchDC(c.skill, spec)
	if !ok {
		return false, nil
	}
	sg := ts.sightOf(c.viewer, s.pointSquares(ts.g, p))
	total := c.totals[0]
	if c.skill == "perception" && sg.obscured && len(c.totals) > 1 {
		total = min(c.totals[0], c.totals[1])
	}
	if dc == 0 || total < dc || !sg.eligible {
		return false, nil
	}
	n, err := q.InsertPointReveal(ctx, mapsdb.InsertPointRevealParams{PointID: p.ID, CharacterID: c.who.CharacterID, How: "searched", At: c.at})
	if err != nil {
		return false, fmt.Errorf("reveal the trap: %w", err)
	}
	return n == 1, nil
}

// Told tells the players that the map changed, after a commit that revealed a
// trap to their characters. It implements play.TrapBook.
func (s *Service) Told(ctx context.Context, campaignID, mapID string, users []string) {
	row, err := s.queries.GetMap(ctx, mapsdb.GetMapParams{CampaignID: campaignID, ID: mapID})
	if err != nil {
		s.logger.ErrorContext(ctx, "maps: cannot tell the players about a trap", "error", err)
		return
	}
	s.tellUsers(ctx, campaignID, mapID, row, users)
}

// Traps returns the map's traps with the master's data, for the play module to
// fire, search and warn with. It never goes to a player as it is. A map with no
// grid has traps with no squares. It implements play.TrapBook.
func (s *Service) Traps(ctx context.Context, tx pgx.Tx, campaignID, mapID string) ([]link.Trap, error) {
	ts, err := s.loadTrapScene(ctx, tx, campaignID, mapID, false)
	if err != nil {
		return nil, err
	}
	out := make([]link.Trap, 0, len(ts.points))
	for _, p := range ts.points {
		out = append(out, s.linkTrap(ts.g, p, ts.reveals[p.ID], true))
	}
	return out, nil
}

// KnownTraps returns the armed traps the character knows (revealed to it, to
// everyone, or fired and re-armed), with their squares and names and without
// their data, for the warning before a move into one. It implements
// play.TrapBook.
func (s *Service) KnownTraps(ctx context.Context, campaignID, mapID, characterID string) ([]link.Trap, error) {
	ts, err := s.loadTrapScene(ctx, nil, campaignID, mapID, false)
	if err != nil {
		return nil, err
	}
	var out []link.Trap
	for _, p := range ts.points {
		if armed(p) && ts.knows(p, characterID) {
			out = append(out, s.linkTrap(ts.g, p, nil, false))
		}
	}
	return out, nil
}

// linkTrap is a trap point as the play module reads it. withSpec says to include
// the master's data (the DCs, the effect).
func (s *Service) linkTrap(g grid.Grid, p mapsdb.MapPoint, knownBy []string, withSpec bool) link.Trap {
	t := link.Trap{PointID: p.ID, MapID: p.MapID, Name: p.Name, KnownBy: knownBy, Public: everyoneSees(p)}
	if p.TrapState != nil {
		t.State = *p.TrapState
	}
	t.TriggeredAt = p.TrapTriggeredAt
	if spec := s.trapOf(p); spec != nil {
		t.OnEnter = spec.GetTrigger() == rulesv1.TrapTrigger_TRAP_TRIGGER_ENTER
		if withSpec {
			t.Spec = spec
		}
	}
	if g.Valid() {
		t.Squares = s.pointSquares(g, p)
	}
	return t
}

// TriggerTrap marks a trap triggered inside tx and returns it as it was, with
// its data. The play module calls it with the session locked. It is
// link.ErrTrapNotArmed when the trap fired or was disarmed meanwhile. It
// implements play.TrapBook.
func (s *Service) TriggerTrap(ctx context.Context, tx pgx.Tx, campaignID, mapID, pointID string, at time.Time) (link.Trap, error) {
	q := s.queries.WithTx(tx)
	_, p, err := s.lockPoint(ctx, q, campaignID, mapID, pointID, mapsv1.MapPointKind_MAP_POINT_KIND_TRAP)
	if err != nil {
		return link.Trap{}, err
	}
	if !armed(p) {
		return link.Trap{}, link.ErrTrapNotArmed
	}
	// A failed read aborts the transaction: it is returned, so a retryable conflict
	// (40001) is retried, never hidden behind 25P02 or a trap with no squares.
	size, err := q.GetMapGrid(ctx, mapsdb.GetMapGridParams{CampaignID: campaignID, ID: mapID})
	if err != nil {
		return link.Trap{}, fmt.Errorf("read the trap's map grid: %w", err)
	}
	before := s.linkTrap(gridOf(size.GridColumns, size.GridFactor, size.ImageWidth, size.ImageHeight), p, nil, true)
	state := stateTriggered
	if _, err := q.SetTrapState(ctx, mapsdb.SetTrapStateParams{MapID: mapID, ID: pointID, TrapState: &state, TrapTriggeredAt: &at, Now: at}); err != nil {
		return link.Trap{}, fmt.Errorf("trigger the trap: %w", err)
	}
	return before, nil
}

// RestoreTrap puts a trap back to the state and the firing time it had, inside
// tx: the undo of a firing. A trap that was deleted meanwhile is skipped. It
// implements play.TrapBook.
func (s *Service) RestoreTrap(ctx context.Context, tx pgx.Tx, campaignID, mapID, pointID, state string, triggeredAt *time.Time, at time.Time) error {
	q := s.queries.WithTx(tx)
	if _, _, err := s.lockPoint(ctx, q, campaignID, mapID, pointID, mapsv1.MapPointKind_MAP_POINT_KIND_TRAP); err != nil {
		if connect.CodeOf(err) == connect.CodeNotFound {
			return nil
		}
		return err
	}
	if _, err := q.SetTrapState(ctx, mapsdb.SetTrapStateParams{MapID: mapID, ID: pointID, TrapState: &state, TrapTriggeredAt: triggeredAt, Now: at}); err != nil {
		return fmt.Errorf("put the trap back: %w", err)
	}
	return nil
}

// gridOfMap is the map's grid, or the zero grid when the read fails: used only
// after a commit, on the pool, to decorate a trap with its squares. Inside a
// transaction a failed read is returned instead (see TriggerTrap).
func (s *Service) gridOfMap(ctx context.Context, q *mapsdb.Queries, campaignID, mapID string) grid.Grid {
	size, err := q.GetMapGrid(ctx, mapsdb.GetMapGridParams{CampaignID: campaignID, ID: mapID})
	if err != nil {
		return grid.Grid{}
	}
	return gridOf(size.GridColumns, size.GridFactor, size.ImageWidth, size.ImageHeight)
}

// TrapChanged tells the watching members that a trap's state changed after the
// commit (it fired, or an undo took the firing back): a trap that is public now
// reaches everyone who sees the map, as every point change does (fog included).
// It implements play.TrapBook.
func (s *Service) TrapChanged(ctx context.Context, campaignID, mapID, pointID string) {
	row, err := s.queries.GetMap(ctx, mapsdb.GetMapParams{CampaignID: campaignID, ID: mapID})
	if err != nil {
		s.logger.ErrorContext(ctx, "maps: cannot tell the players about a trap", "error", err)
		return
	}
	p, err := s.queries.GetMapPoint(ctx, mapsdb.GetMapPointParams{MapID: mapID, ID: pointID})
	if err != nil {
		return // deleted meanwhile: its deletion told everyone who needed to know
	}
	current, err := s.currentMap(ctx, campaignID)
	if err != nil {
		s.logger.ErrorContext(ctx, "maps: cannot tell the players about a trap", "error", err)
		return
	}
	s.publishPointsChanged(ctx, campaignID, row, playersSee(mapID, row.RevealedAt, current), p)
}

// TrapNames returns the names of those of the points that are traps of the
// campaign, by point ID. The play module asks for the traps that fired (the combat
// log): a trap that fired is public. It implements play.TrapBook.
func (s *Service) TrapNames(ctx context.Context, campaignID string, pointIDs []string) (map[string]string, error) {
	out := map[string]string{}
	if len(pointIDs) == 0 {
		return out, nil
	}
	rows, err := s.queries.ListTrapNamesInCampaign(ctx, mapsdb.ListTrapNamesInCampaignParams{CampaignID: campaignID, Column2: pointIDs})
	if err != nil {
		return nil, fmt.Errorf("read the traps' names: %w", err)
	}
	for _, r := range rows {
		out[r.ID] = r.Name
	}
	return out, nil
}

// GetTrapNoticers implements mapsv1connect.MapServiceHandler.
func (s *Service) GetTrapNoticers(
	ctx context.Context,
	req *connect.Request[mapsv1.GetTrapNoticersRequest],
) (*connect.Response[mapsv1.GetTrapNoticersResponse], error) {
	m, mapID, pointID, err := s.pointCall(ctx, req.Msg.GetCampaignId(), req.Msg.GetMapId(), req.Msg.GetPointId())
	if err != nil {
		return nil, err
	}
	ts, err := s.loadTrapScene(ctx, nil, m.CampaignID, mapID, true)
	if err != nil {
		return nil, s.dbError(ctx, "read the map's traps", err)
	}
	i := slices.IndexFunc(ts.points, func(p mapsdb.MapPoint) bool { return p.ID == pointID })
	if i < 0 {
		return s.noNoticersOf(ctx, mapID, pointID)
	}
	p := ts.points[i]
	spec := ts.specs[pointID]
	area := s.pointSquares(ts.g, p)
	// The party in the order the characters module lists it.
	names, order, err := s.characterNames(ctx, m.CampaignID, ts.order)
	if err != nil {
		return nil, s.dbError(ctx, "read the characters", err)
	}
	stands, err := s.standsOf(ctx, ts, m.CampaignID, mapID)
	if err != nil {
		return nil, s.dbError(ctx, "read where the characters stand", err)
	}
	out := &mapsv1.GetTrapNoticersResponse{NoticeDc: spec.GetNoticeDc()}
	for _, id := range order {
		member, ok := ts.members[id]
		if !ok {
			continue
		}
		at, on := stands[id]
		out.Noticers = append(out.Noticers, noticerOf(ts, p, spec, area, member, names[id], at, on))
	}
	return connect.NewResponse(out), nil
}

// noNoticersOf answers for a point that is not among the traps of a map with a grid: a
// point of another kind is the caller's mistake, a point that is not there is not found,
// and a trap of a map with no grid has no squares to notice from.
func (s *Service) noNoticersOf(ctx context.Context, mapID, pointID string) (*connect.Response[mapsv1.GetTrapNoticersResponse], error) {
	p, err := s.queries.GetMapPoint(ctx, mapsdb.GetMapPointParams{MapID: mapID, ID: pointID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errPointNotFound()
	}
	if err != nil {
		return nil, s.dbError(ctx, "find the point", err)
	}
	if p.Kind != kindToDB[mapsv1.MapPointKind_MAP_POINT_KIND_TRAP] {
		return nil, badSpec("the point is not a TRAP point")
	}
	return connect.NewResponse(&mapsv1.GetTrapNoticersResponse{}), nil // a map with no grid: no squares to notice from
}

// noticerOf is one character of the party as the master's "Quem notaria" shows it for a
// trap: what it would notice from where it stands (at, when on the map).
func noticerOf(ts *trapScene, p mapsdb.MapPoint, spec *mapsv1.TrapSpec, area []grid.Square, member link.PartyMember, name string, at grid.Square, on bool) *mapsv1.TrapNoticer {
	n := &mapsv1.TrapNoticer{CharacterId: member.CharacterID, CharacterName: name, PassivePerception: int32(member.PassivePerception), Knows: ts.knows(p, member.CharacterID)} //nolint:gosec // G115: a score
	if on {
		n.OnMap = true
		sg := ts.sightOf(vision.Viewer{At: at, Senses: member.Senses}, area)
		n.InRange, n.Sees = sg.inRange, sg.sees
		n.LightPenalty = int32(sg.seenPenalty) //nolint:gosec // G115: 0 or -5
		if sg.eligible {
			n.LightPenalty = int32(sg.penalty) //nolint:gosec // G115: 0 or -5
			n.WouldNotice = spec.GetNoticeDc() > 0 && member.PassivePerception+sg.penalty >= int(spec.GetNoticeDc())
		}
	}
	// What the score would do at the DC with the penalty reported here, wherever the character
	// stands and whether or not it sees the squares: the screen shows it for the ones out of range.
	n.PassesDc = spec.GetNoticeDc() > 0 && member.PassivePerception+int(n.LightPenalty) >= int(spec.GetNoticeDc())
	return n
}

// standsOf is where each player character stands on the map: its combatant's
// square while a combat runs on it, else its token's.
func (s *Service) standsOf(ctx context.Context, ts *trapScene, campaignID, mapID string) (map[string]grid.Square, error) {
	tokens, err := s.queries.ListMapTokens(ctx, mapID)
	if err != nil {
		return nil, fmt.Errorf("list the tokens: %w", err)
	}
	combat, err := s.combats.CombatPositions(ctx, nil, campaignID, mapID)
	if err != nil {
		return nil, fmt.Errorf("read the combatants' squares: %w", err)
	}
	out := map[string]grid.Square{}
	for _, t := range tokens {
		out[t.CharacterID] = ts.g.SquareOf(int(t.XBp), int(t.YBp))
	}
	maps.Copy(out, combat.Positions)
	return out, nil
}

// DisarmTrap implements mapsv1connect.MapServiceHandler.
func (s *Service) DisarmTrap(
	ctx context.Context,
	req *connect.Request[mapsv1.DisarmTrapRequest],
) (*connect.Response[mapsv1.DisarmTrapResponse], error) {
	m, mapID, pointID, err := s.pointCall(ctx, req.Msg.GetCampaignId(), req.Msg.GetMapId(), req.Msg.GetPointId())
	if err != nil {
		return nil, err
	}
	current, err := s.currentMap(ctx, m.CampaignID)
	if err != nil {
		return nil, err
	}
	var res trapDisarm
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		res, err = s.disarmTrapTx(ctx, tx, m, mapID, pointID)
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, "disarm a trap", err)
	}
	if res.changed {
		// Everyone who sees a public trap hears of it; one that is not public reaches
		// the players whose characters know it, and nobody else (RN-10).
		s.publishPointsChanged(ctx, m.CampaignID, res.mapRow, everyoneSees(res.after) && playersSee(mapID, res.mapRow.RevealedAt, current), res.after)
		s.tellTrapKnowers(m.CampaignID, mapID, res.mapRow, current, res.knowers)
	}
	out, err := s.masterPoint(ctx, m, res.after)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&mapsv1.DisarmTrapResponse{Point: out}), nil
}

// trapDisarm is what the DisarmTrap transaction ends with: the map and the trap as they are
// now, the players whose characters know the trap, and whether this call disarmed it.
type trapDisarm struct {
	mapRow  mapsdb.Map
	after   mapsdb.MapPoint
	knowers []string
	changed bool
}

// disarmTrapTx is the transaction of DisarmTrap: a trap already disarmed changes nothing;
// otherwise the players who know it are read before the change, it is disarmed (its
// firing time kept) and the disarm is recorded.
func (s *Service) disarmTrapTx(ctx context.Context, tx pgx.Tx, m authz.Membership, mapID, pointID string) (trapDisarm, error) {
	q := s.queries.WithTx(tx)
	mapRow, before, err := s.lockPoint(ctx, q, m.CampaignID, mapID, pointID, mapsv1.MapPointKind_MAP_POINT_KIND_TRAP)
	if err != nil {
		return trapDisarm{}, err
	}
	res := trapDisarm{mapRow: mapRow, after: before}
	if before.TrapState != nil && *before.TrapState == "disarmed" {
		return res, nil // already disarmed: nothing changes, nothing is recorded
	}
	if res.knowers, err = s.trapKnowers(ctx, tx, q, m.CampaignID, before); err != nil {
		return trapDisarm{}, err
	}
	now := s.now()
	disarmed := "disarmed"
	if res.after, err = q.SetTrapState(ctx, mapsdb.SetTrapStateParams{MapID: mapID, ID: pointID, TrapState: &disarmed, TrapTriggeredAt: before.TrapTriggeredAt, Now: now}); err != nil {
		return trapDisarm{}, fmt.Errorf("disarm the trap: %w", err)
	}
	payload, err := json.Marshal(trapDisarmedEvent{PointID: pointID})
	if err != nil {
		return trapDisarm{}, fmt.Errorf("encode the event payload: %w", err)
	}
	// With no open session nothing is written here: the trap is still disarmed.
	if _, err := s.live.AppendEvent(ctx, tx, m.CampaignID, eventTrapDisarmed, m.UserID, payload, now); err != nil {
		return trapDisarm{}, fmt.Errorf("record the disarm: %w", err)
	}
	res.changed = true
	return res, nil
}

// TrapFirer is what this package needs from the play module to fire a trap when
// the master drops a player's character's token inside its area (MR-035, D5):
// the firing is a game event, with dice, damage and a history, so it is play's.
// play.Service implements it and cmd/api connects it with SetTrapFirer, after
// both services exist. Nil means no trap fires from a token.
type TrapFirer interface {
	// TokenDropped fires the armed "Ao entrar na área" traps whose area holds the
	// square the character's token now stands on, during an open session when no
	// combat runs on the map. It takes no caller: it runs after this package's own
	// authorization (the master), and a failure is logged, never the master's.
	TokenDropped(ctx context.Context, campaignID, mapID, characterID, actorUserID string, at grid.Square) error
	// CreatureDropped is the same for a creature of a player's character: its token
	// was dropped on a square, and the traps whose area holds it fire (the creature is
	// caught with whoever stands there). Same terms as TokenDropped.
	CreatureDropped(ctx context.Context, campaignID, mapID string, creature link.MapCreature, actorUserID string, at grid.Square) error
}

// SetTrapFirer connects the module that fires traps.
func (s *Service) SetTrapFirer(f TrapFirer) { s.firer = f }

// tokenLanded is what a token dropped on a map does to traps, after the commit: a
// player's character may fire a trap in whose area it lands, and notices what it
// can from where it stands (NPCs do neither: they never fire a trap by walking,
// and have no player to tell). It never fails the move.
func (s *Service) tokenLanded(ctx context.Context, campaignID, actorUserID, mapID string, token mapsdb.MapToken, character *charactersv1.CharacterSummary) {
	if !isPlayerCharacter(character) {
		return
	}
	ctx = context.WithoutCancel(ctx)
	// Only on a map the players see (revealed, or the session's current one): a token
	// put right on a map the master is still preparing fires and notices nothing.
	row, err := s.queries.GetMap(ctx, mapsdb.GetMapParams{CampaignID: campaignID, ID: mapID})
	if err != nil {
		return
	}
	current, err := s.currentMap(ctx, campaignID)
	if err != nil || !playersSee(mapID, row.RevealedAt, current) {
		return
	}
	g := s.gridOfMap(ctx, s.queries, campaignID, mapID)
	if !g.Valid() {
		return
	}
	at := g.SquareOf(int(token.XBp), int(token.YBp))
	if s.firer != nil {
		if err := s.firer.TokenDropped(ctx, campaignID, mapID, character.GetId(), actorUserID, at); err != nil {
			s.logger.ErrorContext(ctx, "maps: cannot fire a trap for a token", "error", err)
		}
	}
	if err := s.Notice(ctx, campaignID, mapID, []link.Observer{{CharacterID: character.GetId(), At: at}}); err != nil {
		s.logger.ErrorContext(ctx, "maps: cannot notice a trap for a token", "error", err)
	}
}

// creatureLanded is tokenLanded for a creature's token: a creature of a player's
// character fires the traps whose area it lands in, on a map the players see, and it
// never fails the move. It notices nothing: noticing is a character's, and in a combat
// the creature's moves are the combat's (play.TrapBook.Notice is called there).
func (s *Service) creatureLanded(ctx context.Context, campaignID, actorUserID, mapID string, token mapsdb.MapCreatureToken, creature link.MapCreature) {
	if s.firer == nil {
		return
	}
	ctx = context.WithoutCancel(ctx)
	row, err := s.queries.GetMap(ctx, mapsdb.GetMapParams{CampaignID: campaignID, ID: mapID})
	if err != nil {
		return
	}
	current, err := s.currentMap(ctx, campaignID)
	if err != nil || !playersSee(mapID, row.RevealedAt, current) {
		return
	}
	g := s.gridOfMap(ctx, s.queries, campaignID, mapID)
	if !g.Valid() {
		return
	}
	if err := s.firer.CreatureDropped(ctx, campaignID, mapID, creature, actorUserID, g.SquareOf(int(token.XBp), int(token.YBp))); err != nil {
		s.logger.ErrorContext(ctx, "maps: cannot fire a trap for a creature's token", "error", err)
	}
}

// FallFt implements play.TrapBook: the depth of a pit preset, in feet.
func (s *Service) FallFt(presetKey string) int {
	if presetKey == "" {
		return 0
	}
	p, ok := s.rules.TrapPreset(presetKey)
	if !ok {
		return 0
	}
	return p.FallFt
}
