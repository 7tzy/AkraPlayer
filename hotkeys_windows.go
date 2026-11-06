//go:build windows

package main

import (
	"fmt"
	"strconv"
	"syscall"
	"time"
	"unsafe"
)

// ---------- hotkeys ----------

func (a *App) unregisterAll() {
	for _, id := range a.regIDs {
		pUnregisterHotKey.Call(a.hwnd, uintptr(id))
	}
	a.regIDs = a.regIDs[:0]
	hookMu.Lock()
	hookBinds = nil
	hookMu.Unlock()
}

// holdAct: keys that act while held and still reach other apps.
func holdAct(act int) bool { return act == ActDuck }

// registerAll: Windows hotkeys where possible (they work even over admin
// windows); anything Windows refuses (key owned by another app, F12, mouse
// buttons, hold-to-lower) runs through the low-level hooks instead.
func (a *App) registerAll() {
	a.unregisterAll()
	p := a.cfg.p()
	var hb []hookBind
	mouse := false
	for _, act := range a.ui.AllActs() {
		b, ok := p.binds[act]
		if !ok || b.vk == 0 || p.off[act] {
			continue
		}
		if act == ActDeclutter && a.cfg.declLocal {
			continue // handled by the player window itself (localKey)
		}
		if isMouseVK(b.vk) {
			mouse = true
		}
		if holdAct(act) || isMouseVK(b.vk) {
			hb = append(hb, hookBind{act: act, mods: b.mods, vk: b.vk, hold: holdAct(act)})
			continue
		}
		ok = false
		if b.vk != VK_F12 {
			r, _, _ := pRegisterHotKey.Call(a.hwnd, uintptr(act+1), uintptr(b.mods|MOD_NOREPEAT), uintptr(b.vk))
			ok = r != 0
		}
		if ok {
			a.regIDs = append(a.regIDs, act+1)
		} else {
			hb = append(hb, hookBind{act: act, mods: b.mods, vk: b.vk})
			if a.ui.BindWarn[act] == "" && b.vk != VK_F12 {
				a.ui.BindWarn[act] = "Another app uses this key too"
			}
		}
	}
	hookMu.Lock()
	hookBinds = hb
	rec := hookRecording
	hookMu.Unlock()
	setHooks(len(hb) > 0 || rec, mouse || rec)
}

// refreshBinds copies the profile's keys (and their warnings) into the UI.
func (a *App) refreshBinds() {
	u := a.ui
	u.Binds = map[int][]string{}
	u.BindWarn = map[int]string{}
	u.BindOff = map[int]bool{}
	for act, o := range a.cfg.p().off {
		u.BindOff[act] = o
	}
	u.DeclLocal = a.cfg.declLocal
	for act, b := range a.cfg.p().binds {
		if b.vk == 0 {
			continue
		}
		u.Binds[act] = keyLabels(b.mods, b.vk)
		u.BindWarn[act] = bindWarning(act, b)
	}
}

func bindWarning(act int, b bind) string {
	w := ShortcutConflict(b.mods, b.vk, holdAct(act))
	if w == "" && holdAct(act) && b.mods == 0 && !isMouseVK(b.vk) && b.vk >= 'A' && b.vk <= 'Z' {
		w = "Still types the letter (that's how push-to-talk works)"
	}
	return w
}

func (a *App) setNote(act int, s string) {
	a.ui.BindNote[act] = s
	a.noteTill[act] = time.Now().Add(3 * time.Second)
}

func (a *App) startRecording(act int) {
	a.unregisterAll() // existing combos must arrive as key presses
	hookMu.Lock()
	hookRecording = true
	hookDown = map[uint32]int{}
	hookMu.Unlock()
	setHooks(true, true)
	a.ui.RecIdx = act
	a.ui.RecKeys = nil
	a.ui.BindNote[act] = ""
	a.ptRemaps = powerToysRemaps()
	if a.ui.Pop != PopNone && a.pop != nil { // recording from the pop-out: keep it open
		pSetForegroundWindow.Call(a.pop.hwnd)
	} else if a.ui.SetPopView && a.setWin != nil { // from the Settings window
		pSetForegroundWindow.Call(a.setWin.hwnd)
	} else {
		pSetForegroundWindow.Call(a.hwnd)
	}
	a.kick()
}

