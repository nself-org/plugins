package burst_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/burst"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestBurstTeardownRevokesAndReleasesOnlyWhenGone(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	res := r.provision("job-1")
	r.join(res.Token, "node-1")
	r.clk.Advance(20 * time.Minute)
	if err := r.eng.Teardown(ctx, res.Token, "job done"); err != nil {
		t.Fatal(err)
	}
	if len(r.revoked) != 1 || r.revoked[0] != "node-1" || len(r.denied) != 1 || r.denied[0] != "node-1" {
		t.Fatalf("NodeRevoke and the deny-list hook must run for the node: revoked %v denied %v", r.revoked, r.denied)
	}
	if n, err := r.st.NodeGet(ctx, "node-1"); err != nil || n.State != "revoked" {
		t.Fatalf("node %+v %v", n, err)
	}
	if len(r.cloud.All()) != 0 {
		t.Fatal("the machine is still there")
	}
	in := r.intent(res.Token)
	row := r.resourceRow(res.Token)
	if in.State != store.BurstTerminal || row.ConfirmedTeardownAt == nil || row.ActualMicro == nil {
		t.Fatalf("intent %+v ledger %+v", in, row)
	}
	// 20 minutes at 3.6e6 micro an hour with a one-minute increment is 20 x 60 000, well under the reservation.
	if *row.ActualMicro != 20*60_000 || *row.ActualMicro > row.ReservedMicro {
		t.Fatalf("actual %d reserved %d", *row.ActualMicro, row.ReservedMicro)
	}
	if r.auditCount("burst.teardown") != 1 || r.auditCount("node.revoke") != 1 {
		t.Fatalf("audit rows: teardown %d revoke %d", r.auditCount("burst.teardown"), r.auditCount("node.revoke"))
	}
	if err := r.eng.Teardown(ctx, res.Token, "again"); err != nil {
		t.Fatalf("a second teardown of a terminal intent is a no-op: %v", err)
	}
	if len(r.alerts) != 0 {
		t.Fatalf("alerts %v", r.alerts)
	}
}

func TestBurstTeardownRefusesAnotherCoordinatorsIntent(t *testing.T) {
	r := newRig(t)
	res := r.provision("job-1")
	other, err := burst.New(burst.Options{Store: r.st, Providers: []burst.Provider{r.cloud}, Coordinator: "01J00000000000000000COORD2", Spend: r.eng2Spend(), Exclusion: r.eng2Exclusion()})
	if err != nil {
		t.Fatal(err)
	}
	if err = other.Teardown(context.Background(), res.Token, "x"); burst.CodeOf(err) != burst.CodeTeardownFailed {
		t.Fatalf("another coordinator tore down this intent: %v", err)
	}
	if len(r.cloud.All()) != 1 {
		t.Fatal("the machine must be untouched")
	}
}

