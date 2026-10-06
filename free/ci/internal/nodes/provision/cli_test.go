package provision

// Purpose: command-level tests of `nodes provision` and `nodes verify`: every
//   refusal core made before touching a host, in core's order and with core's
//   text; the token source and that the token is never printed; verify's exit
//   policy (fail, unreachable, drift); help output. A fake Executor replaces
//   every host, so nothing here runs a command on a machine.

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

// harness runs one command with a fake executor and returns code, stdout, stderr.
type harness struct {
	env   map[string]string
	fakes []*fakeExecutor
	built [][]string // hosts passed to the factory
}

func (h *harness) run(fn func([]string, Deps) int, args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	d := Deps{Out: &out, Err: &errb, Ctx: context.Background(),
		Getenv: func(k string) string { return h.env[k] },
		Executors: func(hosts []string, _ string) []Executor {
			h.built = append(h.built, hosts)
			if len(hosts) == 0 {
				hosts = []string{"local"}
			}
			ex := make([]Executor, len(hosts))
			for i, name := range hosts {
				f := h.fakes[i%len(h.fakes)]
				if f.label == "" {
					f.label = name
				}
				ex[i] = f
			}
			return ex
		}}
	return fn(args, d), out.String(), errb.String()
}

func okFake() *fakeExecutor { return &fakeExecutor{fallback: fakeResponse{out: "ok"}} }

func TestProvision_RefusalsInCoreOrder(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		args []string
		want string
	}{
		{"no url", nil, nil, "--github-url is required (the repo or org runners register against)"},
		{"no url wins over no token", nil, []string{"--host", "a", "--host", "b"}, "--github-url is required"},
		{"no token", nil, []string{"--github-url", "https://x"}, "a runner registration token is required: pass --token or set GITHUB_RUNNER_TOKEN"},
		{"two hosts", nil, []string{"--github-url", "u", "--token", "t", "--host", "a", "--host", "b"}, "provision takes at most one --host per invocation; run it once per host"},
		{"two hosts csv", nil, []string{"--github-url", "u", "--token", "t", "--host", "a,b"}, "provision takes at most one --host"},
		{"json", nil, []string{"--json"}, "does not support --json"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := &harness{env: tc.env, fakes: []*fakeExecutor{okFake()}}
			code, out, errs := h.run(RunProvision, tc.args...)
			if code != 1 || out != "" || !strings.HasPrefix(errs, "Error: ") || !strings.Contains(errs, tc.want) {
				t.Fatalf("code=%d out=%q err=%q, want exit 1, empty stdout, Error: ...%s", code, out, errs, tc.want)
			}
			if len(h.built) != 0 || len(h.fakes[0].commands) != 0 {
				t.Fatalf("a refusal built an executor or ran a command: %v %v", h.built, h.fakes[0].commands)
			}
		})
	}
}

func TestProvision_TokenSourceAndNeverPrinted(t *testing.T) {
	const flagTok, envTok = "FLAG-TOKEN-1", "ENV-TOKEN-2"
	h := &harness{env: map[string]string{"GITHUB_RUNNER_TOKEN": envTok}, fakes: []*fakeExecutor{okFake()}}
	code, out, errs := h.run(RunProvision, "--github-url", "https://github.com/o/r", "--host", "ci@h1")
	if code != 0 || errs != "" {
		t.Fatalf("code=%d err=%q", code, errs)
	}
	joined := strings.Join(h.fakes[0].commands, "\n")
	if !strings.Contains(joined, "--token '"+envTok+"'") {
		t.Fatalf("env token was not used; commands:\n%s", joined)
	}
	for _, s := range []string{out, errs} {
		if strings.Contains(s, envTok) {
			t.Fatalf("token printed: %q", s)
		}
	}
	if !strings.HasPrefix(out, "Provisioning ci@h1 (1 instance(s))...\n") || !strings.HasSuffix(out, "Provision complete.\n") {
		t.Fatalf("stdout = %q", out)
	}
	// --token overrides the env var
	h = &harness{env: map[string]string{"GITHUB_RUNNER_TOKEN": envTok}, fakes: []*fakeExecutor{okFake()}}
	h.run(RunProvision, "--github-url", "u", "--token", flagTok)
	joined = strings.Join(h.fakes[0].commands, "\n")
	if !strings.Contains(joined, "'"+flagTok+"'") || strings.Contains(joined, envTok) {
		t.Fatalf("--token did not override the env var:\n%s", joined)
	}
}

