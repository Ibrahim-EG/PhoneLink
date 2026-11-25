package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// Handshake (both sides keep a long-term P-256 key):
//  0. client first sends a 32-byte commitment to its key and nonce (see commit)
//  1. server sends: 65-byte public key | 16-byte nonce; then client reveals its own key | nonce
//  2. shared = ECDH(myPriv, peerPub); key = SHA256("phonelink-v2" | shared | nonceClient | nonceServer)[:16]
//  3. first pairing only: both screens show a 6-digit code derived from both public keys;
//     the user compares them and approves on both devices (blocks a man in the middle).
//  4. each side remembers the other's public key; a different key later is refused.
// Data frames: see Session (AES-128-GCM, counter nonces, direction-separated).

func loadKey() *ecdh.PrivateKey {
	dir, _ := os.UserConfigDir()
	p := filepath.Join(dir, "phonelink-key.bin")
	if b, err := os.ReadFile(p); err == nil {
		if k, err := ecdh.P256().NewPrivateKey(b); err == nil {
			return k
		}
	}
	k, _ := ecdh.P256().GenerateKey(rand.Reader)
	os.MkdirAll(dir, 0700)
	os.WriteFile(p, k.Bytes(), 0600)
	return k
}

// commit = SHA256(clientPub | clientNonce). The client sends it BEFORE seeing the server's key,
// so a man in the middle cannot grind keys until both screens show the same code.
func commit(pub, nonce []byte) []byte {
	h := sha256.New()
	h.Write([]byte("phonelink-commit"))
	h.Write(pub)
	h.Write(nonce)
	return h.Sum(nil)
}

func sas(clientPub, serverPub, nc, ns []byte) string {
	h := sha256.New()
	h.Write([]byte("phonelink-sas"))
	h.Write(clientPub)
	h.Write(serverPub)
	h.Write(nc)
	h.Write(ns)
	s := h.Sum(nil)
	return fmt.Sprintf("%06d", binary.BigEndian.Uint32(s[:4])%1000000)
}

func deriveKeyBytes(shared, nc, ns []byte) []byte {
	h := sha256.New()
	h.Write([]byte("phonelink-v2"))
	h.Write(shared)
	h.Write(nc)
	h.Write(ns)
	return h.Sum(nil)[:16]
}

func deriveKey(shared, nc, ns []byte) cipher.AEAD {
	b, _ := aes.NewCipher(deriveKeyBytes(shared, nc, ns))
	g, _ := cipher.NewGCM(b)
	return g
}

// Session numbers every frame. The GCM nonce is direction (1 byte) | 3 zero bytes | 8-byte counter,
// built by each side from its own counters and never sent. A replayed, dropped, reordered or
// reflected frame therefore fails authentication.
// Direction: 0 = client (phone) to server (PC), 1 = server to client.
type Session struct {
	g      cipher.AEAD
	dir    byte
	sn, rn uint64
	wmu    sync.Mutex
}

func newSession(g cipher.AEAD, dir byte) *Session { return &Session{g: g, dir: dir} }

func nonceFor(dir byte, n uint64) []byte {
	b := make([]byte, 12)
	b[0] = dir
	binary.BigEndian.PutUint64(b[4:], n)
	return b
}

// frame: 4-byte big-endian length | ciphertext (includes 16-byte GCM tag)
func (s *Session) Write(w io.Writer, msg []byte) error {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	ct := s.g.Seal(nil, nonceFor(s.dir, s.sn), msg, nil)
	s.sn++
	buf := make([]byte, 4, 4+len(ct))
	binary.BigEndian.PutUint32(buf, uint32(len(ct)))
	buf = append(buf, ct...)
	_, err := w.Write(buf)
	return err
}

func (s *Session) Read(r io.Reader) ([]byte, error) {
	h := make([]byte, 4)
	if _, err := io.ReadFull(r, h); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(h)
	if n < 16 || n > 1<<20 {
		return nil, errors.New("bad frame")
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(r, b); err != nil {
		return nil, err
	}
	p, err := s.g.Open(nil, nonceFor(1-s.dir, s.rn), b, nil)
	if err != nil {
		return nil, err
	}
	s.rn++
	return p, nil
}
