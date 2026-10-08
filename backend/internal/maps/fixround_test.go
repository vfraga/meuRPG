package maps

import (
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
)

// The fix round of slice 9.3: a triggered trap stays public, found treasure is
// never deleted silently, the players who know a trap hear of its changes, a
// hidden token has no light to set, and the master's settings leak no metadata.

func (s *scenes) pointOf(u *user, id string) *mapsv1.MapPoint {
	s.h.t.Helper()
	return pointByID(u.mustGetMap(s.campaign, s.mapID), id)
}

// D5: a trap that fired is visible to everyone for good: disarming it keeps it
// ("Desarmada"), and an edit that does not send the state never re-arms it.
func TestMR035_ATriggeredTrapStaysPublic(t *testing.T) {
	t.Parallel()
	s := newScenes(t, false)
	m := s.master
	trap := s.newTrap("Fosso", 1000, 1000)
	setState := func(state mapsv1.TrapState) *mapsv1.MapPoint {
		t.Helper()
		spec := testTrap()
		spec.State = state
		p, err := m.updatePoint(&mapsv1.UpdateMapPointRequest{CampaignId: s.campaign, MapId: s.mapID, PointId: trap.GetId(), Trap: spec})
		if err != nil {
			t.Fatalf("set the trap's state to %v: %v", state, err)
		}
		return p
	}
	count := func(u *user) int32 {
		for _, mp := range u.listMaps(s.campaign) {
			if mp.GetId() == s.mapID {
				return mp.GetPointCount()
			}
		}
		return -1
	}
	// An edit with no state keeps it (armed); the trap is still hidden.
	spec := testTrap()
	spec.FindDc = 18
	if p, err := m.updatePoint(&mapsv1.UpdateMapPointRequest{CampaignId: s.campaign, MapId: s.mapID, PointId: trap.GetId(), Trap: spec}); err != nil || p.GetTrap().GetState() != mapsv1.TrapState_TRAP_STATE_ARMED || p.GetTrap().GetFindDc() != 18 {
		t.Fatalf("an edit with no state = %v, %v; want it armed", p.GetTrap(), err)
	}
	if s.pointOf(s.ana, trap.GetId()) != nil {
		t.Fatal("an armed trap reached a player")
	}
	setState(mapsv1.TrapState_TRAP_STATE_TRIGGERED)
	for _, u := range []*user{s.ana, s.caio} {
		if p := s.pointOf(u, trap.GetId()); p == nil || p.GetTrap().GetState() != mapsv1.TrapState_TRAP_STATE_TRIGGERED {
			t.Errorf("a triggered trap, as a player sees it = %v, want it triggered", p)
		}
	}
	// Disarmed after firing: still seen, as "Desarmada", and still counted.
	setState(mapsv1.TrapState_TRAP_STATE_DISARMED)
	for _, u := range []*user{s.ana, s.caio} {
		if p := s.pointOf(u, trap.GetId()); p == nil || p.GetTrap().GetState() != mapsv1.TrapState_TRAP_STATE_DISARMED {
			t.Errorf("a disarmed trap that had fired = %v, want it seen as disarmed", p)
		}
		if got := count(u); got != 1 {
			t.Errorf("a player's point_count = %d, want 1 (the fired trap)", got)
		}
	}
	// An edit with no state keeps it disarmed; arming it again does not hide it.
	spec = testTrap()
	spec.NoticeDc = 20
	if p, err := m.updatePoint(&mapsv1.UpdateMapPointRequest{CampaignId: s.campaign, MapId: s.mapID, PointId: trap.GetId(), Trap: spec}); err != nil || p.GetTrap().GetState() != mapsv1.TrapState_TRAP_STATE_DISARMED {
		t.Errorf("an edit with no state = %v, %v; want it still disarmed", p.GetTrap(), err)
	}
	setState(mapsv1.TrapState_TRAP_STATE_ARMED)
	if p := s.pointOf(s.ana, trap.GetId()); p == nil || p.GetTrap().GetState() != mapsv1.TrapState_TRAP_STATE_ARMED {
		t.Errorf("a trap armed again after it fired = %v, want it seen: it fired", p)
	}
	// A trap born disarmed, which never fired, stays hidden.
	quiet := s.newTrap("Nunca disparou", 2000, 2000, func(t *mapsv1.TrapSpec) { t.State = mapsv1.TrapState_TRAP_STATE_DISARMED })
	if s.pointOf(s.ana, quiet.GetId()) != nil {
		t.Error("a trap that never fired reached a player because it was disarmed")
	}
}

