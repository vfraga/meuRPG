package play

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dice"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/platform/tablerules"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/live"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// What a turn can do (MR-012, MR-014, Etapa 6, slice 6.4a): the options of a
// combatant, the attack in two steps (the roll to hit, then the damage), the
// standard actions that only spend the economy, and the master's hand on an
// NPC's hit points. The helpers are in combat_write.go, the visibility in
// combat_view.go; the undo is in combat_undo.go and the log in
// combat_log.go.
//
// An attack is two writes because a physical d20 and its damage dice are
// typed one after the other (RN-18). The hit opens a pending_damages row; an
// NPC target takes the damage at once, a player's character waits for the
// master (RN-02) in "rolled" until he applies or discards it.

// The most damage or healing the master types for an NPC, and the most
// temporary hit points (as for a character's vitals).
const (
	maxHitPointChange = 9999
	maxTempHitPoints  = 999
	// meleeReachFt is the reach of a melee attack that says none: 5 ft.
	meleeReachFt = 5
)

// damageTypePT names the damage types in Portuguese for the log; the keys are
// the content's ("damage-type:fire").
var damageTypePT = map[string]string{
	"damage-type:acid": "ácido", "damage-type:bludgeoning": "concussão", "damage-type:cold": "frio",
	"damage-type:fire": "fogo", "damage-type:force": "energia", "damage-type:lightning": "elétrico",
	"damage-type:necrotic": "necrótico", "damage-type:piercing": "perfurante", "damage-type:poison": "veneno",
	"damage-type:psychic": "psíquico", "damage-type:radiant": "radiante", "damage-type:slashing": "cortante",
	"damage-type:thunder": "trovejante",
}

// The standard actions that are not taken through TakeAction.
const (
	standardAttack = "standard:attack"
	standardCast   = "standard:cast-a-spell"
)

// gate says why the combatant cannot act now, or UNSPECIFIED when it can: the
// combat is not running, the combatant is out of the fight or not on turn, or
// a player's character is down. GetTurnOptions disables every option with it,
// and the writes refuse with the matching EncounterBlocked.
func (s *Service) gate(ctx context.Context, tx pgx.Tx, campaignID string, e playdb.Encounter, c playdb.Combatant) (rulesv1.DisabledReasonCode, error) {
	switch {
	case e.Status != statusActive:
		return rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_COMBAT_NOT_ACTIVE, nil
	case c.Defeated:
		return rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_COMBATANT_DEFEATED, nil
	case !actsNow(e, c): // its group is not on turn, or its part of a joint turn ended
		return rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_NOT_YOUR_TURN, nil
	}
	down, err := s.isDown(ctx, tx, campaignID, c)
	if err != nil {
		return 0, err
	}
	if down {
		return rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_COMBATANT_DOWN, nil
	}
	return rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_UNSPECIFIED, nil
}

// isDown says whether a player's character is at 0 hit points ("Caído"): its
// vitals say so. A character that left the sheet's active state (it died, and the
// combat was not told) is out of the fight too: down, never an error. An NPC and a
// creature are never down: they are defeated.
// A write passes its transaction, so it reads what it will overwrite; a read
// passes nil.
func (s *Service) isDown(ctx context.Context, tx pgx.Tx, campaignID string, c playdb.Combatant) (bool, error) {
	if c.Kind != kindPlayer {
		return false, nil
	}
	var v *playv1.CharacterVitals
	var err error
	if tx != nil {
		v, err = s.vitals.GetVitalsTx(ctx, tx, campaignID, c.CharacterID)
	} else {
		v, err = s.vitals.GetVitals(ctx, campaignID, c.CharacterID)
	}
	if connect.CodeOf(err) == connect.CodeNotFound {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return v.GetHitPointsMax() > 0 && v.GetHitPointsCurrent() == 0, nil
}

// gateError is the failed_precondition a write answers with for a reason of
// gate.
func gateError(code rulesv1.DisabledReasonCode) error {
	switch code {
	case rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_COMBAT_NOT_ACTIVE:
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_ACTIVE, "the combat is not running")
	case rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_COMBATANT_DOWN:
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_COMBATANT_DOWN, "the character is down")
	}
	// Not on turn, or out of the fight (a defeated combatant has no turn).
	return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_YOUR_TURN, "it is not this combatant's turn")
}

// mustNotBeDown refuses a player's character at 0 hit points: a down character
// does not move (RN-03, RN-21). tx is the open transaction, or nil for a read.
func (s *Service) mustNotBeDown(ctx context.Context, tx pgx.Tx, campaignID string, who playdb.Combatant) error {
	down, err := s.isDown(ctx, tx, campaignID, who)
	if err != nil {
		return err
	}
	if down {
		return gateError(rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_COMBATANT_DOWN)
	}
	return nil
}

// mustActNow checks, inside a write, that the combatant is the one that may
// act now (on turn, in a running combat, not down).
func (s *Service) mustActNow(ctx context.Context, c *combatTx, who playdb.Combatant) error {
	if err := notEnded(c.enc); err != nil {
		return err
	}
	code, err := s.gate(ctx, c.tx, c.session.CampaignID, c.enc, who)
	if err != nil {
		return err
	}
	if code != rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_UNSPECIFIED {
		return gateError(code)
	}
	if err := s.mustNotHold(ctx, c); err != nil { // a question waits for the master: nobody's turn goes on
		return err
	}
	return s.mustNotWait(ctx, c, who)
}

func turnOf(c playdb.Combatant) link.Turn {
	return link.Turn{
		ActionUsed: c.ActionUsed, BonusActionUsed: c.BonusActionUsed, ReactionUsed: c.ReactionUsed, AttacksMade: int(c.AttacksMade), AttackKey: deref(c.ActionAttackKey), FlurryLeft: int(c.BonusAttacksLeft), ActionSurged: c.ActionSurged, SpellCast: c.SpellCast, BonusSpellCast: c.BonusSpellCast,
		Dashed: c.Dashed, SpeedFt: int(max(c.SpeedFt, c.SpeedFlyFt)), MovementUsedFt: int(c.MovementUsedDft) / 10,
		MovementUsedDFt: int(c.MovementUsedDft), LastMoveDFt: int(c.LastMoveDft), Disengaged: c.Disengaged,
		JumpLongDFt: int(c.JumpLongDft), JumpHighDFt: int(c.JumpHighDft),
	}
}

// diceRoll builds the API's DiceRoll.
func diceRoll(count, sides int32, faces []int32, modifier, total int32, physical bool) *playv1.DiceRoll {
	return &playv1.DiceRoll{DiceCount: count, DiceSides: sides, Faces: faces, Modifier: modifier, Total: total, Physical: physical}
}

// faces32 converts the faces a roll gave.
func faces32(faces []int) []int32 {
	out := make([]int32, len(faces))
	for i, f := range faces {
		out[i] = clamp32(f, 0, 1000)
	}
	return out
}

// reachFt is how far an attack reaches: its range, its long range, or 5 ft
// for a melee attack that says none.
func reachFt(rangeFt, longRangeFt int32) int32 {
	return max(rangeFt, longRangeFt, meleeReachFt)
}

// attackReach is how far the attack reaches for a player: its range, or for an
// opportunity attack the melee reach, whatever range the weapon has when thrown.
func attackReach(a link.Attack, asReaction bool) int32 {
	if asReaction {
		return meleeReachFt
	}
	return reachFt(clamp32(a.RangeFt, 0, math.MaxInt32), clamp32(a.LongRangeFt, 0, math.MaxInt32))
}

// distanceFt is the distance between two combatants on the grid for an attack's
// reach or a spell's range: the squares between the centers, rounded down, times
// 5 ft (RN-21, grid.RangeFt), so a diagonal neighbor is 5 ft away; false when
// either has no square.
func distanceFt(a, b playdb.Combatant) (int32, bool) {
	if !placed(a) || !placed(b) {
		return 0, false
	}
	return clamp32(grid.RangeFt(squareOfCombatant(a), squareOfCombatant(b)), 0, math.MaxInt32), true
}

// GetTurnOptions implements playv1connect.CombatServiceHandler.
func (s *Service) GetTurnOptions(
	ctx context.Context,
	req *connect.Request[playv1.GetTurnOptionsRequest],
) (*connect.Response[playv1.GetTurnOptionsResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	encID, err := parseCombatID(req.Msg.GetEncounterId(), "encounter")
	if err != nil {
		return nil, err
	}
	combID, err := parseCombatID(req.Msg.GetCombatantId(), "combatant")
	if err != nil {
		return nil, err
	}
	_, d, err := s.readEncounter(ctx, m.CampaignID, encID)
	if err != nil {
		return nil, err
	}
	enc := d.enc
	v, sight, err := s.viewerWith(ctx, m, enc, d.cs)
	if err != nil {
		return nil, s.dbError(ctx, "work out what the player sees", err)
	}
	who, err := findCombatant(d.cs, combID, v)
	if err != nil {
		return nil, err
	}
	if err := v.mayAct(who); err != nil {
		return nil, err
	}

	opts, err := s.optionsOf(ctx, nil, m.CampaignID, who)
	if err != nil {
		return nil, s.dbError(ctx, "work out the turn options", err)
	}
	code, err := s.gate(ctx, nil, m.CampaignID, enc, who)
	if err != nil {
		return nil, s.dbError(ctx, "check the combatant's turn", err)
	}
	if code != rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_UNSPECIFIED {
		disableAll(opts, code)
	}
	if err := s.markSilenced(ctx, m.CampaignID, d, who, opts); err != nil {
		return nil, s.dbError(ctx, "work out the spells a silence stops", err)
	}
	terrain, err := s.terrainOf(ctx, nil, m.CampaignID, enc)
	if err != nil {
		return nil, s.dbError(ctx, "read the terrain", err)
	}
	// The cover a player is shown, and the targets it makes untargetable, come from the
	// terrain they know and the creatures they see.
	known, err := sight.knownTerrain(ctx, nil, v)
	if err != nil {
		return nil, s.dbError(ctx, "read the terrain the player knows", err)
	}
	terrain = planOn(v, terrain, known)
	table, err := s.tableRules(ctx, nil, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "read the table's rules", err)
	}
	res := &playv1.GetTurnOptionsResponse{Options: opts, YourTurn: actsNow(enc, who), CriticalRule: criticalRuleProto(table)}
	for _, a := range opts.GetAttacks() {
		if a.GetAttack().GetSaveDc() > 0 {
			continue // a saving throw, not an attack roll: the spells slice
		}
		res.AttackTargets = append(res.AttackTargets, &playv1.AttackTargets{
			AttackKey: a.GetAttack().GetKey(), Targets: targetsFor(terrain, d.cs, who, v, reachFt(a.GetAttack().GetRangeFt(), a.GetAttack().GetLongRangeFt()), isTheatre(enc)),
		})
	}
	if res.SpellTargets, err = s.spellTargetsFor(ctx, m.CampaignID, terrain, who, d.cs, v, opts, isTheatre(enc)); err != nil {
		return nil, s.dbError(ctx, "work out the spell targets", err)
	}
	open, err := s.queries.ListOpenPendingDamages(ctx, enc.ID)
	if err != nil {
		return nil, s.dbError(ctx, "list the pending damage", err)
	}
	for _, p := range open {
		if deref(p.AttackerID) == who.ID && pendingVisible(p, d.cs, v) {
			res.PendingDamages = append(res.PendingDamages, pendingProto(p, d.cs))
		}
	}
	return connect.NewResponse(res), nil
}

