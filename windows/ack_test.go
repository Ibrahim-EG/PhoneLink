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

func TestStableIDAndPersistentDedup(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	p := filepath.Join(t.TempDir(), "a.txt")
	os.WriteFile(p, []byte("hello"), 0600)
	i1, _ := os.Stat(p)
	i2, _ := os.Stat(p)
	if stableID(p, i1) != stableID(p, i2) {
		t.Fatal("id not stable")
	}
	os.WriteFile(p, []byte("hello!"), 0600)
	i3, _ := os.Stat(p)
	if stableID(p, i1) == stableID(p, i3) {
		t.Fatal("changed file kept same id")
	}
	// completed id survives a "restart" (state is on disk, not in memory)
	mu.Lock()
	cur = nil
	mu.Unlock()
	recvFile(Msg{T: "file", Name: "x.txt", ID: "abc", Data: base64.StdEncoding.EncodeToString([]byte("1")), Last: true})
	if !doneHas("abc") {
		t.Fatal("done id not persisted")
	}
	recvFile(Msg{T: "file", Name: "x.txt", ID: "abc", Data: base64.StdEncoding.EncodeToString([]byte("1")), Last: true})
	es, _ := os.ReadDir(inboxDir())
	if len(es) != 1 {
		t.Fatalf("lost-ack retry duplicated: %d files", len(es))
	}
}
