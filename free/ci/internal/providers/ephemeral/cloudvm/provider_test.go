package cloudvm_test

// Purpose: TestCloudVM*: the provider passes the burst conformance kit through the fake nself, and its
// own rules: Estimate from the declared rate, idempotent adoption, not-found teardown, list filtering,
// the user-data probe, the recipe checks and the error classes.
// Inputs:  the rig. Outputs: pass/fail. Constraints: no sleeps, no real cloud.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/burst"
	"github.com/nself-org/plugins/free/ci/internal/burst/conformance"
	"github.com/nself-org/plugins/free/ci/internal/providers/ephemeral/cloudvm"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
)

func TestCloudVMConformance(t *testing.T) {
	conformance.Run(t, func(t *testing.T) conformance.Subject {
		r := newRig(t)
		return conformance.Subject{Provider: r.prov,
			Foreign: func(labels map[string]string) string { return r.cloud.AddForeign(labels) },
			Outage:  func(on bool) { r.br.set(func(k *knobs) { k.outage = on }) }}
	})
}

func intentOf(token string) burst.Intent {
	return burst.Intent{Token: token, Spec: burst.Spec{Platform: "linux/amd64", TimeoutMs: 600_000}, Labels: burst.IntentLabels(coord, token)}
}

func TestCloudVMEstimateUsesTheDeclaredRate(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	e, err := r.prov.Estimate(ctx, burst.Spec{Platform: "linux/amd64", TimeoutMs: 600_000})
	if err != nil || e.Class != "ephemeral" || e.LatencyMs != cloudvm.BootLatencyMs {
		t.Fatalf("estimate %+v %v", e, err)
	}
	if r.prov.Class() != burst.ClassCloud || r.prov.ID() != providerID {
		t.Fatalf("class %q id %q", r.prov.Class(), r.prov.ID())
	}
	// no declared rate, or an unbounded timeout, is an error (the engine reads it as unknown latency)
	r.pol.Rates = nil
	if _, err = r.prov.Estimate(ctx, burst.Spec{TimeoutMs: 600_000}); cloudvm.ReasonOf(err) != cloudvm.ReasonEstimate {
		t.Fatalf("no rate: %v", err)
	}
	if n := len(r.br.recorded()); n != 0 {
		t.Fatalf("Estimate is local arithmetic: it ran %d children", n)
	}
}

