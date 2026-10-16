package maps

import (
	"slices"
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
)

// The master's calls on a trap or a treasure (RevealTrap, MarkTreasureFound and
// UnmarkTreasureFound), frozen before they were split into phases: which fault a caller
// reads first, who hears of each change and what is written. The happy paths and the
// leaks of RN-10 are in kinds_test.go and fixround_test.go.

// trapCallsTable is newScenes with a second map the players do not see, a trap and a
// treasure on each map, and the master and both players watching.
type trapCallsTable struct {
	*scenes
	hiddenMap                          string
	trap, chest, hiddenTrap, hiddenBox *mapsv1.MapPoint
	watches                            map[string]*watcher
}

func newTrapCallsTable(t *testing.T) *trapCallsTable {
	t.Helper()
	s := newScenes(t, true)
	hidden := s.master.createMap(s.campaign, "Masmorra fechada", s.master.newImage(s.campaign))
	c := &trapCallsTable{scenes: s, hiddenMap: hidden.GetId()}
	c.trap = s.newTrap("Fosso", 1000, 1000)
	c.chest = s.newTreasure("Baú", "Moedas", 250, 2000, 2000)
	c.hiddenTrap = c.pointOn(c.hiddenMap, mapsv1.MapPointKind_MAP_POINT_KIND_TRAP, "Fosso fechado")
	c.hiddenBox = c.pointOn(c.hiddenMap, mapsv1.MapPointKind_MAP_POINT_KIND_TREASURE, "Baú fechado")
	c.watches = map[string]*watcher{"master": s.master.watch(s.campaign), "Ana": s.ana.watch(s.campaign), "Caio": s.caio.watch(s.campaign)}
	return c
}

func (c *trapCallsTable) pointOn(mapID string, kind mapsv1.MapPointKind, name string) *mapsv1.MapPoint {
	c.h.t.Helper()
	req := &mapsv1.CreateMapPointRequest{CampaignId: c.campaign, MapId: mapID, Kind: kind, Name: name, XBp: 3000, YBp: 3000}
	if kind == mapsv1.MapPointKind_MAP_POINT_KIND_TRAP {
		req.Trap = testTrap()
	} else {
		req.TreasureValuePo = new(int32(100))
	}
	return c.master.createPoint(req)
}

// heard says how many times each watcher was told that mapID changed since the last call:
// it ends the stream with a change of a map the players see.
func (c *trapCallsTable) heard(mapID string) map[string]int {
	c.h.t.Helper()
	probe := c.master.createMap(c.campaign, "Sonda", c.master.newImage(c.campaign)).GetId()
	c.probe(probe)
	out := map[string]int{}
	for who, w := range c.watches {
		out[who] = 0
		for _, ev := range w.drain(probe) {
			if ev.GetMapChanged().GetMapId() == mapID {
				out[who]++
			}
		}
	}
	return out
}

func (c *trapCallsTable) reveal(mapID string, point *mapsv1.MapPoint, all bool, ids ...string) error {
	_, err := c.master.maps.RevealTrap(c.h.t.Context(), connect.NewRequest(&mapsv1.RevealTrapRequest{
		CampaignId: c.campaign, MapId: mapID, PointId: point.GetId(), All: all, CharacterIds: ids,
	}))
	return err
}

func (c *trapCallsTable) mark(mapID string, point *mapsv1.MapPoint, ids ...string) error {
	_, err := c.master.maps.MarkTreasureFound(c.h.t.Context(), connect.NewRequest(&mapsv1.MarkTreasureFoundRequest{
		CampaignId: c.campaign, MapId: mapID, PointId: point.GetId(), CharacterIds: ids,
	}))
	return err
}

func (c *trapCallsTable) unmark(mapID string, point *mapsv1.MapPoint) error {
	_, err := c.master.maps.UnmarkTreasureFound(c.h.t.Context(), connect.NewRequest(&mapsv1.UnmarkTreasureFoundRequest{
		CampaignId: c.campaign, MapId: mapID, PointId: point.GetId(),
	}))
	return err
}

