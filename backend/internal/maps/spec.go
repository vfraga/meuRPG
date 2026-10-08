package maps

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/maps/mapsdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/names"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// What the three new kinds of point carry (Etapa 9): a trap's spec (MR-035), a
// treasure's value (MR-041) and a light's source (MR-036). The master writes
// them as API messages; this file checks them against the rules content and
// turns them into what the table keeps, and back.
//
// A trap is kept as JSON in the API's own shape (TrapSpec without its state,
// which has a column of its own), so the stored copy and the message are the
// same thing and nothing translates between two models. The numbers are the
// trap's own copy: a preset only fills the form (ContentService
// .ListTrapPresets), so a change to a preset never changes a trap on a map.

// The limits a trap and a light keep.
const (
	maxTrapParts       = 4 // damage parts, conditions: a handful
	maxConditionLength = 60
	maxTreasurePO      = 1_000_000
	maxLightFt         = 120
	maxFlatDamage      = 100
)

// The database's trap states (map_points.trap_state) and the API's.
var (
	trapStateToDB = map[mapsv1.TrapState]string{
		mapsv1.TrapState_TRAP_STATE_ARMED:     "armed",
		mapsv1.TrapState_TRAP_STATE_TRIGGERED: "triggered",
		mapsv1.TrapState_TRAP_STATE_DISARMED:  "disarmed",
	}
	trapStateFromDB = map[string]mapsv1.TrapState{
		"armed":     mapsv1.TrapState_TRAP_STATE_ARMED,
		"triggered": mapsv1.TrapState_TRAP_STATE_TRIGGERED,
		"disarmed":  mapsv1.TrapState_TRAP_STATE_DISARMED,
	}
	abilityKeys = map[rulesv1.Ability]bool{
		rulesv1.Ability_ABILITY_STRENGTH: true, rulesv1.Ability_ABILITY_DEXTERITY: true, rulesv1.Ability_ABILITY_CONSTITUTION: true,
		rulesv1.Ability_ABILITY_INTELLIGENCE: true, rulesv1.Ability_ABILITY_WISDOM: true, rulesv1.Ability_ABILITY_CHARISMA: true,
	}
)

// stateTriggered is the stored word of a trap that fired: it makes the trap
// visible to everyone.
const stateTriggered = "triggered"

// badSpec is an `invalid_argument` that names the field and the rule, never a
// value.
func badSpec(format string, a ...any) error {
	return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf(format, a...))
}

// trapData is a checked trap, ready for the table.
type trapData struct {
	json  []byte // TrapSpec without its state
	state string
}

// cleanTrap checks a TrapSpec and returns its canonical copy: the same message
// with the output-only names left out and the dice written one way. current is
// the state the trap has now (nil for a new one).
func (s *Service) cleanTrap(in *mapsv1.TrapSpec, current *string) (trapData, error) {
	if in == nil {
		return trapData{}, badSpec("trap is required for a TRAP point")
	}
	if in.GetPresetKey() != "" {
		if _, ok := s.rules.TrapPreset(in.GetPresetKey()); !ok {
			return trapData{}, badSpec("trap.preset_key is not a trap preset")
		}
	}
	switch {
	case in.GetNoticeDc() < 0 || in.GetNoticeDc() > 30:
		return trapData{}, badSpec("trap.notice_dc must be 0 (none) or 1 to 30")
	case in.GetFindDc() < 1 || in.GetFindDc() > 30:
		return trapData{}, badSpec("trap.find_dc must be 1 to 30")
	case in.GetAreaSize() < 1 || in.GetAreaSize() > 4:
		return trapData{}, badSpec("trap.area_size must be 1 to 4")
	case in.GetTrigger() != rulesv1.TrapTrigger_TRAP_TRIGGER_ENTER && in.GetTrigger() != rulesv1.TrapTrigger_TRAP_TRIGGER_MANUAL:
		return trapData{}, badSpec("trap.trigger is required")
	}
	// An unspecified state keeps the current one (a new trap starts armed): an
	// edit that does not mention it never re-arms a trap.
	stateDB := "armed"
	if current != nil {
		stateDB = *current
	}
	if in.GetState() != mapsv1.TrapState_TRAP_STATE_UNSPECIFIED {
		var ok bool
		if stateDB, ok = trapStateToDB[in.GetState()]; !ok {
			return trapData{}, badSpec("trap.state is not a trap state")
		}
	}
	effect, err := s.cleanEffect(in.GetEffect())
	if err != nil {
		return trapData{}, err
	}
	canonical := &mapsv1.TrapSpec{
		PresetKey: in.GetPresetKey(), NoticeDc: in.GetNoticeDc(), FindDc: in.GetFindDc(), AreaSize: in.GetAreaSize(),
		Trigger: in.GetTrigger(), Effect: effect,
	}
	b, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(canonical)
	if err != nil {
		return trapData{}, fmt.Errorf("encode the trap: %w", err)
	}
	return trapData{json: b, state: stateDB}, nil
}

