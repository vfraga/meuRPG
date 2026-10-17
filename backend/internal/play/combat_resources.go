package play

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// The four class resource flows that move more than a use: Lay on Hands, Flexible
// Casting (creating a slot, converting one) and Bardic Inspiration (SRD 5.1,
// Paladin 1, Sorcerer 2, Bard 1). Each is an action of the turn, so it goes through
// the combat's write (one transaction, the idempotency key, the event, the fog), writes
// an `action_taken` event with a resourceEvent in it, and the undo takes it back.

// The feature actions the flows are taken as: the options list them, and TakeAction
// refuses them, because they need more than a use (a target, a number of points).
const (
	layOnHandsAction    = "feature:lay-on-hands"
	flexibleCreate      = "feature:flexible-casting-creating-spell-slots"
	flexibleConvert     = "feature:flexible-casting-converting-spell-slot"
	bardicInspirationFt = "feature:bardic-inspiration-d6"
)

// resourceFlowActions are the feature actions that have a flow of their own.
var resourceFlowActions = []string{layOnHandsAction, flexibleCreate, flexibleConvert, bardicInspirationFt}

// What a resource event did (resourceEvent.Kind).
const (
	resLayOnHands = "lay_on_hands"
	resCreateSlot = "create_slot"
	resConvert    = "convert_slot"
	resBardicGive = "bardic_give"
	resBardicUse  = "bardic_use"
)

// resourceEvent is what a class resource flow did, as an action_taken event keeps it:
// numbers and keys only. The `Before` fields are what the undo puts back.
type resourceEvent struct {
	Kind string `json:"kind"`
	// Spent is the points (or uses) taken from the resource, Healed the hit points the
	// target regained (Lay on Hands), Cure the cure ("disease", "poison"), SlotLevel the
	// slot created or converted, Gained the sorcery points a conversion gave.
	Spent     int32  `json:"spent,omitempty"`
	Healed    int32  `json:"healed,omitempty"`
	Cure      string `json:"cure,omitempty"`
	SlotLevel int32  `json:"slot_level,omitempty"`
	Gained    int32  `json:"gained,omitempty"`
	// Nothing says the touch did nothing, and Why is the master's alone: "undead",
	// "construct" or "no_poison".
	Nothing bool   `json:"nothing,omitempty"`
	Why     string `json:"why,omitempty"`
	// What the target had before, for the undo: hit points, death saves, conditions.
	HPBefore    *hpState    `json:"hp_before,omitempty"`
	DeathBefore *deathState `json:"death_before,omitempty"`
	CondSet     bool        `json:"cond_set,omitempty"`
	CondBefore  []string    `json:"cond_before,omitempty"`
	// Bardic Inspiration: the die given and the round it runs out in.
	Sides        int32 `json:"sides,omitempty"`
	ExpiresRound int32 `json:"expires_round,omitempty"`
	// Keys are the Metamagic options a casting used (resMetamagic).
	Keys []string `json:"keys,omitempty"`
	// Face is the face of the Bardic Inspiration die added to an attack roll (resBardicUse),
	// with Sides, the bard that gave it (FromID) and the round it would have run out in.
	Face   int32  `json:"face,omitempty"`
	FromID string `json:"from_id,omitempty"`
}

// resourceFlow is what the closure of a flow works with after the action is spent.
type resourceFlow struct {
	m    authz.Membership
	v    combatViewer
	cs   []playdb.Combatant
	who  playdb.Combatant
	made actionEvent
}

