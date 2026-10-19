package main

// Purpose: tests of `serve` in V15 mode (P7-NODE-10): the TEAM process role, the start guard before any
// bind, the loopback default address, every v1.4 flag accepted and the dispatch mapping.
// Inputs:  injected guard host (serveGuardLocal), a fake Listen, temporary directories.
// Outputs: pass or fail.
// Constraints: nothing reads the developer's machine or opens a real port; the fixture host has no docker.

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/nodes/deployguard"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// quietHost is a host that is not a deploy host and has no docker runner.
func quietHost() deployguard.Local {
	return deployguard.Local{
		Hostname: func() (string, error) { return "ci-box", nil },
		Addrs:    func() ([]netip.Addr, error) { return nil, nil },
		ReadFile: func(string) ([]byte, error) { return nil, os.ErrNotExist },
		Glob:     func(string) ([]string, error) { return nil, nil },
	}
}

// prodHost has a compose project whose .env says ENV=production, found through a fake docker.
func prodHost(t *testing.T) deployguard.Local {
	t.Helper()
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	l := quietHost()
	l.ReadFile = func(p string) ([]byte, error) {
		if p == "/srv/stack/.env" {
			return []byte("ENV=production\n"), nil
		}
		return nil, os.ErrNotExist
	}
	l.Run = func(context.Context, string, ...string) ([]byte, int, error) { return []byte("/srv/stack\n"), 0, nil }
	return l
}

func serveFixture(t *testing.T, host deployguard.Local) (dir string) {
	t.Helper()
	dir = t.TempDir()
	oldLocal, oldHome, oldDispatch := serveGuardLocal, serveGuardHome, serveDispatch
	serveGuardLocal, serveGuardHome = func() deployguard.Local { return host }, dir
	serveDispatch = func(string, *v15ServeFlags) coordinator.Dispatcher { return nil }
	t.Cleanup(func() {
		serveGuardLocal, serveGuardHome, serveDispatch = oldLocal, oldHome, oldDispatch
		report.SetProcessRole(report.RolePersonal)
	})
	return dir
}

func TestV15ServeSetsRoleTeam(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	dir := serveFixture(t, quietHost())
	report.SetProcessRole(report.RolePersonal)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var bound atomic.Value
	hook := func(c *coordinator.Config) {
		c.Listen = func(network, addr string) (net.Listener, error) {
			bound.Store(addr)
			go cancel() // the listener exists: stop the daemon
			return net.Listen(network, "127.0.0.1:0")
		}
	}
	args := []string{"--insecure", "--workdir", filepath.Join(dir, "wd"), "--data-dir", dir}
	if code := serveV15(ctx, args, hook); code != 0 {
		t.Fatalf("serve exit = %d", code)
	}
	if report.ProcessRole() != report.RoleTeam {
		t.Fatal("V15 serve start did not set RoleTeam")
	}
	if bound.Load() != "127.0.0.1:3845" {
		t.Fatalf("default webhook address = %v, want 127.0.0.1:3845", bound.Load())
	}
	// v1.4 mode: the frozen serve leaves the role alone
	t.Setenv("NSELF_V15", "0")
	report.SetProcessRole(report.RolePersonal)
	if code := run([]string{"serve", "--insecure", "--addr", "999.999.999.999:1", "--workdir", filepath.Join(dir, "wd14")}); code != 1 {
		t.Fatalf("v1.4 serve with a bad address = %d, want 1", code)
	}
	if report.ProcessRole() != report.RolePersonal {
		t.Fatal("v1.4 serve changed the process role")
	}
}

func TestV15ServeRefusesDeployHostBeforeAnyBind(t *testing.T) {
	dir := serveFixture(t, prodHost(t))
	var binds atomic.Int32
	hook := func(c *coordinator.Config) {
		c.Listen = func(string, string) (net.Listener, error) { binds.Add(1); return nil, errors.New("must not bind") }
	}
	code := serveV15(context.Background(), []string{"--insecure", "--workdir", filepath.Join(dir, "wd"), "--data-dir", dir}, hook)
	if code == 0 || binds.Load() != 0 {
		t.Fatalf("exit=%d binds=%d; want a refusal before any bind", code, binds.Load())
	}
	if _, err := os.Stat(filepath.Join(dir, "state.db")); err == nil {
		t.Fatal("the guard ran after the store was opened")
	}
}

