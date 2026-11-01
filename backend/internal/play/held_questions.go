package play

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// The questions a held roll asks, in the order the SRD puts them on the table: the 1 the halfling's
// Lucky may roll again comes first, and the Bardic Inspiration die is added to whatever the d20 is after
// it (combat_lucky.go has the rule). Each question is one link of the chain: it says when it is open,
// what it asks and what its answer writes. A question that comes later (the master's reroll, Cutting
// Words on a check) is one more link with its own columns.

// questionKind names a question of the chain.
type questionKind int

const (
	askLucky questionKind = iota + 1
	askDie
	askIndomitable
	askStroke
)

// question is one link of the chain.
type question interface {
	kind() questionKind
	// pending says the roll waits for this question's answer.
	pending(h heldRoll, st rollerState) bool
	// offer is what the roll asks while this question is the first to wait.
	offer(h heldRoll, st rollerState) heldOffer
	// apply writes the answer in the columns the question owns and gives the roll as it is after it.
	apply(ctx context.Context, ae answering, h heldRoll, a heldReply) (heldRoll, error)
}

// chain is every question a held roll may ask, first to last.
var chain = []question{luckyQuestion{}, dieQuestion{}, indomitableQuestion{}, strokeQuestion{}}

// askedBy is the question a roll asks now: the first of the chain that waits. A roll that waits for
// none (the die ran out before a retry read it) still gets the die's: the path decides what to say.
func askedBy(h heldRoll, st rollerState) heldOffer {
	i := slices.IndexFunc(chain, func(q question) bool { return q.pending(h, st) })
	if i < 0 {
		i = slices.IndexFunc(chain, func(q question) bool { return q.kind() == askDie })
	}
	return chain[i].offer(h, st)
}

// luckyAsks says the roll waits for its Lucky answer: a die showed a 1 and neither the Lucky question nor
// the roll was answered.
func luckyAsks(h heldRoll) bool {
	return h.LuckyIndex != nil && h.LuckyAnswerKey == nil && h.AnswerKey == nil
}

// luckyOf is the die a hold's Lucky answer rolled again; nil before the answer, when the player kept the
// 1, and for a roll that asked no Lucky question.
func luckyOf(h heldRoll) *luckyReroll {
	if h.LuckyIndex == nil || h.LuckyTo == nil {
		return nil
	}
	return &luckyReroll{Index: *h.LuckyIndex, From: 1, To: *h.LuckyTo}
}

// luckyQuestion is the halfling's "Tirou 1: rolar de novo?".
type luckyQuestion struct{}

func (luckyQuestion) kind() questionKind { return askLucky }

func (luckyQuestion) pending(h heldRoll, _ rollerState) bool { return luckyAsks(h) }

func (luckyQuestion) offer(h heldRoll, st rollerState) heldOffer {
	return heldOffer{Hold: h, State: st, Asks: askLucky}
}

// apply replaces the face at lucky_index when the player rolled again, counts the d20 again by the roll's
// mode, re-applies Reliable Talent to the new face and writes the faces, the one that counts, the total
// and the answer. The new roll is never asked about again, even when it is another 1. The question must
// still wait: an answer that got there first refuses this one, and the same answer again (a retry)
// finds the roll as it left it.
func (q luckyQuestion) apply(ctx context.Context, ae answering, h heldRoll, a heldReply) (heldRoll, error) {
	if h.LuckyAnswerKey != nil && *h.LuckyAnswerKey == a.key {
		return h, nil
	}
	if !q.pending(h, ae.st) {
		return h, connect.NewError(connect.CodeFailedPrecondition, errors.New("the Lucky question was answered already"))
	}
	after := h.afterLucky(a, ae.spec)
	if err := newHeldStores(ae.q).answerLucky(ctx, h, after, ae.st.Holds); err != nil {
		return h, fmt.Errorf("answer the Lucky question: %w", err)
	}
	return after, nil
}

