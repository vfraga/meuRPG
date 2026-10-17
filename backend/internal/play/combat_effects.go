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

// Lasting effects on a combatant that end (SRD 5.1): Escudo's +5 armor class
// "until the start of your next turn" ends by itself when the caster's turn
// starts, and Ajuda's hit point bonus ends when the master ends it (the app does
// not count the spell's 8 hours) or at a long rest. Each ending is a line of the
// combat log.

// The effects an effect_end event names.
const (
	effectShield = "shield"
	effectAid    = "aid"
)

var combatEffectToProto = map[string]playv1.CombatEffect{
	effectShield: playv1.CombatEffect_COMBAT_EFFECT_SHIELD,
	effectAid:    playv1.CombatEffect_COMBAT_EFFECT_AID,
}

// effectEnd is what an event keeps of an effect that ended: which one, and, for
// Ajuda, the hit points and the maximum before and after.
type effectEnd struct {
	Effect string `json:"effect"`
	HP0    int32  `json:"hp_before,omitempty"`
	HP1    int32  `json:"hp_after,omitempty"`
	Max0   int32  `json:"max_before,omitempty"`
	Max1   int32  `json:"max_after,omitempty"`
}

// EndCombatEffect implements playv1connect.CombatServiceHandler: the master ends
// Ajuda on a combatant.
func (s *Service) EndCombatEffect(
	ctx context.Context,
	req *connect.Request[playv1.EndCombatEffectRequest],
) (*connect.Response[playv1.EndCombatEffectResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	key, encID, combID, err := readEndCombatEffect(req.Msg)
	if err != nil {
		return nil, err
	}
	v := viewerOf(m)

	var vitals *playv1.CharacterVitals
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventCombatEffectEnded, encounterID: encID}, func(c *combatTx) (any, error) {
		vitals = nil // a retry of the transaction starts over
		return s.endAidTx(ctx, c, v, combID, &vitals)
	})
	if err != nil {
		return nil, s.dbError(ctx, "end an effect", err)
	}
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
		i := slices.IndexFunc(d.cs, func(c playdb.Combatant) bool { return c.ID == combID })
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, i >= 0 && !d.cs[i].Hidden)
	})
	if err != nil {
		return nil, err
	}
	if !res.repeated {
		s.publishVitals(m.CampaignID, vitals)
	}
	return connect.NewResponse(&playv1.EndCombatEffectResponse{Encounter: out}), nil
}

// readEndCombatEffect reads the key, the combat and the combatant, and refuses an effect that is
// not Ajuda.
func readEndCombatEffect(msg *playv1.EndCombatEffectRequest) (key, encID, combID string, err error) {
	if key, err = parseKey(msg.GetIdempotencyKey()); err != nil {
		return "", "", "", err
	}
	if encID, err = parseCombatID(msg.GetEncounterId(), "encounter"); err != nil {
		return "", "", "", err
	}
	if combID, err = parseCombatID(msg.GetCombatantId(), "combatant"); err != nil {
		return "", "", "", err
	}
	if msg.GetEffect() != playv1.CombatEffect_COMBAT_EFFECT_AID {
		return "", "", "", connect.NewError(connect.CodeInvalidArgument, errors.New("effect must be AID: the other effects end by themselves"))
	}
	return key, encID, combID, nil
}

// endAidTx is the change EndCombatEffect makes inside the combat's transaction: find the
// combatant, take Ajuda off it, and answer the event; none when there was nothing to end.
func (s *Service) endAidTx(ctx context.Context, c *combatTx, v combatViewer, combID string, vitals **playv1.CharacterVitals) (any, error) {
	if err := notEnded(c.enc); err != nil {
		return nil, err
	}
	cs, err := c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return nil, fmt.Errorf("list the combatants: %w", err)
	}
	target, err := findCombatant(cs, combID, v)
	if err != nil {
		return nil, err
	}
	end, after, err := s.endAid(ctx, c, target)
	if err != nil || end == nil {
		return nil, err // nothing to end: no event
	}
	*vitals = after
	if err = touchEncounter(ctx, c); err != nil {
		return nil, err
	}
	c.characterID = &target.CharacterID
	return actionEvent{Round: c.enc.Round, Secret: target.Hidden, Actor: target.ID, EffectEnd: end}, nil
}

// endAid takes Ajuda's bonus off a combatant: the maximum falls back and the
// current hit points lose only what is above the new maximum, never the last one
// of a combatant that still has some (SRD 5.1, "Healing": hit points never pass
// the maximum; the spell says nothing else about its end). It returns what the
// event keeps and, for a player's character, its vitals after; nil end when the
// combatant has no bonus.
func (s *Service) endAid(ctx context.Context, c *combatTx, target playdb.Combatant) (*effectEnd, *playv1.CharacterVitals, error) {
	if holdsHP(target) {
		bonus := target.HpMaxBonus
		if bonus == 0 {
			return nil, nil, nil
		}
		before, maxBefore := hpOf(target), num(target.HpMax)
		maxAfter := max(maxBefore-bonus, 1)
		hp := before.HP
		if hp > 0 {
			hp = max(min(hp, maxAfter), 1)
		}
		// The current hit points first: they never pass the maximum (the table says so).
		if hp != before.HP {
			if err := c.q.SetCombatantHitPoints(ctx, playdb.SetCombatantHitPointsParams{ID: target.ID, HpCurrent: &hp, HpTemp: &before.Temp, Defeated: before.Defeated}); err != nil {
				return nil, nil, fmt.Errorf("cut the current hit points: %w", err)
			}
		}
		if err := c.q.SetCombatantHitPointsMax(ctx, playdb.SetCombatantHitPointsMaxParams{ID: target.ID, HpMax: &maxAfter, HpMaxBonus: 0}); err != nil {
			return nil, nil, fmt.Errorf("take the bonus off the maximum hit points: %w", err)
		}
		return &effectEnd{Effect: effectAid, HP0: before.HP, HP1: hp, Max0: maxBefore, Max1: maxAfter}, nil, nil
	}
	before, after, err := s.vitals.SetHitPointsMaxBonus(ctx, c.tx, c.session.CampaignID, target.CharacterID, 0)
	if err != nil {
		return nil, nil, err
	}
	if before.GetHitPointsMaxBonus() == 0 {
		return nil, nil, nil
	}
	return &effectEnd{
		Effect: effectAid, HP0: before.GetHitPointsCurrent(), HP1: after.GetHitPointsCurrent(),
		Max0: before.GetHitPointsMax(), Max1: after.GetHitPointsMax(),
	}, after, nil
}

// endShields writes the line of every Escudo that ends as the turn of ids starts
// (SRD 5.1, Shield: the +5 lasts "until the start of your next turn"). The bonus
// itself is taken off by ResetCombatantTurn.
func (s *Service) endShields(ctx context.Context, c *combatTx, ids []string) error {
	cs, err := c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return fmt.Errorf("list the combatants: %w", err)
	}
	for _, who := range cs {
		if !slices.Contains(ids, who.ID) || who.AcBonus == 0 {
			continue
		}
		if err := insertEvent(ctx, c, eventCombatEffectEnded, &c.actorUserID, nil, actionEvent{
			Round: c.enc.Round, Secret: who.Hidden, Actor: who.ID, EffectEnd: &effectEnd{Effect: effectShield},
		}); err != nil {
			return err
		}
	}
	return nil
}
