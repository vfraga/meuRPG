package play

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dice"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// Casting a spell (MR-014, RN-02, RN-18, RN-22). The cast
// spends the slot and the economy at once; what it does to the targets depends
// on the spell: a spell attack compares a d20 with the armor class, a saving
// throw spell has the server roll each target's save, Magic Missile's darts
// never miss, a healing spell opens a heal, and the rest only spends and logs.
// The damage and the heals wait as pending damages for RollDamage
// (combat_damage.go), which is also where an area spell's one damage roll
// settles all its targets.

// Magic Missile is the one spell whose darts the caster shares out: three, and
// one more for each slot level above the 1st; each dart is its own damage
// divided by the number of darts at the spell's level (1d4 + 1).
const magicMissile = "spell:magic-missile"

// scorchingRay makes three rays at the 2nd level and one more for each level
// above. The caster shares them out as they like, all at one creature if they
// want: a target's darts field is how many rays it gets (none counts as one),
// and each ray is its own spell attack with its own damage.
const scorchingRay = "spell:scorching-ray"

// scorchingBaseRays is the rays Scorching Ray makes at its own level.
const scorchingBaseRays = 3

// scorchingRays is how many rays Scorching Ray makes at a slot level.
func scorchingRays(sp link.Spell, slotLevel int) int {
	return scorchingBaseRays + max(slotLevel-sp.Level, 0)
}

// sharesAt is how many darts or rays a slot level gives the caster to share out among the targets: 0 for a
// spell that shares none.
func sharesAt(sp link.Spell, slotLevel int) int {
	if sp.Key == scorchingRay {
		return scorchingRays(sp, slotLevel)
	}
	return dartsOf(sp, slotLevel)
}

// raysAt lists the target (an index of targets) each attack of a cast is made at, in the order the targets
// were given and the rays of each target together: one attack for a target, but for Scorching Ray as many as
// rays the target is given.
func raysAt(sp link.Spell, targets []castAim) []int {
	var out []int
	for i, t := range targets {
		n := 1
		if sp.Key == scorchingRay {
			n = max(t.darts, 1)
		}
		for range n {
			out = append(out, i)
		}
	}
	return out
}

// rayCombatants lists the combatant each attack of a cast is made at, once for every ray.
func rayCombatants(sp link.Spell, targets []castAim, targs []playdb.Combatant) []playdb.Combatant {
	var out []playdb.Combatant
	for _, i := range raysAt(sp, targets) {
		out = append(out, targs[i])
	}
	return out
}

// shownDarts is the darts a target of the request shows in the cast's event: a ray of Scorching Ray is an
// entry of its own and shows none.
func shownDarts(sp link.Spell, t castAim) int32 {
	if sp.Key == scorchingRay {
		return 0
	}
	return clamp32(t.darts, 0, maxShownDarts)
}

// maxSpellTargets is the most targets one cast takes, the master's too: the
// cast's event and the damage roll that settles it hold what each target did,
// and a session event's payload is at most 4 KiB (migration 00024).
const maxSpellTargets = 10

// castable is a spell the caster may cast, as its turn options list it: a
// prepared spell of options.spells, or a cantrip of options.attacks that asks
// for a saving throw.
type castable struct {
	level   int32
	economy rulesv1.ActionEconomy
	slots   []*rulesv1.SlotChoice
	enabled bool
	reason  *rulesv1.DisabledReason
	// grant is set for a spell cast through a granted_spell effect: no slot, a use of the
	// grant's resource instead (unless it is at will).
	grant *rulesv1.SpellGrantOption
}

// castableOf finds the spell among the options. invalid_argument when the
// caster does not have it, or it is a cantrip with an attack roll.
func castableOf(opts *rulesv1.TurnOptions, key, grantedBy string) (castable, error) {
	for _, sp := range opts.GetSpells() {
		if sp.GetSpell().GetKey() == key && sp.GetGrant().GetSourceKey() == grantedBy {
			return castable{level: sp.GetSpell().GetLevel(), economy: sp.GetEconomy(), slots: sp.GetSlots(), enabled: sp.GetEnabled(), reason: sp.GetReason(), grant: sp.GetGrant()}, nil
		}
	}
	if grantedBy != "" {
		return castable{}, connect.NewError(connect.CodeInvalidArgument, errors.New("granted_by is not a grant of this spell"))
	}
	for _, a := range opts.GetAttacks() {
		if a.GetAttack().GetKey() != key || a.GetAttack().GetKind() != rulesv1.AttackKind_ATTACK_KIND_SPELL {
			continue
		}
		if a.GetAttack().GetSaveDc() == 0 {
			return castable{}, connect.NewError(connect.CodeInvalidArgument, errors.New("this cantrip is an attack roll: use RollAttack"))
		}
		return castable{economy: rulesv1.ActionEconomy_ACTION_ECONOMY_ACTION, enabled: a.GetEnabled(), reason: a.GetReason()}, nil
	}
	return castable{}, connect.NewError(connect.CodeInvalidArgument, errors.New("spell_key is not one of the caster's spells"))
}

