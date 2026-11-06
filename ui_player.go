package main

import (
	"fmt"
	"math"
	"strings"
)

// ---------- player page ----------

func (u *UI) trackText() (string, string) {
	title, artist := u.Title, u.Artist
	switch u.Status {
	case StNotRunning:
		return "Spotify isn't open", "Press play to launch it"
	case StAd:
		return "Advertisement", "Spotify"
	case StPaused:
		if title == "" {
			return "Spotify", "Paused"
		}
	}
	if artist == "" {
		artist = "Spotify"
	}
	return title, artist
}

// declE: how far into Declutter mode the player is (eased 0..1).
func (u *UI) declE() float32 {
	d := clamp01(u.declA.V)
	return d * d * (3 - 2*d)
}

// fadeOut draws something that Declutter hides, fading it with d.
func (u *UI) fadeOut(c *Canvas, d float32, draw func()) {
	if d >= 0.99 {
		return
	}
	ga := c.GA
	c.GA = ga * (1 - d)
	draw()
	c.GA = ga
}

// sleepChip: the running sleep timer in the top bar ("☾ 23m").
func (u *UI) sleepChip(c *Canvas, x, cy, maxX float32) {
	if u.Sleep == SleepOff {
		return
	}
	label := "song"
	if u.Sleep != SleepSong {
		label = fmt.Sprintf("%dm", int(u.SleepLeft/60)+1)
	}
	w := 30 + u.textW(label, 11, 700)
	if x+w > maxX { // a narrow window: just the moon
		label, w = "", 26
	}
	hv := u.h(hSleepChip)
	c.RoundRect(x, cy-11, w, 22, 11, Solid(u.accent.WithA(0.30+0.15*hv)))
	c.StrokeRoundRect(x, cy-11, w, 22, 11, 1, Solid(u.accent.Mix(White(1), 0.5).WithA(0.6)))
	iconMoon(c, x+13, cy, Solid(White(0.95)))
	u.text(c, label, x+24, cy, 11, 700, White(0.95), 0, false)
	u.addHit(c, hSleepChip, x, cy-12, x+w, cy+12)
	u.tip(c, "Sleep timer: Spotify pauses then. Click to change it.", x, cy-12, x+w, cy+12)
}

func iconMoon(c *Canvas, cx, cy float32, p Paint) {
	c.Fill(cx-7, cy-7, cx+7, cy+7, func(px, py float32) float32 {
		return max32(sdCircle(px, py, cx, cy, 5.5), -sdCircle(px, py, cx+3.2, cy-2.4, 4.6))
	}, p, 0, nil)
}

func (u *UI) drawHeader(c *Canvas) {
	d := u.declE()
	R := u.PW
	u.fadeOut(c, d, func() {
		DrawAppIcon(c, 20, 21, 22, u.accent)
		u.text(c, "Akra Player", 51, 32, 13.5, 600, Col{1, 1, 1, 0.88}, 0, true)
		u.headerButton(c, hShrink, R-92, 32, false, iconShrink)
		u.headerButton(c, hEnlarge, R-60, 32, u.Full, iconEnlarge)
	})
	u.sleepChip(c, lerp(51+u.textW("Akra Player", 13.5, 600)+10, 18, d), 32, lerp(R-156, R-92, d)-18)
	// in Declutter the pin and pop-out slide over to where the size buttons were
	u.headerButton(c, hPin, lerp(R-156, R-92, d), 32, u.Toggles[TogOnTop], iconPin)
	u.headerButton(c, hMini, lerp(R-124, R-60, d), 32, false, iconMini)
	u.headerButton(c, hClose, R-28, 32, false, iconClose)
}

// The player lays itself out in whatever size the window is. Text scales
// with the window up to a limit (the app's zoom); past that, the layout
// itself spreads out: wider progress bar, bigger cover, more room.
func (u *UI) drawPlayer(c *Canvas) {
	u.drawHeader(c)
	if u.Toggles[TogArt] {
		u.drawPlayerArt(c)
	} else {
		u.drawPlayerPlain(c)
	}
	u.spotVolBar(c)
}

// drawResizeCorners: grab zones at the four corners (resize by corners only).
func (u *UI) drawResizeCorners(c *Canvas) {
	if u.Full {
		return
	}
	const z = 26
	PW, PH := u.PW, u.PH
	for i, id := range [...]int{hResizeNW, hResizeNE, hResizeSW, hResizeSE} {
		x0, y0 := float32(0), float32(0)
		if i%2 == 1 {
			x0 = PW - z
		}
		if i >= 2 {
			y0 = PH - z
		}
		u.addHit(c, id, x0, y0, x0+z, y0+z)
		if hv := max32(u.h(id), u.p(id)); hv > 0.01 { // a little arc shows it's grabbable
			cx, cy := float32(R), float32(R)
			if i%2 == 1 {
				cx = PW - R
			}
			if i >= 2 {
				cy = PH - R
			}
			rr := u.Radius() - 4
			ang := [...]float32{225, 315, 135, 45}[i] * math.Pi / 180
			c.Fill(cx-R, cy-R, cx+R, cy+R, func(px, py float32) float32 {
				d := abs32(sdCircle(px, py, cx, cy, rr)) - 1.3
				a := float32(math.Atan2(float64(py-cy), float64(px-cx)))
				da := abs32(float32(math.Remainder(float64(a-ang), 2*math.Pi)))
				return max32(d, (da-0.42)*rr)
			}, Solid(White(0.75*hv)), 0, nil)
		}
	}
}

func (u *UI) playButtonGlyph(c *Canvas, cx, cy, s float32) {
	g := Solid(glyf)
	m := u.morph.V
	ps := s
	ga := c.GA
	if m < 0.99 {
		c.GA = ga * (1 - m)
		iconPlay(c, cx, cy, ps*(0.7+0.3*(1-m)), g)
	}
	if m > 0.01 {
		c.GA = ga * m
		iconPause(c, cx, cy, ps*(0.7+0.3*m), g)
	}
	c.GA = ga
}

