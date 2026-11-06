package main

import "math"

// Visualizer styles.
const (
	VisBars = iota
	VisHalo
	VisEdge // along the bottom of a screen (its own window)
	VisThreads
	VisGlow
	VisMesh
	VisRipples
	VisWave
	VisSparks
	NumVisStyles
)

var visNames = [NumVisStyles]string{"Bars", "Halo", "Screen Edge", "Threads", "Glow Line", "Mesh", "Ripples", "Waveform", "Sparks"}

// LightBuf collects light (one intensity per pixel) from thin lines and dots
// and is added onto the glass in one pass. Overlapping strands simply add up,
// which is what makes bundles of thin lines glow.
type LightBuf struct {
	W, H   int
	S      float32
	Pix    []float32
	y0, y1 int // rows touched
}

func (l *LightBuf) Reset(w, h int, s float32) {
	l.W, l.H, l.S = w, h, s
	if cap(l.Pix) < w*h {
		l.Pix = make([]float32, w*h)
	} else {
		l.Pix = l.Pix[:w*h]
		for i := range l.Pix {
			l.Pix[i] = 0
		}
	}
	l.y0, l.y1 = h, 0
}

// add puts v at device pixel (x, y).
func (l *LightBuf) add(x, y int, v float32) {
	if x < 0 || y < 0 || x >= l.W || y >= l.H {
		return
	}
	l.Pix[y*l.W+x] += v
	if y < l.y0 {
		l.y0 = y
	}
	if y >= l.y1 {
		l.y1 = y + 1
	}
}

// Splat adds a soft dot at logical (x, y), spread over 4 pixels.
func (l *LightBuf) Splat(x, y, v float32) {
	fx, fy := x*l.S-0.5, y*l.S-0.5
	ix, iy := int(math.Floor(float64(fx))), int(math.Floor(float64(fy)))
	tx, ty := fx-float32(ix), fy-float32(iy)
	l.add(ix, iy, v*(1-tx)*(1-ty))
	l.add(ix+1, iy, v*tx*(1-ty))
	l.add(ix, iy+1, v*(1-tx)*ty)
	l.add(ix+1, iy+1, v*tx*ty)
}

// Line draws a thin anti-aliased line (logical coordinates).
func (l *LightBuf) Line(x0, y0, x1, y1, v float32) {
	dx, dy := (x1-x0)*l.S, (y1-y0)*l.S
	n := int(math.Ceil(float64(max32(abs32(dx), abs32(dy)))))
	if n < 1 {
		n = 1
	}
	inv := 1 / float32(n)
	for i := 0; i <= n; i++ {
		t := float32(i) * inv
		l.Splat(x0+(x1-x0)*t, y0+(y1-y0)*t, v)
	}
}

// Composite adds the light onto c in color col (only where there's glass).
func (l *LightBuf) Composite(c *Canvas, col Col, strength float32) {
	if l.y1 <= l.y0 || strength <= 0 {
		return
	}
	w := l.W
	y0, y1 := l.y0, l.y1
	parallelRows(y1-y0, func(a, b int) {
		for y := y0 + a; y < y0+b; y++ {
			row := l.Pix[y*w : y*w+w]
			for x, v := range row {
				if v <= 0.002 {
					continue
				}
				i := (y*c.W + x) * 4
				al := c.Pix[i+3]
				if al <= 0 {
					continue
				}
				k := (1 - float32(math.Exp(float64(-v)))) * strength * al
				c.Pix[i] = min32(al, c.Pix[i]+col.R*k)
				c.Pix[i+1] = min32(al, c.Pix[i+1]+col.G*k)
				c.Pix[i+2] = min32(al, c.Pix[i+2]+col.B*k)
			}
		}
	})
}

// CompositeFn is Composite with a color per pixel (device coordinates in).
func (l *LightBuf) CompositeFn(c *Canvas, colAt func(x, y int) Col, strength float32) {
	if l.y1 <= l.y0 || strength <= 0 {
		return
	}
	w := l.W
	y0, y1 := l.y0, l.y1
	parallelRows(y1-y0, func(a, b int) {
		for y := y0 + a; y < y0+b; y++ {
			row := l.Pix[y*w : y*w+w]
			for x, v := range row {
				if v <= 0.002 {
					continue
				}
				i := (y*c.W + x) * 4
				al := c.Pix[i+3]
				if al <= 0 {
					continue
				}
				col := colAt(x, y)
				k := (1 - float32(math.Exp(float64(-v)))) * strength * al
				c.Pix[i] = min32(al, c.Pix[i]+col.R*k)
				c.Pix[i+1] = min32(al, c.Pix[i+1]+col.G*k)
				c.Pix[i+2] = min32(al, c.Pix[i+2]+col.B*k)
			}
		}
	})
}