// castError is the failed_precondition (or invalid_argument) for a reason a
// spell option is disabled with. The master has the last word on the economy:
// an action already used does not stop him (ignoreEconomy).
func castError(r *rulesv1.DisabledReason, ignoreEconomy bool) error {
	switch r.GetCode() {
	case rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_ACTION_USED:
		if ignoreEconomy {
			return nil
		}
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ACTION_USED, "the action of this turn is used")
	case rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_BONUS_ACTION_USED:
		if ignoreEconomy {
			return nil
		}
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_BONUS_ACTION_USED, "the bonus action of this turn is used")
	case rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_BONUS_ACTION_SPELL_LIMIT:
		if ignoreEconomy {
			return nil
		}
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_BONUS_ACTION_SPELL_LIMIT,
			"a spell cast with a bonus action leaves no other spell this turn but a cantrip of 1 action")
	case rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_NO_USES:
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NO_USES, "no uses left",
			func(b *playv1.EncounterBlocked) { b.Recharge = r.GetRecharge() })
	case rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_NO_SLOT:
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NO_SLOT, "there is no free spell slot for this spell",
			func(b *playv1.EncounterBlocked) { b.MinLevel = r.GetMinLevel() })
	case rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_REACTION_ONLY_WHEN_HIT, rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_REACTION_ONLY:
		return connect.NewError(connect.CodeInvalidArgument, errors.New("a reaction spell is cast when its trigger happens (UseReaction, for Escudo)"))
	case rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_CASTING_TIME_TOO_LONG:
		return connect.NewError(connect.CodeInvalidArgument, errors.New("this spell takes too long to cast in a fight"))
	case rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_ARMOR_NOT_PROFICIENT:
		return errArmorNotProficient()
	}
	return connect.NewError(connect.CodeInvalidArgument, errors.New("this spell cannot be cast now"))
}

// slotOf checks the slot the caster chose against the free ones the options
// list; nil for a cantrip. invalid_argument for a slot that does not fit.
func slotOf(in *playv1.SpellSlot, c castable) (*slotRef, error) {
	if c.level == 0 {
		if in != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("a cantrip is cast with no slot"))
		}
		return nil, nil
	}
	if c.grant != nil {
		// A granted spell is cast at its own level with no slot; a slot only if the grant allows it.
		if in == nil {
			return nil, nil
		}
		if !c.grant.GetAlsoWithSlot() {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("this grant does not cast the spell with a slot"))
		}
	}
	if in == nil || in.GetLevel() < c.level || in.GetLevel() > 9 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("slot must be a spell slot of at least the spell's level"))
	}
	if !slices.ContainsFunc(c.slots, func(s *rulesv1.SlotChoice) bool { return s.GetLevel() == in.GetLevel() && s.GetPact() == in.GetPact() }) {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("slot is not one the caster has free"))
	}
	return &slotRef{Level: in.GetLevel(), Pact: in.GetPact()}, nil
}

// reachOf is how far a player's spell reaches from the caster in feet, and
// whether it has a limit at all: Self (that is not an area that comes out of
// the caster) reaches the caster alone, Touch 5 ft, a ranged spell its range
// (link.Spell.RangeFt, already multiplied for a spell attack when the caster's
// table feat has a ranged_attack that says so); sight, unlimited and special
// ranges leave it to the table.
func reachOf(sp link.Spell) (ft int32, limited bool) {
	switch sp.RangeKind {
	case rules.RangeSelf:
		return 0, !sp.Area
	case rules.RangeTouch:
		return meleeReachFt, true
	case rules.RangeRanged:
		return clamp32(sp.RangeFt, 0, math.MaxInt32), true
	}
	return 0, false
}

// selfOnly says the spell reaches the caster alone: Self, and not an area that
// comes out of the caster (Mãos Flamejantes).
func selfOnly(sp link.Spell) bool {
	return sp.CasterOnly || (sp.RangeKind == rules.RangeSelf && !sp.Area)
}

