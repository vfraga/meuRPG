package play

import (
	"slices"
	"testing"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
)

// A held roll is a row of the session (a kept d20, a question waiting), so the write that keeps it or forgets it moves the session's
// number once, and HeldRollsChanged, which goes to the master and the roller only, carries that number: a page that missed the
// question has a gap to notice (#735, part 2).

const kHeldRollsChanged = "*playv1.WatchGameSessionResponse_HeldRollsChanged_"

// wantHeld checks that the master and the roller heard HeldRollsChanged, numbered with the number the write stored, and that the
// others heard nothing.
func wantHeld(t *testing.T, heard map[string][]*playv1.WatchGameSessionResponse, stored int64, roller string) {
	t.Helper()
	for _, who := range []string{"master", roller} {
		var got []int64
		for _, ev := range heard[who] {
			if ev.GetHeldRollsChanged() != nil {
				got = append(got, ev.GetSessionSeq())
			}
		}
		if len(got) != 1 || got[0] != stored {
			t.Errorf("%s heard HeldRollsChanged with numbers %v, want it once, carrying %d", who, got, stored)
		}
	}
}

// A search out of a combat whose d20 is kept for a die's question writes only the held row: the number moves once, and the master and
// the searcher hear HeldRollsChanged with it; the other players hear nothing.
func TestAHeldSearchOutsideACombatMovesTheNumberAndTellsHeldRollsChanged(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	r.trap(t, "Fosso Dourado", 7, 7, func(s *mapsv1.TrapSpec) { s.NoticeDc = 20 })
	r.place(t, r.toren.GetId(), 8, 7)
	giveDieByRow(t, r.h, r.campaignID, r.toren.GetId())
	g := trapRigHints(t, r, "Ana", "Bia")
	g.unnumbered = nil
	before := g.seq()
	held, err := r.trapSearchCall(t, r.caio, trapSearchTyped(perception, 16))
	if err != nil || held.GetHeldRollOffer().GetDie() == nil {
		t.Fatalf("SearchForTraps() = %v, %v; want the die's question", held, err)
	}
	stored := g.seq()
	if stored != before+1 {
		t.Errorf("hint_seq = %d after the held search, want %d (once)", stored, before+1)
	}
	heard := g.drain()
	wantHeld(t, heard, stored, "Caio")
	for who, evs := range heard {
		if n := len(evs); (who == "master" || who == "Caio") != (n > 0) {
			t.Errorf("%s heard %v, want it only for the master and the searcher", who, eventKinds(evs))
		}
	}
}

// The held search of a combat tells HeldRollsChanged with the number too (it was the one unnumbered event of the write).
func TestAHeldSearchInACombatTellsHeldRollsChangedWithTheNumber(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	r.trap(t, "Fosso Dourado", 7, 7, func(s *mapsv1.TrapSpec) { s.NoticeDc = 20 })
	e := r.fight(t)
	r.giveCombatantDie(t, "Toren", e)
	g := trapRigHints(t, r)
	g.unnumbered = nil
	heard := g.first(func() error {
		_, err := r.trapSearchCall(t, r.caio, trapSearchTyped(perception, 16))
		return err
	}, true)
	if !slices.Contains(heard["master"], kHeldRollsChanged) || !slices.Contains(heard["Caio"], kHeldRollsChanged) {
		t.Errorf("heard %v, want HeldRollsChanged for the master and the searcher", heard)
	}
}