// startFlow does what every flow begins with inside the change: the combatant is the
// caller's, it is its turn, the feature action is one it has and may take (the master
// is held to nothing of the economy), and the action is spent. It returns the event
// to fill in.
func (s *Service) startFlow(ctx context.Context, c *combatTx, m authz.Membership, actorID, actionKey string) (*resourceFlow, error) {
	cs, err := c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return nil, fmt.Errorf("list the combatants: %w", err)
	}
	v := c.viewer(m, cs) // the fog: an NPC the player does not see is not found
	who, err := findCombatant(cs, actorID, v)
	if err != nil {
		return nil, err
	}
	if err := v.mayAct(who); err != nil {
		return nil, err
	}
	if who.Kind != kindPlayer {
		// The resources of an NPC are not counted: the master adjusts what it did.
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("this is for a player's character; the master adjusts an NPC by hand"))
	}
	opts, err := s.optionsOf(ctx, c.tx, m.CampaignID, who)
	if err != nil {
		return nil, err
	}
	i := slices.IndexFunc(opts.GetFeatureActions(), func(a *rulesv1.ActionOption) bool { return a.GetAction().GetKey() == actionKey })
	if i < 0 {
		return nil, resourceBlocked(playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_NOT_AVAILABLE, "the character does not have this feature")
	}
	action := opts.GetFeatureActions()[i]
	if err := s.mustActNow(ctx, c, who); err != nil {
		return nil, err
	}
	if !action.GetEnabled() {
		if err := featureError(action.GetReason(), v.master); err != nil {
			return nil, err
		}
	}
	run, err := breakRun(ctx, c, who)
	if err != nil {
		return nil, err
	}
	made := actionEvent{
		Round: c.enc.Round, Secret: who.Hidden, Actor: who.ID, Key: actionKey, RunBefore: run,
		ActionBefore: who.ActionUsed, BonusBefore: who.BonusActionUsed, ReactionBefore: who.ReactionUsed, DashedBefore: who.Dashed,
		AttacksBefore: who.AttacksMade, DisengagedBefore: who.Disengaged, SurgedBefore: who.ActionSurged,
		AttackKeyBefore: deref(who.ActionAttackKey), FlurryBefore: who.BonusAttacksLeft,
	}
	after := who
	switch action.GetAction().GetEconomy() {
	case rulesv1.ActionEconomy_ACTION_ECONOMY_BONUS_ACTION:
		after.BonusActionUsed = true
	case rulesv1.ActionEconomy_ACTION_ECONOMY_REACTION:
		after.ReactionUsed = true
	case rulesv1.ActionEconomy_ACTION_ECONOMY_FREE, rulesv1.ActionEconomy_ACTION_ECONOMY_MOVEMENT:
		// costs no economy
	default:
		after.ActionUsed = true
	}
	if err := c.q.SetCombatantEconomy(ctx, playdb.SetCombatantEconomyParams{
		ID: who.ID, ActionUsed: after.ActionUsed, BonusActionUsed: after.BonusActionUsed, ReactionUsed: after.ReactionUsed, Dashed: after.Dashed,
	}); err != nil {
		return nil, fmt.Errorf("spend the action: %w", err)
	}
	c.characterID = &who.CharacterID
	return &resourceFlow{m: m, v: v, cs: cs, who: who, made: made}, nil
}

// finishFlow touches the combat's revision, which every change does.
func (s *Service) finishFlow(ctx context.Context, c *combatTx, f *resourceFlow) (actionEvent, error) {
	var err error
	if c.enc, err = c.q.TouchEncounter(ctx, c.enc.ID); err != nil {
		return actionEvent{}, fmt.Errorf("touch the encounter: %w", err)
	}
	return f.made, nil
}

// flowRequest is what every flow reads from its request.
type flowRequest struct {
	m                        authz.Membership
	key                      string
	encID, actorID, targetID string
}

func (s *Service) flowRequest(ctx context.Context, campaignID, encounterID, actorID, targetID, rawKey string) (flowRequest, error) {
	m, err := authz.RequireCampaignMember(ctx, campaignID)
	if err != nil {
		return flowRequest{}, err
	}
	key, err := parseKey(rawKey)
	if err != nil {
		return flowRequest{}, err
	}
	encID, err := parseCombatID(encounterID, "encounter")
	if err != nil {
		return flowRequest{}, err
	}
	actor, err := parseCombatID(actorID, "combatant")
	if err != nil {
		return flowRequest{}, err
	}
	out := flowRequest{m: m, key: key, encID: encID, actorID: actor}
	if targetID != "" {
		if out.targetID, err = parseCombatID(targetID, "combatant"); err != nil {
			return flowRequest{}, err
		}
	}
	return out, nil
}

// touchReach checks that a player's target is within the touch's reach, as a melee
// attack does (RN-21): on a map both stand on a square and the distance is at most 5 ft;
// without a map (the theatre of the mind) the master judges, and the master is held to
// nothing.
func touchReach(c *combatTx, v combatViewer, who, target playdb.Combatant, reachFt int32) error {
	if v.master || isTheatre(c.enc) {
		return nil
	}
	if !placed(who) || !placed(target) {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_PLACED, "the combatant and the target must be on the map")
	}
	if dist, _ := distanceFt(who, target); dist > reachFt {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TARGET_OUT_OF_REACH, "the target is beyond the reach",
			func(b *playv1.EncounterBlocked) { b.MissingFt = dist - reachFt })
	}
	return nil
}

// creatureTypeOf is the SRD type ("undead") of a monster combatant, "" for anyone else.
func (s *Service) creatureTypeOf(ctx context.Context, c *combatTx, target playdb.Combatant) (string, error) {
	key := deref(target.MonsterKey)
	if key == "" && target.Kind == kindNPC {
		// A monster the master added is an NPC made from a creature: its sheet knows which.
		chars, err := s.roster.CombatCharacters(ctx, c.tx, c.session.CampaignID, []string{target.CharacterID})
		if err != nil {
			return "", err
		}
		if len(chars) > 0 {
			key = chars[0].MonsterKey
		}
	}
	if key == "" {
		return "", nil
	}
	content, err := s.roster.RulesContent(ctx, c.tx, c.session.CampaignID)
	if err != nil {
		return "", err
	}
	creature, ok := content.CreatureByKey(key)
	if !ok {
		return "", nil
	}
	return creature.Type, nil
}

