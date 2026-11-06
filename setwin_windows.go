//go:build windows

package main

// Settings in its own window. The button next to shrink/enlarge on the
// Settings page pops it out; the main window goes back to the player, so you
// can see every change on it as you make it. Both windows are drawn by the
// same UI: the Settings window has its own "view" (page, size, hit areas,
// glass) that is swapped in while it's drawn or handles input.

import (
	"math"
	"time"
	"unsafe"
)

// inSetView runs f with the Settings window's view in place.
func (a *App) inSetView(f func()) {
	u := a.ui
	u.swapView(&a.setView)
	defer u.swapView(&a.setView)
	f()
}

// popOutSettings opens Settings in its own window beside the player.
func (a *App) popOutSettings() {
	u := a.ui
	if a.setOpen {
		a.setFocus = true
		a.kick()
		return
	}
	if a.setWin == nil {
		hInst, _, _ := pGetModuleHandleW.Call(0)
		h, _, _ := pCreateWindowExW.Call(uintptr(WS_EX_LAYERED|WS_EX_TOOLWINDOW),
			uintptr(unsafe.Pointer(u16(overlayClass))), 0, WS_POPUP, 0, 0, 10, 10, a.hwnd, 0, hInst, 0)
		if h == 0 {
			u.ShowToast("Couldn't open a Settings window")
			return
		}
		a.setWin = &overlay{hwnd: h}
	}
	// its size: the player's, but never smaller than Settings needs
	wa := a.workArea()
	s := a.scale
	w := min32(max32(u.BaseW, 600), float32(wa.R-wa.L)/s-24)
	h := min32(max32(u.BaseH, 480), float32(wa.B-wa.T)/s-24)
	a.setSize = [2]float32{w, h}
	a.setView = SettingsView(w, h)
	// beside the player: right if there's room, else left, else on top of it
	r := a.windowRect()
	pw, ph := int32(w*s), int32(h*s)
	gap := int32(12 * s)
	x := r.R + gap
	if x+pw > wa.R {
		x = r.L - gap - pw
		if x < wa.L {
			x = max(wa.L, wa.R-pw)
		}
	}
	y := max(wa.T, min(r.T, wa.B-ph))
	a.setPos = POINT{x, y}
	a.setOpen, a.setFocus = true, true
	if u.Pop != PopNone && !u.PopPinned {
		a.closePop()
	}
	u.PoppedOut = true
	u.Page = 0 // the main window shows the player again
	u.Sheet = SheetNone
	a.trackTip("")
	a.requestCapture()
	a.kick()
}

// closeSettingsWindow closes it; dock = put Settings back in the player.
func (a *App) closeSettingsWindow(dock bool) {
	if !a.setOpen {
		return
	}
	u := a.ui
	a.inSetView(func() {
		if u.HueOpen {
			u.CancelHue()
		}
		u.PickOpen = false
		if u.EditKind != EditNone {
			a.commitEdit()
		}
	})
	if u.RecIdx >= 0 {
		a.cancelRecording()
	}
	a.setOpen = false
	a.setWin.hide()
	u.PoppedOut = false
	u.toastSet = false
	a.trackTip("")
	if dock {
		u.Page = 1
		pSetForegroundWindow.Call(a.hwnd)
	}
	a.kick()
}

// stepSetWin draws the Settings window (before the main window, which then
// gets the last word on the shared vertical-slider list). Returns true while
// it needs frames on its own (scrolling text).
func (a *App) stepSetWin() bool {
	if !a.setOpen || a.setWin == nil {
		return false
	}
	u := a.ui
	if !a.visible || a.hiding { // hidden along with the player
		a.setWin.hide()
		return false
	}
	pos := a.setPos
	if a.setWin.shown { // it may have been dragged
		var r RECT
		if ok, _, _ := pGetWindowRect.Call(a.setWin.hwnd, uintptr(unsafe.Pointer(&r))); ok != 0 {
			pos = POINT{r.L, r.T}
			a.setPos = pos
		}
	}
	s := a.scale
	w, h := int(math.Ceil(float64(a.setSize[0]*s))), int(math.Ceil(float64(a.setSize[1]*s)))
	if a.setCanvas == nil || a.setCanvas.W != w || a.setCanvas.H != h || a.setCanvas.S != s {
		a.setCanvas = NewCanvas(w, h, s)
	}
	var closeReq, marq bool
	a.inSetView(func() {
		// its own size, whatever happened to the shared fields meanwhile
		u.PW, u.PH, u.BaseW, u.BaseH = a.setSize[0], a.setSize[1], a.setSize[0], a.setSize[1]
		u.pw.Snap(u.PW)
		u.ph.Snap(u.PH)
		u.Mini, u.Full, u.Sheet = false, false, SheetNone
		u.miniA.V, u.fullA.V = 0, 0
		u.sheetA.Snap(0)
		u.WinX, u.WinY = float32(pos.X), float32(pos.Y)
		if u.Page != 1 { // something asked to leave Settings: close the window
			closeReq = true
			u.Page = 1
		}
		u.page.Snap(1)
		u.BaseAsync = nil
		u.Render(a.setCanvas)
		marq = u.marqueeOn
	})
	a.setWin.present(a.setCanvas, pos.X, pos.Y, a.appear.V)
	if a.setFocus {
		a.setFocus = false
		pSetForegroundWindow.Call(a.setWin.hwnd)
	}
	if closeReq {
		a.closeSettingsWindow(false)
	}
	return marq
}

