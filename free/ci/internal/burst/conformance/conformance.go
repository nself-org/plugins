// Package conformance is the lifecycle kit every burst.Provider must pass
// (Epic D13, R-DX-20). Run drives a provider through the guarantees the
// lifecycle builds on: Provision idempotent by the intent label, Teardown
// idempotent with not-found as success, List filtering by labels, labels within
// 63 characters, a real CreatedAt, and a classified error on an outage.
//
// Purpose: one test kit for the fake cloud and for every later provider.
// Inputs:  a Factory that builds a fresh Subject per case.
// Outputs: subtests under the caller's test; any failure names the rule.
// Constraints: no real cloud (a provider under test is a fake or a recorded
// double), no sleeps, no fixed ports; the kit never reads a clock of its own.
package conformance

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/burst"
)

// Subject is one provider under test with the hooks the kit needs.
type Subject struct {
	Provider burst.Provider
	// Foreign puts a machine into the provider that Provision did not create and
	// returns its id (labels may be nil: an unlabelled machine).
	Foreign func(labels map[string]string) string
	// Outage switches the provider API off and on (required: the kit proves the error class).
	Outage func(on bool)
}

// Factory builds a fresh Subject for one case.
type Factory func(t *testing.T) Subject

const (
	coordA = "01J00000000000000000COORDA"
	coordB = "01J00000000000000000COORDB"
)

func intent(coord, token string) burst.Intent {
	return burst.Intent{Token: token, Spec: burst.Spec{Platform: "linux/amd64", CPU: 2, MemMB: 4096, TimeoutMs: 600000}, Labels: burst.IntentLabels(coord, token)}
}

// tb is the part of testing.TB the cases use, so Violations can run them against a recorder.
type tb interface {
	Helper()
	Fatal(args ...any)
	Fatalf(format string, args ...any)
	Error(args ...any)
	Errorf(format string, args ...any)
}

type kase struct {
	name string
	fn   func(tb, Subject)
}

func cases() []kase {
	return []kase{
		{"estimate", estimate}, {"provision_labels_and_created_at", provisionLabels}, {"provision_idempotent_by_intent", provisionIdempotent},
		{"distinct_intents_distinct_resources", distinctIntents}, {"teardown_idempotent_not_found_is_success", teardownIdempotent},
		{"list_filters_by_labels", listFilters}, {"labels_within_63_characters", labelLength}, {"outage_is_classified", outage},
		{"cancelled_context_creates_nothing", cancelled},
	}
}

// Run executes every case against providers built by f.
func Run(t *testing.T, f Factory) {
	t.Helper()
	for _, c := range cases() {
		t.Run(c.name, func(t *testing.T) { c.fn(t, f(t)) })
	}
}

type stop struct{}

type recorder struct{ fails []string }

func (r *recorder) Helper()                   {}
func (r *recorder) Error(a ...any)            { r.fails = append(r.fails, fmt.Sprint(a...)) }
func (r *recorder) Errorf(f string, a ...any) { r.fails = append(r.fails, fmt.Sprintf(f, a...)) }
func (r *recorder) Fatal(a ...any)            { r.Error(a...); panic(stop{}) }
func (r *recorder) Fatalf(f string, a ...any) { r.Errorf(f, a...); panic(stop{}) }

// Violations runs every case against providers built by f and returns the failure
// messages by case name instead of failing t: the way the kit itself is tested
// against a deliberately broken provider.
func Violations(t *testing.T, f Factory) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	for _, c := range cases() {
		r := &recorder{}
		func() {
			defer func() {
				if v := recover(); v != nil {
					if _, ok := v.(stop); !ok {
						panic(v)
					}
				}
			}()
			c.fn(r, f(t))
		}()
		if len(r.fails) > 0 {
			out[c.name] = r.fails
		}
	}
	return out
}

func prov(t tb, p burst.Provider, in burst.Intent) burst.Resource {
	t.Helper()
	r, err := p.Provision(context.Background(), in)
	if err != nil {
		t.Fatalf("provision %s: %v", in.Token, err)
	}
	return r
}

func list(t tb, p burst.Provider, labels map[string]string) []burst.Resource {
	t.Helper()
	rs, err := p.List(context.Background(), labels)
	if err != nil {
		t.Fatalf("list %v: %v", labels, err)
	}
	return rs
}

func estimate(t tb, s Subject) {
	e, err := s.Provider.Estimate(context.Background(), burst.Spec{Platform: "linux/amd64", CPU: 2, MemMB: 4096, TimeoutMs: 600000})
	if err != nil {
		t.Fatalf("estimate: %v", err)
	}
	if e.LatencyMs < 0 {
		t.Errorf("latency %d is negative; unknown is 0", e.LatencyMs)
	}
	if s.Provider.ID() == "" || s.Provider.Class() == "" {
		t.Errorf("a provider needs an id and a class: %q %q", s.Provider.ID(), s.Provider.Class())
	}
}

