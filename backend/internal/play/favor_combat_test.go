package play

import (
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// The spend of the Inspiration on the combat's rolls (favor_spend.go): a weapon attack, a spell attack, a concentration save and
// the save of an effect. The paths that share a fight share the table below; the others have their tests apart.

// favorFight is the casters' combat with Toren first, the goblin next to him and Brisa 30 ft away.
type favorFight struct {
	*favorParty
	e *playv1.Encounter
}

func newFavorFight(t *testing.T) *favorFight {
	t.Helper()
	a := newCasters(t)
	e := a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: a.goblin.GetId()}},
		npcRolls: []int{3},
		players:  map[string]int32{"Toren": 18, "Pensantus": 10, "Brisa": 1},
		reveal:   []string{"Goblin"},
		at:       map[string][2]int32{"Toren": {3, 3}, "Goblin": {4, 3}, "Pensantus": {10, 3}, "Brisa": {4, 9}},
	})
	return favorFightOf(t, a, e)
}

// favorFightOf is the Inspiration's view of a table that is already in a combat.
func favorFightOf(t *testing.T, a *armed, e *playv1.Encounter) *favorFight {
	t.Helper()
	open, err := a.h.svc.queries.GetOpenGameSession(t.Context(), a.campaignID)
	if err != nil {
		t.Fatalf("GetOpenGameSession() error = %v", err)
	}
	return &favorFight{favorParty: &favorParty{armed: a, sessionID: open.ID}, e: e}
}

func (f *favorFight) give(t *testing.T, c *charactersv1.Character) {
	t.Helper()
	if err := f.grant(c.GetId()); err != nil {
		t.Fatalf("GrantFavor(%s) error = %v", c.GetName(), err)
	}
}

// eventFavor is the favor field of the newest event of the kind: "true" when its roll was made with the Inspiration.
func (f *favorFight) eventFavor(t *testing.T, kind string) string {
	t.Helper()
	var got string
	err := f.h.pool.QueryRow(t.Context(), `SELECT COALESCE(payload->>'favor', '') FROM session_events WHERE kind = $1 AND game_session_id = $2 ORDER BY seq DESC LIMIT 1`, kind, f.sessionID).Scan(&got)
	if err != nil {
		t.Fatalf("read the newest %s event: %v", kind, err)
	}
	return got
}

// favorRolled is what a path answers of its d20.
type favorRolled struct {
	faces   []int32
	mode    playv1.RollMode
	sources []*playv1.AdvantageSource
	// holdID is the question the roll asks (Bardic Inspiration, Lucky), and heldByWindow says a reaction window holds the roll: no d20 yet.
	holdID       string
	heldByWindow bool
}

func (r favorRolled) hasFavorSource() bool {
	for _, s := range r.sources {
		if s.GetKind() == playv1.AdvantageSourceKind_ADVANTAGE_SOURCE_KIND_FAVOR && s.GetTextPt() == "Inspiração" {
			return true
		}
	}
	return false
}

// favorPath is a roll of the table: how it is sent, who rolls it and what its event is.
type favorPath struct {
	name  string
	kind  string // the event the roll writes
	actor string // the label of the roller
	// roller says who rolls it, and whose Inspiration it spends.
	roller func(f *favorFight) (*user, *charactersv1.Character)
	// ready brings the fight to the roller's turn.
	ready func(t *testing.T, f *favorFight)
	// send rolls the d20 in the app with the spend set or not, as u, under the key.
	send func(t *testing.T, f *favorFight, u *user, key string, spend bool, edit func(any)) (favorRolled, error)
	// log is the kind of the log's line of the roll.
	log playv1.CombatLogKind
	// answered reads the roll an answer to a held roll wrote.
	answered func(*playv1.AnswerHeldRollResponse) favorRolled
}

var favorPaths = []favorPath{
	{
		name: "a weapon attack", kind: eventAttackRolled, actor: "Toren", log: playv1.CombatLogKind_COMBAT_LOG_KIND_ATTACK,
		roller: func(f *favorFight) (*user, *charactersv1.Character) { return f.caio, f.toren },
		ready:  func(*testing.T, *favorFight) {},
		send: func(t *testing.T, f *favorFight, u *user, key string, spend bool, edit func(any)) (favorRolled, error) {
			t.Helper()
			res, err := f.attack(t, u, f.e, "Toren", battleaxe, "Goblin", func(r *playv1.RollAttackRequest) {
				inAppRoll(r)
				r.IdempotencyKey, r.SpendFavor = key, spend
				if edit != nil {
					edit(r)
				}
			})
			if err != nil {
				return favorRolled{}, err
			}
			return attackRolledOf(res), nil
		},
		answered: func(r *playv1.AnswerHeldRollResponse) favorRolled { return attackRolledOf(r.GetAttack()) },
	},
	{
		name: "a spell attack", kind: eventSpellCast, actor: "Brisa", log: playv1.CombatLogKind_COMBAT_LOG_KIND_SPELL_CAST,
		roller: func(f *favorFight) (*user, *charactersv1.Character) { return f.bia, f.bri },
		ready:  func(t *testing.T, f *favorFight) { t.Helper(); f.e = f.passTo(t, f.e, "Brisa") },
		send: func(t *testing.T, f *favorFight, u *user, key string, spend bool, edit func(any)) (favorRolled, error) {
			t.Helper()
			res, err := f.castKey(t, u, f.e, "Brisa", guidingBolt, slotOfLevel(1), f.at(t, "Goblin"), func(r *playv1.CastSpellRequest) {
				r.Roll = &playv1.CastSpellRequest_RollInApp{RollInApp: true}
				r.SpendFavor = spend
				if edit != nil {
					edit(r)
				}
			}, key)
			if err != nil {
				return favorRolled{}, err
			}
			return castRolledOf(res), nil
		},
		answered: func(r *playv1.AnswerHeldRollResponse) favorRolled { return castRolledOf(r.GetSpellCast()) },
	},
}

