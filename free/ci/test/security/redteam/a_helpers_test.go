//go:build security

package redteam

// Purpose: helpers shared by the a_* red team files (group TM-01, 02, 04 to 11, 45).
// Inputs:  none. Outputs: aModelScope, aServeProxy, aConnectStatus, aFakeDocker.
// Constraints: loopback and temp dirs only; the fake docker CLI only records the argv of `run`.

import (
	"bufio"
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/netscope"
)

func aModelScope(s string) model.NetworkScope { return model.NetworkScope(s) }

// aServeProxy starts p on a loopback listener and returns it with a stop func.
func aServeProxy(t *testing.T, p *netscope.Proxy) (net.Listener, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = p.Serve(ctx, ln); close(done) }()
	return ln, func() { cancel(); _ = ln.Close(); <-done }
}

// aConnectStatus sends one CONNECT and returns the status line ("" on a closed connection).
func aConnectStatus(t *testing.T, proxy, target string) string {
	t.Helper()
	c, err := net.DialTimeout("tcp", proxy, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	_, _ = c.Write([]byte("CONNECT " + target + " HTTP/1.1\r\nHost: " + target + "\r\n\r\n"))
	line, _ := bufio.NewReader(c).ReadString('\n')
	return strings.TrimSpace(line)
}

// aFakeDocker puts a `docker` on PATH that appends the argv of every `run` to a log, one argument per
// line with a "--" separator line between runs, and returns a reader of that log.
func aFakeDocker(t *testing.T) func() [][]string {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "runs.log")
	script := "#!/bin/sh\nif [ \"$1\" = run ]; then\n  for a in \"$@\"; do printf '%s\\n' \"$a\" >> '" + log + "'; done\n  printf -- '--SEP--\\n' >> '" + log + "'\nfi\nexit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":/usr/bin:/bin")
	return func() [][]string {
		raw, _ := os.ReadFile(log)
		var runs [][]string
		var cur []string
		for _, l := range strings.Split(strings.TrimRight(string(raw), "\n"), "\n") {
			switch {
			case l == "":
			case l == "--SEP--":
				runs = append(runs, cur)
				cur = nil
			default:
				cur = append(cur, l)
			}
		}
		return runs
	}
}
