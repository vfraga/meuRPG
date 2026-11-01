package play

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/platform/uid"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// A druid's Wild Shape (MR-037). The form is part of the character's
// vitals for the API (CharacterVitals.wild_shape) and lives in the table
// character_wild_shapes, kept by the characters module; this file is what a table
// does with it:
//
//   - AssumeWildShape: spends one use of the Forma Selvagem resource and, in a
//     combat, the action; the combatant takes the beast's speed, size and jumps
//     (the characters module reads the rest of the beast's numbers from the form:
//     armor class, attacks, senses, no spells).
//   - Damage goes to the beast's pool (damageBeast); when the beast falls to 0 the
//     form ends and the damage left over goes to the druid (the SRD).
//   - Healing in the form heals the beast (healCombatant).
//   - LeaveWildShape: a bonus action in a combat.
//   - Casting is refused in the form (refuseInShape), unless the druid has Beast Spells.
//
// The events are wild_shape_started and wild_shape_ended: ids, keys and numbers
// only. The master's undo takes back the action that started or ended the form
// (combat_undo.go); a form that ended because the beast fell is part of the damage
// that did it.

// wildShapeResource is the key of the Forma Selvagem resource (2 uses per short
// rest in the SRD, effects/druid.json).
const wildShapeResource = "wild_shape"

// The reasons a wild_shape_ended event gives.
const (
	endedByLeaving = "left"   // "Voltar à forma normal"
	endedByDamage  = "damage" // the beast fell to 0
	endedByMaster  = "master" // the master's correction took the beast to 0 hit points
	// The SRD ends the form when the druid falls unconscious or to 0 hit points: its
	// own, whatever did it (endedAtZero), or the Unconscious condition (endedAsleep).
	endedAtZero = "zero_hp"
	endedAsleep = "unconscious"
)

// endsBySelf says a wild_shape_ended reason is part of the change that caused it,
// written before that change's event: it is no action for an undo to take back.
func endsBySelf(reason string) bool {
	return reason == endedByDamage || reason == endedAtZero || reason == endedAsleep || reason == endedByMaster
}

// shapeLine says where a form's end belongs in the combat log: the session's combat that is
// running (not in SETUP) and the druid's combatant in it. Empty when there is none, and the
// event then stays out of the log (it has no combat).
func (s *Service) shapeLine(ctx context.Context, q *playdb.Queries, sessionID, characterID string) (encounterID string, round int32, actor string, hidden bool) {
	enc, err := q.GetOpenEncounter(ctx, sessionID)
	if err != nil || enc.Status != statusActive {
		return "", 0, "", false
	}
	cs, err := q.ListCombatants(ctx, enc.ID)
	if err != nil {
		return "", 0, "", false
	}
	i := slices.IndexFunc(cs, func(c playdb.Combatant) bool { return c.Kind == kindPlayer && c.CharacterID == characterID })
	if i < 0 {
		return "", 0, "", false
	}
	return enc.ID, enc.Round, cs[i].ID, cs[i].Hidden
}

