package play

import (
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

const acidArrow = "spell:acid-arrow"

// A contest start, a contest answer or a Hide that the master's reaction window held (the table asks for the master's check on
// every roll against an enemy) is run again when the window is answered. When that replay holds the d20 for the player's
// Bardic Inspiration die, the player's answer must write what the held request would have written: the contest, its answer or
// the attempt, and its event, once (the event that stands for the held request, which never reaches the log, is not counted).

// windowReplayAsks answers the window: it reads the master's check the request waits in, has the master answer it, and returns the die's
// question the replay put to the player (its hold id).
func (a *armed) windowReplayAsks(t *testing.T, e *playv1.Encounter) string {
	t.Helper()
	w := a.windowOf(t, a.master, playv1.ReactionKind_REACTION_KIND_MASTER_CHECK)
	if w == nil {
		t.Fatalf("the master's windows = %v, want the check the roll waits in", a.windows(t, a.master))
	}
	a.mustAnswer(t, a.master, e, w.GetId(), passAnswer)
	offers := a.listHeldRolls(t, a.caio)
	if len(offers) != 1 || offers[0].GetDie() == nil {
		t.Fatalf("Toren's questions after the window = %v, want the die's", offers)
	}
	return offers[0].GetHoldId()
}

// countOf counts what a query counts.
func (a *armed) countOf(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := a.h.pool.QueryRow(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

// retryTheAnswer answers the die's question again with the same key, which a flaky network does: the answer gives the stored
// outcome (not an error) and rolls nothing (the roller's queue holds a sentinel that must stay there).
func (a *armed) retryTheAnswer(t *testing.T, roller *scriptedRoller, holdID, key string, first *playv1.AnswerHeldRollResponse) {
	t.Helper()
	a.retryTheAnswerOf(t, roller, a.caio, holdID, key, dieAnswer(true), first)
}

// retryTheAnswerOf is retryTheAnswer for the answer u gave, as edit says.
func (a *armed) retryTheAnswerOf(t *testing.T, roller *scriptedRoller, u *user, holdID, key string, edit func(*playv1.AnswerHeldRollRequest), first *playv1.AnswerHeldRollResponse) {
	t.Helper()
	roller.queue(heldSentinel)
	again, err := a.answerHeldRollKey(t, u, holdID, key, edit)
	if err != nil {
		t.Fatalf("the answer again: error = %v, want the stored outcome", err)
	}
	if !proto.Equal(again, first) {
		t.Errorf("the answer again = %v, want what it gave before: %v", again, first)
	}
	if n := roller.pending(); n != 1 {
		t.Errorf("the answer again took %d faces from the roller, want none", 1-n)
	}
	roller.drain()
}

func TestAContestStartThatAWindowHeldIsWrittenWhenTheDieIsAnswered(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	a.setRules(t, enemyReactionsAlways)
	c := a.arena(t, arenaPlan{})
	a.giveCombatantDie(t, "Toren", c.e)

	c.h.roller.queue(15)
	held, err := c.startContest(t, a.caio, func(r *playv1.StartContestRequest) {
		r.InitiatorId, r.TargetId, r.Purpose = c.id(t, "Toren"), c.id(t, c.hob), playv1.ContestPurpose_CONTEST_PURPOSE_GRAPPLE
	})
	if err != nil || held.GetContest() != nil || held.GetHeldRollOffer() != nil {
		t.Fatalf("StartContest() = %v, %v; want it held in the master's window", held, err)
	}
	holdID := a.windowReplayAsks(t, c.e)

	c.h.roller.queue(4) // Athletics +3: 15 + 3 + the die 4 = 22
	key := newKey()
	out, err := a.answerHeldRollKey(t, a.caio, holdID, key, dieAnswer(true))
	if err != nil {
		t.Fatalf("AnswerHeldRoll() error = %v", err)
	}
	a.retryTheAnswer(t, c.h.roller, holdID, key, out)
	if roll := out.GetContest().GetContest().GetInitiatorRoll(); roll.GetTotal() != 22 || roll.GetFaces()[0] != 15 {
		t.Errorf("the answer's contest roll = %v, want d20 15 + 3 + the die 4 = 22", roll)
	}
	if n := a.countOf(t, `SELECT count(*) FROM combat_contests`); n != 1 {
		t.Errorf("combat_contests rows = %d, want 1", n)
	}
	if n := a.countOf(t, `SELECT count(*) FROM session_events WHERE kind = $1 AND NOT COALESCE((payload->'reaction'->>'hold')::bool, false)`, eventContestStarted); n != 1 {
		t.Errorf("%s events = %d, want 1", eventContestStarted, n)
	}
	if got := c.state(t, a.master).GetContests(); len(got) != 1 || got[0].GetInitiatorRoll().GetTotal() != 22 {
		t.Errorf("the contests the master reads = %v, want one with the die counted", got)
	}
	if n := a.countOf(t, `SELECT count(*) FROM combatants WHERE id = $1 AND inspiration_sides IS NOT NULL`, c.id(t, "Toren")); n != 0 {
		t.Error("Toren still holds the die he used")
	}
}

func TestAContestAnswerThatAWindowHeldIsWrittenWhenTheDieIsAnswered(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	a.setRules(t, enemyReactionsAlways)
	c := a.arena(t, arenaPlan{})
	c.advance(t, c.hob)
	a.giveCombatantDie(t, "Toren", c.e)

	c.h.roller.queue(10) // the Hobgoblin's Athletics 10 + 1 = 11
	started := c.mustStart(t, a.master, func(r *playv1.StartContestRequest) {
		r.InitiatorId, r.TargetId, r.Purpose = c.id(t, c.hob), c.id(t, "Toren"), playv1.ContestPurpose_CONTEST_PURPOSE_GRAPPLE
	})
	// Toren's Athletics +3: 5 + 3 = 8 loses, 8 + the die 4 = 12 wins.
	held, err := c.respond(t, a.caio, started.GetContest().GetId(), playv1.ContestSkill_CONTEST_SKILL_ATHLETICS, 5)
	if err != nil || held.GetContest() != nil || held.GetHeldRollOffer() != nil {
		t.Fatalf("RespondContest() = %v, %v; want it held in the master's window", held, err)
	}
	holdID := a.windowReplayAsks(t, c.e)

	c.h.roller.queue(4)
	key := newKey()
	out, err := a.answerHeldRollKey(t, a.caio, holdID, key, dieAnswer(true))
	if err != nil {
		t.Fatalf("AnswerHeldRoll() error = %v", err)
	}
	a.retryTheAnswer(t, c.h.roller, holdID, key, out)
	answered := out.GetContestAnswer().GetContest()
	if answered.GetDefenderRoll().GetTotal() != 12 || answered.GetWinner() != playv1.ContestWinner_CONTEST_WINNER_DEFENDER {
		t.Errorf("answer = %v, want Toren to win with 5 + 3 + the die 4 = 12 against 11", answered)
	}
	if n := a.countOf(t, `SELECT count(*) FROM combat_contests WHERE defender_roll IS NOT NULL`); n != 1 {
		t.Errorf("combat_contests rows with the defender's roll = %d, want 1", n)
	}
	if n := a.countOf(t, `SELECT count(*) FROM session_events WHERE kind = $1 AND NOT COALESCE((payload->'reaction'->>'hold')::bool, false)`, eventContestResolved); n != 1 {
		t.Errorf("%s events = %d, want 1", eventContestResolved, n)
	}
	if got := c.state(t, a.master).GetContests(); len(got) != 1 || got[0].GetDefenderRoll().GetTotal() != 12 {
		t.Errorf("the contests the master reads = %v, want one with Toren's roll", got)
	}
}

func TestAHideThatAWindowHeldIsWrittenWhenTheDieIsAnswered(t *testing.T) {
	t.Parallel()
	a := newSneaks(t)
	a.setRules(t, enemyReactionsAlways)
	c := a.arena(t, arenaPlan{})
	a.giveCombatantDie(t, "Toren", c.e)

	held, err := c.hide(t, a.caio, "Toren", cunningHide, 7)
	if err != nil || held.GetAttempt() != nil || held.GetHeldRollOffer() != nil {
		t.Fatalf("Hide() = %v, %v; want it held in the master's window", held, err)
	}
	holdID := a.windowReplayAsks(t, c.e)

	c.h.roller.queue(4) // Stealth +3: 7 + 3 + the die 4 = 14
	key := newKey()
	out, err := a.answerHeldRollKey(t, a.caio, holdID, key, dieAnswer(true))
	if err != nil {
		t.Fatalf("AnswerHeldRoll() error = %v", err)
	}
	a.retryTheAnswer(t, c.h.roller, holdID, key, out)
	if attempt := out.GetHide().GetAttempt(); attempt.GetRoll().GetTotal() != 14 || attempt.GetRoll().GetFaces()[0] != 7 {
		t.Errorf("the attempt = %v, want d20 7 + 3 + the die 4 = 14", attempt)
	}
	if n := a.countOf(t, `SELECT count(*) FROM combatants WHERE id = $1 AND inspiration_sides IS NOT NULL`, c.id(t, "Toren")); n != 0 {
		t.Error("Toren still holds the die he used")
	}
	if n := a.countOf(t, `SELECT count(*) FROM combat_hide_attempts`); n != 1 {
		t.Errorf("combat_hide_attempts rows = %d, want 1", n)
	}
	if n := a.countOf(t, `SELECT count(*) FROM session_events WHERE kind = $1 AND NOT COALESCE((payload->'reaction'->>'hold')::bool, false)`, eventHideAttempted); n != 1 {
		t.Errorf("%s events = %d, want 1", eventHideAttempted, n)
	}
	if got := c.state(t, a.master).GetHideAttempts(); len(got) != 1 || got[0].GetRoll().GetTotal() != 14 {
		t.Errorf("the hide attempts the master reads = %v, want one with the die counted", got)
	}
}

// A cast with a spell attack that the master's window held (a player's spell on an enemy, with "Sempre") is replayed when the
// window is answered, and a caster that is to be asked about the d20 is asked then, as a weapon attack is: the replay keeps the d20
// with the request, the question under the request's key and marked as the window's (window_hold), and nothing is spent or written
// until the player answers. The answer casts the spell with that d20, once, under the answer's own key.
//
// spellAfterAWindow is Pensantus's turn in the arena, with the master's check asked of every roll against an enemy; race is hers.
func spellAfterAWindow(t *testing.T, race string) (*armed, *cx) {
	t.Helper()
	a := newArmedWith(t, func(a *armed) {
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 2, scores16(), []string{battleaxe}, nil)
		a.pens = a.ana.caster(t, a.campaignID, "Pensantus", "class:wizard", race, 3,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 8}, nil, []string{fireBolt},
			[]string{acidArrow}, []string{acidArrow})
		a.bri = a.bia.hero(t, a.campaignID, "Brisa", "class:fighter", "race:human", 2, scores16(), []string{rapier}, nil)
	})
	a.setRules(t, enemyReactionsAlways)
	c := a.arena(t, arenaPlan{})
	c.advance(t, "Pensantus")
	return a, c
}

// castInTheWindow casts Acid Arrow at the goblin with the d20 typed, and answers the master's check the cast waits in. It returns the
// key of the cast.
func (a *armed) castInTheWindow(t *testing.T, c *cx, face int32) string {
	t.Helper()
	key := newKey()
	held, err := a.castKey(t, a.ana, c.e, "Pensantus", acidArrow, slotOfLevel(2), a.at(t, "Goblin"), d20Face(face), key)
	if err != nil || held.GetCast() != nil || held.GetHeldRollOffer() != nil {
		t.Fatalf("CastSpell() = %v, %v; want it held in the master's window", held, err)
	}
	w := a.windowOf(t, a.master, playv1.ReactionKind_REACTION_KIND_MASTER_CHECK)
	if w == nil {
		t.Fatalf("the master's windows = %v, want the check the cast waits in", a.windows(t, a.master))
	}
	a.mustAnswer(t, a.master, c.e, w.GetId(), passAnswer)
	return key
}

// wantTheQuestionOfTheReplay checks the replay of the cast put one question to Pensantus's player, kept under the cast's key as the
// window's own, and that the cast spent and wrote nothing yet. It returns the hold's id.
func (a *armed) wantTheQuestionOfTheReplay(t *testing.T, _ *cx, key string) *playv1.HeldRollOffer {
	t.Helper()
	offers := a.listHeldRolls(t, a.ana)
	if len(offers) != 1 || offers[0].GetKind() != playv1.HeldRollKind_HELD_ROLL_KIND_SPELL_ATTACK {
		t.Fatalf("Pensantus's questions after the window = %v, want one, about the spell attack", offers)
	}
	var kind string
	var window, unanswered bool
	if err := a.h.pool.QueryRow(t.Context(), `SELECT kind, window_hold, answer_key IS NULL FROM inspiration_holds WHERE idempotency_key = $1`, key).Scan(&kind, &window, &unanswered); err != nil {
		t.Fatalf("read the question kept under the cast's key: %v", err)
	}
	if kind != string(heldSpellAttack) || !window || !unanswered {
		t.Errorf("the hold = kind %q, window_hold %v, open %v; want a spell attack, the window's, open", kind, window, unanswered)
	}
	if n := a.countOf(t, `SELECT count(*) FROM session_events WHERE kind = $1 AND NOT COALESCE((payload->'reaction'->>'hold')::bool, false)`, eventSpellCast); n != 0 {
		t.Errorf("%s events before the answer = %d, want 0", eventSpellCast, n)
	}
	if byLabel(t, a.get(t, a.ana), "Pensantus").GetActionUsed() {
		t.Error("the action was spent before the player answered")
	}
	return offers[0]
}

// wantTheCastOnce checks the answer settled the cast once: one event that is no hold, and the action spent.
func (a *armed) wantTheCastOnce(t *testing.T, _ *cx) {
	t.Helper()
	if n := a.countOf(t, `SELECT count(*) FROM session_events WHERE kind = $1 AND NOT COALESCE((payload->'reaction'->>'hold')::bool, false)`, eventSpellCast); n != 1 {
		t.Errorf("%s events = %d, want 1", eventSpellCast, n)
	}
	if !byLabel(t, a.get(t, a.ana), "Pensantus").GetActionUsed() {
		t.Error("the action is not spent after the answer")
	}
}

// TestASpellAttackCastThatAWindowHeldAsksForTheDieAndUsesIt: Pensantus holds a Bardic Inspiration die; the d20 is 15 and her bonus +6.
// The die's 4 is added: 15 + 6 + 4 = 25. The same answer again gives the same cast and rolls nothing; the cast is taken back with the
// die returned, as any cast's.
func TestASpellAttackCastThatAWindowHeldAsksForTheDieAndUsesIt(t *testing.T) {
	t.Parallel()
	a, c := spellAfterAWindow(t, "race:gnome")
	a.giveCombatantDie(t, "Pensantus", c.e)
	key := a.castInTheWindow(t, c, 15)
	offer := a.wantTheQuestionOfTheReplay(t, c, key)
	if offer.GetDie() == nil || offer.GetD20().GetFaces()[0] != 15 {
		t.Errorf("the question = %v, want the die's, about the d20 15", offer)
	}
	// The cast again, with the request's key: the answer it had, nothing written.
	events := a.countOf(t, `SELECT count(*) FROM session_events`)
	if again, err := a.castKey(t, a.ana, c.e, "Pensantus", acidArrow, slotOfLevel(2), a.at(t, "Goblin"), d20Face(15), key); err != nil || again.GetCast() != nil {
		t.Errorf("the cast again = %v, %v; want it still held", again, err)
	}
	if n := a.countOf(t, `SELECT count(*) FROM session_events`); n != events {
		t.Errorf("the cast again wrote %d events", n-events)
	}

	c.h.roller.queue(4)
	answerKey := newKey()
	out, err := a.answerHeldRollKey(t, a.ana, offer.GetHoldId(), answerKey, dieAnswer(true))
	if err != nil {
		t.Fatalf("AnswerHeldRoll() error = %v", err)
	}
	a.retryTheAnswerOf(t, c.h.roller, a.ana, offer.GetHoldId(), answerKey, dieAnswer(true), out)
	targets := out.GetSpellCast().GetCast().GetTargets()
	if len(targets) != 1 || targets[0].GetAttackRoll().GetFaces()[0] != 15 || targets[0].GetAttackRoll().GetTotal() != 25 ||
		len(targets[0].GetBonusDice()) != 1 || targets[0].GetBonusDice()[0].GetFace() != 4 {
		t.Fatalf("the cast = %v, want the d20 15 + 6 + the die 4 = 25", out.GetSpellCast().GetCast())
	}
	a.wantTheCastOnce(t, c)
	if n := a.countOf(t, `SELECT count(*) FROM combatants WHERE id = $1 AND inspiration_sides IS NOT NULL`, c.id(t, "Pensantus")); n != 0 {
		t.Error("Pensantus still holds the die she used")
	}
	if n := a.countOf(t, `SELECT count(*) FROM inspiration_holds WHERE idempotency_key = $1`, key); n != 1 {
		t.Errorf("the holds under the cast's key = %d, want 1", n)
	}
	// The undo of the cast gives the die and the action back.
	a.undoLast(t, a.get(t, a.master))
	if n := a.countOf(t, `SELECT count(*) FROM combatants WHERE id = $1 AND inspiration_sides IS NOT NULL`, c.id(t, "Pensantus")); n != 1 {
		t.Error("the undo did not give the die back")
	}
	if byLabel(t, a.get(t, a.ana), "Pensantus").GetActionUsed() {
		t.Error("the undo did not give the action back")
	}
	if n := a.countOf(t, `SELECT count(*) FROM session_events WHERE kind = $1`, eventActionUndone); n != 1 {
		t.Errorf("%s events = %d, want 1", eventActionUndone, n)
	}
	if n := a.countOf(t, `SELECT count(*) FROM pending_damages`); n != 0 {
		t.Errorf("pending damages after the undo = %d, want 0", n)
	}
}

// TestASpellAttackCastThatAWindowHeldAsksForTheDieAndKeepsIt: the player keeps the die: 15 + 6 = 21, and the die stays.
func TestASpellAttackCastThatAWindowHeldAsksForTheDieAndKeepsIt(t *testing.T) {
	t.Parallel()
	a, c := spellAfterAWindow(t, "race:gnome")
	a.giveCombatantDie(t, "Pensantus", c.e)
	key := a.castInTheWindow(t, c, 15)
	offer := a.wantTheQuestionOfTheReplay(t, c, key)

	answerKey := newKey()
	out, err := a.answerHeldRollKey(t, a.ana, offer.GetHoldId(), answerKey, dieAnswer(false))
	if err != nil {
		t.Fatalf("AnswerHeldRoll() error = %v", err)
	}
	a.retryTheAnswerOf(t, c.h.roller, a.ana, offer.GetHoldId(), answerKey, dieAnswer(false), out)
	targets := out.GetSpellCast().GetCast().GetTargets()
	if len(targets) != 1 || targets[0].GetAttackRoll().GetTotal() != 21 || len(targets[0].GetBonusDice()) != 0 {
		t.Fatalf("the cast = %v, want the d20 15 + 6 = 21 and no die", out.GetSpellCast().GetCast())
	}
	a.wantTheCastOnce(t, c)
	if n := a.countOf(t, `SELECT count(*) FROM combatants WHERE id = $1 AND inspiration_sides IS NOT NULL`, c.id(t, "Pensantus")); n != 1 {
		t.Error("Pensantus lost the die she kept")
	}
}

// TestASpellAttackCastThatAWindowHeldAsksLuckyAboutAOne: a halfling's 1 is asked about when the replay rolls it, and rolled again to 15
// the cast lands on the 15.
func TestASpellAttackCastThatAWindowHeldAsksLuckyAboutAOne(t *testing.T) {
	t.Parallel()
	a, c := spellAfterAWindow(t, "race:halfling")
	key := a.castInTheWindow(t, c, 1)
	offer := a.wantTheQuestionOfTheReplay(t, c, key)
	if offer.GetLucky() == nil || offer.GetD20().GetFaces()[0] != 1 {
		t.Fatalf("the question = %v, want Lucky's, about the 1", offer)
	}
	answerKey := newKey()
	out, err := a.answerHeldRollKey(t, a.ana, offer.GetHoldId(), answerKey, luckyAnswer(15))
	if err != nil {
		t.Fatalf("AnswerHeldRoll() error = %v", err)
	}
	a.retryTheAnswerOf(t, c.h.roller, a.ana, offer.GetHoldId(), answerKey, luckyAnswer(15), out)
	targets := out.GetSpellCast().GetCast().GetTargets()
	if len(targets) != 1 || targets[0].GetAttackRoll().GetFaces()[0] != 15 || targets[0].GetAttackRoll().GetTotal() != 20 {
		t.Fatalf("the cast = %v, want the 1 rolled again to 15, 15 + 5 = 20 (a halfling has no gnome's Intelligence)", out.GetSpellCast().GetCast())
	}
	a.wantTheCastOnce(t, c)
}

// TestASpellAttackCastThatAWindowHeldAsksNothingOfACasterWithoutADie: no die and no Lucky: the replay writes the cast at once, once.
func TestASpellAttackCastThatAWindowHeldAsksNothingOfACasterWithoutADie(t *testing.T) {
	t.Parallel()
	a, c := spellAfterAWindow(t, "race:gnome")
	a.castInTheWindow(t, c, 15)
	if offers := a.listHeldRolls(t, a.ana); len(offers) != 0 {
		t.Errorf("Pensantus's questions after the window = %v, want none", offers)
	}
	if n := a.countOf(t, `SELECT count(*) FROM inspiration_holds`); n != 0 {
		t.Errorf("inspiration_holds rows = %d, want 0", n)
	}
	a.wantTheCastOnce(t, c)
}

// loreBardEnemy is an enemy with a full sheet that is a bard of the College of Lore, so that it may cut a roll with Cutting Words.
func loreBardEnemy(t *testing.T, a *armed) *charactersv1.Character {
	t.Helper()
	sheet := &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
		BaseScores: abilities(10, 14, 12, 10, 16), RaceKey: "race:human",
		Classes: []*charactersv1.ClassLevel{classLevel("class:bard", 3, "subclass:lore")}, WeaponKeys: []string{rapier},
	}}}
	res, err := a.master.characters.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
		CampaignId: a.campaignID, Kind: charactersv1.CharacterKind_CHARACTER_KIND_ENEMY, Name: "Bardo", Sheet: sheet,
	}))
	if err != nil {
		t.Fatalf("CreateCharacter(Bardo) error = %v", err)
	}
	return res.Msg.GetCharacter()
}

