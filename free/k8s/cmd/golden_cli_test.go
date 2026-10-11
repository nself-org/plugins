// Purpose: golden tests that pin the human (non --json) output of values,
// install, upgrade and status: stdout, stderr and exit status, byte for byte,
// for a success and the failure paths of each subcommand.
//
// Inputs: the built binary (cli_harness_test.go), testdata/golden/*.golden.
//
// Outputs: test results. UPDATE_GOLDEN=1 rewrites the goldens.
//
// Constraints: captured against the code before the JSON work (P7-DEPL-07);
// they must stay green unchanged. The project dir is replaced by <DIR> in the
// captured text. The environment is explicit, so colours are on (NO_COLOR unset).
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type goldenCase struct {
	name    string
	run     cliRun
	args    func(dir string) []string
	prepare func(t *testing.T, dir string) // before the run
	setup   string                         // "" project with manifests; "generated" adds values files
}

func goldenCases() []goldenCase {
	proj := func(dir string) string { return dir }
	return []goldenCase{
		{name: "values_generate", run: cliRun{Docker: "ok"},
			args: func(d string) []string { return []string{"values", "--project-dir", proj(d)} }},
		{name: "values_check_ok", run: cliRun{Docker: "ok"},
			args: func(d string) []string { return []string{"values", "--check", "--project-dir", d} },
			prepare: func(t *testing.T, d string) {
				r := runCLI(t, cliRun{Docker: "ok"}, "values", "--project-dir", d)
				if r.Exit != 0 {
					t.Fatalf("seed values: %s", r.Stderr)
				}
			}},
		{name: "values_check_differs", run: cliRun{Docker: "ok"},
			args: func(d string) []string { return []string{"values", "--check", "--project-dir", d} },
			prepare: func(t *testing.T, d string) {
				r := runCLI(t, cliRun{Docker: "ok"}, "values", "--project-dir", d)
				if r.Exit != 0 {
					t.Fatalf("seed values: %s", r.Stderr)
				}
				p := filepath.Join(d, ".nself", "generated", "k8s", "values.yaml")
				b, _ := os.ReadFile(p)
				edited := strings.Replace(string(b), "tag: v2.44.0", "tag: v1.0.0", 1)
				if edited == string(b) {
					t.Fatal("fixture no longer carries hasura v2.44.0")
				}
				writeFile(t, p, edited, 0o644)
			}},
		{name: "values_check_no_file", run: cliRun{Docker: "ok"},
			args: func(d string) []string { return []string{"values", "--check", "--project-dir", d} }},
		{name: "values_compose_fails", run: cliRun{Docker: "fail"},
			args: func(d string) []string { return []string{"values", "--project-dir", d} }},
		{name: "values_no_docker", run: cliRun{},
			args: func(d string) []string { return []string{"values", "--project-dir", d} }},
		{name: "install_ok", run: cliRun{Helm: "ok", Env: []string{"NSELF_PLUGIN_LICENSE_KEY=golden-key-value"}}, setup: "generated",
			args: func(d string) []string {
				return []string{"install", "--domain", "app.example.test", "--plugins", "a,b", "--project-dir", d}
			}},
		{name: "install_ok_cluster", run: cliRun{Helm: "ok"}, setup: "generated",
			args: func(d string) []string {
				return []string{"install", "--domain", "app.example.test", "--cluster", "/k/cfg", "--release", "r1", "--project-dir", d}
			}},
		{name: "install_no_domain", run: cliRun{Helm: "ok"}, setup: "generated",
			args: func(d string) []string { return []string{"install", "--project-dir", d} }},
		{name: "install_no_values", run: cliRun{Helm: "ok"},
			args: func(d string) []string { return []string{"install", "--domain", "x.test", "--project-dir", d} }},
		{name: "install_no_helm", run: cliRun{}, setup: "generated",
			args: func(d string) []string { return []string{"install", "--domain", "x.test", "--project-dir", d} }},
		{name: "install_helm_fails", run: cliRun{Helm: "fail"}, setup: "generated",
			args: func(d string) []string { return []string{"install", "--domain", "x.test", "--project-dir", d} }},
		{name: "upgrade_ok", run: cliRun{Helm: "ok"}, setup: "generated",
			args: func(d string) []string { return []string{"upgrade", "--project-dir", d} }},
		{name: "upgrade_ok_domain", run: cliRun{Helm: "ok"}, setup: "generated",
			args: func(d string) []string {
				return []string{"upgrade", "--domain", "x.test", "--plugins", "a", "--project-dir", d}
			}},
		{name: "upgrade_no_values", run: cliRun{Helm: "ok"},
			args: func(d string) []string { return []string{"upgrade", "--project-dir", d} }},
		{name: "upgrade_helm_fails", run: cliRun{Helm: "fail"}, setup: "generated",
			args: func(d string) []string { return []string{"upgrade", "--project-dir", d} }},
		{name: "status_ok", run: cliRun{Helm: "ok"},
			args: func(d string) []string { return []string{"status", "--release", "nself"} }},
		{name: "status_cluster", run: cliRun{Helm: "ok"},
			args: func(d string) []string { return []string{"status", "--cluster", "/k/cfg"} }},
		{name: "status_helm_fails", run: cliRun{Helm: "fail"},
			args: func(d string) []string { return []string{"status"} }},
		{name: "status_not_json", run: cliRun{Helm: "ok", StatusDoc: "not json at all"},
			args: func(d string) []string { return []string{"status"} }},
		{name: "status_no_helm", run: cliRun{},
			args: func(d string) []string { return []string{"status"} }},
	}
}

func TestHumanOutputGolden(t *testing.T) {
	for _, c := range goldenCases() {
		t.Run(c.name, func(t *testing.T) {
			dir := newProject(t)
			if c.setup == "generated" {
				writeGenerated(t, dir)
			}
			if c.prepare != nil {
				c.prepare(t, dir)
			}
			r := runCLI(t, c.run, c.args(dir)...)
			got := fmt.Sprintf("exit: %d\n--- stdout ---\n%s--- stderr ---\n%s", r.Exit,
				strings.ReplaceAll(r.Stdout, dir, "<DIR>"), strings.ReplaceAll(r.Stderr, dir, "<DIR>"))
			if strings.Contains(got, "SHOULD-NEVER-PRINT") || strings.Contains(got, "golden-key-value") {
				t.Fatalf("output leaks a secret:\n%s", got)
			}
			path := filepath.Join("testdata", "golden", c.name+".golden")
			if os.Getenv("UPDATE_GOLDEN") == "1" {
				writeFile(t, path, got, 0o644)
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("golden %s: %v (UPDATE_GOLDEN=1 to create)", path, err)
			}
			if string(want) != got {
				t.Errorf("human output changed for %s\nwant:\n%s\ngot:\n%s", c.name, want, got)
			}
		})
	}
}
