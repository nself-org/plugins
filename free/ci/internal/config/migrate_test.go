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
			wantTimeout := fmt.Sprintf("%ds", legacyDefaultTimeout)
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
		"second document":         {"plugin: p\nstages:\n  - name: a\n    cmd: [x]\n---\nplugin: q\n", "---"},
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

func TestMigrateNotes(t *testing.T) {
	m, err := Migrate(copyFixture(t, filepath.Join("legacy", "gauth")))
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(m.Notes, "\n")
	if !strings.Contains(joined, "read no cmd") || !strings.Contains(joined, "ignored timeout 180") || !strings.Contains(joined, "never skipped") || !strings.Contains(joined, "independent") {
		t.Errorf("notes: %v", m.Notes)
	}
}

func TestWriteRefusesSymlinks(t *testing.T) {
	root := copyFixture(t, filepath.Join("legacy", "gauth"))
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, ".nself")); err != nil {
		t.Fatal(err)
	}
	m, err := Migrate(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Write(root); err == nil {
		t.Fatal("wrote through a symlinked .nself")
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Fatalf("file created outside the repo: %v", entries)
	}
}

func TestMigrateNotesForFlowStages(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".ci.yaml"), []byte("plugin: p\nstages: [{name: test, cmd: [echo, ok]}]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := Migrate(root)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(m.Notes, "\n"), "found 0 stage") {
		t.Errorf("flow-style stages not noted: %v", m.Notes)
	}
}

func TestMigrateDestinationReadsAreBounded(t *testing.T) {
	root := copyFixture(t, filepath.Join("legacy", "gauth"))
	if err := os.MkdirAll(filepath.Join(root, ".nself"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/dev/zero", filepath.Join(root, FileName)); err != nil {
		t.Fatal(err)
	}
	m, err := Migrate(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Diff(root); err == nil {
		t.Error("Diff read a device through the destination symlink")
	}
	if err := m.Write(root); err == nil {
		t.Error("Write accepted a device destination")
	}
}

// blockManifest is valid YAML the legacy line scanner reads differently from
// what it declares: timeout and skip_if_env_missing at indent 4 are never read,
// and an all-digit cmd item is filed under env_required.
const blockManifest = "plugin: plugin-px\nstages:\n  - name: slow\n    cmd:\n    - sh\n    - -c\n    - sleep 3\n    timeout: 1\n  - name: needsenv\n    cmd:\n    - sh\n    - -c\n    - exit 7\n    env_required:\n    - ZZ_UNSET_VAR\n    skip_if_env_missing: true\n  - name: digits\n    cmd:\n    - sleep\n    - 2\n"

func TestMigrateNamesEveryLegacyDivergence(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "free", "px"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "free", "px", ".ci.yaml"), []byte(blockManifest), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := Migrate(root)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(m.Notes, "\n")
	for _, want := range []string{"stage slow: the legacy runner ignored timeout 1", "stage needsenv: the legacy runner never skipped", "stage digits: the legacy runner read cmd as 1 element(s) instead of 2"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing note %q in:\n%s", want, joined)
		}
	}
}

func TestMigrateRefusesNullItemsAndBadIds(t *testing.T) {
	for name, c := range map[string]struct{ doc, field string }{
		"null cmd item":   {"plugin: p\nstages:\n  - name: a\n    cmd:\n    - echo\n    - null\n", "stages[0].cmd[1]"},
		"nested cmd item": {"plugin: p\nstages:\n  - name: a\n    cmd: [echo, [x]]\n", ""},
		"merge key":       {"plugin: p\nstages:\n  - name: a\n    <<: {cmd: [echo, null, tail]}\n", ""},
		"anchor":          {"plugin: p\nstages:\n  - name: a\n    cmd: &c [echo, hi]\n", ""},
		"id too long":     {"plugin: p\nstages:\n  - name: " + strings.Repeat("x", 140) + "\n    cmd: [x]\n", "stages[0].name"},
	} {
		root := t.TempDir()
		_ = os.WriteFile(filepath.Join(root, ".ci.yaml"), []byte(c.doc), 0o644)
		_, err := Migrate(root)
		e, ok := err.(*Error)
		if !ok || e.Code != CodeUntranslatable || !strings.Contains(e.Problems[0].Pointer, c.field) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestMigrateErrorsDoNotEchoValues(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, ".ci.yaml"), []byte("plugin: p\nstages:\n  - name: t\n    cmd: [x]\n    timeout: hunter2\n"), 0o644)
	_, err := Migrate(root)
	if err == nil || strings.Contains(err.Error(), "hunter2") || err.(*Error).Problems[0].Line == 0 {
		t.Fatalf("value echoed or no line: %v", err)
	}
}

