package characters

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// What a combat reads from a sheet while the turns run (MR-012, MR-014):
// the armor class and the attacks that an attack roll and its damage need
// (CombatSheet), and what the combatant can do now (CombatTurnOptions). Like
// the roster and the vitals, package play declares the interface and this
// Service implements it; the rules engine (package rules and its combat
// package) does every number, and this file only reads the sheet and copies.

// fighter reads a living character of the campaign for a combat: its kind
// and the derived sheet. A full sheet is derived by the rules; a basic one has
// no derivation, so one is built from its numbers (its attacks, armor class,
// speed) and the standard actions everyone has, and the rules' own
// combat.Options works for a minion as for a hero. `not_found` for a
// character that is not one of the campaign's living ones.
func (s *Service) fighter(ctx context.Context, tx pgx.Tx, campaignID, characterID string) (kind string, d rules.Derived, content *rules.Content, err error) {
	id, ok := parseUUID(characterID)
	if !ok {
		return "", rules.Derived{}, nil, errCharacterNotFound()
	}
	rows, err := s.queriesIn(tx).ListCombatCharacters(ctx, charactersdb.ListCombatCharactersParams{CampaignID: campaignID, Ids: []string{id}})
	if err != nil {
		return "", rules.Derived{}, nil, s.dbError(ctx, "read a character for a combat", err)
	}
	if len(rows) == 0 {
		return "", rules.Derived{}, nil, errCharacterNotFound()
	}
	sheet, err := loadSheet(rows[0].ID, rows[0].Sheet)
	if err != nil {
		return "", rules.Derived{}, nil, s.dbError(ctx, "read a character for a combat", err)
	}
	content, err = s.contentFor(ctx, tx, campaignID)
	if err != nil {
		return "", rules.Derived{}, nil, s.dbError(ctx, "read rules content", err)
	}
	switch {
	case sheet.GetFull() != nil:
		// A druid in Wild Shape fights as the beast (MR-037).
		return rows[0].Kind, derive(content, sheet.GetFull(), rows[0].WildShapeBeast), content, nil
	case sheet.GetBasic() != nil:
		return rows[0].Kind, basicDerived(content, sheet.GetBasic()), content, nil
	}
	return "", rules.Derived{}, nil, s.dbError(ctx, "read a character for a combat", fmt.Errorf("%w: the sheet of character %s has no content", errCorruptDocument, id))
}

// basicDerived is the little of rules.Derived that a combat reads, for a
// basic sheet: the numbers as written, its attacks with a key of their own
// ("basic:0", "basic:1"...), and the standard actions.
func basicDerived(content *rules.Content, b *charactersv1.BasicSheet) rules.Derived {
	d := rules.Derived{
		ArmorClass:      int(b.GetArmorClass()),
		HitPointsMax:    int(b.GetHitPointsMax()),
		SpeedWalkFt:     int(b.GetSpeedFt()),
		Initiative:      int(b.GetInitiativeBonus()),
		StandardActions: content.StandardActions(), AttacksPerAction: 1,
	}
	// A sheet made from a creature makes as many attacks per Attack action as the
	// creature's Multiattack says (MR-042); the three attacks it holds are the
	// ones it picks from.
	if m, ok := content.MonsterDerived(b.GetMonsterKey()); ok {
		d.AttacksPerAction = max(m.AttacksPerAction, 1)
	}
	creature, _ := content.MonsterDerived(b.GetMonsterKey())
	// Its saving throws are the stat block's (the ability modifier plus the
	// proficiency the creature lists); a basic sheet with no creature has none.
	d.SavingThrows = creature.SavingThrows
	for i, a := range b.GetAttacks() {
		typeKey := "damage-type:" + strings.ToLower(strings.TrimPrefix(a.GetDamageType().String(), "DAMAGE_TYPE_"))
		dice := rules.DiceFormula{Count: int(a.GetDamageDiceCount()), Sides: int(a.GetDamageDiceSides()), Bonus: int(a.GetDamageBonus())}
		attack := rules.Attack{
			Key: fmt.Sprintf("basic:%d", i), Name: a.GetName(), NamePT: a.GetName(), Kind: "weapon",
			AttackBonus: int(a.GetAttackBonus()), DamageDice: dice, Damage: diceText(dice),
			DamageType: typeKey, DamageTypeNamePT: content.NamePT(typeKey), RangeFt: int(a.GetRangeFt()),
			Melee: a.GetRangeFt() <= 5, // a basic sheet's attack that reaches 5 ft or less is a melee one
		}
		// The sheet writes a reach or a range in one number, so a melee attack with
		// a longer reach (a giant's club) or a throwing range (a guard's spear)
		// looks like a ranged one. The creature's stat block says which it is.
		if from, ok := creatureAttackNamed(creature, a.GetName()); ok && from.Melee {
			attack.Melee, attack.LongRangeFt = true, from.LongRangeFt
		}
		d.Attacks = append(d.Attacks, attack)
	}
	return d
}