// maxTargetsOf is how many targets a spell takes at a slot level for a player:
// 0 means any number. An area takes any; a spell that says how many it takes (a
// table spell's target, or an SRD spell's worked out by the rules) takes that many
// and the extra ones of each slot level above its own, one attack roll for each
// when it is a spell attack (Raio Ardente a ray for each target); one that says
// nothing takes one, and one more for each level when the text says "an
// additional creature".
func maxTargetsOf(sp link.Spell, slotLevel int) int {
	switch {
	case selfOnly(sp):
		return 0
	case sp.Key == magicMissile:
		return dartsOf(sp, slotLevel) // a dart each, at most
	case sp.Key == scorchingRay:
		return scorchingRays(sp, slotLevel)
	case sp.Area:
		return 0
	case sp.TargetCount > 0:
		// What the spell says itself (a table spell's target, or an SRD spell's
		// worked out from its text): a spell attack takes that many too, one roll
		// for each.
		return sp.TargetCount + sp.TargetPerLevel*max(slotLevel-sp.Level, 0)
	case sp.AttackType != "":
		return 1
	case sp.ExtraTargetPerLevel:
		return 1 + max(slotLevel-sp.Level, 0)
	}
	return 1
}

// dartsOf is how many darts a slot level makes for the spell: Magic Missile's
// 3 + (level - 1), 0 for any other spell.
func dartsOf(sp link.Spell, slotLevel int) int {
	if sp.Key != magicMissile {
		return 0
	}
	return combat.MissileDarts(slotLevel)
}

// CastSpell implements playv1connect.CombatServiceHandler.
func (s *Service) CastSpell(
	ctx context.Context,
	req *connect.Request[playv1.CastSpellRequest],
) (*connect.Response[playv1.CastSpellResponse], error) {
	m, err := heldMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	// Whether a spell reveals the hidden creatures it hits is the master's to say.
	// A player who sends the choice is refused first, whatever else is wrong with the
	// request: another order would tell them what the rest of it holds (RN-10).
	if req.Msg.RevealHidden != nil && m.Role != authz.RoleMaster {
		return nil, errOnlyMasterReveals()
	}
	return s.castSpell(ctx, m, req)
}

// checkTargets checks, for a player, who a spell may touch: how many targets
// (the master is not held to the number), that Magic Missile's darts add up,
// and that each target is on the map and in range (the master is never held to
// the range, and nobody is without a grid: RN-25), and that no target of a single-target spell has total cover (D4).
// It does not read the database.
func (s *Service) checkTargets(v combatViewer, terrain grid.Terrain, cs []playdb.Combatant, sp link.Spell, caster playdb.Combatant, targs []playdb.Combatant, darts []int, dartsTotal, slotLevel int, theatre bool) error {
	if err := checkTargetCount(v, sp, caster, targs, darts, dartsTotal, slotLevel); err != nil {
		return err
	}
	if v.master {
		return nil
	}
	reach, limited := reachOf(sp)
	for _, t := range targs {
		// A target behind total cover (a wall on the line, or the master's mark)
		// cannot be targeted directly; an area spell may still include it and the
		// master judges.
		if t.ID != caster.ID && !sp.Area && coverAgainst(terrain, caster, t, coverPool(cs, v)).total() {
			return errCoverTotal()
		}
		if t.ID == caster.ID || !limited || theatre { // without a grid the master judges the range (RN-25)
			continue
		}
		if err := checkTargetReach(caster, t, reach); err != nil {
			return err
		}
	}
	return nil
}

// badTargets is the refusal of a request whose targets a spell does not take.
func badTargets(msg string) error {
	return connect.NewError(connect.CodeInvalidArgument, errors.New(msg))
}

// checkTargetCount checks how many targets a spell takes and how the darts of Magic Missile are shared.
func checkTargetCount(v combatViewer, sp link.Spell, caster playdb.Combatant, targs []playdb.Combatant, darts []int, dartsTotal, slotLevel int) error {
	switch {
	case selfOnly(sp):
		if slices.ContainsFunc(targs, func(t playdb.Combatant) bool { return t.ID != caster.ID }) {
			return badTargets("this spell reaches the caster alone: leave the targets out")
		}
	case sp.Key == scorchingRay:
		return checkRays(v, sp, targs, darts, slotLevel)
	case dartsTotal > 0:
		return checkDarts(targs, darts, dartsTotal)
	default:
		if slices.ContainsFunc(darts, func(d int) bool { return d != 0 }) {
			return badTargets("darts are only for Magic Missile")
		}
		if !sp.Area && len(targs) == 0 {
			return badTargets("this spell needs a target")
		}
		if limit := maxTargetsOf(sp, slotLevel); limit > 0 && len(targs) > limit && !v.master {
			return badTargets(fmt.Sprintf("this spell takes at most %d targets with this slot", limit))
		}
	}
	return nil
}

