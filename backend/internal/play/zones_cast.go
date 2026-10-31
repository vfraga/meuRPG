package play

import (
	"context"
	"fmt"

	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// affectZone is what the cast of a spell that keeps an area does: the area goes on the map as a
// zone. Web also keeps what it always did to the creatures inside (the effect on each), so the
// targets are hit as for any placed area; the other three hit nobody at the cast.
func (f *castFlow) affectZone(ctx context.Context, def zoneDef) error {
	if def.keepsCastHits {
		for i, t := range f.targs {
			if err := f.hitTarget(ctx, i, t); err != nil {
				return err
			}
		}
		if err := f.s.applyLastingSpell(ctx, f.c, f.cs, f.sp, f.slotLevel, f.caster, f.targs, f.abilityKey, &f.out.made); err != nil {
			return err
		}
	}
	z, err := f.s.createZone(ctx, f.c, f.caster, f.sp, f.slotLevel, f.out.made.CastID, f.plan)
	if err != nil || z == nil {
		return err
	}
	return f.s.catchWhoIsInside(ctx, f.c, z, &f.out.made)
}

// catchWhoIsInside catches each creature standing in the area of a zone that has just been cast, when the zone
// asks for it (Grease and Entangle): the ones that come in later are the business of the other triggers.
func (s *Service) catchWhoIsInside(ctx context.Context, c *combatTx, z *zone, made *actionEvent) error {
	if !z.def.onCast {
		return nil
	}
	cs, err := listCombatants(ctx, c)
	if err != nil {
		return err
	}
	mark := currentMark(c) + ":cast"
	for _, who := range cs {
		if !placed(who) || !z.has(squareOfCombatant(who)) {
			continue
		}
		if err := s.catchByID(ctx, c, zoneCatch{z: z, why: zoneWhyCast, mark: mark, round: c.enc.Round, made: made}, who.ID); err != nil {
			return err
		}
	}
	return nil
}

// createZone puts the area of the cast on the map. It runs after the concentration is set, so the zone
// the caster held before has already gone with it.
func (s *Service) createZone(ctx context.Context, c *combatTx, owner playdb.Combatant, sp link.Spell, slotLevel int, castID string, plan areaPlan) (*zone, error) {
	def, ok := zoneDefOfSpell(sp)
	if !ok {
		return nil, nil
	}
	content, err := s.contentOf(ctx, c)
	if err != nil {
		return nil, err
	}
	var ends *int32
	if rounds := content.SpellEffectRounds(sp.Key); rounds > 0 {
		ends = new(c.enc.Round + conv.Int32(rounds))
	}
	dice, dtype := zoneDamageOf(sp)
	switch {
	case def.kind != zoneDamage: // no damage: the web's fire is the master's to light
		dice, dtype = "", ""
	case dice == "":
		dice, dtype = def.dice, def.dtype
	}
	dc := sp.CasterDC
	if dc == 0 {
		dc = sp.SaveDC
	}
	row, err := c.q.InsertSpellZone(ctx, playdb.InsertSpellZoneParams{
		EncounterID: c.enc.ID, OwnerID: owner.ID, SpellKey: sp.Key, CastID: castID, SlotLevel: clamp32(slotLevel, 0, 9),
		OriginCol: clamp32(plan.origin.Col, 0, 1<<30), OriginRow: clamp32(plan.origin.Row, 0, 1<<30), Squares: squaresToJSON(plan.squares),
		SaveAbility: def.save, SaveDc: clamp32(dc, 0, 40), DamageDice: dice, DamageType: dtype,
		Concentration: sp.Concentration, StartedRound: c.enc.Round, EndsRound: ends, CreatedAt: c.now, Def: storedDef(sp.Key, def),
	})
	if err != nil {
		return nil, fmt.Errorf("put the spell zone on the map: %w", err)
	}
	z, _ := readZone(row) // the spell's own definition: always known
	return z, nil
}

// zoneDamageOf are the dice and the type of the damage a zone deals, as the cast's slot and the
// caster's pick made them; empty for a zone with no damage.
func zoneDamageOf(sp link.Spell) (dice, dtype string) {
	if len(sp.Damages) == 0 {
		return "", ""
	}
	d := sp.Damages[0]
	return fmt.Sprintf("%dd%d", d.Count, d.Sides), d.DamageType
}

// The reasons a zone's "ended" line gives, besides the duration (endDuration) and the master (endByMaster).
const (
	zoneEndConcentration = "concentration"          // the caster lost the concentration
	zoneEndReplaced      = "concentration_replaced" // the caster began another concentration spell
)

// endZonesOf ends the zones a combatant's concentration holds, with a line for each: why they ended.
func (s *Service) endZonesOf(ctx context.Context, c *combatTx, owner playdb.Combatant, reason string) error {
	rows, err := c.q.DeleteConcentrationZonesOf(ctx, owner.ID)
	if err != nil {
		return fmt.Errorf("end the spell zones: %w", err)
	}
	for _, z := range zonesOfRows(rows) {
		if err := s.closeZoneWindows(ctx, c, z.row.ID, endConcentration); err != nil {
			return err
		}
		// Written now, not held for the end of the change: a concentration also ends outside the changes that
		// flush the lines of the zones (the vitals a character is brought to 0 by).
		if err := insertEvent(ctx, c, eventLastingEnded, &c.actorUserID, nil, zoneEvent(z, c.enc.Round, owner.Hidden, "ended", reason)); err != nil {
			return err
		}
	}
	return nil
}
