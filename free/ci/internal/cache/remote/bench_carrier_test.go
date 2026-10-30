//go:build integration

package remote

// Purpose: measured throughput of the two remote carriers (P7-CACHE-12): the real
// ssh driver (carrier A) and the real agent session over TLS (carrier B), each in
// both directions of a cache blob: the node's upload after a pass (After hook,
// transfer.Sender) and the coordinator's cold restore push to a node that holds
// no copy (RestoreLink, NodeRestorer).
// Inputs:  NSELF_CACHE_BENCH_RUNS (samples per carrier and direction, default 3),
// NSELF_CACHE_BENCH_OUT (a JSON file the run writes for cachebench -carriers).
// Outputs: wall time of one whole run per sample, for a payload and for a 1 KiB
// baseline, so the fixed cost of the session setup can be read off.
// Constraints: the rig is in process (io.Pipe and loopback), so the numbers bound
// the protocol and hashing cost, not a network; a real LAN pair is labelled lan
// and is not part of this test. The payload is a seeded tar under the rig's 4 MiB
// file limit; each restore sample gives the node an empty copy store.

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"math/rand"
	"os"
	"strconv"
	"testing"
	"time"
)

type carrierSample struct {
	Name         string    `json:"name"`
	Direction    string    `json:"direction"`
	PayloadBytes int64     `json:"payload_bytes"`
	WallMs       []float64 `json:"wall_ms"`
	BaselineMs   []float64 `json:"baseline_ms"`
	Source       string    `json:"source"`
}

func seededTar(t *testing.T, size int) []byte {
	t.Helper()
	raw := make([]byte, size-1024)
	rand.New(rand.NewSource(12)).Read(raw)
	var out bytes.Buffer
	tw := tar.NewWriter(&out)
	if err := tw.WriteHeader(&tar.Header{Name: "payload", Mode: 0600, Size: int64(len(raw))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestRemoteCarrierThroughput(t *testing.T) {
	runs := 3
	if v, err := strconv.Atoi(os.Getenv("NSELF_CACHE_BENCH_RUNS")); err == nil && v > 0 {
		runs = v
	}
	payload, small := seededTar(t, 3<<20), seededTar(t, 2048)
	type carrier struct {
		name string
		run  func(*rig, nodeSide) error
	}
	carriers := []carrier{{"ssh", func(r *rig, n nodeSide) error { return r.runSSH(n) }}, {"agent", func(r *rig, n nodeSide) error { return r.runAgent(n) }}}
	var out []carrierSample
	for _, c := range carriers {
		up := carrierSample{Name: c.name, Direction: "upload", PayloadBytes: int64(len(payload)), Source: "in-process rig"}
		rs := carrierSample{Name: c.name, Direction: "restore", PayloadBytes: int64(len(payload)), Source: "in-process rig"}
		for i := 0; i < runs; i++ {
			for _, data := range [][]byte{payload, small} {
				r := newRig(t)
				t0 := time.Now()
				if err := c.run(r, &node{t: t, data: data}); err != nil {
					t.Fatal(err)
				}
				wall := float64(time.Since(t0).Nanoseconds()) / 1e6
				r.assertCommit(data)
				if len(data) == len(payload) {
					up.WallMs = append(up.WallMs, wall)
				} else {
					up.BaselineMs = append(up.BaselineMs, wall)
				}
				r = newRig(t)
				n := newRestoreNode(t)
				b := r.planRestore(data)
				t0 = time.Now()
				if err := c.run(r, n); err != nil {
					t.Fatal(err)
				}
				wall = float64(time.Since(t0).Nanoseconds()) / 1e6
				n.assertCopy(r, b)
				if len(data) == len(payload) {
					rs.WallMs = append(rs.WallMs, wall)
				} else {
					rs.BaselineMs = append(rs.BaselineMs, wall)
				}
			}
		}
		out = append(out, up, rs)
	}
	for _, s := range out {
		t.Logf("%s %s: %d bytes, %d samples, wall ms %v, baseline ms %v", s.Name, s.Direction, s.PayloadBytes, len(s.WallMs), s.WallMs, s.BaselineMs)
	}
	if path := os.Getenv("NSELF_CACHE_BENCH_OUT"); path != "" {
		raw, err := json.MarshalIndent(map[string]any{"carriers": out}, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