func attackRolledOf(res *playv1.RollAttackResponse) favorRolled {
	roll := res.GetRoll()
	return favorRolled{
		faces: roll.GetD20().GetFaces(), mode: roll.GetMode(), sources: roll.GetSources(),
		holdID: res.GetHeldRollOffer().GetHoldId(), heldByWindow: roll.GetHeldForReaction(),
	}
}

func castRolledOf(res *playv1.CastSpellResponse) favorRolled {
	out := favorRolled{holdID: res.GetHeldRollOffer().GetHoldId()}
	if targets := res.GetCast().GetTargets(); len(targets) > 0 {
		hit := targets[0]
		out.faces, out.mode, out.sources = hit.GetAttackRoll().GetFaces(), hit.GetAttackMode(), hit.GetAttackSources()
	}
	out.heldByWindow = res.GetHeldRollOffer() == nil && res.GetCast() == nil
	return out
}

// A spend on a roll that has no advantage gives two dice, an advantage, the source in the roll and its event, and takes the row; the
// refusals take nothing and move no number.
func TestFavorSpendOnACombatRoll(t *testing.T) {
	t.Parallel()
	for _, p := range favorPaths {
		t.Run(p.name, func(t *testing.T) {
			t.Parallel()
			f := newFavorFight(t)
			p.ready(t, f)
			u, who := p.roller(f)
			f.give(t, who)

			// Disadvantage and the Inspiration make a straight roll: one d20.
			f.setConditions(t, f.e, p.actor, "condition:poisoned")
			f.h.roller.queue(9)
			before := f.seq(t)
			got, err := p.send(t, f, u, newKey(), true, nil)
			if err != nil {
				t.Fatalf("%s with a disadvantage and the Inspiration error = %v", p.name, err)
			}
			if len(got.faces) != 1 || got.mode != playv1.RollMode_ROLL_MODE_NORMAL || !got.hasFavorSource() {
				t.Errorf("the roll = %v %v %v, want one d20, normal and the source of the Inspiration", got.faces, got.mode, got.sources)
			}
			if f.favorHolds(t) || f.eventFavor(t, p.kind) != "true" {
				t.Errorf("the Inspiration is still there = %v, the event says %q, want it spent and the event to say so", f.favorHolds(t), f.eventFavor(t, p.kind))
			}
			if after := f.seq(t); after != before+1 {
				t.Errorf("hint_seq moved from %d to %d, want one step for the roll", before, after)
			}
		})
	}
}

// The refusals of the table: each spends nothing and moves no number.
func TestFavorRefusalsOnACombatRoll(t *testing.T) {
	t.Parallel()
	type refusal struct {
		name   string
		setup  func(t *testing.T, f *favorFight, p favorPath)
		master bool
		reason playv1.FavorBlockedReason
	}
	refusals := []refusal{
		{name: "has none", setup: func(*testing.T, *favorFight, favorPath) {}, reason: playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_HAS_NONE},
		{name: "adds nothing under an advantage", setup: func(t *testing.T, f *favorFight, _ favorPath) {
			t.Helper()
			f.setConditions(t, f.e, "Goblin", "condition:restrained")
		}, reason: playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_ADDS_NOTHING},
		{name: "the master is not the holder", master: true, reason: playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_NOT_THE_HOLDER},
	}
	for _, p := range favorPaths {
		for _, r := range refusals {
			t.Run(p.name+" "+r.name, func(t *testing.T) {
				t.Parallel()
				f := newFavorFight(t)
				p.ready(t, f)
				u, who := p.roller(f)
				if r.name != "has none" {
					f.give(t, who)
				}
				if r.setup != nil {
					r.setup(t, f, p)
				}
				if r.master {
					u = f.master
				}
				f.h.roller.queue(9, 11)
				before, rows := f.seq(t), f.rows(t)
				_, err := p.send(t, f, u, newKey(), true, nil)
				wantFavorBlocked(t, p.name, err, r.reason)
				if f.rows(t) != rows || f.seq(t) != before {
					t.Errorf("the refusal left %d rows (had %d) and moved the number from %d to %d", f.rows(t), rows, before, f.seq(t))
				}
			})
		}
	}
}

// A request of a roll that is settled, sent again under its key, answers the same and spends nothing more, even with a new
// Inspiration in the character's hands; one that asks a worse mode than the spend gives is refused with the row left.
func TestFavorRetryAndWorseModeOnACombatRoll(t *testing.T) {
	t.Parallel()
	for _, p := range favorPaths {
		t.Run(p.name, func(t *testing.T) {
			t.Parallel()
			f := newFavorFight(t)
			p.ready(t, f)
			u, who := p.roller(f)
			f.give(t, who)

			_, err := p.send(t, f, u, newKey(), true, func(r any) { setRollMode(r, playv1.RollMode_ROLL_MODE_NORMAL, "sem pressa") })
			if connect.CodeOf(err) != connect.CodeInvalidArgument || !f.favorHolds(t) {
				t.Fatalf("a spend with a normal roll_mode error = %v (row there %v), want invalid_argument and the Inspiration left", err, f.favorHolds(t))
			}

			key := newKey()
			f.h.roller.queue(4, 17)
			first, err := p.send(t, f, u, key, true, nil)
			if err != nil || len(first.faces) != 2 || first.mode != playv1.RollMode_ROLL_MODE_ADVANTAGE || !first.hasFavorSource() {
				t.Fatalf("the spend = %v, %v, want two dice, an advantage and the source", first, err)
			}
			f.give(t, who) // another one, after the spend
			before := f.seq(t)
			again, err := p.send(t, f, u, key, true, nil)
			if err != nil || len(again.faces) != 2 || again.faces[0] != first.faces[0] || again.faces[1] != first.faces[1] {
				t.Errorf("the retry = %v, %v, want the same roll as the first", again, err)
			}
			if !f.favorHolds(t) || f.seq(t) != before {
				t.Errorf("the retry left the Inspiration = %v and moved the number from %d to %d, want it untouched", f.favorHolds(t), before, f.seq(t))
			}
		})
	}
}

