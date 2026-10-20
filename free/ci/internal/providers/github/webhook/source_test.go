package webhook

// Purpose: tests of authentication and identity (acceptance lines 1 and 2 of P7-HOST-05).
// Inputs:  the recorded push and fork pull request payloads, signed with synthetic secrets.
// Outputs: pass or fail.
// Constraints: each test's doc comment cites the acceptance line it proves.

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/providers"
)

// wantE706 fails unless err is a plain (non-decline) E706 signature error carrying status 403.
func wantE706(t *testing.T, err error, what string) {
	t.Helper()
	var pe *providers.Error
	var d *Decline
	if !errors.As(err, &pe) || errors.As(err, &d) || pe.Code != "E706" || pe.Reason != "signature" || pe.Status != http.StatusForbidden {
		t.Fatalf("%s: want E706 signature (403), got %v", what, err)
	}
}

// TestVerifyAuthFacts proves acceptance line 1: a delivery signed with the repository's own secret
// yields auth per-repo; one signed only with GITHUB_WEBHOOK_SECRET yields auth shared for a repository
// without its own secret; --insecure alone yields auth none; a bad, missing or unconfigured signature
// is E706 signature (the ingress adapter and coordinator turn that into 403 with no enqueue).
func TestVerifyAuthFacts(t *testing.T) {
	push := fixture(t, "push.json")
	own := map[string]RepoSecret{"o/r": {Env: "SYNTH_REPO_SECRET"}}
	t.Run("per-repo secret", func(t *testing.T) {
		r := newRig(t, own)
		r.env["SYNTH_REPO_SECRET"] = repoSecret
		f, err := r.deliver("push", repoSecret, push)
		if err != nil || f.Auth != "per-repo" || f.Repo != "o/r" || f.RepoID != "1001" || f.Source != "github" {
			t.Fatalf("per-repo = %+v, %v", f, err)
		}
	})
	t.Run("shared secret for a repository without its own", func(t *testing.T) {
		r := newRig(t, map[string]RepoSecret{"o/other": {Env: "SYNTH_OTHER"}})
		r.env["SYNTH_OTHER"] = "synthetic-other-secret"
		r.env[SharedSecretEnv] = sharedSecret
		f, err := r.deliver("push", sharedSecret, push)
		if err != nil || f.Auth != "shared" {
			t.Fatalf("shared = %+v, %v", f, err)
		}
	})
	t.Run("insecure passthrough is auth none", func(t *testing.T) {
		r := newRig(t, nil)
		r.src.Insecure = func() bool { return true }
		f, err := r.deliver("push", "", push)
		if err != nil || f.Auth != "none" {
			t.Fatalf("insecure = %+v, %v", f, err)
		}
	})
	t.Run("insecure does not excuse a wrong shared signature", func(t *testing.T) {
		r := newRig(t, nil)
		r.src.Insecure = func() bool { return true }
		r.env[SharedSecretEnv] = sharedSecret
		_, err := r.deliver("push", "synthetic-wrong-secret", push)
		wantE706(t, err, "wrong shared signature under --insecure")
	})
	for name, mutate := range map[string]func(*rig) string{
		"missing signature":  func(r *rig) string { r.env["SYNTH_REPO_SECRET"] = repoSecret; return "" },
		"bad signature":      func(r *rig) string { r.env["SYNTH_REPO_SECRET"] = repoSecret; return "synthetic-wrong-secret" },
		"secret env unset":   func(r *rig) string { return repoSecret },
		"nothing configured": nil,
	} {
		t.Run(name, func(t *testing.T) {
			secrets := own
			if mutate == nil {
				secrets = nil
			}
			r := newRig(t, secrets)
			secret := repoSecret
			if mutate != nil {
				secret = mutate(r)
			}
			_, err := r.deliver("push", secret, push)
			wantE706(t, err, name)
		})
	}
	t.Run("unreadable policy fails closed", func(t *testing.T) {
		r := newRig(t, nil)
		r.src.Secrets = func(context.Context) (map[string]RepoSecret, error) { return nil, errors.New("boom") }
		r.src.Insecure = func() bool { return true }
		_, err := r.deliver("push", "", push)
		wantE706(t, err, "unreadable policy")
	})
	t.Run("an empty signature never verifies", func(t *testing.T) {
		if checkSignature("", sign("", push), push) == nil {
			t.Fatal("an empty secret must never verify")
		}
	})
}

// TestNoSharedFallback proves acceptance line 1 (review F2): a repository that has its own secret is
// never verified with the shared secret, even when the shared secret is configured and correct.
func TestNoSharedFallback(t *testing.T) {
	push := fixture(t, "push.json")
	r := newRig(t, map[string]RepoSecret{"o/r": {Env: "SYNTH_REPO_SECRET"}})
	r.env["SYNTH_REPO_SECRET"] = repoSecret
	r.env[SharedSecretEnv] = sharedSecret
	_, err := r.deliver("push", sharedSecret, push)
	wantE706(t, err, "shared-signed delivery for a repository with its own secret")
	if f, err := r.deliver("push", repoSecret, push); err != nil || f.Auth != "per-repo" {
		t.Fatalf("own secret with the shared secret also configured = %+v, %v", f, err)
	}
	t.Run("names are case-insensitive", func(t *testing.T) {
		upper := replace(push, `"full_name":"o/r"`, `"full_name":"O/R"`)
		_, err := r.deliver("push", sharedSecret, upper)
		wantE706(t, err, "case-varied name must not slip past the per-repo secret")
	})
}

