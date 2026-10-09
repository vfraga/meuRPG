package play

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"
	"uuid"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// maxMonsterActionKey bounds an action key from a client, and maxMonsterRoutines a routine (the
// SRD's biggest Multiattack has 6).
const (
	maxMonsterActionKey = 200
	maxMonsterRoutines  = 20
)

// creatureUse is a request to use an action of a monster, once checked.
type creatureUse struct {
	m         authz.Membership
	key       string
	hash      *string
	encID     string
	actorID   string
	actionKey string
	targetIDs []string
	routine   int
	legendary bool
}

// checkedUse reads and checks what the two ways of using an action share.
func checkedUse(m authz.Membership, key, encounterID, combatantID, actionKey string, targets []string, routine int32, legendary bool) (creatureUse, error) {
	encID, err := parseCombatID(encounterID, "encounter")
	if err != nil {
		return creatureUse{}, err
	}
	actorID, err := parseCombatID(combatantID, "combatant")
	if err != nil {
		return creatureUse{}, err
	}
	if actionKey == "" || len(actionKey) > maxMonsterActionKey {
		return creatureUse{}, connect.NewError(connect.CodeInvalidArgument, errors.New("action_key must name an action of the creature"))
	}
	if len(targets) > maxSpellTargets {
		return creatureUse{}, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("target_ids must have at most %d entries", maxSpellTargets))
	}
	use := creatureUse{m: m, key: key, encID: encID, actorID: actorID, actionKey: actionKey, routine: int(routine), legendary: legendary}
	for i, raw := range targets {
		id, err := parseCombatID(raw, "combatant")
		if err != nil {
			return creatureUse{}, err
		}
		if slices.Contains(use.targetIDs, id) {
			return creatureUse{}, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("target_ids[%d] repeats a combatant", i))
		}
		use.targetIDs = append(use.targetIDs, id)
	}
	if routine < 0 || routine > maxMonsterRoutines {
		return creatureUse{}, connect.NewError(connect.CodeInvalidArgument, errors.New("routine is not a routine of the Multiattack action"))
	}
	return use, nil
}

