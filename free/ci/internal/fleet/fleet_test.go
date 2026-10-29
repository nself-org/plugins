package fleet

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/demand"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/probe"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

var passAt = time.Date(2026, 10, 27, 12, 0, 30, 0, time.UTC)

func demandJSON(t *testing.T, ready bool, why []string, queued, running int) []byte {
	t.Helper()
	stamp := passAt.Add(-30 * time.Second).Format(time.RFC3339)
	b, err := json.Marshal(demand.Document{TS: stamp, Host: "h", Queued: queued, Running: running,
		Ready: ready, ReadyWhy: why, Watcher: demand.Watcher{Alive: true, LastPass: stamp, IntervalSec: 30}})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func node(id string, labels ...string) registry.Node {
	n := registry.Node{Record: store.NodeRecord{ID: id, Name: id, State: "online"}}
	n.Capability.Identity.Labels = labels
	n.Capability.Protocol.AgentVersion = "1.5.0"
	return n
}

// TestFleetStatusCapsPerNode: every node gets the 8 s cap, a node past its cap is unreachable with the
// reason while its reader is still blocked, and no more than Parallel readers run at once.
func TestFleetStatusCapsPerNode(t *testing.T) {
	var nodes []registry.Node
	for i := 0; i < 20; i++ {
		nodes = append(nodes, node(string(rune('a'+i))))
	}
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	var mu sync.Mutex
	caps := map[string]time.Duration{}
	fired := make(chan time.Time)
	close(fired)
	timer := func(id string, d time.Duration) <-chan time.Time {
		mu.Lock()
		defer mu.Unlock()
		caps[id] = d
		if id == "c" {
			return fired
		}
		return nil
	}
	var running, peak atomic.Int32
	read := func(ctx context.Context, n registry.Node) (probe.StatusDocs, error) {
		if n.Record.ID == "c" {
			<-release // a paused host: ignores its context; abandoned at its cap, so it frees its slot
			return probe.StatusDocs{}, nil
		}
		now := running.Add(1)
		defer running.Add(-1)
		for p := peak.Load(); now > p && !peak.CompareAndSwap(p, now); p = peak.Load() {
		}
		return probe.StatusDocs{Demand: demandJSON(t, true, nil, 1, 0)}, nil
	}
	replies := Ask(context.Background(), nodes, read, Options{Timer: timer})
	for i, r := range replies {
		id := nodes[i].Record.ID
		if caps[id] != NodeCap {
			t.Errorf("%s cap = %s, want %s", id, caps[id], NodeCap)
		}
		if id == "c" {
			if !errors.Is(r.Err, errCap) || !strings.Contains(r.Err.Error(), "8s") {
				t.Errorf("c err = %v, want the cap", r.Err)
			}
			continue
		}
		if r.Err != nil || r.Docs.Demand == nil {
			t.Errorf("%s reply = %+v", id, r)
		}
	}
	if p := peak.Load(); p > Parallel {
		t.Errorf("peak readers = %d, want <= %d", p, Parallel)
	}
	if r := askOne(context.Background(), nodes[0], nil, NodeCap, nil); r.Err == nil {
		t.Error("nil reader accepted")
	}
}

func TestFleetStatusRows(t *testing.T) {
	results := []Result{{ID: "1", Status: "passed"}, {ID: "2", Status: "failed"}, {ID: "3"}, {ID: "4"}, {ID: "5"}, {ID: "6"}}
	ready := BuildRow(node("w1", "pool.gpu=1", "role=x"), Reply{Docs: probe.StatusDocs{Demand: demandJSON(t, true, nil, 3, 2)}}, results, passAt)
	if ready.Source != SourceLive || ready.Watcher != WatcherAlive || ready.Ready == nil || !*ready.Ready || *ready.Queued != 3 || *ready.Running != 2 {
		t.Fatalf("ready row = %+v", ready)
	}
	if len(ready.Pools) != 1 || ready.Pools[0] != "gpu" || len(ready.LastResults) != MaxResults || ready.LastPass == "" || ready.Demand == nil {
		t.Fatalf("ready row pools/results = %+v", ready)
	}
	busy := BuildRow(node("w2"), Reply{Docs: probe.StatusDocs{Demand: demandJSON(t, false, []string{"memory below floor"}, 0, 0)}}, nil, passAt)
	if busy.Ready == nil || *busy.Ready || len(busy.ReadyWhy) != 1 || busy.ReadyWhy[0] != "memory below floor" {
		t.Fatalf("not-ready row = %+v", busy)
	}
	block := []byte(`{"schema":"ci.readiness/v1","ready":false,"why":["load too high"],"yielded_to":[]}`)
	over := BuildRow(node("w3"), Reply{Docs: probe.StatusDocs{Demand: demandJSON(t, true, nil, 0, 0), Readiness: block}}, nil, passAt)
	if over.Ready == nil || *over.Ready || over.ReadyWhy[0] != "load too high" {
		t.Fatalf("readiness block row = %+v", over)
	}
	stale := BuildRow(node("w4"), Reply{Docs: probe.StatusDocs{Demand: demandJSON(t, true, nil, 0, 0)}}, nil, passAt.Add(10*time.Minute))
	if stale.Watcher != WatcherStale {
		t.Fatalf("stale watcher = %q", stale.Watcher)
	}
	plain := BuildRow(node("n1"), Reply{}, nil, passAt)
	if plain.Watcher != WatcherNone || plain.Ready != nil || plain.Queued != nil || len(plain.ReadyWhy) != 1 || plain.Demand != nil {
		t.Fatalf("no-watcher row = %+v", plain)
	}
	dead := BuildRow(node("n2"), Reply{Err: errors.New("ssh: connect timed out\nmore")}, results, passAt)
	if dead.Source != SourceUnreachable || dead.Unreachable != "ssh: connect timed out" || dead.Watcher != WatcherUnknown || dead.Ready != nil {
		t.Fatalf("unreachable row = %+v", dead)
	}
	torn := BuildRow(node("n3"), Reply{Docs: probe.StatusDocs{Demand: []byte(`{"ts":`)}}, nil, passAt)
	if torn.Watcher != WatcherNone || torn.ReadyWhy[0] != "demand document unreadable" {
		t.Fatalf("torn row = %+v", torn)
	}
	var buf bytes.Buffer
	if err := Render(&buf, Report{Nodes: []Row{ready, busy, plain, dead}}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"w1", "ready", "not ready", "unreachable", "none", "gpu", "w2: memory below floor", "n2: unreachable: ssh"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("table lacks %q:\n%s", want, buf.String())
		}
	}
}

