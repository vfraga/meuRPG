package play

import (
	"context"
	"fmt"
	"math"
	"slices"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dice"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/reaction"
)

// The reactions to a roll and to a damage (PM-04): Cutting Words on an attack roll
// or a damage roll, Uncanny Dodge and Deflect Missiles on a damage the attack rolled,
// the master's check of the table rule "Sempre", and Hellish Rebuke after a damage
// landed. The rolls are held: the damage is rolled when the hold is written, so that
// the reactor decides with the number it would take (never an NPC's total, RN-20), and
// the replay lands that very roll with what the answers changed.

// cuttingWindows lists the Cutting Words windows an enemy's roll opens (SRD,
// College of Lore): each bard of the other side that sees the roller within 60 feet,
// has its reaction and a use of Bardic Inspiration, and whose "Perguntar" setting asks
// for this kind of roll, and whose roller can be cut (it hears the bard and is not
// immune to charm: the server never opens a window that would only say "sem efeito").
func (s *Service) cuttingWindows(ctx context.Context, c *combatTx, cs []playdb.Combatant, roller playdb.Combatant, roll string, base windowTrigger) ([]windowSpec, error) {
	var specs []windowSpec
	for _, r := range cs {
		if r.ID == roller.ID || r.Side == roller.Side || r.Defeated {
			continue
		}
		spec, ok, err := s.cuttingWindowOf(ctx, c, cs, r, roller, roll, base)
		if err != nil {
			return nil, err
		}
		if ok {
			specs = append(specs, spec)
		}
	}
	return specs, nil
}

// cuttingWindowOf is the Cutting Words window one bard of the other side opens for the roll, if it
// opens one.
func (s *Service) cuttingWindowOf(ctx context.Context, c *combatTx, cs []playdb.Combatant, r, roller playdb.Combatant, roll string, base windowTrigger) (windowSpec, bool, error) {
	k, ok, err := s.kitOf(ctx, c.tx, c.session.CampaignID, r)
	if err != nil {
		return windowSpec{}, false, err
	}
	if !ok || !k.st.CuttingWords || !k.canReact() || k.resourceLeft(resBardic) == 0 || !cuttingAsks(k.st.CuttingAsk, roll) {
		return windowSpec{}, false, nil
	}
	dist, sees, err := s.reactorSees(ctx, c, cs, r, roller)
	if err != nil || !sees {
		return windowSpec{}, false, err
	}
	// A roller that cannot hear the bard or is immune to charm opens no window: the prompt would
	// tell the bard what the creature is (decisions-batch2-A, item 4).
	cut, err := s.rollerCanBeCut(ctx, c, roller)
	if err != nil || !cut {
		return windowSpec{}, false, err
	}
	t := base
	t.Roll, t.Distance = roll, dist
	return windowSpec{kind: reaction.CuttingWords, reactor: &r, trigger: t}, true, nil
}

// cuttingAsks says the bard's "Perguntar" setting asks for this kind of roll.
func cuttingAsks(ask, roll string) bool {
	switch ask {
	case "never":
		return false
	case "all":
		return true
	}
	return roll == "attack" // "attacks", the default
}

// holdAttackRoll holds an attack roll that Cutting Words can change: nothing is rolled
// or spent until the windows are answered (the replay subtracts the die from the roll).
func (s *Service) holdAttackRoll(ctx context.Context, c *combatTx, m authz.Membership, req *playv1.RollAttackRequest, cs []playdb.Combatant, attacker, target playdb.Combatant, key string) (actionEvent, bool, error) {
	if c.replay != nil || req.GetCatchWindowId() != "" {
		return actionEvent{}, false, nil
	}
	specs, err := s.cuttingWindows(ctx, c, cs, attacker, "attack", windowTrigger{Actor: attacker.ID, Target: target.ID, Key: key})
	if err != nil {
		return actionEvent{}, false, err
	}
	// "Sempre": a player's attack on an enemy waits for the master's check also when no bard could
	// cut the roll, so the pause (a miss included) tells the table nothing about a bard.
	if len(specs) == 0 && c.rules.EnemyReactionsAlways && playerAgainstEnemy(attacker, target) {
		specs = append(specs, windowSpec{kind: reaction.MasterCheck, trigger: windowTrigger{Actor: attacker.ID, Target: target.ID, Key: key, Roll: "attack"}})
	}
	if len(specs) == 0 {
		return actionEvent{}, false, nil
	}
	ev := actionEvent{Round: c.enc.Round, Secret: secretOf(attacker, target), Actor: attacker.ID, Target: target.ID, Key: key}
	if ev, err = s.holdAction(ctx, c, m, "attack", attacker, req, holdData{}, specs, ev); err != nil {
		return ev, false, err
	}
	if err = touchEncounter(ctx, c); err != nil {
		return ev, false, err
	}
	c.characterID = &attacker.CharacterID
	return ev, true, nil
}

