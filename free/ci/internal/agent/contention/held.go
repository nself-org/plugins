package contention

// Purpose: the needs of the leases this node's gates hold, so a reservation is checked against cores and
// megabytes in use, not against a count of jobs (two 4-core jobs on 8 cores leave one more 1-core job
// no room under a 50% reservation).
// Inputs:  the agent state directory; always called with <state>/contention.lock held.
// Outputs: <state>/contention-held.json (0600), rewritten atomically.
// Constraints: an entry whose owner process is gone is dropped on read, the same rule the slot table
//          uses, so a crashed agent never strands capacity.

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"

	"github.com/nself-org/plugins/free/ci/internal/agent/slots"
)

const heldFile = "contention-held.json"

type hold struct {
	// Token tells two leases of one owner and attempt apart, so a stale Release frees only its own hold.
	Token   string      `json:"token"`
	Attempt string      `json:"attempt"`
	Owner   slots.Owner `json:"owner"`
	CPU     float64     `json:"cpu"`
	MemMB   int64       `json:"mem_mb"`
}

// readHolds returns the live holds. A missing file is empty; an unreadable or corrupt one is an error,
// so a damaged file refuses leases (fail closed) instead of silently counting nothing held.
func readHolds(dir string) ([]hold, error) {
	raw, err := os.ReadFile(filepath.Join(dir, heldFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var all []hold
	if err := json.Unmarshal(raw, &all); err != nil {
		return nil, fmt.Errorf("%s is corrupt: %w", heldFile, err)
	}
	live := all[:0]
	for _, h := range all {
		if slots.HolderLive(h.Owner) {
			live = append(live, h)
		}
	}
	return live, nil
}

func newToken() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func writeHolds(dir string, hs []hold) error {
	raw, err := json.Marshal(hs)
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, heldFile+"."+strconv.Itoa(os.Getpid())+".tmp")
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err = f.Write(raw); err == nil {
		err = f.Sync() // a power loss must not leave an empty file under the final name
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, filepath.Join(dir, heldFile)); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// without returns hs minus the hold with this token.
func without(hs []hold, token string) []hold {
	out := make([]hold, 0, len(hs))
	for _, h := range hs {
		if h.Token != token {
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
