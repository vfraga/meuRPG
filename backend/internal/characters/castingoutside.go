package characters

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/uid"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// Casting outside a combat (SRD 5.1, "Spellcasting"): which spells a character can
// cast with no turn, how each is cast and what the server does with it. The rules
// are package rules's and rules/combat's (OutsideSpells, SpellStanding, CastMinutes);
// this file reads the sheet and the slots and copies the answer into the plain types
// of package link, like the combat's reads (combatspells.go).

// outsideCaster reads a living character that may cast outside a combat: its derived
// sheet, the slots it spent (a player's; an NPC keeps none) and the content.
func (s *Service) outsideCaster(ctx context.Context, tx pgx.Tx, campaignID, characterID string) (kind string, d rules.Derived, u combat.Usage, content *rules.Content, err error) {
	kind, d, content, err = s.fighter(ctx, tx, campaignID, characterID)
	if err != nil {
		return "", rules.Derived{}, combat.Usage{}, nil, err
	}
	if kind == kindPlayer {
		v, err := s.getVitals(ctx, tx, campaignID, characterID, false)
		if err != nil {
			return "", rules.Derived{}, combat.Usage{}, nil, err
		}
		u = usageOf(v)
	}
	return kind, d, u, content, nil
}

// CastingOptions implements play.CombatRoster: the spells the character can cast
// outside a combat, each with its slots, its times and what the server applies.
// `not_found` for a character that is not one of the campaign's living ones.
func (s *Service) CastingOptions(ctx context.Context, tx pgx.Tx, campaignID, characterID string) ([]*playv1.CastingSpell, error) {
	_, d, u, content, err := s.outsideCaster(ctx, tx, campaignID, characterID)
	if err != nil {
		return nil, err
	}
	var out []*playv1.CastingSpell
	for _, o := range combat.OutsideSpells(d, u) {
		det, ok := content.SpellDetails(o.Spell.Key)
		if !ok {
			continue
		}
		cs := &playv1.CastingSpell{
			Spell: spellToProto(o.Spell), CanCast: o.CanCast, RitualAllowed: o.CanRitual, Reason: reasonProto(o.Reason),
			CastingMinutes: i32(o.Minutes), RitualMinutes: i32(o.RitualMinutes), CastingTimePt: castingTimePT(det.CastingTime),
			Lasts: det.Duration.Lasts(), DurationSeconds: durationSeconds(det.Duration), RestEnds: restToProto[rules.EndedByRest(det.Duration)],
			MaxTargets: i32(det.Target.MaxTargets(o.Spell.Level, o.Spell.Level)), TargetsPerLevel: i32(det.Target.PerSlotLevel),
			CasterOnly: det.Target.CasterOnly(), RangeKind: det.Range.Kind,
			// The reach as the caster casts it (a table feat may multiply a spell attack's), as OutsideSpell checks it.
			RangeFt: i32(d.SpellRangeFt(det.AttackType, det.Range.Kind, det.Range.DistanceFt)),
		}
		for _, c := range o.Slots {
			cs.Slots = append(cs.Slots, &rulesv1.SlotChoice{Level: i32(c.Level), Pact: c.Pact, Free: i32(c.Free), Total: i32(c.Total)})
		}
		for _, g := range o.Grants {
			cs.Grants = append(cs.Grants, &playv1.CastingGrant{
				SourceKey: g.Source, SourceNamePt: g.SourcePT, AtWill: g.AtWill, Resource: g.Resource, UsesLeft: i32(g.UsesLeft),
				AlsoWithSlot: g.AlsoWithSlot, RitualOnly: g.RitualOnly, Ready: g.Ready(), Ability: string(g.Ability),
				AttackBonus: i32(g.AttackBonus), SaveDc: i32(g.SaveDC),
			})
		}
		effect, dice := castingEffect(content, det)
		cs.Effect, cs.RollsDice, cs.RollDice = effectKindToProto[effect], dice != "", dice
		cs.Summons = effect == link.EffectSummon
		out = append(out, cs)
	}
	return out, nil
}

var restToProto = map[string]playv1.RestThatEnds{
	rules.RestShort: playv1.RestThatEnds_REST_THAT_ENDS_SHORT,
	rules.RestLong:  playv1.RestThatEnds_REST_THAT_ENDS_LONG,
}

var effectKindToProto = map[string]playv1.CastingEffectKind{
	link.EffectNarrated:   playv1.CastingEffectKind_CASTING_EFFECT_KIND_NARRATED,
	link.EffectHeal:       playv1.CastingEffectKind_CASTING_EFFECT_KIND_HEAL,
	link.EffectTempHP:     playv1.CastingEffectKind_CASTING_EFFECT_KIND_TEMPORARY_HIT_POINTS,
	link.EffectMaxHP:      playv1.CastingEffectKind_CASTING_EFFECT_KIND_MAX_HIT_POINTS,
	link.EffectArmorClass: playv1.CastingEffectKind_CASTING_EFFECT_KIND_ARMOR_CLASS,
	link.EffectSummon:     playv1.CastingEffectKind_CASTING_EFFECT_KIND_SUMMON,
}

// durationSeconds is the spell's timed duration in seconds, 0 for none, clipped to the
// column that keeps it.
func durationSeconds(d rules.SpellDuration) int32 {
	secs, _ := d.Seconds()
	return i32(secs)
}

