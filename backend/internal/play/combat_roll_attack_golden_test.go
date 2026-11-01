package play

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dbtest"
)

// These tests freeze what RollAttack does today, scenario by scenario: every refusal with its code and
// message, and for each attack that goes through the answer, the events it writes (in order, the
// combatants named by label), the combatants' economy and the damage it opens. They are the net under the
// split of rollAttack into phases. testdata/roll_attack.golden holds what each scenario reads; run with
// UPDATE_GOLDEN=1 to write the file again, and read the diff.

const rollAttackGolden = "testdata/roll_attack.golden"

// jsonOfMessage is a message as sorted JSON, with the ids replaced by labels.
func jsonOfMessage(m proto.Message, names map[string]string, skip ...string) string {
	if m == nil || !m.ProtoReflect().IsValid() {
		return ""
	}
	c := proto.Clone(m)
	for _, f := range skip {
		if fd := c.ProtoReflect().Descriptor().Fields().ByName(protoreflect.Name(f)); fd != nil {
			c.ProtoReflect().Clear(fd)
		}
	}
	raw, err := protojson.Marshal(c)
	if err != nil {
		return err.Error()
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return err.Error()
	}
	out, _ := json.Marshal(v)
	return uuidPattern.ReplaceAllStringFunc(string(out), func(id string) string {
		if l, ok := names[id]; ok {
			return l
		}
		return "?"
	})
}

// rowsOf is the rows of a query as text, one line per row, sorted.
func (a *armed) rowsOf(t *testing.T, query string, args ...any) []string {
	t.Helper()
	rows, err := a.h.pool.Query(t.Context(), query, args...)
	if err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			t.Fatalf("read a row: %v", err)
		}
		parts := make([]string, len(vals))
		for i, v := range vals {
			parts[i] = dbtest.SortJSONKeys(fmt.Sprint(v))
		}
		out = append(out, strings.Join(parts, " | "))
	}
	sort.Strings(out)
	return out
}

// attackReport is what a scenario read after its attack: the answer or the refusal, the events written since
// `after`, the combatants' economy and the damage left open.
func (a *armed) attackReport(t *testing.T, e *playv1.Encounter, after int, res *playv1.RollAttackResponse, err error) string {
	t.Helper()
	names := labelsOf(e)
	var lines []string
	if err != nil {
		lines = append(lines, fmt.Sprintf("refused %v: %v", connect.CodeOf(err), err.Error()[strings.Index(err.Error(), ": ")+2:]))
	} else {
		lines = append(lines, "roll "+jsonOfMessage(res.GetRoll(), names), "pending "+jsonOfMessage(res.GetPendingDamage(), names, "id"))
	}
	for _, l := range a.eventTrace(t, e, after) {
		lines = append(lines, "event "+l)
	}
	var state []string
	for _, r := range a.rowsOf(t, `SELECT label, action_used, bonus_action_used, reaction_used, attacks_made, COALESCE(action_attack_key, '-'), hidden FROM combatants ORDER BY label`) {
		state = append(state, "combatant "+r)
	}
	for _, r := range a.rowsOf(t, `SELECT attack_key, status, critical, dice_count, dice_sides, dice_bonus, damage_type, COALESCE(attack_total, 0), COALESCE(attack_armor_class, 0) FROM pending_damages`) {
		state = append(state, "damage "+r)
	}
	lines = append(lines, a.printedState(t, err != nil, state)...)
	return strings.Join(lines, "\n")
}

