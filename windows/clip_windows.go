//go:build windows

package main

import (
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

var (
	u32     = syscall.NewLazyDLL("user32.dll")
	k32     = syscall.NewLazyDLL("kernel32.dll")
	pOpen   = u32.NewProc("OpenClipboard")
	pClose  = u32.NewProc("CloseClipboard")
	pGet    = u32.NewProc("GetClipboardData")
	pSet    = u32.NewProc("SetClipboardData")
	pEmpty  = u32.NewProc("EmptyClipboard")
	pSeq    = u32.NewProc("GetClipboardSequenceNumber")
	pLock   = k32.NewProc("GlobalLock")
	pUnlock = k32.NewProc("GlobalUnlock")
	pAlloc  = k32.NewProc("GlobalAlloc")
)

const cfUnicode = 13

func clipSeq() uint32 { r, _, _ := pSeq.Call(); return uint32(r) }

func openClip() bool {
	for i := 0; i < 10; i++ {
		if r, _, _ := pOpen.Call(0); r != 0 {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

func getClip() string {
	if !openClip() {
		return ""
	}
	defer pClose.Call()
	h, _, _ := pGet.Call(cfUnicode)
	if h == 0 {
		return ""
	}
	p, _, _ := pLock.Call(h)
	if p == 0 {
		return ""
	}
	defer pUnlock.Call(h)
	var u []uint16
	for i := uintptr(0); i < 1<<20; i += 2 {
		c := *(*uint16)(unsafe.Pointer(p + i))
		if c == 0 {
			break
		}
		u = append(u, c)
	}
	return string(utf16.Decode(u))
}

func setClip(s string) {
	u := utf16.Encode([]rune(s))
	u = append(u, 0)
	if !openClip() {
		return
	}
	defer pClose.Call()
	pEmpty.Call()
	h, _, _ := pAlloc.Call(0x0002, uintptr(len(u)*2))
	if h == 0 {
		return
	}
	p, _, _ := pLock.Call(h)
	for i, c := range u {
		*(*uint16)(unsafe.Pointer(p + uintptr(i)*2)) = c
	}
	pUnlock.Call(h)
	pSet.Call(cfUnicode, h)
}
