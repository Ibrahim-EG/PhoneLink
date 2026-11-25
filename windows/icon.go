package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
)

// renderIcon draws the app icon (phone, link dots, monitor on a blue rounded square) at size x size.
func renderIcon(size int) *image.RGBA {
	const ss = 4 // supersampling
	n := size * ss
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	s := float64(n)
	inRR := func(x, y, x0, y0, x1, y1, r float64) bool {
		if x < x0 || x > x1 || y < y0 || y > y1 {
			return false
		}
		cx := math.Max(x0+r, math.Min(x, x1-r))
		cy := math.Max(y0+r, math.Min(y, y1-r))
		return (x-cx)*(x-cx)+(y-cy)*(y-cy) <= r*r
	}
	white := color.RGBA{255, 255, 255, 255}
	screen := color.RGBA{40, 96, 220, 255}
	for py := 0; py < size; py++ {
		for px := 0; px < size; px++ {
			var r, g, b, a float64
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					x := float64(px*ss+sx) + .5
					y := float64(py*ss+sy) + .5
					fx, fy := x/s, y/s
					var c color.RGBA
					ok := false
					if inRR(fx, fy, 0, 0, 1, 1, .22) {
						t := fy
						c = color.RGBA{uint8(30 + 5*t), uint8(110 - 40*t), uint8(235 - 30*t), 255}
						ok = true
						switch {
						case inRR(fx, fy, .19, .35, .35, .64, .015):
							c = screen
						case inRR(fx, fy, .16, .30, .38, .72, .04):
							c = white
						case inRR(fx, fy, .53, .29, .83, .52, .012):
							c = screen
						case inRR(fx, fy, .50, .26, .86, .55, .03):
							c = white
						case fx > .65 && fx < .71 && fy > .55 && fy < .63:
							c = white
						case inRR(fx, fy, .58, .62, .78, .66, .02):
							c = white
						default:
							for _, dx := range []float64{.42, .45, .48} {
								if (fx-dx)*(fx-dx)+(fy-.47)*(fy-.47) < .012*.012 {
									c = white
								}
							}
						}
					}
					if ok {
						r += float64(c.R)
						g += float64(c.G)
						b += float64(c.B)
						a += 255
					}
				}
			}
			cnt := float64(ss * ss)
			if a > 0 {
				k := a / 255
				img.SetRGBA(px, py, color.RGBA{uint8(r / k), uint8(g / k), uint8(b / k), uint8(a / cnt)})
			}
		}
	}
	return img
}

// writeICO writes a multi-size .ico (PNG-compressed entries) used as the exe icon at build time.
func writeICO(sizes []int) []byte {
	var pngs [][]byte
	for _, sz := range sizes {
		var b bytes.Buffer
		png.Encode(&b, renderIcon(sz))
		pngs = append(pngs, b.Bytes())
	}
	var out bytes.Buffer
	le := func(v int, n int) {
		for i := 0; i < n; i++ {
			out.WriteByte(byte(v >> (8 * i)))
		}
	}
	le(0, 2)
	le(1, 2)
	le(len(sizes), 2)
	off := 6 + 16*len(sizes)
	for i, sz := range sizes {
		d := sz
		if d >= 256 {
			d = 0
		}
		out.WriteByte(byte(d))
		out.WriteByte(byte(d))
		out.WriteByte(0)
		out.WriteByte(0)
		le(1, 2)
		le(32, 2)
		le(len(pngs[i]), 4)
		le(off, 4)
		off += len(pngs[i])
	}
	for _, p := range pngs {
		out.Write(p)
	}
	return out.Bytes()
}
