//go:build windows

package main

// The browser extension (bundled in the "browser-extension" folder) talks to
// Akra Player through Chrome's "native messaging": the browser starts this
// same exe as a tiny helper (it passes chrome-extension://... on the command
// line) and exchanges JSON with it over stdin/stdout. The helper forwards
// commands from the player with WM_COPYDATA.
//
// Works in Chrome, Brave, Edge, Vivaldi, Opera and other Chromium browsers.

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	extID       = "hlacagjhahhpholleaiokphcbjoblpfo"
	extHostName = "com.akra.player"
	hostClass   = "AkraPlayerExtHost"
	WM_COPYDATA = 0x004A
)

type COPYDATASTRUCT struct {
	Data uintptr
	Size uint32
	Ptr  uintptr
}

var (
	pSendMessageTimeoutW         = user32.NewProc("SendMessageTimeoutW")
	pChangeWindowMessageFilterEx = user32.NewProc("ChangeWindowMessageFilterEx")
)

// allowCopyData lets the browser helper reach us even if we run as admin.
func allowCopyData(hwnd uintptr) {
	if pChangeWindowMessageFilterEx.Find() == nil {
		pChangeWindowMessageFilterEx.Call(hwnd, WM_COPYDATA, 1 /*MSGFLT_ALLOW*/, 0)
	}
}

func sendCopyData(to, from uintptr, s string) bool {
	b := []byte(s)
	if len(b) == 0 {
		b = []byte{0}
	}
	cds := COPYDATASTRUCT{Data: 0x414b, Size: uint32(len(b)), Ptr: uintptr(unsafe.Pointer(&b[0]))}
	var res uintptr
	r, _, _ := pSendMessageTimeoutW.Call(to, WM_COPYDATA, from, uintptr(unsafe.Pointer(&cds)), 0x0002 /*SMTO_ABORTIFHUNG*/, 500, uintptr(unsafe.Pointer(&res)))
	runtime.KeepAlive(b)
	return r != 0
}

func readCopyData(lp uintptr) string {
	cds := (*COPYDATASTRUCT)(unsafe.Pointer(lp))
	if cds == nil || cds.Data != 0x414b || cds.Size == 0 || cds.Size > 4096 {
		return ""
	}
	return string(unsafe.Slice((*byte)(unsafe.Pointer(cds.Ptr)), cds.Size))
}

// ---------- registration (done by the player at startup) ----------

