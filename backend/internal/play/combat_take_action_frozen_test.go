package play

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// These tests freeze what TakeAction does today, scenario by scenario: every refusal with its code, message
// and blocked reason, and for each action that goes through the answer, the events it writes (in order, the
// combatants named by label), the combatants' economy and turn flags, the states it begins and the resources
// it spends. They are the net under the split of TakeAction into one function for each kind of action.
// testdata/take_action.golden holds what each scenario reads; run with UPDATE_GOLDEN=1 to write the file
// again, and read the diff.

const takeActionGolden = "testdata/take_action.golden"

// takeActionScenario is one scenario of the golden: run plays it on a fight of its own and reads the outcome.
type takeActionScenario struct {
	name string
	run  func(t *testing.T) string
}

// takeActionFight starts a fight and hands back the campaign and the combat.
type takeActionFight func(t *testing.T) (*armed, *playv1.Encounter)

// takeActionCase is a scenario that prepares a fight (prep), then takes one action as one user: who acts, as
// plays it (Toren's player when empty), key is the action and edit fills the rest of the request.
type takeActionCase struct {
	name  string
	fight takeActionFight
	prep  func(t *testing.T, a *armed, e *playv1.Encounter)
	who   string
	as    func(a *armed) *user
	key   string
	edit  func(*playv1.TakeActionRequest)
}

// takeActionPlayerOf is the user that plays a hero by default.
func takeActionPlayerOf(a *armed, who string) *user {
	switch who {
	case "Pensantus":
		return a.ana
	case "Brisa":
		return a.bia
	case "Toren", "Ragna":
		return a.caio
	}
	return a.master
}

func takeActionAsMaster(a *armed) *user { return a.master }

func (c takeActionCase) scenario() takeActionScenario {
	return takeActionScenario{name: c.name, run: func(t *testing.T) string {
		t.Helper()
		fight := c.fight
		if fight == nil {
			fight = takeActionFighters
		}
		a, e := fight(t)
		if c.prep != nil {
			c.prep(t, a, e)
		}
		e = a.get(t, a.master)
		who := c.who
		if who == "" {
			who = "Toren"
		}
		u := takeActionPlayerOf(a, who)
		if c.as != nil {
			u = c.as(a)
		}
		before := a.events(t)
		res, err := a.takeActionRaw(t, u, e, who, c.key, c.edit)
		return a.takeActionReport(t, e, before, res, err)
	}}
}

