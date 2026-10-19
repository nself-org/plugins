package backup

// Purpose: the marker of a restore that has begun (P7-NODE-20, review F15): `restore.pending` in the data dir
//          is written before anything is replaced and removed only when the restore finished. While it
//          exists `ci serve` and `serve backup` refuse (incomplete_restore), and a re-run of the restore
//          remembers that the dir it replaces was empty, so a restore that failed half way can never leave
//          nodes trusted without `nodes enable`.
// Inputs:  the data dir. Outputs: Pending, plus the package-private read, write and clear.
// Constraints: the file holds no secret; a marker that cannot be written stops the restore before any change.

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const pendingName = "restore.pending"

type pendingMark struct {
	Empty bool   `json:"empty"`
	At    string `json:"at"`
}

func pendingPath(dir string) string { return filepath.Join(dir, pendingName) }

// Pending reports whether an unfinished restore left its marker in the data dir.
func Pending(dataDir string) bool {
	_, err := os.Stat(pendingPath(dataDir))
	return err == nil
}

func readPending(dir string) (pendingMark, bool) {
	raw, err := os.ReadFile(pendingPath(dir))
	if err != nil {
		return pendingMark{}, false
	}
	var m pendingMark
	if json.Unmarshal(raw, &m) != nil {
		return pendingMark{Empty: true}, true // an unreadable marker reads as empty (fail closed)
	}
	return m, true
}

func writePending(dir string, empty bool, at string) error {
	raw, err := json.Marshal(pendingMark{Empty: empty, At: at})
	if err != nil {
		return err
	}
	return writeFileAtomic(pendingPath(dir), raw)
}

func clearPending(dir string) error {
	if err := os.Remove(pendingPath(dir)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
