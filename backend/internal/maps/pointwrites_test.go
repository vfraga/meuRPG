package maps

import (
	"strings"
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
)

// The master's writes to points and tokens (CreateMapPoint, UpdateMapPoint,
// DeleteMapPoint, SetMapPointRevealed, PlaceMapToken, RemoveMapToken), frozen
// before the refactor that splits them into phases: each refusal with its code
// and message, which check wins when two apply, and what a successful write
// leaves in the point. Who sees what (RN-10) is frozen by the fog, kinds and
// scenes tests; these add the refusals and the order.

// missingUUID is a well-formed ID that names nothing.
const missingUUID = "00000000-0000-4000-8000-0000000000aa"

func (s *scenes) tryCreate(req *mapsv1.CreateMapPointRequest) error {
	s.h.t.Helper()
	req.CampaignId = s.campaign
	if req.MapId == "" {
		req.MapId = s.mapID
	}
	_, err := s.master.tryCreatePoint(req)
	return err
}

func (s *scenes) tryUpdate(p *mapsv1.MapPoint, edit func(*mapsv1.UpdateMapPointRequest)) (*mapsv1.MapPoint, error) {
	s.h.t.Helper()
	req := &mapsv1.UpdateMapPointRequest{CampaignId: s.campaign, MapId: s.mapID, PointId: p.GetId()}
	edit(req)
	return s.master.updatePoint(req)
}

