package play

import (
	"slices"
	"strings"
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/against"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// The master uses the full actions of an NPC's stat block from its turn (UseCreatureAction): a breath weapon
// rolls each target's save against the stat block's DC, opens the damage the save governs and gives the
// condition to a target that fails. The fixture is the party and a hell hound (monster:hell-hound: Fire
// Breath, Dexterity DC 12, 6d6 fire, half on a save, a 15-foot cone; bite 1d8+3 piercing plus 2d6 fire).
// These tests need the database (MEURPG_TEST_DATABASE_URL).

const (
	houndKey   = "monster:hell-hound"
	fireBreath = "monster:hell-hound#fire-breath"
)

// useAction calls UseCreatureAction as u, by labels.
func (a *armed) useAction(t *testing.T, u *user, e *playv1.Encounter, actor, action string, targets []string, edit ...func(*playv1.UseCreatureActionRequest)) (*playv1.UseCreatureActionResponse, error) {
	t.Helper()
	req := &playv1.UseCreatureActionRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, actor), ActionKey: action, IdempotencyKey: newKey(),
	}
	for _, l := range targets {
		req.TargetIds = append(req.TargetIds, a.id(t, l))
	}
	for _, f := range edit {
		f(req)
	}
	res, err := u.combat.UseCreatureAction(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func (a *armed) mustUseAction(t *testing.T, u *user, e *playv1.Encounter, actor, action string, targets []string, edit ...func(*playv1.UseCreatureActionRequest)) *playv1.UseCreatureActionResponse {
	t.Helper()
	res, err := a.useAction(t, u, e, actor, action, targets, edit...)
	if err != nil {
		t.Fatalf("UseCreatureAction(%s %s) error = %v", actor, action, err)
	}
	return res
}

// houndFight starts the combat with the party and a hell hound on turn: on the map when theatre is false
// (Toren at 8,5 and the Goblin at 7,5 in the hound's cone toward the west, Pensantus out of it), without
// one otherwise. The hound is the combatant "Cão Infernal" and has a turn of its own.
func houndFight(t *testing.T, theatre bool) (*armed, *playv1.Encounter, string) {
	t.Helper()
	return creatureFight(t, theatre, houndKey, false)
}

// creatureFight is houndFight for any creature, hidden or not: the creature is the first combatant to act
// after the party's, and the answer is its label.
func creatureFight(t *testing.T, theatre bool, key string, hidden bool) (*armed, *playv1.Encounter, string) {
	t.Helper()
	return creatureFightOf(t, newCastingParty(t), theatre, key, hidden)
}

// dwarfParty is the casting party with Toren a dwarf: resistant to poison.
func dwarfParty(t *testing.T) *armed {
	t.Helper()
	return newArmedWith(t, func(a *armed) {
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:dwarf", 5,
			&rulesv1.AbilityScores{Strength: 16, Dexterity: 13, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 8}, []string{battleaxe}, nil)
		a.pens = a.ana.castingHero(t, a.campaignID, "Pensantus", classLevel("class:wizard", 5, ""),
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 8}, []string{fireBolt},
			[]string{mageArmorKey, detectMagicKey, alarmKey, identifyKey, magicMissileSpell, shieldSpell, holdPerson},
			[]string{mageArmorKey, magicMissileSpell, shieldSpell}, "")
		a.bri = a.bia.castingHero(t, a.campaignID, "Brisa", classLevel("class:cleric", 3, "subclass:life"),
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 16, Constitution: 14, Intelligence: 10, Wisdom: 16, Charisma: 8}, []string{sacredFlame}, nil,
			[]string{cureWounds, healingWord, prayerKey, detectMagicKey, blessKey, aidSpell}, "")
	})
}

// creatureFightOf is creatureFight on a party already built.
func creatureFightOf(t *testing.T, a *armed, theatre bool, key string, hidden bool) (*armed, *playv1.Encounter, string) {
	t.Helper()
	e := a.start(t, plan{
		npcs: []*playv1.Participant{{CharacterId: a.goblin.GetId()}}, npcRolls: []int{1},
		players: map[string]int32{"Toren": 10, "Pensantus": 5, "Brisa": 1},
		reveal:  []string{"Goblin"}, setup: true, theatre: theatre,
	})
	res := a.mustAddMonsters(t, e, func(r *playv1.AddMonstersRequest) {
		r.CreatureKey, r.Count, r.Hidden = key, 1, &hidden
	})
	hound := ""
	for _, c := range res.GetEncounter().GetCombatants() {
		if c.GetId() == res.GetCombatantIds()[0] {
			hound = c.GetLabel()
		}
	}
	e = a.begin(t, a.get(t, a.master))
	if !theatre {
		a.castStand(t, map[string][2]int32{hound: {10, 5}, "Toren": {8, 5}, "Goblin": {7, 5}, "Pensantus": {10, 9}, "Brisa": {1, 1}})
	}
	e = a.passTo(t, e, hound)
	return a, e, hound
}

func hitOn(res *playv1.UseCreatureActionResponse, id string) *playv1.CreatureActionTarget {
	for _, t := range res.GetResult().GetTargets() {
		if t.GetCombatantId() == id {
			return t
		}
	}
	return nil
}