// checkRays checks the rays of Scorching Ray: a target to aim at, and no more rays than the slot makes. The
// master is held to the most targets a cast takes instead, which keeps the cast's event small.
func checkRays(v combatViewer, sp link.Spell, targs []playdb.Combatant, rays []int, slotLevel int) error {
	if len(targs) == 0 {
		return badTargets("this spell needs a target")
	}
	total := 0
	for _, n := range rays {
		total += max(n, 1)
	}
	limit := scorchingRays(sp, slotLevel)
	if v.master {
		limit = maxSpellTargets
	}
	if total > limit {
		return badTargets(fmt.Sprintf("this spell takes at most %d rays with this slot", limit))
	}
	return nil
}

// checkDarts checks that every target of Magic Missile gets a dart and that the darts add up.
func checkDarts(targs []playdb.Combatant, darts []int, dartsTotal int) error {
	sum := 0
	for _, d := range darts {
		if d < 1 {
			return badTargets("every target of Magic Missile needs at least one dart")
		}
		sum += d
	}
	if len(targs) == 0 || sum != dartsTotal {
		return badTargets(fmt.Sprintf("the darts of the targets must add up to %d", dartsTotal))
	}
	return nil
}

// checkTargetReach checks that the caster and the target are on the map and that the target is in reach.
func checkTargetReach(caster, t playdb.Combatant, reach int32) error {
	if !placed(caster) || !placed(t) {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_PLACED, "the caster and the target must be on the map")
	}
	if dist, _ := distanceFt(caster, t); dist > reach {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TARGET_OUT_OF_REACH, "the target is beyond the spell's range",
			func(b *playv1.EncounterBlocked) { b.MissingFt = dist - reach })
	}
	return nil
}

// resolveOnTarget does what the spell does to one target and fills the hit: a
// spell attack's roll, a saving throw's roll, and the pending damage or heal
// the cast opens. Anything else leaves the hit empty. made is the cast's event: a spell attack
// keeps in it what the roll used up, for the cast's undo.
func (s *Service) resolveOnTarget(ctx context.Context, c *combatTx, m authz.Membership, sp link.Spell, caster, target playdb.Combatant, slotLevel int, in rollInput, cp coverPair, hit *castHit, modes *spellModes, made *actionEvent) error {
	// A table feat's ranged_attack effect: the caster's ranged spell attack gets past
	// half and three-quarters cover (total cover was refused before).
	cp = cp.ignoring(modes != nil && spellIgnoresCover(modes.traits, sp))
	cover := cp.real
	// The cover counts for a spell attack and a Dexterity save (D4).
	if sp.AttackType != "" || (sp.SaveAbility == "dex" && !cover.total() && !sp.IgnoresCover) {
		hit.Cover, hit.CoverSource, hit.CoverBonus = cover.key(), cover.sourceKey(), clamp32(cover.bonus(), 0, 5)
		hit.CoverRestricted, hit.CoverSeenBy = cp.restricted, cp.seenBy
	}
	switch {
	case sp.AttackType != "":
		return s.spellAttack(ctx, c, m, sp, caster, target, in, cover, hit, modes, made)
	case sp.SaveAbility != "":
		return s.spellSave(ctx, c, m, sp, caster, target, cover, hit, modes)
	case dartsOf(sp, slotLevel) > 0:
		return s.openDarts(ctx, c, sp, caster, target, hit)
	case sp.Heal != nil:
		return s.openSpellPending(ctx, c, sp, *sp.Heal, caster, target, true, false, hit)
	}
	return nil
}

// d20 rolls the d20 of a roll in the app, or checks the face typed from a
// physical die; the face and the roll's total (the modifier added).
func (s *Service) d20(in rollInput, modifier int) (face int, roll dice.Result, err error) {
	expr := dice.Expr{Count: 1, Sides: 20, Modifier: modifier}
	if in.inApp {
		if roll, err = dice.Roll(s.roller, expr); err != nil {
			return 0, dice.Result{}, fmt.Errorf("roll the d20: %w", err)
		}
		return roll.Faces[0], roll, nil
	}
	if roll, err = dice.Physical(expr, in.typed); err != nil {
		return 0, dice.Result{}, connect.NewError(connect.CodeInvalidArgument, errors.New("d20_face must be 1 to 20"))
	}
	return in.typed, roll, nil
}

// spellAttackRolled is the d20 of a spell attack: the mode as it was worked out, the choice made on it
// and the gate that rolled the die.
type spellAttackRolled struct {
	truth  modeResult
	choice modeChoice
	step   gateResult
}