func setRollMode(r any, mode playv1.RollMode, reason string) {
	switch req := r.(type) {
	case *playv1.RollAttackRequest:
		req.RollMode, req.ModeReason = mode, reason
	case *playv1.CastSpellRequest:
		req.RollMode, req.ModeReason = mode, reason
	}
}

// lineOf is the newest log line of the kind that u reads.
func (f *favorFight) lineOf(t *testing.T, u *user, kind playv1.CombatLogKind) *playv1.CombatLogEntry {
	t.Helper()
	var line *playv1.CombatLogEntry
	for _, l := range logEntries(f.log(t, u, f.e)) {
		if l.GetKind() == kind && (line == nil || line.GetFavorTextPt() == "" || l.GetFavorTextPt() != "") {
			line = l
		}
	}
	return line
}

// The line of a roll made with the Inspiration says so to the master and the roller's own player, as part of the roll's entry; another
// player and someone outside the campaign read the roll as any other.
func TestFavorLineOfACombatRoll(t *testing.T) {
	t.Parallel()
	for _, p := range favorPaths {
		t.Run(p.name, func(t *testing.T) {
			t.Parallel()
			f := newFavorFight(t)
			p.ready(t, f)
			u, who := p.roller(f)
			f.give(t, who)
			f.h.roller.queue(6, 13)
			if _, err := p.send(t, f, u, newKey(), true, nil); err != nil {
				t.Fatalf("%s with the Inspiration error = %v", p.name, err)
			}
			want := p.actor + " usou a Inspiração."
			for name, reader := range map[string]*user{"the master": f.master, "the roller's player": u} {
				if line := f.lineOf(t, reader, p.log); line == nil || line.GetFavorTextPt() != want {
					t.Errorf("%s reads %q, want %q", name, line.GetFavorTextPt(), want)
				}
			}
			for name, reader := range map[string]*user{"another player": f.ana, "a third player": f.caio} {
				if reader == u {
					continue
				}
				if line := f.lineOf(t, reader, p.log); line != nil && line.GetFavorTextPt() != "" {
					t.Errorf("%s reads %q, want no line of the Inspiration", name, line.GetFavorTextPt())
				}
			}
			outsider := f.h.newUser("Fora")
			_, err := outsider.combat.ListCombatLog(t.Context(), connect.NewRequest(&playv1.ListCombatLogRequest{CampaignId: f.campaignID, EncounterId: f.e.GetId()}))
			wantCode(t, "a user outside the campaign reads the combat log", err, connect.CodeNotFound)
		})
	}
}

// The master's Desfazer takes the roll back and gives the Inspiration back with it: the row returns, the event goes and the next spend
// works. The spend never closes the chain (the roll is the last action). A grant that came after the spend is not stacked.
func TestFavorUndoOfACombatRoll(t *testing.T) {
	t.Parallel()
	for _, p := range favorPaths {
		t.Run(p.name, func(t *testing.T) {
			t.Parallel()
			f := newFavorFight(t)
			p.ready(t, f)
			u, who := p.roller(f)
			f.give(t, who)
			f.h.roller.queue(6, 13)
			if _, err := p.send(t, f, u, newKey(), true, nil); err != nil {
				t.Fatalf("%s with the Inspiration error = %v", p.name, err)
			}
			id := f.log(t, f.master, f.e).GetUndoableEventId()
			if id == "" || f.favorHolds(t) {
				t.Fatalf("undoable event %q, row there %v, want the roll to be the last action and the Inspiration spent", id, f.favorHolds(t))
			}
			if err := f.undo(t, f.master, f.e, id); err != nil {
				t.Fatalf("UndoLastAction() error = %v", err)
			}
			if !f.favorHolds(t) {
				t.Error("the undo did not give the Inspiration back")
			}
			f.h.roller.queue(2, 19)
			if got, err := p.send(t, f, u, newKey(), true, nil); err != nil || !got.hasFavorSource() {
				t.Errorf("the spend after the undo = %v, %v, want it to work", got, err)
			}
			// Spend, receive another: the grant is the combat's last action, so the first undo takes the grant back, and the second takes the
			// roll and gives the spend back: one row, never two.
			f.give(t, who)
			if err := f.undo(t, f.master, f.e, f.undoableLine(t).GetId()); err != nil {
				t.Fatalf("UndoLastAction(the grant) error = %v", err)
			}
			if got := f.rows(t); got != 0 {
				t.Errorf("character_favor has %d rows after the undo of the grant, want 0", got)
			}
			if err := f.undo(t, f.master, f.e, f.log(t, f.master, f.e).GetUndoableEventId()); err != nil {
				t.Fatalf("UndoLastAction(the roll) error = %v", err)
			}
			if got := f.rows(t); got != 1 {
				t.Errorf("character_favor has %d rows after the undo of the roll, want 1", got)
			}
		})
	}
}

// holdFavorOf is the favor_spent of the held roll under the key.
func (f *favorFight) holdFavorOf(t *testing.T, id string) bool {
	t.Helper()
	var spent bool
	if err := f.h.pool.QueryRow(t.Context(), `SELECT favor_spent FROM inspiration_holds WHERE id = $1`, id).Scan(&spent); err != nil {
		t.Fatalf("read the hold %s: %v", id, err)
	}
	return spent
}