// creatureAttackNamed is the attack of the creature's stat block that a basic
// sheet's attack of that name was made from (the sheet cuts a name at its
// limit).
func creatureAttackNamed(creature rules.Derived, name string) (rules.Attack, bool) {
	for _, a := range creature.Attacks {
		if n := a.NamePT; n == name || (len([]rune(n)) > maxAttackNameLength && string([]rune(n)[:maxAttackNameLength]) == name) {
			return a, true
		}
	}
	return rules.Attack{}, false
}

// diceText writes a formula as the sheet shows it: "1d6+2", "2d8-1".
func diceText(f rules.DiceFormula) string {
	text := fmt.Sprintf("%dd%d", f.Count, f.Sides)
	switch {
	case f.Bonus > 0:
		return fmt.Sprintf("%s+%d", text, f.Bonus)
	case f.Bonus < 0:
		return fmt.Sprintf("%s%d", text, f.Bonus)
	}
	return text
}

// CombatSheet implements play.CombatRoster: what an attack needs from a
// character's sheet. It takes no caller: it runs after play's authorization
// check, and its armor class never goes to a player.
func (s *Service) CombatSheet(ctx context.Context, tx pgx.Tx, campaignID, characterID string) (link.Sheet, error) {
	_, d, _, err := s.fighter(ctx, tx, campaignID, characterID)
	if err != nil {
		return link.Sheet{}, err
	}
	out := link.Sheet{ArmorClass: d.ArmorClass, Senses: senseRanges(d.Senses)}
	for _, a := range d.Attacks {
		name := a.NamePT
		if name == "" {
			name = a.Name
		}
		out.Attacks = append(out.Attacks, link.Attack{
			Key: a.Key, Name: name, Save: a.SaveDC > 0, Spell: a.Kind == "spell", ToHit: a.AttackBonus,
			DiceCount: a.DamageDice.Count, DiceSides: a.DamageDice.Sides, DiceBonus: a.DamageDice.Bonus,
			DamageType: a.DamageType, RangeFt: a.RangeFt, LongRangeFt: a.LongRangeFt, Melee: a.Melee,
			Beams: a.Beams, Light: a.Light, Unarmed: a.Key == rules.UnarmedStrikeKey, MartialArts: a.MartialArts, AbilityMod: a.AbilityMod,
			AmmunitionItem: a.AmmunitionItem, AmmunitionOut: a.AmmunitionOut,
		})
	}
	for _, a := range d.StandardActions {
		out.Actions = append(out.Actions, link.Action{Key: a.Key, Name: a.NamePT})
	}
	out.AttacksPerAction = max(d.AttacksPerAction, 1)
	out.CriticalRange, out.TwoWeaponFighting = d.CriticalRange, d.TwoWeaponFighting
	for _, a := range d.Actions {
		out.FeatureActions = append(out.FeatureActions, link.FeatureAction{
			Key: a.Key, Name: a.NamePT, Economy: a.Economy, Resource: a.Resource, Pool: poolResources[a.Resource], Standard: a.Standard,
		})
	}
	for _, cl := range d.Classes {
		if cl.ClassKey == "class:fighter" {
			out.FighterLevel = cl.Level
		}
	}
	return out, nil
}

