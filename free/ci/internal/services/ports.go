package services

// Purpose: pick free TCP ports on the loopback interface for the host side of
// a service. Compose binds them on 127.0.0.1 only (the generator does).
// Inputs:  how many ports. Outputs: distinct ports, none handed out twice in
// this process while a provision holds it.
// Constraints: a port is free when picked and can still be taken by another
// process before Docker binds it; the caller retries a start that fails on it.

import (
	"fmt"
	"net"
	"sync"
)

var (
	portMu   sync.Mutex
	portHeld = map[int]bool{}
)

// pickPorts returns n free loopback ports and marks them held until released.
func pickPorts(n int) ([]int, error) {
	portMu.Lock()
	defer portMu.Unlock()
	var out []int
	var listeners []net.Listener
	defer func() {
		for _, l := range listeners {
			_ = l.Close()
		}
	}()
	for tries := 0; len(out) < n && tries < n*20; tries++ {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			for _, p := range out {
				delete(portHeld, p)
			}
			return nil, fail(CodeProvision, "no free loopback port: %v", err)
		}
		listeners = append(listeners, l)
		p := l.Addr().(*net.TCPAddr).Port
		if portHeld[p] {
			continue
		}
		portHeld[p] = true
		out = append(out, p)
	}
	if len(out) < n {
		for _, p := range out {
			delete(portHeld, p)
		}
		return nil, fail(CodeProvision, "could not find %d distinct free loopback ports", n)
	}
	return out, nil
}

// releasePorts forgets held ports.
func releasePorts(ports []int) {
	portMu.Lock()
	defer portMu.Unlock()
	for _, p := range ports {
		delete(portHeld, p)
	}
}

// loopback formats a host:port on 127.0.0.1.
func loopback(port int) string { return fmt.Sprintf("127.0.0.1:%d", port) }
