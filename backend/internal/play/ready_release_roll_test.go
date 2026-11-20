package play

import (
	"testing"
	"time"

	"connectrpc.com/connect"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// What the release of a readied action rolls, as the answer tells it (#938): the effect's dice the table types, the attack roll,
// the pending damage and the cast the answer carries, and what the player is busy with while the window is marked.

// physicalRelease readies Toren's attack at a table with physical dice, puts Bless on him, and fires: the window his player answers.
func physicalRelease(t *testing.T) (*armed, *playv1.Encounter, *playv1.ReactionWindow) {
	t.Helper()
	a := newArmed(t)
	a.forceDice(t, campaignsv1.DiceMode_DICE_MODE_PHYSICAL)
	e := a.readyThenPass(t, nil)
	a.mustAddEffect(t, e, blessKey, []string{"Toren"}, a.rounds(t, 10, "Toren"))
	w := a.mustFire(t, e, "Toren")
	return a, e, w
}

func withExtra(faces ...int32) func(*playv1.AnswerReactionRequest) {
	return func(r *playv1.AnswerReactionRequest) { r.ExtraDieFaces = faces }
}

// The die an effect adds (Bless) typed on the release reaches the attack: the roll lists it and the answer carries that roll.
func TestTheReleaseOfAReadiedAttackTakesTheDiceAnEffectAdds(t *testing.T) {
	t.Parallel()
	a, e, w := physicalRelease(t)
	res := a.mustAnswer(t, a.caio, e, w.GetId(), func(r *playv1.AnswerReactionRequest) {
		useTyped(15)(r)
		withExtra(3)(r)
	})
	extra := res.GetAttackRoll().GetD20().GetExtraDice()
	if len(extra) != 1 || extra[0].GetFace() != 3 || extra[0].GetFaces() != 4 || extra[0].GetSign() != 1 {
		t.Errorf("the extra dice of the released roll = %v, want one +d4 showing 3", extra)
	}
}

// The extra dice are refused where the answer rolls no attack, and the refusal leaves the window and the row as they were.
func TestTheDiceAnEffectAddsAreRefusedWhereTheAnswerRollsNoAttack(t *testing.T) {
	t.Parallel()
	rows := []struct {
		name  string
		setup func(t *testing.T) (*armed, *playv1.Encounter, *playv1.ReactionWindow, *user, string)
		set   func(*playv1.AnswerReactionRequest)
	}{
		{"a pass", func(t *testing.T) (*armed, *playv1.Encounter, *playv1.ReactionWindow, *user, string) {
			t.Helper()
			a, e, w := physicalRelease(t)
			return a, e, w, a.caio, "Toren"
		}, func(r *playv1.AnswerReactionRequest) {
			r.Answer = playv1.ReactionChoice_REACTION_CHOICE_PASS
			withExtra(3)(r)
		}},
		{"a readied cast", func(t *testing.T) (*armed, *playv1.Encounter, *playv1.ReactionWindow, *user, string) {
			t.Helper()
			a := newCasters(t)
			e := a.readyForRelease(t, magicMissileSpell, 1, "Capitão Goblin")
			w := a.mustFire(t, e, "Pensantus")
			return a, e, w, a.ana, "Pensantus"
		}, func(r *playv1.AnswerReactionRequest) {
			useSpellAnswer(r)
			withExtra(3)(r)
		}},
		{"another readied action", func(t *testing.T) (*armed, *playv1.Encounter, *playv1.ReactionWindow, *user, string) {
			t.Helper()
			a := newArmed(t)
			e := a.threeAndAGoblin(t)
			a.mustReady(t, a.caio, &playv1.ReadyActionRequest{
				CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, "Toren"), IdempotencyKey: newKey(),
				TriggerText: readyTrigger, Kind: playv1.ReadiedKind_READIED_KIND_OTHER, ActionKey: "standard:dodge",
			})
			e = a.mustEndTurn(t, a.master, e)
			w := a.mustFire(t, e, "Toren")
			return a, e, w, a.caio, "Toren"
		}, func(r *playv1.AnswerReactionRequest) {
			r.Answer = playv1.ReactionChoice_REACTION_CHOICE_USE
			withExtra(3)(r)
		}},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			a, e, w, by, who := row.setup(t)
			before := a.stateSnapshot(t)
			_, err := a.answerReaction(t, by, e, w.GetId(), row.set)
			if connect.CodeOf(err) != connect.CodeInvalidArgument {
				t.Fatalf("extra_die_faces on %s: error = %v, want invalid_argument", row.name, err)
			}
			if !a.readiedOpen(t, who) || a.readiedStatus(t, who) != readiedWaiting {
				t.Errorf("the window is open %v and the row %q, want open and waiting", a.readiedOpen(t, who), a.readiedStatus(t, who))
			}
			if after := a.stateSnapshot(t); after != before {
				t.Errorf("the refusal changed the state:\nbefore:\n%s\nafter:\n%s", before, after)
			}
		})
	}
}

