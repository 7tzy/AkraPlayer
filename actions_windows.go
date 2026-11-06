//go:build windows

package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// ---------- Spotify ----------

// onPoll takes the newest snapshot from the poll worker.
func (a *App) onPoll() {
	for {
		select {
		case sn := <-pollCh:
			a.snap = sn
		default:
			a.applyState()
			return
		}
	}
}

func (a *App) applyMedia() { a.applyState() }

// applyState merges what the poll worker and the media helper know into the UI.
func (a *App) applyState() {
	now := time.Now()
	dt := float32(now.Sub(a.lastPoll).Seconds())
	if a.lastPoll.IsZero() || dt > 5 {
		dt = 0
	}
	a.lastPoll = now
	uiNow := float32(time.Since(a.start).Seconds())

	sn := a.snap
	if (a.spot == 0) != (sn.hwnd == 0) {
		defer a.updateMeters() // Spotify opened or closed
	}
	a.spot = sn.hwnd
	st, track, artist := StNotRunning, "", ""
	if sn.hwnd != 0 {
		st, track, artist = parseTitle(sn.title)
	}
	live := a.mediaLive() && sn.hwnd != 0
	ml := a.med.last
	if live && st != StAd {
		switch ml.Status {
		case 4:
			st = StPlaying
		case 5:
			st = StPaused
		}
		if ml.Title != "" {
			track, artist = ml.Title, ml.Artist
		}
	}
	switch st {
	case StPlaying:
		a.lastTrack, a.lastArtist = track, artist
	case StPaused:
		if track == "" {
			track, artist = a.lastTrack, a.lastArtist
		}
	case StNotRunning:
		a.lastTrack, a.lastArtist = "", ""
	}
	u := a.ui
	changed := st != u.Status || track != u.Title || artist != u.Artist
	u.Status, u.Title, u.Artist = st, track, artist

	key := artist + "|" + track
	if track != "" && key != a.trackKey && st != StAd {
		a.onSongChange(a.trackKey, key, track, artist, st)
		a.progKnown = a.trackKey != "" && st == StPlaying // saw the previous song end
		a.trackKey, a.keySince = key, now
		a.elapsed = 0.35
		if !live {
			a.requestArt(artist, track)
		}
		changed = true
	} else if st == StPlaying {
		a.elapsed += dt
	}
	if st == StNotRunning {
		a.trackKey, a.progKnown = "", false
	}
	if l := a.liked[a.trackKey]; l != u.Liked {
		u.Liked = l
		changed = true
	}

	// progress: exact from Windows media controls, else an estimate
	if live && ml.End > 0 {
		if now.After(a.med.holdTo) && !u.Seeking {
			pos := float32(ml.Pos)
			if ml.Status == 4 {
				pos += float32(now.Sub(a.med.at).Seconds())
			}
			u.Pos, u.PosT = pos, uiNow
		}
		u.Dur, u.ProgKnown, u.Seekable = float32(ml.End), true, true
	} else {
		u.Pos, u.PosT, u.ProgKnown, u.Seekable = a.elapsed, uiNow, a.progKnown, false
		u.Dur = 0
		if r, ok := a.arts[a.trackKey]; ok {
			u.Dur = r.dur
		}
	}

	// cover: the real one from Spotify if we have it, else looked up
	var img *Canvas
	if live {
		img = a.med.art[key]
		if img == nil && now.Sub(a.keySince) > 2500*time.Millisecond {
			a.requestArt(artist, track)
		}
	}
	if img == nil {
		img = a.arts[a.trackKey].img
	}
	if u.Art != img {
		u.Art = img
		u.ArtVer++
		changed = true
		if a.cfg.trayAccent && u.AccentIdx == AccentCover {
			a.refreshTrayIcon() // the tray follows the cover's color
		}
	}

	if sn.mutedKnown && sn.muted != u.SpotMuted {
		u.SpotMuted = sn.muted
		changed = true
	}
	if sn.volKnown && now.After(a.spotVolHold) && abs32(sn.vol-u.SpotVol) > 0.004 {
		u.SpotVol = sn.vol
		changed = true
	}
	for act, till := range a.noteTill {
		if u.BindNote[act] != "" && now.After(till) {
			u.BindNote[act] = ""
			delete(a.noteTill, act)
			changed = true
		}
	}
	if u.Pop == PopMixer && (a.visible || a.pinnedPopShown()) && now.Sub(a.mixAt) > time.Second {
		a.mixAt = now
		a.refreshMixer() // keep the mixer live while it's open
	}
	if changed {
		a.updateTrayTip()
		a.kick()
		a.kickPill()
	} else if a.visible && u.NeedsIdleFrames() {
		a.kick()
	}
}

type artResult struct {
	img *Canvas
	dur float32
}

type artMsg struct {
	key string
	res artResult
}

var artCh = make(chan artMsg, 8)

func (a *App) requestArt(artist, track string) {
	key := artist + "|" + track
	if !a.cfg.art || a.arts[key] != (artResult{}) || a.artPending[key] {
		return
	}
	a.artPending[key] = true
	hwnd := a.hwnd
	go func() {
		img, dur, _ := FetchArt(artist, track)
		artCh <- artMsg{key, artResult{img, dur}}
		pPostMessageW.Call(hwnd, WM_ART, 0, 0)
	}()
}

