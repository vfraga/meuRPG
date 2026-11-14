package play

import (
	"context"
	"errors"
	"fmt"
	"math"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/reaction"
)

// An NPC's Parry is decided by the server, with no window (RN-10, RN-31): a window would tell the players, by the
// wait, which NPCs have a reaction.
// When a melee weapon attack hits an NPC that can parry, and the Parry's bonus turns the hit into a miss, the server
// spends the NPC's reaction and the attack is a miss.
// When the bonus would not change the outcome (or the NPC cannot react, or does not see the attacker), nothing
// happens and the hit goes on.
// A critical hit is never parried: it hits whatever the armor class. Neither is a hit made with Stroke of Luck.
// The reaction comes back at the creature's turn.
// The master can switch a creature to "Perguntar" (SetMonsterReactionMode): its Parry then opens the window of
// combat_reaction_parry.go.

// parryUsed is the Parry the server used on an attack: the event of its line, written before the attack's own.
type parryUsed struct {
	Event string `json:"event"`
}

// parryAsks says the master asked to answer the creature's Parry in a window. The view, the attack and the write
// of SetMonsterReactionMode read the same column, combatants.creature_spent; the write reads it again inside its own
// transaction, and the attack reads the combatant its transaction loaded under the session's lock.
func parryAsks(who playdb.Combatant) bool {
	return parseSpent(who.CreatureSpent).count(spentParryAsk) == 1
}

// autoParry parries for the target when its Parry turns the hit into a miss. It runs after the roll is resolved and
// before the attack's event: the result becomes a miss, so no damage opens and no rider is offered.
func (f *attackFlow) autoParry(ctx context.Context) error {
	if !f.result.Hit || f.result.Critical || f.rolled.Stroke != nil || f.target.Kind != kindNPC || parryAsks(f.target) {
		return nil
	}
	base, bonus, ok, err := f.parryApplies(ctx)
	if err != nil || !ok {
		return err
	}
	return f.parryHit(ctx, base, bonus)
}

// parryApplies says the target's Parry turns this hit into a miss, with the armor class the hit was compared with and
// the Parry's bonus. The cheap checks come first: an ordinary monster has no Parry, and one read of its stat block
// settles it before the kit's other queries.
func (f *attackFlow) parryApplies(ctx context.Context) (base, bonus int32, ok bool, err error) {
	st, err := f.s.roster.ReactionStats(ctx, f.c.tx, f.m.CampaignID, f.target.CharacterID)
	if connect.CodeOf(err) == connect.CodeNotFound {
		return 0, 0, false, nil
	}
	if err != nil {
		return 0, 0, false, err
	}
	if _, has := parryOf(st); !has {
		return 0, 0, false, nil
	}
	k, ok, err := f.s.kitOf(ctx, f.c.tx, f.m.CampaignID, f.target)
	if err != nil || !ok || !k.canReact() {
		return 0, 0, false, err
	}
	parry, has := parryOf(k.st)
	if !has || len(k.slotsFor(spellShield)) > 0 { // a Shield in the stat block keeps its window
		return 0, 0, false, nil
	}
	base, bonus = clamp32(f.targetAC, 0, math.MaxInt32), clamp32(parry.ACBonus, 0, math.MaxInt32)
	if conv.Int32(f.result.Total) >= base+bonus {
		return 0, 0, false, nil
	}
	ok, err = f.parryCanAnswer(ctx)
	return base, bonus, ok, err
}

// parryCanAnswer says the blow is a melee weapon attack and the target sees its attacker.
func (f *attackFlow) parryCanAnswer(ctx context.Context) (bool, error) {
	melee, err := f.s.meleeWeaponUse(ctx, f.c, f.attacker, f.target, f.attackKey)
	if err != nil || !melee {
		return false, err
	}
	_, sees, err := f.s.reactorSees(ctx, f.c, f.cs, f.target, f.attacker)
	return sees, err
}

