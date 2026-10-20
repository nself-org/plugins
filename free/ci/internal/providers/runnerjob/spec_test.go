package runnerjob_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/providers/runnerjob"
)

// TestSynthesizeShape: one adhoc provider-runner pipeline whose single job is the confined runner job of
// Epic D6: sandboxed-container, no secrets, not idempotent, no infra retry, remote only, the credential
// kind set, a linux platform, the pinned image, and no field a demand could have influenced.
func TestSynthesizeShape(t *testing.T) {
	p, err := runnerjob.Synthesize(runnerSpec("987654321"))
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "provider-runner" || p.Kind != "adhoc" || len(p.Jobs) != 1 || p.ID != "provider-runner-fakeforge-987654321" {
		t.Fatalf("pipeline %+v", p)
	}
	j := p.Jobs[0]
	switch {
	case j.Isolation != "sandboxed-container", len(j.Secrets) != 0, j.Idempotent == nil || *j.Idempotent, j.Retry == nil || j.Retry.InfraMax != 0:
		t.Fatalf("job is not confined: %+v", j)
	case !j.RemoteOnly, j.RunnerCredential != testKind, j.Container == nil || j.Container.Image != testImage:
		t.Fatalf("job is not remote with a pinned image and a credential: %+v", j)
	case j.Matrix == nil || j.Matrix.Platform != "linux/amd64", len(j.Run) != 1 || j.Run[0] != "/home/runner/run.sh":
		t.Fatalf("platform or command: %+v", j)
	case j.Env[runnerjob.EnvProvider] != "fakeforge" || j.Env[runnerjob.EnvRepo] != testRepo || j.Env[runnerjob.EnvDemand] != "987654321" || len(j.Env) != 3:
		t.Fatalf("env %v", j.Env)
	}
	if runnerjob.BandOf(p) != "background" || !runnerjob.IsRunnerPipeline(p) {
		t.Fatalf("band %q", runnerjob.BandOf(p))
	}
	if runnerjob.BandOf(&model.Pipeline{Name: "default", Kind: "default"}) != "" || runnerjob.BandOf(nil) != "" {
		t.Fatal("only a runner pipeline is background")
	}
}

// TestSynthesizeRefusesUnsafeSpecs: a spec that could describe an unpinned image, another repo shape, a
// non-numeric demand or a platform other than linux amd64/arm64 builds nothing.
func TestSynthesizeRefusesUnsafeSpecs(t *testing.T) {
	bad := map[string]func(*runnerjob.Spec){
		"tag-only image": func(s *runnerjob.Spec) { s.Image = "ghcr.io/actions/actions-runner:2.338.0" },
		"short digest":   func(s *runnerjob.Spec) { s.Image = "ghcr.io/a/b@sha256:abc" },
		"repo not pair":  func(s *runnerjob.Spec) { s.Repo = "acme" },
		"repo traversal": func(s *runnerjob.Spec) { s.Repo = "acme/../x y" },
		"demand letters": func(s *runnerjob.Spec) { s.Demand = "12ab" },
		"macos":          func(s *runnerjob.Spec) { s.Arch = "darwin" },
		"windows arch":   func(s *runnerjob.Spec) { s.Arch = "x86" },
		"no command":     func(s *runnerjob.Spec) { s.Command = nil },
		"bad kind":       func(s *runnerjob.Spec) { s.Credential = "Bad Kind" },
		"no provider":    func(s *runnerjob.Spec) { s.Provider = "" },
	}
	for name, mutate := range bad {
		s := runnerSpec("1")
		mutate(&s)
		if p, err := runnerjob.Synthesize(s); err == nil || p != nil {
			t.Errorf("%s: want a refusal, got %+v", name, p)
		}
	}
}

// TestRestoreAfterTheIntakeStore: RunnerCredential and RemoteOnly are json:"-" fields, so a pipeline read
// back from the intake store has lost them; Restore puts them back for a runner pipeline and for no other.
func TestRestoreAfterTheIntakeStore(t *testing.T) {
	use(t)
	p, _ := runnerjob.Synthesize(runnerSpec("5"))
	raw, err := json.Marshal(p)
	if err != nil || strings.Contains(string(raw), testKind) || strings.Contains(string(raw), "remote_only") {
		t.Fatalf("the credential kind or remote flag leaked into the stored pipeline: %s", raw)
	}
	var back model.Pipeline
	if err = json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if back.Jobs[0].RemoteOnly || back.Jobs[0].RunnerCredential != "" {
		t.Fatal("the fields should have been dropped by JSON")
	}
	runnerjob.Restore(&back, runnerjob.KindOf)
	if !back.Jobs[0].RemoteOnly || back.Jobs[0].RunnerCredential != testKind {
		t.Fatalf("restore: %+v", back.Jobs[0])
	}
	other := &model.Pipeline{Name: "provider-runner", Kind: "adhoc", Jobs: []model.Job{{Env: map[string]string{"X": "1"}}}}
	runnerjob.Restore(other, runnerjob.KindOf)
	if other.Jobs[0].RemoteOnly || other.Jobs[0].RunnerCredential != "" {
		t.Fatal("a pipeline without the three markers must not become a runner job")
	}
}
