package play

import (
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// A monster of a combat fights with its whole SRD stat block (SRD 5.1, "Monsters"). These tests
// need the database (MEURPG_TEST_DATABASE_URL).

// monsterFight is a theatre combat with one monster of the creature added by the master and the
// combat begun. init is the d20 the monster rolls for its initiative: the party's are 10, 5 and 1
// (plus modifiers), so 20 puts it first and 1 last.
func monsterFight(t *testing.T, creature string, init int) (*armed, *playv1.Encounter, string) {
	t.Helper()
	scores := &rulesv1.AbilityScores{Strength: 16, Dexterity: 13, Constitution: 14, Intelligence: 16, Wisdom: 10, Charisma: 8}
	a := newArmedWith(t, func(a *armed) {
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 2, scores, []string{maceKey}, nil)
		a.pens = a.ana.hero(t, a.campaignID, "Pensantus", "class:wizard", "race:gnome", 1, scores, nil, []string{poisonSpray, acidSplash})
		a.bri = a.bia.hero(t, a.campaignID, "Brisa", "class:fighter", "race:human", 2, scores, []string{rapier}, nil)
	})
	e := a.monsterSetup(t, true)
	a.h.roller.queue(init)
	res := a.mustAddMonsters(t, e, func(r *playv1.AddMonstersRequest) {
		r.CreatureKey, r.Count = creature, 1
		r.Hidden = new(false)
	})
	label := ""
	for _, c := range res.GetEncounter().GetCombatants() {
		if c.GetId() == res.GetCombatantIds()[0] {
			label = c.GetLabel()
		}
	}
	if label == "" {
		t.Fatalf("%s is not in the combat", creature)
	}
	return a, a.begin(t, a.get(t, a.master)), label
}

func (a *armed) creatureTurn(t *testing.T, u *user, e *playv1.Encounter, label string) (*playv1.CreatureTurn, error) {
	t.Helper()
	res, err := u.creatures.GetCreatureTurn(t.Context(), connect.NewRequest(&playv1.GetCreatureTurnRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, label)}))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetTurn(), nil
}

func (a *armed) useAction(t *testing.T, e *playv1.Encounter, who, action string, targets ...string) (*playv1.UseCreatureActionResponse, error) {
	t.Helper()
	req := &playv1.UseCreatureActionRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, who), ActionKey: action, IdempotencyKey: newKey()}
	for _, tg := range targets {
		req.TargetIds = append(req.TargetIds, a.id(t, tg))
	}
	res, err := a.master.creatures.UseCreatureAction(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func actionOf(turn *playv1.CreatureTurn, key string) *playv1.CreatureAction {
	for _, a := range turn.GetActions() {
		if a.GetKey() == key {
			return a
		}
	}
	return nil
}

func refusedWith(err error, reason playv1.EncounterBlockedReason) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), reason.String()) || connect.CodeOf(err) == connect.CodeFailedPrecondition
}

