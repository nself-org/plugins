package runtime

// Purpose: tests of the providers.* keys and the credential rule (P7-HOST-16, acceptance line 5).
// Inputs:  the key specs, scopes built in the test, environment variables set with t.Setenv.
// Outputs: pass or fail.
// Constraints: every token value is synthetic; no network and no gh process (the rules under test
//          read the environment only).

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

func scope(kind trust.Source, kv map[string]string) trust.Scope {
	vals := map[string]json.RawMessage{}
	for k, v := range kv {
		vals[k] = json.RawMessage(v)
	}
	return trust.Scope{Kind: kind, Values: vals}
}

func merge(t *testing.T, scopes ...trust.Scope) (trust.Effective, error) {
	t.Helper()
	reg, err := trust.NewRegistry(Keys()...)
	if err != nil {
		t.Fatal(err)
	}
	eff, _, err := reg.Merge("", scopes...)
	return eff, err
}

// TestKeysOperatorOnly proves acceptance line 5: setting providers.github.api_url in a revision's
// .nself/ci.yaml changes nothing and is listed in Effective.Ignored (E671); an operator-scope value
// is used. The same holds for the protected branch, the pipeline and the job, and for all seven keys.
func TestKeysOperatorOnly(t *testing.T) {
	if errs := RegistrationErrors(); len(errs) != 0 {
		t.Fatal(errs)
	}
	for _, spec := range Keys() {
		got, ok := policy.Default.Find(spec.Key)
		if !ok || !got.Spend || got.Kind != policy.TightenFixed {
			t.Errorf("%s: registered=%v operator-only=%v kind=%v", spec.Key, ok, got.Spend, got.Kind)
		}
	}
	evil := `"https://attacker.example"`
	for _, kind := range []trust.Source{trust.SourceRevision, trust.SourceProtectedBranch, trust.SourcePipeline, trust.SourceJob} {
		eff, err := merge(t, scope(kind, map[string]string{KeyGitHubAPIURL: evil, KeyGitHubTokenEnv: `"STOLEN"`, KeyGitLabAllowPrivateBase: `true`}))
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		s, err := SettingsFrom(&eff)
		if err != nil || s != DefaultSettings() {
			t.Fatalf("%s scope changed the settings: %+v %v", kind, s, err)
		}
		ignored := map[string]bool{}
		for _, ig := range eff.Ignored() {
			if !strings.HasPrefix(ig.Reason, "E671") {
				t.Errorf("%s: %s ignored with %q, want E671", kind, ig.Key, ig.Reason)
			}
			ignored[ig.Key] = true
		}
		for _, key := range []string{KeyGitHubAPIURL, KeyGitHubTokenEnv, KeyGitLabAllowPrivateBase} {
			if !ignored[key] {
				t.Errorf("%s: %s missing from Effective.Ignored", kind, key)
			}
		}
	}
	eff, err := merge(t, scope(trust.SourceUser, map[string]string{
		KeyGitHubAPIURL: `"https://ghe.example/api/v3/"`, KeyGitHubTokenEnv: `"GHE_TOKEN"`, KeyGitHubAllowPrivateBase: `true`,
		KeyGitLabAPIURL: `"https://gitlab.lan/api/v4"`, KeyGitLabTokenEnv: `"GL_TOKEN"`, KeyGitLabRunnerTokenEnv: `"GL_RUNNER"`, KeyGitLabAllowPrivateBase: `true`}))
	if err != nil {
		t.Fatal(err)
	}
	s, err := SettingsFrom(&eff)
	want := Settings{GitHubAPIURL: "https://ghe.example/api/v3", GitHubTokenEnv: "GHE_TOKEN", GitHubAllowPrivateBase: true,
		GitLabAPIURL: "https://gitlab.lan/api/v4", GitLabTokenEnv: "GL_TOKEN", GitLabRunnerTokenEnv: "GL_RUNNER", GitLabAllowPrivateBase: true}
	if err != nil || s != want || len(eff.Ignored()) != 0 {
		t.Fatalf("operator values: %+v %v ignored=%v", s, err, eff.Ignored())
	}
	if _, err = SettingsFrom(nil); err == nil {
		t.Fatal("a nil policy is an error, never the defaults")
	}
}

