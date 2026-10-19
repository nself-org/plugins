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

var errNoStatfs = errors.New("entries: free-space check unsupported on this platform")

// space reports free bytes and the floor; ok is false when no check is
// configured (no Free hook and no Home).
func (s *Store) space() (free, floor int64, ok bool, err error) {
	probe := s.opt.Free
	if probe == nil && s.opt.Home != "" {
		probe = func() (int64, int64, error) { return statfs(s.opt.Home) }
	}
	if probe == nil {
		return 0, 0, false, nil
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

// sweepBlobs removes unreferenced cache blobs older than cas's grace. A commit
// in any process that races this scan only references blobs younger than
// BlobMaxAge, which the sweep never removes.
func (s *Store) sweepBlobs(ctx context.Context) error {
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
	return s.blobs.Sweep(ctx, "cache", func(r cas.Realm, digest string) bool { return keep[string(r)+"/"+digest] }, s.now())
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
	if err := s.sweepBlobs(ctx); err != nil {
		return out, err
	}
	free, floor, ok, err := s.space()
	if err != nil || !ok || free >= floor {
		return out, err
	}
	step.Limit, step.FreeBytes = 0, floor-free
	r, err := s.be.CachePrune(ctx, step)
	out.Entries, out.Bytes = out.Entries+r.Entries, out.Bytes+r.Bytes
	if err != nil {
		return out, err
	}
	if err = s.sweepBlobs(ctx); err != nil {
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
