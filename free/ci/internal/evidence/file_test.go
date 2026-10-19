package evidence

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// heldDocument is the run-level document the rig's first run recorded.
func heldDocument(t *testing.T, r *rig) model.Evidence {
	t.Helper()
	s, err := Show(context.Background(), r.env(), "latest")
	ok(t, err)
	ev := s.Evidence
	ev.Freshness = model.Freshness{State: "current"}
	return ev
}

func TestVerifyFile(t *testing.T) {
	r, sha := newRig(t)
	ctx := context.Background()
	ev := heldDocument(t, r)
	verify := func(env Env, doc model.Evidence, at string) (Verdict, error) {
		b, err := json.Marshal(doc)
		ok(t, err)
		return VerifyFile(ctx, env, b, at)
	}
	if v, err := verify(r.env(), ev, ""); err != nil || v.Exit != ExitOK || v.Report.Class != ClassDefault {
		t.Fatalf("the recorded document: %+v %v", v, err)
	}
	noStore := r.env()
	noStore.Store = nil
	v, err := verify(noStore, ev, "")
	if err != nil || v.Exit != ExitInvalid || len(v.Warnings) == 0 || !strings.Contains(v.Warnings[0], "not authenticated") {
		t.Fatalf("an unknown origin must be refused and reported: %+v %v", v, err)
	}
	forged := ev
	forged.RunID = "r-forged" // a failing run made to look like a pass cannot hide behind a new run id
	if v, err = verify(r.env(), forged, ""); err != nil || v.Exit != ExitInvalid {
		t.Fatalf("a document under a run id this machine never saw: exit %d %v", v.Exit, err)
	}
	foreign := ev
	foreign.RepoID = "local:" + strings.Repeat("f", 64)
	if v, err = verify(r.env(), foreign, ""); err != nil || v.Exit != ExitInvalid {
		t.Fatalf("a document of another repository: exit %d %v", v.Exit, err)
	}

	other := ev
	other.Revision = strings.Repeat("9", 40)
	if v, err = verify(r.env(), other, ""); err != nil || v.Exit != ExitInvalid {
		t.Fatalf("another revision: exit %d %v", v.Exit, err)
	}
	edited := ev
	edited.InputDigests.Lockfiles = []model.DigestRef{{Path: "go.sum", SHA256: strings.Repeat("e", 64)}}
	if v, err = verify(r.env(), edited, ""); err != nil || v.Exit != ExitInvalid {
		t.Fatalf("an edited digest of a run this machine recorded: exit %d %v", v.Exit, err)
	}
	if v, err = verify(noStore, edited, ""); err != nil || v.Exit != ExitStale {
		t.Fatalf("an edited digest of an unknown run reads stale: exit %d %v", v.Exit, err)
	}
	if _, err = VerifyFile(ctx, r.env(), []byte(`{"schema":"ci.evidence/v1"}`), ""); err == nil {
		t.Fatal("a schema-invalid document verified")
	} else if e, _ := AsError(err); e.Exit != ExitInvalid {
		t.Fatalf("exit %d", e.Exit)
	}
	unbound := ev
	reason := model.BindingReason("dirty")
	why := "the worktree is not the named commit"
	unbound.WorktreeState, unbound.Binding = "dirty", model.Binding{State: "unbound", Reason: &reason}
	unbound.Freshness = model.Freshness{State: "unknown", Reason: &why}
	if v, err = verify(noStore, unbound, ""); err != nil || v.Exit != ExitUnbound {
		t.Fatalf("an unbound document: exit %d %v", v.Exit, err)
	}
	if _, err = VerifyFile(ctx, noStore, []byte(`{}`), "HEAD"); err == nil {
		t.Fatal("a ref name was accepted for --at-revision")
	}
	if _, err = VerifyFile(ctx, Env{}, mustJSON(t, ev), ""); err == nil {
		t.Fatal("outside a repository with no --at-revision must be refused")
	}

	// --at-revision: the document stays fresh for its own revision after the tree moved on.
	commitFiles(t, r.dir, map[string]string{"go.sum": "h1:later\n"})
	if v, err = verify(noStore, ev, ""); err != nil || v.Exit != ExitInvalid {
		t.Fatalf("against the new HEAD the revision differs: exit %d %v", v.Exit, err)
	}
	if v, err = verify(r.env(), ev, sha); err != nil || v.Exit != ExitOK || v.Report.Freshness.State != "current" {
		t.Fatalf("at its own revision: %+v %v", v, err)
	}

	// A binding for a path the revision does not have fails closed.
	early, _ := newRepo(t, map[string]string{"go.mod": "module x\n"})
	pre := ev
	pre.Revision = gitRun(t, early, "rev-parse", "HEAD")
	if v, err = verify(Env{Root: early}, pre, pre.Revision); err != nil || v.Exit != ExitInvalid || len(v.Warnings) == 0 {
		t.Fatalf("a bound input absent at the revision: exit %d %+v %v", v.Exit, v.Warnings, err)
	}
}