// spellTargetsFor lists, for each spell the combatant can cast now and each save
// cantrip, who it can target as the viewer sees them, with the distance, whether
// the spell cannot reach it, how many targets it takes and, for Magic Missile,
// the darts of each slot level. Spells that cannot be cast now are left out:
// they carry their reason and need no targets.
func (s *Service) spellTargetsFor(ctx context.Context, campaignID string, terrain grid.Terrain, who playdb.Combatant, cs []playdb.Combatant, v combatViewer, opts *rulesv1.TurnOptions, theatre bool) ([]*playv1.SpellTargets, error) {
	type entry struct {
		key   string
		level int
		slots []*rulesv1.SlotChoice
	}
	var spells []entry
	for _, sp := range opts.GetSpells() {
		if sp.GetEnabled() {
			spells = append(spells, entry{sp.GetSpell().GetKey(), int(sp.GetSpell().GetLevel()), sp.GetSlots()})
		}
	}
	for _, a := range opts.GetAttacks() {
		if a.GetEnabled() && a.GetAttack().GetKind() == rulesv1.AttackKind_ATTACK_KIND_SPELL && a.GetAttack().GetSaveDc() > 0 {
			spells = append(spells, entry{key: a.GetAttack().GetKey()})
		}
	}
	var out []*playv1.SpellTargets
	for _, e := range spells {
		sp, err := s.roster.CombatSpell(ctx, nil, campaignID, who.CharacterID, e.key, e.level, "")
		if err != nil {
			return nil, err
		}
		st := &playv1.SpellTargets{
			SpellKey: e.key, Targets: spellTargetList(terrain, cs, who, v, sp, theatre),
			Placement: placementFor(sp, theatre), AreaShape: areaShapeProto(sp, theatre), AreaSizeFt: areaSizeFor(sp, theatre),
			AreaWidthFt: areaWidthFor(sp, theatre), RangeFt: placedRangeFor(sp, theatre),
			MaxTargets: clamp32(maxTargetsOf(sp, e.level), 0, maxCombatants), ExtraTargetPerLevel: sp.ExtraTargetPerLevel || sp.Key == magicMissile,
			TargetsPerLevel: clamp32(sp.TargetPerLevel, 0, maxCombatants),
		}
		for _, slot := range e.slots {
			if n := dartsOf(sp, int(slot.GetLevel())); n > 0 && !slices.ContainsFunc(st.Darts, func(d *playv1.DartsAtSlot) bool { return d.GetSlotLevel() == slot.GetLevel() }) {
				st.Darts = append(st.Darts, &playv1.DartsAtSlot{SlotLevel: slot.GetLevel(), Darts: clamp32(n, 0, 100)})
			}
		}
		out = append(out, st)
	}
	return out, nil
}

// spellTargetList lists who a spell can target as the viewer sees them: the
// caster itself and every combatant that is not defeated (one at 0 hit points
// can be healed), in turn order, with the distance and whether the spell's range
// cannot reach it. A spell that stays on the caster lists only the caster. In a
// combat without a grid (theatre) there is no distance and nothing is too far: the
// master judges the range (RN-25).
func spellTargetList(terrain grid.Terrain, cs []playdb.Combatant, caster playdb.Combatant, v combatViewer, sp link.Spell, theatre bool) []*playv1.TargetInReach {
	reach, limited := reachOf(sp)
	pool := coverPool(cs, v)
	var out []*playv1.TargetInReach
	for _, t := range cs {
		if t.Defeated || !v.sees(t) || (selfOnly(sp) && t.ID != caster.ID) {
			continue
		}
		cover := coverAgainst(terrain, caster, t, pool)
		target := withCover(&playv1.TargetInReach{CombatantId: t.ID, Label: t.Label, State: stateOf(t)}, cover)
		target.Untargetable = cover.total() && !v.master && !sp.Area && t.ID != caster.ID
		if theatre {
			out = append(out, target)
			continue
		}
		if dist, ok := distanceFt(caster, t); ok {
			target.DistanceFt = &dist
			target.TooFar = limited && t.ID != caster.ID && dist > reach
		} else if t.ID == caster.ID {
			dist := int32(0)
			target.DistanceFt = &dist
		} else {
			target.TooFar = limited && !v.master // a player cannot cast at what has no square (CastSpell)
		}
		out = append(out, target)
	}
	return out
}

// disableAll disables every option with the reason: nothing can be done off
// turn, outside a running combat or by a combatant that is out.
func disableAll(o *rulesv1.TurnOptions, code rulesv1.DisabledReasonCode) {
	reason := func() *rulesv1.DisabledReason { return &rulesv1.DisabledReason{Code: code} }
	for _, a := range o.GetAttacks() {
		a.Enabled, a.Reason = false, reason()
	}
	for _, sp := range o.GetSpells() {
		sp.Enabled, sp.Reason = false, reason()
	}
	for _, a := range slices.Concat(o.GetStandardActions(), o.GetFeatureActions()) {
		a.Enabled, a.Reason = false, reason()
	}
}

// targetsFor lists who the combatant's attack of the given reach can target,
// as the viewer sees them: not itself, not a defeated one, not a hidden one
// for a player (RN-10), in turn order, each with the distance and whether it
// is too far. In a combat without a grid (theatre) there is no distance and
// nothing is too far: every combatant the rules allow is a target, and the master
// judges the reach (RN-25).
func targetsFor(terrain grid.Terrain, cs []playdb.Combatant, attacker playdb.Combatant, v combatViewer, reach int32, theatre bool) []*playv1.TargetInReach {
	pool := coverPool(cs, v)
	var out []*playv1.TargetInReach
	for _, t := range cs {
		if t.ID == attacker.ID || t.Defeated || !v.sees(t) {
			continue
		}
		cover := coverAgainst(terrain, attacker, t, pool)
		target := withCover(&playv1.TargetInReach{CombatantId: t.ID, Label: t.Label, State: stateOf(t)}, cover)
		target.Untargetable = cover.total() && !v.master
		if theatre {
			out = append(out, target)
			continue
		}
		if dist, ok := distanceFt(attacker, t); ok {
			target.DistanceFt = &dist
			target.TooFar = dist > reach
		} else {
			target.TooFar = !v.master // a player cannot attack what has no square (RollAttack)
		}
		out = append(out, target)
	}
	return out
}

// withCover puts the cover a target has against the attacker on its list entry.
func withCover(t *playv1.TargetInReach, cv coverView) *playv1.TargetInReach {
	t.Cover, t.CoverSource = coverDegreeProto(cv.key()), cv.source
	return t
}

// pendingVisible says whether the viewer may get a pending damage: a player
// never learns anything about a target they cannot see (RN-10, RN-20), for
// instance an NPC the master hid after the hit.
func pendingVisible(p playdb.PendingDamage, cs []playdb.Combatant, v combatViewer) bool {
	i := slices.IndexFunc(cs, func(c playdb.Combatant) bool { return c.ID == p.TargetID })
	return i >= 0 && v.sees(cs[i])
}

// pendingProto builds the PendingDamage that the master and the attacker's
// player get. cs are the combat's combatants, for the target's defeat.
func pendingProto(p playdb.PendingDamage, cs []playdb.Combatant) *playv1.PendingDamage {
	out := &playv1.PendingDamage{
		Id: p.ID, AttackerId: deref(p.AttackerID), TargetId: p.TargetID, AttackKey: p.AttackKey,
		Status: pendingStatusToProto[p.Status], Critical: p.Critical,
		CriticalRule: pendingCriticalRule(p.Critical, p.CriticalMaxRule), CriticalMax: p.CriticalMax,
		DiceCount: p.DiceCount, DiceSides: p.DiceSides, Bonus: p.DiceBonus,
		DamageTypeKey: p.DamageType, DamageTypePt: damageTypePT[p.DamageType],
		CastId: deref(p.CastID), Healing: p.Healing, Half: p.Half, AppliedAmount: p.AppliedAmount,
		TrapPointId: deref(p.TrapPointID), // a trap's damage has no attacker (MR-035)
	}
	if p.Amount != nil {
		out.Amount = *p.Amount
		total := *p.Amount
		if p.RollTotal != nil { // a half damage's roll is the whole one
			total = *p.RollTotal
		}
		out.Roll = diceRoll(p.DiceCount, p.DiceSides, p.Faces, p.DiceBonus+p.CriticalMax, total, p.Physical)
	}
	if p.Status == pendingApplied {
		out.TargetDefeated = slices.ContainsFunc(cs, func(c playdb.Combatant) bool { return c.ID == p.TargetID && holdsHP(c) && c.Defeated })
	}
	return out
}

var pendingStatusToProto = map[string]playv1.PendingDamageStatus{
	pendingAwaitingReaction: playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_AWAITING_REACTION,
	pendingAwaitingRoll:     playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_AWAITING_ROLL,
	pendingRolled:           playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_ROLLED,
	pendingApplied:          playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_APPLIED,
	pendingDiscarded:        playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_DISCARDED,
}

var outcomeToProto = map[string]playv1.AttackOutcome{
	outcomeHit:  playv1.AttackOutcome_ATTACK_OUTCOME_HIT,
	outcomeCrit: playv1.AttackOutcome_ATTACK_OUTCOME_CRITICAL_HIT,
	outcomeMiss: playv1.AttackOutcome_ATTACK_OUTCOME_MISS,
}

// rollInput is how the caller says a roll comes: rolled by the app, or a
// number typed from physical dice.
type rollInput struct {
	inApp bool
	typed int
	// pool says typed is the sum of the physical dice of a spell's pool (not a
	// d20 face): CastSpell's pool_sum.
	pool bool
}

// mustRollThisWay checks a player's way of rolling against the campaign's dice
// setting (RN-18): only a mode the master forced binds them (a typed value is
// refused when everybody rolls in the app, the app's roll when everybody rolls
// real dice); with "each player chooses" they pick on every roll, and their
// preference is only the default the screen offers. The master rolls either
// way, for anyone. Inside a change it reads the setting in the change's
// transaction (tx).
func (s *Service) mustRollThisWay(ctx context.Context, tx pgx.Tx, m authz.Membership, in rollInput) error {
	if m.Role == authz.RoleMaster {
		return nil
	}
	force, err := s.dice.ForcedDice(ctx, tx, m.CampaignID, m.UserID)
	if err != nil {
		return err
	}
	if force.refuses(in.inApp) {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_WRONG_DICE_MODE, "this is not how the campaign has you roll your dice")
	}
	return nil
}

