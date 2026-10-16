package maps

import (
	"slices"
	"testing"

	"connectrpc.com/connect"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
)

// DisarmTrap and GetTrapNoticers (the master's calls on a trap in play), frozen before they
// were split into phases: the order of their refusals, who is told of a disarm, what is
// written, and the answer of "Quem notaria" for characters in range, out of range and off the map.

// trapPlayTable is newScenes with a grid, a second map the players do not see, and the
// master and both players watching.
type trapPlayTable struct {
	*scenes
	hiddenMap string
	watches   map[string]*watcher
}

func newTrapPlayTable(t *testing.T) *trapPlayTable {
	t.Helper()
	s := newScenes(t, true)
	s.master.mustSetGrid(s.campaign, s.mapID, 20)
	c := &trapPlayTable{scenes: s}
	c.hiddenMap = s.master.createMap(s.campaign, "Masmorra fechada", s.master.newImage(s.campaign)).GetId()
	s.master.mustSetGrid(s.campaign, c.hiddenMap, 20)
	c.watches = map[string]*watcher{"master": s.master.watch(s.campaign), "Ana": s.ana.watch(s.campaign), "Caio": s.caio.watch(s.campaign)}
	return c
}

func (c *trapPlayTable) trapOn(mapID, name string, x, y int32) *mapsv1.MapPoint {
	c.h.t.Helper()
	return c.master.createPoint(&mapsv1.CreateMapPointRequest{
		CampaignId: c.campaign, MapId: mapID, Kind: mapsv1.MapPointKind_MAP_POINT_KIND_TRAP, Name: name, XBp: x, YBp: y, Trap: testTrap(),
	})
}

