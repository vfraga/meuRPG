package play

import (
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The scenarios of the golden for the master's hand on an NPC's hit points: AdjustCombatantHitPoints.

// actionsRestAdjust is a scenario of the master (or another user) adjusting the hit points of a combatant of
// a world (the goblin by default): prep changes the fight first and edit finishes the request.
type actionsRestAdjust struct {
	name  string
	world actionsRestWorld
	prep  func(t *testing.T, a *armed, e *playv1.Encounter)
	label string
	as    func(a *armed) *user
	edit  func(*playv1.AdjustCombatantHitPointsRequest)
}

func (c actionsRestAdjust) scenario() actionsRestScenario {
	return actionsRestScenario{name: c.name, run: func(t *testing.T) string {
		t.Helper()
		world := c.world
		if world == nil {
			world = actionsRestUsual
		}
		a, e := world(t)
		if c.prep != nil {
			c.prep(t, a, e)
		}
		e = a.get(t, a.master)
		label := c.label
		if label == "" {
			label = "Goblin"
		}
		as := c.as
		if as == nil {
			as = asMaster
		}
		before := a.afterSetup(t)
		req := &playv1.AdjustCombatantHitPointsRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, label), IdempotencyKey: newKey()}
		if c.edit != nil {
			c.edit(req)
		}
		res, err := as(a).combat.AdjustCombatantHitPoints(t.Context(), connect.NewRequest(req))
		head := "ok"
		if err != nil {
			head = "refused " + takeActionRefusalText(err)
		} else {
			head += " " + jsonOfMessage(byLabel(t, res.Msg.GetEncounter(), label), labelsOf(e))
		}
		return a.actionsRestReport(t, before, head)
	}}
}

func actionsRestHealBy(n int32) func(*playv1.AdjustCombatantHitPointsRequest) {
	return func(r *playv1.AdjustCombatantHitPointsRequest) {
		r.Change = &playv1.AdjustCombatantHitPointsRequest_Heal{Heal: n}
	}
}

func actionsRestSetHP(n int32) func(*playv1.AdjustCombatantHitPointsRequest) {
	return func(r *playv1.AdjustCombatantHitPointsRequest) {
		r.Change = &playv1.AdjustCombatantHitPointsRequest_HitPoints{HitPoints: n}
	}
}

func actionsRestTempHP(n int32) func(*playv1.AdjustCombatantHitPointsRequest) {
	return func(r *playv1.AdjustCombatantHitPointsRequest) { r.HitPointsTemporary = &n }
}

// actionsRestAdjusting chains the edits of a request.
func actionsRestAdjusting(edits ...func(*playv1.AdjustCombatantHitPointsRequest)) func(*playv1.AdjustCombatantHitPointsRequest) {
	return func(r *playv1.AdjustCombatantHitPointsRequest) {
		for _, e := range edits {
			e(r)
		}
	}
}

// actionsRestHurt takes hit points off a combatant, as the master.
func actionsRestHurt(label string, points int32) func(t *testing.T, a *armed, e *playv1.Encounter) {
	return func(t *testing.T, a *armed, e *playv1.Encounter) {
		t.Helper()
		if _, err := a.adjustHP(t, e, label, damageHP(points)); err != nil {
			t.Fatalf("AdjustCombatantHitPoints(%s) error = %v", label, err)
		}
	}
}

// actionsRestWarm gives the goblin 4 temporary hit points.
func actionsRestWarm(t *testing.T, a *armed, _ *playv1.Encounter) {
	t.Helper()
	a.execSQL(t, `UPDATE combatants SET hp_temp = 4 WHERE id = $1`, a.id(t, "Goblin"))
}

