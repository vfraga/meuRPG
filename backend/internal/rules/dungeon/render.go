package dungeon

import (
	"encoding/json"
	"slices"
	"strings"
)

// The renderings the caller derives from the result (spec 1.5 a, b and d).
// The image itself (1.5 c) is not here: the map integration renders floors
// and walls from the grid.

// WallsMask is spec 1.5 a: open is true on every open square (rooms,
// corridors and doors; a door is open in this mask, its state lives in the
// doors layer); wall is true on a square that is not open and has an open
// square among its 8 neighbors. Everything else is rock (the interior of
// solid mass, or the shape mask's exclusion).
func WallsMask(d *Dungeon) (open, wall []bool) {
	w, h := d.Width, d.Height
	open = make([]bool, w*h)
	wall = make([]bool, w*h)
	for i, k := range d.Kinds {
		open[i] = k != KindRock
	}
	for y := range h {
		for x := range w {
			if !open[y*w+x] {
				wall[y*w+x] = touchesOpen(open, w, h, x, y)
			}
		}
	}
	return open, wall
}

// touchesOpen reports whether one of the 8 neighbors of (x, y) inside the
// w by h grid is open.
func touchesOpen(open []bool, w, h, x, y int) bool {
	for ny := max(0, y-1); ny <= min(h-1, y+1); ny++ {
		for nx := max(0, x-1); nx <= min(w-1, x+1); nx++ {
			if open[ny*w+nx] {
				return true
			}
		}
	}
	return false
}

// Marker is one editable object of spec 1.5 b: a door or a stairway.
type Marker struct {
	// Type is "door", "stairs_up" or "stairs_down".
	Type string
	X, Y int
	// Doors: Door, the kind, the trapped flag and the axis.
	Door    DoorKind
	Trapped bool
	Axis    Axis
	// Stairs: the facing.
	Facing Side
}

// Markers is spec 1.5 b: one marker per door, then one per stairs overlay.
func Markers(d *Dungeon) []Marker {
	out := make([]Marker, 0, len(d.Doors)+len(d.Stairs))
	for _, dr := range d.Doors {
		out = append(out, Marker{Type: "door", X: dr.X, Y: dr.Y, Door: dr.Kind, Trapped: dr.Trapped, Axis: dr.Axis})
	}
	for _, s := range d.Stairs {
		out = append(out, Marker{Type: s.Kind.String(), X: s.X, Y: s.Y, Facing: s.Facing})
	}
	return out
}

// Prompt is spec 1.5 d: the structured description of the level for an AI
// image prompt. No pixels, no randomness: a function of the final grid.
type Prompt struct {
	Size        PromptSize         `json:"size"`
	Style       PromptStyle        `json:"style"`
	Rooms       []PromptRoom       `json:"rooms"`
	Doors       []PromptDoor       `json:"doors"`
	Corridors   []PromptCorridor   `json:"corridors"`
	Connections []PromptConnection `json:"connections"`
	Networks    []PromptNetwork    `json:"networks"`
	Stairs      []PromptStair      `json:"stairs"`
	Entrance    PromptEntrance     `json:"entrance"`
}

// PromptSize is the size in squares and feet.
type PromptSize struct {
	Width    int `json:"width"`
	Height   int `json:"height"`
	WidthFt  int `json:"width_ft"`
	HeightFt int `json:"height_ft"`
}

// PromptStyle repeats the option values used.
type PromptStyle struct {
	Mask           string  `json:"mask"`
	Placement      string  `json:"placement"`
	CorridorStyle  string  `json:"corridor_style"`
	DoorMix        string  `json:"door_mix"`
	DoorDensity    int     `json:"door_density"`
	RoomDensity    int     `json:"room_density"`
	RoomSideMin    int     `json:"room_side_min"`
	RoomSideMax    int     `json:"room_side_max"`
	AspectLimit    float64 `json:"aspect_limit"`
	DeadendRemoval int     `json:"deadend_removal"`
	ExtraLoops     int     `json:"extra_loops"`
	Stairs         int     `json:"stairs"`
}

// PromptExit is an exit of a room.
type PromptExit struct {
	DoorKind string `json:"door_kind"`
	Trapped  bool   `json:"trapped"`
	Side     string `json:"side"`
}

// PromptRoom is a room: always a rectangle in version 1.
type PromptRoom struct {
	ID       int          `json:"id"`
	X        int          `json:"x"`
	Y        int          `json:"y"`
	WidthFt  int          `json:"width_ft"`
	HeightFt int          `json:"height_ft"`
	Shape    string       `json:"shape"`
	Exits    []PromptExit `json:"exits"`
}

// PromptJoin is one thing a door joins: a room, a second room or a corridor.
type PromptJoin struct {
	Room     int `json:"room,omitempty"`
	Corridor int `json:"corridor,omitempty"`
}

