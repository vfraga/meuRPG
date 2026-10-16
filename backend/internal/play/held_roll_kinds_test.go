package play

import (
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The held roll, kind by kind, behind one small interface. The tests of the held roll (the
// halfling's Lucky, the Bardic Inspiration die) were written once for the weapon attack
// (roll_holds) and once for each of the other seven d20 rolls (inspiration_holds); this file lets
// one table walk the same story through all eight, so that a refactor of the two mechanisms
// into one changes none of it. Every roll here is made in the app, with the roller's queue as
// the witness that a retry rolls nothing: the engine queues a sentinel before each retry and
// checks it was never taken.

// pending is how many faces the scripted roller still holds.
func (r *scriptedRoller) pending() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.faces)
}

// drain forgets the faces the scripted roller still holds.
func (r *scriptedRoller) drain() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.faces = nil
}

// heldView is what a roll path says about a held roll, the same for every kind: the question it
// asks (the Lucky one comes first), or the roll once it is resolved.
type heldView struct {
	// luckyHold and dieHold are the hold of the question asked, empty when it asks no such question.
	luckyHold, dieHold string
	// resolved says the roll is written: no question is left.
	resolved bool
	// face is the d20 that counts (as the question or the roll shows it), value what it counts as
	// (Reliable Talent can raise it), modifier the bonus with the die's face in it (some kinds show the
	// modifier with the die and some without: the view puts it in), total what the roll adds up to.
	face, value, modifier, total int32
	// bonus is the face of the Bardic Inspiration die added, 0 when none was.
	bonus int32
	// rerolled is how many d20 Lucky rolled again.
	rerolled int
}

// heldCountedFace is the d20 of a DiceRoll that counts, and what it counts as.
func heldCountedFace(d *playv1.DiceRoll) (face, value int32) {
	faces := d.GetFaces()
	if len(faces) == 0 {
		return 0, 0
	}
	face = faces[min(max(int(d.GetCountedIndex()), 0), len(faces)-1)]
	value = face
	if d.GetTreatedAs() != 0 {
		value = d.GetTreatedAs()
	}
	return face, value
}

// askedView is the view of a roll that waits for an answer: the Lucky question when there is one,
// else the die's.
func askedView(lucky *playv1.LuckyOffer, dieHold string, dieD20 *playv1.DiceRoll) heldView {
	v := heldView{dieHold: dieHold}
	d20 := dieD20
	if lucky != nil {
		v.luckyHold, d20 = lucky.GetHoldId(), lucky.GetD20()
	}
	v.face, v.value = heldCountedFace(d20)
	v.modifier, v.total, v.rerolled = d20.GetModifier(), d20.GetTotal(), len(d20.GetRerolled())
	return v
}

// doneView is the view of a roll that is written.
func doneView(d20 *playv1.DiceRoll, bonus []*playv1.BonusDie) heldView {
	v := heldView{resolved: true, modifier: d20.GetModifier(), total: d20.GetTotal(), rerolled: len(d20.GetRerolled())}
	v.face, v.value = heldCountedFace(d20)
	if len(bonus) > 0 {
		v.bonus = bonus[0].GetFace()
	}
	return v
}

// attackView is the view of RollAttack's (or an answer's) response.
func attackView(r *playv1.RollAttackResponse) heldView {
	if r.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_UNSPECIFIED {
		return doneView(r.GetRoll().GetD20(), r.GetRoll().GetBonusDice())
	}
	return askedView(r.GetLuckyOffer(), r.GetInspirationOffer().GetHoldId(), r.GetInspirationOffer().GetD20())
}

