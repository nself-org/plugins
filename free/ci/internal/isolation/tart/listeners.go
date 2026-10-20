package tart

// Purpose: refuse a scope below lan when the host listens on every interface for anything the check cannot name.
// Inputs:  `netstat -anv -p tcp` of the host, run through the exec funnel.
// Outputs: the wildcard-bound TCP listeners; nil error when there are none.
// Constraints: softnet filters by address, so a service bound to every interface is reachable from the VM at the
//          gateway whatever its port, and the fixed port list in verify.go cannot name them all. Listeners bound
//          to one address (loopback, the egress proxy on the gateway) are not on this list. A scan that cannot
//          run fails closed (E679), never passes.

import (
	"bufio"
	"bytes"
	"context"
	"sort"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/isolation"
)

// listener is one TCP listen socket bound to every interface.
type listener struct{ Proto, Port string }

// parseListeners reads macOS `netstat -an -p tcp` output: a LISTEN row whose local address is `*.port`, `::.port`
// or `0.0.0.0.port`. The result is sorted and without duplicates (a tcp46 socket is one row, tcp4 plus tcp6 two).
func parseListeners(out []byte) []listener {
	seen := map[listener]bool{}
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 6 || !strings.HasPrefix(f[0], "tcp") || f[5] != "LISTEN" {
			continue
		}
		i := strings.LastIndexByte(f[3], '.')
		if i < 0 {
			continue
		}
		switch f[3][:i] {
		case "*", "::", "0.0.0.0":
			seen[listener{Proto: f[0], Port: f[3][i+1:]}] = true
		}
	}
	var ls []listener
	for l := range seen {
		ls = append(ls, l)
	}
	sort.Slice(ls, func(i, j int) bool { return ls[i].Port+ls[i].Proto < ls[j].Port+ls[j].Proto })
	return ls
}

// checkHostListeners fails when the host has a wildcard-bound TCP listener.
func (a *attempt) checkHostListeners(ctx context.Context) error {
	res, err := a.tool.Run(ctx, []string{a.p.opts.Netstat, "-anv", "-p", "tcp"}, nil, nil)
	if err != nil {
		return isolation.ScopeUnenforceable("the host's listening ports could not be listed: %v", err)
	}
	if res.Exit != 0 {
		return isolation.ScopeUnenforceable("the host's listening ports could not be listed (netstat exit %d)", res.Exit)
	}
	ls := parseListeners(res.Out)
	if len(ls) == 0 {
		return nil
	}
	ports := make([]string, 0, len(ls))
	for _, l := range ls {
		ports = append(ports, l.Port)
	}
	return isolation.ScopeUnenforceable("this host listens on every interface (tcp port %s), and softnet cannot filter by port, so the %s scope cannot hold: turn those services off or bind them to loopback", strings.Join(ports, ", "), a.plan.Scope)
}
