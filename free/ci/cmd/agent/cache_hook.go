package main

// Purpose: restore and collect a remote lease's coordinator-planned cache.
// Inputs: lease cache plan, agent state, pinned workspace and registered secrets.
// Outputs: tool environment, read-only container mounts and scanned cache uploads.
// Constraints: a failed or cancelled job sends nothing; a secret in any planned
// directory refuses the entire save set before the first transfer frame.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/cache/remote"
	"github.com/nself-org/plugins/free/ci/internal/cache/specs"
	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

func init() {
	must(session.RegisterJobHook(session.JobHook{Name: "cache", Before: cacheBefore, After: cacheAfter}))
}

type nodeCache struct {
	lease     protocol.Lease
	stage     cacheStage
	base      *os.Root
	work      *os.Root
	miss      map[string]bool
	container bool
}

var nodeCaches sync.Map

func cacheBefore(ctx context.Context, l protocol.Lease, root session.JobRoot) (session.JobSetup, error) {
	setup := session.JobSetup{Env: map[string]string{}}
	if l.Cache == nil || l.Cache.Access == "none" {
		return setup, nil
	}
	if l.Cache.Access != "rw" && l.Cache.Access != "ro" {
		return setup, fmt.Errorf("E711 not_planned: invalid cache access")
	}
	ns, err := remote.DecodePlanNamespace(l.Cache.Namespace)
	if err != nil {
		return setup, err
	}
	container := l.Job.Isolation == "container" || l.Job.Isolation == "sandboxed-container"
	if ns.Trust == "untrusted" && l.Cache.Access == "rw" {
		return setup, fmt.Errorf("E711 not_planned: untrusted writable cache")
	}
	if l.Cache.Access == "ro" && !container {
		return setup, nil
	}
	if err := validateCachePlan(l.Cache); err != nil {
		return setup, err
	}
	state, err := stateDir()
	if err != nil {
		return cacheSetupMiss(err)
	}
	copyStore, err := remote.NewReadThrough(filepath.Join(state, "cache-readthrough"), 0)
	if err != nil {
		return cacheSetupMiss(err)
	}
	if err = copyStore.Prune(); err != nil {
		return cacheSetupMiss(err)
	}
	dir := filepath.Join(root.Dir, "cache-work")
	if err = os.Mkdir(dir, 0755); err != nil {
		return cacheSetupMiss(err)
	}
	base, err := os.OpenRoot(dir)
	if err != nil {
		return cacheSetupMiss(err)
	}
	n := &nodeCache{lease: l, base: base, miss: map[string]bool{}, container: container}
	good := false
	defer func() {
		if !good {
			_ = base.Close()
		}
	}()
	fetchRestores(ctx, copyStore, l)
	seen := map[string]bool{}
	for _, restore := range l.Cache.Restore {
		sp, ok := specs.Get(restore.Spec)
		if !ok || restore.Dir != restore.Spec {
			return setup, fmt.Errorf("E711 not_planned: invalid restore spec")
		}
		seen[sp.ID] = true
		valid := len(restore.Blobs) > 0
		for _, d := range sp.Dirs {
			if err := base.MkdirAll(filepath.Join(sp.ID, d.Name), 0755); err != nil {
				cacheWarn(err)
				valid = false
				break
			}
		}
		for _, b := range restore.Blobs {
			known := false
			for _, d := range sp.Dirs {
				known = known || d.Name == b.Path
			}
			if !known {
				return setup, fmt.Errorf("E711 not_planned: invalid cache blob path")
			}
			if !valid {
				break
			}
			// fetchRestores filled the read-through copies; a blob still missing
			// is a cold miss (nil fetch), and coordinator paths are never read.
			f, err := copyStore.Open(ctx, l.Cache.Namespace, b.SHA256, b.Size, nil)
			if err != nil {
				valid = false
				break
			}
			lim := authz.DefaultLimits()
			lim.AllowDotGit = true
			_, err = specs.Unpack(f, base, filepath.Join(sp.ID, b.Path), lim, specs.UnpackOptions{OtherRead: container})
			_ = f.Close()
			if err != nil {
				valid = false
				break
			}
		}
		if !valid {
			if err := specs.RemoveIn(base, sp.ID); err != nil {
				cacheWarn(err)
			}
			// A missing/partial restore binds nothing. Tool defaults remain
			// writable and a partial directory is never exposed to the job.
			continue
		}
		if container {
			_ = base.Chmod(sp.ID, 0755)
		}
		bindCache(&setup, sp, dir, container)
	}
	if l.Cache.Access == "rw" {
		var saves []protocol.CacheSave
		for _, save := range l.Cache.Save {
			if seen[save.Spec] {
				continue
			}
			sp, ok := specs.Get(save.Spec)
			if !ok {
				return setup, fmt.Errorf("E711 not_planned: invalid save spec")
			}
			if container {
				bindWorkspaceCache(&setup, sp)
			} else {
				ready := true
				for _, d := range sp.Dirs {
					if err = base.MkdirAll(filepath.Join(sp.ID, d.Name), 0700); err != nil {
						cacheWarn(err)
						ready = false
						break
					}
				}
				if !ready {
					continue
				}
				bindCache(&setup, sp, dir, false)
			}
			n.miss[save.Spec] = true
			saves = append(saves, save)
		}
		plan := *l.Cache
		plan.Save = saves
		n.lease.Cache = &plan
		if len(saves) > 0 {
			stage, err := remote.NewNodeStage(filepath.Join(root.Dir, "cache-stage"), n.lease)
			if err != nil {
				cacheWarn(err)
			} else {
				stage.Copies = copyStore
				n.stage = stage
			}
		}
	}
	if old, loaded := nodeCaches.Swap(l.AttemptID, n); loaded {
		closeNodeCache(old.(*nodeCache))
	}
	good = true
	return setup, nil
}

func bindCache(setup *session.JobSetup, sp specs.Spec, dir string, container bool) {
	if sp.RootEnv != "" {
		host := filepath.Join(dir, sp.ID)
		setup.Env[sp.RootEnv] = host
		if container {
			setup.Mounts = append(setup.Mounts, ciexec.Mount{Host: host, Target: host, ReadOnly: true})
		}
		return
	}
	for _, d := range sp.Dirs {
		host := filepath.Join(dir, sp.ID, d.Name)
		if d.Env != "" {
			setup.Env[d.Env] = host
		}
		if container {
			setup.Mounts = append(setup.Mounts, ciexec.Mount{Host: host, Target: host, ReadOnly: true})
		}
	}
}

func bindWorkspaceCache(setup *session.JobSetup, sp specs.Spec) {
	if sp.RootEnv != "" {
		setup.Env[sp.RootEnv] = filepath.Join(specs.WorkspaceCacheDir, sp.ID)
	}
	for _, d := range sp.Dirs {
		if d.Env != "" {
			setup.Env[d.Env] = filepath.Join(specs.WorkspaceCacheDir, sp.ID, d.Name)
		}
	}
}
