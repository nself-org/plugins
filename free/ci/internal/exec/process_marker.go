package exec

import (
	"context"
	"encoding/json"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type processMarker struct {
	PID         int    `json:"pid"`
	Coordinator string `json:"coordinator"`
	Identity    string `json:"identity"`
	// Descendants are the processes the tracker saw under the leader, each with its start
	// identity. They are how a later agent finds a job process that left the leader's group
	// (setsid, double fork) when the agent that ran the job was killed.
	Descendants []markerProc `json:"descendants,omitempty"`
}

// markerProc names one tracked descendant; Identity is what sameProcess compares.
type markerProc struct {
	PID      int    `json:"pid"`
	Identity string `json:"identity"`
}

// maxMarkedDescendants bounds the marker: a job that forks without end must not grow it forever.
const maxMarkedDescendants = 512

func markerPath(home, attempt string) string {
	return filepath.Join(home, "processes", attempt+".json")
}

// processIdentity names a process by its start instant. The native form
// ("st:" prefix) needs no fork; markers written by older builds hold the ps
// lstart text, so psIdentity stays for comparing against those.
func processIdentity(pid int) (string, error) {
	if id, ok := nativeStartID(pid); ok {
		return id, nil
	}
	return psIdentity(pid)
}

// sameProcess compares the live start instant of pid with a marker's recorded
// identity, in the form the marker was written in: "st:" markers use the native
// reader, older markers hold ps lstart text. known is false when the live
// identity cannot be read in that form, which is not the same as "different".
func sameProcess(pid int, recorded string) (same, known bool) {
	var now string
	if strings.HasPrefix(recorded, "st:") {
		id, ok := nativeStartID(pid)
		if !ok {
			return false, false
		}
		now = id
	} else {
		id, err := psIdentity(pid)
		if err != nil || id == "" {
			return false, false
		}
		now = id
	}
	return now == recorded, true
}

func psIdentity(pid int) (string, error) {
	out, err := osexec.Command("ps", "-o", "lstart=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func writeProcessMarker(home, attempt, coordinator string, pid int) error {
	identity, err := processIdentity(pid)
	if err != nil {
		return err
	}
	if identity == "" {
		return coded("E610", "process identity unavailable")
	}
	root := filepath.Join(home, "processes")
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	data, err := json.Marshal(processMarker{PID: pid, Coordinator: coordinator, Identity: identity})
	if err != nil {
		return err
	}
	return os.WriteFile(markerPath(home, attempt), data, 0600)
}

// recordDescendants adds the start identity of the newly seen pids to the attempt's marker and
// drops entries whose process has gone (or whose pid now belongs to another process). It only
// edits a marker that exists, so a marker removed by cleanup is not brought back. Only the job's
// tracker writes after the marker's first write, so there is no concurrent writer.
func recordDescendants(home, attempt string, pids []int) error {
	if !safeID.MatchString(attempt) {
		return nil
	}
	data, err := os.ReadFile(markerPath(home, attempt))
	if err != nil {
		return err
	}
	var marker processMarker
	if err := json.Unmarshal(data, &marker); err != nil {
		return err
	}
	kept := make([]markerProc, 0, len(marker.Descendants)+len(pids))
	have := map[int]bool{}
	for _, d := range marker.Descendants {
		if same, known := sameProcess(d.PID, d.Identity); known && same {
			kept = append(kept, d)
			have[d.PID] = true
		}
	}
	for _, pid := range pids {
		if pid <= 1 || have[pid] || len(kept) >= maxMarkedDescendants {
			continue
		}
		if id, err := processIdentity(pid); err == nil && id != "" {
			kept = append(kept, markerProc{PID: pid, Identity: id})
		}
	}
	marker.Descendants = kept
	out, err := json.Marshal(marker)
	if err != nil {
		return err
	}
	tmp := markerPath(home, attempt) + ".tmp"
	if err := os.WriteFile(tmp, out, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, markerPath(home, attempt))
}

func removeProcessMarker(home, attempt string) {
	if safeID.MatchString(attempt) {
		_ = os.Remove(markerPath(home, attempt))
	}
}

func (e *Executor) sweepProcesses(ctx context.Context, home, current string) error {
	entries, err := os.ReadDir(filepath.Join(home, "processes"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		attempt := strings.TrimSuffix(entry.Name(), ".json")
		if !safeID.MatchString(attempt) {
			continue
		}
		if err := e.sweepProcessMarker(ctx, home, current, attempt); err != nil {
			return err
		}
	}
	return nil
}

func (e *Executor) sweepProcessMarker(ctx context.Context, home, current, attempt string) error {
	data, err := os.ReadFile(markerPath(home, attempt))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var marker processMarker
	if json.Unmarshal(data, &marker) != nil || !safeID.MatchString(marker.Coordinator) || marker.PID <= 0 {
		return nil
	}
	if marker.Coordinator == current {
		return nil
	}
	stale, err := e.Stale.Stale(ctx, marker.Coordinator)
	if err != nil {
		return err
	}
	if !stale {
		return nil
	}
	killMarkedDescendants(marker.Descendants)
	if groupAlive(marker.PID) {
		if same, known := sameProcess(marker.PID, marker.Identity); known && !same {
			// A different process has reused the leader PID. Do not kill its group.
			return coded("E610", "stale group identity changed")
		}
		if err := killGroup(marker.PID); err != nil && groupAlive(marker.PID) {
			return coded("E610", "stale process group survived")
		}
		deadline := time.Now().Add(2 * time.Second)
		for groupAlive(marker.PID) && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		if groupAlive(marker.PID) {
			return coded("E610", "stale process group survived")
		}
	}
	removeProcessMarker(home, attempt)
	_ = os.RemoveAll(filepath.Join(home, "work", attempt))
	return nil
}
