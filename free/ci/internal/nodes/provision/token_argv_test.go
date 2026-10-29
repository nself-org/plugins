package provision

// Purpose: prove the runner registration token never appears on an argv or
//   in a command string (D-0263): not in the rendered remote script, not in
//   the ssh argv, not in the argv of sudo, sh or config.sh on the host. The
//   token reaches config.sh only as ACTIONS_RUNNER_INPUT_TOKEN.
// Inputs:  an obviously fake token; a recording ssh stub on PATH; for the
//   end-to-end case, stub sudo/svc.sh/config.sh in a temp dir run by the real
//   rendered script under LocalExecutor (bash), so no real host is touched.
// Outputs: failures naming where the token leaked.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const fakeRegToken = "FAKE-REG-TOKEN-d0263-0000"

func tokenOpts(root string) ProvisionOptions {
	return ProvisionOptions{InstallRoot: root, GithubURL: "https://github.com/nself-org/example", RegToken: fakeRegToken, Labels: []string{"x"}}
}

func TestInstallRunnerInstance_TokenOnlyOnStdin(t *testing.T) {
	ex := &fakeExecutor{label: "ci@h", fallback: fakeResponse{out: "ok"}}
	if _, err := installRunnerInstance(context.Background(), ex, testManifest(), tokenOpts("/opt/actions-runner"), 1); err != nil {
		t.Fatal(err)
	}
	script := ex.commands[0]
	for _, bad := range []string{fakeRegToken, "--token"} {
		if strings.Contains(script, bad) {
			t.Fatalf("rendered script holds %q:\n%s", bad, script)
		}
	}
	if ex.inputs[0] != fakeRegToken+"\n" {
		t.Fatalf("stdin = %q, want the token and a newline", ex.inputs[0])
	}
	for _, want := range []string{"export ACTIONS_RUNNER_INPUT_TOKEN", "\nset +x\n", `"$NSELF_RUNNER_TOKEN" 2>/dev/null | sudo`, "\nunset NSELF_RUNNER_TOKEN\n"} {
		if !strings.Contains(script, want) {
			t.Fatalf("script lacks %q:\n%s", want, script)
		}
	}
}

func TestProvision_RefusesMultiLineToken(t *testing.T) {
	ex := &fakeExecutor{fallback: fakeResponse{out: "ok"}}
	opts := tokenOpts("")
	opts.RegToken = "FAKE-LINE-1\nFAKE-LINE-2"
	res, err := Provision(context.Background(), ex, testManifest(), opts)
	if err == nil || res == nil || len(ex.commands) != 0 {
		t.Fatalf("err=%v res=%v commands=%d, want a refusal before any host work", err, res, len(ex.commands))
	}
	if strings.Contains(err.Error(), "FAKE-LINE") {
		t.Fatalf("refusal prints the token: %v", err)
	}
}

// stubSSHStdin is stubSSH that also records ssh's stdin.
func stubSSHStdin(t *testing.T, body string) (argvLog, stdinLog string) {
	t.Helper()
	dir := t.TempDir()
	argvLog, stdinLog = filepath.Join(dir, "argv.log"), filepath.Join(dir, "stdin.log")
	script := "#!/bin/sh\nfor a in \"$@\"; do printf '%s\\n' \"$a\"; done > " + argvLog + "\ncat > " + stdinLog + "\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return argvLog, stdinLog
}

func TestSSHExecutor_TokenNeverOnSSHArgv(t *testing.T) {
	argvLog, stdinLog := stubSSHStdin(t, "echo remote-out; echo remote-err >&2; exit 0")
	ex := NewSSHExecutor("ci@host-1", "/k/key")
	out, err := installRunnerInstance(context.Background(), ex, testManifest(), tokenOpts("/opt/actions-runner"), 1)
	if err != nil || !strings.Contains(out, "remote-out") || !strings.Contains(out, "remote-err") {
		t.Fatalf("out=%q err=%v", out, err)
	}
	argv, _ := os.ReadFile(argvLog)
	if strings.Contains(string(argv), fakeRegToken) || !strings.Contains(string(argv), "--\nci@host-1\nset -e\n") {
		t.Fatalf("ssh argv holds the token or lost its shape:\n%s", argv)
	}
	if in, _ := os.ReadFile(stdinLog); string(in) != fakeRegToken+"\n" {
		t.Fatalf("ssh stdin = %q", in)
	}
}

func TestSSHExecutor_RunInputFailureKeepsRunShape(t *testing.T) {
	stubSSHStdin(t, "echo boom; exit 3")
	_, err := NewSSHExecutor("ci@host-1", "/k/key").RunInput(context.Background(), "x", "in\n")
	var coder interface{ ExitCode() int }
	if err == nil || !strings.Contains(err.Error(), "remote command on ci@host-1 failed") ||
		!strings.Contains(err.Error(), "boom") || !errors.As(err, &coder) || coder.ExitCode() != 3 {
		t.Fatalf("err=%v", err)
	}
	if _, err := NewSSHExecutor("-oProxyCommand=x", "/k").RunInput(context.Background(), "x", ""); err == nil {
		t.Fatal("RunInput accepted a destination Run refuses")
	}
}

