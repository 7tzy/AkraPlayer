//go:build windows

package main

// Two pieces that make the app feel right:
//  1. A frame pump synced to the monitor's refresh (DwmFlush), so animations
//     run at a true 60 fps (or 120/144 on fast screens) instead of the
//     ~32 fps that Windows' 15.6 ms timers give.
//  2. A low-level keyboard hook used to record keybinds (catches every key,
//     F1-F24 and media keys included) and to run any hotkey that Windows
//     won't let us register (e.g. another app already owns that F-key).

import (
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

const (
	WM_RECKEY  = WM_APP + 3 // wParam = vk, lParam = mods (from the hook)
	WM_HOOKHOT = WM_APP + 4 // wParam = action
	WM_FRAME   = WM_APP + 5

	WM_KEYUP      = 0x0101
	WM_SYSKEYUP   = 0x0105
	WM_MOUSEWHEEL = 0x020A
	VK_F12        = 0x7B
)

var (
	pDwmFlush            = syscall.NewLazyDLL("dwmapi.dll").NewProc("DwmFlush")
	pSetWindowsHookExW   = user32.NewProc("SetWindowsHookExW")
	pCallNextHookEx      = user32.NewProc("CallNextHookEx")
	pUnhookWindowsHookEx = user32.NewProc("UnhookWindowsHookEx")
	pGetAsyncKeyState    = user32.NewProc("GetAsyncKeyState")
)

// ---------- frame pump ----------

var (
	frameWanted  int32
	framePending int32
	frameWake    = make(chan struct{}, 1)
)

func startFramePump(hwnd uintptr) {
	go func() {
		runtime.LockOSThread()
		for range frameWake {
			for atomic.LoadInt32(&frameWanted) == 1 {
				t0 := time.Now()
				r, _, _ := pDwmFlush.Call() // blocks until the next vsync
				// If DWM didn't wait (no compositor / remote session), pace ourselves.
				if el := time.Since(t0); r != 0 || el < 3*time.Millisecond {
					time.Sleep(16*time.Millisecond - el)
				}
				if atomic.CompareAndSwapInt32(&framePending, 0, 1) {
					pPostMessageW.Call(hwnd, WM_FRAME, 0, 0)
				}
			}
		}
	}()
}

func wantFrames(on bool) {
	if on {
		if atomic.SwapInt32(&frameWanted, 1) == 0 {
			select {
			case frameWake <- struct{}{}:
			default:
			}
		}
	} else {
		atomic.StoreInt32(&frameWanted, 0)
	}
}

// ---------- keyboard + mouse hooks ----------
//
// The hooks live on their own thread with its own message loop. Windows
// waits on a low-level hook for every key and click on the PC, so they must
// never sit behind a busy UI thread (a slow frame would lag your mouse).

type KBDLLHOOKSTRUCT struct {
	VkCode    uint32
	ScanCode  uint32
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
}

type MSLLHOOKSTRUCT struct {
	Pt        POINT
	MouseData uint32
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
}

// hookBind: a key the hook handles. hold = report press and release and let
// the key through (hold-to-lower, so Discord still gets push-to-talk).
type hookBind struct {
	act      int
	mods, vk uint32
	hold     bool
}

const wmHookCtl = WM_APP + 30 // to the hook thread: wParam = keyboard on, lParam = mouse on

var (
	pPostThreadMessageW = user32.NewProc("PostThreadMessageW")
	pGetCurrentThreadId = kernel32.NewProc("GetCurrentThreadId")
	pPeekMessageW       = user32.NewProc("PeekMessageW")

	hookMu         sync.Mutex
	hookRecording  bool
	hookBinds      []hookBind
	hookDown       = map[uint32]int{} // vk -> action (-1 = swallowed while recording)
	hookHeld       = map[uint32]int{} // hold keys currently down
	hookHwnd       uintptr
	hookTid        uintptr
	hookReady      = make(chan struct{})
	kbHook, msHook uintptr
	kbCB           = syscall.NewCallback(llKeyboardProc)
	msCB           = syscall.NewCallback(llMouseProc)
)

func asyncMods() uint32 {
	down := func(vk uintptr) bool { r, _, _ := pGetAsyncKeyState.Call(vk); return r&0x8000 != 0 }
	var m uint32
	if down(VK_CONTROL) {
		m |= MOD_CONTROL
	}
	if down(VK_MENU) {
		m |= MOD_ALT
	}
	if down(VK_SHIFT) {
		m |= MOD_SHIFT
	}
	if down(VK_LWIN) || down(VK_RWIN) {
		m |= MOD_WIN
	}
	return m
}

// hookInput handles one key or button event. Returns true to swallow it.
func hookInput(vk uint32, down, up, injected bool) bool {
	hookMu.Lock()
	defer hookMu.Unlock()
	if hookRecording {
		if down {
			if _, ok := hookDown[vk]; !ok {
				hookDown[vk] = -1
				lp := uintptr(asyncMods())
				if injected {
					lp |= 0x100 // typed by another program (a key remapper)
				}
				pPostMessageW.Call(hookHwnd, WM_RECKEY, uintptr(vk), lp)
			}
		}
		if up {
			delete(hookDown, vk)
		}
		return true // keys pressed while recording don't reach other apps
	}
	if up {
		if act, ok := hookHeld[vk]; ok {
			delete(hookHeld, vk)
			pPostMessageW.Call(hookHwnd, WM_HOOKHOT, uintptr(act), 2)
			return false
		}
		if _, ok := hookDown[vk]; ok {
			delete(hookDown, vk)
			return true
		}
		return false
	}
	if !down {
		return false
	}
	if _, ok := hookHeld[vk]; ok {
		return false // auto-repeat of a held key: let it through
	}
	if _, ok := hookDown[vk]; ok {
		return true // auto-repeat of a hotkey we handled
	}
	m := asyncMods()
	for _, b := range hookBinds {
		if b.vk != vk {
			continue
		}
		if b.hold {
			if m&b.mods == b.mods { // extra modifiers are fine (Shift while running)
				hookHeld[vk] = b.act
				pPostMessageW.Call(hookHwnd, WM_HOOKHOT, uintptr(b.act), 1)
				return false
			}
			continue
		}
		if b.mods == m {
			hookDown[vk] = b.act
			pPostMessageW.Call(hookHwnd, WM_HOOKHOT, uintptr(b.act), 0)
			return true
		}
	}
	return false
}

func llKeyboardProc(nCode, wp, lp uintptr) uintptr {
	if int32(nCode) == 0 { // HC_ACTION
		k := (*KBDLLHOOKSTRUCT)(unsafe.Pointer(lp))
		vk := k.VkCode
		down := wp == WM_KEYDOWN || wp == WM_SYSKEYDOWN
		up := wp == WM_KEYUP || wp == WM_SYSKEYUP
		injected := k.Flags&0x10 != 0
		// keys typed by other programs count too (that's how key remappers
		// like PowerToys send the new key), except the ones we type ourselves
		if k.ExtraInfo != akraInputTag {
			if isModifierKey(vk) {
				hookMu.Lock()
				rec := hookRecording
				hookMu.Unlock()
				if rec && down && !injected { // show held modifiers live; let them through
					pPostMessageW.Call(hookHwnd, WM_RECKEY, uintptr(vk), 0)
				}
			} else if hookInput(vk, down, up, injected) {
				return 1
			}
		}
	}
	r, _, _ := pCallNextHookEx.Call(0, nCode, wp, lp)
	return r
}

func llMouseProc(nCode, wp, lp uintptr) uintptr {
	if int32(nCode) == 0 {
		m := (*MSLLHOOKSTRUCT)(unsafe.Pointer(lp))
		var vk uint32
		down, up := false, false
		switch wp {
		case 0x0207, 0x0208: // middle button
			vk, down, up = vkMButton, wp == 0x0207, wp == 0x0208
		case 0x020B, 0x020C: // side buttons
			vk = vkXButton1
			if m.MouseData>>16 == 2 {
				vk = vkXButton2
			}
			down, up = wp == 0x020B, wp == 0x020C
		}
		if vk != 0 && m.ExtraInfo != akraInputTag && hookInput(vk, down, up, m.Flags&1 != 0) {
			return 1
		}
	}
	r, _, _ := pCallNextHookEx.Call(0, nCode, wp, lp)
	return r
}

// startHookThread runs the hook thread for the life of the app.
func startHookThread(hwnd uintptr) {
	hookHwnd = hwnd
	go func() {
		runtime.LockOSThread()
		tid, _, _ := pGetCurrentThreadId.Call()
		var msg MSG
		pPeekMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, 0) // create the queue
		hookTid = tid
		close(hookReady)
		hInst, _, _ := pGetModuleHandleW.Call(0)
		for {
			r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
			if r == 0 || int32(r) == -1 {
				return
			}
			if msg.Message == wmHookCtl {
				kb, ms := msg.WParam != 0, msg.LParam != 0
				if kb && kbHook == 0 {
					kbHook, _, _ = pSetWindowsHookExW.Call(13 /*WH_KEYBOARD_LL*/, kbCB, hInst, 0)
				} else if !kb && kbHook != 0 {
					pUnhookWindowsHookEx.Call(kbHook)
					kbHook = 0
				}
				if ms && msHook == 0 {
					msHook, _, _ = pSetWindowsHookExW.Call(14 /*WH_MOUSE_LL*/, msCB, hInst, 0)
				} else if !ms && msHook != 0 {
					pUnhookWindowsHookEx.Call(msHook)
					msHook = 0
				}
				continue
			}
			pTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
			pDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
		}
	}()
	<-hookReady
}

// setHooks turns the hooks on or off (from the UI thread).
func setHooks(kb, mouse bool) {
	if !kb && !mouse {
		hookMu.Lock()
		hookDown = map[uint32]int{}
		hookHeld = map[uint32]int{}
		hookMu.Unlock()
	}
	pPostThreadMessageW.Call(hookTid, wmHookCtl, uintptr(b2i(kb)), uintptr(b2i(mouse)))
}
