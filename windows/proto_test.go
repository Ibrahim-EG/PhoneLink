package main

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Full handshake + pairing approval + encrypted file transfer against accept().
func TestPairingAndFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "cfg"))
	srvPriv := loadKey()
	cli, _ := ecdh.P256().GenerateKey(rand.Reader)
	a, b := net.Pipe()
	go accept(b, srvPriv)

	nc := make([]byte, 16)
	rand.Read(nc)
	go a.Write(commit(cli.PublicKey().Bytes(), nc))
	resp := make([]byte, 81)
	if _, err := io.ReadFull(a, resp); err != nil {
		t.Fatal(err)
	}
	go a.Write(append(append([]byte{}, cli.PublicKey().Bytes()...), nc...))
	spk, _ := ecdh.P256().NewPublicKey(resp[:65])
	sh, _ := cli.ECDH(spk)
	g := newSession(deriveKey(sh, nc, resp[65:]), 0)

	// wait for pending pairing, check the code matches, approve
	var code string
	for i := 0; i < 100; i++ {
		uiMu.Lock()
		code = pending
		ch := decision
		uiMu.Unlock()
		if code != "" && ch != nil {
			ch <- true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if code != sas(cli.PublicKey().Bytes(), srvPriv.PublicKey().Bytes(), nc, resp[65:]) {
		t.Fatalf("code mismatch %q", code)
	}
	if err := g.Write(a, []byte(`{"t":"hello"}`)); err != nil {
		t.Fatal(err)
	}
	data := make([]byte, 500000) // spans two chunks worth of size
	rand.Read(data)
	for off := 0; off < len(data); off += 192 * 1024 {
		end := off + 192*1024
		last := end >= len(data)
		if last {
			end = len(data)
		}
		m, _ := json.Marshal(Msg{T: "file", Name: "../evil/test.bin", Data: base64.StdEncoding.EncodeToString(data[off:end]), Last: last})
		if err := g.Write(a, m); err != nil {
			t.Fatal(err)
		}
	}
	var got []byte
	for i := 0; i < 100; i++ {
		got, _ = os.ReadFile(filepath.Join(inboxDir(), "test.bin")) // path traversal stripped
		if len(got) == len(data) {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	if string(got) != string(data) {
		t.Fatalf("file mismatch %d vs %d", len(got), len(data))
	}
	if !isTrusted(cli.PublicKey().Bytes()) {
		t.Fatal("not remembered")
	}
}