// formEnds ends a druid's form where only the vitals are at hand (it fell to 0 hit
// points): the form goes, the combatant of the session's combat takes its own numbers
// back, and a wild_shape_ended event says why, with the damage that carried over
// to the druid, if any. It returns the vitals after.
func (s *Service) formEnds(ctx context.Context, q *playdb.Queries, tx pgx.Tx, sessionID, campaignID, characterID, beast, reason string, carried int32) (*playv1.CharacterVitals, error) {
	after, body, err := s.vitals.SetWildShape(ctx, tx, campaignID, characterID, "", 0)
	if err != nil {
		return nil, err
	}
	if err := q.SetCombatantBodyOfCharacter(ctx, playdb.SetCombatantBodyOfCharacterParams{
		CharacterID: characterID, GameSessionID: sessionID, SpeedFt: clamp32(body.SpeedFt, 0, 600), SpeedFlyFt: clamp32(body.SpeedFlyFt, 0, 600),
		Size: sizeKey(body.Size), JumpLongDft: clamp32(body.JumpLongDFt, 0, 6000), JumpHighDft: clamp32(body.JumpHighDFt, 0, 6000),
		JumpLongBonusDft: clamp32(body.JumpLongBonusDFt, 0, 6000),
	}); err != nil {
		return nil, fmt.Errorf("give the combatant its own numbers: %w", err)
	}
	seq, err := q.NextSessionEventSeq(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("next event number: %w", err)
	}
	encID, round, actor, hidden := s.shapeLine(ctx, q, sessionID, characterID)
	payload, err := json.Marshal(actionEvent{OwnerCharacter: characterID, Beast: beast, Carried: carried, Reason: reason, EncounterID: encID, Round: round, Actor: actor, Secret: hidden})
	if err != nil {
		return nil, fmt.Errorf("encode the event payload: %w", err)
	}
	var encounterID *string
	if encID != "" {
		encounterID = &encID
	}
	if _, err := insertSessionEvent(ctx, q, playdb.InsertSessionEventParams{
		GameSessionID: sessionID, Seq: seq, Kind: eventWildShapeEnded, CharacterID: &characterID, Payload: payload, CreatedAt: s.now(), EncounterID: encounterID,
	}); err != nil {
		return nil, fmt.Errorf("insert session event: %w", err)
	}
	return after, nil
}

// endFormIfAsleep ends the form of a druid that was just given the Unconscious
// condition (a sleep spell, the master's hand): the SRD returns it to its own shape.
func (s *Service) endFormIfAsleep(ctx context.Context, c *combatTx, who playdb.Combatant, conditions []string) error {
	if who.Kind != kindPlayer || !slices.Contains(conditions, unconscious) {
		return nil
	}
	now, err := s.vitals.GetVitalsTx(ctx, c.tx, c.session.CampaignID, who.CharacterID)
	if err != nil {
		if connect.CodeOf(err) == connect.CodeNotFound {
			return nil
		}
		return err
	}
	w := now.GetWildShape()
	if w == nil {
		return nil
	}
	after, body, err := s.vitals.SetWildShape(ctx, c.tx, c.session.CampaignID, who.CharacterID, "", 0)
	if err != nil {
		return err
	}
	if err := applyBody(ctx, c, who, body); err != nil {
		return err
	}
	c.characterID = &who.CharacterID
	c.told = append(c.told, after)
	return insertEvent(ctx, c, eventWildShapeEnded, &c.actorUserID, nil, actionEvent{
		Round: c.enc.Round, Secret: who.Hidden, Actor: who.ID, OwnerCharacter: who.CharacterID, Beast: w.GetBeastKey(), Reason: endedAsleep, EncounterID: c.enc.ID,
	})
}

// refuseIfDown refuses a Wild Shape for a druid at 0 hit points or unconscious: it
// cannot take an action, and the form would end at once (SRD).
func refuseIfDown(v *playv1.CharacterVitals, who *playdb.Combatant) error {
	if (v.GetHitPointsMax() > 0 && v.GetHitPointsCurrent() == 0) || (who != nil && slices.Contains(who.Conditions, unconscious)) {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_COMBATANT_DOWN, "the character is down or unconscious")
	}
	return nil
}

// errShape turns the characters module's refusals into the typed ones.
func errShape(err error) error {
	switch {
	case errors.Is(err, link.ErrBeastNotAllowed):
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_WILD_SHAPE_BEAST_NOT_ALLOWED, "the beast is not one this character's Wild Shape allows")
	case errors.Is(err, link.ErrAlreadyInWildShape):
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ALREADY_IN_WILD_SHAPE, "the character is in a beast form already")
	}
	return err
}

// refuseInShape refuses a spell for a player's character in a beast form (MR-037).
func (s *Service) refuseInShape(ctx context.Context, c *combatTx, who playdb.Combatant) error {
	if who.Kind != kindPlayer {
		return nil
	}
	v, err := s.vitals.GetVitalsTx(ctx, c.tx, c.session.CampaignID, who.CharacterID)
	if err != nil {
		return err
	}
	if v.GetWildShape() == nil {
		return nil
	}
	chars, err := s.roster.CombatCharacters(ctx, c.tx, c.session.CampaignID, []string{who.CharacterID})
	if err != nil {
		return err
	}
	return noSpellsIn(v, len(chars) == 1 && chars[0].CastsInBeastForm)
}

