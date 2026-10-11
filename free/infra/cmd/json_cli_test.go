// Purpose: the --json contract of every infra subcommand: one valid v1
// envelope on stdout (success on mocked providers, an error with a code and an
// exit class on a failure fixture), a process exit status equal to the class,
// nothing else on stdout, and a golden file per case.
//
// Inputs: the in-process harness (cli_harness_test.go): a stub tofu on PATH, a
// fake server.Client, an overridden TTY answer.
//
// Outputs: test results. `go test ./cmd -run TestJSONEnvelope -update` rewrites
// testdata/json/*.golden.json.
//
// Constraints: no network, no cloud credentials, no spend. Envelopes are
// validated with output.CheckEnvelope from sdk/go/output.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/output"
	"github.com/nself-org/nself-infra/internal/server"
	"github.com/spf13/cobra"
)

var update = flag.Bool("update", false, "rewrite testdata/json/*.golden.json")

// envDoc is the decoded envelope.
type envDoc struct {
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
}

// fixtureServer is the server every fake client starts with.
func fixtureClient() *fakeServerClient {
	fc := newFakeServerClient()
	fc.servers[1] = &server.Server{ID: 1, Name: "web-1", Status: "running", ServerType: "cx22", Location: "fsn1", IPv4: "203.0.113.10"}
	fc.primaryIPs[10] = &server.PrimaryIP{ID: 10, IP: "203.0.113.10", Type: "ipv4", AssigneeID: 1, AutoDelete: true}
	return fc
}

const (
	tfDomain = "example.test"
	consent  = "--i-accept-cloud-spend"
)

type jsonCase struct {
	name    string // golden name
	cmd     string // expected envelope command
	args    []string
	env     cliEnv
	code    string // "" for a success envelope
	class   string
	wantLog string // substring the stub tofu log must hold ("" = no check)
}