func (a *App) onArt() {
	for {
		select {
		case m := <-artCh:
			delete(a.artPending, m.key)
			a.arts[m.key] = m.res
		default:
			a.applyState()
			return
		}
	}
}

func (a *App) spotifyCommand(cmd uintptr, fromUI bool) {
	if a.spot == 0 {
		if fromUI && cmd == APPCOMMAND_MEDIA_PLAY_PAUSE {
			launchSpotify() // never from a hotkey: it would pull you out of a game
			a.ui.ShowToast("Opening Spotify…")
			a.kick()
		}
		return
	}
	sendAppCommand(a.spot, cmd)
	if cmd == APPCOMMAND_MEDIA_PLAY_PAUSE {
		switch a.ui.Status {
		case StPlaying:
			a.ui.Status = StPaused
		case StPaused:
			a.ui.Status = StPlaying
		}
	}
	a.kick()
	a.kickPill()
}

func (a *App) toggleSpotifyMute() {
	m, found := ToggleAppMute("Spotify.exe")
	switch {
	case !found:
		a.ui.ShowToast("Spotify isn't playing audio")
	case m:
		a.ui.SpotMuted = true
		a.ui.ShowToast("Spotify muted")
	default:
		a.ui.SpotMuted = false
		a.ui.ShowToast("Spotify unmuted")
	}
	a.kick()
}

// setSpotifyVolume: Spotify's own volume (its slider in the Windows mixer).
func (a *App) setSpotifyVolume(v float32) {
	v = clamp01(v)
	a.ui.SpotVol = v
	a.spotVolHold = time.Now().Add(1200 * time.Millisecond)
	SetAppVolume("Spotify.exe", int(v*100+0.5))
	if a.ui.SpotMuted && v > 0 {
		if m, found := ToggleAppMute("Spotify.exe"); found {
			a.ui.SpotMuted = m
		}
	}
	a.kick()
}

// notify: from a hotkey it's the corner popup (if on); in the app it's the
// in-window toast.
func (a *App) notify(iconKey, name, value, sentence string) {
	if a.fromKey {
		if a.cfg.toasts {
			a.keyToast(iconKey, name, value)
			return
		}
	}
	a.ui.ShowToast(sentence)
	a.kick()
}

func isSelf(exe string) bool {
	self, _ := os.Executable()
	return strings.EqualFold(exe, baseName(self))
}

func (a *App) muteForeground() {
	path := ForegroundPath()
	exe := baseName(path)
	if exe == "" {
		return
	}
	if isSelf(exe) {
		a.notify("akra", "Mute", "click a window first", "Click the window you want to mute first")
		return
	}
	m, found := ToggleAppMute(exe)
	name := prettyApp(exe)
	switch {
	case !found:
		a.notify(path, name, "no sound", name+" isn't playing audio")
	case m:
		a.notify(path, name, "Muted", "Muted "+name)
	default:
		a.notify(path, name, "Unmuted", "Unmuted "+name)
	}
	if strings.EqualFold(exe, "Spotify.exe") && found {
		a.ui.SpotMuted = m
	}
	a.kick()
}

// ---------- windows ----------

type winInfo struct {
	hwnd               uintptr
	title, class, path string
}

var (
	winList   []winInfo
	winEnumCB = syscall.NewCallback(winEnumProc)
	selfHwnd  uintptr
)

func winEnumProc(hwnd, _ uintptr) uintptr {
	if hwnd == selfHwnd {
		return 1
	}
	if v, _, _ := pIsWindowVisible.Call(hwnd); v == 0 {
		return 1
	}
	if owner, _, _ := pGetWindow.Call(hwnd, 4 /*GW_OWNER*/); owner != 0 {
		return 1
	}
	ex, _, _ := pGetWindowLongPtrW.Call(hwnd, ^uintptr(19) /*GWL_EXSTYLE = -20*/)
	if ex&WS_EX_TOOLWINDOW != 0 {
		return 1
	}
	var cloaked uint32
	pDwmGetWindowAttribute.Call(hwnd, 14 /*DWMWA_CLOAKED*/, uintptr(unsafe.Pointer(&cloaked)), 4)
	if cloaked != 0 {
		return 1
	}
	title := windowText(hwnd)
	if title == "" {
		return 1
	}
	class := className(hwnd)
	switch class {
	case "Progman", "WorkerW", "Shell_TrayWnd", "Shell_SecondaryTrayWnd":
		return 1
	}
	var pid uint32
	pGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	winList = append(winList, winInfo{hwnd, title, class, freshProcessPath(pid)})
	return 1
}

func listWindows() []winInfo {
	winList = nil
	pEnumWindows.Call(winEnumCB, 0)
	return winList
}

func windowPath(hwnd uintptr) string {
	var pid uint32
	pGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	return freshProcessPath(pid)
}

// resolvePlayWindow finds the chosen window again (it may have been reopened).
func (a *App) resolvePlayWindow() uintptr {
	t := a.cfg.p().play
	if t.path == "" {
		return 0
	}
	exe := baseName(t.path)
	if a.playHwnd != 0 {
		if ok, _, _ := pIsWindow.Call(a.playHwnd); ok != 0 && strings.EqualFold(baseName(windowPath(a.playHwnd)), exe) {
			return a.playHwnd
		}
	}
	best, bestScore := uintptr(0), 0
	for _, w := range listWindows() {
		if !strings.EqualFold(baseName(w.path), exe) {
			continue
		}
		score := 1
		if t.class != "" && w.class == t.class {
			score += 2
		}
		if t.title != "" && w.title == t.title {
			score += 3
		}
		if score > bestScore {
			best, bestScore = w.hwnd, score
		}
	}
	a.playHwnd = best
	return best
}

