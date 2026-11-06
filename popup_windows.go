//go:build windows

package main

// Pop-out windows: the mixer and the keystrokes list open as their own small
// glass windows next to the player. They close when you click anywhere else
// (the player included), unless you pin them.

import (
	"math"
	"time"
	"unsafe"
)

func (a *App) openPop(kind int) {
	u := a.ui
	if u.Pop == kind {
		a.closePop()
		return
	}
	u.Pop = kind
	u.PopPinned = false
	a.popKind = kind
	a.popFree, a.popDrag = false, false // opens beside the player again
	u.popScroll.Pos.Snap(0)
	u.mixScroll.Pos.Snap(0)
	if kind == PopMixer {
		a.refreshMixer()
	}
	if a.pop == nil {
		hInst, _, _ := pGetModuleHandleW.Call(0)
		h, _, _ := pCreateWindowExW.Call(uintptr(WS_EX_LAYERED|WS_EX_TOOLWINDOW),
			uintptr(unsafe.Pointer(u16(overlayClass))), 0, WS_POPUP, 0, 0, 10, 10, a.hwnd, 0, hInst, 0)
		a.pop = &overlay{hwnd: h}
	}
	a.popFocus = true
	a.kick()
}

func (a *App) closePop() {
	u := a.ui
	if u.Pop == PopNone {
		return
	}
	if u.RecIdx >= 0 {
		a.cancelRecording()
	}
	u.Pop = PopNone
	if u.Hover >= hNone && isPopOnly(u.Hover) {
		u.Hover = hNone
	}
	a.kick()
}

// pinnedPopShown: a pinned pop-out is on screen (it stays when the player hides).
func (a *App) pinnedPopShown() bool {
	return a.pop != nil && a.pop.shown && a.ui.PopPinned && a.ui.Pop != PopNone
}

func isPopOnly(h int) bool {
	return h == hPopPin || h == hPopClose || inRange(h, hMixVol0) || inRange(h, hMixIcon0)
}

// stepPop animates and draws the pop-out beside the player.
func (a *App) stepPop(dt float32) bool {
	u := a.ui
	if (!a.visible || a.hiding) && !u.PopPinned { // a pinned one stays when the player hides
		if u.Pop != PopNone {
			u.Pop = PopNone
		}
	}
	busy := u.popA.V > 0.001 && u.popA.V < 0.999
	if u.popA.V <= 0.01 && u.Pop == PopNone {
		if a.pop != nil && a.pop.shown {
			a.pop.hide()
		}
		return false
	}
	if a.pop == nil {
		return false
	}
	now := time.Now()
	hover := u.Hover != hNone && a.popHover
	if !busy && !hover && !a.popDrag && u.Press == hNone && now.Sub(a.popLast) < 32*time.Millisecond {
		return true
	}
	a.popLast = now
	s := a.scale
	lw, lh := u.PopSize(a.popKind)
	w, h := int(math.Ceil(float64(lw*s))), int(math.Ceil(float64(lh*s)))
	if a.popCanvas == nil || a.popCanvas.W != w || a.popCanvas.H != h || a.popCanvas.S != s {
		a.popCanvas = NewCanvas(w, h, s)
	}
	// beside the player: right if there's room, else left, else inside the screen
	r := a.windowRect()
	gap := int32(10 * s)
	wa := a.workArea()
	x := r.R + gap
	if x+int32(w) > wa.R {
		x = r.L - gap - int32(w)
		if x < wa.L {
			x = wa.R - int32(w)
		}
	}
	y := max(wa.T, min(r.T, wa.B-int32(h)))
	e := u.popA.V
	e = e * e * (3 - 2*e)
	x += int32((1 - e) * 10 * s)
	if a.popFree { // you moved it: it stays where you put it
		x, y = a.popFreePos.X, a.popFreePos.Y
	}
	a.popPos = POINT{x, y}
	bx, by := float32(x-a.bdX), float32(y-a.bdY)
	u.withScale(s, func() float32 { u.RenderPopup(a.popCanvas, a.popKind, bx, by); return 0 })
	alpha := e * a.appear.V
	if u.PopPinned {
		alpha = e // pinned: it doesn't fade out with the player
	}
	a.pop.present(a.popCanvas, x, y, alpha)
	if a.popFocus { // take focus, so clicking elsewhere closes it
		a.popFocus = false
		pSetForegroundWindow.Call(a.pop.hwnd)
	}
	return true
}