// A roll that waits for the Bardic Inspiration question keeps the Inspiration it spent in its hold: a retry under the same key finds the
// row gone and spends nothing more, and the answer writes the roll with the advantage without spending (and the event says so).
func TestFavorOnAHeldCombatRoll(t *testing.T) {
	t.Parallel()
	for _, p := range favorPaths {
		t.Run(p.name, func(t *testing.T) {
			t.Parallel()
			f := newFavorFight(t)
			p.ready(t, f)
			u, who := p.roller(f)
			f.giveCombatantDie(t, p.actor, f.e)
			f.give(t, who)

			key := newKey()
			f.h.roller.queue(4, 15)
			held, err := p.send(t, f, u, key, true, nil)
			if err != nil || held.holdID == "" {
				t.Fatalf("the spend on a roll that holds a die = %v, %v, want the die's question", held, err)
			}
			if f.favorHolds(t) || !f.holdFavorOf(t, held.holdID) {
				t.Errorf("the Inspiration is there = %v, the hold remembers it = %v, want it spent and kept in the hold", f.favorHolds(t), f.holdFavorOf(t, held.holdID))
			}

			f.give(t, who) // another one: the retry and the answer must leave it
			before := f.seq(t)
			again, err := p.send(t, f, u, key, true, nil)
			if err != nil || again.holdID != held.holdID || !f.favorHolds(t) || f.seq(t) != before {
				t.Errorf("the retry = %+v, %v (Inspiration there %v, number %d -> %d), want the same question and nothing spent", again, err, f.favorHolds(t), before, f.seq(t))
			}

			out, err := f.answerHeldRoll(t, u, held.holdID, dieAnswer(false))
			if err != nil {
				t.Fatalf("AnswerHeldRoll() error = %v", err)
			}
			got := p.answered(out)
			if len(got.faces) != 2 || got.mode != playv1.RollMode_ROLL_MODE_ADVANTAGE || !got.hasFavorSource() {
				t.Errorf("the answered roll = %+v, want the two dice kept with the advantage and the source of the Inspiration", got)
			}
			if !f.favorHolds(t) || f.eventFavor(t, p.kind) != "true" {
				t.Errorf("after the answer the Inspiration is there = %v and the event says %q, want the new one untouched and the event to say it was spent", f.favorHolds(t), f.eventFavor(t, p.kind))
			}
			if retry, err := p.send(t, f, u, key, true, nil); err != nil || len(retry.faces) != 2 || !f.favorHolds(t) {
				t.Errorf("the retry after the answer = %+v, %v (Inspiration there %v), want the written roll and nothing spent", retry, err, f.favorHolds(t))
			}
		})
	}
}

// A reaction window holds the attack or the cast before the d20: nothing is spent while it waits (the hold only checks the Inspiration is
// there), the replay spends it, and a replay that finds it gone drops the action with the refusal and spends nothing.
func TestFavorOnAReplayOfAHeldCombatRoll(t *testing.T) {
	t.Parallel()
	for _, p := range favorPaths {
		t.Run(p.name, func(t *testing.T) {
			t.Parallel()
			f := newFavorFight(t)
			f.setRules(t, enemyReactionsAlways)
			p.ready(t, f)
			u, who := p.roller(f)

			// With none, the hold itself is refused: the player does not wait for the windows to learn it.
			_, err := p.send(t, f, u, newKey(), true, nil)
			wantFavorBlocked(t, "the spend with none on a roll a window holds", err, playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_HAS_NONE)
			if w := f.windowOf(t, f.master, playv1.ReactionKind_REACTION_KIND_MASTER_CHECK); w != nil {
				t.Fatalf("a refused spend left the window %v", w)
			}

			f.give(t, who)
			held, err := p.send(t, f, u, newKey(), true, nil)
			if err != nil || !held.heldByWindow {
				t.Fatalf("the spend on a roll a window holds = %+v, %v, want it held", held, err)
			}
			if !f.favorHolds(t) {
				t.Fatal("the Inspiration was spent while the roll waits for the window")
			}
			w := f.windowOf(t, f.master, playv1.ReactionKind_REACTION_KIND_MASTER_CHECK)
			f.h.roller.queue(3, 17)
			f.mustAnswer(t, f.master, f.e, w.GetId(), passAnswer)
			if f.favorHolds(t) || f.eventFavor(t, p.kind) != "true" {
				t.Errorf("after the replay the Inspiration is there = %v and the event says %q, want it spent by the replay", f.favorHolds(t), f.eventFavor(t, p.kind))
			}
		})
	}
}

// B3: an Inspiração that is gone when the window's replay comes does not drop the roll: it is made without it (a normal roll from the
// real sources, nothing spent, the event says it was not made with it), and the master and the roller's own player read why in the line
// of the roll.
func TestFavorGoneAtTheReplayRollsWithoutIt(t *testing.T) {
	t.Parallel()
	for _, p := range favorPaths {
		t.Run(p.name, func(t *testing.T) {
			t.Parallel()
			f := newFavorFight(t)
			f.setRules(t, enemyReactionsAlways)
			p.ready(t, f)
			u, who := p.roller(f)
			f.give(t, who)
			held, err := p.send(t, f, u, newKey(), true, nil)
			if err != nil || !held.heldByWindow {
				t.Fatalf("the spend on a roll a window holds = %+v, %v, want it held", held, err)
			}
			if _, err := f.h.pool.Exec(t.Context(), `DELETE FROM character_favor WHERE character_id = $1`, who.GetId()); err != nil { // given away meanwhile
				t.Fatalf("take the Inspiration away: %v", err)
			}
			w := f.windowOf(t, f.master, playv1.ReactionKind_REACTION_KIND_MASTER_CHECK)
			f.h.roller.queue(12)
			f.mustAnswer(t, f.master, f.e, w.GetId(), passAnswer)
			f.wantRolledWithoutIt(t, p, u, p.actor+" não tinha mais a Inspiração: a rolagem foi feita sem ela.", 1)
			if f.favorHolds(t) {
				t.Error("the replay gave an Inspiration out of nothing")
			}
		})
	}
}