// UseCreatureAction implements playv1connect.CreatureServiceHandler.
func (s *Service) UseCreatureAction(
	ctx context.Context,
	req *connect.Request[playv1.UseCreatureActionRequest],
) (*connect.Response[playv1.UseCreatureActionResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	key, err := parseKey(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	use, err := checkedUse(m, key, req.Msg.GetEncounterId(), req.Msg.GetCombatantId(), req.Msg.GetActionKey(), req.Msg.GetTargetIds(), req.Msg.GetRoutine(), false)
	if err != nil {
		return nil, err
	}
	use.campaignID(req.Msg.GetCampaignId())
	use.hash = idem.Hash(req.Msg)
	if err := checkAnchor(req.Msg.GetOrigin(), req.Msg.GetDirection()); err != nil {
		return nil, err
	}
	out, result, err := s.runCreatureAction(ctx, use)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.UseCreatureActionResponse{Encounter: out, Result: result}), nil
}

// UseLegendaryAction implements playv1connect.CreatureServiceHandler.
func (s *Service) UseLegendaryAction(
	ctx context.Context,
	req *connect.Request[playv1.UseLegendaryActionRequest],
) (*connect.Response[playv1.UseLegendaryActionResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	key, err := parseKey(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	use, err := checkedUse(m, key, req.Msg.GetEncounterId(), req.Msg.GetCombatantId(), req.Msg.GetOptionKey(), req.Msg.GetTargetIds(), 0, true)
	if err != nil {
		return nil, err
	}
	use.hash = idem.Hash(req.Msg)
	if err := checkAnchor(req.Msg.GetOrigin(), req.Msg.GetDirection()); err != nil {
		return nil, err
	}
	out, result, err := s.runCreatureAction(ctx, use)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.UseLegendaryActionResponse{Encounter: out, Result: result}), nil
}

// campaignID is a no-op kept for the readers of the two handlers: the membership carries it.
func (u *creatureUse) campaignID(string) {}

// checkAnchor checks where an area starts or points: a square with a negative coordinate is not on
// any map. The master picks the targets, so nothing more is checked.
func checkAnchor(origin *playv1.GridPoint, _ playv1.CompassDirection) error {
	if origin != nil && (origin.GetCol() < 0 || origin.GetRow() < 0) {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("origin is not a square of the map"))
	}
	return nil
}

// derivedKey is the idempotency key of one of the steps of a use of an action that is several
// changes (the attack roll, each damage roll): the same for the same use, so a retry of the
// whole replays the steps already made.
func derivedKey(base, step string) string {
	sum := sha256.Sum256([]byte(base + "/" + step))
	var id uuid.UUID
	copy(id[:], sum[:16])
	id[6] = id[6]&0x0f | 0x50
	id[8] = id[8]&0x3f | 0x80
	return id.String()
}

// runCreatureAction uses the action: an attack is made whole (its roll and the damage of every
// part), anything else is one change of the combat.
func (s *Service) runCreatureAction(ctx context.Context, use creatureUse) (*playv1.Encounter, *playv1.CreatureActionResult, error) {
	cs, err := s.queries.ListCombatants(ctx, use.encID)
	if err != nil || len(cs) == 0 {
		return s.useThroughWrite(ctx, use) // a combat that is not the campaign's: the write says so
	}
	actor, err := findCombatant(cs, use.actorID, combatViewer{master: true})
	if err != nil {
		return s.useThroughWrite(ctx, use)
	}
	mon, err := s.monsterOf(ctx, nil, use.m.CampaignID, actor)
	if err != nil {
		return nil, nil, err
	}
	if mon == nil {
		return nil, nil, errNotAMonster()
	}
	ap, opt, err := resolveMonsterAction(mon.plan, use.actionKey, use.legendary)
	if err != nil {
		return nil, nil, err
	}
	if opt != nil && opt.ActionKey != "" {
		if linked, ok := mon.plan.Action(opt.ActionKey); ok && linked.Kind == rules.ActionKindAttack {
			return s.useAttack(ctx, use, mon, linked, opt)
		}
	}
	if opt == nil && ap.Kind == rules.ActionKindAttack {
		return s.useAttack(ctx, use, mon, ap, nil)
	}
	return s.useThroughWrite(ctx, use)
}

// useAttack makes an attack of a monster whole: the attack roll, then the damage of every part,
// each its own roll, rolled by the app (the master's NPC rolls either way, and the damage of a
// monster is the server's roll). The steps are the master's own calls, replayed by a retry.
func (s *Service) useAttack(ctx context.Context, use creatureUse, mon *monsterRef, ap rules.ActionPlan, opt *rules.LegendaryOption) (*playv1.Encounter, *playv1.CreatureActionResult, error) {
	if len(use.targetIDs) != 1 {
		return nil, nil, connect.NewError(connect.CodeInvalidArgument, errors.New("an attack takes exactly one target"))
	}
	campaignID := use.m.CampaignID
	attackReq := &playv1.RollAttackRequest{
		CampaignId: campaignID, EncounterId: use.encID, AttackerId: use.actorID, AttackKey: ap.Key, TargetId: use.targetIDs[0],
		IdempotencyKey: derivedKey(use.key, "attack"), Roll: &playv1.RollAttackRequest_RollInApp{RollInApp: true},
	}
	if opt != nil {
		attackReq.LegendaryOptionKey = opt.Key
	}
	attack, err := s.RollAttack(ctx, connect.NewRequest(attackReq))
	if err != nil {
		return nil, nil, err
	}
	out := attack.Msg.GetEncounter()
	result := &playv1.CreatureActionResult{
		AttackRoll: attack.Msg.GetRoll(), Outcome: attack.Msg.GetRoll().GetOutcome(), Rider: attack.Msg.GetRider(),
	}
	first := attack.Msg.GetPendingDamage()
	if first == nil {
		return out, result, nil // a miss, or a hit with no damage (a web)
	}
	result.PendingDamageId = first.GetId()
	if first.GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_AWAITING_ROLL {
		return out, result, nil // the target's Shield is awaited: the damage waits for the answer
	}
	target, tmods, err := s.stepTarget(ctx, use, use.targetIDs[0])
	if err != nil {
		return nil, nil, err
	}
	pending := []string{first.GetId()}
	for i := 0; i < len(pending); i++ {
		dmg, err := s.RollDamage(ctx, connect.NewRequest(&playv1.RollDamageRequest{
			CampaignId: campaignID, EncounterId: use.encID, PendingDamageId: pending[i],
			IdempotencyKey: derivedKey(use.key, fmt.Sprintf("damage-%d", i)), Roll: &playv1.RollDamageRequest_RollInApp{RollInApp: true},
		}))
		if err != nil {
			return nil, nil, err
		}
		out = dmg.Msg.GetEncounter()
		result.DamageParts = append(result.DamageParts, damageRollProto(dmg.Msg.GetPendingDamage(), target, tmods))
		if r := dmg.Msg.GetRider(); r != nil {
			result.Rider = r
		}
		for _, f := range dmg.Msg.GetFollowUpPendingDamages() {
			pending = append(pending, f.GetId())
		}
	}
	return out, result, nil
}

// stepTarget is the target of an attack with the damage modifiers its resistance steps read:
// only a creature that takes its damage at once and has a stat block (an NPC) has any; a
// character's are the master's to apply (RN-02).
func (s *Service) stepTarget(ctx context.Context, use creatureUse, targetID string) (playdb.Combatant, *combat.TypeModifiers, error) {
	cs, err := s.queries.ListCombatants(ctx, use.encID)
	if err != nil {
		return playdb.Combatant{}, nil, s.dbError(ctx, "list the combatants", err)
	}
	target, err := findCombatant(cs, targetID, combatViewer{master: true})
	if err != nil || !holdsHP(target) {
		return target, nil, nil //nolint:nilerr // a target that left has no steps to show
	}
	mods, err := s.roster.DamageModifiers(ctx, nil, use.m.CampaignID, target.CharacterID, deref(target.MonsterKey))
	if err != nil {
		return target, nil, s.dbError(ctx, "read the damage modifiers", err)
	}
	return target, &mods, nil
}

// damageRollProto is the damage of one part of a monster's attack once rolled, with the steps of
// the target's immunity, resistance and vulnerability to its type, in the SRD's order (SRD 5.1,
// "Damage Resistance and Vulnerability": the modifiers first, then the resistance, then the
// vulnerability, each rounded down). mods is nil when the target has none to show.
func damageRollProto(p *playv1.PendingDamage, _ playdb.Combatant, mods *combat.TypeModifiers) *playv1.CreatureDamageRoll {
	out := &playv1.CreatureDamageRoll{
		DamageTypeKey: p.GetDamageTypeKey(), DamageTypePt: p.GetDamageTypePt(), Rolled: p.GetRoll(), Amount: p.GetAmount(),
		PendingDamageId: p.GetId(), Status: p.GetStatus(),
	}
	if mods != nil && p.GetRoll() != nil {
		out.Steps = damageSteps(*mods, p.GetDamageTypeKey(), p.GetRoll().GetTotal())
	}
	return out
}

// damageSteps lists what a target's modifiers to a damage type did to the damage.
func damageSteps(mods combat.TypeModifiers, typeKey string, before int32) []*playv1.CreatureDamageStep {
	var steps []*playv1.CreatureDamageStep
	value := before
	step := func(kind playv1.CreatureActionStepKind, after int32) {
		steps = append(steps, &playv1.CreatureDamageStep{Kind: kind, SourceKey: typeKey, Before: value, After: after})
		value = after
	}
	switch {
	case slices.Contains(mods.Immune, typeKey):
		step(playv1.CreatureActionStepKind_CREATURE_ACTION_STEP_KIND_IMMUNITY, 0)
	default:
		if slices.Contains(mods.Resistant, typeKey) {
			step(playv1.CreatureActionStepKind_CREATURE_ACTION_STEP_KIND_RESISTANCE, value/2)
		}
		if slices.Contains(mods.Vulnerable, typeKey) {
			step(playv1.CreatureActionStepKind_CREATURE_ACTION_STEP_KIND_VULNERABILITY, value*2)
		}
	}
	return steps
}

// resolveMonsterAction finds the action a request names: an action of the stat block, or, for a
// legendary action, the option, which is the option's own saving throw, the action of the stat block
// it makes, or a reminder.
func resolveMonsterAction(plan *rules.MonsterPlan, key string, legendary bool) (rules.ActionPlan, *rules.LegendaryOption, error) {
	if !legendary {
		ap, ok := plan.Action(key)
		if !ok {
			return rules.ActionPlan{}, nil, connect.NewError(connect.CodeInvalidArgument, errors.New("action_key is not an action of the creature"))
		}
		return ap, nil, nil
	}
	if plan.Legendary == nil {
		return rules.ActionPlan{}, nil, connect.NewError(connect.CodeInvalidArgument, errors.New("the creature has no legendary actions"))
	}
	i := slices.IndexFunc(plan.Legendary.Options, func(o rules.LegendaryOption) bool { return o.Key == key })
	if i < 0 {
		return rules.ActionPlan{}, nil, connect.NewError(connect.CodeInvalidArgument, errors.New("option_key is not a legendary action of the creature"))
	}
	opt := plan.Legendary.Options[i]
	switch {
	case opt.Action != nil:
		return *opt.Action, &opt, nil
	case opt.ActionKey != "":
		linked, _ := plan.Action(opt.ActionKey)
		// The limit of the action it makes is not charged: a legendary action costs legendary actions.
		linked.Usage = rules.ActionUsage{Kind: rules.UsageAtWill}
		return linked, &opt, nil
	}
	return rules.ActionPlan{Key: opt.Key, Name: opt.Name, Kind: rules.ActionKindOther, Class: rules.ActionText, Text: opt.Text}, &opt, nil
}

// useThroughWrite uses an action that is one change of the combat: a saving throw, the
// Multiattack action, or an action with no roll.
func (s *Service) useThroughWrite(ctx context.Context, use creatureUse) (*playv1.Encounter, *playv1.CreatureActionResult, error) {
	m := use.m
	v := viewerOf(m)
	var made actionEvent
	res, err := s.write(ctx, combatWrite{m: m, key: use.key, hash: use.hash, kind: eventSpellCast, altKind: eventActionTaken, encounterID: use.encID}, func(c *combatTx) (any, error) {
		made = actionEvent{}
		cs, err := c.q.ListCombatants(ctx, c.enc.ID)
		if err != nil {
			return nil, fmt.Errorf("list the combatants: %w", err)
		}
		v = c.viewer(m, cs)
		actor, err := findCombatant(cs, use.actorID, v)
		if err != nil {
			return nil, err
		}
		mon, err := s.monsterOf(ctx, c.tx, m.CampaignID, actor)
		if err != nil {
			return nil, err
		}
		if mon == nil {
			return nil, errNotAMonster()
		}
		st, err := monsterStateOf(actor)
		if err != nil {
			return nil, err
		}
		before := st.Clone()

		ap, opt, err := resolveMonsterAction(mon.plan, use.actionKey, use.legendary)
		if err != nil {
			return nil, err
		}
		if ap.Kind == rules.ActionKindAttack {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("an attack roll is made with an attack target: set exactly one target"))
		}
		if opt == nil && ap.Kind == rules.ActionKindOther && isSpellcastingAction(ap) {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("a spell is cast with CastSpell"))
		}
		if opt != nil {
			err = s.legendaryGate(c, actor, st, mon.plan, opt.Cost)
		} else {
			err = s.mustActNow(ctx, c, actor)
		}
		if err != nil {
			return nil, err
		}
		targs := make([]playdb.Combatant, len(use.targetIDs))
		for i, id := range use.targetIDs {
			if targs[i], err = findCombatant(cs, id, v); err != nil {
				return nil, err
			}
			if targs[i].ID == actor.ID {
				return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("a creature does not target itself"))
			}
			if targs[i].Defeated {
				return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TARGET_DEFEATED, "a target is defeated")
			}
		}
		switch {
		case ap.Kind == rules.ActionKindSave && len(targs) == 0:
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("target_ids needs the creatures the action affects"))
		case ap.Kind != rules.ActionKindSave && len(targs) > 0:
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("this action takes no targets"))
		}
		made = actionEvent{
			Round: c.enc.Round, Actor: actor.ID, Key: ap.Key, CastID: uuid.New().String(),
			ActionBefore: actor.ActionUsed, BonusBefore: actor.BonusActionUsed, ReactionBefore: actor.ReactionUsed, DashedBefore: actor.Dashed,
			SpellCastBefore: actor.SpellCast, BonusSpellBefore: actor.BonusSpellCast, DisengagedBefore: actor.Disengaged,
			SurgedBefore: actor.ActionSurged, AttacksBefore: actor.AttacksMade, AttackKeyBefore: deref(actor.ActionAttackKey), FlurryBefore: actor.BonusAttacksLeft,
			Monster: &monsterEvent{},
		}
		if opt != nil {
			made.Key = opt.Key
			made.Monster.Option, made.Monster.Cost = opt.Key, clamp32(opt.Cost, 0, maxLegendaryCost)
			if err := st.SpendLegendary(mon.plan, opt.Cost); err != nil {
				return nil, err
			}
			st.Offer = nil // one for each offer
		} else {
			if err := st.Spend(ap.Usage, ap.Key); err != nil {
				return nil, limitError(err)
			}
			if made.RunBefore, err = breakRun(ctx, c, actor); err != nil {
				return nil, err
			}
			// The action is spent; the master may act again with it used.
			if err := c.q.SetCombatantEconomy(ctx, playdb.SetCombatantEconomyParams{
				ID: actor.ID, ActionUsed: true, BonusActionUsed: actor.BonusActionUsed, ReactionUsed: actor.ReactionUsed, Dashed: actor.Dashed,
			}); err != nil {
				return nil, fmt.Errorf("spend the action: %w", err)
			}
		}
		if ap.Kind == rules.ActionKindMultiattack {
			if err := st.StartMultiattack(ap, use.routine); err != nil {
				return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("routine is not a routine of the Multiattack action"))
			}
			made.Monster.Routine, made.Monster.Routined = clamp32(use.routine, 0, maxMonsterRoutines), true
		}
		c.kind = eventActionTaken
		if ap.Kind == rules.ActionKindSave {
			c.kind = eventSpellCast
			if err := s.rollMonsterSave(ctx, c, m, v, cs, actor, targs, ap, &made); err != nil {
				return nil, err
			}
		}
		if !statesEqual(before, st) {
			if err := saveMonsterState(ctx, c, actor, st); err != nil {
				return nil, err
			}
			made.Monster.Before = &before
		}
		if c.enc, err = c.q.TouchEncounter(ctx, c.enc.ID); err != nil {
			return nil, fmt.Errorf("touch the encounter: %w", err)
		}
		c.characterID = &actor.CharacterID
		return made, nil
	})
	if err != nil {
		return nil, nil, s.dbError(ctx, "use a creature action", err)
	}
	if v, err = s.viewerAfter(ctx, m, res, v); err != nil {
		return nil, nil, s.dbError(ctx, "work out what the player sees", err)
	}
	ev, err := resultEvent(res, made)
	if err != nil {
		return nil, nil, s.dbError(ctx, "read the action", err)
	}
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, !ev.Secret)
	})
	if err != nil {
		return nil, nil, err
	}
	result := &playv1.CreatureActionResult{}
	if ev.Monster != nil {
		result.ImmuneTargetIds = ev.Monster.Immune
	}
	if len(ev.Hits) > 0 {
		if result.Cast, err = s.castProto(ctx, res, ev, v); err != nil {
			return nil, nil, err
		}
	}
	return out, result, nil
}

