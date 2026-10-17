package play

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// These tests freeze what RollDamage does today, scenario by scenario: every refusal with its code and
// message, and for each damage that goes through the answer, the events it writes (in order, the combatants
// named by label), the hit points and the death saves it leaves, the pending damages with the parts and the
// steps it kept, and the windows it holds. They are the net under the split of rollDamage into phases.
// testdata/roll_damage.golden holds what each scenario reads; run with UPDATE_GOLDEN=1 to write the file
// again, and read the diff.

const rollDamageGolden = "testdata/roll_damage.golden"

// damageReport is what a scenario read after its damage roll: the answer or the refusal, the events written
// since `after`, the combatants, the vitals, the pending damages and the windows the roll left.
func (a *armed) damageReport(t *testing.T, after int, res *playv1.RollDamageResponse, err error) string {
	t.Helper()
	names := labelsOf(a.get(t, a.master))
	var lines []string
	if err != nil {
		lines = append(lines, "refused "+damageRefusalText(err))
	} else {
		lines = append(lines, "pending "+jsonOfMessage(res.GetPendingDamage(), names, "id"))
		for _, p := range res.GetCastPendingDamages() {
			lines = append(lines, "cast pending "+jsonOfMessage(p, names, "id"))
		}
	}
	for _, l := range a.eventTrace(t, a.get(t, a.master), after) {
		lines = append(lines, "event "+l)
	}
	for _, r := range a.rowsOf(t, `SELECT label, hp_current, hp_temp, defeated, death_successes, death_failures, COALESCE(concentration_spell, '-'), took_damage, COALESCE(sneak_attack_turn, '-'), COALESCE(colossus_slayer_turn, '-'), reaction_used, grid_col, grid_row, hidden, rage_end_pending FROM combatants ORDER BY label`) {
		lines = append(lines, "combatant "+r)
	}
	for _, r := range a.rowsOf(t, `SELECT c.name, v.hit_points_current, v.hit_points_temporary, v.spell_slots_used::TEXT, v.pact_slots_used, v.resources_used::TEXT FROM character_vitals v JOIN characters c ON c.id = v.character_id`) {
		lines = append(lines, "vitals "+r)
	}
	for _, r := range a.rowsOf(t, `SELECT attack_key, status, critical, dice_count, dice_sides, dice_bonus, damage_type, faces::TEXT, physical, COALESCE(amount, -1), COALESCE(applied_amount, -1), COALESCE(roll_total, -1), half, healing, COALESCE(taken, -1), critical_max, critical_max_rule, savage_dice, parts::TEXT, part_rolls::TEXT, steps::TEXT, COALESCE(after_steps, -1), COALESCE(landed_before::TEXT, '-'), resolved_at IS NOT NULL FROM pending_damages`) {
		lines = append(lines, "damage "+r)
	}
	for _, r := range a.rowsOf(t, `SELECT kind, status, COALESCE(closed_reason, '-'), step, trigger::TEXT, COALESCE(outcome::TEXT, '-') FROM reaction_windows`) {
		lines = append(lines, "window "+r)
	}
	for _, r := range a.rowsOf(t, `SELECT kind, state, data::TEXT FROM reaction_holds`) {
		lines = append(lines, "hold "+r)
	}
	for _, r := range a.rowsOf(t, `SELECT kind, used, COALESCE(choice, '-') FROM hit_riders`) {
		lines = append(lines, "rider "+r)
	}
	out := strings.Join(lines, "\n")
	return uuidPattern.ReplaceAllStringFunc(out, func(id string) string {
		if l, ok := names[id]; ok {
			return l
		}
		return "?"
	})
}

// refusalOf is a refusal as text: its code, its message and, for a blocked action, the reason.
func damageRefusalText(err error) string {
	msg := err.Error()
	out := fmt.Sprintf("%v: %v", connect.CodeOf(err), msg[strings.Index(msg, ": ")+2:])
	var cerr *connect.Error
	if errors.As(err, &cerr) {
		for _, d := range cerr.Details() {
			if m, derr := d.Value(); derr == nil {
				if b, ok := m.(*playv1.EncounterBlocked); ok {
					out += " [" + b.GetReason().String() + "]"
				}
			}
		}
	}
	return out
}

