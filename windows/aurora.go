package main

import (
	"image"
	"image/color"
	"math"
)

// Palette recovered from the user's AuroraScreensaver shader (aurora.frag) and HUD (AuroraHudView.kt):
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
	skyTop    = rgb{10, 14, 46}    // deep blue
	skyBottom = rgb{34, 14, 74}    // blue-violet
	auPurple  = rgb{107, 41, 184}  // shader purple (0.42,0.16,0.72)
	auTeal    = rgb{36, 143, 140}  // shader teal (0.14,0.56,0.55)
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

// Render draws a w x h frame: deep blue to violet gradient, slow drifting purple/teal/sky glows
// (the aurora curtains), a soft glow that follows the pointer, and click ripples.
func (a *AuroraState) Render(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	t := a.T
	type blob struct {
		x, y, rx, ry float64
		c            rgb
		k            float64
	}
	bl := []blob{
		{.25 + .18*math.Sin(t*.21), .42 + .10*math.Sin(t*.33+1), .34, .55, auPurple, .75},
		{.62 + .20*math.Sin(t*.17+2), .35 + .12*math.Sin(t*.27), .38, .50, auTeal, .55},
		{.82 + .12*math.Sin(t*.23+4), .55 + .10*math.Sin(t*.19+2), .30, .50, auSky, .28},
		{.45 + .25*math.Sin(t*.13+5), .70 + .08*math.Sin(t*.31), .30, .40, auRose, .16},
	}
	pointer := blob{a.PX, a.PY, .20, .50, auSky, .30 * a.Active}
	bl = append(bl, pointer)
	for y := 0; y < h; y++ {
		fy := float64(y) / float64(h-1)
		for x := 0; x < w; x++ {
			fx := float64(x) / float64(w-1)
			asp := float64(w) / float64(h)
			r := skyTop.r + (skyBottom.r-skyTop.r)*fy
			g := skyTop.g + (skyBottom.g-skyTop.g)*fy
			b := skyTop.b + (skyBottom.b-skyTop.b)*fy
			// slow curtain shimmer
			curtain := 0.5 + 0.5*math.Sin(fx*9+t*.45+math.Sin(fy*3+t*.2)*2)
			for _, o := range bl {
				dx := (fx - o.x) / o.rx
				dy := (fy - o.y) / o.ry
				d := dx*dx + dy*dy
				if d > 6 {
					continue
				}
				v := o.k * math.Exp(-d) * (0.75 + 0.25*curtain)
				r += o.c.r * v
				g += o.c.g * v
				b += o.c.b * v
			}
			for _, rp := range a.Ripples {
				dx := (fx - rp.X) * asp
				dy := fy - rp.Y
				dist := math.Sqrt(dx*dx+dy*dy) / 0.45
				ring := math.Exp(-math.Pow((dist-rp.Age*.9)*7, 2)) * (1 - rp.Age/1.6)
				r += 150 * ring
				g += 190 * ring
				b += 255 * ring
			}
			// soft vignette
			vg := 1 - 0.25*math.Pow(math.Abs(fx-.5)*2, 3)
			img.SetRGBA(x, y, color.RGBA{clamp8(r * vg), clamp8(g * vg), clamp8(b * vg), 255})
		}
	}
	return img
}