func provisionLabels(t tb, s Subject) {
	r := prov(t, s.Provider, intent(coordA, "01J0000000000000000000TOK1"))
	if r.ID == "" || r.CreatedAt.IsZero() {
		t.Fatalf("resource needs an id and the provider's CreatedAt: %+v", r)
	}
	for k, want := range burst.IntentLabels(coordA, "01J0000000000000000000TOK1") {
		if r.Labels[k] != want {
			t.Errorf("label %s = %q, want %q", k, r.Labels[k], want)
		}
	}
}

func provisionIdempotent(t tb, s Subject) {
	a := prov(t, s.Provider, intent(coordA, "01J0000000000000000000TOK2"))
	b := prov(t, s.Provider, intent(coordA, "01J0000000000000000000TOK2"))
	if a.ID != b.ID {
		t.Fatalf("a second Provision for the same intent created %s after %s: it must adopt", b.ID, a.ID)
	}
	got := list(t, s.Provider, map[string]string{burst.LabelIntent: "01J0000000000000000000TOK2"})
	if len(got) != 1 {
		t.Fatalf("List by intent returned %d resources, want 1", len(got))
	}
}

func distinctIntents(t tb, s Subject) {
	a := prov(t, s.Provider, intent(coordA, "01J0000000000000000000TOK3"))
	b := prov(t, s.Provider, intent(coordA, "01J0000000000000000000TOK4"))
	if a.ID == b.ID {
		t.Fatalf("two intents share resource %s", a.ID)
	}
}

func teardownIdempotent(t tb, s Subject) {
	ctx := context.Background()
	r := prov(t, s.Provider, intent(coordA, "01J0000000000000000000TOK5"))
	if err := s.Provider.Teardown(ctx, r.ID); err != nil {
		t.Fatalf("teardown: %v", err)
	}
	if got := list(t, s.Provider, map[string]string{burst.LabelIntent: "01J0000000000000000000TOK5"}); len(got) != 0 {
		t.Fatalf("List still returns %d after teardown", len(got))
	}
	if err := s.Provider.Teardown(ctx, r.ID); err != nil {
		t.Errorf("second teardown must succeed (not-found is success): %v", err)
	}
	if err := s.Provider.Teardown(ctx, "no-such-resource"); err != nil {
		t.Errorf("teardown of an unknown id must succeed: %v", err)
	}
}

func listFilters(t tb, s Subject) {
	a := prov(t, s.Provider, intent(coordA, "01J0000000000000000000TOK6"))
	b := prov(t, s.Provider, intent(coordB, "01J0000000000000000000TOK7"))
	if s.Foreign != nil {
		s.Foreign(nil)
		s.Foreign(map[string]string{"role": "something-else"})
	}
	mine := list(t, s.Provider, burst.OwnLabels(coordA))
	if len(mine) != 1 || mine[0].ID != a.ID {
		t.Fatalf("List(role+coordinator A) = %+v, want only %s (not %s)", mine, a.ID, b.ID)
	}
	if got := list(t, s.Provider, map[string]string{burst.LabelRole: "no-such-role"}); len(got) != 0 {
		t.Errorf("a selector nothing carries returned %d", len(got))
	}
}

func labelLength(t tb, s Subject) {
	in := intent(coordA, "01J0000000000000000000TOK8")
	for k, v := range in.Labels {
		if len(v) > burst.MaxLabelValue {
			t.Fatalf("kit bug: label %s is %d long", k, len(v))
		}
	}
	long := intent(coordA, strings.Repeat("x", burst.MaxLabelValue+1))
	if _, err := s.Provider.Provision(context.Background(), long); err == nil {
		t.Error("a label value over 63 characters must be refused")
	}
}

func outage(t tb, s Subject) {
	if s.Outage == nil {
		t.Fatal("the Subject must provide Outage: a provider that cannot simulate one cannot prove its error class")
	}
	ctx := context.Background()
	r := prov(t, s.Provider, intent(coordA, "01J0000000000000000000TOK9"))
	s.Outage(true)
	if err := s.Provider.Teardown(ctx, r.ID); !errors.Is(err, burst.ErrUnavailable) {
		t.Errorf("teardown during an outage = %v, want burst.ErrUnavailable", err)
	}
	if _, err := s.Provider.List(ctx, burst.OwnLabels(coordA)); !errors.Is(err, burst.ErrUnavailable) {
		t.Errorf("list during an outage = %v, want burst.ErrUnavailable", err)
	}
	if _, err := s.Provider.Provision(ctx, intent(coordA, "01J000000000000000000TOK10")); !errors.Is(err, burst.ErrUnavailable) {
		t.Errorf("provision during an outage = %v, want burst.ErrUnavailable", err)
	}
	s.Outage(false)
	if err := s.Provider.Teardown(ctx, r.ID); err != nil {
		t.Errorf("teardown after the outage: %v", err)
	}
}

func cancelled(t tb, s Subject) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Provider.Provision(ctx, intent(coordA, "01J000000000000000000TOK11")); err == nil {
		t.Error("Provision with a cancelled context must fail")
	}
	if got := list(t, s.Provider, burst.OwnLabels(coordA)); len(got) != 0 {
		t.Errorf("a cancelled Provision left %d resources", len(got))
	}
}
