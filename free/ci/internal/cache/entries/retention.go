package entries

// Purpose: retention (Epic D4/D5): the byte cap, the age cap, the free-space
// floor, usage-row expiry and the sweep of unreferenced cache-kind blobs.
// Inputs: a time budget. Outputs: Pruned, *Refused disk_floor.
// Constraints: entries leave the database in one transaction before any blob is
// unlinked, so a kill -9 at any point leaves every entry whole or absent; the
// sweep removes only cache-kind blobs outside the reference set and older than
// cas's one hour grace. Pruning evicts least recently used first.

import (
	"context"
	"errors"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
)

const pruneBatch = 64

var (
	errNoStatfs = errors.New("entries: free-space check unsupported on this platform")
	// errNoProbe: neither Options.Home nor Options.Free is set, so the floor cannot be checked.
	errNoProbe = errors.New("entries: no free-space probe (set Options.Home or NSELF_CI_HOME, or DisableFloor)")
)

// space reports free bytes and the floor; ok is false only when the floor is
// disabled or the platform cannot stat a volume.
func (s *Store) space() (free, floor int64, ok bool, err error) {
	probe := s.opt.Free
	if probe == nil && s.opt.Home != "" {
		probe = func() (int64, int64, error) { return statfs(s.opt.Home) }
	}
	if probe == nil && s.opt.DisableFloor {
		return 0, 0, false, nil
	}
	if probe == nil {
		return 0, 0, false, errNoProbe
	}
	free, total, err := probe()
	if errors.Is(err, errNoStatfs) {
		return 0, 0, false, nil
	}
	if err != nil {
		return 0, 0, false, err
	}
	floor = s.minFree
	if floor < 0 {
		floor = max(defaultMinFree, total/10)
	}
	return free, floor, true, nil
}

// sweepBlobs removes unreferenced cache blobs older than cas's one hour grace,
// measured from cutoff, a clock reading the caller took BEFORE its last prune
// write transaction (that transaction is the publication barrier: the single
// SQLite writer lock means every commit in flight in any process has finished
// before it, so the reference scan below sees it, and every later commit checks
// its blobs' age after cutoff, so they are younger than BlobMaxAge relative to it
// and hence inside cas's grace). No pause of the pruner or of a committer can
// make a committed blob look old. The sweep runs in its own goroutine so the caller's deadline bounds the wait
// even while cas holds its cross-process lock; the sweep itself stops at the
// next realm boundary and is safe to stop at any point.
func (s *Store) sweepBlobs(ctx context.Context, cutoff time.Time) error {
	list, err := s.be.CacheEntries(ctx, "")
	if err != nil {
		return err
	}
	keep := map[string]bool{}
	for _, m := range list {
		for _, b := range m.Blobs {
			keep[string(cas.CacheRealm(string(m.Namespace)))+"/"+b.SHA256] = true
		}
	}
	done := make(chan error, 1)
	go func() {
		done <- s.blobs.Sweep(ctx, "cache", func(r cas.Realm, digest string) bool { return keep[string(r)+"/"+digest] }, cutoff)
	}()
	select {
	case err = <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// refuse writes the audit row for a refusal decided outside the backend.
func (s *Store) refuse(ctx context.Context, op, target, reason string) error {
	if err := s.be.AppendAudit(ctx, "cache", "cache."+op+".refused", target, "reason="+reason); err != nil {
		return err
	}
	return &Refused{Reason: reason}
}

// Prune enforces the caps and the floor within budget (0 = no limit). The
// budget bounds every stage (a deadline on the context reaches the database
// steps, the reference scan and the cas sweep); when it ends first, More is set
// and the next call continues. Cancelling ctx itself is an error.
func (s *Store) Prune(ctx context.Context, budget time.Duration) (out Pruned, err error) {
	if budget > 0 {
		parent := ctx
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, budget)
		defer cancel()
		defer func() {
			if errors.Is(err, context.DeadlineExceeded) && parent.Err() == nil {
				out.More, err = true, nil
			}
		}()
	}
	return s.prune(ctx, budget)
}

func (s *Store) prune(ctx context.Context, budget time.Duration) (Pruned, error) {
	start, out := s.now(), Pruned{}
	cutoff := start // before the first prune transaction: see sweepBlobs
	step := PruneStep{MaxAge: s.maxAge, Retention: UsageRetention, MaxBytes: s.maxBytes, Limit: pruneBatch}
	for {
		r, err := s.be.CachePrune(ctx, step)
		if err != nil {
			return out, err
		}
		out.Entries, out.Bytes = out.Entries+r.Entries, out.Bytes+r.Bytes
		if !r.More {
			break
		}
		if err = ctx.Err(); err != nil {
			return out, err
		}
		if budget > 0 && s.now().Sub(start) >= budget {
			out.More = true
			return out, nil
		}
	}
	if err := s.sweepBlobs(ctx, cutoff); err != nil {
		return out, err
	}
	free, floor, ok, err := s.space()
	if err != nil || !ok || free >= floor {
		return out, err
	}
	step.Limit, step.FreeBytes, cutoff = 0, floor-free, s.now()
	r, err := s.be.CachePrune(ctx, step)
	out.Entries, out.Bytes = out.Entries+r.Entries, out.Bytes+r.Bytes
	if err != nil {
		return out, err
	}
	if err = s.sweepBlobs(ctx, cutoff); err != nil {
		return out, err
	}
	if free, _, _, err = s.space(); err != nil {
		return out, err
	}
	if free < floor {
		return out, s.refuse(ctx, "prune", "store", ReasonDiskFloor)
	}
	return out, nil
}
