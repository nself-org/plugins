//go:build !windows

package exec

// A multi-line secret (a PEM key, wrapped base64) cannot ride a docker --env-file, whose
// format is one KEY=VALUE per line. On the hardened path it travels as `--env NAME` plus the
// runtime CLI's own environment, exactly like the baseline path; single-line values keep
// the stdin env file (D-1072).

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeRuntimeProbe records argv, stdin and the value of PEM in the runtime CLI's environment.
func fakeRuntimeProbe(t *testing.T) (dir string) {
	t.Helper()
	dir = t.TempDir()
	script := "#!/bin/sh\nd=" + dir + "\necho \"$*\" >> \"$d/calls\"\ncase \"$*\" in *--entrypoint*) exit 0;; esac\ncase \"$1\" in run) ;; *) exit 0;; esac\nprintenv PEM > \"$d/pem\"\ncat > \"$d/stdin\"\nexit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir
}

func TestPlannedContainerCarriesMultilineSecretOutsideEnvFile(t *testing.T) {
	const pem = "-----BEGIN KEY-----\nfake-body-3c9d\n-----END KEY-----"
	dir := fakeRuntimeProbe(t)
	spec := plannedSpec(t, "plan-pem", hardPlanner{})
	spec.Secrets = fixedSecrets{"PEM": pem, "TOKEN": "single-line-7f"}
	spec.Job.Secrets = []string{"project/PEM", "project/TOKEN"}
	res := (&Executor{}).Run(context.Background(), spec)
	if res.Err != nil {
		t.Fatalf("multi-line secret refused: %v", res.Err)
	}
	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	calls, stdin, host := read("calls"), read("stdin"), strings.TrimSuffix(read("pem"), "\n")
	if strings.Contains(calls, "fake-body-3c9d") || strings.Contains(calls, "single-line-7f") {
		t.Fatalf("secret value on argv: %q", calls)
	}
	if !strings.Contains(calls, "--env PEM ") || strings.Contains(calls, "--env TOKEN") {
		t.Fatalf("only the multi-line secret passes by name: %q", calls)
	}
	if host != pem {
		t.Fatalf("runtime CLI env PEM = %q", host)
	}
	if strings.Contains(stdin, "fake-body-3c9d") || !strings.Contains(stdin, "TOKEN=single-line-7f\n") {
		t.Fatalf("env file placement wrong: %q", stdin)
	}
}

func TestPlannedContainerStillRefusesMultilinePlainEnv(t *testing.T) {
	fakeRuntimeProbe(t)
	spec := plannedSpec(t, "plan-nl", hardPlanner{})
	spec.Job.Env = map[string]string{"PLAIN": "a\nb"}
	if res := (&Executor{}).Run(context.Background(), spec); res.Err == nil || !strings.Contains(res.Err.Error(), "E612") {
		t.Fatalf("want E612, got %v", res.Err)
	}
}

func TestCheckerRefusesUndeclaredPassThroughEnv(t *testing.T) {
	run := ContainerRun{Name: "nself-ci-x", Image: "alpine:3.19", Workspace: "/w", AttemptID: "x", EnvFile: envFileStdin, PassEnv: []string{"PEM"}}
	for name, tc := range map[string]struct {
		env string
		ok  bool
	}{"declared": {"PEM", true}, "undeclared": {"OTHER", false}, "with value": {"PEM=x", false}} {
		cmd, _ := hardPlanner{}.Plan(run)
		cmd.Args = append([]string{"run", "--env", tc.env}, cmd.Args[1:]...)
		if err := checkContainerCommand(cmd, run); (err == nil) != tc.ok {
			t.Fatalf("%s: %v", name, err)
		}
	}
}
