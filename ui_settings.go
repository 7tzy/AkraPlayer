package main

import (
	"fmt"
	"math"
	"strings"
)

// ---------- settings page ----------

func (u *UI) drawSettings(c *Canvas) {
	if u.SetPopView { // its own window: put back, close
		iconGear(c, 30, 32, Solid(White(0.9)))
		u.text(c, "Settings", 50, 32, 19, 700, txt, 0, true)
		u.headerButton(c, hSetPop, u.PW-60, 32, false, iconPopIn)
		u.tip(c, "Put Settings back into the player window.", u.PW-74, 18, u.PW-46, 46)
		u.headerButton(c, hClose, u.PW-28, 32, false, iconClose)
	} else {
		u.smallButton(c, hBack, 30, 32, false)
		iconBack(c, 29, 32, Solid(White(0.95)))
		u.text(c, "Settings", 54, 32, 19, 700, txt, 0, true)
		u.headerButton(c, hSetPop, u.PW-124, 32, false, iconPopOut)
		u.tip(c, "Pop Settings out into its own window. The player comes back here, so you can see your changes on it.", u.PW-138, 18, u.PW-110, 46)
		u.headerButton(c, hShrink, u.PW-92, 32, false, iconShrink)
		u.headerButton(c, hEnlarge, u.PW-60, 32, u.Full, iconEnlarge)
		u.headerButton(c, hClose, u.PW-28, 32, false, iconClose)
	}

	// sidebar
	sx, sy, sw, sh := float32(14), float32(58), float32(136), u.PH-72
	u.card(c, sx, sy, sw, sh, 22)
	itemY := func(i float32) float32 { return sy + 28 + i*38 }
	ny := itemY(u.nav.X)
	c.RoundRect(sx+8, ny-16, sw-16, 32, 12, VGrad(ny-16, ny+16, White(0.20), White(0.12)))
	c.StrokeRoundRect(sx+8, ny-16, sw-16, 32, 12, 1, VGrad(ny-16, ny+16, White(0.40), White(0.06)))
	for i := 0; i < NumSections; i++ {
		y := itemY(float32(i))
		id := hNav0 + i
		if hv := u.h(id); hv > 0.01 && i != u.Section {
			c.RoundRect(sx+8, y-16, sw-16, 32, 12, Solid(White(0.06*hv)))
		}
		active := clamp01(1 - abs32(u.nav.X-float32(i)))
		col := White(0.70 + 0.30*active)
		ip := Solid(col)
		switch i {
		case SecGeneral:
			iconSwitch(c, sx+27, y, ip)
		case SecLook:
			iconPalette(c, sx+27, y, ip)
		case SecKeys:
			iconKeyboard(c, sx+27, y, ip)
		}
		u.text(c, secLabels[i], sx+43, y, 12.5, 500+100*int(active+0.5), col, 0, false)
		u.addHit(c, id, sx+6, y-17, sx+sw-6, y+17)
	}
	// little page dots show where scrolling will take you
	for i := 0; i < NumSections; i++ {
		dx := sx + sw/2 + (float32(i)-1)*12
		a := 0.25 + 0.65*clamp01(1-abs32(u.sec.X-float32(i)))
		c.Circle(dx, sy+sh-56, 2.6, Solid(White(a)))
	}
	u.text(c, "Akra Player", sx+sw/2, sy+sh-34, 11.5, 600, sub, 1, false)
	u.text(c, "Version "+appVersion, sx+sw/2, sy+sh-18, 10.5, 500, dim, 1, false)

	// content card with scrolling, paging sections
	kx, ky, kw, kh := float32(158), float32(58), u.PW-172, u.PH-72
	u.card(c, kx, ky, kw, kh, 22)
	oldOY, oldGA := c.OY, c.GA
	c.PushClip(kx+1, ky+1, kx+kw-1, ky+kh-1, 21)
	lim := u.scrollLimit()
	for i := 0; i < NumSections; i++ {
		// pages use the room they have, up to a comfortable reading width
		maxW := float32(620)
		if i == SecKeys {
			maxW = 860
		}
		cw := min32(kw-32, maxW)
		x0 := kx + (kw-cw)/2
		x1 := x0 + cw
		d := float32(i) - u.sec.X
		if abs32(d) >= 0.98 {
			continue
		}
		sc := &u.secScroll[i]
		// pages glide a short way and cross-fade, so text never jumps
		c.OY = oldOY + d*34 - sc.Pos.X + sc.Rubber(lim)
		c.GA = oldGA * clamp01(1-abs32(d)*1.9)
		var hgt float32
		switch i {
		case SecGeneral:
			hgt = u.drawGeneral(c, x0, x1, ky)
		case SecLook:
			hgt = u.drawLook(c, x0, x1, ky)
		case SecKeys:
			hgt = u.drawKeysSettings(c, x0, x1, ky)
		}
		sc.Max = max32(0, hgt-kh+6)
	}
	c.OY, c.GA = oldOY, oldGA
	c.PopClip()

	sc := &u.secScroll[u.Section]
	u.scrollbar(c, sc, kx+kw-6, ky+12, kh-24, 0)
}

// ---------- keybinds (profiles, folders, grid) ----------

// editField: the inline text box used to rename folders and profiles.
func (u *UI) editField(c *Canvas, x, cy, w float32) {
	c.RoundRect(x, cy-13, w, 26, 8, Solid(Black(0.25)))
	c.StrokeRoundRect(x, cy-13, w, 26, 8, 1.3, Solid(u.accent.Mix(White(1), 0.35)))
	tw := u.text(c, u.fit(u.EditText, 12.5, 600, w-16), x+8, cy, 12.5, 600, txt, 0, false)
	if math.Mod(float64(u.T), 1.0) < 0.55 {
		c.RoundRect(x+9+tw, cy-8, 1.6, 16, 0.8, Solid(u.accent.Mix(White(1), 0.4)))
	}
	u.addHit(c, hEditField, x, cy-13, x+w, cy+13)
}

// profileChips: one click switches between saved sets of keys and volumes.
func (u *UI) profileChips(c *Canvas, x0, x1, y float32) float32 {
	u.text(c, "Profiles", x0, y+12, 12, 700, txt, 0, false)
	u.marquee(c, "Keys, volume toggles and volumes", x0+62, y+12, 10.5, 500, dim, x1-x0-62-56)
	u.tip(c, "Profiles keep separate sets of keys and volumes, like Gaming and Studying. Click one to switch.", x0, y, x0+60, y+24)
	if u.EditKind != EditProfile {
		if len(u.Profiles) > 1 {
			u.iconButton(c, hProfileDel, x1-36, y+12, iconTrash)
		}
		u.iconButton(c, hProfileRen, x1-10, y+12, iconPencil)
	}
	y += 30
	if u.EditKind == EditProfile {
		u.editField(c, x0, y+14, x1-x0-70)
		u.accentPill(c, hProfileRen, "Done", x1, y+14, 2)
		return y + 36
	}
	cx, cy := x0, y+14
	for i, name := range u.Profiles {
		id := hProfile0 + i
		lbl := u.fit(name, 11.5, 600, 140)
		w := u.textW(lbl, 11.5, 600) + 26
		if cx+w > x1 && cx > x0 {
			cx, cy = x0, cy+32
		}
		hv, pr := u.h(id), u.p(id)
		sel := u.selV(id)
		in := 1.5 * pr
		c.RoundRect(cx+in, cy-13+in, w-2*in, 26-2*in, 13, Solid(lerpCol(White(0.08+0.07*hv), u.accent.WithA(0.85), sel)))
		c.StrokeRoundRect(cx+in, cy-13+in, w-2*in, 26-2*in, 13, 1, VGrad(cy-13, cy+13, White(0.40), White(0.06)))
		col := White(0.92)
		if sel > 0.5 {
			col = u.onAccent()
		}
		u.text(c, lbl, cx+w/2, cy, 11.5, 600, col, 1, false)
		u.addHit(c, id, cx, cy-14, cx+w, cy+14)
		cx += w + 6
	}
	if cx+30 > x1 {
		cx, cy = x0, cy+32
	}
	hv := u.h(hProfileAdd)
	c.Circle(cx+13, cy, 13, Solid(White(0.08+0.08*hv)))
	c.StrokeCircle(cx+13, cy, 13, 1, VGrad(cy-13, cy+13, White(0.36), White(0.06)))
	iconPlus(c, cx+13, cy, 4.6, Solid(White(0.9)))
	u.addHitC(c, hProfileAdd, cx+13, cy, 15)
	return cy + 22
}

// actHelp: what each keybind does (shown in the tooltip bubble).
func (u *UI) actHelp(act int) string {
	switch act {
	case ActPlay:
		return "Plays or pauses Spotify from anywhere, even while you're in a game."
	case ActShow:
		return "Shows or hides this player."
	case ActMute:
		return "Mutes or unmutes the app you're in (the window in front)."
	case ActAppPlay:
		return "Plays or pauses the window you choose here: a browser tab, VLC, a video player and so on."
	case ActVolCur:
		return "Sets the app you're in to this volume. Press it again to go back to 100%."
	case ActDuck:
		return "While you hold it, your music drops to this volume, like push-to-talk. The key still reaches Discord."
	case ActLike:
		return "Likes the song that's playing in Spotify."
	case ActYouTube:
		return "Plays or pauses YouTube in your browser (needs the Akra browser extension)."
	case ActDeclutter:
		return "Hides everything but the song and its controls, or brings it all back. Choose whether it works everywhere or only while Akra Player is the window in front."
	}
	if i := u.presetIndex(act); i >= 0 {
		p := u.Presets[i]
		name := p.App.Name
		if name == "" {
			return "Volume Toggle: pick an app (click its name), then set a key. Press once to set it to this volume; press again to put it back to 100%."
		}
		return fmt.Sprintf("Volume Toggle: press once to set %s to %d%%. Press again to put it back to 100%%.", name, p.Pct)
	}
	return ""
}

func (u *UI) bindBoxH(act int) float32 {
	switch {
	case IsPresetAct(act):
		return 150
	case act == ActAppPlay:
		return 172
	case act == ActDeclutter:
		return 172
	case act == ActVolCur:
		return 140
	case act == ActDuck:
		return 192
	}
	return 112
}

