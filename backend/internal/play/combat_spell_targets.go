package play

import (
	"context"
	"math"
	"slices"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// The targets of the spells in the turn options (GetTurnOptions): for each spell the combatant can cast now
// and each save cantrip, who it can target as the viewer sees them.

// maxDartsAtSlot is the most darts a slot level of Magic Missile lists.
const maxDartsAtSlot = 100

// spellTargetEntry is a spell that is listed: its key, its level and the slots it can be cast with.
type spellTargetEntry struct {
	key   string
	level int
	slots []*rulesv1.SlotChoice
}

// spellTargetScene is what the targets of every spell are listed against: the terrain the viewer plans on,
// the caster, the combat, the viewer, whether the combat has no grid and the roll modes of the turn.
type spellTargetScene struct {
	terrain grid.Terrain
	who     playdb.Combatant
	cs      []playdb.Combatant
	v       combatViewer
	theatre bool
	modes   turnModes
}

// spellsToList are the spells the options list targets for: the ones that can be cast now, then the save
// cantrips among the attacks. Spells that cannot be cast now are left out: they carry their reason and need
// no targets.
func spellsToList(opts *rulesv1.TurnOptions) []spellTargetEntry {
	var spells []spellTargetEntry
	for _, sp := range opts.GetSpells() {
		if sp.GetEnabled() {
			level, slots := int(sp.GetSpell().GetLevel()), sp.GetSlots()
			if cl := sp.GetGrant().GetCastLevel(); cl > 0 { // a cast through a grant with no slot is read at its cast level
				level, slots = int(cl), append(slices.Clone(slots), &rulesv1.SlotChoice{Level: cl})
			}
			spells = append(spells, spellTargetEntry{sp.GetSpell().GetKey(), level, slots})
		}
	}
	for _, a := range opts.GetAttacks() {
		if a.GetEnabled() && a.GetAttack().GetKind() == rulesv1.AttackKind_ATTACK_KIND_SPELL && a.GetAttack().GetSaveDc() > 0 {
			spells = append(spells, spellTargetEntry{key: a.GetAttack().GetKey()})
		}
	}
	return spells
}

// spellTargetsFor lists, for each spell the combatant can cast now and each save
// cantrip, who it can target as the viewer sees them, with the distance, whether
// the spell cannot reach it, how many targets it takes and, for Magic Missile,
// the darts of each slot level. Spells that cannot be cast now are left out:
// they carry their reason and need no targets. entries are the spells to list.
func (s *Service) spellTargetsFor(ctx context.Context, campaignID string, scene spellTargetScene, entries []spellTargetEntry) ([]*playv1.SpellTargets, error) {
	var out []*playv1.SpellTargets
	for _, e := range entries {
		sp, err := s.roster.CombatSpell(ctx, nil, campaignID, scene.who.CharacterID, e.key, e.level, "")
		if err != nil {
			return nil, err
		}
		out = append(out, scene.targetsOf(e, sp))
	}
	return out, nil
}

// targetsOf lists the targets of one spell: who, the area and the range it is cast with, how many targets it
// takes, the roll mode of a spell attack and the darts of each slot.
func (sc *spellTargetScene) targetsOf(e spellTargetEntry, sp link.Spell) *playv1.SpellTargets {
	st := &playv1.SpellTargets{
		SpellKey: e.key, Targets: spellTargetList(sc.terrain, sc.cs, sc.who, sc.v, sp, sc.theatre),
		Placement: placementFor(sp, sc.theatre), AreaShape: areaShapeProto(sp, sc.theatre), AreaSizeFt: areaSizeFor(sp, sc.theatre),
		AreaWidthFt: areaWidthFor(sp, sc.theatre), RangeFt: placedRangeFor(sp, sc.theatre),
		MaxTargets: clamp32(maxTargetsOf(sp, e.level), 0, maxCombatants), ExtraTargetPerLevel: sp.ExtraTargetPerLevel || sp.Key == magicMissile,
		TargetsPerLevel: clamp32(sp.TargetPerLevel, 0, maxCombatants),
	}
	if reach := teleportReachFt[sp.Key]; reach > 0 && !sc.theatre {
		st.TeleportRangeFt = clamp32(reach, 0, math.MaxInt32)
	}
	if sp.AttackType != "" { // a spell attack roll: the target's mode, as CastSpell settles it (SRD 5.1)
		shape := shapeOfSpell(sp)
		sc.modes.annotateShape(&shape, st.Targets)
	}
	if spellIgnoresCover(sc.modes.sheet.Traits, sp) { // the caster's own feature, as CastSpell rolls it
		for _, tg := range st.Targets {
			clearPartialCover(tg)
		}
	}
	st.Darts = dartsOfSlots(sp, e.slots)
	return st
}

// dartsOfSlots lists the darts of each slot level a spell can be cast with, once for each level.
func dartsOfSlots(sp link.Spell, slots []*rulesv1.SlotChoice) []*playv1.DartsAtSlot {
	var out []*playv1.DartsAtSlot
	for _, slot := range slots {
		if n := sharesAt(sp, int(slot.GetLevel())); n > 0 && !slices.ContainsFunc(out, func(d *playv1.DartsAtSlot) bool { return d.GetSlotLevel() == slot.GetLevel() }) {
			out = append(out, &playv1.DartsAtSlot{SlotLevel: slot.GetLevel(), Darts: clamp32(n, 0, maxDartsAtSlot)})
		}
	}
	return out
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