// TestASpellAttackCutByAnEnemyBardAsksForTheDieOnTheCutRoll: an enemy bard's Cutting Words holds Pensantus's spell attack, and the master
// answers it with a 5. The replay keeps the d20 15 with the bard's die off it (15 + 6 - 5), and the question is the window's own; the
// answer that uses the die 4 settles the cast with the cut still off: 15 + 6 - 5 + 4 = 20, not 25.
func TestASpellAttackCutByAnEnemyBardAsksForTheDieOnTheCutRoll(t *testing.T) {
	t.Parallel()
	var bard *charactersv1.Character
	a := newArmedWith(t, func(a *armed) {
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 2, scores16(), []string{battleaxe}, nil)
		a.pens = a.ana.caster(t, a.campaignID, "Pensantus", "class:wizard", "race:gnome", 3,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 8}, nil, []string{fireBolt},
			[]string{acidArrow}, []string{acidArrow})
		a.bri = a.bia.hero(t, a.campaignID, "Brisa", "class:fighter", "race:human", 2, scores16(), []string{rapier}, nil)
		bard = loreBardEnemy(t, a)
	})
	e := a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: bard.GetId()}},
		npcRolls: []int{3},
		players:  map[string]int32{"Toren": 18, "Pensantus": 10, "Brisa": 1},
		reveal:   []string{"Bardo"},
		at:       map[string][2]int32{"Toren": {3, 3}, "Bardo": {4, 3}, "Pensantus": {10, 3}, "Brisa": {4, 9}},
	})
	e = a.passTo(t, e, "Pensantus")
	a.giveCombatantDie(t, "Pensantus", e)

	key := newKey()
	held, err := a.castKey(t, a.ana, e, "Pensantus", acidArrow, slotOfLevel(2), a.at(t, "Bardo"), d20Face(15), key)
	if err != nil || held.GetCast() != nil || held.GetHeldRollOffer() != nil {
		t.Fatalf("CastSpell() = %v, %v; want it held in the bard's window", held, err)
	}
	w := a.windowOf(t, a.master, playv1.ReactionKind_REACTION_KIND_CUTTING_WORDS)
	if w == nil {
		t.Fatalf("the master's windows = %v, want the bard's Cutting Words", a.windows(t, a.master))
	}
	a.mustAnswer(t, a.master, e, w.GetId(), func(r *playv1.AnswerReactionRequest) {
		r.Answer, r.Roll = playv1.ReactionChoice_REACTION_CHOICE_USE, &playv1.AnswerReactionRequest_Typed{Typed: 5}
	})
	offers := a.listHeldRolls(t, a.ana)
	if len(offers) != 1 || offers[0].GetDie() == nil || offers[0].GetD20().GetTotal() != 16 {
		t.Fatalf("Pensantus's questions after the window = %v, want the die's, about the d20 15 + 6 - 5 = 16", offers)
	}
	if n := a.countOf(t, `SELECT count(*) FROM inspiration_holds WHERE idempotency_key = $1 AND window_hold`, key); n != 1 {
		t.Errorf("the window's holds under the cast's key = %d, want 1", n)
	}

	a.h.roller.queue(4)
	out, err := a.answerHeldRoll(t, a.ana, offers[0].GetHoldId(), dieAnswer(true))
	if err != nil {
		t.Fatalf("AnswerHeldRoll() error = %v", err)
	}
	targets := out.GetSpellCast().GetCast().GetTargets()
	if len(targets) != 1 || targets[0].GetAttackRoll().GetTotal() != 20 || len(targets[0].GetBonusDice()) != 1 {
		t.Fatalf("the cast = %v, want the d20 15 + 6 - 5 + the die 4 = 20", out.GetSpellCast().GetCast())
	}
}