// A breath on two targets in the theatre of the mind: Toren (a player's character) passes his save and
// takes half, the Goblin (an NPC) fails and takes all. The master reads each roll and the DC; the pending
// damages are Half for the one that saved; the action is spent.
func TestCreatureAction_BreathHalfOnASaveFullOnAFail(t *testing.T) {
	t.Parallel()
	a, e, hound := houndFight(t, true)
	a.h.roller.queue(18, 3) // Toren's save, then the Goblin's
	res := a.mustUseAction(t, a.master, e, hound, fireBreath, []string{"Toren", "Goblin"})

	toren, goblin := hitOn(res, a.id(t, "Toren")), hitOn(res, a.id(t, "Goblin"))
	if toren == nil || goblin == nil {
		t.Fatalf("targets = %v, want Toren and the Goblin", res.GetResult().GetTargets())
	}
	if toren.GetSave().GetOutcome() != playv1.SaveOutcome_SAVE_OUTCOME_SAVED || goblin.GetSave().GetOutcome() != playv1.SaveOutcome_SAVE_OUTCOME_FAILED {
		t.Errorf("saves = %v / %v, want saved and failed", toren.GetSave().GetOutcome(), goblin.GetSave().GetOutcome())
	}
	if dc := toren.GetSave().GetDc(); dc != 12 {
		t.Errorf("the master's DC = %d, want the stat block's 12", dc)
	}
	halves := map[string]bool{}
	for _, p := range res.GetResult().GetPendingDamages() {
		halves[p.GetTargetId()] = p.GetHalf()
		if p.GetDamageTypeKey() != "damage-type:fire" || p.GetAttackKey() != fireBreath {
			t.Errorf("pending damage = %q from %q, want fire from the breath", p.GetDamageTypeKey(), p.GetAttackKey())
		}
	}
	if len(halves) != 2 || !halves[a.id(t, "Toren")] || halves[a.id(t, "Goblin")] {
		t.Errorf("half by target = %v, want half for Toren and full for the Goblin", halves)
	}
	if toren.GetPendingDamageId() == "" || goblin.GetPendingDamageId() == "" {
		t.Error("each target should have a pending damage")
	}
	if !byLabel(t, res.GetEncounter(), hound).GetActionUsed() {
		t.Error("the hound's action is not spent")
	}
	// Rolled and applied, the damage lands: 21 fire on the Goblin in full, half (10) on Toren.
	hpGoblin, _, _ := a.hp(t, "Goblin")
	hpToren, _, _ := a.hp(t, "Toren")
	a.mustDamage(t, a.master, e, goblin.GetPendingDamageId(), typedDamage(21)) // one roll for the whole breath
	if _, err := a.settle(t, a.master, e, toren.GetPendingDamageId(), true); err != nil {
		t.Fatalf("apply Toren's damage: %v", err)
	}
	afterGoblin, _, _ := a.hp(t, "Goblin")
	afterToren, _, _ := a.hp(t, "Toren")
	if hpGoblin-afterGoblin != 21 && afterGoblin != 0 {
		t.Errorf("the Goblin took %d, want 21 (or fall)", hpGoblin-afterGoblin)
	}
	if hpToren-afterToren != 10 {
		t.Errorf("Toren took %d, want half of 21 = 10", hpToren-afterToren)
	}
}

// Wrong turn, a spent action, a player calling it, an unknown action and an attack are refused.
func TestCreatureAction_Refusals(t *testing.T) {
	t.Parallel()
	a, e, hound := houndFight(t, true)

	// A player cannot call it.
	_, err := a.useAction(t, a.caio, e, hound, fireBreath, []string{"Toren"})
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("a player's call: code = %v, want permission_denied", connect.CodeOf(err))
	}
	// An action the stat block does not have, and the bite (an attack roll).
	for _, key := range []string{"monster:hell-hound#nothing", "monster:hell-hound#bite", "monster:ghoul#claws"} {
		_, err = a.useAction(t, a.master, e, hound, key, []string{"Toren"})
		wantBlockedBy(t, key, err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_CREATURE_ACTION_UNAVAILABLE)
	}
	// A player's character is no NPC.
	_, err = a.useAction(t, a.master, e, "Toren", fireBreath, []string{"Goblin"})
	wantBlockedBy(t, "a player's character", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_CREATURE_ACTION_UNAVAILABLE)
	// A target is needed, not oneself, not twice.
	_, err = a.useAction(t, a.master, e, hound, fireBreath, nil)
	wantCode(t, "no target", err, connect.CodeInvalidArgument)
	_, err = a.useAction(t, a.master, e, hound, fireBreath, []string{hound})
	wantCode(t, "itself", err, connect.CodeInvalidArgument)
	_, err = a.useAction(t, a.master, e, hound, fireBreath, []string{"Toren", "Toren"})
	wantCode(t, "a target twice", err, connect.CodeInvalidArgument)
	_, err = a.useAction(t, a.master, e, hound, "", []string{"Toren"})
	wantCode(t, "no action key", err, connect.CodeInvalidArgument)
	// An area is for a map.
	_, err = a.useAction(t, a.master, e, hound, fireBreath, []string{"Toren"}, func(r *playv1.UseCreatureActionRequest) {
		r.Area = &playv1.UseCreatureActionRequest_Direction{Direction: toward(-1, 0)}
	})
	wantCode(t, "an area without a map", err, connect.CodeInvalidArgument)

	// Used once, the action is spent; and it is not the Goblin's turn.
	a.mustUseAction(t, a.master, e, hound, fireBreath, []string{"Toren"})
	_, err = a.useAction(t, a.master, e, hound, fireBreath, []string{"Toren"})
	wantBlockedBy(t, "the action twice", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ACTION_USED)
	_, err = a.useAction(t, a.master, e, "Goblin", fireBreath, []string{"Toren"})
	wantBlockedBy(t, "another's turn", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_YOUR_TURN)
}

