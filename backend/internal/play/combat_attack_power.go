package play

import (
	"context"
	"errors"
	"slices"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/play/damagesteps"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// The power attack of a table feature (the power_attack effect, rules/powerattack.go) on a weapon attack of a player's
// character: the request names it (power_attack_key), the penalty comes off the attack roll and, when the effect has a
// resource, one use is spent with the attack. The server checks the key, the attacker, the weapon line and the uses
// itself; the options only list what it will accept.

// powerAttackEvent is the trade an attack made, kept on its event: the feature, and what it took off the roll and
// added to the damage.
type powerAttackEvent struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Penalty     int32  `json:"penalty"`
	DamageBonus int32  `json:"damage_bonus"`
	// Hidden says the master had retired or switched off the feature when the attack was made: another player reads
	// "Outra fonte" for it, never its name or its numbers.
	Hidden bool `json:"hidden,omitempty"`
}

// logView is the marker of the attack's line in the combat log, nil for an attack that made no trade. The master and the
// attacker's player (owner) read the feature and the numbers; another player reads the feature's name, or "Outra fonte"
// when the master had retired it, and no key or number (RN-10).
func (p *powerAttackEvent) logView(owner bool) *playv1.CombatLogPowerAttack {
	switch {
	case p == nil:
		return nil
	case owner:
		return &playv1.CombatLogPowerAttack{Key: p.Key, NamePt: p.Name, Penalty: p.Penalty, DamageBonus: p.DamageBonus}
	}
	return &playv1.CombatLogPowerAttack{NamePt: powerNameForOthers(p.Hidden, p.Key, p.Name), Hidden: p.Hidden}
}

// powerNameForOthers is the name of the trade's feature as another player reads it: "Outra fonte" when the feature is
// hidden from the players, through the same path as every hidden source of a damage (damagesteps.HidingNames).
func powerNameForOthers(hidden bool, key, name string) string {
	return damagesteps.HidingNames(func(string) string { return name }, []combat.Modifier{{Source: key, Hidden: hidden}})(key)
}

// powerPart is the automatic part the trade adds to the damage of a hit: its flat number, of the weapon's damage type,
// from the feature. It has no dice, so a critical hit never doubles it.
func (p *powerAttackEvent) powerPart(damageType string) partRecord {
	return partRecord{
		Key: partPowerAttack, Label: p.Name, Kind: partAuto, Flat: clamp32(int(p.DamageBonus), 0, rules.MaxPowerAttackDamage), DamageType: damageType,
		Source: p.Key, Selected: true, Available: true, Reason: "Ataque poderoso (automático)", Table: tablePowerAttack, Hide: p.Hidden,
	}
}

// readPowerHidden notes whether the master has hidden the trade's feature from the players (retired or switched it off):
// the part, the roll and the log say "Outra fonte" to another player then, whatever the master does later.
func (f *attackFlow) readPowerHidden(ctx context.Context) error {
	if f.power == nil {
		return nil
	}
	names, err := f.s.roster.SourceNames(ctx, f.c.tx, f.m.CampaignID)
	if err != nil {
		return err
	}
	f.powerHidden = names(f.power.Key) == ""
	return nil
}

// The part of the damage the trade adds to a hit: its key in the damage's parts and its table.
const (
	partPowerAttack  = "power_attack"
	tablePowerAttack = "power_attack"
)

// powerAttackRule is the engine's view of a trade of the sheet, for its weapon filter.
func powerAttackRule(p link.PowerAttack) rules.PowerAttack {
	return rules.PowerAttack{
		Key: p.Key, NamePT: p.NamePT, AttackPenalty: p.AttackPenalty, DamageBonus: p.DamageBonus, Weapon: p.Weapon,
		Properties: p.Properties, PropertyNamesPT: p.PropertyNamesPT, OnlyProficient: p.OnlyProficient, Resource: p.Resource,
	}
}

// powerAttackLine is the attack line of the sheet as the weapon filter reads it.
func powerAttackLine(a link.Attack) rules.Attack {
	line := rules.Attack{Melee: a.Melee, Properties: a.Properties, Proficient: a.Proficient}
	if a.Weapon && !a.Spell {
		line.Kind = "weapon"
	}
	return line
}

// checkPowerAttack checks the trade the request names goes on this attack: a weapon attack of a player's character
// that has the feature, whose weapon filter accepts the line, and a use of its resource left. Nothing is spent here.
func (f *attackFlow) checkPowerAttack(ctx context.Context) error {
	key := f.req.GetPowerAttackKey()
	if key == "" {
		return nil
	}
	if f.attacker.Kind != kindPlayer || !backedByCharacter(f.attacker) || f.attack.Spell || !f.attack.Weapon {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("a power attack goes on a weapon attack of a player's character"))
	}
	i := slices.IndexFunc(f.sheet.Traits.PowerAttacks, func(p link.PowerAttack) bool { return p.Key == key })
	if i < 0 {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("power_attack_key is not a power attack of the character"))
	}
	p := f.sheet.Traits.PowerAttacks[i]
	if pin := f.req.GetPowerAttackPin(); pin != nil { // the roll was held: its numbers stay as they were
		p.NamePT, p.AttackPenalty, p.DamageBonus, p.Resource = pin.GetNamePt(), int(pin.GetPenalty()), int(pin.GetDamageBonus()), pin.GetResource()
	}
	if ok, why := powerAttackRule(p).Allows(powerAttackLine(f.attack)); !ok {
		return connect.NewError(connect.CodeInvalidArgument, errors.New(why))
	}
	if p.Resource != "" {
		vit, err := f.s.vitals.GetVitalsTx(ctx, f.c.tx, f.m.CampaignID, f.attacker.CharacterID)
		if err != nil {
			return err
		}
		need := int32(1)
		if f.maneuver != nil && f.maneuver.Resource == p.Resource {
			need = 2 // the maneuver die and the trade each spend a use of the one pool
		}
		if left, _ := poolLeft(vit, p.Resource); left < need {
			return resourceBlocked(playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_NO_USES_LEFT, "no uses left")
		}
	}
	f.power = &p
	return nil
}