// TestASpellAttackIsRefusedWhileTheCastersOtherQuestionWaits: Pensantus holds a die and her first spell attack waits for her answer. A second
// cast, which the master's window would hold, is refused at once as a weapon attack is (INSPIRATION_PENDING): she does not wait for the window to learn
// it, no window opens, nothing is spent (no slot, no action), and the first question stays open and answerable.
func TestASpellAttackIsRefusedWhileTheCastersOtherQuestionWaits(t *testing.T) {
	t.Parallel()
	a, c := spellAfterAWindow(t, "race:gnome")
	a.setRules(t, func(r *campaignsv1.TableRules) {
		r.EnemyReactions = campaignsv1.EnemyReactionsRule_ENEMY_REACTIONS_RULE_ONLY_WHEN_POSSIBLE
	})
	a.giveCombatantDie(t, "Pensantus", c.e)
	first, err := a.cast(t, a.ana, c.e, "Pensantus", acidArrow, slotOfLevel(2), a.at(t, "Goblin"), d20Face(15))
	if err != nil || first.GetHeldRollOffer() == nil {
		t.Fatalf("the first CastSpell() = %v, %v; want the die's question", first, err)
	}
	a.setRules(t, enemyReactionsAlways)
	before := a.stableVitals(t)
	// The spend of an Inspiração the caster has none of would be refused too, but the open question comes first, as it does for an attack.
	_, err = a.cast(t, a.ana, c.e, "Pensantus", acidArrow, slotOfLevel(2), a.at(t, "Goblin"), func(r *playv1.CastSpellRequest) { d20Face(12)(r); r.SpendFavor = true })
	wantResourceBlocked(t, err, playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_INSPIRATION_PENDING)
	if w := a.windowOf(t, a.master, playv1.ReactionKind_REACTION_KIND_MASTER_CHECK); w != nil {
		t.Errorf("the refused cast left the window %v", w)
	}
	if byLabel(t, a.get(t, a.ana), "Pensantus").GetActionUsed() {
		t.Error("the action was spent by a cast that waits")
	}
	if got := a.stableVitals(t); got != before {
		t.Errorf("the refused cast changed her vitals (slots, resources, hit points):\nbefore %s\nafter  %s", before, got)
	}
	if n := a.countOf(t, `SELECT count(*) FROM inspiration_holds WHERE answer_key IS NULL`); n != 1 {
		t.Errorf("open questions = %d, want the first alone", n)
	}
	if out, err := a.answerHeldRoll(t, a.ana, first.GetHeldRollOffer().GetHoldId(), dieAnswer(false)); err != nil || out.GetSpellCast().GetCast() == nil {
		t.Errorf("the first question's answer = %v, %v; want the cast", out, err)
	}
}