// waveColorFn: the waveform's colors (Custom, RGB, Gradient, Liquid) for a
// canvas at scale s. Coordinates are device pixels.
func (u *UI) waveColorFn(s, PW float32) func(x, y int) Col {
	t := u.T
	custom := u.WaveCol.Mix(White(1), 0.08)
	ac := u.accent.Mix(White(1), 0.4)
	switch u.WaveMode {
	case WaveCustom:
		return func(x, y int) Col { return custom }
	case WaveRGB:
		return func(x, y int) Col {
			fx := float32(x) / s / PW
			return HSV(fx*0.8+t*0.08, 0.75, 1)
		}
	case WaveGradient: // your color in the middle, the accent at the ends
		return func(x, y int) Col {
			d := abs32(float32(x)/s/PW-0.5) * 2
			return lerpCol(custom, ac, d*d*(3-2*d))
		}
	case WaveLiquid: // flowing fluid colors inside the shape
		pal := u.fluidColors()
		c0, c1, c2 := pal[0].Mix(White(1), 0.1), pal[1].Mix(White(1), 0.1), pal[2].Mix(White(1), 0.1)
		return func(x, y int) Col {
			fx, fy := float32(x)/s/60, float32(y)/s/60
			a := 0.5 + 0.5*sinf(fx*1.3+t*0.7+sinf(fy*1.1+t*0.4))
			b := 0.5 + 0.5*sinf(fy*1.7-t*0.5+sinf(fx*0.9-t*0.3))
			return lerpCol(lerpCol(c0, c1, a), c2, b*0.7)
		}
	}
	return func(x, y int) Col { return ac }
}

// levelN: a level relative to how loud the song has been lately, so quiet
// songs (or a low Spotify volume) still move the visuals fully.
func (u *UI) levelN(i int) float32 {
	r := u.peakRef
	if r <= 0 {
		r = 1
	}
	return clamp01(u.smoothLevel(i) / r * u.visGain())
}

// smoothLevel: a level averaged with the ones just before it (Smoothness).
func (u *UI) smoothLevel(i int) float32 {
	k := int(u.VisSmooth*6 + 0.5)
	if k <= 0 {
		return u.level(i)
	}
	var s, w float32
	for j := 0; j <= k; j++ {
		wt := 1 - float32(j)/float32(k+1)
		s += u.level(i+j) * wt
		w += wt
	}
	return s / w
}

// visGain: Sensitivity, 0.5x .. 1x (middle) .. 2.5x.
func (u *UI) visGain() float32 {
	if u.VisSens < 0.5 {
		return 0.5 + u.VisSens
	}
	return 1 + (u.VisSens-0.5)*3
}

// visRaw: a level for the bar styles (not normalized, with Sensitivity and
// Smoothness).
func (u *UI) visRaw(i int) float32 {
	return clamp01(u.smoothLevel(i) * 1.15 * u.visGain())
}

// energyN: recent loudness, normalized the same way.
func (u *UI) energyN(from, n int) float32 {
	var s float32
	for i := from; i < from+n; i++ {
		s += u.levelN(i)
	}
	return clamp01(s / float32(n))
}

// energy: recent loudness (0..1), smoothed over the last few hundred ms.
func (u *UI) energy(from, n int) float32 {
	var s float32
	for i := from; i < from+n; i++ {
		s += u.level(i)
	}
	return clamp01(s / float32(n))
}

// drawVisualizer draws the chosen in-player style behind everything.
func (u *UI) drawVisualizer(c *Canvas) {
	a := c.GA
	if a < 0.01 {
		return
	}
	switch u.VisStyle {
	case VisBars:
		u.bottomVisualizer(c)
		return
	case VisEdge:
		return // drawn in its own window along the screen
	}
	l := &u.light
	l.Reset(c.W, c.H, c.S)
	col := u.accent.Mix(White(1), 0.45)
	switch u.VisStyle {
	case VisHalo:
		u.visHalo(l)
	case VisThreads:
		u.visThreads(l)
		col = u.accent.Mix(White(1), 0.6)
	case VisGlow:
		u.visGlowLine(l)
		col = u.accent.Mix(White(1), 0.55)
	case VisMesh:
		u.visMesh(l)
		col = u.accent.Mix(White(1), 0.6)
	case VisRipples:
		u.visRipples(l)
		col = u.accent.Mix(White(1), 0.5)
	case VisWave:
		u.visWave(l)
		if u.WaveMode != WaveTheme {
			l.CompositeFn(c, u.waveColorFn(c.S, u.PW), a*0.9)
			return
		}
		col = u.accent.Mix(White(1), 0.5)
	case VisSparks:
		u.visSparks(l)
		col = u.accent.Mix(White(1), 0.55)
	}
	l.Composite(c, col, a*0.9)
}

