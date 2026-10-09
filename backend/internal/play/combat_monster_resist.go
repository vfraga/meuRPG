package play

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// AnswerLegendaryResistance implements playv1connect.CreatureServiceHandler.
func (s *Service) AnswerLegendaryResistance(
	ctx context.Context,
	req *connect.Request[playv1.AnswerLegendaryResistanceRequest],
) (*connect.Response[playv1.AnswerLegendaryResistanceResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
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
	combID, err := parseCombatID(req.Msg.GetCombatantId(), "combatant")
	if err != nil {
		return nil, err
	}
	castID, err := parseCombatID(req.Msg.GetCastId(), "cast")
	if err != nil {
		return nil, err
	}
	use := req.Msg.GetUse()

	var made actionEvent
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventLegendaryResistance, encounterID: encID}, func(c *combatTx) (any, error) {
		made = actionEvent{}
		if err := notEnded(c.enc); err != nil {
			return nil, err
		}
		cs, err := c.q.ListCombatants(ctx, c.enc.ID)
		if err != nil {
			return nil, fmt.Errorf("list the combatants: %w", err)
		}
		who, err := findCombatant(cs, combID, combatViewer{master: true})
		if err != nil {
			return nil, err
		}
		mon, err := s.monsterOf(ctx, c.tx, m.CampaignID, who)
		if err != nil {
			return nil, err
		}
		if mon == nil {
			return nil, errNotAMonster()
		}
		if mon.plan.LegendaryResistance == 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("the creature has no Legendary Resistance"))
		}
		st, err := monsterStateOf(who)
		if err != nil {
			return nil, err
		}
		before := st.Clone()
		c.characterID = &who.CharacterID
		if !use { // "Deixar falhar": the prompt goes, the save stays failed
			st.DropPrompt(castID)
			if !statesEqual(before, st) {
				if err := saveMonsterState(ctx, c, who, st); err != nil {
					return nil, err
				}
			}
			c.kind = eventCombatantCheck
			if c.enc, err = c.q.TouchEncounter(ctx, c.enc.ID); err != nil {
				return nil, fmt.Errorf("touch the encounter: %w", err)
			}
			made = actionEvent{Round: c.enc.Round, Secret: true, Actor: who.ID, CastID: castID, Monster: &monsterEvent{Passed: true, Left: clamp32(st.ResistanceLeft(mon.plan), 0, 10)}}
			return made, nil
		}
		if err := st.SpendResistance(mon.plan); err != nil {
			return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_LEGENDARY_RESISTANCE_SPENT, "the creature has used all its Legendary Resistance")
		}
		st.DropPrompt(castID)

		// The cast whose save it failed, among the session's latest events.
		recent, err := c.q.ListRecentSessionEvents(ctx, playdb.ListRecentSessionEventsParams{GameSessionID: c.session.ID, Limit: recentEvents})
		if err != nil {
			return nil, fmt.Errorf("read the latest events: %w", err)
		}
		var cast *actionEvent
		for _, e := range recent {
			ev, err := readEvent(e.Payload)
			if err != nil {
				continue
			}
			switch {
			case e.Kind == eventSpellCast && ev.CastID == castID && e.EncounterID != nil && *e.EncounterID == c.enc.ID:
				cast = &ev
			case e.Kind == eventLegendaryResistance && ev.Monster != nil && ev.Monster.Resist != nil && ev.Monster.Resist.Cast == castID && ev.Actor == who.ID:
				return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("the creature used its Legendary Resistance on this cast already"))
			}
		}
		if cast == nil {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("cast not found"))
		}
		i := slices.IndexFunc(cast.Hits, func(h castHit) bool { return h.Target == who.ID && h.Save != nil })
		if i < 0 {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("the cast has no saving throw of the creature"))
		}
		hit := cast.Hits[i]
		if hit.Save.Saved {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("the creature's saving throw was not failed"))
		}

		// The damage of the failed save, while it has not been rolled: half of it, or none.
		resist := &resistEv{Cast: castID, Left: clamp32(st.ResistanceLeft(mon.plan), 0, 10)}
		made = actionEvent{
			Round: c.enc.Round, Secret: true, Actor: who.ID, Key: cast.Key, CastID: castID, Monster: &monsterEvent{Resist: resist},
		}
		for _, id := range append([]string{hit.Pending}, hit.More...) {
			if id == "" {
				continue
			}
			p, err := c.q.GetPendingDamage(ctx, playdb.GetPendingDamageParams{EncounterID: c.enc.ID, ID: id})
			if err != nil {
				continue // an undo took it away
			}
			if p.Status != pendingAwaitingRoll {
				return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_SAVE_SETTLED, "the damage of the failed save was rolled already")
			}
			switch hit.Save.OnSuccess {
			case "half":
				if _, err := c.q.SetPendingDamageHalf(ctx, playdb.SetPendingDamageHalfParams{ID: p.ID, Status: pendingAwaitingRoll, Half: true}); err != nil {
					return nil, fmt.Errorf("halve the pending damage: %w", err)
				}
			case "none":
				if _, err := c.q.SetPendingDamageHalf(ctx, playdb.SetPendingDamageHalfParams{ID: p.ID, Status: pendingDiscarded, Half: false, ResolvedAt: &c.now}); err != nil {
					return nil, fmt.Errorf("drop the pending damage: %w", err)
				}
			default: // the stat block leaves a success to the table: the master rules on the amount
				continue
			}
			resist.Pending = append(resist.Pending, resistedPending{ID: p.ID, Status: p.Status, Half: p.Half})
		}
		// The condition the failure gave comes off.
		if ch := slices.IndexFunc(cast.Monster.conds(), func(ch condChange) bool { return ch.Target == who.ID }); ch >= 0 {
			now := slices.Clone(who.Conditions)
			if err := c.q.SetCombatantConditions(ctx, playdb.SetCombatantConditionsParams{ID: who.ID, Conditions: nonNil(cast.Monster.Conds[ch].Before)}); err != nil {
				return nil, fmt.Errorf("take the condition off: %w", err)
			}
			made.Monster.Conds = append(made.Monster.Conds, condChange{Target: who.ID, Before: now})
		}
		if err := saveMonsterState(ctx, c, who, st); err != nil {
			return nil, err
		}
		made.Monster.Before = &before
		if c.enc, err = c.q.TouchEncounter(ctx, c.enc.ID); err != nil {
			return nil, fmt.Errorf("touch the encounter: %w", err)
		}
		return made, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "answer legendary resistance", err)
	}
	ev, err := resultEvent(res, made)
	if err != nil {
		return nil, s.dbError(ctx, "read the legendary resistance", err)
	}
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, use) // the players' line of the cast says the creature saved
	})
	if err != nil {
		return nil, err
	}
	left := int32(0)
	switch {
	case ev.Monster != nil && ev.Monster.Resist != nil:
		left = ev.Monster.Resist.Left
	case ev.Monster != nil:
		left = ev.Monster.Left
	}
	return connect.NewResponse(&playv1.AnswerLegendaryResistanceResponse{Encounter: out, UsesLeft: left}), nil
}

