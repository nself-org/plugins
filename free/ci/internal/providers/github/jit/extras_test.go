package jit_test

// Purpose: tests beyond the Ticket's named seven: the policy keys, the fork-approval fact, and the
// rule that the encoded JIT config appears nowhere but the credential.
// Inputs:  synthetic scopes and fakeGH.
// Outputs: pass or fail.
// Constraints: no live network.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/jit"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

func merge(t *testing.T, kind trust.Source, kv map[string]string) (trust.Effective, error) {
	t.Helper()
	reg, err := trust.NewRegistry(jit.Keys()...)
	if err != nil {
		t.Fatal(err)
	}
	vals := map[string]json.RawMessage{}
	for k, v := range kv {
		vals[k] = json.RawMessage(v)
	}
	eff, _, err := reg.Merge("", trust.Scope{Kind: kind, Values: vals})
	return eff, err
}

// TestKeys proves the three keys are operator-only and strict: a repository scope cannot set them
// (E671), an operator can, and a reserved label, a hosted label, a bad scope or a bad group map is E672.
func TestKeys(t *testing.T) {
	if errs := jit.RegistrationErrors(); len(errs) != 0 {
		t.Fatal(errs)
	}
	for _, spec := range jit.Keys() {
		got, ok := policy.Default.Find(spec.Key)
		if !ok || !got.Spend || got.Kind != policy.TightenFixed || got.Default != nil {
			t.Errorf("%s: registered=%v operator-only=%v kind=%v default=%v", spec.Key, ok, got.Spend, got.Kind, got.Default)
		}
	}
	evil := map[string]string{jit.KeyLabel: `"evil"`, jit.KeyScope: `"org"`, jit.KeyGroups: `{"untrusted":1}`}
	eff, err := merge(t, trust.SourceRevision, evil)
	if err != nil {
		t.Fatal(err)
	}
	s, err := jit.SettingsFrom(&eff)
	if err != nil || s.Label != jit.DefaultLabel || s.Scope != jit.ScopeRepo || s.Groups != nil || len(eff.Ignored()) != 3 {
		t.Fatalf("a revision changed the settings: %+v %v ignored=%v", s, err, eff.Ignored())
	}
	eff, err = merge(t, trust.SourceUser, map[string]string{jit.KeyLabel: `"buildbox"`, jit.KeyScope: `"org"`, jit.KeyGroups: `{"untrusted":11,"trusted":12}`})
	if err != nil {
		t.Fatal(err)
	}
	s, err = jit.SettingsFrom(&eff)
	if err != nil || s.Label != "buildbox" || s.Scope != jit.ScopeOrg || s.Groups["trusted"] != 12 || len(eff.Ignored()) != 0 {
		t.Fatalf("operator values: %+v %v", s, err)
	}
	if _, err = jit.SettingsFrom(nil); err == nil {
		t.Fatal("a nil policy is an error")
	}
	for _, bad := range []map[string]string{
		{jit.KeyLabel: `"ubuntu-latest"`}, {jit.KeyLabel: `"self-hosted"`}, {jit.KeyLabel: `"X64"`}, {jit.KeyLabel: `"nself-jit-abc"`},
		{jit.KeyLabel: `"has space"`}, {jit.KeyLabel: `7`}, {jit.KeyScope: `"enterprise"`}, {jit.KeyScope: `true`},
		{jit.KeyGroups: `[]`}, {jit.KeyGroups: `null`}, {jit.KeyGroups: `{"untrusted":0}`}, {jit.KeyGroups: `{"untrusted":"11"}`}, {jit.KeyGroups: `{"Bad Tier":3}`},
	} {
		for k, v := range bad {
			if _, err := merge(t, trust.SourceUser, bad); err == nil || !strings.Contains(err.Error(), "E672") {
				t.Errorf("%s=%s must be E672, got %v", k, v, err)
			}
		}
	}
}

type sink struct{ puts []store.ProviderFact }

func (s *sink) ProviderFactPut(_ context.Context, f store.ProviderFact) (bool, error) {
	s.puts = append(s.puts, f)
	return true, nil
}

// TestApprovalPolicy proves the fork-approval read is stored as the host-provider fact
// github/<repo>/fork_pr_approval_policy and that an odd value or a missing repo is refused.
func TestApprovalPolicy(t *testing.T) {
	f := newFakeGH(t)
	s := &sink{}
	r := f.registrar(jit.Settings{}, s)
	got, err := r.FetchApprovalPolicy(context.Background(), repoOnly)
	if err != nil || got != "first_time_contributors" || len(s.puts) != 1 {
		t.Fatalf("%q %v %v", got, err, s.puts)
	}
	p := s.puts[0]
	if p.Provider != "github" || p.Scope != testScope || p.Key != "fork_pr_approval_policy" || !p.ObservedAt.Equal(epoch) ||
		string(p.Value) != `{"approval_policy":"first_time_contributors"}` {
		t.Fatalf("fact %+v", p)
	}
	f.approval = "../../etc"
	if _, err = r.FetchApprovalPolicy(context.Background(), repoOnly); err == nil || len(s.puts) != 1 {
		t.Fatalf("an invalid policy name must be refused and not stored: %v", err)
	}
	if _, err = r.FetchApprovalPolicy(context.Background(), providers.RunnerScope{Repo: "acme"}); err == nil {
		t.Fatal("a scope without owner/repo must be refused")
	}
}

// TestSecretsNeverLeak proves the encoded JIT config reaches the caller in the credential only: not
// in the Minted print forms, its JSON or slog value, not in the error of a failed teardown, and the
// request the funnel sends carries it nowhere.
func TestSecretsNeverLeak(t *testing.T) {
	f := newFakeGH(t)
	r := f.registrar(jit.Settings{}, nil)
	var funnel bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&funnel, nil)))
	m, err := r.MintRunner(context.Background(), providers.RunnerRequest{Scope: repoOnly})
	slog.SetDefault(old)
	if err != nil {
		t.Fatal(err)
	}
	secret := m.Credential.Value
	if secret == "" || m.Credential.Kind != "github-jit-config" {
		t.Fatalf("credential %+v", m.Credential)
	}
	var logs bytes.Buffer
	slog.New(slog.NewTextHandler(&logs, nil)).Info("minted", "runner", m)
	js, _ := json.Marshal(m)
	var shown []string
	shown = append(shown, funnel.String(), fmt.Sprint(m), fmt.Sprintf("%v %+v %#v %s", m, m, m, m), string(js), logs.String())
	f.keepDel = true
	derr := r.Deregister(context.Background(), m.Ref())
	var pe *providers.Error
	if !errors.As(derr, &pe) {
		t.Fatalf("want a provider error, got %v", derr)
	}
	shown = append(shown, derr.Error())
	for i, s := range shown {
		if strings.Contains(s, secret) {
			t.Errorf("form %d leaks the config", i)
		}
	}
}
