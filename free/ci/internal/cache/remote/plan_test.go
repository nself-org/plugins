package remote

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/detect"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

func TestBuildPlanTrustAndToolchain(t *testing.T) {
	in := PlanInput{Facts: entries.Facts{Project: "proj", Trust: "owner", Protection: "protected"}, Detected: []detect.Fact{{Kind: "stack", Name: "go", Confidence: detect.Certain}}, Platform: "linux/amd64", Isolation: "process"}
	p, err := BuildPlan(context.Background(), in)
	if err != nil || p != nil {
		t.Fatalf("unknown toolchain plan=%+v err=%v", p, err)
	}
	in.Probe = func(context.Context, []string) (string, error) { return "go synthetic version", nil }
	p, err = BuildPlan(context.Background(), in)
	if err != nil || p == nil || p.Access != "rw" || len(p.Save) != 1 {
		t.Fatalf("owner plan=%+v err=%v", p, err)
	}
	if p.Save[0].Spec != "go" || len(p.Save[0].Dirs) != 2 || p.Save[0].Dirs[0] != "go/mod" {
		t.Fatalf("save paths=%+v", p.Save)
	}
	ownerKey := p.Save[0].Key
	in.Facts = entries.Facts{Project: "proj", Trust: "untrusted", Protection: "unprotected", HeadRepo: "fork", PR: "7"}
	p, err = BuildPlan(context.Background(), in)
	if err != nil || p != nil {
		t.Fatalf("untrusted plan=%+v err=%v", p, err)
	}
	in.Facts.Project = "other"
	in.Facts.Trust = "owner"
	in.Facts.Protection = "protected"
	in.Facts.PR = ""
	in.Facts.HeadRepo = ""
	p, err = BuildPlan(context.Background(), in)
	if err != nil || p.Save[0].Key == ownerKey {
		t.Fatalf("project namespace did not change key: %+v err=%v", p, err)
	}
}

func TestBuildPlanEmptyAndUnsupportedLevelsBindNothing(t *testing.T) {
	base := PlanInput{Facts: entries.Facts{Project: "proj", Trust: "owner", Protection: "protected"}, Image: "image@sha256:" + strings.Repeat("a", 64), Detected: []detect.Fact{{Kind: "stack", Name: "go", Confidence: detect.Certain}}}
	for _, isolation := range []string{"vm", "sandbox", "hosted"} {
		t.Run(isolation, func(t *testing.T) {
			in := base
			in.Isolation = isolation
			for _, container := range []bool{false, true} {
				in.Container = container
				if p, err := BuildPlan(context.Background(), in); p != nil || err != nil {
					t.Fatalf("unsupported level container=%v plan=%+v err=%v", container, p, err)
				}
			}
		})
	}
	base.Detected = nil
	if p, err := BuildPlan(context.Background(), base); p != nil || err != nil {
		t.Fatalf("empty plan=%+v err=%v", p, err)
	}
}

func TestBuildPlanDeadlineAndCancellationDegradeToNoPlan(t *testing.T) {
	in := PlanInput{Facts: entries.Facts{Project: "proj", Trust: "owner", Protection: "protected"}, Isolation: "process", Detected: []detect.Fact{{Kind: "stack", Name: "go", Confidence: detect.Certain}}}
	called := false
	in.Probe = func(ctx context.Context, _ []string) (string, error) {
		called = true
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 2*time.Second {
			t.Fatal("cache work has no short deadline")
		}
		<-ctx.Done()
		return "", ctx.Err()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	if p, err := BuildPlan(ctx, in); p != nil || !errors.Is(err, context.DeadlineExceeded) || !called {
		t.Fatalf("deadline plan=%+v err=%v probe=%v", p, err, called)
	}
	cancelled, stop := context.WithCancel(context.Background())
	stop()
	if p, err := BuildPlan(cancelled, in); p != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled plan=%+v err=%v", p, err)
	}
}

type metadataPlanBackend struct{ planBackend }

type failedPlanState struct {
	*metadataPlanBackend
	err error
}

func (b failedPlanState) AttemptStateEpoch(context.Context, string) (model.JobState, int64, error) {
	return "", 0, b.err
}

func (b *metadataPlanBackend) CacheDrop(context.Context, entries.Manifest, string) error { return nil }

func TestBuildPlanReadOnlyProcessBindsNothingEvenWithSnapshotHit(t *testing.T) {
	in := PlanInput{Facts: entries.Facts{Project: "proj", Trust: "owner", Protection: "protected"}, Isolation: "process", Image: "image@sha256:" + strings.Repeat("a", 64), Detected: []detect.Fact{{Kind: "stack", Name: "go", Confidence: detect.Certain}}}
	p, err := BuildPlan(context.Background(), in)
	if err != nil || p == nil {
		t.Fatalf("save plan=%+v err=%v", p, err)
	}
	ns, err := DecodePlanNamespace(p.Namespace)
	if err != nil {
		t.Fatal(err)
	}
	blobs, err := cas.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	be := &planBackend{state: "passed", manifest: entries.Manifest{Namespace: ns.Canonical(), Key: p.Save[0].Key, Spec: "go", Writer: entries.Writer{SourceTrust: "owner", ScannedClean: true}}}
	in.Backend = be
	in.Entries = entries.New(be, blobs, entries.Options{DisableFloor: true, Project: func(string) authz.ProjectFacts { return authz.ProjectFacts{NoPrivateDeps: true} }})
	in.Facts = entries.Facts{Project: "proj", Trust: "untrusted", Protection: "unprotected", HeadRepo: "fork", PR: "7"}
	if p, err := BuildPlan(context.Background(), in); p != nil || err != nil {
		t.Fatalf("read-only process plan=%+v err=%v", p, err)
	}
}