// bindBox: one keybind as a glass card in the grid (at most two across).
// Name and keys on top, Change below, so long combos never hit the button.
func (u *UI) bindBox(c *Canvas, act int, bx, by, bw, bh float32) {
	u.card(c, bx, by, bw, bh, 14)
	// keybinds that can be switched off: the switch sits in the bottom-right
	// corner, and everything else on the card fades while it's off
	var offA float32
	if CanSwitchOff(act) {
		onV := u.selV(hBindOn0 + act)
		offA = 1 - onV
		u.toggle(c, hBindOn0+act, onV, bx+bw-56, by+bh-20)
		u.tip(c, "Switch this keybind on or off. While it's off the key does nothing here and goes to other apps as normal.", bx+bw-62, by+bh-38, bx+bw, by+bh)
	}
	oldGA := c.GA
	c.GA = oldGA * (1 - 0.45*offA)
	defer func() { c.GA = oldGA }()
	u.actTitle(c, act, bx+12, by+19, 12, 600, Col{1, 1, 1, 0.94}, bw-48)
	u.tip(c, u.actHelp(act), bx, by, bx+bw-34, by+34)
	u.iconButton(c, hMove0+act, bx+bw-17, by+19, iconMove)
	id := hBind0 + act
	if u.RecIdx == act {
		u.recordingPill(c, bx+12, by+48, bw-24)
	} else {
		u.keyCaps(c, u.Binds[act], bx+12, by+48, bw-24, 11, 0)
	}
	if note := u.BindNote[act]; note != "" && u.RecIdx != act {
		u.marquee(c, note, bx+12, by+72, 10.5, 600, Hex(0xffb0a6, 1), bw-24)
	} else {
		u.warnLine(c, act, bx+12, by+72, bw-24)
	}
	pctRow := func(sl int, pct int, y float32) {
		u.slider(c, sl, float32(pct)/100, bx+20, bx+bw-52, y)
		u.text(c, fmt.Sprintf("%d%%", pct), bx+bw-12, y, 11.5, 600, u.accent.Mix(White(1), 0.5), 2, false)
	}
	switch {
	case u.presetIndex(act) >= 0:
		i := u.presetIndex(act)
		pctRow(SlPreset0+i, u.Presets[i].Pct, by+98)
		u.iconButton(c, hPresetDel0+i, bx+bw-20, by+bh-20, iconTrash)
		u.addHit(c, hPresetChip0+i, bx+8, by+6, bx+bw-36, by+32) // click the name to pick the app
	case act == ActAppPlay:
		u.targetChip(c, hChipPlay, u.playChipTarget(), bx+10, by+104, bw-20)
	case act == ActDeclutter:
		u.chipRow(c, hDeclScope0, []string{"Everywhere", "Only in app"}, bx+12, bx+bw-12, by+92)
		u.tip(c, "Everywhere: the key works in any app, even in a game. Only in app: it works only while Akra Player is the window in front, so the key stays free everywhere else.", bx, by+86, bx+bw, by+120)
	case act == ActVolCur:
		pctRow(SlCurVol, u.CurPct, by+96)
	case act == ActDuck:
		dt := u.DuckTarget
		if dt.Name == "" {
			dt = Target{Name: "Spotify", Sub: "Music to lower"}
		}
		u.targetChip(c, hDuckChip, dt, bx+10, by+106, bw-20)
		pctRow(SlDuck, u.DuckPct, by+144)
	}
	switch {
	case u.RecIdx == act:
		w := u.ghostPill(c, id, "Cancel", bx+12, by+bh-20, 0)
		u.marquee(c, "Press Backspace to remove keybind", bx+20+w, by+bh-20, 10, 500, dim, bw-w-30-b2f(IsPresetAct(act))*28-b2f(CanSwitchOff(act))*60)
	case len(u.Binds[act]) == 0:
		u.accentPill(c, id, "Set key", bx+12, by+bh-20, 0)
	default:
		u.accentPill(c, id, "Change", bx+12, by+bh-20, 0)
	}
}

// addToggleBox: the "+" card at the end of the Volume folder.
func (u *UI) addToggleBox(c *Canvas, bx, by, bw, bh float32) {
	hv, pr := u.h(hAddToggle), u.p(hAddToggle)
	in := 2 * pr
	c.RoundRect(bx+in, by+in, bw-2*in, bh-2*in, 14, Solid(White(0.04+0.06*hv)))
	c.Fill(bx, by, bx+bw, by+bh, func(px, py float32) float32 {
		d := sdRoundRect(px, py, bx+in, by+in, bw-2*in, bh-2*in, 14)
		dash := sinf((px + py) * 0.9) // a dashed outline
		return max32(abs32(d+0.6)-0.6, -dash*0.8)
	}, Solid(White(0.30+0.25*hv)), 0, nil)
	cy := by + bh/2
	c.Circle(bx+bw/2, cy-12, 13, Solid(u.accent.WithA(0.35+0.25*hv)))
	iconPlus(c, bx+bw/2, cy-12, 4.6, Solid(White(0.95)))
	u.text(c, "Add volume toggle", bx+bw/2, cy+14, 11.5, 600, txt, 1, false)
	u.addHit(c, hAddToggle, bx, by, bx+bw, by+bh)
	u.tip(c, "Adds a Volume Toggle for another app: one key that sets it to a volume, and back to 100%.", bx, by, bx+bw, by+bh)
}

func (u *UI) drawKeysSettings(c *Canvas, x0, x1, top float32) float32 {
	y := u.profileChips(c, x0, x1, top+6) + 6
	u.separator(c, x0, x1, y)
	y += 6
	cols := 1
	if x1-x0 > 300 {
		cols = 2
	}
	const gap = 8
	gx0, gx1 := x0-6, x1+6
	bw := (gx1 - gx0 - gap*float32(cols-1)) / float32(cols)
	for fi := range u.Folders {
		f := &u.Folders[fi]
		open := float32(1)
		if t := u.fold[fi]; t != nil {
			open = t.V
		}
		hy := y + 18
		hv := u.h(hFold0 + fi)
		if hv > 0.01 {
			c.RoundRect(x0-6, y+2, x1-x0+12, 32, 10, Solid(White(0.06*hv)))
		}
		iconChevron(c, x0+5, hy, 1-open, Solid(White(0.8)))
		iconFolder(c, x0+24, hy, Solid(u.accent.Mix(White(1), 0.45)))
		if u.EditKind == EditFolder && u.EditIdx == fi {
			u.editField(c, x0+38, hy, x1-x0-80)
			u.accentPill(c, hFoldRen0+fi, "Done", x1, hy, 2)
		} else {
			u.text(c, u.fit(f.Name, 13, 700, x1-x0-110), x0+38, hy, 13, 700, txt, 0, false)
			nw := u.textW(f.Name, 13, 700)
			u.text(c, fmt.Sprintf("%d", len(f.Acts)), x0+44+min32(nw, x1-x0-110), hy, 11, 600, dim, 0, false)
			u.addHit(c, hFold0+fi, x0-6, y+2, x1-60, y+34)
			u.iconButton(c, hFoldRen0+fi, x1-36, hy, iconPencil)
			if len(u.Folders) > 1 {
				u.iconButton(c, hFoldDel0+fi, x1-10, hy, iconTrash)
			}
		}
		y += 38

		// cards in this folder (-1 = the "add volume toggle" card in Volume)
		cards := append([]int(nil), f.Acts...)
		if f.Name == "Volume" {
			cards = append(cards, -1)
		}
		boxH := func(a int) float32 {
			if a < 0 {
				return 112
			}
			return u.bindBoxH(a)
		}
		var rows []float32
		for i := 0; i < len(cards); i += cols {
			h := boxH(cards[i])
			if i+1 < len(cards) && cols == 2 {
				h = max32(h, boxH(cards[i+1]))
			}
			rows = append(rows, h)
		}
		bodyH := float32(28)
		if len(rows) > 0 {
			bodyH = 0 // (rows of cards)
			for _, h := range rows {
				bodyH += h + gap
			}
		}
		vis := bodyH * open
		if vis > 0.5 {
			oldGA := c.GA
			c.PushClip(x0-8, y, x1+8, y+vis, 0)
			c.GA = oldGA * open
			ry := y - (1-open)*16
			if len(cards) == 0 {
				u.marquee(c, "Empty. Move keybinds here with the folder button.", x0+8, ry+14, 10.5, 500, dim, x1-x0-16)
			}
			for r, h := range rows {
				for k := 0; k < cols; k++ {
					i := r*cols + k
					if i >= len(cards) {
						break
					}
					if cards[i] < 0 {
						u.addToggleBox(c, gx0+float32(k)*(bw+gap), ry, bw, h)
					} else {
						u.bindBox(c, cards[i], gx0+float32(k)*(bw+gap), ry, bw, h)
					}
				}
				ry += h + gap
			}
			c.GA = oldGA
			c.PopClip()
		}
		y += vis + 4
	}
	u.ghostPill(c, hNewFolder, "+  New folder", (x0+x1)/2, y+18, 1)
	msg := "Keys without Ctrl/Alt/Shift work too, just watch for warnings"
	if u.textW(msg, 10.5, 500) <= x1-x0 {
		u.text(c, msg, (x0+x1)/2, y+48, 10.5, 500, dim, 1, false)
	} else {
		u.marquee(c, msg, x0, y+48, 10.5, 500, dim, x1-x0)
	}
	return y + 64 - top
}

func (u *UI) iconButton(c *Canvas, id int, cx, cy float32, icon func(*Canvas, float32, float32, Paint)) {
	hv, pr := u.h(id), u.p(id)
	r := 12 * (1 - 0.08*pr)
	c.Circle(cx, cy, r, Solid(White(0.07+0.10*hv)))
	icon(c, cx, cy, Solid(White(0.75+0.2*hv)))
	u.addHitC(c, id, cx, cy, r+2)
}

// ---------- general ----------

