package play

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// "Ver pelos olhos do familiar" (MR-036, Etapa 9, D6): the player of a character
// with a familiar sees what the familiar sees, with the familiar's senses, while
// the familiar is on the same map within 30 m. The vision is the maps module's
// (a viewer of its own for the fog, link.PartyMember.Eyes); what is kept here is
// when it starts and ends, and what it costs:
//
//   - outside a combat it lasts until the player stops it;
//   - in a combat it costs the character's action, on its turn, and ends at the
//     start of the character's next turn (startTurn calls endFamiliarSights), or
//     earlier if the player stops it: the action is not given back;
//   - while it lasts the character is blind and deaf: in a combat the combatant
//     gets the blinded and deafened conditions (the master's reminder; the app does
//     no maths with them), outside one the vitals say it (FamiliarSightState).
//
// The state is on the vitals (character_vitals.familiar_sight_*), the events are
// familiar_sight with "start" or "stop": ids only.

// familiarSightRange is how far the familiar may be from the character, in squares:
// 30 m (SRD: 100 ft) of 1,5 m. The distance is the circle of RN-21: the squares'
// centers in a straight line.
const familiarSightRange = 20

// The reasons a familiar_sight event gives for a "stop".
const (
	sightStopped      = "stopped" // the player stopped it
	sightTurnStart    = "turn"    // the character's next turn started
	sightCombatJoined = "combat"  // the character joined a combat, or its combat began
)

// sightConditions are the conditions a character looking through its familiar
// has: it neither sees nor hears what is around it.
var sightConditions = []string{"condition:blinded", "condition:deafened"}

func errSight(reason playv1.FamiliarSightBlockedReason, msg string) error {
	return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_FAMILIAR_SIGHT_BLOCKED, msg,
		func(b *playv1.EncounterBlocked) { b.FamiliarSightReason = reason })
}

// StartFamiliarSight implements playv1connect.PlayServiceHandler.
func (s *Service) StartFamiliarSight(
	ctx context.Context,
	req *connect.Request[playv1.StartFamiliarSightRequest],
) (*connect.Response[playv1.StartFamiliarSightResponse], error) {
	vitals, enc, err := s.sightChange(ctx, req.Msg.GetCampaignId(), req.Msg.GetCharacterId(), req.Msg.GetIdempotencyKey(), false)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.StartFamiliarSightResponse{Vitals: vitals, Encounter: enc}), nil
}

// StopFamiliarSight implements playv1connect.PlayServiceHandler.
func (s *Service) StopFamiliarSight(
	ctx context.Context,
	req *connect.Request[playv1.StopFamiliarSightRequest],
) (*connect.Response[playv1.StopFamiliarSightResponse], error) {
	vitals, enc, err := s.sightChange(ctx, req.Msg.GetCampaignId(), req.Msg.GetCharacterId(), req.Msg.GetIdempotencyKey(), true)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.StopFamiliarSightResponse{Vitals: vitals, Encounter: enc}), nil
}

// squaresApart says whether two squares are within familiarSightRange of each
// other.
func withinSightRange(a, b [2]int32) bool {
	dc, dr := int(a[0]-b[0]), int(a[1]-b[1])
	return dc*dc+dr*dr <= familiarSightRange*familiarSightRange
}

