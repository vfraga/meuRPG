package play

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// Where an effect lives (RN-22). An effect belongs to the character it is on: in a running
// combat its record is a row of combatant_states (the clock in rounds, read by the combatant of
// the character); out of a combat it is a row of character_effects (the clock in game time).
// The record changes home when a combat takes the character in or lets it go, whole and with
// the same id in one transaction, so there is never a copy of it. A round is 6 seconds
// (SRD 5.1, "The Order of Combat"), in a combat and out of it.

// backedByCharacter says the combatant is a character (a player's, or an NPC with a sheet):
// its effects outlive the combat. A monster and a creature are the combat's alone.
func backedByCharacter(a playdb.Combatant) bool {
	return a.CharacterID != "" && !isCreature(a) && (a.Kind == kindPlayer || a.Kind == kindNPC)
}

// castCharacterEffects puts the effect of a spell that lasts on the characters it was cast
// on, outside a combat. The duration is the spell's, in game time; the caster's concentration
// holds it. A spell that asks a saving throw has none here: the master adds it where it took hold.
// So the rider of an instantaneous spell never comes here either (Zombaria Viciosa asks a saving
// throw, and its clock is a turn, which only a combat has): cast outside a combat, it leaves nothing.
func (s *Service) castCharacterEffects(ctx context.Context, c *combatTx, plan castPlan) error {
	content, err := s.contentOf(ctx, c)
	if err != nil {
		return err
	}
	def, ok := content.CombatSpellEffect(plan.spell)
	if !ok || def.Applies != "all" || len(s.effectTargetsAll(plan)) == 0 { // a spell on the caster alone has no targets listed
		return nil
	}
	seconds, kind := castClockOf(plan, def)
	caster := plan.g.caster.ID
	opts, err := s.castOptsOf(ctx, c, def, plan.ability, caster)
	if err != nil {
		return err
	}
	var touched []string
	for _, t := range s.effectTargetsAll(plan) {
		if !t.Player {
			continue // an NPC has no vitals and no combat outside a fight: the master narrates
		}
		ok, err := s.castOnCharacter(ctx, c, plan, def, t.ID, castClock{seconds: seconds, kind: kind}, opts)
		if err != nil {
			return err
		}
		if ok {
			touched = append(touched, t.ID)
		}
	}
	return s.syncArmorBase(ctx, c, touched...)
}

// castOnCharacter puts the effect of a cast on one character and gives its temporary hit points.
// It says whether the effect took hold (a base armor class takes none on a creature that wears armor).
func (s *Service) castOnCharacter(ctx context.Context, c *combatTx, plan castPlan, def *rules.EffectDef, characterID string, clock castClock, opts castOpts) (bool, error) {
	mods, ok, err := s.modifiersFor(ctx, c, def, characterID, opts)
	if err != nil || !ok {
		return false, err
	}
	prior, err := s.priorCharacterClaim(ctx, c, characterID, plan)
	if err != nil {
		return false, err
	}
	row, err := s.putCharacterEffect(ctx, c, plan, def, characterID, plan.g.caster.ID, mods, clock)
	if err != nil {
		return false, err
	}
	return true, s.giveCharacterTempHP(ctx, c, row, mods, prior)
}

// castClock is the clock a spell cast outside a combat gives its effect: game time, or none.
type castClock struct {
	seconds *int32
	kind    string
}

// castClockOf is the duration of the effect of a cast: the spell's time when it has one, the
// concentration, or until the master ends it.
func castClockOf(plan castPlan, def *rules.EffectDef) (*int32, string) {
	switch {
	case plan.osp.DurationSeconds > 0:
		return new(clamp32(plan.osp.DurationSeconds, 1, 1<<30)), rules.EffectDurationRounds
	case def.Concentration:
		return nil, rules.EffectDurationConcentration
	}
	return nil, rules.EffectDurationUntilDismissed
}

