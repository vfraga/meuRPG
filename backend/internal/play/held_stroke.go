package play

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// Stroke of Luck (SRD 5.1, Rogue 20): "If your attack misses a target within range, you can turn the miss into a hit.
// Alternatively, if you fail an ability check, you can treat the d20 roll as a 20. Once you use this feature, you
// can't use it again until you finish a short or long rest." It is a link of the chain (held_questions.go) in the
// place of Indomitable's, which it never meets in one roll (a save is neither an attack nor an ability check): it
// comes after the result is known, so after Lucky's reroll and the Bardic Inspiration die. The roll is held only when
// the roller has a use left; the gate works out whether the attack missed or the check failed and asks only then.
// The kinds that ask it: the weapon attack and the single-target spell attack, and the ability checks of a player's
// character that go through the gate with a rule (the scene check, the group check, the contest, Hide and the
// Counterspell check). The trap search and the puzzle hint try go through the gate but give it no rule, so they never
// ask it; initiative, the saving throws and the death save never ask it either.

const (
	// strokeResource is the resource of the feature (effects/rogue.json: one use, back on a short rest).
	strokeResource = "stroke_of_luck"
	// strokeSourceKey names the feature in the Reroll a log line or a result carries.
	strokeSourceKey = "feature:stroke-of-luck"
)

// strokeRule tells the gate how an attack is told to have missed, or a check to have failed, for the paths that ask
// Stroke of Luck.
type strokeRule struct {
	// Failed says the roll as it stands failed: the attack missed the armor class, the check is below the DC. The d20
	// that counts and the total (with the die the roller added) are given. Nil when the path has no number to
	// compare with (it is then Blind).
	Failed func(face, total int) bool
	// Prepare reads what Failed compares with (the target's armor class) once the gate knows the roller has a use left, so
	// that the rolls of everyone else read nothing. Nil when Failed needs nothing.
	Prepare func() error
	// Blind says the roller may not know the result yet, so the question comes on every roll that could use the
	// feature and says nothing: the table hides the DC of a check, the check is opposed (its result depends on the
	// other side), or dice of effects come after the gate and move the total.
	Blind bool
	// Attack says the roll is an attack: a miss becomes a hit and the d20 is not changed. A check counts the d20 as 20.
	Attack bool
}

// decide says whether the question is asked for a roll with the d20 `face` that totals `total`, and whether the
// roller may be told it failed. A d20 that is already a 20 is never asked about: the feature changes nothing on it, and
// the roller sees the die anyway.
func (r strokeRule) decide(face, total int) (ask, told bool) {
	if face >= d20Sides {
		return false, false
	}
	failed := r.Failed != nil && r.Failed(face, total)
	return r.Blind || failed, failed && !r.Blind
}

// strokeUse is the use of Stroke of Luck on a roll: the die that counted and what it counted as. It is what the event
// keeps (a use was spent, which the undo gives back) and the log line tells.
type strokeUse struct {
	Index int32 `json:"index"`
	From  int32 `json:"from"`
	// To is 20 for a check; for an attack the d20 is not changed, so it is From, and the attack's path turns the miss
	// into a hit.
	To int32 `json:"to"`
}

// attack says the use turned a miss into a hit: the d20 it counted as is the d20 it was.
func (u *strokeUse) attack() bool { return u != nil && u.To == u.From }

// proto is the use as a result lists it; none for a roll the player kept.
func (u *strokeUse) proto() []*playv1.Reroll {
	if u == nil {
		return nil
	}
	return []*playv1.Reroll{{Index: u.Index, From: u.From, To: u.To, SourceKey: strokeSourceKey}}
}

// strokeOf is the use a hold's answer made; nil before the answer and when the player kept the roll.
func strokeOf(h heldRoll, spec rollSpec) *strokeUse {
	if h.StrokeFrom == nil {
		return nil
	}
	u := &strokeUse{Index: clamp32(h.Counted, 0, 1), From: *h.StrokeFrom, To: d20Sides}
	if spec.Stroke != nil && spec.Stroke.Attack {
		u.To = u.From
	}
	return u
}

