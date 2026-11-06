//go:build windows

package main

// Background workers, so the UI thread only ever draws and handles input:
//   - poll worker: finds Spotify, reads its mute state, and samples its audio
//     level ~100x a second for the visualizer
//   - capture worker: grabs and blurs the screen behind the glass, then builds
//     the glass slab

import (
	"hash/fnv"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

const (
	WM_POLL = WM_APP + 7
	WM_BASE = WM_APP + 8
)

var IID_IAudioMeterInformation = GUID{0xC02216F6, 0x8C67, 0x4B5B, [8]byte{0x9D, 0x00, 0xD0, 0x08, 0xE7, 0x3E, 0x00, 0x64}}

// ---------- poll worker ----------

type pollSnap struct {
	hwnd       uintptr
	title      string
	muted      bool
	mutedKnown bool
	vol        float32
	volKnown   bool
}

var (
	pollCh        = make(chan pollSnap, 4)
	workerVisible int32

	levelsMu sync.Mutex
	levelBuf [96]float32
	levelPos int
)

func pushLevel(v float32) {
	levelsMu.Lock()
	levelBuf[levelPos%len(levelBuf)] = v
	levelPos++
	levelsMu.Unlock()
}

// copyLevels fills dst with the recent levels, oldest first.
func copyLevels(dst []float32) []float32 {
	levelsMu.Lock()
	defer levelsMu.Unlock()
	n := len(levelBuf)
	dst = dst[:0]
	for i := 0; i < n; i++ {
		dst = append(dst, levelBuf[(levelPos+i)%n])
	}
	return dst
}

// spotifyMeters returns peak meters for Spotify's audio sessions (caller releases).
func spotifyMeters() []uintptr {
	var out []uintptr
	forEachSession(func(ctl, ctl2 uintptr, pid uint32) {
		if pid == 0 || freshProcessName(pid) != "Spotify.exe" {
			return
		}
		var m uintptr
		call(ctl, 0, uintptr(unsafe.Pointer(&IID_IAudioMeterInformation)), uintptr(unsafe.Pointer(&m)))
		if m != 0 {
			out = append(out, m)
		}
	})
	return out
}

func startPollWorker(hwnd uintptr) {
	go func() {
		runtime.LockOSThread()
		pCoInitializeEx.Call(0, 0) // COINIT_MULTITHREADED
		var meters []uintptr
		var lastPoll, lastMeters, bucketStart time.Time
		var bucket float32
		for {
			now := time.Now()
			visible := atomic.LoadInt32(&workerVisible) == 1
			if now.Sub(lastPoll) >= 450*time.Millisecond {
				lastPoll = now
				h, t := findSpotify()
				snap := pollSnap{hwnd: h, title: t}
				if visible && h != 0 {
					snap.muted, snap.mutedKnown = IsAppMuted("Spotify.exe")
					snap.vol, snap.volKnown = GetAppVolume("Spotify.exe")
				}
				select {
				case pollCh <- snap:
				default:
				}
				pPostMessageW.Call(hwnd, WM_POLL, 0, 0)
			}
			if !visible {
				for _, m := range meters {
					release(m)
				}
				meters = nil
				time.Sleep(120 * time.Millisecond)
				continue
			}
			if now.Sub(lastMeters) > 2*time.Second {
				for _, m := range meters {
					release(m)
				}
				meters = spotifyMeters()
				lastMeters = now
			}
			var peak float32
			for _, m := range meters {
				var v float32
				call(m, 3, uintptr(unsafe.Pointer(&v))) // GetPeakValue
				if v > peak {
					peak = v
				}
			}
			if peak > bucket {
				bucket = peak
			}
			if now.Sub(bucketStart) >= 40*time.Millisecond {
				pushLevel(bucket)
				bucket = 0
				bucketStart = now
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
}

// ---------- capture worker ----------

type capReq struct {
	capture bool // grab a new backdrop first
	rect    RECT // screen area to grab (the window, plus the pill above it)
	win     RECT // the main window
	seq     int
	params  BaseParams
}

type capRes struct {
	backdrop *Canvas
	bscale   float32
	bver     int
	bx, by   int32 // screen position of the backdrop's top-left
	layer    *BaseLayer
}

const WM_CAPGRAB = WM_APP + 11 // the screen was grabbed (wParam = request seq)

var (
	capReqCh = make(chan capReq, 1)
	capResCh = make(chan capRes, 4)
)

// sendCap replaces any request that's still waiting, so only the newest runs.
func sendCap(r capReq) {
	for {
		select {
		case capReqCh <- r:
			return
		default:
			select {
			case old := <-capReqCh:
				if old.capture && !r.capture { // keep the capture part
					r.capture, r.rect, r.win, r.seq = true, old.rect, old.win, old.seq
				}
			default:
			}
		}
	}
}

func startCaptureWorker(hwnd uintptr) {
	go func() {
		runtime.LockOSThread()
		var cs capState
		var cur *Canvas
		var curScale, curBX, curBY float32
		var curOrg RECT
		wver, lastKey := 0, ""
		for req := range capReqCh {
			p := req.params
			var res capRes
			if req.capture {
				// we're hidden from capture only for this moment: give the
				// compositor a frame to leave us out, grab, then tell the UI
				pDwmFlush.Call()
				bd, sc, changed := cs.grab(req.rect, cur == nil || req.rect != curOrg)
				pPostMessageW.Call(hwnd, WM_CAPGRAB, uintptr(req.seq), 0)
				if changed {
					wver++
					cur, curScale, curOrg = bd, sc, req.rect
					curBX, curBY = float32(req.win.L-req.rect.L), float32(req.win.T-req.rect.T)
					res.backdrop, res.bscale, res.bver = bd, sc, wver
					res.bx, res.by = req.rect.L, req.rect.T
				}
			}
			if p.Blur && cur != nil {
				p.Backdrop, p.BScale, p.BVer = cur, curScale, wver
				if res.backdrop != nil { // new capture: the window is where it was at the request
					p.BX, p.BY = curBX, curBY
				}
			} else {
				p.Blur, p.Backdrop = false, nil
			}
			key := p.Key()
			if key == lastKey && res.backdrop == nil {
				continue // nothing changed
			}
			lastKey = key
			res.layer = BuildBase(p)
			capResCh <- res
			pPostMessageW.Call(hwnd, WM_BASE, 0, 0)
		}
	}()
}

type capState struct {
	dc, dib, old uintptr
	bits         unsafe.Pointer
	w, h         int
	sum          uint64
}

// grab copies the screen behind rect at 1/5 size and blurs it (frost).
// Returns changed=false if the screen looks the same as last time.
func (cs *capState) grab(r RECT, force bool) (*Canvas, float32, bool) {
	w, h := int(r.R-r.L), int(r.B-r.T)
	if w <= 0 || h <= 0 {
		return nil, 0, false
	}
	bw, bh := max(1, w/5), max(1, h/5)
	if cs.dc == 0 {
		cs.dc, _, _ = pCreateCompatibleDC.Call(0)
	}
	if cs.dib == 0 || cs.w != bw || cs.h != bh {
		if cs.dib != 0 {
			pSelectObject.Call(cs.dc, cs.old)
			pDeleteObject.Call(cs.dib)
		}
		cs.dib, cs.bits = newDIB(bw, bh)
		cs.old, _, _ = pSelectObject.Call(cs.dc, cs.dib)
		cs.w, cs.h = bw, bh
		force = true
	}
	screen, _, _ := pGetDC.Call(0)
	pSetStretchBltMode.Call(cs.dc, 4 /*HALFTONE*/)
	pSetBrushOrgEx.Call(cs.dc, 0, 0, 0)
	pStretchBlt.Call(cs.dc, 0, 0, uintptr(bw), uintptr(bh), screen, uintptr(r.L), uintptr(r.T), uintptr(w), uintptr(h), 0x00CC0020)
	pReleaseDC.Call(0, screen)
	pGdiFlush.Call()

	px := unsafe.Slice((*byte)(cs.bits), bw*bh*4)
	hs := fnv.New64a()
	hs.Write(px)
	sum := hs.Sum64()
	if sum == cs.sum && !force {
		return nil, 0, false
	}
	cs.sum = sum
	cv := NewCanvas(bw, bh, 1)
	for i := 0; i < bw*bh; i++ {
		cv.Pix[i*4] = float32(px[i*4+2]) / 255
		cv.Pix[i*4+1] = float32(px[i*4+1]) / 255
		cv.Pix[i*4+2] = float32(px[i*4]) / 255
		cv.Pix[i*4+3] = 1
	}
	cv.BoxBlur(3)
	return cv, float32(w) / float32(bw), true
}
