package gates

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/detect"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// fixtureChecks detects the fixture ../detect/testdata/<group>/<name> and
// plans its catalogue checks.
func fixtureChecks(t *testing.T, group, name string, o Options) []PlannedCheck {
	t.Helper()
	r := DefaultRegistry()
	facts, err := detect.DefaultRegistry().Detect(os.DirFS(filepath.Join("..", "detect", "testdata", group, name)), r.Has)
	if err != nil {
		t.Fatal(err)
	}
	return r.Plan(facts, o)
}

// plannedRow is the golden shape of one planned check.
type plannedRow struct {
	ID       string   `json:"id"`
	Workdir  string   `json:"workdir"`
	Target   string   `json:"target,omitempty"`
	Required bool     `json:"required"`
	Tool     string   `json:"tool"`
	Argv     []string `json:"argv"`
	Platform string   `json:"platform,omitempty"`
	Requires []string `json:"requires_tools,omitempty"`
}

// checkPlanGolden compares the planned check list with <name>.checks.golden.json
// (UPDATE_GOLDEN=1 rewrites it).
func checkPlanGolden(t *testing.T, group, name string, planned []PlannedCheck) {
	t.Helper()
	rows := make([]plannedRow, 0, len(planned))
	for _, p := range planned {
		rows = append(rows, plannedRow{ID: p.Def.ID, Workdir: p.Workdir, Target: p.Target, Required: p.Required, Tool: p.ToolName(),
			Argv: p.Command(), Platform: p.Def.Platform, Requires: p.Def.RequiresTools})
	}
	got, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	golden := filepath.Join("..", "detect", "testdata", group, name+".checks.golden.json")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(golden, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("planned checks differ from %s:\n%s", golden, got)
	}
}

func find(planned []PlannedCheck, id string) []PlannedCheck {
	var out []PlannedCheck
	for _, p := range planned {
		if p.Def.ID == id {
			out = append(out, p)
		}
	}
	return out
}

func TestSwiftPlannedChecks(t *testing.T) {
	for _, name := range []string{"swiftpm", "xcode-app", "monorepo"} {
		t.Run(name, func(t *testing.T) {
			planned := fixtureChecks(t, "swift", name, Options{})
			checkPlanGolden(t, "swift", name, planned)
			for _, id := range []string{"swift-build", "swift-test", "xcode-test"} {
				for _, p := range find(planned, id) {
					if !p.Required {
						t.Errorf("%s must be required by default", id)
					}
				}
			}
		})
	}
}

func TestSwiftCheckShapes(t *testing.T) {
	planned := fixtureChecks(t, "swift", "swiftpm", Options{})
	if len(find(planned, "swift-build")) != 1 || len(find(planned, "swift-test")) != 1 || len(find(planned, "xcode-test")) != 0 {
		t.Fatalf("SwiftPM fixture plans the wrong checks: %+v", planned)
	}
	x := find(fixtureChecks(t, "swift", "xcode-app", Options{}), "xcode-test")
	if len(x) != 1 {
		t.Fatalf("Xcode fixture plans %d xcode-test checks", len(x))
	}
	got := x[0]
	if got.Target != "App" || got.Workdir != "." || got.Def.Platform != "darwin/arm64" || !reflect.DeepEqual(got.Def.RequiresTools, []string{"xcode"}) {
		t.Fatalf("xcode-test: %+v", got)
	}
	want := []string{"xcodebuild", "test", "-scheme", "App", "-destination", "platform=macOS"}
	if !reflect.DeepEqual(got.Command(), want) {
		t.Fatalf("xcode-test argv %v, want %v", got.Command(), want)
	}
	if got.Def.VersionProbe[0] != "xcodebuild" {
		t.Fatalf("xcode-test probe %v", got.Def.VersionProbe)
	}
}

