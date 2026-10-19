package specs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
)

// D-1039: a directory swapped for an in-root symlink between the Lstat and the
// OpenRoot of removeIn must not make the sweep chmod the symlink's target.
func TestRemoveInDoesNotChmodASwappedInSymlinkTarget(t *testing.T) {
	dir := t.TempDir()
	base, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = base.Close() }()
	for _, d := range []string{"victim", "live"} {
		if err := os.Mkdir(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(filepath.Join(dir, "live"), 0o755); err != nil {
		t.Fatal(err)
	}
	afterLstat = func() {
		_ = os.Remove(filepath.Join(dir, "victim"))
		_ = os.Symlink("live", filepath.Join(dir, "victim"))
	}
	t.Cleanup(func() { afterLstat = nil })
	if err := removeIn(base, "victim"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, "live"))
	if err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("the live sibling was touched: %v %v", info, err)
	}
}

// D-1039: only size and ratio failures are size_cap; every other refusal is
// audited with its own reason, and a cancelled context is not a refusal.
func TestSaveAuditsTheRealRestoreCheckReason(t *testing.T) {
	e := newEnv(t)
	req := SaveRequest{Attempt: "a1", Spec: Spec{ID: "go"}}
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"ratio", &authz.PathError{Reason: authz.SizeRatio, Name: "x"}, "reason=" + entries.ReasonSizeCap},
		{"traversal", &authz.PathError{Reason: authz.PathTraversal, Name: "../x"}, "reason=" + entries.ReasonPathOutside},
		{"gzip", errors.New("specs: not a gzip stream"), "reason=" + entries.ReasonBlobInvalid},
	}
	for _, c := range cases {
		before := len(e.audit)
		got := e.eng.refuse(e.ctx, req, restoreCheckError("build", c.err))
		var rf *entries.Refused
		if !errors.As(got, &rf) || len(e.audit) != before+1 || !strings.Contains(e.audit[before].Detail, c.want+" ") {
			t.Fatalf("%s: got %v audit %+v", c.name, got, e.audit[before:])
		}
	}
	if got := restoreCheckError("build", context.Canceled); !errors.Is(got, context.Canceled) {
		t.Fatalf("a cancel must pass through, got %v", got)
	}
}
