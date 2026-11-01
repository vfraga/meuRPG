package play

import (
	"context"
	"errors"
	"slices"
	"strings"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/play/live"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// The wire of the held rolls: the one question every roll that waits asks (HeldRollOffer), the one RPC that answers
// any of them (AnswerHeldRoll), the list of the questions the caller may answer (ListHeldRolls) and the hint that
// they changed (HeldRollsChanged). The paths keep putting `inspiration_offer` and `lucky_offer` in their
// answers; HeldRollOffer is set beside them.

// heldKindProto is the kind of a held roll on the wire.
var heldKindProto = map[heldKind]playv1.HeldRollKind{
	heldWeaponAttack:      playv1.HeldRollKind_HELD_ROLL_KIND_WEAPON_ATTACK,
	heldSpellAttack:       playv1.HeldRollKind_HELD_ROLL_KIND_SPELL_ATTACK,
	heldEffectSave:        playv1.HeldRollKind_HELD_ROLL_KIND_EFFECT_SAVE,
	heldConcentrationSave: playv1.HeldRollKind_HELD_ROLL_KIND_CONCENTRATION_SAVE,
	heldDeathSave:         playv1.HeldRollKind_HELD_ROLL_KIND_DEATH_SAVE,
	heldHide:              playv1.HeldRollKind_HELD_ROLL_KIND_HIDE,
	heldCounterspell:      playv1.HeldRollKind_HELD_ROLL_KIND_COUNTERSPELL_CHECK,
	heldContest:           playv1.HeldRollKind_HELD_ROLL_KIND_CONTEST,
	heldInitiative:        playv1.HeldRollKind_HELD_ROLL_KIND_INITIATIVE,
	heldSceneCheck:        playv1.HeldRollKind_HELD_ROLL_KIND_SCENE_CHECK,
	heldGroupCheck:        playv1.HeldRollKind_HELD_ROLL_KIND_GROUP_CHECK,
	heldTrapSearch:        playv1.HeldRollKind_HELD_ROLL_KIND_TRAP_SEARCH,
	heldHintTry:           playv1.HeldRollKind_HELD_ROLL_KIND_HINT_TRY,
}

// offerRoller is who a question is about, as the viewer reads it.
type offerRoller struct {
	// Label is the roller's name: the combatant's label in a combat, the character's name out of one.
	Label string
	// Character says the roller is a player's character, whose id may be named; a creature and an NPC are not.
	Character bool
	// Giver is the name of the bard that gave the die the question is about.
	Giver string
}

// heldRollOfferOf is the question a held roll asks, in the one shape every roll has: the d20 and its bonus, the kind of
// roll, who rolls it and the question that waits, the Lucky one first. It carries what the roller already sees
// and nothing else (RN-10, RN-20): the callers give it to the roller's player and the master only.
func heldRollOfferOf(o heldOffer, who offerRoller) *playv1.HeldRollOffer {
	h := o.Hold
	attackKey, targetID := attackOfHold(h)
	out := &playv1.HeldRollOffer{
		HoldId: h.ID, Kind: heldKindProto[h.Kind], CombatantId: h.CombatantID, RollerLabel: who.Label, D20: heldD20Of(h),
		AttackKey: attackKey, TargetId: targetID, CreatedAt: timestamppb.New(h.CreatedAt),
	}
	if who.Character {
		out.CharacterId = h.CharacterID
	}
	switch o.Asks {
	case askLucky:
		out.Question = &playv1.HeldRollOffer_Lucky{Lucky: &playv1.LuckyQuestion{LuckyIndex: derefInt32(h.LuckyIndex)}}
	case askIndomitable:
		out.Question = &playv1.HeldRollOffer_Indomitable{Indomitable: &playv1.IndomitableQuestion{Failed: h.IndomFailed}}
		if h.DieUsed != nil { // the total the roller sees counts the die already added
			out.D20.Total += h.DieUsed.Face
		}
	case askStroke:
		out.Question = &playv1.HeldRollOffer_StrokeOfLuck{StrokeOfLuck: &playv1.StrokeOfLuckQuestion{Failed: h.StrokeFailed}}
		if h.DieUsed != nil { // the total the roller sees counts the die already added
			out.D20.Total += h.DieUsed.Face
		}
	default:
		out.Question = &playv1.HeldRollOffer_Die{Die: &playv1.DieQuestion{Sides: o.State.Die.Sides, FromName: who.Giver}}
	}
	return out
}

// outsideHeldRollOffer is the question a roll that is not a weapon attack asks, as the paths put it in their answers: the
// roller is a player's character, in a combat or out of one.
func (s *Service) outsideHeldRollOffer(ctx context.Context, tx pgx.Tx, o heldOffer) *playv1.HeldRollOffer {
	who := offerRoller{Character: true, Giver: s.nameOf(ctx, tx, o.Hold.CampaignID, o.State.Die.Source)}
	if c := o.State.Die.Combatant; c != nil {
		who.Label = c.Label
	} else {
		who.Label = s.nameOf(ctx, tx, o.Hold.CampaignID, &o.Hold.CharacterID)
	}
	return heldRollOfferOf(o, who)
}

// combatantOffer is the question a combatant's weapon attack asks, as RollAttack puts it in its answer.
func combatantOffer(o heldOffer, c playdb.Combatant, cs []playdb.Combatant) *playv1.HeldRollOffer {
	return heldRollOfferOf(o, offerRoller{Label: c.Label, Character: c.Kind == kindPlayer && backedByCharacter(c), Giver: giverLabel(c, cs)})
}

// giverLabel is the name of the combatant that gave the die the combatant holds, "" when it holds none or the giver is gone.
func giverLabel(c playdb.Combatant, cs []playdb.Combatant) string {
	if c.InspirationFrom == nil {
		return ""
	}
	if i := slices.IndexFunc(cs, func(o playdb.Combatant) bool { return o.ID == *c.InspirationFrom }); i >= 0 {
		return cs[i].Label
	}
	return ""
}

// heldOfferOfCombatant is the question the combatant's roll that waits asks in the round, nil when none: the oldest of
// its holds that is not stale. A combatant's question is its master's and its player's, and nobody else's.
func heldOfferOfCombatant(d *encounterData, c playdb.Combatant, v combatViewer) *playv1.HeldRollOffer {
	if !v.master && !v.owns(c) {
		return nil
	}
	if asked, ok := d.setupOffers[c.CharacterID]; ok && backedByCharacter(c) {
		return combatantOffer(asked, c, d.cs)
	}
	st := stateOfCombatant(c, d.enc.Round)
	for _, h := range d.allHolds {
		if h.CombatantID != c.ID || derefInt32(h.Round) != d.enc.Round || h.stale(st) {
			continue
		}
		return combatantOffer(askedBy(h, st), c, d.cs)
	}
	return nil
}

// AnswerHeldRoll implements playv1connect.ResourceServiceHandler.
func (s *Service) AnswerHeldRoll(
	ctx context.Context,
	req *connect.Request[playv1.AnswerHeldRollRequest],
) (*connect.Response[playv1.AnswerHeldRollResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	key, err := parseKey(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	holdID, err := parseCombatID(req.Msg.GetHoldId(), "held roll")
	if err != nil {
		return nil, err
	}
	ans, err := answerOfHeldRoll(req.Msg)
	if err != nil {
		return nil, err
	}
	ans.ref, ans.key, ans.concealed = holdRef{Campaign: m.CampaignID, ID: holdID}, key, true
	out, err := s.answerHeld(ctx, m, ans)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(answerHeldResultOf(out)), nil
}

// answerOfHeldRoll reads the answer of an AnswerHeldRoll: the question it answers, the yes or no, and how the new die comes.
func answerOfHeldRoll(msg *playv1.AnswerHeldRollRequest) (answerRequest, error) {
	var ans answerRequest
	var p rollPick
	switch a := msg.GetAnswer().(type) {
	case *playv1.AnswerHeldRollRequest_Lucky:
		ans.question, ans.yes = askLucky, a.Lucky.GetReroll()
		p = rollPickOf(a.Lucky.GetRoll())
	case *playv1.AnswerHeldRollRequest_Die:
		ans.question, ans.yes = askDie, a.Die.GetUse()
		p = rollPickOf(a.Die.GetRoll())
	case *playv1.AnswerHeldRollRequest_Indomitable:
		ans.question, ans.yes = askIndomitable, a.Indomitable.GetReroll()
		p = rollPickOf(a.Indomitable.GetRoll())
		p.faces = a.Indomitable.GetTypedFaces()
	case *playv1.AnswerHeldRollRequest_StrokeOfLuck:
		ans.question, ans.yes = askStroke, a.StrokeOfLuck.GetUse()
		return ans, nil // nothing is rolled: no way of rolling to read
	default:
		return ans, connect.NewError(connect.CodeInvalidArgument, errors.New("answer must be set to a lucky, a die, an indomitable or a stroke of luck answer"))
	}
	if !ans.yes {
		return ans, nil
	}
	var err error
	ans.in, err = rollOfAnswer(p, ans.question)
	return ans, err
}

// rollPickOf reads the oneof a Lucky or a die answer carries for the new die.
func rollPickOf(roll any) rollPick {
	var p rollPick
	switch r := roll.(type) {
	case *playv1.LuckyAnswer_RollInApp:
		p.app = &r.RollInApp
	case *playv1.LuckyAnswer_TypedFace:
		p.face = &r.TypedFace
	case *playv1.DieAnswer_RollInApp:
		p.app = &r.RollInApp
	case *playv1.DieAnswer_TypedFace:
		p.face = &r.TypedFace
	case *playv1.IndomitableAnswer_RollInApp:
		p.app = &r.RollInApp
	case *playv1.IndomitableAnswer_TypedFace:
		p.face = &r.TypedFace
	}
	return p
}

// answerHeldResultOf is the response of the path that was run again, in the arm of the oneof it belongs to, with the
// next question of the same roll copied out of it.
func answerHeldResultOf(out proto.Message) *playv1.AnswerHeldRollResponse {
	res := &playv1.AnswerHeldRollResponse{}
	switch o := out.(type) {
	case *playv1.RollAttackResponse:
		res.Result, res.Next = &playv1.AnswerHeldRollResponse_Attack{Attack: o}, o.GetHeldRollOffer()
	case *playv1.CastSpellResponse:
		res.Result, res.Next = &playv1.AnswerHeldRollResponse_SpellCast{SpellCast: o}, o.GetHeldRollOffer()
	case *playv1.RollEffectSaveResponse:
		res.Result, res.Next = &playv1.AnswerHeldRollResponse_EffectSave{EffectSave: o}, o.GetHeldRollOffer()
	case *playv1.ResolveConcentrationSaveResponse:
		res.Result, res.Next = &playv1.AnswerHeldRollResponse_ConcentrationSave{ConcentrationSave: o}, o.GetHeldRollOffer()
	case *playv1.RollSceneCheckResponse:
		res.Result, res.Next = &playv1.AnswerHeldRollResponse_SceneCheck{SceneCheck: o}, o.GetHeldRollOffer()
	case *playv1.RollGroupCheckResponse:
		res.Result, res.Next = &playv1.AnswerHeldRollResponse_GroupCheck{GroupCheck: o}, o.GetHeldRollOffer()
	case *playv1.RollForPlayerResponse: // the master's roll for a player is a group check's, in the same arm
		g := &playv1.RollGroupCheckResponse{GroupCheck: o.GetGroupCheck(), InspirationOffer: o.GetInspirationOffer(), LuckyOffer: o.GetLuckyOffer(), HeldRollOffer: o.GetHeldRollOffer()}
		res.Result, res.Next = &playv1.AnswerHeldRollResponse_GroupCheck{GroupCheck: g}, g.GetHeldRollOffer()
	default:
		answerHeldCheckResultOf(out, res)
	}
	return res
}

// answerHeldCheckResultOf fills res for the checks outside the attack and save family: either side of a contest, Hide,
// the Counterspell check, a death save, an initiative, a trap search and a puzzle hint.
func answerHeldCheckResultOf(out proto.Message, res *playv1.AnswerHeldRollResponse) {
	switch o := out.(type) {
	case *playv1.StartContestResponse:
		res.Result, res.Next = &playv1.AnswerHeldRollResponse_Contest{Contest: o}, o.GetHeldRollOffer()
	case *playv1.RespondContestResponse:
		res.Result, res.Next = &playv1.AnswerHeldRollResponse_ContestAnswer{ContestAnswer: o}, o.GetHeldRollOffer()
	case *playv1.HideResponse:
		res.Result, res.Next = &playv1.AnswerHeldRollResponse_Hide{Hide: o}, o.GetHeldRollOffer()
	case *playv1.AnswerReactionResponse:
		res.Result, res.Next = &playv1.AnswerHeldRollResponse_CounterspellCheck{CounterspellCheck: o}, o.GetHeldRollOffer()
	case *playv1.RollDeathSaveResponse:
		res.Result, res.Next = &playv1.AnswerHeldRollResponse_DeathSave{DeathSave: o}, o.GetHeldRollOffer()
	case *playv1.SubmitInitiativeResponse:
		res.Result, res.Next = &playv1.AnswerHeldRollResponse_Initiative{Initiative: o}, o.GetHeldRollOffer()
	case *playv1.SearchForTrapsResponse:
		res.Result, res.Next = &playv1.AnswerHeldRollResponse_TrapSearch{TrapSearch: o}, o.GetHeldRollOffer()
	case *playv1.TryPuzzleHintResponse:
		res.Result, res.Next = &playv1.AnswerHeldRollResponse_HintTry{HintTry: o}, o.GetHeldRollOffer()
	}
}

// ListHeldRolls implements playv1connect.ResourceServiceHandler.
func (s *Service) ListHeldRolls(
	ctx context.Context,
	req *connect.Request[playv1.ListHeldRollsRequest],
) (*connect.Response[playv1.ListHeldRollsResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	holds, err := s.openHeldRolls(ctx, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "list the held rolls", err)
	}
	list := heldList{s: s, m: m, combats: map[string]*listedCombat{}}
	res := &playv1.ListHeldRollsResponse{}
	for _, h := range holds {
		offer, err := list.offerOf(ctx, h)
		if err != nil {
			return nil, err
		}
		if offer != nil {
			res.Offers = append(res.Offers, offer)
		}
	}
	return connect.NewResponse(res), nil
}

// openHeldRolls are the rolls of the campaign that wait for an answer, the oldest first.
func (s *Service) openHeldRolls(ctx context.Context, campaignID string) ([]heldRoll, error) {
	rows, err := s.queries.ListOpenInspirationHoldsOfCampaign(ctx, campaignID)
	if err != nil {
		return nil, err //nolint:wrapcheck // the caller wraps
	}
	out := make([]heldRoll, 0, len(rows))
	for _, row := range rows {
		out = append(out, heldOfInspirationHold(row))
	}
	return out, nil
}

// listedCombat is a combat the list has read: its row and its combatants.
type listedCombat struct {
	enc playdb.Encounter
	cs  []playdb.Combatant
}

// heldList builds the questions of ListHeldRolls for one caller.
type heldList struct {
	s       *Service
	m       authz.Membership
	combats map[string]*listedCombat
}

// offerOf is the question of a hold if the caller may answer it and it is not stale; nil otherwise.
func (l *heldList) offerOf(ctx context.Context, h heldRoll) (*playv1.HeldRollOffer, error) {
	if h.CombatantID != "" {
		return l.combatOffer(ctx, h)
	}
	if l.m.Role != authz.RoleMaster {
		mine, err := l.s.isMyCharacter(ctx, l.m, h.CharacterID)
		if err != nil || !mine {
			return nil, err
		}
	}
	die, found, err := l.s.dieOf(ctx, l.s.queries, h.CampaignID, h.CharacterID)
	if err != nil {
		return nil, l.s.dbError(ctx, "find the die", err)
	}
	st := rollerState{Die: die, Holds: found}
	asked := askedBy(h, st)
	if h.stale(st) || (asked.Asks != askLucky && asked.Asks != askStroke && !found) {
		return nil, nil
	}
	return l.s.outsideHeldRollOffer(ctx, nil, asked), nil
}

// combatOffer is offerOf for a hold made by a combatant.
func (l *heldList) combatOffer(ctx context.Context, h heldRoll) (*playv1.HeldRollOffer, error) {
	c, err := l.combatOf(ctx, h.EncounterID)
	if err != nil || c == nil || c.enc.Status != statusActive {
		return nil, err
	}
	i := slices.IndexFunc(c.cs, func(o playdb.Combatant) bool { return o.ID == h.CombatantID })
	if i < 0 {
		return nil, nil
	}
	who := c.cs[i]
	if !viewerOf(l.m).master && !viewerOf(l.m).owns(who) {
		return nil, nil
	}
	st := stateOfCombatant(who, c.enc.Round)
	if derefInt32(h.Round) != c.enc.Round || h.stale(st) {
		return nil, nil
	}
	return combatantOffer(askedBy(h, st), who, c.cs), nil
}

// combatOf reads a combat once.
func (l *heldList) combatOf(ctx context.Context, encounterID string) (*listedCombat, error) {
	if c, ok := l.combats[encounterID]; ok {
		return c, nil
	}
	enc, err := l.s.queries.GetEncounterByID(ctx, encounterID)
	if err != nil {
		return nil, l.s.dbError(ctx, "find the combat", err)
	}
	cs, err := l.s.queries.ListCombatants(ctx, encounterID)
	if err != nil {
		return nil, l.s.dbError(ctx, "list the combatants", err)
	}
	l.combats[encounterID] = &listedCombat{enc: enc, cs: cs}
	return l.combats[encounterID], nil
}

// heldRollProcedures are the paths that answer with the question of a roll that waits.
func heldHintInterceptor(s *Service) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			res, err := next(ctx, req)
			if err != nil || res == nil {
				return res, err
			}
			asked, ok := res.Any().(interface{ GetHeldRollOffer() *playv1.HeldRollOffer })
			if !ok || asked.GetHeldRollOffer() == nil || !strings.HasPrefix(req.Spec().Procedure, "/meurpg.play.v1.") {
				return res, err
			}
			if userID, uerr := authz.RequireSignedIn(ctx); uerr == nil {
				s.publishHeldRollsChanged(campaignOf(req.Any()), userID)
			}
			return res, err
		}
	}
}

// campaignOf is the campaign a request names.
func campaignOf(msg any) string {
	c, _ := msg.(interface{ GetCampaignId() string })
	if c == nil {
		return ""
	}
	return c.GetCampaignId()
}

// publishHeldRollsChanged tells the master and the given users that the questions of the held rolls changed: a hint
// with no content, read again by each as their own role (RN-10).
func (s *Service) publishHeldRollsChanged(campaignID string, userIDs ...string) {
	if campaignID == "" {
		return
	}
	msg := &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_HeldRollsChanged_{
		HeldRollsChanged: &playv1.WatchGameSessionResponse_HeldRollsChanged{},
	}}
	s.hub.Publish(campaignID, live.Event{Audience: live.Audience{Master: true}, Message: msg})
	for _, id := range slices.Compact(slices.Sorted(slices.Values(userIDs))) {
		if id != "" {
			s.hub.Publish(campaignID, live.Event{Audience: live.Audience{UserID: id}, Message: msg})
		}
	}
}