// A ghoul's claws are one call: the attack roll, the damage, the paralysis its saving throw asks
// of the target. The player of the target reads nothing of the stat block.
func TestW7M_AGhoulsClawsAreOneCall(t *testing.T) {
	t.Parallel()
	a, e, ghoul := monsterFight(t, "monster:ghoul", 20)
	turn, err := a.creatureTurn(t, a.master, e, ghoul)
	if err != nil {
		t.Fatalf("GetCreatureTurn() error = %v", err)
	}
	claws := actionOf(turn, "monster:ghoul#claws")
	if claws == nil || claws.GetKind() != playv1.CreatureActionKind_CREATURE_ACTION_KIND_ATTACK || claws.GetRider().GetSave().GetDc() != 10 {
		t.Fatalf("claws = %v, want an attack with a DC 10 rider", claws)
	}
	if _, err := a.creatureTurn(t, a.caio, e, ghoul); err == nil {
		t.Error("a player read the turn of a monster")
	}

	a.h.roller.queue(18, 3, 4, 1) // the attack, 2d4, Toren's saving throw
	res, err := a.useAction(t, e, ghoul, "monster:ghoul#claws", "Toren")
	if err != nil {
		t.Fatalf("UseCreatureAction() error = %v", err)
	}
	r := res.GetResult()
	if r.GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_HIT || len(r.GetDamageParts()) != 1 {
		t.Fatalf("result = %v, want a hit with one damage part", r)
	}
	if got := r.GetDamageParts()[0].GetDamageTypeKey(); got != "damage-type:slashing" {
		t.Errorf("damage type = %q, want slashing", got)
	}
	if rd := r.GetRider(); rd.GetSave().GetOutcome() != playv1.SaveOutcome_SAVE_OUTCOME_FAILED || rd.GetConditionKey() != "condition:paralyzed" {
		t.Errorf("rider = %v, want a failed save and paralyzed", rd)
	}
	if !strings.Contains(strings.Join(byLabel(t, a.get(t, a.master), "Toren").GetConditions(), ","), "paralyzed") {
		t.Errorf("Toren's conditions = %v, want paralyzed", byLabel(t, a.get(t, a.master), "Toren").GetConditions())
	}

	// A player's copy of the combat holds nothing of the stat block (RN-10, RN-20).
	player, err := protojson.Marshal(a.get(t, a.caio))
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{"creatureSheet", "legendary", "monster:ghoul", "monsterKey", "recharg"} {
		if strings.Contains(string(player), banned) {
			t.Errorf("a player's combat has %q: %s", banned, player)
		}
	}
}

const (
	redDragon  = "monster:adult-red-dragon"
	acidSplash = "spell:acid-splash"
)

// A legendary action is taken at the end of another creature's turn, one for each offer, and never
// while the creature is incapacitated (SRD 5.1, "Legendary Creatures"). The offers are the
// master's.
func TestW7M_LegendaryActionsComeAtTheEndOfAnotherTurn(t *testing.T) {
	t.Parallel()
	a, e, dragon := monsterFight(t, redDragon, 1)
	use := func(option string) (*playv1.UseLegendaryActionResponse, error) {
		res, err := a.master.creatures.UseLegendaryAction(t.Context(), connect.NewRequest(&playv1.UseLegendaryActionRequest{
			CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, dragon), OptionKey: option, IdempotencyKey: newKey(),
		}))
		if err != nil {
			return nil, err
		}
		return res.Msg, nil
	}
	turn, err := a.creatureTurn(t, a.master, e, dragon)
	if err != nil {
		t.Fatalf("GetCreatureTurn() error = %v", err)
	}
	if turn.GetLegendary() == nil || len(turn.GetLegendary().GetOptions()) != 3 || turn.GetLegendary().GetState().GetPerRound() != 3 {
		t.Fatalf("legendary = %v, want three options and three a round", turn.GetLegendary())
	}
	detect := turn.GetLegendary().GetOptions()[0].GetKey()
	if len(a.get(t, a.master).GetLegendaryOffers()) != 0 {
		t.Fatal("an offer before any turn ended")
	}
	if _, err := use(detect); !refusedWith(err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_LEGENDARY_NOT_NOW) {
		t.Fatalf("a legendary action with no offer: error = %v, want LEGENDARY_NOT_NOW", err)
	}

	// The end of Toren's turn offers one.
	if _, err := a.endTurn(t, a.master, e, true); err != nil {
		t.Fatalf("EndTurn() error = %v", err)
	}
	offers := a.get(t, a.master).GetLegendaryOffers()
	if len(offers) != 1 || offers[0].GetLeft() != 3 || len(offers[0].GetOptions()) != 3 {
		t.Fatalf("offers = %v, want one with 3 left", offers)
	}
	if got := a.get(t, a.caio).GetLegendaryOffers(); len(got) != 0 {
		t.Errorf("a player has offers: %v", got)
	}
	if _, err := use(detect); err != nil {
		t.Fatalf("UseLegendaryAction() error = %v", err)
	}
	if len(a.get(t, a.master).GetLegendaryOffers()) != 0 {
		t.Error("the offer stayed after a legendary action was taken: one for each offer")
	}
	if _, err := use(detect); err == nil {
		t.Error("a second legendary action on the same offer was taken")
	}
	// The count is the master's; a player's log says only that it happened.
	if l := a.get(t, a.master).GetCombatants(); l == nil {
		t.Fatal("no combatants")
	}
	for _, c := range byLabelAll(a.get(t, a.master), dragon) {
		if c.GetLegendary().GetLeft() != 2 {
			t.Errorf("legendary actions left = %v, want 2", c.GetLegendary())
		}
	}

	// An incapacitated creature is offered nothing.
	if _, err := a.master.combat.SetCombatantConditions(t.Context(), connect.NewRequest(&playv1.SetCombatantConditionsRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, dragon), IdempotencyKey: newKey(), Conditions: &playv1.ConditionList{Keys: []string{"condition:stunned"}},
	})); err != nil {
		t.Fatalf("SetCombatantConditions() error = %v", err)
	}
	if _, err := a.endTurn(t, a.master, e, true); err != nil {
		t.Fatalf("EndTurn() error = %v", err)
	}
	if got := a.get(t, a.master).GetLegendaryOffers(); len(got) != 0 {
		t.Errorf("a stunned dragon is offered %v", got)
	}
	if _, err := use(detect); err == nil {
		t.Error("a stunned dragon took a legendary action")
	}
}