// D8: a treasure that was found cannot be deleted, nor its map; unmarked, it can.
func TestMR041_FoundTreasureIsNeverDeletedSilently(t *testing.T) {
	t.Parallel()
	s := newScenes(t, false)
	m := s.master
	chest := s.newTreasure("Baú", "Moedas", 250, 100, 100)
	mark := func() {
		t.Helper()
		if _, err := m.maps.MarkTreasureFound(t.Context(), connect.NewRequest(&mapsv1.MarkTreasureFoundRequest{
			CampaignId: s.campaign, MapId: s.mapID, PointId: chest.GetId(), CharacterIds: []string{s.pens.GetId()},
		})); err != nil {
			t.Fatal(err)
		}
	}
	deletePoint := func() error {
		_, err := m.maps.DeleteMapPoint(t.Context(), connect.NewRequest(&mapsv1.DeleteMapPointRequest{CampaignId: s.campaign, MapId: s.mapID, PointId: chest.GetId()}))
		return err
	}
	deleteMap := func() error {
		_, err := m.maps.DeleteMap(t.Context(), connect.NewRequest(&mapsv1.DeleteMapRequest{CampaignId: s.campaign, MapId: s.mapID}))
		return err
	}
	mark()
	wantMapBlocked(t, "delete a found treasure", deletePoint(), mapsv1.MapBlockedReason_MAP_BLOCKED_REASON_TREASURE_FOUND)
	wantMapBlocked(t, "delete a map with a found treasure", deleteMap(), mapsv1.MapBlockedReason_MAP_BLOCKED_REASON_TREASURE_FOUND)
	// Converted wins over found.
	var award string
	if err := s.h.pool.QueryRow(t.Context(), `
		INSERT INTO xp_awards (campaign_id, created_at, mode, reason, gold, total_xp, idempotency_key)
		VALUES ($1, now(), 'gold', 'Voltar à cidade', 250, 250, gen_random_uuid()) RETURNING id::TEXT`, s.campaign).Scan(&award); err != nil {
		t.Fatal(err)
	}
	if _, err := s.h.pool.Exec(t.Context(), `UPDATE map_points SET treasure_converted_award_id = $1 WHERE id = $2`, award, chest.GetId()); err != nil {
		t.Fatal(err)
	}
	wantMapBlocked(t, "delete a converted treasure", deletePoint(), mapsv1.MapBlockedReason_MAP_BLOCKED_REASON_TREASURE_CONVERTED)
	wantMapBlocked(t, "delete a map with a converted treasure", deleteMap(), mapsv1.MapBlockedReason_MAP_BLOCKED_REASON_TREASURE_CONVERTED)
	if _, err := s.h.pool.Exec(t.Context(), `UPDATE map_points SET treasure_converted_award_id = NULL WHERE id = $1`, chest.GetId()); err != nil {
		t.Fatal(err)
	}
	// Unmarked, both go; a treasure that was never found goes at once.
	if _, err := m.maps.UnmarkTreasureFound(t.Context(), connect.NewRequest(&mapsv1.UnmarkTreasureFoundRequest{CampaignId: s.campaign, MapId: s.mapID, PointId: chest.GetId()})); err != nil {
		t.Fatal(err)
	}
	if err := deletePoint(); err != nil {
		t.Errorf("delete an unmarked treasure: %v", err)
	}
	other := s.newTreasure("Outro", "x", 1, 200, 200)
	chest = other
	if err := deletePoint(); err != nil {
		t.Errorf("delete a treasure that was never found: %v", err)
	}
	chest = s.newTreasure("De novo", "x", 1, 300, 300)
	mark()
	if _, err := m.maps.UnmarkTreasureFound(t.Context(), connect.NewRequest(&mapsv1.UnmarkTreasureFoundRequest{CampaignId: s.campaign, MapId: s.mapID, PointId: chest.GetId()})); err != nil {
		t.Fatal(err)
	}
	if err := deleteMap(); err != nil {
		t.Errorf("delete a map whose treasure was unmarked: %v", err)
	}
}