// spellAttackD20 settles the mode of a spell attack's roll, as RollAttack does, and rolls its d20. The
// answer to a Bardic Inspiration question brings the mode and the d20 it was held with.
func (s *Service) spellAttackD20(ctx context.Context, c *combatTx, sp link.Spell, caster, target playdb.Combatant, in rollInput, modes *spellModes) (spellAttackRolled, error) {
	if pre := modes.pre; pre != nil && pre.target == target.ID {
		return spellAttackRolled{truth: pre.truth, choice: pre.choice, step: pre.step}, nil
	}
	var r spellAttackRolled
	var err error
	r.truth = modes.in.facts(c.enc, c.sight).attackMode(caster, target, modes.traits, shapeOfSpell(sp), actsNow(c.enc, caster), modes.reader, modes.names)
	if r.choice, err = s.chooseMode(ctx, c, modes.v, caster, target, sp.Key, r.truth.Mode, modes.ask); err != nil {
		return r, err
	}
	// This roll waits for nobody (MayAsk is false): the gate rolls the d20 and settles it.
	r.step, err = s.heldGate(ctx, combatGate(c), rollSpec{Kind: heldSpellAttack, In: in, Modifier: sp.ToHit, Mode: r.choice.Mode})
	return r, err
}

// spendSpellAttackRoll uses up what the roll used: the Help aimed at the target and Zombaria Viciosa's
// disadvantage, which a second attack of the cast reads spent. made keeps both, for the cast's undo.
func (s *Service) spendSpellAttackRoll(ctx context.Context, c *combatTx, caster, target playdb.Combatant, modes *spellModes, made *actionEvent) error {
	used, err := s.consumeHelps(ctx, c, caster, target, modes.in.cs) // the Help aimed at the target is used (SRD 5.1, "Help")
	if err != nil {
		return err
	}
	made.HelpUsed = append(made.HelpUsed, used...)
	if err := s.spendAttackEffects(ctx, c, caster, made); err != nil {
		return err
	}
	markSpent(modes.in.states, caster.ID, made.EffectsSpent, c.now)
	return nil
}

// spellAttack rolls a spell attack against a target, as RollAttack does, and
// opens its pending damage on a hit. Like an attack (afterAttack), the roll uses the
// Help aimed at the target and spends the effect that gave the caster's next attack
// roll disadvantage; made keeps both, for the cast's undo.
func (s *Service) spellAttack(ctx context.Context, c *combatTx, m authz.Membership, sp link.Spell, caster, target playdb.Combatant, in rollInput, cover coverView, hit *castHit, modes *spellModes, made *actionEvent) error {
	rolled, err := s.spellAttackD20(ctx, c, sp, caster, target, in, modes)
	if err != nil {
		return err
	}
	step := rolled.step
	hit.Lucky = step.Lucky // the d20 Lucky rolled again: the cast's line says "Sortudo: 1 → 14"
	hit.Stroke = step.Stroke
	bonusFace := 0
	if bonus := step.Bonus; bonus != nil {
		bonusFace = int(bonus.Face) // the die the player added after rolling
		hit.BonusSides, hit.BonusFace = bonus.Sides, bonus.Face
	}
	d20 := step.Roll
	modes.attack[target.ID] = rolled.truth.Shown
	if err := s.spendSpellAttackRoll(ctx, c, caster, target, modes, made); err != nil {
		return err
	}
	choice := rolled.choice
	hit.D20B, hit.Counted, hit.RollMode, hit.SuggestedMode, hit.ReasonID = clamp32(d20.otherFace(), 0, 20), clamp32(d20.Index, 0, 1), modeKey(choice.Mode), modeKey(choice.Suggested), choice.ReasonID
	result, targetAC, err := s.resolveSpellAttack(ctx, c, m, sp, caster, target, cover, hit, modes, rolled, bonusFace)
	if err != nil {
		return err
	}
	return s.openSpellAttackHit(ctx, c, sp, caster, target, hit, result, targetAC)
}

