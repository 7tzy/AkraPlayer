package main

import (
	"fmt"
	"math"
	"strings"
)

// ---------- hit testing ----------

func (u *UI) addHit(c *Canvas, id int, x0, y0, x1, y1 float32) {
	if c.GA < 0.5 {
		return
	}
	ax0, ay0, ax1, ay1 := x0+c.OX, y0+c.OY, x1+c.OX, y1+c.OY
	if c.Clip != nil && c.Clip((ax0+ax1)/2, (ay0+ay1)/2) > 0 {
		return // scrolled out of view
	}
	u.hits = append(u.hits, hitArea{id, ax0, ay0, ax1, ay1, false})
}

func (u *UI) addHitC(c *Canvas, id int, cx, cy, r float32) {
	if c.GA < 0.5 {
		return
	}
	if c.Clip != nil && c.Clip(cx+c.OX, cy+c.OY) > 0 {
		return
	}
	u.hits = append(u.hits, hitArea{id, cx + c.OX, cy + c.OY, r, 0, true})
}

func (u *UI) Hit(x, y float32) int {
	for i := len(u.hits) - 1; i >= 0; i-- {
		h := u.hits[i]
		if h.circle {
			if hypot(x-h.x0, y-h.y0) <= h.x1 {
				return h.id
			}
		} else if x >= h.x0 && x <= h.x1 && y >= h.y0 && y <= h.y1 {
			return h.id
		}
	}
	return hNone
}

func (u *UI) InPanel(x, y float32) bool { return sdRoundRect(x, y, 0, 0, u.PW, u.PH, u.Radius()) <= 0 }

// VSliderValue: vertical sliders (bottom = 0, top = 1).
func (u *UI) VSliderValue(id int, y float32) float32 {
	g, ok := u.vsliders[id]
	if !ok {
		return 0
	}
	return clamp01((g[1] - y) / (g[1] - g[0]))
}

func (u *UI) IsVSlider(id int) bool { _, ok := u.vsliders[id]; return ok }

func (u *UI) SliderValue(id int, x float32) float32 {
	g, ok := u.sliders[id]
	if !ok {
		return 0
	}
	return clamp01((x - g[0]) / (g[1] - g[0]))
}

// Wheel routes the scroll wheel (delta > 0 = down). Returns true if used.
func (u *UI) Wheel(delta float32) bool {
	switch {
	case u.PickOpen:
		u.pickScroll.Scroll(delta, 0)
	case u.Page == 1 && u.page.X > 0.5:
		dir := u.secScroll[u.Section].Scroll(delta, u.scrollLimit())
		next := u.Section + dir
		switch {
		case dir == 0:
		case next < 0 || next >= NumSections:
			// past the first or last page: close settings (not in its own window)
			u.secScroll[u.Section].Over = 0
			if !u.SetPopView {
				u.Page = 0
			}
		default:
			u.Section = next
			u.flipHold = 0.28 // let the pill show "full" for a moment
			u.pushF.Target = 1
			if dir > 0 {
				u.secScroll[next].Pos.Snap(0)
			} else {
				u.secScroll[next].Pos.Snap(max32(0, u.secScroll[next].Max))
			}
			u.secScroll[next].Over = 0
		}
	case u.Sheet == SheetControls:
		u.mixScroll.Scroll(delta, 0) // the mixer scrolls sideways
	case u.Sheet != SheetNone:
		u.sheetScroll.Scroll(delta, 0)
	case u.Page == 0 && !u.Mini && !u.HueOpen:
		// scrolling down on the player opens Settings (with its own resistance)
		if delta < 0 {
			u.playScroll.Over = 0
			return true
		}
		if u.playScroll.Scroll(delta, u.openLimit()) > 0 {
			u.playScroll.Over = 0
			u.Page, u.Section = 1, SecGeneral
			u.secScroll[SecGeneral].Pos.Snap(0)
			u.secScroll[SecGeneral].Over = 0
			u.nav.Snap(SecGeneral)
			u.sec.Snap(SecGeneral)
		}
	default:
		return false
	}
	return true
}

// ---------- text ----------

func (u *UI) maskF(s string, size float32, weight, font int) *Mask {
	key := fmt.Sprintf("%s\x00%.2f\x00%d\x00%d\x00%.3f", s, size, weight, font, u.scale)
	if m, ok := u.cache[key]; ok {
		return m
	}
	if len(u.cache) > 700 || u.maskBytes > 48<<20 { // keep memory in check
		u.cache = map[string]*Mask{}
		u.maskBytes = 0
	}
	m := u.Text.Render(s, size*u.scale, weight, font)
	if m != nil {
		u.maskBytes += len(m.A) * 4
	}
	u.cache[key] = m
	return m
}

func (u *UI) mask(s string, size float32, weight int) *Mask { return u.maskF(s, size, weight, u.Font) }

// TextMeasurer: renderers that can measure text without drawing it (fast).
type TextMeasurer interface {
	Measure(s string, px float32, weight, font int) float32
}

// textW measures without rendering when it can: fitting long titles tries
// many lengths, and rendering every try (bigger the bigger the window) was
// what made large windows slow.
func (u *UI) textW(s string, size float32, weight int) float32 {
	if tm, ok := u.Text.(TextMeasurer); ok {
		key := fmt.Sprintf("%s\x00%.2f\x00%d\x00%d\x00%.3f", s, size, weight, u.Font, u.scale)
		if w, ok := u.widths[key]; ok {
			return w
		}
		if len(u.widths) > 4000 {
			u.widths = map[string]float32{}
		}
		w := tm.Measure(s, size*u.scale, weight, u.Font) / u.scale
		u.widths[key] = w
		return w
	}
	m := u.mask(s, size, weight)
	if m == nil {
		return 0
	}
	return float32(m.W) / u.scale
}

// marquee: text that doesn't fit slides slowly to show the rest (and back),
// instead of being cut off with "…".
func (u *UI) marquee(c *Canvas, s string, x, cy, size float32, weight int, col Col, maxW float32) {
	w := u.textW(s, size, weight)
	if w <= maxW {
		u.text(c, s, x, cy, size, weight, col, 0, false)
		return
	}
	u.marqueeOn = true
	over := w - maxW + 6
	move := over / 28 // seconds to slide across
	cycle := 2*1.6 + 2*move
	t := float32(math.Mod(float64(u.T), float64(cycle)))
	var f float32
	switch {
	case t < 1.6:
		f = 0
	case t < 1.6+move:
		f = (t - 1.6) / move
	case t < 3.2+move:
		f = 1
	default:
		f = 1 - (t-3.2-move)/move
	}
	f = f * f * (3 - 2*f)
	c.PushClip(x, cy-size, x+maxW, cy+size, 0)
	u.text(c, s, x-over*f, cy, size, weight, col, 0, false)
	c.PopClip()
}

// tip registers a tooltip for an area (shown in a bubble under the window).
func (u *UI) tip(c *Canvas, text string, x0, y0, x1, y1 float32) {
	if c.GA < 0.5 || text == "" {
		return
	}
	ax0, ay0, ax1, ay1 := x0+c.OX, y0+c.OY, x1+c.OX, y1+c.OY
	if c.Clip != nil && c.Clip((ax0+ax1)/2, (ay0+ay1)/2) > 0 {
		return
	}
	u.tips = append(u.tips, tipZone{text, ax0, ay0, ax1, ay1})
}

// TipAt: the tooltip text under a point (logical), or "".
func (u *UI) TipAt(x, y float32) string {
	for i := len(u.tips) - 1; i >= 0; i-- {
		t := u.tips[i]
		if x >= t.x0 && x <= t.x1 && y >= t.y0 && y <= t.y1 {
			return t.text
		}
	}
	return ""
}

