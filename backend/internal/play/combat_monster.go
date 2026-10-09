package play

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dice"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// A monster of a combat fights with its whole SRD stat block (SRD 5.1, "Monsters"): the
// attacks and spells go through RollAttack and CastSpell, and what they cannot do is in
// CreatureService (combat_monster_*.go). This file has what they share: which creature a
// combatant is, what it has spent of its stat block (combatants.monster_state), the start
// of its turn (the d6 of each action that waits to recharge, the legendary actions it
// regains), and the conditions its actions give.
//
// Everything about a stat block is the master's secret (RN-10, RN-20): what is spent, what
// recharged, the legendary actions and Legendary Resistance left, the conditions the
// creature is immune to. The events keep it under actionEvent.Monster, and the log and the
// answers give it to the master alone.

// The kinds of event a monster's stat block writes (migration 00196).
const (
	eventMonsterRecharge     = "monster_recharge_rolled"
	eventLegendaryResistance = "legendary_resistance_used"
	eventCombatantCheck      = "combatant_check_rolled"
)

// rechargeEv is the d6 of an action that waited for its recharge.
type rechargeEv struct {
	Action    string `json:"action"`
	Face      int32  `json:"face"`
	Min       int32  `json:"min"`
	Recharged bool   `json:"recharged,omitempty"`
}

// condChange is the conditions of a combatant before an action gave it one: what an undo puts
// back.
type condChange struct {
	Target string   `json:"target"`
	Before []string `json:"before,omitempty"`
}

// riderEv is what a monster's hit gave its target besides the damage (RollDamage's hook).
type riderEv struct {
	Save *saveRoll `json:"save,omitempty"`
	// Condition is the condition the target got ("condition:paralyzed"); Immune says it would
	// have got it but is immune to it.
	Condition string `json:"condition,omitempty"`
	Immune    bool   `json:"immune,omitempty"`
	EscapeDC  int32  `json:"escape_dc,omitempty"`
	// Pending are the pending damages the rider opened (a poison bite's damage).
	Pending []string `json:"pending,omitempty"`
}

// checkEv is a check a monster rolled.
type checkEv struct {
	Key   string `json:"key"`
	D20   int32  `json:"d20"`
	Bonus int32  `json:"bonus"`
	Total int32  `json:"total"`
}

// resistEv is a Legendary Resistance used on the failed save of a cast.
type resistEv struct {
	Cast string `json:"cast"`
	Left int32  `json:"left"`
	// Pending are the damages of the cast that the new success changed, each with the status
	// and the half flag it had: an undo puts them back.
	Pending []resistedPending `json:"pending,omitempty"`
}

// resistedPending is a pending damage a Legendary Resistance changed.
type resistedPending struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Half   bool   `json:"half,omitempty"`
}

// monsterEvent is what the events of a stat block keep besides the common fields of actionEvent.
// IDs, keys and numbers only (docs/privacy.md).
type monsterEvent struct {
	// Before is the creature's state before the change, whole, for the undo; set when the
	// change spent something (nil: it spent nothing).
	Before *combat.MonsterState `json:"before,omitempty"`
	// Recharges are the rolls at the start of the turn.
	Recharges []rechargeEv `json:"recharges,omitempty"`
	// Option is the legendary option taken, and Cost what it cost.
	Option string `json:"option,omitempty"`
	Cost   int32  `json:"cost,omitempty"`
	// Routine is the Multiattack routine chosen, from 0 (Option-less); Routined says it was.
	Routine  int32 `json:"routine,omitempty"`
	Routined bool  `json:"routined,omitempty"`
	// Immune are the targets that failed and are immune to the condition the action gives.
	Immune []string `json:"immune,omitempty"`
	// Conds are the conditions the action gave, to put back.
	Conds []condChange `json:"conds,omitempty"`
	// Rider is what a hit gave besides the damage; Extras the pending damages the damage roll
	// opened for the rest of the attack's damage parts.
	Rider  *riderEv `json:"rider,omitempty"`
	Extras []string `json:"extras,omitempty"`
	// Resist is a Legendary Resistance; Check a check.
	Resist *resistEv `json:"resist,omitempty"`
	Check  *checkEv  `json:"check,omitempty"`
	// Passed says the event is the master letting a prompt or an offer go: it has no line in the
	// log; Left are the uses of Legendary Resistance left then.
	Passed bool  `json:"passed,omitempty"`
	Left   int32 `json:"left,omitempty"`
}