// A roll that forgets a question gone stale (the switch of the reroll went off while it waited) and writes its own check moves the
// number once, and the master and the roller hear that the questions changed, with it.
func TestForgettingAStaleQuestionTellsHeldRollsChangedWithTheNumber(t *testing.T) {
	t.Parallel()
	r := d20RerollParty(t, true, d20RerollEffects("2", 2, "1"), "race:halfling")
	point, actions := r.h.newScene(r.mapID, "O muro do pátio", true, 5, sceneSpec{key: "skill:athletics", dc: new(int32(15))})
	r.openScene(t, point)
	held, err := r.rollSceneTyped(t, r.caio, actions[0], newKey(), 2)
	if err != nil || held.GetHeldRollOffer().GetReroll() == nil {
		t.Fatalf("the check on a 2 = %v, %v; want the reroll question", held, err)
	}
	r.h.svc.d20Rerolls = false
	g := newHintRig(t, r.h, r.campaignID, r.master, map[string]*user{"Caio": r.caio, "Ana": r.ana, "Bia": r.bia}, "Ana", "Bia")
	before := g.seq()
	if next, err := r.rollSceneTyped(t, r.caio, actions[0], newKey(), 2); err != nil || next.GetRoll() == nil {
		t.Fatalf("the next check = %v, %v; want it written", next, err)
	}
	if n := r.holds(t); n != 0 {
		t.Fatalf("%d rows in inspiration_holds, want the old question forgotten", n)
	}
	if got := g.seq(); got != before+1 {
		t.Errorf("hint_seq = %d, want %d (once, for the check and the forgotten row together)", got, before+1)
	}
	heard := g.drain()
	wantHeld(t, heard, before+1, "Caio")
	for who, evs := range heard {
		for _, ev := range evs {
			if ev.GetSessionSeq() != before+1 {
				t.Errorf("%s heard %T with number %d, want %d", who, ev.GetEvent(), ev.GetSessionSeq(), before+1)
			}
		}
	}
}

// Every roll that waits in inspiration_holds, out of a combat (a scene check, a group check, a search, a hint try of a puzzle), keeps its
// d20 for a die's question in a write that tells nothing else: the number moves once, and the master and the roller hear
// HeldRollsChanged with it.
func TestAHeldRollOfEveryOutsideKindMovesTheNumberAndTellsHeldRollsChanged(t *testing.T) {
	t.Parallel()
	for _, kind := range heldKinds {
		if !slices.Contains([]string{"scene check", "group check", "trap search", "hint try"}, kind.name) {
			continue // a roll of a combat moves the number with the combat's revision
		}
		t.Run(kind.name, func(t *testing.T) {
			t.Parallel()
			rig := kind.rig(t, true)
			g := newHintRig(t, rig.h, rig.campaignID, rig.master, map[string]*user{"roller": rig.player})
			before := g.seq()
			rig.roller.drain()
			if v, err := rig.roll(t, newKey()); err != nil || v.dieHold == "" {
				t.Fatalf("the roll = %+v, %v; want the die's question", v, err)
			}
			if got := g.seq(); got != before+1 {
				t.Errorf("hint_seq = %d, want %d (once)", got, before+1)
			}
			wantHeld(t, g.drain(), before+1, "roller")
		})
	}
}

// A retry of the key of a held search finds the question it kept: no row changes, so the number stays, and the master and the searcher
// hear HeldRollsChanged again with the stored number, like any keyed replay; the other players hear nothing.
func TestARetryOfAHeldSearchTellsHeldRollsChangedAgainWithTheStoredNumber(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	r.trap(t, "Fosso Dourado", 7, 7, func(s *mapsv1.TrapSpec) { s.NoticeDc = 20 })
	r.place(t, r.toren.GetId(), 8, 7)
	giveDieByRow(t, r.h, r.campaignID, r.toren.GetId())
	g := trapRigHints(t, r, "Ana", "Bia")
	g.unnumbered = nil
	key := newKey()
	call := func() error {
		_, err := r.caio.play.SearchForTraps(t.Context(), connect.NewRequest(&playv1.SearchForTrapsRequest{
			CampaignId: r.campaignID, IdempotencyKey: key, Skill: perception, Roll: &playv1.SearchForTrapsRequest_D20Face{D20Face: 16},
		}))
		return err
	}
	first := g.first(call, true)
	again := g.replay(call, first)
	for _, who := range []string{"master", "Caio"} {
		if !slices.Contains(again[who], kHeldRollsChanged) {
			t.Errorf("%s heard %v on the retry, want HeldRollsChanged again", who, again[who])
		}
	}
}