// mustReactNow checks, inside a write, that the combatant may spend its
// reaction now: the combat is running, it is not out of the fight or down, and
// it is not its own turn (then an attack spends the action).
func (s *Service) mustReactNow(ctx context.Context, c *combatTx, who playdb.Combatant, onOffer bool) error {
	if err := notEnded(c.enc); err != nil {
		return err
	}
	switch {
	case c.enc.Status != statusActive:
		return gateError(rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_COMBAT_NOT_ACTIVE)
	case who.Defeated:
		return gateError(rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_COMBATANT_DEFEATED)
	case actsNow(c.enc, who) && !onOffer: // an opportunity attack may come in a joint turn, on any turn (SRD)
		return connect.NewError(connect.CodeInvalidArgument, errors.New("on its own turn an attack spends the action, not the reaction"))
	}
	down, err := s.isDown(ctx, c.tx, c.session.CampaignID, who)
	if err != nil {
		return err
	}
	if down {
		return gateError(rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_COMBATANT_DOWN)
	}
	return nil
}

// RollAttack implements playv1connect.CombatServiceHandler.
func (s *Service) RollAttack(
	ctx context.Context,
	req *connect.Request[playv1.RollAttackRequest],
) (*connect.Response[playv1.RollAttackResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	key, err := parseKey(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	encID, err := parseCombatID(req.Msg.GetEncounterId(), "encounter")
	if err != nil {
		return nil, err
	}
	attackerID, err := parseCombatID(req.Msg.GetAttackerId(), "combatant")
	if err != nil {
		return nil, err
	}
	targetID, err := parseCombatID(req.Msg.GetTargetId(), "combatant")
	if err != nil {
		return nil, err
	}
	attackKey := req.Msg.GetAttackKey()
	if attackKey == "" || len(attackKey) > 100 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("attack_key must name one of the attacker's attacks"))
	}
	var in rollInput
	switch roll := req.Msg.GetRoll().(type) {
	case *playv1.RollAttackRequest_RollInApp:
		if !roll.RollInApp {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("roll_in_app must be true"))
		}
		in.inApp = true
	case *playv1.RollAttackRequest_D20Face:
		in.typed = int(roll.D20Face)
		if in.typed < 1 || in.typed > 20 {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("d20_face must be 1 to 20"))
		}
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("set roll_in_app or d20_face"))
	}
	offerID := ""
	if raw := req.Msg.GetOpportunityOfferId(); raw != "" {
		if offerID, err = parseOfferID(raw); err != nil {
			return nil, err
		}
	}
	// An opportunity attack is a reaction attack (MR-034).
	asReaction := req.Msg.GetAsReaction() || offerID != ""
	v := viewerOf(m)

	var made actionEvent
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventAttackRolled, encounterID: encID}, func(c *combatTx) (any, error) {
		cs, err := c.q.ListCombatants(ctx, c.enc.ID)
		if err != nil {
			return nil, fmt.Errorf("list the combatants: %w", err)
		}
		v = c.viewer(m, cs) // the fog: an NPC the player does not see is not found
		attacker, err := findCombatant(cs, attackerID, v)
		if err != nil {
			return nil, err
		}
		if err := v.mayAct(attacker); err != nil {
			return nil, err
		}
		target, err := findCombatant(cs, targetID, v)
		if err != nil && offerID != "" && connect.CodeOf(err) == connect.CodeNotFound {
			// An opportunity attack is made at the mover on the square it left: a mover
			// the player saw leave is theirs to hit even if it is out of their sight now
			// (an NPC that retreated into the dark).
			if o, oerr := pendingOffer(ctx, c, offerID); oerr == nil && o.ReactorID == attacker.ID && o.MoverID == targetID {
				if i := slices.IndexFunc(cs, func(x playdb.Combatant) bool { return x.ID == targetID }); i >= 0 && v.seesAtOffer(cs[i], o) {
					target, err = cs[i], nil
				}
			}
		}
		if err != nil {
			return nil, err
		}
		if target.ID == attacker.ID {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("an attacker cannot be its own target"))
		}
		// An opportunity attack answers an offer: the offer is the reactor's, on
		// the mover, and still waits. Gone or answered is aborted (the screen reads
		// the combat again); the wrong person was told apart above (mayAct).
		var offer playdb.OpportunityOffer
		if offerID != "" {
			if offer, err = pendingOffer(ctx, c, offerID); err != nil {
				return nil, err
			}
			if offer.ReactorID != attacker.ID || offer.MoverID != target.ID {
				return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("an opportunity attack is made by the offer's reactor on its mover"))
			}
			if attacker.ReactionUsed {
				return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_REACTION_USED, "the reaction is used")
			}
		}
		if asReaction {
			err = s.mustReactNow(ctx, c, attacker, offerID != "")
		} else {
			err = s.mustActNow(ctx, c, attacker)
		}
		if err != nil {
			return nil, err
		}
		if target.Defeated {
			return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TARGET_DEFEATED, "the target is defeated")
		}
		if !v.master {
			if err := s.mustRollThisWay(ctx, c.tx, m, in); err != nil {
				return nil, err
			}
		}

		if err := mayAttack(attacker, asReaction); err != nil {
			return nil, err
		}
		attackerSheet, err := s.sheetOf(ctx, c.tx, m.CampaignID, attacker)
		if err != nil {
			return nil, err
		}
		i := slices.IndexFunc(attackerSheet.Attacks, func(a link.Attack) bool { return a.Key == attackKey })
		if i < 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("attack_key is not one of the attacker's attacks"))
		}
		attack := attackerSheet.Attacks[i]
		terrain, err := s.terrainOf(ctx, c.tx, m.CampaignID, c.enc)
		if err != nil {
			return nil, err
		}
		if attack.Save {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("this attack asks for a saving throw, not an attack roll: use CastSpell"))
		}
		// An opportunity attack is a melee attack (SRD): a melee weapon, thrown or
		// not (a dagger counts), not a bow or a spell. It reaches 5 ft.
		if asReaction && !attack.Melee {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("an opportunity attack is a melee attack"))
		}
		// RN-18, then the economy: a player has one action a turn (the Attack
		// action's attacks, with Extra Attack), or the reaction for an opportunity
		// attack. The master has the last word, and an NPC with several attacks is
		// his to run.
		bonusKind := combat.BonusNone
		if !v.master {
			switch {
			case asReaction && attacker.ReactionUsed:
				return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_REACTION_USED, "the reaction is used")
			case !asReaction:
				if bonusKind, err = attackEconomy(attacker, attackerSheet, attack); err != nil {
					return nil, err
				}
			}
			// RN-21: the reach is a player's limit on a map. Without one (RN-25) nobody
			// has a square and the master judges the reach, so nothing is checked.
			if !isTheatre(c.enc) {
				if !placed(attacker) || !placed(target) {
					return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_PLACED, "the attacker and the target must be on the map")
				}
				// An opportunity attack comes right before the mover leaves the reach, so
				// it asks no reach check (the mover has moved already).
				dist, _ := distanceFt(attacker, target)
				if reach := attackReach(attack, asReaction); offerID == "" && dist > reach {
					return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TARGET_OUT_OF_REACH, "the target is beyond the attack's range",
						func(b *playv1.EncounterBlocked) { b.MissingFt = dist - reach })
				}
			}
		}
		// D4: the cover the target has against this attacker. Total cover (a wall on
		// the line, or the master's mark) makes it untargetable for a player; the
		// master has the last word.
		// An opportunity attack comes right before the mover leaves the reach: the
		// cover is measured with it on the square it left the reach at, not where
		// it stands now (a door it walked through is not between the two).
		coverTarget := target
		if offerID != "" && offer.LeftCol != nil && offer.LeftRow != nil { // an offer made by hand has no square
			coverTarget.GridCol, coverTarget.GridRow = offer.LeftCol, offer.LeftRow
		}
		known, err := c.sight.knownTerrain(ctx, c.tx, v)
		if err != nil {
			return nil, err
		}
		// The real cover sets the armor class; the player is told, and refused, only by
		// what they know of the map and see of the creatures (RN-10, MR-036).
		cp, err := c.coverOf(ctx, v, terrain, known, attacker, coverTarget, cs)
		if err != nil {
			return nil, err
		}
		cover := cp.real
		if cp.shown.total() && !v.master {
			return nil, errCoverTotal()
		}

		// A cantrip with a verbal component cannot be cast inside a zone of silence (SRD, Silence).
		if attack.Spell && silencedAt(c.zones, attacker, isTheatre(c.enc)) {
			if sp, err := s.roster.CombatSpell(ctx, c.tx, m.CampaignID, attacker.CharacterID, attack.Key, 0, ""); err == nil && sp.Verbal {
				return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_SILENCED, "a spell with a verbal component cannot be cast inside a zone of silence")
			}
		}

		// The roll, and what it did against the target's armor class (with the
		// +5 of an active Escudo and the cover). The class stays on the server (RN-20).
		face, roll, err := s.d20(in, attack.ToHit)
		if err != nil {
			return nil, err
		}
		targetSheet, err := s.sheetOf(ctx, c.tx, m.CampaignID, target)
		if err != nil {
			return nil, err
		}
		targetAC := targetSheet.ArmorClass + int(target.AcBonus) + cover.bonus()
		// Improved Critical and Superior Critical are for weapon attacks only.
		criticalFrom := attackerSheet.CriticalRange
		if attack.Spell {
			criticalFrom = 0
		}
		result := combat.ResolveAttackFrom(attack.ToHit, targetAC, face, criticalFrom)

		after := attacker
		var run int32
		if !asReaction {
			if run, err = breakRun(ctx, c, attacker); err != nil {
				return nil, err
			}
		}
		made = actionEvent{
			RunBefore: run, Round: c.enc.Round, Secret: secretOf(attacker, target), Actor: attacker.ID, Target: target.ID, Key: attackKey,
			D20: clamp32(face, 1, 20), Modifier: clamp32(attack.ToHit, math.MinInt32, math.MaxInt32), Total: clamp32(result.Total, math.MinInt32, math.MaxInt32),
			Physical: roll.Physical, Outcome: outcomeMiss, AsReaction: asReaction, CriticalMaxRule: c.rules.CriticalMaxPlusRoll,
			ActionBefore: attacker.ActionUsed, BonusBefore: attacker.BonusActionUsed, ReactionBefore: attacker.ReactionUsed, AttacksBefore: attacker.AttacksMade,
			AsBonus: bonusKind != combat.BonusNone, AttackKeyBefore: deref(attacker.ActionAttackKey), FlurryBefore: attacker.BonusAttacksLeft,
			// The armor class the roll was compared with (an active Escudo's +5
			// included): the master's answer shows it, a player's never does.
			TargetAC: clamp32(targetAC, 0, math.MaxInt32),
			Cover:    cover.key(), CoverSource: cover.sourceKey(), CoverBonus: clamp32(cover.bonus(), 0, 5),
			CoverRestricted: cp.restricted, CoverSeenBy: cp.seenBy,
		}
		switch {
		case asReaction:
			after.ReactionUsed = true
		case bonusKind == combat.BonusFlurry:
			// The bonus action went to Flurry of Blows: one of its strikes.
			if err := c.q.SetCombatantAttackState(ctx, playdb.SetCombatantAttackStateParams{ID: attacker.ID, ActionAttackKey: attacker.ActionAttackKey, BonusAttacksLeft: attacker.BonusAttacksLeft - 1}); err != nil {
				return nil, fmt.Errorf("count the flurry strike: %w", err)
			}
		case bonusKind != combat.BonusNone:
			after.BonusActionUsed = true
		default:
			after.ActionUsed = true
			// A cantrip is no attack of the Attack action, but its beams are counted
			// (Eldritch Blast), so a cast makes no more than its beams.
			if !attack.Spell || attack.Beams > 1 {
				if err := c.q.SetCombatantAttacksMade(ctx, playdb.SetCombatantAttacksMadeParams{ID: attacker.ID, AttacksMade: attacker.AttacksMade + 1}); err != nil {
					return nil, fmt.Errorf("count the attack: %w", err)
				}
			}
			if err := c.q.SetCombatantAttackState(ctx, playdb.SetCombatantAttackStateParams{ID: attacker.ID, ActionAttackKey: &attack.Key, BonusAttacksLeft: attacker.BonusAttacksLeft}); err != nil {
				return nil, fmt.Errorf("remember the attack: %w", err)
			}
		}
		if err := c.q.SetCombatantEconomy(ctx, playdb.SetCombatantEconomyParams{
			ID: attacker.ID, ActionUsed: after.ActionUsed, BonusActionUsed: after.BonusActionUsed, ReactionUsed: after.ReactionUsed, Dashed: after.Dashed,
		}); err != nil {
			return nil, fmt.Errorf("spend the action: %w", err)
		}
		if result.Hit {
			made.Outcome = outcomeHit
			if result.Critical {
				made.Outcome = outcomeCrit
			}
			bonus := attack.DiceBonus
			if bonusKind == combat.BonusTwoWeapon {
				// The off-hand attack adds no ability modifier to its damage (unless
				// it is negative, or the character fights with two weapons).
				bonus = combat.OffHandBonus(bonus, attack.AbilityMod, attackerSheet.TwoWeaponFighting)
			}
			p, err := s.openHit(ctx, c, m.CampaignID, attacker, target, attackKey,
				link.Dice{Count: attack.DiceCount, Sides: attack.DiceSides, Bonus: bonus, DamageType: attack.DamageType}, result.Critical, result.Total, targetAC)
			if err != nil {
				return nil, err
			}
			made.Pending = p.ID
		}
		if offerID != "" {
			var damage *string // the damage the attack opened: the 0 hit points rule finds the offer by it
			if made.Pending != "" {
				damage = &made.Pending
			}
			if _, err := c.q.SetOpportunityOfferState(ctx, playdb.SetOpportunityOfferStateParams{ID: offer.ID, State: offerAttacked, AttackPendingID: damage, AnsweredAt: &c.now}); err != nil {
				return nil, fmt.Errorf("answer the opportunity offer: %w", err)
			}
			made.OfferID = offer.ID
			if offer.LeftCol != nil && offer.LeftRow != nil {
				made.Col, made.Row = *offer.LeftCol, *offer.LeftRow // where the mover was when it was attacked
			}
		}
		if c.enc, err = c.q.TouchEncounter(ctx, c.enc.ID); err != nil {
			return nil, fmt.Errorf("touch the encounter: %w", err)
		}
		c.characterID = &attacker.CharacterID
		return made, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "roll an attack", err)
	}
	if v, err = s.viewerAfter(ctx, m, res, v); err != nil { // a replay never ran the closure: the fog filter still holds
		return nil, s.dbError(ctx, "work out what the player sees", err)
	}
	ev, err := resultEvent(res, made)
	if err != nil {
		return nil, s.dbError(ctx, "read the attack", err)
	}
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, !ev.Secret)
	})
	if err != nil {
		return nil, err
	}
	pending, err := s.pendingFor(ctx, res, ev.Pending, v)
	if err != nil {
		return nil, err
	}
	roll := &playv1.AttackRoll{
		AttackerId: ev.Actor, TargetId: ev.Target, AttackKey: ev.Key,
		D20: diceRoll(1, 20, faceList(ev), ev.Modifier, ev.Total, ev.Physical), Outcome: outcomeToProto[ev.Outcome],
		CriticalRule: criticalRuleProto(tablerules.Rules{CriticalMaxPlusRoll: ev.CriticalMaxRule}),
	}
	coverKey, coverSource := ev.coverFor(v)
	roll.Cover, roll.CoverSource = coverDegreeProto(coverKey), coverSourceProto(coverSource)
	if v.master {
		roll.TargetArmorClass = new(ev.TargetAC) // "Acertou contra CA 18": never a player's
	}
	return connect.NewResponse(&playv1.RollAttackResponse{Encounter: out, PendingDamage: pending, Roll: roll}), nil
}