// Halo: rays around a ring in the middle of the player.
func (u *UI) visHalo(l *LightBuf) {
	cx, cy := u.PW/2, u.PH/2-2
	r0 := min32(u.PW, u.PH) * u.visP(VisHalo, 0, 0.16, 0.30, 0.40)
	e := u.energyN(0, 4)
	r0 *= 1 + 0.08*e
	rays := u.visP(VisHalo, 1, 12, 30, 60)
	const n = 120
	for i := 0; i < n; i++ {
		ang := float64(i) / n * 2 * math.Pi
		// mirror left/right so the halo is symmetric; older levels toward the bottom
		k := int(math.Abs(math.Remainder(ang, 2*math.Pi)) / math.Pi * 36)
		lv := float32(math.Sqrt(float64(u.levelN(k))))
		ln := 3 + rays*lv
		sx, sy := float32(math.Sin(ang)), -float32(math.Cos(ang))
		l.Line(cx+sx*r0, cy+sy*r0, cx+sx*(r0+ln), cy+sy*(r0+ln), 0.55)
		l.Line(cx+sx*r0+sy*0.6, cy+sy*r0-sx*0.6, cx+sx*(r0+ln)+sy*0.6, cy+sy*(r0+ln)-sx*0.6, 0.35)
	}
	// the ring itself, brighter when it's loud
	steps := int(2 * math.Pi * float64(r0*l.S))
	for i := 0; i < steps; i++ {
		ang := float64(i) / float64(steps) * 2 * math.Pi
		sx, sy := float32(math.Sin(ang)), -float32(math.Cos(ang))
		for _, d := range [...]float32{-2.5, 0, 2.5} {
			v := float32(0.5)
			if d != 0 {
				v = 0.12 + 0.2*e
			}
			l.Splat(cx+sx*(r0+d-1), cy+sy*(r0+d-1), v)
		}
	}
}

// musicWave: a smooth wave across the player whose shape and size follow the
// music. x in 0..1, delay in levels (older = later strands).
func (u *UI) musicWave(x float32, delay int) float32 {
	t := u.visPhase
	var y float32
	for m := 1; m <= 4; m++ {
		lv := u.levelN(delay + m*2)
		amp := (0.08 + 0.92*lv*lv) / float32(m) * 1.4
		y += amp * sinf(x*float32(m)*5.1+t*(0.9+0.45*float32(m))+float32(m)*1.7)
	}
	env := sinf(x*math.Pi)*0.75 + 0.25
	return y * env * 0.6
}

// Threads: a bundle of thin strands that move together, each a little late.
func (u *UI) visThreads(l *LightBuf) {
	PW, PH := u.PW, u.PH
	yc := PH * 0.56
	e := u.energyN(0, 4)
	amp := PH * (0.10 + 0.32*e) // loud parts swing much wider
	n := int(u.visP(VisThreads, 0, 40, 150, 280))
	spread := u.visP(VisThreads, 1, 0.3, 1, 2.2)
	const delays = 10
	step := 3 / l.S * 1.5
	ns := int(PW/step) + 2
	// the music's wave, worked out once per delay (strands share them)
	if len(u.waveCache) < delays*ns {
		u.waveCache = make([]float32, delays*ns)
	}
	for d := 0; d < delays; d++ {
		for k := 0; k < ns; k++ {
			u.waveCache[d*ns+k] = u.musicWave(float32(k)*step/PW, d)
		}
	}
	for j := 0; j < n; j++ {
		h := float32(grain(j, 7) + 0.5) // stable per-strand randomness
		g := float32(grain(j, 13) + 0.5)
		off := (h - 0.5) * (24 + 40*e) * spread // the bundle spreads with the beat
		sc := 0.65 + 0.6*g
		delay := min(int(h*delays), delays-1)
		ph := g * 6.28
		v := 0.03 + 0.035*g
		wave := u.waveCache[delay*ns : delay*ns+ns]
		o1 := off * (0.6 + 0.4*sinf(u.T*0.4+ph))
		var px, py float32
		for k := 0; k < ns; k++ {
			x := float32(k) * step
			fx := x / PW
			y := yc + o1 + amp*sc*wave[k] + 3*sinf(fx*17*g+u.T*1.3+ph)
			if k > 0 {
				l.Line(px, py, x, y, v)
			}
			px, py = x, y
		}
	}
}

