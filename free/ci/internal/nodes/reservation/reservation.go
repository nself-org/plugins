// Package reservation is the node-settings policy of Epic D10 (P7-NODE-13): the defaults of a node
// nobody configured, the capacity an operator reservation leaves to CI, and the parsing of the
// `nodes set` values.
//
// Purpose: one place that turns store.NodeSettings plus a host snapshot into "how much may CI use".
// Inputs:  store.NodeSettings, exec.Capacity (the P7-CI-32 probe snapshot), a Need per job.
// Outputs: Defaults, Share (cores, memory, slot limit, refusal reason), parsed flag values.
// Constraints: pure functions, no I/O; a reservation holds whenever CI is not the caller's
//
//	environment (CI=true switches reservations off, CI D6); the operator's own foreground run
//	is never limited here (floor of one job, X17), callers do not ask for it.
package reservation

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Settings is the stored policy of one node.
type Settings = store.NodeSettings

// Need is what one job asks of its slot. Zero values mean one CPU and no memory figure.
type Need struct {
	CPU   float64
	MemMB int64
}

// Share is the capacity CI may use on a node after the reservation.
type Share struct {
	// Active is false when no reservation applies (none set, or CI=true).
	Active bool
	// CPUCapped and MemCapped say which resource the operator reserved, so only that one is capped.
	CPUCapped, MemCapped bool
	// CPU and MemMB are what remains for CI; with Active false they are the host totals.
	CPU   float64
	MemMB int64
	// Slots is how many jobs of this Need fit in the share (0 means none).
	Slots int
	// Reason is model.NodeReservation when Slots is 0.
	Reason model.PlacementReason
}

// Defaults are the settings of a node the operator never configured (Epic D10): protect on the
// operator's own machine, intermittent when a battery exists, pause on battery, nothing reserved.
func Defaults(nodeID string, operatorOwned, hasBattery bool) Settings {
	s := Settings{NodeID: nodeID, Interactive: store.InteractiveIgnore, OnBattery: store.OnBatteryPause, Persistence: store.PersistencePersist}
	if operatorOwned {
		s.Interactive = store.InteractiveProtect
	}
	if hasBattery {
		s.Persistence = store.PersistenceInterm
	}
	return s
}

// HasBattery is true when the probe found a battery (a percentage), plugged in or not.
func HasBattery(c exec.Capacity) bool { return c.BatteryPercent >= 0 }

// Compute returns the share of c that CI may use under s for a job of need.
func Compute(c exec.Capacity, s Settings, need Need) Share {
	cpus, mem := float64(c.CPUs), int64(c.MemoryMB)
	sh := Share{CPU: cpus, MemMB: mem}
	if !c.CI && (s.ReservedCPU > 0 || s.ReservedMemMB > 0) {
		sh.Active = true
		sh.CPUCapped, sh.MemCapped = s.ReservedCPU > 0, s.ReservedMemMB > 0
		sh.CPU = math.Max(0, cpus-s.ReservedCPU)
		sh.MemMB = max(0, mem-s.ReservedMemMB)
	}
	perCPU := need.CPU
	if perCPU < 1 {
		perCPU = 1
	}
	sh.Slots = int(math.Floor(sh.CPU / perCPU))
	if need.MemMB > 0 {
		sh.Slots = min(sh.Slots, int(sh.MemMB/need.MemMB))
	} else if sh.MemMB <= 0 {
		sh.Slots = 0
	}
	if sh.Slots < 1 {
		sh.Slots, sh.Reason = 0, model.NodeReservation
	}
	return sh
}

// ParseCPU reads a `--reserve-cpu` value: cores ("2", "0.5") or a percentage of totalCPUs ("50%").
// A percentage needs totalCPUs > 0 (the node's known core count).
func ParseCPU(v string, totalCPUs float64) (float64, error) {
	v = strings.TrimSpace(v)
	pct := strings.HasSuffix(v, "%")
	n, err := strconv.ParseFloat(strings.TrimSuffix(v, "%"), 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 {
		return 0, fmt.Errorf("reserved cpu %q: want cores (2, 0.5) or a percentage (50%%)", v)
	}
	if !pct {
		return n, nil
	}
	if n > 100 {
		return 0, fmt.Errorf("reserved cpu %q: a percentage is at most 100%%", v)
	}
	if totalCPUs <= 0 {
		return 0, fmt.Errorf("reserved cpu %q: the node's core count is unknown, give cores instead", v)
	}
	return totalCPUs * n / 100, nil
}

// ParseMemMB reads a `--reserve-mem` value in MB ("2048") or GB ("2g").
func ParseMemMB(v string) (int64, error) {
	orig := v
	v = strings.ToLower(strings.TrimSpace(v))
	mult := int64(1)
	switch {
	case strings.HasSuffix(v, "gb"):
		mult, v = 1024, strings.TrimSuffix(v, "gb")
	case strings.HasSuffix(v, "g"):
		mult, v = 1024, strings.TrimSuffix(v, "g")
	case strings.HasSuffix(v, "mb"):
		v = strings.TrimSuffix(v, "mb")
	case strings.HasSuffix(v, "m"):
		v = strings.TrimSuffix(v, "m")
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 || n > math.MaxInt64/mult {
		return 0, fmt.Errorf("reserved memory %q: want MB (2048) or GB (2g)", orig)
	}
	return n * mult, nil
}
