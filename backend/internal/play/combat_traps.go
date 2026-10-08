package play

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	maplink "github.com/PuraFome/meuRPG/backend/internal/maps/link"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dice"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// A trap fired in a combat (MR-035, Etapa 9, D5, slice 9.8). The maps module owns
// the trap (where it is, whether it is armed, who knows it); this file is the
// combat's side of it: who the trap catches, what the dice do to each combatant
// and how the master takes it back.
//
// How a trap fires in a combat:
//
//   - A player's character or a creature of one ends its move on a square of the
//     area, or the straight move runs into the area and stops on its first square
//     (Terrain.MoveUntil with the area as stopAt), or a jump lands in it. An NPC
//     never fires a trap by walking. The master's move of a player's character or
//     creature counts as a landing. MoveCombatant calls the hook (trapHook) at
//     three points and does nothing else here.
//   - The master fires any trap by hand (PlayService.FireTrap) and picks who is
//     caught.
//   - The effect is rolled by the server (traps_effect.go). Damage to an NPC or a
//     creature lands at once, conditions go on the combatants, and damage to a
//     player's character is a pending damage that waits for the master (RN-02).
//   - The firing is one event, `trap_triggered`, with everything an undo needs: the
//     master's "Desfazer" puts the trap, the hit points, the conditions and the
//     pending damages back. A move that fired a trap writes two events (the move,
//     then the firing), so the master takes the firing back first.

// trapFireEvent is the payload of `trap_triggered` (IDs and numbers only, no name:
// docs/privacy.md), and what a combat's undo and log read back.
type trapFireEvent struct {
	PointID string `json:"point_id"`
	MapID   string `json:"map_id"`
	// PrevState and PrevTriggeredAt are the trap's state before the firing: the
	// undo puts them back.
	PrevState       string     `json:"prev_state"`
	PrevTriggeredAt *time.Time `json:"prev_triggered_at,omitempty"`
	// Manual says the master fired it. ExtendsID is the firing this one adds creatures
	// to (the master picks who is caught, D5): the trap is not triggered again, and an
	// undo takes back only the creatures it added.
	Manual    bool   `json:"manual,omitempty"`
	ExtendsID string `json:"extends_id,omitempty"`
	// Part says this event holds the creatures that did not fit the firing's own
	// event (the payload is capped): it extends the firing, and an undo takes it
	// back together with the events of the same firing.
	Part bool `json:"part,omitempty"`
	// Caught is what happened to each creature the trap caught, in order. In a
	// combat the target is a combatant; outside one, a character.
	Caught []trapCaughtEvent `json:"caught,omitempty"`
}

// trapCaughtEvent is what a trap did to one creature.
type trapCaughtEvent struct {
	Target    string `json:"target_id"`
	Character string `json:"character_id,omitempty"`
	// Player says the target is a player's character: its damage waits for the
	// master.
	Player bool `json:"player,omitempty"`
	// Hidden says the master had hidden the target when the trap fired: its line is
	// never the players', even after the master reveals it (RN-10).
	Hidden  bool              `json:"hidden,omitempty"`
	Attacks []trapAttackEvent `json:"attacks,omitempty"`
	Saves   []saveRoll        `json:"saves,omitempty"` // one, or one for each hit of a trap that asks it of the creatures it hit
	Damages []trapDamageEvent `json:"damages,omitempty"`
	// Conditions are the keys the trap gave; in a combat CondBefore are the
	// combatant's conditions before, which the undo puts back (CondSet).
	Conditions []string `json:"conditions,omitempty"`
	// On a map with the fog of war, Fogged says an NPC was caught and SeenBy lists the
	// players (user IDs) who saw it then: the firing is public, the NPC's part of its
	// line is theirs alone (combat_fog.go).
	Fogged     bool     `json:"fogged,omitempty"`
	SeenBy     []string `json:"seen_by,omitempty"`
	CondSet    bool     `json:"cond_set,omitempty"`
	CondBefore []string `json:"cond_before,omitempty"`
}

