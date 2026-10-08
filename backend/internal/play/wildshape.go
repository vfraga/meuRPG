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
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// A druid's Wild Shape (MR-037, Etapa 9, D7). The form is part of the character's
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
//   - Casting is refused in the form (refuseInShape).
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
	if _, err := q.InsertSessionEvent(ctx, playdb.InsertSessionEventParams{
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
	return noSpellsIn(v)
}

// noSpellsIn is the refusal for vitals that are in a beast form, nil otherwise.
func noSpellsIn(v *playv1.CharacterVitals) error {
	if v.GetWildShape() != nil {
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

// playerCharacterOf finds the character a caller may change inside a write: a
// living player's character of the campaign that the master, or its own player,
// acts for. It returns it, and the combat it is in, if it is a combatant of one that
// is not ended (c.enc is then set).
func (s *Service) playerCharacterOf(ctx context.Context, c *combatTx, m authz.Membership, characterID string) (link.Character, *playdb.Combatant, []playdb.Combatant, error) {
	chars, err := s.roster.CombatCharacters(ctx, c.tx, m.CampaignID, []string{characterID})
	if err != nil {
		return link.Character{}, nil, nil, err
	}
	if len(chars) != 1 || !chars[0].Player {
		return link.Character{}, nil, nil, errCharacterNotFound()
	}
	if m.Role != authz.RoleMaster && chars[0].PlayerUserID != m.UserID {
		return link.Character{}, nil, nil, connect.NewError(connect.CodePermissionDenied, errors.New("only the character's player or the master may do this"))
	}
	enc, err := c.q.GetOpenEncounter(ctx, c.session.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return chars[0], nil, nil, nil
	}
	if err != nil {
		return link.Character{}, nil, nil, fmt.Errorf("find the open encounter: %w", err)
	}
	cs, err := c.q.ListCombatants(ctx, enc.ID)
	if err != nil {
		return link.Character{}, nil, nil, fmt.Errorf("list the combatants: %w", err)
	}
	for i := range cs {
		if cs[i].Kind == kindPlayer && cs[i].CharacterID == characterID {
			c.enc = enc
			return chars[0], &cs[i], cs, nil
		}
	}
	return chars[0], nil, nil, nil
}

// spendAction spends the combatant's action (or its bonus action) for a change
// made on its turn in a running combat, and fills in what an undo puts back. The
// master may act again with the action used. It checks that it is the combatant's
// turn and that it may act, and adds what an undo puts back to ev.
func (s *Service) spendAction(ctx context.Context, c *combatTx, v combatViewer, who playdb.Combatant, bonus bool, ev *actionEvent) error {
	if err := s.mustActNow(ctx, c, who); err != nil {
		return err
	}
	used, reason, what := who.ActionUsed, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ACTION_USED, "the action of this turn is used"
	if bonus {
		used, reason, what = who.BonusActionUsed, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_BONUS_ACTION_USED, "the bonus action of this turn is used"
	}
	if used && !v.master {
		return errEncounter(reason, what)
	}
	run, err := breakRun(ctx, c, who)
	if err != nil {
		return err
	}
	ev.Round, ev.Secret, ev.Actor, ev.RunBefore = c.enc.Round, who.Hidden, who.ID, run
	ev.ActionBefore, ev.BonusBefore, ev.ReactionBefore, ev.DashedBefore = who.ActionUsed, who.BonusActionUsed, who.ReactionUsed, who.Dashed
	ev.Spent = true
	after := who
	if bonus {
		after.BonusActionUsed = true
	} else {
		after.ActionUsed = true
	}
	if err := c.q.SetCombatantEconomy(ctx, playdb.SetCombatantEconomyParams{
		ID: who.ID, ActionUsed: after.ActionUsed, BonusActionUsed: after.BonusActionUsed, ReactionUsed: after.ReactionUsed, Dashed: after.Dashed,
	}); err != nil {
		return fmt.Errorf("spend the action: %w", err)
	}
	return nil
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
	characterID, ok := parseID(rawCharacter)
	if !ok {
		return nil, nil, errCharacterNotFound()
	}
	v := viewerOf(m)
	kind := eventWildShapeStarted
	if leave {
		kind = eventWildShapeEnded
	}

	var made actionEvent
	var vitals *playv1.CharacterVitals
	var mapID string
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: requestHash(campaignID, rawCharacter, beast, strconv.FormatBool(leave)), kind: kind}, func(c *combatTx) (any, error) {
		made, vitals, mapID = actionEvent{}, nil, deref(c.session.CurrentMapID)
		_, who, _, err := s.playerCharacterOf(ctx, c, m, characterID)
		if err != nil {
			return nil, err
		}
		inCombat := who != nil
		turns := inCombat && c.enc.Status == statusActive // in SETUP nobody has a turn to spend
		if inCombat {
			mapID = deref(c.enc.MapID)
		}

		var before *playv1.CharacterVitals
		var body link.Character
		if !leave {
			now, err := s.vitals.GetVitalsTx(ctx, c.tx, c.session.CampaignID, characterID)
			if err != nil {
				return nil, err
			}
			if err := refuseIfDown(now, who); err != nil {
				return nil, err
			}
		}
		if leave {
			if before, err = s.vitals.GetVitalsTx(ctx, c.tx, c.session.CampaignID, characterID); err != nil {
				return nil, err
			}
			if before.GetWildShape() == nil {
				return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_IN_WILD_SHAPE, "the character is in its own shape")
			}
		}
		if turns {
			// Only on its turn; the cost comes after the form is checked, so a refusal
			// says what is wrong with the change before it says the action is used.
			if err := s.mustActNow(ctx, c, *who); err != nil {
				return nil, err
			}
		}
		if leave {
			if vitals, body, err = s.vitals.SetWildShape(ctx, c.tx, c.session.CampaignID, characterID, "", 0); err != nil {
				return nil, err
			}
			w := before.GetWildShape()
			made.Beast, made.ShapeSet, made.ShapeBefore = w.GetBeastKey(), true, &shapeState{Beast: w.GetBeastKey(), HP: w.GetHitPointsCurrent()}
			made.Reason = endedByLeaving
		} else {
			if _, vitals, body, err = s.vitals.AssumeWildShape(ctx, c.tx, c.session.CampaignID, characterID, beast); err != nil {
				return nil, errShape(err)
			}
			// One use of the Forma Selvagem resource (the character has no more when
			// the app says "Sem usos": NO_USES, nothing is changed).
			if vitals, err = s.spendResource(ctx, c, characterID, wildShapeResource, 1); err != nil {
				return nil, err
			}
		}
		if turns && (!leave || !v.master) {
			// "Voltar à forma normal" is a bonus action (the master's costs nothing).
			if err := s.spendAction(ctx, c, v, *who, leave, &made); err != nil {
				return nil, err
			}
		}
		if !leave {
			made.Beast, made.Resource, made.ShapeSet = beast, wildShapeResource, true
		}
		made.OwnerCharacter = characterID
		if inCombat {
			if err := applyBody(ctx, c, *who, body); err != nil {
				return nil, err
			}
			made.EncounterID = c.enc.ID
			if made.Actor == "" { // in SETUP no action is spent: still the combatant's
				made.Round, made.Secret, made.Actor = c.enc.Round, who.Hidden, who.ID
			}
			if c.enc, err = c.q.TouchEncounter(ctx, c.enc.ID); err != nil {
				return nil, fmt.Errorf("touch the encounter: %w", err)
			}
		}
		c.characterID = &characterID
		return made, nil
	})
	if err != nil {
		return nil, nil, s.dbError(ctx, "change a Wild Shape form", err)
	}
	ev, err := resultEvent(res, made)
	if err != nil {
		return nil, nil, s.dbError(ctx, "read the Wild Shape change", err)
	}
	if res.repeated && ev.OwnerCharacter != characterID {
		// The key is another character's change: never answer with that character's vitals.
		return nil, nil, connect.NewError(connect.CodeInvalidArgument, errors.New("idempotency_key was already used for another change"))
	}
	if res.repeated { // a retry: nothing changed, answer with the vitals as they are now
		if vitals, err = s.vitals.GetVitals(ctx, m.CampaignID, characterID); err != nil {
			return nil, nil, s.dbError(ctx, "get vitals", err)
		}
	}
	var out *playv1.Encounter
	if ev.EncounterID != "" {
		res.encounterID = ev.EncounterID
		if out, err = s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
			s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
			s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, !ev.Secret)
		}); err != nil {
			return nil, nil, err
		}
	}
	if !res.repeated {
		s.publishVitals(m.CampaignID, vitals)
		s.maps.VisionChanged(ctx, m.CampaignID, mapID) // the beast's senses, or the character's again
	}
	return vitals, out, nil
}

