package play

import (
	"context"
	"errors"
	"fmt"

	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// Spending the legendary actions of a creature (SRD 5.1, "Legendary Actions"). A creature with
// legendary actions takes them only while another creature has the turn, never on its own turn,
// never when it is defeated or incapacitated, and no more than it has left; the master alone spends
// them. The three ways to spend (the text option of SpendLegendaryAction, the attack of RollAttack
// and the move of MoveCombatant) share one rule, legendarySpend: it checks and spends inside the
// transaction of the action, so an action that is refused spends nothing. What a spend leaves is
// SpendCost on the action's event, which the undo gives back.

// legendaryNow says why a creature cannot spend legendary actions at this moment, nil when it can: the
// combat is not running, it has none or is out of the fight (defeated, incapacitated, or in the
// lethargy a Haste leaves), or it has the turn. It is also the master's "usable_now".
func legendaryNow(enc playdb.Encounter, who playdb.Combatant, p rules.LegendaryProfile) error {
	switch {
	case enc.Status != statusActive:
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_ACTIVE, "the combat is not running")
	case who.Kind != kindNPC || p.PerRound == 0 || who.Defeated || cannotActCode(who) != 0:
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_LEGENDARY_UNAVAILABLE, "the creature cannot take legendary actions now")
	case actsNow(enc, who):
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_LEGENDARY_OWN_TURN, "legendary actions are taken on the turn of another creature")
	}
	return nil
}

// legendaryAsk is what a spend names: the creature and the option, and the kind the call makes.
type legendaryAsk struct {
	who playdb.Combatant
	// master says the caller is the campaign's master: the one who asked, which a replay of a held attack keeps
	// even though the player's answer released it.
	master bool
	option string
	// kind is the kind of option the call makes (rules.LegendaryKind*).
	kind string
	// actionKey, for an attack, is the stat block action the call attacks with; it must be the option's.
	actionKey string
}

// legendarySpend checks that the master may spend the option now and spends its cost, inside the action's
// transaction. It answers the option, whose cost the action's event keeps.
func (s *Service) legendarySpend(ctx context.Context, c *combatTx, ask legendaryAsk) (rules.LegendaryOption, error) {
	opt, err := s.legendaryCheck(ctx, c, ask)
	if err != nil {
		return opt, err
	}
	return opt, legendaryPay(ctx, c, ask.who, opt)
}

// legendaryPay spends the cost of an option that legendaryCheck allowed, in the same transaction.
func legendaryPay(ctx context.Context, c *combatTx, who playdb.Combatant, opt rules.LegendaryOption) error {
	_, err := bumpCreatureSpent(ctx, c.q, who.ID, spentLegendaryActions, opt.Cost)
	return err
}

// legendaryCheck is the rule of a spend, without the spend: an action that rolls first (the attack)
// checks it once, before the d20, and pays with legendaryPay in the same transaction.
func (s *Service) legendaryCheck(ctx context.Context, c *combatTx, ask legendaryAsk) (rules.LegendaryOption, error) {
	none := rules.LegendaryOption{}
	if !ask.master {
		return none, connect.NewError(connect.CodePermissionDenied, errors.New("only the master spends legendary actions"))
	}
	p, err := s.legendaryProfileOf(ctx, c, ask.who)
	if err != nil {
		return none, err
	}
	if err := legendaryNow(c.enc, ask.who, p); err != nil {
		return none, err
	}
	if err := s.legendaryTableHeld(ctx, c, ask.who); err != nil {
		return none, err
	}
	if ask.kind != rules.LegendaryKindMove { // an attack or a text option is an action: a mount that carries a rider takes none but three
		if err := s.mustNotBeLimited(ctx, c, ask.who, ""); err != nil {
			return none, err
		}
	}
	opt, err := legendaryOptionOf(p, ask)
	if err != nil {
		return none, err
	}
	spent, err := readCreatureSpent(ctx, c.q, ask.who.ID)
	if err != nil {
		return none, err
	}
	if opt.Cost > p.PerRound-spent.count(spentLegendaryActions) {
		return none, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_LEGENDARY_NONE_LEFT, "the creature has fewer legendary actions left than the option costs")
	}
	return opt, nil
}

