package main

import (
	"crypto/ecdh"
	"crypto/rand"
	"io"
	"net"
	"testing"
)

func pair() (*Session, *Session, net.Conn, net.Conn) {
	srv, _ := ecdh.P256().GenerateKey(rand.Reader)
	cli, _ := ecdh.P256().GenerateKey(rand.Reader)
	a, b := net.Pipe()
	nc := make([]byte, 16)
	ns := make([]byte, 16)
	rand.Read(nc)
	rand.Read(ns)
	sh1, _ := srv.ECDH(cli.PublicKey())
	sh2, _ := cli.ECDH(srv.PublicKey())
	return newSession(deriveKey(sh2, nc, ns), 0), newSession(deriveKey(sh1, nc, ns), 1), a, b
}

func TestSessionRoundTripAndWrongKey(t *testing.T) {
	cs, ss, a, b := pair()
	go cs.Write(a, []byte("hello"))
	m, err := ss.Read(b)
	if err != nil || string(m) != "hello" {
		t.Fatal(err, string(m))
	}
	// a session with a different key must not be able to read
	_, other, a2, b2 := pair()
	go cs.Write(a2, []byte("x"))
	if _, err := other.Read(b2); err == nil {
		t.Fatal("wrong key accepted")
	}
}

type capture struct{ buf []byte }

func (c *capture) Write(p []byte) (int, error) { c.buf = append(c.buf, p...); return len(p), nil }

func TestReplayReorderReflectDropRejected(t *testing.T) {
	cs, ss, _, _ := pair()
	var c1, c2, c3 capture
	cs.Write(&c1, []byte("one"))
	cs.Write(&c2, []byte("two"))
	cs.Write(&c3, []byte("three"))
	rd := func(c capture) *net_r { return &net_r{b: c.buf} }
	// in order works
	if m, err := ss.Read(rd(c1)); err != nil || string(m) != "one" {
		t.Fatal("first frame", err)
	}
	// replay of frame 1 must fail
	if _, err := ss.Read(rd(c1)); err == nil {
		t.Fatal("replay accepted")
	}
	// dropping frame 2: frame 3 must fail
	if _, err := ss.Read(rd(c3)); err == nil {
		t.Fatal("gap accepted")
	}
	// frame 2 still OK (failed reads do not advance the counter)
	if m, err := ss.Read(rd(c2)); err != nil || string(m) != "two" {
		t.Fatal("frame 2", err)
	}
	// reflection: a frame the server sent must not be accepted by the server itself
	var s1 capture
	ss.Write(&s1, []byte("echo"))
	if _, err := ss.Read(rd(s1)); err == nil {
		t.Fatal("reflected frame accepted")
	}
}

type net_r struct {
	b []byte
	i int
}

func (r *net_r) Read(p []byte) (int, error) {
	if r.i >= len(r.b) {
		return 0, io.EOF
	}
	n := copy(p, r.b[r.i:])
	r.i += n
	return n, nil
}
