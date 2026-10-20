//go:build security

package security

// Purpose: the cache and artifact slice of the directive list (I11; TM-12, TM-13, TM-14, TM-17): the
// cache respects trust (a namespace per untrusted PR, no trusted job reads what untrusted code wrote,
// no write by a stale, failed or foreign attempt) and artifacts stay authorized (crafted names and
// links are refused, another project never sees an artifact, untrusted output is not promoted).
// The full cache, artifact and release matrix is P7-CACHE-11's suite (free/ci/test/cachesec); this
// file keeps the cases the security acceptance list names, against the same production rules.
// Inputs:  authz types built in the test.
// Outputs: pass or fail.
// Constraints: pure rules; no filesystem except the destination check's temporary directory.

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

func cacheNS(t *testing.T, class model.TrustClass, p authz.Protection, head, pr string) authz.Namespace {
	t.Helper()
	ns, err := authz.CacheNamespace("proj-1", class, p, head, pr)
	if err != nil {
		t.Fatalf("namespace %s/%s: %v", class, p, err)
	}
	return ns
}

func liveJob(ns authz.Namespace, state authz.AttemptState, passed bool, attempt string, epoch int64) authz.Job {
	return authz.Job{Namespace: ns, State: state, Passed: passed, Capability: authz.Capability{Attempt: attempt, Epoch: epoch}}
}

// TestCacheRespectsTrust is the cache half of the directive list.
func TestCacheRespectsTrust(t *testing.T) {
	trusted := cacheNS(t, "owner", authz.Protected, "", "")
	prA := cacheNS(t, "untrusted", authz.Unprotected, "fork/repo", "7")
	prB := cacheNS(t, "untrusted", authz.Unprotected, "fork/repo", "8")
	otherFork := cacheNS(t, "untrusted", authz.Unprotected, "other/repo", "7")
	if prA.Equal(prB) || prA.Equal(otherFork) || prA.Equal(trusted) {
		t.Fatal("two pull requests, two forks or a PR and a trusted job share a cache namespace")
	}
	untrustedJob := liveJob(prA, authz.StateRunning, false, "att-a", 1)
	trustedJob := liveJob(trusted, authz.StateRunning, false, "att-t", 1)
	// A trusted job never reads what untrusted code wrote.
	for _, foreign := range []authz.Namespace{prA, prB, otherFork} {
		if m := authz.CacheAccess(trustedJob, foreign); m != authz.ModeNone {
			t.Errorf("a trusted job got %s on the untrusted namespace %+v", m, foreign)
		}
	}
	// One PR never reads or writes another PR's namespace.
	for _, other := range []authz.Namespace{prB, otherFork} {
		if m := authz.CacheAccess(untrustedJob, other); m != authz.ModeNone {
			t.Errorf("PR 7 got %s on %+v", m, other)
		}
	}
	// Untrusted may restore the protected snapshot read-only, never write it.
	if m := authz.CacheAccess(untrustedJob, trusted); m != authz.ModeROSnapshot {
		t.Errorf("untrusted access to the protected snapshot = %s, want read-only", m)
	}
	fin := liveJob(prA, authz.StateFinalizing, true, "att-a", 1)
	if err := authz.CheckWrite(fin, trusted, authz.Capability{Attempt: "att-a", Epoch: 1}); err == nil {
		t.Error("an untrusted job wrote the protected namespace")
	}
	if err := authz.CheckWrite(fin, prA, authz.Capability{Attempt: "att-a", Epoch: 1}); err != nil {
		t.Errorf("control: the untrusted job could not write its own namespace: %v", err)
	}
	// A stale, failed, running or foreign attempt writes nothing.
	for name, tc := range map[string]struct {
		job authz.Job
		cur authz.Capability
	}{
		"stale epoch":   {fin, authz.Capability{Attempt: "att-a", Epoch: 2}},
		"other lease":   {fin, authz.Capability{Attempt: "att-z", Epoch: 1}},
		"still running": {liveJob(prA, authz.StateRunning, true, "att-a", 1), authz.Capability{Attempt: "att-a", Epoch: 1}},
		"ended":         {liveJob(prA, authz.StateEnded, true, "att-a", 1), authz.Capability{Attempt: "att-a", Epoch: 1}},
		"failed":        {liveJob(prA, authz.StateFinalizing, false, "att-a", 1), authz.Capability{Attempt: "att-a", Epoch: 1}},
	} {
		if err := authz.CheckWrite(tc.job, tc.job.Namespace, tc.cur); err == nil {
			t.Errorf("%s: the write was allowed", name)
		}
	}
}

