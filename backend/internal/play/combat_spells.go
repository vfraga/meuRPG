package play

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"uuid"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dice"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// Casting a spell (MR-014, RN-02, RN-18, RN-22, Etapa 6, slice 6.4b). The cast
// spends the slot and the economy at once; what it does to the targets depends
// on the spell: a spell attack compares a d20 with the armor class, a saving
// throw spell has the server roll each target's save, Magic Missile's darts
// never miss, a healing spell opens a heal, and the rest only spends and logs.
// The damage and the heals wait as pending damages for RollDamage
// (combat_actions.go), which is also where an area spell's one damage roll
// settles all its targets.

// Magic Missile is the one spell whose darts the caster shares out: three, and
// one more for each slot level above the 1st; each dart is its own damage
// divided by the number of darts at the spell's level (1d4 + 1).
const magicMissile = "spell:magic-missile"

// scorchingRay: a ray for each target, three at the 2nd level and one more for
// each level above (the SRD lets several rays go to one target; the table keeps
// it to one ray per target here, and the master the rest).
const scorchingRay = "spell:scorching-ray"

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
}

// castableOf finds the spell among the options. invalid_argument when the
// caster does not have it, or it is a cantrip with an attack roll.
func castableOf(opts *rulesv1.TurnOptions, key string) (castable, error) {
	for _, sp := range opts.GetSpells() {
		if sp.GetSpell().GetKey() == key {
			return castable{level: sp.GetSpell().GetLevel(), economy: sp.GetEconomy(), slots: sp.GetSlots(), enabled: sp.GetEnabled(), reason: sp.GetReason()}, nil
		}
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
	case rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_NO_SLOT:
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NO_SLOT, "there is no free spell slot for this spell",
			func(b *playv1.EncounterBlocked) { b.MinLevel = r.GetMinLevel() })
	case rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_REACTION_ONLY_WHEN_HIT, rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_REACTION_ONLY:
		return connect.NewError(connect.CodeInvalidArgument, errors.New("a reaction spell is cast when its trigger happens (UseReaction, for Escudo)"))
	case rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_CASTING_TIME_TOO_LONG:
		return connect.NewError(connect.CodeInvalidArgument, errors.New("this spell takes too long to cast in a fight"))
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
// the caster) reaches the caster alone, Touch 5 ft, a ranged spell its range;
// sight, unlimited and special ranges leave it to the table.
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
		return 3 + max(slotLevel-sp.Level, 0)
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
	casterID, err := parseCombatID(req.Msg.GetCasterId(), "combatant")
	if err != nil {
		return nil, err
	}
	spellKey := req.Msg.GetSpellKey()
	if spellKey == "" || len(spellKey) > 100 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("spell_key must name one of the caster's spells"))
	}
	if len(req.Msg.GetTargets()) > maxSpellTargets {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("targets must have at most %d entries", maxSpellTargets))
	}
	type target struct {
		id    string
		darts int
	}
	var targets []target
	for i, t := range req.Msg.GetTargets() {
		id, err := parseCombatID(t.GetCombatantId(), "combatant")
		if err != nil {
			return nil, err
		}
		if slices.ContainsFunc(targets, func(o target) bool { return o.id == id }) {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("targets[%d] repeats a combatant", i))
		}
		if t.GetDarts() < 0 || t.GetDarts() > 20 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("targets[%d].darts must be 0 to 20", i))
		}
		targets = append(targets, target{id: id, darts: int(t.GetDarts())})
	}
	var in rollInput
	var rolled bool
	switch roll := req.Msg.GetRoll().(type) {
	case *playv1.CastSpellRequest_RollInApp:
		if !roll.RollInApp {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("roll_in_app must be true"))
		}
		in.inApp, rolled = true, true
	case *playv1.CastSpellRequest_D20Face:
		in.typed, rolled = int(roll.D20Face), true
		if in.typed < 1 || in.typed > 20 {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("d20_face must be 1 to 20"))
		}
	case *playv1.CastSpellRequest_PoolSum:
		in.typed, in.pool, rolled = int(roll.PoolSum), true, true
	}
	v := viewerOf(m)

	var made actionEvent
	var vitals []*playv1.CharacterVitals // the slot spent, if it was a player's, and the characters a hit point spell changed
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventSpellCast, encounterID: encID}, func(c *combatTx) (any, error) {
		vitals = nil
		cs, err := c.q.ListCombatants(ctx, c.enc.ID)
		if err != nil {
			return nil, fmt.Errorf("list the combatants: %w", err)
		}
		v = c.viewer(m, cs) // the fog: an NPC the player does not see is not found
		caster, err := findCombatant(cs, casterID, v)
		if err != nil {
			return nil, err
		}
		if err := v.mayAct(caster); err != nil {
			return nil, err
		}
		if err := s.mustActNow(ctx, c, caster); err != nil {
			return nil, err
		}
		if err := s.refuseInShape(ctx, c, caster); err != nil { // no spells in a beast form (MR-037)
			return nil, err
		}
		targs := make([]playdb.Combatant, len(targets))
		for i, t := range targets {
			if targs[i], err = findCombatant(cs, t.id, v); err != nil {
				return nil, err
			}
			if targs[i].Defeated {
				return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TARGET_DEFEATED, "a target is defeated")
			}
		}

		// The spell must be one the caster may cast now; the master has the last
		// word on the economy only.
		opts, err := s.optionsOf(ctx, c.tx, m.CampaignID, caster)
		if err != nil {
			return nil, err
		}
		cast, err := castableOf(opts, spellKey)
		if err != nil {
			return nil, err
		}
		if !cast.enabled {
			// A summoning spell that takes too long is refused with a reason the screen
			// can say ("Leva 1 hora: conjure fora do combate").
			if cast.reason.GetCode() == rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_CASTING_TIME_TOO_LONG {
				if long, err := s.roster.CombatSpell(ctx, c.tx, m.CampaignID, caster.CharacterID, spellKey, int(cast.level)); err == nil && long.Summon {
					return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_CASTING_TIME_TOO_LONG,
						"this spell takes too long to cast in a fight: cast it outside the combat")
				}
			}
			if err := castError(cast.reason, v.master); err != nil {
				return nil, err
			}
		}
		slot, err := slotOf(req.Msg.GetSlot(), cast)
		if err != nil {
			return nil, err
		}
		slotLevel := int(cast.level)
		if slot != nil {
			slotLevel = int(slot.Level)
		}
		sp, err := s.roster.CombatSpell(ctx, c.tx, m.CampaignID, caster.CharacterID, spellKey, slotLevel)
		if err != nil {
			return nil, err
		}

		// Who it touches.
		darts := dartsOf(sp, slotLevel)
		dartList := make([]int, len(targets))
		for i, t := range targets {
			dartList[i] = t.darts
		}
		if sp.Summon != (req.Msg.GetSummon() != nil) {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("summon is for a summoning spell, and a summoning spell needs it"))
		}
		known, err := c.sight.knownTerrain(ctx, c.tx, v) // what the player knows of the map, for the cover they are told
		if err != nil {
			return nil, err
		}
		terrain, err := s.terrainOf(ctx, c.tx, m.CampaignID, c.enc)
		if err != nil {
			return nil, err
		}
		if sp.Summon {
			if len(targs) > 0 {
				return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("a summoning spell takes no targets: the creatures appear next to the caster"))
			}
		} else if err := s.checkTargets(v, planOn(v, terrain, known), cs, sp, caster, targs, dartList, darts, slotLevel, isTheatre(c.enc)); err != nil {
			return nil, err
		}
		var summon *summoning
		if sp.Summon {
			if summon, err = s.prepareSummon(ctx, c, m, v, caster, spellKey, slotLevel, req.Msg.GetSummon(), in, rolled, sp.Concentration); err != nil {
				return nil, err
			}
		}
		if len(targs) == 0 && selfOnly(sp) {
			targs, targets = []playdb.Combatant{caster}, []target{{id: caster.ID}}
		}
		if sp.HP != nil && sp.HP.Kind == rules.SpellKindHPPool {
			if err := s.mustRollPool(ctx, c.tx, m, in, rolled); err != nil {
				return nil, err
			}
		}
		if sp.AttackType != "" {
			if in.pool {
				return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("pool_sum is for a spell that rolls a pool of dice"))
			}
			if !rolled {
				return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("set roll_in_app or d20_face for a spell attack"))
			}
			if !in.inApp && len(targs) != 1 {
				return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("d20_face fits a single target: use roll_in_app for several"))
			}
			if !v.master {
				if err := s.mustRollThisWay(ctx, c.tx, m, in); err != nil {
					return nil, err
				}
			}
		}

		// The cast spends the slot and the economy at once (MR-014).
		run, err := breakRun(ctx, c, caster)
		if err != nil {
			return nil, err
		}
		made = actionEvent{
			RunBefore: run, Round: c.enc.Round, Actor: caster.ID, Key: spellKey, CastID: uuid.New().String(), Slot: slot,
			ActionBefore: caster.ActionUsed, BonusBefore: caster.BonusActionUsed, ReactionBefore: caster.ReactionUsed, DashedBefore: caster.Dashed,
		}
		if slot != nil && caster.Kind == kindPlayer {
			slotVitals, err := s.spendSlot(ctx, c, caster.CharacterID, *slot, 1)
			if err != nil {
				return nil, err
			}
			vitals = append(vitals, slotVitals)
		}
		after := caster
		switch sp.Economy {
		case rules.EconomyBonusAction:
			after.BonusActionUsed = true
		default:
			after.ActionUsed = true
		}
		if err := c.q.SetCombatantEconomy(ctx, playdb.SetCombatantEconomyParams{
			ID: caster.ID, ActionUsed: after.ActionUsed, BonusActionUsed: after.BonusActionUsed, ReactionUsed: after.ReactionUsed, Dashed: after.Dashed,
		}); err != nil {
			return nil, fmt.Errorf("spend the economy: %w", err)
		}
		if sp.Concentration {
			made.Concentrate, made.ConcBefore = true, deref(caster.ConcentrationSpell)
			made.ConcEnded = made.ConcBefore
			// A new concentration spell ends the old one, and the creatures that lasted
			// only while it did (MR-037, RN-22).
			if made.Dismissed, err = s.endSummons(ctx, c, caster); err != nil {
				return nil, err
			}
			if err := c.q.SetCombatantConcentration(ctx, playdb.SetCombatantConcentrationParams{ID: caster.ID, ConcentrationSpell: &spellKey}); err != nil {
				return nil, fmt.Errorf("set the concentration: %w", err)
			}
		}
		if summon != nil {
			if err := s.joinSummon(ctx, c, caster, spellKey, sp.Concentration, summon, &made); err != nil {
				return nil, err
			}
		}

		// What it does to each target.
		c.castID = made.CastID
		hidden := caster.Hidden
		if sp.HP != nil {
			hits, changed, err := s.castHPSpell(ctx, c, sp, targs, in, &made)
			if err != nil {
				return nil, err
			}
			made.Hits = hits
			vitals = append(vitals, changed...)
			for _, t := range targs {
				hidden = hidden || t.Hidden
			}
		} else {
			for i, t := range targs {
				hit := castHit{Target: t.ID, Darts: clamp32(targets[i].darts, 0, 100)}
				cp, err := c.coverOf(ctx, v, terrain, known, caster, t, cs)
				if err != nil {
					return nil, err
				}
				if err := s.resolveOnTarget(ctx, c, m, sp, caster, t, slotLevel, in, cp, &hit); err != nil {
					return nil, err
				}
				made.Hits = append(made.Hits, hit)
				hidden = hidden || t.Hidden
			}
		}
		made.Secret = hidden
		if c.enc, err = c.q.TouchEncounter(ctx, c.enc.ID); err != nil {
			return nil, fmt.Errorf("touch the encounter: %w", err)
		}
		c.characterID = &caster.CharacterID
		return made, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "cast a spell", err)
	}
	if v, err = s.viewerAfter(ctx, m, res, v); err != nil { // a replay never ran the closure: the fog filter still holds
		return nil, s.dbError(ctx, "work out what the player sees", err)
	}
	ev, err := resultEvent(res, made)
	if err != nil {
		return nil, s.dbError(ctx, "read the spell", err)
	}
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, !ev.Secret)
		for _, vit := range vitals {
			s.publishVitals(m.CampaignID, vit)
		}
	})
	if err != nil {
		return nil, err
	}
	spell, err := s.castProto(ctx, res, ev, v)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.CastSpellResponse{Encounter: out, Cast: spell, SummonedCombatantIds: s.combatantsOfCreatures(ctx, res, ev.Created)}), nil
}