func (u *UI) fit(s string, size float32, weight int, maxW float32) string {
	if u.textW(s, size, weight) <= maxW {
		return s
	}
	r := []rune(s)
	lo, hi := 0, len(r)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if u.textW(strings.TrimSpace(string(r[:mid]))+"…", size, weight) <= maxW {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return strings.TrimSpace(string(r[:lo])) + "…"
}

func (u *UI) text(c *Canvas, s string, x, cy, size float32, weight int, col Col, align int, halo bool) float32 {
	return u.textF(c, s, x, cy, size, weight, u.Font, col, align, halo)
}

// textF draws s centered vertically on cy. align: 0 left, 1 center, 2 right.
func (u *UI) textF(c *Canvas, s string, x, cy, size float32, weight, font int, col Col, align int, halo bool) float32 {
	m := u.maskF(s, size, weight, font)
	if m == nil || m.W == 0 {
		return 0
	}
	w := float32(m.W) / u.scale
	h := float32(m.H) / u.scale
	switch align {
	case 1:
		x -= w / 2
	case 2:
		x -= w
	}
	y := cy - h/2
	if halo {
		o := float32(0.9)
		for _, d := range [][2]float32{{-o, 0}, {o, 0}, {0, -o}, {0, o}} {
			c.DrawMask(m, x+d[0], y+d[1]+0.6, Black(0.10*col.A))
		}
		c.DrawMask(m, x, y+1, Black(0.20*col.A))
	}
	c.DrawMask(m, x, y, col)
	return w
}

var (
	txt  = Col{0.98, 0.98, 0.99, 1}
	sub  = Col{0.94, 0.95, 0.97, 0.78}
	dim  = Col{0.92, 0.93, 0.96, 0.56}
	glyf = Col{1, 1, 1, 0.96}
)

func (u *UI) onAccent() Col {
	if u.accent.Lum() > 0.62 {
		return Col{0.08, 0.08, 0.09, 0.92}
	}
	return White(1)
}

// ---------- render ----------

func (u *UI) Render(c *Canvas) {
	u.scale = c.S
	if len(u.sprites) > 160 || u.spriteBytes > 96<<20 { // keep memory in check
		u.sprites = map[string]*Canvas{}
		u.spriteBytes = 0
	}
	u.hits = u.hits[:0]
	u.tips = u.tips[:0]
	u.marqueeOn = false
	for k := range u.vsliders {
		delete(u.vsliders, k)
	}
	c.ResetState()
	u.drawBase(c)
	if !u.Mini && !u.Full && !u.SetPopView {
		u.drawResizeCorners(c) // every page; first, so buttons near the corners win
	}

	mini := clamp01(u.miniA.V)
	if pa := 1 - mini; pa > 0.004 {
		p := u.page.X
		sh := clamp01(u.sheetA.X)
		pull := u.playScroll.Rubber(u.openLimit()) // scrolling down pulls the player up a little
		if a := clamp01(1 - p*1.7); a > 0.004 {
			if u.tog[TogVis].V > 0.01 {
				c.GA = pa * a * clamp01(1-sh*1.2) * u.tog[TogVis].V
				u.drawVisualizer(c)
			}
			// the player recedes while a sheet slides up over it
			c.OX, c.OY, c.GA = -p*u.PW*0.38, pull-sh*18, pa*a*clamp01(1-sh*1.4)
			u.drawPlayer(c)
			if sh > 0.004 {
				c.OX, c.OY, c.GA = -p*u.PW*0.38, (1-u.sheetA.X)*u.PH*0.75, pa*a*clamp01(sh*1.5-0.2)
				which := u.Sheet
				if which == SheetNone {
					which = u.lastSheet // still sliding away
				}
				u.drawSheet(c, which)
			}
		}
		if a := clamp01((p - 0.38) * 1.62); a > 0.004 {
			c.OX, c.OY, c.GA = (1-p)*u.PW*0.62, 0, pa*a
			u.drawSettings(c)
		}
		c.ResetState()
		c.GA = pa
		u.drawPushPill(c)
	}
	if mini > 0.004 {
		c.ResetState()
		c.GA = mini * mini
		u.drawMini(c)
	}
	if !u.PoppedOut || u.SetPopView { // with Settings popped out, its pickers show there
		c.ResetState()
		u.drawPicker(c)
		c.ResetState()
		u.drawHuePicker(c)
	}
	if u.toastSet == u.SetPopView {
		c.ResetState()
		u.drawToast(c)
	}
	c.ResetState()
	u.drawTooltip(c)
}

// putSprite caches a sprite and keeps count of the memory it uses.
func (u *UI) putSprite(k string, sp *Canvas) {
	if sp != nil {
		u.spriteBytes += len(sp.Pix) * 4
	}
	u.sprites[k] = sp
}

// drawPushPill: the glass "keep scrolling" pill. It fills as you push past
// the end of a page, then slides away.
func (u *UI) drawPushPill(c *Canvas) {
	show := u.pushA.V
	if show < 0.01 {
		return
	}
	var label string
	var kx, ky, kw, kh float32
	if u.Page == 0 && u.page.X < 0.5 {
		label = "Keep scrolling for Settings"
		kx, ky, kw, kh = 0, 0, u.PW, u.PH
		if u.flipHold > 0 {
			label = "Settings"
		}
	} else {
		kx, ky, kw, kh = 158, 58, u.PW-172, u.PH-72
		next := u.Section + u.pushDir
		label = "Keep scrolling to close settings"
		switch {
		case u.flipHold > 0:
			label = secLabels[u.Section]
		case next >= 0 && next < NumSections:
			label = "Keep scrolling for " + secLabels[next]
		}
	}
	bw := u.textW(label, 10.5, 600) + 34
	bh := float32(28)
	bx := kx + kw/2 - bw/2
	e := show * show * (3 - 2*show)
	y := ky + kh - 20 + (1-e)*30 // rises from the bottom edge
	if u.pushDir < 0 {
		y = ky + 20 - (1-e)*30
	}
	ga := c.GA
	c.PushClip(kx+1, ky+1, kx+kw-1, ky+kh-1, 21)
	c.GA = ga * e
	// glass: the same card as the rest of the app, then the fill
	u.card(c, bx, y-bh/2, bw, bh, bh/2)
	c.RoundRect(bx+0.5, y-bh/2+0.5, bw-1, bh-1, bh/2, VGrad(y-bh/2, y+bh/2, Col{0.26, 0.27, 0.30, 0.55}, Col{0.14, 0.15, 0.17, 0.62}))
	if fill := clamp01(u.pushF.X); fill > 0.005 {
		fw := max32(bh, bw*fill)
		c.RoundRect(bx, y-bh/2, fw, bh, bh/2, HGrad(bx, bx+bw, u.accent.WithA(0.30), u.accent.WithA(0.58)))
	}
	if buttonShine {
		c.Fill(bx, y-bh/2, bx+bw, y, func(px, py float32) float32 { return sdRoundRect(px, py, bx+3, y-bh/2+1.5, bw-6, bh*0.42, bh*0.21) },
			VGrad(y-bh/2, y, White(0.18), White(0)), 0, nil)
	}
	c.StrokeRoundRect(bx, y-bh/2, bw, bh, bh/2, 1, VGrad(y-bh/2, y+bh/2, White(0.45), White(0.08)))
	u.text(c, label, bx+bw/2, y, 10.5, 600, White(0.96), 1, false)
	c.GA = ga
	c.PopClip()
}

// BaseParamsFor describes the glass this canvas needs.
func (u *UI) BaseParamsFor(c *Canvas) BaseParams {
	p := BaseParams{PixW: c.W, PixH: c.H, S: c.S, PW: u.PW, PH: u.PH, Clarity: u.Clarity,
		Blur: u.Toggles[TogBlur] && u.Backdrop != nil, Backdrop: u.Backdrop, BScale: u.BackdropScale, BVer: u.BackdropVer,
		R: u.Radius(), Outline: u.Toggles[TogOutline]}
	if p.Blur { // where the window sits in the (wider) captured backdrop
		p.BX, p.BY = u.WinX-u.BdX, u.WinY-u.BdY
	}
	return p
}

// fluidColors: theme (default), your own pick, or the album cover.
func (u *UI) fluidColors() [4]Col {
	switch u.FluidMode {
	case FluidCustom:
		return FluidPalette(u.FluidCol)
	case FluidArt:
		if u.Art != nil {
			u.ensureArtColors()
			return u.artCols
		}
	}
	return FluidPalette(u.accent)
}

// drawBase puts the glass slab down. If only the look changed (clarity, new
// backdrop) the rebuild happens in the background and the old slab is used
// meanwhile; if the size changed it's built right away.
func (u *UI) drawBase(c *Canvas) {
	want := u.BaseParamsFor(c)
	switch {
	case u.Base == nil || !u.Base.P.SameSize(want):
		if u.Base != nil && u.Resizing() {
			u.Base = BuildBaseFast(want) // mid-resize
		} else {
			u.Base = BuildBase(want)
		}
		u.baseWant = want.Key()
	case want.Blur && (u.Base.P.BX != want.BX || u.Base.P.BY != want.BY) && u.Base.P.BVer == want.BVer:
		// the window is being moved: the backdrop around it was captured
		// already, so just look at a different part of it, right away
		u.Base = BuildBaseFast(want)
		u.baseWant = ""
	case u.Base.P.Key() != want.Key():
		if u.SetPopView && u.Base.P.SameSize(want) && u.Base.P.Clarity == want.Clarity {
			u.Base = BuildBaseFast(want) // the Settings window: a new backdrop, quickly
		} else if u.BaseAsync == nil {
			u.Base = BuildBase(want)
		} else if u.baseWant != want.Key() {
			u.baseWant = want.Key()
			u.BaseAsync(want)
		}
	}
	var fl *Fluid
	strength := 0.62 * u.tog[TogFluid].V
	if strength > 0.005 {
		cols := u.fluidColors()
		UpdateFluid(&u.fluid, u.T, cols, u.PW/max32(u.PH, 1))
		fl = &u.fluid
	}
	u.Base.Composite(c, u.accent, fl, strength)
}

// SetBase receives a slab built in the background.
func (u *UI) SetBase(L *BaseLayer) bool {
	if L == nil || (u.Base != nil && !u.Base.P.SameSize(L.P)) {
		return false
	}
	u.Base = L
	return true
}

// ---------- shared components ----------

// card: a raised glass sub-panel. Built once per corner radius as a
// nine-slice sprite, so cards of any size (even while the window grows)
// cost almost nothing.
func (u *UI) card(c *Canvas, x, y, w, h, r float32) {
	const pad = 24
	shine := false // no glossy highlights
	key := fmt.Sprintf("card9|%.1f|%.3f|%v", r, c.S, shine)
	sp := u.sprites[key]
	side := 2*(r+pad) + 48
	if sp == nil {
		px := int(math.Ceil(float64(side * c.S)))
		sp = NewCanvas(px, px, c.S)
		drawCard(sp, pad, pad, side-2*pad, side-2*pad, r, shine)
		u.putSprite(key, sp)
	}
	ins := int(math.Ceil(float64((r + pad + 6) * c.S)))
	c.Blit9(sp, ins, x-pad, y-pad, w+2*pad, h+2*pad)
}

func (u *UI) Prewarm(s float32) {
	c := NewCanvas(1, 1, s)
	c.GA = 0.01
	u.card(c, 14, 58, 136, 228, 22)
	u.card(c, 158, 58, 248, 228, 22)
}

func drawCard(c *Canvas, x, y, w, h, r float32, shine bool) {
	shape := func(px, py float32) float32 { return sdRoundRect(px, py, x, y, w, h, r) }
	c.Fill(x, y, x+w, y+h, func(px, py float32) float32 { return sdRoundRect(px, py-5, x+3, y+3, w-6, h-6, r) },
		Solid(Black(0.20)), 12, func(px, py float32) float32 { return -shape(px, py) })
	c.Fill(x, y, x+w, y+h, shape, VGrad(y, y+h, White(0.14), White(0.07), White(0.05)), 0, nil)
	if shine {
		c.Fill(x, y, x+w, y+h, shape, Radial(x+w*0.15, y-10, max32(w, h)*0.9, White(0.09)), 0, nil)
	}
	c.StrokeRoundRect(x, y, w, h, r, 1.1, VGrad(y, y+h, White(0.50), White(0.12), White(0.08), White(0.22)))
}

func (u *UI) glassButton(c *Canvas, id int, cx, cy, r float32) {
	hv, pr := u.h(id), u.p(id)
	r *= 1 - 0.07*pr + 0.04*hv
	c.Fill(cx-r, cy-r, cx+r, cy+r, func(x, y float32) float32 { return sdCircle(x, y-4, cx, cy, r-1) },
		Solid(Black(0.30)), 9, func(x, y float32) float32 { return -sdCircle(x, y, cx, cy, r) })
	f := 0.12 + 0.08*hv
	c.Circle(cx, cy, r, Solid(White(f)))
	if buttonShine {
		c.Fill(cx-r, cy-r, cx+r, cy, func(x, y float32) float32 {
			return sdEllipse(x, y, cx, cy-r*0.46, r*0.72, r*0.40)
		}, VGrad(cy-r, cy, White(0.26), White(0)), 0, nil)
	}
	c.StrokeCircle(cx, cy, r, 1, Solid(White(0.22+0.1*hv)))
	u.addHitC(c, id, cx, cy, r+3)
}

func (u *UI) smallButton(c *Canvas, id int, cx, cy float32, on bool) {
	hv, pr := u.h(id), u.p(id)
	r := 15 * (1 - 0.08*pr)
	if on {
		c.Circle(cx, cy, r, VGrad(cy-r, cy+r, u.accent.Mix(White(1), 0.25), u.accent))
		c.StrokeCircle(cx, cy, r, 1, Solid(White(0.45)))
	} else {
		c.Circle(cx, cy, r, Solid(White(0.08+0.10*hv)))
		c.StrokeCircle(cx, cy, r, 1, Solid(White(0.18+0.12*hv)))
	}
	u.addHitC(c, id, cx, cy, r+3)
}

// btnScale: glyph buttons grow a little on hover and dip when pressed.
func (u *UI) btnScale(c *Canvas, id int, cx, cy, r float32) float32 {
	u.addHitC(c, id, cx, cy, r)
	return 1 + 0.12*u.h(id) - 0.10*u.p(id)
}

func (u *UI) bubble(c *Canvas, id int, cx, cy, r float32) {
	hv, pr := u.h(id), u.p(id)
	a := max32(hv*0.85, pr)
	if a > 0.01 {
		rr := r * (0.82 + 0.18*a)
		c.Circle(cx, cy, rr, Solid(White(0.10*a+0.06*pr)))
		c.StrokeCircle(cx, cy, rr, 1, VGrad(cy-rr, cy+rr, White(0.32*a), White(0.04*a)))
	}
	u.addHitC(c, id, cx, cy, r)
}

func (u *UI) accentPill(c *Canvas, id int, label string, x, cy float32, align int) float32 {
	w := u.textW(label, 11.5, 600) + 22
	if align == 2 {
		x -= w
	} else if align == 1 {
		x -= w / 2
	}
	hv, pr := u.h(id), u.p(id)
	ac := u.accent.Mix(White(1), 0.12*hv)
	h := float32(25)
	y := cy - h/2
	sc := 1 - 0.05*pr
	cx := x + w/2
	ww := w * sc
	xx := cx - ww/2
	c.Fill(xx, y, xx+ww, y+h, func(px, py float32) float32 { return sdRoundRect(px, py-3, xx+2, y+2, ww-4, h-4, h/2) },
		Solid(ac.WithA(0.45)), 7, func(px, py float32) float32 { return -sdRoundRect(px, py, xx, y, ww, h, h/2) })
	c.RoundRect(xx, y, ww, h, h/2, VGrad(y, y+h, ac.Mix(White(1), 0.08), ac))
	if buttonShine {
		c.Fill(xx, y, xx+ww, y+h/2, func(px, py float32) float32 {
			return sdRoundRect(px, py, xx+3, y+1.5, ww-6, h*0.42, h*0.21)
		}, VGrad(y, y+h/2, White(0.38), White(0)), 0, nil)
	}
	c.StrokeRoundRect(xx, y, ww, h, h/2, 1, Solid(White(0.30)))
	u.text(c, label, cx, cy, 11.5, 600, u.onAccent(), 1, false)
	u.addHit(c, id, x-2, y-3, x+w+2, y+h+3)
	return w
}

func (u *UI) ghostPill(c *Canvas, id int, label string, x, cy float32, align int) float32 {
	w := u.textW(label, 11.5, 600) + 22
	if align == 2 {
		x -= w
	} else if align == 1 {
		x -= w / 2
	}
	hv := u.h(id)
	c.RoundRect(x, cy-12.5, w, 25, 12.5, Solid(White(0.10+0.08*hv)))
	c.StrokeRoundRect(x, cy-12.5, w, 25, 12.5, 1, VGrad(cy-12, cy+12, White(0.35), White(0.06)))
	u.text(c, label, x+w/2, cy, 11.5, 600, White(0.95), 1, false)
	u.addHit(c, id, x-2, cy-15, x+w+2, cy+15)
	return w
}

// keyCaps draws keys as caps. align 0 = start at x, 2 = end at x.
func (u *UI) keyCaps(c *Canvas, keys []string, x, cy, maxW, size float32, align int) float32 {
	if len(keys) == 0 {
		return u.text(c, "Not set", x, cy, size+0.5, 500, dim, align, false)
	}
	pad, hh := size+3, size*2+2
	widths := make([]float32, len(keys))
	total := float32(-4)
	for i, k := range keys {
		widths[i] = max32(u.textW(k, size, 600)+pad, hh)
		total += widths[i] + 4
	}
	if total > maxW {
		keys = []string{u.fit(strings.Join(keys, "+"), size, 600, maxW-pad)}
		widths = []float32{u.textW(keys[0], size, 600) + pad}
		total = widths[0]
	}
	if align == 2 {
		x -= total
	}
	for i, k := range keys {
		w := widths[i]
		c.RoundRect(x, cy-hh/2, w, hh, hh*0.32, VGrad(cy-hh/2, cy+hh/2, Black(0.16), Black(0.28)))
		c.StrokeRoundRect(x, cy-hh/2, w, hh, hh*0.32, 1, VGrad(cy-hh/2, cy+hh/2, White(0.30), White(0.07)))
		u.text(c, k, x+w/2, cy, size, 600, txt, 1, false)
		x += w + 4
	}
	return total
}

func (u *UI) separator(c *Canvas, x0, x1, y float32) {
	c.Line(x0, y, x1, y, 1, HGrad(x0, x1, White(0.02), White(0.14), White(0.14), White(0.02)))
}

func (u *UI) toggle(c *Canvas, id int, t float32, x, cy float32) {
	w, h := float32(44), float32(26)
	y := cy - h/2
	hv, pr := u.h(id), u.p(id)
	off := White(0.16 + 0.06*hv)
	c.RoundRect(x, y, w, h, h/2, Solid(lerpCol(off, u.accent, t)))
	c.StrokeRoundRect(x, y, w, h, h/2, 1, VGrad(y, y+h, White(0.10+0.2*t), White(0.25)))
	kr := float32(11) + 0.6*hv
	kw := kr * 2 * (1 + 0.18*pr)
	kx := x + 2 + kw/2 + (w-4-kw)*t
	c.Fill(kx-kw/2, cy-kr, kx+kw/2, cy+kr, func(px, py float32) float32 {
		return sdRoundRect(px, py-2, kx-kw/2, cy-kr, kw, kr*2, kr)
	}, Solid(Black(0.32)), 4, nil)
	c.RoundRect(kx-kw/2, cy-kr, kw, kr*2, kr, VGrad(cy-kr, cy+kr, White(1), Hex(0xe8ebf0, 1)))
	u.addHit(c, id, x-4, y-6, x+w+4, y+h+6)
}

// slider draws a bar for value v (0..1) and records its geometry for dragging.
func (u *UI) slider(c *Canvas, sl int, v, sx0, sx1, cy float32) {
	id := hSlider0 + sl
	if c.GA >= 0.5 {
		u.sliders[id] = [2]float32{sx0 + c.OX, sx1 + c.OX}
	}
	hv, pr := u.h(id), u.p(id)
	th := 6 + 2*max32(hv, pr)
	c.RoundRect(sx0, cy-th/2, sx1-sx0, th, th/2, Solid(White(0.16)))
	kx := sx0 + (sx1-sx0)*v
	if kx-sx0 > 0.5 {
		c.RoundRect(sx0, cy-th/2, kx-sx0, th, th/2, HGrad(sx0, sx1, u.accent.Mix(Black(1), 0.1), u.accent.Mix(White(1), 0.2)))
	}
	kr := 9.5 + 1.5*hv + 1.5*pr
	c.Fill(kx-kr, cy-kr, kx+kr, cy+kr, func(px, py float32) float32 { return sdCircle(px, py-2, kx, cy, kr) },
		Solid(Black(0.35)), 5, nil)
	c.Circle(kx, cy, kr, VGrad(cy-kr, cy+kr, White(1), Hex(0xe4e8ef, 1)))
	c.StrokeCircle(kx, cy, kr, 1, Solid(Black(0.08)))
	u.addHit(c, id, sx0-12, cy-15, sx1+12, cy+15)
}

// image draws img (premultiplied) inside a rounded rect with smooth edges.
// imageSprite: img scaled to pw x ph pixels (cached).
func (u *UI) imageSprite(img *Canvas, key string, pw, ph int) *Canvas {
	if pw <= 0 || ph <= 0 || img == nil {
		return nil
	}
	k := fmt.Sprintf("img|%s|%d|%d", key, pw, ph)
	sp := u.sprites[k]
	if sp == nil {
		if img.W == pw && img.H == ph {
			sp = img
		} else {
			sp = img.Resized(pw, ph)
		}
		u.putSprite(k, sp)
	}
	return sp
}

func (u *UI) image(c *Canvas, img *Canvas, key string, x, y, w, h, r float32) {
	pw, ph := int(math.Round(float64(w*c.S))), int(math.Round(float64(h*c.S)))
	sp := u.imageSprite(img, key, pw, ph)
	if sp == nil {
		return
	}
	S := c.S
	c.Fill(x, y, x+w, y+h, func(px, py float32) float32 { return sdRoundRect(px, py, x, y, w, h, r) },
		func(px, py float32) Col {
			ix, iy := int((px-x)*S), int((py-y)*S)
			ix = max(0, min(ix, sp.W-1))
			iy = max(0, min(iy, sp.H-1))
			i := (iy*sp.W + ix) * 4
			a := sp.Pix[i+3]
			if a <= 0 {
				return Col{}
			}
			return Col{sp.Pix[i] / a, sp.Pix[i+1] / a, sp.Pix[i+2] / a, a}
		}, 0, nil)
}

// appIcon draws an app's real icon, or a lettered circle if there's none.
func (u *UI) appIcon(c *Canvas, key, name string, cx, cy, size float32, ring bool) {
	if ring {
		r := size/2 + 2.6
		c.Fill(cx-r-2, cy-r-2, cx+r+2, cy+r+2, func(px, py float32) float32 {
			return abs32(sdCircle(px, py, cx, cy, r)) - 1.1
		}, func(px, py float32) Col {
			a := float32(math.Atan2(float64(py-cy), float64(px-cx))/(2*math.Pi)) + 0.5
			return u.accent.Mix(HSV(a*0.25+0.9, 0.7, 1), 0.45)
		}, 0, nil)
	}
	if key != "" && u.Icons != nil {
		px := int(math.Round(float64(size * c.S)))
		if ic := u.Icons(key, px); ic != nil {
			u.image(c, ic, "icon|"+key, cx-size/2, cy-size/2, size, size, size*0.22)
			return
		}
	}
	r := size / 2
	hue := float32(0)
	for _, ch := range strings.ToLower(name) {
		hue = hue*1.37 + float32(ch)*0.0137
	}
	col := HSV(hue, 0.55, 0.92)
	c.Circle(cx, cy, r, VGrad(cy-r, cy+r, col.Mix(White(1), 0.25), col.Mix(Black(1), 0.15)))
	letter := "?"
	if rs := []rune(strings.TrimSpace(name)); len(rs) > 0 {
		letter = strings.ToUpper(string(rs[0]))
	}
	u.text(c, letter, cx, cy, r*1.05, 600, White(0.97), 1, false)
}

// ---------- icons ----------

func iconRewind(c *Canvas, cx, cy, s float32, p Paint) {
	c.Poly([]float32{cx, cy - 9.5*s, cx, cy + 9.5*s, cx - 13*s, cy}, 1.8*s, p)
	c.Poly([]float32{cx + 13*s, cy - 9.5*s, cx + 13*s, cy + 9.5*s, cx, cy}, 1.8*s, p)
}

func iconForward(c *Canvas, cx, cy, s float32, p Paint) {
	c.Poly([]float32{cx, cy - 9.5*s, cx, cy + 9.5*s, cx + 13*s, cy}, 1.8*s, p)
	c.Poly([]float32{cx - 13*s, cy - 9.5*s, cx - 13*s, cy + 9.5*s, cx, cy}, 1.8*s, p)
}

func iconPlay(c *Canvas, cx, cy, s float32, p Paint) {
	c.Poly([]float32{cx - 9*s, cy - 15*s, cx - 9*s, cy + 15*s, cx + 16*s, cy}, 2.6*s, p)
}

func iconPause(c *Canvas, cx, cy, s float32, p Paint) {
	c.RoundRect(cx-11*s, cy-15*s, 7.6*s, 30*s, 2.6*s, p)
	c.RoundRect(cx+3.4*s, cy-15*s, 7.6*s, 30*s, 2.6*s, p)
}

func iconSpeaker(c *Canvas, cx, cy float32, muted bool, p Paint) {
	x := cx - 4
	c.Poly([]float32{x - 9, cy - 3.8, x - 4.5, cy - 3.8, x + 1.5, cy - 9, x + 1.5, cy + 9, x - 4.5, cy + 3.8, x - 9, cy + 3.8}, 1.2, p)
	if muted {
		c.Line(x+6, cy-4.5, x+14, cy+4.5, 2, p)
		c.Line(x+14, cy-4.5, x+6, cy+4.5, 2, p)
		return
	}
	for i, r := range []float32{6, 11} {
		rr := r
		ang := float32(0.85)
		if i == 1 {
			ang = 0.9
		}
		c.Fill(x, cy-rr-2, x+rr+2, cy+rr+2, func(px, py float32) float32 {
			return sdArcRight(px, py, x+1.5, cy, rr, 2, ang)
		}, p, 0, nil)
	}
}

func iconSpeakerScaled(c *Canvas, cx, cy, sc float32, muted bool, p Paint) {
	x := cx - 4*sc
	c.Poly([]float32{x - 9*sc, cy - 3.8*sc, x - 4.5*sc, cy - 3.8*sc, x + 1.5*sc, cy - 9*sc, x + 1.5*sc, cy + 9*sc, x - 4.5*sc, cy + 3.8*sc, x - 9*sc, cy + 3.8*sc}, 1.2*sc, p)
	if muted {
		c.Line(x+6*sc, cy-4.5*sc, x+14*sc, cy+4.5*sc, 2*sc, p)
		c.Line(x+14*sc, cy-4.5*sc, x+6*sc, cy+4.5*sc, 2*sc, p)
		return
	}
	for _, r := range []float32{6, 11} {
		rr := r * sc
		c.Fill(x, cy-rr-2, x+rr+2, cy+rr+2, func(px, py float32) float32 {
			return sdArcRight(px, py, x+1.5*sc, cy, rr, 2*sc, 0.88)
		}, p, 0, nil)
	}
}

func iconGear(c *Canvas, cx, cy float32, p Paint) {
	for i := 0; i < 8; i++ {
		a := float64(i) * math.Pi / 4
		dx, dy := float32(math.Cos(a)), float32(math.Sin(a))
		c.Line(cx+dx*6, cy+dy*6, cx+dx*8.6, cy+dy*8.6, 3.6, p)
	}
	c.Fill(cx-9, cy-9, cx+9, cy+9, func(px, py float32) float32 {
		return max32(sdCircle(px, py, cx, cy, 7), -sdCircle(px, py, cx, cy, 2.8))
	}, p, 0, nil)
}

func iconPin(c *Canvas, cx, cy float32, p Paint) {
	c.RoundRect(cx-3.4, cy-7.5, 6.8, 6.8, 1.8, p)
	c.Line(cx-5.6, cy-0.4, cx+5.6, cy-0.4, 2.0, p)
	c.Line(cx, cy, cx, cy+7.2, 1.5, p)
}

func iconClose(c *Canvas, cx, cy float32, p Paint) {
	c.Line(cx-4.4, cy-4.4, cx+4.4, cy+4.4, 1.8, p)
	c.Line(cx+4.4, cy-4.4, cx-4.4, cy+4.4, 1.8, p)
}

func iconBack(c *Canvas, cx, cy float32, p Paint) {
	c.Line(cx+2.5, cy-6, cx-3, cy, 2.1, p)
	c.Line(cx-3, cy, cx+2.5, cy+6, 2.1, p)
}

// iconChevron: rot 0 = pointing down, 1 = pointing right.
func iconChevron(c *Canvas, cx, cy, rot float32, p Paint) {
	pts := [][2]float32{{-4.5, -2.2}, {0, 2.3}, {4.5, -2.2}}
	a := float64(-rot * math.Pi / 2)
	ca, sa := float32(math.Cos(a)), float32(math.Sin(a))
	tp := func(v [2]float32) (float32, float32) { return cx + v[0]*ca - v[1]*sa, cy + v[0]*sa + v[1]*ca }
	x0, y0 := tp(pts[0])
	x1, y1 := tp(pts[1])
	x2, y2 := tp(pts[2])
	c.Line(x0, y0, x1, y1, 1.9, p)
	c.Line(x1, y1, x2, y2, 1.9, p)
}

func iconKeyboard(c *Canvas, cx, cy float32, p Paint) {
	c.StrokeRoundRect(cx-9, cy-6.5, 18, 13, 3, 1.5, p)
	for i := 0; i < 4; i++ {
		c.Circle(cx-5+float32(i)*3.4, cy-2, 0.95, p)
	}
	c.Line(cx-4, cy+2.6, cx+4, cy+2.6, 1.5, p)
}

func iconSwitch(c *Canvas, cx, cy float32, p Paint) {
	c.StrokeRoundRect(cx-9, cy-5.5, 18, 11, 5.5, 1.5, p)
	c.Circle(cx+3.5, cy, 3, p)
}

func iconPalette(c *Canvas, cx, cy float32, p Paint) {
	c.StrokeCircle(cx, cy, 8.5, 1.5, p)
	c.Circle(cx-3.5, cy-2.5, 1.5, p)
	c.Circle(cx+1, cy-4.2, 1.5, p)
	c.Circle(cx+4.4, cy-0.5, 1.5, p)
	c.Circle(cx-1.5, cy+3.8, 2.1, p)
}

func iconMixer(c *Canvas, cx, cy float32, p Paint) {
	for i, k := range []float32{-2.5, 3, -0.5} {
		x := cx - 6 + float32(i)*6
		c.Line(x, cy-7.5, x, cy+7.5, 1.4, p)
		c.Circle(x, cy+k, 2.6, p)
	}
}

func iconFolder(c *Canvas, cx, cy float32, p Paint) {
	c.Fill(cx-9, cy-8, cx+9, cy+8, func(px, py float32) float32 {
		body := sdRoundRect(px, py, cx-8, cy-4, 16, 11, 2.2)
		tab := sdRoundRect(px, py, cx-8, cy-6.5, 7, 5, 1.6)
		return abs32(min32(body, tab)+0.75) - 0.75
	}, p, 0, nil)
}

func iconPencil(c *Canvas, cx, cy float32, p Paint) {
	c.Line(cx-3.5, cy+3.5, cx+4, cy-4, 2.6, p)
	c.Poly([]float32{cx - 6, cy + 6, cx - 5.4, cy + 2.4, cx - 2.4, cy + 5.4}, 0.3, p)
}

func iconTrash(c *Canvas, cx, cy float32, p Paint) {
	c.Line(cx-5.5, cy-4.5, cx+5.5, cy-4.5, 1.6, p)
	c.Line(cx-1.8, cy-6.3, cx+1.8, cy-6.3, 1.6, p)
	c.Fill(cx-6, cy-4, cx+6, cy+7, func(px, py float32) float32 {
		return abs32(sdRoundRect(px, py, cx-4.3, cy-3.5, 8.6, 10, 1.8)+0.75) - 0.75
	}, p, 0, nil)
}

func iconMove(c *Canvas, cx, cy float32, p Paint) {
	iconFolder(c, cx-2, cy, p)
	c.Line(cx+1, cy+1.5, cx+8, cy+1.5, 1.6, p)
	c.Line(cx+5.3, cy-1.2, cx+8, cy+1.5, 1.6, p)
	c.Line(cx+5.3, cy+4.2, cx+8, cy+1.5, 1.6, p)
}

func iconPlus(c *Canvas, cx, cy, s float32, p Paint) {
	c.Line(cx-s, cy, cx+s, cy, 1.8, p)
	c.Line(cx, cy-s, cx, cy+s, 1.8, p)
}

// App icon: accent squircle with beamed notes.
func DrawAppIcon(c *Canvas, x, y, s float32, ac Col) {
	drawIcon(c, x, y, s, ac, VGrad(y, y+s, ac.Mix(White(1), 0.30), ac, ac.Mix(Black(1), 0.22)))
}

// DrawBrandIcon: Akra Player's own icon, a dark red with a gradient (the exe,
// the tray and the browser extension).
func DrawBrandIcon(c *Canvas, x, y, s float32) {
	// lit from the top-left: warm crimson fading into a deep wine red
	top, bot := Hex(0xd23a4e, 1), Hex(0x3e0611, 1)
	drawIcon(c, x, y, s, BrandRed, func(px, py float32) Col {
		t := clamp01(((px-x)*0.45 + (py - y)) / (s * 1.45))
		if t < 0.5 {
			return lerpCol(top, BrandRed, t*2)
		}
		return lerpCol(BrandRed, bot, (t-0.5)*2)
	})
}

func drawIcon(c *Canvas, x, y, s float32, ac Col, bg Paint) {
	r := s * 0.30
	c.RoundRect(x, y, s, s, r, bg)
	c.Fill(x, y, x+s, y+s*0.55, func(px, py float32) float32 {
		return sdRoundRect(px, py, x+s*0.08, y+s*0.05, s*0.84, s*0.42, r*0.8)
	}, VGrad(y, y+s*0.5, White(0.45), White(0)), 0, nil)
	nc := Solid(White(1))
	if ac.Lum() > 0.62 {
		nc = Solid(Col{0.1, 0.1, 0.1, 0.9})
	}
	lw := s * 0.085
	c.Line(x+s*0.40, y+s*0.30, x+s*0.40, y+s*0.68, lw, nc)
	c.Line(x+s*0.70, y+s*0.24, x+s*0.70, y+s*0.62, lw, nc)
	c.Line(x+s*0.40, y+s*0.30, x+s*0.70, y+s*0.24, lw*1.5, nc)
	c.Fill(x, y, x+s, y+s, func(px, py float32) float32 { return sdEllipse(px, py, x+s*0.325, y+s*0.70, s*0.115, s*0.09) }, nc, 0, nil)
	c.Fill(x, y, x+s, y+s, func(px, py float32) float32 { return sdEllipse(px, py, x+s*0.625, y+s*0.64, s*0.115, s*0.09) }, nc, 0, nil)
	c.StrokeRoundRect(x, y, s, s, r, max32(0.8, s*0.03), VGrad(y, y+s, White(0.6), White(0.1), White(0.25)))
}

// ---------- 2.4 components ----------

// headerButton: small round button for the top bar. Enlarge / shrink show
// their 3-second hold as a fill (left→right to grow, right→left to reset).
func (u *UI) headerButton(c *Canvas, id int, cx, cy float32, on bool, icon func(*Canvas, float32, float32, Paint)) {
	hv, pr := u.h(id), u.p(id)
	r := 13 * (1 - 0.08*pr + 0.04*hv)
	if on {
		c.Circle(cx, cy, r, VGrad(cy-r, cy+r, u.accent.Mix(White(1), 0.25), u.accent))
		c.StrokeCircle(cx, cy, r, 1, Solid(White(0.45)))
	} else {
		c.Circle(cx, cy, r, Solid(White(0.08+0.10*hv)))
		c.StrokeCircle(cx, cy, r, 1, Solid(White(0.18+0.12*hv)))
	}
	if u.HoldID == id && u.HoldT > 0.04 {
		t := clamp01((u.HoldT - 0.04) / 0.96)
		edge := cx - r + 2*r*t
		grow := id != hShrink
		if !grow {
			edge = cx + r - 2*r*t
		}
		c.Fill(cx-r-1, cy-r-1, cx+r+1, cy+r+1, func(px, py float32) float32 {
			d := sdCircle(px, py, cx, cy, r-0.6)
			if grow {
				return max32(d, px-edge)
			}
			return max32(d, edge-px)
		}, HGrad(cx-r, cx+r, u.accent.Mix(White(1), 0.2), u.accent), 0, nil)
	}
	col := White(0.92)
	if on && u.accent.Lum() > 0.62 {
		col = Black(0.8)
	}
	icon(c, cx, cy, Solid(col))
	u.addHitC(c, id, cx, cy, r+2)
	if id == u.TipID {
		u.tipX, u.tipY = cx+c.OX, cy+c.OY+r
	}
}

func (u *UI) drawTooltip(c *Canvas) {
	a := u.tipA.V
	if a < 0.01 {
		return
	}
	label := "Hold to fullscreen"
	if !u.Toggles[TogFullscreen] {
		label = "Hold to fill the screen"
	}
	if u.TipID == hShrink || (u.TipID == hNone && u.lastTip == hShrink) {
		label = "Hold to reset size"
	}
	if u.TipID != hNone {
		u.lastTip = u.TipID
	}
	w := u.textW(label, 11, 600) + 22
	h := float32(26)
	x := min32(max32(8, u.tipX-w/2), u.PW-8-w)
	y := u.tipY + 8 + (1-a)*6
	c.GA = a
	c.Fill(x, y, x+w, y+h, func(px, py float32) float32 { return sdRoundRect(px, py-4, x+2, y+2, w-4, h-4, h/2) },
		Solid(Black(0.45)), 8, nil)
	c.RoundRect(x, y, w, h, h/2, VGrad(y, y+h, Col{0.26, 0.27, 0.30, 0.96}, Col{0.16, 0.17, 0.19, 0.96}))
	c.StrokeRoundRect(x, y, w, h, h/2, 1, VGrad(y, y+h, White(0.45), White(0.08)))
	u.text(c, label, x+w/2, y+h/2, 11, 600, White(0.96), 1, false)
	c.GA = 1
}

// bottomVisualizer: live bars along the bottom edge, behind the buttons.
// The newest level is in the middle and ripples outward.
func (u *UI) bottomVisualizer(c *Canvas) {
	if c.GA < 0.01 {
		return
	}
	PW, PH := u.PW, u.PH
	c.PushClip(0, 0, PW, PH, u.Radius())
	bw := u.visP(VisBars, 1, 2, 3.6, 6.5) // bar width
	step := bw * 7 / 3.6
	n := int(PW/step) + 1
	mid := PW / 2
	ac := u.accent
	maxH := u.visP(VisBars, 0, 22, 58, 120)
	for i := 0; i < n; i++ {
		x := float32(i)*step + step/2
		k := int(abs32(x-mid) / 7 / 1.6)
		lv := u.visRaw(k)
		v := float32(math.Sqrt(float64(lv)))
		h := 3 + maxH*v*(1-0.35*abs32(x-mid)/mid)
		top := PH - h
		col := ac.Mix(White(1), 0.35-0.3*v)
		c.RoundRect(x-bw/2, top, bw, h+6, bw/2, VGrad(top, PH, col.WithA(0.10+0.32*v), col.WithA(0.04+0.10*v)))
	}
	c.PopClip()
}

// ensureArtColors pulls a fluid palette out of the cover (once per cover).
func (u *UI) ensureArtColors() {
	if u.artColVer == u.ArtVer || u.Art == nil {
		return
	}
	u.artColVer = u.ArtVer
	sm := u.Art.Resized(4, 4)
	var avg [4]Col
	for q := 0; q < 4; q++ { // quadrants
		var r, g, b, n float32
		for y := (q / 2) * 2; y < (q/2)*2+2; y++ {
			for x := (q % 2) * 2; x < (q%2)*2+2; x++ {
				i := (y*4 + x) * 4
				a := max32(sm.Pix[i+3], 0.001)
				r, g, b, n = r+sm.Pix[i]/a, g+sm.Pix[i+1]/a, b+sm.Pix[i+2]/a, n+1
			}
		}
		avg[q] = Col{r / n, g / n, b / n, 1}
	}
	// sort by brightness: the darkest becomes the base the others flow over
	for i := 1; i < 4; i++ {
		for j := i; j > 0 && avg[j].Lum() > avg[j-1].Lum(); j-- {
			avg[j], avg[j-1] = avg[j-1], avg[j]
		}
	}
	for i := 0; i < 3; i++ {
		h, s, v := toHSV(avg[i])
		avg[i] = HSV(h, min32(1, s*1.25+0.05), min32(1, max32(v, 0.45)*1.1))
	}
	h, s, v := toHSV(avg[3])
	avg[3] = HSV(h, min32(1, s*1.1), min32(0.35, v))
	u.artCols = avg
}

// artGlow: a soft glow around the cover. Each side of the cover gives three
// colors (one per third), blended smoothly all the way around, so the light
// looks like it spills off the image. bloom (0..1) grows it outward.
func (u *UI) artGlow(c *Canvas, x, y, s, r, bloom float32) {
	if u.Art == nil || c.GA < 0.01 {
		return
	}
	g := 26 + s*0.05 // glow reach (logical)
	S := c.S
	key := fmt.Sprintf("glow|%d|%.0f|%.3f", u.ArtVer, s, S)
	sp := u.sprites[key]
	if sp == nil {
		stops := coverEdgeStops(u.Art)
		colAt := func(lx, ly float32) Col {
			// position around the edge, 0..4 (top, right, bottom, left) of
			// the nearest point on the cover's border
			qx, qy := max32(0, min32(lx, s)), max32(0, min32(ly, s))
			side := 0
			if lx > 0 && lx < s && ly > 0 && ly < s { // inside: nearest edge
				best := ly
				if s-lx < best {
					best, side = s-lx, 1
				}
				if s-ly < best {
					best, side = s-ly, 2
				}
				if lx < best {
					side = 3
				}
			} else {
				switch {
				case qy == 0:
					side = 0
				case qx == s:
					side = 1
				case qy == s:
					side = 2
				default:
					side = 3
				}
			}
			p := [4]float32{qx / s, 1 + qy/s, 2 + (s-qx)/s, 3 + (s-qy)/s}[side]
			q := p*3 - 0.5
			i0 := int(math.Floor(float64(q)))
			f := q - float32(i0)
			f = f * f * (3 - 2*f)
			a, b := stops[((i0%12)+12)%12], stops[(((i0+1)%12)+12)%12]
			return lerpCol(a, b, f)
		}
		px := int(math.Ceil(float64((s + 2*g) * S)))
		sp = NewCanvas(px, px, S)
		sigma := g * 0.42
		sp.Fill(0, 0, s+2*g, s+2*g, func(px, py float32) float32 { return -1 }, func(px, py float32) Col {
			lx, ly := px-g, py-g
			d := sdRoundRect(lx, ly, 0, 0, s, s, r)
			if d < -r {
				return Col{}
			}
			dd := max32(d, 0) / sigma
			a := 0.34 * float32(math.Exp(float64(-dd*dd))) // soft: it shouldn't steal the show
			if a <= 0.003 {
				return Col{}
			}
			cc := colAt(lx, ly)
			cc.A = a
			return cc
		}, 0, nil)
		u.putSprite(key, sp)
	}
	if bloom >= 0.999 {
		c.Blit(sp, x-g, y-g)
		return
	}
	// blooming: a soft front moves outward from the cover's edge
	front := bloom * g * 1.15
	soft := g * 0.35
	ox, oy := x-g, y-g
	c.Fill(ox, oy, ox+s+2*g, oy+s+2*g, func(px, py float32) float32 { return -1 }, func(px, py float32) Col {
		d := sdRoundRect(px, py, x, y, s, s, r)
		k := clamp01((front-d)/soft) * bloom
		if k <= 0.002 {
			return Col{}
		}
		ix, iy := int((px-ox)*S), int((py-oy)*S)
		if ix < 0 || iy < 0 || ix >= sp.W || iy >= sp.H {
			return Col{}
		}
		i := (iy*sp.W + ix) * 4
		al := sp.Pix[i+3]
		if al <= 0 {
			return Col{}
		}
		return Col{sp.Pix[i] / al, sp.Pix[i+1] / al, sp.Pix[i+2] / al, al * k}
	}, 0, nil)
}

// coverEdgeStops: 12 colors around the cover, three per side (each the
// average of a third of that side's edge), clockwise from the top-left.
func coverEdgeStops(art *Canvas) [12]Col {
	const n = 24
	sm := art.Resized(n, n)
	avg := func(x0, y0, x1, y1 int) Col {
		var r, g, b, w float32
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				i := (y*n + x) * 4
				a := sm.Pix[i+3]
				if a > 0 {
					r, g, b, w = r+sm.Pix[i]/a, g+sm.Pix[i+1]/a, b+sm.Pix[i+2]/a, w+1
				}
			}
		}
		if w == 0 {
			return Col{0.5, 0.5, 0.5, 1}
		}
		c := Col{r / w, g / w, b / w, 1}
		h, s, v := toHSV(c)
		return HSV(h, min32(1, s*1.15+0.02), min32(1, max32(v, 0.3)*1.08))
	}
	const d = 4 // strip depth
	var out [12]Col
	for k := 0; k < 3; k++ {
		a, b := k*n/3, (k+1)*n/3
		out[k] = avg(a, 0, b, d)         // top, left to right
		out[3+k] = avg(n-d, a, n, b)     // right, top to bottom
		out[6+k] = avg(n-b, n-d, n-a, n) // bottom, right to left
		out[9+k] = avg(0, n-b, d, n-a)   // left, bottom to top
	}
	return out
}