// ---------- play / pause any window ----------
//
// 2.3 sent the window a "media key" message. Apps that don't handle it pass
// it on to Windows, which hands it to the active media app (Spotify), and VLC
// could crash on it. Now each app is reached the way it actually listens:
//  1. Windows' media controls, aimed at that app's own session (Chrome,
//     Brave, Edge, Firefox, Windows Media Player, most modern players)
//  2. the browser extension, for browser tabs
//  3. the Space key sent straight to the window, for players where Space
//     means play/pause (VLC, MPC, PotPlayer, mpv)

type togglePending struct {
	exe, name, path string
	hwnd            uintptr
	fromKey         bool
	until           time.Time
}

func sessionName(exe string) string {
	return strings.TrimSuffix(strings.ToLower(exe), ".exe")
}

func spaceIsPlayPause(exe string) bool {
	switch sessionName(exe) {
	case "vlc", "mpc-hc", "mpc-hc64", "mpc-be", "mpc-be64", "potplayer", "potplayermini", "potplayermini64", "potplayer64", "mpv", "smplayer", "kmplayer", "gom":
		return true
	}
	return false
}

func (a *App) playPauseTarget() {
	t := a.cfg.p().play
	if t.path == "" {
		a.notify("akra", "Play / pause", "choose a window", "Choose a window in Controls")
		return
	}
	exe := baseName(t.path)
	if strings.EqualFold(exe, "Spotify.exe") {
		a.spotifyCommand(APPCOMMAND_MEDIA_PLAY_PAUSE, false)
		return
	}
	name := prettyApp(exe)
	hwnd := a.resolvePlayWindow()
	if hwnd == 0 {
		a.notify(t.path, name, "isn't open", name+" isn't open")
		return
	}
	a.pendingToggle = &togglePending{exe: exe, name: name, path: t.path, hwnd: hwnd, fromKey: a.fromKey,
		until: time.Now().Add(900 * time.Millisecond)}
	if !a.mediaToggle(sessionName(exe)) {
		a.toggleFallback()
		return
	}
	time.AfterFunc(950*time.Millisecond, func() { pPostMessageW.Call(a.hwnd, WM_MEDIA, 2, 0) })
}

// onToggleResult: the media helper answered (ok = it found and toggled a session).
func (a *App) onToggleResult(ok bool) {
	p := a.pendingToggle
	if p == nil {
		return
	}
	if !ok {
		a.toggleFallback()
		return
	}
	a.pendingToggle = nil
	a.fromKey = p.fromKey
	a.notify(p.path, p.name, "Play / pause", "Play / pause › "+p.name)
	a.fromKey = false
}

func (a *App) toggleFallback() {
	p := a.pendingToggle
	a.pendingToggle = nil
	if p == nil {
		return
	}
	a.fromKey = p.fromKey
	defer func() { a.fromKey = false }()
	switch {
	case isBrowser(p.exe) && a.extConnected():
		a.extSend("toggle-media")
		a.notify(p.path, p.name, "Play / pause", "Play / pause › "+p.name)
	case spaceIsPlayPause(p.exe):
		postKey(p.hwnd, VK_SPACE)
		a.notify(p.path, p.name, "Play / pause", "Play / pause › "+p.name)
	case isBrowser(p.exe):
		a.notify(p.path, p.name, "add the extension", p.name+": add the Akra browser extension (see README)")
	default:
		a.notify(p.path, p.name, "can't be controlled", p.name+" doesn't take play/pause from other apps")
	}
}

// postKey sends a key press straight to a window (no focus change).
func postKey(hwnd uintptr, vk uintptr) {
	sc, _, _ := pMapVirtualKeyW.Call(vk, 0)
	lp := uintptr(1) | sc<<16
	pPostMessageW.Call(hwnd, WM_KEYDOWN, vk, lp)
	pPostMessageW.Call(hwnd, WM_KEYUP, vk, lp|1<<30|1<<31)
}

func (a *App) youTubeToggle() {
	if a.extConnected() {
		a.extSend("toggle-youtube")
		a.notify("youtube", "YouTube", "Play / pause", "Play / pause › YouTube")
		return
	}
	a.notify("youtube", "YouTube", "extension not connected", "Add the Akra browser extension to control YouTube (see README)")
}

// ---------- like ----------

var (
	pSendInput         = user32.NewProc("SendInput")
	pAttachThreadInput = user32.NewProc("AttachThreadInput")
	pBringWindowToTop  = user32.NewProc("BringWindowToTop")
)

type kbInput struct {
	typ   uint32
	_     uint32
	vk    uint16
	scan  uint16
	flags uint32
	time  uint32
	extra uintptr
	_     [8]byte
}