// Glow Line: one smooth line with a soft glow around it.
func (u *UI) visGlowLine(l *LightBuf) {
	PW, PH := u.PW, u.PH
	S := l.S
	yc := PH * 0.56
	amp := PH * (0.08 + 0.36*u.energyN(0, 4)) * u.visP(VisGlow, 1, 0.4, 1, 1.6)
	gk := u.visP(VisGlow, 0, 0.25, 1, 2.2)
	reach := 64 * S * max32(1, gk)
	// brightness by distance from the line, looked up in quarter pixels
	nt := int(reach*4) + 2
	if len(u.glowLUT) != nt || u.glowS != S || u.glowK != gk {
		u.glowK = gk
		u.glowLUT = make([]float32, nt)
		u.glowS = S
		for i := range u.glowLUT {
			d := float64(i) / 4
			core := math.Exp(-(d * d) / float64(0.9*S*S))
			glow := (0.30*math.Exp(-d/float64(9*S)) + 0.10*math.Exp(-d/float64(30*S*gk))) * float64(gk)
			u.glowLUT[i] = float32(core*2.4 + glow)
		}
	}
	lut := u.glowLUT
	prev := (yc + amp*u.musicWave(0, 0)) * S
	for x := 0; x < l.W; x++ {
		fx := (float32(x) + 1.5) / S / PW
		next := (yc + amp*u.musicWave(fx, 0)) * S
		f := (prev + next) / 2
		slope := next - prev
		prev = next
		k := 1 / float32(math.Sqrt(float64(1+slope*slope)))
		ya := max(0, int(f-reach))
		yb := min(l.H-1, int(f+reach))
		for y := ya; y <= yb; y++ {
			d := abs32(float32(y)+0.5-f) * k * 4
			i := int(d)
			if i >= nt {
				continue
			}
			l.add(x, y, lut[i])
		}
	}
}

// Mesh: a dotted 3D blob that turns slowly and pulses with the music.
func (u *UI) visMesh(l *LightBuf) {
	cx, cy := u.PW/2, u.PH/2-2
	e := u.energyN(0, 3)     // the beat: makes it pulse
	slow := u.energyN(0, 16) // the song's loudness: makes it wobble
	R := min32(u.PW, u.PH) * (0.26 + 0.07*e) * u.visP(VisMesh, 0, 0.6, 1, 1.35)
	t := float64(u.visPhase)
	const rings, pts = 44, 150
	ry, rx := t*0.35, 0.45+0.15*math.Sin(float64(u.T)*0.2)
	cyw, syw := math.Cos(ry), math.Sin(ry)
	cxw, sxw := math.Cos(rx), math.Sin(rx)
	lob := (0.10 + 0.30*float64(slow)) * float64(u.visP(VisMesh, 1, 0.2, 1, 2.2))
	for i := 1; i < rings; i++ {
		th := float64(i) / rings * math.Pi
		st, ct := math.Sin(th), math.Cos(th)
		// each ring takes a slightly older level: waves run over the blob
		rl := float64(u.levelN(int(th / math.Pi * 14)))
		for j := 0; j < pts; j++ {
			ph := float64(j) / pts * 2 * math.Pi
			w := 1 + lob*math.Sin(3*ph+t*0.8+2*ct) + 0.12*math.Sin(2*th+t*1.1) +
				0.22*rl*math.Sin(4*ph-t*1.9)*st
			x, y, z := st*math.Cos(ph)*w, ct*w, st*math.Sin(ph)*w
			x, z = x*cyw+z*syw, -x*syw+z*cyw // turn
			y, z = y*cxw-z*sxw, y*sxw+z*cxw  // tilt
			p := 1 / (1 + z*0.25)            // perspective
			v := float32(0.16+0.30*(1-(z+1.3)/2.6)) * (0.7 + 0.6*e)
			l.Splat(cx+float32(x*p)*R, cy+float32(y*p)*R, max32(0.05, v))
		}
	}
}

// levelAt: a level at a fractional age (smooth between samples).
func (u *UI) levelAt(f float32) float32 {
	i := int(f)
	t := f - float32(i)
	return u.levelN(i)*(1-t) + u.levelN(i+1)*t
}

