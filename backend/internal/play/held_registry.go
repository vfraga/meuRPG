package play

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/anypb"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// heldKindDef is what a kind of held roll does that the gate cannot know.
type heldKindDef struct {
	// ownKindOnly says only the holds of this very kind refuse it, and are forgotten when stale: the weapon attack
	// shares the table with the combatant's other rolls, which heldBlocked, called first by its path, refuses it
	// for (and does not when a reaction window replays the attack, a case the gate keeps as it was).
	ownKindOnly bool
	// rerun runs the request the hold kept again, with the answer in the context, by the very path that
	// held it, and gives the path's own response; key is the key the roll settles under.
	rerun func(ctx context.Context, s *Service, h heldRoll, key string) (proto.Message, error)
	// offerOf is the attack and the target a question names (the weapon attack only).
	offerOf func(h heldRoll) (attackKey, targetID string)
	// withFaces pins the d20 a roll was made with in the request the hold keeps (the weapon attack only: a
	// roll with advantage or disadvantage keeps both dice).
	withFaces func(req proto.Message, faces []int)
	// ownRoll says the path rolls its own d20 when nothing is asked, so the gate gives it nothing back then.
	// Today's behavior, pinned by the tests of the group check and of the trap search; they keep rollCheck
	// and the second die of a Perception search.
	ownRoll bool
	// rerollApplies is the word of the reroll rules that cover the kind (rules.RollApplies*: "attack", "check" or "save"),
	// "" for a kind no reroll rule reaches (held_reroll.go).
	rerollApplies string
}

// pinned is the request to keep with a roll: as the caller pinned it, with the dice of the roll.
func (d heldKindDef) pinned(req proto.Message, faces []int) ([]byte, error) {
	if d.withFaces != nil {
		d.withFaces(req, faces)
	}
	return proto.Marshal(req) //nolint:wrapcheck // the gate wraps it
}

// heldRegistry is the registry of the kinds of held roll. It is filled by init, not by its declaration: the
// paths it runs again read it, and Go refuses an initialization that would go round.
var heldRegistry map[heldKind]heldKindDef

func init() {
	heldRegistry = map[heldKind]heldKindDef{
		heldWeaponAttack: {
			ownKindOnly: true,
			rerun:       rerunAttack, offerOf: attackOfRequest, withFaces: pinAttackFaces, rerollApplies: rules.RollAppliesAttack,
		},
		heldSpellAttack:       {rerun: registerRerun((*Service).CastSpell), rerollApplies: rules.RollAppliesAttack},
		heldEffectSave:        {rerun: registerRerun((*Service).RollEffectSave), rerollApplies: rules.RollAppliesSave},
		heldConcentrationSave: {rerun: registerRerun((*Service).ResolveConcentrationSave), rerollApplies: rules.RollAppliesSave},
		heldSceneCheck:        {rerun: registerRerun((*Service).RollSceneCheck), rerollApplies: rules.RollAppliesCheck},
		heldGroupCheck:        {rerun: rerunGroupCheck, ownRoll: true, rerollApplies: rules.RollAppliesCheck},
		heldTrapSearch:        {rerun: registerRerun((*Service).SearchForTraps), ownRoll: true},
		heldHintTry:           {rerun: registerRerun((*Service).TryPuzzleHint)},
		heldDeathSave:         {rerun: registerRerun((*Service).RollDeathSave)},
		heldHide:              {rerun: registerRerun((*Service).Hide)},
		heldCounterspell:      {rerun: registerRerun((*Service).AnswerReaction)},
		heldContest:           {rerun: rerunContest},
		heldInitiative:        {rerun: registerRerun((*Service).SubmitInitiative)},
	}
}

// registerRerun is the rerun of a path: it reads the request the hold kept and calls the path with it.
func registerRerun[Req, Res any, PReq interface {
	*Req
	proto.Message
}, PRes interface {
	*Res
	proto.Message
}](path func(*Service, context.Context, *connect.Request[Req]) (*connect.Response[Res], error)) func(context.Context, *Service, heldRoll, string) (proto.Message, error) {
	return func(ctx context.Context, s *Service, h heldRoll, key string) (proto.Message, error) {
		orig := PReq(new(Req))
		if err := proto.Unmarshal(h.Request, orig); err != nil {
			return nil, s.dbError(ctx, "read the held roll", err)
		}
		if h.Window {
			underKey(orig, key)
		}
		out, err := path(s, ctx, connect.NewRequest((*Req)(orig)))
		if err != nil {
			return nil, err
		}
		return PRes(out.Msg), nil
	}
}

// underKey puts the request under the idempotency key given. The request that a reaction window held has its key on the event that
// held it (a retry of the request answers with that event), so the roll the window's replay held is answered, and the path run
// again, under the answer's own key: under the request's, the path finds the held event, takes the run for a retry and writes nothing.
func underKey(req proto.Message, key string) {
	m := req.ProtoReflect()
	if f := m.Descriptor().Fields().ByName("idempotency_key"); f != nil {
		m.Set(f, protoreflect.ValueOfString(key))
	}
}

