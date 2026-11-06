package main

// A tiny anti-aliased software renderer (signed-distance-field shapes) that
// draws into a premultiplied RGBA float buffer. All drawing coordinates are
// in logical units; the canvas scale converts them to pixels (for DPI).

import "math"

type Col struct{ R, G, B, A float32 }

func Hex(h uint32, a float32) Col {
	return Col{float32(h>>16&255) / 255, float32(h>>8&255) / 255, float32(h&255) / 255, a}
}

func White(a float32) Col { return Col{1, 1, 1, a} }
func Black(a float32) Col { return Col{0, 0, 0, a} }

type Paint func(x, y float32) Col

func Solid(c Col) Paint { return func(float32, float32) Col { return c } }

func lerp(a, b, t float32) float32 { return a + (b-a)*t }

func lerpCol(a, b Col, t float32) Col {
	return Col{lerp(a.R, b.R, t), lerp(a.G, b.G, t), lerp(a.B, b.B, t), lerp(a.A, b.A, t)}
}

func stops(t float32, s []Col) Col {
	if len(s) == 1 {
		return s[0]
	}
	f := clamp01(t) * float32(len(s)-1)
	i := int(f)
	if i >= len(s)-1 {
		return s[len(s)-1]
	}
	return lerpCol(s[i], s[i+1], f-float32(i))
}

// Vertical gradient between y0 and y1 through evenly spaced colors.
func VGrad(y0, y1 float32, s ...Col) Paint {
	return func(_, y float32) Col { return stops((y-y0)/(y1-y0), s) }
}

// Horizontal gradient.
func HGrad(x0, x1 float32, s ...Col) Paint {
	return func(x, _ float32) Col { return stops((x-x0)/(x1-x0), s) }
}

// Radial glow: color c at the center fading to transparent at radius r.
func Radial(cx, cy, r float32, c Col) Paint {
	return func(x, y float32) Col {
		d := hypot(x-cx, y-cy) / r
		t := clamp01(1 - d)
		t = t * t
		return Col{c.R, c.G, c.B, c.A * t}
	}
}