// heard says how many times each watcher was told that mapID changed since the last call.
func (c *trapPlayTable) heard(mapID string) map[string]int {
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

func (c *trapPlayTable) disarm(mapID string, trap *mapsv1.MapPoint) (*mapsv1.MapPoint, error) {
	res, err := c.master.maps.DisarmTrap(c.h.t.Context(), connect.NewRequest(&mapsv1.DisarmTrapRequest{CampaignId: c.campaign, MapId: mapID, PointId: trap.GetId()}))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetPoint(), nil
}

func TestTrapDisarmRefusesInItsOrderAndTellsWhoMayKnow(t *testing.T) {
	t.Parallel()
	c := newTrapPlayTable(t)
	toren := c.other.GetId()
	missing := newKey()
	other := c.h.newCampaign(c.master)
	foreign := c.master.createMap(other, "De outra mesa", c.master.newImage(other))

	// Refusals, in order.
	dis := func(mapID, pointID string) error {
		_, err := c.master.maps.DisarmTrap(t.Context(), connect.NewRequest(&mapsv1.DisarmTrapRequest{CampaignId: c.campaign, MapId: mapID, PointId: pointID}))
		return err
	}
	trap := c.trapOn(c.mapID, "Fosso", 1000, 1000)
	wantRefusal(t, "a bad map id", dis("x", trap.GetId()), connect.CodeNotFound, "map not found")
	wantRefusal(t, "a bad point id", dis(c.mapID, "x"), connect.CodeNotFound, "point not found")
	wantRefusal(t, "the map before the point", dis("x", "x"), connect.CodeNotFound, "map not found")
	wantRefusal(t, "a point that is not there", dis(c.mapID, missing), connect.CodeNotFound, "point not found")
	wantRefusal(t, "a map of another campaign", dis(foreign.GetId(), trap.GetId()), connect.CodeNotFound, "map not found")
	wantRefusal(t, "a point of another map", dis(c.hiddenMap, trap.GetId()), connect.CodeNotFound, "point not found")
	wantRefusal(t, "a scene", dis(c.mapID, c.point.GetId()), connect.CodeInvalidArgument, "the point is not a TRAP point")
	c.heard(c.mapID)

	// A trap nobody knows: a disarm tells the master only. A trap Toren knows: Toren too. A
	// public one: everyone who sees the map. On a map the players do not see: the master only.
	known := c.trapOn(c.mapID, "Fosso conhecido", 3000, 3000)
	if _, err := c.master.maps.RevealTrap(t.Context(), connect.NewRequest(&mapsv1.RevealTrapRequest{CampaignId: c.campaign, MapId: c.mapID, PointId: known.GetId(), CharacterIds: []string{toren}})); err != nil {
		t.Fatal(err)
	}
	public := c.trapOn(c.mapID, "Fosso público", 5000, 5000)
	c.master.setPointRevealed(c.campaign, public, true)
	hiddenKnown := c.trapOn(c.hiddenMap, "Fosso fechado", 3000, 3000)
	if _, err := c.master.maps.RevealTrap(t.Context(), connect.NewRequest(&mapsv1.RevealTrapRequest{CampaignId: c.campaign, MapId: c.hiddenMap, PointId: hiddenKnown.GetId(), CharacterIds: []string{toren}})); err != nil {
		t.Fatal(err)
	}
	c.heard(c.mapID)
	type step struct {
		name    string
		mapID   string
		trap    *mapsv1.MapPoint
		want    map[string]int
		changed bool
	}
	steps := []step{
		{"an unknown trap", c.mapID, trap, map[string]int{"master": 1, "Ana": 0, "Caio": 0}, true},
		{"the same again", c.mapID, trap, map[string]int{"master": 0, "Ana": 0, "Caio": 0}, false},
		{"a trap Toren knows", c.mapID, known, map[string]int{"master": 1, "Ana": 0, "Caio": 1}, true},
		{"a public trap", c.mapID, public, map[string]int{"master": 1, "Ana": 1, "Caio": 1}, true},
		{"a trap Toren knows on a map the players do not see", c.hiddenMap, hiddenKnown, map[string]int{"master": 1, "Ana": 0, "Caio": 0}, true},
	}
	for _, st := range steps {
		got, err := c.disarm(st.mapID, st.trap)
		if err != nil {
			t.Fatalf("%s: %v", st.name, err)
		}
		if got.GetTrap().GetState() != mapsv1.TrapState_TRAP_STATE_DISARMED {
			t.Errorf("%s: the trap reads %v, want disarmed", st.name, got.GetTrap().GetState())
		}
		heard := c.heard(st.mapID)
		for who, n := range st.want {
			if heard[who] != n {
				t.Errorf("%s: %s was told %d times, want %d", st.name, who, heard[who], n)
			}
		}
	}
	if got := len(c.h.eventsOf("trap_disarmed")); got != 4 {
		t.Errorf("trap_disarmed events = %d, want 4 (the repeat writes none)", got)
	}

	// A trap that fired stays public and keeps when it fired.
	fired := c.trapOn(c.mapID, "Fosso disparado", 7000, 7000)
	spec := testTrap()
	spec.State = mapsv1.TrapState_TRAP_STATE_TRIGGERED
	if _, err := c.master.updatePoint(&mapsv1.UpdateMapPointRequest{CampaignId: c.campaign, MapId: c.mapID, PointId: fired.GetId(), Trap: spec}); err != nil {
		t.Fatal(err)
	}
	c.heard(c.mapID)
	if _, err := c.disarm(c.mapID, fired); err != nil {
		t.Fatal(err)
	}
	if heard := c.heard(c.mapID); heard["master"] != 1 || heard["Ana"] != 1 || heard["Caio"] != 1 {
		t.Errorf("a fired trap disarmed was heard %v, want everyone once", heard)
	}
	if p := c.pointOf(c.ana, fired.GetId()); p == nil || p.GetTrap().GetState() != mapsv1.TrapState_TRAP_STATE_DISARMED {
		t.Errorf("Ana reads the fired trap as %v, want it seen and disarmed", p)
	}
	var triggeredAt *string
	if err := c.h.pool.QueryRow(t.Context(), `SELECT trap_triggered_at::TEXT FROM map_points WHERE id = $1`, fired.GetId()).Scan(&triggeredAt); err != nil || triggeredAt == nil {
		t.Errorf("a disarmed trap that fired lost when it fired: %v, %v", triggeredAt, err)
	}
}

// "Quem notaria": the party in the order of the characters module, each with what it would
// do from where it stands: in range, out of range, and (with no token) off the map.
func TestTrapNoticersAnswerForEveryoneOfTheParty(t *testing.T) {
	t.Parallel()
	c := newTrapPlayTable(t)
	trap := c.master.createPoint(&mapsv1.CreateMapPointRequest{
		CampaignId: c.campaign, MapId: c.mapID, Kind: mapsv1.MapPointKind_MAP_POINT_KIND_TRAP, Name: "Fosso", XBp: 1000, YBp: 1000,
		Trap: func() *mapsv1.TrapSpec { s := testTrap(); s.NoticeDc = 11; return s }(),
	})
	get := func(mapID, pointID string) (*mapsv1.GetTrapNoticersResponse, error) {
		res, err := c.master.maps.GetTrapNoticers(t.Context(), connect.NewRequest(&mapsv1.GetTrapNoticersRequest{CampaignId: c.campaign, MapId: mapID, PointId: pointID}))
		if err != nil {
			return nil, err
		}
		return res.Msg, nil
	}
	// Refusals.
	_, err := get("x", trap.GetId())
	wantRefusal(t, "a bad map id", err, connect.CodeNotFound, "map not found")
	_, err = get(c.mapID, "x")
	wantRefusal(t, "a bad point id", err, connect.CodeNotFound, "point not found")
	_, err = get(c.mapID, newKey())
	wantRefusal(t, "a point that is not there", err, connect.CodeNotFound, "point not found")
	_, err = get(c.mapID, c.point.GetId())
	wantRefusal(t, "a scene", err, connect.CodeInvalidArgument, "the point is not a TRAP point")

	// Nobody on the map: everyone off it.
	res, err := get(c.mapID, trap.GetId())
	if err != nil || res.GetNoticeDc() != 11 || len(res.GetNoticers()) != 2 {
		t.Fatalf("GetTrapNoticers() = %v, %v, want the DC and the two characters", res, err)
	}
	if res.GetNoticers()[0].GetCharacterName() != "Pensantus" || res.GetNoticers()[1].GetCharacterName() != "Toren" {
		t.Errorf("the party reads %v, want Pensantus then Toren", res.GetNoticers())
	}
	for _, n := range res.GetNoticers() {
		if n.GetOnMap() || n.GetInRange() || n.GetSees() || n.GetWouldNotice() || n.GetKnows() || n.GetLightPenalty() != 0 {
			t.Errorf("%s off the map reads %v, want only the passive score", n.GetCharacterName(), n)
		}
		if want := n.GetPassivePerception() >= 11; n.GetPassesDc() != want {
			t.Errorf("%s: passes_dc = %v with a passive %d against 11", n.GetCharacterName(), n.GetPassesDc(), n.GetPassivePerception())
		}
	}

	// Pensantus on the trap, Toren across the map; Toren knows the trap because the master told
	// him. Dropping Pensantus's token there made her notice it herself (passive 11 against DC 11).
	c.master.placeToken(c.campaign, c.mapID, c.pens.GetId(), 1250, 1000)
	c.master.placeToken(c.campaign, c.mapID, c.other.GetId(), 9000, 9000)
	if _, err := c.master.maps.RevealTrap(t.Context(), connect.NewRequest(&mapsv1.RevealTrapRequest{CampaignId: c.campaign, MapId: c.mapID, PointId: trap.GetId(), CharacterIds: []string{c.other.GetId()}})); err != nil {
		t.Fatal(err)
	}
	res, err = get(c.mapID, trap.GetId())
	if err != nil || len(res.GetNoticers()) != 2 {
		t.Fatalf("GetTrapNoticers() = %v, %v", res, err)
	}
	pens, toren := res.GetNoticers()[0], res.GetNoticers()[1]
	if !pens.GetOnMap() || !pens.GetInRange() || !pens.GetSees() || !pens.GetKnows() || pens.GetLightPenalty() != 0 ||
		pens.GetWouldNotice() != (pens.GetPassivePerception() >= 11) || pens.GetPassesDc() != pens.GetWouldNotice() {
		t.Errorf("Pensantus on the trap reads %v", pens)
	}
	if !toren.GetOnMap() || toren.GetInRange() || !toren.GetSees() || !toren.GetKnows() || toren.GetWouldNotice() || toren.GetLightPenalty() != 0 ||
		toren.GetPassesDc() != (toren.GetPassivePerception() >= 11) {
		t.Errorf("Toren across the map reads %v", toren)
	}
	t.Logf("passive scores: Pensantus %d, Toren %d", pens.GetPassivePerception(), toren.GetPassivePerception())

	// A trap with no DC to notice never passes; a map with no grid has no squares to notice from.
	noDC := c.master.createPoint(&mapsv1.CreateMapPointRequest{
		CampaignId: c.campaign, MapId: c.mapID, Kind: mapsv1.MapPointKind_MAP_POINT_KIND_TRAP, Name: "Sem teste", XBp: 1000, YBp: 1000,
		Trap: func() *mapsv1.TrapSpec { s := testTrap(); s.NoticeDc = 0; return s }(),
	})
	res, err = get(c.mapID, noDC.GetId())
	if err != nil || slices.ContainsFunc(res.GetNoticers(), func(n *mapsv1.TrapNoticer) bool { return n.GetPassesDc() || n.GetWouldNotice() }) {
		t.Errorf("a trap with no DC = %v, %v, want nobody passing", res, err)
	}
	bare := c.master.createMap(c.campaign, "Sem grade", c.master.newImage(c.campaign)).GetId()
	bareTrap := c.master.createPoint(&mapsv1.CreateMapPointRequest{
		CampaignId: c.campaign, MapId: bare, Kind: mapsv1.MapPointKind_MAP_POINT_KIND_TRAP, Name: "Sem grade", XBp: 1000, YBp: 1000, Trap: testTrap(),
	})
	res, err = get(bare, bareTrap.GetId())
	if err != nil || len(res.GetNoticers()) != 0 || res.GetNoticeDc() != 0 {
		t.Errorf("a trap on a map with no grid = %v, %v, want an empty answer", res, err)
	}
}
