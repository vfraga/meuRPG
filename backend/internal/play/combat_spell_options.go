package play

import (
	"context"
	"errors"
	"math"
	"slices"
	"strings"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The cast options a character's table features offer (the engine's spell_modifier effect). They are what the
// features add to the damage of a spell: a swap of the damage type, a fixed damage, more dice for a higher slot.
// The options without a resource that only add (bonus damage, extra dice) are always on and apply by
// themselves; the others are ticked on the cast sheet (CastSpellRequest.spell_options), and spend a use of
// their resource with the slot. Everything is worked out on link.Spell, so the damage rolls as the spell's own.

// damageTypePrefix starts the key of an SRD damage type.
const damageTypePrefix = "damage-type:"

// spellFacts is what an option reads of a spell: its level and school and the damage types it deals.
type spellFacts struct {
	level  int
	school string
	types  []string
}

func factsOfSpell(sp link.Spell) spellFacts {
	f := spellFacts{level: sp.Level, school: sp.School}
	for _, d := range sp.Damages {
		if !slices.Contains(f.types, d.DamageType) {
			f.types = append(f.types, d.DamageType)
		}
	}
	return f
}

// fits says the modifier is for the spell: it deals damage and is what applies_to names.
func (f spellFacts) fits(m link.SpellModifier) bool {
	if len(f.types) == 0 {
		return false
	}
	switch m.AppliesTo {
	case rules.SpellAppliesDamageSpell:
		return true
	case rules.SpellAppliesCantrip:
		return f.level == 0
	}
	return m.AppliesTo == f.school || slices.Contains(f.types, m.AppliesTo)
}

// swapTypes are the damage types a swap lets the caster pick for the spell: the effect's types, when the spell
// deals damage of one of them, except those it already deals. Nil when the swap does not fit.
func (f spellFacts) swapTypes(m link.SpellModifier) []string {
	if m.Change != rules.SpellChangeDamageTypeSwap || !slices.ContainsFunc(f.types, func(t string) bool { return slices.Contains(m.From, t) }) {
		return nil
	}
	var out []string
	for _, t := range m.From {
		if !slices.Contains(f.types, t) {
			out = append(out, t)
		}
	}
	return out
}

// alwaysOn says the option applies without being ticked.
func alwaysOn(m link.SpellModifier) bool {
	return m.Resource == "" && m.Change != rules.SpellChangeDamageTypeSwap
}

// tableTick is an option the caster ticked, with the damage type a swap picked.
type tableTick struct {
	mod  link.SpellModifier
	pick string
}

// applyTableOptions gives the spell what the options that apply do to its damage: the always-on ones and the
// ticked ones, swaps first so the others see the new type.
func applyTableOptions(sp link.Spell, mods []link.SpellModifier, ticks []tableTick, slotLevel int) link.Spell {
	f := factsOfSpell(sp)
	sp.Damages = slices.Clone(sp.Damages)
	for _, t := range ticks {
		if t.mod.Change == rules.SpellChangeDamageTypeSwap && f.fits(t.mod) {
			swapDamage(&sp, t.mod, t.pick)
		}
	}
	for _, m := range mods {
		if m.Change == rules.SpellChangeDamageTypeSwap || !f.fits(m) {
			continue
		}
		if !alwaysOn(m) && !slices.ContainsFunc(ticks, func(t tableTick) bool { return t.mod.Key == m.Key }) {
			continue
		}
		addDamage(&sp, m, slotLevel)
	}
	return sp
}

// swapDamage turns the damage of the swap's types into the picked one.
func swapDamage(sp *link.Spell, m link.SpellModifier, pick string) {
	for i, d := range sp.Damages {
		if slices.Contains(m.From, d.DamageType) {
			sp.Damages[i].DamageType = pick
		}
	}
}

// addDamage adds a bonus damage or the extra dice of a higher slot to the spell's damage of the type it names.
func addDamage(sp *link.Spell, m link.SpellModifier, slotLevel int) {
	switch m.Change {
	case rules.SpellChangeBonusDamage:
		typ := m.BonusType
		if typ == "" {
			typ = sp.Damages[0].DamageType
		}
		if i := slices.IndexFunc(sp.Damages, func(d link.Dice) bool { return d.DamageType == typ }); i >= 0 {
			sp.Damages[i].Bonus += m.Bonus
			return
		}
		sp.Damages = append(sp.Damages, link.Dice{Bonus: m.Bonus, DamageType: typ})
	case rules.SpellChangeExtraDice:
		over := slotLevel - sp.Level
		if sp.Level == 0 || over <= 0 {
			return
		}
		i := slices.IndexFunc(sp.Damages, func(d link.Dice) bool {
			return d.Count > 0 && d.Sides > 0 && (!strings.HasPrefix(m.AppliesTo, damageTypePrefix) || d.DamageType == m.AppliesTo)
		})
		if i >= 0 {
			sp.Damages[i].Count += m.ExtraDice * over
		}
	}
}

// tickedOptions reads and checks the options the caster ticked: each one the spell offers, ticked once, a swap
// with a type it offers. The resource is spent later, with the slot.
func tickedOptions(chosen []*playv1.SpellOptionChoice, mods []link.SpellModifier, sp link.Spell) ([]tableTick, error) {
	f := factsOfSpell(sp)
	var out []tableTick
	for _, c := range chosen {
		i := slices.IndexFunc(mods, func(m link.SpellModifier) bool { return m.Key == c.GetKey() })
		if i < 0 || !f.fits(mods[i]) || alwaysOn(mods[i]) || slices.ContainsFunc(out, func(t tableTick) bool { return t.mod.Key == c.GetKey() }) {
			return nil, optionRefusal("the spell does not take this option, or it is ticked twice")
		}
		m := mods[i]
		if m.Change == rules.SpellChangeDamageTypeSwap {
			if !slices.Contains(f.swapTypes(m), c.GetDamageTypeKey()) {
				return nil, optionRefusal("a damage type swap names a new damage type the spell can take")
			}
		} else if c.GetDamageTypeKey() != "" {
			return nil, optionRefusal("only a damage type swap names a damage type")
		}
		out = append(out, tableTick{mod: m, pick: c.GetDamageTypeKey()})
	}
	return out, nil
}

