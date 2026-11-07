package play

import (
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// What the combat reads of a stat block's actions beyond the sheet (rules.ActionPlan): the actions that
// ask a saving throw or roll nothing (UseCreatureAction), the riders of an attack and the damage parts an
// attack has after its first. The plan of a stat block comes from the content; a combatant is tied to
// its stat block by its monster key.

// maxPlanAttackName is the longest attack name a basic sheet keeps (characters.maxAttackNameLength): the
// name of a sheet attack made from a stat block action is the action's, cut to it.
const maxPlanAttackName = 40

// monsterKeyOf is the stat block key of an NPC or a creature ("monster:ghoul"), "" when the combatant has
// none (a player's character, an NPC made by hand).
func (s *Service) monsterKeyOf(ctx context.Context, c *combatTx, who playdb.Combatant) (string, error) {
	return s.monsterKeyIn(ctx, c.tx, c.session.CampaignID, who)
}

// monsterKeyIn is monsterKeyOf outside a write: tx is the open transaction, or nil for a read.
func (s *Service) monsterKeyIn(ctx context.Context, tx pgx.Tx, campaignID string, who playdb.Combatant) (string, error) {
	if key := conv.OrZero(who.MonsterKey); key != "" {
		return key, nil
	}
	if who.Kind != kindNPC {
		return "", nil
	}
	chars, err := s.roster.CombatCharacters(ctx, tx, campaignID, []string{who.CharacterID})
	if err != nil || len(chars) == 0 {
		return "", err
	}
	return chars[0].MonsterKey, nil
}

// planOfAction finds the action of the combatant's stat block that has the key.
func (s *Service) planOfAction(ctx context.Context, c *combatTx, who playdb.Combatant, actionKey string) (rules.ActionPlan, bool, error) {
	monster, err := s.monsterKeyOf(ctx, c, who)
	if err != nil || monster == "" || !strings.HasPrefix(actionKey, monster+"#") {
		return rules.ActionPlan{}, false, err
	}
	content, err := s.contentOf(ctx, c)
	if err != nil {
		return rules.ActionPlan{}, false, err
	}
	plan, ok := content.MonsterActionPlan(actionKey)
	return plan, ok, nil
}

// planOfAttack finds the action of the attacker's stat block that an attack of its sheet is: a creature's
// attack has the action's key; an NPC made from a stat block has a basic sheet whose attack is named like
// the action (cut to the sheet's length).
// Two actions whose names are equal up to the cut are told apart by the numbers of the sheet's attack (attack
// bonus, damage, range). When the numbers do not tell them apart either, and one of them is limited, the attack
// is refused when refuse is set (the making of the attack): the limit cannot be enforced on the right action.
// What follows the attack (its damage, its rider) never refuses.
func (s *Service) planOfAttack(ctx context.Context, c *combatTx, attacker playdb.Combatant, attackKey string, refuse bool) (rules.ActionPlan, bool, error) {
	if attacker.Kind != kindNPC && attacker.Kind != kindCreature {
		return rules.ActionPlan{}, false, nil
	}
	monster, err := s.monsterKeyOf(ctx, c, attacker)
	if err != nil || monster == "" {
		return rules.ActionPlan{}, false, err
	}
	content, err := s.contentOf(ctx, c)
	if err != nil {
		return rules.ActionPlan{}, false, err
	}
	if strings.Contains(attackKey, "#") {
		plan, ok := content.MonsterActionPlan(attackKey)
		return plan, ok && plan.Attack != nil, nil
	}
	sheet, err := s.sheetOf(ctx, c.tx, c.session.CampaignID, attacker)
	if err != nil {
		return rules.ActionPlan{}, false, err
	}
	i := slices.IndexFunc(sheet.Attacks, func(a link.Attack) bool { return a.Key == attackKey })
	monsterPlan, ok := content.MonsterPlan(monster)
	if i < 0 || !ok {
		return rules.ActionPlan{}, false, nil
	}
	plan, ok, err := planOfSheetAttack(monsterPlan, sheet.Attacks[i])
	if err != nil && !refuse {
		return rules.ActionPlan{}, false, nil
	}
	return plan, ok, err
}

// planOfSheetAttack is planOfAttack for an attack of a basic sheet: the refusal of an attack that cannot be told
// from another action that has a limit.
func planOfSheetAttack(p *rules.MonsterPlan, at link.Attack) (rules.ActionPlan, bool, error) {
	plan, ambiguous, ok := planByCutName(p, at)
	if ambiguous {
		return rules.ActionPlan{}, false, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_CREATURE_ACTION_UNAVAILABLE,
			"the attack cannot be told from another action of the stat block that has a limit")
	}
	return plan, ok, nil
}

// planByCutName finds the attack action that a basic sheet's attack is made from: the one whose name, cut to
// the length a basic sheet keeps, is the attack's. When several share the cut name, the one whose numbers are
// the attack's. Actions the numbers do not tell apart match none (a limit or a rider is never taken from the
// wrong one), and ambiguous says that at least one of them is limited.
func planByCutName(p *rules.MonsterPlan, at link.Attack) (plan rules.ActionPlan, ambiguous, ok bool) {
	var found []rules.ActionPlan
	for _, a := range p.Actions {
		if a.Attack != nil && cutName(a.NamePT) == at.Name {
			found = append(found, a)
		}
	}
	if len(found) > 1 {
		found = slices.DeleteFunc(found, func(a rules.ActionPlan) bool { return !sameNumbers(a, at) })
		if len(found) > 1 {
			return rules.ActionPlan{}, slices.ContainsFunc(found, func(a rules.ActionPlan) bool { return a.Usage.Limited() }), false
		}
	}
	if len(found) != 1 {
		return rules.ActionPlan{}, false, false
	}
	return found[0], false, true
}