// strokeNote is the sentence a log line written as text adds for the roll Stroke of Luck changed
// (" Golpe de Sorte: o erro virou acerto."), "" when it did not.
func strokeNote(u *strokeUse) string {
	switch {
	case u == nil:
		return ""
	case u.attack():
		return " Golpe de Sorte: o erro virou acerto."
	}
	return fmt.Sprintf(" Golpe de Sorte: %d → %d.", u.From, u.To)
}

// strokeRerolls lists the rerolls of a roll that is not a save: Lucky's, and Stroke of Luck's for a check (an
// attack's d20 does not change, and its line says the miss became a hit).
func strokeRerolls(lucky *luckyReroll, stroke *strokeUse) []*playv1.Reroll {
	return append(lucky.proto(), stroke.proto()...)
}

// dieKept is the Bardic Inspiration die the row keeps while a question after the die's waits (JSON), nil for none.
func dieKept(d *bonusUse) ([]byte, error) {
	if d == nil {
		return nil, nil
	}
	raw, err := json.Marshal(dieUsedJSON{Sides: d.Sides, Face: d.Face, From: d.From, ExpiresRound: d.ExpiresRound})
	if err != nil {
		return nil, fmt.Errorf("keep the die: %w", err)
	}
	return raw, nil
}

func (st inspirationHoldStore) askStroke(ctx context.Context, h heldRoll) error {
	raw, err := dieKept(h.DieUsed)
	if err != nil {
		return err
	}
	if err := st.q.AskInspirationHoldStroke(ctx, playdb.AskInspirationHoldStrokeParams{
		ID: h.ID, StrokeFailed: h.StrokeFailed, DieAnswerKey: h.DieAnswerKey, DieUsed: raw,
	}); err != nil {
		return fmt.Errorf("ask Stroke of Luck: %w", err)
	}
	return nil
}

func (st inspirationHoldStore) answerStroke(ctx context.Context, h heldRoll) error {
	if err := st.q.AnswerInspirationHoldStroke(ctx, playdb.AnswerInspirationHoldStrokeParams{
		ID: h.ID, Faces: faces32(h.Faces), Counted: clamp32(h.Counted, 0, 1), Total: clamp32(h.Total, -heldBonusLimit, heldBonusLimit),
		StrokeAnswerKey: h.StrokeAnswerKey, StrokeFrom: h.StrokeFrom,
	}); err != nil {
		return fmt.Errorf("answer Stroke of Luck: %w", err)
	}
	return nil
}

func (hs heldStores) askStroke(ctx context.Context, h heldRoll) error { return hs.st.askStroke(ctx, h) }

func (hs heldStores) answerStroke(ctx context.Context, h heldRoll) error {
	return hs.st.answerStroke(ctx, h)
}

// strokeQuestion is the rogue's "Errou o ataque. Usar Golpe de Sorte para acertar?" or "Falhou no teste. Usar Golpe
// de Sorte para contar o d20 como 20?".
type strokeQuestion struct{}

func (strokeQuestion) kind() questionKind { return askStroke }

// pending: the question was asked and neither it nor the roll was answered.
func (strokeQuestion) pending(h heldRoll, _ rollerState) bool { return strokeAsks(h) }

func (strokeQuestion) offer(h heldRoll, st rollerState) heldOffer {
	return heldOffer{Hold: h, State: st, Asks: askStroke}
}

// strokeAsks says the roll waits for the Stroke of Luck answer.
func strokeAsks(h heldRoll) bool {
	return h.StrokeAsk && h.StrokeAnswerKey == nil && h.AnswerKey == nil
}