// familiarSquares finds where the character and its familiar stand, in squares:
// on the combat's map by their combatants while a combat runs, else on the
// session's current map by their tokens. ok is false when either has no square.
func (s *Service) familiarSquares(ctx context.Context, c *combatTx, who *playdb.Combatant, cs []playdb.Combatant, characterID, creatureID string) (me, it [2]int32, ok bool, err error) {
	if who != nil {
		i := slices.IndexFunc(cs, func(o playdb.Combatant) bool { return deref(o.CreatureID) == creatureID && !o.Dismissed })
		if i < 0 || !placed(*who) || !placed(cs[i]) {
			return me, it, false, nil
		}
		return [2]int32{*who.GridCol, *who.GridRow}, [2]int32{*cs[i].GridCol, *cs[i].GridRow}, true, nil
	}
	mapID := deref(c.session.CurrentMapID)
	if mapID == "" {
		return me, it, false, nil
	}
	g, err := s.maps.MapGrid(ctx, c.tx, c.session.CampaignID, mapID)
	if connect.CodeOf(err) == connect.CodeNotFound || (err == nil && !g.OK()) {
		return me, it, false, nil // a map that is gone, or has no grid, has no distances
	}
	if err != nil {
		return me, it, false, err
	}
	tokens, err := s.maps.MapTokens(ctx, c.tx, mapID)
	if err != nil {
		return me, it, false, err
	}
	var gotMe, gotIt bool
	for _, t := range tokens {
		switch {
		case t.CreatureID == "" && t.CharacterID == characterID:
			col, row := squareOf(g, t.XBP, t.YBP)
			me, gotMe = [2]int32{col, row}, true
		case t.CreatureID == creatureID:
			col, row := squareOf(g, t.XBP, t.YBP)
			it, gotIt = [2]int32{col, row}, true
		}
	}
	return me, it, gotMe && gotIt, nil
}

// withConditions adds the conditions the combatant does not have, and returns the
// new list and the ones it added.
func withConditions(have, add []string) (all, added []string) {
	all = slices.Clone(have)
	for _, key := range add {
		if !slices.Contains(all, key) {
			all, added = append(all, key), append(added, key)
		}
	}
	return all, added
}

// withoutConditions takes the conditions away from the list.
func withoutConditions(have, remove []string) []string {
	return slices.DeleteFunc(slices.Clone(have), func(k string) bool { return slices.Contains(remove, k) })
}

// setConditions writes a combatant's conditions.
func setConditions(ctx context.Context, c *combatTx, who playdb.Combatant, list []string) error {
	if err := c.q.SetCombatantConditions(ctx, playdb.SetCombatantConditionsParams{ID: who.ID, Conditions: nonNil(list)}); err != nil {
		return fmt.Errorf("set the conditions: %w", err)
	}
	return nil
}