// sameNumbers says the sheet's attack has the numbers of the action: the attack bonus, the first damage part
// and the range as basicAttackOf writes them.
func sameNumbers(a rules.ActionPlan, at link.Attack) bool {
	if a.Attack == nil || len(a.Damage) == 0 {
		return false
	}
	d := a.Damage[0]
	return a.Attack.Bonus == at.ToHit && d.Dice.Count == at.DiceCount && d.Dice.Sides == at.DiceSides &&
		d.Dice.Bonus == at.DiceBonus && d.TypeKey == at.DamageType && sheetRangeOf(a.Attack) == at.RangeFt
}

// sheetRangeOf is the range a basic sheet writes for the attack: the range of a ranged or thrown attack, else
// the reach, and 0 for a melee attack at the normal reach.
func sheetRangeOf(a *rules.ActionAttack) int {
	reach := a.ReachFt
	if a.RangeFt > 0 {
		reach = a.RangeFt
	}
	if a.Melee && reach <= normalReachFt {
		return 0
	}
	return min(reach, maxPlanRangeFt)
}

// normalReachFt is the reach of a melee attack a basic sheet writes as range 0; maxPlanRangeFt the longest
// range it keeps (characters.maxRangeFt).
const (
	normalReachFt  = 5
	maxPlanRangeFt = 600
)

// nameOfPlan is the action's name in Portuguese, or its English name when the content has no translation.
func nameOfPlan(plan rules.ActionPlan) string {
	if plan.NamePT != "" {
		return plan.NamePT
	}
	return plan.Name
}

// cutName cuts an action's name to the length a basic sheet keeps.
func cutName(name string) string {
	if r := []rune(name); len(r) > maxPlanAttackName {
		return string(r[:maxPlanAttackName])
	}
	return name
}

// diceOfPlan is a damage part of an action as the spell machinery rolls it.
func diceOfPlan(d rules.ActionDamage) link.Dice {
	return link.Dice{Count: d.Dice.Count, Sides: d.Dice.Sides, Bonus: d.Dice.Bonus, DamageType: d.TypeKey}
}

// plannedParts are the damage parts of an attack after the first, as parts of the one pending damage:
// each rolls with its own dice, doubles on a critical hit and takes the resistance of its own type. The
// sheet keeps the first part only (an NPC's basic sheet, Derived.Attacks), so the others come from the
// plan. An attack with a single part has none.
func plannedParts(plan rules.ActionPlan) []partRecord {
	if len(plan.Damage) <= 1 {
		return nil
	}
	out := make([]partRecord, 0, len(plan.Damage)-1)
	for i, d := range plan.Damage[1:] {
		out = append(out, partRecord{
			Key: planPartKey(i + 1), Label: d.TypeNamePT, Kind: partAuto, Count: clamp32(d.Dice.Count, 0, 100), Sides: clamp32(d.Dice.Sides, 0, 100),
			Flat: clamp32(d.Dice.Bonus, -1000, 1000), DamageType: d.TypeKey, Selected: true, Available: true,
		})
	}
	return out
}

// planPartKey is the key of the part of an action's damage at the index.
func planPartKey(i int) string { return "plan-damage-" + strconv.Itoa(i) }

// spellOfPlan is the action as the spell machinery reads it: the save and the damage it governs, and the
// area. A save "other" (the text's own effect) is read as none: the engine knows no other. The key is the
// action's, so that the pending damages it opens say where they come from.
// Limitation: every action is spent as an action; the plan does not say whether a stat block action costs a
// bonus action or a reaction.
func spellOfPlan(plan rules.ActionPlan) link.Spell {
	sp := link.Spell{Key: plan.Key, Name: nameOfPlan(plan), Economy: rules.EconomyAction, RangeKind: rules.RangeSelf, Nonmagical: plan.Attack == nil || !plan.Attack.Spell}
	if sv := plan.Save; sv != nil {
		sp.SaveAbility, sp.SaveDC, sp.CasterDC = string(sv.Ability), sv.DC, sv.DC
		sp.SaveOnSuccess = sv.OnSuccess
		if sp.SaveOnSuccess != "half" {
			sp.SaveOnSuccess = "none"
		}
		for _, d := range sv.Damage {
			sp.Damages = append(sp.Damages, diceOfPlan(d))
		}
	}
	if a := plan.Area; a != nil {
		sp.Area, sp.AreaSizeFt, sp.AreaWidthFt = true, a.SizeFt, a.WidthFt
		sp.AreaShape = a.Shape
		switch a.Shape {
		case rules.AreaRadius:
			sp.AreaShape = rules.AreaSphere
		case rules.AreaSphere, rules.AreaCylinder:
			sp.RangeKind, sp.RangeFt = rules.RangeRanged, maxPlanPointFt
		}
	}
	return sp
}

// maxPlanPointFt is how far from the creature the point of a sphere or cylinder may be: the stat block
// words the distance in its text, and the master places the point.
const maxPlanPointFt = 120

// shownKey is a key as the viewer may read it: the key of a stat block action ("monster:ghoul#claws")
// names the creature's type, which a player must not learn from an NPC's line (RN-20). A player reads the
// action's name instead; the master, and the key of a creature that belongs to a character, are whole.
func shownKey(v combatViewer, actor playdb.Combatant, key string) string {
	if !v.master && !v.wholeKeys && actor.Kind == kindNPC && strings.Contains(key, "#") {
		return ""
	}
	return key
}