func jsonCases() []jsonCase {
	tty := func(e cliEnv) cliEnv { e.TTY = true; return e }
	ok := tty(cliEnv{Tofu: "ok"})
	bad := tty(cliEnv{Tofu: "fail"})
	srv := cliEnv{Client: fixtureClient()}
	hetz500 := &errClient{fakeServerClient: fixtureClient(), list: errors.New("list servers: hetzner API GET /servers: 500 server_error: secret-looking text")}
	gone := &errClient{fakeServerClient: fixtureClient(), get: errors.New("get server 9: hetzner API GET /servers/9: 404 not_found: nope")}
	delFail := &errClient{fakeServerClient: fixtureClient(), del: errors.New("delete server 1: hetzner API DELETE /servers/1: 503 unavailable")}
	rejected := &errClient{fakeServerClient: fixtureClient(), list: errors.New("list servers: hetzner API GET /servers: 401 unauthorized: token text")}
	forbidden := &errClient{fakeServerClient: fixtureClient(), create: errors.New("create server: hetzner API POST /servers: 403 forbidden: read only")}
	shrink := fixtureClient()
	shrink.servers[1].ServerType = "cx41"
	provFail := &errClient{fakeServerClient: fixtureClient(), create: errors.New("create server: hetzner API POST /servers: 422 invalid_input: bad")}
	tf := []string{"--provider", "hetzner", "--domain", tfDomain}
	destroyArgs := []string{"destroy", "--provider", "hetzner", consent, "--auto-approve", "--json"}
	return []jsonCase{
		{name: "validate_ok", cmd: "infra validate", args: []string{"validate", "--json"}, env: ok, wantLog: "validate"},
		{name: "validate_unknown_provider", cmd: "infra validate", args: []string{"validate", "--provider", "aws", "--json"}, env: ok, code: CodeUsage, class: output.ClassUser},
		{name: "validate_tofu_fails", cmd: "infra validate", args: []string{"validate", "--json"}, env: bad, code: CodeTerraformFail, class: output.ClassInfra},
		{name: "validate_no_tofu", cmd: "infra validate", args: []string{"validate", "--json"}, env: tty(cliEnv{}), code: CodeTerraformGone, class: output.ClassInfra},
		{name: "plan_ok", cmd: "infra plan", args: append([]string{"plan"}, append(tf, "--json")...), env: ok, wantLog: "plan"},
		{name: "plan_missing_domain", cmd: "infra plan", args: []string{"plan", "--provider", "hetzner", "--json"}, env: ok, code: CodeUsage, class: output.ClassUser},
		{name: "plan_tofu_fails", cmd: "infra plan", args: append([]string{"plan"}, append(tf, "--json")...), env: bad, code: CodeTerraformFail, class: output.ClassInfra},
		{name: "plan_unknown_flag", cmd: "infra plan", args: []string{"plan", "--nope", "--json"}, env: ok, code: CodeUsage, class: output.ClassUser},
		{name: "apply_ok", cmd: "infra apply", args: append([]string{"apply"}, append(tf, consent, "--auto-approve", "--json")...), env: ok, wantLog: "apply"},
		{name: "apply_no_spend_flag", cmd: "infra apply", args: append([]string{"apply"}, append(tf, "--json")...), env: ok, code: CodeBlocked, class: output.ClassDestructiveBlocked},
		{name: "apply_no_tty", cmd: "infra apply", args: append([]string{"apply"}, append(tf, consent, "--json")...), env: cliEnv{Tofu: "ok"}, code: CodeBlocked, class: output.ClassDestructiveBlocked},
		{name: "apply_unshipped_provider", cmd: "infra apply", args: []string{"apply", "--provider", "aws", "--domain", tfDomain, consent, "--json"}, env: ok, code: CodeUsage, class: output.ClassUser},
		{name: "apply_tofu_fails", cmd: "infra apply", args: append([]string{"apply"}, append(tf, consent, "--auto-approve", "--json")...), env: bad, code: CodeTerraformFail, class: output.ClassInfra},
		{name: "destroy_ok", cmd: "infra destroy", args: destroyArgs, env: ok, wantLog: "destroy"},
		{name: "destroy_no_spend_flag", cmd: "infra destroy", args: []string{"destroy", "--provider", "hetzner", "--auto-approve", "--json"}, env: ok, code: CodeBlocked, class: output.ClassDestructiveBlocked},
		{name: "destroy_no_tty", cmd: "infra destroy", args: destroyArgs, env: cliEnv{Tofu: "ok"}, code: CodeBlocked, class: output.ClassDestructiveBlocked},
		{name: "destroy_no_auto_approve", cmd: "infra destroy", args: []string{"destroy", "--provider", "hetzner", consent, "--json"}, env: ok, code: CodeBlocked, class: output.ClassDestructiveBlocked},
		{name: "destroy_tofu_fails", cmd: "infra destroy", args: destroyArgs, env: bad, code: CodeTerraformFail, class: output.ClassInfra},
		{name: "server_list_ok", cmd: "infra server list", args: []string{"server", "list", "--json"}, env: srv},
		{name: "server_list_hetzner_error", cmd: "infra server list", args: []string{"server", "list", "--json"}, env: cliEnv{Client: hetz500}, code: CodeHetznerFailed, class: output.ClassInfra},
		{name: "server_list_token_rejected", cmd: "infra server list", args: []string{"server", "list", "--json"}, env: cliEnv{Client: rejected}, code: CodeTokenRejected, class: output.ClassAuth},
		{name: "server_provision_forbidden", cmd: "infra server provision", args: []string{"server", "provision", "--name", "web-2", "--type", "cx22", "--location", "fsn1", "--image", "ubuntu-24.04", "--json"}, env: cliEnv{Client: forbidden}, code: CodeTokenRejected, class: output.ClassAuth},
		{name: "server_list_unexpected", cmd: "infra server list", args: []string{"server", "list", "--json"}, env: cliEnv{Client: &errClient{fakeServerClient: fixtureClient(), list: errors.New("boom")}}, code: CodeUnexpected, class: output.ClassInfra},
		{name: "server_list_no_token", cmd: "infra server list", args: []string{"server", "list", "--json"}, env: cliEnv{NoTok: true, realClient: true}, code: CodeNoToken, class: output.ClassAuth},
		{name: "server_list_token_env_refused", cmd: "infra server list", args: []string{"server", "list", "--token-env", TerraformTokenEnvVar, "--json"}, env: cliEnv{realClient: true}, code: CodeTokenEnvBanned, class: output.ClassUser},
		{name: "server_provision_ok", cmd: "infra server provision", args: []string{"server", "provision", "--name", "web-2", "--type", "cx22", "--location", "fsn1", "--image", "ubuntu-24.04", "--label", "purpose=ci", "--json"}, env: srv},
		{name: "server_provision_missing_name", cmd: "infra server provision", args: []string{"server", "provision", "--type", "cx22", "--location", "fsn1", "--image", "ubuntu-24.04", "--json"}, env: srv, code: CodeUsage, class: output.ClassUser},
		{name: "server_provision_hetzner_error", cmd: "infra server provision", args: []string{"server", "provision", "--name", "web-2", "--type", "cx22", "--location", "fsn1", "--image", "ubuntu-24.04", "--json"}, env: cliEnv{Client: provFail}, code: CodeHetznerFailed, class: output.ClassInfra},
		{name: "server_resize_ok", cmd: "infra server resize", args: []string{"server", "resize", "--id", "1", "--type", "cx41", "--json"}, env: srv},
		{name: "server_resize_disk_shrink", cmd: "infra server resize", args: []string{"server", "resize", "--id", "1", "--type", "cx22", "--json"}, env: cliEnv{Client: shrink}, code: CodeDiskShrink, class: output.ClassUser},
		{name: "server_resize_not_found", cmd: "infra server resize", args: []string{"server", "resize", "--id", "9", "--type", "cx41", "--json"}, env: cliEnv{Client: gone}, code: CodeServerNotFound, class: output.ClassUser},
		{name: "server_destroy_ok", cmd: "infra server destroy", args: []string{"server", "destroy", "--id", "1", "--snapshot", "--json"}, env: srv},
		{name: "server_destroy_no_backup", cmd: "infra server destroy", args: []string{"server", "destroy", "--id", "1", "--json"}, env: srv, code: CodeBlocked, class: output.ClassDestructiveBlocked},
		{name: "server_destroy_hetzner_error", cmd: "infra server destroy", args: []string{"server", "destroy", "--id", "1", "--force-no-backup", "--json"}, env: cliEnv{Client: delFail}, code: CodeHetznerFailed, class: output.ClassInfra},
	}
}