// sightChange is StartFamiliarSight (stop false) and StopFamiliarSight (stop true).
func (s *Service) sightChange(ctx context.Context, campaignID, rawCharacter, rawKey string, stop bool) (*playv1.CharacterVitals, *playv1.Encounter, error) {
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

	var made actionEvent
	var vitals *playv1.CharacterVitals
	var mapID string
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: requestHash(campaignID, rawCharacter, strconv.FormatBool(stop)), kind: eventFamiliarSight}, func(c *combatTx) (any, error) {
		made, vitals, mapID = actionEvent{}, nil, deref(c.session.CurrentMapID)
		_, who, cs, err := s.playerCharacterOf(ctx, c, m, characterID)
		if err != nil {
			return nil, err
		}
		inCombat := who != nil
		turns := inCombat && c.enc.Status == statusActive
		if inCombat {
			mapID = deref(c.enc.MapID)
		}
		now, err := s.vitals.GetVitalsTx(ctx, c.tx, c.session.CampaignID, characterID)
		if err != nil {
			return nil, err
		}
		sight := now.GetFamiliarSight()

		if stop {
			if sight == nil {
				return nil, errSight(playv1.FamiliarSightBlockedReason_FAMILIAR_SIGHT_BLOCKED_REASON_NOT_SEEING, "the player is not looking through the familiar's eyes")
			}
			if who != nil {
				if err := setConditions(ctx, c, *who, withoutConditions(who.Conditions, sight.GetConditionsGiven())); err != nil {
					return nil, err
				}
			}
			if vitals, err = s.vitals.SetFamiliarSight(ctx, c.tx, c.session.CampaignID, characterID, "", false, nil); err != nil {
				return nil, err
			}
			made = actionEvent{Sight: "stop", Creature: sight.GetCreatureId(), Reason: sightStopped, SightConds: sight.GetConditionsGiven()}
		} else {
			if sight != nil {
				return nil, errSight(playv1.FamiliarSightBlockedReason_FAMILIAR_SIGHT_BLOCKED_REASON_ALREADY_SEEING, "the player is looking through the familiar's eyes already")
			}
			fam, ok, err := s.vitals.FamiliarOf(ctx, c.tx, c.session.CampaignID, characterID)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, errSight(playv1.FamiliarSightBlockedReason_FAMILIAR_SIGHT_BLOCKED_REASON_NO_FAMILIAR, "the character has no familiar")
			}
			if inCombat && isTheatre(c.enc) {
				return nil, errSight(playv1.FamiliarSightBlockedReason_FAMILIAR_SIGHT_BLOCKED_REASON_NO_MAP, "a combat without a map has no vision to look through") // the familiar's eyes are the map's vision: without a map there is nothing to see through them (RN-25)
			}
			if inCombat && !turns {
				// In SETUP nobody has a turn: the sight is an action of a turn, and one
				// that was on is ended when the combat starts or the character joins it.
				return nil, errSight(playv1.FamiliarSightBlockedReason_FAMILIAR_SIGHT_BLOCKED_REASON_COMBAT_NOT_BEGUN, "the combat has not begun")
			}
			me, it, found, err := s.familiarSquares(ctx, c, who, cs, characterID, fam.ID)
			if err != nil {
				return nil, err
			}
			if !found {
				return nil, errSight(playv1.FamiliarSightBlockedReason_FAMILIAR_SIGHT_BLOCKED_REASON_NOT_ON_MAP, "the character and the familiar are not both on the map")
			}
			if !withinSightRange(me, it) {
				return nil, errSight(playv1.FamiliarSightBlockedReason_FAMILIAR_SIGHT_BLOCKED_REASON_TOO_FAR, "the familiar is more than 30 m away")
			}
			if turns {
				if err := s.spendAction(ctx, c, v, *who, false, &made); err != nil {
					return nil, err
				}
			}
			var added []string
			if who != nil {
				var all []string
				all, added = withConditions(who.Conditions, sightConditions)
				if err := setConditions(ctx, c, *who, all); err != nil {
					return nil, err
				}
			}
			if vitals, err = s.vitals.SetFamiliarSight(ctx, c.tx, c.session.CampaignID, characterID, fam.ID, turns, added); err != nil {
				return nil, err
			}
			made.Sight, made.Creature, made.SightConds = "start", fam.ID, added
		}
		made.OwnerCharacter = characterID
		if inCombat {
			made.EncounterID = c.enc.ID
			if made.Actor == "" {
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
		return nil, nil, s.dbError(ctx, "change the familiar's sight", err)
	}
	ev, err := resultEvent(res, made)
	if err != nil {
		return nil, nil, s.dbError(ctx, "read the familiar's sight change", err)
	}
	if res.repeated && ev.OwnerCharacter != characterID {
		// The key is another character's change: never answer with that character's vitals.
		return nil, nil, connect.NewError(connect.CodeInvalidArgument, errors.New("idempotency_key was already used for another change"))
	}
	if res.repeated {
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
		s.maps.VisionChanged(ctx, m.CampaignID, mapID)
	}
	return vitals, out, nil
}

// endFamiliarSights ends, at the start of a turn, the sight of every character
// on turn that began it in a combat (MR-036): the vitals forget it, the combatant
// loses the conditions it gave, and a familiar_sight event says why. It runs inside
// the transaction that starts the turn; what it changed is told after the commit.
func (s *Service) endFamiliarSights(ctx context.Context, c *combatTx, ids []string) error {
	cs, err := c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return fmt.Errorf("list the combatants: %w", err)
	}
	for _, id := range ids {
		if i := slices.IndexFunc(cs, func(o playdb.Combatant) bool { return o.ID == id }); i >= 0 {
			if err := s.endSight(ctx, c, cs[i], sightTurnStart, true); err != nil {
				return err
			}
		}
	}
	return nil
}