// legendaryTableHeld refuses a spend while the table waits: the creature is surprised, a question waits for the
// master, or a reaction window or hold is open (REACTION_PENDING). Only the creature's turn and action economy are
// replaced by the legendary rules; nobody's turn goes on past a pending question.
func (s *Service) legendaryTableHeld(ctx context.Context, c *combatTx, who playdb.Combatant) error {
	if err := s.mustNotBeSurprised(ctx, c, who); err != nil {
		return err
	}
	if err := s.mustNotHold(ctx, c); err != nil {
		return err
	}
	return s.mustNotWait(ctx, c, who)
}

// legendaryOptionOf finds the option the call names, of the kind it makes (and, for an attack, with the
// action it attacks with). A move option of a creature with no square (a combat without a map, or a creature
// not placed) cannot be walked: the engine cannot resolve it, so it is offered as text (SRD 5.1) and spent
// as one.
func legendaryOptionOf(p rules.LegendaryProfile, ask legendaryAsk) (rules.LegendaryOption, error) {
	for _, o := range p.Options {
		if o.Key != ask.option {
			continue
		}
		unwalkable := o.Kind == rules.LegendaryKindMove && ask.kind == rules.LegendaryKindText && !placed(ask.who)
		if (o.Kind != ask.kind && !unwalkable) || (ask.kind == rules.LegendaryKindAttack && o.ActionKey != ask.actionKey) {
			return o, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_LEGENDARY_WRONG_KIND, "the option is not made this way")
		}
		return o, nil
	}
	return rules.LegendaryOption{}, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_LEGENDARY_OPTION_UNKNOWN, "the creature has no such legendary option")
}

// legendaryProfileOf reads the stat block of the creature: the zero profile for a combatant with none.
func (s *Service) legendaryProfileOf(ctx context.Context, c *combatTx, who playdb.Combatant) (rules.LegendaryProfile, error) {
	key, err := s.monsterKeyOf(ctx, c, who)
	if err != nil || key == "" {
		return rules.LegendaryProfile{}, err
	}
	content, err := s.contentOf(ctx, c)
	if err != nil {
		return rules.LegendaryProfile{}, err
	}
	p, _ := content.LegendaryProfile(key)
	return p, nil
}

// legendaryBack gives back the legendary actions an event spent (its undo).
func (u *undoer) legendaryBack(ctx context.Context, ev actionEvent) error {
	if ev.SpendCost == 0 {
		return nil
	}
	if _, err := bumpCreatureSpent(ctx, u.c.q, ev.Actor, spentLegendaryActions, -int(ev.SpendCost)); err != nil {
		return fmt.Errorf("give the legendary actions back: %w", err)
	}
	return nil
}

// SpendLegendaryAction implements playv1connect.CombatServiceHandler.
func (s *Service) SpendLegendaryAction(
	ctx context.Context,
	req *connect.Request[playv1.SpendLegendaryActionRequest],
) (*connect.Response[playv1.SpendLegendaryActionResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	key, encID, err := parseCall(req.Msg.GetIdempotencyKey(), req.Msg.GetEncounterId())
	if err != nil {
		return nil, err
	}
	who, err := parseCombatID(req.Msg.GetCombatantId(), "combatant")
	if err != nil {
		return nil, err
	}
	option := req.Msg.GetOptionKey()
	if option == "" || len(option) > maxActionKeyLen {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("option_key must name a legendary option of the creature"))
	}
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventLegendarySpent, encounterID: encID}, func(c *combatTx) (any, error) {
		return s.spendTextOption(ctx, c, m, who, option)
	})
	if err != nil {
		return nil, s.dbError(ctx, "spend a legendary action", err)
	}
	enc, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		// A text spend changes nothing a player sees: both hints are the master's alone, so the players
		// cannot count the spends.
		s.publishEncounterChangedToMaster(ctx, m.CampaignID, d.enc)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, false)
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.SpendLegendaryActionResponse{Encounter: enc}), nil
}

