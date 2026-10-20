package tart

// Purpose: prove from inside the VM, before the job starts, that the network scope holds.
// Inputs:  the softnet plan, the gateway, the started proxy (if any) and the netscope facts.
// Outputs: nil when every positive control answered and no forbidden address did; E679 otherwise.
// Constraints: the checks run over the same ssh session as the job, with the script on stdin (all targets are
//          parsed IP addresses and numbers, never free text); a control that cannot be established fails
//          closed, so a scope is never "proven" by silence. Negative targets are ICMP (a closed TCP port looks
//          the same as a blocked one; an address that answers a ping is reachable).

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

// publicCanary answers pings on the public internet; under every scope below lan the VM must not reach it directly.
var publicCanary = netip.MustParseAddr("1.1.1.1")

const maxNegativeTargets = 12

// netChecks are the targets of the in-VM check.
type netChecks struct {
	// TCPOpen must connect (the proxy). PingUp must answer (the gateway). PingDown must not answer.
	TCPOpen []netip.AddrPort
	PingUp  []netip.Addr
	// PingDown are addresses that must stay unreachable.
	PingDown []netip.Addr
}

// planChecks derives the checks for a plan.
func planChecks(plan softnetPlan, gateway netip.Addr, proxy *netip.AddrPort, f netscope.Facts) netChecks {
	c := netChecks{PingUp: []netip.Addr{gateway}, PingDown: []netip.Addr{publicCanary}}
	if proxy != nil {
		c.TCPOpen = append(c.TCPOpen, *proxy)
	}
	seen := map[netip.Addr]bool{gateway: true, publicCanary: true}
	add := func(a netip.Addr) {
		a = netscope.Unmap(a)
		if a.Is4() && !seen[a] && len(c.PingDown) < maxNegativeTargets {
			seen[a] = true
			c.PingDown = append(c.PingDown, a)
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

// script renders the check as a bash script; every output line is `OPEN x`, `UP x` or `DOWN x`.
func (c netChecks) script() []byte {
	var b bytes.Buffer
	b.WriteString("tcp() { ( exec 3<>/dev/tcp/$1/$2 ) >/dev/null 2>&1 & p=$!; ( sleep 4; kill $p ) >/dev/null 2>&1 & k=$!; " +
		"if wait $p 2>/dev/null; then echo \"OPEN $1:$2\"; else echo \"SHUT $1:$2\"; fi; kill $k >/dev/null 2>&1; }\n")
	b.WriteString("png() { if ping -c 1 -t 2 \"$1\" >/dev/null 2>&1; then echo \"UP $1\"; else echo \"DOWN $1\"; fi; }\n")
	for _, ap := range c.TCPOpen {
		fmt.Fprintf(&b, "tcp %s %d\n", ap.Addr(), ap.Port())
	}
	for _, a := range c.PingUp {
		fmt.Fprintf(&b, "png %s\n", a)
	}
	for _, a := range c.PingDown {
		fmt.Fprintf(&b, "png %s\n", a)
	}
	b.WriteString("exit 0\n")
	return b.Bytes()
}

// judge compares the script output with the checks.
func (c netChecks) judge(out []byte) error {
	got := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		if k, v, ok := strings.Cut(strings.TrimSpace(sc.Text()), " "); ok && (k == "OPEN" || k == "SHUT" || k == "UP" || k == "DOWN") {
			got[v] = k
		}
	}
	for _, ap := range c.TCPOpen {
		if got[ap.String()] != "OPEN" {
			return isolation.ScopeUnenforceable("the VM cannot reach the egress proxy at %s", ap)
		}
	}
	for _, a := range c.PingUp {
		if len(c.TCPOpen) == 0 && got[a.String()] != "UP" { // without a proxy this is the only positive control
			return isolation.ScopeUnenforceable("the network check has no positive control: the VM got no answer from the gateway %s", a)
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