// endSight ends one player's character's familiar sight, if it has one (and, with
// onlyInCombat, if it began in a combat): the conditions it gave leave the combatant,
// the vitals forget it and a familiar_sight event says why.
func (s *Service) endSight(ctx context.Context, c *combatTx, who playdb.Combatant, reason string, onlyInCombat bool) error {
	if who.Kind != kindPlayer {
		return nil
	}
	now, err := s.vitals.GetVitalsTx(ctx, c.tx, c.session.CampaignID, who.CharacterID)
	if err != nil {
		if connect.CodeOf(err) == connect.CodeNotFound {
			return nil // the character died or left meanwhile
		}
		return err
	}
	sight := now.GetFamiliarSight()
	if sight == nil || (onlyInCombat && !sight.GetInCombat()) {
		return nil
	}
	if err := setConditions(ctx, c, who, withoutConditions(who.Conditions, sight.GetConditionsGiven())); err != nil {
		return err
	}
	after, err := s.vitals.SetFamiliarSight(ctx, c.tx, c.session.CampaignID, who.CharacterID, "", false, nil)
	if err != nil {
		return err
	}
	keep := c.characterID
	defer func() { c.characterID = keep }()
	c.characterID = &who.CharacterID
	if err := insertEvent(ctx, c, eventFamiliarSight, &c.actorUserID, nil, actionEvent{
		Round: c.enc.Round, Secret: who.Hidden, Actor: who.ID, OwnerCharacter: who.CharacterID, Sight: "stop",
		Creature: sight.GetCreatureId(), Reason: reason, EncounterID: c.enc.ID,
	}); err != nil {
		return err
	}
	c.told = append(c.told, after)
	return nil
}

// sightUndo takes back the start of a familiar's sight in a combat (the master's
// undo): the action comes back, the conditions it gave go away, and the player looks
// through the character's eyes again.
func (s *Service) sightUndo(ctx context.Context, c *combatTx, ev actionEvent, find func(string) (playdb.Combatant, bool), keep func(*playv1.CharacterVitals)) error {
	who, ok := find(ev.Actor)
	if ok {
		if ev.Spent {
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
		if err := setConditions(ctx, c, who, withoutConditions(who.Conditions, ev.SightConds)); err != nil {
			return err
		}
	}
	after, err := s.vitals.SetFamiliarSight(ctx, c.tx, c.session.CampaignID, ev.OwnerCharacter, "", false, nil)
	if err != nil {
		return err
	}
	keep(after)
	c.told = append(c.told, after)
	return nil
}

// endCombatSights ends, when a combat ends, the sight every character began in it:
// the next turn that would end it never comes. The conditions the sight gave leave the
// combatants and the vitals forget it; what changed is told after the commit (write),
// and no event is needed: the combat's own end says it.
func (s *Service) endCombatSights(ctx context.Context, c *combatTx, cs []playdb.Combatant) error {
	for _, who := range cs {
		if who.Kind != kindPlayer {
			continue
		}
		now, err := s.vitals.GetVitalsTx(ctx, c.tx, c.session.CampaignID, who.CharacterID)
		if err != nil {
			if connect.CodeOf(err) == connect.CodeNotFound {
				continue // the character died or left meanwhile
			}
			return err
		}
		sight := now.GetFamiliarSight()
		if sight == nil || !sight.GetInCombat() {
			continue
		}
		if err := setConditions(ctx, c, who, withoutConditions(who.Conditions, sight.GetConditionsGiven())); err != nil {
			return err
		}
		after, err := s.vitals.SetFamiliarSight(ctx, c.tx, c.session.CampaignID, who.CharacterID, "", false, nil)
		if err != nil {
			return err
		}
		c.told = append(c.told, after)
	}
	return nil
}