// trapAttackEvent is one attack of the trap at a creature.
type trapAttackEvent struct {
	D20      int32  `json:"d20"`
	Modifier int32  `json:"modifier"`
	Total    int32  `json:"total"`
	Outcome  string `json:"outcome"`
	TargetAC int32  `json:"target_ac,omitempty"` // the master's alone
}

// trapDamageEvent is one damage part rolled for a creature: the roll, and what it
// did (damageHit: where it waits, what landed at once, what an undo puts back).
type trapDamageEvent struct {
	damageHit
	Type      string  `json:"damage_type"`
	DiceCount int32   `json:"dice_count,omitempty"`
	DiceSides int32   `json:"dice_sides,omitempty"`
	Bonus     int32   `json:"bonus,omitempty"`
	Faces     []int32 `json:"faces,omitempty"`
	RollTotal int32   `json:"roll_total"`
	Critical  bool    `json:"critical,omitempty"`
	// CriticalMax is what the table's critical rule added without rolling (already
	// in RollTotal); MaxRule says the table's rule was "máximo mais uma rolagem".
	CriticalMax int32 `json:"critical_max,omitempty"`
	MaxRule     bool  `json:"critical_max_rule,omitempty"`
}

// trapD20 and trapDice are the server's rolls for a trap: always in the app (the
// trap is the master's; the table's own dice are for the players' rolls, RN-18).
func (s *Service) trapD20() (int, error) {
	face, _, err := s.d20(rollInput{inApp: true}, 0)
	return face, err
}

func (s *Service) trapDice(e dice.Expr) (dice.Result, error) {
	res, err := dice.Roll(s.roller, e)
	if err != nil {
		return dice.Result{}, fmt.Errorf("roll the trap's damage: %w", err)
	}
	return res, nil
}

// trapTargetsOf works out what the arithmetic needs of each combatant caught: its
// armor class when the trap attacks, and its saving throw bonus when it asks one.
func (s *Service) trapTargetsOf(ctx context.Context, tx pgx.Tx, campaignID string, effect *rulesv1.TrapEffect, caught []playdb.Combatant) ([]trapTarget, error) {
	ability := trapSaveAbility(effect)
	out := make([]trapTarget, len(caught))
	for i, c := range caught {
		out[i].id = c.ID
		if trapNeedsAC(effect) {
			sheet, err := s.sheetOf(ctx, tx, campaignID, c)
			if err != nil {
				return nil, err
			}
			out[i].armorClass = sheet.ArmorClass + int(c.AcBonus)
		}
		if ability != "" {
			save, err := s.saveOf(ctx, tx, campaignID, c, ability)
			if err != nil {
				return nil, err
			}
			out[i].save, out[i].saveKnown = save.Bonus, save.Known
		}
	}
	return out, nil
}

