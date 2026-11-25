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

// runGUI shows the native window and blocks until the user chooses Exit.
func runGUI(l net.Listener) {
	var mw *walk.MainWindow
	var statusLbl, codeLbl *walk.Label
	var pairBox *walk.GroupBox
	var logBox *walk.TextEdit
	var sendBtn *walk.PushButton

	appIcon, _ := walk.NewIconFromImageForDPI(renderIcon(64), 96)
	quitting := false

	err := MainWindow{
		AssignTo: &mw,
		Title:    "PhoneLink",
		MinSize:  Size{Width: 460, Height: 560},
		Size:     Size{Width: 500, Height: 620},
		Layout:   VBox{Margins: Margins{Left: 16, Top: 14, Right: 16, Bottom: 14}, Spacing: 10},
		Children: []Widget{
			Label{Text: "PhoneLink", Font: Font{Family: "Segoe UI", PointSize: 20, Bold: true}},
			Label{Text: "Connects your phone and this PC directly over your local network.", Font: Font{Family: "Segoe UI", PointSize: 9}},
			GroupBox{
				Title:  "Connection",
				Layout: VBox{Margins: Margins{Left: 10, Top: 8, Right: 10, Bottom: 10}, Spacing: 6},
				Children: []Widget{
					Label{AssignTo: &statusLbl, Text: "Waiting for the phone", Font: Font{Family: "Segoe UI", PointSize: 12, Bold: true}},
					Label{Text: "Type this PC address in the phone app:"},
					LineEdit{Text: addressText(), ReadOnly: true, Font: Font{Family: "Segoe UI", PointSize: 13, Bold: true}},
				},
			},
			GroupBox{
				AssignTo: &pairBox,
				Title:    "Pairing: compare this code with the phone",
				Visible:  false,
				Layout:   VBox{Margins: Margins{Left: 10, Top: 8, Right: 10, Bottom: 10}, Spacing: 6},
				Children: []Widget{
					Label{AssignTo: &codeLbl, Text: "------", Alignment: AlignHCenterVCenter, Font: Font{Family: "Consolas", PointSize: 30, Bold: true}},
					Label{Text: "Only press Accept if the phone shows the same six digits.", Alignment: AlignHCenterVCenter},
					Composite{
						Layout: HBox{MarginsZero: true, Spacing: 8},
						Children: []Widget{
							PushButton{Text: "Accept", OnClicked: func() { answerPairing(true) }},
							PushButton{Text: "Reject", OnClicked: func() { answerPairing(false) }},
						},
					},
				},
			},
			GroupBox{
				Title:  "Files",
				Layout: VBox{Margins: Margins{Left: 10, Top: 8, Right: 10, Bottom: 10}, Spacing: 6},
				Children: []Widget{
					PushButton{
						AssignTo: &sendBtn,
						Text:     "Send a file to the phone...",
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
						Layout: HBox{MarginsZero: true, Spacing: 8},
						Children: []Widget{
							PushButton{Text: "Open received files", OnClicked: func() { openFolder(inboxDir()) }},
							PushButton{Text: "Open send folder", OnClicked: func() { openFolder(outboxDir()) }},
						},
					},
				},
			},
			GroupBox{
				Title:  "Recent activity",
				Layout: VBox{Margins: Margins{Left: 6, Top: 6, Right: 6, Bottom: 6}},
				Children: []Widget{
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
	go func() {
		for {
			time.Sleep(700 * time.Millisecond)
			mw.Synchronize(refresh)
		}
	}()
	mw.Run()
}