// outsideView is the view of the response of a roll path that waits in inspiration_holds, as its own
// path answers it or as AnswerOutsideLucky and AnswerOutsideInspiration carry it.
func outsideView(res proto.Message, characterID string) heldView {
	switch r := res.(type) {
	case *playv1.RollSceneCheckResponse:
		if r.GetRoll() != nil {
			v := doneView(r.GetRoll().GetRoll(), r.GetRoll().GetBonusDice())
			v.modifier += v.bonus // the scene roll's modifier leaves the die out of it
			return v
		}
		return askedView(r.GetLuckyOffer(), r.GetInspirationOffer().GetHoldId(), r.GetInspirationOffer().GetD20())
	case *playv1.RollGroupCheckResponse:
		if r.GetGroupCheck() == nil {
			return askedView(r.GetLuckyOffer(), r.GetInspirationOffer().GetHoldId(), r.GetInspirationOffer().GetD20())
		}
		roll := memberOf(r.GetGroupCheck(), characterID).GetRoll()
		v := heldView{resolved: true, face: roll.GetFaces()[0], modifier: roll.GetModifier() + roll.GetBonusDieFace(), total: roll.GetTotal(), bonus: roll.GetBonusDieFace(), rerolled: len(roll.GetRerolled())}
		v.value = v.face // the member's modifier leaves the die out of it
		return v
	case *playv1.SearchForTrapsResponse:
		if r.GetRoll() != nil {
			return doneView(r.GetRoll(), r.GetBonusDice())
		}
		return askedView(r.GetLuckyOffer(), r.GetInspirationOffer().GetHoldId(), r.GetInspirationOffer().GetD20())
	case *playv1.TryPuzzleHintResponse:
		if r.GetRoll() != nil {
			return doneView(r.GetRoll(), r.GetBonusDice())
		}
		return askedView(r.GetLuckyOffer(), r.GetInspirationOffer().GetHoldId(), r.GetInspirationOffer().GetD20())
	case *playv1.CastSpellResponse:
		if r.GetCast() != nil {
			target := r.GetCast().GetTargets()[0]
			return doneView(target.GetAttackRoll(), target.GetBonusDice())
		}
		return askedView(r.GetLuckyOffer(), r.GetInspirationOffer().GetHoldId(), r.GetInspirationOffer().GetD20())
	case *playv1.RollEffectSaveResponse:
		if res := r.GetResult(); res != nil {
			return heldView{
				resolved: true, face: res.GetD20(), value: res.GetD20(), modifier: res.GetModifier() + res.GetInspirationFace(), total: res.GetTotal(),
				bonus: res.GetInspirationFace(), rerolled: len(res.GetRerolled()),
			}
		}
		return askedView(r.GetLuckyOffer(), r.GetInspirationOffer().GetHoldId(), r.GetInspirationOffer().GetD20())
	case *playv1.ResolveConcentrationSaveResponse:
		if r.GetResult() != nil {
			return doneView(r.GetResult().GetSave(), r.GetResult().GetBonusDice())
		}
		return askedView(r.GetLuckyOffer(), r.GetInspirationOffer().GetHoldId(), r.GetInspirationOffer().GetD20())
	}
	return heldView{}
}

// luckyPayload is the path's own response inside AnswerOutsideLucky's.
func luckyPayload(r *playv1.AnswerOutsideLuckyResponse) proto.Message {
	switch x := r.GetResult().(type) {
	case *playv1.AnswerOutsideLuckyResponse_SceneCheck:
		return x.SceneCheck
	case *playv1.AnswerOutsideLuckyResponse_GroupCheck:
		return x.GroupCheck
	case *playv1.AnswerOutsideLuckyResponse_TrapSearch:
		return x.TrapSearch
	case *playv1.AnswerOutsideLuckyResponse_HintTry:
		return x.HintTry
	case *playv1.AnswerOutsideLuckyResponse_SpellCast:
		return x.SpellCast
	case *playv1.AnswerOutsideLuckyResponse_EffectSave:
		return x.EffectSave
	case *playv1.AnswerOutsideLuckyResponse_ConcentrationSave:
		return x.ConcentrationSave
	}
	return nil
}

