package main

import (
	"context"
	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

// A delivery raised to container isolation uses the local run's pinned-image hook.
func agentJobImage(ctx context.Context, spec execSpec, hook cirun.ContainerImageHook) (model.Job, error) {
	job := spec.Job
	if job.Isolation != "container" && job.Isolation != "sandboxed-container" {
		return job, nil
	}
	if job.Container != nil && job.Container.Image != "" {
		return job, nil
	}
	var image string
	var err error
	if hook != nil {
		image, err = hook.Image(ctx, cirun.AttemptInfo{AttemptID: spec.AttemptID, JobID: spec.JobID, Revision: spec.Revision, Job: job, Container: true})
	}
	if err != nil || image == "" {
		return job, &ciexec.Error{Code: "E613", Message: "remote container job needs a pinned toolchain image; configure NSELF_CI_IMAGE on the agent", Err: err}
	}
	container := model.ContainerConfig{}
	if job.Container != nil {
		container = *job.Container
	}
	container.Image = image
	job.Container = &container
	return job, nil
}
