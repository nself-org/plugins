package plan

import (
	"testing"
)

func polyglotInput(t *testing.T) Input {
	facts, checks := checksFor(t, "polyglot")
	// trivy and govulncheck are absent: their optional checks are skipped.
	w := linuxWorld("go", "gofmt", "npm", "cargo", "gitleaks", "hadolint", "git")
	return Input{Facts: facts, Checks: checks, World: w}
}

func TestGoldenPolyglot(t *testing.T) {
	in := polyglotInput(t)
	p := mustBuild(t, in)
	golden(t, "polyglot.golden.json", envelope(t, p))
	if p.Selection.Reason != "selection.no_base" || p.Selection.Mode != "full" {
		t.Errorf("selection = %+v", p.Selection)
	}
	if r := skippedReason(p, "go.govulncheck"); r != "advisory.tool_missing" {
		t.Errorf("govulncheck skip reason = %q", r)
	}
	again, _ := Build(in)
	if string(envelope(t, again)) != string(envelope(t, p)) {
		t.Error("two builds differ")
	}
}

func matrixInput(t *testing.T, tools ...string) Input {
	return Input{Config: loadConfig(t, "support-matrix.config.json"), World: linuxWorld(tools...),
		Revision: Revision{Branch: "main", DefaultBranch: true, BaseKnown: true, Changed: []string{"tools/a.sh"}}}
}

func TestGoldenMatrix(t *testing.T) {
	p := mustBuild(t, matrixInput(t, "xcode"))
	golden(t, "matrix.golden.json", envelope(t, p))
	for _, id := range []string{"build@linux/amd64", "build@darwin/arm64"} {
		if find(p, id) == nil {
			t.Errorf("missing matrix entry %s", id)
		}
	}
	n := 0
	for _, j := range p.Jobs {
		if j.Name == "build" {
			n++
		}
	}
	if n != 2 {
		t.Errorf("support [linux/amd64, darwin/arm64] expanded to %d entries, want 2", n)
	}
	if j := find(p, "build@darwin/arm64"); j.State != StateUnplaceable || j.Blocked != "unplaceable.platform" {
		t.Errorf("darwin entry on linux = %s %s", j.State, j.Blocked)
	}
	if j := find(p, "ios"); j.State != StatePlanned || j.Job.Requirements == nil || j.Job.Requirements.Tools[0] != "xcode" {
		t.Errorf("ios = %+v", j)
	}
}

func TestGoldenAffected(t *testing.T) {
	in := matrixInput(t, "xcode")
	in.Revision = Revision{Branch: "feature/x", BaseKnown: true, Changed: []string{"tools/a.sh"}}
	p := mustBuild(t, in)
	golden(t, "affected.golden.json", envelope(t, p))
	if p.Selection.Mode != "affected" || find(p, "lint") == nil || find(p, "build@linux/amd64") != nil {
		t.Errorf("affected plan = %+v", p.Selection)
	}
}

func TestRequiredToolMissing(t *testing.T) {
	p := mustBuild(t, matrixInput(t))
	j := find(p, "ios")
	if j.State != StateUnplaceable || j.Blocked != "unplaceable.tool" || j.Detail != "tool xcode is not on PATH" {
		t.Errorf("ios without xcode = %s %s %q", j.State, j.Blocked, j.Detail)
	}
	in := matrixInput(t, "xcode")
	in.World.CPU = 1
	if j := find(mustBuild(t, in), "ios"); j.Blocked != "unplaceable.resources" {
		t.Errorf("ios with one cpu = %s", j.Blocked)
	}
	in.World.CPU, in.World.Remote = 1, true
	if j := find(mustBuild(t, in), "ios"); j.State != StatePlanned {
		t.Errorf("remote world leaves requirements to the scheduler, got %s", j.State)
	}
}
