package play

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// Metamagic on a casting (SRD 5.1, Sorcerer 3). The choice rides on CastSpell: the
// options are checked against what the sorcerer knows and the spell takes at the slot it
// is cast with, the sorcery points are spent with the slot, and the effects the engine
// can give are given: Twinned Spell's second target, Careful Spell's saving throws that
// pass, Heightened Spell's disadvantage on the first save, Distant Spell's range and
// Quickened Spell's bonus action. Subtle Spell, Extended Spell and Empowered Spell cost
// their points and are written on the line; the table plays them (no component, no
// duration clock and no choice of the damage dice to reroll are in the engine).

const resMetamagic = "metamagic"

// castMeta is the Metamagic a casting uses, once checked.
type castMeta struct {
	keys []string
	cost int
	// second is the target Twinned Spell adds; zero without it.
	second playdb.Combatant
	// careful are the combatants that pass the saving throw on their own, heightened
	// the one that has disadvantage on its first.
	careful    map[string]bool
	heightened string
	quickened  bool
	distant    bool
}

// passes says the combatant passes its saving throw on its own (Careful Spell).
func (m *castMeta) passes(id string) bool { return m != nil && m.careful[id] }

// disadvantaged says the combatant has disadvantage on its first saving throw against
// the spell (Heightened Spell).
func (m *castMeta) disadvantaged(id string) bool { return m != nil && m.heightened == id }

// apply gives the spell what Distant Spell and Quickened Spell change: the range
// (doubled, or 30 feet for a spell of touch) and the casting time (a bonus action).
func (m *castMeta) apply(sp link.Spell) link.Spell {
	if m == nil {
		return sp
	}
	if m.distant && (sp.RangeKind == rules.RangeTouch || sp.RangeKind == rules.RangeRanged) {
		sp.RangeFt, sp.RangeKind = rules.DistantRangeFt(sp.RangeKind, sp.RangeFt), rules.RangeRanged
	}
	if m.quickened {
		sp.Economy = rules.EconomyBonusAction
	}
	return sp
}

// metaRefusal is the invalid_argument of a Metamagic choice that does not hold.
func metaRefusal(msg string) error {
	return connect.NewError(connect.CodeInvalidArgument, errors.New(msg))
}

// prepareMetamagic checks the choice of Metamagic of a casting and works out what it
// does. It returns nil for a casting with none.
func (s *Service) prepareMetamagic(
	ctx context.Context, c *combatTx, m authz.Membership, v combatViewer, caster playdb.Combatant,
	choices []*playv1.MetamagicChoice, spellKey string, slotLevel int, sp link.Spell, targs, cs []playdb.Combatant,
) (*castMeta, error) {
	if len(choices) == 0 {
		return nil, nil
	}
	if caster.Kind != kindPlayer {
		return nil, metaRefusal("Metamagic is for a player's character; the master adjusts an NPC by hand")
	}
	sheet, err := s.sheetOf(ctx, c.tx, m.CampaignID, caster)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(choices))
	for _, ch := range choices {
		keys = append(keys, ch.GetKey())
	}
	if err := rules.CheckMetamagicChoice(keys, sheet.Metamagic); err != nil {
		return nil, metaRefusal("the Metamagic is not one the character knows, or it is more than one option (only Empowered Spell joins another)")
	}
	meta, err := s.takeMetamagic(ctx, c, m, v, sheet, choices, keys, spellKey, slotLevel, targs, cs)
	if err != nil {
		return nil, err
	}
	if err := meta.checkQuickened(v, caster, sp); err != nil {
		return nil, err
	}
	if err := s.checkSorceryPoints(ctx, c, m, caster, meta); err != nil {
		return nil, err
	}
	return meta, nil
}

// takeMetamagic checks each option against what the spell takes at the slot it is cast with, adds up
// the sorcery points they cost and reads their parameters.
func (s *Service) takeMetamagic(
	ctx context.Context, c *combatTx, m authz.Membership, v combatViewer, sheet link.Sheet,
	choices []*playv1.MetamagicChoice, keys []string, spellKey string, slotLevel int, targs, cs []playdb.Combatant,
) (*castMeta, error) {
	content, err := s.roster.RulesContent(ctx, c.tx, m.CampaignID)
	if err != nil {
		return nil, err
	}
	allowed := map[string]rules.MetamagicOption{}
	for _, o := range content.MetamagicOptions(sheet.Metamagic, spellKey, slotLevel) {
		allowed[o.Key] = o
	}
	meta := &castMeta{keys: keys, careful: map[string]bool{}}
	for _, ch := range choices {
		o, ok := allowed[ch.GetKey()]
		if !ok || !o.Allowed {
			return nil, metaRefusal("the spell does not take this Metamagic option")
		}
		meta.cost += o.Cost
		if err := meta.take(ch, sheet.ChaMod, v, targs, cs); err != nil {
			return nil, err
		}
	}
	return meta, nil
}

