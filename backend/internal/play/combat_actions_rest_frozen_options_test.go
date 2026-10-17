package play

import (
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The scenarios of the golden for the read of a combatant's turn: GetTurnOptions, by viewer, by blocked
// reason and by kind of option, and the spells' targets.

// actionsRestOption is a scenario that prepares a fight and reads the turn options of a combatant (Toren by
// default) as a user (his player by default): the whole answer, or the refusal, and the events the read wrote.
type actionsRestOption struct {
	name  string
	world actionsRestWorld
	prep  func(t *testing.T, a *armed, e *playv1.Encounter)
	who   string
	as    func(a *armed) *user
	edit  func(*playv1.GetTurnOptionsRequest)
}

func (c actionsRestOption) scenario() actionsRestScenario {
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
		who := c.who
		if who == "" {
			who = "Toren"
		}
		as := c.as
		if as == nil {
			as = asToren
		}
		before := a.afterSetup(t)
		req := &playv1.GetTurnOptionsRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, who)}
		if c.edit != nil {
			c.edit(req)
		}
		res, err := as(a).combat.GetTurnOptions(t.Context(), connect.NewRequest(req))
		head := "options"
		if err != nil {
			head = "refused " + takeActionRefusalText(err)
		} else {
			head += " " + jsonOfMessage(res.Msg, labelsOf(e))
		}
		return a.actionsRestReport(t, before, head)
	}}
}

// actionsRestCasters is the fight of casters: Pensantus (wizard 3) acts first, with the Capitão and a goblin.
func actionsRestCasters(t *testing.T) (*armed, *playv1.Encounter) {
	t.Helper()
	a := newCasters(t)
	e := a.castersFight(t, 1)
	return a, e
}

// actionsRestCastersAt is actionsRestCasters with the combatant on turn.
func actionsRestCastersAt(label string) actionsRestWorld {
	return func(t *testing.T) (*armed, *playv1.Encounter) {
		t.Helper()
		a, e := actionsRestCasters(t)
		e = a.passTo(t, e, label)
		return a, e
	}
}

// actionsRestPack is the fight of casters with three goblins, as in the area spells.
func actionsRestPack(t *testing.T) (*armed, *playv1.Encounter) {
	t.Helper()
	a := newCasters(t)
	e := a.castersFight(t, 2)
	a.put(t, "Goblin 1", 7, 5)
	a.put(t, "Capitão Goblin", 8, 6)
	return a, e
}

// actionsRestTheatreCasters is the fight of casters without a map.
func actionsRestTheatreCasters(t *testing.T) (*armed, *playv1.Encounter) {
	t.Helper()
	a := newCasters(t)
	e := a.theatreCasters(t, 1)
	return a, e
}

// actionsRestSniper is the fight of the sniper table, with Spell Sniper for Pensantus and half cover on the
// goblin, 150 ft away.
func actionsRestSniper(feat bool) actionsRestWorld {
	return func(t *testing.T) (*armed, *playv1.Encounter) {
		t.Helper()
		s := newSniperTable(t)
		if feat {
			s.giveFeat(t, s.pens, s.spellSniper)
		}
		e := s.fight(t)
		s.markHalf(t, "Goblin")
		return s.armed, e
	}
}

// actionsRestSpent spends the spell slots of Pensantus, all but the given ones used.
func actionsRestSpent(t *testing.T, a *armed, _ *playv1.Encounter) {
	t.Helper()
	a.execSQL(t, `UPDATE character_vitals SET spell_slots_used = ARRAY[4, 2, 0, 0, 0, 0, 0, 0, 0] WHERE character_id = $1`, a.pens.GetId())
}

// actionsRestPut places a combatant.
func actionsRestPut(label string, col, row int32) func(t *testing.T, a *armed, e *playv1.Encounter) {
	return func(t *testing.T, a *armed, _ *playv1.Encounter) {
		t.Helper()
		a.put(t, label, col, row)
	}
}

// actionsRestHide hides a combatant from the players.
func actionsRestHide(label string) func(t *testing.T, a *armed, e *playv1.Encounter) {
	return func(t *testing.T, a *armed, e *playv1.Encounter) {
		t.Helper()
		setHidden(t, a, e, label, true)
	}
}

