package main

// Purpose: `nself ci serve admin audit checkpoint export <path>` and `... verify --pubkey <fingerprint>
// <path>` (P7-TRUST-20, Epic D13/D20): the TEAM coordinator signs the head of the audit chain with its own
// ci-audit key; verify checks a checkpoint file against a fingerprint pinned off-host and against the live
// chain. It also registers the release quarantine seam of `ci artifacts verify`.
// Inputs:  a path; --pubkey (the fingerprint `export` printed); --json; $NSELF_CI_HOME (state.db and the
//          0600 key audit-checkpoint.key, created at the first export); $NSELF_CI_COORDINATOR_MODE=team.
// Outputs: export writes the checkpoint file and prints the public-key fingerprint; verify exits 0 when
//          the checkpoint is genuine and the chain still has its head, else 1.
// Constraints: V15 only; export is admin-only, refuses outside TEAM mode (a key on the machine that holds
//          the database proves nothing), refuses a damaged chain and seals first; verify refuses to run
//          without --pubkey and never trusts the key in the data dir; the live chain is verified with its
//          hashes, so a rewrite that recomputed every later hash still fails on the checkpointed head.

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// checkpointDir is the coordinator data dir that holds the checkpoint key; tests replace it.
var checkpointDir = lifecycle.CIHome

const maxCheckpointFile = 64 << 10

type checkpointFlags struct {
	json   bool
	pubkey string
}

func checkpointFlagSet(f *checkpointFlags, w io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet("serve admin audit checkpoint", flag.ContinueOnError)
	fs.SetOutput(w)
	fs.BoolVar(&f.json, "json", false, "print one v1 JSON envelope")
	fs.StringVar(&f.pubkey, "pubkey", "", "verify: the checkpoint key fingerprint pinned off this host")
	return fs
}

func init() {
	registerV15("serve admin audit checkpoint", subcommandEntry{
		V15Only: true,
		Flags:   func() *flag.FlagSet { var f checkpointFlags; return checkpointFlagSet(&f, io.Discard) },
		Handler: func(args []string) int { return runAdminCheckpoint(args, productionAdminEnv()) },
	})
	releaseQuarantined = storeQuarantined
}

// storeQuarantined reads the quarantine index of the coordinator store; a machine without one has nothing
// quarantined.
func storeQuarantined(ctx context.Context, ev model.Evidence) (bool, error) {
	path := filepath.Join(lifecycle.CIHome(), "state.db")
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	st, err := store.Open(path, store.Options{})
	if err != nil {
		return false, err
	}
	defer func() { _ = st.Close() }()
	return store.AttestLedger{S: st}.Quarantined(ctx, ev.RunID)
}

const checkpointUsage = "usage: nself ci serve admin audit checkpoint export <path> [--json] | verify --pubkey <fingerprint> <path> [--json]"