// The hell hound's breath on a map: the master places the cone toward the west and the server says who is
// inside. The hound is not one of them; Pensantus, out of the cone, is not either.
func TestCreatureAction_AreaTakesWhoIsInside(t *testing.T) {
	t.Parallel()
	a, e, hound := houndFight(t, false)
	a.h.roller.queue(3, 3)
	res := a.mustUseAction(t, a.master, e, hound, fireBreath, nil, func(r *playv1.UseCreatureActionRequest) {
		r.Area = &playv1.UseCreatureActionRequest_Direction{Direction: toward(-1, 0)}
	})
	var got []string
	for _, tg := range res.GetResult().GetTargets() {
		got = append(got, byLabelID(t, res.GetEncounter(), tg.GetCombatantId()))
	}
	slices.Sort(got)
	if !slices.Equal(got, []string{"Goblin", "Toren"}) {
		t.Fatalf("targets = %v, want the Goblin and Toren", got)
	}
	if res.GetArea() == nil {
		t.Error("the answer has no area")
	}
	// Without a direction the cone is refused; naming targets on a map is not how it is placed either.
	a2, e2, hound2 := houndFight(t, false)
	_, err := a2.useAction(t, a2.master, e2, hound2, fireBreath, nil)
	wantCode(t, "a cone without a direction", err, connect.CodeInvalidArgument)
}

func byLabelID(t *testing.T, e *playv1.Encounter, id string) string {
	t.Helper()
	for _, c := range e.GetCombatants() {
		if c.GetId() == id {
			return c.GetLabel()
		}
	}
	t.Fatalf("no combatant %s", id)
	return ""
}

// A mummy's Dreadful Glare: Toren fails his Wisdom save and is frightened; the Goblin passes. The
// condition is the plain label, the log says who got it, and the undo takes it back with the action.
func TestCreatureAction_ConditionOnAFailedSave(t *testing.T) {
	t.Parallel()
	a, e, mummy := creatureFight(t, true, "monster:mummy", false)
	const glare = "monster:mummy#dreadful-glare"
	a.h.roller.queue(1, 20)
	res := a.mustUseAction(t, a.master, e, mummy, glare, []string{"Toren", "Goblin"})
	if res.GetResult().GetConditionKey() != "condition:frightened" || res.GetResult().GetConditionNamePt() == "" {
		t.Errorf("result condition = %q (%q), want frightened, named", res.GetResult().GetConditionKey(), res.GetResult().GetConditionNamePt())
	}
	if !hasCondition(res.GetEncounter(), t, "Toren", "condition:frightened") || hasCondition(res.GetEncounter(), t, "Goblin", "condition:frightened") {
		t.Errorf("conditions = Toren %v, Goblin %v, want only Toren frightened", byLabel(t, res.GetEncounter(), "Toren").GetConditions(), byLabel(t, res.GetEncounter(), "Goblin").GetConditions())
	}
	if !hitOn(res, a.id(t, "Toren")).GetConditionApplied() || hitOn(res, a.id(t, "Goblin")).GetConditionApplied() {
		t.Error("condition_applied should be true for Toren only")
	}
	if len(res.GetResult().GetPendingDamages()) != 0 {
		t.Errorf("a save with no damage opened %d pending damages", len(res.GetResult().GetPendingDamages()))
	}
	line := lastCreatureLine(t, a.log(t, a.master, e))
	if line.GetConditionKey() != "condition:frightened" || !slices.Equal(line.GetConditionTargetIds(), []string{a.id(t, "Toren")}) || line.GetActionNamePt() == "" {
		t.Errorf("log line = %v, want frightened for Toren alone, named", line)
	}
	l := a.log(t, a.master, e)
	if err := a.undo(t, a.master, e, l.GetUndoableEventId()); err != nil {
		t.Fatalf("UndoLastAction() error = %v", err)
	}
	back := a.get(t, a.master)
	if hasCondition(back, t, "Toren", "condition:frightened") || byLabel(t, back, mummy).GetActionUsed() {
		t.Errorf("after the undo: Toren %v, action used %v; want no condition and the action back", byLabel(t, back, "Toren").GetConditions(), byLabel(t, back, mummy).GetActionUsed())
	}
}

func lastCreatureLine(t *testing.T, l *playv1.ListCombatLogResponse) *playv1.CombatLogCreatureAction {
	t.Helper()
	var out *playv1.CombatLogCreatureAction
	for _, en := range logEntries(l) {
		if en.GetCreatureAction() != nil {
			out = en.GetCreatureAction()
		}
	}
	if out == nil {
		t.Fatal("the log has no creature action line")
	}
	return out
}

// An action with no roll (a knight's Leadership) spends the action and writes the line, nothing else.
func TestCreatureAction_NoRollIsAReminder(t *testing.T) {
	t.Parallel()
	a, e, knight := creatureFight(t, true, "monster:knight", false)
	const leadership = "monster:knight#leadership"
	_, err := a.useAction(t, a.master, e, knight, leadership, []string{"Toren"})
	wantCode(t, "a target for an action with no roll", err, connect.CodeInvalidArgument)
	res := a.mustUseAction(t, a.master, e, knight, leadership, nil)
	if !res.GetResult().GetReminder() || len(res.GetResult().GetTargets()) != 0 || len(res.GetResult().GetPendingDamages()) != 0 {
		t.Errorf("result = %v, want a reminder with no target and no damage", res.GetResult())
	}
	if !byLabel(t, res.GetEncounter(), knight).GetActionUsed() {
		t.Error("the action is not spent")
	}
	if line := lastCreatureLine(t, a.log(t, a.master, e)); !line.GetReminder() || line.GetText() == "" {
		t.Errorf("log line = %v, want the reminder with the master's text", line)
	}
	for _, who := range []*user{a.caio, a.ana} {
		if line := lastCreatureLine(t, a.log(t, who, e)); !line.GetReminder() || line.GetText() != "" {
			t.Errorf("a player's line = %v, want the reminder without the stat block's text", line)
		}
	}
}