// checkQuickened checks the casting time Quickened Spell turns into a bonus action: it must be free, and
// a bonus action spell leaves no other spell in the turn but a cantrip of 1 action (SRD 5.1). The master
// is not held to it.
func (m *castMeta) checkQuickened(v combatViewer, caster playdb.Combatant, sp link.Spell) error {
	if !m.quickened || v.master {
		return nil
	}
	if caster.BonusActionUsed {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_BONUS_ACTION_USED, "the bonus action of this turn is used")
	}
	if combat.SpellLimited(combat.TurnState{SpellCast: caster.SpellCast, BonusSpellCast: caster.BonusSpellCast}, rules.EconomyBonusAction, sp.Level) {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_BONUS_ACTION_SPELL_LIMIT,
			"a spell cast with a bonus action leaves no other spell this turn but a cantrip of 1 action")
	}
	return nil
}

// checkSorceryPoints checks the sorcerer has the points the options cost.
func (s *Service) checkSorceryPoints(ctx context.Context, c *combatTx, m authz.Membership, caster playdb.Combatant, meta *castMeta) error {
	vitals, err := s.vitals.GetVitalsTx(ctx, c.tx, m.CampaignID, caster.CharacterID)
	if err != nil {
		return err
	}
	if left, _ := poolLeft(vitals, rules.SorceryPointsKey); int(left) < meta.cost {
		return resourceBlocked(playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_NOT_ENOUGH_POINTS, "there are not enough sorcery points",
			func(b *playv1.ResourceBlocked) { b.Needed, b.Available = clamp32(meta.cost, 0, 1<<20), left })
	}
	return nil
}

// take reads the parameters of one option: Twinned Spell's second target, the creatures
// Careful Spell protects and the target of Heightened Spell.
func (m *castMeta) take(ch *playv1.MetamagicChoice, chaMod int, v combatViewer, targs, cs []playdb.Combatant) error {
	switch ch.GetKey() {
	case rules.MetamagicTwinned:
		return m.takeTwinned(ch, v, targs, cs)
	case rules.MetamagicCareful:
		return m.takeCareful(ch, chaMod)
	case rules.MetamagicHeightened:
		id, err := parseCombatID(ch.GetHeightenedId(), "combatant")
		if err != nil {
			return err
		}
		m.heightened = id
	case rules.MetamagicQuickened:
		m.quickened = true
	case rules.MetamagicDistant:
		m.distant = true
	}
	return nil
}

// takeTwinned reads Twinned Spell's second target: another creature the caller sees, still standing.
func (m *castMeta) takeTwinned(ch *playv1.MetamagicChoice, v combatViewer, targs, cs []playdb.Combatant) error {
	if len(ch.GetTargetIds()) != 1 || len(targs) != 1 {
		return metaRefusal("Twinned Spell takes the spell's target and one more")
	}
	id, err := parseCombatID(ch.GetTargetIds()[0], "combatant")
	if err != nil {
		return err
	}
	if id == targs[0].ID {
		return metaRefusal("the second target must be another creature")
	}
	second, err := findCombatant(cs, id, v)
	if err != nil {
		return err
	}
	if second.Defeated {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TARGET_DEFEATED, "a target is defeated")
	}
	m.second = second
	return nil
}

// takeCareful reads the creatures Careful Spell protects: one up to the Charisma modifier, each once.
func (m *castMeta) takeCareful(ch *playv1.MetamagicChoice, chaMod int) error {
	ids := ch.GetCarefulIds()
	if len(ids) < 1 || len(ids) > rules.CarefulCreatures(chaMod) {
		return metaRefusal("Careful Spell protects from one creature up to the Charisma modifier")
	}
	for _, raw := range ids {
		id, err := parseCombatID(raw, "combatant")
		if err != nil {
			return err
		}
		m.careful[id] = true
	}
	if len(m.careful) != len(ids) {
		return metaRefusal("a creature is protected twice")
	}
	return nil
}