// wantRolledWithoutIt checks the replay of a held roll wrote the roll with nothing spent: the hold is released, one event of the roll that
// does not say it was made with the Inspiração, the line the master and the roller's player read, and the dice the mode makes.
func (f *favorFight) wantRolledWithoutIt(t *testing.T, p favorPath, roller *user, line string, dice int) {
	t.Helper()
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
	if err := rows.Err(); err != nil {
		t.Fatalf("read the holds: %v", err)
	}
	if len(states) != 1 || states[0] != "released" {
		t.Errorf("the holds are %v, want the one released and none dropped", states)
	}
	if f.eventCount(t, p.kind) != 2 || f.eventFavor(t, p.kind) == "true" { // the hold's event, and the roll the replay wrote
		t.Errorf("the roll's events are %d and the roll says %q, want the hold's and the roll's, made without the Inspiração", f.eventCount(t, p.kind), f.eventFavor(t, p.kind))
	}
	for name, reader := range map[string]*user{"the master": f.master, "the roller's player": roller} {
		entry := f.lineOf(t, reader, p.log)
		if entry == nil || entry.GetFavorTextPt() != line {
			t.Errorf("%s reads %q, want %q", name, entry.GetFavorTextPt(), line)
			continue
		}
		if got := len(diceOfEntry(entry)); got != dice {
			t.Errorf("%s reads %d d20 in the roll, want %d", name, got, dice)
		}
	}
	for name, reader := range map[string]*user{"another player": f.ana, "a third player": f.caio} {
		if reader == roller {
			continue
		}
		if entry := f.lineOf(t, reader, p.log); entry != nil && entry.GetFavorTextPt() != "" {
			t.Errorf("%s reads %q, want nothing of the Inspiração", name, entry.GetFavorTextPt())
		}
	}
}

// diceOfEntry are the d20 a log line of an attack or of a spell attack shows.
func diceOfEntry(e *playv1.CombatLogEntry) []int32 {
	if r := e.GetAttackRoll(); r != nil {
		return r.GetFaces()
	}
	for _, tg := range e.GetSpell().GetTargets() {
		if r := tg.GetAttackRoll(); r != nil {
			return r.GetFaces()
		}
	}
	return nil
}

// With physical dice the typed faces are as many as the mode the Inspiration makes: one face under a spend that makes advantage is
// refused, the pair is written.
func TestFavorTypedFacesFollowTheSpend(t *testing.T) {
	t.Parallel()
	f := newFavorFight(t)
	f.setPhysical(t, f.caio)
	f.give(t, f.toren)
	typed := func(faces ...int32) func(*playv1.RollAttackRequest) {
		return func(r *playv1.RollAttackRequest) { r.D20Faces, r.SpendFavor = faces, true }
	}
	if _, err := f.attack(t, f.caio, f.e, "Toren", battleaxe, "Goblin", typed(10)); connect.CodeOf(err) != connect.CodeInvalidArgument || !f.favorHolds(t) {
		t.Fatalf("one face under a spend error = %v (Inspiration there %v), want invalid_argument and the Inspiration left", err, f.favorHolds(t))
	}
	res, err := f.attack(t, f.caio, f.e, "Toren", battleaxe, "Goblin", typed(10, 15))
	if err != nil || len(res.GetRoll().GetD20().GetFaces()) != 2 || res.GetRoll().GetMode() != playv1.RollMode_ROLL_MODE_ADVANTAGE || f.favorHolds(t) {
		t.Errorf("the pair under a spend = %v, %v (Inspiration there %v), want the advantage written and the Inspiration spent", res.GetRoll(), err, f.favorHolds(t))
	}
}

// favorSavePath is a saving throw of the combat the owner rolls in a window: a concentration save and the save of an effect.
type favorSavePath struct {
	name  string
	kind  string // the event the save writes
	actor string // the label of the roller
	log   playv1.CombatLogKind
	setup func(t *testing.T) (*favorFight, *playv1.ReactionWindow, *user, *charactersv1.Character)
	send  func(t *testing.T, f *favorFight, w *playv1.ReactionWindow, u *user, key string, spend bool) (favorRolled, error)
	// answered reads the save an answer to a held roll wrote.
	answered func(*playv1.AnswerHeldRollResponse) favorRolled
}

var favorSavePaths = []favorSavePath{
	{
		name: "a concentration save", kind: eventConcentrationSaveRolled, actor: "Pensantus", log: playv1.CombatLogKind_COMBAT_LOG_KIND_REACTION_WINDOW,
		setup: func(t *testing.T) (*favorFight, *playv1.ReactionWindow, *user, *charactersv1.Character) {
			t.Helper()
			a, e := concentratingHit(t, 6)
			w := a.windowOf(t, a.ana, playv1.ReactionKind_REACTION_KIND_CONCENTRATION_SAVE)
			if w == nil {
				t.Fatal("Pensantus has no concentration window")
			}
			return favorFightOf(t, a, e), w, a.ana, a.pens
		},
		send: func(t *testing.T, f *favorFight, w *playv1.ReactionWindow, u *user, key string, spend bool) (favorRolled, error) {
			t.Helper()
			res, err := f.resolveConcentration(t, u, f.e, w.GetId(), func(r *playv1.ResolveConcentrationSaveRequest) {
				r.Roll = &playv1.ResolveConcentrationSaveRequest_RollInApp{RollInApp: true}
				r.IdempotencyKey, r.SpendFavor = key, spend
			})
			if err != nil {
				return favorRolled{}, err
			}
			return concentrationRolledOf(res.GetResult(), res.GetHeldRollOffer()), nil
		},
		answered: func(r *playv1.AnswerHeldRollResponse) favorRolled {
			return concentrationRolledOf(r.GetConcentrationSave().GetResult(), nil)
		},
	},
	{
		name: "the save of an effect", kind: eventLastingSaved, actor: "Toren", log: playv1.CombatLogKind_COMBAT_LOG_KIND_EFFECT,
		setup: func(t *testing.T) (*favorFight, *playv1.ReactionWindow, *user, *charactersv1.Character) {
			t.Helper()
			a, e, w := indomitableTable(t, "")
			return favorFightOf(t, a, e), w, a.caio, a.toren
		},
		send: func(t *testing.T, f *favorFight, w *playv1.ReactionWindow, u *user, key string, spend bool) (favorRolled, error) {
			t.Helper()
			res, err := f.rollEffectSave(t, u, f.e, w.GetId(), func(r *playv1.RollEffectSaveRequest) {
				r.Roll = &playv1.RollEffectSaveRequest_RollInApp{RollInApp: true}
				r.IdempotencyKey, r.SpendFavor = key, spend
			})
			if err != nil {
				return favorRolled{}, err
			}
			return effectSaveRolledOf(res.GetResult(), res.GetHeldRollOffer()), nil
		},
		answered: func(r *playv1.AnswerHeldRollResponse) favorRolled {
			return effectSaveRolledOf(r.GetEffectSave().GetResult(), nil)
		},
	},
}

