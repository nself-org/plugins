//go:build cachesec

package cachesec

// Purpose: TM-13 cases on the local carrier (writes by a stale or zombie attempt).
// Inputs: a real store and cas (newWorld). Outputs: registered cases of matrix.yaml.
// Constraints: production packages only; no check is weakened.

import (
	"errors"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
)

var untrustedPR1 = entries.Facts{Project: "proj", Trust: "untrusted", Protection: "unprotected", HeadRepo: "fork/example", PR: "1"}

func init() {
	register("C13-LOC-01", func(t *testing.T) Observed {
		w := newWorld(t)
		keyHonest, keyZombie := w.Key(untrustedPR1, "honest"), w.Key(untrustedPR1, "zombie")
		ns := w.NS(untrustedPR1)
		fin := w.Attempt("z1", untrustedPR1, []string{keyHonest, keyZombie}, true)
		// control: the same attempt saves a planned key with the current epoch
		if err := w.Es.Commit(w.ctx, "z1", fin, ns, w.Manifest(untrustedPR1, keyHonest, "honest bytes")); err != nil {
			t.Fatalf("control: the honest commit was refused: %v", err)
		}
		before := w.EntryBytes()
		m := w.Manifest(untrustedPR1, keyZombie, "zombie bytes")
		err := w.Es.Commit(w.ctx, "z1", fin-1, ns, m) // the zombie holds the epoch before the current lease
		got := refusal(t, err, func(err error) string {
			var r *entries.Refused
			if !errors.As(err, &r) {
				t.Fatalf("want E711, got %v", err)
			}
			return r.Reason
		})
		got.Audit = w.AuditOf("cache.commit.refused")
		got.Landed = w.EntryBytes() - before
		return got
	})
}
