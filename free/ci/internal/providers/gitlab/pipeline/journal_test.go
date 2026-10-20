package pipeline_test

// Purpose: the dispatch journal tests of P7-HOST-06: the durable store journal (restart sees the same
// nonce, higher numbers replace lower, lower are refused, a retry id supersedes, an earlier try cannot
// replace a later retry, a changed revision or spec is a conflict) and the lineage rule.
// Inputs:  a temp store, the memory journal.
// Outputs: pass or fail.
// Constraints: no network.

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/pipeline"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func rec(attempt string, n int, nonce string) pipeline.Record {
	return pipeline.Record{Attempt: attempt, Lineage: pipeline.Lineage(attempt), N: n, Revision: testRevision, SpecDigest: "sha256:" + strings.Repeat("a", 64),
		Nonce: nonce, Ref: "main", Project: "acme/widgets", Mode: pipeline.ModeInputs, Trusted: true}
}

// TestJournals runs the same sequence against the memory and the store journal.
func TestJournals(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	for name, j := range map[string]pipeline.Journal{"memory": pipeline.NewMemJournal(), "store": pipeline.NewStoreJournal(st)} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			first, prev, err := j.Begin(ctx, rec("att-1", 1, "n1"))
			if err != nil || prev != nil || first.Nonce != "n1" || !first.Trusted || first.Project != "acme/widgets" {
				t.Fatalf("first: %+v %v %v", first, prev, err)
			}
			again, prev, err := j.Begin(ctx, rec("att-1", 1, "other"))
			if err != nil || prev != nil || again.Nonce != "n1" {
				t.Fatalf("again: %+v %v %v", again, prev, err)
			}
			changed := rec("att-1", 1, "n9")
			changed.Revision = strings.Repeat("b", 40)
			if _, _, err = j.Begin(ctx, changed); err == nil {
				t.Fatal("the same number with another revision was accepted")
			}
			changed = rec("att-1", 1, "n9")
			changed.SpecDigest = "sha256:" + strings.Repeat("c", 64)
			if _, _, err = j.Begin(ctx, changed); err == nil {
				t.Fatal("the same number with another spec was accepted")
			}
			next, prev, err := j.Begin(ctx, rec("att-1", 2, "n2"))
			if err != nil || next.Nonce != "n2" || prev == nil || prev.Nonce != "n1" {
				t.Fatalf("next: %+v %+v %v", next, prev, err)
			}
			if _, _, err = j.Begin(ctx, rec("att-1", 1, "n3")); err == nil {
				t.Fatal("a lower attempt number was accepted")
			}
			retry, prev, err := j.Begin(ctx, rec("att-1-retry-2", 1, "n4"))
			if err != nil || retry.Nonce != "n4" || prev == nil || prev.Nonce != "n2" {
				t.Fatalf("retry: %+v %+v %v", retry, prev, err)
			}
			if _, _, err = j.Begin(ctx, rec("att-1", 7, "n5")); err == nil {
				t.Fatal("a late dispatch of the first try replaced the later retry")
			}
			if latest, ok, err := j.Latest(ctx, "att-1"); err != nil || !ok || latest.Nonce != "n4" {
				t.Fatalf("latest: %+v %v %v", latest, ok, err)
			}
			got, ok, err := j.ByNonce(ctx, "n1")
			if err != nil || !ok || got.N != 1 || got.Revision != testRevision || got.Project != "acme/widgets" {
				t.Fatalf("by nonce: %+v %v %v", got, ok, err)
			}
			if _, ok, _ = j.ByNonce(ctx, "nope"); ok {
				t.Fatal("an unknown nonce was found")
			}
		})
	}
}

// TestLineage proves the retry suffixes of the lease store and of CI-40 share one journal line.
func TestLineage(t *testing.T) {
	for in, want := range map[string]string{"job7.a3": "job7", "att-1-retry-2": "att-1", "plain": "plain", "x.a1-retry-2": "x", ".a1": ".a1", "a.b": "a.b"} {
		if got := pipeline.Lineage(in); got != want {
			t.Errorf("Lineage(%q) = %q, want %q", in, got, want)
		}
	}
}