// resolveSpellAttack works the armor class of the target out, rolls the dice that add to the attack and
// settles the hit; hit keeps the roll as the cast shows it.
func (s *Service) resolveSpellAttack(ctx context.Context, c *combatTx, m authz.Membership, sp link.Spell, caster, target playdb.Combatant, cover coverView, hit *castHit, modes *spellModes, rolled spellAttackRolled, bonusFace int) (combat.AttackResult, int, error) {
	d20 := rolled.step.Roll
	face := d20.Face()
	sheet, err := s.sheetOf(ctx, c.tx, m.CampaignID, target)
	if err != nil {
		return combat.AttackResult{}, 0, err
	}
	targetAC := sheet.ArmorClass + int(target.AcBonus) + int(target.EffectAcBonus) + cover.bonus()
	// Bênção and Perdição (SRD 5.1): the d4 the app rolls for the caster's spell attack.
	extra, extraTotal, err := s.rollEffectDice(rollInput{inApp: true}, effectDiceFor(modes.in.states, caster.ID, rules.RollAppliesAttack), nil)
	if err != nil {
		return combat.AttackResult{}, 0, err
	}
	hit.Extra = extra
	result := combat.ResolveAttack(sp.ToHit+bonusFace+extraTotal, targetAC, face)
	if rolled.step.Stroke != nil && !result.Hit { // Stroke of Luck: the miss the caster turned into a hit
		result.Hit, result.Fumble = true, false
	}
	if result.Hit && rolled.truth.CriticalOnHit { // a paralyzed or unconscious target within 5 ft (SRD 5.1)
		result.Critical = true
	}
	hit.TargetAC = clamp32(targetAC, 0, math.MaxInt32)
	hit.D20, hit.Modifier, hit.Total, hit.Physical = clamp32(face, 1, 20), conv.Int32(sp.ToHit+bonusFace), conv.Int32(result.Total), d20.Physical
	return result, targetAC, nil
}

// openSpellAttackHit says the outcome of a spell attack and opens its pending damage on a hit.
func (s *Service) openSpellAttackHit(ctx context.Context, c *combatTx, sp link.Spell, caster, target playdb.Combatant, hit *castHit, result combat.AttackResult, targetAC int) error {
	hit.Outcome = outcomeMiss
	if !result.Hit {
		return s.openSplash(ctx, c, sp, caster, target, hit)
	}
	hit.Outcome = outcomeHit
	if result.Critical {
		hit.Outcome = outcomeCrit
	}
	if len(sp.Damages) == 0 {
		return nil
	}
	// A spell attack deals its first damage type: no spell of the SRD that rolls an
	// attack lists a second one.
	p, err := s.openHit(ctx, c, caster, target, sp.Key, sp.Damages[0], result.Critical, 0, 0, result.Total, targetAC)
	if err != nil {
		return err
	}
	hit.Pending = p.ID
	// A part the caster's table options added (another damage type) is its own damage, apart from the hit.
	for _, part := range sp.Damages[1:] {
		if err := s.openSpellPending(ctx, c, sp, part, caster, target, false, false, hit); err != nil {
			return err
		}
	}
	return nil
}

// openSplash opens the half damage a spell attack that misses still deals (Acid Arrow), nothing
// for any other spell.
func (s *Service) openSplash(ctx context.Context, c *combatTx, sp link.Spell, caster, target playdb.Combatant, hit *castHit) error {
	if !sp.HalfOnMiss || len(sp.Damages) == 0 {
		return nil
	}
	return s.openSpellPending(ctx, c, sp, sp.Damages[0], caster, target, false, true, hit)
}

// spellSave has the server roll a target's saving throw against the spell and
// opens the damage the save leaves: all of it for a target that failed, half
// (rounded down) for one that saved when the spell halves, none when it avoids.
// The d20 is always rolled by the app (RN-18 is for the table's own dice).
func (s *Service) spellSave(ctx context.Context, c *combatTx, m authz.Membership, sp link.Spell, caster, target playdb.Combatant, cover coverView, hit *castHit, modes *spellModes) error {
	if c.sculpted[target.ID] {
		return s.sculptedSave(ctx, c, sp, caster, target, hit)
	}
	save, err := s.saveOf(ctx, c.tx, m.CampaignID, target, sp.SaveAbility)
	if err != nil {
		return err
	}
	targetSheet, creature, against, err := s.spellSaveScene(ctx, c, m.CampaignID, sp, target, modes)
	if err != nil {
		return err
	}
	sources := combat.SaveMode(combat.SaveScene{Creature: creature, Ability: sp.SaveAbility, EffectVisible: true, Against: against})
	modes.save[target.ID] = shownSources(sources, modes.names)
	// Cover adds to a Dexterity save (SRD): half +2, three-quarters +5. The bonus
	// the save shows already has it.
	if sp.SaveAbility == "dex" && !cover.total() && !sp.IgnoresCover { // Chama Sagrada: no benefit from cover (SRD)
		save.Bonus += cover.bonus()
	}
	var saved bool
	if combat.AutoFailsSave(creature, sp.SaveAbility) {
		// Stunned, paralyzed, unconscious and petrified creatures fail Strength and
		// Dexterity saves without rolling (SRD 5.1, Conditions).
		hit.Save = &saveRoll{DC: clamp32(sp.SaveDC, 0, math.MaxInt32), Auto: true, Unknown: !save.Known}
	} else if saved, err = s.rollSpellSave(ctx, c, sp, target, save, targetSheet, sources, combat.Resolve(sources), hit, modes); err != nil {
		return err
	}
	if len(sp.Damages) == 0 || (saved && sp.SaveOnSuccess != "half" && sp.SaveOnSuccess != "other") {
		return nil
	}
	share := combat.ShareFull
	if sp.SaveOnSuccess == "half" {
		// Evasion: a Dexterity save that passes takes none, one that fails takes half.
		share = combat.HalfSaveShare(creature, sp.SaveAbility, saved)
	}
	if share == combat.ShareNone {
		return nil
	}
	// One pending damage for each damage type, all of the cast: each is its own
	// roll, and the resistance of the target is read for its type when it lands.
	return s.openSavePendings(ctx, c, sp, caster, target, share == combat.ShareHalf, hit)
}