// What a player reads of the breath: the save of their own character, how it ended, and the damage they
// take; never the DC (the table rule "Os jogadores veem a CD" does not cover a stat block's number), and not
// the Goblin's roll. A hidden hound's line is the master's alone.
func TestCreatureAction_PlayersNeverReadTheStatBlock(t *testing.T) {
	t.Parallel()
	a, e, hound := houndFight(t, true)
	a.setRules(t, showSaveDC)
	a.h.roller.queue(18, 3)
	a.mustUseAction(t, a.master, e, hound, fireBreath, []string{"Toren", "Goblin"})
	saveOf := func(who *user, label string) *playv1.SaveResult {
		for _, en := range logEntries(a.log(t, who, e)) {
			for _, tg := range en.GetSpell().GetTargets() {
				if tg.GetTargetLabel() == label {
					return tg.GetSave()
				}
			}
		}
		return nil
	}
	if sv := saveOf(a.caio, "Toren"); sv == nil || sv.GetRoll() == nil || sv.GetDc() != 0 || sv.GetOutcome() != playv1.SaveOutcome_SAVE_OUTCOME_SAVED {
		t.Errorf("Toren's player reads %v, want his roll and the result, with no DC", sv)
	}
	if sv := saveOf(a.ana, "Toren"); sv == nil || sv.GetRoll() != nil || sv.GetDc() != 0 {
		t.Errorf("another player reads %v of Toren's save, want the result alone", sv)
	}
	if sv := saveOf(a.caio, "Goblin"); sv == nil || sv.GetRoll() != nil || sv.GetDc() != 0 {
		t.Errorf("a player reads %v of the Goblin's save, want the result alone", sv)
	}
	if sv := saveOf(a.master, "Toren"); sv == nil || sv.GetDc() != 12 {
		t.Errorf("the master reads %v, want the DC 12", sv)
	}
}

// The line of an action that rolls no attack names it in Portuguese (names_pt.json), for the master and for a
// player alike: a breath ("Sopro de fogo") and a reminder ("Liderança").
func TestCreatureAction_TheLogNamesTheActionInPortuguese(t *testing.T) {
	t.Parallel()
	a, e, hound := houndFight(t, true)
	a.h.roller.queue(18, 3)
	a.mustUseAction(t, a.master, e, hound, fireBreath, []string{"Toren", "Goblin"})
	for name, who := range map[string]*user{"master": a.master, "player": a.caio} {
		if got := lastCreatureLine(t, a.log(t, who, e)).GetActionNamePt(); got != "Sopro de fogo" {
			t.Errorf("the %s reads the breath as %q, want %q", name, got, "Sopro de fogo")
		}
	}
	k, ke, knight := creatureFight(t, true, "monster:knight", false)
	k.mustUseAction(t, k.master, ke, knight, "monster:knight#leadership", nil)
	if got := lastCreatureLine(t, k.log(t, k.caio, ke)).GetActionNamePt(); got != "Liderança" {
		t.Errorf("a player reads the reminder as %q, want %q", got, "Liderança")
	}
}

// A hidden hound's breath is the master's line alone, and the hound is not named to a player.
func TestCreatureAction_AHiddenCreatureMasksTheLine(t *testing.T) {
	t.Parallel()
	a, e, hound := creatureFight(t, true, houndKey, true)
	a.mustUseAction(t, a.master, e, hound, fireBreath, []string{"Toren"})
	for _, who := range []*user{a.caio, a.ana} {
		for _, en := range logEntries(a.log(t, who, e)) {
			if en.GetCreatureAction() != nil || en.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_SPELL_CAST {
				t.Errorf("a player reads the hidden hound's breath: %v", en)
			}
		}
	}
	if line := lastCreatureLine(t, a.log(t, a.master, e)); line.GetActionKey() != fireBreath {
		t.Errorf("the master's line = %v, want the breath", line)
	}
}

// The hell hound's bite is 1d8+3 piercing and 2d6 fire: the sheet of an NPC keeps the first part, the stat
// block's plan adds the others as parts of the same damage, each with its own type, and every one rolls.
func TestCreatureAction_AMultiPartAttackRollsEveryPart(t *testing.T) {
	t.Parallel()
	a, e, hound := houndFight(t, true)
	res := a.mustAttack(t, a.master, e, hound, "basic:0", "Toren", d20(19))
	wantHit(t, "the bite", res, hit)
	pd := res.GetPendingDamage()
	types := map[string]bool{}
	for _, p := range pd.GetParts() {
		types[p.GetDamageTypeKey()] = true
	}
	if len(pd.GetParts()) != 2 || !types["damage-type:piercing"] || !types["damage-type:fire"] {
		t.Fatalf("parts = %v, want the bite's piercing and the fire", pd.GetParts())
	}
	before, _, _ := a.hp(t, "Toren")
	a.h.roller.queue(4, 3, 5) // 1d8, then 2d6
	rolled := a.mustDamage(t, a.master, e, pd.GetId(), inAppDamage).GetPendingDamage()
	sums := map[string]int32{}
	for _, r := range rolled.GetPartRolls() {
		sums[r.GetDamageTypeKey()] += r.GetSum()
	}
	if sums["damage-type:piercing"] != 4 || sums["damage-type:fire"] != 8 {
		t.Errorf("part rolls by type = %v, want the d8 (4) and the 2d6 of fire (3+5 = 8)", sums)
	}
	if _, err := a.settle(t, a.master, e, pd.GetId(), true); err != nil {
		t.Fatalf("apply the bite: %v", err)
	}
	if after, _, _ := a.hp(t, "Toren"); before-after != 15 {
		t.Errorf("Toren took %d, want 15", before-after)
	}
}

