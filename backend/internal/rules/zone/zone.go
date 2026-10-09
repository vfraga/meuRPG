// Package zone is what a spell leaves on the map after it is cast (SRD 5.1,
// "Areas of Effect", "Vision and Light", and the text of each spell): an area that
// lasts, blocks or dims sight, slows creatures down and hurts or hinders the ones
// that enter it or spend a turn in it. It is pure, like package rules (ADR-0008):
// the catalog of the twelve spells that leave a zone, the shapes they take on the
// grid and the questions a zone answers (who is inside, what a move through it
// costs, who sees through it). The play module keeps the zones of a combat and
// runs their triggers.
//
// The catalog follows each spell's SRD text, with two readings of the app where the
// text is silent, written where they apply: a zone that moves onto a creature counts
// as the creature entering it, and a heavily obscured area hides symmetrically (a
// creature inside it sees only itself, and nobody sees it from outside).
package zone

import (
	"slices"

	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// Obscurity is how a zone affects sight (SRD, "Vision and Light").
type Obscurity string

// The obscurities a zone can have.
const (
	// ObscureNone: the zone does not touch sight.
	ObscureNone Obscurity = ""
	// ObscureLight: lightly obscured (Web: "lightly obscure their area"; Moonbeam's
	// dim light). It blocks nothing.
	ObscureLight Obscurity = "light"
	// ObscureHeavy: heavily obscured. Sight is blocked by it: a creature trying to
	// see something there is effectively blinded.
	ObscureHeavy Obscurity = "heavy"
	// ObscureDark: magical darkness (Darkness). Darkvision does not see through it and
	// nonmagical light does not light it.
	ObscureDark Obscurity = "dark"
	// ObscureOpaque: an opaque wall (Wall of Fire).
	ObscureOpaque Obscurity = "opaque"
)

// Valid says whether it is one of the obscurities.
func (o Obscurity) Valid() bool {
	switch o {
	case ObscureNone, ObscureLight, ObscureHeavy, ObscureDark, ObscureOpaque:
		return true
	}
	return false
}

// BlocksSight says nobody sees into, out of or through the zone (the app's reading
// of "heavily obscured", symmetric): heavy, magical dark and opaque.
func (o Obscurity) BlocksSight() bool {
	return o == ObscureHeavy || o == ObscureDark || o == ObscureOpaque
}

// Placement is how the caster sets a zone on the map.
type Placement string

// The placements.
const (
	// PlacementPoint: the zone is centered on a point within range (a sphere, a
	// cylinder, a ring, a cube).
	PlacementPoint Placement = "point"
	// PlacementPointDirection: a wall begins at a point and runs in a direction.
	PlacementPointDirection Placement = "point_direction"
	// PlacementCaster: the zone is centered on the caster and goes where it goes.
	PlacementCaster Placement = "caster"
)

// TriggerKind says when a zone does something to a creature.
type TriggerKind string

// The triggers, as each spell's text says them.
const (
	// AtCast: a creature in the area when the spell is cast.
	AtCast TriggerKind = "at_cast"
	// OnEnter: every time a creature enters the area (Web, Grease: the SRD has no limit
	// to the first time on a turn).
	OnEnter TriggerKind = "on_enter"
	// OnEnterFirstTimeOnATurn: "enters the area for the first time on a turn". It shares
	// one damage a turn with StartOfTurn: a creature that entered and then starts its turn
	// there is hurt once for each turn (the ledger's key is the creature, the zone, the
	// round and whose turn it is).
	OnEnterFirstTimeOnATurn TriggerKind = "on_enter_first_time_on_a_turn"
	// StartOfTurn: a creature starts its turn in the area.
	StartOfTurn TriggerKind = "start_of_turn"
	// EndOfTurn: a creature ends its turn in the area (Grease).
	EndOfTurn TriggerKind = "end_of_turn"
	// EndOfTurnWithin: a creature ends its turn inside the wall or within the reach of the
	// damaging side of it (Wall of Fire, 10 ft).
	EndOfTurnWithin TriggerKind = "end_of_turn_within"
	// PerDistanceMoved: damage for every 5 feet a creature travels in the area, forced
	// movement included (Spike Growth).
	PerDistanceMoved TriggerKind = "per_distance_moved"
)

// Valid says whether it is one of the kinds.
func (k TriggerKind) Valid() bool {
	switch k {
	case AtCast, OnEnter, OnEnterFirstTimeOnATurn, StartOfTurn, EndOfTurn, EndOfTurnWithin, PerDistanceMoved:
		return true
	}
	return false
}

// Fail is what a failed saving throw does besides the damage.
type Fail string

// The effects of a failed save.
const (
	FailNone       Fail = ""
	FailRestrained Fail = "restrained"  // Web, Entangle: "condition:restrained"
	FailProne      Fail = "prone"       // Grease: falls prone, "condition:prone"
	FailLoseAction Fail = "lose_action" // Stinking Cloud: spends its action retching and reeling
)

// OnSuccess is what a successful saving throw leaves.
type OnSuccess string

// The effects of a successful save.
const (
	SuccessNone OnSuccess = "none"
	SuccessHalf OnSuccess = "half"
)

// Trigger is one thing a zone does. It carries the rule of the spell's text, not the
// numbers of one cast: the save's DC and the damage dice are the cast's, and the zone
// stores them beside the trigger.
type Trigger struct {
	Kind TriggerKind `json:"kind"`
	// RequiresFullyInside: only a creature entirely inside the area counts (Stinking
	// Cloud: "completely within the cloud"; Silence). A creature larger than a square is
	// inside when any of its squares is, and entirely inside when all are.
	RequiresFullyInside bool `json:"requires_fully_inside,omitempty"`
	// Save is the saving throw's ability ("dex", "con", "wis", "str"); empty when the
	// trigger has none (Wall of Fire's burn, Spike Growth).
	Save string `json:"save,omitempty"`
	// Damage says the trigger deals the spell's damage (at the slot level of the cast).
	Damage bool `json:"damage,omitempty"`
	// OnSuccess is what a successful save leaves of the damage: "half" or "none".
	OnSuccess OnSuccess `json:"on_success,omitempty"`
	// OnFail is what a failed save does besides the damage.
	OnFail Fail `json:"on_fail,omitempty"`
}

// Rule is a rule of a zone that is not a trigger: no save, nothing to roll.
type Rule string

// The rules.
const (
	// NoVerbalCast: casting a spell with a verbal component is impossible inside.
	NoVerbalCast Rule = "no_verbal_cast"
	// Deaf: a creature entirely inside is deafened.
	Deaf Rule = "deaf"
	// ThunderImmune: a creature or object entirely inside is immune to thunder damage.
	ThunderImmune Rule = "thunder_immune"
)

// Valid says whether it is one of the rules.
func (r Rule) Valid() bool { return r == NoVerbalCast || r == Deaf || r == ThunderImmune }

// MovesWith says how a zone goes about the map.
type MovesWith string

// The ways a zone moves.
const (
	// MovesNone: the zone stays where it is (the caster can still move it, when the
	// spell says so: Moonbeam).
	MovesNone MovesWith = ""
	// MovesWithCaster: the zone is centered on the caster and goes with it (Spirit
	// Guardians).
	MovesWithCaster MovesWith = "caster"
	// MovesAtTurnStart: the zone walks by itself at the start of the caster's turn,
	// away from the caster (Cloudkill: 10 ft).
	MovesAtTurnStart MovesWith = "self_at_turn_start"
)

// Wind is the strength of a wind that disperses a zone.
type Wind string

// The winds the SRD names: moderate is at least 10 miles an hour (16 km/h), strong at
// least 20 (32 km/h).
const (
	WindModerate Wind = "moderate"
	WindStrong   Wind = "strong"
)

// Valid says whether it is a wind.
func (w Wind) Valid() bool { return w == WindModerate || w == WindStrong }

// Dispersal says in how many rounds a wind disperses the zone; absent when that wind
// does not.
type Dispersal map[Wind]int

// Spec is a spell that leaves a zone.
type Spec struct {
	// Key is the spell's key ("spell:web").
	Key string
	// Level is the spell's own level; SizeFt grows by GrowthFt for each slot level above it.
	Level int
	// Shape is the form of the zone. SizeFt is the radius of a sphere and a cylinder,
	// the side of a cube, the length of a wall and the diameter of a ring, in feet, at
	// the spell's own level.
	Shape  grid.Shape
	SizeFt int
	// GrowthFt is how much SizeFt grows with each slot level above the spell's own
	// (Fog Cloud: the radius grows by 20 ft).
	GrowthFt int
	// Placement is how the caster sets it.
	Placement Placement
	// DurationRounds is how long it lasts, in rounds of 6 seconds: 10 for a minute, 100
	// for ten minutes, 600 for an hour.
	DurationRounds int
	// Concentration: the zone ends with the caster's concentration.
	Concentration bool
	// Obscurity is the zone's effect on sight; Difficult makes it difficult terrain; HalvesSpeed
	// halves the speed of a creature inside (Spirit Guardians).
	Obscurity   Obscurity
	Difficult   bool
	HalvesSpeed bool
	// SpreadsAroundCorners: the zone is the part of the shape connected to its point (the
	// spell "spreads around corners").
	SpreadsAroundCorners bool
	// Camouflaged: the zone looks like the ground until a creature recognises it (Spike
	// Growth): a player does not see it, or its cost, until then.
	Camouflaged bool
	// Visible: the zone is shown to the players by default. A zone with no visible form
	// (Silence) is not.
	Visible bool
	// Triggers and Rules are what it does.
	Triggers []Trigger
	Rules    []Rule
	// Moves is how it goes about the map; StepSquares is how far MovesAtTurnStart walks.
	Moves       MovesWith
	StepSquares int
	// CasterMoves says an action of the caster moves the zone (Moonbeam), at most
	// MoveFt feet.
	CasterMoves bool
	MoveFt      int
	// Excludes: the caster designates the creatures it sees that the spell does not affect
	// (Spirit Guardians).
	Excludes bool
	// Anchors: the caster says whether the zone is anchored (Web): one that is not
	// collapses and ends at the start of the caster's next turn.
	Anchors bool
	// Sides: the wall has a damaging side, chosen at the cast (Wall of Fire), and the
	// damage reaches ReachFt feet from it.
	Sides   bool
	ReachFt int
	// Dispersal is what winds do to it.
	Dispersal Dispersal
}

// Sized returns the size in feet at a slot level, never below the spell's own.
func (s Spec) Sized(slotLevel int) int {
	return s.SizeFt + s.GrowthFt*max(slotLevel-s.Level, 0)
}

// Trigger returns the trigger of the kind, and false when the zone has none.
func (s Spec) Trigger(kind TriggerKind) (Trigger, bool) {
	i := slices.IndexFunc(s.Triggers, func(t Trigger) bool { return t.Kind == kind })
	if i < 0 {
		return Trigger{}, false
	}
	return s.Triggers[i], true
}

// HasRule says whether the zone has the rule.
func (s Spec) HasRule(r Rule) bool { return slices.Contains(s.Rules, r) }

const (
	minute = 10
	tenMin = 100
	hour   = 600
)

// specs is the catalog, in the SRD's own words: every number is from the spell's text
// (Areas of Effect for the shapes, a foot being a fifth of a square).
var specs = []Spec{
	{
		// "You create a 20-foot-radius sphere of fog centered on a point within range. The
		// sphere spreads around corners, and its area is heavily obscured. It lasts for the
		// duration or until a wind of moderate or greater speed ... disperses it." The radius
		// grows by 20 feet for each slot level above 1st.
		Key: "spell:fog-cloud", Level: 1, Shape: grid.ShapeSphere, SizeFt: 20, GrowthFt: 20, Placement: PlacementPoint,
		DurationRounds: hour, Concentration: true, Obscurity: ObscureHeavy, SpreadsAroundCorners: true, Visible: true,
		Dispersal: Dispersal{WindModerate: 0, WindStrong: 0},
	},
	{
		// "The webs fill a 20-foot cube from that point ... difficult terrain and lightly
		// obscure their area ... Each creature that starts its turn in the webs or that
		// enters them during its turn must make a dexterity saving throw. On a failed save,
		// the creature is restrained".
		Key: "spell:web", Level: 2, Shape: grid.ShapeCube, SizeFt: 20, Placement: PlacementPoint,
		DurationRounds: hour, Concentration: true, Obscurity: ObscureLight, Difficult: true, Visible: true, Anchors: true,
		Triggers: []Trigger{
			{Kind: StartOfTurn, Save: "dex", OnSuccess: SuccessNone, OnFail: FailRestrained},
			{Kind: OnEnter, Save: "dex", OnSuccess: SuccessNone, OnFail: FailRestrained},
		},
	},
	{
		// "a wall up to 60 feet long, 20 feet high, and 1 foot thick, or a ringed wall up to
		// 20 feet in diameter ... The wall is opaque ... When the wall appears, each creature
		// within its area must make a Dexterity saving throw. On a failed save, a creature
		// takes 5d8 fire damage, or half ... One side of the wall ... deals 5d8 fire damage to
		// each creature that ends its turn within 10 feet of that side or inside the wall. A
		// creature takes the same damage when it enters the wall for the first time on a turn
		// or ends its turn there."
		Key: "spell:wall-of-fire", Level: 4, Shape: grid.ShapeWall, SizeFt: 60, Placement: PlacementPointDirection,
		DurationRounds: minute, Concentration: true, Obscurity: ObscureOpaque, Visible: true, Sides: true, ReachFt: 10,
		Triggers: []Trigger{
			{Kind: AtCast, Save: "dex", Damage: true, OnSuccess: SuccessHalf},
			{Kind: EndOfTurnWithin, Damage: true},
			{Kind: OnEnterFirstTimeOnATurn, Damage: true},
		},
	},
	{
		// "The ground in a 20-foot radius ... becomes difficult terrain ... When a creature
		// moves into or within the area, it takes 2d4 piercing damage for every 5 feet it
		// travels. The transformation of the ground is camouflaged".
		Key: "spell:spike-growth", Level: 2, Shape: grid.ShapeCylinder, SizeFt: 20, Placement: PlacementPoint,
		DurationRounds: tenMin, Concentration: true, Difficult: true, Camouflaged: true, Visible: true,
		Triggers: []Trigger{{Kind: PerDistanceMoved, Damage: true}},
	},
	{
		// "Magical darkness spreads from a point you choose within range to fill a 15-foot-radius
		// sphere ... spreads around corners. A creature with darkvision can't see through this
		// darkness, and nonmagical light can't illuminate it."
		Key: "spell:darkness", Level: 2, Shape: grid.ShapeSphere, SizeFt: 15, Placement: PlacementPoint,
		DurationRounds: tenMin, Concentration: true, Obscurity: ObscureDark, SpreadsAroundCorners: true, Visible: true,
	},
	{
		// "a 20-foot-radius sphere of yellow, nauseating gas ... spreads around corners, and
		// its area is heavily obscured ... Each creature that is completely within the cloud at
		// the start of its turn must make a constitution saving throw against poison. On a
		// failed save, the creature spends its action that turn retching and reeling.
		// Creatures that don't need to breathe or are immune to poison automatically succeed.
		// A moderate wind disperses the cloud after 4 rounds. A strong wind ... after 1 round."
		Key: "spell:stinking-cloud", Level: 3, Shape: grid.ShapeSphere, SizeFt: 20, Placement: PlacementPoint,
		DurationRounds: minute, Concentration: true, Obscurity: ObscureHeavy, SpreadsAroundCorners: true, Visible: true,
		Triggers:  []Trigger{{Kind: StartOfTurn, RequiresFullyInside: true, Save: "con", OnSuccess: SuccessNone, OnFail: FailLoseAction}},
		Dispersal: Dispersal{WindModerate: 4, WindStrong: 1},
	},
	{
		// "a 20-foot-radius sphere of poisonous, yellow-green fog ... spreads around corners ...
		// until strong wind disperses the fog ... heavily obscured. When a creature enters the
		// spell's area for the first time on a turn or starts its turn there, that creature must
		// make a constitution saving throw ... 5d8 poison damage on a failed save, or half ...
		// The fog moves 10 feet away from you at the start of each of your turns".
		Key: "spell:cloudkill", Level: 5, Shape: grid.ShapeSphere, SizeFt: 20, Placement: PlacementPoint,
		DurationRounds: tenMin, Concentration: true, Obscurity: ObscureHeavy, SpreadsAroundCorners: true, Visible: true,
		Triggers: []Trigger{
			{Kind: OnEnterFirstTimeOnATurn, Save: "con", Damage: true, OnSuccess: SuccessHalf},
			{Kind: StartOfTurn, Save: "con", Damage: true, OnSuccess: SuccessHalf},
		},
		Moves: MovesAtTurnStart, StepSquares: 2, Dispersal: Dispersal{WindStrong: 0},
	},
	{
		// "no sound can be created within or pass through a 20-foot-radius sphere ... Any
		// creature or object entirely inside the sphere is immune to thunder damage, and
		// creatures are deafened while entirely inside it. Casting a spell that includes a
		// verbal component is impossible there." It has no visible form.
		Key: "spell:silence", Level: 2, Shape: grid.ShapeSphere, SizeFt: 20, Placement: PlacementPoint,
		DurationRounds: tenMin, Concentration: true,
		Rules: []Rule{NoVerbalCast, Deaf, ThunderImmune},
	},
	{
		// "They flit around you to a distance of 15 feet ... you can designate any number of
		// creatures you can see to be unaffected by it. An affected creature's speed is halved
		// in the area, and when the creature enters the area for the first time on a turn or
		// starts its turn there, it must make a wisdom saving throw ... 3d8 radiant (if you are
		// good or neutral) or 3d8 necrotic (if you are evil)".
		Key: "spell:spirit-guardians", Level: 3, Shape: grid.ShapeSphere, SizeFt: 15, Placement: PlacementCaster,
		DurationRounds: tenMin, Concentration: true, HalvesSpeed: true, Visible: true, Excludes: true,
		Triggers: []Trigger{
			{Kind: OnEnterFirstTimeOnATurn, Save: "wis", Damage: true, OnSuccess: SuccessHalf},
			{Kind: StartOfTurn, Save: "wis", Damage: true, OnSuccess: SuccessHalf},
		},
		Moves: MovesWithCaster,
	},
	{
		// "A silvery beam of pale light shines down in a 5-foot radius, 40-foot-high cylinder
		// ... dim light fills the cylinder. When a creature enters the spell's area for the
		// first time on a turn or starts its turn there ... constitution saving throw ... 2d10
		// radiant damage on a failed save, or half ... On each of your turns after you cast
		// this spell, you can use an action to move the beam 60 feet in any direction."
		Key: "spell:moonbeam", Level: 2, Shape: grid.ShapeCylinder, SizeFt: 5, Placement: PlacementPoint,
		DurationRounds: minute, Concentration: true, Obscurity: ObscureLight, Visible: true,
		Triggers: []Trigger{
			{Kind: OnEnterFirstTimeOnATurn, Save: "con", Damage: true, OnSuccess: SuccessHalf},
			{Kind: StartOfTurn, Save: "con", Damage: true, OnSuccess: SuccessHalf},
		},
		CasterMoves: true, MoveFt: 60,
	},
	{
		// "Slick grease covers the ground in a 10-foot square centered on a point within range
		// and turns it into difficult terrain for the duration. When the grease appears, each
		// creature standing in its area must succeed on a dexterity saving throw or fall prone.
		// A creature that enters the area or ends its turn there must also succeed". 1 minute,
		// and no concentration.
		Key: "spell:grease", Level: 1, Shape: grid.ShapeCube, SizeFt: 10, Placement: PlacementPoint,
		DurationRounds: minute, Difficult: true, Visible: true,
		Triggers: []Trigger{
			{Kind: AtCast, Save: "dex", OnSuccess: SuccessNone, OnFail: FailProne},
			{Kind: OnEnter, Save: "dex", OnSuccess: SuccessNone, OnFail: FailProne},
			{Kind: EndOfTurn, Save: "dex", OnSuccess: SuccessNone, OnFail: FailProne},
		},
	},
	{
		// "Grasping weeds and vines sprout from the ground in a 20-foot square starting from a
		// point within range ... difficult terrain. A creature in the area when you cast the
		// spell must succeed on a strength saving throw or be restrained by the entangling plants
		// until the spell ends."
		Key: "spell:entangle", Level: 1, Shape: grid.ShapeCube, SizeFt: 20, Placement: PlacementPoint,
		DurationRounds: minute, Concentration: true, Difficult: true, Visible: true,
		Triggers: []Trigger{{Kind: AtCast, Save: "str", OnSuccess: SuccessNone, OnFail: FailRestrained}},
	},
}

// Lookup returns the zone a spell leaves, and false for a spell that leaves none.
func Lookup(key string) (Spec, bool) {
	i := slices.IndexFunc(specs, func(s Spec) bool { return s.Key == key })
	if i < 0 {
		return Spec{}, false
	}
	s := specs[i]
	s.Triggers, s.Rules = slices.Clone(s.Triggers), slices.Clone(s.Rules)
	return s, true
}

// Keys lists the spells that leave a zone, in the catalog's order.
func Keys() []string {
	out := make([]string, len(specs))
	for i, s := range specs {
		out[i] = s.Key
	}
	return out
}