// takeActionRaw calls TakeAction as u for the combatant who, with a request that edit finishes.
func (a *armed) takeActionRaw(t *testing.T, u *user, e *playv1.Encounter, who, key string, edit func(*playv1.TakeActionRequest)) (*playv1.TakeActionResponse, error) {
	t.Helper()
	req := &playv1.TakeActionRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, who), ActionKey: key, IdempotencyKey: newKey(),
	}
	if edit != nil {
		edit(req)
	}
	res, err := u.combat.TakeAction(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

// takeActionRefusalText is a refusal as text: its code, its message and, for a blocked action, the reason.
func takeActionRefusalText(err error) string {
	out := connect.CodeOf(err).String() + ": "
	var cerr *connect.Error
	if !errors.As(err, &cerr) {
		return out + err.Error()
	}
	out += cerr.Message()
	for _, d := range cerr.Details() {
		if m, derr := d.Value(); derr == nil {
			if b, ok := m.(*playv1.EncounterBlocked); ok {
				out += " [" + b.GetReason().String() + "]"
				if b.GetRecharge() != rulesv1.Recharge_RECHARGE_UNSPECIFIED {
					out += " recharge " + b.GetRecharge().String()
				}
			}
		}
	}
	return out
}

// takeActionReport is what a scenario read after its action: the answer or the refusal, the events written
// since `after`, the combatants' economy and turn flags, the states, the vitals and the pending damages.
func (a *armed) takeActionReport(t *testing.T, e *playv1.Encounter, after int, res *playv1.TakeActionResponse, err error) string {
	t.Helper()
	names := labelsOf(e)
	var lines []string
	if err != nil {
		lines = append(lines, "refused "+takeActionRefusalText(err))
	} else {
		line := "ok"
		if res.Healed != nil {
			line += fmt.Sprintf(" healed %d", res.GetHealed())
		}
		lines = append(lines, line, "roll "+jsonOfMessage(res.GetRoll(), names))
	}
	for _, l := range a.eventTrace(t, e, after) {
		lines = append(lines, "event "+l)
	}
	for _, r := range a.rowsOf(t, `SELECT label, action_used, bonus_action_used, reaction_used, dashed, disengaged, attacks_made, action_surged, extra_action_used, jump_doubled, bonus_attacks_left, COALESCE(action_attack_key, '-'), hp_current, hp_temp, defeated, hidden, movement_used_dft FROM combatants ORDER BY label`) {
		lines = append(lines, "combatant "+r)
	}
	for _, r := range a.rowsOf(t, `SELECT c.label, s.kind, s.amount, COALESCE(s.ends_phase, '-'), COALESCE(s.ends_round::TEXT, '-'), s.started_round, COALESCE(s.ends_combatant_id::TEXT, '-') FROM combatant_states s JOIN combatants c ON c.id = s.combatant_id`) {
		lines = append(lines, "state "+r)
	}
	for _, r := range a.rowsOf(t, `SELECT c.name, v.hit_points_current, v.hit_points_temporary, v.spell_slots_used::TEXT, v.pact_slots_used, v.resources_used::TEXT FROM character_vitals v JOIN characters c ON c.id = v.character_id`) {
		lines = append(lines, "vitals "+r)
	}
	for _, r := range a.rowsOf(t, `SELECT attack_key, status, COALESCE(amount, -1), damage_type FROM pending_damages`) {
		lines = append(lines, "damage "+r)
	}
	out := strings.Join(lines, "\n")
	return uuidPattern.ReplaceAllStringFunc(out, func(id string) string {
		if l, ok := names[id]; ok {
			return l
		}
		return "?"
	})
}

// takeActionFighters is the usual fight: Toren (fighter 2) acts first, next to the goblin.
func takeActionFighters(t *testing.T) (*armed, *playv1.Encounter) {
	t.Helper()
	a := newArmed(t)
	e := a.threeAndAGoblin(t)
	return a, e
}

// takeActionTheatre is the usual fight without a map.
func takeActionTheatre(t *testing.T) (*armed, *playv1.Encounter) {
	t.Helper()
	a := newArmed(t)
	e := a.theatreThree(t)
	return a, e
}

// takeActionUnrevealed is the usual fight with the goblin still hidden from the players.
func takeActionUnrevealed(t *testing.T) (*armed, *playv1.Encounter) {
	t.Helper()
	a := newArmed(t)
	e := a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: a.goblin.GetId()}},
		npcRolls: []int{3},
		players:  map[string]int32{"Toren": 18, "Pensantus": 10, "Brisa": 1},
		at:       map[string][2]int32{"Toren": {3, 3}, "Goblin": {4, 3}, "Pensantus": {10, 3}, "Brisa": {8, 8}},
	})
	return a, e
}

// takeActionSetup is the usual fight left in SETUP, before the first turn.
func takeActionSetup(t *testing.T) (*armed, *playv1.Encounter) {
	t.Helper()
	a := newArmed(t)
	e := a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: a.goblin.GetId()}},
		npcRolls: []int{3},
		players:  map[string]int32{"Toren": 18, "Pensantus": 10, "Brisa": 1},
		reveal:   []string{"Goblin"},
		at:       map[string][2]int32{"Toren": {3, 3}, "Goblin": {4, 3}, "Pensantus": {10, 3}, "Brisa": {8, 8}},
		setup:    true,
	})
	return a, e
}

// takeActionHero is a player's character of a fight made by takeActionFightOf.
type takeActionHero struct {
	class    string
	level    int32
	subclass string
	scores   *rulesv1.AbilityScores
	weapons  []string
}

// takeActionStart is how a takeActionFightWith fight begins: without a map, left in SETUP, or with a
// combatant surprised.
type takeActionStart struct {
	theatre, setup bool
	surprised      string
}

// takeActionFightOf is the usual fight with Toren and Brisa of the given classes (Pensantus stays a wizard).
func takeActionFightOf(toren, brisa takeActionHero, theatre bool) takeActionFight {
	return takeActionFightWith(toren, brisa, takeActionStart{theatre: theatre})
}

