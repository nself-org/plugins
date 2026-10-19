package readiness

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/readiness/sample"
)

func i(v int64) *int64     { return &v }
func f(v float64) *float64 { return &v }

// live is a healthy reading; each case overrides what it needs.
func live() sample.Live {
	return sample.Live{MemFreeMB: i(8000), LoadPerCore: f(0.2), CPUIdlePct: f(90), DiskFreeMB: i(50000), Notes: map[string]string{}}
}

func TestIdleBasedLoad(t *testing.T) {
	cfg := DefaultConfig()
	cases := []struct {
		name  string
		load  float64 // per core
		idle  float64
		ready bool
		why   string
	}{
		{"load 46 on 8 cores at 93% idle", 46.0 / 8, 93, true, ""},
		{"load 9 on 8 cores at 5% idle", 9.0 / 8, 5, false, "load"},
		{"busy but not heavy", 0.5, 60, true, ""},
		{"load just under the ceiling at low idle refuses on idle only", 1.0, 10, false, "CPU idle"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := live()
			l.LoadPerCore, l.CPUIdlePct = f(tc.load), f(tc.idle)
			r := Decide(cfg, Inputs{Live: l})
			if r.Ready != tc.ready {
				t.Fatalf("ready=%v why=%v", r.Ready, r.Why)
			}
			if tc.why != "" && !strings.Contains(strings.Join(r.Why, "|"), tc.why) {
				t.Fatalf("why %v lacks %q", r.Why, tc.why)
			}
		})
	}
	// With CPU idle unreadable the load average decides alone.
	l := live()
	l.CPUIdlePct, l.LoadPerCore = nil, f(5)
	cfg.MinIdlePct = 0
	if r := Decide(cfg, Inputs{Live: l}); r.Ready || len(r.Why) != 1 || !strings.Contains(r.Why[0], "load") {
		t.Fatalf("load must refuse alone when idle is off: %+v", r)
	}
}

func TestFloorsIndependent(t *testing.T) {
	cfg := DefaultConfig()
	if r := Decide(cfg, Inputs{Live: live()}); !r.Ready || len(r.Why) != 0 {
		t.Fatalf("healthy host must be ready: %+v", r)
	}
	cases := []struct {
		name  string
		mod   func(*sample.Live)
		token string
	}{
		{"memory", func(l *sample.Live) { l.MemFreeMB = i(100) }, "free memory 100 MB"},
		{"disk", func(l *sample.Live) { l.DiskFreeMB = i(10) }, "free disk 10 MB"},
		{"idle", func(l *sample.Live) { l.CPUIdlePct = f(3) }, "CPU idle 3%"},
		{"memory unreadable", func(l *sample.Live) { l.MemFreeMB = nil; l.Notes["mem_free_mb"] = "no meminfo" }, "free memory unreadable (no meminfo)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := live()
			tc.mod(&l)
			r := Decide(cfg, Inputs{Live: l})
			if r.Ready || len(r.Why) != 1 || !strings.Contains(r.Why[0], tc.token) {
				t.Fatalf("exactly one reason containing %q expected: %+v", tc.token, r)
			}
		})
	}
	// Every floor on its own can be switched off with 0, including when its value is unreadable.
	l := sample.Live{Notes: map[string]string{}}
	if r := Decide(Config{}, Inputs{Live: l}); !r.Ready {
		t.Fatalf("all floors off must be ready: %+v", r)
	}
	// Two floors failing at once give two reasons.
	l = live()
	l.MemFreeMB, l.DiskFreeMB = i(1), i(1)
	if r := Decide(cfg, Inputs{Live: l}); len(r.Why) != 2 {
		t.Fatalf("want two reasons: %v", r.Why)
	}
}

func TestReportMatchesSchema(t *testing.T) {
	schema, err := os.ReadFile("../../schemas/readiness.v1.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	l := live()
	for name, r := range map[string]Report{
		"ready":     Decide(DefaultConfig(), Inputs{Live: l}),
		"refused":   Decide(DefaultConfig(), Inputs{Live: sample.Live{Notes: map[string]string{}}, YieldedTo: []string{"zoom.us"}}),
		"all null":  {Schema: SchemaID, Ready: true, Why: []string{}, YieldedTo: []string{}},
		"yielding":  Decide(DefaultConfig(), Inputs{Live: l, YieldedTo: []string{"obs", "zoom.us"}}),
		"hook said": Decide(DefaultConfig(), Inputs{Live: l, Hook: Hook{Ran: true, Result: HookNotReady, Why: "gaming"}}),
	} {
		body, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		if err := model.ValidateJSON(schema, body); err != nil {
			t.Errorf("%s: %v\n%s", name, err, body)
		}
	}
	// A document that drops a required field or adds one is refused.
	for name, doc := range map[string]string{
		"missing":             `{"schema":"ci.readiness/v1","ready":true,"why":[]}`,
		"extra":               `{"schema":"ci.readiness/v1","ready":true,"why":[],"mem_free_mb":1,"load_per_core":0,"cpu_idle_pct":1,"disk_free_mb":1,"yielded_to":[],"x":1}`,
		"wrong id":            `{"schema":"ci.readiness/v2","ready":true,"why":[],"mem_free_mb":1,"load_per_core":0,"cpu_idle_pct":1,"disk_free_mb":1,"yielded_to":[]}`,
		"ready with a reason": `{"schema":"ci.readiness/v1","ready":true,"why":["x"],"mem_free_mb":1,"load_per_core":0,"cpu_idle_pct":1,"disk_free_mb":1,"yielded_to":[]}`,
		"refused without why": `{"schema":"ci.readiness/v1","ready":false,"why":[],"mem_free_mb":1,"load_per_core":0,"cpu_idle_pct":1,"disk_free_mb":1,"yielded_to":[]}`,
		"idle 101":            `{"schema":"ci.readiness/v1","ready":true,"why":[],"mem_free_mb":1,"load_per_core":0,"cpu_idle_pct":101,"disk_free_mb":1,"yielded_to":[]}`,
	} {
		if err := model.ValidateJSON(schema, []byte(doc)); err == nil {
			t.Errorf("%s must be refused", name)
		}
	}
}

func TestCodesRegistered(t *testing.T) {
	if p := model.CodeProblems(); len(p) != 0 {
		t.Fatal(p)
	}
	if got := coded("E799", "x").Error(); got != "E799: x" {
		t.Fatal(got)
	}
}
