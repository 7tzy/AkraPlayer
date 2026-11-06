//go:build windows

package main

// Two small layered windows besides the player:
//   - pill: a mini player floating above the glass while Settings is open
//   - toast: the key-press popup in the corner of the screen. It's topmost and
//     click-through, so it shows over games in borderless/windowed mode.
//     (Exclusive-fullscreen games own the screen; nothing can draw over them.)

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime/debug"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

const (
	overlayClass      = "AkraPlayerOverlay"
	WS_EX_TRANSPARENT = 0x20
	WS_EX_NOACTIVATE  = 0x08000000
	WM_LIKED          = WM_APP + 10
	WM_MOUSEACTIVATE  = 0x0021
)

type overlay struct {
	hwnd       uintptr
	memDC, dib uintptr
	oldBmp     uintptr
	bits       unsafe.Pointer
	w, h       int
	canvas     *Canvas
	shown      bool
	tracking   bool
	topmost    bool
}

var overlayProcCB = syscall.NewCallback(overlayProc)

func registerOverlayClass() {
	hInst, _, _ := pGetModuleHandleW.Call(0)
	arrow, _, _ := pLoadCursorW.Call(0, 32512)
	wc := WNDCLASSEX{LpfnWndProc: overlayProcCB, HInstance: hInst, HCursor: arrow, LpszClassName: u16(overlayClass)}
	wc.CbSize = uint32(unsafe.Sizeof(wc))
	pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
}

// newOverlay: owner != 0 keeps the overlay just above that window (and
// hidden with it) instead of above every app.
func newOverlay(clickThrough bool, owner uintptr) *overlay {
	hInst, _, _ := pGetModuleHandleW.Call(0)
	ex := uintptr(WS_EX_LAYERED | WS_EX_TOOLWINDOW | WS_EX_NOACTIVATE)
	if owner == 0 {
		ex |= WS_EX_TOPMOST
	}
	if clickThrough {
		ex |= WS_EX_TRANSPARENT
	}
	h, _, _ := pCreateWindowExW.Call(ex, uintptr(unsafe.Pointer(u16(overlayClass))), 0, WS_POPUP,
		0, 0, 10, 10, owner, 0, hInst, 0)
	return &overlay{hwnd: h, topmost: owner == 0}
}

// present draws c at (x, y) with the given opacity.
func (o *overlay) present(c *Canvas, x, y int32, alpha float32) {
	if o.hwnd == 0 {
		return
	}
	w, h := c.W, c.H
	if o.memDC == 0 {
		o.memDC, _, _ = pCreateCompatibleDC.Call(0)
	}
	if o.dib == 0 || o.w != w || o.h != h {
		if o.dib != 0 {
			pSelectObject.Call(o.memDC, o.oldBmp)
			pDeleteObject.Call(o.dib)
		}
		o.dib, o.bits = newDIB(w, h)
		o.oldBmp, _, _ = pSelectObject.Call(o.memDC, o.dib)
		o.w, o.h = w, h
	}
	c.BGRA(unsafe.Slice((*byte)(o.bits), w*h*4), true)
	sz := SIZE{int32(w), int32(h)}
	src := POINT{}
	pos := POINT{x, y}
	bf := BLENDFUNCTION{0, 0, byte(255 * clamp01(alpha)), 1}
	pUpdateLayeredWindow.Call(o.hwnd, 0, uintptr(unsafe.Pointer(&pos)), uintptr(unsafe.Pointer(&sz)), o.memDC,
		uintptr(unsafe.Pointer(&src)), 0, uintptr(unsafe.Pointer(&bf)), 2)
	if !o.shown {
		pShowWindow.Call(o.hwnd, 4) // SW_SHOWNOACTIVATE
		o.shown = true
	}
	if o.topmost { // stay above everything (games in borderless mode included)
		pSetWindowPos.Call(o.hwnd, ^uintptr(0), 0, 0, 0, 0, SWP_NOMOVE|SWP_NOSIZE|SWP_NOACTIVATE)
	}
}

func (o *overlay) hide() {
	if o.shown {
		pShowWindow.Call(o.hwnd, SW_HIDE)
		o.shown = false
	}
}

// ---------- key-press toast ----------