// TestXcodeMonorepoNamesItsContainer: a project listed by a workspace builds
// through that workspace, and the scheme the two share is one check.
func TestXcodeMonorepoNamesItsContainer(t *testing.T) {
	x := find(fixtureChecks(t, "swift", "monorepo", Options{}), "xcode-test")
	want := []string{"xcodebuild", "test", "-workspace", "App.xcworkspace", "-scheme", "App", "-destination", "platform=macOS"}
	if len(x) != 1 || x[0].Workdir != "app" || x[0].Target != "App.xcworkspace/App" || !reflect.DeepEqual(x[0].Command(), want) {
		t.Fatalf("want one xcode-test through the workspace, got %+v", x)
	}
	// CocoaPods layout: the workspace has no scheme of its own, the project does.
	pods := find(DefaultRegistry().Plan([]detect.Fact{
		{Kind: "stack", Name: "xcode", Path: "App.xcodeproj/project.pbxproj", Confidence: detect.Certain},
		{Kind: "stack", Name: "xcode", Path: "App.xcworkspace/contents.xcworkspacedata", Confidence: detect.Certain},
		{Kind: "xcode_member", Name: "App.xcodeproj", Path: "App.xcworkspace/contents.xcworkspacedata", Confidence: detect.Certain},
		{Kind: "xcode_scheme", Name: "App", Path: "App.xcodeproj/xcshareddata/xcschemes/App.xcscheme", Confidence: detect.Certain},
	}, Options{}), "xcode-test")
	if len(pods) != 1 || pods[0].Command()[2] != "-workspace" || pods[0].Command()[3] != "App.xcworkspace" {
		t.Fatalf("a project listed by a workspace must build through it: %+v", pods)
	}
	// Two projects in one directory that share a scheme name both stay.
	facts := []detect.Fact{
		{Kind: "stack", Name: "xcode", Path: "A.xcodeproj/project.pbxproj", Confidence: detect.Certain},
		{Kind: "stack", Name: "xcode", Path: "B.xcodeproj/project.pbxproj", Confidence: detect.Certain},
		{Kind: "xcode_scheme", Name: "Tests", Path: "A.xcodeproj/xcshareddata/xcschemes/Tests.xcscheme", Confidence: detect.Certain},
		{Kind: "xcode_scheme", Name: "Tests", Path: "B.xcodeproj/xcshareddata/xcschemes/Tests.xcscheme", Confidence: detect.Certain},
	}
	got := find(DefaultRegistry().Plan(facts, Options{}), "xcode-test")
	if len(got) != 2 || got[0].Command()[3] != "A.xcodeproj" || got[1].Command()[3] != "B.xcodeproj" {
		t.Fatalf("two projects sharing a scheme must both be checked: %+v", got)
	}
}

func TestEcosystemOfChecks(t *testing.T) {
	for id, want := range map[string]string{"go.test": "go", "swift-build": "swift", "xcode-test": "swift", "python-lint": "python", "secrets.gitleaks": "secrets"} {
		if got := Ecosystem(id); got != want {
			t.Errorf("Ecosystem(%s) = %s, want %s", id, got, want)
		}
	}
}

// TestSwiftNeverPassesWithoutItsTool: a required Swift check on a host with
// no Swift is an error with the remediation, never a pass (E614).
func TestSwiftNeverPassesWithoutItsTool(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	r := DefaultRegistry()
	for _, id := range []string{"swift-build", "swift-test", "xcode-test"} {
		d, _ := r.Get(id)
		c := runWithoutTools(t, PlannedCheck{Def: d, Required: true, Target: "App"})
		if c.Result != "error" || !strings.Contains(c.Excerpt, "E614") || !strings.Contains(c.Excerpt, d.Remediation) {
			t.Errorf("%s: %+v", id, c)
		}
	}
}

func TestSwiftDefsAreWellFormed(t *testing.T) {
	for _, d := range swiftDefs() {
		if d.Platform != "" && !strings.Contains(d.Platform, "/") {
			t.Errorf("%s platform %q is not os/arch", d.ID, d.Platform)
		}
	}
	r := NewRegistry()
	for _, d := range append(swiftDefs(), pythonDefs()...) {
		if err := r.Register(d); err != nil {
			t.Errorf("%s: %v", d.ID, err)
		}
	}
	_ = model.PipelineConfig{}
}

// copyFixtureTree copies ../detect/testdata/<group>/<name> into a new temp
// dir, the working tree the integration tests run checks in.
func copyFixtureTree(t *testing.T, group, name string) string {
	t.Helper()
	src, dst := filepath.Join("..", "detect", "testdata", group, name), t.TempDir()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out := filepath.Join(dst, rel)
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		return os.WriteFile(out, b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}

func TestFixtureTreeCopyIsFaithful(t *testing.T) {
	root := copyFixtureTree(t, "swift", "xcode-app")
	for _, rel := range []string{"App.xcodeproj/project.pbxproj", "App.xcodeproj/xcshareddata/xcschemes/App.xcscheme", "AppTests/AppTests.swift"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("copy lost %s: %v", rel, err)
		}
	}
}
