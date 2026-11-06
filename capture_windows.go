//go:build windows

package main

import (
	"strconv"
	"syscall"
	"time"
	"unsafe"
)

var (
	pGetDC                 = user32.NewProc("GetDC")
	pReleaseDC             = user32.NewProc("ReleaseDC")
	pStretchBlt            = gdi32.NewProc("StretchBlt")
	pSetStretchBltMode     = gdi32.NewProc("SetStretchBltMode")
	pSetBrushOrgEx         = gdi32.NewProc("SetBrushOrgEx")
	pPrivateExtractIconsW  = user32.NewProc("PrivateExtractIconsW")
	pDrawIconEx            = user32.NewProc("DrawIconEx")
	pDestroyIcon           = user32.NewProc("DestroyIcon")
	pSetTextCharacterExtra = gdi32.NewProc("SetTextCharacterExtra")
	pIsWindow              = user32.NewProc("IsWindow")
	pGetWindow             = user32.NewProc("GetWindow")
	pGetWindowLongPtrW     = user32.NewProc("GetWindowLongPtrW")
	pDwmGetWindowAttribute = syscall.NewLazyDLL("dwmapi.dll").NewProc("DwmGetWindowAttribute")
	pDeleteDC              = gdi32.NewProc("DeleteDC")
)

var pSetWindowDisplayAffinity = user32.NewProc("SetWindowDisplayAffinity")

// The glass blurs a capture of the screen behind it, so it must not be in
// that capture itself (it would blur a picture of itself: haze). 2.4 hid the
// player from all captures while blur was on, which also kept it out of your
// screenshots. Now it's hidden only for the instant of each grab (a few
// frames, every couple of seconds or when it moves), so screenshots show it.

const (
	wdaNone    = 0
	wdaExclude = 0x11 // WDA_EXCLUDEFROMCAPTURE (Windows 10 2004+)
)

// applyCaptureExclusion checks whether Windows can hide us from a capture
// and makes sure we're visible to screenshots.
func (a *App) applyCaptureExclusion() {
	r, _, _ := pSetWindowDisplayAffinity.Call(a.hwnd, wdaExclude)
	a.wdaOK = r != 0
	pSetWindowDisplayAffinity.Call(a.hwnd, wdaNone)
}

func (a *App) setExcluded(on bool) {
	mode := uintptr(wdaNone)
	if on {
		mode = wdaExclude
	}
	if a.excluded == on {
		return
	}
	a.excluded = on
	pSetWindowDisplayAffinity.Call(a.hwnd, mode)
	if a.pill != nil && a.pill.hwnd != 0 {
		pSetWindowDisplayAffinity.Call(a.pill.hwnd, mode)
	}
	if a.pop != nil && a.pop.hwnd != 0 {
		pSetWindowDisplayAffinity.Call(a.pop.hwnd, mode)
	}
	if a.setWin != nil && a.setWin.hwnd != 0 {
		pSetWindowDisplayAffinity.Call(a.setWin.hwnd, mode)
	}
}

// requestCapture asks the capture worker for a fresh backdrop + glass.
func (a *App) requestCapture() {
	if !a.cfg.blur || !(a.visible || a.pinnedPopShown()) || a.canvas == nil {
		return
	}
	if !a.wdaOK && a.appear.V > 0 { // can't hide from capture: only grab while hidden
		return
	}
	win := a.windowRect()
	// grab a margin around the window too: when you drag it, the blur can
	// follow instantly from what's already captured
	m := max(int32(150*a.scale), (max(win.R-win.L, win.B-win.T))*2/5)
	area := RECT{win.L - m, win.T - m, win.R + m, win.B + m}
	if a.ui.Full {
		area = win
	}
	vx, _, _ := pGetSystemMetrics.Call(76)
	vy, _, _ := pGetSystemMetrics.Call(77)
	vw, _, _ := pGetSystemMetrics.Call(78)
	vh, _, _ := pGetSystemMetrics.Call(79)
	if vw > 0 && vh > 0 {
		vl, vt := int32(vx), int32(vy)
		area = RECT{max(area.L, vl), max(area.T, vt), min(area.R, vl+int32(vw)), min(area.B, vt+int32(vh))}
	}
	if a.pill != nil && a.pill.shown { // the pill above uses the same backdrop
		pr := RECT{a.pillPos.X, a.pillPos.Y, a.pillPos.X + int32(a.pillCanvas.W), a.pillPos.Y + int32(a.pillCanvas.H)}
		area = RECT{min(area.L, pr.L), min(area.T, pr.T), max(area.R, pr.R), max(area.B, pr.B)}
	}
	if pr, ok := a.setRect(); ok { // and the Settings window
		area = RECT{min(area.L, pr.L), min(area.T, pr.T), max(area.R, pr.R), max(area.B, pr.B)}
	}
	if a.pop != nil && a.pop.shown { // so does a pop-out (mixer, keystrokes)
		pr := RECT{a.popPos.X, a.popPos.Y, a.popPos.X + int32(a.popCanvas.W), a.popPos.Y + int32(a.popCanvas.H)}
		area = RECT{min(area.L, pr.L), min(area.T, pr.T), max(area.R, pr.R), max(area.B, pr.B)}
	}
	a.capArea = area
	a.lastCap = time.Now()
	if a.wdaOK {
		a.setExcluded(true)
	}
	a.capSeq++
	sendCap(capReq{capture: true, rect: area, win: win, seq: a.capSeq, params: a.ui.BaseParamsFor(a.canvas)})
}