// TestBaseRepoIdentity proves acceptance line 2 (review F2): a fork pull request is looked up by the
// BASE repository (name and id), the head repository is only a fact, and a delivery whose
// repository.id differs from the configured id for that name is refused.
func TestBaseRepoIdentity(t *testing.T) {
	pr := fixture(t, "pr_fork_opened.json")
	t.Run("fork PR is identified by the base repository", func(t *testing.T) {
		r := newRig(t, map[string]RepoSecret{"o/r": {Env: "SYNTH_REPO_SECRET", ID: 1001}})
		r.env["SYNTH_REPO_SECRET"] = repoSecret
		f, err := r.deliver("pull_request", repoSecret, pr)
		if err != nil || f.Repo != "o/r" || f.RepoID != "1001" || f.Auth != "per-repo" {
			t.Fatalf("fork PR = %+v, %v", f, err)
		}
		if f.HeadRepo.Value == nil || *f.HeadRepo.Value != "newbie/r" || f.HeadRepo.Source != "payload" || f.Fork.Value == nil || !*f.Fork.Value {
			t.Fatalf("the head repository is a payload fact only: head=%+v fork=%+v", f.HeadRepo, f.Fork)
		}
	})
	t.Run("a different configured id is refused", func(t *testing.T) {
		r := newRig(t, map[string]RepoSecret{"o/r": {Env: "SYNTH_REPO_SECRET", ID: 999}})
		r.env["SYNTH_REPO_SECRET"] = repoSecret
		_, err := r.deliver("pull_request", repoSecret, pr)
		wantE706(t, err, "repository.id differs from the configured id")
	})
	t.Run("a recreated repository cannot reuse the pinned name", func(t *testing.T) {
		r := newRig(t, map[string]RepoSecret{"o/r": {Env: "SYNTH_REPO_SECRET"}})
		r.env["SYNTH_REPO_SECRET"] = repoSecret
		if _, err := r.deliver("pull_request", repoSecret, pr); err != nil {
			t.Fatal(err)
		}
		recreated := replace(pr, `"repository":{"id":1001`, `"repository":{"id":5005`)
		_, err := r.deliver("pull_request", repoSecret, recreated)
		wantE706(t, err, "same name, new id")
	})
	t.Run("no id or an invalid name never holds", func(t *testing.T) {
		r := newRig(t, nil)
		r.env[SharedSecretEnv] = sharedSecret
		noID := replace(pr, `"repository":{"id":1001,`, `"repository":{`)
		_, err := r.deliver("pull_request", sharedSecret, noID)
		wantE706(t, err, "missing repository.id")
		bad := replace(pr, `"full_name":"o/r","private":false,"visibility"`, `"full_name":"o/../r","private":false,"visibility"`)
		_, err = r.deliver("pull_request", sharedSecret, bad)
		wantE706(t, err, "invalid repository name")
	})
}

// TestSecretsKey proves the key registration (Registers key providers.github.webhook_secrets): the
// policy key exists, accepts a name or {env,id}, and refuses secrets that look like values.
func TestSecretsKey(t *testing.T) {
	if errs := RegistrationErrors(); len(errs) != 0 {
		t.Fatalf("init registration errors: %v", errs)
	}
	if _, ok := policy.Default.Find(KeyWebhookSecrets); !ok {
		t.Fatal("providers.github.webhook_secrets is not registered")
	}
	if p, ok := providers.Lookup("github"); !ok {
		t.Fatal("provider github missing")
	} else if _, ok = p.(providers.TrustFacts); !ok {
		t.Fatal("provider github must implement TrustFacts for RefetchFacts")
	}
	found := false
	for _, s := range providers.Sources() {
		found = found || s.Name() == "github"
	}
	if !found {
		t.Fatal("trigger source github is not registered")
	}
	good, err := decodeSecrets([]byte(`{"O/R":"MY_SECRET","o/x":{"env":"OTHER","id":12}}`))
	m, _ := good.(map[string]RepoSecret)
	if err != nil || m["o/r"].Env != "MY_SECRET" || m["o/x"].ID != 12 {
		t.Fatalf("decode = %v, %v", good, err)
	}
	for _, bad := range []string{`[]`, `null`, `{"o/r":"has space"}`, `{"o/r":"whsec_abc.def"}`, `{"o/r":{"env":"X","extra":1}}`, `{"../r":"X"}`, `{"o/r":"A","O/R":"B"}`, `{"o/r":5}`, `{"o/r":"X"} {}`} {
		if _, err = decodeSecrets([]byte(bad)); err == nil || !strings.Contains(err.Error(), "E672") {
			t.Fatalf("%s must be refused with E672, got %v", bad, err)
		}
	}
	if New().HasSecrets() {
		t.Fatal("with no policy installed there are no per-repo secrets")
	}
	s := &Source{Secrets: func(context.Context) (map[string]RepoSecret, error) {
		return map[string]RepoSecret{"o/r": {Env: "X"}}, nil
	}}
	if !s.HasSecrets() {
		t.Fatal("HasSecrets must be true when a repository has its own secret")
	}
}
