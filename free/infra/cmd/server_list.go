package main

// Purpose: `nself infra server list` handler — renders server.List's result as a
// table (default) or one v1 envelope (--json).
// Inputs: --label-selector, --token, --token-env, --json.
// Outputs: a table of servers in the Hetzner project, or the envelope with them.

import (
	"fmt"

	"github.com/nself-org/nself-infra/internal/server"
	"github.com/nself-org/nself-infra/internal/ui"

	"github.com/spf13/cobra"
)

var serverListCmd = &cobra.Command{
	Use:   "list",
	Short: "List Hetzner Cloud servers",
	Example: `  nself infra server list
  nself infra server list --label-selector managed-by=nself-cli
  nself infra server list --json`,
	RunE: runServerList,
}

func init() {
	serverListCmd.Flags().String("label-selector", "", "filter by Hetzner label selector, e.g. managed-by=nself-cli")
	addJSONFlag(serverListCmd)
}

func runServerList(cmd *cobra.Command, args []string) error {
	r := begin(cmd)
	defer r.end()
	labelSelector, _ := cmd.Flags().GetString("label-selector")

	client, err := newServerClient(cmd)
	if err != nil {
		return r.fail(err)
	}

	servers, err := server.List(cmd.Context(), client, server.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return r.fail(fmt.Errorf("list servers: %w", err))
	}

	if r.json {
		if servers == nil {
			servers = []server.Server{}
		}
		return r.done(map[string]any{"servers": servers, "count": len(servers)})
	}

	if len(servers) == 0 {
		fmt.Println("No servers found.")
		return nil
	}

	table := ui.NewTable("ID", "NAME", "STATUS", "TYPE", "LOCATION", "IPV4")
	for _, s := range servers {
		table.AddRow(fmt.Sprintf("%d", s.ID), s.Name, s.Status, s.ServerType, s.Location, s.IPv4)
	}
	table.Render()
	return nil
}