func registerNativeHost() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	dir := configDir("AkraPlayer")
	os.MkdirAll(dir, 0755)
	manifest := map[string]any{
		"name":            extHostName,
		"description":     "Akra Player",
		"path":            hostCopy(exe, dir),
		"type":            "stdio",
		"allowed_origins": []string{"chrome-extension://" + extID + "/"},
	}
	b, _ := json.MarshalIndent(manifest, "", "  ")
	path := filepath.Join(dir, extHostName+".json")
	if old, err := os.ReadFile(path); err != nil || string(old) != string(b) {
		os.WriteFile(path, b, 0644)
	}
	// (Opera and Vivaldi read Chrome's entry)
	for _, browser := range []string{
		`Software\Google\Chrome`, `Software\BraveSoftware\Brave-Browser`, `Software\Microsoft\Edge`, `Software\Chromium`,
	} {
		key := browser + `\NativeMessagingHosts\` + extHostName
		if cur, ok := regGetString(key, ""); ok && strings.EqualFold(cur, path) {
			continue
		}
		regSetString(key, "", path)
	}
}

// hostCopy: the browser runs the extension's helper from a copy of the exe in
// %APPDATA%\AkraPlayer\host, never from AkraPlayer.exe itself. Browsers keep
// that helper running, and a running exe can't be replaced, so this keeps
// AkraPlayer.exe free for updates. Falls back to the exe if copying fails.
func hostCopy(exe, dir string) string {
	st, err := os.Stat(exe)
	if err != nil {
		return exe
	}
	hd := filepath.Join(dir, "host")
	if os.MkdirAll(hd, 0755) != nil {
		return exe
	}
	name := fmt.Sprintf("AkraPlayerHost-%x.exe", uint64(st.Size())*1000003^uint64(st.ModTime().UnixNano()))
	dst := filepath.Join(hd, name)
	if ds, err := os.Stat(dst); err != nil || ds.Size() != st.Size() {
		b, err := os.ReadFile(exe)
		if err != nil {
			return exe
		}
		tmp := dst + ".tmp"
		if os.WriteFile(tmp, b, 0755) != nil || os.Rename(tmp, dst) != nil {
			os.Remove(tmp)
			return exe
		}
	}
	// old copies go (the one the browser still runs stays until it closes)
	if ents, err := os.ReadDir(hd); err == nil {
		for _, e := range ents {
			if e.Name() != name && strings.HasPrefix(e.Name(), "AkraPlayerHost-") {
				os.Remove(filepath.Join(hd, e.Name()))
			}
		}
	}
	return dst
}

func regGetString(key, name string) (string, bool) {
	var buf [520]uint16
	n := uint32(len(buf) * 2)
	var np uintptr
	if name != "" {
		np = uintptr(unsafe.Pointer(u16(name)))
	}
	r, _, _ := pRegGetValueW.Call(HKEY_CURRENT_USER, uintptr(unsafe.Pointer(u16(key))), np,
		0x2 /*RRF_RT_REG_SZ*/, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n)))
	if r != 0 {
		return "", false
	}
	return syscall.UTF16ToString(buf[:]), true
}

func regSetString(key, name, val string) {
	v := u16(val)
	var np uintptr
	if name != "" {
		np = uintptr(unsafe.Pointer(u16(name)))
	}
	pRegSetKeyValueW.Call(HKEY_CURRENT_USER, uintptr(unsafe.Pointer(u16(key))), np, 1, /*REG_SZ*/
		uintptr(unsafe.Pointer(v)), uintptr((len(syscall.StringToUTF16(val)))*2))
}

// ---------- player side ----------

func (a *App) extConnected() bool {
	live := a.extHosts[:0]
	for _, h := range a.extHosts {
		if ok, _, _ := pIsWindow.Call(h); ok != 0 {
			live = append(live, h)
		}
	}
	a.extHosts = live
	a.ui.ExtConnected = len(live) > 0
	return len(live) > 0
}

func (a *App) extSend(cmd string) {
	for _, h := range a.extHosts {
		sendCopyData(h, a.hwnd, cmd)
	}
}

// onCopyData: messages from browser helpers ("hello", "bye").
func (a *App) onCopyData(from uintptr, msg string) {
	switch msg {
	case "hello":
		for _, h := range a.extHosts {
			if h == from {
				return
			}
		}
		a.extHosts = append(a.extHosts, from)
	case "bye":
		for i, h := range a.extHosts {
			if h == from {
				a.extHosts = append(a.extHosts[:i], a.extHosts[i+1:]...)
				break
			}
		}
	}
	a.extConnected()
}

// ---------- helper side (started by the browser) ----------

func isNativeHostLaunch() bool {
	for _, arg := range os.Args[1:] {
		if strings.HasPrefix(arg, "chrome-extension://") {
			return true
		}
	}
	return false
}

var hostOut = make(chan []byte, 16)

func hostWndProc(hwnd, msg, wp, lp uintptr) uintptr {
	if msg == WM_COPYDATA {
		if s := readCopyData(lp); s != "" {
			b, _ := json.Marshal(map[string]string{"cmd": s})
			select {
			case hostOut <- b:
			default:
			}
		}
		return 1
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, msg, wp, lp)
	return r
}

func writeNative(w io.Writer, b []byte) error {
	var n [4]byte
	binary.LittleEndian.PutUint32(n[:], uint32(len(b)))
	if _, err := w.Write(n[:]); err != nil {
		return err
	}
	_, err := w.Write(b)
	return err
}

func runNativeHost() {
	runtime.LockOSThread()
	hInst, _, _ := pGetModuleHandleW.Call(0)
	wc := WNDCLASSEX{LpfnWndProc: syscall.NewCallback(hostWndProc), HInstance: hInst, LpszClassName: u16(hostClass)}
	wc.CbSize = uint32(unsafe.Sizeof(wc))
	pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	hwnd, _, _ := pCreateWindowExW.Call(0, uintptr(unsafe.Pointer(u16(hostClass))), 0, 0, 0, 0, 0, 0,
		^uintptr(2) /*HWND_MESSAGE*/, 0, hInst, 0)

	// browser -> helper: read (and answer pings); exit when the browser closes the pipe
	go func() {
		for {
			var n [4]byte
			if _, err := io.ReadFull(os.Stdin, n[:]); err != nil {
				break
			}
			size := binary.LittleEndian.Uint32(n[:])
			if size > 1<<20 {
				break
			}
			body := make([]byte, size)
			if _, err := io.ReadFull(os.Stdin, body); err != nil {
				break
			}
			if strings.Contains(string(body), `"ping"`) {
				hostOut <- []byte(`{"cmd":"pong"}`)
			}
		}
		if main, _, _ := pFindWindowW.Call(uintptr(unsafe.Pointer(u16(wndClass))), 0); main != 0 {
			sendCopyData(main, hwnd, "bye")
		}
		os.Exit(0)
	}()
	// helper -> browser
	go func() {
		for b := range hostOut {
			if writeNative(os.Stdout, b) != nil {
				os.Exit(0)
			}
		}
	}()
	// keep saying hello to the player (it may start later, or restart)
	go func() {
		var last uintptr
		for {
			main, _, _ := pFindWindowW.Call(uintptr(unsafe.Pointer(u16(wndClass))), 0)
			if main != 0 && (main != last || time.Now().Unix()%30 == 0) {
				if sendCopyData(main, hwnd, "hello") {
					if main != last {
						hostOut <- []byte(`{"cmd":"connected"}`)
					}
					last = main
				}
			}
			if main == 0 && last != 0 {
				last = 0
				hostOut <- []byte(`{"cmd":"disconnected"}`)
			}
			time.Sleep(2 * time.Second)
		}
	}()
	var msg MSG
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if r == 0 || int32(r) == -1 {
			return
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}
