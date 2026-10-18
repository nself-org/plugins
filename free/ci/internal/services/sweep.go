package services

// Purpose: remove the service projects of coordinators that died (kill -9, a
// crash, a lost machine) without tearing down. A live coordinator's projects
// are never touched.
// Inputs:  the work directory under Home and the Docker daemon.
// Outputs: how many stale projects were removed.
// Constraints: ownership is the flock on work/<project>.lock, which the kernel
// drops when its holder dies; a project with containers but no lock file is
// swept only when its compose working directory is under this Home, so another
// coordinator home on the same daemon is left alone.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const projectPrefix = "nselfci-"

// Sweep tears down stale service projects. It returns the number removed and
// the joined error of any teardown that did not finish.
func (p *Provisioner) Sweep(ctx context.Context) (int, error) {
	work := filepath.Join(p.Home, "work")
	var errs []error
	removed := 0
	tear := func(h *Handle) {
		if err := p.Down(ctx, h); err != nil {
			errs = append(errs, err)
			return
		}
		removed++
	}
	p.removeStaleCalls()
	_ = p.endCalls(ctx, p.ownerDead(work)) // markers whose provision is gone; each teardown below checks its own
	locks, _ := filepath.Glob(filepath.Join(work, projectPrefix+"*.lock"))
	for _, lf := range locks {
		l, held, err := acquireLock(lf)
		if err != nil || held {
			continue // alive, or not ours to judge
		}
		project := strings.TrimSuffix(filepath.Base(lf), ".lock")
		root := filepath.Join(work, project)
		if !p.callsEnded(ctx, project, l, &errs) {
			continue
		}
		tear(&Handle{Project: project, root: root, Dir: filepath.Join(root, "svc"), lock: l})
	}
	orphans, err := p.orphanProjects(ctx, work)
	if err != nil {
		errs = append(errs, err)
	}
	for _, project := range orphans {
		lf := filepath.Join(work, project+".lock")
		l, held, err := acquireLock(lf) // creates the file: the teardown below removes it
		if err != nil || held {
			continue
		}
		root := filepath.Join(work, project)
		if !p.callsEnded(ctx, project, l, &errs) {
			continue
		}
		tear(&Handle{Project: project, root: root, Dir: filepath.Join(root, "svc"), lock: l})
	}
	return removed, errors.Join(errs...)
}

// orphanProjects lists nselfci- compose projects with containers whose working
// directory is under work and that no lock file protects.
func (p *Provisioner) orphanProjects(ctx context.Context, work string) ([]string, error) {
	res, err := p.call(ctx, queryTimeout, Call{Argv: []string{p.docker(), "ps", "-a", "--filter", "label=com.docker.compose.project",
		"--format", `{{.Label "com.docker.compose.project"}} {{.Label "com.docker.compose.project.working_dir"}}`}})
	if err != nil || res.ExitCode != 0 {
		return nil, nil // no Docker: nothing to sweep
	}
	roots := []string{work}
	if real, err := filepath.EvalSymlinks(work); err == nil && real != work {
		roots = append(roots, real) // the daemon reports the resolved path of a symlinked home
	}
	seen := map[string]bool{}
	var out []string
	for _, line := range strings.Split(res.Output, "\n") {
		f := strings.SplitN(strings.TrimSpace(line), " ", 2)
		if len(f) < 2 || !strings.HasPrefix(f[0], projectPrefix) || seen[f[0]] {
			continue
		}
		if !underAny(f[1], roots) {
			continue
		}
		seen[f[0]] = true
		if _, err := os.Stat(filepath.Join(work, f[0]+".lock")); err == nil {
			continue // a lock file exists: the first pass already judged it
		}
		out = append(out, f[0])
	}
	return out, nil
}

func underAny(dir string, roots []string) bool {
	for _, r := range roots {
		if strings.HasPrefix(filepath.Clean(dir), filepath.Clean(r)+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}

// callKiller is a Runner that can end the CLI calls of dead owners (the Funnel).
type callKiller interface {
	KillStale(ctx context.Context, dead func(owner string) bool) error
}

func (p *Provisioner) endCalls(ctx context.Context, dead func(string) bool) error {
	if k, ok := p.Run.(callKiller); ok {
		return k.KillStale(ctx, dead)
	}
	return nil
}

// callsEnded ends the CLI calls of a project whose lock this Sweep holds, just
// before its teardown: a call left running by a dead coordinator would create
// resources again after the project was removed. When a call cannot be ended
// the project is left for the next Sweep (lock file kept) and false returned.
func (p *Provisioner) callsEnded(ctx context.Context, project string, l *ownerLock, errs *[]error) bool {
	err := p.endCalls(ctx, func(o string) bool { return o == project })
	if err == nil {
		return true
	}
	l.Unlock()
	*errs = append(*errs, fail(CodeProvision, "cannot end the CLI calls left by the owner of %s: %v", project, err))
	return false
}

// ownerDead tells whether the coordinator that owned a provision is gone: its
// lock file is absent, or free (the kernel drops a dead owner's flock).
func (p *Provisioner) ownerDead(work string) func(string) bool {
	return func(owner string) bool {
		if !strings.HasPrefix(owner, projectPrefix) {
			return false // a call with no provision: short, left to its own end
		}
		lf := filepath.Join(work, owner+".lock")
		if _, err := os.Stat(lf); err != nil {
			return true
		}
		l, held, err := acquireLock(lf)
		if err != nil || held {
			return false
		}
		l.Unlock()
		return true
	}
}

// removeStaleCalls deletes the workspaces and process markers of CLI calls whose coordinator died
// mid-call (the funnel removes them itself on every normal exit). An hour old
// is far past any live call.
func (p *Provisioner) removeStaleCalls() {
	base := filepath.Join(p.Home, funnelDir)
	dirs, _ := filepath.Glob(filepath.Join(base, "work", "svccall-*"))
	markers, _ := filepath.Glob(filepath.Join(base, "processes", "svccall-*.json"))
	dirs = append(dirs, markers...)
	for _, d := range dirs {
		if fi, err := os.Stat(d); err == nil && time.Since(fi.ModTime()) > time.Hour {
			_ = os.RemoveAll(d)
		}
	}
}