// DeclineLegendary implements playv1connect.CreatureServiceHandler.
func (s *Service) DeclineLegendary(
	ctx context.Context,
	req *connect.Request[playv1.DeclineLegendaryRequest],
) (*connect.Response[playv1.DeclineLegendaryResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
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
	combID, err := parseCombatID(req.Msg.GetCombatantId(), "combatant")
	if err != nil {
		return nil, err
	}
	var made actionEvent
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventCombatantCheck, encounterID: encID}, func(c *combatTx) (any, error) {
		made = actionEvent{}
		if err := notEnded(c.enc); err != nil {
			return nil, err
		}
		cs, err := c.q.ListCombatants(ctx, c.enc.ID)
		if err != nil {
			return nil, fmt.Errorf("list the combatants: %w", err)
		}
		who, err := findCombatant(cs, combID, combatViewer{master: true})
		if err != nil {
			return nil, err
		}
		st, err := monsterStateOf(who)
		if err != nil {
			return nil, err
		}
		if st.Offer != nil { // an offer already gone is let pass already
			st.Offer = nil
			if err := saveMonsterState(ctx, c, who, st); err != nil {
				return nil, err
			}
		}
		if c.enc, err = c.q.TouchEncounter(ctx, c.enc.ID); err != nil {
			return nil, fmt.Errorf("touch the encounter: %w", err)
		}
		c.characterID = &who.CharacterID
		made = actionEvent{Round: c.enc.Round, Secret: true, Actor: who.ID, Monster: &monsterEvent{Passed: true}}
		return made, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "let a legendary action pass", err)
	}
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.DeclineLegendaryResponse{Encounter: out}), nil
}

// openResistancePrompts writes down, for each creature of a cast or an action that failed its
// saving throw and still has Legendary Resistance, the prompt the master answers (the view reads
// it back while the damage of the failure is not rolled).
func (s *Service) openResistancePrompts(ctx context.Context, c *combatTx, caster playdb.Combatant, cs []playdb.Combatant, made *actionEvent) error {
	for _, h := range made.Hits {
		if h.Save == nil || h.Save.Saved {
			continue
		}
		i := slices.IndexFunc(cs, func(x playdb.Combatant) bool { return x.ID == h.Target })
		if i < 0 {
			continue
		}
		target := cs[i]
		if target.Kind != kindNPC {
			continue
		}
		mon, err := s.monsterOf(ctx, c.tx, c.session.CampaignID, target)
		if err != nil {
			return err
		}
		if mon == nil || mon.plan.LegendaryResistance == 0 {
			continue
		}
		st, err := monsterStateOf(target)
		if err != nil {
			return err
		}
		if st.ResistanceLeft(mon.plan) < 1 {
			continue
		}
		st.AddPrompt(combat.ResistPrompt{
			Cast: made.CastID, Spell: made.Key, Caster: caster.ID, Ability: h.Save.Ability,
			DC: int(h.Save.DC), D20: int(h.Save.D20), Bonus: int(h.Save.Bonus), Total: int(h.Save.Total),
			Pending: slices.DeleteFunc(append([]string{h.Pending}, h.More...), func(id string) bool { return id == "" }),
		})
		if err := saveMonsterState(ctx, c, target, st); err != nil {
			return err
		}
	}
	return nil
}

