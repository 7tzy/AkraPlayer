//go:build windows

package main

import (
	"fmt"
	"os"
	"runtime"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

const (
	appName   = "Akra Player"
	wndClass  = "AkraPlayerGlassWnd"
	runKey    = `Software\Microsoft\Windows\CurrentVersion\Run`
	runValue  = "AkraPlayer"
	oldRunVal = "SpotifyToggle"
	oldClass  = "SpotifyToggleGlassWnd"

	timerCapture = 4
	timerRecord  = 5
	timerTip     = 6
	timerSleep   = 7 // once a second, always (the sleep timer runs while hidden)

	WM_ART  = WM_APP + 6
	WM_MOVE = 0x0003
	WM_CHAR = 0x0102
)

type App struct {
	hwnd     uintptr
	ui       *UI
	canvas   *Canvas
	scale    float32 // monitor DPI scale
	visible  bool
	tracking bool

	memDC, dib, oldBmp uintptr
	bits               unsafe.Pointer
	dibW, dibH         int

	cfg                   config
	noteTill              map[int]time.Time
	spot                  uintptr
	lastTrack, lastArtist string
	regIDs                []int

	trayIcon  uintptr
	tbCreated uintptr
	handCur   uintptr

	lastFrame  time.Time
	lastRender time.Time
	start      time.Time

	appear     Tween
	hiding     bool
	posX, posY int32

	pick     []pickEntry
	playHwnd uintptr

	// song progress + art
	lastPoll   time.Time
	trackKey   string
	keySince   time.Time
	elapsed    float32
	progKnown  bool
	arts       map[string]artResult
	artPending map[string]bool
	liked      map[string]bool
	liking     bool

	snap        pollSnap
	med         *media
	levels      []float32
	wdaOK       bool
	cx, cy      float32 // window center on screen (physical px); the glass grows around it
	lastSize    [2]int
	lastPos     POINT
	capSize     [2]int
	canvasBuf   []float32
	spotVolHold time.Time

	fromKey       bool // the current action came from a hotkey
	pendingToggle *togglePending
	ducked        string
	duckPrev      float32

	// size. The window's size is in DIPs; the layout adapts to it.
	sizeW, sizeH float32 // the player's size in DIPs (not fullscreen)
	z            float32 // content scale: S = scale * z
	sizeAnim     struct {
		on             bool
		fw, fh, tw, th float32
		t              float32
	}
	anchor struct { // keep a header button under the cursor while resizing
		on     bool
		sx, sy float32 // its screen position (px)
		rx, ly float32 // its position: distance from the right edge, from the top (logical)
	}
	cdrag struct { // dragging a corner
		on     bool
		corner int
		mx, my int32   // mouse at the start (screen)
		w0, h0 float32 // size at the start (DIPs)
		fx, fy int32   // the corner that stays put
	}
	move struct {
		on             bool
		fx, fy, tx, ty float32
		t              Tween
	}
	fade          Tween // fullscreen switches fade out and back in
	fadeThen      func()
	excluded      bool // hidden from screen capture right now
	renderCost    time.Duration
	slowFull      int
	lastPlayCheck time.Time
	// Settings in its own window
	setWin            *overlay
	setOpen, setFocus bool
	setView           View
	setCanvas         *Canvas
	setSize           [2]float32
	setPos            POINT
	setTracking       bool
	setMarq, dockReq  bool
	tipFor            uintptr // the window the tooltip bubble goes under (0 = the player)
	expDir            string  // where exports go (Exported Profiles unless you change it)
	idle              Tween   // Fade when idle: 1 = faded
	idleWant          bool
	lastInput         time.Time
	sleepAt           time.Time // minute sleep timers end here
	sleepKey          string    // "End of song": the song it waits for to end
	pop               *overlay
	popCanvas         *Canvas
	popPos            POINT
	popFree           bool // dragged somewhere by you: stays there until closed
	popFreePos        POINT
	popDrag           bool
	popGrab           POINT // where in the window you grabbed it
	popKind           int
	popLast           time.Time
	popHover          bool
	popFocus          bool
	capArea           RECT
	lastCap           time.Time
	lastChange        time.Time
	recPending        *bind
	ptRemaps          map[bind]bind
	tipText           string // the setting tooltip under the cursor
	tipShown          string
	tipLast           string
	tipDrawn          string
	tipOv             *overlay
	tipCanvas         *Canvas
	tipA              Tween
	mixExes           []string
	mixHold           time.Time
	mixAt             time.Time
	capSeq            int
	bdX, bdY          int32 // screen position of the blur backdrop
	preFull           struct{ cx, cy float32 }
	preMini           struct{ cx, cy float32 }
	holdDone          bool

	// overlays
	toast       keyToastState
	toastCanvas *Canvas
	pill        *overlay
	pillCanvas  *Canvas
	pillA       Tween
	pillPos     POINT
	pillLast    time.Time
	edge        *overlay
	edgeCanvas  *Canvas
	edgeLast    time.Time
	overlayAnim bool
	extHosts    []uintptr
}

var app App

func main() {
	runtime.LockOSThread()
	hidden := false
	if isNativeHostLaunch() { // started by the browser extension
		runNativeHost()
		return
	}
	for _, a := range os.Args[1:] {
		if a == "--media-helper" {
			runMediaHelper()
			return
		}
		if a == "--hidden" || a == "/hidden" {
			hidden = true
		}
	}

	if pSetProcessDpiAwarenessContext.Find() == nil {
		pSetProcessDpiAwarenessContext.Call(^uintptr(3)) // PER_MONITOR_AWARE_V2 (-4)
	} else {
		pSetProcessDPIAware.Call()
	}

	_, _, err := pCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(u16("AkraPlayer.SingleInstance"))))
	if errno, ok := err.(syscall.Errno); ok && errno == 183 { // already running: show it
		if h, _, _ := pFindWindowW.Call(uintptr(unsafe.Pointer(u16(wndClass))), 0); h != 0 {
			pPostMessageW.Call(h, WM_SHOWME, 0, 0)
		}
		return
	}

	closeOldSpotifyToggle()
	loadFonts()
	comInit()

	a := &app
	a.start = time.Now()
	a.arts = map[string]artResult{}
	a.artPending = map[string]bool{}
	a.liked = map[string]bool{}
	a.noteTill = map[int]time.Time{}
	a.med = &media{ch: make(chan mediaMsg, 32), art: map[string]*Canvas{}}
	a.cfg = loadConfig()
	a.ui = NewUI(newGDIText())
	a.ui.Icons = iconFor
	a.ui.BaseAsync = func(p BaseParams) { sendCap(capReq{params: p}) }
	a.applyConfigToUI()
	a.ui.Toggles[TogStartup] = migrateAutostart()
	a.sizeW, a.sizeH = a.cfg.winW, a.cfg.winH
	a.fade.V = 1
	go registerNativeHost()

	a.handCur, _, _ = pLoadCursorW.Call(0, 32649) // IDC_HAND
	arrow, _, _ := pLoadCursorW.Call(0, 32512)

	hInst, _, _ := pGetModuleHandleW.Call(0)
	wc := WNDCLASSEX{
		LpfnWndProc:   syscall.NewCallback(wndProc),
		HInstance:     hInst,
		HCursor:       arrow,
		LpszClassName: u16(wndClass),
	}
	wc.CbSize = uint32(unsafe.Sizeof(wc))
	wc.HIcon, _, _ = user32.NewProc("LoadIconW").Call(hInst, 1)
	pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	ex := uintptr(WS_EX_LAYERED | WS_EX_TOOLWINDOW)
	if a.cfg.pinned {
		ex |= WS_EX_TOPMOST
	}
	w, h := WinSize(1)
	a.hwnd, _, _ = pCreateWindowExW.Call(ex, uintptr(unsafe.Pointer(u16(wndClass))), uintptr(unsafe.Pointer(u16(appName))),
		WS_POPUP, 0, 0, uintptr(w), uintptr(h), 0, 0, hInst, 0)
	if a.hwnd == 0 {
		return
	}
	selfHwnd = a.hwnd
	allowCopyData(a.hwnd)
	registerOverlayClass()
	a.tbCreated, _, _ = pRegisterWindowMessageW.Call(uintptr(unsafe.Pointer(u16("TaskbarCreated"))))

	a.placeInitially()
	a.makeTrayIcon()
	a.addTray()

	startFramePump(a.hwnd)
	pSetTimer.Call(a.hwnd, timerSleep, 1000, 0)
	startHookThread(a.hwnd)
	startPollWorker(a.hwnd)
	startCaptureWorker(a.hwnd)
	a.startMedia()
	a.applyCaptureExclusion()
	a.refreshBinds()
	a.registerAll()

	if !hidden {
		a.show()
	}

	var msg MSG
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if r == 0 || int32(r) == -1 {
			break
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

// The old Spotify Toggle app holds the same hotkey; close it if it's running.
func closeOldSpotifyToggle() {
	h, _, _ := pFindWindowW.Call(uintptr(unsafe.Pointer(u16(oldClass))), 0)
	if h == 0 {
		return
	}
	pPostMessageW.Call(h, WM_DESTROY, 0, 0)
	for i := 0; i < 20; i++ {
		time.Sleep(50 * time.Millisecond)
		if h, _, _ = pFindWindowW.Call(uintptr(unsafe.Pointer(u16(oldClass))), 0); h == 0 {
			break
		}
	}
	time.Sleep(150 * time.Millisecond)
}

func (a *App) applyConfigToUI() {
	u := a.ui
	c := a.cfg
	u.Clarity = c.clarity
	u.ScrollRes = c.scrollRes
	u.Toggles[TogOnTop] = c.pinned
	u.Toggles[TogBlur] = c.blur
	u.Toggles[TogArt] = c.art
	u.Toggles[TogToasts] = c.toasts
	u.Toggles[TogVis] = c.vis
	u.Toggles[TogOutline] = c.outline
	u.Toggles[TogFluid] = c.fluid
	u.Toggles[TogAnchor] = c.anchor
	u.Toggles[TogGlow] = c.glow
	u.Toggles[TogTips] = c.tips
	u.Toggles[TogFullscreen] = c.fullscreen
	u.Toggles[TogSongPopup] = c.songPopup
	u.Toggles[TogIdleFade] = c.idleFade
	u.Toggles[TogTrayAccent] = c.trayAccent
	u.Remaining = c.remaining
	u.IdleDelay, u.IdleOpacity = c.idleDelay, c.idleOpacity
	u.SongPos, u.SongMon = c.songPos, c.songMon
	u.VisSens, u.VisSmooth, u.VisP = c.visSens, c.visSmooth, c.visP
	u.WaveMode, u.WaveCol, u.EdgeStyle = c.waveMode, Hex(c.waveCol, 1), c.edgeStyle
	u.TogStyle, u.ToastPos = c.togStyle, c.toastPos
	u.Declutter = c.declutter
	u.VisStyle, u.VisMonitor = c.visStyle, c.visMon
	u.OpenRes = c.openRes
	u.Monitors = monitorNames()
	u.FluidMode = c.fluidMode
	u.FluidCol = Hex(c.fluidCol, 1)
	u.AccentIdx = c.accent
	u.Custom = Hex(c.custom, 1)
	u.Font = c.font
	a.syncTargets()
	u.SnapAnimations()
}

// ---------- placement / DPI ----------

func (a *App) dpiScale() float32 {
	if pGetDpiForWindow.Find() == nil {
		if d, _, _ := pGetDpiForWindow.Call(a.hwnd); d != 0 {
			return float32(d) / 96
		}
	}
	return 1
}

func (a *App) placeInitially() {
	var x, y int32
	if a.cfg.hasPos && posOnScreen(a.cfg.x, a.cfg.y) {
		x, y = a.cfg.x, a.cfg.y
	} else {
		var wa RECT
		pSystemParametersInfoW.Call(0x30 /*SPI_GETWORKAREA*/, 0, uintptr(unsafe.Pointer(&wa)), 0)
		pSetWindowPos.Call(a.hwnd, 0, uintptr(wa.R-500), uintptr(wa.B-400), 0, 0, SWP_NOSIZE|SWP_NOACTIVATE)
		ds := a.dpiScale()
		x = wa.R - int32(a.sizeW*ds) - int32(18*ds)
		y = wa.B - int32(a.sizeH*ds) - int32(18*ds)
	}
	pSetWindowPos.Call(a.hwnd, 0, uintptr(x), uintptr(y), 0, 0, SWP_NOSIZE|SWP_NOACTIVATE)
	a.posX, a.posY = x, y
	a.scale = a.dpiScale()
	a.applySize()
	w, h := a.sizeW*a.scale, a.sizeH*a.scale
	a.cx, a.cy = float32(x)+float32(w)/2, float32(y)+float32(h)/2
}

// keepCenter records where the glass is, so it can grow and shrink around it.
func (a *App) keepCenter() {
	r := a.windowRect()
	if r.R > r.L {
		a.cx, a.cy = float32(r.L+r.R)/2, float32(r.T+r.B)/2
	}
}

func posOnScreen(x, y int32) bool {
	vx, _, _ := pGetSystemMetrics.Call(76)
	vy, _, _ := pGetSystemMetrics.Call(77)
	vw, _, _ := pGetSystemMetrics.Call(78)
	vh, _, _ := pGetSystemMetrics.Call(79)
	X, Y, W, H := int32(vx), int32(vy), int32(vw), int32(vh)
	return x+100 > X && y+60 > Y && x+100 < X+W && y+60 < Y+H
}

func (a *App) windowRect() RECT {
	var r RECT
	pGetWindowRect.Call(a.hwnd, uintptr(unsafe.Pointer(&r)))
	return r
}

// ---------- rendering & animation loop ----------

// S: pixels per logical unit right now (monitor DPI × the content scale).
func (a *App) S() float32 { return a.scale * a.z }

// applySize sets the player's logical size from the window size (or the
// screen, in fullscreen). Text and buttons stay the same size; the layout
// adapts to the room it has.
func (a *App) applySize() {
	u := a.ui
	w, h := a.sizeW, a.sizeH
	if u.Full {
		if mi, ok := monitorOf(a.hwnd); ok {
			m := mi.RcMonitor
			w, h = float32(m.R-m.L)/a.scale, float32(m.B-m.T)/a.scale
		}
	}
	a.z = 1
	u.BaseW, u.BaseH = w, h
	wa := a.workArea()
	u.MaxW, u.MaxH = float32(wa.R-wa.L)/a.scale, float32(wa.B-wa.T)/a.scale
}

func (a *App) minSize() (float32, float32) { return W, H }

func (a *App) ensureCanvas(c **Canvas, buf *[]float32, w, h int, s float32) {
	if *c == nil || (*c).W != w || (*c).H != h || (*c).S != s {
		if cap(*buf) < w*h*4 {
			*buf = make([]float32, w*h*4)
		}
		*c = &Canvas{W: w, H: h, S: s, Pix: (*buf)[:w*h*4], GA: 1}
	}
}

func (a *App) render() {
	if !a.visible {
		return
	}
	a.setMarq = a.stepSetWin() // the popped-out Settings window first
	u := a.ui
	a.syncLookUI()
	u.T = float32(time.Since(a.start).Seconds())
	S := a.S()
	w, h := u.WinSize(S)
	wr := a.windowRect() // the blur needs to know where we are
	u.WinX, u.WinY = float32(wr.L), float32(wr.T)
	a.ensureCanvas(&a.canvas, &a.canvasBuf, w, h, S)
	u.Render(a.canvas)

	if a.memDC == 0 {
		a.memDC, _, _ = pCreateCompatibleDC.Call(0)
	}
	if a.dib == 0 || a.dibW != w || a.dibH != h {
		if a.dib != 0 {
			pSelectObject.Call(a.memDC, a.oldBmp)
			pDeleteObject.Call(a.dib)
		}
		a.dib, a.bits = newDIB(w, h)
		if a.dib == 0 { // out of memory for the bitmap: skip this frame
			a.dibW, a.dibH = 0, 0
			return
		}
		a.oldBmp, _, _ = pSelectObject.Call(a.memDC, a.dib)
		a.dibW, a.dibH = w, h
	}
	dstPix := unsafe.Slice((*byte)(a.bits), w*h*4)
	parallelRows(h, func(y0, y1 int) {
		sub := Canvas{W: w, H: y1 - y0, Pix: a.canvas.Pix[y0*w*4 : y1*w*4]}
		sub.BGRA(dstPix[y0*w*4:y1*w*4], true)
	})

	e := a.appear.V
	e = e * e * (3 - 2*e)
	sz := SIZE{int32(w), int32(h)}
	src := POINT{}
	idle := a.idle.V
	idle = idle * idle * (3 - 2*idle)
	bf := BLENDFUNCTION{0, 0, byte(255 * e * clamp01(a.fade.V) * (1 - (1-a.cfg.idleOpacity)*idle)), 1}
	var dst uintptr
	pos := a.targetPos(w, h)
	pos.Y += int32((1 - e) * 14 * a.scale) // rise in while appearing
	if a.appear.V < 1 || a.lastSize != [2]int{w, h} || pos != a.lastPos {
		dst = uintptr(unsafe.Pointer(&pos))
		a.lastSize, a.lastPos = [2]int{w, h}, pos
	}
	pUpdateLayeredWindow.Call(a.hwnd, 0, dst, uintptr(unsafe.Pointer(&sz)), a.memDC,
		uintptr(unsafe.Pointer(&src)), 0, uintptr(unsafe.Pointer(&bf)), 2 /*ULW_ALPHA*/)
}

// targetPos: where the window goes for size w x h. It grows around its
// center, around the button you clicked (enlarge/shrink), from the corner
// opposite the one you drag, or glides to a corner (mini mode).
func (a *App) targetPos(w, h int) POINT {
	u := a.ui
	if u.Full {
		if mi, ok := monitorOf(a.hwnd); ok {
			return POINT{mi.RcMonitor.L, mi.RcMonitor.T}
		}
	}
	cx, cy := a.cx, a.cy
	if a.move.on {
		e := a.move.t.V
		e = e * e * (3 - 2*e)
		cx, cy = lerp(a.move.fx, a.move.tx, e), lerp(a.move.fy, a.move.ty, e)
	}
	var pos POINT
	switch {
	case a.cdrag.on:
		pos = POINT{a.cdrag.fx, a.cdrag.fy}
		if a.cdrag.corner == hResizeNW || a.cdrag.corner == hResizeSW {
			pos.X -= int32(w)
		}
		if a.cdrag.corner == hResizeNW || a.cdrag.corner == hResizeNE {
			pos.Y -= int32(h)
		}
		return pos
	case a.anchor.on:
		S := a.S()
		lx := u.PW - a.anchor.rx
		pos = POINT{int32(a.anchor.sx - lx*S + 0.5), int32(a.anchor.sy - a.anchor.ly*S + 0.5)}
	default:
		pos = POINT{int32(cx - float32(w)/2 + 0.5), int32(cy - float32(h)/2 + 0.5)}
	}
	return clampToWork(a.hwnd, pos, int32(w), int32(h))
}

// kick starts vsync-paced frames (the pump stops itself when everything settles).
func (a *App) kick() {
	if !a.visible && !a.overlaysActive() {
		return
	}
	if atomic.LoadInt32(&frameWanted) == 0 {
		a.lastFrame = time.Now()
	}
	wantFrames(true)
}

func (a *App) overlaysActive() bool {
	return a.toast.a.V > 0.01 || time.Now().Before(a.toast.until) || a.edgeWanted() || (a.edge != nil && a.edge.shown) ||
		a.pinnedPopShown() || (a.pop != nil && a.pop.shown)
}

func (a *App) frame() {
	defer a.recoverPanic()
	// The next frame is only requested once this one is done. (Before, a slow
	// frame, like fullscreen, already had the next one waiting, and waiting
	// frames go ahead of mouse and keyboard input: the app stopped responding.)
	defer atomic.StoreInt32(&framePending, 0)
	now := time.Now()
	dt := float32(now.Sub(a.lastFrame).Seconds())
	a.lastFrame = now
	if dt > 0.05 {
		dt = 0.05
	}
	a.levels = copyLevels(a.levels)
	if !a.ui.playing() {
		for i := range a.levels {
			a.levels[i] = 0
		}
	}
	a.ui.Levels = a.levels
	busy := a.stepToast(dt)
	if a.stepTip(dt) {
		busy = true
	}
	if a.stepEdge(now) {
		busy = true
	}
	if a.visible {
		if a.frameMain(now, dt) {
			busy = true
		}
	} else if a.setWin != nil && a.setWin.shown { // hidden with the player
		a.setWin.hide()
	}
	if a.stepPill(dt) {
		busy = true
	}
	if a.stepPop(dt) {
		busy = true
	}
	if !busy {
		wantFrames(false)
	}
}

// frameMain animates and draws the player. Returns true while it needs frames.
func (a *App) frameMain(now time.Time, dt float32) bool {
	u := a.ui
	u.T = float32(time.Since(a.start).Seconds())
	busy := u.Step(dt)
	if u.PoppedOut && u.Page == 1 { // Settings is in its own window: go there instead
		u.Page = 0
		u.page.Snap(0)
		a.setFocus = true
		busy = true
	}
	if a.setMarq {
		busy = true
	}
	if u.HoldFired != hNone {
		a.onHold(u.HoldFired)
		u.HoldFired = hNone
		busy = true
	}
	u.Sizing = a.sizeAnim.on || a.cdrag.on
	// enlarge / shrink steps
	if a.sizeAnim.on {
		a.sizeAnim.t = min32(1, a.sizeAnim.t+dt/0.32)
		e := a.sizeAnim.t
		e = 1 - (1-e)*(1-e)*(1-e)
		a.sizeW = lerp(a.sizeAnim.fw, a.sizeAnim.tw, e)
		a.sizeH = lerp(a.sizeAnim.fh, a.sizeAnim.th, e)
		a.applySize()
		u.SnapPanel()
		if a.sizeAnim.t >= 1 {
			a.sizeAnim.on = false
			a.settleAnchor()
			a.savePos()
		}
		busy = true
	}
	// fullscreen switches: fade out, switch, fade back in
	if a.fadeThen != nil || a.fade.V < 1 {
		target := float32(1)
		if a.fadeThen != nil {
			target = 0
		}
		a.fade.Step(target, 22, dt)
		if a.fadeThen != nil && a.fade.V <= 0.03 {
			f := a.fadeThen
			a.fadeThen = nil
			a.fade.V = 0
			f()
		}
		busy = true
	}
	if a.move.on {
		a.move.t.Step(1, 9, dt)
		if a.move.t.V >= 0.999 && !u.Resizing() {
			a.move.on = false
			a.cx, a.cy = a.move.tx, a.move.ty
		}
		busy = true
	}
	target := float32(1)
	if a.hiding {
		target = 0
	}
	// Fade when idle
	if a.idle.Step(b2f(a.idleWant), 5, dt) {
		busy = true
	}
	if a.appear.Step(target, 13, dt) {
		busy = true
	}
	if a.hiding && a.appear.V <= 0.01 {
		a.appear.V = 0
		a.hiding = false
		a.visible = false
		pShowWindow.Call(a.hwnd, SW_HIDE)
		if a.setWin != nil {
			a.setWin.hide() // the Settings window goes with it
		}
		pKillTimer.Call(a.hwnd, timerCapture)
		a.updateMeters()
		return false
	}
	if !busy && a.capSize != a.lastSize && !a.cdrag.on { // glass finished resizing: refresh the blur
		a.capSize = a.lastSize
		a.requestCapture()
	}
	// big windows can take a while to draw: never spend more than about half
	// the time drawing, so clicks and keys always get through
	slow := a.renderCost > 18*time.Millisecond && now.Sub(a.lastRender) < a.renderCost
	switch {
	case (busy || a.cdrag.on) && slow:
		return true
	case busy || a.cdrag.on:
		a.timedRender(now)
		return true
	case u.NeedsIdleFrames():
		gap := 32 * time.Millisecond
		if u.OnlyFluidAnimating() {
			gap = 48 * time.Millisecond // the fluid is slow; ~20 fps is plenty
		}
		if a.canvas != nil { // big windows: fewer idle frames
			if px := a.canvas.W * a.canvas.H; px > 1200000 {
				gap = time.Duration(float64(gap) * float64(px) / 1200000)
			}
		}
		if gap < 2*a.renderCost {
			gap = 2 * a.renderCost
		}
		if now.Sub(a.lastRender) >= gap {
			a.timedRender(now)
		}
		return true
	}
	a.timedRender(now)
	return false
}

// timedRender draws and remembers how long drawing takes.
func (a *App) timedRender(now time.Time) {
	t0 := time.Now()
	a.render()
	cost := time.Since(t0)
	a.renderCost = (a.renderCost*3 + cost) / 4
	a.lastRender = now
	// safety net: if fullscreen is too heavy for this PC, leave it instead of
	// locking up (3 frames in a row over 150 ms)
	if a.ui.Full && a.fadeThen == nil && cost > 150*time.Millisecond {
		a.slowFull++
		if a.slowFull >= 3 {
			a.slowFull = 0
			a.exitFullscreen()
			a.ui.ShowToast("Fullscreen was too slow here, so it closed")
		}
	} else {
		a.slowFull = 0
	}
}

func (a *App) show() {
	if a.visible && !a.hiding {
		pSetForegroundWindow.Call(a.hwnd)
		a.kick()
		return
	}
	if !a.visible {
		a.keepCenter()
		a.appear.V = 0
		a.ui.SnapAnimations()
		a.ui.toastTTL, a.ui.toastA.V = 0, 0
	}
	a.hiding = false
	a.visible = true
	a.updateMeters()
	a.scale = a.dpiScale()
	a.applySize()
	a.ui.SnapPanel()
	a.applyState()
	a.render()
	a.requestCapture()
	a.ui.Prewarm(a.S())
	pShowWindow.Call(a.hwnd, SW_SHOW)
	pSetForegroundWindow.Call(a.hwnd)
	pSetTimer.Call(a.hwnd, timerCapture, 90, 0)
	a.kick()
}

func (a *App) hide() {
	if !a.visible {
		return
	}
	if a.ui.RecIdx >= 0 {
		a.cancelRecording()
	}
	a.commitEdit()
	a.ui.PickOpen = false
	if a.ui.Full {
		a.ui.Full = false
		a.applySize()
		a.ui.SnapPanel()
		a.cx, a.cy = a.preFull.cx, a.preFull.cy
	}
	if !a.move.on {
		a.keepCenter()
	}
	a.hiding = true
	a.kick()
}

func (a *App) toggleVisible() {
	fg, _, _ := pGetForegroundWindow.Call()
	if a.visible && !a.hiding && fg == a.hwnd {
		a.hide()
	} else {
		a.show()
	}
}

// updateMeters: Spotify's audio levels are read while something shows them.
func (a *App) updateMeters() {
	on := (a.visible && !a.hiding) || a.edgeWanted()
	atomic.StoreInt32(&workerVisible, int32(b2i(on)))
}

// recoverPanic keeps the player alive after an unexpected error and writes
// it to %APPDATA%\AkraPlayer\crash.txt so it can be fixed.
func (a *App) recoverPanic() {
	if r := recover(); r != nil {
		logCrash(r)
	}
}

// ---------- size: steps, corners, fullscreen, mini ----------

const sizeStep = 1.15

func (a *App) workArea() RECT {
	if mi, ok := monitorOf(a.hwnd); ok {
		return mi.RcWork
	}
	var wa RECT
	pSystemParametersInfoW.Call(0x30, 0, uintptr(unsafe.Pointer(&wa)), 0)
	return wa
}

// resizeTo animates the window to w x h DIPs, keeping button id (if any)
// under the cursor.
func (a *App) resizeTo(w, h float32, id int) {
	mw, mh := a.minSize()
	wa := a.workArea()
	w = max32(mw, min32(w, float32(wa.R-wa.L)/a.scale))
	h = max32(mh, min32(h, float32(wa.B-wa.T)/a.scale))
	if !a.anchor.on && a.cfg.anchor && id != hNone && !a.move.on {
		if x, y, ok := a.buttonPos(id); ok {
			r := a.windowRect()
			S := a.S()
			a.anchor.on = true
			a.anchor.rx, a.anchor.ly = a.ui.PW-x, y
			a.anchor.sx, a.anchor.sy = float32(r.L)+x*S, float32(r.T)+y*S
		}
	}
	if !a.anchor.on {
		a.keepCenter()
	}
	a.sizeAnim.on = true
	a.sizeAnim.fw, a.sizeAnim.fh, a.sizeAnim.tw, a.sizeAnim.th = a.sizeW, a.sizeH, w, h
	a.sizeAnim.t = 0
	a.kick()
}

// buttonPos: where a header button is (logical, inside the window).
func (a *App) buttonPos(id int) (float32, float32, bool) {
	for _, h := range a.ui.hits {
		if h.id == id && h.circle {
			return h.x0, h.y0, true
		}
	}
	return 0, 0, false
}

func (a *App) settleAnchor() {
	if a.anchor.on {
		a.anchor.on = false
		a.keepCenter()
	}
}

func (a *App) enlarge() {
	u := a.ui
	if u.Full {
		u.ShowToast("Hold shrink to reset the size")
		return
	}
	if u.Mini {
		a.exitMini()
		return
	}
	tw, th := a.sizeW*sizeStep, a.sizeH*sizeStep
	if a.sizeAnim.on {
		tw, th = a.sizeAnim.tw*sizeStep, a.sizeAnim.th*sizeStep
	}
	wa := a.workArea()
	if tw*a.scale >= 0.8*float32(wa.R-wa.L) || th*a.scale >= 0.8*float32(wa.B-wa.T) {
		if a.cfg.fullscreen {
			a.enterFullscreen()
			return
		}
		// fullscreen is off (Settings > General): stop at the screen's size
		tw, th = min32(tw, float32(wa.R-wa.L)/a.scale), min32(th, float32(wa.B-wa.T)/a.scale)
		if a.sizeW >= tw-1 && a.sizeH >= th-1 {
			u.ShowToast("That's as big as it gets (fullscreen is off in Settings)")
			return
		}
	}
	a.resizeTo(tw, th, hEnlarge)
}

func (a *App) shrink() {
	u := a.ui
	if u.Full {
		a.exitFullscreen()
		return
	}
	tw, th := a.sizeW/sizeStep, a.sizeH/sizeStep
	if a.sizeAnim.on {
		tw, th = a.sizeAnim.tw/sizeStep, a.sizeAnim.th/sizeStep
	}
	mw, mh := a.minSize()
	if a.sizeW <= mw+1 && a.sizeH <= mh+1 {
		u.ShowToast("That's the smallest size")
		return
	}
	a.resizeTo(max32(tw, mw), max32(th, mh), hShrink)
}

func (a *App) onHold(id int) {
	a.holdDone = true
	switch id {
	case hEnlarge:
		switch {
		case a.ui.Full:
		case a.cfg.fullscreen:
			a.enterFullscreen()
		default: // fullscreen is off: fill the screen above the taskbar
			wa := a.workArea()
			a.resizeTo(float32(wa.R-wa.L)/a.scale, float32(wa.B-wa.T)/a.scale, hNone)
			a.keepCenter()
			a.startMove(float32(wa.L+wa.R)/2, float32(wa.T+wa.B)/2)
		}
	case hShrink: // reset the size
		dw, dh := defaultSize()
		if a.ui.Full {
			a.sizeW, a.sizeH = dw, dh
			a.exitFullscreen()
		} else {
			a.resizeTo(dw, dh, hShrink)
		}
		a.ui.ShowToast("Size reset")
	}
}

func defaultSize() (float32, float32) { return 650, 500 }

func (a *App) startMove(tx, ty float32) {
	cx, cy := a.cx, a.cy
	if a.move.on { // continue from where it is now
		e := a.move.t.V
		e = e * e * (3 - 2*e)
		cx, cy = lerp(a.move.fx, a.move.tx, e), lerp(a.move.fy, a.move.ty, e)
	}
	a.settleAnchor()
	a.move.on = true
	a.move.fx, a.move.fy, a.move.tx, a.move.ty = cx, cy, tx, ty
	a.move.t.V = 0
}

// enterFullscreen fades out, covers the screen, fades back in. (2.4 grew the
// window frame by frame at full resolution, which could stall the app.)
func (a *App) enterFullscreen() {
	u := a.ui
	if u.Full || a.fadeThen != nil {
		return
	}
	if u.Mini {
		a.exitMini()
	}
	a.sizeAnim.on = false
	a.settleAnchor()
	if !a.move.on {
		a.keepCenter()
	}
	a.preFull.cx, a.preFull.cy = a.cx, a.cy
	a.fadeThen = func() {
		u.Full = true
		a.applySize()
		u.SnapPanel()
		u.fullA.V = 1
		pSetWindowPos.Call(a.hwnd, ^uintptr(0), 0, 0, 0, 0, SWP_NOMOVE|SWP_NOSIZE|SWP_NOACTIVATE)
	}
	a.kick()
}

func (a *App) exitFullscreen() {
	u := a.ui
	if !u.Full || a.fadeThen != nil {
		return
	}
	a.fadeThen = func() {
		u.Full = false
		u.fullA.V = 0
		a.applySize()
		u.SnapPanel()
		a.cx, a.cy = a.preFull.cx, a.preFull.cy
		if !a.cfg.pinned {
			pSetWindowPos.Call(a.hwnd, ^uintptr(1), 0, 0, 0, 0, SWP_NOMOVE|SWP_NOSIZE|SWP_NOACTIVATE)
		}
		a.capSize = [2]int{}
	}
	a.kick()
}

// enterMini shrinks the player to a pill in the nearest screen corner.
func (a *App) enterMini() {
	u := a.ui
	if u.Mini {
		return
	}
	if u.Full {
		u.Full = false
		u.fullA.V = 0
		a.cx, a.cy = a.preFull.cx, a.preFull.cy
	}
	if u.RecIdx >= 0 {
		a.cancelRecording()
	}
	u.Page, u.Sheet, u.PickOpen = 0, SheetNone, false
	a.settleAnchor()
	if !a.move.on {
		a.keepCenter()
	}
	a.preMini.cx, a.preMini.cy = a.cx, a.cy
	if a.move.on {
		a.preMini.cx, a.preMini.cy = a.move.tx, a.move.ty
	}
	u.Mini = true
	a.applySize()
	wa := a.workArea()
	S := a.S()
	mw, mh := MiniW*S, MiniH*S
	m := 16 * a.scale
	tx, ty := float32(wa.R)-m-mw/2, float32(wa.B)-m-mh/2
	if a.cx < float32(wa.L+wa.R)/2 {
		tx = float32(wa.L) + m + mw/2
	}
	if a.cy < float32(wa.T+wa.B)/2 {
		ty = float32(wa.T) + m + mh/2
	}
	a.startMove(tx, ty)
	a.kick()
}

func (a *App) exitMini() {
	u := a.ui
	if !u.Mini {
		return
	}
	u.Mini = false
	a.applySize()
	a.startMove(a.preMini.cx, a.preMini.cy)
	a.kick()
}

// ---------- corner resizing ----------

func isCorner(h int) bool { return h >= hResizeNW && h <= hResizeSE }

func (a *App) startCornerDrag(h int) {
	var pt POINT
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	r := a.windowRect()
	a.settleAnchor()
	a.sizeAnim.on = false
	a.cdrag.on = true
	a.cdrag.corner = h
	a.cdrag.mx, a.cdrag.my = pt.X, pt.Y
	a.cdrag.w0, a.cdrag.h0 = a.sizeW, a.sizeH
	// the opposite corner stays where it is
	a.cdrag.fx, a.cdrag.fy = r.L, r.T
	if h == hResizeNW || h == hResizeSW {
		a.cdrag.fx = r.R
	}
	if h == hResizeNW || h == hResizeNE {
		a.cdrag.fy = r.B
	}
}

func (a *App) dragCorner() {
	var pt POINT
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	dx, dy := float32(pt.X-a.cdrag.mx)/a.scale, float32(pt.Y-a.cdrag.my)/a.scale
	switch a.cdrag.corner {
	case hResizeNW:
		dx, dy = -dx, -dy
	case hResizeNE:
		dy = -dy
	case hResizeSW:
		dx = -dx
	}
	mw, mh := a.minSize()
	mi, _ := monitorOf(a.hwnd)
	m := mi.RcWork
	a.sizeW = max32(mw, min32(a.cdrag.w0+dx, float32(m.R-m.L)/a.scale))
	a.sizeH = max32(mh, min32(a.cdrag.h0+dy, float32(m.B-m.T)/a.scale))
	a.applySize()
	a.ui.SnapPanel()
	a.kick()
}

func (a *App) endCornerDrag() {
	if !a.cdrag.on {
		return
	}
	a.cdrag.on = false
	r := a.windowRect()
	a.cx, a.cy = float32(r.L+r.R)/2, float32(r.T+r.B)/2
	a.savePos()
	a.requestCapture()
	a.kick()
}

// ---------- input ----------

func lparamXY(lp uintptr) (int32, int32) {
	return int32(int16(lp & 0xffff)), int32(int16((lp >> 16) & 0xffff))
}

func (a *App) logical(px, py int32) (float32, float32) {
	S := a.S()
	return float32(px) / S, float32(py) / S
}

func isSliderID(h int) bool { return h >= hSlider0 && h < hSlider0+SlPreset0+200 }

func isDragID(h int) bool {
	return isSliderID(h) || h == hProgress || h == hHueSV || h == hHueBar || inRange(h, hMixVol0) || h == hMiniVol
}

// drag updates whatever is being dragged.
func (a *App) drag(h int, lx, ly float32) {
	u := a.ui
	switch {
	case h == hSlider0+SlSpotVol:
		if u.IsVSlider(h) {
			a.setSpotifyVolume(u.VSliderValue(h, ly))
		}
	case h == hMiniVol:
		a.setSpotifyVolume(u.SliderValue(h, lx))
	case inRange(h, hMixVol0):
		if u.IsVSlider(h) {
			a.setMixerVolume(h-hMixVol0, u.VSliderValue(h, ly))
		}
	case isSliderID(h):
		a.setSlider(h, u.SliderValue(h, lx))
	case h == hProgress:
		u.Seeking = true
		u.SeekFrac = u.SliderValue(hProgress, lx)
		a.kick()
	case h == hHueSV || h == hHueBar:
		u.HueAt(h, lx, ly)
		a.kick()
	}
}

func (a *App) setSlider(id int, v float32) {
	p := a.cfg.p()
	pct := int(v*100 + 0.5)
	switch sl := id - hSlider0; {
	case sl == SlClarity:
		a.ui.Clarity = v
		a.cfg.clarity = v
	case sl == SlScrollRes:
		a.ui.ScrollRes = v
		a.cfg.scrollRes = v
	case sl == SlOpenRes:
		a.ui.OpenRes = v
		a.cfg.openRes = v
	case sl == SlVisSens:
		a.ui.VisSens, a.cfg.visSens = v, v
	case sl == SlVisSmooth:
		a.ui.VisSmooth, a.cfg.visSmooth = v, v
	case sl == SlVisP0 || sl == SlVisP1:
		st := a.ui.VisStyle
		a.ui.VisP[st][sl-SlVisP0] = v
		a.cfg.visP[st][sl-SlVisP0] = v
	case sl == SlIdleDelay:
		d := float32(int(1 + 14*v + 0.5)) // whole seconds
		a.ui.IdleDelay, a.cfg.idleDelay = d, d
	case sl == SlIdleOpacity:
		o := 0.05 + 0.85*v
		a.ui.IdleOpacity, a.cfg.idleOpacity = o, o
		a.previewIdle()
	case sl == SlCurVol:
		p.curPct = pct
	case sl == SlDuck:
		p.duckPct = pct
	case sl == SlSpotVol:
		a.setSpotifyVolume(v)
		return
	case sl >= SlPreset0:
		if i := sl - SlPreset0; i < len(p.presets) {
			p.presets[i].pct = pct
		}
	}
	a.syncTargets()
	a.kick()
}

func (a *App) sliderValue(id int) float32 {
	p := a.cfg.p()
	switch sl := id - hSlider0; {
	case sl == SlClarity:
		return a.ui.Clarity
	case sl == SlScrollRes:
		return a.ui.ScrollRes
	case sl == SlOpenRes:
		return a.ui.OpenRes
	case sl == SlVisSens:
		return a.ui.VisSens
	case sl == SlVisSmooth:
		return a.ui.VisSmooth
	case sl == SlVisP0 || sl == SlVisP1:
		return a.ui.VisP[a.ui.VisStyle][sl-SlVisP0]
	case sl == SlIdleDelay:
		return clamp01((a.ui.IdleDelay - 1) / 14)
	case sl == SlIdleOpacity:
		return clamp01((a.ui.IdleOpacity - 0.05) / 0.85)
	case sl == SlCurVol:
		return float32(p.curPct) / 100
	case sl == SlDuck:
		return float32(p.duckPct) / 100
	case sl == SlSpotVol:
		return a.ui.SpotVol
	case sl >= SlPreset0:
		if i := sl - SlPreset0; i < len(p.presets) {
			return float32(p.presets[i].pct) / 100
		}
	}
	return 0
}

func (a *App) click(h int) {
	u := a.ui
	if u.EditKind != EditNone && h != hEditField && h != hFoldRen0+u.EditIdx && h != hProfileRen {
		a.commitEdit()
	}
	switch {
	case h == hClose:
		a.hide()
	case h == hPin:
		a.setPinned(!u.Toggles[TogOnTop])
	case h == hPlay:
		a.spotifyCommand(APPCOMMAND_MEDIA_PLAY_PAUSE, true)
	case h == hPrev:
		a.spotifyCommand(APPCOMMAND_MEDIA_PREVIOUSTRACK, true)
	case h == hNext:
		a.spotifyCommand(APPCOMMAND_MEDIA_NEXTTRACK, true)
	case h == hMute:
		a.toggleSpotifyMute()
	case h == hHeart:
		a.likeSong()
	case h == hArtToggle:
		a.setToggle(TogArt)
	case h == hMini:
		a.enterMini()
	case h == hMiniRestore:
		a.exitMini()
	case h == hEnlarge:
		a.enlarge()
	case h == hShrink:
		a.shrink()
	case h == hColor: // presets → black → white → RGB shift → your custom color → presets
		next := 0
		switch {
		case u.AccentIdx == AccentRainbow:
			next = AccentCover
		case u.AccentIdx == AccentCover:
			next = AccentCustom
		case u.AccentIdx == len(Accents)-1:
			next = AccentRainbow
		case u.AccentIdx >= 0:
			next = u.AccentIdx + 1
		}
		a.setAccent(next)
	case h == hSwatch0+AccentBlack: // black & white: click again to switch
		if u.AccentIdx == AccentBlack {
			a.setAccent(AccentWhite)
		} else {
			a.setAccent(AccentBlack)
		}
	case h == hBgDrop:
		u.BgOpen = !u.BgOpen
	case inRange(h, hTogStyle0) && h-hTogStyle0 < 3:
		u.TogStyle = h - hTogStyle0
		a.cfg.togStyle = u.TogStyle
		saveConfig(a.cfg)
	case inRange(h, hToastPos0) && h-hToastPos0 < len(toastPosNames):
		u.ToastPos = h - hToastPos0
		a.cfg.toastPos = u.ToastPos
		saveConfig(a.cfg)
		a.fromKey = true // show where it'll appear
		a.keyToast("akra", "Popup", toastPosNames[u.ToastPos])
		a.fromKey = false
	case h == hSettings:
		if a.setOpen {
			a.setFocus = true // already in its own window: bring it forward
		} else {
			u.Page = 1
		}
	case h == hSetPop:
		a.popOutSettings()
	case h == hControls:
		a.openPop(PopMixer)
	case inRange(h, hMixIcon0):
		a.toggleMixerMute(h - hMixIcon0)
	case h == hDeclutter || h == hShowAll:
		a.setDeclutter(h == hDeclutter)
	case inRange(h, hBindOn0) && h-hBindOn0 < NumFixed:
		act := h - hBindOn0
		a.setBindOn(act, a.cfg.p().off[act])
	case inRange(h, hDeclScope0) && h-hDeclScope0 < 2:
		a.cfg.declLocal = h-hDeclScope0 == 1
		u.DeclLocal = a.cfg.declLocal
		a.registerAll()
		saveConfig(a.cfg)
	case h == hResetDefaults:
		if u.ResetArmed > 0 {
			u.ResetArmed = 0
			a.resetDefaults()
		} else {
			u.ResetArmed = 3 // ask once more
		}
	case h == hAddToggle:
		a.openAppPicker(len(a.cfg.p().presets))
	case h == hKeysSheet:
		a.openPop(PopKeys)
	case h == hSheetClose:
		u.Sheet = SheetNone
	case h == hEditKeys:
		a.closePop()
		u.Sheet = SheetNone
		u.Section = SecKeys
		u.Page = 1
		a.show() // (the player may be hidden behind a pinned pop-out)
	case h == hBack:
		if u.RecIdx >= 0 {
			a.cancelRecording()
		}
		u.PickOpen = false
		u.Page = 0
	case h == hTrack:
		if u.Mini {
			a.exitMini()
		} else if a.spot != 0 {
			focusWindow(a.spot)
		} else {
			launchSpotify()
			u.ShowToast("Opening Spotify…")
		}
	case h == hChipPlay:
		a.openWindowPicker()
	case h == hDuckChip:
		a.openAppPicker(-1)
	case h == hAddPreset:
		a.openAppPicker(len(a.cfg.p().presets))
	case inRange(h, hPresetChip0):
		a.openAppPicker(h - hPresetChip0)
	case inRange(h, hPresetDel0):
		a.deletePreset(h - hPresetDel0)
	case h == hPickBg:
		u.PickOpen = false
	case inRange(h, hPick0):
		a.choosePicked(h - hPick0)
	case inRange(h, hNav0) && h-hNav0 < NumSections:
		if h-hNav0 != u.Section {
			u.secScroll[h-hNav0].Pos.Snap(0) // a category you click starts at the top
		} else {
			u.secScroll[h-hNav0].Pos.Target = 0 // already there: glide back up
		}
		u.secScroll[h-hNav0].Over = 0
		u.Section = h - hNav0
		u.secScroll[u.Section].Over = 0
	case h >= hBind0 && h < hMove0:
		act := h - hBind0
		if u.RecIdx == act {
			a.cancelRecording()
		} else {
			if u.RecIdx >= 0 {
				a.stopRecording()
			}
			a.startRecording(act)
		}
	case h >= hMove0 && h < hFold0:
		a.openFolderPicker(h - hMove0)
	case inRange(h, hFold0):
		i := h - hFold0
		if f := a.cfg.p().folders; i < len(f) {
			f[i].Collapsed = !f[i].Collapsed
			a.syncTargets()
			saveConfig(a.cfg)
		}
	case inRange(h, hFoldRen0):
		i := h - hFoldRen0
		if u.EditKind == EditFolder && u.EditIdx == i {
			a.commitEdit()
		} else if i < len(a.cfg.p().folders) {
			a.commitEdit()
			a.startEdit(EditFolder, i)
		}
	case inRange(h, hFoldDel0):
		a.deleteFolder(h - hFoldDel0)
	case h == hNewFolder:
		a.newFolder(true)
	case inRange(h, hProfile0):
		a.switchProfile(h - hProfile0)
	case h == hProfileAdd:
		a.addProfile()
	case h == hProfileRen:
		if u.EditKind == EditProfile {
			a.commitEdit()
		} else {
			a.commitEdit()
			a.startEdit(EditProfile, a.cfg.prof)
		}
	case h == hProfileDel:
		a.deleteProfile()
	case inRange(h, hLook0):
		a.switchLook(h - hLook0)
	case h == hLookAdd:
		a.addLook()
	case h == hLookRen:
		if u.EditKind == EditLook {
			a.commitEdit()
		} else if a.cfg.looks[a.cfg.lookIdx].builtin == lookMine {
			a.commitEdit()
			a.startEdit(EditLook, a.cfg.lookIdx)
		}
	case h == hLookDel:
		a.deleteLook()
	case h == hLookRestore:
		a.restoreLook()
	case h == hExport:
		a.openExport()
	case h == hExpChange:
		a.pickFolder("Choose where to save your Akra Player settings", 1)
	case h == hExpOpen:
		if u.ExpDone != "" {
			openFolder(u.ExpDone)
		} else {
			openFolder(a.expDir)
		}
	case inRange(h, hImp0) && h-hImp0 < len(u.ImpList):
		a.selectImport(h - hImp0)
	case inRange(h, hImpInc0) && h-hImpInc0 < NumExp:
		u.ImpInc[h-hImpInc0] = !u.ImpInc[h-hImpInc0]
	case h == hImpBrowse:
		a.pickFolder("Choose a folder of exported Akra Player settings", 2)
	case h == hImpGo:
		a.startImport()
	case inRange(h, hExpInc0) && h-hExpInc0 < NumExp:
		u.ExpInc[h-hExpInc0] = !u.ExpInc[h-hExpInc0]
	case h == hExpGo:
		a.startExport()
	case h == hExpCancel:
		u.ExpOpen = false
		if u.Pop == PopExport || u.Pop == PopImport {
			a.closePop()
		}
	case h == hImport:
		a.openImport()
	case inRange(h, hTog0) && h-hTog0 < NumToggles:
		a.setToggle(h - hTog0)
	case inRange(h, hSwatch0) && h-hSwatch0 < len(Accents):
		a.setAccent(h - hSwatch0)
	case inRange(h, hTogEdit0) && h-hTogEdit0 < NumToggles:
		if u.TogEdit == h-hTogEdit0 {
			u.TogEdit = -1
		} else {
			u.TogEdit = h - hTogEdit0
			u.Monitors = monitorNames()
		}
	case inRange(h, hSongMon0) && h-hSongMon0 <= len(u.Monitors):
		u.SongMon = h - hSongMon0 - 1
		a.cfg.songMon = u.SongMon
		saveConfig(a.cfg)
		a.songPopupSample()
	case inRange(h, hSongPos0) && h-hSongPos0 < len(toastPosNames):
		u.SongPos = h - hSongPos0
		a.cfg.songPos = u.SongPos
		saveConfig(a.cfg)
		a.songPopupSample()
	case inRange(h, hWaveMode0) && h-hWaveMode0 < NumWaveModes:
		u.WaveMode = h - hWaveMode0
		a.cfg.waveMode = u.WaveMode
		saveConfig(a.cfg)
		if u.WaveMode == WaveCustom || u.WaveMode == WaveGradient {
			u.OpenHue(HueWave)
		}
	case h == hWavePick:
		u.OpenHue(HueWave)
	case inRange(h, hEdgeStyle0) && h-hEdgeStyle0 <= EdgeWave:
		u.EdgeStyle = h - hEdgeStyle0
		a.cfg.edgeStyle = u.EdgeStyle
		saveConfig(a.cfg)
	case h == hSwatchCover:
		a.setAccent(AccentCover)
	case h == hSleepChip:
		u.Page, u.Section = 1, SecGeneral
	case h == hTimeRight:
		u.Remaining = !u.Remaining
		a.cfg.remaining = u.Remaining
		saveConfig(a.cfg)
	case inRange(h, hSleep0) && h-hSleep0 < NumSleep:
		a.setSleep(h - hSleep0)
	case h == hSwatchRainbow:
		a.setAccent(AccentRainbow)
	case h == hSwatchCustom:
		u.OpenHue(HueAccent)
	case h == hVisShow || h == hVisHide:
		if u.Toggles[TogVis] != (h == hVisShow) {
			a.setToggle(TogVis)
		}
	case inRange(h, hVisStyle0) && h-hVisStyle0 < NumVisStyles:
		a.setVisStyle(h - hVisStyle0)
	case inRange(h, hVisMon0):
		a.setVisMonitor(h - hVisMon0)
	case h == hFluidPick:
		u.OpenHue(HueFluid)
	case h == hHueDone:
		u.HueOpen = false
		if u.HueFor == HueFluid {
			u.FluidMode, u.FluidCol = FluidCustom, u.hueColor()
			a.cfg.fluidMode, a.cfg.fluidCol = FluidCustom, colToRGB(u.FluidCol)
			saveConfig(a.cfg)
		} else if u.HueFor == HueWave {
			u.WaveCol = u.hueColor()
			a.cfg.waveCol = colToRGB(u.WaveCol)
			saveConfig(a.cfg)
		} else {
			a.cfg.custom = colToRGB(u.Custom)
			a.setAccent(AccentCustom)
		}
	case h == hHueTheme: // fluid follows the theme again
		u.HueOpen = false
		u.FluidMode = FluidTheme
		a.cfg.fluidMode = FluidTheme
		saveConfig(a.cfg)
	case h == hHueArt:
		u.HueOpen = false
		u.FluidMode = FluidArt
		a.cfg.fluidMode = FluidArt
		saveConfig(a.cfg)
		if u.Art == nil {
			u.ShowToast("Uses the cover once a song with art is playing")
		}
	case h == hHueCancel || h == hHueBg:
		u.CancelHue()
	case inRange(h, hFont0) && h-hFont0 < NumFonts:
		a.setFont(h - hFont0)
	}
	a.kick()
}

func wndProc(hwnd, msg, wp, lp uintptr) uintptr {
	a := &app
	defer a.recoverPanic()
	if a.hwnd == 0 {
		a.hwnd = hwnd
	}
	u := a.ui
	switch msg {
	case WM_NCHITTEST:
		x, y := lparamXY(lp)
		r := a.windowRect()
		lx, ly := a.logical(x-r.L, y-r.T)
		if u.Hit(lx, ly) != hNone {
			return HTCLIENT
		}
		if u.InPanel(lx, ly) && !u.Full {
			return HTCAPTION // drag the glass anywhere
		}
		if u.Full {
			return HTCLIENT
		}
		return HTNOWHERE

	case WM_NCMOUSEMOVE:
		if u.Hover != hNone {
			u.Hover = hNone
			a.kick()
		}

	case WM_MOUSEMOVE:
		a.lastInput = time.Now()
		if a.idleWant { // back from idle right away
			a.idleWant = false
			a.kick()
		}
		x, y := lparamXY(lp)
		lx, ly := a.logical(x, y)
		if a.cdrag.on {
			a.dragCorner()
			return 0
		}
		if isDragID(u.Press) {
			a.drag(u.Press, lx, ly)
			return 0
		}
		if !a.tracking {
			tme := TRACKMOUSEEVENT{DwFlags: 2 /*TME_LEAVE*/, Hwnd: hwnd}
			tme.CbSize = uint32(unsafe.Sizeof(tme))
			pTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tme)))
			a.tracking = true
		}
		if h := u.Hit(lx, ly); h != u.Hover {
			u.Hover = h
			a.kick()
		}
		a.tipFor = 0
		a.trackTip(u.TipAt(lx, ly))
		return 0

	case WM_MOUSELEAVE:
		a.tracking = false
		a.trackTip("")
		if u.Press == hNone && u.Hover != hNone {
			u.Hover = hNone
			a.kick()
		}
		return 0

	case WM_LBUTTONDOWN:
		x, y := lparamXY(lp)
		lx, ly := a.logical(x, y)
		h := u.Hit(lx, ly)
		if isCorner(h) {
			pSetCapture.Call(hwnd)
			a.startCornerDrag(h)
			u.Press = h
			return 0
		}
		u.Press = h
		u.FocusSlider = hNone
		if isSliderID(h) {
			u.FocusSlider = h // the wheel may adjust it now
		}
		if h == hEnlarge || h == hShrink {
			u.HoldID, u.HoldT = h, 0 // hold 3 s: fullscreen / reset
			a.holdDone = false
		}
		pSetCapture.Call(hwnd)
		if isDragID(h) {
			a.drag(h, lx, ly)
		}
		a.kick()
		return 0

	case WM_LBUTTONUP:
		pReleaseCapture.Call()
		if a.cdrag.on {
			a.endCornerDrag()
			u.Press = hNone
			return 0
		}
		x, y := lparamXY(lp)
		lx, ly := a.logical(x, y)
		h := u.Hit(lx, ly)
		p := u.Press
		held := u.HoldT
		u.Press = hNone
		u.Hover = h
		switch {
		case isSliderID(p):
			saveConfig(a.cfg)
		case p == hProgress:
			u.Seeking = false
			a.seek(u.SeekFrac * u.Dur)
		case p == hHueSV || p == hHueBar:
		case (p == hEnlarge || p == hShrink) && (a.holdDone || held > 0.3):
			a.holdDone = false // that was a hold, not a click
		case p == h && h != hNone:
			a.click(h)
		}
		a.kick()
		return 0

	case WM_SETCURSOR:
		corner := u.Hover
		if a.cdrag.on {
			corner = a.cdrag.corner
		}
		if lp&0xffff == HTCLIENT && isCorner(corner) {
			id := uintptr(32642) // IDC_SIZENWSE
			if corner == hResizeNE || corner == hResizeSW {
				id = 32643 // IDC_SIZENESW
			}
			cur, _, _ := pLoadCursorW.Call(0, id)
			pSetCursor.Call(cur)
			return 1
		}
		if lp&0xffff == HTCLIENT && u.Hover != hNone {
			pSetCursor.Call(a.handCur)
			return 1
		}

	case WM_MOUSEWHEEL:
		var pt POINT
		pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
		r := a.windowRect()
		lx, ly := a.logical(pt.X-r.L, pt.Y-r.T)
		h := u.Hit(lx, ly)
		notches := float32(int16(wp>>16)) / 120
		switch {
		case h == hMute || h == hSpotVol || h == hSlider0+SlSpotVol || h == hMiniVol: // Spotify volume
			a.setSpotifyVolume(u.SpotVol + notches*0.05)
			return 0
		case inRange(h, hMixVol0) || inRange(h, hMixIcon0): // an app in the mixer
			i := h - hMixVol0
			if inRange(h, hMixIcon0) {
				i = h - hMixIcon0
			}
			if i < len(u.MixerApps) {
				a.setMixerVolume(i, u.MixerApps[i].Vol+notches*0.05)
			}
			return 0
		case isSliderID(h) && h == u.FocusSlider: // sliders only take the wheel after you've clicked them
			a.setSlider(h, clamp01(a.sliderValue(h)+notches*0.05))
			saveConfig(a.cfg)
			return 0
		}
		if u.Wheel(-notches * 48) {
			a.kick()
		}
		return 0

	case WM_KEYDOWN, WM_SYSKEYDOWN:
		vk := uint32(wp)
		if u.RecIdx >= 0 || u.EditKind != EditNone {
			return 0 // the hook / WM_CHAR handle these
		}
		if a.localKey(vk) {
			return 0
		}
		switch vk {
		case VK_ESCAPE:
			switch {
			case u.HueOpen:
				u.CancelHue()
			case u.PickOpen:
				u.PickOpen = false
			case u.Sheet != SheetNone:
				u.Sheet = SheetNone
			case u.Page == 1:
				u.Page = 0
			case u.Full:
				a.exitFullscreen()
			case u.Mini:
				a.exitMini()
			default:
				a.hide()
			}
			a.kick()
		case VK_SPACE:
			a.spotifyCommand(APPCOMMAND_MEDIA_PLAY_PAUSE, true)
		case VK_LEFT:
			a.spotifyCommand(APPCOMMAND_MEDIA_PREVIOUSTRACK, true)
		case VK_RIGHT:
			a.spotifyCommand(APPCOMMAND_MEDIA_NEXTTRACK, true)
		case VK_UP, VK_DOWN: // Spotify's volume, on the player page
			if u.Page == 0 && u.Sheet == SheetNone && !u.PickOpen {
				step := float32(0.05)
				if vk == VK_DOWN {
					step = -step
				}
				a.setSpotifyVolume(u.SpotVol + step)
				u.ShowToast(fmt.Sprintf("Spotify volume %d%%", int(clamp01(u.SpotVol+0.0001)*100+0.5)))
			}
		case 'M':
			if u.Page == 0 && u.Sheet == SheetNone && !u.PickOpen && asyncMods() == 0 {
				a.toggleSpotifyMute()
			}
		}
		return 0

	case WM_CHAR:
		if u.EditKind != EditNone {
			a.onChar(rune(wp))
		}
		return 0

	case WM_SYSCHAR:
		return 0

	case WM_HOTKEY:
		if act := int(wp) - 1; act >= 0 {
			a.runAction(act, 0)
		}
		return 0

	case WM_HOOKHOT:
		a.runAction(int(wp), int(lp))
		return 0

	case WM_RECKEY:
		a.onRecordKey(uint32(wp), uint32(lp))
		return 0

	case WM_COPYDATA:
		a.onCopyData(wp, readCopyData(lp))
		return 1

	case WM_CAPGRAB:
		a.onCapGrab(int(wp))
		return 0

	case WM_LIKED:
		a.liking = false
		return 0

	case WM_FRAME:
		a.frame()
		return 0

	case WM_ART:
		a.onArt()
		return 0

	case WM_POLL:
		a.onPoll()
		return 0

	case WM_BASE:
		a.onBase()
		return 0

	case WM_MEDIA:
		if wp == 2 { // a play/pause request got no answer in time
			if p := a.pendingToggle; p != nil && time.Now().After(p.until) {
				a.toggleFallback()
			}
			return 0
		}
		a.onMedia()
		return 0

	case WM_MOVE:
		// keep the blur in sync while the glass is dragged: redraw right away
		// from the captured margin; grab again only when it runs out
		if a.visible && !a.hiding && a.appear.V >= 1 {
			a.kick()
			r := a.windowRect()
			c := a.capArea
			if !u.Resizing() && !a.move.on && !a.sizeAnim.on && !a.cdrag.on &&
				(r.L < c.L || r.T < c.T || r.R > c.R || r.B > c.B) {
				a.requestCapture()
			}
		}

	case WM_FOLDERPICKED:
		a.onFolderPicked()
		return 0

	case WM_TIMER:
		switch wp {
		case timerCapture:
			// often while something behind is changing (a video), rarely when not
			gap := 1200 * time.Millisecond
			if time.Since(a.lastChange) < 3*time.Second {
				gap = 180 * time.Millisecond
			}
			if time.Since(a.lastCap) >= gap {
				a.requestCapture()
			}
			a.checkIdle()
			// is the chosen play/pause window still open? (only while it's shown)
			if a.visible && (u.Page == 1 || a.setOpen || a.pop != nil || u.Sheet != SheetNone) && time.Since(a.lastPlayCheck) > 1500*time.Millisecond {
				a.lastPlayCheck = time.Now()
				a.checkPlayTarget()
			}
		case timerRecord: // no remapper replaced the key: keep the one pressed
			pKillTimer.Call(hwnd, timerRecord)
			if p := a.recPending; p != nil {
				a.recPending = nil
				a.commitRecord(*p)
			}
		case timerTip:
			pKillTimer.Call(hwnd, timerTip)
			a.showTip()
		case timerSleep:
			a.stepSleep()
		}
		return 0

	case WM_TRAY:
		switch lp & 0xffff {
		case WM_LBUTTONUP:
			if a.visible && !a.hiding {
				a.hide()
			} else {
				a.show()
			}
		case WM_RBUTTONUP, WM_CONTEXTMENU:
			a.trayMenu()
		}
		return 0

	case WM_SHOWME:
		a.show()
		return 0

	case WM_ACTIVATE:
		toPop := (a.pop != nil && lp == a.pop.hwnd) || (a.setWin != nil && lp == a.setWin.hwnd) // our own windows
		if wp&0xffff == 0 && !toPop {                                                           // lost focus
			if u.RecIdx >= 0 {
				a.cancelRecording()
			}
			if u.EditKind != EditNone {
				a.commitEdit()
				a.kick()
			}
		}

	case WM_DPICHANGED:
		a.scale = float32(wp&0xffff) / 96
		a.applySize()
		u.SnapPanel()
		r := (*RECT)(unsafe.Pointer(lp))
		a.cx, a.cy = float32(r.L+r.R)/2, float32(r.T+r.B)/2
		a.lastSize = [2]int{}
		a.render()
		a.requestCapture()
		return 0

	case WM_EXITSIZEMOVE:
		a.keepCenter()
		if !u.Mini {
			a.savePos()
		}
		if s := a.dpiScale(); s != a.scale {
			a.scale = s
			a.lastSize = [2]int{}
		}
		a.render()
		a.requestCapture()

	case WM_CLOSE:
		if u.RecIdx < 0 {
			a.hide()
		}
		return 0

	case WM_DESTROY:
		a.removeTray()
		pPostQuitMessage.Call(0)
		return 0

	default:
		if a.tbCreated != 0 && msg == a.tbCreated {
			a.addTray()
			return 0
		}
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, msg, wp, lp)
	return r
}

