package sim

// Purpose: P7-AUTO-19 acceptance A9: with one paused (dead) node among four, a fleet status pass
// returns in under 15 s with that node unreachable and the other three complete.
// Inputs:  a state.db in t.TempDir() with four registered nodes; a reader whose dead node never
//          answers and ignores its context (a paused host). Outputs: pass or fail.
// Constraints: pure Go, no processes, no network. The dead node's 8 s cap is an injected timer that
//          fires when its reader starts, so the test proves the pass stops waiting at the cap without
//          sleeping through it.

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/demand"
	"github.com/nself-org/plugins/free/ci/internal/fleet"
	"github.com/nself-org/plugins/free/ci/internal/nodes/probe"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestStatusOneDeadNodeUnder15s(t *testing.T) {
	st := openSimStore(t)
	for _, id := range []string{"n1", "n2", "n3", "paused"} {
		whereNode(t, st, id)
	}
	at := time.Date(2026, 10, 27, 12, 0, 10, 0, time.UTC)
	stamp := at.Add(-10 * time.Second).Format(time.RFC3339)
	live, err := json.Marshal(demand.Document{TS: stamp, Host: "h", Queued: 1, Running: 1, Ready: true, ReadyWhy: []string{},
		Watcher: demand.Watcher{Alive: true, LastPass: stamp, IntervalSec: 30}})
	if err != nil {
		t.Fatal(err)
	}
	paused := make(chan struct{})
	t.Cleanup(func() { close(paused) })
	expired := make(chan time.Time)
	var once sync.Once
	var mu sync.Mutex
	caps := map[string]time.Duration{}
	timer := func(id string, d time.Duration) <-chan time.Time {
		mu.Lock()
		defer mu.Unlock()
		caps[id] = d
		if id == "paused" {
			return expired
		}
		return nil
	}
	read := func(ctx context.Context, n registry.Node) (probe.StatusDocs, error) {
		if n.Record.ID == "paused" {
			once.Do(func() { close(expired) }) // the cap passes while the host stays silent
			<-paused
			return probe.StatusDocs{}, ctx.Err()
		}
		return probe.StatusDocs{Demand: live}, nil
	}
	start := time.Now()
	rep, err := fleet.Gather(context.Background(), st, read, fleet.Options{Timer: timer, Now: func() time.Time { return at }})
	if err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took >= 15*time.Second {
		t.Fatalf("status pass took %s, want < 15s", took)
	}
	if len(rep.Nodes) != 4 || rep.Unreachable != 1 {
		t.Fatalf("report = %+v", rep)
	}
	for _, row := range rep.Nodes {
		if caps[row.Node] != fleet.NodeCap {
			t.Errorf("%s cap = %s, want %s", row.Node, caps[row.Node], fleet.NodeCap)
		}
		if row.Node == "paused" {
			if row.Source != fleet.SourceUnreachable || row.Unreachable == "" {
				t.Errorf("paused row = %+v", row)
			}
			continue
		}
		if row.Source != fleet.SourceLive || row.Watcher != fleet.WatcherAlive || row.Ready == nil || !*row.Ready || row.Queued == nil || *row.Queued != 1 {
			t.Errorf("%s row incomplete: %+v", row.Node, row)
		}
	}
}

// openSimStore opens a fresh state database for one test.
func openSimStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(t.TempDir()+"/state.db", store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}
