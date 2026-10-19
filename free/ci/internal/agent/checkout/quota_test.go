package checkout

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// bombRepo commits one large, all-zero file: it packs to a few KiB and inflates to size bytes.
func bombRepo(t *testing.T, size int) (repo, rev string) {
	t.Helper()
	repo = t.TempDir()
	run := func(args ...string) string {
		cmd := exec.Command("git", append([]string{"-C", repo, "-c", "user.name=t", "-c", "user.email=t@example.invalid", "-c", "commit.gpgsign=false"}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	run("init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(repo, "zeros.bin"), make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "zeros.bin")
	run("commit", "-q", "-m", "bomb")
	return repo, run("rev-parse", "HEAD")
}

func stageBundle(t *testing.T, r *Receiver, repo, rev, src string) error {
	t.Helper()
	data, digest := bundleFor(t, repo, rev)
	l := leaseFor(rev, digest, len(data))
	if err := r.Expect(l); err != nil {
		t.Fatal(err)
	}
	if err := feedAll(t, r, l, data, 4096); err != nil {
		t.Fatal(err)
	}
	_, err := r.Stage(context.Background(), l, src)
	return err
}

func TestInflatedBundleOverTheStagingQuotaIsRefused(t *testing.T) {
	repo, rev := bombRepo(t, 24<<20)
	r := NewReceiver(filepath.Join(t.TempDir(), "stage"), nil)
	r.MaxStageBytes = 4 << 20
	root := t.TempDir()
	err := stageBundle(t, r, repo, rev, root)
	if !errors.Is(err, ErrQuota) {
		t.Fatalf("Stage of a 24 MiB bomb under a 4 MiB quota = %v, want ErrQuota", err)
	}
	if n := dirBytes(filepath.Join(root, "src")); n > 16<<20 {
		t.Fatalf("the bomb inflated to %d bytes before it was stopped", n)
	}
}

func TestBundleOverTheObjectLimitIsRefusedBeforeAnyGit(t *testing.T) {
	repo, rev := makeRepo(t)
	r := NewReceiver(filepath.Join(t.TempDir(), "stage"), nil)
	r.MaxObjects = 2
	root := t.TempDir()
	err := stageBundle(t, r, repo, rev, root)
	if !errors.Is(err, ErrQuota) {
		t.Fatalf("Stage over the object limit = %v, want ErrQuota", err)
	}
	if _, serr := os.Stat(filepath.Join(root, "src")); serr == nil {
		t.Fatal("a git process ran (src exists) for a bundle over the object limit")
	}
}

func TestBundleWithinTheQuotaStillStages(t *testing.T) {
	repo, rev := bombRepo(t, 1<<20)
	r := NewReceiver(filepath.Join(t.TempDir(), "stage"), nil)
	r.MaxStageBytes = 8 << 20
	if err := stageBundle(t, r, repo, rev, t.TempDir()); err != nil {
		t.Fatalf("Stage within the quota: %v", err)
	}
}
