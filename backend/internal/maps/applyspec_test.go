package maps

import (
	"context"
	"strconv"
	"testing"
	"time"

	"connectrpc.com/connect"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	"github.com/PuraFome/meuRPG/backend/internal/maps/mapsdb"
)

// applySpecChange is frozen here before it is split by kind (MR-035, MR-036,
// MR-041): UpdateMapPoint's trap, treasure and light data for the point after the
// change, what a point that changes kind leaves behind, and the order in which the
// three kinds are checked (trap, then treasure, then light, all inside the
// transaction that rolls back every write when one refuses).

// specRow is the columns applySpecChange decides, as the table holds them.
type specRow struct {
	kind      string
	trap      bool   // the spec is stored
	trapState string // "" for NULL
	triggered bool   // trap_triggered_at is set
	treasure  int32  // -1 for NULL
	found     bool
	light     string // "preset/bright/dim", "" for NULL
	revealed  bool
}

func mapsdbGetPoint(mapID, id string) mapsdb.GetMapPointParams {
	return mapsdb.GetMapPointParams{MapID: mapID, ID: id}
}

func itoa(n int32) string { return strconv.Itoa(int(n)) }

// convertTreasure turns the treasure into XP, as the party's return to town does (a
// treasure is found before it is converted: the table's CHECK says so).
func (s *scenes) convertTreasure(p *mapsv1.MapPoint) {
	s.h.t.Helper()
	s.markFound(p, s.pens.GetId())
	var award string
	if err := s.h.pool.QueryRow(s.h.t.Context(), `
		INSERT INTO xp_awards (campaign_id, created_at, mode, reason, gold, total_xp, idempotency_key)
		VALUES ($1, now(), 'gold', 'Voltar à cidade', 100, 100, gen_random_uuid()) RETURNING id::TEXT`, s.campaign).Scan(&award); err != nil {
		s.h.t.Fatalf("insert an XP award: %v", err)
	}
	if _, err := s.h.pool.Exec(s.h.t.Context(), `UPDATE map_points SET treasure_converted_award_id = $1 WHERE id = $2`, award, p.GetId()); err != nil {
		s.h.t.Fatalf("convert the treasure: %v", err)
	}
}

func (s *scenes) specRowOf(id string) specRow {
	s.h.t.Helper()
	p, err := s.h.svc.queries.GetMapPoint(s.h.t.Context(), mapsdbGetPoint(s.mapID, id))
	if err != nil {
		s.h.t.Fatalf("read the point: %v", err)
	}
	r := specRow{kind: p.Kind, trap: p.Trap != nil, treasure: -1, triggered: p.TrapTriggeredAt != nil, found: p.TreasureFoundAt != nil, revealed: p.RevealedAt != nil}
	if p.TrapState != nil {
		r.trapState = *p.TrapState
	}
	if p.TreasureValuePo != nil {
		r.treasure = *p.TreasureValuePo
	}
	if p.LightBrightFt != nil && p.LightDimFt != nil {
		preset := ""
		if p.LightPreset != nil {
			preset = *p.LightPreset
		}
		r.light = preset + "/" + itoa(*p.LightBrightFt) + "/" + itoa(*p.LightDimFt)
	}
	return r
}

// specRefusal is what a refused UpdateMapPoint says; reason is set for a MapBlocked.
type specRefusal struct {
	code    connect.Code
	message string
	reason  mapsv1.MapBlockedReason
}

func wantSpecRefusal(t *testing.T, call string, err error, want specRefusal) {
	t.Helper()
	wantRefusal(t, call, err, want.code, want.message)
	if want.reason != mapsv1.MapBlockedReason_MAP_BLOCKED_REASON_UNSPECIFIED {
		wantMapBlocked(t, call, err, want.reason)
	}
}

func specInvalid(msg string) specRefusal {
	return specRefusal{code: connect.CodeInvalidArgument, message: msg}
}

var (
	specTreasureFound = specRefusal{
		code:    connect.CodeFailedPrecondition,
		message: "the treasure was found: unmark it before deleting it or changing its kind",
		reason:  mapsv1.MapBlockedReason_MAP_BLOCKED_REASON_TREASURE_FOUND,
	}
	specTreasureConverted = specRefusal{
		code:    connect.CodeFailedPrecondition,
		message: "the treasure was turned into XP: its value and its found mark cannot change",
		reason:  mapsv1.MapBlockedReason_MAP_BLOCKED_REASON_TREASURE_CONVERTED,
	}
)