func clamp01(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

func max32(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

func min32(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

func hypot(x, y float32) float32 { return float32(math.Sqrt(float64(x*x + y*y))) }

// ---------- signed distance functions (negative = inside) ----------

func sdRoundRect(px, py, x, y, w, h, r float32) float32 {
	cx, cy := x+w/2, y+h/2
	qx := abs32(px-cx) - (w/2 - r)
	qy := abs32(py-cy) - (h/2 - r)
	return hypot(max32(qx, 0), max32(qy, 0)) + min32(max32(qx, qy), 0) - r
}

func sdCircle(px, py, cx, cy, r float32) float32 { return hypot(px-cx, py-cy) - r }

func sdSegment(px, py, ax, ay, bx, by, r float32) float32 {
	pax, pay := px-ax, py-ay
	bax, bay := bx-ax, by-ay
	h := clamp01((pax*bax + pay*bay) / (bax*bax + bay*bay + 1e-9))
	return hypot(pax-bax*h, pay-bay*h) - r
}

// Approximate ellipse distance (good enough for soft highlights).
func sdEllipse(px, py, cx, cy, a, b float32) float32 {
	k := hypot((px-cx)/a, (py-cy)/b)
	return (k - 1) * min32(a, b)
}

// Polygon given as x0,y0,x1,y1,...
func sdPolygon(px, py float32, v []float32) float32 {
	n := len(v) / 2
	d := (px-v[0])*(px-v[0]) + (py-v[1])*(py-v[1])
	s := float32(1)
	for i, j := 0, n-1; i < n; j, i = i, i+1 {
		vix, viy := v[2*i], v[2*i+1]
		vjx, vjy := v[2*j], v[2*j+1]
		ex, ey := vjx-vix, vjy-viy
		wx, wy := px-vix, py-viy
		t := clamp01((wx*ex + wy*ey) / (ex*ex + ey*ey))
		bx, by := wx-ex*t, wy-ey*t
		if dd := bx*bx + by*by; dd < d {
			d = dd
		}
		c1 := py >= viy
		c2 := py < vjy
		c3 := ex*wy > ey*wx
		if (c1 && c2 && c3) || (!c1 && !c2 && !c3) {
			s = -s
		}
	}
	return s * float32(math.Sqrt(float64(d)))
}

// ---------- canvas ----------

type Canvas struct {
	W, H int
	S    float32 // pixels per logical unit
	Pix  []float32

	OX, OY float32 // translation added to all drawing (logical units)
	GA     float32 // global alpha for everything drawn
	Clip   SDF     // optional clip in absolute logical coords (draw where <= 0)

	clips []clipRR
	cin   [4]float32 // inside this box the clip can't cut anything
	cout  [4]float32 // outside this box nothing is visible
}

type clipRR struct{ x0, y0, x1, y1, r float32 }

// PushClip limits drawing to a rounded rect (page coords), on top of any
// clip already active. Pair with PopClip.
func (c *Canvas) PushClip(x0, y0, x1, y1, r float32) {
	c.clips = append(c.clips, clipRR{x0 + c.OX, y0 + c.OY, x1 + c.OX, y1 + c.OY, r})
	c.updateClip()
}

func (c *Canvas) PopClip() {
	if len(c.clips) > 0 {
		c.clips = c.clips[:len(c.clips)-1]
	}
	c.updateClip()
}

func (c *Canvas) updateClip() {
	if len(c.clips) == 0 {
		c.Clip = nil
		return
	}
	in := [4]float32{-1e9, -1e9, 1e9, 1e9}
	out := in
	for _, k := range c.clips {
		ins := k.r*0.3 + 1
		in = [4]float32{max32(in[0], k.x0+ins), max32(in[1], k.y0+ins), min32(in[2], k.x1-ins), min32(in[3], k.y1-ins)}
		out = [4]float32{max32(out[0], k.x0), max32(out[1], k.y0), min32(out[2], k.x1), min32(out[3], k.y1)}
	}
	c.cin, c.cout = in, out
	clips := append([]clipRR(nil), c.clips...)
	c.Clip = func(x, y float32) float32 {
		d := float32(-1e9)
		for _, k := range clips {
			d = max32(d, sdRoundRect(x, y, k.x0, k.y0, k.x1-k.x0, k.y1-k.y0, k.r))
		}
		return d
	}
}

// clipped reports whether the absolute point needs the clip test.
func (c *Canvas) needClip(ax, ay float32) bool {
	return c.Clip != nil && !(ax >= c.cin[0] && ax <= c.cin[2] && ay >= c.cin[1] && ay <= c.cin[3])
}

func NewCanvas(w, h int, s float32) *Canvas {
	return &Canvas{W: w, H: h, S: s, Pix: make([]float32, w*h*4), GA: 1}
}

// Reset translation/alpha/clip to defaults.
func (c *Canvas) ResetState() {
	c.OX, c.OY, c.GA, c.Clip = 0, 0, 1, nil
	c.clips = c.clips[:0]
}

func (c *Canvas) Clear() {
	for i := range c.Pix {
		c.Pix[i] = 0
	}
}

func (c *Canvas) blend(i int, col Col, cov float32) {
	a := col.A * cov * c.GA
	if a <= 0 {
		return
	}
	inv := 1 - a
	p := c.Pix[i : i+4 : i+4]
	p[0] = col.R*a + p[0]*inv
	p[1] = col.G*a + p[1]*inv
	p[2] = col.B*a + p[2]*inv
	p[3] = a + p[3]*inv
}

type SDF func(x, y float32) float32

// Fill draws the shape described by sdf inside the logical bbox.
// blur > 0 gives a soft edge of that logical width (for shadows/glows).
// clip (optional) restricts drawing to where clip <= 0.
func (c *Canvas) Fill(bx0, by0, bx1, by1 float32, sdf SDF, p Paint, blur float32, clip SDF) {
	if c.GA <= 0.003 {
		return
	}
	S := c.S
	ox, oy := c.OX, c.OY
	pad := float32(2)
	if blur > 0 {
		pad += blur * S
	}
	x0 := int(math.Floor(float64((bx0+ox)*S - pad)))
	y0 := int(math.Floor(float64((by0+oy)*S - pad)))
	x1 := int(math.Ceil(float64((bx1+ox)*S + pad)))
	y1 := int(math.Ceil(float64((by1+oy)*S + pad)))
	if c.Clip != nil && len(c.clips) > 0 { // nothing visible: skip the whole shape
		if float32(x1)/S < c.cout[0] || float32(x0)/S > c.cout[2] || float32(y1)/S < c.cout[1] || float32(y0)/S > c.cout[3] {
			return
		}
	}
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	if x1 > c.W {
		x1 = c.W
	}
	if y1 > c.H {
		y1 = c.H
	}
	rows := func(ry0, ry1 int) {
		for py := ry0; py < ry1; py++ {
			ay := (float32(py) + 0.5) / S
			ly := ay - oy
			for px := x0; px < x1; px++ {
				ax := (float32(px) + 0.5) / S
				lx := ax - ox
				d := sdf(lx, ly)
				var cov float32
				if blur > 0 {
					t := clamp01(0.5 - d/blur)
					cov = t * t * (3 - 2*t)
				} else {
					cov = clamp01(0.5 - d*S)
				}
				if cov <= 0 {
					continue
				}
				if clip != nil {
					cov *= clamp01(0.5 - clip(lx, ly)*S)
					if cov <= 0 {
						continue
					}
				}
				if c.needClip(ax, ay) {
					cov *= clamp01(0.5 - c.Clip(ax, ay)*S)
					if cov <= 0 {
						continue
					}
				}
				c.blend((py*c.W+px)*4, p(lx, ly), cov)
			}
		}
	}
	// big shapes: split the rows across cores (rows never overlap)
	if (x1-x0)*(y1-y0) > 24000 && y1-y0 >= 16 {
		parallelSpan(y0, y1, rows)
	} else {
		rows(y0, y1)
	}
}

// parallelSpan runs fn over [y0, y1) split across cores.
func parallelSpan(y0, y1 int, fn func(a, b int)) {
	parallelRows(y1-y0, func(a, b int) { fn(y0+a, y0+b) })
}

func (c *Canvas) RoundRect(x, y, w, h, r float32, p Paint) {
	c.Fill(x, y, x+w, y+h, func(px, py float32) float32 { return sdRoundRect(px, py, x, y, w, h, r) }, p, 0, nil)
}

// Stroke drawn on the inside edge of the rounded rect.
func (c *Canvas) StrokeRoundRect(x, y, w, h, r, sw float32, p Paint) {
	c.Fill(x, y, x+w, y+h, func(px, py float32) float32 {
		d := sdRoundRect(px, py, x, y, w, h, r)
		return abs32(d+sw/2) - sw/2
	}, p, 0, nil)
}

func (c *Canvas) Circle(cx, cy, r float32, p Paint) {
	c.Fill(cx-r, cy-r, cx+r, cy+r, func(px, py float32) float32 { return sdCircle(px, py, cx, cy, r) }, p, 0, nil)
}

func (c *Canvas) StrokeCircle(cx, cy, r, sw float32, p Paint) {
	c.Fill(cx-r, cy-r, cx+r, cy+r, func(px, py float32) float32 {
		return abs32(sdCircle(px, py, cx, cy, r)+sw/2) - sw/2
	}, p, 0, nil)
}

func (c *Canvas) Line(ax, ay, bx, by, w float32, p Paint) {
	r := w / 2
	c.Fill(min32(ax, bx)-r, min32(ay, by)-r, max32(ax, bx)+r, max32(ay, by)+r,
		func(px, py float32) float32 { return sdSegment(px, py, ax, ay, bx, by, r) }, p, 0, nil)
}

func (c *Canvas) Poly(v []float32, round float32, p Paint) {
	x0, y0, x1, y1 := v[0], v[1], v[0], v[1]
	for i := 0; i < len(v); i += 2 {
		x0, x1 = min32(x0, v[i]), max32(x1, v[i])
		y0, y1 = min32(y0, v[i+1]), max32(y1, v[i+1])
	}
	c.Fill(x0-round, y0-round, x1+round, y1+round,
		func(px, py float32) float32 { return sdPolygon(px, py, v) - round }, p, 0, nil)
}

// ---------- text masks ----------

// Mask is an 8-bit-ish coverage map in PIXELS (already scaled).
type Mask struct {
	W, H int
	A    []float32
}

func (c *Canvas) DrawMask(m *Mask, lx, ly float32, col Col) {
	if m == nil || m.W == 0 || c.GA <= 0.003 {
		return
	}
	ox := int(math.Round(float64((lx + c.OX) * c.S)))
	oy := int(math.Round(float64((ly + c.OY) * c.S)))
	for y := 0; y < m.H; y++ {
		py := oy + y
		if py < 0 || py >= c.H {
			continue
		}
		for x := 0; x < m.W; x++ {
			px := ox + x
			if px < 0 || px >= c.W {
				continue
			}
			a := m.A[y*m.W+x]
			if a > 0 {
				if ax, ay := (float32(px)+0.5)/c.S, (float32(py)+0.5)/c.S; c.needClip(ax, ay) {
					a *= clamp01(0.5 - c.Clip(ax, ay)*c.S)
				}
				c.blend((py*c.W+px)*4, col, a)
			}
		}
	}
}

// ---------- output ----------

// BGRA writes 8-bit BGRA. premul=true for layered windows,
// false (straight alpha) for icons and PNGs.
func (c *Canvas) BGRA(dst []byte, premul bool) {
	for i, j := 0, 0; i < len(c.Pix); i, j = i+4, j+4 {
		r, g, b, a := c.Pix[i], c.Pix[i+1], c.Pix[i+2], c.Pix[i+3]
		if !premul && a > 0 {
			r, g, b = r/a, g/a, b/a
		}
		dst[j] = to8(b)
		dst[j+1] = to8(g)
		dst[j+2] = to8(r)
		dst[j+3] = to8(a)
	}
}

func to8(v float32) byte {
	if v <= 0 {
		return 0
	}
	if v >= 1 {
		return 255
	}
	return byte(v*255 + 0.5)
}

// ---------- color helpers ----------

func HSV(h, s, v float32) Col {
	h = h - float32(math.Floor(float64(h)))
	i := int(h * 6)
	f := h*6 - float32(i)
	p, q, t := v*(1-s), v*(1-f*s), v*(1-(1-f)*s)
	switch i % 6 {
	case 0:
		return Col{v, t, p, 1}
	case 1:
		return Col{q, v, p, 1}
	case 2:
		return Col{p, v, t, 1}
	case 3:
		return Col{p, q, v, 1}
	case 4:
		return Col{t, p, v, 1}
	}
	return Col{v, p, q, 1}
}

func (c Col) WithA(a float32) Col { return Col{c.R, c.G, c.B, a} }

func (c Col) Mix(o Col, t float32) Col { return lerpCol(c, o, t) }

func (c Col) Lum() float32 { return 0.2126*c.R + 0.7152*c.G + 0.0722*c.B }

// Ring segment (arc) centered at cx,cy on the right side, half-angle a (radians).
func sdArcRight(px, py, cx, cy, r, w, a float32) float32 {
	ring := abs32(hypot(px-cx, py-cy)-r) - w/2
	// keep the part within +-a of the +x axis; rounded caps at the ends
	ang := float32(math.Atan2(float64(py-cy), float64(px-cx)))
	if abs32(ang) <= a {
		return ring
	}
	ex := cx + r*float32(math.Cos(float64(a)))
	ey := r * float32(math.Sin(float64(a)))
	if py < cy {
		ey = cy - ey
	} else {
		ey = cy + ey
	}
	return hypot(px-ex, py-ey) - w/2
}

// Blit composites a premultiplied sprite whose origin is at logical (lx, ly).
// Snaps to whole pixels (fine for motion) and honors GA.
func (c *Canvas) Blit(src *Canvas, lx, ly float32) {
	if c.GA <= 0.003 {
		return
	}
	ox := int(math.Round(float64((lx + c.OX) * c.S)))
	oy := int(math.Round(float64((ly + c.OY) * c.S)))
	ga := c.GA
	for y := 0; y < src.H; y++ {
		py := oy + y
		if py < 0 || py >= c.H {
			continue
		}
		x0, x1 := 0, src.W
		if ox+x0 < 0 {
			x0 = -ox
		}
		if ox+x1 > c.W {
			x1 = c.W - ox
		}
		si := (y*src.W + x0) * 4
		di := (py*c.W + ox + x0) * 4
		for x := x0; x < x1; x, si, di = x+1, si+4, di+4 {
			a := src.Pix[si+3] * ga
			if a <= 0 {
				continue
			}
			inv := 1 - a
			d := c.Pix[di : di+4 : di+4]
			d[0] = src.Pix[si]*ga + d[0]*inv
			d[1] = src.Pix[si+1]*ga + d[1]*inv
			d[2] = src.Pix[si+2]*ga + d[2]*inv
			d[3] = a + d[3]*inv
		}
	}
}

// ---------- images ----------

// Sample returns the bilinearly interpolated (premultiplied) color at pixel
// coordinates x, y, clamped to the edges.
func (c *Canvas) Sample(x, y float32) Col {
	x -= 0.5
	y -= 0.5
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	mx, my := float32(c.W-1), float32(c.H-1)
	if x > mx {
		x = mx
	}
	if y > my {
		y = my
	}
	x0, y0 := int(x), int(y)
	x1, y1 := x0+1, y0+1
	if x1 > c.W-1 {
		x1 = c.W - 1
	}
	if y1 > c.H-1 {
		y1 = c.H - 1
	}
	fx, fy := x-float32(x0), y-float32(y0)
	p := c.Pix
	i00, i10 := (y0*c.W+x0)*4, (y0*c.W+x1)*4
	i01, i11 := (y1*c.W+x0)*4, (y1*c.W+x1)*4
	var out [4]float32
	for k := 0; k < 4; k++ {
		a := p[i00+k] + (p[i10+k]-p[i00+k])*fx
		b := p[i01+k] + (p[i11+k]-p[i01+k])*fx
		out[k] = a + (b-a)*fy
	}
	return Col{out[0], out[1], out[2], out[3]}
}

// Resized returns a w x h copy using area averaging (good for downscaling).
func (c *Canvas) Resized(w, h int) *Canvas {
	out := NewCanvas(w, h, 1)
	sx, sy := float32(c.W)/float32(w), float32(c.H)/float32(h)
	for y := 0; y < h; y++ {
		fy0, fy1 := float32(y)*sy, float32(y+1)*sy
		for x := 0; x < w; x++ {
			fx0, fx1 := float32(x)*sx, float32(x+1)*sx
			var acc [4]float32
			var wsum float32
			for yy := int(fy0); yy < int(math.Ceil(float64(fy1))) && yy < c.H; yy++ {
				wy := min32(fy1, float32(yy+1)) - max32(fy0, float32(yy))
				for xx := int(fx0); xx < int(math.Ceil(float64(fx1))) && xx < c.W; xx++ {
					wx := min32(fx1, float32(xx+1)) - max32(fx0, float32(xx))
					wgt := wx * wy
					i := (yy*c.W + xx) * 4
					acc[0] += c.Pix[i] * wgt
					acc[1] += c.Pix[i+1] * wgt
					acc[2] += c.Pix[i+2] * wgt
					acc[3] += c.Pix[i+3] * wgt
					wsum += wgt
				}
			}
			if wsum > 0 {
				i := (y*w + x) * 4
				out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = acc[0]/wsum, acc[1]/wsum, acc[2]/wsum, acc[3]/wsum
			}
		}
	}
	return out
}

// BoxBlur blurs the canvas in place (3 box passes ~ gaussian).
func (c *Canvas) BoxBlur(r int) {
	if r <= 0 {
		return
	}
	tmp := make([]float32, len(c.Pix))
	pass := func(src, dst []float32, horiz bool) {
		n, m := c.W, c.H
		if !horiz {
			n, m = c.H, c.W
		}
		for j := 0; j < m; j++ {
			idx := func(i int) int {
				if i < 0 {
					i = 0
				}
				if i >= n {
					i = n - 1
				}
				if horiz {
					return (j*c.W + i) * 4
				}
				return (i*c.W + j) * 4
			}
			var acc [4]float32
			for i := -r; i <= r; i++ {
				k := idx(i)
				acc[0] += src[k]
				acc[1] += src[k+1]
				acc[2] += src[k+2]
				acc[3] += src[k+3]
			}
			inv := 1 / float32(2*r+1)
			for i := 0; i < n; i++ {
				o := idx(i)
				dst[o], dst[o+1], dst[o+2], dst[o+3] = acc[0]*inv, acc[1]*inv, acc[2]*inv, acc[3]*inv
				a, b := idx(i+r+1), idx(i-r)
				acc[0] += src[a] - src[b]
				acc[1] += src[a+1] - src[b+1]
				acc[2] += src[a+2] - src[b+2]
				acc[3] += src[a+3] - src[b+3]
			}
		}
	}
	for k := 0; k < 3; k++ {
		pass(c.Pix, tmp, true)
		pass(tmp, c.Pix, false)
	}
}

// Intersect two clip SDFs (either may be nil).
func ClipAnd(a, b SDF) SDF {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	return func(x, y float32) float32 { return max32(a(x, y), b(x, y)) }
}

// Blit9 draws a nine-slice sprite into the logical rect (x, y, w, h): the
// corners are copied 1:1, the edges and middle are stretched. ins is the
// corner size in sprite pixels.
func (c *Canvas) Blit9(src *Canvas, ins int, x, y, w, h float32) {
	if c.GA <= 0.003 {
		return
	}
	ox := int(math.Round(float64((x + c.OX) * c.S)))
	oy := int(math.Round(float64((y + c.OY) * c.S)))
	dw := int(math.Round(float64(w * c.S)))
	dh := int(math.Round(float64(h * c.S)))
	if dw < 2*ins || dh < 2*ins {
		return
	}
	mapAxis := func(dsize, ssize int) []int {
		m := make([]int, dsize)
		for d := range m {
			switch {
			case d < ins:
				m[d] = d
			case d >= dsize-ins:
				m[d] = ssize - (dsize - d)
			default:
				m[d] = ins + (d-ins)*(ssize-2*ins)/(dsize-2*ins)
			}
		}
		return m
	}
	mx, my := mapAxis(dw, src.W), mapAxis(dh, src.H)
	x0, x1 := max(0, -ox), min(dw, c.W-ox)
	y0, y1 := max(0, -oy), min(dh, c.H-oy)
	ga := c.GA
	clip := c.Clip
	S := c.S
	sp, dp := src.Pix, c.Pix
	for dy := y0; dy < y1; dy++ {
		py := oy + dy
		srow := my[dy] * src.W
		drow := py * c.W
		for dx := x0; dx < x1; dx++ {
			si := (srow + mx[dx]) * 4
			sa := sp[si+3]
			if sa <= 0.002 {
				continue
			}
			k := ga
			if clip != nil {
				ax, ay := (float32(ox+dx)+0.5)/S, (float32(py)+0.5)/S
				if c.needClip(ax, ay) {
					k *= clamp01(0.5 - clip(ax, ay)*S)
					if k <= 0 {
						continue
					}
				}
			}
			a := sa * k
			di := (drow + ox + dx) * 4
			inv := 1 - a
			dp[di] = sp[si]*k + dp[di]*inv
			dp[di+1] = sp[si+1]*k + dp[di+1]*inv
			dp[di+2] = sp[si+2]*k + dp[di+2]*inv
			dp[di+3] = a + dp[di+3]*inv
		}
	}
}
