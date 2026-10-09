package fake

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/providers"
)

type Clock interface{ Now() time.Time }
type Options struct {
	States                                                       []providers.LiveState
	Evidence                                                     func(providers.Attempt) []byte
	Facts                                                        map[string]model.TriggerFacts
	Clock                                                        Clock
	Demand                                                       providers.DemandHandler
	Token                                                        string
	LogToken, CancelTwiceFails, ForeignEvidence, ExtraAuthHeader bool
}
type Fake struct {
	mu       sync.Mutex
	opts     Options
	states   int
	cancels  map[string]bool
	runners  map[string]bool
	attempts map[string]providers.Attempt
	requests http.Header
}

func New(opts Options) *Fake {
	return &Fake{opts: opts, cancels: map[string]bool{}, runners: map[string]bool{}, attempts: map[string]providers.Attempt{}, requests: http.Header{}}
}
func (f *Fake) ID() string { return "fake" }
func (f *Fake) Describe(context.Context, providers.Query) ([]model.Capability, error) {
	return []model.Capability{}, nil
}
func (f *Fake) Dispatch(ctx context.Context, a providers.Attempt) (providers.Handle, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.opts.LogToken {
		slog.InfoContext(ctx, "fake dispatch", "token", f.opts.Token)
	}
	f.requests.Set("Authorization", "Bearer "+f.opts.Token)
	if f.opts.ExtraAuthHeader {
		f.requests.Set("X-Api-Key", f.opts.Token)
	}
	f.attempts[a.ID] = a
	return providers.Handle{Provider: f.ID(), RunID: a.ID, Nonce: "nonce-" + a.ID, Ref: a.Revision}, nil
}
func (f *Fake) Requests() []http.Header {
	f.mu.Lock()
	defer f.mu.Unlock()
	return []http.Header{f.requests.Clone()}
}
func (f *Fake) Liveness(context.Context, providers.Handle) (providers.LiveState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.opts.States) == 0 {
		f.opts.States = []providers.LiveState{providers.Queued, providers.Running, providers.Finished, providers.Vanished}
	}
	i := f.states
	if i >= len(f.opts.States) {
		i = len(f.opts.States) - 1
	}
	f.states++
	return f.opts.States[i], nil
}
func (f *Fake) Cancel(_ context.Context, h providers.Handle) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.cancels[h.RunID] && f.opts.CancelTwiceFails {
		return fmt.Errorf("already canceled")
	}
	f.cancels[h.RunID] = true
	return nil
}
func (f *Fake) Collect(_ context.Context, h providers.Handle) (providers.Collected, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.attempts[h.RunID]
	if !ok {
		return providers.Collected{}, fmt.Errorf("unknown attempt")
	}
	id := a.ID
	if f.opts.ForeignEvidence {
		id = "foreign"
	}
	var evidence []byte
	if f.opts.Evidence != nil {
		evidence = f.opts.Evidence(a)
	} else {
		evidence = []byte(`{"attempt":"` + id + `","revision":"` + a.Revision + `","nonce":"` + h.Nonce + `"}`)
	}
	return providers.Collected{Evidence: evidence, Attestation: model.ProviderAttestation{Provider: f.ID(), RunID: h.RunID, RunAttempt: a.N, Ref: a.Revision, Nonce: h.Nonce}}, nil
}
func (f *Fake) Profile() providers.LeaseProfile {
	return providers.LeaseProfile{TTL: 30 * time.Minute, Poll: 20 * time.Second}
}
func (f *Fake) Mint(_ context.Context, r providers.RunnerRequest) (providers.RunnerCredential, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.runners[r.Attempt.ID] = true
	now := time.Now()
	if f.opts.Clock != nil {
		now = f.opts.Clock.Now()
	}
	return providers.RunnerCredential{Kind: "fake", Value: "credential", ExpiresAt: now.Add(time.Hour)}, nil
}
func (f *Fake) Deregister(_ context.Context, r providers.RunnerRef) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.runners, r.ID)
	return nil
}
func (f *Fake) Orphans(context.Context, providers.RunnerScope, time.Time) ([]providers.RunnerRef, error) {
	return nil, nil
}
func (f *Fake) Restriction(context.Context, providers.RunnerScope, int64) (providers.Restriction, error) {
	return providers.Restriction{}, nil
}
func (f *Fake) Name() string             { return f.ID() }
func (f *Fake) Match(*http.Request) bool { return true }
func (f *Fake) Verify(_ context.Context, req *http.Request, body []byte) (model.TriggerFacts, error) {
	if req == nil {
		return model.TriggerFacts{}, fmt.Errorf("missing trigger request")
	}
	mac := hmac.New(sha256.New, []byte("fake-trigger-secret"))
	_, _ = mac.Write(body)
	want := mac.Sum(nil)
	header := req.Header.Get("X-Hub-Signature-256")
	if len(header) < 7 || header[:7] != "sha256=" {
		return model.TriggerFacts{}, fmt.Errorf("missing trigger signature")
	}
	got, err := hex.DecodeString(header[7:])
	if err != nil || !hmac.Equal(got, want) {
		return model.TriggerFacts{}, fmt.Errorf("invalid trigger signature")
	}
	return model.TriggerFacts{Schema: "ci.trigger-facts/v1", Source: f.ID(), Kind: "push", Auth: "per-repo"}, nil
}
func (f *Fake) SignedTriggerFixture() (*http.Request, []byte) {
	body := []byte(`{"event":"push"}`)
	req, _ := http.NewRequest("POST", "http://example.invalid/trigger", nil)
	mac := hmac.New(sha256.New, []byte("fake-trigger-secret"))
	_, _ = mac.Write(body)
	req.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	return req, body
}
func (f *Fake) Refetch(_ context.Context, facts model.TriggerFacts) (model.TriggerFacts, error) {
	if v, ok := f.opts.Facts[facts.Repo]; ok {
		return v, nil
	}
	return facts, nil
}
func (f *Fake) Usage(context.Context, providers.Period) ([]providers.UsageRow, error) {
	return []providers.UsageRow{}, nil
}
func (f *Fake) Health(context.Context) providers.Health { return providers.Health{State: "up"} }
func (f *Fake) Synthesize(ctx context.Context, facts model.TriggerFacts) (*model.Pipeline, error) {
	if f.opts.Demand == nil {
		return nil, nil
	}
	return f.opts.Demand.Synthesize(ctx, facts)
}

var _ providers.Provider = (*Fake)(nil)
var _ providers.Dispatcher = (*Fake)(nil)
var _ providers.RunnerRegistrar = (*Fake)(nil)
var _ providers.TriggerSource = (*Fake)(nil)
var _ providers.TrustFacts = (*Fake)(nil)
var _ providers.UsageSource = (*Fake)(nil)
var _ providers.HealthProbe = (*Fake)(nil)
var _ providers.DemandHandler = (*Fake)(nil)
