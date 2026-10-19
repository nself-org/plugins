package main

// Purpose: the job shim's request parsing, its user guard and its exec (a re-executed test binary
// becomes /usr/bin/env, so the test sees the exact environment and directory the shim hands over).
// Constraints: the real sudo path is proved by TestBootstrapDistros on a sim node.

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestJobShimRegistered(t *testing.T) {
	if _, ok := lookup("job-shim"); !ok {
		t.Fatal("job-shim is not in the agent's subcommand table")
	}
}

func TestParseShimRequest(t *testing.T) {
	good := `{"argv":["/bin/echo","a b","$(x)"],"dir":"/tmp","env":["A=1","EMPTY="]}`
	req, err := parseShimRequest([]byte(good))
	if err != nil || len(req.Argv) != 3 || req.Argv[2] != "$(x)" {
		t.Fatalf("good request: %+v %v", req, err)
	}
	for name, bad := range map[string]string{
		"empty argv":    `{"argv":[]}`,
		"empty program": `{"argv":[""]}`,
		"unknown field": `{"argv":["x"],"shell":true}`,
		"relative dir":  `{"argv":["x"],"dir":"work"}`,
		"env no equals": `{"argv":["x"],"env":["A"]}`,
		"env empty key": `{"argv":["x"],"env":["=v"]}`,
		"NUL in argv":   "{\"argv\":[\"x\\u0000\"]}",
		"not json":      `argv`,
	} {
		if _, err := parseShimRequest([]byte(bad)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestReadShimRequestLeavesTheRestForTheJob(t *testing.T) {
	in := bytes.NewBufferString("{\"argv\":[\"x\"]}\nJOB STDIN\n")
	line, err := readShimRequest(in, 1024)
	if err != nil || string(line) != `{"argv":["x"]}` || in.String() != "JOB STDIN\n" {
		t.Fatalf("line=%q err=%v rest=%q", line, err, in.String())
	}
	if _, err = readShimRequest(bytes.NewBufferString(strings.Repeat("a", 50)), 10); err == nil {
		t.Fatal("an over-long request was accepted")
	}
	if _, err = readShimRequest(bytes.NewBufferString("no newline"), 1024); err == nil {
		t.Fatal("a request without a newline was accepted")
	}
}

func TestShimUserGuard(t *testing.T) {
	for _, c := range []struct {
		euid int
		name string
		want bool
	}{
		{1001, "nself-ci-job-1", true}, {1001, "nself-ci-job-12", true}, {0, "nself-ci-job-1", false}, {1001, "nself-ci", false},
		{1001, "nself-ci-job-0", false}, {1001, "nself-ci-job-01", false}, {1001, "root", false}, {1001, "xnself-ci-job-1", false}, {1001, "nself-ci-job-1x", false},
	} {
		if got := shimUserOK(c.euid, c.name); got != c.want {
			t.Errorf("shimUserOK(%d, %q) = %v", c.euid, c.name, got)
		}
	}
}

func TestJobShimRefusesOutsideAJobUser(t *testing.T) {
	var out, errb bytes.Buffer
	code := runJobShim(context.Background(), nil, IO{Stdin: bytes.NewBufferString(`{"argv":["/bin/true"]}` + "\n"), Stdout: &out, Stderr: &errb})
	if code != 1 || !strings.Contains(errb.String(), "only as a job user") {
		t.Fatalf("code=%d err=%q", code, errb.String())
	}
	if code = runJobShim(context.Background(), []string{"x"}, IO{Stdin: bytes.NewBuffer(nil), Stdout: &out, Stderr: &errb}); code != 2 {
		t.Fatalf("args: code=%d", code)
	}
}

// TestShimExecHelper is the child of TestShimExecHandsOverExactlyTheRequest: it becomes env(1).
func TestShimExecHelper(t *testing.T) {
	if os.Getenv("NSELF_SHIM_HELPER") != "1" {
		t.Skip("child process only")
	}
	dir := os.Getenv("NSELF_SHIM_DIR")
	req, err := parseShimRequest([]byte(`{"argv":["env"],"dir":"` + dir + `","env":["ONLY=this","PATH=/usr/bin:/bin"]}`))
	if err != nil {
		t.Fatal(err)
	}
	os.Exit(execShimRequest(req, IO{Stderr: os.Stderr}))
}

func TestShimExecHandsOverExactlyTheRequest(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestShimExecHelper$")
	cmd.Env = append(os.Environ(), "NSELF_SHIM_HELPER=1", "NSELF_SHIM_DIR="+dir, "SECRET_FROM_AGENT=leak")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("child: %v\n%s", err, out)
	}
	lines := strings.Fields(string(out))
	if len(lines) != 2 || !strings.Contains(string(out), "ONLY=this") || strings.Contains(string(out), "SECRET_FROM_AGENT") {
		t.Fatalf("the job saw %q", out)
	}
}

func TestShimExecFailuresHaveExitCodes(t *testing.T) {
	var errb bytes.Buffer
	if code := execShimRequest(shimRequest{Argv: []string{"definitely-not-a-command-xyz"}}, IO{Stderr: &errb}); code != 127 {
		t.Fatalf("missing program: %d", code)
	}
	if code := execShimRequest(shimRequest{Argv: []string{"/bin/true"}, Dir: "/nonexistent/dir"}, IO{Stderr: &errb}); code != 126 {
		t.Fatalf("missing dir: %d", code)
	}
}