// poolLeft is the points a resource has left in a character's vitals, and its key's usage.
func poolLeft(v *playv1.CharacterVitals, key string) (left int32, found bool) {
	for _, r := range v.GetResources() {
		if r.GetKey() == key {
			return r.GetTotal() - r.GetUsed(), true
		}
	}
	return 0, false
}

// layCall is what UseLayOnHands reads from its request before the change.
type layCall struct {
	fr     flowRequest
	hash   *string
	amount int32
	cure   string
}

// layRun is what the change of UseLayOnHands leaves for after the commit.
type layRun struct {
	made actionEvent
	told []*playv1.CharacterVitals
	v    combatViewer
}

// readLayCall reads and checks the request, in the order the refusals have always come.
func (s *Service) readLayCall(ctx context.Context, req *playv1.UseLayOnHandsRequest) (layCall, error) {
	fr, err := s.flowRequest(ctx, req.GetCampaignId(), req.GetEncounterId(), req.GetActorId(), req.GetTargetId(), req.GetIdempotencyKey())
	if err != nil {
		return layCall{}, err
	}
	if fr.targetID == "" {
		return layCall{}, connect.NewError(connect.CodeNotFound, errors.New("combatant not found"))
	}
	amount, cure, err := layOnHandsEffect(req)
	if err != nil {
		return layCall{}, err
	}
	return layCall{fr: fr, hash: idem.Hash(req), amount: amount, cure: cure}, nil
}

// UseLayOnHands implements playv1connect.ResourceServiceHandler.
func (s *Service) UseLayOnHands(
	ctx context.Context,
	req *connect.Request[playv1.UseLayOnHandsRequest],
) (*connect.Response[playv1.UseLayOnHandsResponse], error) {
	call, err := s.readLayCall(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	m := call.fr.m
	run := layRun{v: viewerOf(m)}
	res, err := s.write(ctx, combatWrite{m: m, key: call.fr.key, hash: call.hash, kind: eventActionTaken, encounterID: call.fr.encID}, func(c *combatTx) (any, error) {
		run.told = nil
		return s.layOnHandsIn(ctx, c, call, &run)
	})
	if err != nil {
		return nil, s.dbError(ctx, "use lay on hands", err)
	}
	ev, err := resultEvent(res, run.made)
	if err != nil {
		return nil, s.dbError(ctx, "read the lay on hands", err)
	}
	if run.v, err = s.viewerAfter(ctx, m, res, run.v); err != nil {
		return nil, s.dbError(ctx, "work out what the player sees", err)
	}
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, !ev.Secret)
		for _, vit := range run.told {
			s.publishVitals(m.CampaignID, vit)
		}
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(s.layOnHandsAnswer(ctx, m, out, ev, run.v)), nil
}

// layOnHandsAnswer is the answer of a touch as the caller may read it: the healed amount is the
// master's and the target's player's, and the pool is what the character's own vitals say now.
func (s *Service) layOnHandsAnswer(ctx context.Context, m authz.Membership, out *playv1.Encounter, ev actionEvent, v combatViewer) *playv1.UseLayOnHandsResponse {
	resp := &playv1.UseLayOnHandsResponse{Encounter: out}
	if ev.Res != nil {
		resp.Spent, resp.NothingHappened = ev.Res.Spent, ev.Res.Nothing
		if target, ok := findInEncounter(out, ev.Target); ok && (v.master || target.GetMine()) {
			healed := ev.Res.Healed
			resp.Healed = &healed
		}
	}
	// What is left in the pool, from the character's own vitals now.
	if cur, err := s.vitals.GetVitals(ctx, m.CampaignID, characterOf(out, ev.Actor)); err == nil {
		resp.PoolLeft, _ = poolLeft(cur, rules.LayOnHandsKey)
	}
	return resp
}

// layOnHandsIn is the change of UseLayOnHands: the flow, the target in reach, the points
// taken from the pool, the touch's effect.
func (s *Service) layOnHandsIn(ctx context.Context, c *combatTx, call layCall, run *layRun) (any, error) {
	m := call.fr.m
	f, err := s.startFlow(ctx, c, m, call.fr.actorID, layOnHandsAction)
	if err != nil {
		return nil, err
	}
	run.v = f.v
	target, err := findCombatant(f.cs, call.fr.targetID, f.v)
	if err != nil {
		return nil, err
	}
	if err := touchReach(c, f.v, f.who, target, meleeReachFt); err != nil {
		return nil, err
	}
	spent, err := s.layOnHandsCost(ctx, c, f.who.CharacterID, call)
	if err != nil {
		return nil, err
	}
	after, err := s.spendResource(ctx, c, f.who.CharacterID, rules.LayOnHandsKey, spent)
	if err != nil {
		return nil, err
	}
	run.told = append(run.told, after)
	made := f.made
	made.Target, made.Secret = target.ID, f.made.Secret || target.Hidden
	res := &resourceEvent{Kind: resLayOnHands, Spent: spent, Cure: call.cure}
	made.Res = res
	run.made = made
	more, err := s.layOnHandsEffect(ctx, c, target, res, call.amount, call.cure)
	if err != nil {
		return nil, err
	}
	run.told = append(run.told, more...)
	run.made.Heal = false
	return s.finishFlow(ctx, c, &resourceFlow{made: run.made})
}

