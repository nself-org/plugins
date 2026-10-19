package backup

// Purpose: `nself ci serve restore --in <file> --kek <file>` (P7-NODE-20): put a backup into a data dir.
// Inputs:  the archive, the KEK file's bytes (the operator keeps it; it is not in the archive), Options.
// Outputs: Result (what was saved, merged and rotated, and the rollback line); the data dir holds the
//          restored state.db, the supplied KEK, the sealed PKI files and the home files.
// Constraints: the HMAC is verified before ANY write (Read touches no file); a coordinator running on the
//          data dir is a refusal (running_coordinator); the previous state.db is saved as
//          state.db.pre-restore-<ts> (and a different KEK, known_hosts, config.yaml likewise) before the
//          swap; CI-31 Restore bumps every incarnation; the job-token keys are replaced, so a token minted
//          before the restore fails verification; every lease passes restart grace; revocations and the deny
//          list of the replaced dir are merged in; onto an EMPTY dir every node is needs-reconfirm.

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/lease"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// ReconfirmReason is the reason text of a node restored onto an empty dir.
const ReconfirmReason = "restored onto an empty data dir; confirm it with `nself ci nodes enable <node>`"

// Result says what a restore did.
type Result struct {
	Manifest         Manifest
	Empty            bool     // the dir held no data: every node is needs-reconfirm
	Saved            []string // pre-restore copies written
	Rollback         string   // shell line that puts the saved files back (stop the coordinator first)
	Incarnation      int64    // highest coordinator incarnation after the restore
	DeniedMerged     int64
	RevokedKept      int64
	NodesReconfirm   int64
	TokenKeysRotated int64
	LeasesGraced     int64
	Installed        []string
}

// Restore verifies the archive and replaces the data dir's state with it.
func Restore(ctx context.Context, o Options, in io.Reader, kek []byte) (res Result, err error) {
	if len(kek) != pki.KEKSize {
		return res, fail(ReasonMissingKEK, "the KEK must be %d bytes", pki.KEKSize)
	}
	if o.DataDir == "" {
		return res, fail(ReasonNoState, "no data dir")
	}
	m, es, err := Read(in, kek)
	if err != nil {
		return res, err
	}
	res.Manifest = m
	var image []byte
	for _, e := range es {
		if e.Name == nameState {
			image = e.Data
		}
	}
	if image == nil {
		return res, fail(ReasonFormat, "the backup has no %s", nameState)
	}
	_, statErr := os.Stat(o.statePath())
	old, err := store.Open(o.statePath(), o.storeOpts())
	if err != nil {
		return res, fail(ReasonIO, "the data dir database cannot be opened: %v", err)
	}
	defer func() { _ = old.Close() }()
	if id, rerr := running(ctx, o, old); rerr != nil || id != "" {
		if rerr != nil {
			return res, rerr
		}
		return res, fail(ReasonRunningCoordinator, "coordinator %s is running on %s; stop it before `serve restore`", id, o.DataDir)
	}
	p, err := snapshot(ctx, old, statErr == nil)
	if err != nil {
		return res, err
	}
	res.Empty = p.empty
	ts := o.now().UTC().Format("20060102T150405Z")
	saved, err := saveAside(ctx, o, old, p, es, kek, ts)
	res.Saved = savedNames(saved)
	res.Rollback = rollbackLine(o.DataDir, saved)
	if err != nil {
		return res, err
	}
	st, err := old.Restore(ctx, bytes.NewReader(image))
	if err != nil {
		return res, fail(ReasonIO, "the database image was refused: %v%s", err, rollbackHint(res.Rollback))
	}
	defer func() { _ = st.Close() }()
	if err = finish(ctx, o, st, p, es, kek, ts, &res); err != nil {
		return res, fail(ReasonIO, "%v%s", err, rollbackHint(res.Rollback))
	}
	return res, nil
}