func (u *UI) toggleRow(c *Canvas, i int, x0, x1, y float32) float32 {
	cy := y + 33
	u.marquee(c, togLabels[i][0], x0, cy-9, 13, 600, Col{1, 1, 1, 0.94}, x1-x0-62)
	u.marquee(c, togLabels[i][1], x0, cy+10, 11, 500, dim, x1-x0-62)
	u.toggle(c, hTog0+i, u.tog[i].V, x1-44, cy)
	u.addHit(c, hTog0+i, x0-6, y+6, x1-50, y+60)
	u.tip(c, togTips[i], x0-6, y+4, x1+6, y+62)
	y += 66
	u.separator(c, x0, x1, y)
	return y
}

// ---------- feature boxes ----------

var featGreen = Hex(0x30d158, 1)

// toggleBox: a switchable feature as a glass box, like the keybind cards.
// How "on" looks depends on Appearance > "Switched-on look".
func (u *UI) toggleBox(c *Canvas, i int, x, y, w, h float32) {
	id := hTog0 + i
	on := u.tog[i].V
	hv, pr := u.h(id), u.p(id)
	in := 1.5 * pr
	bx, by, bw, bh := x+in, y+in, w-2*in, h-2*in
	u.card(c, bx, by, bw, bh, 14)
	if hv > 0.01 {
		c.RoundRect(bx, by, bw, bh, 14, Solid(White(0.05*hv)))
	}
	right := float32(14) // room on the right for the state
	switch u.TogStyle {
	case TogEdge: // green light along the inside edges
		if on > 0.01 {
			reach := float32(13)
			c.Fill(bx, by, bx+bw, by+bh, func(px, py float32) float32 { return sdRoundRect(px, py, bx, by, bw, bh, 14) },
				func(px, py float32) Col {
					d := -sdRoundRect(px, py, bx, by, bw, bh, 14)
					t := clamp01(1 - d/reach)
					return featGreen.WithA(0.55 * t * t * on)
				}, 0, nil)
			c.StrokeRoundRect(bx, by, bw, bh, 14, 1.2, Solid(featGreen.Mix(White(1), 0.3).WithA(0.75*on)))
		}
	case TogFrosted: // the whole box frosted green
		if on > 0.01 {
			c.RoundRect(bx, by, bw, bh, 14, VGrad(by, by+bh, featGreen.Mix(White(1), 0.25).WithA(0.42*on), featGreen.WithA(0.30*on)))
			c.StrokeRoundRect(bx, by, bw, bh, 14, 1, Solid(featGreen.Mix(White(1), 0.5).WithA(0.6*on)))
		}
	default: // a switch
		u.toggle(c, id, on, bx+bw-44, by+bh/2)
		right = 62
	}
	if u.TogStyle != TogClassic {
		state := "Off"
		if u.Toggles[i] {
			state = "On"
			if i == TogVis {
				state = "Shown"
			}
		} else if i == TogVis {
			state = "Hidden"
		}
		sw := u.textW(state, 10.5, 700)
		u.text(c, state, bx+bw-12, by+16, 10.5, 700, lerpCol(dim, featGreen.Mix(White(1), 0.45), on), 2, false)
		right = sw + 22
	}
	// features with their own settings get a pencil while they're on
	edit := HasTogEdit(i) && on > 0.01
	ex, ey := bx+bw-20, by+bh-19 // bottom-right (the state is top-right)
	if u.TogStyle == TogClassic {
		ex, ey = bx+bw-74, by+bh/2 // just left of the switch
		if edit {
			right = 92
		}
	}
	u.marquee(c, togLabels[i][0], bx+12, by+18, 12.5, 600, Col{1, 1, 1, 0.94}, bw-12-right)
	descW := bw - 24
	if u.TogStyle == TogClassic {
		descW = bw - 12 - right // keep clear of the switch (and pencil)
	}
	if edit && u.TogStyle != TogClassic {
		descW = bw - 24 - 30*on
	}
	u.marquee(c, togLabels[i][1], bx+12, by+bh-18, 10.5, 500, dim, descW)
	u.addHit(c, id, bx, by, bx+bw, by+bh)
	u.tip(c, togTips[i], bx, by, bx+bw, by+bh)
	if edit {
		ga := c.GA
		c.GA = ga * on
		pid := hTogEdit0 + i
		hv := u.h(pid)
		sel := u.selV(pid)
		c.Circle(ex, ey, 12, Solid(lerpCol(White(0.10+0.10*hv), u.accent.WithA(0.85), sel)))
		iconPencil(c, ex, ey, Solid(White(0.92)))
		u.addHitC(c, pid, ex, ey, 13)
		u.tip(c, "Change how "+strings.ToLower(togLabels[i][0])+" works.", ex-13, ey-13, ex+13, ey+13)
		c.GA = ga
	}
}

// togEditH: height of the open feature settings panel.
func (u *UI) togEditH(x0, x1 float32) float32 {
	switch u.togEditShown {
	case TogIdleFade:
		return 30 + 2*62 + 18
	case TogSongPopup:
		return 30 + 26 + u.chipRowsH(u.songScreens(), x0+6, x1-6) + 26 + 64 + 18
	}
	return 0
}

func (u *UI) songScreens() []string {
	return append([]string{"Where you're working"}, u.Monitors...)
}

// togEditor: the settings of Fade when idle or Now playing popups.
func (u *UI) togEditor(c *Canvas, x0, x1, t float32) float32 {
	h := u.togEditH(x0, x1) - 8
	c.RoundRect(x0-6, t, x1-x0+12, h, 14, Solid(White(0.05)))
	c.StrokeRoundRect(x0-6, t, x1-x0+12, h, 14, 1, Solid(u.accent.Mix(White(1), 0.4).WithA(0.35)))
	ax0, ax1 := x0+6, x1-6
	switch u.togEditShown {
	case TogIdleFade:
		u.text(c, "Fade when idle", ax0, t+20, 12.5, 700, txt, 0, false)
		y := t + 26
		dv := clamp01((u.IdleDelay - 1) / 14)
		y = u.compactSlider(c, SlIdleDelay, dv, "Fade after", fmt.Sprintf("%d s", int(u.IdleDelay+0.5)),
			"How long your mouse has to be away before the player fades.", ax0, ax1, y)
		ov := clamp01((u.IdleOpacity - 0.05) / 0.85)
		u.compactSlider(c, SlIdleOpacity, ov, "Opacity when faded", fmt.Sprintf("%d%%", int(u.IdleOpacity*100+0.5)),
			"How visible the player stays while it's faded. Lower = more see-through.", ax0, ax1, y)
	case TogSongPopup:
		u.text(c, "Now playing popups", ax0, t+20, 12.5, 700, txt, 0, false)
		y := t + 30
		u.text(c, "Screen", ax0, y+12, 11.5, 600, sub, 0, false)
		u.tip(c, "Which screen the popup shows on. \"Where you're working\" follows the window you're using.", ax0-6, y, ax1, y+22)
		y = u.chipRow(c, hSongMon0, u.songScreens(), ax0, ax1, y+22) + 4
		u.text(c, "Position", ax0, y+12, 11.5, 600, sub, 0, false)
		u.chipGrid(c, hSongPos0, toastPosNames[:], 3, ax0, ax1, y+22)
	}
	return h
}

// toggleGrid lays feature boxes out two across (one when narrow).
func (u *UI) toggleGrid(c *Canvas, ids []int, x0, x1, y float32) float32 {
	cols := 1
	if x1-x0 > 330 {
		cols = 2
	}
	const gap, bh = float32(8), float32(62)
	bw := (x1 - x0 + 12 - gap*float32(cols-1)) / float32(cols)
	for k, i := range ids {
		bx := x0 - 6 + float32(k%cols)*(bw+gap)
		by := y + float32(k/cols)*(bh+gap)
		u.toggleBox(c, i, bx, by, bw, bh)
	}
	rows := (len(ids) + cols - 1) / cols
	return y + float32(rows)*(bh+gap)
}

// chipRow: a row of choice chips (wraps). Returns the bottom.
func (u *UI) chipRow(c *Canvas, base int, labels []string, x0, x1, y float32) float32 {
	cx, cy := x0, y+14
	for i, lbl := range labels {
		id := base + i
		w := u.textW(lbl, 11, 600) + 24
		if cx+w > x1 && cx > x0 {
			cx, cy = x0, cy+32
		}
		hv, pr := u.h(id), u.p(id)
		s := u.selV(id)
		in := 1.2 * pr
		c.RoundRect(cx+in, cy-13+in, w-2*in, 26-2*in, 13, Solid(lerpCol(White(0.08+0.07*hv), u.accent.WithA(0.85), s)))
		c.StrokeRoundRect(cx+in, cy-13+in, w-2*in, 26-2*in, 13, 1, Solid(White(0.22)))
		col := White(0.92)
		if s > 0.5 {
			col = u.onAccent()
		}
		u.text(c, lbl, cx+w/2, cy, 11, 600, col, 1, false)
		u.addHit(c, id, cx, cy-14, cx+w, cy+14)
		cx += w + 6
	}
	return cy + 18
}

// chipGrid: chips in equal columns (screen positions read like the screen).
func (u *UI) chipGrid(c *Canvas, base int, labels []string, cols int, x0, x1, y float32) float32 {
	const gap = float32(6)
	w := (x1 - x0 - gap*float32(cols-1)) / float32(cols)
	cy := y + 14
	for i, lbl := range labels {
		id := base + i
		cx := x0 + float32(i%cols)*(w+gap)
		cy = y + 14 + float32(i/cols)*32
		hv, pr := u.h(id), u.p(id)
		s := u.selV(id)
		in := 1.2 * pr
		c.RoundRect(cx+in, cy-13+in, w-2*in, 26-2*in, 13, Solid(lerpCol(White(0.08+0.07*hv), u.accent.WithA(0.85), s)))
		c.StrokeRoundRect(cx+in, cy-13+in, w-2*in, 26-2*in, 13, 1, Solid(White(0.22)))
		col := White(0.92)
		if s > 0.5 {
			col = u.onAccent()
		}
		u.marquee(c, lbl, cx+w/2-min32(u.textW(lbl, 11, 600), w-16)/2, cy, 11, 600, col, w-16)
		u.addHit(c, id, cx, cy-14, cx+w, cy+14)
	}
	return cy + 18
}