// A window that is not a readied one refuses the dice too: an answer never drops what it carries.
func TestTheDiceAnEffectAddsAreRefusedOnAnotherKindOfWindow(t *testing.T) {
	t.Parallel()
	r, re := reactorFight(t, rogueFive)
	hit := r.mustAttack(t, r.master, re, "Goblin", sword, "Reator", d20(15))
	r.mustDamage(t, r.master, re, hit.GetPendingDamage().GetId(), typedDamage(6))
	w := r.windowOf(t, r.ana, playv1.ReactionKind_REACTION_KIND_UNCANNY_DODGE)
	if w == nil {
		t.Fatal("the rogue has no Uncanny Dodge window")
	}
	_, err := r.answerReaction(t, r.ana, re, w.GetId(), func(req *playv1.AnswerReactionRequest) {
		req.Answer = playv1.ReactionChoice_REACTION_CHOICE_USE
		withExtra(3)(req)
	})
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("extra_die_faces on an Uncanny Dodge window: error = %v, want invalid_argument", err)
	}
	if r.windowOf(t, r.ana, playv1.ReactionKind_REACTION_KIND_UNCANNY_DODGE) == nil {
		t.Error("the refusal closed the window")
	}
}

// The answer of a release carries the roll the attack made and the pending damage of a hit: the weapon attack of a readied
// attack, and the spell attack a readied cantrip holds.
func TestTheAnswerOfAReleaseCarriesTheAttackRollAndTheDamageItOwes(t *testing.T) {
	t.Parallel()
	rows := []struct {
		name  string
		ready func(t *testing.T) (*armed, *playv1.Encounter, *playv1.ReactionWindow, *user)
		by    func(face int32) func(*playv1.AnswerReactionRequest)
	}{
		{"a weapon attack", func(t *testing.T) (*armed, *playv1.Encounter, *playv1.ReactionWindow, *user) {
			t.Helper()
			a := newArmed(t)
			e := a.readyThenPass(t, nil)
			w := a.mustFire(t, e, "Toren")
			return a, e, w, a.caio
		}, useTyped},
		{"a spell attack", func(t *testing.T) (*armed, *playv1.Encounter, *playv1.ReactionWindow, *user) {
			t.Helper()
			a := newCasters(t)
			e := a.readyForRelease(t, fireBolt, 0, "Goblin")
			w := a.mustFire(t, e, "Pensantus")
			return a, e, w, a.ana
		}, func(face int32) func(*playv1.AnswerReactionRequest) {
			return useFaces(nil, face, face) // a pair: the spell attack's nearby foe asks for two d20
		}},
	}
	for _, row := range rows {
		for _, hit := range []bool{true, false} {
			face, name := int32(2), row.name+" that misses"
			if hit {
				face, name = 15, row.name+" that hits"
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				a, e, w, by := row.ready(t)
				res := a.mustAnswer(t, by, e, w.GetId(), row.by(face))
				roll := res.GetAttackRoll()
				if roll == nil || len(roll.GetD20().GetFaces()) == 0 || roll.GetD20().GetFaces()[0] != face {
					t.Fatalf("the answer's attack roll = %v, want the roll of the typed %d", roll, face)
				}
				if roll.GetAttackerId() == "" || roll.GetTargetId() == "" {
					t.Errorf("the roll names no attacker or target: %v", roll)
				}
				if (roll.GetOutcome() == playv1.AttackOutcome_ATTACK_OUTCOME_HIT) != hit {
					t.Errorf("outcome = %v with hit %v", roll.GetOutcome(), hit)
				}
				if (res.GetPendingDamage() != nil) != hit {
					t.Errorf("pending damage = %v with hit %v", res.GetPendingDamage(), hit)
				}
				if hit && res.GetPendingDamage().GetId() == "" {
					t.Error("the pending damage has no id to roll")
				}
				if res.GetCast() != nil {
					t.Errorf("a released attack carries a cast: %v", res.GetCast())
				}
			})
		}
	}
}

// A readied cast answers with the cast, and no attack roll or damage.
func TestTheAnswerOfAReadiedCastCarriesTheCast(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.readyForRelease(t, magicMissileSpell, 1, "Capitão Goblin")
	w := a.mustFire(t, e, "Pensantus")
	res := a.mustAnswer(t, a.ana, e, w.GetId(), withTargets(darts(a, t, "Capitão Goblin", 3)))
	if res.GetCast() == nil || res.GetCast().GetSpellKey() != magicMissileSpell {
		t.Errorf("the answer's cast = %v, want the Magic Missile", res.GetCast())
	}
	if res.GetAttackRoll() != nil || res.GetPendingDamage() != nil {
		t.Errorf("a released cast carries an attack roll %v or a pending damage %v", res.GetAttackRoll(), res.GetPendingDamage())
	}
}