// TestStatusJSONEnvelope: rows carry the node's contract:ci.demand v1 document and arrays are never null.
func TestStatusJSONEnvelope(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	register(t, st, "alpha", "beta")
	read := func(_ context.Context, n registry.Node) (probe.StatusDocs, error) {
		if n.Record.ID == "beta" {
			return probe.StatusDocs{}, errors.New("connection refused")
		}
		return probe.StatusDocs{Demand: demandJSON(t, true, nil, 2, 1)}, nil
	}
	rep, err := Gather(context.Background(), st, read, Options{Now: func() time.Time { return passAt }})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Nodes) != 2 || rep.Unreachable != 1 || rep.Nodes[0].Node != "alpha" {
		t.Fatalf("report = %+v", rep)
	}
	b, err := json.Marshal(rep)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Nodes []map[string]json.RawMessage `json:"nodes"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(got.Nodes[0]["demand"], &doc); err != nil || doc["queued"] != float64(2) || doc["ts"] == nil {
		t.Fatalf("alpha demand = %s (%v)", got.Nodes[0]["demand"], err)
	}
	for _, key := range []string{"pools", "ready_why", "last_results"} {
		if string(got.Nodes[1][key]) == "null" {
			t.Errorf("beta %s is null", key)
		}
	}
	if string(got.Nodes[1]["source"]) != `"unreachable"` || string(got.Nodes[1]["demand"]) != "null" {
		t.Errorf("beta row = %s", b)
	}
}

// register adds online nodes from a valid capability fixture.
func register(t *testing.T, st *store.Store, ids ...string) {
	t.Helper()
	raw, err := os.ReadFile("../model/testdata/capability/valid/vps.valid.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		var c model.Capability
		if err := json.Unmarshal(raw, &c); err != nil {
			t.Fatal(err)
		}
		c.Identity.ID, c.Identity.Name = id, id
		doc, err := json.Marshal(c)
		if err != nil {
			t.Fatal(err)
		}
		rec := store.NodeRecord{ID: id, Name: id, State: "online", Lifecycle: []byte("{}"), Trust: []byte("{}"), DeployHost: []byte("{}"), Capability: doc, ObservedAt: passAt.UnixNano()}
		if err := st.NodePut(context.Background(), rec, "test"); err != nil {
			t.Fatal(err)
		}
	}
}
