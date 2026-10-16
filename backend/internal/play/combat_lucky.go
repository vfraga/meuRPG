package play

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dice"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// The halfling's Lucky (SRD 5.1, Halfling): a halfling who rolls a 1 on the d20 of an attack roll,
// an ability check or a saving throw may roll that die again, and has to keep the new roll. With
// advantage or disadvantage a feature like this rerolls only one of the two d20 (SRD 5.1,
// "Advantage and Disadvantage"), and the SRD's own example rerolls a 1 that advantage would drop:
// the question comes when any of the dice shows a 1, and only that die is rolled again (the first
// one when both show a 1).
//
// The table asks the player, "Tirou 1: rolar de novo?", on the roll of a weapon attack or an
// attack cantrip. The roll waits in the same roll_holds row a Bardic Inspiration die waits in
// (combat_inspiration.go): RollAttack keeps the d20 of a lucky attacker that shows a 1 with the
// request (lucky_index), resolves and spends nothing, and answers with the question. AnswerLucky
// rolls the die again (in the app or typed, RN-18), or keeps the 1, and asks RollAttack again for
// the same request with the new d20: the attack is resolved, or, when the attacker also holds a
// Bardic Inspiration die, the roll stays held for the die's question, now on the new roll. The 1
// is on the table first, so Lucky comes first; the die is added to whatever the d20 is after it.
// The new roll is never asked about again, even when it is another 1.
//
// The saving throws the app rolls for a player's character inside someone else's call (a spell's
// save, a trap's, the Dexterity save against Hellish Rebuke) have nobody to ask, so the app rolls
// the 1 again by itself (autoLucky): a reroll of a 1 never lowers the result, so a player always
// takes it. The new roll stands, even another 1; with advantage or disadvantage the first die that
// shows a 1 is the one rolled again, and the die that counts is picked again from the new pair.
// Only a player's character is lucky there (luckySaver): an NPC never is, whatever its sheet says.
// The reroll is kept where the save keeps its d20 (saveRoll.Lucky for a spell's and a trap's save,
// actionEvent.Lucky for Hellish Rebuke), so the result and the log say "Sortudo: 1 → 14" to whoever
// reads the save's dice, and the undo of the call has nothing more to put back.

// luckyReroll is the die Lucky rolled again: its index in the faces in the order rolled, the 1 it
// showed and the new face. An attack's event keeps it, so the log still says "Sortudo: 1 → 14"
// after a reload.
type luckyReroll struct {
	Index int32 `json:"index"`
	From  int32 `json:"from"`
	To    int32 `json:"to"`
}

// proto is the reroll as a DiceRoll lists it: none for a nil reroll (no question, or the 1 kept).
func (r *luckyReroll) proto() []*playv1.Reroll {
	if r == nil {
		return nil
	}
	return []*playv1.Reroll{{Index: r.Index, From: r.From, To: r.To, SourceKey: link.LuckyTrait}}
}

// luckyNote is the sentence a log line written as text adds for the d20 Lucky rolled again
// (" Sortudo: 1 → 14."), "" when none was: the structured lines carry it in DiceRoll.rerolled.
func luckyNote(r *luckyReroll) string {
	if r == nil {
		return ""
	}
	return fmt.Sprintf(" Sortudo: %d → %d.", r.From, r.To)
}

// luckyPending says the hold waits for its Lucky answer: a die showed a 1 and neither the Lucky
// question nor the roll was answered.
func luckyPending(h playdb.RollHold) bool {
	return h.LuckyIndex != nil && h.LuckyAnswerKey == nil && h.AnswerKey == nil
}

// luckyOfHold is the die a hold's Lucky answer rolled again; nil before the answer, when the
// player kept the 1, and for a hold that asked no Lucky question.
func luckyOfHold(h playdb.RollHold) *luckyReroll {
	if h.LuckyIndex == nil || h.LuckyTo == nil {
		return nil
	}
	return &luckyReroll{Index: *h.LuckyIndex, From: 1, To: *h.LuckyTo}
}

// luckyIndexOf is the die of a lucky attacker's roll that shows a 1, the one Lucky would roll
// again: nil when none does, or when the attacker is not lucky.
func luckyIndexOf(faces []int, lucky bool) *int32 {
	if !lucky {
		return nil
	}
	i := slices.Index(faces, 1)
	if i < 0 {
		return nil
	}
	return new(clamp32(i, 0, 1))
}

