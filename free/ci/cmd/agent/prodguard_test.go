package main

import (
	"bytes"
	"context"
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/nodes/deployguard"
)

// Every test in this package runs on a calm fixture host with an empty CI home, never on the developer's
// machine; the guard tests below swap in the host they need.
func init() {
	prodLocal, prodHome = calmLocal, "/nonexistent/nself-ci-home"
}

func calmLocal() deployguard.Local { return hostLocal("calm.example.test", "192.0.2.4", nil, nil) }

func hostLocal(host, addr string, files map[string]string, docker deployguard.RunFunc) deployguard.Local {
	return deployguard.Local{
		Hostname: func() (string, error) { return host, nil },
		Addrs:    func() ([]netip.Addr, error) { return []netip.Addr{netip.MustParseAddr(addr)}, nil },
		ReadFile: func(p string) ([]byte, error) {
			if v, ok := files[p]; ok {
				return []byte(v), nil
			}
			return nil, os.ErrNotExist
		},
		Glob: func(string) ([]string, error) { return nil, nil },
		Run:  docker,
	}
}

func useGuard(t *testing.T, l deployguard.Local, neverList string) {
	t.Helper()
	home := t.TempDir()
	if neverList != "" {
		if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("ci:\n  never_hosts: ["+neverList+"]\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	oldLocal, oldHome := prodLocal, prodHome
	prodLocal, prodHome = func() deployguard.Local { return l }, home
	t.Cleanup(func() { prodLocal, prodHome = oldLocal, oldHome })
}

func TestServeStdioExitsNonZeroWithE663OnADeployHost(t *testing.T) {
	useState(t)
	useGuard(t, hostLocal("box.example.test", "203.0.113.10", nil, nil), "203.0.113.10")
	in := &tripReader{t: t}
	var out, errb bytes.Buffer
	code := run(context.Background(), []string{"serve", "--stdio"}, IO{Stdin: in, Stdout: &out, Stderr: &errb})
	if code == 0 || in.read || out.Len() != 0 || !strings.Contains(errb.String(), "E663") || !strings.Contains(errb.String(), "refuses to start") {
		t.Fatalf("code %d read %v stdout %q stderr %q", code, in.read, out.String(), errb.String())
	}
}

func TestServeStdioExitsNonZeroWithE663OnAProdStackMarker(t *testing.T) {
	useState(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("ENV=prod\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	docker := func(context.Context, string, ...string) ([]byte, int, error) { return []byte(dir + "\n"), 0, nil }
	l := hostLocal("calm.example.test", "192.0.2.4", nil, docker)
	l.ReadFile = os.ReadFile
	useGuard(t, l, "")
	code, _, stderr := runCmd(t, "serve", "--stdio")
	if code == 0 || !strings.Contains(stderr, "E663") || !strings.Contains(stderr, "production stack") {
		t.Fatalf("code %d stderr %q", code, stderr)
	}
}

func TestGuardStopsServeJoinAndPullThroughThePreStartHook(t *testing.T) {
	useGuard(t, hostLocal("box.example.test", "203.0.113.10", nil, nil), "box.example.test")
	for _, cmd := range []string{"serve", "join", "pull"} {
		err := RunPreStart(context.Background(), cmd)
		var e *deployguard.Error
		if !errors.As(err, &e) || e.Code != "E663" || !strings.Contains(err.Error(), "nself-ci-agent "+cmd) {
			t.Errorf("%s: %v", cmd, err)
		}
	}
	useGuard(t, calmLocal(), "box.example.test")
	for _, cmd := range []string{"serve", "join", "pull"} {
		if err := RunPreStart(context.Background(), cmd); err != nil {
			t.Errorf("calm host, %s: %v", cmd, err)
		}
	}
}

func TestGuardIsRegisteredFirst(t *testing.T) {
	regMu.RLock()
	defer regMu.RUnlock()
	if len(prestart) == 0 || prestart[0].name != prodGuardName {
		t.Fatalf("pre-start order %+v", prestart)
	}
}

// Every subcommand that runs a job must go through RunPreStart: a deploy host stops it with E663 before
// it reads its spec. A new subcommand has to be classified here, so a job runner cannot skip the guard
// by accident.
func TestEveryJobRunningSubcommandRunsThePreStartGuard(t *testing.T) {
	guarded := map[string][]string{"serve": {"--stdio"}, "exec": {"--spec", "/nonexistent", "--evidence", "/nonexistent"}}
	exempt := map[string]bool{ // no job runs, or started by an already guarded agent
		"version": true, "slots": true, "verify-release": true, "job-shim": true, "cred-wrapper": true,
	}
	regMu.RLock()
	var names []string
	for n := range commands {
		names = append(names, n)
	}
	regMu.RUnlock()
	for _, n := range names {
		if _, ok := guarded[n]; !ok && !exempt[n] {
			t.Errorf("subcommand %q is neither guarded nor exempt: classify it", n)
		}
	}
	useGuard(t, hostLocal("box.example.test", "203.0.113.10", nil, nil), "box.example.test")
	for n, args := range guarded {
		code, _, stderr := runCmd(t, append([]string{n}, args...)...)
		if code == 0 || !strings.Contains(stderr, "E663") {
			t.Errorf("%s: code %d stderr %q", n, code, stderr)
		}
	}
}