// saveAside writes the pre-restore copies and returns original -> copy.
func saveAside(ctx context.Context, o Options, old *store.Store, p prior, es []Entry, kek []byte, ts string) (map[string]string, error) {
	saved := map[string]string{}
	suffix := ".pre-restore-" + ts
	if p.existed {
		to := o.statePath() + suffix
		f, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return saved, fail(ReasonIO, "the previous database cannot be saved: %v", err)
		}
		err = old.Backup(ctx, f)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			_ = os.Remove(to)
			return saved, fail(ReasonIO, "the previous database cannot be saved: %v", err)
		}
		saved[o.statePath()] = to
	}
	cur, kerr := os.ReadFile(pki.KEKPath(o.DataDir))
	if kerr == nil && !bytes.Equal(cur, kek) {
		to, err := copyAside(pki.KEKPath(o.DataDir), suffix)
		if err != nil {
			return saved, fail(ReasonIO, "the previous KEK cannot be saved: %v", err)
		}
		saved[pki.KEKPath(o.DataDir)] = to
	}
	for _, e := range es {
		if !strings.HasPrefix(e.Name, dirFiles+"/") {
			continue
		}
		dst := targetPath(o.DataDir, e.Name)
		to, err := copyAside(dst, suffix)
		if err != nil {
			return saved, fail(ReasonIO, "%s cannot be saved: %v", filepath.Base(e.Name), err)
		}
		if to != "" {
			saved[dst] = to
		}
	}
	return saved, nil
}

func savedNames(m map[string]string) []string {
	var out []string
	for _, v := range m {
		out = append(out, v)
	}
	sortStrings(out)
	return out
}

// finish installs the KEK and files, merges the replaced dir's revocations, marks nodes, rotates the token
// keys and gives every lease its restart grace.
func finish(ctx context.Context, o Options, st *store.Store, p prior, es []Entry, kek []byte, ts string, res *Result) error {
	actor := o.Actor
	if actor == "" {
		actor = "cli:serve-restore"
	}
	if err := writeFileAtomic(pki.KEKPath(o.DataDir), kek); err != nil {
		return err
	}
	c, err := mergePrior(ctx, st, p, actor)
	if err != nil {
		return err
	}
	res.DeniedMerged, res.RevokedKept = c.denied, c.revoked
	if p.empty {
		if res.NodesReconfirm, err = st.NodesNeedReconfirm(ctx, actor, ReconfirmReason); err != nil {
			return err
		}
	}
	if res.TokenKeysRotated, err = rotateTokens(ctx, o, st, actor); err != nil {
		return err
	}
	cs, err := st.Coordinators(ctx)
	if err != nil {
		return err
	}
	for _, c := range cs {
		res.Incarnation = max(res.Incarnation, c.Incarnation)
	}
	if res.LeasesGraced, err = lease.New(st, nil, lease.Config{Clock: o.Clock}).Start(ctx, "restore-"+ts, res.Incarnation); err != nil {
		return err
	}
	for _, e := range es {
		if e.Name == nameState {
			continue
		}
		dst := targetPath(o.DataDir, e.Name)
		if err = writeFileAtomic(dst, e.Data); err != nil {
			return err
		}
		res.Installed = append(res.Installed, e.Name)
	}
	return st.AuditAppend(ctx, store.AuditInput{Actor: actor, Action: "backup.restore", Target: "coordinator",
		Detail: "schema=" + itoa(int64(res.Manifest.Schema)) + " incarnation=" + itoa(res.Incarnation) + " empty=" + btoa(p.empty)})
}

// rotateTokens replaces the job-token keys of the restored image with a fresh one.
func rotateTokens(ctx context.Context, o Options, st *store.Store, actor string) (int64, error) {
	n, err := st.PKITokenKeysPurge(ctx, actor)
	if err != nil {
		return 0, err
	}
	auth, err := st.PKIAuthorities(ctx)
	if err != nil || len(auth) == 0 {
		return n, err // a backup of a coordinator that never paired has no hierarchy to unseal
	}
	ca, err := pki.New(ctx, pki.Options{Store: st, DataDir: o.DataDir, Now: o.Clock})
	if err != nil {
		return n, err
	}
	_, err = ca.Tokens(ctx)
	return n, err
}

// targetPath maps an archive entry to its place in the data dir: pki/x.sealed stays, files/x lands in the home.
func targetPath(dataDir, name string) string {
	if strings.HasPrefix(name, dirFiles+"/") {
		return filepath.Join(dataDir, strings.TrimPrefix(name, dirFiles+"/"))
	}
	return filepath.Join(dataDir, filepath.FromSlash(name))
}
