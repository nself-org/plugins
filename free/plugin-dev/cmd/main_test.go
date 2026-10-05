package main

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// capture runs fn with stdout and stderr redirected to pipes.
func capture(t *testing.T, fn func()) (string, string) {
	t.Helper()
	ro, wo, _ := os.Pipe()
	re, we, _ := os.Pipe()
	so, se := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = wo, we
	fn()
	os.Stdout, os.Stderr = so, se
	_ = wo.Close()
	_ = we.Close()
	bo, _ := io.ReadAll(ro)
	be, _ := io.ReadAll(re)
	return string(bo), string(be)
}

func sandbox(t *testing.T) string {
	t.Helper()
	h := t.TempDir()
	t.Setenv("HOME", h)
	t.Setenv("NO_COLOR", "1")
	return h
}

func TestRootHelpAndUnknown(t *testing.T) {
	out, _ := capture(t, func() {
		if run(nil) != 0 {
			t.Error("bare invocation must exit 0")
		}
	})
	for _, c := range commands {
		if !strings.Contains(out, c.name) {
			t.Errorf("root help lacks %s", c.name)
		}
	}
	_, errOut := capture(t, func() {
		if run([]string{"bogus"}) != 1 {
			t.Error("unknown command must exit 1")
		}
	})
	if !strings.Contains(errOut, `unknown command "bogus"`) {
		t.Fatalf("stderr %q", errOut)
	}
}

func TestEveryCommandHasHelpAndAlias(t *testing.T) {
	for _, c := range commands {
		out, _ := capture(t, func() { run([]string{c.name, "--help"}) })
		if !strings.Contains(out, "Usage:\n  nself plugin-dev "+c.name) {
			t.Errorf("%s help usage line: %q", c.name, out)
		}
	}
	if c, ok := lookup("scaffold"); !ok || c.name != "init" {
		t.Fatal("scaffold alias must resolve to init")
	}
}

func TestDeprecatedNewNotice(t *testing.T) {
	sandbox(t)
	_, errOut := capture(t, func() { run([]string{"new", "--help"}) })
	if errOut != "Command \"new\" is deprecated, use 'nself plugin-dev init' instead\n" {
		t.Fatalf("stderr %q", errOut)
	}
}

func TestLinkUnlinkRoundTrip(t *testing.T) {
	h := sandbox(t)
	plug := filepath.Join(t.TempDir(), "p")
	_ = os.MkdirAll(plug, 0o750)
	_ = os.WriteFile(filepath.Join(plug, "plugin.yaml"), []byte("name: rt\n"), 0o600)
	out, _ := capture(t, func() {
		if run([]string{"link", plug, "--host"}) != 0 {
			t.Error("link failed")
		}
	})
	if !strings.Contains(out, "Linked rt -> "+plug+" (host-process mode)") {
		t.Fatalf("stdout %q", out)
	}
	if _, err := os.Stat(filepath.Join(h, ".nself", "plugin-links.json")); err != nil {
		t.Fatal(err)
	}
	out, _ = capture(t, func() { run([]string{"link", "--list", "x"}) })
	if !strings.Contains(out, "│ rt   │") {
		t.Fatalf("list %q", out)
	}
	out, _ = capture(t, func() { run([]string{"unlink", "rt"}) })
	if !strings.Contains(out, "Unlinked rt.") {
		t.Fatalf("unlink %q", out)
	}
	out, _ = capture(t, func() { run([]string{"unlink", "rt"}) })
	if !strings.Contains(out, `Plugin "rt" is not linked (no-op).`) {
		t.Fatalf("noop %q", out)
	}
	out, _ = capture(t, func() { run([]string{"link", "--list", "x"}) })
	if !strings.Contains(out, "No plugins currently linked.") {
		t.Fatalf("empty list %q", out)
	}
}

func TestFailUsesChildExitCode(t *testing.T) {
	err := exec.Command("sh", "-c", "exit 7").Run()
	_, _ = capture(t, func() {
		if got := fail(errors.New("plain")); got != 1 {
			t.Errorf("plain error status %d", got)
		}
		if got := fail(err); got != 7 {
			t.Errorf("child status %d", got)
		}
	})
}

func TestInitCreatesTreeNoPrompt(t *testing.T) {
	sandbox(t)
	t.Chdir(t.TempDir())
	out, _ := capture(t, func() {
		if run([]string{"init", "demo", "--no-interactive"}) != 0 {
			t.Error("init failed")
		}
	})
	if !strings.Contains(out, `Scaffolding go plugin "demo" (tier: free, tenancy: none)...`) {
		t.Fatalf("stdout %q", out)
	}
	if _, err := os.Stat("demo/plugin.json"); err != nil {
		t.Fatal(err)
	}
	_, errOut := capture(t, func() {
		if run([]string{"init", "demo", "--no-interactive"}) != 1 {
			t.Error("a non-empty destination must fail")
		}
	})
	if !strings.Contains(errOut, "is not empty") {
		t.Fatalf("stderr %q", errOut)
	}
}