// The three calls check in one order: the ids of the map and the point, then (for the
// two that name characters) the characters, then the map and the point in the
// transaction, then the kind of the point.
func TestTrapCallsRefuseInTheirOrder(t *testing.T) {
	t.Parallel()
	c := newTrapCallsTable(t)
	npc := c.master.createCharacter(c.campaign, charactersv1.CharacterKind_CHARACTER_KIND_MINION, "Goblin")
	pens := c.pens.GetId()
	missing := newKey()
	foreignCampaign := c.h.newCampaign(c.master)
	foreign := c.master.createMap(foreignCampaign, "De outra mesa", c.master.newImage(foreignCampaign))

	// RevealTrap.
	rv := func(mapID, pointID string, all bool, ids ...string) error {
		_, err := c.master.maps.RevealTrap(t.Context(), connect.NewRequest(&mapsv1.RevealTrapRequest{
			CampaignId: c.campaign, MapId: mapID, PointId: pointID, All: all, CharacterIds: ids,
		}))
		return err
	}
	wantRefusal(t, "reveal: a bad map id", rv("x", c.trap.GetId(), true), connect.CodeNotFound, "map not found")
	wantRefusal(t, "reveal: a bad point id", rv(c.mapID, "x", true), connect.CodeNotFound, "point not found")
	wantRefusal(t, "reveal: the map before the point", rv("x", "x", true), connect.CodeNotFound, "map not found")
	wantRefusal(t, "reveal: the ids before the choice of who", rv("x", c.trap.GetId(), false), connect.CodeNotFound, "map not found")
	both := "send character_ids or all, not both and not neither"
	wantRefusal(t, "reveal: neither", rv(c.mapID, c.trap.GetId(), false), connect.CodeInvalidArgument, both)
	wantRefusal(t, "reveal: both before the characters", rv(c.mapID, c.trap.GetId(), true, npc.GetId()), connect.CodeInvalidArgument, both)
	wantRefusal(t, "reveal: the choice before the point is found", rv(c.mapID, missing, false), connect.CodeInvalidArgument, both)
	wantRefusal(t, "reveal: a character that is no player's", rv(c.mapID, c.trap.GetId(), false, npc.GetId()), connect.CodeNotFound, "character not found")
	wantRefusal(t, "reveal: an id that is no id", rv(c.mapID, c.trap.GetId(), false, pens, "x"), connect.CodeNotFound, "character not found")
	wantRefusal(t, "reveal: 51 characters", rv(c.mapID, c.trap.GetId(), false, slices.Repeat([]string{pens}, 51)...), connect.CodeInvalidArgument, "character_ids must have 1 to 50 characters")
	wantRefusal(t, "reveal: the characters before the point", rv(c.mapID, missing, false, npc.GetId()), connect.CodeNotFound, "character not found")
	wantRefusal(t, "reveal: a point that is not there", rv(c.mapID, missing, true), connect.CodeNotFound, "point not found")
	wantRefusal(t, "reveal: a map of another campaign", rv(foreign.GetId(), c.trap.GetId(), true), connect.CodeNotFound, "map not found")
	wantRefusal(t, "reveal: a point of another map", rv(c.hiddenMap, c.trap.GetId(), true), connect.CodeNotFound, "point not found")
	wantRefusal(t, "reveal: a treasure", rv(c.mapID, c.chest.GetId(), true), connect.CodeInvalidArgument, "the point is not a TRAP point")
	// A character repeated is told once.
	if err := rv(c.mapID, c.trap.GetId(), false, pens, pens); err != nil {
		t.Errorf("reveal: a character twice: %v", err)
	}
	if got := c.masterPointOf(c.trap.GetId()).GetTrapRevealedTo(); len(got) != 1 {
		t.Errorf("a character named twice knows the trap %d times, want 1", len(got))
	}

	// MarkTreasureFound.
	mk := func(mapID, pointID string, ids ...string) error {
		_, err := c.master.maps.MarkTreasureFound(t.Context(), connect.NewRequest(&mapsv1.MarkTreasureFoundRequest{
			CampaignId: c.campaign, MapId: mapID, PointId: pointID, CharacterIds: ids,
		}))
		return err
	}
	wantRefusal(t, "mark: a bad map id", mk("x", c.chest.GetId(), pens), connect.CodeNotFound, "map not found")
	wantRefusal(t, "mark: a bad point id", mk(c.mapID, "x", pens), connect.CodeNotFound, "point not found")
	wantRefusal(t, "mark: the ids before the characters", mk("x", "x", npc.GetId()), connect.CodeNotFound, "map not found")
	wantRefusal(t, "mark: nobody", mk(c.mapID, c.chest.GetId()), connect.CodeInvalidArgument, "character_ids must have 1 to 50 characters")
	wantRefusal(t, "mark: an NPC", mk(c.mapID, c.chest.GetId(), npc.GetId()), connect.CodeNotFound, "character not found")
	wantRefusal(t, "mark: the characters before the point", mk(c.mapID, missing, npc.GetId()), connect.CodeNotFound, "character not found")
	wantRefusal(t, "mark: a point that is not there", mk(c.mapID, missing, pens), connect.CodeNotFound, "point not found")
	wantRefusal(t, "mark: a trap", mk(c.mapID, c.trap.GetId(), pens), connect.CodeInvalidArgument, "the point is not a TREASURE point")
	wantRefusal(t, "mark: a map of another campaign", mk(foreign.GetId(), c.chest.GetId(), pens), connect.CodeNotFound, "map not found")

	// UnmarkTreasureFound.
	um := func(mapID, pointID string) error {
		_, err := c.master.maps.UnmarkTreasureFound(t.Context(), connect.NewRequest(&mapsv1.UnmarkTreasureFoundRequest{CampaignId: c.campaign, MapId: mapID, PointId: pointID}))
		return err
	}
	wantRefusal(t, "unmark: a bad map id", um("x", c.chest.GetId()), connect.CodeNotFound, "map not found")
	wantRefusal(t, "unmark: a bad point id", um(c.mapID, "x"), connect.CodeNotFound, "point not found")
	wantRefusal(t, "unmark: a point that is not there", um(c.mapID, missing), connect.CodeNotFound, "point not found")
	wantRefusal(t, "unmark: a trap", um(c.mapID, c.trap.GetId()), connect.CodeInvalidArgument, "the point is not a TREASURE point")
	wantRefusal(t, "unmark: a map of another campaign", um(foreign.GetId(), c.chest.GetId()), connect.CodeNotFound, "map not found")
}

