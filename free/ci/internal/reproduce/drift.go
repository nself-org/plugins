package reproduce

// Purpose: the report printed before a reproduction runs: what the original
// run recorded, what this host and checkout look like now, and every
// difference (tool versions, inputs that changed, isolation not available).
// Inputs:  a Source, the repository root, git access and a version probe.
// Outputs: a Report; E634 as an error when the recorded platform differs.
// Constraints: read-only; probes only the tools the evidence names, with the
// same probe argv and trimming the gate runner used when it recorded them;
// a tool that cannot be probed is reported as missing, never as a failure;
// the unbound warning (E635) is a warning line, not an error.

import (
	"context"
	"runtime"
	"sort"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/evidence"
	"github.com/nself-org/plugins/free/ci/internal/gates"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// Drift kinds.
const (
	DriftTool      = "tool_version"
	DriftToolGone  = "tool_missing"
	DriftInputs    = "inputs_changed"
	DriftIsolation = "isolation"
)

// Drift is one difference between the recording and now.
type Drift struct {
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Recorded string `json:"recorded,omitempty"`
	Current  string `json:"current,omitempty"`
}

// Report is everything known before the job runs.
type Report struct {
	Job          string          `json:"job"`
	Pipeline     string          `json:"pipeline"`
	Revision     string          `json:"revision"`
	Platform     string          `json:"platform"`
	HostPlatform string          `json:"host_platform"`
	Isolation    model.Isolation `json:"isolation"`
	Bound        bool            `json:"bound"`
	Services     []string        `json:"services,omitempty"`
	Drift        []Drift         `json:"drift"`
	Warnings     []string        `json:"warnings"`
}

// HostPlatform is the platform string evidence records.
func HostPlatform() string { return runtime.GOOS + "/" + runtime.GOARCH }

// Preflight reads the recording against this host. On a platform mismatch it
// returns the partial report with the E634 error.
func Preflight(ctx context.Context, env Env, src Source) (Report, error) {
	doc := src.Doc
	host := firstOf(env.Platform, HostPlatform())
	rep := Report{Job: src.JobID, Pipeline: doc.Pipeline.Name, Revision: doc.Revision, Platform: doc.Runner.Platform, HostPlatform: host,
		Isolation: doc.Runner.Isolation, Bound: doc.Binding.State == "bound", Drift: []Drift{}, Warnings: []string{}}
	if doc.Runner.Platform != "" && doc.Runner.Platform != host {
		return rep, fail(CodePlatform, "job %s ran on %s and this host is %s; reproduce it on a %s host", src.JobID, doc.Runner.Platform, host, doc.Runner.Platform)
	}
	if doc.Revision == "" {
		return rep, fail(evidence.CodeNone, "the run recorded no revision, so there is nothing to reproduce")
	}
	if _, err := evidence.ParseRevision(doc.Revision); err != nil {
		return rep, fail(evidence.CodeRejected, "the run recorded an unusable revision: %v", err)
	}
	if id, ok := evidence.RepoID(ctx, env.Git, env.Root); ok && doc.RepoID != "" && doc.RepoID != id {
		return rep, fail(evidence.CodeRejected, "run %s belongs to another repository (%s); run this command inside that repository", src.Run.ID, doc.RepoID)
	}
	if !evidence.CommitExists(ctx, env.Git, env.Root, doc.Revision) {
		return rep, fail(evidence.CodeUsage, "%s is not a commit of this repository; fetch it first", short(doc.Revision))
	}
	if !rep.Bound {
		why := "uncommitted changes"
		if doc.Binding.Reason != nil {
			why = string(*doc.Binding.Reason)
		}
		rep.Warnings = append(rep.Warnings, warn(CodeUnbound, "the original run was not bound to its commit (%s); reproducing the recorded revision %s only", why, short(doc.Revision)))
	}
	rep.Drift = append(rep.Drift, inputDrift(ctx, env, doc, &rep)...)
	rep.Drift = append(rep.Drift, toolDrift(ctx, env, src)...)
	if iso := doc.Runner.Isolation; iso != "" && iso != "process" && iso != "container" {
		rep.Drift = append(rep.Drift, Drift{Kind: DriftIsolation, Name: "isolation", Recorded: string(iso), Current: "process"})
	}
	return rep, nil
}

func inputDrift(ctx context.Context, env Env, doc model.Evidence, rep *Report) []Drift {
	cur, err := evidence.InputsAt(ctx, env.Git, env.Root, doc.Revision)
	if err != nil {
		rep.Warnings = append(rep.Warnings, "the inputs at the recorded revision cannot be read: "+firstLine(err.Error()))
		return nil
	}
	var out []Drift
	if cur.Config != doc.InputDigests.Config {
		out = append(out, Drift{Kind: DriftInputs, Name: "config", Recorded: doc.InputDigests.Config, Current: cur.Config})
	}
	out = append(out, refDrift("lockfile", doc.InputDigests.Lockfiles, cur.Lockfiles)...)
	return append(out, refDrift("generated", doc.InputDigests.Generated, cur.Generated)...)
}

// refDrift compares two lists of path digests; a path on one side only is a
// change with the other side empty.
func refDrift(label string, recorded, current []model.DigestRef) []Drift {
	rec, cur := map[string]string{}, map[string]string{}
	for _, r := range recorded {
		rec[r.Path] = r.SHA256
	}
	for _, c := range current {
		cur[c.Path] = c.SHA256
	}
	var out []Drift
	for path, sum := range rec {
		if cur[path] != sum {
			out = append(out, Drift{Kind: DriftInputs, Name: label + " " + path, Recorded: sum, Current: cur[path]})
		}
	}
	for path, sum := range cur {
		if _, ok := rec[path]; !ok {
			out = append(out, Drift{Kind: DriftInputs, Name: label + " " + path, Current: sum})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// toolProbe names one recorded tool version and the argv that reads it.
type toolProbe struct {
	name, recorded string
	argv           []string
}

func toolProbes(src Source) []toolProbe {
	reg := gates.DefaultRegistry()
	seen := map[string]bool{}
	var out []toolProbe
	for _, c := range src.Checks {
		if c.ToolVersion == nil || seen[c.Tool] {
			continue
		}
		seen[c.Tool] = true
		argv := []string{c.Tool, "--version"}
		if d, ok := reg.Get(c.ID); ok {
			argv = d.VersionProbe
		}
		out = append(out, toolProbe{c.Tool, *c.ToolVersion, argv})
	}
	names := make([]string, 0, len(src.Doc.ToolVersions))
	for name := range src.Doc.ToolVersions {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if !seen[name] {
			seen[name] = true
			out = append(out, toolProbe{name, src.Doc.ToolVersions[name], []string{name, "--version"}})
		}
	}
	return out
}

func toolDrift(ctx context.Context, env Env, src Source) []Drift {
	probe := env.Probe
	if probe == nil {
		probe = defaultProbe
	}
	var out []Drift
	for _, p := range toolProbes(src) {
		got, exit, err := probe(ctx, p.argv...)
		got = strings.TrimSpace(got)
		if len(got) > 120 { // the gate runner stored at most this much
			got = got[:120]
		}
		switch {
		case err != nil || exit != 0:
			out = append(out, Drift{Kind: DriftToolGone, Name: p.name, Recorded: p.recorded})
		case got != p.recorded:
			out = append(out, Drift{Kind: DriftTool, Name: p.name, Recorded: p.recorded, Current: got})
		}
	}
	return out
}

func firstOf(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

func short(rev string) string {
	if len(rev) > 12 {
		return rev[:12]
	}
	return rev
}
