package planview

// Purpose: map the planner's jobs to scheduler jobs, in the plan's
// topological order (layer, priority, then id), so `ci plan` and `ci explain` ask the
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
	Support         []string         // the pipeline's support platforms; empty means the platforms the jobs use
	DefaultPlatform string           // platform of a job without a matrix entry
	Trust           model.TrustClass // source classifier verdict; empty preserves the local caller default
	Foreground      bool             // a person is waiting at a terminal
}

// PlannedJob is one job of the plan. State, Blocked and Detail are the
// planner's own verdict (plan.PlannedJob); Tool is the catalogue check's tool.
type PlannedJob struct {
	ID, Name, Platform string
	Layer              int
	Deps               []string
	Job                model.Job
	Tool               string
	State              string // planned (or empty), skip or unplaceable
	Blocked, Detail    string
}

// Blocked is a job the planner already refused: it never reaches the scheduler.
type Blocked struct {
	Job          sched.Job
	Code, Detail string
}

// SchedJobs returns the jobs the scheduler places, in plan order, and the
// unplaceable jobs the planner refused (a skipped job needs no runner and is in
// neither list).
func SchedJobs(c Context, jobs []PlannedJob) ([]sched.Job, []Blocked) {
	ordered := append([]PlannedJob(nil), jobs...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Layer != ordered[j].Layer {
			return ordered[i].Layer < ordered[j].Layer
		}
		if priorityRank(ordered[i].Job.Priority) != priorityRank(ordered[j].Job.Priority) {
			return priorityRank(ordered[i].Job.Priority) < priorityRank(ordered[j].Job.Priority)
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
	band := bandOf(c)
	var out []sched.Job
	var blocked []Blocked
	for _, pj := range ordered {
		j := schedJob(c, band, support, platform(pj), pj)
		switch pj.State {
		case "", "planned":
			out = append(out, j)
		case "unplaceable":
			blocked = append(blocked, Blocked{Job: j, Code: pj.Blocked, Detail: pj.Detail})
		}
	}
	return out, blocked
}

// SchedJob maps one planned job exactly as SchedJobs does, for a caller that
// places jobs one at a time (a run). Support defaults to the job's own platform.
func SchedJob(c Context, pj PlannedJob) sched.Job {
	platform := pj.Platform
	if platform == "" {
		platform = c.DefaultPlatform
	}
	support := append([]string(nil), c.Support...)
	if len(support) == 0 && platform != "" {
		support = []string{platform}
	}
	return schedJob(c, bandOf(c), support, platform, pj)
}

func bandOf(c Context) string {
	if c.Foreground {
		return "interactive"
	}
	return "normal"
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
	if j.Isolation == "container" || j.Isolation == "sandboxed-container" || j.Container != nil || len(j.NeedsServices) > 0 {
		req.Tools = appendOnce(req.Tools, "docker")
	}
	if pj.Tool != "" {
		req.Tools = appendOnce(req.Tools, pj.Tool)
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
		Requirements: req, Accelerators: accelerators, Trust: classOf(c), SecretClasses: secretClasses(j.Secrets), Network: DefaultNetwork,
		TimeoutMs: timeout, Attempt: 1, Priority: priority, RemoteOnly: j.RemoteOnly, PersonalOwnRun: classOf(c) == "owner", Project: c.Project,
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

func classOf(c Context) model.TrustClass {
	if c.Trust == "" {
		return "owner"
	}
	return c.Trust
}

// Dependencies precede dependants; priority orders jobs ready in one layer.
func priorityRank(p model.Priority) int {
	switch p {
	case "high":
		return 0
	case "low":
		return 2
	default:
		return 1
	}
}
