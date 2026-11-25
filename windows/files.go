package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

func baseDir() string {
	h, _ := os.UserHomeDir()
	return filepath.Join(h, "Downloads", "PhoneLink")
}
func inboxDir() string  { d := filepath.Join(baseDir(), "Received"); os.MkdirAll(d, 0700); return d }
func outboxDir() string { d := filepath.Join(baseDir(), "Send"); os.MkdirAll(d, 0700); return d }

var (
	rmu sync.Mutex
	rcv = map[string]*os.File{}
)

func recvFile(m Msg) {
	name := filepath.Base(m.Name) // never trust paths from the phone
	if name == "." || name == string(filepath.Separator) || name == "" {
		return
	}
	rmu.Lock()
	defer rmu.Unlock()
	f := rcv[name]
	if f == nil {
		var err error
		f, err = os.Create(filepath.Join(inboxDir(), name))
		if err != nil {
			return
		}
		rcv[name] = f
	}
	if b, err := base64.StdEncoding.DecodeString(m.Data); err == nil {
		f.Write(b)
	}
	if m.Last {
		f.Close()
		delete(rcv, name)
		fmt.Println("Received file:", name)
		go toast("File received", name)
	}
}

func sendFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	name := filepath.Base(path)
	buf := make([]byte, 192*1024)
	for {
		n, err := io.ReadFull(f, buf)
		last := err != nil
		send(Msg{T: "file", Name: name, Data: base64.StdEncoding.EncodeToString(buf[:n]), Last: last})
		if last {
			break
		}
	}
	fmt.Println("Sent file:", name)
}

// Files dropped in the Send folder go to the phone, then move to Send\sent.
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
			if e.IsDir() {
				continue
			}
			p := filepath.Join(outboxDir(), e.Name())
			sendFile(p)
			d := filepath.Join(outboxDir(), "sent")
			os.MkdirAll(d, 0700)
			os.Rename(p, filepath.Join(d, e.Name()))
		}
	}
}
