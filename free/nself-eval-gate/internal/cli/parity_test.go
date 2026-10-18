package cli

// Purpose: parity of the nself-eval surface with core `nself ci eval [gate]`.
//   Every case in testdata/eval-parity/cases.json was run against a core
//   v1.4 build (cli origin/main 8302caa0) with the routes in routes.json; the
//   captured stdout, stderr and exit code are the goldens compared here.
// Inputs:  testdata/eval-parity/{cases.json,routes.json,<case>.{stdout,stderr,exit}}.
// Outputs: pass/fail per case; TestEvalParity is the named verify test.
// Constraints: no network beyond a loopback httptest server; no skips.

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const parityDir = "../../testdata/eval-parity"

type parityCase struct {
	Name     string            `json:"name"`
	Args     []string          `json:"args"`
	Files    map[string]string `json:"files"`
	Artifact bool              `json:"artifact"`
}

type parityRoute struct {
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body"`
}

func readGolden(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(parityDir, name))
	if err != nil {
		t.Fatalf("read golden %s: %v", name, err)
	}
	return string(b)
}

// parityServer serves routes.json the way the capture server did: POST
// /eval/run and /eval/validate are keyed by suite slug / empty content.
func parityServer(t *testing.T) *httptest.Server {
	t.Helper()
	var routes map[string]parityRoute
	if err := json.Unmarshal([]byte(readGolden(t, "routes.json")), &routes); err != nil {
		t.Fatalf("routes.json: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path
		if key == "POST /eval/run" || key == "POST /eval/validate" {
			b, _ := io.ReadAll(r.Body)
			var m map[string]string
			_ = json.Unmarshal(b, &m)
			switch {
			case key == "POST /eval/run":
				key += "#" + m["suite_slug"]
			case m["content"] == "":
				key += "#empty"
			default:
				key += "#content"
			}
		}
		rt, ok := routes[key]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(rt.Status)
		_, _ = w.Write(rt.Body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestEvalParity replays every captured core case against Run.
func TestEvalParity(t *testing.T) {
	old := pollInterval
	pollInterval = 5 * time.Millisecond
	t.Cleanup(func() { pollInterval = old })

	var cases []parityCase
	if err := json.Unmarshal([]byte(readGolden(t, "cases.json")), &cases); err != nil {
		t.Fatalf("cases.json: %v", err)
	}
	if len(cases) < 30 {
		t.Fatalf("cases.json has %d cases; the captured set has at least 30", len(cases))
	}
	srv := parityServer(t)
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			repo := t.TempDir()
			for rel, body := range c.Files {
				p := filepath.Join(repo, rel)
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			args := make([]string, len(c.Args))
			for i, a := range c.Args {
				a = strings.ReplaceAll(a, "{{URL}}", srv.URL)
				args[i] = strings.ReplaceAll(a, "{{REPO}}", repo)
			}
			var out, errb bytes.Buffer
			code := Run(args, &out, &errb)
			norm := func(s string) string {
				return strings.ReplaceAll(strings.ReplaceAll(s, srv.URL, "{{URL}}"), repo, "{{REPO}}")
			}
			wantCode, _ := strconv.Atoi(strings.TrimSpace(readGolden(t, c.Name+".exit")))
			if code != wantCode {
				t.Errorf("exit = %d, want %d", code, wantCode)
			}
			if got, want := norm(out.String()), readGolden(t, c.Name+".stdout"); got != want {
				t.Errorf("stdout differs\n got: %q\nwant: %q", got, want)
			}
			if got, want := norm(errb.String()), readGolden(t, c.Name+".stderr"); got != want {
				t.Errorf("stderr differs\n got: %q\nwant: %q", got, want)
			}
			if c.Artifact {
				got, err := os.ReadFile(filepath.Join(repo, ".nself-ci", "artifacts", "eval-results.json"))
				if err != nil {
					t.Fatalf("artifact: %v", err)
				}
				if string(got) != readGolden(t, c.Name+".artifact.json") {
					t.Errorf("artifact differs from golden")
				}
			}
		})
	}
}