// PromptDoor is a door.
type PromptDoor struct {
	ID      int           `json:"id"`
	Kind    string        `json:"kind"`
	Trapped bool          `json:"trapped"`
	Side    string        `json:"side"`
	Joins   [2]PromptJoin `json:"joins"`
}

// PromptEnd is where a corridor ends.
type PromptEnd struct {
	Type   string `json:"type"`
	DoorID int    `json:"door_id,omitempty"`
}

// PromptCorridor is a corridor run.
type PromptCorridor struct {
	ID            int         `json:"id"`
	LengthSquares int         `json:"length_squares"`
	Ends          []PromptEnd `json:"ends"`
	Junctions     int         `json:"junctions"`
}

// PromptConnection is a direct door between two rooms (a door in their
// shared wall).
type PromptConnection struct {
	A        int    `json:"a"`
	B        int    `json:"b"`
	DoorKind string `json:"door_kind"`
	Trapped  bool   `json:"trapped"`
}

// PromptNetwork is a connected network of corridor squares: the corridors it
// is made of, the rooms with a door onto it (ascending) and its size.
type PromptNetwork struct {
	ID            int   `json:"id"`
	Corridors     []int `json:"corridors"`
	Rooms         []int `json:"rooms"`
	LengthSquares int   `json:"length_squares"`
}

// PromptStair is a stairway and where it is.
type PromptStair struct {
	Kind       string `json:"kind"`
	X          int    `json:"x"`
	Y          int    `json:"y"`
	InRoom     int    `json:"in_room,omitempty"`
	InCorridor int    `json:"in_corridor,omitempty"`
}

// PromptEntrance is the arrival point.
type PromptEntrance struct {
	X          int `json:"x"`
	Y          int `json:"y"`
	InRoom     int `json:"in_room,omitempty"`
	InCorridor int `json:"in_corridor,omitempty"`
}

// PromptJSON is Prompt(d) as indented JSON.
func PromptJSON(d *Dungeon) ([]byte, error) {
	return json.MarshalIndent(PromptOf(d), "", "  ")
}

// PromptOf builds the structured list of spec 1.5 d.
func PromptOf(d *Dungeon) *Prompt {
	o := d.Options
	p := &Prompt{
		Size: PromptSize{d.Width, d.Height, d.Width * 5, d.Height * 5},
		Style: PromptStyle{
			string(o.Mask), string(o.Placement), string(o.CorridorStyle), string(o.DoorMix),
			o.DoorDensity, o.RoomDensity, o.RoomSideMin, o.RoomSideMax, float64(aspectX100(o.AspectLimit)) / 100,
			o.DeadendRemoval, o.ExtraLoops, o.Stairs,
		},
		Rooms: []PromptRoom{}, Doors: []PromptDoor{}, Corridors: []PromptCorridor{},
		Connections: []PromptConnection{}, Networks: []PromptNetwork{}, Stairs: []PromptStair{},
	}
	for _, r := range d.Rooms {
		pr := PromptRoom{ID: r.ID, X: r.X, Y: r.Y, WidthFt: r.Width * 5, HeightFt: r.Height * 5, Shape: "rectangle", Exits: []PromptExit{}}
		for _, e := range r.Exits {
			pr.Exits = append(pr.Exits, PromptExit{e.Kind.String(), e.Trapped, e.Side.String()})
		}
		p.Rooms = append(p.Rooms, pr)
	}
	for _, dr := range d.Doors {
		pd := PromptDoor{ID: dr.ID, Kind: dr.Kind.String(), Trapped: dr.Trapped, Side: dr.Side.String()}
		pd.Joins[0] = PromptJoin{Room: dr.RoomA}
		if dr.RoomB != 0 {
			pd.Joins[1] = PromptJoin{Room: dr.RoomB}
		} else {
			pd.Joins[1] = PromptJoin{Corridor: dr.Corridor}
		}
		p.Doors = append(p.Doors, pd)
	}
	for _, c := range d.Corridors {
		pc := PromptCorridor{ID: c.ID, LengthSquares: c.Length, Junctions: c.Junctions, Ends: []PromptEnd{}}
		for _, e := range c.Ends {
			pc.Ends = append(pc.Ends, PromptEnd{Type: e.Type, DoorID: e.DoorID})
		}
		p.Corridors = append(p.Corridors, pc)
	}
	p.Connections, p.Networks = connectionsAndNetworks(d)
	for _, s := range d.Stairs {
		ps := PromptStair{Kind: s.Kind.String(), X: s.X, Y: s.Y, InRoom: s.InRoom}
		if s.InRoom == 0 {
			ps.InCorridor = int(d.CorridorIDs[s.Y*d.Width+s.X])
		}
		p.Stairs = append(p.Stairs, ps)
	}
	p.Entrance = PromptEntrance{d.Entrance.X, d.Entrance.Y, d.Entrance.Room, d.Entrance.Corridor}
	return p
}

