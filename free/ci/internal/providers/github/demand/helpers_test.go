package demand_test

// Purpose: shared fixtures for the demand tests: a real store behind the runnerjob ledger, a Handler with
// fixed settings, a webhook Source with no network, and builders for signed workflow_job deliveries.
// Inputs:  synthetic values only (repository acme/widgets, a secret that exists nowhere).
// Outputs: helpers; no test lives here.
// Constraints: no real network and no gh login: the webhook Source gets a NewAPI that always fails, so
//          its stale check reads nothing.

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/demand"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/webhook"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
	"github.com/nself-org/plugins/free/ci/internal/providers/runnerjob"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

const (
	testRepo   = "acme/widgets"
	testSecret = "synthetic-shared-secret"
	testImage  = "ghcr.io/actions/actions-runner@sha256:4ffadc0002b2581327e06101fc8c06cd189232baf79fe561fac9caeb76f5e807"
	sha        = "cccccccccccccccccccccccccccccccccccccccc"
)

var epoch = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// bind gives the test a fresh store and ledger and V15 mode, and returns the store.
func bind(t *testing.T) *store.Store {
	t.Helper()
	t.Setenv("NSELF_V15", "1")
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	runnerjob.Use(runnerjob.NewLedger(st, func() time.Time { return epoch }))
	t.Cleanup(func() { runnerjob.Use(nil); _ = st.Close() })
	return st
}

// handler is a Handler with the default settings (label nself, cap 2) and the fixed test image.
func handler(maxPending int) *demand.Handler {
	return &demand.Handler{
		Settings: func(context.Context) (demand.Settings, error) {
			return demand.Settings{Label: "nself", MaxPending: maxPending}, nil
		},
		Image: func() (string, error) { return testImage, nil },
	}
}

// source is the real webhook Source, wired to a shared secret and no API.
func hookSource() *webhook.Source {
	return &webhook.Source{
		Now:      func() time.Time { return epoch },
		Getenv:   func(k string) string { return map[string]string{webhook.SharedSecretEnv: testSecret}[k] },
		Secrets:  func(context.Context) (map[string]webhook.RepoSecret, error) { return nil, nil },
		Insecure: func() bool { return false },
		NewAPI:   func(context.Context) (*rest.Client, error) { return nil, errors.New("no network in tests") },
	}
}

// jobPayload is a workflow_job delivery body.
func jobPayload(action string, jobID int, labels ...string) []byte {
	quoted := make([]string, len(labels))
	for i, l := range labels {
		quoted[i] = fmt.Sprintf("%q", l)
	}
	return []byte(fmt.Sprintf(`{"action":%q,"workflow_job":{"id":%d,"run_id":3001,"status":"queued","head_sha":%q,"head_branch":"fix","name":"nself-ci","labels":[%s]},`+
		`"repository":{"id":1001,"full_name":%q,"private":false,"visibility":"public","fork":false,"default_branch":"main"},"sender":{"login":"newbie","type":"User"}}`,
		action, jobID, sha, strings.Join(quoted, ","), testRepo))
}

// verified runs a signed delivery through the webhook Source.
func verified(t *testing.T, body []byte) (model.TriggerFacts, error) {
	t.Helper()
	mac := hmac.New(sha256.New, []byte(testSecret))
	mac.Write(body)
	r := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(body))
	r.Header.Set("X-GitHub-Event", "workflow_job")
	r.Header.Set("X-GitHub-Delivery", "d-1")
	r.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	return hookSource().Verify(context.Background(), r, body)
}

// demandFor is the facts of a queued job with the given labels, as the webhook Source builds them.
func demandFor(t *testing.T, jobID int, labels ...string) model.TriggerFacts {
	t.Helper()
	f, err := verified(t, jobPayload("queued", jobID, labels...))
	if err != nil {
		t.Fatal(err)
	}
	if f.Kind != "runner_demand" {
		t.Fatalf("kind %q", f.Kind)
	}
	return f
}

type demandSettings = demand.Settings

func itoa(n int) string { return fmt.Sprint(n) }
