package play

import (
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// The rules of the gate that both hold tables share, as they are today: one question at a time
// for a character, kept apart for the combatants that share a character. The four hand-wired
// mechanisms of E4 (roll_holds for a weapon attack, inspiration_holds for the rest) each read
// the other table to enforce them, and a unified gate has to keep every line of these tests.

// brisaFight is the halfling cleric Brisa next to the goblin, on her turn; with a die she holds a d8.
func brisaFight(t *testing.T, die bool) (*armed, *playv1.Encounter) {
	t.Helper()
	a := newCastersOf(t, 3, "race:halfling")
	e := a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: a.goblin.GetId()}},
		npcRolls: []int{3},
		players:  map[string]int32{"Toren": 18, "Pensantus": 10, "Brisa": 1},
		reveal:   []string{"Goblin"},
		at:       map[string][2]int32{"Toren": {3, 3}, "Goblin": {4, 3}, "Pensantus": {10, 3}, "Brisa": {4, 4}},
	})
	a.passTo(t, e, "Brisa")
	e = a.get(t, a.master)
	if die {
		a.giveCombatantDie(t, "Brisa", e)
	}
	return a, e
}

// castGuidingBolt casts Guiding Bolt as Brisa's player with the pair of d20 the roller will give (a
// hostile creature stands next to her: disadvantage).
func (a *armed) castGuidingBolt(t *testing.T, e *playv1.Encounter) (*playv1.CastSpellResponse, error) {
	t.Helper()
	return a.cast(t, a.bia, e, "Brisa", guidingBolt, slotOfLevel(1), a.at(t, "Goblin"),
		func(r *playv1.CastSpellRequest) { r.Roll = &playv1.CastSpellRequest_RollInApp{RollInApp: true} })
}

// TestOneQuestionAtATimeAcrossBothHoldTables: a character's roll that waits for its Lucky or Bardic
// Inspiration answer in one table (a weapon attack in roll_holds, a spell attack in inspiration_holds)
// refuses a roll that would wait in the other, with the reason of the question that is open: LUCKY_PENDING
// while the 1 asks, INSPIRATION_PENDING for the die. The refused roll rolls nothing and holds nothing.
func TestOneQuestionAtATimeAcrossBothHoldTables(t *testing.T) {
	t.Parallel()
	pending := playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_LUCKY_PENDING
	asking := playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_INSPIRATION_PENDING
	cases := []struct {
		name        string
		die         bool
		spellFirst  bool
		first, then []int // the faces the app rolls: for the roll that holds, and for the one refused
		want        playv1.ResourceBlockedReason
	}{
		{"a spell attack's Lucky refuses the weapon attack", false, true, []int{1, 13}, nil, pending},
		{"a spell attack's die refuses the weapon attack", true, true, []int{4, 13}, nil, asking},
		{"a weapon attack's Lucky refuses the spell attack", false, false, []int{1}, []int{9, 13}, pending},
		{"a weapon attack's die refuses the spell attack", true, false, []int{4}, []int{9, 13}, asking},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a, e := brisaFight(t, tc.die)
			a.h.roller.queue(tc.first...)
			var err error
			if tc.spellFirst {
				_, err = a.castGuidingBolt(t, e)
			} else {
				_, err = a.attack(t, a.bia, e, "Brisa", maceKey, "Goblin", inAppRoll)
			}
			if err != nil {
				t.Fatalf("the roll that holds: error = %v", err)
			}
			a.h.roller.queue(tc.then...)
			if tc.spellFirst {
				_, err = a.attack(t, a.bia, e, "Brisa", maceKey, "Goblin", d20(15))
			} else {
				_, err = a.castGuidingBolt(t, e)
			}
			wantResourceBlocked(t, err, tc.want)
			if n := a.h.roller.pending(); n != len(tc.then) {
				t.Errorf("the refused roll took %d faces from the roller, want none", len(tc.then)-n)
			}
			a.h.roller.drain()
			rolls := countRows(t, a.h, countRollHolds, e.GetId())
			outside := countRows(t, a.h, countInspirationHold, a.campaignID)
			if rolls+outside != 1 {
				t.Errorf("held rolls = %d in roll_holds and %d in inspiration_holds, want exactly one", rolls, outside)
			}
		})
	}
}