// putCharacterEffect writes the effect of a spell on a character, replacing the same spell on it.
func (s *Service) putCharacterEffect(ctx context.Context, c *combatTx, plan castPlan, def *rules.EffectDef, characterID, caster string, mods []rules.EffectModifier, clock castClock) (playdb.CharacterEffect, error) {
	body, err := json.Marshal(mods)
	if err != nil {
		return playdb.CharacterEffect{}, fmt.Errorf("encode the modifiers: %w", err)
	}
	// The same spell on the same target does not stack (SRD 5.1, Combining Magical Effects).
	if _, err := c.q.DeleteCharacterEffectsOfSpell(ctx, playdb.DeleteCharacterEffectsOfSpellParams{CharacterID: characterID, SourceKey: plan.spell, GroupID: plan.row.ID}); err != nil {
		return playdb.CharacterEffect{}, fmt.Errorf("replace the same spell: %w", err)
	}
	p := playdb.InsertCharacterEffectParams{
		CampaignID: c.session.CampaignID, CharacterID: characterID, SourceCharacterID: &caster, GroupID: plan.row.ID, SourceKey: plan.spell,
		SourceKind: "spell", Concentration: def.Concentration, ConditionKeys: append([]string{}, def.Conditions...), Modifiers: body,
		DurationKind: clock.kind, SecondsLeft: clock.seconds, PlayerVisible: true, Audience: "all", CreatedAt: c.now,
	}
	if def.Visibility == rules.EffectVisibilityOwner {
		p.Audience = audienceOwner
	}
	row, err := c.q.InsertCharacterEffect(ctx, p)
	if err != nil {
		return playdb.CharacterEffect{}, fmt.Errorf("put the effect on the character: %w", err)
	}
	return row, nil
}

// endCharacterEffectsOfCast takes away the effects a cast left on characters out of a combat
// (the cast ended, or its concentration did).
func (s *Service) endCharacterEffectsOfCast(ctx context.Context, c *combatTx, castID string) error {
	gone, err := c.q.DeleteCharacterEffectsOfGroup(ctx, playdb.DeleteCharacterEffectsOfGroupParams{CampaignID: c.session.CampaignID, GroupID: castID})
	if err != nil {
		return fmt.Errorf("end the effects of a cast: %w", err)
	}
	var touched []string
	for _, r := range gone {
		touched = append(touched, r.CharacterID)
	}
	if err := s.afterCharacterEffectsGone(ctx, c, characterEffects(gone)); err != nil {
		return err
	}
	// The cast ended while a fight ran: what it left in the combat goes too.
	inFight, err := c.q.ListCombatEffectsOfGroup(ctx, &castID)
	if err != nil {
		return fmt.Errorf("list the effects of a cast in the combats: %w", err)
	}
	for _, st := range inFight {
		if err := c.q.DeleteLastingEffect(ctx, st.ID); err != nil {
			return fmt.Errorf("end an effect of a cast in a combat: %w", err)
		}
		if st.CharacterID != "" {
			touched = append(touched, st.CharacterID)
			if err := c.q.ClearMageArmorACOfCharacter(ctx, st.CharacterID); err != nil {
				return fmt.Errorf("take the base armor class off the combatants: %w", err)
			}
		}
	}
	return s.syncArmorBase(ctx, c, touched...)
}

// advanceGameTime moves game time on by seconds (the master's: a cast that took time): what
// had less time left ends, with the cast that held it.
func (s *Service) advanceGameTime(ctx context.Context, c *combatTx, seconds int32) error {
	if seconds <= 0 {
		return nil
	}
	if err := s.dieAdvance(ctx, c, seconds); err != nil {
		return err
	}
	rows, err := c.q.AdvanceCharacterEffects(ctx, playdb.AdvanceCharacterEffectsParams{CampaignID: c.session.CampaignID, Column2: seconds})
	if err != nil {
		return fmt.Errorf("move game time on: %w", err)
	}
	var touched []string
	var ended []playdb.CharacterEffect
	for _, r := range rows {
		if r.SecondsLeft == nil || *r.SecondsLeft > 0 {
			continue
		}
		if err := c.q.DeleteCharacterEffect(ctx, r.ID); err != nil {
			return fmt.Errorf("end an effect whose time ran out: %w", err)
		}
		touched = append(touched, r.CharacterID)
		ended = append(ended, playdb.CharacterEffect(r))
	}
	if err := s.afterCharacterEffectsGone(ctx, c, ended); err != nil {
		return err
	}
	return s.syncArmorBase(ctx, c, touched...)
}