// unfold draws something that slides open with f (0..1); returns its height.
func (u *UI) unfold(c *Canvas, f, x0, x1, y float32, full float32, draw func(top float32) float32) float32 {
	if f < 0.01 {
		return 0
	}
	ga := c.GA
	c.PushClip(x0-8, y, x1+8, y+full*f+2, 0)
	c.GA = ga * f
	h := draw(y - (1-f)*12)
	c.GA = ga
	c.PopClip()
	_ = h
	return full * f
}

func (u *UI) sliderRow(c *Canvas, sl int, v float32, title, desc, tip string, x0, x1, y float32) float32 {
	u.text(c, title, x0, y+24, 13, 600, Col{1, 1, 1, 0.94}, 0, false)
	u.text(c, fmt.Sprintf("%d%%", int(v*100+0.5)), x1, y+24, 11.5, 600, sub, 2, false)
	u.marquee(c, desc, x0, y+44, 10.5, 500, dim, x1-x0)
	u.tip(c, tip, x0-6, y+4, x1+6, y+90)
	u.slider(c, sl, v, x0+6, x1-6, y+72)
	y += 96
	u.separator(c, x0, x1, y)
	return y
}

func (u *UI) drawGeneral(c *Canvas, x0, x1, top float32) float32 {
	y := top + 10
	y = u.toggleGrid(c, []int{TogStartup, TogOnTop, TogArt, TogToasts, TogTips, TogSongPopup, TogIdleFade}, x0, x1, y)
	// a feature's own settings (its pencil)
	y += u.unfold(c, u.editA.V, x0, x1, y, u.togEditH(x0, x1), func(t float32) float32 {
		return u.togEditor(c, x0, x1, t)
	})
	// where key-press popups show (only matters when they're on)
	f := u.tog[TogToasts].V
	y += u.unfold(c, f, x0, x1, y, 108, func(t float32) float32 {
		u.text(c, "Key-press popup position", x0, t+16, 12.5, 600, Col{1, 1, 1, 0.92}, 0, false)
		u.tip(c, "Which corner (or edge) of the screen the key-press popup shows in.", x0-6, t+2, x1+6, t+30)
		return u.chipGrid(c, hToastPos0, toastPosNames[:], 3, x0, x1, t+30) - t
	})
	u.separator(c, x0, x1, y)
	// sleep timer
	u.text(c, "Sleep timer", x0, y+24, 12.5, 600, Col{1, 1, 1, 0.92}, 0, false)
	status := "Pauses Spotify after a while"
	switch {
	case u.Sleep == SleepSong:
		status = "Pausing when this song ends"
	case u.Sleep != SleepOff:
		status = fmt.Sprintf("Pausing in %d min", int(u.SleepLeft/60)+1)
	}
	u.marquee(c, status, x0, y+43, 10.5, 500, lerpCol(dim, u.accent.Mix(White(1), 0.5), b2f(u.Sleep != SleepOff)), x1-x0)
	u.tip(c, "Pauses Spotify after the time you pick, or when the song that's playing ends. It shows in the player's top bar while it runs. Off stops it.", x0-6, y+8, x1+6, y+52)
	y = u.chipRow(c, hSleep0, sleepNames[:], x0, x1, y+52) + 8
	u.separator(c, x0, x1, y)
	y = u.sliderRow(c, SlScrollRes, u.ScrollRes, "Menu scroll resistance", "How far you scroll past the end to change page",
		"In Settings, scrolling past the end of a page moves to the next one. Higher = you push further before it changes.", x0, x1, y)
	y = u.sliderRow(c, SlOpenRes, u.OpenRes, "Scroll to open Settings", "How far you scroll down on the player to open Settings",
		"Scrolling down on the player opens Settings. Higher = you scroll further before it opens.", x0, x1, y)
	// export / import
	u.text(c, "Export & import", x0, y+24, 12.5, 600, Col{1, 1, 1, 0.92}, 0, false)
	u.marquee(c, "Back up your settings or move them to another PC", x0, y+43, 10.5, 500, dim, x1-x0)
	u.tip(c, "Export saves General, Appearance and Keybinds (you pick) into the Exported Profiles folder next to Akra Player. Import loads one back.", x0-6, y+8, x1+6, y+52)
	ew := u.ghostPill(c, hExport, "Export…", x0, y+74, 0)
	u.ghostPill(c, hImport, "Import…", x0+ew+8, y+74, 0)
	y += 92
	u.separator(c, x0, x1, y)
	// reset: asks once more before it does anything
	label := "Reset to default"
	if u.ResetArmed > 0 {
		label = "Click again to reset"
		u.accentPill(c, hResetDefaults, label, (x0+x1)/2, y+30, 1)
	} else {
		u.ghostPill(c, hResetDefaults, label, (x0+x1)/2, y+30, 1)
	}
	u.tip(c, "Puts every setting back to how it came: look, sizes, toggles. Your keybinds, profiles and volume toggles stay.", x0, y+14, x1, y+46)
	u.text(c, "Keybinds and profiles are kept", (x0+x1)/2, y+56, 10.5, 500, dim, 1, false)
	y += 74
	return y - top
}

// ---------- appearance ----------

// swatchOrder: the color row (black & white share one swatch).
var swatchOrder = []int{0, 1, 2, 3, 4, AccentBlack}

// lookChips: appearance profiles (Default and Glass come built in).
func (u *UI) lookChips(c *Canvas, x0, x1, y float32) float32 {
	u.text(c, "Profiles", x0, y+12, 12, 700, txt, 0, false)
	u.marquee(c, "Colors, glass and background", x0+62, y+12, 10.5, 500, dim, x1-x0-62-90)
	u.tip(c, "Appearance profiles save everything on this page. Default and Glass come built in; + saves the current look as a new one. Changes go into the selected profile.", x0, y, x0+60, y+24)
	builtin := u.Look >= 0 && u.Look < len(u.LookBuiltin) && u.LookBuiltin[u.Look]
	if u.EditKind != EditLook {
		switch {
		case builtin && u.LookChanged:
			u.ghostPill(c, hLookRestore, "Restore", x1, y+12, 2)
			u.tip(c, "Puts this built-in profile back the way it came.", x1-80, y, x1, y+24)
		case !builtin:
			u.iconButton(c, hLookDel, x1-36, y+12, iconTrash)
			u.iconButton(c, hLookRen, x1-10, y+12, iconPencil)
		}
	}
	y += 30
	if u.EditKind == EditLook {
		u.editField(c, x0, y+14, x1-x0-70)
		u.accentPill(c, hLookRen, "Done", x1, y+14, 2)
		return y + 36
	}
	cx, cy := x0, y+14
	for i, name := range u.Looks {
		id := hLook0 + i
		lbl := u.fit(name, 11.5, 600, 140)
		w := u.textW(lbl, 11.5, 600) + 26
		if cx+w > x1 && cx > x0 {
			cx, cy = x0, cy+32
		}
		hv, pr := u.h(id), u.p(id)
		sel := u.selV(id)
		in := 1.5 * pr
		c.RoundRect(cx+in, cy-13+in, w-2*in, 26-2*in, 13, Solid(lerpCol(White(0.08+0.07*hv), u.accent.WithA(0.85), sel)))
		c.StrokeRoundRect(cx+in, cy-13+in, w-2*in, 26-2*in, 13, 1, VGrad(cy-13, cy+13, White(0.40), White(0.06)))
		col := White(0.92)
		if sel > 0.5 {
			col = u.onAccent()
		}
		u.text(c, lbl, cx+w/2, cy, 11.5, 600, col, 1, false)
		u.addHit(c, id, cx, cy-14, cx+w, cy+14)
		cx += w + 6
	}
	if cx+30 > x1 {
		cx, cy = x0, cy+32
	}
	hv := u.h(hLookAdd)
	c.Circle(cx+13, cy, 13, Solid(White(0.08+0.08*hv)))
	c.StrokeCircle(cx+13, cy, 13, 1, VGrad(cy-13, cy+13, White(0.36), White(0.06)))
	iconPlus(c, cx+13, cy, 4.6, Solid(White(0.9)))
	u.addHitC(c, hLookAdd, cx+13, cy, 15)
	u.tip(c, "Saves the current look as a new profile.", cx, cy-14, cx+26, cy+14)
	return cy + 22
}