// takeActionFightWith is takeActionFightOf with the way the combat begins.
func takeActionFightWith(toren, brisa takeActionHero, st takeActionStart) takeActionFight {
	return func(t *testing.T) (*armed, *playv1.Encounter) {
		t.Helper()
		a := newArmedWith(t, func(a *armed) {
			a.toren = a.caio.heroWith(t, a.campaignID, "Toren", classLevel(toren.class, toren.level, toren.subclass), toren.scores, toren.weapons, nil, nil)
			a.pens = a.ana.hero(t, a.campaignID, "Pensantus", "class:wizard", "race:gnome", 1, abilities(10, 14, 12, 10, 8), nil, []string{fireBolt})
			a.bri = a.bia.heroWith(t, a.campaignID, "Brisa", classLevel(brisa.class, brisa.level, brisa.subclass), brisa.scores, brisa.weapons, nil, nil)
		})
		p := plan{
			npcs:     []*playv1.Participant{{CharacterId: a.goblin.GetId()}},
			npcRolls: []int{3},
			players:  map[string]int32{"Toren": 18, "Pensantus": 10, "Brisa": 1},
			reveal:   []string{"Goblin"},
			theatre:  st.theatre,
			setup:    st.setup || st.surprised != "",
		}
		if !st.theatre {
			p.at = map[string][2]int32{"Toren": {3, 3}, "Goblin": {4, 3}, "Pensantus": {10, 3}, "Brisa": {4, 4}}
		}
		e := a.start(t, p)
		if st.surprised != "" {
			if _, err := a.master.contests.SetSurprised(t.Context(), connect.NewRequest(&playv1.SetSurprisedRequest{
				CampaignId: a.campaignID, EncounterId: e.GetId(), IdempotencyKey: newKey(), CombatantId: a.id(t, st.surprised), Surprised: true,
			})); err != nil {
				t.Fatalf("SetSurprised(%s) error = %v", st.surprised, err)
			}
		}
		if st.surprised != "" && !st.setup {
			e = a.begin(t, e)
		}
		return a, e
	}
}

var (
	takeActionFighter2  = takeActionHero{class: "class:fighter", level: 2, scores: abilities(16, 13, 14, 10, 8), weapons: []string{battleaxe}}
	takeActionFighter17 = takeActionHero{class: "class:fighter", level: 17, scores: abilities(16, 13, 14, 10, 8), weapons: []string{battleaxe}}
	takeActionRogue3    = takeActionHero{class: "class:rogue", level: 3, scores: abilities(10, 16, 14, 12, 8), weapons: []string{rapier}}
	takeActionMonk4     = takeActionHero{class: "class:monk", level: 4, scores: abilities(12, 16, 14, 14, 8)}
	takeActionBarb3     = takeActionHero{class: "class:barbarian", level: 3, scores: abilities(16, 14, 14, 10, 8), weapons: []string{battleaxe}}
	takeActionBerserk3  = takeActionHero{class: "class:barbarian", level: 3, subclass: "subclass:berserker", scores: abilities(16, 14, 14, 10, 8), weapons: []string{battleaxe}}
	takeActionDruid2    = takeActionHero{class: "class:druid", level: 2, scores: abilities(10, 14, 14, 16, 8)}
	takeActionPaladin2  = takeActionHero{class: "class:paladin", level: 2, scores: abilities(16, 10, 14, 10, 14), weapons: []string{battleaxe}}
	takeActionBard2     = takeActionHero{class: "class:bard", level: 2, scores: abilities(10, 14, 14, 10, 16)}
)

// takeActionSteps runs the steps one after the other.
func takeActionSteps(steps ...func(t *testing.T, a *armed, e *playv1.Encounter)) func(t *testing.T, a *armed, e *playv1.Encounter) {
	return func(t *testing.T, a *armed, e *playv1.Encounter) {
		t.Helper()
		for _, s := range steps {
			s(t, a, e)
		}
	}
}

// takeActionFirst takes an action first, as the combatant's own player.
func takeActionFirst(who, key string, edit func(*playv1.TakeActionRequest)) func(t *testing.T, a *armed, e *playv1.Encounter) {
	return func(t *testing.T, a *armed, e *playv1.Encounter) {
		t.Helper()
		if _, err := a.takeActionRaw(t, takeActionPlayerOf(a, who), e, who, key, edit); err != nil {
			t.Fatalf("TakeAction(%s %s) before the scenario error = %v", who, key, err)
		}
	}
}

// takeActionAttackFirst makes Toren's weapon attack on the goblin, which opens a damage.
func takeActionAttackFirst(t *testing.T, a *armed, e *playv1.Encounter) {
	t.Helper()
	a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", d20(15))
}

