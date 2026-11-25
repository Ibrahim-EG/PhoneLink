package main

import (
	"bytes"
	"crypto/ecdh"
	"encoding/hex"
	"fmt"
	"testing"
)

// Fixed inputs shared with android/app/src/test (ProtoVectorsTest.kt). Both must produce the same outputs.
func vectorValues() (cliPub, srvPub, com []byte, code string, key, f0c, f0s []byte) {
	cp, _ := ecdh.P256().NewPrivateKey(bytes.Repeat([]byte{0x11}, 32))
	sp, _ := ecdh.P256().NewPrivateKey(bytes.Repeat([]byte{0x22}, 32))
	nc := bytes.Repeat([]byte{0x33}, 16)
	ns := bytes.Repeat([]byte{0x44}, 16)
	cliPub, srvPub = cp.PublicKey().Bytes(), sp.PublicKey().Bytes()
	com = commit(cliPub, nc)
	code = sas(cliPub, srvPub, nc, ns)
	sh, _ := cp.ECDH(sp.PublicKey())
	key = deriveKeyBytes(sh, nc, ns)
	cs := newSession(deriveKey(sh, nc, ns), 0)
	ss := newSession(deriveKey(sh, nc, ns), 1)
	var c1, c2 capture
	cs.Write(&c1, []byte("hello"))
	ss.Write(&c2, []byte("world"))
	return cliPub, srvPub, com, code, key, c1.buf, c2.buf
}

func TestVectorsStable(t *testing.T) {
	_, _, c, d, e, f, g := vectorValues()
	if hex.EncodeToString(c) != "9be3710efd77e6bb083d6c4922aac5dbce1e76c20aebde0557ee1006caa1a686" || d != "890468" ||
		hex.EncodeToString(e) != "f5280218fb86ca52aa50593588e4b5ca" ||
		hex.EncodeToString(f) != "000000159578d0bd15a7d0bc814ee4f79ee8f74a3b73f02482" ||
		hex.EncodeToString(g) != "0000001547ba9797777842ddc301ce22c1379d61eb6dab3fa5" {
		t.Fatal("test vectors changed; update Kotlin ProtoVectorsTest too")
	}
}

func printVectors() {
	a, b, c, d, e, f, g := vectorValues()
	fmt.Printf("CLIPUB=%s\nSRVPUB=%s\nCOMMIT=%s\nSAS=%s\nKEY=%s\nFRAME_C0=%s\nFRAME_S0=%s\n",
		hex.EncodeToString(a), hex.EncodeToString(b), hex.EncodeToString(c), d, hex.EncodeToString(e), hex.EncodeToString(f), hex.EncodeToString(g))
}
