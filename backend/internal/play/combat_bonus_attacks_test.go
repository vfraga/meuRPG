package play

import (
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// Bonus action attacks, Eldritch Blast beams and the critical range in a
// combat. The fixtures are small parties against the canonical fight's goblin
// (AC 12, 7 hit points): a natural 15 hits it and a natural 19 is a critical
// hit only with Improved Critical.

const (
	shortsword = "equipment:shortsword"
	dagger     = "equipment:dagger"
	blast      = "spell:eldritch-blast"
	unarmed    = "attack:unarmed-strike"
	flurry     = "feature:flurry-of-blows"
	patient    = "feature:patient-defense"
	windDash   = "feature:step-of-the-wind:dash"
)

var (
	blockedBonusUsed   = playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_BONUS_ACTION_USED
	blockedNoUses      = playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NO_USES
	blockedAttackFirst = playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ATTACK_ACTION_FIRST
)

// heroWith creates a player's character with a subclass and the choices of
// fighting styles and invocations.
func (u *user) heroWith(t *testing.T, campaignID, name string, cl *charactersv1.ClassLevel, scores *rulesv1.AbilityScores, weapons, cantrips, choices []string) *charactersv1.Character {
	t.Helper()
	sheet := &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
		BaseScores: scores, RaceKey: "race:human", Classes: []*charactersv1.ClassLevel{cl},
		WeaponKeys: weapons, CantripKeys: cantrips, FeatureChoiceKeys: styleOf(cl.GetClassKey(), cl.GetLevel(), choices),
	}}}
	res, err := u.characters.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
		CampaignId: campaignID, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: name, Sheet: sheet,
	}))
	if err != nil {
		t.Fatalf("CreateCharacter(%s) error = %v", name, err)
	}
	return res.Msg.GetCharacter()
}

func classLevel(class string, level int32, subclass string) *charactersv1.ClassLevel {
	cl := &charactersv1.ClassLevel{ClassKey: class, Level: level}
	if subclass != "" {
		cl.Subclass = &charactersv1.ClassLevel_SubclassKey{SubclassKey: subclass}
	}
	return cl
}

func abilities(str, dex, con, wis, cha int32) *rulesv1.AbilityScores {
	return &rulesv1.AbilityScores{Strength: str, Dexterity: dex, Constitution: con, Intelligence: 10, Wisdom: wis, Charisma: cha}
}

// turnOf passes the turns on, the master dropping any damage left, until it is
// the next turn of the combatant with the label. A full round has at most one turn
// per combatant, so one more pass than there are combatants always gets there (a
// fixed 8 was too few for the cave's nine).
func (a *armed) turnOf(t *testing.T, label string) *playv1.Encounter {
	t.Helper()
	for range len(a.get(t, a.master).GetCombatants()) + 1 {
		e := a.get(t, a.master)
		if _, err := a.endTurn(t, a.master, e, true); err != nil {
			t.Fatalf("EndTurn() error = %v", err)
		}
		if e = a.get(t, a.master); e.GetCurrentCombatantId() == a.id(t, label) {
			return e
		}
	}
	t.Fatalf("it never became %s's turn", label)
	return nil
}

// closeFight is the usual start with Brisa next to the goblin too.
func (a *armed) closeFight(t *testing.T) *playv1.Encounter {
	t.Helper()
	return a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: a.goblin.GetId()}},
		npcRolls: []int{3},
		players:  map[string]int32{"Toren": 18, "Pensantus": 10, "Brisa": 1},
		reveal:   []string{"Goblin"},
		at:       map[string][2]int32{"Toren": {3, 3}, "Goblin": {4, 3}, "Pensantus": {10, 3}, "Brisa": {4, 4}},
	})
}

func wantHit(t *testing.T, what string, res *playv1.RollAttackResponse, outcome playv1.AttackOutcome) *playv1.PendingDamage {
	t.Helper()
	if got := res.GetRoll().GetOutcome(); got != outcome {
		t.Fatalf("%s: outcome = %v, want %v", what, got, outcome)
	}
	return res.GetPendingDamage()
}

const (
	hit  = playv1.AttackOutcome_ATTACK_OUTCOME_HIT
	crit = playv1.AttackOutcome_ATTACK_OUTCOME_CRITICAL_HIT
)

