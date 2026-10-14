package config

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// copyFixture copies testdata/<dir>/input into a temp repo root, turning each
// "dot-ci.yaml" into ".ci.yaml" (a tracked "*.ci.yaml" file would itself be a
// leftover in this repo).
func copyFixture(t *testing.T, dir string) string {
	t.Helper()
	src := filepath.Join("testdata", dir, "input")
	root := t.TempDir()
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(root, rel), 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if filepath.Base(rel) == "dot-ci.yaml" {
			rel = filepath.Join(filepath.Dir(rel), ".ci.yaml")
		}
		return os.WriteFile(filepath.Join(root, rel), data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// TestMigrateGolden migrates every legacy fixture, compares the text with its
// golden file (UPDATE_GOLDEN=1 rewrites it) and proves each planned job runs
// the same argv with the same timeout and env condition as its legacy stage.
func TestMigrateGolden(t *testing.T) {
	cases, err := os.ReadDir(filepath.Join("testdata", "legacy"))
	if err != nil || len(cases) == 0 {
		t.Fatalf("no legacy fixtures: %v", err)
	}
	for _, c := range cases {
		name := filepath.Join("legacy", c.Name())
		t.Run(c.Name(), func(t *testing.T) {
			root := copyFixture(t, name)
			m, err := Migrate(root)
			if err != nil {
				t.Fatal(err)
			}
			golden := filepath.Join("testdata", name, "want.yaml")
			if os.Getenv("UPDATE_GOLDEN") == "1" {
				if err := os.WriteFile(golden, m.YAML, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatal(err)
			}
			if string(want) != string(m.YAML) {
				t.Fatalf("golden mismatch\n--- want\n%s\n--- got\n%s", want, m.YAML)
			}
			if _, err := Parse(FileName, m.YAML); err != nil {
				t.Fatalf("migrated text does not round-trip: %v", err)
			}
			assertSameJobs(t, root, m)
		})
	}
}

func assertSameJobs(t *testing.T, root string, m *Migration) {
	t.Helper()
	next := 0
	for _, rel := range m.Sources {
		data, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		lf, err := parseLegacy(rel, data)
		if err != nil {
			t.Fatal(err)
		}
		for _, st := range lf.Stages {
			job := m.Config.Jobs[m.Order[next]]
			next++
			if strings.Join(job.Run, "\x00") != strings.Join(st.Cmd, "\x00") {
				t.Errorf("%s: argv %v != %v", st.Name, job.Run, st.Cmd)
			}
			wantTimeout := ""
			if st.Timeout > 0 {
				wantTimeout = fmt.Sprintf("%ds", st.Timeout)
			}
			if job.Timeout != wantTimeout {
				t.Errorf("%s: timeout %q != %q", st.Name, job.Timeout, wantTimeout)
			}
			skips := st.SkipIfEnvMissing && len(st.EnvRequired) > 0
			var env []string
			if job.When != nil {
				env = job.When.EnvPresent
			}
			if skips && (strings.Join(env, ",") != strings.Join(st.EnvRequired, ",") || job.Required == nil || *job.Required) {
				t.Errorf("%s: env condition %v required=%v does not match %v", st.Name, env, job.Required, st.EnvRequired)
			}
			if !skips && (len(env) != 0 || job.Required != nil) {
				t.Errorf("%s: env condition invented: %v required=%v", st.Name, env, job.Required)
			}
		}
	}
	if next != len(m.Order) {
		t.Fatalf("%d jobs for %d stages", len(m.Order), next)
	}
}

func TestMigrateUntranslatable(t *testing.T) {
	cases := map[string]struct{ doc, field string }{
		"unknown top-level field": {"plugin: p\ncache: true\nstages: []\n", "cache"},
		"unknown stage field":     {"plugin: p\nstages:\n  - name: a\n    cmd: [x]\n    retries: 2\n", "retries"},
		"gateway routes":          {"plugin: p\nstages:\n  - name: a\n    cmd: [x]\ngateway_routes:\n  - POST /x\n", "gateway_routes"},
		"stage without cmd":       {"plugin: p\nstages:\n  - name: a\n", "stages[0].cmd"},
		"negative timeout":        {"plugin: p\nstages:\n  - name: a\n    cmd: [x]\n    timeout: -1\n", "stages[0].timeout"},
		"unreadable":              {"plugin: [\n", ""},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, ".ci.yaml"), []byte(c.doc), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := Migrate(root)
			e, ok := err.(*Error)
			if !ok || e.Code != CodeUntranslatable {
				t.Fatalf("want E622, got %v", err)
			}
			if !strings.Contains(e.Problems[0].Pointer, c.field) {
				t.Errorf("pointer %q does not name %q", e.Problems[0].Pointer, c.field)
			}
		})
	}
}

func TestMigrateNothingAndNoOverwrite(t *testing.T) {
	if _, err := Migrate(t.TempDir()); err != ErrNothingToMigrate {
		t.Fatalf("empty repo: %v", err)
	}
	root := copyFixture(t, filepath.Join("legacy", "gauth"))
	m, err := Migrate(root)
	if err != nil {
		t.Fatal(err)
	}
	if diff, _ := m.Diff(root); !strings.HasPrefix(diff, "--- /dev/null\n+++ b/.nself/ci.yaml\n") {
		t.Errorf("diff header: %q", diff)
	}
	if _, err := os.Stat(filepath.Join(root, FileName)); err == nil {
		t.Fatal("Migrate/Diff wrote the file without Write")
	}
	if err := m.Write(root); err != nil {
		t.Fatal(err)
	}
	if err := m.Write(root); err != nil {
		t.Fatalf("identical rewrite must succeed: %v", err)
	}
	if diff, _ := m.Diff(root); diff != "" {
		t.Errorf("diff after write: %q", diff)
	}
	if err := os.WriteFile(filepath.Join(root, FileName), []byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.Write(root); err == nil {
		t.Fatal("Write replaced a file with other content")
	}
}
