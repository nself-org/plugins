// Purpose: the --json contract of every subcommand, through the built binary:
// one valid v1 envelope on stdout, an error code and class for each forced
// failure, and a process exit status equal to the class.
//
// Inputs: the binary and stubs from cli_harness_test.go.
//
// Outputs: test results.
//
// Constraints: no network, no cluster. Envelopes are validated with
// output.CheckEnvelope from sdk/go/output.
package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/output"
)

// doc is the decoded envelope.
type doc struct {
	Command string         `json:"command"`
	Data    map[string]any `json:"data"`
	Error   *struct {
		Code        string `json:"code"`
		Message     string `json:"message"`
		Cause       string `json:"cause"`
		Remediation string `json:"remediation"`
		ExitCode    int    `json:"exit_code"`
		Class       string `json:"class"`
	} `json:"error"`
	Meta struct {
		Warnings []string `json:"warnings"`
	} `json:"meta"`
}

// mustEnvelope validates stdout as exactly one envelope and decodes it.
func mustEnvelope(t *testing.T, r cliResult, command string) doc {
	t.Helper()
	if err := output.CheckEnvelope([]byte(r.Stdout)); err != nil {
		t.Fatalf("stdout is not one valid envelope: %v\nstdout:\n%s\nstderr:\n%s", err, r.Stdout, r.Stderr)
	}
	var d doc
	if err := json.Unmarshal([]byte(r.Stdout), &d); err != nil {
		t.Fatal(err)
	}
	if d.Command != command {
		t.Errorf("command = %q, want %q", d.Command, command)
	}
	return d
}

// wantFailure asserts an error envelope with code, class and a matching exit status.
func wantFailure(t *testing.T, r cliResult, command, code, class string) doc {
	t.Helper()
	d := mustEnvelope(t, r, command)
	if d.Error == nil {
		t.Fatalf("want an error envelope, got:\n%s", r.Stdout)
	}
	if d.Error.Code != code || d.Error.Class != class {
		t.Errorf("error = %s/%s, want %s/%s\n%s", d.Error.Code, d.Error.Class, code, class, r.Stdout)
	}
	if want := output.ExitCodeFor(class); r.Exit != want || d.Error.ExitCode != want {
		t.Errorf("exit = %d, envelope exit_code = %d, want %d", r.Exit, d.Error.ExitCode, want)
	}
	return d
}

func wantSuccess(t *testing.T, r cliResult, command string) doc {
	t.Helper()
	d := mustEnvelope(t, r, command)
	if d.Error != nil || r.Exit != 0 {
		t.Fatalf("want success, exit=%d\n%s\nstderr:\n%s", r.Exit, r.Stdout, r.Stderr)
	}
	return d
}

func TestJSONValuesSuccess(t *testing.T) {
	dir := newProject(t)
	r := runCLI(t, cliRun{Docker: "ok"}, "values", "--json", "--project-dir", dir)
	d := wantSuccess(t, r, "k8s values")
	if d.Data["mode"] != "generate" || d.Data["mapped"].(float64) != 9 {
		t.Errorf("data = %v", d.Data)
	}
	if u, _ := d.Data["unsupported"].([]any); len(u) != 2 {
		t.Errorf("unsupported = %v, want 2 entries", d.Data["unsupported"])
	}
	if strings.Contains(r.Stdout, "fixture-") {
		t.Error("envelope leaks a secret value")
	}
	chk := runCLI(t, cliRun{Docker: "ok"}, "values", "--json", "--check", "--project-dir", dir)
	c := wantSuccess(t, chk, "k8s values")
	if c.Data["mode"] != "check" || c.Data["parity"] != "ok" || c.Data["services"].(float64) < 1 {
		t.Errorf("check data = %v", c.Data)
	}
}

func TestJSONValuesFailures(t *testing.T) {
	dir := newProject(t)
	wantFailure(t, runCLI(t, cliRun{Docker: "fail"}, "values", "--json", "--project-dir", dir), "k8s values", "E724", "infra")
	wantFailure(t, runCLI(t, cliRun{}, "values", "--json", "--project-dir", dir), "k8s values", "E724", "infra")
	wantFailure(t, runCLI(t, cliRun{Docker: "ok"}, "values", "--json", "--check", "--project-dir", dir), "k8s values", "E721", "user")
	if r := runCLI(t, cliRun{Docker: "ok"}, "values", "--project-dir", dir); r.Exit != 0 {
		t.Fatal(r.Stderr)
	}
	vf := dir + "/.nself/generated/k8s/values.yaml"
	b, _ := os.ReadFile(vf)
	writeFile(t, vf, strings.Replace(string(b), "tag: v2.44.0", "tag: v1.0.0", 1), 0o644)
	d := wantFailure(t, runCLI(t, cliRun{Docker: "ok"}, "values", "--json", "--check", "--project-dir", dir), "k8s values", "E723", "user")
	if !strings.Contains(d.Error.Cause, "hasura: differs") {
		t.Errorf("cause = %q, want the differing service", d.Error.Cause)
	}
}

