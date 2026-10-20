package burst_test

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/burst"
	"github.com/nself-org/plugins/free/ci/internal/nodes/deployguard"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

const otherCoord = "01J00000000000000000COORD2"

// put records an intent row directly (the sweeper reads rows, whoever wrote them).
func (r *rig) put(token, provider string, state string) {
	r.t.Helper()
	ctx := context.Background()
	err := r.st.BurstWrite(ctx, func(b store.BurstTx) error {
		now := b.NowMs()
		if err := b.PutIntent(store.BurstIntent{Token: token, Coordinator: coord, Provider: provider, JobKey: "j-" + token, Platform: "linux/amd64", LifetimeMs: r.cfg.LifetimeMs(), CreatedAt: now},
			store.BurstLease{IdleUntil: now + 300_000, MaxUntil: now + 3_600_000}); err != nil {
			return err
		}
		switch state {
		case store.BurstTearingDown:
			return b.MarkTearingDown(token, "test", now)
		case store.BurstTerminal:
			return b.MarkTerminal(token, now)
		}
		return nil
	})
	if err != nil {
		r.t.Fatal(err)
	}
}

func token(i int) string { return fmt.Sprintf("01J0000000000000000TOK%05d", i) }

func TestSweepNeverDeletesForeignUnlabelledOrExcluded(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	old := 10 * time.Hour
	foreign := r.cloud.AddForeign(burst.IntentLabels(otherCoord, token(1)), addr("192.0.2.201"))
	unlabelled := r.cloud.AddForeign(nil, addr("192.0.2.202"))
	otherRole := r.cloud.AddForeign(map[string]string{"role": "builder", burst.LabelCoordinator: coord}, addr("192.0.2.203"))
	r.put(token(2), "fakecloud", store.BurstTerminal)
	excl := r.cloud.AddForeign(burst.IntentLabels(coord, token(2)), addr("198.51.100.77")) // terminal intent, but a deploy host
	r.exclude = []string{"198.51.100.77"}
	// a label that claims this coordinator while the intent row belongs to another one is not touched either
	stolen := r.cloud.AddForeign(burst.IntentLabels(coord, token(3)), addr("192.0.2.204"))
	if err := r.st.BurstWrite(ctx, func(b store.BurstTx) error {
		return b.PutIntent(store.BurstIntent{Token: token(3), Coordinator: otherCoord, Provider: "fakecloud", JobKey: "j3", Platform: "x", CreatedAt: b.NowMs()}, store.BurstLease{})
	}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{foreign, unlabelled, otherRole, excl, stolen} {
		r.cloud.Age(id, old)
	}
	for _, dry := range []bool{true, false} {
		rep, err := r.eng.Sweep(ctx, dry)
		if err != nil {
			t.Fatal(err)
		}
		if rep.Deleted != 0 {
			t.Fatalf("dry=%v deleted %d: %+v", dry, rep.Deleted, rep.Actions)
		}
		for _, id := range []string{foreign, unlabelled, otherRole, excl, stolen} {
			if !r.cloud.Has(id) {
				t.Fatalf("dry=%v: machine %s that is not ours (or is a deploy host) was deleted", dry, id)
			}
		}
	}
}

func TestSweepDeletesExpiredLiveTerminalAndTearingDown(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	live := r.provision("job-live")
	// terminal intent whose machine is still listed (eventual consistency, a late create)
	r.put(token(10), "fakecloud", store.BurstTerminal)
	back := r.cloud.AddForeign(burst.IntentLabels(coord, token(10)), addr("192.0.2.210"))
	// tearing-down intent whose machine survived a failed teardown
	r.put(token(11), "fakecloud", store.BurstTearingDown)
	stuck := r.cloud.AddForeign(burst.IntentLabels(coord, token(11)), addr("192.0.2.211"))

	r.clk.Advance(65 * time.Minute) // past max_lifetime (60), inside the grace (70)
	rep, err := r.eng.Sweep(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if !r.cloud.Has(live.Resource.ID) {
		t.Fatal("a live intent's machine inside max_lifetime + grace is not the sweeper's to delete")
	}
	if r.cloud.Has(back) || r.cloud.Has(stuck) || rep.Deleted != 2 {
		t.Fatalf("terminal and tearing-down intents' machines must go: %+v", rep)
	}
	if r.auditCount("burst.sweep_deleted") != 1 || r.intent(token(11)).State != store.BurstTerminal {
		t.Fatalf("terminal-intent machine deleted with an audit row, tearing-down intent ended: audit %d", r.auditCount("burst.sweep_deleted"))
	}
	r.clk.Advance(6 * time.Minute) // 71 minutes: past max_lifetime + grace
	if _, err = r.eng.Sweep(ctx, false); err != nil {
		t.Fatal(err)
	}
	if r.cloud.Has(live.Resource.ID) {
		t.Fatal("age forces a deletion past max_lifetime + grace, even for a live intent")
	}
	if got := r.intent(live.Token); got.State != store.BurstTerminal {
		t.Fatalf("%+v", got)
	}
	if row := r.resourceRow(live.Token); row.ConfirmedTeardownAt == nil {
		t.Fatal("reservation released after the forced deletion")
	}
}

func TestSweepDryRunChangesNothing(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	live := r.provision("job-live")
	r.put(token(20), "fakecloud", store.BurstTerminal)
	back := r.cloud.AddForeign(burst.IntentLabels(coord, token(20)), addr("192.0.2.220"))
	orphan := r.cloud.AddForeign(burst.IntentLabels(coord, token(21)), addr("192.0.2.221")) // own labels, no intent row
	r.cloud.Age(orphan, 3*time.Hour)
	before := len(r.cloud.All())
	audit := r.auditCount("burst.sweep_deleted")
	rep, err := r.eng.Sweep(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Deleted != 0 || len(r.cloud.All()) != before || r.auditCount("burst.sweep_deleted") != audit || len(r.alerts) != 0 {
		t.Fatalf("--dry-run changed something: deleted %d machines %d alerts %v", rep.Deleted, len(r.cloud.All()), r.alerts)
	}
	planned := map[string]burst.SweepAction{}
	for _, a := range rep.Actions {
		planned[a.Resource] = a
	}
	if !planned[back].Delete || !planned[orphan].Delete || !planned[orphan].Intentless || planned[live.Resource.ID].Delete {
		t.Fatalf("dry run must list the terminal-intent machine and the intent-less orphan as deletions, and keep the live one: %+v", rep.Actions)
	}
}

func TestSweepIntentlessMachineIsAlertedThenDeletedAfterMaxLifetimePlusGrace(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	orphan := r.cloud.AddForeign(burst.IntentLabels(coord, token(30)), addr("192.0.2.230"))
	r.clk.Advance(30 * time.Minute)
	for i := 0; i < 2; i++ {
		if _, err := r.eng.Sweep(ctx, false); err != nil {
			t.Fatal(err)
		}
	}
	if !r.cloud.Has(orphan) || len(r.alerts) != 1 || r.alerts[0].Kind != burst.AlertOrphan {
		t.Fatalf("a young intent-less machine is kept and alerted once: has=%v alerts=%v", r.cloud.Has(orphan), r.alerts)
	}
	r.clk.Advance(41 * time.Minute) // 71 minutes old
	if _, err := r.eng.Sweep(ctx, false); err != nil {
		t.Fatal(err)
	}
	if r.cloud.Has(orphan) || r.auditCount("burst.sweep_deleted") != 1 {
		t.Fatal("deleted after max_lifetime + grace, with an audit row")
	}
}

func TestSweepFailsClosedWithoutTheExclusionSet(t *testing.T) {
	r := newRig(t)
	r.put(token(40), "fakecloud", store.BurstTerminal)
	m := r.cloud.AddForeign(burst.IntentLabels(coord, token(40)), addr("192.0.2.240"))
	eng, err := burst.New(burst.Options{Store: r.st, Providers: []burst.Provider{r.cloud}, Coordinator: coord, Config: r.cfg, Now: r.clk.Now, Spend: r.eng2Spend(),
		Exclusion: func(context.Context) (deployguard.Set, error) {
			return deployguard.Set{}, errors.New("inventory unreadable")
		}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = eng.Sweep(context.Background(), false); err == nil || !r.cloud.Has(m) {
		t.Fatalf("an unreadable exclusion set must stop the sweep without deleting: %v", err)
	}
}

// TestSweepProperty: random clouds against an independent statement of the rules.
func TestSweepProperty(t *testing.T) {
	seed := int64(77)
	if v := os.Getenv("BURST_SEED"); v != "" {
		seed, _ = strconv.ParseInt(v, 10, 64)
	}
	t.Logf("seed %d (set BURST_SEED to replay)", seed)
	rnd := rand.New(rand.NewSource(seed))
	for round := 0; round < 40; round++ {
		r := newRig(t)
		type m struct {
			id        string
			own, excl bool
			state     string // "" = no intent row
			age       time.Duration
		}
		var ms []*m
		for i := 0; i < 12; i++ {
			x := &m{age: time.Duration(rnd.Intn(100)) * time.Minute}
			tok := token(round*100 + i)
			ip := addr(fmt.Sprintf("192.0.2.%d", 10+i))
			x.excl = rnd.Intn(5) == 0
			if x.excl {
				ip = addr(fmt.Sprintf("198.51.100.%d", 10+i))
				r.exclude = append(r.exclude, ip.String())
			}
			switch rnd.Intn(5) {
			case 0: // unlabelled
				x.id = r.cloud.AddForeign(nil, ip)
			case 1: // another coordinator's
				x.id = r.cloud.AddForeign(burst.IntentLabels(otherCoord, tok), ip)
			default:
				x.own = true
				x.id = r.cloud.AddForeign(burst.IntentLabels(coord, tok), ip)
				switch rnd.Intn(4) {
				case 0:
					x.state = ""
				case 1:
					x.state = store.BurstLive
				case 2:
					x.state = store.BurstTearingDown
				case 3:
					x.state = store.BurstTerminal
				}
				if x.state != "" {
					r.put(tok, "fakecloud", x.state)
				}
			}
			r.cloud.Age(x.id, x.age)
			ms = append(ms, x)
		}
		r.build() // exclusion list is read at use; rebuild is harmless
		before := len(r.cloud.All())
		if rep, err := r.eng.Sweep(context.Background(), true); err != nil || rep.Deleted != 0 || len(r.cloud.All()) != before {
			t.Fatalf("seed %d round %d: dry run changed the cloud: %v", seed, round, err)
		}
		if _, err := r.eng.Sweep(context.Background(), false); err != nil {
			t.Fatalf("seed %d round %d: %v", seed, round, err)
		}
		for _, x := range ms {
			want := x.own && !x.excl && (x.state == store.BurstTearingDown || x.state == store.BurstTerminal || x.age > 70*time.Minute)
			if got := !r.cloud.Has(x.id); got != want {
				t.Fatalf("seed %d round %d: machine own=%v excluded=%v intent=%q age=%v: deleted=%v, want %v", seed, round, x.own, x.excl, x.state, x.age, got, want)
			}
		}
	}
}

// TestSweepFiltersLabelsItself: the sweeper does not rely on the provider honouring the selector.
func TestSweepFiltersLabelsItself(t *testing.T) {
	r := newRig(t, func(r *rig) { r.wrap = func(p burst.Provider) burst.Provider { return superset{p} } })
	foreign := r.cloud.AddForeign(burst.IntentLabels(otherCoord, token(50)), addr("192.0.2.150"))
	unlabelled := r.cloud.AddForeign(nil, addr("192.0.2.151"))
	r.cloud.Age(foreign, 10*time.Hour)
	r.cloud.Age(unlabelled, 10*time.Hour)
	if _, err := r.eng.Sweep(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if !r.cloud.Has(foreign) || !r.cloud.Has(unlabelled) {
		t.Fatal("a provider that lists everything must not make the sweeper delete what is not ours")
	}
}

type zeroCreated struct{ burst.Provider }

func (z zeroCreated) List(ctx context.Context, l map[string]string) ([]burst.Resource, error) {
	rs, err := z.Provider.List(ctx, l)
	for i := range rs {
		rs[i].CreatedAt = time.Time{}
	}
	return rs, err
}

// TestSweepUnknownAgeIsNotInfiniteAge: a provider that reports no creation time must not make a
// young live machine look expired; an intent-less machine of unknown age is kept and alerted.
func TestSweepUnknownAgeIsNotInfiniteAge(t *testing.T) {
	r := newRig(t, func(r *rig) { r.wrap = func(p burst.Provider) burst.Provider { return zeroCreated{p} } })
	live := r.provision("job-1")
	orphan := r.cloud.AddForeign(burst.IntentLabels(coord, token(60)), addr("192.0.2.160"))
	if _, err := r.eng.Sweep(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if !r.cloud.Has(live.Resource.ID) || !r.cloud.Has(orphan) {
		t.Fatal("unknown creation time was read as expired")
	}
	r.clk.Advance(71 * time.Minute) // the intent's own age now forces the live machine
	if _, err := r.eng.Sweep(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if r.cloud.Has(live.Resource.ID) {
		t.Fatal("the intent's age still forces a deletion")
	}
}

func TestSweepRetriesAFailedDirectDelete(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	r.put(token(70), "fakecloud", store.BurstTerminal)
	m := r.cloud.AddForeign(burst.IntentLabels(coord, token(70)), addr("192.0.2.170"))
	r.cloud.FailTeardown(errors.New("api 500"))
	if _, err := r.eng.Sweep(ctx, false); burst.CodeOf(err) != burst.CodeTeardownFailed || !r.cloud.Has(m) {
		t.Fatalf("a failed delete is reported: %v", err)
	}
	r.cloud.FailTeardown(nil)
	if _, err := r.eng.Sweep(ctx, false); err != nil || r.cloud.Has(m) {
		t.Fatalf("the next sweep deletes it: %v", err)
	}
}

func TestSweepAlertsOnceForAnOwnMachineOnADeployHost(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	m := r.cloud.AddForeign(burst.IntentLabels(coord, token(80)), addr("198.51.100.80"))
	r.exclude = []string{"198.51.100.80"}
	r.cloud.Age(m, 5*time.Hour)
	for i := 0; i < 3; i++ {
		if _, err := r.eng.Sweep(ctx, false); err != nil {
			t.Fatal(err)
		}
	}
	if !r.cloud.Has(m) || len(r.alerts) != 1 || r.alerts[0].Kind != burst.AlertExcluded {
		t.Fatalf("kept=%v alerts=%v", r.cloud.Has(m), r.alerts)
	}
}