// faceList is the face of the d20 of an attack roll; none for a physical one,
// whose d20 was typed (the typed face is still in D20: the master and the
// player know it, so it is sent as the face too).
func faceList(ev actionEvent) []int32 { return []int32{ev.D20} }

// resultEvent is the event a write made: the one the closure made, or, for a
// retry the closure never ran for, the one stored the first time.
func resultEvent(res combatResult, made actionEvent) (actionEvent, error) {
	if !res.repeated {
		if res.stamped != nil {
			return *res.stamped, nil // with who could see it (combat_fog.go)
		}
		return made, nil
	}
	return readEvent(res.payload)
}

// pendingFor reads the pending damage an event is about, for the answer;
// nil when there is none (a miss) or an undo took it away meanwhile.
func (s *Service) pendingFor(ctx context.Context, res combatResult, id string, v combatViewer) (*playv1.PendingDamage, error) {
	if id == "" {
		return nil, nil
	}
	p, err := s.queries.GetPendingDamage(ctx, playdb.GetPendingDamageParams{EncounterID: res.encounterID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, s.dbError(ctx, "read the pending damage", err)
	}
	cs, err := s.queries.ListCombatants(ctx, res.encounterID)
	if err != nil {
		return nil, s.dbError(ctx, "list the combatants", err)
	}
	if !pendingVisible(p, cs, v) {
		return nil, nil
	}
	out := pendingProto(p, cs)
	if p.TrapPointID != nil && v.master && s.traps != nil { // a fired trap is public; its name is the master's card's
		if names, err := s.traps.TrapNames(ctx, res.session.CampaignID, []string{*p.TrapPointID}); err == nil {
			out.TrapName = names[*p.TrapPointID]
		}
	}
	return out, nil
}

// RollDamage implements playv1connect.CombatServiceHandler.
func (s *Service) RollDamage(
	ctx context.Context,
	req *connect.Request[playv1.RollDamageRequest],
) (*connect.Response[playv1.RollDamageResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	key, err := parseKey(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	encID, err := parseCombatID(req.Msg.GetEncounterId(), "encounter")
	if err != nil {
		return nil, err
	}
	pendingID, err := parseCombatID(req.Msg.GetPendingDamageId(), "pending damage")
	if err != nil {
		return nil, err
	}
	var in rollInput
	switch roll := req.Msg.GetRoll().(type) {
	case *playv1.RollDamageRequest_RollInApp:
		if !roll.RollInApp {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("roll_in_app must be true"))
		}
		in.inApp = true
	case *playv1.RollDamageRequest_TypedSum:
		in.typed = int(roll.TypedSum)
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("set roll_in_app or typed_sum"))
	}
	v := viewerOf(m)

	var made actionEvent
	var vitals []*playv1.CharacterVitals // the characters a heal or a death save touched
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventDamageRolled, encounterID: encID}, func(c *combatTx) (any, error) {
		vitals = nil
		if err := notEnded(c.enc); err != nil {
			return nil, err
		}
		p, err := c.q.GetPendingDamage(ctx, playdb.GetPendingDamageParams{EncounterID: c.enc.ID, ID: pendingID})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("pending damage not found"))
		}
		if err != nil {
			return nil, fmt.Errorf("find the pending damage: %w", err)
		}
		cs, err := c.q.ListCombatants(ctx, c.enc.ID)
		if err != nil {
			return nil, fmt.Errorf("list the combatants: %w", err)
		}
		v = c.viewer(m, cs) // the fog: an NPC the player does not see is not found
		attacker, err := findCombatant(cs, deref(p.AttackerID), v)
		if err != nil {
			return nil, err
		}
		if err := v.mayAct(attacker); err != nil {
			return nil, err
		}
		target, _ := findCombatant(cs, p.TargetID, combatViewer{master: true})
		if !v.sees(target) {
			return nil, errCombatantNotFound() // a target hidden after the hit: the master resolves it
		}
		switch p.Status {
		case pendingAwaitingReaction:
			return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_REACTION_PENDING, "the hit waits for the target's reaction")
		case pendingRolled:
			return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_DAMAGE_ALREADY_ROLLED, "the damage was rolled already")
		case pendingApplied, pendingDiscarded:
			return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_DAMAGE_RESOLVED, "the damage was applied or discarded already")
		}

		// An area spell's damage is one roll for the whole cast: the pending damages
		// of the cast still to be rolled are settled by this roll.
		group := []playdb.PendingDamage{p}
		if p.CastID != nil {
			all, err := c.q.ListCastPendingDamages(ctx, playdb.ListCastPendingDamagesParams{EncounterID: c.enc.ID, CastID: p.CastID})
			if err != nil {
				return nil, fmt.Errorf("list the cast's pending damage: %w", err)
			}
			// A spell with two damage types opens one roll for each: only the ones of
			// this damage type settle together.
			group = slices.DeleteFunc(all, func(o playdb.PendingDamage) bool {
				return o.Status != pendingAwaitingRoll || o.DamageType != p.DamageType || o.Healing != p.Healing
			})
		}

		// The damage: the dice (doubled on a critical hit under the SRD's rule, as the
		// pending damage kept them), plus the modifier once, plus what the table's
		// critical rule adds without rolling (the dice's maximum, in "máximo mais uma
		// rolagem"; typed_sum never includes it). A flat damage needs no dice (and so
		// no way of rolling).
		expr := dice.Expr{Count: int(p.DiceCount), Sides: int(p.DiceSides), Modifier: int(p.DiceBonus) + int(p.CriticalMax)}
		var roll dice.Result
		if p.DiceCount == 0 {
			roll = dice.Result{Expr: expr, Modifier: expr.Modifier, Total: expr.Modifier}
		} else {
			if !v.master {
				if err := s.mustRollThisWay(ctx, c.tx, m, in); err != nil {
					return nil, err
				}
			}
			if in.inApp {
				if roll, err = dice.Roll(s.roller, expr); err != nil {
					return nil, fmt.Errorf("roll the damage: %w", err)
				}
			} else if roll, err = dice.Physical(expr, in.typed); err != nil {
				return nil, connect.NewError(connect.CodeInvalidArgument,
					fmt.Errorf("typed_sum must be %d to %d", expr.Count, expr.Count*expr.Sides))
			}
		}
		total := max(roll.Total, 0) // damage is never below 0
		faces := faces32(roll.Faces)
		rolledTotal := clamp32(total, 0, math.MaxInt32)

		made = actionEvent{
			Round: c.enc.Round, Secret: secretOf(attacker, target), Actor: attacker.ID, Target: p.TargetID, Pending: p.ID, Key: p.AttackKey,
			DiceCount: p.DiceCount, DiceSides: p.DiceSides, Modifier: clamp32(int(p.DiceBonus)+int(p.CriticalMax), math.MinInt32, math.MaxInt32), CriticalMax: p.CriticalMax, CriticalMaxRule: p.CriticalMaxRule, Faces: faces,
			DamageType: p.DamageType, Critical: p.Critical, Physical: roll.Physical, Heal: p.Healing, Total: rolledTotal,
		}
		for _, g := range group {
			tgt, _ := findCombatant(cs, g.TargetID, combatViewer{master: true})
			// A hidden creature in the one roll of a cast that settles several makes the
			// roll the master's alone, unless the cast lists its targets for each viewer:
			// an area's line leaves out the hidden ones itself (spellView).
			made.Secret = made.Secret || (tgt.Hidden && p.CastID == nil)
			amount := clamp32(total, 0, math.MaxInt32)
			if g.Half {
				amount = clamp32(combat.HalfDamage(total), 0, math.MaxInt32)
			}
			if !g.Healing {
				if amount, err = s.afterResistance(ctx, c, tgt, g.DamageType, amount); err != nil {
					return nil, err
				}
			}
			hit, vit, err := s.landDamage(ctx, c, g, tgt, amount)
			if err != nil {
				return nil, err
			}
			if vit != nil {
				vitals = append(vitals, vit)
			}
			status := pendingRolled // a player's character waits for the master (RN-02)
			if hit.Applied {
				status = pendingApplied
			}
			if _, err := c.q.SetPendingDamageRolled(ctx, playdb.SetPendingDamageRolledParams{
				ID: g.ID, Status: status, Faces: faces, Physical: roll.Physical, Amount: &amount, ResolvedAt: resolvedAt(status, c),
				RollTotal: &rolledTotal,
			}); err != nil {
				return nil, fmt.Errorf("save the damage roll: %w", err)
			}
			if p.CastID != nil && hit.Applied && !g.Healing {
				// A spell asks one concentration save of a target, not one per damage type.
				if hit.ConcentrationDC, err = s.castConcentrationDC(ctx, c, g, tgt, takenBy(hit.Amount, hit.Before, hit.After)); err != nil {
					return nil, err
				}
			}
			if g.ID == p.ID {
				made.Amount, made.Applied, made.Before, made.After, made.ConcentrationDC = hit.Amount, hit.Applied, hit.Before, hit.After, hit.ConcentrationDC
				made.DeathBefore = hit.DeathBefore
				// The 0 hit points rule: an opportunity attack that drops its mover to 0
				// takes it back to where it left the reach.
				if hit.Applied && !g.Healing && hit.Before != nil && hit.After != nil && hit.Before.HP > 0 && hit.After.HP == 0 {
					if made.ReturnedFrom, made.ReturnBlocked, err = s.returnToReach(ctx, c, g.ID, tgt); err != nil {
						return nil, err
					}
				}
			}
			if p.CastID != nil || p.Healing {
				made.Settled = append(made.Settled, hit)
			}
		}
		if c.enc, err = c.q.TouchEncounter(ctx, c.enc.ID); err != nil {
			return nil, fmt.Errorf("touch the encounter: %w", err)
		}
		c.characterID = &attacker.CharacterID
		return made, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "roll damage", err)
	}
	if v, err = s.viewerAfter(ctx, m, res, v); err != nil { // a replay never ran the closure: the fog filter still holds
		return nil, s.dbError(ctx, "work out what the player sees", err)
	}
	ev, err := resultEvent(res, made)
	if err != nil {
		return nil, s.dbError(ctx, "read the damage", err)
	}
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
		s.publishReturned(ctx, m.CampaignID, d, ev)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, !ev.Secret)
		for _, vit := range vitals {
			s.publishVitals(m.CampaignID, vit)
		}
	})
	if err != nil {
		return nil, err
	}
	resp := &playv1.RollDamageResponse{Encounter: out}
	if resp.PendingDamage, err = s.pendingFor(ctx, res, ev.Pending, v); err != nil {
		return nil, err
	}
	decorate(resp.PendingDamage, ev, v, s.membersOf(ctx, res))
	for _, h := range ev.Settled {
		if h.Pending == ev.Pending {
			continue
		}
		other, err := s.pendingFor(ctx, res, h.Pending, v)
		if err != nil {
			return nil, err
		}
		if other != nil {
			decorate(other, ev, v, s.membersOf(ctx, res))
			resp.CastPendingDamages = append(resp.CastPendingDamages, other)
		}
	}
	return connect.NewResponse(resp), nil
}