// spendTextOption is the transaction of a text option: it spends the cost and writes the master's event.
func (s *Service) spendTextOption(ctx context.Context, c *combatTx, m authz.Membership, id, option string) (actionEvent, error) {
	cs, err := listCombatants(ctx, c)
	if err != nil {
		return actionEvent{}, err
	}
	who, err := findCombatant(cs, id, c.viewer(m, cs))
	if err != nil {
		return actionEvent{}, err
	}
	opt, err := s.legendarySpend(ctx, c, legendaryAsk{who: who, master: c.master, option: option, kind: rules.LegendaryKindText})
	if err != nil {
		return actionEvent{}, err
	}
	if err := touch(ctx, c); err != nil {
		return actionEvent{}, err
	}
	// Secret: the event is the master's alone, in the log and in the stream.
	return actionEvent{Round: c.enc.Round, Secret: true, Actor: who.ID, Key: option, SpendCost: conv.Int32(opt.Cost), SpendName: opt.Name}, nil
}

// checkLegendary refuses a legendary attack that the rule refuses, before the d20 rolls, and keeps the
// option for spendLegendary. The ask names the attacker and the option, with the stat block action the
// attack is (a roll of a sheet attack that is no action of the stat block is not an option's).
func (f *attackFlow) checkLegendary(ctx context.Context) error {
	if f.call.legendary == "" {
		return nil
	}
	plan, _, err := f.s.planOfAttack(ctx, f.c, f.attacker, f.attackKey, true)
	if err != nil {
		return err
	}
	f.legendary, err = f.s.legendaryCheck(ctx, f.c, legendaryAsk{who: f.attacker, master: f.m.Role == authz.RoleMaster, option: f.call.legendary, kind: rules.LegendaryKindAttack, actionKey: plan.Key})
	return err
}

// spendLegendary pays the cost of the option checkLegendary allowed, and keeps it on the attack's event.
func (f *attackFlow) spendLegendary(ctx context.Context) error {
	if err := legendaryPay(ctx, f.c, f.attacker, f.legendary); err != nil {
		return err
	}
	f.out.made.SpendCost = conv.Int32(f.legendary.Cost)
	return nil
}

// validateLegendary refuses a legendary move that no state of the combat could make right: not the
// master's, or a jump, a forced move or a placement (a legendary move is a walk).
func (r moveRequest) validateLegendary(master bool) error {
	switch {
	case r.legendary == "":
		return nil
	case !master:
		return connect.NewError(connect.CodePermissionDenied, errors.New("only the master spends legendary actions"))
	case r.forced || r.place || r.jump != playv1.JumpKind_JUMP_KIND_UNSPECIFIED:
		return connect.NewError(connect.CodeInvalidArgument, errors.New("a legendary move is a walk: no jump, no forced move, no placement"))
	}
	return nil
}

// spendLegendaryMove pays the cost of a legendary move, in the move's transaction: a move that is
// refused after this (too far, a wall) rolls the spend back with it.
//
// The budget of a legendary move (movementBudgetDFt): the creature moves by its usual rules (its speed
// with the conditions and effects that change it, difficult terrain, walls, creatures in the way, the
// doors), but each move is made with the whole speed of its walking or flying, as at the start of a
// turn: what it walked on its own turn is not subtracted and the Dash it took does not double it, and
// what the move walks is not added to the turn's movement, which the creature's own turn resets
// anyway. So it neither consumes nor depends on the movement of the creature's turn.
func (s *Service) spendLegendaryMove(ctx context.Context, a *moveAttempt) error {
	if a.req.legendary == "" {
		return nil
	}
	if !placed(a.target) {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_PLACED, "a legendary move starts from a square of the map")
	}
	opt, err := s.legendarySpend(ctx, a.c, legendaryAsk{who: a.target, master: a.v.master, option: a.req.legendary, kind: rules.LegendaryKindMove})
	if err != nil {
		return err
	}
	a.spent, a.speedPct = conv.Int32(opt.Cost), opt.MoveSpeedPercent()
	return nil
}

// movementBudgetDFt is the movement the mover can walk now, in tenths of a foot: what is left of its
// turn's on the floor; for a readied move, the most of its own budget on any surface (readiedMoveDFt); for a legendary
// move, a whole speed of its own (or the half the option says).
func (a *moveAttempt) movementBudgetDFt() int {
	if a.readied != nil {
		return a.readiedMoveDFt()
	}
	if a.req.legendary == "" {
		return movementLeftAtDFt(a.target, grid.SurfaceFloor)
	}
	fresh := a.target
	fresh.MovementUsedDft, fresh.Dashed = 0, false
	return movementLeftAtDFt(fresh, grid.SurfaceFloor) * a.speedPct / rules.FullSpeedPercent
}
