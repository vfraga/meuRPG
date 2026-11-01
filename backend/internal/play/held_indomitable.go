package play

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// Indomitable (SRD 5.1, Fighter 9/13/17): "You can reroll a saving throw that you fail. If you do so, you must use the
// new roll, and you can't use this feature again until you finish a long rest." It is the last link of the chain
// (held_questions.go): it comes after the result of the save is known, so after Lucky's reroll and the Bardic
// Inspiration die. The roll is held only when the roller has a use left; the gate works out whether the save failed
// and asks only then. It covers the saving throws a player rolls himself that go through the gate: the effect's save,
// the concentration save and the death save. The saves rolled inside someone else's call are never held.

const (
	// indomitableResource is the resource of the feature (effects/fighter.json: uses per long rest).
	indomitableResource = "indomitable"
	// indomitableSourceKey names the feature in the Reroll a log line or a result carries.
	indomitableSourceKey = "feature:indomitable-1-use"
)

// saveRule tells the gate how a saving throw is told to have failed, for the paths whose d20 is a saving throw.
type saveRule struct {
	// DC is the number the total has to reach (the effect's or the concentration's).
	DC int
	// Death says the save is a death save: it fails when the total is below 10 (a natural 1 included) and a natural
	// 20 never fails.
	Death bool
	// Blind says the roller may not know the result yet, so the question comes on every save and says nothing:
	// the table hides the DC of a save, or dice of effects come after the gate and move the total.
	Blind bool
}

// decide says whether the question is asked for a save that totals `total` with the d20 `face`, and whether the
// roller may be told it failed.
func (r saveRule) decide(face, total int) (ask, told bool) {
	failed := !combat.SaveSucceeded(total, r.DC)
	if r.Death {
		out := deathOutcomeOf(face, total-face)
		failed = out == deathFailure || out == deathCritical
	}
	return r.Blind || failed, failed && !r.Blind
}

// indomitableUse is the d20 Indomitable rolled again: the die that counts in the new roll and the face that counted
// before and after. It is what the event keeps (a use was spent) and the log line tells ("Indomitável: 4 → 15").
type indomitableUse struct {
	Index int32 `json:"index"`
	From  int32 `json:"from"`
	To    int32 `json:"to"`
}

// proto is the reroll as a result lists it; none for a roll the player kept.
func (u *indomitableUse) proto() []*playv1.Reroll {
	if u == nil {
		return nil
	}
	return []*playv1.Reroll{{Index: u.Index, From: u.From, To: u.To, SourceKey: indomitableSourceKey}}
}

// indomitableOf is the reroll a hold's answer made; nil before the answer and when the player kept the roll.
func indomitableOf(h heldRoll) *indomitableUse {
	if h.IndomFrom == nil || h.IndomTo == nil {
		return nil
	}
	return &indomitableUse{Index: clamp32(h.Counted, 0, 1), From: *h.IndomFrom, To: *h.IndomTo}
}

// indomitableNote is the sentence a log line written as text adds for the save Indomitable rolled again
// (" Indomitável: 4 → 15."), "" when it did not.
func indomitableNote(u *indomitableUse) string {
	if u == nil {
		return ""
	}
	return fmt.Sprintf(" Indomitável: %d → %d.", u.From, u.To)
}

// rerollsOf lists the d20 rerolls of a save: Lucky's and Indomitable's.
func rerollsOf(lucky *luckyReroll, ind *indomitableUse) []*playv1.Reroll {
	return append(lucky.proto(), ind.proto()...)
}

// rerolls are the rerolls an event of a saving throw tells.
func (ev actionEvent) rerolls() []*playv1.Reroll { return rerollsOf(ev.Lucky, ev.Indomitable) }

// dieUsedJSON is the Bardic Inspiration die the row keeps while Indomitable's question follows the die's.
type dieUsedJSON struct {
	Sides        int32  `json:"sides"`
	Face         int32  `json:"face"`
	From         string `json:"from,omitempty"`
	ExpiresRound int32  `json:"expires_round,omitempty"`
}