// afterResistance is the damage that lands on the target once its stat block's
// resistances, vulnerabilities and immunities to the damage type are applied, after
// the bonuses and the half of a saving throw (SRD 5.1). Only a target that takes
// the damage at once and has a stat block is adjusted: an NPC made from a creature
// and a creature. A player's character keeps its Rage and other resistances as
// notes, for the master to apply when he sets the amount (RN-02).
func (s *Service) afterResistance(ctx context.Context, c *combatTx, target playdb.Combatant, damageType string, amount int32) (int32, error) {
	if !holdsHP(target) || damageType == "" || amount <= 0 {
		return amount, nil
	}
	// A creature entirely inside a zone of silence is immune to thunder damage (SRD, Silence).
	if damageType == "damage-type:thunder" && thunderImmune(c.zones, target, isTheatre(c.enc)) {
		return 0, nil
	}
	mods, err := s.roster.DamageModifiers(ctx, c.tx, c.session.CampaignID, target.CharacterID, deref(target.MonsterKey))
	if err != nil {
		return 0, err
	}
	return clamp32(combat.AdjustForType(int(amount), damageType, mods), 0, math.MaxInt32), nil
}

// landDamage puts a rolled damage or heal on its target, as far as the target's
// kind lets it land at once. An NPC takes a damage at once: temporary hit points
// first, then the hit points, defeated at 0 (and a heal puts it back above 0).
// A heal lands at once on a player's character too, up to its maximum, through
// the vitals (a character healed from 0 gets up and its death saves reset). A
// damage on a player's character waits for the master: nothing lands. The hit
// says what changed, with what an undo needs.
func (s *Service) landDamage(ctx context.Context, c *combatTx, p playdb.PendingDamage, target playdb.Combatant, amount int32) (damageHit, *playv1.CharacterVitals, error) {
	hit := damageHit{Pending: p.ID, Target: target.ID, Amount: amount, Half: p.Half}
	switch {
	case p.Healing:
		healed, vit, err := s.healCombatant(ctx, c, target, amount)
		healed.Pending, healed.Half = p.ID, p.Half
		return healed, vit, err
	case holdsHP(target): // an NPC and a creature take a damage at once
		dmg := combat.ApplyDamage(int(num(target.HpCurrent)), int(num(target.HpTemp)), int(amount))
		after := hpState{HP: clamp32(dmg.HP, 0, math.MaxInt32), Temp: clamp32(dmg.TempHP, 0, math.MaxInt32), Defeated: dmg.HP == 0}
		if err := c.q.SetCombatantHitPoints(ctx, playdb.SetCombatantHitPointsParams{ID: target.ID, HpCurrent: &after.HP, HpTemp: &after.Temp, Defeated: after.Defeated}); err != nil {
			return hit, nil, fmt.Errorf("apply the damage: %w", err)
		}
		before := hpOf(target)
		hit.Before, hit.After, hit.Applied = &before, &after, true
		hit.ConcentrationDC = concentrationDC(target, amount-clamp32(dmg.Absorbed, 0, math.MaxInt32))
	}
	return hit, nil, nil
}

// healCombatant heals a combatant: an NPC's hit points, up to its maximum (it
// comes back above 0), or a player's character's through the vitals, up to its
// maximum (healed from 0 it gets up and its death saves reset, RN-03). The hit
// says how much it regained, with what an undo needs; the vitals are the
// character's after.
func (s *Service) healCombatant(ctx context.Context, c *combatTx, target playdb.Combatant, amount int32) (damageHit, *playv1.CharacterVitals, error) {
	hit := damageHit{Target: target.ID, Applied: true}
	if holdsHP(target) {
		before := hpOf(target)
		r := combat.ApplyHeal(int(before.HP), int(num(target.HpMax)), int(amount))
		after := hpState{HP: clamp32(r.HP, 0, math.MaxInt32), Temp: before.Temp, Defeated: r.HP == 0}
		if err := c.q.SetCombatantHitPoints(ctx, playdb.SetCombatantHitPointsParams{ID: target.ID, HpCurrent: &after.HP, HpTemp: &after.Temp, Defeated: after.Defeated}); err != nil {
			return hit, nil, fmt.Errorf("apply the heal: %w", err)
		}
		hit.Before, hit.After, hit.Amount = &before, &after, clamp32(r.Healed, 0, math.MaxInt32)
		return hit, nil, nil
	}
	now, err := s.vitals.GetVitalsTx(ctx, c.tx, c.session.CampaignID, target.CharacterID)
	if err != nil {
		return hit, nil, err
	}
	if w := now.GetWildShape(); w != nil {
		// In a beast form the healing is the beast's (MR-037).
		r := combat.ApplyHeal(int(w.GetHitPointsCurrent()), int(w.GetHitPointsMax()), int(amount))
		hp := clamp32(r.HP, 0, math.MaxInt32)
		before, after, err := s.vitalsOf(ctx, c, target.CharacterID, &playv1.AdjustCharacterVitalsRequest{WildShapeHitPointsCurrent: &hp})
		if err != nil {
			return hit, nil, err
		}
		hit.DeathBefore = deathOf(target)
		hit.Before, hit.After = new(hpStateOf(before)), new(hpStateOf(after))
		hit.Amount = clamp32(r.Healed, 0, math.MaxInt32)
		return hit, after, nil
	}
	r := combat.ApplyHeal(int(now.GetHitPointsCurrent()), int(now.GetHitPointsMax()), int(amount))
	hp := clamp32(r.HP, 0, math.MaxInt32)
	before, after, err := s.vitalsOf(ctx, c, target.CharacterID, &playv1.AdjustCharacterVitalsRequest{HitPointsCurrent: &hp})
	if err != nil {
		return hit, nil, err
	}
	hit.DeathBefore = deathOf(target)
	hit.Before = new(hpStateOf(before))
	hit.After = new(hpStateOf(after))
	hit.Amount = clamp32(r.Healed, 0, math.MaxInt32)
	return hit, after, nil
}

