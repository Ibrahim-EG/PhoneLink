package main

import (
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Local-only page (127.0.0.1) so the PC side has a real window instead of a console.
var (
	uiMu      sync.Mutex
	connected bool
	pending   string
	decision  chan bool
)

func setConnected(v bool) { uiMu.Lock(); connected = v; uiMu.Unlock() }

func waitPairing(code string) bool {
	uiMu.Lock()
	pending = code
	decision = make(chan bool, 1)
	ch := decision
	uiMu.Unlock()
	var ok bool
	select {
	case ok = <-ch:
	case <-time.After(80 * time.Second):
	}
	uiMu.Lock()
	pending = ""
	uiMu.Unlock()
	return ok
}

func page(w http.ResponseWriter) {
	uiMu.Lock()
	c, p := connected, pending
	uiMu.Unlock()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, `<!doctype html><meta charset=utf-8><title>PhoneLink</title><meta http-equiv=refresh content=3>
<body style="font-family:Segoe UI,Arial;max-width:640px;margin:30px auto;padding:0 14px">
<h2>PhoneLink</h2>`)
	if c {
		fmt.Fprint(w, `<p style="color:#0a7d2c;font-size:20px"><b>Phone connected</b></p>`)
	} else {
		fmt.Fprint(w, `<p style="font-size:20px"><b>Waiting for phone</b></p><p>In the phone app, enter one of these addresses:</p><ul>`)
		for _, ip := range localIPs() {
			fmt.Fprintf(w, "<li><b>%s</b></li>", ip)
		}
		fmt.Fprint(w, "</ul>")
	}
	if p != "" {
		fmt.Fprintf(w, `<div style="border:2px solid #c60;padding:12px;margin:12px 0"><p>New phone wants to pair. Code: <b style="font-size:28px">%s</b></p>
<p>Only accept if the phone shows the same code.</p>
<form method=post action=/pair style="display:inline"><button name=a value=yes style="font-size:18px">Accept</button> <button name=a value=no style="font-size:18px">Reject</button></form></div>`, html.EscapeString(p))
	}
	fmt.Fprint(w, `<h3>Send a file to the phone</h3><form method=post action=/send enctype=multipart/form-data><input type=file name=f multiple> <button>Send</button></form>`)
	fmt.Fprintf(w, "<h3>Received files</h3><p>Folder: %s</p><ul>", html.EscapeString(inboxDir()))
	es, _ := os.ReadDir(inboxDir())
	for i := len(es) - 1; i >= 0 && i > len(es)-16; i-- {
		fmt.Fprintf(w, "<li>%s</li>", html.EscapeString(es[i].Name()))
	}
	fmt.Fprint(w, "</ul><p style=color:#666>Clipboard text and notifications sync automatically while connected.</p>")
}

func startUI() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { page(w) })
	mux.HandleFunc("/pair", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		uiMu.Lock()
		ch := decision
		uiMu.Unlock()
		if ch != nil {
			select {
			case ch <- (r.Form.Get("a") == "yes"):
			default:
			}
		}
		http.Redirect(w, r, "/", 303)
	})
	mux.HandleFunc("/send", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 2<<30)
		mr, err := r.MultipartReader()
		if err == nil {
			for {
				p, err := mr.NextPart()
				if err != nil {
					break
				}
				name := filepath.Base(p.FileName())
				if name == "" || name == "." {
					continue
				}
				dst := filepath.Join(outboxDir(), name)
				if f, err := os.Create(dst); err == nil {
					io.Copy(f, p)
					f.Close()
				}
			}
		}
		http.Redirect(w, r, "/", 303)
	})
	// Reject requests from other sites/hosts (DNS rebinding/CSRF): only localhost is served.
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if !(strings.HasPrefix(host, "127.0.0.1:") || strings.HasPrefix(host, "localhost:")) {
			http.Error(w, "forbidden", 403)
			return
		}
		if r.Method == "POST" {
			if o := r.Header.Get("Origin"); o != "" && !strings.HasPrefix(o, "http://127.0.0.1:") && !strings.HasPrefix(o, "http://localhost:") {
				http.Error(w, "forbidden", 403)
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
	go func() {
		time.Sleep(700 * time.Millisecond)
		if runtime.GOOS == "windows" {
			exec.Command("rundll32", "url.dll,FileProtocolHandler", "http://127.0.0.1:47801/").Start()
		}
	}()
	if err := http.ListenAndServe("127.0.0.1:47801", h); err != nil {
		fmt.Println("UI not available:", err)
	}
}