func TestPointWrites_CreateMapPointRefusals(t *testing.T) {
	t.Parallel()
	s := newScenes(t, false)
	scene, battle, submap := mapsv1.MapPointKind_MAP_POINT_KIND_SCENE, mapsv1.MapPointKind_MAP_POINT_KIND_BATTLE, mapsv1.MapPointKind_MAP_POINT_KIND_SUBMAP
	other := s.master.createMap(s.campaign, "Outra", s.master.newImage(s.campaign))
	const invalid = connect.CodeInvalidArgument
	const notFound = connect.CodeNotFound
	tooLongText := strings.Repeat("a", maxDescriptionLength+1)
	tooLongHooks := strings.Repeat("a", maxHooksLength+1)
	const onlyScenes = "only a SCENE point has hooks"
	const onlyDC = "only a SCENE point shows a DC to the players"
	const position = "x_bp and y_bp must be 0 to 10000"

	cases := []struct {
		name string
		req  *mapsv1.CreateMapPointRequest
		code connect.Code
		msg  string
	}{
		{"a malformed map ID", &mapsv1.CreateMapPointRequest{MapId: "nope", Kind: scene, Name: "P"}, notFound, "map not found"},
		{"an unspecified kind", &mapsv1.CreateMapPointRequest{Name: "P"}, invalid, "kind is required"},
		{"an empty name", &mapsv1.CreateMapPointRequest{Kind: scene, Name: "  "}, invalid, "name must not be empty"},
		{"a long description", &mapsv1.CreateMapPointRequest{Kind: scene, Name: "P", Description: tooLongText}, invalid, "description must be at most 2000 characters, got 2001"},
		{"long hooks", &mapsv1.CreateMapPointRequest{Kind: scene, Name: "P", Hooks: tooLongHooks}, invalid, "hooks must be at most 4000 characters, got 4001"},
		{"hooks on a battle", &mapsv1.CreateMapPointRequest{Kind: battle, Name: "P", Hooks: "x"}, invalid, onlyScenes},
		{"the DC switch on a battle", &mapsv1.CreateMapPointRequest{Kind: battle, Name: "P", ShowDc: true}, invalid, onlyDC},
		{"x out of range", &mapsv1.CreateMapPointRequest{Kind: scene, Name: "P", XBp: maxPosition + 1}, invalid, position},
		{"y below range", &mapsv1.CreateMapPointRequest{Kind: scene, Name: "P", YBp: -1}, invalid, position},
		{"a target on a scene", &mapsv1.CreateMapPointRequest{Kind: scene, Name: "P", TargetMapId: other.GetId()}, invalid, "target_map_id is only for a SUBMAP or a BATTLE point"},
		{"a malformed target", &mapsv1.CreateMapPointRequest{Kind: submap, Name: "P", TargetMapId: "nope"}, invalid, "target_map_id is not a map of the campaign"},
		{"the map as its own target", &mapsv1.CreateMapPointRequest{Kind: submap, Name: "P", TargetMapId: s.mapID}, invalid, "target_map_id must be another map, not the point's own"},
		{"a target that is no map", &mapsv1.CreateMapPointRequest{Kind: battle, Name: "P", TargetMapId: missingUUID}, invalid, "target_map_id is not a map of the campaign"},
		{"a trap on a scene", &mapsv1.CreateMapPointRequest{Kind: scene, Name: "P", Trap: testTrap()}, invalid, "trap is only for a TRAP point"},
		{"a treasure value on a scene", &mapsv1.CreateMapPointRequest{Kind: scene, Name: "P", TreasureValuePo: new(int32(1))}, invalid, "treasure_value_po is only for a TREASURE point"},
		{"a light on a scene", &mapsv1.CreateMapPointRequest{Kind: scene, Name: "P", Light: &mapsv1.LightSpec{BrightFt: 5}}, invalid, "light is only for a LIGHT point"},
		{"a long idempotency key", &mapsv1.CreateMapPointRequest{Kind: scene, Name: "P", IdempotencyKey: strings.Repeat("k", 65)}, invalid, "idempotency_key must be 1 to 64 characters"},
		{"an unknown map", &mapsv1.CreateMapPointRequest{MapId: missingUUID, Kind: scene, Name: "P"}, notFound, "map not found"},

		// Which check wins when two apply: the order of the code.
		{"a bad map ID before a bad kind", &mapsv1.CreateMapPointRequest{MapId: "nope", Name: "P"}, notFound, "map not found"},
		{"a bad kind before a bad name", &mapsv1.CreateMapPointRequest{Name: ""}, invalid, "kind is required"},
		{"a bad name before a bad description", &mapsv1.CreateMapPointRequest{Kind: scene, Description: tooLongText}, invalid, "name must not be empty"},
		{"a bad description before bad hooks", &mapsv1.CreateMapPointRequest{Kind: scene, Name: "P", Description: tooLongText, Hooks: tooLongHooks}, invalid, "description must be at most 2000 characters, got 2001"},
		{"bad hooks before hooks on a battle", &mapsv1.CreateMapPointRequest{Kind: battle, Name: "P", Hooks: tooLongHooks, ShowDc: true}, invalid, "hooks must be at most 4000 characters, got 4001"},
		{"hooks on a battle before the DC switch", &mapsv1.CreateMapPointRequest{Kind: battle, Name: "P", Hooks: "x", ShowDc: true, XBp: -1}, invalid, onlyScenes},
		{"the DC switch before the position", &mapsv1.CreateMapPointRequest{Kind: battle, Name: "P", ShowDc: true, XBp: -1}, invalid, onlyDC},
		{"the position before the target", &mapsv1.CreateMapPointRequest{Kind: scene, Name: "P", XBp: -1, TargetMapId: other.GetId()}, invalid, position},
		{"the target before the spec", &mapsv1.CreateMapPointRequest{Kind: scene, Name: "P", TargetMapId: other.GetId(), Trap: testTrap()}, invalid, "target_map_id is only for a SUBMAP or a BATTLE point"},
		{"the spec before the idempotency key", &mapsv1.CreateMapPointRequest{Kind: scene, Name: "P", Trap: testTrap(), IdempotencyKey: strings.Repeat("k", 65)}, invalid, "trap is only for a TRAP point"},
		{"the idempotency key before the unknown map", &mapsv1.CreateMapPointRequest{MapId: missingUUID, Kind: scene, Name: "P", IdempotencyKey: strings.Repeat("k", 65)}, invalid, "idempotency_key must be 1 to 64 characters"},
		{"the unknown map before an unknown target", &mapsv1.CreateMapPointRequest{MapId: missingUUID, Kind: battle, Name: "P", TargetMapId: "00000000-0000-4000-8000-0000000000bb"}, notFound, "map not found"},
	}
	for _, c := range cases {
		wantRefusal(t, "CreateMapPoint with "+c.name, s.tryCreate(c.req), c.code, c.msg)
	}
	if got := len(s.master.mustGetMap(s.campaign, s.mapID).GetPoints()); got != 1 {
		t.Errorf("the map has %d points after the refusals, want only the fixture's", got)
	}
}

// What a point created of each plain kind keeps: the text cleaned, the target
// of a submap and a battle, hidden to start, the DC switch and the hooks of a
// scene.
func TestPointWrites_CreateMapPointKeepsWhatWasSent(t *testing.T) {
	t.Parallel()
	s := newScenes(t, false)
	other := s.master.createMap(s.campaign, "Outra", s.master.newImage(s.campaign))
	scene := s.master.createPoint(&mapsv1.CreateMapPointRequest{
		CampaignId: s.campaign, MapId: s.mapID, Kind: mapsv1.MapPointKind_MAP_POINT_KIND_SCENE, Name: "  Cena  ",
		Description: "linha 1\r\nlinha 2 ", Hooks: " segredo ", ShowDc: true, XBp: 10, YBp: 20,
	})
	if scene.GetName() != "Cena" || scene.GetDescription() != "linha 1\nlinha 2" || scene.GetHooks() != "segredo" || !scene.GetShowDc() ||
		scene.GetXBp() != 10 || scene.GetYBp() != 20 || scene.GetRevealed() || scene.GetTargetMap().GetId() != "" {
		t.Errorf("the created scene = %v, want it cleaned, hidden, with its hooks, DC switch and place", scene)
	}
	for _, kind := range []mapsv1.MapPointKind{mapsv1.MapPointKind_MAP_POINT_KIND_SUBMAP, mapsv1.MapPointKind_MAP_POINT_KIND_BATTLE} {
		p := s.master.createPoint(&mapsv1.CreateMapPointRequest{
			CampaignId: s.campaign, MapId: s.mapID, Kind: kind, Name: "Destino", TargetMapId: other.GetId(), XBp: maxPosition, YBp: maxPosition,
		})
		if p.GetKind() != kind || p.GetTargetMap().GetId() != other.GetId() || p.GetRevealed() || p.GetXBp() != maxPosition || p.GetYBp() != maxPosition {
			t.Errorf("the created %v point = %v, want it hidden, at the corner, leading to the other map", kind, p)
		}
	}
	// A kind that does not lead somewhere keeps no target even when it is empty.
	if p := s.newTreasure("Baú", "", 5, 1, 1); p.GetTargetMap().GetId() != "" || p.GetHooks() != "" || p.GetShowDc() {
		t.Errorf("the created treasure = %v, want no target, hooks or DC switch", p)
	}
}

