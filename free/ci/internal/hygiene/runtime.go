package hygiene

// Purpose: the container runtime view hygiene needs: attempt-labelled containers, networks and
// volumes, listed and removed through the internal/exec funnel.
// Inputs:  an attempt id (or "" for every labelled resource).
// Outputs: labelled resources; removals by id from a prior listing.
// Constraints: a runtime that is not installed, or whose daemon is down, has no live resources to
// find: it reads as empty rather than as a failure (a stopped daemon cannot be running a job).

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/exec"
)

// Runtime lists and removes the resources labelled with an attempt id.
type Runtime interface {
	List(ctx context.Context, kind exec.ResourceKind, attempt string) ([]exec.Labelled, error)
	Remove(ctx context.Context, kind exec.ResourceKind, id string) error
}

// Kinds is every resource kind hygiene sweeps, containers first so networks and volumes are free.
var Kinds = []exec.ResourceKind{exec.KindContainer, exec.KindNetwork, exec.KindVolume}

// ExecRuntime is the Runtime backed by the docker or podman CLI.
type ExecRuntime struct {
	// Binaries are tried in order; empty means docker then podman.
	Binaries []string
	mu       sync.Mutex
	usable   map[string]probeResult
}

type probeResult struct {
	ok bool
	at time.Time
}

const probeTTL = 15 * time.Second

var defaultRuntime Runtime = &ExecRuntime{}

func (r *ExecRuntime) binaries() []string {
	if len(r.Binaries) > 0 {
		return r.Binaries
	}
	return []string{"docker", "podman"}
}

// available reports whether the binary is installed and its daemon answers `info`.
func (r *ExecRuntime) available(ctx context.Context, bin string) (bool, error) {
	r.mu.Lock()
	cached, hit := r.usable[bin]
	r.mu.Unlock()
	if hit && time.Since(cached.at) < probeTTL {
		return cached.ok, nil
	}
	_, err := exec.RuntimeQuery(ctx, bin, "info")
	ok := err == nil
	if err != nil && ctx.Err() != nil {
		return false, ctx.Err()
	}
	r.mu.Lock()
	if r.usable == nil {
		r.usable = map[string]probeResult{}
	}
	r.usable[bin] = probeResult{ok: ok, at: time.Now()}
	r.mu.Unlock()
	return ok, nil
}

// List returns the labelled resources of every usable runtime, tagged by kind through the call.
func (r *ExecRuntime) List(ctx context.Context, kind exec.ResourceKind, attempt string) ([]exec.Labelled, error) {
	var out []exec.Labelled
	for _, bin := range r.binaries() {
		ok, err := r.available(ctx, bin)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		found, err := exec.RuntimeListLabelled(ctx, bin, kind, attempt)
		if err != nil {
			return nil, err
		}
		for _, f := range found {
			f.ID = bin + ":" + f.ID
			out = append(out, f)
		}
	}
	return out, nil
}

// Remove removes one resource by the "<binary>:<id>" a List returned.
func (r *ExecRuntime) Remove(ctx context.Context, kind exec.ResourceKind, id string) error {
	for _, bin := range r.binaries() {
		if len(id) > len(bin)+1 && id[:len(bin)+1] == bin+":" {
			return exec.RuntimeRemoveLabelled(ctx, bin, kind, id[len(bin)+1:])
		}
	}
	return errors.New("hygiene: resource id has no runtime prefix")
}
