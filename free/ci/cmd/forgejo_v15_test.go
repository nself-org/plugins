package main

// Purpose: tests of `nself ci forgejo` in v1.4 and v1.5 mode (P7-HOST-12): the v1.4 output equals the
// CANON-09 golden, the V15 alias prints one deprecation line per process and then the server and API
// check (no runner line), and the URL comes from --url, then the policy key, then the default.
// Inputs:  httptest servers on loopback, a stub docker on PATH for the v1.4 half.
// Outputs: pass or fail; no network beyond loopback.
// Constraints: the Ticket's verify line names TestForgejoAlias and requires its PASS.

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/forgejo"
)

func forgejoServer(t *testing.T, healthy bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/-/health" {
			_, _ = w.Write([]byte(map[bool]string{true: `{"healthy": true}`, false: `{"healthy": false}`}[healthy]))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// captureStd runs fn with stdout and stderr redirected and returns both.
func captureStd(t *testing.T, fn func()) (string, string) {
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

// TestForgejoAlias proves acceptance line 4: v1.4 `nself-ci forgejo` output equals the CANON-09 golden;
// the V15 `nself ci forgejo` prints one deprecation line naming `nself ci doctor --providers` and removal
// v1.6, then the server and API check result.
func TestForgejoAlias(t *testing.T) {
	srv := forgejoServer(t, true)
	dir := t.TempDir()
	script := "#!/bin/sh\n[ \"$1\" = inspect ] && echo running\nexit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+"/usr/bin:/bin")
	t.Setenv("NSELF_FORGEJO_ADMIN_USER", "")
	t.Setenv("NSELF_FORGEJO_ADMIN_PASSWORD", "")

	// v1.4: the frozen table entry, byte for byte as the CANON-09 port prints it.
	t.Setenv("NSELF_V15", "")
	golden := "\n→ Forgejo CI stack (ops profile)\n" +
		"  ✓ Forgejo server   " + srv.URL + "   healthy\n" +
		"  ✓ Forgejo runner   container=my_runner   state=running\n" +
		"    API auth         n/a (set NSELF_FORGEJO_ADMIN_USER + NSELF_FORGEJO_ADMIN_PASSWORD)\n\n" +
		"✓ Forgejo CI stack is healthy.\n"
	out, errOut := captureStd(t, func() {
		if code := run([]string{"forgejo", "--url", srv.URL, "--runner", "my_runner"}); code != 0 {
			t.Errorf("v1.4 exit %d", code)
		}
	})
	if out != golden || errOut != "" {
		t.Fatalf("v1.4 output changed:\n%q\nwant\n%q\nstderr %q", out, golden, errOut)
	}
	if _, v14 := subcommands["forgejo"]; !v14 {
		t.Fatal("the v1.4 entry must stay registered")
	}
	e, ok := v15Subcommands["forgejo"]
	if !ok || !e.V15Only || e.Flags == nil || e.Handler == nil {
		t.Fatalf("the V15 entry: %+v registered=%v", e, ok)
	}

	// V15: one deprecation line per process, then the check; no runner line.
	forgejoDeprecationOnce = sync.Once{}
	var stdout, stderr bytes.Buffer
	env := forgejoV15Env{Out: &stdout, Err: &stderr, Getenv: func(string) string { return "" }, PolicyURL: func(context.Context) (string, error) { return forgejo.DefaultURL, nil }}
	if code := runForgejoV15([]string{"--url", srv.URL}, env); code != 0 {
		t.Fatalf("V15 exit %d: %s %s", code, stdout.String(), stderr.String())
	}
	if code := runForgejoV15([]string{"--url", srv.URL}, env); code != 0 {
		t.Fatalf("second run exit %d", code)
	}
	dep := forgejo.Deprecation
	if strings.Count(stderr.String(), dep) != 1 || !strings.Contains(dep, "nself ci doctor --providers") || !strings.Contains(dep, "v1.6") {
		t.Fatalf("one deprecation line naming the doctor and v1.6, got %q", stderr.String())
	}
	if strings.Count(stdout.String(), "✓ Forgejo server   "+srv.URL+"   healthy") != 2 || strings.Contains(stdout.String(), "runner") || strings.Contains(stdout.String(), "container") {
		t.Fatalf("V15 output: %s", stdout.String())
	}

	// The URL: flag, then the policy key, then the default; an unreadable policy warns and falls back.
	stdout.Reset()
	stderr.Reset()
	env.PolicyURL = func(context.Context) (string, error) { return srv.URL, nil }
	code := runForgejoV15([]string{"--json"}, env)
	var got struct {
		Data forgejo.Result `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); code != 0 || err != nil || got.Data.URL != srv.URL || !got.Data.Healthy {
		t.Fatalf("the policy key's URL: exit %d, %v: %s", code, err, stdout.String())
	}
	sick := forgejoServer(t, false)
	if code := runForgejoV15([]string{"--url", sick.URL}, env); code != 1 {
		t.Fatalf("an unhealthy server must exit 1, got %d", code)
	}
	stderr.Reset()
	env.PolicyURL = func(context.Context) (string, error) { return "", io.ErrUnexpectedEOF }
	runForgejoV15([]string{"--url", ""}, env)
	if !strings.Contains(stderr.String(), "using "+forgejo.DefaultURL) {
		t.Fatalf("an unreadable policy must warn and use the default: %q", stderr.String())
	}
	if code := runForgejoV15([]string{"extra"}, env); code != 2 {
		t.Fatalf("a stray argument is a usage error, got %d", code)
	}
}