// connectionsAndNetworks is the room adjacency of spec 1.5 d, in time linear
// in the grid: one labeling of the corridor squares into networks
// (4-neighbor components, IDs in row-major order of their first square) and
// one pass over the doors, which gives the direct connections and the rooms
// of each network.
func connectionsAndNetworks(d *Dungeon) ([]PromptConnection, []PromptNetwork) {
	net := make([]int32, len(d.Kinds)) // network ID per corridor square, 0 elsewhere
	var nets []PromptNetwork
	var queue []int
	for i0, k := range d.Kinds {
		if k != KindCorridor || net[i0] != 0 {
			continue
		}
		var n PromptNetwork
		n, queue = floodNetwork(d, net, queue, i0, int32(len(nets)+1)) //nolint:gosec // G115: at most one network per square
		nets = append(nets, n)
	}
	conns := []PromptConnection{}
	for _, dr := range d.Doors {
		if dr.RoomB != 0 {
			conns = append(conns, PromptConnection{A: min(dr.RoomA, dr.RoomB), B: max(dr.RoomA, dr.RoomB), DoorKind: dr.Kind.String(), Trapped: dr.Trapped})
			continue
		}
		x, y := corridorEnd(d, dr)
		if id := net[y*d.Width+x]; id != 0 {
			nets[id-1].Rooms = append(nets[id-1].Rooms, dr.RoomA)
		}
	}
	for i := range nets {
		slices.Sort(nets[i].Rooms)
		nets[i].Rooms = slices.Compact(nets[i].Rooms)
		slices.Sort(nets[i].Corridors)
		nets[i].Corridors = slices.Compact(nets[i].Corridors)
	}
	return conns, nets
}

// floodNetwork labels the network of corridor square i0 with id
// (breadth-first, reusing queue) and lists its corridors as met.
func floodNetwork(d *Dungeon, net []int32, queue []int, i0 int, id int32) (PromptNetwork, []int) {
	w := d.Width
	n := PromptNetwork{ID: int(id), Corridors: []int{}, Rooms: []int{}}
	net[i0] = id
	queue = append(queue[:0], i0)
	for head := 0; head < len(queue); head++ {
		u := queue[head]
		if c := int(d.CorridorIDs[u]); !slices.Contains(n.Corridors[max(0, len(n.Corridors)-1):], c) {
			n.Corridors = append(n.Corridors, c)
		}
		x, y := u%w, u/w
		for dd := range 4 {
			nx, ny := x+dx[dd], y+dy[dd]
			if nx < 0 || ny < 0 || nx >= w || ny >= d.Height {
				continue
			}
			if v := ny*w + nx; d.Kinds[v] == KindCorridor && net[v] == 0 {
				net[v] = id
				queue = append(queue, v)
			}
		}
	}
	n.LengthSquares = len(queue)
	return n, queue
}

// ASCII draws the level for a human: '#' rock or wall, '.' room floor, ',' corridor,
// '+' closed door, '/' archway, 'B' barred, 'L' locked, 'S' secret, 'T' marks a trapped door
// (shown in place of its kind letter), '<' stairs up, '>' stairs down, '@' the entrance on
// the stairs or in a room's center. Blocked squares by the mask are ' '.
func ASCII(d *Dungeon) string {
	var b strings.Builder
	w := d.Width
	doorAt := map[int]Door{}
	for _, dr := range d.Doors {
		doorAt[dr.Y*w+dr.X] = dr
	}
	for y := range d.Height {
		for x := range w {
			b.WriteByte(asciiOverlay(d, x, y, asciiSquare(d, doorAt, y*w+x)))
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// asciiSquare is the character of a square's kind.
func asciiSquare(d *Dungeon, doorAt map[int]Door, i int) byte {
	switch d.Kinds[i] {
	case KindRock:
		if !d.Mask[i] {
			return ' '
		}
	case KindRoom:
		return '.'
	case KindCorridor:
		return ','
	case KindDoor:
		dr := doorAt[i]
		if dr.Trapped {
			return 'T'
		}
		return "/+BLS"[dr.Kind]
	}
	return '#'
}

// asciiOverlay puts a stair, else the entrance, over the character c.
func asciiOverlay(d *Dungeon, x, y int, c byte) byte {
	if s, ok := d.StairAt(x, y); ok {
		if s.Kind == StairDown {
			return '>'
		}
		return '<'
	}
	if x == d.Entrance.X && y == d.Entrance.Y {
		return '@'
	}
	return c
}

// corridorEnd is the square of a corridor door that is not a room floor: the
// first square of the corridor beside it.
func corridorEnd(d *Dungeon, dr Door) (x, y int) {
	ax, ay := 0, 1
	if dr.Axis == VerticalWall {
		ax, ay = 1, 0
	}
	if d.RoomIDs[(dr.Y-ay)*d.Width+dr.X-ax] != 0 {
		return dr.X + ax, dr.Y + ay
	}
	return dr.X - ax, dr.Y - ay
}
