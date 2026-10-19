package contention

// Purpose: the needs of the leases this node's gates hold, so a reservation is checked against cores and
// megabytes in use, not against a count of jobs (two 4-core jobs on 8 cores leave one more 1-core job
// no room under a 50% reservation).
// Inputs:  the agent state directory; always called with <state>/contention.lock held.
// Outputs: <state>/contention-held.json (0600), rewritten atomically.
// Constraints: an entry whose owner process is gone is dropped on read, the same rule the slot table
//          uses, so a crashed agent never strands capacity.

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"

	"github.com/nself-org/plugins/free/ci/internal/agent/slots"
)

const heldFile = "contention-held.json"

type hold struct {
	Attempt string      `json:"attempt"`
	Owner   slots.Owner `json:"owner"`
	CPU     float64     `json:"cpu"`
	MemMB   int64       `json:"mem_mb"`
}

// readHolds returns the live holds; an unreadable file reads as empty (the slot table still caps).
func readHolds(dir string) []hold {
	raw, err := os.ReadFile(filepath.Join(dir, heldFile))
	if errors.Is(err, fs.ErrNotExist) || err != nil {
		return nil
	}
	var all []hold
	if json.Unmarshal(raw, &all) != nil {
		return nil
	}
	live := all[:0]
	for _, h := range all {
		if slots.HolderLive(h.Owner) {
			live = append(live, h)
		}
	}
	return live
}

func writeHolds(dir string, hs []hold) error {
	raw, err := json.Marshal(hs)
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, heldFile+"."+strconv.Itoa(os.Getpid())+".tmp")
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, filepath.Join(dir, heldFile)); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// without returns hs minus the hold of attempt under owner.
func without(hs []hold, owner slots.Owner, attempt string) []hold {
	out := make([]hold, 0, len(hs))
	for _, h := range hs {
		if h.Owner != owner || h.Attempt != attempt {
			out = append(out, h)
		}
	}
	return out
}

// totals sums the cores and megabytes held.
func totals(hs []hold) (cpu float64, mem int64) {
	for _, h := range hs {
		cpu += h.CPU
		mem += h.MemMB
	}
	return cpu, mem
}
