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

// ErrNoRuntime says no container runtime CLI is installed at all.
var ErrNoRuntime = errors.New("no container runtime is installed")

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
	switch {
	case errors.As(err, &down) && !h.RuntimeRequired:
		return found, string(kind) + "s: " + down.Error(), nil
	case errors.Is(err, ErrNoRuntime) && !h.RuntimeRequired:
		return found, "", nil // nothing is installed, so nothing can be running
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

// cachedProbe returns a runtime's state only while its probe window is fresh.
func (r *ExecRuntime) cachedProbe(bin string) (runtimeState, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cached, hit := r.usable[bin]
	return cached.state, hit && time.Since(cached.at) < probeTTL
}

// recordProbe starts a probe window after a listing or info establishes the runtime's state.
func (r *ExecRuntime) recordProbe(bin string, state runtimeState) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.usable == nil {
		r.usable = map[string]probeResult{}
	}
	r.usable[bin] = probeResult{state: state, at: time.Now()}
}

// available reports whether the binary is installed and its daemon answers `info`.
func (r *ExecRuntime) available(ctx context.Context, bin string) (runtimeState, error) {
	if state, hit := r.cachedProbe(bin); hit {
		return state, nil
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
	r.recordProbe(bin, state)
	return state, nil
}

// listBinary uses a fresh probe or lets the listing establish availability before asking info.
func (r *ExecRuntime) listBinary(ctx context.Context, bin string, kind exec.ResourceKind, attempt string) ([]exec.Labelled, runtimeState, error) {
	return r.listBinaryUsing(ctx, bin, kind, attempt, queryListing)
}

type listingQuery func(context.Context, string, exec.ResourceKind, string) listingResult

// listBinaryUsing applies the same probe policy to live and concurrently collected listings.
func (r *ExecRuntime) listBinaryUsing(ctx context.Context, bin string, kind exec.ResourceKind, attempt string, query listingQuery) ([]exec.Labelled, runtimeState, error) {
	state, hit := r.cachedProbe(bin)
	if hit && state != runtimeUp {
		return nil, state, nil
	}
	result := query(ctx, bin, kind, attempt)
	found, err := result.items, result.err
	if err == nil {
		if !hit {
			r.recordProbe(bin, runtimeUp)
		}
		return found, runtimeUp, nil
	}
	if result.contextErr != nil {
		return nil, runtimeDown, result.contextErr
	}
	if exec.IsRuntimeMissing(err) {
		r.recordProbe(bin, runtimeAbsent)
		return nil, runtimeAbsent, nil
	}
	if !hit {
		var probeErr error
		state, probeErr = r.available(ctx, bin)
		if probeErr != nil || state != runtimeUp {
			return nil, state, probeErr
		}
	}
	return nil, runtimeUp, err
}

// List returns the labelled resources of every usable runtime; an installed runtime that does not
// answer is reported as a RuntimeDownError next to what the others returned.
func (r *ExecRuntime) List(ctx context.Context, kind exec.ResourceKind, attempt string) ([]exec.Labelled, error) {
	return r.listUsing(ctx, kind, attempt, queryListing)
}

// listUsing reduces listings in binary order, including short-circuiting on the first error.
func (r *ExecRuntime) listUsing(ctx context.Context, kind exec.ResourceKind, attempt string, query listingQuery) ([]exec.Labelled, error) {
	var out []exec.Labelled
	var down []string
	present := 0
	for _, bin := range r.binaries() {
		found, state, err := r.listBinaryUsing(ctx, bin, kind, attempt, query)
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
		present++
		for _, f := range found {
			f.ID = bin + ":" + f.ID
			out = append(out, f)
		}
	}
	if len(down) > 0 {
		return out, &RuntimeDownError{Binaries: down}
	}
	if present == 0 {
		return nil, ErrNoRuntime
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

// BatchRuntime wraps a Runtime so that a pass over many attempts lists each resource kind once
// and answers per-attempt questions from that snapshot (a run of a hundred jobs would otherwise
// ask the daemon hundreds of times). Removals update the snapshot. Use one per pass, not for long.
func BatchRuntime(inner Runtime) Runtime {
	if inner == nil {
		inner = defaultRuntime
	}
	return &batchRuntime{inner: inner, snap: map[exec.ResourceKind]*snapshot{}}
}

type snapshot struct {
	items []exec.Labelled
	err   error
}

type batchRuntime struct {
	inner    Runtime
	mu       sync.Mutex
	snap     map[exec.ResourceKind]*snapshot
	parallel bool
}

func (b *batchRuntime) List(ctx context.Context, kind exec.ResourceKind, attempt string) ([]exec.Labelled, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.parallel && len(b.snap) == 0 {
		if r, ok := b.inner.(*ExecRuntime); ok {
			b.snap = r.listKinds(ctx)
		}
	}
	sn := b.snap[kind]
	if sn == nil {
		items, err := b.inner.List(ctx, kind, "")
		sn = &snapshot{items: items, err: err}
		b.snap[kind] = sn
	}
	var out []exec.Labelled
	for _, it := range sn.items {
		if attempt == "" || it.Attempt == attempt {
			out = append(out, it)
		}
	}
	return out, sn.err
}

func (b *batchRuntime) Remove(ctx context.Context, kind exec.ResourceKind, id, attempt string) error {
	if err := b.inner.Remove(ctx, kind, id, attempt); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if sn := b.snap[kind]; sn != nil {
		kept := sn.items[:0:0]
		for _, it := range sn.items {
			if it.ID != id {
				kept = append(kept, it)
			}
		}
		sn.items = kept
	}
	return nil
}
