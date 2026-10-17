package play

import (
	"context"
	"errors"
	"fmt"
	"math"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// The master's hand on an NPC's hit points (AdjustCombatantHitPoints), in phases. The call reads the request,
// runs the transaction as a sequence of phases of a flow, each one of the checks or the writes the call has
// always made in that order, and answers with the encounter. The golden of combat_actions_rest_frozen_hp_test.go
// reads it.

// The ways the master changes the hit points of an NPC; the event keeps the word.
const (
	adjustDamage = "damage"
	adjustHeal   = "heal"
	adjustSet    = "set"
)

// adjustHitPointsCall is what AdjustCombatantHitPoints reads from its request, checked.
type adjustHitPointsCall struct {
	key, encounterID, combatantID string
	// mode and amount are the change of the hit points, and temp the temporary hit points to set; the
	// mode is empty when the request only sets the temporary ones.
	mode   string
	amount int32
	temp   *int32
}

// readAdjustHitPointsCall reads the request, in the order the answers are given: the ids, the change.
func readAdjustHitPointsCall(req *playv1.AdjustCombatantHitPointsRequest) (adjustHitPointsCall, error) {
	var call adjustHitPointsCall
	var err error
	if call.key, err = parseKey(req.GetIdempotencyKey()); err != nil {
		return call, err
	}
	if call.encounterID, err = parseCombatID(req.GetEncounterId(), "encounter"); err != nil {
		return call, err
	}
	if call.combatantID, err = parseCombatID(req.GetCombatantId(), "combatant"); err != nil {
		return call, err
	}
	call.mode, call.amount = adjustChangeOf(req)
	call.temp = req.HitPointsTemporary
	return call, call.check()
}

// adjustChangeOf is the way and the number the request changes the hit points by.
func adjustChangeOf(req *playv1.AdjustCombatantHitPointsRequest) (mode string, amount int32) {
	switch change := req.GetChange().(type) {
	case *playv1.AdjustCombatantHitPointsRequest_Damage:
		return adjustDamage, change.Damage
	case *playv1.AdjustCombatantHitPointsRequest_Heal:
		return adjustHeal, change.Heal
	case *playv1.AdjustCombatantHitPointsRequest_HitPoints:
		return adjustSet, change.HitPoints
	}
	return "", 0
}

// check refuses a request that changes nothing or by a number out of range.
func (call adjustHitPointsCall) check() error {
	switch {
	case call.mode == "" && call.temp == nil:
		return connect.NewError(connect.CodeInvalidArgument, errors.New("set damage, heal, hit_points or hit_points_temporary"))
	case call.mode != "" && (call.amount < 0 || call.amount > maxHitPointChange):
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("%s must be 0 to %d", call.mode, maxHitPointChange))
	case call.temp != nil && (*call.temp < 0 || *call.temp > maxTempHitPoints):
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("hit_points_temporary must be 0 to %d", maxTempHitPoints))
	}
	return nil
}

// AdjustCombatantHitPoints implements playv1connect.CombatServiceHandler.
func (s *Service) AdjustCombatantHitPoints(
	ctx context.Context,
	req *connect.Request[playv1.AdjustCombatantHitPointsRequest],
) (*connect.Response[playv1.AdjustCombatantHitPointsResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	call, err := readAdjustHitPointsCall(req.Msg)
	if err != nil {
		return nil, err
	}
	res, err := s.write(ctx, combatWrite{m: m, key: call.key, hash: idem.Hash(req.Msg), kind: eventHitPointsAdjusted, encounterID: call.encounterID}, func(c *combatTx) (any, error) {
		return newAdjustHitPointsFlow(s, c, m, call).run(ctx)
	})
	if err != nil {
		return nil, s.dbError(ctx, "adjust a combatant's hit points", err)
	}
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChangedFor(ctx, m.CampaignID, d, call.combatantID)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, false) // the master's correction: his line only
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.AdjustCombatantHitPointsResponse{Encounter: out}), nil
}

// adjustHitPointsFlow is one run of the adjustment's transaction: what each phase reads, and the next one uses.
type adjustHitPointsFlow struct {
	s    *Service
	c    *combatTx
	m    authz.Membership
	call adjustHitPointsCall

	target playdb.Combatant
	// after are the hit points the change leaves.
	after hpState
	made  actionEvent
}

func newAdjustHitPointsFlow(s *Service, c *combatTx, m authz.Membership, call adjustHitPointsCall) *adjustHitPointsFlow {
	return &adjustHitPointsFlow{s: s, c: c, m: m, call: call}
}

// run is the transaction: the checks, the change and its event. It answers the event the write stores.
func (f *adjustHitPointsFlow) run(ctx context.Context) (any, error) {
	phases := []func(context.Context) error{f.locate, f.change, f.save, f.touch}
	for _, phase := range phases {
		if err := phase(ctx); err != nil {
			return nil, err
		}
	}
	return f.made, nil
}

// locate finds the combatant, which has to keep its hit points in the combat.
func (f *adjustHitPointsFlow) locate(ctx context.Context) error {
	if err := notEnded(f.c.enc); err != nil {
		return err
	}
	cs, err := f.c.q.ListCombatants(ctx, f.c.enc.ID)
	if err != nil {
		return fmt.Errorf("list the combatants: %w", err)
	}
	if f.target, err = findCombatant(cs, f.call.combatantID, viewerOf(f.m)); err != nil {
		return err
	}
	if !holdsHP(f.target) {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("a player's character keeps its hit points in its vitals: use AdjustCharacterVitals"))
	}
	return nil
}

// change works out the hit points the request leaves.
func (f *adjustHitPointsFlow) change(context.Context) error {
	hp, tmp, hpMax := int(num(f.target.HpCurrent)), int(num(f.target.HpTemp)), int(num(f.target.HpMax))
	amount := int(f.call.amount)
	switch f.call.mode {
	case adjustDamage:
		dmg := combat.ApplyDamage(hp, tmp, amount)
		hp, tmp = dmg.HP, dmg.TempHP
	case adjustHeal:
		hp = combat.ApplyHeal(hp, hpMax, amount).HP
	case adjustSet:
		if amount > hpMax {
			return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("hit_points must be 0 to %d", hpMax))
		}
		hp = amount
	}
	if f.call.temp != nil {
		tmp = int(*f.call.temp)
	}
	f.after = hpState{HP: clamp32(hp, 0, math.MaxInt32), Temp: clamp32(tmp, 0, math.MaxInt32), Defeated: hp == 0}
	return nil
}

// save writes the hit points.
func (f *adjustHitPointsFlow) save(ctx context.Context) error {
	if err := f.c.q.SetCombatantHitPoints(ctx, playdb.SetCombatantHitPointsParams{ID: f.target.ID, HpCurrent: &f.after.HP, HpTemp: &f.after.Temp, Defeated: f.after.Defeated}); err != nil {
		return fmt.Errorf("save the hit points: %w", err)
	}
	return nil
}

// touch moves the encounter's revision, names the character the change was about and builds the event.
func (f *adjustHitPointsFlow) touch(ctx context.Context) error {
	if err := touchEncounter(ctx, f.c); err != nil {
		return err
	}
	before := hpOf(f.target)
	f.c.characterID = &f.target.CharacterID
	f.made = actionEvent{
		Round: f.c.enc.Round, Secret: true, Actor: f.target.ID, Mode: f.call.mode, Amount: max(f.call.amount, 0),
		Delta:  f.after.HP - before.HP,
		Before: &before, After: &f.after,
	}
	return nil
}