func TestPointWrites_UpdateMapPointRefusals(t *testing.T) {
	t.Parallel()
	s := newScenes(t, false)
	scene, battle, submap, trapKind, treasureKind, lightKind := mapsv1.MapPointKind_MAP_POINT_KIND_SCENE, mapsv1.MapPointKind_MAP_POINT_KIND_BATTLE,
		mapsv1.MapPointKind_MAP_POINT_KIND_SUBMAP, mapsv1.MapPointKind_MAP_POINT_KIND_TRAP, mapsv1.MapPointKind_MAP_POINT_KIND_TREASURE, mapsv1.MapPointKind_MAP_POINT_KIND_LIGHT
	other := s.master.createMap(s.campaign, "Outra", s.master.newImage(s.campaign))
	otherPoint := s.master.createPoint(&mapsv1.CreateMapPointRequest{CampaignId: s.campaign, MapId: other.GetId(), Kind: scene, Name: "Da outra"})
	sub := s.master.createPoint(&mapsv1.CreateMapPointRequest{CampaignId: s.campaign, MapId: s.mapID, Kind: submap, Name: "Porta", TargetMapId: other.GetId()})
	trap := s.newTrap("Armadilha", 100, 100)
	treasure := s.newTreasure("Baú", "x", 10, 200, 200)
	light := s.newLight("Tocha", "light:torch")
	const invalid = connect.CodeInvalidArgument
	const notFound = connect.CodeNotFound
	const position = "x_bp and y_bp must be 0 to 10000"
	tooLongHooks := strings.Repeat("a", maxHooksLength+1)
	unspecified := mapsv1.MapPointKind_MAP_POINT_KIND_UNSPECIFIED

	cases := []struct {
		name  string
		point *mapsv1.MapPoint
		edit  func(*mapsv1.UpdateMapPointRequest)
		code  connect.Code
		msg   string
	}{
		{"a malformed map ID", s.point, func(r *mapsv1.UpdateMapPointRequest) { r.MapId = "nope"; r.Name = new("x") }, notFound, "map not found"},
		{"a malformed point ID", s.point, func(r *mapsv1.UpdateMapPointRequest) { r.PointId = "nope"; r.Name = new("x") }, notFound, "point not found"},
		{"nothing to change", s.point, func(*mapsv1.UpdateMapPointRequest) {}, invalid, "nothing to change"},
		{"an unspecified kind", s.point, func(r *mapsv1.UpdateMapPointRequest) { r.Kind = &unspecified }, invalid, "kind must not be unspecified"},
		{"an empty name", s.point, func(r *mapsv1.UpdateMapPointRequest) { r.Name = new(" ") }, invalid, "name must not be empty"},
		{"a long description", s.point, func(r *mapsv1.UpdateMapPointRequest) {
			r.Description = new(strings.Repeat("a", maxDescriptionLength+1))
		}, invalid, "description must be at most 2000 characters, got 2001"},
		{"long hooks", s.point, func(r *mapsv1.UpdateMapPointRequest) { r.Hooks = &tooLongHooks }, invalid, "hooks must be at most 4000 characters, got 4001"},
		{"x out of range", s.point, func(r *mapsv1.UpdateMapPointRequest) { r.XBp = new(int32(maxPosition + 1)) }, invalid, position},
		{"y below range", s.point, func(r *mapsv1.UpdateMapPointRequest) { r.YBp = new(int32(-1)) }, invalid, position},
		{"an unknown map", s.point, func(r *mapsv1.UpdateMapPointRequest) { r.MapId = missingUUID; r.Name = new("x") }, notFound, "map not found"},
		{"an unknown point", s.point, func(r *mapsv1.UpdateMapPointRequest) { r.PointId = missingUUID; r.Name = new("x") }, notFound, "point not found"},
		{"a point of another map", otherPoint, func(r *mapsv1.UpdateMapPointRequest) { r.Name = new("x") }, notFound, "point not found"},
		{"hooks on a battle", s.point, func(r *mapsv1.UpdateMapPointRequest) { r.Kind = &battle; r.Hooks = new("x") }, invalid, "only a SCENE point has hooks"},
		{"the DC switch on a battle", s.point, func(r *mapsv1.UpdateMapPointRequest) { r.Kind = &battle; r.ShowDc = new(true) }, invalid, "only a SCENE point shows a DC to the players"},
		{"hooks on a submap that already is one", sub, func(r *mapsv1.UpdateMapPointRequest) { r.Hooks = new("x") }, invalid, "only a SCENE point has hooks"},
		{"a target on a scene", s.point, func(r *mapsv1.UpdateMapPointRequest) { r.TargetMapId = new(other.GetId()) }, invalid, "target_map_id is only for a SUBMAP or a BATTLE point"},
		{"a malformed target", sub, func(r *mapsv1.UpdateMapPointRequest) { r.TargetMapId = new("nope") }, invalid, "target_map_id is not a map of the campaign"},
		{"the map as its own target", sub, func(r *mapsv1.UpdateMapPointRequest) { r.TargetMapId = new(s.mapID) }, invalid, "target_map_id must be another map, not the point's own"},
		{"a target that is no map", sub, func(r *mapsv1.UpdateMapPointRequest) { r.TargetMapId = new(missingUUID) }, invalid, "target_map_id is not a map of the campaign"},
		{"a trap on a scene", s.point, func(r *mapsv1.UpdateMapPointRequest) { r.Trap = testTrap() }, invalid, "trap is only for a TRAP point"},
		{"a treasure value on a scene", s.point, func(r *mapsv1.UpdateMapPointRequest) { r.TreasureValuePo = new(int32(1)) }, invalid, "treasure_value_po is only for a TREASURE point"},
		{"a light on a scene", s.point, func(r *mapsv1.UpdateMapPointRequest) { r.Light = &mapsv1.LightSpec{BrightFt: 5} }, invalid, "light is only for a LIGHT point"},
		{"a scene made a trap with no trap", s.point, func(r *mapsv1.UpdateMapPointRequest) { r.Kind = &trapKind }, invalid, "trap is required to make a point a TRAP"},
		{"a scene made a light with no light", s.point, func(r *mapsv1.UpdateMapPointRequest) { r.Kind = &lightKind }, invalid, "light is required to make a point a LIGHT"},
		{"a light revealed", light, func(r *mapsv1.UpdateMapPointRequest) { r.Revealed = new(true) }, invalid, "a LIGHT point is never shown to the players, so it cannot be revealed"},
		{"a bad treasure value", treasure, func(r *mapsv1.UpdateMapPointRequest) { r.TreasureValuePo = new(int32(-1)) }, invalid, "treasure_value_po must be 0 to 1000000"},
		{"a trap kept but given a treasure value", trap, func(r *mapsv1.UpdateMapPointRequest) { r.TreasureValuePo = new(int32(1)) }, invalid, "treasure_value_po is only for a TREASURE point"},

		// Which check wins when two apply.
		{"a bad map ID before a bad point ID", s.point, func(r *mapsv1.UpdateMapPointRequest) { r.MapId = "nope"; r.PointId = "nope" }, notFound, "map not found"},
		{"a bad point ID before nothing to change", s.point, func(r *mapsv1.UpdateMapPointRequest) { r.PointId = "nope" }, notFound, "point not found"},
		{"nothing to change before an unknown map", s.point, func(r *mapsv1.UpdateMapPointRequest) { r.MapId = missingUUID }, invalid, "nothing to change"},
		{"a bad kind before a bad name", s.point, func(r *mapsv1.UpdateMapPointRequest) { r.Kind = &unspecified; r.Name = new("") }, invalid, "kind must not be unspecified"},
		{"a bad name before a bad description", s.point, func(r *mapsv1.UpdateMapPointRequest) {
			r.Name = new("")
			r.Description = new(strings.Repeat("a", maxDescriptionLength+1))
		}, invalid, "name must not be empty"},
		{"bad hooks before a bad position", s.point, func(r *mapsv1.UpdateMapPointRequest) { r.Hooks = &tooLongHooks; r.XBp = new(int32(-1)) }, invalid, "hooks must be at most 4000 characters, got 4001"},
		{"a bad position before an unknown map", s.point, func(r *mapsv1.UpdateMapPointRequest) { r.MapId = missingUUID; r.XBp = new(int32(-1)) }, invalid, position},
		{"an unknown map before an unknown point", s.point, func(r *mapsv1.UpdateMapPointRequest) {
			r.MapId = missingUUID
			r.PointId = missingUUID
			r.Name = new("x")
		}, notFound, "map not found"},
		{"an unknown point before a bad target", s.point, func(r *mapsv1.UpdateMapPointRequest) { r.PointId = missingUUID; r.TargetMapId = new("nope") }, notFound, "point not found"},
		{"a bad target before the spec", s.point, func(r *mapsv1.UpdateMapPointRequest) { r.TargetMapId = new(other.GetId()); r.Trap = testTrap() }, invalid, "target_map_id is only for a SUBMAP or a BATTLE point"},
		{"a target that is no map before the spec", sub, func(r *mapsv1.UpdateMapPointRequest) { r.TargetMapId = new(missingUUID); r.Trap = testTrap() }, invalid, "target_map_id is not a map of the campaign"},
		{"the spec before hooks on a trap", s.point, func(r *mapsv1.UpdateMapPointRequest) { r.Kind = &trapKind; r.Hooks = new("x") }, invalid, "trap is required to make a point a TRAP"},
		{"hooks before the DC switch", s.point, func(r *mapsv1.UpdateMapPointRequest) { r.Kind = &battle; r.Hooks = new("x"); r.ShowDc = new(true) }, invalid, "only a SCENE point has hooks"},
		{"a light revealed before its missing light", s.point, func(r *mapsv1.UpdateMapPointRequest) { r.Kind = &lightKind; r.Revealed = new(true) }, invalid, "light is required to make a point a LIGHT"},
		{"a treasure's bad value before a light's missing data", s.point, func(r *mapsv1.UpdateMapPointRequest) {
			r.Kind = &treasureKind
			r.TreasureValuePo = new(int32(-1))
			r.Light = &mapsv1.LightSpec{BrightFt: 5}
		}, invalid, "treasure_value_po must be 0 to 1000000"},
	}
	for _, c := range cases {
		_, err := s.tryUpdate(c.point, c.edit)
		wantRefusal(t, "UpdateMapPoint with "+c.name, err, c.code, c.msg)
	}
	// A refused change writes nothing.
	got := s.master.mustGetMap(s.campaign, s.mapID)
	for _, p := range got.GetPoints() {
		if p.GetId() == s.point.GetId() && (p.GetKind() != scene || p.GetName() != s.point.GetName() || p.GetHooks() != "" || p.GetXBp() != 3000) {
			t.Errorf("the scene after the refusals = %v, want it unchanged", p)
		}
	}
}

