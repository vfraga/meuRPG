package play

import (
	"context"
	"errors"
	"slices"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dice"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// The answer to a question of a held roll, whichever question and whichever table the roll waits in. The
// RPC that carries it reads its request into an answerRequest; answerHeld finds the roll, checks who may
// answer, rolls the new die the answer asks for (the Lucky reroll, the Bardic Inspiration die) and runs the
// path that held the roll again with the answer in the context (heldReply): the path answers with its own
// response, still held with the next question, or resolved and written under the roll's key.

// answerRequest is an answer as the RPC read it.
type answerRequest struct {
	ref      holdRef
	key      string
	question questionKind
	// yes is "roll the 1 again" for the Lucky question and "use the die" for the die's.
	yes bool
	// in is how the new die comes, when yes: rolled in the app or typed from a real die.
	in rollInput
}

// answerState says where the roll stands when the answer comes.
type answerState int

const (
	// answerOpen: the question waits, and the answer is the first.
	answerOpen answerState = iota
	// answerRepeated: the Lucky answer again, while the die's question waits: nothing is rolled.
	answerRepeated
	// answerSettled: the roll is resolved already; the same answer again gives the result.
	answerSettled
)

// answerHeld answers the question of a held roll and gives the response of the path that held it.
func (s *Service) answerHeld(ctx context.Context, m authz.Membership, req answerRequest) (proto.Message, error) {
	h, err := s.heldToAnswer(ctx, m, req.ref)
	if err != nil {
		return nil, err
	}
	a, state, err := h.replyTo(req)
	if err != nil {
		return nil, err
	}
	if state == answerOpen {
		if err := s.rollAnswer(ctx, m, h, req, &a); err != nil {
			return nil, err
		}
	}
	return s.rerunHeld(ctx, h, a, state)
}

// heldToAnswer finds the roll the answer is for. The question of a roll that is not a weapon attack is its
// player's alone: another's is not found.
func (s *Service) heldToAnswer(ctx context.Context, m authz.Membership, ref holdRef) (heldRoll, error) {
	h, err := newHeldStores(s.queries).byID(ctx, ref)
	if errors.Is(err, pgx.ErrNoRows) {
		return heldRoll{}, errHeldNotFound()
	}
	if err != nil {
		return heldRoll{}, s.dbError(ctx, "find the held roll", err)
	}
	if h.Kind != heldWeaponAttack {
		if err := s.mayAnswerHeld(ctx, m, h); err != nil {
			return heldRoll{}, err
		}
	}
	return h, nil
}

func errHeldNotFound() error {
	return connect.NewError(connect.CodeNotFound, errors.New("held roll not found"))
}

// mayAnswerHeld checks that the caller answers for the roller, as each table has always done: the
// question of a weapon attack is answered by whoever may act for its combatant (its player or the master);
// the question of every other roll is its player's alone (the master cannot, TestTheMasterCannotAnswerAnOutsideHoldToday;
// E4-7 lets the master answer every question).
func (s *Service) mayAnswerHeld(ctx context.Context, m authz.Membership, h heldRoll) error {
	if h.Kind != heldWeaponAttack {
		if _, err := authz.RequireCampaignRole(ctx, m.CampaignID, authz.RolePlayer); err != nil {
			return err //nolint:wrapcheck // the authz refusal, as the RPC gives it
		}
		if h.UserID != m.UserID {
			return errHeldNotFound()
		}
		return nil
	}
	who, found, err := s.holdCombatant(ctx, h)
	if err != nil {
		return err
	}
	if !found {
		return errHeldNotFound()
	}
	return viewerOf(m).mayAct(who) //nolint:wrapcheck // the refusal, as rollAttack gives it
}

// holdCombatant is the combatant a weapon attack's hold belongs to, read now.
func (s *Service) holdCombatant(ctx context.Context, h heldRoll) (playdb.Combatant, bool, error) {
	cs, err := s.queries.ListCombatants(ctx, h.EncounterID)
	if err != nil {
		return playdb.Combatant{}, false, s.dbError(ctx, "list the combatants", err)
	}
	i := slices.IndexFunc(cs, func(c playdb.Combatant) bool { return c.ID == h.CombatantID })
	if i < 0 {
		return playdb.Combatant{}, false, nil
	}
	return cs[i], true, nil
}

// replyTo checks the answer against the roll as it stands and makes the heldReply of it: the question must
// be open, the Lucky one first, and an answer already given is answered again, never twice with another
// key.
func (h heldRoll) replyTo(req answerRequest) (heldReply, answerState, error) {
	a := heldReply{holdID: h.ID, key: req.key, question: req.question}
	if req.question == askLucky {
		return h.luckyReplyTo(a)
	}
	switch {
	case h.AnswerKey != nil && *h.AnswerKey != req.key:
		return a, answerOpen, errAnsweredAlready()
	case h.AnswerKey != nil:
		return a, answerSettled, nil
	case luckyAsks(h):
		return a, answerOpen, resourceBlocked(playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_LUCKY_PENDING, "answer the Lucky question of the roll first")
	}
	return a, answerOpen, nil
}

// luckyReplyTo is replyTo for the Lucky question.
func (h heldRoll) luckyReplyTo(a heldReply) (heldReply, answerState, error) {
	switch {
	case h.LuckyIndex == nil:
		return a, answerOpen, connect.NewError(connect.CodeFailedPrecondition, errors.New("the roll asks no Lucky question"))
	case h.AnswerKey != nil:
		// Resolved already: the same Lucky answer again gives the roll as it was resolved, under the key it
		// was resolved with.
		if *h.AnswerKey != a.key && deref(h.LuckyAnswerKey) != a.key {
			return a, answerOpen, errAnsweredAlready()
		}
		return heldReply{holdID: h.ID, key: *h.AnswerKey, question: askDie}, answerSettled, nil
	case h.LuckyAnswerKey != nil:
		// Answered, and held for the Bardic Inspiration question: the same answer again reads the roll as it
		// is, and rolls nothing.
		if *h.LuckyAnswerKey != a.key {
			return a, answerOpen, connect.NewError(connect.CodeFailedPrecondition, errors.New("the Lucky question was answered already"))
		}
		a.reroll = luckyOf(h)
		return a, answerRepeated, nil
	}
	return a, answerOpen, nil
}

func errAnsweredAlready() error {
	return connect.NewError(connect.CodeFailedPrecondition, errors.New("the roll was answered already"))
}

// rollAnswer rolls the new die the first answer to a question asks for, after checking that the caller
// may answer and rolls it the way the campaign has them roll (RN-18; the master is exempt).
func (s *Service) rollAnswer(ctx context.Context, m authz.Membership, h heldRoll, req answerRequest, a *heldReply) error {
	if req.question == askDie {
		return s.rollDieAnswer(ctx, m, h, req, a)
	}
	if h.Kind == heldWeaponAttack {
		if err := s.mayAnswerHeld(ctx, m, h); err != nil {
			return err
		}
	}
	if !req.yes {
		return nil
	}
	if err := s.mustRollAs(ctx, m, req.in); err != nil {
		return err
	}
	var err error
	a.reroll, err = s.rollLuckyDie(ctx, derefInt32(h.LuckyIndex), req.in)
	return err
}

// rollDieAnswer rolls the Bardic Inspiration die the player uses, from the die the roller holds now.
func (s *Service) rollDieAnswer(ctx context.Context, m authz.Membership, h heldRoll, req answerRequest, a *heldReply) error {
	if !req.yes {
		return nil
	}
	st, err := s.dieToUse(ctx, h)
	if err != nil {
		return err
	}
	if h.Kind == heldWeaponAttack {
		if err := s.mayAnswerHeld(ctx, m, h); err != nil {
			return err
		}
	}
	if err := s.mustRollAs(ctx, m, req.in); err != nil {
		return err
	}
	face, err := s.rollDieFace(ctx, st.Die.Sides, req.in)
	if err != nil {
		return err
	}
	a.die = &bonusUse{Sides: st.Die.Sides, Face: face}
	if c := st.Die.Combatant; c != nil {
		a.die.From, a.die.ExpiresRound = deref(c.InspirationFrom), derefInt32(c.InspirationExpiresRound)
	}
	return nil
}

// mustRollAs checks the way a player rolls the new die; the master rolls either way, for anyone.
func (s *Service) mustRollAs(ctx context.Context, m authz.Membership, in rollInput) error {
	if m.Role == authz.RoleMaster {
		return nil
	}
	return s.mustRollThisWay(ctx, nil, m, in)
}

// dieToUse is the die the roller holds now, which the answer uses. A weapon attack's is its combatant's, in
// the round the roll was held, whichever table keeps it (a creature or the copy of an NPC shares its character
// with others, which hold dice of their own); every other roll's is wherever its character keeps it.
func (s *Service) dieToUse(ctx context.Context, h heldRoll) (rollerState, error) {
	if h.Kind == heldWeaponAttack {
		who, found, err := s.holdCombatant(ctx, h)
		if err != nil {
			return rollerState{}, err
		}
		st := stateOfCombatant(who, derefInt32(h.Round))
		if !found || !st.Holds {
			return rollerState{}, connect.NewError(connect.CodeFailedPrecondition, errors.New("the combatant holds no die any more"))
		}
		return st, nil
	}
	die, found, err := s.dieOf(ctx, s.queries, h.CampaignID, h.CharacterID)
	if err != nil {
		return rollerState{}, s.dbError(ctx, "find the die", err)
	}
	if !found {
		return rollerState{}, errDieGone()
	}
	return rollerState{Die: die, Holds: true}, nil
}

// rollDieFace rolls the die in the app, or takes the face the player typed from a real die.
func (s *Service) rollDieFace(ctx context.Context, sides int32, in rollInput) (int32, error) {
	face := in.typed
	if in.inApp {
		rolled, err := dice.Roll(s.roller, dice.Expr{Count: 1, Sides: int(sides)})
		if err != nil {
			return 0, s.dbError(ctx, "roll the die", err)
		}
		face = rolled.Faces[0]
	} else if face < 1 || face > int(sides) {
		return 0, connect.NewError(connect.CodeInvalidArgument, errors.New("typed_face must be a face of the die"))
	}
	return clamp32(face, 1, int(sides)), nil
}

// rerunHeld runs the path that held the roll again with the answer. A weapon attack that was resolved
// already is not given the answer: the path finds its event by the key and gives it as it was.
func (s *Service) rerunHeld(ctx context.Context, h heldRoll, a heldReply, state answerState) (proto.Message, error) {
	def, ok := heldRegistry[h.Kind]
	if !ok || def.rerun == nil {
		return nil, errHeldNotFound()
	}
	if state != answerSettled || h.Kind != heldWeaponAttack {
		ctx = withHeldReply(ctx, a)
	}
	return def.rerun(ctx, s, h, a.key)
}

// rollPick is the oneof an answer carries for the new die: roll_in_app or typed_face, nil for the one not
// set.
type rollPick struct {
	app  *bool
	face *int32
}

// rollOfAnswer reads how the new die of an answer comes: rolled in the app or typed from a real die. The
// Lucky die is a d20, so its typed face is 1 to 20; the Bardic Inspiration die's face is checked against the
// die it is held.
func rollOfAnswer(p rollPick, q questionKind) (rollInput, error) {
	var in rollInput
	switch {
	case p.app != nil:
		if !*p.app {
			return in, connect.NewError(connect.CodeInvalidArgument, errors.New("roll_in_app must be true"))
		}
		in.inApp = true
	case p.face != nil:
		if q == askLucky && (*p.face < 1 || *p.face > d20Sides) {
			return in, connect.NewError(connect.CodeInvalidArgument, errors.New("typed_face must be 1 to 20"))
		}
		in.typed = int(*p.face)
	case q == askLucky:
		return in, connect.NewError(connect.CodeInvalidArgument, errors.New("set roll_in_app or typed_face to roll again"))
	default:
		return in, connect.NewError(connect.CodeInvalidArgument, errors.New("set roll_in_app or typed_face to use the die"))
	}
	return in, nil
}