func (u *UI) colorDot(c *Canvas, cx, cy float32) {
	sc := u.btnScale(c, hColor, cx, cy, 21)
	hc := (sc - 1) * 12
	if u.AccentIdx == AccentRainbow {
		c.Fill(cx-11, cy-11, cx+11, cy+11, func(px, py float32) float32 { return sdCircle(px, py, cx, cy, 9.5+hc) },
			func(px, py float32) Col {
				a := float32(math.Atan2(float64(py-cy), float64(px-cx))/(2*math.Pi)) + u.T*0.25
				return HSV(a, 0.6, 1)
			}, 0, nil)
	} else {
		c.Circle(cx, cy, 9.5+hc, VGrad(cy-10, cy+10, u.accent.Mix(White(1), 0.3), u.accent))
	}
	c.StrokeCircle(cx, cy, 12.5+hc, 1.6, Solid(White(0.85)))
}

func (u *UI) bottomRow(c *Canvas, withExtras bool) {
	d := u.declE()
	by := u.PH - 38
	R := u.PW
	u.glassButton(c, hSettings, 40, by, 19)
	iconGear(c, 40, by, Solid(White(0.95)))
	u.fadeOut(c, d, func() {
		u.glassButton(c, hArtToggle, 82, by, 15)
		pc := White(0.62)
		if u.Toggles[TogArt] {
			pc = u.accent.Mix(White(1), 0.45)
		}
		iconPicture(c, 82, by, Solid(pc))
		u.colorDot(c, 121, by)
		u.glassButton(c, hKeysSheet, R-82, by, 15)
		iconKeyboard(c, R-82, by, Solid(White(0.92)))
	})
	u.glassButton(c, hControls, R-40, by, 19)
	iconMixer(c, R-40, by, Solid(White(0.95)))
	if withExtras {
		u.speaker(c, lerp(R-121, R-82, d), by) // takes the keystrokes button's place
	}
	cx := R / 2
	// the open / close key, hidden in Declutter
	pillY := by
	hint := false
	u.fadeOut(c, d, func() {
		if note := u.BindNote[ActShow]; note != "" {
			u.text(c, u.fit(note, 11.5, 600, 140), cx, by-12, 11.5, 600, Hex(0xffb4a8, 1), 1, true)
			hint = true
		} else if keys := u.Binds[ActShow]; len(keys) > 0 {
			u.text(c, u.fit(strings.Join(keys, " + "), 11.5, 600, 132), cx, by-15, 11.5, 600, Col{1, 1, 1, 0.75}, 1, true)
			u.text(c, "open / close", cx, by+0.5, 10.5, 500, dim, 1, false)
			hint = true
		}
	})
	if hint || d < 0.99 && len(u.Binds[ActShow]) > 0 {
		pillY = lerp(by+20, by, d)
	}
	// Declutter / Show swap places with a cross-fade
	ga := c.GA
	if d < 0.99 {
		c.GA = ga * (1 - d)
		u.miniPill(c, hDeclutter, "Declutter", cx, pillY, false, 1)
	}
	if d > 0.01 {
		c.GA = ga * d
		// Show is bigger, and grows with the window (up to a point)
		k := 1.3 + 0.35*clamp01((min32(u.PW/W, u.PH/H)-1)/0.6)
		u.miniPill(c, hShowAll, "Show", cx, pillY-(k-1)*4, true, k)
	}
	c.GA = ga
}

// miniPill: a small pill button (Declutter / Show).
func (u *UI) miniPill(c *Canvas, id int, label string, cx, cy float32, accent bool, k float32) {
	hv, pr := u.h(id), u.p(id)
	fs := 10 * k
	w := u.textW(label, fs, 600) + 20*k
	h := 19 * k * (1 - 0.06*pr)
	x := cx - w/2
	if accent {
		c.RoundRect(x, cy-h/2, w, h, h/2, VGrad(cy-h/2, cy+h/2, u.accent.Mix(White(1), 0.2+0.1*hv), u.accent))
		c.StrokeRoundRect(x, cy-h/2, w, h, h/2, 1, Solid(White(0.45)))
		u.text(c, label, cx, cy, fs, 600, u.onAccent(), 1, false)
	} else {
		c.RoundRect(x, cy-h/2, w, h, h/2, Solid(White(0.07+0.08*hv)))
		c.StrokeRoundRect(x, cy-h/2, w, h, h/2, 1, VGrad(cy-h/2, cy+h/2, White(0.32), White(0.06)))
		u.text(c, label, cx, cy, fs, 600, White(0.85), 1, false)
	}
	u.addHit(c, id, x-4, cy-h/2-3, x+w+4, cy+h/2+3)
}

func (u *UI) speaker(c *Canvas, cx, cy float32) {
	sc := u.btnScale(c, hMute, cx, cy, 21)
	ga := c.GA
	c.GA *= 0.92
	iconSpeakerScaled(c, cx, cy, sc, u.SpotMuted || u.SpotVol < 0.005, Solid(White(1)))
	c.GA = ga
	u.spkX, u.spkY = cx, cy
}

// spotVolBar: Spotify's own volume. Rises out of the speaker on hover.
func (u *UI) spotVolBar(c *Canvas) {
	e := u.spotVolA.V
	if e < 0.01 {
		return
	}
	e = e * e * (3 - 2*e)
	cx, cy := u.spkX, u.spkY
	bw := float32(30)
	bh := 132 * e
	x, y1 := cx-bw/2, cy-22
	y0 := y1 - bh
	ga := c.GA
	c.GA = ga * clamp01(e*1.6)
	c.Fill(x, y0, x+bw, y1, func(px, py float32) float32 { return sdRoundRect(px, py-4, x+2, y0+2, bw-4, bh-4, bw/2) },
		Solid(Black(0.38)), 10, nil)
	ac := u.themeTint()
	c.RoundRect(x, y0, bw, bh, bw/2, VGrad(y0, y1, ac.Mix(Black(1), 0.58).WithA(0.92), ac.Mix(Black(1), 0.78).WithA(0.94)))
	if buttonShine {
		c.Fill(x, y0, x+bw, y0+bh*0.4, func(px, py float32) float32 { return sdRoundRect(px, py, x+2, y0+1.5, bw-4, bh*0.35, bw/2-2) },
			VGrad(y0, y0+bh*0.4, White(0.16), White(0)), 0, nil)
	}
	c.StrokeRoundRect(x, y0, bw, bh, bw/2, 1, VGrad(y0, y1, ac.Mix(White(1), 0.6).WithA(0.55), White(0.10)))
	if e > 0.35 {
		t0, t1 := y0+30, y1-12
		v := clamp01(u.SpotVol)
		if u.SpotMuted {
			v = 0
		}
		id := hSlider0 + SlSpotVol
		hv, pr := u.h(id), u.p(id)
		tw := 5 + 2*max32(hv, pr)
		c.RoundRect(cx-tw/2, t0, tw, t1-t0, tw/2, Solid(White(0.16)))
		ky := t1 - (t1-t0)*v
		if t1-ky > 0.5 {
			c.RoundRect(cx-tw/2, ky, tw, t1-ky, tw/2, VGrad(t0, t1, u.accent.Mix(White(1), 0.25), u.accent))
		}
		kr := 7 + 1.2*max32(hv, pr)
		c.Circle(cx, ky, kr, VGrad(ky-kr, ky+kr, White(1), Hex(0xe4e8ef, 1)))
		u.text(c, fmt.Sprintf("%d", int(v*100+0.5)), cx, y0+16, 10.5, 600, White(0.9), 1, false)
		if c.GA > 0.5 {
			u.vsliders[id] = [2]float32{t0 + c.OY, t1 + c.OY}
		}
		u.addHit(c, hSpotVol, x-4, y0-4, x+bw+4, y1+2)
		u.addHit(c, id, x-4, t0-10, x+bw+4, t1+10)
	}
	c.GA = ga
}

