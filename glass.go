package main

import (
	"fmt"
	"runtime"
	"sync"
)

// BaseParams is everything the glass slab depends on. It's a plain value so
// the slab can be built on a background thread.
type BaseParams struct {
	PixW, PixH int
	S          float32 // pixels per logical unit
	PW, PH     float32 // panel size (logical)
	Clarity    float32
	Blur       bool
	Backdrop   *Canvas // screen behind the window, small + blurred (nil = none)
	BScale     float32 // window pixels per backdrop pixel
	BVer       int
	R          float32 // corner radius (logical)
	Outline    bool    // bright rim + glints
	BX, BY     float32 // where this window's top-left sits in the backdrop (pixels)
}

func (p BaseParams) Key() string {
	return fmt.Sprintf("%d|%d|%.3f|%.2f|%.2f|%.3f|%v|%d|%.1f|%v|%.0f|%.0f", p.PixW, p.PixH, p.S, p.PW, p.PH, p.Clarity, p.Blur && p.Backdrop != nil, p.BVer, p.R, p.Outline, p.BX, p.BY)
}

func (p BaseParams) SameSize(o BaseParams) bool {
	return p.PixW == o.PixW && p.PixH == o.PixH && p.S == o.S && p.PW == o.PW && p.PH == o.PH && p.R == o.R
}

// BaseLayer: the neutral glass (premultiplied RGBA) plus how strongly the
// accent color shows through at each pixel. The accent is mixed in every
// frame, so color changes (and RGB shift) never rebuild the glass.
type BaseLayer struct {
	P    BaseParams
	Pix  []float32
	Glow []float32
}

func grain(x, y int) float32 {
	h := uint32(x)*374761393 + uint32(y)*668265263
	h = (h ^ (h >> 13)) * 1274126177
	return float32(h&0xffff)/65535 - 0.5
}