// takeActionHurt takes hit points off an NPC, as the master.
func takeActionHurt(label string, points int32) func(t *testing.T, a *armed, e *playv1.Encounter) {
	return func(t *testing.T, a *armed, e *playv1.Encounter) {
		t.Helper()
		if _, err := a.adjustHP(t, e, label, func(r *playv1.AdjustCombatantHitPointsRequest) {
			r.Change = &playv1.AdjustCombatantHitPointsRequest_Damage{Damage: points}
		}); err != nil {
			t.Fatalf("AdjustCombatantHitPoints(%s) error = %v", label, err)
		}
	}
}

// takeActionHeroHP sets a player's hero hit points, as the master.
func takeActionHeroHP(label string, hp int32) func(t *testing.T, a *armed, e *playv1.Encounter) {
	return func(t *testing.T, a *armed, _ *playv1.Encounter) {
		t.Helper()
		hero := map[string]*charactersv1.Character{"Toren": a.toren, "Pensantus": a.pens, "Brisa": a.bri}[label]
		a.hurt(t, hero, hp)
	}
}

// takeActionHeroTemp gives Toren temporary hit points, as the master.
func takeActionHeroTemp(t *testing.T, a *armed, _ *playv1.Encounter) {
	t.Helper()
	temp := int32(5)
	if _, err := a.master.play.AdjustCharacterVitals(t.Context(), connect.NewRequest(&playv1.AdjustCharacterVitalsRequest{
		CampaignId: a.campaignID, CharacterId: a.toren.GetId(), IdempotencyKey: newKey(), HitPointsTemporary: &temp,
	})); err != nil {
		t.Fatalf("AdjustCharacterVitals() error = %v", err)
	}
}

// takeActionConditions sets a combatant's conditions, as the master.
func takeActionConditions(label string, keys ...string) func(t *testing.T, a *armed, e *playv1.Encounter) {
	return func(t *testing.T, a *armed, e *playv1.Encounter) {
		t.Helper()
		a.setConditions(t, e, label, keys...)
	}
}

// takeActionSQL runs a statement on the fight, to reach a state no call would.
func takeActionSQL(sql string) func(t *testing.T, a *armed, e *playv1.Encounter) {
	return func(t *testing.T, a *armed, _ *playv1.Encounter) {
		t.Helper()
		a.execSQL(t, sql)
	}
}

// takeActionDice queues the dice the app rolls next.
func takeActionDice(faces ...int) func(t *testing.T, a *armed, e *playv1.Encounter) {
	return func(t *testing.T, a *armed, _ *playv1.Encounter) {
		t.Helper()
		a.h.roller.queue(faces...)
	}
}

// takeActionPhysical makes Toren's player roll real dice.
func takeActionPhysical(t *testing.T, a *armed, _ *playv1.Encounter) {
	t.Helper()
	a.setPhysical(t, a.caio)
}

// takeActionPass passes the turns, as the master, until the combatant is on turn.
func takeActionPass(label string) func(t *testing.T, a *armed, e *playv1.Encounter) {
	return func(t *testing.T, a *armed, e *playv1.Encounter) {
		t.Helper()
		a.passTo(t, e, label)
	}
}

// takeActionHaste puts Haste on Toren: one extra action in the turn.
func takeActionHaste(t *testing.T, a *armed, e *playv1.Encounter) {
	t.Helper()
	a.mustAddEffect(t, e, "spell:haste", []string{"Toren"}, a.rounds(t, 3, "Toren"))
}

// takeActionRequestEdits chains edits of a request.
func takeActionRequestEdits(edits ...func(*playv1.TakeActionRequest)) func(*playv1.TakeActionRequest) {
	return func(r *playv1.TakeActionRequest) {
		for _, e := range edits {
			e(r)
		}
	}
}

func takeActionInApp(r *playv1.TakeActionRequest) {
	r.Roll = &playv1.TakeActionRequest_RollInApp{RollInApp: true}
}

func takeActionTyped(sum int32) func(*playv1.TakeActionRequest) {
	return func(r *playv1.TakeActionRequest) { r.Roll = &playv1.TakeActionRequest_TypedSum{TypedSum: sum} }
}

func takeActionExtra(r *playv1.TakeActionRequest)  { r.UseExtraAction = true }
func takeActionFrenzy(r *playv1.TakeActionRequest) { r.Frenzy = true }

