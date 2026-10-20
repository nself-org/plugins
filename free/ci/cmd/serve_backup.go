package main

// Purpose: `nself ci serve backup --out <file>` and `nself ci serve restore --in <file> --kek <file>`
// (P7-NODE-20, Epic D5/D21): the coordinator backup over CI-31's Backup/Restore, implemented in
// internal/coordinator/backup. Capability cap:ci.coordinator-backup.
// Inputs:  --data-dir (default $NSELF_CI_HOME or ~/.nself/ci: a TEAM data dir or a PERSONAL home), --out,
//          --in, --kek, --json.
// Outputs: backup writes the archive (0600) and prints its size, incarnation and checksum; restore prints
//          what it saved, merged and rotated, and the rollback command. One v1 JSON envelope with --json;
//          failures are E668 with a reason key. Exit 0, 1 (refused: bad MAC, wrong KEK, running
//          coordinator, bad usage), 2 (infrastructure).
// Constraints: V15 only, so v1.4 argv is frozen. Side-effect classes: serve backup read (it writes only the
//          archive file the operator names), serve restore destructive (it replaces state.db; the previous one
//          is kept as state.db.pre-restore-<ts>). The KEK is read from a file and never printed or archived;
//          a restore verifies the archive before it writes anything.

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/output"
	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/coordinator/backup"
	"github.com/nself-org/plugins/free/ci/internal/pki"
)

// backupEnv is the environment of the two commands; tests replace Clock and Alive.
type backupEnv struct {
	Out, Err io.Writer
	Clock    func() time.Time
	Alive    func(pid int) bool
}

func productionBackupEnv() backupEnv { return backupEnv{Out: os.Stdout, Err: os.Stderr} }

type backupFlags struct {
	dataDir, out, in, kek string
	json                  bool
}

func backupFlagSet(name string, f *backupFlags, w io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(w)
	fs.Usage = func() {}
	fs.StringVar(&f.dataDir, "data-dir", "", "coordinator state directory (state.db, pki/); default NSELF_CI_HOME or ~/.nself/ci")
	fs.BoolVar(&f.json, "json", false, "print one JSON envelope")
	switch name {
	case "serve backup":
		fs.StringVar(&f.out, "out", "", "archive file to write (required)")
	case "serve restore":
		fs.StringVar(&f.in, "in", "", "archive file to restore (required)")
		fs.StringVar(&f.kek, "kek", "", "key-encryption key file of the coordinator that made the backup (required)")
	}
	return fs
}

func init() {
	registerV15("serve backup", subcommandEntry{V15Only: true,
		Flags:   func() *flag.FlagSet { var f backupFlags; return backupFlagSet("serve backup", &f, io.Discard) },
		Handler: func(args []string) int { return runServeBackup(args, productionBackupEnv()) }})
	registerV15("serve restore", subcommandEntry{V15Only: true,
		Flags:   func() *flag.FlagSet { var f backupFlags; return backupFlagSet("serve restore", &f, io.Discard) },
		Handler: func(args []string) int { return runServeRestore(args, productionBackupEnv()) }})
}

// backupFail prints err and returns the exit code: 1 for a refusal the operator can fix, 2 for infrastructure.
func backupFail(env backupEnv, command string, asJSON bool, err error) int {
	code, exit := backup.CodeBackup, output.ExitUser
	var be *backup.Error
	switch {
	case errors.As(err, &be):
		if be.Reason == backup.ReasonIO {
			exit = output.ExitInfra
		}
	default:
		code = "E651" // usage
	}
	msg := strings.Join(strings.Fields(err.Error()), " ")
	if asJSON {
		_ = output.WriteError(env.Out, command, output.ErrorDetail{Code: code, Message: msg, ExitCode: exit}, nil)
	} else {
		_, _ = io.WriteString(env.Err, msg+"\n")
	}
	return exit
}

func backupParse(name string, args []string, env backupEnv) (backupFlags, error) {
	var f backupFlags
	fs := backupFlagSet(name, &f, io.Discard)
	if err := fs.Parse(args); err != nil {
		return f, fmt.Errorf("%s: %v", name, err)
	}
	if fs.NArg() != 0 {
		return f, fmt.Errorf("%s takes flags only (got %q)", name, fs.Arg(0))
	}
	f.dataDir = records.StoreDir(f.dataDir)
	return f, nil
}

