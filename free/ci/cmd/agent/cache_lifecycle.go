package main

// Purpose: best-effort workspace setup, collection and bounded remote cache upload.
// Inputs: validated lease cache state and executor-owned workspace/environment.
// Outputs: collector callbacks and cache frames; setup failures drop bindings.
// Constraints: cache errors never fail the job; uploads have a separate deadline.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/cache/remote"
	"github.com/nself-org/plugins/free/ci/internal/cache/specs"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

type cacheStage interface {
	Collect(context.Context, *os.Root, authz.ValueScanner) error
	Ship(context.Context, <-chan protocol.Offset, remote.CacheUploader) error
	Close() error
}

func cacheBeforeJob(spec execSpec, env map[string]string) func(context.Context, *os.Root) error {
	v, ok := nodeCaches.Load(spec.AttemptID)
	if !ok {
		return nil
	}
	n := v.(*nodeCache)
	if !n.container || len(n.miss) == 0 {
		return nil
	}
	return func(_ context.Context, ws *os.Root) error {
		if err := ws.Mkdir(specs.WorkspaceCacheName, 0755); err != nil {
			n.dropMisses(env, err)
			return nil
		}
		work, err := ws.OpenRoot(specs.WorkspaceCacheName)
		if err != nil {
			n.dropMisses(env, err)
			return nil
		}
		n.work = work
		_ = work.Chmod(".", 0755)
	nextSpec:
		for id := range n.miss {
			sp, _ := specs.Get(id)
			for _, d := range sp.Dirs {
				if err = work.MkdirAll(filepath.Join(id, d.Name), 0755); err != nil {
					n.dropMiss(id, env, err)
					continue nextSpec
				}
			}
			_ = work.Chmod(id, 0755)
		}
		return nil
	}
}

// agentAfterJob composes the artifact collector with the cache collector while
// the executor still owns the workspace. Cache refusal stays best effort.
func agentAfterJob(spec execSpec, in jobInputs) func(context.Context, *os.Root) error {
	artifact := artifactsAfterJob(spec, in)
	v, ok := nodeCaches.Load(spec.AttemptID)
	if artifact == nil && (!ok || v.(*nodeCache).stage == nil) {
		return nil
	}
	return func(ctx context.Context, ws *os.Root) error {
		if artifact != nil {
			if err := artifact(ctx, ws); err != nil {
				return err
			}
		}
		if v, ok := nodeCaches.Load(spec.AttemptID); ok {
			n := v.(*nodeCache)
			if n.stage != nil {
				base := n.base
				if n.container {
					base = n.work
				}
				if base != nil {
					if err := n.stage.Collect(ctx, base, secretScanner(in)); err != nil {
						if in.Output != nil {
							in.Output([]byte(fmt.Sprintln("cache save refused:", err)))
						} else {
							fmt.Fprintln(os.Stderr, "cache save refused:", err)
						}
					}
				}
			}
		}
		return nil
	}
}

func cacheAfter(ctx context.Context, l protocol.Lease, _ session.JobRoot, out session.Outcome, up session.Uploader) error {
	v, ok := nodeCaches.LoadAndDelete(l.AttemptID)
	if !ok {
		return nil
	}
	n := v.(*nodeCache)
	defer closeNodeCache(n)
	if out.State != "passed" || n.stage == nil {
		return nil
	}
	src, ok := up.(session.OffsetSource)
	if !ok {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, cacheShipTimeout)
	defer cancel()
	if err := n.stage.Ship(ctx, src.Offsets(), up); err != nil {
		fmt.Fprintln(os.Stderr, "cache upload refused:", err)
	}
	return nil
}

const cacheShipTimeout = 90 * time.Second

func cacheWarn(err error) { fmt.Fprintln(os.Stderr, "cache setup refused:", err) }

func cacheSetupMiss(err error) (session.JobSetup, error) {
	cacheWarn(err)
	return session.JobSetup{}, nil
}

func closeNodeCache(n *nodeCache) {
	if n.base != nil {
		_ = n.base.Close()
	}
	if n.work != nil {
		_ = n.work.Close()
	}
	if n.stage != nil {
		_ = n.stage.Close()
	}
}

func (n *nodeCache) dropMisses(env map[string]string, err error) {
	for id := range n.miss {
		n.dropMiss(id, env, err)
	}
}

func (n *nodeCache) dropMiss(id string, env map[string]string, err error) {
	cacheWarn(err)
	sp, _ := specs.Get(id)
	delete(env, sp.RootEnv)
	for _, d := range sp.Dirs {
		delete(env, d.Env)
	}
	delete(n.miss, id)
	if n.stage != nil {
		_ = n.stage.Close()
		n.stage = nil
	}
}

// Validate the complete plan before optional filesystem setup can degrade it.
func validateCachePlan(plan *protocol.LeaseCache) error {
	for _, restore := range plan.Restore {
		sp, ok := specs.Get(restore.Spec)
		if !ok || restore.Dir != restore.Spec {
			return fmt.Errorf("E711 not_planned: invalid restore spec")
		}
		for _, b := range restore.Blobs {
			found := false
			for _, d := range sp.Dirs {
				found = found || d.Name == b.Path
			}
			if !found {
				return fmt.Errorf("E711 not_planned: invalid cache blob path")
			}
		}
	}
	for _, save := range plan.Save {
		sp, ok := specs.Get(save.Spec)
		if !ok || len(save.Dirs) != len(sp.Dirs) {
			return fmt.Errorf("E711 not_planned: invalid save spec")
		}
		seen := map[string]bool{}
		for _, p := range save.Dirs {
			found := false
			for _, d := range sp.Dirs {
				found = found || p == filepath.Join(sp.ID, d.Name)
			}
			if !found || seen[p] {
				return fmt.Errorf("E711 not_planned: invalid save path")
			}
			seen[p] = true
		}
	}
	return nil
}

// fetchRestores takes the lease's restore blobs from the coordinator over the
// session's reverse blob path (carrier A) into the read-through copies. Without a
// feed (carrier B, or no restore) nothing is fetched: copies already held are still
// used, and a missing one is a cold miss that binds nothing.
func fetchRestores(ctx context.Context, copies *remote.ReadThrough, l protocol.Lease) {
	feed, ok := session.FeedFrom(ctx)
	if !ok {
		return
	}
	defer feed.Close()
	var blobs []protocol.CacheBlob
	for _, r := range l.Cache.Restore {
		blobs = append(blobs, r.Blobs...)
	}
	(&remote.NodeRestorer{Copies: copies, Feed: feed, Realm: l.Cache.Namespace}).Fetch(ctx, blobs)
}