func (a *App) stopRecording() {
	hookMu.Lock()
	hookRecording = false
	hookMu.Unlock()
	a.ui.RecIdx = -1
	a.ui.RecKeys = nil
}

func (a *App) cancelRecording() {
	a.stopRecording()
	a.registerAll()
	a.kick()
}

func isModifierKey(vk uint32) bool {
	switch vk {
	case 0x10, 0x11, 0x12, 0x5B, 0x5C, 0xA0, 0xA1, 0xA2, 0xA3, 0xA4, 0xA5:
		return true
	}
	return false
}

// onRecordKey gets keys (and mouse buttons) from the hooks while a keybind
// is being recorded. Any key is allowed; risky ones get a warning.
func (a *App) onRecordKey(vk uint32, lp uint32) {
	act := a.ui.RecIdx
	if act < 0 {
		return
	}
	mods := lp & 0xFF
	injected := lp&0x100 != 0
	if isModifierKey(vk) {
		a.ui.RecKeys = keyLabels(asyncMods(), 0)
		a.kick()
		return
	}
	if !injected && vk == VK_ESCAPE && mods == 0 {
		a.cancelRecording()
		return
	}
	p := a.cfg.p()
	if !injected && (vk == 0x08 || vk == 0x2E) && mods == 0 { // Backspace/Delete clears the key
		pKillTimer.Call(a.hwnd, timerRecord)
		a.recPending = nil
		a.stopRecording()
		delete(p.binds, act)
		a.refreshBinds()
		saveConfig(a.cfg)
		a.registerAll()
		a.kick()
		return
	}
	nb := bind{mods, vk}
	if r, ok := a.ptRemaps[nb]; ok { // PowerToys turns this key into another one
		nb = r
	}
	if injected || a.ptRemaps[bind{mods, vk}] != (bind{}) {
		pKillTimer.Call(a.hwnd, timerRecord)
		a.recPending = nil
		a.commitRecord(nb)
		return
	}
	// a physical key: wait a moment in case a remapper sends its replacement
	a.recPending = &nb
	pSetTimer.Call(a.hwnd, timerRecord, 70, 0)
}

// commitRecord saves the recorded key for the action being recorded.
func (a *App) commitRecord(nb bind) {
	act := a.ui.RecIdx
	if act < 0 {
		return
	}
	p := a.cfg.p()
	for other, b := range p.binds {
		if other != act && b == nb {
			a.cancelRecording()
			a.setNote(act, "Already used by "+a.ui.ActLabel(other))
			return
		}
	}
	a.stopRecording()
	p.binds[act] = nb
	a.refreshBinds()
	saveConfig(a.cfg)
	a.registerAll()
	a.kick()
}

// localKey: keys that only work while Akra Player is in front (the declutter
// key when set to "Only in app"). Returns true if it was one.
func (a *App) localKey(vk uint32) bool {
	if !a.cfg.declLocal {
		return false
	}
	b, ok := a.cfg.p().binds[ActDeclutter]
	if !ok || b.vk != vk || b.mods != asyncMods() {
		return false
	}
	a.runAction(ActDeclutter, 0)
	a.kick()
	return true
}

// setBindOn switches a keybind on or off (its key is let go while off).
func (a *App) setBindOn(act int, on bool) {
	p := a.cfg.p()
	if p.off == nil {
		p.off = map[int]bool{}
	}
	if on {
		delete(p.off, act)
	} else {
		p.off[act] = true
	}
	a.refreshBinds()
	a.registerAll()
	saveConfig(a.cfg)
	state := "on"
	if !on {
		state = "off"
	}
	a.ui.ShowToast(a.ui.ActLabel(act) + ": " + state)
	a.kick()
}