// TestACounterspelledSpellAttackAsksNothingAndRollsNothing: Brisa holds a die and casts Guiding Bolt; the Mage's Counterspell holds the cast and
// counters it. The replay writes the countered cast at once, as it did before the cast asked about its d20 when a window held it: no d20 rolled
// and kept, no question to the player, one countered cast event, the slot and the action spent once, and no damage.
func TestACounterspelledSpellAttackAsksNothingAndRollsNothing(t *testing.T) {
	t.Parallel()
	a, e := mageFight(t)
	e = a.passTo(t, e, "Brisa")
	a.giveCombatantDie(t, "Brisa", e)
	slotsBefore := a.vitals(t, a.bri).GetSpellSlots()

	held, err := a.cast(t, a.bia, e, "Brisa", guidingBolt, slotOfLevel(1), a.at(t, "Goblin"), d20Face(15))
	if err != nil || held.GetCast() != nil || held.GetHeldRollOffer() != nil {
		t.Fatalf("CastSpell() = %v, %v; want it held in the Mage's Counterspell window", held, err)
	}
	w := a.windowOf(t, a.master, playv1.ReactionKind_REACTION_KIND_COUNTERSPELL)
	if w == nil {
		t.Fatalf("the master's windows = %v, want the Mage's Counterspell", a.windows(t, a.master))
	}
	res := a.mustAnswer(t, a.master, e, w.GetId(), useAnswer(3))
	if c := res.GetResult().GetCounterspell(); !c.GetCountered() {
		t.Fatalf("the result = %v, want the spell countered", res.GetResult())
	}
	if offers := a.listHeldRolls(t, a.bia); len(offers) != 0 {
		t.Errorf("Brisa's questions after the counter = %v, want none", offers)
	}
	if n := a.countOf(t, `SELECT count(*) FROM inspiration_holds`); n != 0 {
		t.Errorf("inspiration_holds rows = %d, want 0", n)
	}
	if n := a.countOf(t, `SELECT count(*) FROM session_events WHERE kind = $1 AND NOT COALESCE((payload->'reaction'->>'hold')::bool, false) AND (payload->'reaction'->>'countered')::bool`, eventSpellCast); n != 1 {
		t.Errorf("countered %s events = %d, want 1", eventSpellCast, n)
	}
	if !byLabel(t, a.get(t, a.bia), "Brisa").GetActionUsed() || sameSlots(a.vitals(t, a.bri).GetSpellSlots(), slotsBefore) {
		t.Error("the countered cast's action or slot is not spent")
	}
	if n := a.countOf(t, `SELECT count(*) FROM pending_damages`); n != 0 {
		t.Errorf("pending damages = %d, want 0", n)
	}
	if n := a.countOf(t, `SELECT count(*) FROM combatants WHERE id = $1 AND inspiration_sides IS NOT NULL`, a.id(t, "Brisa")); n != 1 {
		t.Error("the countered cast used Brisa's die")
	}
}

