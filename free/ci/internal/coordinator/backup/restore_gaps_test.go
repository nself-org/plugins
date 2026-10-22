package backup

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRestoreHomeKeepsNewerSettingsAndUnionsDenyList(t *testing.T) {
	for _, tc := range []struct{ name, current, archive, want string }{
		{"known_hosts", "new-key\n", "old-key\n", "new-key\n"},
		{"config.yaml", "ci:\n  never_hosts: [new]\n  timeout: 20\n", "ci:\n  never_hosts: [old]\n  timeout: 10\n", "timeout: 20"},
		{"config.yaml", "ci:\n  never_hosts: [new]\n  timeout: 20\n", "ci.never_hosts: [old]\n", "timeout: 20"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			dst := filepath.Join(dir, tc.name)
			if err := os.WriteFile(dst, []byte(tc.current), 0600); err != nil {
				t.Fatal(err)
			}
			got, err := restoreHome(dst, dirFiles+"/"+tc.name, []byte(tc.archive))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(got), tc.want) {
				t.Fatalf("restored %s", got)
			}
			if tc.name == "config.yaml" && (!strings.Contains(string(got), "old") || !strings.Contains(string(got), "new")) {
				t.Fatalf("deny list lost: %s", got)
			}
		})
	}
}

func TestRestoreRefusesLockedDataDir(t *testing.T) {
	r, arch := seeded(t)
	unlock, err := LockDataDir(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	if _, err = Restore(context.Background(), r.opts(), bytes.NewReader(arch), r.kek); ReasonOf(err) != ReasonRunningCoordinator {
		t.Fatalf("restore: %v", err)
	}
	if Pending(r.dir) {
		t.Fatal("locked restore wrote marker")
	}
	if _, err = LockDataDir(r.dir); err == nil {
		t.Fatal("second data-dir lock succeeded")
	}
}