// The ghoul's claws ask a Constitution save of the creature they hit (paralysis): once the damage landed the
// master is offered the rider, and only the master. Applying it has the server roll the save: Toren fails and
// is paralyzed. His player reads the result and his own roll, never the DC; the master reads the DC.
func TestCreatureRider_OfferedToTheMasterAndApplied(t *testing.T) {
	t.Parallel()
	a, e, ghoul := creatureFight(t, true, "monster:ghoul", false)
	res := a.mustAttack(t, a.master, e, ghoul, "basic:1", "Toren", d20(19))
	wantHit(t, "the claws", res, hit)
	id := res.GetPendingDamage().GetId()
	a.mustDamage(t, a.master, e, id, typedDamage(5))
	if offers := ridersOfKind(a.get(t, a.master), playv1.HitRiderKind_HIT_RIDER_KIND_CREATURE); len(offers) != 0 {
		t.Fatalf("the rider is offered before the damage lands: %v", offers)
	}
	if _, err := a.settle(t, a.master, e, id, true); err != nil {
		t.Fatalf("apply the claws: %v", err)
	}
	offers := ridersOfKind(a.get(t, a.master), playv1.HitRiderKind_HIT_RIDER_KIND_CREATURE)
	if len(offers) != 1 || offers[0].GetActionKey() != "monster:ghoul#claws" || offers[0].GetSaveDc() != 10 || offers[0].GetConditionKey() != "condition:paralyzed" {
		t.Fatalf("creature offers = %v, want the claws' rider (DC 10, paralyzed)", offers)
	}
	for _, who := range []*user{a.caio, a.ana, a.bia} {
		if got := a.get(t, who).GetHitRiders(); len(got) != 0 {
			t.Errorf("a player reads %d riders, want none", len(got))
		}
	}
	_, err := a.useRider(t, a.caio, e, offers[0].GetId(), playv1.HitRiderChoice_HIT_RIDER_CHOICE_APPLY)
	wantCode(t, "a player answers a creature's rider", err, connect.CodePermissionDenied)
	_, err = a.useRider(t, a.master, e, offers[0].GetId(), playv1.HitRiderChoice_HIT_RIDER_CHOICE_STUN)
	wantCode(t, "a choice of another rider", err, connect.CodeInvalidArgument)

	a.h.roller.queue(2) // Toren's save: 2 + his bonus fails against 10
	after, err := a.useRider(t, a.master, e, offers[0].GetId(), playv1.HitRiderChoice_HIT_RIDER_CHOICE_APPLY)
	if err != nil {
		t.Fatalf("UseHitRider(apply) error = %v", err)
	}
	if !hasCondition(after, t, "Toren", "condition:paralyzed") {
		t.Errorf("Toren's conditions = %v, want paralyzed", byLabel(t, after, "Toren").GetConditions())
	}
	if len(ridersOfKind(after, playv1.HitRiderKind_HIT_RIDER_KIND_CREATURE)) != 0 {
		t.Error("the answered offer is still open")
	}
	line := a.lastRiderLine(t)
	if line.GetResult() != playv1.HitRiderLogResult_HIT_RIDER_LOG_RESULT_FAILED || line.GetDc() != 10 || line.GetConditionKey() != "condition:paralyzed" || line.GetDurationText() == "" {
		t.Errorf("the master's line = %v, want a failed save against DC 10 that paralyzed for the stat block's duration", line)
	}
	mine := a.lastRiderLineOf(t, a.caio)
	if mine.GetDc() != 0 || mine.GetEscapeDc() != 0 || mine.GetResult() != playv1.HitRiderLogResult_HIT_RIDER_LOG_RESULT_FAILED || mine.GetD20() != 2 {
		t.Errorf("Toren's player reads %v, want the result and his roll, with no DC", mine)
	}
	if other := a.lastRiderLineOf(t, a.ana); other.GetDc() != 0 || other.GetD20() != 0 || other.GetNumbers() {
		t.Errorf("another player reads %v, want the result alone", other)
	}
	_, err = a.useRider(t, a.master, e, offers[0].GetId(), playv1.HitRiderChoice_HIT_RIDER_CHOICE_APPLY)
	if err == nil {
		t.Error("an answered offer was answered again")
	}
}

// A skipped rider does nothing; and a spider's bite opens its poison damage (half on a save) as pending damage.
func TestCreatureRider_DeclineAndSaveDamage(t *testing.T) {
	t.Parallel()
	a, e, ghoul := creatureFight(t, true, "monster:ghoul", false)
	res := a.mustAttack(t, a.master, e, ghoul, "basic:1", "Toren", d20(19))
	a.mustDamage(t, a.master, e, res.GetPendingDamage().GetId(), typedDamage(5))
	if _, err := a.settle(t, a.master, e, res.GetPendingDamage().GetId(), true); err != nil {
		t.Fatalf("apply the claws: %v", err)
	}
	offers := ridersOfKind(a.get(t, a.master), playv1.HitRiderKind_HIT_RIDER_KIND_CREATURE)
	after, err := a.useRider(t, a.master, e, offers[0].GetId(), playv1.HitRiderChoice_HIT_RIDER_CHOICE_DECLINE)
	if err != nil {
		t.Fatalf("UseHitRider(decline) error = %v", err)
	}
	if hasCondition(after, t, "Toren", "condition:paralyzed") || a.lastRiderLine(t).GetResult() != playv1.HitRiderLogResult_HIT_RIDER_LOG_RESULT_DECLINED {
		t.Error("a skipped rider changed the target or was not logged as declined")
	}

	b, eb, spider := creatureFight(t, true, "monster:giant-spider", false)
	r := b.mustAttack(t, b.master, eb, spider, "basic:0", "Toren", d20(19))
	b.mustDamage(t, b.master, eb, r.GetPendingDamage().GetId(), typedDamage(5))
	if _, err := b.settle(t, b.master, eb, r.GetPendingDamage().GetId(), true); err != nil {
		t.Fatalf("apply the bite: %v", err)
	}
	spiderOffers := ridersOfKind(b.get(t, b.master), playv1.HitRiderKind_HIT_RIDER_KIND_CREATURE)
	if len(spiderOffers) != 1 || spiderOffers[0].GetSaveDc() != 11 {
		t.Fatalf("spider offers = %v, want one (DC 11)", spiderOffers)
	}
	b.h.roller.queue(20) // Toren saves
	if _, err := b.useRider(t, b.master, eb, spiderOffers[0].GetId(), playv1.HitRiderChoice_HIT_RIDER_CHOICE_APPLY); err != nil {
		t.Fatalf("UseHitRider(apply) error = %v", err)
	}
	pendings := b.mustOptions(t, b.master, b.get(t, b.master), spider).GetPendingDamages() // the damage the spider still has to roll
	var poison *playv1.PendingDamage
	for _, p := range pendings {
		if p.GetDamageTypeKey() == "damage-type:poison" {
			poison = p
		}
	}
	if poison == nil || !poison.GetHalf() || poison.GetDiceCount() != 2 || poison.GetDiceSides() != 8 {
		t.Fatalf("pending damages after the saved rider = %v, want 2d8 poison, half", pendings)
	}
}

