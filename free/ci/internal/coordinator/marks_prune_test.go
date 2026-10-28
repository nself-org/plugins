package coordinator

// Purpose: the maintenance prune of provider marks (D-1319).

import (
	"context"
	"testing"
	"time"
)

func TestPruneForgetsOldReconcileMarks(t *testing.T) {
	ctx, r := context.Background(), newRig(t, nil)
	now := r.clk.Now()
	day := 24 * time.Hour
	put := func(name, key string, age time.Duration) {
		t.Helper()
		if err := r.st.ProviderMarkPut(ctx, name, key, now.Add(-age)); err != nil {
			t.Fatal(err)
		}
	}
	put("github-reconcile", "run|o/r|1", 8*day)
	put("github-reconcile", "run|o/r|2", 6*day)
	put("github-reconcile", "demand|o/r|1", 8*day)
	put("github-reconcile", "term|o/r|abc", 29*day)
	put("github-reconcile", "enq|o/r|abc", 31*day)
	put("github-reconcile", "redeliver|o/r|g", 31*day)
	put("github-usage", "acct", 90*day) // another name: never touched by the github-reconcile rules
	r.c.prune(ctx, now)
	for _, m := range []struct {
		name, key string
		want      bool
	}{
		{"github-reconcile", "run|o/r|1", false}, {"github-reconcile", "run|o/r|2", true},
		{"github-reconcile", "demand|o/r|1", false}, {"github-reconcile", "term|o/r|abc", true},
		{"github-reconcile", "enq|o/r|abc", false}, {"github-reconcile", "redeliver|o/r|g", false},
		{"github-usage", "acct", true},
	} {
		if _, ok, err := r.st.ProviderMarkGet(ctx, m.name, m.key); err != nil || ok != m.want {
			t.Fatalf("mark %s/%s present = %v (%v), want %v", m.name, m.key, ok, err, m.want)
		}
	}
}

func TestPruneKeepsMarksWhenRetentionIsEmpty(t *testing.T) {
	ctx, r := context.Background(), newRig(t, func(c *Config) { c.MarkRetention = []MarkRetention{} })
	if err := r.st.ProviderMarkPut(ctx, "github-reconcile", "enq|o/r|a", r.clk.Now().Add(-90*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	r.c.prune(ctx, r.clk.Now())
	if _, ok, _ := r.st.ProviderMarkGet(ctx, "github-reconcile", "enq|o/r|a"); !ok {
		t.Fatal("an empty retention list must keep every mark")
	}
}
