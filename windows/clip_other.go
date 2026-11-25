//go:build !windows

package main

func clipSeq() uint32  { return 0 }
func getClip() string  { return "" }
func setClip(s string) {}