type keyToastState struct {
	ov                *overlay
	icon, name, value string
	a                 Tween
	until             time.Time
	scale             float32
	mon               RECT
	pos               int
}

// keyToast: the key-press popup, where Settings > General puts it, on the
// screen you're working on.
func (a *App) keyToast(iconKey, name, value string) {
	a.keyToastAt(iconKey, name, value, a.cfg.toastPos, -1)
}

// keyToastAt shows a popup at pos on screen mon (-1 = the screen you're
// working on).
func (a *App) keyToastAt(iconKey, name, value string, pos, monIdx int) {
	t := &a.toast
	if t.ov == nil {
		t.ov = newOverlay(true, 0)
	}
	if iconKey == "spotify" && a.spot != 0 {
		iconKey = windowPath(a.spot) // Spotify's real icon
	}
	t.icon, t.name, t.value = iconKey, name, value
	t.until = time.Now().Add(1600 * time.Millisecond)
	// the corner of the screen you're looking at (where the active window is)
	fg, _, _ := pGetForegroundWindow.Call()
	if fg == 0 {
		fg = a.hwnd
	}
	mon, _, _ := pMonitorFromWindow.Call(fg, 2)
	mi := MONITORINFO{}
	mi.CbSize = uint32(unsafe.Sizeof(mi))
	pGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&mi)))
	t.mon = mi.RcWork
	if fullscreenForeground() {
		t.mon = mi.RcMonitor
	}
	if ms := enumMonitors(); monIdx >= 0 && monIdx < len(ms) {
		t.mon = ms[monIdx].RcWork
	}
	t.pos = pos
	t.scale = a.dpiScale()
	a.overlayAnim = true
	if atomic.LoadInt32(&frameWanted) == 0 {
		a.lastFrame = time.Now()
	}
	wantFrames(true)
}

// stepToast animates and draws the toast. Returns true while it's on screen.
func (a *App) stepToast(dt float32) bool {
	t := &a.toast
	if t.ov == nil {
		return false
	}
	on := time.Now().Before(t.until)
	busy := t.a.Step(b2f(on), 14, dt)
	if t.a.V <= 0.01 && !on {
		t.ov.hide()
		return false
	}
	u := a.ui
	s := t.scale
	lw := u.withScale(s, func() float32 { return u.ToastWidth(t.name, t.value) })
	w, h := int(math.Ceil(float64(lw*s))), int(math.Ceil(float64(ToastH*s)))
	if a.toastCanvas == nil || a.toastCanvas.W != w || a.toastCanvas.H != h || a.toastCanvas.S != s {
		a.toastCanvas = NewCanvas(w, h, s)
	}
	u.withScale(s, func() float32 { u.RenderKeyToast(a.toastCanvas, t.icon, t.name, t.value, 1); return 0 })
	e := t.a.V
	e = e * e * (3 - 2*e)
	m := int32(22 * s)
	// the corner (or edge) chosen in Settings > General
	mon := t.mon
	cx := (mon.L + mon.R - int32(w)) / 2
	slide := int32((1 - e) * 10 * s)
	var x, y int32
	switch t.pos {
	case ToastTopLeft:
		x, y = mon.L+m, mon.T+m-slide
	case ToastTopCenter:
		x, y = cx, mon.T+m-slide
	case ToastBottomLeft:
		x, y = mon.L+m, mon.B-int32(h)-m+slide
	case ToastBottomCenter:
		x, y = cx, mon.B-int32(h)-m+slide
	case ToastBottomRight:
		x, y = mon.R-int32(w)-m, mon.B-int32(h)-m+slide
	default:
		x, y = mon.R-int32(w)-m, mon.T+m-slide
	}
	t.ov.present(a.toastCanvas, x, y, e)
	return busy || on
}

// ---------- settings pill player ----------

func (a *App) kickPill() {
	if a.pill != nil && a.pill.shown {
		a.kick()
	}
}