func TestJSONInstall(t *testing.T) {
	dir := newProject(t)
	writeGenerated(t, dir)
	r := runCLI(t, cliRun{Helm: "ok", Env: []string{"NSELF_PLUGIN_LICENSE_KEY=json-key-value"}},
		"install", "--json", "--domain", "app.example.test", "--release", "r1", "--project-dir", dir)
	d := wantSuccess(t, r, "k8s install")
	if d.Data["release"] != "r1" || d.Data["installed"] != true {
		t.Errorf("data = %v", d.Data)
	}
	if h, _ := d.Data["ingress_hosts"].([]any); len(h) != 1 || h[0] != "api.example.test" {
		t.Errorf("ingress_hosts = %v", d.Data["ingress_hosts"])
	}
	if len(d.Meta.Warnings) != 1 || !strings.Contains(d.Meta.Warnings[0], "NSELF_PLUGIN_LICENSE_KEY") {
		t.Errorf("warnings = %v, want the unused-licence warning", d.Meta.Warnings)
	}
	if strings.Contains(r.Stdout+r.Stderr, "json-key-value") {
		t.Error("the licence key value leaked")
	}
	if strings.Contains(r.Stdout, "STUB-HELM") || !strings.Contains(r.Stderr, "STUB-HELM install stdout") {
		t.Errorf("helm stdout must go to stderr:\nstdout:\n%s\nstderr:\n%s", r.Stdout, r.Stderr)
	}
	if strings.Contains(r.Stdout, "ℹ") || strings.Contains(r.Stdout, "✓") {
		t.Error("tui lines must not reach stdout")
	}
}

func TestJSONInstallFailures(t *testing.T) {
	dir := newProject(t)
	args := []string{"install", "--json", "--domain", "x.test", "--project-dir", dir}
	wantFailure(t, runCLI(t, cliRun{Helm: "ok"}, args...), "k8s install", "E721", "user")
	writeGenerated(t, dir)
	wantFailure(t, runCLI(t, cliRun{Helm: "ok"}, "install", "--json", "--project-dir", dir), "k8s install", "E401", "user")
	wantFailure(t, runCLI(t, cliRun{}, args...), "k8s install", "E720", "infra")
	d := wantFailure(t, runCLI(t, cliRun{Helm: "fail"}, args...), "k8s install", "E722", "infra")
	if d.Error.Cause != "exit status 1" || strings.Contains(r2(d), "stub helm") {
		t.Errorf("cause = %q, message = %q", d.Error.Cause, d.Error.Message)
	}
	wantFailure(t, runCLI(t, cliRun{Helm: "ok"}, append(args, "extra")...), "k8s install", "E401", "user")
	// A secrets file others can read is a values problem, not a helm one.
	if err := os.Chmod(dir+"/.nself/generated/k8s/secrets.yaml", 0o644); err != nil {
		t.Fatal(err)
	}
	wantFailure(t, runCLI(t, cliRun{Helm: "ok"}, args...), "k8s install", "E727", "user")
}

func r2(d doc) string { return d.Error.Message + d.Error.Cause + d.Error.Remediation }

func TestJSONUpgrade(t *testing.T) {
	dir := newProject(t)
	args := []string{"upgrade", "--json", "--project-dir", dir}
	wantFailure(t, runCLI(t, cliRun{Helm: "ok"}, args...), "k8s upgrade", "E721", "user")
	writeGenerated(t, dir)
	d := wantSuccess(t, runCLI(t, cliRun{Helm: "ok"}, append(args, "--domain", "x.test")...), "k8s upgrade")
	if d.Data["upgraded"] != true || d.Data["release"] != "nself" {
		t.Errorf("data = %v", d.Data)
	}
	if len(d.Meta.Warnings) == 0 || !strings.Contains(d.Meta.Warnings[0], "Not passing") {
		t.Errorf("warnings = %v, want the not-passed warning", d.Meta.Warnings)
	}
	wantFailure(t, runCLI(t, cliRun{Helm: "fail"}, args...), "k8s upgrade", "E722", "infra")
	wantFailure(t, runCLI(t, cliRun{}, args...), "k8s upgrade", "E720", "infra")
}

func TestJSONStatus(t *testing.T) {
	r := runCLI(t, cliRun{Helm: "ok"}, "status", "--json")
	d := wantSuccess(t, r, "k8s status")
	want := map[string]any{"name": "nself", "namespace": "default", "version": float64(3), "chart_version": "0.1.0", "status": "deployed"}
	if len(d.Data) != len(want) {
		t.Errorf("data has %d keys, want exactly %d: %v", len(d.Data), len(want), d.Data)
	}
	for k, v := range want {
		if d.Data[k] != v {
			t.Errorf("data[%s] = %v, want %v", k, d.Data[k], v)
		}
	}
	if strings.Contains(r.Stdout, "SHOULD-NEVER-PRINT") || strings.Contains(r.Stdout, "kind: Secret") {
		t.Error("helm's release JSON leaked into the envelope")
	}
	wantFailure(t, runCLI(t, cliRun{Helm: "fail"}, "status", "--json"), "k8s status", "E722", "infra")
	wantFailure(t, runCLI(t, cliRun{Helm: "notfound"}, "status", "--json"), "k8s status", "E725", "user")
	wantFailure(t, runCLI(t, cliRun{Helm: "ok", StatusDoc: "not json"}, "status", "--json"), "k8s status", "E726", "infra")
	wantFailure(t, runCLI(t, cliRun{}, "status", "--json"), "k8s status", "E720", "infra")
}