func TestCloudVMProvisionAdoptsTheMachineOfTheIntent(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	a, err := r.prov.Provision(ctx, intentOf("01J0000000000000000000ADOPT"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := r.prov.Provision(ctx, intentOf("01J0000000000000000000ADOPT"))
	if err != nil || a.ID != b.ID {
		t.Fatalf("second provision: %v %v %v", a.ID, b.ID, err)
	}
	if len(r.cloud.All()) != 1 || len(r.br.callsOf("provision")) != 2 { // the help probe + one create
		t.Fatalf("servers %d, provision children %d", len(r.cloud.All()), len(r.br.callsOf("provision")))
	}
	var names []string
	for _, c := range r.br.callsOf("provision") {
		names = append(names, flagValues(c.Argv, "--name")...)
	}
	if len(names) != 1 || names[0] != "nselfci-"+strings.ToLower("01J0000000000000000000ADOPT") {
		t.Fatalf("server name argv %v", names)
	}
}

func TestCloudVMTeardownTreatsNotFoundAsSuccess(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	res, err := r.prov.Provision(ctx, intentOf("01J0000000000000000000GONE1"))
	if err != nil {
		t.Fatal(err)
	}
	if err = r.prov.Teardown(ctx, res.ID); err != nil {
		t.Fatal(err)
	}
	if err = r.prov.Teardown(ctx, res.ID); err != nil { // E745 from the CLI
		t.Fatalf("second teardown: %v", err)
	}
	if err = r.prov.Teardown(ctx, "not-a-number"); err != nil {
		t.Fatal(err)
	}
	args := r.br.callsOf("destroy")[0].Argv
	for _, want := range []string{"--force-no-backup", "--release-ip", "--json", "--id"} {
		if !has(args, want) {
			t.Errorf("destroy argv lacks %s: %v", want, args)
		}
	}
}

func has(argv []string, s string) bool {
	for _, a := range argv {
		if a == s {
			return true
		}
	}
	return false
}

func TestCloudVMListFiltersLocallyAndNeverListsAProject(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	if _, err := r.prov.Provision(ctx, intentOf("01J0000000000000000000LIST1")); err != nil {
		t.Fatal(err)
	}
	r.cloud.AddForeign(map[string]string{"role": "ci-ephemeral", "coordinator_id": "someone-else"})
	r.br.set(func(k *knobs) { k.ignoreSelector = true }) // a CLI that ignored the selector
	got, err := r.prov.List(ctx, burst.OwnLabels(coord))
	if err != nil || len(got) != 1 {
		t.Fatalf("list %+v %v", got, err)
	}
	if _, err = r.prov.List(ctx, nil); cloudvm.ReasonOf(err) != cloudvm.ReasonRecipe {
		t.Fatalf("an empty selector must be refused: %v", err)
	}
	if _, err = r.prov.List(ctx, map[string]string{"role": "a,b=c"}); cloudvm.ReasonOf(err) != cloudvm.ReasonRecipe {
		t.Fatalf("a selector with operators must be refused: %v", err)
	}
}

func TestCloudVMRefusesWithoutTheUserDataFlag(t *testing.T) {
	r := newRig(t)
	r.br.set(func(k *knobs) { k.noUserDataFlag = true })
	_, err := r.prov.Provision(context.Background(), intentOf("01J0000000000000000000NOUD1"))
	if cloudvm.ReasonOf(err) != cloudvm.ReasonNoUserData || cloudvm.CodeOf(err) != "E698" {
		t.Fatalf("want E698 no_user_data, got %v", err)
	}
	if len(r.cloud.All()) != 0 || len(r.br.callsOf("provision")) != 1 {
		t.Fatalf("nothing may be created: %d servers, %d provision children", len(r.cloud.All()), len(r.br.callsOf("provision")))
	}
}

func TestCloudVMRecipeErrors(t *testing.T) {
	cases := []struct {
		name   string
		tweak  func(*rig)
		reason string
	}{
		{"no type", func(r *rig) { r.pol.Burst.CloudVM.Type = "" }, cloudvm.ReasonRecipe},
		{"no enroll url", func(r *rig) { r.pol.Burst.CloudVM.EnrollURL = "" }, cloudvm.ReasonRecipe},
		{"no token file", func(r *rig) { r.pol.Burst.CloudVM.TokenFile = "" }, cloudvm.ReasonRecipe},
		{"no agent for the arch", func(r *rig) { r.pol.Burst.CloudVM.Agent = map[string]schedpolicy.AgentBinary{} }, cloudvm.ReasonAgent},
		{"arm type, amd64 agent only", func(r *rig) { r.pol.Burst.CloudVM.Type = "cax11" }, cloudvm.ReasonPlatform},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t, c.tweak)
			_, err := r.prov.Provision(context.Background(), intentOf("01J0000000000000000000RCP01"))
			if cloudvm.ReasonOf(err) != c.reason || cloudvm.CodeOf(err) != "E698" {
				t.Fatalf("want E698 %s, got %v", c.reason, err)
			}
			if len(r.cloud.All()) != 0 {
				t.Fatal("a refused recipe created a server")
			}
		})
	}
}

func TestCloudVMClassifiesInfraFailures(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	for _, c := range []struct {
		cause string
		want  error
	}{{"HTTP 429", burst.ErrQuota}, {"network error", burst.ErrUnavailable}, {"HTTP 503", burst.ErrUnavailable}} {
		r.br.set(func(k *knobs) { k.failCause = c.cause })
		_, err := r.prov.List(ctx, burst.OwnLabels(coord))
		if !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", c.cause, err, c.want)
		}
	}
	r.br.set(func(k *knobs) { k.failCause = "HTTP 403" })
	if _, err := r.prov.List(ctx, burst.OwnLabels(coord)); cloudvm.ReasonOf(err) != cloudvm.ReasonTokenRefused {
		t.Errorf("a rejected token must be named: %v", err)
	}
}