// membersIn checks that the creatures Careful Spell protects and Heightened Spell burdens are
// among the spell's targets. An area spell on a map only knows its targets once the area is
// placed, so it runs after them.
func (m *castMeta) membersIn(targs []playdb.Combatant) error {
	if m == nil {
		return nil
	}
	listed := func(id string) bool {
		return slices.ContainsFunc(targs, func(t playdb.Combatant) bool { return t.ID == id })
	}
	for id := range m.careful {
		if !listed(id) {
			return metaRefusal("a protected creature must be one of the spell's targets")
		}
	}
	if m.heightened != "" && !listed(m.heightened) {
		return metaRefusal("the creature with disadvantage must be one of the spell's targets")
	}
	return nil
}

// checkMetaTargets checks the targets of a casting. With Twinned Spell each of the two is
// checked on its own, as the spell's one target: the spell takes one, and the second
// comes from the Metamagic.
func (s *Service) checkMetaTargets(meta *castMeta, v combatViewer, terrain grid.Terrain, cs []playdb.Combatant, sp link.Spell, caster playdb.Combatant, targs []playdb.Combatant, dartList []int, darts, slotLevel int, theatre bool) error {
	if meta == nil || meta.second.ID == "" {
		return s.checkTargets(v, terrain, cs, sp, caster, targs, dartList, darts, slotLevel, theatre)
	}
	for i, t := range targs {
		if err := s.checkTargets(v, terrain, cs, sp, caster, []playdb.Combatant{t}, dartList[i:i+1], darts, slotLevel, theatre); err != nil {
			return err
		}
	}
	return nil
}

// metamagicOptionsOf lists, for each spell of the turn options, the Metamagic options
// the caster knows and whether the spell takes them, at the spell's own level. Only a
// player's character has them.
func (s *Service) attachMetamagic(ctx context.Context, reads *turnReads, who playdb.Combatant, opts *rulesv1.TurnOptions) error {
	if who.Kind != kindPlayer || len(opts.GetSpells()) == 0 {
		return nil
	}
	sheet, err := reads.sheetOf(ctx)
	if err != nil {
		return err
	}
	for _, sp := range opts.GetSpells() {
		sp.SculptSpells = sheet.Traits.SculptSpells && sp.GetSpell().GetSchoolKey() == "school:evocation"
	}
	if len(sheet.Metamagic) == 0 {
		return nil
	}
	content, err := s.roster.RulesContent(ctx, nil, reads.campaignID)
	if err != nil {
		return err
	}
	for _, sp := range opts.GetSpells() {
		for _, o := range content.MetamagicOptions(sheet.Metamagic, sp.GetSpell().GetKey(), int(sp.GetSpell().GetLevel())) {
			sp.MetamagicOptions = append(sp.MetamagicOptions, &rulesv1.MetamagicOption{
				Key: o.Key, NamePt: o.NamePT, Cost: clamp32(o.Cost, 0, 100), SummaryPt: o.SummaryPT, Allowed: o.Allowed, DisabledReasonPt: o.ReasonPT,
			})
		}
	}
	return nil
}

// sculptedOf checks the creatures a cast spares with Sculpt Spells (SRD 5.1, School of
// Evocation, level 2) and returns them; nil for a cast with none. The caster must have the
// feature and the spell be of evocation; each creature must be one the spell reaches and
// the caller sees, other than the caster, named once; and there are at most 1 + the level
// the spell is cast at (a cantrip is 0, so 1).
func sculptedOf(raw []string, sp link.Spell, slotLevel int, caster playdb.Combatant, targs []playdb.Combatant, v combatViewer) (map[string]bool, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if !sp.Sculpts {
		return nil, metaRefusal("only a wizard with Sculpt Spells can spare creatures from a spell")
	}
	if !sp.Evocation {
		return nil, metaRefusal("Sculpt Spells is for spells of the school of evocation")
	}
	if limit := 1 + slotLevel; len(raw) > limit {
		return nil, metaRefusal(fmt.Sprintf("Sculpt Spells spares at most %d creatures with this spell", limit))
	}
	out := make(map[string]bool, len(raw))
	for _, r := range raw {
		id, err := parseCombatID(r, "combatant")
		if err != nil {
			return nil, err
		}
		if id == caster.ID {
			return nil, metaRefusal("Sculpt Spells spares other creatures, not the caster")
		}
		i := slices.IndexFunc(targs, func(t playdb.Combatant) bool { return t.ID == id })
		if i < 0 || !v.sees(targs[i]) {
			return nil, metaRefusal("a creature to spare must be one the spell reaches and the caster sees")
		}
		if out[id] {
			return nil, metaRefusal("a creature is spared twice")
		}
		out[id] = true
	}
	return out, nil
}