// conds are the conditions an event's action gave; nil for an event that kept no monster part.
func (m *monsterEvent) conds() []condChange {
	if m == nil {
		return nil
	}
	return m.Conds
}

// RollCombatantCheck implements playv1connect.MonsterServiceHandler.
func (s *Service) RollCombatantCheck(
	ctx context.Context,
	req *connect.Request[playv1.RollCombatantCheckRequest],
) (*connect.Response[playv1.RollCombatantCheckResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
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
	combID, err := parseCombatID(req.Msg.GetCombatantId(), "combatant")
	if err != nil {
		return nil, err
	}
	skill := req.Msg.GetSkillKey()
	ability := req.Msg.GetAbility()
	if (skill == "") == (ability == rulesv1Unspecified) {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("set exactly one of skill_key and ability"))
	}
	var abilityKey rules.Ability
	if skill == "" {
		var ok bool
		if abilityKey, ok = abilityFromProto[ability]; !ok {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("ability is not one of the six abilities"))
		}
	}

	var made actionEvent
	var checkPT string
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventCombatantCheck, encounterID: encID}, func(c *combatTx) (any, error) {
		made, checkPT = actionEvent{}, ""
		if err := notEnded(c.enc); err != nil {
			return nil, err
		}
		cs, err := c.q.ListCombatants(ctx, c.enc.ID)
		if err != nil {
			return nil, fmt.Errorf("list the combatants: %w", err)
		}
		who, err := findCombatant(cs, combID, combatViewer{master: true})
		if err != nil {
			return nil, err
		}
		mon, err := s.monsterOf(ctx, c.tx, m.CampaignID, who)
		if err != nil {
			return nil, err
		}
		if mon == nil {
			return nil, errNotAMonster()
		}
		d, _ := mon.content.MonsterDerived(mon.key)
		var bonus int
		var checkKey string
		if skill != "" {
			i := slices.IndexFunc(d.Skills, func(sk rules.Skill) bool { return sk.Key == skill })
			if i < 0 {
				return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("skill_key is not an SRD skill"))
			}
			bonus, checkKey = d.Skills[i].Bonus, skill
			checkPT = d.Skills[i].NamePT
		} else {
			i := slices.IndexFunc(d.Abilities, func(a rules.AbilityScore) bool { return a.Ability == abilityKey })
			bonus, checkKey, checkPT = d.Abilities[i].Modifier, string(abilityKey), d.Abilities[i].NamePT
		}
		face, roll, err := s.d20(rollInput{inApp: true}, bonus)
		if err != nil {
			return nil, err
		}
		made = actionEvent{
			Round: c.enc.Round, Secret: true, Actor: who.ID,
			Monster: &monsterEvent{Check: &checkEv{Key: checkKey, D20: clamp32(face, 1, 20), Bonus: clamp32(bonus, -100, 100), Total: clamp32(roll.Total, -100, 200)}},
		}
		if c.enc, err = c.q.TouchEncounter(ctx, c.enc.ID); err != nil {
			return nil, fmt.Errorf("touch the encounter: %w", err)
		}
		c.characterID = &who.CharacterID
		return made, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "roll a check", err)
	}
	ev, err := resultEvent(res, made)
	if err != nil {
		return nil, s.dbError(ctx, "read the check", err)
	}
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, false) // a monster's check is the master's
	})
	if err != nil {
		return nil, err
	}
	resp := &playv1.RollCombatantCheckResponse{Encounter: out}
	if ck := ev.Monster.checkOf(); ck != nil {
		resp.D20, resp.Bonus, resp.Total = ck.D20, ck.Bonus, ck.Total
		resp.CheckPt = checkPT
		if checkPT == "" { // a retry: the name is read again
			resp.CheckPt = s.checkNamePT(ctx, m.CampaignID, ck.Key)
		}
	}
	return connect.NewResponse(resp), nil
}

func (m *monsterEvent) checkOf() *checkEv {
	if m == nil {
		return nil
	}
	return m.Check
}

// checkNamePT is the Portuguese name of a check by its key ("skill:perception", "str").
func (s *Service) checkNamePT(ctx context.Context, campaignID, key string) string {
	return s.namesFor(ctx, campaignID)(key)
}
