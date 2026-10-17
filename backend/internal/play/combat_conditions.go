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

// conditionsCall is what SetCombatantConditions reads from its request, checked.
type conditionsCall struct {
	m                authz.Membership
	key              string
	encounterID      string
	combatantID      string
	set              bool     // the request lists the conditions the combatant carries
	conditions       []string // checked, in the order given
	endConcentration bool
}

// SetCombatantConditions implements playv1connect.CombatServiceHandler.
func (s *Service) SetCombatantConditions(
	ctx context.Context,
	req *connect.Request[playv1.SetCombatantConditionsRequest],
) (*connect.Response[playv1.SetCombatantConditionsResponse], error) {
	call, err := s.readConditionsRequest(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	res, err := s.write(ctx, combatWrite{m: call.m, key: call.key, hash: idem.Hash(req.Msg), kind: eventConditionsSet, encounterID: call.encounterID}, func(c *combatTx) (any, error) {
		return s.conditionsWrite(ctx, c, call)
	})
	if err != nil {
		return nil, s.dbError(ctx, "set conditions", err)
	}
	out, err := s.finish(ctx, call.m, res, func(ctx context.Context, d *encounterData) {
		s.publishConditionsSet(ctx, call, d)
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.SetCombatantConditionsResponse{Encounter: out}), nil
}

// readConditionsRequest reads the request in the order the answers are given: who may call,
// the key, the encounter, the combatant, that there is something to do, then the conditions.
func (s *Service) readConditionsRequest(ctx context.Context, req *playv1.SetCombatantConditionsRequest) (conditionsCall, error) {
	m, key, encID, err := contestCaller(ctx, req.GetCampaignId(), req.GetIdempotencyKey(), req.GetEncounterId())
	if err != nil {
		return conditionsCall{}, err
	}
	call := conditionsCall{m: m, key: key, encounterID: encID, set: req.Conditions != nil, endConcentration: req.GetEndConcentration()}
	if call.combatantID, err = parseCombatID(req.GetCombatantId(), "combatant"); err != nil {
		return conditionsCall{}, err
	}
	if !call.set && !call.endConcentration {
		return conditionsCall{}, connect.NewError(connect.CodeInvalidArgument, errors.New("set conditions or end_concentration"))
	}
	if call.set {
		if call.conditions, err = s.cleanConditions(req.GetConditions().GetKeys()); err != nil {
			return conditionsCall{}, err
		}
	}
	return call, nil
}

// conditionsWrite is the transaction of SetCombatantConditions: the conditions the master
// marks, and the concentration its owner or the master ends. It writes no event when nothing
// differs from what is stored.
func (s *Service) conditionsWrite(ctx context.Context, c *combatTx, call conditionsCall) (any, error) {
	if err := notEnded(c.enc); err != nil {
		return nil, err
	}
	cs, err := listCombatants(ctx, c)
	if err != nil {
		return nil, err
	}
	v := c.viewer(call.m, cs) // the fog: an NPC the player does not see is not found
	target, err := findCombatant(cs, call.combatantID, v)
	if err != nil {
		return nil, err
	}
	if err := v.mayAct(target); err != nil {
		return nil, err
	}
	// A player ends their own concentration; the labels are the master's.
	if call.set && !v.master {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("only the master marks conditions"))
	}
	ev := actionEvent{Round: c.enc.Round, Secret: target.Hidden, Actor: target.ID}
	changed := false
	if call.set {
		if changed, err = s.markConditions(ctx, c, &target, call.conditions, &ev); err != nil {
			return nil, err
		}
	}
	if call.endConcentration && target.ConcentrationSpell != nil {
		// The creatures that lasted only while it did leave with it (MR-037, RN-22).
		if err := s.stopConcentrating(ctx, c, target, &ev); err != nil {
			return nil, err
		}
		changed = true
	}
	if !changed {
		return nil, nil // nothing differs from what is stored: no event
	}
	if err := touchEncounter(ctx, c); err != nil {
		return nil, err
	}
	c.characterID = &target.CharacterID
	ev.Round = c.enc.Round
	return ev, nil
}

// markConditions stores the conditions the master gave, when they differ from what the
// combatant carries, and tells ev what they were. It says whether it changed anything.
func (s *Service) markConditions(ctx context.Context, c *combatTx, target *playdb.Combatant, conditions []string, ev *actionEvent) (bool, error) {
	condBefore := target.Conditions
	// A condition an effect leaves goes when the master takes it off: the effect
	// ends. Exhaustion has its own levels (RN-22).
	conditions, reconciled, err := s.reconcileConditions(ctx, c, *target, conditions)
	if err != nil {
		return false, err
	}
	*target = reconciled
	if slices.Equal(conditions, target.Conditions) {
		return false, nil
	}
	if err := c.q.SetCombatantConditions(ctx, playdb.SetCombatantConditionsParams{ID: target.ID, Conditions: conditions}); err != nil {
		return false, fmt.Errorf("set the conditions: %w", err)
	}
	ev.CondSet, ev.Conditions, ev.CondBefore = true, conditions, condBefore
	// A rage ends when the barbarian is knocked unconscious (SRD 5.1); a condition
	// the master takes off by hand loses its source.
	if err := s.afterConditionsSet(ctx, c, *target, conditions); err != nil {
		return false, err
	}
	// A druid that falls unconscious is itself again (SRD).
	if err := s.endFormIfAsleep(ctx, c, *target, conditions); err != nil {
		return false, err
	}
	return true, nil
}

// publishConditionsSet tells the streams the conditions or the concentration of a combatant
// changed.
func (s *Service) publishConditionsSet(ctx context.Context, call conditionsCall, d *encounterData) {
	campaignID := call.m.CampaignID
	s.publishEncounterChangedFor(ctx, campaignID, d, call.combatantID)
	i := slices.IndexFunc(d.cs, func(c playdb.Combatant) bool { return c.ID == call.combatantID })
	s.publishLogChanged(ctx, campaignID, d.enc.ID, i >= 0 && !d.cs[i].Hidden)
	if i >= 0 && call.endConcentration && d.cs[i].UserID != nil {
		s.publishCreaturesChanged(campaignID, *d.cs[i].UserID)
	}
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