// TestAHeldAttackOfOneNPCCopyDoesNotBlockTheOther: the copies of one NPC share a character_id, but each
// is a combatant of its own with its own economy and, here, its own die: the question of one copy's
// attack never stops the other copy's, and each answer settles its own roll. (The same copy, rolling
// twice, is asked to answer first: that is the one question at a time.)
func TestAHeldAttackOfOneNPCCopyDoesNotBlockTheOther(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: a.goblin.GetId(), Count: 2}},
		npcRolls: []int{18, 18}, // the same total: the two copies take a joint turn
		players:  map[string]int32{"Toren": 5, "Pensantus": 5, "Brisa": 5},
		reveal:   []string{"Goblin 1", "Goblin 2"},
		at:       map[string][2]int32{"Toren": {3, 3}, "Goblin 1": {4, 3}, "Goblin 2": {3, 4}, "Pensantus": {10, 3}, "Brisa": {8, 8}},
	})
	a.giveCombatantDie(t, "Goblin 1", e)
	a.giveCombatantDie(t, "Goblin 2", e)
	first := a.mustAttack(t, a.master, e, "Goblin 1", sword, "Toren", d20(8))
	second, err := a.attack(t, a.master, e, "Goblin 2", sword, "Toren", d20(9))
	if err != nil {
		t.Fatalf("the second copy's attack while the first one waits: error = %v", err)
	}
	if first.GetInspirationOffer() == nil || second.GetInspirationOffer() == nil || first.GetInspirationOffer().GetHoldId() == second.GetInspirationOffer().GetHoldId() {
		t.Fatalf("the questions = %v and %v, want one for each copy", first.GetInspirationOffer(), second.GetInspirationOffer())
	}
	if n := countRows(t, a.h, countRollHolds, e.GetId()); n != 2 {
		t.Errorf("%d held rolls, want 2", n)
	}
	view := a.get(t, a.master)
	for label, hold := range map[string]string{"Goblin 1": first.GetInspirationOffer().GetHoldId(), "Goblin 2": second.GetInspirationOffer().GetHoldId()} {
		if got := byLabel(t, view, label).GetInspirationOffer().GetHoldId(); got != hold {
			t.Errorf("%s reads the question %q, want its own, %q", label, got, hold)
		}
	}
	_, err = a.attack(t, a.master, e, "Goblin 1", sword, "Toren", d20(12))
	wantResourceBlocked(t, err, playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_INSPIRATION_PENDING)

	// Answered in the other order, each answer settles its own d20: 9 + 4 keeps the die, 8 + 4 + 3 uses it.
	kept, err := a.answer(t, a.master, e, second.GetInspirationOffer().GetHoldId(), keepTheDie)
	if err != nil {
		t.Fatalf("the second copy's answer: error = %v", err)
	}
	a.h.roller.queue(3)
	used, err := a.answer(t, a.master, e, first.GetInspirationOffer().GetHoldId(), useDieInApp)
	if err != nil {
		t.Fatalf("the first copy's answer: error = %v", err)
	}
	if r := kept.GetAttack().GetRoll(); r.GetD20().GetFaces()[0] != 9 || r.GetD20().GetTotal() != 13 || len(r.GetBonusDice()) != 0 {
		t.Errorf("the second copy's roll = %v, want the d20 of 9 (13) and no die", r)
	}
	if r := used.GetAttack().GetRoll(); r.GetD20().GetFaces()[0] != 8 || r.GetD20().GetTotal() != 15 || len(r.GetBonusDice()) != 1 {
		t.Errorf("the first copy's roll = %v, want the d20 of 8 and the d8's 3 (15)", r)
	}
}

// halflingRaider is an enemy with a full sheet whose race is halfling, so that its 1 asks Lucky.
func halflingRaider(t *testing.T, a *armed) *charactersv1.Character {
	t.Helper()
	sheet := &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
		BaseScores: &rulesv1.AbilityScores{Strength: 16, Dexterity: 12, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 10}, RaceKey: "race:halfling",
		Classes: []*charactersv1.ClassLevel{{ClassKey: "class:fighter", Level: 2}}, WeaponKeys: []string{battleaxe},
	}}}
	res, err := a.master.characters.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
		CampaignId: a.campaignID, Kind: charactersv1.CharacterKind_CHARACTER_KIND_ENEMY, Name: "Salteador", Sheet: sheet,
	}))
	if err != nil {
		t.Fatalf("CreateCharacter(Salteador) error = %v", err)
	}
	return res.Msg.GetCharacter()
}