func (u *UI) drawLook(c *Canvas, x0, x1, top float32) float32 {
	y := top
	if len(u.Looks) > 0 {
		y = u.lookChips(c, x0, x1, top+6) + 6
		u.separator(c, x0, x1, y)
	}
	u.text(c, "Accent color", x0, y+24, 12.5, 600, Col{1, 1, 1, 0.92}, 0, false)
	u.tip(c, "The color of buttons and the glow in the glass. Click black & white again to switch between them. RGB shift cycles through every color, the picture one takes its color from the album cover, and the last one is your own.", x0-6, y+6, x1+6, y+40)
	label := "Custom"
	switch {
	case u.AccentIdx == AccentRainbow:
		label = "RGB shift"
	case u.AccentIdx == AccentCover:
		label = "Album cover"
	case u.AccentIdx >= 0:
		label = AccentNames[u.AccentIdx]
	}
	u.text(c, label, x1, y+24, 11, 600, sub, 2, false)
	sy := y + 58
	n := len(swatchOrder) + 3
	step := min32((x1-x0-24)/float32(n-1), 44)
	for k := 0; k < n; k++ {
		cx := x0 + 12 + float32(k)*step
		var id int
		switch {
		case k < len(swatchOrder):
			id = hSwatch0 + swatchOrder[k]
		case k == len(swatchOrder):
			id = hSwatchRainbow
		case k == len(swatchOrder)+1:
			id = hSwatchCover
		default:
			id = hSwatchCustom
		}
		hv, pr := u.h(id), u.p(id)
		sel := u.selV(id)
		r := 11 * (1 + 0.08*hv - 0.08*pr)
		if sel > 0.01 {
			c.StrokeCircle(cx, sy, 15*(0.85+0.15*sel), 2, Solid(White(0.9*sel)))
		}
		switch {
		case id == hSwatch0+AccentBlack: // black & white: the half in use is on top-left
			first, second := Hex(0x111114, 1), Hex(0xf2f2f7, 1)
			if u.AccentIdx == AccentWhite {
				first, second = second, first
			}
			c.Circle(cx, sy, r, Solid(second))
			c.Fill(cx-r, sy-r, cx+r, sy+r, func(px, py float32) float32 {
				return max32(sdCircle(px, py, cx, sy, r), (px-cx)+(py-sy))
			}, Solid(first), 0, nil)
		case id >= hSwatch0 && id < hSwatch0+len(Accents):
			col := Accents[id-hSwatch0]
			c.Circle(cx, sy, r, Solid(col))
		case id == hSwatchCover: // the album cover (or a picture icon without one)
			if art := u.Art; art != nil {
				aw, ah := float32(art.W), float32(art.H)
				c.Fill(cx-r, sy-r, cx+r, sy+r, func(px, py float32) float32 { return sdCircle(px, py, cx, sy, r) },
					func(px, py float32) Col {
						col := art.Sample((px-cx+r)/(2*r)*aw, (py-sy+r)/(2*r)*ah)
						col.A = 1
						return col
					}, 0, nil)
			} else {
				c.Circle(cx, sy, r, VGrad(sy-r, sy+r, Hex(0x5e5ce6, 1), Hex(0xbf5af2, 1)))
				iconPicture(c, cx, sy, Solid(White(0.95)))
			}
		case id == hSwatchRainbow:
			spin := u.T * 0.25
			c.Fill(cx-r, sy-r, cx+r, sy+r, func(px, py float32) float32 { return sdCircle(px, py, cx, sy, r) },
				func(px, py float32) Col {
					a := float32(math.Atan2(float64(py-sy), float64(px-cx))/(2*math.Pi)) + 0.5 + spin
					return HSV(a, 0.62, 1)
				}, 0, nil)
		default: // custom: shows your custom color with a small rainbow ring
			u.customSwatch(c, cx, sy, r, u.Custom)
		}
		c.StrokeCircle(cx, sy, r, 1, Solid(White(0.35)))
		u.addHitC(c, id, cx, sy, 14)
	}
	y += 88
	u.separator(c, x0, x1, y)

	u.text(c, "Glass clarity", x0, y+24, 12.5, 600, Col{1, 1, 1, 0.92}, 0, false)
	u.tip(c, "How see-through the glass is. Lower is more frosted and easier to read; higher shows more of what's behind.", x0-6, y+6, x1+6, y+70)
	u.text(c, fmt.Sprintf("%d%%", int(u.Clarity*100+0.5)), x1, y+24, 11.5, 600, sub, 2, false)
	u.slider(c, SlClarity, u.Clarity, x0+6, x1-6, y+54)
	y += 78
	u.separator(c, x0, x1, y)

	// Background: blur, outline, fluid and visualizer, folded together
	{
		hv := u.h(hBgDrop)
		if hv > 0.01 {
			c.RoundRect(x0-6, y+6, x1-x0+12, 36, 12, Solid(White(0.06*hv)))
		}
		iconChevron(c, x1-12, y+24, 1-u.bgA.V, Solid(White(0.85)))
		u.text(c, "Background", x0, y+24, 13, 600, Col{1, 1, 1, 0.94}, 0, false)
		var on []string
		for _, i := range []int{TogBlur, TogOutline, TogFluid, TogVis} {
			if u.Toggles[i] {
				on = append(on, strings.ToLower(strings.TrimPrefix(togLabels[i][0], "Background ")))
			}
		}
		summary := "Nothing on"
		if len(on) > 0 {
			summary = strings.Join(on, ", ")
			summary = strings.ToUpper(summary[:1]) + summary[1:]
		}
		u.marquee(c, summary, x0+92, y+24, 10.5, 500, dim, x1-x0-120)
		u.addHit(c, hBgDrop, x0-6, y+6, x1+6, y+42)
		u.tip(c, "Blur, outline, fluid and the visualizer: everything about what's behind and around the glass.", x0-6, y+6, x1+6, y+42)
		y += 48
		f := u.bgA.V
		if f > 0.01 {
			// measure the open height by drawing it (clipped while it opens)
			inner := u.bgInnerH(x0, x1)
			ga := c.GA
			c.PushClip(x0-10, y, x1+10, y+inner*f+2, 0)
			c.GA = ga * f
			u.bgInner(c, x0, x1, y-(1-f)*14)
			c.GA = ga
			c.PopClip()
			y += inner * f
		}
		u.separator(c, x0, x1, y)
	}

	y = u.toggleGrid(c, []int{TogGlow, TogAnchor, TogTrayAccent}, x0, x1, y+10)
	u.separator(c, x0, x1, y)

	// how switched-on features look
	u.text(c, "Switched-on look", x0, y+24, 12.5, 600, Col{1, 1, 1, 0.92}, 0, false)
	u.tip(c, "How a feature box shows it's on: a green glow along its edges, the whole box frosted green, or a classic switch.", x0-6, y+6, x1+6, y+40)
	y = u.chipRow(c, hTogStyle0, []string{"Edge glow", "Frosted fill", "Classic"}, x0, x1, y+42)
	y += 8
	u.separator(c, x0, x1, y)

	u.text(c, "Font", x0, y+24, 12.5, 600, Col{1, 1, 1, 0.92}, 0, false)
	u.tip(c, "The typeface used everywhere in the player.", x0-6, y+6, x1+6, y+40)
	bw := (x1 - x0 - 12) / NumFonts
	for i := 0; i < NumFonts; i++ {
		id := hFont0 + i
		bx := x0 + float32(i)*(bw+6)
		by := y + 42
		hv, pr := u.h(id), u.p(id)
		sel := u.selV(id)
		bh := float32(58)
		inset := 2 * pr
		c.RoundRect(bx+inset, by+inset, bw-2*inset, bh-2*inset, 14, Solid(lerpCol(White(0.07+0.06*hv), u.accent.WithA(0.30), sel)))
		c.StrokeRoundRect(bx+inset, by+inset, bw-2*inset, bh-2*inset, 14, 1+0.4*sel,
			Solid(lerpCol(White(0.22), u.accent.Mix(White(1), 0.45), sel)))
		u.textF(c, "Aa", bx+bw/2, by+22, 19, 700, i, txt, 1, false)
		u.textF(c, FontNames[i], bx+bw/2, by+44, 10.5, 600, i, sub, 1, false)
		u.addHit(c, id, bx, by, bx+bw, by+bh)
	}
	y += 112
	u.text(c, "Tip: drag the glass to move it, or a corner to resize it", (x0+x1)/2, y+8, 10.5, 500, dim, 1, false)
	return y + 24 - top
}

// bgInnerH: height of the open Background group.
func (u *UI) bgInnerH(x0, x1 float32) float32 {
	cols := 1
	if x1-x0 > 330 {
		cols = 2
	}
	h := float32((4+cols-1)/cols)*70 + 6
	h += 58 * u.tog[TogFluid].V
	h += u.visStylesH(x0, x1) * u.tog[TogVis].V
	return h
}

func (u *UI) bgInner(c *Canvas, x0, x1, y float32) {
	y = u.toggleGrid(c, []int{TogBlur, TogOutline, TogFluid, TogVis}, x0, x1, y+2)
	// fluid color: follows the theme unless you pick one (or use the cover)
	if f := u.tog[TogFluid].V; f > 0.01 {
		ga := c.GA
		c.GA = ga * f
		cy := y + 26
		u.text(c, "Fluid color", x0, cy-8, 12.5, 600, Col{1, 1, 1, 0.92}, 0, false)
		u.tip(c, "Where the fluid takes its colors from: your theme, a color you pick, or the album cover.", x0-6, cy-20, x1-120, cy+20)
		mode := [...]string{"Matches the theme", "Your color", "From the album cover"}[u.FluidMode]
		u.text(c, mode, x0, cy+10, 10.5, 500, dim, 0, false)
		hv := u.h(hFluidPick)
		lw := u.textW("Change", 11, 600)
		w := 14 + 3*14 + 2*5 + 10 + lw + 14
		bx := x1 - w
		c.RoundRect(bx, cy-14, w, 28, 14, Solid(White(0.08+0.08*hv)))
		c.StrokeRoundRect(bx, cy-14, w, 28, 14, 1, Solid(White(0.22)))
		cols := u.fluidColors()
		for k := 0; k < 3; k++ {
			dx := bx + 14 + 7 + float32(k)*19
			c.Circle(dx, cy, 7, Solid(cols[k]))
			c.StrokeCircle(dx, cy, 7, 1, Solid(White(0.5)))
		}
		u.text(c, "Change", bx+w-14, cy, 11, 600, White(0.92), 2, false)
		u.addHit(c, hFluidPick, bx, cy-15, x1, cy+15)
		c.GA = ga
		y += 58 * f
	}
	if f := u.tog[TogVis].V; f > 0.01 {
		ga := c.GA
		c.GA = ga * f
		u.visStyles(c, x0, x1, y)
		c.GA = ga
	}
}

func (u *UI) customSwatch(c *Canvas, cx, sy, r float32, col Col) {
	c.Fill(cx-r, sy-r, cx+r, sy+r, func(px, py float32) float32 { return abs32(sdCircle(px, py, cx, sy, r-1.5)) - 1.5 },
		func(px, py float32) Col {
			a := float32(math.Atan2(float64(py-sy), float64(px-cx))/(2*math.Pi)) + 0.5
			return HSV(a, 0.7, 1)
		}, 0, nil)
	c.Circle(cx, sy, r-3.2, Solid(col))
	if col.Lum() > 0.6 {
		iconPlus(c, cx, sy, 3.2, Solid(Black(0.75)))
	} else {
		iconPlus(c, cx, sy, 3.2, Solid(White(1)))
	}
}

// ---------- picker overlay ----------

