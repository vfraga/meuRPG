package play

import (
	"context"
	"slices"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// What a controlled mount does (SRD 5.1, "Mounted Combat"): while it carries a rider it acts as the rider directs and can take only the
// Dash, Disengage and Dodge actions. Every action of the combat goes through mustActNow, which refuses the others; TakeAction knows the
// action it is asked for and lets the three through. A reaction is not an action: it stays free (mustReactNow does not ask). The movement
// is the mount's own, on its own counter. An independent mount, or a controlled one with no rider, acts as any creature does.

// mountLimitText is the reason the options give for an action of a mount that carries a rider.
const mountLimitText = "A montaria que leva alguém só pode usar Disparada, Desengajar ou Esquivar."

// mountActionAllowed says the action is one a controlled mount that carries a rider takes.
func mountActionAllowed(key string) bool {
	return key == actionDash || key == actionDisengage || key == actionDodge
}

// carriesRider says the combatant is a controlled mount with a rider on it: the one whose actions are limited.
func carriesRider(cs []playdb.Combatant, who playdb.Combatant) bool {
	return who.MountKind == mountKindControlled && riderOf(cs, who.ID) != nil
}

// mustNotBeLimited refuses the action key of a controlled mount that carries a rider, unless it is one of the three it takes. key is
// empty for an action that is none of them (an attack, a spell, a contest, a creature's action).
func (s *Service) mustNotBeLimited(ctx context.Context, c *combatTx, who playdb.Combatant, key string) error {
	if who.MountKind != mountKindControlled || mountActionAllowed(key) {
		return nil
	}
	cs, err := listCombatants(ctx, c)
	if err != nil {
		return err
	}
	return mountLimit(cs, who, key)
}

// mountLimit is mustNotBeLimited for a caller that has the combatants (cs) in hand.
func mountLimit(cs []playdb.Combatant, who playdb.Combatant, key string) error {
	if mountActionAllowed(key) || !carriesRider(cs, who) {
		return nil
	}
	return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_MOUNT_ACTION_LIMITED, "a mount that carries a rider takes only Dash, Disengage and Dodge",
		func(b *playv1.EncounterBlocked) { b.CombatantIds = []string{who.ID} })
}

// limitMountOptions shows the turn options of a mount that carries a rider as it takes them: Dash, Disengage and Dodge stay as they were,
// and every other row is disabled with the words that say why. The player who directs the mount without owning it (the rider's, for a
// creature of the master's) reads the three actions and the mount's economy, and nothing else: the rest of an NPC's stat block is not
// theirs to read (RN-20).
func limitMountOptions(res *playv1.GetTurnOptionsResponse, cs []playdb.Combatant, v combatViewer, who playdb.Combatant) {
	o := res.GetOptions()
	if o == nil || !carriesRider(cs, who) {
		return
	}
	no := func() *rulesv1.DisabledReason {
		return &rulesv1.DisabledReason{TextPt: mountLimitText, TextMasterPt: mountLimitText}
	}
	keep := func(a *rulesv1.ActionOption) bool { return mountActionAllowed(a.GetAction().GetKey()) }
	if !v.master && !v.owns(who) {
		o.Attacks, o.Spells, o.FeatureActions = nil, nil, nil
		o.StandardActions = slices.DeleteFunc(o.StandardActions, func(a *rulesv1.ActionOption) bool { return !keep(a) })
		res.AttackTargets, res.SpellTargets, res.ResourceTargets, res.FeatureTargets = nil, nil, nil, nil
		res.ContestAttackOptions, res.ManeuverDice, res.ExtraAction = nil, nil, nil
		// What is left is the mount's economy and the three actions: the dice of its pending damage, the rolls of its contests, its notes and
		// its jumps are the master's (RN-20).
		res.PendingDamages, res.ContestState, res.EffectNotes, o.Jumps = nil, nil, nil, nil
		return
	}
	for _, a := range o.GetAttacks() {
		a.Enabled, a.Reason = false, no()
	}
	for _, sp := range o.GetSpells() {
		sp.Enabled, sp.Reason = false, no()
	}
	for _, a := range slices.Concat(o.GetStandardActions(), o.GetFeatureActions()) {
		if !keep(a) {
			a.Enabled, a.Reason = false, no()
		}
	}
	for _, co := range res.GetContestAttackOptions() {
		co.Enabled, co.Reason = false, no()
	}
	for _, a := range res.GetContestState().GetHideActions() {
		a.Enabled, a.Reason = false, no()
	}
	if h := res.GetContestState().GetHelpAction(); h != nil {
		h.Enabled, h.Reason = false, no()
	}
}