// A grapple has no save: the hit gives the condition and the line notes the escape DC, which only the master reads.
func TestCreatureRider_GrappleNotesTheEscapeDC(t *testing.T) {
	t.Parallel()
	a, e, crab := creatureFight(t, true, "monster:giant-crab", false)
	res := a.mustAttack(t, a.master, e, crab, "basic:0", "Toren", d20(19))
	a.mustDamage(t, a.master, e, res.GetPendingDamage().GetId(), typedDamage(3))
	if _, err := a.settle(t, a.master, e, res.GetPendingDamage().GetId(), true); err != nil {
		t.Fatalf("apply the claw: %v", err)
	}
	offers := ridersOfKind(a.get(t, a.master), playv1.HitRiderKind_HIT_RIDER_KIND_CREATURE)
	if len(offers) != 1 || offers[0].GetEscapeDc() != 11 || offers[0].GetConditionKey() != "condition:grappled" {
		t.Fatalf("offers = %v, want the claw's grapple (escape DC 11)", offers)
	}
	after, err := a.useRider(t, a.master, e, offers[0].GetId(), playv1.HitRiderChoice_HIT_RIDER_CHOICE_APPLY)
	if err != nil {
		t.Fatalf("UseHitRider(apply) error = %v", err)
	}
	if !hasCondition(after, t, "Toren", "condition:grappled") {
		t.Errorf("Toren's conditions = %v, want grappled", byLabel(t, after, "Toren").GetConditions())
	}
	if line := a.lastRiderLine(t); line.GetEscapeDc() != 11 || line.GetResult() != playv1.HitRiderLogResult_HIT_RIDER_LOG_RESULT_APPLIED {
		t.Errorf("the master's line = %v, want the escape DC 11, applied", line)
	}
	if mine := a.lastRiderLineOf(t, a.caio); mine.GetEscapeDc() != 0 || mine.GetConditionKey() != "condition:grappled" {
		t.Errorf("Toren's player reads %v, want the grapple without the escape DC", mine)
	}
}

// monsterKeyed lists the lines of a log that name a stat block's key ("monster:...") in any field a player
// reads: the entry's key, the creature action's, the rider's.
func monsterKeyed(l *playv1.ListCombatLogResponse) []string {
	var out []string
	for _, en := range logEntries(l) {
		for _, k := range []string{en.GetKey(), en.GetCreatureAction().GetActionKey(), en.GetHitRider().GetActionKey()} {
			if strings.Contains(k, "monster:") {
				out = append(out, k)
			}
		}
	}
	return out
}

// A player never reads the key of an NPC's stat block action, in the line of the action nor in the lines of
// the damage it opened (RN-20); the master does. The condition, its duration and its repeating are told with
// who got it: a breath every target saved against tells a player none of them.
func TestCreatureAction_PlayersReadNoMonsterKeyAndTheConditionOnlyWithItsTarget(t *testing.T) {
	t.Parallel()
	const glare = "monster:mummy#dreadful-glare"
	for _, saved := range []bool{true, false} {
		a, e, mummy := creatureFight(t, true, "monster:mummy", false)
		if saved {
			a.h.roller.queue(20)
		} else {
			a.h.roller.queue(1)
		}
		a.mustUseAction(t, a.master, e, mummy, glare, []string{"Toren"})
		for _, who := range []*user{a.caio, a.ana} {
			l := a.log(t, who, e)
			if keys := monsterKeyed(l); len(keys) != 0 {
				t.Errorf("saved=%v: a player reads the stat block keys %v", saved, keys)
			}
			line := lastCreatureLine(t, l)
			told := line.GetConditionKey() != "" || line.GetDurationText() != "" || line.GetRepeatSave() || len(line.GetConditionTargetIds()) != 0
			if saved && told {
				t.Errorf("every target saved, a player reads the condition: %v", line)
			}
			// Frightened is a condition with no outward sign: Toren's player reads it, the other does not.
			owner := who == a.caio
			if !saved && owner && (line.GetConditionKey() != "condition:frightened" || len(line.GetConditionTargetIds()) != 1) {
				t.Errorf("Toren failed, his player reads %v, want the condition on him", line)
			}
			if !saved && !owner && told {
				t.Errorf("Toren failed, another player reads %v of a condition with no outward sign", line)
			}
		}
		if keys := monsterKeyed(a.log(t, a.master, e)); len(keys) == 0 {
			t.Errorf("saved=%v: the master reads no key: the control is unreachable", saved)
		}
	}
}

