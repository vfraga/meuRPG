package play

import "testing"

// The map on the table screen: the TV reads it as a player with no character does, through the
// maps module. These tests need the database (MEURPG_TEST_DATABASE_URL).

// withTableScreenMaps connects the actual maps service to the play service, as cmd/api does.
func (f *fogCave) withTableScreenMaps() *fogCave {
	f.h.vision.SetService(f.msvc)
	return f
}

// imageOfMap is the id of the map's background image.
func (f *fogCave) imageOfMap(t *testing.T) string {
	t.Helper()
	var id string
	if err := f.h.pool.QueryRow(t.Context(), `SELECT image_id FROM maps WHERE id = $1`, f.mapID).Scan(&id); err != nil {
		t.Fatalf("read the map's image: %v", err)
	}
	return id
}

// With the fog on and "Visão do grupo" on, the map goes to the TV as the group sees it: the fog's squares, no
// image.
func TestTableScreenMapStageOnAFogMapWithGroupVision(t *testing.T) {
	t.Parallel()
	f := newFogCave(t).withTableScreenMaps()
	f.groupVision(t, true)
	got := f.mustGetScreen(t)
	m := got.GetStage().GetMap()
	if got.GetStage().GetKind() != kMap || m == nil || m.GetHiddenByFog() {
		t.Fatalf("stage = %v, body %v; want the map, shown", got.GetStage().GetKind(), m)
	}
	if m.GetMap().GetMap().GetId() != f.mapID || !m.GetMap().GetMap().GetImageWithheld() || m.GetMap().GetMap().GetImage().GetId() != "" {
		t.Errorf("map = %v, want the fog map with its image withheld", m.GetMap().GetMap())
	}
	if m.GetVision().GetGridColumns() == 0 || !m.GetVision().GetGroupVision() || m.GetRevision() == 0 {
		t.Errorf("vision = %v, revision %d; want the group's squares and the TV's revision", m.GetVision(), m.GetRevision())
	}
	mustNotContain(t, "the table screen", asJSON(t, got), f.imageOfMap(t))
}

// With the fog on and "Visão do grupo" off, the TV has hidden_by_fog and nothing of the map (RN-10).
func TestTableScreenCombatMapIsHiddenByTheFogWithoutGroupVision(t *testing.T) {
	t.Parallel()
	f := newFogCave(t).withTableScreenMaps()
	f.fight(t)
	got := f.mustGetScreen(t)
	m := got.GetStage().GetCombat().GetMap()
	if got.GetStage().GetKind() != kCombat || !m.GetHiddenByFog() {
		t.Fatalf("stage = %v, map %v; want the combat with its map hidden by the fog", got.GetStage().GetKind(), m)
	}
	if m.GetName() != "" || m.GetMap() != nil || m.GetVision() != nil || m.GetRevision() != 0 {
		t.Errorf("hidden map = %v, want hidden_by_fog and nothing else", m)
	}
	mustNotContain(t, "the table screen", asJSON(t, got), f.mapID, f.imageOfMap(t), "A caverna do Vale Seco")
}

// The combat's map is set when the encounter has one, and carries what the group sees.
func TestTableScreenCombatCarriesItsMap(t *testing.T) {
	t.Parallel()
	f := newFogCave(t).withTableScreenMaps()
	f.groupVision(t, true)
	f.fight(t)
	got := f.mustGetScreen(t)
	m := got.GetStage().GetCombat().GetMap()
	if got.GetStage().GetKind() != kCombat || m.GetHiddenByFog() || m.GetMap().GetMap().GetId() != f.mapID || m.GetVision().GetGridColumns() == 0 {
		t.Fatalf("stage = %v, combat map %v; want the combat with the map and its squares", got.GetStage().GetKind(), m)
	}
	f.stage(t)
	// An NPC the group does not see is not a token of the TV's map.
	mustNotContain(t, "the table screen", asJSON(t, f.mustGetScreen(t).GetStage().GetCombat().GetMap()), f.id(t, "Goblin 1"), f.id(t, "Ogro"))
}
