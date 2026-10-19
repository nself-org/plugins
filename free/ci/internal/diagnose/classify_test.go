package diagnose

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func TestClassifyOOM(t *testing.T) {
	for name, in := range map[string]Input{
		"exit 137 with no timeout":     {ExecClass: "code", ExitCode: 137},
		"container memory limit":       {ExecClass: "code", ExitCode: 1, Reason: "resource_limit"},
		"kernel oom line in output":    {ExecClass: "code", ExitCode: 1, Tail: "build\nOut of memory: Killed process 4242 (cc1)\n"},
		"exec reports a killed signal": {ExecClass: "infra", ExitCode: -1, Err: "signal: killed"},
	} {
		d := Classify(in)
		if d.Class != "infra" || d.Signature != SigOOM || !d.Retryable || d.Remediation == "" {
			t.Errorf("%s: got %+v, want a retryable infra.oom with a remediation", name, d)
		}
	}
}

func TestClassifyClasses(t *testing.T) {
	for _, c := range []struct {
		name string
		in   Input
		want model.FailureClass
		sig  string
		try  bool
	}{
		{"failing test", Input{ExecClass: "code", ExitCode: 1, Tail: "--- FAIL: TestX\nFAIL\n"}, "code", SigCode, false},
		{"unknown failure is code", Input{ExitCode: 3}, "code", SigCode, false},
		{"timeout beats the SIGKILL signature", Input{ExecClass: "timeout", ExitCode: 137, TimedOut: true}, "timeout", SigTimeout, false},
		{"timeout from the class alone", Input{ExecClass: "timeout", ExitCode: -1}, "timeout", SigTimeout, false},
		{"cancelled", Input{ExecClass: "cancelled", ExitCode: 137}, "cancelled", SigCancelled, false},
		{"lost", Input{Lost: true}, "lost", SigLost, false},
		{"disk full", Input{ExecClass: "code", ExitCode: 1, Tail: "write /tmp/x: no space left on device\n"}, "infra", SigDisk, true},
		{"dns", Input{ExecClass: "code", ExitCode: 1, Tail: "curl: (6) Could not resolve host: proxy.golang.org\n"}, "infra", SigDNS, true},
		{"docker down", Input{ExecClass: "infra", ExitCode: -1, Err: "Cannot connect to the Docker daemon at unix:///var/run/docker.sock"}, "infra", SigDocker, true},
		{"missing tool by exit 127", Input{ExecClass: "code", ExitCode: 127, Tail: "sh: 1: golangci-lint: not found\n"}, "infra", SigTool, true},
		{"missing tool by exec error", Input{ExecClass: "infra", Err: `exec: "pnpm": executable file not found in $PATH`}, "infra", SigTool, true},
		{"unmatched executor error stays infra", Input{ExecClass: "infra", Err: "workspace cleanup failed"}, "infra", SigExecutor, false},
		{"program output naming a signature after a zero exit is not a failure", Input{ExecClass: "code", ExitCode: 0, Tail: "no space left on device"}, "code", SigCode, false},
		{"a word in a code failure does not match", Input{ExecClass: "code", ExitCode: 1, Tail: "Killed 3 stale workers\nFAIL\n"}, "code", SigCode, false},
	} {
		d := Classify(c.in)
		if d.Class != c.want || d.Signature != c.sig || d.Retryable != c.try || d.Remediation == "" {
			t.Errorf("%s: got %+v, want class %s signature %s retryable %v", c.name, d, c.want, c.sig, c.try)
		}
	}
}

func TestForClass(t *testing.T) {
	for c, sig := range map[model.FailureClass]string{"infra": SigExecutor, "timeout": SigTimeout, "cancelled": SigCancelled, "lost": SigLost, "code": SigCode, "": SigCode} {
		if d := ForClass(c); d.Signature != sig || d.Remediation == "" || d.Retryable {
			t.Errorf("ForClass(%q) = %+v, want signature %s with a remediation and no retry", c, d, sig)
		}
	}
}

func TestSignatureOnlyInTail(t *testing.T) {
	old := "no space left on device\n" + strings.Repeat("ok line\n", 1200) + "FAIL\n"
	if d := Classify(Input{ExecClass: "code", ExitCode: 1, Tail: old}); d.Class != "code" {
		t.Fatalf("a signature outside the tail window decided the class: %+v", d)
	}
}

func TestExcerpt(t *testing.T) {
	if got := Excerpt("short\n", 0); got != "short" {
		t.Fatalf("short text changed: %q", got)
	}
	long := strings.Repeat("line of output é\n", 1000)
	got := Excerpt(long, 0)
	if len(got) > ExcerptMax || !utf8.ValidString(got) || !strings.HasSuffix(got, "output é") {
		t.Fatalf("excerpt len %d valid %v", len(got), utf8.ValidString(got))
	}
	if !strings.HasPrefix(got, "line of output") {
		t.Fatalf("excerpt does not start at a line: %q", got[:20])
	}
	if got := Excerpt(strings.Repeat("é", 4000), 100); len(got) > 100 || !utf8.ValidString(got) {
		t.Fatalf("multi-byte cut broke: len %d", len(got))
	}
}