// apply writes the answer. A player who uses the feature spends one use through the vitals; a check's d20 that counts
// becomes a 20 in the row (the total is 20 plus the roll's bonus), an attack's stays and its path turns the miss into a
// hit. The roll is never asked about again. A player who keeps the roll spends nothing. The same answer again (a
// retry) finds the roll as it left it.
func (q strokeQuestion) apply(ctx context.Context, ae answering, h heldRoll, a heldReply) (heldRoll, error) {
	if h.StrokeAnswerKey != nil && *h.StrokeAnswerKey == a.key {
		return h, nil
	}
	if !q.pending(h, ae.st) {
		return h, connect.NewError(connect.CodeFailedPrecondition, errors.New("the Stroke of Luck question was answered already"))
	}
	after := h
	after.StrokeAnswerKey = &a.key
	if a.stroke {
		if err := ae.s.spendUse(ctx, ae.gateEnv, ae.spec.Roller, strokeResource); err != nil {
			return h, err
		}
		counted := min(max(h.Counted, 0), len(h.Faces)-1)
		after.StrokeFrom = new(clamp32(h.Faces[counted], 1, d20Sides))
		if ae.spec.Stroke == nil || !ae.spec.Stroke.Attack {
			after.Faces = slices.Clone(h.Faces)
			after.Faces[counted] = d20Sides
			after.Total = d20Sides + ae.spec.Modifier
		}
	}
	if err := newHeldStores(ae.q).answerStroke(ctx, after); err != nil {
		return h, fmt.Errorf("answer the Stroke of Luck question: %w", err)
	}
	return after, nil
}

// spendUse spends one use of a resource of the roller's character through the vitals: inside the combat's change
// when the gate runs in one, else in the transaction of the path that rolls. Either way the read and the write do not
// interleave with another spend: every path that reaches the gate locked the open session first
// (GetOpenGameSessionForUpdate), so two changes of a table take turns, as for every other spend of a use.
func (s *Service) spendUse(ctx context.Context, env gateEnv, r heldRoller, key string) error {
	if r.CharacterID == "" {
		return connect.NewError(connect.CodeInternal, errors.New("a use outside a character"))
	}
	if env.c != nil {
		_, err := s.spendResource(ctx, env.c, r.CharacterID, key, 1)
		return err
	}
	now, err := s.vitals.GetVitalsTx(ctx, env.tx, r.CampaignID, r.CharacterID)
	if err != nil {
		return fmt.Errorf("read the vitals: %w", err)
	}
	left := resourceOf(now, key)
	if left == nil || left.GetTotal()-left.GetUsed() <= 0 {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NO_USES, "no uses left",
			func(b *playv1.EncounterBlocked) { b.Recharge = left.GetRecharge() })
	}
	_, _, err = s.vitals.AdjustVitals(ctx, env.tx, r.CampaignID, r.CharacterID, &playv1.AdjustCharacterVitalsRequest{
		ResourcesUsed: []*playv1.ResourceUsed{{Key: key, Used: left.GetUsed() + 1}},
	})
	if err != nil {
		return fmt.Errorf("spend the use: %w", err)
	}
	return nil
}

// strokeLeft says the roller has the feature with a use left: only a roll of a player's character that may ask and
// has a rule is looked at, so the vitals of the others are never read.
func (s *Service) strokeLeft(ctx context.Context, env gateEnv, spec rollSpec) (bool, error) {
	if spec.Stroke == nil || !spec.MayAsk || spec.Roller.CharacterID == "" {
		return false, nil
	}
	v, err := s.vitals.GetVitalsTx(ctx, env.tx, spec.Roller.CampaignID, spec.Roller.CharacterID)
	if err != nil {
		return false, fmt.Errorf("read the vitals: %w", err)
	}
	r := resourceOf(v, strokeResource)
	return r != nil && r.GetTotal()-r.GetUsed() > 0, nil
}

// asksStrokeAtOnce says the roll is held for Stroke of Luck as soon as it is rolled: the roller has a use, nothing
// else waits for an answer first (no 1 for Lucky, no die) and the attack missed or the check failed (or the roller
// may not know).
func (ae answering) asksStrokeAtOnce(roll d20Roll, luckyAt *int32) (ask, told bool) {
	if luckyAt != nil || ae.st.Holds || !ae.st.Stroke {
		return false, false
	}
	return ae.spec.Stroke.decide(roll.Face(), roll.Total)
}