// luckySaver says the app rolls a 1 again on a saving throw it rolls for this combatant: a
// player's character whose sheet has Lucky. An NPC is never lucky here, even with a halfling's
// sheet.
func luckySaver(c playdb.Combatant, traits link.Traits) bool {
	return c.Kind == kindPlayer && traits.Lucky
}

// autoLucky rolls again, with d20, the first of the faces that shows a 1 when the roller is lucky,
// and gives the new faces and what Lucky did; the faces as they were and nil when nothing was
// rolled again. The new face stands, even another 1.
func autoLucky(faces []int, lucky bool, d20 func() (int, error)) ([]int, *luckyReroll, error) {
	at := luckyIndexOf(faces, lucky)
	if at == nil {
		return faces, nil, nil
	}
	face, err := d20()
	if err != nil {
		return nil, nil, err
	}
	out := slices.Clone(faces)
	out[*at] = face
	return out, &luckyReroll{Index: *at, From: 1, To: clamp32(face, 1, d20Sides)}, nil
}

// appD20 rolls one d20 in the app: the die autoLucky rolls again on a save the app rolled.
func (s *Service) appD20() (int, error) {
	face, _, err := s.d20(rollInput{inApp: true}, 0)
	return face, err
}

// luckySave is a saving throw's d20 the app rolled (roll, with the mode and the modifier it was
// rolled with) after autoLucky: the die that counts and the total are picked again from the new
// faces, as d20With picks them. The roll as it was and nil when nothing was rolled again.
func (s *Service) luckySave(roll d20Roll, modifier int, mode combat.RollMode, lucky bool) (d20Roll, *luckyReroll, error) {
	faces, reroll, err := autoLucky(roll.Faces, lucky, s.appD20)
	if err != nil || reroll == nil {
		return roll, nil, err
	}
	idx := mode.Pick(faces)
	return d20Roll{Faces: faces, Index: idx, Total: faces[idx] + modifier, Physical: roll.Physical}, reroll, nil
}

// luckyOfferOf is the question a hold asks the player of a lucky attacker: the d20 with the
// attack's bonus, the die that shows the 1, the attack and its target.
func luckyOfferOf(hold playdb.RollHold, physical bool) *playv1.LuckyOffer {
	var orig playv1.RollAttackRequest
	_ = proto.Unmarshal(hold.Request, &orig) // a request this module wrote
	return &playv1.LuckyOffer{
		HoldId: hold.ID, D20: heldD20(hold, physical), LuckyIndex: derefInt32(hold.LuckyIndex),
		AttackKey: orig.GetAttackKey(), TargetId: orig.GetTargetId(),
	}
}

// luckyOfferView is the held roll of a combatant that waits for its Lucky answer, for its player
// and the master, so the question is still there when the screen is read again.
func luckyOfferView(d *encounterData, c playdb.Combatant, v combatViewer) *playv1.LuckyOffer {
	if !v.master && !v.owns(c) {
		return nil
	}
	for _, h := range d.holds {
		if h.CombatantID == c.ID && h.Round == d.enc.Round && luckyPending(h) {
			_, physical := rollOfHold(h)
			return luckyOfferOf(h, physical)
		}
	}
	return nil
}

// mayHold says the attack's d20 may wait for a question in this call: the roll of an attacker
// that holds a Bardic Inspiration die, or of a lucky one (held only when a die shows a 1), or the
// Lucky answer of an attacker that holds a die (held again for the die's question). Its mode is
// then worked out without writing (chooseMode's dry), and keepReason writes the reason of a
// change of mode once the roll goes on after all.
func mayHold(f *forcedRoll, attacker playdb.Combatant, round int32, lucky bool) bool {
	die := holdsDie(attacker, round)
	if f != nil {
		return f.luckyKey != "" && die
	}
	return die || lucky
}