// diePayload is the path's own response inside AnswerOutsideInspiration's.
func diePayload(r *playv1.AnswerOutsideInspirationResponse) proto.Message {
	switch x := r.GetResult().(type) {
	case *playv1.AnswerOutsideInspirationResponse_SceneCheck:
		return x.SceneCheck
	case *playv1.AnswerOutsideInspirationResponse_GroupCheck:
		return x.GroupCheck
	case *playv1.AnswerOutsideInspirationResponse_TrapSearch:
		return x.TrapSearch
	case *playv1.AnswerOutsideInspirationResponse_HintTry:
		return x.HintTry
	case *playv1.AnswerOutsideInspirationResponse_SpellCast:
		return x.SpellCast
	case *playv1.AnswerOutsideInspirationResponse_EffectSave:
		return x.EffectSave
	case *playv1.AnswerOutsideInspirationResponse_ConcentrationSave:
		return x.ConcentrationSave
	}
	return nil
}

// heldRig is one kind of held roll, ready to be rolled and answered, whatever table it waits in.
// Every call is made in the app (the roller's queue gives the faces) and takes the key it runs under.
type heldRig struct {
	roller      *scriptedRoller
	roll        func(t *testing.T, key string) (heldView, error)
	answerLucky func(t *testing.T, holdID, key string, reroll bool) (heldView, error)
	answerDie   func(t *testing.T, holdID, key string, use bool) (heldView, error)
	// holds is how many held rolls the kind's table keeps for the fixture.
	holds func(t *testing.T) int
	// replayed is what the roll or the answer made again says once the roll is written, given what
	// the answer that wrote it said; nil when it says the same. Today the replay of some kinds is
	// poorer than the first answer (each one is marked FINDING in its rig below).
	replayed func(v heldView) heldView
	// refusesTheRollReplay says the first request made again after its roll is written is refused,
	// where the other kinds give the written roll back.
	refusesTheRollReplay bool
}

// countRows counts a table's rows for a campaign or an encounter.
func countRows(t *testing.T, a *harness, query, id string) int {
	t.Helper()
	var n int
	if err := a.pool.QueryRow(t.Context(), query, id).Scan(&n); err != nil {
		t.Fatalf("count the held rolls: %v", err)
	}
	return n
}

const (
	countRollHolds       = `SELECT count(*) FROM roll_holds WHERE encounter_id = $1`
	countInspirationHold = `SELECT count(*) FROM inspiration_holds WHERE campaign_id = $1`
)

// outsideRig is the rig of the seven kinds that wait in inspiration_holds: they are answered by
// AnswerOutsideLucky and AnswerOutsideInspiration, by the player whose character rolled.
func outsideRig(h *harness, campaignID, characterID string, who *user, roll func(t *testing.T, key string) (proto.Message, error)) heldRig {
	return heldRig{
		roller: h.roller,
		roll: func(t *testing.T, key string) (heldView, error) {
			t.Helper()
			res, err := roll(t, key)
			if err != nil {
				return heldView{}, err
			}
			return outsideView(res, characterID), nil
		},
		answerLucky: func(t *testing.T, holdID, key string, reroll bool) (heldView, error) {
			t.Helper()
			edit := outsideKeepTheOne
			if reroll {
				edit = outsideRerollInApp
			}
			res, err := answerOutsideLuckyKey(t, who, campaignID, holdID, key, edit)
			if err != nil {
				return heldView{}, err
			}
			return outsideView(luckyPayload(res), characterID), nil
		},
		answerDie: func(t *testing.T, holdID, key string, use bool) (heldView, error) {
			t.Helper()
			req := &playv1.AnswerOutsideInspirationRequest{CampaignId: campaignID, HoldId: holdID, Use: use, IdempotencyKey: key}
			if use {
				req.Roll = &playv1.AnswerOutsideInspirationRequest_RollInApp{RollInApp: true}
			}
			res, err := who.resource.AnswerOutsideInspiration(t.Context(), connect.NewRequest(req))
			if err != nil {
				return heldView{}, err
			}
			return outsideView(diePayload(res.Msg), characterID), nil
		},
		holds: func(t *testing.T) int { t.Helper(); return countRows(t, h, countInspirationHold, campaignID) },
	}
}

