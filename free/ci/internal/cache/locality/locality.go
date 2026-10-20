// Package locality is the `cache` world contributor (Epic CACHE D8): the bytes a runner would
// fetch for a job's cache entries, handed to the scheduler as sched.World.Cache.
package locality

// Purpose: tell the placement scorer how much cache data each runner is missing for each job, so
// work lands where its caches already are.
// Inputs:  a Source bound by the run or plan code (the jobs about to be placed with their
// detector facts, and a read-only Ledger of entries and node copies); the world with its runners.
// Outputs: World.Cache[runner id][job key] = CacheFact{MissBytes, Confidence}. The job key is the
// sched.Job key, `<job id>[<matrix key>]`.
// Constraints: read-only (CacheEntries and CacheLocality only: no touch, drop, audit or blob
// read, so a world build never changes the store), no network, no process spawn (the toolchain
// probe of a process-level job is the Source's, memoized per build), sorted and deterministic.
// 0 means the runner holds the entry: it is the coordinator host or the local store, or a node
// with a recorded read-through copy younger than RowMaxAge; otherwise the entry size. A job with
// no cache spec, or whose spec has no entry in any namespace the job may read, is unknown and
// carries a nil MissBytes: a zero is never reported for an unknown fact. When only some of a
// job's specs have an entry the sum covers those and the confidence is "estimated". With no
// Source bound the contributor leaves Cache empty (every fact unknown) and never fails a build.

import (
	"context"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/detect"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Name and Field are the world.Contributor registration of this package.
const (
	Name  = "cache"
	Field = "Cache"
)

// RowMaxAge is how old a recorded node copy may be before it is ignored. Nodes evict
// least-recently-used copies at lease start, so an old row is a weak hint; a row the contributor
// ignores only raises the reported miss (the safe direction).
const RowMaxAge = 7 * 24 * time.Hour

// Ledger is the read-only view of the shared store the contributor needs. *store.Store satisfies it.
type Ledger interface {
	CacheEntries(ctx context.Context, key string) ([]entries.Manifest, error)
	CacheLocality(ctx context.Context, node, realm, key string) ([]store.CacheLocalityRow, error)
}

// Job is one job as the scheduler is about to place it: the facts its cache keys are made of.
// Platform comes from the runner, not from the job.
type Job struct {
	Key       string // the sched.Job key
	Facts     entries.Facts
	Detected  []detect.Fact
	Scope     []string
	ExtraKeys []string
	Environ   []string
	Isolation string // "" means process
	Image     string // pinned image digest of a container job
	Container bool
	// Probe runs a toolchain probe of a process-level job. The Source supplies a cached one;
	// nil makes a spec that needs a probe unavailable (unknown), never keyed by a guess.
	Probe func(ctx context.Context, argv []string) (string, error)
}

// Inputs is what one world build reads.
type Inputs struct {
	Ledger Ledger
	Jobs   []Job
	// Warn receives a partial failure (one key could not be read); that key is then unknown.
	Warn func(error)
}

// Source returns the inputs of the world build now running. An error leaves every fact unknown.
type Source func(ctx context.Context) (Inputs, error)

var bound = struct {
	sync.RWMutex
	src Source
}{}

// Bind makes src the source the contributor reads and returns the function that unbinds it.
func Bind(src Source) (unbind func()) {
	bound.Lock()
	bound.src = src
	bound.Unlock()
	return func() {
		bound.Lock()
		bound.src = nil
		bound.Unlock()
	}
}

func current() Source {
	bound.RLock()
	defer bound.RUnlock()
	return bound.src
}
