package discovery

// Purpose: an in-memory responder for tests: a Net that never opens a socket, counts every call so tests
//          can prove when the network layer runs, and lets a test publish spoofed records.

import (
	"context"
	"errors"
	"net/netip"
	"sync"
	"time"
)

type memNet struct {
	mu         sync.Mutex
	calls      int // every Net method call
	browses    int
	advertised []Service
	stopped    int
	injected   []Entry // raw records any host on the LAN could publish
	browseErr  error
	local      []netip.Addr
}

func newMemNet() *memNet { return &memNet{local: []netip.Addr{netip.MustParseAddr("192.168.1.20")}} }

func (m *memNet) Browse(_ context.Context, serviceType string, _ time.Duration) ([]Entry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	m.browses++
	if serviceType != ServiceType {
		return nil, errors.New("wrong service type")
	}
	if m.browseErr != nil {
		return nil, m.browseErr
	}
	out := append([]Entry(nil), m.injected...)
	for _, s := range m.advertised {
		out = append(out, Entry{Name: s.Instance + "." + s.Type + "." + s.Domain, Host: s.HostName, Addrs: s.IPs, Port: s.Port, TXT: s.TXT})
	}
	return out, nil
}

func (m *memNet) Advertise(svc Service) (func(), error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	m.advertised = append(m.advertised, svc)
	return func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.stopped++
	}, nil
}

func (m *memNet) LocalAddrs() []netip.Addr {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	return m.local
}

func (m *memNet) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

func (m *memNet) stopCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopped
}

// guardFor is a Guard on an empty CI home with the given environment.
func guardFor(n Net, env map[string]string, home string) *Guard {
	return &Guard{Home: home, Net: n, Getenv: func(k string) string { return env[k] }}
}