func sendKeys(vks []uint16, up bool) {
	in := make([]kbInput, len(vks))
	for i, vk := range vks {
		in[i] = kbInput{typ: 1, vk: vk, extra: akraInputTag}
		if up {
			in[i].flags = 2 // KEYEVENTF_KEYUP
		}
	}
	pSendInput.Call(uintptr(len(in)), uintptr(unsafe.Pointer(&in[0])), unsafe.Sizeof(in[0]))
}

// fullscreenForeground: is a game (or anything) covering the whole screen?
func fullscreenForeground() bool {
	fg, _, _ := pGetForegroundWindow.Call()
	if fg == 0 || fg == selfHwnd {
		return false
	}
	switch className(fg) {
	case "Progman", "WorkerW":
		return false
	}
	var r RECT
	pGetWindowRect.Call(fg, uintptr(unsafe.Pointer(&r)))
	mon, _, _ := pMonitorFromWindow.Call(fg, 2)
	mi := MONITORINFO{}
	mi.CbSize = uint32(unsafe.Sizeof(mi))
	pGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&mi)))
	m := mi.RcMonitor
	return r.L <= m.L && r.T <= m.T && r.R >= m.R && r.B >= m.B
}

// likeSong presses Spotify's own "Like" shortcut (Alt+Shift+B) in Spotify,
// then puts focus back where it was. Spotify has no other way in without
// signing in to its web API.
func (a *App) likeSong() {
	spot := a.spot
	if spot == 0 || a.trackKey == "" {
		a.notify("spotify", "Spotify", "nothing playing", "Play something in Spotify first")
		return
	}
	if a.fromKey && fullscreenForeground() {
		// switching to Spotify would minimize a fullscreen game
		a.notify("spotify", "Like", "use the heart in the player", "Can't like while a fullscreen game is in front")
		return
	}
	if a.liking {
		return
	}
	a.liking = true
	key := a.trackKey
	self := a.hwnd
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		// wait until the hotkey's own modifiers are released
		for i := 0; i < 40 && asyncMods() != 0; i++ {
			time.Sleep(15 * time.Millisecond)
		}
		prev, _, _ := pGetForegroundWindow.Call()
		hidden := false
		if v, _, _ := pIsWindowVisible.Call(spot); v == 0 {
			hidden = true
			pShowWindow.Call(spot, 4) // SW_SHOWNOACTIVATE
		}
		minimized := false
		if m, _, _ := pIsIconic.Call(spot); m != 0 {
			minimized = true
			pShowWindow.Call(spot, 4)
		}
		focus(spot)
		time.Sleep(90 * time.Millisecond)
		sendKeys([]uint16{0x12, 0x10, 'B'}, false) // Alt, Shift, B
		sendKeys([]uint16{'B', 0x10, 0x12}, true)
		time.Sleep(140 * time.Millisecond)
		switch {
		case hidden:
			pShowWindow.Call(spot, 0) // back to the tray
		case minimized:
			pShowWindow.Call(spot, 7) // SW_SHOWMINNOACTIVE
		}
		if prev != 0 && prev != spot {
			focus(prev)
		}
		pPostMessageW.Call(self, WM_LIKED, 0, 0)
	}()
	a.liked[key] = !a.liked[key]
	a.ui.Liked = a.liked[key]
	if a.liked[key] {
		a.notify("spotify", "Spotify", "Liked", "Added to Liked Songs")
	} else {
		a.notify("spotify", "Spotify", "Removed from Liked", "Removed from Liked Songs")
	}
	a.kick()
}

// focus brings a window to the front even when Windows' focus-stealing
// rules would refuse (by briefly sharing input with the current window).
func focus(hwnd uintptr) {
	fg, _, _ := pGetForegroundWindow.Call()
	cur, _, _ := pGetCurrentThreadId.Call()
	ft, _, _ := pGetWindowThreadProcessId.Call(fg, 0)
	if ft != 0 && ft != cur {
		pAttachThreadInput.Call(cur, ft, 1)
		defer pAttachThreadInput.Call(cur, ft, 0)
	}
	pBringWindowToTop.Call(hwnd)
	pSetForegroundWindow.Call(hwnd)
}

// ---------- volume ----------

// toggleVolume: slot = preset index, or -1 for the current window.
// Presets toggle between their % and 100%.
func (a *App) toggleVolume(slot int) {
	p := a.cfg.p()
	var path string
	pct := p.curPct
	if slot >= 0 {
		if slot >= len(p.presets) {
			return
		}
		path, pct = p.presets[slot].app, p.presets[slot].pct
		if path == "" {
			a.notify("akra", "Volume Toggle", "choose an app", "Choose an app for this Volume Toggle in Settings > Keybinds")
			return
		}
	} else {
		path = ForegroundPath()
		if path == "" || isSelf(baseName(path)) {
			a.notify("akra", "Volume", "click a window first", "Click the window you want first")
			return
		}
	}
	exe := baseName(path)
	name := prettyApp(exe)
	cur, found := GetAppVolume(exe)
	if !found {
		a.notify(path, name, "no sound", name+" isn't playing audio")
		return
	}
	if d := cur - float32(pct)/100; d > -0.015 && d < 0.015 && pct != 100 {
		pct = 100 // second press: back to full volume
	}
	SetAppVolume(exe, pct)
	if strings.EqualFold(exe, "Spotify.exe") {
		a.ui.SpotVol = float32(pct) / 100
		a.spotVolHold = time.Now().Add(1200 * time.Millisecond)
	}
	a.notify(path, name, fmt.Sprintf("%d%%", pct), fmt.Sprintf("%s › %d%%", name, pct))
}