func TestBuildPlanChecksHitMetadataWithoutHashingBlob(t *testing.T) {
	ctx := context.Background()
	in := PlanInput{Facts: entries.Facts{Project: "proj", Trust: "owner", Protection: "protected"}, Container: true, Isolation: "container", Image: "image@sha256:" + strings.Repeat("a", 64), Detected: []detect.Fact{{Kind: "stack", Name: "go", Confidence: detect.Certain}}}
	p, err := BuildPlan(ctx, in)
	if err != nil || p == nil {
		t.Fatalf("save plan=%+v err=%v", p, err)
	}
	ns, err := DecodePlanNamespace(p.Namespace)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	blobs, err := cas.New(home)
	if err != nil {
		t.Fatal(err)
	}
	digest, n, err := blobs.Put(ctx, cacheRealm(p.Namespace), strings.NewReader("before"))
	if err != nil {
		t.Fatal(err)
	}
	changed := false
	if err := filepath.WalkDir(home, func(path string, d fs.DirEntry, err error) error {
		if err == nil && d.Name() == digest {
			changed = true
			return os.WriteFile(path, []byte("after!"), 0600)
		}
		return err
	}); err != nil || !changed {
		t.Fatalf("corrupt fixture: changed=%v err=%v", changed, err)
	}
	be := &metadataPlanBackend{planBackend{state: "passed", manifest: entries.Manifest{Namespace: ns.Canonical(), Key: p.Save[0].Key, Spec: "go", Writer: entries.Writer{AttemptID: "writer"}, Blobs: []entries.Blob{{Path: "mod", SHA256: digest, Size: n}}}}}
	in.Entries = entries.New(be, blobs, entries.Options{DisableFloor: true})
	in.Backend = be
	p, err = BuildPlan(ctx, in)
	if err != nil || p == nil || len(p.Restore) != 1 || len(p.Save) != 0 {
		t.Fatalf("metadata hit was hashed during delivery: plan=%+v err=%v", p, err)
	}
	failure := errors.New("writer state unavailable")
	in.Backend = failedPlanState{be, failure}
	if p, err := BuildPlan(ctx, in); p != nil || !errors.Is(err, failure) {
		t.Fatalf("state failure hidden: plan=%+v err=%v", p, err)
	}
	if _, err := blobs.Open(cacheRealm(p.Namespace), digest); err == nil {
		t.Fatal("fixture corruption should still be caught by the byte reader")
	}
}

type planBackend struct {
	entries.Backend
	manifest entries.Manifest
	state    model.JobState
}

func (b *planBackend) CacheEntries(context.Context, string) ([]entries.Manifest, error) {
	return []entries.Manifest{b.manifest}, nil
}
func (b *planBackend) CacheTouch(context.Context, []byte, string) error { return nil }
func (b *planBackend) AttemptStateEpoch(context.Context, string) (model.JobState, int64, error) {
	return b.state, 1, nil
}

func TestBuildPlanRestoresOnlyPassedWriter(t *testing.T) {
	ctx := context.Background()
	in := PlanInput{Facts: entries.Facts{Project: "proj", Trust: "owner", Protection: "protected"}, Detected: []detect.Fact{{Kind: "stack", Name: "go", Confidence: detect.Certain}}, Platform: "linux/amd64", Isolation: "container", Container: true, Image: "image@sha256:" + strings.Repeat("a", 64)}
	p, err := BuildPlan(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	ns, err := DecodePlanNamespace(p.Namespace)
	if err != nil {
		t.Fatal(err)
	}
	blobs, err := cas.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sha, n, err := blobs.Put(ctx, cacheRealm(p.Namespace), strings.NewReader("synthetic archive"))
	if err != nil {
		t.Fatal(err)
	}
	b := &planBackend{state: "passed", manifest: entries.Manifest{Namespace: ns.Canonical(), Key: p.Save[0].Key, Spec: "go", Writer: entries.Writer{AttemptID: "writer", SourceTrust: "owner", ScannedClean: true}, Blobs: []entries.Blob{{Path: "mod", SHA256: sha, Size: n}}}}
	in.Backend = b
	in.Entries = entries.New(b, blobs, entries.Options{DisableFloor: true, Project: func(string) authz.ProjectFacts { return authz.ProjectFacts{NoPrivateDeps: true} }})
	for _, state := range []model.JobState{"finalizing", "cancelled", "passed"} {
		b.state = state
		p, err = BuildPlan(ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		if state == "passed" {
			if len(p.Restore) != 1 || len(p.Save) != 0 {
				t.Fatalf("hit re-saved: %+v", p)
			}
		} else if len(p.Restore) != 0 || len(p.Save) != 1 {
			t.Fatalf("nonpassed writer restored: %+v", p)
		}
	}
	in.Facts = entries.Facts{Project: "proj", Trust: "untrusted", Protection: "unprotected", HeadRepo: "fork", PR: "7"}
	p, err = BuildPlan(ctx, in)
	if err != nil || len(p.Restore) != 1 || p.Access != "ro" || len(p.Save) != 0 {
		t.Fatalf("snapshot plan=%+v err=%v", p, err)
	}
}