// gateStrokeNext is what the gate does when the Lucky and die questions are answered and the roll would settle: when
// the attack missed or the check failed on the roll as it stands, with the die added, the question is asked and the
// roll stays open. The die answered with it is kept on the row, since the roll is not closed. It answers false when the
// roll settles: it went well, nothing is left to use, or Stroke of Luck itself is being answered.
func (s *Service) gateStrokeNext(ctx context.Context, ae answering, h heldRoll, a heldReply, roll d20Roll, die *bonusUse) (gateResult, bool, error) {
	if a.question == askStroke || h.StrokeAnswerKey != nil {
		return gateResult{}, false, nil
	}
	if h.StrokeAsk { // the answer that asked it, again
		return gateResult{Held: new(askedBy(h, ae.st))}, true, nil
	}
	if !ae.st.Stroke {
		return gateResult{}, false, nil
	}
	total := roll.Total
	if die != nil {
		total += int(die.Face)
	}
	ask, told := ae.spec.Stroke.decide(roll.Face(), total)
	if !ask {
		return gateResult{}, false, nil
	}
	if a.die != nil {
		if err := ae.spendDie(ctx, h, a.die); err != nil {
			return gateResult{}, false, err
		}
	}
	h.StrokeAsk, h.StrokeFailed, h.DieUsed = true, told, die
	if a.question == askDie {
		h.DieAnswerKey = &a.key
	}
	stores := newHeldStores(ae.q)
	if err := stores.askStroke(ctx, h); err != nil {
		return gateResult{}, false, fmt.Errorf("ask the Stroke of Luck question: %w", err)
	}
	if err := stores.asked(ctx, ae.gateEnv, h); err != nil {
		return gateResult{}, false, err
	}
	return gateResult{Held: new(askedBy(h, ae.st))}, true, nil
}

// gateStrokeAnswer applies the Stroke of Luck answer and settles the roll.
func (s *Service) gateStrokeAnswer(ctx context.Context, ae answering, h heldRoll, a heldReply) (gateResult, error) {
	h, err := (strokeQuestion{}).apply(ctx, ae, h, a)
	if err != nil {
		return gateResult{}, err
	}
	return s.gateSettle(ctx, ae, h, a)
}

// strokeAvailable says the roll may be held for Stroke of Luck: a path that asks it, and a roller with a use left. A
// failure to read the vitals counts as no, since the gate reads them again and reports the error.
func (s *Service) strokeAvailable(ctx context.Context, env gateEnv, spec rollSpec) bool {
	left, err := s.strokeLeft(ctx, env, spec)
	return err == nil && left
}

// checkStrokeRule is the rule of an ability check against a DC the path knows: it fails when the total is below it.
// blind is set when the roller may not be told (the table hides the DC). A check with no DC cannot fail: nil, so it is
// never asked.
func checkStrokeRule(dc int, blind bool) *strokeRule {
	if dc <= 0 {
		return nil
	}
	return &strokeRule{Blind: blind, Failed: func(_, total int) bool { return total < dc }}
}

// opposedStrokeRule is the rule of a check whose result depends on the other side (a contest, Hide against the
// observers): the roller cannot know whether it failed, so the question comes on every roll and says nothing.
func opposedStrokeRule() *strokeRule { return &strokeRule{Blind: true} }

// groupStrokeRule is the rule of a member's roll in a group check: it fails against the check's DC, which the player is
// told only when the master shows it. No DC, no failure, and a saving throw never asks.
func groupStrokeRule(check playdb.GroupCheck) *strokeRule {
	if check.Dc == nil || strings.HasPrefix(check.SkillKey, "save:") {
		return nil
	}
	return checkStrokeRule(int(*check.Dc), !check.ShowDc)
}