func (u *UI) drawPicker(c *Canvas) {
	a := clamp01(u.pick.X)
	if a < 0.01 {
		return
	}
	PW, PH := u.PW, u.PH
	rad := u.Radius()
	c.Fill(0, 0, PW, PH, func(x, y float32) float32 { return sdRoundRect(x, y, 0, 0, PW, PH, rad) },
		Solid(Black(0.45*a)), 0, nil)
	if a > 0.5 {
		u.hits = append(u.hits, hitArea{hPickBg, 0, 0, PW, PH, false})
	}
	n := len(u.PickItems)
	rowH := float32(44)
	if u.PickMode == PickFolder {
		rowH = 38
	}
	rows := float32(min(max(n, 1), 4))
	bw := float32(300)
	bh := 62 + rows*rowH + 12
	bx := (PW - bw) / 2
	by := (PH-bh)/2 + (1-a)*26
	c.GA = a
	u.themedPanel(c, bx, by, bw, bh, 22)
	if a > 0.5 {
		u.hits = append(u.hits, hitArea{hPickBox, bx, by, bx + bw, by + bh, false})
	}
	var title, hint string
	switch u.PickMode {
	case PickWindow:
		title, hint = "Choose a window", "Apps playing sound are marked"
	case PickApp:
		switch {
		case u.PickSlot == -1:
			title, hint = "Music to lower", "Lowered while you hold the key"
		case u.PickSlot >= len(u.Presets):
			title, hint = "Add a Volume Toggle", "Apps with sound open right now"
		default:
			title, hint = "App for this Volume Toggle", "Apps with sound open right now"
		}
	case PickFolder:
		title, hint = "Move to folder", u.ActLabel(u.PickSlot)
	}
	u.text(c, title, bx+18, by+24, 14, 700, txt, 0, false)
	u.text(c, u.fit(hint, 11, 500, bw-36), bx+18, by+44, 11, 500, dim, 0, false)
	if n == 0 {
		u.text(c, "Nothing found. Open the app, then try again.", PW/2, by+84, 11, 500, sub, 1, false)
	}

	// scrolling list
	lx0, ly0 := bx+8, by+58
	lh := rows * rowH
	sc := &u.pickScroll
	sc.Max = max32(0, float32(n)*rowH-lh)
	c.PushClip(lx0, ly0, bx+bw-8, ly0+lh, 0)
	for i := 0; i < n; i++ {
		it := u.PickItems[i]
		ry := ly0 + float32(i)*rowH + rowH/2 - sc.Pos.X
		if ry < ly0-rowH || ry > ly0+lh+rowH {
			continue
		}
		id := hPick0 + i
		if hv := max32(u.h(id), u.p(id)); hv > 0.01 {
			c.RoundRect(bx+8, ry-rowH/2+2, bw-16, rowH-4, 12, Solid(u.accent.Mix(White(1), 0.3).WithA(0.22*hv)))
			c.StrokeRoundRect(bx+8, ry-rowH/2+2, bw-16, rowH-4, 12, 1, Solid(u.accent.Mix(White(1), 0.5).WithA(0.35*hv)))
		}
		if u.PickMode == PickFolder {
			iconFolder(c, bx+30, ry, Solid(u.accent.Mix(White(1), 0.45)))
			u.text(c, u.fit(it.Name, 13, 600, bw-110), bx+50, ry, 13, 600, txt, 0, false)
		} else {
			u.appIcon(c, it.IconKey, it.Sub, bx+30, ry, 24, it.Active)
			u.text(c, u.fit(it.Name, 12.5, 600, bw-120), bx+52, ry-7, 12.5, 600, txt, 0, false)
			u.text(c, u.fit(it.Sub, 10.5, 500, bw-120), bx+52, ry+9, 10.5, 500, dim, 0, false)
		}
		if it.Selected {
			cx := bx + bw - 30
			c.Circle(cx, ry, 9, Solid(u.accent))
			ck := Solid(u.onAccent())
			c.Line(cx-4, ry, cx-1, ry+3, 1.8, ck)
			c.Line(cx-1, ry+3, cx+4.5, ry-3, 1.8, ck)
		} else if it.Active {
			u.text(c, "playing", bx+bw-22, ry, 10.5, 600, u.accent.Mix(White(1), 0.45), 2, false)
		}
		if a > 0.5 && ry > ly0 && ry < ly0+lh {
			u.hits = append(u.hits, hitArea{id, bx + 8, ry - rowH/2, bx + bw - 8, ry + rowH/2, false})
		}
	}
	c.PopClip()
	u.scrollbar(c, sc, bx+bw-7, ly0+4, lh-8, 0)
	c.GA = 1
}

// ---------- toast ----------

func (u *UI) drawToast(c *Canvas) {
	a := u.toastA.V
	if a < 0.01 || u.shownTst == "" {
		return
	}
	s := u.fit(u.shownTst, 12, 600, u.PW-80)
	w := u.textW(s, 12, 600) + 34
	h := float32(32)
	x := u.PW/2 - w/2
	y := u.PH - 58 + (1-a)*12
	c.GA = a
	c.Fill(x, y, x+w, y+h, func(px, py float32) float32 { return sdRoundRect(px, py-5, x+3, y+3, w-6, h-6, h/2) },
		Solid(Black(0.45)), 10, nil)
	c.RoundRect(x, y, w, h, h/2, VGrad(y, y+h, Col{0.26, 0.27, 0.30, 0.95}, Col{0.16, 0.17, 0.19, 0.95}))
	c.StrokeRoundRect(x, y, w, h, h/2, 1, VGrad(y, y+h, White(0.45), White(0.08)))
	u.text(c, s, u.PW/2, y+h/2, 12, 600, White(0.96), 1, false)
	c.GA = 1
}

// ---------- custom color picker (glass) ----------

const (
	hueSVW, hueSVH = 240, 112
	hueBoxW        = 276
)

// hueBoxH: the fluid picker has two extra buttons.
func (u *UI) hueBoxH() float32 {
	if u.HueFor == HueFluid {
		return 312
	}
	return 236
}

func (u *UI) hueGeom() (bx, by float32) {
	a := clamp01(u.hue.X)
	return (u.PW - hueBoxW) / 2, (u.PH-u.hueBoxH())/2 + (1-a)*26
}

func (u *UI) hueColor() Col { return HSV(u.HueH, u.HueS, u.HueV) }

// HueAt maps a point to a new color while dragging in the picker.
func (u *UI) HueAt(id int, x, y float32) {
	bx, by := u.hueGeom()
	sx, sy := bx+18, by+50
	switch id {
	case hHueSV:
		u.HueS = clamp01((x - sx) / hueSVW)
		u.HueV = clamp01(1 - (y-sy)/hueSVH)
	case hHueBar:
		u.HueH = clamp01((x - sx) / hueSVW)
	}
	if u.HueFor == HueFluid {
		u.FluidCol = u.hueColor()
		u.FluidMode = FluidCustom // live preview
	} else if u.HueFor == HueWave {
		u.WaveCol = u.hueColor()
	} else {
		u.Custom = u.hueColor()
	}
}

// OpenHue starts the picker for the accent (HueAccent) or the fluid.
func (u *UI) OpenHue(forWhat int) {
	u.HueFor = forWhat
	col := u.Custom
	if forWhat == HueFluid {
		col = u.FluidCol
		if u.FluidMode == FluidTheme {
			col = u.accent
		}
		u.hueMode = u.FluidMode
	}
	if forWhat == HueWave {
		col = u.WaveCol
	}
	h, s, v := toHSV(col)
	u.HueH, u.HueS, u.HueV = h, s, v
	u.hueBefore = col
	u.HueOpen = true
}

func (u *UI) CancelHue() {
	if u.HueFor == HueFluid {
		u.FluidCol = u.hueBefore
		u.FluidMode = u.hueMode
	} else if u.HueFor == HueWave {
		u.WaveCol = u.hueBefore
	} else {
		u.Custom = u.hueBefore
	}
	u.HueOpen = false
}

func toHSV(c Col) (float32, float32, float32) {
	mx := max32(c.R, max32(c.G, c.B))
	mn := min32(c.R, min32(c.G, c.B))
	d := mx - mn
	var h float32
	switch {
	case d == 0:
		h = 0
	case mx == c.R:
		h = (c.G - c.B) / d / 6
	case mx == c.G:
		h = ((c.B-c.R)/d + 2) / 6
	default:
		h = ((c.R-c.G)/d + 4) / 6
	}
	if h < 0 {
		h++
	}
	s := float32(0)
	if mx > 0 {
		s = d / mx
	}
	return h, s, mx
}