// heart likes the song in Spotify.
func (u *UI) heart(c *Canvas, cx, cy float32) {
	sc := u.btnScale(c, hHeart, cx, cy, 17)
	l := u.likeA.V
	if l < 0.99 {
		iconHeart(c, cx, cy, 7.5*sc, false, Solid(White(0.80*(1-l)+0.2)))
	}
	if l > 0.01 {
		pop := 1 + 0.25*float32(math.Sin(float64(l)*math.Pi))
		iconHeart(c, cx, cy, 7.5*sc*pop*l, true, Solid(u.accent.Mix(White(1), 0.12)))
	}
}

// Layout without album art: big centered title, iOS-style controls.
func (u *UI) drawPlayerPlain(c *Canvas) {
	cx := u.PW / 2
	d := u.declE()
	dy := (u.PH-H)/2 - 8*d // keep the block centered between header and buttons
	grow := min32((u.PW-W)*0.35, 180) + 16*d
	title, artist := u.trackText()
	tw := float32(250) + grow*1.4
	if hv := u.h(hTrack); hv > 0.01 {
		c.RoundRect(cx-tw/2-6, 72+dy, tw+12, 66, 18, Solid(White(0.06*hv)))
	}
	u.addHit(c, hTrack, cx-tw/2-6, 72+dy, cx+tw/2+6, 138+dy)
	u.text(c, u.fit(title, 22, 700, tw), cx, 94+dy, 22, 700, txt, 1, true)
	u.text(c, u.fit(artist, 14.5, 500, tw), cx, 122+dy, 14.5, 500, sub, 1, true)
	half := 140 + grow
	if u.Dur > 0 {
		u.progress(c, cx-half, cx+half, 150+dy)
	} else {
		u.statusPill(c, cx, 154+dy)
	}

	cy := 206 + dy
	side := 168 + grow*0.8
	g := Solid(glyf)
	iconRewind(c, cx-86, cy, u.btnScale(c, hPrev, cx-86, cy, 26), g)
	iconForward(c, cx+86, cy, u.btnScale(c, hNext, cx+86, cy, 26), g)
	u.playButtonGlyph(c, cx, cy, u.btnScale(c, hPlay, cx, cy, 30))
	u.heart(c, cx-side, cy)
	u.speaker(c, cx+side, cy)
	u.bottomRow(c, false)
}

// level returns the i-th most recent audio level (0 = newest).
func (u *UI) level(i int) float32 {
	n := len(u.Levels)
	if i >= n || i < 0 {
		return 0
	}
	return u.Levels[n-1-i]
}

func (u *UI) statusPill(c *Canvas, cx, cy float32) {
	var label string
	switch u.Status {
	case StPlaying:
		label = "Playing"
	case StPaused:
		label = "Paused"
	case StAd:
		label = "Ad"
	default:
		label = "Not running"
	}
	pw := u.textW(label, 11, 600) + 30
	px, py := cx-pw/2, cy-11
	if u.playing() {
		c.RoundRect(px, py, pw, 22, 11, Solid(u.accent.WithA(0.28)))
		c.StrokeRoundRect(px, py, pw, 22, 11, 1, VGrad(py, py+22, u.accent.Mix(White(1), 0.5).WithA(0.7), u.accent.WithA(0.2)))
	} else {
		c.RoundRect(px, py, pw, 22, 11, Solid(White(0.08)))
		c.StrokeRoundRect(px, py, pw, 22, 11, 1, VGrad(py, py+22, White(0.28), White(0.06)))
	}
	dot := u.accent.Mix(White(1), 0.25)
	if !u.playing() {
		dot = White(0.6)
	}
	if u.Status == StNotRunning {
		dot = Hex(0xff8a7a, 0.9)
	}
	pulse := float32(1)
	if u.playing() {
		pulse = float32(0.75 + 0.25*math.Sin(float64(u.T)*4))
	}
	c.Circle(px+12, cy, 3.4, Solid(dot.WithA(pulse)))
	u.text(c, label, px+20, cy, 11, 600, White(0.92), 0, false)
}

// artLayout: where the cover and the column next to it go for this size.
func (u *UI) artLayout() (ax, ay, as, x0, x1, oy float32) {
	d := u.declE()
	as = max32(150, min32(min32(u.PH-150, u.PW*0.42), 560)) + 26*d // Declutter: the cover takes the freed space
	colW := max32(200, min32(u.PW-as-70, 520))
	gx := (u.PW - (as + 22 + colW)) / 2
	ax = gx
	ay = (u.PH-10)/2 - as/2 - 10 - 8*d
	x0 = gx + as + 22
	x1 = x0 + colW
	oy = ay + (as-150)/2 // the column sits level with the middle of the cover
	return
}