// rerunGroupCheck runs a group check's roll again: the player's own roll (RollGroupCheck), or the master's roll for a
// player (RollForPlayer, E4-10), which the hold's roller says: the master rolled it when the user who rolled is
// not the character's player.
func rerunGroupCheck(ctx context.Context, s *Service, h heldRoll, key string) (proto.Message, error) {
	role, err := s.holdRollerRole(ctx, h)
	if err != nil {
		return nil, err
	}
	if role == authz.RoleMaster {
		return registerRerun((*Service).RollForPlayer)(ctx, s, h, key)
	}
	return registerRerun((*Service).RollGroupCheck)(ctx, s, h, key)
}

// rerunContest runs a contest's roll again: the initiator's (StartContest) or the defender's (RespondContest), which the
// request the hold kept says (it is kept as an Any, since both are the one kind of held roll).
func rerunContest(ctx context.Context, s *Service, h heldRoll, key string) (proto.Message, error) {
	var kept anypb.Any
	if err := proto.Unmarshal(h.Request, &kept); err != nil {
		return nil, s.dbError(ctx, "read the held roll", err)
	}
	req, err := kept.UnmarshalNew()
	if err != nil {
		return nil, s.dbError(ctx, "read the held roll", err)
	}
	if h.Window {
		underKey(req, key)
	}
	switch r := req.(type) {
	case *playv1.StartContestRequest:
		out, err := s.StartContest(ctx, connect.NewRequest(r))
		if err != nil {
			return nil, err
		}
		return out.Msg, nil
	case *playv1.RespondContestRequest:
		out, err := s.RespondContest(ctx, connect.NewRequest(r))
		if err != nil {
			return nil, err
		}
		return out.Msg, nil
	}
	return nil, s.dbError(ctx, "read the held roll", fmt.Errorf("a held contest of request %T", req))
}

// rerunAttack runs the weapon attack again. The attack that a reaction window held first has its event
// under the request's key: the question's answer settles under the answer's own.
func rerunAttack(ctx context.Context, s *Service, h heldRoll, key string) (proto.Message, error) {
	var orig playv1.RollAttackRequest
	if err := proto.Unmarshal(h.Request, &orig); err != nil {
		return nil, s.dbError(ctx, "read the held roll", err)
	}
	if h.Window {
		orig.IdempotencyKey = key
	}
	out, err := s.rollAttackAgain(ctx, connect.NewRequest(&orig))
	if err != nil {
		return nil, err
	}
	return out.Msg, nil
}

// retryOfHeldAttack says the request is the retry of a weapon attack that was held and answered: the attack was written under
// the request's key from the request the hold kept, which the gate pinned (the mode and its reason, both d20 of a pair), so
// the stored hash is that one's. The retry is the same request when pinning it as the hold did gives that hash.
func retryOfHeldAttack(msg *playv1.RollAttackRequest) func(ctx context.Context, q *playdb.Queries, stored *string) bool {
	return func(ctx context.Context, q *playdb.Queries, stored *string) bool {
		row, err := q.GetInspirationHoldByKey(ctx, playdb.GetInspirationHoldByKeyParams{CampaignID: msg.GetCampaignId(), IdempotencyKey: msg.GetIdempotencyKey()})
		if err != nil {
			return false
		}
		h := heldOfInspirationHold(row)
		var pinned playv1.RollAttackRequest
		if h.Kind != heldWeaponAttack || h.Window || proto.Unmarshal(h.Request, &pinned) != nil {
			return false
		}
		return !hashDiffers(stored, idem.Hash(&pinned)) && sameApartFromPins(msg, &pinned)
	}
}

// sameApartFromPins says two attack requests are the same but for what the gate pins in a held one.
func sameApartFromPins(a, b *playv1.RollAttackRequest) bool {
	unpin := func(r *playv1.RollAttackRequest) *playv1.RollAttackRequest {
		c, _ := proto.Clone(r).(*playv1.RollAttackRequest)
		c.RollMode, c.ModeReason, c.D20Faces, c.PowerAttackPin = 0, "", nil, nil
		return c
	}
	return proto.Equal(unpin(a), unpin(b))
}

// attackOfRequest is the attack and the target of the request a weapon attack's hold kept.
func attackOfRequest(h heldRoll) (attackKey, targetID string) {
	var orig playv1.RollAttackRequest
	_ = proto.Unmarshal(h.Request, &orig) // a request this module wrote
	return orig.GetAttackKey(), orig.GetTargetId()
}

// pinAttackFaces keeps both d20 of a weapon attack rolled with advantage or disadvantage in the request,
// so that the answer settles this roll and not another.
func pinAttackFaces(req proto.Message, faces []int) {
	r, ok := req.(*playv1.RollAttackRequest)
	if !ok {
		return
	}
	sent := len(r.D20Faces) > 0 // a die typed as a list of one is kept as such: the answer asks for the roll again
	r.D20Faces = nil
	if len(faces) > 1 || sent {
		r.D20Faces = faces32(faces)
	}
}
