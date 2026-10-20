package health_test

// Purpose: the D13 thresholds of the breaker under an injected clock (P7-HOST-11).
// Inputs:  scripted outcomes at offsets from a fixed start.
// Outputs: pass or fail.
// Constraints: no real time, no network.

import (
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/health"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
)

// step is one event: an outcome at an offset (out set) or a health read at an offset (out nil).
type step struct {
	at        time.Duration
	out       *rest.Outcome
	want      string // expected state after the step ("" for none)
	reason    string
	recovered bool
}

func oc(class providers.ErrorClass, status int) *rest.Outcome {
	o := outcome(class, status)
	return &o
}

// TestBreakerTable proves acceptance line 4 (Breaker transitions follow the D13 thresholds under an
// injected clock): five consecutive transient outcomes within two minutes, 5xx answers for sixty
// seconds, the stale run that does not count, the half-open probe gap that doubles to ten minutes,
// recovery on one answer, and a quota answer that never opens the breaker.
func TestBreakerTable(t *testing.T) {
	s := time.Second
	cases := []struct {
		name  string
		steps []step
	}{
		{"four transient outcomes stay up, the fifth opens", []step{
			{0, oc("transient", 0), "up", "", false}, {10 * s, oc("transient", 0), "up", "", false}, {20 * s, oc("outage", 0), "up", "", false},
			{30 * s, oc("transient", 0), "up", "", false}, {40 * s, oc("transient", 0), "down", health.ReasonDown, false},
		}},
		{"five outcomes spread over more than two minutes do not open", []step{
			{0, oc("transient", 0), "up", "", false}, {40 * s, oc("transient", 0), "up", "", false}, {80 * s, oc("transient", 0), "up", "", false},
			{120 * s, oc("transient", 0), "up", "", false}, {150 * s, oc("transient", 0), "up", "", false},
		}},
		{"an answer between failures restarts the run", []step{
			{0, oc("transient", 0), "up", "", false}, {1 * s, oc("transient", 0), "up", "", false}, {2 * s, oc("transient", 0), "up", "", false},
			{3 * s, oc("transient", 0), "up", "", false}, {4 * s, oc("", 200), "up", "", false},
			{5 * s, oc("transient", 0), "up", "", false}, {6 * s, oc("transient", 0), "up", "", false}, {7 * s, oc("transient", 0), "up", "", false},
			{8 * s, oc("transient", 0), "up", "", false},
		}},
		{"5xx answers for sixty seconds open it on the third", []step{
			{0, oc("transient", 503), "up", "", false}, {30 * s, oc("transient", 502), "up", "", false}, {60 * s, oc("transient", 500), "down", health.ReasonDown, false},
		}},
		{"timeouts alone never trip the sixty second rule", []step{
			{0, oc("transient", 0), "up", "", false}, {30 * s, oc("transient", 0), "up", "", false}, {61 * s, oc("transient", 0), "up", "", false},
		}},
		{"a stale 5xx does not make the next one a sixty second run", []step{
			{0, oc("transient", 503), "up", "", false}, {10 * time.Minute, oc("transient", 503), "up", "", false},
		}},
		{"the probe gap doubles from 30 s to the 10 min cap and one answer closes it", []step{
			{0, oc("outage", 0), "up", "", false}, {1 * s, oc("outage", 0), "up", "", false}, {2 * s, oc("outage", 0), "up", "", false},
			{3 * s, oc("outage", 0), "up", "", false}, {4 * s, oc("outage", 0), "down", health.ReasonDown, false}, // next probe at 34 s
			{20 * s, nil, "down", health.ReasonDown, false},
			{34 * s, nil, "degraded", health.ReasonProbing, false},           // the probe slot
			{35 * s, nil, "down", health.ReasonDown, false},                  // the slot is taken
			{36 * s, oc("transient", 0), "down", health.ReasonDown, false},   // probe failed: next at 96 s (gap 60 s)
			{95 * s, nil, "down", health.ReasonDown, false},                  //
			{96 * s, nil, "degraded", health.ReasonProbing, false},           //
			{97 * s, oc("transient", 503), "down", health.ReasonDown, false}, // gap 120 s: next at 217 s
			{216 * s, nil, "down", health.ReasonDown, false},                 //
			{217 * s, oc("transient", 0), "down", health.ReasonDown, false},  // gap 240 s: next at 457 s
			{457 * s, oc("transient", 0), "down", health.ReasonDown, false},  // gap 480 s: next at 937 s
			{937 * s, oc("transient", 0), "down", health.ReasonDown, false},  // gap 600 s (cap): next at 1537 s
			{1536 * s, nil, "down", health.ReasonDown, false},                //
			{1537 * s, oc("transient", 0), "down", health.ReasonDown, false}, // still 600 s: next at 2137 s
			{2136 * s, nil, "down", health.ReasonDown, false},                //
			{2137 * s, oc("", 200), "up", "", true},                          // one answer closes it
		}},
		{"a failure in flight before the probe gap does not double it", []step{
			{0, oc("outage", 0), "up", "", false}, {1 * s, oc("outage", 0), "up", "", false}, {2 * s, oc("outage", 0), "up", "", false},
			{3 * s, oc("outage", 0), "up", "", false}, {4 * s, oc("outage", 0), "down", health.ReasonDown, false},
			{10 * s, oc("transient", 0), "down", health.ReasonDown, false}, {20 * s, oc("transient", 0), "down", health.ReasonDown, false},
			{34 * s, nil, "degraded", health.ReasonProbing, false}, // still the first 30 s gap
		}},
		{"an auth refusal proves the provider answers and ends the outage", []step{
			{0, oc("outage", 0), "up", "", false}, {1 * s, oc("outage", 0), "up", "", false}, {2 * s, oc("outage", 0), "up", "", false},
			{3 * s, oc("outage", 0), "up", "", false}, {4 * s, oc("outage", 0), "down", health.ReasonDown, false},
			{40 * s, oc("auth", 401), "up", "", true},
		}},
		{"auth and config refusals alone never open it", []step{
			{0, oc("auth", 401), "up", "", false}, {1 * s, oc("config", 422), "up", "", false}, {2 * s, oc("auth", 403), "up", "", false},
			{3 * s, oc("auth", 401), "up", "", false}, {4 * s, oc("config", 404), "up", "", false}, {5 * s, oc("auth", 401), "up", "", false},
		}},
	}
	start := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := health.NewBreaker()
			for i, st := range tc.steps {
				now := start.Add(st.at)
				if st.out != nil {
					if rec, _ := b.Observe(*st.out, now); rec != st.recovered {
						t.Fatalf("step %d at %v: recovered = %v, want %v", i, st.at, rec, st.recovered)
					}
				}
				h := b.Health(now)
				if st.want != "" && (h.State != st.want || h.Reason != st.reason) {
					t.Fatalf("step %d at %v: health = %s/%s, want %s/%s", i, st.at, h.State, h.Reason, st.want, st.reason)
				}
			}
		})
	}
}

// TestBreakerProbeReportsRetryAt: while down the health names when the next probe is allowed.
func TestBreakerProbeReportsRetryAt(t *testing.T) {
	b := health.NewBreaker()
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	for i := 0; i < health.DownAfter; i++ {
		b.Observe(outcome("transient", 0), now)
	}
	h := b.Health(now)
	if h.State != "down" || !h.RetryAt.Equal(now.Add(health.ProbeAfter)) || !h.Since.Equal(now) {
		t.Fatalf("down health = %+v", h)
	}
}