func TestJSONUsageErrors(t *testing.T) {
	d := wantFailure(t, runCLI(t, cliRun{Helm: "ok"}, "status", "--json", "--bogus"), "k8s status", "E401", "user")
	if !strings.Contains(d.Error.Message, "bogus") {
		t.Errorf("message = %q", d.Error.Message)
	}
	wantFailure(t, runCLI(t, cliRun{Helm: "ok"}, "values", "--json", "extra"), "k8s values", "E401", "user")
	wantFailure(t, runCLI(t, cliRun{Helm: "ok"}, "nope", "--json"), "k8s", "E401", "user")
	wantFailure(t, runCLI(t, cliRun{Helm: "ok"}, "--json"), "k8s", "E401", "user")
}

func TestJSONUninstall(t *testing.T) {
	// Refused: no --yes, stdin is not a terminal. No helm command runs.
	r := runCLI(t, cliRun{Helm: "ok"}, "uninstall", "--json")
	d := wantFailure(t, r, "k8s uninstall", "E403", "destructive_blocked")
	if r.Exit != 4 || !strings.Contains(d.Error.Remediation, "--yes") {
		t.Errorf("exit = %d, remediation = %q", r.Exit, d.Error.Remediation)
	}
	if calls := argvCalls(t, r.ArgvFile); calls != nil {
		t.Fatalf("a refused uninstall ran helm: %v", calls)
	}
	// Confirmed.
	r = runCLI(t, cliRun{Helm: "ok"}, "uninstall", "--json", "--yes", "--release", "r1", "--cluster", "/k/cfg")
	d = wantSuccess(t, r, "k8s uninstall")
	if d.Data["release"] != "r1" || d.Data["uninstalled"] != true {
		t.Errorf("data = %v", d.Data)
	}
	calls := argvCalls(t, r.ArgvFile)
	if len(calls) != 1 || strings.Join(calls[0], " ") != "uninstall r1 --kubeconfig /k/cfg" {
		t.Errorf("helm argv = %v", calls)
	}
	if strings.Contains(r.Stdout, "STUB-HELM") {
		t.Error("helm stdout reached stdout")
	}
	wantFailure(t, runCLI(t, cliRun{Helm: "fail"}, "uninstall", "--json", "--yes"), "k8s uninstall", "E722", "infra")
	wantFailure(t, runCLI(t, cliRun{Helm: "notfound"}, "uninstall", "--json", "--yes"), "k8s uninstall", "E725", "user")
	wantFailure(t, runCLI(t, cliRun{}, "uninstall", "--json", "--yes"), "k8s uninstall", "E720", "infra")
	wantFailure(t, runCLI(t, cliRun{Helm: "ok"}, "uninstall", "--json", "--yes", "extra"), "k8s uninstall", "E401", "user")
}

// TestUninstallHumanRefusal: without --yes and without a terminal the human
// mode refuses too, exits 4 and runs no helm command.
func TestUninstallHumanRefusal(t *testing.T) {
	r := runCLI(t, cliRun{Helm: "ok"}, "uninstall")
	if r.Exit != 4 || r.Stdout != "" || !strings.HasPrefix(r.Stderr, "Error: uninstall of release \"nself\" needs confirmation") {
		t.Errorf("exit = %d\nstdout:\n%s\nstderr:\n%s", r.Exit, r.Stdout, r.Stderr)
	}
	if strings.Count(r.Stderr, "Error:") != 1 {
		t.Errorf("the error must print once:\n%s", r.Stderr)
	}
	if calls := argvCalls(t, r.ArgvFile); calls != nil {
		t.Fatalf("a refused uninstall ran helm: %v", calls)
	}
	ok := runCLI(t, cliRun{Helm: "ok"}, "uninstall", "--yes")
	if ok.Exit != 0 || !strings.Contains(ok.Stdout, "STUB-HELM uninstall stdout") || !strings.Contains(ok.Stdout, "uninstalled") {
		t.Errorf("human --yes: exit %d\n%s\n%s", ok.Exit, ok.Stdout, ok.Stderr)
	}
}

func TestConfirmRelease(t *testing.T) {
	var out strings.Builder
	if !confirmRelease(strings.NewReader("nself\n"), &out, "nself") {
		t.Error("the release name must confirm")
	}
	for _, in := range []string{"", "\n", "yes\n", "nselfx\n"} {
		if confirmRelease(strings.NewReader(in), &out, "nself") {
			t.Errorf("%q must not confirm", in)
		}
	}
}
