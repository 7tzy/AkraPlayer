package main

// The Export settings and Import settings windows (glass pop-outs).

import "strings"

// ImportItem: one saved export (a folder with general/appearance/keybinds).
type ImportItem struct {
	Name, Date, Path string
	Has              [NumExp]bool
}

// Parts lists what an export holds ("General · Keybinds").
func (it ImportItem) Parts() string {
	var p []string
	for i, h := range it.Has {
		if h {
			p = append(p, expNames[i])
		}
	}
	if len(p) == 0 {
		return "Empty"
	}
	return strings.Join(p, " · ")
}

// fitTail shortens s from the front ("…\AkraPlayer\Exported Profiles").
func (u *UI) fitTail(s string, size float32, weight int, maxW float32) string {
	if u.textW(s, size, weight) <= maxW {
		return s
	}
	r := []rune(s)
	for len(r) > 1 {
		r = r[1:]
		if t := "…" + string(r); u.textW(t, size, weight) <= maxW {
			return t
		}
	}
	return "…"
}

// field: a read-only box with a label above it.
func (u *UI) field(c *Canvas, label, value string, x0, x1, y float32, tail bool, rightW float32) {
	u.text(c, label, x0, y+10, 11.5, 600, sub, 0, false)
	by := y + 22
	c.RoundRect(x0, by, x1-x0-rightW, 34, 10, Solid(Black(0.22)))
	c.StrokeRoundRect(x0, by, x1-x0-rightW, 34, 10, 1, Solid(White(0.16)))
	maxW := x1 - x0 - rightW - 24
	v := u.fit(value, 12, 600, maxW)
	if tail {
		v = u.fitTail(value, 12, 600, maxW)
	}
	u.text(c, v, x0+12, by+17, 12, 600, txt, 0, false)
}

// transferDone: the "it worked" view (a check, a line, and buttons).
func (u *UI) transferDone(c *Canvas, w, h float32, title, detail string, openBtn bool) {
	cy := h/2 - 30
	c.Circle(w/2, cy, 26, Solid(featGreen.WithA(0.85)))
	c.StrokeCircle(w/2, cy, 26, 1.2, Solid(White(0.5)))
	c.Line(w/2-10, cy+1, w/2-3, cy+8, 3, Solid(White(1)))
	c.Line(w/2-3, cy+8, w/2+11, cy-7, 3, Solid(White(1)))
	u.text(c, title, w/2, cy+48, 15, 700, txt, 1, false)
	u.marquee(c, detail, w/2-min32(u.textW(detail, 11, 500), w-48)/2, cy+70, 11, 500, dim, w-48)
	if openBtn {
		u.ghostPill(c, hExpOpen, "Open folder", w/2-6, h-34, 2)
		u.accentPill(c, hExpCancel, "Done", w/2+6, h-34, 0)
	} else {
		u.accentPill(c, hExpCancel, "Done", w/2, h-34, 1)
	}
}