// actionsRestAdjustments are the corrections of the master and the refusals of the call.
func actionsRestAdjustments() []actionsRestScenario {
	stranger := func(a *armed) *user { return a.h.newUser("Zé") }
	missing := "00000000-0000-4000-8000-000000000001"
	hide := func(t *testing.T, a *armed, e *playv1.Encounter) {
		t.Helper()
		setHidden(t, a, e, "Goblin", true)
	}
	cases := []actionsRestAdjust{
		{name: "adjust refused a campaign that is not a UUID", edit: actionsRestAdjusting(damageHP(1), func(r *playv1.AdjustCombatantHitPointsRequest) { r.CampaignId = "nope" })},
		{name: "adjust refused a damage over the limit", edit: damageHP(10000)},
		{name: "adjust refused a hit point count over the maximum", edit: actionsRestSetHP(8)},
		{name: "adjust refused the player of a hero", as: asToren, edit: damageHP(1)},
		{name: "adjust refused a user who is not in the campaign", as: stranger, edit: damageHP(1)},
		{name: "adjust refused a combatant nobody knows", edit: actionsRestAdjusting(damageHP(1), func(r *playv1.AdjustCombatantHitPointsRequest) { r.CombatantId = missing })},
		{name: "adjust refused an encounter nobody knows", edit: actionsRestAdjusting(damageHP(1), func(r *playv1.AdjustCombatantHitPointsRequest) {
			r.EncounterId = "00000000-0000-4000-8000-000000000002"
		})},
		{name: "adjust refused a combat that ended", edit: damageHP(1), prep: func(t *testing.T, a *armed, e *playv1.Encounter) {
			t.Helper()
			endEncounter(t, a, e)
		}},
		{name: "adjust refused a player's character", label: "Toren", edit: damageHP(1)},
		{name: "adjust refused a player's character for a heal", label: "Pensantus", edit: actionsRestHealBy(1)},
		{name: "adjust a damage", edit: damageHP(3)},
		{name: "adjust a damage that defeats the goblin", edit: damageHP(7)},
		{name: "adjust a damage over the hit points", edit: damageHP(100)},
		{name: "adjust a damage of 0", edit: damageHP(0)},
		{name: "adjust the largest damage", edit: damageHP(9999)},
		{name: "adjust a damage on a defeated goblin", edit: damageHP(2), prep: actionsRestHurt("Goblin", 7)},
		{name: "adjust a damage absorbed by temporary hit points", edit: damageHP(3), prep: actionsRestWarm},
		{name: "adjust a damage that goes through temporary hit points", edit: damageHP(6), prep: actionsRestWarm},
		{name: "adjust a damage with new temporary hit points", edit: actionsRestAdjusting(damageHP(2), actionsRestTempHP(5))},
		{name: "adjust a heal", edit: actionsRestHealBy(3), prep: actionsRestHurt("Goblin", 5)},
		{name: "adjust a heal over the maximum", edit: actionsRestHealBy(50), prep: actionsRestHurt("Goblin", 5)},
		{name: "adjust a heal at full hit points", edit: actionsRestHealBy(3)},
		{name: "adjust a heal of 0", edit: actionsRestHealBy(0), prep: actionsRestHurt("Goblin", 5)},
		{name: "adjust a heal that revives a defeated goblin", edit: actionsRestHealBy(2), prep: actionsRestHurt("Goblin", 7)},
		{name: "adjust a heal with temporary hit points", edit: actionsRestAdjusting(actionsRestHealBy(2), actionsRestTempHP(3)), prep: actionsRestHurt("Goblin", 5)},
		{name: "adjust the hit points to 0", edit: actionsRestSetHP(0)},
		{name: "adjust the hit points down", edit: actionsRestSetHP(4)},
		{name: "adjust the hit points to the maximum", edit: actionsRestSetHP(7), prep: actionsRestHurt("Goblin", 5)},
		{name: "adjust the hit points of a defeated goblin", edit: actionsRestSetHP(5), prep: actionsRestHurt("Goblin", 7)},
		{name: "adjust the hit points with temporary hit points", edit: actionsRestAdjusting(actionsRestSetHP(3), actionsRestTempHP(2))},
		{name: "adjust only the temporary hit points", edit: actionsRestTempHP(5)},
		{name: "adjust the temporary hit points to 0", edit: actionsRestTempHP(0), prep: actionsRestWarm},
		{name: "adjust the largest temporary hit points", edit: actionsRestTempHP(999)},
		{name: "adjust a hidden goblin", edit: damageHP(3), prep: hide},
		{name: "adjust the Capitão in a fight of casters", world: actionsRestCasters, label: "Capitão Goblin", edit: damageHP(10)},
		{name: "adjust the Capitão's hit points in a fight of casters", world: actionsRestCasters, label: "Capitão Goblin", edit: actionsRestSetHP(1)},
		{name: "adjust a goblin in a combat without a map", world: takeActionTheatre, edit: damageHP(3)},
		{name: "adjust a goblin that concentrates", edit: damageHP(3), prep: func(t *testing.T, a *armed, _ *playv1.Encounter) {
			t.Helper()
			a.concentrate(t, "Goblin", webSpell)
		}},
	}
	out := make([]actionsRestScenario, 0, len(cases)+1)
	for _, c := range cases {
		out = append(out, c.scenario())
	}
	return append(out, actionsRestAdjustTwice())
}

// actionsRestAdjustTwice is a retry of one request: the same key is the same answer.
func actionsRestAdjustTwice() actionsRestScenario {
	return actionsRestScenario{name: "adjust the same request twice", run: func(t *testing.T) string {
		t.Helper()
		a, e := actionsRestUsual(t)
		key := newKey()
		set := func(r *playv1.AdjustCombatantHitPointsRequest) { r.IdempotencyKey = key; damageHP(3)(r) }
		if _, err := a.adjustHP(t, e, "Goblin", set); err != nil {
			t.Fatalf("AdjustCombatantHitPoints() error = %v", err)
		}
		before := a.afterSetup(t)
		_, err := a.adjustHP(t, e, "Goblin", set)
		head := "ok"
		if err != nil {
			head = "refused " + takeActionRefusalText(err)
		}
		return a.actionsRestReport(t, before, head)
	}}
}