// dieUsedOf reads the die a row kept; nil when it kept none.
func dieUsedOf(raw []byte) *bonusUse {
	var d dieUsedJSON
	if len(raw) == 0 || json.Unmarshal(raw, &d) != nil || d.Face == 0 {
		return nil
	}
	return &bonusUse{Sides: d.Sides, Face: d.Face, From: d.From, ExpiresRound: d.ExpiresRound}
}

func (st inspirationHoldStore) askIndomitable(ctx context.Context, h heldRoll) error {
	var raw []byte
	if d := h.DieUsed; d != nil {
		var err error
		if raw, err = json.Marshal(dieUsedJSON{Sides: d.Sides, Face: d.Face, From: d.From, ExpiresRound: d.ExpiresRound}); err != nil {
			return fmt.Errorf("keep the die: %w", err)
		}
	}
	if err := st.q.AskInspirationHoldIndomitable(ctx, playdb.AskInspirationHoldIndomitableParams{
		ID: h.ID, IndomitableFailed: h.IndomFailed, DieAnswerKey: h.DieAnswerKey, DieUsed: raw,
	}); err != nil {
		return fmt.Errorf("ask Indomitable: %w", err)
	}
	return nil
}

func (st inspirationHoldStore) answerIndomitable(ctx context.Context, h heldRoll) error {
	if err := st.q.AnswerInspirationHoldIndomitable(ctx, playdb.AnswerInspirationHoldIndomitableParams{
		ID: h.ID, Faces: faces32(h.Faces), Counted: clamp32(h.Counted, 0, 1), Total: clamp32(h.Total, -heldBonusLimit, heldBonusLimit),
		IndomitableAnswerKey: h.IndomAnswerKey, IndomitableFrom: h.IndomFrom, IndomitableTo: h.IndomTo,
	}); err != nil {
		return fmt.Errorf("answer Indomitable: %w", err)
	}
	return nil
}

// indomitableQuestion is the fighter's "Falhou na salvaguarda. Usar Indomitável para rolar de novo?".
type indomitableQuestion struct{}

func (indomitableQuestion) kind() questionKind { return askIndomitable }

// pending: the question was asked and neither it nor the roll was answered.
func (indomitableQuestion) pending(h heldRoll, _ rollerState) bool { return indomitableAsks(h) }

func (indomitableQuestion) offer(h heldRoll, st rollerState) heldOffer {
	return heldOffer{Hold: h, State: st, Asks: askIndomitable}
}

// indomitableAsks says the roll waits for the Indomitable answer.
func indomitableAsks(h heldRoll) bool {
	return h.IndomAsk && h.IndomAnswerKey == nil && h.AnswerKey == nil
}

// apply writes the answer. A player who rerolls gets the save's d20 rolled again the way the save is rolled
// (the same mode, in the app or typed), a use of the feature is spent through the vitals, and the new roll replaces
// the old in the row; it stands even when it is worse and is never asked about again. A player who keeps the roll
// spends nothing. The same answer again (a retry) finds the roll as it left it.
func (q indomitableQuestion) apply(ctx context.Context, ae answering, h heldRoll, a heldReply) (heldRoll, error) {
	if h.IndomAnswerKey != nil && *h.IndomAnswerKey == a.key {
		return h, nil
	}
	if !q.pending(h, ae.st) {
		return h, connect.NewError(connect.CodeFailedPrecondition, errors.New("the Indomitable question was answered already"))
	}
	after := h
	after.IndomAnswerKey = &a.key
	if a.indomitable != nil {
		roll, err := ae.s.d20With(*a.indomitable, ae.spec.Modifier, ae.spec.Mode)
		if err != nil {
			return h, err
		}
		roll = reliableD20(roll, ae.spec.Modifier, ae.spec.Reliable)
		if ae.c == nil {
			return h, connect.NewError(connect.CodeInternal, errors.New("the Indomitable question outside a combat"))
		}
		if _, err := ae.s.spendResource(ctx, ae.c, ae.spec.Roller.CharacterID, indomitableResource, 1); err != nil {
			return h, err
		}
		from := clamp32(h.Faces[min(h.Counted, len(h.Faces)-1)], 1, d20Sides)
		after.Faces, after.Counted, after.Total, after.Physical = roll.Faces, roll.Index, roll.Total, roll.Physical
		after.IndomFrom, after.IndomTo = new(from), new(clamp32(roll.Face(), 1, d20Sides))
	}
	if err := newHeldStores(ae.q).answerIndomitable(ctx, after); err != nil {
		return h, fmt.Errorf("answer the Indomitable question: %w", err)
	}
	return after, nil
}