// A successful update: each field changes alone, the others stay; leaving a
// scene clears its hooks and DC switch; a submap's target is cleared by an
// empty one or by leaving the kinds that lead; revealing keeps the first time.
func TestPointWrites_UpdateMapPointChangesWhatWasSent(t *testing.T) {
	t.Parallel()
	s := newScenes(t, false)
	other := s.master.createMap(s.campaign, "Outra", s.master.newImage(s.campaign))
	battle, submap, scene := mapsv1.MapPointKind_MAP_POINT_KIND_BATTLE, mapsv1.MapPointKind_MAP_POINT_KIND_SUBMAP, mapsv1.MapPointKind_MAP_POINT_KIND_SCENE
	must := func(p *mapsv1.MapPoint, edit func(*mapsv1.UpdateMapPointRequest)) *mapsv1.MapPoint {
		t.Helper()
		got, err := s.tryUpdate(p, edit)
		if err != nil {
			t.Fatalf("UpdateMapPoint() error = %v", err)
		}
		return got
	}

	p := must(s.point, func(r *mapsv1.UpdateMapPointRequest) {
		r.Name = new(" Nova ")
		r.Description = new("")
		r.Hooks = new("segredo")
		r.ShowDc = new(true)
		r.XBp = new(int32(1))
		r.YBp = new(int32(2))
	})
	if p.GetName() != "Nova" || p.GetDescription() != "" || p.GetHooks() != "segredo" || !p.GetShowDc() || p.GetXBp() != 1 || p.GetYBp() != 2 || p.GetKind() != scene {
		t.Errorf("the scene after the update = %v, want every sent field changed", p)
	}
	p = must(p, func(r *mapsv1.UpdateMapPointRequest) { r.Name = new("Só o nome") })
	if p.GetName() != "Só o nome" || p.GetHooks() != "segredo" || !p.GetShowDc() || p.GetXBp() != 1 {
		t.Errorf("the scene after a name-only update = %v, want the rest as it was", p)
	}
	revealed := must(p, func(r *mapsv1.UpdateMapPointRequest) { r.Revealed = new(true) })
	if !revealed.GetRevealed() {
		t.Fatalf("the revealed scene = %v, want it revealed", revealed)
	}
	again := s.master.setPointRevealed(s.campaign, revealed, true)
	if !again.GetRevealed() {
		t.Errorf("a second reveal = %v, want it still revealed", again)
	}

	// Leaving the scene clears its hooks and DC switch; a battle leads somewhere.
	b := must(revealed, func(r *mapsv1.UpdateMapPointRequest) { r.Kind = &battle; r.TargetMapId = new(other.GetId()) })
	if b.GetKind() != battle || b.GetHooks() != "" || b.GetShowDc() || b.GetTargetMap().GetId() != other.GetId() || !b.GetRevealed() {
		t.Errorf("the scene made a battle = %v, want no hooks or DC switch, the target set, still revealed", b)
	}
	// The same kind keeps its target; an empty target removes it.
	b = must(b, func(r *mapsv1.UpdateMapPointRequest) { r.Name = new("Luta") })
	if b.GetTargetMap().GetId() != other.GetId() {
		t.Errorf("the battle after a rename = %v, want its target kept", b)
	}
	b = must(b, func(r *mapsv1.UpdateMapPointRequest) { r.TargetMapId = new("") })
	if b.GetTargetMap().GetId() != "" {
		t.Errorf("the battle with an empty target = %v, want it removed", b)
	}
	b = must(b, func(r *mapsv1.UpdateMapPointRequest) { r.Kind = &submap; r.TargetMapId = new(other.GetId()) })
	if b.GetKind() != submap || b.GetTargetMap().GetId() != other.GetId() {
		t.Errorf("the battle made a submap = %v, want the target set", b)
	}
	// Back to a scene: the target goes, a scene sent with hooks keeps them.
	back := must(b, func(r *mapsv1.UpdateMapPointRequest) { r.Kind = &scene; r.Hooks = new("de volta") })
	if back.GetKind() != scene || back.GetTargetMap().GetId() != "" || back.GetHooks() != "de volta" {
		t.Errorf("the submap made a scene = %v, want no target and the hooks sent", back)
	}
	// Hiding clears the time.
	if hidden := must(back, func(r *mapsv1.UpdateMapPointRequest) { r.Revealed = new(false) }); hidden.GetRevealed() {
		t.Errorf("the hidden scene = %v, want it hidden", hidden)
	}
	// Empty hooks and a false DC switch are valid on any kind.
	if p := must(back, func(r *mapsv1.UpdateMapPointRequest) { r.Hooks = new(""); r.ShowDc = new(false) }); p.GetKind() != scene || p.GetHooks() != "" {
		t.Errorf("a scene with empty hooks = %v", p)
	}
}

