package dungeon

import (
	"errors"
	"fmt"
	"math"
)

// Mask is the overall silhouette of the level (spec 3.2).
type Mask string

// The silhouettes.
const (
	MaskNone    Mask = "none"
	MaskDonut   Mask = "donut"
	MaskPlus    Mask = "plus"
	MaskLShape  Mask = "l_shape"
	MaskEllipse Mask = "ellipse"
	MaskDiamond Mask = "diamond"
	MaskCustom  Mask = "custom"
)

// Placement is how rooms are placed (spec 3.3).
type Placement string

// The placement styles.
const (
	PlacementSpread Placement = "spread"
	PlacementTiled  Placement = "tiled"
)

// CorridorStyle is the tendency of corridors to keep their direction.
type CorridorStyle string

// The corridor styles: 0, 40 and 85 percent to try the arrival direction first.
const (
	StyleTwisty     CorridorStyle = "twisty"
	StyleMeandering CorridorStyle = "meandering"
	StyleLongRuns   CorridorStyle = "long_runs"
)

// DoorMix is the mix of door kinds (spec 2, the weights table).
type DoorMix string

// The door mixes.
const (
	MixOpen     DoorMix = "open"
	MixTypical  DoorMix = "typical"
	MixSecured  DoorMix = "secured"
	MixParanoid DoorMix = "paranoid"
)

// CustomMask is a coarse yes/no bitmap stretched over the grid by
// nearest-neighbor sampling: On is row-major, Width * Height long, true
// where the level may be.
type CustomMask struct {
	Width, Height int
	On            []bool
}

// Options are the inputs of Generate (spec 2). Every field has a default in
// DefaultOptions; Generate itself takes the options as given and rejects
// what is out of range with an *OptionError (never silently clamps), except
// where the spec says "normalised": even sizes go down to the next odd
// value, an even RoomSideMin goes up and an even RoomSideMax goes down. The
// normalised options come back in Dungeon.Options.
type Options struct {
	// Seed selects the output. The generator never picks one.
	Seed uint64
	// Version pins the algorithm and the draw order (1 to Version).
	Version int
	// Width and Height are in squares: 15 to 199 and 15 to 399.
	Width, Height int
	// Mask is the silhouette. MaskHole (20 to 60, percent of each side) only
	// counts for MaskDonut; CustomMask only for MaskCustom.
	Mask       Mask
	MaskHole   int
	CustomMask *CustomMask
	// RoomSideMin (3 to 15) and RoomSideMax (RoomSideMin to 31) are odd.
	RoomSideMin, RoomSideMax int
	// AspectLimit (1.0 to 6.0): the longest room side over the shortest.
	AspectLimit float64
	// RoomDensity is 10 to 200 percent.
	RoomDensity int
	Placement   Placement
	// CorridorStyle shapes the maze walks.
	CorridorStyle CorridorStyle
	// DoorDensity is 25 to 200 percent; DoorMix picks the kinds.
	DoorDensity int
	DoorMix     DoorMix
	// DeadendRemoval is 0 to 100 percent; Stairs 0 to 8; ExtraLoops 0 to 100.
	DeadendRemoval int
	Stairs         int
	ExtraLoops     int
	// SelfCheck runs every invariant of spec 5 on the result (spec 3.9 step
	// 6) and makes Generate fail with ErrSelfCheck if one breaks. For tests
	// and debug builds; all the checks are linear.
	SelfCheck bool
}

// Errors.
var (
	// ErrInvalidOption is what every *OptionError unwraps to.
	ErrInvalidOption = errors.New("dungeon: invalid option")
	// ErrNoSpace: the mask (or the size) leaves no room for even one room of
	// RoomSideMin; the generator never returns a dungeon with no room.
	ErrNoSpace = errors.New("dungeon: no space for a room")
	// ErrSelfCheck: the self-check found a broken invariant (a bug).
	ErrSelfCheck = errors.New("dungeon: self-check failed")
)

// OptionError names the option that was rejected (the spec's name, such as
// "room_side_min") and why.
type OptionError struct {
	Option string
	Reason string
}

func (e *OptionError) Error() string {
	return fmt.Sprintf("dungeon: invalid option %s: %s", e.Option, e.Reason)
}

// Unwrap lets errors.Is(err, ErrInvalidOption) work.
func (e *OptionError) Unwrap() error { return ErrInvalidOption }

func optErr(option, format string, args ...any) error {
	return &OptionError{Option: option, Reason: fmt.Sprintf(format, args...)}
}

// DefaultOptions are the proposed defaults of spec 2 for a seed.
func DefaultOptions(seed uint64) Options {
	return Options{
		Seed: seed, Version: Version,
		Width: 51, Height: 51,
		Mask: MaskNone, MaskHole: 40,
		RoomSideMin: 3, RoomSideMax: 11,
		AspectLimit: 3.0, RoomDensity: 100,
		Placement: PlacementSpread, CorridorStyle: StyleMeandering,
		DoorDensity: 100, DoorMix: MixTypical,
		DeadendRemoval: 60, Stairs: 2, ExtraLoops: 0,
	}
}

