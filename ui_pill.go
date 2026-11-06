package main

import (
	"fmt"
	"strings"
)

// Small glass windows that live outside the main player:
//   - the pill player that floats above the window while Settings is open
//   - the key-press popup in the corner of the screen ("Discord → 30%")
// Neither can see what's behind it cheaply, so they use a tinted glass.

const (
	PillW  = 360
	PillH  = 52
	ToastH = 44
)

// overlayGlass: a self-contained frosted pill (no screen capture needed).
func (u *UI) overlayGlass(c *Canvas, w, h float32) {
	r := h / 2
	c.Fill(0, 0, w, h, func(x, y float32) float32 { return sdRoundRect(x, y, 0, 0, w, h, r) },
		VGrad(0, h, Col{0.24, 0.25, 0.28, 0.90}, Col{0.13, 0.14, 0.16, 0.93}), 0, nil)
	c.Fill(0, 0, w, h, func(x, y float32) float32 { return sdRoundRect(x, y, 0, 0, w, h, r) },
		Radial(w*0.18, -h*0.6, w*0.7, u.accent.WithA(0.20)), 0, nil)
	if buttonShine {
		c.Fill(0, 0, w, h*0.5, func(x, y float32) float32 { return sdRoundRect(x, y, 3, 1.5, w-6, h*0.42, h*0.21) },
			VGrad(0, h*0.5, White(0.14), White(0)), 0, nil)
	}
	if u.Toggles[TogOutline] {
		px := 1 / c.S
		c.Fill(0, 0, w, h, func(x, y float32) float32 {
			d := sdRoundRect(x, y, 0, 0, w, h, r)
			return abs32(d+px*0.5) - px*0.5
		}, VGrad(0, h, White(0.55), White(0.12), White(0.28)), 0, nil)
	}
}

// RenderPill draws the settings pill player. Its hit areas are kept apart
// from the main window's (see PillHit).
// bx, by: where the pill sits in the player's blur backdrop (pixels), so it
// shows the same frosted background as the player.
func (u *UI) RenderPill(c *Canvas, bx, by float32) {
	u.scale = c.S
	c.ResetState()
	c.Clear()
	saved := u.hits
	u.hits = u.pillHits[:0]
	defer func() { u.pillHits, u.hits = u.hits, saved }()

	w, h := float32(PillW), float32(PillH)
	u.pillGlass(c, bx, by)
	cy := h / 2
	as := h - 16
	if u.Art != nil && u.Toggles[TogArt] {
		u.image(c, u.Art, fmt.Sprintf("art%d", u.ArtVer), 8, 8, as, as, as/2)
	} else {
		DrawAppIcon(c, 10, 10, as-4, u.accent)
	}
	u.addHit(c, hPillTrack, 8, 8, 8+as, 8+as)
	g := Solid(glyf)
	nx, px, bx := w-26, w-60, w-94
	iconForward(c, nx, cy, 0.56*u.btnScale(c, hPillNext, nx, cy, 15), g)
	u.playButtonGlyph(c, px, cy, 0.56*u.btnScale(c, hPillPlay, px, cy, 17))
	iconRewind(c, bx, cy, 0.56*u.btnScale(c, hPillPrev, bx, cy, 15), g)
	sx := bx - 32
	title, artist := u.trackText()
	tx := 8 + as + 10
	mw := sx - 18 - tx
	open := u.pillVolume(c, 1, hPillMute, hPillVol, sx, cy, tx-4, sx-16)
	if open < 0.99 {
		ga := c.GA
		c.GA = ga * (1 - open)
		u.text(c, u.fit(title, 12.5, 700, mw), tx, cy-8, 12.5, 700, txt, 0, true)
		u.text(c, u.fit(artist, 10.5, 500, mw), tx, cy+9, 10.5, 500, sub, 0, true)
		c.GA = ga
	}
}