func (u *UI) drawTransfer(c *Canvas, kind int, w, h float32) {
	x0, x1 := float32(20), w-20
	if kind == PopExport {
		if u.ExpDone != "" {
			name := u.ExpDone
			if i := strings.LastIndexAny(name, `\/`); i >= 0 {
				name = name[i+1:]
			}
			u.transferDone(c, w, h, "Exported", name, true)
			return
		}
		y := float32(56)
		u.text(c, "What to include", x0, y+10, 11.5, 600, sub, 0, false)
		u.chipRow(c, hExpInc0, expNames[:], x0, x1, y+22)
		y += 64
		u.field(c, "Name", u.ExpName, x0, x1, y, false, 0)
		y += 66
		u.field(c, "Save to", u.ExpDir, x0, x1, y, true, 96)
		u.ghostPill(c, hExpChange, "Change…", x1, y+39, 2)
		y += 66
		u.marquee(c, "Your settings are saved as small text files in a folder with this name.", x0, y+12, 10.5, 500, dim, x1-x0)
		if u.TransferMsg != "" {
			u.marquee(c, u.TransferMsg, x0, y+32, 11, 600, Hex(0xffb0a6, 1), x1-x0)
		}
		u.ghostPill(c, hExpCancel, "Cancel", x1-u.textW("Export", 11.5, 600)-22-8, h-30, 2)
		u.accentPill(c, hExpGo, "Export", x1, h-30, 2)
		return
	}
	// Import
	if u.ImpDone != "" {
		u.transferDone(c, w, h, "Imported", u.ImpDone, false)
		return
	}
	y := float32(56)
	u.text(c, "Saved exports", x0, y+12, 11.5, 600, sub, 0, false)
	u.ghostPill(c, hImpBrowse, "Browse…", x1, y+12, 2)
	u.tip(c, "Pick a folder of exported settings from somewhere else.", x1-80, y, x1, y+24)
	ly0, ly1 := y+30, h-122
	if len(u.ImpList) == 0 {
		c.RoundRect(x0, ly0, x1-x0, ly1-ly0, 14, Solid(White(0.04)))
		iconFolder(c, w/2, (ly0+ly1)/2-16, Solid(White(0.5)))
		u.text(c, "No exports yet", w/2, (ly0+ly1)/2+8, 12.5, 600, sub, 1, false)
		u.text(c, "Exports are saved in the Exported Profiles folder.", w/2, (ly0+ly1)/2+28, 10.5, 500, dim, 1, false)
	} else {
		sc := &u.popScroll
		oy := c.OY
		c.PushClip(x0-4, ly0, x1+4, ly1, 0)
		c.OY = oy - sc.Pos.X + sc.Rubber(200)
		ry := ly0 + 2
		for i, it := range u.ImpList {
			id := hImp0 + i
			hv, sel := u.h(id), u.selV(id)
			c.RoundRect(x0, ry, x1-x0, 50, 12, Solid(lerpCol(White(0.06+0.05*hv), u.accent.WithA(0.32), sel)))
			c.StrokeRoundRect(x0, ry, x1-x0, 50, 12, 1, Solid(lerpCol(White(0.16), u.accent.Mix(White(1), 0.45), sel)))
			iconFolder(c, x0+22, ry+25, Solid(u.accent.Mix(White(1), 0.5)))
			u.text(c, u.fit(it.Name, 12.5, 600, x1-x0-60), x0+42, ry+17, 12.5, 600, txt, 0, false)
			u.marquee(c, it.Date+"  ·  "+it.Parts(), x0+42, ry+35, 10.5, 500, dim, x1-x0-56)
			u.addHit(c, id, x0, ry, x1, ry+50)
			ry += 56
		}
		c.OY = oy
		c.PopClip()
		sc.Max = max32(0, ry-ly0-(ly1-ly0))
	}
	y = ly1 + 8
	u.text(c, "Import", x0, y+10, 11.5, 600, sub, 0, false)
	if u.ImpSel >= 0 && u.ImpSel < len(u.ImpList) {
		has := u.ImpList[u.ImpSel].Has
		cx := x0
		for i, n := range expNames {
			id := hImpInc0 + i
			wd := u.textW(n, 11, 600) + 24
			s := u.selV(id)
			if !has[i] { // not in this export
				c.RoundRect(cx, y+22, wd, 26, 13, Solid(White(0.03)))
				u.text(c, n, cx+wd/2, y+35, 11, 600, White(0.3), 1, false)
			} else {
				hv := u.h(id)
				c.RoundRect(cx, y+22, wd, 26, 13, Solid(lerpCol(White(0.08+0.07*hv), u.accent.WithA(0.85), s)))
				c.StrokeRoundRect(cx, y+22, wd, 26, 13, 1, Solid(White(0.22)))
				col := White(0.92)
				if s > 0.5 {
					col = u.onAccent()
				}
				u.text(c, n, cx+wd/2, y+35, 11, 600, col, 1, false)
				u.addHit(c, id, cx, y+21, cx+wd, y+49)
			}
			cx += wd + 6
		}
	} else {
		u.text(c, "Pick an export above", x0, y+35, 11, 500, dim, 0, false)
	}
	u.marquee(c, "Imported parts replace your current ones.", x0, y+64, 10.5, 500, dim, x1-x0)
	if u.TransferMsg != "" {
		u.marquee(c, u.TransferMsg, x0, y+64, 11, 600, Hex(0xffb0a6, 1), x1-x0)
	}
	u.ghostPill(c, hExpCancel, "Cancel", x1-u.textW("Import", 11.5, 600)-22-8, h-30, 2)
	ga := c.GA
	if !u.canImport() {
		c.GA = ga * 0.45
	}
	u.accentPill(c, hImpGo, "Import", x1, h-30, 2)
	c.GA = ga
}

// canImport: an export is picked and at least one part of it is ticked.
func (u *UI) canImport() bool {
	if u.ImpSel < 0 || u.ImpSel >= len(u.ImpList) {
		return false
	}
	for i, on := range u.ImpInc {
		if on && u.ImpList[u.ImpSel].Has[i] {
			return true
		}
	}
	return false
}
