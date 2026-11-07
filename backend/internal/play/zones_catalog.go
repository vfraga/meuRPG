package play

import (
	"encoding/json"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// The spells that keep an area on the map while they last (SRD 5.1):
// Spirit Guardians, Moonbeam, Cloudkill, Web, Grease, Entangle, Fog Cloud, Darkness, Stinking Cloud, Silence,
// Spike Growth and Wall of Fire.
// The cast puts a zone on the map (the spell_zones table).
// What the zone does, where it stands and how it moves is written here, one row for each spell, in our own words:
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
//   - Web: a cube of sticky strands at a point. The cast puts its effect on who is inside;
//     the zone adds who comes in later: a Dexterity save on entering or beginning a turn there, and
//     a failure restrains.
//   - Grease: a 10-foot square at a point, with no concentration. Who stands in it at the cast, comes in
//     or ends a turn there makes a Dexterity save or falls prone.
//   - Entangle: a 20-foot square at a point. Only who stands in it at the cast makes a Strength save,
//     and a failure restrains until the spell ends.
//   - Fog Cloud: a sphere of 20 feet of radius at a point, heavily obscured, 20 feet more for each slot above
//     the 1st. It asks nobody; it blocks the sight of the players on a fog map.
//   - Darkness: a sphere of 15 feet of radius at a point, magical darkness. It asks nobody; no light and no
//     darkvision see into it.
//   - Stinking Cloud: a sphere of 20 feet of radius at a point, heavily obscured. A creature whose square is in
//     it at the start of its turn makes a Constitution save, and a failure spends its action.
//   - Silence: a sphere of 20 feet of radius at a point, ruling sound. It asks nobody. A creature whose square is
//     in it is immune to thunder damage and deafened, and a caster there cannot cast a spell with a verbal
//     component (zones_silence.go).
//   - Spike Growth: a cylinder of 20 feet of radius at a point, difficult terrain. It asks no save: a creature
//     that moves through it takes piercing damage for every square it enters inside, on every move.
//   - Wall of Fire: a wall of fire, a line or a ring that the caster lays from a point (zones_wall.go). A Dexterity save
//     at the cast for the creatures inside it, fire damage, half on a pass.
//     Later hits ask no save: a creature that enters the wall, or ends its turn inside it or within 10 feet of its
//     damaging side, takes the damage dice.
//
// A creature is hit at most once on a turn by the same zone, whether it came in or began the turn
// inside.
// A wall hits twice when a creature enters it and ends the turn there.
// Spike Growth has no count: it hits on every move.
// For Spirit Guardians, Moonbeam, Cloudkill and Stinking Cloud the cast itself hits nobody: a creature standing in the area when the spell is cast is hit
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
	// zoneOpen: no save and no trigger; the zone only changes what is seen (Fog Cloud, Darkness) or what is heard
	// (Silence).
	zoneOpen
	// zoneRetch: a saving throw at the start of a turn of a creature whose square is inside; a failure spends its action
	// (Stinking Cloud).
	zoneRetch
	// zoneSpikes: no save; a creature that travels through the zone takes damage for every square it enters (Spike Growth).
	zoneSpikes
	// zoneWall: a wall of squares the caster lays from the point of origin (zones_wall.go). The creatures inside it make a
	// saving throw for damage at the cast, half on a pass (Wall of Fire).
	zoneWall
)

// zoneSight says how a zone changes what is seen on the map.
type zoneSight int

const (
	sightNone zoneSight = iota
	// sightCloud: heavily obscured; a line that crosses it is blocked (Fog Cloud, Stinking Cloud).
	sightCloud
	// sightDark: magical darkness; light does not light it and darkvision does not see in it (Darkness).
	sightDark
)

// zoneDef is what a spell's zone is.
type zoneDef struct {
	kind  zoneKind
	shape grid.Shape
	// sizeFt is the radius of a sphere or a cylinder, the side of a cube, the length of a line.
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
	// creature that ends its turn inside (Grease) or, for a wall, within 10 feet of its damaging side (Wall of Fire).
	onCast, onTurnEnd bool
	// sight is what the zone does to what is seen (rules/vision).
	sight zoneSight
	// growsFt is how much the size grows for each slot level above the spell's own (Fog Cloud).
	growsFt int
	// level is the spell's own level: the slot of that level adds nothing, and each level above it adds growsFt.
	level int
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
	// silences says the creatures inside the zone are deafened and immune to thunder damage, and that no spell with a
	// verbal component is cast from inside it (Silence).
	silences bool
	// call is the key of the master's call the spell leaves to the table (a line on the master's zone card).
	call string
}

