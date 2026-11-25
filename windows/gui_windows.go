//go:build windows

package main

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"github.com/lxn/win"
)

func addressText() string {
	ips := localIPs()
	if len(ips) == 0 {
		return "No network found"
	}
	var parts []string
	for _, ip := range ips {
		parts = append(parts, ip)
	}
	return strings.Join(parts, "   or   ")
}

func openFolder(dir string) { exec.Command("explorer", dir).Start() }

var (
	cText  = walk.RGB(228, 240, 255) // HUD text colour from the Aurora project
	cMuted = walk.RGB(168, 220, 255) // HUD secondary text colour
	cBody  = walk.RGB(19, 17, 54)    // deep blue-violet window body
)

// runGUI shows the native window and blocks until the user chooses Exit.
func runGUI(l net.Listener) {
	var mw *walk.MainWindow
	var statusLbl, codeLbl *walk.Label
	var pairBox *walk.Composite
	var logBox *walk.TextEdit
	var header *walk.CustomWidget
	au := NewAurora()
	var hdrW, hdrH int

	appIcon, _ := walk.NewIconFromImageForDPI(renderIcon(64), 96)
	quitting := false
	bodyBrush := SolidColorBrush{Color: cBody}
	f := func(pt int, bold bool) Font { return Font{Family: "Segoe UI", PointSize: pt, Bold: bold} }

	err := MainWindow{
		AssignTo:   &mw,
		Title:      "PhoneLink",
		MinSize:    Size{Width: 460, Height: 600},
		Size:       Size{Width: 500, Height: 660},
		Background: bodyBrush,
		Layout:     VBox{MarginsZero: true, Spacing: 0},
		Children: []Widget{
			CustomWidget{
				AssignTo:            &header,
				MinSize:             Size{Height: 150},
				MaxSize:             Size{Height: 150},
				ClearsBackground:    false,
				InvalidatesOnResize: true,
				PaintMode:           PaintNormal,
				Paint: func(canvas *walk.Canvas, _ walk.Rectangle) error {
					b := header.ClientBoundsPixels()
					hdrW, hdrH = b.Width, b.Height
					img := au.Render(b.Width/3+1, b.Height/3+1)
					bmp, err := walk.NewBitmapFromImageForDPI(img, 96)
					if err != nil {
						return err
					}
					defer bmp.Dispose()
					if err := canvas.DrawImageStretchedPixels(bmp, b); err != nil {
						return err
					}
					title, _ := walk.NewFont("Segoe UI", 22, walk.FontBold)
					sub, _ := walk.NewFont("Segoe UI", 10, 0)
					defer title.Dispose()
					defer sub.Dispose()
					canvas.DrawTextPixels("PhoneLink", title, cText, walk.Rectangle{X: 24, Y: 34, Width: b.Width - 48, Height: 40}, walk.TextLeft|walk.TextSingleLine)
					canvas.DrawTextPixels("Your phone and this PC, directly over your local network", sub, cMuted, walk.Rectangle{X: 26, Y: 80, Width: b.Width - 52, Height: 24}, walk.TextLeft|walk.TextSingleLine)
					return nil
				},
				OnMouseMove: func(x, y int, _ walk.MouseButton) {
					if hdrW > 0 {
						au.TX, au.TY, au.TargetActive = float64(x)/float64(hdrW), float64(y)/float64(hdrH), 1
					}
				},
				OnMouseDown: func(x, y int, _ walk.MouseButton) {
					if hdrW > 0 {
						au.Click(float64(x)/float64(hdrW), float64(y)/float64(hdrH))
					}
				},
			},
			Composite{
				Background: bodyBrush,
				Layout:     VBox{Margins: Margins{Left: 20, Top: 14, Right: 20, Bottom: 14}, Spacing: 8},
				Children: []Widget{
					Label{AssignTo: &statusLbl, Text: "Waiting for the phone", TextColor: cText, Font: f(13, true)},
					Label{Text: "Type this PC address in the phone app:", TextColor: cMuted},
					LineEdit{Text: addressText(), ReadOnly: true, Font: f(14, true)},
					Composite{
						AssignTo:   &pairBox,
						Visible:    false,
						Background: bodyBrush,
						Layout:     VBox{MarginsZero: true, Spacing: 6},
						Children: []Widget{
							Label{Text: "Pairing: compare this code with the phone", TextColor: cMuted},
							Label{AssignTo: &codeLbl, Text: "------", TextColor: cText, Alignment: AlignHCenterVCenter, Font: Font{Family: "Consolas", PointSize: 32, Bold: true}},
							Label{Text: "Only press Accept if the phone shows the same six digits.", TextColor: cMuted, Alignment: AlignHCenterVCenter},
							Composite{
								Background: bodyBrush,
								Layout:     HBox{MarginsZero: true, Spacing: 8},
								Children: []Widget{
									PushButton{Text: "Accept", OnClicked: func() { answerPairing(true) }},
									PushButton{Text: "Reject", OnClicked: func() { answerPairing(false) }},
								},
							},
						},
					},
					VSpacer{Size: 4},
					PushButton{
						Text: "Send a file to the phone...",
						OnClicked: func() {
							dlg := new(walk.FileDialog)
							dlg.Title = "Choose a file to send to the phone"
							if ok, _ := dlg.ShowOpen(mw); ok && dlg.FilePath != "" {
								if err := queueForPhone(dlg.FilePath); err != nil {
									walk.MsgBox(mw, "PhoneLink", fmt.Sprint("Could not queue the file: ", err), walk.MsgBoxIconError)
								}
							}
						},
					},
					Composite{
						Background: bodyBrush,
						Layout:     HBox{MarginsZero: true, Spacing: 8},
						Children: []Widget{
							PushButton{Text: "Open received files", OnClicked: func() { openFolder(inboxDir()) }},
							PushButton{Text: "Open send folder", OnClicked: func() { openFolder(outboxDir()) }},
						},
					},
					Label{Text: "Recent activity", TextColor: cMuted},
					TextEdit{AssignTo: &logBox, ReadOnly: true, VScroll: true, MinSize: Size{Height: 120}},
				},
			},
		},
	}.Create()
	if err != nil {
		return
	}
	if appIcon != nil {
		mw.SetIcon(appIcon)
	}

	// Tray icon: closing the window keeps the app running so sync continues.
	ni, err := walk.NewNotifyIcon(mw)
	if err == nil {
		defer ni.Dispose()
		if appIcon != nil {
			ni.SetIcon(appIcon)
		}
		ni.SetToolTip("PhoneLink")
		show := func() { mw.Show(); mw.SetVisible(true) }
		ni.MouseDown().Attach(func(x, y int, b walk.MouseButton) {
			if b == walk.LeftButton {
				show()
			}
		})
		open := walk.NewAction()
		open.SetText("Open PhoneLink")
		open.Triggered().Attach(show)
		exit := walk.NewAction()
		exit.SetText("Exit")
		exit.Triggered().Attach(func() { quitting = true; walk.App().Exit(0) })
		ni.ContextMenu().Actions().Add(open)
		ni.ContextMenu().Actions().Add(exit)
		ni.SetVisible(true)
	}
	mw.Closing().Attach(func(canceled *bool, reason walk.CloseReason) {
		if !quitting && ni != nil {
			*canceled = true
			mw.Hide()
			ni.ShowInfo("PhoneLink is still running", "Right-click the tray icon to exit.")
		}
	})

	refresh := func() {
		st := snapshot()
		if st.Connected {
			statusLbl.SetText("Phone connected")
		} else {
			statusLbl.SetText("Waiting for the phone")
		}
		if st.Pending != "" {
			codeLbl.SetText(st.Pending)
			if !pairBox.Visible() {
				pairBox.SetVisible(true)
				mw.Show()
			}
		} else if pairBox.Visible() {
			pairBox.SetVisible(false)
		}
		logBox.SetText(strings.Join(st.Events, "\r\n"))
	}
	refresh()
	go func() { // slow refresh of state
		for {
			time.Sleep(700 * time.Millisecond)
			mw.Synchronize(refresh)
		}
	}()
	go func() { // animation, about 20 frames a second, only while the window is visible
		last := time.Now()
		for {
			time.Sleep(50 * time.Millisecond)
			now := time.Now()
			dt := now.Sub(last).Seconds()
			last = now
			mw.Synchronize(func() {
				if !mw.Visible() {
					return
				}
				au.Step(dt)
				// pointer influence fades when the mouse leaves the header
				if x, y, ok := cursorInside(header); !ok {
					au.TargetActive = 0
				} else {
					_ = x
					_ = y
				}
				header.Invalidate()
			})
		}
	}()
	mw.Run()
}

// cursorInside reports whether the mouse pointer is over the widget.
func cursorInside(w walk.Widget) (int, int, bool) {
	var p win.POINT
	if !win.GetCursorPos(&p) {
		return 0, 0, false
	}
	if !win.ScreenToClient(w.Handle(), &p) {
		return 0, 0, false
	}
	b := w.ClientBoundsPixels()
	x, y := int(p.X), int(p.Y)
	return x, y, x >= 0 && y >= 0 && x < b.Width && y < b.Height
}