func TestCloudVMDeletesAMachineItCannotUse(t *testing.T) {
	for name, knob := range map[string]func(*knobs){
		"answered without user data": func(k *knobs) { k.dropUserData = true },
		"answered without labels":    func(k *knobs) { k.stripLabels = true },
	} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t)
			r.br.set(knob)
			_, err := r.prov.Provision(context.Background(), intentOf("01J0000000000000000000BAD01"))
			if cloudvm.ReasonOf(err) != cloudvm.ReasonAnswer {
				t.Fatalf("want an unusable answer, got %v", err)
			}
			if len(r.cloud.All()) != 0 {
				t.Fatalf("the unusable machine was left running: %+v", r.cloud.All())
			}
		})
	}
}

func TestCloudVMQuoteUsesHourlyIncrementMinimumAndExtras(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	// 5.00 per hour, billed by the hour, 0.10 extras: ten minutes cost one whole hour
	q, err := r.prov.Quote(ctx, burst.Spec{TimeoutMs: 600_000})
	if err != nil || q.Reason != "" || q.Micro != 5_100_000 || q.BilledMs != 3_600_000 {
		t.Fatalf("quote %+v %v", q, err)
	}
	// 61 minutes bill two hours
	if q, _ = r.prov.Quote(ctx, burst.Spec{TimeoutMs: 61 * 60_000}); q.Micro != 10_100_000 {
		t.Fatalf("61 minutes: %+v", q)
	}
	// a minimum charge above the usage wins over the increment arithmetic's floor
	rate := r.pol.Rates[providerID]
	rate.MinimumChargeMicro = 20_000_000
	r.pol.Rates[providerID] = rate
	if q, _ = r.prov.Quote(ctx, burst.Spec{TimeoutMs: 600_000}); q.Micro != 25_100_000 {
		t.Fatalf("with a minimum: %+v", q)
	}
	// no timeout is unbounded and has no figure
	if q, _ = r.prov.Quote(ctx, burst.Spec{}); q.Reason == "" {
		t.Fatalf("an unbounded job was priced: %+v", q)
	}
}

func TestCloudVMUserDataFileIsPrivateAndGoneAfterTheCall(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	if _, err := r.prov.Provision(ctx, intentOf("01J0000000000000000000UDF01")); err != nil {
		t.Fatal(err)
	}
	r.br.set(func(k *knobs) { k.dieOnProvision = true })
	if _, err := r.prov.Provision(ctx, intentOf("01J0000000000000000000UDF02")); err == nil {
		t.Fatal("a lost response must fail")
	}
	for _, c := range r.br.callsOf("provision") {
		if has(c.Argv, "--user-data-file") && c.UDMode != 0o600 {
			t.Errorf("the user-data file was %v while the child read it", c.UDMode)
		}
	}
	dir := filepath.Join(r.home, "burst-userdata")
	left, err := os.ReadDir(dir)
	if err != nil || len(left) != 0 {
		t.Fatalf("user-data files left behind: %v %v", left, err)
	}
	if st, _ := os.Stat(dir); st.Mode().Perm() != 0o700 {
		t.Fatalf("the user-data directory is %v", st.Mode().Perm())
	}
}

func TestCloudVMPairingCodeNeverOnArgvEnvOrErrors(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	r.br.set(func(k *knobs) { k.dieOnProvision = true })
	_, err := r.prov.Provision(ctx, intentOf("01J0000000000000000000CODE1"))
	if err == nil {
		t.Fatal("a lost response must fail")
	}
	code, _ := r.userDataOf("01J0000000000000000000CODE1")
	for _, c := range r.br.recorded() {
		for _, s := range append(append([]string(nil), c.Argv...), c.Env...) {
			if strings.Contains(s, code) {
				t.Fatalf("the pairing code is on a command line or in an environment: %v", c.Argv)
			}
		}
	}
	if strings.Contains(err.Error(), code) {
		t.Fatalf("the error repeats the pairing code: %v", err)
	}
}
