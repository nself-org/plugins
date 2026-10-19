package adhoc

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

func has(levels ...model.Isolation) Selector {
	return func(_ context.Context, l model.Isolation) (model.Isolation, error) {
		for _, have := range levels {
			if have == l {
				return l, nil
			}
		}
		return "", errors.New("level " + string(l) + " is not available")
	}
}

// serves is a registry that answers every request with the one profile it has,
// as isolation.Select does when only a higher profile is available.
func serves(served model.Isolation) Selector {
	return func(context.Context, model.Isolation) (model.Isolation, error) { return served, nil }
}

func codeOf(err error) string { return cirun.CodeOf(err) }

func TestResolveLevelAndNetwork(t *testing.T) {
	svc := []string{"postgres"}
	for _, tc := range []struct {
		name  string
		o     cirun.Options
		fl    Flags
		trust model.TrustClass
		have  []model.Isolation
		level model.Isolation
		net   model.NetworkScope
		code  string
	}{
		{name: "default is a contained job with no network", have: []model.Isolation{"container"}, level: "container", net: "none"},
		{name: "services get the service network only", o: cirun.Options{Services: svc}, have: []model.Isolation{"container"}, level: "container", net: "restricted"},
		{name: "internet needs a container and is allowed there", o: cirun.Options{Network: "internet"}, have: []model.Isolation{"container"}, level: "container", net: "internet"},
		{name: "no docker and no flag is E646", code: CodeNoIsolation},
		{name: "process needs the flag and records lan", fl: Flags{Isolation: "process"}, level: "process", net: "lan"},
		{name: "process with services keeps lan", fl: Flags{Isolation: "process"}, o: cirun.Options{Services: svc}, level: "process", net: "lan"},
		{name: "process cannot hold network none", fl: Flags{Isolation: "process"}, o: cirun.Options{Network: "none"}, code: cirun.CodeOptions},
		{name: "process cannot do internet", fl: Flags{Isolation: "process"}, o: cirun.Options{Network: "internet"}, code: cirun.CodeOptions},
		{name: "lan needs process", o: cirun.Options{Network: "lan"}, have: []model.Isolation{"container"}, code: cirun.CodeOptions},
		{name: "services refuse network none", o: cirun.Options{Services: svc, Network: "none"}, have: []model.Isolation{"container"}, code: cirun.CodeOptions},
		{name: "restricted needs a service", o: cirun.Options{Network: "restricted"}, have: []model.Isolation{"container"}, code: cirun.CodeOptions},
		{name: "unknown isolation", fl: Flags{Isolation: "vm"}, have: []model.Isolation{"container"}, code: cirun.CodeOptions},
		{name: "untrusted never gets a service", o: cirun.Options{Services: svc}, trust: "untrusted", have: []model.Isolation{"container", "sandboxed-container"}, code: CodeTrustIsolation},
		{name: "untrusted without the sandbox fails closed", trust: "untrusted", have: []model.Isolation{"container"}, code: CodeTrustIsolation},
		{name: "untrusted never downgrades to process", trust: "untrusted", fl: Flags{Isolation: "process"}, have: []model.Isolation{"sandboxed-container"}, code: CodeTrustIsolation},
		{name: "untrusted never downgrades to container", trust: "untrusted", fl: Flags{Isolation: "container"}, have: []model.Isolation{"container", "sandboxed-container"}, code: CodeTrustIsolation},
		{name: "untrusted with the sandbox uses it", trust: "untrusted", have: []model.Isolation{"sandboxed-container"}, level: "sandboxed-container", net: "none"},
		{name: "untrusted never gets the internet", trust: "untrusted", o: cirun.Options{Network: "internet"}, have: []model.Isolation{"sandboxed-container"}, code: CodeTrustIsolation},
		{name: "collaborator needs a container", trust: "collaborator", fl: Flags{Isolation: "process"}, code: CodeTrustIsolation},
		{name: "explicit sandbox that is missing", fl: Flags{Isolation: "sandboxed-container"}, have: []model.Isolation{"container"}, code: CodeTrustIsolation},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, err := Resolve(context.Background(), tc.o, tc.fl, tc.trust, has(tc.have...))
			if tc.code != "" {
				if codeOf(err) != tc.code {
					t.Fatalf("code %q (%v), want %s", codeOf(err), err, tc.code)
				}
				return
			}
			if err != nil || m.Level != tc.level || m.Network != tc.net {
				t.Fatalf("got %+v %v, want %s/%s", m, err, tc.level, tc.net)
			}
		})
	}
}

// D-1021: the mode keeps the level asked for and also records the profile the registry served.
func TestResolveRecordsServedLevel(t *testing.T) {
	for name, tc := range map[string]struct {
		fl      Flags
		trust   model.TrustClass
		level   model.Isolation
		served  model.Isolation
		profile model.Isolation
	}{
		"container served by the sandbox": {level: "container", served: "sandboxed-container", profile: "sandboxed-container"},
		"container served as asked":       {level: "container", served: "container", profile: "container"},
		"untrusted served by the sandbox": {trust: "untrusted", level: "sandboxed-container", served: "sandboxed-container", profile: "sandboxed-container"},
		"explicit sandbox":                {fl: Flags{Isolation: "sandboxed-container"}, level: "sandboxed-container", served: "sandboxed-container", profile: "sandboxed-container"},
	} {
		t.Run(name, func(t *testing.T) {
			m, err := Resolve(context.Background(), cirun.Options{}, tc.fl, tc.trust, serves(tc.served))
			if err != nil || m.Level != tc.level || m.Served != tc.profile {
				t.Fatalf("got %+v %v, want level %s served %s", m, err, tc.level, tc.profile)
			}
		})
	}
	m, err := Resolve(context.Background(), cirun.Options{}, Flags{Isolation: "process"}, "", has())
	if err != nil || m.Served != "process" {
		t.Fatalf("process must record itself as served: %+v %v", m, err)
	}
}

func TestNoIsolationNamesBothOptions(t *testing.T) {
	_, err := Resolve(context.Background(), cirun.Options{}, Flags{}, "", has())
	if err == nil || !strings.Contains(err.Error(), "start Docker") || !strings.Contains(err.Error(), "--isolation process") {
		t.Fatalf("E646 must name both options: %v", err)
	}
}

func TestCodesAreRegistered(t *testing.T) {
	if p := model.CodeProblems(); len(p) != 0 {
		t.Fatal(p)
	}
}
