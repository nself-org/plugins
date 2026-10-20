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
		ns, err := entries.NamespaceOf(untrustedPR1)
		if err != nil {
			t.Fatal(err)
		}
		key, err := entries.Key(entries.KeyInputs{Namespace: ns, SpecID: "go", SpecVersion: "1", Platform: "linux/amd64", Isolation: "container"})
		if err != nil {
			t.Fatal(err)
		}
		fin := w.Attempt("z1", untrustedPR1, []string{key}, true)
		m := w.Manifest(untrustedPR1, key, "zombie bytes")
		err = w.Es.Commit(w.ctx, "z1", fin-1, ns, m) // the zombie holds the epoch before the current lease
		got := refusal(t, err, func(err error) string {
			var r *entries.Refused
			if !errors.As(err, &r) {
				t.Fatalf("want E711, got %v", err)
			}
			return r.Reason
		})
		got.Audit = w.AuditOf("cache.commit.refused")
		got.Landed = w.EntryBytes()
		return got
	})
}