// RN-10: the players whose characters know a hidden trap hear of its changes and
// its deletion, and nobody else does.
func TestRN10_PlayersWhoKnowATrapHearOfItsChanges(t *testing.T) {
	t.Parallel()
	s := newScenes(t, true)
	m := s.master
	trap := s.newTrap("Fosso", 1000, 1000)
	if _, err := m.maps.RevealTrap(t.Context(), connect.NewRequest(&mapsv1.RevealTrapRequest{
		CampaignId: s.campaign, MapId: s.mapID, PointId: trap.GetId(), CharacterIds: []string{s.other.GetId()},
	})); err != nil {
		t.Fatal(err)
	}
	other := m.createMap(s.campaign, "Sonda", m.newImage(s.campaign)).GetId()
	anaWatch, caioWatch := s.ana.watch(s.campaign), s.caio.watch(s.campaign)
	check := func(what string, wantCaio int) {
		t.Helper()
		probe := m.createMap(s.campaign, "Sonda "+what, m.newImage(s.campaign)).GetId()
		s.probe(probe)
		if got := anaWatch.drain(probe); len(got) != 0 {
			t.Errorf("%s: Ana's stream got %v, want nothing", what, got)
		}
		if got := caioWatch.drain(probe); len(got) != wantCaio {
			t.Errorf("%s: Toren's stream got %v, want %d hint(s)", what, got, wantCaio)
		}
	}
	_ = other
	if _, err := m.updatePoint(&mapsv1.UpdateMapPointRequest{CampaignId: s.campaign, MapId: s.mapID, PointId: trap.GetId(), XBp: proto.Int32(2000)}); err != nil {
		t.Fatal(err)
	}
	check("a moved trap", 1)
	if _, err := m.maps.SetMapPointRevealed(t.Context(), connect.NewRequest(&mapsv1.SetMapPointRevealedRequest{CampaignId: s.campaign, MapId: s.mapID, PointId: trap.GetId(), Revealed: false})); err != nil {
		t.Fatal(err)
	}
	check("a hide that changes nothing", 1) // still a private trap: the knower hears the master touched it
	if _, err := m.maps.DeleteMapPoint(t.Context(), connect.NewRequest(&mapsv1.DeleteMapPointRequest{CampaignId: s.campaign, MapId: s.mapID, PointId: trap.GetId()})); err != nil {
		t.Fatal(err)
	}
	check("a deleted trap", 1)
	if got := s.caio.mustGetMap(s.campaign, s.mapID).GetPoints(); len(got) != 0 {
		t.Errorf("Toren still sees %v after the trap was deleted", got)
	}
}

// D6: a player has no light to set on a hidden token, and learns nothing from asking.
func TestMR036_ACarriedLightOnAHiddenToken(t *testing.T) {
	t.Parallel()
	s := newScenes(t, true)
	m := s.master
	m.placeToken(s.campaign, s.mapID, s.pens.GetId(), 3000, 3000)
	m.setTokenHidden(s.campaign, s.mapID, s.pens.GetId(), true)
	probeMap := m.createMap(s.campaign, "Sonda", m.newImage(s.campaign)).GetId()
	anaWatch := s.ana.watch(s.campaign)
	_, err := s.ana.maps.SetCarriedLight(t.Context(), connect.NewRequest(&mapsv1.SetCarriedLightRequest{CampaignId: s.campaign, MapId: s.mapID, CharacterId: s.pens.GetId(), LightKey: "light:torch"}))
	wantCode(t, "a player's light on their hidden token", err, connect.CodeNotFound)
	// The same answer as for a character with no token at all.
	_, err = s.ana.maps.SetCarriedLight(t.Context(), connect.NewRequest(&mapsv1.SetCarriedLightRequest{CampaignId: s.campaign, MapId: s.mapID, CharacterId: s.pens.GetId(), LightKey: ""}))
	wantCode(t, "a player's light-out on their hidden token", err, connect.CodeNotFound)
	for _, tk := range m.mustGetMap(s.campaign, s.mapID).GetTokens() {
		if tk.GetCarriedLight() != "" {
			t.Errorf("a refused call set a light: %v", tk)
		}
	}
	// The master can, and the player is not told while the token is hidden.
	if _, err := m.maps.SetCarriedLight(t.Context(), connect.NewRequest(&mapsv1.SetCarriedLightRequest{CampaignId: s.campaign, MapId: s.mapID, CharacterId: s.pens.GetId(), LightKey: "light:torch"})); err != nil {
		t.Fatalf("the master's light on a hidden token: %v", err)
	}
	s.probe(probeMap)
	if got := anaWatch.drain(probeMap); len(got) != 1 {
		t.Errorf("Ana's stream got %v, want only the master's own call's hint", got)
	}
}

