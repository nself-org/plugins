package jit

// Purpose: Deregister and Orphans, the teardown half (Epic D6).
// Inputs:  a RunnerRef from Minted.Ref or Orphans (its ID is "<scope>:<runner id>"; a bare id is
//          accepted); an orphan cutoff.
// Outputs: nil when the runner is gone and proven gone; the orphan list of the configured scope.
// Constraints: Deregister reads the runner first. Absent (404) is done; a runner whose name is not
//          nself-jit-<ulid> is never deleted (E707 deregister.not_ours), so a wrong handle cannot
//          remove a persistent runner. After the DELETE (a 404 there is also done) the runner is read
//          once more by id, and one still answering is E707 deregister_unverified. Reading by id and
//          not walking the list is deliberate: ephemeral runners leave a busy list while it is paged,
//          and a shifted page would hide a runner that is still registered. A ref minted at the other
//          scope than the configured one is refused (E707 deregister.scope_changed) instead of
//          being "deleted" from the wrong collection. Orphans are runners this package named, not
//          busy, born before the cutoff; a name that is not ours or whose ULID does not parse is never
//          returned, because deleting a persistent runner is worse than leaving an orphan.

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers"
)

var runnerID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,40}$`)

// refID is the handle a ref carries: the scope the runner was registered at, then its id.
func (r *Registrar) refID(id string) string { return r.set.Scope + ":" + id }

// splitRefID reads "<scope>:<id>"; a bare id has no scope.
func splitRefID(s string) (kind, id string) {
	if i := strings.IndexByte(s, ':'); i >= 0 {
		return s[:i], s[i+1:]
	}
	return "", s
}

// runnerRead is GET of one runner: found is false on 404.
func (r *Registrar) runnerRead(ctx context.Context, t target, id string) (name string, found bool, err error) {
	tmpl, params := r.runnersPath(t)
	params["id"] = id
	var out struct {
		Name string `json:"name"`
	}
	resp, err := r.c.Do(ctx, "GET", tmpl+"/{id}", params, nil, &out)
	if resp.Status == 404 {
		return "", false, nil
	}
	return out.Name, err == nil, err
}

// Deregister deletes the runner and verifies it is gone.
func (r *Registrar) Deregister(ctx context.Context, ref providers.RunnerRef) error {
	t, err := r.resolve(ref.Scope)
	if err != nil {
		return err
	}
	kind, id := splitRefID(ref.ID)
	if !runnerID.MatchString(id) || kind != "" && kind != ScopeRepo && kind != ScopeOrg {
		return providers.NewError(providers.Config, "E702", "runner.id", 0, time.Time{})
	}
	if kind != "" && kind != r.set.Scope {
		return providers.NewError(providers.Config, "E707", "deregister.scope_changed", 0, time.Time{})
	}
	name, found, err := r.runnerRead(ctx, t, id)
	if err != nil {
		return runnerErr(err, "deregister")
	}
	if !found {
		return nil
	}
	if _, ok := runnerBirth(name); !ok {
		return providers.NewError(providers.Config, "E707", "deregister.not_ours", 0, time.Time{})
	}
	tmpl, params := r.runnersPath(t)
	params["id"] = id
	resp, err := r.c.Do(ctx, "DELETE", tmpl+"/{id}", params, nil, nil)
	if err != nil && resp.Status != 404 {
		return runnerErr(err, "deregister")
	}
	_, still, err := r.runnerRead(ctx, t, id)
	var pe *providers.Error
	if err != nil && errors.As(err, &pe) && pe.Class != providers.Config {
		return err
	}
	if err != nil || still {
		return providers.NewError(providers.Config, "E707", "deregister_unverified", 0, time.Time{})
	}
	return nil
}

// Orphans lists the runners of scope that are older than before and idle. A zero before means
// OrphanAge ago.
func (r *Registrar) Orphans(ctx context.Context, scope providers.RunnerScope, before time.Time) ([]providers.RunnerRef, error) {
	t, err := r.resolve(scope)
	if err != nil {
		return nil, err
	}
	if before.IsZero() {
		before = r.now().Add(-OrphanAge)
	}
	all, err := r.listRunners(ctx, t)
	if err != nil {
		return nil, err
	}
	var out []providers.RunnerRef
	for _, x := range all {
		born, ok := runnerBirth(x.Name)
		if ok && !x.Busy && born.Before(before) {
			out = append(out, providers.RunnerRef{ID: r.refID(strconv.FormatInt(x.ID, 10)), Scope: scope})
		}
	}
	return out, nil
}