func byLabelAll(e *playv1.Encounter, label string) []*playv1.Combatant {
	var out []*playv1.Combatant
	for _, c := range e.GetCombatants() {
		if c.GetLabel() == label {
			out = append(out, c)
		}
	}
	return out
}

func (a *armed) answerResistance(t *testing.T, e *playv1.Encounter, who, cast string, use bool) (*playv1.AnswerLegendaryResistanceResponse, error) {
	t.Helper()
	res, err := a.master.creatures.AnswerLegendaryResistance(t.Context(), connect.NewRequest(&playv1.AnswerLegendaryResistanceRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, who), CastId: cast, Use: use, IdempotencyKey: newKey(),
	}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

// A creature that fails a saving throw may succeed instead with Legendary Resistance (SRD 5.1,
// "Legendary Creatures"): the master is asked, the prompt lives with the damage of the failed save,
// and the answer is the master's alone.
func TestW7M_LegendaryResistanceTurnsAFailedSaveIntoASuccess(t *testing.T) {
	t.Parallel()
	a, e, dragon := monsterFight(t, redDragon, 1)
	e = a.passTo(t, e, "Pensantus")
	a.h.roller.queue(1, 4) // the dragon's saving throw fails; Acid Splash's 1d6
	res, err := a.cast(t, a.ana, e, "Pensantus", acidSplash, nil, a.at(t, dragon), noCastRoll)
	if err != nil {
		t.Fatalf("CastSpell() error = %v", err)
	}
	cast := res.GetCast().GetCastId()
	prompts := a.get(t, a.master).GetLegendaryResistancePrompts()
	if len(prompts) != 1 || prompts[0].GetCastId() != cast || prompts[0].GetUsesLeft() != 3 || prompts[0].GetDc() == 0 {
		t.Fatalf("prompts = %v, want one for the cast with 3 uses left", prompts)
	}
	if got := a.get(t, a.ana).GetLegendaryResistancePrompts(); len(got) != 0 {
		t.Errorf("the caster's player has prompts: %v", got)
	}
	if _, err := a.master.creatures.AnswerLegendaryResistance(t.Context(), connect.NewRequest(&playv1.AnswerLegendaryResistanceRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, dragon), CastId: cast, Use: true, IdempotencyKey: newKey(),
	})); err != nil {
		t.Fatalf("AnswerLegendaryResistance() error = %v", err)
	}
	if got := a.get(t, a.master).GetLegendaryResistancePrompts(); len(got) != 0 {
		t.Errorf("prompts after the answer = %v, want none", got)
	}
	if _, err := a.answerResistance(t, e, dragon, cast, true); err == nil {
		t.Error("a second Legendary Resistance on the same cast was used")
	}
	// The save negates the damage: nothing is left to roll.
	if _, err := a.damage(t, a.ana, e, res.GetCast().GetPendingDamages()[0].GetId(), typedDamage(6)); err == nil {
		t.Error("the damage of a save the creature passed was rolled")
	}
	for _, c := range byLabelAll(a.get(t, a.master), dragon) {
		if c.GetLegendary().GetResistanceLeft() != 2 {
			t.Errorf("Legendary Resistance left = %v, want 2", c.GetLegendary())
		}
	}
}

