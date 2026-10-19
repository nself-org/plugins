package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const credSecret = "JITCONFIG-fake-synthetic-value-0001"

func childAgent(t *testing.T, stdin string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = append(os.Environ(), "NSELF_AGENT_TEST_CHILD=1", "NSELF_CI_AGENT_HOME=/agent-state-must-not-leak", "SOME_SECRET_TOKEN=agent-side-value")
	cmd.Stdin = strings.NewReader(stdin)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// TestCredWrapper proves the acceptance line: cred-wrapper given a credential
// on stdin exposes it only to the child's environment: it is not in the
// child's argv, not in the state dir, and the agent's own environment is not
// inherited. (The job-log redaction half is session.TestLogLinesNumberedAndMasked.)
func TestCredWrapper(t *testing.T) {
	skipUnixOnly(t)
	state := useState(t)
	script := `printf 'env=%s\n' "$ACTIONS_RUNNER_INPUT_JITCONFIG"; ps -o args= -p $$; env | grep -c -e NSELF_CI_AGENT_HOME -e SOME_SECRET_TOKEN || true`
	out, err := childAgent(t, credSecret+"\n", "cred-wrapper", "github-jitconfig", "--", "sh", "-c", script)
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if lines[0] != "env="+credSecret {
		t.Fatalf("child did not get the credential in its environment:\n%s", out)
	}
	if strings.Contains(strings.Join(lines[1:len(lines)-1], "\n"), credSecret) {
		t.Fatalf("credential is in the child's argv:\n%s", out)
	}
	if lines[len(lines)-1] != "0" {
		t.Fatalf("agent environment leaked into the child:\n%s", out)
	}
	entries, _ := os.ReadDir(state)
	for _, e := range entries {
		if b, _ := os.ReadFile(filepath.Join(state, e.Name())); strings.Contains(string(b), credSecret) {
			t.Fatalf("credential written to %s", e.Name())
		}
	}
	out, err = childAgent(t, "glrt-fake-token\n", "cred-wrapper", "gitlab-runner-token", "--", "sh", "-c", `printf %s "$CI_SERVER_TOKEN"`)
	if err != nil || out != "glrt-fake-token" {
		t.Fatalf("gitlab kind: %v %q", err, out)
	}
}

func TestCredWrapperRefusals(t *testing.T) {
	skipUnixOnly(t)
	useState(t)
	for name, c := range map[string]struct {
		stdin string
		args  []string
		code  int
	}{
		"unknown kind":        {credSecret + "\n", []string{"cred-wrapper", "nope", "--", "true"}, 2},
		"no command":          {credSecret + "\n", []string{"cred-wrapper", "github-jitconfig", "--"}, 2},
		"empty credential":    {"\n", []string{"cred-wrapper", "github-jitconfig", "--", "true"}, 1},
		"closed stdin":        {"", []string{"cred-wrapper", "github-jitconfig", "--", "true"}, 1},
		"missing command":     {credSecret + "\n", []string{"cred-wrapper", "github-jitconfig", "--", "no-such-command-xyz"}, 1},
		"overlong credential": {strings.Repeat("a", 70<<10) + "\n", []string{"cred-wrapper", "github-jitconfig", "--", "true"}, 1},
	} {
		out, err := childAgent(t, c.stdin, c.args...)
		code := 0
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		}
		if code != c.code {
			t.Errorf("%s: exit %d, want %d (%s)", name, code, c.code, out)
		}
		if strings.Contains(out, credSecret) {
			t.Errorf("%s: credential echoed: %s", name, out)
		}
	}
}