// TestBurstTeardownFailsForTwoHoursThenRecovers: the API is down for 2 h; every
// sweep retries and fails, the operator is told once, the provider takes no new
// machine, the reservation stays; the first sweep after recovery deletes the
// machine and only then releases the money.
func TestBurstTeardownFailsForTwoHoursThenRecovers(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	res := r.provision("job-1")
	r.join(res.Token, "node-1")
	r.clk.Advance(30 * time.Minute)
	r.cloud.Outage(true)
	err := r.eng.Teardown(ctx, res.Token, "job done")
	if burst.CodeOf(err) != burst.CodeTeardownFailed {
		t.Fatalf("want E697: %v", err)
	}
	if got := r.intent(res.Token); got.State != store.BurstTearingDown {
		t.Fatalf("a failed teardown leaves the intent tearing-down: %+v", got)
	}
	if row := r.resourceRow(res.Token); row.ConfirmedTeardownAt != nil {
		t.Fatal("money released before the machine was confirmed gone")
	}
	blocked := r.request("job-2")
	blocked.QueuedDemand = 3
	if _, perr := r.eng.Provision(ctx, blocked); burst.ReasonOf(perr) != burst.ReasonTeardownFailed {
		t.Fatalf("a failed teardown blocks new provisioning on the provider: %v", perr)
	}
	for i := 0; i < 24; i++ { // two hours of sweeps, every five minutes
		r.clk.Advance(5 * time.Minute)
		if _, terr := r.eng.Tick(ctx); terr == nil {
			t.Fatalf("sweep %d reported success during the outage", i)
		}
		if row := r.resourceRow(res.Token); row.ConfirmedTeardownAt != nil {
			t.Fatalf("sweep %d released the reservation during the outage", i)
		}
	}
	if len(r.alerts) != 1 || r.alerts[0].Kind != burst.AlertTeardownFailed || r.auditCount("burst.teardown_failed") != 1 {
		t.Fatalf("one alert and one audit row for the whole outage: alerts %v audit %d", r.alerts, r.auditCount("burst.teardown_failed"))
	}
	r.cloud.Outage(false)
	if _, terr := r.eng.Tick(ctx); terr != nil {
		t.Fatalf("the first sweep after recovery: %v", terr)
	}
	if len(r.cloud.All()) != 0 {
		t.Fatal("the machine survived the first sweep after the API recovered")
	}
	in := r.intent(res.Token)
	row := r.resourceRow(res.Token)
	if in.State != store.BurstTerminal || row.ConfirmedTeardownAt == nil {
		t.Fatalf("intent %+v ledger %+v", in, row)
	}
	// the machine billed 30 min + 2 h; the actual exceeds the 105 min reservation and the budgets show it.
	if row.ActualMicro == nil || *row.ActualMicro <= row.ReservedMicro {
		t.Fatalf("actual %v reserved %d", row.ActualMicro, row.ReservedMicro)
	}
	if len(r.alerts) != 1 || r.auditCount("burst.teardown_failed") != 1 {
		t.Fatal("recovery must not alert again")
	}
	if _, perr := r.eng.Provision(ctx, blocked); perr != nil && burst.ReasonOf(perr) == burst.ReasonTeardownFailed {
		t.Fatalf("provisioning stays blocked after the machine is gone: %v", perr)
	}
}

type secretErr struct{}

func (secretErr) Error() string { return "403 token=FAKE-PROVIDER-CRED-0000 rejected" }

func TestBurstTeardownTextsCarryNoProviderError(t *testing.T) {
	r := newRig(t)
	res := r.provision("job-1")
	r.cloud.FailTeardown(secretErr{})
	err := r.eng.Teardown(context.Background(), res.Token, "job done")
	if burst.CodeOf(err) != burst.CodeTeardownFailed {
		t.Fatalf("%v", err)
	}
	if strings.Contains(err.Error(), "FAKE-PROVIDER-CRED") || !errors.Is(err, secretErr{}) {
		t.Fatalf("the error text must not echo the provider's message, the chain must keep it: %q", err)
	}
	es, _ := r.st.AuditEntries(context.Background(), 0, 1000)
	for _, e := range es {
		if strings.Contains(e.Detail, "FAKE-PROVIDER-CRED") {
			t.Fatalf("audit row echoes the provider error: %+v", e)
		}
	}
	for _, a := range r.alerts {
		if strings.Contains(a.Detail, "FAKE-PROVIDER-CRED") {
			t.Fatalf("alert echoes the provider error: %+v", a)
		}
	}
	if len(r.alerts) != 1 {
		t.Fatalf("alerts %v", r.alerts)
	}
}

func TestBurstTeardownDoesNotDeleteExcludedAddress(t *testing.T) {
	r := newRig(t)
	res := r.provision("job-1")
	r.exclude = []string{res.Resource.Addrs[0].String()}
	err := r.eng.Teardown(context.Background(), res.Token, "job done")
	if burst.CodeOf(err) != burst.CodeTeardownFailed || !r.cloud.Has(res.Resource.ID) {
		t.Fatalf("a machine on a deploy-host address must not be deleted: %v", err)
	}
}

