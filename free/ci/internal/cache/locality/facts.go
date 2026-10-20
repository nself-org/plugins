package locality

// Purpose: the per-runner, per-job miss bytes (Epic CACHE D8).
// Inputs:  the world (runners), the bound Inputs, the clock (World.NowMs).
// Outputs: the Cache map for sched.World.
// Constraints: see locality.go. A runner holds an entry when it is the coordinator host or the
// local machine (the entries live in its store), or when a recorded read-through copy of the
// same (realm, key) is at most RowMaxAge old. Only those runners can probe a process-level
// toolchain; a node is planned for container jobs with a pinned image digest only, because no
// probe result of a remote node exists at world build time (CACHE-19 ruling 4). The work is one
// ledger read per distinct key and one table read; per (runner, job) it is a map lookup.

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/sched"
)

const (
	known     = "known"
	estimated = "estimated"
	unknown   = "unknown"
)

type copyKey struct{ node, realm, key string }

// entrySize is what the ledger says about one (realm, key): ok when an entry exists, bytes its
// size, snapshotOK when its writer facts permit a read-only snapshot restore (no secrets
// received, no private dependencies, scanned clean; entries.Store.snapshotOK adds the
// project's own policy, which the contributor cannot see).
type entrySize struct {
	bytes      int64
	ok         bool
	snapshotOK bool
}

// holdsStore reports a runner whose own store is the entries store: the coordinator host, or the
// injected local runner (transport and provider "local").
func holdsStore(r sched.Runner) bool {
	id := r.Capability.Identity
	return r.CoordinatorHost || (id.Transport == "local" && id.Provider == "local")
}

// platformOf is "os/arch" as the cache key carries it; "" when the runner never reported either.
func platformOf(r sched.Runner) string {
	c := r.Capability.Platform
	if c.OS.Value == nil || c.Arch.Value == nil || *c.OS.Value == "" || *c.Arch.Value == "" {
		return ""
	}
	return *c.OS.Value + "/" + *c.Arch.Value
}

// Contribute fills w.Cache from the bound Source. It is registered as the `cache` contributor.
// It never fails a build: no source, a source error or an empty job list leaves Cache empty,
// which the scheduler reads as unknown for every pair.
func Contribute(ctx context.Context, w *sched.World) error {
	src := current()
	if src == nil {
		return nil
	}
	in, err := src(ctx)
	if err != nil || in.Ledger == nil || len(in.Jobs) == 0 {
		return nil
	}
	w.Cache = compute(ctx, w, in)
	return nil
}

// compute reads each distinct key and the copies table at most once per call.
func compute(ctx context.Context, w *sched.World, in Inputs) map[string]map[string]sched.CacheFact {
	warn := func(err error) {
		if in.Warn != nil {
			in.Warn(err)
		}
	}
	sizes := map[candidate]entrySize{}
	size := func(c candidate) entrySize {
		if s, done := sizes[c]; done {
			return s
		}
		var s entrySize
		list, err := in.Ledger.CacheEntries(ctx, c.key)
		if err != nil {
			warn(fmt.Errorf("cache locality: entries %s: %w", c.key, err))
			list = nil // a failed read is unknown, whatever it returned
		}
		for _, m := range list {
			if base64.RawURLEncoding.EncodeToString(m.Namespace) == c.realm {
				w := m.Writer
				s = entrySize{m.Bytes(), true, !w.SecretsReceived && !w.PrivateDeps && w.ScannedClean}
				break
			}
		}
		sizes[c] = s
		return s
	}
	var copies map[copyKey]time.Time
	seenAt := func(k copyKey) (time.Time, bool) {
		if copies == nil {
			copies = map[copyKey]time.Time{}
			rows, err := in.Ledger.CacheLocality(ctx, "", "", "")
			if err != nil {
				warn(fmt.Errorf("cache locality: copies: %w", err))
				rows = nil
			}
			for _, r := range rows {
				copies[copyKey{r.Node, r.Realm, r.Key}] = r.LastSeen
			}
		}
		t, ok := copies[k]
		return t, ok
	}
	now := time.UnixMilli(w.NowMs)
	var memo memoProbe
	type planKey struct {
		job      int
		platform string
		holder   bool
	}
	plans := map[planKey]jobPlan{}
	out := map[string]map[string]sched.CacheFact{}
	for _, r := range w.Runners {
		holder, platform := holdsStore(r), platformOf(r)
		holds := func(c candidate) bool {
			if holder {
				return true
			}
			t, ok := seenAt(copyKey{r.ID, c.realm, c.key})
			return ok && now.Sub(t) <= RowMaxAge
		}
		facts := map[string]sched.CacheFact{}
		for i, j := range in.Jobs {
			pk := planKey{i, platform, holder}
			p, done := plans[pk]
			if !done {
				var probe func(context.Context, []string) (string, error)
				if holder {
					probe = memo.wrap(j.Probe)
				}
				p = planJob(ctx, j, platform, !holder, probe)
				plans[pk] = p
			}
			facts[j.Key] = fold(p, size, holds)
		}
		out[r.ID] = facts
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// fold combines the specs of one job: a spec counts when an entry exists in the first readable
// namespace that has a usable one (a snapshot entry must be snapshot eligible); it adds 0 when
// the runner holds it, else the entry size. Specs the plan could not key or read count as
// unknown, so "known" means every spec the job's facts select was looked at. Entries are not
// re-hashed and the writer attempt's state is not read (the store retracts entries whose attempt
// did not pass), so the fact is advisory.
func fold(p jobPlan, size func(candidate) entrySize, holds func(candidate) bool) sched.CacheFact {
	var miss int64
	found := 0
	for _, ps := range p.specs {
		for _, c := range ps.candidates {
			s := size(c)
			if !s.ok || c.snapshot && !s.snapshotOK {
				continue
			}
			found++
			if !holds(c) {
				miss += s.bytes
			}
			break
		}
	}
	switch {
	case found == 0:
		return sched.CacheFact{Confidence: unknown}
	case found == p.total:
		return sched.CacheFact{MissBytes: &miss, Confidence: known}
	}
	return sched.CacheFact{MissBytes: &miss, Confidence: estimated}
}