func TestPointWrites_DeleteMapPointRefusals(t *testing.T) {
	t.Parallel()
	s := newScenes(t, false)
	other := s.master.createMap(s.campaign, "Outra", s.master.newImage(s.campaign))
	otherPoint := s.master.createPoint(&mapsv1.CreateMapPointRequest{CampaignId: s.campaign, MapId: other.GetId(), Kind: mapsv1.MapPointKind_MAP_POINT_KIND_SCENE, Name: "Da outra"})
	cases := []struct {
		name         string
		mapID, point string
		msg          string
	}{
		{"a malformed map ID", "nope", s.point.GetId(), "map not found"},
		{"a malformed point ID", s.mapID, "nope", "point not found"},
		{"an unknown map", missingUUID, s.point.GetId(), "map not found"},
		{"an unknown point", s.mapID, missingUUID, "point not found"},
		{"a point of another map", s.mapID, otherPoint.GetId(), "point not found"},
		{"a bad map ID before a bad point ID", "nope", "nope", "map not found"},
		{"an unknown map before an unknown point", missingUUID, missingUUID, "map not found"},
	}
	for _, c := range cases {
		_, err := s.master.maps.DeleteMapPoint(t.Context(), connect.NewRequest(&mapsv1.DeleteMapPointRequest{CampaignId: s.campaign, MapId: c.mapID, PointId: c.point}))
		wantRefusal(t, "DeleteMapPoint with "+c.name, err, connect.CodeNotFound, c.msg)
	}
	// Deleting twice: the second finds nothing.
	del := &mapsv1.DeleteMapPointRequest{CampaignId: s.campaign, MapId: s.mapID, PointId: s.point.GetId()}
	if _, err := s.master.maps.DeleteMapPoint(t.Context(), connect.NewRequest(del)); err != nil {
		t.Fatalf("DeleteMapPoint() error = %v", err)
	}
	_, err := s.master.maps.DeleteMapPoint(t.Context(), connect.NewRequest(del))
	wantRefusal(t, "DeleteMapPoint again", err, connect.CodeNotFound, "point not found")
}

