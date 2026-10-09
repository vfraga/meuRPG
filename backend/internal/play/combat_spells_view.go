package play

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// How a cast is shown (RN-20): the caster's player and the master get the d20 of
// a spell attack and the DC; a target's saving throw dice go to the master and
// the target's own player only, everyone else gets the outcome as a word (an
// NPC's dice never reach a player); whether an NPC's save bonus is known is the
// master's.

func slotProto(s *slotRef) *playv1.SpellSlot {
	if s == nil {
		return nil
	}
	return &playv1.SpellSlot{Level: s.Level, Pact: s.Pact}
}

// saveView is a target's saving throw as the viewer may see it.
func saveView(sr *saveRoll, v combatViewer, caster, target playdb.Combatant) *playv1.SaveResult {
	if sr == nil {
		return nil
	}
	out := &playv1.SaveResult{Outcome: playv1.SaveOutcome_SAVE_OUTCOME_FAILED}
	if sr.Saved {
		out.Outcome = playv1.SaveOutcome_SAVE_OUTCOME_SAVED
	}
	if v.master || v.owns(target) {
		out.Roll = diceRoll(1, 20, []int32{sr.D20}, sr.Bonus, sr.Total, false)
	}
	if v.master {
		out.BonusKnown = !sr.Unknown
	}
	if v.master || v.owns(caster) {
		out.Dc = sr.DC
	}
	return out
}

// attackRollView is a spell attack's d20 for the master and the caster's player.
func attackRollView(h castHit, v combatViewer, caster playdb.Combatant) *playv1.DiceRoll {
	if h.Outcome == "" || (!v.master && !v.owns(caster)) {
		return nil
	}
	return diceRoll(1, 20, []int32{h.D20}, h.Modifier, h.Total, h.Physical)
}

var effectKindToProto = map[string]playv1.SpellEffectKind{
	rules.SpellKindHPPool:      playv1.SpellEffectKind_SPELL_EFFECT_KIND_POOL,
	rules.SpellKindHPThreshold: playv1.SpellEffectKind_SPELL_EFFECT_KIND_THRESHOLD,
	rules.SpellKindZeroHP:      playv1.SpellEffectKind_SPELL_EFFECT_KIND_ZERO_HP,
	rules.SpellKindFlatHeal:    playv1.SpellEffectKind_SPELL_EFFECT_KIND_FLAT_HEAL,
	rules.SpellKindTempHP:      playv1.SpellEffectKind_SPELL_EFFECT_KIND_TEMP_HP,
	rules.SpellKindMaxHP:       playv1.SpellEffectKind_SPELL_EFFECT_KIND_MAX_HP,
}

var effectReasonToProto = map[string]playv1.SpellEffectReason{
	combat.PoolTooHigh: playv1.SpellEffectReason_SPELL_EFFECT_REASON_ABOVE_POOL,
	combat.PoolSkipped: playv1.SpellEffectReason_SPELL_EFFECT_REASON_SKIPPED,
	fxAboveLimit:       playv1.SpellEffectReason_SPELL_EFFECT_REASON_ABOVE_LIMIT,
	fxNotAtZero:        playv1.SpellEffectReason_SPELL_EFFECT_REASON_NOT_AT_ZERO,
}

var effectGainToProto = map[string]playv1.SpellEffectGain{
	gainMaximum:   playv1.SpellEffectGain_SPELL_EFFECT_GAIN_MAXIMUM,
	gainTemporary: playv1.SpellEffectGain_SPELL_EFFECT_GAIN_TEMPORARY,
	gainCurrent:   playv1.SpellEffectGain_SPELL_EFFECT_GAIN_CURRENT,
}

// effectView is what a spell that reads hit points did to a target, as the
// viewer may see it (RN-20): everyone gets the outcome as a word; the master
// alone gets why, the target's hit points and the pool's arithmetic (an enemy's
// hit points never reach a player, and the order of the pool would tell who has
// fewer); a heal's amount, capped at the target's maximum, goes only to the
// master and the target's own player, as for any heal (slice 6.4b): on an NPC,
// the amount would tell the caster how many hit points it lacked.
func effectView(h castHit, v combatViewer, target playdb.Combatant) *playv1.SpellEffectResult {
	if h.Fx == "" {
		return nil
	}
	out := &playv1.SpellEffectResult{Outcome: playv1.SpellEffectOutcome_SPELL_EFFECT_OUTCOME_NOT_AFFECTED}
	if h.Fx == fxAffected {
		out.Outcome = playv1.SpellEffectOutcome_SPELL_EFFECT_OUTCOME_AFFECTED
	}
	if v.master {
		out.Reason = effectReasonToProto[h.FxReason]
		out.HitPointsBefore = &h.HPBefore
		if h.Order > 0 {
			out.PoolLeft, out.PoolOrder = &h.Left, &h.Order
		}
	}
	out.Gain = effectGainToProto[h.Gain]
	if h.Healed != nil && (v.master || v.owns(target)) {
		out.Healed = h.Healed
	}
	return out
}