// duck lowers the music while the hold key is down (push-to-talk style).
func (a *App) duck(on bool) {
	p := a.cfg.p()
	path := p.duckApp
	if path == "" {
		path = "Spotify.exe"
	}
	exe := baseName(path)
	if on {
		if a.ducked != "" {
			return
		}
		cur, found := GetAppVolume(exe)
		if !found {
			return
		}
		a.ducked, a.duckPrev = exe, cur
		SetAppVolume(exe, min(int(cur*100+0.5), p.duckPct)) // lower, never louder
		if a.cfg.toasts {
			a.keyToast(path, prettyApp(exe), fmt.Sprintf("%d%%", p.duckPct))
		}
		return
	}
	if a.ducked == "" {
		return
	}
	SetAppVolume(a.ducked, int(a.duckPrev*100+0.5))
	a.ducked = ""
}

// ---------- pickers ----------

type pickEntry struct {
	win    winInfo
	path   string
	folder int // -1 = new folder
}

func (a *App) openWindowPicker() {
	active := map[string]bool{}
	for _, ap := range ListAudioApps() {
		if ap.Active {
			active[strings.ToLower(ap.Exe)] = true
		}
	}
	wins := listWindows()
	var playing, rest []winInfo
	for _, w := range wins {
		if active[strings.ToLower(baseName(w.path))] {
			playing = append(playing, w)
		} else {
			rest = append(rest, w)
		}
	}
	wins = append(playing, rest...)
	if len(wins) > 24 {
		wins = wins[:24]
	}
	u := a.ui
	pl := a.cfg.p().play
	u.PickItems, a.pick = u.PickItems[:0], a.pick[:0]
	for _, w := range wins {
		exe := baseName(w.path)
		sel := strings.EqualFold(w.path, pl.path) && w.class == pl.class && (w.hwnd == a.playHwnd || w.title == pl.title)
		u.PickItems = append(u.PickItems, PickItem{Name: w.title, Sub: prettyApp(exe), IconKey: w.path,
			Active: active[strings.ToLower(exe)], Selected: sel})
		a.pick = append(a.pick, pickEntry{win: w})
	}
	a.openPicker(PickWindow, 0)
}

// openAppPicker: slot = preset index, len(presets) = a new preset, -1 = the music to lower.
func (a *App) openAppPicker(slot int) {
	p := a.cfg.p()
	apps := ListAudioApps()
	cur := ""
	switch {
	case slot == -1:
		cur = p.duckApp
		if cur == "" {
			cur = "Spotify.exe"
		}
	case slot < len(p.presets):
		cur = p.presets[slot].app
	}
	have := false
	for _, ap := range apps {
		if strings.EqualFold(ap.Exe, baseName(cur)) {
			have = true
		}
	}
	if cur != "" && !have {
		apps = append([]AudioApp{{Exe: baseName(cur), Path: cur}}, apps...)
	}
	u := a.ui
	u.PickItems, a.pick = u.PickItems[:0], a.pick[:0]
	for _, ap := range apps {
		state := "Sound open"
		if ap.Active {
			state = "Playing now"
		}
		u.PickItems = append(u.PickItems, PickItem{Name: prettyApp(ap.Exe), Sub: state, IconKey: ap.Path,
			Active: ap.Active, Selected: cur != "" && strings.EqualFold(ap.Exe, baseName(cur))})
		a.pick = append(a.pick, pickEntry{path: ap.Path})
	}
	a.openPicker(PickApp, slot)
}

func (a *App) openFolderPicker(act int) {
	u := a.ui
	u.PickItems, a.pick = u.PickItems[:0], a.pick[:0]
	for i, f := range a.cfg.p().folders {
		in := false
		for _, x := range f.Acts {
			if x == act {
				in = true
			}
		}
		u.PickItems = append(u.PickItems, PickItem{Name: f.Name, Selected: in})
		a.pick = append(a.pick, pickEntry{folder: i})
	}
	u.PickItems = append(u.PickItems, PickItem{Name: "+  New folder"})
	a.pick = append(a.pick, pickEntry{folder: -1})
	a.openPicker(PickFolder, act)
}

func (a *App) openPicker(mode, slot int) {
	u := a.ui
	u.PickMode, u.PickSlot, u.PickOpen = mode, slot, true
	u.pickScroll.Pos.Snap(0)
	u.pickScroll.Over = 0
	a.kick()
}

func (a *App) choosePicked(i int) {
	u := a.ui
	if i < 0 || i >= len(a.pick) {
		return
	}
	e := a.pick[i]
	p := a.cfg.p()
	switch u.PickMode {
	case PickWindow:
		p.play = winTarget{path: e.win.path, class: e.win.class, title: e.win.title}
		a.playHwnd = e.win.hwnd
	case PickApp:
		switch {
		case u.PickSlot == -1:
			p.duckApp = e.path
			if strings.EqualFold(baseName(e.path), "Spotify.exe") {
				p.duckApp = ""
			}
		case u.PickSlot >= len(p.presets):
			a.addPreset(e.path)
		default:
			p.presets[u.PickSlot].app = e.path
		}
	case PickFolder:
		dst := e.folder
		if dst < 0 {
			dst = a.newFolder(false)
		}
		a.moveToFolder(u.PickSlot, dst)
	}
	u.PickOpen = false
	a.syncTargets()
	saveConfig(a.cfg)
	a.kick()
}