// aspectX100 is AspectLimit in hundredths, so that no decision uses floating
// point (the one conversion happens here, on a validated value).
func aspectX100(limit float64) int { return int(math.Round(limit * 100)) }

func inRange(option string, v, lo, hi int) error {
	if v < lo || v > hi {
		return optErr(option, "%d is outside %d to %d", v, lo, hi)
	}
	return nil
}

// normalize validates the options and returns them with the even sizes
// normalised. The checks run in a fixed order and the first refusal is the
// one returned (with the options normalised so far).
func normalize(o Options) (Options, error) {
	if err := checkGrid(o); err != nil {
		return o, err
	}
	o.Width -= (o.Width + 1) % 2 // even goes down to the next odd
	o.Height -= (o.Height + 1) % 2
	if err := checkMask(o); err != nil {
		return o, err
	}
	if err := inRange("room_side_min", o.RoomSideMin, 3, 15); err != nil {
		return o, err
	}
	if err := inRange("room_side_max", o.RoomSideMax, o.RoomSideMin, 31); err != nil {
		return o, err
	}
	o.RoomSideMin, o.RoomSideMax = oddSides(o.RoomSideMin, o.RoomSideMax)
	if err := checkLayout(o); err != nil {
		return o, err
	}
	return o, checkDoorsAndFinish(o)
}

// checkGrid checks the version and the grid size.
func checkGrid(o Options) error {
	if err := inRange("version", o.Version, 1, Version); err != nil {
		return err
	}
	if err := inRange("width", o.Width, 15, 199); err != nil {
		return err
	}
	return inRange("height", o.Height, 15, 399)
}

// checkMask checks the silhouette and the option it needs (the hole of a
// donut, the bitmap of a custom mask), against the normalised grid.
func checkMask(o Options) error {
	switch o.Mask {
	case MaskNone, MaskPlus, MaskLShape, MaskEllipse, MaskDiamond:
		return nil
	case MaskDonut:
		return inRange("mask_hole", o.MaskHole, 20, 60)
	case MaskCustom:
		return checkCustomMask(o.CustomMask, o.Width, o.Height)
	}
	return optErr("mask", "unknown mask %q", o.Mask)
}

// checkCustomMask checks a custom bitmap against a w by h grid.
func checkCustomMask(m *CustomMask, w, h int) error {
	switch {
	case m == nil:
		return optErr("custom_mask", "required with mask custom")
	case m.Width < 3 || m.Height < 3:
		return optErr("custom_mask", "%d by %d is smaller than 3 by 3", m.Width, m.Height)
	case m.Width > w || m.Height > h:
		return optErr("custom_mask", "%d by %d is larger than the %d by %d grid", m.Width, m.Height, w, h)
	case len(m.On) != m.Width*m.Height:
		return optErr("custom_mask", "%d cells for a %d by %d bitmap", len(m.On), m.Width, m.Height)
	}
	return nil
}

// oddSides makes an even least side odd upward and an even greatest side odd
// downward; 14, 14 gives 15, 13, and the smallest reading is max = min.
func oddSides(lo, hi int) (minSide, maxSide int) {
	if lo%2 == 0 {
		lo++
	}
	if hi%2 == 0 {
		hi--
	}
	return lo, max(hi, lo)
}

// checkLayout checks the room shapes, the density and the two styles.
func checkLayout(o Options) error {
	if !(o.AspectLimit >= 1 && o.AspectLimit <= 6) { // also rejects NaN
		return optErr("aspect_limit", "%v is outside 1.0 to 6.0", o.AspectLimit)
	}
	if err := inRange("room_density", o.RoomDensity, 10, 200); err != nil {
		return err
	}
	switch o.Placement {
	case PlacementSpread, PlacementTiled:
	default:
		return optErr("placement", "unknown placement %q", o.Placement)
	}
	switch o.CorridorStyle {
	case StyleTwisty, StyleMeandering, StyleLongRuns:
		return nil
	}
	return optErr("corridor_style", "unknown style %q", o.CorridorStyle)
}

// checkDoorsAndFinish checks the doors and the last phases' options.
func checkDoorsAndFinish(o Options) error {
	if err := inRange("door_density", o.DoorDensity, 25, 200); err != nil {
		return err
	}
	switch o.DoorMix {
	case MixOpen, MixTypical, MixSecured, MixParanoid:
	default:
		return optErr("door_mix", "unknown mix %q", o.DoorMix)
	}
	if err := inRange("deadend_removal", o.DeadendRemoval, 0, 100); err != nil {
		return err
	}
	if err := inRange("stairs", o.Stairs, 0, 8); err != nil {
		return err
	}
	return inRange("extra_loops", o.ExtraLoops, 0, 100)
}