// ring adds a circle of light (wobbling a little with w) around cx, cy.
func (l *LightBuf) ring(cx, cy, r, v, w, ph float32) {
	steps := int(2 * math.Pi * float64(r*l.S))
	if steps < 12 {
		steps = 12
	}
	for i := 0; i < steps; i++ {
		a := float32(i) / float32(steps) * 2 * math.Pi
		rr := r * (1 + w*sinf(5*a+ph))
		l.Splat(cx+rr*sinf(a), cy-rr*float32(math.Cos(float64(a))), v)
	}
}

// Ripples: rings that keep spreading out from the middle. Each ring carries
// how loud the music was when it left, so beats travel outward as bright
// rings.
func (u *UI) visRipples(l *LightBuf) {
	cx, cy := u.PW/2, u.PH/2-2
	m := min32(u.PW, u.PH)
	r0 := m * 0.10
	maxR := float32(math.Hypot(float64(u.PW), float64(u.PH))) * 0.5
	n := int(u.visP(VisRipples, 1, 6, 14, 26))
	gap := (maxR - r0) / float32(n)
	drift := u.visPhase * 0.55 * u.visP(VisRipples, 0, 0.3, 1, 2.4)
	frac := drift - float32(math.Floor(float64(drift)))
	base := int(math.Floor(float64(drift)))
	for k := 0; k <= n; k++ {
		age := float32(k) + frac
		r := r0 + age*gap
		lv := u.levelAt(age * 2.2)
		fade := 1 - age/float32(n+1)
		if k == 0 {
			fade *= frac // a new ring fades in at the middle
		}
		v := (0.06 + 0.75*lv*lv) * fade * fade
		w := 0.012 * lv
		ph := float32(k-base) * 1.3 // each ring wobbles its own way
		l.ring(cx, cy, r, v, w, ph+u.T*0.6)
		if lv > 0.35 { // loud rings get a soft echo
			l.ring(cx, cy, r+2.2, v*0.35, w, ph+u.T*0.6)
		}
	}
	// a small glowing core that beats
	e := u.energyN(0, 3)
	l.ring(cx, cy, r0*(0.55+0.25*e), 0.25+0.45*e, 0, 0)
}

// Waveform: a mirrored shape through the middle, newest sound in the center
// and older sound flowing out to both sides.
func (u *UI) visWave(l *LightBuf) {
	PW, PH := u.PW, u.PH
	yc := PH * 0.55
	maxH := PH * u.visP(VisWave, 0, 0.12, 0.30, 0.46)
	fill := u.visP(VisWave, 1, 0, 1, 3)
	half := PW / 2
	S := l.S
	height := func(x float32) (float32, float32) {
		d := min32(1, abs32(x-half)/half) // 0 middle, 1 edges
		age := d * 26
		lv := (u.levelAt(age) + u.levelAt(age+0.8) + u.levelAt(age+1.6)) / 3 // smoothed
		return maxH * (0.03 + 0.97*lv*lv) * (1 - d*d), lv
	}
	var pt, pb, px float32
	for dx := 0; dx < l.W; dx++ {
		x := (float32(dx) + 0.5) / S
		h, lv := height(x)
		// fill: one soft column per pixel
		v := (0.035 + 0.06*lv) * fill
		for y := int((yc - h) * S); y <= int((yc+h)*S); y++ {
			l.add(dx, y, v)
		}
		top, bot := yc-h, yc+h
		if dx > 0 {
			l.Line(px, pt, x, top, 0.55)
			l.Line(px, pb, x, bot, 0.55)
		}
		px, pt, pb = x, top, bot
	}
	l.Line(0, yc, PW, yc, 0.12) // the center line
}