// layOnHandsCost is the points the touch takes from the pool: a cure costs a fixed number,
// a heal what was drawn, which the pool must hold.
func (s *Service) layOnHandsCost(ctx context.Context, c *combatTx, characterID string, call layCall) (int32, error) {
	pool, err := s.vitals.GetVitalsTx(ctx, c.tx, call.fr.m.CampaignID, characterID)
	if err != nil {
		return 0, err
	}
	left, _ := poolLeft(pool, rules.LayOnHandsKey)
	if call.cure == "" {
		if err := rules.LayOnHandsAmount(int(call.amount), int(left)); err != nil {
			return 0, connect.NewError(connect.CodeInvalidArgument, errors.New("amount must be from 1 to what the pool has left"))
		}
		return call.amount, nil
	}
	var spent int32 = rules.LayOnHandsCureCost
	if left < spent {
		return 0, resourceBlocked(playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_NOT_ENOUGH_POINTS, "there are not enough points in the pool",
			func(b *playv1.ResourceBlocked) { b.Needed, b.Available = spent, left })
	}
	return spent, nil
}

// layOnHandsEffect reads what the request asks of the touch: an amount of points to heal, or
// the cure of a disease or a poison (which costs a fixed number of points).
func layOnHandsEffect(req *playv1.UseLayOnHandsRequest) (amount int32, cure string, err error) {
	switch e := req.GetEffect().(type) {
	case *playv1.UseLayOnHandsRequest_Amount:
		return e.Amount, "", nil
	case *playv1.UseLayOnHandsRequest_Cure:
		switch e.Cure {
		case playv1.LayOnHandsCure_LAY_ON_HANDS_CURE_DISEASE:
			return 0, cureDisease, nil
		case playv1.LayOnHandsCure_LAY_ON_HANDS_CURE_POISON:
			return 0, curePoison, nil
		default:
			return 0, "", connect.NewError(connect.CodeInvalidArgument, errors.New("cure must be a disease or a poison"))
		}
	default:
		return 0, "", connect.NewError(connect.CodeInvalidArgument, errors.New("set amount or cure"))
	}
}

// layOnHandsEffect does what the touch does to the target and fills the event; it returns
// the vitals that changed, to tell the stream after the commit.
func (s *Service) layOnHandsEffect(ctx context.Context, c *combatTx, target playdb.Combatant, res *resourceEvent, amount int32, cure string) ([]*playv1.CharacterVitals, error) {
	typ, err := s.creatureTypeOf(ctx, c, target)
	if err != nil {
		return nil, err
	}
	switch {
	case rules.LayOnHandsNoEffect(typ):
		// "This feature has no effect on undead and constructs": the action and the points
		// are spent, and nothing is said of why.
		res.Nothing, res.Why = true, typ
	case cure == curePoison:
		if !slices.Contains(target.Conditions, conditionPoisoned) {
			res.Nothing, res.Why = true, "no_poison"
			break
		}
		res.CondSet, res.CondBefore = true, slices.Clone(target.Conditions)
		if err := c.q.SetCombatantConditions(ctx, playdb.SetCombatantConditionsParams{
			ID: target.ID, Conditions: nonNil(slices.DeleteFunc(slices.Clone(target.Conditions), func(k string) bool { return k == conditionPoisoned })),
		}); err != nil {
			return nil, fmt.Errorf("neutralize the poison: %w", err)
		}
	case cure == cureDisease:
		// The app tracks no disease: the master gives the effect. The line says it.
	default:
		hit, vit, err := s.healCombatant(ctx, c, target, amount)
		if err != nil {
			return nil, err
		}
		res.Healed, res.HPBefore, res.DeathBefore = hit.Amount, hit.Before, hit.DeathBefore
		if hit.Before != nil && !holdsHP(target) {
			res.DeathBefore = deathOf(target)
		}
		if vit != nil {
			return []*playv1.CharacterVitals{vit}, nil
		}
	}
	return nil, nil
}

// The cures of Lay on Hands, as an event keeps them, and the condition a poison is.
const (
	cureDisease       = "disease"
	curePoison        = "poison"
	conditionPoisoned = "condition:poisoned"
)

// findInEncounter finds a combatant in the combat as the caller sees it.
func findInEncounter(e *playv1.Encounter, id string) (*playv1.Combatant, bool) {
	for _, c := range e.GetCombatants() {
		if c.GetId() == id {
			return c, true
		}
	}
	return nil, false
}

// characterOf is the character a combatant of the combat stands for, "" when the caller
// does not see it (a player gets the character of a player's own combatants and the
// party's).
func characterOf(e *playv1.Encounter, combatantID string) string {
	if c, ok := findInEncounter(e, combatantID); ok {
		return c.GetCharacterId()
	}
	return ""
}

