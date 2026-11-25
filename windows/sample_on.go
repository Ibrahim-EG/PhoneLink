//go:build sample

package main

// Test-only build (CI layout screenshots): shows the pairing box with a made-up code.
// It does not talk to a phone and Accept/Reject do nothing real.
func init() {
	uiMu.Lock()
	pending = "123 456 (SAMPLE)"
	uiMu.Unlock()
}
