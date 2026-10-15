// Purpose: regression tests for D-0481: status and uninstall read the same
// explicit kube context as install and upgrade, and upgrade says it ignores
// --wait=false (helm --atomic always waits).
//
// Inputs: the built nself-k8s binary and the stub helm of the CLI harness.
//
// Outputs: test results.
//
// Constraints: no cluster, no real helm.
package main

import (
	"slices"
	"strings"
	"testing"
)

func TestD0481StatusAndUninstallPassKubeContext(t *testing.T) {
	for _, args := range [][]string{
		{"status", "--context", "ctx-a"},
		{"status", "--context", "ctx-a", "--json"},
		{"uninstall", "--yes", "--context", "ctx-a"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			r := runCLI(t, cliRun{Helm: "ok", Env: []string{"HELM_KUBECONTEXT=inherited"}}, args...)
			calls := argvCalls(t, r.ArgvFile)
			if r.Exit != 0 || len(calls) != 1 {
				t.Fatalf("exit %d, calls %v, stderr %q", r.Exit, calls, r.Stderr)
			}
			i := slices.Index(calls[0], "--kube-context")
			if i < 0 || i+1 >= len(calls[0]) || calls[0][i+1] != "ctx-a" {
				t.Errorf("--kube-context ctx-a missing: %v", calls[0])
			}
			if !strings.Contains(r.Stderr, "target context ctx-a (--context)") {
				t.Errorf("target not announced: %q", r.Stderr)
			}
		})
	}
}

func TestD0481UpgradeWaitFalseIsIgnoredAndSaid(t *testing.T) {
	dir := newProject(t)
	writeGenerated(t, dir)
	r := runCLI(t, cliRun{Helm: "ok"}, "upgrade", "--project-dir", dir, "--wait=false")
	calls := argvCalls(t, r.ArgvFile)
	if r.Exit != 0 || len(calls) != 1 {
		t.Fatalf("exit %d, calls %v, stderr %q", r.Exit, calls, r.Stderr)
	}
	if !slices.Contains(calls[0], "--atomic") || !slices.Contains(calls[0], "--wait") {
		t.Errorf("upgrade must still wait: %v", calls[0])
	}
	if !strings.Contains(r.Stderr, "--wait=false has no effect on upgrade") {
		t.Errorf("no warning: %q", r.Stderr)
	}
	// install honours it, so no warning there; the default upgrade is silent too.
	r = runCLI(t, cliRun{Helm: "ok"}, "upgrade", "--project-dir", dir)
	if strings.Contains(r.Stderr, "--wait=false") {
		t.Errorf("warning without --wait=false: %q", r.Stderr)
	}
}