// TestLuckyAfterACuttingWordsWindow: the halfling raider's attack on Toren waits for the lore bard's
// Cutting Words window first (nothing is rolled until it is answered); then its typed 1 is held, under
// a key of its own (the request's, with "#window"), with the Lucky question, on the d20 already minus
// the bard's die. Lucky rolls again, the new roll keeps the bard's die and the attack is resolved
// once: the retries of the request, the window's answer and Lucky's answer find what they made.
func TestLuckyAfterACuttingWordsWindow(t *testing.T) {
	t.Parallel()
	var raider *charactersv1.Character
	a := newArmedWith(t, func(a *armed) {
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 5, abilities(16, 13, 14, 10, 8), []string{battleaxe}, nil)
		a.pens = loreBard(t, a)
		a.bri = a.bia.caster(t, a.campaignID, "Brisa", "class:cleric", "race:human", 3, abilities(10, 16, 14, 16, 8), []string{maceKey}, []string{sacredFlame}, nil, []string{cureWounds})
		raider = halflingRaider(t, a)
	})
	e := a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: raider.GetId()}},
		npcRolls: []int{3},
		players:  map[string]int32{"Toren": 18, "Reator": 10, "Brisa": 1},
		reveal:   []string{"Salteador"},
		at:       map[string][2]int32{"Toren": {3, 3}, "Salteador": {4, 3}, "Reator": {4, 4}, "Brisa": {8, 8}},
	})
	e = a.passTo(t, e, "Salteador")
	key := newKey()
	attack := func(r *playv1.RollAttackRequest) { d20(1)(r); r.IdempotencyKey = key }
	held := a.mustAttack(t, a.master, e, "Salteador", battleaxe, "Toren", attack)
	if !held.GetRoll().GetHeldForReaction() || held.GetLuckyOffer() != nil {
		t.Fatalf("the attack = %v, want it held for the bard's window, with no question yet", held)
	}
	w := a.windowOf(t, a.ana, playv1.ReactionKind_REACTION_KIND_CUTTING_WORDS)
	if w == nil {
		t.Fatal("the bard has no window")
	}
	a.mustAnswer(t, a.ana, e, w.GetId(), func(r *playv1.AnswerReactionRequest) {
		r.Answer, r.Roll = playv1.ReactionChoice_REACTION_CHOICE_USE, &playv1.AnswerReactionRequest_Typed{Typed: 3}
	})
	offer := byLabel(t, a.get(t, a.master), "Salteador").GetLuckyOffer()
	if offer == nil || offer.GetD20().GetFaces()[0] != 1 {
		t.Fatalf("the raider's question = %v, want Lucky's, about the 1", offer)
	}
	var holdKey string
	if err := a.h.pool.QueryRow(t.Context(), `SELECT idempotency_key FROM roll_holds WHERE encounter_id = $1`, e.GetId()).Scan(&holdKey); err != nil || holdKey != key+windowHoldSuffix {
		t.Errorf("the hold's key = %q (%v), want the request's with %q", holdKey, err, windowHoldSuffix)
	}
	// Another attack waits for Lucky's answer, and no second window opens.
	_, err := a.attack(t, a.master, e, "Salteador", battleaxe, "Toren", d20(19))
	wantResourceBlocked(t, err, playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_LUCKY_PENDING)
	if w := a.windowOf(t, a.ana, playv1.ReactionKind_REACTION_KIND_CUTTING_WORDS); w != nil {
		t.Errorf("a second window opened while Lucky's question waited: %v", w)
	}

	answerKey := newKey()
	withKey := func(r *playv1.AnswerLuckyRequest) { rerollTyped(15)(r); r.IdempotencyKey = answerKey }
	res, err := a.answerLucky(t, a.master, e, offer.GetHoldId(), withKey)
	if err != nil {
		t.Fatalf("AnswerLucky() error = %v", err)
	}
	roll := res.GetAttack().GetRoll()
	if roll.GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_HIT || roll.GetD20().GetFaces()[0] != 15 || res.GetAttack().GetLuckyOffer() != nil {
		t.Fatalf("the attack = %v, want a hit with the new 15 and no question", roll)
	}
	wantLuckyReroll(t, "the resolved d20", roll.GetD20(), 0, 15)
	total := roll.GetD20().GetTotal()
	// The same answer again gives the same attack; another answer is refused; one attack is written.
	again, err := a.answerLucky(t, a.master, e, offer.GetHoldId(), withKey)
	if err != nil || again.GetAttack().GetRoll().GetD20().GetTotal() != total {
		t.Errorf("the answer again = %v, %v; want the same attack (%d)", again.GetAttack().GetRoll().GetD20(), err, total)
	}
	if _, err := a.answerLucky(t, a.master, e, offer.GetHoldId(), keepTheOne); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Errorf("another answer = %v, want failed_precondition", err)
	}
	// Two events stand for the attack: the one that held it for the window, under the request's key,
	// and the attack the answer wrote, once.
	const attackEvents = `SELECT count(*) FROM session_events WHERE kind = 'attack_rolled' AND encounter_id = $1`
	if n := countRows(t, a.h, attackEvents, e.GetId()); n != 2 {
		t.Errorf("attack events = %d, want 2 (the held one and the resolved one)", n)
	}
	if n := countRows(t, a.h, attackEvents+` AND payload->>'outcome' = 'hit'`, e.GetId()); n != 1 {
		t.Errorf("resolved attack events = %d, want 1", n)
	}
}
