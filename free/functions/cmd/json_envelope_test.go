package main

// Purpose: TestJSONEnvelope, the --json contract of every functions subcommand
// in both compat modes (list, deploy, delete, invoke, logs).
//
// Inputs: the in-process harness (json_harness_test.go), goldens in
// testdata/json (v14 bytes captured from the pre-contract code, v15 and _any
// envelopes with synthetic values).
//
// Outputs: failures when a v1.5 envelope is not accepted by output.CheckEnvelope,
// when stdout carries more than the envelope, when the exit status is not the
// class status, or when the v1.4 list bytes changed.
//
// Constraints: no network beyond 127.0.0.1 test servers. list is the only
// command that printed JSON before the contract (gated); every other command
// prints the envelope in both modes.

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/compat"
	"github.com/nself-org/cli/sdk/go/v2/compat/compattest"
	"github.com/nself-org/cli/sdk/go/v2/output"
)

const fakeAuth = "fake-bearer-for-tests"

// wantEnvelope checks res is exactly one valid envelope with the given exit
// status and compares it with the golden (scrubbed).
func wantEnvelope(t *testing.T, e *testEnv, res cliResult, exit int, name string) {
	t.Helper()
	if res.Exit != exit {
		t.Errorf("exit = %d, want %d (stderr %q)", res.Exit, exit, res.Stderr)
	}
	if err := output.CheckEnvelope([]byte(res.Stdout)); err != nil {
		t.Fatalf("stdout is not one v1 envelope: %v\n%s", err, res.Stdout)
	}
	if strings.Contains(res.Stdout+res.Stderr, fakeAuth) {
		t.Errorf("the auth token leaked into the output")
	}
	golden(t, name, e.scrub(res.Stdout))
}

func mkFns(t *testing.T, e *testEnv, names ...string) {
	t.Helper()
	for _, n := range names {
		if err := os.MkdirAll(filepath.Join(e.Dir, "functions", n), 0o750); err != nil {
			t.Fatal(err)
		}
	}
}

func TestJSONEnvelope(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		t.Run("list", testListJSON)
		t.Run("deploy", testDeployJSON)
		t.Run("delete", testDeleteJSON)
		t.Run("invoke", testInvokeJSON)
		t.Run("logs", testLogsJSON)
		t.Run("usage", testUsageJSON)
	})
}

// testListJSON: the one pre-contract JSON command. v1.4 keeps the old bytes
// (array, or plain text when nothing is listed); v1.5 prints the envelope.
func testListJSON(t *testing.T) {
	for _, c := range []struct{ name, golden string }{
		{"two", "two"}, {"empty", "empty"}, {"nodir", "nodir"},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := newTestEnv(t)
			switch c.name {
			case "two":
				mkFns(t, e, "fn-a", "fn-b")
				if err := os.WriteFile(filepath.Join(e.Dir, "functions", "notes.txt"), nil, 0o640); err != nil {
					t.Fatal(err)
				}
			case "empty":
				mkFns(t, e)
				_ = os.MkdirAll(filepath.Join(e.Dir, "functions"), 0o750)
			}
			res := runCLI(t, "list", "--json")
			if compat.V15() {
				wantEnvelope(t, e, res, 0, "list_v15_"+c.golden)
				return
			}
			if res.Exit != 0 {
				t.Fatalf("exit = %d, stderr %q", res.Exit, res.Stderr)
			}
			golden(t, "list_v14_"+c.golden, e.scrub(res.Stdout))
		})
	}
	t.Run("human", func(t *testing.T) {
		e := newTestEnv(t)
		mkFns(t, e, "fn-a")
		res := runCLI(t, "list")
		if res.Exit != 0 || !strings.HasPrefix(res.Stdout, "NAME ") || strings.Contains(res.Stdout, "schema_version") {
			t.Errorf("human list changed: exit %d, %q", res.Exit, res.Stdout)
		}
	})
}

func testDeployJSON(t *testing.T) {
	e := newTestEnv(t)
	src := filepath.Join(e.Dir, "hello.ts")
	if err := os.WriteFile(src, []byte("export default () => new Response('hi')"), 0o640); err != nil {
		t.Fatal(err)
	}
	res := runCLI(t, "deploy", "hello.ts", "--env", "GREETING=hi", "--json")
	wantEnvelope(t, e, res, 0, "deploy_ok_any")
	if _, err := os.Stat(filepath.Join(e.Dir, "functions", "hello", "hello.ts")); err != nil {
		t.Errorf("function not copied: %v", err)
	}
	if !strings.Contains(res.Stderr, "deployed to") {
		t.Errorf("human text must go to stderr in JSON mode, got %q", res.Stderr)
	}
	wantEnvelope(t, e, runCLI(t, "deploy", "missing.ts", "--json"), 1, "deploy_no_source_any")
	wantEnvelope(t, e, runCLI(t, "deploy", "hello.ts", "--name", "Bad_Name", "--json"), 1, "deploy_bad_name_any")
	human := runCLI(t, "deploy", "hello.ts")
	if human.Exit != 0 || !strings.HasPrefix(human.Stdout, `Function "hello" deployed`) {
		t.Errorf("human deploy changed: %d %q", human.Exit, human.Stdout)
	}
}