// stepPill shows the pill above the glass while Settings is open.
func (a *App) stepPill(dt float32) bool {
	u := a.ui
	want := a.visible && !a.hiding && u.Page == 1 && !u.Mini && !u.Full
	busy := a.pillA.Step(b2f(want), 12, dt)
	if a.pillA.V <= 0.01 && !want {
		if a.pill != nil {
			a.pill.hide()
		}
		return false
	}
	if a.pill == nil {
		a.pill = newOverlay(false, a.hwnd)
	}
	hover := isPillID(u.Hover) || isPillID(u.Press)
	now := time.Now()
	if !busy && !hover && now.Sub(a.pillLast) < 32*time.Millisecond {
		return true // the fluid animates slowly; ~30 fps is plenty
	}
	a.pillLast = now
	s := a.scale
	w, h := int(math.Ceil(float64(PillW*s))), int(math.Ceil(float64(PillH*s)))
	if a.pillCanvas == nil || a.pillCanvas.W != w || a.pillCanvas.H != h || a.pillCanvas.S != s {
		a.pillCanvas = NewCanvas(w, h, s)
	}
	r := a.windowRect()
	gap := int32(10 * s)
	e := a.pillA.V
	e = e * e * (3 - 2*e)
	x := (r.L+r.R)/2 - int32(w)/2
	y := r.T - gap - int32(h) + int32((1-e)*12*s)
	if mi, ok := monitorOf(a.hwnd); ok && y < mi.RcWork.T { // no room above: sit below instead
		y = r.B + gap - int32((1-e)*12*s)
	}
	a.pillPos = POINT{x, y}
	// same glass as the player: blur from the same backdrop, fluid, outline
	bx, by := float32(x-a.bdX), float32(y-a.bdY)
	u.withScale(s, func() float32 { u.RenderPill(a.pillCanvas, bx, by); return 0 })
	a.pill.present(a.pillCanvas, x, y, e*a.appear.V)
	return busy || hover || u.Toggles[TogFluid] || u.playing()
}

// ---------- Screen Edge visualizer ----------

func (a *App) edgeWanted() bool {
	return a.cfg.vis && a.cfg.visStyle == VisEdge && a.spot != 0
}

// stepEdge draws bars along the bottom of the chosen screen (~30 fps).
func (a *App) stepEdge(now time.Time) bool {
	want := a.edgeWanted()
	if !want {
		if a.edge != nil && a.edge.shown {
			a.edge.hide()
		}
		return false
	}
	if now.Sub(a.edgeLast) < 32*time.Millisecond {
		return true
	}
	a.edgeLast = now
	if a.edge == nil {
		a.edge = newOverlay(true, 0)
	}
	rects := monitorWorkRects()
	if len(rects) == 0 {
		return false
	}
	m := rects[0]
	if i := a.cfg.visMon; i >= 0 && i < len(rects) {
		m = rects[i]
	}
	s := a.scale
	w, h := int(m.R-m.L), int(72*s)
	if a.edgeCanvas == nil || a.edgeCanvas.W != w || a.edgeCanvas.H != h || a.edgeCanvas.S != s {
		a.edgeCanvas = NewCanvas(w, h, s)
	}
	u := a.ui
	u.withScale(s, func() float32 { u.RenderEdge(a.edgeCanvas); return 0 })
	a.edge.present(a.edgeCanvas, m.L, m.B-int32(h), 1)
	return u.playing() // paused: bars are flat, no need to keep drawing
}

// ---------- monitors ----------

var (
	pEnumDisplayMonitors = user32.NewProc("EnumDisplayMonitors")
	monList              []MONITORINFO
	monEnumCB            = syscall.NewCallback(func(mon, hdc, rc, lp uintptr) uintptr {
		mi := MONITORINFO{}
		mi.CbSize = uint32(unsafe.Sizeof(mi))
		pGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&mi)))
		monList = append(monList, mi)
		return 1
	})
	monCache     []RECT
	monCacheTime time.Time
)

func enumMonitors() []MONITORINFO {
	monList = nil
	pEnumDisplayMonitors.Call(0, 0, monEnumCB, 0)
	return monList
}

// monitorWorkRects: each screen's usable area (above the taskbar), refreshed every few seconds.
func monitorWorkRects() []RECT {
	if time.Since(monCacheTime) > 3*time.Second || monCache == nil {
		monCache = monCache[:0]
		for _, mi := range enumMonitors() {
			monCache = append(monCache, mi.RcWork)
		}
		monCacheTime = time.Now()
	}
	return monCache
}

