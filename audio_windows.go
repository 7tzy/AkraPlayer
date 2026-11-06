//go:build windows

package main

// Per-app mute using the Windows Core Audio session API (the same thing the
// Volume Mixer uses). COM objects are called through their vtables.

import (
	"math"
	"os"
	"strings"
	"syscall"
	"unsafe"
)

func osExecutable() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	if i := strings.LastIndexAny(p, `\/`); i >= 0 {
		p = p[i+1:]
	}
	return p, nil
}

type GUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

var (
	CLSID_MMDeviceEnumerator  = GUID{0xBCDE0395, 0xE52F, 0x467C, [8]byte{0x8E, 0x3D, 0xC4, 0x57, 0x92, 0x91, 0x69, 0x2E}}
	IID_IMMDeviceEnumerator   = GUID{0xA95664D2, 0x9614, 0x4F35, [8]byte{0xA7, 0x46, 0xDE, 0x8D, 0xB6, 0x36, 0x17, 0xE6}}
	IID_IAudioSessionManager2 = GUID{0x77AA99A0, 0x1BD6, 0x484F, [8]byte{0x8B, 0xC7, 0x2C, 0x65, 0x4C, 0x9A, 0x9B, 0x6F}}
	IID_IAudioSessionControl2 = GUID{0xBFB7FF88, 0x7239, 0x4FC9, [8]byte{0x8F, 0xA2, 0x07, 0xC9, 0x50, 0xBE, 0x9C, 0x6D}}
	IID_ISimpleAudioVolume    = GUID{0x87CE5498, 0x68D6, 0x44E5, [8]byte{0x92, 0x15, 0x6D, 0xA4, 0x7E, 0xF8, 0x83, 0xD8}}
)

// call invokes vtable method idx on a COM object.
func call(obj uintptr, idx int, args ...uintptr) uintptr {
	vtbl := *(*uintptr)(unsafe.Pointer(obj))
	fn := *(*uintptr)(unsafe.Pointer(vtbl + uintptr(idx)*unsafe.Sizeof(uintptr(0))))
	r, _, _ := syscall.SyscallN(fn, append([]uintptr{obj}, args...)...)
	return r
}

func release(obj uintptr) {
	if obj != 0 {
		call(obj, 2)
	}
}

var comReady bool

func comInit() {
	if !comReady {
		pCoInitializeEx.Call(0, 2) // COINIT_APARTMENTTHREADED
		comReady = true
	}
}

// eachVolume calls fn for every audio session (on every active playback
// device) whose process exe matches name. Returns how many matched.
func eachVolume(name string, fn func(vol uintptr)) int {
	comInit()
	var enum uintptr
	hr, _, _ := pCoCreateInstance.Call(uintptr(unsafe.Pointer(&CLSID_MMDeviceEnumerator)), 0, 0x17, /*CLSCTX_ALL*/
		uintptr(unsafe.Pointer(&IID_IMMDeviceEnumerator)), uintptr(unsafe.Pointer(&enum)))
	if int32(hr) < 0 || enum == 0 {
		return 0
	}
	defer release(enum)

	var coll uintptr
	if int32(call(enum, 3, 0 /*eRender*/, 1 /*ACTIVE*/, uintptr(unsafe.Pointer(&coll)))) < 0 || coll == 0 {
		return 0
	}
	defer release(coll)
	var nDev uint32
	call(coll, 3, uintptr(unsafe.Pointer(&nDev)))

	matched := 0
	for d := uint32(0); d < nDev; d++ {
		var dev uintptr
		if int32(call(coll, 4, uintptr(d), uintptr(unsafe.Pointer(&dev)))) < 0 || dev == 0 {
			continue
		}
		var mgr uintptr
		if int32(call(dev, 3, uintptr(unsafe.Pointer(&IID_IAudioSessionManager2)), 0x17, 0, uintptr(unsafe.Pointer(&mgr)))) < 0 || mgr == 0 {
			release(dev)
			continue
		}
		var sessions uintptr
		if int32(call(mgr, 5, uintptr(unsafe.Pointer(&sessions)))) >= 0 && sessions != 0 {
			var n int32
			call(sessions, 3, uintptr(unsafe.Pointer(&n)))
			for i := int32(0); i < n; i++ {
				var ctl uintptr
				if int32(call(sessions, 4, uintptr(i), uintptr(unsafe.Pointer(&ctl)))) < 0 || ctl == 0 {
					continue
				}
				var ctl2 uintptr
				call(ctl, 0, uintptr(unsafe.Pointer(&IID_IAudioSessionControl2)), uintptr(unsafe.Pointer(&ctl2)))
				if ctl2 != 0 {
					var pid uint32
					call(ctl2, 14, uintptr(unsafe.Pointer(&pid)))
					if pid != 0 && strings.EqualFold(freshProcessName(pid), name) {
						var vol uintptr
						call(ctl, 0, uintptr(unsafe.Pointer(&IID_ISimpleAudioVolume)), uintptr(unsafe.Pointer(&vol)))
						if vol != 0 {
							fn(vol)
							matched++
							release(vol)
						}
					}
					release(ctl2)
				}
				release(ctl)
			}
			release(sessions)
		}
		release(mgr)
		release(dev)
	}
	return matched
}