// onCapGrab: the screen has been grabbed; be visible to screenshots again.
func (a *App) onCapGrab(seq int) {
	if seq >= a.capSeq {
		a.setExcluded(false)
	}
}

// onBase receives backdrop/glass from the capture worker.
func (a *App) onBase() {
	got := false
	for {
		select {
		case r := <-capResCh:
			if r.backdrop != nil {
				a.ui.Backdrop, a.ui.BackdropScale, a.ui.BackdropVer = r.backdrop, r.bscale, r.bver
				a.bdX, a.bdY = r.bx, r.by
				a.ui.BdX, a.ui.BdY = float32(r.bx), float32(r.by)
				a.lastChange = time.Now() // something behind changed
				got = true
			}
			if a.ui.SetBase(r.layer) {
				got = true
			}
		default:
			if got {
				a.kick()
			}
			return
		}
	}
}

// ---------- app icons ----------

var iconCache = map[string]*Canvas{}

// iconFor returns the icon of an exe at px x px (premultiplied), or nil.
func iconFor(path string, px int) *Canvas {
	if path == "" || px <= 0 {
		return nil
	}
	k := path + "|" + strconv.Itoa(px)
	if c, ok := iconCache[k]; ok {
		return c
	}
	var hicon uintptr
	var id uint32
	n, _, _ := pPrivateExtractIconsW.Call(uintptr(unsafe.Pointer(u16(path))), 0, uintptr(px), uintptr(px),
		uintptr(unsafe.Pointer(&hicon)), uintptr(unsafe.Pointer(&id)), 1, 0)
	if n == 0 || n == 0xFFFFFFFF || hicon == 0 {
		iconCache[k] = nil
		return nil
	}
	c := iconToCanvas(hicon, px)
	pDestroyIcon.Call(hicon)
	iconCache[k] = c
	return c
}

// Draw the icon on black and on white; the difference gives exact alpha
// for every kind of icon (old 1-bit masks or modern 32-bit ones).
func iconToCanvas(h uintptr, px int) *Canvas {
	dc, _, _ := pCreateCompatibleDC.Call(0)
	defer pDeleteDC.Call(dc)
	draw := func(bg byte) []byte {
		bmp, bits := newDIB(px, px)
		old, _, _ := pSelectObject.Call(dc, bmp)
		pix := unsafe.Slice((*byte)(bits), px*px*4)
		for i := range pix {
			pix[i] = bg
		}
		pDrawIconEx.Call(dc, 0, 0, h, uintptr(px), uintptr(px), 0, 0, 3 /*DI_NORMAL*/)
		pGdiFlush.Call()
		out := append([]byte(nil), pix...)
		pSelectObject.Call(dc, old)
		pDeleteObject.Call(bmp)
		return out
	}
	blk, wht := draw(0), draw(255)
	c := NewCanvas(px, px, 1)
	for i := 0; i < px*px; i++ {
		var a float32
		for k := 0; k < 3; k++ {
			a += 1 - (float32(wht[i*4+k])-float32(blk[i*4+k]))/255
		}
		a = clamp01(a / 3)
		c.Pix[i*4] = float32(blk[i*4+2]) / 255
		c.Pix[i*4+1] = float32(blk[i*4+1]) / 255
		c.Pix[i*4+2] = float32(blk[i*4]) / 255
		c.Pix[i*4+3] = a
	}
	return c
}
