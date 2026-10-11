// Purpose: `nself-k8s restore <file> --yes` streams a k8s backup into the postgres
// pod with pg_restore --clean --if-exists and succeeds only when the table
// fingerprints read afterwards equal the recorded ones (EPIC D8, P7-DEPL-08).
//
// Inputs: the dump file, --yes, --release, --cluster, --namespace, --json.
//
// Outputs: human mode: Info lines and, only after the fingerprints were compared and
// are equal, one Success line. --json: one envelope, data {file, verified:true,
// tables:[{schema,table,rows}]}. A failure is a coded error (E733 to E737, E734 when
// the dump or its sidecar is unusable).
// The file, sidecar and checksum checks (E734, E735) run before the kubectl and pod lookup.
//
// Constraints: destructive. Without --yes the command refuses with E403, exit 4,
// in both modes, before kubectl is looked up; there is no terminal prompt. Nothing
// reaches the cluster until the sidecar and the checksum of the file are verified.
package main

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/nself-org/cli/sdk/go/v2/output"
	"github.com/nself-org/nself-k8s/internal/k8s"
	"github.com/nself-org/nself-k8s/internal/ops"
)

var restoreCmd = &cobra.Command{
	Use:   "restore <file>",
	Short: "Restore a backup and verify it with the recorded fingerprints",
	Long: `Restore a backup made by "nself k8s backup" into the postgres pod.

This is destructive: pg_restore --clean --if-exists drops and recreates the
objects of the backup. Pass --yes to confirm; without it (terminal or not) the
command refuses with exit status 4 and nothing runs. Before anything runs, the
file's sha256 must equal the one recorded in .meta/<file>.json. After the restore
the fingerprint of every table must equal the recorded one; any difference is
E737 and the command never reports success. Stop other writers of the database
first: a write during the restore makes the fingerprints differ.

Example:
  nself k8s restore backups/myapp_full_20261011_040506.dump --yes
  nself k8s restore backups/myapp_full_20261011_040506.dump --yes --namespace nself --json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		r := newInvocation(cmd, "restore")
		cluster, _ := cmd.Flags().GetString("cluster")
		namespace, _ := cmd.Flags().GetString("namespace")
		release, _ := cmd.Flags().GetString("release")
		yes, _ := cmd.Flags().GetBool("yes")
		release = releaseOrDefault(release)
		cmd.SilenceErrors = true // main prints a human-mode error once
		if err := r.checkRelease(release, true); err != nil {
			return err
		}
		file := args[0]
		if !yes {
			return r.refuseRestore(file)
		}
		// The file, its sidecar and its checksum are checked first: a bad file never
		// touches the cluster (not even the pod lookup).
		r.info("Checking " + file + " against its metadata...")
		prep, err := ops.PrepareRestore(file)
		if err != nil {
			return r.failOps(err)
		}
		r.info("Looking for the postgres pod of release " + release + "...")
		ex, err := newExec(cmd.Context(), cluster, namespace, release)
		if err != nil {
			return r.failOps(err)
		}
		// Compare with the pod's real namespace (the --namespace flag may be empty).
		if w := prep.TargetWarning(release, ops.PodNamespace(ex)); w != "" {
			r.warn(w)
		}
		r.info("Restoring " + file + " (this replaces the database content)...")
		res, err := prep.Run(cmd.Context(), ex)
		if err != nil {
			return r.failOps(err)
		}
		r.success(fmt.Sprintf("Restored %s: %d tables verified against the backup fingerprints.", filepath.Base(file), len(res.Tables)))
		return r.done(map[string]any{"file": res.File, "verified": true, "tables": tableRows(res.Tables)})
	},
}

// restoreBlockedDetail is the refusal of an unconfirmed restore.
func restoreBlockedDetail(file string) output.ErrorDetail {
	return output.ErrorDetail{Code: CodeBlocked, Class: output.ClassDestructiveBlocked,
		Message:     fmt.Sprintf("restore of %q needs confirmation: nothing was changed", filepath.Base(file)),
		Remediation: "run it again with --yes to replace the database content with this backup"}
}

// refuseRestore reports an unconfirmed restore: E403, exit 4, in both modes.
func (r *invocation) refuseRestore(file string) error {
	d := restoreBlockedDetail(file)
	if r.json {
		return r.emit(d)
	}
	return &exitErr{code: output.ExitCodeFor(d.Class), msg: d.Code + " " + d.Message + "; " + d.Remediation}
}

func init() {
	addJSONFlag(restoreCmd)
	restoreCmd.Flags().String("cluster", "", "Path to kubeconfig (default: kubectl reads the KUBECONFIG env var or ~/.kube/config)")
	restoreCmd.Flags().String("namespace", "", "Kubernetes namespace of the release (default: the kubeconfig context's namespace)")
	restoreCmd.Flags().String("release", k8s.HelmReleaseName, "Helm release name")
	restoreCmd.Flags().BoolP("yes", "y", false, "Confirm that the database content is replaced (required)")
}
