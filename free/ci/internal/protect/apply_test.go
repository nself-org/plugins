package protect

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

const prot = "repos/example-org/example-repo/branches/main/protection"

func TestProtectionReadsAndClassifiesErrors(t *testing.T) {
	live := string(fixture(t, "live_full.json"))
	t.Run("ok", func(t *testing.T) {
		f := &fakeGH{replies: []reply{{match: prot, out: live}}}
		got, err := f.client().Protection(context.Background(), "example-org/example-repo", "main")
		if err != nil || string(got) != live {
			t.Fatalf("got %v %v", len(got), err)
		}
	})
	t.Run("unprotected is nil, nil", func(t *testing.T) {
		f := &fakeGH{replies: []reply{{match: prot, exit: 1, out: `{"message":"Branch not protected"}` + "\ngh: Branch not protected (HTTP 404)"}}}
		got, err := f.client().Protection(context.Background(), "example-org/example-repo", "main")
		if err != nil || got != nil {
			t.Fatalf("got %q %v", got, err)
		}
	})
	t.Run("403 is E643", func(t *testing.T) {
		f := &fakeGH{replies: []reply{{match: prot, exit: 1, out: "gh: Upgrade to GitHub Pro or make this repository public to enable this feature. (HTTP 403)"}}}
		_, err := f.client().Protection(context.Background(), "example-org/example-repo", "main")
		var pe *Error
		if !errors.As(err, &pe) || pe.Code != CodeNoPlan {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("404 without the not-protected text is an error", func(t *testing.T) {
		f := &fakeGH{replies: []reply{{match: prot, exit: 1, out: "gh: Not Found (HTTP 404)"}}}
		if _, err := f.client().Protection(context.Background(), "example-org/example-repo", "main"); err == nil {
			t.Fatal("a missing repository must not read as an unprotected branch")
		}
	})
}

func TestApplySnapshotsThenPutsAndVerifies(t *testing.T) {
	live := fixture(t, "live_full.json")
	plan, err := Build("example-org/example-repo", "main", live, AppID)
	if err != nil {
		t.Fatal(err)
	}
	after := fixture(t, "live_already.json") // what GitHub answers once the check is required
	f := &fakeGH{replies: []reply{{match: "--method PUT", out: "{}"}, {match: prot, out: string(after)}}}
	dir := t.TempDir()
	snap, err := f.client().Apply(context.Background(), plan, live, dir, t0)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "example-org__example-repo__main-20261019T120000Z.json"); snap != want {
		t.Fatalf("snapshot = %s, want %s", snap, want)
	}
	s, err := LoadSnapshot(snap)
	if err != nil || s.Unprotected || s.Repo != "example-org/example-repo" || !strings.Contains(string(s.Protection), `"lint"`) {
		t.Fatalf("snapshot = %+v %v", s, err)
	}
	if f.puts() != 1 || len(f.bodies) != 1 || f.bodies[0] != string(plan.JSON()) {
		t.Fatalf("PUT calls = %d, bodies = %q", f.puts(), f.bodies)
	}
	if strings.Contains(strings.Join(f.calls, "\n"), "app_id") {
		t.Fatal("the body must travel in a file (--input), never on argv")
	}
	left, _ := filepath.Glob(filepath.Join(dir, "put-*.json"))
	if len(left) != 0 {
		t.Fatalf("staged body left behind: %v", left)
	}
}

func TestApplyNoSnapshotNoPut(t *testing.T) {
	live := fixture(t, "live_full.json")
	plan, _ := Build("example-org/example-repo", "main", live, AppID)
	f := &fakeGH{}
	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := f.client().Apply(context.Background(), plan, live, filepath.Join(blocked, "protect"), t0); err == nil {
		t.Fatal("apply must fail when the snapshot cannot be written")
	}
	if len(f.calls) != 0 {
		t.Fatalf("no gh call may happen without a snapshot: %v", f.calls)
	}
}

func TestApplyNoopWritesNothing(t *testing.T) {
	live := fixture(t, "live_already.json")
	plan, _ := Build("example-org/example-repo", "main", live, AppID)
	f := &fakeGH{}
	dir := filepath.Join(t.TempDir(), "protect")
	snap, err := f.client().Apply(context.Background(), plan, live, dir, t0)
	if err != nil || snap != "" || len(f.calls) != 0 {
		t.Fatalf("snap=%q err=%v calls=%v", snap, err, f.calls)
	}
	if _, err := os.Stat(dir); err == nil {
		t.Fatal("a no-op must not create the snapshot directory")
	}
}

func TestApplyDetectsReadBackMismatch(t *testing.T) {
	live := fixture(t, "live_full.json")
	plan, _ := Build("example-org/example-repo", "main", live, AppID)
	// GitHub answers the old protection after the PUT: the new check never landed.
	f := &fakeGH{replies: []reply{{match: "--method PUT", out: "{}"}, {match: prot, out: string(live)}}}
	_, err := f.client().Apply(context.Background(), plan, live, t.TempDir(), t0)
	if err == nil || !strings.Contains(err.Error(), "read-back differs") || !strings.Contains(err.Error(), "required_status_checks") {
		t.Fatalf("err = %v", err)
	}
}

func TestRestoreFromSnapshot(t *testing.T) {
	live := fixture(t, "live_full.json")
	dir := t.TempDir()
	path, err := SaveSnapshot(dir, "example-org/example-repo", "main", live, t0)
	if err != nil {
		t.Fatal(err)
	}
	snap, err := LoadSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeGH{replies: []reply{{match: "--method PUT", out: "{}"}, {match: prot, out: string(live)}}}
	before, err := f.client().Restore(context.Background(), snap, dir, t0.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if b, err := LoadSnapshot(before); err != nil || b.Unprotected {
		t.Fatalf("a restore must save what it replaces: %q %v", before, err)
	}
	pre, _ := Normalise(live)
	if len(f.bodies) != 1 || f.bodies[0] != string(mustIndent(pre)) {
		t.Fatalf("restore body = %q", f.bodies)
	}
	un, _ := SaveSnapshot(dir, "example-org/example-repo", "dev", nil, t0)
	s2, _ := LoadSnapshot(un)
	if _, err := f.client().Restore(context.Background(), s2, dir, t0); err == nil {
		t.Fatal("an unprotected snapshot has nothing to PUT back")
	}
}

// TestRestoreCreatesTheSnapshotDir: a snapshot copied to a machine whose
// $NSELF_CI_HOME/protect does not exist yet restores there.
func TestRestoreCreatesTheSnapshotDir(t *testing.T) {
	live := fixture(t, "live_full.json")
	src, _ := SaveSnapshot(t.TempDir(), "example-org/example-repo", "main", live, t0)
	snap, _ := LoadSnapshot(src)
	f := &fakeGH{replies: []reply{{match: "--method PUT", out: "{}"}, {match: prot, out: string(live)}}}
	dest := filepath.Join(t.TempDir(), "new", "protect")
	if _, err := f.client().Restore(context.Background(), snap, dest, t0); err != nil {
		t.Fatal(err)
	}
}

// TestSnapshotsInTheSameSecondDoNotOverwrite: the first recovery point survives.
func TestSnapshotsInTheSameSecondDoNotOverwrite(t *testing.T) {
	dir := t.TempDir()
	a, err1 := SaveSnapshot(dir, "example-org/example-repo", "main", fixture(t, "live_full.json"), t0)
	b, err2 := SaveSnapshot(dir, "example-org/example-repo", "main", fixture(t, "live_already.json"), t0)
	if err1 != nil || err2 != nil || a == b {
		t.Fatalf("%q %q %v %v", a, b, err1, err2)
	}
	sa, _ := LoadSnapshot(a)
	if strings.Contains(string(sa.Protection), `"nself-ci"`) {
		t.Fatal("the first snapshot was overwritten by the second")
	}
}

// TestOnly403ForThePlanIsE643: a rate limit or a token without admin rights is
// an ordinary error, not the "no protection on this plan" answer.
func TestOnly403ForThePlanIsE643(t *testing.T) {
	f := &fakeGH{replies: []reply{{match: prot, exit: 1, out: "gh: API rate limit exceeded for user ID 1. (HTTP 403)"}}}
	_, err := f.client().Protection(context.Background(), "example-org/example-repo", "main")
	var pe *Error
	if err == nil || errors.As(err, &pe) {
		t.Fatalf("err = %v, want a plain error", err)
	}
}

// TestReadBackOrderIsNotADifference: GitHub may list the checks in another order.
func TestReadBackOrderIsNotADifference(t *testing.T) {
	live := fixture(t, "live_full.json")
	plan, _ := Build("example-org/example-repo", "main", live, AppID)
	got, _ := Normalise(live)
	got.RequiredStatusChecks.Checks = []Check{{"nself-ci", 15368}, {"build (ubuntu-latest)", 15368}, {"lint", 15368}}
	if d := Diff(plan.Body, got); len(d) != 0 {
		t.Fatalf("reordered checks reported as a difference: %v", d)
	}
}

func TestLoadSnapshotRejectsOtherJSON(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.json")
	_ = os.WriteFile(p, []byte(`{"hello":1}`), 0o600)
	if _, err := LoadSnapshot(p); err == nil {
		t.Fatal("not a snapshot")
	}
}

func TestDefaultBranch(t *testing.T) {
	f := &fakeGH{replies: []reply{{match: "repos/example-org/example-repo --jq", out: "trunk\n"}}}
	b, err := f.client().DefaultBranch(context.Background(), "example-org/example-repo")
	if err != nil || b != "trunk" {
		t.Fatalf("%q %v", b, err)
	}
}