func runAdminCheckpoint(args []string, env adminEnv) int {
	const command = "ci serve admin audit checkpoint"
	var f checkpointFlags
	pos, err := adminParse(checkpointFlagSet(&f, env.Err), args)
	if err != nil {
		return adminFail(env, command, adminHasJSON(args), err)
	}
	if len(pos) != 2 || (pos[0] != "export" && pos[0] != "verify") {
		return adminFail(env, command, f.json, adminUsage("%s", checkpointUsage))
	}
	if pos[0] == "verify" && f.pubkey == "" {
		return adminFail(env, command, f.json, adminUsage("verify needs --pubkey <fingerprint>, pinned off this host: the key in the data dir is not trusted"))
	}
	if pos[0] == "export" && !teamCoordinator() {
		return adminFail(env, command, f.json, adminUsage("PERSONAL mode keeps no audit checkpoint key: a checkpoint signed on the machine that holds the database proves nothing; export on a TEAM coordinator"))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	svc, op, closeFn, err := env.Open(ctx)
	if err != nil {
		return adminFail(env, command, f.json, fmt.Errorf("the CI store cannot be opened: %w", err))
	}
	defer closeFn()
	if pos[0] == "export" {
		return checkpointExport(ctx, env, svc, op, pos[1], f.json)
	}
	return checkpointVerify(ctx, env, svc, op, pos[1], f)
}

// chainHead returns the newest row of the chain, which must be sealed and intact.
func chainHead(ctx context.Context, log *audit.Log) (store.AuditEntry, error) {
	rep, err := log.Verify(ctx, audit.SealInterval)
	if err != nil {
		return store.AuditEntry{}, err
	}
	if !rep.OK() || rep.Rows == 0 || rep.Unsealed > 0 {
		return store.AuditEntry{}, adminUsage("the audit chain is empty, damaged or has unsealed rows (rows %d, unsealed %d, first bad row %d); run `nself ci serve admin audit --verify`", rep.Rows, rep.Unsealed, rep.BadSeq)
	}
	var last store.AuditEntry
	for after := int64(0); ; {
		page, err := log.St.AuditEntries(ctx, after, 1000)
		if err != nil {
			return last, err
		}
		if len(page) == 0 {
			return last, nil
		}
		for _, e := range page {
			if e.Seq > int64(rep.Rows) {
				return last, nil // a row appended since Verify is neither verified nor sealed: never the head
			}
			last = e
		}
		after = page[len(page)-1].Seq
	}
}

func checkpointExport(ctx context.Context, env adminEnv, svc *roles.Service, op roles.Actor, path string, asJSON bool) int {
	const command = "ci serve admin audit checkpoint"
	if err := op.Can(roles.Admin, ""); err != nil {
		return adminFail(env, command, asJSON, err)
	}
	if _, err := svc.Log.Seal(ctx); err != nil {
		return adminFail(env, command, asJSON, err)
	}
	last, err := chainHead(ctx, svc.Log)
	if err != nil {
		return adminFail(env, command, asJSON, err)
	}
	signer, pub, err := attest.LoadOrCreateCheckpointKey(checkpointDir())
	if err != nil {
		return adminFail(env, command, asJSON, fmt.Errorf("the checkpoint key: %w", err))
	}
	cp, err := attest.NewCheckpoint(signer, pub, last.Head, last.Seq, last.Seq, svc.Now().UnixNano())
	if err != nil {
		return adminFail(env, command, asJSON, err)
	}
	doc, _ := json.MarshalIndent(cp, "", "  ")
	if err = writeAtomic(path, append(doc, '\n')); err != nil {
		return adminFail(env, command, asJSON, err)
	}
	fp := attest.Fingerprint(pub)
	if err = svc.Log.Append(ctx, audit.Event{Actor: op.Label(), Action: "audit.checkpoint", Target: "audit", Detail: fmt.Sprintf("seq=%d key=%s", cp.Seq, cp.KeyID)}); err != nil {
		return adminFail(env, command, asJSON, err)
	}
	if asJSON {
		return adminWriteData(env, command, map[string]any{"path": path, "seq": cp.Seq, "rows": cp.Rows, "head": cp.Head, "key_id": cp.KeyID, "fingerprint": fp})
	}
	_, _ = fmt.Fprintf(env.Out, "checkpoint written: %s\nrows: %d head: %s\npublic-key fingerprint: %s\nPin the fingerprint off this host; `checkpoint verify --pubkey` trusts nothing else.\n", path, cp.Rows, cp.Head, fp)
	return 0
}

// writeAtomic replaces path through a temporary file in the same directory.
func writeAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".checkpoint-")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err = tmp.Write(data); err == nil {
		err = tmp.Chmod(0o644)
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func checkpointVerify(ctx context.Context, env adminEnv, svc *roles.Service, op roles.Actor, path string, f checkpointFlags) int {
	const command = "ci serve admin audit checkpoint"
	if err := op.Can(roles.Release, ""); err != nil {
		return adminFail(env, command, f.json, err)
	}
	raw, err := readLimited(path)
	if err != nil {
		return adminFail(env, command, f.json, err)
	}
	var cp attest.Checkpoint
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&cp); err != nil {
		return adminFail(env, command, f.json, adminUsage("%s is not a checkpoint: %v", path, err))
	}
	fail := func(reason string) int {
		if f.json {
			_ = adminWriteData(env, command, map[string]any{"ok": false, "reason": reason})
		} else {
			_, _ = fmt.Fprintln(env.Err, "checkpoint NOT verified: "+reason)
		}
		return 1
	}
	if err = attest.VerifyCheckpoint(cp, f.pubkey); err != nil {
		return fail(err.Error())
	}
	rep, err := svc.Log.Verify(ctx, audit.SealInterval)
	if err != nil {
		return adminFail(env, command, f.json, err)
	}
	if rep.BadSeq != 0 {
		return fail(fmt.Sprintf("the audit chain is broken at row %d: %s", rep.BadSeq, rep.Reason))
	}
	page, err := svc.St.AuditEntries(ctx, cp.Seq-1, 1)
	if err != nil {
		return adminFail(env, command, f.json, err)
	}
	if len(page) == 0 || page[0].Seq != cp.Seq {
		return fail(fmt.Sprintf("row %d of the checkpoint is gone from the chain: rows were removed", cp.Seq))
	}
	if hex.EncodeToString(page[0].Head) != cp.Head {
		return fail(fmt.Sprintf("row %d has another head than the checkpoint: the chain was rewritten", cp.Seq))
	}
	if f.json {
		return adminWriteData(env, command, map[string]any{"ok": true, "seq": cp.Seq, "head": cp.Head, "key_id": cp.KeyID})
	}
	_, _ = fmt.Fprintf(env.Out, "checkpoint verified: row %d head %s, key %s\n", cp.Seq, cp.Head, cp.KeyID)
	return 0
}

func readLimited(path string) ([]byte, error) {
	fh, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = fh.Close() }()
	raw, err := io.ReadAll(io.LimitReader(fh, maxCheckpointFile+1))
	if err == nil && len(raw) > maxCheckpointFile {
		err = adminUsage("%s is too large for a checkpoint", path)
	}
	return raw, err
}