func TestPointWrites_SetMapPointRevealedRefusals(t *testing.T) {
	t.Parallel()
	s := newScenes(t, false)
	other := s.master.createMap(s.campaign, "Outra", s.master.newImage(s.campaign))
	otherPoint := s.master.createPoint(&mapsv1.CreateMapPointRequest{CampaignId: s.campaign, MapId: other.GetId(), Kind: mapsv1.MapPointKind_MAP_POINT_KIND_SCENE, Name: "Da outra"})
	light := s.newLight("Tocha", "light:torch")
	cases := []struct {
		name         string
		mapID, point string
		code         connect.Code
		msg          string
	}{
		{"a malformed map ID", "nope", s.point.GetId(), connect.CodeNotFound, "map not found"},
		{"a malformed point ID", s.mapID, "nope", connect.CodeNotFound, "point not found"},
		{"an unknown map", missingUUID, s.point.GetId(), connect.CodeNotFound, "map not found"},
		{"an unknown point", s.mapID, missingUUID, connect.CodeNotFound, "point not found"},
		{"a point of another map", s.mapID, otherPoint.GetId(), connect.CodeNotFound, "point not found"},
		{"a bad map ID before a bad point ID", "nope", "nope", connect.CodeNotFound, "map not found"},
		{"a light", s.mapID, light.GetId(), connect.CodeInvalidArgument, "a LIGHT point is never shown to the players, so it cannot be revealed"},
	}
	for _, c := range cases {
		_, err := s.master.maps.SetMapPointRevealed(t.Context(), connect.NewRequest(&mapsv1.SetMapPointRevealedRequest{CampaignId: s.campaign, MapId: c.mapID, PointId: c.point, Revealed: true}))
		wantRefusal(t, "SetMapPointRevealed with "+c.name, err, c.code, c.msg)
	}
	// Hiding a light is valid (it stays hidden), as is hiding what is hidden.
	if p := s.master.setPointRevealed(s.campaign, light, false); p.GetRevealed() {
		t.Errorf("the hidden light = %v", p)
	}
	if p := s.master.setPointRevealed(s.campaign, s.point, false); p.GetRevealed() {
		t.Errorf("the scene hidden twice = %v", p)
	}
	if p := s.master.setPointRevealed(s.campaign, s.point, true); !p.GetRevealed() || p.GetKind() != mapsv1.MapPointKind_MAP_POINT_KIND_SCENE {
		t.Errorf("the revealed scene = %v", p)
	}
}