// savePos stores the normal-size window position (top-left), whatever size it is now.
func (a *App) savePos() {
	w, h := a.sizeW*a.scale, a.sizeH*a.scale
	a.cfg.x, a.cfg.y, a.cfg.hasPos = int32(a.cx-w/2), int32(a.cy-h/2), true
	a.cfg.winW, a.cfg.winH = a.sizeW, a.sizeH
	saveConfig(a.cfg)
}

func colToRGB(c Col) uint32 {
	r8 := func(v float32) uint32 { return uint32(clamp01(v)*255 + 0.5) }
	return r8(c.R)<<16 | r8(c.G)<<8 | r8(c.B)
}

var (
	pMonitorFromWindow = user32.NewProc("MonitorFromWindow")
	pGetMonitorInfoW   = user32.NewProc("GetMonitorInfoW")
)

type MONITORINFO struct {
	CbSize    uint32
	RcMonitor RECT
	RcWork    RECT
	DwFlags   uint32
}

// clampToWork keeps a window of size w x h inside its monitor's work area.
func clampToWork(hwnd uintptr, p POINT, w, h int32) POINT {
	mi, ok := monitorOf(hwnd)
	if !ok {
		return p
	}
	return clampTo(mi.RcWork, p, w, h)
}

func clampTo(wa RECT, p POINT, w, h int32) POINT {
	if p.X+w > wa.R {
		p.X = wa.R - w
	}
	if p.Y+h > wa.B {
		p.Y = wa.B - h
	}
	if p.X < wa.L {
		p.X = wa.L
	}
	if p.Y < wa.T {
		p.Y = wa.T
	}
	return p
}

