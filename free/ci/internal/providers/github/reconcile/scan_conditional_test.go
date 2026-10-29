package reconcile

// Purpose: P7-AUTO-11 conditional requests on the head scan: once a full scan has
// stored its ETags, an unchanged repository answers 304 on the repository, the pull
// list and (with extra branches) the branch list, and ScanHeads makes no further
// request; a changed pull list is read in full again.
// Inputs: the HOST-17 fixture forge and engine rig. Outputs: pass or fail.
// Constraints: loopback TLS only; synthetic names and shas.

import (
	"context"
	"testing"
)

func TestScanHeadsConditional(t *testing.T) {
	ctx := context.Background()
	r := newRig(t, "o/r")
	r.f.do(func() { r.f.repos["o/r"].Pulls = []fPull{{Number: 7, Head: sha('b', 7), Assoc: "OWNER"}} })
	opts := []ScanOptions{{}, {Branches: []string{"release/*"}, MaxBranches: 5}}
	for _, o := range opts {
		first, err := r.e.ScanHeads(ctx, "o/r", o)
		if err != nil || first.NotModified || len(first.Heads) == 0 {
			t.Fatalf("the first scan must list the heads: %+v %v", first, err)
		}
		r.f.reset()
		again, err := r.e.ScanHeads(ctx, "o/r", o)
		if err != nil || !again.NotModified || len(again.Heads) != 0 {
			t.Fatalf("an unchanged repository must answer NotModified: %+v %v", again, err)
		}
		want := 2 // the repository and the pull list
		if len(o.Branches) > 0 {
			want = 3 // and the branch list
		}
		if n := r.f.total(); n != want || r.f.nonCond304() != 0 || again.Calls != want {
			t.Fatalf("a 304 pass must make exactly %d conditional requests: total=%d non-304=%d calls=%d", want, n, r.f.nonCond304(), again.Calls)
		}
		if r.f.count(func(q reqRec) bool { return !q.Cond }) != 0 {
			t.Fatal("every repeated read must be conditional")
		}
	}

	// A new pull request changes the list: the scan reads it and reports the new head.
	r.f.do(func() {
		r.f.repos["o/r"].Pulls = append(r.f.repos["o/r"].Pulls, fPull{Number: 8, Head: sha('c', 8), Assoc: "OWNER"})
	})
	r.f.reset()
	changed, err := r.e.ScanHeads(ctx, "o/r", ScanOptions{})
	if err != nil || changed.NotModified {
		t.Fatalf("a changed pull list must not be NotModified: %+v %v", changed, err)
	}
	found := false
	for _, h := range changed.Heads {
		found = found || h.PR == 8
	}
	if !found {
		t.Fatalf("the new pull request head is missing: %+v", changed.Heads)
	}
}
