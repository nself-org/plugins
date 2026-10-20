package burst_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/burst"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// spy checks, at the moment the provider is called, that the intent and its
// reservation are already committed: no provider call before the intent row.
type spy struct {
	burst.Provider
	r         *rig
	order     []string
	intentRow bool
	reserved  int64
}

func (s *spy) List(ctx context.Context, l map[string]string) ([]burst.Resource, error) {
	s.order = append(s.order, "List")
	return s.Provider.List(ctx, l)
}

func (s *spy) Provision(ctx context.Context, in burst.Intent) (burst.Resource, error) {
	s.order = append(s.order, "Provision")
	row, ok, err := s.r.st.BurstIntent(ctx, in.Token)
	s.intentRow = err == nil && ok && row.State == store.BurstLive && row.Coordinator == coord
	if res, ok, _ := s.r.st.Resource(ctx, in.Token); ok {
		s.reserved = res.ReservedMicro
	}
	return s.Provider.Provision(ctx, in)
}

func TestBurstProvisionIntentFirst(t *testing.T) {
	var sp *spy
	r := newRig(t, func(r *rig) {
		r.wrap = func(p burst.Provider) burst.Provider { sp = &spy{Provider: p, r: r}; return sp }
	})
	res := r.provision("job-1")
	if !sp.intentRow || sp.reserved <= 0 {
		t.Fatalf("at the provider call the intent row existed=%v and the reservation was %d: both must be committed first", sp.intentRow, sp.reserved)
	}
	if strings.Join(sp.order, ",") != "List,Provision" {
		t.Fatalf("a provision lists by the intent label before it creates: %v", sp.order)
	}
	if len(res.Token) != 26 || res.Resource.Labels[burst.LabelIntent] != res.Token || res.Resource.Labels[burst.LabelCoordinator] != coord || res.Resource.Labels[burst.LabelRole] != burst.RoleEphemeral {
		t.Fatalf("token %q labels %v", res.Token, res.Resource.Labels)
	}
	in := r.intent(res.Token)
	row := r.resourceRow(res.Token)
	if in.ResourceID != res.Resource.ID || in.ReservedMicro != row.ReservedMicro || in.ReservedMicro <= 0 || in.State != store.BurstLive || in.LifetimeMs != r.cfg.LifetimeMs() {
		t.Fatalf("intent %+v, ledger %+v", in, row)
	}
	// lifetime reservation = ceil(105 min / 1 min) x 1 min at 3.6e6 per hour = 105 x 60 000
	if want := int64(105 * 60_000); row.ReservedMicro != want {
		t.Fatalf("reserved %d, want %d (max_lifetime + grace + sweep + retry window)", row.ReservedMicro, want)
	}
	ds, err := r.st.BurstDecisions(context.Background(), "job-1")
	if err != nil || len(ds) != 1 || !ds[0].Allowed || ds[0].Reason != burst.ReasonAllowed {
		t.Fatalf("decision log: %+v %v", ds, err)
	}
	var rec burst.Input
	if err = json.Unmarshal([]byte(ds[0].Input), &rec); err != nil || rec.Provider != "fakecloud" || !rec.Enabled || !rec.PaidAllowed || !rec.ReservationFits || rec.ExpectedWaitMs != 600_000 || rec.MaxConcurrent != 2 {
		t.Fatalf("the record must hold every input: %s (%v)", ds[0].Input, err)
	}
}

func TestBurstProvisionRefusals(t *testing.T) {
	for _, c := range []struct {
		name   string
		tweak  func(*rig)
		mutate func(*burst.Request)
		reason string
	}{
		{"personal mode", func(r *rig) { r.team = false }, nil, burst.ReasonTeamOnly},
		{"provider not enabled", func(r *rig) { r.pol.Burst.Enabled["fakecloud"] = false }, nil, burst.ReasonDisabled},
		{"paid spend off", func(r *rig) { r.pol.PaidEnabled = false }, nil, burst.ReasonPaidDisabled},
		{"no budget set", func(r *rig) { r.pol.Budgets.Day.Set = false }, nil, burst.ReasonPaidDisabled},
		{"reservation does not fit the budget", func(r *rig) { r.pol.Budgets.Day.Micro = 1_000 }, nil, burst.ReasonNoFit},
		{"wait too short to be worth a machine", nil, func(q *burst.Request) { q.ExpectedWaitMs = 150_000 }, burst.ReasonWaitShort},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t, c.tweak)
			calls := r.cloud.Calls["Provision"]
			q := r.request("job-x")
			if c.mutate != nil {
				c.mutate(&q)
			}
			res, err := r.eng.Provision(context.Background(), q)
			if burst.CodeOf(err) != burst.CodeProvisionRefused || burst.ReasonOf(err) != c.reason || res.Decision.Allow {
				t.Fatalf("got %v (%s), want E696 %s", err, res.Decision.Reason, c.reason)
			}
			if r.cloud.Calls["Provision"] != calls || len(r.cloud.All()) != 0 {
				t.Fatal("a refused decision must not call the provider")
			}
			if _, ok, _ := r.st.Resource(context.Background(), res.Token); ok {
				t.Fatal("a refused decision must not reserve money")
			}
			ds, _ := r.st.BurstDecisions(context.Background(), "job-x")
			if len(ds) != 1 || ds[0].Allowed || ds[0].Reason != c.reason {
				t.Fatalf("the refusal is logged with its reason: %+v", ds)
			}
		})
	}
}

