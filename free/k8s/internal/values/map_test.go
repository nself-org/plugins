// Purpose: tests for unsupported detection, ports, probes and routes.
//
// Inputs: inline models and testdata/full.
//
// Outputs: test results.
//
// Constraints: no docker.
package values

import (
	"encoding/json"
	"testing"
)

func TestUnsupportedReasons(t *testing.T) {
	cases := map[string]ComposeSvc{
		"host-net":   {Image: "x:1", NetworkMode: "host"},
		"priv":       {Image: "x:1", Privileged: true},
		"dev":        {Image: "x:1", Devices: []json.RawMessage{json.RawMessage(`"/dev/kvm"`)}},
		"Bad_Name":   {Image: "x:1"},
		"no-image":   {},
		"pid-shared": {Image: "x:1", Pid: "host"},
	}
	m := &Model{Name: "p", Services: cases}
	v, _, err := Map(m, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Services) != 0 || len(v.Unsupported) != len(cases) {
		t.Fatalf("services=%d unsupported=%d", len(v.Services), len(v.Unsupported))
	}
}

func TestProbeMapping(t *testing.T) {
	p, nm := mapProbe(&ComposeHealth{Test: []string{"CMD-SHELL", "pg_isready"}, Interval: "10s", Timeout: "1500ms", Retries: 5, StartPeriod: "2s"}, nil)
	if p == nil || len(nm) != 0 {
		t.Fatalf("probe=%v notMapped=%v", p, nm)
	}
	if p.PeriodS != 10 || p.TimeoutS != 2 || p.Retries != 5 || p.StartPeriodS != 2 || p.Exec[0] != "sh" {
		t.Errorf("probe = %+v", p)
	}
	if p, _ := mapProbe(&ComposeHealth{Test: []string{"NONE"}}, nil); p != nil {
		t.Error("NONE must give no probe")
	}
	if p, nm := mapProbe(&ComposeHealth{Test: []string{"CMD", "x"}, Interval: "bogus"}, nil); p != nil || len(nm) != 1 {
		t.Errorf("bad interval: probe=%v notMapped=%v", p, nm)
	}
}

func TestIngressRulesFromRoutes(t *testing.T) {
	_, v, _ := mapFixture(t, "full")
	byName := map[string]Rule{}
	for _, r := range v.Ingress.Rules {
		byName[r.Name] = r
	}
	r, ok := byName["core-hasura"]
	if !ok || r.Host != "api.example.test" || r.Service != "hasura" || r.Port != 8080 {
		t.Errorf("core-hasura rule = %+v ok=%v", r, ok)
	}
	// worker routes target unsupported services; the frontend targets the host.
	for _, id := range []string{"cs:1", "frontend:1"} {
		found := false
		for _, u := range v.UnmappedRoute {
			found = found || u.Name == id
		}
		if !found {
			t.Errorf("route %s should be listed unmapped", id)
		}
	}
}