func TestApplySpecChange_Matrix(t *testing.T) { //nolint:tparallel // the cases share one database connection and edit one map in order
	t.Parallel()
	s := newScenes(t, false)
	m := s.master
	// Each case starts from a fresh point, edits it and says what the table holds after, or what refused.
	startScene := func() *mapsv1.MapPoint {
		return m.createPoint(&mapsv1.CreateMapPointRequest{CampaignId: s.campaign, MapId: s.mapID, Kind: mapsv1.MapPointKind_MAP_POINT_KIND_SCENE, Name: "Cena"})
	}
	startTrap := func() *mapsv1.MapPoint { return s.newTrap("Armadilha", 2000, 2000) }
	startTreasure := func() *mapsv1.MapPoint { return s.newTreasure("Tesouro", "", 100, 2100, 2000) }
	startLight := func() *mapsv1.MapPoint { return s.newLight("Luz", "light:torch") }
	startConverted := func() *mapsv1.MapPoint {
		p := startTreasure()
		s.convertTreasure(p)
		return p
	}
	convertedRow := func(v int32) specRow { return specRow{kind: "treasure", treasure: v, found: true} }
	startFound := func() *mapsv1.MapPoint {
		p := startTreasure()
		s.markFound(p, s.pens.GetId())
		return p
	}
	kind := func(k mapsv1.MapPointKind) *mapsv1.MapPointKind { return &k }
	trapSpec := func(edit ...func(*mapsv1.TrapSpec)) *mapsv1.TrapSpec {
		spec := testTrap()
		for _, e := range edit {
			e(spec)
		}
		return spec
	}
	scene, trap, treasure, light := mapsv1.MapPointKind_MAP_POINT_KIND_SCENE, mapsv1.MapPointKind_MAP_POINT_KIND_TRAP,
		mapsv1.MapPointKind_MAP_POINT_KIND_TREASURE, mapsv1.MapPointKind_MAP_POINT_KIND_LIGHT
	armed := specRow{kind: "trap", trap: true, trapState: "armed", treasure: -1}
	sceneRow := specRow{kind: "scene", treasure: -1}
	treasureRow := func(v int32) specRow { return specRow{kind: "treasure", treasure: v} }
	lightRow := func(l string) specRow { return specRow{kind: "light", treasure: -1, light: l} }

	for _, tt := range []struct {
		name    string
		start   func() *mapsv1.MapPoint
		edit    func(*mapsv1.UpdateMapPointRequest)
		want    specRow
		refused *specRefusal
	}{
		// The trap.
		{
			name: "a scene becomes a trap", start: startScene, want: armed,
			edit: func(r *mapsv1.UpdateMapPointRequest) { r.Kind, r.Trap = kind(trap), trapSpec() },
		},
		{
			name: "a scene becomes a triggered trap", start: startScene,
			want: specRow{kind: "trap", trap: true, trapState: "triggered", triggered: true, treasure: -1},
			edit: func(r *mapsv1.UpdateMapPointRequest) {
				r.Kind, r.Trap = kind(trap), trapSpec(func(t *mapsv1.TrapSpec) { t.State = mapsv1.TrapState_TRAP_STATE_TRIGGERED })
			},
		},
		{
			name: "a scene becomes a trap without a spec", start: startScene, refused: new(specInvalid("trap is required to make a point a TRAP")),
			edit: func(r *mapsv1.UpdateMapPointRequest) { r.Kind = kind(trap) },
		},
		{
			name: "a trap is renamed and keeps its spec", start: startTrap, want: armed,
			edit: func(r *mapsv1.UpdateMapPointRequest) { r.Name = new("Armadilha velha") },
		},
		{
			name: "a trap is sent its kind again", start: startTrap, want: armed,
			edit: func(r *mapsv1.UpdateMapPointRequest) { r.Kind = kind(trap) },
		},
		{
			name: "a trap gets a new spec that does not mention the state", start: startTrap, want: armed,
			edit: func(r *mapsv1.UpdateMapPointRequest) { r.Trap = trapSpec(func(t *mapsv1.TrapSpec) { t.FindDc = 9 }) },
		},
		{
			name: "a trap is triggered", start: startTrap,
			want: specRow{kind: "trap", trap: true, trapState: "triggered", triggered: true, treasure: -1},
			edit: func(r *mapsv1.UpdateMapPointRequest) {
				r.Trap = trapSpec(func(t *mapsv1.TrapSpec) { t.State = mapsv1.TrapState_TRAP_STATE_TRIGGERED })
			},
		},
		{
			name: "a trap is disarmed", start: startTrap, want: specRow{kind: "trap", trap: true, trapState: "disarmed", treasure: -1},
			edit: func(r *mapsv1.UpdateMapPointRequest) {
				r.Trap = trapSpec(func(t *mapsv1.TrapSpec) { t.State = mapsv1.TrapState_TRAP_STATE_DISARMED })
			},
		},
		{
			name: "a trap with a bad spec", start: startTrap, refused: new(specInvalid("trap.find_dc must be 1 to 30")),
			edit: func(r *mapsv1.UpdateMapPointRequest) { r.Trap = trapSpec(func(t *mapsv1.TrapSpec) { t.FindDc = 0 }) },
		},
		{
			name: "a trap becomes a scene", start: startTrap, want: sceneRow,
			edit: func(r *mapsv1.UpdateMapPointRequest) { r.Kind = kind(scene) },
		},
		{
			name: "a scene is sent a trap", start: startScene, refused: new(specInvalid("trap is only for a TRAP point")),
			edit: func(r *mapsv1.UpdateMapPointRequest) { r.Trap = trapSpec() },
		},
		// The treasure.
		{
			name: "a scene becomes a treasure of nothing", start: startScene, want: treasureRow(0),
			edit: func(r *mapsv1.UpdateMapPointRequest) { r.Kind = kind(treasure) },
		},
		{
			name: "a scene becomes a treasure with a value", start: startScene, want: treasureRow(250),
			edit: func(r *mapsv1.UpdateMapPointRequest) { r.Kind, r.TreasureValuePo = kind(treasure), new(int32(250)) },
		},
		{
			name: "a treasure changes its value", start: startTreasure, want: treasureRow(7),
			edit: func(r *mapsv1.UpdateMapPointRequest) { r.TreasureValuePo = new(int32(7)) },
		},
		{
			name: "a treasure is renamed and keeps its value", start: startTreasure, want: treasureRow(100),
			edit: func(r *mapsv1.UpdateMapPointRequest) { r.Name = new("Tesouro velho") },
		},
		{
			name: "a treasure is sent its kind again", start: startTreasure, want: treasureRow(100),
			edit: func(r *mapsv1.UpdateMapPointRequest) { r.Kind = kind(treasure) },
		},
		{
			name: "a treasure of too much", start: startTreasure, refused: new(specInvalid("treasure_value_po must be 0 to 1000000")),
			edit: func(r *mapsv1.UpdateMapPointRequest) { r.TreasureValuePo = new(int32(1_000_001)) },
		},
		{
			name: "a treasure of less than nothing", start: startTreasure, refused: new(specInvalid("treasure_value_po must be 0 to 1000000")),
			edit: func(r *mapsv1.UpdateMapPointRequest) { r.TreasureValuePo = new(int32(-1)) },
		},
		{
			name: "a scene is sent a value", start: startScene, refused: new(specInvalid("treasure_value_po is only for a TREASURE point")),
			edit: func(r *mapsv1.UpdateMapPointRequest) { r.TreasureValuePo = new(int32(5)) },
		},
		{
			name: "a treasure becomes a scene", start: startTreasure, want: sceneRow,
			edit: func(r *mapsv1.UpdateMapPointRequest) { r.Kind = kind(scene) },
		},
		{
			name: "a found treasure keeps its mark when it is renamed", start: startFound,
			want: specRow{kind: "treasure", treasure: 100, found: true},
			edit: func(r *mapsv1.UpdateMapPointRequest) { r.Name = new("Achado") },
		},
		{
			name: "a found treasure becomes a scene", start: startFound, refused: &specTreasureFound,
			edit: func(r *mapsv1.UpdateMapPointRequest) { r.Kind = kind(scene) },
		},
		{
			name: "a converted treasure becomes a scene", start: startConverted, refused: &specTreasureConverted,
			edit: func(r *mapsv1.UpdateMapPointRequest) { r.Kind = kind(scene) },
		},
		{
			name: "a converted treasure keeps its value when the same one is sent", start: startConverted, want: convertedRow(100),
			edit: func(r *mapsv1.UpdateMapPointRequest) { r.TreasureValuePo = new(int32(100)) },
		},
		{
			name: "a converted treasure changes its value", start: startConverted, refused: &specTreasureConverted,
			edit: func(r *mapsv1.UpdateMapPointRequest) { r.TreasureValuePo = new(int32(101)) },
		},
		{
			name: "a converted treasure with a value out of range", start: startConverted, refused: new(specInvalid("treasure_value_po must be 0 to 1000000")),
			edit: func(r *mapsv1.UpdateMapPointRequest) { r.TreasureValuePo = new(int32(-5)) },
		},
		{
			name: "a converted treasure is renamed", start: startConverted, want: convertedRow(100),
			edit: func(r *mapsv1.UpdateMapPointRequest) { r.Name = new("Convertido") },
		},
		// The light.
		{
			name: "a scene becomes a light", start: startScene, want: lightRow("light:torch/20/20"),
			edit: func(r *mapsv1.UpdateMapPointRequest) {
				r.Kind, r.Light = kind(light), &mapsv1.LightSpec{PresetKey: "light:torch", BrightFt: 20, DimFt: 20}
			},
		},
		{
			name: "a scene becomes a light without a spec", start: startScene, refused: new(specInvalid("light is required to make a point a LIGHT")),
			edit: func(r *mapsv1.UpdateMapPointRequest) { r.Kind = kind(light) },
		},
		{
			name: "a light is renamed and keeps its radii", start: startLight, want: lightRow("light:torch/20/20"),
			edit: func(r *mapsv1.UpdateMapPointRequest) { r.Name = new("Luz velha") },
		},
		{
			name: "a light changes its radii", start: startLight, want: lightRow("/10/15"),
			edit: func(r *mapsv1.UpdateMapPointRequest) { r.Light = &mapsv1.LightSpec{BrightFt: 10, DimFt: 15} },
		},
		{
			name: "a light with no radius", start: startLight, refused: new(specInvalid("light needs a radius: bright_ft and dim_ft cannot both be 0")),
			edit: func(r *mapsv1.UpdateMapPointRequest) { r.Light = &mapsv1.LightSpec{} },
		},
		{
			name: "a light is revealed", start: startLight,
			refused: new(specInvalid("a LIGHT point is never shown to the players, so it cannot be revealed")),
			edit:    func(r *mapsv1.UpdateMapPointRequest) { r.Revealed = new(true) },
		},
		{
			name: "a light is hidden", start: startLight, want: lightRow("light:torch/20/20"),
			edit: func(r *mapsv1.UpdateMapPointRequest) { r.Revealed = new(false) },
		},
		{
			name: "a scene is sent a light", start: startScene, refused: new(specInvalid("light is only for a LIGHT point")),
			edit: func(r *mapsv1.UpdateMapPointRequest) { r.Light = &mapsv1.LightSpec{PresetKey: "light:torch"} },
		},
		{
			name: "a light becomes a scene", start: startLight, want: sceneRow,
			edit: func(r *mapsv1.UpdateMapPointRequest) { r.Kind = kind(scene) },
		},
		// The order: the trap, then the treasure, then the light.
		{
			name: "a scene becomes a trap without a spec but with a bad value", start: startScene,
			refused: new(specInvalid("trap is required to make a point a TRAP")),
			edit:    func(r *mapsv1.UpdateMapPointRequest) { r.Kind, r.TreasureValuePo = kind(trap), new(int32(-1)) },
		},
		{
			name: "a trap spec on a treasure with a bad value", start: startScene,
			refused: new(specInvalid("trap is only for a TRAP point")),
			edit: func(r *mapsv1.UpdateMapPointRequest) {
				r.Kind, r.Trap, r.TreasureValuePo = kind(treasure), trapSpec(), new(int32(-1))
			},
		},
		{
			name: "a trap with a value", start: startScene, refused: new(specInvalid("treasure_value_po is only for a TREASURE point")),
			edit: func(r *mapsv1.UpdateMapPointRequest) {
				r.Kind, r.Trap, r.TreasureValuePo = kind(trap), trapSpec(), new(int32(3))
			},
		},
		{
			name: "a trap with a light", start: startScene, refused: new(specInvalid("light is only for a LIGHT point")),
			edit: func(r *mapsv1.UpdateMapPointRequest) {
				r.Kind, r.Trap, r.Light = kind(trap), trapSpec(), &mapsv1.LightSpec{PresetKey: "light:torch"}
			},
		},
		{
			name: "a found treasure becomes a scene with a light", start: startFound, refused: &specTreasureFound,
			edit: func(r *mapsv1.UpdateMapPointRequest) {
				r.Kind, r.Light = kind(scene), &mapsv1.LightSpec{PresetKey: "light:torch"}
			},
		},
		{
			name: "a found and converted treasure becomes a scene", start: startConverted, refused: &specTreasureConverted,
			edit: func(r *mapsv1.UpdateMapPointRequest) { r.Kind = kind(scene) },
		},
		{
			name: "a converted treasure becomes a trap without a spec", start: startConverted,
			refused: new(specInvalid("trap is required to make a point a TRAP")),
			edit:    func(r *mapsv1.UpdateMapPointRequest) { r.Kind = kind(trap) },
		},
		{
			name: "a light without a spec is refused before it is told it cannot be revealed", start: startScene,
			refused: new(specInvalid("light is required to make a point a LIGHT")),
			edit:    func(r *mapsv1.UpdateMapPointRequest) { r.Kind, r.Revealed = kind(light), new(true) },
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.start()
			before := s.specRowOf(p.GetId())
			req := &mapsv1.UpdateMapPointRequest{CampaignId: s.campaign, MapId: s.mapID, PointId: p.GetId()}
			tt.edit(req)
			_, err := m.updatePoint(req)
			if tt.refused != nil {
				wantSpecRefusal(t, tt.name, err, *tt.refused)
				if after := s.specRowOf(p.GetId()); after != before {
					t.Errorf("a refused change left %+v, want %+v", after, before)
				}
				return
			}
			if err != nil {
				t.Fatalf("UpdateMapPoint() error = %v", err)
			}
			if got := s.specRowOf(p.GetId()); got != tt.want {
				t.Errorf("the point is %+v, want %+v", got, tt.want)
			}
		})
	}
}