// noSpellsIn is the refusal for vitals that are in a beast form, nil otherwise.
// A druid of level 18 (Beast Spells) casts in any form, so castsInForm lets it
// pass.
func noSpellsIn(v *playv1.CharacterVitals, castsInForm bool) error {
	if v.GetWildShape() != nil && !castsInForm {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_WILD_SHAPE_NO_SPELLS, "a druid in a beast form cannot cast spells")
	}
	return nil
}

// applyBody gives a combatant the numbers a character has in its form: speed, fly
// speed, size and jump limits (the beast's, or its own again when the form ends).
func applyBody(ctx context.Context, c *combatTx, who playdb.Combatant, body link.Character) error {
	if err := c.q.SetCombatantBody(ctx, playdb.SetCombatantBodyParams{
		ID: who.ID, SpeedFt: clamp32(body.SpeedFt, 0, 600), SpeedFlyFt: clamp32(body.SpeedFlyFt, 0, 600), Size: sizeKey(body.Size),
		JumpLongDft: clamp32(body.JumpLongDFt, 0, 6000), JumpHighDft: clamp32(body.JumpHighDFt, 0, 6000),
		JumpLongBonusDft: clamp32(body.JumpLongBonusDFt, 0, 6000),
	}); err != nil {
		return fmt.Errorf("give the combatant its form's numbers: %w", err)
	}
	return nil
}

// beastDamageRequest is the vitals change a damage makes on a druid in a beast
// form (RN-02, SRD): the beast's pool takes it, with no temporary hit points; when
// the beast falls (left 0), what is left (carried) goes to the druid's own pool
// and temporary hit points. It does not end the form.
func beastDamageRequest(now *playv1.CharacterVitals, amount int32) (req *playv1.AdjustCharacterVitalsRequest, left, carried int32) {
	w := now.GetWildShape()
	left = clamp32(combat.ApplyDamage(int(w.GetHitPointsCurrent()), 0, int(amount)).HP, 0, math.MaxInt32)
	req = &playv1.AdjustCharacterVitalsRequest{WildShapeHitPointsCurrent: &left}
	if left == 0 {
		carried = max(amount-w.GetHitPointsCurrent(), 0)
		if carried > 0 {
			dmg := combat.ApplyDamage(int(now.GetHitPointsCurrent()), int(now.GetHitPointsTemporary()), int(carried))
			hp, temp := clamp32(dmg.HP, 0, math.MaxInt32), clamp32(dmg.TempHP, 0, math.MaxInt32)
			req.HitPointsCurrent, req.HitPointsTemporary = &hp, &temp
		}
	}
	return req, left, carried
}

// damageBeast lands a damage on a druid in a beast form (RN-02, SRD): the beast's
// pool takes it, with no temporary hit points; at 0 the form ends and what is left
// goes to the character through its own pool and temporary hit points. It records
// the carried damage in ev, ends the combatant's form numbers and writes the
// wild_shape_ended event before the damage's own, so the damage stays the last
// thing an undo takes back. It returns the vitals before and after.
func (s *Service) damageBeast(ctx context.Context, c *combatTx, target playdb.Combatant, now *playv1.CharacterVitals, amount int32, ev *actionEvent) (before, after *playv1.CharacterVitals, err error) {
	w := now.GetWildShape()
	req, left, carried := beastDamageRequest(now, amount)
	if before, after, err = s.vitalsOf(ctx, c, target.CharacterID, req); err != nil {
		return nil, nil, err
	}
	if left > 0 {
		return before, after, nil
	}
	// The beast fell: the druid is itself again, with its own numbers.
	var body link.Character
	if after, body, err = s.vitals.SetWildShape(ctx, c.tx, c.session.CampaignID, target.CharacterID, "", 0); err != nil {
		return nil, nil, err
	}
	if err := applyBody(ctx, c, target, body); err != nil {
		return nil, nil, err
	}
	c.told = append(c.told, after)
	ev.Carried = carried
	c.characterID = &target.CharacterID
	if err := insertEvent(ctx, c, eventWildShapeEnded, &c.actorUserID, nil, actionEvent{
		Round: c.enc.Round, Secret: target.Hidden, Actor: target.ID, OwnerCharacter: target.CharacterID, Beast: w.GetBeastKey(),
		Carried: carried, Reason: endedByDamage, EncounterID: c.enc.ID,
	}); err != nil {
		return nil, nil, err
	}
	return before, after, nil
}