// takeActionRefusals are the refusals of the request itself and of who may act and when.
func takeActionRefusals() []takeActionScenario {
	long := strings.Repeat("x", 101)
	stranger := func(a *armed) *user { return a.h.newUser("Zé") }
	cases := []takeActionCase{
		{name: "refused an empty action key", key: ""},
		{name: "refused an action key of 101 characters", key: long},
		{name: "refused an idempotency key that is not a UUID", key: "standard:dodge", edit: func(r *playv1.TakeActionRequest) { r.IdempotencyKey = "not-a-key" }},
		{name: "refused a combatant nobody knows", key: "standard:dodge", edit: func(r *playv1.TakeActionRequest) {
			r.CombatantId = "00000000-0000-4000-8000-000000000001"
		}},
		{name: "refused an encounter nobody knows", key: "standard:dodge", edit: func(r *playv1.TakeActionRequest) {
			r.EncounterId = "00000000-0000-4000-8000-000000000002"
		}},
		{name: "refused a campaign that is not a UUID", key: "standard:dodge", edit: func(r *playv1.TakeActionRequest) { r.CampaignId = "nope" }},
		{name: "refused a user who is not in the campaign", key: "standard:dodge", as: stranger},
		{name: "refused the Wild Shape action", key: "feature:wild-shape"},
		{name: "refused the Wild Shape action of a level", key: "feature:wild-shape-1"},
		{name: "refused Lay on Hands", key: "feature:lay-on-hands"},
		{name: "refused creating a spell slot", key: "feature:flexible-casting-creating-spell-slots"},
		{name: "refused converting a spell slot", key: "feature:flexible-casting-converting-spell-slot"},
		{name: "refused Bardic Inspiration", key: "feature:bardic-inspiration-d6"},
		{name: "refused an action that does not exist", key: "standard:fly"},
		{name: "refused a key with no kind", key: "dodge"},
		{name: "refused a feature the hero does not have", key: "feature:rage"},
		{name: "refused a trait the hero does not have", key: "trait:nimble-escape"},
		{name: "refused Hide, which has its own call", key: "standard:hide"},
		{name: "refused Help, which has its own call", key: "standard:help"},
		{name: "refused the extra action for a feature", key: "feature:second-wind", edit: takeActionExtra},
		{name: "refused the extra action without Haste", key: "standard:dash", edit: takeActionExtra},
		{name: "refused the extra action for the master without Haste", key: "standard:dash", edit: takeActionExtra, as: takeActionAsMaster},
		{name: "refused another player's combatant", key: "standard:dodge", who: "Brisa", as: func(a *armed) *user { return a.caio }},
		{name: "refused a combatant out of turn", key: "standard:dodge", who: "Brisa"},
		{name: "refused a combatant out of turn for the master", key: "standard:dodge", who: "Brisa", as: takeActionAsMaster},
		{name: "refused a hidden NPC the player does not see", key: "standard:dodge", who: "Goblin", fight: takeActionUnrevealed},
		{name: "refused a combat that did not begin", key: "standard:dodge", fight: takeActionSetup},
		{name: "refused a combatant at 0 hit points", key: "standard:dodge", prep: takeActionHeroHP("Toren", 0)},
		{name: "refused a defeated NPC", key: "standard:dodge", who: "Goblin", as: takeActionAsMaster, prep: takeActionHurt("Goblin", 200)},
		{name: "refused an NPC defeated in its own turn", key: "standard:dodge", who: "Goblin", as: takeActionAsMaster, prep: takeActionSteps(takeActionPass("Goblin"), takeActionHurt("Goblin", 200))},
		{name: "refused a hidden NPC on its turn to a player", key: "standard:dodge", who: "Goblin", as: func(a *armed) *user { return a.caio }, fight: takeActionUnrevealed, prep: takeActionPass("Goblin")},
		{name: "refused a revealed NPC on its turn to a player", key: "standard:dodge", who: "Goblin", as: func(a *armed) *user { return a.caio }, prep: takeActionPass("Goblin")},
		{name: "refused an incapacitated combatant", key: "standard:dodge", prep: takeActionConditions("Toren", "condition:incapacitated")},
		{name: "refused a paralyzed combatant", key: "standard:dodge", prep: takeActionConditions("Toren", "condition:paralyzed")},
		{name: "refused a stunned combatant", key: "standard:dodge", prep: takeActionConditions("Toren", "condition:stunned")},
		{name: "refused an unconscious combatant", key: "standard:dodge", prep: takeActionConditions("Toren", "condition:unconscious")},
		{name: "refused a petrified combatant", key: "standard:dodge", prep: takeActionConditions("Toren", "condition:petrified")},
		{name: "refused a combatant with no action by an effect", key: "standard:dodge", prep: takeActionSQL(`UPDATE combatants SET effect_no_action = true WHERE label = 'Toren'`)},
		{name: "refused a feature of a combatant with no action by an effect", key: "feature:second-wind", edit: takeActionInApp, prep: takeActionSQL(`UPDATE combatants SET effect_no_action = true WHERE label = 'Toren'`)},
		{name: "refused while a question waits for the master", key: "standard:dodge", prep: takeActionSQL(`INSERT INTO hidden_reveals (encounter_id, caster_id, spell_key, combatant_ids, origin_col, origin_row, seq, created_at) SELECT encounter_id, id, 'spell:fire-bolt', ARRAY[]::TEXT[], 4, 3, 1, now() FROM combatants WHERE label = 'Pensantus'`)},
		{name: "refused for the master while a question waits for the master", key: "standard:dodge", as: takeActionAsMaster, prep: takeActionSQL(`INSERT INTO hidden_reveals (encounter_id, caster_id, spell_key, combatant_ids, origin_col, origin_row, seq, created_at) SELECT encounter_id, id, 'spell:fire-bolt', ARRAY[]::TEXT[], 4, 3, 1, now() FROM combatants WHERE label = 'Pensantus'`)},
		{name: "refused a surprised combatant", key: "standard:dodge", fight: takeActionFightWith(takeActionFighter2, takeActionFighter2, takeActionStart{surprised: "Toren"})},
		{name: "a Dodge with a Frenzy that is only for a rage", key: "standard:dodge", edit: takeActionFrenzy},
		{name: "a Dodge with a typed sum that it does not use", key: "standard:dodge", edit: takeActionTyped(4)},
		{name: "a Dodge with the app's roll that it does not use", key: "standard:dodge", edit: takeActionInApp},
		{name: "a poisoned and a prone combatant still acts", key: "standard:dodge", prep: takeActionConditions("Toren", "condition:poisoned", "condition:prone")},
	}
	out := make([]takeActionScenario, 0, len(cases))
	for _, c := range cases {
		out = append(out, c.scenario())
	}
	return out
}