func concentrationRolledOf(r *playv1.ConcentrationSaveResult, offer *playv1.HeldRollOffer) favorRolled {
	return favorRolled{faces: r.GetSave().GetFaces(), holdID: offer.GetHoldId()}
}

func effectSaveRolledOf(r *playv1.EffectSaveResult, offer *playv1.HeldRollOffer) favorRolled {
	return favorRolled{faces: r.GetD20Faces(), holdID: offer.GetHoldId()}
}

// The Inspiration on a save: the spend gives the pair of dice, the event and the line; the refusals take nothing; a settled retry and a
// held save spend it once.
func TestFavorOnACombatSave(t *testing.T) {
	t.Parallel()
	for _, p := range favorSavePaths {
		t.Run(p.name, func(t *testing.T) {
			t.Parallel()
			t.Run("spend", func(t *testing.T) {
				t.Parallel()
				f, w, u, who := p.setup(t)
				f.give(t, who)
				f.h.roller.queue(4, 20)
				before := f.seq(t)
				got, err := p.send(t, f, w, u, newKey(), true)
				if err != nil || len(got.faces) != 2 {
					t.Fatalf("the spend = %+v, %v, want the pair of dice", got, err)
				}
				if f.favorHolds(t) || f.eventFavor(t, p.kind) != "true" || f.seq(t) != before+1 {
					t.Errorf("the Inspiration is there = %v, the event says %q, the number went %d -> %d, want it spent, said by the event and one step", f.favorHolds(t), f.eventFavor(t, p.kind), before, f.seq(t))
				}
				want := p.actor + " usou a Inspiração."
				for name, reader := range map[string]*user{"the master": f.master, "the roller's player": u} {
					if line := f.lineOf(t, reader, p.log); line == nil || line.GetFavorTextPt() != want {
						t.Errorf("%s reads %q, want %q", name, line.GetFavorTextPt(), want)
					}
				}
				for name, reader := range map[string]*user{"another player": f.bia, "a third player": f.ana} {
					if reader == u {
						continue
					}
					if line := f.lineOf(t, reader, p.log); line != nil && line.GetFavorTextPt() != "" {
						t.Errorf("%s reads %q, want no line of the Inspiration", name, line.GetFavorTextPt())
					}
				}
			})
			t.Run("refusals", func(t *testing.T) {
				t.Parallel()
				f, w, u, who := p.setup(t)
				f.h.roller.queue(4, 20, 9, 12)
				before := f.seq(t)
				_, err := p.send(t, f, w, u, newKey(), true)
				wantFavorBlocked(t, p.name+" with none", err, playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_HAS_NONE)
				f.give(t, who)
				_, err = p.send(t, f, w, f.master, newKey(), true)
				wantFavorBlocked(t, p.name+" rolled by the master", err, playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_NOT_THE_HOLDER)
				if !f.favorHolds(t) || f.seq(t) != before+1 { // the grant is the one step
					t.Errorf("the refusals left the Inspiration there = %v and the number at %d (from %d), want it there and one step for the grant", f.favorHolds(t), f.seq(t), before)
				}
			})
			t.Run("retry", func(t *testing.T) {
				t.Parallel()
				f, w, u, who := p.setup(t)
				f.give(t, who)
				key := newKey()
				f.h.roller.queue(4, 20)
				first, err := p.send(t, f, w, u, key, true)
				if err != nil || f.favorHolds(t) {
					t.Fatalf("the spend error = %v (Inspiration there %v), want it spent", err, f.favorHolds(t))
				}
				f.give(t, who)
				before := f.seq(t)
				again, err := p.send(t, f, w, u, key, true)
				if err != nil || len(first.faces) != 2 || len(again.faces) > 0 && again.faces[0] != first.faces[0] || !f.favorHolds(t) || f.seq(t) != before {
					t.Errorf("the retry = %+v, %v (Inspiration there %v, number %d -> %d), want a settled save and nothing spent", again, err, f.favorHolds(t), before, f.seq(t))
				}
			})
			t.Run("held", func(t *testing.T) {
				t.Parallel()
				f, w, u, who := p.setup(t)
				f.giveCombatantDie(t, p.actor, f.e)
				f.give(t, who)
				key := newKey()
				f.h.roller.queue(4, 20)
				held, err := p.send(t, f, w, u, key, true)
				if err != nil || held.holdID == "" || f.favorHolds(t) || !f.holdFavorOf(t, held.holdID) {
					t.Fatalf("the spend on a save that holds a die = %+v, %v (Inspiration there %v), want the question, with the Inspiration kept in the hold", held, err, f.favorHolds(t))
				}
				f.give(t, who)
				if again, err := p.send(t, f, w, u, key, true); err != nil || again.holdID != held.holdID || !f.favorHolds(t) {
					t.Errorf("the retry = %+v, %v (Inspiration there %v), want the same question and nothing spent", again, err, f.favorHolds(t))
				}
				out, err := f.answerHeldRoll(t, u, held.holdID, dieAnswer(false))
				if err != nil {
					t.Fatalf("AnswerHeldRoll() error = %v", err)
				}
				if got := p.answered(out); len(got.faces) != 2 || !f.favorHolds(t) || f.eventFavor(t, p.kind) != "true" {
					t.Errorf("the answer wrote %+v (Inspiration there %v, event says %q), want the pair kept, the new Inspiration untouched and the event to say it was spent", got, f.favorHolds(t), f.eventFavor(t, p.kind))
				}
			})
		})
	}
}

