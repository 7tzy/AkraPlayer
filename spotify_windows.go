//go:build windows

package main

import (
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

// Finding Spotify: its main window belongs to Spotify.exe, has a Chromium
// window class, and its title is "Artist - Song" while playing or
// "Spotify Premium"/"Spotify Free"/"Spotify" while paused.

var (
	procNames = map[uint32]string{}
	enumBest  uintptr
	enumTitle string
	enumScore int
	enumCB    = syscall.NewCallback(enumProc)
)

func processName(pid uint32) string {
	if n, ok := procNames[pid]; ok {
		return n
	}
	if len(procNames) > 500 {
		procNames = map[uint32]string{}
	}
	name := ""
	h, _, _ := pOpenProcess.Call(0x1000 /*QUERY_LIMITED_INFORMATION*/, 0, uintptr(pid))
	if h != 0 {
		var buf [520]uint16
		size := uint32(len(buf))
		r, _, _ := pQueryFullProcessImageNameW.Call(h, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
		if r != 0 {
			name = filepath.Base(syscall.UTF16ToString(buf[:size]))
		}
		pCloseHandle.Call(h)
	}
	procNames[pid] = name
	return name
}

func windowText(hwnd uintptr) string {
	var buf [512]uint16
	n, _, _ := pGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return syscall.UTF16ToString(buf[:n])
}

func className(hwnd uintptr) string {
	var buf [256]uint16
	n, _, _ := pGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return syscall.UTF16ToString(buf[:n])
}

func enumProc(hwnd, _ uintptr) uintptr {
	title := windowText(hwnd)
	if title == "" {
		return 1
	}
	if !strings.HasPrefix(className(hwnd), "Chrome_WidgetWin") {
		return 1
	}
	var pid uint32
	pGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if !strings.EqualFold(processName(pid), "Spotify.exe") {
		return 1
	}
	score := 1
	if strings.HasPrefix(title, "Spotify") || strings.Contains(title, " - ") {
		score = 3
	}
	if score > enumScore {
		enumBest, enumTitle, enumScore = hwnd, title, score
	}
	return 1
}

func findSpotify() (uintptr, string) {
	enumBest, enumTitle, enumScore = 0, "", 0
	pEnumWindows.Call(enumCB, 0)
	return enumBest, enumTitle
}

// parseTitle turns Spotify's window title into a status + track info.
func parseTitle(title string) (st Status, track, artist string) {
	t := strings.TrimSpace(title)
	switch {
	case t == "Spotify" || t == "Spotify Premium" || t == "Spotify Free":
		return StPaused, "", ""
	case strings.EqualFold(t, "Advertisement") || strings.EqualFold(t, "Spotify Advertisement") ||
		strings.HasPrefix(t, "Advertisement"):
		return StAd, "Advertisement", "Spotify"
	}
	if i := strings.Index(t, " - "); i > 0 {
		return StPlaying, strings.TrimSpace(t[i+3:]), strings.TrimSpace(t[:i])
	}
	return StPlaying, t, ""
}

func sendAppCommand(hwnd uintptr, cmd uintptr) {
	// Send to Spotify's own window, so only Spotify reacts (not YouTube/Chrome).
	pPostMessageW.Call(hwnd, WM_APPCOMMAND, hwnd, cmd<<16)
}

func focusWindow(hwnd uintptr) {
	if r, _, _ := pIsIconic.Call(hwnd); r != 0 {
		pShowWindow.Call(hwnd, SW_RESTORE)
	} else if r, _, _ := pIsWindowVisible.Call(hwnd); r == 0 {
		pShowWindow.Call(hwnd, SW_SHOW)
	}
	pSetForegroundWindow.Call(hwnd)
}

func launchSpotify() {
	pShellExecuteW.Call(0, uintptr(unsafe.Pointer(u16("open"))), uintptr(unsafe.Pointer(u16("spotify:"))), 0, 0, 1)
}