func TestPointWrites_TokenRefusals(t *testing.T) {
	t.Parallel()
	s := newScenes(t, false)
	npc := s.master.createCharacter(s.campaign, charactersv1.CharacterKind_CHARACTER_KIND_BOSS, "Capitão")
	const invalid = connect.CodeInvalidArgument
	const position = "x_bp and y_bp must be 0 to 10000"
	const subject = "set exactly one of character_id and creature_id"
	place := func(edit func(*mapsv1.PlaceMapTokenRequest)) error {
		req := &mapsv1.PlaceMapTokenRequest{CampaignId: s.campaign, MapId: s.mapID, CharacterId: s.pens.GetId(), XBp: 1, YBp: 1}
		edit(req)
		_, err := s.master.maps.PlaceMapToken(t.Context(), connect.NewRequest(req))
		return err
	}
	for _, c := range []struct {
		name string
		edit func(*mapsv1.PlaceMapTokenRequest)
		code connect.Code
		msg  string
	}{
		{"a malformed map ID", func(r *mapsv1.PlaceMapTokenRequest) { r.MapId = "nope" }, connect.CodeNotFound, "map not found"},
		{"x out of range", func(r *mapsv1.PlaceMapTokenRequest) { r.XBp = maxPosition + 1 }, invalid, position},
		{"y below range", func(r *mapsv1.PlaceMapTokenRequest) { r.YBp = -1 }, invalid, position},
		{"no subject", func(r *mapsv1.PlaceMapTokenRequest) { r.CharacterId = "" }, invalid, subject},
		{"two subjects", func(r *mapsv1.PlaceMapTokenRequest) { r.CreatureId = missingUUID }, invalid, subject},
		{"a malformed character ID", func(r *mapsv1.PlaceMapTokenRequest) { r.CharacterId = "nope" }, connect.CodeNotFound, "character not found"},
		{"an unknown character", func(r *mapsv1.PlaceMapTokenRequest) { r.CharacterId = missingUUID }, connect.CodeNotFound, "character not found"},
		{"an unknown map", func(r *mapsv1.PlaceMapTokenRequest) { r.MapId = missingUUID }, connect.CodeNotFound, "map not found"},
		{"a bad map ID before a bad position", func(r *mapsv1.PlaceMapTokenRequest) { r.MapId = "nope"; r.XBp = -1 }, connect.CodeNotFound, "map not found"},
		{"a bad position before the subject", func(r *mapsv1.PlaceMapTokenRequest) { r.XBp = -1; r.CharacterId = "" }, invalid, position},
		{"the subject before an unknown character", func(r *mapsv1.PlaceMapTokenRequest) { r.CreatureId = missingUUID; r.CharacterId = missingUUID }, invalid, subject},
		{"an unknown character before an unknown map", func(r *mapsv1.PlaceMapTokenRequest) { r.CharacterId = missingUUID; r.MapId = missingUUID }, connect.CodeNotFound, "character not found"},
	} {
		wantRefusal(t, "PlaceMapToken with "+c.name, place(c.edit), c.code, c.msg)
	}

	remove := func(edit func(*mapsv1.RemoveMapTokenRequest)) error {
		req := &mapsv1.RemoveMapTokenRequest{CampaignId: s.campaign, MapId: s.mapID, CharacterId: npc.GetId()}
		edit(req)
		_, err := s.master.maps.RemoveMapToken(t.Context(), connect.NewRequest(req))
		return err
	}
	for _, c := range []struct {
		name string
		edit func(*mapsv1.RemoveMapTokenRequest)
		code connect.Code
		msg  string
	}{
		{"a malformed map ID", func(r *mapsv1.RemoveMapTokenRequest) { r.MapId = "nope" }, connect.CodeNotFound, "map not found"},
		{"no subject", func(r *mapsv1.RemoveMapTokenRequest) { r.CharacterId = "" }, invalid, subject},
		{"two subjects", func(r *mapsv1.RemoveMapTokenRequest) { r.CreatureId = missingUUID }, invalid, subject},
		{"a malformed character ID", func(r *mapsv1.RemoveMapTokenRequest) { r.CharacterId = "nope" }, connect.CodeNotFound, "token not found"},
		{"a character with no token", func(*mapsv1.RemoveMapTokenRequest) {}, connect.CodeNotFound, "token not found"},
		{"an unknown map", func(r *mapsv1.RemoveMapTokenRequest) { r.MapId = missingUUID }, connect.CodeNotFound, "map not found"},
		{"a bad map ID before the subject", func(r *mapsv1.RemoveMapTokenRequest) { r.MapId = "nope"; r.CharacterId = "" }, connect.CodeNotFound, "map not found"},
		{"the subject before a malformed character", func(r *mapsv1.RemoveMapTokenRequest) { r.CharacterId = "nope"; r.CreatureId = missingUUID }, invalid, subject},
		{"a malformed character before an unknown map", func(r *mapsv1.RemoveMapTokenRequest) { r.CharacterId = "nope"; r.MapId = missingUUID }, connect.CodeNotFound, "token not found"},
		{"an unknown map before a missing token", func(r *mapsv1.RemoveMapTokenRequest) { r.MapId = missingUUID }, connect.CodeNotFound, "map not found"},
	} {
		wantRefusal(t, "RemoveMapToken with "+c.name, remove(c.edit), c.code, c.msg)
	}
}