// AssumeWildShape implements playv1connect.PlayServiceHandler.
func (s *Service) AssumeWildShape(
	ctx context.Context,
	req *connect.Request[playv1.AssumeWildShapeRequest],
) (*connect.Response[playv1.AssumeWildShapeResponse], error) {
	// The caller is checked before the request is, so an anonymous caller
	// learns "unauthenticated", never what a valid request looks like.
	if _, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId()); err != nil {
		return nil, err
	}
	beast := req.Msg.GetBeastKey()
	if beast == "" || len(beast) > 100 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("beast_key must name an SRD beast"))
	}
	vitals, enc, err := s.shapeChange(ctx, req.Msg.GetCampaignId(), req.Msg.GetCharacterId(), req.Msg.GetIdempotencyKey(), beast, false)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.AssumeWildShapeResponse{Vitals: vitals, Encounter: enc}), nil
}

// LeaveWildShape implements playv1connect.PlayServiceHandler.
func (s *Service) LeaveWildShape(
	ctx context.Context,
	req *connect.Request[playv1.LeaveWildShapeRequest],
) (*connect.Response[playv1.LeaveWildShapeResponse], error) {
	vitals, enc, err := s.shapeChange(ctx, req.Msg.GetCampaignId(), req.Msg.GetCharacterId(), req.Msg.GetIdempotencyKey(), "", true)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.LeaveWildShapeResponse{Vitals: vitals, Encounter: enc}), nil
}

// shapeRun is what an AssumeWildShape or LeaveWildShape works with inside its
// transaction, and what the answer reads after it.
type shapeRun struct {
	m           authz.Membership
	v           combatViewer
	characterID string
	beast       string
	leave       bool

	made   actionEvent
	vitals *playv1.CharacterVitals
	mapID  string

	// who is the character's combatant, when it is in a combat; turns says the
	// combat has begun, so the change costs an action of a turn. before is the
	// vitals of a form that is left, and body the numbers the combatant takes.
	who      *playdb.Combatant
	inCombat bool
	turns    bool
	before   *playv1.CharacterVitals
	body     link.Character
}

// shapeChange is AssumeWildShape (leave false) and LeaveWildShape (leave true):
// one transaction locks the session, spends what the change costs, changes the
// form and writes the event. It returns the character's vitals after and the combat
// as the caller sees it, when the character is in one.
func (s *Service) shapeChange(ctx context.Context, campaignID, rawCharacter, rawKey, beast string, leave bool) (*playv1.CharacterVitals, *playv1.Encounter, error) {
	m, err := authz.RequireCampaignMember(ctx, campaignID)
	if err != nil {
		return nil, nil, err
	}
	key, err := parseKey(rawKey)
	if err != nil {
		return nil, nil, err
	}
	characterID, ok := uid.Canonical(rawCharacter)
	if !ok {
		return nil, nil, errCharacterNotFound()
	}
	kind := eventWildShapeStarted
	if leave {
		kind = eventWildShapeEnded
	}
	r := &shapeRun{m: m, v: viewerOf(m), characterID: characterID, beast: beast, leave: leave}
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: requestHash(campaignID, rawCharacter, beast, strconv.FormatBool(leave)), kind: kind}, func(c *combatTx) (any, error) {
		return s.shapeTx(ctx, c, r)
	})
	if err != nil {
		return nil, nil, s.dbError(ctx, "change a Wild Shape form", err)
	}
	ev, err := resultEvent(res, r.made)
	if err != nil {
		return nil, nil, s.dbError(ctx, "read the Wild Shape change", err)
	}
	vitals, out, err := s.answerFormChange(ctx, m, characterID, res, ev, r.vitals)
	if err != nil {
		return nil, nil, err
	}
	if !res.repeated {
		pctx, stop := afterCommit(ctx) // the change is committed: a caller that hangs up must not leave the fog unheard
		defer stop()
		s.publishVitals(m.CampaignID, vitals)
		s.maps.VisionChanged(pctx, m.CampaignID, r.mapID) // the beast's senses, or the character's again
	}
	return vitals, out, nil
}