func TestKeyDecodersRefuseBadShapes(t *testing.T) {
	bad := map[string]string{
		KeyGitHubAPIURL:           `"not a url"`,
		KeyGitLabAPIURL:           `"https://@gitlab.example/api/v4"`,
		KeyGitHubTokenEnv:         `"HAS SPACE"`,
		KeyGitLabTokenEnv:         `7`,
		KeyGitLabRunnerTokenEnv:   `["A"]`,
		KeyGitHubAllowPrivateBase: `"true"`,
		KeyGitLabAllowPrivateBase: `1`,
	}
	bad[KeyGitHubAPIURL] = `"http://ghe.example/api/v3"` // the REST funnel refuses plain http, so the policy does too
	for key, raw := range bad {
		if _, err := merge(t, scope(trust.SourceUser, map[string]string{key: raw})); err == nil || !strings.Contains(err.Error(), "E672") {
			t.Errorf("%s=%s must be refused with E672, got %v", key, raw, err)
		}
	}
	for key, raw := range map[string]string{KeyGitHubTokenEnv: `""`, KeyGitLabAPIURL: `"https://gitlab.example/api/v4?x=1"`} {
		_, err := merge(t, scope(trust.SourceUser, map[string]string{key: raw}))
		if (key == KeyGitHubTokenEnv) != (err == nil) {
			t.Errorf("%s=%s: err=%v", key, raw, err)
		}
	}
}

func TestCredentialFollowsOperatorSettings(t *testing.T) {
	useProvider(t)
	ctx := context.Background()
	t.Setenv("GHE_TOKEN", "synthetic-ghe-token")
	t.Setenv("NSELF_CI_GITLAB_TOKEN", "synthetic-gitlab-default")
	t.Setenv("GL_OTHER", "synthetic-gl-other")
	if tok, err := Credential(ctx, "gitlab")(ctx); err != nil || tok != "synthetic-gitlab-default" {
		t.Fatalf("gitlab default env: %q %v", tok, err)
	}
	UsePolicy(func(context.Context) (*policy.Effective, error) {
		eff, err := merge(t, scope(trust.SourceUser, map[string]string{KeyGitHubTokenEnv: `"GHE_TOKEN"`, KeyGitLabTokenEnv: `"GL_OTHER"`}))
		return &eff, err
	})
	if tok, err := Credential(ctx, "github")(ctx); err != nil || tok != "synthetic-ghe-token" {
		t.Fatalf("github token_env: %q %v", tok, err)
	}
	if tok, err := Credential(ctx, "gitlab")(ctx); err != nil || tok != "synthetic-gl-other" {
		t.Fatalf("gitlab token_env: %q %v", tok, err)
	}
	if tok, err := Credential(ctx, "fake")(ctx); err != nil || tok == "" {
		t.Fatalf("a registered rule: %q %v", tok, err)
	}
	for _, provider := range []string{"nobody"} {
		_, err := Credential(ctx, provider)(ctx)
		if err == nil || !strings.Contains(err.Error(), "E700") {
			t.Fatalf("%s: no rule must be E700, got %v", provider, err)
		}
	}
	UsePolicy(func(context.Context) (*policy.Effective, error) {
		eff, err := merge(t, scope(trust.SourceUser, map[string]string{KeyGitLabTokenEnv: `"GL_NEVER_SET"`}))
		return &eff, err
	})
	if _, err := Credential(ctx, "gitlab")(ctx); err == nil || !strings.Contains(err.Error(), "E700") {
		t.Fatalf("an unset variable must be E700, got %v", err)
	}
	if got := ghHost("https://api.github.com"); got != "github.com" {
		t.Fatalf("ghHost = %q", got)
	}
	if got := ghHost("https://ghe.example/api/v3"); got != "ghe.example" {
		t.Fatalf("ghHost GHES = %q", got)
	}
}
