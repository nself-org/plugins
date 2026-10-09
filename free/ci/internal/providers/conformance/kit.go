package conformance

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/providers"
)

type Clock interface{ Now() time.Time }
type fixedClock struct{}

func (fixedClock) Now() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }

type Subject struct {
	Name     string
	New      func(baseURL string, clk Clock) providers.Provider
	Fixtures string
	Secrets  [][]byte
}

// Run checks the base contract and every extension implemented by the subject.
func Run(t *testing.T, s Subject) {
	t.Helper()
	r, err := NewReplay(s.Fixtures)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	var logs bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	defer slog.SetDefault(old)
	p := s.New(r.URL, fixedClock{})
	if p == nil {
		t.Fatal("nil provider")
	}
	for _, issue := range Check(p, s.Secrets, &logs) {
		t.Error(issue)
	}
	for _, issue := range r.Problems() {
		t.Error(issue)
	}
	for _, req := range r.Requests() {
		if issue := requestLeak(req.Header, s.Secrets); issue != "" {
			t.Error(issue)
		}
	}
}

// Check returns independent contract failures, allowing planted defects to be
// proved without making a parent test fail intentionally.
func Check(p providers.Provider, secrets [][]byte, logs *bytes.Buffer) []string {
	var issues []string
	ctx := context.Background()
	if p.ID() == "" {
		issues = append(issues, "provider id")
	}
	if _, err := p.Describe(ctx, providers.Query{}); err != nil {
		clean, leaked := redactKnownSecrets(err.Error(), secrets)
		issues = append(issues, "describe: "+redactCredential(clean))
		if leaked || clean != redactCredential(clean) {
			issues = append(issues, "secret in provider error")
		}
	}
	if d, ok := p.(providers.Dispatcher); ok {
		a := providers.Attempt{ID: "attempt-1", N: 2, Revision: "revision-1"}
		h, err := d.Dispatch(ctx, a)
		if err != nil {
			issues = append(issues, "dispatch: "+err.Error())
		} else {
			if d.Profile().TTL <= 0 || d.Profile().Poll <= 0 {
				issues = append(issues, "profile missing")
			}
			for _, want := range []providers.LiveState{providers.Queued, providers.Running, providers.Finished, providers.Vanished} {
				state, e := d.Liveness(ctx, h)
				if e != nil || state != want {
					issues = append(issues, "liveness mapping")
					break
				}
			}
			if e := d.Cancel(ctx, h); e != nil {
				issues = append(issues, "cancel failed")
			}
			if e := d.Cancel(ctx, h); e != nil {
				issues = append(issues, "cancel idempotence")
			}
			c, e := d.Collect(ctx, h)
			if e != nil {
				issues = append(issues, "collect failed")
			} else if !bindingOK(c, a, h) {
				issues = append(issues, "collect binding")
			}
		}
	}
	if r, ok := p.(providers.RunnerRegistrar); ok {
		credential, err := r.Mint(ctx, providers.RunnerRequest{Attempt: providers.Attempt{ID: "runner-1"}})
		if err != nil || credential.Kind == "" || credential.Value == "" {
			issues = append(issues, "mint invalid")
		}
		ref := providers.RunnerRef{ID: "runner-1"}
		if e := r.Deregister(ctx, ref); e != nil {
			issues = append(issues, "deregister failed")
		}
		if e := r.Deregister(ctx, ref); e != nil {
			issues = append(issues, "deregister idempotence")
		}
		if _, e := r.Orphans(ctx, providers.RunnerScope{}, time.Time{}); e != nil {
			issues = append(issues, "orphans failed")
		}
		if _, e := r.Restriction(ctx, providers.RunnerScope{}, 0); e != nil {
			issues = append(issues, "restriction failed")
		}
	}
	if source, ok := p.(providers.TriggerSource); ok {
		req, _ := http.NewRequest("POST", "http://example.invalid/trigger", nil)
		if source.Name() == "" || !source.Match(req) {
			issues = append(issues, "trigger match")
		}
		if _, err := source.Verify(ctx, req, nil); err == nil {
			issues = append(issues, "trigger verify: unsigned accepted")
		}
		fixture, ok := p.(interface {
			SignedTriggerFixture() (*http.Request, []byte)
		})
		if !ok {
			issues = append(issues, "trigger verify: signed fixture missing")
		} else {
			signed, body := fixture.SignedTriggerFixture()
			if signed == nil {
				issues = append(issues, "trigger verify: signed fixture missing")
			} else if _, err := source.Verify(ctx, signed, body); err != nil {
				issues = append(issues, "trigger verify: signed rejected")
			}
		}
	}
	if facts, ok := p.(providers.TrustFacts); ok {
		if _, err := facts.Refetch(ctx, model.TriggerFacts{Repo: "fixture"}); err != nil {
			issues = append(issues, "facts refetch")
		}
	}
	if usage, ok := p.(providers.UsageSource); ok {
		if _, err := usage.Usage(ctx, providers.Period{}); err != nil {
			issues = append(issues, "usage failed")
		}
	}
	if h, ok := p.(providers.HealthProbe); ok {
		if !oneHealth(h.Health(ctx).State) {
			issues = append(issues, "health invalid")
		}
	}
	if logs != nil {
		for _, secret := range secrets {
			if len(secret) > 0 && bytes.Contains(logs.Bytes(), secret) {
				issues = append(issues, "secret in logs")
			}
		}
	}
	if r, ok := p.(interface{ Requests() []http.Header }); ok {
		for _, headers := range r.Requests() {
			if issue := requestLeak(headers, secrets); issue != "" {
				issues = append(issues, issue)
			}
		}
	}
	return issues
}
func oneHealth(s string) bool { return s == "up" || s == "degraded" || s == "down" }
func bindingOK(c providers.Collected, a providers.Attempt, h providers.Handle) bool {
	if c.Attestation.RunID != h.RunID || c.Attestation.RunAttempt != a.N || c.Attestation.Ref != a.Revision || c.Attestation.Nonce != h.Nonce {
		return false
	}
	var evidence map[string]any
	if json.Unmarshal(c.Evidence, &evidence) != nil {
		return false
	}
	return evidence["attempt"] == a.ID && evidence["revision"] == a.Revision && evidence["nonce"] == h.Nonce
}
func requestLeak(header http.Header, secrets [][]byte) string {
	for key, values := range header {
		if strings.EqualFold(key, "Authorization") {
			continue
		}
		if sensitive(key) {
			return fmt.Sprintf("extra auth header %s", key)
		}
		for _, value := range values {
			for _, secret := range secrets {
				if len(secret) > 0 && strings.Contains(value, string(secret)) {
					return "secret in request"
				}
			}
		}
	}
	return ""
}