// The damage a breath opened has the action's key in its pending damage; the lines that follow the roll and
// the apply do not show it to a player.
func TestCreatureAction_TheDamageLinesShowAPlayerNoMonsterKey(t *testing.T) {
	t.Parallel()
	a, e, hound := houndFight(t, true)
	a.h.roller.queue(1) // Toren fails
	res := a.mustUseAction(t, a.master, e, hound, fireBreath, []string{"Toren"})
	ids := res.GetResult().GetPendingDamages()
	if len(ids) == 0 {
		t.Fatal("the breath opened no damage")
	}
	a.mustDamage(t, a.master, e, ids[0].GetId(), typedDamage(12))
	if _, err := a.settle(t, a.master, e, ids[0].GetId(), true); err != nil {
		t.Fatalf("apply the breath's damage: %v", err)
	}
	for _, who := range []*user{a.caio, a.ana} {
		if keys := monsterKeyed(a.log(t, who, e)); len(keys) != 0 {
			t.Errorf("a player reads %v", keys)
		}
	}
	if keys := monsterKeyed(a.log(t, a.master, e)); len(keys) == 0 {
		t.Error("the master reads no key in the lines of the breath's damage")
	}
}

// The undo of a breath takes back the action, the condition and the pending damage it opened.
func TestCreatureAction_UndoTakesBackThePendingDamage(t *testing.T) {
	t.Parallel()
	a, e, hound := houndFight(t, true)
	a.h.roller.queue(1, 1)
	res := a.mustUseAction(t, a.master, e, hound, fireBreath, []string{"Toren", "Goblin"})
	if len(res.GetResult().GetPendingDamages()) != 2 {
		t.Fatalf("pending damages = %d, want 2", len(res.GetResult().GetPendingDamages()))
	}
	l := a.log(t, a.master, e)
	if err := a.undo(t, a.master, e, l.GetUndoableEventId()); err != nil {
		t.Fatalf("UndoLastAction() error = %v", err)
	}
	opts, err := a.master.combat.GetTurnOptions(t.Context(), connect.NewRequest(&playv1.GetTurnOptionsRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, hound)}))
	if err != nil {
		t.Fatal(err)
	}
	if n := len(opts.Msg.GetPendingDamages()); n != 0 || byLabel(t, a.get(t, a.master), hound).GetActionUsed() {
		t.Errorf("after the undo: %d pending damages, action used %v; want none and the action back", n, byLabel(t, a.get(t, a.master), hound).GetActionUsed())
	}
	// The same breath can be used again.
	a.h.roller.queue(20, 20)
	a.mustUseAction(t, a.master, e, hound, fireBreath, []string{"Toren", "Goblin"})
}

// The undo of the damage that opened a rider takes the offer back, and applying the damage again offers it
// once: a stale offer would let the master apply a rider of a hit that did not happen. A hit that downs the
// target offers none.
func TestCreatureRider_UndoOfTheDamageTakesTheOfferBack(t *testing.T) {
	t.Parallel()
	a, e, ghoul := creatureFight(t, true, "monster:ghoul", false)
	res := a.mustAttack(t, a.master, e, ghoul, "basic:1", "Toren", d20(19))
	id := res.GetPendingDamage().GetId()
	a.mustDamage(t, a.master, e, id, typedDamage(5))
	if _, err := a.settle(t, a.master, e, id, true); err != nil {
		t.Fatalf("apply the claws: %v", err)
	}
	if n := len(ridersOfKind(a.get(t, a.master), playv1.HitRiderKind_HIT_RIDER_KIND_CREATURE)); n != 1 {
		t.Fatalf("offers = %d, want 1", n)
	}
	l := a.log(t, a.master, e)
	if err := a.undo(t, a.master, e, l.GetUndoableEventId()); err != nil {
		t.Fatalf("UndoLastAction() error = %v", err)
	}
	if n := len(ridersOfKind(a.get(t, a.master), playv1.HitRiderKind_HIT_RIDER_KIND_CREATURE)); n != 0 {
		t.Errorf("after the undo of the damage the offer is still open (%d)", n)
	}
	if _, err := a.settle(t, a.master, e, id, true); err != nil {
		t.Fatalf("apply the claws again: %v", err)
	}
	if n := len(ridersOfKind(a.get(t, a.master), playv1.HitRiderKind_HIT_RIDER_KIND_CREATURE)); n != 1 {
		t.Errorf("after applying again the offers = %d, want exactly 1", n)
	}
}

// A creature's action is no spell: its save is not against magic (Magic Resistance, Gnome Cunning give no
// advantage), unless the stat block's attack is a spell attack.
func TestCreatureAction_ItsSaveIsNotAgainstMagic(t *testing.T) {
	t.Parallel()
	plan := rules.ActionPlan{Key: "monster:x#breath", Save: &rules.ActionSave{Ability: "dex", DC: 12}}
	if slices.Contains(against.Spell(spellOfPlan(plan)), combat.AgainstMagic) {
		t.Error("a breath's save is against magic")
	}
	if !slices.Contains(against.Spell(link.Spell{Key: "spell:fireball"}), combat.AgainstMagic) {
		t.Error("a spell's save is not against magic")
	}
	spell := rules.ActionPlan{Key: "monster:x#bolt", Attack: &rules.ActionAttack{Spell: true}, Save: plan.Save}
	if spellOfPlan(spell).Nonmagical {
		t.Error("a spell attack action is not read as nonmagical")
	}
}

// riderDamageOf is the damage the last rider line of a log carries, with the lines of the other types.
func riderDamageOf(t *testing.T, l *playv1.ListCombatLogResponse) []*playv1.CombatLogDamage {
	t.Helper()
	var out []*playv1.CombatLogDamage
	for _, en := range logEntries(l) {
		if r := en.GetHitRider(); r != nil && en.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_HIT_RIDER {
			out = nil
			if r.GetDamage() != nil {
				out = append(out, r.GetDamage())
			}
			out = append(out, r.GetMoreDamages()...)
		}
	}
	return out
}