func getMute(vol uintptr) bool {
	var m int32
	call(vol, 6, uintptr(unsafe.Pointer(&m)))
	return m != 0
}

func setMute(vol uintptr, on bool) {
	v := uintptr(0)
	if on {
		v = 1
	}
	call(vol, 5, v, 0)
}

// IsAppMuted reports whether all of an app's audio sessions are muted.
func IsAppMuted(exe string) (muted bool, found bool) {
	all := true
	n := eachVolume(exe, func(v uintptr) {
		if !getMute(v) {
			all = false
		}
	})
	return n > 0 && all, n > 0
}

// ToggleAppMute mutes the app if any of its sessions are audible, otherwise
// unmutes it. Returns the new state and whether the app had any audio.
func ToggleAppMute(exe string) (nowMuted bool, found bool) {
	muted, found := IsAppMuted(exe)
	if !found {
		return false, false
	}
	target := !muted
	eachVolume(exe, func(v uintptr) { setMute(v, target) })
	return target, true
}

func freshProcessName(pid uint32) string { return baseName(freshProcessPath(pid)) }

// freshProcessPath returns the full exe path of a process ("" if unknown).
func freshProcessPath(pid uint32) string {
	h, _, _ := pOpenProcess.Call(0x1000, 0, uintptr(pid))
	if h == 0 {
		return ""
	}
	defer pCloseHandle.Call(h)
	var buf [520]uint16
	size := uint32(len(buf))
	if r, _, _ := pQueryFullProcessImageNameW.Call(h, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size))); r == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:size])
}

func baseName(p string) string {
	if i := strings.LastIndexAny(p, `\/`); i >= 0 {
		return p[i+1:]
	}
	return p
}

// GetAppVolume returns the app's volume (0-1, averaged over its sessions).
func GetAppVolume(exe string) (float32, bool) {
	var sum float32
	n := eachVolume(exe, func(v uintptr) {
		var lvl float32
		call(v, 4, uintptr(unsafe.Pointer(&lvl)))
		sum += lvl
	})
	if n == 0 {
		return 0, false
	}
	return sum / float32(n), true
}

// ForegroundPath returns the full exe path of the window the user is in.
func ForegroundPath() string {
	hwnd, _, _ := pGetForegroundWindow.Call()
	if hwnd == 0 {
		return ""
	}
	var pid uint32
	pGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	return freshProcessPath(pid)
}

// ForegroundExe returns the exe name of the window the user is in.
func ForegroundExe() string {
	hwnd, _, _ := pGetForegroundWindow.Call()
	if hwnd == 0 {
		return ""
	}
	var pid uint32
	pGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	return freshProcessName(pid)
}

// Friendly display name: "VALORANT-Win64-Shipping.exe" -> "VALORANT"
func prettyApp(exe string) string {
	s := strings.TrimSuffix(strings.TrimSuffix(exe, ".exe"), ".EXE")
	if i := strings.Index(s, "-Win64"); i > 0 {
		s = s[:i]
	}
	switch strings.ToLower(s) {
	case "chrome":
		return "Chrome"
	case "msedge":
		return "Edge"
	case "firefox":
		return "Firefox"
	case "discord":
		return "Discord"
	case "spotify":
		return "Spotify"
	}
	if s == "" {
		return "this window"
	}
	return s
}

// ---------- app list, volume, play/pause for any app ----------