// cleanEffect checks a trap's effect, in the rules the loader of effects/traps.json
// applies to the presets: DCs 1 to 30, dice of d4 to d12, a failed save that
// does something, half damage only where there is damage to halve.
func (s *Service) cleanEffect(in *rulesv1.TrapEffect) (*rulesv1.TrapEffect, error) {
	out := &rulesv1.TrapEffect{Targets: in.GetTargets()}
	switch out.GetTargets() {
	case rulesv1.TrapTargets_TRAP_TARGETS_UNSPECIFIED:
		out.Targets = rulesv1.TrapTargets_TRAP_TARGETS_AREA
	case rulesv1.TrapTargets_TRAP_TARGETS_AREA, rulesv1.TrapTargets_TRAP_TARGETS_MANUAL:
	default:
		return nil, badSpec("trap.effect.targets is not a valid value")
	}
	if a := in.GetAttack(); a != nil {
		if a.GetBonus() < 0 || a.GetBonus() > 20 || a.GetCount() < 1 || a.GetCount() > 10 {
			return nil, badSpec("trap.effect.attack needs a bonus of 0 to 20 and 1 to 10 attacks")
		}
		d, err := s.cleanDamage("trap.effect.attack.damage", a.GetDamage())
		if err != nil {
			return nil, err
		}
		out.Attack = &rulesv1.TrapAttack{Bonus: a.GetBonus(), Count: a.GetCount(), Damage: d}
	}
	var err error
	if out.Damage, err = s.cleanDamages("trap.effect.damage", in.GetDamage()); err != nil {
		return nil, err
	}
	if len(in.GetConditions()) > maxTrapParts {
		return nil, badSpec("trap.effect.conditions has at most %d conditions", maxTrapParts)
	}
	for i, c := range in.GetConditions() {
		cond, err := s.cleanCondition(fmt.Sprintf("trap.effect.conditions[%d]", i), c)
		if err != nil {
			return nil, err
		}
		out.Conditions = append(out.Conditions, cond)
	}
	if sv := in.GetSave(); sv != nil {
		if out.Save, err = s.cleanSave(sv, out.GetAttack() != nil); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Service) cleanSave(sv *rulesv1.TrapSaveEffect, hasAttack bool) (*rulesv1.TrapSaveEffect, error) {
	switch {
	case !abilityKeys[sv.GetAbility()]:
		return nil, badSpec("trap.effect.save.ability is required")
	case sv.GetDc() < 1 || sv.GetDc() > 30:
		return nil, badSpec("trap.effect.save.dc must be 1 to 30")
	case sv.GetAppliesTo() != rulesv1.TrapSaveApplies_TRAP_SAVE_APPLIES_CAUGHT && sv.GetAppliesTo() != rulesv1.TrapSaveApplies_TRAP_SAVE_APPLIES_HIT:
		return nil, badSpec("trap.effect.save.applies_to is required")
	case sv.GetAppliesTo() == rulesv1.TrapSaveApplies_TRAP_SAVE_APPLIES_HIT && !hasAttack:
		return nil, badSpec("trap.effect.save.applies_to HIT needs an attack")
	case sv.GetOnPass() != rulesv1.TrapPassOutcome_TRAP_PASS_OUTCOME_HALF && sv.GetOnPass() != rulesv1.TrapPassOutcome_TRAP_PASS_OUTCOME_NONE:
		return nil, badSpec("trap.effect.save.on_pass is required")
	}
	fail, err := s.cleanDamages("trap.effect.save.on_fail.damage", sv.GetOnFail().GetDamage())
	if err != nil {
		return nil, err
	}
	onFail := &rulesv1.TrapOnFail{Damage: fail}
	if c := sv.GetOnFail().GetCondition(); c != nil {
		if onFail.Condition, err = s.cleanCondition("trap.effect.save.on_fail.condition", c); err != nil {
			return nil, err
		}
	}
	switch {
	case len(fail) == 0 && onFail.Condition == nil:
		return nil, badSpec("trap.effect.save.on_fail must do damage or give a condition")
	case sv.GetOnPass() == rulesv1.TrapPassOutcome_TRAP_PASS_OUTCOME_HALF && len(fail) == 0:
		return nil, badSpec("trap.effect.save.on_pass HALF needs damage on a failure")
	}
	return &rulesv1.TrapSaveEffect{
		Ability: sv.GetAbility(), Dc: sv.GetDc(), AppliesTo: sv.GetAppliesTo(), OnFail: onFail, OnPass: sv.GetOnPass(),
	}, nil
}

func (s *Service) cleanDamages(where string, in []*rulesv1.TrapDamage) ([]*rulesv1.TrapDamage, error) {
	if len(in) > maxTrapParts {
		return nil, badSpec("%s has at most %d parts", where, maxTrapParts)
	}
	var out []*rulesv1.TrapDamage
	for i, d := range in {
		clean, err := s.cleanDamage(fmt.Sprintf("%s[%d]", where, i), d)
		if err != nil {
			return nil, err
		}
		out = append(out, clean)
	}
	return out, nil
}

// cleanDamage checks one damage part: 1 to 20 dice of d4, d6, d8, d10 or d12, or
// a flat number from 1 to 100, and a damage type of the rules.
func (s *Service) cleanDamage(where string, in *rulesv1.TrapDamage) (*rulesv1.TrapDamage, error) {
	if in == nil {
		return nil, badSpec("%s is required", where)
	}
	d, ok := rules.ParseDice(in.GetDice())
	switch {
	case !ok || d.AddsModifier:
		return nil, badSpec("%s.dice is not dice such as \"2d6\" or a flat number", where)
	case d.Count == 0 && (d.Bonus < 1 || d.Bonus > maxFlatDamage):
		return nil, badSpec("%s.dice: a flat number is 1 to %d", where, maxFlatDamage)
	case d.Count > 0 && (d.Count > 20 || d.Bonus != 0 || !validSides(d.Sides)):
		return nil, badSpec("%s.dice is 1 to 20 dice of d4, d6, d8, d10 or d12", where)
	}
	if !strings.HasPrefix(in.GetDamageTypeKey(), "damage-type:") || s.rules.NamePT(in.GetDamageTypeKey()) == "" {
		return nil, badSpec("%s.damage_type_key is not a damage type", where)
	}
	text := strconv.Itoa(d.Bonus)
	if d.Count > 0 {
		text = strconv.Itoa(d.Count) + "d" + strconv.Itoa(d.Sides)
	}
	return &rulesv1.TrapDamage{Dice: text, DamageTypeKey: in.GetDamageTypeKey()}, nil
}

func validSides(n int) bool { return n == 4 || n == 6 || n == 8 || n == 10 || n == 12 }

func (s *Service) cleanCondition(where string, in *rulesv1.TrapCondition) (*rulesv1.TrapCondition, error) {
	if !strings.HasPrefix(in.GetConditionKey(), "condition:") || s.rules.NamePT(in.GetConditionKey()) == "" {
		return nil, badSpec("%s.condition_key is not a condition", where)
	}
	duration := strings.TrimSpace(in.GetDurationPt())
	if duration != "" {
		var err error
		if duration, err = names.Clean(duration, maxConditionLength); err != nil {
			return nil, badSpec("%s.duration_pt %v", where, err)
		}
	}
	return &rulesv1.TrapCondition{ConditionKey: in.GetConditionKey(), DurationPt: duration}, nil
}

// trapOf reads a point's stored trap back as the API message, with its state
// and the Portuguese names filled in. A point that is not a trap, or whose JSON
// cannot be read (never: the API wrote it), has none.
func (s *Service) trapOf(p mapsdb.MapPoint) *mapsv1.TrapSpec {
	if p.Kind != kindToDB[mapsv1.MapPointKind_MAP_POINT_KIND_TRAP] || p.Trap == nil {
		return nil
	}
	spec := &mapsv1.TrapSpec{}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(p.Trap, spec); err != nil {
		s.logger.Error("maps: cannot read a stored trap", "error", err)
		return nil
	}
	if p.TrapState != nil {
		spec.State = trapStateFromDB[*p.TrapState]
	}
	name := func(key string) string { return s.rules.NamePT(key) }
	for _, d := range allDamages(spec.GetEffect()) {
		d.DamageTypePt = name(d.GetDamageTypeKey())
	}
	for _, c := range allConditions(spec.GetEffect()) {
		c.ConditionPt = name(c.GetConditionKey())
	}
	return spec
}

func allDamages(e *rulesv1.TrapEffect) []*rulesv1.TrapDamage {
	var out []*rulesv1.TrapDamage
	if e.GetAttack() != nil {
		out = append(out, e.GetAttack().GetDamage())
	}
	out = append(out, e.GetDamage()...)
	out = append(out, e.GetSave().GetOnFail().GetDamage()...)
	return out
}

func allConditions(e *rulesv1.TrapEffect) []*rulesv1.TrapCondition {
	out := append([]*rulesv1.TrapCondition(nil), e.GetConditions()...)
	if c := e.GetSave().GetOnFail().GetCondition(); c != nil {
		out = append(out, c)
	}
	return out
}

// publicTrap is what a player gets of a trap they see: where it reaches and
// where it is, never its preset, DCs, trigger or effect (RN-10).
func publicTrap(spec *mapsv1.TrapSpec) *mapsv1.TrapSpec {
	if spec == nil {
		return nil
	}
	return &mapsv1.TrapSpec{AreaSize: spec.GetAreaSize(), State: spec.GetState()}
}

// cleanTreasureValue checks a treasure's worth, in gold pieces.
func cleanTreasureValue(po int32) (int32, error) {
	if po < 0 || po > maxTreasurePO {
		return 0, badSpec("treasure_value_po must be 0 to %d", maxTreasurePO)
	}
	return po, nil
}

// lightData is a checked light, ready for the table.
type lightData struct {
	preset         *string
	bright, dimmed int32
}

// cleanLight checks a light's radii, in feet. A preset's key with both radii at
// 0 takes the preset's radii; otherwise the radii are the master's own copy.
func (s *Service) cleanLight(in *mapsv1.LightSpec) (lightData, error) {
	if in == nil {
		return lightData{}, badSpec("light is required for a LIGHT point")
	}
	out := lightData{bright: in.GetBrightFt(), dimmed: in.GetDimFt()}
	if key := in.GetPresetKey(); key != "" {
		preset, ok := s.rules.LightPreset(key)
		if !ok {
			return lightData{}, badSpec("light.preset_key is not a light preset")
		}
		out.preset = &key
		if out.bright == 0 && out.dimmed == 0 {
			out.bright, out.dimmed = presetFt(preset.BrightFt), presetFt(preset.DimFt)
		}
	}
	for _, ft := range []int32{out.bright, out.dimmed} {
		if ft < 0 || ft > maxLightFt || ft%5 != 0 {
			return lightData{}, badSpec("light radii are 0 to %d feet, in whole squares of 5", maxLightFt)
		}
	}
	if out.bright+out.dimmed == 0 {
		return lightData{}, badSpec("light needs a radius: bright_ft and dim_ft cannot both be 0")
	}
	return out, nil
}

func presetFt(n int) int32 {
	return int32(min(max(n, 0), maxLightFt))
}

// lightOf reads a point's light back as the API message.
func lightOf(p mapsdb.MapPoint) *mapsv1.LightSpec {
	if p.Kind != kindToDB[mapsv1.MapPointKind_MAP_POINT_KIND_LIGHT] || p.LightBrightFt == nil || p.LightDimFt == nil {
		return nil
	}
	spec := &mapsv1.LightSpec{BrightFt: *p.LightBrightFt, DimFt: *p.LightDimFt}
	if p.LightPreset != nil {
		spec.PresetKey = *p.LightPreset
	}
	return spec
}

// pointSpec is what a new or changed point carries beside its name and place:
// the columns of the kinds that have data of their own. Only the fields of the
// point's own kind are ever set.
type pointSpec struct {
	trap     []byte
	trapSt   *string // 'triggered' makes the trap public for good: see triggeredAt
	value    *int32
	light    lightData
	hasLight bool
}

// errNotThatKind says a request carries data of a kind the point is not.
func errNotThatKind(field, kind string) error {
	return badSpec("%s is only for a %s point", field, kind)
}

// specFor checks the data a request carries for a point of this kind and
// returns it. A kind needs its own data (a trap needs its spec) and refuses
// another kind's. treasure is the value as sent; for a treasure that has none
// the value is 0.
func (s *Service) specFor(kind string, trap *mapsv1.TrapSpec, treasure *int32, light *mapsv1.LightSpec) (pointSpec, error) {
	var out pointSpec
	switch {
	case kind == kindToDB[mapsv1.MapPointKind_MAP_POINT_KIND_TRAP]:
		t, err := s.cleanTrap(trap, nil)
		if err != nil {
			return out, err
		}
		out.trap, out.trapSt = t.json, &t.state
	case trap != nil:
		return out, errNotThatKind("trap", "TRAP")
	}
	switch {
	case kind == kindToDB[mapsv1.MapPointKind_MAP_POINT_KIND_TREASURE]:
		v := int32(0)
		if treasure != nil {
			var err error
			if v, err = cleanTreasureValue(*treasure); err != nil {
				return out, err
			}
		}
		out.value = &v
	case treasure != nil:
		return out, errNotThatKind("treasure_value_po", "TREASURE")
	}
	switch {
	case kind == kindToDB[mapsv1.MapPointKind_MAP_POINT_KIND_LIGHT]:
		l, err := s.cleanLight(light)
		if err != nil {
			return out, err
		}
		out.light, out.hasLight = l, true
	case light != nil:
		return out, errNotThatKind("light", "LIGHT")
	}
	return out, nil
}

// lightBright and lightDim are the light's radii as the table keeps them: NULL
// for a point that is not a light.
func (ps pointSpec) lightBright() *int32 {
	if !ps.hasLight {
		return nil
	}
	return &ps.light.bright
}

func (ps pointSpec) lightDim() *int32 {
	if !ps.hasLight {
		return nil
	}
	return &ps.light.dimmed
}

// triggeredAt is the time a new trap that is born triggered fired: now.
func (ps pointSpec) triggeredAt(now func() time.Time) *time.Time {
	if ps.trapSt == nil || *ps.trapSt != stateTriggered {
		return nil
	}
	t := now()
	return &t
}

func errTreasureFound() error {
	return errMapBlocked(mapsv1.MapBlockedReason_MAP_BLOCKED_REASON_TREASURE_FOUND,
		"the treasure was found: unmark it before deleting it or changing its kind")
}

func errTreasureConverted() error {
	return errMapBlocked(mapsv1.MapBlockedReason_MAP_BLOCKED_REASON_TREASURE_CONVERTED,
		"the treasure was turned into XP: its value and its found mark cannot change")
}
