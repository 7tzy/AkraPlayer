//go:build windows

package main

import (
	"os"
	"strings"
	"syscall"
	"unsafe"
)

// ---------- tray ----------

func (a *App) makeTrayIcon() {
	sz, _, _ := pGetSystemMetrics.Call(49) // SM_CXSMICON
	s := int(sz)
	if s <= 0 {
		s = 16
	}
	c := NewCanvas(s, s, float32(s)/32)
	if a.cfg.trayAccent { // in your color
		ac := a.ui.AccentTarget()
		if a.ui.AccentIdx == AccentRainbow {
			ac = Hex(0xbf5af2, 1)
		}
		DrawAppIcon(c, 1.5, 1.2, 29, ac)
	} else {
		DrawBrandIcon(c, 1.5, 1.2, 29)
	}
	dib, bits := newDIB(s, s)
	c.BGRA(unsafe.Slice((*byte)(bits), s*s*4), false)
	mask, _, _ := pCreateBitmap.Call(uintptr(s), uintptr(s), 1, 1, 0)
	ii := ICONINFO{FIcon: 1, HbmMask: mask, HbmColor: dib}
	a.trayIcon, _, _ = pCreateIconIndirect.Call(uintptr(unsafe.Pointer(&ii)))
	pDeleteObject.Call(mask)
	pDeleteObject.Call(dib)
}

func (a *App) refreshTrayIcon() {
	old := a.trayIcon
	a.makeTrayIcon()
	a.updateTrayTip()
	if old != 0 {
		user32.NewProc("DestroyIcon").Call(old)
	}
}

func (a *App) trayData() NOTIFYICONDATA {
	var nid NOTIFYICONDATA
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	nid.HWnd = a.hwnd
	nid.UID = 1
	nid.UFlags = NIF_MESSAGE | NIF_ICON | NIF_TIP
	nid.UCallbackMessage = WM_TRAY
	nid.HIcon = a.trayIcon
	tip := appName
	switch a.ui.Status {
	case StPlaying:
		tip = "▶ " + a.ui.Title
		if a.ui.Artist != "" {
			tip += " — " + a.ui.Artist
		}
	case StPaused:
		tip = appName + " (paused)"
	}
	if a.ui.Sleep != SleepOff {
		tip = "☾ " + tip
	}
	u, _ := syscall.UTF16FromString(strings.ReplaceAll(tip, "\x00", ""))
	if len(u) > 127 {
		u = append(u[:126], 0)
	}
	copy(nid.SzTip[:], u)
	return nid
}

func (a *App) addTray() {
	nid := a.trayData()
	pShell_NotifyIconW.Call(NIM_ADD, uintptr(unsafe.Pointer(&nid)))
}

func (a *App) updateTrayTip() {
	nid := a.trayData()
	pShell_NotifyIconW.Call(NIM_MODIFY, uintptr(unsafe.Pointer(&nid)))
}

func (a *App) removeTray() {
	nid := a.trayData()
	pShell_NotifyIconW.Call(NIM_DELETE, uintptr(unsafe.Pointer(&nid)))
}

