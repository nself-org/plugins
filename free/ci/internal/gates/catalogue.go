// Package gates maps detector facts to runnable CI checks.
package gates

import (
	"errors"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/detect"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// CheckDef describes one check without binding it to a particular runner.
type CheckDef struct {
	ID, Kind, Tool               string
	Argv, VersionProbe           []string
	RequiredDefault, Substantive bool
	Path                         model.JobPath
	Remediation, ImpliedBy       string
	// Platform and RequiresTools bind a check to a host kind (P7-CI-64): the
	// planned job runs only on that platform and needs these placement tools.
	Platform      string
	RequiresTools []string
}

// PlannedCheck is a check applied to a discovered directory or file.
type PlannedCheck struct {
	Def             CheckDef
	Workdir, Target string
	Required        bool
	// Argv and Tool replace the definition's when the project decides them
	// (the Python package manager, the Xcode container); empty means the definition's.
	Argv []string
	Tool string
}

// Command is the argv to run, with {target} filled in.
func (p PlannedCheck) Command() []string {
	argv := append([]string(nil), p.Def.Argv...)
	if len(p.Argv) > 0 {
		argv = append([]string(nil), p.Argv...)
	}
	for i, a := range argv {
		argv[i] = strings.ReplaceAll(a, "{target}", p.Target)
	}
	return argv
}

// PlacementTool is the tool name placement matches: empty when the check
// states its requirement as requires.tools (xcode for xcode-test), so a missing
// tool is named once.
func (p PlannedCheck) PlacementTool() string {
	if len(p.Def.RequiresTools) > 0 {
		return ""
	}
	return p.ToolName()
}

// ToolName is the executable the check needs.
func (p PlannedCheck) ToolName() string {
	if p.Tool != "" {
		return p.Tool
	}
	return p.Def.Tool
}

// Options resolves default-advisory checks at plan time.
type Options struct {
	Container bool
	Kind      model.JobKind
	Config    model.PipelineConfig
}

func required(d CheckDef, o Options) bool {
	if d.RequiredDefault {
		return true
	}
	if o.Container || o.Kind == "release" {
		return true
	}
	for _, p := range o.Config.Presets {
		if p == "strict" {
			return true
		}
	}
	if c, ok := o.Config.Checks[d.ID]; ok && c.Required != nil {
		return *c.Required
	}
	return false
}

// Plan creates stable check instances from certain detector facts.
func (r *Registry) Plan(facts []detect.Fact, o Options) []PlannedCheck {
	var out []PlannedCheck
	seen := map[string]bool{}
	add := func(id, dir, target string, mods ...func(*PlannedCheck)) {
		d, ok := r.Get(id)
		if !ok {
			return
		}
		key := id + "\x00" + dir + "\x00" + target
		if seen[key] {
			return
		}
		seen[key] = true
		pc := PlannedCheck{Def: d, Workdir: dir, Target: target, Required: required(d, o)}
		for _, mod := range mods {
			mod(&pc)
		}
		out = append(out, pc)
	}
	py, xc, swiftTests, node := pythonIndex(facts), xcodeIndex(facts), swiftTestIndex(facts), nodeIndex(facts)
	for _, f := range facts {
		if f.Confidence != detect.Certain {
			continue
		}
		dir := path.Dir(f.Path)
		switch {
		case f.Kind == "stack" && f.Name == "go":
			for _, id := range []string{"go.fmt", "go.vet", "go.test", "go.govulncheck"} {
				add(id, dir, "")
			}
			if v, ok := o.Config.Checks["go.coverage"]; ok && v.CoverageFloor != nil {
				add("go.coverage", dir, "")
			}
		case f.Kind == "stack" && f.Name == "node":
			for _, id := range []string{"node.lint", "node.typecheck", "node.test", "node.build"} {
				add(id, dir, "", node.mod(dir, id))
			}
			if v, ok := o.Config.Checks["node.coverage"]; ok && v.CoverageFloor != nil {
				add("node.coverage", dir, "", node.mod(dir, "node.coverage"))
			}
		case f.Kind == "stack" && (f.Name == "flutter" || f.Name == "dart"):
			add("flutter.analyze", dir, "")
			add("flutter.test", dir, "")
		case f.Kind == "stack" && f.Name == "rust":
			add("rust.clippy", dir, "")
			add("rust.test", dir, "")
		case f.Kind == "stack" && f.Name == "swiftpm":
			add("swift-build", dir, "")
			if swiftTests[f.Path] {
				add("swift-test", dir, "")
			}
		case f.Kind == "xcode_scheme":
			xc.plan(f, add)
		case f.Kind == "stack" && f.Name == "python":
			py.plan(dir, add)
		case f.Kind == "container" && f.Name == "dockerfile":
			add("container.hadolint", dir, path.Base(f.Path))
			add("container.trivy.config", dir, path.Base(f.Path))
		case f.Kind == "container" && f.Name == "compose":
			add("container.trivy.config", dir, path.Base(f.Path))
		}
	}
	if len(facts) > 0 {
		add("secrets.gitleaks", ".", "")
		add("security.trivy.fs", ".", "")
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Def.ID != b.Def.ID {
			return a.Def.ID < b.Def.ID
		}
		if a.Workdir != b.Workdir {
			return a.Workdir < b.Workdir
		}
		return a.Target < b.Target
	})
	return out
}