func TestV15ServeFlags(t *testing.T) {
	fs, f, _ := newServeV15Flags()
	if fs.Lookup("addr").DefValue != "127.0.0.1:3845" {
		t.Fatalf("V15 --addr default = %q", fs.Lookup("addr").DefValue)
	}
	// every v1.4 flag is accepted, plus --data-dir and --force
	args := []string{"--addr", "0.0.0.0:9", "--secret", "s", "--concurrency", "3", "--workdir", "/w", "--timeout", "5", "-v", "--verbose",
		"--insecure", "--allow-unsandboxed", "--allowed-repos", "a/b,c/d", "--allowed-repos", "e/f", "--data-dir", "/d", "--force"}
	if err := fs.Parse(args); err != nil {
		t.Fatal(err)
	}
	if f.addr != "0.0.0.0:9" || f.concurrency != 3 || f.timeout != 5 || !f.verbose || !f.insecure || !f.unsandboxed || !f.force ||
		f.dataDir != "/d" || len(f.allowed.vals) != 3 {
		t.Fatalf("flags = %+v", f)
	}
	v14, err := parseServeArgs(nil, 0)
	if err != nil || v14.Addr != ":3845" {
		t.Fatalf("v1.4 default address changed: %q, %v", v14.Addr, err)
	}
}

func TestV15ServeFlagsAfterPositionalCount(t *testing.T) {
	fs, f, _ := newServeV15Flags()
	pos, err := parseInterspersed(fs, []string{"extra", "--addr", "0.0.0.0:9", "more", "--insecure"})
	if err != nil || len(pos) != 2 || f.addr != "0.0.0.0:9" || !f.insecure {
		t.Fatalf("pos=%v addr=%q insecure=%v err=%v: flags after a positional argument were dropped", pos, f.addr, f.insecure, err)
	}
}

func TestV15ServeRegisteredV15Only(t *testing.T) {
	e, ok := v15Subcommands["serve"]
	if !ok || !e.V15Only || e.Handler == nil || e.Flags == nil {
		t.Fatalf("V15 serve entry = %+v", e)
	}
	if subcommands["serve"] == nil {
		t.Fatal("the v1.4 serve entry is gone")
	}
}

func TestServeTriggerMapping(t *testing.T) {
	for in, want := range map[string]string{"push": "push", "pull_request": "pull_request", "api": "api", "": "", "weird": ""} {
		if got := serveTrigger(in); got != want {
			t.Fatalf("serveTrigger(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDispatchPipelineReportsCheckoutFailure(t *testing.T) {
	f := &v15ServeFlags{workdir: filepath.Join(t.TempDir(), "missing")}
	_, err := dispatchPipeline(t.TempDir(), f)(context.Background(), store.IntakeRow{Source: "gitea-hook", Project: "o/r", Revision: "abc1234def5678901234567890abcdef12345678", Facts: []byte(`{"clone_url":"https://127.0.0.1:1/o/r.git"}`)})
	if err == nil {
		t.Fatal("a failed checkout was reported as success")
	}
}

func TestDispatchPipelineRefusesBadRows(t *testing.T) {
	f := &v15ServeFlags{workdir: t.TempDir()}
	good := "abc1234def5678901234567890abcdef12345678"
	rows := map[string]store.IntakeRow{
		"git option as revision": {Source: "github", Project: "o/r", Revision: "--upload-pack=x"},
		"git option as project":  {Source: "github", Project: "-o/r", Revision: good},
		"file clone url":         {Source: "github", Project: "o/r", Revision: good, Facts: []byte(`{"clone_url":"file:///srv/repo"}`)},
		"foreign host":           {Source: "github", Project: "o/r", Revision: good, Facts: []byte(`{"clone_url":"https://evil.example.test/o/r.git"}`)},
		"synthetic pipeline":     {Source: "github", Project: "o/r", Revision: good, Facts: []byte(`{"synthetic":{"name":"x"}}`)},
	}
	for name, row := range rows {
		if _, err := dispatchPipeline(t.TempDir(), f)(context.Background(), row); err == nil {
			t.Errorf("%s was dispatched", name)
		}
	}
	if entries, _ := os.ReadDir(f.workdir); len(entries) != 0 {
		t.Fatalf("a refused row left %d checkouts behind", len(entries))
	}
}