// fireInCombat fires the trap on the combatants it caught, inside the change's
// transaction, and returns the event: the trap is marked triggered (public from
// now on), the effect is rolled, and each result lands where it belongs. The
// caller writes the event and touches the combat. maplink.ErrTrapNotArmed comes
// back as it is.
func (s *Service) fireInCombat(ctx context.Context, c *combatTx, trap maplink.Trap, caught []playdb.Combatant, manual bool, extends string) (*trapFireEvent, error) {
	campaignID := c.session.CampaignID
	prev := trap
	ev := &trapFireEvent{PointID: trap.PointID, MapID: trap.MapID, Manual: manual, ExtendsID: extends}
	if extends == "" {
		var err error
		if prev, err = s.traps.TriggerTrap(ctx, c.tx, campaignID, trap.MapID, trap.PointID, c.now); err != nil {
			if connect.CodeOf(err) == connect.CodeNotFound {
				err = maplink.ErrTrapNotArmed // deleted meanwhile: it is simply not there
			}
			return nil, err // maplink.ErrTrapNotArmed is the caller's to turn into an answer
		}
		ev.PrevState, ev.PrevTriggeredAt = prev.State, prev.TriggeredAt
	}
	effect := prev.Spec.GetEffect()
	targets, err := s.trapTargetsOf(ctx, c.tx, campaignID, effect, caught)
	if err != nil {
		return nil, err
	}
	outcomes, err := resolveTrap(effect, targets, criticalRuleOf(c.rules), s.trapD20, s.trapDice)
	if err != nil {
		return nil, err
	}
	for i, o := range outcomes {
		who := caught[i]
		cc := trapCaughtEvent{Target: who.ID, Character: who.CharacterID, Player: who.Kind == kindPlayer, Hidden: who.Hidden}
		for _, a := range o.attacks {
			outcome := outcomeMiss
			switch {
			case a.hit && a.critical:
				outcome = outcomeCrit
			case a.hit:
				outcome = outcomeHit
			}
			cc.Attacks = append(cc.Attacks, trapAttackEvent{D20: clampInt32(a.d20), Modifier: clampInt32(a.bonus), Total: clampInt32(a.total), Outcome: outcome, TargetAC: clampInt32(a.armorClass)})
		}
		cc.Saves = savesEventOf(o.saves)
		if len(o.conditions) > 0 {
			merged := slices.Clone(who.Conditions)
			for _, k := range o.conditions {
				merged = appendOnce(merged, k)
			}
			cc.Conditions = o.conditions
			if !slices.Equal(merged, who.Conditions) {
				if err := c.q.SetCombatantConditions(ctx, playdb.SetCombatantConditionsParams{ID: who.ID, Conditions: merged}); err != nil {
					return nil, fmt.Errorf("give the trap's conditions: %w", err)
				}
				cc.CondSet, cc.CondBefore = true, who.Conditions
				who.Conditions = merged
			}
		}
		for _, d := range o.damages {
			if d.amount <= 0 && !d.half {
				continue // a roll of 0 has nothing to apply
			}
			de, err := s.trapDamageInCombat(ctx, c, ev.PointID, &who, d)
			if err != nil {
				return nil, err
			}
			cc.Damages = append(cc.Damages, de)
		}
		ev.Caught = append(ev.Caught, cc)
	}
	return ev, nil
}

// savesEventOf are the rolled saving throws as the event keeps them.
func savesEventOf(rs []trapSaveRoll) []saveRoll {
	var out []saveRoll
	for _, r := range rs {
		out = append(out, saveRoll{D20: clampInt32(r.d20), Bonus: clampInt32(r.bonus), Total: clampInt32(r.total), DC: clampInt32(r.dc), Saved: r.saved, Unknown: !r.known})
	}
	return out
}

// trapDamageInCombat records one damage part for a combatant: a player's
// character's waits for the master as a ROLLED pending damage; an NPC's or a
// creature's is APPLIED at once (who is updated with its new hit points, so the next
// part lands on them).
func (s *Service) trapDamageInCombat(ctx context.Context, c *combatTx, pointID string, who *playdb.Combatant, d trapDamageRoll) (trapDamageEvent, error) {
	status := pendingRolled
	var resolved *time.Time
	if holdsHP(*who) {
		status, resolved = pendingApplied, &c.now
	}
	amount := clampInt32(d.amount)
	p, err := c.q.InsertTrapPendingDamage(ctx, playdb.InsertTrapPendingDamageParams{
		EncounterID: c.enc.ID, TargetID: who.ID, Status: status, Critical: d.critical,
		DiceCount: clamp32(d.count, 0, 100), DiceSides: clamp32(d.sides, 0, 100), DiceBonus: clamp32(d.bonus, -1000, 1000),
		DamageType: d.damageType, Faces: faces32(d.faces), Amount: &amount, RollTotal: new(clampInt32(d.rollTotal)), Half: d.half,
		CreatedAt: c.now, ResolvedAt: resolved, TrapPointID: &pointID, CriticalMax: clamp32(d.criticalMax, 0, 10000), CriticalMaxRule: d.maxRule,
	})
	if err != nil {
		return trapDamageEvent{}, fmt.Errorf("open the trap's damage: %w", err)
	}
	hit, _, err := s.landDamage(ctx, c, p, *who, amount) // only an NPC or a creature takes it now
	if err != nil {
		return trapDamageEvent{}, err
	}
	if hit.Applied && hit.After != nil {
		who.HpCurrent, who.HpTemp, who.Defeated = &hit.After.HP, &hit.After.Temp, hit.After.Defeated
	}
	return trapDamageEvent{
		damageHit: hit, Type: d.damageType, DiceCount: clamp32(d.count, 0, 100), DiceSides: clamp32(d.sides, 0, 100), Bonus: clamp32(d.bonus, -1000, 1000),
		Faces: faces32(d.faces), RollTotal: clampInt32(d.rollTotal), Critical: d.critical, CriticalMax: clamp32(d.criticalMax, 0, 10000), MaxRule: d.maxRule,
	}, nil
}

