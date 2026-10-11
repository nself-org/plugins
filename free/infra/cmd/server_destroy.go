package main

// Purpose: `nself infra server destroy` handler — the safety-critical command
// G-011 exists for. Flag parsing and reporting only; every actual safety
// decision (backup gate, IP protection, delete ordering) lives in
// server.Destroy so it is unit-tested without a command layer in the way.
// Inputs: --id, --snapshot, --force-no-backup, --release-ip,
// --snapshot-timeout, --token, --token-env, --json.
// Outputs: printed confirmation (or the v1 envelope) naming the snapshot taken (if any) and
// which IPs were retained/released; a non-nil error (including
// server.ErrNoBackup) that leaves the server untouched.

import (
	"errors"
	"fmt"

	"github.com/nself-org/nself-infra/internal/server"
	"github.com/nself-org/nself-infra/internal/ui"

	"github.com/spf13/cobra"
)

var serverDestroyCmd = &cobra.Command{
	Use:   "destroy",
	Short: "Delete a Hetzner Cloud server",
	Long: `Delete a Hetzner Cloud server. Safe by default:

  - Refuses to run at all unless you pass --snapshot (takes one and waits
    for it to reach status=available before deleting anything) or
    --force-no-backup (explicit acknowledgment that no backup is taken).
  - Hetzner primary IPs default to auto_delete=true, so deleting the server
    would permanently destroy its IP too. This command sets
    auto_delete=false on the server's primary IP(s) first and prints which
    IPs were retained, unless --release-ip says to let them go with the
    server.

If the snapshot fails or never reaches status=available within
--snapshot-timeout, the server is NOT deleted.`,
	Example: `  nself infra server destroy --id 12345 --snapshot
  nself infra server destroy --id 12345 --snapshot --snapshot-timeout 20m
  nself infra server destroy --id 12345 --force-no-backup
  nself infra server destroy --id 12345 --snapshot --release-ip`,
	RunE: runServerDestroy,
}

func init() {
	f := serverDestroyCmd.Flags()
	f.Int64("id", 0, "server ID (required)")
	f.Bool("snapshot", false, "take a snapshot and verify it before deleting")
	f.Bool("force-no-backup", false, "proceed without any backup (explicit acknowledgment)")
	f.Bool("release-ip", false, "let the server's primary IP(s) be released with it, instead of retaining them")
	f.Duration("snapshot-timeout", server.DefaultSnapshotWait, "how long to wait for the snapshot to become available")
	addJSONFlag(serverDestroyCmd)
}

func runServerDestroy(cmd *cobra.Command, args []string) error {
	r := begin(cmd)
	defer r.end()
	id, _ := cmd.Flags().GetInt64("id")
	takeSnapshot, _ := cmd.Flags().GetBool("snapshot")
	forceNoBackup, _ := cmd.Flags().GetBool("force-no-backup")
	releaseIP, _ := cmd.Flags().GetBool("release-ip")
	snapshotTimeout, _ := cmd.Flags().GetDuration("snapshot-timeout")

	client, err := newServerClient(cmd)
	if err != nil {
		return r.fail(err)
	}

	if !r.json {
		ui.CommandHeader("nself infra server destroy", fmt.Sprintf("server %d", id))
		if takeSnapshot {
			ui.Info("Taking a snapshot and waiting for it to become available (up to " + snapshotTimeout.String() + ")...")
		}
	}

	result, err := server.Destroy(cmd.Context(), client, server.DestroyRequest{
		ServerID: id, TakeSnapshot: takeSnapshot, ForceNoBackup: forceNoBackup,
		ReleaseIP: releaseIP, SnapshotWait: snapshotTimeout,
	})
	if err != nil {
		if errors.Is(err, server.ErrNoBackup) {
			return r.fail(fmt.Errorf("%w — see 'nself infra server destroy --help'", err))
		}
		return r.fail(fmt.Errorf("destroy server %d: %w", id, err))
	}

	if r.json {
		return r.done(destroyData(id, result))
	}
	printDestroyResult(result)
	return nil
}

// destroyData is the envelope data of a completed destroy: what was done, from
// the result and never assumed from the flags.
func destroyData(id int64, result *server.DestroyResult) map[string]any {
	retained, released := result.RetainedIPs, result.ReleasedIPs
	if retained == nil {
		retained = []server.PrimaryIP{}
	}
	if released == nil {
		released = []server.PrimaryIP{}
	}
	return map[string]any{"server_id": id, "snapshot_id": result.SnapshotID, "retained_ips": retained, "released_ips": released}
}

// printDestroyResult reports exactly what Destroy did — never assumed from
// the request flags, since e.g. a server with no primary IPs retains none
// regardless of --release-ip.
func printDestroyResult(result *server.DestroyResult) {
	if result.SnapshotID != 0 {
		ui.Success(fmt.Sprintf("Snapshot %d verified available", result.SnapshotID))
	}
	for _, ip := range result.RetainedIPs {
		ui.Info(fmt.Sprintf("Retained primary IP %s (auto_delete=false)", ip.IP))
	}
	for _, ip := range result.ReleasedIPs {
		ui.Warn(fmt.Sprintf("Released primary IP %s (auto_delete=true, --release-ip was passed)", ip.IP))
	}
	ui.Success("Server destroyed")
}
