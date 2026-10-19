package specs

// Purpose: the cache of a container job that missed (Epic CACHE D5, debt
// D-1032). The hardened container profile refuses a writable host mount (E611),
// so a miss cannot fill a mounted cache dir. Its dirs live in the job workspace
// instead, where the container already has a writable bind at /repo: BeforeJob
// creates them (and seeds a tool home), AfterJob packs them while the workspace
// still exists, and Finalize commits what AfterJob staged.
// Inputs:  the workspace root the executor opened before the job started.
// Outputs: Session.BeforeJob, Session.AfterJob, WorkspaceCacheDir.
// Constraints: the workspace cache dir is opened as a pinned os.Root before the
// job runs, so a job that swaps it for a symlink cannot steer the pack outside
// it; a failure of the cache itself never fails the job (it becomes a usage row
// and a warning); a hit keeps its read-only mount and is never saved again.

import (
	"context"
	"errors"
	"os"
)

// WorkspaceCacheName is the cache directory in the job workspace.
const WorkspaceCacheName = ".nself-ci-cache"

// WorkspaceCacheDir is WorkspaceCacheName as the container sees it (the
// workspace is bound at /repo).
const WorkspaceCacheDir = "/repo/" + WorkspaceCacheName

// BeforeJob creates the workspace cache dirs of every container miss.
func (s *Session) BeforeJob(ctx context.Context, ws *os.Root) error {
	var todo []*Item
	for _, it := range s.items {
		if it.InWorkspace {
			todo = append(todo, it)
		}
	}
	if len(todo) == 0 {
		return nil
	}
	if err := s.openWork(ws); err != nil {
		s.warn("cache dirs could not be created in the workspace: " + reasonOf(err))
		for _, it := range todo {
			it.Bound, it.InWorkspace = false, false
		}
		return nil
	}
	for _, it := range todo {
		for _, d := range it.Dirs {
			if err := s.work.MkdirAll(d, 0o755); err != nil {
				s.warn("cache dir for spec " + it.Spec.ID + " could not be created in the workspace")
				it.Bound, it.InWorkspace = false, false
				break
			}
		}
		if it.InWorkspace {
			s.seed(s.work, it)
		}
	}
	return nil
}

// openWork creates the workspace cache dir and pins it. A project that already
// has a file or directory of that name gets no workspace cache.
func (s *Session) openWork(ws *os.Root) (err error) {
	if err = ws.Mkdir(WorkspaceCacheName, 0o755); err != nil {
		return err
	}
	_ = ws.Chmod(WorkspaceCacheName, 0o755) // the umask must not hide it from the job UID
	s.work, err = ws.OpenRoot(WorkspaceCacheName)
	return err
}

// AfterJob packs the workspace cache dirs of every container miss that may be
// saved. The commit waits for Finalize. It never fails the attempt.
func (s *Session) AfterJob(ctx context.Context, _ *os.Root) error {
	if s.work == nil {
		return nil
	}
	defer func() {
		_ = s.work.Close()
		s.work = nil
	}()
	for _, it := range s.items {
		if !it.InWorkspace || !it.canSaveFromWorkspace() {
			continue
		}
		st, err := s.in.Engine.Stage(ctx, SaveRequest{Base: s.work, Attempt: s.in.Attempt, Namespace: s.own, Key: it.Key, Spec: it.Spec, Dirs: it.Dirs})
		switch {
		case err == nil:
			it.staged = st
		case errors.Is(err, ErrEmpty):
		default:
			s.use(ctx, it.Spec.ID, it.Key, resultOf(err), 0, 0)
			s.warn("cache save of spec " + it.Spec.ID + " refused: " + reasonOf(err))
		}
	}
	return nil
}