// actionsRestConditioned sets a combatant's conditions.
func actionsRestConditioned(label string, keys ...string) func(t *testing.T, a *armed, e *playv1.Encounter) {
	return func(t *testing.T, a *armed, e *playv1.Encounter) {
		t.Helper()
		a.setConditions(t, e, label, keys...)
	}
}

// actionsRestFirst runs the prepares one after the other.
func actionsRestFirst(preps ...func(t *testing.T, a *armed, e *playv1.Encounter)) func(t *testing.T, a *armed, e *playv1.Encounter) {
	return func(t *testing.T, a *armed, e *playv1.Encounter) {
		t.Helper()
		for _, p := range preps {
			p(t, a, e)
		}
	}
}

// actionsRestExec runs a statement on the fight.
func actionsRestExec(sql string) func(t *testing.T, a *armed, e *playv1.Encounter) {
	return func(t *testing.T, a *armed, _ *playv1.Encounter) {
		t.Helper()
		a.execSQL(t, sql)
	}
}

// actionsRestAct takes an action as the combatant's player (the master when Toren is not the one).
func actionsRestAct(who, key string, u func(a *armed) *user) func(t *testing.T, a *armed, e *playv1.Encounter) {
	return func(t *testing.T, a *armed, e *playv1.Encounter) {
		t.Helper()
		if _, err := a.action(t, u(a), e, who, key); err != nil {
			t.Fatalf("TakeAction(%s %s) error = %v", who, key, err)
		}
	}
}

// actionsRestAttack makes Toren's attack on the goblin, which leaves a damage waiting.
func actionsRestAttack(t *testing.T, a *armed, e *playv1.Encounter) {
	t.Helper()
	a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", d20(15))
}

// actionsRestOptionRefusals are the refusals of the request itself and of who may read what.
func actionsRestOptionRefusals() []actionsRestOption {
	stranger := func(a *armed) *user { return a.h.newUser("Zé") }
	return []actionsRestOption{
		{name: "options refused a campaign that is not a UUID", edit: func(r *playv1.GetTurnOptionsRequest) { r.CampaignId = "nope" }},
		{name: "options refused a combatant that is not a UUID", edit: func(r *playv1.GetTurnOptionsRequest) { r.CombatantId = "nope" }},
		{name: "options refused an encounter nobody knows", edit: func(r *playv1.GetTurnOptionsRequest) { r.EncounterId = "00000000-0000-4000-8000-000000000002" }},
		{name: "options refused a combatant nobody knows", edit: func(r *playv1.GetTurnOptionsRequest) { r.CombatantId = "00000000-0000-4000-8000-000000000001" }},
		{name: "options refused a user who is not in the campaign", as: stranger},
		{name: "options refused another player's hero", as: asDamageAna},
		{name: "options refused another player's hero to a third player", who: "Pensantus", as: func(a *armed) *user { return a.bia }},
		{name: "options refused a revealed NPC to a player", who: "Goblin"},
		{name: "options refused a hidden NPC to a player", who: "Goblin", world: takeActionUnrevealed},
		{name: "options refused a hidden NPC to a player on its turn", who: "Goblin", world: takeActionUnrevealed, prep: actionsRestPass("Goblin")},
		{name: "options refused a combatant of a combat that ended", prep: func(t *testing.T, a *armed, e *playv1.Encounter) {
			t.Helper()
			endEncounter(t, a, e)
		}},
	}
}

// actionsRestPass passes the turns, as the master, until the combatant is on turn.
func actionsRestPass(label string) func(t *testing.T, a *armed, e *playv1.Encounter) {
	return func(t *testing.T, a *armed, e *playv1.Encounter) {
		t.Helper()
		a.passTo(t, e, label)
	}
}