// Who is told of each change, on a map the players see and on one they do not: the master
// always (when something changed), a player only when the change is theirs to know.
func TestTrapCallsTellWhoMayKnow(t *testing.T) {
	t.Parallel()
	c := newTrapCallsTable(t)
	toren := c.other.GetId()
	pens := c.pens.GetId()
	c.heard(c.mapID) // the table's own changes
	type step struct {
		name string
		do   func() error
		want map[string]int
	}
	only := func(master, ana, caio int) map[string]int {
		return map[string]int{"master": master, "Ana": ana, "Caio": caio}
	}
	for _, shown := range []bool{true, false} {
		mapID, trap, box := c.mapID, c.trap, c.chest
		visible := 1
		if !shown {
			mapID, trap, box, visible = c.hiddenMap, c.hiddenTrap, c.hiddenBox, 0
		}
		steps := []step{
			{"a trap revealed to Toren", func() error { return c.reveal(mapID, trap, false, toren) }, only(1, 0, visible)},
			{"the same again", func() error { return c.reveal(mapID, trap, false, toren) }, only(0, 0, 0)},
			{"then to Pensantus too", func() error { return c.reveal(mapID, trap, false, toren, pens) }, only(1, visible, 0)},
			{"to everyone", func() error { return c.reveal(mapID, trap, true) }, only(1, visible, visible)},
			{"to everyone again", func() error { return c.reveal(mapID, trap, true) }, only(0, 0, 0)},
			{"to a character once it is public", func() error { return c.reveal(mapID, trap, false, toren) }, only(0, 0, 0)},
			{"a treasure found", func() error { return c.mark(mapID, box, pens) }, only(1, visible, visible)},
			{"found again by someone else", func() error { return c.mark(mapID, box, toren) }, only(1, visible, visible)},
			{"unmarked", func() error { return c.unmark(mapID, box) }, only(1, visible, visible)},
			{"unmarked again", func() error { return c.unmark(mapID, box) }, only(0, 0, 0)},
		}
		for _, st := range steps {
			if err := st.do(); err != nil {
				t.Fatalf("shown=%v, %s: %v", shown, st.name, err)
			}
			got := c.heard(mapID)
			for who, want := range st.want {
				if got[who] != want {
					t.Errorf("shown=%v, %s: %s was told %d times, want %d", shown, st.name, who, got[who], want)
				}
			}
		}
	}
	// One event for each change that was made, with ids only: the reveals (to Toren, to
	// Pensantus alone, to everyone) and the finds and unmarks, on both maps.
	if got := len(c.h.eventsOf("trap_revealed")); got != 6 {
		t.Errorf("trap_revealed events = %d, want 6", got)
	}
	if got := len(c.h.eventsOf("treasure_found")); got != 2 {
		t.Errorf("treasure_found events = %d, want 2", got)
	}
	if got := len(c.h.eventsOf("treasure_unfound")); got != 2 {
		t.Errorf("treasure_unfound events = %d, want 2", got)
	}
}

