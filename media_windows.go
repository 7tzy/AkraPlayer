//go:build windows

package main

// Exact song position, length, cover art and seeking come from Windows'
// media controls (the same thing the volume/media flyout uses).
//
// That API is WinRT, reached here through raw COM calls. To be safe it runs in
// a small helper process (this same exe with --media-helper): if anything in
// it ever fails, the player keeps working and falls back to estimates.

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

type mediaMsg struct {
	OK     bool    `json:"ok"`    // media controls reachable
	Found  bool    `json:"found"` // a Spotify session exists
	Title  string  `json:"title"`
	Artist string  `json:"artist"`
	Status int     `json:"status"` // 4 playing, 5 paused
	Pos    float64 `json:"pos"`    // seconds, as of now
	End    float64 `json:"end"`    // song length, seconds
	ArtKey string  `json:"artKey,omitempty"`
	Art    string  `json:"art,omitempty"` // base64 image, sent once per song

	Kind    string `json:"kind,omitempty"` // "toggle": answer to a toggle request
	Toggled bool   `json:"toggled,omitempty"`
}

// ---------- helper process (WinRT side) ----------

var (
	combase                       = syscall.NewLazyDLL("combase.dll")
	pRoInitialize                 = combase.NewProc("RoInitialize")
	pRoGetActivationFactory       = combase.NewProc("RoGetActivationFactory")
	pWindowsCreateString          = combase.NewProc("WindowsCreateString")
	pWindowsDeleteString          = combase.NewProc("WindowsDeleteString")
	pWindowsGetStringRawBuffer    = combase.NewProc("WindowsGetStringRawBuffer")
	pCreateStreamOverRandomAccess = syscall.NewLazyDLL("shcore.dll").NewProc("CreateStreamOverRandomAccessStream")

	IID_GSMTCManagerStatics = GUID{0x2050C4EE, 0x11A0, 0x57DE, [8]byte{0xAE, 0xD7, 0xC9, 0x7C, 0x70, 0x33, 0x82, 0x45}}
	IID_IAsyncInfo          = GUID{0x00000036, 0x0000, 0x0000, [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	IID_IStream             = GUID{0x0000000c, 0x0000, 0x0000, [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
)

func hstring(s string) uintptr {
	u, _ := syscall.UTF16FromString(s)
	var h uintptr
	pWindowsCreateString.Call(uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1), uintptr(unsafe.Pointer(&h)))
	return h
}

func readHString(h uintptr) string {
	if h == 0 {
		return ""
	}
	var n uint32
	p, _, _ := pWindowsGetStringRawBuffer.Call(h, uintptr(unsafe.Pointer(&n)))
	s := ""
	if p != 0 && n > 0 {
		s = syscall.UTF16ToString(unsafe.Slice((*uint16)(unsafe.Pointer(p)), n))
	}
	pWindowsDeleteString.Call(h)
	return s
}

// await waits for a WinRT async operation and returns its result object.
func await(op uintptr) uintptr {
	if op == 0 {
		return 0
	}
	defer release(op)
	var info uintptr
	if int32(call(op, 0, uintptr(unsafe.Pointer(&IID_IAsyncInfo)), uintptr(unsafe.Pointer(&info)))) < 0 || info == 0 {
		return 0
	}
	defer release(info)
	for i := 0; i < 300; i++ {
		var st int32
		call(info, 7, uintptr(unsafe.Pointer(&st))) // get_Status
		if st == 1 {
			break
		}
		if st >= 2 {
			return 0
		}
		time.Sleep(5 * time.Millisecond)
	}
	var res uintptr
	if int32(call(op, 8, uintptr(unsafe.Pointer(&res)))) < 0 { // GetResults
		return 0
	}
	return res
}

func smtcManager() uintptr {
	cls := hstring("Windows.Media.Control.GlobalSystemMediaTransportControlsSessionManager")
	defer pWindowsDeleteString.Call(cls)
	var statics uintptr
	hr, _, _ := pRoGetActivationFactory.Call(cls, uintptr(unsafe.Pointer(&IID_GSMTCManagerStatics)), uintptr(unsafe.Pointer(&statics)))
	if int32(hr) < 0 || statics == 0 {
		return 0
	}
	defer release(statics)
	var op uintptr
	if int32(call(statics, 6, uintptr(unsafe.Pointer(&op)))) < 0 { // RequestAsync
		return 0
	}
	return await(op)
}

// spotifySession finds Spotify among the media sessions (caller releases).
func spotifySession(mgr uintptr) uintptr {
	var vec uintptr
	if int32(call(mgr, 7, uintptr(unsafe.Pointer(&vec)))) < 0 || vec == 0 { // GetSessions
		return 0
	}
	defer release(vec)
	var n uint32
	call(vec, 7, uintptr(unsafe.Pointer(&n))) // get_Size
	for i := uint32(0); i < n && i < 32; i++ {
		var s uintptr
		if int32(call(vec, 6, uintptr(i), uintptr(unsafe.Pointer(&s)))) < 0 || s == 0 { // GetAt
			continue
		}
		var hs uintptr
		call(s, 6, uintptr(unsafe.Pointer(&hs))) // get_SourceAppUserModelId
		if strings.Contains(strings.ToLower(readHString(hs)), "spotify") {
			return s
		}
		release(s)
	}
	return 0
}

// sessionMatches: does a media session's app id belong to this exe name?
func sessionMatches(aumid, name string) bool {
	a := strings.ToLower(aumid)
	switch name {
	case "firefox":
		return strings.Contains(a, "308046b0af4a39cb") || strings.Contains(a, "firefox")
	case "microsoft.media.player":
		return strings.Contains(a, "zunemusic")
	case "msedge":
		return strings.Contains(a, "msedge") || strings.Contains(a, "microsoftedge")
	}
	return name != "" && strings.Contains(a, name)
}

// toggleSession plays/pauses the media session of one app (the one that's
// playing, if it has several). Returns false if the app has no session.
func toggleSession(mgr uintptr, name string) bool {
	var vec uintptr
	if int32(call(mgr, 7, uintptr(unsafe.Pointer(&vec)))) < 0 || vec == 0 {
		return false
	}
	defer release(vec)
	var n uint32
	call(vec, 7, uintptr(unsafe.Pointer(&n)))
	var first, playing uintptr
	for i := uint32(0); i < n && i < 32; i++ {
		var s uintptr
		if int32(call(vec, 6, uintptr(i), uintptr(unsafe.Pointer(&s)))) < 0 || s == 0 {
			continue
		}
		var hs uintptr
		call(s, 6, uintptr(unsafe.Pointer(&hs)))
		if !sessionMatches(readHString(hs), name) {
			release(s)
			continue
		}
		st := int32(0)
		var info uintptr
		if call(s, 9, uintptr(unsafe.Pointer(&info))); info != 0 {
			call(info, 7, uintptr(unsafe.Pointer(&st)))
			release(info)
		}
		switch {
		case st == 4 && playing == 0:
			playing = s
		case first == 0:
			first = s
		default:
			release(s)
		}
	}
	target := playing
	if target == 0 {
		target = first
	}
	ok := false
	if target != 0 {
		var op uintptr
		call(target, 20, uintptr(unsafe.Pointer(&op))) // TryTogglePlayPauseAsync
		ok = op != 0
		release(await(op))
	}
	if playing != 0 {
		release(playing)
	}
	if first != 0 {
		release(first)
	}
	return ok
}

func readThumbnail(ref uintptr) []byte {
	var op uintptr
	if int32(call(ref, 6, uintptr(unsafe.Pointer(&op)))) < 0 { // OpenReadAsync
		return nil
	}
	stream := await(op)
	if stream == 0 {
		return nil
	}
	defer release(stream)
	var is uintptr
	hr, _, _ := pCreateStreamOverRandomAccess.Call(stream, uintptr(unsafe.Pointer(&IID_IStream)), uintptr(unsafe.Pointer(&is)))
	if int32(hr) < 0 || is == 0 {
		return nil
	}
	defer release(is)
	var out bytes.Buffer
	buf := make([]byte, 32*1024)
	for out.Len() < 4<<20 {
		var got uint32
		hr := int32(call(is, 3, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), uintptr(unsafe.Pointer(&got)))) // Read
		if got > 0 {
			out.Write(buf[:got])
		}
		if hr != 0 || got == 0 {
			break
		}
	}
	return out.Bytes()
}

const ticksTo1970 = 116444736000000000

func runMediaHelper() {
	runtime.LockOSThread()
	pRoInitialize.Call(1) // multithreaded
	cmds := make(chan string, 8)
	go func() {
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			cmds <- sc.Text()
		}
		os.Exit(0) // the player went away
	}()
	enc := json.NewEncoder(os.Stdout)
	var mgr uintptr
	lastArt, artTries := "", 0
	tick := time.NewTicker(350 * time.Millisecond)
	for {
		var cmd string
		select {
		case cmd = <-cmds:
		case <-tick.C:
		}
		if mgr == 0 {
			if mgr = smtcManager(); mgr == 0 {
				enc.Encode(mediaMsg{OK: false})
				time.Sleep(3 * time.Second)
				continue
			}
		}
		if strings.HasPrefix(cmd, "toggle ") {
			ok := toggleSession(mgr, strings.TrimPrefix(cmd, "toggle "))
			enc.Encode(mediaMsg{OK: true, Kind: "toggle", Toggled: ok})
			continue
		}
		s := spotifySession(mgr)
		if s == 0 {
			enc.Encode(mediaMsg{OK: true})
			continue
		}
		if strings.HasPrefix(cmd, "seek ") {
			if sec, err := strconv.ParseFloat(strings.TrimPrefix(cmd, "seek "), 64); err == nil {
				var op uintptr
				call(s, 24, uintptr(int64(sec*1e7)), uintptr(unsafe.Pointer(&op))) // TryChangePlaybackPositionAsync
				release(await(op))
			}
		}
		m := mediaMsg{OK: true, Found: true}
		var info uintptr
		if call(s, 9, uintptr(unsafe.Pointer(&info))); info != 0 { // GetPlaybackInfo
			var st int32
			call(info, 7, uintptr(unsafe.Pointer(&st))) // get_PlaybackStatus
			m.Status = int(st)
			release(info)
		}
		var tl uintptr
		if call(s, 8, uintptr(unsafe.Pointer(&tl))); tl != 0 { // GetTimelineProperties
			var start, end, pos, upd int64
			call(tl, 6, uintptr(unsafe.Pointer(&start)))
			call(tl, 7, uintptr(unsafe.Pointer(&end)))
			call(tl, 10, uintptr(unsafe.Pointer(&pos)))
			call(tl, 11, uintptr(unsafe.Pointer(&upd)))
			release(tl)
			m.End = float64(end-start) / 1e7
			m.Pos = float64(pos-start) / 1e7
			if m.Status == 4 && upd > 0 { // extrapolate from when Spotify last reported
				now := time.Now().UnixNano()/100 + ticksTo1970
				if dt := float64(now-upd) / 1e7; dt > 0 && dt < 6*3600 {
					m.Pos += dt
				}
			}
			if m.End > 0 && m.Pos > m.End {
				m.Pos = m.End
			}
		}
		var op uintptr
		if call(s, 7, uintptr(unsafe.Pointer(&op))); op != 0 { // TryGetMediaPropertiesAsync
			if props := await(op); props != 0 {
				var h1, h2 uintptr
				call(props, 6, uintptr(unsafe.Pointer(&h1))) // get_Title
				call(props, 9, uintptr(unsafe.Pointer(&h2))) // get_Artist
				m.Title, m.Artist = readHString(h1), readHString(h2)
				key := m.Artist + "|" + m.Title
				if key != lastArt {
					lastArt, artTries = key, 0
				}
				if artTries >= 0 && artTries < 6 && m.Title != "" {
					artTries++
					var ref uintptr
					if call(props, 15, uintptr(unsafe.Pointer(&ref))); ref != 0 { // get_Thumbnail
						if b := readThumbnail(ref); len(b) > 0 {
							m.ArtKey, m.Art = key, base64.StdEncoding.EncodeToString(b)
							artTries = -1 // done for this song
						}
						release(ref)
					}
				}
				release(props)
			}
		}
		release(s)
		enc.Encode(m)
	}
}

// ---------- player side ----------

const WM_MEDIA = WM_APP + 9

type media struct {
	in     io.WriteCloser
	cmd    *exec.Cmd
	ch     chan mediaMsg
	last   mediaMsg
	at     time.Time
	fails  int
	art    map[string]*Canvas
	holdTo time.Time // ignore positions briefly after a seek
}

func (a *App) startMedia() {
	m := a.med
	if m.fails >= 3 {
		return // give up: estimates only
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	cmd := exec.Command(exe, "--media-helper")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	in, err1 := cmd.StdinPipe()
	out, err2 := cmd.StdoutPipe()
	if err1 != nil || err2 != nil || cmd.Start() != nil {
		m.fails++
		return
	}
	m.cmd, m.in = cmd, in
	hwnd := a.hwnd
	go func() {
		dec := json.NewDecoder(out)
		for {
			var msg mediaMsg
			if err := dec.Decode(&msg); err != nil {
				break
			}
			m.ch <- msg
			pPostMessageW.Call(hwnd, WM_MEDIA, 0, 0)
		}
		cmd.Wait()
		m.ch <- mediaMsg{Status: -1} // helper exited
		pPostMessageW.Call(hwnd, WM_MEDIA, 0, 0)
	}()
}

func (a *App) mediaLive() bool {
	m := a.med
	return m.cmd != nil && m.last.OK && m.last.Found && time.Since(m.at) < 3*time.Second
}

func (a *App) onMedia() {
	m := a.med
	for {
		select {
		case msg := <-m.ch:
			if msg.Status == -1 {
				m.cmd, m.in = nil, nil
				m.fails++
				time.AfterFunc(3*time.Second, func() { pPostMessageW.Call(a.hwnd, WM_MEDIA, 1, 0) })
				continue
			}
			if msg.Art != "" {
				if b, err := base64.StdEncoding.DecodeString(msg.Art); err == nil {
					if img, _, err := image.Decode(bytes.NewReader(b)); err == nil {
						m.art[msg.ArtKey] = imageToCanvas(img)
					}
				}
				msg.Art = ""
			}
			m.last, m.at = msg, time.Now()
		default:
			if m.cmd == nil {
				a.startMedia() // restart after a crash (a few times)
			}
			a.applyMedia()
			return
		}
	}
}

// mediaToggle asks the helper to play/pause one app's media session.
func (a *App) mediaToggle(name string) bool {
	m := a.med
	if m.in == nil {
		return false
	}
	_, err := io.WriteString(m.in, "toggle "+name+"\n")
	return err == nil
}

// seek sends the new position to Spotify.
func (a *App) seek(sec float32) {
	m := a.med
	if m.in == nil {
		return
	}
	io.WriteString(m.in, "seek "+strconv.FormatFloat(float64(sec), 'f', 2, 64)+"\n")
	m.holdTo = time.Now().Add(900 * time.Millisecond)
	a.ui.Pos, a.ui.PosT = sec, float32(time.Since(a.start).Seconds())
}

func (a *App) stopMedia() {
	if m := a.med; m.cmd != nil && m.cmd.Process != nil {
		m.cmd.Process.Kill()
	}
}
