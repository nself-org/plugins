package sbom_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/detect"
	"github.com/nself-org/plugins/free/ci/internal/gates"
	gsbom "github.com/nself-org/plugins/free/ci/internal/gates/sbom"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/redact"
	"github.com/nself-org/plugins/free/ci/internal/sbom"
)

func TestRegisterAddsTheSBOMKindToTheCatalogue(t *testing.T) {
	r := gates.DefaultRegistry()
	if err := gsbom.Register(r); err != nil {
		t.Fatal(err)
	}
	d, ok := r.Get(gsbom.CheckID)
	if !ok || d.Kind != "sbom" || d.Tool != "syft" || !d.Substantive || d.Path != "deep" || !strings.Contains(d.Remediation, sbom.InstallCommand) {
		t.Fatalf("def = %+v", d)
	}
	if err := gsbom.Register(r); err != gates.ErrDuplicate {
		t.Fatalf("second registration: %v", err)
	}
}

func TestRequiredForReleaseAdvisoryElsewhere(t *testing.T) {
	yes, no := true, false
	cases := []struct {
		kind model.JobKind
		cfg  model.PipelineConfig
		want bool
	}{
		{"release", model.PipelineConfig{}, true},
		{"release", model.PipelineConfig{Checks: map[string]model.CheckDef{gsbom.CheckID: {Required: &no}}}, true}, // a release job cannot opt out
		{"build", model.PipelineConfig{}, false},
		{"quality", model.PipelineConfig{}, false},
		{"build", model.PipelineConfig{Presets: []model.Preset{"strict"}}, true},
		{"build", model.PipelineConfig{Checks: map[string]model.CheckDef{gsbom.CheckID: {Required: &yes}}}, true},
		{"build", model.PipelineConfig{Checks: map[string]model.CheckDef{gsbom.CheckID: {Required: &no}}}, false},
	}
	for i, c := range cases {
		if got := gsbom.Required(c.kind, c.cfg); got != c.want {
			t.Errorf("case %d (%s): Required = %v, want %v", i, c.kind, got, c.want)
		}
	}
}

func TestRequireForReleaseWantsTheCheckAsTheLastStep(t *testing.T) {
	ok := model.Job{Kind: "release", Run: []string{"make dist", gsbom.CheckID}}
	if err := gsbom.RequireForRelease(ok); err != nil {
		t.Fatal(err)
	}
	for name, job := range map[string]model.Job{
		"no steps":      {Kind: "release"},
		"no sbom step":  {Kind: "release", Run: []string{"make dist"}},
		"sbom not last": {Kind: "release", Run: []string{gsbom.CheckID, "make publish"}},
		"substring":     {Kind: "release", Run: []string{"echo " + gsbom.CheckID + "x"}},
		"sh -c echo":    {Kind: "release", Run: []string{"sh", "-c", "echo " + gsbom.CheckID}},
	} {
		if err := gsbom.RequireForRelease(job); !sbom.IsReason(err, sbom.ReasonInvalid) || !strings.HasPrefix(err.Error(), "E718") {
			t.Errorf("%s: got %v", name, err)
		}
	}
	if err := gsbom.RequireForRelease(model.Job{Kind: "build", Run: []string{"make"}}); err != nil {
		t.Fatalf("a build job needs no sbom step: %v", err)
	}
}

func TestImplicitDeclarationOnlyForJobsRunningTheCheck(t *testing.T) {
	with := model.Job{Kind: "release", Run: []string{"make dist", gsbom.CheckID}}
	without := model.Job{Kind: "release", Run: []string{"make dist"}}
	if got := records.ImplicitDecls(without); len(got) != 0 {
		t.Fatalf("job without the check got %+v", got)
	}
	decls, err := records.Normalize(with, true, records.ImplicitDecls(with))
	if err != nil {
		t.Fatal(err)
	}
	if len(decls) != 1 || decls[0].Path != ".nself-ci/sbom" || decls[0].Kind != records.KindSBOM || decls[0].Retention != records.KeepRetention {
		t.Fatalf("decls = %+v", decls)
	}
}

// TestSBOMFilesAreCommittedAsSBOMArtifactsWithVerifiedDigests collects the
// implicit declaration from a workdir holding generated documents.
func TestSBOMFilesAreCommittedAsSBOMArtifactsWithVerifiedDigests(t *testing.T) {
	home, ws := t.TempDir(), filepath.Join(t.TempDir(), "ws")
	docs := map[string][]byte{}
	for _, n := range []string{"cli", "web"} {
		cdx, spdx := sbom.Outputs(n)
		docs[cdx], docs[spdx] = []byte(`{"cdx":"`+n+`"}`), []byte(`{"spdx":"`+n+`"}`)
	}
	for rel, b := range docs {
		p := filepath.Join(ws, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	job := model.Job{Kind: "release", Run: []string{gsbom.CheckID}}
	decls, err := records.Normalize(job, true, records.ImplicitDecls(job))
	if err != nil {
		t.Fatal(err)
	}
	blobs, err := cas.New(home)
	if err != nil {
		t.Fatal(err)
	}
	c := records.Collector{Blobs: blobs, Scanner: redact.New(), Policy: records.Policy{MaxFile: 1 << 20, MaxTotal: 64 << 20, ProjectShare: 50, StorePath: home,
		Statfs: func(string) (uint64, uint64, error) { return 100 << 30, 200 << 30, nil }}}
	staged, err := c.Collect(context.Background(), "proj", ws, decls)
	if err != nil {
		t.Fatal(err)
	}
	if len(staged) != len(docs) {
		t.Fatalf("staged %d records, want %d", len(staged), len(docs))
	}
	for _, s := range staged {
		if s.Decl.Kind != records.KindSBOM || len(s.SHA256) != 64 {
			t.Errorf("staged %+v", s)
		}
		want, ok := docs[".nself-ci/sbom/"+filepath.Base(s.Rel)]
		if !ok {
			t.Errorf("unexpected record %q", s.Rel)
			continue
		}
		sum := sha256.Sum256(want)
		if s.Size != int64(len(want)) || s.SHA256 != hex.EncodeToString(sum[:]) {
			t.Errorf("%s size %d digest %s do not match the generated document", s.Rel, s.Size, s.SHA256)
		}
	}
}

func TestPlanNeverEmitsTheSBOMCheck(t *testing.T) {
	r := gates.DefaultRegistry()
	if err := gsbom.Register(r); err != nil {
		t.Fatal(err)
	}
	facts := []detect.Fact{{Kind: "stack", Name: "go", Path: "go.mod", Confidence: detect.Certain}, {Kind: "container", Name: "dockerfile", Path: "Dockerfile", Confidence: detect.Certain}}
	for _, kind := range []model.JobKind{"release", "build"} {
		for _, pc := range r.Plan(facts, gates.Options{Kind: kind, Container: true}) {
			if pc.Def.ID == gsbom.CheckID {
				t.Fatalf("%s: Plan emitted %s, which would run an unresolved PATH syft", kind, pc.Def.ID)
			}
		}
	}
}