// giveDieByRow puts a d8 on a character out of a combat, as the bard's would be, for the kinds whose
// fixture has no bard to give it.
func giveDieByRow(t *testing.T, h *harness, campaignID, characterID string) {
	t.Helper()
	if _, err := h.pool.Exec(t.Context(),
		`INSERT INTO character_inspiration (character_id, campaign_id, sides, seconds_left) VALUES ($1, $2, 8, 600)`, characterID, campaignID); err != nil {
		t.Fatalf("give the die: %v", err)
	}
}

// heldKinds are the eight d20 rolls that can be held, in the order of the design (E4).
var heldKinds = []struct {
	name string
	rig  func(t *testing.T, die bool) heldRig
}{
	{"weapon attack", attackRig},
	{"spell attack", spellAttackRig},
	{"concentration save", concentrationRig},
	{"effect save", effectSaveRig},
	{"scene check", sceneCheckRig},
	{"group check", groupCheckRig},
	{"trap search", trapSearchRig},
	{"hint try", hintTryRig},
}

// attackRig is Pipo, a halfling fighter, attacking the goblin with the battleaxe; with a die it is
// Orla's d8.
func attackRig(t *testing.T, die bool) heldRig {
	t.Helper()
	a := newLuckyTable(t)
	var e *playv1.Encounter
	if die {
		e = a.luckyInspired(t)
	} else {
		e = a.luckyFight(t)
	}
	return heldRig{
		roller: a.h.roller,
		// FINDING: the request that was held, made again once it is answered, is refused with invalid_argument
		// ("idempotency_key was already used for another change"): the attack is written under the answer's
		// key, and the key of the request stands for the hold's event alone. The other kinds give the roll back.
		refusesTheRollReplay: true,
		roll: func(t *testing.T, key string) (heldView, error) {
			t.Helper()
			res, err := a.attack(t, a.caio, e, "Pipo", battleaxe, "Goblin", func(r *playv1.RollAttackRequest) { inAppRoll(r); r.IdempotencyKey = key })
			if err != nil {
				return heldView{}, err
			}
			return attackView(res), nil
		},
		answerLucky: func(t *testing.T, holdID, key string, reroll bool) (heldView, error) {
			t.Helper()
			res, err := a.answerLucky(t, a.caio, e, holdID, func(r *playv1.AnswerLuckyRequest) {
				if reroll {
					rerollInApp(r)
				} else {
					keepTheOne(r)
				}
				r.IdempotencyKey = key
			})
			if err != nil {
				return heldView{}, err
			}
			return attackView(res.GetAttack()), nil
		},
		answerDie: func(t *testing.T, holdID, key string, use bool) (heldView, error) {
			t.Helper()
			res, err := a.answer(t, a.caio, e, holdID, func(r *playv1.AnswerBardicInspirationRequest) {
				if use {
					useDieInApp(r)
				} else {
					keepTheDie(r)
				}
				r.IdempotencyKey = key
			})
			if err != nil {
				return heldView{}, err
			}
			return attackView(res.GetAttack()), nil
		},
		holds: func(t *testing.T) int { t.Helper(); return countRows(t, a.h, countRollHolds, e.GetId()) },
	}
}

// spellAttackRig is Brisa, a halfling cleric far from the goblin (no disadvantage, one d20), casting
// Guiding Bolt; with a die it is on her combatant.
func spellAttackRig(t *testing.T, die bool) heldRig {
	t.Helper()
	a := newCastersOf(t, 3, "race:halfling")
	e := a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: a.goblin.GetId()}},
		npcRolls: []int{3},
		players:  map[string]int32{"Toren": 18, "Pensantus": 10, "Brisa": 1},
		reveal:   []string{"Goblin"},
		at:       map[string][2]int32{"Toren": {3, 3}, "Goblin": {4, 3}, "Pensantus": {10, 3}, "Brisa": {4, 9}},
	})
	a.passTo(t, e, "Brisa")
	e = a.get(t, a.master)
	if die {
		a.giveCombatantDie(t, "Brisa", e)
	}
	return outsideRig(a.h, a.campaignID, a.id(t, "Brisa"), a.bia, func(t *testing.T, key string) (proto.Message, error) {
		t.Helper()
		res, err := a.castKey(t, a.bia, e, "Brisa", guidingBolt, slotOfLevel(1), a.at(t, "Goblin"),
			func(r *playv1.CastSpellRequest) { r.Roll = &playv1.CastSpellRequest_RollInApp{RollInApp: true} }, key)
		return res, err
	})
}