// "Deixar falhar": the prompt goes, no use is spent and the damage stays to roll.
func TestW7M_LettingTheSaveFailSpendsNothing(t *testing.T) {
	t.Parallel()
	a, e, dragon := monsterFight(t, redDragon, 1)
	e = a.passTo(t, e, "Pensantus")
	a.h.roller.queue(1, 4)
	res, err := a.cast(t, a.ana, e, "Pensantus", acidSplash, nil, a.at(t, dragon), noCastRoll)
	if err != nil {
		t.Fatalf("CastSpell() error = %v", err)
	}
	left, err := a.answerResistance(t, e, dragon, res.GetCast().GetCastId(), false)
	if err != nil {
		t.Fatalf("AnswerLegendaryResistance(false) error = %v", err)
	}
	if left.GetUsesLeft() != 3 {
		t.Errorf("uses left = %d, want 3", left.GetUsesLeft())
	}
	if got := a.get(t, a.master).GetLegendaryResistancePrompts(); len(got) != 0 {
		t.Errorf("prompts = %v, want none", got)
	}
	if _, err := a.damage(t, a.ana, e, res.GetCast().GetPendingDamages()[0].GetId(), typedDamage(6)); err != nil {
		t.Errorf("RollDamage() error = %v, want the damage to roll", err)
	}
}

const fireBreath = redDragon + "#fire-breath"

// Fire Breath asks a Dexterity saving throw and a success takes half (SRD 5.1, Adult Red Dragon);
// it recharges on a 5 or 6 at the start of the dragon's turns, and the d6 is the master's.
func TestW7M_FireBreathTakesHalfOnASuccessAndRecharges(t *testing.T) {
	t.Parallel()
	a, e, dragon := monsterFight(t, redDragon, 20)
	turn, err := a.creatureTurn(t, a.master, e, dragon)
	if err != nil {
		t.Fatalf("GetCreatureTurn() error = %v", err)
	}
	fb := actionOf(turn, fireBreath)
	if fb == nil || fb.GetUsage().GetKind() != playv1.CreatureUsageKind_CREATURE_USAGE_KIND_RECHARGE || fb.GetUsage().GetRechargeMin() != 5 ||
		!fb.GetSave().GetHalfOnSuccess() {
		t.Fatalf("Fire Breath = %v, want Recharge 5-6 with half on a success", fb)
	}

	a.h.roller.queue(20) // Toren's Dexterity save
	res, err := a.useAction(t, e, dragon, fireBreath, "Toren")
	if err != nil {
		t.Fatalf("UseCreatureAction() error = %v", err)
	}
	pending := res.GetResult().GetCast().GetPendingDamages()
	if len(pending) != 1 || !pending[0].GetHalf() {
		t.Fatalf("pending damages = %v, want one, halved by Toren's success", pending)
	}

	// Used: it waits for its recharge, and it is refused until then.
	turn, _ = a.creatureTurn(t, a.master, e, dragon)
	if fb = actionOf(turn, fireBreath); fb.GetAvailable() || !fb.GetUsage().GetRecharging() {
		t.Errorf("Fire Breath after use = %v, want recharging", fb)
	}
	if _, err := a.useAction(t, e, dragon, fireBreath, "Toren"); err == nil {
		t.Error("Fire Breath was used again before it recharged")
	}

	// The dragon's next turn rolls the d6.
	a.h.roller.queue(6)
	e = a.passTo(t, e, "Toren")
	e = a.passTo(t, e, dragon)
	turn, _ = a.creatureTurn(t, a.master, e, dragon)
	if fb = actionOf(turn, fireBreath); !fb.GetAvailable() || fb.GetUsage().GetRecharging() || fb.GetUsage().GetRechargeRoll() != 6 {
		t.Errorf("Fire Breath on the next turn = %v, want recharged by a 6", fb)
	}
	var seen bool
	for _, who := range []struct {
		u      *user
		master bool
	}{{a.master, true}, {a.caio, false}} {
		log, err := who.u.combat.ListCombatLog(t.Context(), connect.NewRequest(&playv1.ListCombatLogRequest{CampaignId: a.campaignID, EncounterId: e.GetId()}))
		if err != nil {
			t.Fatalf("ListCombatLog() error = %v", err)
		}
		for _, round := range log.Msg.GetRounds() {
			for _, entry := range round.GetEntries() {
				if entry.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_MONSTER_RECHARGE {
					if !who.master {
						t.Errorf("a player's log has a recharge line: %v", entry)
					}
					seen = true
				}
			}
		}
	}
	if !seen {
		t.Error("the master's log has no recharge line")
	}
}