// ---------- 2.4 icons ----------

func iconEnlarge(c *Canvas, cx, cy float32, p Paint) {
	c.Line(cx+1.2, cy-1.2, cx+5.2, cy-5.2, 1.6, p)
	c.Line(cx+1.6, cy-5.4, cx+5.4, cy-5.4, 1.6, p)
	c.Line(cx+5.4, cy-5.4, cx+5.4, cy-1.6, 1.6, p)
	c.Line(cx-1.2, cy+1.2, cx-5.2, cy+5.2, 1.6, p)
	c.Line(cx-1.6, cy+5.4, cx-5.4, cy+5.4, 1.6, p)
	c.Line(cx-5.4, cy+5.4, cx-5.4, cy+1.6, 1.6, p)
}

func iconShrink(c *Canvas, cx, cy float32, p Paint) {
	c.Line(cx+1.6, cy-1.6, cx+5.6, cy-5.6, 1.6, p)
	c.Line(cx+1.4, cy-1.4, cx+1.4, cy-4.8, 1.6, p)
	c.Line(cx+1.4, cy-1.4, cx+4.8, cy-1.4, 1.6, p)
	c.Line(cx-1.6, cy+1.6, cx-5.6, cy+5.6, 1.6, p)
	c.Line(cx-1.4, cy+1.4, cx-1.4, cy+4.8, 1.6, p)
	c.Line(cx-1.4, cy+1.4, cx-4.8, cy+1.4, 1.6, p)
}