// TestInstallRunnerInstance_EndToEndArgvAndEnv runs the real rendered script
// under bash with recording stubs: sudo logs its argv and runs the command
// (dropping -u USER), config.sh logs its argv and ACTIONS_RUNNER_INPUT_TOKEN.
func TestInstallRunnerInstance_EndToEndArgvAndEnv(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	root, bin := t.TempDir(), t.TempDir()
	argvLog, envLog := filepath.Join(root, "argv.log"), filepath.Join(root, "env.log")
	inst := filepath.Join(root, "runner-1")
	if err := os.MkdirAll(inst, 0o755); err != nil {
		t.Fatal(err)
	}
	logArgv := "printf '%s\\n' \"$0 $*\" >> " + argvLog + "\n"
	write := func(path, body string) {
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"+logArgv+body+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(bin, "sudo"), `case "$1" in chown) exit 0;; -u) shift 2;; esac; exec "$@"`)
	write(filepath.Join(inst, "config.sh"), `printf '%s\n' "${ACTIONS_RUNNER_INPUT_TOKEN-unset}" > `+envLog)
	write(filepath.Join(inst, "svc.sh"), "exit 0")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	if out, err := installRunnerInstance(context.Background(), LocalExecutor{}, testManifest(), tokenOpts(root), 1); err != nil {
		t.Fatalf("script failed: %v\n%s", err, out)
	}
	argv, _ := os.ReadFile(argvLog)
	if strings.Contains(string(argv), fakeRegToken) {
		t.Fatalf("token on an argv:\n%s", argv)
	}
	if !strings.Contains(string(argv), "./config.sh --url https://github.com/nself-org/example --name local-1") {
		t.Fatalf("config.sh did not run with the expected argv:\n%s", argv)
	}
	if env, _ := os.ReadFile(envLog); string(env) != fakeRegToken+"\n" {
		t.Fatalf("config.sh saw ACTIONS_RUNNER_INPUT_TOKEN=%q", env)
	}
	// no token on stdin: the script refuses before any host work
	os.Remove(argvLog)
	out, err := LocalExecutor{}.RunInput(context.Background(), renderedScript(t, root), "")
	if err == nil || !strings.Contains(out, "no runner registration token on stdin") {
		t.Fatalf("empty stdin: out=%q err=%v", out, err)
	}
	if _, statErr := os.Stat(argvLog); statErr == nil {
		t.Fatal("the script ran a command without a token")
	}
}

// A host shell that starts with tracing on must not print the expanded token.
// Enable tracing in the same shell immediately before the generated script; this
// exercises inherited xtrace without relying on platform-specific BASH_ENV startup rules.
func TestInstallRunnerInstance_InheritedXtraceDoesNotLeak(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	root, bin := t.TempDir(), t.TempDir()
	inst := filepath.Join(root, "runner-1")
	if err := os.MkdirAll(inst, 0o755); err != nil {
		t.Fatal(err)
	}
	for path, body := range map[string]string{
		filepath.Join(bin, "sudo"):       `case "$1" in chown) exit 0;; -u) shift 2;; esac; exec "$@"`,
		filepath.Join(inst, "config.sh"): "cat >/dev/null",
		filepath.Join(inst, "svc.sh"):    "exit 0",
	} {
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// The stub PATH belongs to this child only.
	ex := LocalExecutor{env: []string{
		"PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH"),
	}}
	raw, err := ex.RunInput(context.Background(), "set -x\n"+renderedScript(t, root), fakeRegToken+"\n")
	if err != nil || !strings.Contains(raw, "+ set -e") {
		t.Fatalf("tracing was not on, the test proves nothing: err=%v out=%q", err, raw)
	}
	if strings.Contains(raw, fakeRegToken) {
		t.Fatalf("traced output holds the token:\n%s", raw)
	}
}

// Output and errors that carry the token (a pty echoing stdin under
// RequestTTY force) are redacted, and the exit code survives for finish().
func TestInstallRunnerInstance_RedactsEchoedToken(t *testing.T) {
	echo := &echoExecutor{fail: exitErr(t, 7)}
	out, err := installRunnerInstance(context.Background(), echo, testManifest(), tokenOpts("/opt/actions-runner"), 1)
	if strings.Contains(out, fakeRegToken) || !strings.Contains(out, redacted) {
		t.Fatalf("out = %q", out)
	}
	var coder interface{ ExitCode() int }
	if err == nil || strings.Contains(err.Error(), fakeRegToken) || !errors.As(err, &coder) || coder.ExitCode() != 7 {
		t.Fatalf("err = %v", err)
	}
}

// echoExecutor returns its stdin as output and inside its error, as a pty
// that echoes input would.
type echoExecutor struct{ fail error }

func (e *echoExecutor) Run(ctx context.Context, c string) (string, error) {
	return e.RunInput(ctx, c, "")
}
func (e *echoExecutor) Label() string { return "echo" }
func (e *echoExecutor) RunInput(_ context.Context, _, in string) (string, error) {
	return "echo: " + in, fmt.Errorf("remote failed: %w\necho: %s", e.fail, in)
}

// exitErr returns a real *exec.ExitError with the given code.
func exitErr(t *testing.T, code int) error {
	t.Helper()
	err := exec.Command("sh", "-c", "exit "+strconv.Itoa(code)).Run()
	if err == nil {
		t.Fatal("no exit error")
	}
	return err
}

// renderedScript captures the script installRunnerInstance sends.
func renderedScript(t *testing.T, root string) string {
	t.Helper()
	ex := &fakeExecutor{label: "local", fallback: fakeResponse{out: "ok"}}
	if _, err := installRunnerInstance(context.Background(), ex, testManifest(), tokenOpts(root), 1); err != nil {
		t.Fatal(err)
	}
	return ex.commands[0]
}
