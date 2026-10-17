package main

// json_envelope_test.go: the --json contract of every sentry subcommand.
//
// Purpose: v1.4 mode keeps the pre-contract bytes (goldens captured from the
//   code before P7-SURF-31) for the commands that printed JSON before; v1.5
//   mode prints exactly one v1 envelope accepted by output.CheckEnvelope and
//   exits with the status of its error class. login and logout printed no JSON
//   before, so their envelope is the same in both modes.
// Inputs: the fixture API of json_harness_test.go (httptest, no network).
// Outputs: test results. `go test ./cmd/sentry -run TestJSONEnvelope -update`
//   rewrites testdata/json/v15/*.golden.json (never the v14 goldens).
// Constraints: all keys, ids and URLs are synthetic.

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/compat/compattest"
	"github.com/nself-org/cli/sdk/go/v2/output"
)

var update = flag.Bool("update", false, "rewrite testdata/json/v15/*.golden.json")

// jsonCase is one invocation. legacy: a v1.4 golden exists (the command printed
// JSON before). always: the command printed no JSON before. errCode: the error code expected in v1.5 mode ("" = success).
type jsonCase struct {
	name    string
	cmd     string // envelope command
	args    []string
	legacy  bool
	always  bool // no JSON before: the envelope in both modes
	errCode string
	class   string
	exit    int    // v1.5 exit status
	stderr  string // the only text allowed on stderr ("" = none)
}

func jsonCases(url string) []jsonCase {
	api := []string{"--api-url", url, "--api-key", fixtureKey, "--json"}
	bad := []string{"--api-url", url, "--api-key", "nsk_wrongkey9999", "--json"}
	with := func(a ...string) []string { return append(append([]string{}, a...), api...) }
	return []jsonCase{
		{name: "monitors-list", cmd: "sentry monitors list", args: with("monitors", "list"), legacy: true},
		{name: "monitors-add", cmd: "sentry monitors add", args: with("monitors", "add", "--name", "api", "--url", "https://api.example.test", "--interval", "60s"), legacy: true},
		{name: "monitors-rm", cmd: "sentry monitors rm", args: with("monitors", "rm", "m1"), legacy: true},
		{name: "monitors-pause", cmd: "sentry monitors pause", args: with("monitors", "pause", "m1"), legacy: true},
		{name: "monitors-resume", cmd: "sentry monitors resume", args: with("monitors", "resume", "m1"), legacy: true},
		{name: "incidents-list", cmd: "sentry incidents list", args: with("incidents", "list", "--status", "open"), legacy: true},
		{name: "incidents-ack", cmd: "sentry incidents ack", args: with("incidents", "ack", "i1"), legacy: true},
		{name: "incidents-resolve", cmd: "sentry incidents resolve", args: with("incidents", "resolve", "i1"), legacy: true},
		{name: "status-pages-list", cmd: "sentry status-pages list", args: with("status-pages", "list"), legacy: true},
		{name: "status-pages-create", cmd: "sentry status-pages create", args: with("status-pages", "create", "--name", "Public", "--slug", "public"), legacy: true},
		{name: "alerts-channels", cmd: "sentry alerts channels", args: with("alerts", "channels"), legacy: true},
		{name: "alerts-test", cmd: "sentry alerts test", args: with("alerts", "test", "c1"), legacy: true},
		{name: "whoami", cmd: "sentry whoami", args: with("whoami"), legacy: true},
		{name: "status-ok", cmd: "sentry status", args: []string{"status", "--json", "--url", url + "/ok.json"}, legacy: true},
		{name: "status-degraded", cmd: "sentry status", args: []string{"status", "--json", "--url", url + "/degraded.json"}, legacy: true, exit: 1},
		{name: "monitors-list-unauth", cmd: "sentry monitors list", args: append([]string{"monitors", "list"}, bad...), legacy: true, errCode: "E780", class: "auth", exit: 3},
		{name: "monitors-rm-500", cmd: "sentry monitors rm", args: with("monitors", "rm", "boom"), legacy: true, errCode: "E782", class: "infra", exit: 2},
		{name: "monitors-rm-404", cmd: "sentry monitors rm", args: with("monitors", "rm", "gone"), errCode: "E782", class: "user", exit: 1},
		{name: "monitors-add-bad-kind", cmd: "sentry monitors add", args: with("monitors", "add", "--url", "x", "--kind", "smtp"), errCode: "E401", class: "user", exit: 1},
		{name: "monitors-list-unknown-flag", cmd: "sentry monitors list", args: with("monitors", "list", "--bogus"), errCode: "E401", class: "user", exit: 1},
		{name: "monitors-list-unreachable", cmd: "sentry monitors list", args: []string{"monitors", "list", "--api-url", "http://127.0.0.1:1", "--api-key", fixtureKey, "--json"}, errCode: "E782", class: "infra", exit: 2},
		{name: "status-down", cmd: "sentry status", args: []string{"status", "--json", "--url", url + "/down.json"}, errCode: "E783", class: "infra", exit: 2},
		{name: "login", always: true, cmd: "sentry login", args: []string{"login", "--api-url", url, "--api-key", fixtureKey, "--json"}},
		{name: "login-rejected", always: true, cmd: "sentry login", args: []string{"login", "--api-url", url, "--api-key", "nsk_wrongkey9999", "--json"}, errCode: "E780", class: "auth", exit: 3},
		{name: "login-short-key", always: true, cmd: "sentry login", args: []string{"login", "--api-url", url, "--api-key", "nsk_x", "--json"}, errCode: "E401", class: "user", exit: 1},
		{name: "login-no-key", always: true, cmd: "sentry login", args: []string{"login", "--api-url", url, "--json"}, stderr: "Paste your ɳSentry API key (nsk_...): ", errCode: "E401", class: "user", exit: 1},
		{name: "login-unknown-flag", always: true, cmd: "sentry login", args: []string{"login", "--bogus", "--json"}, errCode: "E401", class: "user", exit: 1},
		{name: "logout-unknown-flag", always: true, cmd: "sentry logout", args: []string{"logout", "--bogus=SECRET", "--json"}, errCode: "E401", class: "user", exit: 1},
		{name: "logout", always: true, cmd: "sentry logout", args: []string{"logout", "--json"}},
	}
}