// A pass rolls nothing: the answer carries none of it.
func TestTheAnswerOfAPassCarriesNoRoll(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.readyThenPass(t, nil)
	w := a.mustFire(t, e, "Toren")
	res := a.mustAnswer(t, a.caio, e, w.GetId(), func(r *playv1.AnswerReactionRequest) { r.Answer = playv1.ReactionChoice_REACTION_CHOICE_PASS })
	if res.GetAttackRoll() != nil || res.GetPendingDamage() != nil || res.GetCast() != nil {
		t.Errorf("a pass carries %v, %v, %v", res.GetAttackRoll(), res.GetPendingDamage(), res.GetCast())
	}
}

// A readied attack that hits off its readier's turn owes damage: the readier's player rolls it, and so does the master.
func TestTheDamageOfAReadiedHitIsRolledOffTheReadiersTurn(t *testing.T) {
	t.Parallel()
	for _, who := range []string{"the readier's player", "the master"} {
		t.Run(who, func(t *testing.T) {
			t.Parallel()
			a := newArmed(t)
			e := a.readyThenPass(t, nil)
			w := a.mustFire(t, e, "Toren")
			res := a.mustAnswer(t, a.caio, e, w.GetId(), useTyped(15))
			p := res.GetPendingDamage()
			if p.GetId() == "" {
				t.Fatalf("the hit owes no damage: %v", res)
			}
			u := a.caio
			if who == "the master" {
				u = a.master
			}
			if _, err := a.damage(t, u, e, p.GetId(), typedDamage(6)); err != nil {
				t.Fatalf("RollDamage by %s off Toren's turn: %v", who, err)
			}
		})
	}
}

func (a *armed) markBusy(t *testing.T, u *user, e *playv1.Encounter, windowID string, busy playv1.ReactionBusy) error {
	t.Helper()
	_, err := u.combat.MarkReactionRolling(t.Context(), connect.NewRequest(&playv1.MarkReactionRollingRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), WindowId: windowID, Busy: busy,
	}))
	return err
}

// What the player is busy with is kept beside the mark: a mark that names nothing is the map, a change of it updates it
// without moving the wait, and the clear takes both off. The master and the reactor's player read it; another player reads
// no window.
func TestTheMarkOfAReadiedWindowKeepsWhatThePlayerIsBusyWith(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.readyForRelease(t, magicMissileSpell, 1, "Capitão Goblin")
	w := a.mustFire(t, e, "Pensantus")
	const (
		unspecified = playv1.ReactionBusy_REACTION_BUSY_UNSPECIFIED
		onMap       = playv1.ReactionBusy_REACTION_BUSY_MAP
		onDice      = playv1.ReactionBusy_REACTION_BUSY_DICE
	)
	read := func(u *user) *playv1.ReactionWindow {
		t.Helper()
		win := a.windowOf(t, u, kindReadied)
		if win == nil {
			t.Fatalf("%s reads no READIED window", u.id)
		}
		return win
	}
	for _, u := range []*user{a.master, a.ana} {
		if got := read(u).GetBusy(); got != unspecified {
			t.Errorf("%s reads busy %v before any mark", u.id, got)
		}
	}
	if err := a.markRolling(t, a.ana, e, w.GetId()); err != nil { // an old client names nothing
		t.Fatalf("the mark: %v", err)
	}
	for _, u := range []*user{a.master, a.ana} {
		if got := read(u).GetBusy(); got != onMap {
			t.Errorf("%s reads busy %v after a mark that names nothing, want the map", u.id, got)
		}
	}
	a.h.clock.advance(5 * time.Second)
	if err := a.markBusy(t, a.ana, e, w.GetId(), onDice); err != nil {
		t.Fatalf("the mark of the dice: %v", err)
	}
	for _, u := range []*user{a.master, a.ana} {
		win := read(u)
		if win.GetBusy() != onDice || !win.GetRolling() || win.GetWaitedMs() != 5000 {
			t.Errorf("%s reads busy %v, rolling %v, waited %d ms after the switch; want the dice, rolling and 5000 (the wait counted from the first mark)", u.id, win.GetBusy(), win.GetRolling(), win.GetWaitedMs())
		}
	}
	a.h.clock.advance(2 * time.Second)
	if err := a.markBusy(t, a.ana, e, w.GetId(), onMap); err != nil {
		t.Fatalf("the mark of the map: %v", err)
	}
	if win := read(a.master); win.GetBusy() != onMap || win.GetWaitedMs() != 7000 {
		t.Errorf("the master reads busy %v, waited %d ms after the switch back; want the map and 7000", win.GetBusy(), win.GetWaitedMs())
	}
	if err := a.clearRolling(t, a.ana, e, w.GetId()); err != nil {
		t.Fatalf("the clear: %v", err)
	}
	for _, u := range []*user{a.master, a.ana} {
		if win := read(u); win.GetBusy() != unspecified || win.GetRolling() {
			t.Errorf("%s reads busy %v, rolling %v after the clear, want neither", u.id, win.GetBusy(), win.GetRolling())
		}
	}
	if got := a.windowOf(t, a.caio, kindReadied); got != nil {
		t.Errorf("another player reads the window %v", got)
	}
}
