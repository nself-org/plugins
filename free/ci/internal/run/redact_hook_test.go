package run

// P7-TRUST-19: the redactor is installed through the Redactor hook, so a job
// that prints a registered value leaves no occurrence in what a store would
// persist (the executor sink) or in what the coordinator keeps and renders.

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/redact"
)

const hookSecret = "hook-test-secret-9f3a7c21"

// printer builds a script that prints the secret raw, in base64, split across
// two writes with a pause, after a workflow command and an OSC escape.
func printer() string {
	return `printf 'raw=%s\n' ` + hookSecret + `;` +
		` printf 'b64='; printf %s ` + hookSecret + ` | base64; echo;` +
		` printf 'split=hook-test-'; sleep 0.1; printf 'secret-9f3a7c21\n';` +
		` echo '::add-mask::other'; printf '\033]52;c;Zm9v\007done\n'`
}

func TestRedactHookInstalledByDefault(t *testing.T) {
	if Registered.NewRedactor == nil {
		t.Fatal("redact_hook.go must register the per-attempt redactor")
	}
	if got := (Hooks{}).Resolved().NewRedactor; got == nil {
		t.Fatal("Resolved() must carry the registered redactor")
	}
	if _, ok := Registered.NewRedactor().(*redact.Filter); !ok {
		t.Fatalf("redactor is %T", Registered.NewRedactor())
	}
}

func TestRegisterSecretValueMinimum(t *testing.T) {
	err := RegisterSecretValue([]byte("12345"))
	var e *redact.Error
	if !errors.As(err, &e) || e.Code != "E673" {
		t.Fatalf("want E673, got %v", err)
	}
}

func TestRedactHookExecutorSink(t *testing.T) {
	if err := RegisterSecretValue([]byte(hookSecret)); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var store []byte // the fake store: whatever the sink receives is persisted
	e := &ciexec.Executor{NewRedactor: Registered.NewRedactor}
	res := e.Run(context.Background(), ciexec.JobSpec{AttemptID: "redact-hook", CoordinatorID: "coord", Home: t.TempDir(),
		Command: []string{"sh", "-c", printer()},
		Output:  func(p []byte) { mu.Lock(); defer mu.Unlock(); store = append(store, p...) }})
	if res.Err != nil || res.ExitCode != 0 {
		t.Fatalf("run: %+v", res)
	}
	assertMasked(t, string(store))
}

func TestRedactHookCoordinatorPath(t *testing.T) {
	if err := RegisterSecretValue([]byte(hookSecret)); err != nil {
		t.Fatal(err)
	}
	r := newRig(t, map[string]model.Job{"a": sh(printer())}, gitFake{})
	r.C.Opts.Stream = true
	res := r.run(t)
	o := outcome(res, "a")
	assertMasked(t, o.Tail)
	assertMasked(t, r.Out.String())
}

func assertMasked(t *testing.T, out string) {
	t.Helper()
	if strings.Contains(out, hookSecret) || strings.Contains(out, "aG9vay10ZXN0LXNlY3JldC05ZjNhN2MyMQ") ||
		strings.Contains(out, "hook-test-secret") {
		t.Fatalf("a registered value reached the output: %q", out)
	}
	if !strings.Contains(out, redact.Placeholder) {
		t.Fatalf("no mask in the output: %q", out)
	}
	if strings.Contains(out, "\x1b]52") || strings.Contains(out, "\n::add-mask::") {
		t.Fatalf("escape or workflow command not neutralised: %q", out)
	}
}

// The executor keeps the default line redactor (named secrets) behind the
// registered streaming redactor, so neither layer is lost.
func TestNewExecutorChainsRedactors(t *testing.T) {
	if err := RegisterSecretValue([]byte(hookSecret)); err != nil {
		t.Fatal(err)
	}
	r := newExecutor(nil, Registered.NewRedactor).NewRedactor()
	in := "api_token=named-value-123 raw=" + hookSecret + " ::add-mask::x\n"
	out := string(r.Redact([]byte(in))) + string(r.Flush())
	if strings.Contains(out, hookSecret) || strings.Contains(out, "named-value-123") || strings.Contains(out, "::add-mask::") {
		t.Fatalf("a layer was lost: %q", out)
	}
	if plain := newExecutor(nil, nil).NewRedactor(); plain == nil {
		t.Fatal("no default redactor")
	}
}