func TestJSONEnvelope(t *testing.T) {
	okSeen, errSeen := map[string]bool{}, map[string]bool{}
	for _, tc := range jsonCases() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if tc.env.Client == nil && !tc.env.realClient {
				tc.env.Client = fixtureClient()
			}
			r := runCLI(t, tc.env, tc.args...)
			if err := output.CheckEnvelope([]byte(r.Stdout)); err != nil {
				t.Fatalf("stdout is not exactly one valid envelope: %v\nstdout:\n%s\nstderr:\n%s", err, r.Stdout, r.Stderr)
			}
			var d envDoc
			if err := json.Unmarshal([]byte(r.Stdout), &d); err != nil {
				t.Fatal(err)
			}
			if d.Command != tc.cmd {
				t.Errorf("command = %q, want %q", d.Command, tc.cmd)
			}
			if tc.code == "" {
				okSeen[tc.cmd] = true
				if d.Error != nil || r.Exit != 0 {
					t.Fatalf("want success, exit=%d\n%s\nstderr:\n%s", r.Exit, r.Stdout, r.Stderr)
				}
			} else {
				errSeen[tc.cmd] = true
				if d.Error == nil || d.Error.Code != tc.code || d.Error.Class != tc.class {
					t.Fatalf("want error %s/%s, got:\n%s", tc.code, tc.class, r.Stdout)
				}
				if want := output.ExitCodeFor(tc.class); r.Exit != want || d.Error.ExitCode != want {
					t.Errorf("exit = %d, envelope exit_code = %d, want %d", r.Exit, d.Error.ExitCode, want)
				}
			}
			if tc.wantLog != "" && !strings.Contains(r.TofuLog, tc.wantLog) {
				t.Errorf("stub tofu log %q lacks %q", r.TofuLog, tc.wantLog)
			}
			for _, secret := range []string{fakeToken, "secret-looking text", "stub-tofu"} {
				if strings.Contains(r.Stdout, secret) {
					t.Errorf("stdout leaks %q:\n%s", secret, r.Stdout)
				}
			}
			checkGolden(t, tc.name, r.Stdout)
		})
	}
	t.Run("every json subcommand is covered", func(t *testing.T) {
		walkLeaves(infraCmd, func(c *cobra.Command) {
			path := c.CommandPath()
			if c.Flags().Lookup("json") == nil {
				t.Errorf("%s has no --json flag", path)
			}
			if !okSeen[path] || !errSeen[path] {
				t.Errorf("%s: success covered=%v, error covered=%v", path, okSeen[path], errSeen[path])
			}
		})
	})
}

// walkLeaves calls fn for every runnable command that does only real work
// (the root and the `server` group print help and take no --json).
func walkLeaves(c *cobra.Command, fn func(*cobra.Command)) {
	if c.Hidden {
		return
	}
	if len(c.Commands()) == 0 {
		fn(c)
		return
	}
	for _, sub := range c.Commands() {
		walkLeaves(sub, fn)
	}
}

// checkGolden compares stdout with testdata/json/<name>.golden.json.
func checkGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("..", "testdata", "json", name+".golden.json")
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("missing golden (run with -update): %v", err)
	}
	if string(want) != got {
		t.Errorf("stdout differs from %s\nwant:\n%s\ngot:\n%s", path, want, got)
	}
}

// Every committed golden is itself a valid envelope.
func TestJSONGoldensAreValid(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "testdata", "json", "*.golden.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no goldens found (%v)", err)
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if err := output.CheckEnvelope(b); err != nil {
			t.Errorf("%s: %v", filepath.Base(f), err)
		}
	}
}