// monsterRef is the creature a combatant is made from, and its stat block as a combat reads it.
type monsterRef struct {
	key     string
	plan    *rules.MonsterPlan
	content *rules.Content
	// combatOnly says the NPC is the one the app keeps for a monster of "Pôr no combate" (it
	// fights with the stat block's attacks, not the sheet's).
	combatOnly bool
}

// monsterOf says which creature a combatant is made from: nil for a player's character, an NPC
// the master typed with no creature, and a character's creature (a familiar). tx is the
// caller's transaction, or nil outside one.
func (s *Service) monsterOf(ctx context.Context, tx pgx.Tx, campaignID string, who playdb.Combatant) (*monsterRef, error) {
	all, err := s.monstersOf(ctx, tx, campaignID, []playdb.Combatant{who})
	if err != nil {
		return nil, err
	}
	return all[who.ID], nil
}

// monstersOf is monsterOf for many combatants at once, by combatant ID: one read of the NPCs'
// sheets and one of the content, whatever the number of monsters.
func (s *Service) monstersOf(ctx context.Context, tx pgx.Tx, campaignID string, cs []playdb.Combatant) (map[string]*monsterRef, error) {
	var ids []string
	for _, c := range cs {
		if c.Kind == kindNPC && !slices.Contains(ids, c.CharacterID) {
			ids = append(ids, c.CharacterID)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	chars, err := s.roster.CombatCharacters(ctx, tx, campaignID, ids)
	if err != nil {
		return nil, err
	}
	var content *rules.Content
	out := map[string]*monsterRef{}
	for _, c := range cs {
		i := slices.IndexFunc(chars, func(x link.Character) bool { return x.ID == c.CharacterID })
		if c.Kind != kindNPC || i < 0 || chars[i].MonsterKey == "" {
			continue
		}
		if content == nil {
			if content, err = s.roster.RulesContent(ctx, tx, campaignID); err != nil {
				return nil, err
			}
		}
		if plan, ok := content.MonsterPlan(chars[i].MonsterKey); ok {
			out[c.ID] = &monsterRef{key: chars[i].MonsterKey, plan: plan, content: content, combatOnly: chars[i].CombatOnly}
		}
	}
	return out, nil
}

// errNotAMonster is the refusal for a combatant that has no stat block.
func errNotAMonster() error {
	return connect.NewError(connect.CodeInvalidArgument, errors.New("combatant_id is not a monster made from an SRD creature"))
}

// monsterStateOf reads what a combatant has spent of its stat block.
func monsterStateOf(who playdb.Combatant) (combat.MonsterState, error) {
	var st combat.MonsterState
	if len(who.MonsterState) == 0 {
		return st, nil
	}
	if err := json.Unmarshal(who.MonsterState, &st); err != nil {
		return combat.MonsterState{}, fmt.Errorf("read the monster state: %w", err)
	}
	return st, nil
}

// saveMonsterState writes what a combatant has spent.
func saveMonsterState(ctx context.Context, c *combatTx, who playdb.Combatant, st combat.MonsterState) error {
	body, err := json.Marshal(st)
	if err != nil {
		return fmt.Errorf("encode the monster state: %w", err)
	}
	if err := c.q.SetCombatantMonsterState(ctx, playdb.SetCombatantMonsterStateParams{ID: who.ID, MonsterState: body}); err != nil {
		return fmt.Errorf("save the monster state: %w", err)
	}
	return nil
}

// rechargeDieFaces is the d6 of a recharge, and maxLegendaryCost the most a legendary option
// costs (the creature has at most 5 a round).
const (
	rechargeDieFaces = 6
	maxLegendaryCost = 5
)

// statesEqual says whether two states are the same.
func statesEqual(a, b combat.MonsterState) bool {
	x, errA := json.Marshal(a)
	y, errB := json.Marshal(b)
	return errA == nil && errB == nil && bytes.Equal(x, y)
}

// startMonsterTurns is what the start of a turn does for the monsters (SRD 5.1, "Monsters":
// recharge; "Legendary Creatures"): the creatures whose turn begins roll the d6 of each action that
// waits to recharge (one master-only event), regain their legendary actions and forget the
// Multiattack routine of their last turn; every other legendary creature that can act is offered
// a legendary action at the end of the turn that passed (prev), and the offers of the turn before
// are gone.
func (s *Service) startMonsterTurns(ctx context.Context, c *combatTx, ids []string, round int32) error {
	cs, err := c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return fmt.Errorf("list the combatants: %w", err)
	}
	mons, err := s.monstersOf(ctx, c.tx, c.session.CampaignID, cs)
	if err != nil || len(mons) == 0 {
		return err
	}
	prev := ""
	if c.enc.Status == statusActive {
		prev = deref(c.enc.CurrentCombatantID)
	}
	for _, who := range cs {
		mon := mons[who.ID]
		if mon == nil {
			continue
		}
		st, err := monsterStateOf(who)
		if err != nil {
			return err
		}
		next := st.Clone()
		var rolls []combat.RechargeRoll
		if slices.Contains(ids, who.ID) {
			var rollErr error
			rolls = next.StartTurn(mon.plan, func() int {
				res, err := dice.Roll(s.roller, dice.Expr{Count: 1, Sides: rechargeDieFaces})
				if err != nil {
					rollErr = err
					return 1
				}
				return res.Total
			})
			if rollErr != nil {
				return fmt.Errorf("roll a recharge: %w", rollErr)
			}
			next.Prompts = nil
		} else {
			next.Offer = nil
			if prev != "" && prev != who.ID && mon.plan.Legendary != nil && !who.Defeated && combat.CanAct(who.Conditions) && next.LegendaryLeft(mon.plan) > 0 {
				next.Offer = &combat.LegendaryOffer{After: prev}
			}
		}
		if !statesEqual(st, next) {
			if err := saveMonsterState(ctx, c, who, next); err != nil {
				return err
			}
		}
		if len(rolls) == 0 {
			continue
		}
		ev := actionEvent{Round: round, Secret: true, Actor: who.ID, Monster: &monsterEvent{}}
		for _, r := range rolls {
			ev.Monster.Recharges = append(ev.Monster.Recharges, rechargeEv{
				Action: r.Action, Face: clamp32(r.Face, 1, rechargeDieFaces), Min: clamp32(r.Min, 1, rechargeDieFaces), Recharged: r.Recharged,
			})
		}
		if err := insertEvent(ctx, c, eventMonsterRecharge, &c.actorUserID, nil, ev); err != nil {
			return err
		}
	}
	return nil
}

// conditionImmune says whether a combatant cannot suffer a condition: the stat block of the
// creature it is lists it in its condition immunities (SRD 5.1, Monsters). A player's
// character is never immune here.
func (s *Service) conditionImmune(ctx context.Context, tx pgx.Tx, campaignID string, target playdb.Combatant, condition string) (bool, error) {
	var content *rules.Content
	var key string
	switch {
	case isCreature(target):
		key = deref(target.MonsterKey)
	case target.Kind == kindNPC:
		mon, err := s.monsterOf(ctx, tx, campaignID, target)
		if err != nil || mon == nil {
			return false, err
		}
		content, key = mon.content, mon.key
	default:
		return false, nil
	}
	if key == "" {
		return false, nil
	}
	if content == nil {
		var err error
		if content, err = s.roster.RulesContent(ctx, tx, campaignID); err != nil {
			return false, err
		}
	}
	creature, ok := content.CreatureByKey(key)
	if !ok {
		return false, nil
	}
	return slices.ContainsFunc(creature.ConditionImmunities, func(k rules.NamedKey) bool { return k.Key == condition }), nil
}

// addCondition adds a condition label to a combatant (the master takes it off when the effect
// ends; a duration is another unit's). It reports whether the condition was added, and whether
// the combatant is immune to it (then nothing changes). A combatant that already has the condition
// is unchanged. The conditions before are returned for an undo.
func (s *Service) addCondition(ctx context.Context, c *combatTx, target playdb.Combatant, condition string) (added, immune bool, before []string, err error) {
	if slices.Contains(target.Conditions, condition) {
		return false, false, nil, nil
	}
	if immune, err = s.conditionImmune(ctx, c.tx, c.session.CampaignID, target, condition); err != nil || immune {
		return false, immune, nil, err
	}
	if len(target.Conditions) >= maxConditions {
		return false, false, nil, nil
	}
	before = slices.Clone(target.Conditions)
	conditions := append(slices.Clone(target.Conditions), condition)
	if err := c.q.SetCombatantConditions(ctx, playdb.SetCombatantConditionsParams{ID: target.ID, Conditions: conditions}); err != nil {
		return false, false, nil, fmt.Errorf("set the conditions: %w", err)
	}
	// A druid that falls unconscious is itself again (SRD).
	if err := s.endFormIfAsleep(ctx, c, target, conditions); err != nil {
		return false, false, nil, err
	}
	return true, false, before, nil
}

// takeBackMonster puts back what the event changed of a monster's stat block: the state it had,
// the conditions its action gave, the pending damages its hit opened and a Legendary
// Resistance (inside the undo's transaction, before the event's own part of takeBack).
func (s *Service) takeBackMonster(ctx context.Context, c *combatTx, ev actionEvent, find func(string) (playdb.Combatant, bool)) error {
	m := ev.Monster
	if m == nil {
		return nil
	}
	if m.Before != nil {
		if who, ok := find(ev.Actor); ok {
			if err := saveMonsterState(ctx, c, who, *m.Before); err != nil {
				return err
			}
		}
	}
	for _, ch := range m.Conds {
		if who, ok := find(ch.Target); ok {
			if err := c.q.SetCombatantConditions(ctx, playdb.SetCombatantConditionsParams{ID: who.ID, Conditions: nonNil(ch.Before)}); err != nil {
				return fmt.Errorf("put back the conditions: %w", err)
			}
		}
	}
	for _, id := range m.Extras {
		if err := c.q.DeletePendingDamage(ctx, id); err != nil {
			return fmt.Errorf("delete the pending damage: %w", err)
		}
	}
	if m.Rider != nil {
		for _, id := range m.Rider.Pending {
			if err := c.q.DeletePendingDamage(ctx, id); err != nil {
				return fmt.Errorf("delete the pending damage: %w", err)
			}
		}
	}
	if m.Resist != nil {
		for _, p := range m.Resist.Pending {
			if _, err := c.q.SetPendingDamageHalf(ctx, playdb.SetPendingDamageHalfParams{ID: p.ID, Status: p.Status, Half: p.Half, ResolvedAt: nil}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("put back the pending damage: %w", err)
			}
		}
	}
	return nil
}

// legendaryReason says why the creature cannot take a legendary action of the given cost now
// (SRD 5.1, "Legendary Creatures"), or UNSPECIFIED when it can: the combat runs, the creature
// is up and able to act, it is not its own turn (a legendary action is taken at the end of
// another creature's turn), and it has the actions left.
func legendaryReason(enc playdb.Encounter, who playdb.Combatant, st combat.MonsterState, plan *rules.MonsterPlan, cost int) playv1.EncounterBlockedReason {
	switch {
	case enc.Status != statusActive:
		return playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_ACTIVE
	case who.Defeated || actsNow(enc, who) || !combat.CanAct(who.Conditions) || st.Offer == nil:
		return playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_LEGENDARY_NOT_NOW
	case st.LegendaryLeft(plan) < cost:
		return playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_LEGENDARY_ACTIONS_SPENT
	}
	return playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_UNSPECIFIED
}

// legendaryGate refuses a legendary action the creature may not take now.
func (s *Service) legendaryGate(c *combatTx, who playdb.Combatant, st combat.MonsterState, plan *rules.MonsterPlan, cost int) error {
	if err := notEnded(c.enc); err != nil {
		return err
	}
	switch legendaryReason(c.enc, who, st, plan, cost) {
	case playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_ACTIVE:
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_ACTIVE, "the combat is not running")
	case playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_LEGENDARY_NOT_NOW:
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_LEGENDARY_NOT_NOW,
			"a legendary action is taken at the end of another creature's turn, by a creature able to act")
	case playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_LEGENDARY_ACTIONS_SPENT:
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_LEGENDARY_ACTIONS_SPENT, "the creature has no legendary actions left for this")
	}
	return nil
}

// limitError turns the reason an action cannot be used into the refusal.
func limitError(err error) error {
	switch {
	case errors.Is(err, combat.ErrMonsterRecharging):
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_MONSTER_ACTION_RECHARGING, "the action is recharging")
	case errors.Is(err, combat.ErrNoMonsterUses):
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NO_USES, "the action has no uses left")
	}
	return err
}

// attackKey is an attack's key as the viewer may read it: the key of a monster's action carries
// the creature's key ("monster:goblin#scimitar"), which is the master's secret (RN-10), so a
// player gets none; the name of the attack ("Cimitarra") is what they read.
func (v combatViewer) attackKey(key string) string {
	if v.master || !strings.Contains(key, "#") {
		return key
	}
	return ""
}
