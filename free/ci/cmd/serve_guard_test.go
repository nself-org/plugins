package main

// Purpose: D-1218. `ci serve` is a deploy-host start guard caller and the owner of the 10-minute exclusion
// refresh (Guard.Run); both used to have no production caller.
// Inputs:  fixture hosts and CI homes only; the daemon and the refresh loop are replaced by recorders.
// Outputs: assertions on the exit code, stderr, and which seams ran.
// Constraints: never reads the developer's host or CI home.

import (
	"context"
	"flag"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/nodes/deployguard"
	"github.com/nself-org/plugins/free/ci/internal/serve"
)

func serveFixtureHost(host, addr string) deployguard.Local {
	return deployguard.Local{
		Hostname: func() (string, error) { return host, nil },
		Addrs:    func() ([]netip.Addr, error) { return []netip.Addr{netip.MustParseAddr(addr)}, nil },
		ReadFile: func(string) ([]byte, error) { return nil, os.ErrNotExist },
		Glob:     func(string) ([]string, error) { return nil, nil },
	}
}

// useServeSeams swaps the serve seams for recorders and returns counters for the daemon and the refresh.
func useServeSeams(t *testing.T, l deployguard.Local, neverList string) (daemon, refresh *int) {
	t.Helper()
	home := t.TempDir()
	if neverList != "" {
		cfg := "ci:\n  never_hosts: [" + neverList + "]\n"
		if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte(cfg), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	oldLocal, oldHome, oldRun, oldRefresh := serveLocal, serveHome, serveRun, serveRefresh
	t.Cleanup(func() { serveLocal, serveHome, serveRun, serveRefresh = oldLocal, oldHome, oldRun, oldRefresh })
	daemon, refresh = new(int), new(int)
	serveLocal = func() deployguard.Local { return l }
	serveHome = func() string { return home }
	serveRun = func(serve.ServeConfig) error { *daemon++; return nil }
	serveRefresh = func(ctx context.Context, _ string, _ func(error)) { *refresh++; <-ctx.Done() }
	return daemon, refresh
}

func TestServeRefusesToStartOnADeployHost(t *testing.T) {
	daemon, refresh := useServeSeams(t, serveFixtureHost("box.example.test", "203.0.113.10"), "203.0.113.10")
	stderr := captureStderr(t, func() {
		if code := serveCmd([]string{"--insecure"}, flag.ContinueOnError); code == 0 {
			t.Fatalf("serve started on a deploy host")
		}
	})
	if *daemon != 0 || *refresh != 0 || !strings.Contains(stderr, "E663") {
		t.Fatalf("daemon %d refresh %d stderr %q", *daemon, *refresh, stderr)
	}
}

func TestServeStartRunsTheGuardRefreshAndStopsItWithTheDaemon(t *testing.T) {
	daemon, refresh := useServeSeams(t, serveFixtureHost("calm.example.test", "192.0.2.4"), "")
	if code := serveCmd([]string{"--insecure"}, flag.ContinueOnError); code != 0 {
		t.Fatalf("code %d", code)
	}
	if *daemon != 1 || *refresh != 1 {
		t.Fatalf("daemon %d refresh %d", *daemon, *refresh)
	}
}