// popProc: mouse and keys for the pop-out window.
func popProc(hwnd, msg, wp, lp uintptr) uintptr {
	a := &app
	u := a.ui
	o := a.pop
	logical := func() (float32, float32) {
		x, y := lparamXY(lp)
		return float32(x) / a.scale, float32(y) / a.scale
	}
	switch msg {
	case WM_ACTIVATE:
		folderMu.Lock()
		picking := folderBusy // the folder picker took focus: stay open
		folderMu.Unlock()
		if wp&0xffff == 0 && !u.PopPinned && !picking { // clicked somewhere else
			a.closePop()
		}
		return 0
	case WM_MOUSEMOVE:
		if !o.tracking {
			tme := TRACKMOUSEEVENT{DwFlags: 2, Hwnd: hwnd}
			tme.CbSize = uint32(unsafe.Sizeof(tme))
			pTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tme)))
			o.tracking = true
		}
		lx, ly := logical()
		if a.popDrag && wp&1 == 0 { // the button came up somewhere we didn't see
			a.popDrag = false
			a.requestCapture()
		}
		if a.popDrag { // moving the window
			var pt POINT
			pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
			a.popFree = true
			a.popFreePos = POINT{pt.X - a.popGrab.X, pt.Y - a.popGrab.Y}
			// keep the glass behind it fresh once it leaves the grabbed area
			if c := a.capArea; a.popCanvas != nil && time.Since(a.lastCap) > 120*time.Millisecond &&
				(a.popFreePos.X < c.L || a.popFreePos.Y < c.T ||
					a.popFreePos.X+int32(a.popCanvas.W) > c.R || a.popFreePos.Y+int32(a.popCanvas.H) > c.B) {
				a.popPos = a.popFreePos
				a.requestCapture()
			}
			a.stepPop(0)
			return 0
		}
		if inRange(u.Press, hMixVol0) && u.IsVSlider(u.Press) {
			a.setMixerVolume(u.Press-hMixVol0, u.VSliderValue(u.Press, ly))
			return 0
		}
		a.popHover = true
		if h := u.PopHit(lx, ly); h != u.Hover {
			u.Hover = h
			a.kick()
		}
		a.trackTip(u.popTipAt(lx, ly))
		return 0
	case WM_MOUSELEAVE:
		o.tracking = false
		a.popHover = false
		if u.Press == hNone {
			u.Hover = hNone
		}
		a.trackTip("")
		a.kick()
		return 0
	case WM_LBUTTONDOWN:
		lx, ly := logical()
		u.Press = u.PopHit(lx, ly)
		pSetCapture.Call(hwnd)
		if u.Press == hNone { // grabbed the glass itself: move the window
			var pt POINT
			pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
			a.popDrag = true
			a.popGrab = POINT{pt.X - a.popPos.X, pt.Y - a.popPos.Y}
			return 0
		}
		if inRange(u.Press, hMixVol0) && u.IsVSlider(u.Press) {
			a.setMixerVolume(u.Press-hMixVol0, u.VSliderValue(u.Press, ly))
		}
		a.kick()
		return 0
	case WM_LBUTTONUP:
		pReleaseCapture.Call()
		if a.popDrag {
			a.popDrag = false
			a.requestCapture()
			a.kick()
			return 0
		}
		lx, ly := logical()
		h := u.PopHit(lx, ly)
		p := u.Press
		u.Press = hNone
		if p == h && h != hNone {
			switch {
			case h == hPopClose:
				a.closePop()
			case h == hPopPin:
				u.PopPinned = !u.PopPinned
			case h == hEditKeys:
				a.closePop()
				a.click(h)
			case inRange(h, hMixVol0):
			default:
				a.click(h)
			}
		}
		a.kick()
		return 0
	case WM_MOUSEWHEEL:
		var pt POINT
		pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
		lx := float32(pt.X-a.popPos.X) / a.scale
		ly := float32(pt.Y-a.popPos.Y) / a.scale
		notches := float32(int16(wp>>16)) / 120
		h := u.PopHit(lx, ly)
		switch {
		case inRange(h, hMixVol0) || inRange(h, hMixIcon0):
			i := h - hMixVol0
			if inRange(h, hMixIcon0) {
				i = h - hMixIcon0
			}
			if i < len(u.MixerApps) {
				a.setMixerVolume(i, u.MixerApps[i].Vol+notches*0.05)
			}
		default:
			u.PopWheel(-notches * 48)
		}
		a.kick()
		return 0
	case WM_KEYDOWN, WM_SYSKEYDOWN:
		if wp == VK_ESCAPE && u.RecIdx < 0 {
			a.closePop()
		} else if u.RecIdx < 0 {
			a.localKey(uint32(wp))
		}
		return 0
	case WM_SETCURSOR:
		if u.Hover != hNone && a.popHover {
			pSetCursor.Call(a.handCur)
			return 1
		}
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, msg, wp, lp)
	return r
}