// effectHeader fills what a spell that reads hit points says about the cast as a
// whole: its kind and condition for everyone, the pool roll for the master and
// the caster's player, the limit for the master. It returns them as the fields
// SpellCast and CombatLogSpell share.
func effectHeader(ev actionEvent, v combatViewer, caster playdb.Combatant) (kind playv1.SpellEffectKind, pool *playv1.DiceRoll, condition string, limit *int32) {
	kind = effectKindToProto[ev.FxKind]
	if kind == playv1.SpellEffectKind_SPELL_EFFECT_KIND_UNSPECIFIED {
		return kind, nil, "", nil
	}
	if (kind == playv1.SpellEffectKind_SPELL_EFFECT_KIND_POOL || kind == playv1.SpellEffectKind_SPELL_EFFECT_KIND_TEMP_HP) && (v.master || v.owns(caster)) {
		pool = diceRoll(ev.DiceCount, ev.DiceSides, ev.Faces, ev.Modifier, ev.Total, ev.Physical)
	}
	if v.master && ev.FxLimit > 0 {
		limit = &ev.FxLimit
	}
	return kind, pool, ev.FxCondition, limit
}

// castProto builds the SpellCast a cast event tells, for the viewer, who is the
// caster's player or the master: the pending damages of the cast, with their
// current state. A target the viewer does not see is left out.
func (s *Service) castProto(ctx context.Context, res combatResult, ev actionEvent, v combatViewer) (*playv1.SpellCast, error) {
	cs, err := s.queries.ListCombatants(ctx, res.encounterID)
	if err != nil {
		return nil, s.dbError(ctx, "list the combatants", err)
	}
	byID := make(map[string]playdb.Combatant, len(cs))
	for _, c := range cs {
		byID[c.ID] = c
	}
	caster := byID[ev.Actor]
	out := &playv1.SpellCast{
		CastId: ev.CastID, SpellKey: ev.Key, Slot: slotProto(ev.Slot), Concentrating: ev.Concentrate,
		ConcentrationEndedSpellKey: ev.ConcEnded,
	}
	out.EffectKind, out.PoolRoll, out.EffectConditionKey, out.EffectThreshold = effectHeader(ev, v, caster)
	for _, h := range ev.Hits {
		target := byID[h.Target]
		if !v.canSee(target) || (!v.master && h.HiddenAtCast) { // a retry is built from the combat as it stands: a target hidden since is not told, and one that was hidden when the area hit it never is
			continue
		}
		r := &playv1.SpellTargetResult{
			CombatantId: h.Target, Darts: h.Darts, Outcome: outcomeToProto[h.Outcome],
			AttackRoll: attackRollView(h, v, caster), Save: saveView(h.Save, v, caster, target),
			Effect: effectView(h, v, target),
		}
		coverKey, coverSource := h.coverFor(v, ev.CoverUsers)
		r.Cover, r.CoverSource = coverDegreeProto(coverKey), coverSourceProto(coverSource)
		if h.Pending != "" && (v.master || v.owns(caster)) {
			for i, id := range append([]string{h.Pending}, h.More...) {
				p, err := s.queries.GetPendingDamage(ctx, playdb.GetPendingDamageParams{EncounterID: res.encounterID, ID: id})
				switch {
				case errors.Is(err, pgx.ErrNoRows): // an undo took it away meanwhile
				case err != nil:
					return nil, s.dbError(ctx, "read the pending damage", err)
				default:
					if i == 0 {
						r.PendingDamageId = id
					} else {
						r.MorePendingDamageIds = append(r.MorePendingDamageIds, id)
					}
					out.PendingDamages = append(out.PendingDamages, pendingProto(p, cs))
				}
			}
		}
		out.Targets = append(out.Targets, r)
	}
	return out, nil
}
