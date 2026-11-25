package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Shared state between the network code and the native window (gui_windows.go).
var (
	uiMu      sync.Mutex
	connected bool
	pending   string
	decision  chan bool
	events    []string
)

func setConnected(v bool) { uiMu.Lock(); connected = v; uiMu.Unlock() }

// logln records a line in the activity list shown in the window (and on stdout when run from a console).
func logln(a ...interface{}) {
	line := strings.TrimSpace(fmt.Sprintln(a...))
	fmt.Println(line)
	uiMu.Lock()
	events = append([]string{time.Now().Format("15:04:05") + "  " + line}, events...)
	if len(events) > 200 {
		events = events[:200]
	}
	uiMu.Unlock()
}

type uiState struct {
	Connected bool
	Pending   string
	Events    []string
}

func snapshot() uiState {
	uiMu.Lock()
	defer uiMu.Unlock()
	return uiState{connected, pending, append([]string(nil), events...)}
}

// answerPairing is called by the window's Accept / Reject buttons.
func answerPairing(ok bool) {
	uiMu.Lock()
	ch := decision
	uiMu.Unlock()
	if ch != nil {
		select {
		case ch <- ok:
		default:
		}
	}
}

func waitPairing(code string) bool {
	uiMu.Lock()
	pending = code
	decision = make(chan bool, 1)
	ch := decision
	uiMu.Unlock()
	var ok bool
	if testAutoAccept {
		time.Sleep(1500 * time.Millisecond)
		ch <- true
	}
	select {
	case ok = <-ch:
	case <-time.After(80 * time.Second):
	}
	uiMu.Lock()
	pending = ""
	uiMu.Unlock()
	return ok
}

// queueForPhone copies a chosen file into the Send folder; the outbox watcher delivers it.
// It is written under a hidden name first so the sender never sees a half-copied file.
func queueForPhone(src string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp, err := os.CreateTemp(outboxDir(), ".upload-*")
	if err != nil {
		return err
	}
	if _, err := tmp.ReadFrom(in); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	tmp.Close()
	name := filepath.Base(src)
	if err := os.Rename(tmp.Name(), filepath.Join(outboxDir(), uniqueName(outboxDir(), name))); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	logln("Queued for phone:", name)
	return nil
}