// A save that fails by itself rolls no d20: the spend is refused, as the owner's own and as another's.
func TestFavorOnASaveThatFailsByItselfHasNothingToRoll(t *testing.T) {
	t.Parallel()
	reactor := playdb.Combatant{Kind: kindPlayer, CharacterID: "c", UserID: new("owner")}
	x := &effectSaveStep{msg: &playv1.RollEffectSaveRequest{SpendFavor: true}, reactor: reactor}
	var s Service
	x.call.m = authz.Membership{Role: authz.RolePlayer, UserID: "owner"}
	wantFavorBlocked(t, "the owner's spend", s.refuseFavorOnAutoFail(x), playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_NOTHING_TO_ROLL)
	x.call.m = authz.Membership{Role: authz.RoleMaster, UserID: "master"}
	wantFavorBlocked(t, "the master's spend", s.refuseFavorOnAutoFail(x), playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_NOT_THE_HOLDER)
	x.msg = &playv1.RollEffectSaveRequest{}
	if err := s.refuseFavorOnAutoFail(x); err != nil {
		t.Errorf("a save that does not spend, error = %v", err)
	}
}

// A save that already has advantage gains nothing from the Inspiration: it is refused and nothing is spent.
func TestFavorOnASaveWithAdvantageAddsNothing(t *testing.T) {
	t.Parallel()
	a, e, w := indomitableTable(t, "race:gnome")
	f := favorFightOf(t, a, e)
	f.give(t, a.toren)
	before := f.seq(t)
	_, err := f.rollEffectSave(t, a.caio, e, w.GetId(), func(r *playv1.RollEffectSaveRequest) {
		r.Roll = &playv1.RollEffectSaveRequest_RollInApp{RollInApp: true}
		r.SpendFavor = true
	})
	wantFavorBlocked(t, "the spend with advantage", err, playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_ADDS_NOTHING)
	if !f.favorHolds(t) || f.seq(t) != before {
		t.Errorf("the refusal left the Inspiration there = %v, the number %d -> %d, want it there and no step", f.favorHolds(t), before, f.seq(t))
	}
}

// A spend on a roll with no source gives advantage: two dice, the source, the row gone, the event and one step.
func TestFavorSpendMakesAnAdvantage(t *testing.T) {
	t.Parallel()
	for _, p := range favorPaths {
		t.Run(p.name, func(t *testing.T) {
			t.Parallel()
			f := newFavorFight(t)
			p.ready(t, f)
			u, who := p.roller(f)
			f.give(t, who)
			f.h.roller.queue(4, 17)
			before := f.seq(t)
			got, err := p.send(t, f, u, newKey(), true, nil)
			if err != nil || len(got.faces) != 2 || got.mode != playv1.RollMode_ROLL_MODE_ADVANTAGE || !got.hasFavorSource() {
				t.Fatalf("the spend = %+v, %v, want two dice, an advantage and the source", got, err)
			}
			if f.favorHolds(t) || f.eventFavor(t, p.kind) != "true" || f.seq(t) != before+1 {
				t.Errorf("the Inspiration is there = %v, the event says %q, the number went %d -> %d, want it spent, said and one step", f.favorHolds(t), f.eventFavor(t, p.kind), before, f.seq(t))
			}
		})
	}
}

// A window that would hold the roll refuses at once a spend its replay would refuse (a worse roll_mode than the spend gives): nothing
// is held and the Inspiration stays.
func TestFavorWorseModeIsRefusedAtTheHold(t *testing.T) {
	t.Parallel()
	for _, p := range favorPaths {
		t.Run(p.name, func(t *testing.T) {
			t.Parallel()
			f := newFavorFight(t)
			f.setRules(t, enemyReactionsAlways)
			p.ready(t, f)
			u, who := p.roller(f)
			f.give(t, who)
			_, err := p.send(t, f, u, newKey(), true, func(r any) { setRollMode(r, playv1.RollMode_ROLL_MODE_NORMAL, "sem pressa") })
			wantCode(t, "a spend with a worse roll_mode at the hold", err, connect.CodeInvalidArgument)
			if !f.favorHolds(t) || f.windowOf(t, f.master, playv1.ReactionKind_REACTION_KIND_MASTER_CHECK) != nil {
				t.Errorf("the refusal left the Inspiration = %v and a window = %v, want it there and nothing held", f.favorHolds(t), f.windowOf(t, f.master, playv1.ReactionKind_REACTION_KIND_MASTER_CHECK))
			}
		})
	}
}

// B3: a replay whose sources changed, in what the roller reads, so that the Inspiração adds nothing, rolls without it: the Inspiração
// stays with the player, the roll is made from the real sources, and the master and the roller's player read why.
func TestFavorThatAddsNothingAtTheReplayRollsWithoutIt(t *testing.T) {
	t.Parallel()
	for _, p := range favorPaths {
		t.Run(p.name, func(t *testing.T) {
			t.Parallel()
			f := newFavorFight(t)
			f.setRules(t, enemyReactionsAlways)
			p.ready(t, f)
			u, who := p.roller(f)
			f.give(t, who)
			if held, err := p.send(t, f, u, newKey(), true, nil); err != nil || !held.heldByWindow {
				t.Fatalf("the spend on a roll a window holds = %+v, %v, want it held", held, err)
			}
			f.setConditions(t, f.e, "Goblin", "condition:restrained") // an advantage of its own meanwhile
			w := f.windowOf(t, f.master, playv1.ReactionKind_REACTION_KIND_MASTER_CHECK)
			f.h.roller.queue(3, 17)
			f.mustAnswer(t, f.master, f.e, w.GetId(), passAnswer)
			f.wantRolledWithoutIt(t, p, u, "A Inspiração de "+p.actor+" não mudaria mais a rolagem: ela foi guardada e a rolagem foi feita sem ela.", 2)
			if !f.favorHolds(t) {
				t.Error("the Inspiração was taken though the roll was made without it")
			}
		})
	}
}