// Layout with album art: the cover on the left with a glow spilling off its
// edges; song info next to it.
func (u *UI) drawPlayerArt(c *Canvas) {
	title, artist := u.trackText()
	ax, ay, as, x0, x1, oy := u.artLayout()
	rad := float32(20) + (as-150)*0.06
	u.drawCover(c, ax, ay, as, rad)
	u.addHit(c, hTrack, ax, ay, ax+as, ay+as)

	u.text(c, u.fit(title, 17, 700, x1-x0-34), x0, oy+28, 17, 700, txt, 0, true)
	u.text(c, u.fit(artist, 13, 500, x1-x0-34), x0, oy+51, 13, 500, sub, 0, true)
	u.heart(c, x1-10, oy+38)
	u.progress(c, x0, x1, oy+86)

	cy := oy + 140
	mid := (x0 + x1) / 2
	g := Solid(glyf)
	iconRewind(c, mid-70, cy, 0.82*u.btnScale(c, hPrev, mid-70, cy, 24), g)
	iconForward(c, mid+70, cy, 0.82*u.btnScale(c, hNext, mid+70, cy, 24), g)
	u.playButtonGlyph(c, mid+0.5, cy, 0.85*u.btnScale(c, hPlay, mid, cy, 28))

	u.bottomRow(c, true)
}

// drawCover: glow, shadow, the cover and its rim. A new cover fizzles in,
// then its glow blooms outward.
func (u *UI) drawCover(c *Canvas, ax, ay, as, rad float32) {
	t := u.artIn
	if u.Art == nil {
		t = 1
	}
	phase := clamp01(t / 0.55)          // the cover phases in
	bloom := clamp01((t - 0.35) / 0.65) // then the glow spreads out
	bloom = 1 - (1-bloom)*(1-bloom)*(1-bloom)
	ga := c.GA
	if u.Art != nil && bloom > 0.001 && u.Toggles[TogGlow] {
		u.artGlow(c, ax, ay, as, rad, bloom)
	}
	c.GA = ga * (0.35 + 0.65*phase)
	c.Fill(ax, ay, ax+as, ay+as, func(px, py float32) float32 { return sdRoundRect(px, py-7, ax+5, ay+5, as-10, as-10, rad) },
		Solid(Black(0.30)), 14, func(px, py float32) float32 { return -sdRoundRect(px, py, ax, ay, as, as, rad) })
	c.GA = ga
	switch {
	case u.Art == nil:
		u.fallbackArt(c, ax, ay, as)
	case phase < 1:
		u.coverFizzle(c, ax, ay, as, rad, phase)
	default:
		u.image(c, u.Art, fmt.Sprintf("art%d", u.ArtVer), ax, ay, as, as, rad)
	}
	if buttonShine {
		c.Fill(ax, ay, ax+as, ay+as*0.5, func(px, py float32) float32 { return sdRoundRect(px, py, ax, ay, as, as, rad) },
			VGrad(ay, ay+as*0.5, White(0.18), White(0)), 0, nil)
	}
	c.GA = ga * (0.3 + 0.7*phase)
	c.StrokeRoundRect(ax, ay, as, as, rad, 1.2, VGrad(ay, ay+as, White(0.55), White(0.10), White(0.30)))
	c.GA = ga
}

// coverFizzle: the cover appears grain by grain, each grain flashing
// brighter as it lands.
func (u *UI) coverFizzle(c *Canvas, ax, ay, as, rad, t float32) {
	S := c.S
	pw, ph := int(math.Round(float64(as*S))), int(math.Round(float64(as*S)))
	sp := u.imageSprite(u.Art, fmt.Sprintf("art%d", u.ArtVer), pw, ph)
	if sp == nil {
		return
	}
	edge := float32(0.16)
	th := t * (1 + edge) // threshold sweeps past every grain
	c.Fill(ax, ay, ax+as, ay+as, func(px, py float32) float32 { return sdRoundRect(px, py, ax, ay, as, as, rad) },
		func(px, py float32) Col {
			ix, iy := int((px-ax)*S), int((py-ay)*S)
			n := grain(ix/2, iy/2) + 0.5 // 2x2-pixel grains
			// grains near the top-left arrive a little earlier: it sweeps in
			n = n*0.8 + 0.2*((px-ax)+(py-ay))/(2*as)
			d := th - n
			if d <= 0 {
				return Col{}
			}
			ix = max(0, min(ix, sp.W-1))
			iy = max(0, min(iy, sp.H-1))
			i := (iy*sp.W + ix) * 4
			a := sp.Pix[i+3]
			if a <= 0 {
				return Col{}
			}
			col := Col{sp.Pix[i] / a, sp.Pix[i+1] / a, sp.Pix[i+2] / a, a}
			if d < edge { // freshly landed: bright, then settles
				k := 1 - d/edge
				col = col.Mix(White(1), 0.65*k)
				col.A = a * (0.4 + 0.6*(1-k))
			}
			return col
		}, 0, nil)
}

func (u *UI) fallbackArt(c *Canvas, x, y, s float32) {
	ac := u.accent
	c.RoundRect(x, y, s, s, 20, func(px, py float32) Col {
		t := clamp01(((px - x) + (py - y)) / (2 * s))
		return lerpCol(ac.Mix(White(1), 0.55), ac.Mix(Black(1), 0.1), t)
	})
	cx, cy := x+s/2, y+s/2
	c.Circle(cx, cy, s*0.36, Solid(White(0.22)))
	c.StrokeCircle(cx, cy, s*0.36, 1, Solid(White(0.35)))
	c.Circle(cx, cy, s*0.15, Solid(White(0.28)))
	c.Circle(cx, cy, s*0.045, Solid(ac.Mix(Black(1), 0.2).WithA(0.8)))
}

// progressFrac returns 0..1, or -1 when unknown.
func (u *UI) progressFrac() float32 {
	if u.Dur <= 0 || !u.ProgKnown {
		return -1
	}
	return clamp01(u.posNow() / u.Dur)
}

func (u *UI) posNow() float32 {
	p := u.Pos
	if u.playing() {
		p += u.T - u.PosT
	}
	if u.Dur > 0 && p > u.Dur {
		p = u.Dur
	}
	return p
}

func fmtTime(s float32) string {
	if s < 0 {
		s = 0
	}
	t := int(s + 0.5)
	return fmt.Sprintf("%d:%02d", t/60, t%60)
}