func TestDiffRefusesSymlinkedDestination(t *testing.T) {
	root := copyFixture(t, filepath.Join("legacy", "gauth"))
	outside := filepath.Join(t.TempDir(), "secret.env")
	_ = os.WriteFile(outside, []byte("TOKEN=ghp_FAKE_OUTSIDE\n"), 0o644)
	_ = os.MkdirAll(filepath.Join(root, ".nself"), 0o755)
	if err := os.Symlink(outside, filepath.Join(root, FileName)); err != nil {
		t.Fatal(err)
	}
	m, err := Migrate(root)
	if err != nil {
		t.Fatal(err)
	}
	if diff, err := m.Diff(root); err == nil || strings.Contains(diff, "ghp_FAKE") {
		t.Fatalf("Diff followed the symlink: %q %v", diff, err)
	}
}

func TestOracleMirrorsLegacyScanner(t *testing.T) {
	if _, err := oracleStages([]byte("plugin: p\n#" + strings.Repeat("x", 70000) + "\nstages:\n  - name: a\n")); err == nil {
		t.Error("a line over 64 KiB must fail like the legacy scanner")
	}
	st, err := oracleStages([]byte("plugin: p\nstages :\n  - name: a\n    cmd:\n    - echo\n    - hello\n"))
	if err != nil || len(st) != 1 || strings.Join(st[0].Cmd, " ") != "echo hello" {
		t.Errorf("whitespace before the colon: %v %v", st, err)
	}
}

func TestOracleSkipsManifestsWithoutAPluginKey(t *testing.T) {
	for _, doc := range []string{"plugin: \"p\nplugin\n\"\nstages:\n  - name: a\n    cmd:\n    - echo\n    - hello\n", "\"plugin\": p\nstages:\n  - name: a\n    cmd:\n    - echo\n    - hello\n", "\ufeffplugin: p\nstages:\n  - name: a\n    cmd:\n    - echo\n    - hello\n"} {
		root := t.TempDir()
		_ = os.WriteFile(filepath.Join(root, ".ci.yaml"), []byte(doc), 0o644)
		m, err := Migrate(root)
		if err != nil {
			continue // a BOM may be refused by the YAML decode; either way it is not silent
		}
		if !strings.Contains(strings.Join(m.Notes, "\n"), "skipped this whole manifest") {
			t.Errorf("silent skip difference for %q: %v", doc, m.Notes)
		}
	}
}

// A .nself directory swapped for a symlink after checkDestination must not
// carry the write out of the repository.
func TestWriteRefusesSymlinkSwappedAfterCheck(t *testing.T) {
	root := copyFixture(t, filepath.Join("legacy", "gauth"))
	outside := t.TempDir()
	m, err := Migrate(root)
	if err != nil {
		t.Fatal(err)
	}
	testHookBeforeCreate = func(r string) {
		testHookBeforeCreate = func(string) {}
		if err := os.Symlink(outside, filepath.Join(r, ".nself")); err != nil {
			t.Error(err)
		}
	}
	t.Cleanup(func() { testHookBeforeCreate = func(string) {} })
	if err := m.Write(root); err == nil {
		t.Fatal("wrote through a .nself symlink swapped in after the check")
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Fatalf("file created outside the repo: %v", entries)
	}
}

// A manifest below the root migrates to path-gated jobs the legacy runner
// never had; the migration must say so.
func TestMigrateNotesPathSelection(t *testing.T) {
	m, err := Migrate(copyFixture(t, filepath.Join("legacy", "multi")))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, n := range m.Notes {
		if strings.Contains(n, "when.paths") {
			got = append(got, n)
		}
	}
	if len(got) != 1 || !strings.HasPrefix(got[0], "free/other/.ci.yaml: ") || !strings.Contains(got[0], "ran every stage on every run") {
		t.Errorf("path-selection note: %v (all notes %v)", got, m.Notes)
	}
}
