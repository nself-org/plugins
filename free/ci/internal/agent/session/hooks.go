package session

import (
	"context"
	"fmt"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// JobRoot is the agent-owned directory of one lease, removed when the lease
// ends. Hooks may stage files in it; a job sees only what a JobSetup mounts.
type JobRoot struct{ Dir string }

// JobSetup is what a Before hook contributes to the job it precedes.
type JobSetup struct {
	// SourceDir replaces the job source tree when set.
	SourceDir string
	Mounts    []exec.Mount
	Env       map[string]string
	// Secrets are resolved values for the job's declared secret names.
	Secrets map[string]string
}

// Outcome is how a job ended, as After hooks and the result see it.
type Outcome struct {
	State        string
	ExitCode     int
	FailureClass string
}

// Uploader sends transfer frames for the current lease through the session's
// carrier: digest, offset, chunk, artifact-ref and cache-ref.
type Uploader interface {
	Digest(protocol.Digest) error
	Offset(protocol.Offset) error
	Chunk(protocol.Chunk) error
	ArtifactRef(protocol.ArtifactRef) error
	CacheRef(protocol.CacheRef) error
}

// JobHook runs in the agent around a job, outside the job's UID and sandbox
// (CACHE delta 5): Before stages inputs, After uploads outputs. After runs
// even when the job failed, and a Before error fails the lease.
type JobHook struct {
	Name   string
	Before func(ctx context.Context, lease protocol.Lease, root JobRoot) (JobSetup, error)
	After  func(ctx context.Context, lease protocol.Lease, root JobRoot, out Outcome, up Uploader) error
}

var (
	hookMu sync.RWMutex
	hooks  []JobHook
)

// RegisterJobHook adds a hook; hooks run in registration order. A duplicate or
// empty name is refused.
func RegisterJobHook(h JobHook) error {
	hookMu.Lock()
	defer hookMu.Unlock()
	if h.Name == "" {
		return fmt.Errorf("job hook needs a name")
	}
	for _, have := range hooks {
		if have.Name == h.Name {
			return fmt.Errorf("duplicate job hook %q", h.Name)
		}
	}
	hooks = append(hooks, h)
	return nil
}

func registeredHooks() []JobHook {
	hookMu.RLock()
	defer hookMu.RUnlock()
	return append([]JobHook(nil), hooks...)
}

func mergeSetup(into *JobSetup, s JobSetup) {
	if s.SourceDir != "" {
		into.SourceDir = s.SourceDir
	}
	into.Mounts = append(into.Mounts, s.Mounts...)
	for k, v := range s.Env {
		if into.Env == nil {
			into.Env = map[string]string{}
		}
		into.Env[k] = v
	}
	for k, v := range s.Secrets {
		if into.Secrets == nil {
			into.Secrets = map[string]string{}
		}
		into.Secrets[k] = v
	}
}
