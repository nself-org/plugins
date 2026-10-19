package backup

// Purpose: `nself ci serve backup` (P7-NODE-20): one consistent archive of a coordinator data dir.
// Inputs:  Options{DataDir}: <data>/state.db, <data>/pki/*.sealed, <data>/pki/kek (read only to derive the MAC
//          key), and, when present, <data>/known_hosts (SSH pins) and <data>/config.yaml (user config) so a
//          PERSONAL home keeps its SSH nodes and paired agents (Epic D21).
// Outputs: the archive on the writer and its Manifest; the incarnation field is the highest coordinator
//          incarnation in the snapshot.
// Constraints: the database snapshot is CI-31 store.Backup (VACUUM INTO), safe beside a running coordinator;
//          the KEK file is never an entry (only *.sealed files of pki/ are), no unsealed key exists on disk
//          to copy; a missing KEK or database is a refusal, never an empty or unauthenticated archive.

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Options configure Create and Restore.
type Options struct {
	// DataDir holds state.db and pki/; for a PERSONAL home it is $NSELF_CI_HOME.
	DataDir string
	// Clock is the time source (default time.Now); the store is opened with it too.
	Clock func() time.Time
	// Actor names the audit rows a restore writes (default "cli:serve-restore").
	Actor string
	// Alive reports whether a process id is alive (default: signal 0 on unix).
	Alive func(pid int) bool
}

func (o Options) now() time.Time {
	if o.Clock != nil {
		return o.Clock()
	}
	return time.Now()
}

func (o Options) storeOpts() store.Options { return store.Options{Clock: o.Clock} }

func (o Options) statePath() string { return filepath.Join(o.DataDir, nameState) }

// loadKEK reads the data dir's KEK as a refusal-shaped error.
func loadKEK(dataDir string) ([]byte, error) {
	kek, err := pki.LoadKEK(dataDir, false)
	switch {
	case errors.Is(err, pki.ErrNoKEK):
		return nil, fail(ReasonMissingKEK, "no key-encryption key at %s", pki.KEKPath(dataDir))
	case err != nil:
		return nil, fail(ReasonMissingKEK, "the key-encryption key is unusable: %v", err)
	}
	return kek, nil
}

// Create writes the archive of o.DataDir to w and returns its manifest.
func Create(ctx context.Context, o Options, w io.Writer) (Manifest, error) {
	var m Manifest
	if o.DataDir == "" {
		return m, fail(ReasonNoState, "no data dir")
	}
	if Pending(o.DataDir) {
		return m, fail(ReasonIncomplete, "an earlier `serve restore` did not finish on %s; run it again, or roll it back with the printed command, before taking a backup (the marker is %s)", o.DataDir, pendingPath(o.DataDir))
	}
	kek, err := loadKEK(o.DataDir)
	if err != nil {
		return m, err
	}
	if st, serr := os.Stat(o.statePath()); serr != nil || !st.Mode().IsRegular() {
		return m, fail(ReasonNoState, "no coordinator database at %s", o.statePath())
	}
	st, err := store.Open(o.statePath(), o.storeOpts())
	if err != nil {
		return m, fail(ReasonIO, "the database cannot be opened: %v", err)
	}
	defer func() { _ = st.Close() }()
	var db bytes.Buffer
	if err = st.Backup(ctx, &db); err != nil {
		return m, fail(ReasonIO, "the database snapshot failed: %v", err)
	}
	coords, err := st.Coordinators(ctx)
	if err != nil {
		return m, fail(ReasonIO, "the coordinator table cannot be read: %v", err)
	}
	for _, c := range coords {
		m.Incarnation = max(m.Incarnation, c.Incarnation)
	}
	es := []Entry{{Name: nameState, Data: db.Bytes()}}
	sealed, err := sealedFiles(o.DataDir)
	if err != nil {
		return m, err
	}
	extra, err := homeFiles(o.DataDir)
	if err != nil {
		return m, err
	}
	m.CreatedAt = o.now().UTC().Format(time.RFC3339)
	all := append(append(es, sealed...), extra...)
	m.Schema = Schema
	m.SHA256 = payloadDigest(all) // Write computes the same value for the archive; the caller reports it
	if err = Write(w, kek, m, all); err != nil {
		return m, err
	}
	return m, nil
}

// sealedFiles reads pki/*.sealed. The KEK file and every other name are skipped.
func sealedFiles(dataDir string) ([]Entry, error) {
	names, err := os.ReadDir(filepath.Join(dataDir, dirPKI))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fail(ReasonIO, "the pki directory cannot be read: %v", err)
	}
	var out []Entry
	for _, n := range names {
		if n.IsDir() || !strings.HasSuffix(n.Name(), ".sealed") || !safeName(dirPKI+"/"+n.Name()) {
			continue
		}
		b, rerr := os.ReadFile(filepath.Join(dataDir, dirPKI, n.Name()))
		if rerr != nil {
			return nil, fail(ReasonIO, "%s cannot be read: %v", n.Name(), rerr)
		}
		out = append(out, Entry{Name: dirPKI + "/" + n.Name(), Data: b})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// homeFiles reads the CI-home files that are not in the database.
func homeFiles(dataDir string) ([]Entry, error) {
	var out []Entry
	for _, n := range []string{"known_hosts", "config.yaml"} {
		b, err := os.ReadFile(filepath.Join(dataDir, n))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fail(ReasonIO, "%s cannot be read: %v", n, err)
		}
		out = append(out, Entry{Name: dirFiles + "/" + n, Data: b})
	}
	return out, nil
}