func TestBurstRevokeFailureStillStopsTheMoney(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	res := r.provision("job-1")
	// node never registered in the node table AND revoker errors: use a revoker that fails once.
	r.addNode("node-9")
	if err := r.eng.NodeJoined(ctx, res.Token, "node-9"); err != nil {
		t.Fatal(err)
	}
	failing := &failOnce{rig: r}
	eng, err := burst.New(burst.Options{Store: r.st, Providers: []burst.Provider{r.cloud}, Coordinator: coord, Config: r.cfg, Now: r.clk.Now, Spend: r.eng2Spend(), Exclusion: r.eng2Exclusion(), Revoker: failing})
	if err != nil {
		t.Fatal(err)
	}
	if err = eng.Teardown(ctx, res.Token, "job done"); burst.CodeOf(err) != burst.CodeTeardownFailed {
		t.Fatalf("a failed revoke keeps the intent open: %v", err)
	}
	if len(r.cloud.All()) != 0 {
		t.Fatal("the machine must be deleted even when the revoke failed: the money stops")
	}
	if got := r.intent(res.Token); got.State == store.BurstTerminal {
		t.Fatalf("the intent must stay open until the node is revoked: %+v", got)
	}
	if err = eng.Teardown(ctx, res.Token, "retry"); err != nil {
		t.Fatalf("the retry revokes and ends the intent: %v", err)
	}
	if got := r.intent(res.Token); got.State != store.BurstTerminal || len(r.revoked) != 1 {
		t.Fatalf("%+v revoked %v", got, r.revoked)
	}
}

type failOnce struct {
	rig  *rig
	done bool
}

func (f *failOnce) Revoke(ctx context.Context, id string) error {
	if !f.done {
		f.done = true
		return errors.New("registry busy")
	}
	return revoker{f.rig}.Revoke(ctx, id)
}

// ghost accepts Teardown and keeps the machine: the API said yes, List says it is still there.
type ghost struct{ burst.Provider }

func (ghost) Teardown(context.Context, string) error { return nil }

// TestBurstTeardownTrustsListNotTheDeleteResponse: money is released only when List no longer returns the machine.
func TestBurstTeardownTrustsListNotTheDeleteResponse(t *testing.T) {
	r := newRig(t, func(r *rig) { r.wrap = func(p burst.Provider) burst.Provider { return ghost{p} } })
	ctx := context.Background()
	res := r.provision("job-1")
	err := r.eng.Teardown(ctx, res.Token, "job done")
	if burst.CodeOf(err) != burst.CodeTeardownFailed {
		t.Fatalf("a delete that left the machine listed is a failed teardown: %v", err)
	}
	if row := r.resourceRow(res.Token); row.ConfirmedTeardownAt != nil {
		t.Fatal("money released while List still returns the machine")
	}
	if in := r.intent(res.Token); in.State == store.BurstTerminal {
		t.Fatalf("%+v", in)
	}
	if len(r.alerts) != 1 {
		t.Fatalf("alerts %v", r.alerts)
	}
}

func TestBurstTeardownDeletesOnlyMachinesCarryingThisCoordinatorsLabels(t *testing.T) {
	r := newRig(t)
	res := r.provision("job-1")
	// a machine that carries the same intent label but another coordinator's id (a collision, or a hostile provider listing)
	other := r.cloud.AddForeign(burst.IntentLabels(otherCoord, res.Token), addr("192.0.2.99"))
	if err := r.eng.Teardown(context.Background(), res.Token, "job done"); err != nil {
		t.Fatal(err)
	}
	if !r.cloud.Has(other) || r.cloud.Has(res.Resource.ID) {
		t.Fatalf("own machine gone=%v, other coordinator's machine kept=%v", !r.cloud.Has(res.Resource.ID), r.cloud.Has(other))
	}
}

// superset lists every machine whatever the selector: a provider that breaks the List contract.
type superset struct{ burst.Provider }

func (s superset) List(ctx context.Context, _ map[string]string) ([]burst.Resource, error) {
	return s.Provider.List(ctx, nil)
}