// rawAttack calls RollAttack as u with a request that edit builds from Toren's battleaxe attack on the goblin.
func (a *armed) rawAttack(t *testing.T, u *user, e *playv1.Encounter, edit func(*playv1.RollAttackRequest)) (*playv1.RollAttackResponse, error) {
	t.Helper()
	req := &playv1.RollAttackRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), AttackerId: a.id(t, "Toren"), AttackKey: battleaxe, TargetId: a.id(t, "Goblin"),
		IdempotencyKey: newKey(), Roll: &playv1.RollAttackRequest_D20Face{D20Face: 15},
	}
	edit(req)
	res, err := u.combat.RollAttack(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func asToren(a *armed) *user  { return a.caio }
func asMaster(a *armed) *user { return a.master }

// attackThrough is a scenario of an attack that goes through, on the usual fight: as plays it with edit.
func attackThrough(name string, as func(a *armed) *user, edit func(*playv1.RollAttackRequest)) goldenScenario {
	return goldenScenario{name: name, run: func(t *testing.T) string {
		t.Helper()
		a := newArmed(t)
		e := a.threeAndAGoblin(t)
		before := a.markCall(t)
		res, err := a.rawAttack(t, as(a), e, edit)
		return a.attackReport(t, e, before, res, err)
	}}
}

func typedD20(face int32) func(*playv1.RollAttackRequest) {
	return func(r *playv1.RollAttackRequest) { r.Roll = &playv1.RollAttackRequest_D20Face{D20Face: face} }
}

// attackOn is a scenario that plays its own fight: play returns the combat, what it wrote before the attack
// and the attack's answer.
func attackOn(name string, play func(t *testing.T) (*armed, *playv1.Encounter, int, *playv1.RollAttackResponse, error)) goldenScenario {
	return goldenScenario{name: name, run: func(t *testing.T) string {
		t.Helper()
		a, e, before, res, err := play(t)
		return a.attackReport(t, e, before, res, err)
	}}
}

// customAttacks are the scenarios that need more than the usual fight.
func customAttacks() []goldenScenario {
	return []goldenScenario{
		attackOn("an NPC attacks as the master", func(t *testing.T) (*armed, *playv1.Encounter, int, *playv1.RollAttackResponse, error) {
			t.Helper()
			a := newArmed(t)
			e := a.threeAndAGoblin(t)
			e = a.passTo(t, e, "Goblin")
			before := a.markCall(t)
			res, err := a.attack(t, a.master, e, "Goblin", sword, "Toren", d20(14))
			return a, e, before, res, err
		}),
		attackOn("an attack cantrip with the pair of dice", func(t *testing.T) (*armed, *playv1.Encounter, int, *playv1.RollAttackResponse, error) {
			t.Helper()
			a := newArmed(t)
			e := a.threeAndAGoblin(t)
			e = a.passTo(t, e, "Pensantus")
			before := a.markCall(t)
			res, err := a.attack(t, a.ana, e, "Pensantus", fireBolt, "Goblin", func(r *playv1.RollAttackRequest) { r.Roll, r.D20Faces = nil, []int32{12, 7} })
			return a, e, before, res, err
		}),
		attackOn("the extra action for a cantrip", func(t *testing.T) (*armed, *playv1.Encounter, int, *playv1.RollAttackResponse, error) {
			t.Helper()
			a := newArmed(t)
			e := a.threeAndAGoblin(t)
			e = a.passTo(t, e, "Pensantus")
			before := a.markCall(t)
			res, err := a.attack(t, a.ana, e, "Pensantus", fireBolt, "Goblin", func(r *playv1.RollAttackRequest) {
				r.Roll, r.UseExtraAction = &playv1.RollAttackRequest_D20Face{D20Face: 9}, true
			})
			return a, e, before, res, err
		}),
		attackOn("an attack out of reach", func(t *testing.T) (*armed, *playv1.Encounter, int, *playv1.RollAttackResponse, error) {
			t.Helper()
			a := newArmed(t)
			e := a.threeAndAGoblin(t)
			before := a.markCall(t)
			res, err := a.attack(t, a.caio, e, "Toren", battleaxe, "Pensantus", d20(15))
			return a, e, before, res, err
		}),
		attackOn("an attack by someone who is not on the map", func(t *testing.T) (*armed, *playv1.Encounter, int, *playv1.RollAttackResponse, error) {
			t.Helper()
			a := newArmed(t)
			e := a.start(t, plan{
				npcs:     []*playv1.Participant{{CharacterId: a.goblin.GetId()}},
				npcRolls: []int{3},
				players:  map[string]int32{"Toren": 18, "Pensantus": 10, "Brisa": 1},
				reveal:   []string{"Goblin"},
				at:       map[string][2]int32{"Goblin": {4, 3}},
			})
			before := a.markCall(t)
			res, err := a.attack(t, a.caio, e, "Toren", battleaxe, "Goblin", d20(15))
			return a, e, before, res, err
		}),
		attackOn("an attack in a combat without a map", func(t *testing.T) (*armed, *playv1.Encounter, int, *playv1.RollAttackResponse, error) {
			t.Helper()
			a := newArmed(t)
			e := a.start(t, plan{
				npcs:     []*playv1.Participant{{CharacterId: a.goblin.GetId()}},
				npcRolls: []int{3},
				players:  map[string]int32{"Toren": 18, "Pensantus": 10, "Brisa": 1},
				reveal:   []string{"Goblin"},
				theatre:  true,
			})
			before := a.markCall(t)
			res, err := a.attack(t, a.caio, e, "Toren", battleaxe, "Goblin", d20(16))
			return a, e, before, res, err
		}),
		attackOn("a second attack in the same turn", func(t *testing.T) (*armed, *playv1.Encounter, int, *playv1.RollAttackResponse, error) {
			t.Helper()
			a := newArmed(t)
			e := a.threeAndAGoblin(t)
			a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", d20(3))
			before := a.markCall(t)
			res, err := a.attack(t, a.caio, e, "Toren", battleaxe, "Goblin", d20(15))
			return a, e, before, res, err
		}),
		attackOn("an attack out of turn", func(t *testing.T) (*armed, *playv1.Encounter, int, *playv1.RollAttackResponse, error) {
			t.Helper()
			a := newArmed(t)
			e := a.threeAndAGoblin(t)
			before := a.markCall(t)
			res, err := a.attack(t, a.bia, e, "Brisa", rapier, "Goblin", d20(15))
			return a, e, before, res, err
		}),
		attackOn("an attack on a defeated target", func(t *testing.T) (*armed, *playv1.Encounter, int, *playv1.RollAttackResponse, error) {
			t.Helper()
			a := newArmed(t)
			e := a.threeAndAGoblin(t)
			if _, err := a.adjustHP(t, e, "Goblin", func(r *playv1.AdjustCombatantHitPointsRequest) {
				r.Change = &playv1.AdjustCombatantHitPointsRequest_Damage{Damage: 20}
			}); err != nil {
				t.Fatalf("AdjustCombatantHitPoints() error = %v", err)
			}
			before := a.markCall(t)
			res, err := a.attack(t, a.caio, e, "Toren", battleaxe, "Goblin", d20(15))
			return a, e, before, res, err
		}),
	}
}

// attackScenarios are the scenarios of the golden, in the order they are listed.
func attackScenarios() []goldenScenario {
	return append(customAttacks(), []goldenScenario{
		attackThrough("a hit", asToren, typedD20(15)),
		attackThrough("a miss", asToren, typedD20(2)),
		attackThrough("a natural 20", asToren, typedD20(20)),
		attackThrough("a natural 1", asToren, typedD20(1)),
		attackThrough("with the roll made in the app", asToren, inAppRoll),
		attackThrough("with an advantage asked for and refused", asToren, func(r *playv1.RollAttackRequest) {
			r.Roll, r.D20Faces, r.RollMode = nil, []int32{3, 17}, playv1.RollMode_ROLL_MODE_ADVANTAGE
		}),
		attackThrough("with a disadvantage asked for and a reason", asToren, func(r *playv1.RollAttackRequest) {
			r.Roll, r.D20Faces, r.RollMode, r.ModeReason = nil, []int32{3, 17}, playv1.RollMode_ROLL_MODE_DISADVANTAGE, "Toren está cansado"
		}),
		attackThrough("with an extra die", asToren, func(r *playv1.RollAttackRequest) { r.ExtraDieFaces = []int32{3} }),
		attackThrough("as the master for the player's character", asMaster, typedD20(12)),
	}...)
}

// TestRollAttackIsFrozen runs every scenario on a fight of its own and compares what each reads with the golden.
func TestRollAttackIsFrozen(t *testing.T) {
	t.Parallel()
	runGolden(t, rollAttackGolden, attackScenarios())
}
