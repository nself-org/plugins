//go:build !windows

package exec

import (
	"context"
	"errors"
	"os"
	osexec "os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

type gatedTestEnv []string

func (f gatedTestEnv) Environment(context.Context, model.Job) ([]string, error) {
	return append([]string(nil), f...), nil
}

func selfExe(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return exe
}

// probeJob runs command through the executor and returns what it printed (and the result).
func probeJob(t *testing.T, gated bool, env []string, timeout string, command ...string) (string, AttemptResult) {
	t.Helper()
	var out strings.Builder
	spec := JobSpec{AttemptID: "gate-probe", CoordinatorID: "coord", Home: t.TempDir(), Command: command, Output: func(b []byte) { out.Write(b) }}
	spec.Job.Timeout = timeout
	res := (&Executor{Isolation: BaselineIsolator{}, Env: gatedTestEnv(env), GatedStart: gated}).Run(context.Background(), spec)
	return out.String(), res
}

func nulEnv(dump string) []string {
	entries := strings.Split(dump, "\x00")
	sort.Strings(entries)
	return entries
}

// perlDumper is a payload that is not a Go program: it prints its environment NUL-delimited.
func perlDumper(t *testing.T) []string {
	t.Helper()
	perl, err := osexec.LookPath("perl")
	if err != nil {
		t.Fatalf("this test needs perl as a non-Go payload: %v", err)
	}
	return []string{perl, "-e", `print join("\0", map { "$_=$ENV{$_}" } keys %ENV)`}
}

// The gated payload's environment is exactly the one a direct start gives it (the ungated, unlimited path is
// the oracle): same entries, byte for byte, with nothing added by a shell and nothing dropped, including keys
// a shell would reject, duplicate keys, an exported g and a value with a newline. The Go runtime variables
// belong to the payload only: the helper is a Go program and must neither fail nor log because of them.
// Run with a Go payload and with a non-Go one (a Go payload would hide a helper that leaked them to it).
func TestGatedStartKeepsTheEnvironmentExact(t *testing.T) {
	exe := selfExe(t)
	path := os.Getenv("PATH")
	payloads := map[string][]string{"go": {exe, "--dump-env-nul"}, "perl": perlDumper(t)}
	for name, env := range map[string][]string{
		"plain":                {"PATH=" + path, "ONLY=this"},
		"caller shell vars":    {"PATH=" + path, "PWD=/not/the/cwd", "OLDPWD=it's a 'quote'", "SHLVL=7", "_=x"},
		"empty value":          {"PATH=" + path, "PWD="},
		"exported g":           {"PATH=" + path, "g=caller-value"},
		"odd keys":             {"PATH=" + path, "BAD-NAME=dash", "1NUM=digit", "a.b=dot"},
		"newline value":        {"PATH=" + path, "NL=line one\nline two\n", "Q=\"'$`\\"},
		"duplicate PATH":       {"PATH=/nonexistent", "PATH=" + path, "K=1", "K=2"},
		"different PATH":       {"PATH=/nonexistent"},
		"no PATH":              {"ONLY=this"},
		"hostile PATH":         {"PATH=/nonexistent:" + path},
		"empty environment":    {},
		"unicode and equals":   {"U=héllo=wörld", "E==x"},
		"bad entry, no equal":  {"PATH=" + path, "NOEQUALS"},
		"bad GOMEMLIMIT":       {"PATH=" + path, "GOMEMLIMIT=invalid"},
		"bad GOGC, GOMAXPROCS": {"PATH=" + path, "GOGC=bogus", "GOMAXPROCS=abc"},
		"GODEBUG inittrace":    {"PATH=" + path, "GODEBUG=inittrace=1"},
		"GOTRACEBACK crash":    {"PATH=" + path, "GOTRACEBACK=crash", "GORACE=halt_on_error=1"},
		"GODEBUG nonsense":     {"PATH=" + path, "GODEBUG=nosuchsetting=1,,=", "GOFLAGS=-nope"},
	} {
		for pname, payload := range payloads {
			if pname == "go" && strings.Contains(name, "GO") {
				continue // a Go payload would itself fail on a malformed Go runtime variable
			}
			want, res := probeJob(t, false, env, "", payload...)
			if res.Err != nil || res.ExitCode != 0 {
				t.Fatalf("%s/%s: direct run: %+v", name, pname, res)
			}
			for _, timeout := range []string{"", "1h"} {
				got, res := probeJob(t, true, env, timeout, payload...)
				if res.Err != nil || res.ExitCode != 0 {
					t.Fatalf("%s/%s (timeout %q): gated run: %+v out %q", name, pname, timeout, res, got)
				}
				if g, w := nulEnv(got), nulEnv(want); strings.Join(g, "\x00") != strings.Join(w, "\x00") {
					t.Errorf("%s/%s (timeout %q): the payload's environment is not the direct one\n--- direct %q\n--- gated  %q", name, pname, timeout, w, g)
				}
			}
		}
	}
	// The cases above must actually carry what they claim to.
	got, _ := probeJob(t, true, []string{"BAD-NAME=dash", "g=caller-value", "NL=a\nb", "GOMEMLIMIT=invalid"}, "", perlDumper(t)...)
	for _, need := range []string{"BAD-NAME=dash", "g=caller-value", "NL=a\nb", "GOMEMLIMIT=invalid"} {
		if !strings.Contains(got, need) {
			t.Errorf("the gated payload lost %q: %q", need, got)
		}
	}
}

// An environment far larger than a pipe buffer reaches the payload whole: no argv limit, no deadlock between
// the agent writing it and the helper reading it.
func TestGatedStartHugeEnvironment(t *testing.T) {
	env := []string{"PATH=" + os.Getenv("PATH")}
	for i := 0; i < 150; i++ {
		env = append(env, "BIG"+strconv.Itoa(i)+"="+strings.Repeat("v", 4000+i))
	}
	for pname, payload := range map[string][]string{"go": {selfExe(t), "--dump-env-nul"}, "perl": perlDumper(t)} {
		want, res := probeJob(t, false, env, "", payload...)
		if res.Err != nil || res.ExitCode != 0 {
			t.Fatalf("%s: direct run: %+v", pname, res)
		}
		got, res := probeJob(t, true, env, "1h", payload...)
		if res.Err != nil || res.ExitCode != 0 {
			t.Fatalf("%s: gated run: %+v", pname, res)
		}
		if strings.Join(nulEnv(got), "\x00") != strings.Join(nulEnv(want), "\x00") || len(got) < 600000 {
			t.Errorf("%s: the large environment did not arrive intact (%d bytes, direct %d)", pname, len(got), len(want))
		}
	}
}

// PATH=/nonexistent does not stop an absolute payload.
func TestGatedStartAbsolutePayloadNeedsNoPATH(t *testing.T) {
	out, res := probeJob(t, true, []string{"PATH=/nonexistent"}, "1h", selfExe(t), "--dump-env-nul")
	if res.Err != nil || res.ExitCode != 0 || !strings.Contains(out, "PATH=/nonexistent") {
		t.Fatalf("result %+v out %q", res, out)
	}
}

// A bare payload name is found on the job's effective PATH (the last PATH=) and nowhere else: not on the
// agent's PATH, not on an earlier PATH=.
func TestGatedStartResolvesOnTheJobPATHOnly(t *testing.T) {
	dir := t.TempDir()
	if err := os.Symlink(selfExe(t), filepath.Join(dir, "gate-probe-d1531")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH")) // the agent can see it
	for name, tc := range map[string]struct {
		env []string
		ok  bool
	}{
		"job PATH has it":         {[]string{"PATH=" + dir}, true},
		"last PATH wins (has it)": {[]string{"PATH=/nonexistent", "PATH=" + dir}, true},
		"last PATH wins (lacks)":  {[]string{"PATH=" + dir, "PATH=/nonexistent"}, false},
		"job PATH lacks it":       {[]string{"PATH=/nonexistent"}, false},
		"empty job PATH":          {[]string{"PATH="}, false},
	} {
		// the dump mode keys on argv[1] only, so the symlinked name works as the payload
		out, res := probeJob(t, true, tc.env, "", "gate-probe-d1531", "--dump-env-nul")
		if tc.ok && (res.Err != nil || res.ExitCode != 0 || out == "") {
			t.Errorf("%s: want the payload to run: %+v %q", name, res, out)
		}
		if !tc.ok && res.Err == nil {
			t.Errorf("%s: the payload was found outside the job PATH: %+v %q", name, res, out)
		}
	}
}

// A missing or non-executable payload is still a start error, as for a direct start, not a failure inside the helper.
func TestGatedStartMissingBinaryIsAStartError(t *testing.T) {
	for _, cmd := range []string{"definitely-not-a-binary-d1531", "./nope-d1531", "/nonexistent/bin", t.TempDir()} {
		var errs [2]error
		var codes [2]int
		for i, gated := range []bool{false, true} {
			_, res := probeJob(t, gated, []string{"PATH=" + os.Getenv("PATH")}, "", cmd)
			errs[i], codes[i] = res.Err, res.ExitCode
		}
		if errs[0] == nil || errs[1] == nil || codes[0] != codes[1] {
			t.Errorf("%q: direct err=%v code=%d, gated err=%v code=%d", cmd, errs[0], codes[0], errs[1], codes[1])
		}
		var a, b *osexec.Error
		if errors.As(errs[0], &a) != errors.As(errs[1], &b) {
			t.Errorf("%q: the error class differs: %v vs %v", cmd, errs[0], errs[1])
		}
	}
}

// The helper sets the limits `ulimit` set (soft and hard, CPU in seconds, data in KiB on Linux), so the
// payload sees what the old wrapper gave it.
func TestGatedStartAppliesTheSameLimitsAsUlimit(t *testing.T) {
	script := "ulimit -St; ulimit -Ht; ulimit -Sd; ulimit -Hd"
	env := []string{"PATH=" + os.Getenv("PATH")}
	run := func(gated bool) string {
		var out strings.Builder
		spec := JobSpec{AttemptID: "gate-lim", CoordinatorID: "coord", Home: t.TempDir(), MemoryLimitMB: 512, Command: []string{"sh", "-c", script}, Output: func(b []byte) { out.Write(b) }}
		spec.Job.Timeout = "3600s"
		res := (&Executor{Isolation: BaselineIsolator{}, Env: gatedTestEnv(env), GatedStart: gated}).Run(context.Background(), spec)
		if res.Err != nil || res.ExitCode != 0 {
			t.Fatalf("gated=%v: %+v", gated, res)
		}
		return out.String()
	}
	old, got := run(false), run(true)
	if got != old {
		t.Fatalf("limits differ: ulimit wrapper %q, gate helper %q", old, got)
	}
	want := "3600\n3600\n"
	if runtime.GOOS == "linux" {
		want += "524288\n524288\n"
	} else {
		want += "unlimited\nunlimited\n"
	}
	if got != want {
		t.Fatalf("limits %q, want %q", got, want)
	}
}

// The CPU limit is enforced from the payload's first instruction: a busy payload behind the helper is ended
// by the kernel, with no wall-clock deadline involved. Soft and hard are equal (as with `ulimit -t`), so
// Linux ends it with SIGKILL and macOS with SIGXCPU.
func TestGateHelperEnforcesTheCPULimit(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	er, ew, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd := osexec.Command(selfExe(t), GateArg, "1", "0", "/bin/sh", "sh", "-c", "while :; do :; done")
	cmd.ExtraFiles = []*os.File{r, er}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	_ = r.Close()
	_ = er.Close()
	_ = ew.Close() // an empty payload environment
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	if _, err = w.Write([]byte("g\n")); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err = <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("the busy payload was not stopped by its CPU limit")
	}
	var ee *osexec.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("want a signal exit, got %v", err)
	}
	if sig := ee.Sys().(syscall.WaitStatus).Signal(); sig != syscall.SIGXCPU && sig != syscall.SIGKILL {
		t.Fatalf("want SIGXCPU or SIGKILL, got %v", err)
	}
}

// End of file on the gate (no go-ahead) means nothing runs.
func TestGateHelperExitsWithoutRunningOnEOF(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	ran := filepath.Join(t.TempDir(), "ran")
	er, ew, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd := osexec.Command(selfExe(t), GateArg, "0", "0", "/bin/sh", "sh", "-c", "echo x > "+ran)
	cmd.ExtraFiles = []*os.File{r, er}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	_ = r.Close()
	_ = er.Close()
	_ = ew.Close()
	_ = w.Close() // the agent died
	err = cmd.Wait()
	var ee *osexec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != 125 {
		t.Fatalf("want exit 125, got %v", err)
	}
	if _, err = os.Stat(ran); err == nil {
		t.Fatal("the payload ran without a go-ahead")
	}
}