// actionsRestOptionViews are the same turn read by each viewer.
func actionsRestOptionViews() []actionsRestOption {
	return []actionsRestOption{
		{name: "options of Toren on his turn to his player"},
		{name: "options of Toren on his turn to the master", as: asMaster},
		{name: "options of Pensantus out of turn to her player", who: "Pensantus", as: asDamageAna},
		{name: "options of Pensantus out of turn to the master", who: "Pensantus", as: asMaster},
		{name: "options of Brisa out of turn to her player", who: "Brisa", as: func(a *armed) *user { return a.bia }},
		{name: "options of the goblin to the master out of turn", who: "Goblin", as: asMaster},
		{name: "options of the goblin to the master on its turn", who: "Goblin", as: asMaster, prep: actionsRestPass("Goblin")},
		{name: "options of Toren to the master out of turn", prep: actionsRestPass("Pensantus"), as: asMaster},
		{name: "options of a hidden goblin to the master", who: "Goblin", world: takeActionUnrevealed, as: asMaster, prep: actionsRestPass("Goblin")},
		{name: "options of Toren with a hidden goblin to his player", world: takeActionUnrevealed},
		{name: "options of Toren with a hidden goblin to the master", world: takeActionUnrevealed, as: asMaster},
		{name: "options of Toren with the goblin hidden after the attack to his player", prep: actionsRestFirst(actionsRestAttack, actionsRestHide("Goblin"))},
		{name: "options of Toren with the goblin hidden after the attack to the master", as: asMaster, prep: actionsRestFirst(actionsRestAttack, actionsRestHide("Goblin"))},
		{name: "options of Toren with a damage waiting to his player", prep: actionsRestAttack},
		{name: "options of Toren with a damage waiting to the master", as: asMaster, prep: actionsRestAttack},
		{name: "options of Toren in a combat without a map to his player", world: takeActionTheatre},
		{name: "options of Toren in a combat without a map to the master", world: takeActionTheatre, as: asMaster},
		{name: "options of Pensantus in a combat without a map", world: actionsRestTheatreCasters, who: "Pensantus", as: asDamageAna},
		{name: "options of Pensantus in a combat without a map to the master", world: actionsRestTheatreCasters, who: "Pensantus", as: asMaster},
	}
}