func (u *UI) drawHuePicker(c *Canvas) {
	a := clamp01(u.hue.X)
	if a < 0.01 {
		return
	}
	PW, PH := u.PW, u.PH
	rad := u.Radius()
	c.Fill(0, 0, PW, PH, func(x, y float32) float32 { return sdRoundRect(x, y, 0, 0, PW, PH, rad) },
		Solid(Black(0.45*a)), 0, nil)
	bx, by := u.hueGeom()
	bh := u.hueBoxH()
	if a > 0.5 {
		u.hits = append(u.hits, hitArea{hHueBg, 0, 0, PW, PH, false})
		u.hits = append(u.hits, hitArea{hHueBox, bx, by, bx + hueBoxW, by + bh, false})
	}
	c.GA = a
	u.card(c, bx, by, hueBoxW, bh, 22)
	c.RoundRect(bx+1, by+1, hueBoxW-2, bh-2, 21, VGrad(by, by+bh, Col{0.20, 0.21, 0.24, 0.88}, Col{0.12, 0.13, 0.15, 0.92}))
	c.StrokeRoundRect(bx, by, hueBoxW, bh, 22, 1.1, VGrad(by, by+bh, White(0.5), White(0.1), White(0.2)))
	title := "Custom color"
	if u.HueFor == HueFluid {
		title = "Fluid color"
	} else if u.HueFor == HueWave {
		title = "Waveform color"
	}
	u.text(c, title, bx+18, by+26, 14, 700, txt, 0, false)
	col := u.hueColor()
	c.Circle(bx+hueBoxW-30, by+26, 11, Solid(col))
	c.StrokeCircle(bx+hueBoxW-30, by+26, 11, 1.2, Solid(White(0.7)))

	// saturation / brightness square
	sx, sy := bx+18, by+50
	hue := u.HueH
	c.Fill(sx, sy, sx+hueSVW, sy+hueSVH, func(x, y float32) float32 { return sdRoundRect(x, y, sx, sy, hueSVW, hueSVH, 12) },
		func(x, y float32) Col {
			return HSV(hue, clamp01((x-sx)/hueSVW), clamp01(1-(y-sy)/hueSVH))
		}, 0, nil)
	c.StrokeRoundRect(sx, sy, hueSVW, hueSVH, 12, 1, Solid(White(0.25)))
	kx, ky := sx+u.HueS*hueSVW, sy+(1-u.HueV)*hueSVH
	c.Fill(kx-12, ky-12, kx+12, ky+12, func(x, y float32) float32 { return sdCircle(x, y-1.5, kx, ky, 9) }, Solid(Black(0.4)), 4, nil)
	c.Circle(kx, ky, 8, Solid(col))
	c.StrokeCircle(kx, ky, 9, 2.4, Solid(White(1)))
	if a > 0.5 {
		u.hits = append(u.hits, hitArea{hHueSV, sx - 6, sy - 6, sx + hueSVW + 6, sy + hueSVH + 6, false})
	}

	// hue bar
	hy := sy + hueSVH + 22
	c.Fill(sx, hy-7, sx+hueSVW, hy+7, func(x, y float32) float32 { return sdRoundRect(x, y, sx, hy-7, hueSVW, 14, 7) },
		func(x, y float32) Col { return HSV(clamp01((x-sx)/hueSVW), 1, 1) }, 0, nil)
	c.StrokeRoundRect(sx, hy-7, hueSVW, 14, 7, 1, Solid(White(0.25)))
	hx := sx + u.HueH*hueSVW
	c.Fill(hx-12, hy-12, hx+12, hy+12, func(x, y float32) float32 { return sdCircle(x, y-1.5, hx, hy, 9) }, Solid(Black(0.4)), 4, nil)
	c.Circle(hx, hy, 8.5, Solid(HSV(u.HueH, 1, 1)))
	c.StrokeCircle(hx, hy, 9, 2.4, Solid(White(1)))
	if a > 0.5 {
		u.hits = append(u.hits, hitArea{hHueBar, sx - 6, hy - 14, sx + hueSVW + 6, hy + 14, false})
	}

	if u.HueFor == HueFluid {
		// "Match with theme" closes the picker; album colors sit below it
		u.wideButton(c, hHueTheme, "Match with theme", sx, hy+32, hueSVW, false)
		art := u.FluidMode == FluidArt
		u.wideButton(c, hHueArt, "Use album cover colors", sx, hy+66, hueSVW, art)
	}

	// buttons
	byb := by + bh - 26
	r8 := func(v float32) int { return int(v*255 + 0.5) }
	u.text(c, fmt.Sprintf("#%02X%02X%02X", r8(col.R), r8(col.G), r8(col.B)), sx, byb, 12, 600, sub, 0, false)
	u.accentPill(c, hHueDone, "Use color", bx+hueBoxW-18, byb, 2)
	u.ghostPill(c, hHueCancel, "Cancel", bx+hueBoxW-18-u.textW("Use color", 11.5, 600)-30, byb, 2)
	c.GA = 1
}

// wideButton: a full-width glass button (selected = accent tint + check).
func (u *UI) wideButton(c *Canvas, id int, label string, x, cy, w float32, sel bool) {
	hv, pr := u.h(id), u.p(id)
	h := float32(28)
	in := 1.5 * pr
	bg := White(0.08 + 0.08*hv)
	if sel {
		bg = u.accent.WithA(0.32 + 0.1*hv)
	}
	c.RoundRect(x+in, cy-h/2+in, w-2*in, h-2*in, 10, Solid(bg))
	c.StrokeRoundRect(x+in, cy-h/2+in, w-2*in, h-2*in, 10, 1, VGrad(cy-h/2, cy+h/2, White(0.38), White(0.06)))
	u.text(c, label, x+w/2, cy, 11.5, 600, White(0.95), 1, false)
	if sel {
		ck := Solid(White(0.95))
		c.Line(x+14, cy, x+17, cy+3, 1.8, ck)
		c.Line(x+17, cy+3, x+22.5, cy-3, 1.8, ck)
	}
	u.addHit(c, id, x, cy-h/2, x+w, cy+h/2)
}

// segmented: a two-option switch like [Show | Hide]. Returns its width.
func (u *UI) segmented(c *Canvas, ids [2]int, labels [2]string, sel int, x1, cy float32) {
	w0, w1 := u.textW(labels[0], 11.5, 600)+26, u.textW(labels[1], 11.5, 600)+26
	w := w0 + w1 + 6
	x := x1 - w
	h := float32(30)
	c.RoundRect(x, cy-h/2, w, h, h/2, Solid(Black(0.18)))
	c.StrokeRoundRect(x, cy-h/2, w, h, h/2, 1, VGrad(cy-h/2, cy+h/2, White(0.10), White(0.28)))
	// the knob slides between the two
	t := u.selV(ids[0])
	kx := lerp(x+3+w0+0, x+3, t)
	kw := lerp(w1, w0, t)
	c.RoundRect(kx, cy-h/2+3, kw, h-6, (h-6)/2, VGrad(cy-h/2, cy+h/2, u.accent.Mix(White(1), 0.2), u.accent))
	for i, lbl := range labels {
		bx, bw := x+3, w0
		if i == 1 {
			bx, bw = x+3+w0, w1
		}
		col := White(0.75)
		if i == sel {
			col = u.onAccent()
		}
		u.text(c, lbl, bx+bw/2, cy, 11.5, 600, col, 1, false)
		u.addHit(c, ids[i], bx, cy-h/2, bx+bw, cy+h/2)
	}
}

// visStylesH: height of the visualizer style picker.
func (u *UI) visStylesH(x0, x1 float32) float32 {
	h := float32(visRows(x0, x1)*62 + 14)
	if e := u.edgeA(); e > 0.01 {
		rows := float32(1)
		if len(u.Monitors) > 1 {
			var tw float32
			for _, m := range u.Monitors {
				tw += u.textW(m, 11, 600) + 30
			}
			rows = float32(math.Ceil(float64(tw / max32(80, x1-x0-64))))
		}
		h += e * (26 + 32*rows)
	}
	return h + u.visOptionsH(x0, x1)
}

// chipRowsH: how tall a chipRow of these labels is.
func (u *UI) chipRowsH(labels []string, x0, x1 float32) float32 {
	cx, rows := x0, float32(1)
	for _, l := range labels {
		w := u.textW(l, 11, 600) + 24
		if cx+w > x1 && cx > x0 {
			cx, rows = x0, rows+1
		}
		cx += w + 6
	}
	return rows * 32
}

func (u *UI) waveColorsShown() bool {
	return u.VisStyle == VisWave || (u.VisStyle == VisEdge && u.EdgeStyle == EdgeWave)
}

// visOptionsH: Sensitivity, Smoothness, the style's own sliders and options.
func (u *UI) visOptionsH(x0, x1 float32) float32 {
	h := float32(8 + 2*62)
	for _, n := range visParamNames[u.VisStyle] {
		if n != "" {
			h += 62
		}
	}
	if u.VisStyle == VisEdge {
		h += 30 + 32
	}
	if u.waveColorsShown() {
		h += 30 + u.chipRowsH(waveModeNames[:], x0, x1-96)
	}
	return h
}

// compactSlider: a title, its value on the right, and the slider under it.
func (u *UI) compactSlider(c *Canvas, sl int, v float32, title, value, tip string, x0, x1, y float32) float32 {
	u.text(c, title, x0, y+18, 12, 600, Col{1, 1, 1, 0.92}, 0, false)
	u.text(c, value, x1, y+18, 11, 600, sub, 2, false)
	u.tip(c, tip, x0-6, y+4, x1+6, y+58)
	u.slider(c, sl, v, x0+6, x1-6, y+42)
	return y + 62
}

var visParamTips = [NumVisStyles][2]string{
	VisBars:    {"How tall the bars can get.", "How wide each bar is."},
	VisHalo:    {"How big the ring is.", "How far the rays reach on loud parts."},
	VisEdge:    {"How tall it gets along the bottom of the screen.", ""},
	VisThreads: {"How many strands there are.", "How far apart the strands drift."},
	VisGlow:    {"How wide and bright the glow around the line is.", "How far the line swings."},
	VisMesh:    {"How big the blob is.", "How much its shape wobbles."},
	VisRipples: {"How fast the rings spread out.", "How many rings there are."},
	VisWave:    {"How tall the waveform gets.", "How bright the inside of the shape is (the outline stays)."},
	VisSparks:  {"How many embers there are.", "How big and soft each ember glows."},
}

func (u *UI) visOptions(c *Canvas, x0, x1, y float32) {
	y += 8
	u.separator(c, x0, x1, y)
	y = u.compactSlider(c, SlVisSens, u.VisSens, "Sensitivity", fmt.Sprintf("%d%%", int(u.VisSens*100+0.5)),
		"How strongly it reacts. Higher makes quiet songs move more; lower calms loud ones down.", x0, x1, y)
	y = u.compactSlider(c, SlVisSmooth, u.VisSmooth, "Smoothness", fmt.Sprintf("%d%%", int(u.VisSmooth*100+0.5)),
		"Higher = calmer, flowing movement. Lower = snappier, follows every beat.", x0, x1, y)
	st := u.VisStyle
	for k, n := range visParamNames[st] {
		if n == "" {
			continue
		}
		v := u.VisP[st][k]
		y = u.compactSlider(c, SlVisP0+k, v, visNames[st]+": "+strings.ToLower(n), fmt.Sprintf("%d%%", int(v*100+0.5)),
			visParamTips[st][k]+" Middle is the original look.", x0, x1, y)
	}
	if st == VisEdge {
		u.text(c, "Look", x0, y+18, 12, 600, Col{1, 1, 1, 0.92}, 0, false)
		u.tip(c, "Bars along the bottom of the screen, or a waveform rising from it (its colors are below).", x0-6, y+4, x1+6, y+30)
		y = u.chipRow(c, hEdgeStyle0, []string{"Bars", "Waveform"}, x0, x1, y+30) - 4
	}
	if u.waveColorsShown() {
		u.text(c, "Waveform colors", x0, y+18, 12, 600, Col{1, 1, 1, 0.92}, 0, false)
		u.tip(c, "Theme: your color. Custom: one color you pick. RGB: every color, shifting. Gradient: your pick in the middle fading into your theme color. Liquid: flowing colors like the fluid background.", x0-6, y+4, x1+6, y+30)
		u.chipRow(c, hWaveMode0, waveModeNames[:], x0, x1-96, y+30)
		if u.WaveMode == WaveCustom || u.WaveMode == WaveGradient {
			hv := u.h(hWavePick)
			bx, cy := x1-88, y+44
			c.RoundRect(bx, cy-13, 88, 26, 13, Solid(White(0.08+0.08*hv)))
			c.StrokeRoundRect(bx, cy-13, 88, 26, 13, 1, Solid(White(0.22)))
			c.Circle(bx+16, cy, 7, Solid(u.WaveCol))
			c.StrokeCircle(bx+16, cy, 7, 1, Solid(White(0.5)))
			u.text(c, "Change", bx+78, cy, 11, 600, White(0.92), 2, false)
			u.addHit(c, hWavePick, bx, cy-14, x1, cy+14)
		}
	}
}

