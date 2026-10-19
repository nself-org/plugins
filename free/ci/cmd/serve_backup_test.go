package main

// Purpose: tests of `nself ci serve backup|restore` (P7-NODE-20): table registration (V15 only, v1.4 argv
// frozen), the archive file mode, the JSON envelopes, the E668 refusal for another KEK with nothing written,
// and a round trip into a fresh data dir.
// Inputs:  a real SQLite store and CA in t.TempDir(); the commands' own run functions.
// Outputs: pass or fail; no network.
// Constraints: no test touches NSELF_CI_HOME or the real home; the KEK files are generated per test.

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func seedBackupDir(t *testing.T) (dir string, kek []byte) {
	t.Helper()
	dir = t.TempDir()
	st, err := store.Open(filepath.Join(dir, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pki.New(context.Background(), pki.Options{Store: st, DataDir: dir}); err != nil {
		t.Fatal(err)
	}
	if _, err = st.RegisterCoordinator(context.Background(), 4242, "start", "boot"); err != nil {
		t.Fatal(err)
	}
	if err = st.Close(); err != nil {
		t.Fatal(err)
	}
	if kek, err = pki.LoadKEK(dir, false); err != nil {
		t.Fatal(err)
	}
	return dir, kek
}

func backupEnvFor() (backupEnv, *bytes.Buffer, *bytes.Buffer) {
	var out, errb bytes.Buffer
	return backupEnv{Out: &out, Err: &errb, Alive: func(int) bool { return false }, Clock: func() time.Time { return time.Unix(1_800_000_000, 0) }}, &out, &errb
}

func TestBackupRestoreCommandsRegistered(t *testing.T) {
	for _, key := range []string{"serve backup", "serve restore"} {
		e, ok := v15Subcommands[key]
		if !ok || !e.V15Only || e.Handler == nil || e.Flags == nil {
			t.Fatalf("%s: %+v", key, e)
		}
		if _, frozen := subcommands[key]; frozen {
			t.Fatalf("%s must not be a v1.4 key", key)
		}
	}
	if problems := tableProblems(v15Subcommands); len(problems) != 0 {
		t.Fatal(problems)
	}
}

func TestBackupRestoreCommandsRoundTrip(t *testing.T) {
	dir, kek := seedBackupDir(t)
	out := filepath.Join(t.TempDir(), "coordinator.backup")
	env, stdout, stderr := backupEnvFor()
	if code := runServeBackup([]string{"--data-dir", dir, "--out", out, "--json"}, env); code != 0 {
		t.Fatalf("backup exit %d: %s%s", code, stdout, stderr)
	}
	var env1 struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env1); err != nil || env1.Data["kek_included"] != false || env1.Data["out"] != out || env1.Data["sha256"] == "" || env1.Data["sha256"] == nil {
		t.Fatalf("backup envelope: %s (%v)", stdout, err)
	}
	if st, err := os.Stat(out); err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("archive: %v %v", st, err)
	}
	if raw, _ := os.ReadFile(out); bytes.Contains(raw, kek) {
		t.Fatal("the KEK is in the archive")
	}

	kekFile := filepath.Join(t.TempDir(), "kek")
	if err := os.WriteFile(kekFile, kek, 0o600); err != nil {
		t.Fatal(err)
	}
	fresh := t.TempDir()
	env, stdout, stderr = backupEnvFor()
	if code := runServeRestore([]string{"--data-dir", fresh, "--in", out, "--kek", kekFile, "--json"}, env); code != 0 {
		t.Fatalf("restore exit %d: %s%s", code, stdout, stderr)
	}
	var env2 struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env2); err != nil || env2.Data["incarnation"] != float64(2) || env2.Data["empty_data_dir"] != true {
		t.Fatalf("restore envelope: %s (%v)", stdout, err)
	}
	if _, err := os.Stat(filepath.Join(fresh, "state.db")); err != nil {
		t.Fatal(err)
	}
}

func TestBackupRestoreCommandsRefusals(t *testing.T) {
	dir, kek := seedBackupDir(t)
	out := filepath.Join(t.TempDir(), "coordinator.backup")
	env, _, _ := backupEnvFor()
	if code := runServeBackup([]string{"--data-dir", dir, "--out", out}, env); code != 0 {
		t.Fatalf("backup exit %d", code)
	}
	other := filepath.Join(t.TempDir(), "other-kek")
	if err := os.WriteFile(other, bytes.Repeat([]byte{3}, len(kek)), 0o600); err != nil {
		t.Fatal(err)
	}
	fresh := t.TempDir()
	env, stdout, stderr := backupEnvFor()
	if code := runServeRestore([]string{"--data-dir", fresh, "--in", out, "--kek", other}, env); code != 1 || !strings.Contains(stderr.String(), "E668") || !strings.Contains(stderr.String(), "(mac)") {
		t.Fatalf("another KEK: exit %d %q %q", code, stdout, stderr)
	}
	if es, _ := os.ReadDir(fresh); len(es) != 0 {
		t.Fatalf("a refused restore wrote %d file(s)", len(es))
	}
	env, stdout, _ = backupEnvFor()
	if code := runServeRestore([]string{"--data-dir", fresh, "--in", out, "--kek", other, "--json"}, env); code != 1 || !strings.Contains(stdout.String(), `"E668"`) {
		t.Fatalf("json refusal: exit %d %s", code, stdout)
	}
	for _, args := range [][]string{{}, {"--out"}, {"extra"}} {
		env, _, _ = backupEnvFor()
		if code := runServeBackup(args, env); code != 1 {
			t.Fatalf("backup %v: exit %d, want 1 (usage)", args, code)
		}
	}
	env, _, _ = backupEnvFor()
	if code := runServeRestore([]string{"--in", out}, env); code != 1 {
		t.Fatalf("restore without --kek: exit %d", code)
	}
	env, _, errb := backupEnvFor()
	if code := runServeBackup([]string{"--data-dir", t.TempDir(), "--out", filepath.Join(t.TempDir(), "x")}, env); code != 1 || !strings.Contains(errb.String(), "missing_kek") {
		t.Fatalf("no KEK in the data dir: exit %d %q", code, errb)
	}
}
