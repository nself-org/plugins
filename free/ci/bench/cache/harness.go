package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"time"
)

// PerfbenchSchema is the contract identifier emitted by the harness.
const PerfbenchSchema = "perfbench/v1"

// RunResult captures one scenario of the cache harness.
type RunResult struct {
	// Fixture identifies the repository shape (go, pnpm, rust).
	Fixture string `json:"fixture"`
	// Mode is either "cold" or "warm".
	Mode string `json:"mode"`
	// RestoreMs records restore durations in milliseconds.
	RestoreMs []float64 `json:"restore_ms"`
	// SaveMs records save durations in milliseconds.
	SaveMs []float64 `json:"save_ms"`
	// HitRate is the fraction of cache hits observed during the run.
	HitRate float64 `json:"hit_rate"`
	// BytesTransferred is the total payload moved during the run.
	BytesTransferred int64 `json:"bytes_transferred"`
}

// TransferResult records remote transfer throughput for a carrier.
type TransferResult struct {
	// Carrier names the transport (sim-a, sim-b, lan).
	Carrier string `json:"carrier"`
	// ThroughputMiBs is the measured throughput in MiB/s.
	ThroughputMiBs float64 `json:"throughput_mib_s"`
}

// Report is the top-level perfbench/v1 document produced by the harness.
type Report struct {
	Schema    string           `json:"schema"`
	Revision  string           `json:"revision"`
	Machine   string           `json:"machine"`
	Timestamp time.Time        `json:"timestamp"`
	Runs      []RunResult      `json:"runs"`
	Transfer  []TransferResult `json:"transfer"`
	Metrics   []Observation    `json:"metrics"`
	Injected  bool             `json:"injected"`
}

// NewReport builds a report skeleton populated with machine metadata.
func NewReport(revision string) *Report {
	return &Report{
		Schema:    PerfbenchSchema,
		Revision:  revision,
		Machine:   fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		Timestamp: time.Now().UTC(),
		Injected:  false,
	}
}

// WriteJSON encodes the report as indented JSON to path.
func (r *Report) WriteJSON(path string) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// LoadReport reads a perfbench/v1 JSON file back into memory.
func LoadReport(path string) (*Report, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rep Report
	if err := json.Unmarshal(data, &rep); err != nil {
		return nil, err
	}
	if rep.Schema != PerfbenchSchema {
		return nil, fmt.Errorf("unexpected schema %q", rep.Schema)
	}
	return &rep, nil
}
