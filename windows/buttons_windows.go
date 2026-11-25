//go:build windows

package main

import (
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

// allButtons lets the animation loop repaint hover state when the pointer leaves a button.
var allButtons []*walk.CustomWidget

// auroraButton is a flat, owner-painted button in the window's dark violet style
// (the stock Windows button cannot be coloured). It reacts to hover and press.
func auroraButton(text string, primary bool, onClick func()) Widget {
	var w *walk.CustomWidget
	pressed := false
	return CustomWidget{
		AssignTo:            &w,
		MinSize:             Size{Height: 38},
		MaxSize:             Size{Height: 38},
		InvalidatesOnResize: true,
		PaintMode:           PaintNormal,
		Paint: func(c *walk.Canvas, _ walk.Rectangle) error {
			if w != nil {
				found := false
				for _, b := range allButtons {
					if b == w {
						found = true
					}
				}
				if !found {
					allButtons = append(allButtons, w)
				}
			}
			b := w.ClientBoundsPixels()
			_, _, hover := cursorInside(w)
			fill := walk.RGB(30, 27, 78)
			border := walk.RGB(96, 90, 190)
			if primary {
				fill = walk.RGB(52, 70, 170)
				border = walk.RGB(110, 190, 255)
			}
			if hover {
				fill = walk.RGB(byte(clampc(fill_r(fill)+22)), byte(clampc(fill_g(fill)+22)), byte(clampc(fill_b(fill)+34)))
			}
			if pressed {
				fill = walk.RGB(byte(clampc(fill_r(fill)-14)), byte(clampc(fill_g(fill)-14)), byte(clampc(fill_b(fill)-14)))
			}
			br, err := walk.NewSolidColorBrush(fill)
			if err != nil {
				return err
			}
			defer br.Dispose()
			pen, err := walk.NewCosmeticPen(walk.PenSolid, border)
			if err != nil {
				return err
			}
			defer pen.Dispose()
			bg, err := walk.NewSolidColorBrush(cBody)
			if err != nil {
				return err
			}
			defer bg.Dispose()
			if err := c.FillRectanglePixels(bg, b); err != nil {
				return err
			}
			r := walk.Rectangle{X: 1, Y: 1, Width: b.Width - 2, Height: b.Height - 2}
			if err := c.FillRoundedRectanglePixels(br, r, walk.Size{Width: 12, Height: 12}); err != nil {
				return err
			}
			if err := c.DrawRoundedRectanglePixels(pen, r, walk.Size{Width: 12, Height: 12}); err != nil {
				return err
			}
			font, _ := walk.NewFont("Segoe UI", 10, walk.FontBold)
			defer font.Dispose()
			return c.DrawTextPixels(text, font, cText, b, walk.TextCenter|walk.TextVCenter|walk.TextSingleLine)
		},
		OnMouseDown: func(x, y int, _ walk.MouseButton) { pressed = true; w.Invalidate() },
		OnMouseUp: func(x, y int, _ walk.MouseButton) {
			was := pressed
			pressed = false
			w.Invalidate()
			if _, _, in := cursorInside(w); was && in {
				onClick()
			}
		},
		OnMouseMove: func(x, y int, _ walk.MouseButton) { w.Invalidate() },
	}
}

func fill_r(c walk.Color) int { return clampc(int(c & 0xff)) }
func fill_g(c walk.Color) int { return clampc(int((c >> 8) & 0xff)) }
func fill_b(c walk.Color) int { return clampc(int((c >> 16) & 0xff)) }
func clampc(v int) int {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}
