package conformance_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/conformance"
	"github.com/nself-org/plugins/free/ci/internal/providers/fake"
)

// TestKitCatchesBroken proves each planted fake fails its own conformance check.
func TestKitCatchesBroken(t *testing.T) {
	secret := []byte("kit-secret-123")
	variants := []struct {
		name, expected string
		change         func(*fake.Options)
	}{
		{"logged-token", "secret in logs", func(o *fake.Options) { o.LogToken = true }},
		{"cancel-twice", "cancel idempotence", func(o *fake.Options) { o.CancelTwiceFails = true }},
		{"foreign-evidence", "collect binding", func(o *fake.Options) { o.ForeignEvidence = true }},
		{"extra-header", "extra auth header X-Api-Key", func(o *fake.Options) { o.ExtraAuthHeader = true }},
	}
	for _, v := range variants {
		t.Run(v.name, func(t *testing.T) {
			o := fake.Options{Token: string(secret)}
			v.change(&o)
			f := fake.New(o)
			var logs bytes.Buffer
			old := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
			defer slog.SetDefault(old)
			issues := conformance.Check(f, [][]byte{secret}, &logs)
			if len(issues) != 1 || issues[0] != v.expected {
				t.Fatalf("want only %q, got %v", v.expected, issues)
			}
		})
	}
	conformance.Run(t, conformance.Subject{Name: "fake", New: func(_ string, _ conformance.Clock) providers.Provider {
		return fake.New(fake.Options{Token: string(secret), States: []providers.LiveState{providers.Queued, providers.Running, providers.Finished, providers.Vanished}})
	}, Secrets: [][]byte{secret}})
}

func TestReplayAndRecord(t *testing.T) {
	static, err := conformance.NewReplay(filepath.Join("testdata", "replay"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.Get(static.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	if err := res.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 || len(static.Problems()) != 0 {
		t.Fatalf("static replay failed: %v", static.Problems())
	}
	static.Close()
	dir := t.TempDir()
	body := []byte("request")
	sum := sha256.Sum256(body)
	req := conformance.Request{Method: "POST", Path: "/dispatch", Query: "a=1", BodySHA256: hex.EncodeToString(sum[:]), HeadersWithoutAuth: http.Header{"Authorization": {"Bearer secret"}, "X-Api-Key": {"secret"}}}
	resp := conformance.Response{Status: 202, Headers: http.Header{"X-Token": {"secret"}}, Body: "accepted"}
	if err := conformance.Record(dir, "001", req, resp); err != nil {
		t.Fatal(err)
	}
	recorded, err := os.ReadFile(filepath.Join(dir, "001.json"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(recorded, []byte("secret")) || !bytes.Contains(recorded, []byte("REDACTED")) {
		t.Fatal("record leaked a credential")
	}
	r, err := conformance.NewReplay(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	httpReq, err := http.NewRequest("POST", r.URL+"/dispatch?a=1", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	got, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		t.Fatal(err)
	}
	if err := got.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if got.StatusCode != 202 || len(r.Requests()) != 1 || len(r.Problems()) != 0 {
		t.Fatalf("replay failed: status=%d problems=%v", got.StatusCode, r.Problems())
	}
}

func TestCheckRedactsProviderError(t *testing.T) {
	secret := []byte("reviewer-secret-token")
	p := &errorProvider{Fake: fake.New(fake.Options{Token: string(secret)}), err: fmt.Errorf("upstream: %s", secret)}
	issues := conformance.Check(p, [][]byte{secret}, nil)
	if !containsIssue(issues, "secret in provider error") || strings.Contains(strings.Join(issues, " "), string(secret)) {
		t.Fatalf("provider error leaked or was not detected: %v", issues)
	}
}

type errorProvider struct {
	*fake.Fake
	err error
}

func (p *errorProvider) Describe(context.Context, providers.Query) ([]model.Capability, error) {
	return nil, p.err
}

func TestRecordRedactsCredentialValues(t *testing.T) {
	dir := t.TempDir()
	secret := "reviewer-secret-token"
	req := conformance.Request{Method: "POST", Path: "/", HeadersWithoutAuth: http.Header{"X-Trace": {secret}}}
	resp := conformance.Response{Status: 200, Headers: http.Header{"X-Trace": {secret}}, Body: `{"trace":"reviewer-secret-token","nested":{"access_token":"short"}}`}
	if err := conformance.Record(dir, "001", req, resp); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "001.json"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(b, []byte(secret)) || bytes.Contains(b, []byte("short")) || !bytes.Contains(b, []byte("REDACTED")) {
		t.Fatalf("credential persisted: %s", b)
	}
}

func TestLivenessMapping(t *testing.T) {
	for _, states := range [][]providers.LiveState{{providers.Queued}, {providers.Queued, providers.Running, providers.Finished, providers.Vanished}} {
		issues := conformance.Check(fake.New(fake.Options{States: states}), nil, nil)
		if len(states) == 1 && !containsIssue(issues, "liveness mapping") {
			t.Fatalf("stuck provider accepted: %v", issues)
		}
		if len(states) == 4 && containsIssue(issues, "liveness mapping") {
			t.Fatalf("valid provider rejected: %v", issues)
		}
	}
}

func TestTriggerSignatureVerification(t *testing.T) {
	issues := conformance.Check(fake.New(fake.Options{}), nil, nil)
	if containsIssue(issues, "trigger verify") {
		t.Fatalf("signed trigger rejected: %v", issues)
	}
	body := []byte(`{"event":"push"}`)
	req, _ := http.NewRequest("POST", "http://example.invalid/trigger", bytes.NewReader(body))
	mac := hmac.New(sha256.New, []byte("fake-trigger-secret"))
	_, _ = mac.Write(body)
	req.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	if _, err := fake.New(fake.Options{}).Verify(context.Background(), req, body); err != nil {
		t.Fatal(err)
	}
	req.Header.Del("X-Hub-Signature-256")
	if _, err := fake.New(fake.Options{}).Verify(context.Background(), req, body); err == nil {
		t.Fatal("unsigned trigger accepted")
	}
}

func containsIssue(issues []string, want string) bool {
	for _, issue := range issues {
		if strings.Contains(issue, want) {
			return true
		}
	}
	return false
}

// TestFaultClassification proves generated HTTP and timeout cases map to contract classes.
func TestFaultClassification(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		status  int
		headers http.Header
		network error
		class   providers.ErrorClass
		reason  string
		retry   bool
	}{
		{401, nil, nil, providers.Auth, "", false}, {403, http.Header{"X-Ratelimit-Remaining": []string{"0"}}, nil, providers.Quota, "", false},
		{403, http.Header{"X-Accepted-Oauth-Scopes": []string{"admin:repo"}}, nil, providers.Auth, "admin:repo", false},
		{404, nil, nil, providers.Config, "", false}, {410, nil, nil, providers.Config, "", false}, {422, nil, nil, providers.Config, "", false},
		{429, http.Header{"Retry-After": []string{"12"}}, nil, providers.Quota, "", true}, {500, nil, nil, providers.Transient, "", false},
		{0, nil, context.DeadlineExceeded, providers.Transient, "", false},
	}
	for _, tc := range cases {
		e := conformance.ClassifyFault(tc.status, tc.headers, tc.network, now)
		if e.Class != tc.class || (!tc.retry && !e.RetryAt.IsZero()) || (tc.retry && !e.RetryAt.Equal(now.Add(12*time.Second))) || !strings.Contains(e.Reason, tc.reason) {
			t.Fatalf("status %d: %+v", tc.status, e)
		}
	}
}