// ImpliedJobs adds a required job when an authored check key is present and
// no existing job runs that check. The planner can call this before selection.
func (r *Registry) ImpliedJobs(c model.PipelineConfig) map[string]model.Job {
	jobs := map[string]model.Job{}
	for name, job := range c.Jobs {
		jobs[name] = job
	}
	for _, d := range r.All() {
		if d.ImpliedBy == "" || !present(c, d.ImpliedBy) {
			continue
		}
		run := false
		for _, j := range jobs {
			for _, step := range j.Run {
				for _, token := range strings.Fields(step) {
					if token == d.ID {
						run = true
					}
				}
			}
		}
		if run {
			continue
		}
		name := strings.TrimPrefix(d.ID, "checks.")
		if i := strings.IndexByte(name, '.'); i >= 0 {
			name = name[:i]
		}
		if _, exists := jobs[name]; exists {
			name = "check-" + strings.ReplaceAll(d.ID, ".", "-")
			for n := 2; ; n++ {
				if _, taken := jobs[name]; !taken {
					break
				}
				name = "check-" + strings.ReplaceAll(d.ID, ".", "-") + "-" + strconv.Itoa(n)
			}
		}
		jobs[name] = model.Job{Kind: "quality", Run: []string{d.ID}, Required: boolPtr(true), Path: d.Path}
	}
	return jobs
}

func boolPtr(v bool) *bool { return &v }
func present(c model.PipelineConfig, key string) bool {
	parts := strings.Split(key, ".")
	if len(parts) < 2 || parts[0] != "checks" {
		return false
	}
	id := strings.Join(parts[1:len(parts)-1], ".")
	field := parts[len(parts)-1]
	def, ok := c.Checks[id]
	if !ok {
		return false
	}
	switch field {
	case "rules":
		return def.Rules != nil
	case "required":
		return def.Required != nil
	case "coverage_floor":
		return def.CoverageFloor != nil
	}
	return false
}

var ErrDuplicate = errors.New("duplicate check id")

// Ecosystem names the detector ecosystem a check belongs to: the part of its
// id before the first dot or dash (`go.test` is go, `python-test` is python),
// with the Xcode check counted under swift. `detect.disable` uses it.
func Ecosystem(id string) string {
	if strings.HasPrefix(id, "xcode-") {
		return "swift"
	}
	if i := strings.IndexAny(id, ".-"); i >= 0 {
		return id[:i]
	}
	return id
}