// The time a trap first fires is kept: triggering it again changes nothing, and
// editing it keeps it; re-arming keeps it too (finding, frozen: "for good" only
// means the stamp is never overwritten, it is not cleared on a re-arm).
func TestApplySpecChange_TheFirstTriggerIsKept(t *testing.T) {
	t.Parallel()
	s := newScenes(t, false)
	m := s.master
	p := s.newTrap("Armadilha", 2000, 2000)
	state := func(st mapsv1.TrapState) *mapsv1.TrapSpec {
		spec := testTrap()
		spec.State = st
		return spec
	}
	stamp := func() *time.Time {
		t.Helper()
		row, err := s.h.svc.queries.GetMapPoint(t.Context(), mapsdbGetPoint(s.mapID, p.GetId()))
		if err != nil {
			t.Fatalf("read the point: %v", err)
		}
		return row.TrapTriggeredAt
	}
	if stamp() != nil {
		t.Fatal("a new trap has a trigger time")
	}
	update := func(spec *mapsv1.TrapSpec) {
		t.Helper()
		if _, err := m.updatePoint(&mapsv1.UpdateMapPointRequest{CampaignId: s.campaign, MapId: s.mapID, PointId: p.GetId(), Trap: spec}); err != nil {
			t.Fatalf("UpdateMapPoint(trap) error = %v", err)
		}
	}
	update(state(mapsv1.TrapState_TRAP_STATE_TRIGGERED))
	first := stamp()
	if first == nil {
		t.Fatal("triggering a trap left no trigger time")
	}
	update(state(mapsv1.TrapState_TRAP_STATE_TRIGGERED))
	update(state(mapsv1.TrapState_TRAP_STATE_UNSPECIFIED))
	update(state(mapsv1.TrapState_TRAP_STATE_ARMED))
	update(state(mapsv1.TrapState_TRAP_STATE_TRIGGERED))
	if got := stamp(); got == nil || !got.Equal(*first) {
		t.Errorf("the trigger time is %v after more edits, want the first, %v", got, first)
	}
	// Leaving the kind clears it.
	if _, err := m.updatePoint(&mapsv1.UpdateMapPointRequest{CampaignId: s.campaign, MapId: s.mapID, PointId: p.GetId(), Kind: new(mapsv1.MapPointKind_MAP_POINT_KIND_SCENE)}); err != nil {
		t.Fatalf("UpdateMapPoint(scene) error = %v", err)
	}
	if got := stamp(); got != nil {
		t.Errorf("a scene has a trigger time %v", got)
	}
}

