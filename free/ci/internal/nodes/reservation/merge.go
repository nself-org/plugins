package reservation

// Purpose: turn the `nodes set` flags into the node's next settings.
// Inputs:  the current settings, the flag values and which of them the operator gave, the node's known
//          core count (nil when unknown).
// Outputs: the merged, validated settings.
// Constraints: a flag not given keeps the current value; the error text has no code prefix (the command
//          wraps it as a usage error).

import (
	"errors"
	"fmt"
	"strings"
)

// Flags are the raw `nodes set` values; Given names the flags the operator typed.
type Flags struct {
	CPU, Mem, Interactive, OnBattery, Persistence string
	Given                                         map[string]bool
}

// Merge applies f over cur.
func Merge(cur Settings, f Flags, cpus *float64, memMB *int64) (Settings, error) {
	if f.Given["reserve-cpu"] {
		total := 0.0
		if cpus != nil {
			total = *cpus
		}
		v, err := ParseCPU(f.CPU, total)
		if err != nil {
			return cur, err
		}
		if cpus != nil && v >= *cpus {
			return cur, fmt.Errorf("reserved cpu %g is the node's whole %g cores or more, which would refuse all CI work", v, *cpus)
		}
		cur.ReservedCPU = v
	}
	if f.Given["reserve-mem"] {
		v, err := ParseMemMB(f.Mem)
		if err != nil {
			return cur, err
		}
		if memMB != nil && v >= *memMB {
			return cur, fmt.Errorf("reserved memory %d MB is the node's whole %d MB or more, which would refuse all CI work", v, *memMB)
		}
		cur.ReservedMemMB = v
	}
	if f.Given["interactive"] {
		cur.Interactive = f.Interactive
	}
	if f.Given["on-battery"] {
		cur.OnBattery = f.OnBattery
	}
	if f.Given["persistence"] {
		cur.Persistence = f.Persistence
	}
	if err := cur.Validate(); err != nil {
		return cur, errors.New(strings.TrimPrefix(err.Error(), "E651: "))
	}
	return cur, nil
}

// Check rejects a bad flag value before any node is looked up: unknown enum words, a memory figure
// that does not parse and a CPU figure that does not parse (a percentage is checked later, against the
// node's core count).
func (f Flags) Check() error {
	probe := Settings{NodeID: "x", Interactive: "protect", OnBattery: "pause", Persistence: "persistent"}
	if f.Given["reserve-cpu"] && !strings.HasSuffix(strings.TrimSpace(f.CPU), "%") {
		if _, err := ParseCPU(f.CPU, 0); err != nil {
			return err
		}
	}
	g := f
	g.CPU = ""
	g.Given = map[string]bool{}
	for k, v := range f.Given {
		g.Given[k] = v && k != "reserve-cpu"
	}
	_, err := Merge(probe, g, nil, nil)
	return err
}