// concentrationDC is the Constitution save DC to keep concentrating after a
// combatant took damage (RN-22): 0 when it does not concentrate or took none.
// The damage it takes is what temporary hit points did not absorb, even when it
// drops the combatant below 0.
func concentrationDC(target playdb.Combatant, taken int32) int32 {
	if target.ConcentrationSpell == nil || taken <= 0 {
		return 0
	}
	return clamp32(combat.ConcentrationDC(int(taken)), 0, math.MaxInt32)
}

// takenBy is what a damage of amount cost a combatant: what temporary hit points
// did not absorb, even when it drops the combatant below 0.
func takenBy(amount int32, before, after *hpState) int32 {
	if before == nil || after == nil {
		return max(amount, 0)
	}
	return max(amount-max(before.Temp-after.Temp, 0), 0)
}

// castConcentrationDC notes what a damage of a spell cost its target (taken) and
// gives the Constitution save DC the target owes for the whole cast (RN-22): the
// SRD asks one save for each source of damage, and a spell is one source however
// many damage types it deals. It is 0 while another pending damage of the cast is
// still to be settled for the target, and when the target does not concentrate;
// the pending damage that settles last carries the DC of the sum of what the
// cast's damages cost the target.
func (s *Service) castConcentrationDC(ctx context.Context, c *combatTx, p playdb.PendingDamage, target playdb.Combatant, taken int32) (int32, error) {
	if err := c.q.SetPendingDamageTaken(ctx, playdb.SetPendingDamageTakenParams{ID: p.ID, Taken: &taken}); err != nil {
		return 0, fmt.Errorf("note what the damage cost: %w", err)
	}
	all, err := c.q.ListCastPendingDamages(ctx, playdb.ListCastPendingDamagesParams{EncounterID: c.enc.ID, CastID: p.CastID})
	if err != nil {
		return 0, fmt.Errorf("list the cast's pending damage: %w", err)
	}
	var total int32
	for _, o := range all {
		if o.TargetID != p.TargetID || o.Healing {
			continue
		}
		switch o.Status {
		case pendingApplied:
			total = clamp32(int(total)+int(num(o.Taken)), 0, math.MaxInt32)
		case pendingDiscarded:
		default:
			return 0, nil // another damage type of the cast has still to land on it
		}
	}
	return concentrationDC(target, total), nil
}

// decorate adds to a pending damage what its event knows and the row does not:
// the concentration DC (for the master and the target's own player) and the
// death save failures a damage at 0 caused.
func decorate(p *playv1.PendingDamage, ev actionEvent, v combatViewer, owner func(targetID string) (playdb.Combatant, bool)) {
	if p == nil {
		return
	}
	var dc int32
	if p.GetId() == ev.Pending {
		dc = ev.ConcentrationDC
		p.DeathFailuresAdded = ev.FailuresAdded
		if t, ok := owner(p.GetTargetId()); ok && v.hiddenFrom(ev.DeathHidden, t) { // RN-24: the owner's and the master's
			p.DeathFailuresAdded = 0
		}
	}
	for _, h := range ev.Settled {
		if h.Pending == p.GetId() {
			dc = h.ConcentrationDC
		}
	}
	if t, ok := owner(p.GetTargetId()); ok && dc > 0 && (v.master || v.owns(t)) {
		p.ConcentrationDc = &dc
	}
}

// membersOf returns a lookup of the combat's combatants, for decorate.
func (s *Service) membersOf(ctx context.Context, res combatResult) func(string) (playdb.Combatant, bool) {
	cs, _ := s.queries.ListCombatants(ctx, res.encounterID)
	return func(id string) (playdb.Combatant, bool) {
		i := slices.IndexFunc(cs, func(c playdb.Combatant) bool { return c.ID == id })
		if i < 0 {
			return playdb.Combatant{}, false
		}
		return cs[i], true
	}
}

// resolvedAt is when a pending damage was settled: now for an applied one, not
// yet for one that waits for the master.
func resolvedAt(status string, c *combatTx) *time.Time {
	if status == pendingApplied {
		return &c.now
	}
	return nil
}

// ApplyPendingDamage implements playv1connect.CombatServiceHandler.
func (s *Service) ApplyPendingDamage(
	ctx context.Context,
	req *connect.Request[playv1.ApplyPendingDamageRequest],
) (*connect.Response[playv1.ApplyPendingDamageResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	key, err := parseKey(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	encID, err := parseCombatID(req.Msg.GetEncounterId(), "encounter")
	if err != nil {
		return nil, err
	}
	pendingID, err := parseCombatID(req.Msg.GetPendingDamageId(), "pending damage")
	if err != nil {
		return nil, err
	}
	override := req.Msg.Amount // the master's last word, if he sets one
	if override != nil && (*override < 0 || *override > maxHitPointChange) {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("amount must be 0 to %d", maxHitPointChange))
	}

	var made actionEvent
	var vitals *playv1.CharacterVitals // the target's, after
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventDamageApplied, encounterID: encID}, func(c *combatTx) (any, error) {
		vitals = nil
		if err := notEnded(c.enc); err != nil {
			return nil, err
		}
		p, attacker, target, err := s.settling(ctx, c, pendingID)
		if err != nil {
			return nil, err
		}
		switch p.Status {
		case pendingAwaitingReaction:
			return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_REACTION_PENDING, "the hit waits for the target's reaction")
		case pendingAwaitingRoll:
			return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_DAMAGE_NOT_ROLLED, "the damage was not rolled yet")
		case pendingApplied, pendingDiscarded:
			return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_DAMAGE_RESOLVED, "the damage was applied or discarded already")
		}
		if target.Kind != kindPlayer {
			return nil, fmt.Errorf("pending damage %s is rolled for an NPC", p.ID) // an NPC takes it when rolled
		}

		rolled := num(p.Amount)
		amount := rolled
		var appliedAmount *int32
		if override != nil && *override != rolled {
			amount, appliedAmount = *override, override
		}
		made = actionEvent{
			Round: c.enc.Round, Secret: secretOf(attacker, target), Actor: attacker.ID, Target: target.ID, Pending: p.ID, Key: p.AttackKey,
			Amount: amount, DamageType: p.DamageType, Overridden: appliedAmount != nil, Rolled: rolled,
		}
		now, err := s.vitals.GetVitalsTx(ctx, c.tx, c.session.CampaignID, target.CharacterID)
		if err != nil {
			return nil, err
		}
		down := isDownIn(now)
		switch {
		case down:
			// RN-03: a damage at 0 hit points is a death save failure (two for a
			// critical hit), and the hit points stay at 0.
			before, after, added, err := s.failuresWhileDown(ctx, c, target, p.Critical, amount)
			if err != nil {
				return nil, err
			}
			made.DeathBefore, made.Death, made.FailuresAdded, made.DeathHidden = before, after, added, c.rules.DeathSavesHidden
			made.Before = new(hpStateOf(now))
			made.After = made.Before
		case now.GetWildShape() != nil:
			// A druid in a beast form: the beast takes it, and what is left over when
			// the beast falls goes to the druid (MR-037, SRD).
			before, after, err := s.damageBeast(ctx, c, target, now, amount, &made)
			if err != nil {
				return nil, err
			}
			vitals = after
			made.Before, made.After = new(hpStateOf(before)), new(hpStateOf(after))
			made.ConcentrationDC = concentrationDC(target, amount)
			made.DeathBefore = deathOf(target)
			// The 0 hit points rule, as below: the damage that carried over dropped
			// the druid to 0 on an opportunity attack.
			if made.Before.HP > 0 && made.After.HP == 0 {
				if made.ReturnedFrom, made.ReturnBlocked, err = s.returnToReach(ctx, c, p.ID, target); err != nil {
					return nil, err
				}
			}
		default:
			// RN-02: the damage goes through the character's vitals, temporary hit
			// points first, never below 0. At 0 it is down ("Caído") and makes death
			// saves.
			dmg := combat.ApplyDamage(int(now.GetHitPointsCurrent()), int(now.GetHitPointsTemporary()), int(amount))
			hp, temp := clamp32(dmg.HP, 0, math.MaxInt32), clamp32(dmg.TempHP, 0, math.MaxInt32)
			before, after, err := s.vitalsOf(ctx, c, target.CharacterID, &playv1.AdjustCharacterVitalsRequest{HitPointsCurrent: &hp, HitPointsTemporary: &temp})
			if err != nil {
				return nil, err
			}
			vitals = after
			made.Before = new(hpStateOf(before))
			made.After = new(hpStateOf(after))
			made.ConcentrationDC = concentrationDC(target, amount-clamp32(dmg.Absorbed, 0, math.MaxInt32))
			made.DeathBefore = deathOf(target) // the undo puts the turn's save back too
			// The 0 hit points rule: an opportunity attack that drops its mover to 0
			// takes it back to where it left the reach.
			if made.Before.HP > 0 && made.After.HP == 0 {
				if made.ReturnedFrom, made.ReturnBlocked, err = s.returnToReach(ctx, c, p.ID, target); err != nil {
					return nil, err
				}
			}
		}
		if _, err := c.q.SetPendingDamageApplied(ctx, playdb.SetPendingDamageAppliedParams{ID: p.ID, ResolvedAt: &c.now, AppliedAmount: appliedAmount}); err != nil {
			return nil, fmt.Errorf("apply the pending damage: %w", err)
		}
		if p.CastID != nil {
			// A spell asks one concentration save of a target, not one per damage type.
			var taken int32
			if !down {
				taken = takenBy(amount, made.Before, made.After)
			}
			if made.ConcentrationDC, err = s.castConcentrationDC(ctx, c, p, target, taken); err != nil {
				return nil, err
			}
		}
		if c.enc, err = c.q.TouchEncounter(ctx, c.enc.ID); err != nil {
			return nil, fmt.Errorf("touch the encounter: %w", err)
		}
		c.characterID = &target.CharacterID
		return made, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "apply a pending damage", err)
	}
	ev, err := resultEvent(res, made)
	if err != nil {
		return nil, s.dbError(ctx, "read the applied damage", err)
	}
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
		s.publishReturned(ctx, m.CampaignID, d, ev)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, !ev.Secret)
		s.publishVitals(m.CampaignID, vitals)
	})
	if err != nil {
		return nil, err
	}
	pending, err := s.pendingFor(ctx, res, ev.Pending, combatViewer{master: true})
	if err != nil {
		return nil, err
	}
	decorate(pending, ev, combatViewer{master: true}, s.membersOf(ctx, res))
	return connect.NewResponse(&playv1.ApplyPendingDamageResponse{Encounter: out, PendingDamage: pending}), nil
}