// CreateSpellSlot implements playv1connect.ResourceServiceHandler: Flexible Casting's
// "Criar espaço".
func (s *Service) CreateSpellSlot(
	ctx context.Context,
	req *connect.Request[playv1.CreateSpellSlotRequest],
) (*connect.Response[playv1.CreateSpellSlotResponse], error) {
	fr, err := s.flowRequest(ctx, req.Msg.GetCampaignId(), req.Msg.GetEncounterId(), req.Msg.GetActorId(), "", req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	level := int(req.Msg.GetSlotLevel())
	if level < 1 || level > maxSlotLevel {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("slot_level must be 1 to 9"))
	}
	enc, vit, cost, err := s.flexible(ctx, fr, idem.Hash(req.Msg), resCreateSlot, level)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.CreateSpellSlotResponse{Encounter: enc, Vitals: vit, Cost: cost}), nil
}

// ConvertSpellSlot implements playv1connect.ResourceServiceHandler: Flexible Casting's
// "Converter espaço".
func (s *Service) ConvertSpellSlot(
	ctx context.Context,
	req *connect.Request[playv1.ConvertSpellSlotRequest],
) (*connect.Response[playv1.ConvertSpellSlotResponse], error) {
	fr, err := s.flowRequest(ctx, req.Msg.GetCampaignId(), req.Msg.GetEncounterId(), req.Msg.GetActorId(), "", req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	level := int(req.Msg.GetSlotLevel())
	if level < 1 || level > maxSlotLevel {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("slot_level must be 1 to 9"))
	}
	enc, vit, gain, err := s.flexible(ctx, fr, idem.Hash(req.Msg), resConvert, level)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.ConvertSpellSlotResponse{Encounter: enc, Vitals: vit, Gain: gain}), nil
}

// maxSlotLevel is the highest spell level.
const maxSlotLevel = 9

// flexible is Flexible Casting, both ways: a bonus action of the sorcerer's turn that
// creates a slot from sorcery points (kind resCreateSlot) or turns a free slot into
// points (resConvert). It returns the combat, the sorcerer's vitals after and the points
// the slot cost or gave.
func (s *Service) flexible(ctx context.Context, fr flowRequest, hash *string, kind string, level int) (*playv1.Encounter, *playv1.CharacterVitals, int32, error) {
	m := fr.m
	var made actionEvent
	var after *playv1.CharacterVitals
	res, err := s.write(ctx, combatWrite{m: m, key: fr.key, hash: hash, kind: eventActionTaken, encounterID: fr.encID}, func(c *combatTx) (any, error) {
		after = nil
		var err error
		if made, after, err = s.flexibleIn(ctx, c, fr, kind, level); err != nil {
			return nil, err
		}
		return made, nil
	})
	if err != nil {
		return nil, nil, 0, s.dbError(ctx, "use flexible casting", err)
	}
	ev, err := resultEvent(res, made)
	if err != nil {
		return nil, nil, 0, s.dbError(ctx, "read flexible casting", err)
	}
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, !ev.Secret)
		s.publishVitals(m.CampaignID, after)
	})
	if err != nil {
		return nil, nil, 0, err
	}
	if after == nil { // a retry: the vitals as they are now
		if cur, err := s.vitals.GetVitals(ctx, m.CampaignID, characterOf(out, ev.Actor)); err == nil {
			after = cur
		}
	}
	points := int32(0)
	if ev.Res != nil {
		points = ev.Res.Spent + ev.Res.Gained
	}
	return out, after, points, nil
}

// flexibleIn is the change of Flexible Casting: the flow of the feature action, the keeper's
// slot, the event. It returns the event and the sorcerer's vitals after.
func (s *Service) flexibleIn(ctx context.Context, c *combatTx, fr flowRequest, kind string, level int) (actionEvent, *playv1.CharacterVitals, error) {
	action := flexibleCreate
	if kind == resConvert {
		action = flexibleConvert
	}
	f, err := s.startFlow(ctx, c, fr.m, fr.actorID, action)
	if err != nil {
		return actionEvent{}, nil, err
	}
	var after *playv1.CharacterVitals
	var points int
	if kind == resCreateSlot {
		_, after, points, err = s.vitals.CreateSpellSlot(ctx, c.tx, fr.m.CampaignID, f.who.CharacterID, level)
	} else {
		_, after, points, err = s.vitals.ConvertSpellSlot(ctx, c.tx, fr.m.CampaignID, f.who.CharacterID, level)
	}
	if err != nil {
		return actionEvent{}, after, resourceError(err)
	}
	f.made.Res = &resourceEvent{Kind: kind, SlotLevel: clamp32(level, 0, maxSlotLevel)}
	if kind == resCreateSlot {
		f.made.Res.Spent = clamp32(points, 0, math.MaxInt32)
	} else {
		f.made.Res.Gained = clamp32(points, 0, math.MaxInt32)
	}
	made, err := s.finishFlow(ctx, c, f)
	return made, after, err
}

