package tart

// Purpose: delete clones whose owner is gone: the orphan sweep at start.
// Inputs:  the marker files under <CI home>/tart; each is locked for the life of its attempt (clone.go).
// Outputs: stopped and deleted clones, removed marker files; failures are logged and retried next sweep.
// Constraints: only clones with a marker (labelled by this profile) are touched, never a VM the operator named
//          nself-ci-* by hand; a marker whose lock cannot be taken belongs to a live attempt and is skipped; the
//          lock is the liveness proof, so a coordinator killed by SIGKILL is found with no pid or clock check.
//          This replaces a StaleChecker: the Isolator cannot reach the executor's checker, and a lock cannot
//          be fooled by pid reuse.

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// sweepWindow is how long one sweep stands for the attempts that start after it.
const sweepWindow = 30 * time.Second

// sweepBudget bounds one background sweep.
const sweepBudget = 10 * time.Minute

var cloneName = regexp.MustCompile(`^nself-ci-[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

// sweepOnce sweeps the state directories this process knows, at most once per window each.
func (p *Profile) sweepOnce(ctx context.Context, coordinator string) {
	p.mu.Lock()
	dirs := make([]string, 0, len(p.dirs)+1)
	for d := range p.dirs {
		dirs = append(dirs, d)
	}
	p.mu.Unlock()
	dirs = append(dirs, stateDir(p.home("")))
	seen := map[string]bool{}
	for _, d := range dirs {
		if seen[d] {
			continue
		}
		seen[d] = true
		p.mu.Lock()
		due := p.opts.Now().Sub(p.sweepAt[d]) >= sweepWindow
		if due {
			p.sweepAt[d] = p.opts.Now()
		}
		p.mu.Unlock()
		if due {
			p.Sweep(ctx, p.opts.NewTool(coordinator, filepath.Dir(d)), d)
		}
	}
}

// sweepAsync starts a sweep that outlives the caller's context (a probe is bounded by probeTimeout, a sweep is
// not) unless one is already running.
func (p *Profile) sweepAsync(ctx context.Context, coordinator string) {
	p.mu.Lock()
	if p.sweeping {
		p.mu.Unlock()
		return
	}
	p.sweeping = true
	p.mu.Unlock()
	p.bg.Add(1)
	go func() {
		defer p.bg.Done()
		sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), sweepBudget)
		defer cancel()
		p.sweepOnce(sctx, coordinator)
		p.mu.Lock()
		p.sweeping = false
		p.mu.Unlock()
	}()
}

// Sweep removes the orphan clones recorded in dir and returns their names.
func (p *Profile) Sweep(ctx context.Context, tool Tool, dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var removed []string
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") && strings.Contains(e.Name(), ".out-") { // collect scratch a killed coordinator left (VM-produced data)
			if info, err := e.Info(); err == nil && p.opts.Now().Sub(info.ModTime()) > 24*time.Hour {
				_ = os.Remove(filepath.Join(dir, e.Name()))
			}
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".json")
		if e.IsDir() || name == e.Name() || !cloneName.MatchString(name) {
			continue
		}
		path := filepath.Join(dir, e.Name())
		f, err := os.OpenFile(path, os.O_RDWR, 0)
		if err != nil {
			continue
		}
		ok, err := tryLock(f)
		if err != nil || !ok || !samePath(f, path) { // a live attempt holds it, or a claim is replacing it
			_ = f.Close()
			continue
		}
		if err := destroyClone(ctx, tool, p.opts.Tart, name, nil); err != nil {
			p.opts.Log("orphan clone %s could not be removed (%v); retrying at the next start", name, err)
			_ = f.Close()
			continue
		}
		c := &claim{f: f, path: path, dir: dir, name: name}
		c.release()
		removed = append(removed, name)
	}
	return removed
}
