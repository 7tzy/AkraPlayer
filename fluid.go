package main

import "math"

// Fluid is the optional "liquid" background: a tiny, slowly flowing color
// field (a few dozen cells across) that is smoothly scaled up under the UI.
// Cheap by design: the field costs well under a millisecond per frame.
type Fluid struct {
	W, H int
	Pix  []float32 // r, g, b per cell (straight color)
}

// FluidPalette returns the colors the fluid flows between.
func FluidPalette(base Col) [4]Col {
	h, s, v := toHSV(base)
	if s < 0.08 { // black & white theme: cool grays with a hint of blue
		return [4]Col{HSV(0.62, 0.18, 0.78), HSV(0.58, 0.10, 0.50), HSV(0.70, 0.20, 0.62), HSV(0.62, 0.20, 0.22)}
	}
	s = max32(s, 0.55)
	v = max32(v, 0.75)
	return [4]Col{
		HSV(h, s, v),
		HSV(h+0.07, s*0.85, v),
		HSV(h-0.09, min32(1, s*1.1), v*0.9),
		HSV(h+0.02, min32(1, s*1.05), v*0.30),
	}
}

func fract(v float32) float32 { return v - float32(math.Floor(float64(v))) }

func sinf(v float32) float32 { return float32(math.Sin(float64(v))) }
func cosf(v float32) float32 { return float32(math.Cos(float64(v))) }

// UpdateFluid fills f for time t. aspect = panel width / height.
func UpdateFluid(f *Fluid, t float32, cols [4]Col, aspect float32) {
	f.W = 40
	f.H = max(8, int(40/aspect+0.5))
	if n := f.W * f.H * 3; cap(f.Pix) < n {
		f.Pix = make([]float32, n)
	} else {
		f.Pix = f.Pix[:n]
	}
	t *= 0.16 // slow, calm motion
	// three soft blobs drifting on Lissajous paths
	type blob struct{ x, y, r float32 }
	bl := [3]blob{
		{0.5 + 0.38*sinf(t*0.9+0.3), 0.5 + 0.34*cosf(t*0.7), 0.42},
		{0.5 + 0.40*cosf(t*0.6+2.1), 0.5 + 0.36*sinf(t*0.8+1.0), 0.36},
		{0.5 + 0.34*sinf(t*0.5+4.0), 0.5 + 0.40*cosf(t*1.1+3.3), 0.30},
	}
	i := 0
	for yy := 0; yy < f.H; yy++ {
		y := (float32(yy) + 0.5) / float32(f.H)
		for xx := 0; xx < f.W; xx++ {
			x := (float32(xx) + 0.5) / float32(f.W)
			// domain warp: this is what makes it look liquid instead of blobby
			wx := x + 0.16*sinf(y*5.1+t*1.3) + 0.08*sinf((x+y)*7.3-t*0.9)
			wy := y + 0.16*cosf(x*4.3-t*1.1) + 0.08*cosf((x-y)*6.1+t*0.7)
			r, g, b := cols[3].R, cols[3].G, cols[3].B
			for k, o := range bl {
				dx, dy := (wx-o.x)*aspect, wy-o.y
				w := float32(math.Exp(float64(-(dx*dx + dy*dy) / (o.r * o.r))))
				c := cols[k]
				r += (c.R - r) * w
				g += (c.G - g) * w
				b += (c.B - b) * w
			}
			// a faint light ribbon running through it
			rib := sinf(wx*6.0+wy*3.0+t*2.0)*0.5 + 0.5
			rib = rib * rib * rib * rib * 0.16
			f.Pix[i] = r + (1-r)*rib
			f.Pix[i+1] = g + (1-g)*rib
			f.Pix[i+2] = b + (1-b)*rib
			i += 3
		}
	}
}
