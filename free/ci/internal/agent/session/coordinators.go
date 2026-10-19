package session

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/agent/slots"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// CoordinatorsFile is <state>/coordinators.json: one entry per coordinator the
// agent has had a welcome from. P7-NODE-24 reads it for the upgrade ceiling.
const CoordinatorsFile = "coordinators.json"

// Coordinator identifies the peer of a session.
type Coordinator struct {
	ID              string `json:"id"`
	RootFingerprint string `json:"root_fingerprint"`
}

// CoordinatorRecord is one entry of coordinators.json.
type CoordinatorRecord struct {
	Coordinator
	// ProtocolMin and ProtocolMax are the protocol versions the coordinator
	// accepted from this agent: v1 negotiates one version, so both are the
	// welcome's version.
	ProtocolMin     int    `json:"protocol_min"`
	ProtocolMax     int    `json:"protocol_max"`
	MinAgentVersion string `json:"min_agent_version"`
	LastWelcome     string `json:"last_welcome"`
}

type coordinatorsDoc struct {
	Coordinators []CoordinatorRecord `json:"coordinators"`
}

// RecordWelcome upserts the coordinator's entry, atomically and under a lock
// (several agent processes, one per carrier, write the same file).
func RecordWelcome(stateDir string, c Coordinator, w protocol.Welcome, now time.Time) error {
	path := filepath.Join(stateDir, CoordinatorsFile)
	return slots.WithLock(stateDir, CoordinatorsFile+".lock", func() error {
		var doc coordinatorsDoc
		raw, err := os.ReadFile(path)
		switch {
		case errors.Is(err, fs.ErrNotExist):
		case err != nil:
			return err
		default:
			if err := json.Unmarshal(raw, &doc); err != nil {
				doc = coordinatorsDoc{} // a damaged cache of welcomes is rebuilt, not fatal
			}
		}
		rec := CoordinatorRecord{Coordinator: c, ProtocolMin: w.Version, ProtocolMax: w.Version, MinAgentVersion: w.MinAgentVersion, LastWelcome: now.UTC().Format(time.RFC3339)}
		replaced := false
		for i := range doc.Coordinators {
			if doc.Coordinators[i].ID == c.ID {
				doc.Coordinators[i], replaced = rec, true
			}
		}
		if !replaced {
			doc.Coordinators = append(doc.Coordinators, rec)
		}
		sort.Slice(doc.Coordinators, func(i, j int) bool { return doc.Coordinators[i].ID < doc.Coordinators[j].ID })
		out, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			return err
		}
		tmp := path + ".tmp"
		if err := os.WriteFile(tmp, out, 0o600); err != nil {
			return err
		}
		return os.Rename(tmp, path)
	})
}

// ReadCoordinators returns the recorded coordinators (none when absent).
func ReadCoordinators(stateDir string) ([]CoordinatorRecord, error) {
	raw, err := os.ReadFile(filepath.Join(stateDir, CoordinatorsFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var doc coordinatorsDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	return doc.Coordinators, nil
}