// The sizes the spells give their areas, in feet.
const (
	spiritGuardiansFt = 15
	moonbeamFt        = 5
	cloudkillFt       = 20
	webFt             = 20
	greaseFt          = 10
	entangleFt        = 20
	fogCloudFt        = 20
	darknessFt        = 15
	stinkingCloudFt   = 20
	silenceFt         = 20
	spikeGrowthFt     = 20
)

// The damage Spike Growth deals for each square a creature enters in its area.
const (
	spikeDice   = "2d4"
	spikeDamage = "damage-type:piercing"
)

// moveRangeFt is how far the caster moves an area with an action (Moonbeam), and driftFt how far a
// drifting area goes at the start of each of the caster's turns (Cloudkill).
const (
	moveRangeFt = 60
	driftFt     = 10
)

var zoneDefs = map[string]zoneDef{
	"spell:fog-cloud": {
		kind: zoneOpen, shape: grid.ShapeSphere, sizeFt: fogCloudFt, growsFt: fogCloudFt, level: 1, corners: true, sight: sightCloud, call: "master_wind",
		placement: playv1.AreaPlacement_AREA_PLACEMENT_POINT, movement: playv1.SpellZoneMovement_SPELL_ZONE_MOVEMENT_FIXED,
	},
	"spell:darkness": {
		kind: zoneOpen, shape: grid.ShapeSphere, sizeFt: darknessFt, corners: true, sight: sightDark, call: "master_light",
		placement: playv1.AreaPlacement_AREA_PLACEMENT_POINT, movement: playv1.SpellZoneMovement_SPELL_ZONE_MOVEMENT_FIXED,
	},
	"spell:stinking-cloud": {
		kind: zoneRetch, onStart: true, shape: grid.ShapeSphere, sizeFt: stinkingCloudFt, save: "con", corners: true, sight: sightCloud, call: "master_breath",
		placement: playv1.AreaPlacement_AREA_PLACEMENT_POINT, movement: playv1.SpellZoneMovement_SPELL_ZONE_MOVEMENT_FIXED,
	},
	"spell:spike-growth": {
		kind: zoneSpikes, shape: grid.ShapeCylinder, sizeFt: spikeGrowthFt, difficult: true, dice: spikeDice, dtype: spikeDamage,
		placement: playv1.AreaPlacement_AREA_PLACEMENT_POINT, movement: playv1.SpellZoneMovement_SPELL_ZONE_MOVEMENT_FIXED,
	},
	"spell:silence": {
		kind: zoneOpen, shape: grid.ShapeSphere, sizeFt: silenceFt, silences: true,
		placement: playv1.AreaPlacement_AREA_PLACEMENT_POINT, movement: playv1.SpellZoneMovement_SPELL_ZONE_MOVEMENT_FIXED,
	},
	"spell:wall-of-fire": {
		kind: zoneWall, onCast: true, onEnter: true, onTurnEnd: true, shape: grid.ShapeLine, sizeFt: wallLineFt, save: "dex", sight: sightCloud,
		placement: playv1.AreaPlacement_AREA_PLACEMENT_POINT, movement: playv1.SpellZoneMovement_SPELL_ZONE_MOVEMENT_FIXED,
	},
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

// zoneDefOf is the zone one of the twelve SRD spells keeps.
func zoneDefOf(spellKey string) (zoneDef, bool) {
	d, ok := zoneDefs[spellKey]
	return d, ok
}

// zoneDefOfSpell is the zone a spell keeps, if it keeps one: the row of the twelve SRD spells, or the table spell's own
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

// storedDef is the definition as the zone row keeps it, nil for one of the twelve SRD spells (the code's table is its truth).
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

// burns says the zone deals the damage of its spell to a creature it catches, half of it on a passed save.
func (d zoneDef) burns() bool { return d.kind == zoneDamage || d.kind == zoneWall }

// area is the zone's shape and size for the geometry of the map.
func (d zoneDef) area() grid.Area { return grid.Area{Shape: d.shape, SizeFt: d.sizeFt} }

// sizeAt is the size of the zone cast with a slot: the catalog's, plus growsFt for each level above the spell's own.
func (d zoneDef) sizeAt(spellLevel, slotLevel int) int {
	return d.sizeFt + d.growsFt*max(slotLevel-spellLevel, 0)
}

// withZoneSize gives the spell the size its zone has at the slot of the cast.
func withZoneSize(sp link.Spell, slotLevel int) link.Spell {
	if d, ok := zoneDefOfSpell(sp); ok && d.growsFt > 0 {
		sp.ZoneSizeFt = d.sizeAt(sp.Level, slotLevel)
	}
	return sp
}

// notes are the keys of what else the zone does, for the screens. The master's calls are the master's alone: the caster's
// player gets the other notes and no call.
func (d zoneDef) notes(master bool) []string {
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
	if d.call != "" && master {
		out = append(out, d.call)
	}
	return out
}