// takeBackTrap puts back what a firing did, inside the undo's transaction: the
// pending damages it opened are deleted, the hit points and conditions of the
// combatants it changed are put back, and the trap is armed again. A combatant or
// a trap that is gone is skipped.
func (s *Service) takeBackTrap(ctx context.Context, c *combatTx, ev trapFireEvent, find func(string) (playdb.Combatant, bool)) error {
	// The last damage of a combatant is undone first, so the hit points go back
	// part by part to where each part found them.
	for _, cc := range slices.Backward(ev.Caught) {
		who, ok := find(cc.Target)
		for _, d := range slices.Backward(cc.Damages) {
			if err := c.q.DeletePendingDamage(ctx, d.Pending); err != nil {
				return fmt.Errorf("take back the trap's damage: %w", err)
			}
			if ok && d.Applied && d.Before != nil {
				if err := c.q.SetCombatantHitPoints(ctx, playdb.SetCombatantHitPointsParams{ID: who.ID, HpCurrent: &d.Before.HP, HpTemp: &d.Before.Temp, Defeated: d.Before.Defeated}); err != nil {
					return fmt.Errorf("put back the hit points: %w", err)
				}
			}
		}
		if ok && cc.CondSet {
			before := cc.CondBefore
			if before == nil {
				before = []string{} // the column is not null; an empty list is not written in the event
			}
			if err := c.q.SetCombatantConditions(ctx, playdb.SetCombatantConditionsParams{ID: who.ID, Conditions: before}); err != nil {
				return fmt.Errorf("put back the conditions: %w", err)
			}
		}
	}
	if s.traps == nil || ev.ExtendsID != "" {
		return nil // an extension never changed the trap
	}
	return s.traps.RestoreTrap(ctx, c.tx, c.session.CampaignID, ev.MapID, ev.PointID, ev.PrevState, ev.PrevTriggeredAt, c.now)
}

// ---- who a trap catches ----

// inArea lists the combatants standing in the area: placed, not defeated.
func inArea(cs []playdb.Combatant, area maplink.Trap) []playdb.Combatant {
	var out []playdb.Combatant
	for _, c := range cs {
		if placed(c) && !c.Defeated && !c.Dismissed && area.Covers(squareOfCombatant(c)) {
			out = append(out, c)
		}
	}
	return out
}

// ---- the hook MoveCombatant calls ----

// trapHook is a move's side of the traps: it makes the move stop at the area of an
// armed "Ao entrar na área" trap, and, once the move is made, fires what it
// entered. MoveCombatant creates one with newTrapHook, passes stops() to
// Terrain.MoveUntil, tells it where the mover landed and calls finish where it used
// to return the move's event. A hook with no traps does nothing.
type trapHook struct {
	s      *Service
	mapID  string
	mover  playdb.Combatant
	origin grid.Square
	// traps are the armed "Ao entrar na área" traps the mover is not standing in.
	traps []maplink.Trap
	// entered is the trap the move entered (the first one along the line, or the one
	// holding the landing square).
	entered *maplink.Trap
	// fired is what the move fired; set by finish, read after the commit.
	fired *trapFireEvent
}

