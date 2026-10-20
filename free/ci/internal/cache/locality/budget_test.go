package locality

// Purpose: the 50 ms budget of one world build (Epic CACHE D8): 50 runners x 200 jobs.

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/detect"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// bigWorld has 50 runners (two platforms, a few holders) and 200 jobs over eight lockfile variants,
// half of them with a stored entry and a third of the nodes holding a copy.
func bigWorld(t testing.TB) (sched.World, Inputs) {
	t.Helper()
	w := sched.World{NowMs: nowMs}
	for i := 0; i < 50; i++ {
		platform := platformLinux
		if i%2 == 1 {
			platform = "darwin/arm64"
		}
		w.Runners = append(w.Runners, runner(fmt.Sprintf("node-%02d", i), platform, i == 0))
	}
	ledger := &fakeLedger{entries: map[string][]entries.Manifest{}}
	var jobs []Job
	for i := 0; i < 200; i++ {
		facts := []detect.Fact{{Kind: "stack", Name: "go", Path: "go.mod", Confidence: detect.Certain},
			{Kind: "lockfile", Name: "go", Path: "go.sum", SHA256: strings.Repeat(fmt.Sprintf("%02x", i%8), 32), Confidence: detect.Certain}}
		j := Job{Key: fmt.Sprintf("job%03d[m]", i), Facts: ownerFacts, Detected: facts, Image: imageDigest,
			Probe: func(context.Context, []string) (string, error) { return "go version go1.22.0", nil }}
		jobs = append(jobs, j)
		if i%2 == 0 {
			for _, platform := range []string{platformLinux, "darwin/arm64"} {
				c := planned(t, j, platform, "go")
				ledger.entries[c.key] = []entries.Manifest{entry(c, 1000+int64(i))}
				for n := 2; n < 50; n += 3 {
					ledger.rows = append(ledger.rows, store.CacheLocalityRow{Node: fmt.Sprintf("node-%02d", n), Realm: c.realm, Key: c.key, Bytes: 1, LastSeen: time.UnixMilli(nowMs)})
				}
			}
		}
	}
	return w, Inputs{Ledger: ledger, Jobs: jobs}
}

func TestContributorBudget(t *testing.T) {
	w, in := bigWorld(t)
	unbind := Bind(func(context.Context) (Inputs, error) { return in, nil })
	defer unbind()
	best := time.Hour
	for i := 0; i < 5; i++ { // best of five: a loaded CI host can stall one run
		fresh := w
		start := time.Now()
		if err := Contribute(context.Background(), &fresh); err != nil {
			t.Fatal(err)
		}
		if d := time.Since(start); d < best {
			best = d
		}
		if len(fresh.Cache) != 50 || len(fresh.Cache["node-00"]) != 200 {
			t.Fatalf("cache shape: %d runners, %d jobs", len(fresh.Cache), len(fresh.Cache["node-00"]))
		}
	}
	if budget := 50 * time.Millisecond * raceSlowdown; best > budget {
		t.Fatalf("world build contributor took %v for 50 runners x 200 jobs, budget %v", best, budget)
	}
	t.Logf("best of five: %v", best)
}

func BenchmarkContributor(b *testing.B) {
	w, in := bigWorld(b)
	unbind := Bind(func(context.Context) (Inputs, error) { return in, nil })
	defer unbind()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fresh := w
		if err := Contribute(context.Background(), &fresh); err != nil {
			b.Fatal(err)
		}
	}
}
