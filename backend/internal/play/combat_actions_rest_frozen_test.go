package play

import (
	"strings"
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// These tests freeze what the rest of combat_actions.go does today, scenario by scenario: ApplyPendingDamage
// and DiscardPendingDamage (the master settles a hit on a player's character), AdjustCombatantHitPoints (the
// master's hand on an NPC's hit points) and GetTurnOptions (the read of what a combatant can do, as each viewer
// sees it). For a write: every refusal with its code, message and blocked reason, and for each one that goes
// through the answer, the events it writes (in order, the combatants named by label), the combatants, the
// vitals, the pending damages and the windows it left. For the read: the whole answer, by viewer. They are the
// net under the split of those functions into phases. testdata/actions_rest.golden holds what each scenario
// reads; run with UPDATE_GOLDEN=1 to write the file again, and read the diff.

const actionsRestGolden = "testdata/actions_rest.golden"

// actionsRestScenario is one scenario of the golden: run plays it on a fight of its own and reads the outcome.
type actionsRestScenario struct {
	name string
	run  func(t *testing.T) string
}

// actionsRestWorld starts a fight and hands back the campaign and the combat.
type actionsRestWorld func(t *testing.T) (*armed, *playv1.Encounter)

// actionsRestUsual is the usual fight: Toren (fighter 2) acts first, next to the goblin.
func actionsRestUsual(t *testing.T) (*armed, *playv1.Encounter) {
	t.Helper()
	a := newArmed(t)
	e := a.threeAndAGoblin(t)
	return a, e
}

// actionsRestReport is what a scenario read after its call: the head (the answer or the refusal), the events
// written since `after`, the combatants, the vitals, the wild shapes, the pending damages and the windows.
func (a *armed) actionsRestReport(t *testing.T, after int, head ...string) string {
	t.Helper()
	e := a.get(t, a.master)
	names := labelsOf(e)
	lines := append([]string{}, head...)
	for _, l := range a.eventTrace(t, e, after) {
		lines = append(lines, "event "+l)
	}
	tables := []struct{ tag, query string }{
		{"combatant", `SELECT label, hp_current, hp_temp, defeated, death_successes, death_failures, COALESCE(concentration_spell, '-'), took_damage, reaction_used, grid_col, grid_row, hidden, rage_end_pending, COALESCE(death_save_rolled::TEXT, '-') FROM combatants ORDER BY label`},
		{"vitals", `SELECT c.name, c.status, v.hit_points_current, v.hit_points_temporary, v.spell_slots_used::TEXT, v.pact_slots_used, v.resources_used::TEXT FROM character_vitals v JOIN characters c ON c.id = v.character_id`},
		{"shape", `SELECT c.name, w.beast, w.hp FROM character_wild_shapes w JOIN characters c ON c.id = w.character_id`},
		{"damage", `SELECT attack_key, status, critical, damage_type, COALESCE(amount, -1), COALESCE(applied_amount, -1), healing, COALESCE(taken, -1), steps::TEXT, COALESCE(after_steps, -1), resolved_at IS NOT NULL, cast_id IS NOT NULL FROM pending_damages`},
		{"window", `SELECT kind, status, COALESCE(closed_reason, '-'), step, trigger::TEXT, COALESCE(outcome::TEXT, '-') FROM reaction_windows`},
		{"hold", `SELECT kind, state, data::TEXT FROM reaction_holds`},
	}
	for _, tb := range tables {
		for _, r := range a.rowsOf(t, tb.query) {
			lines = append(lines, tb.tag+" "+r)
		}
	}
	out := strings.Join(lines, "\n")
	return uuidPattern.ReplaceAllStringFunc(out, func(id string) string {
		if l, ok := names[id]; ok {
			return l
		}
		return "?"
	})
}

// actionsRestApply calls ApplyPendingDamage as u, with a request that edit finishes.
func (a *armed) actionsRestApply(t *testing.T, u *user, e *playv1.Encounter, id string, edit func(*playv1.ApplyPendingDamageRequest)) (*playv1.ApplyPendingDamageResponse, error) {
	t.Helper()
	req := &playv1.ApplyPendingDamageRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), PendingDamageId: id, IdempotencyKey: newKey()}
	if edit != nil {
		edit(req)
	}
	res, err := u.combat.ApplyPendingDamage(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

// actionsRestDiscard calls DiscardPendingDamage as u, with a request that edit finishes.
func (a *armed) actionsRestDiscard(t *testing.T, u *user, e *playv1.Encounter, id string, edit func(*playv1.DiscardPendingDamageRequest)) (*playv1.DiscardPendingDamageResponse, error) {
	t.Helper()
	req := &playv1.DiscardPendingDamageRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), PendingDamageId: id, IdempotencyKey: newKey()}
	if edit != nil {
		edit(req)
	}
	res, err := u.combat.DiscardPendingDamage(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

// actionsRestHit plays a fight up to a hit that waits for the master: the encounter and the pending damage.
type actionsRestHit func(t *testing.T) (*armed, *playv1.Encounter, string)

// actionsRestGoblinHits is the goblin's hit with its sword on the target, on its turn, in the world.
func actionsRestGoblinHits(world actionsRestWorld, target string, face int32, prep func(t *testing.T, a *armed, e *playv1.Encounter)) actionsRestHit {
	return func(t *testing.T) (*armed, *playv1.Encounter, string) {
		t.Helper()
		a, e := world(t)
		if prep != nil {
			prep(t, a, e)
		}
		e = a.passTo(t, e, "Goblin")
		res := a.mustAttack(t, a.master, e, "Goblin", sword, target, d20(face))
		if res.GetPendingDamage() == nil {
			t.Fatalf("RollAttack() = %v, want a hit", res.GetRoll())
		}
		e = a.get(t, a.master)
		return a, e, res.GetPendingDamage().GetId()
	}
}

// actionsRestOnToren is the goblin's hit on Toren in the usual fight (a natural 20 for a critical hit).
func actionsRestOnToren(crit bool) actionsRestHit {
	face := int32(15)
	if crit {
		face = 20
	}
	return actionsRestGoblinHits(actionsRestUsual, "Toren", face, nil)
}

// actionsRestRolled rolls the damage of a pending damage as the master, with a typed sum.
func actionsRestRolled(t *testing.T, a *armed, e *playv1.Encounter, id string, sum int32) {
	t.Helper()
	if _, err := a.rawDamage(t, a.master, e, id, typedSum(sum)); err != nil {
		t.Fatalf("RollDamage(%d) error = %v", sum, err)
	}
}

// actionsRestSettle is a scenario of the master settling a hit: hit plays the fight, the damage is rolled with
// the typed sum (4 when 0, not rolled at all when unrolled), prep changes the fight after the roll, and the
// settle is the apply or the discard of the request that edit or discardEdit finishes.
type actionsRestSettle struct {
	name        string
	hit         actionsRestHit
	sum         int32
	unrolled    bool
	prep        func(t *testing.T, a *armed, e *playv1.Encounter, id string)
	as          func(a *armed) *user
	discard     bool
	edit        func(*playv1.ApplyPendingDamageRequest)
	discardEdit func(*playv1.DiscardPendingDamageRequest)
}

func (c actionsRestSettle) scenario() actionsRestScenario {
	return actionsRestScenario{name: c.name, run: func(t *testing.T) string {
		t.Helper()
		hit := c.hit
		if hit == nil {
			hit = actionsRestOnToren(false)
		}
		a, e, id := hit(t)
		if !c.unrolled {
			sum := c.sum
			if sum == 0 {
				sum = 4
			}
			actionsRestRolled(t, a, e, id, sum)
		}
		if c.prep != nil {
			c.prep(t, a, e, id)
		}
		e = a.get(t, a.master)
		as := c.as
		if as == nil {
			as = asMaster
		}
		before := a.afterSetup(t)
		names := labelsOf(e)
		if c.discard {
			res, err := a.actionsRestDiscard(t, as(a), e, id, c.discardEdit)
			return a.actionsRestReport(t, before, actionsRestHead(res.GetPendingDamage(), names, err))
		}
		res, err := a.actionsRestApply(t, as(a), e, id, c.edit)
		return a.actionsRestReport(t, before, actionsRestHead(res.GetPendingDamage(), names, err))
	}}
}

// actionsRestHead is the first line of a report: the pending damage the call answered or its refusal.
func actionsRestHead(p *playv1.PendingDamage, names map[string]string, err error) string {
	if err != nil {
		return "refused " + takeActionRefusalText(err)
	}
	return "pending " + jsonOfMessage(p, names, "id")
}

func actionsRestAmount(n int32) func(*playv1.ApplyPendingDamageRequest) {
	return func(r *playv1.ApplyPendingDamageRequest) { r.Amount = &n }
}

func actionsRestIgnoring(sources ...string) func(*playv1.ApplyPendingDamageRequest) {
	return func(r *playv1.ApplyPendingDamageRequest) { r.IgnoreModifiers = sources }
}

// actionsRestToren changes Toren's hit points, temporary hit points and death saves (-1 leaves one alone).
func actionsRestToren(hp, temp int32, failures int) func(t *testing.T, a *armed, e *playv1.Encounter, id string) {
	return func(t *testing.T, a *armed, _ *playv1.Encounter, _ string) {
		t.Helper()
		a.correct(t, a.toren, func(r *playv1.AdjustCharacterVitalsRequest) {
			if hp >= 0 {
				r.HitPointsCurrent = &hp
			}
			if temp >= 0 {
				r.HitPointsTemporary = &temp
			}
		})
		if failures > 0 {
			a.execSQL(t, `UPDATE combatants SET death_failures = $2, death_successes = 1 WHERE id = $1`, a.id(t, "Toren"), failures)
		}
	}
}

// actionsRestSteps runs the steps one after the other.
func actionsRestSteps(steps ...func(t *testing.T, a *armed, e *playv1.Encounter, id string)) func(t *testing.T, a *armed, e *playv1.Encounter, id string) {
	return func(t *testing.T, a *armed, e *playv1.Encounter, id string) {
		t.Helper()
		for _, s := range steps {
			s(t, a, e, id)
		}
	}
}

// actionsRestSQL runs a statement on the fight (with the pending damage as $1), to reach a state no call would.
func actionsRestSQL(sql string) func(t *testing.T, a *armed, e *playv1.Encounter, id string) {
	return func(t *testing.T, a *armed, _ *playv1.Encounter, id string) {
		t.Helper()
		if strings.Contains(sql, "$1") {
			a.execSQL(t, sql, id)
			return
		}
		a.execSQL(t, sql)
	}
}

// actionsRestConcentrating makes Toren concentrate.
func actionsRestConcentrating(t *testing.T, a *armed, _ *playv1.Encounter, _ string) {
	t.Helper()
	a.concentrate(t, "Toren", webSpell)
}

// actionsRestEnded ends the combat.
func actionsRestEnded(t *testing.T, a *armed, e *playv1.Encounter, _ string) {
	t.Helper()
	endEncounter(t, a, e)
}

// actionsRestSettled applies or discards the pending damage first, as the master.
func actionsRestSettled(apply bool) func(t *testing.T, a *armed, e *playv1.Encounter, id string) {
	return func(t *testing.T, a *armed, e *playv1.Encounter, id string) {
		t.Helper()
		if _, err := a.settle(t, a.master, e, id, apply); err != nil {
			t.Fatalf("settle(apply=%v) error = %v", apply, err)
		}
	}
}

// actionsRestSettleRefusals are the refusals of the two calls that settle a hit.
func actionsRestSettleRefusals() []actionsRestScenario {
	stranger := func(a *armed) *user { return a.h.newUser("Zé") }
	missing := "00000000-0000-4000-8000-000000000001"
	cases := []actionsRestSettle{
		{name: "apply refused a campaign that is not a UUID", edit: func(r *playv1.ApplyPendingDamageRequest) { r.CampaignId = "nope" }},
		{name: "apply refused an amount over the limit", edit: actionsRestAmount(10000)},
		{name: "apply accepted the largest amount", edit: actionsRestAmount(9999)},
		{name: "apply refused the player of the target", as: asToren},
		{name: "apply refused another player", as: asDamageAna},
		{name: "apply refused a user who is not in the campaign", as: stranger},
		{name: "apply refused a pending damage nobody knows", edit: func(r *playv1.ApplyPendingDamageRequest) { r.PendingDamageId = missing }},
		{name: "apply refused an encounter nobody knows", edit: func(r *playv1.ApplyPendingDamageRequest) { r.EncounterId = "00000000-0000-4000-8000-000000000002" }},
		{name: "apply refused a combat that ended", prep: actionsRestEnded},
		{name: "apply refused a damage that was not rolled", unrolled: true},
		{name: "apply refused a damage that waits for a reaction", prep: actionsRestSQL(`UPDATE pending_damages SET status = 'awaiting_reaction' WHERE id = $1`)},
		{name: "apply refused a damage applied already", prep: actionsRestSettled(true)},
		{name: "apply refused a damage discarded already", prep: actionsRestSettled(false)},
		{name: "apply refused a damage rolled for an NPC", hit: actionsRestHitOnGoblin, unrolled: true, prep: actionsRestSQL(`UPDATE pending_damages SET status = 'rolled', amount = 3 WHERE id = $1`)},
		{name: "discard refused a pending damage that is not a UUID", discard: true, discardEdit: func(r *playv1.DiscardPendingDamageRequest) { r.PendingDamageId = "nope" }},
		{name: "discard refused the player of the target", discard: true, as: asToren},
		{name: "discard refused a user who is not in the campaign", discard: true, as: stranger},
		{name: "discard refused a pending damage nobody knows", discard: true, discardEdit: func(r *playv1.DiscardPendingDamageRequest) { r.PendingDamageId = missing }},
		{name: "discard refused a combat that ended", discard: true, prep: actionsRestEnded},
		{name: "discard refused a damage applied already", discard: true, prep: actionsRestSettled(true)},
		{name: "discard refused a damage discarded already", discard: true, prep: actionsRestSettled(false)},
	}
	return actionsRestSettleScenarios(cases)
}

func actionsRestSettleScenarios(cases []actionsRestSettle) []actionsRestScenario {
	out := make([]actionsRestScenario, 0, len(cases))
	for _, c := range cases {
		out = append(out, c.scenario())
	}
	return out
}

// actionsRestHitOnGoblin is Toren's hit on the goblin, which waits for its damage roll.
func actionsRestHitOnGoblin(t *testing.T) (*armed, *playv1.Encounter, string) {
	t.Helper()
	a, e := actionsRestUsual(t)
	return a, e, torenHits(t, a, e, battleaxe, 15)
}