// isAttackHit says a pending damage is the damage of an attack that hit (not a spell's
// saving throw, a trap or a heal): what Uncanny Dodge and Deflect Missiles answer.
func isAttackHit(p playdb.PendingDamage) bool {
	return p.AttackTotal != nil && p.CastID == nil && !p.Healing && p.TrapPointID == nil
}

// damageWindows lists the windows a rolled damage opens before it lands.
func (s *Service) damageWindows(ctx context.Context, c *combatTx, cs []playdb.Combatant, p playdb.PendingDamage, attacker, target playdb.Combatant, total int32) ([]windowSpec, error) {
	if p.Healing {
		return nil, nil
	}
	base := windowTrigger{Actor: attacker.ID, Target: target.ID, Key: p.AttackKey, Damage: total, Pending: p.ID}
	var specs []windowSpec
	if isAttackHit(p) {
		own, err := s.targetWindows(ctx, c, cs, p, attacker, target, base)
		if err != nil {
			return nil, err
		}
		specs = own
	}
	cw, err := s.cuttingWindows(ctx, c, cs, attacker, "damage", base)
	if err != nil {
		return nil, err
	}
	specs = append(specs, cw...)
	if len(specs) == 0 && c.rules.EnemyReactionsAlways && playerAgainstEnemy(attacker, target) {
		t := base
		t.Roll = "damage"
		specs = append(specs, windowSpec{kind: reaction.MasterCheck, trigger: t})
	}
	// The windows are answered in the order of the combat's initiative, but the
	// target's own reactions come before a bard's: they change the damage taken.
	slices.SortStableFunc(specs, func(a, b windowSpec) int { return rankOf(a.kind) - rankOf(b.kind) })
	return specs, nil
}

// targetWindows lists the windows the target of an attack that hit opens for its own reactions:
// Uncanny Dodge, a maneuver that reduces the damage and Deflect Missiles.
func (s *Service) targetWindows(ctx context.Context, c *combatTx, cs []playdb.Combatant, p playdb.PendingDamage, attacker, target playdb.Combatant, base windowTrigger) ([]windowSpec, error) {
	k, ok, err := s.kitOf(ctx, c.tx, c.session.CampaignID, target)
	if err != nil || !ok || !k.canReact() {
		return nil, err
	}
	dist, sees, err := s.reactorSees(ctx, c, cs, target, attacker)
	if err != nil {
		return nil, err
	}
	t := base
	t.Distance = dist
	var specs []windowSpec
	if k.st.UncannyDodge && sees {
		specs = append(specs, windowSpec{kind: reaction.UncannyDodgeKind, reactor: &target, trigger: t})
	}
	reduces, err := s.reducesMelee(ctx, c, k, attacker, p.AttackKey)
	if err != nil {
		return nil, err
	}
	if reduces {
		specs = append(specs, windowSpec{kind: reaction.ManeuverReduceKind, reactor: &target, trigger: t})
	}
	deflects, err := s.deflectsRanged(ctx, c, k, attacker, p.AttackKey)
	if err != nil {
		return nil, err
	}
	if deflects {
		t.Ranged = true
		specs = append(specs, windowSpec{kind: reaction.DeflectKind, reactor: &target, trigger: t})
	}
	return specs, nil
}

// reducesMelee says the target has a maneuver that reduces the damage of this melee weapon attack.
func (s *Service) reducesMelee(ctx context.Context, c *combatTx, k kit, attacker playdb.Combatant, key string) (bool, error) {
	if len(reduceOptionsOf(k)) == 0 {
		return false, nil
	}
	return s.meleeWeaponAttack(ctx, c, attacker, key)
}

// deflectsRanged says the target can deflect this ranged weapon attack.
func (s *Service) deflectsRanged(ctx context.Context, c *combatTx, k kit, attacker playdb.Combatant, key string) (bool, error) {
	if !k.st.Deflect {
		return false, nil
	}
	return s.rangedWeaponAttack(ctx, c, attacker, key)
}

// rankOf is the order windows of one damage are answered in: what changes the roll
// (Cutting Words) first, then what changes the damage taken, then the master's check.
const (
	rankDeflect     = 2
	rankMasterCheck = 3
)

func rankOf(k reaction.Kind) int {
	switch k {
	case reaction.CuttingWords:
		return 0
	case reaction.UncannyDodgeKind:
		return 1
	case reaction.DeflectKind, reaction.ManeuverReduceKind:
		return rankDeflect
	}
	return rankMasterCheck
}

// rangedWeaponAttack says an attack of the attacker's sheet is a ranged weapon attack
// (Deflect Missiles, SRD, Monk 3): not a melee weapon and not a spell.
func (s *Service) rangedWeaponAttack(ctx context.Context, c *combatTx, attacker playdb.Combatant, key string) (bool, error) {
	sheet, err := s.sheetOf(ctx, c.tx, c.session.CampaignID, attacker)
	if connect.CodeOf(err) == connect.CodeNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	i := slices.IndexFunc(sheet.Attacks, func(a link.Attack) bool { return a.Key == key })
	return i >= 0 && !sheet.Attacks[i].Melee && !sheet.Attacks[i].Spell, nil
}