// publishVitals tells the master and the character's player the vitals
// changed; nothing when the change did not touch any.
func (s *Service) publishVitals(campaignID string, v *playv1.CharacterVitals) {
	if v == nil {
		return
	}
	s.hub.Publish(campaignID, live.Event{
		Audience: vitalsAudience(v),
		Message: &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_VitalsChanged_{
			VitalsChanged: &playv1.WatchGameSessionResponse_VitalsChanged{Vitals: v},
		}},
	})
}

// settling finds the pending damage the master settles and the two
// combatants it is about, all in the change's own transaction.
func (s *Service) settling(ctx context.Context, c *combatTx, pendingID string) (p playdb.PendingDamage, attacker, target playdb.Combatant, err error) {
	p, err = c.q.GetPendingDamage(ctx, playdb.GetPendingDamageParams{EncounterID: c.enc.ID, ID: pendingID})
	if errors.Is(err, pgx.ErrNoRows) {
		return p, attacker, target, connect.NewError(connect.CodeNotFound, errors.New("pending damage not found"))
	}
	if err != nil {
		return p, attacker, target, fmt.Errorf("find the pending damage: %w", err)
	}
	cs, err := c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return p, attacker, target, fmt.Errorf("list the combatants: %w", err)
	}
	master := combatViewer{master: true}
	if target, err = findCombatant(cs, p.TargetID, master); err != nil {
		return p, attacker, target, err
	}
	if p.AttackerID == nil {
		return p, target, target, nil // a trap's damage (MR-035) has no attacker: the target stands in
	}
	attacker, err = findCombatant(cs, *p.AttackerID, master)
	return p, attacker, target, err
}

// DiscardPendingDamage implements playv1connect.CombatServiceHandler.
func (s *Service) DiscardPendingDamage(
	ctx context.Context,
	req *connect.Request[playv1.DiscardPendingDamageRequest],
) (*connect.Response[playv1.DiscardPendingDamageResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	key, err := parseKey(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	encID, err := parseCombatID(req.Msg.GetEncounterId(), "encounter")
	if err != nil {
		return nil, err
	}
	pendingID, err := parseCombatID(req.Msg.GetPendingDamageId(), "pending damage")
	if err != nil {
		return nil, err
	}

	var made actionEvent
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventDamageDiscarded, encounterID: encID}, func(c *combatTx) (any, error) {
		if err := notEnded(c.enc); err != nil {
			return nil, err
		}
		p, attacker, target, err := s.settling(ctx, c, pendingID)
		if err != nil {
			return nil, err
		}
		if p.Status == pendingApplied || p.Status == pendingDiscarded {
			return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_DAMAGE_RESOLVED, "the damage was applied or discarded already")
		}
		if _, err := c.q.SetPendingDamageStatus(ctx, playdb.SetPendingDamageStatusParams{ID: p.ID, Status: pendingDiscarded, ResolvedAt: &c.now}); err != nil {
			return nil, fmt.Errorf("discard the pending damage: %w", err)
		}
		if c.enc, err = c.q.TouchEncounter(ctx, c.enc.ID); err != nil {
			return nil, fmt.Errorf("touch the encounter: %w", err)
		}
		c.characterID = &attacker.CharacterID
		made = actionEvent{
			Round: c.enc.Round, Secret: secretOf(attacker, target), Actor: attacker.ID, Target: target.ID, Pending: p.ID, Key: p.AttackKey,
			Amount: num(p.Amount), PrevStatus: p.Status,
		}
		if p.CastID != nil && !p.Healing {
			// The discard may settle the cast's last damage for the target: what landed
			// before it still owes a concentration save.
			if made.ConcentrationDC, err = s.castConcentrationDC(ctx, c, p, target, 0); err != nil {
				return nil, err
			}
		}
		return made, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "discard a pending damage", err)
	}
	ev, err := resultEvent(res, made)
	if err != nil {
		return nil, s.dbError(ctx, "read the discarded damage", err)
	}
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, !ev.Secret)
	})
	if err != nil {
		return nil, err
	}
	pending, err := s.pendingFor(ctx, res, ev.Pending, combatViewer{master: true})
	if err != nil {
		return nil, err
	}
	decorate(pending, ev, combatViewer{master: true}, s.membersOf(ctx, res))
	return connect.NewResponse(&playv1.DiscardPendingDamageResponse{Encounter: out, PendingDamage: pending}), nil
}

// The feature actions with their own effect on the numbers (the others only
// spend the economy and a use and go to the log). The keys are the SRD feature
// keys ("feature:second-wind").
const (
	secondWind  = "feature:second-wind"
	actionSurge = "feature:action-surge-1-use"
)

// featureError is the failed_precondition for a reason a feature action is
// disabled with. The master has the last word on the economy only: a spent
// resource stops him too (he corrects the uses).
func featureError(r *rulesv1.DisabledReason, master bool) error {
	switch r.GetCode() {
	case rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_ACTION_USED:
		if !master {
			return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ACTION_USED, "the action of this turn is used")
		}
	case rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_BONUS_ACTION_USED:
		if !master {
			return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_BONUS_ACTION_USED, "the bonus action of this turn is used")
		}
	case rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_REACTION_USED:
		if !master {
			return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_REACTION_USED, "the reaction is used")
		}
	case rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_ALREADY_USED_THIS_TURN:
		if !master {
			return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ALREADY_USED_THIS_TURN, "already used in this turn")
		}
	case rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_ATTACK_ACTION_FIRST:
		if !master {
			return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ATTACK_ACTION_FIRST, "Flurry of Blows comes after the Attack action")
		}
	case rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_NO_USES:
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NO_USES, "no uses left",
			func(b *playv1.EncounterBlocked) { b.Recharge = r.GetRecharge() })
	}
	return nil
}

// TakeAction implements playv1connect.CombatServiceHandler.
func (s *Service) TakeAction(
	ctx context.Context,
	req *connect.Request[playv1.TakeActionRequest],
) (*connect.Response[playv1.TakeActionResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	key, err := parseKey(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	encID, err := parseCombatID(req.Msg.GetEncounterId(), "encounter")
	if err != nil {
		return nil, err
	}
	combID, err := parseCombatID(req.Msg.GetCombatantId(), "combatant")
	if err != nil {
		return nil, err
	}
	actionKey := req.Msg.GetActionKey()
	if actionKey == standardAttack || actionKey == standardCast {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("an attack is rolled with RollAttack, and a spell is cast with CastSpell"))
	}
	var in rollInput
	var rolled bool
	switch roll := req.Msg.GetRoll().(type) {
	case *playv1.TakeActionRequest_RollInApp:
		if !roll.RollInApp {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("roll_in_app must be true"))
		}
		in.inApp, rolled = true, true
	case *playv1.TakeActionRequest_TypedSum:
		in.typed, rolled = int(roll.TypedSum), true
	}
	v := viewerOf(m)

	var made actionEvent
	var vitals *playv1.CharacterVitals // the resource spent, or the hit points healed
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventActionTaken, encounterID: encID}, func(c *combatTx) (any, error) {
		vitals = nil
		cs, err := c.q.ListCombatants(ctx, c.enc.ID)
		if err != nil {
			return nil, fmt.Errorf("list the combatants: %w", err)
		}
		v = c.viewer(m, cs) // the fog: an NPC the player does not see is not found
		who, err := findCombatant(cs, combID, v)
		if err != nil {
			return nil, err
		}
		if err := v.mayAct(who); err != nil {
			return nil, err
		}
		opts, err := s.optionsOf(ctx, c.tx, m.CampaignID, who)
		if err != nil {
			return nil, err
		}
		if wildShapeFeature(actionKey) {
			// It needs the beast: AssumeWildShape takes it (MR-037).
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("the Wild Shape action is taken with AssumeWildShape"))
		}
		feature := strings.HasPrefix(actionKey, "feature:")
		list := opts.GetStandardActions()
		if feature {
			list = opts.GetFeatureActions()
		}
		i := slices.IndexFunc(list, func(a *rulesv1.ActionOption) bool { return a.GetAction().GetKey() == actionKey })
		if i < 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("action_key is not one of the standard or feature actions"))
		}
		action := list[i]
		economy := action.GetAction().GetEconomy()
		// A reaction is taken off turn, when its trigger happens; everything else
		// on the combatant's turn.
		if feature && economy == rulesv1.ActionEconomy_ACTION_ECONOMY_REACTION {
			if err := s.mustReactNowOrOnTurn(ctx, c, who); err != nil {
				return nil, err
			}
		} else if err := s.mustActNow(ctx, c, who); err != nil {
			return nil, err
		}
		// The master has the last word: he may act again with the action used.
		if !action.GetEnabled() {
			if feature {
				if err := featureError(action.GetReason(), v.master); err != nil {
					return nil, err
				}
			} else if !v.master {
				return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ACTION_USED, "the action of this turn is used")
			}
		}
		run, err := breakRun(ctx, c, who)
		if err != nil {
			return nil, err
		}
		made = actionEvent{
			Round: c.enc.Round, Secret: who.Hidden, Actor: who.ID, Key: actionKey, RunBefore: run,
			ActionBefore: who.ActionUsed, BonusBefore: who.BonusActionUsed, ReactionBefore: who.ReactionUsed, DashedBefore: who.Dashed,
			AttacksBefore: who.AttacksMade, DisengagedBefore: who.Disengaged, SurgedBefore: who.ActionSurged,
			AttackKeyBefore: deref(who.ActionAttackKey), FlurryBefore: who.BonusAttacksLeft,
		}
		after := who
		switch economy {
		case rulesv1.ActionEconomy_ACTION_ECONOMY_BONUS_ACTION:
			after.BonusActionUsed = true
		case rulesv1.ActionEconomy_ACTION_ECONOMY_REACTION:
			after.ReactionUsed = true
		case rulesv1.ActionEconomy_ACTION_ECONOMY_FREE, rulesv1.ActionEconomy_ACTION_ECONOMY_MOVEMENT:
			// costs no economy (Surto de ação)
		default:
			after.ActionUsed = true
		}
		if actionKey == actionSurge {
			// An additional action this turn, with the attacks of the Attack action.
			after.ActionUsed = false
			if err := c.q.SetCombatantActionSurged(ctx, playdb.SetCombatantActionSurgedParams{ID: who.ID, ActionSurged: true}); err != nil {
				return nil, fmt.Errorf("mark the action surge: %w", err)
			}
			if err := c.q.SetCombatantAttacksMade(ctx, playdb.SetCombatantAttacksMadeParams{ID: who.ID, AttacksMade: 0}); err != nil {
				return nil, fmt.Errorf("give the action back: %w", err)
			}
			// The new Attack action starts from nothing.
			if err := c.q.SetCombatantAttackState(ctx, playdb.SetCombatantAttackStateParams{ID: who.ID, BonusAttacksLeft: who.BonusAttacksLeft}); err != nil {
				return nil, fmt.Errorf("forget the last attack: %w", err)
			}
		}
		if err := c.q.SetCombatantEconomy(ctx, playdb.SetCombatantEconomyParams{
			ID: who.ID, ActionUsed: after.ActionUsed, BonusActionUsed: after.BonusActionUsed, ReactionUsed: after.ReactionUsed, Dashed: after.Dashed,
		}); err != nil {
			return nil, fmt.Errorf("spend the action: %w", err)
		}
		// What the action does to the turn: a standard action's own, or the one a
		// feature performs (Cunning Action's Dash, Step of the Wind's Disengage).
		standard := actionKey
		if feature {
			sheet, err := s.sheetOf(ctx, c.tx, m.CampaignID, who)
			if err != nil {
				return nil, err
			}
			fi := slices.IndexFunc(sheet.FeatureActions, func(a link.FeatureAction) bool { return a.Key == actionKey })
			if fi < 0 {
				return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("action_key is not one of the character's feature actions"))
			}
			fa := sheet.FeatureActions[fi]
			if fa.Standard != "" {
				standard = fa.Standard
			}
			// One use of its resource: only a player's character counts them, and a
			// pool of points (Cura pelas mãos) is the master's.
			if fa.Resource != "" && !fa.Pool && who.Kind == kindPlayer {
				if vitals, err = s.spendResource(ctx, c, who.CharacterID, fa.Resource, 1); err != nil {
					return nil, err
				}
				made.Resource = fa.Resource
			}
			if actionKey == flurryOfBlows {
				// Flurry of Blows comes right after the Attack action; its two unarmed
				// strikes are rolled as attacks (the master has the last word).
				last, hasLast := lastAttack(sheet, who)
				if !v.master && (!who.ActionUsed || who.AttacksMade == 0 || !hasLast || last.Spell) {
					return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ATTACK_ACTION_FIRST, "Flurry of Blows comes after the Attack action")
				}
				if err := c.q.SetCombatantAttackState(ctx, playdb.SetCombatantAttackStateParams{ID: who.ID, ActionAttackKey: who.ActionAttackKey, BonusAttacksLeft: flurryStrikes}); err != nil {
					return nil, fmt.Errorf("give the flurry strikes: %w", err)
				}
			}
			if actionKey == secondWind {
				vit, err := s.secondWind(ctx, c, m, v, who, sheet.FighterLevel, in, rolled, &made)
				if err != nil {
					return nil, err
				}
				if vit != nil {
					vitals = vit
				}
			}
		}
		if standard == "standard:dash" {
			if err := markDashed(ctx, c.q, who.ID); err != nil {
				return nil, err
			}
		}
		// Disengage: no opportunity attacks for the rest of the turn (slice 9.6b
		// reads the flag).
		if standard == "standard:disengage" {
			if err := c.q.SetCombatantDisengaged(ctx, playdb.SetCombatantDisengagedParams{ID: who.ID, Disengaged: true}); err != nil {
				return nil, fmt.Errorf("mark the disengage: %w", err)
			}
		}
		if c.enc, err = c.q.TouchEncounter(ctx, c.enc.ID); err != nil {
			return nil, fmt.Errorf("touch the encounter: %w", err)
		}
		c.characterID = &who.CharacterID
		return made, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "take an action", err)
	}
	ev, err := resultEvent(res, made)
	if err != nil {
		return nil, s.dbError(ctx, "read the action", err)
	}
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, !ev.Secret)
		s.publishVitals(m.CampaignID, vitals)
	})
	if err != nil {
		return nil, err
	}
	resp := &playv1.TakeActionResponse{Encounter: out}
	if ev.Heal {
		resp.Roll = diceRoll(ev.DiceCount, ev.DiceSides, ev.Faces, ev.Modifier, ev.Total, ev.Physical)
		resp.Healed = &ev.Amount
	}
	return connect.NewResponse(resp), nil
}

