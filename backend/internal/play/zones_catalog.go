package play

import (
	"encoding/json"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// The spells that keep an area on the map while they last (SRD 5.1): Spirit Guardians, Moonbeam,
// Cloudkill, Web, Grease and Entangle. The cast puts a zone on the map (the spell_zones table); what the zone does,
// where it stands and how it moves is written here, one row for each spell, in our own words:
//
//   - Spirit Guardians: an aura around the caster that goes where the caster goes. A creature that
//     comes into it or begins its turn in it makes a Wisdom saving throw and takes radiant damage
//     (half on a pass). The table spares the caster and the caster's allies: the spell lets the
//     caster name who is not affected, and this is the usual choice.
//   - Moonbeam: a narrow column of light at a point. Whoever enters it or begins a turn in it makes a
//     Constitution save for radiant damage (half on a pass). The caster can spend an action on a later
//     turn to move it.
//   - Cloudkill: a fog sphere at a point, which creeps away from the caster by two squares at the start
//     of each of the caster's turns. A Constitution save for poison damage, half on a pass.
//   - Web: a cube of sticky strands at a point. The cast puts its effect on who is inside, as it always
//     did; the zone adds who comes in later: a Dexterity save on entering or beginning a turn there, and
//     a failure restrains.
//   - Grease: a 10-foot square at a point, with no concentration. Who stands in it at the cast, comes in
//     or ends a turn there makes a Dexterity save or falls prone.
//   - Entangle: a 20-foot square at a point. Only who stands in it at the cast makes a Strength save,
//     and a failure restrains until the spell ends.
//
// A creature is hit at most once on a turn by the same zone, whether it came in or began the turn
// inside. For the first four the cast itself hits nobody: a creature standing in the area when the spell is cast is hit
// when it begins its turn there or comes back in.

// zoneKind says what a zone does to the creature it catches.
type zoneKind int

const (
	// zoneDamage: a saving throw, damage on a failure and half of it on a pass.
	zoneDamage zoneKind = iota
	// zoneRestrain: a saving throw; a failure restrains the creature (Web, Entangle).
	zoneRestrain
	// zoneProne: a saving throw; a failure knocks the creature prone (Grease).
	zoneProne
)

// zoneDef is what a spell's zone is.
type zoneDef struct {
	kind  zoneKind
	shape grid.Shape
	// sizeFt is the radius of a sphere or a cylinder, the side of a cube.
	sizeFt int
	// placement is how the cast puts the area on the map.
	placement playv1.AreaPlacement
	// movement is how the zone changes place, if it does.
	movement playv1.SpellZoneMovement
	// save is the ability of the saving throw.
	save string
	// sparesAllies says the zone does not touch the caster or the caster's allies.
	sparesAllies bool
	// dice and dtype are the damage of a zone whose spell does not give it as a table the cast can read (the
	// zone of a spell with no saving throw: the damage just lands); empty: the cast's own.
	dice, dtype string
	// onEnter and onStart say when the zone catches a creature: when it comes in the first time on a turn,
	// when it starts its turn inside.
	onEnter, onStart bool
	// onCast says the zone catches the creatures standing in the area when the spell is cast, and onTurnEnd the
	// creature that ends its turn inside (Grease).
	onCast, onTurnEnd bool
	// restrainKey is the effect a failed save puts on the creature of a restraining zone.
	restrainKey string
	// slows says a creature the zone catches moves at half speed for the turn (Spirit Guardians), and
	// difficult says the squares are difficult terrain (Web, Grease, Entangle).
	slows, difficult bool
	// corners says the area spreads around corners: it is the part of the shape the origin reaches.
	corners bool
	// keepsCastHits says the cast still hits the creatures inside the area when the spell is cast (the
	// effect Web puts on them).
	keepsCastHits bool
}

// The sizes the spells give their areas, in feet.
const (
	spiritGuardiansFt = 15
	moonbeamFt        = 5
	cloudkillFt       = 20
	webFt             = 20
	greaseFt          = 10
	entangleFt        = 20
)

// moveRangeFt is how far the caster moves an area with an action (Moonbeam), and driftFt how far a
// drifting area goes at the start of each of the caster's turns (Cloudkill).
const (
	moveRangeFt = 60
	driftFt     = 10
)

var zoneDefs = map[string]zoneDef{
	"spell:spirit-guardians": {
		kind: zoneDamage, onEnter: true, onStart: true, shape: grid.ShapeSphere, sizeFt: spiritGuardiansFt, save: "wis", sparesAllies: true, slows: true,
		placement: playv1.AreaPlacement_AREA_PLACEMENT_CASTER, movement: playv1.SpellZoneMovement_SPELL_ZONE_MOVEMENT_FOLLOWS_CASTER,
	},
	"spell:moonbeam": {
		kind: zoneDamage, onEnter: true, onStart: true, shape: grid.ShapeCylinder, sizeFt: moonbeamFt, save: "con",
		placement: playv1.AreaPlacement_AREA_PLACEMENT_POINT, movement: playv1.SpellZoneMovement_SPELL_ZONE_MOVEMENT_ACTION,
	},
	"spell:cloudkill": {
		kind: zoneDamage, onEnter: true, onStart: true, shape: grid.ShapeSphere, sizeFt: cloudkillFt, save: "con", corners: true,
		placement: playv1.AreaPlacement_AREA_PLACEMENT_POINT, movement: playv1.SpellZoneMovement_SPELL_ZONE_MOVEMENT_DRIFTS,
	},
	"spell:web": {
		kind: zoneRestrain, onEnter: true, onStart: true, shape: grid.ShapeCube, sizeFt: webFt, save: "dex", keepsCastHits: true, difficult: true,
		restrainKey: webRestrainedKey,
		placement:   playv1.AreaPlacement_AREA_PLACEMENT_POINT, movement: playv1.SpellZoneMovement_SPELL_ZONE_MOVEMENT_FIXED,
	},
	"spell:grease": {
		kind: zoneProne, onCast: true, onEnter: true, onTurnEnd: true, shape: grid.ShapeCube, sizeFt: greaseFt, save: "dex", difficult: true,
		placement: playv1.AreaPlacement_AREA_PLACEMENT_POINT, movement: playv1.SpellZoneMovement_SPELL_ZONE_MOVEMENT_FIXED,
	},
	"spell:entangle": {
		kind: zoneRestrain, onCast: true, shape: grid.ShapeCube, sizeFt: entangleFt, save: "str", difficult: true, restrainKey: entangleRestrainedKey,
		placement: playv1.AreaPlacement_AREA_PLACEMENT_POINT, movement: playv1.SpellZoneMovement_SPELL_ZONE_MOVEMENT_FIXED,
	},
}

// zoneDefOf is the zone one of the SRD's six spells keeps.
func zoneDefOf(spellKey string) (zoneDef, bool) {
	d, ok := zoneDefs[spellKey]
	return d, ok
}

// zoneDefOfSpell is the zone a spell keeps, if it keeps one: the row of the SRD's six, or the table spell's own
// area, which the master wrote as the model says. Both are the same value, so the zone is made and worked the
// same way from either.
func zoneDefOfSpell(sp link.Spell) (zoneDef, bool) {
	if d, ok := zoneDefOf(sp.Key); ok {
		return d, true
	}
	if sp.Zone == nil {
		return zoneDef{}, false
	}
	return zoneDefOfRule(sp), true
}

// zoneDefOfRule is the definition of a table spell's zone: the spell's area, saving throw and damage, and the
// trigger and the following the master chose.
func zoneDefOfRule(sp link.Spell) zoneDef {
	d := zoneDef{
		kind: zoneDamage, shape: grid.Shape(sp.AreaShape), sizeFt: sp.AreaSizeFt, save: sp.SaveAbility, corners: sp.SpreadsAroundCorners,
		onStart: sp.Zone.Trigger != rules.ZoneFirstEntry, onEnter: sp.Zone.Trigger != rules.ZoneStartOfTurn,
		placement: playv1.AreaPlacement_AREA_PLACEMENT_POINT, movement: playv1.SpellZoneMovement_SPELL_ZONE_MOVEMENT_FIXED,
	}
	if sp.Zone.FollowsCaster {
		d.placement, d.movement = playv1.AreaPlacement_AREA_PLACEMENT_CASTER, playv1.SpellZoneMovement_SPELL_ZONE_MOVEMENT_FOLLOWS_CASTER
	}
	return d
}

// zoneDefJSON is what a zone row keeps of the definition of a table spell's zone.
type zoneDefJSON struct {
	Shape     string `json:"shape"`
	SizeFt    int    `json:"size_ft"`
	Placement int32  `json:"placement"`
	Movement  int32  `json:"movement"`
	Save      string `json:"save,omitempty"`
	OnEnter   bool   `json:"on_enter,omitempty"`
	OnStart   bool   `json:"on_start,omitempty"`
	Corners   bool   `json:"corners,omitempty"`
}

// storedDef is the definition as the zone row keeps it, nil for one of the SRD's six (the code's table is its truth).
func storedDef(spellKey string, d zoneDef) []byte {
	if _, srd := zoneDefOf(spellKey); srd {
		return nil
	}
	body, _ := json.Marshal(zoneDefJSON{ // plain values: they cannot fail to encode
		Shape: string(d.shape), SizeFt: d.sizeFt, Placement: int32(d.placement), Movement: int32(d.movement), Save: d.save,
		OnEnter: d.onEnter, OnStart: d.onStart, Corners: d.corners,
	})
	return body
}

// defOfStored reads the definition a zone row keeps.
func defOfStored(raw []byte) (zoneDef, bool) {
	var j zoneDefJSON
	if json.Unmarshal(raw, &j) != nil || j.Shape == "" {
		return zoneDef{}, false
	}
	return zoneDef{
		kind: zoneDamage, shape: grid.Shape(j.Shape), sizeFt: j.SizeFt, placement: playv1.AreaPlacement(j.Placement),
		movement: playv1.SpellZoneMovement(j.Movement), save: j.Save, onEnter: j.OnEnter, onStart: j.OnStart, corners: j.Corners,
	}, true
}

// area is the zone's shape and size for the geometry of the map.
func (d zoneDef) area() grid.Area { return grid.Area{Shape: d.shape, SizeFt: d.sizeFt} }

// notes are the keys of what else the zone does, for the screens.
func (d zoneDef) notes() []string {
	var out []string
	if d.slows {
		out = append(out, "speed_halved")
	}
	if d.difficult {
		out = append(out, "difficult_terrain")
	}
	if d.kind == zoneRestrain {
		out = append(out, "restrains")
	}
	return out
}
