package main

// json_envelope_test.go: the --json contract of 'sentry-server provision'.
//
// Purpose: provision printed no JSON before, so its envelope is the --json
//   output in v1.4 and v1.5 mode alike. A dry run proves the success envelope
//   and that progress lines stay off stdout; a missing argument and a failing
//   step prove the error envelopes and their exit status.
// Inputs: --dry-run (no terraform, ssh or nself is started), a temp HOME and
//   working directory.
// Outputs: test results.
// Constraints: no network, no cloud credentials.

import (
	"os"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/compat/compattest"
	"github.com/nself-org/cli/sdk/go/v2/output"
	"github.com/nself-org/nself-sentry-cli/cmd/jsonout"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// run executes the tree with args, capturing the real stdout and stderr.
func run(t *testing.T, args []string) (stdout, stderr string, code int) {
	t.Helper()
	dir := t.TempDir()
	so, _ := os.Create(dir + "/o")
	se, _ := os.Create(dir + "/e")
	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = so, se
	setup()
	code = jsonout.Execute(sentryServerCmd, args)
	os.Stdout, os.Stderr = oldOut, oldErr
	_ = so.Close()
	_ = se.Close()
	ob, _ := os.ReadFile(dir + "/o")
	eb, _ := os.ReadFile(dir + "/e")
	reset(sentryServerCmd)
	return string(ob), string(eb), code
}

func reset(c *cobra.Command) {
	c.Flags().VisitAll(func(f *pflag.Flag) { _ = f.Value.Set(f.DefValue); f.Changed = false })
	for _, s := range c.Commands() {
		reset(s)
	}
}

func TestJSONEnvelope(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		t.Chdir(t.TempDir())
		t.Setenv("NSELF_DEPLOY_KEY_PATH", "")

		out, errOut, code := run(t, []string{"provision", "demo", "--dry-run", "--json"})
		if code != 0 {
			t.Fatalf("dry run exit = %d\nstdout=%s\nstderr=%s", code, out, errOut)
		}
		if err := output.CheckEnvelope([]byte(out)); err != nil {
			t.Fatalf("CheckEnvelope: %v\n%s", err, out)
		}
		want := `{
  "schema_version": "1",
  "command": "sentry-server provision",
  "data": {
    "project": "demo",
    "dry_run": true,
    "terraform": false,
    "deployed": false
  }
}
`
		if out != want {
			t.Fatalf("envelope differs\n--- got\n%s\n--- want\n%s", out, want)
		}
		if !strings.Contains(errOut, "DRY-RUN") {
			t.Errorf("progress lines must be on stderr, got %q", errOut)
		}

		// provision printed no JSON before: a missing argument is a usage error
		// envelope in v1.4 and v1.5 mode alike.
		out, errOut, code = run(t, []string{"provision", "--json"})
		if code != 1 {
			t.Fatalf("usage exit = %d", code)
		}
		if err := output.CheckEnvelope([]byte(out)); err != nil || !strings.Contains(out, `"E401"`) {
			t.Fatalf("usage envelope: %v\n%s", err, out)
		}
		if errOut != "" {
			t.Fatalf("usage error must not print on stderr, got %q", errOut)
		}

		// A failing step: with --host the secrets push finds no .env.ops or .env in the
		// working directory and fails (class infra, exit 2).
		out, _, code = run(t, []string{"provision", "demo", "--host", "u@203.0.113.9:/srv", "--json", "--dry-run"})
		if err := output.CheckEnvelope([]byte(out)); err != nil {
			t.Fatalf("error CheckEnvelope: %v\n%s", err, out)
		}
		if !strings.Contains(out, `"E786"`) || strings.Contains(out, "203.0.113.9") {
			t.Fatalf("want E786 without the host:\n%s", out)
		}
		if code != 2 {
			t.Fatalf("step failure exit = %d, want 2", code)
		}
	})
}