// answerLuckyHold writes the Lucky answer in the hold, inside the attack's transaction: the
// answer's key and the new face, and, for a roll that stays held for the Bardic Inspiration
// question (stays), the d20 after it (the face that counts and the request's pair of faces), which
// the die's answer settles. A roll resolved now keeps its request as it was, the one its event is
// written for. The question must still wait: an answer that got there first refuses this one. The
// same answer again (a retry) finds the hold as it left it.
func answerLuckyHold(ctx context.Context, c *combatTx, f *forcedRoll, faces []int, idx int, stays bool) (playdb.RollHold, error) {
	hold, err := c.q.GetRollHold(ctx, playdb.GetRollHoldParams{EncounterID: c.enc.ID, ID: f.holdID})
	if err != nil {
		return playdb.RollHold{}, fmt.Errorf("find the held roll: %w", err)
	}
	if hold.LuckyAnswerKey != nil && *hold.LuckyAnswerKey == f.luckyKey {
		return hold, nil
	}
	if !luckyPending(hold) {
		return playdb.RollHold{}, connect.NewError(connect.CodeFailedPrecondition, errors.New("the Lucky question was answered already"))
	}
	face, body := hold.Face, hold.Request
	if stays {
		var req playv1.RollAttackRequest
		if err := proto.Unmarshal(hold.Request, &req); err != nil {
			return playdb.RollHold{}, fmt.Errorf("read the held request: %w", err)
		}
		if len(faces) == pairDice {
			req.D20Faces = faces32(faces)
		}
		if body, err = proto.Marshal(&req); err != nil {
			return playdb.RollHold{}, fmt.Errorf("keep the request: %w", err)
		}
		face = clamp32(faces[idx], 1, d20Sides)
	}
	var to *int32
	if f.lucky != nil {
		to = new(f.lucky.To)
	}
	hold, err = c.q.AnswerRollHoldLucky(ctx, playdb.AnswerRollHoldLuckyParams{
		ID: hold.ID, Face: face, Request: body, LuckyAnswerKey: &f.luckyKey, LuckyTo: to,
	})
	if err != nil {
		return playdb.RollHold{}, fmt.Errorf("answer the Lucky question: %w", err)
	}
	return hold, nil
}

// heldFaces are the d20 of a hold in the order rolled: the request's pair for a roll with
// advantage or disadvantage, else the one face.
func heldFaces(hold playdb.RollHold, orig *playv1.RollAttackRequest) []int {
	if faces := orig.GetD20Faces(); len(faces) == pairDice {
		return intsOf(faces)
	}
	return []int{int(hold.Face)}
}

// luckyRollOf reads how the new d20 of AnswerLucky comes: none when the player keeps the 1; else
// rolled in the app or typed from a real die, 1 to 20.
func luckyRollOf(msg *playv1.AnswerLuckyRequest) (rollInput, error) {
	var in rollInput
	if !msg.GetReroll() {
		return in, nil
	}
	switch roll := msg.GetRoll().(type) {
	case *playv1.AnswerLuckyRequest_RollInApp:
		if !roll.RollInApp {
			return in, connect.NewError(connect.CodeInvalidArgument, errors.New("roll_in_app must be true"))
		}
		in.inApp = true
	case *playv1.AnswerLuckyRequest_TypedFace:
		if roll.TypedFace < 1 || roll.TypedFace > d20Sides {
			return in, connect.NewError(connect.CodeInvalidArgument, errors.New("typed_face must be 1 to 20"))
		}
		in.typed = int(roll.TypedFace)
	default:
		return in, connect.NewError(connect.CodeInvalidArgument, errors.New("set roll_in_app or typed_face to roll again"))
	}
	return in, nil
}

// AnswerLucky implements playv1connect.ResourceServiceHandler: the player of a held roll whose d20
// shows a 1 rolls that die again, or keeps the 1. The attack is then resolved, or, when the
// attacker holds a Bardic Inspiration die, held again for the die's question on the new roll.
func (s *Service) AnswerLucky(
	ctx context.Context,
	req *connect.Request[playv1.AnswerLuckyRequest],
) (*connect.Response[playv1.AnswerLuckyResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	key, err := parseKey(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	encID, err := parseCombatID(req.Msg.GetEncounterId(), "encounter")
	if err != nil {
		return nil, err
	}
	holdID, err := parseCombatID(req.Msg.GetHoldId(), "held roll")
	if err != nil {
		return nil, err
	}
	in, err := luckyRollOf(req.Msg)
	if err != nil {
		return nil, err
	}
	hold, err := s.queries.GetRollHold(ctx, playdb.GetRollHoldParams{EncounterID: encID, ID: holdID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("held roll not found"))
	}
	if err != nil {
		return nil, s.dbError(ctx, "find the held roll", err)
	}
	var orig playv1.RollAttackRequest
	if err := proto.Unmarshal(hold.Request, &orig); err != nil || orig.GetCampaignId() != m.CampaignID {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("held roll not found"))
	}
	if hold.LuckyIndex == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("the roll asks no Lucky question"))
	}
	window := strings.HasSuffix(hold.IdempotencyKey, windowHoldSuffix) // the original key belongs to the event that stood for the held attack
	if hold.AnswerKey != nil {
		// Resolved already: the same Lucky answer again gives the attack as it was resolved,
		// under the key it was resolved with.
		if *hold.AnswerKey != key && deref(hold.LuckyAnswerKey) != key {
			return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("the roll was answered already"))
		}
		if window {
			orig.IdempotencyKey = *hold.AnswerKey
		}
		return s.luckyAnswered(ctx, &orig, nil)
	}
	if window {
		orig.IdempotencyKey = key
	}
	if hold.LuckyAnswerKey != nil {
		// Answered, and held for the Bardic Inspiration question: the same answer again reads the
		// hold as it is, and rolls nothing.
		if *hold.LuckyAnswerKey != key {
			return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("the Lucky question was answered already"))
		}
		return s.luckyAnswered(ctx, &orig, forcedLucky(hold, &orig, key, luckyOfHold(hold)))
	}
	if err := s.mayAnswerLucky(ctx, m, hold, in, req.Msg.GetReroll()); err != nil {
		return nil, err
	}
	var reroll *luckyReroll
	if req.Msg.GetReroll() {
		if reroll, err = s.rollLuckyDie(ctx, *hold.LuckyIndex, in); err != nil {
			return nil, err
		}
	}
	// The attack as it was asked, with the new d20 (or the 1 kept): it resolves against the combat
	// as it is now, and spends the action in its own transaction, under the key the roll came with.
	return s.luckyAnswered(ctx, &orig, forcedLucky(hold, &orig, key, reroll))
}