// iconMini: a window with a small pill in its corner.
func iconMini(c *Canvas, cx, cy float32, p Paint) {
	c.StrokeRoundRect(cx-6.5, cy-5, 13, 10, 2.4, 1.4, p)
	c.RoundRect(cx-0.5, cy+0.2, 5.2, 3, 1.5, p)
}

// iconRestore: back from mini mode.
func iconRestore(c *Canvas, cx, cy float32, p Paint) {
	c.StrokeRoundRect(cx-6, cy-5, 12, 10, 2.4, 1.5, p)
	c.Line(cx-6, cy-1.6, cx+6, cy-1.6, 1.3, p)
}

// iconPicture: the album-art on/off button.
func iconPicture(c *Canvas, cx, cy float32, p Paint) {
	c.StrokeRoundRect(cx-7, cy-6, 14, 12, 2.6, 1.4, p)
	c.Poly([]float32{cx - 5, cy + 3.6, cx - 1.2, cy - 1.2, cx + 1.6, cy + 1.8, cx + 3, cy + 0.4, cx + 5, cy + 3.6}, 0.6, p)
	c.Circle(cx+2.6, cy-2.4, 1.3, p)
}

// sdHeart: heart shape centered on (cx, cy), about 2*s wide.
func sdHeart(px, py, cx, cy, s float32) float32 {
	x := abs32(px-cx) / (s * 1.95)
	y := (cy + s*0.95 - py) / (s * 1.95) // tip at the bottom
	dot2 := func(a, b float32) float32 { return a*a + b*b }
	var d float32
	if y+x > 1 {
		d = float32(math.Sqrt(float64(dot2(x-0.25, y-0.75)))) - 0.35355339
	} else {
		m := max32(x+y, 0) * 0.5
		d1 := dot2(x, y-1)
		d2 := dot2(x-m, y-m)
		d = float32(math.Sqrt(float64(min32(d1, d2))))
		if x < y {
			d = -d
		}
	}
	return d * s * 1.95
}