// A warlock fires one Eldritch Blast beam per attack roll, as many beams as
// the sheet shows (two at 5th level), each with its own damage, Agonizing Blast
// on every beam; the cast takes the one action, and a further beam is refused.
func TestEldritchBlastBeamsAreEachAnAttackRoll(t *testing.T) {
	t.Parallel()
	a := newArmedWith(t, func(a *armed) {
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 2, abilities(16, 13, 14, 10, 8), []string{battleaxe}, nil)
		a.pens = a.ana.heroWith(t, a.campaignID, "Pensantus", classLevel("class:warlock", 5, ""), abilities(10, 14, 12, 10, 16), nil, []string{blast},
			[]string{"feature:eldritch-invocation-agonizing-blast"})
		a.bri = a.bia.hero(t, a.campaignID, "Brisa", "class:fighter", "race:human", 2, abilities(10, 16, 14, 10, 8), []string{rapier}, nil)
	})
	a.threeAndAGoblin(t)
	e := a.turnOf(t, "Pensantus")
	first := wantHit(t, "first beam", a.mustAttack(t, a.ana, e, "Pensantus", blast, "Goblin", d20(15)), hit)
	// The turn options offer the next beam, with how many are left, and its targets.
	opts := a.mustOptions(t, a.ana, e, "Pensantus")
	if o := attackOption(opts, blast); !o.GetEnabled() || o.GetBeamsLeft() != 1 || targetOf(opts, blast, "Goblin") == nil {
		t.Errorf("Eldritch Blast after the first beam = enabled %v, beams left %d, want enabled with 1 left and a target", o.GetEnabled(), o.GetBeamsLeft())
	}
	// The second beam: another attack roll, another damage; the cast spent the
	// action but not the beams.
	second := wantHit(t, "second beam", a.mustAttack(t, a.ana, e, "Pensantus", blast, "Goblin", d20(15)), hit)
	if first.GetId() == second.GetId() {
		t.Error("the two beams share one damage")
	}
	for i, p := range []*playv1.PendingDamage{first, second} {
		// 1d10 force, plus the Charisma modifier of Agonizing Blast on each beam.
		if p.GetDiceCount() != 1 || p.GetDiceSides() != 10 || p.GetBonus() != 3 {
			t.Errorf("beam %d damage = %dd%d%+d, want 1d10+3", i+1, p.GetDiceCount(), p.GetDiceSides(), p.GetBonus())
		}
	}
	if o := attackOption(a.mustOptions(t, a.ana, e, "Pensantus"), blast); o.GetEnabled() || o.GetReason().GetCode() != rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_ACTION_USED {
		t.Errorf("Eldritch Blast after the last beam = enabled %v, reason %v, want the action's", o.GetEnabled(), o.GetReason().GetCode())
	}
	_, err := a.attack(t, a.ana, e, "Pensantus", blast, "Goblin", d20(15))
	wantBlockedBy(t, "a third beam at 5th level", err, blockedActionUsed)

	// Taking the last beam back gives it again; nothing is left of it.
	l := a.log(t, a.master, e)
	if err := a.undo(t, a.master, e, l.GetUndoableEventId()); err != nil {
		t.Fatalf("UndoLastAction() error = %v", err)
	}
	a.mustAttack(t, a.ana, e, "Pensantus", blast, "Goblin", d20(15))
}