// A spider's bite rider whose save fails opens poison damage as pending damage. The damage was never on the log
// (the rider's event hosted no pending damage, so the roll and the apply that followed found no line): the master
// reads it rolled and applied, Toren's player reads what became of his character, another player reads the amount
// as rolled and no stat block key.
func TestCreatureRider_TheDamageItOpensIsInTheLog(t *testing.T) {
	t.Parallel()
	a, e, spider := creatureFight(t, true, "monster:giant-spider", false)
	r := a.mustAttack(t, a.master, e, spider, "basic:0", "Toren", d20(19))
	a.mustDamage(t, a.master, e, r.GetPendingDamage().GetId(), typedDamage(5))
	if _, err := a.settle(t, a.master, e, r.GetPendingDamage().GetId(), true); err != nil {
		t.Fatalf("apply the bite: %v", err)
	}
	offers := ridersOfKind(a.get(t, a.master), playv1.HitRiderKind_HIT_RIDER_KIND_CREATURE)
	if len(offers) != 1 {
		t.Fatalf("spider offers = %v, want one", offers)
	}
	a.h.roller.queue(1) // Toren fails the save
	if _, err := a.useRider(t, a.master, e, offers[0].GetId(), playv1.HitRiderChoice_HIT_RIDER_CHOICE_APPLY); err != nil {
		t.Fatalf("UseHitRider(apply) error = %v", err)
	}
	for _, who := range []*user{a.master, a.caio, a.ana} {
		got := riderDamageOf(t, a.log(t, who, e))
		if len(got) != 1 || got[0].GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_AWAITING_ROLL {
			t.Fatalf("before the roll, the rider's damage on the log = %v, want one line awaiting the roll", got)
		}
	}
	var poison *playv1.PendingDamage
	for _, p := range a.mustOptions(t, a.master, a.get(t, a.master), spider).GetPendingDamages() {
		if p.GetDamageTypeKey() == "damage-type:poison" {
			poison = p
		}
	}
	if poison == nil {
		t.Fatal("the failed save opened no poison damage")
	}
	a.mustDamage(t, a.master, e, poison.GetId(), typedDamage(9))
	for _, who := range []*user{a.master, a.caio, a.ana} {
		got := riderDamageOf(t, a.log(t, who, e))
		if len(got) != 1 || got[0].GetStatus() == playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_AWAITING_ROLL || got[0].GetAmount() != 9 || got[0].GetDamageTypeKey() != "damage-type:poison" {
			t.Fatalf("after the roll, the rider's damage on the log = %v, want 9 poison rolled", got)
		}
	}
	if _, err := a.settle(t, a.master, e, poison.GetId(), true); err != nil {
		t.Fatalf("apply the poison: %v", err)
	}
	for _, who := range []*user{a.master, a.caio, a.ana} {
		l := a.log(t, who, e)
		got := riderDamageOf(t, l)
		if len(got) != 1 || got[0].GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_APPLIED || got[0].GetAmount() != 9 {
			t.Fatalf("after the apply, the rider's damage on the log = %v, want 9 poison applied", got)
		}
		keys := monsterKeyed(l)
		if (who == a.master) == (len(keys) == 0) {
			t.Errorf("monster keys read by %p = %v, want some for the master alone", who, keys)
		}
	}
}

// The poison a rider opens on a poison-resistant dwarf: the master and Toren's player read what the dwarf took
// (the resistance halved it), another player reads the damage as rolled, as for an attack: the line must not tell
// the resistance. This was read from the apply, which overwrote the amount as rolled for every viewer.
func TestCreatureRider_AResistedDamageShowsAnotherPlayerTheAmountAsRolled(t *testing.T) {
	t.Parallel()
	a, e, spider := creatureFightOf(t, dwarfParty(t), true, "monster:giant-spider", false)
	r := a.mustAttack(t, a.master, e, spider, "basic:0", "Toren", d20(19))
	a.mustDamage(t, a.master, e, r.GetPendingDamage().GetId(), typedDamage(5))
	if _, err := a.settle(t, a.master, e, r.GetPendingDamage().GetId(), true); err != nil {
		t.Fatalf("apply the bite: %v", err)
	}
	offers := ridersOfKind(a.get(t, a.master), playv1.HitRiderKind_HIT_RIDER_KIND_CREATURE)
	a.h.roller.queue(1, 1) // Toren fails the save (a dwarf rolls two dice against poison)
	if _, err := a.useRider(t, a.master, e, offers[0].GetId(), playv1.HitRiderChoice_HIT_RIDER_CHOICE_APPLY); err != nil {
		t.Fatalf("UseHitRider(apply) error = %v", err)
	}
	var poison *playv1.PendingDamage
	for _, p := range a.mustOptions(t, a.master, a.get(t, a.master), spider).GetPendingDamages() {
		if p.GetDamageTypeKey() == "damage-type:poison" {
			poison = p
		}
	}
	if poison == nil {
		t.Fatal("the failed save opened no poison damage")
	}
	a.mustDamage(t, a.master, e, poison.GetId(), typedDamage(10))
	if _, err := a.settle(t, a.master, e, poison.GetId(), true); err != nil {
		t.Fatalf("apply the poison: %v", err)
	}
	amount := func(u *user) int32 {
		d := riderDamageOf(t, a.log(t, u, e))
		if len(d) != 1 || d[0].GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_APPLIED {
			t.Fatalf("the rider's damage = %v, want one applied line", d)
		}
		return d[0].GetAmount()
	}
	if m, o, other := amount(a.master), amount(a.caio), amount(a.ana); m != 5 || o != 5 || other != 10 {
		t.Errorf("amounts: master %d, Toren's player %d, another player %d; want 5, 5 and 10 (as rolled)", m, o, other)
	}
}
