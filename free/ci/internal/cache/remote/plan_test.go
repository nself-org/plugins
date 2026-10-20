package remote

import (
	"context"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/detect"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

func TestBuildPlanTrustAndToolchain(t *testing.T) {
	in := PlanInput{Facts: entries.Facts{Project: "proj", Trust: "owner", Protection: "protected"}, Detected: []detect.Fact{{Kind: "stack", Name: "go", Confidence: detect.Certain}}, Platform: "linux/amd64", Isolation: "process"}
	p, err := BuildPlan(context.Background(), in)
	if err != nil || p == nil || len(p.Save) != 0 {
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
	if err != nil || p == nil || p.Access != "ro" || len(p.Save) != 0 {
		t.Fatalf("untrusted plan=%+v err=%v", p, err)
	}
	ns, err := DecodePlanNamespace(p.Namespace)
	if err != nil || ns.Project != "proj" || ns.Trust != "owner" {
		t.Fatalf("snapshot namespace=%+v err=%v", ns, err)
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
	in := PlanInput{Facts: entries.Facts{Project: "proj", Trust: "owner", Protection: "protected"}, Detected: []detect.Fact{{Kind: "stack", Name: "go", Confidence: detect.Certain}}, Platform: "linux/amd64", Isolation: "process", Probe: func(context.Context, []string) (string, error) { return "synthetic toolchain", nil }}
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