// Improved Critical (Champion 3) makes a weapon attack critical on a natural
// 19; the same roll is an ordinary hit for a fighter without it.
func TestImprovedCriticalMakesANatural19ACriticalHit(t *testing.T) {
	t.Parallel()
	a := newArmedWith(t, func(a *armed) {
		a.toren = a.caio.heroWith(t, a.campaignID, "Toren", classLevel("class:fighter", 3, "subclass:champion"), abilities(16, 13, 14, 10, 8), []string{battleaxe}, nil, nil)
		a.pens = a.ana.hero(t, a.campaignID, "Pensantus", "class:wizard", "race:gnome", 1, abilities(10, 14, 12, 16, 8), nil, []string{fireBolt})
		a.bri = a.bia.hero(t, a.campaignID, "Brisa", "class:fighter", "race:human", 2, abilities(10, 16, 14, 10, 8), []string{rapier}, nil)
	})
	e := a.closeFight(t)
	p := wantHit(t, "champion 19", a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", d20(19)), crit)
	if !p.GetCritical() || p.GetDiceCount() != 2 {
		t.Errorf("the champion's critical damage = %dd%d (critical %v), want twice the battleaxe's dice", p.GetDiceCount(), p.GetDiceSides(), p.GetCritical())
	}
	e = a.turnOf(t, "Brisa")
	if p := wantHit(t, "fighter 19", a.mustAttack(t, a.bia, e, "Brisa", rapier, "Goblin", d20(19)), hit); p.GetCritical() {
		t.Error("a fighter without Improved Critical made a critical hit on a 19")
	}
}

// After the Attack action with a light melee weapon, another light melee
// weapon attacks with the bonus action, once; its damage leaves out the
// ability modifier, unless the character fights with two weapons. A weapon that
// is not light, or the bonus action spent, is refused. Taking the attack back
// gives the bonus action back.
func TestTwoWeaponFightingAttacksWithTheBonusAction(t *testing.T) {
	t.Parallel()
	a := newArmedWith(t, func(a *armed) {
		weapons := []string{shortsword, dagger, "equipment:longsword"}
		a.toren = a.caio.heroWith(t, a.campaignID, "Toren", classLevel("class:fighter", 1, ""), abilities(16, 13, 14, 10, 8), weapons, nil,
			[]string{"feature:fighter-fighting-style-defense"})
		a.pens = a.ana.hero(t, a.campaignID, "Pensantus", "class:wizard", "race:gnome", 1, abilities(10, 14, 12, 16, 8), nil, []string{fireBolt})
		a.bri = a.bia.heroWith(t, a.campaignID, "Brisa", classLevel("class:fighter", 1, ""), abilities(10, 16, 14, 10, 8), weapons, nil,
			[]string{"feature:fighter-fighting-style-two-weapon-fighting"})
	})
	e := a.closeFight(t)

	// Toren, no style: STR 17 gives +3 to the shortsword and none to the off hand.
	if p := wantHit(t, "main hand", a.mustAttack(t, a.caio, e, "Toren", shortsword, "Goblin", d20(15)), hit); p.GetBonus() != 3 {
		t.Errorf("main hand damage bonus = %d, want +3", p.GetBonus())
	}
	// The turn options offer the dagger as the off-hand attack, with the damage
	// the server will roll, and its targets; the longsword is not offered.
	opts := a.mustOptions(t, a.caio, e, "Toren")
	off := attackOption(opts, dagger)
	if !off.GetEnabled() || off.GetBonusRule() != rulesv1.BonusAttackRule_BONUS_ATTACK_RULE_OFF_HAND {
		t.Errorf("dagger after the main hand = enabled %v, rule %v, want the enabled off hand", off.GetEnabled(), off.GetBonusRule())
	}
	if off.GetAttack().GetDamageDice().GetBonus() != 0 || targetOf(opts, dagger, "Goblin") == nil {
		t.Errorf("off hand option damage bonus %d, goblin target %v, want no modifier and a target", off.GetAttack().GetDamageDice().GetBonus(), targetOf(opts, dagger, "Goblin"))
	}
	if sword := attackOption(opts, "equipment:longsword"); sword.GetEnabled() || sword.GetBonusRule() != rulesv1.BonusAttackRule_BONUS_ATTACK_RULE_UNSPECIFIED {
		t.Errorf("longsword after the main hand = enabled %v, rule %v, want a disabled plain attack", sword.GetEnabled(), sword.GetBonusRule())
	}
	_, err := a.attack(t, a.caio, e, "Toren", "equipment:longsword", "Goblin", d20(15))
	wantBlockedBy(t, "a longsword off hand", err, blockedActionUsed)
	if p := wantHit(t, "off hand", a.mustAttack(t, a.caio, e, "Toren", dagger, "Goblin", d20(15)), hit); p.GetBonus() != 0 {
		t.Errorf("off hand damage bonus = %d, want none (no ability modifier)", p.GetBonus())
	}
	_, err = a.attack(t, a.caio, e, "Toren", dagger, "Goblin", d20(15))
	wantBlockedBy(t, "a second off hand attack", err, blockedBonusUsed)
	spent := attackOption(a.mustOptions(t, a.caio, e, "Toren"), dagger)
	if spent.GetEnabled() || spent.GetReason().GetCode() != rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_BONUS_ACTION_USED {
		t.Errorf("off hand with the bonus action spent = enabled %v, reason %v, want BONUS_ACTION_USED", spent.GetEnabled(), spent.GetReason().GetCode())
	}

	// The undo of the off hand attack gives the bonus action back.
	l := a.log(t, a.master, e)
	if err := a.undo(t, a.master, e, l.GetUndoableEventId()); err != nil {
		t.Fatalf("UndoLastAction() error = %v", err)
	}
	a.mustAttack(t, a.caio, e, "Toren", dagger, "Goblin", d20(15))

	// Brisa fights with two weapons: the modifier stays.
	e = a.turnOf(t, "Brisa")
	a.mustAttack(t, a.bia, e, "Brisa", shortsword, "Goblin", d20(15))
	if got := attackOption(a.mustOptions(t, a.bia, e, "Brisa"), dagger).GetAttack().GetDamageDice().GetBonus(); got != 3 {
		t.Errorf("off hand option damage bonus with the style = %d, want +3", got)
	}
	if p := wantHit(t, "style off hand", a.mustAttack(t, a.bia, e, "Brisa", dagger, "Goblin", d20(15)), hit); p.GetBonus() != 3 {
		t.Errorf("off hand damage bonus with the style = %d, want +3", p.GetBonus())
	}
}

// The off-hand attack needs a different light melee weapon (SRD 5.1, "Two-Weapon Fighting"):
// a fighter with one light weapon and a longsword is not offered it as a bonus action.
func TestTwoWeaponFightingNeedsASecondLightWeapon(t *testing.T) {
	t.Parallel()
	a := newArmedWith(t, func(a *armed) {
		a.toren = a.caio.heroWith(t, a.campaignID, "Toren", classLevel("class:fighter", 1, ""), abilities(16, 13, 14, 10, 8), []string{dagger, "equipment:longsword"}, nil,
			[]string{"feature:fighter-fighting-style-defense"})
		a.pens = a.ana.hero(t, a.campaignID, "Pensantus", "class:wizard", "race:gnome", 1, abilities(10, 14, 12, 16, 8), nil, []string{fireBolt})
		a.bri = a.bia.hero(t, a.campaignID, "Brisa", "class:fighter", "race:human", 1, abilities(10, 16, 14, 10, 8), []string{shortsword}, nil)
	})
	e := a.closeFight(t)
	a.mustAttack(t, a.caio, e, "Toren", dagger, "Goblin", d20(15))
	off := attackOption(a.mustOptions(t, a.caio, e, "Toren"), dagger)
	if off.GetEnabled() || off.GetBonusRule() == rulesv1.BonusAttackRule_BONUS_ATTACK_RULE_OFF_HAND {
		t.Errorf("the only dagger after attacking with it = enabled %v, rule %v, want no off-hand offer", off.GetEnabled(), off.GetBonusRule())
	}
	_, err := a.attack(t, a.caio, e, "Toren", dagger, "Goblin", d20(15))
	wantBlockedBy(t, "the same single dagger in the other hand", err, blockedActionUsed)
}

// A monk follows the Attack action with an unarmed strike as a bonus action
// (Martial Arts), and with Flurry of Blows (1 ki point) with two. Patient
// Defense and Step of the Wind spend 1 ki point too, and the points run out.
func TestMonkBonusActionStrikesSpendKi(t *testing.T) {
	t.Parallel()
	a := newArmedWith(t, func(a *armed) {
		a.toren = a.caio.heroWith(t, a.campaignID, "Toren", classLevel("class:monk", 3, ""), abilities(10, 16, 14, 14, 8), nil, nil, nil)
		a.pens = a.ana.hero(t, a.campaignID, "Pensantus", "class:wizard", "race:gnome", 1, abilities(10, 14, 12, 16, 8), nil, []string{fireBolt})
		a.bri = a.bia.hero(t, a.campaignID, "Brisa", "class:fighter", "race:human", 2, abilities(10, 16, 14, 10, 8), []string{rapier}, nil)
	})
	e := a.threeAndAGoblin(t)
	ki := func() int32 { used, _ := resourceUsed(a.vitals(t, a.toren), "ki"); return used }
	strike := func(what string) *playv1.PendingDamage {
		t.Helper()
		return wantHit(t, what, a.mustAttack(t, a.caio, e, "Toren", unarmed, "Goblin", d20(15)), hit)
	}

	// Round 1: Martial Arts. DEX 17 gives +3, and the martial die the 1d4.
	strike("the Attack action")
	opts := a.mustOptions(t, a.caio, e, "Toren")
	if ma := attackOption(opts, unarmed); !ma.GetEnabled() || ma.GetBonusRule() != rulesv1.BonusAttackRule_BONUS_ATTACK_RULE_MARTIAL_ARTS || targetOf(opts, unarmed, "Goblin") == nil {
		t.Errorf("unarmed strike after the Attack action = %v, want the enabled Martial Arts strike with a target", ma)
	}
	p := strike("the Martial Arts strike")
	if p.GetDiceSides() != 4 || p.GetBonus() != 3 {
		t.Errorf("Martial Arts strike damage = d%d%+d, want d4+3 (the modifier stays)", p.GetDiceSides(), p.GetBonus())
	}
	_, err := a.attack(t, a.caio, e, "Toren", unarmed, "Goblin", d20(15))
	wantBlockedBy(t, "a third strike", err, blockedBonusUsed)
	if ki() != 0 {
		t.Errorf("ki used = %d after Martial Arts, want 0", ki())
	}

	// Round 2: Flurry of Blows comes after the Attack action, not before.
	e = a.turnOf(t, "Toren")
	if o := actionOption(a.mustOptions(t, a.caio, e, "Toren"), flurry); o.GetEnabled() || o.GetReason().GetCode() != rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_ATTACK_ACTION_FIRST {
		t.Errorf("Flurry of Blows before the Attack action = enabled %v, reason %v, want ATTACK_ACTION_FIRST", o.GetEnabled(), o.GetReason().GetCode())
	}
	_, err = a.action(t, a.caio, e, "Toren", flurry)
	wantBlockedBy(t, "Flurry of Blows before the Attack action", err, blockedAttackFirst)
	if ki() != 0 {
		t.Errorf("ki used = %d after a refused Flurry, want 0", ki())
	}
	strike("the Attack action")
	if _, err = a.action(t, a.caio, e, "Toren", flurry); err != nil {
		t.Fatalf("Flurry of Blows error = %v", err)
	}
	if ki() != 1 {
		t.Errorf("ki used = %d after Flurry of Blows, want 1", ki())
	}
	flurryLeft := func(want int32) {
		t.Helper()
		o := attackOption(a.mustOptions(t, a.caio, e, "Toren"), unarmed)
		if !o.GetEnabled() || o.GetBonusRule() != rulesv1.BonusAttackRule_BONUS_ATTACK_RULE_FLURRY_OF_BLOWS || o.GetBonusAttacksLeft() != want {
			t.Errorf("unarmed strike = enabled %v, rule %v, left %d, want an enabled Flurry of Blows strike with %d left", o.GetEnabled(), o.GetBonusRule(), o.GetBonusAttacksLeft(), want)
		}
	}
	flurryLeft(2)
	strike("first flurry strike")
	flurryLeft(1)
	strike("second flurry strike")
	_, err = a.attack(t, a.caio, e, "Toren", unarmed, "Goblin", d20(15))
	wantBlockedBy(t, "a third flurry strike", err, blockedBonusUsed)

	// The undo of a flurry strike gives it back.
	l := a.log(t, a.master, e)
	if err := a.undo(t, a.master, e, l.GetUndoableEventId()); err != nil {
		t.Fatalf("UndoLastAction() error = %v", err)
	}
	strike("the strike taken back")

	// Patient Defense, then Step of the Wind: 1 ki each, and monk 3 has 3.
	e = a.turnOf(t, "Toren")
	if _, err = a.action(t, a.caio, e, "Toren", patient); err != nil {
		t.Fatalf("Patient Defense error = %v", err)
	}
	if ki() != 2 {
		t.Errorf("ki used = %d after Patient Defense, want 2", ki())
	}
	e = a.turnOf(t, "Toren")
	if _, err = a.action(t, a.caio, e, "Toren", windDash); err != nil {
		t.Fatalf("Step of the Wind error = %v", err)
	}
	if ki() != 3 {
		t.Errorf("ki used = %d after Step of the Wind, want 3", ki())
	}
	e = a.turnOf(t, "Toren")
	_, err = a.action(t, a.caio, e, "Toren", patient)
	wantBlockedBy(t, "Patient Defense without ki", err, blockedNoUses)
}

func actionOption(o *playv1.GetTurnOptionsResponse, key string) *rulesv1.ActionOption {
	for _, a := range o.GetOptions().GetFeatureActions() {
		if a.GetAction().GetKey() == key {
			return a
		}
	}
	return nil
}
