//go:build e2e

package main

// Test-only build (CI end-to-end test): accepts the pairing request without a click.
// The test itself compares the code shown on the phone with the code the PC logged.
const testAutoAccept = true