// Placing and moving a token, and taking it off, on the fixture's map: the
// token a player's character gets, the hidden NPC, a move that keeps hidden, and
// a removal of a character that has died or is unknown to the campaign (its
// token goes all the same).
func TestPointWrites_TokensPlacedMovedRemoved(t *testing.T) {
	t.Parallel()
	s := newScenes(t, false)
	npc := s.master.createCharacter(s.campaign, charactersv1.CharacterKind_CHARACTER_KIND_BOSS, "Capitão")
	tk := s.master.placeToken(s.campaign, s.mapID, s.pens.GetId(), 0, maxPosition)
	if tk.GetHidden() || tk.GetXBp() != 0 || tk.GetYBp() != maxPosition || tk.GetCharacterId() != s.pens.GetId() || tk.GetName() != "Pensantus" {
		t.Errorf("the placed player token = %v", tk)
	}
	tk = s.master.placeToken(s.campaign, s.mapID, s.pens.GetId(), 7, 8)
	if tk.GetXBp() != 7 || tk.GetYBp() != 8 || tk.GetHidden() {
		t.Errorf("the moved player token = %v", tk)
	}
	s.master.placeToken(s.campaign, s.mapID, npc.GetId(), 5, 5)
	s.master.setTokenHidden(s.campaign, s.mapID, npc.GetId(), false)
	if tk = s.master.placeToken(s.campaign, s.mapID, npc.GetId(), 6, 6); tk.GetHidden() {
		t.Errorf("a moved NPC token = %v, want the visibility it had kept", tk)
	}
	if got := tokenIDs(s.master.mustGetMap(s.campaign, s.mapID)); len(got) != 2 {
		t.Errorf("the map's tokens = %v, want the two", got)
	}
	for _, id := range []string{s.pens.GetId(), npc.GetId()} {
		if _, err := s.master.maps.RemoveMapToken(t.Context(), connect.NewRequest(&mapsv1.RemoveMapTokenRequest{CampaignId: s.campaign, MapId: s.mapID, CharacterId: id})); err != nil {
			t.Errorf("RemoveMapToken(%s) error = %v", id, err)
		}
	}
	if got := tokenIDs(s.master.mustGetMap(s.campaign, s.mapID)); len(got) != 0 {
		t.Errorf("the map's tokens after the removals = %v, want none", got)
	}
}