// sculptedSave is the save of a creature Sculpt Spells (SRD 5.1) spares: it succeeds on its own, with no
// roll, and takes no damage when the spell would deal half on a success.
func (s *Service) sculptedSave(ctx context.Context, c *combatTx, sp link.Spell, caster, target playdb.Combatant, hit *castHit) error {
	hit.Save = &saveRoll{DC: clamp32(sp.SaveDC, 0, math.MaxInt32), Saved: true, Sculpted: true}
	if len(sp.Damages) == 0 || sp.SaveOnSuccess != "other" {
		return nil
	}
	return s.openSavePendings(ctx, c, sp, caster, target, false, hit)
}

// openSavePendings opens a pending damage for each damage type of the spell.
func (s *Service) openSavePendings(ctx context.Context, c *combatTx, sp link.Spell, caster, target playdb.Combatant, half bool, hit *castHit) error {
	for _, part := range sp.Damages {
		if err := s.openSpellPending(ctx, c, sp, part, caster, target, false, half, hit); err != nil {
			return err
		}
	}
	return nil
}

// rollSpellSave rolls a target's saving throw against the spell and fills the hit with it; it says whether
// the target saved.
func (s *Service) rollSpellSave(ctx context.Context, c *combatTx, sp link.Spell, target playdb.Combatant, save link.Save, targetSheet link.Sheet, sources []combat.Source, mode combat.RollMode, hit *castHit, modes *spellModes) (bool, error) {
	saveMode := mode
	if c.meta.disadvantaged(target.ID) { // Heightened Spell: disadvantage on its first save, which an advantage cancels
		saveMode = combat.Resolve(append(slices.Clone(sources), combat.Source{Kind: "heightened-spell", Effect: combat.ModeDisadvantage}))
	}
	d20, err := s.d20With(rollInput{inApp: true}, save.Bonus, saveMode)
	if err != nil {
		return false, err
	}
	// Nobody is asked about this save: a halfling player's 1 is rolled again by the app (combat_lucky.go).
	d20, lucky, err := s.luckySave(d20, save.Bonus, saveMode, luckySaver(target, targetSheet.Traits))
	if err != nil {
		return false, err
	}
	// Bênção and Perdição: a d4 on the target's saving throw.
	extra, extraTotal, err := s.rollEffectDice(rollInput{inApp: true}, effectDiceFor(modes.in.states, target.ID, rules.RollAppliesSave), nil)
	if err != nil {
		return false, err
	}
	if err := s.spendOnceEffects(ctx, c, extra); err != nil {
		return false, err
	}
	saved := combat.SaveSucceeded(d20.Total+extraTotal, sp.SaveDC) || c.meta.passes(target.ID) // Careful Spell: it passes on its own
	hit.Save = &saveRoll{
		Extra: extra,
		D20:   clamp32(d20.Face(), 1, 20), Bonus: conv.Int32(save.Bonus), Total: conv.Int32(d20.Total + extraTotal),
		DC: clamp32(sp.SaveDC, 0, math.MaxInt32), Saved: saved, Unknown: !save.Known,
		D20B: clamp32(d20.otherFace(), 0, 20), Counted: clamp32(d20.Index, 0, 1), RollMode: modeKey(saveMode), Lucky: lucky,
	}
	return saved, nil
}

