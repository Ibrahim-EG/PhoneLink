package main

import (
	"image"
	"image/color"
	"math"
)

// Palette recovered from the user's private Aurora repo (src/aurora.frag: green (0.10,1.00,0.42), teal (0.05,0.55,0.75),
// violet (0.45,0.18,0.75), sky (0.008,0.022,0.048)) and the AuroraScreensaver shader/HUD (AuroraHudView.kt):
//
//	night sky   (0.003,0.006,0.013) -> (0.001,0.014,0.024)   near-black blue
//	aurora      green (0.08,0.85,0.42), teal (0.14,0.56,0.55), purple (0.42,0.16,0.72), rose (0.85,0.22,0.36)
//	HUD text    (228,240,255) and (168,220,255), glow (110,200,255)
//
// For a window the near-black sky is lifted to a deep blue that leans purple, as the user asked
// ("deep blue blended with purple, not solid blue"); the moving glows use the shader's purple, teal
// and the HUD's sky blue. Colours below are those values, scaled for screen use.
type rgb struct{ r, g, b float64 }

var (
	skyTop    = rgb{10, 14, 46}   // deep blue
	skyBottom = rgb{34, 14, 74}   // blue-violet
	auPurple  = rgb{115, 46, 191} // Aurora violet (0.45,0.18,0.75)
	auTeal    = rgb{13, 140, 191} // Aurora teal (0.05,0.55,0.75)
	auGreen   = rgb{26, 255, 107} // Aurora green (0.10,1.00,0.42)
	auSkyDark = rgb{2, 6, 12}
	auSky     = rgb{110, 200, 255} // HUD glow
	auRose    = rgb{217, 56, 92}   // shader rose (0.85,0.22,0.36)
)

// Ripple is a click wave that spreads from the pointer and fades.
type Ripple struct{ X, Y, Age float64 }

// AuroraState carries the animation: time, a smoothed pointer position and click ripples.
type AuroraState struct {
	T            float64
	PX, PY       float64 // smoothed pointer, 0..1
	TX, TY       float64 // target pointer
	Active       float64 // 0..1, how much the pointer influences the glow
	TargetActive float64
	Ripples      []Ripple
}

func NewAurora() *AuroraState { return &AuroraState{PX: .5, PY: .5, TX: .5, TY: .5} }

// Step advances the animation by dt seconds.
func (a *AuroraState) Step(dt float64) {
	a.T += dt
	k := 1 - math.Exp(-dt*4)
	a.PX += (a.TX - a.PX) * k
	a.PY += (a.TY - a.PY) * k
	a.Active += (a.TargetActive - a.Active) * k
	live := a.Ripples[:0]
	for _, r := range a.Ripples {
		r.Age += dt
		if r.Age < 1.6 {
			live = append(live, r)
		}
	}
	a.Ripples = live
}

func (a *AuroraState) Click(fx, fy float64) {
	a.Ripples = append(a.Ripples, Ripple{fx, fy, 0})
	if len(a.Ripples) > 6 {
		a.Ripples = a.Ripples[1:]
	}
}

func clamp8(v float64) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

// hash gives a stable pseudo random number per pixel (stars).
func hash2(x, y int) float64 {
	n := uint32(x)*374761393 + uint32(y)*668265263
	n = (n ^ (n >> 13)) * 1274126177
	return float64((n^(n>>16))&0xffff) / 65535
}

// Render draws a w x h frame: deep blue to violet sky, three slow drifting aurora curtains with
// vertical rays (teal and violet mostly, a little green, as in the shader), twinkling stars,
// a gentle lift of the curtains toward the pointer, and click ripples.
func (a *AuroraState) Render(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	t := a.T
	asp := float64(w) / float64(h)
	type layer struct {
		seed, base, width, k float64
		c1, c2               rgb // bottom colour, top colour
	}
	layers := []layer{
		{0.0, 0.50, 0.17, 0.62, auTeal, auPurple},
		{2.3, 0.40, 0.14, 0.50, auPurple, auSky},
		{5.1, 0.60, 0.15, 0.30, auGreen, auTeal},
	}
	for y := 0; y < h; y++ {
		fy := float64(y) / float64(h-1)
		for x := 0; x < w; x++ {
			fx := float64(x) / float64(w-1)
			r := skyTop.r + (skyBottom.r-skyTop.r)*fy
			g := skyTop.g + (skyBottom.g-skyTop.g)*fy
			b := skyTop.b + (skyBottom.b-skyTop.b)*fy
			xw := fx * asp * 1.1
			for _, L := range layers {
				flow := 0.55*math.Sin(xw*1.3+t*.16+L.seed) + 0.30*math.Sin(xw*2.9-t*.11+L.seed*2)
				centre := L.base + 0.11*flow
				// the curtain leans toward the pointer
				px := (fx - a.PX) * 3
				centre += (a.PY - centre) * 0.35 * a.Active * math.Exp(-px*px)
				d := (fy - centre) / L.width
				if d > 0 {
					d *= 1.5 // sharper lower edge, softer top, like real curtains
				}
				band := math.Exp(-d * d)
				if band < 0.01 {
					continue
				}
				rays := 0.62 + 0.38*math.Sin(xw*26+3*flow+t*.5+L.seed)*math.Sin(xw*9-t*.3)
				v := band * rays * L.k
				mix := math.Max(0, math.Min(1, (centre-fy)/0.3+0.5))
				r += (L.c1.r*(1-mix) + L.c2.r*mix) * v
				g += (L.c1.g*(1-mix) + L.c2.g*mix) * v
				b += (L.c1.b*(1-mix) + L.c2.b*mix) * v
			}
			if hs := hash2(x, y); hs > 0.994 {
				tw := 0.5 + 0.5*math.Sin(t*2.5+hs*400)
				v := 150 * tw * (1 - fy*0.6)
				r += v * .85
				g += v * .92
				b += v
			}
			for _, rp := range a.Ripples {
				dx := (fx - rp.X) * asp
				dy := fy - rp.Y
				dist := math.Sqrt(dx*dx+dy*dy) / 0.45
				ring := math.Exp(-math.Pow((dist-rp.Age*.9)*7, 2)) * (1 - rp.Age/1.6)
				r += 90 * ring
				g += 130 * ring
				b += 200 * ring
			}
			vg := 1 - 0.25*math.Pow(math.Abs(fx-.5)*2, 3)
			img.SetRGBA(x, y, color.RGBA{clamp8(r * vg), clamp8(g * vg), clamp8(b * vg), 255})
		}
	}
	return img
}