// forcedLucky is the held roll's d20 after the Lucky answer, for the RollAttack it calls: the die
// that showed the 1 replaced by the new face when the player rolled again.
func forcedLucky(hold playdb.RollHold, orig *playv1.RollAttackRequest, key string, reroll *luckyReroll) *forcedRoll {
	faces := heldFaces(hold, orig)
	if reroll != nil && hold.LuckyAnswerKey == nil && int(reroll.Index) < len(faces) {
		faces[reroll.Index] = int(reroll.To)
	}
	f := &forcedRoll{
		holdID: hold.ID, answerKey: key, luckyKey: key, toHit: int(hold.Modifier), lucky: reroll,
		// The d20 was typed when the roll was: the attack knows it from the request.
		physical: typedRoll(orig),
	}
	if len(faces) == pairDice {
		f.faces = faces
	} else {
		f.face = faces[0]
	}
	return f
}

// mayAnswerLucky checks, before anything is rolled, that the caller answers for the attacker (its
// player or the master) and, for a player who rolls again, rolls the way the campaign has them
// roll (RN-18).
func (s *Service) mayAnswerLucky(ctx context.Context, m authz.Membership, hold playdb.RollHold, in rollInput, reroll bool) error {
	cs, err := s.queries.ListCombatants(ctx, hold.EncounterID)
	if err != nil {
		return s.dbError(ctx, "list the combatants", err)
	}
	i := slices.IndexFunc(cs, func(c playdb.Combatant) bool { return c.ID == hold.CombatantID })
	if i < 0 {
		return connect.NewError(connect.CodeNotFound, errors.New("held roll not found"))
	}
	if err := viewerOf(m).mayAct(cs[i]); err != nil {
		return err
	}
	if !reroll || m.Role == authz.RoleMaster {
		return nil
	}
	return s.mustRollThisWay(ctx, nil, m, in)
}

// rollLuckyDie rolls again the die of a hold that shows the 1, at index (or takes the face the
// player typed from a real die): the new roll stands, whatever it is. AnswerLucky and
// AnswerOutsideLucky (inspiration_lucky.go) both roll it here.
func (s *Service) rollLuckyDie(ctx context.Context, index int32, in rollInput) (*luckyReroll, error) {
	face := in.typed
	if in.inApp {
		rolled, err := dice.Roll(s.roller, dice.Expr{Count: 1, Sides: d20Sides})
		if err != nil {
			return nil, s.dbError(ctx, "roll the d20", err)
		}
		face = rolled.Faces[0]
	}
	return &luckyReroll{Index: index, From: 1, To: clamp32(face, 1, d20Sides)}, nil
}

// luckyAnswered runs RollAttack for the held request with the Lucky answer's d20 (forced), or, for
// an answer already resolved (nil), reads the attack as it was resolved.
func (s *Service) luckyAnswered(ctx context.Context, orig *playv1.RollAttackRequest, forced *forcedRoll) (*connect.Response[playv1.AnswerLuckyResponse], error) {
	out, err := s.RollAttack(withForced(ctx, forced), connect.NewRequest(orig))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.AnswerLuckyResponse{Attack: out.Msg}), nil
}