// rawDamage calls RollDamage as u with a request that edit builds from a roll made in the app.
func (a *armed) rawDamage(t *testing.T, u *user, e *playv1.Encounter, pendingID string, edit func(*playv1.RollDamageRequest)) (*playv1.RollDamageResponse, error) {
	t.Helper()
	req := &playv1.RollDamageRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), PendingDamageId: pendingID, IdempotencyKey: newKey(),
		Roll: &playv1.RollDamageRequest_RollInApp{RollInApp: true},
	}
	edit(req)
	res, err := u.combat.RollDamage(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

// damagePlay is what a scenario that plays its own fight hands back: the fight, the events it wrote before
// the damage and the damage's answer.
type damagePlay struct {
	a      *armed
	before int
	res    *playv1.RollDamageResponse
	err    error
}

type damageScenario struct {
	name string
	run  func(t *testing.T) string
}

// damageOn is a scenario that plays its own fight.
func damageOn(name string, play func(t *testing.T) damagePlay) damageScenario {
	return damageScenario{name: name, run: func(t *testing.T) string {
		t.Helper()
		p := play(t)
		return p.a.damageReport(t, p.before, p.res, p.err)
	}}
}

// rolled is the damage roll of u on a pending damage, with the events counted just before it.
func (a *armed) rolled(t *testing.T, u *user, e *playv1.Encounter, pendingID string, edit func(*playv1.RollDamageRequest)) damagePlay {
	t.Helper()
	before := a.afterSetup(t)
	res, err := a.rawDamage(t, u, e, pendingID, edit)
	return damagePlay{a: a, before: before, res: res, err: err}
}

func asDamageAna(a *armed) *user { return a.ana }

// torenHits is Toren's hit on the goblin (a natural 20 for a critical one) in the usual fight; it answers the
// pending damage.
func torenHits(t *testing.T, a *armed, e *playv1.Encounter, key string, face int32) string {
	t.Helper()
	res := a.mustAttack(t, a.caio, e, "Toren", key, "Goblin", d20(face))
	if res.GetPendingDamage() == nil {
		t.Fatalf("RollAttack() = %v, want a hit", res.GetRoll())
	}
	return res.GetPendingDamage().GetId()
}

// goblinHitsToren is the goblin's hit on Toren on its turn; it answers the pending damage.
func goblinHitsToren(t *testing.T, a *armed, e *playv1.Encounter, face int32) (*playv1.Encounter, string) {
	t.Helper()
	e = a.passTo(t, e, "Goblin")
	res := a.mustAttack(t, a.master, e, "Goblin", sword, "Toren", d20(face))
	if res.GetPendingDamage() == nil {
		t.Fatalf("RollAttack() = %v, want a hit", res.GetRoll())
	}
	return e, res.GetPendingDamage().GetId()
}

// damageRefusal is a scenario of a damage roll on Toren's hit that edits the request and reads the refusal.
func damageRefusal(name string, as func(a *armed) *user, edit func(*playv1.RollDamageRequest)) damageScenario {
	return damageOn("refused "+name, func(t *testing.T) damagePlay {
		t.Helper()
		a := newArmed(t)
		e := a.threeAndAGoblin(t)
		id := torenHits(t, a, e, battleaxe, 15)
		return a.rolled(t, as(a), e, id, edit)
	})
}

// damageThrough is a scenario of a damage roll on Toren's hit (a critical one when crit is set).
func damageThrough(name string, crit bool, as func(a *armed) *user, edit func(*playv1.RollDamageRequest)) damageScenario {
	return damageOn(name, func(t *testing.T) damagePlay {
		t.Helper()
		a := newArmed(t)
		e := a.threeAndAGoblin(t)
		face := int32(15)
		if crit {
			face = 20
		}
		id := torenHits(t, a, e, battleaxe, face)
		return a.rolled(t, as(a), e, id, edit)
	})
}

func typedSum(n int32) func(*playv1.RollDamageRequest) {
	return func(r *playv1.RollDamageRequest) { r.Roll = &playv1.RollDamageRequest_TypedSum{TypedSum: n} }
}

func typedParts(parts ...*playv1.TypedPart) func(*playv1.RollDamageRequest) {
	return func(r *playv1.RollDamageRequest) { r.Roll, r.TypedParts = nil, parts }
}

// refusedDamages are the requests the damage roll refuses before it writes anything.
func refusedDamages() []damageScenario {
	return []damageScenario{
		damageRefusal("a key that is not a UUID", asToren, func(r *playv1.RollDamageRequest) { r.IdempotencyKey = "not-a-key" }),
		damageRefusal("a typed sum of 0", asToren, typedSum(0)),
		damageRefusal("a typed sum of 9 for 1d8", asToren, typedSum(9)),
		damageRefusal("a typed part of another key", asToren, typedParts(&playv1.TypedPart{PartKey: "sneak-attack", Sum: 3})),
		damageRefusal("a typed part out of range", asToren, typedParts(&playv1.TypedPart{PartKey: "weapon", Sum: 99})),
		damageRefusal("a pending damage that is not there", asToren, func(r *playv1.RollDamageRequest) {
			r.PendingDamageId = "00000000-0000-4000-8000-000000000001"
		}),
		damageRefusal("an encounter that is not there", asToren, func(r *playv1.RollDamageRequest) {
			r.EncounterId = "00000000-0000-4000-8000-000000000002"
		}),
		damageRefusal("another player's attacker", asDamageAna, inAppDamage),
		damageRefusal("a selected extra the hit does not offer", asToren, func(r *playv1.RollDamageRequest) {
			r.ExtrasChosen = true
			r.SelectedExtras = []*playv1.SelectedExtra{{Key: "sneak-attack"}}
		}),
	}
}

// damageScenarios are the scenarios of the golden, in the order they are listed.
func damageScenarios() []damageScenario {
	out := refusedDamages()
	out = append(out, plainDamages()...)
	out = append(out, rogueAndBarbarianDamages()...)
	out = append(out, paladinDamages()...)
	out = append(out, maneuverDamages()...)
	out = append(out, skeletonDamages()...)
	out = append(out, spellDamages()...)
	out = append(out, reactionDamages()...)
	out = append(out, opportunityDamages()...)
	return out
}

// usualDamage is a scenario on the usual fight: Toren hits the goblin (a natural 20 for a critical hit), or the
// goblin hits Toren on its turn. before changes the fight before the attack, prepare after it, with the
// pending damage; as rolls the damage with edit.
type usualDamage struct {
	name   string
	crit   bool
	goblin bool
	// key is the attack Toren hits with; the battleaxe when empty.
	key     string
	before  func(t *testing.T, a *armed, e *playv1.Encounter)
	prepare func(t *testing.T, a *armed, e *playv1.Encounter, id string)
	as      func(a *armed) *user
	edit    func(*playv1.RollDamageRequest)
}

func (u usualDamage) scenario() damageScenario {
	return damageOn(u.name, func(t *testing.T) damagePlay {
		t.Helper()
		a := newArmed(t)
		e := a.threeAndAGoblin(t)
		if u.before != nil {
			u.before(t, a, e)
		}
		face := int32(15)
		if u.crit {
			face = 20
		}
		var id string
		if u.goblin {
			e, id = goblinHitsToren(t, a, e, face)
		} else {
			id = torenHits(t, a, e, battleaxe, face)
		}
		if u.prepare != nil {
			u.prepare(t, a, e, id)
		}
		as, edit := u.as, u.edit
		if as == nil {
			as = asToren
			if u.goblin {
				as = asMaster
			}
		}
		if edit == nil {
			edit = inAppDamage
		}
		return a.rolled(t, as(a), e, id, edit)
	})
}

// scriptedFaces queues the faces the app rolls next.
func scriptedFaces(fs ...int) func(t *testing.T, a *armed, e *playv1.Encounter) {
	return func(t *testing.T, a *armed, _ *playv1.Encounter) {
		t.Helper()
		a.h.roller.queue(fs...)
	}
}

// plainDamages are the hits of the usual fight that go through.
func plainDamages() []damageScenario {
	var out []damageScenario
	for _, u := range usualDamages() {
		out = append(out, u.scenario())
	}
	return append(out,
		damageThrough("a hit rolled in the app", false, asToren, inAppDamage),
		damageThrough("a hit with a typed sum", false, asToren, typedSum(5)),
		damageThrough("a hit with a typed sum that defeats the goblin", false, asToren, typedSum(8)),
		damageThrough("a critical hit rolled in the app", true, asToren, inAppDamage),
		damageThrough("a critical hit with a typed sum", true, asToren, typedSum(11)),
		damageThrough("a hit rolled as the master", false, asMaster, inAppDamage),
		damageThrough("a hit with extras chosen and none marked", false, asToren, func(r *playv1.RollDamageRequest) { r.ExtrasChosen = true }),
	)
}

// TestRollDamageIsFrozen runs every scenario on a fight of its own and compares what each reads with the golden.
//
//nolint:tparallel // the group waits for its parallel scenarios, so that the golden is settled after them.
func TestRollDamageIsFrozen(t *testing.T) {
	t.Parallel()
	g := &goldenBlocks{}
	t.Run("scenarios", func(t *testing.T) {
		for _, sc := range damageScenarios() {
			t.Run(sc.name, func(t *testing.T) {
				t.Parallel()
				g.put(sc.name, sc.run(t))
			})
		}
	})
	g.settle(t, rollDamageGolden)
}
