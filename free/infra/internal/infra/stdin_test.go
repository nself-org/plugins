package infra

// Purpose: prove apply and destroy hand the caller's stdin to the tofu or
// terraform child, so the tool's own approval prompt can be answered when
// --auto-approve is not given (D-0384).
// Inputs: a fake `tofu` shell script on a private PATH. It exits 0 for init,
// and for apply or destroy it exits 0 only when its first stdin line is "yes".
// Outputs: test failures. No network, no cloud credentials, no real tofu.
// Constraints: os.Stdin is swapped for a file for the length of one test.

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

const promptStub = `#!/bin/sh
# $1 is -chdir=<dir>, $2 the verb.
case "$2" in
  init) exit 0 ;;
  apply|destroy) read ans; [ "$ans" = yes ] ;;
  *) exit 0 ;;
esac
`

// stubPromptTofu puts the fake tofu alone on PATH and points os.Stdin at a file
// holding input; both are restored when the test ends.
func stubPromptTofu(t *testing.T, input string) {
	t.Helper()
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "tofu"), []byte(promptStub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	in := filepath.Join(t.TempDir(), "stdin")
	if err := os.WriteFile(in, []byte(input), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(in)
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdin
	os.Stdin = f
	t.Cleanup(func() { os.Stdin = old; _ = f.Close() })
}

func TestApplyPassesStdinToChild(t *testing.T) {
	stubPromptTofu(t, "yes\n")
	opts := ApplyOptions{PlanOptions: PlanOptions{Provider: ProviderHetzner, Domain: "x.test", ModulesDir: t.TempDir()}}
	if err := Apply(context.Background(), opts); err != nil {
		t.Fatalf("Apply with \"yes\" on stdin: %v (the child did not receive stdin)", err)
	}
}

func TestApplyStdinAnswerNoFails(t *testing.T) {
	stubPromptTofu(t, "no\n")
	opts := ApplyOptions{PlanOptions: PlanOptions{Provider: ProviderHetzner, Domain: "x.test", ModulesDir: t.TempDir()}}
	if err := Apply(context.Background(), opts); err == nil {
		t.Fatal("Apply with \"no\" on stdin: want an error, got nil")
	}
}

func TestDestroyPassesStdinToChild(t *testing.T) {
	stubPromptTofu(t, "yes\n")
	if err := Destroy(context.Background(), ProviderHetzner, "x.test", t.TempDir(), false); err != nil {
		t.Fatalf("Destroy with \"yes\" on stdin: %v (the child did not receive stdin)", err)
	}
}

// TestPlanDoesNotNeedStdin pins that plan and init give the child no stdin: the
// stub answers both without reading, so an empty stdin file must be enough.
func TestPlanDoesNotNeedStdin(t *testing.T) {
	stubPromptTofu(t, "")
	if err := Plan(context.Background(), PlanOptions{Provider: ProviderHetzner, Domain: "x.test", ModulesDir: t.TempDir()}); err != nil {
		t.Fatalf("Plan: %v", err)
	}
}
