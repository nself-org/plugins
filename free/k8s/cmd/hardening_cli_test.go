// Purpose: tests for the review fixes of P7-DEPL-07: release-name validation,
// the cluster-unreachable classification, --json spellings, a command that ran
// in human mode although the arguments mention --json, double error printing,
// and the status remediation.
//
// Inputs: the binary and stubs from cli_harness_test.go.
//
// Outputs: test results.
//
// Constraints: no network, no cluster.
package main

import (
	"strings"
	"testing"
)

func TestWantsJSON(t *testing.T) {
	cases := []struct {
		args []string
		want bool
	}{
		{[]string{"status", "--json"}, true},
		{[]string{"status", "--json=true"}, true},
		{[]string{"status", "--json=1"}, true},
		{[]string{"status", "--json=t"}, true},
		{[]string{"status", "--json=T"}, true},
		{[]string{"status", "--json=TRUE"}, true},
		{[]string{"status", "--json=True"}, true},
		{[]string{"status", "--json=false"}, false},
		{[]string{"status", "--json=0"}, false},
		{[]string{"status", "--json=F"}, false},
		{[]string{"status", "--json=maybe"}, false},
		{[]string{"status", "--", "--json"}, false},
		{[]string{"status"}, false},
	}
	for _, c := range cases {
		if got := wantsJSON(c.args); got != c.want {
			t.Errorf("wantsJSON(%v) = %v, want %v", c.args, got, c.want)
		}
	}
}

func TestJSONSpellingsReachTheEnvelope(t *testing.T) {
	for _, form := range []string{"--json=1", "--json=t", "--json=TRUE"} {
		wantSuccess(t, runCLI(t, cliRun{Helm: "ok"}, "status", form), "k8s status")
	}
	// =false and =0 are human mode: the plain summary line, no envelope.
	for _, form := range []string{"--json=false", "--json=0"} {
		r := runCLI(t, cliRun{Helm: "ok"}, "status", form)
		if r.Exit != 0 || !strings.HasPrefix(r.Stdout, `{"name":"nself"`) {
			t.Errorf("%s: exit %d, stdout %q", form, r.Exit, r.Stdout)
		}
	}
}

func TestClusterUnreachableIsInfraNotReleaseNotFound(t *testing.T) {
	for _, args := range [][]string{{"status", "--json"}, {"uninstall", "--yes", "--json"}} {
		d := wantFailure(t, runCLI(t, cliRun{Helm: "unreachable"}, args...), "k8s "+args[0], "E722", "infra")
		if d.Error.Code == "E725" {
			t.Errorf("%v: an unknown kube context must not be E725", args)
		}
	}
}

func TestReleaseNameValidation(t *testing.T) {
	long := strings.Repeat("a", 54)
	bad := []string{"--dry-run", "-x", "Upper", "a_b", "trailing-", "has space", long, "-"}
	for _, name := range bad {
		for _, js := range []bool{false, true} {
			args := []string{"uninstall", "--yes", "--release=" + name}
			if js {
				args = append(args, "--json")
			}
			r := runCLI(t, cliRun{Helm: "ok"}, args...)
			if calls := argvCalls(t, r.ArgvFile); calls != nil {
				t.Errorf("uninstall --release=%q json=%v ran helm: %v", name, js, calls)
			}
			if js {
				wantFailure(t, r, "k8s uninstall", "E401", "user")
			} else if r.Exit != 1 || strings.Count(r.Stderr, "Error:") != 1 {
				t.Errorf("human uninstall --release=%q: exit %d stderr %q", name, r.Exit, r.Stderr)
			}
		}
		// install, upgrade and status validate under --json only.
		dir := newProject(t)
		writeGenerated(t, dir)
		for _, args := range [][]string{
			{"status", "--json", "--release=" + name},
			{"install", "--json", "--domain", "x.test", "--project-dir", dir, "--release=" + name},
			{"upgrade", "--json", "--project-dir", dir, "--release=" + name},
		} {
			r := runCLI(t, cliRun{Helm: "ok"}, args...)
			wantFailure(t, r, "k8s "+args[0], "E401", "user")
			if calls := argvCalls(t, r.ArgvFile); calls != nil {
				t.Errorf("%v ran helm: %v", args, calls)
			}
		}
	}
	for _, name := range []string{"nself", "a", "my-nself-2", strings.Repeat("a", 53)} {
		if !validReleaseName(name) {
			t.Errorf("%q must be valid", name)
		}
	}
	// Human mode of status is unchanged: a name helm would judge itself still reaches helm.
	r := runCLI(t, cliRun{Helm: "ok"}, "status", "--release=Upper")
	if r.Exit != 0 || len(argvCalls(t, r.ArgvFile)) != 1 {
		t.Errorf("human status --release=Upper: exit %d, calls %v", r.Exit, argvCalls(t, r.ArgvFile))
	}
}

// TestStatusReleaseSwallowsJSON: `status --release --json` makes pflag read
// "--json" as the release, so the command runs in human mode. A release name
// starting with "-" would reach helm as a flag (`helm status --json`), so it is
// a usage error in human mode too and helm never runs (D-0376, K8S-02).
func TestStatusReleaseSwallowsJSON(t *testing.T) {
	for _, h := range []string{"fail", "ok"} {
		r := runCLI(t, cliRun{Helm: h}, "status", "--release", "--json")
		if r.Exit != 1 || r.Stdout != "" || !strings.HasPrefix(r.Stderr, "Error: --release must be a helm release name") || strings.Contains(r.Stderr, "E401") {
			t.Errorf("helm %s: exit %d, stdout %q, stderr %q", h, r.Exit, r.Stdout, r.Stderr)
		}
		if calls := argvCalls(t, r.ArgvFile); calls != nil {
			t.Errorf("helm %s: a dash release ran helm: %v", h, calls)
		}
	}
	// A real cobra usage error under --json is still an E401 envelope.
	wantFailure(t, runCLI(t, cliRun{Helm: "ok"}, "status", "--json", "--bogus"), "k8s status", "E401", "user")
}

func TestUninstallHumanFailurePrintsOnce(t *testing.T) {
	r := runCLI(t, cliRun{Helm: "fail"}, "uninstall", "--yes")
	if r.Exit != 1 || strings.Count(r.Stderr, "Error:") != 1 {
		t.Errorf("exit %d, stderr:\n%s", r.Exit, r.Stderr)
	}
}

func TestStatusFailureRemediationIsNotAboutStderr(t *testing.T) {
	st := wantFailure(t, runCLI(t, cliRun{Helm: "fail"}, "status", "--json"), "k8s status", "E722", "infra")
	if strings.Contains(st.Error.Remediation, "stderr") || !strings.Contains(st.Error.Remediation, "helm status") {
		t.Errorf("status remediation = %q", st.Error.Remediation)
	}
	un := wantFailure(t, runCLI(t, cliRun{Helm: "fail"}, "uninstall", "--yes", "--json"), "k8s uninstall", "E722", "infra")
	if !strings.Contains(un.Error.Remediation, "stderr") {
		t.Errorf("uninstall remediation = %q", un.Error.Remediation)
	}
}
