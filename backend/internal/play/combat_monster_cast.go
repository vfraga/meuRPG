package play

import (
	"context"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// A monster casts its spells with the spell flow of an NPC (CastSpell): the turn options list the
// spells of its Spellcasting and Innate Spellcasting traits (characters.monsterDerived), and the
// cast rolls the attack or the saving throw with the stat block's DC and bonus. What is the
// monster's own is what the cast costs: a spell cast with slots spends a slot of the creature, an
// innate spell cast "3/day" spends one of its uses, and what is cast at will costs nothing (SRD
// 5.1, "Monsters": Spellcasting, Innate Spellcasting).

// Where a monster's spell comes from.
const (
	castAtWill = iota + 1
	castPerDay
	castSlots
)

// castSource says how a creature casts a spell.
type castSource struct {
	kind int
	// useKey and uses are the uses a day of a spell cast "N/day".
	useKey string
	uses   int
	// level is the spell's level.
	level int
	// slots[l-1] are the slots of level l the creature has.
	slots [9]int
}

// monsterSpellSource finds how the creature casts the spell; false when it does not have it.
func monsterSpellSource(plan *rules.MonsterPlan, spellKey string) (castSource, bool) {
	var src castSource
	found := false
	for _, sc := range plan.Spellcasting {
		for _, l := range sc.Levels {
			if l.Level >= 1 && l.Level <= 9 {
				src.slots[l.Level-1] += l.Slots
			}
		}
	}
	for _, sc := range plan.Spellcasting {
		for _, r := range sc.AtWill {
			if r.Key == spellKey {
				return castSource{kind: castAtWill, level: r.Level}, true
			}
		}
		for g, grp := range sc.PerDay {
			for _, r := range grp.Spells {
				if r.Key == spellKey {
					return castSource{kind: castPerDay, useKey: innateUseKey(sc, g, r.Key), uses: grp.Uses, level: r.Level}, true
				}
			}
		}
		for _, l := range sc.Levels {
			for _, r := range l.Spells {
				if r.Key == spellKey && !found {
					src.kind, src.level, found = castSlots, r.Level, true
				}
			}
		}
	}
	return src, found
}

// monsterCasting corrects the turn options of a monster for what it has spent: a slot it
// has not got is not offered, a spell "3/day" shows its uses as its slots, and a spell cast at
// will needs no slot. The options come from the sheet, which does not know what was spent.
func (s *Service) monsterCasting(ctx context.Context, tx pgx.Tx, campaignID string, c playdb.Combatant, opts *rulesv1.TurnOptions) error {
	if c.Kind != kindNPC || len(opts.GetSpells()) == 0 {
		return nil
	}
	mon, err := s.monsterOf(ctx, tx, campaignID, c)
	if err != nil || mon == nil || len(mon.plan.Spellcasting) == 0 {
		return err
	}
	st, err := monsterStateOf(c)
	if err != nil {
		return err
	}
	for _, so := range opts.Spells {
		src, ok := monsterSpellSource(mon.plan, so.GetSpell().GetKey())
		if !ok {
			continue
		}
		level := int(so.GetSpell().GetLevel())
		var choices []*rulesv1.SlotChoice
		var out *rulesv1.DisabledReason
		switch src.kind {
		case castAtWill:
			if level > 0 {
				choices = []*rulesv1.SlotChoice{{Level: int32(level), Free: 1}} //nolint:gosec // a spell level, 1 to 9
			}
		case castPerDay:
			left := src.uses - st.Used[src.useKey]
			if left > 0 {
				choices = []*rulesv1.SlotChoice{{Level: int32(level), Free: int32(left)}} //nolint:gosec // a spell level and a few uses
			} else {
				out = &rulesv1.DisabledReason{Code: rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_NO_USES}
			}
		case castSlots:
			for l := level; l <= 9; l++ {
				if free := st.SlotLeft(l, src.slots[l-1]); free > 0 {
					choices = append(choices, &rulesv1.SlotChoice{Level: int32(l), Free: int32(free)}) //nolint:gosec // a spell level and its slots
				}
			}
			if len(choices) == 0 {
				out = &rulesv1.DisabledReason{Code: rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_NO_SLOT, MinLevel: int32(level)} //nolint:gosec // a spell level
			}
		}
		so.Slots = choices
		// Only the slot's own reasons change; the economy's stay.
		if r := so.GetReason().GetCode(); r == rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_NO_SLOT || r == rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_NO_USES {
			so.Reason, so.Enabled = nil, true
		}
		if out != nil && so.GetReason() == nil {
			so.Reason, so.Enabled = out, false
		}
	}
	return nil
}

// spendMonsterCast spends what casting a spell costs a monster, inside CastSpell's transaction: a
// slot, or one of the uses a day of an innate spell. A caster that is not a monster, and a spell
// cast at will, spend nothing here.
func (s *Service) spendMonsterCast(ctx context.Context, c *combatTx, caster playdb.Combatant, spellKey string, slot *slotRef, made *actionEvent) error {
	if caster.Kind != kindNPC {
		return nil
	}
	mon, err := s.monsterOf(ctx, c.tx, c.session.CampaignID, caster)
	if err != nil || mon == nil {
		return err
	}
	src, ok := monsterSpellSource(mon.plan, spellKey)
	if !ok || src.kind == castAtWill {
		return nil
	}
	st, err := monsterStateOf(caster)
	if err != nil {
		return err
	}
	before := st.Clone()
	switch src.kind {
	case castPerDay:
		if st.Used[src.useKey] >= src.uses {
			return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NO_USES, "the spell has no uses left")
		}
		if st.Used == nil {
			st.Used = map[string]int{}
		}
		st.Used[src.useKey]++
	case castSlots:
		if slot == nil || int(slot.Level) < src.level || st.SlotLeft(int(slot.Level), src.slots[slot.Level-1]) < 1 {
			return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NO_SLOT, "the creature has no free slot for this spell")
		}
		st.SpendSlot(int(slot.Level))
	}
	if err := saveMonsterState(ctx, c, caster, st); err != nil {
		return err
	}
	if made.Monster == nil {
		made.Monster = &monsterEvent{}
	}
	made.Monster.Before = &before
	return nil
}
