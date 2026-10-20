//go:build security

package redteam

// Purpose: red team group F helpers (P7-HOST-12 providers commands, P7-TRUST-25 last-verified record). The
// providers commands live in package main, so the attacks run the REAL nself-ci binary built from ./cmd
// against a fixture: a temp CI home, a temp git repository, a PATH that holds only git and a fake gh.
// Inputs:  none. Outputs: fBin (build, run, list helpers), fCanary (loopback listener that counts connections).
// Constraints: loopback and temp dirs only; no docker (PATH holds only git and the fake gh), no network, no
//          real account: the fake gh prints a synthetic token; every secret is fakeSecret.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fBin struct {
	t               *testing.T
	exe, home, repo string
	pathDir, ghLog  string
	extra           []string
}

var (
	fBuildOnce sync.Once
	fBuildPath string
	fBuildErr  error
)

// fBinary builds nself-ci once per test process (every F test runs the same binary) into a directory that
// TestMain removes.
func fBinary(t *testing.T) string {
	t.Helper()
	fBuildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "redteam-bin-")
		if err != nil {
			fBuildErr = err
			return
		}
		fBuildDir = dir
		fBuildPath = filepath.Join(dir, "nself-ci")
		build := exec.Command("go", "build", "-mod=vendor", "-o", fBuildPath, "github.com/nself-org/plugins/free/ci/cmd")
		build.Env = append(os.Environ(), "CGO_ENABLED=0")
		if out, err := build.CombinedOutput(); err != nil {
			fBuildErr = fmt.Errorf("build nself-ci: %v\n%s", err, out)
		}
	})
	if fBuildErr != nil {
		t.Fatal(fBuildErr)
	}
	return fBuildPath
}

var fBuildDir string

func TestMain(m *testing.M) {
	code := m.Run()
	if fBuildDir != "" {
		_ = os.RemoveAll(fBuildDir)
	}
	os.Exit(code)
}

// fNew builds the binary and a fixture (home, git repo, PATH) for one test.
func fNew(t *testing.T) *fBin {
	t.Helper()
	root := t.TempDir()
	b := &fBin{t: t, home: filepath.Join(root, "ci-home"), repo: filepath.Join(root, "repo"),
		pathDir: filepath.Join(root, "bin"), ghLog: filepath.Join(root, "gh.log")}
	for _, d := range []string{b.home, b.repo, b.pathDir, filepath.Join(root, "userhome")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	b.exe = fBinary(t)
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	b.pathDir += ":/usr/bin:/bin:" + filepath.Dir(git) // git needs its own directory and /bin
	ghDir := strings.Split(b.pathDir, string(os.PathListSeparator))[0]
	gh := "#!/bin/sh\necho \"$@\" >> '" + b.ghLog + "'\nprintf 'github.com\\n  Logged in as fake\\n  - Token: ghp_" + fakeSecret + "\\n'\n"
	if err = os.WriteFile(filepath.Join(ghDir, "gh"), []byte(gh), 0o755); err != nil {
		t.Fatal(err)
	}
	b.git("init", "-q", b.repo)
	b.git("-C", b.repo, "-c", "user.name=a", "-c", "user.email=a@example.invalid", "commit", "-q", "--allow-empty", "-m", "init")
	return b
}

func (b *fBin) git(args ...string) {
	b.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Env = []string{"PATH=" + b.pathDir, "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1"}
	if out, err := cmd.CombinedOutput(); err != nil {
		b.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func (b *fBin) env(extra ...string) []string {
	env := []string{"PATH=" + b.pathDir, "HOME=" + filepath.Join(filepath.Dir(b.home), "userhome"), "NSELF_CI_HOME=" + b.home, "NSELF_V15=1",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1"}
	return append(append(env, b.extra...), extra...)
}

// run executes the binary in the fixture repository; extra is KEY=VALUE pairs for this call only.
func (b *fBin) run(extra []string, args ...string) (stdout, stderr string, exit int) {
	b.t.Helper()
	return b.runIn(b.repo, extra, args...)
}

func (b *fBin) runIn(dir string, extra []string, args ...string) (stdout, stderr string, exit int) {
	b.t.Helper()
	cmd := exec.Command(b.exe, args...)
	cmd.Dir, cmd.Env = dir, b.env(extra...)
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	err := cmd.Run()
	exit = 0
	if ee, ok := err.(*exec.ExitError); ok {
		exit = ee.ExitCode()
	} else if err != nil {
		b.t.Fatalf("run %v: %v", args, err)
	}
	return so.String(), se.String(), exit
}

// authorized is the trust.providers list `providers list --json` reports for the fixture repository.
func (b *fBin) authorized(dir string) (list []string, zone string) {
	b.t.Helper()
	out, errOut, exit := b.runIn(dir, nil, "providers", "list", "--json")
	var env struct {
		Data struct {
			Authorized  []string `json:"authorized"`
			PrivacyZone string   `json:"privacy_zone"`
		} `json:"data"`
	}
	if exit != 0 || json.Unmarshal([]byte(out), &env) != nil {
		b.t.Fatalf("providers list: exit %d\n%s\n%s", exit, out, errOut)
	}
	return env.Data.Authorized, env.Data.PrivacyZone
}

// doctor returns the checks of `doctor --providers --json` keyed by id (the first of a repeated id wins).
func (b *fBin) doctor(extra []string) map[string]map[string]any {
	b.t.Helper()
	out, errOut, _ := b.run(extra, "doctor", "--providers", "--json")
	var env struct {
		Data struct {
			Checks []map[string]any `json:"checks"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		b.t.Fatalf("doctor output: %v\n%s\n%s", err, out, errOut)
	}
	m := map[string]map[string]any{}
	for _, c := range env.Data.Checks {
		if id, _ := c["id"].(string); m[id] == nil {
			m[id] = c
		}
	}
	return m
}

func fHas(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// fCanary is a loopback TCP listener that counts connections and keeps the first bytes each one sent.
type fCanary struct {
	ln   net.Listener
	hits atomic.Int32
	mu   sync.Mutex
	data strings.Builder
}

func newFCanary(t *testing.T, network, addr string) *fCanary {
	t.Helper()
	ln, err := net.Listen(network, addr)
	if err != nil {
		return nil
	}
	c := &fCanary{ln: ln}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			c.hits.Add(1)
			go func() {
				defer func() { _ = conn.Close() }()
				_ = conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
				buf := make([]byte, 4096)
				n, _ := conn.Read(buf)
				c.mu.Lock()
				c.data.Write(buf[:n])
				c.mu.Unlock()
			}()
		}
	}()
	return c
}

func (c *fCanary) port() string { _, p, _ := net.SplitHostPort(c.ln.Addr().String()); return p }

func (c *fCanary) seen() string { c.mu.Lock(); defer c.mu.Unlock(); return c.data.String() }

// fHTTP is a plain loopback HTTP server that answers every path with body and records Authorization headers.
type fHTTP struct {
	*httptest.Server
	mu    sync.Mutex
	auths []string
}

func newFHTTP(t *testing.T, body string) *fHTTP {
	t.Helper()
	h := &fHTTP{}
	h.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.mu.Lock()
		if a := r.Header.Get("Authorization"); a != "" {
			h.auths = append(h.auths, a)
		}
		h.mu.Unlock()
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(h.Close)
	return h
}

func (h *fHTTP) auth() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.auths...)
}
