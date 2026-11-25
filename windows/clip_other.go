//go:build !windows

package main

import (
	"os"
	"path/filepath"
)

// Non-Windows builds exist only for CI tests (no release ships them). The clipboard is two files in
// the home folder so the emulator test can drive it: clip-send.txt is "the PC clipboard" (a change
// is sent to the phone), clip-recv.txt receives what the phone sent.
func clipFile(n string) string { h, _ := os.UserHomeDir(); return filepath.Join(h, n) }

func clipSeq() uint32 {
	st, err := os.Stat(clipFile("clip-send.txt"))
	if err != nil {
		return 0
	}
	return uint32(st.ModTime().UnixNano() / 1e6)
}

func getClip() string {
	b, _ := os.ReadFile(clipFile("clip-send.txt"))
	return string(b)
}

func setClip(s string) { os.WriteFile(clipFile("clip-recv.txt"), []byte(s), 0o600) }