// checkTargets checks, for a player, who a spell may touch: how many targets
// (the master is not held to the number), that Magic Missile's darts add up,
// and that each target is on the map and in range (the master is never held to
// the range, and nobody is without a grid: RN-25), and that no target of a single-target spell has total cover (D4).
// It does not read the database.
func (s *Service) checkTargets(v combatViewer, terrain grid.Terrain, cs []playdb.Combatant, sp link.Spell, caster playdb.Combatant, targs []playdb.Combatant, darts []int, dartsTotal, slotLevel int, theatre bool) error {
	bad := func(msg string) error { return connect.NewError(connect.CodeInvalidArgument, errors.New(msg)) }
	switch {
	case selfOnly(sp):
		if slices.ContainsFunc(targs, func(t playdb.Combatant) bool { return t.ID != caster.ID }) {
			return bad("this spell reaches the caster alone: leave the targets out")
		}
	case dartsTotal > 0:
		sum := 0
		for _, d := range darts {
			if d < 1 {
				return bad("every target of Magic Missile needs at least one dart")
			}
			sum += d
		}
		if len(targs) == 0 || sum != dartsTotal {
			return bad(fmt.Sprintf("the darts of the targets must add up to %d", dartsTotal))
		}
	default:
		if slices.ContainsFunc(darts, func(d int) bool { return d != 0 }) {
			return bad("darts are only for Magic Missile")
		}
		if !sp.Area && len(targs) == 0 {
			return bad("this spell needs a target")
		}
		if limit := maxTargetsOf(sp, slotLevel); limit > 0 && len(targs) > limit && !v.master {
			return bad(fmt.Sprintf("this spell takes at most %d targets with this slot", limit))
		}
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
		if !placed(caster) || !placed(t) {
			return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_PLACED, "the caster and the target must be on the map")
		}
		if dist, _ := distanceFt(caster, t); dist > reach {
			return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TARGET_OUT_OF_REACH, "the target is beyond the spell's range",
				func(b *playv1.EncounterBlocked) { b.MissingFt = dist - reach })
		}
	}
	return nil
}