// BuildBase renders the glass slab: frosted backdrop with lens refraction at
// the rim, grain, tint, light and rim highlights. Safe to call from any thread.
func BuildBase(p BaseParams) *BaseLayer {
	n := p.PixW * p.PixH
	L := &BaseLayer{P: p, Pix: make([]float32, n*4), Glow: make([]float32, n)}
	S := p.S
	PW, PH := p.PW, p.PH
	r := p.R
	panel := func(x, y float32) float32 { return sdRoundRect(x, y, 0, 0, PW, PH, r) }
	cl := p.Clarity
	blur := p.Blur && p.Backdrop != nil

	// tint: how milky the glass is. With blur, clarity 100% = no veil at all.
	var a, white, glare float32
	if blur {
		k := 1 - cl
		a = 0.62 * k * k
		white = 0.16 * (1 - 0.85*cl)
		glare = 0.08 * (1 - 0.8*cl)
	} else {
		a = 0.84 - 0.62*cl
		white, glare = 0.18, 0.09
	}
	top := Col{0.30, 0.31, 0.34, a}
	mid := Col{0.18, 0.19, 0.21, a}
	bot := Col{0.11, 0.12, 0.13, min32(1, a+0.06)}
	rimStops := []Col{White(0.66), White(0.22), White(0.07), White(0.10), White(0.36)}
	px1 := 1 / S // one device pixel, in logical units
	bd, bs := p.Backdrop, p.BScale
	pix := L.Pix

	parallelRows(p.PixH, func(py0, py1 int) {
		blend := func(i int, col Col, k float32) {
			al := col.A * k
			if al <= 0 {
				return
			}
			inv := 1 - al
			pix[i] = col.R*al + pix[i]*inv
			pix[i+1] = col.G*al + pix[i+1]*inv
			pix[i+2] = col.B*al + pix[i+2]*inv
			pix[i+3] = al + pix[i+3]*inv
		}
		for py := py0; py < py1; py++ {
			y := (float32(py) + 0.5) / S
			ty := clamp01(y / PH)
			tint := stops(ty, []Col{top, mid, bot})
			rim := stops(ty, rimStops)
			gl := clamp01(1-y/64) * glare
			vy := max32(clamp01(1-y/PH*2), clamp01((y/PH-0.5)*2)*0.7)
			for px := 0; px < p.PixW; px++ {
				k := py*p.PixW + px
				i := k * 4
				x := (float32(px) + 0.5) / S
				d := panel(x, y)
				cov := clamp01(0.5 - d*S)
				if cov <= 0 {
					continue
				}
				in := -d
				if blur {
					sx, sy := x, y
					if in < 14 { // liquid-glass lensing near the rim
						t := 1 - in/14
						t *= t
						nx := panel(x+0.5, y) - panel(x-0.5, y)
						ny := panel(x, y+0.5) - panel(x, y-0.5)
						sx -= nx * t * 10
						sy -= ny * t * 10
					}
					col := bd.Sample(sx*S/bs, sy*S/bs)
					l := (col.R + col.G + col.B) / 3
					// frost: a little more color, lifted toward a soft gray, plus fine grain
					g := grain(px, py) * 0.05
					pix[i] = clamp01((l+(col.R-l)*1.22)*0.93 + 0.035 + g)
					pix[i+1] = clamp01((l+(col.G-l)*1.22)*0.93 + 0.035 + g)
					pix[i+2] = clamp01((l+(col.B-l)*1.22)*0.93 + 0.035 + g)
					pix[i+3] = 1
				}
				blend(i, tint, 1)
				t1 := clamp01(1 - hypot(x-70, y+50)/330)
				if t1 > 0 {
					blend(i, White(white), t1*t1)
				}
				t2 := clamp01(1 - hypot(x-(PW-20), y-(PH+50))/300)
				L.Glow[k] = a*0.09 + 0.24*t2*t2
				if gl > 0 {
					blend(i, White(1), gl)
				}
				if p.Outline {
					if in < 6 {
						t := 1 - in/6
						blend(i, White(1), 0.10*t*t*(0.45+0.55*vy))
					}
					// a crisp one-pixel rim right at the edge (no soft halo)
					if in < 2*px1 {
						if sc := clamp01(1 - abs32(in-0.5*px1)/px1); sc > 0 {
							blend(i, rim, sc)
						}
					}
					if in < 6 { // specular glints top-left and bottom-right
						band := clamp01(0.5 - (abs32(d+2.6)-0.8)*S)
						if band > 0 && y < 34 && x < 190 {
							blend(i, White(1), band*0.45*(1-abs32(x-95)/95))
						}
						if band > 0 && y > PH-30 && x > PW-170 {
							blend(i, White(1), band*0.24*(1-abs32(x-(PW-85))/85))
						}
					}
				}
				if cov < 1 { // anti-aliased outer edge
					pix[i] *= cov
					pix[i+1] *= cov
					pix[i+2] *= cov
					pix[i+3] *= cov
					L.Glow[k] *= cov
				}
			}
		}
	})
	return L
}