// TestArtifactsStayAuthorized is the artifact half: crafted entries, other projects and untrusted
// output never get through.
func TestArtifactsStayAuthorized(t *testing.T) {
	lim := authz.Limits{}
	bad := map[string]authz.Entry{
		"parent traversal":  {Name: "../../etc/passwd", Mode: 0o644, Size: 1},
		"absolute":          {Name: "/etc/passwd", Mode: 0o644, Size: 1},
		"dot git":           {Name: ".git/hooks/pre-commit", Mode: 0o755, Size: 1},
		"backslash":         {Name: `a\..\b`, Mode: 0o644, Size: 1},
		"symlink escape":    {Name: "link", Mode: fs.ModeSymlink | 0o777, LinkTarget: "../../../etc", Size: 0},
		"absolute symlink":  {Name: "link", Mode: fs.ModeSymlink | 0o777, LinkTarget: "/etc/shadow", Size: 0},
		"hardlink":          {Name: "h", Mode: 0o644, Hardlink: true, Size: 0},
		"device":            {Name: "dev", Mode: fs.ModeDevice | 0o666, Size: 0},
		"fifo":              {Name: "pipe", Mode: fs.ModeNamedPipe | 0o666, Size: 0},
		"setuid":            {Name: "su", Mode: fs.ModeSetuid | 0o755, Size: 1},
		"control character": {Name: "a\x00b", Mode: 0o644, Size: 1},
		"empty":             {Name: "", Mode: 0o644, Size: 1},
	}
	for name, e := range bad {
		if got, err := authz.ArtifactPath(e, lim); err == nil {
			t.Errorf("%s: %q was accepted as %q", name, e.Name, got)
		}
	}
	if got, err := authz.ArtifactPath(authz.Entry{Name: "dist/app.tar", Mode: 0o644, Size: 10}, lim); err != nil || got != "dist/app.tar" {
		t.Errorf("control: a plain file was refused: %q %v", got, err)
	}
	// `artifacts get` never writes into the repository's own hooks.
	cwd := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cwd, ".git", "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got, err := authz.ArtifactGetDest(".git/hooks", cwd); err == nil {
		t.Errorf("artifacts get accepted .git/hooks as a destination: %q", got)
	}
	if _, err := authz.ArtifactGetDest("out", cwd); err != nil {
		t.Errorf("control: a plain destination was refused: %v", err)
	}
	// Visibility: another project never sees the artifact, whatever the role; restricted needs release.
	meta := authz.ArtifactMeta{Project: "proj-1"}
	if authz.ArtifactVisibility(meta, authz.Viewer{Project: "proj-2", Role: authz.RoleAdmin}) {
		t.Error("an admin of another project saw the artifact")
	}
	if !authz.ArtifactVisibility(meta, authz.Viewer{Project: "proj-1", Role: authz.RoleMember}) {
		t.Error("control: a member of the project was refused")
	}
	meta.Visibility = authz.VisibilityRestricted
	if authz.ArtifactVisibility(meta, authz.Viewer{Project: "proj-1", Role: authz.RoleMember}) || !authz.ArtifactVisibility(meta, authz.Viewer{Project: "proj-1", Role: authz.RoleRelease}) {
		t.Error("restricted visibility does not follow the role")
	}
	if authz.ArtifactVisibility(authz.ArtifactMeta{Project: "proj-1", Visibility: "weird"}, authz.Viewer{Project: "proj-1", Role: authz.RoleMember}) {
		t.Error("an unknown visibility value was read as public")
	}
	// Promotion: untrusted output, an unbound digest or another revision never reaches a release.
	ev := goodEvidence()
	ev.Artifacts = []model.Artifact{{Path: "dist/app.tar", SHA256: strings.Repeat("a", 64), Size: 1, Committed: true}}
	g := goodGate(ev)
	art := authz.Artifact{Project: ev.RepoID, Revision: ev.Revision, SHA256: strings.Repeat("a", 64)}
	if ok, why := authz.Promotable(art, ev, g); !ok {
		t.Fatalf("control: honest evidence and artifact were refused: %v", why)
	}
	untrusted := ev
	untrusted.SourceTrust = "untrusted"
	if ok, why := authz.Promotable(art, untrusted, g); ok || !hasReason(why, attest.ReasonTrustBelowMin) {
		t.Errorf("untrusted output was promotable: %v %v", ok, why)
	}
	collaborator := ev
	collaborator.SourceTrust = "collaborator"
	if ok, why := authz.Promotable(art, collaborator, g); ok || !hasReason(why, attest.ReasonTrustBelowMin) {
		t.Errorf("a collaborator's output was promotable: %v %v", ok, why)
	}
	swapped := art
	swapped.SHA256 = strings.Repeat("b", 64)
	if ok, why := authz.Promotable(swapped, ev, g); ok || !hasReason(why, authz.ReasonArtifactUnbound) {
		t.Errorf("a substituted artifact was promotable: %v %v", ok, why)
	}
	other := art
	other.Revision = strings.Repeat("b", 40)
	if ok, _ := authz.Promotable(other, ev, g); ok {
		t.Error("an artifact of another revision was promotable")
	}
}
