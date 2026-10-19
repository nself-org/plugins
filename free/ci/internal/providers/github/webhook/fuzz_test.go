package webhook

// Purpose: fuzz targets FuzzGitHubWebhook (payload parse) and FuzzSignatureHeader (acceptance line 7).
// Inputs:  the recorded payloads and a few hostile headers as seeds, plus testdata/fuzz corpora.
// Outputs: no panic; every success validates; only the declared error types come back.
// Constraints: the fuzz runs offline (the API client always fails), so a payload can never reach the
//          network; run with `go test -fuzz FuzzGitHubWebhook -fuzztime 180s`.

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
)

// FuzzGitHubWebhook feeds arbitrary event names and bodies through Verify (auth none, no API). It
// proves acceptance line 7: no crash, and any accepted delivery validates as trigger facts for a
// well-formed repository and revision.
func FuzzGitHubWebhook(f *testing.F) {
	for event, name := range map[string]string{
		"push": "push.json", "pull_request": "pr_fork_opened.json", "create": "create_tag.json", "repository_dispatch": "repository_dispatch.json",
		"workflow_job": "workflow_job_queued.json", "workflow_dispatch": "workflow_dispatch.json",
	} {
		f.Add(event, fixture(f, name))
	}
	f.Add("push", fixture(f, "push_delete.json"))
	f.Add("push", fixture(f, "push_tag.json"))
	f.Add("ping", []byte(`{}`))
	f.Add("push", []byte(`{"repository":{"id":1,"full_name":"o/r"},"after":"`+strings.Repeat("a", 40)+`","ref":"refs/heads/x"}`))
	src := &Source{
		Now:      func() time.Time { return fixedNow },
		Getenv:   func(string) string { return "" },
		Secrets:  func(context.Context) (map[string]RepoSecret, error) { return nil, nil },
		Insecure: func() bool { return true },
		NewAPI:   func(context.Context) (*rest.Client, error) { return nil, errors.New("offline") },
	}
	f.Fuzz(func(t *testing.T, event string, body []byte) {
		facts, err := src.Verify(context.Background(), request(event, "d", "", body), body)
		if err != nil {
			var d *Decline
			var pe *providers.Error
			if !errors.As(err, &d) && !errors.As(err, &pe) {
				t.Fatalf("undeclared error type %T: %v", err, err)
			}
			return
		}
		if verr := facts.Validate(); verr != nil {
			t.Fatalf("accepted facts do not validate: %v", verr)
		}
		if !coordinator.ValidProject(facts.Repo) || !coordinator.ValidRevision(facts.Revision) || facts.Source != Name {
			t.Fatalf("accepted facts name a bad repository or revision: %+v", facts)
		}
	})
}

// FuzzSignatureHeader checks the verifier against an independent HMAC: it accepts a header exactly
// when the header is "sha256=" plus the hex HMAC-SHA256 of the body under a non-empty secret, and never
// panics on any header bytes.
func FuzzSignatureHeader(f *testing.F) {
	good := func(secret string, body []byte) string {
		m := hmac.New(sha256.New, []byte(secret))
		m.Write(body)
		return sigPrefix + hex.EncodeToString(m.Sum(nil))
	}
	f.Add("synthetic-secret", good("synthetic-secret", []byte("{}")), []byte("{}"))
	f.Add("synthetic-secret", "sha256="+strings.Repeat("0", 64), []byte("{}"))
	f.Add("synthetic-secret", "sha1=abc", []byte("{}"))
	f.Add("", good("", []byte("{}")), []byte("{}"))
	f.Add("synthetic-secret", strings.ToUpper(good("synthetic-secret", []byte("x"))), []byte("x"))
	f.Add("synthetic-secret", "sha256="+strings.Repeat("zz", 32), []byte(""))
	f.Fuzz(func(t *testing.T, secret, header string, body []byte) {
		err := checkSignature(secret, header, body)
		want := secret != "" && strings.HasPrefix(header, sigPrefix) && strings.EqualFold(header[len(sigPrefix):], good(secret, body)[len(sigPrefix):]) && len(header) == len(sigPrefix)+64
		if (err == nil) != want {
			t.Fatalf("checkSignature(%q, %q) = %v, want accepted=%v", secret, header, err, want)
		}
	})
}