func testDeleteJSON(t *testing.T) {
	e := newTestEnv(t)
	mkFns(t, e, "gone")
	wantEnvelope(t, e, runCLI(t, "delete", "gone", "--json"), output.ExitDestructiveBlocked, "delete_no_confirm_any")
	if _, err := os.Stat(filepath.Join(e.Dir, "functions", "gone")); err != nil {
		t.Fatalf("refused delete removed the function: %v", err)
	}
	wantEnvelope(t, e, runCLI(t, "delete", "gone", "--confirm", "--json"), 0, "delete_ok_any")
	wantEnvelope(t, e, runCLI(t, "delete", "gone", "--confirm", "--json"), 1, "delete_not_found_any")
	res := runCLI(t, "delete", "gone")
	if res.Exit != 1 || !strings.Contains(res.Stderr, "--confirm is required") || res.Stdout != "" {
		t.Errorf("human refusal changed: %d %q %q", res.Exit, res.Stdout, res.Stderr)
	}
}

func testInvokeJSON(t *testing.T) {
	var gotBody, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody, gotAuth = string(b), r.Header.Get("Authorization")
		switch {
		case strings.HasSuffix(r.URL.Path, "/boom"):
			http.Error(w, "boom secret detail", http.StatusInternalServerError)
		case strings.HasSuffix(r.URL.Path, "/nope"):
			http.Error(w, "nope", http.StatusNotFound)
		case strings.HasSuffix(r.URL.Path, "/text"):
			_, _ = io.WriteString(w, "plain text")
		default:
			_, _ = io.WriteString(w, `{"greeting":"hello"}`)
		}
	}))
	defer srv.Close()
	e := newTestEnv(t)
	u, _ := url.Parse(srv.URL)
	t.Setenv("FUNCTIONS_PORT", u.Port())
	e.Port = u.Port()

	res := runCLI(t, "invoke", "hello", "--payload", `{"n":1}`, "--auth", fakeAuth, "--json")
	wantEnvelope(t, e, res, 0, "invoke_ok_any")
	if gotBody != `{"n":1}` || gotAuth != "Bearer "+fakeAuth {
		t.Errorf("request not forwarded: %q %q", gotBody, gotAuth)
	}
	wantEnvelope(t, e, runCLI(t, "invoke", "text", "--method", "get", "--json"), 0, "invoke_text_any")
	wantEnvelope(t, e, runCLI(t, "invoke", "nope", "--json"), 1, "invoke_http_404_any")
	boom := runCLI(t, "invoke", "boom", "--json")
	wantEnvelope(t, e, boom, 2, "invoke_http_500_any")
	if strings.Contains(boom.Stdout, "secret detail") {
		t.Errorf("function body leaked into the error envelope")
	}
	wantEnvelope(t, e, runCLI(t, "invoke", "hello", "--method", "bad method", "--json"), 1, "invoke_bad_method_any")
	srv.Close()
	wantEnvelope(t, e, runCLI(t, "invoke", "hello", "--json"), 2, "invoke_down_any")
	human := runCLI(t, "invoke", "hello")
	if human.Exit != 1 || !strings.Contains(human.Stderr, "invoking hello") || human.Stdout != "" {
		t.Errorf("human invoke failure changed: %d %q %q", human.Exit, human.Stdout, human.Stderr)
	}
}

func testLogsJSON(t *testing.T) {
	e := newTestEnv(t)
	stubDocker(t, 0)
	res := runCLI(t, "logs", "fn-a", "--json")
	wantEnvelope(t, e, res, 0, "logs_ok_any")
	if strings.Contains(res.Stdout, "unrelated") {
		t.Errorf("unrelated line not filtered out")
	}
	wantEnvelope(t, e, runCLI(t, "logs", "fn-a", "--follow", "--json"), 1, "logs_follow_any")
	human := runCLI(t, "logs", "fn-a")
	if human.Exit != 0 || !strings.Contains(human.Stdout, "GET /v1/fn-a 200") || strings.Contains(human.Stdout, "schema_version") {
		t.Errorf("human logs changed: %d %q", human.Exit, human.Stdout)
	}
	stubDocker(t, 1)
	wantEnvelope(t, e, runCLI(t, "logs", "fn-a", "--json"), 2, "logs_docker_fails_any")
}

func testUsageJSON(t *testing.T) {
	e := newTestEnv(t)
	res := runCLI(t, "list", "--bogus=SECRET", "--json")
	if compat.V15() {
		wantEnvelope(t, e, res, 1, "usage_unknown_flag_v15")
	} else {
		// list printed JSON before, but never on a usage error: v1.4 keeps the
		// plain stderr line and an empty stdout (the original behaviour).
		if res.Exit != 1 || res.Stdout != "" {
			t.Errorf("v1.4 list usage error: exit %d stdout %q, want 1 and empty", res.Exit, res.Stdout)
		}
		golden(t, "usage_unknown_flag_v14", "--- stdout\n"+res.Stdout+"--- stderr\n"+res.Stderr)
	}
	if strings.Contains(res.Stdout+res.Stderr, "SECRET") {
		t.Errorf("flag value echoed")
	}
	wantEnvelope(t, e, runCLI(t, "invoke", "--json"), 1, "usage_missing_arg_any")
	human := runCLI(t, "list", "--bogus")
	if human.Exit != 1 || human.Stdout != "" || !strings.HasPrefix(human.Stderr, "Error: unknown flag") {
		t.Errorf("human usage error changed: %d %q %q", human.Exit, human.Stdout, human.Stderr)
	}
}