func TestProvision_FailureStopsAndExitsOne(t *testing.T) {
	f := okFake()
	f.when("apt-get install", "apt said no", errors.New("exit 100"))
	h := &harness{fakes: []*fakeExecutor{f}}
	code, out, errs := h.run(RunProvision, "--github-url", "u", "--token", "t", "--instances", "2", "--host", "h")
	if code != 1 || !strings.Contains(out, "  [install-packages] apt said no\n") || strings.Contains(out, "Provision complete.") {
		t.Fatalf("code=%d out=%q", code, out)
	}
	if errs != "Error: runner provision: step \"install-packages\": exit 100\n" {
		t.Fatalf("err = %q", errs)
	}
	if strings.Contains(strings.Join(f.commands, "\n"), "config.sh") {
		t.Fatal("an instance was configured after a failed step")
	}
}

func TestProvision_OptionsReachTheScript(t *testing.T) {
	f := okFake()
	h := &harness{fakes: []*fakeExecutor{f}}
	code, out, _ := h.run(RunProvision, "--github-url=https://github.com/o/r", "--token=t", "--instances=2",
		"--install-root=/srv/runners", "--labels", "gpu,fast", "--labels=x", "--host=ci@h")
	if code != 0 || !strings.HasPrefix(out, "Provisioning ci@h (2 instance(s))...") {
		t.Fatalf("code=%d out=%q", code, out)
	}
	joined := strings.Join(f.commands, "\n")
	for _, want := range []string{"/srv/runners/runner-1", "/srv/runners/runner-2", "--labels 'self-hosted,Linux,X64,gpu,fast,x'", "--name 'ci@h-2'"} {
		if !strings.Contains(joined, want) {
			t.Errorf("script lacks %q", want)
		}
	}
}

func TestVerify_ExitPolicy(t *testing.T) {
	good := &fakeExecutor{label: "a", fallback: fakeResponse{out: "/usr/bin/x installed DIR"}}
	good.when("echo reachable", "reachable", nil)
	good.when("shopt -s nullglob", "CHROME /c/chrome", nil)
	bad := &fakeExecutor{label: "b", fallback: fakeResponse{out: "", err: errors.New("nope")}}
	bad.when("echo reachable", "reachable", nil)
	down := &fakeExecutor{label: "c", fallback: fakeResponse{err: errors.New("ssh: connect refused")}}

	cases := []struct {
		name  string
		fakes []*fakeExecutor
		hosts []string
		code  int
		want  string
	}{
		{"all pass", []*fakeExecutor{good}, []string{"a"}, 0, "No drift detected across reachable hosts."},
		{"failing check", []*fakeExecutor{bad}, []string{"b"}, 1, "FAIL"},
		{"unreachable host", []*fakeExecutor{down}, []string{"c"}, 1, "CHECK"},
		{"drift", []*fakeExecutor{good, bad}, []string{"a", "b"}, 1, "DRIFT DETECTED"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := &harness{fakes: tc.fakes}
			args := []string{}
			for _, host := range tc.hosts {
				args = append(args, "--host", host)
			}
			code, out, errs := h.run(RunVerify, args...)
			if code != tc.code || !strings.Contains(out, tc.want) || errs != "" {
				t.Fatalf("code=%d out=%q err=%q, want %d with %q", code, out, errs, tc.code, tc.want)
			}
		})
	}
}

func TestVerify_JSONAndDefaultHost(t *testing.T) {
	h := &harness{fakes: []*fakeExecutor{{label: "local", fallback: fakeResponse{err: errors.New("down")}}}}
	code, out, _ := h.run(RunVerify, "--json")
	if code != 1 || !strings.HasPrefix(out, "[\n  {\n    \"host\": \"local\",\n    \"error\": ") {
		t.Fatalf("code=%d out=%q", code, out)
	}
	if len(h.built) != 1 || len(h.built[0]) != 0 {
		t.Fatalf("no --host must pass an empty host list (local executor), got %v", h.built)
	}
}

func TestExecutorsFromFlags(t *testing.T) {
	if ex := ExecutorsFromFlags(nil, ""); len(ex) != 1 || ex[0].Label() != "local" {
		t.Fatalf("no host: %v", ex)
	}
	ex := ExecutorsFromFlags([]string{"local", "ci@h"}, "/k")
	if _, ok := ex[0].(LocalExecutor); !ok || ex[1].Label() != "ci@h" {
		t.Fatalf("hosts: %v", ex)
	}
}

func TestHelp_PrintedAndNoHostTouched(t *testing.T) {
	for _, tc := range []struct {
		fn   func([]string, Deps) int
		name string
		want string
	}{
		{RunProvision, "provision", "nself ci nodes provision [flags]"},
		{RunVerify, "verify", "nself ci nodes verify [flags]"},
	} {
		for _, flag := range []string{"--help", "-h"} {
			h := &harness{fakes: []*fakeExecutor{okFake()}}
			code, out, errs := h.run(tc.fn, flag)
			if code != 0 || errs != "" || !strings.Contains(out, tc.want) || len(h.built) != 0 {
				t.Fatalf("%s %s: code=%d out=%q err=%q", tc.name, flag, code, out, errs)
			}
		}
	}
}
