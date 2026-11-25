package main

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func runSender(t *testing.T, confirm bool) bool {
	t.Setenv("HOME", t.TempDir())
	cs, ss, a, b := pair()
	mu.Lock()
	cur, sess = a, cs
	mu.Unlock()
	p := filepath.Join(t.TempDir(), "f.bin")
	os.WriteFile(p, []byte("some data"), 0600)
	go func() { // fake phone: read chunks, confirm (or stay silent) after the last one
		for {
			raw, err := ss.Read(b)
			if err != nil {
				return
			}
			var m Msg
			json.Unmarshal(raw, &m)
			if m.Last && confirm {
				ackReceived(m.ID, true)
			}
		}
	}()
	old := ackWait
	ackWait = 300 * time.Millisecond
	defer func() { ackWait = old }()
	return sendFile(p)
}

func TestSendNeedsConfirmation(t *testing.T) {
	if !runSender(t, true) {
		t.Fatal("confirmed transfer reported as failed")
	}
	if runSender(t, false) {
		t.Fatal("unconfirmed transfer reported as success")
	}
}

func TestRetriedTransferNotDuplicated(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	mu.Lock()
	cur = nil // acks go nowhere, only the file handling is checked
	mu.Unlock()
	for i := 0; i < 2; i++ {
		recvFile(Msg{T: "file", Name: "x.txt", ID: "same", Data: base64.StdEncoding.EncodeToString([]byte("1")), Last: true})
	}
	es, _ := os.ReadDir(inboxDir())
	if len(es) != 1 {
		t.Fatalf("expected 1 file, got %d", len(es))
	}
}