// resetDefaults puts every setting back to how it came. Keybinds, profiles
// and volume toggles are kept, and so is where the window is.
func (a *App) resetDefaults() {
	old := a.cfg
	d := defaultConfig()
	d.profiles, d.prof = old.profiles, old.prof
	// appearance profiles: yours stay, Default and Glass go back to how they came
	for _, l := range old.looks {
		if l.builtin != lookMine {
			l = presetLook(l.builtin)
		}
		d.looks = append(d.looks, l)
	}
	fixLooks(&d, false)
	for i, l := range d.looks {
		if l.builtin == lookDefault {
			d.lookIdx = i
			d.applyLook(l)
		}
	}
	d.x, d.y, d.hasPos = old.x, old.y, old.hasPos
	a.cfg = d
	u := a.ui
	startup := u.Toggles[TogStartup]
	a.applyConfigToUI()
	u.Toggles[TogStartup] = startup
	a.setPinned(false)
	u.Page, u.Section = 1, SecGeneral
	u.SnapAnimations()
	if u.Full {
		a.exitFullscreen()
	}
	a.resizeTo(d.winW, d.winH, hNone)
	a.refreshTrayIcon()
	a.updateMeters()
	a.requestCapture()
	saveConfig(a.cfg)
	u.ShowToast("Settings reset to default")
	a.kick()
}