// concentrationRig is Pensantus, a halfling wizard, hit while he concentrates on Teia: the save is his.
func concentrationRig(t *testing.T, die bool) heldRig {
	t.Helper()
	a, e := concentratingHitOn(t, newCastersOf(t, 3, "race:halfling"), 6)
	if die {
		a.giveCombatantDie(t, "Pensantus", e)
	}
	w := a.windowOf(t, a.ana, playv1.ReactionKind_REACTION_KIND_CONCENTRATION_SAVE)
	if w == nil {
		t.Fatal("Pensantus has no concentration window")
	}
	rig := outsideRig(a.h, a.campaignID, byLabel(t, e, "Pensantus").GetCharacterId(), a.ana, func(t *testing.T, key string) (proto.Message, error) {
		t.Helper()
		res, err := a.resolveConcentration(t, a.ana, e, w.GetId(), func(r *playv1.ResolveConcentrationSaveRequest) {
			r.IdempotencyKey = key
			r.Roll = &playv1.ResolveConcentrationSaveRequest_RollInApp{RollInApp: true}
		})
		return res, err
	})
	// FINDING: once the save is written, its replay (the roll or the answer made again) answers the
	// combat alone: no result, no question.
	rig.replayed = func(heldView) heldView { return heldView{} }
	return rig
}

// effectSaveRig is Toren, a halfling, holding on to the Hold Person he is under, at the end of his turn.
func effectSaveRig(t *testing.T, die bool) heldRig {
	t.Helper()
	a := newCastersOf(t, 3, "race:halfling")
	e := a.closeFight(t)
	a.mustAddEffect(t, e, holdPerson, []string{"Toren"}, a.rounds(t, 10, "Toren"))
	a.passTo(t, e, "Toren")
	if _, err := a.endTurnRaw(t, a.master, e, true); err != nil {
		t.Fatalf("EndTurn() error = %v", err)
	}
	w := a.windowOf(t, a.caio, playv1.ReactionKind_REACTION_KIND_EFFECT_SAVE)
	if w == nil {
		t.Fatalf("Toren's windows = %v, want the save of Hold Person", a.windows(t, a.caio))
	}
	if die {
		a.giveCombatantDie(t, "Toren", e)
	}
	rig := outsideRig(a.h, a.campaignID, byLabel(t, e, "Toren").GetCharacterId(), a.caio, func(t *testing.T, key string) (proto.Message, error) {
		t.Helper()
		res, err := a.rollEffectSave(t, a.caio, e, w.GetId(), func(r *playv1.RollEffectSaveRequest) {
			r.IdempotencyKey = key
			r.Roll = &playv1.RollEffectSaveRequest_RollInApp{RollInApp: true}
		})
		return res, err
	})
	// FINDING: the replay of a written save keeps the total but not the die's face (inspiration_face).
	rig.replayed = func(v heldView) heldView { v.modifier, v.bonus = v.modifier-v.bonus, 0; return v }
	return rig
}