// GiveBardicInspiration implements playv1connect.ResourceServiceHandler.
func (s *Service) GiveBardicInspiration(
	ctx context.Context,
	req *connect.Request[playv1.GiveBardicInspirationRequest],
) (*connect.Response[playv1.GiveBardicInspirationResponse], error) {
	fr, err := s.flowRequest(ctx, req.Msg.GetCampaignId(), req.Msg.GetEncounterId(), req.Msg.GetActorId(), req.Msg.GetTargetId(), req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	if fr.targetID == "" {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("combatant not found"))
	}
	m := fr.m
	var made actionEvent
	var after *playv1.CharacterVitals
	res, err := s.write(ctx, combatWrite{m: m, key: fr.key, hash: idem.Hash(req.Msg), kind: eventActionTaken, encounterID: fr.encID}, func(c *combatTx) (any, error) {
		after = nil
		var err error
		if made, after, err = s.bardicGiveIn(ctx, c, fr); err != nil {
			return nil, err
		}
		return made, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "give bardic inspiration", err)
	}
	ev, err := resultEvent(res, made)
	if err != nil {
		return nil, s.dbError(ctx, "read bardic inspiration", err)
	}
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, !ev.Secret)
		s.publishVitals(m.CampaignID, after)
	})
	if err != nil {
		return nil, err
	}
	if after == nil {
		if cur, err := s.vitals.GetVitals(ctx, m.CampaignID, characterOf(out, ev.Actor)); err == nil {
			after = cur
		}
	}
	return connect.NewResponse(&playv1.GiveBardicInspirationResponse{Encounter: out, Vitals: after}), nil
}

// bardicGiveIn is the change of GiveBardicInspiration: the flow, a fit target, the use spent,
// the die given. It returns the event and the bard's vitals after.
func (s *Service) bardicGiveIn(ctx context.Context, c *combatTx, fr flowRequest) (actionEvent, *playv1.CharacterVitals, error) {
	f, err := s.startFlow(ctx, c, fr.m, fr.actorID, bardicInspirationFt)
	if err != nil {
		return actionEvent{}, nil, err
	}
	target, dieSize, err := s.bardicTargetOf(ctx, c, f, fr)
	if err != nil {
		return actionEvent{}, nil, err
	}
	after, err := s.spendResource(ctx, c, f.who.CharacterID, rules.BardicInspirationKey, 1)
	if err != nil {
		return actionEvent{}, after, err
	}
	expires := clamp32(rules.BardicInspirationExpiry(int(c.enc.Round)), 1, math.MaxInt32)
	sides := clamp32(dieSize, 0, math.MaxInt32)
	if err := c.q.SetCombatantInspirationDie(ctx, playdb.SetCombatantInspirationDieParams{ID: target.ID, Sides: &sides, FromID: &f.who.ID, ExpiresRound: &expires}); err != nil {
		return actionEvent{}, after, fmt.Errorf("give the die: %w", err)
	}
	f.made.Target = target.ID
	f.made.Res = &resourceEvent{Kind: resBardicGive, Spent: 1, Sides: sides, ExpiresRound: expires}
	made, err := s.finishFlow(ctx, c, f)
	return made, after, err
}

// bardicTargetOf finds the creature the bard speaks to and checks that it can get a die and
// that the bard has the feature; it returns the target and the size of the bard's die.
func (s *Service) bardicTargetOf(ctx context.Context, c *combatTx, f *resourceFlow, fr flowRequest) (playdb.Combatant, int, error) {
	target, err := findCombatant(f.cs, fr.targetID, f.v)
	if err != nil {
		return target, 0, err
	}
	if target.Defeated {
		return target, 0, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TARGET_DEFEATED, "the target is defeated")
	}
	sheet, err := s.sheetOf(ctx, c.tx, fr.m.CampaignID, f.who)
	if err != nil {
		return target, 0, err
	}
	if sheet.BardicDie == 0 {
		return target, 0, resourceBlocked(playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_NOT_AVAILABLE, "the character does not have this feature")
	}
	if reason := s.inspirationRefusal(c, f, target); reason != "" {
		return target, 0, resourceBlocked(playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_TARGET_REFUSED, "the creature cannot get a die")
	}
	return target, sheet.BardicDie, nil
}

// holdsDie says the combatant holds a Bardic Inspiration die that has not run out
// in the round.
func holdsDie(c playdb.Combatant, round int32) bool {
	return c.InspirationSides != nil && c.InspirationExpiresRound != nil && *c.InspirationExpiresRound >= round
}

