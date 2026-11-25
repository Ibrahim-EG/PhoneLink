package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

func baseDir() string {
	h, _ := os.UserHomeDir()
	return filepath.Join(h, "Downloads", "PhoneLink")
}
func inboxDir() string  { d := filepath.Join(baseDir(), "Received"); os.MkdirAll(d, 0700); return d }
func outboxDir() string { d := filepath.Join(baseDir(), "Send"); os.MkdirAll(d, 0700); return d }

type partial struct {
	f    *os.File
	path string
}

var (
	rmu sync.Mutex
	rcv = map[string]*partial{}

	amu  sync.Mutex
	acks = map[string]chan bool{}
)

// Completed transfer ids are kept on disk so a retry after a lost confirmation (even after
// the app restarted) is acknowledged again instead of saved a second time.
func donePath() string { os.MkdirAll(baseDir(), 0700); return filepath.Join(baseDir(), ".done-ids") }

// Only recent ids count, so deliberately sending the same unchanged file again later still works.
const dedupWindow = 10 * time.Minute

func doneHas(id string) bool {
	b, _ := os.ReadFile(donePath())
	f := strings.Fields(string(b))
	for i := 0; i+1 < len(f); i += 2 {
		var ts int64
		fmt.Sscan(f[i+1], &ts)
		if f[i] == id && time.Since(time.Unix(ts, 0)) < dedupWindow {
			return true
		}
	}
	return false
}
func doneAdd(id string) {
	if f, err := os.OpenFile(donePath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600); err == nil {
		f.WriteString(fmt.Sprintf("%s %d\n", id, time.Now().Unix()))
		f.Close()
	}
}

// Same file (name, size, modified time) always gets the same id, so every retry is recognisable.
func stableID(path string, info os.FileInfo) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%d", filepath.Base(path), info.Size(), info.ModTime().UnixNano())))
	return hex.EncodeToString(h[:8])
}

func ackReceived(id string, ok bool) {
	amu.Lock()
	ch := acks[id]
	amu.Unlock()
	if ch != nil {
		select {
		case ch <- ok:
		default:
		}
	}
}

// Files arrive into a hidden ".part" file and only get their real name when the last chunk
// has been written. A dropped connection deletes unfinished parts, so no half file is ever
// left looking like a finished one.
func recvFile(m Msg) {
	name := filepath.Base(m.Name) // never trust paths from the phone
	if name == "." || name == string(filepath.Separator) || name == "" || strings.HasSuffix(name, ".part") {
		return
	}
	rmu.Lock()
	defer rmu.Unlock()
	if m.ID != "" && doneHas(m.ID) {
		if m.Last {
			go send(Msg{T: "ack", ID: m.ID, Name: name, Ok: true})
		}
		return
	}
	p := rcv[name]
	if p == nil {
		f, err := os.CreateTemp(inboxDir(), ".incoming-*.part")
		if err != nil {
			return
		}
		p = &partial{f: f, path: f.Name()}
		rcv[name] = p
	}
	b, err := base64.StdEncoding.DecodeString(m.Data)
	if err != nil {
		p.f.Close()
		os.Remove(p.path)
		delete(rcv, name)
		return
	}
	if _, err := p.f.Write(b); err != nil {
		p.f.Close()
		os.Remove(p.path)
		delete(rcv, name)
		return
	}
	if m.Last {
		p.f.Close()
		delete(rcv, name)
		final := uniqueName(inboxDir(), name)
		if os.Rename(p.path, filepath.Join(inboxDir(), final)) == nil {
			fmt.Println("Received file:", final)
			go toast("File received", final)
			if m.ID != "" {
				doneAdd(m.ID)
			}
			go send(Msg{T: "ack", ID: m.ID, Name: name, Ok: true})
		} else {
			os.Remove(p.path)
			go send(Msg{T: "ack", ID: m.ID, Name: name, Ok: false})
		}
	}
}

func abortReceives() {
	rmu.Lock()
	defer rmu.Unlock()
	for k, p := range rcv {
		p.f.Close()
		os.Remove(p.path)
		delete(rcv, k)
	}
}

// never overwrite an existing file: photo.jpg -> photo (1).jpg
func uniqueName(dir, name string) string {
	if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
		return name
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for i := 1; ; i++ {
		n := fmt.Sprintf("%s (%d)%s", base, i, ext)
		if _, err := os.Stat(filepath.Join(dir, n)); err != nil {
			return n
		}
	}
}

// returns true only if every chunk was written to the connection
var ackWait = 30 * time.Second

func sendFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	name := filepath.Base(path)
	st, err := f.Stat()
	if err != nil {
		return false
	}
	id := stableID(path, st)
	ch := make(chan bool, 1)
	amu.Lock()
	acks[id] = ch
	amu.Unlock()
	defer func() { amu.Lock(); delete(acks, id); amu.Unlock() }()
	buf := make([]byte, 192*1024)
	for {
		n, err := io.ReadFull(f, buf)
		last := err != nil
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			return false
		}
		if e := send(Msg{T: "file", Name: name, ID: id, Data: base64.StdEncoding.EncodeToString(buf[:n]), Last: last}); e != nil {
			fmt.Println("Send interrupted, will retry when the phone reconnects:", name)
			return false
		}
		if last {
			break
		}
	}
	// Written to the connection is not the same as delivered: wait for the receiver's confirmation.
	select {
	case ok := <-ch:
		if ok {
			fmt.Println("Delivered (confirmed by phone):", name)
		} else {
			fmt.Println("Phone could not save:", name)
		}
		return ok
	case <-time.After(ackWait):
		fmt.Println("Sent but NOT confirmed by the phone, kept in Send folder:", name)
		return false
	}
}

const maxAttempts = 3

var (
	tries   = map[string]int{}       // transfer id -> attempts so far
	nextTry = map[string]time.Time{} // transfer id -> earliest next attempt
)

// Files in the Send folder go to the phone, then move to Send\sent once the phone confirmed
// them. A failed or unconfirmed file is retried with the same transfer id (the phone ignores
// a duplicate), at most maxAttempts times with a growing pause. After that it is moved to
// Send\failed and a notification says so: it is never retried automatically again.
func outboxWatcher() {
	for {
		time.Sleep(2 * time.Second)
		mu.Lock()
		connected := cur != nil
		mu.Unlock()
		if !connected {
			continue
		}
		es, _ := os.ReadDir(outboxDir())
		for _, e := range es {
			if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			info, err := e.Info()
			if err != nil || time.Since(info.ModTime()) < 3*time.Second {
				continue // possibly still being copied into the folder
			}
			p := filepath.Join(outboxDir(), e.Name())
			id := stableID(p, info)
			if time.Now().Before(nextTry[id]) {
				continue
			}
			if sendFile(p) {
				d := filepath.Join(outboxDir(), "sent")
				os.MkdirAll(d, 0700)
				os.Rename(p, filepath.Join(d, uniqueName(d, e.Name())))
				delete(tries, id)
				continue
			}
			tries[id]++
			if tries[id] >= maxAttempts {
				d := filepath.Join(outboxDir(), "failed")
				os.MkdirAll(d, 0700)
				os.Rename(p, filepath.Join(d, uniqueName(d, e.Name())))
				msg := fmt.Sprintf("%s was not confirmed by the phone after %d tries. It may or may not have arrived. Moved to Send\\failed.", e.Name(), maxAttempts)
				fmt.Println(msg)
				go toast("File not delivered", msg)
				delete(tries, id)
			} else {
				nextTry[id] = time.Now().Add(time.Duration(tries[id]) * 20 * time.Second)
			}
			break
		}
	}
}
