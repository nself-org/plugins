package run

// Purpose: what happens around a leased attempt before and after the job
// process: start hooks, cache restore, services, container image, and the
// executor spec.
// Inputs:  the attempt info. Outputs: the job as it will run, and its spec.
// Constraints: a hook error fails the job with the error's code; services are
// always torn down with a context that survives Ctrl-C.

import (
	"context"
	"encoding/json"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
	"github.com/nself-org/plugins/free/ci/internal/release/job"
)

type prepared struct {
	job      model.Job
	services *ServicesRequest
	mounts   []Mount
}

// prepare runs the start hooks, restores the cache and starts services.
func (rs *runState) prepare(ctx context.Context, info *AttemptInfo) (prepared, error) {
	p := prepared{job: info.Job}
	env := map[string]string{}
	for k, v := range info.Job.Env {
		env[k] = v
	}
	for _, h := range rs.hooks.AttemptStarts {
		if err := guard("attempt-start hook", func() error { return h.AttemptStart(ctx, *info) }); err != nil {
			return p, err
		}
	}
	if rs.hooks.Cache != nil {
		var setup CacheSetup
		err := guard("cache hook", func() (e error) { setup, e = rs.hooks.Cache.Before(ctx, *info); return })
		if err != nil {
			return p, err
		}
		for k, v := range setup.Env {
			env[k] = v
		}
		p.mounts = setup.Mounts
		if len(setup.Mounts) > 0 {
			b, _ := json.Marshal(setup.Mounts)
			env["NSELF_CI_MOUNTS"] = string(b)
		}
	}
	if names := append(append([]string(nil), info.Job.NeedsServices...), rs.c.Opts.Services...); len(names) > 0 {
		if rs.hooks.Services == nil {
			return p, fail(CodeOptions, "job needs services %v and no services hook is registered", names)
		}
		iso := info.Job.Isolation
		if rs.c.Opts.Container {
			iso = "container"
		}
		req := ServicesRequest{RunID: info.RunID, AttemptID: info.AttemptID, Names: names, Specs: ServiceSpecsAt(rs.c.Opts.Root, names),
			Isolation: iso, Network: model.NetworkScope(rs.c.Opts.Network)}
		var up ServicesSetup
		err := guard("services hook", func() (e error) { up, e = rs.hooks.Services.Up(ctx, req); return })
		if err != nil {
			return p, err
		}
		p.services = &req
		for k, v := range up.Env {
			env[k] = v
		}
	}
	if info.IdempotencyKey != "" {
		env["NSELF_CI_IDEMPOTENCY_KEY"] = info.IdempotencyKey
	}
	env["NSELF_CI_RUN_ID"] = info.RunID
	p.job.Env = env
	if p.job.Kind == "release" { // the final environment, after the cache and services hooks: no isolator can skip it
		if e := job.CheckDeclared(p.job); e != nil {
			return p, releaseFailure(e)
		}
	}
	if rs.c.Opts.Container {
		img := ""
		if p.job.Container != nil {
			img = p.job.Container.Image
		}
		if img == "" && rs.hooks.Image != nil {
			var err error
			if err = guard("image hook", func() (e error) { img, e = rs.hooks.Image.Image(ctx, *info); return }); err != nil {
				return p, err
			}
		}
		if img == "" {
			return p, fail(CodeOptions, "--container needs a pinned toolchain image and none is registered")
		}
		p.job.Isolation, p.job.Container = "container", &model.ContainerConfig{Image: img}
	}
	return p, nil
}

func (rs *runState) unprepare(info AttemptInfo, p prepared) {
	if p.services != nil && rs.hooks.Services != nil {
		bg, done := background(context.Background())
		defer done()
		_ = guard("services down hook", func() error { return rs.hooks.Services.Down(bg, *p.services) })
	}
}

func (rs *runState) spec(ctx context.Context, pj plan.PlannedJob, job model.Job, id string) ciexec.JobSpec {
	s := ciexec.JobSpec{AttemptID: id, CoordinatorID: safeID(rs.sess.Coordinator.ID), Job: job, Command: job.Run,
		SourceDir: rs.c.Opts.Root, Home: rs.sess.Home, Network: model.NetworkScope(rs.c.Opts.Network)}
	cpu, mem := demandOf(&job)
	s.CPU, s.MemoryMB = float64(cpu), mem // a container gets exactly its reservation, not the engine default
	if job.Requirements != nil && job.Requirements.MemMB > 0 {
		s.MemoryLimitMB = job.Requirements.MemMB // only a declared demand caps a process job; the default is admission-only
	}
	if rs.c.Opts.Timeout > 0 {
		s.Timeout = rs.c.Opts.Timeout
	}
	return s
}
