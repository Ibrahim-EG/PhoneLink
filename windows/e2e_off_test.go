//go:build !e2e

package main

import "testing"

// The release build must never skip the pairing confirmation.
func TestReleaseBuildHasNoAutoAccept(t *testing.T) {
	if testAutoAccept {
		t.Fatal("auto-accept is compiled into a normal build")
	}
}
