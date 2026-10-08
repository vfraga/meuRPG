package maps

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dbtest"
)

// The new kinds of point (Etapa 9): the trap (MR-035), the treasure (MR-041)
// and the light (MR-036), who sees each of them (RN-10), the treasure found, and
// the light a character carries.

// testTrap is a valid trap with every part of an effect.
func testTrap() *mapsv1.TrapSpec {
	return &mapsv1.TrapSpec{
		NoticeDc: 15, FindDc: 15, AreaSize: 2, Trigger: rulesv1.TrapTrigger_TRAP_TRIGGER_ENTER,
		Effect: &rulesv1.TrapEffect{
			Attack: &rulesv1.TrapAttack{Bonus: 5, Count: 2, Damage: &rulesv1.TrapDamage{Dice: "1d6", DamageTypeKey: "damage-type:piercing"}},
			Damage: []*rulesv1.TrapDamage{{Dice: "2d6", DamageTypeKey: "damage-type:bludgeoning"}},
			Conditions: []*rulesv1.TrapCondition{
				{ConditionKey: "condition:restrained", DurationPt: "1 hora"},
			},
			Save: &rulesv1.TrapSaveEffect{
				Ability: rulesv1.Ability_ABILITY_DEXTERITY, Dc: 12, AppliesTo: rulesv1.TrapSaveApplies_TRAP_SAVE_APPLIES_CAUGHT,
				OnFail: &rulesv1.TrapOnFail{
					Damage:    []*rulesv1.TrapDamage{{Dice: "1d8", DamageTypeKey: "damage-type:fire"}},
					Condition: &rulesv1.TrapCondition{ConditionKey: "condition:poisoned"},
				},
				OnPass: rulesv1.TrapPassOutcome_TRAP_PASS_OUTCOME_HALF,
			},
			Targets: rulesv1.TrapTargets_TRAP_TARGETS_MANUAL,
		},
	}
}

