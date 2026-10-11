// Purpose: the metadata sidecar of a k8s backup: <dir>/.meta/<file>.json.
//
// Inputs: a Meta (backup) or a dump path (restore).
//
// Outputs: the sidecar file (0600, dir 0700, written under a temp name and
// renamed), or the parsed and validated Meta.
//
// Constraints: core "nself backup create" writes no metadata for a local full
// backup, and core list/latest/prune read only top-level files, so the sidecar
// lives in the .meta subdirectory and leaves them unchanged. It holds no secret
// and no host name: project, file, size, checksum, time, server version, release,
// namespace and the table fingerprints. Restore refuses a dump without a valid
// sidecar (E734) before anything runs.
package ops

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const (
	metaDir        = ".meta"
	metaVersion    = "1"
	metaKind       = "k8s-backup"
	maxMetaBytes   = 64 << 20
	sha256HexBytes = 64
)

// Meta is the content of the sidecar.
type Meta struct {
	SchemaVersion string        `json:"schema_version"`
	Kind          string        `json:"kind"`
	Project       string        `json:"project"`
	File          string        `json:"file"`
	Bytes         int64         `json:"bytes"`
	SHA256        string        `json:"sha256"`
	CreatedAt     string        `json:"created_at"`
	ServerVersion string        `json:"server_version"`
	Release       string        `json:"release"`
	Namespace     string        `json:"namespace"`
	Fingerprints  []Fingerprint `json:"fingerprints"`
}

// MetaPath is the sidecar path of a dump.
func MetaPath(dump string) string {
	return filepath.Join(filepath.Dir(dump), metaDir, filepath.Base(dump)+".json")
}

// writeMeta writes m as the sidecar of dump: temp file in .meta, then a link that never
// replaces an existing sidecar (errExists, E738).
func writeMeta(dump string, m Meta) error {
	final := MetaPath(dump)
	dir := filepath.Dir(final)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(name) // also after a successful link: the temp name is not needed
		}
	}()
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// Publish without replacing: a sidecar that already exists belongs to another backup.
	// A hard link fails if final exists; the temp name is removed either way (ok stays
	// false). A file system without hard links falls back to rename after an Lstat check.
	switch lerr := os.Link(name, final); {
	case lerr == nil:
		return nil
	case errors.Is(lerr, os.ErrExist):
		return errExists()
	default:
		if _, serr := os.Lstat(final); serr == nil {
			return errExists()
		}
		if rerr := os.Rename(name, final); rerr != nil {
			return rerr
		}
		ok = true
		return nil
	}
}

// readMeta reads and validates the sidecar of dump (E734 on any problem).
func readMeta(dump string) (Meta, error) {
	var m Meta
	p := MetaPath(dump)
	fi, err := os.Stat(p)
	if err != nil || !fi.Mode().IsRegular() {
		return m, errInvalid("the backup has no metadata file: it was not made by nself k8s backup", "")
	}
	if fi.Size() > maxMetaBytes {
		return m, errInvalid("the backup metadata file is too large", "")
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return m, errInvalid("the backup metadata file cannot be read", "")
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return Meta{}, errInvalid("the backup metadata file is not valid JSON", "")
	}
	if m.SchemaVersion != metaVersion || m.Kind != metaKind {
		return Meta{}, errInvalid("the backup metadata file is not a k8s backup record of a known version", "")
	}
	if raw, err := hex.DecodeString(m.SHA256); err != nil || len(m.SHA256) != sha256HexBytes || len(raw) != 32 {
		return Meta{}, errInvalid("the backup metadata file has no valid checksum", "")
	}
	if m.Bytes < 0 || m.Fingerprints == nil {
		return Meta{}, errInvalid("the backup metadata file is incomplete", "")
	}
	return m, nil
}