// inspirationRefusal is why the target cannot be given a die, "" when it can: it is
// not the bard, it is within 60 feet (on a map), it can hear, and it holds no die.
func (s *Service) inspirationRefusal(c *combatTx, f *resourceFlow, target playdb.Combatant) string {
	t := rules.BardicInspirationTarget{
		IsBard:  target.ID == f.who.ID,
		OnMap:   !isTheatre(c.enc) && placed(f.who) && placed(target),
		CanHear: !slices.Contains(target.Conditions, "condition:deafened"),
		HasDie:  holdsDie(target, c.enc.Round),
	}
	if t.OnMap {
		t.Distance = int(distanceOf(f.who, target))
	}
	if !f.v.master && !isTheatre(c.enc) && (!placed(f.who) || !placed(target)) {
		return "Sem posição no mapa"
	}
	return rules.BardicInspirationRefusal(t)
}

// distanceOf is the distance in feet between two combatants that stand on squares.
func distanceOf(a, b playdb.Combatant) int32 {
	d, _ := distanceFt(a, b)
	return d
}

// takeBackResource puts back what a class resource flow did, for the master's undo
// (the economy comes back with the rest of the action): the points or the use it spent,
// the hit points and conditions of the target, the slot Flexible Casting made or used,
// the die the bard gave.
func (u *undoer) takeBackResource(ctx context.Context, ev actionEvent, who playdb.Combatant) error {
	campaign := u.c.session.CampaignID
	res := ev.Res
	switch res.Kind {
	case resLayOnHands:
		return u.layOnHandsBack(ctx, ev, who)
	case resCreateSlot:
		v, err := u.s.vitals.UndoCreateSpellSlot(ctx, u.c.tx, campaign, who.CharacterID, int(res.SlotLevel), int(res.Spent))
		if err != nil {
			return err
		}
		u.keep(v)
	case resConvert:
		v, err := u.s.vitals.UndoConvertSpellSlot(ctx, u.c.tx, campaign, who.CharacterID, int(res.SlotLevel), int(res.Gained))
		if err != nil {
			return err
		}
		u.keep(v)
	case resBardicGive:
		return u.bardicGiveBack(ctx, ev, who)
	}
	return nil
}

// layOnHandsBack gives back the points of Lay on Hands, the hit points and the
// conditions it changed on the target.
func (u *undoer) layOnHandsBack(ctx context.Context, ev actionEvent, who playdb.Combatant) error {
	res := ev.Res
	v, err := u.s.spendResource(ctx, u.c, who.CharacterID, rules.LayOnHandsKey, -res.Spent)
	if err != nil {
		return err
	}
	u.keep(v)
	target, ok := u.find(ev.Target)
	if !ok {
		return nil
	}
	if res.HPBefore != nil {
		if err := u.putCreatureOrVitals(ctx, target, *res.HPBefore, res.DeathBefore); err != nil {
			return err
		}
	}
	if res.CondSet {
		return u.setConditionsBack(ctx, target.ID, res.CondBefore)
	}
	return nil
}

// bardicGiveBack gives back the use of Bardic Inspiration and takes the die from the
// creature it was given to.
func (u *undoer) bardicGiveBack(ctx context.Context, ev actionEvent, who playdb.Combatant) error {
	v, err := u.s.spendResource(ctx, u.c, who.CharacterID, rules.BardicInspirationKey, -ev.Res.Spent)
	if err != nil {
		return err
	}
	u.keep(v)
	if target, ok := u.find(ev.Target); ok {
		if err := u.c.q.SetCombatantInspirationDie(ctx, playdb.SetCombatantInspirationDieParams{ID: target.ID}); err != nil {
			return fmt.Errorf("take the die back: %w", err)
		}
	}
	return nil
}

// resourceLogView is a class resource line of the combat log as the viewer reads it:
// what was spent is the table's to see (a bard spending a use, a paladin's pool going
// down); the hit points a touch gave back are the master's and the target's player's
// (on an NPC the number would say how many were missing); why a touch did nothing is the
// master's alone (it would name a creature's type); the die the bard gave, and when it
// runs out, are the bard's, the holder's and the master's (RN-10, RN-20).
func resourceLogView(ev actionEvent, v combatViewer, actor, target playdb.Combatant) *playv1.CombatLogResource {
	r := ev.Res
	if r == nil {
		return nil
	}
	out := &playv1.CombatLogResource{Spent: r.Spent, SlotLevel: r.SlotLevel, Gained: r.Gained, NothingHappened: r.Nothing}
	switch r.Kind {
	case resLayOnHands:
		layOnHandsLogView(out, r, v, target)
	case resCreateSlot:
		out.Key = flexibleCreate
	case resConvert:
		out.Key = flexibleConvert
	case resBardicGive:
		out.Key = bardicInspirationFt
		if v.master || v.owns(actor) || v.owns(target) {
			out.DieSides, out.ExpiresAtRound = r.Sides, r.ExpiresRound
		}
	}
	return out
}

