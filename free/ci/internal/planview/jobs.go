package planview

// Purpose: map the planner's jobs to scheduler jobs, in the plan's
// topological order (layer, then id), so `ci plan` and `ci explain` ask the
// scheduler exactly what a run would.
// Inputs: Context (pipeline and revision facts) and the planned jobs.
// Outputs: []sched.Job, one per planned job.
// Constraints: pure; secret classes come from the `class/NAME` prefix of
// job.secrets and only the class is kept; an unparsable timeout falls back to
// DefaultTimeoutMs; the local owner's own checkout is trust class owner.

import (
	"sort"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
)

// DefaultTimeoutMs bounds a job that declares no timeout (30 minutes).
const DefaultTimeoutMs = int64(30 * 60 * 1000)

// DefaultNetwork is the network scope a job needs: jobs fetch dependencies.
const DefaultNetwork = model.NetworkScope("internet")

// Context carries the facts every job of one pipeline shares.
type Context struct {
	PipelineID      string
	Project         string
	Support         []string // the pipeline's support platforms; empty means the platforms the jobs use
	DefaultPlatform string   // platform of a job without a matrix entry
	Foreground      bool     // a person is waiting at a terminal
}

// PlannedJob is one job of the plan.
type PlannedJob struct {
	ID, Name, Platform string
	Layer              int
	Deps               []string
	Job                model.Job
}

// SchedJobs returns the scheduler jobs in plan order.
func SchedJobs(c Context, jobs []PlannedJob) []sched.Job {
	ordered := append([]PlannedJob(nil), jobs...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Layer != ordered[j].Layer {
			return ordered[i].Layer < ordered[j].Layer
		}
		return ordered[i].ID < ordered[j].ID
	})
	platform := func(pj PlannedJob) string {
		if pj.Platform != "" {
			return pj.Platform
		}
		return c.DefaultPlatform
	}
	support := append([]string(nil), c.Support...)
	if len(support) == 0 {
		seen := map[string]bool{}
		for _, pj := range ordered {
			if p := platform(pj); p != "" && !seen[p] {
				seen[p] = true
				support = append(support, p)
			}
		}
		sort.Strings(support)
	}
	band := "normal"
	if c.Foreground {
		band = "interactive"
	}
	out := make([]sched.Job, len(ordered))
	for i, pj := range ordered {
		out[i] = schedJob(c, band, support, platform(pj), pj)
	}
	return out
}

func schedJob(c Context, band string, support []string, platform string, pj PlannedJob) sched.Job {
	j := pj.Job
	req := model.PlacementRequirements{Platform: platform, IsolationMin: j.Isolation, Network: DefaultNetwork}
	var accelerators []string
	if j.Requirements != nil {
		req.Labels = append([]string(nil), j.Requirements.Labels...)
		req.Tools = append([]string(nil), j.Requirements.Tools...)
		req.CPU, req.MemMB = int64(j.Requirements.CPU), int64(j.Requirements.MemMB)
		accelerators = append(accelerators, j.Requirements.Accelerators...)
	}
	if j.Isolation == "container" || j.Container != nil {
		req.Tools = appendOnce(req.Tools, "docker")
	}
	sort.Strings(req.Labels)
	sort.Strings(req.Tools)
	timeout := DefaultTimeoutMs
	if d, err := time.ParseDuration(j.Timeout); err == nil && d > 0 {
		timeout = d.Milliseconds()
	}
	priority := string(j.Priority)
	if priority == "" {
		priority = "normal"
	}
	return sched.Job{
		Key: pj.ID, ID: pj.ID, Name: pj.Name, MatrixKey: pj.Platform, Deps: append([]string(nil), pj.Deps...),
		Pipeline:     sched.Pipeline{ID: c.PipelineID, Support: support, Band: band, Foreground: c.Foreground, Release: j.Kind == "release"},
		Requirements: req, Accelerators: accelerators, Trust: "owner", SecretClasses: secretClasses(j.Secrets), Network: DefaultNetwork,
		TimeoutMs: timeout, Attempt: 1, Priority: priority, RemoteOnly: j.RemoteOnly, PersonalOwnRun: true, Project: c.Project,
	}
}

// secretClasses keeps the sorted, de-duplicated class of each `class/NAME`.
func secretClasses(secrets []string) []model.SecretClass {
	seen := map[model.SecretClass]bool{}
	var out []model.SecretClass
	for _, s := range secrets {
		class := model.SecretClass(strings.SplitN(s, "/", 2)[0])
		if class.Valid() && class != "none" && !seen[class] {
			seen[class] = true
			out = append(out, class)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func appendOnce(s []string, v string) []string {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}