// Composite copies the glass into c, lays the fluid in (if any) and mixes
// in the accent glow.
func (L *BaseLayer) Composite(c *Canvas, ac Col, fl *Fluid, strength float32) {
	src, dst, glow := L.Pix, c.Pix, L.Glow
	w := c.W
	h := c.H
	useFluid := fl != nil && fl.W > 1 && fl.H > 1 && strength > 0
	var fx []int32
	var fw []float32
	if useFluid { // precompute the horizontal sampling once per frame
		fx = make([]int32, w)
		fw = make([]float32, w)
		for x := 0; x < w; x++ {
			v := (float32(x)+0.5)/float32(w)*float32(fl.W) - 0.5
			if v < 0 {
				v = 0
			}
			ix := min(int(v), fl.W-2)
			fx[x], fw[x] = int32(ix), min32(1, v-float32(ix))
		}
	}
	parallelRows(h, func(y0, y1 int) {
		copy(dst[y0*w*4:y1*w*4], src[y0*w*4:y1*w*4])
		for y := y0; y < y1; y++ {
			var r0, r1 []float32
			var ty float32
			if useFluid {
				v := (float32(y)+0.5)/float32(h)*float32(fl.H) - 0.5
				if v < 0 {
					v = 0
				}
				iy := min(int(v), fl.H-2)
				ty = min32(1, v-float32(iy))
				r0 = fl.Pix[iy*fl.W*3:]
				r1 = fl.Pix[(iy+1)*fl.W*3:]
			}
			for x := 0; x < w; x++ {
				k := y*w + x
				i := k * 4
				if useFluid {
					if a := dst[i+3]; a > 0.001 {
						j := int(fx[x]) * 3
						tx := fw[x]
						w00, w10, w01, w11 := (1-tx)*(1-ty), tx*(1-ty), (1-tx)*ty, tx*ty
						fr := r0[j]*w00 + r0[j+3]*w10 + r1[j]*w01 + r1[j+3]*w11
						fg := r0[j+1]*w00 + r0[j+4]*w10 + r1[j+1]*w01 + r1[j+4]*w11
						fb := r0[j+2]*w00 + r0[j+5]*w10 + r1[j+2]*w01 + r1[j+5]*w11
						t := strength
						dst[i] = dst[i]*(1-t) + fr*t*a
						dst[i+1] = dst[i+1]*(1-t) + fg*t*a
						dst[i+2] = dst[i+2]*(1-t) + fb*t*a
					}
				}
				g := glow[k]
				if g <= 0.001 {
					continue
				}
				inv := 1 - g
				dst[i] = ac.R*g + dst[i]*inv
				dst[i+1] = ac.G*g + dst[i+1]*inv
				dst[i+2] = ac.B*g + dst[i+2]*inv
				dst[i+3] = g + dst[i+3]*inv
			}
		}
	})
}

// parallelRows splits rows across CPU cores.
func parallelRows(h int, fn func(y0, y1 int)) {
	n := runtime.NumCPU()
	if n > 8 {
		n = 8
	}
	if n < 2 || h < 16 {
		fn(0, h)
		return
	}
	var wg sync.WaitGroup
	step := (h + n - 1) / n
	for y := 0; y < h; y += step {
		wg.Add(1)
		go func(a, b int) {
			defer wg.Done()
			fn(a, b)
		}(y, min(y+step, h))
	}
	wg.Wait()
}

// BuildBaseFast builds at half resolution and scales up. Used only while the
// window is resizing (motion hides the softness); a full build replaces it
// as soon as the size settles.
func BuildBaseFast(p BaseParams) *BaseLayer {
	q := p
	q.PixW, q.PixH, q.S = (p.PixW+1)/2, (p.PixH+1)/2, p.S/2
	small := BuildBase(q)
	n := p.PixW * p.PixH
	L := &BaseLayer{P: p, Pix: make([]float32, n*4), Glow: make([]float32, n)}
	L.P.Clarity = -1 // never equal to a real key: forces the full rebuild later
	sw, sh := q.PixW, q.PixH
	parallelRows(p.PixH, func(y0, y1 int) {
		for y := y0; y < y1; y++ {
			fy := (float32(y)+0.5)/2 - 0.5
			if fy < 0 {
				fy = 0
			}
			iy := min(int(fy), sh-1)
			iy1 := min(iy+1, sh-1)
			ty := fy - float32(iy)
			for x := 0; x < p.PixW; x++ {
				fx := (float32(x)+0.5)/2 - 0.5
				if fx < 0 {
					fx = 0
				}
				ix := min(int(fx), sw-1)
				ix1 := min(ix+1, sw-1)
				tx := fx - float32(ix)
				k00, k10, k01, k11 := iy*sw+ix, iy*sw+ix1, iy1*sw+ix, iy1*sw+ix1
				w00, w10, w01, w11 := (1-tx)*(1-ty), tx*(1-ty), (1-tx)*ty, tx*ty
				d := y*p.PixW + x
				for ch := 0; ch < 4; ch++ {
					L.Pix[d*4+ch] = small.Pix[k00*4+ch]*w00 + small.Pix[k10*4+ch]*w10 + small.Pix[k01*4+ch]*w01 + small.Pix[k11*4+ch]*w11
				}
				L.Glow[d] = small.Glow[k00]*w00 + small.Glow[k10]*w10 + small.Glow[k01]*w01 + small.Glow[k11]*w11
			}
		}
	})
	return L
}