// stableVitals is Pensantus's vitals as JSON, without what moves by itself (the revision and the time), to see a refused change left them alone.
func (a *armed) stableVitals(t *testing.T) string {
	t.Helper()
	v := a.vitals(t, a.pens)
	v.Revision, v.UpdatedAt = 0, nil
	return asJSON(t, v)
}

// TestASpellAttackAnswerSettlesOnTheBonusItWasHeldWith: no window holds Brisa's spell attack; her d20 waits for the die's question with her
// bonus as it was when it was rolled. A roll bonus that lands on her before she answers (a Bênção, say) does not count: the answer settles the
// roll she was asked about, as the answer of a weapon attack does (rollHeldAttack).
func TestASpellAttackAnswerSettlesOnTheBonusItWasHeldWith(t *testing.T) {
	t.Parallel()
	f := newFavorFight(t)
	f.e = f.passTo(t, f.e, "Brisa")
	f.giveCombatantDie(t, "Brisa", f.e)
	held, err := f.cast(t, f.bia, f.e, "Brisa", guidingBolt, slotOfLevel(1), f.at(t, "Goblin"), d20Face(4))
	offer := held.GetHeldRollOffer()
	if err != nil || offer.GetDie() == nil {
		t.Fatalf("CastSpell() = %v, %v; want the die's question", held, err)
	}
	heldTotal := offer.GetD20().GetTotal()
	f.hiddenEffect(t, "Brisa", true, `[{"kind":"roll_bonus","value":2,"applies_to":["attack"]}]`)
	out, err := f.answerHeldRoll(t, f.bia, offer.GetHoldId(), dieAnswer(false))
	if err != nil {
		t.Fatalf("AnswerHeldRoll() error = %v", err)
	}
	targets := out.GetSpellCast().GetCast().GetTargets()
	if len(targets) != 1 || targets[0].GetAttackRoll().GetTotal() != heldTotal {
		t.Errorf("the answered roll = %v, want the total it was held with, %d", targets, heldTotal)
	}
}

