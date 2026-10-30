package sched

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"
	"time"
)

func TestSummarize(t *testing.T) {
	samples := []float64{10, 1, 9, 2, 8, 3, 7, 4, 6, 5}
	original := append([]float64(nil), samples...)
	got, err := Summarize("test", samples)
	if err != nil || got.Median != 5.5 || got.P95 != 10 {
		t.Fatalf("summary = %+v, %v", got, err)
	}
	if !reflect.DeepEqual(samples, original) {
		t.Fatal("modified input samples")
	}
	samples[0] = 100
	if got.Samples[0] != 10 {
		t.Fatal("summary aliases input")
	}
	for _, invalid := range [][]float64{nil, {0}, {-1}, {math.NaN()}, {math.Inf(1)}} {
		if _, err := Summarize("test", invalid); err == nil {
			t.Fatalf("accepted %v", invalid)
		}
	}
}

func TestMeasureInterleavesAndDiscardsWarmups(t *testing.T) {
	var order []string
	prepared, cleaned := 0, 0
	probes := map[string]Probe{}
	for _, name := range MetricNames() {
		probes[name] = Probe{
			Prepare: func(context.Context) error { prepared++; return nil },
			Run: func(context.Context) error {
				order = append(order, name)
				time.Sleep(time.Microsecond)
				return nil
			},
			Cleanup: func(context.Context) error { cleaned++; return nil },
		}
	}
	got, err := Measure(context.Background(), probes)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 11 || prepared != 132 || cleaned != 132 {
		t.Fatalf("metrics=%d prepared=%d cleaned=%d", len(got), prepared, cleaned)
	}
	for _, observation := range got {
		if len(observation.Samples) != 10 || observation.Unit != "ms" {
			t.Fatalf("invalid observation: %+v", observation)
		}
	}
	if order[7] != "ci.e2e_tiny.v14" || order[18] != "ci.e2e_tiny.v15" {
		t.Fatal("legacy and V15 order did not alternate")
	}
}

func TestProbeFailureAndCleanup(t *testing.T) {
	want := errors.New("failed")
	for _, failPrepare := range []bool{false, true} {
		cleaned, ran := false, false
		p := Probe{
			Prepare: func(context.Context) error {
				if failPrepare {
					return want
				}
				return nil
			},
			Run:     func(context.Context) error { ran = true; return want },
			Cleanup: func(context.Context) error { cleaned = true; return nil },
		}
		_, err := measureOne(context.Background(), p)
		if !errors.Is(err, want) || !cleaned || ran == failPrepare {
			t.Fatalf("err=%v cleaned=%v ran=%v", err, cleaned, ran)
		}
	}
	if _, err := Measure(context.Background(), nil); err == nil {
		t.Fatal("missing probes accepted")
	}
}

func TestTinyRatioBoundaryAndSlowdown(t *testing.T) {
	metric := func(name string, v float64) Observation {
		samples := make([]float64, 10)
		for i := range samples {
			samples[i] = v
		}
		return Observation{Name: name, Unit: "ms", Samples: samples}
	}
	legacy := metric("ci.e2e_tiny.v14", 1000)
	v15 := metric("ci.e2e_tiny.v15", 1350)
	if err := CheckTinyRatio([]Observation{legacy, v15}); err != nil {
		t.Fatal(err)
	}
	slow := metric("ci.e2e_tiny.v15", 1351)
	slow.Median = 1 // A forged summary must not hide slow raw samples.
	for _, invalid := range [][]Observation{nil, {legacy}, {legacy, legacy, v15}, {legacy, slow}} {
		if err := CheckTinyRatio(invalid); err == nil {
			t.Fatalf("accepted invalid result: %+v", invalid)
		}
	}
}
