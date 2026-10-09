package conformance_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
		return fake.New(fake.Options{Token: string(secret), States: []providers.LiveState{providers.Queued, providers.Running, providers.Finished}})
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
