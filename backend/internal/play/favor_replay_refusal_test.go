package play

import (
	"slices"
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// A replay a window released that the roll's own checks refuse (#937): the player spent the Inspiração and asked an advantage with a
// reason over a disadvantage that came while the roll waited, and the replay's chooseMode refuses it after the Inspiração was
// spent. The replay runs in a savepoint: the refusal rolls it back whole, the Inspiração included. What stays spent is the window's own
// answer, written before the savepoint.

// askedAdvantage is the request of a player who spends the Inspiração and asks an advantage with a reason.
func askedAdvantage(spend bool) func(any) {
	return func(r any) {
		setRollMode(r, playv1.RollMode_ROLL_MODE_ADVANTAGE, "o inimigo está distraído")
		if q, ok := r.(*playv1.RollAttackRequest); ok {
			q.SpendFavor = spend
		}
		if q, ok := r.(*playv1.CastSpellRequest); ok {
			q.SpendFavor = spend
		}
	}
}

// refuseTheReplay holds the roll of the path with the Inspiração spent and an advantage asked, gives the roller a disadvantage meanwhile and
// returns the master's window: its answer releases the roll, and the replay is refused.
func (f *favorFight) refuseTheReplay(t *testing.T, p favorPath, edit func(any)) string {
	t.Helper()
	f.setRules(t, enemyReactionsAlways)
	p.ready(t, f)
	u, who := p.roller(f)
	f.give(t, who)
	held, err := p.send(t, f, u, newKey(), true, edit)
	if err != nil || !held.heldByWindow {
		t.Fatalf("the spend with an advantage asked on a roll a window holds = %+v, %v, want it held", held, err)
	}
	if !f.favorHolds(t) {
		t.Fatal("the Inspiração was spent while the roll waits for the window")
	}
	f.setConditions(t, f.e, p.actor, "condition:poisoned") // the disadvantage that is there when the window is answered
	w := f.windowOf(t, f.master, playv1.ReactionKind_REACTION_KIND_MASTER_CHECK)
	if w == nil {
		t.Fatal("the master has no check window")
	}
	return w.GetId()
}

func (f *favorFight) countRows(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := f.h.pool.QueryRow(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatalf("count (%s): %v", query, err)
	}
	return n
}

// wantNothingKept checks what a refused replay leaves: the Inspiração back, no event that says it was spent, no kept roll or question, the
// hold dropped and the window's answer standing.
func (f *favorFight) wantNothingKept(t *testing.T) {
	t.Helper()
	if !f.favorHolds(t) {
		t.Error("the Inspiração is gone: a refused replay must give it back")
	}
	if n := f.countRows(t, `SELECT count(*) FROM session_events WHERE game_session_id = $1 AND (kind = $2 OR payload->>'favor' = 'true')`, f.sessionID, eventFavorSpent); n != 0 {
		t.Errorf("%d event(s) say the Inspiração was spent, want none", n)
	}
	if n := f.countRows(t, `SELECT count(*) FROM inspiration_holds`); n != 0 {
		t.Errorf("%d inspiration_holds row(s) stand, want none", n)
	}
	if n := f.countRows(t, `SELECT count(*) FROM roll_holds`); n != 0 {
		t.Errorf("%d roll_holds row(s) stand, want none", n)
	}
	var states []string
	rows, err := f.h.pool.Query(t.Context(), `SELECT state FROM reaction_holds`)
	if err != nil {
		t.Fatalf("read the holds: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var st string
		if err := rows.Scan(&st); err != nil {
			t.Fatalf("read a hold: %v", err)
		}
		states = append(states, st)
	}
	if !slices.Equal(states, []string{"dropped"}) {
		t.Errorf("the holds are %v, want one, dropped", states)
	}
	if n := f.countRows(t, `SELECT count(*) FROM session_events WHERE game_session_id = $1 AND payload->'reaction'->>'kind' = $2`, f.sessionID, reactionHeldDropped); n != 1 {
		t.Errorf("%d held_dropped line(s) written, want the master's one", n)
	}
}

const (
	kCombatLog    = "*playv1.WatchGameSessionResponse_CombatLogChanged_"
	kWindowClosed = "*playv1.WatchGameSessionResponse_ReactionWindowClosed_"
)

// A weapon attack and a spell attack: the refusal of the replay gives the Inspiração back and leaves only the window's answer.
func TestAReplayRefusedAfterTheSpendGivesTheInspiracaoBack(t *testing.T) {
	t.Parallel()
	for _, p := range favorPaths {
		t.Run(p.name, func(t *testing.T) {
			t.Parallel()
			f := newFavorFight(t)
			id := f.refuseTheReplay(t, p, askedAdvantage(true))
			ears := listen(t, f.h, f.campaignID, f.master.id, true)
			roller, _ := p.roller(f)
			rollerEars := listen(t, f.h, f.campaignID, roller.id, false)
			f.h.roller.queue(3, 17)
			before := f.seq(t)
			f.mustAnswer(t, f.master, f.e, id, passAnswer)
			f.wantNothingKept(t)
			if n := f.countRows(t, `SELECT count(*) FROM reaction_windows WHERE status = 'answered'`); n != 1 {
				t.Errorf("%d answered window(s), want the master's one: the window's own answer stays", n)
			}
			if got := f.seq(t); got != before+1 {
				t.Errorf("hint_seq moved from %d to %d, want one step for the answer", before, got)
			}
			// Nobody is told about a roll that did not happen: the master hears the window closing, the combat and its log; the roller's
			// player hears nothing of the roll.
			want := []string{kCombatLog, kEncounter, kWindowClosed}
			if got := eventKinds(ears.heard(t, f.h)); !slices.Equal(got, want) {
				t.Errorf("the master heard %v, want %v", got, want)
			}
			if got := eventKinds(rollerEars.heard(t, f.h)); slices.Contains(got, kVitals) || slices.Contains(got, kCasts) {
				t.Errorf("the roller heard %v, want nothing of a roll that did not happen", got)
			}
		})
	}
}

// The reactor's answer sent again under its key, after the drop, answers as the first did and spends nothing a second time: the
// Inspiração stays where the rollback put it, the hold stays dropped, no line and no number is added.
func TestTheAnswerOfARefusedReplaySentAgainSpendsNothing(t *testing.T) {
	t.Parallel()
	for _, p := range favorPaths {
		t.Run(p.name, func(t *testing.T) {
			t.Parallel()
			f := newFavorFight(t)
			id := f.refuseTheReplay(t, p, askedAdvantage(true))
			req := &playv1.AnswerReactionRequest{CampaignId: f.campaignID, EncounterId: f.e.GetId(), WindowId: id, IdempotencyKey: newKey()}
			passAnswer(req)
			f.h.roller.queue(3, 17)
			_, err := f.master.combat.AnswerReaction(t.Context(), connect.NewRequest(req))
			if err != nil {
				t.Fatalf("AnswerReaction() error = %v", err)
			}
			f.wantNothingKept(t)
			seq, events := f.seq(t), f.eventsAll(t)
			f.h.roller.queue(5, 9)
			_, err = f.master.combat.AnswerReaction(t.Context(), connect.NewRequest(req))
			if err != nil {
				t.Fatalf("the answer sent again error = %v", err)
			}
			if f.seq(t) != seq || f.eventsAll(t) != events {
				t.Errorf("the answer sent again moved the number or wrote events: seq %d -> %d, events %v -> %v", seq, f.seq(t), events, f.eventsAll(t))
			}
			f.wantNothingKept(t)
		})
	}
}

// A table of physical dice: the faces the roller typed for the roll that the replay refuses are not kept as a roll. They stay in the
// held request (reaction_holds.request, a dropped hold) and nowhere else: no roll, no question, no event of the roll.
func TestTypedFacesOfARefusedReplayAreNotKept(t *testing.T) {
	t.Parallel()
	typed := func(r any) {
		askedAdvantage(true)(r)
		switch q := r.(type) {
		case *playv1.RollAttackRequest:
			q.Roll, q.D20Faces = nil, []int32{4, 17}
		case *playv1.CastSpellRequest:
			q.Roll, q.D20Faces = nil, []int32{4, 17}
		}
	}
	for _, p := range favorPaths {
		t.Run(p.name, func(t *testing.T) {
			t.Parallel()
			f := newFavorFight(t)
			p.ready(t, f)
			_, who := p.roller(f)
			roller, _ := p.roller(f)
			f.setPhysical(t, roller)
			f.setRules(t, enemyReactionsAlways)
			f.give(t, who)
			held, err := p.send(t, f, roller, newKey(), true, typed)
			if err != nil || !held.heldByWindow {
				t.Fatalf("the spend with typed faces on a roll a window holds = %+v, %v, want it held", held, err)
			}
			f.setConditions(t, f.e, p.actor, "condition:poisoned")
			w := f.windowOf(t, f.master, playv1.ReactionKind_REACTION_KIND_MASTER_CHECK)
			f.mustAnswer(t, f.master, f.e, w.GetId(), passAnswer)
			f.wantNothingKept(t)
			if n := f.eventCount(t, p.kind); n != 1 {
				t.Errorf("%d %s events, want the hold's marker alone: the typed faces made no roll", n, p.kind)
			}
		})
	}
}

// What the window's own answer spent stays spent when the replay it released is refused: the Guardião's Protection uses its reaction
// before the savepoint, as a reaction used against an attack that then did not happen, while the Inspiração the replay spent comes back.
func TestAProtectionAnswerStaysSpentWhenTheReplayItReleasedIsRefused(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	guard, err := a.master.characters.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
		CampaignId: a.campaignID, Kind: charactersv1.CharacterKind_CHARACTER_KIND_BOSS, Name: "Guardião", Sheet: protector().sheet(),
	}))
	if err != nil {
		t.Fatalf("CreateCharacter(Guardião) error = %v", err)
	}
	e := a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: a.goblin.GetId()}, {CharacterId: guard.Msg.GetCharacter().GetId()}},
		npcRolls: []int{3, 2},
		players:  map[string]int32{"Toren": 18, "Pensantus": 10, "Brisa": 1},
		reveal:   []string{"Goblin", "Guardião"},
		at:       map[string][2]int32{"Toren": {3, 3}, "Goblin": {4, 3}, "Guardião": {5, 4}, "Pensantus": {10, 3}, "Brisa": {4, 9}},
	})
	f := favorFightOf(t, a, e)
	f.give(t, a.toren)
	held, err := favorPaths[0].send(t, f, a.caio, newKey(), true, askedAdvantage(true))
	if err != nil || !held.heldByWindow {
		t.Fatalf("the spend with an advantage asked on an attack the Guardião may protect against = %+v, %v, want it held", held, err)
	}
	f.setConditions(t, f.e, "Toren", "condition:poisoned")
	w := a.windowOf(t, a.master, playv1.ReactionKind_REACTION_KIND_PROTECTION)
	if w == nil {
		t.Fatalf("the master has no Protection window: %v", a.windows(t, a.master))
	}
	if byLabel(t, a.get(t, a.master), "Guardião").GetReactionUsed() {
		t.Fatal("the Guardião's reaction is spent before it answered")
	}
	a.h.roller.queue(3, 17)
	a.mustAnswer(t, a.master, f.e, w.GetId(), useAnswer(0))
	f.wantNothingKept(t)
	if !byLabel(t, a.get(t, a.master), "Guardião").GetReactionUsed() {
		t.Error("the Guardião's reaction is free again: the answer's own writes must stay, the replay's go")
	}
	if n := f.countRows(t, `SELECT count(*) FROM reaction_windows WHERE status = 'answered'`); n != 1 {
		t.Errorf("%d answered window(s), want the Protection's one", n)
	}
}