func iconHeart(c *Canvas, cx, cy, s float32, filled bool, p Paint) {
	if filled {
		c.Fill(cx-s*1.3, cy-s*1.3, cx+s*1.3, cy+s*1.3, func(px, py float32) float32 { return sdHeart(px, py, cx, cy, s) }, p, 0, nil)
		return
	}
	c.Fill(cx-s*1.3, cy-s*1.3, cx+s*1.3, cy+s*1.3, func(px, py float32) float32 {
		return abs32(sdHeart(px, py, cx, cy, s)+0.75) - 0.75
	}, p, 0, nil)
}

// buttonShine: the glossy highlight on buttons was removed (it looked dated).
const buttonShine = false

// iconPopOut: a window with an arrow leaving it (Settings into its own window).
func iconPopOut(c *Canvas, cx, cy float32, p Paint) {
	c.Line(cx-6, cy-5.5, cx-1.5, cy-5.5, 1.4, p)
	c.Line(cx-6, cy-5.5, cx-6, cy+6, 1.4, p)
	c.Line(cx-6, cy+6, cx+5.5, cy+6, 1.4, p)
	c.Line(cx+5.5, cy+6, cx+5.5, cy+1.5, 1.4, p)
	c.Line(cx-0.5, cy+0.5, cx+6, cy-6, 1.4, p)
	c.Line(cx+1.5, cy-6, cx+6, cy-6, 1.4, p)
	c.Line(cx+6, cy-6, cx+6, cy-1.5, 1.4, p)
}

// iconPopIn: the arrow going back into the window.
func iconPopIn(c *Canvas, cx, cy float32, p Paint) {
	c.Line(cx-6, cy-5.5, cx-1.5, cy-5.5, 1.4, p)
	c.Line(cx-6, cy-5.5, cx-6, cy+6, 1.4, p)
	c.Line(cx-6, cy+6, cx+5.5, cy+6, 1.4, p)
	c.Line(cx+5.5, cy+6, cx+5.5, cy+1.5, 1.4, p)
	c.Line(cx+6, cy-6, cx-0.5, cy+0.5, 1.4, p)
	c.Line(cx-0.5, cy+0.5, cx+4, cy+0.5, 1.4, p)
	c.Line(cx-0.5, cy+0.5, cx-0.5, cy-4, 1.4, p)
}