func monitorNames() []string {
	var out []string
	for i, mi := range enumMonitors() {
		name := fmt.Sprintf("Display %d", i+1)
		if mi.DwFlags&1 != 0 {
			name += " (main)"
		}
		out = append(out, name)
	}
	monCacheTime = time.Time{}
	return out
}

// logCrash writes an unexpected error to %APPDATA%\AkraPlayer\crash.txt.
func logCrash(r any) {
	dir := configDir("AkraPlayer")
	os.MkdirAll(dir, 0755)
	f, err := os.OpenFile(filepath.Join(dir, "crash.txt"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "---- %s (Akra Player %s)\n%v\n%s\n", time.Now().Format(time.RFC3339), appVersion, r, debug.Stack())
}

func monitorOf(hwnd uintptr) (MONITORINFO, bool) {
	mi := MONITORINFO{}
	mi.CbSize = uint32(unsafe.Sizeof(mi))
	mon, _, _ := pMonitorFromWindow.Call(hwnd, 2)
	if mon == 0 {
		return mi, false
	}
	r, _, _ := pGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&mi)))
	return mi, r != 0
}

func overlayProc(hwnd, msg, wp, lp uintptr) uintptr {
	a := &app
	if a.pop != nil && hwnd == a.pop.hwnd {
		return popProc(hwnd, msg, wp, lp)
	}
	if a.setWin != nil && hwnd == a.setWin.hwnd {
		return setProc(hwnd, msg, wp, lp)
	}
	o := a.pill
	if o == nil || hwnd != o.hwnd {
		r, _, _ := pDefWindowProcW.Call(hwnd, msg, wp, lp)
		return r
	}
	u := a.ui
	logical := func() (float32, float32) {
		x, y := lparamXY(lp)
		return float32(x) / a.scale, float32(y) / a.scale
	}
	switch msg {
	case WM_MOUSEACTIVATE:
		return 3 // MA_NOACTIVATE: clicking the pill doesn't steal focus
	case WM_MOUSEMOVE:
		if !o.tracking {
			tme := TRACKMOUSEEVENT{DwFlags: 2, Hwnd: hwnd}
			tme.CbSize = uint32(unsafe.Sizeof(tme))
			pTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tme)))
			o.tracking = true
		}
		lx, _ := logical()
		if u.Press == hPillVol { // dragging the volume bar
			a.setSpotifyVolume(u.SliderValue(hPillVol, lx))
			return 0
		}
		if h := u.PillHit(logical()); h != u.Hover {
			u.Hover = h
			a.kick()
		}
		return 0
	case WM_MOUSELEAVE:
		o.tracking = false
		if isPillID(u.Hover) && u.Press == hNone {
			u.Hover = hNone
			a.kick()
		}
		return 0
	case WM_LBUTTONDOWN:
		u.Press = u.PillHit(logical())
		pSetCapture.Call(hwnd)
		if u.Press == hPillVol {
			lx, _ := logical()
			a.setSpotifyVolume(u.SliderValue(hPillVol, lx))
		}
		a.kick()
		return 0
	case WM_MOUSEWHEEL:
		if h := u.Hover; h == hPillMute || h == hPillVol {
			notches := float32(int16(wp>>16)) / 120
			a.setSpotifyVolume(u.SpotVol + notches*0.05)
		}
		return 0
	case WM_LBUTTONUP:
		pReleaseCapture.Call()
		h := u.PillHit(logical())
		p := u.Press
		u.Press = hNone
		if p == h {
			switch h {
			case hPillPlay:
				a.spotifyCommand(APPCOMMAND_MEDIA_PLAY_PAUSE, true)
			case hPillPrev:
				a.spotifyCommand(APPCOMMAND_MEDIA_PREVIOUSTRACK, true)
			case hPillNext:
				a.spotifyCommand(APPCOMMAND_MEDIA_NEXTTRACK, true)
			case hPillTrack:
				u.Page = 0 // back to the player
			case hPillMute:
				a.toggleSpotifyMute()
			}
		}
		a.kick()
		return 0
	case WM_SETCURSOR:
		if isPillID(u.Hover) {
			pSetCursor.Call(a.handCur)
			return 1
		}
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, msg, wp, lp)
	return r
}

func isPillID(h int) bool {
	return (h >= hPillPrev && h <= hPillTrack) || h == hPillMute || h == hPillVol
}