// newTrapHook reads the armed traps of the combat's map that this mover may fire:
// a player's character or a creature of one, in a combat that is running. An NPC,
// a combat that has not begun and a map with no traps give a hook that does
// nothing. A failure to read them is the move's failure: the read runs in the
// move's transaction, where a failed statement aborts it, so swallowing the error
// would turn a retryable conflict (40001) into a 25P02.
func (s *Service) newTrapHook(ctx context.Context, tx pgx.Tx, campaignID string, enc playdb.Encounter, mover playdb.Combatant) (*trapHook, error) {
	th := &trapHook{s: s, mover: mover}
	if s.traps == nil || enc.MapID == nil || enc.Status != statusActive || mover.Kind == kindNPC || !placed(mover) {
		return th, nil
	}
	th.mapID, th.origin = *enc.MapID, squareOfCombatant(mover)
	traps, err := s.traps.Traps(ctx, tx, campaignID, th.mapID) // inside the move's transaction: a trap disarmed or deleted meanwhile makes it retry, and is simply not there
	if err != nil {
		return nil, fmt.Errorf("read the map's traps: %w", err)
	}
	for _, t := range traps {
		if t.Armed() && t.OnEnter && len(t.Squares) > 0 && !t.Covers(th.origin) {
			th.traps = append(th.traps, t)
		}
	}
	return th, nil
}

// stops is the set of squares the move must stop at: the areas of the armed traps.
// nil when there are none (MoveUntil then ignores it).
func (th *trapHook) stops() grid.Set {
	if th == nil || len(th.traps) == 0 {
		return nil
	}
	var all grid.Squares
	for _, t := range th.traps {
		all = append(all, t.Squares...)
	}
	return all
}

// marked records that a move on foot entered a stop square (Stopped.Marked).
func (th *trapHook) marked(entered bool, at grid.Square) {
	if th != nil && entered {
		th.landed(at)
	}
}

// landed records where the mover ended (a jump, the master's move): a trap whose
// area holds the square fires. A trap already entered stays the first.
func (th *trapHook) landed(at grid.Square) {
	if th == nil || th.entered != nil {
		return
	}
	for i := range th.traps {
		if th.traps[i].Covers(at) {
			th.entered = &th.traps[i]
			return
		}
	}
}

// finish returns what MoveCombatant's closure returns: the move's event, or, when
// the move entered a trap, the move's event is written now and the trap's firing is
// the change's own (so a retry finds it under the idempotency key). cs are the
// combatants as they stood before the move; moved is the mover after it.
func (th *trapHook) finish(ctx context.Context, c *combatTx, made actionEvent, cs []playdb.Combatant, moved playdb.Combatant) (any, error) {
	if th == nil || th.entered == nil {
		return made, nil
	}
	// Who is caught: the mover, and, when the trap hits the area, everyone in it.
	after := slices.Clone(cs)
	if i := slices.IndexFunc(after, func(o playdb.Combatant) bool { return o.ID == moved.ID }); i >= 0 {
		after[i] = moved
	}
	caught := []playdb.Combatant{moved}
	if th.entered.Spec.GetEffect().GetTargets() != rulesv1.TrapTargets_TRAP_TARGETS_MANUAL {
		caught = inArea(after, *th.entered)
		if !slices.ContainsFunc(caught, func(o playdb.Combatant) bool { return o.ID == moved.ID }) {
			caught = append([]playdb.Combatant{moved}, caught...)
		}
	}
	ev, err := th.s.fireInCombat(ctx, c, *th.entered, caught, false, "")
	if errors.Is(err, maplink.ErrTrapNotArmed) {
		return made, nil // fired or disarmed meanwhile: the move stands, the trap does not fire again
	}
	if err != nil {
		return nil, err
	}
	// The move comes first in the history, the firing after it (the change's own event).
	if err := insertEvent(ctx, c, eventCombatantMoved, &c.actorUserID, nil, made); err != nil {
		return nil, err
	}
	th.fired = ev
	c.kind = eventTrapTriggered
	return actionEvent{Round: c.enc.Round, Actor: moved.ID, Trap: ev, StoppedEarly: made.StoppedEarly, OnTurn: made.OnTurn, MoveID: made.MoveID}, nil
}