// checkPlayTarget: does the chosen play/pause window still exist?
func (a *App) checkPlayTarget() {
	gone := a.cfg.p().play.path != "" && a.resolvePlayWindow() == 0
	if gone != a.ui.PlayGone {
		a.ui.PlayGone = gone
		a.kick()
	}
}

// runAction performs a hotkey's action. phase: 0 press, 1 held down, 2 released.
func (a *App) runAction(act, phase int) {
	a.fromKey = true
	defer func() { a.fromKey = false }()
	switch {
	case act == ActDuck:
		if phase == 1 {
			a.duck(true)
		} else if phase == 2 {
			a.duck(false)
		}
	case act == ActPlay:
		if a.spot == 0 {
			a.notify("spotify", "Spotify", "isn't open", "Spotify isn't open")
			return
		}
		a.spotifyCommand(APPCOMMAND_MEDIA_PLAY_PAUSE, false)
		if a.cfg.toasts {
			a.keyToast("spotify", "Spotify", map[bool]string{true: "Play", false: "Pause"}[a.ui.Status == StPlaying])
		}
	case act == ActShow:
		a.toggleVisible()
	case act == ActMute:
		a.muteForeground()
	case act == ActAppPlay:
		a.playPauseTarget()
	case act == ActVolCur:
		a.toggleVolume(-1)
	case act == ActLike:
		a.likeSong()
	case act == ActYouTube:
		a.youTubeToggle()
	case act == ActDeclutter:
		a.setDeclutter(!a.ui.Declutter)
	case IsPresetAct(act):
		for i, ps := range a.cfg.p().presets {
			if PresetAct(ps.id) == act {
				a.toggleVolume(i)
			}
		}
	}
}

func keyLabels(mods, vk uint32) []string {
	var out []string
	if mods&MOD_WIN != 0 {
		out = append(out, "Win")
	}
	if mods&MOD_CONTROL != 0 {
		out = append(out, "Ctrl")
	}
	if mods&MOD_ALT != 0 {
		out = append(out, "Alt")
	}
	if mods&MOD_SHIFT != 0 {
		out = append(out, "Shift")
	}
	if vk != 0 {
		out = append(out, vkName(vk))
	}
	return out
}

func vkName(vk uint32) string {
	switch {
	case vk >= 0x70 && vk <= 0x87:
		return "F" + strconv.Itoa(int(vk-0x6F))
	case (vk >= '0' && vk <= '9') || (vk >= 'A' && vk <= 'Z'):
		return string(rune(vk))
	case vk >= 0x60 && vk <= 0x69:
		return "Num " + strconv.Itoa(int(vk-0x60))
	}
	names := map[uint32]string{
		vkMButton: "Middle click", vkXButton1: "Mouse 4", vkXButton2: "Mouse 5",
		0xB3: "Play/Pause", 0xB0: "Next", 0xB1: "Prev", 0xB2: "Stop", 0xAD: "Mute", 0xAE: "Vol −", 0xAF: "Vol +",
		0x20: "Space", 0x13: "Pause", 0x91: "ScrLk", 0x14: "Caps Lock", 0x2D: "Insert", 0x2E: "Delete", 0x24: "Home", 0x23: "End",
		0x21: "PgUp", 0x22: "PgDn", 0x25: "←", 0x26: "↑", 0x27: "→", 0x28: "↓", 0x0D: "Enter", 0x09: "Tab", 0x2C: "PrtSc",
	}
	if n, ok := names[vk]; ok {
		return n
	}
	sc, _, _ := pMapVirtualKeyW.Call(uintptr(vk), 0)
	var buf [64]uint16
	n, _, _ := pGetKeyNameTextW.Call(sc<<16, uintptr(unsafe.Pointer(&buf[0])), 64)
	if n > 0 {
		return syscall.UTF16ToString(buf[:n])
	}
	return fmt.Sprintf("Key %d", vk)
}
