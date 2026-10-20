//go:build security

package redteam

// Purpose: red team attacks on workspace confinement, cleanup and resource exhaustion (TM-09, TM-10).
//   - A22: a hostile attempt root (symlinks to outside data, hardlinks, mode 000 dirs, odd names, a link as the
//     root itself, a linked base) must be removed without touching anything outside it;
//   - A23: a directory tree deeper than the remover's depth limit must not survive cleanup;
//   - A24: attempt ids and a pre-planted entry cannot make JobRoot reuse or follow anything;
//   - A25: a job cannot fill the node disk through its workspace.
// Inputs:  hygiene.RemoveConfined, hygiene.JobRoot and the real container profile over temp dirs.
// Outputs: pass (control held) or fail (attack worked).
// Constraints: every path is under t.TempDir; links point only at sentinel files in the same temp tree.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/hygiene"
)

func aMust(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// aOutside makes a sentinel tree the attacker wants deleted or read.
func aOutside(t *testing.T) (dir, file string) {
	t.Helper()
	dir = filepath.Join(t.TempDir(), "outside")
	aMust(t, os.MkdirAll(filepath.Join(dir, "sub"), 0o700))
	file = filepath.Join(dir, "sub", "keep.txt")
	aMust(t, os.WriteFile(file, []byte(fakeSecret), 0o600))
	return dir, file
}

func TestA22_HostileRootIsRemovedWithoutEscape(t *testing.T) {
	out, keep := aOutside(t)
	base := filepath.Join(t.TempDir(), "work")
	root := filepath.Join(base, "att-1")
	aMust(t, os.MkdirAll(filepath.Join(root, "d", "e"), 0o700))
	aMust(t, os.Symlink(out, filepath.Join(root, "linkdir")))
	aMust(t, os.Symlink(filepath.Join(out, "sub"), filepath.Join(root, "d", "linksub")))
	aMust(t, os.Symlink(keep, filepath.Join(root, "linkfile")))
	aMust(t, os.Symlink("../../outside", filepath.Join(root, "rel"))) // resolves to the sibling of work
	aMust(t, os.Symlink(filepath.Join(root, "d"), filepath.Join(root, "d", "e", "loop")))
	aMust(t, os.Link(keep, filepath.Join(root, "hardlink")))
	aMust(t, os.WriteFile(filepath.Join(root, "-rf"), nil, 0o600))
	aMust(t, os.WriteFile(filepath.Join(root, "a\nb"), nil, 0o600))
	aMust(t, os.WriteFile(filepath.Join(root, "d", "e", "f"), []byte("x"), 0o600))
	aMust(t, os.Chmod(filepath.Join(root, "d", "e"), 0))
	if err := hygiene.RemoveConfined(base, "att-1", nil); err != nil {
		t.Fatalf("RemoveConfined: %v", err)
	}
	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		t.Errorf("attempt root survived: %v", err)
	}
	if b, err := os.ReadFile(keep); err != nil || string(b) != fakeSecret {
		t.Errorf("sentinel outside the root was touched: %v", err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Errorf("outside dir gone: %v", err)
	}
}

// A26: a directory without the write bit (a Go module cache is one) holds entries the remover cannot unlink: it
// opens the directory but never restores u+w, so the tree survives cleanup. The same code runs as the job user
// for a separated attempt, so the job user cannot clean it either.
func TestA26_ReadOnlyDirSurvivesCleanup(t *testing.T) {
	if !openFinding(t, "A09") {
		return
	}
	base := filepath.Join(t.TempDir(), "work")
	root := filepath.Join(base, "att-7")
	aMust(t, os.MkdirAll(filepath.Join(root, "ro", "sub"), 0o700))
	aMust(t, os.WriteFile(filepath.Join(root, "ro", "sub", "big.bin"), []byte(strings.Repeat("x", 1<<16)), 0o400))
	aMust(t, os.Chmod(filepath.Join(root, "ro", "sub"), 0o500))
	aMust(t, os.Chmod(filepath.Join(root, "ro"), 0o500))
	t.Cleanup(func() {
		_ = os.Chmod(filepath.Join(root, "ro"), 0o700)
		_ = os.Chmod(filepath.Join(root, "ro", "sub"), 0o700)
	})
	err := hygiene.RemoveConfined(base, "att-7", nil)
	if _, statErr := os.Lstat(root); err != nil || !os.IsNotExist(statErr) {
		t.Errorf("a tree of read-only directories survived cleanup: err=%v stat=%v", err, statErr)
	}
}

func TestA22b_RootAndBaseAreLinks(t *testing.T) {
	out, keep := aOutside(t)
	// the attempt root itself is a symlink to the outside tree
	base := filepath.Join(t.TempDir(), "work")
	aMust(t, os.MkdirAll(base, 0o700))
	aMust(t, os.Symlink(out, filepath.Join(base, "att-2")))
	_ = hygiene.RemoveConfined(base, "att-2", nil)
	if _, err := os.Stat(keep); err != nil {
		t.Errorf("a symlinked root made the remover delete the target: %v", err)
	}
	// the base is a symlink to a tree holding an attempt
	real := filepath.Join(t.TempDir(), "real")
	aMust(t, os.MkdirAll(filepath.Join(real, "att-3"), 0o700))
	aMust(t, os.WriteFile(filepath.Join(real, "att-3", "f"), nil, 0o600))
	linkBase := filepath.Join(t.TempDir(), "linkbase")
	aMust(t, os.Symlink(real, linkBase))
	if err := hygiene.RemoveConfined(linkBase, "att-3", nil); err == nil {
		t.Error("a symlinked base was accepted")
	}
	if _, err := os.Stat(filepath.Join(real, "att-3", "f")); err != nil {
		t.Errorf("a symlinked base was followed: %v", err)
	}
	for _, name := range []string{"..", ".", "", "a/b", "../x", "-", ".hidden", "att\x00", "att\n", "x y", "Ä"} {
		if err := hygiene.RemoveConfined(base, name, nil); err == nil && name != "-" && name != ".hidden" {
			// "-" and ".hidden" are safe single elements (ValidAttempt needs a leading alphanumeric, so they fail too)
			t.Errorf("RemoveConfined accepted name %q", name)
		}
	}
	if _, err := os.Stat(keep); err != nil {
		t.Errorf("sentinel gone after name attacks: %v", err)
	}
}

// A23: nesting beyond the remover's depth limit. A job can build such a tree (one mkdir per level, no long
// path needed); cleanup refuses it, so the tree and anything big inside it stays on the node after the attempt.
func TestA23_DeepTreeSurvivesCleanup(t *testing.T) {
	if !openFinding(t, "A07") {
		return
	}
	base := filepath.Join(t.TempDir(), "work")
	root := filepath.Join(base, "att-4")
	aMust(t, os.MkdirAll(root, 0o700))
	r, err := os.OpenRoot(root)
	aMust(t, err)
	for i := 0; i < 700; i++ {
		aMust(t, r.Mkdir("d", 0o700))
		next, err := r.OpenRoot("d")
		aMust(t, err)
		_ = r.Close()
		r = next
	}
	aMust(t, r.WriteFile("payload.bin", []byte(strings.Repeat("x", 1<<16)), 0o600))
	_ = r.Close()
	err = hygiene.RemoveConfined(base, "att-4", nil)
	_, statErr := os.Lstat(root)
	if err != nil || !os.IsNotExist(statErr) {
		t.Errorf("a 700-level tree survived cleanup: err=%v stat=%v (data stays on the node, next sweeps fail the same way)", err, statErr)
	}
}

func TestA24_JobRootRefusesReuseAndBadIDs(t *testing.T) {
	out, keep := aOutside(t)
	base := filepath.Join(t.TempDir(), "work")
	root, err := hygiene.JobRoot(base, "att-5")
	aMust(t, err)
	if fi, err := os.Stat(root); err != nil || fi.Mode().Perm() != 0o700 {
		t.Errorf("job root mode: %v %v", fi, err)
	}
	if _, err := hygiene.JobRoot(base, "att-5"); err == nil {
		t.Error("a second JobRoot for the same attempt reused the first one's directory")
	}
	// an entry pre-planted at the next attempt's name (the previous job left a symlink)
	aMust(t, os.Symlink(out, filepath.Join(base, "att-6")))
	if _, err := hygiene.JobRoot(base, "att-6"); err == nil {
		t.Error("JobRoot followed a pre-planted symlink")
	}
	for _, id := range []string{"", ".", "..", "a/b", "../att-5", "-rf", ".x", "a b", "a\x00b", "é", strings.Repeat("a", 129), "att-5/.."} {
		if hygiene.ValidAttempt(id) {
			t.Errorf("ValidAttempt(%q) = true", id)
		}
		if p, err := hygiene.JobRoot(base, id); err == nil {
			t.Errorf("JobRoot accepted %q -> %s", id, p)
		}
	}
	if _, err := os.Stat(keep); err != nil {
		t.Errorf("sentinel gone: %v", err)
	}
}

// A25: the container workspace is a plain host bind with a 256 MB /tmp tmpfs; nothing bounds what a job writes
// under /repo, so a job can fill the node disk (TM-10 names disk fill; the control column names no disk limit).
func TestA25_WorkspaceDiskIsBounded(t *testing.T) {
	if !openFinding(t, "A08") {
		return
	}
	res, runs := runJobPlain(t)
	if res.Err != nil || len(runs) != 1 {
		t.Fatalf("control run failed: err=%v runs=%d", res.Err, len(runs))
	}
	joined := strings.Join(runs[0], " ")
	if !strings.Contains(joined, "--storage-opt") && !strings.Contains(joined, "tmpfs-size") && !strings.Contains(joined, "volume-opt") {
		t.Errorf("no disk bound on the workspace mount: %s", joined)
	}
}

func runJobPlain(t *testing.T) (exec.AttemptResult, [][]string) {
	t.Helper()
	return aRunJob(t, aRunOpts{level: "container", plan: aMutPlanner{}})
}
