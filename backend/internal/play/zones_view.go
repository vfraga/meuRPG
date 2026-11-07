package play

import (
	"math"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// The areas spells keep on the map, as a viewer reads them (Encounter.zones). A player gets the zone
// of a caster they see and no other: the zone of a hidden caster is the master's, who reads it marked
// as such, and nothing of it reaches a player (RN-10). The squares are the zone's own; the map draws them.

// zonesView lists the zones the viewer reads, oldest first.
func (d *encounterData) zonesView(v combatViewer, names func(string) string) []*playv1.SpellZone {
	if d.enc.Status != statusActive || isTheatre(d.enc) {
		return nil
	}
	var out []*playv1.SpellZone
	for _, z := range d.zones {
		owner, ok := combatantByID(d.cs, z.row.OwnerID)
		if !ok || !v.sees(owner) {
			continue
		}
		view := z.view(owner, d.enc.Round, names)
		if v.master || v.owns(owner) {
			z.addDetails(view, names, v.master)
		} else {
			view.DetailsHidden, view.RoundsLeft = true, 0
		}
		view.OccupantIds = z.occupantsFor(d.cs, v)
		out = append(out, view)
	}
	return out
}

// occupantsFor are the creatures standing in the area now that the viewer sees.
func (z *zone) occupantsFor(cs []playdb.Combatant, v combatViewer) []string {
	var out []string
	for _, o := range cs {
		if placed(o) && !o.Defeated && v.sees(o) && z.has(squareOfCombatant(o)) {
			out = append(out, o.ID)
		}
	}
	return out
}

// view is the zone as the API tells it to anyone who sees its caster: the name, the shape, the squares, the
// concentration and how it moves.
func (z *zone) view(owner playdb.Combatant, round int32, names func(string) string) *playv1.SpellZone {
	shape, size := z.def.shape, z.sizeFt()
	if z.wall != nil {
		shape, size = z.wall.shape() // the wall the caster laid, not the spell's largest
	}
	out := &playv1.SpellZone{
		Id: z.row.ID, SpellKey: z.row.SpellKey, OwnerCombatantId: owner.ID, OwnerLabel: owner.Label,
		Shape: shapeProto(shape), SizeFt: clamp32(size, 0, math.MaxInt32),
		Origin: originProto(z.origin()), Squares: squaresProto(z.squares),
		Concentration: z.row.Concentration, RoundsLeft: z.roundsLeft(round), Movement: z.def.movement,
		Visibility: playv1.SpellZoneVisibility_SPELL_ZONE_VISIBILITY_EVERYONE,
	}
	if names != nil {
		out.SpellNamePt = names(z.row.SpellKey)
	}
	if owner.Hidden {
		out.Visibility = playv1.SpellZoneVisibility_SPELL_ZONE_VISIBILITY_MASTER_ONLY
	}
	return out
}

// addDetails adds the numbers of the spell: the master's, and the caster's own player's.
func (z *zone) addDetails(out *playv1.SpellZone, names func(string) string, master bool) {
	out.SaveAbility, out.DamageDice, out.DamageTypeKey = z.row.SaveAbility, z.row.DamageDice, z.row.DamageType
	if z.row.SaveAbility != "" {
		out.SaveDc = new(z.row.SaveDc)
		out.HalfOnSave = z.row.DamageDice != ""
	}
	if z.def.movement == playv1.SpellZoneMovement_SPELL_ZONE_MOVEMENT_DRIFTS {
		out.MovesPerTurnFt = driftFt
	}
	out.Notes = z.def.notes(master)
	if names != nil {
		if z.row.SaveAbility != "" {
			out.SaveAbilityNamePt = names("ability:" + z.row.SaveAbility)
		}
		if z.row.DamageType != "" {
			out.DamageTypeNamePt = names(z.row.DamageType)
		}
	}
}

// shapeProto is the API's name of a grid shape.
func shapeProto(s grid.Shape) rulesv1.SpellAreaShape {
	switch s {
	case grid.ShapeCone:
		return rulesv1.SpellAreaShape_SPELL_AREA_SHAPE_CONE
	case grid.ShapeCube:
		return rulesv1.SpellAreaShape_SPELL_AREA_SHAPE_CUBE
	case grid.ShapeCylinder:
		return rulesv1.SpellAreaShape_SPELL_AREA_SHAPE_CYLINDER
	case grid.ShapeLine:
		return rulesv1.SpellAreaShape_SPELL_AREA_SHAPE_LINE
	case grid.ShapeSphere:
		return rulesv1.SpellAreaShape_SPELL_AREA_SHAPE_SPHERE
	}
	return rulesv1.SpellAreaShape_SPELL_AREA_SHAPE_UNSPECIFIED
}