func (a *App) addPreset(path string) {
	p := a.cfg.p()
	id := p.nextPresetID()
	p.presets = append(p.presets, presetCfg{id: id, app: path, pct: 50})
	dst := 0
	for i, f := range p.folders {
		if f.Name == "Volume" {
			dst = i
		}
	}
	p.folders[dst].Acts = append(p.folders[dst].Acts, PresetAct(id))
	p.folders[dst].Collapsed = false
	a.syncTargets()
	a.registerAll()
}

func (a *App) deletePreset(i int) {
	p := a.cfg.p()
	if i < 0 || i >= len(p.presets) {
		return
	}
	act := PresetAct(p.presets[i].id)
	p.presets = append(p.presets[:i], p.presets[i+1:]...)
	delete(p.binds, act)
	for k := range p.folders {
		var keep []int
		for _, x := range p.folders[k].Acts {
			if x != act {
				keep = append(keep, x)
			}
		}
		p.folders[k].Acts = keep
	}
	a.syncTargets()
	a.refreshBinds()
	a.registerAll()
	saveConfig(a.cfg)
	a.ui.ShowToast("Preset removed")
}

// syncTargets copies the active profile into the UI.
func (a *App) syncTargets() {
	u := a.ui
	p := a.cfg.p()
	if pl := p.play; pl.path != "" {
		name := pl.title
		if name == "" {
			name = prettyApp(baseName(pl.path))
		}
		u.PlayTarget = Target{Name: name, Sub: prettyApp(baseName(pl.path)), IconKey: pl.path}
	} else {
		u.PlayTarget = Target{}
	}
	u.Presets = u.Presets[:0]
	for _, ps := range p.presets {
		t := Target{}
		if ps.app != "" {
			t = Target{Name: prettyApp(baseName(ps.app)), Sub: baseName(ps.app), IconKey: ps.app}
		}
		u.Presets = append(u.Presets, PresetUI{ID: ps.id, App: t, Pct: ps.pct})
	}
	u.CurPct, u.DuckPct = p.curPct, p.duckPct
	if p.duckApp != "" {
		u.DuckTarget = Target{Name: prettyApp(baseName(p.duckApp)), Sub: "Music to lower", IconKey: p.duckApp}
	} else {
		u.DuckTarget = Target{}
	}
	u.Folders = p.folders
	u.Profiles = u.Profiles[:0]
	for _, pr := range a.cfg.profiles {
		u.Profiles = append(u.Profiles, pr.name)
	}
	u.Profile = a.cfg.prof
}

// ---------- profiles ----------

func (a *App) switchProfile(i int) {
	if i < 0 || i >= len(a.cfg.profiles) || i == a.cfg.prof {
		return
	}
	a.leaveProfile()
	a.cfg.prof = i
	a.applyProfile()
}

// leaveProfile undoes anything the current profile is in the middle of.
func (a *App) leaveProfile() {
	if a.ui.RecIdx >= 0 {
		a.cancelRecording()
	}
	if a.ducked != "" {
		a.duck(false)
	}
	a.commitEdit()
}

func (a *App) applyProfile() {
	a.playHwnd = 0
	a.syncTargets()
	a.resetFolds()
	a.refreshBinds()
	a.registerAll()
	saveConfig(a.cfg)
	a.notify("akra", "Profile", a.cfg.p().name, "Profile: "+a.cfg.p().name)
	a.kick()
}

func (a *App) addProfile() {
	a.leaveProfile()
	name := fmt.Sprintf("Profile %d", len(a.cfg.profiles)+1)
	a.cfg.profiles = append(a.cfg.profiles, a.cfg.p().clone(name))
	a.cfg.prof = len(a.cfg.profiles) - 1
	a.applyProfile()
	a.startEdit(EditProfile, a.cfg.prof)
}

func (a *App) deleteProfile() {
	if len(a.cfg.profiles) <= 1 {
		return
	}
	a.leaveProfile()
	i := a.cfg.prof
	name := a.cfg.profiles[i].name
	a.cfg.profiles = append(a.cfg.profiles[:i], a.cfg.profiles[i+1:]...)
	a.cfg.prof = max(0, i-1)
	a.applyProfile()
	a.ui.ShowToast("Deleted profile " + name)
}

// ---------- folders ----------

func (a *App) resetFolds() {
	a.ui.fold = map[int]*Tween{}
	for k, f := range a.cfg.p().folders {
		a.ui.fold[k] = &Tween{V: b2f(!f.Collapsed)}
	}
}

func (a *App) newFolder(rename bool) int {
	p := a.cfg.p()
	name := fmt.Sprintf("Folder %d", len(p.folders)+1)
	p.folders = append(p.folders, Folder{Name: name})
	idx := len(p.folders) - 1
	a.syncTargets()
	if rename {
		a.startEdit(EditFolder, idx)
	}
	saveConfig(a.cfg)
	return idx
}