// takeActionStandards are the standard actions: who takes them, when, and what each does to the turn.
func takeActionStandards() []takeActionScenario {
	cases := []takeActionCase{
		{name: "Dodge", key: "standard:dodge"},
		{name: "Dash", key: "standard:dash"},
		{name: "Disengage", key: "standard:disengage"},
		{name: "Search", key: "standard:search"},
		{name: "Ready", key: "standard:ready"},
		{name: "Use an Object", key: "standard:use-an-object"},
		{name: "Dodge as the master for the player's hero", key: "standard:dodge", as: takeActionAsMaster},
		{name: "Dash as the master for the player's hero", key: "standard:dash", as: takeActionAsMaster},
		{name: "Dodge in a combat without a map", key: "standard:dodge", fight: takeActionTheatre},
		{name: "Dash in a combat without a map", key: "standard:dash", fight: takeActionTheatre},
		{name: "Disengage in a combat without a map", key: "standard:disengage", fight: takeActionTheatre},
		{name: "Dodge after the Attack action", key: "standard:dodge", prep: takeActionAttackFirst},
		{name: "Dash after the Attack action", key: "standard:dash", prep: takeActionAttackFirst},
		{name: "a second Dodge in the same turn", key: "standard:dodge", prep: takeActionFirst("Toren", "standard:dodge", nil)},
		{name: "a Dash after a Dodge", key: "standard:dash", prep: takeActionFirst("Toren", "standard:dodge", nil)},
		{name: "a Disengage after a Dash", key: "standard:disengage", prep: takeActionFirst("Toren", "standard:dash", nil)},
		{name: "a second Dash after a Dash", key: "standard:dash", prep: takeActionFirst("Toren", "standard:dash", nil)},
		{name: "a second Dodge as the master", key: "standard:dodge", as: takeActionAsMaster, prep: takeActionFirst("Toren", "standard:dodge", nil)},
		{name: "a second Dash as the master", key: "standard:dash", as: takeActionAsMaster, prep: takeActionFirst("Toren", "standard:dash", nil)},
		{name: "a Search after a Search as the master", key: "standard:search", as: takeActionAsMaster, prep: takeActionFirst("Toren", "standard:search", nil)},
		{name: "a Search after an Attack", key: "standard:search", prep: takeActionAttackFirst},
		{name: "Dodge by an NPC as the master", key: "standard:dodge", who: "Goblin", prep: takeActionPass("Goblin")},
		{name: "Dash by an NPC as the master", key: "standard:dash", who: "Goblin", prep: takeActionPass("Goblin")},
		{name: "Disengage by an NPC as the master", key: "standard:disengage", who: "Goblin", prep: takeActionPass("Goblin")},
		{name: "Search by an NPC as the master", key: "standard:search", who: "Goblin", prep: takeActionPass("Goblin")},
		{name: "Dodge by a hidden NPC as the master", key: "standard:dodge", who: "Goblin", fight: takeActionUnrevealed, prep: takeActionPass("Goblin")},
		{name: "Dodge by Pensantus on her turn", key: "standard:dodge", who: "Pensantus", prep: takeActionPass("Pensantus")},
		{name: "Dodge by a player's hero below full hit points", key: "standard:dodge", prep: takeActionHeroHP("Toren", 5)},
		{name: "Dodge of a combatant that was Dodging last round", key: "standard:dodge", prep: takeActionSteps(
			takeActionFirst("Toren", "standard:dodge", nil), takeActionPass("Toren"))},
	}
	out := make([]takeActionScenario, 0, len(cases)+4)
	for _, c := range cases {
		out = append(out, c.scenario())
	}
	return out
}