func (u *UI) progress(c *Canvas, x0, x1, cy float32) {
	f := u.progressFrac()
	if u.Seeking {
		f = u.SeekFrac
	}
	hv, pr := u.h(hProgress), u.p(hProgress)
	th := 4 + 2*max32(hv, pr)
	c.RoundRect(x0, cy-th/2, x1-x0, th, th/2, Solid(White(0.18)))
	left, right := "—:—", "—:—"
	if u.Dur > 0 {
		right = fmtTime(u.Dur)
	}
	if f >= 0 {
		kx := x0 + (x1-x0)*f
		c.RoundRect(x0, cy-th/2, max32(th, kx-x0), th, th/2, HGrad(x0, x1, u.accent.Mix(White(1), 0.45), u.accent))
		kr := 5.5 + 2*max32(hv, pr)
		c.Circle(kx, cy, kr, Solid(u.accent.Mix(White(1), 0.25)))
		c.StrokeCircle(kx, cy, kr, 1, Solid(White(0.7)))
		pos := u.posNow()
		if u.Seeking {
			pos = u.SeekFrac * u.Dur
		}
		left = fmtTime(pos)
	}
	if u.Seekable && u.Dur > 0 {
		u.sliders[hProgress] = [2]float32{x0 + c.OX, x1 + c.OX}
		u.addHit(c, hProgress, x0-6, cy-12, x1+6, cy+12)
	}
	if u.Remaining && u.Dur > 0 && f >= 0 { // time left instead of the length
		pos := u.posNow()
		if u.Seeking {
			pos = u.SeekFrac * u.Dur
		}
		right = "-" + fmtTime(u.Dur-pos)
	}
	u.text(c, left, x0, cy+16, 10.5, 500, dim, 0, false)
	rc := dim
	if hv := u.h(hTimeRight); hv > 0.01 {
		rc = lerpCol(dim, txt, hv)
	}
	u.text(c, right, x1, cy+16, 10.5, 500, rc, 2, false)
	if u.Dur > 0 { // click the length to show the time left instead
		rw := u.textW(right, 10.5, 500)
		u.addHit(c, hTimeRight, x1-rw-6, cy+11, x1+6, cy+26)
		u.tip(c, "Click to switch between the song's length and the time left.", x1-rw-6, cy+11, x1+6, cy+26)
	}
}

// ---------- sheets (slide up over the player) ----------

func (u *UI) drawSheet(c *Canvas, which int) {
	title := "Mixer"
	if which == SheetKeys {
		title = "Keystrokes"
	}
	u.smallButton(c, hSheetClose, 30, 32, false)
	iconChevron(c, 30, 33, 0, Solid(White(0.95)))
	u.text(c, title, 54, 32, 19, 700, txt, 0, true)
	if which == SheetKeys {
		u.ghostPill(c, hEditKeys, "Edit", u.PW-24, 32, 2)
	} else {
		u.smallButton(c, hClose, u.PW-30, 32, false)
		iconClose(c, u.PW-30, 32, Solid(White(0.92)))
	}

	// scrolling body, clipped to a rounded viewport
	vx, vy, vw, vh := float32(14), float32(56), u.PW-28, u.PH-56-12
	if vw > 560 { // very wide windows: keep the sheet a readable width
		vx, vw = (u.PW-560)/2, 560
	}
	u.card(c, vx, vy, vw, vh, 22)
	oldOY := c.OY
	c.PushClip(vx+1, vy+1, vx+vw-1, vy+vh-1, 21)
	sc := &u.sheetScroll
	c.OY = oldOY - sc.Pos.X + sc.Rubber(160)
	var contentH float32
	if which == SheetKeys {
		contentH = u.drawKeysList(c, vx+18, vx+vw-18, vy+8)
	} else {
		c.OY = oldOY
		u.drawMixer(c, vx+10, vx+vw-10, vy+6, vy+vh-6)
		contentH = vh - 8
	}
	sc.Max = max32(0, contentH-vh+8)
	c.OY = oldOY
	c.PopClip()
	u.scrollbar(c, sc, vx+vw-6, vy+10, vh-20, contentH)
}

func (u *UI) scrollbar(c *Canvas, sc *Scroller, x, y, h, contentH float32) {
	if sc.Max <= 1 {
		return
	}
	vis := h / (h + sc.Max)
	bh := max32(24, h*vis)
	by := y + (h-bh)*clamp01(sc.Pos.X/sc.Max)
	c.RoundRect(x-1.5, by, 3, bh, 1.5, Solid(White(0.30)))
}

// chip shows a chosen window/app with its icon; opens the picker.
// playChipTarget: the chosen window, or "None" once it has been closed.
func (u *UI) playChipTarget() Target {
	t := u.PlayTarget
	if u.PlayGone && t.Name != "" {
		sub := t.Sub + " was closed"
		if t.Sub == "" {
			sub = "That window was closed"
		}
		return Target{Name: "None", Sub: sub, IconKey: "none"}
	}
	return t
}

func (u *UI) targetChip(c *Canvas, id int, t Target, x, cy, maxW float32) {
	hv, pr := u.h(id), u.p(id)
	h := float32(40)
	y := cy - h/2
	sc := 1 - 0.03*pr
	w := maxW * sc
	xx := x + (maxW-w)/2
	c.RoundRect(xx, y, w, h, 14, VGrad(y, y+h, White(0.13+0.06*hv), White(0.06+0.04*hv)))
	c.StrokeRoundRect(xx, y, w, h, 14, 1, VGrad(y, y+h, White(0.40), White(0.08)))
	if t.IconKey == "none" { // the chosen window was closed
		c.StrokeCircle(x+22, cy, 11, 1.4, Solid(White(0.5)))
		c.RoundRect(x+16, cy-0.8, 12, 1.6, 0.8, Solid(White(0.75)))
		u.text(c, "None", x+42, cy-7, 12.5, 600, txt, 0, false)
		u.text(c, u.fit(t.Sub, 10.5, 500, maxW-80), x+42, cy+9, 10.5, 500, dim, 0, false)
	} else if t.Name == "" {
		c.StrokeCircle(x+22, cy, 11, 1.4, Solid(White(0.6)))
		iconPlus(c, x+22, cy, 4.5, Solid(White(0.85)))
		u.text(c, "Choose…", x+42, cy, 12.5, 600, White(0.9), 0, false)
	} else {
		u.appIcon(c, t.IconKey, t.Name, x+22, cy, 24, false)
		u.text(c, u.fit(t.Name, 12.5, 600, maxW-80), x+42, cy-7, 12.5, 600, txt, 0, false)
		u.text(c, u.fit(t.Sub, 10.5, 500, maxW-80), x+42, cy+9, 10.5, 500, dim, 0, false)
	}
	iconChevron(c, x+maxW-18, cy, 0, Solid(White(0.8)))
	u.addHit(c, id, x, y, x+maxW, y+h)
}