// castingEffect says what the server applies for a spell, from the spell alone, and
// the dice it rolls at the spell's own level ("2d8", "" for none).
func castingEffect(content *rules.Content, det *rules.SpellDetails) (effect, dice string) {
	key, level := det.Spell.Key, det.Spell.Level
	if key == rules.MageArmorSpell {
		return link.EffectArmorClass, ""
	}
	if _, err := content.SummonOptions(key, level, rules.Build{}); !errors.Is(err, rules.ErrNotSummonSpell) {
		return link.EffectSummon, ""
	}
	if fx, ok := content.SpellEffect(key, level); ok {
		switch fx.Kind {
		case rules.SpellKindFlatHeal:
			return link.EffectHeal, ""
		case rules.SpellKindTempHP:
			return link.EffectTempHP, fmt.Sprintf("%dd%d", fx.Dice.Count, fx.Dice.Sides)
		case rules.SpellKindMaxHP:
			return link.EffectMaxHP, ""
		}
		return link.EffectNarrated, "" // Sleep, Power Word Kill...: the combat's, narrated here
	}
	if heal, ok := det.HealAt(level); ok && heal.Parsed {
		return link.EffectHeal, fmt.Sprintf("%dd%d", heal.Dice.Count, heal.Dice.Sides)
	}
	return link.EffectNarrated, ""
}

// OutsideSpell implements play.CombatRoster: the spell as the character casts it
// outside a combat, with a slot of slotLevel (0 for a cantrip, and for a ritual: the
// spell's own level). It says what the sheet allows (Known, Prepared, CanRitual,
// CanCast, Slots) and does not refuse anything itself. `not_found` for a character
// that is not one of the campaign's living ones, and for a spell the content does not have.
func (s *Service) OutsideSpell(ctx context.Context, tx pgx.Tx, campaignID, characterID, spellKey string, slotLevel int) (link.OutsideSpell, error) {
	kind, d, u, content, err := s.outsideCaster(ctx, tx, campaignID, characterID)
	if err != nil {
		return link.OutsideSpell{}, err
	}
	det, ok := content.SpellDetails(spellKey)
	if !ok {
		return link.OutsideSpell{}, connect.NewError(connect.CodeNotFound, errUnknownSpell)
	}
	if slotLevel < det.Spell.Level {
		slotLevel = det.Spell.Level
	}
	sp, err := s.CombatSpell(ctx, tx, campaignID, characterID, spellKey, slotLevel, "")
	if err != nil {
		return link.OutsideSpell{}, err
	}
	out := link.OutsideSpell{Spell: sp, Ritual: det.Spell.Ritual, NPC: kind != kindPlayer}
	st := d.SpellStanding(spellKey, det.Spell.Ritual)
	out.Known, out.Prepared, out.CanRitual = st.Known, st.Prepared, st.CanRitual
	out.CastMinutes, out.RitualMinutes = rules.CastMinutes(det.CastingTime, false), rules.CastMinutes(det.CastingTime, true)
	for _, o := range combat.OutsideSpells(d, u) {
		if o.Spell.Key != spellKey {
			continue
		}
		out.CanCast = o.CanCast
		for _, c := range o.Slots {
			out.Slots = append(out.Slots, link.CastSlot{Level: c.Level, Pact: c.Pact, Free: c.Free})
		}
		for _, g := range o.Grants {
			out.Grants = append(out.Grants, link.OutsideGrant{
				Source: g.Source, SourcePT: g.SourcePT, AtWill: g.AtWill, Resource: g.Resource, UsesLeft: g.UsesLeft,
				AlsoWithSlot: g.AlsoWithSlot, RitualOnly: g.RitualOnly, Ready: g.Ready(),
			})
		}
	}
	out.Lasts, out.RestEnds = det.Duration.Lasts(), rules.EndedByRest(det.Duration)
	secs, _ := det.Duration.Seconds()
	out.DurationSeconds = secs
	out.Effect, _ = castingEffect(content, det)
	h := combat.HealFeaturesOf(d.Features)
	out.Healing = link.HealFeatures{Disciple: h.Disciple, Blessed: h.Blessed, Supreme: h.Supreme}
	return out, nil
}

// MageArmorAC implements play.CombatRoster: the armor class a character would have
// with Mage Armor on it (SRD 5.1: base 13 + Dexterity, for a creature that wears no
// armor), and whether it wears armor. The sheet's own armor class stays what it is:
// the spell lasts hours and belongs to the game session.
func (s *Service) MageArmorAC(ctx context.Context, tx pgx.Tx, campaignID, characterID string) (link.MageArmor, error) {
	id, ok := uid.Canonical(characterID)
	if !ok {
		return link.MageArmor{}, errCharacterNotFound()
	}
	rows, err := s.queriesIn(tx).ListCombatCharacters(ctx, charactersdb.ListCombatCharactersParams{CampaignID: campaignID, Ids: []string{id}})
	if err != nil {
		return link.MageArmor{}, s.dbError(ctx, "read a character for Mage Armor", err)
	}
	if len(rows) == 0 {
		return link.MageArmor{}, errCharacterNotFound()
	}
	sheet, err := loadSheet(rows[0].ID, rows[0].Sheet)
	if err != nil {
		return link.MageArmor{}, s.dbError(ctx, "read a character for Mage Armor", err)
	}
	full := sheet.GetFull()
	if full == nil {
		return link.MageArmor{}, nil // a basic sheet has no armor to read: the master narrates
	}
	content, err := s.contentFor(ctx, tx, campaignID)
	if err != nil {
		return link.MageArmor{}, s.dbError(ctx, "read rules content", err)
	}
	b := buildWithStates(full, stateKeysOf(rows[0].ActiveStates))
	if b.Armor != "" {
		return link.MageArmor{Applies: true, Wears: true}, nil
	}
	b.MageArmor = true
	return link.MageArmor{Applies: true, AC: rules.Derive(b, content).ArmorClass}, nil
}
