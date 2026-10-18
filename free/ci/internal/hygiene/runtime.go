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
	"strings"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/exec"
)

// Runtime lists and removes the resources labelled with an attempt id.
type Runtime interface {
	List(ctx context.Context, kind exec.ResourceKind, attempt string) ([]exec.Labelled, error)
	// Remove removes the resource only while it still carries the attempt's label.
	Remove(ctx context.Context, kind exec.ResourceKind, id, attempt string) error
}

// RuntimeDownError says a container runtime is installed but did not answer (daemon stopped, socket
// not permitted, timeout). The resources it may hold could not be listed.
type RuntimeDownError struct{ Binaries []string }

func (e *RuntimeDownError) Error() string {
	return "container runtime installed but not answering: " + strings.Join(e.Binaries, ", ")
}

// listResources lists the labelled resources of one kind. A runtime that is installed but silent is
// a leak (error) when the host requires a runtime, and a skipped check (note) otherwise.
func (h *Host) listResources(ctx context.Context, kind exec.ResourceKind, attempt string) (found []exec.Labelled, note string, err error) {
	found, err = h.runtime().List(ctx, kind, attempt)
	var down *RuntimeDownError
	if errors.As(err, &down) && !h.RuntimeRequired {
		return found, string(kind) + "s: " + down.Error(), nil
	}
	return found, "", err
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

type runtimeState int

const (
	runtimeAbsent runtimeState = iota // not installed: nothing to find
	runtimeUp
	runtimeDown // installed, did not answer `info`
)

type probeResult struct {
	state runtimeState
	at    time.Time
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
func (r *ExecRuntime) available(ctx context.Context, bin string) (runtimeState, error) {
	r.mu.Lock()
	cached, hit := r.usable[bin]
	r.mu.Unlock()
	if hit && time.Since(cached.at) < probeTTL {
		return cached.state, nil
	}
	_, err := exec.RuntimeQuery(ctx, bin, "info")
	state := runtimeUp
	switch {
	case err == nil:
	case ctx.Err() != nil:
		return runtimeDown, ctx.Err()
	case exec.IsRuntimeMissing(err):
		state = runtimeAbsent
	default:
		state = runtimeDown
	}
	r.mu.Lock()
	if r.usable == nil {
		r.usable = map[string]probeResult{}
	}
	r.usable[bin] = probeResult{state: state, at: time.Now()}
	r.mu.Unlock()
	return state, nil
}

// List returns the labelled resources of every usable runtime; an installed runtime that does not
// answer is reported as a RuntimeDownError next to what the others returned.
func (r *ExecRuntime) List(ctx context.Context, kind exec.ResourceKind, attempt string) ([]exec.Labelled, error) {
	var out []exec.Labelled
	var down []string
	for _, bin := range r.binaries() {
		state, err := r.available(ctx, bin)
		if err != nil {
			return nil, err
		}
		if state == runtimeDown {
			down = append(down, bin)
			continue
		}
		if state == runtimeAbsent {
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
	if len(down) > 0 {
		return out, &RuntimeDownError{Binaries: down}
	}
	return out, nil
}

// Remove removes one resource by the "<binary>:<id>" a List returned.
func (r *ExecRuntime) Remove(ctx context.Context, kind exec.ResourceKind, id, attempt string) error {
	for _, bin := range r.binaries() {
		if len(id) > len(bin)+1 && id[:len(bin)+1] == bin+":" {
			return exec.RuntimeRemoveLabelled(ctx, bin, kind, id[len(bin)+1:], attempt)
		}
	}
	return errors.New("hygiene: resource id has no runtime prefix")
}
