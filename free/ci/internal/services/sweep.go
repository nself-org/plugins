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
	locks, _ := filepath.Glob(filepath.Join(work, projectPrefix+"*.lock"))
	for _, lf := range locks {
		l, held, err := acquireLock(lf)
		if err != nil || held {
			continue // alive, or not ours to judge
		}
		project := strings.TrimSuffix(filepath.Base(lf), ".lock")
		root := filepath.Join(work, project)
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
		tear(&Handle{Project: project, root: root, Dir: filepath.Join(root, "svc"), lock: l})
	}
	return removed, errors.Join(errs...)
}

// orphanProjects lists nselfci- compose projects with containers whose working
// directory is under work and that no lock file protects.
func (p *Provisioner) orphanProjects(ctx context.Context, work string) ([]string, error) {
	res, err := p.Run.Run(ctx, Call{Argv: []string{p.docker(), "ps", "-a", "--filter", "label=com.docker.compose.project",
		"--format", `{{.Label "com.docker.compose.project"}} {{.Label "com.docker.compose.project.working_dir"}}`}})
	if err != nil || res.ExitCode != 0 {
		return nil, nil // no Docker: nothing to sweep
	}
	seen := map[string]bool{}
	var out []string
	for _, line := range strings.Split(res.Output, "\n") {
		f := strings.SplitN(strings.TrimSpace(line), " ", 2)
		if len(f) < 2 || !strings.HasPrefix(f[0], projectPrefix) || seen[f[0]] {
			continue
		}
		if !strings.HasPrefix(filepath.Clean(f[1]), filepath.Clean(work)+string(os.PathSeparator)) {
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
