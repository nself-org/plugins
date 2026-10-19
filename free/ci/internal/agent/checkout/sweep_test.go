package checkout

import (
	"context"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestCreateSweepsStaleBundleRefs covers D-1058: a coordinator killed by SIGKILL never runs the
// deferred ref delete, so the next Create must remove the leftover refs/nself-ci/bundle-* pins
// (legacy and timestamped) while leaving a recent pin of a concurrent Create and foreign refs alone.
func TestCreateSweepsStaleBundleRefs(t *testing.T) {
	repo, rev := makeRepo(t)
	old := strconv.FormatInt(time.Now().Add(-48*time.Hour).Unix(), 10)
	fresh := strconv.FormatInt(time.Now().Unix(), 10)
	stale := []string{RefPrefix + "bundle-aabbccddeeff", RefPrefix + "bundle-" + old + "-aabbccddeeff"}
	keep := []string{RefPrefix + "bundle-" + fresh + "-112233445566", RefPrefix + "other"}
	for _, ref := range append(append([]string{}, stale...), keep...) {
		if out, err := exec.Command("git", "-C", repo, "update-ref", ref, rev).CombinedOutput(); err != nil {
			t.Fatalf("plant %s: %v\n%s", ref, err, out)
		}
	}
	if _, _, err := Create(context.Background(), nil, repo, rev, filepath.Join(t.TempDir(), "out.bundle")); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("git", "-C", repo, "for-each-ref", "--format=%(refname)", RefPrefix).Output()
	if err != nil {
		t.Fatal(err)
	}
	left := strings.Fields(string(out))
	has := func(ref string) bool {
		for _, l := range left {
			if l == ref {
				return true
			}
		}
		return false
	}
	for _, ref := range stale {
		if has(ref) {
			t.Errorf("stale ref %s survived Create", ref)
		}
	}
	for _, ref := range keep {
		if !has(ref) {
			t.Errorf("Create removed %s, which is not a stale bundle pin", ref)
		}
	}
}