// takeActionHasteCases are the extra action of Haste: which standard actions it takes and when.
func takeActionHasteCases() []takeActionScenario {
	cases := []takeActionCase{
		{name: "Dash with the extra action", key: "standard:dash", edit: takeActionExtra, prep: takeActionHaste},
		{name: "Disengage with the extra action", key: "standard:disengage", edit: takeActionExtra, prep: takeActionHaste},
		{name: "Use an Object with the extra action", key: "standard:use-an-object", edit: takeActionExtra, prep: takeActionHaste},
		{name: "Dodge with the extra action, which Haste does not allow", key: "standard:dodge", edit: takeActionExtra, prep: takeActionHaste},
		{name: "Search with the extra action, which Haste does not allow", key: "standard:search", edit: takeActionExtra, prep: takeActionHaste},
		{name: "Hide with the extra action", key: "standard:hide", edit: takeActionExtra, prep: takeActionHaste},
		{name: "the extra action twice", key: "standard:dash", edit: takeActionExtra, prep: takeActionSteps(takeActionHaste, takeActionFirst("Toren", "standard:dash", takeActionExtra))},
		{name: "the extra action with the action already used", key: "standard:dash", edit: takeActionExtra, prep: takeActionSteps(takeActionHaste, takeActionFirst("Toren", "standard:dodge", nil))},
		{name: "the action after the extra action", key: "standard:dodge", prep: takeActionSteps(takeActionHaste, takeActionFirst("Toren", "standard:dash", takeActionExtra))},
		{name: "the extra action as the master", key: "standard:dash", edit: takeActionExtra, as: takeActionAsMaster, prep: takeActionHaste},
		{name: "the extra action of Haste after a lethargy", key: "standard:dash", edit: takeActionExtra, prep: takeActionSteps(takeActionHaste, takeActionSQL(`UPDATE combatants SET effect_no_action = true WHERE label = 'Toren'`))},
		{name: "the extra action already marked used", key: "standard:dash", edit: takeActionExtra, prep: takeActionSteps(takeActionHaste, takeActionSQL(`UPDATE combatants SET extra_action_used = true WHERE label = 'Toren'`))},
		{name: "the extra action for a feature with Haste", key: "feature:second-wind", edit: takeActionRequestEdits(takeActionExtra, takeActionInApp), prep: takeActionHaste},
		{name: "the extra action in a combat without a map", key: "standard:dash", edit: takeActionExtra, fight: takeActionTheatre, prep: takeActionHaste},
	}
	out := make([]takeActionScenario, 0, len(cases))
	for _, c := range cases {
		out = append(out, c.scenario())
	}
	return out
}

