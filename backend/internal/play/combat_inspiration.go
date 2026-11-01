package play

import (
	"context"
	"slices"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// Using a Bardic Inspiration die (SRD 5.1, Bard 1): the creature "can wait until after it
// rolls the d20 before deciding to use the Bardic Inspiration die, but must decide before
// the GM says whether the roll succeeds or fails". A character that holds a die and rolls an
// attack therefore does not get the result with its roll: RollAttack rolls (or takes) the d20
// and keeps it with the request in an inspiration_holds row, writes nothing else, and answers with
// the offer. AnswerBardicInspiration then asks RollAttack again for the same request with the
// d20 already rolled, and with the die rolled and added when the player chose to use it.
// A weapon attack (and the attack cantrips) is held here, in inspiration_holds (roll_holds is only
// read, for the questions an older release left open). The other d20 rolls of the character wait in
// the same table, as the scene check does (inspiration_held.go): the spell attack against one target, the saving throw of an effect, the concentration save, the trap
// search and the puzzle hint try. The saving throws the app rolls for a target in a caster's call
// (a spell's save, a hit rider's) do not wait for the die: the target's player is not rolling.
// The same row also asks the halfling's Lucky question about a d20 that shows a 1
// (combat_lucky.go), before the die's.

// bonusUse is a Bardic Inspiration die rolled and added to an attack roll, with the die as it
// was held, for the undo.
type bonusUse struct {
	Sides, Face  int32
	From         string
	ExpiresRound int32
}

// windowHoldSuffix marks the key of a held roll that a reaction window held before it was rolled.
const windowHoldSuffix = "#window"

// d20Sides is the d20 the held roll was made with.
const d20Sides = 20

// dieHeldBy is the Bardic Inspiration die the combatant holds in the round, nil when it holds none.
func dieHeldBy(c playdb.Combatant, round int32) *playv1.InspirationDie {
	if !holdsDie(c, round) {
		return nil
	}
	return &playv1.InspirationDie{Sides: *c.InspirationSides, ExpiresAtRound: *c.InspirationExpiresRound, FromCombatantId: deref(c.InspirationFrom)}
}

// heldAttackOffer is what RollAttack answers for a held roll: the d20 and the question, the attack
// not resolved (no outcome, no action spent, no damage). The Lucky question comes first; the
// die's comes once it is answered.
func heldAttackOffer(enc *playv1.Encounter, h *heldOffer, attackerID, targetID, attackKey string, cs []playdb.Combatant) *playv1.RollAttackResponse {
	out := &playv1.RollAttackResponse{Encounter: enc}
	var d20 *playv1.DiceRoll
	switch {
	case h.Asks == askLucky:
		out.LuckyOffer = h.luckyOffer()
		d20 = out.LuckyOffer.GetD20()
	case h.Asks == askStroke: // the one question has no older field: the d20 alone
		d20 = heldD20Of(h.Hold)
	case h.State.Holds:
		out.InspirationOffer = h.attackDieOffer(dieHeldBy(*h.State.Die.Combatant, h.State.Die.Round), cs)
		d20 = out.InspirationOffer.GetD20()
	default: // the die ran out before a retry read the hold: the d20 alone
		d20 = heldD20Of(h.Hold)
	}
	out.Roll = &playv1.AttackRoll{AttackerId: attackerID, TargetId: targetID, AttackKey: attackKey, D20: d20}
	return out
}

// inspirationOfferView is the held roll of a combatant, for its player and the master. A roll
// that waits for its Lucky answer asks that first (luckyOfferView): one question at a time.
func inspirationOfferView(d *encounterData, c playdb.Combatant, v combatViewer) *playv1.InspirationOffer {
	die := dieHeldBy(c, d.enc.Round)
	if (!v.master && !v.owns(c)) || die == nil {
		return nil
	}
	for _, h := range d.holds {
		if h.CombatantID != c.ID || derefInt32(h.Round) != d.enc.Round {
			continue
		}
		if luckyAsks(h) || strokeAsks(h) {
			return nil
		}
		return heldOffer{Hold: h, Asks: askDie}.attackDieOffer(die, d.cs)
	}
	return nil
}

// bonusRollOf reads how the Bardic Inspiration die of AnswerBardicInspiration comes, when the player
// uses it: rolled in the app or typed from a real die.
func bonusRollOf(msg *playv1.AnswerBardicInspirationRequest) (rollInput, error) {
	if !msg.GetUse() {
		return rollInput{}, nil
	}
	var p rollPick
	switch roll := msg.GetRoll().(type) {
	case *playv1.AnswerBardicInspirationRequest_RollInApp:
		p.app = &roll.RollInApp
	case *playv1.AnswerBardicInspirationRequest_TypedFace:
		p.face = &roll.TypedFace
	}
	return rollOfAnswer(p, askDie)
}

// AnswerBardicInspiration implements playv1connect.ResourceServiceHandler: the player of a
// held roll uses the die on it or keeps it, and the attack is resolved.
func (s *Service) AnswerBardicInspiration(
	ctx context.Context,
	req *connect.Request[playv1.AnswerBardicInspirationRequest],
) (*connect.Response[playv1.AnswerBardicInspirationResponse], error) {
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
	in, err := bonusRollOf(req.Msg)
	if err != nil {
		return nil, err
	}
	out, err := s.answerHeld(ctx, m, answerRequest{
		ref: holdRef{Class: attackHold, Campaign: m.CampaignID, Encounter: encID, ID: holdID}, key: key, question: askDie, yes: req.Msg.GetUse(), in: in,
	})
	if err != nil {
		return nil, err
	}
	attack, _ := out.(*playv1.RollAttackResponse)
	return connect.NewResponse(&playv1.AnswerBardicInspirationResponse{Attack: attack}), nil
}

// bonusDiceOf is the Bardic Inspiration die an attack roll used, as the roll shows it.
func bonusDiceOf(ev actionEvent) []*playv1.BonusDie {
	if ev.Res == nil || ev.Res.Kind != resBardicUse {
		return nil
	}
	return []*playv1.BonusDie{{SourceKey: bardicInspirationFt, Sides: ev.Res.Sides, Face: ev.Res.Face, Used: true}}
}

// heldAttackResponse is RollAttack's answer for a roll that waits: the combat as the caller sees it,
// the d20 and the question about the die.
func (s *Service) heldAttackResponse(ctx context.Context, m authz.Membership, res combatResult, h *heldOffer, req *playv1.RollAttackRequest) (*connect.Response[playv1.RollAttackResponse], error) {
	out, err := s.finish(ctx, m, res, s.changed(m.CampaignID))
	if err != nil {
		return nil, err
	}
	cs, err := s.queries.ListCombatants(ctx, res.encounterID)
	if err != nil {
		return nil, s.dbError(ctx, "list the combatants", err)
	}
	resp := heldAttackOffer(out, h, req.GetAttackerId(), req.GetTargetId(), req.GetAttackKey(), cs)
	if i := slices.IndexFunc(cs, func(c playdb.Combatant) bool { return c.ID == req.GetAttackerId() }); i >= 0 {
		resp.HeldRollOffer = combatantOffer(*h, cs[i], cs)
	}
	return connect.NewResponse(resp), nil
}

// typedRoll says the d20 of a request came from real dice: a typed face, or typed faces with
// no roll in the app (a held roll keeps the faces it rolled in the app too, with the app's roll).
func typedRoll(r *playv1.RollAttackRequest) bool {
	switch r.GetRoll().(type) {
	case *playv1.RollAttackRequest_D20Face:
		return true
	case nil:
		return len(r.GetD20Faces()) > 0
	}
	return false
}

// intsOf converts the faces of a request.
func intsOf(faces []int32) []int {
	out := make([]int, len(faces))
	for i, f := range faces {
		out[i] = int(f)
	}
	return out
}