// mustReactNowOrOnTurn checks that a combatant may take a reaction now: the
// combat is running and it is not out of the fight or down; whether it is its
// own turn does not matter.
func (s *Service) mustReactNowOrOnTurn(ctx context.Context, c *combatTx, who playdb.Combatant) error {
	if err := notEnded(c.enc); err != nil {
		return err
	}
	switch {
	case c.enc.Status != statusActive:
		return gateError(rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_COMBAT_NOT_ACTIVE)
	case who.Defeated:
		return gateError(rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_COMBATANT_DEFEATED)
	}
	down, err := s.isDown(ctx, c.tx, c.session.CampaignID, who)
	if err != nil {
		return err
	}
	if down {
		return gateError(rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_COMBATANT_DOWN)
	}
	return nil
}

// secondWind is Retomar o fôlego: 1d10 plus the fighter's level of hit points,
// rolled like a damage (RN-18) and healed through the vitals (or the NPC's hit
// points). It fills the event with the roll and what changed, and returns the
// vitals of a player's character.
func (s *Service) secondWind(ctx context.Context, c *combatTx, m authz.Membership, v combatViewer, who playdb.Combatant, fighterLevel int, in rollInput, rolled bool, ev *actionEvent) (*playv1.CharacterVitals, error) {
	if !rolled {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("set roll_in_app or typed_sum for Retomar o fôlego"))
	}
	if !v.master {
		if err := s.mustRollThisWay(ctx, c.tx, m, in); err != nil {
			return nil, err
		}
	}
	expr := dice.Expr{Count: 1, Sides: 10, Modifier: fighterLevel}
	var roll dice.Result
	var err error
	if in.inApp {
		if roll, err = dice.Roll(s.roller, expr); err != nil {
			return nil, fmt.Errorf("roll the d10: %w", err)
		}
	} else if roll, err = dice.Physical(expr, in.typed); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("typed_sum must be 1 to 10"))
	}
	hit, vit, err := s.healCombatant(ctx, c, who, clamp32(max(roll.Total, 0), 0, math.MaxInt32))
	if err != nil {
		return nil, err
	}
	ev.Heal, ev.DiceCount, ev.DiceSides, ev.Faces = true, 1, 10, faces32(roll.Faces)
	ev.Modifier, ev.Total, ev.Physical = clamp32(fighterLevel, 0, 100), clamp32(roll.Total, 0, math.MaxInt32), roll.Physical
	ev.Amount, ev.Before, ev.After, ev.DeathBefore = hit.Amount, hit.Before, hit.After, hit.DeathBefore
	return vit, nil
}

// AdjustCombatantHitPoints implements playv1connect.CombatServiceHandler.
func (s *Service) AdjustCombatantHitPoints(
	ctx context.Context,
	req *connect.Request[playv1.AdjustCombatantHitPointsRequest],
) (*connect.Response[playv1.AdjustCombatantHitPointsResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	key, err := parseKey(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	encID, err := parseCombatID(req.Msg.GetEncounterId(), "encounter")
	if err != nil {
		return nil, err
	}
	combID, err := parseCombatID(req.Msg.GetCombatantId(), "combatant")
	if err != nil {
		return nil, err
	}
	var mode string
	var amount int32
	switch change := req.Msg.GetChange().(type) {
	case *playv1.AdjustCombatantHitPointsRequest_Damage:
		mode, amount = "damage", change.Damage
	case *playv1.AdjustCombatantHitPointsRequest_Heal:
		mode, amount = "heal", change.Heal
	case *playv1.AdjustCombatantHitPointsRequest_HitPoints:
		mode, amount = "set", change.HitPoints
	}
	temp := req.Msg.HitPointsTemporary
	switch {
	case mode == "" && temp == nil:
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("set damage, heal, hit_points or hit_points_temporary"))
	case mode != "" && (amount < 0 || amount > maxHitPointChange):
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("%s must be 0 to %d", mode, maxHitPointChange))
	case temp != nil && (*temp < 0 || *temp > maxTempHitPoints):
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("hit_points_temporary must be 0 to %d", maxTempHitPoints))
	}

	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventHitPointsAdjusted, encounterID: encID}, func(c *combatTx) (any, error) {
		if err := notEnded(c.enc); err != nil {
			return nil, err
		}
		cs, err := c.q.ListCombatants(ctx, c.enc.ID)
		if err != nil {
			return nil, fmt.Errorf("list the combatants: %w", err)
		}
		target, err := findCombatant(cs, combID, viewerOf(m))
		if err != nil {
			return nil, err
		}
		if !holdsHP(target) {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("a player's character keeps its hit points in its vitals: use AdjustCharacterVitals"))
		}
		hp, tmp, hpMax := int(num(target.HpCurrent)), int(num(target.HpTemp)), int(num(target.HpMax))
		switch mode {
		case "damage":
			dmg := combat.ApplyDamage(hp, tmp, int(amount))
			hp, tmp = dmg.HP, dmg.TempHP
		case "heal":
			hp = combat.ApplyHeal(hp, hpMax, int(amount)).HP
		case "set":
			if int(amount) > hpMax {
				return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("hit_points must be 0 to %d", hpMax))
			}
			hp = int(amount)
		}
		if temp != nil {
			tmp = int(*temp)
		}
		after := hpState{HP: clamp32(hp, 0, math.MaxInt32), Temp: clamp32(tmp, 0, math.MaxInt32), Defeated: hp == 0}
		if err := c.q.SetCombatantHitPoints(ctx, playdb.SetCombatantHitPointsParams{ID: target.ID, HpCurrent: &after.HP, HpTemp: &after.Temp, Defeated: after.Defeated}); err != nil {
			return nil, fmt.Errorf("save the hit points: %w", err)
		}
		if c.enc, err = c.q.TouchEncounter(ctx, c.enc.ID); err != nil {
			return nil, fmt.Errorf("touch the encounter: %w", err)
		}
		before := hpOf(target)
		c.characterID = &target.CharacterID
		return actionEvent{
			Round: c.enc.Round, Secret: true, Actor: target.ID, Mode: mode, Amount: max(amount, 0),
			Delta:  after.HP - before.HP,
			Before: &before, After: &after,
		}, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "adjust a combatant's hit points", err)
	}
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChangedFor(ctx, m.CampaignID, d, combID)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, false) // the master's correction: his line only
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.AdjustCombatantHitPointsResponse{Encounter: out}), nil
}
