package characters

import (
	"context"
	"errors"
	"slices"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// What a cast reads from a spell and the caster's sheet (MR-014, Etapa 6): the
// numbers of the spell at the slot level, the caster's attack bonus and save
// DC, the target's saving throw bonus, and the death the master confirms. Like
// the rest of the combat's reads (combatturn.go), package play declares the
// interface (CombatRoster) and this Service implements it; the rules do every
// number and this file only copies them into the plain types of package link.

// CombatSpell implements play.CombatRoster: the spell as the caster casts it
// with a slot of slotLevel (0 for a cantrip). It does not say whether the
// caster may cast it: CombatTurnOptions does. damageType is the damage type the
// caster picks for a spell that lets them choose ("" takes the first). `not_found`
// for a character that is not one of the campaign's living ones, or a spell the
// content does not have.
func (s *Service) CombatSpell(ctx context.Context, tx pgx.Tx, campaignID, characterID, spellKey string, slotLevel int, damageType string) (link.Spell, error) {
	_, d, content, err := s.fighter(ctx, tx, campaignID, characterID)
	if err != nil {
		return link.Spell{}, err
	}
	det, ok := content.SpellDetails(spellKey)
	if !ok {
		return link.Spell{}, connect.NewError(connect.CodeNotFound, errUnknownSpell)
	}
	out := link.Spell{
		Key: det.Spell.Key, Name: det.Spell.NamePT, Level: det.Spell.Level, Concentration: det.Duration.Concentration,
		RangeKind: det.Range.Kind, RangeFt: det.Range.DistanceFt, AttackType: det.AttackType,
	}
	if out.Name == "" {
		out.Name = det.Spell.Name
	}
	switch det.CastingTime.Unit {
	case rules.CastAction:
		out.Economy = rules.EconomyAction
	case rules.CastBonusAction:
		out.Economy = rules.EconomyBonusAction
	case rules.CastReaction:
		out.Economy = rules.EconomyReaction
	}

	// The class that casts it: the first that has it on its list, or the first
	// class that casts at all (as the sheet's own attacks pick it).
	var mod int
	if sc := casterFor(d, det.Spell.Classes); sc != nil {
		mod = abilityMod(d, sc.Ability)
		if det.AttackType != "" {
			out.ToHit = sc.AttackBonus
		}
		out.SaveDC = sc.SaveDC
		out.SpellDC = sc.SaveDC
	}
	out.Verbal = det.Components.Verbal
	if det.Save != nil {
		out.SaveAbility, out.SaveOnSuccess = string(det.Save.Ability), det.Save.OnSuccess
	} else {
		out.SaveDC = 0
	}
	out.DamageChoice, out.DamageTypes = det.DamageChoice, det.DamageTypeChoices()
	for _, roll := range det.DamageAtChoosing(slotLevel, d.TotalLevel, damageType) {
		// A damage without a type is no damage the engine rolls (Sono's pool of
		// hit points); one it cannot parse is the master's.
		if roll.Parsed && roll.Type != "" {
			out.Damages = append(out.Damages, link.Dice{Count: roll.Dice.Count, Sides: roll.Dice.Sides, Bonus: roll.Dice.Bonus, DamageType: roll.Type})
		}
	}
	if heal, ok := det.HealAt(slotLevel); ok && heal.Parsed {
		h := &link.Dice{Count: heal.Dice.Count, Sides: heal.Dice.Sides, Bonus: heal.Dice.Bonus}
		if heal.Dice.AddsModifier {
			h.Bonus += mod
		}
		out.Heal = h
	}
	// A spell that reads hit points rolls no damage and opens no heal: the cast
	// applies the effect itself.
	if fx, ok := content.SpellEffect(spellKey, slotLevel); ok {
		out.Damages, out.Heal = nil, nil
		out.HP = &link.HPEffect{
			Kind: fx.Kind, Pool: link.Dice{Count: fx.Dice.Count, Sides: fx.Dice.Sides}, Condition: fx.Condition,
			Threshold: fx.Threshold, Dies: fx.Dies, Heal: fx.Heal, Ends: fx.Ends, Amount: fx.Amount,
		}
	}
	_, summonErr := content.SummonOptions(spellKey, det.Spell.Level, rules.Build{})
	out.Summon = !errors.Is(summonErr, rules.ErrNotSummonSpell)
	out.IgnoresCover = content.IgnoresCover(spellKey)
	// Whom it reaches: the table spell's own target, or the SRD spell's (the
	// structured area, then the text; see rules.SpellTarget).
	out.Area = det.Target.AnyNumber()
	if det.Target.Kind == rules.TargetArea {
		out.AreaShape, out.AreaSizeFt, out.AreaWidthFt = det.Target.Shape, det.Target.SizeFt, det.AreaWidthFt()
		out.SpreadsAroundCorners = det.SpreadsAroundCorners()
	}
	out.CasterOnly = det.Target.CasterOnly()
	out.TargetCount = det.Target.MaxTargets(det.Spell.Level, det.Spell.Level)
	out.TargetPerLevel = det.Target.PerSlotLevel
	out.ExtraTargetPerLevel = det.Target.PerSlotLevel > 0
	if out.CasterOnly {
		// There is nobody else to roll for: a spell that only reaches the caster
		// (Contact Other Plane asks the caster's own save) rolls no saving throw.
		out.SaveAbility, out.SaveOnSuccess, out.SaveDC = "", "", 0
	}
	return out, nil
}

// casterFor picks the Spellcasting of a class that has the spell on its list,
// or the first one; nil for a character that casts nothing. classes are the
// spell's classes (SpellEntry.Classes), and what a caster has on its list is its
// SpellList: its own class, or, for a third caster's subclass (a table Fighter
// that casts from the wizard's list), the class the subclass casts from, so a
// Fighter 5 / Cleric 3 casts a wizard spell with the Fighter's ability and DC,
// not with the Cleric's.
func casterFor(d rules.Derived, classes []string) *rules.Spellcasting {
	for i := range d.Spellcasting {
		list := d.Spellcasting[i].SpellList
		if list == "" {
			list = d.Spellcasting[i].Class
		}
		if slices.Contains(classes, list) {
			return &d.Spellcasting[i]
		}
	}
	if len(d.Spellcasting) > 0 {
		return &d.Spellcasting[0]
	}
	return nil
}

// abilityMod is the ability's modifier on the sheet.
func abilityMod(d rules.Derived, a rules.Ability) int {
	for _, as := range d.Abilities {
		if as.Ability == a {
			return as.Modifier
		}
	}
	return 0
}

// CombatSave implements play.CombatRoster: a creature's saving throw bonus
// against an ability. A full sheet has all six, and so does a basic sheet made
// from a creature (the stat block's); any other basic-sheet NPC has none, so the
// bonus is 0 and Known is false, and the log says so. `not_found` for a
// character that is not one of the campaign's living ones.
func (s *Service) CombatSave(ctx context.Context, tx pgx.Tx, campaignID, characterID, ability string) (link.Save, error) {
	_, d, _, err := s.fighter(ctx, tx, campaignID, characterID)
	if err != nil {
		return link.Save{}, err
	}
	for _, st := range d.SavingThrows {
		if string(st.Ability) == ability {
			return link.Save{Bonus: st.Bonus, Known: true}, nil
		}
	}
	return link.Save{}, nil // a basic sheet: no saving throws derived
}

// MarkDead implements play.CombatRoster: the master's confirmation of a death
// in a combat (RN-03) is MarkCharacterDead's effect, inside the combat's
// transaction: the character changes status, never row, and its player may
// create another. It is idempotent, as MarkCharacterDead is.
func (s *Service) MarkDead(ctx context.Context, tx pgx.Tx, campaignID, characterID string, at time.Time) error {
	id, ok := parseUUID(characterID)
	if !ok {
		return errCharacterNotFound()
	}
	if _, err := s.queries.WithTx(tx).MarkCharacterDead(ctx, charactersdb.MarkCharacterDeadParams{CampaignID: campaignID, ID: id, Now: at}); err != nil {
		return wrap("mark dead", err)
	}
	return nil
}

// Conditions implements play.CombatRoster: the SRD's conditions the master may
// mark as labels (RN-22), with their Portuguese names. They are the SRD's: no
// table content adds one (plan D1), so this needs no campaign.
func (s *Service) Conditions() []link.Named {
	var out []link.Named
	for _, c := range s.srd.Conditions() {
		out = append(out, link.Named{Key: c.Key, NamePT: c.NamePT})
	}
	return out
}

// ContentNames implements play.CombatRoster: the Portuguese name of a content
// key ("" for an unknown one), bound to the campaign's content, which is read
// once. The combat shows names for spells (the one a character concentrates on,
// Escudo) and Wild Shape beasts.
func (s *Service) ContentNames(ctx context.Context, tx pgx.Tx, campaignID string) (func(key string) string, error) {
	content, err := s.contentFor(ctx, tx, campaignID)
	if err != nil {
		return nil, wrap("read rules content", err)
	}
	return content.NamePT, nil
}