// bindLine: key caps + Change (or the recording state) for one action.
func (u *UI) bindLine(c *Canvas, act int, x0, x1, cy float32) {
	id := hBind0 + act
	if u.RecIdx == act {
		u.recordingPill(c, x0, cy, x1-x0-80)
		u.ghostPill(c, id, "Cancel", x1, cy, 2)
		return
	}
	label := "Change"
	if len(u.Binds[act]) == 0 {
		label = "Set key"
	}
	bw := u.textW(label, 11.5, 600) + 22
	u.keyCaps(c, u.Binds[act], x0, cy, x1-x0-bw-10, 11.5, 0)
	if note := u.BindNote[act]; note != "" {
		nw := min32(u.textW(note, 11, 600)+20, x1-x0)
		c.RoundRect(x1-nw, cy-12.5, nw, 25, 12.5, Solid(Hex(0xff453a, 0.32)))
		c.StrokeRoundRect(x1-nw, cy-12.5, nw, 25, 12.5, 1, Solid(Hex(0xffb0a6, 0.5)))
		u.text(c, u.fit(note, 11, 600, nw-16), x1-nw/2, cy, 11, 600, White(1), 1, false)
		u.addHit(c, id, x1-nw-2, cy-15, x1+2, cy+15)
	} else {
		u.accentPill(c, id, label, x1, cy, 2)
	}
}

func (u *UI) recordingPill(c *Canvas, x, cy, maxW float32) {
	label := "Press a key…"
	if len(u.RecKeys) > 0 {
		label = strings.Join(u.RecKeys, " + ") + " + …"
	}
	label = u.fit(label, 11.5, 600, maxW-22)
	w := u.textW(label, 11.5, 600) + 22
	pulse := float32(0.45 + 0.4*math.Abs(math.Sin(float64(u.T)*3.2)))
	c.RoundRect(x, cy-12.5, w, 25, 12.5, Solid(u.accent.WithA(0.22)))
	c.StrokeRoundRect(x, cy-12.5, w, 25, 12.5, 1.3, Solid(u.accent.Mix(White(1), 0.4).WithA(pulse)))
	u.text(c, label, x+11, cy, 11.5, 600, White(0.95), 0, false)
}

// warnLine: a heads-up under a keybind (never blocks the key).
func (u *UI) warnLine(c *Canvas, act int, x, cy, maxW float32) {
	w := u.BindWarn[act]
	if w == "" || u.RecIdx == act {
		return
	}
	col := Hex(0xffd60a, 0.95)
	s := u.fit(w, 10.5, 600, maxW-22)
	pw := u.textW(s, 10.5, 600) + 24
	c.RoundRect(x-2, cy-9, pw, 18, 9, Solid(Col{0.10, 0.09, 0.05, 0.42}))
	c.Circle(x+7, cy, 4, Solid(col.WithA(0.9)))
	c.Line(x+7, cy-2, x+7, cy+0.4, 1.1, Solid(Black(0.8)))
	c.Circle(x+7, cy+1.8, 0.6, Solid(Black(0.8)))
	u.text(c, s, x+15, cy, 10.5, 600, col.Mix(White(1), 0.45), 0, false)
}

// actTitle draws an action's name; Volume Toggles get the app's icon and name.
func (u *UI) actTitle(c *Canvas, act int, x, cy, size float32, weight int, col Col, maxW float32) {
	if i := u.presetIndex(act); i >= 0 {
		p := u.Presets[i]
		w := u.text(c, "Volume Toggle", x, cy, size, weight, col, 0, false)
		isz := size + 5
		ix := x + w + 6 + isz/2
		if p.App.Name == "" {
			c.StrokeCircle(ix, cy, isz/2-1, 1.2, Solid(White(0.5)))
			iconPlus(c, ix, cy, 3, Solid(White(0.7)))
			u.marquee(c, "pick app", ix+isz/2+5, cy, size-1.5, 500, sub, maxW-(ix+isz/2+5-x))
		} else {
			u.appIcon(c, p.App.IconKey, p.App.Name, ix, cy, isz, false)
			u.marquee(c, p.App.Name, ix+isz/2+5, cy, size-1, 500, sub, maxW-(ix+isz/2+5-x))
		}
		return
	}
	u.marquee(c, u.ActLabel(act), x, cy, size, weight, col, maxW)
}