// endLongRestEffects ends what lasts until a long rest on the characters of the table: the
// effects out of a combat and the ones a combat in setup already holds.
func (s *Service) endLongRestEffects(ctx context.Context, c *combatTx) error {
	gone, err := c.q.DeleteLongRestEffectsOfCampaign(ctx, c.session.CampaignID)
	if err != nil {
		return fmt.Errorf("end the effects that last until a long rest: %w", err)
	}
	var touched []string
	for _, r := range gone {
		touched = append(touched, r.CharacterID)
	}
	if err := s.afterCharacterEffectsGone(ctx, c, characterEffects(gone)); err != nil {
		return err
	}
	if enc, err := c.q.GetOpenEncounter(ctx, c.session.ID); err == nil {
		rows, err := c.q.ListLastingEffects(ctx, enc.ID)
		if err != nil {
			return fmt.Errorf("list the effects in the combat: %w", err)
		}
		for _, st := range rows {
			if conv.OrZero(st.DurationKind) != rules.EffectDurationLongRest {
				continue
			}
			if err := c.q.DeleteLastingEffect(ctx, st.ID); err != nil {
				return fmt.Errorf("end an effect in the combat: %w", err)
			}
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("find the open encounter: %w", err)
	}
	return s.syncArmorBase(ctx, c, touched...)
}

// effectsToCombat runs when characters join a combat: their effects come into it with the
// time they have left, in whole rounds (a partial round does not count: 59 seconds are 9
// rounds), counted from the start of the first round. The record is the same: it only moves.
func (s *Service) effectsToCombat(ctx context.Context, c *combatTx, all, added []playdb.Combatant) error {
	if err := s.dieToCombat(ctx, c, all, added); err != nil {
		return err
	}
	var ids []string
	for _, a := range added {
		if backedByCharacter(a) {
			ids = append(ids, a.CharacterID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	rows, err := c.q.ListCharacterEffects(ctx, ids)
	if err != nil {
		return fmt.Errorf("list the characters' effects: %w", err)
	}
	var touched []string
	for _, r := range rows {
		i := slices.IndexFunc(added, func(a playdb.Combatant) bool { return a.CharacterID == r.CharacterID && backedByCharacter(a) })
		if i < 0 {
			continue
		}
		if err := s.bringEffectIn(ctx, c, all, added[i], r); err != nil {
			return err
		}
		touched = append(touched, added[i].ID)
	}
	// Every character that joined is worked out again: its exhaustion comes from its vitals.
	for _, a := range added {
		if backedByCharacter(a) {
			touched = append(touched, a.ID)
		}
	}
	slices.Sort(touched)
	return s.refreshCombatants(ctx, c, slices.Compact(touched)...)
}

// bringEffectIn moves the record of an effect on a character into the combat, on its combatant.
func (s *Service) bringEffectIn(ctx context.Context, c *combatTx, all []playdb.Combatant, target playdb.Combatant, r playdb.CharacterEffect) error {
	var casterCombatant *playdb.Combatant
	if r.SourceCharacterID != nil {
		if j := slices.IndexFunc(all, func(a playdb.Combatant) bool { return a.CharacterID == *r.SourceCharacterID && backedByCharacter(a) }); j >= 0 {
			casterCombatant = &all[j]
		}
	}
	p := combatEffectParamsOf(c, r, target, endsInCombat(c, all, r, casterCombatant, target))
	if casterCombatant != nil {
		p.SourceID = &casterCombatant.ID
	}
	if _, err := c.q.InsertLastingEffect(ctx, p); err != nil {
		return fmt.Errorf("bring the effect into the combat: %w", err)
	}
	if err := c.q.DeleteCharacterEffect(ctx, r.ID); err != nil {
		return fmt.Errorf("take the effect off the character: %w", err)
	}
	return nil
}

// effectEndAt is where an effect that came into a combat ends: the round, whose turn and the phase.
type effectEndAt struct {
	round *int32
	who   *string
	phase *string
}

// endsInCombat works out where the time left of an effect on a character ends in the combat.
func endsInCombat(c *combatTx, all []playdb.Combatant, r playdb.CharacterEffect, caster *playdb.Combatant, target playdb.Combatant) effectEndAt {
	dur := durationSpec{Kind: r.DurationKind}
	if r.SecondsLeft != nil {
		dur.Rounds = roundsOfSeconds(*r.SecondsLeft)
	}
	round, who, phase := endsOf(c.enc, all, dur, caster, target)
	if r.DurationKind == rules.EffectDurationRounds && r.SecondsLeft == nil {
		return effectEndAt{}
	}
	return effectEndAt{round, who, phase}
}

// combatEffectParamsOf is the record of an effect of a character as a row of the combat.
func combatEffectParamsOf(c *combatTx, r playdb.CharacterEffect, target playdb.Combatant, end effectEndAt) playdb.InsertLastingEffectParams {
	return playdb.InsertLastingEffectParams{
		ID: &r.ID, EncounterID: c.enc.ID, CombatantID: target.ID, StartedRound: max(c.enc.Round, 1), CreatedAt: r.CreatedAt,
		GroupID: &r.GroupID, SourceKey: &r.SourceKey, SourceKind: &r.SourceKind, Concentration: r.Concentration,
		ConditionKeys: r.ConditionKeys, Modifiers: r.Modifiers, DurationKind: &r.DurationKind,
		PlayerVisible: r.PlayerVisible, Audience: r.Audience, EndsRound: end.round, EndsCombatantID: end.who, EndsPhase: end.phase,
		EndSaveAbility: r.EndSaveAbility, StartSaveAbility: r.StartSaveAbility, SaveDc: r.SaveDc, OnFailEffect: r.OnFailEffect,
		FollowsKey: r.FollowsKey, TriggerDice: r.TriggerDice, TriggerDamageType: r.TriggerDamageType, TriggerMaxTriggers: r.TriggerMaxTriggers,
		PlayerLabel: r.PlayerLabel,
	}
}

// effectsToCharacters runs when a combat ends, or when a character leaves it: the effects on
// the characters go back to game time with the rounds they had left, 6 seconds each, and keep
// running. One that was to end at a turn that never comes (until the end of a turn, until the
// start of a turn) ends with the combat. only limits it to those combatants (a leaver).
func (s *Service) effectsToCharacters(ctx context.Context, c *combatTx, cs, only []playdb.Combatant) error {
	if err := s.dieToCharacters(ctx, c, cs, only); err != nil {
		return err
	}
	return s.effectRowsToCharacters(ctx, c, cs, only)
}

// effectRowsToCharacters is the effects half of effectsToCharacters: the rows of the combatants
// backed by a character (only limits it to some) go back to game time. A character taken out of
// a combat in setup takes this path without its die, which stays with the inspiration rules.
func (s *Service) effectRowsToCharacters(ctx context.Context, c *combatTx, cs, only []playdb.Combatant) error {
	rows, err := c.q.ListLastingEffects(ctx, c.enc.ID)
	if err != nil {
		return fmt.Errorf("list the effects: %w", err)
	}
	for _, st := range rows {
		i := slices.IndexFunc(cs, func(a playdb.Combatant) bool { return a.ID == st.CombatantID })
		if i < 0 || !backedByCharacter(cs[i]) || (only != nil && !slices.ContainsFunc(only, func(a playdb.Combatant) bool { return a.ID == st.CombatantID })) {
			continue
		}
		if err := s.giveEffectBack(ctx, c, cs, cs[i], st); err != nil {
			return err
		}
	}
	return nil
}

// giveEffectBack moves the record of an effect on a combatant back to its character, with the
// rounds left as game time; it drops the ones with nothing to give back.
func (s *Service) giveEffectBack(ctx context.Context, c *combatTx, cs []playdb.Combatant, who playdb.Combatant, st playdb.CombatantState) error {
	// The casting that held it ended while the fight ran: nothing is left to give back.
	if cast, err := c.q.GetSpellCast(ctx, playdb.GetSpellCastParams{ID: conv.OrZero(st.GroupID), CampaignID: c.session.CampaignID}); err == nil && cast.Status != castActive {
		if err := c.q.DeleteLastingEffect(ctx, st.ID); err != nil {
			return fmt.Errorf("end the effect of an ended cast: %w", err)
		}
		return nil
	}
	kind := conv.OrZero(st.DurationKind)
	if kind == rules.EffectDurationUntilStartOfTurnOf || kind == rules.EffectDurationUntilEndOfTurnOf {
		return nil // its turn never comes: it ends with the combat
	}
	var seconds *int32
	if st.EndsRound != nil {
		left := max(*st.EndsRound-max(c.enc.Round, 1), 0)
		if left == 0 {
			return nil
		}
		seconds = new(left * rules.SecondsPerRound)
	}
	p := characterEffectParamsOf(c, st, who, kind, seconds)
	if st.SourceID != nil {
		if j := slices.IndexFunc(cs, func(a playdb.Combatant) bool { return a.ID == *st.SourceID }); j >= 0 && backedByCharacter(cs[j]) {
			p.SourceCharacterID = &cs[j].CharacterID
		}
	}
	if _, err := c.q.InsertCharacterEffect(ctx, p); err != nil {
		return fmt.Errorf("give the effect back to the character: %w", err)
	}
	if err := c.q.DeleteLastingEffect(ctx, st.ID); err != nil {
		return fmt.Errorf("take the effect off the combatant: %w", err)
	}
	return nil
}

// characterEffectParamsOf is the record of an effect of a combatant as a row of its character.
func characterEffectParamsOf(c *combatTx, st playdb.CombatantState, who playdb.Combatant, kind string, seconds *int32) playdb.InsertCharacterEffectParams {
	return playdb.InsertCharacterEffectParams{
		ID: &st.ID, CampaignID: c.session.CampaignID, CharacterID: who.CharacterID, GroupID: conv.OrZero(st.GroupID), SourceKey: conv.OrZero(st.SourceKey),
		SourceKind: conv.OrZero(st.SourceKind), Concentration: st.Concentration, ConditionKeys: st.ConditionKeys, Modifiers: st.Modifiers,
		DurationKind: kind, SecondsLeft: seconds, EndSaveAbility: st.EndSaveAbility, StartSaveAbility: st.StartSaveAbility, SaveDc: st.SaveDc,
		OnFailEffect: st.OnFailEffect, FollowsKey: st.FollowsKey, TriggerDice: st.TriggerDice, TriggerDamageType: st.TriggerDamageType,
		TriggerMaxTriggers: st.TriggerMaxTriggers, TriggersFired: st.TriggersFired, PlayerVisible: st.PlayerVisible, Audience: st.Audience,
		PlayerLabel: st.PlayerLabel, CreatedAt: st.CreatedAt,
	}
}

// giveCharacterTempHP gives a character the temporary hit points of a temp_hp modifier of a spell cast outside a
// combat. They do not stack: it keeps the larger of what it had and what the effect gives (SRD 5.1,
// "Temporary Hit Points"). As in a combat (giveEffectTempHP), the row keeps the pool it set, or no temp_hp
// when it raised nothing, and takes the claim off the character's other effects; they go when the row does
// (afterCharacterEffectsGone).
func (s *Service) giveCharacterTempHP(ctx context.Context, c *combatTx, row playdb.CharacterEffect, mods []rules.EffectModifier, prior int) error {
	amount := combat.TempHP(mods)
	if amount <= 0 {
		return nil
	}
	v, err := s.vitals.GetVitalsTx(ctx, c.tx, c.session.CampaignID, row.CharacterID)
	if connect.CodeOf(err) == connect.CodeNotFound { // no vitals: no hit points to give, and nothing to take back
		return c.q.SetCharacterEffectModifiers(ctx, playdb.SetCharacterEffectModifiersParams{ID: row.ID, Modifiers: claimedTempHP(mods, 0)})
	}
	if err != nil {
		return err
	}
	if int(v.GetHitPointsTemporary()) >= amount {
		amount = prior // a recast keeps the claim of the cast it replaced
	} else if err := s.raiseCharacterTempHP(ctx, c, row, amount); err != nil {
		return err
	}
	return c.q.SetCharacterEffectModifiers(ctx, playdb.SetCharacterEffectModifiersParams{ID: row.ID, Modifiers: claimedTempHP(mods, amount)})
}

// priorCharacterClaim is the temp_hp claim of the same spell's effect another casting left on the character,
// which the recast replaces.
func (s *Service) priorCharacterClaim(ctx context.Context, c *combatTx, characterID string, plan castPlan) (int, error) {
	all, err := c.q.ListCharacterEffects(ctx, []string{characterID})
	if err != nil {
		return 0, fmt.Errorf("list the effects: %w", err)
	}
	n := 0
	for _, o := range all {
		if o.SourceKey == plan.spell && o.GroupID != plan.row.ID {
			n = max(n, combat.TempHPClaim(modifiersOf(o.Modifiers)))
		}
	}
	return n, nil
}

func (s *Service) raiseCharacterTempHP(ctx context.Context, c *combatTx, row playdb.CharacterEffect, amount int) error {
	temp := clamp32(amount, 0, maxTempHitPoints)
	_, got, err := s.vitalsOf(ctx, c, row.CharacterID, &playv1.AdjustCharacterVitalsRequest{HitPointsTemporary: &temp})
	if err != nil {
		return err
	}
	if got != nil {
		c.told = append(c.told, got)
	}
	others, err := c.q.ListCharacterEffects(ctx, []string{row.CharacterID})
	if err != nil {
		return fmt.Errorf("list the effects: %w", err)
	}
	for _, o := range others {
		if o.ID == row.ID || combat.TempHPClaim(modifiersOf(o.Modifiers)) == 0 {
			continue
		}
		if err := c.q.SetCharacterEffectModifiers(ctx, playdb.SetCharacterEffectModifiersParams{ID: o.ID, Modifiers: claimedTempHP(modifiersOf(o.Modifiers), 0)}); err != nil {
			return fmt.Errorf("drop the claim of an older effect: %w", err)
		}
	}
	return nil
}

// afterCharacterEffectsGone does what follows effects that ended on characters out of a combat:
// the temporary hit points a spell gave go with it (SRD 5.1, Heroism). Those of temp_hp go only if the character
// has no more than the effect gave.
func (s *Service) afterCharacterEffectsGone(ctx context.Context, c *combatTx, rows []playdb.CharacterEffect) error {
	none := int32(0)
	for _, r := range rows {
		mods := modifiersOf(r.Modifiers)
		turn := slices.ContainsFunc(mods, func(m rules.EffectModifier) bool { return m.Kind == rules.ModifierTurnTempHP })
		given := combat.TempHPClaim(mods)
		if !turn && given == 0 {
			continue
		}
		v, err := s.vitals.GetVitalsTx(ctx, c.tx, c.session.CampaignID, r.CharacterID)
		if err != nil || v.GetHitPointsTemporary() == 0 || (!turn && int(v.GetHitPointsTemporary()) > given) {
			continue
		}
		if _, got, err := s.vitalsOf(ctx, c, r.CharacterID, &playv1.AdjustCharacterVitalsRequest{HitPointsTemporary: &none}); err != nil {
			return err
		} else if got != nil {
			c.told = append(c.told, got)
		}
	}
	return nil
}