// shapeTx is the Wild Shape transaction: the form is checked, the turn's cost is
// paid, the form changes and the event gets its fields.
func (s *Service) shapeTx(ctx context.Context, c *combatTx, r *shapeRun) (any, error) {
	r.made, r.vitals, r.mapID = actionEvent{}, nil, conv.OrZero(c.session.CurrentMapID)
	_, who, _, err := s.playerCharacterOf(ctx, c, r.m, r.characterID)
	if err != nil {
		return nil, err
	}
	r.who = who
	r.inCombat = who != nil
	r.turns = r.inCombat && c.enc.Status == statusActive // in SETUP nobody has a turn to spend
	if r.inCombat {
		r.mapID = conv.OrZero(c.enc.MapID)
	}
	if err := s.shapeCheck(ctx, c, r); err != nil {
		return nil, err
	}
	if r.turns {
		// Only on its turn; the cost comes after the form is checked, so a refusal
		// says what is wrong with the change before it says the action is used.
		if err := s.mustActNow(ctx, c, *r.who); err != nil {
			return nil, err
		}
	}
	if err := s.shapeApply(ctx, c, r); err != nil {
		return nil, err
	}
	if r.turns && (!r.leave || !r.v.master) {
		// "Voltar à forma normal" is a bonus action (the master's costs nothing).
		if err := s.spendAction(ctx, c, r.v, *r.who, r.leave, &r.made); err != nil {
			return nil, err
		}
	}
	if !r.leave {
		r.made.Beast, r.made.ShapeSet = r.beast, true
	}
	return s.shapeSeal(ctx, c, r)
}

// shapeCheck refuses a change the form does not allow: a druid that is down cannot
// assume a form, and one in its own shape has none to leave.
func (s *Service) shapeCheck(ctx context.Context, c *combatTx, r *shapeRun) error {
	now, err := s.vitals.GetVitalsTx(ctx, c.tx, c.session.CampaignID, r.characterID)
	if err != nil {
		return err
	}
	if !r.leave {
		return refuseIfDown(now, r.who)
	}
	r.before = now
	if now.GetWildShape() == nil {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_IN_WILD_SHAPE, "the character is in its own shape")
	}
	return nil
}

// shapeApply changes the form: the druid leaves it, or assumes the beast and
// spends a use of the resource.
func (s *Service) shapeApply(ctx context.Context, c *combatTx, r *shapeRun) error {
	var err error
	if r.leave {
		if r.vitals, r.body, err = s.vitals.SetWildShape(ctx, c.tx, c.session.CampaignID, r.characterID, "", 0); err != nil {
			return err
		}
		w := r.before.GetWildShape()
		r.made.Beast, r.made.ShapeSet, r.made.ShapeBefore = w.GetBeastKey(), true, &shapeState{Beast: w.GetBeastKey(), HP: w.GetHitPointsCurrent()}
		r.made.Reason = endedByLeaving
		return nil
	}
	if _, r.vitals, r.body, err = s.vitals.AssumeWildShape(ctx, c.tx, c.session.CampaignID, r.characterID, r.beast); err != nil {
		return errShape(err)
	}
	// The uses of the Forma Selvagem resource the change costs: one, unless a feature of the
	// table makes it cheaper, even free (the character has no more when the app says "Sem
	// usos": NO_USES, nothing is changed).
	cost := r.body.WildShapeCost
	if cost <= 0 {
		return nil
	}
	if r.vitals, err = s.spendResource(ctx, c, r.characterID, wildShapeResource, cost); err != nil {
		return err
	}
	r.made.Resource, r.made.PointsSpent = wildShapeResource, cost
	return nil
}

