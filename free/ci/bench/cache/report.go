package cache

// Purpose: turn the raw samples into the perfbench/v1 document (package bench's
// Result, the shape `perfbench check` reads) and the detail the evidence file
// quotes: medians, p95, hit rate, bytes and integrity overhead per fixture.
// Inputs:  []FixtureResult, carrier measurements, the revision.
// Outputs: Report, written as JSON.
// Constraints: metric names are cache.<fixture>.<mode>.<what>_ms, plus the
// aggregates restore_ms (every warm restore) and save_ms (every cold save)
// that perf-budget.json has always guarded; every metric carries its sample
// count, and a metric with no samples is left out, never filled.

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"

	"github.com/nself-org/plugins/free/ci/bench"
)

// Scenario is the scenario name of the cache result.
const Scenario = "cache-perf"

// FixtureDetail is what the evidence file quotes for one fixture.
type FixtureDetail struct {
	Fixture             string  `json:"fixture"`
	Runs                int     `json:"runs"`
	Files               int     `json:"files"`
	TreeBytes           int64   `json:"tree_bytes"`
	PackedBytes         int64   `json:"packed_bytes"`
	HitRate             float64 `json:"hit_rate"`
	ColdRestoreMedianMs float64 `json:"cold_restore_p50_ms"`
	ColdRestoreP95Ms    float64 `json:"cold_restore_p95_ms"`
	SaveMedianMs        float64 `json:"save_p50_ms"`
	SaveP95Ms           float64 `json:"save_p95_ms"`
	WarmRestoreMedianMs float64 `json:"warm_restore_p50_ms"`
	WarmRestoreP95Ms    float64 `json:"warm_restore_p95_ms"`
	RestoreMiBs         float64 `json:"warm_restore_mib_s"`
	SaveMiBs            float64 `json:"save_mib_s"`
	IntegrityMedianMs   float64 `json:"integrity_p50_ms"`
	IntegrityPct        float64 `json:"integrity_pct_of_warm_restore"`
	WarmFasterThanCold  bool    `json:"warm_restore_below_cold_save_plus_miss"`
}

// Carrier is one remote carrier and direction measured by the integration test.
type Carrier struct {
	Name         string    `json:"name"`
	Direction    string    `json:"direction"`
	PayloadBytes int64     `json:"payload_bytes"`
	WallMs       []float64 `json:"wall_ms"`
	BaselineMs   []float64 `json:"baseline_ms"`
	Source       string    `json:"source,omitempty"`
}

// CarrierDetail is a Carrier summarized.
type CarrierDetail struct {
	Name, Direction string
	PayloadBytes    int64
	N               int
	MedianMs, P95Ms float64
	MedianMiBs      float64
	BaselineP50Ms   float64
}

// Report is the perfbench/v1 document plus the detail section.
type Report struct {
	bench.Result
	Machine   string          `json:"machine"`
	Seed      int64           `json:"seed"`
	Fixtures  []FixtureDetail `json:"fixtures"`
	Transfers []CarrierDetail `json:"transfers,omitempty"`
}

func mib(b int64) float64 { return float64(b) / (1 << 20) }

// BuildReport summarizes res and carriers.
func BuildReport(sha string, seed int64, res []FixtureResult, carriers []Carrier) Report {
	var c bench.Collector
	rep := Report{Seed: seed, Machine: fmt.Sprintf("%s/%s %d cpu %s", runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), runtime.Version())}
	for _, fr := range res {
		d := FixtureDetail{Fixture: fr.Fixture, Runs: len(fr.Samples), Files: fr.Tree.Files, TreeBytes: fr.Tree.Bytes}
		var cold, save, warm, integ []float64
		hits := 0
		for _, s := range fr.Samples {
			cold, save, warm, integ = append(cold, s.ColdRestoreMs), append(save, s.SaveMs), append(warm, s.WarmRestoreMs), append(integ, s.IntegrityMs)
			c.Add("restore_ms", "ms", s.WarmRestoreMs)
			c.Add("save_ms", "ms", s.SaveMs)
			if s.Hit {
				hits++
			}
			d.PackedBytes = s.PackedBytes
		}
		d.HitRate = float64(hits) / float64(len(fr.Samples))
		for _, m := range []struct {
			name string
			v    []float64
		}{{"cold.restore_ms", cold}, {"cold.save_ms", save}, {"warm.restore_ms", warm}, {"warm.integrity_ms", integ}} {
			for _, v := range m.v {
				c.Add("cache."+fr.Fixture+"."+m.name, "ms", v)
			}
		}
		cm, sm, wm, im := bench.Summarize("", "ms", cold), bench.Summarize("", "ms", save), bench.Summarize("", "ms", warm), bench.Summarize("", "ms", integ)
		d.ColdRestoreMedianMs, d.ColdRestoreP95Ms = cm.P50, cm.P95
		d.SaveMedianMs, d.SaveP95Ms = sm.P50, sm.P95
		d.WarmRestoreMedianMs, d.WarmRestoreP95Ms = wm.P50, wm.P95
		d.IntegrityMedianMs = im.P50
		if wm.P50 > 0 {
			d.IntegrityPct = 100 * im.P50 / wm.P50
			d.RestoreMiBs = mib(fr.Tree.Bytes) / (wm.P50 / 1000)
		}
		if sm.P50 > 0 {
			d.SaveMiBs = mib(fr.Tree.Bytes) / (sm.P50 / 1000)
		}
		d.WarmFasterThanCold = wm.P50 < cm.P50+sm.P50
		rep.Fixtures = append(rep.Fixtures, d)
	}
	for _, k := range carriers {
		det := CarrierDetail{Name: k.Name, Direction: k.Direction, PayloadBytes: k.PayloadBytes, N: len(k.WallMs)}
		if len(k.WallMs) == 0 {
			continue
		}
		w, b := bench.Summarize("", "ms", k.WallMs), bench.Summarize("", "ms", k.BaselineMs)
		det.MedianMs, det.P95Ms, det.BaselineP50Ms = w.P50, w.P95, b.P50
		if w.P50 > 0 {
			det.MedianMiBs = mib(k.PayloadBytes) / (w.P50 / 1000)
		}
		for _, v := range k.WallMs {
			c.Add("transfer."+k.Name+"."+k.Direction+"_ms", "ms", v)
		}
		rep.Transfers = append(rep.Transfers, det)
	}
	n := 0
	for _, fr := range res {
		n = max(n, len(fr.Samples))
	}
	rep.Result = bench.Result{Schema: bench.SchemaV1, Scenario: Scenario, SHA: sha, GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Runs: n, Metrics: c.Metrics()}
	return rep
}

// Slowdown scales every timing of r by factor and keeps injected false: it is a
// derived fixture for the breach check, built from a recorded run.
func (r *Report) Slowdown(factor float64) {
	for i := range r.Metrics {
		m := &r.Metrics[i]
		m.P50, m.P95, m.Max = m.P50*factor, m.P95*factor, m.Max*factor
	}
}

// LoadCarriers reads the file TestRemoteCarrierThroughput writes.
func LoadCarriers(path string) ([]Carrier, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Carriers []Carrier `json:"carriers"`
	}
	return doc.Carriers, json.Unmarshal(raw, &doc)
}

// Load reads a report back (for -from).
func Load(path string) (Report, error) {
	var r Report
	raw, err := os.ReadFile(path)
	if err != nil {
		return r, err
	}
	if err = json.Unmarshal(raw, &r); err != nil {
		return r, err
	}
	if r.Schema != bench.SchemaV1 {
		return r, fmt.Errorf("report %s: schema %q", path, r.Schema)
	}
	return r, nil
}
