package main

// Purpose: a TCP proxy that can drop every connection and refuse new ones, to partition the agent from the
//          coordinator on loopback (the same helper as in internal/transport/agent).
// Inputs:  the coordinator address. Outputs: a listener address for the agent.
// Constraints: partition closes live connections at once; heal accepts again; closed by t.Cleanup.

import (
	"io"
	"net"
	"sync"
	"testing"
)

type netProxy struct {
	ln     net.Listener
	target string
	mu     sync.Mutex
	conns  map[net.Conn]struct{}
	down   bool
	wg     sync.WaitGroup
}

func newNetProxy(t *testing.T, target string) *netProxy {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p := &netProxy{ln: ln, target: target, conns: map[net.Conn]struct{}{}}
	p.wg.Add(1)
	go p.accept()
	t.Cleanup(func() { _ = ln.Close(); p.cut(); p.wg.Wait() })
	return p
}

func (p *netProxy) addr() string { return p.ln.Addr().String() }

func (p *netProxy) accept() {
	defer p.wg.Done()
	for {
		c, err := p.ln.Accept()
		if err != nil {
			return
		}
		p.mu.Lock()
		down := p.down
		p.mu.Unlock()
		up, derr := net.Dial("tcp", p.target)
		if down || derr != nil {
			_ = c.Close()
			if up != nil {
				_ = up.Close()
			}
			continue
		}
		p.mu.Lock()
		p.conns[c], p.conns[up] = struct{}{}, struct{}{}
		p.mu.Unlock()
		p.wg.Add(2)
		go p.pipe(c, up)
		go p.pipe(up, c)
	}
}

func (p *netProxy) pipe(dst, src net.Conn) {
	defer p.wg.Done()
	_, _ = io.Copy(dst, src)
	_ = dst.Close()
	_ = src.Close()
	p.mu.Lock()
	delete(p.conns, dst)
	delete(p.conns, src)
	p.mu.Unlock()
}

func (p *netProxy) partition() {
	p.mu.Lock()
	p.down = true
	p.mu.Unlock()
	p.cut()
}

func (p *netProxy) heal() {
	p.mu.Lock()
	p.down = false
	p.mu.Unlock()
}

func (p *netProxy) cut() {
	p.mu.Lock()
	cs := make([]net.Conn, 0, len(p.conns))
	for c := range p.conns {
		cs = append(cs, c)
	}
	p.mu.Unlock()
	for _, c := range cs {
		_ = c.Close()
	}
}
