package forgejo

// Purpose: test the Forgejo health command against an httptest server and a
// recording stub docker on PATH; no real Docker, no network beyond loopback.
// Outputs: assertions on stdout, stderr and the recorded docker argv.

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// run executes Run with stdout and stderr captured.
func run(t *testing.T, cfg Config) (string, string) {
	t.Helper()
	ro, wo, _ := os.Pipe()
	re, we, _ := os.Pipe()
	so, se := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = wo, we
	err := Run(cfg)
	os.Stdout, os.Stderr = so, se
	_ = wo.Close()
	_ = we.Close()
	if err != nil {
		t.Fatalf("Run returned %v (core never failed)", err)
	}
	bo, _ := io.ReadAll(ro)
	be, _ := io.ReadAll(re)
	return string(bo), string(be)
}

// stubDocker puts a docker on PATH that logs argv and answers inspect with state.
func stubDocker(t *testing.T, state string, ok bool) string {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "docker.log")
	exit := "0"
	if !ok {
		exit = "1"
	}
	script := "#!/bin/sh\necho \"$*\" >> '" + log + "'\n[ \"$1\" = inspect ] && echo " + state + "\nexit " + exit + "\n"
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+"/usr/bin:/bin")
	return log
}

// TestHealthyStack needs only the server; Docker must never be invoked.
func TestHealthyStack(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"healthy": true}`))
	}))
	defer srv.Close()
	log := stubDocker(t, "", false)
	out, errOut := run(t, Config{URL: srv.URL + "/"})
	if !strings.Contains(out, "Forgejo CI stack is healthy.") || strings.Contains(out, "runner") || errOut != "" {
		t.Fatalf("healthy server without runner: stdout=%q stderr=%q", out, errOut)
	}
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Fatalf("Docker was invoked: %v", err)
	}
}

func TestUnreachable(t *testing.T) {
	log := stubDocker(t, "", false)
	out, errOut := run(t, Config{URL: "http://127.0.0.1:1"})
	if !strings.Contains(out, "unreachable") || !strings.Contains(errOut, "Forgejo server unreachable") || strings.Contains(out+errOut, "runner") {
		t.Fatalf("unreachable server: stdout=%q stderr=%q", out, errOut)
	}
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Fatalf("Docker was invoked: %v", err)
	}
}

// TestAPIProbe: admin credentials turn the API line on.
func TestAPIProbe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/-/health" {
			_, _ = w.Write([]byte(`{"healthy": true}`))
			return
		}
		if u, p, ok := r.BasicAuth(); !ok || u != "adm" || p != "pw" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	stubDocker(t, "running", true)
	t.Setenv("NSELF_FORGEJO_ADMIN_USER", "adm")
	t.Setenv("NSELF_FORGEJO_ADMIN_PASSWORD", "pw")
	out, _ := run(t, Config{URL: srv.URL})
	if !strings.Contains(out, "API reachable (admin authenticated)") {
		t.Errorf("stdout:\n%s", out)
	}
	t.Setenv("NSELF_FORGEJO_ADMIN_PASSWORD", "wrong")
	out, _ = run(t, Config{URL: srv.URL})
	if !strings.Contains(out, "API returned HTTP 401") {
		t.Errorf("stdout:\n%s", out)
	}
}
