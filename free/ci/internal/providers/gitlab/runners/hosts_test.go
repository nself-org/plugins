package runners_test

// Purpose: tests of providers.gitlab.runner.hosts (D-1393): the extra hosts a GitLab runner may reach
// under the restricted network scope (artifact and object storage, a cache host).
// Inputs:  hand-built policy scopes and trigger facts.
// Outputs: pass or fail.
// Constraints: no network; the key is operator-only like the other runner keys.

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/runners"
	"github.com/nself-org/plugins/free/ci/internal/providers/runnerjob"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// TestHostsKeyOperatorOnly: only the operator sets the hosts; a repository scope is ignored.
func TestHostsKeyOperatorOnly(t *testing.T) {
	eff, err := merge(t, scope(trust.SourceRevision, map[string]string{runners.KeyHosts: `["evil.example.net"]`}))
	if err != nil {
		t.Fatal(err)
	}
	s, err := runners.SettingsFrom(&eff)
	if err != nil || len(s.Hosts) != 0 || len(eff.Ignored()) != 1 {
		t.Fatalf("a repository scope set the hosts: %+v %v ignored=%v", s, err, eff.Ignored())
	}
	eff, err = merge(t, scope(trust.SourceUser, map[string]string{runners.KeyHosts: `["objects.example.com","cache.example.com"]`}))
	if err != nil {
		t.Fatal(err)
	}
	s, err = runners.SettingsFrom(&eff)
	if err != nil || !reflect.DeepEqual(s.Hosts, []string{"objects.example.com", "cache.example.com"}) {
		t.Fatalf("operator hosts: %+v %v", s, err)
	}
	if d := runners.DefaultSettings(); len(d.Hosts) != 0 {
		t.Fatalf("no default hosts, got %v", d.Hosts)
	}
}

// TestHostsKeyRefusesBadShapes: a value the restricted scope could not serve, or that is not a plain
// lower-case host name, is E672.
func TestHostsKeyRefusesBadShapes(t *testing.T) {
	for _, raw := range []string{
		`"objects.example.com"`, `null`, `[]`, `[1]`, `["Objects.Example.com"]`, `["a b.example.com"]`, `["a,b.example.com"]`,
		`["10.0.0.5"]`, `["[::1]"]`, `["objects.example.com:8443"]`, `["https://objects.example.com"]`, `["x.example.com","x.example.com"]`,
		`["a.example.com","b.example.com","c.example.com","d.example.com"]`, `["a.example.com"] ["b.example.com"]`,
	} {
		if _, err := merge(t, scope(trust.SourceUser, map[string]string{runners.KeyHosts: raw})); err == nil || !strings.Contains(err.Error(), "E672") {
			t.Errorf("%s must be refused with E672, got %v", raw, err)
		}
	}
}

// TestSynthesizeAddsConfiguredHosts: the job's host list is the instance host first, then the configured
// hosts; the instance host is not repeated.
func TestSynthesizeAddsConfiguredHosts(t *testing.T) {
	bind(t)
	h := handler(2)
	h.Settings = func(context.Context) (runners.Settings, error) {
		return runners.Settings{Tags: []string{"nself"}, MaxPending: 2,
			Hosts: []string{"objects.example.com", "gitlab.example.com", "cache.example.com"}}, nil
	}
	p, err := h.Synthesize(context.Background(), demandFor("4001", "nself"))
	if err != nil || p == nil {
		t.Fatalf("synthesize: %v", err)
	}
	if got := p.Jobs[0].Env[runnerjob.EnvHosts]; got != "gitlab.example.com,objects.example.com,cache.example.com" {
		t.Fatalf("hosts = %q", got)
	}
}
