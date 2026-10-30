// Command cachebench measures the cache engine on seeded Go, pnpm and Rust
// fixtures and writes a perfbench/v1 document (P7-CACHE-12). Every timing is
// taken from a real specs.Begin and Finalize cycle; nothing is simulated.
//
//	cachebench -runs 20 -json > result.json
//	cachebench -runs 20 -carriers carriers.json -json > result.json
//	cachebench -from result.json -synthetic-slowdown 2 -json > slow.json
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/nself-org/plugins/free/ci/bench/cache"
)

func main() {
	runs := flag.Int("runs", 10, "interleaved cold/warm cycles per fixture")
	seed := flag.Int64("seed", 20260430, "fixture seed")
	carriers := flag.String("carriers", "", "carrier measurements written by TestRemoteCarrierThroughput")
	from := flag.String("from", "", "load a recorded report instead of running")
	slow := flag.Float64("synthetic-slowdown", 0, "scale the recorded timings of -from by this factor (breach-check fixture)")
	jsonOut := flag.Bool("json", false, "write the report as JSON on stdout")
	work := flag.String("work", "", "work directory (default: a fresh one under $TMPDIR)")
	flag.Parse()
	if err := run(*runs, *seed, *carriers, *from, *work, *slow, *jsonOut); err != nil {
		fmt.Fprintln(os.Stderr, "cachebench:", err)
		os.Exit(1)
	}
}

func run(runs int, seed int64, carriersPath, from, work string, slow float64, jsonOut bool) error {
	var rep cache.Report
	var err error
	if from != "" {
		if rep, err = cache.Load(from); err != nil {
			return err
		}
		if slow > 0 {
			rep.Slowdown(slow)
		}
	} else {
		if slow != 0 {
			return fmt.Errorf("-synthetic-slowdown scales a recorded report: pass -from")
		}
		if rep, err = measure(runs, seed, carriersPath, work); err != nil {
			return err
		}
	}
	if !jsonOut {
		return nil
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(rep)
}

func measure(runs int, seed int64, carriersPath, work string) (cache.Report, error) {
	if runs < 1 {
		return cache.Report{}, fmt.Errorf("-runs must be at least 1")
	}
	if work == "" {
		dir, err := os.MkdirTemp(os.Getenv("TMPDIR"), "cachebench.")
		if err != nil {
			return cache.Report{}, err
		}
		defer os.RemoveAll(dir)
		work = dir
	}
	res, err := cache.Run(context.Background(), cache.Options{Runs: runs, Seed: seed, Dir: work})
	if err != nil {
		return cache.Report{}, err
	}
	var carriers []cache.Carrier
	if carriersPath != "" {
		if carriers, err = cache.LoadCarriers(carriersPath); err != nil {
			return cache.Report{}, err
		}
	}
	return cache.BuildReport(revision(), seed, res, carriers), nil
}

// revision is the sha of the checkout under measurement, passed in NSELF_REVISION
// by the caller (this package never starts a process); "unknown" when unset.
func revision() string {
	if v := os.Getenv("NSELF_REVISION"); v != "" {
		return v
	}
	return "unknown"
}