// resolveOnTarget does what the spell does to one target and fills the hit: a
// spell attack's roll, a saving throw's roll, and the pending damage or heal
// the cast opens. Anything else leaves the hit empty.
func (s *Service) resolveOnTarget(ctx context.Context, c *combatTx, m authz.Membership, sp link.Spell, caster, target playdb.Combatant, slotLevel int, in rollInput, cp coverPair, hit *castHit) error {
	cover := cp.real
	// The cover counts for a spell attack and a Dexterity save (D4).
	if sp.AttackType != "" || (sp.SaveAbility == "dex" && !cover.total() && !sp.IgnoresCover) {
		hit.Cover, hit.CoverSource, hit.CoverBonus = cover.key(), cover.sourceKey(), clamp32(cover.bonus(), 0, 5)
		hit.CoverRestricted, hit.CoverSeenBy = cp.restricted, cp.seenBy
	}
	switch {
	case sp.AttackType != "":
		return s.spellAttack(ctx, c, m, sp, caster, target, in, cover, hit)
	case sp.SaveAbility != "":
		return s.spellSave(ctx, c, m, sp, caster, target, cover, hit)
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

// spellAttack rolls a spell attack against a target, as RollAttack does, and
// opens its pending damage on a hit.
func (s *Service) spellAttack(ctx context.Context, c *combatTx, m authz.Membership, sp link.Spell, caster, target playdb.Combatant, in rollInput, cover coverView, hit *castHit) error {
	face, roll, err := s.d20(in, sp.ToHit)
	if err != nil {
		return err
	}
	sheet, err := s.sheetOf(ctx, c.tx, m.CampaignID, target)
	if err != nil {
		return err
	}
	targetAC := sheet.ArmorClass + int(target.AcBonus) + cover.bonus()
	result := combat.ResolveAttack(sp.ToHit, targetAC, face)
	hit.TargetAC = clamp32(targetAC, 0, math.MaxInt32)
	hit.D20, hit.Modifier, hit.Total, hit.Physical = clamp32(face, 1, 20), clamp32(sp.ToHit, math.MinInt32, math.MaxInt32), clamp32(result.Total, math.MinInt32, math.MaxInt32), roll.Physical
	hit.Outcome = outcomeMiss
	if !result.Hit {
		return nil
	}
	hit.Outcome = outcomeHit
	if result.Critical {
		hit.Outcome = outcomeCrit
	}
	if sp.Damage == nil {
		return nil
	}
	p, err := s.openHit(ctx, c, m.CampaignID, caster, target, sp.Key, *sp.Damage, result.Critical, result.Total, targetAC)
	if err != nil {
		return err
	}
	hit.Pending = p.ID
	return nil
}

// spellSave has the server roll a target's saving throw against the spell and
// opens the damage the save leaves: all of it for a target that failed, half
// (rounded down) for one that saved when the spell halves, none when it avoids.
// The d20 is always rolled by the app (RN-18 is for the table's own dice).
func (s *Service) spellSave(ctx context.Context, c *combatTx, m authz.Membership, sp link.Spell, caster, target playdb.Combatant, cover coverView, hit *castHit) error {
	save, err := s.saveOf(ctx, c.tx, m.CampaignID, target, sp.SaveAbility)
	if err != nil {
		return err
	}
	// Cover adds to a Dexterity save (SRD): half +2, three-quarters +5. The bonus
	// the save shows already has it.
	if sp.SaveAbility == "dex" && !cover.total() && !sp.IgnoresCover { // Chama Sagrada: no benefit from cover (SRD)
		save.Bonus += cover.bonus()
	}
	face, roll, err := s.d20(rollInput{inApp: true}, save.Bonus)
	if err != nil {
		return err
	}
	saved := combat.SaveSucceeded(roll.Total, sp.SaveDC)
	hit.Save = &saveRoll{
		D20: clamp32(face, 1, 20), Bonus: clamp32(save.Bonus, math.MinInt32, math.MaxInt32), Total: clamp32(roll.Total, math.MinInt32, math.MaxInt32),
		DC: clamp32(sp.SaveDC, 0, math.MaxInt32), Saved: saved, Unknown: !save.Known,
	}
	if sp.Damage == nil || (saved && sp.SaveOnSuccess != "half" && sp.SaveOnSuccess != "other") {
		return nil
	}
	return s.openSpellPending(ctx, c, sp, *sp.Damage, caster, target, false, saved && sp.SaveOnSuccess == "half", hit)
}

// openDarts opens Magic Missile's damage for a target: its darts, each one
// 1d4 + 1 (the spell's damage at its own level divided by its three darts).
func (s *Service) openDarts(ctx context.Context, c *combatTx, sp link.Spell, caster, target playdb.Combatant, hit *castHit) error {
	if sp.Damage == nil {
		return nil
	}
	// The damage at the cast's slot level is the whole volley (3d4 + 3 at the 1st
	// level, 6d4 + 6 at the 4th): one dart is always 1d4 + 1, whatever the level.
	darts := max(sp.Damage.Count, 1)
	perDart := link.Dice{Count: 1, Sides: sp.Damage.Sides, Bonus: sp.Damage.Bonus / darts, DamageType: sp.Damage.DamageType}
	n := int(hit.Darts)
	// A dart is its own roll: n darts are n dice and n times the bonus. The cast
	// has no cast id for them: each target's damage is its own roll.
	dmg := link.Dice{Count: perDart.Count * n, Sides: perDart.Sides, Bonus: perDart.Bonus * n, DamageType: perDart.DamageType}
	p, err := c.q.InsertPendingDamage(ctx, playdb.InsertPendingDamageParams{
		EncounterID: c.enc.ID, AttackerID: &caster.ID, TargetID: target.ID, AttackKey: sp.Key, Status: pendingAwaitingRoll,
		DiceCount: clamp32(dmg.Count, 0, 100), DiceSides: clamp32(dmg.Sides, 0, 100), DiceBonus: clamp32(dmg.Bonus, -1000, 1000), DamageType: dmg.DamageType,
		CreatedAt: c.now,
	})
	if err != nil {
		return fmt.Errorf("open the pending damage: %w", err)
	}
	hit.Pending = p.ID
	return nil
}

// openSpellPending opens a damage or a heal of a spell for a target, in the
// cast's group (the pending damages of one cast share an id: an area spell's
// damage is one roll).
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
	hit.Pending = p.ID
	return nil
}
