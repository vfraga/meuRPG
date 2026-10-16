package grid

import "fmt"

// Door is the state of a square of the doors layer (MR-010, RN-26). The values
// are what the four bits of a DoorLayer hold, and what the API's DoorState
// numbers mean.
type Door uint8

// The six states of a door.
const (
	// DoorNone: no door on the square.
	DoorNone Door = iota
	// DoorOpen: nothing blocks.
	DoorOpen
	// DoorClosed blocks sight, light and cover like a wall, and a creature that
	// moves into the square opens it.
	DoorClosed
	// DoorLocked is closed and stays closed: a move that enters it stops before.
	DoorLocked
	// DoorBarred ("grade") blocks movement only: sight and light pass, and it
	// gives no cover.
	DoorBarred
	// DoorSecret is a wall to everyone who has not found it.
	DoorSecret
)

// doorHighest is the largest value a DoorLayer stores.
const doorHighest = DoorSecret

// Valid says whether d is one of the six states.
func (d Door) Valid() bool { return d <= doorHighest }

// BlocksSight says whether the door stops sight, light and cover as a wall
// does: closed, locked and secret.
func (d Door) BlocksSight() bool { return d == DoorClosed || d == DoorLocked || d == DoorSecret }

// BlocksMove says whether a creature cannot walk through the door: locked,
// barred and secret. A closed door does not block: the move opens it.
func (d Door) BlocksMove() bool { return d == DoorLocked || d == DoorBarred || d == DoorSecret }

// DoorLayer is the doors of a grid, four bits a square (see layer.go for the
// byte layout). A nil *DoorLayer reads as "no door painted".
type DoorLayer struct {
	g    Grid
	bits []byte
}

// DoorLayerSize is how many bytes a DoorLayer of the grid takes when encoded.
func DoorLayerSize(g Grid) int { return (g.Squares() + 1) / 2 }

// NewDoorLayer is a DoorLayer of the grid with no door.
func NewDoorLayer(g Grid) *DoorLayer {
	return &DoorLayer{g: g, bits: make([]byte, DoorLayerSize(g))}
}

// DecodeDoorLayer reads the bytes Encode wrote for the same grid. It returns
// ErrLayerSize for the wrong length, a nonzero unused nibble or a square
// holding a value above DoorSecret.
func DecodeDoorLayer(g Grid, b []byte) (*DoorLayer, error) {
	if !g.Valid() || len(b) != DoorLayerSize(g) {
		return nil, fmt.Errorf("%w: %d bytes for %d x %d squares", ErrLayerSize, len(b), g.Columns, g.Rows)
	}
	if g.Squares()%2 == 1 && b[len(b)-1]>>4 != 0 {
		return nil, fmt.Errorf("%w: unused bits are set", ErrLayerSize)
	}
	l := &DoorLayer{g: g, bits: append([]byte(nil), b...)}
	for n := 0; n < g.Squares(); n++ {
		if v := l.at(n); !v.Valid() {
			return nil, fmt.Errorf("%w: square %d holds %d", ErrLayerSize, n, v)
		}
	}
	return l, nil
}

// Encode is the layer's bytes, in the layout of layer.go. The caller may keep
// them.
func (l *DoorLayer) Encode() []byte {
	if l == nil {
		return nil
	}
	return append([]byte(nil), l.bits...)
}

// Grid is the grid the layer is sized for.
func (l *DoorLayer) Grid() Grid {
	if l == nil {
		return Grid{}
	}
	return l.g
}

func (l *DoorLayer) at(n int) Door { return Door(l.bits[n>>1] >> (4 * (n & 1)) & 15) }

// Get is the door on a square: DoorNone for one with none and for one outside
// the grid.
func (l *DoorLayer) Get(col, row int) Door {
	if l == nil || len(l.bits) == 0 || col < 0 || col >= l.g.Columns || row < 0 || row >= l.g.Rows {
		return DoorNone
	}
	return l.at(l.g.index(col, row))
}

// At is Get for a Square.
func (l *DoorLayer) At(s Square) Door { return l.Get(s.Col, s.Row) }

