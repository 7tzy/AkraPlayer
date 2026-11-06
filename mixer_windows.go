//go:build windows

package main

import (
	"math"
	"strings"
	"time"
)

// ---------- volume mixer ----------

// refreshMixer lists every app with sound and its volume.
func (a *App) refreshMixer() {
	if time.Now().Before(a.mixHold) {
		return // you're dragging a slider: don't fight it
	}
	apps := ListAudioApps()
	u := a.ui
	u.MixerApps = u.MixerApps[:0]
	a.mixExes = a.mixExes[:0]
	seen := map[string]bool{}
	for _, ap := range apps {
		k := strings.ToLower(ap.Exe)
		if seen[k] || isSelf(ap.Exe) {
			continue
		}
		seen[k] = true
		vol, ok := GetAppVolume(ap.Exe)
		if !ok {
			continue
		}
		muted, _ := IsAppMuted(ap.Exe)
		u.MixerApps = append(u.MixerApps, MixerApp{Name: prettyApp(ap.Exe), IconKey: ap.Path, Vol: vol, Muted: muted})
		a.mixExes = append(a.mixExes, ap.Exe)
	}
	a.kick()
}

func (a *App) setMixerVolume(i int, v float32) {
	u := a.ui
	if i < 0 || i >= len(a.mixExes) || i >= len(u.MixerApps) {
		return
	}
	v = clamp01(v)
	exe := a.mixExes[i]
	SetAppVolume(exe, int(math.Round(float64(v*100))))
	u.MixerApps[i].Vol = v
	if u.MixerApps[i].Muted && v > 0 {
		if m, ok := ToggleAppMute(exe); ok {
			u.MixerApps[i].Muted = m
		}
	}
	if strings.EqualFold(exe, "Spotify.exe") {
		u.SpotVol = v
		a.spotVolHold = time.Now().Add(1200 * time.Millisecond)
	}
	a.mixHold = time.Now().Add(1200 * time.Millisecond)
	a.kick()
}

func (a *App) toggleMixerMute(i int) {
	u := a.ui
	if i < 0 || i >= len(a.mixExes) || i >= len(u.MixerApps) {
		return
	}
	if m, ok := ToggleAppMute(a.mixExes[i]); ok {
		u.MixerApps[i].Muted = m
		if strings.EqualFold(a.mixExes[i], "Spotify.exe") {
			u.SpotMuted = m
		}
	}
	a.kick()
}

// ---------- settings tooltips (a bubble under the window) ----------

// trackTip: the cursor is over a setting with this tooltip ("" = none).
func (a *App) trackTip(t string) {
	if !a.cfg.tips {
		t = ""
	}
	if t == a.tipText {
		return
	}
	a.tipText = t
	pKillTimer.Call(a.hwnd, timerTip)
	switch {
	case t == "":
		a.tipShown = ""
		a.kick()
	case a.tipShown != "": // already showing one: switch right away
		a.tipShown = t
		a.kick()
	default:
		pSetTimer.Call(a.hwnd, timerTip, 450, 0)
	}
}

func (a *App) showTip() {
	if a.tipText != "" && a.visible {
		a.tipShown = a.tipText
		a.kick()
	}
}

// stepTip animates and draws the bubble just below the window.
func (a *App) stepTip(dt float32) bool {
	want := a.tipShown != "" && a.visible && !a.hiding && a.ui.PickOpen == false
	busy := a.tipA.Step(b2f(want), 14, dt)
	if a.tipA.V <= 0.01 && !want {
		if a.tipOv != nil {
			a.tipOv.hide()
		}
		return false
	}
	if a.tipOv == nil {
		a.tipOv = newOverlay(true, a.hwnd)
	}
	text := a.tipShown
	if text == "" {
		text = a.tipLast // fading out: keep the last text
	}
	a.tipLast = text
	u := a.ui
	s := a.scale
	var lw, lh float32
	u.withScale(s, func() float32 { lw, lh = u.TipBubbleSize(text); return 0 })
	w, h := int(math.Ceil(float64(lw*s))), int(math.Ceil(float64(lh*s)))
	if a.tipCanvas == nil || a.tipCanvas.W != w || a.tipCanvas.H != h || a.tipCanvas.S != s || a.tipDrawn != text {
		a.tipCanvas = NewCanvas(w, h, s)
		u.withScale(s, func() float32 { u.RenderTipBubble(a.tipCanvas, text); return 0 })
		a.tipDrawn = text
	}
	r := a.windowRect()
	if a.tipFor != 0 && a.setOpen && a.setWin != nil && a.tipFor == a.setWin.hwnd {
		if sr, ok := a.setRect(); ok {
			r = sr // under the Settings window
		}
	}
	e := a.tipA.V
	e = e * e * (3 - 2*e)
	x := (r.L+r.R)/2 - int32(w)/2
	y := r.B + int32(6*s) - int32((1-e)*8*s)
	if mi, ok := monitorOf(a.hwnd); ok {
		wa := mi.RcWork
		if y+int32(h) > wa.B { // no room below: tuck it just inside the bottom edge
			y = wa.B - int32(h)
		}
		x = max(wa.L, min(x, wa.R-int32(w)))
	}
	a.tipOv.present(a.tipCanvas, x, y, e*a.appear.V)
	return busy
}