// isolate gives one case a clean HOME and no ambient credentials.
func isolate(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	// No key on stdin either: a login without a key must hit EOF, not wait.
	null, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	oldIn := os.Stdin
	os.Stdin = null
	t.Cleanup(func() { os.Stdin = oldIn; _ = null.Close() })
	for _, k := range []string{"NSELF_SENTRY_API_URL", "NSELF_SENTRY_API_KEY", "NSENTRY_PRIVATE_TOKEN", "STATUS_PAGE_PRIVATE_TOKEN"} {
		t.Setenv(k, "")
	}
}

// golden14 renders a run the way the v14 goldens were captured.
func golden14(r runResult, url string) string {
	return "--- stdout\n" + r.stdout + "--- stderr\n" + strings.ReplaceAll(r.stderr, url, "SERVER") +
		"--- exit\n" + string(rune('0'+r.code)) + "\n"
}

func TestJSONEnvelope(t *testing.T) {
	srv := fixtureAPI(t)
	compattest.Both(t, func(t *testing.T) {
		v15 := strings.HasSuffix(t.Name(), "v1.5")
		for _, c := range jsonCases(srv.URL) {
			t.Run(c.name, func(t *testing.T) {
				isolate(t)
				r := captureRun(t, c.args)
				if !v15 && !c.always {
					if !c.legacy {
						// A failure the v1.4 code reported as plain text; the
						// bytes are not pinned, but it must still fail.
						if r.code == 0 || strings.Contains(r.stdout, "schema_version") {
							t.Fatalf("v1.4 must fail without an envelope: code=%d stdout=%s", r.code, r.stdout)
						}
						return
					}
					want, err := os.ReadFile(filepath.Join("..", "..", "testdata", "json", "v14", c.name+".golden"))
					if err != nil {
						t.Fatalf("missing v14 golden: %v", err)
					}
					if got := golden14(r, srv.URL); got != string(want) {
						t.Fatalf("v1.4 bytes changed\n--- got\n%s\n--- want\n%s", got, want)
					}
					return
				}
				checkEnvelope(t, c, r)
			})
		}
	})
}

// checkEnvelope asserts one run printed exactly one valid envelope.
func checkEnvelope(t *testing.T, c jsonCase, r runResult) {
	t.Helper()
	if r.code != c.exit {
		t.Fatalf("exit = %d, want %d (stdout=%s stderr=%s)", r.code, c.exit, r.stdout, r.stderr)
	}
	if r.stderr != c.stderr {
		t.Fatalf("stderr must be %q in JSON mode, got %q", c.stderr, r.stderr)
	}
	out := []byte(r.stdout)
	if err := output.CheckEnvelope(out); err != nil {
		t.Fatalf("CheckEnvelope: %v\n%s", err, out)
	}
	if bytes.Count(out, []byte(`"schema_version"`)) != 1 {
		t.Fatalf("want exactly one envelope:\n%s", out)
	}
	var doc struct {
		Command string `json:"command"`
		Error   *struct {
			Code     string `json:"code"`
			Class    string `json:"class"`
			ExitCode int    `json:"exit_code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Command != c.cmd {
		t.Errorf("command = %q, want %q", doc.Command, c.cmd)
	}
	if c.errCode == "" {
		if doc.Error != nil {
			t.Fatalf("unexpected error envelope: %s", out)
		}
	} else if doc.Error == nil || doc.Error.Code != c.errCode || doc.Error.Class != c.class || doc.Error.ExitCode != c.exit {
		t.Fatalf("error = %+v, want %s/%s/%d\n%s", doc.Error, c.errCode, c.class, c.exit, out)
	}
	for _, leak := range []string{fixtureKey, "nsk_wrongkey9999", "127.0.0.1", "database exploded", "SECRET"} {
		if strings.Contains(r.stdout, leak) {
			t.Errorf("envelope leaks %q:\n%s", leak, out)
		}
	}
	path := filepath.Join("..", "..", "testdata", "json", "v15", c.name+".golden.json")
	if *update {
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, out, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("missing v15 golden (run with -update): %v", err)
	}
	if !bytes.Equal(out, want) {
		t.Errorf("envelope differs from golden\n--- got\n%s\n--- want\n%s", out, want)
	}
}