func TestBurstProvisionCaps(t *testing.T) {
	r := newRig(t)
	a := r.provision("job-a")
	second := r.request("job-b")
	second.QueuedDemand = 2 // two queued jobs, one machine already on its way
	if _, err := r.eng.Provision(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	more := r.request("job-c")
	more.QueuedDemand = 5
	if _, err := r.eng.Provision(context.Background(), more); burst.ReasonOf(err) != burst.ReasonProviderCap {
		t.Fatalf("a third open machine on a provider capped at 2: %v", err)
	}
	// one queued job, one provision per window: the same job asks again while its machine boots.
	again := r.request("job-a")
	again.QueuedDemand = 5
	r.pol.Burst.MaxConcurrent = 9
	r.build()
	if _, err := r.eng.Provision(context.Background(), again); burst.ReasonOf(err) != burst.ReasonJobWindowCap {
		t.Fatalf("the same job twice in its window: %v", err)
	}
	// the machine on its way counts as capacity for a single queued job.
	if _, err := r.eng.Provision(context.Background(), r.request("job-d")); burst.ReasonOf(err) != burst.ReasonInFlightCovers {
		t.Fatalf("machines already on their way cover the demand: %v", err)
	}
	// after the window, the same job may provision again.
	r.clk.Advance(5 * time.Minute)
	if err := r.eng.Teardown(context.Background(), a.Token, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.eng.Provision(context.Background(), again); err != nil {
		t.Fatalf("after the window and a teardown: %v", err)
	}
}

func TestBurstJoinBreaker(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	for i, job := range []string{"j1", "j2", "j3"} {
		res := r.provision(job)
		if err := r.eng.JoinFailed(ctx, res.Token); err != nil {
			t.Fatalf("join failure %d: %v", i+1, err)
		}
		if got := r.intent(res.Token); got.State != store.BurstTerminal {
			t.Fatalf("a failed join ends the intent: %+v", got)
		}
		r.clk.Advance(5 * time.Minute)
	}
	if len(r.cloud.All()) != 0 {
		t.Fatal("a machine that failed to join was left running")
	}
	if _, err := r.eng.Provision(ctx, r.request("j4")); burst.ReasonOf(err) != burst.ReasonBreakerOpen {
		t.Fatalf("three consecutive join failures open the breaker: %v", err)
	}
	r.clk.Advance(15 * time.Minute)
	res, err := r.eng.Provision(ctx, r.request("j4"))
	if err != nil {
		t.Fatalf("the breaker closes after 15 minutes: %v", err)
	}
	r.join(res.Token, "node-ok")
	if b, _ := r.st.Breaker(ctx, "fakecloud"); b.Failures != 0 {
		t.Fatalf("a successful join resets the breaker: %+v", b)
	}
}

func TestBurstResumeAdoptsByIntentLabel(t *testing.T) {
	r := newRig(t)
	r.fault = func(p string) error {
		if p == burst.PointAfterCreate {
			return burst.ErrKilled
		}
		return nil
	}
	res, err := r.eng.Provision(context.Background(), r.request("job-1"))
	if !errors.Is(err, burst.ErrKilled) || res.Token == "" {
		t.Fatalf("the kill: %v %+v", err, res)
	}
	if in := r.intent(res.Token); in.ResourceID != "" {
		t.Fatalf("the id was never recorded: %+v", in)
	}
	r.fault = nil
	r.build() // the restarted coordinator
	provisions := r.cloud.Calls["Provision"]
	got, err := r.eng.Resume(context.Background(), res.Token, r.request("job-1").Spec)
	if err != nil {
		t.Fatal(err)
	}
	if r.cloud.Calls["Provision"] != provisions {
		t.Fatal("adoption is by listing the intent label: Provision must not be called when the machine already exists")
	}
	all := r.cloud.All()
	if len(all) != 1 || all[0].ID != got.ID || r.intent(res.Token).ResourceID != got.ID {
		t.Fatalf("resume must adopt the machine the lost call created, not create a second: %+v", all)
	}
}

func TestBurstLostResponseIsCleanedUp(t *testing.T) {
	r := newRig(t)
	r.cloud.LoseResponse(1)
	res, err := r.eng.Provision(context.Background(), r.request("job-1"))
	if burst.ReasonOf(err) != "provider.error" {
		t.Fatalf("a provider error is an E696: %v", err)
	}
	if len(r.cloud.All()) != 0 {
		t.Fatal("the machine whose response was lost is still running")
	}
	if in := r.intent(res.Token); in.State != store.BurstTerminal {
		t.Fatalf("intent %+v", in)
	}
	if row := r.resourceRow(res.Token); row.ConfirmedTeardownAt == nil {
		t.Fatal("the reservation is released once the machine is confirmed gone")
	}
}

func TestBurstLocalProviderWorksInPersonalMode(t *testing.T) {
	r := newRig(t, func(r *rig) {
		r.team = false
		r.cloud.SetID("fakelocal")
		r.cloud.ClassName = burst.ClassLocal
		r.pol.Burst.Enabled["fakelocal"] = true
		r.pol.PaidEnabled = false
	})
	res := r.provision("job-1")
	if _, ok, _ := r.st.Resource(context.Background(), res.Token); ok {
		t.Fatal("a local machine costs nothing, so nothing is reserved")
	}
	if err := r.eng.Teardown(context.Background(), res.Token, "done"); err != nil {
		t.Fatal(err)
	}
	if in := r.intent(res.Token); in.State != store.BurstTerminal || len(r.cloud.All()) != 0 {
		t.Fatalf("%+v", in)
	}
}

// TestBurstConcurrentProvisionsShareOneWindow: one queued job asks twice at once, from the same engine
// and from a second engine on the same store; exactly one machine appears.
func TestBurstConcurrentProvisionsShareOneWindow(t *testing.T) {
	for _, name := range []string{"same engine", "two engines"} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t)
			// A barrier between the counters and the commit makes the race certain: both callers have counted an empty window
			// before either commits, unless something serializes them.
			var bm sync.Mutex
			arrived, gate := 0, make(chan struct{})
			r.teamHook = func() {
				bm.Lock()
				arrived++
				if arrived == 2 {
					close(gate)
				}
				bm.Unlock()
				select {
				case <-gate:
				case <-time.After(300 * time.Millisecond):
				}
			}
			r.build()
			second := r.eng
			if name == "two engines" {
				second = r.newEngine()
			}
			q := r.request("job-1")
			q.QueuedDemand = 3
			var wg sync.WaitGroup
			errs := make([]error, 2)
			for i, e := range []*burst.Engine{r.eng, second} {
				wg.Add(1)
				go func() {
					defer wg.Done()
					_, errs[i] = e.Provision(context.Background(), q)
				}()
			}
			wg.Wait()
			ok := 0
			for _, err := range errs {
				if err == nil {
					ok++
				} else if burst.CodeOf(err) != burst.CodeProvisionRefused {
					t.Fatalf("unexpected error: %v", err)
				}
			}
			if ok != 1 || len(r.cloud.All()) != 1 {
				t.Fatalf("%d succeeded, %d machines: one job, one provision per window", ok, len(r.cloud.All()))
			}
		})
	}
}

type listFails struct{ burst.Provider }

func (listFails) List(context.Context, map[string]string) ([]burst.Resource, error) {
	return nil, burst.ErrUnavailable
}

func TestBurstNoCreateWithoutAList(t *testing.T) {
	r := newRig(t, func(r *rig) { r.wrap = func(p burst.Provider) burst.Provider { return listFails{p} } })
	if _, err := r.eng.Provision(context.Background(), r.request("job-1")); burst.ReasonOf(err) != "provider.error" {
		t.Fatalf("%v", err)
	}
	if r.cloud.Calls["Provision"] != 0 {
		t.Fatal("Provision was called although the adopt-by-label List failed")
	}
}

func TestBurstEngineRejectsACoordinatorThatIsNotALabel(t *testing.T) {
	r := newRig(t)
	for _, bad := range []string{"", "has space", strings.Repeat("x", 64), "a=b", "ü"} {
		if _, err := burst.New(burst.Options{Store: r.st, Coordinator: bad, Spend: r.eng2Spend(), Exclusion: r.eng2Exclusion()}); err == nil {
			t.Errorf("coordinator %q accepted", bad)
		}
	}
}