// TestPromptTenancyAnswers drives the interactive prompt through a pipe on stdin.
func TestPromptTenancyAnswers(t *testing.T) {
	for in, want := range map[string]string{
		"n\n": "none", "\n": "none", "y\na\n": "app-isolation", "yes\nb\n": "cloud-tenant",
		"Y\nC\n": "both", "y\nd\n": "none", "y\nz\n": "none",
	} {
		r, w, _ := os.Pipe()
		_, _ = w.WriteString(in)
		_ = w.Close()
		old := os.Stdin
		os.Stdin = r
		var got string
		capture(t, func() {
			m, err := promptTenancy()
			if err != nil {
				t.Errorf("%q: %v", in, err)
			}
			got = string(m)
		})
		os.Stdin = old
		if got != want {
			t.Errorf("%q: got %s want %s", in, got, want)
		}
	}
	r, w, _ := os.Pipe()
	_ = w.Close()
	old := os.Stdin
	os.Stdin = r
	var err error
	capture(t, func() { _, err = promptTenancy() })
	os.Stdin = old
	if err == nil || !strings.Contains(err.Error(), "reading input") {
		t.Fatalf("EOF: %v", err)
	}
}

func TestResolveTenancy(t *testing.T) {
	if m, err := resolveTenancy("both", false); err != nil || m != "both" {
		t.Fatalf("%v %v", m, err)
	}
	if _, err := resolveTenancy("x", false); err == nil {
		t.Fatal("bad tenancy accepted")
	}
	if m, _ := resolveTenancy("", true); m != "none" {
		t.Fatalf("no-interactive: %s", m)
	}
}

func TestResolveDebugPort(t *testing.T) {
	if p, err := resolveDebugPort(3000); err != nil || p != 3000 {
		t.Fatalf("%d %v", p, err)
	}
	for _, bad := range []int{80, 1023, 65536} {
		if _, err := resolveDebugPort(bad); err == nil {
			t.Errorf("%d accepted", bad)
		}
	}
	if p, err := resolveDebugPort(0); err != nil || p < 2345 || p > 2399 {
		t.Fatalf("auto %d %v", p, err)
	}
}

// stubBin writes an executable shell script named name into a fresh dir that is
// put first on PATH; the script appends its argv to the returned log file.
func stubBin(t *testing.T, name, body string) string {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, name+".log")
	script := "#!/bin/sh\necho \"$@\" >> '" + log + "'\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

// TestSmokePhasesUseNselfOnPath covers the smoke install and uninstall phases that core
// cannot run offline: a stub nself records `plugin install <name> --force` and
// `plugin remove <name>`, a stub curl answers 200.
func TestSmokePhasesUseNselfOnPath(t *testing.T) {
	sandbox(t)
	nlog := stubBin(t, "nself", "exit 0")
	stubBin(t, "curl", "printf 200")
	if err := runSmokeInstall("fx"); err != nil {
		t.Fatal(err)
	}
	if err := runSmokeUninstall("fx"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(nlog)
	if string(b) != "plugin install fx --force\nplugin remove fx\n" {
		t.Fatalf("nself calls %q", b)
	}
}

func TestSmokeInstallFailureAndHealthFailure(t *testing.T) {
	sandbox(t)
	stubBin(t, "nself", "exit 4")
	if err := runSmokeInstall("fx"); err == nil || !strings.Contains(err.Error(), "smoke install failed") {
		t.Fatalf("%v", err)
	}
	if err := runSmokeUninstall("fx"); err == nil || !strings.Contains(err.Error(), "smoke uninstall failed") {
		t.Fatalf("%v", err)
	}
	stubBin(t, "curl", "printf 503")
	if err := waitForHealth("http://x/healthz", 2, time.Millisecond); err == nil || !strings.Contains(err.Error(), "did not return 200 after 2 retries") {
		t.Fatalf("%v", err)
	}
}

func TestPluginTestPhasesAndHealthURL(t *testing.T) {
	sandbox(t)
	t.Setenv("NSELF_LOCAL_URL", "http://h:1")
	if got := resolvePluginHealthURL("x"); got != "http://h:1/healthz" {
		t.Fatal(got)
	}
	_, errOut := capture(t, func() {
		if run([]string{"test", "x", "--phase", "bogus"}) != 1 {
			t.Error("bad phase must exit 1")
		}
	})
	if !strings.Contains(errOut, `invalid --phase "bogus": must be unit, smoke, or both`) {
		t.Fatalf("%q", errOut)
	}
}

func TestEntrypointPolicy(t *testing.T) {
	sandbox(t)
	plug := filepath.Join(t.TempDir(), "p")
	_ = os.MkdirAll(plug, 0o750)
	_ = os.WriteFile(filepath.Join(plug, "plugin.yaml"), []byte("name: ep\n"), 0o600)
	t.Chdir(plug)
	for _, ep := range []string{"../x", "a b", "a/../..", "$(id)"} {
		_, errOut := capture(t, func() {
			if run([]string{"dev", "ep", "--no-link", "--entrypoint", ep}) != 1 {
				t.Errorf("%q accepted", ep)
			}
		})
		if !strings.Contains(errOut, "plugin dev: entrypoint") {
			t.Errorf("%q: stderr %q", ep, errOut)
		}
	}
}

func TestInlineWatchScriptWritten(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	p, err := writeInlineDevWatchScript()
	if err != nil || filepath.Dir(p) != tmp {
		t.Fatalf("%q %v", p, err)
	}
	b, _ := os.ReadFile(p)
	if !strings.HasPrefix(string(b), "#!/usr/bin/env bash\nset -euo pipefail\n") {
		t.Fatalf("script %q", b)
	}
}