// B3: what the master hides is not what the roller reads: an advantage the master put on the roller meanwhile (an effect the players do not
// read) does not make the spend refuse at the replay. The Inspiração is spent, as at a table where the player does not know.
func TestFavorWithAHiddenAdvantageAtTheReplayIsSpent(t *testing.T) {
	t.Parallel()
	for _, p := range favorPaths {
		t.Run(p.name, func(t *testing.T) {
			t.Parallel()
			f := newFavorFight(t)
			f.setRules(t, enemyReactionsAlways)
			p.ready(t, f)
			u, who := p.roller(f)
			f.give(t, who)
			if held, err := p.send(t, f, u, newKey(), true, nil); err != nil || !held.heldByWindow {
				t.Fatalf("the spend on a roll a window holds = %+v, %v, want it held", held, err)
			}
			f.hiddenEffect(t, p.actor, false, "["+attackAdv+"]")
			w := f.windowOf(t, f.master, playv1.ReactionKind_REACTION_KIND_MASTER_CHECK)
			f.h.roller.queue(3, 17)
			f.mustAnswer(t, f.master, f.e, w.GetId(), passAnswer)
			if f.favorHolds(t) || f.eventFavor(t, p.kind) != "true" || f.eventCount(t, p.kind) != 2 {
				t.Errorf("the Inspiração is there = %v, the roll says %q and the events are %d, want it spent by the replay and the hold's event and the roll's written", f.favorHolds(t), f.eventFavor(t, p.kind), f.eventCount(t, p.kind))
			}
		})
	}
}

// With physical dice a cast under a spend takes the pair of faces: one face is refused with the Inspiration left.
func TestFavorTypedFacesOnACast(t *testing.T) {
	t.Parallel()
	f := newFavorFight(t)
	f.e = f.passTo(t, f.e, "Brisa")
	f.setPhysical(t, f.bia)
	f.give(t, f.bri)
	typed := func(faces ...int32) func(*playv1.CastSpellRequest) {
		return func(r *playv1.CastSpellRequest) {
			r.Roll, r.D20Faces, r.SpendFavor = nil, faces, true
		}
	}
	_, err := f.castKey(t, f.bia, f.e, "Brisa", guidingBolt, slotOfLevel(1), f.at(t, "Goblin"), typed(10), newKey())
	wantCode(t, "one face under a spend", err, connect.CodeInvalidArgument)
	if !f.favorHolds(t) {
		t.Fatal("the refusal took the Inspiration")
	}
	res, err := f.castKey(t, f.bia, f.e, "Brisa", guidingBolt, slotOfLevel(1), f.at(t, "Goblin"), typed(10, 15), newKey())
	got := castRolledOf(res)
	if err != nil || len(got.faces) != 2 || got.mode != playv1.RollMode_ROLL_MODE_ADVANTAGE || f.favorHolds(t) {
		t.Errorf("the pair under a spend = %+v, %v (Inspiration there %v), want the advantage written and the Inspiration spent", got, err, f.favorHolds(t))
	}
}

// A player's character that rolls the save of a zone at the cast spends the Inspiration there: the line names the roller, not the
// zone's owner, for the master and for the roller's own player only, and the undo of the cast gives the row back.
func TestFavorOnTheSaveOfAZoneAtTheCast(t *testing.T) {
	t.Parallel()
	c := wallCave(t)
	c.wallFight(t)
	e := c.get(t, c.master)
	f := favorFightOf(t, c.armed, e)
	c.mustCastArea(t, c.ana, "Pensantus", wallOfFireKey, slotOfLevel(4), at(5, 8), nil, northWall(playv1.WallSide_WALL_SIDE_LEFT))
	w := c.windowOf(t, c.caio, playv1.ReactionKind_REACTION_KIND_EFFECT_SAVE)
	if w == nil {
		t.Fatal("Toren's player has no question: the wall caught his character at the cast")
	}
	// The row is put in the table, not granted: a grant in the combat is the last action of its chain, and the undo below is the cast's.
	f.execSQL(t, `INSERT INTO character_favor (character_id, campaign_id) SELECT $1, campaign_id FROM characters WHERE id = $1`, c.toren.GetId())
	c.h.roller.queue(4, 4, 4, 4, 4, 4, 4)
	res, err := c.rollEffectSave(t, c.caio, e, w.GetId(), func(r *playv1.RollEffectSaveRequest) {
		r.Roll, r.SpendFavor = &playv1.RollEffectSaveRequest_RollInApp{RollInApp: true}, true
	})
	if err != nil || len(res.GetResult().GetD20Faces()) != 2 || f.favorHolds(t) || f.eventFavor(t, eventLastingSaved) != "true" {
		t.Fatalf("the spend on the zone's save = %v, %v (Inspiration there %v), want the pair of dice and the row gone", res.GetResult(), err, f.favorHolds(t))
	}
	want := "Toren usou a Inspiração."
	for name, reader := range map[string]*user{"the master": c.master, "the roller's player": c.caio} {
		if line := f.lineOf(t, reader, playv1.CombatLogKind_COMBAT_LOG_KIND_EFFECT); line == nil || line.GetFavorTextPt() != want {
			t.Errorf("%s reads %q, want %q", name, line.GetFavorTextPt(), want)
		}
	}
	if line := f.lineOf(t, c.ana, playv1.CombatLogKind_COMBAT_LOG_KIND_EFFECT); line != nil && line.GetFavorTextPt() != "" {
		t.Errorf("another player reads %q, want no line of the Inspiration", line.GetFavorTextPt())
	}
	id := f.log(t, f.master, e).GetUndoableEventId()
	if id == "" {
		t.Fatal("the cast is not undoable")
	}
	if err := f.undo(t, f.master, e, id); err != nil {
		t.Fatalf("UndoLastAction() error = %v", err)
	}
	if !f.favorHolds(t) {
		t.Error("the undo of the cast did not give the Inspiration of the zone's save back")
	}
}