// A point that stops being a trap forgets who knew it, and a treasure that stops
// being one forgets who found it; a refusal later in the same call rolls both
// back (the writes are one transaction).
func TestApplySpecChange_WhatAKindChangeForgets(t *testing.T) {
	t.Parallel()
	s := newScenes(t, false)
	m := s.master
	count := func(table, id string) int {
		t.Helper()
		var n int
		if err := s.h.pool.QueryRow(t.Context(), "SELECT count(*) FROM "+table+" WHERE point_id = $1", id).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		return n
	}
	trap := s.newTrap("Armadilha", 2000, 2000)
	if _, err := m.maps.RevealTrap(t.Context(), connect.NewRequest(&mapsv1.RevealTrapRequest{
		CampaignId: s.campaign, MapId: s.mapID, PointId: trap.GetId(), CharacterIds: []string{s.pens.GetId()},
	})); err != nil {
		t.Fatalf("RevealTrap() error = %v", err)
	}
	if count("map_point_reveals", trap.GetId()) != 1 {
		t.Fatal("the reveal was not recorded")
	}
	// The trap forgets first, then the light is refused: nothing is lost.
	_, err := m.updatePoint(&mapsv1.UpdateMapPointRequest{
		CampaignId: s.campaign, MapId: s.mapID, PointId: trap.GetId(),
		Kind: new(mapsv1.MapPointKind_MAP_POINT_KIND_SCENE), Light: &mapsv1.LightSpec{PresetKey: "light:torch"},
	})
	wantSpecRefusal(t, "a trap becomes a scene with a light", err, specInvalid("light is only for a LIGHT point"))
	if count("map_point_reveals", trap.GetId()) != 1 {
		t.Error("a refused call forgot who knew the trap")
	}
	if _, err := m.updatePoint(&mapsv1.UpdateMapPointRequest{
		CampaignId: s.campaign, MapId: s.mapID, PointId: trap.GetId(), Kind: new(mapsv1.MapPointKind_MAP_POINT_KIND_SCENE),
	}); err != nil {
		t.Fatalf("UpdateMapPoint(scene) error = %v", err)
	}
	if count("map_point_reveals", trap.GetId()) != 0 {
		t.Error("a scene still remembers who knew it as a trap")
	}

	// A treasure with finders and nothing found mark cannot exist; a found one is refused, so the
	// finders are only ever deleted for an unmarked treasure: its table has none to forget, and
	// the call is frozen with the rows it has.
	treasure := s.newTreasure("Tesouro", "", 100, 2100, 2000)
	if _, err := m.updatePoint(&mapsv1.UpdateMapPointRequest{
		CampaignId: s.campaign, MapId: s.mapID, PointId: treasure.GetId(), Kind: new(mapsv1.MapPointKind_MAP_POINT_KIND_SCENE),
	}); err != nil {
		t.Fatalf("UpdateMapPoint(treasure to scene) error = %v", err)
	}
	if count("map_treasure_finders", treasure.GetId()) != 0 {
		t.Error("a scene has treasure finders")
	}
	// Finders left by hand (a mark undone elsewhere) go with the kind.
	stray := s.newTreasure("Tesouro solto", "", 100, 2200, 2000)
	if _, err := s.h.pool.Exec(context.Background(), `INSERT INTO map_treasure_finders (point_id, character_id) VALUES ($1, $2)`, stray.GetId(), s.pens.GetId()); err != nil {
		t.Fatalf("insert a finder: %v", err)
	}
	if _, err := m.updatePoint(&mapsv1.UpdateMapPointRequest{
		CampaignId: s.campaign, MapId: s.mapID, PointId: stray.GetId(), Kind: new(mapsv1.MapPointKind_MAP_POINT_KIND_SCENE),
	}); err != nil {
		t.Fatalf("UpdateMapPoint(stray treasure to scene) error = %v", err)
	}
	if count("map_treasure_finders", stray.GetId()) != 0 {
		t.Error("the finders of a treasure that became a scene stayed")
	}
}