// drawControls: everything for app play/pause and volume in one place.
// Returns content height.
func (u *UI) drawControls(c *Canvas, x0, x1, top float32) float32 {
	y := top
	head := func(label, hint string) {
		u.text(c, label, x0, y+18, 13, 600, Col{1, 1, 1, 0.94}, 0, false)
		if hint != "" {
			u.text(c, hint, x1, y+18, 11, 500, dim, 2, false)
		}
	}

	head("Play / pause a window", "any window")
	u.targetChip(c, hChipPlay, u.playChipTarget(), x0, y+52, x1-x0)
	u.bindLine(c, ActAppPlay, x0, x1, y+90)
	u.warnLine(c, ActAppPlay, x0, y+112, x1-x0)
	y += 122
	u.separator(c, x0, x1, y)

	for i, p := range u.Presets {
		act := PresetAct(p.ID)
		u.actTitle(c, act, x0, y+18, 13, 600, Col{1, 1, 1, 0.94}, x1-x0)
		u.text(c, fmt.Sprintf("%d%%", p.Pct), x1-30, y+18, 13, 600, u.accent.Mix(White(1), 0.5), 2, false)
		u.iconButton(c, hPresetDel0+i, x1-10, y+18, iconTrash)
		u.targetChip(c, hPresetChip0+i, p.App, x0, y+52, x1-x0)
		u.slider(c, SlPreset0+i, float32(p.Pct)/100, x0+8, x1-8, y+90)
		u.bindLine(c, act, x0, x1, y+124)
		u.warnLine(c, act, x0, y+146, x1-x0)
		y += 156
		u.separator(c, x0, x1, y)
	}
	u.ghostPill(c, hAddPreset, "+  Add an app", (x0+x1)/2, y+24, 1)
	y += 48
	u.separator(c, x0, x1, y)

	head("Current window volume", "")
	u.text(c, fmt.Sprintf("%d%%", u.CurPct), x1, y+18, 13, 600, u.accent.Mix(White(1), 0.5), 2, false)
	u.text(c, "Whatever app you're in when you press the key", x0, y+40, 11, 500, dim, 0, false)
	u.slider(c, SlCurVol, float32(u.CurPct)/100, x0+8, x1-8, y+70)
	u.bindLine(c, ActVolCur, x0, x1, y+104)
	u.warnLine(c, ActVolCur, x0, y+126, x1-x0)
	y += 136
	u.separator(c, x0, x1, y)

	head("Hold to lower music", "")
	u.text(c, fmt.Sprintf("%d%%", u.DuckPct), x1, y+18, 13, 600, u.accent.Mix(White(1), 0.5), 2, false)
	u.text(c, u.fit("Like push-to-talk: the key still reaches Discord", 11, 500, x1-x0), x0, y+40, 11, 500, dim, 0, false)
	dt := u.DuckTarget
	if dt.Name == "" {
		dt = Target{Name: "Spotify", Sub: "Music to lower"}
	}
	u.targetChip(c, hDuckChip, dt, x0, y+74, x1-x0)
	u.slider(c, SlDuck, float32(u.DuckPct)/100, x0+8, x1-8, y+112)
	u.bindLine(c, ActDuck, x0, x1, y+146)
	u.warnLine(c, ActDuck, x0, y+168, x1-x0)
	y += 178

	u.text(c, "Preset keys toggle: first press sets the %, next press goes back to 100%.", (x0+x1)/2, y+14, 10.5, 500, dim, 1, false)
	return y + 30 - top
}

// drawKeysList: read-only overview of every keystroke, grouped by folder.
func (u *UI) drawKeysList(c *Canvas, x0, x1, top float32) float32 {
	y := top + 6
	if len(u.Profiles) > 1 {
		u.text(c, "Profile: "+u.Profiles[u.Profile], x0, y+12, 11.5, 600, u.accent.Mix(White(1), 0.5), 0, false)
		y += 26
	}
	for _, f := range u.Folders {
		if len(f.Acts) == 0 {
			continue
		}
		iconFolder(c, x0+9, y+14, Solid(u.accent.Mix(White(1), 0.45)))
		u.text(c, f.Name, x0+26, y+14, 12.5, 700, txt, 0, false)
		u.text(c, fmt.Sprintf("%d", len(f.Acts)), x1, y+14, 11, 600, dim, 2, false)
		y += 30
		for _, act := range f.Acts {
			// each row can be changed right here: click Change, press a key
			rec := u.RecIdx == act
			rh := float32(38)
			if rec {
				rh = 58
			}
			c.RoundRect(x0, y, x1-x0, rh, 11, Solid(White(0.05+0.04*b2f(rec))))
			cy := y + 19
			id := hBind0 + act
			if rec {
				u.recordingPill(c, x0+10, cy, x1-x0-90)
				u.ghostPill(c, id, "Cancel", x1-8, cy, 2)
				u.marquee(c, "Press Backspace to remove keybind · Esc to cancel", x0+12, y+44, 10, 500, dim, x1-x0-24)
			} else {
				bw := u.ghostPill(c, id, "Change", x1-8, cy, 2)
				capsW := min32(170, (x1-x0)*0.45)
				u.actTitle(c, act, x0+12, cy, 12, 500, Col{1, 1, 1, 0.9}, x1-x0-capsW-bw-34)
				u.keyCaps(c, u.Binds[act], x1-bw-16, cy, capsW, 10.5, 2)
				if len(u.Binds[act]) == 0 {
					u.text(c, "Not set", x1-bw-16, cy, 10.5, 500, dim, 2, false)
				}
			}
			y += rh + 4
		}
		y += 8
	}
	return y - top
}

// ---------- mini mode ----------

func (u *UI) drawMini(c *Canvas) {
	PW, PH := u.PW, u.PH
	cy := PH / 2
	title, artist := u.trackText()
	as := min32(PH-22, 54)
	ax, ay := (PH-as)/2, cy-as/2
	if u.Art != nil && u.Toggles[TogArt] {
		u.image(c, u.Art, fmt.Sprintf("art%d", u.ArtVer), ax, ay, as, as, as*0.28)
		c.StrokeRoundRect(ax, ay, as, as, as*0.28, 1, VGrad(ay, ay+as, White(0.5), White(0.1)))
	} else {
		DrawAppIcon(c, ax+as*0.12, ay+as*0.12, as*0.76, u.accent)
	}
	u.addHit(c, hTrack, ax, ay, ax+as, ay+as)
	g := Solid(glyf)
	rx := PW - 30
	u.headerButton(c, hMiniRestore, rx, cy, false, iconRestore)
	nx, px, bx := rx-36, rx-70, rx-104
	iconForward(c, nx, cy, 0.62*u.btnScale(c, hNext, nx, cy, 17), g)
	u.playButtonGlyph(c, px, cy, 0.62*u.btnScale(c, hPlay, px, cy, 19))
	iconRewind(c, bx, cy, 0.62*u.btnScale(c, hPrev, bx, cy, 17), g)
	sx := bx - 36
	tx := ax + as + 12
	mw := sx - 20 - tx
	open := u.pillVolume(c, 0, hMute, hMiniVol, sx, cy, tx, sx-18)
	if open < 0.99 {
		ga := c.GA
		c.GA = ga * (1 - open)
		u.text(c, u.fit(title, 13.5, 700, mw), tx, cy-9, 13.5, 700, txt, 0, true)
		u.text(c, u.fit(artist, 11.5, 500, mw), tx, cy+10, 11.5, 500, sub, 0, true)
		c.GA = ga
	}
	bx = sx // (progress line below ends before the speaker)
	if f := u.progressFrac(); f >= 0 {
		x0, x1 := tx, bx-20
		c.RoundRect(x0, cy+23, x1-x0, 2.4, 1.2, Solid(White(0.16)))
		c.RoundRect(x0, cy+23, max32(2.4, (x1-x0)*f), 2.4, 1.2, Solid(u.accent.Mix(White(1), 0.3)))
	}
}