type AudioApp struct {
	Exe    string // file name, e.g. Discord.exe
	Path   string // full path, for the icon
	Active bool
}

// ListAudioApps returns apps that have an audio session (the Volume Mixer list),
// apps playing sound first.
func ListAudioApps() []AudioApp {
	comInit()
	seen := map[string]*AudioApp{}
	var order []string
	self := ""
	if p, err := osExecutable(); err == nil {
		self = strings.ToLower(p)
	}
	forEachSession(func(ctl, ctl2 uintptr, pid uint32) {
		if pid == 0 {
			return
		}
		if r := call(ctl2, 15); r == 0 { // IsSystemSoundsSession: S_OK means system sounds
			return
		}
		path := freshProcessPath(pid)
		exe := baseName(path)
		if exe == "" || strings.ToLower(exe) == self {
			return
		}
		var state int32
		call(ctl, 3, uintptr(unsafe.Pointer(&state)))
		k := strings.ToLower(exe)
		if a, ok := seen[k]; ok {
			a.Active = a.Active || state == 1
			return
		}
		seen[k] = &AudioApp{Exe: exe, Path: path, Active: state == 1}
		order = append(order, k)
	})
	var out []AudioApp
	for _, active := range []bool{true, false} {
		for _, k := range order {
			if seen[k].Active == active {
				out = append(out, *seen[k])
			}
		}
	}
	return out
}

// forEachSession visits every audio session on every active playback device.
func forEachSession(fn func(ctl, ctl2 uintptr, pid uint32)) {
	var enum uintptr
	hr, _, _ := pCoCreateInstance.Call(uintptr(unsafe.Pointer(&CLSID_MMDeviceEnumerator)), 0, 0x17,
		uintptr(unsafe.Pointer(&IID_IMMDeviceEnumerator)), uintptr(unsafe.Pointer(&enum)))
	if int32(hr) < 0 || enum == 0 {
		return
	}
	defer release(enum)
	var coll uintptr
	if int32(call(enum, 3, 0, 1, uintptr(unsafe.Pointer(&coll)))) < 0 || coll == 0 {
		return
	}
	defer release(coll)
	var nDev uint32
	call(coll, 3, uintptr(unsafe.Pointer(&nDev)))
	for d := uint32(0); d < nDev; d++ {
		var dev uintptr
		if int32(call(coll, 4, uintptr(d), uintptr(unsafe.Pointer(&dev)))) < 0 || dev == 0 {
			continue
		}
		var mgr uintptr
		if int32(call(dev, 3, uintptr(unsafe.Pointer(&IID_IAudioSessionManager2)), 0x17, 0, uintptr(unsafe.Pointer(&mgr)))) >= 0 && mgr != 0 {
			var sessions uintptr
			if int32(call(mgr, 5, uintptr(unsafe.Pointer(&sessions)))) >= 0 && sessions != 0 {
				var n int32
				call(sessions, 3, uintptr(unsafe.Pointer(&n)))
				for i := int32(0); i < n; i++ {
					var ctl uintptr
					if int32(call(sessions, 4, uintptr(i), uintptr(unsafe.Pointer(&ctl)))) < 0 || ctl == 0 {
						continue
					}
					var ctl2 uintptr
					call(ctl, 0, uintptr(unsafe.Pointer(&IID_IAudioSessionControl2)), uintptr(unsafe.Pointer(&ctl2)))
					if ctl2 != 0 {
						var pid uint32
						call(ctl2, 14, uintptr(unsafe.Pointer(&pid)))
						fn(ctl, ctl2, pid)
						release(ctl2)
					}
					release(ctl)
				}
				release(sessions)
			}
			release(mgr)
		}
		release(dev)
	}
}

// SetAppVolume sets every audio session of exe to pct (0-100). Returns false if
// the app has no audio right now.
func SetAppVolume(exe string, pct int) bool {
	level := float32(pct) / 100
	n := eachVolume(exe, func(v uintptr) {
		// float arg: Go's Windows call path mirrors integer args into XMM registers
		call(v, 3, uintptr(math.Float32bits(level)), 0)
	})
	return n > 0
}

func isBrowser(exe string) bool {
	switch strings.ToLower(exe) {
	case "chrome.exe", "msedge.exe", "brave.exe", "opera.exe", "opera_gx.exe", "firefox.exe", "vivaldi.exe", "arc.exe":
		return true
	}
	return false
}