// optionRefusal is the invalid_argument of a cast option that does not hold.
func optionRefusal(msg string) error {
	return connect.NewError(connect.CodeInvalidArgument, errors.New(msg))
}

// applyTableSpellOptions reads the caster's options for a player's cast and gives the spell what they change.
// It returns the options ticked, whose uses the cast spends.
func (s *Service) applyTableSpellOptions(ctx context.Context, c *combatTx, campaignID string, caster playdb.Combatant, chosen []*playv1.SpellOptionChoice, sp link.Spell, slotLevel int) (link.Spell, []tableTick, error) {
	if caster.Kind != kindPlayer {
		if len(chosen) > 0 {
			return sp, nil, optionRefusal("cast options are for a player's character")
		}
		return sp, nil, nil
	}
	sheet, err := s.sheetOf(ctx, c.tx, campaignID, caster)
	if err != nil {
		return sp, nil, err
	}
	mods := sheet.Traits.SpellModifiers
	ticks, err := tickedOptions(chosen, mods, sp)
	if err != nil {
		return sp, nil, err
	}
	return applyTableOptions(sp, mods, ticks, slotLevel), ticks, nil
}

// spendTableOptions spends a use of the resource of each option ticked, noting them in the event for the undo.
func (f *castFlow) spendTableOptions(ctx context.Context) error {
	for _, t := range f.ticks {
		if t.mod.Resource == "" {
			continue
		}
		v, err := f.s.spendResource(ctx, f.c, f.caster.CharacterID, t.mod.Resource, clamp32(optionCost(t.mod), 1, math.MaxInt32))
		if err != nil {
			return err
		}
		f.out.vitals = append(f.out.vitals, v)
		for range optionCost(t.mod) {
			f.out.made.TableUses = append(f.out.made.TableUses, t.mod.Resource)
		}
	}
	return nil
}

// optionCost is how many uses of its resource an option spends: the cost the feature says, at least 1.
func optionCost(m link.SpellModifier) int { return max(m.Cost, 1) }

// tableUsesBack gives back the uses a cast's options spent.
func (u *undoer) tableUsesBack(ctx context.Context, who playdb.Combatant, ev actionEvent) error {
	if who.Kind != kindPlayer {
		return nil
	}
	for _, key := range ev.TableUses {
		v, err := u.s.spendResource(ctx, u.c, who.CharacterID, key, -1)
		if err != nil {
			return err
		}
		u.keep(v)
	}
	return nil
}

// attachTableOptions lists, for each spell of the turn options, the cast options the caster's table features
// offer at the spell's own level. Only a player's character has them.
func (s *Service) attachTableOptions(ctx context.Context, reads *turnReads, who playdb.Combatant, opts *rulesv1.TurnOptions) error {
	if who.Kind != kindPlayer || len(opts.GetSpells()) == 0 {
		return nil
	}
	sheet, err := reads.sheetOf(ctx)
	if err != nil || len(sheet.Traits.SpellModifiers) == 0 {
		return err
	}
	content, err := s.roster.RulesContent(ctx, nil, reads.campaignID)
	if err != nil {
		return err
	}
	vitals, err := reads.vitalsOf(ctx)
	if err != nil {
		return err
	}
	for _, so := range opts.GetSpells() {
		det, ok := content.SpellDetails(so.GetSpell().GetKey())
		if !ok {
			continue
		}
		so.TableOptions = tableOptionsFor(factsOfDetails(det), sheet.Traits.SpellModifiers, vitals)
	}
	return nil
}

// factsOfDetails is what the options read of a spell the rules know: the types its damage can be of.
func factsOfDetails(det *rules.SpellDetails) spellFacts {
	f := spellFacts{level: det.Spell.Level, school: det.Spell.School}
	for _, d := range det.Damage {
		if d.Type != "" && !slices.Contains(f.types, d.Type) {
			f.types = append(f.types, d.Type)
		}
	}
	return f
}

// tableOptionsFor lists the options that fit a spell, for the cast sheet.
func tableOptionsFor(f spellFacts, mods []link.SpellModifier, vitals *playv1.CharacterVitals) []*rulesv1.SpellTableOption {
	var out []*rulesv1.SpellTableOption
	for _, m := range mods {
		if !f.fits(m) {
			continue
		}
		o := &rulesv1.SpellTableOption{
			Key: m.Key, NamePt: m.NamePT, SummaryPt: m.TextPT, Change: m.Change, AlwaysOn: alwaysOn(m), Resource: m.Resource, Allowed: true,
			DamageTypeKeys: f.swapTypes(m),
		}
		if m.Change == rules.SpellChangeDamageTypeSwap && len(o.DamageTypeKeys) == 0 {
			continue
		}
		if m.Resource != "" {
			left, _ := poolLeft(vitals, m.Resource)
			o.UsesLeft, o.Cost = max(left, 0), clamp32(optionCost(m), 1, math.MaxInt32)
			if int(left) < optionCost(m) {
				o.Allowed, o.DisabledReasonPt = false, "Sem usos do recurso."
			}
		}
		out = append(out, o)
	}
	return out
}