// afterLucky is the roll with the die that showed the 1 replaced by the new face when the player rolled
// again, the die that counts picked again by the mode (and Reliable Talent counting the new face), and
// the answer's key.
func (h heldRoll) afterLucky(a heldReply, spec rollSpec) heldRoll {
	faces := slices.Clone(h.Faces)
	var to *int32
	if r := a.reroll; r != nil && int(r.Index) < len(faces) {
		faces[r.Index] = int(r.To)
		to = new(r.To)
	}
	roll := d20Roll{Faces: faces, Index: countedAfter(h.Faces, h.Counted, faces, spec.Mode), Physical: h.Physical}
	roll.Total = roll.Face() + spec.Modifier
	roll = reliableD20(roll, spec.Modifier, spec.Reliable)
	h.Faces, h.Counted, h.Total, h.LuckyTo, h.LuckyAnswerKey = faces, roll.Index, roll.Total, to, &a.key
	return h
}

// countedAfter is the die that counts once Lucky changed a face: 0 for a single d20; for a pair, the pick
// of the roll's mode (the higher with advantage, the lower with disadvantage), as the path works it out
// for the answer. Should the mode no longer roll two dice (a condition ended meanwhile), the pair's own
// mode is read from the roll: the die that counted was the higher, or the lower.
func countedAfter(before []int, counted int, faces []int, mode combat.RollMode) int {
	if len(faces) != pairDice || len(before) != pairDice {
		return 0
	}
	if mode.Dice() != pairDice {
		c := int(clamp32(counted, 0, 1))
		mode = combat.ModeAdvantage
		if before[c] < before[1-c] {
			mode = combat.ModeDisadvantage
		}
	}
	return mode.Pick(faces)
}

// dieQuestion is the Bardic Inspiration die's: use it on the d20 or keep it.
type dieQuestion struct{}

func (dieQuestion) kind() questionKind { return askDie }

// pending: the roller holds a die and nothing is answered. Its answer is final.
func (dieQuestion) pending(h heldRoll, st rollerState) bool {
	return st.Holds && h.AnswerKey == nil && h.DieAnswerKey == nil
}

func (dieQuestion) offer(h heldRoll, st rollerState) heldOffer {
	return heldOffer{Hold: h, State: st, Asks: askDie}
}

// apply closes the roll: the die is used up when the player chose it, and the answer's key is kept,
// which is what a retry of the same answer finds. A Lucky answer that leaves no die to ask about closes
// the roll the same way, keeping nothing.
func (dieQuestion) apply(ctx context.Context, ae answering, h heldRoll, a heldReply) (heldRoll, error) {
	if a.die != nil {
		if err := ae.spendDie(ctx, h, a.die); err != nil {
			return h, err
		}
	}
	if err := newHeldStores(ae.q).answer(ctx, h, a.key); err != nil {
		return h, fmt.Errorf("answer the held roll: %w", err)
	}
	h.AnswerKey = &a.key
	return h, nil
}

// spendDie uses up the die the roll settled on. A roll that checks its answer (checksTheAnswer) also
// wants the face to be one of the die.
func (ae answering) spendDie(ctx context.Context, h heldRoll, die *bonusUse) error {
	if h.checksTheAnswer() && (die.Face < 1 || die.Face > ae.st.Die.Sides) {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("typed_face must be a face of the die"))
	}
	return spendDie(ctx, ae.q, ae.spec.Roller.CharacterID, ae.st.Die)
}

// heldOffer is the question a held roll asks, with the roller as it is, for the path to put in its answer.
type heldOffer struct {
	Hold  heldRoll
	State rollerState
	Asks  questionKind
}

// luckyOffer is the Lucky question: the d20 with the roll's bonus, the die that shows the 1 and, for a
// weapon attack, the attack and its target.
func (o heldOffer) luckyOffer() *playv1.LuckyOffer {
	attackKey, targetID := attackOfHold(o.Hold)
	return &playv1.LuckyOffer{
		HoldId: o.Hold.ID, D20: heldD20Of(o.Hold), LuckyIndex: derefInt32(o.Hold.LuckyIndex), AttackKey: attackKey, TargetId: targetID,
	}
}

