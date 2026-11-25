//go:build !windows

package main

import "net"

// No native window off Windows (used by tests and CI only).
func runGUI(l net.Listener) { select {} }