// A gate that is about to write a held roll with no hints to tell it through (a path with no session's transaction) refuses: the row
// would move no number and tell nobody.
func TestTheGateRefusesToWriteAHeldRollWithNoHints(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	giveDieByRow(t, r.h, r.campaignID, r.toren.GetId())
	s := r.h.svc
	err := db.InTx(t.Context(), s.pool, func(tx pgx.Tx) error {
		_, err := s.heldGate(t.Context(), s.outsideGate(tx, s.queries.WithTx(tx), nil), rollSpec{
			Kind: heldTrapSearch, Key: newKey(), Request: &playv1.SearchForTrapsRequest{},
			Roller: heldRoller{CampaignID: r.campaignID, CharacterID: r.toren.GetId(), UserID: r.caio.id},
			In:     rollInput{typed: 16}, MayAsk: true,
		})
		return err
	})
	if err == nil {
		t.Fatal("the gate wrote a held roll with no hints to tell it")
	}
	var n int
	if qerr := r.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM inspiration_holds WHERE campaign_id = $1`, r.campaignID).Scan(&n); qerr != nil || n != 0 {
		t.Errorf("held rolls = %d, %v; want none (the refusal rolled the write back)", n, qerr)
	}
}

// The answer to a question held in a combat moves the number once, and the hint that the questions changed carries it.
func TestTheAnswerToAHeldRollInACombatTellsHeldRollsChangedWithTheNumber(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	r.trap(t, "Fosso Dourado", 7, 7, func(s *mapsv1.TrapSpec) { s.NoticeDc = 20 })
	e := r.fight(t)
	r.giveCombatantDie(t, "Toren", e)
	held, err := r.trapSearchCall(t, r.caio, trapSearchTyped(perception, 16))
	if err != nil || held.GetHeldRollOffer().GetDie() == nil {
		t.Fatalf("SearchForTraps() = %v, %v; want the die's question", held, err)
	}
	g := trapRigHints(t, r)
	r.h.roller.queue(4)
	heard := g.first(func() error {
		_, err := r.answerHeldRoll(t, r.caio, held.GetHeldRollOffer().GetHoldId(), dieAnswer(true))
		return err
	}, true)
	if !slices.Contains(heard["master"], kHeldRollsChanged) || !slices.Contains(heard["Caio"], kHeldRollsChanged) {
		t.Errorf("heard %v, want HeldRollsChanged for the master and the roller", heard)
	}
}

// Forgetting a stale question moves a bystander's number like the same roll where no question was waiting, and tells the bystanders
// the same (nothing): the twin worlds read alike to them.
func TestForgettingAStaleQuestionReadsTheSameToTheOtherPlayers(t *testing.T) {
	t.Parallel()
	bystanders := func(stale bool) (moved int64, heard map[string][]string, master []string) {
		r := d20RerollParty(t, true, d20RerollEffects("2", 2, "1"), "race:halfling")
		point, actions := r.h.newScene(r.mapID, "O muro do pátio", true, 5, sceneSpec{key: "skill:athletics", dc: new(int32(15))})
		r.openScene(t, point)
		if stale {
			if held, err := r.rollSceneTyped(t, r.caio, actions[0], newKey(), 2); err != nil || held.GetHeldRollOffer().GetReroll() == nil {
				t.Fatalf("the check on a 2 = %v, %v; want the reroll question", held, err)
			}
		}
		r.h.svc.d20Rerolls = false
		g := newHintRig(t, r.h, r.campaignID, r.master, map[string]*user{"Caio": r.caio, "Ana": r.ana, "Bia": r.bia}, "Ana", "Bia")
		before := g.seq()
		if next, err := r.rollSceneTyped(t, r.caio, actions[0], newKey(), 2); err != nil || next.GetRoll() == nil {
			t.Fatalf("the next check = %v, %v; want it written", next, err)
		}
		moved = g.seq() - before
		kinds := kindsOf(g.drain())
		return moved, kinds, kinds["master"]
	}
	forgotMoved, forgot, forgotMaster := bystanders(true)
	freshMoved, fresh, freshMaster := bystanders(false)
	if forgotMoved != 1 || freshMoved != 1 {
		t.Errorf("the number moved by %d (a question forgotten) and %d (none), want 1 and 1", forgotMoved, freshMoved)
	}
	for _, who := range []string{"Ana", "Bia"} {
		if len(forgot[who]) != 0 || len(fresh[who]) != 0 {
			t.Errorf("%s heard %v (forgotten) and %v (none), want nothing in both", who, forgot[who], fresh[who])
		}
	}
	if !slices.Contains(forgotMaster, kHeldRollsChanged) || slices.Contains(freshMaster, kHeldRollsChanged) {
		t.Errorf("the master heard %v (forgotten) and %v (none): the control is gone", forgotMaster, freshMaster)
	}
}
