package tart

// Purpose: prove from inside the VM, before the job starts, that the network scope holds.
// Inputs:  the softnet plan, the gateway, the started proxy (if any) and the netscope facts.
// Outputs: nil when every positive control answered and no forbidden address, port or name did; E679 otherwise.
// Constraints: the checks run over the same ssh session as the job, with the script on stdin (every target is a
//          parsed IP address, a number or a fixed name, never free text); a control that cannot be established
//          fails closed, so a scope is never "proven" by silence: the guest must be macOS (ping flags), the
//          gateway must answer a ping (the ICMP controls work) and, with a proxy, its TCP port must open (the
//          TCP controls work). Closed-port TCP and a blocked address look the same, so the TCP negatives are
//          conclusive only because of that positive control. What stays reachable at the gateway address (host
//          services, the resolver) is exactly what softnet cannot filter by port: the check names it and fails.

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net/netip"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/isolation"
	"github.com/nself-org/plugins/free/ci/internal/netscope"
)

var (
	publicCanary    = netip.MustParseAddr("1.1.1.1")
	publicCanary6   = netip.MustParseAddr("2606:4700:4700::1111")
	dnsCanary       = "example.com"
	gatewayServices = []uint16{22, 53, 80, 111, 443, 445, 548, 631, 3283, 5900, 8080}
)

const maxNegativeTargets = 12

// netChecks are the targets of the in-VM check.
type netChecks struct {
	// TCPOpen must connect (the proxy). TCPShut must not. PingUp must answer (the gateway). PingDown must not.
	TCPOpen, TCPShut []netip.AddrPort
	PingUp, PingDown []netip.Addr
	// DNSDown are names that must not resolve through the VM's resolver.
	DNSDown []string
}

// planChecks derives the checks for a plan.
func planChecks(plan softnetPlan, gateway netip.Addr, proxy *netip.AddrPort, f netscope.Facts) netChecks {
	c := netChecks{PingUp: []netip.Addr{gateway}, PingDown: []netip.Addr{publicCanary}}
	c.TCPShut = append(c.TCPShut, netip.AddrPortFrom(publicCanary, 443), netip.AddrPortFrom(publicCanary, 80), netip.AddrPortFrom(publicCanary6, 443))
	for _, port := range gatewayServices {
		if proxy == nil || proxy.Port() != port {
			c.TCPShut = append(c.TCPShut, netip.AddrPortFrom(gateway, port))
		}
	}
	if plan.Scope != "internet" { // the public internet is reachable through the proxy under internet, resolver included
		c.DNSDown = []string{dnsCanary}
	}
	if proxy != nil {
		c.TCPOpen = append(c.TCPOpen, *proxy)
	}
	seen := map[netip.Addr]bool{gateway: true, publicCanary: true}
	add := func(a netip.Addr) {
		a = netscope.Unmap(a)
		if a.Is4() && !seen[a] && len(c.PingDown) < maxNegativeTargets {
			seen[a] = true
			c.PingDown = append(c.PingDown, a)
			c.TCPShut = append(c.TCPShut, netip.AddrPortFrom(a, 443))
		}
	}
	for _, a := range f.HostAddrs {
		if !a.IsLoopback() {
			add(a)
		}
	}
	for _, p := range f.Exclusion {
		if p.Addr().Is4() {
			a := p.Masked().Addr()
			if p.Bits() < 32 {
				a = a.Next()
			}
			add(a)
		}
	}
	return c
}

const checkHeader = "# nself-ci network check\n"

