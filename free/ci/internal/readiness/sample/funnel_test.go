package sample

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// TestHelperProcess is the child the Funnel tests start (the test binary itself, no shell).
func TestHelperProcess(t *testing.T) {
	args := flag.Args()
	if len(args) == 0 {
		return
	}
	switch args[0] {
	case "exit3":
		fmt.Println("busy: render job")
		os.Exit(3)
	case "env":
		fmt.Println(len(os.Environ()), os.Getenv("SECRET_FOR_TEST") == "")
	case "streams":
		fmt.Fprintln(os.Stderr, "warning from stderr")
		fmt.Println("reason from stdout")
		os.Exit(3)
	case "flood":
		fmt.Print(strings.Repeat("x", 4*MaxOutput))
	case "deaf":
		signal.Ignore(syscall.SIGTERM)
		time.Sleep(time.Minute)
	case "hang":
		time.Sleep(time.Minute)
	}
	os.Exit(0)
}

func helper(mode string) []string {
	return []string{os.Args[0], "-test.run=^TestHelperProcess$", "--", mode}
}

func TestFunnelRunsAnArgvAndMapsTheExitCode(t *testing.T) {
	f := Funnel{Home: t.TempDir()}
	res, err := f.Run(context.Background(), helper("exit3"), 20*time.Second)
	if err != nil || res.ExitCode != 3 || !strings.Contains(res.Stdout, "busy: render job") {
		t.Fatalf("%+v %v", res, err)
	}
	if res, err = f.Run(context.Background(), helper("ok"), 20*time.Second); err != nil || res.ExitCode != 0 {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestFunnelEnvironmentIsMinimal(t *testing.T) {
	t.Setenv("SECRET_FOR_TEST", "must-not-reach-the-child")
	res, err := Funnel{Home: t.TempDir()}.Run(context.Background(), helper("env"), 20*time.Second)
	if err != nil || !strings.Contains(res.Stdout, "true") {
		t.Fatalf("the caller's environment must not reach the command: %+v %v", res, err)
	}
	env, _ := minimalEnv{}.Environment(context.Background(), model.Job{})
	if len(env) != 2 || !strings.HasPrefix(env[0], "PATH=") || env[1] != "LANG=C" {
		t.Fatalf("env %v", env)
	}
}

func TestFunnelTimeoutAndErrors(t *testing.T) {
	f := Funnel{Home: t.TempDir()}
	if _, err := f.Run(context.Background(), helper("hang"), 300*time.Millisecond); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("timeout: %v", err)
	}
	if _, err := f.Run(context.Background(), nil, time.Second); err == nil {
		t.Fatal("empty argv")
	}
	// The funnel's launcher reports a missing binary as a shell-style exit (126/127), never as
	// 0 or 3, so a broken readiness.cmd path can only read as "unknown".
	res, err := f.Run(context.Background(), []string{"/nonexistent/readiness-hook"}, time.Second)
	if err == nil && (res.ExitCode == 0 || res.ExitCode == 3) {
		t.Fatalf("a missing binary must not read as ready or not-ready: %+v", res)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := f.Run(ctx, helper("ok"), time.Second); err == nil {
		t.Fatal("a cancelled context is an error")
	}
}

func TestFunnelCapsOutput(t *testing.T) {
	res, err := Funnel{Home: t.TempDir()}.Run(context.Background(), helper("flood"), 20*time.Second)
	if err != nil || len(res.Stdout) != MaxOutput || !res.Truncated {
		t.Fatalf("a chatty command is cut at MaxOutput and says so: %d bytes truncated=%v %v", len(res.Stdout), res.Truncated, err)
	}
	if res, err = (Funnel{Home: t.TempDir()}).Run(context.Background(), helper("ok"), 20*time.Second); err != nil || res.Truncated {
		t.Fatalf("a quiet command is not truncated: %+v %v", res, err)
	}
}

// The funnel has one output stream: stderr text arrives in Result.Stdout, ahead of or between
// stdout lines. readiness.hook documents it; this pins it so a funnel change is noticed.
func TestHookReasonTakesTheFirstLineOfBothStreams(t *testing.T) {
	res, err := Funnel{Home: t.TempDir()}.Run(context.Background(), helper("streams"), 20*time.Second)
	if err != nil || res.ExitCode != 3 {
		t.Fatalf("%+v %v", res, err)
	}
	if !strings.Contains(res.Stdout, "warning from stderr") || !strings.Contains(res.Stdout, "reason from stdout") {
		t.Fatalf("both streams are expected in the one output: %q", res.Stdout)
	}
}

// A command that ignores SIGTERM must not outlive its timeout by the executor's 10 s default grace.
func TestFunnelKillsPastTheTimeoutQuickly(t *testing.T) {
	start := time.Now()
	_, err := Funnel{Home: t.TempDir()}.Run(context.Background(), helper("deaf"), 300*time.Millisecond)
	if err == nil || time.Since(start) > 5*time.Second {
		t.Fatalf("err %v after %v", err, time.Since(start))
	}
}

// The funnel keeps its own state directory below the CI home, away from the job home that
// every job start sweeps.
func TestFunnelHomeIsASubdirectoryOfTheCIHome(t *testing.T) {
	ci := t.TempDir()
	t.Setenv("NSELF_CI_HOME", ci)
	if _, err := (Funnel{}).Run(context.Background(), helper("ok"), 20*time.Second); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(ci, "readiness")); err != nil {
		t.Fatalf("the funnel must work in <CI home>/readiness: %v", err)
	}
	if _, err := os.Stat(filepath.Join(ci, "work")); err == nil {
		t.Fatal("the job home <CI home>/work must stay untouched")
	}
}