// holdDamage holds a damage roll that a reaction can change. The dice were rolled
// already (roll, total): the roll is kept with the hold and the windows are opened with
// the damage they would take; the replay lands that roll. It returns the event that
// stands for the held roll, or false when nothing waits.
func (s *Service) holdDamage(ctx context.Context, c *combatTx, m authz.Membership, req *playv1.RollDamageRequest, cs []playdb.Combatant, p playdb.PendingDamage, attacker, target playdb.Combatant, roll dice.Result, pr *partsRoll) (actionEvent, bool, error) {
	if c.replay != nil {
		return actionEvent{}, false, nil
	}
	total := clamp32(max(roll.Total, 0), 0, math.MaxInt32)
	specs, err := s.damageWindows(ctx, c, cs, p, attacker, target, total)
	if err != nil || len(specs) == 0 {
		return actionEvent{}, false, err
	}
	faces := faces32(roll.Faces)
	ev := actionEvent{Round: c.enc.Round, Secret: secretOf(attacker, target), Actor: attacker.ID, Target: target.ID, Pending: p.ID, Key: p.AttackKey}
	ev, err = s.holdAction(ctx, c, m, "damage", attacker, req, holdDataOf(roll, faces, pr), specs, ev)
	if err != nil {
		return ev, false, err
	}
	if err = touchEncounter(ctx, c); err != nil {
		return ev, false, err
	}
	c.characterID = &attacker.CharacterID
	return ev, true, nil
}

// replayedRoll is the roll a held damage kept, as the result of its dice.
func replayedRoll(expr dice.Expr, r *rolledDamage) dice.Result {
	faces := make([]int, len(r.Faces))
	for i, f := range r.Faces {
		faces[i] = int(f)
	}
	return dice.Result{Expr: expr, Faces: faces, Modifier: expr.Modifier, Total: int(r.Total), Physical: r.Physical}
}

// adjustedDamage is a replayed damage after what the answers did: the die Cutting
// Words took off the roll, half of an Uncanny Dodge, the Deflect Missiles reduction.
func adjustedDamage(st *replayState, total int) int {
	total = max(total-int(st.reduction), 0)
	if st.halve {
		total = reaction.UncannyDodge(total)
	}
	return max(total-int(st.deflected), 0)
}

// hellishWindow opens the Hellish Rebuke window a landed damage owes its target
// (SRD, Hellish Rebuke): the target survived, the damage came from a creature within
// 60 feet that it sees, and it has the spell and a slot or the Infernal Legacy.
func (s *Service) hellishWindow(ctx context.Context, c *combatTx, target playdb.Combatant, d damageLanded) error {
	if !rebukeOwed(target, d) {
		return nil
	}
	cs, err := c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return fmt.Errorf("list the combatants: %w", err)
	}
	i := slices.IndexFunc(cs, func(x playdb.Combatant) bool { return x.ID == d.attacker })
	if i < 0 || cs[i].Defeated || cs[i].Side == target.Side {
		return nil
	}
	k, ok, err := s.kitOf(ctx, c.tx, c.session.CampaignID, target)
	if err != nil || !ok || !k.canReact() {
		return err
	}
	if len(k.slotsFor(spellHellishRebuke)) == 0 && k.legacyLeft() == 0 {
		return nil
	}
	dist, sees, err := s.reactorSees(ctx, c, cs, target, cs[i])
	if err != nil || !sees {
		return err
	}
	_, err = s.openWindows(ctx, c, newGroup(), nil, []windowSpec{{
		kind: reaction.HellishRebukeKind, reactor: &target,
		trigger: windowTrigger{Actor: d.attacker, Target: target.ID, Key: d.key, Damage: d.hit.Amount, Distance: dist, Pending: d.pending.ID},
	}})
	return err
}

// rebukeOwed says the damage that landed on a player's character could be answered with Hellish
// Rebuke: it came from a creature, it was not the rebuke's own fire or a healing, and it took hit
// points off a target that still stands (at 0 hit points the target is incapacitated, SRD).
func rebukeOwed(target playdb.Combatant, d damageLanded) bool {
	if d.attacker == "" || d.key == spellHellishRebuke || isCreature(target) || d.pending.Healing {
		return false
	}
	if d.hit.After != nil && d.hit.After.HP == 0 {
		return false
	}
	return takenBy(d.hit.Amount, d.hit.Before, d.hit.After) > 0
}

// holdDataOf is what a held damage keeps: the roll, and for a damage with extras every part
// as it rolled (the replay lands those very dice and spends the slot only then).
func holdDataOf(roll dice.Result, faces []int32, pr *partsRoll) holdData {
	data := holdData{Rolled: &rolledDamage{Faces: faces, Total: clamp32(roll.Total, math.MinInt32, math.MaxInt32), Physical: roll.Physical}}
	if pr != nil {
		data.Parts, data.PartRolls = pr.parts, pr.rolls
	}
	return data
}