// script renders the check as a bash script; every output line is `OS x`, `OPEN x`, `SHUT x`, `UP x` or `DOWN x`.
func (c netChecks) script() []byte {
	var b bytes.Buffer
	b.WriteString(checkHeader)
	b.WriteString("echo \"OS $(uname -s)\"\n")
	b.WriteString("bound() { \"$@\" >/dev/null 2>&1 & p=$!; ( sleep 8; kill $p ) >/dev/null 2>&1 & k=$!; wait $p 2>/dev/null; rc=$?; kill $k >/dev/null 2>&1; return $rc; }\n")
	b.WriteString("tcp() { if bound bash -c 'exec 3<>/dev/tcp/$0/$1' \"$1\" \"$2\"; then echo \"OPEN $1:$2\"; else echo \"SHUT $1:$2\"; fi; }\n")
	b.WriteString("png() { if bound ping -c 1 -t 2 \"$1\"; then echo \"UP $1\"; else echo \"DOWN $1\"; fi; }\n")
	b.WriteString("dns() { if bound nslookup -timeout=3 -retry=1 \"$1\"; then echo \"UP dns:$1\"; else echo \"DOWN dns:$1\"; fi; }\n")
	for _, ap := range append(append([]netip.AddrPort(nil), c.TCPOpen...), c.TCPShut...) {
		fmt.Fprintf(&b, "tcp %s %d &\n", ap.Addr(), ap.Port())
	}
	for _, a := range append(append([]netip.Addr(nil), c.PingUp...), c.PingDown...) {
		fmt.Fprintf(&b, "png %s &\n", a)
	}
	for _, n := range c.DNSDown {
		fmt.Fprintf(&b, "dns %s &\n", n)
	}
	b.WriteString("wait\nexit 0\n") // the targets run at once, so a dropped one costs one timeout, not one each
	return b.Bytes()
}

func tcpKey(ap netip.AddrPort) string { return fmt.Sprintf("%s:%d", ap.Addr(), ap.Port()) }

// judge compares the script output with the checks.
func (c netChecks) judge(out []byte) error {
	got := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		if k, v, ok := strings.Cut(strings.TrimSpace(sc.Text()), " "); ok && (k == "OS" || k == "OPEN" || k == "SHUT" || k == "UP" || k == "DOWN") {
			got[v] = k
			if k == "OS" {
				got["OS"] = v
			}
		}
	}
	if got["OS"] != "Darwin" {
		return isolation.ScopeUnenforceable("the network check needs a macOS guest (ping and resolver flags); the VM reports %q", got["OS"])
	}
	for _, ap := range c.TCPOpen {
		if got[tcpKey(ap)] != "OPEN" {
			return isolation.ScopeUnenforceable("the VM cannot reach the egress proxy at %s", ap)
		}
	}
	for _, a := range c.PingUp { // the control that shows ICMP works at all
		if got[a.String()] != "UP" {
			return isolation.ScopeUnenforceable("the network check has no positive control: the VM got no answer from the gateway %s", a)
		}
	}
	for _, ap := range c.TCPShut {
		switch got[tcpKey(ap)] {
		case "SHUT":
		case "OPEN":
			return isolation.ScopeUnenforceable("the VM can connect to %s, which the network scope forbids (softnet cannot filter by port: turn off the host service or use a Mac that offers none)", ap)
		default:
			return isolation.ScopeUnenforceable("the network check did not report on %s", ap)
		}
	}
	for _, a := range c.PingDown {
		switch got[a.String()] {
		case "DOWN":
		case "UP":
			return isolation.ScopeUnenforceable("the VM can reach %s, which the network scope forbids", a)
		default:
			return isolation.ScopeUnenforceable("the network check did not report on %s", a)
		}
	}
	for _, n := range c.DNSDown {
		switch got["dns:"+n] {
		case "DOWN":
		case "UP":
			return isolation.ScopeUnenforceable("the VM resolves public names through the gateway (%s), which would let a job send data out under this scope", n)
		default:
			return isolation.ScopeUnenforceable("the network check did not report on the resolver")
		}
	}
	return nil
}

// verifyNetwork runs the check in the VM.
func (a *attempt) verifyNetwork(ctx context.Context, c netChecks) error {
	res, err := a.tool.Run(ctx, a.sshArgv("/bin/bash -s"), bytes.NewReader(c.script()), nil)
	if err != nil {
		return isolation.ScopeUnenforceable("the network check could not run: %v", err)
	}
	if res.Exit != 0 {
		return isolation.ScopeUnenforceable("the network check failed (exit %d)", res.Exit)
	}
	return c.judge(res.Out)
}