// actionsRestOptionBlocks are the reasons that disable the options of a combatant, and the economy used.
func actionsRestOptionBlocks() []actionsRestOption {
	return []actionsRestOption{
		{name: "options of a combat that did not begin", world: takeActionSetup},
		{name: "options of a combat that did not begin to the master", world: takeActionSetup, as: asMaster},
		{name: "options of a defeated goblin to the master", who: "Goblin", as: asMaster, prep: actionsRestFirst(actionsRestPass("Goblin"), actionsRestHurt("Goblin", 200))},
		{name: "options of a goblin defeated out of turn to the master", who: "Goblin", as: asMaster, prep: actionsRestHurt("Goblin", 200)},
		{name: "options of Toren at 0 hit points", prep: func(t *testing.T, a *armed, _ *playv1.Encounter) {
			t.Helper()
			a.correct(t, a.toren, hpIs(0))
		}},
		{name: "options of Toren incapacitated", prep: actionsRestConditioned("Toren", "condition:incapacitated")},
		{name: "options of Toren paralyzed", prep: actionsRestConditioned("Toren", "condition:paralyzed")},
		{name: "options of Toren stunned", prep: actionsRestConditioned("Toren", "condition:stunned")},
		{name: "options of Toren unconscious", prep: actionsRestConditioned("Toren", "condition:unconscious")},
		{name: "options of Toren petrified", prep: actionsRestConditioned("Toren", "condition:petrified")},
		{name: "options of Toren prone and poisoned", prep: actionsRestConditioned("Toren", "condition:prone", "condition:poisoned")},
		{name: "options of Toren frightened of the goblin", prep: actionsRestConditioned("Toren", "condition:frightened")},
		{name: "options of Toren restrained", prep: actionsRestConditioned("Toren", "condition:restrained")},
		{name: "options of Toren blinded", prep: actionsRestConditioned("Toren", "condition:blinded")},
		{name: "options of the goblin restrained to the master", who: "Goblin", as: asMaster, prep: actionsRestConditioned("Goblin", "condition:restrained")},
		{name: "options of Toren with no action by an effect", prep: actionsRestExec(`UPDATE combatants SET effect_no_action = true WHERE label = 'Toren'`)},
		{name: "options of Toren surprised", world: actionsRestWorld(takeActionFightWith(takeActionFighter2, takeActionFighter2, takeActionStart{surprised: "Toren"}))},
		{name: "options of Toren after a Dodge", prep: actionsRestAct("Toren", "standard:dodge", asToren)},
		{name: "options of Toren after a Dash", prep: actionsRestAct("Toren", "standard:dash", asToren)},
		{name: "options of Toren after a Disengage", prep: actionsRestAct("Toren", "standard:disengage", asToren)},
		{name: "options of Toren after the Second Wind", prep: func(t *testing.T, a *armed, e *playv1.Encounter) {
			t.Helper()
			if _, err := a.takeActionRaw(t, a.caio, e, "Toren", "feature:second-wind", takeActionInApp); err != nil {
				t.Fatalf("TakeAction(Second Wind) error = %v", err)
			}
		}},
		{name: "options of Toren after an attack", prep: actionsRestAttack},
		{name: "options of Toren with the reaction used", prep: actionsRestExec(`UPDATE combatants SET reaction_used = true WHERE label = 'Toren'`)},
		{name: "options of Toren with the action and the bonus action used", prep: actionsRestExec(`UPDATE combatants SET action_used = true, bonus_action_used = true WHERE label = 'Toren'`)},
		{name: "options of Toren with Haste", prep: func(t *testing.T, a *armed, e *playv1.Encounter) {
			t.Helper()
			takeActionHaste(t, a, e)
		}},
		{name: "options of Toren with the extra action used", prep: actionsRestFirst(func(t *testing.T, a *armed, e *playv1.Encounter) {
			t.Helper()
			takeActionHaste(t, a, e)
		}, actionsRestExec(`UPDATE combatants SET extra_action_used = true WHERE label = 'Toren'`))},
		{name: "options of Toren while a question waits for the master", prep: actionsRestExec(`INSERT INTO hidden_reveals (encounter_id, caster_id, spell_key, combatant_ids, origin_col, origin_row, seq, created_at) SELECT encounter_id, id, 'spell:fire-bolt', ARRAY[]::TEXT[], 4, 3, 1, now() FROM combatants WHERE label = 'Pensantus'`)},
		{name: "options of Toren with the dice rolled for real", prep: func(t *testing.T, a *armed, _ *playv1.Encounter) {
			t.Helper()
			a.setPhysical(t, a.caio)
		}},
		{name: "options of Toren with a rule of the table", prep: func(t *testing.T, a *armed, _ *playv1.Encounter) {
			t.Helper()
			a.setRules(t, criticalIs(maxRoll))
		}},
	}
}