// layOnHandsLogView fills the Lay on Hands part of a resource line for the viewer.
func layOnHandsLogView(out *playv1.CombatLogResource, r *resourceEvent, v combatViewer, target playdb.Combatant) {
	out.Key = layOnHandsAction
	switch r.Cure {
	case cureDisease:
		out.Cure = playv1.LayOnHandsCureKind_LAY_ON_HANDS_CURE_KIND_DISEASE
	case curePoison:
		out.Cure = playv1.LayOnHandsCureKind_LAY_ON_HANDS_CURE_KIND_POISON
	}
	if r.Cure == "" && (v.master || v.owns(target)) {
		healed := r.Healed
		out.Healed = &healed
	}
	if v.master && r.Nothing {
		why := r.Why
		out.NothingReason = &why
	}
}

// inspirationDieView is the Bardic Inspiration die a combatant holds, as the viewer
// may read it: the master and the holder's player only (the others see the bard spend
// a use, not who holds the die). A die that has run out is no die.
func inspirationDieView(cs []playdb.Combatant, c playdb.Combatant, round int32, v combatViewer) *playv1.InspirationDie {
	if !holdsDie(c, round) || (!v.master && !v.owns(c)) {
		return nil
	}
	out := &playv1.InspirationDie{Sides: *c.InspirationSides, ExpiresAtRound: *c.InspirationExpiresRound, FromCombatantId: deref(c.InspirationFrom)}
	if i := slices.IndexFunc(cs, func(o playdb.Combatant) bool { return o.ID == out.FromCombatantId }); i >= 0 {
		out.FromLabel = cs[i].Label
	}
	return out
}

// resourceTargetsFor lists, for the feature actions that pick a creature (Cura pelas
// Mãos, Inspiração de Bardo), who the combatant can pick, as the viewer sees them: the
// combatants it sees, with the distance and whether the action cannot reach them. Lay on
// Hands reaches 5 ft and may touch the paladin itself; Bardic Inspiration reaches 60 ft
// and never the bard, and a creature that cannot get a die says why, in words that name
// no creature's type (RN-10).
func resourceTargetsFor(terrain grid.Terrain, cs []playdb.Combatant, who playdb.Combatant, v combatViewer, opts *rulesv1.TurnOptions, enc playdb.Encounter) []*playv1.ResourceTargets {
	if who.Kind != kindPlayer {
		return nil
	}
	has := func(key string) bool {
		return slices.ContainsFunc(opts.GetFeatureActions(), func(a *rulesv1.ActionOption) bool { return a.GetAction().GetKey() == key })
	}
	theatre := isTheatre(enc)
	var out []*playv1.ResourceTargets
	if has(layOnHandsAction) {
		out = append(out, layOnHandsTargets(terrain, cs, who, v, theatre))
	}
	if has(bardicInspirationFt) {
		out = append(out, bardicInspirationTargets(terrain, cs, who, v, enc))
	}
	return out
}

// layOnHandsTargets lists who the paladin can touch: itself and what its reach holds.
func layOnHandsTargets(terrain grid.Terrain, cs []playdb.Combatant, who playdb.Combatant, v combatViewer, theatre bool) *playv1.ResourceTargets {
	touch := resourceList(layOnHandsAction)
	self := int32(0) // the paladin's distance to itself
	touch.Targets = append(touch.Targets, &playv1.ResourceTarget{Target: &playv1.TargetInReach{CombatantId: who.ID, Label: who.Label, State: stateOf(who), DistanceFt: &self}})
	for _, t := range targetsFor(terrain, cs, who, v, meleeReachFt, theatre) {
		touch.Targets = append(touch.Targets, &playv1.ResourceTarget{Target: t})
	}
	return touch
}

// bardicInspirationTargets lists who the bard can speak to, each with the reason it cannot
// get a die when it cannot.
func bardicInspirationTargets(terrain grid.Terrain, cs []playdb.Combatant, who playdb.Combatant, v combatViewer, enc playdb.Encounter) *playv1.ResourceTargets {
	theatre := isTheatre(enc)
	voice := resourceList(bardicInspirationFt)
	byID := combatantsByID(cs)
	for _, t := range targetsFor(terrain, cs, who, v, rules.BardicInspirationRangeFt, theatre) {
		voice.Targets = append(voice.Targets, &playv1.ResourceTarget{Target: t, DisabledReasonPt: bardicTargetReason(t, byID[t.GetCombatantId()], theatre, enc.Round)})
	}
	return voice
}

// bardicTargetReason is why a creature in reach cannot get a die, "" when it can.
func bardicTargetReason(t *playv1.TargetInReach, c playdb.Combatant, theatre bool, round int32) string {
	target := rules.BardicInspirationTarget{
		OnMap:   !theatre && t.DistanceFt != nil,
		CanHear: !slices.Contains(c.Conditions, "condition:deafened"),
		HasDie:  holdsDie(c, round),
	}
	if t.DistanceFt != nil {
		target.Distance = int(t.GetDistanceFt())
	}
	reason := rules.BardicInspirationRefusal(target)
	if t.GetTooFar() && reason == "" {
		reason = "Além de 18 m"
	}
	return reason
}

func resourceList(action string) *playv1.ResourceTargets {
	return &playv1.ResourceTargets{ActionKey: action}
}
