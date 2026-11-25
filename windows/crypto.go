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
)

// Handshake (both sides keep a long-term P-256 key):
//  0. client first sends a 32-byte commitment to its key and nonce (see commit)
//  1. server sends: 65-byte public key | 16-byte nonce; then client reveals its own key | nonce
//  2. shared = ECDH(myPriv, peerPub); key = SHA256("phonelink-v2" | shared | nonceClient | nonceServer)[:16]
//  3. first pairing only: both screens show a 6-digit code derived from both public keys;
//     the user compares them and approves on both devices (blocks a man in the middle).
//  4. each side remembers the other's public key; a different key later is refused.
// Data frames: 4-byte length | 12-byte nonce | AES-128-GCM ciphertext.

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

func deriveKey(shared, nc, ns []byte) cipher.AEAD {
	h := sha256.New()
	h.Write([]byte("phonelink-v2"))
	h.Write(shared)
	h.Write(nc)
	h.Write(ns)
	b, _ := aes.NewCipher(h.Sum(nil)[:16])
	g, _ := cipher.NewGCM(b)
	return g
}

func writeFrame(w io.Writer, g cipher.AEAD, msg []byte) error {
	nonce := make([]byte, 12)
	rand.Read(nonce)
	ct := g.Seal(nil, nonce, msg, nil)
	buf := make([]byte, 4, 4+12+len(ct))
	binary.BigEndian.PutUint32(buf, uint32(12+len(ct)))
	buf = append(buf, nonce...)
	buf = append(buf, ct...)
	_, err := w.Write(buf)
	return err
}

func readFrame(r io.Reader, g cipher.AEAD) ([]byte, error) {
	h := make([]byte, 4)
	if _, err := io.ReadFull(r, h); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(h)
	if n < 28 || n > 1<<20 {
		return nil, errors.New("bad frame")
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(r, b); err != nil {
		return nil, err
	}
	return g.Open(nil, b[:12], b[12:], nil)
}