// A creature immune to the damage and the condition of an action is never offered the saving
// throw, and the reason is the master's alone (SRD 5.1, Monsters).
func TestW7M_AnImmuneCreatureIsNeverOfferedTheSave(t *testing.T) {
	t.Parallel()
	a, e, dragon := monsterFight(t, redDragon, 20)
	other := a.mustAddMonsters(t, e, func(r *playv1.AddMonstersRequest) {
		r.CreatureKey, r.Count = redDragon, 1
		r.Hidden = new(false)
	})
	var second string
	for _, c := range other.GetEncounter().GetCombatants() {
		if c.GetId() == other.GetCombatantIds()[0] {
			second = c.GetLabel()
		}
	}
	a.h.roller.queue(1)
	res, err := a.useAction(t, e, dragon, fireBreath, second, "Toren")
	if err != nil {
		t.Fatalf("UseCreatureAction() error = %v", err)
	}
	r := res.GetResult()
	if len(r.GetImmuneTargetIds()) != 1 || r.GetImmuneTargetIds()[0] != a.id(t, second) {
		t.Errorf("immune targets = %v, want %s", r.GetImmuneTargetIds(), second)
	}
	for _, tg := range r.GetCast().GetTargets() {
		if tg.GetCombatantId() == a.id(t, second) {
			t.Errorf("the immune dragon was offered a save: %v", tg)
		}
	}
	if len(r.GetCast().GetTargets()) != 1 {
		t.Errorf("targets = %v, want only Toren", r.GetCast().GetTargets())
	}
}

// An undo of a used action gives back what it spent (its recharge, a legendary action), and a retry
// of the same key changes nothing twice.
func TestW7M_UndoGivesBackWhatTheActionSpentAndARetryIsTheSame(t *testing.T) {
	t.Parallel()
	a, e, dragon := monsterFight(t, redDragon, 20)
	req := &playv1.UseCreatureActionRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, dragon), ActionKey: fireBreath,
		TargetIds: []string{a.id(t, "Toren")}, IdempotencyKey: newKey(),
	}
	a.h.roller.queue(1)
	first, err := a.master.creatures.UseCreatureAction(t.Context(), connect.NewRequest(req))
	if err != nil {
		t.Fatalf("UseCreatureAction() error = %v", err)
	}
	again, err := a.master.creatures.UseCreatureAction(t.Context(), connect.NewRequest(req))
	if err != nil {
		t.Fatalf("UseCreatureAction() retry error = %v", err)
	}
	if first.Msg.GetResult().GetCast().GetCastId() != again.Msg.GetResult().GetCast().GetCastId() {
		t.Errorf("a retry made another cast: %v and %v", first.Msg.GetResult().GetCast().GetCastId(), again.Msg.GetResult().GetCast().GetCastId())
	}
	other := proto.Clone(req).(*playv1.UseCreatureActionRequest)
	other.ActionKey = redDragon + "#frightful-presence"
	if _, err := a.master.creatures.UseCreatureAction(t.Context(), connect.NewRequest(other)); err == nil {
		t.Error("the key of one change was taken for another")
	}

	if err := a.undo(t, a.master, e, a.log(t, a.master, e).GetUndoableEventId()); err != nil {
		t.Fatalf("UndoLastAction() error = %v", err)
	}
	turn, _ := a.creatureTurn(t, a.master, e, dragon)
	if fb := actionOf(turn, fireBreath); !fb.GetAvailable() || fb.GetUsage().GetRecharging() {
		t.Errorf("Fire Breath after the undo = %v, want it back", fb)
	}
}
