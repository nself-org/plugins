package contention

// Purpose: the node's local copy of its policy, so every coordinator's session on the node enforces the
// same reservation (the registry belongs to one coordinator; the node serves several).
// Inputs:  the agent state directory and the node's store.NodeSettings.
// Outputs: <state>/contention.json (0600), written atomically; a Settings loader for Gate.Settings.
// Constraints: a missing file means "never configured" (defaults apply); a corrupt or invalid file is
//          an error, never silently replaced by defaults, because defaults hold no reservation.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"

	"github.com/nself-org/plugins/free/ci/internal/agent/nodekey"
	"github.com/nself-org/plugins/free/ci/internal/nodes/reservation"
)

const policyFile = "contention.json"

// SavePolicy writes s as the node's local policy.
func SavePolicy(stateDir string, s reservation.Settings) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if err := nodekey.EnsureDir(stateDir); err != nil {
		return err
	}
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	tmp := filepath.Join(stateDir, policyFile+"."+strconv.Itoa(os.Getpid())+".tmp")
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, filepath.Join(stateDir, policyFile)); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// LoadPolicy reads the local policy; ok is false when none was ever saved.
func LoadPolicy(stateDir string) (s reservation.Settings, ok bool, err error) {
	raw, err := os.ReadFile(filepath.Join(stateDir, policyFile))
	if errors.Is(err, fs.ErrNotExist) {
		return s, false, nil
	}
	if err != nil {
		return s, false, err
	}
	if err = json.Unmarshal(raw, &s); err != nil {
		return reservation.Settings{}, false, fmt.Errorf("%s is corrupt: %w", policyFile, err)
	}
	if err = s.Validate(); err != nil {
		return reservation.Settings{}, false, fmt.Errorf("%s is invalid: %w", policyFile, err)
	}
	return s, true, nil
}

// SettingsFrom returns a Gate.Settings function reading the local file on every call, falling back to
// defaults when none exists. An unreadable file yields the most restrictive answer (all CPU reserved),
// so a broken file never opens the node up.
func SettingsFrom(stateDir, nodeID string, operatorOwned, hasBattery bool) func() reservation.Settings {
	return func() reservation.Settings {
		s, ok, err := LoadPolicy(stateDir)
		switch {
		case err != nil:
			d := reservation.Defaults(nodeID, operatorOwned, hasBattery)
			d.ReservedCPU = 1e9
			return d
		case ok:
			return s
		}
		return reservation.Defaults(nodeID, operatorOwned, hasBattery)
	}
}