// ---------- mixer ----------

// drawMixer: every app playing sound, each with a vertical volume slider and
// its icon and name underneath. Scrolls sideways when there are many.
func (u *UI) drawMixer(c *Canvas, x0, x1, y0, y1 float32) {
	apps := u.MixerApps
	if len(apps) == 0 {
		u.text(c, "No apps are playing sound right now", (x0+x1)/2, (y0+y1)/2-8, 12.5, 600, sub, 1, false)
		u.text(c, "Start something and it shows up here", (x0+x1)/2, (y0+y1)/2+12, 10.5, 500, dim, 1, false)
		return
	}
	const colW, gap = float32(72), float32(6)
	total := float32(len(apps))*(colW+gap) - gap
	sc := &u.mixScroll
	sc.Max = max32(0, total-(x1-x0))
	off := sc.Pos.X - sc.Rubber(120)
	start := x0
	if total < x1-x0 { // few apps: center them
		start = (x0+x1)/2 - total/2
		off = 0
	}
	c.PushClip(x0, y0, x1, y1, 0)
	for i, ap := range apps {
		cx := start + float32(i)*(colW+gap) + colW/2 - off
		if cx < x0-colW || cx > x1+colW {
			continue
		}
		hv := max32(u.h(hMixVol0+i), u.h(hMixIcon0+i))
		c.RoundRect(cx-colW/2, y0, colW, y1-y0, 16, Solid(White(0.04+0.04*hv)))
		v := clamp01(ap.Vol)
		if ap.Muted {
			v = 0
		}
		u.text(c, fmt.Sprintf("%d", int(ap.Vol*100+0.5)), cx, y0+16, 11, 600, lerpCol(sub, txt, b2f(!ap.Muted)), 1, false)
		t0, t1 := y0+32, y1-68
		id := hMixVol0 + i
		pr := u.p(id)
		tw := 6 + 2*max32(u.h(id), pr)
		c.RoundRect(cx-tw/2, t0, tw, t1-t0, tw/2, Solid(White(0.16)))
		ky := t1 - (t1-t0)*v
		fill := u.accent
		if ap.Muted {
			fill = White(0.4)
		}
		if t1-ky > 0.5 {
			c.RoundRect(cx-tw/2, ky, tw, t1-ky, tw/2, VGrad(t0, t1, fill.Mix(White(1), 0.25), fill))
		}
		kr := 8 + 1.5*max32(u.h(id), pr)
		c.Circle(cx, ky, kr, VGrad(ky-kr, ky+kr, White(1), Hex(0xe4e8ef, 1)))
		u.vsliders[id] = [2]float32{t0 + c.OY, t1 + c.OY}
		u.addHit(c, id, cx-colW/2+4, t0-12, cx+colW/2-4, t1+12)
		// icon (click to mute) and name
		iy := y1 - 42
		ga := c.GA
		if ap.Muted {
			c.GA = ga * 0.45
		}
		u.appIcon(c, ap.IconKey, ap.Name, cx, iy, 28*(1+0.08*u.h(hMixIcon0+i)), false)
		c.GA = ga
		if ap.Muted {
			c.Line(cx-12, iy+12, cx+12, iy-12, 2, Solid(Hex(0xff6b5e, 0.95)))
		}
		u.addHitC(c, hMixIcon0+i, cx, iy, 17)
		nw := u.textW(ap.Name, 10.5, 600)
		if nw <= colW-8 {
			u.text(c, ap.Name, cx, y1-14, 10.5, 600, txt, 1, false)
		} else {
			u.marquee(c, ap.Name, cx-colW/2+4, y1-14, 10.5, 600, txt, colW-8)
		}
	}
	c.PopClip()
}

// pillVolume: a speaker button for the pills. Hovering it slides out a
// volume bar over the song text (x0..x1). Returns how open the bar is.
func (u *UI) pillVolume(c *Canvas, slot, spkID, barID int, sx, cy, x0, x1 float32) float32 {
	sc := u.btnScale(c, spkID, sx, cy, 16)
	iconSpeakerScaled(c, sx, cy, 0.78*sc, u.SpotMuted || u.SpotVol < 0.005, Solid(White(0.95)))
	open := u.pvol[slot].V
	if open < 0.01 {
		return 0
	}
	e := open * open * (3 - 2*open)
	ga := c.GA
	c.GA = ga * e
	h := float32(30)
	bx0 := lerp(x1, x0, e)
	ac := u.themeTint()
	c.RoundRect(bx0, cy-h/2, x1-bx0, h, h/2, Solid(ac.Mix(Black(1), 0.65).WithA(0.85)))
	c.StrokeRoundRect(bx0, cy-h/2, x1-bx0, h, h/2, 1, Solid(ac.Mix(White(1), 0.5).WithA(0.5)))
	t0, t1 := bx0+14, x1-36
	if t1 > t0+10 {
		v := clamp01(u.SpotVol)
		if u.SpotMuted {
			v = 0
		}
		hv, pr := u.h(barID), u.p(barID)
		th := 5 + 2*max32(hv, pr)
		c.RoundRect(t0, cy-th/2, t1-t0, th, th/2, Solid(White(0.18)))
		kx := t0 + (t1-t0)*v
		if kx-t0 > 0.5 {
			c.RoundRect(t0, cy-th/2, kx-t0, th, th/2, HGrad(t0, t1, u.accent.Mix(White(1), 0.25), u.accent))
		}
		kr := 7 + 1.2*max32(hv, pr)
		c.Circle(kx, cy, kr, VGrad(cy-kr, cy+kr, White(1), Hex(0xe4e8ef, 1)))
		u.text(c, fmt.Sprintf("%d", int(v*100+0.5)), x1-12, cy, 10.5, 600, White(0.9), 2, false)
		if c.GA > 0.5 {
			u.sliders[barID] = [2]float32{t0 + c.OX, t1 + c.OX}
			u.addHit(c, barID, bx0, cy-h/2, x1+4, cy+h/2)
		}
	}
	c.GA = ga
	return e
}

// themeTint: the theme color for tinted glass (neutral gray for black & white).
func (u *UI) themeTint() Col {
	ac := u.accent
	if ac.Lum() > 0.8 {
		ac = Col{0.5, 0.52, 0.56, 1}
	}
	return ac
}