// Sparks: embers rising through the glass. They speed up and burn brighter
// when the music gets louder.
func (u *UI) visSparks(l *LightBuf) {
	PW, PH := u.PW, u.PH
	n := int(min32(160, 40+PW*PH/2200) * u.visP(VisSparks, 0, 0.25, 1, 2.6))
	glow := u.visP(VisSparks, 1, 0.4, 1, 2.2)
	e := u.energyN(0, 3)
	span := PH + 40
	for j := 0; j < n; j++ {
		g := grain(j, 3) + 0.5
		h := grain(j, 11) + 0.5
		sp := 0.45 + 0.9*g
		f := h + u.visPhase*0.07*sp
		f -= float32(math.Floor(float64(f)))
		y := PH + 20 - f*span
		x := (grain(j, 5)+0.5)*PW + 9*sinf(u.T*0.8*sp+h*20) + 14*(f-0.5)*(g-0.5)
		edge := clamp01(min32(f*6, (1-f)*4)) // fade in at the bottom, out at the top
		lv := u.levelN(j % 20)
		v := (0.18 + 0.8*lv) * edge * (0.55 + 0.45*e)
		tail := 1 + 4*lv*sp
		l.Line(x, y, x-0.8*sinf(h*9), y+tail, v*0.3)
		// a soft round ember
		sz := (0.9 + 1.3*g) * glow
		l.Splat(x, y, v*2.2)
		for k := 0; k < 8; k++ {
			a := float32(k) * (math.Pi / 4)
			ca, sa := float32(math.Cos(float64(a))), sinf(a)
			l.Splat(x+ca*sz*0.6, y+sa*sz*0.6, v*0.5)
			l.Splat(x+ca*sz*1.3, y+sa*sz*1.3, v*0.12*glow)
		}
	}
}

// RenderEdge draws the Screen Edge visualizer: bars along the bottom of a
// screen, newest in the middle, rippling outward. c is the edge window.
func (u *UI) RenderEdge(c *Canvas) {
	u.scale = c.S
	c.ResetState()
	c.Clear()
	W, H := float32(c.W)/c.S, float32(c.H)/c.S
	if u.EdgeStyle == EdgeWave {
		u.edgeWave(c, W, H)
		return
	}
	step := float32(9)
	n := int(W/step) + 1
	mid := W / 2
	ac := u.accent
	hk := u.visP(VisEdge, 0, 0.35, 1, 1)
	for i := 0; i < n; i++ {
		x := float32(i)*step + step/2
		k := int(abs32(x-mid) / step / 2.2)
		lv := u.visRaw(k)
		v := float32(math.Sqrt(float64(lv)))
		h := 2 + (H-6)*hk*v*(1-0.3*abs32(x-mid)/mid)
		top := H - h
		col := ac.Mix(White(1), 0.35-0.3*v)
		c.RoundRect(x-2.2, top, 4.4, h+6, 2.2, VGrad(top, H, col.WithA(0.25+0.55*v), col.WithA(0.10+0.25*v)))
	}
}

// edgeWave: the Screen Edge as a waveform rising from the bottom of the
// screen (newest sound in the middle), drawn straight into the pixels so it
// stays cheap on wide screens. The window is click-through and transparent.
func (u *UI) edgeWave(c *Canvas, W, H float32) {
	S := c.S
	hk := u.visP(VisEdge, 0, 0.35, 1, 1)
	half := W / 2
	colFn := u.waveColorFn(S, W)
	if u.WaveMode == WaveTheme {
		ac := u.accent.Mix(White(1), 0.35)
		colFn = func(x, y int) Col { return ac }
	}
	fill := u.visP(VisWave, 1, 0.3, 1, 2)
	prev := float32(-1)
	for dx := 0; dx < c.W; dx++ {
		x := (float32(dx) + 0.5) / S
		d := min32(1, abs32(x-half)/half)
		age := d * 34
		at := func(a float32) float32 { // smooth between samples
			i := int(a)
			f := a - float32(i)
			return u.visRaw(i)*(1-f) + u.visRaw(i+1)*f
		}
		lv := (at(age) + at(age+1) + at(age+2)) / 3
		h := (2 + (H-4)*hk*float32(math.Sqrt(float64(lv)))*(1-0.35*d)) * S
		top := float32(c.H) - h
		// the outline: brightest at the top edge, joined to the last column
		lo, hi := top, top
		if prev >= 0 {
			lo, hi = min32(prev, top), max32(prev, top)
		}
		prev = top
		for y := max(0, int(lo-1.5*S)); y < c.H; y++ {
			fy := float32(y) + 0.5
			var a float32
			switch {
			case fy < lo-1.2*S:
				a = clamp01(1-(lo-fy)/(1.2*S)) * 0.9
			case fy <= hi+1.2*S:
				a = 0.9
			default: // the body fades toward the bottom of the screen
				t := (fy - hi) / max32(1, float32(c.H)-hi)
				a = (0.32 - 0.22*t) * fill
			}
			if a <= 0.003 {
				continue
			}
			col := colFn(dx, y)
			k := (y*c.W + dx) * 4
			a = min32(1, a)
			c.Pix[k] = col.R * a
			c.Pix[k+1] = col.G * a
			c.Pix[k+2] = col.B * a
			c.Pix[k+3] = a
		}
	}
}