// actionsRestOptionSpells are the casters' options, with the targets of each spell.
func actionsRestOptionSpells() []actionsRestOption {
	pens := func(o actionsRestOption) actionsRestOption {
		if o.world == nil {
			o.world = actionsRestCasters
		}
		if o.as == nil {
			o.as = asDamageAna
		}
		o.who = "Pensantus"
		return o
	}
	rows := []actionsRestOption{
		{name: "options of Pensantus to her player"},
		{name: "options of Pensantus to the master", as: asMaster},
		{name: "options of Pensantus with the Goblin far away", prep: actionsRestPut("Goblin", 19, 9)},
		{name: "options of Pensantus with a hidden goblin to her player", prep: actionsRestHide("Goblin")},
		{name: "options of Pensantus with a hidden goblin to the master", as: asMaster, prep: actionsRestHide("Goblin")},
		{name: "options of Pensantus with a defeated goblin", prep: actionsRestHurt("Goblin", 100)},
		{name: "options of Pensantus with no slots left", prep: actionsRestSpent},
		{name: "options of Pensantus with the goblin next to her", prep: actionsRestPut("Goblin", 5, 8)},
		{name: "options of Pensantus with the Capitão too far", prep: actionsRestPut("Capitão Goblin", 19, 1)},
		{name: "options of Pensantus with the goblin restrained", prep: actionsRestConditioned("Goblin", "condition:restrained")},
		{name: "options of Pensantus with the goblin prone", prep: actionsRestConditioned("Goblin", "condition:prone")},
		{name: "options of Pensantus blinded", prep: actionsRestConditioned("Pensantus", "condition:blinded")},
		{name: "options of Pensantus after her action", prep: actionsRestAct("Pensantus", "standard:dodge", asDamageAna)},
		{name: "options of Pensantus with a damage waiting", prep: func(t *testing.T, a *armed, e *playv1.Encounter) {
			t.Helper()
			a.mustCast(t, a.ana, e, "Pensantus", "spell:magic-missile", slotOfLevel(1), []*playv1.SpellTarget{darts(a, t, "Goblin", 3)}, noCastRoll)
		}},
		{name: "options of Pensantus concentrating", prep: func(t *testing.T, a *armed, _ *playv1.Encounter) {
			t.Helper()
			a.concentrate(t, "Pensantus", webSpell)
		}},
		{name: "options of Pensantus in a pack of goblins", world: actionsRestPack},
		{name: "options of Pensantus in a pack of goblins to the master", world: actionsRestPack, as: asMaster},
		{name: "options of Pensantus in a combat without a map to her player", world: actionsRestTheatreCasters},
		{name: "options of Pensantus in the sniper table without the feat", world: actionsRestSniper(false)},
		{name: "options of Pensantus in the sniper table with the feat", world: actionsRestSniper(true)},
	}
	out := make([]actionsRestOption, 0, len(rows)+8)
	for _, r := range rows {
		out = append(out, pens(r))
	}
	brisa := func(name string, edit func(o *actionsRestOption)) actionsRestOption {
		o := actionsRestOption{name: name, world: actionsRestCastersAt("Brisa"), who: "Brisa", as: func(a *armed) *user { return a.bia }}
		if edit != nil {
			edit(&o)
		}
		return o
	}
	return append(out,
		brisa("options of Brisa to her player", nil),
		brisa("options of Brisa to the master", func(o *actionsRestOption) { o.as = asMaster }),
		brisa("options of Brisa with Toren hurt", func(o *actionsRestOption) { o.prep = actionsRestToren0 }),
		brisa("options of Brisa with the Capitão near", func(o *actionsRestOption) { o.prep = actionsRestPut("Capitão Goblin", 4, 6) }),
		brisa("options of Brisa with the goblin far away", func(o *actionsRestOption) { o.prep = actionsRestPut("Goblin", 19, 9) }),
		brisa("options of Brisa with the Capitão hidden", func(o *actionsRestOption) { o.prep = actionsRestHide("Capitão Goblin") }),
		brisa("options of Brisa with her spells spent", func(o *actionsRestOption) {
			o.prep = actionsRestExec(`UPDATE character_vitals SET spell_slots_used = ARRAY[4, 2, 2, 0, 0, 0, 0, 0, 0]`)
		}),
		brisa("options of Brisa in a pack of goblins", func(o *actionsRestOption) { o.world = actionsRestPackAt("Brisa") }),
	)
}

// actionsRestToren0 drops Toren to 0 hit points.
func actionsRestToren0(t *testing.T, a *armed, _ *playv1.Encounter) {
	t.Helper()
	a.correct(t, a.toren, hpIs(0))
}

// actionsRestPackAt is the pack of goblins with the combatant on turn.
func actionsRestPackAt(label string) actionsRestWorld {
	return func(t *testing.T) (*armed, *playv1.Encounter) {
		t.Helper()
		a, e := actionsRestPack(t)
		e = a.passTo(t, e, label)
		return a, e
	}
}

// actionsRestOptionScenarios are the options of the golden, in the order they are listed.
func actionsRestOptionScenarios() []actionsRestScenario {
	var rows []actionsRestOption
	rows = append(rows, actionsRestOptionRefusals()...)
	rows = append(rows, actionsRestOptionViews()...)
	rows = append(rows, actionsRestOptionBlocks()...)
	rows = append(rows, actionsRestOptionSpells()...)
	out := make([]actionsRestScenario, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.scenario())
	}
	return out
}
