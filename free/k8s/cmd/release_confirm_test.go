// Purpose: tests for the D-0376 tail of P7-DEPL-07: a release name starting
// with "-" is refused in every mode of every command (K8S-02), and the
// uninstall confirmation reads at most maxConfirmBytes (K8S-04).
package main

import (
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestDashReleaseRefusedInEveryMode(t *testing.T) {
	dir := newProject(t)
	writeGenerated(t, dir)
	for _, name := range []string{"-", "-x", "--dry-run", "--json", "-nself"} {
		for _, js := range []bool{false, true} {
			cases := [][]string{
				{"status", "--release=" + name},
				{"install", "--domain", "x.test", "--project-dir", dir, "--release=" + name},
				{"upgrade", "--project-dir", dir, "--release=" + name},
				{"uninstall", "--yes", "--release=" + name},
			}
			for _, args := range cases {
				if js {
					args = append(args, "--json")
				}
				r := runCLI(t, cliRun{Helm: "ok"}, args...)
				if calls := argvCalls(t, r.ArgvFile); calls != nil {
					t.Errorf("%v ran helm: %v", args, calls)
				}
				if js {
					wantFailure(t, r, "k8s "+args[0], "E401", "user")
				} else if r.Exit != 1 || !strings.Contains(r.Stderr, "--release must be a helm release name") {
					t.Errorf("%v: exit %d stderr %q", args, r.Exit, r.Stderr)
				}
			}
		}
	}
}

func TestCheckReleaseModes(t *testing.T) {
	cases := []struct {
		name         string
		json, always bool
		wantErr      bool
	}{
		{"nself", false, false, false},
		{"nself", true, true, false},
		{"Upper", false, false, false}, // human install/upgrade/status: helm judges it
		{"Upper", true, false, true},
		{"Upper", false, true, true},
		{"-x", false, false, true}, // a flag to helm: refused everywhere
		{"--dry-run", false, false, true},
		{"-x", true, false, true},
		{"-", false, true, true},
	}
	for _, c := range cases {
		// usageFail under --json writes an envelope through cmd.
		cmd := &cobra.Command{}
		cmd.SetOut(io.Discard)
		r := &invocation{cmd: cmd, sub: "status", json: c.json}
		err := r.checkRelease(c.name, c.always)
		if (err != nil) != c.wantErr {
			t.Errorf("checkRelease(%q, json=%v, always=%v) = %v, want error %v", c.name, c.json, c.always, err, c.wantErr)
		}
	}
}

// endless yields 'a' with no newline and counts what was read. It stops at
// 1 MiB so an unbounded read fails the test instead of hanging it.
type endless struct{ n int }

func (e *endless) Read(p []byte) (int, error) {
	if e.n >= 1<<20 {
		return 0, io.EOF
	}
	for i := range p {
		p[i] = 'a'
	}
	e.n += len(p)
	return len(p), nil
}

func TestConfirmReleaseBoundedRead(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"exact", "nself\n", true},
		{"no newline", "nself", true},
		{"spaces", "  nself  \n", true},
		{"other", "nselfx\n", false},
		{"long line", strings.Repeat("a", 2*maxConfirmBytes) + "\n", false},
		{"name after limit", strings.Repeat(" ", maxConfirmBytes) + "nself\n", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := confirmRelease(strings.NewReader(c.in), io.Discard, "nself"); got != c.want {
				t.Errorf("confirmRelease(%q) = %v, want %v", c.name, got, c.want)
			}
		})
	}
	src := &endless{}
	if confirmRelease(src, io.Discard, "nself") {
		t.Error("an endless stdin must not confirm")
	}
	if src.n > maxConfirmBytes {
		t.Errorf("read %d bytes from an endless stdin, want at most %d", src.n, maxConfirmBytes)
	}
}

func TestConfirmPromptSaysClaimsAreKept(t *testing.T) {
	var out strings.Builder
	confirmRelease(strings.NewReader("\n"), &out, "nself")
	if !strings.Contains(out.String(), "PersistentVolumeClaims are kept") {
		t.Errorf("prompt = %q", out.String())
	}
}