func (u *user) tryCreatePoint(req *mapsv1.CreateMapPointRequest) (*mapsv1.MapPoint, error) {
	res, err := u.maps.CreateMapPoint(u.h.t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetPoint(), nil
}

func (s *scenes) newTrap(name string, x, y int32, edit ...func(*mapsv1.TrapSpec)) *mapsv1.MapPoint {
	s.h.t.Helper()
	spec := testTrap()
	for _, e := range edit {
		e(spec)
	}
	return s.master.createPoint(&mapsv1.CreateMapPointRequest{
		CampaignId: s.campaign, MapId: s.mapID, Kind: mapsv1.MapPointKind_MAP_POINT_KIND_TRAP, Name: name, Description: "descrição de " + name,
		XBp: x, YBp: y, Trap: spec,
	})
}

func (s *scenes) newTreasure(name, description string, value int32, x, y int32) *mapsv1.MapPoint {
	s.h.t.Helper()
	return s.master.createPoint(&mapsv1.CreateMapPointRequest{
		CampaignId: s.campaign, MapId: s.mapID, Kind: mapsv1.MapPointKind_MAP_POINT_KIND_TREASURE, Name: name, Description: description,
		TreasureValuePo: new(value), XBp: x, YBp: y,
	})
}

func (s *scenes) newLight(name, preset string) *mapsv1.MapPoint {
	s.h.t.Helper()
	return s.master.createPoint(&mapsv1.CreateMapPointRequest{
		CampaignId: s.campaign, MapId: s.mapID, Kind: mapsv1.MapPointKind_MAP_POINT_KIND_LIGHT, Name: name,
		Light: &mapsv1.LightSpec{PresetKey: preset}, XBp: 2500, YBp: 2500,
	})
}

// MR-035, D5: a trap is created from each of the SRD's presets, copying it
// whole, and a light from each of the light presets, with the radii of the
// preset when none are sent; everything stays editable.
func TestMR035_PresetsFillTrapsAndLights(t *testing.T) {
	t.Parallel()
	s := newScenes(t, false)
	presets, err := s.master.content.ListTrapPresets(t.Context(), connect.NewRequest(&rulesv1.ListTrapPresetsRequest{CampaignId: s.campaign}))
	if err != nil {
		t.Fatalf("ListTrapPresets() error = %v", err)
	}
	if len(presets.Msg.GetPresets()) != 8 || len(presets.Msg.GetSeverities()) != 3 || len(presets.Msg.GetDamageByLevel()) != 4 {
		t.Fatalf("ListTrapPresets() = %d presets, %d severities, %d damage rows; want 8, 3 and 4",
			len(presets.Msg.GetPresets()), len(presets.Msg.GetSeverities()), len(presets.Msg.GetDamageByLevel()))
	}
	for _, p := range presets.Msg.GetPresets() {
		created, err := s.master.tryCreatePoint(&mapsv1.CreateMapPointRequest{
			CampaignId: s.campaign, MapId: s.mapID, Kind: mapsv1.MapPointKind_MAP_POINT_KIND_TRAP, Name: p.GetNamePt(), Description: p.GetDescriptionPt(),
			Trap: &mapsv1.TrapSpec{
				PresetKey: p.GetKey(), NoticeDc: p.GetNoticeDc(), FindDc: p.GetFindDc(), AreaSize: p.GetAreaSize(),
				Trigger: p.GetTrigger(), Effect: p.GetEffect(),
			},
		})
		if err != nil {
			t.Errorf("a trap from the preset %s: %v", p.GetKey(), err)
			continue
		}
		got := created.GetTrap()
		if got.GetPresetKey() != p.GetKey() || got.GetState() != mapsv1.TrapState_TRAP_STATE_ARMED || !proto.Equal(got.GetEffect(), p.GetEffect()) ||
			got.GetFindDc() != p.GetFindDc() || got.GetNoticeDc() != p.GetNoticeDc() || got.GetAreaSize() != p.GetAreaSize() || got.GetTrigger() != p.GetTrigger() {
			t.Errorf("the trap from %s = %v, want a copy of the preset %v", p.GetKey(), got, p)
		}
	}

	lights, err := s.master.content.ListLightPresets(t.Context(), connect.NewRequest(&rulesv1.ListLightPresetsRequest{CampaignId: s.campaign}))
	if err != nil || len(lights.Msg.GetPresets()) != 7 {
		t.Fatalf("ListLightPresets() = %v, %v; want 7 presets", lights.Msg, err)
	}
	for _, l := range lights.Msg.GetPresets() {
		created := s.newLight(l.GetNamePt(), l.GetKey())
		if got := created.GetLight(); got.GetPresetKey() != l.GetKey() || got.GetBrightFt() != l.GetBrightFt() || got.GetDimFt() != l.GetDimFt() {
			t.Errorf("the light from %s = %v, want the preset's radii %d and %d", l.GetKey(), got, l.GetBrightFt(), l.GetDimFt())
		}
	}
	// The master edits anything: a preset's radii are only where it starts.
	torch := s.master.createPoint(&mapsv1.CreateMapPointRequest{
		CampaignId: s.campaign, MapId: s.mapID, Kind: mapsv1.MapPointKind_MAP_POINT_KIND_LIGHT, Name: "Tocha fraca",
		Light: &mapsv1.LightSpec{PresetKey: "light:torch", BrightFt: 10, DimFt: 5},
	})
	if got := torch.GetLight(); got.GetPresetKey() != "light:torch" || got.GetBrightFt() != 10 || got.GetDimFt() != 5 {
		t.Errorf("an edited torch = %v, want 10 and 5", got)
	}
}

// MR-035, MR-036, MR-041: what each kind needs, and what it refuses.
func TestMR035_PointKindsAreValidated(t *testing.T) {
	t.Parallel()
	s := newScenes(t, false)
	m := s.master
	create := func(kind mapsv1.MapPointKind, edit func(*mapsv1.CreateMapPointRequest)) error {
		req := &mapsv1.CreateMapPointRequest{CampaignId: s.campaign, MapId: s.mapID, Kind: kind, Name: "Ponto"}
		edit(req)
		_, err := m.tryCreatePoint(req)
		return err
	}
	trapKind, treasureKind, lightKind := mapsv1.MapPointKind_MAP_POINT_KIND_TRAP, mapsv1.MapPointKind_MAP_POINT_KIND_TREASURE, mapsv1.MapPointKind_MAP_POINT_KIND_LIGHT
	tooLong := strings.Repeat("a", 61)

	// Every refusal of a trap, one change at a time to a valid trap.
	for name, edit := range map[string]func(*mapsv1.TrapSpec){
		"notice_dc 31":                 func(t *mapsv1.TrapSpec) { t.NoticeDc = 31 },
		"notice_dc -1":                 func(t *mapsv1.TrapSpec) { t.NoticeDc = -1 },
		"find_dc 0":                    func(t *mapsv1.TrapSpec) { t.FindDc = 0 },
		"find_dc 31":                   func(t *mapsv1.TrapSpec) { t.FindDc = 31 },
		"area 0":                       func(t *mapsv1.TrapSpec) { t.AreaSize = 0 },
		"area 5":                       func(t *mapsv1.TrapSpec) { t.AreaSize = 5 },
		"no trigger":                   func(t *mapsv1.TrapSpec) { t.Trigger = rulesv1.TrapTrigger_TRAP_TRIGGER_UNSPECIFIED },
		"unknown preset":               func(t *mapsv1.TrapSpec) { t.PresetKey = "trap:nope" },
		"unknown state":                func(t *mapsv1.TrapSpec) { t.State = mapsv1.TrapState(9) },
		"unknown targets":              func(t *mapsv1.TrapSpec) { t.Effect.Targets = rulesv1.TrapTargets(9) },
		"attack bonus 21":              func(t *mapsv1.TrapSpec) { t.Effect.Attack.Bonus = 21 },
		"attack count 0":               func(t *mapsv1.TrapSpec) { t.Effect.Attack.Count = 0 },
		"attack count 11":              func(t *mapsv1.TrapSpec) { t.Effect.Attack.Count = 11 },
		"attack with no damage":        func(t *mapsv1.TrapSpec) { t.Effect.Attack.Damage = nil },
		"21 dice":                      func(t *mapsv1.TrapSpec) { t.Effect.Damage[0].Dice = "21d6" },
		"a d7":                         func(t *mapsv1.TrapSpec) { t.Effect.Damage[0].Dice = "2d7" },
		"a d20":                        func(t *mapsv1.TrapSpec) { t.Effect.Damage[0].Dice = "1d20" },
		"dice with a bonus":            func(t *mapsv1.TrapSpec) { t.Effect.Damage[0].Dice = "2d6+1" },
		"dice with the modifier":       func(t *mapsv1.TrapSpec) { t.Effect.Damage[0].Dice = "1d8 + MOD" },
		"zero dice":                    func(t *mapsv1.TrapSpec) { t.Effect.Damage[0].Dice = "0d6" },
		"flat 0":                       func(t *mapsv1.TrapSpec) { t.Effect.Damage[0].Dice = "0" },
		"flat 101":                     func(t *mapsv1.TrapSpec) { t.Effect.Damage[0].Dice = "101" },
		"words for dice":               func(t *mapsv1.TrapSpec) { t.Effect.Damage[0].Dice = "muito" },
		"a class as a damage type":     func(t *mapsv1.TrapSpec) { t.Effect.Damage[0].DamageTypeKey = "class:wizard" },
		"an unknown damage type":       func(t *mapsv1.TrapSpec) { t.Effect.Damage[0].DamageTypeKey = "damage-type:nope" },
		"no damage type":               func(t *mapsv1.TrapSpec) { t.Effect.Damage[0].DamageTypeKey = "" },
		"5 damage parts":               func(t *mapsv1.TrapSpec) { t.Effect.Damage = slices.Repeat(t.Effect.Damage, 5) },
		"an unknown condition":         func(t *mapsv1.TrapSpec) { t.Effect.Conditions[0].ConditionKey = "condition:nope" },
		"a damage type as a condition": func(t *mapsv1.TrapSpec) { t.Effect.Conditions[0].ConditionKey = "damage-type:fire" },
		"5 conditions":                 func(t *mapsv1.TrapSpec) { t.Effect.Conditions = slices.Repeat(t.Effect.Conditions, 5) },
		"a duration of 61 characters":  func(t *mapsv1.TrapSpec) { t.Effect.Conditions[0].DurationPt = tooLong },
		"a duration with a line break": func(t *mapsv1.TrapSpec) { t.Effect.Conditions[0].DurationPt = "1\nhora" },
		"save with no ability":         func(t *mapsv1.TrapSpec) { t.Effect.Save.Ability = rulesv1.Ability_ABILITY_UNSPECIFIED },
		"save dc 0":                    func(t *mapsv1.TrapSpec) { t.Effect.Save.Dc = 0 },
		"save dc 31":                   func(t *mapsv1.TrapSpec) { t.Effect.Save.Dc = 31 },
		"save with no applies_to": func(t *mapsv1.TrapSpec) {
			t.Effect.Save.AppliesTo = rulesv1.TrapSaveApplies_TRAP_SAVE_APPLIES_UNSPECIFIED
		},
		"save of the hit, no attack": func(t *mapsv1.TrapSpec) {
			t.Effect.Attack = nil
			t.Effect.Save.AppliesTo = rulesv1.TrapSaveApplies_TRAP_SAVE_APPLIES_HIT
		},
		"save with no on_pass":        func(t *mapsv1.TrapSpec) { t.Effect.Save.OnPass = rulesv1.TrapPassOutcome_TRAP_PASS_OUTCOME_UNSPECIFIED },
		"a failure that does nothing": func(t *mapsv1.TrapSpec) { t.Effect.Save.OnFail = &rulesv1.TrapOnFail{} },
		"half of a failure with no damage": func(t *mapsv1.TrapSpec) {
			t.Effect.Save.OnFail = &rulesv1.TrapOnFail{Condition: &rulesv1.TrapCondition{ConditionKey: "condition:poisoned"}}
		},
		"an unknown condition on a failure": func(t *mapsv1.TrapSpec) { t.Effect.Save.OnFail.Condition.ConditionKey = "condition:nope" },
	} {
		spec := testTrap()
		edit(spec)
		err := create(trapKind, func(r *mapsv1.CreateMapPointRequest) { r.Trap = spec })
		wantCode(t, "a trap with "+name, err, connect.CodeInvalidArgument)
	}
	wantCode(t, "a TRAP with no trap", create(trapKind, func(*mapsv1.CreateMapPointRequest) {}), connect.CodeInvalidArgument)

	// What is valid: the edges of every limit, the flat needle damage, an empty
	// effect ("só descrição"), and an unspecified state, which is armed.
	for name, edit := range map[string]func(*mapsv1.TrapSpec){
		"notice_dc 0 (none)":      func(t *mapsv1.TrapSpec) { t.NoticeDc = 0 },
		"DCs 30":                  func(t *mapsv1.TrapSpec) { t.NoticeDc, t.FindDc, t.Effect.Save.Dc = 30, 30, 30 },
		"DCs 1":                   func(t *mapsv1.TrapSpec) { t.NoticeDc, t.FindDc, t.Effect.Save.Dc = 1, 1, 1 },
		"area 1":                  func(t *mapsv1.TrapSpec) { t.AreaSize = 1 },
		"area 4":                  func(t *mapsv1.TrapSpec) { t.AreaSize = 4 },
		"a manual trigger":        func(t *mapsv1.TrapSpec) { t.Trigger = rulesv1.TrapTrigger_TRAP_TRIGGER_MANUAL },
		"20d6":                    func(t *mapsv1.TrapSpec) { t.Effect.Damage[0].Dice = "20d6" },
		"1d12 and 1d4":            func(t *mapsv1.TrapSpec) { t.Effect.Damage[0].Dice, t.Effect.Attack.Damage.Dice = "1d12", "1d4" },
		"a flat 1 and a flat 100": func(t *mapsv1.TrapSpec) { t.Effect.Damage[0].Dice, t.Effect.Attack.Damage.Dice = "1", "100" },
		"attack 0, 10 attacks":    func(t *mapsv1.TrapSpec) { t.Effect.Attack.Bonus, t.Effect.Attack.Count = 0, 10 },
		"4 damage parts":          func(t *mapsv1.TrapSpec) { t.Effect.Damage = slices.Repeat(t.Effect.Damage, 4) },
		"a duration of 60":        func(t *mapsv1.TrapSpec) { t.Effect.Conditions[0].DurationPt = strings.Repeat("a", 60) },
		"an empty effect":         func(t *mapsv1.TrapSpec) { t.Effect = nil },
		"a save that none passes": func(t *mapsv1.TrapSpec) { t.Effect.Save.OnPass = rulesv1.TrapPassOutcome_TRAP_PASS_OUTCOME_NONE },
		"a save of the hit":       func(t *mapsv1.TrapSpec) { t.Effect.Save.AppliesTo = rulesv1.TrapSaveApplies_TRAP_SAVE_APPLIES_HIT },
	} {
		spec := testTrap()
		edit(spec)
		if err := create(trapKind, func(r *mapsv1.CreateMapPointRequest) { r.Trap = spec }); err != nil {
			t.Errorf("a trap with %s: %v", name, err)
		}
	}
	if got := s.newTrap("Sem estado", 100, 100).GetTrap(); got.GetState() != mapsv1.TrapState_TRAP_STATE_ARMED {
		t.Errorf("a new trap's state = %v, want ARMED", got.GetState())
	}
	if got := s.newTrap("Já disparada", 100, 100, func(t *mapsv1.TrapSpec) { t.State = mapsv1.TrapState_TRAP_STATE_DISARMED }).GetTrap(); got.GetState() != mapsv1.TrapState_TRAP_STATE_DISARMED {
		t.Errorf("a trap created disarmed = %v, want DISARMED", got.GetState())
	}
	// The dice are kept as one writing, the names filled in.
	got := s.newTrap("Escrita", 100, 100, func(t *mapsv1.TrapSpec) { t.Effect.Damage[0].Dice = " 2d6 " }).GetTrap()
	if d := got.GetEffect().GetDamage()[0]; d.GetDice() != "2d6" || d.GetDamageTypePt() != "concussão" {
		t.Errorf("the stored damage = %v, want 2d6 and the Portuguese name", d)
	}

	// Treasure and light.
	for _, v := range []int32{-1, 1_000_001} {
		wantCode(t, "a treasure worth "+strconv.Itoa(int(v)), create(treasureKind, func(r *mapsv1.CreateMapPointRequest) { r.TreasureValuePo = new(v) }), connect.CodeInvalidArgument)
	}
	for _, v := range []int32{0, 1, 1_000_000} {
		if err := create(treasureKind, func(r *mapsv1.CreateMapPointRequest) { r.TreasureValuePo = new(v) }); err != nil {
			t.Errorf("a treasure worth %d: %v", v, err)
		}
	}
	if got := s.master.createPoint(&mapsv1.CreateMapPointRequest{CampaignId: s.campaign, MapId: s.mapID, Kind: treasureKind, Name: "Sem valor"}); got.GetTreasureValuePo() != 0 {
		t.Errorf("a treasure with no value sent is worth %d, want 0", got.GetTreasureValuePo())
	}
	for name, spec := range map[string]*mapsv1.LightSpec{
		"121 feet":       {BrightFt: 125},
		"not a square":   {BrightFt: 7},
		"negative":       {BrightFt: -5, DimFt: 10},
		"both 0":         {},
		"a dim of 125":   {BrightFt: 5, DimFt: 125},
		"unknown preset": {PresetKey: "light:nope", BrightFt: 5},
		"a trap preset":  {PresetKey: "trap:simple-pit", BrightFt: 5},
	} {
		wantCode(t, "a light with "+name, create(lightKind, func(r *mapsv1.CreateMapPointRequest) { r.Light = spec }), connect.CodeInvalidArgument)
	}
	wantCode(t, "a LIGHT with no light", create(lightKind, func(*mapsv1.CreateMapPointRequest) {}), connect.CodeInvalidArgument)
	for _, spec := range []*mapsv1.LightSpec{{BrightFt: 120, DimFt: 120}, {BrightFt: 0, DimFt: 5}, {BrightFt: 5}} {
		if err := create(lightKind, func(r *mapsv1.CreateMapPointRequest) { r.Light = spec }); err != nil {
			t.Errorf("a light of %v: %v", spec, err)
		}
	}

	// Each kind refuses another's data.
	scene := mapsv1.MapPointKind_MAP_POINT_KIND_SCENE
	wantCode(t, "a scene with a trap", create(scene, func(r *mapsv1.CreateMapPointRequest) { r.Trap = testTrap() }), connect.CodeInvalidArgument)
	wantCode(t, "a scene with a treasure value", create(scene, func(r *mapsv1.CreateMapPointRequest) { r.TreasureValuePo = proto.Int32(1) }), connect.CodeInvalidArgument)
	wantCode(t, "a scene with a light", create(scene, func(r *mapsv1.CreateMapPointRequest) { r.Light = &mapsv1.LightSpec{BrightFt: 5} }), connect.CodeInvalidArgument)
	wantCode(t, "a trap with a treasure value", create(trapKind, func(r *mapsv1.CreateMapPointRequest) { r.Trap = testTrap(); r.TreasureValuePo = proto.Int32(1) }), connect.CodeInvalidArgument)
	wantCode(t, "a trap with hooks", create(trapKind, func(r *mapsv1.CreateMapPointRequest) { r.Trap = testTrap(); r.Hooks = "x" }), connect.CodeInvalidArgument)
	wantCode(t, "a treasure that leads somewhere", create(treasureKind, func(r *mapsv1.CreateMapPointRequest) { r.TargetMapId = s.mapID }), connect.CodeInvalidArgument)
}

// UpdateMapPoint: a trap is replaced whole (its state too), a treasure's value
// changes, and a point that changes kind leaves the old kind's data behind and
// needs the new kind's.
func TestMR035_UpdatingPointsOfTheNewKinds(t *testing.T) {
	t.Parallel()
	s := newScenes(t, false)
	m := s.master
	update := func(p *mapsv1.MapPoint, edit func(*mapsv1.UpdateMapPointRequest)) (*mapsv1.MapPoint, error) {
		req := &mapsv1.UpdateMapPointRequest{CampaignId: s.campaign, MapId: s.mapID, PointId: p.GetId()}
		edit(req)
		return m.updatePoint(req)
	}
	trap := s.newTrap("Fosso", 1000, 1000)
	spec := testTrap()
	spec.FindDc, spec.State = 20, mapsv1.TrapState_TRAP_STATE_DISARMED
	spec.Effect = nil
	got, err := update(trap, func(r *mapsv1.UpdateMapPointRequest) { r.Trap = spec })
	if err != nil || got.GetTrap().GetFindDc() != 20 || got.GetTrap().GetState() != mapsv1.TrapState_TRAP_STATE_DISARMED || len(got.GetTrap().GetEffect().GetDamage()) != 0 {
		t.Fatalf("a replaced trap = %v, %v; want the new spec, disarmed", got.GetTrap(), err)
	}
	// Other changes keep the trap.
	moved, err := update(trap, func(r *mapsv1.UpdateMapPointRequest) { r.XBp = proto.Int32(2000) })
	if err != nil || moved.GetTrap().GetFindDc() != 20 || moved.GetXBp() != 2000 {
		t.Errorf("a moved trap = %v, %v; want it to keep its spec", moved.GetTrap(), err)
	}
	bad := testTrap()
	bad.AreaSize = 5
	_, err = update(trap, func(r *mapsv1.UpdateMapPointRequest) { r.Trap = bad })
	wantCode(t, "update a trap's area to 5", err, connect.CodeInvalidArgument)
	_, err = update(trap, func(r *mapsv1.UpdateMapPointRequest) { r.TreasureValuePo = proto.Int32(5) })
	wantCode(t, "a treasure value on a trap", err, connect.CodeInvalidArgument)
	_, err = update(trap, func(r *mapsv1.UpdateMapPointRequest) { r.Light = &mapsv1.LightSpec{BrightFt: 5} })
	wantCode(t, "a light on a trap", err, connect.CodeInvalidArgument)

	// A trap that stops being one forgets who knew it.
	chest := m.createPoint(&mapsv1.CreateMapPointRequest{CampaignId: s.campaign, MapId: s.mapID, Kind: mapsv1.MapPointKind_MAP_POINT_KIND_BATTLE, Name: "Emboscada"})
	_, err = update(trap, func(r *mapsv1.UpdateMapPointRequest) { r.Kind = mapsv1.MapPointKind_MAP_POINT_KIND_TREASURE.Enum() })
	if err != nil {
		t.Fatalf("a trap becomes a treasure: %v", err)
	}
	asTreasure := m.mustGetMap(s.campaign, s.mapID)
	for _, p := range asTreasure.GetPoints() {
		if p.GetId() == trap.GetId() && (p.GetTrap() != nil || p.GetKind() != mapsv1.MapPointKind_MAP_POINT_KIND_TREASURE || p.GetTreasureValuePo() != 0) {
			t.Errorf("a trap that became a treasure = %v, want a treasure worth 0 with no trap", p)
		}
	}
	// A point that becomes a trap or a light needs the spec; one that has it is fine.
	_, err = update(chest, func(r *mapsv1.UpdateMapPointRequest) { r.Kind = mapsv1.MapPointKind_MAP_POINT_KIND_TRAP.Enum() })
	wantCode(t, "a battle becomes a trap with no spec", err, connect.CodeInvalidArgument)
	_, err = update(chest, func(r *mapsv1.UpdateMapPointRequest) { r.Kind = mapsv1.MapPointKind_MAP_POINT_KIND_LIGHT.Enum() })
	wantCode(t, "a battle becomes a light with no spec", err, connect.CodeInvalidArgument)
	asLight, err := update(chest, func(r *mapsv1.UpdateMapPointRequest) {
		r.Kind = mapsv1.MapPointKind_MAP_POINT_KIND_LIGHT.Enum()
		r.Light = &mapsv1.LightSpec{PresetKey: "light:lamp"}
	})
	if err != nil || asLight.GetLight().GetBrightFt() != 15 || asLight.GetLight().GetDimFt() != 30 {
		t.Errorf("a battle that became a lamp = %v, %v; want 15 and 30", asLight.GetLight(), err)
	}
	// A light cannot be revealed: no player ever receives it.
	_, err = update(chest, func(r *mapsv1.UpdateMapPointRequest) { r.Revealed = new(true) })
	wantCode(t, "revealing a light", err, connect.CodeInvalidArgument)
	_, err = m.maps.SetMapPointRevealed(t.Context(), connect.NewRequest(&mapsv1.SetMapPointRevealedRequest{CampaignId: s.campaign, MapId: s.mapID, PointId: chest.GetId(), Revealed: true}))
	wantCode(t, "SetMapPointRevealed on a light", err, connect.CodeInvalidArgument)
	back, err := update(chest, func(r *mapsv1.UpdateMapPointRequest) { r.Kind = mapsv1.MapPointKind_MAP_POINT_KIND_SCENE.Enum() })
	if err != nil || back.GetLight() != nil || back.GetKind() != mapsv1.MapPointKind_MAP_POINT_KIND_SCENE {
		t.Errorf("a light that became a scene = %v, %v; want no light", back, err)
	}

	// A treasure's value changes, within its limits.
	gold := s.newTreasure("Baú", "Moedas", 250, 3000, 3000)
	if got, err := update(gold, func(r *mapsv1.UpdateMapPointRequest) { r.TreasureValuePo = proto.Int32(400) }); err != nil || got.GetTreasureValuePo() != 400 {
		t.Errorf("a treasure's new value = %v, %v; want 400", got.GetTreasureValuePo(), err)
	}
	_, err = update(gold, func(r *mapsv1.UpdateMapPointRequest) { r.TreasureValuePo = proto.Int32(1_000_001) })
	wantCode(t, "a treasure worth too much", err, connect.CodeInvalidArgument)
}

// RN-10, D5, D6, D8: no player response, in any read or on the stream, carries a
// trap that was not revealed to their character (not even its ID or position), a
// DC or an effect, a light point, the light layer, or a hidden treasure. A trap
// revealed to Toren reaches only Toren's player; a triggered trap reaches everyone.
func TestRN10_PlayersNeverReceiveTrapsLightsOrHiddenTreasure(t *testing.T) {
	t.Parallel()
	s := newScenes(t, true)
	m := s.master
	// The watchers open first: everything the master prepares below reaches (or
	// does not reach) their streams, and the check sees all of it.
	probeSetup := m.createMap(s.campaign, "Sonda da preparação", m.newImage(s.campaign)).GetId()
	anaWatch, caioWatch := s.ana.watch(s.campaign), s.caio.watch(s.campaign)
	m.mustSetGrid(s.campaign, s.mapID, 20)
	m.mustPaint(s.campaign, s.mapID, mapsv1.MapLayer_MAP_LAYER_WALL, 1, [2]int32{3, 3})
	m.mustPaint(s.campaign, s.mapID, mapsv1.MapLayer_MAP_LAYER_LIGHT, 3, [2]int32{5, 5})
	// The base light and a torch Toren carries are the master's (and Toren's).
	if _, err := m.maps.SetMapFog(t.Context(), connect.NewRequest(&mapsv1.SetMapFogRequest{CampaignId: s.campaign, MapId: s.mapID, BaseLight: mapsv1.LightLevel_LIGHT_LEVEL_DIM.Enum()})); err != nil {
		t.Fatal(err)
	}
	m.placeToken(s.campaign, s.mapID, s.other.GetId(), 4000, 4000)
	if _, err := m.maps.SetCarriedLight(t.Context(), connect.NewRequest(&mapsv1.SetCarriedLightRequest{CampaignId: s.campaign, MapId: s.mapID, CharacterId: s.other.GetId(), LightKey: "light:torch"})); err != nil {
		t.Fatal(err)
	}

	trapA := s.newTrap("ARMADILHA-SECRETA-A", 1234, 2345)
	trapB := s.newTrap("ARMADILHA-SECRETA-B", 3456, 4567)
	trapC := s.newTrap("ARMADILHA-DISPARADA-C", 5678, 6789)
	trapD := s.newTrap("ARMADILHA-REVELADA-D", 7890, 8901)
	light := s.newLight("LUZ-SECRETA", "light:torch")
	hidden := s.newTreasure("TESOURO-ESCONDIDO", "CONTEUDO-ESCONDIDO", 7777, 1111, 9111)
	shown := s.newTreasure("TESOURO-REVELADO", "CONTEUDO-REVELADO", 8888, 2222, 9222)
	found := s.newTreasure("TESOURO-ACHADO", "CONTEUDO-ACHADO", 9999, 3333, 9333)
	m.setPointRevealed(s.campaign, trapD, true)
	m.setPointRevealed(s.campaign, shown, true)
	if _, err := m.maps.MarkTreasureFound(t.Context(), connect.NewRequest(&mapsv1.MarkTreasureFoundRequest{
		CampaignId: s.campaign, MapId: s.mapID, PointId: found.GetId(), CharacterIds: []string{s.pens.GetId()},
	})); err != nil {
		t.Fatalf("MarkTreasureFound() error = %v", err)
	}
	probeMap := m.createMap(s.campaign, "Sonda", m.newImage(s.campaign)).GetId()

	s.probe(probeSetup)
	anaSetup, caioSetup := anaWatch.drain(probeSetup), caioWatch.drain(probeSetup)
	if _, err := m.maps.RevealTrap(t.Context(), connect.NewRequest(&mapsv1.RevealTrapRequest{
		CampaignId: s.campaign, MapId: s.mapID, PointId: trapB.GetId(), CharacterIds: []string{s.other.GetId()},
	})); err != nil {
		t.Fatalf("RevealTrap(Toren) error = %v", err)
	}
	s.probe(probeMap)
	// A reveal to Toren reaches Toren's stream, and Ana's gets nothing at all.
	if got := anaWatch.drain(probeMap); len(got) != 0 {
		t.Errorf("Ana's stream got %v for a trap revealed to Toren, want nothing", got)
	}
	caioEvents := caioWatch.drain(probeMap)
	if len(caioEvents) != 1 || caioEvents[0].GetMapChanged().GetMapId() != s.mapID {
		t.Errorf("Toren's stream got %v for the trap revealed to him, want one map_changed", caioEvents)
	}

	// Phase 2: the master triggers trap C. Everyone who sees the map hears.
	spec := testTrap()
	spec.State = mapsv1.TrapState_TRAP_STATE_TRIGGERED
	if _, err := m.updatePoint(&mapsv1.UpdateMapPointRequest{CampaignId: s.campaign, MapId: s.mapID, PointId: trapC.GetId(), Trap: spec}); err != nil {
		t.Fatal(err)
	}
	probe2 := m.createMap(s.campaign, "Sonda 2", m.newImage(s.campaign)).GetId()
	s.probe(probe2)
	anaTrig := append(anaSetup, anaWatch.drain(probe2)...)
	caioTrig := append(caioSetup, caioWatch.drain(probe2)...)
	for who, got := range map[string][]*playv1.WatchGameSessionResponse{"Ana": anaTrig, "Toren": caioTrig} {
		if !slices.ContainsFunc(got, func(ev *playv1.WatchGameSessionResponse) bool { return ev.GetMapChanged().GetMapId() == s.mapID }) {
			t.Errorf("%s's stream = %v after trap C was triggered, want a map_changed for the map", who, got)
		}
	}

	// What each player received, in every read and event.
	read := func(u *user, events []*playv1.WatchGameSessionResponse) string {
		t.Helper()
		var seen []string
		maps, err := u.maps.ListMaps(t.Context(), connect.NewRequest(&mapsv1.ListMapsRequest{CampaignId: s.campaign}))
		if err != nil {
			t.Fatal(err)
		}
		seen = append(seen, compactJSON(t, maps.Msg), compactJSON(t, u.mustGetMap(s.campaign, s.mapID)), compactJSON(t, u.mustLayers(s.campaign, s.mapID)))
		live, err := u.play.GetLiveSession(t.Context(), connect.NewRequest(&playv1.GetLiveSessionRequest{CampaignId: s.campaign}))
		if err != nil {
			t.Fatal(err)
		}
		seen = append(seen, compactJSON(t, live.Msg))
		for _, ev := range events {
			seen = append(seen, compactJSON(t, ev))
		}
		return strings.Join(seen, "\n")
	}
	anaSaw, caioSaw := read(s.ana, anaTrig), read(s.caio, caioTrig)
	never := []string{
		"findDc", "noticeDc", "presetKey", `"effect"`, `"trigger"`, "damageTypeKey", "conditionKey", `"light"`, "LUZ-SECRETA", light.GetId(),
		`"xBp":2500`, hidden.GetId(), "TESOURO-ESCONDIDO", "CONTEUDO-ESCONDIDO", `"treasureValuePo":7777`, `"xBp":1111`,
		// A treasure that is revealed but not found shows neither what is in it nor its worth.
		"CONTEUDO-REVELADO", `"treasureValuePo":8888`,
		// What is the master's: the base light, and who knows each trap.
		`"baseLight"`, `"trapRevealedTo"`,
	}
	check := func(who, saw string, extra ...string) {
		t.Helper()
		for _, f := range append(slices.Clone(never), extra...) {
			if strings.Contains(saw, f) {
				t.Errorf("%s received %q:\n%s", who, f, saw)
			}
		}
	}
	check("Ana", anaSaw, `"carriedLight"`, "ARMADILHA-SECRETA-A", trapA.GetId(), `"xBp":1234`, `"yBp":2345`, "ARMADILHA-SECRETA-B", trapB.GetId(), `"xBp":3456`)
	check("Toren", caioSaw, "ARMADILHA-SECRETA-A", trapA.GetId(), `"xBp":1234`, `"yBp":2345`)
	// What the master prepared is in its own read, so the names above are the right ones to look for.
	if masterSaw := compactJSON(t, m.mustGetMap(s.campaign, s.mapID)); !strings.Contains(masterSaw, "ARMADILHA-SECRETA-A") || !strings.Contains(masterSaw, "findDc") || !strings.Contains(masterSaw, "LUZ-SECRETA") {
		t.Errorf("the master's read lacks the secrets the players must not get: %s", masterSaw)
	}
	// Positive controls: what each may see is there, or the test proves nothing.
	for who, saw := range map[string]string{"Ana": anaSaw, "Toren": caioSaw} {
		for _, want := range []string{
			"ARMADILHA-DISPARADA-C", trapC.GetId(), "ARMADILHA-REVELADA-D", "TESOURO-REVELADO",
			"TESOURO-ACHADO", "CONTEUDO-ACHADO", `"treasureValuePo":9999`, "Pensantus",
		} {
			if !strings.Contains(saw, want) {
				t.Errorf("%s's receipts lack %q, the test would prove nothing:\n%s", who, want, saw)
			}
		}
	}
	for _, want := range []string{"ARMADILHA-SECRETA-B", trapB.GetId()} {
		if !strings.Contains(caioSaw, want) {
			t.Errorf("Toren's receipts lack %q: the trap revealed to him", want)
		}
	}
	// A player sees a known trap's area and state, and nothing else of it.
	for _, p := range s.caio.mustGetMap(s.campaign, s.mapID).GetPoints() {
		if p.GetKind() != mapsv1.MapPointKind_MAP_POINT_KIND_TRAP {
			continue
		}
		if tr := p.GetTrap(); tr.GetAreaSize() != 2 || tr.GetFindDc() != 0 || tr.GetNoticeDc() != 0 || tr.GetEffect() != nil || tr.GetPresetKey() != "" || tr.GetTrigger() != 0 || len(p.GetTrapRevealedTo()) != 0 {
			t.Errorf("a player's trap = %v, want only the area and the state", p)
		}
	}
	// The counts follow what each sees: C, D, the revealed and the found treasure,
	// and Toren's own trap B.
	if got := len(s.ana.mustGetMap(s.campaign, s.mapID).GetPoints()); got != 4 {
		t.Errorf("Ana sees %d points, want 4", got)
	}
	if got := len(s.caio.mustGetMap(s.campaign, s.mapID).GetPoints()); got != 5 {
		t.Errorf("Toren sees %d points, want 5", got)
	}
	for who, u := range map[string]*user{"Ana": s.ana, "Toren": s.caio} {
		want := int32(4)
		if who == "Toren" {
			want = 5
		}
		var listed int32
		for _, mp := range u.listMaps(s.campaign) {
			if mp.GetId() == s.mapID {
				listed = mp.GetPointCount()
			}
		}
		if listed != want {
			t.Errorf("%s's ListMaps point_count = %d, want %d", who, listed, want)
		}
	}
	// The light layer, never: but the walls are there.
	for who, u := range map[string]*user{"Ana": s.ana, "Toren": s.caio} {
		layers := u.mustLayers(s.campaign, s.mapID)
		if len(layers.GetLight()) != 0 || !decode(t, layers).walls.Get(3, 3) {
			t.Errorf("%s's layers = %v, want the walls and no light", who, layers)
		}
	}
	// The master sees every point, with the DCs.
	all := m.mustGetMap(s.campaign, s.mapID)
	if len(all.GetPoints()) != 9 || all.GetMap().GetPointCount() != 9 {
		t.Errorf("the master sees %d points, count %d; want 9 (the scene too)", len(all.GetPoints()), all.GetMap().GetPointCount())
	}
	// The session's history names ids only.
	for _, e := range s.h.eventsOf("trap_revealed", "treasure_found") {
		for _, f := range []string{"ARMADILHA", "TESOURO", "Pensantus", "Toren", "CONTEUDO"} {
			if strings.Contains(e, f) {
				t.Errorf("the event %q carries a name or a text", e)
			}
		}
	}
}

// compactJSON is a message as the app's JSON, without the optional spaces
// protojson adds at random, so a test can look for `"xBp":1234`.
func compactJSON(t *testing.T, m proto.Message) string {
	t.Helper()
	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(asJSON(t, m))); err != nil {
		t.Fatalf("compact the JSON: %v", err)
	}
	return buf.String()
}