// shapeSeal fills in the event's owner and, in a combat, gives the combatant the
// numbers of its form, sets the event's combat, round and actor and touches the
// combat.
func (s *Service) shapeSeal(ctx context.Context, c *combatTx, r *shapeRun) (any, error) {
	r.made.OwnerCharacter = r.characterID
	if r.inCombat {
		if err := applyBody(ctx, c, *r.who, r.body); err != nil {
			return nil, err
		}
		r.made.EncounterID = c.enc.ID
		if r.made.Actor == "" { // in SETUP no action is spent: still the combatant's
			r.made.Round, r.made.Secret, r.made.Actor = c.enc.Round, r.who.Hidden, r.who.ID
		}
		if err := touch(ctx, c); err != nil {
			return nil, err
		}
	}
	c.characterID = &r.characterID
	return r.made, nil
}

// shapeUndo takes back a wild_shape_started or wild_shape_ended event the master's
// undo reaches (the action that started or ended the form): the economy, the
// running start and the use come back, and the form is as it was (none for a start,
// the beast with its hit points for a "Voltar à forma normal"). A form that ended
// because the beast fell is the damage's to take back, never this.
func (s *Service) shapeUndo(ctx context.Context, c *combatTx, kind string, ev actionEvent, find func(string) (playdb.Combatant, bool), keep func(*playv1.CharacterVitals)) error {
	who, ok := find(ev.Actor)
	if ok {
		if err := putBackSpentAction(ctx, c, who, ev); err != nil {
			return err
		}
	}
	if kind == eventWildShapeStarted {
		return s.shapeUndoStart(ctx, c, ev, who, ok, keep)
	}
	return s.shapeUndoEnd(ctx, c, ev, who, ok, keep)
}

// shapeUndoStart takes back the start of a form: the use comes back and the
// druid is in its own shape.
func (s *Service) shapeUndoStart(ctx context.Context, c *combatTx, ev actionEvent, who playdb.Combatant, ok bool, keep func(*playv1.CharacterVitals)) error {
	if ev.Resource != "" {
		back := ev.PointsSpent // an event without PointsSpent counts as one use
		if back <= 0 {
			back = 1
		}
		v, err := s.spendResource(ctx, c, ev.OwnerCharacter, ev.Resource, -back)
		if err != nil {
			return err
		}
		keep(v)
	}
	return s.shapeUndoSet(ctx, c, ev, who, ok, "", 0, keep)
}

// shapeUndoEnd takes back a "Voltar à forma normal": the beast is back with the
// hit points it had.
func (s *Service) shapeUndoEnd(ctx context.Context, c *combatTx, ev actionEvent, who playdb.Combatant, ok bool, keep func(*playv1.CharacterVitals)) error {
	if ev.ShapeBefore == nil {
		return nil
	}
	return s.shapeUndoSet(ctx, c, ev, who, ok, ev.ShapeBefore.Beast, ev.ShapeBefore.HP, keep)
}

// shapeUndoSet sets the form an undo puts back (none for an empty beast), tells
// the vitals and gives the combatant, if it is still there, the numbers of that form.
func (s *Service) shapeUndoSet(ctx context.Context, c *combatTx, ev actionEvent, who playdb.Combatant, ok bool, beast string, hp int32, keep func(*playv1.CharacterVitals)) error {
	after, body, err := s.vitals.SetWildShape(ctx, c.tx, c.session.CampaignID, ev.OwnerCharacter, beast, hp)
	if err != nil {
		return err
	}
	keep(after)
	c.told = append(c.told, after)
	if ok {
		return applyBody(ctx, c, who, body)
	}
	return nil
}

// wildShapeFeature says an action key is the Wild Shape action of the sheet, which
// is taken with AssumeWildShape because it needs the beast.
func wildShapeFeature(actionKey string) bool {
	return strings.HasPrefix(actionKey, "feature:wild-shape")
}