// openDarts opens Magic Missile's damage for a target: its darts, each one
// 1d4 + 1 (the spell's damage at its own level divided by its three darts).
func (s *Service) openDarts(ctx context.Context, c *combatTx, sp link.Spell, caster, target playdb.Combatant, hit *castHit) error {
	if len(sp.Damages) == 0 {
		return nil
	}
	volley := sp.Damages[0]
	// The damage at the cast's slot level is the whole volley (3d4 + 3 at the 1st
	// level, 6d4 + 6 at the 4th): one dart is always 1d4 + 1, whatever the level.
	darts := max(volley.Count, 1)
	perDart := link.Dice{Count: 1, Sides: volley.Sides, Bonus: volley.Bonus / darts, DamageType: volley.DamageType}
	n := int(hit.Darts)
	// A dart is its own roll: n darts are n dice and n times the bonus. The cast
	// has no cast id for them: each target's damage is its own roll.
	dmg := link.Dice{Count: perDart.Count * n, Sides: perDart.Sides, Bonus: perDart.Bonus * n, DamageType: perDart.DamageType}
	// The darts hit by themselves: a target that can cast Escudo may stop all of them
	// (SRD, Shield), and the master's check of "Sempre" waits for each target too.
	specs, err := s.hitWindows(ctx, c, caster, target, sp.Key, false, true, 0, 0)
	if err != nil {
		return err
	}
	status := pendingAwaitingRoll
	if len(specs) > 0 {
		status = pendingAwaitingReaction
	}
	p, err := c.q.InsertPendingDamage(ctx, playdb.InsertPendingDamageParams{
		EncounterID: c.enc.ID, AttackerID: &caster.ID, TargetID: target.ID, AttackKey: sp.Key, Status: status,
		DiceCount: clamp32(dmg.Count, 0, 100), DiceSides: clamp32(dmg.Sides, 0, 100), DiceBonus: clamp32(dmg.Bonus, -1000, 1000), DamageType: dmg.DamageType,
		CreatedAt: c.now,
	})
	if err != nil {
		return fmt.Errorf("open the pending damage: %w", err)
	}
	if err := s.openPendingWindows(ctx, c, p, specs); err != nil {
		return err
	}
	hit.Pending = p.ID
	return nil
}

// openSpellPending opens a damage or a heal of a spell for a target, in the
// cast's group (the pending damages of one cast share an id: an area spell's
// damage is one roll for each damage type). The first one a target gets is the
// hit's Pending, the others go to its More.
func (s *Service) openSpellPending(ctx context.Context, c *combatTx, sp link.Spell, dmg link.Dice, caster, target playdb.Combatant, healing, half bool, hit *castHit) error {
	castID := c.castID
	p, err := c.q.InsertPendingDamage(ctx, playdb.InsertPendingDamageParams{
		EncounterID: c.enc.ID, AttackerID: &caster.ID, TargetID: target.ID, AttackKey: sp.Key, Status: pendingAwaitingRoll,
		DiceCount: clamp32(dmg.Count, 0, 100), DiceSides: clamp32(dmg.Sides, 0, 100), DiceBonus: clamp32(dmg.Bonus, -1000, 1000), DamageType: dmg.DamageType,
		CreatedAt: c.now, CastID: &castID, Healing: healing, Half: half,
	})
	if err != nil {
		return fmt.Errorf("open the pending damage: %w", err)
	}
	if hit.Pending == "" {
		hit.Pending = p.ID
	} else {
		hit.More = append(hit.More, p.ID)
	}
	return nil
}

// spellSaveScene is what the saving throw of a target against a spell needs: the target's sheet, what the combat
// knows of it and what the save is against.
func (s *Service) spellSaveScene(ctx context.Context, c *combatTx, campaignID string, sp link.Spell, target playdb.Combatant, modes *spellModes) (link.Sheet, combat.Creature, []string, error) {
	targetSheet, err := s.sheetOf(ctx, c.tx, campaignID, target)
	if err != nil {
		return link.Sheet{}, combat.Creature{}, nil, err
	}
	against, err := s.spellSaveAgainst(ctx, c, sp)
	return targetSheet, creatureFacts(target, modes.in.states, targetSheet.Traits), against, err
}

// spellSaveAgainst is what the saving throw against a spell is against: its own kinds (spellAgainst) and the
// conditions its lasting effect puts on a target that fails, which a roll mode against a condition reads.
func (s *Service) spellSaveAgainst(ctx context.Context, c *combatTx, sp link.Spell) ([]string, error) {
	against := spellAgainst(sp)
	content, err := s.contentOf(ctx, c)
	if err != nil {
		return nil, err
	}
	if def, ok := content.CombatSpellEffect(sp.Key); ok {
		against = withSaveConditions(against, def.Conditions)
	}
	return against, nil
}
