package android

// Purpose: choose the pipeline the Android lane runs (Epic CACHE D18): a
// `kind: release` job named `android` in the project's config is used as it is;
// otherwise the job is synthesized and the pipeline `android-release` is the
// project's required jobs plus that job.
// Inputs:  the loaded config (the zero config when the project has none), the
// project directory, the timeout and the required job names of the project's
// default plan.
// Outputs: the config to plan with, the pipeline name and whether the job was
// synthesized.
// Constraints: pure; the input config is never changed; a job named `android`
// that is not a release job, or a pipeline named `android-release` without the
// job, is refused instead of being replaced.

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
)

// ErrConflict marks a config that already uses a lane name for something else.
var ErrConflict = errors.New("android lane name conflict")

// Input is what Resolve needs.
type Input struct {
	Config model.PipelineConfig
	// Workdir is the Android project relative to the repository root ("." for the root).
	Workdir string
	Timeout time.Duration
	// Required are the names of the project's required jobs (its default plan).
	Required []string
	// Env is the toolchain locations the synthesized job declares (EnvFrom).
	Env map[string]string
}

// Lane is the resolved pipeline.
type Lane struct {
	Config      model.PipelineConfig
	Pipeline    string
	Synthesized bool
}

// Resolve returns the lane for in.
func Resolve(in Input) (Lane, error) {
	cfg := cloneConfig(in.Config)
	if cfg.Version == 0 {
		cfg.Version = 1
	}
	existing, hasJob := cfg.Jobs[JobName]
	_, hasPipeline := cfg.Pipelines[PipelineName]
	switch {
	case hasJob && existing.Kind != "release":
		return Lane{}, fmt.Errorf("%w: job %q is kind %q; the Android lane needs a kind: release job of that name", ErrConflict, JobName, existing.Kind)
	case hasJob && hasPipeline:
		return Lane{Config: cfg, Pipeline: PipelineName}, nil
	case hasJob:
		cfg.Pipelines[PipelineName] = model.PipelineDef{Triggers: []model.Trigger{"local"}, Jobs: []string{JobName}}
		return Lane{Config: cfg, Pipeline: PipelineName}, nil
	case hasPipeline:
		return Lane{}, fmt.Errorf("%w: pipeline %q exists but the config has no %q job", ErrConflict, PipelineName, JobName)
	}
	deps := uniqueSorted(in.Required)
	cfg.Jobs[JobName] = Job(in.Workdir, deps, in.Timeout, in.Env)
	cfg.Pipelines[PipelineName] = model.PipelineDef{Triggers: []model.Trigger{"local"}, Jobs: append(append([]string(nil), deps...), JobName)}
	return Lane{Config: cfg, Pipeline: PipelineName, Synthesized: true}, nil
}

// cloneConfig copies the two maps Resolve adds to.
func cloneConfig(c model.PipelineConfig) model.PipelineConfig {
	out := c
	out.Jobs = make(map[string]model.Job, len(c.Jobs)+1)
	for k, v := range c.Jobs {
		out.Jobs[k] = v
	}
	out.Pipelines = make(map[string]model.PipelineDef, len(c.Pipelines)+1)
	for k, v := range c.Pipelines {
		out.Pipelines[k] = v
	}
	return out
}

func uniqueSorted(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if s != "" && s != JobName && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

// RequiredJobs lists the names of the required jobs a plan runs, once each: the
// jobs the lane's release job depends on. A release or drill job never counts.
func RequiredJobs(p *plan.Plan) []string {
	var names []string
	for _, j := range p.Jobs {
		if j.Job.Required != nil && *j.Job.Required && j.Job.Kind != "release" && j.Job.Kind != "drill" {
			names = append(names, j.Name)
		}
	}
	return uniqueSorted(names)
}