// after tells the streams about the traps the move changed, after the commit: the
// trap is public now, and the combat, its log and the pending damages changed.
func (th *trapHook) after(ctx context.Context, campaignID string, enc playdb.Encounter) {
	if th == nil || th.fired == nil {
		return
	}
	th.s.traps.TrapChanged(ctx, campaignID, th.fired.MapID, th.fired.PointID)
	th.s.publishEncounterChanged(ctx, campaignID, enc)
	th.s.publishLogChanged(ctx, campaignID, enc.ID, true)
}

// noticeAfterMove is the passive notice of a player's character or its creature
// that ended a move on a map with traps (MR-035): after the commit, so the maps
// module sees the square the mover stands on now. A failure is logged and never
// fails the move.
func (s *Service) noticeAfterMove(ctx context.Context, campaignID string, enc playdb.Encounter, moved playdb.Combatant) {
	if s.traps == nil || enc.MapID == nil || enc.Status != statusActive || moved.Kind == kindNPC || !placed(moved) {
		return
	}
	ob := maplink.Observer{CharacterID: moved.CharacterID, At: squareOfCombatant(moved)}
	if isCreature(moved) {
		eyes, ok, err := s.roster.CreatureEyes(ctx, nil, campaignID, deref(moved.MonsterKey))
		if err != nil {
			s.logger.ErrorContext(ctx, "play: cannot read a creature's senses after a move", "error", err)
			return
		}
		if !ok {
			return
		}
		ob.Creature = &eyes
	}
	if err := s.traps.Notice(context.WithoutCancel(ctx), campaignID, *enc.MapID, []maplink.Observer{ob}); err != nil {
		s.logger.ErrorContext(ctx, "play: cannot notice a trap after a move", "error", err)
	}
}

// ---- the warning before stepping into a trap ----

// markKnownTraps marks, on the squares a player's character can reach, the ones a
// move to would run into an armed trap the character knows: the app asks "Isso entra
// no Fosso escondido. Mover assim mesmo?" first. The master is not warned, and a
// trap the character does not know is never marked (RN-10).
func (s *Service) markKnownTraps(ctx context.Context, campaignID string, enc playdb.Encounter, who playdb.Combatant, out *playv1.GetMoveOptionsResponse) {
	if s.traps == nil || enc.MapID == nil || who.Kind == kindNPC || len(out.GetReachable()) == 0 {
		return
	}
	known, err := s.traps.KnownTraps(ctx, campaignID, *enc.MapID, who.CharacterID)
	if err != nil {
		s.logger.ErrorContext(ctx, "play: cannot read the traps the character knows", "error", err)
		return
	}
	from := squareOfCombatant(who)
	var ahead []maplink.Trap
	for _, t := range known {
		if t.OnEnter && len(t.Squares) > 0 && !t.Covers(from) {
			ahead = append(ahead, t)
		}
	}
	if len(ahead) == 0 {
		return
	}
	for _, r := range out.Reachable {
		to := grid.Square{Col: int(r.GetCol()), Row: int(r.GetRow())}
		for _, step := range grid.Line(from, to) {
			if i := slices.IndexFunc(ahead, func(t maplink.Trap) bool { return t.Covers(step.Square) }); i >= 0 {
				r.KnownTrapPointId, r.KnownTrapName = ahead[i].PointID, ahead[i].Name
				break
			}
		}
	}
}

