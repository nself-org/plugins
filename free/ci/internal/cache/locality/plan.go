package locality

// Purpose: the cache keys a job would use on a runner, per spec, with the namespaces it may read.
// Inputs:  a Job, the runner's platform, a per-build memoized probe.
// Outputs: planned specs in spec id order; each lists candidates in read order (own namespace
// first, then the protected snapshot namespaces authz allows).
// Constraints: pure over its inputs; mirrors the key derivation of specs.Session and
// remote.BuildPlan (specs.Resolve + Material.Key) so a planned key equals the key the job
// restores with; a spec that cannot be keyed is left out.

import (
	"context"
	"encoding/base64"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/cache/specs"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

type candidate struct{ realm, key string }

type plannedSpec struct {
	spec       string
	candidates []candidate
}

// namespaces lists, in read order, the namespaces the job's attempt may read: its own (when
// authz grants rw-own) then each protected snapshot class of the same project with read-only access.
func namespaces(f entries.Facts) []authz.Namespace {
	own, err := entries.NamespaceOf(f)
	if err != nil {
		return nil
	}
	job := authz.Job{Namespace: own, State: authz.StateRunning}
	var out []authz.Namespace
	if authz.CacheAccess(job, own) == authz.ModeRWOwn {
		out = append(out, own)
	}
	for _, class := range []model.TrustClass{"internal", "owner"} {
		ns := authz.Namespace{Project: own.Project, Trust: class, Protection: authz.Protected}
		if authz.CacheAccess(job, ns) == authz.ModeROSnapshot {
			out = append(out, ns)
		}
	}
	return out
}

// planJob derives the planned keys of j on platform. Specs whose probe or toolchain is
// unavailable are skipped.
func planJob(ctx context.Context, j Job, platform string, probe func(context.Context, []string) (string, error)) []plannedSpec {
	nss := namespaces(j.Facts)
	if len(nss) == 0 || platform == "" {
		return nil
	}
	isolation := j.Isolation
	if isolation == "" {
		isolation = "process"
	}
	var out []plannedSpec
	for _, sp := range specs.For(j.Detected, j.Container) {
		mat, err := specs.Resolve(ctx, specs.Input{Spec: sp, Platform: platform, Isolation: isolation, Image: j.Image, Facts: j.Detected,
			Scope: j.Scope, ExtraKeys: j.ExtraKeys, Environ: j.Environ, Probe: probe})
		if err != nil {
			continue
		}
		p := plannedSpec{spec: sp.ID}
		for _, ns := range nss {
			key, err := mat.Key(ns)
			if err != nil {
				continue
			}
			p.candidates = append(p.candidates, candidate{realm: base64.RawURLEncoding.EncodeToString(ns.Canonical()), key: key})
		}
		if len(p.candidates) > 0 {
			out = append(out, p)
		}
	}
	return out
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