// attackDieOffer is the question a weapon attack asks about the die its attacker holds: the d20 with the
// attack's bonus, the attack, its target and the die.
func (o heldOffer) attackDieOffer(die *playv1.InspirationDie, cs []playdb.Combatant) *playv1.InspirationOffer {
	if i := slices.IndexFunc(cs, func(c playdb.Combatant) bool { return c.ID == die.GetFromCombatantId() }); i >= 0 {
		die.FromLabel = cs[i].Label
	}
	attackKey, targetID := attackOfHold(o.Hold)
	return &playv1.InspirationOffer{HoldId: o.Hold.ID, Die: die, AttackKey: attackKey, TargetId: targetID, D20: heldD20Of(o.Hold)}
}

// outsideDieOffer is the question every other roll asks about the die: the d20 with the check's bonus and
// the die, with the name of the bard that gave it.
func (s *Service) outsideDieOffer(ctx context.Context, tx pgx.Tx, o heldOffer) *playv1.OutsideInspirationOffer {
	return &playv1.OutsideInspirationOffer{
		HoldId: o.Hold.ID, Sides: o.State.Die.Sides, FromName: s.nameOf(ctx, tx, o.Hold.CampaignID, o.State.Die.Source), D20: heldD20Of(o.Hold),
	}
}

// outsideOffers is a roll that is not a weapon attack answering with its question: the Lucky one while it
// waits (it comes first), else the die's.
func (s *Service) outsideOffers(ctx context.Context, tx pgx.Tx, o *heldOffer) (*playv1.OutsideInspirationOffer, *playv1.LuckyOffer, *playv1.HeldRollOffer) {
	held := s.outsideHeldRollOffer(ctx, tx, *o)
	if o.Asks == askLucky {
		return nil, o.luckyOffer(), held
	}
	if o.Asks == askIndomitable || o.Asks == askStroke { // only the one question, which has no older field
		return nil, nil, held
	}
	return s.outsideDieOffer(ctx, tx, *o), nil, held
}

// outsideAsk is the question a path of a player's own character answers with instead of its result.
type outsideAsk struct {
	Offer *playv1.OutsideInspirationOffer
	Lucky *playv1.LuckyOffer
	Held  *playv1.HeldRollOffer
}

// outsideAskOf is the question a roll that is not a weapon attack asks, as the paths put it in their answers.
func (s *Service) outsideAskOf(ctx context.Context, tx pgx.Tx, o *heldOffer) *outsideAsk {
	offer, lucky, held := s.outsideOffers(ctx, tx, o)
	return &outsideAsk{Offer: offer, Lucky: lucky, Held: held}
}

// attackOfHold is the attack and the target a weapon attack's question names; "" for any other roll.
func attackOfHold(h heldRoll) (attackKey, targetID string) {
	if def := heldRegistry[h.Kind]; def.offerOf != nil {
		return def.offerOf(h)
	}
	return "", ""
}

// heldD20Of is the d20 of a held roll with the roll's bonus as the modifier: both dice of a roll with
// advantage or disadvantage, the one that counts marked, and the die Lucky rolled again once the player did
// (the roll's faces are already the new ones then).
func heldD20Of(h heldRoll) *playv1.DiceRoll {
	modifier, total := clamp32(h.Modifier, math.MinInt32, math.MaxInt32), clamp32(h.Total, math.MinInt32, math.MaxInt32)
	roll := diceRoll(1, d20Sides, faces32(h.Faces), modifier, total, h.Physical)
	if len(h.Faces) == pairDice {
		roll = diceRoll(pairDice, d20Sides, faces32(h.Faces), modifier, total, h.Physical)
		roll.CountedIndex = clamp32(h.Counted, 0, 1)
		markTreated(roll, modifier, total)
	}
	roll.Rerolled = luckyOf(h).proto()
	return roll
}
