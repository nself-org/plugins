package services

// Purpose: tear a provision down on every exit path (pass, fail, cancel,
// timeout) and prove nothing is left: `nself stop --volumes --remove-orphans`,
// removal of the project directory, then a listing of compose containers,
// volumes and networks carrying the project label.
// Inputs:  a Handle (from Up, or rebuilt from disk by Sweep).
// Outputs: nil when the project is gone, E636 naming what remains otherwise.
// Constraints: safe to call twice; the caller passes a context that survives
// cancellation; the owner lock and lock file are released only after the
// check, so a failed teardown stays visible to the next Sweep. The compose
// fallback touches only the one nselfci- project this Handle names.

import (
	"context"
	"os"
	"strings"
)

// Down removes the project. It never touches anything outside it.
func (p *Provisioner) Down(ctx context.Context, h *Handle) error {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.done {
		return nil
	}
	if !h.lock.active() {
		l, held, err := acquireLock(h.root + ".lock")
		if err != nil {
			return fail(CodeProvision, "teardown of %s skipped: cannot lock it: %v", h.Project, err)
		}
		if held {
			return fail(CodeProvision, "teardown of %s skipped: another coordinator holds it", h.Project)
		}
		h.lock = l
	}
	var problems []string
	// Only a directory whose own .env names this project may be handed to the
	// CLI; otherwise the compose fallback below (by project label) does the work.
	if h.owns() == nil {
		if err := p.cli(ctx, h, "stop", "--volumes", "--remove-orphans"); err != nil {
			problems = append(problems, err.Error())
		}
	}
	left, lerr := p.leftovers(ctx, h.Project)
	if lerr == nil && len(left) > 0 {
		// The CLI could not finish (project files gone, or a failed stop): the
		// compose project is named by label, so removing it needs no file.
		_, _ = p.call(ctx, cliTimeout, Call{Argv: []string{p.docker(), "compose", "-p", h.Project, "down", "--volumes", "--remove-orphans"}})
		left, lerr = p.leftovers(ctx, h.Project)
	}
	if err := os.RemoveAll(h.root); err != nil {
		problems = append(problems, "cannot remove "+h.root+": "+err.Error())
	}
	releasePorts(h.ports)
	switch {
	case lerr != nil:
		problems = append(problems, "cannot verify teardown: "+lerr.Error())
	case len(left) > 0:
		problems = append(problems, "left behind: "+strings.Join(left, ", "))
	}
	if len(problems) > 0 {
		h.lock.Unlock() // keep the lock file: the next Sweep retries
		return fail(CodeProvision, "teardown of %s incomplete: %s", h.Project, strings.Join(problems, "; "))
	}
	_ = os.Remove(h.root + ".lock")
	h.lock.Unlock()
	h.done = true
	return nil
}

// leftovers lists containers, volumes and networks carrying the project label.
func (p *Provisioner) leftovers(ctx context.Context, project string) ([]string, error) {
	filter := "label=com.docker.compose.project=" + project
	var left []string
	for _, kind := range [][]string{{"ps", "-a", "-q"}, {"volume", "ls", "-q"}, {"network", "ls", "-q"}} {
		argv := append([]string{p.docker()}, kind...)
		argv = append(argv, "--filter", filter)
		res, err := p.call(ctx, queryTimeout, Call{Argv: argv})
		if err != nil {
			return nil, err
		}
		if res.ExitCode != 0 {
			return nil, fail(CodeProvision, "`docker %s` failed (exit %d)", kind[0], res.ExitCode)
		}
		for _, line := range strings.Split(res.Output, "\n") {
			if id := strings.TrimSpace(line); id != "" {
				left = append(left, kind[0]+":"+id)
			}
		}
	}
	return left, nil
}
