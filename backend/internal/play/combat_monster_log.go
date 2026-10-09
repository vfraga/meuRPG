package play

import (
	"context"

	"github.com/PuraFome/meuRPG/backend/internal/authz"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// How a monster's hit and its stat block's own events are shown in the combat log (RN-10,
// RN-20): everyone who sees the attack reads that its target saved or failed and the condition it
// got; the dice and the DC are the master's and the target's own player's; the immunity, the
// recharge rolls, the Legendary Resistance left and a monster's checks are the master's alone.

// riderProto is what a hit gave besides the damage, as the viewer gets it.
func riderProto(r *riderEv, v combatViewer, attacker, target playdb.Combatant, conditionPT func(string) string) *playv1.CombatLogRider {
	if r == nil {
		return nil
	}
	out := &playv1.CombatLogRider{Save: saveView(r.Save, v, attacker, target), ConditionKey: r.Condition}
	if r.Condition != "" {
		out.ConditionPt = conditionPT(r.Condition)
	}
	if v.master {
		out.Immune = r.Immune
	}
	if v.master || v.owns(target) {
		out.EscapeDc = r.EscapeDC
	}
	if v.master || v.owns(attacker) {
		out.PendingDamageIds = append(out.PendingDamageIds, r.Pending...)
	}
	return out
}

// monsterHitView adds to an attack's line what a monster's hit gave: the rider and the damage of
// the attack's other parts, in the order they were opened.
func (e *logEntry) monsterHitView(ctx context.Context, v combatViewer, actor, target playdb.Combatant, names *keyNames, out *playv1.CombatLogEntry) {
	if e.rider != nil {
		out.Rider = riderProto(e.rider, v, actor, target, func(key string) string { return names.contentName(ctx, key) })
	} else if e.ev.Monster != nil && e.ev.Monster.Rider != nil { // a hit with no damage to roll
		out.Rider = riderProto(e.ev.Monster.Rider, v, actor, target, func(key string) string { return names.contentName(ctx, key) })
	}
	for _, id := range e.extras {
		if dl, ok := e.pend[id]; ok {
			out.MoreDamages = append(out.MoreDamages, dl.view(v, actor, target))
		}
	}
}

// monsterEventView is the master's details of a recharge roll, a Legendary Resistance or a check.
func (e *logEntry) monsterEventView(ctx context.Context, actor playdb.Combatant, names *keyNames) *playv1.CombatLogMonsterEvent {
	m := e.ev.Monster
	if m == nil {
		return nil
	}
	out := &playv1.CombatLogMonsterEvent{}
	if len(m.Recharges) > 0 {
		mon, _ := names.s.monsterOf(ctx, nil, names.campaignID, actor)
		for _, r := range m.Recharges {
			name := r.Action
			if mon != nil {
				if ap, ok := mon.plan.Action(r.Action); ok {
					name = ap.Name
					if ap.NamePT != "" {
						name = ap.NamePT
					}
				}
			}
			out.Recharges = append(out.Recharges, &playv1.CombatLogRecharge{
				ActionKey: r.Action, ActionName: name, Face: r.Face, Min: r.Min, Recharged: r.Recharged,
			})
		}
	}
	if m.Resist != nil {
		out.ResistanceLeft = m.Resist.Left
	}
	if ck := m.Check; ck != nil {
		out.CheckPt = names.contentName(ctx, ck.Key)
		out.CheckRoll = diceRoll(1, 20, []int32{ck.D20}, ck.Bonus, ck.Total, false)
	}
	return out
}

// riderResponse is the rider of a roll's event for the response of the roll: the master's alone.
func (s *Service) riderResponse(ctx context.Context, m authz.Membership, v combatViewer, ev actionEvent) *playv1.CombatLogRider {
	if !v.master || ev.Monster == nil || ev.Monster.Rider == nil {
		return nil
	}
	return riderProto(ev.Monster.Rider, v, playdb.Combatant{}, playdb.Combatant{}, s.namesFor(ctx, m.CampaignID))
}