// pinnedRequest is the request a roll that waits keeps: as sent, with the trade as it is now, so the answer settles on
// these numbers and not on what the sheet says by then (RollAttackRequest.power_attack_pin).
func (f *attackFlow) pinnedRequest() *playv1.RollAttackRequest {
	if f.power == nil {
		return f.req
	}
	pinned, _ := proto.Clone(f.req).(*playv1.RollAttackRequest)
	pinned.PowerAttackPin = &playv1.PowerAttackPin{
		NamePt: f.power.NamePT, Penalty: conv.Int32(f.power.AttackPenalty), DamageBonus: conv.Int32(f.power.DamageBonus), Resource: f.power.Resource,
	}
	return pinned
}

// powerPenalty is what the trade takes off the attack roll, 0 for an attack that makes none.
func (f *attackFlow) powerPenalty() int {
	if f.power == nil {
		return 0
	}
	return f.power.AttackPenalty
}

// spendPowerAttack spends the use of the trade's resource with the attack, and notes it on the attack's resources so
// the undo gives it back (the resource of a maneuver die is kept the same way). A held roll writes nothing until its
// answer runs the attack again, so it spends once, there.
func (f *attackFlow) spendPowerAttack(ctx context.Context) error {
	if f.power == nil || f.power.Resource == "" {
		return nil
	}
	if _, err := f.s.spendResource(ctx, f.c, f.attacker.CharacterID, f.power.Resource, 1); err != nil {
		return err
	}
	f.maneuverSpent = append(f.maneuverSpent, f.power.Resource)
	return nil
}

// powerEvent is the trade as the attack's event keeps it, nil when the attack made none.
func (f *attackFlow) powerEvent() *powerAttackEvent {
	p := f.power
	if p == nil {
		return nil
	}
	return &powerAttackEvent{Key: p.Key, Name: p.NamePT, Penalty: conv.Int32(p.AttackPenalty), DamageBonus: conv.Int32(p.DamageBonus), Hidden: f.powerHidden}
}

// listPowerAttacks lists, on each weapon attack option of a player's character, the trades its table features offer for
// that attack line (the ones whose weapon filter accepts it). An NPC, and a line the filter refuses, list none.
func (s *Service) listPowerAttacks(ctx context.Context, who playdb.Combatant, reads *turnReads, opts *rulesv1.TurnOptions) error {
	if who.Kind != kindPlayer || !backedByCharacter(who) || len(opts.GetAttacks()) == 0 {
		return nil
	}
	sheet, err := reads.sheetOf(ctx)
	if err != nil {
		return s.dbError(ctx, "read the character's power attacks", err)
	}
	if len(sheet.Traits.PowerAttacks) == 0 {
		return nil
	}
	for _, o := range opts.GetAttacks() {
		i := slices.IndexFunc(sheet.Attacks, func(a link.Attack) bool { return a.Key == o.GetAttack().GetKey() })
		if i < 0 {
			continue
		}
		line := powerAttackLine(sheet.Attacks[i])
		for _, p := range sheet.Traits.PowerAttacks {
			if ok, _ := powerAttackRule(p).Allows(line); !ok {
				continue
			}
			opt, err := s.powerAttackOption(ctx, reads, p)
			if err != nil {
				return err
			}
			o.PowerAttacks = append(o.PowerAttacks, opt)
		}
	}
	return nil
}

// powerAttackOption is a trade as the options list it: disabled, with its reason, when its resource has no use left.
func (s *Service) powerAttackOption(ctx context.Context, reads *turnReads, p link.PowerAttack) (*rulesv1.PowerAttackOption, error) {
	opt := &rulesv1.PowerAttackOption{Key: p.Key, Name: p.NamePT, Penalty: conv.Int32(p.AttackPenalty), DamageBonus: conv.Int32(p.DamageBonus)}
	if p.Resource == "" {
		return opt, nil
	}
	vit, err := reads.vitalsOf(ctx)
	if err != nil {
		return nil, s.dbError(ctx, "read the character's power attack uses", err)
	}
	left, _ := poolLeft(vit, p.Resource)
	opt.ResourceNamePt, opt.UsesLeft, opt.Recharge = poolName(vit, p.Resource), max(left, 0), poolRecharge(vit, p.Resource)
	if left <= 0 {
		opt.Disabled, opt.DisabledReasonPt = true, "Sem usos do recurso."
	}
	return opt, nil
}