// TestLuckyThenTheDieOnASpellAttackAWindowHeld: a halfling that holds a die rolls a 1 in the replay of a cast a window held. Lucky asks first,
// and the die's question follows on the same hold: one row, the cast written once, under the last answer, with the slot and the action
// spent once and the die gone.
func TestLuckyThenTheDieOnASpellAttackAWindowHeld(t *testing.T) {
	t.Parallel()
	a, c := spellAfterAWindow(t, "race:halfling")
	a.giveCombatantDie(t, "Pensantus", c.e)
	key := a.castInTheWindow(t, c, 1)
	offer := a.wantTheQuestionOfTheReplay(t, c, key)
	if offer.GetLucky() == nil {
		t.Fatalf("the first question = %v, want Lucky's", offer)
	}
	lucky, err := a.answerHeldRollKey(t, a.ana, offer.GetHoldId(), newKey(), luckyAnswer(15))
	if err != nil || lucky.GetNext().GetDie() == nil || lucky.GetSpellCast().GetCast() != nil {
		t.Fatalf("the Lucky answer = %v, %v; want the die's question next and no cast", lucky, err)
	}
	if n := a.countOf(t, `SELECT count(*) FROM session_events WHERE kind = $1 AND NOT COALESCE((payload->'reaction'->>'hold')::bool, false)`, eventSpellCast); n != 0 {
		t.Fatalf("%s events after Lucky's answer = %d, want 0", eventSpellCast, n)
	}
	c.h.roller.queue(4)
	answerKey := newKey()
	out, err := a.answerHeldRollKey(t, a.ana, offer.GetHoldId(), answerKey, dieAnswer(true))
	if err != nil {
		t.Fatalf("the die's answer error = %v", err)
	}
	targets := out.GetSpellCast().GetCast().GetTargets()
	if len(targets) != 1 || targets[0].GetAttackRoll().GetTotal() != 24 || len(targets[0].GetBonusDice()) != 1 {
		t.Fatalf("the cast = %v, want the 1 rolled again to 15, 15 + 5 + the die 4 = 24", out.GetSpellCast().GetCast())
	}
	a.wantTheCastOnce(t, c)
	if n := a.countOf(t, `SELECT count(*) FROM inspiration_holds WHERE idempotency_key = $1`, key); n != 1 {
		t.Errorf("the holds under the cast's key = %d, want 1", n)
	}
	if n := a.countOf(t, `SELECT count(*) FROM combatants WHERE id = $1 AND inspiration_sides IS NOT NULL`, c.id(t, "Pensantus")); n != 0 {
		t.Error("Pensantus still holds the die she used")
	}
	if n := a.countOf(t, `SELECT count(*) FROM session_events WHERE kind = $1 AND payload->>'d20_reroll' IS NOT NULL AND NOT COALESCE((payload->'reaction'->>'hold')::bool, false)`, eventSpellCast); n > 1 {
		t.Errorf("cast events with the reroll = %d, want at most 1", n)
	}
}
