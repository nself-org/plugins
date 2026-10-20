package backup

// Purpose: D-1308 regressions for a restore over a live data dir: the dir lock, and the user files of the
//          replaced dir (a never_hosts entry or an SSH pin added after the backup) surviving the swap.
// Inputs:  the rig; a real SQLite store in t.TempDir().
// Outputs: pass or fail.
// Constraints: no network; the lock is a real flock on a real file.

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRestoreRefusesWhileDataDirIsLocked(t *testing.T) {
	r, arch := seeded(t)
	dir := t.TempDir()
	release, err := Lock(dir) // what `ci serve` holds for its whole run
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Lock(dir); ReasonOf(err) != ReasonRunningCoordinator {
		t.Fatalf("a second lock: %v, want running_coordinator", err)
	}
	o := r.opts()
	o.DataDir = dir
	_, err = Restore(context.Background(), o, bytes.NewReader(arch), r.kek)
	wantReason(t, err, ReasonRunningCoordinator)
	if _, statErr := os.Stat(filepath.Join(dir, "state.db")); statErr == nil {
		t.Fatal("a refused restore wrote state.db")
	}
	release()
	restoreInto(t, r, dir, arch) // the lock is free again and a restore takes and drops it
	again, err := Lock(dir)
	if err != nil {
		t.Fatalf("the restore kept the lock: %v", err)
	}
	again()
}

func TestRestoreKeepsNewerUserFiles(t *testing.T) {
	r, _ := seeded(t)
	old := "ci:\n  never_hosts: [203.0.113.99]\n"
	oldPins := "[203.0.113.7]:22 ssh-ed25519 AAAAOLDPIN\n"
	for name, body := range map[string]string{"config.yaml": old, "known_hosts": oldPins} {
		if err := os.WriteFile(filepath.Join(r.dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	arch := r.archive()
	// after the backup: a never-host and an SSH pin are added to the live dir
	newer := "ci:\n  never_hosts: [203.0.113.99, 198.51.100.5]\n"
	newerPins := oldPins + "[203.0.113.8]:22 ssh-ed25519 AAAANEWPIN\n"
	for name, body := range map[string]string{"config.yaml": newer, "known_hosts": newerPins} {
		if err := os.WriteFile(filepath.Join(r.dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	restoreInto(t, r, r.dir, arch)
	cfg, _ := os.ReadFile(filepath.Join(r.dir, "config.yaml"))
	for _, want := range []string{"203.0.113.99", "198.51.100.5"} {
		if !strings.Contains(string(cfg), want) {
			t.Fatalf("never_hosts lost %s after the restore:\n%s", want, cfg)
		}
	}
	kh, _ := os.ReadFile(filepath.Join(r.dir, "known_hosts"))
	if !strings.Contains(string(kh), "AAAANEWPIN") || !strings.Contains(string(kh), "AAAAOLDPIN") || strings.Count(string(kh), "AAAAOLDPIN") != 1 {
		t.Fatalf("known_hosts after the restore:\n%s", kh)
	}
}

func TestMergeConfigNeverHosts(t *testing.T) {
	cases := []struct{ name, restored, replaced, want string }{
		{"equal lists keep the archived bytes", "ci:\n  never_hosts: [a]\n", "ci:\n  never_hosts: [a]\n", "ci:\n  never_hosts: [a]\n"},
		{"no replaced file", "ci:\n  never_hosts: [a]\n", "", "ci:\n  never_hosts: [a]\n"},
		{"flat key on the replaced side", "ci:\n  never_hosts: [a]\n", "ci.never_hosts: [b]\n", "b"},
		{"unparsable replaced file is ignored", "ci:\n  never_hosts: [a]\n", "ci: [", "ci:\n  never_hosts: [a]\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := string(mergeConfig([]byte(c.restored), []byte(c.replaced)))
			if c.want == "b" {
				if !strings.Contains(got, "a") || !strings.Contains(got, "b") {
					t.Fatalf("got %q", got)
				}
				return
			}
			if got != c.want {
				t.Fatalf("got %q want %q", got, c.want)
			}
		})
	}
}