// parryHit spends the target's reaction, writes the line and turns the hit into a miss.
func (f *attackFlow) parryHit(ctx context.Context, base, bonus int32) error {
	if err := f.s.spendReaction(ctx, f.c, f.target, reaction.ParryKind); err != nil {
		return err
	}
	id, err := insertEventID(ctx, f.c, eventReactionUsed, &f.c.actorUserID, nil, actionEvent{
		Round: f.c.enc.Round, Secret: f.target.Hidden, AttackerHidden: f.attacker.Hidden, Actor: f.target.ID, Target: f.attacker.ID,
		Stopped: true, ReactionBefore: f.target.ReactionUsed,
		Reaction: &reactionEvent{Kind: string(reaction.ParryKind), Used: true, Stopped: true, Auto: true, Before: conv.Int32(f.result.Total), AC: base, Bonus: bonus},
	})
	if err != nil {
		return err
	}
	f.result.Hit = false
	f.parried = &parryUsed{Event: id}
	return nil
}

// parryBack gives the target the reaction its automatic Parry spent, when the attack is undone.
func (u *undoer) parryBack(ctx context.Context, ev actionEvent) error {
	if ev.Parried == nil {
		return nil
	}
	who, ok := u.find(ev.Target)
	if !ok {
		return nil
	}
	return u.setEconomy(ctx, who, who.ActionUsed, who.BonusActionUsed, false, who.Dashed)
}

// SetMonsterReactionMode implements playv1connect.CombatServiceHandler.
func (s *Service) SetMonsterReactionMode(
	ctx context.Context,
	req *connect.Request[playv1.SetMonsterReactionModeRequest],
) (*connect.Response[playv1.SetMonsterReactionModeResponse], error) {
	call, err := readMasterTargetCall(ctx, req.Msg.GetCampaignId(), req.Msg.GetIdempotencyKey(), req.Msg.GetEncounterId(), req.Msg.GetCombatantId())
	if err != nil {
		return nil, err
	}
	m, ask := call.m, req.Msg.GetAsk()
	res, err := s.write(ctx, combatWrite{m: m, key: call.key, hash: idem.Hash(req.Msg), kind: eventMonsterReactionSet, encounterID: call.encID}, func(c *combatTx) (any, error) {
		return s.reactionModeWrite(ctx, c, call, ask)
	})
	if err != nil {
		return nil, s.dbError(ctx, "set a monster's reaction mode", err)
	}
	out, err := s.finish(ctx, m, res, func(_ context.Context, d *encounterData) { s.publishEncounterChangedToMaster(ctx, m.CampaignID, d.enc) })
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.SetMonsterReactionModeResponse{Encounter: out}), nil
}

// reactionModeWrite is the change of SetMonsterReactionMode: only an NPC whose stat block has a Parry has a mode.
func (s *Service) reactionModeWrite(ctx context.Context, c *combatTx, call masterTargetCall, ask bool) (any, error) {
	if err := notEnded(c.enc); err != nil {
		return nil, err
	}
	target, err := s.masterTargetOf(ctx, c, call)
	if err != nil {
		return nil, err
	}
	if target.Kind != kindNPC {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("only an NPC has a Parry mode"))
	}
	st, err := s.roster.ReactionStats(ctx, c.tx, c.session.CampaignID, target.CharacterID)
	if err != nil {
		return nil, fmt.Errorf("read the stat block: %w", err)
	}
	if _, ok := parryOf(st); !ok {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("this combatant has no Parry"))
	}
	spent, err := readCreatureSpent(ctx, c.q, target.ID)
	if err != nil {
		return nil, err
	}
	if ask {
		spent[spentParryAsk] = []byte("1")
	} else {
		delete(spent, spentParryAsk)
	}
	if err := writeCreatureSpent(ctx, c.q, target.ID, spent); err != nil {
		return nil, err
	}
	if err := touch(ctx, c); err != nil {
		return nil, err
	}
	c.characterID = &target.CharacterID
	return actionEvent{Round: c.enc.Round, Secret: true, Actor: target.ID, ParryAsk: ask}, nil
}
