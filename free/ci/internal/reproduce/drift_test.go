package reproduce

import (
	"context"
	"errors"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func ptr(s string) *string { return &s }

func TestToolDriftReportedBeforeRun(t *testing.T) {
	src := Source{
		Checks: []model.Check{{ID: "go.fmt", Tool: "gofmt", ToolVersion: ptr("go version go1.20.1 linux/amd64")}, {ID: "x.y", Tool: "gone", ToolVersion: ptr("1.0")}, {ID: "z", Tool: "same", ToolVersion: ptr("2.0")}},
		Doc:    model.Evidence{ToolVersions: map[string]string{"git": "git version 2.40.0"}},
	}
	var asked [][]string
	env := Env{Probe: func(_ context.Context, argv ...string) (string, int, error) {
		asked = append(asked, argv)
		switch argv[0] {
		case "go":
			return "go version go1.22.3 linux/amd64\n", 0, nil
		case "gone":
			return "", 127, errors.New("not found")
		case "same":
			return "2.0\n", 0, nil
		}
		return "git version 2.40.0\n", 0, nil
	}}
	got := toolDrift(context.Background(), env, src)
	want := []Drift{
		{Kind: DriftTool, Name: "gofmt", Recorded: "go version go1.20.1 linux/amd64", Current: "go version go1.22.3 linux/amd64"},
		{Kind: DriftToolGone, Name: "gone", Recorded: "1.0"},
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("drift = %+v", got)
	}
	if len(asked) != 4 || asked[0][0] != "go" || asked[0][1] != "version" {
		t.Fatalf("probes = %v (go.fmt uses the gate's own `go version` probe)", asked)
	}
}

func TestRefDrift(t *testing.T) {
	rec := []model.DigestRef{{Path: "go.sum", SHA256: "a"}, {Path: "gone.lock", SHA256: "b"}, {Path: "same", SHA256: "c"}}
	cur := []model.DigestRef{{Path: "go.sum", SHA256: "z"}, {Path: "same", SHA256: "c"}, {Path: "new.lock", SHA256: "n"}}
	got := refDrift("lockfile", rec, cur)
	if len(got) != 3 || got[0].Name != "lockfile go.sum" || got[0].Current != "z" || got[1].Name != "lockfile gone.lock" || got[1].Current != "" || got[2].Name != "lockfile new.lock" || got[2].Recorded != "" {
		t.Fatalf("drift = %+v", got)
	}
	if len(refDrift("lockfile", rec, rec)) != 0 {
		t.Fatal("equal lists must not drift")
	}
}

func TestIsolationDrift(t *testing.T) {
	repo, _ := newRepo(t)
	home := t.TempDir()
	original(t, repo, home)
	src, err := Locate(context.Background(), openStore(t, home), "", "unit")
	if err != nil {
		t.Fatal(err)
	}
	src.Doc.Runner.Isolation = "vm"
	rep, err := Preflight(context.Background(), Env{Root: repo}, src)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, d := range rep.Drift {
		found = found || (d.Kind == DriftIsolation && d.Recorded == "vm")
	}
	if !found {
		t.Fatalf("a recorded vm isolation must be reported: %+v", rep.Drift)
	}
	src.Doc.RepoID = "local:other"
	if _, err = Preflight(context.Background(), Env{Root: repo}, src); err == nil {
		t.Fatal("another repository must be refused")
	}
}

func modelEvidenceWithTool(name, version string) model.Evidence {
	return model.Evidence{ToolVersions: map[string]string{name: version}}
}