// shapeUndo takes back a wild_shape_started or wild_shape_ended event the master's
// undo reaches (the action that started or ended the form): the economy, the
// running start and the use come back, and the form is as it was (none for a start,
// the beast with its hit points for a "Voltar à forma normal"). A form that ended
// because the beast fell is the damage's to take back, never this.
func (s *Service) shapeUndo(ctx context.Context, c *combatTx, kind string, ev actionEvent, find func(string) (playdb.Combatant, bool), keep func(*playv1.CharacterVitals)) error {
	who, ok := find(ev.Actor)
	if ok && ev.Spent {
		if err := c.q.SetCombatantEconomy(ctx, playdb.SetCombatantEconomyParams{
			ID: who.ID, ActionUsed: ev.ActionBefore, BonusActionUsed: ev.BonusBefore, ReactionUsed: ev.ReactionBefore, Dashed: ev.DashedBefore,
		}); err != nil {
			return fmt.Errorf("put back the economy: %w", err)
		}
		if ev.RunBefore != 0 {
			if err := c.q.SetCombatantRun(ctx, playdb.SetCombatantRunParams{ID: who.ID, LastMoveDft: ev.RunBefore}); err != nil {
				return fmt.Errorf("put back the running start: %w", err)
			}
		}
	}
	campaign, character := c.session.CampaignID, ev.OwnerCharacter
	if kind == eventWildShapeStarted {
		if ev.Resource != "" {
			v, err := s.spendResource(ctx, c, character, ev.Resource, -1)
			if err != nil {
				return err
			}
			keep(v)
		}
		after, body, err := s.vitals.SetWildShape(ctx, c.tx, campaign, character, "", 0)
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
	if ev.ShapeBefore == nil {
		return nil
	}
	after, body, err := s.vitals.SetWildShape(ctx, c.tx, campaign, character, ev.ShapeBefore.Beast, ev.ShapeBefore.HP)
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