// What the master's own settings must not tell the players: painting the light
// leaves the revision they see alone, and the base light is no change to them.
func TestMR036_MastersSettingsLeaveNoTraceForPlayers(t *testing.T) {
	t.Parallel()
	s := newScenes(t, true)
	m := s.master
	m.mustSetGrid(s.campaign, s.mapID, 20)
	m.mustPaint(s.campaign, s.mapID, mapsv1.MapLayer_MAP_LAYER_WALL, 1, [2]int32{1, 1})
	seen := func() *mapsv1.Map { return s.ana.mustGetMap(s.campaign, s.mapID).GetMap() }
	before := seen()
	masterBefore := m.mustGetMap(s.campaign, s.mapID).GetMap().GetLayersRevision()

	light := m.mustPaint(s.campaign, s.mapID, mapsv1.MapLayer_MAP_LAYER_LIGHT, 3, [2]int32{4, 4})
	if got := seen().GetLayersRevision(); got != before.GetLayersRevision() {
		t.Errorf("painting the light moved the players' layers_revision, %d to %d", before.GetLayersRevision(), got)
	}
	if got := m.mustGetMap(s.campaign, s.mapID).GetMap().GetLayersRevision(); got != masterBefore+1 || light.GetLayersRevision() != got || m.mustLayers(s.campaign, s.mapID).GetLayersRevision() != got {
		t.Errorf("the master's revision after painting the light = %d (call said %d), want %d", got, light.GetLayersRevision(), masterBefore+1)
	}
	if again := m.mustPaint(s.campaign, s.mapID, mapsv1.MapLayer_MAP_LAYER_LIGHT, 3, [2]int32{4, 4}); again.GetLayersRevision() != light.GetLayersRevision() {
		t.Errorf("painting the same light moved the revision to %d", again.GetLayersRevision())
	}

	probeMap := m.createMap(s.campaign, "Sonda", m.newImage(s.campaign)).GetId()
	anaWatch := s.ana.watch(s.campaign)
	if _, err := m.maps.SetMapFog(t.Context(), connect.NewRequest(&mapsv1.SetMapFogRequest{CampaignId: s.campaign, MapId: s.mapID, BaseLight: mapsv1.LightLevel_LIGHT_LEVEL_BRIGHT.Enum()})); err != nil {
		t.Fatal(err)
	}
	s.probe(probeMap)
	if got := anaWatch.drain(probeMap); len(got) != 0 {
		t.Errorf("a change of the base light reached Ana: %v", got)
	}
	if got := seen().GetUpdatedAt(); !got.AsTime().Equal(before.GetUpdatedAt().AsTime()) {
		t.Errorf("a change of the base light moved the map's updated_at the players read: %v to %v", before.GetUpdatedAt(), got)
	}
}

// Changing the kind of a found treasure would erase its finders and the found mark without a
// trace, so it is refused like a delete is; unmarked, the same change goes through.
func TestAFoundTreasureCannotChangeItsKindUntilItIsUnmarked(t *testing.T) {
	t.Parallel()
	s := newScenes(t, false)
	m := s.master
	chest := s.newTreasure("Baú", "Moedas", 250, 100, 100)
	if _, err := m.maps.MarkTreasureFound(t.Context(), connect.NewRequest(&mapsv1.MarkTreasureFoundRequest{
		CampaignId: s.campaign, MapId: s.mapID, PointId: chest.GetId(), CharacterIds: []string{s.pens.GetId()},
	})); err != nil {
		t.Fatal(err)
	}
	toScene := &mapsv1.UpdateMapPointRequest{
		CampaignId: s.campaign, MapId: s.mapID, PointId: chest.GetId(), Kind: mapsv1.MapPointKind_MAP_POINT_KIND_SCENE.Enum(),
	}
	_, err := m.updatePoint(toScene)
	wantMapBlocked(t, "change the kind of a found treasure", err, mapsv1.MapBlockedReason_MAP_BLOCKED_REASON_TREASURE_FOUND)
	var found int
	if err := s.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM map_points WHERE id = $1 AND kind = 'treasure' AND treasure_found_at IS NOT NULL`, chest.GetId()).Scan(&found); err != nil {
		t.Fatal(err)
	}
	if found != 1 {
		t.Error("the found treasure is no longer a found treasure after the refused kind change")
	}

	if _, err := m.maps.UnmarkTreasureFound(t.Context(), connect.NewRequest(&mapsv1.UnmarkTreasureFoundRequest{
		CampaignId: s.campaign, MapId: s.mapID, PointId: chest.GetId(),
	})); err != nil {
		t.Fatalf("UnmarkTreasureFound() error = %v", err)
	}
	if _, err := m.updatePoint(toScene); err != nil {
		t.Errorf("change the kind of an unmarked treasure error = %v, want success", err)
	}
}
