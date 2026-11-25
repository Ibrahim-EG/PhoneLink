package main

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestPartialFileDiscardedOnDisconnect(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	recvFile(Msg{T: "file", Name: "big.bin", Data: base64.StdEncoding.EncodeToString([]byte("half")), Last: false})
	abortReceives()
	es, _ := os.ReadDir(inboxDir())
	if len(es) != 0 {
		t.Fatalf("leftover files: %d", len(es))
	}
	// complete file never overwrites an existing one
	for i := 0; i < 2; i++ {
		recvFile(Msg{T: "file", Name: "a.txt", Data: base64.StdEncoding.EncodeToString([]byte("x")), Last: true})
	}
	if _, err := os.Stat(filepath.Join(inboxDir(), "a (1).txt")); err != nil {
		t.Fatal("second copy not renamed")
	}
}