// setRect: where the Settings window is (for the backdrop grab).
func (a *App) setRect() (RECT, bool) {
	if !a.setOpen || a.setWin == nil || !a.setWin.shown || a.setCanvas == nil {
		return RECT{}, false
	}
	return RECT{a.setPos.X, a.setPos.Y, a.setPos.X + int32(a.setCanvas.W), a.setPos.Y + int32(a.setCanvas.H)}, true
}

// setProc: mouse and keys for the Settings window.
func setProc(hwnd, msg, wp, lp uintptr) uintptr {
	a := &app
	u := a.ui
	logical := func() (float32, float32) {
		x, y := lparamXY(lp)
		return float32(x) / a.scale, float32(y) / a.scale
	}
	var ret uintptr
	handled := true
	a.inSetView(func() {
		switch msg {
		case WM_NCHITTEST:
			x, y := lparamXY(lp)
			lx, ly := float32(x-a.setPos.X)/a.scale, float32(y-a.setPos.Y)/a.scale
			switch {
			case u.Hit(lx, ly) != hNone:
				ret = HTCLIENT
			case u.InPanel(lx, ly):
				ret = HTCAPTION // drag the glass to move it
			default:
				ret = HTNOWHERE
			}
		case WM_NCMOUSEMOVE:
			if u.Hover != hNone {
				u.Hover = hNone
				a.kick()
			}
			handled = false
		case WM_MOUSEMOVE:
			lx, ly := logical()
			if isDragID(u.Press) {
				a.drag(u.Press, lx, ly)
				return
			}
			if !a.setTracking {
				tme := TRACKMOUSEEVENT{DwFlags: 2, Hwnd: hwnd}
				tme.CbSize = uint32(unsafe.Sizeof(tme))
				pTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tme)))
				a.setTracking = true
			}
			if h := u.Hit(lx, ly); h != u.Hover {
				u.Hover = h
				a.kick()
			}
			a.tipFor = a.setWin.hwnd
			a.trackTip(u.TipAt(lx, ly))
		case WM_MOUSELEAVE:
			a.setTracking = false
			a.trackTip("")
			if u.Press == hNone && u.Hover != hNone {
				u.Hover = hNone
				a.kick()
			}
		case WM_LBUTTONDOWN:
			lx, ly := logical()
			h := u.Hit(lx, ly)
			u.Press = h
			u.FocusSlider = hNone
			if isSliderID(h) {
				u.FocusSlider = h
			}
			pSetCapture.Call(hwnd)
			if isDragID(h) {
				a.drag(h, lx, ly)
			}
			a.kick()
		case WM_LBUTTONUP:
			pReleaseCapture.Call()
			lx, ly := logical()
			h := u.Hit(lx, ly)
			p := u.Press
			u.Press = hNone
			u.Hover = h
			switch {
			case isSliderID(p):
				saveConfig(a.cfg)
			case p == hHueSV || p == hHueBar:
			case p == h && h == hClose:
				u.Page = 0 // closes the window (see stepSetWin)
			case p == h && h == hSetPop:
				a.dockReq = true
			case p == h && h != hNone:
				a.click(h)
			}
			a.kick()
		case WM_MOUSEWHEEL:
			var pt POINT
			pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
			lx, ly := float32(pt.X-a.setPos.X)/a.scale, float32(pt.Y-a.setPos.Y)/a.scale
			h := u.Hit(lx, ly)
			notches := float32(int16(wp>>16)) / 120
			if isSliderID(h) && h == u.FocusSlider {
				a.setSlider(h, clamp01(a.sliderValue(h)+notches*0.05))
				saveConfig(a.cfg)
				return
			}
			if u.Wheel(-notches * 48) {
				a.kick()
			}
		case WM_SETCURSOR:
			if lp&0xffff == HTCLIENT && u.Hover != hNone {
				pSetCursor.Call(a.handCur)
				ret = 1
				return
			}
			handled = false
		case WM_KEYDOWN, WM_SYSKEYDOWN:
			if u.RecIdx >= 0 || u.EditKind != EditNone {
				return // the hook / WM_CHAR handle these
			}
			if wp == VK_ESCAPE {
				switch {
				case u.HueOpen:
					u.CancelHue()
				case u.PickOpen:
					u.PickOpen = false
				default:
					u.Page = 0 // close the window
				}
				a.kick()
			}
		case WM_CHAR:
			if u.EditKind != EditNone {
				a.onChar(rune(wp))
			}
		case WM_ACTIVATE:
			other := lp != a.hwnd && (a.pop == nil || lp != a.pop.hwnd)
			if wp&0xffff == 0 && other { // focus left Akra Player
				if u.RecIdx >= 0 {
					a.cancelRecording()
				}
				if u.EditKind != EditNone {
					a.commitEdit()
					a.kick()
				}
			}
		case WM_MOVE:
			// keep the glass behind it in step; grab again once it leaves the area
			if r, ok := a.setRect(); ok {
				var wr RECT
				pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&wr)))
				a.setPos = POINT{wr.L, wr.T}
				c := a.capArea
				if (wr.L < c.L || wr.T < c.T || wr.L+(r.R-r.L) > c.R || wr.T+(r.B-r.T) > c.B) && time.Since(a.lastCap) > 120*time.Millisecond {
					a.requestCapture()
				}
			}
			a.kick()
			handled = false
		default:
			handled = false
		}
	})
	if a.dockReq {
		a.dockReq = false
		a.closeSettingsWindow(true)
	}
	if handled {
		return ret
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, msg, wp, lp)
	return r
}