func (u *UI) PillHit(x, y float32) int {
	for i := len(u.pillHits) - 1; i >= 0; i-- {
		h := u.pillHits[i]
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

// ToastWidth is the logical width of a key-press popup.
func (u *UI) ToastWidth(name, value string) float32 {
	w := 14 + 24 + 10 + u.textW(name, 12.5, 700) + 20
	if value != "" {
		w += 22 + u.textW(value, 12.5, 700)
	}
	return w
}

// RenderKeyToast: "Discord → 30%" with the app's icon.
func (u *UI) RenderKeyToast(c *Canvas, iconKey, name, value string, a float32) {
	u.scale = c.S
	c.ResetState()
	c.Clear()
	w, h := u.ToastWidth(name, value), float32(ToastH)
	u.overlayGlass(c, w, h)
	cy := h / 2
	if iconKey == "akra" || iconKey == "" {
		DrawAppIcon(c, 14, cy-12, 24, u.accent)
	} else {
		u.appIcon(c, iconKey, name, 26, cy, 24, false)
	}
	x := u.text(c, name, 48, cy, 12.5, 700, txt, 0, false) + 48
	if value != "" {
		ac := u.accent.Mix(White(1), 0.35)
		c.Line(x+6, cy, x+15, cy, 1.6, Solid(White(0.7)))
		c.Line(x+11.5, cy-3.5, x+15, cy, 1.6, Solid(White(0.7)))
		c.Line(x+11.5, cy+3.5, x+15, cy, 1.6, Solid(White(0.7)))
		u.text(c, value, x+22, cy, 12.5, 700, ac, 0, false)
	}
}

// withScale runs fn with text measured/rendered at scale s.
func (u *UI) withScale(s float32, fn func() float32) float32 {
	old := u.scale
	u.scale = s
	v := fn()
	u.scale = old
	return v
}

// pillGlass: the player's glass (blur, clarity, fluid, outline, color) on the pill.
func (u *UI) pillGlass(c *Canvas, bx, by float32) {
	p := BaseParams{PixW: c.W, PixH: c.H, S: c.S, PW: PillW, PH: PillH, Clarity: u.Clarity,
		Blur: u.Toggles[TogBlur] && u.Backdrop != nil, Backdrop: u.Backdrop, BScale: u.BackdropScale, BVer: u.BackdropVer,
		R: PillH / 2, Outline: u.Toggles[TogOutline], BX: bx, BY: by}
	key := fmt.Sprintf("%s|%.0f|%.0f", p.Key(), bx, by)
	if u.pillBase == nil || u.pillBaseKey != key {
		u.pillBase = BuildBase(p)
		u.pillBaseKey = key
	}
	var fl *Fluid
	strength := 0.62 * u.tog[TogFluid].V
	if strength > 0.005 {
		UpdateFluid(&u.pillFluid, u.T, u.fluidColors(), PillW/PillH)
		fl = &u.pillFluid
	}
	u.pillBase.Composite(c, u.accent, fl, strength)
}

// ---------- tooltip bubble (under the window) ----------

const tipMaxW = 300

// wrapText splits text into lines no wider than maxW.
func (u *UI) wrapText(s string, size float32, weight int, maxW float32) []string {
	var lines []string
	cur := ""
	for _, w := range strings.Fields(s) {
		try := w
		if cur != "" {
			try = cur + " " + w
		}
		if cur != "" && u.textW(try, size, weight) > maxW {
			lines = append(lines, cur)
			cur = w
		} else {
			cur = try
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}

// TipBubbleSize: the bubble's size (logical) for a tooltip text.
func (u *UI) TipBubbleSize(s string) (float32, float32) {
	lines := u.wrapText(s, 11.5, 500, tipMaxW-28)
	var w float32
	for _, l := range lines {
		w = max32(w, u.textW(l, 11.5, 500))
	}
	return w + 28 + 12, float32(len(lines))*17 + 22 + 12 // (+12: room for the shadow)
}

// RenderTipBubble draws a themed glass bubble with the tooltip text.
func (u *UI) RenderTipBubble(c *Canvas, s string) {
	u.scale = c.S
	c.ResetState()
	c.Clear()
	w, h := float32(c.W)/c.S, float32(c.H)/c.S
	bx, by, bw, bh := float32(6), float32(4), w-12, h-12
	c.Fill(0, 0, w, h, func(px, py float32) float32 { return sdRoundRect(px, py-3, bx+2, by+2, bw-4, bh-4, 14) },
		Solid(Black(0.35)), 6, nil)
	u.themedPanel(c, bx, by, bw, bh, 14)
	y := by + 11 + 8.5
	for _, l := range u.wrapText(s, 11.5, 500, tipMaxW-28) {
		u.text(c, l, bx+14, y, 11.5, 500, White(0.96), 0, false)
		y += 17
	}
}

// ---------- pop-out windows (mixer, keystrokes) ----------

// PopSize: the pop-out's size (logical).
func (u *UI) PopSize(kind int) (float32, float32) {
	switch kind {
	case PopMixer:
		n := float32(max(len(u.MixerApps), 3))
		return max32(300, min32(n*78+34, 620)), 330
	case PopExport:
		return 440, 372
	case PopImport:
		return 460, 470
	}
	return 430, 500
}

// glassFor: the player's glass (blur, clarity, fluid, outline, color) for a
// small window. bx, by: where it sits in the blur backdrop (pixels).
func (u *UI) glassFor(c *Canvas, slot string, w, h, r, bx, by float32) {
	p := BaseParams{PixW: c.W, PixH: c.H, S: c.S, PW: w, PH: h, Clarity: u.Clarity,
		Blur: u.Toggles[TogBlur] && u.Backdrop != nil, Backdrop: u.Backdrop, BScale: u.BackdropScale, BVer: u.BackdropVer,
		R: r, Outline: u.Toggles[TogOutline], BX: bx, BY: by}
	key := p.Key()
	if u.smallBases == nil {
		u.smallBases = map[string]*BaseLayer{}
		u.smallFluids = map[string]*Fluid{}
	}
	L := u.smallBases[slot]
	if L == nil || L.P.Key() != key {
		L = BuildBase(p)
		u.smallBases[slot] = L
	}
	var fl *Fluid
	strength := 0.62 * u.tog[TogFluid].V
	if strength > 0.005 {
		f := u.smallFluids[slot]
		if f == nil {
			f = &Fluid{}
			u.smallFluids[slot] = f
		}
		UpdateFluid(f, u.T, u.fluidColors(), w/h)
		fl = f
	}
	L.Composite(c, u.accent, fl, strength)
}

// RenderPopup draws the mixer or keystrokes pop-out. Its hit areas are kept
// apart from the main window's (see PopHit).
func (u *UI) RenderPopup(c *Canvas, kind int, bx, by float32) {
	u.scale = c.S
	c.ResetState()
	c.Clear()
	saved := u.hits
	u.hits = u.popHits[:0]
	defer func() { u.popHits, u.hits = u.hits, saved }()
	w, h := float32(c.W)/c.S, float32(c.H)/c.S
	u.glassFor(c, "pop", w, h, 24, bx, by)
	title := "Mixer"
	switch kind {
	case PopKeys:
		title = "Keystrokes"
	case PopExport:
		title = "Export settings"
	case PopImport:
		title = "Import settings"
	}
	u.text(c, title, 20, 30, 16, 700, txt, 0, true)
	u.headerButton(c, hPopClose, w-26, 30, false, iconClose)
	if kind == PopExport || kind == PopImport {
		u.drawTransfer(c, kind, w, h)
		return
	}
	u.headerButton(c, hPopPin, w-58, 30, u.PopPinned, iconPin)
	u.tip(c, "Pinned: stays open when you click somewhere else.", w-74, 14, w-42, 46)
	if kind == PopKeys {
		u.ghostPill(c, hEditKeys, "Edit folders", w-80, 30, 2)
		vy0, vy1 := float32(54), h-10
		sc := &u.popScroll
		oy := c.OY
		c.PushClip(8, vy0, w-8, vy1, 0)
		c.OY = oy - sc.Pos.X + sc.Rubber(200)
		ch := u.drawKeysList(c, 18, w-18, vy0+2)
		c.OY = oy
		c.PopClip()
		sc.Max = max32(0, ch-(vy1-vy0)+8)
		u.scrollbar(c, sc, w-8, vy0+6, vy1-vy0-12, ch)
		return
	}
	u.drawMixer(c, 14, w-14, 54, h-14)
}

func (u *UI) PopHit(x, y float32) int {
	for i := len(u.popHits) - 1; i >= 0; i-- {
		h := u.popHits[i]
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

// PopWheel scrolls the pop-out (keystrokes up/down, mixer sideways).
func (u *UI) PopWheel(delta float32) {
	if u.Pop == PopMixer {
		u.mixScroll.Scroll(delta, 0)
	} else {
		u.popScroll.Scroll(delta, 0)
	}
}

// popTipAt: tooltips aren't tracked for pop-outs (kept simple).
func (u *UI) popTipAt(x, y float32) string { return "" }
