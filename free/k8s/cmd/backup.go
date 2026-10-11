// Purpose: `nself-k8s backup` streams pg_dump from the postgres pod into the core
// backup layout and records per-table content fingerprints (EPIC D8, P7-DEPL-08).
//
// Inputs: --output, --tag, --release, --cluster, --namespace, --project-dir, --json.
// PROJECT_NAME and BACKUP_DIR come from the environment, else the project .env.
//
// Outputs: <BACKUP_DIR>/<PROJECT_NAME>_full_<YYYYMMDD_HHMMSS>[_<tag>].dump (or
// exactly --output) and its sidecar <dir>/.meta/<file>.json. Human mode prints
// Info lines and one Success line; --json prints one envelope whose data is
// {file, metadata_file, bytes, sha256, tables:[{schema,table,rows}]}.
//
// Constraints: all work is in internal/ops. This file maps *ops.Error (codes E730
// to E738) to the envelope, shared with restore. The exec seam is a package
// variable so tests can replace the pod. No password is ever on an argv.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/nself-org/cli/sdk/go/v2/output"
	"github.com/nself-org/nself-k8s/internal/k8s"
	"github.com/nself-org/nself-k8s/internal/ops"
)

// newExec finds the postgres pod and returns the exec into it. Tests replace it.
var newExec = func(ctx context.Context, kubeconfig, namespace, release string) (ops.Exec, error) {
	return ops.NewKubectl(ctx, kubeconfig, namespace, release)
}

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Back up the postgres database with content fingerprints",
	Long: `Stream pg_dump -Fc from the postgres pod into the nSelf backup layout.

The file is <BACKUP_DIR>/<PROJECT_NAME>_full_<YYYYMMDD_HHMMSS>[_<tag>].dump
(BACKUP_DIR defaults to ./backups; PROJECT_NAME and BACKUP_DIR come from the
environment or the project .env), mode 0600, and "nself backup restore" can
restore it. Next to it, .meta/<file>.json records the size, the sha256 and a
fingerprint (row count and content sum) of every table in every non-system
schema, read in the same snapshot as the dump. An existing file is never replaced.

Example:
  nself k8s backup
  nself k8s backup --tag before-upgrade --namespace nself --json`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		r := newInvocation(cmd, "backup")
		cluster, _ := cmd.Flags().GetString("cluster")
		namespace, _ := cmd.Flags().GetString("namespace")
		release, _ := cmd.Flags().GetString("release")
		outFile, _ := cmd.Flags().GetString("output")
		tag, _ := cmd.Flags().GetString("tag")
		projectDir, _ := cmd.Flags().GetString("project-dir")
		release = releaseOrDefault(release)
		cmd.SilenceErrors = true // main prints a human-mode error once
		if err := r.checkRelease(release, true); err != nil {
			return err
		}
		now := time.Now()
		path, project, err := ops.ResolveTarget(ops.TargetOptions{ProjectDir: projectDir, Output: outFile, Tag: tag, Getenv: os.Getenv, Now: now})
		if err != nil {
			return r.failOps(err)
		}
		r.info("Looking for the postgres pod of release " + release + "...")
		ex, err := newExec(cmd.Context(), cluster, namespace, release)
		if err != nil {
			return r.failOps(err)
		}
		r.info("Backing up to " + path + "...")
		res, err := ops.Backup(cmd.Context(), ops.BackupOptions{Exec: ex, Path: path, Project: project, Release: release, Namespace: namespace,
			Now: func() time.Time { return now }, Warn: r.warn})
		if err != nil {
			return r.failOps(err)
		}
		r.success(fmt.Sprintf("Backup written: %s (%d bytes, %d tables fingerprinted).", res.File, res.Bytes, len(res.Tables)))
		return r.done(map[string]any{"file": res.File, "metadata_file": res.MetaFile, "bytes": res.Bytes, "sha256": res.SHA256, "tables": tableRows(res.Tables)})
	},
}

// tableRows is the envelope view of fingerprints: schema, table and row count.
func tableRows(fps []ops.Fingerprint) []map[string]any {
	out := make([]map[string]any, 0, len(fps))
	for _, f := range fps {
		out = append(out, map[string]any{"schema": f.Schema, "table": f.Table, "rows": f.Rows})
	}
	return out
}

// opsDetail maps a coded ops error to its envelope detail.
func opsDetail(e *ops.Error) output.ErrorDetail {
	class := output.ClassInfra
	if e.Class == ops.ClassUser {
		class = output.ClassUser
	}
	return output.ErrorDetail{Code: e.Code, Class: class, Message: e.Message, Cause: e.Cause, Remediation: e.Remediation}
}

// failOps finishes a failed backup or restore. JSON mode writes the envelope and
// exits with the class status. Human mode prints one "Error:" line and exits 1,
// except the two integrity failures (E735 checksum, E737 fingerprints), which exit 2
// in both modes so a script can tell "restored data is not the backup" from a usage
// or environment error.
func (r *invocation) failOps(err error) error {
	oe, ok := err.(*ops.Error)
	if !ok {
		return r.fail(err)
	}
	d := opsDetail(oe)
	if r.json {
		return r.emit(d)
	}
	code := 1
	if oe.Code == ops.CodeChecksum || oe.Code == ops.CodeMismatch {
		code = output.ExitCodeFor(d.Class)
	}
	msg := oe.Code + " " + oe.Message
	if oe.Cause != "" {
		msg += ": " + oe.Cause
	}
	if oe.Remediation != "" {
		msg += " (" + strings.TrimSpace(oe.Remediation) + ")"
	}
	return &exitErr{code: code, msg: msg}
}

func init() {
	addJSONFlag(backupCmd)
	backupCmd.Flags().String("cluster", "", "Path to kubeconfig (default: kubectl reads the KUBECONFIG env var or ~/.kube/config)")
	backupCmd.Flags().String("namespace", "", "Kubernetes namespace of the release (default: the kubeconfig context's namespace)")
	backupCmd.Flags().String("output", "", "Write the dump to exactly this file instead of BACKUP_DIR (must not exist)")
	backupCmd.Flags().String("project-dir", ".", "nSelf project directory (holds .env; BACKUP_DIR is relative to it)")
	backupCmd.Flags().String("release", k8s.HelmReleaseName, "Helm release name")
	backupCmd.Flags().String("tag", "", "Label appended to the file name (letters, digits, '.', '_', '-')")
}