// isSpellcastingAction says an action with no roll is the creature's casting of a spell, which
// CastSpell does.
func isSpellcastingAction(a rules.ActionPlan) bool {
	return a.Name == "Spellcasting" || a.Name == "Innate Spellcasting"
}

// rollMonsterSave does what an action that asks a saving throw does to its targets, as a spell
// does (CastSpell): the server rolls each target's saving throw, opens the damage it leaves and
// gives a failed target the condition of the action.
func (s *Service) rollMonsterSave(ctx context.Context, c *combatTx, m authz.Membership, v combatViewer, cs []playdb.Combatant, actor playdb.Combatant, targs []playdb.Combatant, ap rules.ActionPlan, made *actionEvent) error {
	sv := ap.Save
	sp := link.Spell{
		Key: ap.Key, Name: ap.Name, SaveAbility: string(sv.Ability), SaveDC: sv.DC, SaveOnSuccess: sv.OnSuccess, Area: true,
	}
	for _, d := range sv.Damage {
		sp.Damages = append(sp.Damages, link.Dice{Count: d.Dice.Count, Sides: d.Dice.Sides, Bonus: d.Dice.Bonus, DamageType: d.TypeKey})
	}
	known, err := c.sight.knownTerrain(ctx, c.tx, v)
	if err != nil {
		return err
	}
	terrain, err := s.terrainOf(ctx, c.tx, m.CampaignID, c.enc)
	if err != nil {
		return err
	}
	c.castID = made.CastID
	hidden := actor.Hidden
	for _, t := range targs {
		// A creature immune to everything the action does is never offered the save: the
		// condition it gives (and no damage), or every damage type it deals (SRD 5.1, Monsters).
		skip, err := s.immuneToAction(ctx, c, t, sv)
		if err != nil {
			return err
		}
		if skip {
			made.Monster.Immune = append(made.Monster.Immune, t.ID)
			hidden = hidden || t.Hidden
			continue
		}
		hit := castHit{Target: t.ID}
		cp, err := c.coverOf(ctx, v, terrain, known, actor, t, cs)
		if err != nil {
			return err
		}
		if err := s.resolveOnTarget(ctx, c, m, sp, actor, t, 0, rollInput{inApp: true}, cp, &hit); err != nil {
			return err
		}
		if hit.Save != nil && !hit.Save.Saved && sv.ConditionKey != "" {
			added, immune, before, err := s.addCondition(ctx, c, t, sv.ConditionKey)
			if err != nil {
				return err
			}
			if added {
				made.Monster.Conds = append(made.Monster.Conds, condChange{Target: t.ID, Before: before})
			}
			if immune {
				made.Monster.Immune = append(made.Monster.Immune, t.ID)
			}
		}
		made.Hits = append(made.Hits, hit)
		hidden = hidden || t.Hidden
	}
	packCoverSeen(made)
	made.Secret = hidden
	return s.openResistancePrompts(ctx, c, actor, cs, made)
}

// immuneToAction says whether the target cannot be affected by the save action at all: it is
// immune to the condition and the action deals no damage, or immune to every damage type the
// action deals and it gives no condition (or is immune to it too). The reason is the master's alone.
func (s *Service) immuneToAction(ctx context.Context, c *combatTx, target playdb.Combatant, sv *rules.ActionSave) (bool, error) {
	if sv.ConditionKey == "" && len(sv.Damage) == 0 {
		return false, nil
	}
	condImmune := sv.ConditionKey == ""
	if sv.ConditionKey != "" {
		var err error
		if condImmune, err = s.conditionImmune(ctx, c.tx, c.session.CampaignID, target, sv.ConditionKey); err != nil {
			return false, err
		}
	}
	dmgImmune := len(sv.Damage) == 0
	if len(sv.Damage) > 0 {
		mods, err := s.roster.DamageModifiers(ctx, c.tx, c.session.CampaignID, target.CharacterID, deref(target.MonsterKey))
		if err != nil {
			return false, err
		}
		dmgImmune = true
		for _, d := range sv.Damage {
			dmgImmune = dmgImmune && slices.Contains(mods.Immune, d.TypeKey)
		}
	}
	return condImmune && dmgImmune, nil
}