// poolResources are the resources that count points, not uses: Cura pelas mãos
// has 5 points for each paladin level, and one use of the action spends however
// many the player chooses, so the combat leaves the pool to the master.
var poolResources = map[string]bool{"lay_on_hands": true}

// CombatTurnOptions implements play.CombatRoster: what the character can do
// now, from the sheet, what it used this turn and, for a player's character,
// the spell slots already spent (its vitals).
func (s *Service) CombatTurnOptions(ctx context.Context, tx pgx.Tx, campaignID, characterID string, turn link.Turn) (*rulesv1.TurnOptions, error) {
	kind, d, _, err := s.fighter(ctx, tx, campaignID, characterID)
	if err != nil {
		return nil, err
	}
	var usage combat.Usage
	if kind == "player" {
		v, err := s.getVitals(ctx, tx, campaignID, characterID)
		if err != nil {
			return nil, err
		}
		usage = usageOf(v)
	}
	// The speed of this combat, copied when the combatant joined, rules.
	d.SpeedWalkFt = turn.SpeedFt
	opts := combat.Options(d, combat.TurnState{
		ActionUsed: turn.ActionUsed, BonusActionUsed: turn.BonusActionUsed, ReactionUsed: turn.ReactionUsed,
		MovementUsedFt: turn.MovementUsedFt, Dashed: turn.Dashed, AttacksMade: turn.AttacksMade, LastAttackKey: turn.AttackKey, FlurryLeft: turn.FlurryLeft, ActionSurged: turn.ActionSurged, SpellCast: turn.SpellCast, BonusSpellCast: turn.BonusSpellCast,
	}, usage)
	out := turnOptionsToProto(opts)
	// The movement is kept in tenths of a foot (RN-21): the feet fields are those
	// rounded down, so the two never disagree.
	speed := turn.SpeedFt * 10
	if turn.Dashed {
		speed *= 2
	}
	left := max(speed-turn.MovementUsedDFt, 0)
	out.Economy.Movement = &rulesv1.MovementLeft{
		SpeedFt: i32(speed / 10), UsedFt: i32(turn.MovementUsedDFt / 10), LeftFt: i32(left / 10),
		SpeedDft: i32(speed), UsedDft: i32(turn.MovementUsedDFt), LeftDft: i32(left),
	}
	if len(d.Abilities) > 0 { // a basic sheet has no Strength to jump with
		// What the combatant has on it, which is what MoveCombatant enforces.
		out.Jumps = &rulesv1.JumpLimits{
			LongRunningDft: i32(turn.JumpLongDFt), LongStandingDft: i32(turn.JumpLongDFt / 2),
			HighRunningDft: i32(turn.JumpHighDFt), HighStandingDft: i32(turn.JumpHighDFt / 2),
			RunningStart: combat.HasRunningStart(turn.LastMoveDFt),
		}
	}
	return out, nil
}

// usageOf reads the slots a character has spent from its vitals.
func usageOf(v *playv1.CharacterVitals) combat.Usage {
	var u combat.Usage
	for _, slot := range v.GetSpellSlots() {
		if l := int(slot.GetLevel()); l >= 1 && l <= len(u.SlotsUsed) {
			u.SlotsUsed[l-1] = int(slot.GetUsed())
		}
	}
	u.PactSlotsUsed = int(v.GetPactSlots().GetUsed())
	for _, r := range v.GetResources() {
		if u.ResourcesUsed == nil {
			u.ResourcesUsed = map[string]int{}
		}
		u.ResourcesUsed[r.GetKey()] = int(r.GetUsed())
	}
	return u
}