// indomitableLeft says the roller has the feature with a use left: only a save of a player's character that may
// ask is looked at, so the vitals of the others are never read.
func (s *Service) indomitableLeft(ctx context.Context, env gateEnv, spec rollSpec) (bool, error) {
	if spec.Save == nil || !spec.MayAsk || spec.Roller.CharacterID == "" || env.c == nil {
		return false, nil
	}
	v, err := s.vitals.GetVitalsTx(ctx, env.tx, spec.Roller.CampaignID, spec.Roller.CharacterID)
	if err != nil {
		return false, fmt.Errorf("read the vitals: %w", err)
	}
	for _, r := range v.GetResources() {
		if r.GetKey() == indomitableResource {
			return r.GetTotal()-r.GetUsed() > 0, nil
		}
	}
	return false, nil
}

// asksAtOnce says the roll is held for Indomitable as soon as it is rolled: the roller has a use, nothing else
// waits for an answer first (no 1 for Lucky, no die) and the save failed (or the roller may not know).
func (ae answering) asksAtOnce(roll d20Roll, luckyAt *int32) (ask, told bool) {
	if luckyAt != nil || ae.st.Holds || !ae.st.Indom {
		return false, false
	}
	return ae.spec.Save.decide(roll.Face(), roll.Total)
}

// gateIndomitableNext is what the gate does when the Lucky and die questions are answered and the roll would settle:
// when the save failed on the roll as it stands, with the die added, the question is asked and the roll stays open.
// The die answered with it is kept on the row, since the roll is not closed. It answers false when the roll
// settles: the save passed, nothing is left to use, or Indomitable itself is being answered.
func (s *Service) gateIndomitableNext(ctx context.Context, ae answering, h heldRoll, a heldReply, roll d20Roll, die *bonusUse) (gateResult, bool, error) {
	if a.question == askIndomitable || h.IndomAnswerKey != nil {
		return gateResult{}, false, nil
	}
	if h.IndomAsk { // the answer that asked it, again
		return gateResult{Held: new(askedBy(h, ae.st))}, true, nil
	}
	if !ae.st.Indom {
		return gateResult{}, false, nil
	}
	total := roll.Total
	if die != nil {
		total += int(die.Face)
	}
	ask, told := ae.spec.Save.decide(roll.Face(), total)
	if !ask {
		return gateResult{}, false, nil
	}
	if a.die != nil {
		if err := ae.spendDie(ctx, h, a.die); err != nil {
			return gateResult{}, false, err
		}
	}
	h.IndomAsk, h.IndomFailed, h.DieUsed = true, told, die
	if a.question == askDie {
		h.DieAnswerKey = &a.key
	}
	stores := newHeldStores(ae.q)
	if err := stores.askIndomitable(ctx, h); err != nil {
		return gateResult{}, false, fmt.Errorf("ask the Indomitable question: %w", err)
	}
	if err := stores.asked(ctx, ae.gateEnv, h); err != nil {
		return gateResult{}, false, err
	}
	return gateResult{Held: new(askedBy(h, ae.st))}, true, nil
}

// gateIndomitableAnswer applies the Indomitable answer and settles the roll with the new d20.
func (s *Service) gateIndomitableAnswer(ctx context.Context, ae answering, h heldRoll, a heldReply) (gateResult, error) {
	h, err := (indomitableQuestion{}).apply(ctx, ae, h, a)
	if err != nil {
		return gateResult{}, err
	}
	return s.gateSettle(ctx, ae, h, a)
}