func runServeBackup(args []string, env backupEnv) int {
	const command = "ci serve backup"
	f, err := backupParse("serve backup", args, env)
	if err == nil && f.out == "" {
		err = errors.New("usage: nself ci serve backup --out <file> [--data-dir <dir>] [--json]")
	}
	if err != nil {
		return backupFail(env, command, backupJSON(args), err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(f.out), ".backup-*")
	if err != nil {
		return backupFail(env, command, f.json, &backup.Error{Reason: backup.ReasonIO, Msg: "the output file cannot be created: " + err.Error()})
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	m, err := backup.Create(context.Background(), backup.Options{DataDir: f.dataDir, Clock: env.Clock}, tmp)
	if cerr := tmp.Close(); err == nil && cerr != nil {
		err = &backup.Error{Reason: backup.ReasonIO, Msg: cerr.Error()}
	}
	if err == nil {
		if err = os.Chmod(tmp.Name(), 0o600); err == nil {
			err = os.Rename(tmp.Name(), f.out)
		}
		if err != nil {
			err = &backup.Error{Reason: backup.ReasonIO, Msg: "the archive cannot be placed: " + err.Error()}
		}
	}
	if err != nil {
		return backupFail(env, command, f.json, err)
	}
	st, _ := os.Stat(f.out)
	if f.json {
		return backupData(env, command, map[string]any{"out": f.out, "bytes": st.Size(), "incarnation": m.Incarnation, "created_at": m.CreatedAt, "sha256": m.SHA256, "kek_included": false})
	}
	_, _ = fmt.Fprintf(env.Out, "backup written to %s (%d bytes, incarnation %d, sha256 %s)\n", f.out, st.Size(), m.Incarnation, m.SHA256)
	_, _ = fmt.Fprintf(env.Err, "the key-encryption key is NOT in the archive: keep %s apart from it, restore needs it\n", pki.KEKPath(f.dataDir))
	return 0
}

func runServeRestore(args []string, env backupEnv) int {
	const command = "ci serve restore"
	f, err := backupParse("serve restore", args, env)
	if err == nil && (f.in == "" || f.kek == "") {
		err = errors.New("usage: nself ci serve restore --in <file> --kek <file> [--data-dir <dir>] [--json]")
	}
	if err != nil {
		return backupFail(env, command, backupJSON(args), err)
	}
	kek, err := os.ReadFile(f.kek)
	if err != nil || len(kek) != pki.KEKSize {
		return backupFail(env, command, f.json, &backup.Error{Reason: backup.ReasonMissingKEK, Msg: fmt.Sprintf("the KEK file %s must hold exactly %d bytes", f.kek, pki.KEKSize)})
	}
	in, err := os.Open(f.in)
	if err != nil {
		return backupFail(env, command, f.json, &backup.Error{Reason: backup.ReasonIO, Msg: "the backup cannot be opened: " + err.Error()})
	}
	defer func() { _ = in.Close() }()
	res, err := backup.Restore(context.Background(), backup.Options{DataDir: f.dataDir, Clock: env.Clock, Alive: env.Alive, Actor: "cli:" + operatorPrincipal()}, in, kek)
	if err != nil {
		return backupFail(env, command, f.json, err)
	}
	if f.json {
		return backupData(env, command, map[string]any{"data_dir": f.dataDir, "incarnation": res.Incarnation, "empty_data_dir": res.Empty, "nodes_needs_reconfirm": res.NodesReconfirm,
			"deny_rows_merged": res.DeniedMerged, "revocations_kept": res.RevokedKept, "token_keys_rotated": res.TokenKeysRotated, "leases_in_grace": res.LeasesGraced,
			"saved": res.Saved, "rollback": res.Rollback})
	}
	_, _ = fmt.Fprintf(env.Out, "restored %s into %s (incarnation %d, %d lease(s) in restart grace, token keys rotated)\n", f.in, f.dataDir, res.Incarnation, res.LeasesGraced)
	if res.Empty {
		_, _ = fmt.Fprintf(env.Out, "%d node(s) are needs-reconfirm and refused until `nself ci nodes enable <node>`\n", res.NodesReconfirm)
	}
	if res.DeniedMerged+res.RevokedKept > 0 {
		_, _ = fmt.Fprintf(env.Out, "kept %d revoked node(s) and merged %d deny-list row(s) of the replaced data dir\n", res.RevokedKept, res.DeniedMerged)
	}
	if res.Rollback != "" {
		_, _ = fmt.Fprintf(env.Out, "previous state saved; to roll back, stop the coordinator and run:\n  %s\n", res.Rollback)
	}
	return 0
}

func backupData(env backupEnv, command string, data any) int {
	if output.WriteData(env.Out, command, data, nil) != nil {
		return output.ExitInfra
	}
	return 0
}

func backupJSON(args []string) bool { return adminHasJSON(args) }