var reasonToProto = map[string]rulesv1.DisabledReasonCode{
	combat.ReasonActionUsed:          rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_ACTION_USED,
	combat.ReasonBonusActionUsed:     rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_BONUS_ACTION_USED,
	combat.ReasonReactionUsed:        rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_REACTION_USED,
	combat.ReasonNoSlot:              rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_NO_SLOT,
	combat.ReasonNoUses:              rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_NO_USES,
	combat.ReasonAlreadyUsedThisTurn: rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_ALREADY_USED_THIS_TURN,
	combat.ReasonReactionOnlyWhenHit: rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_REACTION_ONLY_WHEN_HIT,
	combat.ReasonReactionOnly:        rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_REACTION_ONLY,
	combat.ReasonTooLong:             rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_CASTING_TIME_TOO_LONG,
	combat.ReasonAttacksUsed:         rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_ATTACKS_USED,
	combat.ReasonAttackActionFirst:   rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_ATTACK_ACTION_FIRST,

	combat.ReasonBonusActionSpellLimit: rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_BONUS_ACTION_SPELL_LIMIT,
}

var bonusRuleToProto = map[combat.BonusKind]rulesv1.BonusAttackRule{
	combat.BonusNone:        rulesv1.BonusAttackRule_BONUS_ATTACK_RULE_UNSPECIFIED,
	combat.BonusTwoWeapon:   rulesv1.BonusAttackRule_BONUS_ATTACK_RULE_OFF_HAND,
	combat.BonusMartialArts: rulesv1.BonusAttackRule_BONUS_ATTACK_RULE_MARTIAL_ARTS,
	combat.BonusFlurry:      rulesv1.BonusAttackRule_BONUS_ATTACK_RULE_FLURRY_OF_BLOWS,
}

// turnOptionsToProto copies package combat's TurnOptions into the API's.
func turnOptionsToProto(o combat.TurnOptions) *rulesv1.TurnOptions {
	slot := func(s combat.Slot) *rulesv1.EconomyState {
		return &rulesv1.EconomyState{Used: s.Used, Available: s.Available}
	}
	out := &rulesv1.TurnOptions{Economy: &rulesv1.TurnEconomy{
		Action: slot(o.Economy.Action), BonusAction: slot(o.Economy.BonusAction), Reaction: slot(o.Economy.Reaction),
		Movement:         &rulesv1.MovementLeft{SpeedFt: i32(o.Economy.Movement.SpeedFt), UsedFt: i32(o.Economy.Movement.UsedFt), LeftFt: i32(o.Economy.Movement.LeftFt)},
		AttacksPerAction: i32(o.Economy.AttacksPerAction), AttacksLeft: i32(o.Economy.AttacksLeft),
	}}
	for _, a := range o.Attacks {
		out.Attacks = append(out.Attacks, &rulesv1.AttackOption{
			Attack: attackToProto(a.Attack), Enabled: a.Enabled, Reason: reasonProto(a.Reason),
			BonusRule: bonusRuleToProto[a.Bonus], BonusAttacksLeft: i32(a.FlurryLeft), BonusDropsModifier: a.DropsModifier, BeamsLeft: i32(a.BeamsLeft),
		})
	}
	for _, sp := range o.Spells {
		so := &rulesv1.SpellOption{
			Spell: spellToProto(sp.Spell), Economy: economyToProto[sp.Economy], Enabled: sp.Enabled, Reason: reasonProto(sp.Reason),
		}
		for _, c := range sp.Slots {
			so.Slots = append(so.Slots, &rulesv1.SlotChoice{Level: i32(c.Level), Pact: c.Pact, Free: i32(c.Free)})
		}
		out.Spells = append(out.Spells, so)
	}
	for _, a := range o.StandardActions {
		out.StandardActions = append(out.StandardActions, actionOptionToProto(a))
	}
	for _, a := range o.FeatureActions {
		out.FeatureActions = append(out.FeatureActions, actionOptionToProto(a))
	}
	return out
}

func actionOptionToProto(a combat.ActionOption) *rulesv1.ActionOption {
	return &rulesv1.ActionOption{Action: actionToProto(a.Action), Enabled: a.Enabled, Reason: reasonProto(a.Reason), UsesLeft: i32(a.UsesLeft)}
}

// reasonProto is nil for an enabled option.
func reasonProto(r *combat.Reason) *rulesv1.DisabledReason {
	if r == nil {
		return nil
	}
	return &rulesv1.DisabledReason{Code: reasonToProto[r.Code], MinLevel: i32(r.MinLevel), Recharge: rechargeToProto[r.Recharge]}
}