// sceneCheckRig is Dalila, a halfling rogue, trying Athletics (no Reliable Talent: no proficiency)
// on a wall too high to climb; with a die it is Orla's d8.
func sceneCheckRig(t *testing.T, die bool) heldRig {
	t.Helper()
	a := newResourceTable(t)
	dalila, dalilaID := joinDalila(t, a.h, a.master, a.campaignID)
	point, actions := a.h.newScene(a.mapID, "O muro do pátio", true, 3, sceneSpec{key: "skill:athletics", dc: new(int32(25))})
	a.openScene(t, point)
	if die {
		if _, err := a.giveOutside(t, a.bia, dalilaID); err != nil {
			t.Fatalf("GiveBardicInspirationOutside() error = %v", err)
		}
	}
	return outsideRig(a.h, a.campaignID, dalilaID, dalila, func(t *testing.T, key string) (proto.Message, error) {
		t.Helper()
		res, err := dalila.play.RollSceneCheck(t.Context(), connect.NewRequest(&playv1.RollSceneCheckRequest{
			CampaignId: a.campaignID, ActionId: actions[0], IdempotencyKey: key, Roll: &playv1.RollSceneCheckRequest_RollInApp{RollInApp: true},
		}))
		if err != nil {
			return nil, err
		}
		return res.Msg, nil
	})
}

// groupCheckRig is Dalila rolling her own part of the party's group check.
func groupCheckRig(t *testing.T, die bool) heldRig {
	t.Helper()
	a := newResourceTable(t)
	dalila, dalilaID := joinDalila(t, a.h, a.master, a.campaignID)
	view := a.mustRequestGroup(t, func(r *playv1.RequestGroupCheckRequest) { r.Dc = 20 })
	if die {
		if _, err := a.giveOutside(t, a.bia, dalilaID); err != nil {
			t.Fatalf("GiveBardicInspirationOutside() error = %v", err)
		}
	}
	return outsideRig(a.h, a.campaignID, dalilaID, dalila, func(t *testing.T, key string) (proto.Message, error) {
		t.Helper()
		res, err := dalila.contests.RollGroupCheck(t.Context(), connect.NewRequest(&playv1.RollGroupCheckRequest{
			CampaignId: a.campaignID, IdempotencyKey: key, GroupCheckId: view.GetId(), Roll: rollIn(),
		}))
		if err != nil {
			return nil, err
		}
		return res.Msg, nil
	})
}

// trapSearchRig is Dalila searching a cave, out of a combat, with Investigation.
func trapSearchRig(t *testing.T, die bool) heldRig {
	t.Helper()
	r := newTrapRig(t)
	r.trap(t, "Fosso Dourado", 9, 7, func(s *mapsv1.TrapSpec) { s.NoticeDc, s.FindDc = 0, 14 })
	dalila, dalilaID := joinDalila(t, r.h, r.master, r.campaignID)
	r.place(t, dalilaID, 8, 7)
	if die {
		giveDieByRow(t, r.h, r.campaignID, dalilaID)
	}
	return outsideRig(r.h, r.campaignID, dalilaID, dalila, func(t *testing.T, key string) (proto.Message, error) {
		t.Helper()
		res, err := dalila.play.SearchForTraps(t.Context(), connect.NewRequest(&playv1.SearchForTrapsRequest{
			CampaignId: r.campaignID, IdempotencyKey: key, Skill: investigation, Roll: &playv1.SearchForTrapsRequest_RollInApp{RollInApp: true},
		}))
		if err != nil {
			return nil, err
		}
		return res.Msg, nil
	})
}

// hintTryRig is Dalila trying for a hint of a riddle with Investigation.
func hintTryRig(t *testing.T, die bool) heldRig {
	t.Helper()
	p := newPuzzleTable(t)
	dalila, dalilaID := joinDalila(t, p.h, p.master, p.campaignID)
	puz := p.riddle(t, "A porta da Cripta pergunta", hintCheck("skill:investigation", 15))
	p.show(t, puz.GetId())
	if die {
		giveDieByRow(t, p.h, p.campaignID, dalilaID)
	}
	rig := outsideRig(p.h, p.campaignID, dalilaID, dalila, func(t *testing.T, key string) (proto.Message, error) {
		t.Helper()
		res, err := p.tryHintKey(t, dalila, puz.GetId(), 0, key)
		return res, err
	})
	// FINDING: the replay of a written try keeps the total but not the die (bonus_dice is empty).
	rig.replayed = func(v heldView) heldView { v.bonus = 0; return v }
	return rig
}
