package runners_test

// Purpose: tests of providers.gitlab.runner.hosts (D-1393): the operator names the artifact and object-storage hosts a runner
//          may reach, and only the operator can.
// Inputs:  the key specs, scopes and a handler built in the test.
// Outputs: pass or fail.
// Constraints: no network, no environment.

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/runners"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

func TestHostsKeyIsOperatorOnly(t *testing.T) {
	if errs := runners.RegistrationErrors(); len(errs) != 0 {
		t.Fatal(errs)
	}
	for _, kind := range []trust.Source{trust.SourceRevision, trust.SourceProtectedBranch, trust.SourcePipeline, trust.SourceJob} {
		eff, err := merge(t, scope(kind, map[string]string{runners.KeyHosts: `["evil.example"]`}))
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		s, err := runners.SettingsFrom(&eff)
		if err != nil || len(s.Hosts) != 0 || len(eff.Ignored()) != 1 {
			t.Fatalf("%s scope widened the network scope: %+v %v ignored=%v", kind, s, err, eff.Ignored())
		}
	}
	eff, err := merge(t, scope(trust.SourceUser, map[string]string{runners.KeyHosts: `["objects.example.com","cdn.example.net"]`}))
	if err != nil {
		t.Fatal(err)
	}
	s, err := runners.SettingsFrom(&eff)
	if err != nil || !reflect.DeepEqual(s.Hosts, []string{"objects.example.com", "cdn.example.net"}) {
		t.Fatalf("operator hosts: %+v %v", s, err)
	}
	if d := runners.DefaultSettings(); len(d.Hosts) != 0 {
		t.Fatalf("default hosts %v", d.Hosts)
	}
}

func TestHostsKeyRefusesBadShapes(t *testing.T) {
	for _, raw := range []string{
		`"objects.example.com"`, `null`, `[1]`, `["a.example","b.example","c.example","d.example"]`, // shape, count
		`["10.0.0.5"]`, `["127.0.0.1"]`, `["::1"]`, `["[::1]"]`, `["1.2.3.4"]`, // IP literals
		`["objects.example.com:8443"]`, `["https://objects.example.com"]`, `["objects.example.com/x"]`, // port, URL, path
		`["Objects.Example.com"]`, `["*.example.com"]`, `["a b.example"]`, `["a,b.example"]`, `["localhost"]`, `["intranet"]`, // case, wildcard, space, comma, single label
		`["objects.example.com","objects.example.com"]`, `[""]`, `["a.example"] ["b.example"]`, `["-a.example"]`, `["a..example"]`, `["a.example."]`,
	} {
		if _, err := merge(t, scope(trust.SourceUser, map[string]string{runners.KeyHosts: raw})); err == nil || !strings.Contains(err.Error(), "E672") {
			t.Errorf("%s must be refused with E672, got %v", raw, err)
		}
	}
	for _, raw := range []string{`[]`, `["objects.example.com"]`, `["a.example","b.example","c.example"]`, `["xn--bcher-kva.example"]`} {
		if _, err := merge(t, scope(trust.SourceUser, map[string]string{runners.KeyHosts: raw})); err != nil {
			t.Errorf("%s must be accepted: %v", raw, err)
		}
	}
}

// The synthesized job names the instance host first and the operator's extra hosts after it, once each.
func TestSynthesizeCarriesTheOperatorsHosts(t *testing.T) {
	bind(t)
	h := handler(50)
	for name, tc := range map[string]struct {
		extra []string
		want  string
	}{
		"none":              {nil, "gitlab.example.com"},
		"two":               {[]string{"objects.example.com", "cdn.example.net"}, "gitlab.example.com,objects.example.com,cdn.example.net"},
		"instance repeated": {[]string{"gitlab.example.com", "objects.example.com"}, "gitlab.example.com,objects.example.com"},
	} {
		extra := tc.extra
		h.Settings = func(context.Context) (runners.Settings, error) {
			return runners.Settings{Tags: []string{"nself"}, MaxPending: 50, Hosts: extra}, nil
		}
		p, err := h.Synthesize(context.Background(), demandFor("5001", "nself"))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := p.Jobs[0].Env["NSELF_RUNNERJOB_HOSTS"]; got != tc.want {
			t.Errorf("%s: hosts %q, want %q", name, got, tc.want)
		}
	}
}