// visStyles: the visualizer's style, and for Screen Edge which screen.
// visCols: three styles across when there's room, else two.
func visCols(x0, x1 float32) int {
	if x1-x0 > 360 {
		return 3
	}
	return 2
}

func visRows(x0, x1 float32) int {
	n := visCols(x0, x1)
	return (NumVisStyles + n - 1) / n
}

func (u *UI) visStyles(c *Canvas, x0, x1, top float32) {
	cols := visCols(x0, x1)
	gap := float32(6)
	bw := (x1 - x0 - gap*float32(cols-1)) / float32(cols)
	for i := 0; i < NumVisStyles; i++ {
		id := hVisStyle0 + i
		bx := x0 + float32(i%cols)*(bw+gap)
		by := top + 4 + float32(i/cols)*62
		hv, pr := u.h(id), u.p(id)
		s := u.selV(id)
		in := 2 * pr
		c.RoundRect(bx+in, by+in, bw-2*in, 56-2*in, 14, Solid(lerpCol(White(0.07+0.06*hv), u.accent.WithA(0.30), s)))
		c.StrokeRoundRect(bx+in, by+in, bw-2*in, 56-2*in, 14, 1+0.4*s, Solid(lerpCol(White(0.22), u.accent.Mix(White(1), 0.45), s)))
		visIcon(c, i, bx+bw/2, by+21, lerpCol(White(0.75), White(1), s))
		u.text(c, u.fit(visNames[i], 10.5, 600, bw-8), bx+bw/2, by+43, 10.5, 600, lerpCol(sub, txt, s), 1, false)
		u.addHit(c, id, bx, by, bx+bw, by+56)
		u.tip(c, visTips[i], bx, by, bx+bw, by+56)
	}
	if edge := u.edgeA(); edge > 0.01 { // which screen the Screen Edge visualizer runs along
		ey := top + float32(visRows(x0, x1)*62) + 10
		ga := c.GA
		c.GA = ga * edge
		u.text(c, "Show on", x0, ey+16, 12, 600, Col{1, 1, 1, 0.9}, 0, false)
		if len(u.Monitors) == 0 {
			u.text(c, "Main screen", x0+64, ey+16, 11, 600, sub, 0, false)
		} else {
			u.chipRow(c, hVisMon0, u.Monitors, x0+64, x1, ey+2)
		}
		c.GA = ga
	}
	oy := top + u.visStylesH(x0, x1) - u.visOptionsH(x0, x1)
	u.visOptions(c, x0, x1, oy)
}

// edgeA: how open the screen picker is (only for Screen Edge).
func (u *UI) edgeA() float32 { return u.selV(hVisStyle0 + VisEdge) }

// visIcon: tiny pictures of each visualizer style.
func visIcon(c *Canvas, style int, cx, cy float32, col Col) {
	p := Solid(col)
	switch style {
	case VisBars:
		for i, h := range []float32{6, 11, 15, 9, 13, 7} {
			x := cx - 12.5 + float32(i)*5
			c.RoundRect(x-1.4, cy+7-h, 2.8, h, 1.4, p)
		}
	case VisHalo:
		c.StrokeCircle(cx, cy, 6.5, 1.3, p)
		for i := 0; i < 12; i++ {
			a := float64(i) / 12 * 2 * math.Pi
			l := float32(2.5 + 2.5*math.Abs(math.Sin(a*2.5)))
			sx, sy := float32(math.Sin(a)), -float32(math.Cos(a))
			c.Line(cx+sx*8.5, cy+sy*8.5, cx+sx*(8.5+l), cy+sy*(8.5+l), 1.1, p)
		}
	case VisEdge:
		c.StrokeRoundRect(cx-13, cy-9, 26, 18, 3, 1.3, p)
		for i, h := range []float32{2, 4, 6, 4, 2} {
			x := cx - 8 + float32(i)*4
			c.RoundRect(x-1, cy+7-h, 2, h, 1, p)
		}
	case VisThreads:
		for i := 0; i < 6; i++ {
			o := float32(i) - 2.5
			var px, py float32
			for k := 0; k <= 12; k++ {
				x := cx - 13 + float32(k)*26/12
				y := cy + o*0.9 + 5*sinf(float32(k)*0.55+o*0.25)
				if k > 0 {
					c.Line(px, py, x, y, 0.6, Solid(col.WithA(0.7)))
				}
				px, py = x, y
			}
		}
	case VisGlow:
		var px, py float32
		for k := 0; k <= 16; k++ {
			x := cx - 13 + float32(k)*26/16
			y := cy + 6*sinf(float32(k)*0.45-0.6)*(0.6+0.4*sinf(float32(k)*0.2))
			if k > 0 {
				c.Line(px, py, x, y, 3.5, Solid(col.WithA(0.18)))
				c.Line(px, py, x, y, 1.2, p)
			}
			px, py = x, y
		}
	case VisRipples:
		for i, r := range []float32{3, 6.5, 10} {
			c.StrokeCircle(cx, cy, r, 1.2, Solid(col.WithA(1-float32(i)*0.28)))
		}
	case VisWave:
		for i := 0; i <= 12; i++ {
			x := cx - 13 + float32(i)*26/12
			d := abs32(float32(i)-6) / 6
			h := (1.5 + 7*float32(math.Abs(math.Sin(float64(i)*1.3)))) * (1 - d*d*0.8)
			c.Line(x, cy-h, x, cy+h, 1.2, p)
		}
	case VisSparks:
		for i, d := range [][3]float32{{-9, 6, 1.4}, {-4, -2, 1.1}, {2, 4, 1.6}, {7, -6, 1.2}, {10, 3, 0.9}, {-1, -8, 1}} {
			c.Line(cx+d[0], cy+d[1], cx+d[0]-0.4, cy+d[1]+3+float32(i%3), 0.9, Solid(col.WithA(0.5)))
			c.Circle(cx+d[0], cy+d[1], d[2], p)
		}
	case VisMesh:
		for r := 0; r < 5; r++ {
			ry := cy - 7 + float32(r)*3.5
			w := 10 * float32(math.Sin(float64(r+1)/6*math.Pi))
			for k := 0; k < 10; k++ {
				a := float64(k) / 10 * 2 * math.Pi
				c.Circle(cx+w*float32(math.Cos(a)), ry+1.2*float32(math.Sin(a)), 0.7, p)
			}
		}
	}
}

// themedPanel: a glass panel tinted with the current color (menus, bubbles).
func (u *UI) themedPanel(c *Canvas, x, y, w, h, r float32) {
	u.card(c, x, y, w, h, r)
	ac := u.accent
	if ac.Lum() > 0.8 { // black & white theme: neutral gray glass
		ac = Col{0.5, 0.52, 0.56, 1}
	}
	c.RoundRect(x+1, y+1, w-2, h-2, r-1, VGrad(y, y+h, ac.Mix(Black(1), 0.62).WithA(0.90), ac.Mix(Black(1), 0.80).WithA(0.94)))
	c.Fill(x, y, x+w, y+h, func(px, py float32) float32 { return sdRoundRect(px, py, x+1, y+1, w-2, h-2, r-1) },
		Radial(x+w*0.2, y-20, max32(w, h)*0.9, ac.Mix(White(1), 0.4).WithA(0.18)), 0, nil)
	c.StrokeRoundRect(x, y, w, h, r, 1.1, VGrad(y, y+h, ac.Mix(White(1), 0.6).WithA(0.6), White(0.1), White(0.2)))
}

var togTips = [NumToggles]string{
	"Starts Akra Player (hidden in the tray) when you sign in to Windows, so your keys always work.",
	"Keeps the player above other windows.",
	"Shows the song's cover next to the title, with a glow in its colors. The picture button on the player does the same.",
	"When a hotkey does something, a small note shows in the top-right corner of the screen (over games in borderless mode too).",
	"Frosts whatever is behind the glass. Turn off for a plain tinted glass that uses a little less power.",
	"Shows the music as moving light. Pick a style below.",
	"A thin bright line around the edge of the glass.",
	"A slow, soft flow of color inside the glass. Pick its colors below: your theme, your own, or the album cover.",
	"Enlarge and shrink grow the window around the button, so your cursor stays on it.",
	"A soft glow around the album cover, colored like the edges of the cover.",
	"Shows a short explanation under the window when you hover a setting (like this one).",
	"Lets enlarge (and holding it) take over the whole screen. Off: it stops at filling the screen above the taskbar.",
	"While the player is hidden, a small popup shows the new song whenever Spotify moves on to the next one. Uses the key-press popup's position.",
	"When your mouse is away from the player for a few seconds (and it's not the window in front), it fades to a faint glass. Move the mouse over it and it comes right back. Handy when it's pinned on top of a game or video.",
	"Off: the tray icon is Akra Player's red icon. On: it takes your accent color (and follows the album cover if that's your color).",
}

var visTips = [NumVisStyles]string{
	"Bars along the bottom of the player.",
	"A ring of light around the middle of the player, behind the song info.",
	"Bars along the bottom edge of a screen, even when the player is hidden. Clicks go straight through them.",
	"Hundreds of thin strands that move together with the music.",
	"One smooth line that waves with the music, with a soft glow.",
	"A dotted 3D blob that turns and pulses with the music.",
	"Rings of light spreading out from the middle. Beats travel outward as bright rings.",
	"A mirrored waveform through the middle: the newest sound in the center, flowing out to both sides.",
	"Embers rising through the glass. They speed up and glow brighter when it gets loud.",
}