func (a *App) moveToFolder(act, dst int) {
	p := a.cfg.p()
	for i := range p.folders {
		var keep []int
		for _, x := range p.folders[i].Acts {
			if x != act {
				keep = append(keep, x)
			}
		}
		p.folders[i].Acts = keep
	}
	p.folders[dst].Acts = append(p.folders[dst].Acts, act)
	p.folders[dst].Collapsed = false
	a.syncTargets()
}

func (a *App) deleteFolder(i int) {
	p := a.cfg.p()
	if len(p.folders) <= 1 || i < 0 || i >= len(p.folders) {
		return
	}
	acts := p.folders[i].Acts
	p.folders = append(p.folders[:i], p.folders[i+1:]...)
	dst := 0
	if i > 0 {
		dst = i - 1
	}
	p.folders[dst].Acts = append(p.folders[dst].Acts, acts...)
	a.ui.EditKind = EditNone
	a.resetFolds()
	a.syncTargets()
	saveConfig(a.cfg)
	a.ui.ShowToast("Folder deleted, keybinds moved")
}

func (a *App) startEdit(kind, i int) {
	u := a.ui
	u.EditKind, u.EditIdx = kind, i
	if kind == EditProfile {
		u.EditText = a.cfg.profiles[i].name
	} else if kind == EditLook {
		u.EditText = a.cfg.looks[i].name
	} else {
		u.EditText = a.cfg.p().folders[i].Name
	}
	if u.SetPopView && a.setWin != nil { // typing into the Settings window
		pSetForegroundWindow.Call(a.setWin.hwnd)
	} else {
		pSetForegroundWindow.Call(a.hwnd)
	}
	a.kick()
}

func (a *App) commitEdit() {
	u := a.ui
	name := strings.TrimSpace(u.EditText)
	switch u.EditKind {
	case EditFolder:
		if f := a.cfg.p().folders; u.EditIdx >= 0 && u.EditIdx < len(f) && name != "" {
			f[u.EditIdx].Name = name
		}
	case EditProfile:
		if u.EditIdx >= 0 && u.EditIdx < len(a.cfg.profiles) && name != "" {
			a.cfg.profiles[u.EditIdx].name = name
		}
	case EditLook:
		if u.EditIdx >= 0 && u.EditIdx < len(a.cfg.looks) && name != "" {
			a.cfg.looks[u.EditIdx].name = name
		}
	default:
		return
	}
	u.EditKind = EditNone
	a.syncTargets()
	saveConfig(a.cfg)
}

// typed characters go into the name being edited
func (a *App) onChar(ch rune) {
	u := a.ui
	switch {
	case ch == 8: // backspace
		if r := []rune(u.EditText); len(r) > 0 {
			u.EditText = string(r[:len(r)-1])
		}
	case ch == 13:
		a.commitEdit()
	case ch == 27:
		u.EditKind = EditNone
	case ch >= 32 && len([]rune(u.EditText)) < 24:
		u.EditText += string(ch)
	}
	a.kick()
}

// ---------- settings actions ----------

func (a *App) setPinned(on bool) {
	a.cfg.pinned = on
	a.ui.Toggles[TogOnTop] = on
	after := ^uintptr(1) // HWND_NOTOPMOST
	if on || a.ui.Full {
		after = ^uintptr(0) // HWND_TOPMOST
	}
	pSetWindowPos.Call(a.hwnd, after, 0, 0, 0, 0, SWP_NOMOVE|SWP_NOSIZE|SWP_NOACTIVATE)
	saveConfig(a.cfg)
	a.kick()
}

func (a *App) setToggle(i int) {
	on := !a.ui.Toggles[i]
	a.ui.Toggles[i] = on
	switch i {
	case TogStartup:
		setAutostart(on)
		a.ui.Toggles[i] = autostartEnabled(runValue)
	case TogOnTop:
		a.setPinned(on)
	case TogBlur:
		a.cfg.blur = on
		if !on {
			a.ui.Backdrop = nil
		} else {
			a.requestCapture()
		}
	case TogArt:
		a.cfg.art = on
		if on {
			a.ui.artIn = 0 // play the cover's entrance
		}
		if on && a.trackKey != "" {
			a.requestArt(a.lastArtist, a.lastTrack)
		}
	case TogToasts:
		a.cfg.toasts = on
	case TogVis:
		a.cfg.vis = on
		a.updateMeters()
	case TogGlow:
		a.cfg.glow = on
	case TogSongPopup:
		a.cfg.songPopup = on
	case TogIdleFade:
		a.cfg.idleFade = on
		a.lastInput = time.Now()
	case TogTrayAccent:
		a.cfg.trayAccent = on
		a.refreshTrayIcon()
	case TogFullscreen:
		a.cfg.fullscreen = on
		if !on && a.ui.Full {
			a.exitFullscreen()
		}
	case TogTips:
		a.cfg.tips = on
		if !on {
			a.trackTip("")
		}
	case TogOutline:
		a.cfg.outline = on
	case TogFluid:
		a.cfg.fluid = on
	case TogAnchor:
		a.cfg.anchor = on
	}
	saveConfig(a.cfg)
	a.kick()
	a.kickPill()
}

func (a *App) setAccent(idx int) {
	a.ui.AccentIdx = idx
	a.cfg.accent = idx
	saveConfig(a.cfg)
	a.refreshTrayIcon()
	a.kick()
}

