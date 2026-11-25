package main

import (
	"crypto/ecdh"
	"crypto/hmac"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Msg struct {
	T     string `json:"t"` // clip | notif | call | ping
	Text  string `json:"text,omitempty"`
	App   string `json:"app,omitempty"`
	Title string `json:"title,omitempty"`
	Name  string `json:"name,omitempty"`
	Data  string `json:"data,omitempty"` // base64 chunk
	Last  bool   `json:"last,omitempty"`
	ID    string `json:"id,omitempty"` // transfer id, echoed in the ack
	Ok    bool   `json:"ok,omitempty"`
}

const port = 47800

var (
	mu       sync.Mutex
	cur      net.Conn
	sess     *Session
	lastRecv string
)

func localIPs() []string {
	var out []string
	as, _ := net.InterfaceAddrs()
	for _, a := range as {
		if n, ok := a.(*net.IPNet); ok && !n.IP.IsLoopback() && n.IP.To4() != nil {
			out = append(out, n.IP.String())
		}
	}
	return out
}

func toast(title, body string) {
	esc := func(s string) string {
		s = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "'", "''").Replace(s)
		return s
	}
	ps := `[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
$x = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02)
$t = $x.GetElementsByTagName('text')
$t.Item(0).AppendChild($x.CreateTextNode('` + esc(title) + `')) | Out-Null
$t.Item(1).AppendChild($x.CreateTextNode('` + esc(body) + `')) | Out-Null
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('PhoneLink').Show([Windows.UI.Notifications.ToastNotification]::new($x))`
	exec.Command("powershell", "-NoProfile", "-WindowStyle", "Hidden", "-Command", ps).Run()
}

func send(m Msg) error {
	mu.Lock()
	c, g := cur, sess
	mu.Unlock()
	if c == nil {
		return errors.New("not connected")
	}
	b, _ := json.Marshal(m)
	if err := g.Write(c, b); err != nil {
		c.Close()
		return err
	}
	return nil
}

func clipWatcher() {
	last := clipSeq()
	for {
		time.Sleep(600 * time.Millisecond)
		s := clipSeq()
		if s == last {
			continue
		}
		last = s
		t := getClip()
		mu.Lock()
		same := t == lastRecv
		mu.Unlock()
		if t != "" && !same {
			send(Msg{T: "clip", Text: t})
		}
	}
}

func handle(c net.Conn, g *Session) {
	mu.Lock()
	if cur != nil {
		cur.Close()
	}
	cur, sess = c, g
	mu.Unlock()
	fmt.Println("Phone connected:", c.RemoteAddr())
	setConnected(true)
	for {
		b, err := g.Read(c)
		if err != nil {
			fmt.Println("Phone disconnected.")
			abortReceives()
			mu.Lock()
			if cur == c {
				cur = nil
				setConnected(false)
			}
			mu.Unlock()
			return
		}
		var m Msg
		if json.Unmarshal(b, &m) != nil {
			continue
		}
		switch m.T {
		case "clip":
			mu.Lock()
			lastRecv = m.Text
			mu.Unlock()
			setClip(m.Text)
		case "notif":
			go toast(m.App+": "+m.Title, m.Text)
		case "file":
			recvFile(m)
		case "ack":
			ackReceived(m.ID, m.Ok)
		case "call":
			go toast("Incoming call", m.Text)
		}
	}
}

func trustedPath() string {
	dir, _ := os.UserConfigDir()
	return filepath.Join(dir, "phonelink-trusted.hex")
}

func isTrusted(pub []byte) bool {
	b, _ := os.ReadFile(trustedPath())
	return strings.Contains(string(b), hex.EncodeToString(pub))
}

func trust(pub []byte) {
	f, _ := os.OpenFile(trustedPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if f != nil {
		f.WriteString(hex.EncodeToString(pub) + "\n")
		f.Close()
	}
}

var pairMu sync.Mutex

func accept(c net.Conn, priv *ecdh.PrivateKey) {
	c.SetDeadline(time.Now().Add(90 * time.Second))
	com := make([]byte, 32)
	if _, err := io.ReadFull(c, com); err != nil {
		c.Close()
		return
	}
	ns := make([]byte, 16)
	rand.Read(ns)
	c.Write(append(append([]byte{}, priv.PublicKey().Bytes()...), ns...))
	in := make([]byte, 65+16)
	if _, err := io.ReadFull(c, in); err != nil {
		c.Close()
		return
	}
	peerPub, nc := in[:65], in[65:]
	if !hmac.Equal(commit(peerPub, nc), com) {
		c.Close()
		return
	}
	pk, err := ecdh.P256().NewPublicKey(peerPub)
	if err != nil {
		c.Close()
		return
	}
	shared, err := priv.ECDH(pk)
	if err != nil {
		c.Close()
		return
	}
	g := newSession(deriveKey(shared, nc, ns), 1)
	if !isTrusted(peerPub) {
		pairMu.Lock()
		code := sas(peerPub, priv.PublicKey().Bytes(), nc, ns)
		fmt.Println("\nNEW PHONE wants to pair. Confirmation code:", code)
		fmt.Println("Compare it with the phone, then press Accept in the PhoneLink page that opened in your browser.")
		ok := waitPairing(code)
		pairMu.Unlock()
		if !ok {
			c.Close()
			return
		}
		trust(peerPub)
	}
	b, err := g.Read(c)
	if err != nil || string(b) != `{"t":"hello"}` {
		c.Close()
		return
	}
	c.SetDeadline(time.Time{})
	handle(c, g)
}

func main() {
	priv := loadKey()
	fmt.Println("PhoneLink (Windows)")
	for _, ip := range localIPs() {
		fmt.Printf("PC address: %s:%d\n", ip, port)
	}
	fmt.Println("Enter the PC address in the phone app. Keep this window open.")
	go clipWatcher()
	go startUI()
	go outboxWatcher()
	fmt.Println("Received files go to:", inboxDir())
	fmt.Println("Drop files into this folder to send them to the phone:", outboxDir())
	l, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		fmt.Println("Cannot listen:", err)
		time.Sleep(10 * time.Second)
		return
	}
	for {
		c, err := l.Accept()
		if err != nil {
			continue
		}
		go accept(c, priv)
	}
}