// What the master reads about a trap's reveals and a treasure's finders: the characters in the
// order the characters module lists them (players first, oldest first), each trap with its
// own reveals, and the finders of a treasure that is not found are never shown.
func TestTrapCallsTheMasterReadsWhoKnowsAndWhoFound(t *testing.T) {
	t.Parallel()
	c := newTrapCallsTable(t)
	toren, pens := c.other.GetId(), c.pens.GetId()
	second := c.pointOn(c.mapID, mapsv1.MapPointKind_MAP_POINT_KIND_TRAP, "Outro fosso")
	third := c.pointOn(c.mapID, mapsv1.MapPointKind_MAP_POINT_KIND_TREASURE, "Cofre")
	if err := c.mark(c.mapID, c.chest, toren, pens); err != nil {
		t.Fatal(err)
	}
	if err := c.reveal(c.mapID, c.trap, false, toren); err != nil {
		t.Fatal(err)
	}
	if err := c.reveal(c.mapID, c.trap, false, pens); err != nil {
		t.Fatal(err)
	}
	if err := c.reveal(c.mapID, second, false, pens); err != nil {
		t.Fatal(err)
	}
	// A finder of a treasure that was never found is a row the API cannot make; it is not shown.
	if _, err := c.h.pool.Exec(t.Context(), `INSERT INTO map_treasure_finders (point_id, character_id) VALUES ($1, $2)`, third.GetId(), pens); err != nil {
		t.Fatal(err)
	}
	names := func(p *mapsv1.MapPoint) (finders, revealed []string) {
		for _, f := range p.GetTreasureFoundBy() {
			finders = append(finders, f.GetCharacterName())
		}
		for _, r := range p.GetTrapRevealedTo() {
			revealed = append(revealed, r.GetCharacterName()+"/"+r.GetHow().String())
		}
		return finders, revealed
	}
	check := func(who string, p *mapsv1.MapPoint, wantFinders, wantRevealed []string) {
		t.Helper()
		finders, revealed := names(p)
		if !slices.Equal(finders, wantFinders) || !slices.Equal(revealed, wantRevealed) {
			t.Errorf("%s reads %s with finders %v and revealed %v, want %v and %v", who, p.GetName(), finders, revealed, wantFinders, wantRevealed)
		}
	}
	m := c.master.mustGetMap(c.campaign, c.mapID)
	check("the master", pointByID(m, c.chest.GetId()), []string{"Pensantus", "Toren"}, nil)
	check("the master", pointByID(m, c.trap.GetId()), nil, []string{"Toren/TRAP_REVEAL_HOW_MASTER", "Pensantus/TRAP_REVEAL_HOW_MASTER"})
	check("the master", pointByID(m, second.GetId()), nil, []string{"Pensantus/TRAP_REVEAL_HOW_MASTER"})
	check("the master", pointByID(m, third.GetId()), nil, nil)
	// A player reads the finders of the found treasure and nothing of who knows a trap.
	p := c.ana.mustGetMap(c.campaign, c.mapID)
	check("Ana", pointByID(p, c.chest.GetId()), []string{"Pensantus", "Toren"}, nil)
	check("Ana", pointByID(p, c.trap.GetId()), nil, nil)
	// Pensantus knows the second trap too, so Ana reads it; the treasure nobody found is hidden.
	check("Ana", pointByID(p, second.GetId()), nil, nil)
	if pointByID(p, third.GetId()) != nil {
		t.Error("Ana reads a treasure nobody found, because a finder row exists")
	}
}
