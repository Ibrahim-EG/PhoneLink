package main

import (
	"crypto/ecdh"
	"crypto/rand"
	"io"
	"net"
	"testing"
)

func TestHandshake(t *testing.T) {
	srv, _ := ecdh.P256().GenerateKey(rand.Reader)
	cli, _ := ecdh.P256().GenerateKey(rand.Reader)
	a, b := net.Pipe()
	nc := make([]byte, 16)
	ns := make([]byte, 16)
	rand.Read(nc)
	rand.Read(ns)
	sh1, _ := srv.ECDH(cli.PublicKey())
	sh2, _ := cli.ECDH(srv.PublicKey())
	g1 := deriveKey(sh1, nc, ns)
	g2 := deriveKey(sh2, nc, ns)
	go writeFrame(a, g1, []byte("hello"))
	m, err := readFrame(b, g2)
	if err != nil || string(m) != "hello" {
		t.Fatal(err, string(m))
	}
	if sas(cli.PublicKey().Bytes(), srv.PublicKey().Bytes(), nc, ns) == "" {
		t.Fatal("sas")
	}
	// wrong key must fail
	other, _ := ecdh.P256().GenerateKey(rand.Reader)
	sh3, _ := other.ECDH(srv.PublicKey())
	g3 := deriveKey(sh3, nc, ns)
	go writeFrame(a, g1, []byte("x"))
	if _, err := readFrame(b, g3); err == nil {
		t.Fatal("wrong key accepted")
	}
	_ = io.EOF
}
