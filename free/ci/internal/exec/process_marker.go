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
}

func markerPath(home, attempt string) string {
	return filepath.Join(home, "processes", attempt+".json")
}

func processIdentity(pid int) (string, error) {
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
	data, err := json.Marshal(processMarker{pid, coordinator, identity})
	if err != nil {
		return err
	}
	return os.WriteFile(markerPath(home, attempt), data, 0600)
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
	identity, err := processIdentity(marker.PID)
	if groupAlive(marker.PID) {
		if err == nil && identity != "" && identity != marker.Identity {
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
