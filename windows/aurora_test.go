package main

import "testing"

func TestAuroraMovesAndReacts(t *testing.T) {
	a := NewAurora()
	f1 := a.Render(80, 30)
	a.Step(2)
	f2 := a.Render(80, 30)
	if string(f1.Pix) == string(f2.Pix) {
		t.Fatal("frame did not change over time")
	}
	a.TargetActive, a.TX, a.TY = 1, .9, .5
	for i := 0; i < 30; i++ {
		a.Step(.05)
	}
	f3 := a.Render(80, 30)
	a.Click(.5, .5)
	a.Step(.3)
	f4 := a.Render(80, 30)
	if string(f3.Pix) == string(f4.Pix) {
		t.Fatal("click ripple changed nothing")
	}
	// deep blue/violet, never solid or black
	var sr, sg, sb int
	for i := 0; i < len(f1.Pix); i += 4 {
		sr += int(f1.Pix[i])
		sg += int(f1.Pix[i+1])
		sb += int(f1.Pix[i+2])
	}
	if sb <= sg || sb <= sr || sb/(len(f1.Pix)/4) < 50 {
		t.Fatalf("frame is not blue-violet dominated: r=%d g=%d b=%d", sr, sg, sb)
	}
}