func (a *App) trayMenu() {
	m, _, _ := pCreatePopupMenu.Call()
	add := func(id uintptr, text string, checked bool) {
		f := uintptr(MF_STRING)
		if checked {
			f |= MF_CHECKED
		}
		pAppendMenuW.Call(m, f, id, uintptr(unsafe.Pointer(u16(text))))
	}
	showText := "Show Akra Player"
	if a.visible && !a.hiding {
		showText = "Hide Akra Player"
	}
	add(1, showText, false)
	add(5, "Settings", false)
	add(2, "Play / Pause Spotify", false)
	add(6, "Keystrokes", false)
	add(7, "Mini player", a.ui.Mini)
	if len(a.cfg.profiles) > 1 {
		pAppendMenuW.Call(m, MF_SEPARATOR, 0, 0)
		for i, p := range a.cfg.profiles {
			add(uintptr(100+i), "Profile: "+p.name, i == a.cfg.prof)
		}
	}
	// sleep timer (a submenu)
	sm, _, _ := pCreatePopupMenu.Call()
	for i, name := range sleepNames {
		f := uintptr(MF_STRING)
		if a.ui.Sleep == i {
			f |= MF_CHECKED
		}
		pAppendMenuW.Call(sm, f, uintptr(300+i), uintptr(unsafe.Pointer(u16(name))))
	}
	sleepLabel := "Sleep timer"
	if a.ui.Sleep != SleepOff {
		sleepLabel = "Sleep timer (" + sleepNames[a.ui.Sleep] + ")"
	}
	pAppendMenuW.Call(m, MF_STRING|0x10 /*MF_POPUP*/, sm, uintptr(unsafe.Pointer(u16(sleepLabel))))
	pAppendMenuW.Call(m, MF_SEPARATOR, 0, 0)
	add(3, "Always on top", a.ui.Toggles[TogOnTop])
	add(4, "Open on startup", autostartEnabled(runValue))
	pAppendMenuW.Call(m, MF_SEPARATOR, 0, 0)
	add(9, "Quit", false)

	var pt POINT
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	pSetForegroundWindow.Call(a.hwnd)
	cmd, _, _ := pTrackPopupMenu.Call(m, TPM_RETURNCMD|TPM_RIGHTBUTTON, uintptr(pt.X), uintptr(pt.Y), 0, a.hwnd, 0)
	pPostMessageW.Call(a.hwnd, WM_NULL, 0, 0)
	pDestroyMenu.Call(m)

	switch cmd {
	case 1:
		if a.visible && !a.hiding {
			a.hide()
		} else {
			a.show()
		}
	case 5:
		a.ui.Sheet = SheetNone
		a.ui.Page = 1
		a.show()
		a.kick()
	case 2:
		a.spotifyCommand(APPCOMMAND_MEDIA_PLAY_PAUSE, false)
	case 6:
		a.ui.Page = 0
		a.show()
		a.openPop(PopKeys)
	case 3:
		a.setPinned(!a.ui.Toggles[TogOnTop])
	case 4:
		a.setToggle(TogStartup)
	case 7:
		if a.ui.Mini {
			a.exitMini()
		} else {
			a.show()
			a.enterMini()
		}
	case 9:
		a.quit()
	default:
		if cmd >= 300 && int(cmd-300) < NumSleep {
			a.setSleep(int(cmd - 300))
		} else if cmd >= 100 && int(cmd-100) < len(a.cfg.profiles) {
			a.switchProfile(int(cmd - 100))
			a.kick()
		}
	}
}

func (a *App) quit() {
	if a.visible {
		a.keepCenter()
	}
	if !a.ui.Mini && !a.ui.Full {
		a.savePos()
	}
	if a.ducked != "" {
		a.duck(false)
	}
	a.stopMedia()
	a.removeTray()
	a.unregisterAll()
	setHooks(false, false)
	pPostQuitMessage.Call(0)
}

// ---------- autostart ----------

func autostartEnabled(value string) bool {
	var size uint32
	r, _, _ := pRegGetValueW.Call(HKEY_CURRENT_USER, uintptr(unsafe.Pointer(u16(runKey))), uintptr(unsafe.Pointer(u16(value))),
		RRF_RT_REG_SZ, 0, 0, uintptr(unsafe.Pointer(&size)))
	return r == 0
}

func setAutostart(on bool) {
	if !on {
		pRegDeleteKeyValueW.Call(HKEY_CURRENT_USER, uintptr(unsafe.Pointer(u16(runKey))), uintptr(unsafe.Pointer(u16(runValue))))
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	val, _ := syscall.UTF16FromString(`"` + exe + `" --hidden`)
	pRegSetKeyValueW.Call(HKEY_CURRENT_USER, uintptr(unsafe.Pointer(u16(runKey))), uintptr(unsafe.Pointer(u16(runValue))),
		REG_SZ, uintptr(unsafe.Pointer(&val[0])), uintptr(len(val)*2))
}

// If the old Spotify Toggle was set to start with Windows, move that over.
func migrateAutostart() bool {
	if autostartEnabled(oldRunVal) {
		pRegDeleteKeyValueW.Call(HKEY_CURRENT_USER, uintptr(unsafe.Pointer(u16(runKey))), uintptr(unsafe.Pointer(u16(oldRunVal))))
		setAutostart(true)
	}
	if autostartEnabled(runValue) {
		setAutostart(true) // refresh the path in case the exe moved
		return true
	}
	return false
}