// takeActionOnce is a scenario that plays an own fight and reads what it hands back.
func takeActionOnce(name string, play func(t *testing.T) (*armed, *playv1.Encounter, int, *playv1.TakeActionResponse, error)) takeActionScenario {
	return takeActionScenario{name: name, run: func(t *testing.T) string {
		t.Helper()
		a, e, before, res, err := play(t)
		return a.takeActionReport(t, e, before, res, err)
	}}
}

// takeActionReplays are the retries of one request: the same key is the same answer, and a different body
// with the same key is a conflict.
func takeActionReplays() []takeActionScenario {
	return []takeActionScenario{
		takeActionOnce("the same request twice", func(t *testing.T) (*armed, *playv1.Encounter, int, *playv1.TakeActionResponse, error) {
			t.Helper()
			a, e := takeActionFighters(t)
			key := newKey()
			set := func(r *playv1.TakeActionRequest) { r.IdempotencyKey = key }
			if _, err := a.takeActionRaw(t, a.caio, e, "Toren", "standard:dash", set); err != nil {
				t.Fatalf("TakeAction() error = %v", err)
			}
			before := a.events(t)
			res, err := a.takeActionRaw(t, a.caio, e, "Toren", "standard:dash", set)
			return a, e, before, res, err
		}),
		takeActionOnce("the same key with another action", func(t *testing.T) (*armed, *playv1.Encounter, int, *playv1.TakeActionResponse, error) {
			t.Helper()
			a, e := takeActionFighters(t)
			key := newKey()
			set := func(r *playv1.TakeActionRequest) { r.IdempotencyKey = key }
			if _, err := a.takeActionRaw(t, a.caio, e, "Toren", "standard:dash", set); err != nil {
				t.Fatalf("TakeAction() error = %v", err)
			}
			before := a.events(t)
			res, err := a.takeActionRaw(t, a.caio, e, "Toren", "standard:dodge", set)
			return a, e, before, res, err
		}),
		takeActionOnce("the same key from another user", func(t *testing.T) (*armed, *playv1.Encounter, int, *playv1.TakeActionResponse, error) {
			t.Helper()
			a, e := takeActionFighters(t)
			key := newKey()
			set := func(r *playv1.TakeActionRequest) { r.IdempotencyKey = key }
			if _, err := a.takeActionRaw(t, a.caio, e, "Toren", "standard:dash", set); err != nil {
				t.Fatalf("TakeAction() error = %v", err)
			}
			before := a.events(t)
			res, err := a.takeActionRaw(t, a.master, e, "Toren", "standard:dash", set)
			return a, e, before, res, err
		}),
		takeActionOnce("the same Second Wind twice", func(t *testing.T) (*armed, *playv1.Encounter, int, *playv1.TakeActionResponse, error) {
			t.Helper()
			a, e := takeActionFighters(t)
			a.h.roller.queue(6)
			takeActionHeroHP("Toren", 8)(t, a, e)
			key := newKey()
			set := func(r *playv1.TakeActionRequest) { r.IdempotencyKey = key; takeActionInApp(r) }
			if _, err := a.takeActionRaw(t, a.caio, e, "Toren", "feature:second-wind", set); err != nil {
				t.Fatalf("TakeAction() error = %v", err)
			}
			before := a.events(t)
			res, err := a.takeActionRaw(t, a.caio, e, "Toren", "feature:second-wind", set)
			return a, e, before, res, err
		}),
	}
}

// takeActionScenarios are the scenarios of the golden, in the order they are listed.
func takeActionScenarios() []takeActionScenario {
	var all []takeActionScenario
	all = append(all, takeActionRefusals()...)
	all = append(all, takeActionStandards()...)
	all = append(all, takeActionHasteCases()...)
	all = append(all, takeActionReplays()...)
	all = append(all, takeActionFeatureScenarios()...)
	all = append(all, takeActionWorldScenarios()...)
	return all
}

// TestTakeActionIsFrozen runs every scenario on a fight of its own and compares what each reads with the golden.
//
//nolint:tparallel // the group waits for its parallel scenarios, so that the golden is settled after them.
func TestTakeActionIsFrozen(t *testing.T) {
	t.Parallel()
	g := &goldenBlocks{}
	t.Run("scenarios", func(t *testing.T) {
		for _, sc := range takeActionScenarios() {
			t.Run(sc.name, func(t *testing.T) {
				t.Parallel()
				g.put(sc.name, sc.run(t))
			})
		}
	})
	g.settle(t, takeActionGolden)
}