// noSpaces drops the spaces of a JSON text the database printed.
func noSpaces(s string) string { return strings.ReplaceAll(s, " ", "") }

// eventsOf reads session events of these kinds as "kind payload" lines.
func (h *harness) eventsOf(kinds ...string) []string {
	h.t.Helper()
	rows, err := h.pool.Query(h.t.Context(), `SELECT kind, payload::TEXT FROM session_events WHERE kind = ANY($1) ORDER BY seq`, kinds)
	if err != nil {
		h.t.Fatalf("read the events: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var kind, payload string
		if err := rows.Scan(&kind, &payload); err != nil {
			h.t.Fatal(err)
		}
		out = append(out, kind+" "+payload)
	}
	return out
}

// MR-035, D5: RevealTrap shows a trap to the characters the master picks or to
// everyone, once; the master sees who knows it; the event holds ids only; with no
// session nothing is written.
func TestMR035_RevealTrap(t *testing.T) {
	t.Parallel()
	s := newScenes(t, false)
	m := s.master
	trap := s.newTrap("Fosso escondido", 4000, 4000)
	reveal := func(u *user, edit func(*mapsv1.RevealTrapRequest)) (*mapsv1.MapPoint, error) {
		req := &mapsv1.RevealTrapRequest{CampaignId: s.campaign, MapId: s.mapID, PointId: trap.GetId()}
		edit(req)
		res, err := u.maps.RevealTrap(t.Context(), connect.NewRequest(req))
		if err != nil {
			return nil, err
		}
		return res.Msg.GetPoint(), nil
	}
	wantTrapFor := func(name string, u *user, want bool) {
		t.Helper()
		seen := slices.ContainsFunc(u.mustGetMap(s.campaign, s.mapID).GetPoints(), func(p *mapsv1.MapPoint) bool { return p.GetId() == trap.GetId() })
		if seen != want {
			t.Errorf("%s sees the trap = %v, want %v", name, seen, want)
		}
	}
	wantTrapFor("Ana", s.ana, false)

	// Refusals.
	_, err := reveal(m, func(*mapsv1.RevealTrapRequest) {})
	wantCode(t, "RevealTrap with nobody", err, connect.CodeInvalidArgument)
	_, err = reveal(m, func(r *mapsv1.RevealTrapRequest) { r.All, r.CharacterIds = true, []string{s.pens.GetId()} })
	wantCode(t, "RevealTrap with all and characters", err, connect.CodeInvalidArgument)
	var many []string
	for range 51 {
		many = append(many, s.pens.GetId())
	}
	_, err = reveal(m, func(r *mapsv1.RevealTrapRequest) { r.CharacterIds = many })
	wantCode(t, "RevealTrap with 51 ids", err, connect.CodeInvalidArgument)
	npc := m.createCharacter(s.campaign, charactersv1.CharacterKind_CHARACTER_KIND_MINION, "Goblin")
	_, err = reveal(m, func(r *mapsv1.RevealTrapRequest) { r.CharacterIds = []string{npc.GetId()} })
	wantCode(t, "RevealTrap to an NPC", err, connect.CodeNotFound)
	_, err = reveal(m, func(r *mapsv1.RevealTrapRequest) { r.CharacterIds = []string{newKey()} })
	wantCode(t, "RevealTrap to a character that is not there", err, connect.CodeNotFound)
	scene := s.point
	_, err = m.maps.RevealTrap(t.Context(), connect.NewRequest(&mapsv1.RevealTrapRequest{CampaignId: s.campaign, MapId: s.mapID, PointId: scene.GetId(), All: true}))
	wantCode(t, "RevealTrap on a scene", err, connect.CodeInvalidArgument)
	wantTrapFor("Ana", s.ana, false)

	// With no session: revealed to Pensantus, nothing written, nobody else sees it.
	got, err := reveal(m, func(r *mapsv1.RevealTrapRequest) { r.CharacterIds = []string{s.pens.GetId()} })
	if err != nil || len(got.GetTrapRevealedTo()) != 1 || got.GetTrapRevealedTo()[0].GetCharacterName() != "Pensantus" ||
		got.GetTrapRevealedTo()[0].GetHow() != mapsv1.TrapRevealHow_TRAP_REVEAL_HOW_MASTER || got.GetTrapRevealedTo()[0].GetAt() == nil {
		t.Fatalf("a trap revealed to Pensantus = %v, %v; want her in trap_revealed_to, by the master", got.GetTrapRevealedTo(), err)
	}
	wantTrapFor("Ana", s.ana, true)
	wantTrapFor("Caio", s.caio, false)
	if events := s.h.eventsOf("trap_revealed"); len(events) != 0 {
		t.Errorf("a reveal with no session wrote %v", events)
	}

	// With a session: one event, ids only; again to her changes nothing and writes
	// nothing; to both adds Toren alone.
	s.master.start(s.campaign)
	again, err := reveal(m, func(r *mapsv1.RevealTrapRequest) { r.CharacterIds = []string{s.pens.GetId()} })
	if err != nil || len(again.GetTrapRevealedTo()) != 1 || len(s.h.eventsOf("trap_revealed")) != 0 {
		t.Errorf("revealing again: %v, %v, events %v; want one reveal and no event", again.GetTrapRevealedTo(), err, s.h.eventsOf("trap_revealed"))
	}
	both, err := reveal(m, func(r *mapsv1.RevealTrapRequest) { r.CharacterIds = []string{s.pens.GetId(), s.other.GetId()} })
	if err != nil || len(both.GetTrapRevealedTo()) != 2 {
		t.Fatalf("revealed to both = %v, %v", both.GetTrapRevealedTo(), err)
	}
	events := s.h.eventsOf("trap_revealed")
	if len(events) != 1 || !strings.Contains(events[0], s.other.GetId()) || strings.Contains(events[0], s.pens.GetId()) || strings.Contains(events[0], "Toren") {
		t.Errorf("trap_revealed events = %v, want one naming Toren's id only", events)
	}
	wantTrapFor("Caio", s.caio, true)

	// To everyone: the point's revealed state, and one event with all.
	trap2 := s.newTrap("Outro fosso", 100, 100)
	trap = trap2
	if _, err := reveal(m, func(r *mapsv1.RevealTrapRequest) { r.All = true }); err != nil {
		t.Fatalf("RevealTrap(all) error = %v", err)
	}
	wantTrapFor("Ana", s.ana, true)
	wantTrapFor("Caio", s.caio, true)
	if events := s.h.eventsOf("trap_revealed"); len(events) != 2 || !strings.Contains(noSpaces(events[1]), `"all":true`) {
		t.Errorf("trap_revealed events = %v, want a second one with all", events)
	}
	if _, err := reveal(m, func(r *mapsv1.RevealTrapRequest) { r.All = true }); err != nil || len(s.h.eventsOf("trap_revealed")) != 2 {
		t.Errorf("revealing to all again: %v, events %v; want no new event", err, s.h.eventsOf("trap_revealed"))
	}
}

// MR-041, D8: found by one or more characters, visible to everyone with its
// contents, the session it was found in, the events (ids and the PO only),
// unmarking, and the converted lock.
func TestMR041_TreasureFound(t *testing.T) {
	t.Parallel()
	s := newScenes(t, false)
	m := s.master
	chest := s.newTreasure("Baú de moedas", "Duzentos e cinquenta PO em moedas antigas", 250, 6000, 6000)
	mark := func(u *user, ids ...string) (*mapsv1.MapPoint, error) {
		res, err := u.maps.MarkTreasureFound(t.Context(), connect.NewRequest(&mapsv1.MarkTreasureFoundRequest{
			CampaignId: s.campaign, MapId: s.mapID, PointId: chest.GetId(), CharacterIds: ids,
		}))
		if err != nil {
			return nil, err
		}
		return res.Msg.GetPoint(), nil
	}
	unmark := func(u *user) (*mapsv1.MapPoint, error) {
		res, err := u.maps.UnmarkTreasureFound(t.Context(), connect.NewRequest(&mapsv1.UnmarkTreasureFoundRequest{CampaignId: s.campaign, MapId: s.mapID, PointId: chest.GetId()}))
		if err != nil {
			return nil, err
		}
		return res.Msg.GetPoint(), nil
	}
	playerSees := func(u *user) *mapsv1.MapPoint {
		for _, p := range u.mustGetMap(s.campaign, s.mapID).GetPoints() {
			if p.GetId() == chest.GetId() {
				return p
			}
		}
		return nil
	}
	sessionOf := func() *string {
		t.Helper()
		var id *string
		if err := s.h.pool.QueryRow(t.Context(), `SELECT treasure_session_id::TEXT FROM map_points WHERE id = $1`, chest.GetId()).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	if playerSees(s.ana) != nil {
		t.Fatal("a hidden treasure reached a player")
	}

	// Refusals.
	_, err := mark(m)
	wantCode(t, "mark with nobody", err, connect.CodeInvalidArgument)
	npc := m.createCharacter(s.campaign, charactersv1.CharacterKind_CHARACTER_KIND_MINION, "Goblin")
	_, err = mark(m, npc.GetId())
	wantCode(t, "an NPC found it", err, connect.CodeNotFound)
	_, err = mark(s.ana, s.pens.GetId())
	wantCode(t, "a player marks a treasure found", err, connect.CodePermissionDenied)
	_, err = m.maps.MarkTreasureFound(t.Context(), connect.NewRequest(&mapsv1.MarkTreasureFoundRequest{CampaignId: s.campaign, MapId: s.mapID, PointId: s.point.GetId(), CharacterIds: []string{s.pens.GetId()}}))
	wantCode(t, "mark a scene found", err, connect.CodeInvalidArgument)
	_, err = m.maps.UnmarkTreasureFound(t.Context(), connect.NewRequest(&mapsv1.UnmarkTreasureFoundRequest{CampaignId: s.campaign, MapId: s.mapID, PointId: s.point.GetId()}))
	wantCode(t, "unmark a scene", err, connect.CodeInvalidArgument)

	// Found with no session open: visible to everyone with its contents and who
	// found it, remembered by no session, and no event.
	got, err := mark(m, s.pens.GetId(), s.other.GetId(), s.pens.GetId())
	if err != nil || got.GetTreasureFoundAt() == nil || len(got.GetTreasureFoundBy()) != 2 {
		t.Fatalf("a treasure found by two = %v, %v", got, err)
	}
	for _, u := range []*user{s.ana, s.caio} {
		p := playerSees(u)
		if p == nil || p.GetDescription() != "Duzentos e cinquenta PO em moedas antigas" || p.GetTreasureValuePo() != 250 || p.GetTreasureFoundAt() == nil ||
			len(p.GetTreasureFoundBy()) != 2 || p.GetTreasureFoundBy()[0].GetCharacterName() != "Pensantus" || p.GetTreasureConverted() {
			t.Errorf("a player's found treasure = %v, want its contents, value, finders and no converted flag", p)
		}
	}
	if sessionOf() != nil {
		t.Errorf("a treasure found with no session remembers session %v", *sessionOf())
	}
	if events := s.h.eventsOf("treasure_found", "treasure_unfound"); len(events) != 0 {
		t.Errorf("no session, yet events %v", events)
	}
	foundAt := got.GetTreasureFoundAt().AsTime()
	// Unmarked: hidden again, finders forgotten; unmarking twice changes nothing.
	un, err := unmark(m)
	if err != nil || un.GetTreasureFoundAt() != nil || len(un.GetTreasureFoundBy()) != 0 || playerSees(s.ana) != nil {
		t.Fatalf("an unmarked treasure = %v, %v; want hidden again", un, err)
	}
	if _, err := unmark(m); err != nil {
		t.Errorf("unmarking twice: %v", err)
	}

	// With a session: it remembers the session, and the events hold ids and the PO.
	session := s.master.start(s.campaign)
	if _, err := mark(m, s.pens.GetId()); err != nil {
		t.Fatal(err)
	}
	if id := sessionOf(); id == nil || *id != session.GetId() {
		t.Errorf("the treasure's session = %v, want %s", id, session.GetId())
	}
	// Marking again replaces the finders, keeps when and which session, and writes no new event.
	first := s.master.mustGetMap(s.campaign, s.mapID)
	firstAt := pointByID(first, chest.GetId()).GetTreasureFoundAt().AsTime()
	again, err := mark(m, s.other.GetId())
	if err != nil || len(again.GetTreasureFoundBy()) != 1 || again.GetTreasureFoundBy()[0].GetCharacterName() != "Toren" || !again.GetTreasureFoundAt().AsTime().Equal(firstAt) {
		t.Errorf("marking a found treasure again = %v, %v; want Toren alone and the first time", again.GetTreasureFoundBy(), err)
	}
	if !firstAt.After(foundAt) {
		t.Errorf("the second find (%v) is not after the first (%v)", firstAt, foundAt)
	}
	if _, err := unmark(m); err != nil {
		t.Fatal(err)
	}
	events := s.h.eventsOf("treasure_found", "treasure_unfound")
	if len(events) != 2 || !strings.HasPrefix(events[0], "treasure_found ") || !strings.HasPrefix(events[1], "treasure_unfound ") {
		t.Fatalf("events = %v, want treasure_found then treasure_unfound", events)
	}
	if !strings.Contains(events[0], s.pens.GetId()) || !strings.Contains(noSpaces(events[0]), `"value_po":250`) || !strings.Contains(noSpaces(events[1]), `"value_po":250`) {
		t.Errorf("the events %v lack the ids or the PO", events)
	}
	for _, e := range events {
		for _, f := range []string{"Baú", "moedas", "Pensantus", "Toren"} {
			if strings.Contains(e, f) {
				t.Errorf("the event %q carries %q: ids and the PO only", e, f)
			}
		}
	}

	// Converted (slice 9.11 sets the link inside the XP award's transaction): the
	// value and the found mark are frozen.
	if _, err := mark(m, s.pens.GetId()); err != nil {
		t.Fatal(err)
	}
	var award string
	if err := s.h.pool.QueryRow(t.Context(), `
		INSERT INTO xp_awards (campaign_id, created_at, mode, reason, gold, total_xp, idempotency_key)
		VALUES ($1, now(), 'gold', 'Voltar à cidade', 250, 250, gen_random_uuid()) RETURNING id::TEXT`, s.campaign).Scan(&award); err != nil {
		t.Fatalf("insert an XP award: %v", err)
	}
	if _, err := s.h.pool.Exec(t.Context(), `UPDATE map_points SET treasure_converted_award_id = $1 WHERE id = $2`, award, chest.GetId()); err != nil {
		t.Fatalf("convert the treasure: %v", err)
	}
	_, err = unmark(m)
	wantMapBlocked(t, "unmark a converted treasure", err, mapsv1.MapBlockedReason_MAP_BLOCKED_REASON_TREASURE_CONVERTED)
	_, err = mark(m, s.other.GetId())
	wantMapBlocked(t, "mark a converted treasure", err, mapsv1.MapBlockedReason_MAP_BLOCKED_REASON_TREASURE_CONVERTED)
	_, err = m.updatePoint(&mapsv1.UpdateMapPointRequest{CampaignId: s.campaign, MapId: s.mapID, PointId: chest.GetId(), TreasureValuePo: proto.Int32(500)})
	wantMapBlocked(t, "change a converted treasure's value", err, mapsv1.MapBlockedReason_MAP_BLOCKED_REASON_TREASURE_CONVERTED)
	_, err = m.updatePoint(&mapsv1.UpdateMapPointRequest{CampaignId: s.campaign, MapId: s.mapID, PointId: chest.GetId(), Kind: mapsv1.MapPointKind_MAP_POINT_KIND_SCENE.Enum()})
	wantMapBlocked(t, "change a converted treasure into a scene", err, mapsv1.MapBlockedReason_MAP_BLOCKED_REASON_TREASURE_CONVERTED)
	// The same value, and other edits, are fine; the master sees the flag, a player does not.
	if got, err := m.updatePoint(&mapsv1.UpdateMapPointRequest{CampaignId: s.campaign, MapId: s.mapID, PointId: chest.GetId(), TreasureValuePo: proto.Int32(250), Name: new("Baú vazio")}); err != nil || !got.GetTreasureConverted() {
		t.Errorf("an edit that keeps the value: %v, %v; want it allowed and the flag on", got, err)
	}
	if p := playerSees(s.ana); p == nil || p.GetTreasureConverted() {
		t.Errorf("a player's converted treasure = %v, want it seen without the flag", p)
	}
}

func pointByID(res *mapsv1.GetMapResponse, id string) *mapsv1.MapPoint {
	for _, p := range res.GetPoints() {
		if p.GetId() == id {
			return p
		}
	}
	return nil
}

// MR-036, D6: a player carries a light for their own character, the master for
// anyone; the key is checked against the presets; the master and the owner read it
// on the token, another player does not.
func TestMR036_CarriedLight(t *testing.T) {
	t.Parallel()
	s := newScenes(t, true)
	m := s.master
	npc := m.createCharacter(s.campaign, charactersv1.CharacterKind_CHARACTER_KIND_MINION, "Goblin")
	m.placeToken(s.campaign, s.mapID, s.pens.GetId(), 3000, 3000)
	m.placeToken(s.campaign, s.mapID, s.other.GetId(), 4000, 4000)
	m.placeToken(s.campaign, s.mapID, npc.GetId(), 5000, 5000)
	m.setTokenHidden(s.campaign, s.mapID, npc.GetId(), false)
	carry := func(u *user, character, key string) (*mapsv1.MapToken, error) {
		res, err := u.maps.SetCarriedLight(t.Context(), connect.NewRequest(&mapsv1.SetCarriedLightRequest{
			CampaignId: s.campaign, MapId: s.mapID, CharacterId: character, LightKey: key,
		}))
		if err != nil {
			return nil, err
		}
		return res.Msg.GetToken(), nil
	}
	lightOn := func(u *user, character string) string {
		for _, tk := range u.mustGetMap(s.campaign, s.mapID).GetTokens() {
			if tk.GetCharacterId() == character {
				return tk.GetCarriedLight()
			}
		}
		return "no token"
	}
	anaWatch, caioWatch := s.ana.watch(s.campaign), s.caio.watch(s.campaign)

	// Her own character: yes. The answer carries the light.
	got, err := carry(s.ana, s.pens.GetId(), "light:torch")
	if err != nil || got.GetCarriedLight() != "light:torch" || !got.GetMine() {
		t.Fatalf("Ana's torch = %v, %v", got, err)
	}
	// Read by the master and by her, not by Toren's player.
	if lightOn(m, s.pens.GetId()) != "light:torch" || lightOn(s.ana, s.pens.GetId()) != "light:torch" {
		t.Error("the master or Ana do not read Pensantus's torch")
	}
	if got := lightOn(s.caio, s.pens.GetId()); got != "" {
		t.Errorf("Toren's player reads Pensantus's carried light %q, want none", got)
	}
	// Another player's character, and an NPC: no.
	_, err = carry(s.ana, s.other.GetId(), "light:torch")
	wantCode(t, "Ana carries a light for Toren", err, connect.CodePermissionDenied)
	_, err = carry(s.ana, npc.GetId(), "light:torch")
	wantCode(t, "Ana carries a light for an NPC", err, connect.CodeNotFound)
	if lightOn(m, s.other.GetId()) != "" || lightOn(m, npc.GetId()) != "" {
		t.Error("a refused call set a light")
	}
	// The master: anyone, NPCs included.
	for _, id := range []string{s.other.GetId(), npc.GetId()} {
		if got, err := carry(m, id, "light:hooded-lantern"); err != nil || got.GetCarriedLight() != "light:hooded-lantern" {
			t.Errorf("the master's lantern for %s = %v, %v", id, got, err)
		}
	}
	// A key must be a light preset; empty puts it out.
	for _, key := range []string{"light:nope", "trap:simple-pit", "torch", "light:"} {
		_, err = carry(s.ana, s.pens.GetId(), key)
		wantCode(t, "the key "+key, err, connect.CodeInvalidArgument)
	}
	if lightOn(s.ana, s.pens.GetId()) != "light:torch" {
		t.Error("a refused key changed the light")
	}
	if got, err := carry(s.ana, s.pens.GetId(), ""); err != nil || got.GetCarriedLight() != "" || lightOn(m, s.pens.GetId()) != "" {
		t.Errorf("putting the torch out = %v, %v", got, err)
	}
	// A character with no token on the map has no light to carry.
	other := m.createMap(s.campaign, "Outro", m.newImage(s.campaign))
	_, err = m.maps.SetCarriedLight(t.Context(), connect.NewRequest(&mapsv1.SetCarriedLightRequest{CampaignId: s.campaign, MapId: other.GetId(), CharacterId: s.pens.GetId(), LightKey: "light:torch"}))
	wantCode(t, "a light with no token", err, connect.CodeNotFound)
	// A hidden map is not found to a player.
	m.placeToken(s.campaign, other.GetId(), s.pens.GetId(), 100, 100)
	_, err = s.ana.maps.SetCarriedLight(t.Context(), connect.NewRequest(&mapsv1.SetCarriedLightRequest{CampaignId: s.campaign, MapId: other.GetId(), CharacterId: s.pens.GetId(), LightKey: "light:torch"}))
	wantCode(t, "a player's light on a hidden map", err, connect.CodeNotFound)

	// The streams: Ana (the owner) heard of her torch and of the master's change;
	// Toren's player heard only of what the master set for his own character.
	probeMap := m.createMap(s.campaign, "Sonda", m.newImage(s.campaign)).GetId()
	s.probe(probeMap)
	anaEvents, caioEvents := anaWatch.drain(probeMap), caioWatch.drain(probeMap)
	if len(anaEvents) != 2 {
		t.Errorf("Ana's stream = %v, want hints for her torch and for putting it out", anaEvents)
	}
	if len(caioEvents) != 1 {
		t.Errorf("Toren's player's stream = %v, want one hint: the master's lantern for his own character", caioEvents)
	}
}

// The master's change to a trap locks the point and then the session (the event it
// records); the play module's trigger locks the session and then the point. When a
// move fires a trap while the master reveals it, the two wait for each other:
// CockroachDB aborts one (40001) and the retry of its transaction goes on, so the
// master's call still succeeds and the trap is revealed.
func TestMR035_ARevealCrossingATriggerInPlayTakesTurns(t *testing.T) {
	t.Parallel()
	s := newScenes(t, true)
	trap := s.newTrap("Fosso", 1000, 1000)
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	probe := dbtest.SideConnection(t, s.h.pool)
	// The sessions of the two sides, to tell their statements from other tests' on the same server.
	var masterSession, playSession string
	if err := s.h.pool.QueryRow(ctx, `SHOW session_id`).Scan(&masterSession); err != nil {
		t.Fatal(err)
	}
	// waiting counts the statements of the two sides asking for the trap's lock right now.
	waiting := func() int {
		var n int
		err := probe.QueryRow(ctx, `SELECT count(*) FROM [SHOW CLUSTER STATEMENTS] WHERE session_id IN ($1, $2) AND query LIKE '%FROM map_points%FOR UPDATE%'`, masterSession, playSession).Scan(&n)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	waitFor := func(what string, n int) {
		t.Helper()
		for waiting() < n {
			if ctx.Err() != nil {
				t.Fatalf("%s never waited for the trap", what)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	// A transaction holds the point, so the master's call stops right at it.
	holder, err := dbtest.SideConnection(t, s.h.pool).Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Rollback(ctx) //nolint:errcheck // rolled back unless it was committed
	if _, err := holder.Exec(ctx, `SELECT id FROM map_points WHERE id = $1 FOR UPDATE`, trap.GetId()); err != nil {
		t.Fatal(err)
	}
	revealed := make(chan error, 1)
	go func() {
		_, err := s.master.maps.RevealTrap(ctx, connect.NewRequest(&mapsv1.RevealTrapRequest{CampaignId: s.campaign, MapId: s.mapID, PointId: trap.GetId(), All: true}))
		revealed <- err
	}()
	waitFor("the master's call", 1)
	// The play module's transaction: the session row first, as every change in play
	// does, then the trap the move fires. It queues behind the master's call.
	play, err := dbtest.SideConnection(t, s.h.pool).Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer play.Rollback(ctx) //nolint:errcheck // rolled back unless it was committed
	if err := play.QueryRow(ctx, `SHOW session_id`).Scan(&playSession); err != nil {
		t.Fatal(err)
	}
	if _, err := play.Exec(ctx, `SELECT id FROM game_sessions WHERE campaign_id = $1 AND ended_at IS NULL FOR UPDATE`, s.campaign); err != nil {
		t.Fatal(err)
	}
	fired := make(chan error, 1)
	go func() {
		_, err := s.h.svc.TriggerTrap(ctx, play, s.campaign, s.mapID, trap.GetId(), time.Now())
		fired <- err
	}()
	waitFor("the move", 2)
	// The master's call takes the point first and then waits for the session, which the
	// move holds while it waits for the point: one of them is aborted.
	if err := holder.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	err = <-fired
	if err == nil {
		err = play.Commit(ctx)
	}
	var pg *pgconn.PgError
	if err != nil && (!errors.As(err, &pg) || pg.Code != "40001") {
		t.Fatalf("the move ended with %v, want success or a retryable abort", err)
	}
	_ = play.Rollback(ctx)
	if err := <-revealed; err != nil {
		t.Errorf("RevealTrap() crossing a trigger = %v, want success", err)
	}
}
