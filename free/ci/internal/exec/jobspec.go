package exec

// Purpose: JobSpec, the local execution request the coordinator assembles for one attempt (moved out of
// executor.go, which the P7-HOST-07 entrypoint field would have kept over the 300 line limit).
// Inputs:  the fields below; Outputs: none (a declaration). Constraints: see each field.

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// JobSpec is the local execution request assembled by the coordinator.
type JobSpec struct {
	AttemptID, CoordinatorID string
	Job                      model.Job
	Command                  []string
	SourceDir                string
	Mounts                   []Mount
	Home                     string
	Timeout                  time.Duration
	Network                  model.NetworkScope
	CPU                      float64
	MemoryMB                 int
	// MemoryLimitMB caps a process job's writable memory (RLIMIT_DATA on Linux). Zero means no cap:
	// MemoryMB alone is a reservation, enforced by container isolation, never by RLIMIT_AS.
	MemoryLimitMB int
	Output        func([]byte)
	// Stdin, when set, is the job's standard input: how a one-job credential reaches a runner without touching
	// argv, a container -e flag or a file (a planned container runs interactive, env on fd 3). Nil keeps /dev/null.
	Stdin io.Reader
	// RunnerRoot gives a planned container the exec-capable runner tmpfs (RunnerRootPath), memory only.
	RunnerRoot bool
	// Entrypoint, when set, replaces the container image's entrypoint (`--entrypoint`), so a runner image
	// whose own entrypoint is a launcher (the gitlab-runner image) still starts the credential wrapper. Only
	// the agent binary mounted for a runner credential is accepted by the backstop. Containers only.
	Entrypoint string
	// Masks are exact values the output must never carry (the value Stdin delivers), even split across chunks.
	Masks []string
	// Slot is the execution slot the attempt holds; isolation profiles derive the job UID from it.
	Slot int
	// Plan, when an Isolator sets it, builds the container command instead of the baseline flags.
	Plan ContainerPlanner
	// BeforeJob, when set, runs after the source is copied into the workspace
	// and before the command starts. It receives the workspace root opened by
	// the executor, so a path swapped later cannot redirect it. An error ends
	// the attempt as failed, class infra, and the command never starts.
	BeforeJob func(ctx context.Context, workspace *os.Root) error
	// AfterJob, when set, runs once the command exited 0 and before the
	// workspace is removed (the only moment a job's outputs can be collected).
	// It receives the workspace root opened by the executor before the job
	// started, so a path swapped by the job cannot redirect it. An error ends
	// the attempt as failed, class infra, with that error.
	AfterJob func(ctx context.Context, workspace *os.Root) error
	// Env, when an Isolator sets it, replaces the executor's EnvPolicy for this job (ad-hoc jobs
	// get a minimal environment, P7-CI-52); job-declared env and secrets are still added after it.
	Env EnvPolicy
	// Secrets, when set, resolves this attempt's references instead of Executor.Secrets (P7-TRUST-02:
	// the source is built per attempt, so it knows the attempt, its trust facts and its node).
	Secrets SecretSource
	// Release, when an Isolator sets it, runs once after the attempt ended on any path (pass, failure, cancel),
	// with a context that survives the attempt's cancellation: it removes what the isolation profile created
	// for this attempt outside the container (P7-TRUST-08: the job network and its proxy sidecar).
	Release func(ctx context.Context)
	// NetEnv, when an Isolator sets it, is added to the job environment after the job's own env, so the job
	// cannot override it: the proxy variables of a restricted or internet scope (P7-TRUST-08).
	NetEnv []string
}
