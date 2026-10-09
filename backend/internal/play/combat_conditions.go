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
)

// Conditions and concentration (RN-22, Etapa 6, slice 6.4b): labels the master
// marks and the app reminds the table about; the engine applies no effect. A
// concentration spell is set by CastSpell (a second one replaces the first); here
// the master, or the character's own player, ends it. When damage reaches a
// concentrating combatant the damage's answer carries the DC to keep it
// (combat.ConcentrationDC).

// maxConditions is how many conditions a combatant may carry: the CHECK of
// combatants.conditions says the same.
const maxConditions = 20

// SetCombatantConditions implements playv1connect.CombatServiceHandler.
func (s *Service) SetCombatantConditions(
	ctx context.Context,
	req *connect.Request[playv1.SetCombatantConditionsRequest],
) (*connect.Response[playv1.SetCombatantConditionsResponse], error) {
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
	combID, err := parseCombatID(req.Msg.GetCombatantId(), "combatant")
	if err != nil {
		return nil, err
	}
	v := viewerOf(m)
	setConditions := req.Msg.Conditions != nil
	endConcentration := req.Msg.GetEndConcentration()
	if !setConditions && !endConcentration {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("set conditions or end_concentration"))
	}
	var conditions []string
	if setConditions {
		if conditions, err = s.cleanConditions(req.Msg.GetConditions().GetKeys()); err != nil {
			return nil, err
		}
	}

	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventConditionsSet, encounterID: encID}, func(c *combatTx) (any, error) {
		if err := notEnded(c.enc); err != nil {
			return nil, err
		}
		cs, err := c.q.ListCombatants(ctx, c.enc.ID)
		if err != nil {
			return nil, fmt.Errorf("list the combatants: %w", err)
		}
		v = c.viewer(m, cs) // the fog: an NPC the player does not see is not found
		target, err := findCombatant(cs, combID, v)
		if err != nil {
			return nil, err
		}
		if err := v.mayAct(target); err != nil {
			return nil, err
		}
		// A player ends their own concentration; the labels are the master's.
		if setConditions && !v.master {
			return nil, connect.NewError(connect.CodePermissionDenied, errors.New("only the master marks conditions"))
		}
		ev := actionEvent{Round: c.enc.Round, Secret: target.Hidden, Actor: target.ID}
		changed := false
		if setConditions && !slices.Equal(conditions, target.Conditions) {
			// A creature immune to a condition cannot suffer it (SRD 5.1, Monsters): the master is
			// told which, and a condition it already had stays on it.
			for _, k := range conditions {
				if slices.Contains(target.Conditions, k) {
					continue
				}
				immune, err := s.conditionImmune(ctx, c.tx, m.CampaignID, target, k)
				if err != nil {
					return nil, err
				}
				if immune {
					return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_CONDITION_IMMUNE, "the creature is immune to this condition",
						func(b *playv1.EncounterBlocked) { b.ConditionKey = k })
				}
			}
			if err := c.q.SetCombatantConditions(ctx, playdb.SetCombatantConditionsParams{ID: target.ID, Conditions: conditions}); err != nil {
				return nil, fmt.Errorf("set the conditions: %w", err)
			}
			ev.CondSet, ev.Conditions, ev.CondBefore = true, conditions, target.Conditions
			changed = true
			// A druid that falls unconscious is itself again (SRD).
			if err := s.endFormIfAsleep(ctx, c, target, conditions); err != nil {
				return nil, err
			}
		}
		if endConcentration && target.ConcentrationSpell != nil {
			// The creatures that lasted only while it did leave with it (MR-037, RN-22).
			if err := s.stopConcentrating(ctx, c, target, &ev); err != nil {
				return nil, err
			}
			changed = true
		}
		if !changed {
			return nil, nil // nothing differs from what is stored: no event
		}
		if c.enc, err = c.q.TouchEncounter(ctx, c.enc.ID); err != nil {
			return nil, fmt.Errorf("touch the encounter: %w", err)
		}
		c.characterID = &target.CharacterID
		ev.Round = c.enc.Round
		return ev, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "set conditions", err)
	}
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChangedFor(ctx, m.CampaignID, d, combID)
		i := slices.IndexFunc(d.cs, func(c playdb.Combatant) bool { return c.ID == combID })
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, i >= 0 && !d.cs[i].Hidden)
		if i >= 0 && endConcentration && d.cs[i].UserID != nil {
			s.publishCreaturesChanged(m.CampaignID, *d.cs[i].UserID)
		}
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.SetCombatantConditionsResponse{Encounter: out}), nil
}

// cleanConditions checks a list of condition keys: SRD conditions, each once, at
// most maxConditions; it returns them in the order given.
func (s *Service) cleanConditions(keys []string) ([]string, error) {
	if len(keys) > maxConditions {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("conditions must have at most %d keys", maxConditions))
	}
	out := make([]string, 0, len(keys))
	for i, k := range keys {
		if _, ok := s.conditionNames[k]; !ok {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("conditions.keys[%d] is not one of the SRD's conditions", i))
		}
		if slices.Contains(out, k) {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("conditions.keys[%d] repeats a condition", i))
		}
		out = append(out, k)
	}
	return out, nil
}
