package dungeon

// buildMask marks the squares outside the silhouette as blocked (spec 3.2).
// All the arithmetic is integer: a mask never depends on floating point.
func (g *gen) buildMask() {
	w, h := g.w, g.h
	for y := range h {
		for x := range w {
			b := !g.insideMask(x, y)
			g.blocked[g.idx(x, y)] = b
			g.anyBlocked = g.anyBlocked || b
		}
	}
}

func (g *gen) insideMask(x, y int) bool {
	switch g.o.Mask {
	case MaskDonut:
		return g.insideDonut(x, y)
	case MaskPlus:
		cw, ch := g.w/4, g.h/4
		return x >= cw && x < g.w-cw || y >= ch && y < g.h-ch
	case MaskLShape:
		return x < g.w-g.w*45/100 || y >= g.h*45/100
	case MaskEllipse:
		return g.insideEllipse(x, y)
	case MaskDiamond:
		return g.insideDiamond(x, y)
	case MaskCustom:
		m := g.o.CustomMask
		cx := (2*x + 1) * m.Width / (2 * g.w)
		cy := (2*y + 1) * m.Height / (2 * g.h)
		return m.On[cy*m.Width+cx]
	}
	return true
}

// insideDonut: outside the centered hole of MaskHole percent of each side.
func (g *gen) insideDonut(x, y int) bool {
	w, h := g.w, g.h
	hw, hh := w*g.o.MaskHole/100, h*g.o.MaskHole/100
	x0, y0 := (w-hw)/2, (h-hh)/2
	return x < x0 || x >= x0+hw || y < y0 || y >= y0+hh
}

// insideEllipse: the center of the square against the inscribed ellipse,
// scaled by 2.
func (g *gen) insideEllipse(x, y int) bool {
	w, h := int64(g.w), int64(g.h)
	a, b := int64(2*x+1)-w, int64(2*y+1)-h
	return a*a*h*h+b*b*w*w <= w*w*h*h
}

// insideDiamond: the center of the square against the inscribed diamond,
// scaled by 2.
func (g *gen) insideDiamond(x, y int) bool {
	w, h := int64(g.w), int64(g.h)
	a, b := int64(2*x+1)-w, int64(2*y+1)-h
	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}
	return a*h+b*w <= w*h
}