func (a *App) setFont(f int) {
	a.ui.Font = f
	a.cfg.font = f
	saveConfig(a.cfg)
	a.kick()
}

func (a *App) setVisStyle(i int) {
	a.ui.VisStyle = i
	a.cfg.visStyle = i
	if i == VisEdge {
		a.ui.Monitors = monitorNames()
	}
	a.updateMeters()
	saveConfig(a.cfg)
	a.kick()
}

func (a *App) setVisMonitor(i int) {
	a.ui.VisMonitor = i
	a.cfg.visMon = i
	saveConfig(a.cfg)
	a.kick()
}

// setDeclutter hides everything but the song and its controls (or brings it
// all back).
func (a *App) setDeclutter(on bool) {
	u := a.ui
	u.Declutter = on
	a.cfg.declutter = on
	saveConfig(a.cfg)
	if a.fromKey && a.cfg.toasts && !(a.visible && !a.hiding) {
		a.keyToast("akra", "Declutter", map[bool]string{true: "On", false: "Off"}[on])
	}
	a.kick()
}

// ---------- 2.9: song popups, sleep timer, fade when idle ----------

// onSongChange runs when Spotify moves to another song.
func (a *App) onSongChange(oldKey, newKey, track, artist string, st Status) {
	u := a.ui
	// sleep timer "End of song": the song that was playing has ended
	if u.Sleep == SleepSong && a.sleepKey != "" && newKey != a.sleepKey {
		a.sleepNow()
		return
	}
	// Now playing popups: only while the player is hidden, not on startup
	if a.cfg.songPopup && oldKey != "" && st == StPlaying && !(a.visible && !a.hiding) {
		short := func(s string, n int) string {
			if r := []rune(s); len(r) > n {
				return string(r[:n-1]) + "…"
			}
			return s
		}
		a.keyToastAt("spotify", short(track, 30), short(artist, 24), a.cfg.songPos, a.cfg.songMon)
		a.toast.until = time.Now().Add(3200 * time.Millisecond)
	}
}

// setSleep starts (or stops) the sleep timer.
func (a *App) setSleep(i int) {
	u := a.ui
	u.Sleep = i
	switch {
	case i == SleepOff:
		u.ShowToast("Sleep timer off")
	case i == SleepSong:
		a.sleepKey = a.trackKey
		if a.trackKey == "" {
			a.sleepKey = "?" // nothing playing yet: the next song change
		}
		u.ShowToast("Spotify pauses when this song ends")
	default:
		a.sleepAt = time.Now().Add(time.Duration(sleepMinutes[i] * float32(time.Minute)))
		u.SleepLeft = sleepMinutes[i] * 60
		u.ShowToast("Spotify pauses in " + sleepNames[i])
	}
	if !(a.visible && !a.hiding) && a.cfg.toasts { // set from the tray
		a.keyToast("akra", "Sleep timer", sleepNames[i])
	}
	a.updateTrayTip()
	a.kick()
}

// stepSleep counts the minute timers down (from the 90 ms timer).
func (a *App) stepSleep() {
	u := a.ui
	if u.Sleep == SleepOff || u.Sleep == SleepSong {
		return
	}
	left := float32(time.Until(a.sleepAt).Seconds())
	if left <= 0 {
		a.sleepNow()
		return
	}
	if int(left/60) != int(u.SleepLeft/60) { // the minute shown changed
		a.kick()
	}
	u.SleepLeft = left
}

// sleepNow pauses Spotify and ends the timer.
func (a *App) sleepNow() {
	u := a.ui
	u.Sleep = SleepOff
	a.sleepKey = ""
	if u.Status == StPlaying && a.spot != 0 {
		a.spotifyCommand(APPCOMMAND_MEDIA_PLAY_PAUSE, false)
	}
	if a.visible && !a.hiding {
		u.ShowToast("Sleep timer: Spotify paused")
	} else if a.cfg.toasts {
		a.keyToast("spotify", "Sleep timer", "Paused")
	}
	a.updateTrayTip()
	a.kick()
}

// checkIdle: Fade when idle fades the player while the mouse is away and
// nothing is open (from the 90 ms timer).
func (a *App) checkIdle() {
	u := a.ui
	want := false
	if a.cfg.idleFade && a.visible && !a.hiding && !u.Full && !a.tracking && !a.setOpen &&
		u.Page == 0 && u.Sheet == SheetNone && u.Pop == PopNone && !u.PickOpen && !u.HueOpen &&
		u.RecIdx < 0 && u.EditKind == EditNone && time.Since(a.lastInput) > time.Duration(a.cfg.idleDelay*float32(time.Second)) {
		fg, _, _ := pGetForegroundWindow.Call()
		want = fg != a.hwnd
	}
	if want != a.idleWant {
		a.idleWant = want
		a.kick()
	}
}

// songPopupSample shows where Now playing popups will appear.
func (a *App) songPopupSample() {
	u := a.ui
	title, artist := u.Title, u.Artist
	if title == "" {
		title, artist = "Now playing", "Song name"
	}
	a.keyToastAt("spotify", title, artist, a.cfg.songPos, a.cfg.songMon)
}

// previewIdle: while you drag the faded opacity, show it for a moment.
func (a *App) previewIdle() {
	a.idle.V = 1
	a.idleWant = false // it eases back in right after
	a.kick()
}