// Set puts a door on a square (DoorNone clears it). It returns false, and
// changes nothing, for a square outside the grid or a value that is not a
// state.
func (l *DoorLayer) Set(col, row int, v Door) bool {
	if l == nil || len(l.bits) == 0 || !v.Valid() || col < 0 || col >= l.g.Columns || row < 0 || row >= l.g.Rows {
		return false
	}
	n := l.g.index(col, row)
	shift := 4 * (n & 1)
	l.bits[n>>1] = l.bits[n>>1]&^(15<<shift) | byte(v)<<shift
	return true
}

// Count is how many squares hold a door (any state but DoorNone).
func (l *DoorLayer) Count() int {
	if l == nil {
		return 0
	}
	n := 0
	for i := 0; i < l.g.Squares(); i++ {
		if l.at(i) != DoorNone {
			n++
		}
	}
	return n
}

// SightWalls is what stops sight and light on a grid: the walls and the squares
// of closed, locked and secret doors. It returns walls itself when no door
// blocks, and a new Layer otherwise (the arguments are not changed). Package
// vision builds its scene on it.
func SightWalls(walls *Layer, doors *DoorLayer) *Layer {
	if doors == nil || (walls != nil && walls.g != doors.g) {
		return walls
	}
	if out := withShutDoors(walls, doors); out != nil {
		return out
	}
	return walls
}

// withShutDoors is a copy of the walls (or a new layer, when there are none)
// with the squares of the doors that block sight set, or nil when no door
// blocks sight.
func withShutDoors(walls *Layer, doors *DoorLayer) *Layer {
	var out *Layer
	for row := range doors.g.Rows {
		for col := range doors.g.Columns {
			if !doors.Get(col, row).BlocksSight() {
				continue
			}
			if out == nil {
				out = copyOrNewLayer(walls, doors.g)
			}
			out.Set(col, row, true)
		}
	}
	return out
}

// copyOrNewLayer is a copy of the layer, or an empty layer of the grid when
// it is nil.
func copyOrNewLayer(l *Layer, g Grid) *Layer {
	out := NewLayer(g)
	if l != nil {
		out.bits = append(out.bits[:0], l.bits...)
	}
	return out
}

// AsPlayerKnows is the door as a player who has not found out more reads it
// (RN-26): a locked door looks closed, since they learn it is locked by
// trying, and a secret door is no door at all but a wall (wall is true: the
// walls layer they get has one there).
func (d Door) AsPlayerKnows() (door Door, wall bool) {
	switch d {
	case DoorLocked:
		return DoorClosed, false
	case DoorSecret:
		return DoorNone, true
	default:
		return d, false
	}
}

// ForPlayers is the walls and the doors as a player who has not found out more
// knows them (see Door.AsPlayerKnows): a copy, the arguments are not changed.
// A nil walls stays nil when there is nothing to add, and nil doors give nil
// doors. Doors sized for another grid than the walls are not trusted: it fails
// closed, with the walls and no doors, so nothing a player must not know is sent.
func (l *DoorLayer) ForPlayers(walls *Layer) (*Layer, *DoorLayer) {
	if l == nil {
		return walls, nil
	}
	if walls != nil && walls.g != l.g {
		return walls, nil
	}
	out := NewDoorLayer(l.g)
	var w *Layer
	if walls != nil {
		w = &Layer{g: walls.g, bits: append([]byte(nil), walls.bits...)}
	}
	for row := range l.g.Rows {
		for col := range l.g.Columns {
			door, wall := l.Get(col, row).AsPlayerKnows()
			out.Set(col, row, door)
			if wall {
				if w == nil {
					w = NewLayer(l.g)
				}
				w.Set(col, row, true)
			}
		}
	}
	return w, out
}

// ForPlayers is the terrain as a player who has not found out more knows it:
// see DoorLayer.ForPlayers. The planning of a player's move runs on it, so a
// locked door looks like a closed one (it will be tried) and a secret door like
// a wall.
func (t Terrain) ForPlayers() Terrain {
	t.Walls, t.Doors = t.Doors.ForPlayers(t.Walls)
	return t
}