// ---- what a trap's firing says, for the master and for the log ----

// trapView says what a viewer of a firing gets (RN-10, RN-20): the master all of
// it; a player the outcome words, their own character's (and creatures') dice, and
// nothing of a hidden combatant.
type trapView struct {
	master bool
	// owns says the viewer plays the target; hidden says a player does not get it.
	owns, hidden func(targetID string) bool
	label        func(targetID string) string
	// status says where a damage is now (a master's apply moves it on); the
	// fallback is what the event knew.
	status func(pendingID string) (playv1.PendingDamageStatus, bool)
	// amount says what the master applied when it is not the rolled amount.
	amount func(pendingID string) (int32, bool)
}

// firingProto is the trap's firing as the viewer gets it, built from its event.
func firingProto(ev *trapFireEvent, id, name string, v trapView) *playv1.TrapFiring {
	out := &playv1.TrapFiring{PointId: ev.PointID, Name: name, Id: id}
	for _, cc := range ev.Caught {
		if !v.master && v.hidden != nil && v.hidden(cc.Target) {
			continue // a combatant the players do not see stays out of their line
		}
		mine := v.master || (v.owns != nil && v.owns(cc.Target))
		t := &playv1.TrapCaught{TargetId: cc.Target, ConditionKeys: cc.Conditions}
		if mine || cc.Player { // an NPC's character is the master's secret
			t.CharacterId = cc.Character
		}
		if v.label != nil {
			t.TargetLabel = v.label(cc.Target)
		}
		for _, a := range cc.Attacks {
			r := &playv1.TrapAttackRoll{Outcome: outcomeToProto[a.Outcome]}
			if mine {
				r.Roll = diceRoll(1, 20, []int32{a.D20}, a.Modifier, a.Total, false)
			}
			if v.master {
				r.TargetArmorClass = a.TargetAC
			}
			t.Attacks = append(t.Attacks, r)
		}
		for _, sv := range cc.Saves {
			r := &playv1.TrapSaveRoll{Outcome: playv1.SaveOutcome_SAVE_OUTCOME_FAILED}
			if sv.Saved {
				r.Outcome = playv1.SaveOutcome_SAVE_OUTCOME_SAVED
			}
			if mine {
				r.Roll = diceRoll(1, 20, []int32{sv.D20}, sv.Bonus, sv.Total, false)
			}
			if v.master {
				r.Dc, r.BonusKnown = sv.DC, !sv.Unknown
			}
			if t.Save == nil {
				t.Save = r
			}
			t.Saves = append(t.Saves, r)
		}
		for _, d := range cc.Damages {
			status := playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_ROLLED
			if d.Applied {
				status = playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_APPLIED
			}
			if now, ok := v.statusOf(d.Pending); ok {
				status = now
			}
			if v.amount != nil {
				if a, ok := v.amount(d.Pending); ok {
					d.Amount = a // the master changed it before applying
				}
			}
			row := &playv1.TrapDamageDone{
				DamageId: d.Pending, Status: status, Amount: d.Amount, DamageTypeKey: d.Type, DamageTypePt: damageTypePT[d.Type], Half: d.Half, Critical: d.Critical,
				TargetDefeated: d.Applied && d.After != nil && d.After.Defeated,
			}
			// The dice of the target's own player and the master's; everyone else
			// gets what the creature lost.
			if mine {
				row.Roll = diceRoll(d.DiceCount, d.DiceSides, d.Faces, d.Bonus+d.CriticalMax, d.RollTotal, false)
			}
			t.Damages = append(t.Damages, row)
		}
		out.Caught = append(out.Caught, t)
	}
	return out
}

func (v trapView) statusOf(pendingID string) (playv1.PendingDamageStatus, bool) {
	if v.status == nil {
		return 0, false
	}
	return v.status(pendingID)
}
