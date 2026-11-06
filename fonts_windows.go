//go:build windows

package main

import (
	"embed"
	"math"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

// Inter (SIL Open Font License) is embedded so the app looks the same on every PC.
//
//go:embed fonts/*.otf fonts/*.ttf
var fontFS embed.FS

var fontsLoaded bool

func loadFonts() {
	entries, err := fontFS.ReadDir("fonts")
	if err != nil {
		return
	}
	ok := 0
	for _, e := range entries {
		data, err := fontFS.ReadFile("fonts/" + e.Name())
		if err != nil || len(data) == 0 {
			continue
		}
		var n uint32
		h, _, _ := pAddFontMemResourceEx.Call(uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)), 0, uintptr(unsafe.Pointer(&n)))
		if h != 0 {
			ok++
		}
	}
	fontsLoaded = ok >= 4
}

type gdiText struct {
	dc    uintptr
	fonts map[string]uintptr
}

func newGDIText() *gdiText {
	dc, _, _ := pCreateCompatibleDC.Call(0)
	return &gdiText{dc: dc, fonts: map[string]uintptr{}}
}

func faceFor(weight, font int, fallback bool) (string, int) {
	if fallback || (!fontsLoaded && font != FontSegoe) || font == FontSegoe {
		if weight >= 600 {
			return "Segoe UI Semibold", 600
		}
		return "Segoe UI", 400
	}
	if font == FontPoppins {
		if weight >= 700 {
			return "Poppins", 700
		}
		return "Poppins Medium", 500
	}
	switch {
	case weight >= 700:
		return "Inter Display SemiBold", 600
	case weight >= 600:
		return "Inter SemiBold", 600
	}
	return "Inter Medium", 500
}

// letter spacing per font (fraction of the font size). Inter is tight at UI sizes.
func tracking(font int, px float32) float32 {
	switch font {
	case FontPoppins:
		return 0.006
	case FontSegoe:
		return 0.008
	}
	if px < 15 {
		return 0.028
	}
	return 0.016
}

func (g *gdiText) font(px float32, weight, font int, fallback bool) uintptr {
	face, w := faceFor(weight, font, fallback)
	h := int32(math.Round(float64(px)))
	key := face + "|" + strconv.Itoa(int(h)) + "|" + strconv.Itoa(w)
	if f, ok := g.fonts[key]; ok {
		return f
	}
	if len(g.fonts) > 96 { // many sizes (big windows, resizing): free the old fonts
		for _, f := range g.fonts {
			pDeleteObject.Call(f)
		}
		g.fonts = map[string]uintptr{}
	}
	f, _, _ := pCreateFontW.Call(uintptr(-h), 0, 0, 0, uintptr(w), 0, 0, 0,
		1 /*DEFAULT_CHARSET*/, 0, 0, 4 /*ANTIALIASED_QUALITY*/, 0, uintptr(unsafe.Pointer(u16(face))))
	g.fonts[key] = f
	return f
}

// hasAllGlyphs reports whether the selected font covers every character
// (otherwise e.g. Japanese titles fall back to Segoe UI, which Windows extends).
func (g *gdiText) hasAllGlyphs(u []uint16, n int) bool {
	idx := make([]uint16, n)
	r, _, _ := pGetGlyphIndicesW.Call(g.dc, uintptr(unsafe.Pointer(&u[0])), uintptr(n), uintptr(unsafe.Pointer(&idx[0])), 1 /*GGI_MARK_NONEXISTING_GLYPHS*/)
	if r == 0xFFFFFFFF {
		return true
	}
	for _, v := range idx {
		if v == 0xFFFF {
			return false
		}
	}
	return true
}

// Render draws text 3x larger and averages it down: this gives fractional
// letter positions (no more cramped, uneven spacing from pixel snapping)
// and very smooth edges.
func (g *gdiText) Render(s string, px float32, weight, font int) *Mask {
	const ss = 3
	s = strings.ReplaceAll(s, "\x00", "")
	if s == "" {
		return &Mask{}
	}
	u, _ := syscall.UTF16FromString(s)
	n := len(u) - 1
	big := px * ss

	oldF, _, _ := pSelectObject.Call(g.dc, g.font(big, weight, font, false))
	if font != FontSegoe && fontsLoaded && !g.hasAllGlyphs(u, n) {
		pSelectObject.Call(g.dc, g.font(big, weight, font, true))
	}
	defer pSelectObject.Call(g.dc, oldF)
	extra := int32(math.Round(float64(big * tracking(font, px))))
	pSetTextCharacterExtra.Call(g.dc, uintptr(extra))

	var sz SIZE
	pGetTextExtentPoint32W.Call(g.dc, uintptr(unsafe.Pointer(&u[0])), uintptr(n), uintptr(unsafe.Pointer(&sz)))
	bw, bh := int(sz.CX)+3*ss, int(sz.CY)
	if bw <= 3*ss || bh <= 0 {
		return &Mask{}
	}
	bmp, bits := newDIB(bw, bh)
	if bmp == 0 {
		return &Mask{}
	}
	oldB, _, _ := pSelectObject.Call(g.dc, bmp)
	pix := unsafe.Slice((*byte)(bits), bw*bh*4)
	for i := range pix {
		pix[i] = 0
	}
	pSetBkMode.Call(g.dc, 1 /*TRANSPARENT*/)
	pSetTextColor.Call(g.dc, 0xFFFFFF)
	pTextOutW.Call(g.dc, ss, 0, uintptr(unsafe.Pointer(&u[0])), uintptr(n))
	pGdiFlush.Call()

	w, h := (bw+ss-1)/ss, (bh+ss-1)/ss
	m := &Mask{W: w, H: h, A: make([]float32, w*h)}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var sum float32
			for yy := y * ss; yy < y*ss+ss && yy < bh; yy++ {
				for xx := x * ss; xx < x*ss+ss && xx < bw; xx++ {
					sum += float32(pix[(yy*bw+xx)*4+1])
				}
			}
			if sum > 0 {
				m.A[y*w+x] = float32(math.Pow(float64(sum/(255*ss*ss)), 0.92))
			}
		}
	}
	pSelectObject.Call(g.dc, oldB)
	pDeleteObject.Call(bmp)
	pSetTextCharacterExtra.Call(g.dc, 0)
	return m
}

// Measure returns the width (pixels) Render would produce, without drawing.
func (g *gdiText) Measure(s string, px float32, weight, font int) float32 {
	const ss = 3
	s = strings.ReplaceAll(s, "\x00", "")
	if s == "" {
		return 0
	}
	u, _ := syscall.UTF16FromString(s)
	n := len(u) - 1
	big := px * ss
	oldF, _, _ := pSelectObject.Call(g.dc, g.font(big, weight, font, false))
	if font != FontSegoe && fontsLoaded && !g.hasAllGlyphs(u, n) {
		pSelectObject.Call(g.dc, g.font(big, weight, font, true))
	}
	extra := int32(math.Round(float64(big * tracking(font, px))))
	pSetTextCharacterExtra.Call(g.dc, uintptr(extra))
	var sz SIZE
	pGetTextExtentPoint32W.Call(g.dc, uintptr(unsafe.Pointer(&u[0])), uintptr(n), uintptr(unsafe.Pointer(&sz)))
	pSetTextCharacterExtra.Call(g.dc, 0)
	pSelectObject.Call(g.dc, oldF)
	bw := int(sz.CX) + 3*ss
	return float32((bw + ss - 1) / ss)
}
