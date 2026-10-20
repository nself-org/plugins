package locality

// Purpose: the cache keys a job would use on a runner, per spec, with the namespaces it may read.
// Inputs:  a Job, the runner's platform, whether the runner is a remote node, a per-build
// memoized probe.
// Outputs: a jobPlan: the specs the job's facts switch on (total) and, for those that can be
// keyed, their candidates in read order.
// Constraints: pure over its inputs. The namespaces follow the code that restores:
//   - a runner that holds the store (coordinator host, local machine) runs specs.Session: the
//     job's own namespace, then, for container jobs only, the protected snapshot namespaces
//     authz allows (a process job cannot hold a read-only mount, so a snapshot is denied);
//   - a remote node gets remote.BuildPlan: a trusted job reads its own namespace only; an
//     untrusted container job reads the project's protected owner namespace only; an untrusted
//     process job gets no cache.
//
// A spec whose toolchain cannot be keyed (a failed probe, no image on a node) still counts in
// total, so the fact is never "known" over a spec the plan could not look at.

import (
	"context"
	"encoding/base64"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/cache/specs"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// candidate is one (realm, key) a spec may restore from. snapshot marks a namespace that is not
// the job's own: its entry counts only when the writer facts make it snapshot eligible.
type candidate struct {
	realm, key string
	snapshot   bool
}

type plannedSpec struct {
	spec       string
	candidates []candidate
}

// jobPlan is the plan of one job on one platform. total is the number of specs the job's facts
// select; specs holds those that could be keyed and have at least one readable namespace.
type jobPlan struct {
	specs []plannedSpec
	total int
}

type readable struct {
	ns       authz.Namespace
	snapshot bool
}

// namespaces lists, in read order, the namespaces the job's attempt may read (see the header).
func namespaces(f entries.Facts, container, remote bool) []readable {
	own, err := entries.NamespaceOf(f)
	if err != nil {
		return nil
	}
	job := authz.Job{Namespace: own, State: authz.StateRunning}
	if remote {
		if own.Trust != model.TrustClass("untrusted") {
			if authz.CacheAccess(job, own) == authz.ModeRWOwn {
				return []readable{{own, false}}
			}
			return nil
		}
		ns := authz.Namespace{Project: own.Project, Trust: "owner", Protection: authz.Protected}
		if container && authz.CacheAccess(job, ns) == authz.ModeROSnapshot {
			return []readable{{ns, true}}
		}
		return nil
	}
	var out []readable
	if authz.CacheAccess(job, own) == authz.ModeRWOwn {
		out = append(out, readable{own, false})
	}
	if !container {
		return out
	}
	for _, class := range []model.TrustClass{"internal", "owner"} {
		ns := authz.Namespace{Project: own.Project, Trust: class, Protection: authz.Protected}
		if authz.CacheAccess(job, ns) == authz.ModeROSnapshot {
			out = append(out, readable{ns, true})
		}
	}
	return out
}

// planJob derives the planned keys of j on platform. remote is true for a runner that does not
// hold the store.
func planJob(ctx context.Context, j Job, platform string, remote bool, probe func(context.Context, []string) (string, error)) jobPlan {
	list := specs.For(j.Detected, j.Container)
	plan := jobPlan{total: len(list)}
	nss := namespaces(j.Facts, j.Container, remote)
	if len(nss) == 0 || platform == "" {
		return plan
	}
	isolation := j.Isolation
	if isolation == "" {
		isolation = "process"
	}
	for _, sp := range list {
		mat, err := specs.Resolve(ctx, specs.Input{Spec: sp, Platform: platform, Isolation: isolation, Image: j.Image, Facts: j.Detected,
			Scope: j.Scope, ExtraKeys: j.ExtraKeys, Environ: j.Environ, Probe: probe})
		if err != nil {
			continue
		}
		p := plannedSpec{spec: sp.ID}
		for _, r := range nss {
			key, err := mat.Key(r.ns)
			if err != nil {
				continue
			}
			p.candidates = append(p.candidates, candidate{realm: base64.RawURLEncoding.EncodeToString(r.ns.Canonical()), key: key, snapshot: r.snapshot})
		}
		if len(p.candidates) > 0 {
			plan.specs = append(plan.specs, p)
		}
	}
	return plan
}

// memoProbe wraps the jobs' probes so one world build runs each distinct argv at most once: every
// probe answers for this machine, so the answer is the same for every job of a build.
type memoProbe struct {
	mu   sync.Mutex
	seen map[string]probeResult
}

type probeResult struct {
	line string
	err  error
}

func (m *memoProbe) wrap(probe func(context.Context, []string) (string, error)) func(context.Context, []string) (string, error) {
	if probe == nil {
		return nil
	}
	return func(ctx context.Context, argv []string) (string, error) {
		k := ""
		for _, a := range argv {
			k += a + "\x00"
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		if r, ok := m.seen[k]; ok {
			return r.line, r.err
		}
		line, err := probe(ctx, argv)
		if m.seen == nil {
			m.seen = map[string]probeResult{}
		}
		m.seen[k] = probeResult{line, err}
		return line, err
	}
}
